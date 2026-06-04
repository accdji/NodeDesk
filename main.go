package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"workflow/api"
	"workflow/config"
	"workflow/storage"
)

func main() {
	// 确定工作目录：优先当前工作目录，其次可执行文件目录
	wd, _ := os.Getwd()
	baseDir := wd
	if _, err := os.Stat(filepath.Join(baseDir, "config")); os.IsNotExist(err) {
		exe, _ := os.Executable()
		baseDir = filepath.Dir(exe)
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

	// 设置路由
	mux := http.NewServeMux()
	api.SetupRoutes(mux)

	// SPA 静态文件服务
	distDir := filepath.Join(baseDir, "web", "dist")

	// 检测开发模式：DEV 环境变量 或 web/dist 不存在
	devMode := os.Getenv("DEV") != ""
	if !devMode {
		if _, err := os.Stat(distDir); os.IsNotExist(err) {
			devMode = true
			log.Println("开发模式: web/dist 不存在，将反向代理到 Vite dev server (http://localhost:5173)")
		}
	}

	if devMode {
		// 开发模式：反向代理非 /api/ 请求到 Vite dev server
		viteURL, _ := url.Parse("http://localhost:5173")
		proxy := httputil.NewSingleHostReverseProxy(viteURL)

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// API 请求由 api.SetupRoutes 处理
			if strings.HasPrefix(path, "/api/") {
				http.NotFound(w, r)
				return
			}

			// 所有其他请求代理到 Vite
			proxy.ServeHTTP(w, r)
		})
	} else {
		// 生产模式：服务 web/dist/ 静态文件
		fs := http.FileServer(http.Dir(distDir))
		mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
			http.StripPrefix("/", fs).ServeHTTP(w, r)
		})
		// SPA fallback：所有非 API 路由返回 index.html
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			// API 请求由 api.SetupRoutes 处理
			if strings.HasPrefix(path, "/api/") {
				http.NotFound(w, r)
				return
			}
			// 静态资源
			if strings.HasPrefix(path, "/assets/") {
				fs.ServeHTTP(w, r)
				return
			}
			// SPA fallback
			http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
		})
	}

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	// 优雅关闭：监听 SIGINT/SIGTERM
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("正在关闭服务器...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("关闭超时: %v", err)
		}
	}()

	log.Printf("⚡ 工作流编排器已启动: http://localhost:%s", port)
	startMsg := "生产模式"
	if devMode {
		startMsg = "开发模式 (HMR 已启用)"
	}
	log.Printf("模式: %s", startMsg)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}
