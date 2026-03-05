package platform

import (
	"fmt"
	"os/exec"
)

type ServiceConfig struct {
	Name        string
	DisplayName string
	Description string
	BinaryPath  string
	Args        []string
}

func InstallWindowsService(config ServiceConfig) error {
	args := []string{
		"create", config.Name,
		"binPath=", config.BinaryPath,
		"DisplayName=", config.DisplayName,
		"start=", "auto",
	}

	cmd := exec.Command("sc.exe", args...)
	return cmd.Run()
}

func UninstallWindowsService(name string) error {
	cmd := exec.Command("sc.exe", "delete", name)
	return cmd.Run()
}

func StartWindowsService(name string) error {
	cmd := exec.Command("sc.exe", "start", name)
	return cmd.Run()
}

func StopWindowsService(name string) error {
	cmd := exec.Command("sc.exe", "stop", name)
	return cmd.Run()
}

func EnableVirtualTerminal() error {
	return fmt.Errorf("not implemented on this platform")
}
