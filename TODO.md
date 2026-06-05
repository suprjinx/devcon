# TODO

## 1. Refactor into `./internal` ✅ done

Moved the implementation out of the flat `package main` into internal packages,
leaving only the CLI entrypoint at the top level.

```
main.go                       -> thin entrypoint, arg parsing, dispatch
internal/config/config.go     -> DevContainer, Load, Mode, discovery, substitution
internal/config/jsonc.go      -> StripJSONC
internal/config/config_test.go
internal/docker/docker.go     -> docker/compose exec helpers, container state (no config dep)
internal/runner/runner.go     -> Up/Shell/Exec/Down/Rebuild/Status/PrintConfig
```

- `config` has zero dependency on Docker/exec. Because methods can't be defined
  on a type from another package, the former `*DevContainer` orchestration
  methods became functions in `runner` taking `*config.DevContainer`.
- `docker` knows nothing about devcontainer.json; callers pass formed args.
- `go build/vet/test ./...` green; end-to-end example re-verified.

## 2. Dogfood: give devcon its own dev container

Add a `.devcontainer/` to this repo so devcon can develop itself.

- Go toolchain image (match `go.mod`, currently Go 1.23+).
- Mount the module cache for fast rebuilds.
- `postCreateCommand` to run `go build ./...` / `go test ./...` as a smoke test.
- Verify with `devcon up && devcon exec -- go test ./...`.

## 3. Implement "Add Dev Container Configuration Files" (`devcon init`)

Fill the gap the official CLI leaves: a scaffolder. The upstream interactive
"Dev Containers: Add Dev Container Configuration Files…" lives in the VS Code
extension, not the CLI — `devcon init` should provide a terminal-native version.

- Detect project type from marker files: `go.mod`, `package.json`, `Gemfile`,
  `pyproject.toml`/`requirements.txt`, `Cargo.toml`, `pom.xml`/`build.gradle`.
- Write a sensible starter `.devcontainer/devcontainer.json` (+ `Dockerfile`
  when appropriate) for the detected stack.
- Refuse to clobber an existing `.devcontainer/` unless `--force`.
- Keep it zero-dependency: no OCI registry / Templates plumbing for v1; ship a
  few built-in templates embedded in the binary (`embed.FS`).
- Optional later: `--template-id` to apply a published Template, matching the
  upstream `devcontainer templates apply` primitive.
