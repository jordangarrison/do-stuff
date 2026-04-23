package main

import "runtime/debug"

// version is overridden at build time via -ldflags "-X main.version=..."
// by Nix and GoReleaser. `go install` / `go build` leave it as the sentinel,
// at which point resolveVersion falls back to runtime/debug.ReadBuildInfo.
var version = "dev"

func resolveVersion() string {
	return resolveVersionFrom(version, func() (string, bool) {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return "", false
		}
		return bi.Main.Version, true
	})
}

func resolveVersionFrom(ldflag string, buildInfo func() (string, bool)) string {
	if ldflag != "dev" {
		return ldflag
	}
	v, ok := buildInfo()
	if !ok {
		return "dev"
	}
	if v == "" || v == "(devel)" {
		return "dev"
	}
	return v
}
