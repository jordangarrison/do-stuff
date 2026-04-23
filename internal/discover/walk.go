package discover

import (
	"os"
	"path/filepath"
)

// Repo describes a discovered git repository.
type Repo struct {
	Name string // discovery-assigned; collides deduped as "<parent-dir>/<name>"
	Path string // absolute path to the repo working tree
	Root string // configured root this repo was found under
}

// Walk scans each root to depth 2 looking for directories containing a .git
// entry. Flat layouts (`<root>/<repo>`) and grouped layouts
// (`<root>/<group>/<repo>`) are both discovered. Repos are returned in the
// order roots and subdirs are visited. Collisions on bare Name get namespaced
// via the immediate parent dir's basename (root basename for flat repos,
// group name for grouped repos).
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
			childPath := filepath.Join(root, entry.Name())
			if hasDotGit(childPath) {
				out = append(out, repoFromPath(childPath, root, seen))
				continue
			}
			// child is not itself a repo; check one level deeper so grouped
			// layouts like ~/dev/<org>/<repo> get discovered.
			subEntries, err := os.ReadDir(childPath)
			if err != nil {
				continue
			}
			for _, sub := range subEntries {
				if !sub.IsDir() {
					continue
				}
				subPath := filepath.Join(childPath, sub.Name())
				if hasDotGit(subPath) {
					out = append(out, repoFromPath(subPath, root, seen))
				}
			}
		}
	}
	return out, nil
}

// repoFromPath builds a Repo with dedup applied. First appearance of a bare name
// keeps it; later appearances get prefixed by the repo's immediate parent
// dir basename.
func repoFromPath(repoPath, root string, seen map[string]bool) Repo {
	bare := filepath.Base(repoPath)
	name := bare
	if seen[bare] {
		name = filepath.Base(filepath.Dir(repoPath)) + "/" + bare
	}
	seen[bare] = true
	return Repo{
		Name: name,
		Path: repoPath,
		Root: root,
	}
}

func hasDotGit(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
