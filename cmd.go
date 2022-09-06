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

	icons := make([]string, len(iconMap))
	i := 0
	for k, _ := range iconMap {
		icons[i] = k
		i++
	}
	iconSelect := widget.NewSelectEntry(icons)

	dlg := dialog.NewForm("New Command", "OK", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Text", textEntry),
		widget.NewFormItem("Icon", iconSelect),
	}, func(b bool) {
		if b {
			cmd := &Cmd{Name: nameEntry.Text, Text: textEntry.Text, Icon: iconSelect.Text}
			w.AddCmd(cmd)
		}
	}, w.win)

	dlg.Resize(fyne.NewSize(400, 300))
	dlg.Show()
}
