package model

import (
	"strings"
	"unicode"
)

// Highlight matches literal words/phrases as well as the user's IRC nickname.
// Share this decision between the relay and TUI so styling and alerts agree.
func Highlight(msg Message, self string, words []string) bool {
	if self == "" || msg.Nick == "" || strings.EqualFold(msg.Nick, self) ||
		(msg.Kind != KindMessage && msg.Kind != KindAction) {
		return false
	}
	text := PlainText(msg.Text)
	if ContainsNickMention(text, self) {
		return true
	}
	runes := []rune(strings.ToLower(text))
	for _, word := range words {
		needle := []rune(strings.ToLower(strings.TrimSpace(word)))
		if len(needle) == 0 {
			continue
		}
		for i := 0; i+len(needle) <= len(runes); i++ {
			end := i + len(needle)
			if (i == 0 || !highlightWordRune(runes[i-1])) &&
				(end == len(runes) || !highlightWordRune(runes[end])) &&
				string(runes[i:end]) == string(needle) {
				return true
			}
		}
	}
	return false
}

func highlightWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || isIRCNickRune(r)
}

// ContainsNickMention reports whether text contains nick as an IRC nickname
// token rather than as a substring of a longer nickname. Matching is
// case-insensitive and permits ordinary punctuation around the nickname.
func ContainsNickMention(text, nick string) bool {
	textRunes := []rune(strings.ToLower(text))
	nickRunes := []rune(strings.ToLower(nick))
	if len(nickRunes) == 0 || len(textRunes) < len(nickRunes) {
		return false
	}

	for i := 0; i+len(nickRunes) <= len(textRunes); i++ {
		match := true
		for j := range nickRunes {
			if textRunes[i+j] != nickRunes[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if i > 0 && isIRCNickRune(textRunes[i-1]) {
			continue
		}
		end := i + len(nickRunes)
		if end < len(textRunes) && isIRCNickRune(textRunes[end]) {
			continue
		}
		return true
	}
	return false
}

func isIRCNickRune(r rune) bool {
	if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '-', '_', '[', ']', '\\', '`', '^', '{', '}', '|':
		return true
	default:
		return false
	}
}
