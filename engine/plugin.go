package engine

import (
	"sync"
	"time"
)

type StepState string

const (
	StatePending StepState = "pending"
	StateRunning StepState = "running"
	StateSuccess StepState = "success"
	StateFailed  StepState = "failed"
	StateSkipped StepState = "skipped"
)

type StepResult struct {
	StepName string         `json:"step_name"`
	State    StepState      `json:"state"`
	Data     map[string]any `json:"data"`
	Error    string         `json:"error"`
	Duration float64        `json:"duration"`
	Target   string         `json:"target"`
	Started  time.Time      `json:"-"`
	Ended    time.Time      `json:"-"`
}

type ParamDef struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Desc     string `json:"desc"`
	Required bool   `json:"required"`
}

type Plugin struct {
	StepID    string   // unique step ID within a workflow
	Name      string
	Label     string
	DependsOn []string
	Target    string
	Runtime   string
	Script    string
	Config    map[string]any
	Type      string     // "script", "condition", "loop", "start", "end"
	Mode      string     // "function" 或 "cli"
	EntryFunc string     // 函数模式入口函数名
	Inputs    []ParamDef
	Outputs   []ParamDef
	Condition string     // expression for condition nodes
	LoopOver  string     // $step.field for loop nodes
	// Condition node branching
	TrueBranch  string // step name to execute if true
	FalseBranch string // step name to execute if false
}

type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
}

var GlobalRegistry = &PluginRegistry{plugins: make(map[string]*Plugin)}

func (r *PluginRegistry) Register(name, label string, dependsOn []string) *Plugin {
	if dependsOn == nil {
		dependsOn = []string{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.plugins[name]; ok {
		if len(dependsOn) > 0 {
			existing.DependsOn = dependsOn
		}
		return existing
	}
	p := &Plugin{Name: name, Label: label, DependsOn: dependsOn, Target: "local"}
	r.plugins[name] = p
	return p
}

func (r *PluginRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = make(map[string]*Plugin)
}

func (r *PluginRegistry) Get(name string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

func (r *PluginRegistry) ListAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.plugins))
	for n := range r.plugins {
		names = append(names, n)
	}
	return names
}

func (r *PluginRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

func (r *PluginRegistry) BuildDeps() {}
