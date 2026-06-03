# 轻量级工作流编排器

Go 标准库实现的插件式 DAG 工作流引擎，单二进制零依赖。

## 快速开始

```bash
go build -o workflow.exe .
./workflow.exe
# 打开浏览器 http://localhost:8080
```

## 目录结构

```
├── main.go              # 入口
├── engine/              # DAG 引擎 + goroutine 调度
├── executor/            # 执行器（子进程 / SSH）
├── api/                 # HTTP API + SSE 实时日志
├── storage/             # JSON 文件持久化
├── config/              # 配置加载 + 示例
├── web/templates/       # 中文前端
│
├── tasks/               # 【通用任务】所有项目共享
│   ├── extract.py
│   └── convert.py
│
└── projects/            # 【项目配置+专属任务】
    └── my_project/
        ├── pipeline.json    # 工作流定义
        └── tasks/           # 专属任务（优先级高于通用）
            └── custom.py
```

## 任务脚本：通用 vs 专属

配置中的 `script` 只写文件名（如 `convert.py`），引擎自动按优先级查找：

1. `projects/<name>/tasks/<script>` — **专属任务**，优先
2. `tasks/<script>` — **通用任务**，所有项目共享

```json
{
  "steps": [
    {"plugin": "extract", "script": "extract.py", ...},       // 通用
    {"plugin": "custom",  "script": "custom.py",  ...}        // 专属（只有这个项目有）
  ]
}
```

## 多语言任务

任何语言编写任务脚本，通过 stdin 读 JSON 参数，stdout 输出 JSON 结果：

```python
import sys, json

input_data = json.loads(sys.stdin.read())  # {"params": {...}}
result = {"status": "success", "data": {"output": "done"}}
print(json.dumps(result))
```

## 配置文件

在 `projects/<name>/pipeline.json` 中定义工作流和项目：

```json
{
  "workflows": {
    "standard": {
      "label": "标准流程",
      "steps": [
        {"plugin": "extract", "target": "local", "runtime": "python", "script": "extract.py", "depends_on": []},
        {"plugin": "convert", "target": "local", "runtime": "python", "script": "convert.py", "depends_on": ["extract"]}
      ]
    }
  },
  "projects": [
    {"name": "my_project", "project_id": "001", "workflow": "standard", "enabled": true}
  ]
}
```

## 协议

个人/学习免费使用，禁止商用。
