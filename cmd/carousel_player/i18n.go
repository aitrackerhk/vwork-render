package main

import (
	"syscall"
)

// Language 語言代碼
type Language string

const (
	LangAuto Language = "auto"  // 系統語言
	LangEn   Language = "en"    // English
	LangZhTW Language = "zh"    // 繁體中文
	LangZhCN Language = "zh-CN" // 简体中文
)

// 翻譯鍵
type TranslationKey string

const (
	TkExit TranslationKey = "exit" // 離開按鈕
)

// 翻譯表
var translations = map[Language]map[TranslationKey]string{
	LangEn: {
		TkExit: "Exit",
	},
	LangZhTW: {
		TkExit: "離開",
	},
	LangZhCN: {
		TkExit: "退出",
	},
}

// 當前語言設定
var currentLanguage Language = LangAuto

// SetLanguage 設定當前語言
func SetLanguage(lang Language) {
	if lang == "" {
		currentLanguage = LangAuto
	} else {
		currentLanguage = lang
	}
}

// GetEffectiveLanguage 獲取實際使用的語言
func GetEffectiveLanguage() Language {
	if currentLanguage == LangAuto || currentLanguage == "" {
		return detectSystemLanguage()
	}
	return currentLanguage
}

// T 獲取翻譯文本
func T(key TranslationKey) string {
	lang := GetEffectiveLanguage()
	if trans, ok := translations[lang]; ok {
		if text, ok := trans[key]; ok {
			return text
		}
	}
	// 回退到繁體中文
	if trans, ok := translations[LangZhTW]; ok {
		if text, ok := trans[key]; ok {
			return text
		}
	}
	return string(key)
}

// detectSystemLanguage 檢測系統語言
func detectSystemLanguage() Language {
	// 使用 Windows API 獲取用戶界面語言
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getUserDefaultUILanguage := kernel32.NewProc("GetUserDefaultUILanguage")

	ret, _, _ := getUserDefaultUILanguage.Call()
	langID := uint16(ret)

	// 主要語言 ID (低 10 位)
	primaryLangID := langID & 0x3FF

	// 中文 = 0x04
	if primaryLangID == 0x04 {
		// 子語言 ID
		subLangID := langID >> 10

		// 簡體中文: 中國大陸 (2), 新加坡 (4)
		// 繁體中文: 台灣 (1), 香港 (3), 澳門 (5)
		switch subLangID {
		case 2, 4:
			return LangZhCN
		default:
			return LangZhTW
		}
	}

	return LangEn
}
