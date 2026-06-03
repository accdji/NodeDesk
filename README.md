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
│   ├── plugin.go        # 插件注册表
│   ├── dag.go           # 拓扑排序 (Kahn)
│   └── runner.go        # 工作流执行器
├── executor/
│   └── local.go         # 子进程执行器（多语言支持）
├── api/
│   └── server.go        # HTTP API + SSE 实时日志
├── storage/
│   └── store.go         # JSON 文件持久化
├── config/
│   └── config.go        # 配置加载
└── web/templates/       # 中文前端 (HTMX + Alpine.js + D3.js)
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
        {"plugin": "extract", "target": "local", "runtime": "python", "script": "tasks/extract.py", "depends_on": []},
        {"plugin": "convert", "target": "local", "runtime": "python", "script": "tasks/convert.py", "depends_on": ["extract"]}
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
