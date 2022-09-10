# GoShell(WIP)

GoShell is a simple terminal GUI client, written in Go,via [Fyne](https://fyne.io). Supports SSH、Docker、K8S.


# Features

- Supports SSH、Docker、K8S(coming soon).
- Supports Windows、Linux、MacOS platform.（thanks [Fyne](https://fyne.io)）
- Supports shortcut command.

# Screenshots
### Main
![GoShell Main](screenshot/main.png)
### SSH Config
![GoShell SSH](screenshot/ssh-conf.png)
### Docker Config
![GoShell Docker](screenshot/docker-conf.png)
### Docker Select Container
![GoShell Docker](screenshot/docker-container.png)
### K8S Config
![GoShell Docker](screenshot/k8s-conf.png)
### K8S Select Container
![GoShell Docker](screenshot/k8s-container.png)

### Building

- Linux / MACOS
``` shell
    git clone https://github.com/tk103331/goshell.git
    cd goshell
    go build
    sudo ./goshell
```
- Windows (Need to run with administrator rights)
``` shell
    git clone https://github.com/tk103331/goshell.git
    cd goshell
    go build
    goshell
```

# TODOs

- UI/UX optimization
- Configuration encryption 
- ~~Supports K8S pod.~~
