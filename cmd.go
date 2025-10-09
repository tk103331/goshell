package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (w *Window) showNewCmdDialog() {
	nameEntry := widget.NewEntry()
	textEntry := widget.NewEntry()
	textEntry.MultiLine = true

	// 创建排序后的图标列表
	icons := make([]string, 0, len(iconMap))
	for k := range iconMap {
		icons = append(icons, k)
	}
	iconSelect := widget.NewSelect(icons, nil)

	// 添加自动提交复选框
	autoSubmitCheck := widget.NewCheck("Auto Submit (press Enter automatically)", func(b bool) {})

	dlg := dialog.NewForm("New Command", "OK", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Text", textEntry),
		widget.NewFormItem("Icon", iconSelect),
		widget.NewFormItem("", autoSubmitCheck),
	}, func(b bool) {
		if b {
			cmd := &Cmd{
				Name:       nameEntry.Text,
				Text:       textEntry.Text,
				Icon:       iconSelect.Selected,
				AutoSubmit: autoSubmitCheck.Checked,
			}
			w.AddCmd(cmd)
		}
	}, w.win)

	dlg.Resize(fyne.NewSize(400, 350))
	dlg.Show()
}

// showModifyCmdDialog 显示修改命令对话框
func (w *Window) showModifyCmdDialog(index int, cmd *Cmd) {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(cmd.Name)
	textEntry := widget.NewEntry()
	textEntry.SetText(cmd.Text)
	textEntry.MultiLine = true

	// 创建排序后的图标列表
	icons := make([]string, 0, len(iconMap))
	for k := range iconMap {
		icons = append(icons, k)
	}
	iconSelect := widget.NewSelect(icons, nil)
	iconSelect.SetSelected(cmd.Icon)

	// 添加自动提交复选框
	autoSubmitCheck := widget.NewCheck("Auto Submit (press Enter automatically)", func(b bool) {})
	autoSubmitCheck.SetChecked(cmd.AutoSubmit)

	dlg := dialog.NewForm("Modify Command", "OK", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Text", textEntry),
		widget.NewFormItem("Icon", iconSelect),
		widget.NewFormItem("", autoSubmitCheck),
	}, func(b bool) {
		if b {
			updatedCmd := &Cmd{
				Name:       nameEntry.Text,
				Text:       textEntry.Text,
				Icon:       iconSelect.Selected,
				AutoSubmit: autoSubmitCheck.Checked,
			}
			w.UpdateCmd(index, updatedCmd)
		}
	}, w.win)

	dlg.Resize(fyne.NewSize(400, 350))
	dlg.Show()
}
