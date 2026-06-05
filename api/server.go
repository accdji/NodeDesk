package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"strconv"
	"strings"
	"time"

	"workflow/config"
	"workflow/engine"
	"workflow/executor"
	"workflow/pkg/mapper"
	"workflow/plugins"
	"workflow/storage"
)

type sseClient struct {
	ch    chan string
	runID string
}

var (
	sseClients  = make(map[string][]*sseClient)
	sseMu       sync.Mutex

	runCancels  = make(map[string]context.CancelFunc)
	runCancelMu sync.Mutex

	currentCfg  *config.PipelineConfig
	cfgMu       sync.RWMutex

	translations map[string]string
	lang         string
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

func RegisterPlugins(cfg *config.PipelineConfig) {
	for _, wf := range cfg.Workflows {
		nameCounter := make(map[string]int)
		for _, step := range wf.Steps {
			regName := step.Plugin
			if count := nameCounter[step.Plugin]; count > 0 {
				regName = fmt.Sprintf("%s#%d", step.Plugin, count)
			}
			nameCounter[step.Plugin]++
			p := engine.GlobalRegistry.Register(regName, step.Plugin, step.DependsOn)
			p.Target = step.Target
				p.Server = step.Server
				p.Runtime = step.Runtime
			p.Script = step.Script
			p.Config = step.Config
			p.Type = step.Type
			if p.Type == "" { p.Type = "script" }
			for _, in := range step.Inputs {
				p.Inputs = append(p.Inputs, engine.ParamDef{Name: in.Name, Type: in.Type, Desc: in.Desc, Required: in.Required})
			}
			for _, out := range step.Outputs {
				p.Outputs = append(p.Outputs, engine.ParamDef{Name: out.Name, Type: out.Type, Desc: out.Desc, Required: out.Required})
			}
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
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func SetupRoutes(mux *http.ServeMux) {

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

	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.Error(w, "not loaded", 400); return }
		var req struct {
			Name     string
			ID       string
			Workflow string
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
			http.Error(w, "missing name", 400); return
		}
		for _, p := range cfg.Projects {
			if p.Name == req.Name { http.Error(w, "exists", 409); return }
		}
		if req.ID == "" { req.ID = req.Name }
		if req.Workflow == "" { req.Workflow = "standard" }
		cfg.Projects = append(cfg.Projects, config.ProjectDef{
			Name: req.Name, ProjectID: req.ID, Workflow: req.Workflow, Enabled: true,
		})
		if err := config.Save(cfg); err != nil {
			http.Error(w, "save: "+err.Error(), 500); return
		}
		SetConfig(cfg)
		RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.Error(w, "not loaded", 500); return }
		writeJSON(w, map[string]any{"path": config.GetPath(), "config": cfg})
	})

	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var cfg config.PipelineConfig
		if json.NewDecoder(r.Body).Decode(&cfg) != nil {
			http.Error(w, "bad json", 400); return
		}
		if err := config.Save(&cfg); err != nil {
			http.Error(w, "save: "+err.Error(), 500); return
		}
		engine.GlobalRegistry.Clear()
		SetConfig(&cfg)
		RegisterPlugins(&cfg)
		log.Printf("config reloaded: %d workflows, %d projects", len(cfg.Workflows), len(cfg.Projects))
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("PATCH /api/projects/{name}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		var req struct{ Enabled *bool }
		json.NewDecoder(r.Body).Decode(&req)
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name {
				if req.Enabled != nil { cfg.Projects[i].Enabled = *req.Enabled }
				config.Save(cfg); SetConfig(cfg); RegisterPlugins(cfg)
				writeJSON(w, map[string]string{"status": "ok"}); return
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
				config.Save(cfg); SetConfig(cfg); RegisterPlugins(cfg)
				writeJSON(w, map[string]string{"status": "ok"}); return
			}
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("POST /api/workflows", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.Error(w, "not loaded", 500); return }
		var req struct{ Name string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" { http.Error(w, "missing name", 400); return }
		if _, ok := cfg.Workflows[req.Name]; ok { http.Error(w, "exists", 409); return }
		cfg.Workflows[req.Name] = config.WorkflowDef{Label: req.Name, Steps: []config.StepDef{}}
		config.Save(cfg); SetConfig(cfg); RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/workflows/{name}/clone", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		src, ok := cfg.Workflows[name]
		if !ok { http.NotFound(w, r); return }
		var req struct{ Name string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" { req.Name = name + "_copy" }
		if _, ok := cfg.Workflows[req.Name]; ok { http.Error(w, "exists", 409); return }
		steps := make([]config.StepDef, len(src.Steps))
		copy(steps, src.Steps)
		cfg.Workflows[req.Name] = config.WorkflowDef{Label: req.Name, Steps: steps}
		config.Save(cfg); SetConfig(cfg); RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

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
		if !ok { http.Error(w, "wf not found", 404); return }
		var step config.StepDef
		if json.NewDecoder(r.Body).Decode(&step) != nil || step.Plugin == "" {
			http.Error(w, "missing plugin", 400); return
		}
		if step.Runtime == "" { step.Runtime = "python" }
		if step.Script == "" { step.Script = step.Plugin + ".py" }
		wf.Steps = append(wf.Steps, step)
		cfg.Workflows[proj.Workflow] = wf
		if err := config.Save(cfg); err != nil { http.Error(w, "save fail", 500); return }
		engine.GlobalRegistry.Clear(); SetConfig(cfg); RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("DELETE /api/projects/{name}/steps/{stepName}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		stepName := r.PathValue("stepName")
		var proj *config.ProjectDef
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name { proj = &cfg.Projects[i]; break }
		}
		if proj == nil { http.NotFound(w, r); return }
		wf, ok := cfg.Workflows[proj.Workflow]
		if !ok { http.Error(w, "wf not found", 404); return }
		idx := -1
		for i, s := range wf.Steps {
			if s.Plugin == stepName { idx = i; break }
		}
		if idx < 0 {
			http.NotFound(w, r)
			return
		}
		wf.Steps = append(wf.Steps[:idx], wf.Steps[idx+1:]...)
		cfg.Workflows[proj.Workflow] = wf
		if err := config.Save(cfg); err != nil { http.Error(w, "save fail", 500); return }
		engine.GlobalRegistry.Clear(); SetConfig(cfg); RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("PUT /api/projects/{name}/steps/{stepName}", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { http.NotFound(w, r); return }
		name := r.PathValue("name")
		stepName := r.PathValue("stepName")
		var proj *config.ProjectDef
		for i := range cfg.Projects {
			if cfg.Projects[i].Name == name { proj = &cfg.Projects[i]; break }
		}
		if proj == nil { http.NotFound(w, r); return }
		wf, ok := cfg.Workflows[proj.Workflow]
		if !ok { http.Error(w, "wf not found", 404); return }
		var updated config.StepDef
		if json.NewDecoder(r.Body).Decode(&updated) != nil { http.Error(w, "bad json", 400); return }
		idx := -1
		for i, s := range wf.Steps {
			if s.Plugin == stepName { idx = i; break }
		}
		if idx < 0 {
			http.NotFound(w, r)
			return
		}
		i := idx
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

		cfg.Workflows[proj.Workflow] = wf
		if err := config.Save(cfg); err != nil { http.Error(w, "save fail", 500); return }
		engine.GlobalRegistry.Clear(); SetConfig(cfg); RegisterPlugins(cfg)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/plugins", func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		list := plugins.Discover(project)
		writeJSON(w, list)
	})

	mux.HandleFunc("GET /api/workflows", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { writeJSON(w, []string{}); return }
		wfs := make([]map[string]any, 0)
		for name, wf := range cfg.Workflows {
			names := make([]string, len(wf.Steps))
			for i, s := range wf.Steps { names[i] = s.Plugin }
			wfs = append(wfs, map[string]any{"name": name, "label": wf.Label, "steps": names})
		}
		writeJSON(w, wfs)
	})

	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if cfg == nil { writeJSON(w, []string{}); return }
		store := storage.GetStore()
		projs := make([]map[string]any, 0)
		for _, proj := range cfg.Projects {
			p := map[string]any{
				"name": proj.Name, "project_id": proj.ProjectID,
				"workflow": proj.Workflow, "enabled": proj.Enabled,
			}
			if store != nil {
				if latest := store.GetLatestRun(proj.Name); latest != nil {
					p["last_run"] = map[string]any{
						"status": latest.Status,
						"created_at": latest.CreatedAt,
						"finished_at": latest.FinishedAt,
					}
				}
			}
			projs = append(projs, p)
		}
		writeJSON(w, projs)
	})

	mux.HandleFunc("POST /api/run/{project}", handleRun)

	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { writeJSON(w, map[string]any{"runs": []any{}, "total": 0, "page": 1, "size": 20}); return }
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 { page = 1 }
		size, _ := strconv.Atoi(q.Get("size"))
		if size < 1 { size = 20 }
		filter := &storage.HistoryFilter{
			Status:  q.Get("status"),
			Project: q.Get("project"),
			Search:  q.Get("q"),
			Page:    page,
			Size:    size,
		}
		result, err := store.GetHistory(filter)
		if err != nil { http.Error(w, err.Error(), 500); return }
		writeJSON(w, result)
	})

	mux.HandleFunc("DELETE /api/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { http.NotFound(w, r); return }
		id := r.PathValue("id")
		if err := store.DeleteRun(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("DELETE /api/history/batch", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { http.Error(w, "no store", 500); return }
		var req struct{ IDs []string `json:"ids"` }
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad json", 400); return
		}
		if err := store.DeleteRuns(req.IDs); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/history/{id}/logs/download", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { http.NotFound(w, r); return }
		doc, err := store.GetRunDetail(r.PathValue("id"))
		if err != nil { http.NotFound(w, r); return }
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=run-"+r.PathValue("id")+".log")
		for _, log := range doc.Logs {
			fmt.Fprintf(w, "[%s] [%s] %s\n", log.Timestamp, log.StepName, log.Message)
		}
	})

	mux.HandleFunc("GET /api/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		store := storage.GetStore()
		if store == nil { http.NotFound(w, r); return }
		doc, err := store.GetRunDetail(r.PathValue("id"))
		if err != nil { http.NotFound(w, r); return }
		writeJSON(w, doc)
	})

	mux.HandleFunc("GET /api/sse/{id}", handleSSE)
	mux.HandleFunc("POST /api/cancel/{id}", handleCancel)
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	dryRun := r.URL.Query().Get("dry") == "1"
	fromStep := r.URL.Query().Get("from")

	cfg := getConfig()
	if cfg == nil { http.Error(w, "not loaded", 400); return }

	var proj *config.ProjectDef
	for i := range cfg.Projects {
		if cfg.Projects[i].Name == projectName { proj = &cfg.Projects[i]; break }
	}
	if proj == nil { http.Error(w, "not found: "+projectName, 404); return }

	wf, ok := cfg.Workflows[proj.Workflow]
	if !ok { http.Error(w, "wf not found: "+proj.Workflow, 404); return }

	nameCounter := make(map[string]int)
	steps := make([]*engine.Plugin, len(wf.Steps))
	for i, s := range wf.Steps {
		stepID := s.Plugin
		if count := nameCounter[s.Plugin]; count > 0 {
			stepID = fmt.Sprintf("%s#%d", s.Plugin, count)
		}
		nameCounter[s.Plugin]++
		p, ok := engine.GlobalRegistry.Get(stepID)
		if !ok {
			p = engine.GlobalRegistry.Register(stepID, s.Plugin, s.DependsOn)
			p.Target = s.Target; p.Server = s.Server; p.Runtime = s.Runtime; p.Script = s.Script; p.Config = s.Config
		}
		step := *p; step.StepID = stepID; steps[i] = &step
	}

	dag := engine.BuildDAG(steps)
	order, err := engine.TopologicalSort(steps, dag)
	if err != nil { http.Error(w, err.Error(), 400); return }

	runID := genID()
	if dryRun {
		writeJSON(w, map[string]any{"plan": formatPlan(order, steps), "run_id": runID, "dry_run": true})
		return
	}

	store := storage.GetStore()
	if store != nil {
		store.SaveRun(storage.RunRecord{
			ID: runID, ProjectName: projectName, WorkflowName: proj.Workflow,
			Status: "running", CreatedAt: time.Now(),
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		timer := time.NewTimer(2 * time.Hour)
		defer timer.Stop()
		select {
		case <-timer.C: cancel()
		case <-ctx.Done():
		}
	}()
	runCancelMu.Lock()
	runCancels[runID] = cancel
	runCancelMu.Unlock()

	go executeWorkflow(ctx, runID, projectName, proj, wf, steps, order, dag, fromStep)
	writeJSON(w, map[string]string{"run_id": runID, "status": "started"})
}

func executeWorkflow(ctx context.Context, runID, projectName string, proj *config.ProjectDef, wf config.WorkflowDef,
	steps []*engine.Plugin, order []string, dag engine.DAG, fromStep string) {
	defer func() {
		if r := recover(); r != nil { log.Printf("panic [%s]: %v", runID, r) }
	}()

	store := storage.GetStore()
	runner := engine.NewRunner()
	runner.StepStartFunc = func(rid, step, target string) {
		data, _ := json.Marshal(map[string]string{"type": "step_start", "run_id": rid, "step": step, "target": target})
		broadcastSSEEventRaw(rid, string(data))
	}
	runner.StepEndFunc = func(rid, step, state string, duration float64, errMsg string) {
			if store != nil {
				store.UpdateStepDuration(rid, step, duration)
			}
			data, _ := json.Marshal(map[string]any{
				"type": "step_end", "run_id": rid, "step": step,
				"state": state, "duration": duration, "error": errMsg,
			})
			broadcastSSEEventRaw(rid, string(data))
		}
		runner.LogFunc = func(rid, step, msg string) {
		ts := time.Now().Format("15:04:05")
		if store != nil {
			store.AppendLog(storage.LogEntry{RunID: rid, StepName: step, Timestamp: ts, Level: "INFO", Message: msg})
		}
		broadcastSSE(rid, step, ts, msg)
	}

	err := runner.Run(ctx, runID, steps, order, dag, func(p *engine.Plugin) (map[string]any, error) {
		stepName := p.StepID
		if stepName == "" { stepName = p.Name }
		if store != nil {
			store.SaveStep(storage.StepRecord{RunID: runID, StepName: stepName, State: string(engine.StateRunning), Target: p.Target})
		}
		// mapper 类型：直接执行字段映射，无需脚本
		if p.Type == "mapper" {
			inputData := map[string]any{"project": projectName, "project_id": proj.ProjectID}
			for _, dep := range p.DependsOn {
				if depRes, ok := runner.GetResults()[dep]; ok && depRes.State == engine.StateSuccess {
					inputData[dep] = depRes.Data
				}
			}
			if p.Config != nil {
				for k, v := range p.Config { inputData[k] = resolveConfigValue(v, runner.GetResults()) }
			}
			var rules []mapper.Rule
			if rawRules, ok := p.Config["mappings"]; ok {
				if rulesJSON, err := json.Marshal(rawRules); err == nil {
					json.Unmarshal(rulesJSON, &rules)
				}
			}
			mapped := mapper.Apply(inputData, rules)
			return mapped, nil
		}

		runtime := p.Runtime
		if runtime == "" { runtime = "python" }
		script := p.Script
		if script == "" { script = p.Name + ".py" }
		script = executor.ResolveScript(projectName, script)
		params := map[string]any{"project": projectName, "project_id": proj.ProjectID, "work_dir": "."}
		if p.Config != nil {
			for k, v := range p.Config { params[k] = resolveConfigValue(v, runner.GetResults()) }
		}
		onLog := func(line string) {
				ts := time.Now().Format("15:04:05")
				if store != nil {
					store.AppendLog(storage.LogEntry{RunID: runID, StepName: stepName, Timestamp: ts, Level: "INFO", Message: line})
				}
				broadcastSSE(runID, stepName, ts, line)
			}

			var result *executor.Result
			var execErr error

			// 根据 target/server 决定本地执行还是远程 SSH 执行
			serverCfg, isRemote := resolveServer(p, getConfig())
			if isRemote {
				result, execErr = executor.ExecuteRemote(serverCfg, runtime, script, p.Mode, p.EntryFunc, params, onLog)
			} else {
				result, execErr = executor.ExecuteLocal(runtime, script, p.Mode, p.EntryFunc, params, onLog)
			}
		if execErr != nil {
			if store != nil {
				store.SaveStep(storage.StepRecord{RunID: runID, StepName: stepName, State: string(engine.StateFailed), Error: execErr.Error(), Target: p.Target})
			}
			return nil, execErr
		}
		dataJSON, _ := json.Marshal(result.Data)
		if store != nil {
			store.SaveStep(storage.StepRecord{RunID: runID, StepName: stepName, State: string(engine.StateSuccess), Data: string(dataJSON), Target: p.Target})
		}
		return result.Data, nil
	}, false, fromStep)

	status := "success"
	if err != nil { status = "failed" }
	if store != nil { store.UpdateRunStatus(runID, status) }
	runCancelMu.Lock()
	delete(runCancels, runID)
	runCancelMu.Unlock()
	flushRunLogs(runID)
	broadcastSSEEvent(runID, "completed", status)
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	runCancelMu.Lock()
	cancel, ok := runCancels[runID]
	runCancelMu.Unlock()
	if !ok { http.NotFound(w, r); return }
	cancel()
	store := storage.GetStore()
	if store != nil { store.UpdateRunStatus(runID, "cancelled") }
	flushRunLogs(runID)
	broadcastSSEEvent(runID, "completed", "cancelled")
	log.Printf("cancelled [%s]", runID)
	writeJSON(w, map[string]string{"status": "cancelled"})
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	flusher, ok := w.(http.Flusher)
	if !ok { http.Error(w, "no flush", 500); return }
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
			if len(sseClients[runID]) == 0 {
				runCancelMu.Lock()
				if cancel, ok := runCancels[runID]; ok { cancel(); delete(runCancels, runID) }
				runCancelMu.Unlock()
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
	// 批量缓冲：累积到 LogBuffer，定时或满 10 条后批量推送
	if len(message) > 500 {
		// 长消息直接推送，不进缓冲
		sendSingleSSE(runID, stepName, timestamp, message)
		return
	}
	lb := getOrCreateLogBuffer(runID, func(lines []logLine) {
		batchBroadcastSSE(lines)
	})
	lb.Add(runID, stepName, timestamp, message)
}

func sendSingleSSE(runID, stepName, timestamp, message string) {
	sseMu.Lock()
	defer sseMu.Unlock()
	msg := map[string]string{"type": "log", "run_id": runID, "step_name": stepName, "timestamp": timestamp, "message": message}
	data, _ := json.Marshal(msg)
	for _, c := range sseClients[runID] {
		select {
		case c.ch <- string(data):
		default:
		}
	}
}

func flushRunLogs(runID string) {
	removeLogBuffer(runID)
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

func broadcastSSEEventRaw(runID, rawJSON string) {
	sseMu.Lock()
	defer sseMu.Unlock()
	for _, c := range sseClients[runID] {
		select {
		case c.ch <- rawJSON:
		default:
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// resolveConfigValue 递归解析配置值中的 $.step_name.field 引用
func resolveConfigValue(v any, results map[string]*engine.StepResult) any {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "$.") {
			resolved, err := engine.ResolveRef(val, results)
			if err == nil {
				return resolved
			}
		}
		return val
	case map[string]any:
		out := make(map[string]any)
		for mk, mv := range val {
			out[mk] = resolveConfigValue(mv, results)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = resolveConfigValue(item, results)
		}
		return out
	default:
		return v
	}
}

// resolveServer 根据 step 的 target/server 字段查找远程服务器配置
// 返回 (ServerConfig, true) 表示需要远程执行，(ServerConfig{}, false) 表示本地执行
func resolveServer(p *engine.Plugin, cfg *config.PipelineConfig) (config.ServerConfig, bool) {
	if cfg == nil || cfg.Servers == nil {
		return config.ServerConfig{}, false
	}
	serverName := p.Server
	if serverName == "" {
		serverName = p.Target
	}
	if serverName == "" || serverName == "local" {
		return config.ServerConfig{}, false
	}
	srv, ok := cfg.Servers[serverName]
	if !ok {
		return config.ServerConfig{}, false
	}
	return srv, true
}

func formatPlan(order []string, steps []*engine.Plugin) []map[string]any {
	pmap := make(map[string]*engine.Plugin)
	for _, p := range steps {
		key := p.StepID
		if key == "" { key = p.Name }
		pmap[key] = p
	}
	result := make([]map[string]any, 0)
	for i, name := range order {
		p := pmap[name]
		result = append(result, map[string]any{
			"index": i + 1, "plugin": name, "target": p.Target, "depends_on": p.DependsOn,
		})
	}
	return result
}
