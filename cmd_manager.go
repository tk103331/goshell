package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showCmdManagerDialog 显示命令管理对话框
func (w *Window) showCmdManagerDialog() {
	// 创建命令列表
	cmdList := widget.NewList(func() int {
		return len(w.cmds)
	}, func() fyne.CanvasObject {
		return container.NewHBox(
			widget.NewLabel(""), // 命令名称
			layout.NewSpacer(),
			widget.NewButtonWithIcon("", theme.SettingsIcon(), nil), // 编辑按钮
			widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),   // 删除按钮
		)
	}, func(id widget.ListItemID, object fyne.CanvasObject) {
		box := object.(*fyne.Container)
		label := box.Objects[0].(*widget.Label)
		editBtn := box.Objects[2].(*widget.Button)
		deleteBtn := box.Objects[3].(*widget.Button)

		cmd := w.cmds[id]
		label.SetText(cmd.Name)

		editBtn.OnTapped = func() {
			w.showModifyCmdDialog(id, cmd)
		}

		deleteBtn.OnTapped = func() {
			dialog.NewConfirm("Delete Command",
				fmt.Sprintf("Are you sure you want to delete command '%s'?", cmd.Name),
				func(confirmed bool) {
					if confirmed {
						w.RemoveCmd(id)
					}
				}, w.win).Show()
		}
	})

	// 如果没有命令，显示提示信息
	var listContent fyne.CanvasObject
	if len(w.cmds) == 0 {
		noCommandsLabel := widget.NewLabel("No commands found. Click 'New Command' to create one.")
		noCommandsLabel.Alignment = fyne.TextAlignCenter
		listContent = container.NewVBox(
			container.NewCenter(noCommandsLabel),
		)
	} else {
		listContent = cmdList
	}

	// 创建按钮区域
	newCmdBtn := widget.NewButtonWithIcon("New Command", theme.ContentAddIcon(), func() {
		w.showNewCmdDialog()
	})

	// 创建内容容器
	content := container.NewVBox(
		widget.NewCard("", "Commands", listContent),
		newCmdBtn,
	)

	// 创建对话框，使用标准的确认对话框以便可以关闭
	dlg := dialog.NewCustom("Command Manager", "Close", content, w.win)
	dlg.Resize(fyne.NewSize(500, 400))
	dlg.Show()
}