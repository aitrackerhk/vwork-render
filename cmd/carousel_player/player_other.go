//go:build !windows

package main

import (
	"fmt"
)

// playMedia 播放媒體 (非 Windows 平台 stub)
func (p *Player) playMedia(item CarouselItem) error {
	return fmt.Errorf("不支持的平台")
}

// createHTMLPlayer 創建 HTML 播放器 (非 Windows 平台 stub)
func createHTMLPlayer(items []CarouselItem, mediaDir string, slideInterval int) error {
	return fmt.Errorf("不支持的平台")
}
