package tui

import (
	"strings"
	"testing"
)

func TestTopicWidgetHeightUsesOneLineWhenTopicFits(t *testing.T) {
	if got := topicWidgetHeight("short topic", 30); got != topicSingleLineHeight {
		t.Fatalf("topicWidgetHeight(short) = %d, want %d", got, topicSingleLineHeight)
	}
}

func TestTopicWidgetHeightUsesTwoLinesWhenTopicWraps(t *testing.T) {
	// outer width 20 leaves 18 cells inside the border.
	topic := strings.Repeat("x", 19)
	if got := topicWidgetHeight(topic, 20); got != topicDoubleLineHeight {
		t.Fatalf("topicWidgetHeight(long) = %d, want %d", got, topicDoubleLineHeight)
	}
}

func TestTopicWidgetHeightNeverExceedsTwoLines(t *testing.T) {
	topic := strings.Repeat("very long topic ", 20)
	if got := topicWidgetHeight(topic, 20); got != topicDoubleLineHeight {
		t.Fatalf("topicWidgetHeight(very long) = %d, want %d", got, topicDoubleLineHeight)
	}
}
