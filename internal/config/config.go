// Package config discovers, parses, and resolves devcontainer.json files. It
// has no dependency on Docker or os/exec: it is pure data and resolution.
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DevContainer mirrors the subset of devcontainer.json that devcon understands.
type DevContainer struct {
	Name                 string            `json:"name"`
	Image                string            `json:"image"`
	Build                *BuildConfig      `json:"build"`
	DockerComposeFile    stringSlice       `json:"dockerComposeFile"`
	Service              string            `json:"service"`
	RunServices          []string          `json:"runServices"`
	WorkspaceFolder      string            `json:"workspaceFolder"`
	WorkspaceMount       string            `json:"workspaceMount"`
	RemoteUser           string            `json:"remoteUser"`
	ContainerUser        string            `json:"containerUser"`
	ForwardPorts         []flexPort        `json:"forwardPorts"`
	AppPort              []flexPort        `json:"appPort"`
	ContainerEnv         map[string]string `json:"containerEnv"`
	RemoteEnv            map[string]string `json:"remoteEnv"`
	RunArgs              []string          `json:"runArgs"`
	Mounts               []mountSpec       `json:"mounts"`
	OverrideCommand      *bool             `json:"overrideCommand"`
	OnCreateCommand      Lifecycle         `json:"onCreateCommand"`
	UpdateContentCommand Lifecycle         `json:"updateContentCommand"`
	PostCreateCommand    Lifecycle         `json:"postCreateCommand"`
	PostStartCommand     Lifecycle         `json:"postStartCommand"`
	PostAttachCommand    Lifecycle         `json:"postAttachCommand"`

	// runtime metadata, not deserialized from JSON
	Path                    string `json:"-"` // path to devcontainer.json
	Root                    string `json:"-"` // project root on the host
	workspaceFolderExplicit bool   `json:"-"`
}

// BuildConfig is the "build" object of devcontainer.json.
type BuildConfig struct {
	Dockerfile string            `json:"dockerfile"`
	Context    string            `json:"context"`
	Args       map[string]string `json:"args"`
	Target     string            `json:"target"`
	CacheFrom  stringSlice       `json:"cacheFrom"`
}

// Mode is how the container is produced.
type Mode int

const (
	ModeImage Mode = iota
	ModeBuild
	ModeCompose
)

func (m Mode) String() string {
	switch m {
	case ModeCompose:
		return "compose"
	case ModeBuild:
		return "dockerfile"
	default:
		return "image"
	}
}

// Mode reports how this container should be brought up.
func (dc *DevContainer) Mode() Mode {
	if len(dc.DockerComposeFile) > 0 {
		return ModeCompose
	}
	if dc.Build != nil && dc.Build.Dockerfile != "" {
		return ModeBuild
	}
	return ModeImage
}

// Load discovers (or, if explicitPath is set, reads) the devcontainer.json
// rooted at root, then resolves defaults and variable substitutions.
func Load(root, explicitPath string) (*DevContainer, error) {
	path := explicitPath
	if path == "" {
		p, err := locateConfig(root)
		if err != nil {
			return nil, err
		}
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var dc DevContainer
	if err := json.Unmarshal(StripJSONC(data), &dc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	dc.Path = path
	dc.Root = root
	dc.applyDefaults()
	dc.substitute()
	if err := dc.validate(); err != nil {
		return nil, err
	}
	return &dc, nil
}

// locateConfig looks for a devcontainer.json the same way the spec does.
func locateConfig(root string) (string, error) {
	for _, c := range []string{
		filepath.Join(root, ".devcontainer", "devcontainer.json"),
		filepath.Join(root, ".devcontainer.json"),
	} {
		if fileExists(c) {
			return c, nil
		}
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".devcontainer", "*", "devcontainer.json"))
	sort.Strings(matches)
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("no devcontainer.json found under %s/.devcontainer", root)
}

func (dc *DevContainer) applyDefaults() {
	base := filepath.Base(dc.Root)
	if dc.Name == "" {
		dc.Name = base
	}
	dc.workspaceFolderExplicit = dc.WorkspaceFolder != ""
	if dc.WorkspaceFolder == "" {
		dc.WorkspaceFolder = "/workspaces/" + base
	}
}

func (dc *DevContainer) validate() error {
	switch dc.Mode() {
	case ModeCompose:
		if dc.Service == "" {
			return fmt.Errorf("dockerComposeFile is set but no \"service\" given in %s", dc.Path)
		}
	case ModeImage:
		if dc.Image == "" {
			return fmt.Errorf("no \"image\", \"build\", or \"dockerComposeFile\" in %s", dc.Path)
		}
	}
	return nil
}

var localEnvRe = regexp.MustCompile(`\$\{localEnv:([A-Za-z_][A-Za-z0-9_]*)\}`)

// substitute expands the devcontainer ${...} variables we support across the
// fields where they commonly appear.
func (dc *DevContainer) substitute() {
	base := filepath.Base(dc.Root)
	repl := func(s string) string {
		s = strings.ReplaceAll(s, "${localWorkspaceFolder}", dc.Root)
		s = strings.ReplaceAll(s, "${localWorkspaceFolderBasename}", base)
		s = strings.ReplaceAll(s, "${containerWorkspaceFolder}", dc.WorkspaceFolder)
		s = strings.ReplaceAll(s, "${containerWorkspaceFolderBasename}", filepath.Base(dc.WorkspaceFolder))
		return localEnvRe.ReplaceAllStringFunc(s, func(m string) string {
			return os.Getenv(localEnvRe.FindStringSubmatch(m)[1])
		})
	}
	dc.WorkspaceMount = repl(dc.WorkspaceMount)
	dc.WorkspaceFolder = repl(dc.WorkspaceFolder)
	for i := range dc.RunArgs {
		dc.RunArgs[i] = repl(dc.RunArgs[i])
	}
	for i := range dc.Mounts {
		dc.Mounts[i].raw = repl(dc.Mounts[i].raw)
		for k, v := range dc.Mounts[i].fields {
			dc.Mounts[i].fields[k] = repl(v)
		}
	}
	for k, v := range dc.ContainerEnv {
		dc.ContainerEnv[k] = repl(v)
	}
	if dc.Build != nil {
		for k, v := range dc.Build.Args {
			dc.Build.Args[k] = repl(v)
		}
	}
}

// ---- identity / derived values ---------------------------------------------

func (dc *DevContainer) ContainerName() string {
	return "devcon-" + sanitize(filepath.Base(dc.Root)) + "-" + shortHash(dc.Root)
}

func (dc *DevContainer) ImageTag() string {
	return "devcon-" + sanitize(filepath.Base(dc.Root)) + ":latest"
}

func (dc *DevContainer) ComposeProject() string {
	return "devcon-" + strings.ReplaceAll(sanitize(filepath.Base(dc.Root)), ".", "-")
}

func (dc *DevContainer) Hostname() string {
	return sanitize(filepath.Base(dc.Root))
}

// Target is a human label for the thing we exec into.
func (dc *DevContainer) Target() string {
	if dc.Mode() == ModeCompose {
		return dc.Service
	}
	return dc.ContainerName()
}

// WorkspaceFolderExplicit reports whether workspaceFolder was set in the config
// (vs. defaulted), which decides whether we pass -w to `compose exec`.
func (dc *DevContainer) WorkspaceFolderExplicit() bool {
	return dc.workspaceFolderExplicit
}

func (dc *DevContainer) WorkspaceMountArg() string {
	if dc.WorkspaceMount != "" {
		return dc.WorkspaceMount
	}
	return "type=bind,source=" + dc.Root + ",target=" + dc.WorkspaceFolder
}

// ComposeFileArgs returns the global docker-compose flags (project name and
// each -f file), resolved relative to the config's directory.
func (dc *DevContainer) ComposeFileArgs() []string {
	dir := filepath.Dir(dc.Path)
	args := []string{"-p", dc.ComposeProject()}
	for _, f := range dc.DockerComposeFile {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		args = append(args, "-f", p)
	}
	return args
}

// Mounts exposes the resolved extra mounts as docker --mount argument strings.
func (dc *DevContainer) MountArgs() []string {
	var out []string
	for _, m := range dc.Mounts {
		if a := m.Arg(); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// ForwardPortArgs exposes forwardPorts + appPort as docker -p values.
func (dc *DevContainer) ForwardPortArgs() []string {
	var out []string
	for _, p := range dc.ForwardPorts {
		out = append(out, p.PublishArg())
	}
	for _, p := range dc.AppPort {
		out = append(out, p.PublishArg())
	}
	return out
}

// ---- custom JSON shapes ----------------------------------------------------

// stringSlice accepts either a JSON string or array of strings.
type stringSlice []string

func (s *stringSlice) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = arr
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*s = []string{one}
	return nil
}

// flexPort accepts a port given as a number (3000) or string ("3000:3000").
type flexPort string

func (p *flexPort) UnmarshalJSON(b []byte) error {
	*p = flexPort(strings.Trim(strings.TrimSpace(string(b)), `"`))
	return nil
}

// PublishArg turns a forwarded port into a docker -p value.
func (p flexPort) PublishArg() string {
	s := string(p)
	if strings.Contains(s, ":") {
		return s
	}
	return s + ":" + s
}

// mountSpec accepts the string form ("type=bind,source=...,target=...") or the
// object form ({ "type": "bind", "source": "...", "target": "..." }).
type mountSpec struct {
	raw    string
	fields map[string]string
}

func (m *mountSpec) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '{' {
		var obj map[string]any
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		m.fields = make(map[string]string, len(obj))
		for k, v := range obj {
			m.fields[k] = fmt.Sprint(v)
		}
		return nil
	}
	return json.Unmarshal(b, &m.raw)
}

// Arg renders the mount as a docker --mount value.
func (m mountSpec) Arg() string {
	if m.raw != "" {
		return m.raw
	}
	if len(m.fields) == 0 {
		return ""
	}
	order := []string{"type", "source", "target", "readonly", "consistency", "bind-propagation"}
	seen := map[string]bool{}
	var parts []string
	for _, k := range order {
		if v, ok := m.fields[k]; ok {
			parts = append(parts, k+"="+v)
			seen[k] = true
		}
	}
	var extra []string
	for k := range m.fields {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		parts = append(parts, k+"="+m.fields[k])
	}
	return strings.Join(parts, ",")
}

// Command is one resolved lifecycle command: either a shell string or argv.
type Command struct {
	Shell string
	Argv  []string
}

func (c Command) Display() string {
	if c.Shell != "" {
		return c.Shell
	}
	return strings.Join(c.Argv, " ")
}

// Lifecycle accepts a string, an argv array, or an object of named commands.
type Lifecycle []Command

func (l *Lifecycle) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	switch b[0] {
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c, err := parseCommand(obj[k])
			if err != nil {
				return err
			}
			*l = append(*l, c)
		}
	default:
		c, err := parseCommand(b)
		if err != nil {
			return err
		}
		*l = Lifecycle{c}
	}
	return nil
}

func parseCommand(b []byte) (Command, error) {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var argv []string
		if err := json.Unmarshal(b, &argv); err != nil {
			return Command{}, err
		}
		return Command{Argv: argv}, nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return Command{}, err
	}
	return Command{Shell: s}, nil
}

// ---- small utilities -------------------------------------------------------

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		return "devcontainer"
	}
	return out
}

func shortHash(s string) string {
	if abs, err := filepath.Abs(s); err == nil {
		s = abs
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}
