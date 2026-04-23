package main

import "testing"

func TestResolveVersion_ldflagWins(t *testing.T) {
	got := resolveVersionFrom("v1.2.3", func() (string, bool) { return "v9.9.9", true })
	if got != "v1.2.3" {
		t.Fatalf("want v1.2.3, got %s", got)
	}
}

func TestResolveVersion_sentinelFallsBackToBuildInfo(t *testing.T) {
	got := resolveVersionFrom("dev", func() (string, bool) { return "v2.0.0", true })
	if got != "v2.0.0" {
		t.Fatalf("want v2.0.0, got %s", got)
	}
}

func TestResolveVersion_develBuildInfoIgnored(t *testing.T) {
	got := resolveVersionFrom("dev", func() (string, bool) { return "(devel)", true })
	if got != "dev" {
		t.Fatalf("want dev, got %s", got)
	}
}

func TestResolveVersion_emptyBuildInfoStaysDev(t *testing.T) {
	got := resolveVersionFrom("dev", func() (string, bool) { return "", false })
	if got != "dev" {
		t.Fatalf("want dev, got %s", got)
	}
}
