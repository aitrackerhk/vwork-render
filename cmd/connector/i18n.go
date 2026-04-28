package main

import (
	"syscall"
)

// Language 語言代碼
type Language string

const (
	LangAuto Language = "auto" // 系統語言
	LangEn   Language = "en"   // English
	LangZhTW Language = "zh"   // 繁體中文
	LangZhCN Language = "zh-CN" // 简体中文
)

// AllLanguages 所有可用語言
var AllLanguages = []Language{LangAuto, LangEn, LangZhTW, LangZhCN}

// LanguageNames 語言顯示名稱
var LanguageNames = map[Language]string{
	LangAuto: "系統語言 / System",
	LangEn:   "English",
	LangZhTW: "繁體中文",
	LangZhCN: "简体中文",
}

// 翻譯鍵
type TranslationKey string

const (
	// 通用
	TkAppName       TranslationKey = "app_name"
	TkVersion       TranslationKey = "version"
	TkSave          TranslationKey = "save"
	TkClose         TranslationKey = "close"
	TkReady         TranslationKey = "ready"
	TkTesting       TranslationKey = "testing"
	TkSuccess       TranslationKey = "success"
	TkError         TranslationKey = "error"
	TkHint          TranslationKey = "hint"

	// 托盤菜單
	TkTrayStatus       TranslationKey = "tray_status"
	TkTrayStatusStart  TranslationKey = "tray_status_start"
	TkTrayStatusRun    TranslationKey = "tray_status_run"
	TkTrayStatusError  TranslationKey = "tray_status_error"
	TkTrayOpenSettings TranslationKey = "tray_open_settings"
	TkTrayPort         TranslationKey = "tray_port"
	TkTrayOpenConfig   TranslationKey = "tray_open_config"
	TkTrayAutoStart    TranslationKey = "tray_auto_start"
	TkTrayQuit         TranslationKey = "tray_quit"

	// 設定窗口
	TkSettingsTitle    TranslationKey = "settings_title"
	TkSettingsGeneral  TranslationKey = "settings_general"
	TkSettingsApiPort  TranslationKey = "settings_api_port"
	TkSettingsAutoStart TranslationKey = "settings_auto_start"
	TkSettingsSaved    TranslationKey = "settings_saved"
	TkSettingsPortChanged TranslationKey = "settings_port_changed"
	TkSettingsInvalidPort TranslationKey = "settings_invalid_port"
	TkSettingsLanguage TranslationKey = "settings_language"
	TkSettingsLangChanged TranslationKey = "settings_lang_changed"

	// 卡機設定
	TkCardTerminal     TranslationKey = "card_terminal"
	TkCardTerminalAdd  TranslationKey = "card_terminal_add"
	TkCardTerminalName TranslationKey = "card_terminal_name"
	TkCardTerminalType TranslationKey = "card_terminal_type"
	TkCardTerminalIP   TranslationKey = "card_terminal_ip"
	TkCardTerminalPort TranslationKey = "card_terminal_port"
	TkCardTerminalMerchant TranslationKey = "card_terminal_merchant"
	TkCardTerminalTerminalId TranslationKey = "card_terminal_terminal_id"
	TkCardTerminalTest TranslationKey = "card_terminal_test"
	TkCardTerminalAdd2 TranslationKey = "card_terminal_add2"
	TkCardTerminalSaved TranslationKey = "card_terminal_saved"
	TkCardTerminalList TranslationKey = "card_terminal_list"
	TkCardTerminalDelete TranslationKey = "card_terminal_delete"
	TkCardTerminalTestSelected TranslationKey = "card_terminal_test_selected"
	TkCardTerminalDeleted TranslationKey = "card_terminal_deleted"
	TkCardTerminalConnFail TranslationKey = "card_terminal_conn_fail"
	TkCardTerminalConnSuccess TranslationKey = "card_terminal_conn_success"
	TkCardTerminalFillRequired TranslationKey = "card_terminal_fill_required"
	TkCardTerminalFillIpPort TranslationKey = "card_terminal_fill_ip_port"
	TkCardTerminalModel TranslationKey = "card_terminal_model"
	TkCardTerminalSerial TranslationKey = "card_terminal_serial"
)

// 翻譯表
var translations = map[Language]map[TranslationKey]string{
	LangEn: {
		TkAppName:       "vWork Connector",
		TkVersion:       "Version",
		TkSave:          "Save",
		TkClose:         "Close",
		TkReady:         "Ready",
		TkTesting:       "Testing...",
		TkSuccess:       "Success",
		TkError:         "Error",
		TkHint:          "Notice",

		TkTrayStatus:       "Status",
		TkTrayStatusStart:  "Status: Starting...",
		TkTrayStatusRun:    "Status: Running (:%d)",
		TkTrayStatusError:  "Status: Error",
		TkTrayOpenSettings: "Open Settings",
		TkTrayPort:         "Port: %d",
		TkTrayOpenConfig:   "Open Config File",
		TkTrayAutoStart:    "Start on Login",
		TkTrayQuit:         "Quit",

		TkSettingsTitle:    "vWork Connector Settings",
		TkSettingsGeneral:  "General Settings",
		TkSettingsApiPort:  "API Port:",
		TkSettingsAutoStart: "Start on Login",
		TkSettingsSaved:    "Settings saved",
		TkSettingsPortChanged: "Port changed, restart required to take effect",
		TkSettingsInvalidPort: "Please enter a valid port (1-65535)",
		TkSettingsLanguage: "Language:",
		TkSettingsLangChanged: "Language changed, restart required to take effect",

		TkCardTerminal:     "Card Terminal",
		TkCardTerminalAdd:  "Add Card Terminal",
		TkCardTerminalName: "Name:",
		TkCardTerminalType: "Type:",
		TkCardTerminalIP:   "IP Address:",
		TkCardTerminalPort: "Port:",
		TkCardTerminalMerchant: "Merchant ID (optional):",
		TkCardTerminalTerminalId: "Terminal ID (optional):",
		TkCardTerminalTest: "Test Connection",
		TkCardTerminalAdd2: "Add Terminal",
		TkCardTerminalSaved: "Terminal added",
		TkCardTerminalList: "Saved Terminals",
		TkCardTerminalDelete: "Delete Selected",
		TkCardTerminalTestSelected: "Test Selected",
		TkCardTerminalDeleted: "Terminal deleted",
		TkCardTerminalConnFail: "Connection failed",
		TkCardTerminalConnSuccess: "Connection successful",
		TkCardTerminalFillRequired: "Please fill in required fields",
		TkCardTerminalFillIpPort: "Please fill in IP and Port",
		TkCardTerminalModel: "Model: %s",
		TkCardTerminalSerial: "Serial: %s",
	},
	LangZhTW: {
		TkAppName:       "vWork Connector",
		TkVersion:       "版本",
		TkSave:          "保存",
		TkClose:         "關閉",
		TkReady:         "就緒",
		TkTesting:       "測試中...",
		TkSuccess:       "成功",
		TkError:         "錯誤",
		TkHint:          "提示",

		TkTrayStatus:       "狀態",
		TkTrayStatusStart:  "狀態: 啟動中...",
		TkTrayStatusRun:    "狀態: 運行中 (:%d)",
		TkTrayStatusError:  "狀態: 錯誤",
		TkTrayOpenSettings: "開啟設定視窗",
		TkTrayPort:         "連接埠: %d",
		TkTrayOpenConfig:   "開啟設定檔",
		TkTrayAutoStart:    "開機自動啟動",
		TkTrayQuit:         "退出",

		TkSettingsTitle:    "vWork Connector 設定",
		TkSettingsGeneral:  "一般設定",
		TkSettingsApiPort:  "API 連接埠:",
		TkSettingsAutoStart: "開機自動啟動",
		TkSettingsSaved:    "設定已保存",
		TkSettingsPortChanged: "連接埠已更改，需要重啟程式才會生效",
		TkSettingsInvalidPort: "請輸入有效的連接埠 (1-65535)",
		TkSettingsLanguage: "語言:",
		TkSettingsLangChanged: "語言已更改，需要重啟程式才會生效",

		TkCardTerminal:     "卡機",
		TkCardTerminalAdd:  "新增卡機",
		TkCardTerminalName: "卡機名稱:",
		TkCardTerminalType: "卡機類型:",
		TkCardTerminalIP:   "IP 地址:",
		TkCardTerminalPort: "連接埠:",
		TkCardTerminalMerchant: "商戶編號 (可選):",
		TkCardTerminalTerminalId: "終端編號 (可選):",
		TkCardTerminalTest: "測試連接",
		TkCardTerminalAdd2: "新增卡機",
		TkCardTerminalSaved: "卡機已新增",
		TkCardTerminalList: "已保存的卡機",
		TkCardTerminalDelete: "刪除選中",
		TkCardTerminalTestSelected: "測試選中",
		TkCardTerminalDeleted: "卡機已刪除",
		TkCardTerminalConnFail: "連接失敗",
		TkCardTerminalConnSuccess: "連接成功",
		TkCardTerminalFillRequired: "請填寫必要欄位",
		TkCardTerminalFillIpPort: "請填寫 IP 和連接埠",
		TkCardTerminalModel: "型號: %s",
		TkCardTerminalSerial: "序號: %s",
	},
	LangZhCN: {
		TkAppName:       "vWork Connector",
		TkVersion:       "版本",
		TkSave:          "保存",
		TkClose:         "关闭",
		TkReady:         "就绪",
		TkTesting:       "测试中...",
		TkSuccess:       "成功",
		TkError:         "错误",
		TkHint:          "提示",

		TkTrayStatus:       "状态",
		TkTrayStatusStart:  "状态: 启动中...",
		TkTrayStatusRun:    "状态: 运行中 (:%d)",
		TkTrayStatusError:  "状态: 错误",
		TkTrayOpenSettings: "打开设置窗口",
		TkTrayPort:         "端口: %d",
		TkTrayOpenConfig:   "打开配置文件",
		TkTrayAutoStart:    "开机自动启动",
		TkTrayQuit:         "退出",

		TkSettingsTitle:    "vWork Connector 设置",
		TkSettingsGeneral:  "常规设置",
		TkSettingsApiPort:  "API 端口:",
		TkSettingsAutoStart: "开机自动启动",
		TkSettingsSaved:    "设置已保存",
		TkSettingsPortChanged: "端口已更改，需要重启程序才会生效",
		TkSettingsInvalidPort: "请输入有效的端口 (1-65535)",
		TkSettingsLanguage: "语言:",
		TkSettingsLangChanged: "语言已更改，需要重启程序才会生效",

		TkCardTerminal:     "卡机",
		TkCardTerminalAdd:  "添加卡机",
		TkCardTerminalName: "卡机名称:",
		TkCardTerminalType: "卡机类型:",
		TkCardTerminalIP:   "IP 地址:",
		TkCardTerminalPort: "端口:",
		TkCardTerminalMerchant: "商户编号 (可选):",
		TkCardTerminalTerminalId: "终端编号 (可选):",
		TkCardTerminalTest: "测试连接",
		TkCardTerminalAdd2: "添加卡机",
		TkCardTerminalSaved: "卡机已添加",
		TkCardTerminalList: "已保存的卡机",
		TkCardTerminalDelete: "删除选中",
		TkCardTerminalTestSelected: "测试选中",
		TkCardTerminalDeleted: "卡机已删除",
		TkCardTerminalConnFail: "连接失败",
		TkCardTerminalConnSuccess: "连接成功",
		TkCardTerminalFillRequired: "请填写必要字段",
		TkCardTerminalFillIpPort: "请填写 IP 和端口",
		TkCardTerminalModel: "型号: %s",
		TkCardTerminalSerial: "序号: %s",
	},
}

var currentLanguage Language = LangAuto

// SetLanguage 設置當前語言
func SetLanguage(lang Language) {
	currentLanguage = lang
}

// GetLanguage 獲取當前語言
func GetLanguage() Language {
	return currentLanguage
}

// GetEffectiveLanguage 獲取有效語言（解析 auto）
func GetEffectiveLanguage() Language {
	if currentLanguage == LangAuto {
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

// GetLanguageIndex 獲取語言在列表中的索引
func GetLanguageIndex(lang Language) int {
	for i, l := range AllLanguages {
		if l == lang {
			return i
		}
	}
	return 0
}

// GetLanguageDisplayNames 獲取語言顯示名稱列表
func GetLanguageDisplayNames() []string {
	names := make([]string, len(AllLanguages))
	for i, lang := range AllLanguages {
		names[i] = LanguageNames[lang]
	}
	return names
}
