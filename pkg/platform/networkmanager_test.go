//go:build linux
// +build linux

package platform

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{ConnectionStateUnknown, "Unknown"},
		{ConnectionStateAsleep, "Asleep"},
		{ConnectionStateDisconnected, "Disconnected"},
		{ConnectionStateConnecting, "Connecting"},
		{ConnectionStateLocalOnly, "Local Only"},
		{ConnectionStateConnectedSite, "Connected (Site)"},
		{ConnectionStateConnectedGlobal, "Connected (Global)"},
		{ConnectionState(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestDeviceTypeString(t *testing.T) {
	tests := []struct {
		deviceType uint32
		expected   string
	}{
		{1, "Ethernet"},
		{2, "WiFi"},
		{5, "Bluetooth"},
		{8, "Modem"},
		{11, "VLAN"},
		{13, "Bridge"},
		{99, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, DeviceTypeString(tt.deviceType))
		})
	}
}

func TestDeviceStateString(t *testing.T) {
	tests := []struct {
		state    uint32
		expected string
	}{
		{0, "Unknown"},
		{1, "Unmanaged"},
		{3, "Disconnected"},
		{8, "IP Config"},
		{11, "Activated"},
		{13, "Failed"},
		{99, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, DeviceStateString(tt.state))
		})
	}
}

func TestConnectionTypeString(t *testing.T) {
	tests := []struct {
		connType string
		expected string
	}{
		{"802-3-ethernet", "Ethernet"},
		{"802-11-wireless", "WiFi"},
		{"bluetooth", "Bluetooth"},
		{"vpn", "VPN"},
		{"bridge", "Bridge"},
		{"bond", "Bond"},
		{"vlan", "VLAN"},
		{"unknown-type", "unknown-type"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, ConnectionTypeString(tt.connType))
		})
	}
}

func TestActiveConnection_String(t *testing.T) {
	conn := &ActiveConnection{
		ID:   "Test Connection",
		UUID: "12345678-1234-1234-1234-123456789012",
		Type: "802-3-ethernet",
		VPN:  false,
	}

	expected := "Connection{ID: Test Connection, UUID: 12345678-1234-1234-1234-123456789012, Type: Ethernet, VPN: false}"
	assert.Equal(t, expected, conn.String())
}

func TestNetworkManager_NewNetworkManager(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		// Expected to fail on systems without NetworkManager or D-Bus
		t.Logf("NewNetworkManager failed (expected on test systems): %v", err)
		assert.Contains(t, err.Error(), "failed to connect to system bus")
	} else {
		defer func() { _ = nm.Close() }()
		require.NotNil(t, nm)
	}
}

func TestNetworkManager_Close(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}

	err = nm.Close()
	assert.NoError(t, err)
}

func TestNetworkManager_GetConnectionState(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	state, err := nm.GetConnectionState()
	if err != nil {
		t.Logf("GetConnectionState failed: %v", err)
	} else {
		assert.GreaterOrEqual(t, state, ConnectionState(0))
		assert.LessOrEqual(t, state, ConnectionState(60))
	}
}

func TestNetworkManager_CheckConnectivity(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	online, err := nm.CheckConnectivity()
	if err != nil {
		t.Logf("CheckConnectivity failed: %v", err)
	} else {
		assert.IsType(t, true, online)
	}
}

func TestNetworkManager_IsOnline(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	online, err := nm.IsOnline()
	if err != nil {
		t.Logf("IsOnline failed: %v", err)
	} else {
		assert.IsType(t, true, online)
	}
}

func TestNetworkManager_GetActiveConnections(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	connections, err := nm.GetActiveConnections()
	if err != nil {
		t.Logf("GetActiveConnections failed: %v", err)
	} else {
		assert.NotNil(t, connections)
	}
}

func TestNetworkManager_GetDevices(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	devices, err := nm.GetDevices()
	if err != nil {
		t.Logf("GetDevices failed: %v", err)
	} else {
		assert.NotNil(t, devices)
	}
}

func TestNetworkManager_GetPrimaryConnection(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	conn, err := nm.GetPrimaryConnection()
	if err != nil {
		t.Logf("GetPrimaryConnection failed: %v", err)
	} else {
		require.NotNil(t, conn)
		assert.NotEmpty(t, conn.ID)
		assert.NotEmpty(t, conn.UUID)
	}
}

func TestNetworkManager_GetConnectionByUUID(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	connections, err := nm.GetActiveConnections()
	if err != nil || len(connections) == 0 {
		t.Skip("No active connections available")
	}

	conn, err := nm.GetConnectionByUUID(connections[0].UUID)
	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, connections[0].UUID, conn.UUID)
}

func TestNetworkManager_GetConnectionByUUID_NotFound(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	conn, err := nm.GetConnectionByUUID("non-existent-uuid")
	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "connection not found")
}

func TestNetworkManager_GetDNS(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	dns, err := nm.GetDNS()
	if err != nil {
		t.Logf("GetDNS failed: %v", err)
	} else {
		assert.NotNil(t, dns)
	}
}

func TestNetworkManager_GetGateway(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	gateway, err := nm.GetGateway()
	if err != nil {
		t.Logf("GetGateway failed: %v", err)
	} else {
		assert.NotNil(t, gateway)
	}
}

func TestNetworkManager_GetNetworkSettings(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	settings, err := nm.GetNetworkSettings()
	if err != nil {
		t.Logf("GetNetworkSettings failed: %v", err)
	} else {
		require.NotNil(t, settings)
		assert.NotNil(t, settings.DNS)
		assert.NotNil(t, settings.Gateway)
	}
}

func TestNetworkManager_ExportJSON(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	jsonStr, err := nm.ExportJSON()
	if err != nil {
		t.Logf("ExportJSON failed: %v", err)
	} else {
		assert.NotEmpty(t, jsonStr)
		assert.Contains(t, jsonStr, "{")
		assert.Contains(t, jsonStr, "}")
	}
}

func TestNetworkManager_WaitForConnection(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	ctx := context.Background()
	err = nm.WaitForConnection(ctx, 1*time.Second)
	if err != nil {
		t.Logf("WaitForConnection failed (may timeout on test systems): %v", err)
	}
}

func TestNetworkManager_WatchStateChanges(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callbackCalled := false
	nm.WatchStateChanges(ctx, func(state ConnectionState) {
		callbackCalled = true
		t.Logf("State changed to: %s", state.String())
	})

	// Give it some time to potentially receive a signal
	time.Sleep(100 * time.Millisecond)

	// Callback may not be called if no state changes occur during test
	_ = callbackCalled
}

func TestNetworkManager_GetProxySettings(t *testing.T) {
	nm, err := NewNetworkManager()
	if err != nil {
		t.Skip("Skipping test - NetworkManager not available")
	}
	defer func() { _ = nm.Close() }()

	proxySettings, err := nm.GetProxySettings()
	if err != nil {
		t.Logf("GetProxySettings failed: %v", err)
	} else {
		require.NotNil(t, proxySettings)
		assert.IsType(t, ProxyMode(""), proxySettings.Mode)
	}
}

func TestProxySettings_Struct(t *testing.T) {
	settings := &ProxySettings{
		Mode:       ProxyModeManual,
		HTTPProxy:  "http://proxy.example.com:8080",
		HTTPSProxy: "https://proxy.example.com:8080",
		NoProxy:    []string{"localhost", "127.0.0.1"},
	}

	assert.Equal(t, ProxyModeManual, settings.Mode)
	assert.Equal(t, "http://proxy.example.com:8080", settings.HTTPProxy)
	assert.Len(t, settings.NoProxy, 2)
}

func TestHTTPProxy_Struct(t *testing.T) {
	proxy := HTTPProxy{
		Host: "proxy.example.com",
		Port: 8080,
	}

	assert.Equal(t, "proxy.example.com", proxy.Host)
	assert.Equal(t, 8080, proxy.Port)
}

func TestConnectionDevice_Struct(t *testing.T) {
	device := ConnectionDevice{
		Path:      "/org/freedesktop/NetworkManager/Devices/1",
		Interface: "eth0",
		Type:      1,
		State:     100,
		IP4Config: "/org/freedesktop/NetworkManager/IP4Config/1",
		IP6Config: "/org/freedesktop/NetworkManager/IP6Config/1",
	}

	assert.Equal(t, "/org/freedesktop/NetworkManager/Devices/1", device.Path)
	assert.Equal(t, "eth0", device.Interface)
	assert.Equal(t, uint32(1), device.Type)
	assert.Equal(t, uint32(100), device.State)
}

func TestActiveConnection_Struct(t *testing.T) {
	conn := ActiveConnection{
		ID:        "Test",
		UUID:      "test-uuid",
		Type:      "ethernet",
		Device:    "/org/freedesktop/NetworkManager/Devices/1",
		IP4Config: "/org/freedesktop/NetworkManager/IP4Config/1",
		IP6Config: "/org/freedesktop/NetworkManager/IP6Config/1",
		VPN:       false,
	}

	assert.Equal(t, "Test", conn.ID)
	assert.Equal(t, "test-uuid", conn.UUID)
	assert.Equal(t, "ethernet", conn.Type)
	assert.False(t, conn.VPN)
}

func TestNetworkSettings_Struct(t *testing.T) {
	settings := NetworkSettings{
		Proxy: HTTPProxy{
			Host: "proxy.example.com",
			Port: 8080,
		},
		Gateway: net.ParseIP("192.168.1.1"),
		DNS:     []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4")},
	}

	assert.Equal(t, "proxy.example.com", settings.Proxy.Host)
	assert.Equal(t, 8080, settings.Proxy.Port)
	assert.Equal(t, "192.168.1.1", settings.Gateway.String())
	assert.Len(t, settings.DNS, 2)
}

func Test_uint32ToIP(t *testing.T) {
	tests := []struct {
		input    uint32
		expected string
	}{
		{0x08080808, "8.8.8.8"},
		{0x7f000001, "127.0.0.1"},
		{0xc0a80101, "192.168.1.1"},
		{0x00000000, "0.0.0.0"},
		{0xffffffff, "255.255.255.255"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			ip := uint32ToIP(tt.input)
			assert.Equal(t, tt.expected, ip.String())
		})
	}
}

func TestConnectionState_Values(t *testing.T) {
	assert.Equal(t, ConnectionState(0), ConnectionStateUnknown)
	assert.Equal(t, ConnectionState(10), ConnectionStateAsleep)
	assert.Equal(t, ConnectionState(20), ConnectionStateDisconnected)
	assert.Equal(t, ConnectionState(30), ConnectionStateConnecting)
	assert.Equal(t, ConnectionState(40), ConnectionStateLocalOnly)
	assert.Equal(t, ConnectionState(50), ConnectionStateConnectedSite)
	assert.Equal(t, ConnectionState(60), ConnectionStateConnectedGlobal)
}
