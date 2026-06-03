package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"workflow/config"
	"workflow/engine"
	"workflow/executor"
	"workflow/storage"
)

// pageData 传递给 layout 模板的页面数据
type pageData map[string]any

// SSE 客户端连接
type sseClient struct {
	ch     chan string
	runID  string
}

var (
	sseClients   = make(map[string][]*sseClient)
	sseMu        sync.Mutex

	currentCfg   *config.PipelineConfig
	cfgMu        sync.RWMutex
)

func SetConfig(cfg *config.PipelineConfig) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	currentCfg = cfg
}

func RegisterPlugins(cfg *config.PipelineConfig) {
	for _, wf := range cfg.Workflows {
		for _, step := range wf.Steps {
			p := engine.GlobalRegistry.Register(step.Plugin, step.Plugin, step.DependsOn)
			p.Target = step.Target
			p.Runtime = step.Runtime
			p.Script = step.Script
			p.Config = step.Config
		}
	}
}

func getConfig() *config.PipelineConfig {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return currentCfg
}

func genID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SetupRoutes 注册所有路由（使用 Go 1.22 标准库路由）
func SetupRoutes(mux *http.ServeMux, tmpl *template.Template) {
	// 页面 — 每个页面通过唯一的 Body 模板名渲染内容区
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "layout.html", pageData{"Body": "projectsBody"})
	})
	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "layout.html", pageData{"Body": "projectsBody"})
	})
	mux.HandleFunc("GET /workflow/{project}", func(w http.ResponseWriter, r *http.Request) {
		proj := r.PathValue("project")
		tmpl.ExecuteTemplate(w, "layout.html", pageData{"Body": "workflowBody", "Project": proj})
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "layout.html", pageData{"Body": "historyBody"})
	})

	// API - 插件
	mux.HandleFunc("GET /api/plugins", func(w http.ResponseWriter, r *http.Request) {
		plugins := make([]map[string]any, 0)
		for _, name := range engine.GlobalRegistry.ListAll() {
			p, ok := engine.GlobalRegistry.Get(name)
			if !ok { continue }
			plugins = append(plugins, map[string]any{
				"name": p.Name, "label": p.Label,
				"depends_on": p.DependsOn, "target": p.Target,
			})
		}
		writeJSON(w, plugins)
	})

	// API - 工作流列表
	mux.HandleFunc("GET /api/workflows", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { writeJSON(w, []string{}); return }
		workflows := make([]map[string]any, 0)
		for name, wf := range cfg.Workflows {
			names := make([]string, len(wf.Steps))
			for i, s := range wf.Steps { names[i] = s.Plugin }
			workflows = append(workflows, map[string]any{"name": name, "label": wf.Label, "steps": names})
		}
		writeJSON(w, workflows)
	})

	// API - 项目列表
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { writeJSON(w, []string{}); return }
		projects := make([]map[string]any, 0)
		for _, proj := range cfg.Projects {
			projects = append(projects, map[string]any{
				"name": proj.Name, "project_id": proj.ProjectID,
				"workflow": proj.Workflow, "enabled": proj.Enabled,
			})
		}
		writeJSON(w, projects)
	})

	// API - 执行
	mux.HandleFunc("POST /api/run/{project}", handleRun)

	// API - 历史
	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { writeJSON(w, []interface{}{}); return }
		runs, err := store.GetHistory()
		if err != nil { http.Error(w, err.Error(), 500); return }
		writeJSON(w, runs)
	})

	mux.HandleFunc("GET /api/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { http.NotFound(w, r); return }
		doc, err := store.GetRunDetail(r.PathValue("id"))
		if err != nil { http.NotFound(w, r); return }
		writeJSON(w, doc)
	})

	// SSE 日志流
	mux.HandleFunc("GET /api/sse/{id}", handleSSE)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	dryRun := r.URL.Query().Get("dry") == "1"
	fromStep := r.URL.Query().Get("from")

	cfg := getConfig()
	if cfg == nil { http.Error(w, "未加载配置", 400); return }

	var proj *config.ProjectDef
	for i := range cfg.Projects {
		if cfg.Projects[i].Name == projectName { proj = &cfg.Projects[i]; break }
	}
	if proj == nil { http.Error(w, "项目不存在: "+projectName, 404); return }

	wf, ok := cfg.Workflows[proj.Workflow]
	if !ok { http.Error(w, "工作流不存在: "+proj.Workflow, 404); return }

	steps := make([]*engine.Plugin, len(wf.Steps))
	for i, s := range wf.Steps {
		p, ok := engine.GlobalRegistry.Get(s.Plugin)
		if !ok {
			p = engine.GlobalRegistry.Register(s.Plugin, s.Plugin, s.DependsOn)
			p.Target = s.Target
			p.Runtime = s.Runtime
			p.Script = s.Script
			p.Config = s.Config
		}
		steps[i] = p
	}

	dag := engine.BuildDAG(steps)
	order, err := engine.TopologicalSort(steps, dag)
	if err != nil { http.Error(w, err.Error(), 400); return }

	runID := genID()

	if dryRun {
		plan := formatPlan(order, steps)
		writeJSON(w, map[string]any{"plan": plan, "run_id": runID, "dry_run": true})
		return
	}

	store := storage.GetStore()
	if store != nil {
		store.SaveRun(storage.RunRecord{
			ID: runID, ProjectName: projectName, WorkflowName: proj.Workflow,
			Status: "running", CreatedAt: time.Now(),
		})
	}

	go executeWorkflow(runID, projectName, proj, wf, steps, order, dag, fromStep)

	writeJSON(w, map[string]string{"run_id": runID, "status": "started"})
}

func executeWorkflow(runID, projectName string, proj *config.ProjectDef, wf config.WorkflowDef,
	steps []*engine.Plugin, order []string, dag engine.DAG, fromStep string) {

	defer func() {
		if r := recover(); r != nil { log.Printf("执行 panic [%s]: %v", runID, r) }
	}()

	store := storage.GetStore()
	runner := engine.NewRunner()
	runner.LogFunc = func(rid, step, msg string) {
		ts := time.Now().Format("15:04:05")
		if store != nil {
			store.AppendLog(storage.LogEntry{
				RunID: rid, StepName: step, Timestamp: ts, Level: "INFO", Message: msg,
			})
		}
		broadcastSSE(rid, step, ts, msg)
	}

	err := runner.Run(runID, steps, order, dag, func(p *engine.Plugin) (map[string]any, error) {
		if store != nil {
			store.SaveStep(storage.StepRecord{
				RunID: runID, StepName: p.Name, State: string(engine.StateRunning), Target: p.Target,
			})
		}

		runtime := p.Runtime
		if runtime == "" { runtime = "python" }
		script := p.Script
		if script == "" { script = p.Name + ".py" }
		script = executor.ResolveScript(projectName, script)

		params := map[string]any{"project": projectName, "project_id": proj.ProjectID, "work_dir": "."}
		if p.Config != nil {
			for k, v := range p.Config { params[k] = v }
		}

		result, execErr := executor.ExecuteLocal(runtime, script, params,
			func(line string) {
				ts := time.Now().Format("15:04:05")
				if store != nil {
					store.AppendLog(storage.LogEntry{
						RunID: runID, StepName: p.Name, Timestamp: ts, Level: "INFO", Message: line,
					})
				}
				broadcastSSE(runID, p.Name, ts, line)
			},
		)

		if execErr != nil {
			if store != nil {
				store.SaveStep(storage.StepRecord{
					RunID: runID, StepName: p.Name, State: string(engine.StateFailed), Error: execErr.Error(), Target: p.Target,
				})
			}
			return nil, execErr
		}

		dataJSON, _ := json.Marshal(result.Data)
		if store != nil {
			store.SaveStep(storage.StepRecord{
				RunID: runID, StepName: p.Name, State: string(engine.StateSuccess), Data: string(dataJSON), Target: p.Target,
			})
		}
		return result.Data, nil
	}, false, fromStep)

	status := "success"
	if err != nil { status = "failed" }
	if store != nil { store.UpdateRunStatus(runID, status) }
	broadcastSSEEvent(runID, "completed", status)
}

// ========== SSE ==========

func handleSSE(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")

	flusher, ok := w.(http.Flusher)
	if !ok { http.Error(w, "不支持流式传输", 500); return }

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 100)
	client := &sseClient{ch: ch, runID: runID}

	sseMu.Lock()
	sseClients[runID] = append(sseClients[runID], client)
	sseMu.Unlock()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			sseMu.Lock()
			clients := sseClients[runID]
			for i, c := range clients {
				if c == client {
					sseClients[runID] = append(clients[:i], clients[i+1:]...)
					break
				}
			}
			sseMu.Unlock()
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func broadcastSSE(runID, stepName, timestamp, message string) {
	sseMu.Lock()
	defer sseMu.Unlock()

	msg := map[string]string{
		"type": "log", "run_id": runID, "step_name": stepName,
		"timestamp": timestamp, "message": message,
	}
	data, _ := json.Marshal(msg)

	for _, c := range sseClients[runID] {
		select {
		case c.ch <- string(data):
		default:
		}
	}
}

func broadcastSSEEvent(runID, eventType, status string) {
	sseMu.Lock()
	defer sseMu.Unlock()

	msg := map[string]string{"type": eventType, "run_id": runID, "status": status}
	data, _ := json.Marshal(msg)

	for _, c := range sseClients[runID] {
		select {
		case c.ch <- string(data):
		default:
		}
	}
}

// ========== 辅助 ==========

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func formatPlan(order []string, steps []*engine.Plugin) []map[string]any {
	pmap := make(map[string]*engine.Plugin)
	for _, p := range steps { pmap[p.Name] = p }
	result := make([]map[string]any, 0)
	for i, name := range order {
		p := pmap[name]
		result = append(result, map[string]any{
			"index": i + 1, "plugin": name, "target": p.Target, "depends_on": p.DependsOn,
		})
	}
	return result
}
