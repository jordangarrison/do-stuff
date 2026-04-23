# do-stuff

An opinionated toolkit for spinning up and resuming multi-repo work organized as tasks. Ships a `ds` CLI binary and a set of Claude skills that drive it.

Status: bootstrap (v0.0) — no commands yet.

## Install

### Nix (primary)

```
nix profile install github:jordangarrison/do-stuff
```

### Go

```
go install github.com/jordangarrison/do-stuff/cmd/ds@latest
```

Plain `go install` builds without the ldflags that Nix and GoReleaser use, so the resulting binary reports the baked-in placeholder version (`v0.0.0`) rather than the installed tag. BuildInfo-derived versioning lands in v0.1. Until then, prefer the Nix install or a tagged release archive for accurate version strings.

## Development

```
nix develop            # or `direnv allow` once
go build ./...
go test ./...
```

## License

MIT
