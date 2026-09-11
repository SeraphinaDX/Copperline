package tui

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"copperline/internal/model"
	"github.com/gdamore/tcell/v3"
	"github.com/metaspartan/gotui/v5/widgets"
)

// gotui 5.0.3 discards EventPaste. Adapt its input queue while retaining its
// normal key/mouse conversion and the original screen for rendering.
const pasteKey tcell.Key = 30000
const pasteTooLargeKey tcell.Key = 30001
const maxPasteBytes = 1 << 20

type pasteScreen struct {
	tcell.Screen
	events chan tcell.Event
}

func (s *pasteScreen) EventQ() chan tcell.Event { return s.events }

func pasteEvents(ctx context.Context, source chan tcell.Event) chan tcell.Event {
	out := make(chan tcell.Event)
	go func() {
		defer close(out)
		var text strings.Builder
		active, overflow := false, false
		emit := func(e tcell.Event) bool {
			select {
			case out <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		appendText := func(s string) {
			if overflow {
				return
			}
			if text.Len()+len(s) > maxPasteBytes {
				overflow = true
				text.Reset()
				return
			}
			text.WriteString(s)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-source:
				if !ok {
					return
				}
				if p, ok := e.(*tcell.EventPaste); ok {
					if p.Start() {
						active = true
						overflow = false
						text.Reset()
						continue
					}
					if active {
						active = false
						key := pasteKey
						if overflow {
							key = pasteTooLargeKey
						}
						if !emit(tcell.NewEventKey(key, text.String(), tcell.ModNone)) {
							return
						}
						text.Reset()
					}
					continue
				}
				if active {
					if k, ok := e.(*tcell.EventKey); ok {
						switch k.Key() {
						case tcell.KeyRune:
							appendText(k.Str())
						case tcell.KeyEnter:
							appendText("\n")
						case tcell.KeyCtrlJ, tcell.Key(10):
							appendText("\n")
						case tcell.KeyTab:
							appendText("\t")
						}
						continue
					}
				}
				if !emit(e) {
					return
				}
			}
		}
	}()
	return out
}

func (a *App) insertPaste(text string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	// Insert at the cursor once, instead of repeatedly reallocating the draft.
	before := []rune(a.input.Text)
	cursor := min(max(a.input.Cursor, 0), len(before))
	a.input.Text = string(before[:cursor]) + text + string(before[cursor:])
	a.input.Cursor = cursor + utf8.RuneCountInString(text)
	a.pasteLiteral = a.pasteLiteral || strings.Contains(text, "\n") || len(a.input.Text) > 240
	a.typingLastEdit = time.Now()
	a.resetNickCompletion()
	a.resetInputHistoryNavigation()
}

// Keep chunks small enough that normal IRC flood throttling fits within a
// single relay request. The backend remains responsible for IRC rate limits.
func pasteMessages(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		for len(line) > 240 {
			end := 240
			for !utf8.RuneStart(line[end]) {
				end--
			}
			lines = append(lines, line[:end])
			line = line[end:]
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

type pasteSendResult struct {
	batch     *pasteSend
	confirmed int
	err       error
	done      bool
}
type pasteSend struct {
	server, target string
	lines          []string
	confirmed      int
	remaining      []string
	active         bool
	cancel         context.CancelFunc
}

func (a *App) startPasteSend() {
	b := a.state.CurrentInfo()
	if b == nil {
		return
	}
	if a.pasteSend != nil {
		a.local(b.Server, b.Target, model.KindSystem, "A paste is already pending. Use /paste status, /paste cancel, or /paste discard.")
		return
	}
	if reason := a.chatWaitReason(b.Server, b.Target); reason != "" {
		a.local(b.Server, b.Target, model.KindSystem, reason)
		return
	}
	lines := pasteMessages(a.input.Text)
	if len(lines) == 0 {
		return
	}
	if a.pasteResults == nil {
		a.pasteResults = make(chan pasteSendResult, 1)
	}
	batch := &pasteSend{server: b.Server, target: b.Target, lines: lines, active: true}
	a.pasteSend = batch
	a.addInputHistory(a.input.Text)
	a.input.Text = ""
	a.input.Cursor = 0
	a.pasteLiteral = false
	a.follow = true
	a.scrollTranscriptBottom()
	a.runPasteSend(batch)
}

func (a *App) runPasteSend(batch *pasteSend) {
	ctx, cancel := context.WithCancel(context.Background())
	batch.cancel = cancel
	backend := a.irc
	results := a.pasteResults
	lines := append([]string(nil), batch.lines...)
	go func() {
		emit := func(r pasteSendResult) {
			// Only the UI mutates batch state. Coalesce progress to one pending event;
			// the final result always replaces older progress without blocking sends.
			select {
			case results <- r:
			default:
				select {
				case <-results:
				default:
				}
				results <- r
			}
			a.requestRedraw()
		}
		for i, line := range lines {
			if err := ctx.Err(); err != nil {
				emit(pasteSendResult{batch: batch, confirmed: i, err: err, done: true})
				return
			}
			if err := backend.SendMessage(batch.server, batch.target, line); err != nil {
				emit(pasteSendResult{batch: batch, confirmed: i, err: err, done: true})
				return
			}
			emit(pasteSendResult{batch: batch, confirmed: i + 1})
		}
		emit(pasteSendResult{batch: batch, confirmed: len(lines), done: true})
	}()
}

func (a *App) finishPasteSend(r pasteSendResult) {
	if a.pasteSend != r.batch {
		return
	}
	batch := a.pasteSend
	batch.confirmed = r.confirmed
	if !r.done {
		return
	}
	batch.cancel()
	batch.active = false
	if r.err != nil {
		batch.remaining = append([]string(nil), batch.lines[r.confirmed:]...)
		a.local(batch.server, batch.target, model.KindError, "Paste stopped: "+r.err.Error()+". Remaining text retained; /paste restore returns it to an empty input. Check the last line before retrying: delivery may be uncertain.")
		return
	}
	a.local(batch.server, batch.target, model.KindSystem, "Paste sent.")
	a.pasteSend = nil
}

func (a *App) inputForDisplay() *widgets.Input {
	if !strings.Contains(a.input.Text, "\n") {
		return a.input
	}
	preview := widgets.NewInput()
	a.theme.applyInput(preview)
	preview.SetRect(a.input.Min.X, a.input.Min.Y, a.input.Max.X, a.input.Max.Y)
	preview.Title = a.input.Title
	preview.Text = strings.ReplaceAll(a.input.Text, "\n", "↵")
	preview.Cursor = a.input.Cursor
	return preview
}
