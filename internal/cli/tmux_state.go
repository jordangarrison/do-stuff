package cli

import "github.com/jordangarrison/do-stuff/internal/tmux"

// probeSessionState reports the live state of a tmux session: "attached",
// "detached", or "absent". Absent covers both "tmux not on PATH" and
// "session does not exist" so callers don't have to fork on those paths.
func probeSessionState(session string) string {
	if err := tmux.Available(); err != nil {
		return "absent"
	}
	has, err := tmux.HasSession(session)
	if err != nil || !has {
		return "absent"
	}
	attached, err := tmux.IsSessionAttached(session)
	switch {
	case err != nil:
		return "detached"
	case attached:
		return "attached"
	default:
		return "detached"
	}
}
