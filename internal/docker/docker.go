// Package docker is a thin wrapper over the docker and docker-compose CLIs. It
// knows nothing about devcontainer.json; callers pass fully-formed arguments.
package docker

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Cmd builds an *exec.Cmd for the docker CLI (no stdio attached yet).
func Cmd(args ...string) *exec.Cmd {
	return exec.Command("docker", args...)
}

// RunCmd attaches the parent's stdio (so interactive exec and live build output
// both work) and runs c.
func RunCmd(c *exec.Cmd) error {
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// Run runs `docker <args>` with inherited stdio.
func Run(args ...string) error {
	return RunCmd(Cmd(args...))
}

// OutputCmd runs c capturing stdout (stderr discarded) and returns the trimmed
// output. Used for inspection queries.
func OutputCmd(c *exec.Cmd) (string, error) {
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = io.Discard
	err := c.Run()
	return strings.TrimSpace(out.String()), err
}

// Output runs `docker <args>` capturing stdout.
func Output(args ...string) (string, error) {
	return OutputCmd(Cmd(args...))
}

// State returns the container status ("running", "exited", "created",
// "paused", ...) or "" when the container does not exist.
func State(name string) string {
	out, err := Output("inspect", "-f", "{{.State.Status}}", name)
	if err != nil {
		return ""
	}
	return out
}

// ImageExists reports whether a local image with the given tag exists.
func ImageExists(tag string) bool {
	return Cmd("image", "inspect", tag).Run() == nil
}

// ---- docker compose --------------------------------------------------------

var (
	composeV2   bool
	composeOnce sync.Once
)

// Compose builds an *exec.Cmd for docker compose, preferring the v2 plugin
// (`docker compose`) and falling back to the standalone `docker-compose`.
func Compose(args ...string) *exec.Cmd {
	composeOnce.Do(func() {
		composeV2 = exec.Command("docker", "compose", "version").Run() == nil
	})
	if composeV2 {
		return exec.Command("docker", append([]string{"compose"}, args...)...)
	}
	return exec.Command("docker-compose", args...)
}
