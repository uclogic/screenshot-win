package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	application "screenshot-win/app"
)

func TestDefaultPreferencesAreValid(t *testing.T) {
	value := defaultPreferences()
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if value.General.Hotkey != "Alt+Shift+A" || value.General.Language != languageEnglish || value.LongCapture.IntervalMS != 100 || value.Diagnostics.Limit != 50 {
		t.Fatalf("unexpected defaults: %+v", value)
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), settingsFileName)
	want := defaultPreferences()
	want.General.Hotkey = "Ctrl+Alt+F8"
	want.General.Language = languageChinese
	want.LongCapture.IntervalMS = 225
	want.LongCapture.MaxScrollRatio = 0.7
	want.Diagnostics.Enabled = true
	want.Diagnostics.Directory = "debug-data"
	want.Diagnostics.Limit = 12
	if err := savePreferences(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("saved settings file is empty")
	}
}

func TestSavePreferencesReportsUnwritableDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", settingsFileName)
	if err := savePreferences(path, defaultPreferences()); err == nil {
		t.Fatal("savePreferences succeeded for a missing destination directory")
	}
}

func TestLoadPreferencesMergesMissingFieldsWithDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), settingsFileName)
	if err := os.WriteFile(path, []byte("[long_capture]\ninterval_ms = 250\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultPreferences()
	if got.LongCapture.IntervalMS != 250 || got.General != want.General || got.LongCapture.MaxScrollRatio != want.LongCapture.MaxScrollRatio || got.Diagnostics != want.Diagnostics {
		t.Fatalf("partial settings did not retain defaults: %+v", got)
	}
}

func TestLoadPreferencesReturnsDefaultsAndErrorForInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), settingsFileName)
	if err := os.WriteFile(path, []byte("[long_capture\ninterval_ms = 0"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err == nil {
		t.Fatal("invalid TOML was accepted")
	}
	if got != defaultPreferences() {
		t.Fatalf("invalid TOML returned %+v, want defaults", got)
	}
}

func TestPreferencesValidateRejectsInvalidValues(t *testing.T) {
	tests := []preferences{
		func() preferences { value := defaultPreferences(); value.General.Language = "fr"; return value }(),
		func() preferences { value := defaultPreferences(); value.General.Hotkey = "A"; return value }(),
		func() preferences { value := defaultPreferences(); value.LongCapture.IntervalMS = 0; return value }(),
		func() preferences { value := defaultPreferences(); value.LongCapture.MaxScrollRatio = 1; return value }(),
		func() preferences {
			value := defaultPreferences()
			value.Diagnostics.Enabled = true
			value.Diagnostics.Directory = ""
			return value
		}(),
		func() preferences { value := defaultPreferences(); value.Diagnostics.Limit = -1; return value }(),
	}
	for _, value := range tests {
		if err := value.Validate(); err == nil {
			t.Errorf("Validate() accepted %+v", value)
		}
	}
}

func TestConfiguredHotkeyParsingAndFormatting(t *testing.T) {
	value, err := parseConfiguredHotkey("shift + ctrl + f12")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := formatConfiguredHotkey(value), "Ctrl+Shift+F12"; got != want {
		t.Fatalf("formatted hotkey = %q, want %q", got, want)
	}
	for _, invalid := range []string{"A", "Ctrl", "Ctrl+A+B", "Win+A", "Ctrl+NoSuchKey"} {
		if _, err := parseConfiguredHotkey(invalid); err == nil {
			t.Errorf("parseConfiguredHotkey(%q) succeeded", invalid)
		}
	}
}

func TestPreferencesApplyResolvesDiagnosticDirectory(t *testing.T) {
	value := defaultPreferences()
	value.LongCapture.IntervalMS = 275
	value.Diagnostics.Enabled = true
	value.Diagnostics.Directory = "diagnostics"
	config := value.apply(application.Config{}, filepath.Join("root", "app"))
	if config.Interval != 275*time.Millisecond || config.DiagnosticDir != filepath.Join("root", "app", "diagnostics") {
		t.Fatalf("applied config = %+v", config)
	}
	value.Diagnostics.Enabled = false
	if got := value.apply(config, "ignored").DiagnosticDir; got != "" {
		t.Fatalf("disabled diagnostic directory = %q", got)
	}
}

func TestSettingsPathUsesExecutableDirectory(t *testing.T) {
	got := settingsPathForExecutable(filepath.Join("opt", "screenshot-win", "screenshot-win.exe"))
	want := filepath.Join("opt", "screenshot-win", settingsFileName)
	if got != want {
		t.Fatalf("settings path = %q, want %q", got, want)
	}
}

func TestEveryLanguageCatalogContainsEnglishKeys(t *testing.T) {
	for _, language := range availableLanguages {
		catalog := catalogs[language.Code]
		if catalog == nil {
			t.Errorf("registered language %q has no catalog", language.Code)
			continue
		}
		for key := range catalogs[languageEnglish] {
			if catalog[key] == "" {
				t.Errorf("catalog %q is missing key %q", language.Code, key)
			}
		}
	}
}

func TestLocalizationFallsBackToEnglish(t *testing.T) {
	if got, want := localize("unknown", textSettings), "Settings"; got != want {
		t.Fatalf("fallback = %q, want %q", got, want)
	}
}
