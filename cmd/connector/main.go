package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var iconData embed.FS

var (
	version = "1.0.0"
	server  *Server
	cfg     *Config
)

// initLogFile sets up log output to a file in the config directory.
// This is necessary because -H windowsgui has no console for stdout/stderr.
func initLogFile() *os.File {
	var logDir string
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
	}
	logDir = filepath.Join(appData, "vWork Connector")
	os.MkdirAll(logDir, 0755)

	logPath := filepath.Join(logDir, "connector.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil
	}

	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	return f
}

func main() {
	// Initialize log file (no console in windowsgui mode)
	if logFile := initLogFile(); logFile != nil {
		defer logFile.Close()
	}

	// 載入配置
	var err error
	cfg, err = LoadConfig()
	if err != nil {
		log.Printf("Warning: Could not load config: %v, using defaults", err)
		cfg = DefaultConfig()
	}

	// 設置語言
	SetLanguage(Language(cfg.Language))

	// 啟動系統托盤
	systray.Run(onReady, onExit)
}

func onReady() {
	// 設置托盤圖標
	iconBytes, err := iconData.ReadFile("icon.ico")
	if err != nil {
		log.Printf("Warning: Could not load icon: %v", err)
	} else {
		systray.SetIcon(iconBytes)
	}

	systray.SetTitle("vWork Connector")
	systray.SetTooltip(fmt.Sprintf("vWork Connector v%s - Port %d", version, cfg.Port))

	// 菜單項目
	mStatus := systray.AddMenuItem(T(TkTrayStatusStart), T(TkTrayStatus))
	mStatus.Disable()

	systray.AddSeparator()

	mOpenSettings := systray.AddMenuItem(T(TkTrayOpenSettings), T(TkTrayOpenSettings))

	mPort := systray.AddMenuItem(fmt.Sprintf(T(TkTrayPort), cfg.Port), "HTTP API Port")
	mPort.Disable()

	systray.AddSeparator()

	mOpenConfig := systray.AddMenuItem(T(TkTrayOpenConfig), T(TkTrayOpenConfig))

	systray.AddSeparator()

	mAutoStart := systray.AddMenuItemCheckbox(T(TkTrayAutoStart), T(TkTrayAutoStart), cfg.AutoStart)

	systray.AddSeparator()

	mQuit := systray.AddMenuItem(T(TkTrayQuit), T(TkTrayQuit))

	// 啟動 HTTP 服務器
	server = NewServer(cfg)
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
			mStatus.SetTitle(T(TkTrayStatusError))
		}
	}()

	mStatus.SetTitle(fmt.Sprintf(T(TkTrayStatusRun), cfg.Port))

	// 啟動時自動打開設定視窗
	go OpenSettingsWindow()

	// 處理系統信號
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 事件循環
	go func() {
		for {
			select {
			case <-mOpenSettings.ClickedCh:
				go OpenSettingsWindow()

			case <-mOpenConfig.ClickedCh:
				OpenConfigFile()

			case <-mAutoStart.ClickedCh:
				cfg.AutoStart = !cfg.AutoStart
				if cfg.AutoStart {
					mAutoStart.Check()
					EnableAutoStart()
				} else {
					mAutoStart.Uncheck()
					DisableAutoStart()
				}
				cfg.Save()

			case <-mQuit.ClickedCh:
				systray.Quit()

			case <-sigChan:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	if server != nil {
		server.Stop()
	}
	log.Println("vWork Connector stopped")
}
