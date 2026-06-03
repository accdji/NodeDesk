package engine

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Runner struct {
	mu      sync.Mutex
	results map[string]*StepResult
	LogFunc func(runID, step, msg string)
}

func NewRunner() *Runner {
	return &Runner{results: make(map[string]*StepResult)}
}

func (r *Runner) Run(runID string, steps []*Plugin, order []string, dag DAG,
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
					r.emitLog(runID, "", fmt.Sprintf("[%d] %s target=%s", i+1, name, p.Target))
				}
			}
		}
		return nil
	}

	pmap := make(map[string]*Plugin)
	for _, p := range steps {
		pmap[p.Name] = p
	}

	for _, name := range order[startIdx:] {
		p, ok := pmap[name]
		if !ok {
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
			continue
		}

		result.State = StateRunning
		r.mu.Lock()
		r.results[name] = result
		r.mu.Unlock()

		r.emitLog(runID, name, fmt.Sprintf("开始执行 (target=%s)", p.Target))
		t0 := time.Now()

		data, err := execFn(p)
		result.Duration = time.Since(t0).Seconds()
		result.Ended = time.Now()

		if err != nil {
			result.State = StateFailed
			result.Error = err.Error()
			r.emitLog(runID, name, fmt.Sprintf("失败: %v", err))
		} else {
			result.State = StateSuccess
			result.Data = data
			r.emitLog(runID, name, fmt.Sprintf("完成 (%.1fs)", result.Duration))
		}

		r.mu.Lock()
		r.results[name] = result
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
