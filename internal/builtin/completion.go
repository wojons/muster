package builtin

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newCompletionCommand creates the completion command
func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `To load completions:

Bash:
  $ source <(openapi-cli completion bash)

  # To load completions for every session, execute once:
  # Linux:
  $ openapi-cli completion bash > /etc/bash_completion.d/openapi-cli
  # macOS:
  $ openapi-cli completion bash > /usr/local/etc/bash_completion.d/openapi-cli

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for every session, execute once:
  $ openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"

  # You will need to start a new shell for this setup to take effect.

fish:
  $ openapi-cli completion fish | source

  # To load completions for every session, execute once:
  $ openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish

PowerShell:
  PS> openapi-cli completion powershell | Out-String | Invoke-Expression

  # To load completions for every session, add the output of the above command to your powershell startup script.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletion(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell type: %s", args[0])
			}
		},
	}

	return cmd
}

// getCompletionDescription returns a description for shell completion
func getCompletionDescription(cmd *cobra.Command) string {
	if cmd.Short != "" {
		return cmd.Short
	}

	name := cmd.Name()
	name = strings.Replace(name, "-", " ", -1)
	name = strings.Title(name)
	return name
}
