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

## Development

```
nix develop            # or `direnv allow` once
go build ./...
go test ./...
```

## License

MIT
