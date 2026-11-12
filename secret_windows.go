//go:build windows

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// DPAPI ties the encrypted key to the current Windows account. A config file
// copied to another account or machine cannot be decrypted, which is the
// point: it is better to ask for the key again than to leave it readable.
var (
	dpapiCrypt32           = syscall.NewLazyDLL("crypt32.dll")
	dpapiKernel32          = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = dpapiCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = dpapiCrypt32.NewProc("CryptUnprotectData")
	procLocalFree          = dpapiKernel32.NewProc("LocalFree")
)

const cryptProtectUIForbidden = 0x1

type dataBlob struct {
	size uint32
	data *byte
}

func protectSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	in := []byte(plain)
	var inBlob dataBlob
	inBlob.size = uint32(len(in))
	if len(in) > 0 {
		inBlob.data = &in[0]
	}
	var out dataBlob
	ok, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, 0, 0, 0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if ok == 0 {
		return "", fmt.Errorf("DPAPI 加密失败: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	sealed := unsafe.Slice(out.data, out.size)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func unprotectSecret(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("密钥数据不是有效的 base64: %w", err)
	}
	if len(sealed) == 0 {
		return "", errors.New("密钥数据为空")
	}
	var inBlob dataBlob
	inBlob.size = uint32(len(sealed))
	inBlob.data = &sealed[0]
	var out dataBlob
	ok, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, 0, 0, 0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if ok == 0 {
		return "", fmt.Errorf("DPAPI 解密失败: %w", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	plain := unsafe.Slice(out.data, out.size)
	return string(plain), nil
}
