package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"screenshot-win"
	application "screenshot-win/app"
)

const settingsFileName = "screenshot-win.toml"

const (
	longCaptureModeBidirectional = "bidirectional"
	longCaptureModeLegacy        = "legacy"
)

type preferences struct {
	General     generalPreferences     `toml:"general"`
	LongCapture longCapturePreferences `toml:"long_capture"`
	Diagnostics diagnosticPreferences  `toml:"diagnostics"`
}

type generalPreferences struct {
	Hotkey   string `toml:"hotkey"`
	Language string `toml:"language"`
}

type longCapturePreferences struct {
	Mode                string  `toml:"mode"`
	IntervalMS          int     `toml:"interval_ms"`
	MaxScrollRatio      float64 `toml:"max_scroll_ratio"`
	MaxMeanDifference   float64 `toml:"max_mean_difference"`
	MinimumConfidence   float64 `toml:"minimum_confidence"`
	StationaryThreshold float64 `toml:"stationary_threshold"`
}

type diagnosticPreferences struct {
	Enabled   bool   `toml:"enabled"`
	Directory string `toml:"directory"`
	Limit     int    `toml:"limit"`
}

type configuredHotkey struct {
	Modifiers uint32
	Key       uint32
}

const (
	hotkeyModifierAlt     = 0x0001
	hotkeyModifierControl = 0x0002
	hotkeyModifierShift   = 0x0004
)

func defaultPreferences() preferences {
	match := screenshotwin.DefaultMatchOptions()
	return preferences{
		General: generalPreferences{Hotkey: "Alt+Shift+A", Language: languageEnglish},
		LongCapture: longCapturePreferences{
			Mode:                longCaptureModeLegacy,
			IntervalMS:          int(defaultCaptureInterval / time.Millisecond),
			MaxScrollRatio:      match.MaxOffsetRatio,
			MaxMeanDifference:   match.MaxMeanDifference,
			MinimumConfidence:   match.MinimumConfidence,
			StationaryThreshold: match.StationaryDifference,
		},
		Diagnostics: diagnosticPreferences{Directory: "diagnostics", Limit: 50},
	}
}

func settingsPathForExecutable(executable string) string {
	return filepath.Join(filepath.Dir(executable), settingsFileName)
}

func loadPreferences(path string) (preferences, error) {
	result := defaultPreferences()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return defaultPreferences(), fmt.Errorf("读取设置文件 %q：%w", path, err)
	}
	if err := toml.Unmarshal(data, &result); err != nil {
		return defaultPreferences(), fmt.Errorf("解析设置文件 %q：%w", path, err)
	}
	if err := result.Validate(); err != nil {
		return defaultPreferences(), fmt.Errorf("设置文件 %q 无效：%w", path, err)
	}
	result.General.Hotkey = mustFormatHotkey(result.General.Hotkey)
	return result, nil
}

func savePreferences(path string, value preferences) error {
	if err := value.Validate(); err != nil {
		return err
	}
	value.General.Hotkey = mustFormatHotkey(value.General.Hotkey)
	data, err := toml.Marshal(value)
	if err != nil {
		return fmt.Errorf("编码设置：%w", err)
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".screenshot-win-*.tmp")
	if err != nil {
		return fmt.Errorf("在程序目录创建临时设置文件：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("设置临时文件权限：%w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("写入设置：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("同步设置：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭设置：%w", err)
	}
	if err := replaceSettingsFile(temporaryPath, path); err != nil {
		return fmt.Errorf("替换设置文件：%w", err)
	}
	return nil
}

func (value preferences) Validate() error {
	if !supportedLanguage(value.General.Language) {
		codes := make([]string, 0, len(availableLanguages))
		for _, language := range availableLanguages {
			codes = append(codes, language.Code)
		}
		return fmt.Errorf("language must be one of: %s", strings.Join(codes, ", "))
	}
	if _, err := parseConfiguredHotkey(value.General.Hotkey); err != nil {
		return fmt.Errorf("截图快捷键：%w", err)
	}
	if value.LongCapture.Mode != longCaptureModeBidirectional && value.LongCapture.Mode != longCaptureModeLegacy {
		return fmt.Errorf("长截图模式必须是 %q 或 %q", longCaptureModeBidirectional, longCaptureModeLegacy)
	}
	if value.LongCapture.IntervalMS <= 0 || int64(value.LongCapture.IntervalMS) > math.MaxInt64/int64(time.Millisecond) {
		return fmt.Errorf("长截图间隔必须大于 0 毫秒")
	}
	match := screenshotwin.MatchOptions{
		MaxOffsetRatio:       value.LongCapture.MaxScrollRatio,
		MaxMeanDifference:    value.LongCapture.MaxMeanDifference,
		MinimumConfidence:    value.LongCapture.MinimumConfidence,
		StationaryDifference: value.LongCapture.StationaryThreshold,
	}
	if err := match.Validate(); err != nil {
		return fmt.Errorf("长截图匹配参数：%w", err)
	}
	if value.Diagnostics.Enabled && strings.TrimSpace(value.Diagnostics.Directory) == "" {
		return fmt.Errorf("启用诊断时目录不能为空")
	}
	if value.Diagnostics.Limit < 0 {
		return fmt.Errorf("诊断上限不能为负数")
	}
	return nil
}

func (value preferences) apply(config application.Config, programDirectory string) application.Config {
	config.LongCaptureImplementation = application.LongCaptureBidirectional
	if value.LongCapture.Mode == longCaptureModeLegacy {
		config.LongCaptureImplementation = application.LongCaptureLegacy
	}
	config.Interval = time.Duration(value.LongCapture.IntervalMS) * time.Millisecond
	config.MatchOptions = screenshotwin.MatchOptions{
		MaxOffsetRatio:       value.LongCapture.MaxScrollRatio,
		MaxMeanDifference:    value.LongCapture.MaxMeanDifference,
		MinimumConfidence:    value.LongCapture.MinimumConfidence,
		StationaryDifference: value.LongCapture.StationaryThreshold,
	}
	config.DiagnosticMax = value.Diagnostics.Limit
	config.DiagnosticDir = ""
	if value.Diagnostics.Enabled {
		config.DiagnosticDir = resolveSettingsPath(programDirectory, value.Diagnostics.Directory)
	}
	return config
}

func resolveSettingsPath(programDirectory, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(programDirectory, path)
}

func parseConfiguredHotkey(text string) (configuredHotkey, error) {
	var result configuredHotkey
	parts := strings.Split(text, "+")
	if len(parts) < 2 {
		return result, fmt.Errorf("必须包含 Ctrl、Alt 或 Shift 修饰键")
	}
	for _, raw := range parts {
		part := strings.ToUpper(strings.TrimSpace(raw))
		switch part {
		case "CTRL", "CONTROL":
			result.Modifiers |= hotkeyModifierControl
		case "ALT":
			result.Modifiers |= hotkeyModifierAlt
		case "SHIFT":
			result.Modifiers |= hotkeyModifierShift
		default:
			if result.Key != 0 {
				return configuredHotkey{}, fmt.Errorf("只能指定一个普通按键")
			}
			key, ok := hotkeyVirtualKey(part)
			if !ok {
				return configuredHotkey{}, fmt.Errorf("不支持按键 %q", strings.TrimSpace(raw))
			}
			result.Key = key
		}
	}
	if result.Modifiers == 0 || result.Key == 0 {
		return configuredHotkey{}, fmt.Errorf("必须包含修饰键和一个普通按键")
	}
	return result, nil
}

func hotkeyVirtualKey(name string) (uint32, bool) {
	if len(name) == 1 {
		value := name[0]
		if value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' {
			return uint32(value), true
		}
	}
	if strings.HasPrefix(name, "F") {
		number, err := strconv.Atoi(strings.TrimPrefix(name, "F"))
		if err == nil && number >= 1 && number <= 24 {
			return uint32(0x70 + number - 1), true
		}
	}
	if strings.HasPrefix(name, "VK_") {
		value, err := strconv.ParseUint(strings.TrimPrefix(name, "VK_"), 16, 8)
		if err == nil && value != 0 {
			return uint32(value), true
		}
	}
	keys := map[string]uint32{
		"BACKSPACE": 0x08, "TAB": 0x09, "ENTER": 0x0D, "ESCAPE": 0x1B, "SPACE": 0x20,
		"PAGEUP": 0x21, "PAGEDOWN": 0x22,
		"END": 0x23, "HOME": 0x24, "LEFT": 0x25, "UP": 0x26,
		"RIGHT": 0x27, "DOWN": 0x28, "INSERT": 0x2D, "DELETE": 0x2E,
	}
	value, ok := keys[name]
	return value, ok
}

func formatConfiguredHotkey(value configuredHotkey) string {
	parts := make([]string, 0, 4)
	if value.Modifiers&hotkeyModifierControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if value.Modifiers&hotkeyModifierAlt != 0 {
		parts = append(parts, "Alt")
	}
	if value.Modifiers&hotkeyModifierShift != 0 {
		parts = append(parts, "Shift")
	}
	parts = append(parts, hotkeyKeyName(value.Key))
	return strings.Join(parts, "+")
}

func hotkeyKeyName(key uint32) string {
	if key >= 'A' && key <= 'Z' || key >= '0' && key <= '9' {
		return string(rune(key))
	}
	if key >= 0x70 && key <= 0x87 {
		return fmt.Sprintf("F%d", key-0x70+1)
	}
	keys := map[uint32]string{
		0x08: "Backspace", 0x09: "Tab", 0x0D: "Enter", 0x1B: "Escape", 0x20: "Space",
		0x21: "PageUp", 0x22: "PageDown",
		0x23: "End", 0x24: "Home", 0x25: "Left", 0x26: "Up",
		0x27: "Right", 0x28: "Down", 0x2D: "Insert", 0x2E: "Delete",
	}
	if name := keys[key]; name != "" {
		return name
	}
	return fmt.Sprintf("VK_%02X", key)
}

func mustFormatHotkey(text string) string {
	value, _ := parseConfiguredHotkey(text)
	return formatConfiguredHotkey(value)
}
