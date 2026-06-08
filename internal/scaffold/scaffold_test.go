package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"devcon/internal/config"
)

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"go.mod":           "go",
		"Cargo.toml":       "rust",
		"requirements.txt": "python",
		"package.json":     "node",
		"Gemfile":          "ruby",
		"main.csproj":      "dotnet", // glob marker
	}
	for marker, want := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, marker), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		got := detect(dir)
		if got == nil || got.Key != want {
			t.Errorf("detect(%s) = %v, want %s", marker, got, want)
		}
	}
	if detect(t.TempDir()) != nil {
		t.Error("empty dir should detect nothing")
	}
}

// Generated configs must be valid JSONC that devcon itself can parse.
func TestRenderedConfigParses(t *testing.T) {
	for _, dockerfile := range []bool{false, true} {
		files := render(find("go"), "demo", "mcr.microsoft.com/devcontainers/go:1", dockerfile)
		clean := config.StripJSONC([]byte(files["devcontainer.json"]))
		var got map[string]any
		if err := json.Unmarshal(clean, &got); err != nil {
			t.Fatalf("dockerfile=%v: invalid JSONC: %v\n%s", dockerfile, err, files["devcontainer.json"])
		}
		if got["name"] != "demo" {
			t.Errorf("name = %v", got["name"])
		}
		if dockerfile {
			if _, ok := files["Dockerfile"]; !ok {
				t.Error("dockerfile mode should emit a Dockerfile")
			}
		} else if got["image"] != "mcr.microsoft.com/devcontainers/go:1" {
			t.Errorf("image = %v", got["image"])
		}
	}
}

func TestInitWritesAndRefuses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)

	if err := Init(dir, nil); err != nil { // auto-detect go
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, ".devcontainer", "devcontainer.json")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("expected %s: %v", cfg, err)
	}
	// second run without --force must refuse
	if err := Init(dir, nil); err == nil {
		t.Error("expected refusal when .devcontainer exists")
	}
	// --force overwrites
	if err := Init(dir, []string{"--force"}); err != nil {
		t.Errorf("--force should overwrite: %v", err)
	}
}
