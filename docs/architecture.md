# AI 驱动自动化测试平台 — 架构与技术方案

> 版本：v1.0  
> 日期：2026-03-07  
> 状态：方案设计阶段

---

## 目录

- [1. 项目概述](#1-项目概述)
- [2. 平台目标与核心能力](#2-平台目标与核心能力)
- [3. 整体架构](#3-整体架构)
- [4. 技术栈选型](#4-技术栈选型)
- [5. 微服务划分与职责](#5-微服务划分与职责)
- [6. 核心业务流程](#6-核心业务流程)
- [7. 服务间通信设计](#7-服务间通信设计)
- [8. 代码仓库策略](#8-代码仓库策略)
- [9. 项目目录结构](#9-项目目录结构)
- [10. 关键设计决策](#10-关键设计决策)
- [11. 非功能性需求](#11-非功能性需求)
- [12. 分阶段实施计划](#12-分阶段实施计划)

---

## 1. 项目概述

本平台是一个 **AI 驱动的端到端自动化测试平台**，核心理念是：

**输入需求 → AI 生成测试用例 → AI 生成 POM + Playwright 脚本 → 自动执行 → 输出报告 / 自动提交 Bug**

平台与 CI/CD 流水线深度集成，在前后端代码提交或版本发布时自动触发测试，实现测试左移和持续质量保障。

### 核心工作流

```
需求文本 ──► AI 解析 ──► 测试用例 ──► 人工审核
                                          │
                        ┌─────────────────┘
                        ▼
                 AI 生成 POM Page Object
                        │
                        ▼
                 AI 生成 Playwright 测试脚本 ──► 人工审核
                                                    │
                        ┌───────────────────────────┘
                        ▼
              手动执行 / CI Webhook 自动触发
                        │
                        ▼
              Docker 容器内并行执行 Playwright
                        │
                        ▼
              Allure 报告 + AI 失败分析
                        │
                ┌───────┴───────┐
                ▼               ▼
          自动创建 JIRA Bug   钉钉/邮件通知
```

---

## 2. 平台目标与核心能力

### 2.1 核心功能

| 编号 | 功能模块 | 描述 |
|------|---------|------|
| F1 | 需求管理 | 录入/导入需求，关联项目和页面 |
| F2 | AI 用例生成 | 基于需求描述，AI 自动生成结构化测试用例 |
| F3 | 页面对象管理 | AI 自动爬取页面元素，生成 POM (Page Object Model) 类 |
| F4 | 可视化脚本编辑器 | 图形化编辑测试脚本（选择定位器+操作+断言+条件循环），支持录制回放、元素拾取、单步调试 |
| F5 | 脚本管理 | AI 生成脚本（输出可视化 JSON 步骤模型），支持可视化编辑和纯代码编辑双模式 |
| F6 | 元素仓库 | 集中管理页面元素定位器，可视化编辑器直接引用，支持 Self-Healing |
| F7 | 可复用组件 | 公共步骤组（如登录流程）封装为可复用组件，多脚本共享 |
| F8 | 测试套件 | 将脚本组织为套件（冒烟套件、回归套件等），按套件触发执行 |
| F9 | 测试数据管理 | 数据集管理 + 环境配置，支持数据驱动测试 |
| F10 | 测试执行 | 支持手动触发、定时执行、CI/CD Webhook 触发 |
| F11 | 报告中心 | Allure 报告生成、历史趋势、通过率统计 |
| F12 | Bug 管理 | AI 分析失败原因，自动创建 JIRA/GitLab Issue |
| F13 | CI/CD 集成 | Webhook 接收代码提交/发布事件，智能选择测试范围 |
| F14 | 标签系统 | 灵活的标签体系，支持按标签筛选用例/脚本/执行 |
| F15 | LLM 成本管控 | Token 用量统计、多模型降级策略、项目级配额 |
| F16 | 通知 | 钉钉、企业微信、邮件多渠道通知 |
| F17 | 审计日志 | 全平台操作审计追踪 |

### 2.2 目标用户

- **测试工程师**：管理用例、审核 AI 生成内容、查看报告
- **测试管理者**：查看质量大盘、配置执行策略
- **开发工程师**：查看自动提交的 Bug、关联代码变更
- **DevOps**：配置 CI/CD 集成、管理执行环境

---

## 3. 整体架构

### 3.1 架构总览

```
┌─────────────────────────────────────────────────────────────────────┐
│                        前端 Web 管理平台                             │
│                     React + TypeScript + Ant Design Pro              │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ REST API + WebSocket
                           ▼
┌──────────────────────────────────────────────────────────────────────┐
│                     Go 主后端服务 (Gin/Echo)                          │
│                                                                      │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌───────┐ │
│  │用户认证 │ │项目管理│ │需求管理│ │用例管理│ │执行调度│ │报告查询│ │
│  │权限控制 │ │        │ │        │ │        │ │        │ │       │ │
│  └────────┘ └────────┘ └────────┘ └────────┘ └────────┘ └───────┘ │
│                                                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────┐                 │
│  │  MQ Producer  │  │ gRPC Client  │  │ WebSocket  │                 │
│  │ (任务分发)    │  │ (同步调用AI) │  │ Hub(推送)  │                 │
│  └──────┬───────┘  └──────┬───────┘  └────────────┘                 │
└─────────┼──────────────────┼────────────────────────────────────────┘
          │                  │
          │  RabbitMQ        │  gRPC
          │  (异步任务)      │  (同步调用)
          │                  │
  ┌───────▼──────┐    ┌─────▼────────────┐
  │ Python       │    │ Python            │
  │ AI 微服务    │    │ 测试执行 Worker   │
  │              │    │                   │
  │ · LLM 调用  │    │ · Playwright 执行 │
  │ · 用例生成  │    │ · 结果收集        │
  │ · POM 生成  │    │ · Allure 报告     │
  │ · 脚本生成  │    │ · Bug 自动提交    │
  │ · 失败分析  │    │ · 截图/Trace 上传 │
  └──────────────┘    └───────────────────┘
          │                    │
          ▼                    ▼
  ┌──────────────────────────────────┐
  │          共享基础设施             │
  │                                  │
  │  PostgreSQL  │  Redis  │  MinIO  │
  │  (主数据库)  │ (缓存)  │(对象存储)│
  └──────────────────────────────────┘
```

### 3.2 架构核心原则

| 原则 | 说明 |
|------|------|
| **Go 做调度中枢** | 接收请求、管理状态、协调流程、对接前端 |
| **Python 做执行引擎** | 运行 AI 推理和 Playwright 测试，发挥 Python 生态优势 |
| **MQ 异步解耦** | 耗时任务（生成、执行）通过消息队列异步处理，互不阻塞 |
| **gRPC 同步调用** | 需要实时返回的轻量级 AI 调用使用 gRPC |
| **共享数据库** | 各服务通过共享 PostgreSQL 交换数据，Go 负责 Schema 管理 |

---

## 4. 技术栈选型

### 4.1 技术栈总表

| 层级 | 技术 | 版本要求 | 选型理由 |
|------|------|---------|---------|
| **前端** | React + TypeScript | React 18+ | 组件生态丰富，TypeScript 保证类型安全 |
| **前端 UI** | Ant Design Pro | 6.x | 开箱即用的企业级后台框架 |
| **后端主服务** | Go + Gin | Go 1.22+ | 高并发、低资源占用、编译型语言部署简单 |
| **AI 微服务** | Python + FastAPI (HTTP health) | Python 3.11+ | AI/ML 生态无可替代 |
| **测试执行** | Python + Playwright | Playwright 1.50+ | 跨浏览器自动化，POM 支持成熟 |
| **测试框架** | pytest + pytest-playwright | - | Python 测试生态标准 |
| **LLM** | OpenAI API / Claude API | - | 支持切换多种大模型 |
| **数据库** | PostgreSQL | 16+ | 稳定可靠，JSONB 支持灵活结构 |
| **缓存** | Redis | 7+ | 会话缓存、任务锁 |
| **消息队列** | RabbitMQ | 3.13+ | 成熟可靠，支持消息确认/重试/死信 |
| **服务间通信** | gRPC + Protobuf | proto3 | 强类型契约、高性能、流式支持 |
| **对象存储** | MinIO (S3 兼容) | - | 存储截图/Trace/报告等文件 |
| **测试报告** | Allure Report | 2.x | 业界标准，丰富的可视化 |
| **Bug 管理** | JIRA API / GitLab Issues API | - | 自动创建缺陷 |
| **容器化** | Docker + Docker Compose | - | 统一运行环境 |
| **CI/CD** | Jenkins / GitLab CI / GitHub Actions | - | Webhook 触发测试流水线 |
| **监控** | Prometheus + Grafana | - | 服务健康监控 |
| **日志** | Loki + Grafana 或 ELK | - | 统一日志收集 |

### 4.2 为什么 Go + Python 混合架构

```
┌──────────────────────────────────────────────────────┐
│                     Go 擅长                           │
│                                                      │
│  · 高并发 HTTP/WebSocket 服务                        │
│  · 低内存占用（适合长期运行的调度服务）              │
│  · 单二进制部署，运维简单                            │
│  · 强类型系统减少运行时错误                          │
│  · goroutine 天然适合并发任务协调                    │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│                    Python 擅长                        │
│                                                      │
│  · AI/LLM 生态第一语言（openai, langchain, etc.）    │
│  · Playwright Python 绑定成熟稳定                    │
│  · pytest 生态丰富（参数化、fixture、插件）          │
│  · 快速原型验证 Prompt 效果                          │
│  · 数据处理和文本分析能力强                          │
└──────────────────────────────────────────────────────┘

>>> 结合方式：Go 做调度中枢，Python 做 AI + 测试执行引擎
>>> 通过 RabbitMQ (异步) + gRPC (同步) 通信
```

---

## 5. 微服务划分与职责

### 5.1 服务清单

本平台共包含 **4 个应用服务** + **4 个基础设施服务**：

| 服务名 | 语言 | 端口 | 职责 |
|--------|------|------|------|
| `backend` | Go | 8080 (HTTP) | 主后端：API 网关、业务逻辑、任务调度、WebSocket 推送 |
| `ai-service` | Python | 50051 (gRPC) | AI 引擎：LLM 调用、用例/POM/脚本生成、失败分析 |
| `test-worker` | Python | - (MQ Consumer) | 测试执行：Playwright 运行、结果收集、报告生成 |
| `frontend` | React | 3000 → Nginx 80 | Web 管理界面 |
| `postgres` | - | 5432 | 主数据库 |
| `redis` | - | 6379 | 缓存、会话、分布式锁 |
| `rabbitmq` | - | 5672 / 15672 | 消息队列 |
| `minio` | - | 9000 / 9001 | 对象存储（截图、报告、Trace 文件） |

### 5.2 Go 主后端服务 (backend)

**定位**：平台的调度中枢和 API 网关，所有外部请求的入口。

```
backend/
├── HTTP API 层
│   ├── 用户认证 & 权限（JWT）
│   ├── 项目管理 CRUD
│   ├── 需求管理 CRUD
│   ├── 测试用例管理 CRUD
│   ├── Page Object 管理 CRUD
│   ├── 测试脚本管理 CRUD
│   ├── 执行管理（创建/取消/查询进度）
│   ├── 报告查询
│   ├── Webhook 接收（GitLab/GitHub CI 触发）
│   └── 系统配置管理
│
├── 任务分发器 (TaskDispatcher)
│   ├── 发布异步任务到 RabbitMQ
│   ├── 监听 MQ 结果回调
│   └── 更新任务状态 + WebSocket 推送前端
│
├── gRPC 客户端
│   ├── 调用 AI 服务（同步：失败分析、定位器建议）
│   └── 调用执行服务（同步：查询 Worker 状态）
│
└── WebSocket Hub
    └── 向前端推送任务进度、执行状态实时更新
```

**核心特点**：
- 不直接调用 LLM，不运行 Playwright——只做调度和数据管理
- 负责数据库 Schema 管理（Migration），是数据模型的唯一所有者
- 负责外部系统集成的协调（JIRA、通知渠道等可由 Go 或 Python Worker 处理）

### 5.3 Python AI 微服务 (ai-service)

**定位**：封装所有 AI 相关能力，对外提供 gRPC 接口和 MQ 消费能力。

```
ai-service/
├── gRPC Server（同步接口，秒级响应）
│   ├── AnalyzeFailure     — 分析测试失败原因
│   ├── SuggestLocator     — 推荐元素定位策略
│   ├── ValidateTestCase   — 校验用例合理性
│   └── StreamGenerateCases — 流式返回生成进度
│
├── MQ Consumer（异步任务，分钟级）
│   ├── generate_cases     — 需求 → 测试用例
│   ├── generate_pom       — 页面元素 → POM 类
│   └── generate_script    — 用例 + POM → 测试脚本
│
├── LLM 客户端
│   ├── 支持 OpenAI / Claude / 本地模型切换
│   ├── 流式输出
│   ├── 重试 & 降级策略
│   └── Token 用量统计
│
└── Prompt 管理
    ├── 版本化 Prompt 模板
    ├── Prompt 注册表（按版本加载）
    └── 支持 A/B 测试不同 Prompt 版本
```

**核心特点**：
- 无状态服务，可水平扩缩容（replicas: N）
- 所有 AI 调用抽象为统一接口，底层模型可切换
- Prompt 模板版本化管理，支持灰度测试

### 5.4 Python 测试执行 Worker (test-worker)

**定位**：消费执行任务，在隔离环境中运行 Playwright 测试。

```
test-worker/
├── MQ Consumer
│   └── 监听 executor.run 队列
│
├── 工作空间管理
│   ├── 从数据库拉取脚本和 POM 文件
│   ├── 组装独立的测试工作目录
│   ├── 生成 conftest.py 和 pytest.ini
│   └── 安装依赖
│
├── 测试运行器
│   ├── 调用 pytest + playwright 执行
│   ├── 支持多浏览器（chromium/firefox/webkit）
│   ├── 支持并行执行（pytest-xdist）
│   ├── 失败自动重试
│   └── 超时控制
│
├── 结果收集器
│   ├── 解析 pytest 结果
│   ├── 写入 execution_results 表
│   ├── 上传截图/Trace/视频到 MinIO
│   └── 生成 Allure 报告并上传
│
└── 集成模块
    ├── JIRA 客户端 — 自动创建 Bug
    └── 通知客户端 — 钉钉/企微/邮件
```

**核心特点**：
- 每个任务使用独立工作目录，互不干扰
- shm_size 需要 ≥ 2GB（Playwright 浏览器需要）
- 可独立扩缩容——执行高峰期增加 Worker 数量

---

## 6. 核心业务流程

### 6.1 需求 → AI 生成测试用例

```
前端                    Go 主服务                 RabbitMQ              Python AI Worker
 │                         │                        │                        │
 │─ POST /requirements ──►│                        │                        │
 │  {title, description,   │                        │                        │
 │   base_url, pages}      │                        │                        │
 │                         │                        │                        │
 │                         │─ INSERT requirement ──► DB (status: draft)      │
 │                         │─ INSERT async_task ───► DB (status: pending)    │
 │                         │                        │                        │
 │                         │─ Publish ────────────►│                        │
 │                         │  ai.generate.cases     │                        │
 │                         │  {task_id, req_id,     │                        │
 │                         │   description, ...}    │                        │
 │                         │                        │                        │
 │◄─ 202 {task_id} ───────│                        │                        │
 │                         │                        │                        │
 │  [WebSocket 连接]       │                        │─ Consume ────────────►│
 │                         │                        │                        │
 │                         │                        │                ┌───────┤
 │                         │                        │                │解析需求│
 │                         │                        │                │调用LLM│
 │                         │                        │                │生成用例│
 │                         │                        │                └───────┤
 │                         │                        │                        │
 │                         │                        │                        │─ INSERT test_cases ► DB
 │                         │                        │                        │─ UPDATE async_task ► DB
 │                         │                        │                        │
 │                         │       results.ai   ◄───│──────── Publish ──────│
 │                         │                        │                        │
 │                         │◄── Consume ────────────│                        │
 │                         │                        │                        │
 │                         │─ UPDATE requirement ─► DB (status: completed)   │
 │                         │                        │                        │
 │◄─ WS: task_completed ──│                        │                        │
 │   {case_count: 15}      │                        │                        │
```

### 6.2 CI/CD Webhook → 自动执行 → 报告 + Bug

```
GitLab/GitHub            Go 主服务               RabbitMQ         Python Executor Worker
     │                       │                       │                      │
     │─ POST /webhooks/ci ──►│                       │                      │
     │  {branch, commit,     │                       │                      │
     │   changed_files}      │                       │                      │
     │                       │                       │                      │
     │                       │─ 分析变更影响模块      │                      │
     │                       │─ 匹配关联测试用例      │                      │
     │                       │─ INSERT execution ──► DB                     │
     │                       │                       │                      │
     │                       │─ Publish ───────────►│                      │
     │                       │  executor.run         │                      │
     │                       │                       │                      │
     │◄─ 200 OK ─────────── │                       │─ Consume ──────────►│
     │                       │                       │                      │
     │                       │                       │              ┌───────┤
     │                       │                       │              │1.拉取脚本+POM
     │                       │                       │              │2.组装工作空间
     │                       │                       │              │3.pytest执行
     │                       │                       │              │4.收集结果
     │                       │                       │              │5.Allure报告
     │                       │                       │              │6.上传MinIO
     │                       │                       │              │7.写入DB
     │                       │                       │              └───────┤
     │                       │                       │                      │
     │                       │                       │ ◄── Publish(result)──│
     │                       │◄── Consume ───────────│                      │
     │                       │                       │                      │
     │                       │─ 对失败用例:                                  │
     │                       │  gRPC AnalyzeFailure ─────────────────────► AI Service
     │                       │◄─ FailureAnalysis ───────────────────────── AI Service
     │                       │                       │                      │
     │                       │─ 创建 JIRA Bug        │                      │
     │                       │─ 发送钉钉/邮件通知    │                      │
     │                       │─ UPDATE execution ──► DB (status: completed) │
```

### 6.3 人工审核流程

AI 生成内容必须经过人工审核，这是质量保障的关键环节：

```
AI 生成 ──► status: generated ──► 测试人员审核
                                       │
                      ┌────────────────┼────────────────┐
                      ▼                ▼                ▼
                   通过             修改后通过         驳回
               status: approved   (编辑后 approved)  status: rejected
                      │                │                │
                      ▼                ▼                ▼
                 可用于执行        可用于执行      反馈给AI优化
```

---

## 7. 服务间通信设计

### 7.1 通信方式选择

| 场景 | 通信方式 | 原因 |
|------|---------|------|
| 用例生成 (10s~60s+) | **RabbitMQ 异步** | 耗时长，不能阻塞前端请求 |
| POM 生成 (10s~30s) | **RabbitMQ 异步** | 同上 |
| 脚本生成 (10s~60s+) | **RabbitMQ 异步** | 同上 |
| 测试执行 (分钟~小时级) | **RabbitMQ 异步** | 耗时最长，必须异步 |
| 失败原因分析 (2s~5s) | **gRPC 同步** | 轻量级，需要实时返回 |
| 定位器建议 (1s~3s) | **gRPC 同步** | 轻量级，需要实时返回 |
| 用例合理性校验 (1s~3s) | **gRPC 同步** | 轻量级，需要实时返回 |
| Worker 状态查询 | **gRPC 同步** | 实时状态查询 |

### 7.2 RabbitMQ 队列设计

```
Exchange: autotest.direct  (Direct 类型)
│
├── 队列: ai.generate.cases         ← 用例生成任务
│   Routing Key: ai.generate.cases
│
├── 队列: ai.generate.pom           ← POM 生成任务
│   Routing Key: ai.generate.pom
│
├── 队列: ai.generate.script        ← 脚本生成任务
│   Routing Key: ai.generate.script
│
├── 队列: executor.run               ← 测试执行任务
│   Routing Key: executor.run
│
├── 队列: results.ai                 ← AI 处理结果回调
│   Routing Key: results.ai
│
├── 队列: results.execution          ← 测试执行结果回调
│   Routing Key: results.execution
│
└── 队列: deadletter                 ← 死信队列（处理失败的消息）
    Routing Key: deadletter
```

**消息格式约定**（所有消息必须包含以下公共字段）：

```json
{
    "task_id": "uuid-v4",
    "type": "generate_cases | generate_pom | generate_script | run_tests",
    "trace_id": "uuid-v4",
    "timestamp": "2026-03-07T10:30:00Z",
    "version": "1.0",
    "payload": { ... }
}
```

### 7.3 gRPC 接口概要

详见 [接口契约文档](./api-contracts.md)。

```
service AIService {
    rpc AnalyzeFailure(FailureRequest) returns (FailureAnalysis);
    rpc SuggestLocator(LocatorRequest) returns (LocatorSuggestion);
    rpc ValidateTestCase(ValidateRequest) returns (ValidateResponse);
    rpc StreamGenerateCases(GenerateCasesRequest) returns (stream GenerateCaseProgress);
}

service ExecutorService {
    rpc GetWorkerStatus(WorkerStatusRequest) returns (WorkerStatusResponse);
    rpc GetExecutionProgress(ProgressRequest) returns (ExecutionProgress);
}
```

### 7.4 链路追踪

所有跨服务的调用必须携带 `trace_id`，确保问题可追踪：

```
前端请求
  └─ X-Trace-ID: abc-123
       └─ Go 主服务日志: trace_id=abc-123
            ├─ MQ 消息: {"trace_id": "abc-123", ...}
            │    └─ Python Worker 日志: trace_id=abc-123
            └─ gRPC metadata: trace_id=abc-123
                 └─ Python AI 日志: trace_id=abc-123
```

---

## 8. 代码仓库策略

本项目涉及 4 个技术栈不同的子项目（Go 后端、Python AI 服务、Python 测试执行器、React 前端），仓库组织方式有三种选择：

### 8.1 三种方案对比

| | Monorepo（单仓库） | Multi-repo（多仓库） | 折中方案 |
|---|---|---|---|
| **结构** | 所有服务放在一个 Git 仓库 | 每个服务独立 Git 仓库 | 按语言/职责分 2~3 个仓库 |
| **适合团队** | ≤ 10 人 | > 10 人，有明确分组 | 5~15 人 |
| **Proto 变更** | 一次 commit 同步所有服务 | 先发布 proto 仓库，再各自更新 | 同仓库内的服务可同步 |
| **CI/CD** | 统一流水线，一个 PR 可跨服务 | 各服务独立流水线、独立发布 | 按仓库独立 |
| **本地开发** | clone 一次，全部到手 | 需 clone 多个仓库 | clone 2~3 次 |
| **权限管理** | 粗粒度（CODEOWNERS 可部分解决） | 各仓库独立权限 | 适中 |
| **仓库体积** | 会逐渐增大 | 各仓库小而聚焦 | 适中 |

### 8.2 推荐策略：Monorepo 起步，按需拆分

```
阶段 1（0~6 个月）: Monorepo
├── 项目刚启动，接口频繁变动
├── 团队规模小，同一拨人维护多个服务
├── Proto 改动一次 commit 全部同步，效率最高
└── Docker Compose 编排路径简单

阶段 2（6~12 个月）: 前端独立
├── 前端变更频率与后端不同，先拆出去
├── auto-test-platform/       ← 仓库 1（Go + Python + Proto + Deploy）
└── auto-test-frontend/       ← 仓库 2（React）

阶段 3（团队扩大后）: 完全拆分
├── auto-test-backend/        ← Go 主后端
├── auto-test-ai-service/     ← Python AI 服务
├── auto-test-executor/       ← Python 测试执行器
├── auto-test-frontend/       ← React 前端
├── auto-test-proto/          ← 共享 Protobuf 定义
└── auto-test-deploy/         ← 部署配置
```

**当前项目采用阶段 1 的 Monorepo 方案**，理由：

1. **Proto 同步最方便**：改一个 `.proto` 文件，`make proto` 一次性生成 Go + Python 代码，一个 commit 搞定
2. **本地开发最简单**：新人 `git clone` 一次 → `make dev` 就能启动全部服务
3. **跨服务改动零成本**：不需要协调多个仓库的 PR 和版本兼容
4. **后续拆分无障碍**：各服务目录本身就是独立模块（各自有 `Dockerfile`、`go.mod` / `requirements.txt` / `package.json`），拆出去只需建新仓库移目录

> **何时考虑拆分**：当出现以下信号时，说明该拆了：
> - 不同服务由不同小组负责，互相 PR review 成为瓶颈
> - CI 流水线耗时过长（整体构建 > 15 分钟）
> - 需要对不同服务设置独立的发布节奏和权限

---

## 9. 项目目录结构

```
auto_test_platform/
│
├── proto/                                 # ★ gRPC Protobuf 定义（跨语言共享契约）
│   ├── common.proto                       #   公共类型定义
│   ├── ai_service.proto                   #   AI 服务接口
│   └── executor_service.proto             #   执行服务接口
│
├── backend/                               # ★ Go 主后端服务
│   ├── cmd/
│   │   └── server/
│   │       └── main.go                    #   应用入口
│   ├── internal/
│   │   ├── config/                        #   配置管理
│   │   ├── server/                        #   HTTP 服务器
│   │   ├── router/                        #   路由注册
│   │   ├── middleware/                    #   中间件（auth, cors, logger, trace）
│   │   ├── handler/                       #   HTTP Handler
│   │   │   ├── project.go
│   │   │   ├── requirement.go
│   │   │   ├── testcase.go
│   │   │   ├── pageobject.go
│   │   │   ├── script.go
│   │   │   ├── execution.go
│   │   │   ├── report.go
│   │   │   ├── webhook.go
│   │   │   └── user.go
│   │   ├── service/                       #   业务逻辑层
│   │   │   ├── project_svc.go
│   │   │   ├── requirement_svc.go
│   │   │   ├── testcase_svc.go
│   │   │   ├── execution_svc.go
│   │   │   ├── report_svc.go
│   │   │   └── task_dispatcher.go         #   ★ 核心任务分发器
│   │   ├── model/                         #   数据模型（ORM）
│   │   ├── repository/                    #   数据访问层
│   │   ├── mq/                            #   RabbitMQ Producer + Consumer
│   │   ├── grpcclient/                    #   gRPC 客户端
│   │   │   └── pb/                        #   protoc 生成的 Go 代码
│   │   ├── ws/                            #   WebSocket 推送
│   │   └── pkg/                           #   公共工具包
│   │       ├── resp/                      #   统一 API 响应格式
│   │       ├── errcode/                   #   统一错误码
│   │       ├── jwt/                       #   JWT 工具
│   │       └── trace/                     #   链路追踪
│   ├── migration/                         #   数据库迁移（SQL）
│   ├── go.mod
│   └── Dockerfile
│
├── ai_service/                            # ★ Python AI 微服务
│   ├── app/
│   │   ├── main.py                        #   入口（启动 gRPC Server + MQ Consumer）
│   │   ├── config.py
│   │   ├── grpc_server/                   #   gRPC 服务端
│   │   │   ├── server.py
│   │   │   ├── ai_servicer.py
│   │   │   └── generated/                 #   protoc 生成的 Python 代码
│   │   ├── mq/                            #   RabbitMQ Consumer + Publisher
│   │   ├── services/                      #   ★ 核心业务逻辑
│   │   │   ├── llm_client.py              #   LLM 统一调用封装
│   │   │   ├── case_generator.py          #   需求 → 用例
│   │   │   ├── page_crawler.py            #   页面元素采集
│   │   │   ├── pom_generator.py           #   元素 → POM 类
│   │   │   ├── script_generator.py        #   用例 + POM → 脚本
│   │   │   └── failure_analyzer.py        #   失败原因分析
│   │   ├── prompts/                       #   ★ Prompt 模板（版本管理）
│   │   │   ├── v1/
│   │   │   │   ├── case_generation.py
│   │   │   │   ├── pom_generation.py
│   │   │   │   ├── script_generation.py
│   │   │   │   └── failure_analysis.py
│   │   │   └── prompt_registry.py
│   │   ├── models/                        #   SQLAlchemy 数据模型
│   │   └── db/
│   ├── requirements.txt
│   ├── Dockerfile
│   └── tests/
│
├── test_executor/                         # ★ Python 测试执行 Worker
│   ├── app/
│   │   ├── main.py                        #   入口（MQ Consumer）
│   │   ├── config.py
│   │   ├── mq/
│   │   ├── executor/                      #   ★ 执行核心
│   │   │   ├── runner.py                  #   pytest 调用
│   │   │   ├── workspace.py              #   工作空间组装
│   │   │   └── result_collector.py        #   结果解析
│   │   ├── reporting/
│   │   │   ├── allure_adapter.py
│   │   │   └── report_uploader.py
│   │   ├── integrations/
│   │   │   ├── jira_client.py
│   │   │   ├── notification.py
│   │   │   └── storage.py                 #   MinIO 客户端
│   │   ├── models/
│   │   └── db/
│   ├── requirements.txt
│   ├── Dockerfile
│   └── tests/
│
├── frontend/                              # ★ React 前端
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Dashboard/                 #   数据大盘
│   │   │   ├── Projects/                  #   项目管理
│   │   │   ├── Requirements/              #   需求管理
│   │   │   ├── TestCases/                 #   用例管理（审核界面）
│   │   │   ├── PageObjects/               #   POM 管理
│   │   │   ├── Scripts/                   #   脚本管理（在线编辑器）
│   │   │   ├── Executions/                #   执行中心
│   │   │   ├── Reports/                   #   报告中心
│   │   │   └── Settings/                  #   系统设置
│   │   ├── components/
│   │   ├── services/                      #   API 调用封装
│   │   ├── stores/                        #   状态管理
│   │   └── utils/
│   ├── package.json
│   └── Dockerfile
│
├── deploy/                                # ★ 部署配置
│   ├── docker-compose.yml                 #   本地开发
│   ├── docker-compose.prod.yml            #   生产部署
│   ├── k8s/                               #   Kubernetes（可选）
│   ├── nginx/                             #   反向代理
│   └── monitoring/                        #   Prometheus + Grafana
│
├── scripts/                               # 开发工具脚本
│   ├── proto-gen.sh                       #   Protobuf 编译
│   ├── migrate.sh                         #   数据库迁移
│   └── dev-setup.sh                       #   环境初始化
│
├── docs/                                  # 项目文档
│   ├── architecture.md                    #   本文档
│   ├── database-design.md                 #   数据库设计
│   ├── api-contracts.md                   #   接口契约
│   └── deployment-guide.md                #   部署指南
│
└── Makefile                               # 统一构建命令
```

---

## 10. 关键设计决策

### 10.1 AI 生成质量保障

这是整个平台最关键的挑战，需要多层防护：

| 层级 | 策略 | 说明 |
|------|------|------|
| **Prompt 工程** | 版本化模板 + A/B 测试 | 持续优化 Prompt，对比不同版本效果 |
| **结构化输出** | 要求 LLM 返回 JSON 步骤模型 | 输出可视化编辑器可直接展示的格式 |
| **AI 自校验** | 生成后再调用 AI 校验一次 | ValidateTestCase 接口 |
| **人工审核** | 必须经过审核才能执行 | 初期不可跳过，是质量底线 |
| **置信度评分** | 记录 AI 生成置信度 | 高置信度用例逐步减少人工干预 |
| **反馈闭环** | 人工修改记录回传 | 用于 Fine-tuning 或 Few-shot 优化 |

### 10.2 LLM 多模型降级与成本控制

#### 降级策略

```
主模型: GPT-4o / Claude 3.5 Sonnet（质量优先）
   │
   │ 失败 / 超时 / 限流
   ▼
备用模型: GPT-4o-mini / Claude 3 Haiku（性价比）
   │
   │ 失败 / 超时 / 限流
   ▼
兜底模型: 本地部署模型（零费用但质量低）
   │
   │ 全部失败
   ▼
返回错误，通知运维
```

#### 不同任务使用不同模型

| 任务 | 推荐模型 | 原因 |
|------|---------|------|
| 用例生成 | 高质量模型（GPT-4o） | 需要深度理解需求 |
| POM 生成 | 中等模型（GPT-4o-mini） | 结构化映射，难度低 |
| 脚本生成 | 高质量模型（GPT-4o） | 需要理解用例逻辑 |
| 失败分析 | 中等模型（GPT-4o-mini） | 模式识别，速度优先 |
| 定位器建议 | 中等模型（GPT-4o-mini） | 简单推理 |

#### 成本控制

- 每次 LLM 调用记录 Token 消耗和费用到 `llm_usage_logs` 表
- 项目级月度配额管理，超出后需管理员审批
- Dashboard 展示 Token 消耗趋势和成本分布
- 告警：单日费用超过阈值时自动通知

### 10.3 POM 维护策略

```
┌─────────────────────────────────┐
│  页面元素变更检测（定时爬取）    │
│  对比元素快照 → 发现变化        │
└──────────┬──────────────────────┘
           │ 发现变化
           ▼
┌─────────────────────────────────┐
│  智能定位器修复 (Self-Healing)  │
│  AI 根据新 DOM 推荐替代定位器  │
└──────────┬──────────────────────┘
           │
           ▼
┌─────────────────────────────────┐
│  通知测试人员确认                │
│  自动更新 or 人工确认后更新     │
└─────────────────────────────────┘
```

### 10.4 测试执行稳定性

| 策略 | 实现方式 |
|------|---------|
| **失败重试** | pytest-rerunfailures，默认重试 2 次 |
| **智能等待** | Playwright auto-waiting，禁止 time.sleep 硬等待 |
| **环境隔离** | 每次执行在独立工作目录，互不干扰 |
| **超时控制** | 单用例超时 60s，整体执行超时根据用例数动态计算 |
| **Trace 录制** | 失败时保存 Playwright Trace，可回放排查 |
| **截图** | 每步操作截图 + 失败时全页面截图 |

### 10.5 CI/CD 增量测试策略

```
代码提交 → 分析变更文件 → 映射到业务模块 → 筛选关联测试用例

示例：
  变更: src/pages/login/LoginForm.tsx
  映射: login 模块
  执行: 所有 P0 用例 + login 模块的 P1/P2 用例

执行策略:
  · 每次 Push:   P0 全量 + 变更模块 P1
  · 每次 MR:     P0 全量 + 变更模块 P1/P2
  · 每日定时:    全量回归 P0/P1/P2
  · 版本发布前:  全量回归 P0/P1/P2/P3
```

### 10.6 任务幂等性

防止消息队列重复投递导致的重复执行：

```
消费消息时:
  1. 根据 task_id 查询 async_tasks 表
  2. 如果状态已是 completed/failed → 跳过（ACK 消息）
  3. 如果状态是 processing → 检查是否超时
     - 未超时 → 跳过（其他 Worker 在处理）
     - 已超时 → 重新处理
  4. 如果状态是 pending → 更新为 processing，开始执行
```

---

## 11. 非功能性需求

### 11.1 性能指标

| 指标 | 目标值 |
|------|--------|
| API 接口响应时间 (P95) | < 200ms |
| 用例生成延迟 (15条) | < 60s |
| POM 生成延迟 (单页面) | < 30s |
| 脚本生成延迟 (单用例) | < 30s |
| 单用例执行时间 | < 60s |
| 并行执行能力 | ≥ 10 用例/Worker |
| WebSocket 推送延迟 | < 500ms |

### 11.2 可用性

| 指标 | 目标值 |
|------|--------|
| 主服务可用性 | 99.9% |
| AI 服务可用性 | 99.5%（依赖外部 LLM） |
| 执行 Worker 恢复时间 | < 30s（自动重启） |
| 消息不丢失 | MQ 持久化 + 手动 ACK |

### 11.3 安全

| 领域 | 策略 |
|------|------|
| API 认证 | JWT Token，支持 RBAC 角色权限 |
| 服务间通信 | 内网通信 + gRPC TLS（生产环境） |
| LLM API Key | 环境变量注入，不入库 |
| 测试数据 | 脱敏处理，不使用生产真实数据 |
| 文件上传 | 大小限制 + 类型校验 |

### 11.4 监控与告警

```
┌───────────────────────────────────────────────────┐
│                 Grafana Dashboard                   │
│                                                     │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐            │
│  │服务健康  │  │MQ 队列  │  │执行统计  │            │
│  │状态面板  │  │堆积监控  │  │通过率趋势│            │
│  └─────────┘  └─────────┘  └─────────┘            │
└──────────────────────┬────────────────────────────┘
                       │
          ┌────────────┼────────────────┐
          ▼            ▼                ▼
    Prometheus      Loki          AlertManager
    (指标采集)    (日志收集)       (告警通知)
          ▲            ▲                │
          │            │                ▼
    各服务 /metrics   各服务 stdout   钉钉/邮件
```

**关键告警项**：
- 服务宕机
- MQ 队列堆积 > 阈值
- 任务执行超时
- AI 调用失败率 > 10%
- 测试通过率突然下降

---

## 12. 分阶段实施计划

### Phase 1：基础骨架 + AI 用例生成（4~6 周）

```
目标：跑通 "输入需求 → AI 生成用例 → 人工审核" 闭环

Week 1-2:
  · 搭建项目骨架（Go + Python + Docker Compose）
  · 编译 Protobuf，验证 gRPC 通信
  · PostgreSQL Schema + Migration
  · Go: 用户认证 + 项目/需求 CRUD API

Week 3-4:
  · Python AI Service: LLM 客户端 + 用例生成 Prompt
  · RabbitMQ 集成：Go 发布任务 → Python 消费
  · Go: 异步任务状态管理 + 结果回调
  · 前端: 需求录入 + 用例列表（基础版）

Week 5-6:
  · 前端: 用例审核界面（编辑/审批/驳回）
  · WebSocket 实时进度推送
  · Prompt 调优与测试
  · 基本的错误处理和重试
```

### Phase 2：POM + 可视化脚本编辑器 + 手动执行（6~8 周）

```
目标：跑通 "用例 → POM → 可视化脚本 → 手动执行 → 报告" 闭环

Week 1-2:
  · Python: 页面元素爬取 (page_crawler)
  · Python: POM 生成 Prompt + 代码生成
  · Go: PageObject / Script / Element Repository CRUD API
  · 数据库: 新增元素仓库、测试套件、数据集、公共组件等表
  · 前端: POM 管理 + 元素仓库管理

Week 3-4:
  · Python: AI 脚本生成输出 JSON 步骤模型（而非 Python 代码）
  · Python test-worker: JSON → Playwright 代码转换器
  · 前端: ★ 可视化脚本编辑器核心（步骤卡片、拖拽排序、操作/定位器选择）
  · 前端: 条件判断 + 循环 + 断言的可视化编辑

Week 5-6:
  · 前端: 元素拾取器（从页面选取元素生成定位器）
  · 前端: 公共组件管理 + 组件调用
  · 前端: 测试数据集管理 + 数据驱动配置
  · 前端: 标签系统
  · Python test-worker: 工作空间组装 + pytest 执行
  · Allure 报告生成与展示

Week 7-8:
  · 前端: 录制回放功能
  · 前端: 单步调试模式
  · Go: 执行管理（手动触发、测试套件、查看进度）
  · Go: 测试套件管理 API
  · 前端: 执行中心 + 报告中心
  · MinIO 文件存储集成（截图/Trace/报告）
  · 端到端联调测试
```

### Phase 3：CI/CD 集成 + Bug 自动提交（3~4 周）

```
目标：实现 CI 触发 → 自动执行 → 自动提 Bug → 通知的全自动闭环

Week 1-2:
  · Go: Webhook 接收（GitLab/GitHub）
  · 变更影响分析 + 增量测试选择
  · JIRA / GitLab Issues API 集成
  · AI 失败原因分析 (gRPC AnalyzeFailure)

Week 3-4:
  · Bug 自动创建 + 去重逻辑
  · 通知集成（钉钉/企微/邮件）
  · 定时执行（cron 调度）
  · CI/CD 流水线配置模板
```

### Phase 4：智能化 + 优化（持续迭代）

```
目标：提升 AI 生成质量、平台稳定性和用户体验

持续进行:
  · 智能定位器修复 (Self-Healing)
  · 页面变更自动检测
  · Prompt 持续优化 + A/B 测试
  · AI 反馈闭环（人工修改 → 模型优化）
  · LLM 成本优化（多模型分任务分配）
  · 数据统计大盘（质量趋势、AI 准确率、LLM 成本）
  · 脚本版本历史 + 回滚
  · 审计日志完善
  · 归档/回收站机制
  · Kubernetes 部署 + 弹性伸缩
  · Prometheus + Grafana 监控体系
  · 性能优化
```

---

## 附录

### A. 相关文档索引

| 文档 | 路径 | 内容 |
|------|------|------|
| 数据库设计 | [docs/database-design.md](./database-design.md) | 完整表结构、索引、ER 关系 |
| 接口契约 | [docs/api-contracts.md](./api-contracts.md) | gRPC Proto、MQ 消息格式、REST API |
| 可视化编辑器设计 | [docs/visual-editor-design.md](./visual-editor-design.md) | JSON 步骤模型、步骤类型、录制回放、调试模式 |
| 部署指南 | [docs/deployment-guide.md](./deployment-guide.md) | Docker Compose、K8s、环境变量 |

### B. 参考资料

- [Playwright Python 文档](https://playwright.dev/python/)
- [gRPC Go 快速入门](https://grpc.io/docs/languages/go/quickstart/)
- [gRPC Python 快速入门](https://grpc.io/docs/languages/python/quickstart/)
- [RabbitMQ 教程](https://www.rabbitmq.com/tutorials)
- [Allure Report](https://allurereport.org/)
- [Page Object Model 模式](https://playwright.dev/python/docs/pom)
