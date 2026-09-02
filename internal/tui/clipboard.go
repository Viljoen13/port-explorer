package tui

import (
	"encoding/base64"
	"os"
	"strings"
)

// copyToClipboard uses the OSC 52 escape sequence, which modern terminals
// (kitty, alacritty, wezterm, foot, iTerm2, Windows Terminal, recent
// gnome-terminal) turn into a system clipboard write. It goes to stderr so it
// bypasses the renderer; the sequence is invisible so it never disturbs layout.
func copyToClipboard(text string) error {
	seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
	if os.Getenv("TMUX") != "" {
		// Wrap for tmux passthrough (requires `set -g allow-passthrough on`).
		seq = "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	_, err := os.Stderr.WriteString(seq)
	return err
}
