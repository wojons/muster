//go:build linux

package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemonizedMetricsWarningLog(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs daemon")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "daemon")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	socket := filepath.Join(dir, "daemon.sock")
	cmd := exec.Command(binary, "start", "--socket-path", socket, "--metrics-addr", held.Addr().String())
	cmd.Env = append(os.Environ(), "MUSTER_DAEMON_CHILD=0")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("start: %v: %s", err, out.String())
	}
	t.Cleanup(func() { _ = exec.Command(binary, "stop", "--socket-path", socket).Run() })
	if !strings.Contains(out.String(), socket+".log") {
		t.Fatalf("missing log path: %s", out.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(socket + ".log")
		if strings.Contains(string(b), "[WARN] metrics listener not started on "+held.Addr().String()) && strings.Contains(string(b), "bind: address already in use") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := os.ReadFile(socket + ".log")
	t.Fatalf("missing warning: %s", b)
}
