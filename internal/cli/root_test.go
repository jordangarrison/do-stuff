package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRootCmd_versionFlagPrints(t *testing.T) {
	var stdout bytes.Buffer
	cmd := NewRootCmd("v1.2.3")
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), "v1.2.3") {
		t.Fatalf("stdout missing version: %q", stdout.String())
	}
}

func TestNewRootCmd_hasJSONAndHumanFlags(t *testing.T) {
	cmd := NewRootCmd("v0.0.0")
	if cmd.PersistentFlags().Lookup("json") == nil {
		t.Fatal("missing --json persistent flag")
	}
	if cmd.PersistentFlags().Lookup("human") == nil {
		t.Fatal("missing --human persistent flag")
	}
}

func TestNewRootCmd_hasReposSubcommand(t *testing.T) {
	cmd := NewRootCmd("v0.0.0")
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "repos" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("repos subcommand not registered")
	}
}

func TestNewRootCmd_hasNewSubcommand(t *testing.T) {
	cmd := NewRootCmd("v0.0.0")
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "new" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("new subcommand not registered")
	}
}

func TestNewRootCmd_hasListSubcommand(t *testing.T) {
	cmd := NewRootCmd("v0.0.0")
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("list subcommand not registered")
	}
}
