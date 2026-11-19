package main

import (
	"context"
	"os"
	"path/filepath"
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
