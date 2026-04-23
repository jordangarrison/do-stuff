package discover

import (
	"os"
	"path/filepath"
)

// Repo describes a discovered git repository.
type Repo struct {
	Name string // discovery-assigned; collides deduped as "<root-basename>/<name>"
	Path string // absolute path to the repo working tree
	Root string // configured root this repo was found under
}

// Walk scans each root at depth 2 looking for directories containing a .git
// entry. Repos are returned in the order roots and subdirs are visited.
// Collisions on bare Name get namespaced via the root's basename.
func Walk(roots []string) ([]Repo, error) {
	var out []Repo
	seen := map[string]bool{}

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			// Missing/unreadable roots are silently skipped; config layer
			// should validate before calling Walk if strict behavior wanted.
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(root, entry.Name())
			if !hasDotGit(candidate) {
				continue
			}
			name := entry.Name()
			if seen[name] {
				name = filepath.Base(root) + "/" + entry.Name()
			}
			seen[entry.Name()] = true
			out = append(out, Repo{
				Name: name,
				Path: candidate,
				Root: root,
			})
		}
	}
	return out, nil
}

func hasDotGit(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
