package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

var (
	settingsWindow *walk.MainWindow
	terminalsList  *walk.ListBox
)

// OpenSettingsWindow opens the native Windows settings window with a clean, modern layout.
func OpenSettingsWindow() {
	if settingsWindow != nil {
		settingsWindow.Show()
		settingsWindow.Activate()
		return
	}

	var (
		nameEdit       *walk.LineEdit
		typeCombo      *walk.ComboBox
		ipEdit         *walk.LineEdit
		portEdit       *walk.LineEdit
		merchantEdit   *walk.LineEdit
		terminalIdEdit *walk.LineEdit
		apiPortEdit    *walk.LineEdit
		autoStartCheck *walk.CheckBox
		langCombo      *walk.ComboBox
		statusLabel    *walk.Label
		tabWidget      *walk.TabWidget
	)

	// Card terminal type options
	terminalTypes := []string{"kpay", "bbmsl", "hsbc", "other"}

	// Update terminal list display
	updateTerminalsList := func() {
		if terminalsList == nil {
			return
		}
		items := make([]string, len(cfg.CardTerminals))
		for i, t := range cfg.CardTerminals {
			items[i] = fmt.Sprintf("%s  |  %s  |  %s:%d", t.Name, t.Type, t.IP, t.Port)
		}
		terminalsList.SetModel(items)
	}

	// Clear the add-terminal form fields
	clearForm := func() {
		nameEdit.SetText("")
		ipEdit.SetText("")
		portEdit.SetText("8080")
		merchantEdit.SetText("")
		terminalIdEdit.SetText("")
	}

	err := MainWindow{
		AssignTo: &settingsWindow,
		Title:    T(TkSettingsTitle),
		MinSize:  Size{Width: 540, Height: 620},
		Size:     Size{Width: 580, Height: 680},
		Layout:   VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 16}, Spacing: 12},
		Font:     Font{Family: "Segoe UI", PointSize: 9},
		Children: []Widget{

			// ── Header ──
			Composite{
				Layout: HBox{Margins: Margins{Bottom: 4}},
				Children: []Widget{
					Label{
						Text: "vWork Connector",
						Font: Font{Family: "Segoe UI Semibold", PointSize: 15},
					},
					HSpacer{},
					Label{
						Text: fmt.Sprintf("v%s", version),
						Font: Font{Family: "Segoe UI", PointSize: 9},
					},
				},
			},

			// ── Status bar ──
			Label{
				AssignTo: &statusLabel,
				Text:     T(TkReady),
				Font:     Font{Family: "Segoe UI", PointSize: 9},
			},

			// ── Tab pages ──
			TabWidget{
				AssignTo: &tabWidget,
				Pages: []TabPage{

					// ═══════ Tab 1: Card Terminals ═══════
					{
						Title:  " " + T(TkCardTerminal) + " ",
						Layout: VBox{Spacing: 12, Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
						Children: []Widget{

							// ── Add terminal form ──
							GroupBox{
								Title:  T(TkCardTerminalAdd),
								Layout: Grid{Columns: 2, Spacing: 10, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 12}},
								Children: []Widget{
									Label{Text: T(TkCardTerminalName)},
									LineEdit{AssignTo: &nameEdit, MinSize: Size{Width: 260}},

									Label{Text: T(TkCardTerminalType)},
									ComboBox{AssignTo: &typeCombo, Model: terminalTypes, Value: "kpay"},

									Label{Text: T(TkCardTerminalIP)},
									LineEdit{AssignTo: &ipEdit, CueBanner: "192.168.1.100"},

									Label{Text: T(TkCardTerminalPort)},
									LineEdit{AssignTo: &portEdit, Text: "8080"},

									Label{Text: T(TkCardTerminalMerchant)},
									LineEdit{AssignTo: &merchantEdit},

									Label{Text: T(TkCardTerminalTerminalId)},
									LineEdit{AssignTo: &terminalIdEdit},
								},
							},

							// ── Action buttons ──
							Composite{
								Layout: HBox{Spacing: 10},
								Children: []Widget{
									PushButton{
										Text:    T(TkCardTerminalTest),
										MinSize: Size{Width: 120, Height: 32},
										OnClicked: func() {
											ip := ipEdit.Text()
											portStr := portEdit.Text()
											termType := ""
											if typeCombo.CurrentIndex() >= 0 {
												termType = terminalTypes[typeCombo.CurrentIndex()]
											}
											if ip == "" || portStr == "" {
												walk.MsgBox(settingsWindow, T(TkHint), T(TkCardTerminalFillIpPort), walk.MsgBoxIconWarning)
												return
											}
											port, _ := strconv.Atoi(portStr)
											terminal := CardTerminalConfig{Type: termType, IP: ip, Port: port}
											statusLabel.SetText(T(TkTesting))
											go func() {
												info, err := TestCardTerminal(terminal)
												settingsWindow.Synchronize(func() {
													if err != nil {
														statusLabel.SetText(T(TkCardTerminalConnFail) + ": " + err.Error())
														walk.MsgBox(settingsWindow, T(TkCardTerminalConnFail), err.Error(), walk.MsgBoxIconError)
													} else {
														statusLabel.SetText(T(TkCardTerminalConnSuccess) + ": " + info.Model)
														walk.MsgBox(settingsWindow, T(TkCardTerminalConnSuccess),
															fmt.Sprintf(T(TkCardTerminalModel)+"\n"+T(TkCardTerminalSerial), info.Model, info.SerialNumber),
															walk.MsgBoxIconInformation)
													}
												})
											}()
										},
									},
									PushButton{
										Text:    T(TkCardTerminalAdd2),
										MinSize: Size{Width: 120, Height: 32},
										OnClicked: func() {
											name := nameEdit.Text()
											ip := ipEdit.Text()
											portStr := portEdit.Text()
											if name == "" || ip == "" || portStr == "" {
												walk.MsgBox(settingsWindow, T(TkHint), T(TkCardTerminalFillRequired), walk.MsgBoxIconWarning)
												return
											}
											port, _ := strconv.Atoi(portStr)
											termType := "kpay"
											if typeCombo.CurrentIndex() >= 0 {
												termType = terminalTypes[typeCombo.CurrentIndex()]
											}
											terminal := CardTerminalConfig{
												ID:         fmt.Sprintf("terminal-%d", len(cfg.CardTerminals)+1),
												Name:       name,
												Type:       termType,
												IP:         ip,
												Port:       port,
												MerchantID: merchantEdit.Text(),
												TerminalID: terminalIdEdit.Text(),
											}
											cfg.CardTerminals = append(cfg.CardTerminals, terminal)
											cfg.Save()
											clearForm()
											updateTerminalsList()
											statusLabel.SetText(T(TkCardTerminalSaved))
										},
									},
									HSpacer{},
								},
							},

							// ── Saved terminals list ──
							GroupBox{
								Title:  T(TkCardTerminalList),
								Layout: VBox{Spacing: 8, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 12}},
								Children: []Widget{
									ListBox{
										AssignTo: &terminalsList,
										MinSize:  Size{Height: 120},
									},
									Composite{
										Layout: HBox{Spacing: 10},
										Children: []Widget{
											PushButton{
												Text:    T(TkCardTerminalDelete),
												MinSize: Size{Width: 110, Height: 30},
												OnClicked: func() {
													idx := terminalsList.CurrentIndex()
													if idx < 0 || idx >= len(cfg.CardTerminals) {
														return
													}
													cfg.CardTerminals = append(cfg.CardTerminals[:idx], cfg.CardTerminals[idx+1:]...)
													cfg.Save()
													updateTerminalsList()
													statusLabel.SetText(T(TkCardTerminalDeleted))
												},
											},
											PushButton{
												Text:    T(TkCardTerminalTestSelected),
												MinSize: Size{Width: 110, Height: 30},
												OnClicked: func() {
													idx := terminalsList.CurrentIndex()
													if idx < 0 || idx >= len(cfg.CardTerminals) {
														return
													}
													terminal := cfg.CardTerminals[idx]
													statusLabel.SetText(T(TkTesting))
													go func() {
														info, err := TestCardTerminal(terminal)
														settingsWindow.Synchronize(func() {
															if err != nil {
																statusLabel.SetText(T(TkCardTerminalConnFail))
																walk.MsgBox(settingsWindow, T(TkCardTerminalConnFail), err.Error(), walk.MsgBoxIconError)
															} else {
																statusLabel.SetText(T(TkCardTerminalConnSuccess))
																walk.MsgBox(settingsWindow, T(TkCardTerminalConnSuccess),
																	fmt.Sprintf(T(TkCardTerminalModel), info.Model),
																	walk.MsgBoxIconInformation)
															}
														})
													}()
												},
											},
											HSpacer{},
										},
									},
								},
							},
						},
					},

					// ═══════ Tab 2: General Settings ═══════
					{
						Title:  " " + T(TkSettingsGeneral) + " ",
						Layout: VBox{Spacing: 14, Margins: Margins{Left: 12, Top: 16, Right: 12, Bottom: 12}},
						Children: []Widget{
							GroupBox{
								Title:  T(TkSettingsGeneral),
								Layout: Grid{Columns: 2, Spacing: 12, Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 14}},
								Children: []Widget{
									Label{Text: T(TkSettingsLanguage)},
									ComboBox{
										AssignTo:     &langCombo,
										Model:        GetLanguageDisplayNames(),
										CurrentIndex: GetLanguageIndex(Language(cfg.Language)),
									},

									Label{Text: T(TkSettingsApiPort)},
									LineEdit{
										AssignTo: &apiPortEdit,
										Text:     fmt.Sprintf("%d", cfg.Port),
										MaxSize:  Size{Width: 120},
									},

									Label{Text: ""},
									CheckBox{
										AssignTo: &autoStartCheck,
										Text:     T(TkSettingsAutoStart),
										Checked:  cfg.AutoStart,
									},
								},
							},
							VSpacer{},
						},
					},
				},
			},

			// ── Bottom action bar ──
			Composite{
				Layout: HBox{Spacing: 10, Margins: Margins{Top: 4}},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text:    T(TkSave),
						MinSize: Size{Width: 110, Height: 34},
						Font:    Font{Family: "Segoe UI Semibold", PointSize: 10},
						OnClicked: func() {
							portStr := apiPortEdit.Text()
							port, err := strconv.Atoi(portStr)
							if err != nil || port < 1 || port > 65535 {
								walk.MsgBox(settingsWindow, T(TkError), T(TkSettingsInvalidPort), walk.MsgBoxIconError)
								return
							}

							portChanged := cfg.Port != port
							cfg.Port = port

							langChanged := false
							if langCombo.CurrentIndex() >= 0 && langCombo.CurrentIndex() < len(AllLanguages) {
								newLang := string(AllLanguages[langCombo.CurrentIndex()])
								if newLang != cfg.Language {
									cfg.Language = newLang
									langChanged = true
								}
							}

							if autoStartCheck.Checked() != cfg.AutoStart {
								cfg.AutoStart = autoStartCheck.Checked()
								if cfg.AutoStart {
									EnableAutoStart()
								} else {
									DisableAutoStart()
								}
							}

							cfg.Save()

							if portChanged || langChanged {
								msg := ""
								if portChanged && langChanged {
									msg = T(TkSettingsPortChanged) + "\n" + T(TkSettingsLangChanged)
								} else if portChanged {
									msg = T(TkSettingsPortChanged)
								} else {
									msg = T(TkSettingsLangChanged)
								}
								walk.MsgBox(settingsWindow, T(TkHint), msg, walk.MsgBoxIconInformation)
							} else {
								statusLabel.SetText(T(TkSettingsSaved))
							}
						},
					},
					PushButton{
						Text:    T(TkClose),
						MinSize: Size{Width: 90, Height: 34},
						OnClicked: func() {
							settingsWindow.Close()
						},
					},
				},
			},
		},
	}.Create()

	if err != nil {
		log.Printf("Failed to create settings window: %v", err)
		return
	}

	// Load window icon from embedded icon.ico
	if iconBytes, err := iconData.ReadFile("icon.ico"); err == nil {
		tempDir := os.TempDir()
		iconPath := filepath.Join(tempDir, "vwork-connector-icon.ico")
		if err := os.WriteFile(iconPath, iconBytes, 0644); err == nil {
			if icon, err := walk.NewIconFromFile(iconPath); err == nil {
				settingsWindow.SetIcon(icon)
			} else {
				log.Printf("Failed to load icon: %v", err)
			}
		} else {
			log.Printf("Failed to write icon file: %v", err)
		}
	} else {
		log.Printf("Failed to read embedded icon: %v", err)
	}

	// Initialize terminal list
	updateTerminalsList()

	// Cleanup on window close
	settingsWindow.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		settingsWindow = nil
	})

	settingsWindow.Run()
}
