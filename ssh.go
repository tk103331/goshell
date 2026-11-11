package main

import (
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"goshell/locales"
)

type SSHConfigData struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	User    string `json:"user,omitempty"`
	Pswd    string `json:"pswd,omitempty"` // 加密存储
	KeyPath string `json:"keyPath,omitempty"`
	KeyPswd string `json:"keyPswd,omitempty"` // 加密存储
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

// secretOf 解密落盘的敏感信息；加密前留下的明文原样返回，让旧的配置
// 在下一次保存时被加密。
func secretOf(stored string) (string, error) {
	plain, err := decryptString(stored)
	if err != nil && stored != "" {
		log.Println("secret was stored unencrypted; it will be encrypted on the next save")
		return stored, nil
	}
	return plain, err
}

type SSHConfig struct {
	data *SSHConfigData
	port string // the port as the form edits it

	pswdPlain  string // the password as the form edits it, encrypted on save
	keyPswdRaw string // the key passphrase as the form edits it
}

func (c *SSHConfig) Name() string      { return c.data.Name }
func (c *SSHConfig) Type() string      { return "ssh" }
func (c *SSHConfig) Data() interface{} { return c.data }

func (c *SSHConfig) Fields(ctx *ui.Context, editing bool, w *Window) {
	ui.Field(ctx, locales.FieldName(), func() {
		e := ui.TextInput(ctx, &c.data.Name)
		if editing {
			e.ReadOnly(true)
		}
	})
	ui.Field(ctx, locales.FieldHost(), func() { ui.TextInput(ctx, &c.data.Host) })
	ui.Field(ctx, locales.FieldPort(), func() { ui.TextInput(ctx, &c.port) })
	ui.Field(ctx, locales.FieldUsername(), func() { ui.TextInput(ctx, &c.data.User) })
	ui.Field(ctx, locales.FieldPassword(), func() {
		e := ui.TextInput(ctx, &c.pswdPlain).Password()
		if editing {
			e.Placeholder("••••••••")
		}
	})
	ui.Field(ctx, locales.FieldPrivateKey(), func() {
		ui.Row(ctx).FillWidth().Gap(6).Children(func() {
			ui.TextInput(ctx, &c.data.KeyPath).Grow(1).Placeholder("~/.ssh/id_ed25519")
			if ui.Button(ctx, locales.BtnChooseKey()).Clicked() {
				go func() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
						Title:       locales.ChooseKeyTitle(),
						ButtonLabel: locales.BtnUseKey(),
					})
					if err != nil || len(paths) == 0 {
						return
					}
					w.onMain(func() { c.data.KeyPath = paths[0] })
				}()
			}
		})
	})
	if c.data.KeyPath != "" {
		ui.Field(ctx, locales.FieldKeyPassphrase(), func() {
			e := ui.TextInput(ctx, &c.keyPswdRaw).Password()
			if editing {
				e.Placeholder("••••••••")
			}
		})
	}
}

func (c *SSHConfig) Apply() {
	c.data.Type = c.Type()
	if p, err := strconv.Atoi(c.port); err == nil {
		c.data.Port = p
	}
	// a password typed in this session is encrypted; a blank one leaves
	// the stored password alone, as when editing without retyping it
	if c.pswdPlain != "" || c.data.Pswd == "" {
		if _, err := c.data.setPassword(c.pswdPlain); err != nil {
			log.Printf("Failed to encrypt password: %v", err)
			c.data.Pswd = c.pswdPlain
		}
	}
	if c.data.KeyPath != "" && (c.keyPswdRaw != "" || c.data.KeyPswd == "") {
		if encrypted, err := encryptString(c.keyPswdRaw); err != nil {
			log.Printf("Failed to encrypt key passphrase: %v", err)
			c.data.KeyPswd = c.keyPswdRaw
		} else {
			c.data.KeyPswd = encrypted
		}
	}
}

func (c *SSHConfig) Prefill() {
	c.port = strconv.Itoa(c.data.Port)
	// passwords stay hidden when editing: retype to change one
	c.pswdPlain = ""
	c.keyPswdRaw = ""
}

func (c *SSHConfig) Validate() string {
	if c.data.Name == "" {
		return locales.ValNameRequired()
	}
	if c.data.Host == "" {
		return locales.ValHostRequired()
	}
	if _, err := strconv.Atoi(c.port); err != nil {
		return locales.ValPortNumber()
	}
	if p, err := strconv.Atoi(c.port); err == nil && (p <= 0 || p > 65535) {
		return locales.ValPortRange()
	}
	if c.data.KeyPath != "" {
		if _, err := os.Stat(c.data.KeyPath); err != nil {
			return locales.ValKeyNotFound(c.data.KeyPath)
		}
	}
	return ""
}

// Term dials the server and opens a tab with its shell.
func (c *SSHConfig) Term(w *Window) {
	data := c.data

	// 获取解密后的密码（空串跳过）
	password, err := secretOf(data.Pswd)
	if err != nil {
		log.Println(err)
		w.showError(err)
		return
	}
	keyPswd, err := secretOf(data.KeyPswd)
	if err != nil {
		log.Println(err)
		w.showError(err)
		return
	}

	// offer the private key before the password
	var auth []ssh.AuthMethod
	if data.KeyPath != "" {
		key, err := os.ReadFile(data.KeyPath)
		if err != nil {
			log.Println(err)
			w.showError(err)
			return
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			if _, ok := err.(*ssh.PassphraseMissingError); ok {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(keyPswd))
			}
			if err != nil {
				log.Println(err)
				w.showError(err)
				return
			}
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if password != "" || len(auth) == 0 {
		auth = append(auth, ssh.Password(password))
	}

	cli := ssh.ClientConfig{
		User:            data.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback(),
		Timeout:         15 * time.Second,
	}
	addr := net.JoinHostPort(data.Host, strconv.Itoa(data.Port))
	conn, err := ssh.Dial("tcp", addr, &cli)
	if err != nil {
		log.Println(err)
		w.showError(err)
		return
	}
	session, err := conn.NewSession()
	if err != nil {
		log.Println(err)
		w.showError(err)
		conn.Close()
		return
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		log.Println(err)
		w.showError(err)
		session.Close()
		conn.Close()
		return
	}
	in, err := session.StdinPipe()
	if err != nil {
		log.Println(err)
		w.showError(err)
		session.Close()
		conn.Close()
		return
	}
	out, err := session.StdoutPipe()
	if err != nil {
		log.Println(err)
		w.showError(err)
		session.Close()
		conn.Close()
		return
	}

	tab, _, err := w.newTab(data.Name, c, func() { session.Close() },
		terminal.Options{Conn: &sshConn{session: session, in: in, out: out}})
	if err != nil {
		log.Println(err)
		w.showError(err)
		session.Close()
		conn.Close()
		return
	}

	go func() {
		if err := session.Shell(); err != nil {
			log.Println(err)
		}
	}()

	w.showToast(locales.ToastConnected(data.User+"@"+data.Name), "success")

	w.onMain(func() {
		w.addTermTab(tab)
	})
}

// hostKeyCallback verifies the server's host key against ~/.ssh/known_hosts,
// as OpenSSH does; without the file, which a first installation has, it
// warns and accepts any key.
func hostKeyCallback() ssh.HostKeyCallback {
	path := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	if _, err := os.Stat(path); err != nil {
		log.Println("no ~/.ssh/known_hosts; host keys will not be verified")
		return ssh.InsecureIgnoreHostKey()
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		log.Println(err)
		return ssh.InsecureIgnoreHostKey()
	}
	return callback
}

// sshConn connects a terminal to an SSH session: the terminal reads what
// the session writes, and writes what is typed to it, and resizes its PTY.
type sshConn struct {
	session *ssh.Session
	in      io.WriteCloser
	out     io.Reader
}

func (c *sshConn) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *sshConn) Write(p []byte) (int, error) { return c.in.Write(p) }
func (c *sshConn) Close() error                { return c.session.Close() }

func (c *sshConn) Resize(cols, rows int) error {
	return c.session.WindowChange(cols, rows)
}
