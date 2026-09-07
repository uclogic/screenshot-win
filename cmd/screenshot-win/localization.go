package main

import "sync/atomic"

const (
	languageEnglish = "en"
	languageChinese = "zh-CN"
)

type languageDefinition struct{ Code, Name string }

// This registry drives both validation and the language selector.
var availableLanguages = []languageDefinition{
	{Code: languageEnglish, Name: "English"},
	{Code: languageChinese, Name: "简体中文"},
}

const (
	textSettings                 = "settings"
	textGeneral                  = "general"
	textAdvanced                 = "advanced"
	textKeyboardShortcut         = "keyboard_shortcut"
	textStartCapture             = "start_capture"
	textStartCaptureLabel        = "start_capture_label"
	textHotkeyHelp               = "hotkey_help"
	textPinClipboard             = "pin_clipboard"
	textClearHotkey              = "clear_hotkey"
	textLanguageLabel            = "language_label"
	textScrollingMatching        = "scrolling_matching"
	textLongCaptureMode          = "long_capture_mode"
	textLongCaptureBidirectional = "long_capture_bidirectional"
	textLongCaptureLegacy        = "long_capture_legacy"
	textCaptureInterval          = "capture_interval"
	textMaxScrollRatio           = "max_scroll_ratio"
	textMaxMeanDifference        = "max_mean_difference"
	textMinConfidence            = "min_confidence"
	textStationaryThreshold      = "stationary_threshold"
	textOK                       = "ok"
	textCancel                   = "cancel"
	textApply                    = "apply"
	textCandidateMode            = "candidate_mode"
	textCandidateWindowsUI       = "candidate_windows_ui"
	textAlreadyRunning           = "already_running"
	textSettingsMenu             = "settings_menu"
	textExit                     = "exit"
	textPNGFilter                = "png_filter"
	textSaveScreenshot           = "save_screenshot"
)

var catalogs = map[string]map[string]string{
	languageEnglish: {
		textSettings: "Settings", textGeneral: "General", textAdvanced: "Advanced", textKeyboardShortcut: "Keyboard shortcut",
		textStartCapture: "Start capture", textStartCaptureLabel: "Start capture:", textPinClipboard: "Pin clipboard", textClearHotkey: "Clear", textHotkeyHelp: "Ctrl / Alt / Shift + a key, or F1–F11. Both shortcuts are optional.", textLanguageLabel: "Language:",
		textScrollingMatching: "Scrolling capture matching", textLongCaptureMode: "Capture direction:", textLongCaptureBidirectional: "Bidirectional (up and down)", textLongCaptureLegacy: "One-way (down only, legacy)",
		textCaptureInterval: "Capture interval (ms):", textMaxScrollRatio: "Maximum scroll ratio:",
		textMaxMeanDifference: "Maximum mean difference:", textMinConfidence: "Minimum confidence:", textStationaryThreshold: "Stationary threshold:",

		textOK: "OK", textCancel: "Cancel", textApply: "Apply",
		textCandidateMode: "Candidate mode:", textCandidateWindowsUI: "windows ui interface (not implemented)",
		textAlreadyRunning: "screenshot-win is already running in the notification area.",
		textSettingsMenu:   "Settings…", textExit: "Exit", textPNGFilter: "PNG image (*.png)\x00*.png\x00All files (*.*)\x00*.*\x00\x00", textSaveScreenshot: "Save screenshot",
	},
	languageChinese: {
		textSettings: "设置", textGeneral: "常规", textAdvanced: "高级", textKeyboardShortcut: "快捷键", textStartCapture: "开始截图",
		textStartCaptureLabel: "开始截图：", textPinClipboard: "剪贴板贴图", textClearHotkey: "清除", textHotkeyHelp: "Ctrl / Alt / Shift + 按键，或单独 F1–F11。截图和贴图快捷键均可留空。", textLanguageLabel: "语言：",
		textScrollingMatching: "长截图匹配", textLongCaptureMode: "截图方向：", textLongCaptureBidirectional: "双向（向上和向下）", textLongCaptureLegacy: "单向（仅向下，旧版）",
		textCaptureInterval: "截图间隔（毫秒）：", textMaxScrollRatio: "最大滚动比例：",
		textMaxMeanDifference: "最大平均差异：", textMinConfidence: "最小置信度：", textStationaryThreshold: "静止判定阈值：",

		textOK: "确定", textCancel: "取消", textApply: "应用",
		textCandidateMode: "候选框实现：", textCandidateWindowsUI: "windows ui interface（未实现）",
		textAlreadyRunning: "screenshot-win 已在通知区域运行。", textSettingsMenu: "设置…", textExit: "退出",
		textPNGFilter: "PNG 图片 (*.png)\x00*.png\x00所有文件 (*.*)\x00*.*\x00\x00", textSaveScreenshot: "保存截图",
	},
}

var currentUILanguage atomic.Value

func init()                                  { currentUILanguage.Store(languageEnglish) }
func supportedLanguage(language string) bool { _, ok := catalogs[language]; return ok }
func setUILanguage(language string) {
	if supportedLanguage(language) {
		currentUILanguage.Store(language)
	}
}
func uiLanguage() string { return currentUILanguage.Load().(string) }

func localize(language, key string) string {
	if catalog := catalogs[language]; catalog != nil {
		if value, ok := catalog[key]; ok {
			return value
		}
	}
	return catalogs[languageEnglish][key]
}

func languageIndex(code string) int {
	for index, language := range availableLanguages {
		if language.Code == code {
			return index
		}
	}
	return 0
}
