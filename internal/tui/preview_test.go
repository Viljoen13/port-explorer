package tui

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Viljoen13/port-explorer/internal/ports"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestPreview renders real screens to PREVIEW_OUT for eyeballing. Skipped
// unless the env var is set, so it never runs in CI.
func TestPreview(t *testing.T) {
	out := os.Getenv("PREVIEW_OUT")
	if out == "" {
		t.Skip("set PREVIEW_OUT to render previews")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	entries, err := ports.List()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{Refresh: 2 * time.Second})
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	model, _ = model.Update(refreshMsg{entries: ports.Merge(entries), at: time.Now()})
	write := func(name string, mm tea.Model) {
		if err := os.WriteFile(fmt.Sprintf("%s/%s.txt", out, name), []byte(mm.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("list", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	write("grouped", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	write("all", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	write("detail", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	write("confirm", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	write("help", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "udp" {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	write("filter", model)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	write("narrow", model)
}
