package executor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"workflow/config"
)

// ExecuteRemote 通过 SSH+SFTP 在远程服务器上执行脚本
// 只用 SFTP 传文件，只用 SSH 执行脚本本身，不执行任何 shell 命令
func ExecuteRemote(server config.ServerConfig, runtime, script, mode, entryFunc string, params map[string]any, onLog LogCallback) (*Result, error) {
	if mode == "" {
		mode = "cli"
	}
	if runtime == "" {
		runtime = server.Python
		if runtime == "" {
			runtime = "python3"
		}
	}

	client, err := sshConnect(server)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %w", err)
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("SFTP连接失败: %w", err)
	}
	defer sftpClient.Close()

	// 读取本地脚本
	scriptData, err := os.ReadFile(script)
	if err != nil {
		return nil, fmt.Errorf("读取本地脚本失败: %w", err)
	}

	// 远程工作目录：RemoteBase + 随机子目录
	remoteBase := server.RemoteBase
	if remoteBase == "" {
		remoteBase = "/tmp"
	}
	tmpDir := fmt.Sprintf("%s/wf_%x", remoteBase, rand.Uint64())
	if err := sftpClient.MkdirAll(tmpDir); err != nil {
		return nil, fmt.Errorf("创建远程目录失败: %w", err)
	}
	defer sftpRemoveAll(sftpClient, tmpDir)

	// SFTP 上传脚本文件
	remoteScript := tmpDir + "/" + filepath.Base(script)
	if err := sftpWrite(sftpClient, remoteScript, scriptData); err != nil {
		return nil, fmt.Errorf("上传脚本失败: %w", err)
	}

	switch mode {
	case "function":
		return execRemoteFunc(client, sftpClient, runtime, remoteScript, tmpDir, entryFunc, params, onLog)
	default:
		return execRemoteCLI(client, sftpClient, runtime, remoteScript, tmpDir, params, onLog)
	}
}

func sshConnect(server config.ServerConfig) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            server.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(server.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	port := server.Port
	if port == 0 {
		port = 22
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", server.Host, port), cfg)
}

// sftpWrite 通过 SFTP 写文件
func sftpWrite(sc *sftp.Client, path string, data []byte) error {
	f, err := sc.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// sftpRead 通过 SFTP 读文件
func sftpRead(sc *sftp.Client, path string) ([]byte, error) {
	f, err := sc.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// sftpRemoveAll 递归删除远程目录
func sftpRemoveAll(sc *sftp.Client, path string) {
	entries, err := sc.ReadDir(path)
	if err != nil {
		sc.Remove(path)
		return
	}
	for _, e := range entries {
		p := path + "/" + e.Name()
		if e.IsDir() {
			sftpRemoveAll(sc, p)
		} else {
			sc.Remove(p)
		}
	}
	sc.Remove(path)
}

func execRemoteFunc(client *ssh.Client, sftpClient *sftp.Client, runtime, script, tmpDir, entryFunc string, params map[string]any, onLog LogCallback) (*Result, error) {
	if entryFunc == "" {
		entryFunc = "run"
	}

	paramsFile := tmpDir + "/params.json"
	resultFile := tmpDir + "/result.json"

	// SFTP 上传 params.json
	paramsJSON, _ := json.Marshal(params)
	if err := sftpWrite(sftpClient, paramsFile, paramsJSON); err != nil {
		return nil, fmt.Errorf("上传参数文件失败: %w", err)
	}

	// SFTP 上传 bridge.py
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
`, tmpDir, script, paramsFile, entryFunc, resultFile)

	bridgeFile := tmpDir + "/bridge.py"
	if err := sftpWrite(sftpClient, bridgeFile, []byte(bridgeCode)); err != nil {
		return nil, fmt.Errorf("上传bridge脚本失败: %w", err)
	}

	// 只执行一条命令：运行 bridge.py
	cmd := fmt.Sprintf("%s %s", runtime, bridgeFile)
	return sshRunAndCapture(client, sftpClient, cmd, resultFile, onLog)
}

func execRemoteCLI(client *ssh.Client, sftpClient *sftp.Client, runtime, script, tmpDir string, params map[string]any, onLog LogCallback) (*Result, error) {
	resultFile := tmpDir + "/result.json"

	args := []string{script}
	for k, v := range params {
		if k == "project" || k == "project_id" || k == "work_dir" {
			continue
		}
		args = append(args, fmt.Sprintf("--%s=%v", k, v))
	}

	cmd := fmt.Sprintf("%s %s", runtime, strings.Join(args, " "))
	return sshRunAndCapture(client, sftpClient, cmd, resultFile, onLog)
}

// sshRunAndCapture 通过 SSH 执行脚本命令，等待完成后通过 SFTP 下载结果文件
func sshRunAndCapture(client *ssh.Client, sftpClient *sftp.Client, cmd, resultFile string, onLog LogCallback) (*Result, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建SSH会话失败: %w", err)
	}
	defer session.Close()

	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	if err := session.Start(cmd); err != nil {
		return nil, fmt.Errorf("启动远程脚本失败: %w", err)
	}

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

	if err := session.Wait(); err != nil {
		return &Result{Status: "failed", Error: err.Error()}, nil
	}

	// SFTP 下载结果文件
	resultData, err := sftpRead(sftpClient, resultFile)
	if err != nil || len(resultData) == 0 {
		return &Result{Status: "success", Data: map[string]any{}}, nil
	}

	var r Result
	if jsonErr := json.Unmarshal(resultData, &r); jsonErr != nil {
		return nil, fmt.Errorf("解析远程结果JSON失败: %w\n内容: %s", jsonErr, string(resultData))
	}
	return &r, nil
}
