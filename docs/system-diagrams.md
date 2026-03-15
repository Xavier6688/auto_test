# AI 自动化测试平台图解

> 目的：基于现有设计文档，整理出更适合学习和讲解的系统架构图与流程图。  
> 适用场景：项目 onboarding、方案评审、架构复盘、开发前统一认知。

---

## 1. 系统全景图

```mermaid
flowchart LR
    U["用户<br/>测试工程师 / 测试经理 / 开发 / DevOps"]
    FE["前端管理平台<br/>React + TypeScript + Ant Design Pro"]
    GO["Go 主后端<br/>API 网关 / 调度中枢 / WebSocket Hub"]
    AI["Python AI 服务<br/>LLM 调用 / 用例生成 / POM 生成 / 脚本生成 / 失败分析"]
    EX["Python Test Worker<br/>Playwright 执行 / Allure 报告 / 结果收集"]

    DB[("PostgreSQL")]
    RD[("Redis")]
    MQ[("RabbitMQ")]
    OS[("MinIO")]
    EXT["外部系统<br/>OpenAI / Claude / JIRA / GitLab / GitHub / DingTalk / Email"]

    U --> FE
    FE -->|REST API / WebSocket| GO
    GO --> DB
    GO --> RD
    GO -->|异步任务| MQ
    GO -->|同步 gRPC| AI
    GO -->|状态查询 gRPC| EX
    AI --> DB
    AI --> MQ
    AI --> EXT
    EX --> DB
    EX --> MQ
    EX --> OS
    EX --> EXT
```

这个图适合先建立“谁和谁交互”的整体印象：
- 前端只直接连 Go 后端。
- Go 是编排中心，不直接执行 Playwright，也不直接承载主要 AI 逻辑。
- AI 服务和执行 Worker 各自承担专门职责，通过 MQ/gRPC 与 Go 协作。

---

## 2. 分层架构图

```mermaid
flowchart TB
    subgraph L1["接入层"]
        FE["Web 前端"]
        WEBHOOK["CI/CD Webhook"]
    end

    subgraph L2["业务编排层"]
        GO["Go Backend"]
        WS["WebSocket 推送"]
        API["REST API"]
        TD["Task Dispatcher"]
        GC["gRPC Client"]
    end

    subgraph L3["能力服务层"]
        AI["AI Service"]
        EX["Test Worker"]
    end

    subgraph L4["数据与基础设施层"]
        DB["PostgreSQL"]
        MQ["RabbitMQ"]
        RD["Redis"]
        OS["MinIO"]
    end

    FE --> API
    FE --> WS
    WEBHOOK --> API

    API --> GO
    WS --> GO
    TD --> GO
    GC --> GO

    GO --> AI
    GO --> EX
    GO --> MQ
    GO --> DB
    GO --> RD

    AI --> DB
    AI --> MQ
    EX --> DB
    EX --> MQ
    EX --> OS
```

这一层图更适合解释设计原则：
- 接入统一收口到 Go。
- 复杂耗时动作下沉到 Python 服务。
- 基础设施是共享资源，不是业务入口。

---

## 3. 服务职责图

```mermaid
mindmap
  root((平台服务职责))
    Go 主后端
      用户认证与权限
      项目管理
      需求管理
      用例管理
      执行调度
      结果聚合
      WebSocket 推送
    AI 服务
      用例生成
      POM 生成
      脚本生成
      用例校验
      定位器建议
      失败分析
      Prompt 版本管理
    Test Worker
      拉取脚本与 POM
      组装执行工作目录
      调用 pytest + Playwright
      收集截图 Trace 视频
      生成 Allure 报告
      回写执行结果
    前端
      管理台页面
      审核与编辑
      执行触发
      报告查看
      配置管理
```

---

## 4. 需求到用例的核心闭环

```mermaid
sequenceDiagram
    participant User as 用户
    participant FE as 前端
    participant GO as Go Backend
    participant DB as PostgreSQL
    participant MQ as RabbitMQ
    participant AI as AI Service

    User->>FE: 新建需求
    FE->>GO: POST /requirements
    GO->>DB: 保存 requirement
    GO-->>FE: 返回 requirement_id

    User->>FE: 触发 AI 生成用例
    FE->>GO: POST /requirements/:id/generate-cases
    GO->>DB: 创建 async_task
    GO->>MQ: 发布 ai.generate.cases
    GO-->>FE: 返回 task_id

    MQ->>AI: 消费生成任务
    AI->>AI: 解析需求 + 调用 LLM
    AI->>DB: 写入 test_cases
    AI->>MQ: 发布 results.ai

    MQ->>GO: 返回生成结果
    GO->>DB: 更新 async_task / requirement 状态
    GO-->>FE: WebSocket 推送生成进度和完成事件
    FE-->>User: 展示生成结果并进入审核
```

这个闭环是系统最早应该打通的 MVP 主线，也是最适合新人先理解的一条业务路径。

---

## 5. 用例到脚本到执行的主流程

```mermaid
flowchart TD
    A["已审核测试用例"] --> B["AI 生成 POM"]
    B --> C["元素仓库 / Page Object"]
    C --> D["AI 生成测试脚本"]
    D --> E["脚本进入可视化编辑器"]
    E --> F["保存为 steps_json / source_code"]
    F --> G["加入测试套件或直接执行"]
    G --> H["Go Backend 创建 execution"]
    H --> I["投递 executor.run 到 RabbitMQ"]
    I --> J["Test Worker 消费任务"]
    J --> K["组装工作空间"]
    K --> L["执行 pytest + Playwright"]
    L --> M["收集结果与附件"]
    M --> N["生成 Allure 报告"]
    N --> O["回写 execution_results / reports"]
```

---

## 6. CI/CD 自动化闭环

```mermaid
sequenceDiagram
    participant SCM as GitLab/GitHub
    participant GO as Go Backend
    participant DB as PostgreSQL
    participant MQ as RabbitMQ
    participant EX as Test Worker
    participant AI as AI Service
    participant JIRA as JIRA
    participant NT as 通知渠道

    SCM->>GO: Webhook 推送代码变更
    GO->>GO: 分析变更影响范围
    GO->>DB: 创建 execution
    GO->>MQ: 发布 executor.run

    MQ->>EX: 消费执行任务
    EX->>EX: 执行自动化测试
    EX->>DB: 写入执行结果
    EX->>MQ: 发布 results.execution

    MQ->>GO: 接收执行结果
    GO->>AI: AnalyzeFailure(仅失败用例)
    AI-->>GO: 根因分析 / 建议 / Bug 描述
    GO->>JIRA: 自动创建 Bug
    GO->>NT: 发送钉钉 / 邮件 / 企微通知
```

这个流程体现了平台最终目标：把测试从“手工触发”升级成“代码变更驱动的自动反馈系统”。

---

## 7. 通信方式图

```mermaid
flowchart LR
    FE["Frontend"] -->|REST API| GO["Go Backend"]
    FE -->|WebSocket| GO

    GO -->|gRPC 同步调用| AI["AI Service"]
    GO -->|gRPC 状态查询| EX["Executor Service / Worker"]

    GO -->|MQ 发布任务| MQ["RabbitMQ"]
    AI -->|MQ 结果回调| MQ
    EX -->|MQ 执行结果回调| MQ
    MQ --> GO
    MQ --> AI
    MQ --> EX

    GO -->|SQL / Migration| DB["PostgreSQL"]
    AI -->|DML| DB
    EX -->|DML| DB
```

可以用这张图快速解释为什么系统里同时存在 REST、WebSocket、gRPC、MQ：
- `REST` 负责页面请求
- `WebSocket` 负责实时进度
- `gRPC` 负责低延迟同步能力调用
- `MQ` 负责耗时任务解耦

---

## 8. 数据模型总览图

```mermaid
erDiagram
    users ||--o{ projects : owns
    users ||--o{ requirements : creates
    users ||--o{ test_cases : reviews
    users ||--o{ audit_logs : triggers

    projects ||--o{ requirements : contains
    projects ||--o{ test_cases : contains
    projects ||--o{ page_objects : contains
    projects ||--o{ element_repository : contains
    projects ||--o{ test_scripts : contains
    projects ||--o{ shared_components : contains
    projects ||--o{ test_datasets : contains
    projects ||--o{ environment_configs : contains
    projects ||--o{ test_suites : contains
    projects ||--o{ executions : contains
    projects ||--o{ tags : contains

    requirements ||--o{ test_cases : generates
    test_cases ||--o{ test_scripts : becomes
    test_datasets ||--o{ test_scripts : drives
    test_suites ||--o{ suite_scripts : maps
    test_scripts ||--o{ suite_scripts : included_in

    executions ||--o{ execution_results : has
    executions ||--|| reports : produces
    execution_results ||--|| bug_reports : may_create

    tags ||--o{ resource_tags : binds
```

如果只是理解业务主线，建议优先记住这一条：

`Project -> Requirement -> Test Case -> Test Script -> Execution -> Report/Bug`

---

## 9. 脚本编辑器内部模型流转图

```mermaid
flowchart LR
    A["测试需求 / 测试用例"] --> B["AI 生成脚本"]
    B --> C["统一中间表示<br/>JSON Steps Model"]
    C --> D["可视化编辑器"]
    C --> E["JSON -> Playwright 转换器"]
    D --> C
    E --> F["Python Playwright 代码"]
    F --> G["Test Worker 执行"]
```

这张图有助于理解为什么 `steps_json` 是核心：
- AI 输出不是直接给“最终代码”，而是先给统一 JSON 模型。
- 可视化编辑和执行都围绕同一份中间表示展开。

---

## 10. 学习顺序建议图

```mermaid
flowchart TD
    A["第 1 步：理解业务目标"] --> B["第 2 步：看系统全景图"]
    B --> C["第 3 步：看服务职责"]
    C --> D["第 4 步：理解需求 -> 用例闭环"]
    D --> E["第 5 步：理解脚本 -> 执行 -> 报告"]
    E --> F["第 6 步：理解数据库主链路"]
    F --> G["第 7 步：再看 CI/CD 自动化闭环"]
    G --> H["第 8 步：最后深入编辑器与高级能力"]
```

推荐的阅读顺序：
- 先读这份图解
- 再读 [architecture.md](/Users/xianming.huang/GoProject/auto_test/docs/architecture.md)
- 然后读 [database-design.md](/Users/xianming.huang/GoProject/auto_test/docs/database-design.md)
- 最后读 [visual-editor-design.md](/Users/xianming.huang/GoProject/auto_test/docs/visual-editor-design.md) 和 [api-contracts.md](/Users/xianming.huang/GoProject/auto_test/docs/api-contracts.md)

---

## 11. 一句话总结

这个系统本质上是一个以 Go 为调度中心、以 Python 承担 AI 与自动化执行能力、以消息队列解耦耗时任务、以统一数据模型和可视化编辑器承接业务协作的 AI 自动化测试平台。
