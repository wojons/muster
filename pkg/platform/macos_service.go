package platform

import (
	"fmt"
	"os"
	"os/exec"
)

func CreateEntitlements() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.cs.allow-jit</key>
    <true/>
    <key>com.apple.security.automation.apple-events</key>
    <true/>
</dict>
</plist>`
}

func BuildUniversalBinary(output, path string) error {
	amd64Binary := output + "-amd64"
	arm64Binary := output + "-arm64"

	cmd1 := exec.Command("go", "build", "-o", output,
		"-ldflags", "-s -w",
		"cmd/openapi-cli/main.go")
	cmd1.Env = append(os.Environ(),
		"GOOS=darwin",
		"GOARCH=amd64")
	if err := cmd1.Run(); err != nil {
		return fmt.Errorf("failed to build amd64: %w", err)
	}

	cmd2 := exec.Command("go", "build", "-o", amd64Binary,
		"-ldflags", "-s -w",
		"cmd/openapi-cli/main.go")
	cmd2.Env = append(os.Environ(),
		"GOOS=darwin",
		"GOARCH=amd64")
	if err := cmd2.Run(); err != nil {
		return fmt.Errorf("failed to build amd64: %w", err)
	}

	cmd3 := exec.Command("go", "build", "-o", arm64Binary,
		"-ldflags", "-s -w",
		"cmd/openapi-cli/main.go")
	cmd3.Env = append(os.Environ(),
		"GOOS=darwin",
		"GOARCH=arm64")
	if err := cmd3.Run(); err != nil {
		return fmt.Errorf("failed to build arm64: %w", err)
	}

	cmd4 := exec.Command("lipo", "-create", "-output", output,
		amd64Binary, arm64Binary)
	if err := cmd4.Run(); err != nil {
		return fmt.Errorf("failed to create universal binary: %w", err)
	}

	os.Remove(amd64Binary)
	os.Remove(arm64Binary)

	return nil
}

func GetEntitlementsPath() string {
	return "entitlements.plist"
}
