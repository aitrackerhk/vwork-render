package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Config 應用配置
type Config struct {
	Port           int                  `json:"port"`
	AutoStart      bool                 `json:"autoStart"`
	MinimizeToTray bool                 `json:"minimizeToTray"`
	LogLevel       string               `json:"logLevel"`
	Language       string               `json:"language"`
	CardTerminals  []CardTerminalConfig `json:"cardTerminals"`

	configPath string
}

// DefaultConfig 默認配置
func DefaultConfig() *Config {
	return &Config{
		Port:           9527,
		AutoStart:      false,
		MinimizeToTray: true,
		LogLevel:       "info",
		Language:       "auto",
		CardTerminals:  []CardTerminalConfig{},
	}
}

// LoadConfig 載入配置
func LoadConfig() (*Config, error) {
	configPath := getConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.configPath = configPath
			return cfg, nil
		}
		return nil, err
	}

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	cfg.configPath = configPath
	return cfg, nil
}

// Save 保存配置
func (c *Config) Save() error {
	// 確保目錄存在
	dir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.configPath, data, 0644)
}

// getConfigPath 獲取配置文件路徑
func getConfigPath() string {
	// Windows: %APPDATA%\vWork Connector\config.json
	// Linux/Mac: ~/.config/vwork-connector/config.json

	var configDir string

	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		configDir = filepath.Join(appData, "vWork Connector")
	} else {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "vwork-connector")
	}

	return filepath.Join(configDir, "config.json")
}

// OpenConfigFile 開啟配置文件
func OpenConfigFile() {
	configPath := getConfigPath()

	// 確保文件存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := DefaultConfig()
		cfg.configPath = configPath
		cfg.Save()
	}

	// 用默認編輯器打開
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad", configPath)
	case "darwin":
		cmd = exec.Command("open", configPath)
	default:
		cmd = exec.Command("xdg-open", configPath)
	}

	cmd.Start()
}

// EnableAutoStart 啟用開機自動啟動
func EnableAutoStart() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	exePath, _ := os.Executable()

	// 添加到註冊表
	cmd := exec.Command("reg", "add",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "vWorkConnector",
		"/t", "REG_SZ",
		"/d", exePath,
		"/f")
	hideWindow(cmd)

	return cmd.Run()
}

// DisableAutoStart 禁用開機自動啟動
func DisableAutoStart() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	cmd := exec.Command("reg", "delete",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "vWorkConnector",
		"/f")
	hideWindow(cmd)

	return cmd.Run()
}
