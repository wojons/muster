# OS Keychain Specification

Complete specification for secure credential storage using platform-native keychain/keyring services.

## Overview

Cross-platform keychain integration supporting macOS Keychain, Linux Secret Service, and Windows Credential Manager for secure API token and credential storage.

## Interface

```go
package keychain

import (
	"context"
	"fmt"
	"strings"
)

// Keychain defines the interface for secure credential storage
type Keychain interface {
	Set(ctx context.Context, service, account, password string) error
	Get(ctx context.Context, service, account string) (string, error)
	Delete(ctx context.Context, service, account string) error
	List(ctx context.Context, service string) ([]Account, error)
}

// Account represents a stored account credential
type Account struct {
	Service string
	Account string
}

// Errors
var (
	ErrNotFound     = fmt.Errorf("credential not found")
	ErrAccessDenied = fmt.Errorf("access denied to keychain")
	ErrNotSupported = fmt.Errorf("keychain not supported on this platform")
)
```

## macOS Implementation

```go
//go:build darwin
// +build darwin

package keychain

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
*/
import "C"
import (
	"context"
	"fmt"
	"unsafe"
)

type macOSKeychain struct{}

func New() (Keychain, error) {
	return &macOSKeychain{}, nil
}

func (k *macOSKeychain) Set(ctx context.Context, service, account, password string) error {
	cservice := C.CString(service)
	caccount := C.CString(account)
	cpassword := C.CString(password)
	defer C.free(unsafe.Pointer(cservice))
	defer C.free(unsafe.Pointer(caccount))
	defer C.free(unsafe.Pointer(cpassword))

	query := C.CFDictionaryCreateMutable(nil, 0, nil, nil)
	C.CFDictionaryAddValue(query, C.kSecClass, C.kSecClassGenericPassword)
	C.CFDictionaryAddValue(query, C.kSecAttrService, unsafe.Pointer(C.CFStringCreateWithCString(nil, cservice, 0)))
	C.CFDictionaryAddValue(query, C.kSecAttrAccount, unsafe.Pointer(C.CFStringCreateWithCString(nil, caccount, 0)))

	data := C.CFDataCreate(nil, (*C.UInt8)(unsafe.Pointer(cpassword)), C.CFIndex(len(password)))
	C.CFDictionaryAddValue(query, C.kSecValueData, unsafe.Pointer(data))

	status := C.SecItemAdd(query, nil)
	C.CFRelease(C.CFTypeRef(query))

	if status == C.errSecDuplicateItem {
		// Update existing item
		updateQuery := C.CFDictionaryCreateMutable(nil, 0, nil, nil)
		C.CFDictionaryAddValue(updateQuery, C.kSecValueData, unsafe.Pointer(data))
		C.SecItemUpdate(query, updateQuery)
		C.CFRelease(C.CFTypeRef(updateQuery))
	} else if status != C.errSecSuccess {
		return fmt.Errorf("keychain add failed: %d", status)
	}

	return nil
}

func (k *macOSKeychain) Get(ctx context.Context, service, account string) (string, error) {
	cservice := C.CString(service)
	caccount := C.CString(account)
	defer C.free(unsafe.Pointer(cservice))
	defer C.free(unsafe.Pointer(caccount))

	query := C.CFDictionaryCreateMutable(nil, 0, nil, nil)
	C.CFDictionaryAddValue(query, C.kSecClass, C.kSecClassGenericPassword)
	C.CFDictionaryAddValue(query, C.kSecAttrService, unsafe.Pointer(C.CFStringCreateWithCString(nil, cservice, 0)))
	C.CFDictionaryAddValue(query, C.kSecAttrAccount, unsafe.Pointer(C.CFStringCreateWithCString(nil, caccount, 0)))
	C.CFDictionaryAddValue(query, C.kSecReturnData, C.kCFBooleanTrue)

	var result C.CFTypeRef
	status := C.SecItemCopyMatching(query, &result)
	C.CFRelease(C.CFTypeRef(query))

	if status == C.errSecItemNotFound {
		return "", ErrNotFound
	}
	if status != C.errSecSuccess {
		return "", fmt.Errorf("keychain get failed: %d", status)
	}

	data := C.CFDataRef(result)
	length := C.CFDataGetLength(data)
	ptr := C.CFDataGetBytePtr(data)
	password := C.GoStringN((*C.char)(unsafe.Pointer(ptr)), C.int(length))
	C.CFRelease(result)

	return password, nil
}

func (k *macOSKeychain) Delete(ctx context.Context, service, account string) error {
	cservice := C.CString(service)
	caccount := C.CString(account)
	defer C.free(unsafe.Pointer(cservice))
	defer C.free(unsafe.Pointer(caccount))

	query := C.CFDictionaryCreateMutable(nil, 0, nil, nil)
	C.CFDictionaryAddValue(query, C.kSecClass, C.kSecClassGenericPassword)
	C.CFDictionaryAddValue(query, C.kSecAttrService, unsafe.Pointer(C.CFStringCreateWithCString(nil, cservice, 0)))
	C.CFDictionaryAddValue(query, C.kSecAttrAccount, unsafe.Pointer(C.CFStringCreateWithCString(nil, caccount, 0)))

	status := C.SecItemDelete(query)
	C.CFRelease(C.CFTypeRef(query))

	if status == C.errSecItemNotFound {
		return ErrNotFound
	}
	if status != C.errSecSuccess {
		return fmt.Errorf("keychain delete failed: %d", status)
	}

	return nil
}

func (k *macOSKeychain) List(ctx context.Context, service string) ([]Account, error) {
	return nil, fmt.Errorf("list not implemented on macOS")
}
```

## Linux Implementation

```go
//go:build linux
// +build linux

package keychain

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/zalando/go-keyring/secret_service"
)

type linuxKeychain struct {
	svc *secret_service.SecretService
}

func New() (Keychain, error) {
	svc, err := secret_service.NewService()
	if err != nil {
		return nil, fmt.Errorf("secret service init failed: %w", err)
	}
	return &linuxKeychain{svc: svc}, nil
}

func (k *linuxKeychain) Set(ctx context.Context, service, account, password string) error {
	return k.svc.Set(service, account, password)
}

func (k *linuxKeychain) Get(ctx context.Context, service, account string) (string, error) {
	password, err := k.svc.Get(service, account)
	if err == secret_service.ErrNotFound {
		return "", ErrNotFound
	}
	return password, err
}

func (k *linuxKeychain) Delete(ctx context.Context, service, account string) error {
	err := k.svc.Delete(service, account)
	if err == secret_service.ErrNotFound {
		return ErrNotFound
	}
	return err
}

func (k *linuxKeychain) List(ctx context.Context, service string) ([]Account, error) {
	items, err := k.svc.List(service)
	if err != nil {
		return nil, err
	}

	accounts := make([]Account, len(items))
	for i, item := range items {
		accounts[i] = Account{
			Service: service,
			Account: item.Account,
		}
	}
	return accounts, nil
}
```

## Windows Implementation

```go
//go:build windows
// +build windows

package keychain

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	credEnumerate = syscall.NewLazyDLL("Advapi32.dll").NewProc("CredEnumerateW")
	credFree      = syscall.NewLazyDLL("Advapi32.dll").NewProc("CredFree")
)

type windowsKeychain struct{}

func New() (Keychain, error) {
	return &windowsKeychain{}, nil
}

func (k *windowsKeychain) Set(ctx context.Context, service, account, password string) error {
	target := fmt.Sprintf("%s/%s", service, account)
	targetUTF16, _ := syscall.UTF16PtrFromString(target)
	passwordUTF16, _ := syscall.UTF16PtrFromString(password)

	cred := &windows.Credential{
		TargetName:   targetUTF16,
		Type:         windows.CRED_TYPE_GENERIC,
		CredentialBlobSize: uint32(len(password) * 2),
		CredentialBlob: uintptr(unsafe.Pointer(passwordUTF16)),
		Persist:      windows.CRED_PERSIST_LOCAL_MACHINE,
	}

	return windows.CredWrite(cred, 0)
}

func (k *windowsKeychain) Get(ctx context.Context, service, account string) (string, error) {
	target := fmt.Sprintf("%s/%s", service, account)
	cred, err := windows.CredRead(target, windows.CRED_TYPE_GENERIC, 0)
	if err != nil {
		if err == windows.ERROR_NOT_FOUND {
			return "", ErrNotFound
		}
		return "", err
	}
	defer windows.CredFree(cred)

	blob := (*uint16)(unsafe.Pointer(cred.CredentialBlob))
	return syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(blob))[:cred.CredentialBlobSize/2]), nil
}

func (k *windowsKeychain) Delete(ctx context.Context, service, account string) error {
	target := fmt.Sprintf("%s/%s", service, account)
	err := windows.CredDelete(target, windows.CRED_TYPE_GENERIC, 0)
	if err == windows.ERROR_NOT_FOUND {
		return ErrNotFound
	}
	return err
}

func (k *windowsKeychain) List(ctx context.Context, service string) ([]Account, error) {
	return nil, fmt.Errorf("list not implemented on Windows")
}
```

## Usage

```go
func StoreAPIToken(ctx context.Context, token string) error {
	kc, err := keychain.New()
	if err != nil {
		return err
	}
	
	return kc.Set(ctx, "muster", "api_token", token)
}

func GetAPIToken(ctx context.Context) (string, error) {
	kc, err := keychain.New()
	if err != nil {
		return "", err
	}
	
	return kc.Get(ctx, "muster", "api_token")
}
```

## Version

**1.0.0** - 2024-01-15
