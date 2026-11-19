package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"goshell/locales"
)

// Config is a host the sidebar keeps: an SSH server, a Docker daemon or a
// Kubernetes cluster. Opening one starts a terminal session in a tab; Docker
// and Kubernetes ask which container first.
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
	kubeconfig := defaultKubeconfig()
	w.pendingK8S = &K8SConfig{data: &K8SConfigData{
		Source:     "kubeconfig",
		Kubeconfig: kubeconfig,
	}, kube: kubeconfig != ""}
	if kubeconfig == "" {
		w.pendingK8S.data.Source = "token"
	}
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
	lk := lookFor(c)
	ui.Modal(c, &w.confOpen, func() {
		ui.Column(c).Width(560).Padding(20).Gap(14).Children(func() {
			dialogHeading(c, w.confTitle)

			var err string
			if w.confEditing != nil {
				w.hostKindBadge(c, lk, w.confEditing.Type())
				w.confEditing.Fields(c, true, w)
				err = w.confEditing.Validate()
			} else {
				ui.Text(c, locales.HostKindHint()).FontSize(12).TextColor(lk.textMuted)
				w.hostKindCards(c, lk)
				ui.Box(c).FillWidth().Height(1).Background(lk.divider).Margin(0, 2, 0, 2)
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
				ui.Text(c, err).FontSize(12.5).TextColor(lk.danger)
			}

			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, locales.BtnCancel()).Clicked() {
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

// hostKindCards are the three ways to add a host, as cards.
func (w *Window) hostKindCards(c *ui.Context, lk *look) {
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		for _, kind := range []string{"SSH", "Docker", "K8S"} {
			kind := kind
			selected := w.confKind == kind
			card := ui.Column(c).Grow(1).Basis(0).Height(78).Padding(10, 10).Gap(4).
				Radius(10).Border(1, lk.cardBorder).Cursor(ui.CursorPointer)
			switch {
			case selected:
				card.Background(lk.accentSoft)
				card.Border(1, lk.accent)
			case card.Hovered():
				card.Background(lk.rowHover)
			}
			card.Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					fg, bg := typeTint(kindType(kind), lk)
					ui.Box(c).Size(22, 22).Radius(6).Center().Background(bg).Children(func() {
						ui.Icon(c, typeIcon(kindType(kind))).Size(12, 12).TextColor(fg)
					})
					ui.Text(c, kindLabel(kindType(kind))).FontSize(12.5).FontWeight(600).
						TextColor(lk.text).Grow(1).SingleLine()
					if selected {
						ui.Icon(c, iconCheck).Size(12, 12).TextColor(lk.accent)
					}
				})
				ui.Text(c, kindDescription(kind)).FontSize(10.5).TextColor(lk.textMuted).LineHeight(1.3)
			})
			if card.Clicked() {
				w.confKind = kind
			}
		}
	})
}

// hostKindBadge is the type of a host being edited.
func (w *Window) hostKindBadge(c *ui.Context, lk *look, kind string) {
	fg, bg := typeTint(kindType(kind), lk)
	ui.Row(c).FillWidth().Height(38).Padding(0, 10).Gap(8).Radius(8).
		Background(lk.chip).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(22, 22).Radius(6).Center().Background(bg).Children(func() {
			ui.Icon(c, typeIcon(kindType(kind))).Size(12, 12).TextColor(fg)
		})
		ui.Text(c, kindLabel(kindType(kind))).FontSize(12.5).FontWeight(600).TextColor(lk.text)
	})
}

func kindType(kind string) string {
	return strings.ToLower(kind)
}

func kindDescription(kind string) string {
	switch kind {
	case "SSH":
		return locales.HostKindSSH()
	case "Docker":
		return locales.HostKindDocker()
	case "K8S":
		return locales.HostKindK8S()
	}
	return ""
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
	FilesHidden   bool              `json:"filesHidden,omitempty"`
	FilesWidth    float32           `json:"filesWidth,omitempty"`
	FollowDir     *bool             `json:"followDir,omitempty"`
	DockerDefault bool              `json:"dockerDefault,omitempty"`
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
	w.showFiles = !f.FilesHidden
	if f.FilesWidth > 0 {
		w.filesWidth = min(max(f.FilesWidth, 200), 560)
	}
	if f.FollowDir != nil {
		w.followDir = *f.FollowDir
	}
	w.dockerDefaultAdded = f.DockerDefault
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
		DockerDefault: w.dockerDefaultAdded,
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
