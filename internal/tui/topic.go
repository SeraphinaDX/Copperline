package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

const (
	topicSingleLineHeight = 3 // border + one content line
	topicDoubleLineHeight = 4 // border + two content lines
)

// topicWidgetHeight keeps the topic compact when it fits on one line, but
// gives it a second content line when wrapping is needed. The widget never
// grows beyond two lines, so an unusually long topic cannot push the message
// list farther down the screen.
func topicWidgetHeight(topic string, outerWidth int) int {
	innerWidth := outerWidth - 2 // Paragraph border consumes one column per side.
	if topic == "" || innerWidth <= 0 {
		return topicSingleLineHeight
	}

	for _, line := range strings.Split(topic, "\n") {
		if runewidth.StringWidth(line) > innerWidth {
			return topicDoubleLineHeight
		}
	}
	if strings.Contains(topic, "\n") {
		return topicDoubleLineHeight
	}
	return topicSingleLineHeight
}
