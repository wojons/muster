//go:build windows
// +build windows

// Package platform provides Windows-specific platform integration for Muster.
//
// This file contains enhanced Windows platform features including:
//   - Full Windows Service Control Manager (SCM) integration
//   - Windows Defender compatibility checks
//   - Windows-specific path utilities
//   - Service lifecycle management with proper event handling
//
// The Windows service implementation uses golang.org/x/sys/windows/svc for
// native Windows service integration with support for start, stop, pause,
// continue, and shutdown operations.
//
// Example usage:
//
//	// Check if running as Windows service
//	if platform.IsWindowsService() {
//	    platform.RunAsService(server, logger)
//	}
//
//	// Install service
//	config := platform.WindowsServiceConfig{
//	    Name:        "MusterDaemon",
//	    DisplayName: "Muster MCP Daemon",
//	    Description: "Muster Model Context Protocol server",
//	    ConfigPath:  "C:\\ProgramData\\Muster\\config.yaml",
//	}
//	if err := platform.InstallWindowsService(config); err != nil {
//	    log.Fatal(err)
//	}
package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// WindowsServiceConfig contains configuration for Windows service installation
type WindowsServiceConfig struct {
	Name            string
	DisplayName     string
	Description     string
	BinaryPath      string
	ConfigPath      string
	Dependencies    []string
	StartType       uint32
	ErrorControl    uint32
	RecoveryActions []RecoveryAction
}

// RecoveryAction defines service recovery behavior
type RecoveryAction struct {
	Type  uint32
	Delay time.Duration
}

// WindowsServiceError represents service-related errors
type WindowsServiceError struct {
	Operation string
	Err       error
}

func (e *WindowsServiceError) Error() string {
	return fmt.Sprintf("Windows service %s failed: %v", e.Operation, e.Err)
}

// IsWindowsService returns true if the current process is running as a Windows service
func IsWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// InstallWindowsServiceEnhanced installs Muster as a Windows service with full configuration
func InstallWindowsServiceEnhanced(config WindowsServiceConfig) error {
	// Connect to Service Control Manager
	m, err := mgr.Connect()
	if err != nil {
		return &WindowsServiceError{Operation: "connect", Err: err}
	}
	defer m.Disconnect()

	// Get executable path if not provided
	binaryPath := config.BinaryPath
	if binaryPath == "" {
		exepath, err := os.Executable()
		if err != nil {
			return &WindowsServiceError{Operation: "get executable path", Err: err}
		}
		binaryPath = exepath
	}

	// Check if service already exists
	s, err := m.OpenService(config.Name)
	if err == nil {
		s.Close()
		return &WindowsServiceError{
			Operation: "create",
			Err:       fmt.Errorf("service %s already exists", config.Name),
		}
	}

	// Configure service
	mgrConfig := mgr.Config{
		StartType:    config.StartType,
		DisplayName:  config.DisplayName,
		Description:  config.Description,
		Dependencies: config.Dependencies,
		ErrorControl: config.ErrorControl,
	}

	// Set defaults
	if mgrConfig.StartType == 0 {
		mgrConfig.StartType = mgr.StartAutomatic
	}
	if mgrConfig.ErrorControl == 0 {
		mgrConfig.ErrorControl = mgr.ErrorNormal
	}
	if len(mgrConfig.Dependencies) == 0 {
		mgrConfig.Dependencies = []string{"Tcpip"}
	}

	// Build arguments
	args := []string{"daemon"}
	if config.ConfigPath != "" {
		args = append(args, "--config", config.ConfigPath)
	}

	// Create service
	s, err = m.CreateService(config.Name, binaryPath, mgrConfig, args...)
	if err != nil {
		return &WindowsServiceError{Operation: "create", Err: err}
	}
	defer s.Close()

	// Configure recovery actions
	if len(config.RecoveryActions) > 0 {
		recoveryActions := make([]mgr.RecoveryAction, len(config.RecoveryActions))
		for i, action := range config.RecoveryActions {
			recoveryActions[i] = mgr.RecoveryAction{
				Type:  int(action.Type),
				Delay: action.Delay,
			}
		}
		// Reset period: 24 hours
		if err := s.SetRecoveryActions(recoveryActions, uint32(24*time.Hour/time.Second)); err != nil {
			return &WindowsServiceError{Operation: "set recovery actions", Err: err}
		}
	}

	return nil
}

// UninstallWindowsServiceEnhanced removes the Windows service
func UninstallWindowsServiceEnhanced(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return &WindowsServiceError{Operation: "connect", Err: err}
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return &WindowsServiceError{Operation: "open service", Err: err}
	}
	defer s.Close()

	// Stop service if running
	status, err := s.Query()
	if err != nil {
		return &WindowsServiceError{Operation: "query status", Err: err}
	}

	if status.State == svc.Running {
		if _, err := s.Control(svc.Stop); err != nil {
			return &WindowsServiceError{Operation: "stop", Err: err}
		}

		// Wait for service to stop (max 30 seconds)
		timeout := time.After(30 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				return &WindowsServiceError{Operation: "stop", Err: fmt.Errorf("timeout waiting for service to stop")}
			case <-ticker.C:
				status, err := s.Query()
				if err != nil {
					return &WindowsServiceError{Operation: "query status", Err: err}
				}
				if status.State == svc.Stopped {
					goto deleteService
				}
			}
		}
	}

deleteService:
	if err := s.Delete(); err != nil {
		return &WindowsServiceError{Operation: "delete", Err: err}
	}

	return nil
}

// StartWindowsServiceEnhanced starts a Windows service
func StartWindowsServiceEnhanced(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return &WindowsServiceError{Operation: "connect", Err: err}
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return &WindowsServiceError{Operation: "open service", Err: err}
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return &WindowsServiceError{Operation: "start", Err: err}
	}

	return nil
}

// StopWindowsServiceEnhanced stops a Windows service
func StopWindowsServiceEnhanced(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return &WindowsServiceError{Operation: "connect", Err: err}
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return &WindowsServiceError{Operation: "open service", Err: err}
	}
	defer s.Close()

	if _, err := s.Control(svc.Stop); err != nil {
		return &WindowsServiceError{Operation: "stop", Err: err}
	}

	return nil
}

// QueryWindowsService returns the status of a Windows service
func QueryWindowsService(name string) (svc.State, error) {
	m, err := mgr.Connect()
	if err != nil {
		return svc.Stopped, &WindowsServiceError{Operation: "connect", Err: err}
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	if err != nil {
		return svc.Stopped, &WindowsServiceError{Operation: "open service", Err: err}
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return svc.Stopped, &WindowsServiceError{Operation: "query", Err: err}
	}

	return status.State, nil
}

// WindowsServiceStatus represents service status information
type WindowsServiceStatus struct {
	Name        string
	DisplayName string
	State       svc.State
	StateText   string
	Pid         uint32
}

// ListWindowsServices returns all installed Windows services
func ListWindowsServices() ([]WindowsServiceStatus, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, &WindowsServiceError{Operation: "connect", Err: err}
	}
	defer m.Disconnect()

	services := []WindowsServiceStatus{}

	// List all services
	svcNames, err := m.ListServices()
	if err != nil {
		return nil, &WindowsServiceError{Operation: "list services", Err: err}
	}

	for _, name := range svcNames {
		s, err := m.OpenService(name)
		if err != nil {
			continue
		}

		config, err := s.Config()
		if err != nil {
			s.Close()
			continue
		}

		status, err := s.Query()
		s.Close()

		svcStatus := WindowsServiceStatus{
			Name:        name,
			DisplayName: config.DisplayName,
			State:       svc.Stopped,
			StateText:   "Stopped",
		}

		if err == nil {
			svcStatus.State = status.State
			svcStatus.Pid = status.ProcessId
			svcStatus.StateText = serviceStateToString(status.State)
		}

		services = append(services, svcStatus)
	}

	return services, nil
}

func serviceStateToString(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "Stopped"
	case svc.StartPending:
		return "Start Pending"
	case svc.StopPending:
		return "Stop Pending"
	case svc.Running:
		return "Running"
	case svc.ContinuePending:
		return "Continue Pending"
	case svc.PausePending:
		return "Pause Pending"
	case svc.Paused:
		return "Paused"
	default:
		return fmt.Sprintf("Unknown (%d)", state)
	}
}

// WindowsServiceRunner is the interface for running as a Windows service
type WindowsServiceRunner interface {
	Run(ctx context.Context) error
	Stop() error
}

// windowsServiceHandler implements svc.Handler
type windowsServiceHandler struct {
	runner WindowsServiceRunner
	logger func(string, ...interface{})
}

// Execute implements svc.Handler.Execute
func (h *windowsServiceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the runner
	go func() {
		if err := h.runner.Run(ctx); err != nil {
			h.logger("service run error", "error", err)
		}
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				h.runner.Stop()
				return false, 0
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				h.logger("unexpected control request", "cmd", c.Cmd)
			}
		}
	}
}

// RunAsService runs the given runner as a Windows service
func RunAsService(runner WindowsServiceRunner, logger func(string, ...interface{})) error {
	return svc.Run("MusterDaemon", &windowsServiceHandler{
		runner: runner,
		logger: logger,
	})
}

// WindowsDefenderCheck contains Windows Defender compatibility information
type WindowsDefenderCheck struct {
	IsInstalled        bool
	IsEnabled          bool
	IsExclusionSet     bool
	ExclusionPath      string
	RealTimeProtection bool
	Version            string
	LastUpdate         time.Time
}

// CheckWindowsDefenderCompatibility checks Windows Defender status and compatibility
func CheckWindowsDefenderCompatibility(exePath string) (*WindowsDefenderCheck, error) {
	check := &WindowsDefenderCheck{}

	// Check if Windows Defender is installed using PowerShell
	cmd := exec.Command("powershell.exe", "-Command",
		"Get-MpComputerStatus | Select-Object -ExpandProperty AntivirusEnabled")
	output, err := cmd.Output()
	if err != nil {
		// Windows Defender not available (might not be Windows or not installed)
		check.IsInstalled = false
		return check, nil
	}

	check.IsInstalled = true
	check.IsEnabled = strings.TrimSpace(string(output)) == "True"

	// Check real-time protection
	cmd = exec.Command("powershell.exe", "-Command",
		"Get-MpComputerStatus | Select-Object -ExpandProperty RealTimeProtectionEnabled")
	output, _ = cmd.Output()
	check.RealTimeProtection = strings.TrimSpace(string(output)) == "True"

	// Check if exclusion exists for the executable
	if exePath == "" {
		exePath, _ = os.Executable()
	}

	if exePath != "" {
		dir := filepath.Dir(exePath)
		cmd = exec.Command("powershell.exe", "-Command",
			fmt.Sprintf("Get-MpPreference | Select-Object -ExpandProperty ExclusionPath | Where-Object { $_ -eq '%s' }", dir))
		output, _ = cmd.Output()
		check.IsExclusionSet = strings.TrimSpace(string(output)) == dir
		check.ExclusionPath = dir
	}

	// Get Defender version
	cmd = exec.Command("powershell.exe", "-Command",
		"Get-MpComputerStatus | Select-Object -ExpandProperty AntivirusSignatureVersion")
	output, _ = cmd.Output()
	check.Version = strings.TrimSpace(string(output))

	return check, nil
}

// AddWindowsDefenderExclusion adds the executable directory to Windows Defender exclusions
func AddWindowsDefenderExclusion(path string) error {
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}
		path = filepath.Dir(path)
	}

	// Add exclusion using PowerShell (requires admin privileges)
	cmd := exec.Command("powershell.exe", "-Command",
		fmt.Sprintf("Add-MpPreference -ExclusionPath '%s'", path))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add exclusion: %w (output: %s)", err, string(output))
	}

	return nil
}

// RemoveWindowsDefenderExclusion removes a path from Windows Defender exclusions
func RemoveWindowsDefenderExclusion(path string) error {
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}
		path = filepath.Dir(path)
	}

	cmd := exec.Command("powershell.exe", "-Command",
		fmt.Sprintf("Remove-MpPreference -ExclusionPath '%s'", path))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove exclusion: %w (output: %s)", err, string(output))
	}

	return nil
}

// WindowsPathUtils provides Windows-specific path utilities
type WindowsPathUtils struct{}

// ExpandEnvironmentStrings expands Windows environment variables in a path
func (u WindowsPathUtils) ExpandEnvironmentStrings(path string) string {
	// Expand common Windows environment variables
	replacements := map[string]string{
		"%USERPROFILE%":  os.Getenv("USERPROFILE"),
		"%APPDATA%":      os.Getenv("APPDATA"),
		"%LOCALAPPDATA%": os.Getenv("LOCALAPPDATA"),
		"%PROGRAMDATA%":  os.Getenv("PROGRAMDATA"),
		"%PROGRAMFILES%": os.Getenv("PROGRAMFILES"),
		"%TEMP%":         os.Getenv("TEMP"),
		"%TMP%":          os.Getenv("TMP"),
	}

	result := path
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, value)
	}

	return result
}

// ToLongPath converts a path to Windows long-path format if needed
func (u WindowsPathUtils) ToLongPath(path string) string {
	// If path exceeds MAX_PATH (260 characters), convert to extended-length path
	if len(path) > 260 {
		// Add \\?\ prefix for extended-length paths
		if !strings.HasPrefix(path, `\\?\`) && !strings.HasPrefix(path, `\\?\UNC\`) {
			return `\\?\` + path
		}
	}
	return path
}

// SanitizeFilename sanitizes a filename for Windows filesystem compatibility
func (u WindowsPathUtils) SanitizeFilename(filename string) string {
	// Remove Windows reserved characters: < > : " / \ | ? *
	reserved := []string{"<", ">", ":", `"`, "/", "\\", "|", "?", "*"}
	result := filename
	for _, char := range reserved {
		result = strings.ReplaceAll(result, char, "_")
	}

	// Remove reserved names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
	reservedNames := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}

	base := strings.ToUpper(strings.TrimSuffix(result, filepath.Ext(result)))
	for _, reserved := range reservedNames {
		if base == reserved {
			result = "_" + result
			break
		}
	}

	// Trim trailing spaces and dots (not allowed in Windows filenames)
	result = strings.TrimRight(result, " .")

	return result
}

// IsElevated returns true if the current process has administrator privileges
func IsElevated() bool {
	// Try to open a handle to the elevated token
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()

	// Check if the token has elevated privileges
	var elevation TokenElevation
	elevationSize := uint32(unsafe.Sizeof(elevation))
	err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), elevationSize, &elevationSize)
	if err != nil {
		return false
	}

	return elevation.TokenIsElevated != 0
}

// TokenElevation is the elevation structure for Windows tokens
type TokenElevation struct {
	TokenIsElevated uint32
}

// GetWindowsVersion returns the Windows version information
// Uses Windows registry to get version information since RtlGetVersion
// is available but has a different signature.
func GetWindowsVersion() (major, minor, build uint32, err error) {
	// Query Windows registry for version info
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return 0, 0, 0, err
	}
	defer key.Close()

	majorVal, _, err := key.GetIntegerValue("CurrentMajorVersionNumber")
	if err != nil {
		// Fallback to legacy method for older Windows versions
		return getWindowsVersionLegacy()
	}

	minorVal, _, err := key.GetIntegerValue("CurrentMinorVersionNumber")
	if err != nil {
		minorVal = 0
	}

	buildStr, _, err := key.GetStringValue("CurrentBuild")
	if err != nil {
		buildStr = "0"
	}

	buildVal, _ := strconv.ParseUint(buildStr, 10, 32)

	return uint32(majorVal), uint32(minorVal), uint32(buildVal), nil
}

// getWindowsVersionLegacy uses the legacy version API for older Windows
func getWindowsVersionLegacy() (major, minor, build uint32, err error) {
	// Fallback: use GetVersionExW for older Windows versions
	// This function is deprecated but works on older systems
	// We'll try the registry method which is more reliable

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return 0, 0, 0, err
	}
	defer key.Close()

	// Try to get version from registry
	majorVal, _, _ := key.GetIntegerValue("CurrentVersionMajor")
	minorVal, _, _ := key.GetIntegerValue("CurrentVersionMinor")

	if majorVal == 0 {
		// Try legacy CurrentVersion string
		curVersion, _, _ := key.GetStringValue("CurrentVersion")
		if curVersion != "" {
			parts := strings.Split(curVersion, ".")
			if len(parts) >= 2 {
				majorInt, _ := strconv.ParseInt(parts[0], 10, 64)
				minorInt, _ := strconv.ParseInt(parts[1], 10, 64)
				majorVal = uint64(majorInt)
				minorVal = uint64(minorInt)
			}
		}
	}

	// Get build number
	buildStr, _, _ := key.GetStringValue("CurrentBuild")
	buildVal, _ := strconv.ParseUint(buildStr, 10, 32)

	return uint32(majorVal), uint32(minorVal), uint32(buildVal), nil
}

// IsWindows10OrLater returns true if running on Windows 10 or later
func IsWindows10OrLater() bool {
	major, _, _, err := GetWindowsVersion()
	if err != nil {
		return false
	}
	return major >= 10
}

// IsWindowsServer returns true if running on Windows Server
// Checks the registry for ProductType to determine if running on Windows Server.
// VER_NT_WORKSTATION = 0x0000001 indicates client OS.
func IsWindowsServer() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	// ProductType: 1 = Workstation, 2 = Domain Controller, 3 = Server
	productType, _, err := key.GetIntegerValue("ProductType")
	if err != nil {
		// Fallback: check InstallationType
		installType, _, _ := key.GetStringValue("InstallationType")
		return installType == "Server" || installType == "Server Core"
	}

	return productType != 1 // 1 = Workstation, 2 or 3 = Server
}
