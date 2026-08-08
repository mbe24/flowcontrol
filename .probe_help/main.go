package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	up := key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move up"))
	dn := key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move down"))
	ent := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open detail"))
	binds := []key.Binding{up, dn, ent}

	h := help.New()
	h.Width = 30
	left := h.ShortHelpView(binds)
	fmt.Printf("Width=30 -> left sten=%d  plain=%q\n", wlen(left), strip(left))
}

func strip(s string) string { return lipless(s) }
func wlen(s string) int     { return lipgloss.Width(s) }
func lipless(s string) string { return s } // placeholder
