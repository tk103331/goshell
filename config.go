package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// Config is a kind of session the sidebar keeps: SSH, Docker or K8S.
type Config interface {
	Name() string
	Type() string
	Data() interface{}
	// Fields builds the fields of a config dialog, binding them to the
	// config's own data, which editing writes through at once. The window
	// serves dialogs the fields open, as a file picker.
	Fields(c *ui.Context, editing bool, w *Window)
	// Validate says what the dialog's fields still miss, "" when they are
	// enough to connect.
	Validate() string
	// Apply moves what the fields hold that is not a string, as the port
	// of SSH, and sets the type.
	Apply()
	// Prefill sets what the fields hold that is not a string from the
	// data, before a modify dialog shows.
	Prefill()
	Term(w *Window)
}

// openCreate opens the dialog that creates a config.
func (w *Window) openCreate() {
	w.pendingSSH = &SSHConfig{data: &SSHConfigData{Port: 22}, port: "22"}
	w.pendingDock = &DockerConfig{data: &DockerConfigData{}}
	w.pendingK8S = &K8SConfig{data: &K8SConfigData{}}
	w.confEditing = nil
	w.confKind = "SSH"
	w.confTitle = locales.CreateConfig()
	w.confOpen = true
}

// openModify opens the dialog that edits a config.
func (w *Window) openModify(conf Config) {
	conf.Prefill()
	w.confEditing = conf
	w.confTitle = locales.ModifyConfig(conf.Name())
	w.confOpen = true
}

func (w *Window) removeConfig(index int) {
	if index < 0 || index >= len(w.confs) {
		return
	}
	w.confs = append(w.confs[:index], w.confs[index+1:]...)
	w.save()
}

func (w *Window) configDialog(c *ui.Context) {
	ui.Modal(c, &w.confOpen, func() {
		ui.Column(c).Width(460).Padding(16).Gap(12).Children(func() {
			ui.Text(c, w.confTitle)

			var err string
			if w.confEditing != nil {
				w.confEditing.Fields(c, true, w)
				err = w.confEditing.Validate()
			} else {
				ui.Field(c, locales.FieldType(), func() {
					ui.Select(c, &w.confKind, []string{"SSH", "Docker", "K8S"})
				})
				switch w.confKind {
				case "SSH":
					w.pendingSSH.Fields(c, false, w)
					err = w.pendingSSH.Validate()
				case "Docker":
					w.pendingDock.Fields(c, false, w)
					err = w.pendingDock.Validate()
				case "K8S":
					w.pendingK8S.Fields(c, false, w)
					err = w.pendingK8S.Validate()
				}
			}

			if err != "" {
				ui.Text(c, err).TextColor(c.Theme().Danger)
			}

			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				cancel := ui.Button(c, locales.BtnCancel())
				if cancel.Clicked() {
					w.confOpen = false
				}
				ok := ui.PrimaryButton(c, locales.BtnOK())
				if err != "" {
					ok.Disabled(true)
				}
				if ok.Clicked() {
					w.confOk()
				}
			})
		})
	})
}

func (w *Window) confOk() {
	var cfg Config
	if w.confEditing != nil {
		cfg = w.confEditing
	} else {
		switch w.confKind {
		case "SSH":
			cfg = w.pendingSSH
		case "Docker":
			cfg = w.pendingDock
		case "K8S":
			cfg = w.pendingK8S
		default:
			return
		}
	}
	cfg.Apply()
	if w.confEditing == nil {
		w.confs = append(w.confs, cfg)
	}
	w.confOpen = false
	w.save()
}

// appFile is what load and save keep in the app's JSON file.
type appFile struct {
	Sessions      []json.RawMessage `json:"sessions"`
	Commands      []*Cmd            `json:"commands"`
	Appearance    string            `json:"appearance,omitempty"`
	TerminalTheme string            `json:"terminalTheme,omitempty"`
	SidebarHidden bool              `json:"sidebarHidden,omitempty"`
}

func (w *Window) load() error {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	w.path = filepath.Join(dir, "goshell.json")

	data, err := os.ReadFile(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var f appFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	for _, raw := range f.Sessions {
		var probe map[string]interface{}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		t, _ := probe["type"].(string)
		switch t {
		case "ssh":
			cfg := &SSHConfig{data: &SSHConfigData{}}
			if json.Unmarshal(raw, cfg.data) == nil {
				w.confs = append(w.confs, cfg)
			}
		case "docker":
			cfg := &DockerConfig{data: &DockerConfigData{}}
			if json.Unmarshal(raw, cfg.data) == nil {
				w.confs = append(w.confs, cfg)
			}
		case "k8s":
			cfg := &K8SConfig{data: &K8SConfigData{}}
			if json.Unmarshal(raw, cfg.data) == nil {
				w.confs = append(w.confs, cfg)
			}
		}
	}
	w.cmds = f.Commands
	w.appTheme = f.Appearance
	w.termThemeName = f.TerminalTheme
	w.showSidebar = !f.SidebarHidden
	if name := f.TerminalTheme; name != "" {
		if t, err := terminal.GhosttyTheme(name); err == nil {
			w.termTheme = t
		} else {
			log.Println("terminal theme:", err)
		}
	}
	return nil
}

func (w *Window) save() {
	if w.path == "" {
		return
	}
	f := appFile{
		Commands:      w.cmds,
		Appearance:    w.appTheme,
		TerminalTheme: w.termThemeName,
		SidebarHidden: !w.showSidebar,
	}
	for _, conf := range w.confs {
		data, err := json.Marshal(conf.Data())
		if err != nil {
			log.Println(err)
			continue
		}
		f.Sessions = append(f.Sessions, json.RawMessage(data))
	}
	data, err := json.Marshal(f)
	if err != nil {
		log.Println(err)
		return
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Println(err)
			return
		}
	}
	if err := os.WriteFile(w.path, data, 0o600); err != nil {
		log.Println(err)
	}
}
