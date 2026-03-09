# 部署指南

> 本文档涵盖本地开发环境搭建、Docker Compose 部署、以及生产环境部署方案。

---

## 目录

- [1. 环境要求](#1-环境要求)
- [2. 本地开发环境搭建](#2-本地开发环境搭建)
- [3. Docker Compose 部署（开发/测试）](#3-docker-compose-部署开发测试)
- [4. 生产环境部署](#4-生产环境部署)
- [5. 环境变量配置](#5-环境变量配置)
- [6. Makefile 命令参考](#6-makefile-命令参考)
- [7. 监控与日志](#7-监控与日志)
- [8. 常见问题排查](#8-常见问题排查)

---

## 1. 环境要求

### 1.1 开发环境

| 工具 | 版本要求 | 用途 |
|------|---------|------|
| Go | 1.22+ | 主后端开发 |
| Python | 3.11+ | AI 服务 + 测试执行 |
| Node.js | 20 LTS+ | 前端开发 |
| Docker | 24+ | 容器化运行 |
| Docker Compose | 2.20+ | 服务编排 |
| protoc | 3.21+ | Protobuf 编译 |
| protoc-gen-go | latest | Go gRPC 代码生成 |
| grpcio-tools | latest | Python gRPC 代码生成 |
| Make | - | 构建命令 |

### 1.2 生产环境

| 组件 | 最低配置 | 推荐配置 |
|------|---------|---------|
| Go Backend | 1 核 1GB | 2 核 4GB |
| AI Service (×2) | 1 核 2GB | 2 核 4GB |
| Test Worker (×3) | 2 核 4GB | 4 核 8GB |
| PostgreSQL | 2 核 4GB | 4 核 8GB + SSD |
| RabbitMQ | 1 核 1GB | 2 核 2GB |
| Redis | 1 核 512MB | 1 核 1GB |
| MinIO | 1 核 1GB | 2 核 2GB + 大容量存储 |

> Test Worker 内存需求较高，因为 Playwright 浏览器实例占用较多内存（每个浏览器约 300-500MB）。

---

## 2. 本地开发环境搭建

### 2.1 克隆项目

```bash
git clone https://github.com/your-org/auto_test_platform.git
cd auto_test_platform
```

### 2.2 启动基础设施

先启动数据库、消息队列等基础设施：

```bash
make infra
# 等价于: cd deploy && docker-compose up -d postgres redis rabbitmq minio
```

### 2.3 编译 Protobuf

```bash
make proto
```

该命令会同时生成 Go 和 Python 的 gRPC 代码：
- Go 代码输出到 `backend/internal/grpcclient/pb/`
- Python 代码输出到 `ai_service/app/grpc_server/generated/`

### 2.4 数据库迁移

```bash
make migrate
# 等价于: cd backend && go run cmd/migrate/main.go up
```

### 2.5 启动各服务（开发模式）

在不同终端分别启动：

```bash
# 终端 1: Go 主后端（支持热重载，需安装 air）
cd backend && air

# 终端 2: Python AI 服务
cd ai_service && pip install -r requirements.txt && python -m app.main

# 终端 3: Python 测试执行 Worker
cd test_executor && pip install -r requirements.txt && python -m app.main

# 终端 4: 前端
cd frontend && npm install && npm run dev
```

### 2.6 验证服务启动

```bash
# Go 后端健康检查
curl http://localhost:8080/api/v1/health

# AI 服务 gRPC 健康检查（需安装 grpcurl）
grpcurl -plaintext localhost:50051 autotest.AIService/HealthCheck

# RabbitMQ 管理界面
# 打开浏览器访问 http://localhost:15672（用户名/密码: autotest/autotest_dev）

# MinIO 管理界面
# 打开浏览器访问 http://localhost:9001（用户名/密码: autotest/autotest_dev）
```

---

## 3. Docker Compose 部署（开发/测试）

### 3.1 docker-compose.yml

```yaml
version: "3.9"

services:
  # ===== 基础设施 =====

  postgres:
    image: postgres:16-alpine
    container_name: autotest-postgres
    environment:
      POSTGRES_DB: autotest
      POSTGRES_USER: autotest
      POSTGRES_PASSWORD: ${DB_PASSWORD:-autotest_dev}
    ports:
      - "5432:5432"
    volumes:
      - pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U autotest"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: autotest-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  rabbitmq:
    image: rabbitmq:3.13-management-alpine
    container_name: autotest-rabbitmq
    environment:
      RABBITMQ_DEFAULT_USER: ${MQ_USER:-autotest}
      RABBITMQ_DEFAULT_PASS: ${MQ_PASSWORD:-autotest_dev}
    ports:
      - "5672:5672"
      - "15672:15672"
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 30s
      timeout: 10s
      retries: 5

  minio:
    image: minio/minio:latest
    container_name: autotest-minio
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ACCESS_KEY:-autotest}
      MINIO_ROOT_PASSWORD: ${MINIO_SECRET_KEY:-autotest_dev}
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio_data:/data
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ===== 应用服务 =====

  backend:
    build:
      context: ../backend
      dockerfile: Dockerfile
    container_name: autotest-backend
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    environment:
      - APP_ENV=development
      - DB_DSN=postgres://autotest:${DB_PASSWORD:-autotest_dev}@postgres:5432/autotest?sslmode=disable
      - REDIS_URL=redis://redis:6379/0
      - RABBITMQ_URL=amqp://${MQ_USER:-autotest}:${MQ_PASSWORD:-autotest_dev}@rabbitmq:5672/
      - AI_GRPC_ADDR=ai-service:50051
      - EXECUTOR_GRPC_ADDR=test-worker:50052
      - MINIO_ENDPOINT=minio:9000
      - MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY:-autotest}
      - MINIO_SECRET_KEY=${MINIO_SECRET_KEY:-autotest_dev}
      - MINIO_USE_SSL=false
      - JWT_SECRET=${JWT_SECRET:-dev_jwt_secret_change_in_prod}
    restart: unless-stopped

  ai-service:
    build:
      context: ../ai_service
      dockerfile: Dockerfile
    container_name: autotest-ai-service
    ports:
      - "50051:50051"
    depends_on:
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    environment:
      - APP_ENV=development
      - DB_URL=postgresql://autotest:${DB_PASSWORD:-autotest_dev}@postgres:5432/autotest
      - RABBITMQ_URL=amqp://${MQ_USER:-autotest}:${MQ_PASSWORD:-autotest_dev}@rabbitmq:5672/
      - GRPC_PORT=50051
      - LLM_PROVIDER=${LLM_PROVIDER:-openai}
      - LLM_API_KEY=${LLM_API_KEY}
      - LLM_API_BASE_URL=${LLM_API_BASE_URL:-}
      - LLM_MODEL=${LLM_MODEL:-gpt-4o}
    deploy:
      replicas: 2
    restart: unless-stopped

  test-worker:
    build:
      context: ../test_executor
      dockerfile: Dockerfile
    container_name: autotest-test-worker
    depends_on:
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      minio:
        condition: service_healthy
    environment:
      - APP_ENV=development
      - DB_URL=postgresql://autotest:${DB_PASSWORD:-autotest_dev}@postgres:5432/autotest
      - RABBITMQ_URL=amqp://${MQ_USER:-autotest}:${MQ_PASSWORD:-autotest_dev}@rabbitmq:5672/
      - GRPC_PORT=50052
      - MINIO_ENDPOINT=minio:9000
      - MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY:-autotest}
      - MINIO_SECRET_KEY=${MINIO_SECRET_KEY:-autotest_dev}
      - MINIO_USE_SSL=false
      - JIRA_URL=${JIRA_URL:-}
      - JIRA_USER=${JIRA_USER:-}
      - JIRA_TOKEN=${JIRA_TOKEN:-}
    shm_size: "2gb"
    deploy:
      replicas: 2
    restart: unless-stopped

  frontend:
    build:
      context: ../frontend
      dockerfile: Dockerfile
    container_name: autotest-frontend
    ports:
      - "3000:80"
    depends_on:
      - backend
    restart: unless-stopped

volumes:
  pg_data:
  redis_data:
  rabbitmq_data:
  minio_data:
```

### 3.2 启动命令

```bash
# 全量启动
make dev
# 等价于: cd deploy && docker-compose up --build -d

# 查看日志
make logs SVC=backend          # 查看 Go 后端日志
make logs SVC=ai-service       # 查看 AI 服务日志
make logs SVC=test-worker      # 查看执行 Worker 日志

# 停止
make down
```

### 3.3 各服务 Dockerfile

#### Go Backend

```dockerfile
# backend/Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# ---
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /server .
COPY migration/ ./migration/

EXPOSE 8080
CMD ["./server"]
```

#### Python AI Service

```dockerfile
# ai_service/Dockerfile
FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Playwright 浏览器（AI 服务需要用于页面爬取）
RUN playwright install chromium --with-deps

COPY . .

EXPOSE 50051
CMD ["python", "-m", "app.main"]
```

#### Python Test Worker

```dockerfile
# test_executor/Dockerfile
FROM mcr.microsoft.com/playwright/python:v1.50.0-noble

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

EXPOSE 50052
CMD ["python", "-m", "app.main"]
```

#### Frontend

```dockerfile
# frontend/Dockerfile
FROM node:20-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

# ---
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

---

## 4. 生产环境部署

### 4.1 部署架构

```
                         ┌───────────┐
                         │  Nginx    │
                         │ (反向代理) │
                         └─────┬─────┘
                               │
                 ┌─────────────┼─────────────┐
                 │             │             │
           ┌─────▼─────┐ ┌────▼────┐ ┌──────▼──────┐
           │ Frontend   │ │ Backend │ │ Backend     │
           │ (静态资源) │ │ Node 1  │ │ Node 2      │
           └───────────┘ └────┬────┘ └──────┬──────┘
                              │             │
              ┌───────────────┼─────────────┘
              │               │
        ┌─────▼─────┐  ┌─────▼─────┐
        │ AI Service│  │ AI Service│
        │ Node 1    │  │ Node 2    │
        └───────────┘  └───────────┘
              │               │
        ┌─────▼─────────────▼─────┐
        │       RabbitMQ          │
        │  (建议使用集群模式)      │
        └─────────┬───────────────┘
                  │
     ┌────────────┼────────────┐
     │            │            │
┌────▼────┐ ┌────▼────┐ ┌────▼────┐
│ Worker 1│ │ Worker 2│ │ Worker 3│
└─────────┘ └─────────┘ └─────────┘
```

### 4.2 Nginx 反向代理配置

```nginx
# deploy/nginx/nginx.conf
upstream backend_servers {
    server backend-1:8080;
    server backend-2:8080;
}

server {
    listen 80;
    server_name autotest.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl;
    server_name autotest.example.com;

    ssl_certificate     /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    # 前端静态资源
    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
    }

    # 后端 API
    location /api/ {
        proxy_pass http://backend_servers;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket
    location /ws {
        proxy_pass http://backend_servers;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }

    # Allure 报告（代理到 MinIO）
    location /reports/ {
        proxy_pass http://minio:9000/autotest-reports/;
    }
}
```

### 4.3 生产环境安全加固

| 项目 | 措施 |
|------|------|
| 数据库 | 独立强密码，限制访问 IP，定时备份 |
| RabbitMQ | 修改默认用户密码，启用 TLS |
| MinIO | 使用强密码，配置 Bucket 访问策略 |
| Redis | 设置密码，禁用危险命令 |
| JWT | 使用强随机密钥，设置合理过期时间 |
| LLM API Key | 通过 Secret 管理（K8s Secret / Vault） |
| 网络 | 基础设施服务不暴露外网端口 |
| gRPC | 内网通信，生产环境建议开启 TLS |

---

## 5. 环境变量配置

### 5.1 Go Backend

| 变量名 | 必填 | 默认值 | 说明 |
|--------|------|--------|------|
| `APP_ENV` | 否 | development | 运行环境：development / staging / production |
| `HTTP_PORT` | 否 | 8080 | HTTP 监听端口 |
| `DB_DSN` | 是 | - | PostgreSQL 连接字符串 |
| `REDIS_URL` | 是 | - | Redis 连接地址 |
| `RABBITMQ_URL` | 是 | - | RabbitMQ 连接地址 |
| `AI_GRPC_ADDR` | 是 | - | AI gRPC 服务地址（host:port） |
| `EXECUTOR_GRPC_ADDR` | 是 | - | 执行 Worker gRPC 地址 |
| `MINIO_ENDPOINT` | 是 | - | MinIO 地址 |
| `MINIO_ACCESS_KEY` | 是 | - | MinIO Access Key |
| `MINIO_SECRET_KEY` | 是 | - | MinIO Secret Key |
| `MINIO_USE_SSL` | 否 | false | 是否使用 SSL 连接 MinIO |
| `JWT_SECRET` | 是 | - | JWT 签名密钥 |
| `JWT_EXPIRE_HOURS` | 否 | 24 | JWT 过期时间（小时） |
| `LOG_LEVEL` | 否 | info | 日志级别：debug / info / warn / error |

### 5.2 Python AI Service

| 变量名 | 必填 | 默认值 | 说明 |
|--------|------|--------|------|
| `APP_ENV` | 否 | development | 运行环境 |
| `DB_URL` | 是 | - | PostgreSQL 连接字符串 |
| `RABBITMQ_URL` | 是 | - | RabbitMQ 连接地址 |
| `GRPC_PORT` | 否 | 50051 | gRPC 监听端口 |
| `LLM_PROVIDER` | 否 | openai | LLM 提供商：openai / claude / local |
| `LLM_API_KEY` | 是 | - | LLM API 密钥 |
| `LLM_API_BASE_URL` | 否 | - | 自定义 API 地址（用于代理或本地模型） |
| `LLM_MODEL` | 否 | gpt-4o | 默认模型名称 |
| `LLM_MAX_RETRIES` | 否 | 3 | LLM 调用最大重试次数 |
| `LLM_TIMEOUT` | 否 | 120 | LLM 调用超时（秒） |
| `LOG_LEVEL` | 否 | INFO | 日志级别 |

### 5.3 Python Test Worker

| 变量名 | 必填 | 默认值 | 说明 |
|--------|------|--------|------|
| `APP_ENV` | 否 | development | 运行环境 |
| `DB_URL` | 是 | - | PostgreSQL 连接字符串 |
| `RABBITMQ_URL` | 是 | - | RabbitMQ 连接地址 |
| `GRPC_PORT` | 否 | 50052 | gRPC 监听端口 |
| `MINIO_ENDPOINT` | 是 | - | MinIO 地址 |
| `MINIO_ACCESS_KEY` | 是 | - | MinIO Access Key |
| `MINIO_SECRET_KEY` | 是 | - | MinIO Secret Key |
| `MINIO_USE_SSL` | 否 | false | 是否使用 SSL |
| `MINIO_BUCKET` | 否 | autotest | 默认 Bucket 名称 |
| `JIRA_URL` | 否 | - | JIRA 服务地址 |
| `JIRA_USER` | 否 | - | JIRA 用户名 |
| `JIRA_TOKEN` | 否 | - | JIRA API Token |
| `DINGTALK_WEBHOOK` | 否 | - | 钉钉机器人 Webhook |
| `WECOM_WEBHOOK` | 否 | - | 企业微信机器人 Webhook |
| `SMTP_HOST` | 否 | - | 邮件 SMTP 服务器 |
| `SMTP_PORT` | 否 | 587 | SMTP 端口 |
| `SMTP_USER` | 否 | - | SMTP 用户 |
| `SMTP_PASSWORD` | 否 | - | SMTP 密码 |
| `WORKER_CONCURRENCY` | 否 | 3 | 单个 Worker 最大并发执行数 |
| `LOG_LEVEL` | 否 | INFO | 日志级别 |

### 5.4 .env 文件模板

```bash
# deploy/.env.example (复制为 .env 并填写实际值)

# ===== 数据库 =====
DB_PASSWORD=your_secure_password

# ===== 消息队列 =====
MQ_USER=autotest
MQ_PASSWORD=your_secure_password

# ===== 对象存储 =====
MINIO_ACCESS_KEY=autotest
MINIO_SECRET_KEY=your_secure_password

# ===== JWT =====
JWT_SECRET=your_random_jwt_secret_at_least_32_chars

# ===== AI/LLM =====
LLM_PROVIDER=openai
LLM_API_KEY=sk-xxxxxxxxxxxxxxxx
LLM_MODEL=gpt-4o
# LLM_API_BASE_URL=  # 如使用代理或本地模型，填写此项

# ===== JIRA（可选）=====
# JIRA_URL=https://your-org.atlassian.net
# JIRA_USER=your-email@example.com
# JIRA_TOKEN=your_jira_api_token

# ===== 通知（可选）=====
# DINGTALK_WEBHOOK=https://oapi.dingtalk.com/robot/send?access_token=xxx
# WECOM_WEBHOOK=https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx
```

---

## 6. Makefile 命令参考

```makefile
.PHONY: proto dev infra migrate down logs test clean

# 编译 Protobuf（同时生成 Go 和 Python 代码）
proto:
	@echo ">>> Generating protobuf code..."
	protoc --proto_path=proto \
		--go_out=backend/internal/grpcclient/pb --go_opt=paths=source_relative \
		--go-grpc_out=backend/internal/grpcclient/pb --go-grpc_opt=paths=source_relative \
		proto/*.proto
	python -m grpc_tools.protoc --proto_path=proto \
		--python_out=ai_service/app/grpc_server/generated \
		--grpc_python_out=ai_service/app/grpc_server/generated \
		proto/*.proto
	@echo ">>> Done."

# 启动全部服务（Docker Compose）
dev:
	cd deploy && docker-compose --env-file .env up --build -d

# 仅启动基础设施（数据库、MQ、Redis、MinIO）
infra:
	cd deploy && docker-compose up -d postgres redis rabbitmq minio

# 数据库迁移
migrate:
	cd backend && go run cmd/migrate/main.go up

# 回滚数据库迁移
migrate-down:
	cd backend && go run cmd/migrate/main.go down 1

# 停止所有服务
down:
	cd deploy && docker-compose down

# 停止并清除数据卷
clean:
	cd deploy && docker-compose down -v

# 查看指定服务日志
logs:
	cd deploy && docker-compose logs -f $(SVC)

# 运行 Go 后端测试
test-backend:
	cd backend && go test ./...

# 运行 Python AI 服务测试
test-ai:
	cd ai_service && python -m pytest tests/ -v

# 运行 Python 执行器测试
test-executor:
	cd test_executor && python -m pytest tests/ -v

# 运行全部测试
test: test-backend test-ai test-executor

# 格式化代码
fmt:
	cd backend && go fmt ./...
	cd ai_service && black app/ tests/
	cd test_executor && black app/ tests/
```

---

## 7. 监控与日志

### 7.1 Prometheus 监控

各服务暴露 `/metrics` 端点：

| 服务 | Metrics 端点 |
|------|-------------|
| Go Backend | `http://backend:8080/metrics` |
| AI Service | `http://ai-service:9090/metrics` |
| Test Worker | `http://test-worker:9090/metrics` |

关键监控指标：

| 指标 | 说明 |
|------|------|
| `http_requests_total` | HTTP 请求计数 |
| `http_request_duration_seconds` | 请求延迟分布 |
| `grpc_server_handled_total` | gRPC 调用计数 |
| `mq_messages_consumed_total` | MQ 消费消息数 |
| `mq_messages_published_total` | MQ 发布消息数 |
| `task_duration_seconds` | 异步任务执行时长 |
| `task_status_total` | 任务状态计数 |
| `llm_requests_total` | LLM 调用次数 |
| `llm_token_usage_total` | LLM Token 用量 |
| `playwright_test_duration_seconds` | 测试执行时长 |
| `test_pass_rate` | 测试通过率 |

### 7.2 日志规范

所有服务统一使用 JSON 格式日志，便于收集和检索：

```json
{
    "timestamp": "2026-03-07T10:30:00.123Z",
    "level": "info",
    "service": "backend",
    "trace_id": "abc-123-def-456",
    "message": "Task dispatched to MQ",
    "fields": {
        "task_id": "...",
        "type": "generate_cases",
        "queue": "ai.generate.cases"
    }
}
```

### 7.3 Grafana Dashboard

建议创建以下 Dashboard：

1. **服务健康总览**：各服务的 UP/DOWN 状态、CPU/内存使用率
2. **API 监控**：请求量、延迟 P50/P95/P99、错误率
3. **MQ 监控**：队列深度、消费速率、死信队列数量
4. **AI 调用监控**：调用量、成功率、Token 用量、延迟
5. **测试执行监控**：执行数量、通过率趋势、平均执行时长
6. **业务大盘**：项目数、用例数、本周执行情况、Bug 发现数

---

## 8. 常见问题排查

### 8.1 Playwright 执行失败："Browser closed unexpectedly"

**原因**：通常是共享内存（shm）不足。

**解决**：确保 Docker 容器配置了 `shm_size: "2gb"` 或更大。

### 8.2 AI 服务 LLM 调用超时

**原因**：网络问题或 LLM 服务负载过高。

**解决**：
- 检查 `LLM_API_BASE_URL` 配置是否正确
- 增大 `LLM_TIMEOUT` 值
- 如使用代理，确认代理可达
- 查看 AI 服务日志中的错误详情

### 8.3 MQ 消息堆积

**原因**：消费者处理能力不足或消费者宕机。

**排查**：
1. 访问 RabbitMQ 管理界面查看队列状态
2. 检查 Consumer 是否正常运行
3. 增加 Worker 副本数：`docker-compose up -d --scale test-worker=5`

### 8.4 数据库连接数耗尽

**原因**：多个服务共享数据库，连接池配置过大。

**建议**：
- Go Backend：连接池 max 20
- AI Service（每个副本）：连接池 max 5
- Test Worker（每个副本）：连接池 max 5
- PostgreSQL `max_connections`：至少 100

### 8.5 Go 与 Python 的 Protobuf 生成代码不一致

**原因**：未统一执行 `make proto`，或 protoc 版本不一致。

**解决**：
- 每次修改 `.proto` 文件后，运行 `make proto` 统一重新生成
- 在 CI 中校验生成代码是否最新
- 固定 protoc 版本（建议在 Docker 中统一编译）
