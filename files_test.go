package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseLS(t *testing.T) {
	entries := parseLS("src/\nREADME.md\n.hidden\nbin/\n")
	if len(entries) != 4 {
		t.Fatalf("entries = %+v", entries)
	}
	if !entries[0].dir || entries[0].name != "bin" || !entries[1].dir || entries[1].name != "src" {
		t.Fatalf("folders are not first: %+v", entries)
	}
	if entries[2].dir || entries[2].name != ".hidden" {
		t.Fatalf("hidden file: %+v", entries[2])
	}
}

func TestLocalFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "z-folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a-file"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := (localFiles{}).List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].name != "z-folder" || !entries[0].dir {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[1].name != "a-file" || entries[1].dir || entries[1].size != 5 || !entries[1].sized {
		t.Fatalf("file = %+v", entries[1])
	}
	if (localFiles{}).Parent(dir) != filepath.Dir(dir) {
		t.Fatal("parent")
	}
}

func TestFileSize(t *testing.T) {
	if got := fileSize(5); got != "5 B" {
		t.Fatalf("fileSize(5) = %q", got)
	}
	if got := fileSize(1536); got != "1.5 KB" {
		t.Fatalf("fileSize(1536) = %q", got)
	}
}

func TestLocalFileWriter(t *testing.T) {
	dir := t.TempDir()
	src, ctx := localFiles{}, context.Background()

	folder := filepath.Join(dir, "docs")
	if err := src.Mkdir(ctx, folder); err != nil {
		t.Fatal(err)
	}
	if err := src.Mkdir(ctx, folder); err == nil {
		t.Fatal("a folder made over one of that name")
	}
	file := filepath.Join(folder, "note.txt")
	if err := src.Touch(ctx, file); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(file); err != nil || info.Size() != 0 {
		t.Fatalf("new file: %v %v", info, err)
	}
	if err := src.Touch(ctx, file); err == nil {
		t.Fatal("a file made over one of that name")
	}
	renamed := filepath.Join(folder, "renamed.txt")
	if err := src.Rename(ctx, file, renamed); err != nil {
		t.Fatal(err)
	}
	if err := src.Rename(ctx, renamed, renamed); err == nil {
		t.Fatal("a rename over a name that is")
	}
	if err := src.Remove(ctx, renamed, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		t.Fatalf("the file remains: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "inner"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := src.Remove(ctx, folder, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("the folder remains: %v", err)
	}
}

// shellRunner runs commands as a container's shell does, for the tests.
func shellRunner(ctx context.Context, cmd []string) (string, string, error) {
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	return stdout.String(), stderr.String(), err
}

func TestCommandFileWriter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the commands are a POSIX shell's")
	}
	dir := t.TempDir()
	src, ctx := commandFiles{run: shellRunner, feed: shellFeeder}, context.Background()

	folder := filepath.Join(dir, "docs")
	if err := src.Mkdir(ctx, folder); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "a note.txt")
	if err := src.Touch(ctx, file); err != nil {
		t.Fatal(err)
	}
	if err := src.Touch(ctx, file); err == nil {
		t.Fatal("a file made over one of that name")
	}
	renamed := filepath.Join(folder, "renamed.txt")
	if err := src.Rename(ctx, file, renamed); err != nil {
		t.Fatal(err)
	}
	if err := src.Rename(ctx, renamed, renamed); err == nil {
		t.Fatal("a rename over a name that is")
	}
	if err := src.Mkdir(ctx, filepath.Join(folder, "sub")); err != nil {
		t.Fatal(err)
	}
	if err := src.Remove(ctx, folder, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("the folder remains: %v", err)
	}
}

// shellFeeder runs a command with stdin as a container's shell does.
func shellFeeder(ctx context.Context, cmd []string, stdin io.Reader) (string, error) {
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Stdin = stdin
	var stderr bytes.Buffer
	c.Stderr = &stderr
	err := c.Run()
	return stderr.String(), err
}

func TestWritersWriteFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the commands are a POSIX shell's")
	}
	dir := t.TempDir()
	ctx := context.Background()
	file := filepath.Join(dir, "notes.txt")
	for name, wr := range map[string]fileWriter{
		"local":   localFiles{},
		"command": commandFiles{run: shellRunner, feed: shellFeeder},
	} {
		file := file
		if err := wr.Touch(ctx, file); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := wr.Write(ctx, file, bytes.NewReader([]byte("hello"))); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		data, err := os.ReadFile(file)
		if err != nil || string(data) != "hello" {
			t.Fatalf("%s wrote %q, %v", name, data, err)
		}
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidFileName(t *testing.T) {
	for _, name := range []string{"a", "a b.txt", ".hidden", "文档"} {
		if !validFileName(name) {
			t.Fatalf("%q refused", name)
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`} {
		if validFileName(name) {
			t.Fatalf("%q taken", name)
		}
	}
}

// rawReader is a file source whose one file holds r, for the tests.
type rawReader struct{ r io.Reader }

func (s rawReader) Reader(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(s.r), nil
}

// zeroReader reads zeros forever, for the tests.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "hello.txt")
	if err := copyFile(context.Background(), rawReader{bytes.NewReader([]byte("hello"))}, "/remote/hello.txt", dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "hello" {
		t.Fatalf("the copy: %q %v", data, err)
	}
}

func TestCopyFileRefusesABigFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "big.bin")
	src := rawReader{io.LimitReader(zeroReader{}, openFileMax+1)}
	if err := copyFile(context.Background(), src, "/remote/big.bin", dst); err == nil {
		t.Fatal("a file too large copied")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("the copy remains: %v", err)
	}
}

func TestLocalPath(t *testing.T) {
	file := filepath.Join("a", "b.txt")
	if p, ok := (localFiles{}).LocalPath(file); !ok || p != file {
		t.Fatalf("LocalPath = %q, %v", p, ok)
	}
}
