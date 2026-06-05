package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devcon/internal/config"
	"devcon/internal/runner"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `devcon - run .devcontainer environments without Node/npm

usage: devcon [global flags] <command> [args]

commands:
  up            build/create and start the dev container
  shell         start it (if needed) and open an interactive shell   [default]
  exec -- CMD   run a command inside the container
  down          stop and remove the container (compose: down)
  rebuild       rebuild image/container from scratch and start
  status        show resolved mode and container state
  config        print the resolved devcontainer.json as JSON

global flags:
  -w, --workspace DIR   project directory (default: current directory)
  -c, --config FILE     path to a devcontainer.json (overrides discovery)
  -h, --help            show this help
  -v, --version         print version and exit

environment:
  DEVCON_SHELL          shell to open for "shell" (default: bash, else sh)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "devcon: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	var workspace, configPath string

	i := 0
flags:
	for i < len(args) {
		switch args[i] {
		case "-w", "--workspace":
			i++
			if i >= len(args) {
				return fmt.Errorf("missing value for %s", args[i-1])
			}
			workspace = args[i]
		case "-c", "--config":
			i++
			if i >= len(args) {
				return fmt.Errorf("missing value for %s", args[i-1])
			}
			configPath = args[i]
		case "-h", "--help", "help":
			fmt.Print(usage)
			return nil
		case "-v", "--version":
			fmt.Println(version)
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag %q (try --help)", args[i])
			}
			break flags
		}
		i++
	}

	rest := args[i:]
	cmd := "shell"
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}

	// Commands that don't need a resolved devcontainer.json.
	if cmd == "version" {
		fmt.Println(version)
		return nil
	}

	root, err := resolveRoot(workspace)
	if err != nil {
		return err
	}
	if configPath != "" {
		if configPath, err = filepath.Abs(configPath); err != nil {
			return err
		}
	}

	dc, err := config.Load(root, configPath)
	if err != nil {
		return err
	}

	switch cmd {
	case "up":
		return runner.Up(dc, false)
	case "shell", "sh":
		return runner.Shell(dc)
	case "exec":
		if len(rest) > 0 && rest[0] == "--" {
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return fmt.Errorf("exec needs a command, e.g. devcon exec -- ls -la")
		}
		return runner.Exec(dc, rest)
	case "down", "stop":
		return runner.Down(dc)
	case "rebuild":
		return runner.Rebuild(dc)
	case "status", "ps":
		return runner.Status(dc)
	case "config":
		return runner.PrintConfig(dc)
	default:
		return fmt.Errorf("unknown command %q (try --help)", cmd)
	}
}

func resolveRoot(workspace string) (string, error) {
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		workspace = wd
	}
	return filepath.Abs(workspace)
}
