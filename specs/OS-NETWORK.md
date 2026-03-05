# OS Network Specification

Complete specification for network interface access, proxy detection, and socket operations.

## Overview

Cross-platform network utilities for interface enumeration, IP detection, proxy configuration, and socket operations.

## Interface Discovery

```go
package netutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Interface represents a network interface
type Interface struct {
	Name        string
	HardwareAddr string
	Flags       []string
	Addresses   []Address
}

// Address represents a network address
type Address struct {
	IP      string
	Netmask string
	CIDR    string
}

// ListInterfaces returns all network interfaces
func ListInterfaces() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []Interface
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		intf := Interface{
			Name:         iface.Name,
			HardwareAddr: iface.HardwareAddr.String(),
			Flags:        parseFlags(iface.Flags),
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				intf.Addresses = append(intf.Addresses, Address{
					IP:      ipnet.IP.String(),
					Netmask: net.IP(ipnet.Mask).String(),
					CIDR:    addr.String(),
				})
			}
		}

		result = append(result, intf)
	}

	return result, nil
}

func parseFlags(flags net.Flags) []string {
	var result []string
	if flags&net.FlagUp != 0 {
		result = append(result, "up")
	}
	if flags&net.FlagBroadcast != 0 {
		result = append(result, "broadcast")
	}
	if flags&net.FlagLoopback != 0 {
		result = append(result, "loopback")
	}
	if flags&net.FlagMulticast != 0 {
		result = append(result, "multicast")
	}
	return result
}
```

## IP Detection

```go
// GetLocalIP returns the local IP address
func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// GetPublicIP retrieves public IP from external service
func GetPublicIP(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org", nil)
	
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	return strings.TrimSpace(string(body)), nil
}
```

## Proxy Detection

```go
//go:build !windows
// +build !windows

package netutil

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ProxyConfig holds proxy settings
type ProxyConfig struct {
	HTTP    *url.URL
	HTTPS   *url.URL
	NoProxy []string
}

// GetProxyConfig detects proxy configuration from environment
func GetProxyConfig() (*ProxyConfig, error) {
	config := &ProxyConfig{}

	if httpProxy := os.Getenv("HTTP_PROXY"); httpProxy == "" {
		httpProxy = os.Getenv("http_proxy")
	}
	if httpProxy != "" {
		u, err := url.Parse(httpProxy)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP_PROXY: %w", err)
		}
		config.HTTP = u
	}

	if httpsProxy := os.Getenv("HTTPS_PROXY"); httpsProxy == "" {
		httpsProxy = os.Getenv("https_proxy")
	}
	if httpsProxy != "" {
		u, err := url.Parse(httpsProxy)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTPS_PROXY: %w", err)
		}
		config.HTTPS = u
	}

	if noProxy := os.Getenv("NO_PROXY"); noProxy == "" {
		noProxy = os.Getenv("no_proxy")
	}
	if noProxy != "" {
		config.NoProxy = strings.Split(noProxy, ",")
	}

	return config, nil
}

// ShouldProxy returns true if host should use proxy
func (c *ProxyConfig) ShouldProxy(host string) bool {
	host = strings.ToLower(host)
	for _, pattern := range c.NoProxy {
		if matched, _ := filepath.Match(pattern, host); matched {
			return false
		}
		if strings.HasPrefix(host, pattern+".") {
			return false
		}
	}
	return true
}
```

## Socket Operations

```go
// DialUnix connects to a Unix domain socket
func DialUnix(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}

// ListenUnix creates a Unix domain socket listener
func ListenUnix(path string) (net.Listener, error) {
	// Remove existing socket
	os.Remove(path)
	
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	// Set permissions
	os.Chmod(path, 0660)
	
	return listener, nil
}

// TCPKeepAliveListener wraps net.Listener with TCP keepalive
func TCPKeepAliveListener(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &tcpKeepAliveListener{ln.(*net.TCPListener)}, nil
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}
```

## Port Utilities

```go
// FindFreePort finds an available TCP port
func FindFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port, nil
}

// IsPortOpen checks if TCP port is reachable
func IsPortOpen(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
```

## Usage Example

```go
func CheckNetwork(ctx context.Context) error {
	ifaces, err := netutil.ListInterfaces()
	if err != nil {
		return err
	}

	localIP, _ := netutil.GetLocalIP()
	publicIP, _ := netutil.GetPublicIP(ctx)
	
	proxy, _ := netutil.GetProxyConfig()
	
	return nil
}
```

## Version

**1.0.0** - 2024-01-15
