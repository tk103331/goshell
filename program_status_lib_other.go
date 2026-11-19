//go:build !(darwin || (linux && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package main

import "errors"

func openStatusLibrary(string) (uintptr, error) {
	return 0, errors.ErrUnsupported
}

func statusLibrarySymbol(uintptr, string) (uintptr, error) {
	return 0, errors.ErrUnsupported
}
