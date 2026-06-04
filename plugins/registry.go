package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PluginManifest 描述 plugin.json 清单文件
type PluginManifest struct {
	Name          string     `json:"name"`
	Label         string     `json:"label"`
	Description   string     `json:"description"`
	Version       string     `json:"version"`
	Runtime       string     `json:"runtime"`
	Entry         string     `json:"entry"`
	Mode          string     `json:"mode"`          // "function" 或 "cli"
	EntryFunction string     `json:"entry_function"` // 函数模式入口函数名
	Inputs        []ParamDef `json:"inputs"`
	Outputs       []ParamDef `json:"outputs"`
}

// ParamDef 参数定义
type ParamDef struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "string", "number", "boolean", "object", "array"
	Desc     string `json:"desc"`
	Required bool   `json:"required"`
}

// PluginInfo 描述一个可用的插件
type PluginInfo struct {
	Name          string     `json:"name"`
	Label         string     `json:"label"`
	Description   string     `json:"description"`
	Version       string     `json:"version"`
	Runtime       string     `json:"runtime"`
	Script        string     `json:"script"` // 入口文件路径
	Source        string     `json:"source"` // "shared" 或 "project"
	Mode          string     `json:"mode"`          // "function" 或 "cli"
	EntryFunction string     `json:"entry_function"` // 函数模式入口函数名
	Inputs        []ParamDef `json:"inputs"`
	Outputs       []ParamDef `json:"outputs"`
	PluginDir     string     `json:"-"` // 插件目录绝对路径（内部使用）
}

// 脚本扩展名 → 运行时映射
var extToRuntime = map[string]string{
	".py": "python",
	".sh": "shell",
	".js": "node",
}

// Discover 返回所有可用插件：共享 + 项目专属
func Discover(projectName string) []PluginInfo {
	var all []PluginInfo

	// 1. 共享插件 plugins/shared/<name>/
	all = append(all, scanPluginDirs(filepath.Join("plugins", "shared"), "shared")...)

	// 2. 项目专属插件 plugins/<project-name>/
	if projectName != "" {
		projPath := filepath.Join("plugins", projectName)
		all = append(all, scanPluginDirs(projPath, "project")...)
	}

	return all
}

// scanPluginDirs 扫描插件目录，每个子目录有 plugin.json 即视为一个插件
func scanPluginDirs(baseDir, source string) []PluginInfo {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil
	}
	var plugins []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		pluginDir := filepath.Join(baseDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // 没有 plugin.json 则跳过
		}
		var m PluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.Name == "" {
			m.Name = entry.Name()
		}
		if m.Label == "" {
			m.Label = m.Name
		}
		if m.Runtime == "" {
			m.Runtime = inferRuntime(m.Entry)
		}
		if m.Entry == "" {
			// 自动找入口：main.py > main.sh > main.js > 第一个匹配的脚本
			m.Entry = findEntry(pluginDir)
		}
		// 脚本相对路径（相对于工作目录）
		relScript := filepath.ToSlash(filepath.Join(pluginDir, m.Entry))

		if m.Mode == "" {
			m.Mode = "cli"
		}
		plugins = append(plugins, PluginInfo{
			Name:          m.Name,
			Label:         m.Label,
			Description:   m.Description,
			Version:       m.Version,
			Runtime:       m.Runtime,
			Script:        relScript,
			Source:        source,
			Mode:          m.Mode,
			EntryFunction: m.EntryFunction,
			Inputs:        m.Inputs,
			Outputs:       m.Outputs,
			PluginDir:     pluginDir,
		})
	}
	return plugins
}

// findEntry 在插件目录中自动寻找入口文件
func findEntry(dir string) string {
	// 优先常见的入口文件名
	candidates := []string{"main.py", "main.sh", "main.js", "index.py", "index.sh", "index.js", "run.py", "run.sh"}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(dir, c)); err == nil {
			return c
		}
	}
	// 兜底：找第一个 .py/.sh/.js
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if _, ok := extToRuntime[ext]; ok {
			return e.Name()
		}
	}
	return "main.py"
}

// inferRuntime 从入口文件扩展名推断运行时
func inferRuntime(entry string) string {
	ext := filepath.Ext(entry)
	if rt, ok := extToRuntime[ext]; ok {
		return rt
	}
	return "python"
}

