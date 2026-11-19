//go:build darwin || (linux && (amd64 || arm64))

package main

import "github.com/ebitengine/purego"

func openStatusLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

func statusLibrarySymbol(lib uintptr, name string) (uintptr, error) {
	return purego.Dlsym(lib, name)
}
