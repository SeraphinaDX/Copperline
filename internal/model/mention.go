package model

import "strings"

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
