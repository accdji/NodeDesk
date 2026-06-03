# 轻量级工作流编排器

Go 标准库实现的插件式 DAG 工作流引擎，单二进制零依赖。支持条件判断、循环、开始/结束节点，中文/英文双语界面。

## 快速开始

```bash
go build -o workflow.exe .
./workflow.exe
# 打开浏览器 http://localhost:8080
```

> **提示**：服务启动后会自动加载 `config/pipeline.example.json` 作为示例配置。可通过环境变量 `CONFIG` 指定配置文件路径，或通过浏览器访问 `/api/config` 在线编辑。

---

## 目录结构

```
├── main.go              # 入口
├── engine/              # DAG 引擎 + 条件/循环 + goroutine 调度
├── executor/            # 执行器（本地子进程）
├── api/                 # HTTP API + SSE 实时日志
├── storage/             # JSON 文件持久化（执行历史）
├── config/              # 配置定义 + 示例配置
│   ├── config.go
│   └── pipeline.example.json   # 主配置文件
├── i18n/                # 国际化翻译文件
│   ├── zh.json          # 中文
│   └── en.json          # English
├── web/templates/       # 前端模板（Alpine.js + D3.js）
├── data/runs/           # 执行历史记录
│
├── plugins/             # 【插件目录】★
│   ├── shared/          # 通用插件（所有项目共享）
│   │   ├── step_a/
│   │   ├── step_b/
│   │   └── step_c/
│   └── <project-name>/  # 项目专属插件（可选）
│       └── custom_logic/
│
├── projects/            # 项目专属插件目录
│   └── example/         # 示例项目
│
├── docs/                # 设计文档
└── examples/            # 示例脚本
```

---

## 插件开发指南

### 插件目录结构

每个插件是一个**目录**，包含一个清单文件 `plugin.json` 和至少一个入口脚本：

```
plugins/shared/my_plugin/
├── plugin.json      ← 插件清单（必须）
├── main.py          ← 入口脚本
├── utils.py         ← 可包含任意多个辅助文件
└── data/
    └── schema.json  ← 可包含数据/配置文件
```

### plugin.json 清单格式

```json
{
  "name": "my_plugin",
  "label": "我的插件（显示名称）",
  "description": "插件功能描述",
  "version": "1.0",
  "runtime": "python",
  "entry": "main.py",
  "inputs": [
    {
      "name": "url",
      "type": "string",
      "required": true,
      "desc": "请求地址"
    },
    {
      "name": "timeout",
      "type": "number",
      "required": false,
      "desc": "超时时间（秒）"
    }
  ],
  "outputs": [
    {
      "name": "status",
      "type": "string",
      "desc": "执行状态"
    },
    {
      "name": "data",
      "type": "object",
      "desc": "返回数据"
    }
  ]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 插件唯一标识符，工作流中通过此名称引用 |
| `label` | string | 否 | 页面显示名称，默认等于 name |
| `description` | string | 否 | 插件功能描述，在添加步骤时显示 |
| `version` | string | 否 | 版本号，如 "1.0" |
| `runtime` | string | 是 | 运行时：`python`、`shell`、`node` |
| `entry` | string | 是 | 入口脚本文件名（相对于插件目录） |
| `mode` | string | 否 | 执行模式：`"function"`（函数调用）或 `"cli"`（命令行），默认 `"cli"` |
| `entry_function` | string | 否 | 函数模式入口函数名，默认 `"run"` |
| `inputs` | array | 否 | 输入参数定义列表 |
| `outputs` | array | 否 | 输出参数定义列表 |

### 执行模式

插件支持两种执行模式，在 `plugin.json` 中通过 `mode` 字段指定。

#### 函数模式（`mode: "function"`）

引擎通过 Python bridge 脚本动态加载插件模块，调用指定的入口函数（默认 `run`），函数返回值即为步骤输出。

```python
# main.py
def run(params):
    """入口函数：接收参数字典，返回结果字典"""
    print(f"项目: {params.get('project', 'unknown')}")  # stdout → 实时日志
    # ... 业务逻辑 ...
    return {"status": "success", "data": {"ready": True}}
```

- **优点**：直接获取结构化返回值（字典、列表等），无需解析 stdout
- **注意**：Python 元组会被 `json.dump(default=str)` 转为字符串
- **参数传递**：引擎将参数写入临时 JSON 文件，bridge 脚本读取后传入函数

#### CLI 模式（`mode: "cli"`，默认）

引擎以子进程方式执行脚本，参数通过 `--name=value` 命令行参数传入，stdout 全部视为日志。

```python
# main.py
import sys, json, os

# 参数通过 --name=value 传入
# python main.py --project=example --url=https://api.example.com

# stdout 全部为实时日志
print("开始处理...")

# 可选：将结果写入 $RESULT_FILE 指定的 JSON 文件
result_file = os.environ.get("RESULT_FILE")
if result_file:
    with open(result_file, 'w', encoding='utf-8') as f:
        json.dump({"status": "success", "data": {"items": [1,2,3]}}, f)
```

- **优点**：灵活，适合任意语言（Shell / Node.js / 任何可执行文件）
- **结果输出**：可选 — 将结果 JSON 写入 `$RESULT_FILE` 文件，不写则视为纯日志输出
- **系统参数过滤**：`project`、`project_id`、`work_dir` 不会传给用户脚本

### 插件执行环境

- 插件目录会被设为**工作目录**（`cmd.Dir`），因此 `import helpers` 或 `open("data/schema.json")` 等相对路径引用开箱即用
- 环境变量自动设置 `PYTHONIOENCODING=utf-8`、`PYTHONUTF8=1` 确保 UTF-8 输出
- CLI 模式额外设置 `OUTPUT_DIR`（临时输出目录）和 `RESULT_FILE`（结果文件路径）

---

## 配置文件详解

配置文件为 JSON 格式，默认路径 `config/pipeline.example.json`。

### 完整结构

```json
{
  "lang": "zh",
  "servers": {},
  "global": {},
  "workflows": {
    "standard": {
      "label": "标准流程",
      "steps": [...]
    }
  },
  "projects": [
    {
      "name": "my_project",
      "project_id": "001",
      "workflow": "standard",
      "enabled": true
    }
  ]
}
```

### 顶层字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `lang` | string | 否 | 界面语言：`"zh"`（中文）或 `"en"`（英文），默认 `"zh"` |
| `servers` | object | 否 | 远程服务器配置（SSH 执行用） |
| `global` | object | 否 | 全局变量，步骤 config 中可通过 `${key}` 引用 |
| `workflows` | object | 是 | 工作流定义，key 为工作流名称 |
| `projects` | array | 是 | 项目列表 |

### 工作流定义 (WorkflowDef)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `label` | string | 否 | 工作流显示名称 |
| `steps` | array | 是 | 步骤（节点）列表 |

### 步骤定义 (StepDef) — 完整字段

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `plugin` | string | 是 | — | 插件名（全局唯一标识） |
| `type` | string | 否 | `"script"` | 节点类型，见下方节点类型表 |
| `runtime` | string | 否 | `"python"` | 运行时：`python`、`shell`、`node` |
| `script` | string | 否 | `{plugin}.py` | 脚本路径（相对于工作目录或插件目录） |
| `target` | string | 否 | `"local"` | 目标服务器名 |
| `server` | string | 否 | — | 指定 servers 中的服务器 key |
| `mode` | string | 否 | `"cli"` | 执行模式：`"function"` 或 `"cli"` |
| `entry_function` | string | 否 | `"run"` | 函数模式入口函数名 |
| `depends_on` | string[] | 否 | `[]` | 依赖的上游步骤 plugin 名列表 |
| `config` | object | 否 | — | 传递给脚本的额外参数 |
| `inputs` | ParamDef[] | 否 | `[]` | 输入参数定义 |
| `outputs` | ParamDef[] | 否 | `[]` | 输出参数定义 |
| `condition` | string | 否 | — | （type=condition 时）条件表达式 |
| `loop_over` | string | 否 | — | （type=loop 时）遍历数据引用 |
| `true_branch` | string | 否 | — | （type=condition 时）条件为真时执行的步骤名 |
| `false_branch` | string | 否 | — | （type=condition 时）条件为假时执行的步骤名 |

### 参数定义 (ParamDef)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 参数名 |
| `type` | string | 是 | 类型：`string`、`number`、`boolean`、`object`、`array` |
| `desc` | string | 否 | 参数说明 |
| `required` | bool | 否 | 是否必填 |

### 项目定义 (ProjectDef)

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | 是 | 项目名称（唯一标识） |
| `project_id` | string | 否 | 项目编号（默认等于 name） |
| `workflow` | string | 是 | 关联的工作流名称 |
| `enabled` | bool | 否 | 是否启用（默认 true） |
| `zip_source_dir` | string | 否 | 源码压缩目录 |
| `server` | string | 否 | 项目默认服务器 |

---

## 节点类型

### script（脚本节点）— 默认类型

执行一个脚本。支持 Python / Shell / Node.js。

```json
{"plugin": "extract", "type": "script", "runtime": "python", "script": "extract.py"}
```

### condition（条件判断）

执行脚本后评估**条件表达式**，根据结果选择执行分支。

```json
{
  "plugin": "check_result",
  "type": "condition",
  "condition": "$.step_a.data.count > 0",
  "true_branch": "step_b",
  "false_branch": "step_c"
}
```

**条件表达式语法**：
- 引用上一步输出：`$.步骤名.字段路径`
- 支持操作符：`==`、`!=`、`>`、`<`、`>=`、`<=`
- 示例：
  - `$.step_a.data.count > 0` — 数值比较
  - `$.step_a.status == "ok"` — 字符串相等
  - `$.validate.result == true` — 布尔判断

### loop（循环节点）

遍历上游输出的数组数据，对每个元素执行后续步骤。

```json
{
  "plugin": "batch_process",
  "type": "loop",
  "loop_over": "$.step_a.data.items"
}
```

### start / end（开始 / 结束节点）

标记工作流入口和出口，无需脚本，执行时直接通过。

```json
{"plugin": "workflow_start", "type": "start"},
{"plugin": "workflow_end", "type": "end"}
```

---

## 工作流示例

### 条件分支工作流

```json
{
  "workflows": {
    "conditional_flow": {
      "label": "条件分支示例",
      "steps": [
        {"plugin": "start",    "type": "start"},
        {"plugin": "fetch",    "type": "script",    "runtime": "python", "script": "fetch.py",    "depends_on": ["start"]},
        {"plugin": "validate", "type": "condition",  "runtime": "python", "script": "validate.py", "depends_on": ["fetch"],
         "condition": "$.fetch.data.valid == true", "true_branch": "process", "false_branch": "reject"},
        {"plugin": "process",  "type": "script",    "runtime": "python", "script": "process.py",  "depends_on": ["validate"]},
        {"plugin": "reject",   "type": "script",    "runtime": "python", "script": "reject.py",   "depends_on": ["validate"]},
        {"plugin": "end",      "type": "end",        "depends_on": ["process", "reject"]}
      ]
    }
  }
}
```

### 循环工作流

```json
{
  "workflows": {
    "batch_flow": {
      "label": "批量处理示例",
      "steps": [
        {"plugin": "load_list", "type": "script",   "script": "load_list.py",  "depends_on": []},
        {"plugin": "each_item", "type": "loop",      "script": "each_item.py",  "depends_on": ["load_list"],
         "loop_over": "$.load_list.data.items"},
        {"plugin": "aggregate", "type": "script",   "script": "aggregate.py",  "depends_on": ["each_item"]}
      ]
    }
  }
}
```

---

## 国际化

1. 修改配置中 `"lang"` 字段：`"zh"` 为中文，`"en"` 为英文
2. 翻译文件位于 `i18n/zh.json` 和 `i18n/en.json`
3. 前端 JS 中使用 `t('key')` 获取翻译文本（如 `t('wf.target')` → "目标服务器" / "Target"）
4. 模板中使用 `{{tr "key"}}` 获取翻译文本（如 `{{tr "app.title"}}`）

---

## API 参考

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/projects` | 项目列表 |
| `GET` | `/api/projects/{name}` | 项目详情（含完整步骤信息） |
| `POST` | `/api/projects` | 创建项目 |
| `PATCH` | `/api/projects/{name}` | 更新项目（启用/停用） |
| `DELETE` | `/api/projects/{name}` | 删除项目 |
| `GET` | `/api/workflows` | 工作流列表 |
| `POST` | `/api/workflows` | 新建空白工作流 |
| `POST` | `/api/workflows/{name}/clone` | 克隆工作流 |
| `POST` | `/api/projects/{name}/steps` | 添加步骤 |
| `PUT` | `/api/projects/{name}/steps/{plugin}` | 更新步骤 |
| `DELETE` | `/api/projects/{name}/steps/{plugin}` | 删除步骤 |
| `GET` | `/api/plugins?project={name}` | 获取可用插件列表 |
| `GET` | `/api/config` | 获取完整配置 |
| `PUT` | `/api/config` | 保存完整配置 |
| `POST` | `/api/run/{project}` | 执行工作流（`?dry=1` 预览计划，`?from=step` 重跑） |
| `GET` | `/api/sse/{id}` | SSE 实时日志流 |
| `GET` | `/api/history` | 执行历史列表 |
| `GET` | `/api/history/{id}` | 执行详情 |

---

## 页面结构

| 路径 | 页面 | 说明 |
|------|------|------|
| `/projects` | 项目看板 | 项目列表、搜索、创建、启用/停用、删除、执行 |
| `/workflow/{name}` | 工作流编辑器 | DAG 可视化、步骤增删改、节点类型/参数配置 |
| `/run/{name}?rid=xxx` | 实时监控 | SSE 实时日志、DAG 状态、取消执行 |
| `/history` | 执行历史 | 分页列表、筛选、搜索、批量删除 |
| `/history/{id}` | 执行详情 | 步骤结果、耗时分布、日志搜索、重跑 |

---

## 协议

个人/学习免费使用，禁止商用。
