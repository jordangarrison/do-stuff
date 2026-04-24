package cli

import "regexp"

// taskSlugRe is the validation regex for a task slug. Shared across every
// command that accepts a <slug> arg so they all validate identically.
var taskSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
