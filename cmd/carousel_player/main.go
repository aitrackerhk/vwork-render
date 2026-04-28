package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Config 配置結構
type Config struct {
	ServerURL  string `json:"server_url"` // vWork 服務器地址
	APIKey     string `json:"api_key"`    // API 密鑰
	Fullscreen bool   `json:"fullscreen"` // 全螢幕模式
	Monitor    int    `json:"monitor"`    // 顯示器編號 (0 = 主顯示器)
}

// Manifest 輪播配置
type Manifest struct {
	Version            int            `json:"version"`
	SlideInterval      int            `json:"slide_interval"`
	TransitionDuration int            `json:"transition_duration"`
	AutoUpdate         bool           `json:"auto_update"`
	UpdateInterval     int            `json:"update_interval"`
	UpdateURL          string         `json:"update_url"`
	Items              []CarouselItem `json:"items"`
	GeneratedAt        string         `json:"generated_at"`
	AdPositionID       string         `json:"ad_position_id"`
	AdPositionCode     string         `json:"ad_position_code"`
	Language           string         `json:"language"` // 語言設定: auto, en, zh, zh-CN
}

// CarouselItem 輪播項目
type CarouselItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name"`
	MediaURL  string `json:"media_url,omitempty"` // 用於遠程更新
	Duration  int    `json:"duration"`
	SortOrder int    `json:"sort_order"`
}

// UpdateResponse 更新檢查響應
type UpdateResponse struct {
	NeedUpdate         bool           `json:"need_update"`
	CurrentVersion     int            `json:"current_version"`
	YourVersion        int            `json:"your_version"`
	Items              []CarouselItem `json:"items,omitempty"`
	SlideInterval      int            `json:"slide_interval,omitempty"`
	TransitionDuration int            `json:"transition_duration,omitempty"`
}

// Player 播放器狀態
type Player struct {
	config     Config
	manifest   Manifest
	mediaDir   string
	configPath string
	currentIdx int
	isPlaying  bool
	mu         sync.RWMutex
	updateChan chan struct{}
	stopChan   chan struct{}
}

var (
	version = "1.0.0"
	player  *Player
)

func main() {
	hideConsoleWindow()
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("🎬 輪播播放器 v%s 啟動中...", version)

	// 獲取執行目錄
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("無法獲取執行路徑: %v", err)
	}
	exeDir := filepath.Dir(exePath)

	// 初始化播放器
	player = &Player{
		mediaDir:   filepath.Join(exeDir, "media"),
		configPath: filepath.Join(exeDir, "config.json"),
		updateChan: make(chan struct{}, 1),
		stopChan:   make(chan struct{}),
	}

	// 載入配置
	if err := player.loadConfig(); err != nil {
		log.Printf("⚠️ 載入配置失敗，使用默認值: %v", err)
		player.config = Config{
			Fullscreen: true,
			Monitor:    0,
		}
	}

	// 載入 manifest
	manifestPath := filepath.Join(exeDir, "manifest.json")
	if err := player.loadManifest(manifestPath); err != nil {
		log.Fatalf("❌ 載入 manifest 失敗: %v", err)
	}

	log.Printf("✅ 載入 %d 個媒體項目", len(player.manifest.Items))

	// 檢查更新（如果有網絡連接）
	if player.manifest.AutoUpdate && player.config.ServerURL != "" {
		go player.checkUpdateOnStart()
	}

	// 啟動更新檢查器
	if player.manifest.AutoUpdate && player.config.ServerURL != "" {
		go player.startUpdateChecker()
	}

	// 設置信號處理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 使用 Webview 播放器
	items := player.getPlayableItems()
	if len(items) > 0 {
		// createHTMLPlayer 使用 webview，會阻塞直到窗口關閉
		log.Printf("🖥️ 正在啟動 Webview 播放器...")
		if err := createHTMLPlayer(items, player.mediaDir, player.manifest.SlideInterval); err != nil {
			log.Printf("⚠️ Webview 播放器啟動失敗: %v，使用控制台模式", err)
			player.startPlayback()
		} else {
			log.Println("✅ 播放器窗口已關閉")
		}
	} else {
		log.Println("⚠️ 沒有可播放的媒體文件")
		// 等待信號
		select {
		case <-sigChan:
			log.Println("收到退出信號，正在關閉...")
		case <-player.stopChan:
			log.Println("播放器已停止")
		}
	}
}

// loadConfig 載入配置文件
func (p *Player) loadConfig() error {
	data, err := os.ReadFile(p.configPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &p.config)
}

// saveConfig 保存配置文件
func (p *Player) saveConfig() error {
	data, err := json.MarshalIndent(p.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.configPath, data, 0644)
}

// loadManifest 載入 manifest 文件
func (p *Player) loadManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &p.manifest); err != nil {
		return err
	}
	// 設定語言
	SetLanguage(Language(p.manifest.Language))
	return nil
}

// saveManifest 保存 manifest 文件
func (p *Player) saveManifest() error {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	manifestPath := filepath.Join(exeDir, "manifest.json")

	data, err := json.MarshalIndent(p.manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, data, 0644)
}

// checkUpdateOnStart 啟動時檢查更新
func (p *Player) checkUpdateOnStart() {
	log.Println("🔄 檢查更新中...")

	resp, err := p.checkUpdate()
	if err != nil {
		log.Printf("⚠️ 更新檢查失敗: %v", err)
		return
	}

	if resp.NeedUpdate {
		log.Printf("📥 發現新版本 v%d（當前 v%d），開始下載...", resp.CurrentVersion, resp.YourVersion)
		if err := p.downloadUpdate(resp); err != nil {
			log.Printf("❌ 下載更新失敗: %v", err)
		} else {
			log.Println("✅ 更新完成")
			// 通知播放器刷新
			select {
			case p.updateChan <- struct{}{}:
			default:
			}
		}
	} else {
		log.Println("✅ 已是最新版本")
	}
}

// startUpdateChecker 啟動定時更新檢查
func (p *Player) startUpdateChecker() {
	interval := time.Duration(p.manifest.UpdateInterval) * time.Second
	if interval < time.Minute {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.checkUpdateOnStart()
		case <-p.stopChan:
			return
		}
	}
}

// checkUpdate 檢查更新
func (p *Player) checkUpdate() (*UpdateResponse, error) {
	if p.config.ServerURL == "" {
		return nil, fmt.Errorf("服務器地址未配置")
	}

	url := fmt.Sprintf("%s%s?version=%d", p.config.ServerURL, p.manifest.UpdateURL, p.manifest.Version)
	if p.config.APIKey != "" {
		url += "&api_key=" + p.config.APIKey
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if p.config.APIKey != "" {
		req.Header.Set("X-API-Key", p.config.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// downloadUpdate 下載更新
func (p *Player) downloadUpdate(update *UpdateResponse) error {
	// 創建新的媒體目錄
	newMediaDir := p.mediaDir + "_new"
	if err := os.MkdirAll(newMediaDir, 0755); err != nil {
		return fmt.Errorf("創建臨時目錄失敗: %v", err)
	}

	// 下載所有媒體文件
	for i, item := range update.Items {
		if item.MediaURL == "" {
			continue
		}

		log.Printf("📥 下載 (%d/%d): %s", i+1, len(update.Items), item.Name)

		// 確定文件名
		fileName := item.FileName
		if fileName == "" {
			ext := filepath.Ext(item.MediaURL)
			if ext == "" {
				if item.MediaType == "video" {
					ext = ".mp4"
				} else {
					ext = ".jpg"
				}
			}
			fileName = fmt.Sprintf("%d_%s%s", i+1, item.ID[:8], ext)
		}

		destPath := filepath.Join(newMediaDir, fileName)
		if err := p.downloadFile(item.MediaURL, destPath); err != nil {
			log.Printf("⚠️ 下載失敗: %s - %v", item.Name, err)
			continue
		}

		// 更新 item 的 FileName
		update.Items[i].FileName = fileName
	}

	// 備份舊的媒體目錄
	oldMediaDir := p.mediaDir + "_old"
	os.RemoveAll(oldMediaDir)
	if _, err := os.Stat(p.mediaDir); err == nil {
		if err := os.Rename(p.mediaDir, oldMediaDir); err != nil {
			log.Printf("⚠️ 備份舊媒體失敗: %v", err)
		}
	}

	// 替換為新的媒體目錄
	if err := os.Rename(newMediaDir, p.mediaDir); err != nil {
		// 恢復舊目錄
		os.Rename(oldMediaDir, p.mediaDir)
		return fmt.Errorf("替換媒體目錄失敗: %v", err)
	}

	// 清理舊目錄
	os.RemoveAll(oldMediaDir)

	// 更新 manifest
	p.mu.Lock()
	p.manifest.Version = update.CurrentVersion
	p.manifest.Items = update.Items
	if update.SlideInterval > 0 {
		p.manifest.SlideInterval = update.SlideInterval
	}
	if update.TransitionDuration > 0 {
		p.manifest.TransitionDuration = update.TransitionDuration
	}
	p.mu.Unlock()

	return p.saveManifest()
}

// downloadFile 下載文件
func (p *Player) downloadFile(url, destPath string) error {
	// 如果 URL 是相對路徑，拼接服務器地址
	if strings.HasPrefix(url, "/") && p.config.ServerURL != "" {
		url = p.config.ServerURL + url
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// startPlayback 開始播放
func (p *Player) startPlayback() {
	p.isPlaying = true

	// 獲取媒體文件列表
	items := p.getPlayableItems()
	if len(items) == 0 {
		log.Println("⚠️ 沒有可播放的媒體文件")
		// 等待更新
		select {
		case <-p.updateChan:
			p.startPlayback()
			return
		case <-p.stopChan:
			return
		}
	}

	log.Printf("▶️ 開始播放，共 %d 個項目", len(items))

	currentIdx := 0
	for {
		select {
		case <-p.stopChan:
			return
		case <-p.updateChan:
			// 刷新播放列表
			items = p.getPlayableItems()
			if len(items) == 0 {
				continue
			}
			currentIdx = 0
			log.Printf("🔄 播放列表已更新，共 %d 個項目", len(items))
		default:
			if len(items) == 0 {
				time.Sleep(time.Second)
				continue
			}

			item := items[currentIdx]
			duration := p.playItem(item)

			// 等待播放時間
			time.Sleep(duration)

			// 下一個
			currentIdx = (currentIdx + 1) % len(items)
		}
	}
}

// getPlayableItems 獲取可播放的項目列表
func (p *Player) getPlayableItems() []CarouselItem {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var items []CarouselItem
	for _, item := range p.manifest.Items {
		mediaPath := filepath.Join(p.mediaDir, item.FileName)
		if _, err := os.Stat(mediaPath); err == nil {
			items = append(items, item)
		}
	}

	// 按 SortOrder 排序
	sort.Slice(items, func(i, j int) bool {
		return items[i].SortOrder < items[j].SortOrder
	})

	return items
}

// playItem 播放一個項目
func (p *Player) playItem(item CarouselItem) time.Duration {
	// 這裡需要實際的媒體播放器
	// 目前只是打印日誌，實際實現需要使用 Windows API 或第三方庫
	log.Printf("🎬 播放: %s (%s)", item.Name, item.MediaType)

	duration := time.Duration(item.Duration) * time.Second
	if duration == 0 {
		p.mu.RLock()
		duration = time.Duration(p.manifest.SlideInterval) * time.Second
		p.mu.RUnlock()
	}
	if duration == 0 {
		duration = 5 * time.Second
	}

	// 實際播放媒體
	if err := p.playMedia(item); err != nil {
		log.Printf("⚠️ 播放失敗: %v", err)
	}

	return duration
}

// Stop 停止播放
func (p *Player) Stop() {
	close(p.stopChan)
}
