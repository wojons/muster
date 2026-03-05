package completion

import (
	"bytes"
	"fmt"
	"text/template"
)

// Shell represents supported shell types
type Shell string

const (
	ShellBash       Shell = "bash"
	ShellZsh        Shell = "zsh"
	ShellFish       Shell = "fish"
	ShellPowerShell Shell = "powershell"
)

// Command represents a CLI command for completion
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Flags       []Flag
	Subcommands []Command
	Args        []Argument
}

// Flag represents a command flag
type Flag struct {
	Long        string
	Short       string
	Description string
	Type        string
	Required    bool
	Default     string
	Values      []string
}

// Argument represents a positional argument
type Argument struct {
	Name        string
	Description string
	Required    bool
	Multiple    bool
}

// Generator creates shell completion scripts
type Generator struct {
	appName    string
	version    string
	commands   []Command
	globalFlag []Flag
	templates  map[Shell]*template.Template
}

// NewGenerator creates a new completion generator
func NewGenerator(appName, version string) *Generator {
	return &Generator{
		appName:   appName,
		version:   version,
		templates: make(map[Shell]*template.Template),
	}
}

// AddCommand adds a command to the completion generator
func (g *Generator) AddCommand(cmd Command) {
	g.commands = append(g.commands, cmd)
}

// AddGlobalFlag adds a global flag available to all commands
func (g *Generator) AddGlobalFlag(flag Flag) {
	g.globalFlag = append(g.globalFlag, flag)
}

// Generate creates completion script for the specified shell
func (g *Generator) Generate(shell Shell) (string, error) {
	if err := g.initTemplates(); err != nil {
		return "", fmt.Errorf("initializing templates: %w", err)
	}

	tmpl, ok := g.templates[shell]
	if !ok {
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}

	data := struct {
		AppName    string
		Version    string
		Commands   []Command
		GlobalFlag []Flag
	}{
		AppName:    g.appName,
		Version:    g.version,
		Commands:   g.commands,
		GlobalFlag: g.globalFlag,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// Supports returns true if the shell is supported
func (g *Generator) Supports(shell Shell) bool {
	switch shell {
	case ShellBash, ShellZsh, ShellFish, ShellPowerShell:
		return true
	default:
		return false
	}
}

func (g *Generator) initTemplates() error {
	if len(g.templates) > 0 {
		return nil
	}

	bashTmpl, err := template.New("bash").Parse(bashTemplate)
	if err != nil {
		return fmt.Errorf("parsing bash template: %w", err)
	}
	g.templates[ShellBash] = bashTmpl

	zshTmpl, err := template.New("zsh").Parse(zshTemplate)
	if err != nil {
		return fmt.Errorf("parsing zsh template: %w", err)
	}
	g.templates[ShellZsh] = zshTmpl

	fishTmpl, err := template.New("fish").Parse(fishTemplate)
	if err != nil {
		return fmt.Errorf("parsing fish template: %w", err)
	}
	g.templates[ShellFish] = fishTmpl

	psTmpl, err := template.New("powershell").Parse(powershellTemplate)
	if err != nil {
		return fmt.Errorf("parsing powershell template: %w", err)
	}
	g.templates[ShellPowerShell] = psTmpl

	return nil
}

const bashTemplate = `#!/bin/bash
# {{.AppName}} completion script
# Version: {{.Version}}

_{{.AppName}}_completion() {
    local cur prev words cword
    _init_completion || return
    
    local completions
    local result
    
    # Call the CLI completion endpoint
    result=$({{.AppName}} __complete "$cur" "${words[@]}" 2>/dev/null)
    
    # Parse the result
    local IFS=$'\n'
    completions=($result)
    
    # Check for directives in the last line
    local directive="${completions[${#completions[@]}-1]}"
    unset completions[${#completions[@]}-1]
    
    # Handle directives
    if [[ $directive -eq 1 ]]; then
        # ShellCompDirectiveError
        return 1
    fi
    if [[ $directive -eq 2 ]]; then
        # ShellCompDirectiveNoSpace
        compopt -o nospace
    fi
    if [[ $directive -eq 4 ]]; then
        # ShellCompDirectiveNoFileComp
        compopt +o default
    fi
    
    COMPREPLY=($(compgen -W '${completions[*]}' -- "$cur"))
}

complete -F _{{.AppName}}_completion {{.AppName}}
`

const zshTemplate = `#compdef {{.AppName}}
# {{.AppName}} completion script
# Version: {{.Version}}

_{{.AppName}}() {
    local -a completions
    local -a response
    local result
    
    # Call the CLI completion endpoint
    result=$({{.AppName}} __complete "${words[@]}" 2>/dev/null)
    
    # Parse response
    response=(${(f)result})
    
    # Extract directive
    local directive="${response[-1]}"
    response=(${response[1,-2]})
    
    # Build completions array
    for line in $response; do
        completions+=($line)
    done
    
    # Handle directive
    if [[ $directive -eq 4 ]]; then
        _describe -t commands "{{.AppName}} commands" completions
    else
        _alternative \
            'files:filename:_files' \
            'commands:command:compadd -a completions'
    fi
}

compdef _{{.AppName}} {{.AppName}}
`

const fishTemplate = `# {{.AppName}} completion script
# Version: {{.Version}}

function __{{.AppName}}_complete
    set -l cur (commandline -t)
    set -l words (commandline -opc)
    
    {{.AppName}} __complete $cur $words 2>/dev/null | while read -l line
        echo $line
    end
end

complete -c {{.AppName}} -f -a '(__{{.AppName}}_complete)'
`

const powershellTemplate = `# {{.AppName}} completion script
# Version: {{.Version}}

Register-ArgumentCompleter -Native -CommandName {{.AppName}} -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    
    $completions = @()
    
    # Get completions from CLI
    $result = & {{.AppName}} __complete $wordToComplete @($commandAst.CommandElements | ForEach-Object { $_.Value })
    
    # Parse results
    foreach ($line in $result) {
        if ($line -match '^\d+$') {
            continue
        }
        if ($line -ne "") {
            $completions += [System.Management.Automation.CompletionResult]::new(
                $line,
                $line,
                'ParameterValue',
                $line
            )
        }
    }
    
    return $completions
}
`

// GetInstallPath returns the recommended installation path for a completion script
func GetInstallPath(shell Shell) (string, error) {
	switch shell {
	case ShellBash:
		return "/etc/bash_completion.d/openapi-cli", nil
	case ShellZsh:
		return "/usr/local/share/zsh/site-functions/_openapi-cli", nil
	case ShellFish:
		return "~/.config/fish/completions/openapi-cli.fish", nil
	case ShellPowerShell:
		return "", fmt.Errorf("PowerShell completion should be piped: openapi-cli completion powershell | Out-String | Invoke-Expression")
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}
