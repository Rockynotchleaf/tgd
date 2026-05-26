// Package app implements the tgd bubbletea application.
package app

import tea "github.com/charmbracelet/bubbletea"

// keyIs returns true if the KeyMsg matches the given rune string (e.g. "j", "q")
// or bubbletea key type.
func keyIs(msg tea.KeyMsg, keys ...string) bool {
	str := msg.String()
	for _, k := range keys {
		if str == k {
			return true
		}
	}
	return false
}
