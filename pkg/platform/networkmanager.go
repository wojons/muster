//go:build linux
// +build linux

// NetworkManager integration for Linux
// Requires: github.com/godbus/dbus/v5

package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	nmBusName       = "org.freedesktop.NetworkManager"
	nmPath          = "/org/freedesktop/NetworkManager"
	nmInterface     = "org.freedesktop.NetworkManager"
	nmSettingsPath  = "/org/freedesktop/NetworkManager/Settings"
	nmSettingsIface = "org.freedesktop.NetworkManager.Settings"
)

// ConnectionState represents the network connectivity state
type ConnectionState int

const (
	ConnectionStateUnknown         ConnectionState = 0
	ConnectionStateAsleep          ConnectionState = 10
	ConnectionStateDisconnected    ConnectionState = 20
	ConnectionStateConnecting      ConnectionState = 30
	ConnectionStateLocalOnly       ConnectionState = 40
	ConnectionStateConnectedSite   ConnectionState = 50
	ConnectionStateConnectedGlobal ConnectionState = 60
)

// String returns a human-readable representation of the connection state
func (s ConnectionState) String() string {
	switch s {
	case ConnectionStateAsleep:
		return "Asleep"
	case ConnectionStateDisconnected:
		return "Disconnected"
	case ConnectionStateConnecting:
		return "Connecting"
	case ConnectionStateLocalOnly:
		return "Local Only"
	case ConnectionStateConnectedSite:
		return "Connected (Site)"
	case ConnectionStateConnectedGlobal:
		return "Connected (Global)"
	default:
		return "Unknown"
	}
}

// ConnectionDevice represents a network device
type ConnectionDevice struct {
	Path      string
	Interface string
	Type      uint32
	State     uint32
	IP4Config string
	IP6Config string
}

// ActiveConnection represents an active network connection
type ActiveConnection struct {
	ID        string
	UUID      string
	Type      string
	Device    string
	IP4Config string
	IP6Config string
	VPN       bool
}

// NetworkSettings represents network configuration
type NetworkSettings struct {
	Proxy   HTTPProxy
	Gateway net.IP
	DNS     []net.IP
}

// HTTPProxy represents proxy configuration
type HTTPProxy struct {
	Host string
	Port int
}

// NetworkManager provides integration with NetworkManager
// Uses org.freedesktop.NetworkManager D-Bus interface for network management
type NetworkManager struct {
	conn     *dbus.Conn
	nm       dbus.BusObject
	settings dbus.BusObject
	signals  chan *dbus.Signal
}

// NewNetworkManager creates a new NetworkManager instance
func NewNetworkManager() (*NetworkManager, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system bus: %w", err)
	}

	nm := &NetworkManager{
		conn:     conn,
		nm:       conn.Object(nmBusName, dbus.ObjectPath(nmPath)),
		settings: conn.Object(nmBusName, dbus.ObjectPath(nmSettingsPath)),
		signals:  make(chan *dbus.Signal, 10),
	}

	conn.Signal(nm.signals)

	// Subscribe to state changes
	if err := nm.subscribeToStateChanges(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return nm, nil
}

func (nm *NetworkManager) subscribeToStateChanges() error {
	// Add match rule for NetworkManager state changes
	matchRule := "type='signal',interface='org.freedesktop.NetworkManager',member='StateChanged'"
	return nm.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err
}

// GetConnectionState returns the current network connectivity state
func (nm *NetworkManager) GetConnectionState() (ConnectionState, error) {
	var state uint32
	err := nm.nm.Call(nmInterface+".state", 0).Store(&state)
	if err != nil {
		return ConnectionStateUnknown, err
	}

	return ConnectionState(state), nil
}

// GetActiveConnections returns the currently active connections
func (nm *NetworkManager) GetActiveConnections() ([]ActiveConnection, error) {
	var paths []dbus.ObjectPath
	err := nm.nm.Call(nmInterface+".GetActiveConnections", 0).Store(&paths)
	if err != nil {
		return nil, err
	}

	var connections []ActiveConnection
	for _, path := range paths {
		conn, err := nm.getActiveConnectionDetails(path)
		if err != nil {
			continue
		}
		connections = append(connections, *conn)
	}

	return connections, nil
}

func (nm *NetworkManager) getActiveConnectionDetails(path dbus.ObjectPath) (*ActiveConnection, error) {
	obj := nm.conn.Object(nmBusName, path)

	var props map[string]dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0,
		"org.freedesktop.NetworkManager.Connection.Active").Store(&props)
	if err != nil {
		return nil, err
	}

	conn := &ActiveConnection{}

	if v, ok := props["Id"]; ok {
		conn.ID = v.Value().(string)
	}
	if v, ok := props["Uuid"]; ok {
		conn.UUID = v.Value().(string)
	}
	if v, ok := props["Type"]; ok {
		conn.Type = v.Value().(string)
	}
	if v, ok := props["Vpn"]; ok {
		conn.VPN = v.Value().(bool)
	}
	if v, ok := props["Devices"]; ok {
		if devices, ok := v.Value().([]dbus.ObjectPath); ok && len(devices) > 0 {
			conn.Device = string(devices[0])
		}
	}
	if v, ok := props["Ip4Config"]; ok {
		conn.IP4Config = v.Value().(string)
	}
	if v, ok := props["Ip6Config"]; ok {
		conn.IP6Config = v.Value().(string)
	}

	return conn, nil
}

// GetDevices returns all network devices
func (nm *NetworkManager) GetDevices() ([]ConnectionDevice, error) {
	var devicePaths []dbus.ObjectPath
	err := nm.nm.Call(nmInterface+".GetDevices", 0).Store(&devicePaths)
	if err != nil {
		return nil, err
	}

	var devices []ConnectionDevice
	for _, path := range devicePaths {
		device, err := nm.getDeviceDetails(path)
		if err != nil {
			continue
		}
		devices = append(devices, *device)
	}

	return devices, nil
}

func (nm *NetworkManager) getDeviceDetails(path dbus.ObjectPath) (*ConnectionDevice, error) {
	obj := nm.conn.Object(nmBusName, path)

	var props map[string]dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0,
		"org.freedesktop.NetworkManager.Device.Generic").Store(&props)
	if err != nil {
		// Try specific device types
		err = obj.Call("org.freedesktop.DBus.Properties.GetAll", 0,
			"org.freedesktop.NetworkManager.Device.Wired").Store(&props)
		if err != nil {
			err = obj.Call("org.freedesktop.DBus.Properties.GetAll", 0,
				"org.freedesktop.NetworkManager.Device.Wireless").Store(&props)
			if err != nil {
				return nil, err
			}
		}
	}

	device := &ConnectionDevice{
		Path: string(path),
	}

	if v, ok := props["Interface"]; ok {
		device.Interface = v.Value().(string)
	}
	if v, ok := props["DeviceType"]; ok {
		if dt, ok := v.Value().(uint32); ok {
			device.Type = dt
		}
	}
	if v, ok := props["State"]; ok {
		device.State = v.Value().(uint32)
	}
	if v, ok := props["Ip4Config"]; ok {
		device.IP4Config = v.Value().(string)
	}
	if v, ok := props["Ip6Config"]; ok {
		device.IP6Config = v.Value().(string)
	}

	return device, nil
}

// CheckConnectivity tests internet connectivity
func (nm *NetworkManager) CheckConnectivity() (bool, error) {
	state, err := nm.GetConnectionState()
	if err != nil {
		return false, err
	}

	return state == ConnectionStateConnectedGlobal, nil
}

// GetPrimaryConnection returns the primary active connection
func (nm *NetworkManager) GetPrimaryConnection() (*ActiveConnection, error) {
	var primaryPath dbus.ObjectPath
	err := nm.nm.Call(nmInterface+".PrimaryConnection", 0).Store(&primaryPath)
	if err != nil {
		return nil, err
	}

	if primaryPath == "" {
		return nil, fmt.Errorf("no primary connection")
	}

	return nm.getActiveConnectionDetails(primaryPath)
}

// GetProxySettings retrieves proxy configuration for the active connection
func (nm *NetworkManager) GetProxySettings() (*ProxySettings, error) {
	// Get proxy method
	method, err := nm.getProxyMethod()
	if err != nil {
		return nil, err
	}

	switch method {
	case "none":
		return &ProxySettings{Mode: ProxyModeDirect}, nil
	case "auto":
		pacURL, err := nm.getPacURL()
		if err != nil {
			return nil, err
		}
		return &ProxySettings{Mode: ProxyModeAuto, PACURL: pacURL}, nil
	case "manual":
		return nm.getManualProxySettings()
	default:
		return &ProxySettings{Mode: ProxyModeDirect}, nil
	}
}

// ProxyMode represents the proxy configuration mode
type ProxyMode string

const (
	ProxyModeDirect ProxyMode = "direct"
	ProxyModeAuto   ProxyMode = "auto"
	ProxyModeManual ProxyMode = "manual"
)

// ProxySettings holds proxy configuration
type ProxySettings struct {
	Mode       ProxyMode
	HTTPProxy  string
	HTTPSProxy string
	FTPProxy   string
	SOCKSProxy string
	NoProxy    []string
	PACURL     string
}

func (nm *NetworkManager) getProxyMethod() (string, error) {
	var method string
	err := nm.nm.Call(nmInterface+".GetProxyMethod", 0).Store(&method)
	return method, err
}

func (nm *NetworkManager) getPacURL() (string, error) {
	var pacURL string
	err := nm.nm.Call(nmInterface+".GetProxyPacUrl", 0).Store(&pacURL)
	return pacURL, err
}

func (nm *NetworkManager) getManualProxySettings() (*ProxySettings, error) {
	var config map[string]dbus.Variant
	err := nm.nm.Call(nmInterface+".GetProxyConfiguration", 0).Store(&config)
	if err != nil {
		return nil, err
	}

	settings := &ProxySettings{Mode: ProxyModeManual}

	if v, ok := config["http"]; ok {
		settings.HTTPProxy = v.Value().(string)
	}
	if v, ok := config["https"]; ok {
		settings.HTTPSProxy = v.Value().(string)
	}
	if v, ok := config["ftp"]; ok {
		settings.FTPProxy = v.Value().(string)
	}
	if v, ok := config["socks"]; ok {
		settings.SOCKSProxy = v.Value().(string)
	}
	if v, ok := config["no-proxy"]; ok {
		if noProxy, ok := v.Value().([]string); ok {
			settings.NoProxy = noProxy
		}
	}

	return settings, nil
}

// SetProxy configures proxy settings
func (nm *NetworkManager) SetProxy(proxy HTTPProxy) error {
	config := map[string]dbus.Variant{
		"http": dbus.MakeVariant(proxy.Host),
	}

	return nm.nm.Call(nmInterface+".SetProxyConfiguration", 0, config).Store()
}

// WatchStateChanges monitors network state changes
func (nm *NetworkManager) WatchStateChanges(ctx context.Context, callback func(ConnectionState)) {
	go func() {
		for {
			select {
			case sig := <-nm.signals:
				if sig.Name == nmInterface+".StateChanged" {
					if len(sig.Body) > 0 {
						if state, ok := sig.Body[0].(uint32); ok {
							callback(ConnectionState(state))
						}
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Close releases resources
func (nm *NetworkManager) Close() error {
	if nm.signals != nil {
		close(nm.signals)
	}
	if nm.conn != nil {
		return nm.conn.Close()
	}
	return nil
}

// GetDNS returns DNS servers for the primary connection
func (nm *NetworkManager) GetDNS() ([]net.IP, error) {
	primary, err := nm.GetPrimaryConnection()
	if err != nil {
		return nil, err
	}

	if primary.IP4Config == "" {
		return nil, fmt.Errorf("no IPv4 config")
	}

	obj := nm.conn.Object(nmBusName, dbus.ObjectPath(primary.IP4Config))
	var props map[string]dbus.Variant
	err = obj.Call("org.freedesktop.DBus.Properties.GetAll", 0,
		"org.freedesktop.NetworkManager.IP4Config").Store(&props)
	if err != nil {
		return nil, err
	}

	var dnsServers []net.IP
	if v, ok := props["Nameservers"]; ok {
		if servers, ok := v.Value().([]uint32); ok {
			for _, server := range servers {
				dnsServers = append(dnsServers, uint32ToIP(server))
			}
		}
	}

	return dnsServers, nil
}

// GetGateway returns the gateway IP for the primary connection
func (nm *NetworkManager) GetGateway() (net.IP, error) {
	primary, err := nm.GetPrimaryConnection()
	if err != nil {
		return nil, err
	}

	if primary.IP4Config == "" {
		return nil, fmt.Errorf("no IPv4 config")
	}

	obj := nm.conn.Object(nmBusName, dbus.ObjectPath(primary.IP4Config))
	var props map[string]dbus.Variant
	err = obj.Call("org.freedesktop.DBus.Properties.GetAll", 0,
		"org.freedesktop.NetworkManager.IP4Config").Store(&props)
	if err != nil {
		return nil, err
	}

	if v, ok := props["Gateway"]; ok {
		gateway := v.Value().(string)
		return net.ParseIP(gateway), nil
	}

	return nil, fmt.Errorf("gateway not found")
}

// GetNetworkSettings retrieves current network configuration
func (nm *NetworkManager) GetNetworkSettings() (*NetworkSettings, error) {
	dns, err := nm.GetDNS()
	if err != nil {
		return nil, err
	}

	gateway, err := nm.GetGateway()
	if err != nil {
		return nil, err
	}

	proxySettings, err := nm.GetProxySettings()
	if err != nil {
		return nil, err
	}

	settings := &NetworkSettings{
		DNS:     dns,
		Gateway: gateway,
	}

	if proxySettings.HTTPProxy != "" {
		parsedURL, err := url.Parse(proxySettings.HTTPProxy)
		if err == nil {
			settings.Proxy.Host = parsedURL.Hostname()
			port := parsedURL.Port()
			if port != "" {
				_, _ = fmt.Sscanf(port, "%d", &settings.Proxy.Port)
			} else {
				settings.Proxy.Port = 80
			}
		}
	}

	return settings, nil
}

// Helper function to convert uint32 to net.IP (little-endian)
func uint32ToIP(ip uint32) net.IP {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

// IsOnline checks if the system is online
func (nm *NetworkManager) IsOnline() (bool, error) {
	state, err := nm.GetConnectionState()
	if err != nil {
		return false, err
	}
	return state >= ConnectionStateConnectedSite, nil
}

// WaitForConnection waits for a connection to become available
func (nm *NetworkManager) WaitForConnection(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			online, err := nm.IsOnline()
			if err != nil {
				continue
			}
			if online {
				return nil
			}
		}
	}
}

// GetConnectionByUUID retrieves a connection by its UUID
func (nm *NetworkManager) GetConnectionByUUID(uuid string) (*ActiveConnection, error) {
	connections, err := nm.GetActiveConnections()
	if err != nil {
		return nil, err
	}

	for _, conn := range connections {
		if conn.UUID == uuid {
			return &conn, nil
		}
	}

	return nil, fmt.Errorf("connection not found: %s", uuid)
}

// ExportJSON exports network settings as JSON
func (nm *NetworkManager) ExportJSON() (string, error) {
	settings, err := nm.GetNetworkSettings()
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// String returns a human-readable representation of the device type
func DeviceTypeString(deviceType uint32) string {
	switch deviceType {
	case 1:
		return "Ethernet"
	case 2:
		return "WiFi"
	case 3:
		return "Unused"
	case 4:
		return "Unused2"
	case 5:
		return "Bluetooth"
	case 6:
		return "OLPC Mesh"
	case 7:
		return "WiMAX"
	case 8:
		return "Modem"
	case 9:
		return "InfiniBand"
	case 10:
		return "Bond"
	case 11:
		return "VLAN"
	case 12:
		return "ADSL"
	case 13:
		return "Bridge"
	case 14:
		return "Generic"
	case 15:
		return "Team"
	case 16:
		return "Tun"
	case 17:
		return "IP Tunnel"
	case 18:
		return "Macvlan"
	case 19:
		return "Vxlan"
	case 20:
		return "Veth"
	default:
		return "Unknown"
	}
}

// DeviceStateString returns a human-readable representation of the device state
func DeviceStateString(state uint32) string {
	switch state {
	case 0:
		return "Unknown"
	case 1:
		return "Unmanaged"
	case 2:
		return "Unavailable"
	case 3:
		return "Disconnected"
	case 4:
		return "Prepare"
	case 5:
		return "Need Auth"
	case 6:
		return "Config"
	case 7:
		return "Need IP Config"
	case 8:
		return "IP Config"
	case 9:
		return "Checking IP"
	case 10:
		return "Secondaries"
	case 11:
		return "Activated"
	case 12:
		return "Deactivating"
	case 13:
		return "Failed"
	default:
		return "Unknown"
	}
}

// ConnectionTypeString returns a human-readable representation of the connection type
func ConnectionTypeString(connType string) string {
	switch connType {
	case "802-3-ethernet":
		return "Ethernet"
	case "802-11-wireless":
		return "WiFi"
	case "bluetooth":
		return "Bluetooth"
	case "vpn":
		return "VPN"
	case "bridge":
		return "Bridge"
	case "bond":
		return "Bond"
	case "vlan":
		return "VLAN"
	case "infiniband":
		return "InfiniBand"
	default:
		return connType
	}
}

// String returns a human-readable string representation of ActiveConnection
func (ac *ActiveConnection) String() string {
	return fmt.Sprintf("Connection{ID: %s, UUID: %s, Type: %s, VPN: %v}",
		ac.ID, ac.UUID, ConnectionTypeString(ac.Type), ac.VPN)
}
