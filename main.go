package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"workflow/api"
	"workflow/config"
	"workflow/storage"
)

func main() {
	// 确定工作目录
	exe, _ := os.Executable()
	baseDir := filepath.Dir(exe)

	// 如果是 go run 方式启动，用当前目录
	if _, err := os.Stat(filepath.Join(baseDir, "config")); os.IsNotExist(err) {
		baseDir, _ = os.Getwd()
	}

	// 确保数据目录存在
	os.MkdirAll(filepath.Join(baseDir, "projects"), 0755)
	os.MkdirAll(filepath.Join(baseDir, "tasks"), 0755)

	// 加载配置：环境变量 CONFIG > 项目目录 pipeline.json > 示例配置
	configPath := os.Getenv("CONFIG")
	if configPath == "" {
		configPath = filepath.Join(baseDir, "config", "pipeline.example.json")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("警告: 加载配置失败 (%v)，使用空配置启动", err)
		cfg = &config.PipelineConfig{}
	}
	api.SetConfig(cfg)
	api.RegisterPlugins(cfg)

	// 加载国际化翻译
	lang := cfg.Lang
	if lang == "" {
		lang = "zh"
	}
	i18nPath := filepath.Join(baseDir, "i18n", lang+".json")
	i18nData, err := os.ReadFile(i18nPath)
	if err != nil {
		log.Printf("警告: 加载语言文件失败 (%s)，使用内建中文", i18nPath)
		lang = "zh"
		i18nData, _ = os.ReadFile(filepath.Join(baseDir, "i18n", "zh.json"))
	}
	translations := make(map[string]string)
	if i18nData != nil {
		json.Unmarshal(i18nData, &translations)
	}
	api.SetTranslations(lang, translations)
	log.Printf("配置已加载: %d 个工作流, %d 个项目, 语言=%s", len(cfg.Workflows), len(cfg.Projects), lang)

	// 初始化存储
	if err := storage.Init(baseDir); err != nil {
		log.Fatalf("存储初始化失败: %v", err)
	}

	// 加载模板（注册 toJSON 辅助函数用于注入 LANG）
	funcMap := template.FuncMap{
		"toJSON": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
			"tr": func(key string) string {
				if v, ok := translations[key]; ok {
					return v
				}
				return key
			},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(filepath.Join(baseDir, "web", "templates", "*.html"))
	if err != nil {
		log.Fatalf("模板加载失败: %v", err)
	}

	// 设置路由
	mux := http.NewServeMux()
	api.SetupRoutes(mux, tmpl)

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	log.Printf("⚡ 工作流编排器已启动: http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}
