package executor

import (
	"bufio"
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

// ResolveScript 解析脚本路径：项目专属插件目录 > 共享插件目录
func ResolveScript(project, script string) string {
	if filepath.IsAbs(script) {
		return script
	}
	if _, err := os.Stat(script); err == nil {
		return script
	}
	if project != "" {
		if found := findInPluginDir(filepath.Join("plugins", project), script); found != "" {
			return found
		}
	}
	if found := findInPluginDir(filepath.Join("plugins", "shared"), script); found != "" {
		return found
	}
	return script
}

func findInPluginDir(parentDir, script string) string {
	candidate := filepath.Join(parentDir, script)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if !strings.Contains(script, "/") && !strings.Contains(script, "\\") {
		nameNoExt := strings.TrimSuffix(script, filepath.Ext(script))
		entries, _ := os.ReadDir(parentDir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// 匹配插件目录名（去掉扩展名后对比，如 step_a.py → step_a）
			if e.Name() != nameNoExt && e.Name() != script {
				continue
			}
			// 优先从 plugin.json 读取入口
			mPath := filepath.Join(parentDir, e.Name(), "plugin.json")
			if data, err := os.ReadFile(mPath); err == nil {
				var m struct {
					Entry string `json:"entry"`
				}
				if json.Unmarshal(data, &m) == nil && m.Entry != "" {
					if found := filepath.Join(parentDir, e.Name(), m.Entry); fileExists(found) {
						return found
					}
				}
			}
			// fallback: 常见入口文件
			for _, entryName := range []string{"main.py", "main.sh", "main.js"} {
				if found := filepath.Join(parentDir, e.Name(), entryName); fileExists(found) {
					return found
				}
			}
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func PluginDir(script string) string {
	dir := filepath.Dir(script)
	if strings.Contains(filepath.ToSlash(dir), "/plugins/") {
		return dir
	}
	return ""
}

// ExecuteLocal 根据 mode 选择执行方式
func ExecuteLocal(runtime, script, mode, entryFunc string, params map[string]any, onLog LogCallback) (*Result, error) {
	if mode == "" {
		mode = "cli"
	}

	pluginDir := PluginDir(script)

	switch mode {
	case "function":
		return execFunction(runtime, script, pluginDir, entryFunc, params, onLog)
	default:
		return execCLI(runtime, script, pluginDir, params, onLog)
	}
}

// execFunction 函数模式：通过临时文件传参，import 模块调用函数获取返回值
func execFunction(runtime, script, pluginDir, entryFunc string, params map[string]any, onLog LogCallback) (*Result, error) {
	if entryFunc == "" {
		entryFunc = "run"
	}

	tmpDir, err := os.MkdirTemp("", "wf_func_*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	paramsFile := filepath.Join(tmpDir, "params.json")
	resultFile := filepath.Join(tmpDir, "result.json")

	// 写入参数文件
	writeJSON(paramsFile, params)

	bridgeCode := fmt.Sprintf(`import json, os, sys, importlib.util
sys.path.insert(0, %[1]q)
os.environ['PYTHONIOENCODING'] = 'utf-8'
os.environ['PYTHONUTF8'] = '1'
spec = importlib.util.spec_from_file_location('__plugin__', %[2]q)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
with open(%[3]q, 'r', encoding='utf-8') as f:
    _params = json.load(f)
_result = mod.%[4]s(_params)
with open(%[5]q, 'w', encoding='utf-8') as f:
    json.dump(_result, f, ensure_ascii=False, default=str)
`, pluginDir, script, paramsFile, entryFunc, resultFile)

	bridgeFile := filepath.Join(tmpDir, "bridge.py")
	writeFile(bridgeFile, bridgeCode)

	cmd := exec.Command(runtime, bridgeFile)
	cmd.Dir = pluginDir

	return runAndCapture(runtime, bridgeFile, cmd, tmpDir, resultFile, onLog)
}

// execCLI 命令行模式：inputs 转为 --name=value 参数，stdout=日志
func execCLI(runtime, script, pluginDir string, params map[string]any, onLog LogCallback) (*Result, error) {
	tmpDir, err := os.MkdirTemp("", "wf_cli_*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	resultFile := filepath.Join(tmpDir, "result.json")

	args := []string{script}
	for k, v := range params {
		// 跳过引擎自动注入的系统参数（避免传给用户脚本）
		if k == "project" || k == "project_id" || k == "work_dir" {
			continue
		}
		args = append(args, fmt.Sprintf("--%s=%v", k, v))
	}

	cmd := exec.Command(runtime, args...)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(),
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
		"OUTPUT_DIR="+tmpDir,
		"RESULT_FILE="+resultFile,
	)

	return runAndCapture(runtime, script, cmd, tmpDir, resultFile, onLog)
}

func runAndCapture(runtime, target string, cmd *exec.Cmd, workDir, resultFile string, onLog LogCallback) (*Result, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动失败 [%s %s]: %w", runtime, target, err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if onLog != nil {
			onLog(line)
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

	return readResult(resultFile)
}

func readResult(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// 没有结果文件也视为成功（CLI 模式可选）
		return &Result{Status: "success", Data: map[string]any{}}, nil
	}
	var r Result
	if jsonErr := json.Unmarshal(data, &r); jsonErr != nil {
		return nil, fmt.Errorf("解析结果 JSON 失败: %w\n内容: %s", jsonErr, string(data))
	}
	return &r, nil
}

func writeJSON(path string, v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	writeFile(path, string(data))
}

func writeFile(path, content string) {
	os.WriteFile(path, []byte(content), 0644)
}
