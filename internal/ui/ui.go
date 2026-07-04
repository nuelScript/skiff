// Package ui holds Skiff's styled terminal output.
package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	brand = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	green = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#16A34A"))
	red   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DC2626"))
	muted = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	label = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Width(9).Inline(true)
	link  = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("#7C3AED"))
)

func Banner(version string) {
	fmt.Println()
	fmt.Println("  " + brand.Render("Skiff") + " " + muted.Render("v"+version))
	fmt.Println()
}

func Accent(s string) string { return brand.Render(s) }

func Field(name, value string) {
	fmt.Println("  " + label.Render(name) + " " + value)
}

func Step(msg string) {
	fmt.Println("  " + muted.Render("→ "+msg))
}

func Done(msg string) {
	fmt.Println("  " + green.Render("✓") + " " + msg)
}

func Fail(msg string) {
	fmt.Println("  " + red.Render("✗") + " " + msg)
}

func Note(s string) {
	fmt.Println("  " + muted.Render(s))
}

func Live(url string, d time.Duration) {
	fmt.Println("  " + green.Render("✓ Live at") + " " + link.Render(url) +
		"  " + muted.Render(fmt.Sprintf("(%.1fs)", d.Seconds())))
}
