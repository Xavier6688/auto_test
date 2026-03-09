# 接口契约文档

> 本文档定义平台所有服务间通信的接口契约，包括：  
> - gRPC 接口（Go ↔ Python 同步通信）  
> - MQ 消息格式（Go ↔ Python 异步通信）  
> - REST API（前端 ↔ Go 主后端）

---

## 目录

- [1. gRPC 接口定义](#1-grpc-接口定义)
- [2. MQ 消息格式定义](#2-mq-消息格式定义)
- [3. REST API 设计](#3-rest-api-设计)
- [4. WebSocket 事件](#4-websocket-事件)
- [5. 通用约定](#5-通用约定)

---

## 1. gRPC 接口定义

### 1.1 公共类型 (common.proto)

```protobuf
syntax = "proto3";
package autotest;
option go_package = "auto_test_platform/backend/internal/grpcclient/pb";

message Empty {}

message StatusResponse {
    bool success = 1;
    string message = 2;
    string trace_id = 3;
}

enum Priority {
    PRIORITY_UNSPECIFIED = 0;
    P0 = 1;
    P1 = 2;
    P2 = 3;
    P3 = 4;
}

message HealthCheckRequest {
    string service = 1;
}

message HealthCheckResponse {
    enum ServingStatus {
        UNKNOWN = 0;
        SERVING = 1;
        NOT_SERVING = 2;
    }
    ServingStatus status = 1;
    string version = 2;
    int64 uptime_seconds = 3;
}
```

### 1.2 AI 服务接口 (ai_service.proto)

```protobuf
syntax = "proto3";
package autotest;
option go_package = "auto_test_platform/backend/internal/grpcclient/pb";

import "common.proto";

service AIService {
    // 健康检查
    rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);

    // 同步接口：分析测试失败原因（2~5s）
    rpc AnalyzeFailure(FailureRequest) returns (FailureAnalysis);

    // 同步接口：推荐元素定位策略（1~3s）
    rpc SuggestLocator(LocatorRequest) returns (LocatorSuggestion);

    // 同步接口：校验用例合理性（1~3s）
    rpc ValidateTestCase(ValidateRequest) returns (ValidateResponse);

    // 服务端流式：实时返回用例生成进度
    rpc StreamGenerateCases(StreamGenerateRequest) returns (stream GenerateProgress);
}

// ---------- 失败分析 ----------

message FailureRequest {
    string test_name = 1;
    string error_message = 2;
    string stack_trace = 3;
    string screenshot_url = 4;
    string page_url = 5;
    string expected_result = 6;
    string actual_result = 7;
}

message FailureAnalysis {
    string root_cause = 1;       // 根因分析
    string suggestion = 2;       // 修复建议
    float confidence = 3;        // 置信度 0.0~1.0
    string category = 4;         // 分类：见下方枚举
    string bug_summary = 5;      // 建议的 Bug 标题
    string bug_description = 6;  // 建议的 Bug 描述
}
// category 枚举值:
//   "element_not_found"  - 元素定位失败
//   "assertion_failed"   - 断言失败（真实 Bug 可能性高）
//   "timeout"            - 超时
//   "network_error"      - 网络错误
//   "env_issue"          - 环境问题
//   "script_error"       - 脚本本身的错误
//   "unknown"            - 无法判断

// ---------- 定位器建议 ----------

message LocatorRequest {
    string page_url = 1;
    string element_description = 2;  // 自然语言描述："登录按钮"
    string html_snippet = 3;         // 元素周围的 HTML 片段
    string failed_locator = 4;       // 失败的定位器（可选，用于修复场景）
}

message LocatorSuggestion {
    repeated LocatorOption options = 1;
}

message LocatorOption {
    string strategy = 1;       // "test_id" | "role" | "text" | "label" | "css" | "xpath"
    string locator = 2;        // 具体定位器表达式
    float reliability = 3;     // 可靠性评分 0.0~1.0
    string explanation = 4;    // 说明
}

// ---------- 用例校验 ----------

message ValidateRequest {
    string test_case_json = 1;  // 序列化的测试用例 JSON
    string requirement = 2;     // 原始需求（可选，用于对比覆盖度）
}

message ValidateResponse {
    bool is_valid = 1;
    float quality_score = 2;          // 质量评分 0.0~1.0
    repeated string issues = 3;       // 发现的问题
    repeated string suggestions = 4;  // 改进建议
}

// ---------- 流式生成进度 ----------

message StreamGenerateRequest {
    int64 requirement_id = 1;
    string description = 2;
    string base_url = 3;
    repeated string pages = 4;
    string prompt_version = 5;
}

message GenerateProgress {
    enum Stage {
        ANALYZING = 0;      // 正在分析需求
        GENERATING = 1;     // 正在生成用例
        VALIDATING = 2;     // 正在自校验
        COMPLETED = 3;      // 生成完成
        FAILED = 4;         // 生成失败
    }
    Stage stage = 1;
    int32 total = 2;         // 预计总数（GENERATING 阶段开始有值）
    int32 current = 3;       // 当前完成数
    string message = 4;      // 进度描述信息
    string error = 5;        // 错误信息（FAILED 阶段）
}
```

### 1.3 执行服务接口 (executor_service.proto)

```protobuf
syntax = "proto3";
package autotest;
option go_package = "auto_test_platform/backend/internal/grpcclient/pb";

import "common.proto";

service ExecutorService {
    // 健康检查
    rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);

    // 查询 Worker 集群状态
    rpc GetWorkerStatus(Empty) returns (WorkerStatusResponse);

    // 查询某次执行的实时进度
    rpc GetExecutionProgress(ExecutionProgressRequest) returns (ExecutionProgress);
}

message WorkerStatusResponse {
    int32 total_workers = 1;
    int32 busy_workers = 2;
    int32 idle_workers = 3;
    int32 queued_tasks = 4;
    repeated WorkerInfo workers = 5;
}

message WorkerInfo {
    string worker_id = 1;
    string status = 2;           // "idle" | "busy"
    string current_task_id = 3;  // 当前执行的任务（busy 时有值）
    int64 uptime_seconds = 4;
}

message ExecutionProgressRequest {
    int64 execution_id = 1;
}

message ExecutionProgress {
    int64 execution_id = 1;
    string status = 2;           // "queued" | "running" | "completed" | "failed"
    int32 total_cases = 3;
    int32 passed = 4;
    int32 failed = 5;
    int32 skipped = 6;
    int32 running = 7;
    string current_test = 8;     // 当前正在执行的测试名
    float elapsed_seconds = 9;
    float estimated_remaining = 10;  // 预估剩余时间（秒）
}
```

---

## 2. MQ 消息格式定义

### 2.1 队列拓扑

| Exchange | 类型 | 队列名 | Routing Key | 消费者 | 说明 |
|----------|------|--------|-------------|--------|------|
| autotest.direct | direct | ai.generate.cases | ai.generate.cases | AI Service | 用例生成 |
| autotest.direct | direct | ai.generate.pom | ai.generate.pom | AI Service | POM 生成 |
| autotest.direct | direct | ai.generate.script | ai.generate.script | AI Service | 脚本生成 |
| autotest.direct | direct | executor.run | executor.run | Test Worker | 测试执行 |
| autotest.direct | direct | results.ai | results.ai | Go Backend | AI 结果回调 |
| autotest.direct | direct | results.execution | results.execution | Go Backend | 执行结果回调 |
| autotest.dlx | fanout | deadletter | - | 监控/告警 | 死信队列 |

### 2.2 消息公共字段

所有消息必须包含以下字段：

```json
{
    "task_id": "string (UUID v4)",
    "type": "string (消息类型)",
    "trace_id": "string (UUID v4, 链路追踪)",
    "timestamp": "string (ISO 8601)",
    "version": "string (消息格式版本, 如 '1.0')"
}
```

### 2.3 AI 用例生成任务

**队列**：`ai.generate.cases`  
**Producer**：Go Backend  
**Consumer**：AI Service

```json
{
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "generate_cases",
    "trace_id": "660e8400-e29b-41d4-a716-446655440001",
    "timestamp": "2026-03-07T10:30:00Z",
    "version": "1.0",
    "payload": {
        "requirement_id": 123,
        "project_id": 1,
        "description": "用户登录功能：支持用户名密码登录，需要校验用户名和密码不为空...",
        "base_url": "https://staging.example.com",
        "pages": ["/login", "/dashboard"],
        "module": "用户认证",
        "prompt_version": "v1",
        "llm_config": {
            "model": "gpt-4o",
            "temperature": 0.3,
            "max_tokens": 4096
        }
    }
}
```

### 2.4 POM 生成任务

**队列**：`ai.generate.pom`  
**Producer**：Go Backend  
**Consumer**：AI Service

```json
{
    "task_id": "...",
    "type": "generate_pom",
    "trace_id": "...",
    "timestamp": "...",
    "version": "1.0",
    "payload": {
        "project_id": 1,
        "page_url": "https://staging.example.com/login",
        "page_name": "LoginPage",
        "crawl_page": true,
        "elements": [
            {
                "tag": "INPUT",
                "type": "text",
                "id": "username",
                "name": "username",
                "placeholder": "请输入用户名",
                "test_id": "username-input",
                "aria_label": "用户名"
            }
        ],
        "prompt_version": "v1"
    }
}
```

### 2.5 脚本生成任务

**队列**：`ai.generate.script`  
**Producer**：Go Backend  
**Consumer**：AI Service

```json
{
    "task_id": "...",
    "type": "generate_script",
    "trace_id": "...",
    "timestamp": "...",
    "version": "1.0",
    "payload": {
        "project_id": 1,
        "case_ids": [101, 102, 103],
        "pom_ids": [10, 11],
        "base_url": "https://staging.example.com",
        "prompt_version": "v1"
    }
}
```

### 2.6 测试执行任务

**队列**：`executor.run`  
**Producer**：Go Backend  
**Consumer**：Test Worker

```json
{
    "task_id": "...",
    "type": "run_tests",
    "trace_id": "...",
    "timestamp": "...",
    "version": "1.0",
    "payload": {
        "execution_id": 456,
        "project_id": 1,
        "script_ids": [201, 202, 203],
        "pom_ids": [10, 11],
        "config": {
            "browsers": ["chromium"],
            "base_url": "https://staging.example.com",
            "parallel": 2,
            "retry": 2,
            "timeout_ms": 60000,
            "headed": false,
            "record_video": false,
            "record_trace": true
        }
    }
}
```

### 2.7 AI 处理结果回调

**队列**：`results.ai`  
**Producer**：AI Service  
**Consumer**：Go Backend

```json
{
    "task_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "generate_cases_result",
    "trace_id": "660e8400-e29b-41d4-a716-446655440001",
    "timestamp": "2026-03-07T10:31:15Z",
    "version": "1.0",
    "payload": {
        "status": "completed",
        "requirement_id": 123,
        "generated_count": 15,
        "case_ids": [301, 302, 303],
        "ai_model": "gpt-4o",
        "prompt_version": "v1",
        "token_usage": {
            "prompt_tokens": 1200,
            "completion_tokens": 3500,
            "total_tokens": 4700
        },
        "duration_ms": 12500,
        "error": null
    }
}
```

### 2.8 测试执行结果回调

**队列**：`results.execution`  
**Producer**：Test Worker  
**Consumer**：Go Backend

```json
{
    "task_id": "...",
    "type": "execution_result",
    "trace_id": "...",
    "timestamp": "2026-03-07T10:45:30Z",
    "version": "1.0",
    "payload": {
        "execution_id": 456,
        "status": "completed",
        "summary": {
            "total": 20,
            "passed": 17,
            "failed": 2,
            "skipped": 1,
            "duration_ms": 185000
        },
        "report_url": "https://minio.internal/reports/456/index.html",
        "failed_tests": [
            {
                "result_id": 1001,
                "test_name": "test_login_invalid_password",
                "error_message": "AssertionError: Expected '密码错误' in error text",
                "screenshot_url": "https://minio.internal/screenshots/456/test_login_001.png",
                "trace_url": "https://minio.internal/traces/456/test_login_001.zip"
            }
        ]
    }
}
```

### 2.9 消息可靠性保障

| 机制 | 说明 |
|------|------|
| **持久化** | Exchange、Queue、Message 均开启持久化 |
| **手动 ACK** | Consumer 处理完毕后手动 ACK，失败时 NACK + requeue |
| **死信队列** | 超过最大重试次数的消息转入死信队列 |
| **TTL** | 消息默认 TTL: 30 分钟（防止无限堆积） |
| **幂等消费** | 通过 task_id + async_tasks 表实现幂等 |
| **消息版本** | version 字段，新旧版本 Consumer 可兼容处理 |

---

## 3. REST API 设计

### 3.1 通用约定

**Base URL**: `http://{host}:8080/api/v1`

**认证方式**: Bearer Token (JWT)

```
Authorization: Bearer <jwt_token>
```

**统一响应格式**:

```json
// 成功
{
    "code": 0,
    "message": "success",
    "data": { ... },
    "trace_id": "uuid"
}

// 失败
{
    "code": 10001,
    "message": "参数校验失败: title 不能为空",
    "data": null,
    "trace_id": "uuid"
}

// 分页
{
    "code": 0,
    "message": "success",
    "data": {
        "list": [ ... ],
        "total": 100,
        "page": 1,
        "page_size": 20
    },
    "trace_id": "uuid"
}
```

**错误码规范**:

| 范围 | 模块 |
|------|------|
| 10000~10999 | 通用错误（参数、认证、权限） |
| 11000~11999 | 项目模块 |
| 12000~12999 | 需求模块 |
| 13000~13999 | 用例模块 |
| 14000~14999 | POM / 元素仓库模块 |
| 15000~15999 | 脚本 / 可视化编辑器模块 |
| 16000~16999 | 执行模块 |
| 17000~17999 | 报告模块 |
| 18000~18999 | AI/任务模块 |
| 19000~19999 | 测试套件 / 数据集 / 组件模块 |
| 20000~20999 | 调试 / 录制模块 |
| 21000~21999 | 标签 / 环境配置模块 |

### 3.2 API 列表

#### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/login` | 用户登录，返回 JWT |
| POST | `/auth/register` | 用户注册 |
| POST | `/auth/refresh` | 刷新 Token |
| GET | `/auth/me` | 获取当前用户信息 |

#### 项目管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects` | 项目列表（分页） |
| POST | `/projects` | 创建项目 |
| GET | `/projects/:id` | 项目详情 |
| PUT | `/projects/:id` | 更新项目 |
| DELETE | `/projects/:id` | 归档项目 |
| GET | `/projects/:id/stats` | 项目统计（用例数、通过率等） |

#### 需求管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/requirements` | 需求列表 |
| POST | `/projects/:pid/requirements` | 创建需求 |
| GET | `/requirements/:id` | 需求详情 |
| PUT | `/requirements/:id` | 更新需求 |
| DELETE | `/requirements/:id` | 删除需求 |
| POST | `/requirements/:id/generate-cases` | **触发 AI 生成用例** |

#### 测试用例

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/cases` | 用例列表（支持按模块/优先级/状态筛选） |
| GET | `/cases/:id` | 用例详情 |
| PUT | `/cases/:id` | 编辑用例 |
| POST | `/cases/:id/review` | 审核用例（approve/reject） |
| POST | `/cases/batch-review` | 批量审核 |
| POST | `/cases/:id/generate-script` | **触发 AI 生成脚本** |

#### 页面对象 (POM)

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/page-objects` | POM 列表 |
| POST | `/projects/:pid/page-objects/crawl` | **触发页面元素采集 + POM 生成** |
| GET | `/page-objects/:id` | POM 详情（含源码） |
| PUT | `/page-objects/:id` | 编辑 POM 源码 |
| POST | `/page-objects/:id/review` | 审核 POM |

#### 测试脚本

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/scripts` | 脚本列表（支持按 script_type/tags/status 筛选） |
| POST | `/projects/:pid/scripts` | 创建空白脚本（可视化模式） |
| GET | `/scripts/:id` | 脚本详情（含 steps_json 或 source_code） |
| PUT | `/scripts/:id` | 编辑脚本（可视化模式保存 steps_json，代码模式保存 source_code） |
| POST | `/scripts/:id/review` | 审核脚本 |
| POST | `/scripts/batch-generate` | **批量生成脚本** |
| GET | `/scripts/:id/versions` | 脚本版本历史列表 |
| POST | `/scripts/:id/rollback` | 回滚到指定版本 |
| POST | `/scripts/:id/convert-to-code` | 可视化模式转为代码模式（不可逆） |
| GET | `/scripts/:id/code-preview` | 预览可视化脚本生成的 Python 代码 |

#### 脚本调试

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/scripts/:id/debug/start` | 启动调试会话（返回 debug_session_id） |
| POST | `/debug/:sid/next` | 执行下一步 |
| POST | `/debug/:sid/continue` | 执行到下一个断点 |
| POST | `/debug/:sid/run` | 执行全部剩余步骤 |
| POST | `/debug/:sid/stop` | 停止调试 |
| GET | `/debug/:sid/state` | 获取当前调试状态（变量、截图、当前步骤） |

#### 录制

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/scripts/:id/record/start` | 启动录制会话（返回浏览器预览 URL） |
| POST | `/record/:sid/stop` | 停止录制，返回录制到的步骤列表 |

#### 元素仓库

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/elements` | 元素列表（支持按 page_name/status 筛选） |
| POST | `/projects/:pid/elements` | 手动添加元素 |
| POST | `/projects/:pid/elements/crawl` | **触发页面元素自动采集** |
| GET | `/elements/:id` | 元素详情 |
| PUT | `/elements/:id` | 编辑元素定位器 |
| DELETE | `/elements/:id` | 删除元素 |
| POST | `/elements/verify` | 验证定位器有效性（返回匹配数、截图） |
| POST | `/elements/pick/start` | 启动元素拾取器（返回浏览器预览 URL） |
| POST | `/elements/pick/:sid/stop` | 停止拾取，返回选中元素信息 |

#### 公共组件

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/components` | 组件列表 |
| POST | `/projects/:pid/components` | 创建组件 |
| GET | `/components/:id` | 组件详情（含 steps_json 和参数定义） |
| PUT | `/components/:id` | 编辑组件 |
| DELETE | `/components/:id` | 废弃组件 |
| GET | `/components/:id/references` | 查看哪些脚本引用了此组件 |

#### 测试套件

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/suites` | 套件列表 |
| POST | `/projects/:pid/suites` | 创建套件 |
| GET | `/suites/:id` | 套件详情（含关联脚本列表） |
| PUT | `/suites/:id` | 编辑套件 |
| DELETE | `/suites/:id` | 删除套件 |
| PUT | `/suites/:id/scripts` | 更新套件关联的脚本列表 |
| POST | `/suites/:id/execute` | **按套件触发执行** |

#### 测试数据集

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/datasets` | 数据集列表 |
| POST | `/projects/:pid/datasets` | 创建数据集 |
| GET | `/datasets/:id` | 数据集详情（含所有行） |
| PUT | `/datasets/:id` | 编辑数据集 |
| DELETE | `/datasets/:id` | 删除数据集 |
| POST | `/projects/:pid/datasets/import` | 从 CSV/Excel 导入数据集 |

#### 环境配置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/env-configs` | 环境配置列表 |
| POST | `/projects/:pid/env-configs` | 创建环境配置 |
| PUT | `/env-configs/:id` | 编辑环境配置 |
| DELETE | `/env-configs/:id` | 删除环境配置 |

#### 标签

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/tags` | 标签列表 |
| POST | `/projects/:pid/tags` | 创建标签 |
| PUT | `/tags/:id` | 编辑标签 |
| DELETE | `/tags/:id` | 删除标签 |
| POST | `/tags/bindresource` | 为资源绑定标签 |
| DELETE | `/tags/unbindresource` | 解除资源标签绑定 |

#### 执行管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/projects/:pid/executions` | 执行记录列表 |
| POST | `/projects/:pid/executions` | **创建并触发执行** |
| GET | `/executions/:id` | 执行详情 |
| GET | `/executions/:id/results` | 执行结果列表 |
| POST | `/executions/:id/cancel` | 取消执行 |
| GET | `/executions/:id/progress` | 实时进度（也可用 WebSocket） |

#### 报告

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/executions/:id/report` | 获取执行报告 |
| GET | `/projects/:pid/reports` | 项目报告列表 |
| GET | `/projects/:pid/reports/trend` | 通过率趋势图数据 |

#### Webhook

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/webhooks/gitlab` | GitLab Webhook 接收 |
| POST | `/webhooks/github` | GitHub Webhook 接收 |
| POST | `/webhooks/generic` | 通用 Webhook（自定义触发） |

#### 异步任务

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/tasks/:id` | 查询任务状态和进度 |
| GET | `/tasks` | 任务列表（支持按类型/状态筛选） |

#### 审计日志

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/audit-logs` | 审计日志列表（支持按用户/操作/资源类型/时间筛选） |
| GET | `/audit-logs/:id` | 审计日志详情 |

#### LLM 用量

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/llm-usage/summary` | LLM 用量汇总（按模型/任务类型分组） |
| GET | `/llm-usage/trend` | Token 消耗趋势（按日/周/月） |
| GET | `/projects/:pid/llm-usage` | 项目级 LLM 用量 |

#### Dashboard

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/dashboard/overview` | 全局概览（项目数、用例数、近期执行） |
| GET | `/dashboard/projects/:pid` | 项目级仪表盘 |
| GET | `/dashboard/llm-cost` | LLM 成本仪表盘 |

### 3.3 关键 API 请求/响应示例

#### 触发 AI 生成用例

```
POST /api/v1/requirements/123/generate-cases
```

Request Body:
```json
{
    "prompt_version": "v1",
    "llm_model": "gpt-4o"
}
```

Response (202 Accepted):
```json
{
    "code": 0,
    "message": "用例生成任务已提交",
    "data": {
        "task_id": "550e8400-e29b-41d4-a716-446655440000",
        "status": "pending",
        "websocket_topic": "task:550e8400-e29b-41d4-a716-446655440000"
    },
    "trace_id": "..."
}
```

#### 创建并触发测试执行

```
POST /api/v1/projects/1/executions
```

Request Body:
```json
{
    "name": "登录模块回归测试",
    "environment": "staging",
    "base_url": "https://staging.example.com",
    "browsers": ["chromium"],
    "script_ids": [201, 202, 203],
    "config": {
        "parallel": 2,
        "retry": 2,
        "timeout_ms": 60000
    }
}
```

Response (202 Accepted):
```json
{
    "code": 0,
    "message": "测试执行任务已提交",
    "data": {
        "execution_id": 456,
        "task_id": "...",
        "status": "queued",
        "total_scripts": 3
    },
    "trace_id": "..."
}
```

---

## 4. WebSocket 事件

### 4.1 连接方式

```
ws://{host}:8080/ws?token={jwt_token}
```

### 4.2 事件格式

```json
{
    "event": "string (事件类型)",
    "task_id": "string (关联的任务ID)",
    "data": { ... },
    "timestamp": "string (ISO 8601)"
}
```

### 4.3 事件类型

#### 任务与执行事件

| 事件 | 触发时机 | data 内容 |
|------|---------|-----------|
| `task:progress` | 异步任务进度更新 | `{task_id, type, progress, message}` |
| `task:completed` | 异步任务完成 | `{task_id, type, result}` |
| `task:failed` | 异步任务失败 | `{task_id, type, error}` |
| `execution:started` | 测试开始执行 | `{execution_id, total_cases}` |
| `execution:case_done` | 单个用例执行完成 | `{execution_id, case_id, status, duration_ms}` |
| `execution:completed` | 整体执行完成 | `{execution_id, summary}` |
| `notification` | 系统通知 | `{title, message, level}` |

#### 调试事件

| 事件 | 触发时机 | data 内容 |
|------|---------|-----------|
| `debug:step_start` | 调试步骤开始执行 | `{session_id, step_id, step_description}` |
| `debug:step_done` | 调试步骤执行完成 | `{session_id, step_id, status, duration_ms, screenshot_url}` |
| `debug:paused` | 调试暂停 | `{session_id, step_id, reason: "breakpoint"/"step_mode"/"error"}` |
| `debug:variables` | 变量状态更新 | `{session_id, variables: {name: value}}` |
| `debug:screenshot` | 调试截图推送 | `{session_id, screenshot_url, page_url}` |
| `debug:console` | 浏览器控制台输出 | `{session_id, level, message}` |
| `debug:finished` | 调试会话结束 | `{session_id, total_steps, passed, failed}` |

#### 录制事件

| 事件 | 触发时机 | data 内容 |
|------|---------|-----------|
| `record:step_captured` | 录制到一个操作 | `{session_id, step: {type, target, params}}` |
| `record:page_navigated` | 页面跳转 | `{session_id, url}` |

#### 元素拾取事件

| 事件 | 触发时机 | data 内容 |
|------|---------|-----------|
| `pick:element_hovered` | 鼠标悬停元素 | `{session_id, element_info}` |
| `pick:element_selected` | 元素被选中 | `{session_id, locator_options: [{strategy, locator, reliability}]}` |

#### 页面变更检测事件

| 事件 | 触发时机 | data 内容 |
|------|---------|-----------|
| `element:changed` | 检测到页面元素变更 | `{project_id, page_url, changed_elements: [...]}` |
| `element:broken` | 元素定位器失效 | `{element_id, page_url, locator_type, locator_value}` |

---

## 5. 通用约定

### 5.1 时间格式

所有时间字段统一使用 **ISO 8601** 格式，带时区：

```
2026-03-07T10:30:00+08:00    (带时区偏移)
2026-03-07T02:30:00Z          (UTC)
```

数据库存储统一使用 `TIMESTAMPTZ`（带时区），应用层按用户所在时区转换显示。

### 5.2 分页参数

```
GET /api/v1/projects/1/cases?page=1&page_size=20&sort=created_at&order=desc
```

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| page | int | 1 | 页码，从 1 开始 |
| page_size | int | 20 | 每页数量，最大 100 |
| sort | string | created_at | 排序字段 |
| order | string | desc | asc / desc |

### 5.3 筛选参数

支持通过 query string 筛选：

```
GET /api/v1/projects/1/cases?status=approved&priority=P0&module=登录&keyword=密码
```

### 5.4 版本管理

- REST API 通过 URL 路径版本化：`/api/v1/`, `/api/v2/`
- gRPC 通过 package 版本化：`package autotest.v1;`
- MQ 消息通过 version 字段版本化：`"version": "1.0"`

### 5.5 幂等性

| 接口类型 | 幂等策略 |
|---------|---------|
| GET | 天然幂等 |
| POST（创建资源） | 客户端生成请求 ID（X-Request-ID header），服务端去重 |
| POST（触发任务） | 基于 task_id 去重，重复提交返回已有任务 |
| PUT | 天然幂等（整体替换） |
| DELETE | 天然幂等（删除不存在的资源返回成功） |
