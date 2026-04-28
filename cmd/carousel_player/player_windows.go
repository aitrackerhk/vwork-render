//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procLoadImageW       = user32.NewProc("LoadImageW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procInvalidateRect   = user32.NewProc("InvalidateRect")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procFillRect         = user32.NewProc("FillRect")
	procGetStockObject   = gdi32.NewProc("GetStockObject")
	procStretchDIBits    = gdi32.NewProc("StretchDIBits")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_POPUP            = 0x80000000
	WS_VISIBLE          = 0x10000000
	WS_EX_TOPMOST       = 0x00000008
	CW_USEDEFAULT       = 0x80000000
	SM_CXSCREEN         = 0
	SM_CYSCREEN         = 1
	SW_SHOW             = 5
	SW_MAXIMIZE         = 3
	WM_DESTROY          = 0x0002
	WM_PAINT            = 0x000F
	WM_TIMER            = 0x0113
	WM_KEYDOWN          = 0x0100
	VK_ESCAPE           = 0x1B
	VK_SPACE            = 0x20
	VK_LEFT             = 0x25
	VK_RIGHT            = 0x27
	HWND_TOPMOST        = ^uintptr(0)
	SWP_NOMOVE          = 0x0002
	SWP_NOSIZE          = 0x0001
	IMAGE_BITMAP        = 0
	LR_LOADFROMFILE     = 0x00000010
	LR_CREATEDIBSECTION = 0x00002000
	BLACK_BRUSH         = 4
	SRCCOPY             = 0x00CC0020
)

type WNDCLASSEXW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type MSG struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type PAINTSTRUCT struct {
	Hdc         syscall.Handle
	Erase       int32
	RcPaint     struct{ Left, Top, Right, Bottom int32 }
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

// playMedia 播放媒體 (Windows 平台)
func (p *Player) playMedia(item CarouselItem) error {
	mediaPath := filepath.Join(p.mediaDir, item.FileName)

	duration := time.Duration(item.Duration) * time.Second
	if duration == 0 {
		duration = 5 * time.Second
	}

	// 啟動播放器
	cmd := exec.Command("cmd", "/c", "start", "", mediaPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	// 啟動清理協程，在播放結束後嘗試關閉播放器窗口
	go func() {
		time.Sleep(duration)

		if item.MediaType == "video" {
			// 嘗試關閉常見的 Windows 視頻播放器
			players := []string{
				"Microsoft.Media.Player.exe", // 新版 Media Player
				"Video.UI.exe",               // Movies & TV (電影與電視)
				"wmplayer.exe",               // 舊版 Windows Media Player
				"vlc.exe",                    // VLC
				"mpv.exe",                    // mpv
			}
			for _, player := range players {
				exec.Command("taskkill", "/IM", player, "/F").Run()
			}
		} else {
			// 嘗試關閉常見的 Windows 圖片查看器
			viewers := []string{
				"Microsoft.Photos.exe", // Photos App
				"PhotosApp.exe",        // Windows 11 Photos
				"dllhost.exe",          // 舊版 Windows 照片查看器 (COM Surrogate)
			}
			for _, viewer := range viewers {
				exec.Command("taskkill", "/IM", viewer, "/F").Run()
			}
		}
	}()

	return nil
}

// GUIPlayer Windows GUI 播放器

type GUIPlayer struct {
	*Player
	hwnd         syscall.Handle
	currentImage syscall.Handle
	screenWidth  uintptr
	screenHeight uintptr
}

var guiPlayer *GUIPlayer

func wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	case WM_PAINT:
		var ps PAINTSTRUCT
		hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))

		// 填充黑色背景
		brush, _, _ := procGetStockObject.Call(BLACK_BRUSH)
		rect := ps.RcPaint
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)

		procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_KEYDOWN:
		switch wParam {
		case VK_ESCAPE:
			procPostQuitMessage.Call(0)
		case VK_SPACE:
			// 暫停/繼續
		case VK_LEFT:
			// 上一個
		case VK_RIGHT:
			// 下一個
		}
		return 0
	case WM_TIMER:
		// 切換到下一個媒體
		if guiPlayer != nil {
			guiPlayer.nextItem()
		}
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

func (g *GUIPlayer) createWindow() error {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("CarouselPlayer")
	windowTitle, _ := syscall.UTF16PtrFromString("輪播播放器")

	var wc WNDCLASSEXW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.WndProc = syscall.NewCallback(wndProc)
	wc.Instance = syscall.Handle(hInstance)
	wc.ClassName = className
	brush, _, _ := procGetStockObject.Call(BLACK_BRUSH)
	wc.Background = syscall.Handle(brush)

	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		return fmt.Errorf("RegisterClassEx failed: %v", err)
	}

	// 獲取屏幕尺寸
	g.screenWidth, _, _ = procGetSystemMetrics.Call(SM_CXSCREEN)
	g.screenHeight, _, _ = procGetSystemMetrics.Call(SM_CYSCREEN)

	// 創建全屏窗口
	hwnd, _, err := procCreateWindowExW.Call(
		WS_EX_TOPMOST,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		WS_POPUP|WS_VISIBLE,
		0, 0,
		g.screenWidth,
		g.screenHeight,
		0, 0,
		hInstance,
		0,
	)

	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx failed: %v", err)
	}

	g.hwnd = syscall.Handle(hwnd)
	procShowWindow.Call(hwnd, SW_SHOW)
	procUpdateWindow.Call(hwnd)

	return nil
}

func (g *GUIPlayer) nextItem() {
	// 實現切換邏輯
}

func (g *GUIPlayer) runMessageLoop() {
	var msg MSG
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0, 0, 0,
		)
		if ret == 0 {
			break
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// 簡化版：使用外部播放器
func playWithExternalPlayer(items []CarouselItem, mediaDir string, slideInterval int) {
	log.Printf("🎬 開始播放，共 %d 個項目，間隔 %d 秒", len(items), slideInterval)

	idx := 0
	for {
		if len(items) == 0 {
			time.Sleep(time.Second)
			continue
		}

		item := items[idx]
		mediaPath := filepath.Join(mediaDir, item.FileName)

		log.Printf("▶️ 播放: %s", item.Name)

		// 對於圖片，使用 Windows 照片查看器
		// 對於影片，使用默認播放器
		duration := time.Duration(item.Duration) * time.Second
		if duration == 0 {
			duration = time.Duration(slideInterval) * time.Second
		}
		if duration == 0 {
			duration = 5 * time.Second
		}

		if item.MediaType == "video" {
			// 播放影片
			cmd := exec.Command("cmd", "/c", "start", "/wait", "", mediaPath)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			cmd.Run()
		} else {
			// 顯示圖片一段時間
			cmd := exec.Command("cmd", "/c", "start", "", mediaPath)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			cmd.Start()
			time.Sleep(duration)
			// 關閉圖片查看器
			exec.Command("taskkill", "/IM", "Microsoft.Photos.exe", "/F").Run()
			exec.Command("taskkill", "/IM", "PhotosApp.exe", "/F").Run()
		}

		idx = (idx + 1) % len(items)
	}
}

// HTML 全屏輪播播放器 (使用本地 HTTP 服務器 + Edge kiosk 模式)
func createHTMLPlayer(items []CarouselItem, mediaDir string, slideInterval int) error {
	// 啟動本地 HTTP 服務器提供媒體文件和 HTML
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("無法啟動本地服務器: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// 準備項目數據
	var itemsJSON []map[string]interface{}
	for _, item := range items {
		itemsJSON = append(itemsJSON, map[string]interface{}{
			"name":       item.Name,
			"media_type": item.MediaType,
			"file_url":   fmt.Sprintf("/media/%s", item.FileName),
			"duration":   item.Duration,
		})
	}

	if len(itemsJSON) == 0 {
		return fmt.Errorf("沒有可播放的媒體文件")
	}

	itemsData, _ := json.Marshal(itemsJSON)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body { 
            background: #000; 
            overflow: hidden;
            width: 100vw;
            height: 100vh;
            cursor: none;
        }
        body.show-cursor { cursor: default; }
        .carousel-item {
            position: absolute;
            top: 0;
            left: 0;
            width: 100%%;
            height: 100%%;
            display: none;
            align-items: center;
            justify-content: center;
        }
        .carousel-item.active {
            display: flex;
        }
        .carousel-item img {
            max-width: 100%%;
            max-height: 100%%;
            object-fit: contain;
        }
        .carousel-item video {
            max-width: 100%%;
            max-height: 100%%;
            object-fit: contain;
        }
        .controls {
            position: fixed;
            bottom: 20px;
            left: 50%%;
            transform: translateX(-50%%);
            display: flex;
            gap: 12px;
            opacity: 0;
            transition: opacity 0.3s ease;
            z-index: 9999;
        }
        .controls.visible {
            opacity: 1;
        }
        .controls button {
            padding: 12px 24px;
            background: rgba(255, 255, 255, 0.9);
            color: #333;
            border: none;
            border-radius: 8px;
            font-size: 18px;
            font-weight: bold;
            cursor: pointer;
            box-shadow: 0 2px 10px rgba(0,0,0,0.3);
            min-width: 50px;
        }
        .controls button:hover {
            background: #007bff;
            color: white;
        }
        #exit-btn:hover {
            background: #ff4444 !important;
            color: white;
        }
    </style>
</head>
<body>
    <div id="carousel"></div>
    <div class="controls" id="controls">
        <button id="prev-btn">&lt;</button>
        <button id="exit-btn">%s</button>
        <button id="next-btn">&gt;</button>
    </div>
    <script>
        const items = %s;
        const defaultInterval = %d * 1000;
        let currentIndex = 0;
        let hideTimeout = null;
        let slideTimer = null;
        const controls = document.getElementById('controls');
		const exitBtn = document.getElementById('exit-btn');
        const prevBtn = document.getElementById('prev-btn');
        const nextBtn = document.getElementById('next-btn');
        
        // 鼠標移動時顯示控制按鈕
        document.addEventListener('mousemove', function() {
            document.body.classList.add('show-cursor');
            controls.classList.add('visible');
            
            if (hideTimeout) clearTimeout(hideTimeout);
            hideTimeout = setTimeout(function() {
                controls.classList.remove('visible');
                document.body.classList.remove('show-cursor');
            }, 3000);
        });
        
		function requestExit() {
			if (window.__exitRequested) return;
			window.__exitRequested = true;
			try { navigator.sendBeacon('/exit'); } catch (e) {}
			fetch('/exit', { method: 'POST', keepalive: true }).catch(function() {});
			setTimeout(function() {
				window.location.href = '/exit?ts=' + Date.now();
			}, 50);
		}

		// 點擊退出按鈕
		exitBtn.addEventListener('click', function() {
			requestExit();
		});
        
        // 點擊上一個
        prevBtn.addEventListener('click', function() {
            goToItem(-1);
        });
        
        // 點擊下一個
        nextBtn.addEventListener('click', function() {
            goToItem(1);
        });
        
        function createItems() {
            const container = document.getElementById('carousel');
            items.forEach((item, i) => {
                const div = document.createElement('div');
                div.className = 'carousel-item' + (i === 0 ? ' active' : '');
                
                if (item.media_type === 'video') {
                    const video = document.createElement('video');
                    video.src = item.file_url;
                    video.loop = false;
                    video.autoplay = (i === 0);
                    video.onended = function() { goToItem(1); };
                    div.appendChild(video);
                } else {
                    const img = document.createElement('img');
                    img.src = item.file_url;
                    div.appendChild(img);
                }
                
                container.appendChild(div);
            });
        }
        
        function stopCurrentMedia() {
            if (slideTimer) {
                clearTimeout(slideTimer);
                slideTimer = null;
            }
            const activeItem = document.querySelector('.carousel-item.active');
            if (activeItem) {
                const video = activeItem.querySelector('video');
                if (video) {
                    video.pause();
                }
            }
        }
        
        function goToItem(direction) {
            stopCurrentMedia();
            
            const allItems = document.querySelectorAll('.carousel-item');
            if (allItems.length === 0) return;
            
            allItems[currentIndex].classList.remove('active');
            
            currentIndex = (currentIndex + direction + items.length) %% items.length;
            allItems[currentIndex].classList.add('active');
            
            playCurrentItem();
        }
        
        function playCurrentItem() {
            const allItems = document.querySelectorAll('.carousel-item');
            const item = items[currentIndex];
            
            if (item.media_type === 'video') {
                const video = allItems[currentIndex].querySelector('video');
                video.currentTime = 0;
                video.play();
            } else {
                const duration = (item.duration > 0 ? item.duration : %d) * 1000;
                slideTimer = setTimeout(function() { goToItem(1); }, duration);
            }
        }
        
        function nextItem() {
            goToItem(1);
        }
        
        function start() {
            createItems();
            if (items.length === 0) return;
            
            playCurrentItem();
        }
        
        // ESC 鍵退出，方向鍵切換
		document.addEventListener('keydown', function(e) {
			if (e.key === 'Escape') {
				requestExit();
			} else if (e.key === 'ArrowLeft') {
				goToItem(-1);
			} else if (e.key === 'ArrowRight') {
				goToItem(1);
			}
		});
        
        start();
    </script>
</body>
</html>`, T(TkExit), string(itemsData), slideInterval, slideInterval)

	// 設置停止 channel
	stopChan := make(chan struct{}, 1)

	// 設置文件服務器
	mux := http.NewServeMux()
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(mediaDir))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	})
	mux.HandleFunc("/exit", func(w http.ResponseWriter, r *http.Request) {
		select {
		case stopChan <- struct{}{}:
		default:
		}
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	// 使用 Edge kiosk 模式全屏顯示
	edgePath := findEdgePath()
	if edgePath == "" {
		return fmt.Errorf("找不到 Microsoft Edge")
	}

	// 用 kiosk 模式啟動 Edge (全屏無邊框)
	cmd := exec.Command(edgePath,
		"--kiosk", url,
		"--edge-kiosk-type=fullscreen",
		"--no-first-run",
		"--disable-features=msEdgeSidebarSearchbar",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("無法啟動 Edge: %v", err)
	}

	// 等待退出信號
	<-stopChan

	// 關閉 Edge
	if cmd.Process != nil {
		pid := cmd.Process.Pid
		killCmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = killCmd.Run()
		_ = cmd.Process.Kill()
	}
	server.Close()

	return nil
}

// findEdgePath 尋找 Microsoft Edge 的路徑
func findEdgePath() string {
	paths := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// setFullscreenBorderless 設置 webview 窗口為無邊框全屏
func setFullscreenBorderless() {
	procSetWindowLongW := user32.NewProc("SetWindowLongW")
	procSetWindowPos := user32.NewProc("SetWindowPos")
	procFindWindowW := user32.NewProc("FindWindowW")

	const (
		GWL_STYLE        = ^uintptr(15) // -16 as uintptr
		GWL_EXSTYLE      = ^uintptr(19) // -20 as uintptr
		SWP_FRAMECHANGED = 0x0020
		SWP_SHOWWINDOW   = 0x0040
	)

	// 找到 webview 窗口 (WebView2 使用的類名)
	className, _ := syscall.UTF16PtrFromString("Chrome_WidgetWin_0")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)

	if hwnd == 0 {
		// 嘗試其他類名
		className, _ = syscall.UTF16PtrFromString("Webview2")
		hwnd, _, _ = procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	}

	if hwnd == 0 {
		return
	}

	// 獲取屏幕尺寸
	screenWidth, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	screenHeight, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)

	// 設置無邊框樣式
	procSetWindowLongW.Call(hwnd, GWL_STYLE, WS_POPUP|WS_VISIBLE)
	procSetWindowLongW.Call(hwnd, GWL_EXSTYLE, WS_EX_TOPMOST)

	// 設置位置和大小
	procSetWindowPos.Call(
		hwnd,
		HWND_TOPMOST,
		0, 0,
		screenWidth,
		screenHeight,
		SWP_FRAMECHANGED|SWP_SHOWWINDOW,
	)
}

// 保留原有的 base64 方法作為備選
func createHTMLPlayerBase64(items []CarouselItem, mediaDir string, slideInterval int) error {
	// 準備項目數據，將媒體轉換為 base64
	var itemsJSON []map[string]interface{}
	for _, item := range items {
		absPath := filepath.Join(mediaDir, item.FileName)

		// 讀取文件並轉換為 base64
		fileData, err := os.ReadFile(absPath)
		if err != nil {
			log.Printf("⚠️ 無法讀取文件 %s: %v", item.FileName, err)
			continue
		}

		// 確定 MIME 類型
		var mimeType string
		ext := strings.ToLower(filepath.Ext(item.FileName))
		switch ext {
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".png":
			mimeType = "image/png"
		case ".gif":
			mimeType = "image/gif"
		case ".webp":
			mimeType = "image/webp"
		case ".mp4":
			mimeType = "video/mp4"
		case ".webm":
			mimeType = "video/webm"
		default:
			if item.MediaType == "video" {
				mimeType = "video/mp4"
			} else {
				mimeType = "image/jpeg"
			}
		}

		// 創建 data URL
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(fileData))

		itemsJSON = append(itemsJSON, map[string]interface{}{
			"name":       item.Name,
			"media_type": item.MediaType,
			"data_url":   dataURL,
			"duration":   item.Duration,
		})
	}

	if len(itemsJSON) == 0 {
		return fmt.Errorf("沒有可播放的媒體文件")
	}

	_ = itemsJSON
	return nil
}
