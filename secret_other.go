//go:build !windows

package main

import "errors"

// The app ships for Windows. The stub keeps `GOOS=darwin go build` and tests
// on other platforms honest: the key cannot be persisted there, and the UI
// says so instead of writing plaintext to disk.
func protectSecret(string) (string, error) {
	return "", errors.New("当前平台不支持 DPAPI，密钥只在本次运行有效")
}

func unprotectSecret(string) (string, error) {
	return "", errors.New("当前平台不支持 DPAPI")
}
