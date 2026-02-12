package main

import (
	"testing"
)

// TestDefaultSettings 测试默认设置
func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()

	if settings.ThemeType != ThemeTypeDark {
		t.Errorf("Expected default theme type to be %s, got %s", ThemeTypeDark, settings.ThemeType)
	}

	if settings.FontSize != 0 {
		t.Errorf("Expected default font size to be 0, got %f", settings.FontSize)
	}

	if settings.CustomColors == nil {
		t.Error("Expected default custom colors to be initialized")
	}

	if settings.Shortcuts == nil {
		t.Error("Expected default shortcuts to be initialized")
	}

	if settings.UIPreferences.ShowCmdBar != true {
		t.Error("Expected default show command bar to be true")
	}
}

// TestCustomColors 测试自定义颜色功能
func TestCustomColors(t *testing.T) {
	settings := DefaultSettings()

	// 测试设置自定义颜色
	settings.SetCustomColor(ColorPrimary, "#FF0000")
	if settings.CustomColors[ColorPrimary] != "#FF0000" {
		t.Errorf("Expected primary color to be #FF0000, got %s", settings.CustomColors[ColorPrimary])
	}

	// 测试获取颜色
	primaryColor := settings.GetColor(ColorPrimary)
	if primaryColor == nil {
		t.Error("Expected primary color to be returned")
	}

	// 验证颜色解析
	red, _, _, _ := primaryColor.RGBA()
	if red != 0xff<<8 {
		t.Errorf("Expected red component to be 255, got %d", red>>8)
	}
}

// TestShortcuts 测试快捷键功能
func TestShortcuts(t *testing.T) {
	settings := DefaultSettings()

	// 测试设置快捷键
	settings.SetShortcut(ShortcutNewTab, "Ctrl+N")
	if settings.Shortcuts[ShortcutNewTab] != "Ctrl+N" {
		t.Errorf("Expected new tab shortcut to be Ctrl+N, got %s", settings.Shortcuts[ShortcutNewTab])
	}

	// 测试获取快捷键
	shortcut := settings.GetShortcut(ShortcutNewTab)
	if shortcut != "Ctrl+N" {
		t.Errorf("Expected new tab shortcut to be Ctrl+N, got %s", shortcut)
	}

	// 测试获取默认快捷键
	defaultShortcut := settings.GetShortcut("nonexistent")
	if defaultShortcut == "" {
		t.Error("Expected default shortcut to be returned for nonexistent action")
	}
}

// TestThemeTypes 测试主题类型
func TestThemeTypes(t *testing.T) {
	settings := &AppSettings{}

	// 测试暗色主题
	settings.ThemeType = ThemeTypeDark
	theme := settings.GetTheme()
	if theme == nil {
		t.Error("Expected dark theme to be returned")
	}

	// 测试亮色主题
	settings.ThemeType = ThemeTypeLight
	theme = settings.GetTheme()
	if theme == nil {
		t.Error("Expected light theme to be returned")
	}

	// 测试自定义主题
	settings.ThemeType = ThemeTypeCustom
	settings.CustomColors = map[string]string{
		ColorPrimary:    "#FF0000",
		ColorBackground: "#000000",
	}
	theme = settings.GetTheme()
	if theme == nil {
		t.Error("Expected custom theme to be returned")
	}
}

// TestPresetThemes 测试预设主题
func TestPresetThemes(t *testing.T) {
	// 测试获取预设主题列表
	themes := ListPresetThemes()
	if len(themes) == 0 {
		t.Error("Expected preset themes list to not be empty")
	}

	// 测试获取特定预设主题
	darkBlueTheme := GetPresetTheme("dark_blue")
	if darkBlueTheme == nil {
		t.Error("Expected dark blue theme to be returned")
	}

	if darkBlueTheme[ColorPrimary] != DarkBlueThemeColors[ColorPrimary] {
		t.Error("Expected dark blue theme primary color to match predefined color")
	}
}

// TestLayoutPresets 测试布局预设
func TestLayoutPresets(t *testing.T) {
	presets := GetLayoutPresets()
	if len(presets) == 0 {
		t.Error("Expected layout presets to not be empty")
	}

	// 测试紧凑布局预设
	compactPreset, exists := presets["compact"]
	if !exists {
		t.Error("Expected compact preset to exist")
	}

	if compactPreset.ShowCmdBar != false {
		t.Error("Expected compact preset to have command bar disabled")
	}
}

// TestValidateShortcut 测试快捷键验证
func TestValidateShortcut(t *testing.T) {
	// 测试有效快捷键
	if !ValidateShortcut("Ctrl+N") {
		t.Error("Expected Ctrl+N to be a valid shortcut")
	}

	if !ValidateShortcut("Alt+F4") {
		t.Error("Expected Alt+F4 to be a valid shortcut")
	}

	if !ValidateShortcut("Shift+Ctrl+S") {
		t.Error("Expected Shift+Ctrl+S to be a valid shortcut")
	}

	// 测试无效快捷键
	if ValidateShortcut("") {
		t.Error("Expected empty string to be an invalid shortcut")
	}

	if ValidateShortcut("N") {
		t.Error("Expected single key without modifier to be invalid")
	}

	if ValidateShortcut("Ctrl+") {
		t.Error("Expected incomplete shortcut to be invalid")
	}
}

// TestParseHexColor 测试十六进制颜色解析
func TestParseHexColor(t *testing.T) {
	// 测试有效的6位十六进制颜色
	c, err := parseHexColor("#FF0000")
	if err != nil {
		t.Errorf("Expected no error parsing #FF0000, got %v", err)
	}

	red, green, blue, alpha := c.RGBA()
	if red != 0xff<<8 || green != 0 || blue != 0 || alpha != 0xff<<8 {
		t.Errorf("Expected RGBA(255, 0, 0, 255), got RGBA(%d, %d, %d, %d)",
			red>>8, green>>8, blue>>8, alpha>>8)
	}

	// 测试有效的8位十六进制颜色
	c, err = parseHexColor("#FF000080")
	if err != nil {
		t.Errorf("Expected no error parsing #FF000080, got %v", err)
	}

	_, _, _, alpha = c.RGBA()
	if alpha != 0x80<<8 {
		t.Errorf("Expected alpha 128, got %d", alpha>>8)
	}

	// 测试无效的颜色格式
	_, err = parseHexColor("FF0000") // 缺少#
	if err == nil {
		t.Error("Expected error for color without #")
	}

	_, err = parseHexColor("#GGGGGG") // 无效字符
	if err == nil {
		t.Error("Expected error for invalid hex characters")
	}
}

// BenchmarkGetColor 基准测试获取颜色性能
func BenchmarkGetColor(b *testing.B) {
	settings := DefaultSettings()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = settings.GetColor(ColorPrimary)
	}
}

// BenchmarkGetShortcut 基准测试获取快捷键性能
func BenchmarkGetShortcut(b *testing.B) {
	settings := DefaultSettings()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = settings.GetShortcut(ShortcutNewTab)
	}
}
