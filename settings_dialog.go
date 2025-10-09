package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// showSettingsDialog 显示设置对话框
func (w *Window) showSettingsDialog() {
	// 加载当前设置
	currentSettings := w.LoadSettings()

	// 主题选择
	themeSelect := widget.NewSelect([]string{"Dark", "Light", "System"}, func(selected string) {
		switch selected {
		case "Dark":
			currentSettings.ThemeType = ThemeTypeDark
		case "Light":
			currentSettings.ThemeType = ThemeTypeLight
		case "System":
			currentSettings.ThemeType = ThemeTypeSystem
		}
	})

	// 设置当前选中的主题
	switch currentSettings.ThemeType {
	case ThemeTypeDark:
		themeSelect.SetSelected("Dark")
	case ThemeTypeLight:
		themeSelect.SetSelected("Light")
	case ThemeTypeSystem:
		themeSelect.SetSelected("System")
	default:
		themeSelect.SetSelected("Dark")
	}

	// 字体名称设置
	fontNameEntry := widget.NewEntry()
	fontNameEntry.SetPlaceHolder("Font name (leave empty for default)")
	fontNameEntry.SetText(currentSettings.FontName)
	fontNameEntry.OnChanged = func(text string) {
		currentSettings.FontName = text
	}

	// 字体大小设置
	fontSizeSlider := widget.NewSlider(8, 24)
	fontSizeSlider.SetValue(float64(currentSettings.FontSize))

	fontSizeValueLabel := widget.NewLabel("Default")
	if currentSettings.FontSize > 0 {
		fontSizeValueLabel.SetText(fmt.Sprintf("%.0f", currentSettings.FontSize))
	}

	// 更新字体大小显示
	fontSizeSlider.OnChanged = func(value float64) {
		currentSettings.FontSize = float32(value)
		if value > 0 {
			fontSizeValueLabel.SetText(fmt.Sprintf("%.0f", value))
		} else {
			fontSizeValueLabel.SetText("Default")
		}
	}

	// 创建字体大小滑块布局，使其更宽更易操作
	fontSizeContainer := container.NewBorder(
		nil, // top
		nil, // bottom
		widget.NewLabel("Font Size:"), // left
		fontSizeValueLabel, // right
		fontSizeSlider, // center
	)

	// 重置按钮
	resetButton := widget.NewButton("Reset to Defaults", func() {
		defaultSettings := DefaultSettings()
		themeSelect.SetSelected("Dark")
		fontNameEntry.SetText("")
		fontSizeSlider.SetValue(0)
		fontSizeValueLabel.SetText("Default")

		// 更新设置对象
		currentSettings.ThemeType = defaultSettings.ThemeType
		currentSettings.FontName = defaultSettings.FontName
		currentSettings.FontSize = defaultSettings.FontSize
	})

	// 设置内容
	content := container.NewVBox(
		widget.NewCard("", "Theme Settings", widget.NewForm(
			widget.NewFormItem("Theme", themeSelect),
		)),

		widget.NewCard("", "Font Settings", container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Font Name", fontNameEntry),
			),
			fontSizeContainer,
		)),

		container.NewHBox(
			layout.NewSpacer(),
			resetButton,
		),
	)

	// 创建对话框
	dlg := dialog.NewCustomConfirm("Settings", "Apply", "Cancel", content, func(apply bool) {
		if apply {
			// 保存并应用设置
			w.SaveSettings(currentSettings)
			w.ApplySettings(currentSettings)
		}
	}, w.win)

	dlg.Resize(fyne.NewSize(500, 350))
	dlg.Show()
}