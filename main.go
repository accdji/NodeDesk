package main

import (
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
	if _, err := os.Stat(filepath.Join(baseDir, "projects")); os.IsNotExist(err) {
		baseDir, _ = os.Getwd()
	}

	// 加载配置
	configPath := filepath.Join(baseDir, "projects", "deepway", "pipeline.json")
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	api.SetConfig(cfg)
	api.RegisterPlugins(cfg)
	log.Printf("配置已加载: %d 个工作流, %d 个项目", len(cfg.Workflows), len(cfg.Projects))

	// 初始化存储
	if err := storage.Init(baseDir); err != nil {
		log.Fatalf("存储初始化失败: %v", err)
	}

	// 加载模板
	tmpl, err := template.ParseGlob(filepath.Join(baseDir, "web", "templates", "*.html"))
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
