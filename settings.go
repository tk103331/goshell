package main

import (
	"encoding/json"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// AppSettings 应用设置
type AppSettings struct {
	ThemeType string `json:"themeType"`
	FontName  string `json:"fontName"`
	FontSize  float32 `json:"fontSize"`
}

const (
	ThemeTypeDark  = "dark"
	ThemeTypeLight = "light"
	ThemeTypeSystem = "system"

	// 设置存储键
	APP_SETTINGS = "app_settings"
)

// DefaultSettings 返回默认设置
func DefaultSettings() *AppSettings {
	return &AppSettings{
		ThemeType: ThemeTypeDark,
		FontName:  "",
		FontSize:  0, // 0表示使用默认字体大小
	}
}

// GetTheme 根据设置获取主题
func (s *AppSettings) GetTheme() fyne.Theme {
	switch s.ThemeType {
	case ThemeTypeLight:
		return theme.LightTheme()
	case ThemeTypeDark:
		return theme.DarkTheme()
	default:
		return theme.DarkTheme()
	}
}

// SaveSettings 保存设置到应用偏好
func (w *Window) SaveSettings(settings *AppSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	w.app.Preferences().SetString(APP_SETTINGS, string(data))
	return nil
}

// LoadSettings 从应用偏好加载设置
func (w *Window) LoadSettings() *AppSettings {
	settingsJson := w.app.Preferences().String(APP_SETTINGS)
	if settingsJson == "" {
		return DefaultSettings()
	}

	var settings AppSettings
	err := json.Unmarshal([]byte(settingsJson), &settings)
	if err != nil {
		return DefaultSettings()
	}

	return &settings
}

// ApplySettings 应用设置
func (w *Window) ApplySettings(settings *AppSettings) {
	// 应用主题
	w.app.Settings().SetTheme(settings.GetTheme())

	// TODO: 应用字体设置需要进一步实现
	// Fyne目前不直接支持全局字体设置，但可以在控件级别设置
}