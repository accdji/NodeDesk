package executor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Result struct {
	Status string         `json:"status"`
	Data   map[string]any `json:"data"`
	Error  string         `json:"error"`
}

type LogCallback func(line string)

// ResolveScript 瑙ｆ瀽鑴氭湰璺緞锛氶」鐩笓灞炴彃浠剁洰褰?> 鍏变韩鎻掍欢鐩綍
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
			// 鍖归厤鎻掍欢鐩綍鍚嶏紙鍘绘帀鎵╁睍鍚嶅悗瀵规瘮锛屽 step_a.py 鈫?step_a锛?
						if e.Name() != nameNoExt && e.Name() != script {
				continue
			}
			// 浼樺厛浠?plugin.json 璇诲彇鍏ュ彛
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
			// fallback: 甯歌鍏ュ彛鏂囦欢
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

// ExecuteLocal 鏍规嵁 mode 閫夋嫨鎵ц鏂瑰紡
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

// execFunction 鍑芥暟妯″紡锛氶€氳繃涓存椂鏂囦欢浼犲弬锛宨mport 妯″潡璋冪敤鍑芥暟鑾峰彇杩斿洖
func execFunction(runtime, script, pluginDir, entryFunc string, params map[string]any, onLog LogCallback) (*Result, error) {
	if entryFunc == "" {
		entryFunc = "run"
	}

	tmpDir, err := os.MkdirTemp("", "wf_func_*")
	if err != nil {
		return nil, fmt.Errorf("鍒涘缓涓存椂鐩綍澶辫触: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	paramsFile := filepath.Join(tmpDir, "params.json")
	resultFile := filepath.Join(tmpDir, "result.json")

	// 鍐欏叆鍙傛暟鏂囦欢
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

// execCLI 鍛戒护琛屾ā寮忥細inputs 杞负 --name=value 鍙傛暟锛宻tdout=鏃ュ織
func execCLI(runtime, script, pluginDir string, params map[string]any, onLog LogCallback) (*Result, error) {
	tmpDir, err := os.MkdirTemp("", "wf_cli_*")
	if err != nil {
		return nil, fmt.Errorf("鍒涘缓涓存椂鐩綍澶辫触: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	resultFile := filepath.Join(tmpDir, "result.json")

	args := []string{script}
	for k, v := range params {
		// 璺宠繃寮曟搸鑷姩娉ㄥ叆鐨勭郴缁熷弬鏁帮紙閬垮厤浼犵粰鐢ㄦ埛鑴氭湰锛?
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
		return nil, fmt.Errorf("鍚姩澶辫触 [%s %s]: %w", runtime, target, err)
	}

	// 并发读取 stdout 和 stderr，防止管道满导致子进程死锁
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			if onLog != nil {
				onLog(scanner.Text())
			}
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if onLog != nil {
				onLog("[stderr] " + scanner.Text())
			}
		}
	}()

	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return &Result{Status: "failed", Error: err.Error()}, nil
	}

	return readResult(resultFile)
}

func readResult(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// 娌℃湁缁撴灉鏂囦欢涔熻涓烘垚鍔燂紙CLI 妯″紡鍙€夛級
		return &Result{Status: "success", Data: map[string]any{}}, nil
	}
	var r Result
	if jsonErr := json.Unmarshal(data, &r); jsonErr != nil {
		return nil, fmt.Errorf("瑙ｆ瀽缁撴灉 JSON 澶辫触: %w\n鍐呭: %s", jsonErr, string(data))
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

