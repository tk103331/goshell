package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// migrateUserData copies the settings of the app's old "Go Shell" name into
// the "GoShell" directory the app now uses. Files already in the new
// directory are left alone; the old directory stays as a fallback.
func migrateUserData() {
	config, err := os.UserConfigDir()
	if err != nil {
		return
	}
	from := filepath.Join(config, "Go Shell")
	if _, err := os.Stat(from); err != nil {
		return
	}
	to, err := mygo.App.Path(mygo.PathUserData)
	if err != nil || to == from {
		return
	}
	for _, name := range []string{"goshell.json", "updater.json", "window-state.json"} {
		dst := filepath.Join(to, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(from, name))
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			log.Println("migrate", name+":", err)
		}
	}
}
