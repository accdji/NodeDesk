package mapper

import (
	"fmt"
	"strings"
)

// Rule 定义字段映射规则：Source→Target 路径，可附带 Transform
type Rule struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Transform string `json:"transform,omitempty"` // 转换函数名（可选）
}

// Apply 按规则列表把源数据映射到目标结构
func Apply(data map[string]any, rules []Rule) map[string]any {
	result := make(map[string]any)
	for _, rule := range rules {
		val := getByPath(data, rule.Source)
		if rule.Transform != "" {
			val = applyTransform(val, rule.Transform)
		}
		setByPath(result, rule.Target, val)
	}
	return result
}

// getByPath 按 "." 分隔路径从嵌套 map 中取值
// 如 "user.address.city" → data["user"]["address"]["city"]
func getByPath(data map[string]any, path string) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur any = data
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
		if cur == nil {
			return nil
		}
	}
	return cur
}

// setByPath 按 "." 分隔路径向嵌套 map 中写入值，自动创建中间 map
func setByPath(data map[string]any, path string, value any) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	parts := strings.Split(path, ".")
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		next, ok := data[part]
		if !ok {
			next = make(map[string]any)
			data[part] = next
		}
		data, ok = next.(map[string]any)
		if !ok {
			// 路径冲突：非 map 值被当作路径中间节点，覆盖
			newMap := make(map[string]any)
			data[part] = newMap
			data = newMap
		}
	}
	data[parts[len(parts)-1]] = value
}

// registered transforms
var transforms = map[string]func(any) any{
	"multiply_100": func(v any) any {
		switch x := v.(type) {
		case float64:
			return x * 100
		case int:
			return x * 100
		case int64:
			return x * 100
		}
		return v
	},
	"divide_100": func(v any) any {
		switch x := v.(type) {
		case float64:
			return x / 100
		case int:
			return float64(x) / 100
		case int64:
			return float64(x) / 100
		}
		return v
	},
	"toString": func(v any) any {
		return fmt.Sprint(v)
	},
	"toInt": func(v any) any {
		switch x := v.(type) {
		case float64:
			return int(x)
		case string:
			var n int
			fmt.Sscanf(x, "%d", &n)
			return n
		}
		return v
	},
}

func applyTransform(val any, name string) any {
	if fn, ok := transforms[name]; ok {
		return fn(val)
	}
	return val
}
