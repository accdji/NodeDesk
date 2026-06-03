package config

import (
	"encoding/json"
	"os"
	"strings"
)

type ServerConfig struct {
	Host       string            `json:"host"`
	Port       int               `json:"port"`
	Username   string            `json:"username"`
	Password   string            `json:"password"`
	RemoteBase string            `json:"remote_base"`
	Python     string            `json:"python"`
	Paths      map[string]string `json:"paths"`
}

type StepDef struct {
	Plugin    string         `json:"plugin"`
	Target    string         `json:"target"`
	Runtime   string         `json:"runtime"`
	Script    string         `json:"script"`
	Server    string         `json:"server"`
	Config    map[string]any `json:"config"`
	DependsOn []string       `json:"depends_on,omitempty"`
}

type WorkflowDef struct {
	Label string    `json:"label"`
	Steps []StepDef `json:"steps"`
}

type ProjectDef struct {
	Name         string `json:"name"`
	ProjectID    string `json:"project_id"`
	Workflow     string `json:"workflow"`
	ZipSourceDir string `json:"zip_source_dir"`
	Server       string `json:"server"`
	Enabled      bool   `json:"enabled"`
}

type PipelineConfig struct {
	Servers   map[string]ServerConfig `json:"servers"`
	Workflows map[string]WorkflowDef  `json:"workflows"`
	Projects  []ProjectDef            `json:"projects"`
	Global    map[string]string       `json:"global"`
}

var currentPath string

// Load 加载 JSON 配置文件，替换 ${VAR} 占位符
func Load(path string) (*PipelineConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	currentPath = path

	content := interpolateEnv(string(data))

	var cfg PipelineConfig
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// GetPath 返回当前配置文件路径
func GetPath() string { return currentPath }

// Save 保存配置到文件
func Save(cfg *PipelineConfig) error {
	if currentPath == "" {
		return os.ErrNotExist
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(currentPath, data, 0644)
}

func interpolateEnv(content string) string {
	envPath := os.ExpandEnv("${USERPROFILE}\\.workflow\\.env")
	if envFile, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(envFile), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			k, v := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			content = strings.ReplaceAll(content, "${"+k+"}", v)
		}
	}
	return content
}
