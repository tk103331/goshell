package main

import (
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// openThemeDialog opens the dialog that picks a terminal theme among the
// ones Ghostty ships and the user's own.
func (w *Window) openThemeDialog() {
	if w.themeNames == nil {
		w.themeNames = terminal.GhosttyThemes()
	}
	w.themeQuery = ""
	w.themeOpen = true
}

func (w *Window) themeDialog(c *ui.Context) {
	ui.Modal(c, &w.themeOpen, func() {
		ui.Column(c).Width(460).Height(520).Padding(16).Gap(12).Children(func() {
			ui.Text(c, locales.ThemeTitle())
			ui.SearchField(c, &w.themeQuery).Label(locales.ThemeSearch())

			ui.Scroll(c).Fill().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					shown := w.filteredThemeNames()
					if len(shown) == 0 {
						ui.Text(c, locales.ThemeNone()).TextColor(c.Theme().TextMuted)
					}
					for _, name := range shown {
						name := name
						if ui.Button(c, "").Children(func() {
							ui.Row(c).FillWidth().Gap(6).Children(func() {
								ui.Text(c, name).Grow(1).SingleLine()
								if w.termThemeName == name {
									ui.Icon(c, iconCheck)
								}
							})
						}).Clicked() {
							w.applyTermTheme(name)
							w.themeOpen = false
						}
					}
				})
			})

			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.ThemeDefault()).Clicked() {
					w.applyTermTheme("")
					w.themeOpen = false
				}
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					w.themeOpen = false
				}
			})
		})
	})
}

// filteredThemeNames narrows the theme names to what the search field asks
// for; with no query it shows the first ones only, for the view to stay fast.
func (w *Window) filteredThemeNames() []string {
	q := strings.ToLower(strings.TrimSpace(w.themeQuery))
	var names []string
	for _, name := range w.themeNames {
		if q == "" && len(names) >= 60 {
			break
		}
		if q != "" && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		names = append(names, name)
	}
	return names
}

// applyTermTheme recolors every open terminal with a Ghostty theme, or back
// to following the window's appearance when the name is empty, and keeps
// new terminals to it.
func (w *Window) applyTermTheme(name string) {
	if name == w.termThemeName {
		return
	}
	w.termThemeName = name
	var t *terminal.Theme
	if name != "" {
		parsed, err := terminal.GhosttyTheme(name)
		if err != nil {
			w.showError(err)
			return
		}
		t = parsed
	}
	w.termTheme = t
	for _, tab := range w.tabs {
		tab.term.UseTheme(t)
	}
	w.save()
}
