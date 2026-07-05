package builtin

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/daemon"
)

func newDaemonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Daemon management commands",
		Long:  `Manage the daemon process for background API proxying and persistent connections.`,
	}

	cmd.AddCommand(newDaemonStartCommand())
	cmd.AddCommand(newDaemonStopCommand())
	cmd.AddCommand(newDaemonStatusCommand())

	return cmd
}

func newDaemonStartCommand() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		Long:  `Start the daemon process in the background.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.ControllerConfig{
				SocketPath: socketPath,
				Version:    "1.0.0",
			}

			controller, err := daemon.NewController(cfg)
			if err != nil {
				return fmt.Errorf("create daemon controller: %w", err)
			}

			if err := controller.Start(); err != nil {
				return fmt.Errorf("start daemon: %w", err)
			}

			fmt.Println("✓ Daemon started successfully")
			if socketPath != "" {
				fmt.Printf("  Socket: %s\n", socketPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", "", "Path to Unix socket file")

	return cmd
}

func newDaemonStopCommand() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		Long:  `Stop the daemon process and clean up resources.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.ControllerConfig{
				SocketPath: socketPath,
				Version:    "1.0.0",
			}

			controller, err := daemon.NewController(cfg)
			if err != nil {
				return fmt.Errorf("create daemon controller: %w", err)
			}

			if err := controller.Stop(); err != nil {
				return fmt.Errorf("stop daemon: %w", err)
			}

			fmt.Println("✓ Daemon stopped successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", "", "Path to Unix socket file")

	return cmd
}

func newDaemonStatusCommand() *cobra.Command {
	var socketPath string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		Long:  `Display the current status of the daemon process.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := daemon.ControllerConfig{
				SocketPath: socketPath,
				Version:    "1.0.0",
			}

			controller, err := daemon.NewController(cfg)
			if err != nil {
				return fmt.Errorf("create daemon controller: %w", err)
			}

			status, err := controller.Status()
			if err != nil {
				return fmt.Errorf("get daemon status: %w", err)
			}

			if status.Running {
				fmt.Printf("Status: Running\n")
				fmt.Printf("  PID: %d\n", status.PID)
				fmt.Printf("  Started: %s\n", status.StartTime.Format("2006-01-02 15:04:05"))
			} else {
				fmt.Println("Status: Stopped")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&socketPath, "socket", "", "Path to Unix socket file")

	return cmd
}
