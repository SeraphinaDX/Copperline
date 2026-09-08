package model

import "regexp"

// Consume IRC color parameters together with their control byte. Letting the
// terminal discard only the control byte leaves visible numbers such as 02.
// Preserve the original message for relaying/logging and normalize at display.
var ircFormatting = regexp.MustCompile("\\x03(?:[0-9]{1,2}(?:,[0-9]{1,2})?)?|\\x04(?:[0-9a-fA-F]{6}(?:,[0-9a-fA-F]{6})?)?|[\\x02\\x0f\\x11\\x16\\x1d\\x1e\\x1f]")

func PlainText(text string) string {
	return ircFormatting.ReplaceAllString(text, "")
}
