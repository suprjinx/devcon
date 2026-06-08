# devcon

Run [Dev Container](https://containers.dev) environments **without Node or the
official `@devcontainers/cli`**. `devcon` is a single Go binary that reads the
`devcontainer.json` in your project, builds/starts the container with Docker,
bind-mounts the project, and drops you into a shell inside it.

It deliberately implements only the parts you need to *get into a working
container*: image / Dockerfile / docker-compose, the workspace mount, lifecycle
commands, and a terminal. It does **not** implement Dev Container *Features*,
VS Code customizations, or templates.

## Install

```sh
make install                 # builds and copies to ~/.local/bin/devcon
# or
go build -o devcon . && mv devcon ~/.local/bin/
```

Requires Docker (with the Compose v2 plugin for compose-based configs).

## Usage

Scaffold a config for a project that doesn't have one (non-interactive — the
terminal-native counterpart of VS Code's "Add Dev Container Configuration Files"):

```sh
devcon init                 # auto-detect the stack from project files
devcon init go              # or pick one explicitly
devcon init python --version 3.11 --dockerfile
devcon init --list          # show available templates
```

Then run from anywhere inside your project (it discovers `.devcontainer/`):

```sh
devcon            # start the container if needed and open a shell  (default)
devcon up         # build/create and start, but don't attach
devcon exec -- go test ./...   # run a command inside the container
devcon status     # show resolved mode + container state
devcon config     # print the resolved devcontainer.json (JSONC -> JSON)
devcon down       # stop and remove
devcon rebuild    # recreate from scratch (image --no-cache / compose --build)
```

Flags:

```
-w, --workspace DIR   project directory (default: current directory)
-c, --config FILE     path to a devcontainer.json (overrides discovery)
DEVCON_SHELL=...      shell to open (default: bash if present, else sh)
```

## Supported `devcontainer.json`

| Area        | Keys |
|-------------|------|
| Image       | `image` |
| Build       | `build.dockerfile`, `build.context`, `build.args`, `build.target` |
| Compose     | `dockerComposeFile`, `service`, `runServices` |
| Workspace   | `workspaceFolder`, `workspaceMount` (defaults to a bind mount at `/workspaces/<name>`) |
| Runtime     | `remoteUser`, `containerUser`, `containerEnv`, `forwardPorts`, `appPort`, `mounts`, `runArgs`, `overrideCommand` |
| Lifecycle   | `onCreateCommand`, `updateContentCommand`, `postCreateCommand`, `postStartCommand`, `postAttachCommand` |
| Variables   | `${localWorkspaceFolder}`, `${localWorkspaceFolderBasename}`, `${containerWorkspaceFolder}`, `${containerWorkspaceFolderBasename}`, `${localEnv:VAR}` |

JSONC is handled natively: `//` and `/* */` comments and trailing commas are
stripped before parsing (see `jsonc.go`).

### Lifecycle semantics

`onCreate` / `updateContent` / `postCreate` run **once**, when the container is
first created. `postStart` runs every time it is (re)started. `postAttach` runs
just before `devcon shell` opens a terminal. Create-time commands are skipped on
subsequent `devcon up`/`shell` calls because the container already exists.

### Which user you get

devcon resolves the container user the same way VS Code does, in precedence order:

1. `remoteUser` / `containerUser` in devcontainer.json
2. the image's `devcontainer.metadata` label (how Microsoft/Features base images
   advertise `remoteUser: vscode`)
3. the image's `USER` directive
4. root

So an image built `FROM mcr.microsoft.com/devcontainers/...` drops you in as
`vscode` automatically, with no `remoteUser` needed in the json — and because
that user's UID usually matches your host UID, bind-mounted files are owned
correctly. (Compose services are skipped here; set `remoteUser` explicitly.)

### Git ownership

Dev container workspaces are bind-mounted from the host, so files are owned by
the host UID while processes inside run as a different user. Git's
"dubious ownership" guard then refuses to operate on the repo. Before running
lifecycle commands, devcon marks the workspace as trusted for all users:

```sh
git config --system --add safe.directory <workspaceFolder>
```

This is best-effort (run as root, ignored if git isn't installed) and matches
what the official tooling does. Disable it with `DEVCON_NO_GIT_SAFE_DIRECTORY=1`.

## Not implemented (yet)

- Dev Container **Features** (`features`) — the big one.
- `initializeCommand` (would run on the host, not in the container).
- `remoteEnv` injection into the opened shell (currently only `containerEnv`).
- `customizations`, port attributes, `secrets`, multi-`.devcontainer` selection
  (the first match wins).

## Try it

```sh
devcon -w examples/minimal up
devcon -w examples/minimal exec -- cat /tmp/devcon-created.txt
devcon -w examples/minimal down
```
