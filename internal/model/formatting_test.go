package model

import "testing"

func TestPlainTextConsumesFormattingNotLiteralDigits(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x0302hello\x0f", "hello"},
		{"\x0302,04hello\x03!", "hello!"},
		{"\x04aabbcc,001122hello\x0f", "hello"},
		{"\x02bold\x02 \x1dunder\x1d", "bold under"},
		{"02 hello, 04 [nick]", "02 hello, 04 [nick]"},
		{"\x03, literal comma", ", literal comma"},
		{"\x0302Ålice: hello", "Ålice: hello"},
	} {
		if got := PlainText(tc.input); got != tc.want {
			t.Errorf("PlainText(%q)=%q, want %q", tc.input, got, tc.want)
		}
	}
}
