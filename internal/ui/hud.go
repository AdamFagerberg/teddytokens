package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

type keyMap struct {
	Toggle, Replay, Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding  { return []key.Binding{k.Toggle, k.Replay, k.Quit} }
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var keys = keyMap{
	Toggle: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "skin")),
	Replay: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

var (
	accent    = lipgloss.Color("#D97757")
	hudStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A8A8A8"))
	numStyle  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	popStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD27F")).Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
	separator = hudStyle.Render(" · ")
)

// formatTokens abbreviates n as 999, 12.3k, 1.24M or 3.50B. Thresholds sit
// just below each unit so rounding never prints "1000.0k".
func formatTokens(n int64) string {
	f := float64(n)
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10)
	case n < 999_950:
		return fmt.Sprintf("%.1fk", f/1e3)
	case n < 999_995_000:
		return fmt.Sprintf("%.2fM", f/1e6)
	default:
		return fmt.Sprintf("%.2fB", f/1e9)
	}
}

// hud renders the one-line status bar, truncated to the terminal width.
func (m *Model) hud() string {
	chars := m.total / m.spawn.per
	parts := []string{
		numStyle.Render(formatTokens(m.total)) + hudStyle.Render(" tokens"),
		numStyle.Render(strconv.FormatInt(chars, 10)) + hudStyle.Render(" chars"),
		m.bar.ViewAs(m.spawn.progress()),
		hudStyle.Render(m.scope),
	}
	if shown := int(math.Round(m.shown)); shown > 0 {
		style := numStyle
		if shown != m.cleared || math.Abs(m.shownVel) > 0.05 {
			style = popStyle // still easing toward the new count
		}
		parts = append(parts, style.Render(fmt.Sprintf("×%d", shown)))
	}
	if m.err != nil {
		parts = append(parts, errStyle.Render("⚠ "+m.err.Error()))
	}
	left := " " + strings.Join(parts, separator) + separator
	m.help.SetWidth(max(0, m.width-lipgloss.Width(left)))
	return lipgloss.NewStyle().MaxWidth(m.width).Render(left + m.help.View(keys))
}
