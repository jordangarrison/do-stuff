package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a config.yaml containing the given repo_roots and returns its path.
func writeConfig(t *testing.T, roots []string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	body := "repo_roots:\n"
	for _, r := range roots {
		body += "  - " + r + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewReposCmd_rejectsPositionalArgs(t *testing.T) {
	cmd := NewReposCmd(&GlobalFlags{})
	cmd.SetArgs([]string{"extra"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from positional arg, got nil")
	}
	// Cobra's NoArgs validator returns an error like `unknown command "extra" for "repos"`
	// or `accepts 0 arg(s), received 1`. Either shape is fine; we just need a non-nil error
	// so HandleExecuteError can render invalid_args.
}

func TestRepos_successGoldenEnvelope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, []string{root})

	var stdout, stderr bytes.Buffer
	code := runRepos(reposOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if code != 0 {
		t.Fatalf("want exit 0, got %d: stderr=%s", code, stderr.String())
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Command != "ds.repos" {
		t.Fatalf("bad envelope: %+v", env)
	}
	data, _ := env.Data.(map[string]any)
	if data == nil {
		t.Fatal("missing data")
	}
	repos, _ := data["repos"].([]any)
	if len(repos) != 1 {
		t.Fatalf("want 1 repo, got %d (data=%+v)", len(repos), data)
	}
}

func TestRepos_emptyRepoRootsReturnsConfigError(t *testing.T) {
	cfgPath := writeConfig(t, nil)

	var stdout, stderr bytes.Buffer
	code := runRepos(reposOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if code != 8 {
		t.Fatalf("want exit 8 (config_error), got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.OK || env.Error == nil {
		t.Fatalf("bad envelope: %+v", env)
	}
	if env.Error.Code != "config_error" {
		t.Fatalf("want config_error, got %s", env.Error.Code)
	}
}
