package main

import (
	"encoding/json"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"log"
)

type Config interface {
	Name() string
	Type() string
	Load(string) error
	Data() interface{}
	Form() *widget.Form
	Term(*Window)
	OnOk()
}

func (w *Window) showCreateConfigDialog() {
	sshConf := &SSHConfig{}
	dockerConf := &DockerConfig{}
	k8sConf := &K8SConfig{}

	sshForm := sshConf.Form()
	dockerForm := dockerConf.Form()
	k8sForm := k8sConf.Form()
	typeSelect := widget.NewSelect([]string{"SSH", "Docker", "K8S"}, func(s string) {
		sshForm.Hide()
		dockerForm.Hide()
		k8sForm.Hide()
		switch s {
		case "SSH":
			sshForm.Show()
		case "Docker":
			dockerForm.Show()
		case "K8S":
			k8sForm.Show()
		default:

		}
	})
	title := "Create Config"
	typeSelect.SetSelectedIndex(0)
	box := container.NewVBox(typeSelect, sshForm, dockerForm, k8sForm)

	dlg := dialog.NewCustomConfirm(title, "OK", "Cancel", box, func(b bool) {
		if b {
			switch typeSelect.Selected {
			case "SSH":
				sshConf.OnOk()
				w.confs = append(w.confs, sshConf)
			case "Docker":
				dockerConf.OnOk()
				w.confs = append(w.confs, dockerConf)
			case "K8S":
				k8sConf.OnOk()
				w.confs = append(w.confs, k8sConf)
			default:
				return
			}
			w.save()
		}
	}, w.win)
	dlg.Resize(fyne.Size{Width: 300})
	dlg.Show()
}

func (w *Window) showModifyConfigDialog(cfg Config) {
	form := cfg.Form()
	title := "Modify Config: " + cfg.Name()
	dlg := dialog.NewCustomConfirm(title, "OK", "Cancel", form, func(b bool) {
		if b {
			cfg.OnOk()
			w.save()
		}
	}, w.win)
	dlg.Resize(fyne.Size{Width: 300})
	dlg.Show()
}

func (w *Window) load() {
	confJson := w.app.Preferences().String(APP_SESSIONS)

	confArr := []map[string]interface{}{}
	err := json.Unmarshal([]byte(confJson), &confArr)
	if err != nil {
		log.Println(err)
	}

	w.confs = make([]Config, 0)
	for _, data := range confArr {
		if v, e := data["type"]; e {
			if t, ok := v.(string); ok {
				var cfg Config
				switch t {
				case "ssh":
					cfg = &SSHConfig{data: &SSHConfigData{}}
				case "docker":
					cfg = &DockerConfig{data: &DockerConfigData{}}
				case "k8s":
					cfg = &K8SConfig{data: &K8SConfigData{}}
				default:
					continue
				}
				bytes, err := json.Marshal(data)
				if err != nil {
					continue
				}
				err = cfg.Load(string(bytes))
				if err != nil {
					continue
				}
				w.confs = append(w.confs, cfg)
			}
		}

	}

	cmdJson := w.app.Preferences().String(APP_COMMANDS)
	err = json.Unmarshal([]byte(cmdJson), &w.cmds)
	if err != nil {
		log.Println(err)
	}
}

func (w *Window) save() {

	confData := make([]interface{}, len(w.confs))
	for i := 0; i < len(w.confs); i++ {
		confData[i] = w.confs[i].Data()
	}

	confJson, err := json.Marshal(confData)
	if err != nil {
		log.Println(err)
		return
	}
	w.app.Preferences().SetString(APP_SESSIONS, string(confJson))

	cmdJson, err := json.Marshal(w.cmds)
	if err != nil {
		log.Println(err)
		return
	}
	w.app.Preferences().SetString(APP_COMMANDS, string(cmdJson))

}
