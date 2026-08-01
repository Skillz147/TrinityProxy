package proxy

import (
	"os"
	"runtime"
)

// UseEmbedded reports whether the agent should run the embedded Go SOCKS proxy
// instead of the Linux Dante installer path.
func UseEmbedded() bool {
	if os.Getenv("TRINITY_SKIP_INSTALLER") == "1" {
		return true
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	default:
		return false
	}
}
