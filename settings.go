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
	lk := lookFor(c)
	ui.Modal(c, &w.themeOpen, func() {
		ui.Column(c).Width(460).Height(520).Padding(20).Gap(14).Children(func() {
			dialogHeading(c, locales.ThemeTitle())
			ui.SearchField(c, &w.themeQuery).Label(locales.ThemeSearch())

			ui.Scroll(c).Fill().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					shown := w.filteredThemeNames()
					if len(shown) == 0 {
						ui.Text(c, locales.ThemeNone()).FontSize(12.5).TextColor(lk.textMuted).Padding(8, 10)
					}
					// the default comes first, then Ghostty's themes that match
					if w.themeQuery == "" {
						w.themeRow(c, lk, "", locales.ThemeDefault())
					}
					for _, name := range shown {
						w.themeRow(c, lk, name, name)
					}
				})
			})

			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
					w.themeOpen = false
				}
			})
		})
	})
}

// themeRow is one choice of the theme dialog: a quiet row that marks the
// theme in use and takes a click.
func (w *Window) themeRow(c *ui.Context, lk *look, name, label string) {
	b := ui.ButtonBase(c).FillWidth().Height(30).Padding(0, 8).Radius(6).Label(label)
	if b.Hovered() {
		b.Background(lk.rowHover)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(lk.text).Grow(1).SingleLine().Ellipsis("…")
		if w.termThemeName == name {
			ui.Icon(c, iconCheck).Size(14, 14).TextColor(lk.accent)
		}
	})
	if b.Clicked() {
		w.applyTermTheme(name)
		w.themeOpen = false
	}
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
		if t != nil {
			tab.term.SetTheme(t, nil)
		} else {
			// back to the card's colors, in each appearance
			tab.term.SetTheme(termLight, termDark)
		}
	}
	w.save()
}
