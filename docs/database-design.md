# 数据库设计文档

> 数据库类型：PostgreSQL 16+  
> Schema 管理：由 Go 主后端服务负责 Migration  
> Python 服务：只做 DML（INSERT/UPDATE/SELECT），不做 DDL

---

## 目录

- [1. ER 关系概览](#1-er-关系概览)
- [2. 表结构定义](#2-表结构定义)
- [3. 索引策略](#3-索引策略)
- [4. 状态机定义](#4-状态机定义)
- [5. 数据访问约定](#5-数据访问约定)

---

## 1. ER 关系概览

```
users
  │
  ├── 1:N ── projects
  │            │
  │            ├── 1:N ── requirements
  │            │            │
  │            │            └── 1:N ── test_cases ──── N:N ── tags (via resource_tags)
  │            │                         │
  │            │                         └── 1:N ── test_scripts ──► dataset_id → test_datasets
  │            │                                      │          ──► steps_json 引用 element_repository
  │            │                                      │          ──► steps_json 引用 shared_components
  │            │                                      │
  │            ├── 1:N ── page_objects                 │
  │            ├── 1:N ── element_repository           │
  │            ├── 1:N ── shared_components            │
  │            ├── 1:N ── test_datasets                │
  │            ├── 1:N ── environment_configs           │
  │            ├── 1:N ── tags                          │
  │            │                                       │
  │            ├── 1:N ── test_suites                   │
  │            │            └── N:N ── test_scripts (via suite_scripts)
  │            │                                       │
  │            └── 1:N ── executions                    │
  │                         │                          │
  │                         ├── 1:N ── execution_results ─┘
  │                         │             │
  │                         │             └── 1:1 ── bug_reports
  │                         │
  │                         └── 1:1 ── reports
  │
  ├── referenced by ── async_tasks (通用异步任务跟踪)
  ├── referenced by ── script_versions (脚本版本历史)
  ├── referenced by ── audit_logs (审计日志)
  └── referenced by ── llm_usage_logs (LLM调用记录)
```

---

## 2. 表结构定义

### 2.1 users — 用户表

```sql
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(100) UNIQUE NOT NULL,
    email           VARCHAR(200) UNIQUE NOT NULL,
    password_hash   VARCHAR(200) NOT NULL,
    display_name    VARCHAR(100),
    role            VARCHAR(20)  NOT NULL DEFAULT 'tester',
    avatar_url      VARCHAR(500),
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON COLUMN users.role IS 'admin: 管理员 | tester: 测试人员 | developer: 开发人员 | viewer: 只读用户';
COMMENT ON COLUMN users.status IS 'active: 活跃 | disabled: 禁用';
```

### 2.2 projects — 项目表

```sql
CREATE TABLE projects (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(200) NOT NULL,
    code            VARCHAR(50)  UNIQUE NOT NULL,
    description     TEXT,
    base_url        VARCHAR(500),
    repo_url        VARCHAR(500),
    repo_type       VARCHAR(20),
    jira_project_key VARCHAR(20),
    owner_id        BIGINT       NOT NULL REFERENCES users(id),
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    settings        JSONB        DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_projects_owner ON projects(owner_id);

COMMENT ON COLUMN projects.code IS '项目唯一编码，如 MALL、CRM';
COMMENT ON COLUMN projects.repo_type IS 'gitlab | github | gitee';
COMMENT ON COLUMN projects.settings IS '项目级配置：默认浏览器、超时时间、LLM模型等';
```

### 2.3 requirements — 需求表

```sql
CREATE TABLE requirements (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    title           VARCHAR(500) NOT NULL,
    description     TEXT         NOT NULL,
    module          VARCHAR(200),
    pages           JSONB,
    priority        VARCHAR(5)   DEFAULT 'P1',
    status          VARCHAR(20)  NOT NULL DEFAULT 'draft',
    created_by      BIGINT       NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_requirements_project ON requirements(project_id);
CREATE INDEX idx_requirements_status ON requirements(status);

COMMENT ON COLUMN requirements.pages IS '涉及的页面URL列表，JSON数组: ["/login", "/register"]';
COMMENT ON COLUMN requirements.status IS 'draft → parsing → parsed → generating → completed → archived';
```

### 2.4 test_cases — 测试用例表

```sql
CREATE TABLE test_cases (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    requirement_id  BIGINT       REFERENCES requirements(id),
    module          VARCHAR(200),
    title           VARCHAR(500) NOT NULL,
    priority        VARCHAR(5)   NOT NULL DEFAULT 'P1',
    test_type       VARCHAR(50)  NOT NULL DEFAULT 'functional',
    preconditions   JSONB,
    steps           JSONB        NOT NULL,
    expected_result TEXT         NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'generated',
    ai_confidence   DECIMAL(3,2),
    ai_model        VARCHAR(50),
    prompt_version  VARCHAR(20),
    reviewed_by     BIGINT       REFERENCES users(id),
    reviewed_at     TIMESTAMPTZ,
    created_by      BIGINT       REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_test_cases_project ON test_cases(project_id);
CREATE INDEX idx_test_cases_requirement ON test_cases(requirement_id);
CREATE INDEX idx_test_cases_status ON test_cases(status);
CREATE INDEX idx_test_cases_priority ON test_cases(priority);
CREATE INDEX idx_test_cases_module ON test_cases(module);

COMMENT ON COLUMN test_cases.priority IS 'P0: 冒烟 | P1: 核心 | P2: 一般 | P3: 边缘';
COMMENT ON COLUMN test_cases.test_type IS 'functional | boundary | negative | compatibility | performance';
COMMENT ON COLUMN test_cases.steps IS '操作步骤JSON数组: [{step, action, element, data, expected}]';
COMMENT ON COLUMN test_cases.status IS 'generated → reviewed → approved → scripted | rejected';
COMMENT ON COLUMN test_cases.ai_confidence IS 'AI生成置信度 0.00~1.00';
```

**steps 字段 JSON 结构**：

```json
[
    {
        "step": 1,
        "action": "打开登录页面",
        "element": "浏览器地址栏",
        "data": "https://example.com/login",
        "expected": "登录页面加载完成"
    },
    {
        "step": 2,
        "action": "输入用户名",
        "element": "用户名输入框",
        "data": "admin",
        "expected": "用户名显示在输入框中"
    }
]
```

### 2.5 element_repository — 元素仓库表（新增）

集中管理页面元素定位器，供可视化编辑器引用。与 `page_objects.elements` 的区别：元素仓库独立管理，一处修改全局生效。

```sql
CREATE TABLE element_repository (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    page_url        VARCHAR(500),
    page_name       VARCHAR(200),
    name            VARCHAR(200) NOT NULL,
    friendly_name   VARCHAR(200),
    locator_type    VARCHAR(20)  NOT NULL,
    locator_value   VARCHAR(500) NOT NULL,
    locator_options JSONB        DEFAULT '{}',
    fallback_locators JSONB      DEFAULT '[]',
    tag             VARCHAR(100),
    element_type    VARCHAR(50),
    screenshot_url  VARCHAR(500),
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    last_verified_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_elements_project ON element_repository(project_id);
CREATE INDEX idx_elements_page ON element_repository(project_id, page_name);
CREATE INDEX idx_elements_status ON element_repository(status);

COMMENT ON COLUMN element_repository.locator_type IS 'test_id | role | text | label | placeholder | css | xpath | id | class';
COMMENT ON COLUMN element_repository.locator_options IS '附加选项: {"name":"登录","exact":true}';
COMMENT ON COLUMN element_repository.fallback_locators IS '备用定位器列表(Self-Healing): [{"locator_type":"css","locator_value":"#btn"}]';
COMMENT ON COLUMN element_repository.element_type IS 'button | input | link | select | checkbox | text | image | other';
COMMENT ON COLUMN element_repository.status IS 'active | broken | deprecated';
```

### 2.6 page_objects — 页面对象表 (POM)

> 注：POM 中的元素定位器可通过 `element_id` 引用 `element_repository` 表中的记录。

```sql
CREATE TABLE page_objects (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    page_name       VARCHAR(200) NOT NULL,
    page_url        VARCHAR(500),
    class_name      VARCHAR(200) NOT NULL,
    file_path       VARCHAR(500),
    source_code     TEXT         NOT NULL,
    elements        JSONB,
    status          VARCHAR(20)  NOT NULL DEFAULT 'generated',
    version         INT          NOT NULL DEFAULT 1,
    ai_model        VARCHAR(50),
    prompt_version  VARCHAR(20),
    reviewed_by     BIGINT       REFERENCES users(id),
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(project_id, class_name, version)
);

CREATE INDEX idx_page_objects_project ON page_objects(project_id);
CREATE INDEX idx_page_objects_status ON page_objects(status);

COMMENT ON COLUMN page_objects.elements IS '页面元素定位器清单 JSON: [{name, locator_type, locator_value, description}]';
COMMENT ON COLUMN page_objects.status IS 'generated → reviewed → active → deprecated';
```

**elements 字段 JSON 结构**：

```json
[
    {
        "name": "username_input",
        "locator_type": "test_id",
        "locator_value": "username",
        "description": "用户名输入框"
    },
    {
        "name": "login_button",
        "locator_type": "role",
        "locator_value": "button:登录",
        "description": "登录按钮"
    }
]
```

### 2.7 test_scripts — 测试脚本表

```sql
CREATE TABLE test_scripts (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    case_id         BIGINT       REFERENCES test_cases(id),
    file_name       VARCHAR(300) NOT NULL,
    file_path       VARCHAR(500),
    script_type     VARCHAR(20)  NOT NULL DEFAULT 'visual',
    steps_json      JSONB,
    source_code     TEXT,
    depends_on_pom  BIGINT[]     DEFAULT '{}',
    dataset_id      BIGINT       REFERENCES test_datasets(id),
    status          VARCHAR(20)  NOT NULL DEFAULT 'generated',
    version         INT          NOT NULL DEFAULT 1,
    tags            VARCHAR(100)[] DEFAULT '{}',
    ai_model        VARCHAR(50),
    prompt_version  VARCHAR(20),
    reviewed_by     BIGINT       REFERENCES users(id),
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_test_scripts_project ON test_scripts(project_id);
CREATE INDEX idx_test_scripts_case ON test_scripts(case_id);
CREATE INDEX idx_test_scripts_status ON test_scripts(status);
CREATE INDEX idx_test_scripts_type ON test_scripts(script_type);
CREATE INDEX idx_test_scripts_tags ON test_scripts USING GIN(tags);

COMMENT ON COLUMN test_scripts.script_type IS 'visual: 可视化模式(steps_json) | code: 纯代码模式(source_code)';
COMMENT ON COLUMN test_scripts.steps_json IS '可视化模式的步骤数据，JSON结构详见 visual-editor-design.md';
COMMENT ON COLUMN test_scripts.source_code IS '纯代码模式的Python代码，或可视化模式自动生成的代码';
COMMENT ON COLUMN test_scripts.depends_on_pom IS '依赖的 page_objects.id 数组';
COMMENT ON COLUMN test_scripts.dataset_id IS '关联的测试数据集（数据驱动测试）';
COMMENT ON COLUMN test_scripts.status IS 'generated → reviewed → active → disabled';
```

### 2.8 shared_components — 公共组件表（新增）

```sql
CREATE TABLE shared_components (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    name            VARCHAR(200) NOT NULL,
    description     TEXT,
    parameters      JSONB        DEFAULT '[]',
    steps_json      JSONB        NOT NULL,
    tags            VARCHAR(100)[] DEFAULT '{}',
    usage_count     INT          DEFAULT 0,
    version         INT          NOT NULL DEFAULT 1,
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_by      BIGINT       REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(project_id, name, version)
);

CREATE INDEX idx_shared_components_project ON shared_components(project_id);

COMMENT ON COLUMN shared_components.parameters IS '组件输入参数定义: [{"name":"username","type":"string","required":true,"default":""}]';
COMMENT ON COLUMN shared_components.steps_json IS '组件步骤定义，格式同 test_scripts.steps_json';
```

### 2.9 test_suites — 测试套件表（新增）

```sql
CREATE TABLE test_suites (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    name            VARCHAR(300) NOT NULL,
    description     TEXT,
    tags            VARCHAR(100)[] DEFAULT '{}',
    execution_config JSONB       DEFAULT '{}',
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_by      BIGINT       REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_test_suites_project ON test_suites(project_id);

COMMENT ON COLUMN test_suites.execution_config IS '套件默认执行配置: {"browsers":["chromium"],"parallel":2,"retry":2}';
```

### 2.10 suite_scripts — 套件-脚本关联表（新增）

```sql
CREATE TABLE suite_scripts (
    id              BIGSERIAL PRIMARY KEY,
    suite_id        BIGINT       NOT NULL REFERENCES test_suites(id) ON DELETE CASCADE,
    script_id       BIGINT       NOT NULL REFERENCES test_scripts(id) ON DELETE CASCADE,
    sort_order      INT          DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(suite_id, script_id)
);

CREATE INDEX idx_suite_scripts_suite ON suite_scripts(suite_id);
CREATE INDEX idx_suite_scripts_script ON suite_scripts(script_id);
```

### 2.11 test_datasets — 测试数据集表（新增）

```sql
CREATE TABLE test_datasets (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    name            VARCHAR(200) NOT NULL,
    description     TEXT,
    columns         JSONB        NOT NULL,
    rows            JSONB        NOT NULL,
    row_count       INT          DEFAULT 0,
    created_by      BIGINT       REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_test_datasets_project ON test_datasets(project_id);

COMMENT ON COLUMN test_datasets.columns IS '列定义: ["username","password","expected_result"]';
COMMENT ON COLUMN test_datasets.rows IS '数据行: [{"username":"admin","password":"123","expected_result":"success"},...]';
```

### 2.12 environment_configs — 环境配置表（新增）

```sql
CREATE TABLE environment_configs (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    variables       JSONB        NOT NULL DEFAULT '{}',
    is_default      BOOLEAN      DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(project_id, name)
);

COMMENT ON COLUMN environment_configs.name IS '环境名称: dev | staging | prod | custom';
COMMENT ON COLUMN environment_configs.variables IS '环境变量: {"base_url":"https://staging.example.com","username":"test_user",...}';
```

### 2.13 tags — 标签表（新增）

```sql
CREATE TABLE tags (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    name            VARCHAR(100) NOT NULL,
    color           VARCHAR(7)   DEFAULT '#1890ff',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(project_id, name)
);

CREATE TABLE resource_tags (
    id              BIGSERIAL PRIMARY KEY,
    tag_id          BIGINT       NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    resource_type   VARCHAR(50)  NOT NULL,
    resource_id     BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    UNIQUE(tag_id, resource_type, resource_id)
);

CREATE INDEX idx_resource_tags_resource ON resource_tags(resource_type, resource_id);
CREATE INDEX idx_resource_tags_tag ON resource_tags(tag_id);

COMMENT ON COLUMN resource_tags.resource_type IS 'test_case | test_script | test_suite | shared_component';
```

### 2.14 script_versions — 脚本版本历史表（新增）

```sql
CREATE TABLE script_versions (
    id              BIGSERIAL PRIMARY KEY,
    script_id       BIGINT       NOT NULL REFERENCES test_scripts(id) ON DELETE CASCADE,
    version         INT          NOT NULL,
    script_type     VARCHAR(20)  NOT NULL,
    steps_json      JSONB,
    source_code     TEXT,
    change_type     VARCHAR(20)  NOT NULL,
    change_note     TEXT,
    changed_by      BIGINT       REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_script_versions_script ON script_versions(script_id, version DESC);

COMMENT ON COLUMN script_versions.change_type IS 'ai_generated | human_edited | ai_regenerated | rollback';
```

### 2.15 audit_logs — 审计日志表（新增）

```sql
CREATE TABLE audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT       REFERENCES users(id),
    username        VARCHAR(100),
    action          VARCHAR(50)  NOT NULL,
    resource_type   VARCHAR(50)  NOT NULL,
    resource_id     BIGINT,
    resource_name   VARCHAR(300),
    detail          JSONB,
    ip_address      VARCHAR(50),
    user_agent      VARCHAR(500),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_time ON audit_logs(created_at DESC);

COMMENT ON COLUMN audit_logs.action IS 'create | update | delete | review | approve | reject | execute | cancel | login | logout';
COMMENT ON COLUMN audit_logs.resource_type IS 'project | requirement | test_case | page_object | test_script | execution | ...';
COMMENT ON COLUMN audit_logs.detail IS '变更详情: {"before":{...},"after":{...}} 或 {"message":"..."}';
```

### 2.16 llm_usage_logs — LLM 调用记录表（新增）

```sql
CREATE TABLE llm_usage_logs (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       REFERENCES projects(id),
    task_id         VARCHAR(36),
    task_type       VARCHAR(50)  NOT NULL,
    model           VARCHAR(50)  NOT NULL,
    provider        VARCHAR(20)  NOT NULL,
    prompt_version  VARCHAR(20),
    prompt_tokens   INT          NOT NULL DEFAULT 0,
    completion_tokens INT        NOT NULL DEFAULT 0,
    total_tokens    INT          NOT NULL DEFAULT 0,
    estimated_cost  DECIMAL(10,6),
    duration_ms     INT,
    status          VARCHAR(20)  NOT NULL,
    error           TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_llm_usage_project ON llm_usage_logs(project_id);
CREATE INDEX idx_llm_usage_time ON llm_usage_logs(created_at DESC);
CREATE INDEX idx_llm_usage_model ON llm_usage_logs(model);

COMMENT ON COLUMN llm_usage_logs.task_type IS 'generate_cases | generate_pom | generate_script | analyze_failure | suggest_locator | validate_case';
COMMENT ON COLUMN llm_usage_logs.provider IS 'openai | claude | local';
COMMENT ON COLUMN llm_usage_logs.estimated_cost IS '预估费用（美元），根据 model 和 token 数计算';
COMMENT ON COLUMN llm_usage_logs.status IS 'success | failed | timeout';
```

### 2.17 executions — 执行记录表

```sql
CREATE TABLE executions (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT       NOT NULL REFERENCES projects(id),
    suite_id        BIGINT       REFERENCES test_suites(id),
    name            VARCHAR(300),
    trigger_type    VARCHAR(20)  NOT NULL,
    trigger_info    JSONB,
    environment     VARCHAR(50)  NOT NULL DEFAULT 'staging',
    base_url        VARCHAR(500),
    browsers        VARCHAR(50)[] DEFAULT '{chromium}',
    config          JSONB        DEFAULT '{}',
    selected_scripts BIGINT[]    DEFAULT '{}',
    status          VARCHAR(20)  NOT NULL DEFAULT 'queued',
    total_cases     INT          DEFAULT 0,
    passed_cases    INT          DEFAULT 0,
    failed_cases    INT          DEFAULT 0,
    skipped_cases   INT          DEFAULT 0,
    pass_rate       DECIMAL(5,2),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    created_by      BIGINT       REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_executions_project ON executions(project_id);
CREATE INDEX idx_executions_suite ON executions(suite_id);
CREATE INDEX idx_executions_status ON executions(status);
CREATE INDEX idx_executions_trigger ON executions(trigger_type);
CREATE INDEX idx_executions_created ON executions(created_at DESC);

COMMENT ON COLUMN executions.suite_id IS '关联的测试套件（通过套件触发执行时有值）';
COMMENT ON COLUMN executions.trigger_type IS 'manual | ci | schedule';
COMMENT ON COLUMN executions.trigger_info IS 'CI触发信息: {branch, commit_sha, commit_msg, author}';
COMMENT ON COLUMN executions.config IS '执行配置: {parallel, retry, timeout_ms, headed}';
COMMENT ON COLUMN executions.status IS 'queued → running → completed | failed | cancelled';
```

**trigger_info 字段 JSON 结构（CI 触发时）**：

```json
{
    "source": "gitlab",
    "branch": "feature/user-login",
    "commit_sha": "abc123def456",
    "commit_message": "feat: add login page",
    "author": "zhangsan",
    "pipeline_url": "https://gitlab.com/..."
}
```

### 2.18 execution_results — 执行结果表

```sql
CREATE TABLE execution_results (
    id              BIGSERIAL PRIMARY KEY,
    execution_id    BIGINT       NOT NULL REFERENCES executions(id),
    script_id       BIGINT       NOT NULL REFERENCES test_scripts(id),
    case_id         BIGINT       REFERENCES test_cases(id),
    test_name       VARCHAR(500),
    browser         VARCHAR(20),
    status          VARCHAR(20)  NOT NULL,
    duration_ms     INT,
    error_message   TEXT,
    stack_trace     TEXT,
    screenshot_url  VARCHAR(500),
    trace_url       VARCHAR(500),
    video_url       VARCHAR(500),
    log_output      TEXT,
    retry_count     INT          DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_exec_results_execution ON execution_results(execution_id);
CREATE INDEX idx_exec_results_status ON execution_results(status);
CREATE INDEX idx_exec_results_case ON execution_results(case_id);

COMMENT ON COLUMN execution_results.status IS 'passed | failed | skipped | error';
```

### 2.19 reports — 测试报告表

```sql
CREATE TABLE reports (
    id              BIGSERIAL PRIMARY KEY,
    execution_id    BIGINT       NOT NULL REFERENCES executions(id) UNIQUE,
    report_url      VARCHAR(500),
    report_type     VARCHAR(20)  DEFAULT 'allure',
    summary         JSONB        NOT NULL,
    pass_rate       DECIMAL(5,2),
    duration_ms     INT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON COLUMN reports.summary IS '{total, passed, failed, skipped, error, duration_ms}';
```

### 2.20 bug_reports — Bug 报告表

```sql
CREATE TABLE bug_reports (
    id                  BIGSERIAL PRIMARY KEY,
    project_id          BIGINT       NOT NULL REFERENCES projects(id),
    execution_result_id BIGINT       REFERENCES execution_results(id),
    external_system     VARCHAR(20),
    external_id         VARCHAR(100),
    external_url        VARCHAR(500),
    summary             VARCHAR(500) NOT NULL,
    description         TEXT,
    priority            VARCHAR(10),
    ai_analysis         TEXT,
    ai_category         VARCHAR(50),
    status              VARCHAR(20)  NOT NULL DEFAULT 'created',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bug_reports_project ON bug_reports(project_id);
CREATE INDEX idx_bug_reports_external ON bug_reports(external_system, external_id);
CREATE INDEX idx_bug_reports_status ON bug_reports(status);

COMMENT ON COLUMN bug_reports.external_system IS 'jira | gitlab | github';
COMMENT ON COLUMN bug_reports.external_id IS '外部系统的 Issue ID，如 PROJ-123';
COMMENT ON COLUMN bug_reports.ai_category IS 'element_not_found | assertion_failed | timeout | env_issue | unknown';
COMMENT ON COLUMN bug_reports.status IS 'created → confirmed → fixed → closed | duplicate';
```

### 2.21 async_tasks — 异步任务跟踪表

```sql
CREATE TABLE async_tasks (
    id              VARCHAR(36)  PRIMARY KEY,
    type            VARCHAR(50)  NOT NULL,
    reference_id    BIGINT,
    reference_type  VARCHAR(50),
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    progress        INT          DEFAULT 0,
    message         TEXT,
    result          JSONB,
    error           TEXT,
    worker_id       VARCHAR(100),
    retry_count     INT          DEFAULT 0,
    max_retries     INT          DEFAULT 3,
    timeout_at      TIMESTAMPTZ,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_async_tasks_status ON async_tasks(status);
CREATE INDEX idx_async_tasks_type ON async_tasks(type);
CREATE INDEX idx_async_tasks_ref ON async_tasks(reference_type, reference_id);
CREATE INDEX idx_async_tasks_timeout ON async_tasks(timeout_at) WHERE status = 'processing';

COMMENT ON COLUMN async_tasks.type IS 'generate_cases | generate_pom | generate_script | run_tests';
COMMENT ON COLUMN async_tasks.reference_type IS 'requirement | execution | page_object | test_script';
COMMENT ON COLUMN async_tasks.status IS 'pending → processing → completed | failed | timeout | cancelled';
COMMENT ON COLUMN async_tasks.progress IS '进度百分比 0~100';
COMMENT ON COLUMN async_tasks.worker_id IS '处理该任务的 Worker 标识（用于排查）';
```

---

## 3. 索引策略

### 3.1 索引设计原则

| 原则 | 说明 |
|------|------|
| 外键索引 | 所有外键字段建立索引 |
| 状态筛选 | 所有 status 字段建立索引（常用于过滤） |
| 时间排序 | 需要按时间排序的表建立时间降序索引 |
| 组合查询 | 常用的组合查询条件建立复合索引 |
| 条件索引 | 只对特定状态的数据建立索引（减少索引大小） |

### 3.2 补充业务索引

```sql
-- 常用查询：查询某项目下某模块的测试用例
CREATE INDEX idx_test_cases_project_module ON test_cases(project_id, module);

-- 常用查询：查询某项目下某状态的执行记录（按时间倒序）
CREATE INDEX idx_executions_project_status_time ON executions(project_id, status, created_at DESC);

-- 常用查询：查询某次执行中失败的结果
CREATE INDEX idx_exec_results_execution_status ON execution_results(execution_id, status);

-- 防重复 Bug：按外部系统+外部ID去重
CREATE UNIQUE INDEX idx_bug_reports_external_unique
    ON bug_reports(external_system, external_id)
    WHERE external_id IS NOT NULL;
```

---

## 4. 状态机定义

### 4.1 需求状态

```
draft ──► parsing ──► parsed ──► generating ──► completed
  │                                                 │
  └────────────────── archived ◄────────────────────┘
```

### 4.2 测试用例状态

```
generated ──► reviewed ──► approved ──► scripted
                 │
                 └──► rejected (可重新生成)
```

### 4.3 POM / 脚本状态

```
generated ──► reviewed ──► active ──► deprecated
```

### 4.4 执行状态

```
queued ──► running ──► completed
              │
              ├──► failed (执行异常，非用例失败)
              │
              └──► cancelled (手动取消)
```

### 4.5 异步任务状态

```
pending ──► processing ──► completed
                │
                ├──► failed (可重试)
                │       │
                │       └──► processing (重试)
                │
                └──► timeout (超时检测)
                        │
                        └──► failed (超过最大重试)
```

---

## 5. 数据访问约定

### 5.1 服务与数据库的关系

```
┌──────────────────┐     ┌───────────────┐     ┌──────────────────┐
│   Go 主后端       │     │ Python AI     │     │ Python Executor  │
│                  │     │               │     │                  │
│ · DDL (Migration)│     │ · INSERT      │     │ · INSERT         │
│ · 全部 CRUD     │     │   test_cases  │     │   exec_results   │
│ · 事务管理       │     │ · UPDATE      │     │ · UPDATE         │
│ · 数据校验       │     │   async_tasks │     │   executions     │
│                  │     │ · SELECT      │     │   async_tasks    │
│                  │     │   requirements│     │ · SELECT         │
│                  │     │               │     │   scripts, POMs  │
└────────┬─────────┘     └───────┬───────┘     └────────┬─────────┘
         │                       │                      │
         └───────────────────────┼──────────────────────┘
                                 ▼
                          PostgreSQL
```

### 5.2 Python 服务的数据访问规范

- 使用 SQLAlchemy 作为 ORM，但 **不使用 alembic 做 Migration**
- Migration 全部由 Go 服务管理，Python 服务的 SQLAlchemy Model 需要与 Go Migration 保持一致
- Python 服务的数据库连接使用只读账号（只对需要写入的表授予 INSERT/UPDATE 权限）
- 禁止 Python 服务执行 DROP/ALTER/TRUNCATE

### 5.3 并发控制

```sql
-- 异步任务状态更新使用乐观锁
UPDATE async_tasks
SET status = 'processing',
    worker_id = :worker_id,
    started_at = NOW(),
    updated_at = NOW()
WHERE id = :task_id
  AND status = 'pending'
RETURNING id;

-- 如果 RETURNING 为空，说明被其他 Worker 抢走了
```

### 5.4 updated_at 自动更新触发器

所有包含 `updated_at` 字段的表需要创建触发器：

```sql
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 为每个有 updated_at 的表创建触发器（以 test_scripts 为例）
CREATE TRIGGER set_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON requirements FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON test_cases FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON element_repository FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON page_objects FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON test_scripts FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON shared_components FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON test_suites FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON test_datasets FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON environment_configs FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON executions FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON bug_reports FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON async_tasks FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
```

### 5.5 归档与回收站策略

使用**状态字段**管理生命周期，不使用软删除（`deleted_at`）：

| 资源 | 归档状态 | 说明 |
|------|---------|------|
| 项目 | `status = 'archived'` | 归档后不可执行，不可创建新内容 |
| 需求 | `status = 'archived'` | 归档后关联用例仍可执行 |
| 用例 | `status = 'archived'` | 归档后不参与执行选择 |
| POM | `status = 'deprecated'` | 废弃后脚本引用时提示更新 |
| 脚本 | `status = 'disabled'` | 禁用后不参与执行，可重新启用 |
| 组件 | `status = 'deprecated'` | 废弃后引用它的脚本提示替换 |

前端提供 **"回收站"** 入口：
- 展示所有已归档/废弃/禁用的资源
- 支持恢复（改回 active 状态）
- 管理员可永久删除（物理删除）

REST API 中的 DELETE 操作统一为**归档**而非物理删除：
```
DELETE /api/v1/requirements/123
  → 实际执行: UPDATE requirements SET status='archived' WHERE id=123
```
