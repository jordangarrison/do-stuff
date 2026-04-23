# do-stuff

An opinionated toolkit for spinning up and resuming multi-repo work organized as tasks. Ships a `ds` CLI binary and a set of Claude skills that drive it.

Status: v0.1 slice 2a — `ds repos`, `ds new`, `ds list` wired. `ds pick` + `ds attach` land in slice 2b.

Full design: [`SPEC.md`](SPEC.md).

## Install

### Nix (primary)

```
nix profile install github:jordangarrison/do-stuff
```

### Go

```
go install github.com/jordangarrison/do-stuff/cmd/ds@latest
```

## Configure

`ds` reads YAML from `$XDG_CONFIG_HOME/do-stuff/config.yaml` (default `~/.config/do-stuff/config.yaml`):

```yaml
# Paths can use ~ or $HOME; expanded at load time.
tasks_dir: ~/.do-stuff
repo_roots:
  - ~/dev/work
  - ~/dev/personal
tmux_prefix: task-
default_base: main
default_type: feat
start_tmux: true
```

`repo_roots` is required — everything else has defaults. `ds repos` errors cleanly with `code: config_error` until it's set.

## Development

```
nix develop            # or `direnv allow` once
go build ./...
go test ./...
```

## License

MIT
