# 可视化脚本编辑器 — 设计文档

> 版本：v1.0  
> 日期：2026-03-07  
> 关联文档：[架构方案](./architecture.md) | [数据库设计](./database-design.md) | [接口契约](./api-contracts.md)

---

## 目录

- [1. 概述](#1-概述)
- [2. 核心数据模型：JSON 步骤模型](#2-核心数据模型json-步骤模型)
- [3. 步骤类型完整定义](#3-步骤类型完整定义)
- [4. 定位器体系](#4-定位器体系)
- [5. 断言体系](#5-断言体系)
- [6. 流程控制](#6-流程控制)
- [7. 可复用组件（公共步骤组）](#7-可复用组件公共步骤组)
- [8. 数据驱动测试](#8-数据驱动测试)
- [9. 录制回放](#9-录制回放)
- [10. 元素拾取器](#10-元素拾取器)
- [11. 单步调试模式](#11-单步调试模式)
- [12. 编辑器交互设计](#12-编辑器交互设计)
- [13. JSON → Playwright 代码转换](#13-json--playwright-代码转换)
- [14. 与现有架构的集成](#14-与现有架构的集成)

---

## 1. 概述

可视化脚本编辑器是平台的核心交互模块，目标是让**不会写代码的测试人员**也能创建、编辑和维护自动化测试脚本。

### 1.1 设计理念

```
传统方式：测试人员 → 写 Python 代码 → 调试运行
本平台方式：测试人员 → 可视化拖拽/选择 → 自动生成代码 → 执行

AI 生成的脚本 ──┐
                 ├──► JSON 步骤模型（统一中间表示）──► 可视化编辑器展示/编辑
手动创建的脚本 ──┘                                         │
                                                           ▼
                                                    执行时自动转为
                                                  Playwright Python 代码
```

### 1.2 双模式支持

编辑器同时支持两种模式，用户可随时切换：

| 模式 | 适用用户 | 编辑方式 | 存储字段 |
|------|---------|---------|---------|
| **可视化模式** | 测试人员、业务人员 | 图形化拖拽/选择 | `steps_json` (JSONB) |
| **代码模式** | 测试开发、高级用户 | 直接编辑 Python 代码 | `source_code` (TEXT) |

> 可视化模式 → 代码模式：自动转换（JSON → Python）  
> 代码模式 → 可视化模式：**不支持反向转换**（代码表达力超过 JSON 模型）  
> 一旦切换到代码模式并手动修改代码，该脚本标记为 `script_type = 'code'`

---

## 2. 核心数据模型：JSON 步骤模型

### 2.1 顶层结构

```json
{
    "version": "1.0",
    "metadata": {
        "name": "test_user_login",
        "description": "验证用户登录功能",
        "tags": ["登录", "冒烟"],
        "author": "zhangsan",
        "created_at": "2026-03-07T10:00:00Z"
    },
    "config": {
        "base_url": "https://staging.example.com",
        "browser": "chromium",
        "viewport": { "width": 1280, "height": 720 },
        "timeout_ms": 30000,
        "retry_on_failure": true,
        "screenshot_on_failure": true,
        "trace_on_failure": true
    },
    "variables": [
        { "name": "username", "value": "admin", "type": "string", "source": "manual" },
        { "name": "password", "value": "123456", "type": "string", "source": "manual" }
    ],
    "dataset_id": null,
    "setup_steps": [],
    "steps": [ ... ],
    "teardown_steps": []
}
```

### 2.2 步骤通用结构

每个步骤共享以下基础字段：

```json
{
    "id": "step-uuid",
    "type": "click",
    "description": "点击登录按钮",
    "enabled": true,
    "is_breakpoint": false,
    "screenshot_after": false,
    "custom_timeout_ms": null,
    "note": "",
    "on_error": "fail",
    "target": { ... },
    "params": { ... }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 步骤唯一标识（UUID） |
| `type` | string | 步骤类型（见第 3 章完整列表） |
| `description` | string | 步骤描述（显示在编辑器中） |
| `enabled` | boolean | 是否启用（false = 禁用/跳过，相当于注释） |
| `is_breakpoint` | boolean | 是否为断点（调试时在此暂停） |
| `screenshot_after` | boolean | 执行后是否截图 |
| `custom_timeout_ms` | int/null | 自定义超时（覆盖全局设置） |
| `note` | string | 步骤备注（不影响执行，方便团队沟通） |
| `on_error` | string | 失败处理策略：`fail`（终止）/ `continue`（跳过继续）/ `retry`（重试） |
| `target` | object | 目标元素定位器（仅交互/断言类步骤需要） |
| `params` | object | 步骤参数（因 type 不同而不同） |

### 2.3 Target（元素定位器）结构

```json
{
    "locator_type": "test_id",
    "locator_value": "login-button",
    "options": {
        "name": "登录",
        "exact": true
    },
    "friendly_name": "登录按钮",
    "element_id": 1001,
    "fallback_locators": [
        { "locator_type": "role", "locator_value": "button", "options": { "name": "登录" } },
        { "locator_type": "css", "locator_value": "#login-btn" }
    ]
}
```

| 字段 | 说明 |
|------|------|
| `locator_type` | 定位方式（见第 4 章） |
| `locator_value` | 定位值 |
| `options` | 附加选项（如 role 的 name、text 的 exact） |
| `friendly_name` | 友好名称，用于编辑器展示和报告 |
| `element_id` | 关联元素仓库的 ID（可选，用于集中管理） |
| `fallback_locators` | 备用定位器列表（Self-Healing：主定位器失败时依次尝试） |

---

## 3. 步骤类型完整定义

### 3.1 导航类

| type | 描述 | params |
|------|------|--------|
| `navigate` | 打开 URL | `{ "url": "/login" }` |
| `go_back` | 浏览器后退 | `{}` |
| `go_forward` | 浏览器前进 | `{}` |
| `reload` | 刷新页面 | `{}` |

### 3.2 交互类

| type | 描述 | 需要 target | params |
|------|------|-----------|--------|
| `click` | 点击 | 是 | `{}` |
| `double_click` | 双击 | 是 | `{}` |
| `right_click` | 右键点击 | 是 | `{}` |
| `input` | 输入文本 | 是 | `{ "value": "admin", "clear_first": true }` |
| `clear` | 清空输入框 | 是 | `{}` |
| `select` | 下拉框选择 | 是 | `{ "by": "label/value/index", "value": "选项1" }` |
| `check` | 勾选复选框 | 是 | `{}` |
| `uncheck` | 取消勾选 | 是 | `{}` |
| `hover` | 鼠标悬停 | 是 | `{}` |
| `drag_and_drop` | 拖拽 | - | `{ "source": {target}, "destination": {target} }` |
| `upload_file` | 上传文件 | 是 | `{ "file_path": "test_data/avatar.png" }` |
| `press_key` | 按键 | 否 | `{ "key": "Enter" }` |
| `scroll` | 滚动 | 否 | `{ "direction": "down", "distance": 500 }` |

### 3.3 等待类

| type | 描述 | params |
|------|------|--------|
| `wait` | 等待条件 | `{ "wait_type": "...", "value": "...", "timeout_ms": 5000 }` |

**wait_type 可选值**：

| wait_type | 说明 | value 含义 |
|-----------|------|-----------|
| `element_visible` | 等待元素可见 | target 指定元素 |
| `element_hidden` | 等待元素消失 | target 指定元素 |
| `url` | 等待 URL 匹配 | URL 通配符 `**/dashboard` |
| `url_contains` | URL 包含子串 | 子串 |
| `title` | 等待页面标题 | 标题文本 |
| `time` | 固定等待（不推荐） | 毫秒数 |
| `network_idle` | 等待网络空闲 | - |
| `load_state` | 等待加载状态 | `load` / `domcontentloaded` / `networkidle` |

### 3.4 断言类

| type | 描述 | params |
|------|------|--------|
| `assertion` | 断言验证 | `{ "assert_type": "...", ... }` |

断言类型详见第 5 章。

### 3.5 流程控制类

| type | 描述 | 特殊字段 |
|------|------|---------|
| `condition` | 条件判断 (IF/ELSE) | `condition`, `then_steps`, `else_steps` |
| `loop` | 循环 | `loop_type`, `body_steps`, `max_iterations` |
| `break` | 跳出循环 | `{}` |
| `group` | 步骤分组 | `steps`（纯组织用，不影响逻辑） |

流程控制详见第 6 章。

### 3.6 数据操作类

| type | 描述 | params |
|------|------|--------|
| `set_variable` | 设置变量 | `{ "name": "token", "value": "abc123" }` |
| `extract_text` | 提取元素文本到变量 | `{ "variable_name": "msg" }` + target |
| `extract_attribute` | 提取元素属性到变量 | `{ "attribute": "href", "variable_name": "link" }` + target |
| `extract_count` | 提取元素数量到变量 | `{ "variable_name": "count" }` + target |

### 3.7 组件调用类

| type | 描述 | params |
|------|------|--------|
| `call_component` | 调用可复用组件 | `{ "component_id": 10, "arguments": {"username": "admin"} }` |

详见第 7 章。

### 3.8 多窗口/iframe 类

| type | 描述 | params |
|------|------|--------|
| `switch_tab` | 切换标签页 | `{ "action": "wait_new/switch_by_index/switch_by_url", "value": "..." }` |
| `close_tab` | 关闭当前标签页 | `{}` |
| `switch_to_iframe` | 进入 iframe | target 指向 iframe 元素 |
| `switch_to_main` | 返回主页面 | `{}` |

### 3.9 API 调用类

| type | 描述 | params |
|------|------|--------|
| `api_call` | HTTP 请求 | `{ "method": "POST", "url": "...", "headers": {}, "body": {}, "extract": [], "assertions": [] }` |

```json
{
    "type": "api_call",
    "description": "创建测试订单",
    "params": {
        "method": "POST",
        "url": "{{base_url}}/api/orders",
        "headers": {
            "Authorization": "Bearer {{token}}",
            "Content-Type": "application/json"
        },
        "body": {
            "product_id": 1,
            "quantity": 2
        },
        "extract": [
            { "variable_name": "order_id", "json_path": "$.id" },
            { "variable_name": "total", "json_path": "$.total_price" }
        ],
        "assertions": [
            { "source": "status_code", "operator": "equals", "expected": 201 },
            { "source": "json_path:$.status", "operator": "equals", "expected": "created" }
        ]
    }
}
```

### 3.10 其他类

| type | 描述 | params |
|------|------|--------|
| `screenshot` | 手动截图 | `{ "full_page": true, "name": "login_page" }` |
| `verify_download` | 验证文件下载 | `{ "trigger": {target}, "filename_pattern": "report_*.xlsx", "timeout_ms": 10000 }` |
| `execute_js` | 执行 JavaScript | `{ "script": "return document.title;" , "variable_name": "page_title" }` |
| `custom_code` | 自定义 Python 代码 | `{ "code": "await page.evaluate(...)" }` |

---

## 4. 定位器体系

### 4.1 支持的定位方式

| locator_type | 编辑器显示名 | Playwright 映射 | 可靠性评级 | 推荐场景 |
|-------------|------------|-----------------|----------|---------|
| `test_id` | Data-TestID | `page.get_by_test_id(value)` | ⭐⭐⭐⭐⭐ | 最推荐，需前端配合 |
| `role` | 角色 (Role) | `page.get_by_role(value, name=...)` | ⭐⭐⭐⭐ | 语义化定位 |
| `text` | 文本内容 | `page.get_by_text(value)` | ⭐⭐⭐ | 按钮、链接等 |
| `label` | 标签 (Label) | `page.get_by_label(value)` | ⭐⭐⭐⭐ | 表单元素 |
| `placeholder` | 占位符 | `page.get_by_placeholder(value)` | ⭐⭐⭐ | 输入框 |
| `alt_text` | Alt 文本 | `page.get_by_alt_text(value)` | ⭐⭐⭐ | 图片 |
| `title` | Title 属性 | `page.get_by_title(value)` | ⭐⭐⭐ | tooltip 元素 |
| `css` | CSS 选择器 | `page.locator("css=value")` | ⭐⭐ | 通用，但易受布局变化影响 |
| `xpath` | XPath | `page.locator("xpath=value")` | ⭐ | 最后手段 |
| `id` | 元素 ID | `page.locator("#value")` | ⭐⭐⭐ | 有唯一 ID 时 |
| `class` | CSS Class | `page.locator(".value")` | ⭐⭐ | 类名稳定时 |

### 4.2 定位器优先级策略

AI 生成和元素拾取器自动选择定位器时遵循此优先级：

```
test_id > role + name > label > placeholder > text > id > css > xpath
```

### 4.3 Self-Healing 机制

当主定位器失败时，自动尝试 `fallback_locators` 列表中的备用定位器：

```
执行步骤 "点击登录按钮"
  │
  ├─ 尝试主定位器: test_id="login-btn"
  │  └─ 失败（元素未找到）
  │
  ├─ 尝试备用定位器 1: role=button, name="登录"
  │  └─ 成功 ✅
  │
  ├─ 记录修复日志，通知测试人员确认
  │
  └─ 可选：自动更新主定位器
```

---

## 5. 断言体系

### 5.1 元素断言

| assert_type | 说明 | 参数 | Playwright 映射 |
|-------------|------|------|----------------|
| `visible` | 元素可见 | target | `expect(loc).to_be_visible()` |
| `hidden` | 元素不可见 | target | `expect(loc).to_be_hidden()` |
| `enabled` | 元素可用 | target | `expect(loc).to_be_enabled()` |
| `disabled` | 元素禁用 | target | `expect(loc).to_be_disabled()` |
| `checked` | 已勾选 | target | `expect(loc).to_be_checked()` |
| `unchecked` | 未勾选 | target | `expect(loc).not_to_be_checked()` |
| `focused` | 获得焦点 | target | `expect(loc).to_be_focused()` |

### 5.2 文本断言

| assert_type | 说明 | 参数 |
|-------------|------|------|
| `text_equals` | 文本完全等于 | target, `expected` |
| `text_contains` | 文本包含 | target, `expected` |
| `text_matches` | 文本匹配正则 | target, `pattern` |
| `text_not_empty` | 文本不为空 | target |
| `input_value` | 输入框值等于 | target, `expected` |

### 5.3 属性断言

| assert_type | 说明 | 参数 |
|-------------|------|------|
| `attribute_equals` | 属性值等于 | target, `attribute`, `expected` |
| `attribute_contains` | 属性值包含 | target, `attribute`, `expected` |
| `has_class` | 包含 CSS class | target, `class_name` |

### 5.4 页面断言

| assert_type | 说明 | 参数 |
|-------------|------|------|
| `url_equals` | URL 完全等于 | `expected` |
| `url_contains` | URL 包含 | `expected` |
| `url_matches` | URL 匹配正则 | `pattern` |
| `title_equals` | 页面标题等于 | `expected` |
| `title_contains` | 页面标题包含 | `expected` |

### 5.5 数量断言

| assert_type | 说明 | 参数 |
|-------------|------|------|
| `element_count` | 元素数量 | target, `operator` (`=`,`>`,`<`,`>=`,`<=`,`!=`), `expected` |

### 5.6 断言步骤完整示例

```json
{
    "id": "step-6",
    "type": "assertion",
    "description": "断言：欢迎文字可见且内容正确",
    "params": {
        "assert_type": "text_contains",
        "target": {
            "locator_type": "css",
            "locator_value": ".welcome-message",
            "friendly_name": "欢迎文字"
        },
        "expected": "欢迎回来，{{username}}"
    }
}
```

---

## 6. 流程控制

### 6.1 条件判断 (IF / ELSE)

```json
{
    "id": "step-7",
    "type": "condition",
    "description": "如果有弹窗则关闭",
    "condition": {
        "check_type": "element_visible",
        "target": {
            "locator_type": "css",
            "locator_value": ".modal-overlay",
            "friendly_name": "弹窗遮罩"
        },
        "timeout_ms": 2000
    },
    "then_steps": [
        {
            "id": "step-7-1",
            "type": "click",
            "description": "点击关闭弹窗",
            "target": {
                "locator_type": "css",
                "locator_value": ".modal-close",
                "friendly_name": "弹窗关闭按钮"
            }
        }
    ],
    "else_steps": []
}
```

**condition.check_type 可选值**：

| check_type | 说明 | 需要 target | 额外参数 |
|-----------|------|-----------|---------|
| `element_visible` | 元素是否可见 | 是 | `timeout_ms` |
| `element_hidden` | 元素是否不可见 | 是 | `timeout_ms` |
| `element_exists` | 元素是否存在于 DOM | 是 | - |
| `text_equals` | 元素文本是否等于 | 是 | `expected` |
| `text_contains` | 元素文本是否包含 | 是 | `expected` |
| `url_contains` | URL 是否包含 | 否 | `expected` |
| `variable_equals` | 变量值是否等于 | 否 | `variable`, `expected` |
| `variable_greater` | 变量值是否大于 | 否 | `variable`, `expected` |
| `element_count_gt` | 元素数量是否大于 | 是 | `expected` (数字) |

### 6.2 循环

#### FOR EACH — 遍历元素集合

```json
{
    "id": "step-8",
    "type": "loop",
    "description": "遍历所有商品卡片",
    "loop_type": "for_each",
    "source": {
        "locator_type": "css",
        "locator_value": ".product-card",
        "friendly_name": "商品卡片"
    },
    "variable_name": "card",
    "max_iterations": 100,
    "body_steps": [
        {
            "id": "step-8-1",
            "type": "assertion",
            "description": "商品名称不为空",
            "params": {
                "assert_type": "text_not_empty",
                "target": {
                    "locator_type": "css",
                    "locator_value": "{{card}} .product-name"
                }
            }
        }
    ]
}
```

#### FOR COUNT — 重复 N 次

```json
{
    "id": "step-9",
    "type": "loop",
    "description": "添加 5 个商品",
    "loop_type": "for_count",
    "count": 5,
    "variable_name": "i",
    "max_iterations": 5,
    "body_steps": [ ... ]
}
```

#### WHILE — 条件循环

```json
{
    "id": "step-10",
    "type": "loop",
    "description": "翻页直到最后一页",
    "loop_type": "while",
    "condition": {
        "check_type": "element_visible",
        "target": {
            "locator_type": "css",
            "locator_value": ".next-page:not([disabled])",
            "friendly_name": "下一页按钮（可点击状态）"
        }
    },
    "max_iterations": 50,
    "body_steps": [
        {
            "id": "step-10-1",
            "type": "click",
            "description": "点击下一页",
            "target": {
                "locator_type": "css",
                "locator_value": ".next-page"
            }
        },
        {
            "id": "step-10-2",
            "type": "wait",
            "description": "等待加载完成",
            "params": { "wait_type": "network_idle", "timeout_ms": 3000 }
        }
    ]
}
```

### 6.3 安全限制

| 限制项 | 默认值 | 说明 |
|--------|--------|------|
| FOR EACH 最大元素数 | 100 | 超过则截断并警告 |
| FOR COUNT 最大次数 | 1000 | 不允许设置更大值 |
| WHILE 最大迭代次数 | **强制要求填写**，默认 50 | 防止无限循环 |
| 循环体单次超时 | 30s | 单次循环体执行超时 |
| 嵌套深度 | 最多 3 层 | 条件/循环的嵌套层级 |

---

## 7. 可复用组件（公共步骤组）

### 7.1 组件定义

```json
{
    "id": 10,
    "name": "用户登录",
    "description": "通用登录流程",
    "parameters": [
        { "name": "username", "type": "string", "required": true, "default": "" },
        { "name": "password", "type": "string", "required": true, "default": "" },
        { "name": "expect_success", "type": "boolean", "required": false, "default": true }
    ],
    "steps": [
        {
            "type": "navigate",
            "description": "打开登录页",
            "params": { "url": "/login" }
        },
        {
            "type": "input",
            "description": "输入用户名",
            "target": { "locator_type": "test_id", "locator_value": "username" },
            "params": { "value": "{{username}}" }
        },
        {
            "type": "input",
            "description": "输入密码",
            "target": { "locator_type": "test_id", "locator_value": "password" },
            "params": { "value": "{{password}}" }
        },
        {
            "type": "click",
            "description": "点击登录",
            "target": { "locator_type": "role", "locator_value": "button", "options": { "name": "登录" } }
        },
        {
            "type": "condition",
            "description": "根据预期结果断言",
            "condition": { "check_type": "variable_equals", "variable": "expect_success", "expected": true },
            "then_steps": [
                { "type": "wait", "params": { "wait_type": "url_contains", "value": "/dashboard" } }
            ],
            "else_steps": [
                { "type": "assertion", "params": { "assert_type": "visible", "target": { "locator_type": "css", "locator_value": ".error-msg" } } }
            ]
        }
    ]
}
```

### 7.2 组件调用

```json
{
    "id": "step-1",
    "type": "call_component",
    "description": "调用登录组件",
    "params": {
        "component_id": 10,
        "arguments": {
            "username": "{{username}}",
            "password": "{{password}}",
            "expect_success": true
        }
    }
}
```

### 7.3 组件管理要点

- 组件隶属于项目（`project_id`），同项目内所有脚本可引用
- 组件有版本管理，修改组件后所有引用自动使用最新版本
- 编辑器中调用组件显示为可展开的卡片，展开可查看内部步骤
- 支持查看组件的引用关系（"哪些脚本使用了这个组件"）

---

## 8. 数据驱动测试

### 8.1 数据集结构

```json
{
    "id": 1,
    "name": "登录测试数据集",
    "description": "覆盖正常、异常、边界场景",
    "columns": ["username", "password", "expected_result", "priority"],
    "rows": [
        { "username": "admin", "password": "admin123", "expected_result": "success", "priority": "P0" },
        { "username": "admin", "password": "wrong", "expected_result": "password_error", "priority": "P0" },
        { "username": "", "password": "admin123", "expected_result": "username_required", "priority": "P1" },
        { "username": "admin", "password": "", "expected_result": "password_required", "priority": "P1" },
        { "username": "disabled_user", "password": "test123", "expected_result": "account_disabled", "priority": "P2" }
    ]
}
```

### 8.2 脚本关联数据集

脚本的 `dataset_id` 字段关联数据集后，执行时会为每一行数据生成一次执行，步骤中通过 `{{column_name}}` 引用数据列。

### 8.3 执行结果展示

```
执行: test_user_login × login_test_data (5 行)
  │
  ├─ 第 1 行 (admin / admin123)        ✅ Passed  1.2s
  ├─ 第 2 行 (admin / wrong)           ✅ Passed  0.8s
  ├─ 第 3 行 ("" / admin123)           ✅ Passed  0.6s
  ├─ 第 4 行 (admin / "")              ❌ Failed  0.7s
  └─ 第 5 行 (disabled_user / test123) ✅ Passed  0.9s

  通过率: 4/5 = 80%
```

---

## 9. 录制回放

### 9.1 录制流程

```
用户点击 [● 开始录制]
    │
    ▼
平台打开一个浏览器实例（通过 Playwright CDP 连接）
    │
    ▼
用户在浏览器中正常操作
    │ 后台捕获每一步操作（点击、输入、导航等）
    │ 实时转为 JSON 步骤，添加到步骤列表
    ▼
用户点击 [⏹ 停止录制]
    │
    ▼
生成的步骤列表展示在编辑器中
    │
    ▼
用户手动补充断言、调整定位器、添加条件/循环
```

### 9.2 录制能力

| 能力 | 说明 |
|------|------|
| 捕获点击 | 自动生成 click 步骤 + 最佳定位器 |
| 捕获输入 | 自动生成 input 步骤 |
| 捕获导航 | 自动生成 navigate 步骤 |
| 捕获选择 | 下拉框选择 → select 步骤 |
| 捕获文件上传 | 文件选择 → upload_file 步骤 |
| 智能等待 | 页面跳转/加载自动插入 wait 步骤 |
| 定位器选择 | 自动选择最稳定的定位器策略 |

### 9.3 录制器不能做的事（需人工补充）

- **断言**：录制器不知道你要验证什么，必须手动添加
- **条件/循环**：录制器只能记录线性操作
- **变量参数化**：录制的是固定值，需要手动替换为变量
- **公共组件提取**：需要人工识别可复用的步骤组

### 9.4 技术实现

基于 Playwright 的 CDP（Chrome DevTools Protocol）协议：

```python
# 录制服务核心逻辑
async def start_recording(browser_context):
    page = browser_context.pages[0]

    page.on("framenavigated", lambda frame: emit_step({
        "type": "navigate",
        "params": {"url": frame.url}
    }))

    await page.expose_function("__recorder_click", lambda selector:
        emit_step({"type": "click", "target": parse_selector(selector)})
    )

    await page.add_init_script("""
        document.addEventListener('click', (e) => {
            const selector = getBestSelector(e.target);
            window.__recorder_click(selector);
        }, true);
    """)
```

---

## 10. 元素拾取器

### 10.1 工作流

```
用户编辑某步骤的 target
    │
    ▼
点击 [🎯 从页面选取]
    │
    ▼
平台加载目标页面到预览窗口（iframe / 新窗口）
    │ 注入高亮脚本
    ▼
用户移动鼠标 → 悬停的元素高亮显示 + 显示元素信息浮层
    │
    ▼
用户点击元素
    │
    ▼
生成多种定位器候选项（按可靠性排序）
    │
    ▼
用户选择一种定位器 → 自动填回编辑表单
```

### 10.2 元素信息浮层

鼠标悬停时显示的浮层内容：

```
┌─────────────────────────────────┐
│ <button>                        │
│ id: login-btn                   │
│ class: btn btn-primary          │
│ data-testid: login-button       │
│ text: 登录                      │
│ 120×40 px                       │
└─────────────────────────────────┘
```

### 10.3 定位器验证

选择定位器后可立即验证：

- 匹配到几个元素（应该是 1 个）
- 元素是否可见
- 元素的位置和尺寸
- 高亮显示匹配的元素（截图预览）

---

## 11. 单步调试模式

### 11.1 调试控制

| 操作 | 快捷键 | 说明 |
|------|--------|------|
| 执行全部 | F5 | 从当前位置执行到结束 |
| 执行下一步 | F10 | 执行一步后暂停 |
| 执行到断点 | F8 | 执行到下一个断点暂停 |
| 停止 | Shift+F5 | 终止调试，关闭浏览器 |
| 重新开始 | Ctrl+Shift+F5 | 从头开始 |

### 11.2 调试面板布局

```
┌─ 步骤列表(左侧) ─────┬─ 浏览器预览(右上) ──────────────┐
│                        │                                  │
│  ✅ 1. 打开登录页      │  ┌─────────────────────────┐    │
│  ✅ 2. 输入用户名      │  │                         │    │
│  ▶️ 3. 输入密码  ◄当前 │  │   (实时浏览器截图)      │    │
│  ⬜ 4. 点击登录        │  │                         │    │
│  ⬜ 5. 断言跳转        │  └─────────────────────────┘    │
│  🔴 6. 断言文字  ◄断点 │                                  │
│                        ├─ 调试信息(右下) ────────────────┤
│                        │                                  │
│                        │  📋 执行日志:                    │
│                        │  [10:30:01] navigate /login  ✅  │
│                        │  [10:30:02] input #user ✅       │
│                        │  [10:30:02] 等待下一步...        │
│                        │                                  │
│                        │  📊 变量:                        │
│                        │  username = "admin"              │
│                        │  password = "123456"             │
│                        │                                  │
│  [⏭ 下一步] [▶ 全部]  │  📸 当前页面 URL:                │
│  [⏹ 停止] [🔄 重启]   │  https://example.com/login       │
└────────────────────────┴──────────────────────────────────┘
```

### 11.3 调试时的 WebSocket 事件

调试过程通过 WebSocket 实时推送状态到前端：

| 事件 | 数据 |
|------|------|
| `debug:step_start` | `{ step_id, step_description }` |
| `debug:step_done` | `{ step_id, status, duration_ms, screenshot_url }` |
| `debug:paused` | `{ step_id, reason: "breakpoint" / "step_mode" / "error" }` |
| `debug:variables` | `{ variables: { name: value, ... } }` |
| `debug:console_log` | `{ level, message }` |
| `debug:screenshot` | `{ screenshot_url, page_url }` |

---

## 12. 编辑器交互设计

### 12.1 步骤卡片操作菜单

每个步骤卡片右上角提供：

| 操作 | 说明 |
|------|------|
| 📋 复制 | 复制步骤（含子步骤） |
| 📄 粘贴 | 在当前步骤后粘贴 |
| 🔇 禁用/启用 | 临时跳过此步骤 |
| 📝 添加备注 | 备注不影响执行 |
| 🔖 设为断点 | 调试时在此暂停 |
| 📸 执行后截图 | 标记需要截图 |
| ⏱️ 自定义超时 | 覆盖全局超时 |
| ⚠️ 失败策略 | fail / continue / retry |
| ❌ 删除 | 删除步骤 |

### 12.2 拖拽排序

- 上下拖拽调整步骤顺序
- 拖入条件/循环容器的 THEN/ELSE/循环体区域
- 从容器内拖出变为顶层步骤
- 拖拽时显示放置指示线

### 12.3 快捷键

| 快捷键 | 操作 |
|--------|------|
| Ctrl+Z | 撤销 |
| Ctrl+Y | 重做 |
| Ctrl+S | 保存 |
| Ctrl+D | 复制当前步骤 |
| Delete | 删除当前步骤 |
| Ctrl+/ | 禁用/启用步骤 |
| Ctrl+F | 搜索步骤 |
| F5 | 运行调试 |
| F10 | 单步执行 |

### 12.4 搜索和过滤

长脚本中快速定位步骤：

- 关键词搜索（搜索步骤描述、定位器值）
- 按类型过滤（仅显示断言 / 仅显示交互 / 仅显示流程控制）
- 按状态过滤（仅失败 / 仅禁用 / 含备注 / 含断点）
- 全部展开 / 全部折叠 / 仅展开条件和循环

### 12.5 撤销/重做

保留最近 50 步操作的历史记录，支持撤销以下操作：
- 添加/删除步骤
- 修改步骤属性
- 拖拽移动步骤
- 启用/禁用步骤

---

## 13. JSON → Playwright 代码转换

### 13.1 转换规则表

| JSON 步骤 | Playwright Python 代码 |
|-----------|----------------------|
| `navigate` url="/login" | `await page.goto("/login")` |
| `click` css="#btn" | `await page.locator("#btn").click()` |
| `click` role=button name="登录" | `await page.get_by_role("button", name="登录").click()` |
| `input` test_id="user" value="admin" | `await page.get_by_test_id("user").fill("admin")` |
| `select` by=label value="选项1" | `await page.locator("...").select_option(label="选项1")` |
| `wait` url_contains="/dashboard" | `await page.wait_for_url("**/dashboard")` |
| `assertion` visible target | `await expect(page.locator("...")).to_be_visible()` |
| `assertion` text_contains target expected | `await expect(page.locator("...")).to_contain_text(expected)` |
| `condition` | `if await page.locator("...").is_visible(): ...` |
| `loop` for_each | `for item in await page.locator("...").all(): ...` |
| `loop` for_count 5 | `for i in range(5): ...` |
| `extract_text` | `var = await page.locator("...").text_content()` |
| `call_component` | 展开组件步骤并转换 |
| `api_call` | `response = await page.request.post(url, data=body)` |
| `execute_js` | `result = await page.evaluate("...")` |
| `switch_to_iframe` | `frame = page.frame_locator("...")` |

### 13.2 转换器位置

转换器运行在 **Python test-worker** 中，执行任务时实时将 `steps_json` 转为 Python 代码并执行：

```
test-worker 收到执行任务
    │
    ├─ 如果 script_type == 'code'
    │      └─ 直接使用 source_code 执行
    │
    └─ 如果 script_type == 'visual'
           ├─ 读取 steps_json
           ├─ 调用 StepConverter 转为 Python 代码
           ├─ 写入临时 .py 文件
           └─ pytest 执行
```

---

## 14. 与现有架构的集成

### 14.1 数据库变更

`test_scripts` 表新增字段：

```sql
ALTER TABLE test_scripts ADD COLUMN script_type VARCHAR(20) DEFAULT 'visual';
ALTER TABLE test_scripts ADD COLUMN steps_json JSONB;
```

新增表：`shared_components`、`element_repository`、`test_datasets`（详见数据库设计文档）。

### 14.2 AI 生成输出格式变更

AI 生成脚本时输出 JSON 步骤模型（而非 Python 代码），这样：
- 生成结果直接可在可视化编辑器中展示和编辑
- JSON 结构化输出比自由格式代码更容易校验
- 测试人员无需看代码，直接在可视化界面审核

### 14.3 新增 MQ 队列

| 队列 | 用途 |
|------|------|
| `executor.debug` | 调试执行任务（单步模式） |

### 14.4 新增 WebSocket 事件

调试相关事件：`debug:step_start`、`debug:step_done`、`debug:paused`、`debug:variables`、`debug:screenshot`（详见第 11 章）。

### 14.5 新增 REST API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/scripts/:id/debug` | 启动调试会话 |
| POST | `/scripts/:id/debug/next` | 执行下一步 |
| POST | `/scripts/:id/debug/continue` | 执行到断点 |
| POST | `/scripts/:id/debug/stop` | 停止调试 |
| POST | `/scripts/:id/record/start` | 开始录制 |
| POST | `/scripts/:id/record/stop` | 停止录制 |
| POST | `/elements/pick` | 启动元素拾取器 |
| POST | `/elements/verify` | 验证定位器 |
