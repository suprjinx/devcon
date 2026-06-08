// Package scaffold implements `devcon init`: it writes a starter
// .devcontainer/devcontainer.json (optionally with a Dockerfile) for a project,
// the same job VS Code's "Add Dev Container Configuration Files" command does,
// but non-interactively. A language is chosen by argument or auto-detected from
// project marker files, and the canonical mcr.microsoft.com/devcontainers/*
// images are used (which advertise the "vscode" user via image metadata).
package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Template is a built-in starter for one language/stack.
type Template struct {
	Key        string   // canonical name, e.g. "go"
	Aliases    []string // alternate names, e.g. "golang"
	Display    string   // human label, e.g. "Go"
	Image      string   // image repo without tag
	DefaultTag string   // default tag/variant
	Detect     []string // marker files (exact name or glob) that imply this stack
	PostCreate string   // a sensible postCreateCommand suggestion (commented out)
}

// templates is ordered most-specific first, which also decides detection order.
var templates = []Template{
	{Key: "go", Aliases: []string{"golang"}, Display: "Go",
		Image: "mcr.microsoft.com/devcontainers/go", DefaultTag: "1",
		Detect: []string{"go.mod", "*.go"}, PostCreate: "go version"},
	{Key: "rust", Display: "Rust",
		Image: "mcr.microsoft.com/devcontainers/rust", DefaultTag: "1",
		Detect: []string{"Cargo.toml"}, PostCreate: "rustc --version"},
	{Key: "python", Aliases: []string{"py"}, Display: "Python",
		Image: "mcr.microsoft.com/devcontainers/python", DefaultTag: "3.12",
		Detect: []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}, PostCreate: "python --version"},
	{Key: "typescript", Aliases: []string{"ts"}, Display: "TypeScript",
		Image: "mcr.microsoft.com/devcontainers/typescript-node", DefaultTag: "22",
		Detect: []string{"tsconfig.json"}, PostCreate: "node --version"},
	{Key: "node", Aliases: []string{"javascript", "js"}, Display: "Node.js",
		Image: "mcr.microsoft.com/devcontainers/javascript-node", DefaultTag: "22",
		Detect: []string{"package.json"}, PostCreate: "node --version"},
	{Key: "java", Display: "Java",
		Image: "mcr.microsoft.com/devcontainers/java", DefaultTag: "21",
		Detect: []string{"pom.xml", "build.gradle", "build.gradle.kts"}, PostCreate: "java -version"},
	{Key: "dotnet", Aliases: []string{"csharp", "cs"}, Display: ".NET",
		Image: "mcr.microsoft.com/devcontainers/dotnet", DefaultTag: "8.0",
		Detect: []string{"*.csproj", "*.sln", "global.json"}, PostCreate: "dotnet --info"},
	{Key: "ruby", Display: "Ruby",
		Image: "mcr.microsoft.com/devcontainers/ruby", DefaultTag: "3",
		Detect: []string{"Gemfile"}, PostCreate: "ruby --version"},
	{Key: "cpp", Aliases: []string{"c"}, Display: "C/C++",
		Image: "mcr.microsoft.com/devcontainers/cpp", DefaultTag: "debian",
		Detect: []string{"CMakeLists.txt"}, PostCreate: "gcc --version"},
	{Key: "base", Aliases: []string{"ubuntu"}, Display: "Ubuntu base",
		Image: "mcr.microsoft.com/devcontainers/base", DefaultTag: "ubuntu"},
}

type options struct {
	template   string
	version    string
	name       string
	dockerfile bool
	force      bool
	list       bool
}

// Init scaffolds a .devcontainer in root from the given `devcon init` arguments.
func Init(root string, args []string) error {
	opt, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opt.list {
		printList()
		return nil
	}

	tpl, err := pick(root, opt.template)
	if err != nil {
		return err
	}

	tag := opt.version
	if tag == "" {
		tag = tpl.DefaultTag
	}
	image := tpl.Image + ":" + tag
	name := opt.name
	if name == "" {
		name = filepath.Base(root)
	}

	dir := filepath.Join(root, ".devcontainer")
	cfg := filepath.Join(dir, "devcontainer.json")
	if _, err := os.Stat(cfg); err == nil && !opt.force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", cfg)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for rel, content := range render(tpl, name, image, opt.dockerfile) {
		p := filepath.Join(dir, rel)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "[devcon] wrote %s\n", p)
	}
	fmt.Fprintln(os.Stderr, "[devcon] done — try: devcon up")
	return nil
}

// pick resolves the requested template, or auto-detects one, or falls back to base.
func pick(root, requested string) (*Template, error) {
	if requested != "" {
		if t := find(requested); t != nil {
			return t, nil
		}
		return nil, fmt.Errorf("unknown template %q (try: devcon init --list)", requested)
	}
	if t := detect(root); t != nil {
		fmt.Fprintf(os.Stderr, "[devcon] detected %s project\n", t.Display)
		return t, nil
	}
	fmt.Fprintln(os.Stderr, "[devcon] no project type detected; using base (ubuntu)")
	return find("base"), nil
}

func find(name string) *Template {
	name = strings.ToLower(name)
	for i := range templates {
		if templates[i].Key == name {
			return &templates[i]
		}
		for _, a := range templates[i].Aliases {
			if a == name {
				return &templates[i]
			}
		}
	}
	return nil
}

func detect(root string) *Template {
	for i := range templates {
		for _, m := range templates[i].Detect {
			if markerExists(root, m) {
				return &templates[i]
			}
		}
	}
	return nil
}

func markerExists(root, pattern string) bool {
	if strings.ContainsAny(pattern, "*?[") {
		matches, _ := filepath.Glob(filepath.Join(root, pattern))
		return len(matches) > 0
	}
	_, err := os.Stat(filepath.Join(root, pattern))
	return err == nil
}

func parseArgs(args []string) (options, error) {
	var o options
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--list":
			o.list = true
		case a == "--dockerfile":
			o.dockerfile = true
		case a == "--image":
			o.dockerfile = false
		case a == "--force", a == "-f":
			o.force = true
		case a == "--version", a == "--tag":
			i++
			if i >= len(args) {
				return o, fmt.Errorf("missing value for %s", a)
			}
			o.version = args[i]
		case a == "--name":
			i++
			if i >= len(args) {
				return o, fmt.Errorf("missing value for %s", a)
			}
			o.name = args[i]
		case strings.HasPrefix(a, "--version="):
			o.version = strings.TrimPrefix(a, "--version=")
		case strings.HasPrefix(a, "--tag="):
			o.version = strings.TrimPrefix(a, "--tag=")
		case strings.HasPrefix(a, "--name="):
			o.name = strings.TrimPrefix(a, "--name=")
		case strings.HasPrefix(a, "-"):
			return o, fmt.Errorf("unknown flag %q for init", a)
		default:
			if o.template != "" {
				return o, fmt.Errorf("unexpected argument %q", a)
			}
			o.template = a
		}
	}
	return o, nil
}

// render returns the relative-path -> content files to write.
func render(t *Template, name, image string, dockerfile bool) map[string]string {
	post := t.PostCreate
	if post == "" {
		post = "echo 'ready'"
	}
	if dockerfile {
		return map[string]string{
			"devcontainer.json": dockerfileJSON(name, post),
			"Dockerfile":        dockerfileText(image),
		}
	}
	return map[string]string{
		"devcontainer.json": imageJSON(name, image, post),
	}
}

const header = "// devcontainer.json — generated by `devcon init`.\n" +
	"// Reference: https://containers.dev/implementors/json_reference/\n"

func imageJSON(name, image, post string) string {
	return header + fmt.Sprintf(`{
  "name": %s,
  "image": %s
  // "features": {},
  // "forwardPorts": [],
  // "postCreateCommand": %s,
  // "remoteUser": "vscode"
}
`, jsonStr(name), jsonStr(image), jsonStr(post))
}

func dockerfileJSON(name, post string) string {
	return header + fmt.Sprintf(`{
  "name": %s,
  "build": {
    "dockerfile": "Dockerfile"
  }
  // "features": {},
  // "forwardPorts": [],
  // "postCreateCommand": %s,
  // "remoteUser": "vscode"
}
`, jsonStr(name), jsonStr(post))
}

func dockerfileText(image string) string {
	return "FROM " + image + `

# Add build steps below, e.g. install OS packages or language tools:
# RUN apt-get update && apt-get install -y <package>
`
}

func printList() {
	fmt.Println("Available templates (devcon init <template>):")
	for _, t := range templates {
		alias := ""
		if len(t.Aliases) > 0 {
			alias = "  [" + strings.Join(t.Aliases, ", ") + "]"
		}
		fmt.Printf("  %-12s %s:%s%s\n", t.Key, t.Image, t.DefaultTag, alias)
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
