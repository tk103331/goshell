package main

import "github.com/egoist/mygo/ui"

import "goshell/locales"

// openNewCmd opens the dialog that creates a shortcut command.
func (w *Window) openNewCmd() {
	w.cmdEdit = -1
	w.cmdName, w.cmdText, w.cmdIcon, w.cmdAutoSubmit = "", "", "", false
	w.cmdOpen = true
}

// editCmd opens the dialog that edits an existing command.
func (w *Window) editCmd(i int) {
	if i < 0 || i >= len(w.cmds) {
		return
	}
	w.cmdEdit = i
	w.cmdName, w.cmdText, w.cmdIcon = w.cmds[i].Name, w.cmds[i].Text, w.cmds[i].Icon
	w.cmdAutoSubmit = w.cmds[i].AutoSubmit
	w.cmdOpen = true
}

func (w *Window) addCmd(cmd *Cmd) {
	w.cmds = append(w.cmds, cmd)
	w.save()
}

func (w *Window) cmdDialog(c *ui.Context) {
	ui.Modal(c, &w.cmdOpen, func() {
		title := locales.NewCommand()
		if w.cmdEdit >= 0 {
			title = locales.EditCommand()
		}
		ui.Column(c).Width(460).Padding(16).Gap(12).Children(func() {
			ui.Text(c, title)
			ui.Form(c, func() {
				ui.Field(c, locales.FieldName(), func() { ui.TextInput(c, &w.cmdName) })
				ui.Field(c, locales.FieldCmdText(), func() { ui.TextArea(c, &w.cmdText).Lines(3, 6) })
				ui.Field(c, locales.FieldCmdIcon(), func() {
					ui.Combobox(c, &w.cmdIcon, w.cmdIcons)
				})
				ui.Field(c, locales.CmdAutoSubmit(), func() {
					ui.Checkbox(c, &w.cmdAutoSubmit, locales.CmdAutoSubmit())
				})
			})
			if w.cmdName == "" || w.cmdText == "" {
				ui.Text(c, locales.CmdRequired()).TextColor(c.Theme().TextMuted)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					w.cmdOpen = false
				}
				ok := ui.PrimaryButton(c, locales.BtnOK())
				if w.cmdName == "" || w.cmdText == "" {
					ok.Disabled(true)
				}
				if ok.Clicked() {
					cmd := &Cmd{
						Name:       w.cmdName,
						Text:       w.cmdText,
						Icon:       w.cmdIcon,
						AutoSubmit: w.cmdAutoSubmit,
					}
					if w.cmdEdit >= 0 && w.cmdEdit < len(w.cmds) {
						w.cmds[w.cmdEdit] = cmd
						w.save()
					} else {
						w.addCmd(cmd)
					}
					w.cmdOpen = false
				}
			})
		})
	})
}
