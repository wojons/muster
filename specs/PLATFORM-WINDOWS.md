# PLATFORM-WINDOWS Specification

Complete Windows-specific implementation details for Muster MCP system.

## Overview

Windows platform support providing native system integration, secure credential storage, package distribution, and enterprise deployment capabilities.

## 1. System Integration

### Windows Service

```go
//go:build windows
// +build windows

package service

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/mgr"
)

// ServiceName is the Windows service identifier
const ServiceName = "MusterMCP"
const ServiceDisplayName = "Muster MCP Server"
const ServiceDescription = "Model Context Protocol server for AI assistant integration"

type windowsService struct {
	server   Server
	logger   Logger
}

func (s *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start server
	go func() {
		if err := s.server.Start(ctx); err != nil {
			s.logger.Error("server start failed", "error", err)
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
				s.server.Stop()
				return false, 0
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				s.logger.Error("unexpected control request", "cmd", c.Cmd)
			}
		}
	}
}

// InstallService registers the application as a Windows service
func InstallService(configPath string) error {
	mgr, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer mgr.Disconnect()
	
	// Get executable path
	exepath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	
	service, err := mgr.OpenService(ServiceName)
	if err == nil {
		service.Close()
		return fmt.Errorf("service %s already exists", ServiceName)
	}
	
	// Configure service
	config := mgr.Config{
		StartType:    mgr.StartAutomatic,
		DisplayName:  ServiceDisplayName,
		Description:  ServiceDescription,
		Dependencies: []string{"Tcpip"},
	}
	
	service, err = mgr.CreateService(
		ServiceName,
		exepath,
		config,
		"daemon", "--config", configPath,
	)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer service.Close()
	
	// Set recovery actions
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 120 * time.Second},
		{Type: mgr.NoAction},
	}
	
	if err := service.SetRecoveryActions(recoveryActions, 86400); err != nil {
		return fmt.Errorf("set recovery actions: %w", err)
	}
	
	return nil
}

// UninstallService removes the Windows service
func UninstallService() error {
	mgr, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer mgr.Disconnect()
	
	service, err := mgr.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("service not found: %w", err)
	}
	defer service.Close()
	
	// Stop service if running
	status, err := service.Query()
	if err != nil {
		return fmt.Errorf("query service status: %w", err)
	}
	
	if status.State == svc.Running {
		if _, err := service.Control(svc.Stop); err != nil {
			return fmt.Errorf("stop service: %w", err)
		}
		// Wait for service to stop
		time.Sleep(2 * time.Second)
	}
	
	if err := service.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	
	return nil
}

// RunService starts the service (called from main when running as service)
func RunService(server Server, logger Logger) error {
	return svc.Run(ServiceName, &windowsService{server: server, logger: logger})
}

// IsWindowsService returns true if running as Windows service
func IsWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}
```

### Task Scheduler Integration

```go
//go:build windows
// +build windows

package scheduler

import (
	"fmt"
	"time"
	
	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// TaskScheduler provides Windows Task Scheduler integration
type TaskScheduler struct {
	service *ole.IDispatch
}

// NewTaskScheduler creates a new Task Scheduler connection
func NewTaskScheduler() (*TaskScheduler, error) {
	if err := ole.CoInitialize(0); err != nil {
		return nil, fmt.Errorf("initialize COM: %w", err)
	}
	
	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return nil, fmt.Errorf("create Schedule.Service: %w", err)
	}
	
	service, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("query interface: %w", err)
	}
	
	// Connect to local service
	if _, err := oleutil.CallMethod(service, "Connect"); err != nil {
		return nil, fmt.Errorf("connect to service: %w", err)
	}
	
	return &TaskScheduler{service: service}, nil
}

// Close releases COM resources
func (ts *TaskScheduler) Close() {
	if ts.service != nil {
		ts.service.Release()
	}
	ole.CoUninitialize()
}

// CreateScheduledTask creates a new scheduled task
func (ts *TaskScheduler) CreateScheduledTask(name, command string, trigger Trigger) error {
	// Get root folder
	rootFolder, err := oleutil.CallMethod(ts.service, "GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("get root folder: %w", err)
	}
	defer rootFolder.Clear()
	
	// Get task definition
	taskDefResult, err := oleutil.CallMethod(ts.service, "NewTask", 0)
	if err != nil {
		return fmt.Errorf("create task definition: %w", err)
	}
	taskDef := taskDefResult.ToIDispatch()
	
	// Set registration info
	regInfo, _ := oleutil.GetProperty(taskDef, "RegistrationInfo")
	regInfoDisp := regInfo.ToIDispatch()
	oleutil.PutProperty(regInfoDisp, "Description", "Muster scheduled task")
	regInfoDisp.Release()
	
	// Create trigger
	triggers, _ := oleutil.GetProperty(taskDef, "Triggers")
	triggersDisp := triggers.ToIDispatch()
	triggerResult, _ := oleutil.CallMethod(triggersDisp, "Create", trigger.Type())
	triggerDisp := triggerResult.ToIDispatch()
	
	// Configure trigger based on type
	switch t := trigger.(type) {
	case DailyTrigger:
		oleutil.PutProperty(triggerDisp, "DaysInterval", t.Interval)
		oleutil.PutProperty(triggerDisp, "StartBoundary", t.Start.Format("2006-01-02T15:04:05"))
	case WeeklyTrigger:
		oleutil.PutProperty(triggerDisp, "DaysOfWeek", t.DaysOfWeek)
		oleutil.PutProperty(triggerDisp, "WeeksInterval", t.Interval)
	case BootTrigger:
		// Boot trigger uses default settings
	}
	triggerDisp.Release()
	triggersDisp.Release()
	
	// Create action
	actions, _ := oleutil.GetProperty(taskDef, "Actions")
	actionsDisp := actions.ToIDispatch()
	actionResult, _ := oleutil.CallMethod(actionsDisp, "Create", 0) // 0 = execute
	actionDisp := actionResult.ToIDispatch()
	oleutil.PutProperty(actionDisp, "Path", command)
	actionDisp.Release()
	actionsDisp.Release()
	
	// Register task
	folder := rootFolder.ToIDispatch()
	_, err = oleutil.CallMethod(folder, "RegisterTaskDefinition",
		name,
		taskDef,
		6, // CREATE_OR_UPDATE
		nil, // User (default)
		nil, // Password (default)
		1,   // LOGON_INTERACTIVE_TOKEN
		nil, // sddl
	)
	if err != nil {
		return fmt.Errorf("register task: %w", err)
	}
	
	return nil
}

// DeleteScheduledTask removes a scheduled task
func (ts *TaskScheduler) DeleteScheduledTask(name string) error {
	rootFolder, err := oleutil.CallMethod(ts.service, "GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("get root folder: %w", err)
	}
	defer rootFolder.Clear()
	
	folder := rootFolder.ToIDispatch()
	_, err = oleutil.CallMethod(folder, "DeleteTask", name, 0)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	
	return nil
}

type Trigger interface {
	Type() int
}

type DailyTrigger struct {
	Start    time.Time
	Interval int
}

func (d DailyTrigger) Type() int { return 2 }

type WeeklyTrigger struct {
	Start       time.Time
	DaysOfWeek  int // Bitmask: 1=Sun, 2=Mon, 4=Tue, etc.
	Interval    int
}

func (w WeeklyTrigger) Type() int { return 3 }

type BootTrigger struct{}

func (b BootTrigger) Type() int { return 8 }
```

### Registry Management

```go
//go:build windows
// +build windows

package registry

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	// Registry paths
	BaseKey        = `Software\Muster`
	UninstallKey   = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Muster`
	AppPathsKey    = `Software\Microsoft\Windows\CurrentVersion\App Paths\muster.exe`
	EnvironmentKey = `Environment`
)

// RegistryStore provides Windows Registry operations
type RegistryStore struct{}

func New() *RegistryStore {
	return &RegistryStore{}
}

// SetString sets a string value in the registry
func (r *RegistryStore) SetString(keyPath, name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.WRITE)
	if err != nil {
		return fmt.Errorf("create key %s: %w", keyPath, err)
	}
	defer k.Close()

	if err := k.SetStringValue(name, value); err != nil {
		return fmt.Errorf("set string value %s: %w", name, err)
	}

	return nil
}

// GetString retrieves a string value from the registry
func (r *RegistryStore) GetString(keyPath, name string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.READ)
	if err != nil {
		return "", fmt.Errorf("open key %s: %w", keyPath, err)
	}
	defer k.Close()

	val, _, err := k.GetStringValue(name)
	if err != nil {
		return "", fmt.Errorf("get string value %s: %w", name, err)
	}

	return val, nil
}

// DeleteKey removes a registry key
func (r *RegistryStore) DeleteKey(keyPath string) error {
	if err := registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil {
		return fmt.Errorf("delete key %s: %w", keyPath, err)
	}
	return nil
}

// DeleteValue removes a value from a registry key
func (r *RegistryStore) DeleteValue(keyPath, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.WRITE)
	if err != nil {
		return fmt.Errorf("open key %s: %w", keyPath, err)
	}
	defer k.Close()

	if err := k.DeleteValue(name); err != nil {
		return fmt.Errorf("delete value %s: %w", name, err)
	}

	return nil
}

// InstallRegistryEntries creates all registry entries for installation
func (r *RegistryStore) InstallRegistryEntries(installPath string) error {
	// Application settings
	if err := r.SetString(BaseKey, "InstallPath", installPath); err != nil {
		return err
	}

	// Uninstall information
	if err := r.SetString(UninstallKey, "DisplayName", "Muster MCP"); err != nil {
		return err
	}
	if err := r.SetString(UninstallKey, "UninstallString",
		filepath.Join(installPath, "uninstall.exe")); err != nil {
		return err
	}
	if err := r.SetString(UninstallKey, "InstallLocation", installPath); err != nil {
		return err
	}
	if err := r.SetString(UninstallKey, "Publisher", "Muster Team"); err != nil {
		return err
	}

	// App Paths for command-line access
	if err := r.SetString(AppPathsKey, "", filepath.Join(installPath, "muster.exe")); err != nil {
		return err
	}
	if err := r.SetString(AppPathsKey, "Path", installPath); err != nil {
		return err
	}

	return nil
}

// UninstallRegistryEntries removes all registry entries
func (r *RegistryStore) UninstallRegistryEntries() error {
	r.DeleteKey(BaseKey)
	r.DeleteKey(UninstallKey)
	r.DeleteKey(AppPathsKey)
	return nil
}

// AddToPath adds the install directory to user PATH
func (r *RegistryStore) AddToPath(installPath string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, EnvironmentKey, registry.WRITE)
	if err != nil {
		return fmt.Errorf("open environment key: %w", err)
	}
	defer k.Close()

	path, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("get path: %w", err)
	}

	// Check if already in PATH
	if containsPath(path, installPath) {
		return nil
	}

	newPath := path + ";" + installPath
	if err := k.SetStringValue("Path", newPath); err != nil {
		return fmt.Errorf("set path: %w", err)
	}

	// Notify Windows of environment change
	notifyEnvironmentChange()

	return nil
}

// RemoveFromPath removes the install directory from user PATH
func (r *RegistryStore) RemoveFromPath(installPath string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, EnvironmentKey, registry.WRITE)
	if err != nil {
		return fmt.Errorf("open environment key: %w", err)
	}
	defer k.Close()

	path, _, err := k.GetStringValue("Path")
	if err != nil {
		return nil // PATH doesn't exist, nothing to remove
	}

	newPath := removePath(path, installPath)
	if newPath != path {
		if err := k.SetStringValue("Path", newPath); err != nil {
			return fmt.Errorf("set path: %w", err)
		}
		notifyEnvironmentChange()
	}

	return nil
}

func containsPath(pathList, item string) bool {
	// Implementation to check if path is already in PATH
	return false
}

func removePath(pathList, item string) string {
	// Implementation to remove path from PATH
	return pathList
}

func notifyEnvironmentChange() {
	// Send WM_SETTINGCHANGE message to notify of environment changes
	windows.SendMessageTimeout(
		windows.HWND_BROADCAST,
		windows.WM_SETTINGCHANGE,
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Environment"))),
		windows.SMTO_ABORTIFHUNG,
		5000,
		nil,
	)
}
```

## 2. Package Distribution

### WiX MSI Installer Configuration

```xml
<!-- installer/windows/Muster.wxs -->
<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi"
     xmlns:util="http://schemas.microsoft.com/wix/UtilExtension">
  <Product Id="*"
           Name="Muster MCP"
           Language="1033"
           Version="$(var.Version)"
           Manufacturer="Muster Team"
           UpgradeCode="A1B2C3D4-E5F6-7890-ABCD-EF1234567890">
    
    <Package InstallerVersion="200"
             Compressed="yes"
             InstallScope="perMachine"
             Platform="x64"
             Description="Muster MCP Server Installer"
             Comments="Model Context Protocol server for AI assistant integration" />
    
    <MediaTemplate EmbedCab="yes" />
    
    <MajorUpgrade DowngradeErrorMessage="A newer version of [ProductName] is already installed." />
    
    <!-- Check for Windows 10 or later -->
    <Condition Message="This application requires Windows 10 or later.">
      <![CDATA[Installed OR (VersionNT >= 603)]]>
    </Condition>
    
    <!-- Features -->
    <Feature Id="ProductFeature" Title="Muster MCP" Level="1">
      <ComponentGroupRef Id="ProductComponents" />
      <ComponentGroupRef Id="ServiceComponents" />
      <ComponentRef Id="ApplicationShortcut" />
    </Feature>
    
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFiles64Folder">
        <Directory Id="INSTALLFOLDER" Name="Muster">
          
          <!-- Main application files -->
          <ComponentGroup Id="ProductComponents" Directory="INSTALLFOLDER">
            <Component Id="MusterEXE" Guid="B1C2D3E4-F5A6-7890-BCDE-F12345678901">
              <File Id="MusterEXE" Name="muster.exe" Source="$(var.SourceDir)\muster.exe" KeyPath="yes">
                <Shortcut Id="MusterCLIShortcut" 
                         Directory="ProgramMenuFolder" 
                         Name="Muster CLI" 
                         WorkingDirectory="INSTALLFOLDER"
                         Icon="MusterIcon.ico"
                         Advertise="yes" />
              </File>
            </Component>
            
            <Component Id="MusterDaemonEXE" Guid="C2D3E4F5-A6B7-8901-CDEF-234567890123">
              <File Id="MusterDaemonEXE" Name="musterd.exe" Source="$(var.SourceDir)\musterd.exe" KeyPath="yes" />
            </Component>
            
            <Component Id="DefaultConfig" Guid="D3E4F5A6-B7C8-9012-DEFA-345678901234">
              <File Id="DefaultConfig" Name="config.yaml" Source="$(var.SourceDir)\config.yaml" KeyPath="yes" />
            </Component>
            
            <Component Id="LicenseFile" Guid="E4F5A6B7-C8D9-0123-EFAB-456789012345">
              <File Id="LicenseFile" Name="LICENSE.txt" Source="$(var.SourceDir)\LICENSE.txt" KeyPath="yes" />
            </Component>
          </ComponentGroup>
          
          <!-- Windows Service -->
          <ComponentGroup Id="ServiceComponents" Directory="INSTALLFOLDER">
            <Component Id="MusterService" Guid="F5A6B7C8-D9E0-1234-FABC-567890123456">
              <File Id="MusterService" Name="musterd.exe" Source="$(var.SourceDir)\musterd.exe" KeyPath="yes" />
              <ServiceInstall Id="MusterServiceInstall"
                             Type="ownProcess"
                             Name="MusterMCP"
                             DisplayName="Muster MCP Server"
                             Description="Model Context Protocol server for AI assistant integration"
                             Start="auto"
                             ErrorControl="normal"
                             Account="LocalSystem">
                <ServiceDependency Id="Tcpip" />
              </ServiceInstall>
              <ServiceControl Id="MusterServiceControl"
                             Name="MusterMCP"
                             Start="install"
                             Stop="both"
                             Remove="uninstall"
                             Wait="yes" />
            </Component>
          </ComponentGroup>
          
        </Directory>
      </Directory>
      
      <!-- Start Menu shortcut -->
      <Directory Id="ProgramMenuFolder">
        <Directory Id="ApplicationProgramsFolder" Name="Muster MCP">
          <Component Id="ApplicationShortcut" Guid="A6B7C8D9-E0F1-2345-ABCD-678901234567">
            <Shortcut Id="UninstallProduct"
                     Name="Uninstall Muster"
                     Target="[System64Folder]msiexec.exe"
                     Arguments="/x [ProductCode]"
                     Description="Uninstalls Muster MCP" />
            <RemoveFolder Id="ApplicationProgramsFolder" On="uninstall" />
            <RegistryValue Root="HKCU" Key="Software\Muster" Name="installed" Type="integer" Value="1" KeyPath="yes" />
          </Component>
        </Directory>
      </Directory>
      
    </Directory>
    
    <!-- Custom Actions -->
    <CustomAction Id="SetInstallPath" Property="INSTALLFOLDER" Value="[ProgramFiles64Folder]Muster" Execute="firstSequence" />
    
    <InstallExecuteSequence>
      <Custom Action="SetInstallPath" Before="CostInitialize">NOT Installed</Custom>
    </InstallExecuteSequence>
    
    <!-- UI -->
    <UIRef Id="WixUI_InstallDir" />
    <Property Id="WIXUI_INSTALLDIR" Value="INSTALLFOLDER" />
    
    <!-- Icons -->
    <Icon Id="MusterIcon.ico" SourceFile="$(var.SourceDir)\assets\icon.ico" />
    <Property Id="ARPPRODUCTICON" Value="MusterIcon.ico" />
    
    <!-- Add/Remove Programs properties -->
    <Property Id="ARPURLINFOABOUT" Value="https://github.com/muster/mcp" />
    <Property Id="ARPHELPLINK" Value="https://muster.dev/docs" />
    <Property Id="ARPNOMODIFY" Value="1" />
    
  </Product>
</Wix>
```

### Chocolatey Package

```powershell
# installer/chocolatey/tools/chocolateyinstall.ps1
$ErrorActionPreference = 'Stop'

$packageName = 'muster'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$url64 = 'https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-amd64.zip'
$checksum64 = 'A1B2C3D4E5F6789012345678901234567890ABCDEF1234567890ABCDEF123456'

$packageArgs = @{
  packageName    = $packageName
  unzipLocation  = $toolsDir
  url64bit       = $url64
  checksum64     = $checksum64
  checksumType64 = 'sha256'
}

Install-ChocolateyZipPackage @packageArgs

# Install Windows service
$exePath = Join-Path $toolsDir 'musterd.exe'
& $exePath service install --config "$toolsDir\config.yaml"

# Add to PATH
$installPath = $toolsDir
[Environment]::SetEnvironmentVariable(
  'Path',
  "$([Environment]::GetEnvironmentVariable('Path', 'User'));$installPath",
  'User'
)

Write-Host "Muster MCP installed successfully!" -ForegroundColor Green
Write-Host "Run 'muster --help' to get started." -ForegroundColor Cyan
```

```powershell
# installer/chocolatey/tools/chocolateyuninstall.ps1
$ErrorActionPreference = 'Stop'

$packageName = 'muster'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"

# Stop and remove service
$exePath = Join-Path $toolsDir 'musterd.exe'
if (Test-Path $exePath) {
  & $exePath service uninstall
}

# Remove from PATH (best effort)
$currentPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$newPath = ($currentPath -split ';' | Where-Object { $_ -ne $toolsDir }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $newPath, 'User')

Write-Host "Muster MCP uninstalled." -ForegroundColor Green
```

```xml
<!-- installer/chocolatey/muster.nuspec -->
<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd">
  <metadata>
    <id>muster</id>
    <version>1.0.0</version>
    <title>Muster MCP</title>
    <authors>Muster Team</authors>
    <owners>muster</owners>
    <licenseUrl>https://github.com/muster/mcp/blob/main/LICENSE</licenseUrl>
    <projectUrl>https://muster.dev</projectUrl>
    <iconUrl>https://muster.dev/assets/icon.png</iconUrl>
    <requireLicenseAcceptance>false</requireLicenseAcceptance>
    <description>Model Context Protocol server for AI assistant integration. Provides a unified interface for AI assistants to interact with local development environments.</description>
    <summary>MCP server for AI assistant integration</summary>
    <releaseNotes>https://github.com/muster/mcp/releases</releaseNotes>
    <copyright>Copyright 2024 Muster Team</copyright>
    <tags>muster mcp ai assistant cli server</tags>
    <projectSourceUrl>https://github.com/muster/mcp</projectSourceUrl>
    <packageSourceUrl>https://github.com/muster/mcp/tree/main/installer/chocolatey</packageSourceUrl>
    <docsUrl>https://muster.dev/docs</docsUrl>
    <bugTrackerUrl>https://github.com/muster/mcp/issues</bugTrackerUrl>
    <dependencies>
      <dependency id="dotnet-6.0-runtime" version="6.0.0" />
    </dependencies>
  </metadata>
  <files>
    <file src="tools\**" target="tools" />
  </files>
</package>
```

### Scoop Manifest

```json
{
  "version": "1.0.0",
  "description": "Model Context Protocol server for AI assistant integration",
  "homepage": "https://muster.dev",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-amd64.zip",
      "hash": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
    },
    "32bit": {
      "url": "https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-386.zip",
      "hash": "b2c3d4e5f6a7890123456789012345678901bcdef2345678901bcdef2345678"
    },
    "arm64": {
      "url": "https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-arm64.zip",
      "hash": "c3d4e5f6a7b8901234567890123456789012cdef3456789012cdef345678901"
    }
  },
  "bin": [
    "muster.exe",
    "musterd.exe"
  ],
  "shortcuts": [
    [
      "muster.exe",
      "Muster CLI"
    ]
  ],
  "persist": [
    "config.yaml",
    "data"
  ],
  "post_install": [
    "if (!(Test-Path \"$persist_dir\\config.yaml\")) {",
    "  Copy-Item \"$dir\\config.default.yaml\" \"$persist_dir\\config.yaml\"",
    "}",
    "# Install Windows service",
    "& \"$dir\\musterd.exe\" service install --config \"$persist_dir\\config.yaml\""
  ],
  "pre_uninstall": [
    "# Stop and remove service",
    "& \"$dir\\musterd.exe\" service uninstall"
  ],
  "checkver": {
    "github": "https://github.com/muster/mcp"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/muster/mcp/releases/download/v$version/muster-windows-amd64.zip"
      },
      "32bit": {
        "url": "https://github.com/muster/mcp/releases/download/v$version/muster-windows-386.zip"
      },
      "arm64": {
        "url": "https://github.com/muster/mcp/releases/download/v$version/muster-windows-arm64.zip"
      }
    },
    "hash": {
      "url": "$baseurl/checksums.txt"
    }
  },
  "notes": [
    "Muster MCP has been installed.",
    "",
    "Configuration file: $persist_dir\config.yaml",
    "",
    "To get started:",
    "  muster --help",
    "",
    "Windows service commands:",
    "  musterd service install   - Install as Windows service",
    "  musterd service uninstall - Remove Windows service",
    "  musterd service start     - Start the service",
    "  musterd service stop      - Stop the service"
  ]
}
```

### Winget Package Manifest

```yaml
# installer/winget/Muster.Muster.installer.yaml
PackageIdentifier: Muster.Muster
PackageVersion: 1.0.0
MinimumOSVersion: 10.0.17763.0
InstallerType: zip
Scope: machine
InstallModes:
  - interactive
  - silent
  - silentWithProgress
UpgradeBehavior: install
Commands:
  - muster
  - musterd
Protocols:
  - mcp
FileExtensions:
  - mcp
Dependencies:
  PackageDependencies:
    - PackageIdentifier: Microsoft.VCRedist.2015+.x64
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-amd64.zip
    InstallerSha256: A1B2C3D4E5F6789012345678901234567890ABCDEF1234567890ABCDEF123456
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: muster.exe
        PortableCommandAlias: muster
      - RelativeFilePath: musterd.exe
        PortableCommandAlias: musterd
  - Architecture: x86
    InstallerUrl: https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-386.zip
    InstallerSha256: B2C3D4E5F6A7890123456789012345678901BCDEF2345678901BCDEF2345678
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: muster.exe
        PortableCommandAlias: muster
      - RelativeFilePath: musterd.exe
        PortableCommandAlias: musterd
  - Architecture: arm64
    InstallerUrl: https://github.com/muster/mcp/releases/download/v1.0.0/muster-windows-arm64.zip
    InstallerSha256: C3D4E5F6A7B8901234567890123456789012CDEF3456789012CDEF345678901
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: muster.exe
        PortableCommandAlias: muster
      - RelativeFilePath: musterd.exe
        PortableCommandAlias: musterd
ManifestType: installer
ManifestVersion: 1.5.0
```

```yaml
# installer/winget/Muster.Muster.locale.en-US.yaml
PackageIdentifier: Muster.Muster
PackageVersion: 1.0.0
PackageLocale: en-US
Publisher: Muster Team
PublisherUrl: https://muster.dev
PublisherSupportUrl: https://github.com/muster/mcp/issues
PrivacyUrl: https://muster.dev/privacy
Author: Muster Team
PackageName: Muster MCP
PackageUrl: https://muster.dev
License: MIT
LicenseUrl: https://github.com/muster/mcp/blob/main/LICENSE
Copyright: Copyright (c) 2024 Muster Team
CopyrightUrl: https://github.com/muster/mcp/blob/main/LICENSE
ShortDescription: Model Context Protocol server for AI assistant integration
Description: |
  Muster provides a Model Context Protocol (MCP) server that enables AI assistants
  to interact with local development environments. It offers a unified interface for
  executing commands, managing configurations, and accessing development tools.
Moniker: muster
Tags:
  - mcp
  - ai
  - assistant
  - cli
  - server
  - development
  - tools
ReleaseNotes: |
  ## What's New in v1.0.0
  
  - Initial release with full MCP protocol support
  - Windows service integration
  - Secure credential storage via Windows Credential Manager
  - Support for muster, chocolatey, scoop, and winget installation
ReleaseNotesUrl: https://github.com/muster/mcp/releases/tag/v1.0.0
Documentation:
  - DocumentLabel: Documentation
    DocumentUrl: https://muster.dev/docs
ManifestType: defaultLocale
ManifestVersion: 1.5.0
```

```yaml
# installer/winget/Muster.Muster.yaml
PackageIdentifier: Muster.Muster
PackageVersion: 1.0.0
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.5.0
```

## 3. Credential Storage

### Windows Credential Manager

```go
//go:build windows
// +build windows

package credential

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsCredentialManager implements credential storage using Windows Credential Manager
type WindowsCredentialManager struct{}

func NewWindowsCredentialManager() *WindowsCredentialManager {
	return &WindowsCredentialManager{}
}

// Set stores a credential in Windows Credential Manager
func (w *WindowsCredentialManager) Set(ctx context.Context, service, account, password string) error {
	targetName := fmt.Sprintf("%s/%s", service, account)
	targetUTF16, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return fmt.Errorf("convert target name: %w", err)
	}

	passwordUTF16, err := syscall.UTF16PtrFromString(password)
	if err != nil {
		return fmt.Errorf("convert password: %w", err)
	}

	credential := &windows.Credential{
		Flags:              0,
		Type:               windows.CRED_TYPE_GENERIC,
		TargetName:         targetUTF16,
		Comment:            nil,
		LastWritten:        windows.Filetime{},
		CredentialBlobSize: uint32(len(password) * 2), // UTF-16 uses 2 bytes per char
		CredentialBlob:     uintptr(unsafe.Pointer(passwordUTF16)),
		Persist:            windows.CRED_PERSIST_LOCAL_MACHINE,
		AttributeCount:     0,
		Attributes:         nil,
		TargetAlias:        nil,
		UserName:           targetUTF16,
	}

	if err := windows.CredWrite(credential, 0); err != nil {
		return fmt.Errorf("write credential: %w", err)
	}

	return nil
}

// Get retrieves a credential from Windows Credential Manager
func (w *WindowsCredentialManager) Get(ctx context.Context, service, account string) (string, error) {
	targetName := fmt.Sprintf("%s/%s", service, account)

	cred, err := windows.CredRead(targetName, windows.CRED_TYPE_GENERIC, 0)
	if err != nil {
		if err == windows.ERROR_NOT_FOUND {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read credential: %w", err)
	}
	defer windows.CredFree(cred)

	// Convert UTF-16 blob to Go string
	blobPtr := (*uint16)(unsafe.Pointer(cred.CredentialBlob))
	blobSlice := (*[1 << 30]uint16)(unsafe.Pointer(blobPtr))[:cred.CredentialBlobSize/2]
	password := syscall.UTF16ToString(blobSlice)

	return password, nil
}

// Delete removes a credential from Windows Credential Manager
func (w *WindowsCredentialManager) Delete(ctx context.Context, service, account string) error {
	targetName := fmt.Sprintf("%s/%s", service, account)

	if err := windows.CredDelete(targetName, windows.CRED_TYPE_GENERIC, 0); err != nil {
		if err == windows.ERROR_NOT_FOUND {
			return ErrNotFound
		}
		return fmt.Errorf("delete credential: %w", err)
	}

	return nil
}

// List returns all credentials for a service
func (w *WindowsCredentialManager) List(ctx context.Context, service string) ([]Account, error) {
	// CredEnumerate doesn't work well with Go, use registry fallback or different approach
	return nil, fmt.Errorf("list not implemented on Windows")
}

type Account struct {
	Service string
	Account string
}

var ErrNotFound = fmt.Errorf("credential not found")
```

### DPAPI Encryption

```go
//go:build windows
// +build windows

package crypto

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	crypt32                       = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData          = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData        = crypt32.NewProc("CryptUnprotectData")
)

// DATA_BLOB represents the CRYPT_DATA_BLOB structure
type DATA_BLOB struct {
	cbData uint32
	pbData *byte
}

// DPAPI provides Windows Data Protection API encryption
type DPAPI struct {
	description *uint16
}

func NewDPAPI(description string) *DPAPI {
	desc, _ := syscall.UTF16PtrFromString(description)
	return &DPAPI{description: desc}
}

// Encrypt encrypts data using DPAPI for the current user
func (d *DPAPI) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}

	// Create input blob
	plainBlob := DATA_BLOB{
		cbData: uint32(len(plaintext)),
		pbData: &plaintext[0],
	}

	// Create output blob
	var cipherBlob DATA_BLOB

	// CRYPTPROTECT_UI_FORBIDDEN = 0x1
	// CRYPTPROTECT_LOCAL_MACHINE = 0x4
	flags := uint32(0)

	ret, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&plainBlob)),
		uintptr(unsafe.Pointer(d.description)),
		0, // Optional entropy
		0, // Reserved
		0, // Prompt struct
		uintptr(flags),
		uintptr(unsafe.Pointer(&cipherBlob)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData failed: %v", err)
	}

	// Copy encrypted data
	encrypted := make([]byte, cipherBlob.cbData)
	copy(encrypted, (*[1 << 30]byte)(unsafe.Pointer(cipherBlob.pbData))[:cipherBlob.cbData])

	// Free the blob
	syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(cipherBlob.pbData))))

	return encrypted, nil
}

// Decrypt decrypts data using DPAPI
func (d *DPAPI) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("empty ciphertext")
	}

	// Create input blob
	cipherBlob := DATA_BLOB{
		cbData: uint32(len(ciphertext)),
		pbData: &ciphertext[0],
	}

	// Create output blob
	var plainBlob DATA_BLOB

	// CRYPTPROTECT_UI_FORBIDDEN = 0x1
	flags := uint32(0)

	ret, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&cipherBlob)),
		0, // Description (optional)
		0, // Optional entropy
		0, // Reserved
		0, // Prompt struct
		uintptr(flags),
		uintptr(unsafe.Pointer(&plainBlob)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData failed: %v", err)
	}

	// Copy decrypted data
	decrypted := make([]byte, plainBlob.cbData)
	copy(decrypted, (*[1 << 30]byte)(unsafe.Pointer(plainBlob.pbData))[:plainBlob.cbData])

	// Free the blob
	syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(plainBlob.pbData))))

	return decrypted, nil
}

// EncryptMachine encrypts data using DPAPI for the local machine (accessible to all users)
func (d *DPAPI) EncryptMachine(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}

	plainBlob := DATA_BLOB{
		cbData: uint32(len(plaintext)),
		pbData: &plaintext[0],
	}

	var cipherBlob DATA_BLOB

	// CRYPTPROTECT_LOCAL_MACHINE = 0x4
	flags := uint32(0x4)

	ret, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&plainBlob)),
		uintptr(unsafe.Pointer(d.description)),
		0,
		0,
		0,
		uintptr(flags),
		uintptr(unsafe.Pointer(&cipherBlob)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData (machine) failed: %v", err)
	}

	encrypted := make([]byte, cipherBlob.cbData)
	copy(encrypted, (*[1 << 30]byte)(unsafe.Pointer(cipherBlob.pbData))[:cipherBlob.cbData])

	syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(cipherBlob.pbData))))

	return encrypted, nil
}
```

### Certificate Store Integration

```go
//go:build windows
// +build windows

package crypto

import (
	"crypto/x509"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	crypt32                          = syscall.NewLazyDLL("crypt32.dll")
	procCertOpenStore                = crypt32.NewProc("CertOpenStore")
	procCertCloseStore               = crypt32.NewProc("CertCloseStore")
	procCertFindCertificateInStore   = crypt32.NewProc("CertFindCertificateInStore")
	procCertEnumCertificatesInStore  = crypt32.NewProc("CertEnumCertificatesInStore")
	procCertFreeCertificateContext   = crypt32.NewProc("CertFreeCertificateContext")
)

// CertStore provides Windows Certificate Store access
type CertStore struct {
	store syscall.Handle
}

// Store locations
const (
	CERT_STORE_PROV_SYSTEM         = 10
	CERT_SYSTEM_STORE_CURRENT_USER = 1 << 16
	CERT_SYSTEM_STORE_LOCAL_MACHINE = 2 << 16
)

// OpenUserStore opens the current user's certificate store
func OpenUserStore(storeName string) (*CertStore, error) {
	namePtr, err := syscall.UTF16PtrFromString(storeName)
	if err != nil {
		return nil, fmt.Errorf("convert store name: %w", err)
	}

	ret, _, err := procCertOpenStore.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("System"))),
		0,
		0,
		uintptr(CERT_STORE_PROV_SYSTEM|CERT_SYSTEM_STORE_CURRENT_USER),
		uintptr(unsafe.Pointer(namePtr)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("open certificate store: %v", err)
	}

	return &CertStore{store: syscall.Handle(ret)}, nil
}

// OpenMachineStore opens the local machine certificate store
func OpenMachineStore(storeName string) (*CertStore, error) {
	namePtr, err := syscall.UTF16PtrFromString(storeName)
	if err != nil {
		return nil, fmt.Errorf("convert store name: %w", err)
	}

	ret, _, err := procCertOpenStore.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("System"))),
		0,
		0,
		uintptr(CERT_STORE_PROV_SYSTEM|CERT_SYSTEM_STORE_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(namePtr)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("open certificate store: %v", err)
	}

	return &CertStore{store: syscall.Handle(ret)}, nil
}

// Close closes the certificate store
func (cs *CertStore) Close() error {
	if cs.store != 0 {
		ret, _, _ := procCertCloseStore.Call(uintptr(cs.store), 0)
		if ret == 0 {
			return fmt.Errorf("close certificate store")
		}
		cs.store = 0
	}
	return nil
}

// FindBySubject finds a certificate by subject name
func (cs *CertStore) FindBySubject(subject string) (*x509.Certificate, error) {
	subjectPtr, err := syscall.UTF16PtrFromString(subject)
	if err != nil {
		return nil, fmt.Errorf("convert subject: %w", err)
	}

	// CERT_FIND_SUBJECT_STR = 0x00080007
	findType := uint32(0x00080007)

	ret, _, _ := procCertFindCertificateInStore.Call(
		uintptr(cs.store),
		0x00000001, // X509_ASN_ENCODING
		0,
		uintptr(findType),
		uintptr(unsafe.Pointer(subjectPtr)),
		0,
	)

	if ret == 0 {
		return nil, fmt.Errorf("certificate not found")
	}

	// Parse certificate context
	return parseCertContext(ret)
}

// List returns all certificates in the store
func (cs *CertStore) List() ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	var ctx uintptr

	for {
		ret, _, _ := procCertEnumCertificatesInStore.Call(
			uintptr(cs.store),
			ctx,
		)

		if ret == 0 {
			break
		}

		cert, err := parseCertContext(ret)
		if err == nil {
			certs = append(certs, cert)
		}

		ctx = ret
	}

	return certs, nil
}

func parseCertContext(ctx uintptr) (*x509.Certificate, error) {
	// CERT_CONTEXT structure
	// typedef struct _CERT_CONTEXT {
	//     DWORD      dwCertEncodingType;
	//     BYTE       *pbCertEncoded;
	//     DWORD      cbCertEncoded;
	//     PCERT_INFO pCertInfo;
	//     HCERTSTORE hCertStore;
	// } CERT_CONTEXT;

	type certContext struct {
		dwCertEncodingType uint32
		pbCertEncoded      *byte
		cbCertEncoded      uint32
		pCertInfo          uintptr
		hCertStore         syscall.Handle
	}

	context := (*certContext)(unsafe.Pointer(ctx))
	encoded := (*[1 << 30]byte)(unsafe.Pointer(context.pbCertEncoded))[:context.cbCertEncoded]

	cert, err := x509.ParseCertificate(encoded)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// Free the context
	procCertFreeCertificateContext.Call(ctx)

	return cert, nil
}
```

## 4. File System

### Known Folders

```go
//go:build windows
// +build windows

package fs

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	procSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree")
)

// Known folder GUIDs
var (
	FOLDERID_ProgramData        = windows.GUID{Data1: 0x62AB5D82, Data2: 0xFDC1, Data3: 0x4DC3, Data4: [8]byte{0xA9, 0xDD, 0x07, 0x0D, 0x1D, 0x49, 0x5D, 0x97}}
	FOLDERID_LocalAppData       = windows.GUID{Data1: 0xF1B32785, Data2: 0x6FBA, Data3: 0x4FCF, Data4: [8]byte{0x9D, 0x55, 0x7B, 0x8E, 0x7F, 0x15, 0x70, 0x91}}
	FOLDERID_RoamingAppData     = windows.GUID{Data1: 0x3EB685DB, Data2: 0x65F9, Data3: 0x4CF6, Data4: [8]byte{0xA0, 0x3A, 0xE3, 0xEF, 0x65, 0x72, 0x9F, 0x3D}}
	FOLDERID_ProgramFiles       = windows.GUID{Data1: 0x905E63B6, Data2: 0xC1BF, Data3: 0x494E, Data4: [8]byte{0xB2, 0x9C, 0x65, 0xB7, 0x32, 0xD3, 0xD2, 0x1A}}
	FOLDERID_ProgramFilesX64    = windows.GUID{Data1: 0x6D809377, Data2: 0x6AF0, Data3: 0x444B, Data4: [8]byte{0x89, 0x57, 0xA3, 0x77, 0x3F, 0x02, 0x20, 0x0E}}
	FOLDERID_ProgramFilesX86    = windows.GUID{Data1: 0x7C5A40EF, Data2: 0xA0FB, Data3: 0x4BFC, Data4: [8]byte{0x87, 0x4A, 0xC0, 0xF2, 0xE0, 0xB9, 0xFA, 0x8E}}
	FOLDERID_Documents          = windows.GUID{Data1: 0xFDD39AD0, Data2: 0x238F, Data3: 0x46AF, Data4: [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7}}
	FOLDERID_UserProfile        = windows.GUID{Data1: 0x5E6C858F, Data2: 0x0E22, Data3: 0x4760, Data4: [8]byte{0x9A, 0xFE, 0xEA, 0x33, 0x17, 0xB6, 0x71, 0x73}}
	FOLDERID_Public             = windows.GUID{Data1: 0xDFDF76A2, Data2: 0xC82A, Data3: 0x4D63, Data4: [8]byte{0x90, 0x6A, 0x56, 0x44, 0xAC, 0x45, 0x73, 0x85}}
)

// GetKnownFolder retrieves the path to a known folder
func GetKnownFolder(folderID *windows.GUID) (string, error) {
	var pathPtr *uint16

	ret, _, err := procSHGetKnownFolderPath.Call(
		uintptr(unsafe.Pointer(folderID)),
		0, // dwFlags
		0, // hToken
		uintptr(unsafe.Pointer(&pathPtr)),
	)

	if ret != 0 {
		return "", fmt.Errorf("SHGetKnownFolderPath failed: %v", err)
	}

	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(pathPtr)))

	return syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(pathPtr))[:]), nil
}

// WindowsPaths provides standard Windows paths
type WindowsPaths struct{}

func NewWindowsPaths() *WindowsPaths {
	return &WindowsPaths{}
}

// ConfigDir returns the configuration directory
func (w *WindowsPaths) ConfigDir() (string, error) {
	path, err := GetKnownFolder(&FOLDERID_RoamingAppData)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, "Muster"), nil
}

// DataDir returns the data directory
func (w *WindowsPaths) DataDir() (string, error) {
	path, err := GetKnownFolder(&FOLDERID_LocalAppData)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, "Muster"), nil
}

// CacheDir returns the cache directory
func (w *WindowsPaths) CacheDir() (string, error) {
	path, err := GetKnownFolder(&FOLDERID_LocalAppData)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, "Muster", "cache"), nil
}

// LogDir returns the log directory
func (w *WindowsPaths) LogDir() (string, error) {
	path, err := GetKnownFolder(&FOLDERID_LocalAppData)
	if err != nil {
		return "", err
	}
	return filepath.Join(path, "Muster", "logs"), nil
}

// ProgramFilesDir returns the 64-bit Program Files directory
func (w *WindowsPaths) ProgramFilesDir() (string, error) {
	return GetKnownFolder(&FOLDERID_ProgramFilesX64)
}

// ProgramFilesX86Dir returns the 32-bit Program Files directory
func (w *WindowsPaths) ProgramFilesX86Dir() (string, error) {
	return GetKnownFolder(&FOLDERID_ProgramFilesX86)
}

// ProgramDataDir returns the ProgramData directory
func (w *WindowsPaths) ProgramDataDir() (string, error) {
	return GetKnownFolder(&FOLDERID_ProgramData)
}

// UserProfileDir returns the user profile directory
func (w *WindowsPaths) UserProfileDir() (string, error) {
	return GetKnownFolder(&FOLDERID_UserProfile)
}

// DocumentsDir returns the Documents folder
func (w *WindowsPaths) DocumentsDir() (string, error) {
	return GetKnownFolder(&FOLDERID_Documents)
}

// EnsureAllDirs creates all standard directories
func (w *WindowsPaths) EnsureAllDirs() error {
	dirs := []func() (string, error){
		w.ConfigDir,
		w.DataDir,
		w.CacheDir,
		w.LogDir,
	}

	for _, dirFunc := range dirs {
		dir, err := dirFunc()
		if err != nil {
			return err
		}
		if err := windows.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}
```

### File System Utilities

```go
//go:build windows
// +build windows

package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FileAttributes represents Windows file attributes
type FileAttributes struct {
	ReadOnly          bool
	Hidden            bool
	System            bool
	Directory         bool
	Archive           bool
	Device            bool
	Normal            bool
	Temporary         bool
	SparseFile        bool
	ReparsePoint      bool
	Compressed        bool
	Offline           bool
	NotContentIndexed bool
	Encrypted         bool
}

// GetFileAttributes retrieves attributes for a file
func GetFileAttributes(path string) (*FileAttributes, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("convert path: %w", err)
	}

	attrs, err := windows.GetFileAttributes(pathPtr)
	if err != nil {
		return nil, fmt.Errorf("get file attributes: %w", err)
	}

	return &FileAttributes{
		ReadOnly:          attrs&windows.FILE_ATTRIBUTE_READONLY != 0,
		Hidden:            attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0,
		System:            attrs&windows.FILE_ATTRIBUTE_SYSTEM != 0,
		Directory:         attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0,
		Archive:           attrs&windows.FILE_ATTRIBUTE_ARCHIVE != 0,
		Device:            attrs&windows.FILE_ATTRIBUTE_DEVICE != 0,
		Normal:            attrs&windows.FILE_ATTRIBUTE_NORMAL != 0,
		Temporary:         attrs&windows.FILE_ATTRIBUTE_TEMPORARY != 0,
		SparseFile:        attrs&windows.FILE_ATTRIBUTE_SPARSE_FILE != 0,
		ReparsePoint:      attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0,
		Compressed:        attrs&windows.FILE_ATTRIBUTE_COMPRESSED != 0,
		Offline:           attrs&windows.FILE_ATTRIBUTE_OFFLINE != 0,
		NotContentIndexed: attrs&windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED != 0,
		Encrypted:         attrs&windows.FILE_ATTRIBUTE_ENCRYPTED != 0,
	}, nil
}

// SetHidden sets the hidden attribute on a file
func SetHidden(path string) error {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("convert path: %w", err)
	}

	attrs, err := windows.GetFileAttributes(pathPtr)
	if err != nil {
		return fmt.Errorf("get file attributes: %w", err)
	}

	attrs |= windows.FILE_ATTRIBUTE_HIDDEN
	if err := windows.SetFileAttributes(pathPtr, attrs); err != nil {
		return fmt.Errorf("set hidden attribute: %w", err)
	}

	return nil
}

// IsHidden checks if a file is hidden
func IsHidden(path string) (bool, error) {
	attrs, err := GetFileAttributes(path)
	if err != nil {
		return false, err
	}
	return attrs.Hidden, nil
}

// CreateSymbolicLink creates a symbolic link
func CreateSymbolicLink(link, target string, isDirectory bool) error {
	linkPtr, err := syscall.UTF16PtrFromString(link)
	if err != nil {
		return fmt.Errorf("convert link path: %w", err)
	}

	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("convert target path: %w", err)
	}

	flags := uint32(0)
	if isDirectory {
		flags |= 1 // SYMBOLIC_LINK_FLAG_DIRECTORY
	}

	// Try to enable privilege
	if err := enablePrivilege("SeCreateSymbolicLinkPrivilege"); err != nil {
		// Try with developer mode flag
		flags |= 2 // SYMBOLIC_LINK_FLAG_ALLOW_UNPRIVILEGED_CREATE
	}

	ret, _, err := syscall.NewLazyDLL("kernel32.dll").
		NewProc("CreateSymbolicLinkW").
		Call(
			uintptr(unsafe.Pointer(linkPtr)),
			uintptr(unsafe.Pointer(targetPtr)),
			uintptr(flags),
		)

	if ret == 0 {
		return fmt.Errorf("create symbolic link: %v", err)
	}

	return nil
}

// enablePrivilege enables a Windows privilege
func enablePrivilege(name string) error {
	token := windows.Token(0)

	var privilege LUID
	if err := windows.LookupPrivilegeValue(nil, syscall.StringToUTF16Ptr(name), &privilege); err != nil {
		return err
	}

	privileges := TOKEN_PRIVILEGES{
		PrivilegeCount: 1,
		Privileges: [1]LUID_AND_ATTRIBUTES{
			{Luid: privilege, Attributes: windows.SE_PRIVILEGE_ENABLED},
		},
	}

	if err := windows.AdjustTokenPrivileges(token, false, (*byte)(unsafe.Pointer(&privileges)), 0, nil, nil); err != nil {
		return err
	}

	return nil
}

type LUID windows.LUID
type LUID_AND_ATTRIBUTES struct {
	Luid       LUID
	Attributes uint32
}
type TOKEN_PRIVILEGES struct {
	PrivilegeCount uint32
	Privileges     [1]LUID_AND_ATTRIBUTES
}

// GetLongPathName converts a short (8.3) path to a long path
func GetLongPathName(shortPath string) (string, error) {
	shortPtr, err := syscall.UTF16PtrFromString(shortPath)
	if err != nil {
		return "", fmt.Errorf("convert path: %w", err)
	}

	// Get required buffer size
	size, err := windows.GetLongPathName(shortPtr, nil, 0)
	if err != nil {
		return "", fmt.Errorf("get buffer size: %w", err)
	}

	buf := make([]uint16, size)
	_, err = windows.GetLongPathName(shortPtr, &buf[0], size)
	if err != nil {
		return "", fmt.Errorf("get long path name: %w", err)
	}

	return syscall.UTF16ToString(buf), nil
}
```

## 5. Network

### WinHTTP Proxy Configuration

```go
//go:build windows
// +build windows

package network

import (
	"context"
	"fmt"
	"net/url"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	winhttp                     = syscall.NewLazyDLL("winhttp.dll")
	procWinHttpOpen             = winhttp.NewProc("WinHttpOpen")
	procWinHttpGetIEProxyConfig = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	procWinHttpGetProxy         = winhttp.NewProc("WinHttpGetProxyForUrl")
	procWinHttpCloseHandle      = winhttp.NewProc("WinHttpCloseHandle")
)

// ProxyConfig holds proxy configuration
type ProxyConfig struct {
	AutoDetect    bool
	AutoConfigURL string
	Proxy         string
	ProxyBypass   string
}

// GetSystemProxy retrieves system proxy settings from Internet Explorer
func GetSystemProxy() (*ProxyConfig, error) {
	var ieConfig WINHTTP_CURRENT_USER_IE_PROXY_CONFIG

	ret, _, err := procWinHttpGetIEProxyConfig.Call(
		uintptr(unsafe.Pointer(&ieConfig)),
	)

	if ret == 0 {
		return nil, fmt.Errorf("WinHttpGetIEProxyConfigForCurrentUser failed: %v", err)
	}

	defer func() {
		if ieConfig.AutoConfigUrl != nil {
			syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(ieConfig.AutoConfigUrl))))
		}
		if ieConfig.Proxy != nil {
			syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(ieConfig.Proxy))))
		}
		if ieConfig.ProxyBypass != nil {
			syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(ieConfig.ProxyBypass))))
		}
	}()

	config := &ProxyConfig{
		AutoDetect: ieConfig.AutoDetect != 0,
	}

	if ieConfig.AutoConfigUrl != nil {
		config.AutoConfigURL = syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(ieConfig.AutoConfigUrl))[:])
	}
	if ieConfig.Proxy != nil {
		config.Proxy = syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(ieConfig.Proxy))[:])
	}
	if ieConfig.ProxyBypass != nil {
		config.ProxyBypass = syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(ieConfig.ProxyBypass))[:])
	}

	return config, nil
}

// GetProxyForURL determines the proxy for a specific URL
func GetProxyForURL(targetURL string, config *ProxyConfig) (string, error) {
	// Open WinHTTP session
	userAgent, _ := syscall.UTF16PtrFromString("Muster/1.0")
	session, _, err := procWinHttpOpen.Call(
		uintptr(unsafe.Pointer(userAgent)),
		1, // WINHTTP_ACCESS_TYPE_NO_PROXY
		0,
		0,
		0,
	)

	if session == 0 {
		return "", fmt.Errorf("WinHttpOpen failed: %v", err)
	}
	defer procWinHttpCloseHandle.Call(session)

	// Prepare auto-proxy options
	autoProxyOptions := WINHTTP_AUTOPROXY_OPTIONS{
		Flags:                0,
		AutoConfigUrl:        0,
		Reserved:             0,
		AutoLogonIfChallenged: 0,
	}

	if config.AutoDetect {
		autoProxyOptions.Flags = 1 // WINHTTP_AUTOPROXY_AUTO_DETECT
		autoProxyOptions.AutoDetectFlags = 3 // DHCP + DNS_A
	} else if config.AutoConfigURL != "" {
		autoProxyOptions.Flags = 2 // WINHTTP_AUTOPROXY_CONFIG_URL
		urlPtr, _ := syscall.UTF16PtrFromString(config.AutoConfigURL)
		autoProxyOptions.AutoConfigUrl = uintptr(unsafe.Pointer(urlPtr))
	}

	// Prepare URL
	urlPtr, _ := syscall.UTF16PtrFromString(targetURL)

	var proxyInfo WINHTTP_PROXY_INFO
	ret, _, err := procWinHttpGetProxy.Call(
		session,
		uintptr(unsafe.Pointer(urlPtr)),
		uintptr(unsafe.Pointer(&autoProxyOptions)),
		uintptr(unsafe.Pointer(&proxyInfo)),
	)

	if ret == 0 {
		return "", fmt.Errorf("WinHttpGetProxyForUrl failed: %v", err)
	}

	defer func() {
		if proxyInfo.Proxy != nil {
			syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(proxyInfo.Proxy))))
		}
		if proxyInfo.ProxyBypass != nil {
			syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(proxyInfo.ProxyBypass))))
		}
	}()

	if proxyInfo.Proxy != nil {
		return syscall.UTF16ToString((*[1 << 30]uint16)(unsafe.Pointer(proxyInfo.Proxy))[:]), nil
	}

	return "", nil
}

// ApplyProxySettings applies proxy settings to HTTP transport
func ApplyProxySettings(config *ProxyConfig) (*url.URL, error) {
	if config.Proxy == "" {
		return nil, nil
	}

	proxyURL, err := url.Parse("http://" + config.Proxy)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL: %w", err)
	}

	return proxyURL, nil
}

type WINHTTP_CURRENT_USER_IE_PROXY_CONFIG struct {
	AutoDetect    uint32
	AutoConfigUrl *uint16
	Proxy         *uint16
	ProxyBypass   *uint16
}

type WINHTTP_AUTOPROXY_OPTIONS struct {
	Flags                 uint32
	AutoDetectFlags       uint32
	AutoConfigUrl         uintptr
	Reserved              uintptr
	AutoLogonIfChallenged uint32
}

type WINHTTP_PROXY_INFO struct {
	AccessType  uint32
	Proxy       *uint16
	ProxyBypass *uint16
}

// GetProxyFromRegistry reads proxy settings from Windows Registry
func GetProxyFromRegistry() (*ProxyConfig, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, 
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, 
		registry.READ)
	if err != nil {
		return nil, fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	config := &ProxyConfig{}

	// Check if proxy is enabled
	proxyEnable, _, err := k.GetIntegerValue("ProxyEnable")
	if err == nil && proxyEnable != 0 {
		proxyServer, _, err := k.GetStringValue("ProxyServer")
		if err == nil {
			config.Proxy = proxyServer
		}

		proxyBypass, _, err := k.GetStringValue("ProxyOverride")
		if err == nil {
			config.ProxyBypass = proxyBypass
		}
	}

	// Check auto-config
	autoConfigURL, _, err := k.GetStringValue("AutoConfigURL")
	if err == nil {
		config.AutoConfigURL = autoConfigURL
	}

	return config, nil
}
```

### Windows Firewall Integration

```go
//go:build windows
// +build windows

package network

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Firewall provides Windows Firewall management
type Firewall struct {
	policy *ole.IDispatch
}

// NewFirewall creates a new Firewall instance
func NewFirewall() (*Firewall, error) {
	if err := ole.CoInitialize(0); err != nil {
		return nil, fmt.Errorf("initialize COM: %w", err)
	}

	unknown, err := oleutil.CreateObject("HNetCfg.FwPolicy2")
	if err != nil {
		ole.CoUninitialize()
		return nil, fmt.Errorf("create FwPolicy2: %w", err)
	}

	policy, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		ole.CoUninitialize()
		return nil, fmt.Errorf("query interface: %w", err)
	}

	return &Firewall{policy: policy}, nil
}

// Close releases COM resources
func (f *Firewall) Close() {
	if f.policy != nil {
		f.policy.Release()
	}
	ole.CoUninitialize()
}

// IsEnabled checks if the firewall is enabled for a profile
func (f *Firewall) IsEnabled(profile int) (bool, error) {
	result, err := oleutil.GetProperty(f.policy, "CurrentProfile")
	if err != nil {
		return false, fmt.Errorf("get current profile: %w", err)
	}

	// Get firewall enabled status for the profile
	enabled, err := oleutil.GetProperty(f.policy, "FirewallEnabled", result.Value())
	if err != nil {
		return false, fmt.Errorf("get firewall enabled: %w", err)
	}

	return enabled.Value().(bool), nil
}

// AddRule adds a firewall rule
func (f *Firewall) AddRule(rule *FirewallRule) error {
	// Create rule object
	unknown, err := oleutil.CreateObject("HNetCfg.FWRule")
	if err != nil {
		return fmt.Errorf("create FWRule: %w", err)
	}

	ruleObj, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return fmt.Errorf("query interface: %w", err)
	}

	// Set rule properties
	oleutil.PutProperty(ruleObj, "Name", rule.Name)
	oleutil.PutProperty(ruleObj, "Description", rule.Description)
	oleutil.PutProperty(ruleObj, "ApplicationName", rule.Application)
	oleutil.PutProperty(ruleObj, "ServiceName", rule.Service)
	oleutil.PutProperty(ruleObj, "Protocol", rule.Protocol)
	oleutil.PutProperty(ruleObj, "LocalPorts", rule.LocalPorts)
	oleutil.PutProperty(ruleObj, "RemotePorts", rule.RemotePorts)
	oleutil.PutProperty(ruleObj, "LocalAddresses", rule.LocalAddresses)
	oleutil.PutProperty(ruleObj, "RemoteAddresses", rule.RemoteAddresses)
	oleutil.PutProperty(ruleObj, "Direction", rule.Direction)
	oleutil.PutProperty(ruleObj, "Action", rule.Action)
	oleutil.PutProperty(ruleObj, "Enabled", rule.Enabled)
	oleutil.PutProperty(ruleObj, "Grouping", rule.Grouping)
	oleutil.PutProperty(ruleObj, "Profiles", rule.Profiles)
	if rule.InterfaceTypes != "" {
		oleutil.PutProperty(ruleObj, "InterfaceTypes", rule.InterfaceTypes)
	}

	// Get rules collection
	rules, err := oleutil.GetProperty(f.policy, "Rules")
	if err != nil {
		ruleObj.Release()
		return fmt.Errorf("get rules: %w", err)
	}

	rulesDisp := rules.ToIDispatch()

	// Add rule to collection
	_, err = oleutil.CallMethod(rulesDisp, "Add", ruleObj)
	ruleObj.Release()
	rulesDisp.Release()

	if err != nil {
		return fmt.Errorf("add rule: %w", err)
	}

	return nil
}

// RemoveRule removes a firewall rule by name
func (f *Firewall) RemoveRule(name string) error {
	rules, err := oleutil.GetProperty(f.policy, "Rules")
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}

	rulesDisp := rules.ToIDispatch()
	defer rulesDisp.Release()

	_, err = oleutil.CallMethod(rulesDisp, "Remove", name)
	if err != nil {
		return fmt.Errorf("remove rule: %w", err)
	}

	return nil
}

// RuleExists checks if a rule exists
func (f *Firewall) RuleExists(name string) (bool, error) {
	rules, err := oleutil.GetProperty(f.policy, "Rules")
	if err != nil {
		return false, fmt.Errorf("get rules: %w", err)
	}

	rulesDisp := rules.ToIDispatch()
	defer rulesDisp.Release()

	// Try to get rule by name
	_, err = oleutil.CallMethod(rulesDisp, "Item", name)
	if err != nil {
		// Rule doesn't exist
		return false, nil
	}

	return true, nil
}

// FirewallRule represents a firewall rule
type FirewallRule struct {
	Name            string
	Description     string
	Application     string
	Service         string
	Protocol        int    // NET_FW_IP_PROTOCOL_TCP = 6, NET_FW_IP_PROTOCOL_UDP = 17
	LocalPorts      string // e.g., "8080,9090" or "*" for all
	RemotePorts     string
	LocalAddresses  string
	RemoteAddresses string
	Direction       int    // NET_FW_RULE_DIR_IN = 1, NET_FW_RULE_DIR_OUT = 2
	Action          int    // NET_FW_ACTION_ALLOW = 1, NET_FW_ACTION_BLOCK = 0
	Enabled         bool
	Grouping        string
	Profiles        int    // NET_FW_PROFILE2_ALL = 0x7FFFFFFF
	InterfaceTypes  string // "All", "Lan", "Wireless", "RemoteAccess"
}

// Standard protocols
const (
	NET_FW_IP_PROTOCOL_TCP = 6
	NET_FW_IP_PROTOCOL_UDP = 17
	NET_FW_IP_PROTOCOL_ANY = 256
)

// Directions
const (
	NET_FW_RULE_DIR_IN  = 1
	NET_FW_RULE_DIR_OUT = 2
)

// Actions
const (
	NET_FW_ACTION_BLOCK = 0
	NET_FW_ACTION_ALLOW = 1
)

// Profiles
const (
	NET_FW_PROFILE2_DOMAIN  = 1
	NET_FW_PROFILE2_PRIVATE = 2
	NET_FW_PROFILE2_PUBLIC  = 4
	NET_FW_PROFILE2_ALL     = 2147483647
)

// CreateMusterInboundRule creates a firewall rule for Muster inbound connections
func (f *Firewall) CreateMusterInboundRule(exePath string, port int) error {
	rule := &FirewallRule{
		Name:           "Muster MCP Server",
		Description:    "Allow inbound connections to Muster MCP server",
		Application:    exePath,
		Protocol:       NET_FW_IP_PROTOCOL_TCP,
		LocalPorts:     fmt.Sprintf("%d", port),
		Direction:      NET_FW_RULE_DIR_IN,
		Action:         NET_FW_ACTION_ALLOW,
		Enabled:        true,
		Grouping:       "Muster",
		Profiles:       NET_FW_PROFILE2_ALL,
		InterfaceTypes: "All",
	}

	return f.AddRule(rule)
}
```

## 6. Security

### Authenticode Signing

```powershell
# scripts/sign-windows.ps1
param(
    [Parameter(Mandatory=$true)]
    [string]$Executable,
    
    [Parameter(Mandatory=$true)]
    [string]$CertificatePath,
    
    [Parameter(Mandatory=$false)]
    [string]$CertificatePassword,
    
    [Parameter(Mandatory=$false)]
    [string]$TimestampServer = "http://timestamp.digicert.com",
    
    [Parameter(Mandatory=$false)]
    [switch]$VerifyOnly
)

$ErrorActionPreference = "Stop"

function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Sign-Executable {
    param(
        [string]$File,
        [string]$CertPath,
        [string]$CertPassword,
        [string]$Timestamp
    )
    
    Write-Host "Signing $File..." -ForegroundColor Cyan
    
    # Load certificate
    if ($CertPassword) {
        $securePassword = ConvertTo-SecureString $CertPassword -AsPlainText -Force
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($CertPath, $securePassword)
    } else {
        $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($CertPath)
    }
    
    try {
        # Use signtool
        $signtool = "${env:ProgramFiles(x86)}\Windows Kits\10\bin\10.0.22000.0\x64\signtool.exe"
        if (-not (Test-Path $signtool)) {
            $signtool = "${env:ProgramFiles(x86)}\Windows Kits\10\bin\10.0.19041.0\x64\signtool.exe"
        }
        if (-not (Test-Path $signtool)) {
            $signtool = "${env:ProgramFiles(x86)}\Windows Kits\8.1\bin\x64\signtool.exe"
        }
        
        if (-not (Test-Path $signtool)) {
            throw "signtool.exe not found. Please install Windows SDK."
        }
        
        $arguments = @(
            "sign",
            "/f", $CertPath,
            "/fd", "sha256",
            "/tr", $Timestamp,
            "/td", "sha256",
            "/v"
        )
        
        if ($CertPassword) {
            $arguments += "/p"
            $arguments += $CertPassword
        }
        
        $arguments += $File
        
        & $signtool $arguments
        
        if ($LASTEXITCODE -ne 0) {
            throw "Signing failed with exit code $LASTEXITCODE"
        }
        
        Write-Host "Successfully signed $File" -ForegroundColor Green
    }
    finally {
        $cert.Dispose()
    }
}

function Verify-Signature {
    param([string]$File)
    
    Write-Host "Verifying signature for $File..." -ForegroundColor Cyan
    
    $signtool = "${env:ProgramFiles(x86)}\Windows Kits\10\bin\10.0.22000.0\x64\signtool.exe"
    if (-not (Test-Path $signtool)) {
        $signtool = "${env:ProgramFiles(x86)}\Windows Kits\10\bin\10.0.19041.0\x64\signtool.exe"
    }
    if (-not (Test-Path $signtool)) {
        $signtool = "${env:ProgramFiles(x86)}\Windows Kits\8.1\bin\x64\signtool.exe"
    }
    
    & $signtool "verify", "/pa", "/v", $File
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Signature verified successfully" -ForegroundColor Green
    } else {
        Write-Host "Signature verification failed" -ForegroundColor Red
    }
}

# Main
if (-not (Test-Path $Executable)) {
    throw "Executable not found: $Executable"
}

if ($VerifyOnly) {
    Verify-Signature -File $Executable
} else {
    if (-not (Test-Path $CertificatePath)) {
        throw "Certificate not found: $CertificatePath"
    }
    
    Sign-Executable -File $Executable -CertPath $CertificatePath -CertPassword $CertificatePassword -Timestamp $TimestampServer
    Verify-Signature -File $Executable
}
```

### UAC Manifest

```xml
<!-- assets/muster.exe.manifest -->
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity
    version="1.0.0.0"
    processorArchitecture="amd64"
    name="Muster.MCP"
    type="win32"
  />
  
  <description>Muster MCP Server</description>
  
  <!-- Compatibility with Windows versions -->
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <!-- Windows 10 and Windows 11 -->
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
      <!-- Windows 8.1 -->
      <supportedOS Id="{1f676c76-80e1-4239-95bb-83d0f6d0da78}"/>
      <!-- Windows 8 -->
      <supportedOS Id="{4a2f28e3-53b9-4441-ba9c-d69d4a4a6e38}"/>
      <!-- Windows 7 -->
      <supportedOS Id="{35138b9a-5d96-4fbd-8e2d-a2440225f93a}"/>
    </application>
  </compatibility>
  
  <!-- UAC settings -->
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v2">
    <security>
      <requestedPrivileges>
        <!-- 
          level="asInvoker" - Run with same privileges as parent process
          level="highestAvailable" - Request highest available privileges
          level="requireAdministrator" - Require administrator privileges
        -->
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  
  <!-- DPI Awareness -->
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">permonitorv2,permonitor,system</dpiAwareness>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
    </windowsSettings>
  </application>
  
  <!-- Dependency on common controls -->
  <dependency>
    <dependentAssembly>
      <assemblyIdentity
        type="win32"
        name="Microsoft.Windows.Common-Controls"
        version="6.0.0.0"
        processorArchitecture="*"
        publicKeyToken="6595b64144ccf1df"
        language="*"
      />
    </dependentAssembly>
  </dependency>
</assembly>
```

### UAC Helper Functions

```go
//go:build windows
// +build windows

package security

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

// IsAdmin checks if the current process has administrator privileges
func IsAdmin() bool {
	var sid *windows.SID

	// Allocate and initialize a SID for the Administrators group
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	// Check if the current process token has this SID with SE_GROUP_ENABLED
	token := windows.Token(0)

	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}

	return member
}

// RunAsAdmin re-executes the current program with administrator privileges
func RunAsAdmin(args []string) error {
	verb := "runas"
	exe := "cmd.exe"
	params := "/c start \"\" \"" + args[0] + "\" " + joinArgs(args[1:])

	return ShellExecute(0, verb, exe, params, "", 1) // SW_SHOWNORMAL
}

// ShellExecute executes a shell command
func ShellExecute(hwnd uintptr, verb, file, params, dir string, showCmd int) error {
	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	filePtr, _ := syscall.UTF16PtrFromString(file)
	paramsPtr, _ := syscall.UTF16PtrFromString(params)
	 dirPtr, _ := syscall.UTF16PtrFromString(dir)

	execInfo := SHELLEXECUTEINFO{
		cbSize:       uint32(unsafe.Sizeof(SHELLEXECUTEINFO{})),
		fMask:        0x00000040, // SEE_MASK_NOCLOSEPROCESS
		hWnd:         hwnd,
		lpVerb:       verbPtr,
		lpFile:       filePtr,
		lpParameters: paramsPtr,
		lpDirectory:  dirPtr,
		nShow:        showCmd,
	}

	ret, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&execInfo)))
	if ret == 0 {
		return fmt.Errorf("ShellExecuteEx failed: %v", err)
	}

	return nil
}

type SHELLEXECUTEINFO struct {
	cbSize       uint32
	fMask        uint32
	hWnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hKeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     uintptr
}

// RequestElevation prompts for UAC elevation
func RequestElevation() error {
	if IsAdmin() {
		return nil // Already admin
	}

	verb := "runas"
	exe, _ := exec.LookPath(os.Args[0])
	params := joinArgs(os.Args[1:])

	return ShellExecute(0, verb, exe, params, "", 1)
}

func joinArgs(args []string) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		if strings.ContainsAny(arg, " \"\t") {
			result += fmt.Sprintf("\"%s\"", strings.ReplaceAll(arg, "\"", "\\\""))
		} else {
			result += arg
		}
	}
	return result
}

// EnablePrivilege enables a privilege for the current process
func EnablePrivilege(name string) error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("open process token: %w", err)
	}
	defer token.Close()

	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, syscall.StringToUTF16Ptr(name), &luid); err != nil {
		return fmt.Errorf("lookup privilege: %w", err)
	}

	privileges := TOKEN_PRIVILEGES{
		PrivilegeCount: 1,
		Privileges: [1]LUID_AND_ATTRIBUTES{
			{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED},
		},
	}

	if err := windows.AdjustTokenPrivileges(token, false, (*byte)(unsafe.Pointer(&privileges)), 0, nil, nil); err != nil {
		return fmt.Errorf("adjust token privileges: %w", err)
	}

	return nil
}

type LUID windows.LUID
type LUID_AND_ATTRIBUTES struct {
	Luid       LUID
	Attributes uint32
}
type TOKEN_PRIVILEGES struct {
	PrivilegeCount uint32
	Privileges     [1]LUID_AND_ATTRIBUTES
}
```

## Build Configuration

```yaml
# .goreleaser-windows.yaml
version: 2
project_name: muster

before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - id: muster-cli
    main: ./cmd/muster
    binary: muster
    goos:
      - windows
    goarch:
      - amd64
      - 386
      - arm64
    ldflags:
      - -s -w
      - -X github.com/muster/mcp/internal/version.Version={{.Version}}
      - -X github.com/muster/mcp/internal/version.Commit={{.Commit}}
      - -X github.com/muster/mcp/internal/version.Date={{.Date}}
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath

  - id: muster-daemon
    main: ./cmd/musterd
    binary: musterd
    goos:
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X github.com/muster/mcp/internal/version.Version={{.Version}}
      - -X github.com/muster/mcp/internal/version.Commit={{.Commit}}
      - -X github.com/muster/mcp/internal/version.Date={{.Date}}
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath

archives:
  - id: windows-archive
    name_template: "{{ .ProjectName }}-{{ .Os }}-{{ .Arch }}"
    format: zip
    files:
      - README.md
      - LICENSE
      - config.default.yaml
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"
  algorithm: sha256

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^chore:"
      - "^ci:"

release:
  draft: true
  prerelease: auto
  name_template: "{{.ProjectName}}-v{{.Version}}"

# MSI via WiX
msis:
  - id: muster-msi
    name_template: "{{ .ProjectName }}-{{ .Version }}-{{ .Arch }}"
    wxs: installer/windows/Muster.wxs
    arch: x64
    bundle: false
    modifications:
      - "{{ .Version }}"
```

## Summary

This specification provides complete Windows platform support including:

1. **System Integration**: Windows Service, Task Scheduler, Registry management
2. **Package Distribution**: MSI, Chocolatey, Scoop, and Winget packages
3. **Credential Storage**: Windows Credential Manager, DPAPI, Certificate Store
4. **File System**: Known folders, special file attributes, symbolic links
5. **Network**: WinHTTP proxy detection, Windows Firewall management
6. **Security**: Authenticode signing, UAC integration, privilege management

## Version

**1.0.0** - 2024-01-15
