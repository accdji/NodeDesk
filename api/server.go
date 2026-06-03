package api

import (
	"context"
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
	"workflow/plugins"
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

	translations  map[string]string
	lang          string
)

func SetConfig(cfg *config.PipelineConfig) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	currentCfg = cfg
}

func SetTranslations(l string, tr map[string]string) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	lang = l
	translations = tr
}

func buildPageData(body string, extra map[string]any) pageData {
	pd := pageData{"Body": body, "Lang": lang, "Tr": translations}
	for k, v := range extra { pd[k] = v }
	return pd
}

func RegisterPlugins(cfg *config.PipelineConfig) {
	for _, wf := range cfg.Workflows {
		for _, step := range wf.Steps {
			p := engine.GlobalRegistry.Register(step.Plugin, step.Plugin, step.DependsOn)
			p.Target = step.Target
			p.Runtime = step.Runtime
			p.Script = step.Script
			p.Config = step.Config
			p.Type = step.Type
			if p.Type == "" { p.Type = "script" }
			for _, in := range step.Inputs { p.Inputs = append(p.Inputs, engine.ParamDef{Name: in.Name, Type: in.Type, Desc: in.Desc, Required: in.Required}) }
			for _, out := range step.Outputs { p.Outputs = append(p.Outputs, engine.ParamDef{Name: out.Name, Type: out.Type, Desc: out.Desc, Required: out.Required}) }
			p.Mode = step.Mode
			p.EntryFunc = step.EntryFunction
			p.Condition = step.Condition
			p.LoopOver = step.LoopOver
			p.TrueBranch = step.TrueBranch
			p.FalseBranch = step.FalseBranch
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
		tmpl.ExecuteTemplate(w, "layout.html", buildPageData("projectsBody", map[string]any{"ConfigPath": config.GetPath()}))
	})
	mux.HandleFunc("GET /projects", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "layout.html", buildPageData("projectsBody", map[string]any{"ConfigPath": config.GetPath()}))
	})
	mux.HandleFunc("GET /workflow/{project}", func(w http.ResponseWriter, r *http.Request) {
		proj := r.PathValue("project")
		tmpl.ExecuteTemplate(w, "layout.html", buildPageData("workflowBody", map[string]any{"Project": proj, "ConfigPath": config.GetPath()}))
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
		tmpl.ExecuteTemplate(w, "layout.html", buildPageData("historyBody", nil))
	})

	// API - 项目详情（含工作流步骤完整信息）
	mux.HandleFunc("GET /api/projects/{name}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		var proj *config.ProjectDef
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name { proj = &cfg.Projects[i]; break }
		}
		if proj == nil { http.NotFound(w, r); return }
		wf, ok := cfg.Workflows[proj.Workflow]
		if !ok { http.NotFound(w, r); return }
		steps := make([]map[string]any, len(wf.Steps))
		for i, s := range wf.Steps {
			steps[i] = map[string]any{
				"plugin": s.Plugin, "type": s.Type, "runtime": s.Runtime, "script": s.Script,
				"target": s.Target, "server": s.Server, "mode": s.Mode, "entry_function": s.EntryFunction,
				"config": s.Config, "depends_on": s.DependsOn,
				"inputs": s.Inputs, "outputs": s.Outputs,
				"condition": s.Condition, "loop_over": s.LoopOver,
				"true_branch": s.TrueBranch, "false_branch": s.FalseBranch,
			}
		}
		writeJSON(w, map[string]any{
			"name": proj.Name, "project_id": proj.ProjectID,
			"workflow": proj.Workflow, "enabled": proj.Enabled,
			"workflow_label": wf.Label, "steps": steps,
		})
	})

	// API - 创建项目
	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.Error(w, "未加载配置", 400); return }
		var req struct {
			Name     string `json:"name"`
			ID       string `json:"project_id"`
			Workflow string `json:"workflow"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
			http.Error(w, "缺少 name 字段", 400); return
		}
		for _, p := range cfg.Projects {
			if p.Name == req.Name { http.Error(w, "项目已存在", 409); return }
		}
		if req.ID == "" { req.ID = req.Name }
		if req.Workflow == "" { req.Workflow = "standard" }
		cfg.Projects = append(cfg.Projects, config.ProjectDef{
			Name: req.Name, ProjectID: req.ID, Workflow: req.Workflow, Enabled: true,
		})
		if err := config.Save(cfg); err != nil {
			http.Error(w, "保存失败: "+err.Error(), 500); return
		}
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// API - 查看/保存配置
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.Error(w, "未加载配置", 500); return }
		writeJSON(w, map[string]any{
			"path": config.GetPath(),
			"config": cfg,
		})
	})

	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.PipelineConfig
		if json.NewDecoder(r.Body).Decode(&cfg) != nil {
			http.Error(w, "JSON 解析失败", 400); return
		}
		if err := config.Save(&cfg); err != nil {
			http.Error(w, "保存失败: "+err.Error(), 500); return
		}
		engine.GlobalRegistry.Clear()
		SetConfig(&cfg)
		RegisterPlugins(&cfg)
		log.Printf("配置已重新加载: %d 个工作流, %d 个项目", len(cfg.Workflows), len(cfg.Projects))
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// API - 项目更新/删除
	mux.HandleFunc("PATCH /api/projects/{name}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		var req struct{ Enabled *bool `json:"enabled"` }
		json.NewDecoder(r.Body).Decode(&req)
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name {
				if req.Enabled != nil { cfg.Projects[i].Enabled = *req.Enabled }
				config.Save(cfg)
				SetConfig(cfg)
				RegisterPlugins(cfg)
				writeJSON(w, map[string]string{"status": "ok"})
				return
			}
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("DELETE /api/projects/{name}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name {
				cfg.Projects = append(cfg.Projects[:i], cfg.Projects[i+1:]...)
				config.Save(cfg)
				SetConfig(cfg)
				RegisterPlugins(cfg)
				writeJSON(w, map[string]string{"status": "ok"})
				return
			}
		}
		http.NotFound(w, r)
	})

	// API - 工作流管理
	mux.HandleFunc("POST /api/workflows", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.Error(w, "未加载配置", 500); return }
		var req struct{ Name string `json:"name"` }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" { http.Error(w, "缺少 name", 400); return }
		if _, ok := cfg.Workflows[req.Name]; ok { http.Error(w, "工作流已存在", 409); return }
		cfg.Workflows[req.Name] = config.WorkflowDef{Label: req.Name, Steps: []config.StepDef{}}
		config.Save(cfg)
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/workflows/{name}/clone", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		src, ok := cfg.Workflows[name]
		if !ok { http.NotFound(w, r); return }
		var req struct{ Name string `json:"name"` }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" { req.Name = name + "_copy" }
		if _, ok := cfg.Workflows[req.Name]; ok { http.Error(w, "工作流已存在", 409); return }
		steps := make([]config.StepDef, len(src.Steps))
		copy(steps, src.Steps)
		cfg.Workflows[req.Name] = config.WorkflowDef{Label: req.Name, Steps: steps}
		config.Save(cfg)
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// API - 步骤增删改
	mux.HandleFunc("POST /api/projects/{name}/steps", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		var proj *config.ProjectDef
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name { proj = &cfg.Projects[i]; break }
		}
		if proj == nil { http.NotFound(w, r); return }
		wf, ok := cfg.Workflows[proj.Workflow]
		if !ok { http.Error(w, "工作流不存在", 404); return }
		var step config.StepDef
		if json.NewDecoder(r.Body).Decode(&step) != nil || step.Plugin == "" {
			http.Error(w, "缺少 plugin 字段", 400); return
		}
		if step.Runtime == "" { step.Runtime = "python" }
		if step.Script == "" { step.Script = step.Plugin + ".py" }
		wf.Steps = append(wf.Steps, step)
		cfg.Workflows[proj.Workflow] = wf
		if err := config.Save(cfg); err != nil { http.Error(w, "保存失败", 500); return }
		engine.GlobalRegistry.Clear()
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("DELETE /api/projects/{name}/steps/{plugin}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name, plugin := r.PathValue("name"), r.PathValue("plugin")
		var proj *config.ProjectDef
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name { proj = &cfg.Projects[i]; break }
		}
		if proj == nil { http.NotFound(w, r); return }
		wf, ok := cfg.Workflows[proj.Workflow]
		if !ok { http.Error(w, "工作流不存在", 404); return }
		idx := -1
		for i, s := range wf.Steps { if s.Plugin == plugin { idx = i; break } }
		if idx < 0 { http.NotFound(w, r); return }
		wf.Steps = append(wf.Steps[:idx], wf.Steps[idx+1:]...)
		cfg.Workflows[proj.Workflow] = wf
		if err := config.Save(cfg); err != nil { http.Error(w, "保存失败", 500); return }
		engine.GlobalRegistry.Clear()
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("PUT /api/projects/{name}/steps/{plugin}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name, plugin := r.PathValue("name"), r.PathValue("plugin")
		var proj *config.ProjectDef
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name { proj = &cfg.Projects[i]; break }
		}
		if proj == nil { http.NotFound(w, r); return }
		wf, ok := cfg.Workflows[proj.Workflow]
		if !ok { http.Error(w, "工作流不存在", 404); return }
		var updated config.StepDef
		if json.NewDecoder(r.Body).Decode(&updated) != nil { http.Error(w, "JSON 解析失败", 400); return }
		found := false
		for i, s := range wf.Steps {
			if s.Plugin == plugin {
				if updated.Plugin != "" { wf.Steps[i].Plugin = updated.Plugin }
				if updated.Target != "" { wf.Steps[i].Target = updated.Target }
				if updated.Runtime != "" { wf.Steps[i].Runtime = updated.Runtime }
				if updated.Script != "" { wf.Steps[i].Script = updated.Script }
				if updated.Server != "" { wf.Steps[i].Server = updated.Server }
				if updated.Mode != "" { wf.Steps[i].Mode = updated.Mode }
				if updated.EntryFunction != "" { wf.Steps[i].EntryFunction = updated.EntryFunction }
				if updated.Config != nil { wf.Steps[i].Config = updated.Config }
				if updated.DependsOn != nil { wf.Steps[i].DependsOn = updated.DependsOn }
				if updated.Type != "" { wf.Steps[i].Type = updated.Type }
				if updated.Inputs != nil { wf.Steps[i].Inputs = updated.Inputs }
				if updated.Outputs != nil { wf.Steps[i].Outputs = updated.Outputs }
				if updated.Condition != "" { wf.Steps[i].Condition = updated.Condition }
				if updated.LoopOver != "" { wf.Steps[i].LoopOver = updated.LoopOver }
				if updated.TrueBranch != "" { wf.Steps[i].TrueBranch = updated.TrueBranch }
				if updated.FalseBranch != "" { wf.Steps[i].FalseBranch = updated.FalseBranch }
				found = true
				break
			}
		}
		if !found { http.NotFound(w, r); return }
		cfg.Workflows[proj.Workflow] = wf
		if err := config.Save(cfg); err != nil { http.Error(w, "保存失败", 500); return }
		engine.GlobalRegistry.Clear()
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// API - 插件
	mux.HandleFunc("GET /api/plugins", func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		list := plugins.Discover(project)
		writeJSON(w, list)
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
		result, err := store.GetHistory(nil)
		if err != nil { http.Error(w, err.Error(), 500); return }
		writeJSON(w, result.Runs)
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

	err := runner.Run(context.Background(), runID, steps, order, dag, func(p *engine.Plugin) (map[string]any, error) {
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

		result, execErr := executor.ExecuteLocal(runtime, script, p.Mode, p.EntryFunc, params,
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
