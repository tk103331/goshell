package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// fileEntry is one row of the file sidebar.
type fileEntry struct {
	name  string
	dir   bool
	size  int64
	sized bool
}

// fileSource lists the folders of one terminal: this machine's, an SSH
// server's, a container's or a pod's.
type fileSource interface {
	// Home is where the sidebar opens.
	Home(ctx context.Context) (string, error)
	List(ctx context.Context, dir string) ([]fileEntry, error)
	Join(dir, name string) string
	Parent(dir string) string
	Close() error
}

// fileBrowser is the browsing state of one terminal: where it is, what it
// last showed, and the request whose answer is still wanted.
type fileBrowser struct {
	src        fileSource
	path       string
	entries    []fileEntry
	loading    bool
	err        string
	request    uint64
	started    bool
	homeLoaded bool
	// lastTermDir is the working directory last followed, so that a manual
	// move is not undone every frame.
	lastTermDir string
	// lastPoll and polling pace a source that has to be asked for its
	// directory, as Docker and SSH's shells do.
	lastPoll time.Time
	polling  bool
}

// dirSource is a file source whose shell's working directory can be asked
// for, for terminals whose shell does not report OSC 7.
type dirSource interface {
	Dir(ctx context.Context) (string, error)
}

// useFiles gives a tab a file source, for the right sidebar.
func (t *Tab) useFiles(src fileSource) {
	t.files = &fileBrowser{src: src}
}

// --- local ---

type localFiles struct{}

func (localFiles) Home(context.Context) (string, error) { return os.UserHomeDir() }

func (localFiles) List(_ context.Context, dir string) ([]fileEntry, error) {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]fileEntry, 0, len(list))
	for _, it := range list {
		info, err := it.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Stat(filepath.Join(dir, it.Name())); err == nil {
				info = target
			}
		}
		entries = append(entries, fileEntry{
			name:  it.Name(),
			dir:   info.IsDir(),
			size:  info.Size(),
			sized: !info.IsDir(),
		})
	}
	sortFileEntries(entries)
	return entries, nil
}

func (localFiles) Join(dir, name string) string { return filepath.Join(dir, name) }

func (localFiles) Parent(dir string) string { return filepath.Dir(dir) }

func (localFiles) Close() error { return nil }

// --- SSH ---

type sftpFiles struct {
	cli  *sftp.Client
	conn *ssh.Client
}

func (f *sftpFiles) Home(ctx context.Context) (string, error) {
	if dir, err := f.Dir(ctx); err == nil && dir != "" {
		return dir, nil
	}
	dir, err := f.cli.Getwd()
	if err != nil || dir == "" {
		return "/", nil
	}
	return dir, nil
}

// Dir asks the server for the interactive shell's directory. A new exec
// over the same SSH connection shares SSH_CONNECTION with that shell, and
// its SSH_TTY marks it among the processes with that connection; on Linux
// its /proc entry then gives the directory.
func (f *sftpFiles) Dir(ctx context.Context) (string, error) {
	if f.conn == nil {
		return "", nil
	}
	sess, err := f.conn.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	type answer struct {
		dir string
		err error
	}
	done := make(chan answer, 1)
	go func() {
		out, err := sess.Output("/bin/sh -c " + shellQuote(sshDirScript))
		done <- answer{strings.TrimSpace(string(out)), err}
	}()
	select {
	case <-ctx.Done():
		sess.Close()
		return "", ctx.Err()
	case a := <-done:
		if a.err != nil {
			return "", a.err
		}
		if !strings.HasPrefix(a.dir, "/") {
			return "", errors.New("shell directory not found")
		}
		return a.dir, nil
	}
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// sshDirScript finds the process with the same SSH_CONNECTION as this exec
// and an SSH_TTY, which is the interactive shell, and reads its directory.
const sshDirScript = `want=$SSH_CONNECTION
for p in /proc/[0-9]*; do
  e=$p/environ
  [ -r "$e" ] || continue
  env=$(tr '\0' '\n' < "$e" 2>/dev/null) || continue
  printf '%s\n' "$env" | grep -qxF "SSH_CONNECTION=$want" || continue
  printf '%s\n' "$env" | grep -q '^SSH_TTY=' || continue
  dir=$(readlink "$p/cwd" 2>/dev/null) || continue
  [ -n "$dir" ] && { printf '%s\n' "$dir"; exit 0; }
done
exit 1`

func (f *sftpFiles) List(_ context.Context, dir string) ([]fileEntry, error) {
	list, err := f.cli.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]fileEntry, 0, len(list))
	for _, info := range list {
		if info.Mode()&os.ModeSymlink != 0 {
			if target, err := f.cli.Stat(path.Join(dir, info.Name())); err == nil {
				info = target
			}
		}
		entries = append(entries, fileEntry{
			name:  info.Name(),
			dir:   info.IsDir(),
			size:  info.Size(),
			sized: !info.IsDir(),
		})
	}
	sortFileEntries(entries)
	return entries, nil
}

func (*sftpFiles) Join(dir, name string) string { return path.Join(dir, name) }

func (*sftpFiles) Parent(dir string) string { return path.Dir(dir) }

func (f *sftpFiles) Close() error { return f.cli.Close() }

// --- commands in a container or pod ---

type commandRunner func(ctx context.Context, cmd []string) (stdout, stderr string, err error)

// lsFiles lists a remote directory with ls, whose -1 -A -p say one entry a
// line, hidden entries too, and a slash after a folder.
func lsFiles(ctx context.Context, run commandRunner, dir string) ([]fileEntry, error) {
	stdout, stderr, err := run(ctx, []string{"ls", "-1Ap", "--", dir})
	if err != nil && stdout == "" {
		return nil, commandError(err, stderr)
	}
	if stdout == "" && strings.TrimSpace(stderr) != "" {
		return nil, errors.New(strings.TrimSpace(stderr))
	}
	return parseLS(stdout), nil
}

func commandError(err error, stderr string) error {
	if text := strings.TrimSpace(stderr); text != "" {
		return fmt.Errorf("%v: %s", err, text)
	}
	return err
}

func parseLS(out string) []fileEntry {
	var entries []fileEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || line == "." || line == ".." {
			continue
		}
		dir := strings.HasSuffix(line, "/")
		name := strings.TrimSuffix(line, "/")
		if name == "" {
			continue
		}
		entries = append(entries, fileEntry{name: name, dir: dir})
	}
	sortFileEntries(entries)
	return entries
}

// procShellDirScript finds the shell marked by marker in /proc and prints
// its working directory: the shell Docker and Kubernetes started for the
// terminal, which has no OSC 7 of its own.
func procShellDirScript(marker string) string {
	return `for p in /proc/[0-9]*; do
  e=$p/environ
  [ -r "$e" ] || continue
  env=$(tr '\0' '\n' < "$e" 2>/dev/null) || continue
  printf '%s\n' "$env" | grep -qxF "GOSHELL_SESSION=` + marker + `" || continue
  case "$(cat "$p/comm" 2>/dev/null)" in
    sh|bash|zsh|dash|ash|ksh|fish) ;;
    *) continue ;;
  esac
  dir=$(readlink "$p/cwd" 2>/dev/null) || continue
  [ -n "$dir" ] && { printf '%s\n' "$dir"; exit 0; }
done
exit 1`
}

// --- Docker ---

type dockerFiles struct {
	cli    *client.Client
	id     string
	marker string // the environment variable that marks the interactive shell
}

func (f *dockerFiles) Home(ctx context.Context) (string, error) {
	if dir, err := f.Dir(ctx); err == nil && dir != "" {
		return dir, nil
	}
	stdout, _, err := f.run(ctx, []string{"sh", "-c", "pwd"})
	if err == nil {
		if dir := strings.TrimSpace(stdout); strings.HasPrefix(dir, "/") {
			return dir, nil
		}
	}
	return "/", nil
}

// Dir is the working directory of the interactive shell, which a separate
// exec can read from /proc: the terminal's OSC 7 does not reach Docker's
// /bin/sh.
func (f *dockerFiles) Dir(ctx context.Context) (string, error) {
	if f.marker == "" {
		return "", nil
	}
	stdout, stderr, err := f.run(ctx, []string{"sh", "-c", procShellDirScript(f.marker)})
	if err != nil {
		return "", commandError(err, stderr)
	}
	dir := strings.TrimSpace(stdout)
	if dir == "" || !strings.HasPrefix(dir, "/") {
		return "", errors.New(strings.TrimSpace(stderr))
	}
	return dir, nil
}

func (f *dockerFiles) List(ctx context.Context, dir string) ([]fileEntry, error) {
	return lsFiles(ctx, f.run, dir)
}

func (f *dockerFiles) run(ctx context.Context, cmd []string) (string, string, error) {
	execID, err := f.cli.ContainerExecCreate(ctx, f.id, container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	})
	if err != nil {
		return "", "", err
	}
	attach, err := f.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", err
	}
	defer attach.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil {
		return stdout.String(), stderr.String(), err
	}
	inspect, err := f.cli.ContainerExecInspect(ctx, execID.ID)
	if err == nil && inspect.ExitCode != 0 {
		return stdout.String(), stderr.String(), fmt.Errorf("exit status %d", inspect.ExitCode)
	}
	return stdout.String(), stderr.String(), nil
}

func (*dockerFiles) Join(dir, name string) string { return path.Join(dir, name) }

func (*dockerFiles) Parent(dir string) string { return path.Dir(dir) }

func (*dockerFiles) Close() error { return nil }

// --- Kubernetes ---

type k8sFiles struct {
	cli    *kubernetes.Clientset
	cfg    *rest.Config
	ns     string
	pod    string
	ct     string
	marker string // the environment variable that marks the interactive shell
}

func (f *k8sFiles) Home(ctx context.Context) (string, error) {
	if dir, err := f.Dir(ctx); err == nil && dir != "" {
		return dir, nil
	}
	stdout, _, err := f.run(ctx, []string{"sh", "-c", "pwd"})
	if err == nil {
		if dir := strings.TrimSpace(stdout); strings.HasPrefix(dir, "/") {
			return dir, nil
		}
	}
	return "/", nil
}

// Dir is the interactive shell's working directory, marked by an
// environment variable and read from /proc beside it.
func (f *k8sFiles) Dir(ctx context.Context) (string, error) {
	if f.marker == "" {
		return "", nil
	}
	stdout, stderr, err := f.run(ctx, []string{"sh", "-c", procShellDirScript(f.marker)})
	if err != nil {
		return "", commandError(err, stderr)
	}
	dir := strings.TrimSpace(stdout)
	if dir == "" || !strings.HasPrefix(dir, "/") {
		return "", errors.New(strings.TrimSpace(stderr))
	}
	return dir, nil
}

func (f *k8sFiles) List(ctx context.Context, dir string) ([]fileEntry, error) {
	return lsFiles(ctx, f.run, dir)
}

func (f *k8sFiles) run(ctx context.Context, cmd []string) (string, string, error) {
	req := f.cli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(f.pod).
		Namespace(f.ns).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: f.ct,
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(f.cfg, "POST", req.URL())
	if err != nil {
		return "", "", err
	}
	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	return stdout.String(), stderr.String(), err
}

func (*k8sFiles) Join(dir, name string) string { return path.Join(dir, name) }

func (*k8sFiles) Parent(dir string) string { return path.Dir(dir) }

func (*k8sFiles) Close() error { return nil }

// --- shared ---

func sortFileEntries(entries []fileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].dir != entries[j].dir {
			return entries[i].dir
		}
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})
}

// fileSize says how large a file is, in the shortest units.
func fileSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// fileTimeout is how long a folder may take to list, for the folders on the
// other side of a network.
const fileTimeout = 10 * time.Second

// followPoll is how often a source that has to be asked for its shell's
// directory is asked, as Docker and SSH are.
const followPoll = 2 * time.Second
