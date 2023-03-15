package main

import (
	"encoding/json"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"strconv"
)

type SSHConfigData struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
	Pswd string `json:"pswd,omitempty"`
}

type SSHConfigForm struct {
	nameEntry *widget.Entry
	hostEntry *widget.Entry
	portEntry *widget.Entry
	userEntry *widget.Entry
	pswdEntry *widget.Entry
}

type SSHConfig struct {
	data *SSHConfigData
	form *SSHConfigForm
	onOk func()
}

func (c *SSHConfig) Name() string {
	return c.data.Name
}

func (c *SSHConfig) Type() string {
	return "ssh"
}

func (c *SSHConfig) Load(s string) error {
	data := &SSHConfigData{}

	err := json.Unmarshal([]byte(s), data)
	if err != nil {
		return err
	}
	c.data = data
	return nil
}

func (c *SSHConfig) Data() interface{} {
	return c.data
}

func (c *SSHConfig) Form() *widget.Form {
	c.form = &SSHConfigForm{}
	nameEntry := widget.NewEntry()
	hostEntry := widget.NewEntry()
	portEntry := widget.NewEntry()
	userEntry := widget.NewEntry()
	pswdEntry := widget.NewEntry()

	portEntry.Text = "22"
	portEntry.Validator = func(s string) error {
		_, err := strconv.Atoi(s)
		return err
	}
	pswdEntry.Password = true
	data := c.data
	if data != nil {
		nameEntry.Text = data.Name
		nameEntry.Disable()
		hostEntry.Text = data.Host
		portEntry.Text = strconv.Itoa(data.Port)
		userEntry.Text = data.User
		pswdEntry.Text = data.Pswd
	}
	c.onOk = func() {
		if c.data == nil {
			c.data = &SSHConfigData{Type: c.Type()}
		}
		c.data.Name = nameEntry.Text
		c.data.Host = hostEntry.Text
		c.data.Port, _ = strconv.Atoi(portEntry.Text)
		c.data.User = userEntry.Text
		c.data.Pswd = pswdEntry.Text
	}
	return widget.NewForm([]*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Host", hostEntry),
		widget.NewFormItem("Port", portEntry),
		widget.NewFormItem("Username", userEntry),
		widget.NewFormItem("Password", pswdEntry),
	}...)
}

func (c *SSHConfig) OnOk() {
	c.onOk()
}

func (c *SSHConfig) Term(win *Window) {
	conf := c.data
	cli := ssh.ClientConfig{User: conf.User, Auth: []ssh.AuthMethod{
		ssh.Password(conf.Pswd),
	}, HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}}

	addr := conf.Host + ":" + strconv.Itoa(conf.Port)
	conn, err := ssh.Dial("tcp", addr, &cli)
	if err != nil {
		log.Println(err)
		win.showError(err)
		return
	}
	session, err := conn.NewSession()
	if err != nil {
		log.Println(err)
		win.showError(err)
		return
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	err = session.RequestPty("xterm-color", 24, 80, modes)
	if err != nil {
		log.Println(err)
		win.showError(err)
		return
	}
	in, err := session.StdinPipe()
	if err != nil {
		log.Println(err)
		win.showError(err)
		return
	}
	out, err := session.StdoutPipe()
	if err != nil {
		log.Println(err)
		win.showError(err)
		return
	}

	term := NewTerm(conf.Name, c)

	go func() {
		defer session.Close()
		err = term.RunWithReaderAndWriter(in, out)
		if err != nil {
			log.Println(err)
		}
		session.Close()
	}()

	go func() {
		err := session.Shell()
		if err != nil {
			log.Println(err)
		}
	}()

	win.AddTermTab(term)
}
