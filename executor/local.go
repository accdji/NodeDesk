package executor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Result struct {
	Status string         `json:"status"`
	Data   map[string]any `json:"data"`
	Error  string         `json:"error"`
}

type LogCallback func(line string)

// ResolveScript 解析脚本路径：专属优先，通用兜底
// project 为空则只查 tasks/
func ResolveScript(project, script string) string {
	if filepath.IsAbs(script) {
		return script
	}

	// 1. 专属: projects/<project>/tasks/<script>
	if project != "" {
		local := filepath.Join("projects", project, "tasks", script)
		if _, err := os.Stat(local); err == nil {
			return local
		}
	}

	// 2. 通用: tasks/<script>
	shared := filepath.Join("tasks", script)
	if _, err := os.Stat(shared); err == nil {
		return shared
	}

	// 兜底返回通用路径（让子进程报明确的错误）
	return shared
}

// ExecuteLocal 子进程执行，stdin JSON → stdout 逐行读
func ExecuteLocal(runtime, script string, params map[string]any, onLog LogCallback) (*Result, error) {
	input := map[string]any{"params": params}
	inputJSON, _ := json.Marshal(input)

	cmd := exec.Command(runtime, script)
	cmd.Stdin = bytes.NewReader(inputJSON)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动失败 [%s %s]: %w", runtime, script, err)
	}

	var lastJSON string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if onLog != nil {
			onLog(line)
		}
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			lastJSON = line
		}
	}

	errScanner := bufio.NewScanner(stderr)
	for errScanner.Scan() {
		line := "[stderr] " + errScanner.Text()
		if onLog != nil {
			onLog(line)
		}
	}

	if err := cmd.Wait(); err != nil {
		return &Result{Status: "failed", Error: err.Error()}, nil
	}

	var result Result
	if lastJSON != "" {
		if json.Unmarshal([]byte(lastJSON), &result) != nil {
			result = Result{Status: "success", Data: map[string]any{"raw": lastJSON}}
		}
	} else {
		result = Result{Status: "success", Data: map[string]any{}}
	}
	return &result, nil
}
