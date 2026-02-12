package main

import (
	"encoding/json"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

type SSHConfigData struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
	Pswd string `json:"pswd,omitempty"` // 加密存储
}

// getPassword 返回解密后的密码
func (d *SSHConfigData) getPassword() (string, error) {
	return decryptString(d.Pswd)
}

// setPassword 加密并设置密码
func (d *SSHConfigData) setPassword(password string) (string, error) {
	encrypted, err := encryptString(password)
	if err != nil {
		return "", err
	}
	d.Pswd = encrypted
	return encrypted, nil
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
	isNewConfig := (data == nil)
	if data != nil {
		nameEntry.Text = data.Name
		nameEntry.Disable()
		hostEntry.Text = data.Host
		portEntry.Text = strconv.Itoa(data.Port)
		userEntry.Text = data.User
		// 修改时显示空密码，用户需要重新输入
		pswdEntry.Text = ""
	}
	c.onOk = func() {
		if c.data == nil {
			c.data = &SSHConfigData{Type: c.Type()}
		}
		c.data.Name = nameEntry.Text
		c.data.Host = hostEntry.Text
		c.data.Port, _ = strconv.Atoi(portEntry.Text)
		c.data.User = userEntry.Text
		// 只在密码不为空时更新密码（允许不修改密码）
		if pswdEntry.Text != "" || isNewConfig {
			if _, err := c.data.setPassword(pswdEntry.Text); err != nil {
				log.Printf("Failed to encrypt password: %v", err)
			}
		}
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

	// 获取解密后的密码
	password, err := conf.getPassword()
	if err != nil {
		log.Printf("Failed to decrypt password: %v", err)
		win.showError(err)
		return
	}

	// 创建已知主机文件路径
	knownHostsPath := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")

	// 创建主机密钥回调函数
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		log.Printf("Failed to create host key callback: %v", err)
		win.showError(err)
		return
	}

	cli := ssh.ClientConfig{
		User:            conf.User,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10, // 10秒超时
	}

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
