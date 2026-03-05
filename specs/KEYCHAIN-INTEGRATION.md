# Keychain Integration Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Security

## Overview

Complete specification of **OS keychain integration** for secure credential storage across platforms.

## 1. Platform Abstraction

```go
type Keychain interface {
    Set(service, account, password string) error
    Get(service, account string) (string, error)
    Delete(service, account string) error
    List(service string) ([]Account, error)
}

type Account struct {
    Account string
    Created time.Time
}
```

## 2. macOS Keychain

```go
type MacOSKeychain struct{}

func (k *MacOSKeychain) Set(service, account, password string) error {
    cmd := exec.Command("security", "add-generic-password",
        "-s", service,
        "-a", account,
        "-w", password,
        "-U", // Update if exists
    )
    return cmd.Run()
}

func (k *MacOSKeychain) Get(service, account string) (string, error) {
    cmd := exec.Command("security", "find-generic-password",
        "-s", service,
        "-a", account,
        "-w", // Output password only
    )
    
    output, err := cmd.Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            if strings.Contains(string(exitErr.Stderr), "could not be found") {
                return "", ErrNotFound
            }
        }
        return "", err
    }
    
    return strings.TrimSpace(string(output)), nil
}
```

## 3. Linux (Secret Service / libsecret)

```go
type LinuxKeychain struct {
    collection dbus.BusObject
}

func (k *LinuxKeychain) Set(service, account, password string) error {
    attributes := map[string]string{
        "service": service,
        "account": account,
    }
    
    _, err := k.collection.Call(
        "org.freedesktop.Secret.Collection.CreateItem",
        0,
        map[string]dbus.Variant{
            "org.freedesktop.Secret.Item.Label": dbus.MakeVariant(fmt.Sprintf("%s/%s", service, account)),
            "org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(attributes),
        },
        dbus.MakeVariant(password),
        true,
    ).Store()
    
    return err
}
```

## 4. Windows Credential Manager

```go
type WindowsKeychain struct{}

func (k *WindowsKeychain) Set(service, account, password string) error {
    cred := windows.Credential{
        TargetName: syscall.StringToUTF16Ptr(service + "/" + account),
        Type:       windows.CRED_TYPE_GENERIC,
        CredentialBlob: []byte(password),
        Persist:    windows.CRED_PERSIST_LOCAL_MACHINE,
    }
    
    return windows.CredWrite(&cred, 0)
}

func (k *WindowsKeychain) Get(service, account string) (string, error) {
    var cred *windows.Credential
    err := windows.CredRead(
        syscall.StringToUTF16Ptr(service+"/"+account),
        windows.CRED_TYPE_GENERIC,
        0,
        &cred,
    )
    if err != nil {
        if err == windows.ERROR_NOT_FOUND {
            return "", ErrNotFound
        }
        return "", err
    }
    defer windows.CredFree(cred)
    
    return string(cred.CredentialBlob), nil
}
```

## 5. Fallback (Encrypted File)

```go
type FileKeychain struct {
    path string
    key  []byte
}

func (k *FileKeychain) Set(service, account, password string) error {
    data, err := k.load()
    if err != nil {
        data = make(map[string]string)
    }
    
    key := fmt.Sprintf("%s/%s", service, account)
    data[key] = password
    
    return k.save(data)
}

func (k *FileKeychain) save(data map[string]string) error {
    jsonData, _ := json.Marshal(data)
    encrypted, _ := encrypt(jsonData, k.key)
    return os.WriteFile(k.path, encrypted, 0600)
}
```

## 6. Factory

```go
func NewKeychain() (Keychain, error) {
    switch runtime.GOOS {
    case "darwin":
        return &MacOSKeychain{}, nil
    case "linux":
        return NewLinuxKeychain()
    case "windows":
        return &WindowsKeychain{}, nil
    default:
        return NewFileKeychain()
    }
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
