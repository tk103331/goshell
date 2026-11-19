package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// openNewCmd opens the dialog that creates a shortcut command.
func (w *Window) openNewCmd() {
	w.cmdEdit = -1
	w.cmdName, w.cmdText, w.cmdIcon, w.cmdAutoSubmit = "", "", "terminal", false
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
	lk := lookFor(c)
	ui.Modal(c, &w.cmdOpen, func() {
		title := locales.NewCommand()
		if w.cmdEdit >= 0 {
			title = locales.EditCommand()
		}
		ui.Column(c).Width(520).Padding(20).Gap(14).Children(func() {
			dialogHeading(c, title)

			// the chip this command makes in the bar under the terminal
			ui.Row(c).FillWidth().Height(38).Padding(0, 10).Gap(6).Radius(8).
				Background(lk.chip).Children(func() {
				if icon := cmdIcon(w.cmdIcon); icon != nil {
					ui.Icon(c, icon).Size(14, 14).TextColor(lk.textMuted)
				}
				name := w.cmdName
				fg := lk.text
				if name == "" {
					name, fg = locales.CmdPreviewPlaceholder(), lk.textFaint
				}
				ui.Text(c, name).FontSize(12.5).TextColor(fg).Grow(1).SingleLine().Ellipsis("…")
				if w.cmdAutoSubmit {
					ui.Text(c, "⏎").FontSize(11).TextColor(lk.textMuted).Padding(1, 5).Radius(4).Background(lk.card)
				}
			})

			ui.Form(c, func() {
				ui.Field(c, locales.FieldName(), func() { ui.TextInput(c, &w.cmdName) })
				ui.Field(c, locales.FieldCmdText(), func() { ui.TextArea(c, &w.cmdText).Lines(4, 8) })
				ui.Field(c, locales.FieldCmdIcon(), func() {
					ui.Row(c).Gap(4).Wrap().Children(func() {
						for _, name := range w.cmdIcons {
							name := name
							selected := name == w.cmdIcon
							b := ui.ButtonBase(c).Size(30, 30).Radius(6).Center().
								Label(iconTitle(name)).Border(1, ui.Transparent)
							if selected {
								b.Background(lk.accentSoft)
								b.BorderColor(lk.accent)
							} else if b.Hovered() {
								b.Background(lk.rowHover)
							}
							b.Children(func() {
								fg := lk.textMuted
								if selected {
									fg = lk.accent
								}
								ui.Icon(c, cmdIcon(name)).Size(15, 15).TextColor(fg)
							})
							b.Tooltip(iconTitle(name))
							if b.Clicked() {
								w.cmdIcon = name
							}
						}
					})
				})
				ui.Field(c, locales.FieldAfterSend(), func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Switch(c, &w.cmdAutoSubmit)
						ui.Text(c, locales.CmdAutoSubmit()).FontSize(12).TextColor(lk.textMuted).SingleLine()
					})
				})
			})

			ui.Text(c, locales.CmdHint()).FontSize(11.5).TextColor(lk.textFaint).SingleLine()
			if w.cmdName == "" || w.cmdText == "" {
				ui.Text(c, locales.CmdRequired()).FontSize(12.5).TextColor(lk.danger)
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

// iconTitle names a command icon for its tooltip.
func iconTitle(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
