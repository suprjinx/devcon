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

## 2. Dogfood: give devcon its own dev container ✅ done

`.devcontainer/devcontainer.json` (scaffolded with `devcon init`, then tweaked):

- Go image `mcr.microsoft.com/devcontainers/go:1` (floats latest 1.x).
- Persistent module-cache volume mounted at `/go/pkg/mod`.
- `postCreateCommand: go version && go build ./... && go test ./...` smoke test.
- Verified: `devcon up` pulls the image, attaches as `vscode` (from metadata),
  and the smoke test builds + passes all tests inside the container.

## 3. Implement "Add Dev Container Configuration Files" (`devcon init`) ✅ done

Non-interactive scaffolder in `internal/scaffold`. Built-in templates rendered
programmatically (no `embed.FS` needed) using the canonical
`mcr.microsoft.com/devcontainers/*` images — which advertise the `vscode` user
via metadata, so generated configs "just work" with the user-resolution above.

- `devcon init [LANG]` — auto-detects from marker files (`go.mod`, `Cargo.toml`,
  `package.json`, `pyproject.toml`/`requirements.txt`, `Gemfile`, `pom.xml`,
  `*.csproj`, `tsconfig.json`, `CMakeLists.txt`); falls back to ubuntu `base`.
- Flags: `--list`, `--version <tag>`, `--name`, `--dockerfile` (vs `image`),
  `--force` (refuses to clobber otherwise).
- Templates: go, rust, python, node, typescript, java, dotnet, ruby, cpp, base.

### Possible follow-ups
- `--template-id` to apply a published OCI Template (`devcontainer templates apply`).
- Read remoteEnv from metadata into the opened shell; compose-image metadata.
