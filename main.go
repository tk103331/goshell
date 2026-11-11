package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// iconPNG is the app's icon, shown in the Dock (macOS) and the taskbar
// (Windows, Linux) of development runs; packaged apps take it from their
// bundle, which mygo build fills with resources/icon.png.
//
//go:embed resources/icon.png
var iconPNG []byte

func main() {
	mygo.App.SetName(APP_NAME)
	mygo.App.WhenReady(func() {
		w := newWindow()
		if err := w.load(); err != nil {
			log.Println(err)
		}
		w.win = mygo.NewWindow(mygo.WindowOptions{
			Title:        APP_NAME,
			Width:        1024,
			Height:       720,
			StateKey:     "main",
			Frameless:    true, // the view draws the whole title bar, controls included
			CornerRadius: 10,   // macOS rounds the corners of a frameless window
			Content:      ui.View(w.view),
		})
		close(w.ready)
		w.win.OnClosed(func() { w.closeAll() })

		setMenus(w)
		// the icon of development runs; packaged apps have theirs in the
		// bundle already
		if err := mygo.App.Dock.SetIcon(iconPNG); err != nil {
			log.Println(err)
		}
		if err := w.win.SetIcon(iconPNG); err != nil {
			log.Println(err)
		}
		// the language changed (View > Language, or the user's languages):
		// say the menus in it again
		mygo.App.OnLocaleChanged(func(string) { setMenus(w) })
		// the system's appearance changed: rebuild the view, which the
		// System appearance follows
		mygo.Theme.OnUpdated(func() { w.onMain(func() {}) })

		w.openLocal()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// setMenus builds the app's menu bar in the language the app shows: File
// for what the toolbar does, with keyboard shortcuts, Edit with the
// standard roles, View with the appearance, the language, the terminal
// theme and Find, and Help with About.
func setMenus(w *Window) {
	appearance := []*mygo.MenuItem{}
	for _, it := range []struct{ kind, label string }{
		{"", locales.MenuFollowSystem()},
		{"light", locales.MenuLight()},
		{"dark", locales.MenuDark()},
	} {
		it := it
		item := &mygo.MenuItem{
			ID:      "appearance-" + it.kind,
			Type:    mygo.MenuItemRadio,
			Label:   it.label,
			Checked: w.appTheme == it.kind,
			Click: func(item *mygo.MenuItem, _ *mygo.Window) {
				w.setAppearance(it.kind)
			},
		}
		w.appearanceItems[it.kind] = item
		appearance = append(appearance, item)
	}

	// the languages the app ships: one that follows the user's languages,
	// then each named in itself ("English", "简体中文")
	chosen := mygo.App.ChosenLocale()
	language := []*mygo.MenuItem{{
		Label:   locales.MenuFollowSystem(),
		Type:    mygo.MenuItemRadio,
		Checked: chosen == "",
		Click: func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.SetLocale("")
		},
	}}
	for _, tag := range mygo.App.Languages() {
		tag := tag
		language = append(language, &mygo.MenuItem{
			Label:   i18n.Autonym(tag),
			Type:    mygo.MenuItemRadio,
			Checked: chosen == tag,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				mygo.App.SetLocale(tag)
			},
		})
	}

	view := []*mygo.MenuItem{
		{Label: locales.MenuAppearance(), Submenu: appearance},
		{Label: locales.MenuLanguage(), Submenu: language},
		{Label: locales.MenuTerminalTheme(), Click: func(*mygo.MenuItem, *mygo.Window) {
			w.openThemeDialog()
		}},
		mygo.Separator(),
		{Label: locales.MenuToggleSidebar(), Accelerator: "CmdOrCtrl+B", Click: func(*mygo.MenuItem, *mygo.Window) {
			w.toggleSidebar()
		}},
		mygo.Separator(),
		{Label: locales.MenuFind(), Accelerator: "CmdOrCtrl+F", Click: func(*mygo.MenuItem, *mygo.Window) {
			w.toggleFind()
		}},
	}

	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: locales.MenuFile(), Submenu: []*mygo.MenuItem{
			{Label: locales.MenuNewShell(), Accelerator: "CmdOrCtrl+T", Click: func(*mygo.MenuItem, *mygo.Window) {
				w.openLocal()
			}},
			{Label: locales.MenuNewConfig(), Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) {
				w.openCreate()
			}},
			{Label: locales.MenuNewCommand(), Accelerator: "CmdOrCtrl+E", Click: func(*mygo.MenuItem, *mygo.Window) {
				w.openNewCmd()
			}},
			mygo.Separator(),
			{Label: locales.MenuCloseTab(), Accelerator: "CmdOrCtrl+W", Click: func(*mygo.MenuItem, *mygo.Window) {
				w.closeSelected()
			}},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: locales.MenuView(), Submenu: view},
		{Label: locales.MenuHelp(), Submenu: []*mygo.MenuItem{
			{Label: locales.MenuAbout(), Click: func(*mygo.MenuItem, *mygo.Window) {
				w.aboutOpen = true
			}},
		}},
	})
	mygo.App.SetMenu(menu)
	w.win.SetMenu(menu)
}
