package main

import (
	"fyne.io/fyne/v2/widget"
)

type K8SConfigData struct {
	Name   string
	Type   string
	Server string
	Token  string
	User   string
	Pswd   string
}
type K8SConfig struct {
	data *K8SConfigData
}

func (c *K8SConfig) Name() string {
	return c.data.Name
}

func (c *K8SConfig) Type() string {
	return "k8s"
}

func (c *K8SConfig) Data() interface{} {
	return c.data
}

func (c *K8SConfig) Form() *widget.Form {
	return widget.NewForm()
}

func (c *K8SConfig) OnOk() {

}

func (c *K8SConfig) Term(win *Window) {
	panic("implement me")
}
