"""
多语言任务示例 — stdin JSON → stdout JSON

用法:
  echo '{"params": {"name": "test"}}' | python examples/task.py
"""
import sys
import json

# 读取 stdin JSON
input_data = json.loads(sys.stdin.read())
params = input_data.get("params", {})

# 执行核心逻辑
name = params.get("name", "unknown")
print(f"处理中: {name}")      # 这行会被 Go 引擎当作日志推送

# 输出结果
result = {"status": "success", "data": {"output": f"hello {name}", "count": 42}}
print(json.dumps(result))     # 最后一行 JSON = 执行结果
