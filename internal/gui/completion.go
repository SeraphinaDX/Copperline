package gui

import (
	"strings"

	"copperline/internal/model"
)

// completeNick mirrors the TUI's IRC-style nickname completion in the desktop
// composer. At the beginning of a message it produces "Nick: "; elsewhere it
// replaces only the nickname fragment. Repeated Tab presses cycle matches.
func (g *App) completeNick() bool {
	if g.mobile {
		g.resetNickCompletion()
		return false
	}
	b := g.state.CurrentInfo()
	if b == nil || !model.IsChannel(b.Target) {
		g.resetNickCompletion()
		return false
	}

	text := []rune(g.entry.Text)
	cursor := g.entry.CursorTextOffset()
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(text) {
		cursor = len(text)
	}

	// Once a completion has started, later Tab presses cycle the same match set.
	if len(g.nickCompletionMatches) > 0 &&
		g.nickCompletionStart >= 0 &&
		g.nickCompletionEnd >= g.nickCompletionStart &&
		g.nickCompletionEnd <= len(text) {
		g.nickCompletionIndex = (g.nickCompletionIndex + 1) % len(g.nickCompletionMatches)
		g.applyNickCompletion(text, g.nickCompletionMatches[g.nickCompletionIndex])
		return true
	}

	start := cursor
	for start > 0 && !isNickCompletionSeparator(text[start-1]) {
		start--
	}
	if start == cursor {
		return false
	}

	fragment := string(text[start:cursor])
	if fragment == "" {
		return false
	}

	end := cursor
	for end < len(text) && !isNickCompletionSeparator(text[end]) {
		end++
	}

	// Prefixes such as @ and + are display metadata in the GUI nick list.
	// Accept one if it was typed, but complete to the nickname itself.
	fragmentRunes := []rune(fragment)
	if len(fragmentRunes) > 1 && strings.ContainsRune("~&@%+", fragmentRunes[0]) {
		fragment = string(fragmentRunes[1:])
		start++
	}
	if fragment == "" {
		return false
	}

	needle := strings.ToLower(fragment)
	matches := make([]string, 0, len(g.users))
	for _, displayed := range g.users {
		nick := strings.TrimLeft(displayed, "~&@%+")
		if strings.HasPrefix(strings.ToLower(nick), needle) {
			matches = append(matches, nick)
		}
	}
	if len(matches) == 0 {
		g.resetNickCompletion()
		return false
	}

	firstWord := start == 0
	if firstWord && end < len(text) && text[end] == ' ' {
		end++
	}

	g.nickCompletionMatches = matches
	g.nickCompletionIndex = 0
	g.nickCompletionStart = start
	g.nickCompletionEnd = end
	g.nickCompletionFirst = firstWord
	g.applyNickCompletion(text, matches[0])
	return true
}

func (g *App) applyNickCompletion(text []rune, nick string) {
	start, end := g.nickCompletionStart, g.nickCompletionEnd
	if start < 0 || end < start || end > len(text) {
		g.resetNickCompletion()
		return
	}

	replacement := nick
	if g.nickCompletionFirst {
		replacement += ": "
	}
	repl := []rune(replacement)
	updated := make([]rune, 0, len(text)-(end-start)+len(repl))
	updated = append(updated, text[:start]...)
	updated = append(updated, repl...)
	updated = append(updated, text[end:]...)

	// SetText invokes OnChanged. Keep the completion state alive while cycling.
	g.nickCompletionApplying = true
	g.entry.SetText(string(updated))
	g.nickCompletionApplying = false

	g.nickCompletionEnd = start + len(repl)
	g.entry.CursorRow = 0
	g.entry.CursorColumn = g.nickCompletionEnd
	g.entry.Refresh()
}

func (g *App) resetNickCompletion() {
	g.nickCompletionMatches = nil
	g.nickCompletionIndex = 0
	g.nickCompletionStart = -1
	g.nickCompletionEnd = -1
	g.nickCompletionFirst = false
}

func isNickCompletionSeparator(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
