//go:build windows

package main

import (
	_ "embed"

	"github.com/lxn/walk"
)

//go:embed vwork-connector.manifest
var manifest []byte

func init() {
	// 設置高 DPI 支援
	walk.AppendToWalkInit(func() {
		walk.InteractionEffect, _ = walk.NewDropShadowEffect(walk.RGB(63, 63, 63))
	})
}
