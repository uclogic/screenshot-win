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
	textSettings                   = "settings"
	textGeneral                    = "general"
	textAdvanced                   = "advanced"
	textKeyboardShortcut           = "keyboard_shortcut"
	textStartCapture               = "start_capture"
	textStartCaptureLabel          = "start_capture_label"
	textHotkeyHelp                 = "hotkey_help"
	textLanguageLabel              = "language_label"
	textScrollingMatching          = "scrolling_matching"
	textCaptureInterval            = "capture_interval"
	textMaxScrollRatio             = "max_scroll_ratio"
	textMaxMeanDifference          = "max_mean_difference"
	textMinConfidence              = "min_confidence"
	textStationaryThreshold        = "stationary_threshold"
	textDiagnostics                = "diagnostics"
	textSaveDiagnostics            = "save_diagnostics"
	textDirectory                  = "directory"
	textBrowse                     = "browse"
	textRejectedFrameLimit         = "rejected_frame_limit"
	textOK                         = "ok"
	textCancel                     = "cancel"
	textApply                      = "apply"
	textOverrideNote               = "override_note"
	textSelectDiagnosticsDirectory = "select_diagnostics_directory"
	textAlreadyRunning             = "already_running"
	textSettingsMenu               = "settings_menu"
	textExit                       = "exit"
	textPNGFilter                  = "png_filter"
	textSaveScreenshot             = "save_screenshot"
)

var catalogs = map[string]map[string]string{
	languageEnglish: {
		textSettings: "Settings", textGeneral: "General", textAdvanced: "Advanced", textKeyboardShortcut: "Keyboard shortcut",
		textStartCapture: "Start capture", textStartCaptureLabel: "Start capture:", textHotkeyHelp: "Combine Ctrl, Alt, or Shift with one regular key.", textLanguageLabel: "Language:",
		textScrollingMatching: "Scrolling capture matching", textCaptureInterval: "Capture interval (ms):", textMaxScrollRatio: "Maximum scroll ratio:",
		textMaxMeanDifference: "Maximum mean difference:", textMinConfidence: "Minimum confidence:", textStationaryThreshold: "Stationary threshold:",
		textDiagnostics: "Diagnostics", textSaveDiagnostics: "Save diagnostic data", textDirectory: "Directory:", textBrowse: "Browse…",
		textRejectedFrameLimit: "Rejected frame pair limit:", textOK: "OK", textCancel: "Cancel", textApply: "Apply",
		textOverrideNote:               "Some settings are overridden by command-line options; saved values apply on the next normal launch.",
		textSelectDiagnosticsDirectory: "Select diagnostics directory", textAlreadyRunning: "screenshot-win is already running in the notification area.",
		textSettingsMenu: "Settings…", textExit: "Exit", textPNGFilter: "PNG image (*.png)\x00*.png\x00All files (*.*)\x00*.*\x00\x00", textSaveScreenshot: "Save screenshot",
	},
	languageChinese: {
		textSettings: "设置", textGeneral: "常规", textAdvanced: "高级", textKeyboardShortcut: "快捷键", textStartCapture: "开始截图",
		textStartCaptureLabel: "开始截图：", textHotkeyHelp: "请使用 Ctrl、Alt 或 Shift 与一个普通按键组合。", textLanguageLabel: "语言：",
		textScrollingMatching: "长截图匹配", textCaptureInterval: "截图间隔（毫秒）：", textMaxScrollRatio: "最大滚动比例：",
		textMaxMeanDifference: "最大平均差异：", textMinConfidence: "最小置信度：", textStationaryThreshold: "静止判定阈值：",
		textDiagnostics: "诊断", textSaveDiagnostics: "保存诊断数据", textDirectory: "目录：", textBrowse: "浏览…",
		textRejectedFrameLimit: "拒绝帧对保存上限：", textOK: "确定", textCancel: "取消", textApply: "应用",
		textOverrideNote: "部分设置当前由命令行覆盖；保存值将在下次普通启动时生效。", textSelectDiagnosticsDirectory: "选择诊断数据目录",
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
