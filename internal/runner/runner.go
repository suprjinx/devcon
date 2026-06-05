// Package runner orchestrates Docker to bring a dev container up and attach to
// it, translating a resolved config.DevContainer into docker/compose commands.
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devcon/internal/config"
	"devcon/internal/docker"
)

// Up brings the dev container up (creating it the first time) without attaching.
func Up(dc *config.DevContainer, rebuild bool) error {
	if dc.Mode() == config.ModeCompose {
		return composeUp(dc, rebuild)
	}
	return localUp(dc, rebuild)
}

// Shell ensures the container is up and drops into an interactive shell.
func Shell(dc *config.DevContainer) error {
	if err := Up(dc, false); err != nil {
		return err
	}
	if err := runLifecycle(dc, dc.PostAttachCommand, "postAttach"); err != nil {
		return err
	}
	sh := detectShell(dc)
	fmt.Fprintf(os.Stderr, "[devcon] entering %s (%s)\n", dc.Target(), sh)
	return docker.RunCmd(execCmd(dc, true, sh))
}

// Exec ensures the container is up and runs an arbitrary command in it.
func Exec(dc *config.DevContainer, command []string) error {
	if err := Up(dc, false); err != nil {
		return err
	}
	return docker.RunCmd(execCmd(dc, true, command...))
}

// Down stops and removes the container (or `compose down`).
func Down(dc *config.DevContainer) error {
	if dc.Mode() == config.ModeCompose {
		return docker.RunCmd(docker.Compose(append(dc.ComposeFileArgs(), "down")...))
	}
	name := dc.ContainerName()
	if docker.State(name) == "" {
		fmt.Fprintf(os.Stderr, "[devcon] %s not running\n", name)
		return nil
	}
	return docker.Run("rm", "-f", name)
}

// Rebuild tears everything down and recreates it from scratch.
func Rebuild(dc *config.DevContainer) error {
	return Up(dc, true)
}

// Status prints the resolved mode and container state.
func Status(dc *config.DevContainer) error {
	fmt.Printf("config:    %s\n", dc.Path)
	fmt.Printf("mode:      %s\n", dc.Mode())
	fmt.Printf("workspace: %s -> %s\n", dc.Root, dc.WorkspaceFolder)
	if dc.Mode() == config.ModeCompose {
		fmt.Printf("project:   %s\n", dc.ComposeProject())
		fmt.Printf("service:   %s\n", dc.Service)
		if out, err := docker.OutputCmd(docker.Compose(append(dc.ComposeFileArgs(), "ps")...)); err == nil && out != "" {
			fmt.Println(out)
		}
		return nil
	}
	name := dc.ContainerName()
	state := docker.State(name)
	if state == "" {
		state = "(not created)"
	}
	fmt.Printf("container: %s [%s]\n", name, state)
	if dc.Mode() == config.ModeBuild {
		fmt.Printf("image:     %s (built: %t)\n", dc.ImageTag(), docker.ImageExists(dc.ImageTag()))
	}
	return nil
}

// PrintConfig prints the resolved devcontainer.json as plain JSON.
func PrintConfig(dc *config.DevContainer) error {
	data, err := os.ReadFile(dc.Path)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, config.StripJSONC(data), "", "  "); err != nil {
		return err
	}
	fmt.Println(pretty.String())
	return nil
}

// ---- image / dockerfile path -----------------------------------------------

func localUp(dc *config.DevContainer, rebuild bool) error {
	name := dc.ContainerName()
	state := docker.State(name)
	// A "created" container never started successfully for us (devcon always
	// runs detached and expects "running"); treat it as incomplete and rebuild
	// it rather than trying to `docker start` a half-made container.
	if state == "created" || (rebuild && state != "") {
		_ = docker.Run("rm", "-f", name)
		state = ""
	}

	switch state {
	case "running":
		return nil
	case "":
		if dc.Mode() == config.ModeBuild && (rebuild || !docker.ImageExists(dc.ImageTag())) {
			if err := buildImage(dc, rebuild); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "[devcon] creating %s\n", name)
		if err := createContainer(dc); err != nil {
			return err
		}
		if err := runCreateHooks(dc); err != nil {
			return err
		}
	default: // exited, created, paused, ...
		fmt.Fprintf(os.Stderr, "[devcon] starting %s\n", name)
		if err := docker.Run("start", name); err != nil {
			return err
		}
	}
	return runLifecycle(dc, dc.PostStartCommand, "postStart")
}

func buildImage(dc *config.DevContainer, noCache bool) error {
	dir := filepath.Dir(dc.Path)
	b := dc.Build
	dockerfile := b.Dockerfile
	if !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(dir, dockerfile)
	}
	context := dir
	if b.Context != "" {
		if filepath.IsAbs(b.Context) {
			context = b.Context
		} else {
			context = filepath.Join(dir, b.Context)
		}
	}
	args := []string{"build", "-t", dc.ImageTag(), "-f", dockerfile}
	if noCache {
		args = append(args, "--no-cache")
	}
	if b.Target != "" {
		args = append(args, "--target", b.Target)
	}
	for k, v := range b.Args {
		args = append(args, "--build-arg", k+"="+v)
	}
	args = append(args, context)
	fmt.Fprintf(os.Stderr, "[devcon] building %s\n", dc.ImageTag())
	return docker.Run(args...)
}

func createContainer(dc *config.DevContainer) error {
	image := dc.Image
	if dc.Mode() == config.ModeBuild {
		image = dc.ImageTag()
	}

	args := []string{
		"run", "-d",
		"--name", dc.ContainerName(),
		"--hostname", dc.Hostname(),
		"-l", "devcon.root=" + dc.Root,
		"--mount", dc.WorkspaceMountArg(),
		"-w", dc.WorkspaceFolder,
	}
	for _, m := range dc.MountArgs() {
		args = append(args, "--mount", m)
	}
	for k, v := range dc.ContainerEnv {
		args = append(args, "-e", k+"="+v)
	}
	for _, p := range dc.ForwardPortArgs() {
		args = append(args, "-p", p)
	}
	if dc.ContainerUser != "" {
		args = append(args, "-u", dc.ContainerUser)
	}
	args = append(args, dc.RunArgs...)
	args = append(args, image)

	// Keep the container alive when it has no long-running process of its own.
	if dc.OverrideCommand == nil || *dc.OverrideCommand {
		args = append(args, "/bin/sh", "-c", "while sleep 1000; do :; done")
	}
	return docker.Run(args...)
}

// ---- compose path ----------------------------------------------------------

func composeUp(dc *config.DevContainer, rebuild bool) error {
	preID, _ := docker.OutputCmd(docker.Compose(append(dc.ComposeFileArgs(), "ps", "-q", dc.Service)...))
	fresh := preID == ""

	up := []string{"up", "-d"}
	if rebuild {
		up = append(up, "--build")
	}
	up = append(up, dc.RunServices...) // empty => all services
	if err := docker.RunCmd(docker.Compose(append(dc.ComposeFileArgs(), up...)...)); err != nil {
		return err
	}

	if fresh {
		if err := runCreateHooks(dc); err != nil {
			return err
		}
	}
	return runLifecycle(dc, dc.PostStartCommand, "postStart")
}

// ---- lifecycle + exec plumbing ---------------------------------------------

// runCreateHooks runs the one-time create-phase lifecycle commands in order.
func runCreateHooks(dc *config.DevContainer) error {
	for _, h := range []struct {
		cmds  config.Lifecycle
		phase string
	}{
		{dc.OnCreateCommand, "onCreate"},
		{dc.UpdateContentCommand, "updateContent"},
		{dc.PostCreateCommand, "postCreate"},
	} {
		if err := runLifecycle(dc, h.cmds, h.phase); err != nil {
			return err
		}
	}
	return nil
}

// execCmd builds a `docker exec` / `docker compose exec` command targeting the
// dev container, honoring remoteUser and the workspace folder.
func execCmd(dc *config.DevContainer, interactive bool, command ...string) *exec.Cmd {
	user := dc.RemoteUser
	if user == "" {
		user = dc.ContainerUser
	}

	if dc.Mode() == config.ModeCompose {
		args := append(dc.ComposeFileArgs(), "exec")
		if !interactive {
			args = append(args, "-T")
		}
		if user != "" {
			args = append(args, "-u", user)
		}
		if dc.WorkspaceFolderExplicit() {
			args = append(args, "-w", dc.WorkspaceFolder)
		}
		args = append(args, dc.Service)
		args = append(args, command...)
		return docker.Compose(args...)
	}

	args := []string{"exec"}
	if interactive {
		args = append(args, "-it")
	} else {
		args = append(args, "-i")
	}
	if user != "" {
		args = append(args, "-u", user)
	}
	args = append(args, "-w", dc.WorkspaceFolder, dc.ContainerName())
	args = append(args, command...)
	return docker.Cmd(args...)
}

func runLifecycle(dc *config.DevContainer, cmds config.Lifecycle, phase string) error {
	for _, c := range cmds {
		fmt.Fprintf(os.Stderr, "[devcon] %s: %s\n", phase, c.Display())
		var ec *exec.Cmd
		switch {
		case c.Shell != "":
			ec = execCmd(dc, false, "/bin/sh", "-c", c.Shell)
		case len(c.Argv) > 0:
			ec = execCmd(dc, false, c.Argv...)
		default:
			continue
		}
		if err := docker.RunCmd(ec); err != nil {
			return fmt.Errorf("%s command failed: %w", phase, err)
		}
	}
	return nil
}

// detectShell prefers bash inside the container, falling back to sh. It can be
// overridden with the DEVCON_SHELL environment variable.
func detectShell(dc *config.DevContainer) string {
	if s := os.Getenv("DEVCON_SHELL"); s != "" {
		return s
	}
	out, err := docker.OutputCmd(execCmd(dc, false, "/bin/sh", "-c",
		"command -v bash >/dev/null 2>&1 && echo bash || echo sh"))
	if err == nil && strings.TrimSpace(out) == "bash" {
		return "bash"
	}
	return "sh"
}
