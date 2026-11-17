//go:build !windows

package main

import "errors"

func protectSecret(string) (string, error) {
	return "", errors.New("当前平台不支持 DPAPI，密钥只在本次运行有效")
}

func unprotectSecret(string) (string, error) {
	return "", errors.New("当前平台不支持 DPAPI")
}
