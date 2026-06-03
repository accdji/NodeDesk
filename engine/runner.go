package engine

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Runner struct {
	mu            sync.Mutex
	results       map[string]*StepResult
	LogFunc       func(runID, step, msg string)
	StepStartFunc func(runID, step, target string)
	StepEndFunc   func(runID, step, state string, duration float64, errMsg string)
}

func NewRunner() *Runner {
	return &Runner{results: make(map[string]*StepResult)}
}

func (r *Runner) Run(ctx context.Context, runID string, steps []*Plugin, order []string, dag DAG,
	execFn func(*Plugin) (map[string]any, error),
	dryRun bool, fromStep string) error {

	r.mu.Lock()
	r.results = make(map[string]*StepResult)
	r.mu.Unlock()

	startIdx := 0
	if fromStep != "" {
		for i, n := range order {
			if n == fromStep {
				startIdx = i
				break
			}
		}
		ds := GetDownstream(dag, fromStep)
		for name := range ds {
			r.emitLog(runID, name, "已重置（上游重跑）")
		}
	}

	if dryRun {
		r.emitLog(runID, "", "=== 执行计划 ===")
		for i, name := range order[startIdx:] {
			for _, p := range steps {
				if p.Name == name {
					r.emitLog(runID, "", fmt.Sprintf("[%d] %s type=%s target=%s", i+1, name, p.Type, p.Target))
				}
			}
		}
		return nil
	}

	pmap := make(map[string]*Plugin)
	for _, p := range steps {
		pmap[p.Name] = p
	}

	// 被跳过的步骤集合（条件分支中未选中的路径）
	skipSet := make(map[string]bool)

	for _, name := range order[startIdx:] {
		// 检查取消
		select {
		case <-ctx.Done():
			r.emitLog(runID, name, "执行已取消")
			r.mu.Lock()
			for _, remaining := range order[startIdx:] {
				if _, exists := r.results[remaining]; !exists {
					r.results[remaining] = &StepResult{StepName: remaining, State: StateSkipped, Error: "已取消"}
				}
			}
			r.mu.Unlock()
			return ctx.Err()
		default:
		}

		p, ok := pmap[name]
		if !ok {
			continue
		}

		// 被条件分支跳过的步骤
		if skipSet[name] {
			r.mu.Lock()
			if _, exists := r.results[name]; !exists {
				r.results[name] = &StepResult{StepName: name, Target: p.Target, State: StateSkipped, Error: "条件分支未选中", Ended: time.Now()}
				if r.StepEndFunc != nil {
					r.StepEndFunc(runID, name, string(StateSkipped), 0, "条件分支未选中")
				}
			}
			r.mu.Unlock()
			r.emitLog(runID, name, "跳过: 条件分支未选中")
			continue
		}

		skip := false
		for _, dep := range p.DependsOn {
			r.mu.Lock()
			res, exists := r.results[dep]
			r.mu.Unlock()
			if !exists || res.State != StateSuccess {
				r.emitLog(runID, name, fmt.Sprintf("跳过: 上游 %s 未成功", dep))
				skip = true
				break
			}
		}

		result := &StepResult{StepName: name, Target: p.Target, Started: time.Now()}
		if skip {
			result.State = StateSkipped
			result.Error = "上游依赖未满足"
			result.Ended = time.Now()
			r.mu.Lock()
			r.results[name] = result
			r.mu.Unlock()
			if r.StepEndFunc != nil {
				r.StepEndFunc(runID, name, string(StateSkipped), 0, result.Error)
			}
			continue
		}

		// ====== 特殊节点类型处理 ======

		// start / end 节点：无需执行脚本
		if p.Type == "start" {
			result.State = StateSuccess
			result.Data = map[string]any{"msg": "开始"}
			result.Ended = time.Now()
			r.emitLog(runID, name, "→ 开始")
			if r.StepEndFunc != nil {
				r.StepEndFunc(runID, name, string(StateSuccess), 0, "")
			}
			r.mu.Lock()
			r.results[name] = result
			r.mu.Unlock()
			continue
		}
		if p.Type == "end" {
			result.State = StateSuccess
			result.Data = map[string]any{"msg": "结束"}
			result.Ended = time.Now()
			r.emitLog(runID, name, "→ 结束")
			if r.StepEndFunc != nil {
				r.StepEndFunc(runID, name, string(StateSuccess), 0, "")
			}
			r.mu.Lock()
			r.results[name] = result
			r.mu.Unlock()
			continue
		}

		result.State = StateRunning
		r.mu.Lock()
		r.results[name] = result
		r.mu.Unlock()

		if r.StepStartFunc != nil {
			r.StepStartFunc(runID, name, p.Target)
		}
		r.emitLog(runID, name, fmt.Sprintf("开始执行 (type=%s target=%s)", p.Type, p.Target))
		t0 := time.Now()

		// 执行脚本
		data, err := execFn(p)
		result.Duration = time.Since(t0).Seconds()
		result.Ended = time.Now()

		if err != nil {
			result.State = StateFailed
			result.Error = err.Error()
			r.emitLog(runID, name, fmt.Sprintf("失败: %v", err))
			if r.StepEndFunc != nil {
				r.StepEndFunc(runID, name, string(StateFailed), result.Duration, err.Error())
			}
		} else {
			result.State = StateSuccess
			result.Data = data
			r.emitLog(runID, name, fmt.Sprintf("完成 (%.1fs)", result.Duration))
			if r.StepEndFunc != nil {
				r.StepEndFunc(runID, name, string(StateSuccess), result.Duration, "")
			}
		}

		r.mu.Lock()
		r.results[name] = result

		// 条件节点：根据条件结果标记分支
		if p.Type == "condition" && result.State == StateSuccess && p.Condition != "" {
			condResult, condErr := evaluateCondition(p.Condition, r.results)
			if condErr != nil {
				r.emitLog(runID, name, fmt.Sprintf("条件评估失败: %v", condErr))
			} else {
				result.Data["condition_result"] = condResult
				r.emitLog(runID, name, fmt.Sprintf("条件结果: %v → %s", condResult, map[bool]string{true: "True分支", false: "False分支"}[condResult]))
				// 跳过未选中的分支
				skipBranch := p.FalseBranch
				if !condResult {
					skipBranch = p.TrueBranch
				}
				if skipBranch != "" {
					downstream := GetDownstream(dag, skipBranch)
					downstream[skipBranch] = true
					for ds := range downstream {
						skipSet[ds] = true
					}
					r.emitLog(runID, name, fmt.Sprintf("跳过分支: %s (%d 个步骤)", skipBranch, len(downstream)))
				}
			}
		}
		// 循环节点：标记循环元数据
		if p.Type == "loop" && result.State == StateSuccess && p.LoopOver != "" {
			if loopData, lErr := resolveRef(p.LoopOver, r.results); lErr == nil {
				result.Data["loop_data"] = loopData
				r.emitLog(runID, name, fmt.Sprintf("循环数据: %v", loopData))
			} else {
				r.emitLog(runID, name, fmt.Sprintf("循环数据解析失败: %v", lErr))
			}
		}

		r.mu.Unlock()
	}
	return nil
}

func (r *Runner) GetResults() map[string]*StepResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*StepResult, len(r.results))
	for k, v := range r.results {
		out[k] = v
	}
	return out
}

func (r *Runner) emitLog(runID, step, msg string) {
	if r.LogFunc != nil {
		r.LogFunc(runID, step, msg)
	}
	log.Printf("[%s][%s] %s", runID, step, msg)
}

// evaluateCondition 评估条件表达式，如 "$.step_a.data.count > 0"
func evaluateCondition(expr string, results map[string]*StepResult) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	var op string
	for _, o := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		if strings.Contains(expr, o) {
			op = o
			break
		}
	}
	if op == "" {
		return false, fmt.Errorf("条件表达式缺少操作符: %s", expr)
	}
	parts := strings.SplitN(expr, op, 2)
	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	lv, err := resolveRef(left, results)
	if err != nil {
		return false, fmt.Errorf("解析左侧值失败 [%s]: %v", left, err)
	}
	rv, err := resolveRef(right, results)
	if err != nil {
		return false, fmt.Errorf("解析右侧值失败 [%s]: %v", right, err)
	}

	lv, rv = coerceTypes(lv, rv)
	return compare(lv, rv, op)
}

func resolveRef(ref string, results map[string]*StepResult) (any, error) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, "$.") {
		if ref == "true" {
			return true, nil
		}
		if ref == "false" {
			return false, nil
		}
		if n, err := strconv.ParseFloat(ref, 64); err == nil {
			return n, nil
		}
		return strings.Trim(ref, "\"'"), nil
	}
	path := strings.TrimPrefix(ref, "$.")
	parts := strings.Split(path, ".")
	stepName := parts[0]
	res, ok := results[stepName]
	if !ok {
		return nil, fmt.Errorf("步骤 %s 尚未执行", stepName)
	}
	if res.State != StateSuccess {
		return nil, fmt.Errorf("步骤 %s 未成功完成", stepName)
	}
	var cur any = res.Data
	for _, part := range parts[1:] {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("无法访问字段 %s（非对象）", part)
		}
		cur = m[part]
	}
	return cur, nil
}

func coerceTypes(a, b any) (any, any) {
	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != nil && tb != nil && ta.Kind() == reflect.Float64 && tb.Kind() == reflect.String {
		if n, err := strconv.ParseFloat(b.(string), 64); err == nil {
			return a, n
		}
	}
	if ta != nil && tb != nil && ta.Kind() == reflect.String && tb.Kind() == reflect.Float64 {
		if n, err := strconv.ParseFloat(a.(string), 64); err == nil {
			return n, b
		}
	}
	return a, b
}

func compare(a, b any, op string) (bool, error) {
	fa, aOk := toFloat(a)
	fb, bOk := toFloat(b)
	if aOk && bOk {
		switch op {
		case "==":
			return fa == fb, nil
		case "!=":
			return fa != fb, nil
		case ">":
			return fa > fb, nil
		case "<":
			return fa < fb, nil
		case ">=":
			return fa >= fb, nil
		case "<=":
			return fa <= fb, nil
		}
	}
	sa := fmt.Sprint(a)
	sb := fmt.Sprint(b)
	switch op {
	case "==":
		return sa == sb, nil
	case "!=":
		return sa != sb, nil
	default:
		return false, fmt.Errorf("操作符 %s 不支持字符串比较", op)
	}
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		n, err := strconv.ParseFloat(val, 64)
		return n, err == nil
	}
	return 0, false
}
