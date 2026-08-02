//go:build windows
// +build windows

package auth

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	advapi32       = syscall.NewLazyDLL("advapi32.dll")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredEnumer = advapi32.NewProc("CredEnumerW")
	procCredFree   = advapi32.NewProc("CredFree")
)

const (
	CRED_TYPE_GENERIC          uint32 = 1
	CRED_PERSIST_LOCAL_MACHINE uint32 = 2
	ERROR_NOT_FOUND            uint32 = 1168
)

// CREDENTIAL structure
type CREDENTIAL struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     uintptr
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// CREDENTIALA structure for enumeration
type CREDENTIALA struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     uintptr
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// windowsKeychainNative implements Keychain using Windows Credential Manager
type windowsKeychainNative struct{}

// NewWindowsKeychainNative creates a new Windows Credential Manager client
func NewWindowsKeychainNative() (*windowsKeychainNative, error) {
	return &windowsKeychainNative{}, nil
}

// Set stores a credential in Windows Credential Manager
func (k *windowsKeychainNative) Set(service, account, password string) error {
	targetName := fmt.Sprintf("%s/%s", service, account)

	targetUTF16, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return fmt.Errorf("convert target name: %w", err)
	}

	passwordUTF16, err := syscall.UTF16PtrFromString(password)
	if err != nil {
		return fmt.Errorf("convert password: %w", err)
	}

	credential := &CREDENTIAL{
		Flags:              0,
		Type:               CRED_TYPE_GENERIC,
		TargetName:         targetUTF16,
		Comment:            nil,
		LastWritten:        syscall.Filetime{},
		CredentialBlobSize: uint32(len(password) * 2),
		CredentialBlob:     uintptr(unsafe.Pointer(passwordUTF16)),
		Persist:            CRED_PERSIST_LOCAL_MACHINE,
		AttributeCount:     0,
		Attributes:         0,
		TargetAlias:        nil,
		UserName:           targetUTF16,
	}

	r0, _, _ := procCredWrite.Call(
		uintptr(unsafe.Pointer(credential)),
		0,
	)
	if r0 == 0 {
		return fmt.Errorf("write credential failed")
	}

	return nil
}

// Get retrieves a credential from Windows Credential Manager
func (k *windowsKeychainNative) Get(service, account string) (string, error) {
	targetName := fmt.Sprintf("%s/%s", service, account)

	targetUTF16, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return "", fmt.Errorf("convert target name: %w", err)
	}

	var cred *CREDENTIALA
	r0, _, _ := procCredRead.Call(
		uintptr(unsafe.Pointer(targetUTF16)),
		uintptr(CRED_TYPE_GENERIC),
		0,
		uintptr(unsafe.Pointer(&cred)),
	)
	if r0 == 0 {
		return "", ErrNotFound
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(cred)))

	// Convert UTF-16 blob to Go string
	blobPtr := (*uint16)(unsafe.Pointer(cred.CredentialBlob))
	blobSlice := (*[1 << 30]uint16)(unsafe.Pointer(blobPtr))[:cred.CredentialBlobSize/2]
	password := syscall.UTF16ToString(blobSlice)

	return password, nil
}

// Delete removes a credential from Windows Credential Manager
func (k *windowsKeychainNative) Delete(service, account string) error {
	targetName := fmt.Sprintf("%s/%s", service, account)

	targetUTF16, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return fmt.Errorf("convert target name: %w", err)
	}

	r0, _, _ := procCredDelete.Call(
		uintptr(unsafe.Pointer(targetUTF16)),
		uintptr(CRED_TYPE_GENERIC),
		0,
	)
	if r0 == 0 {
		return ErrNotFound
	}

	return nil
}

// List returns all credentials for a service
func (k *windowsKeychainNative) List(service string) ([]Account, error) {
	serviceUTF16, err := syscall.UTF16PtrFromString(service)
	if err != nil {
		return nil, fmt.Errorf("convert service name: %w", err)
	}

	var count uint32
	var creds *CREDENTIALA
	r0, _, _ := procCredEnumer.Call(
		uintptr(unsafe.Pointer(serviceUTF16)),
		0,
		uintptr(unsafe.Pointer(&creds)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r0 == 0 {
		return []Account{}, nil
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(creds)))

	// Convert enumerated credentials to Account slice
	accounts := make([]Account, 0, count)
	credArray := (*[1 << 30]*CREDENTIALA)(unsafe.Pointer(creds))[:count]
	for i := 0; i < int(count); i++ {
		cred := credArray[i]
		if cred.TargetName != nil {
			targetName := syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(cred.TargetName))[:])
			accounts = append(accounts, Account{
				Account: targetName,
				Created: time.Unix(0, int64(cred.LastWritten.HighDateTime)<<32|int64(cred.LastWritten.LowDateTime)),
			})
		}
	}

	return accounts, nil
}
