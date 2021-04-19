package fyneshell

import (
	"fyne.io/fyne/v2/container"
	"github.com/fyne-io/terminal"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"strconv"
)

type TermTab struct {
	name string
	term *terminal.Terminal
	tab container.TabItem
	stat string
	session *ssh.Session
	local bool
}

func newLocalTermTab() (*TermTab,error) {
	term := terminal.New()
	go func() {
		err := term.RunLocalShell()
		if err != nil {
			log.Println(err)
			return
		}
	}()
	return &TermTab{name: "local", term: term, local: true},nil
}

func newSSHTermTab(conf *Config) (*TermTab,error) {
	c := ssh.ClientConfig{User: conf.User, Auth: []ssh.AuthMethod{
		ssh.Password(conf.Pswd),
	}}
	c.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}
	addr := conf.Host + ":" + strconv.Itoa(conf.Port)
	conn, err := ssh.Dial("tcp", addr, &c)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	session, err := conn.NewSession()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	in, err := session.StdinPipe()
	if err != nil {
		log.Println(err)
		return nil, err
	}
	out, err := session.StdoutPipe()
	if err != nil {
		log.Println(err)
		return nil, err
	}

	term := terminal.New()
	go func() {
		err = term.RunWithConnection(in, out)
		if err != nil {
			log.Println(err)
		}
	}()

	return &TermTab{name: conf.Name, term: term, session: session, local: false},nil
}