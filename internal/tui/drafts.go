package tui

import "copperline/internal/model"

type inputDraft struct {
	text    string
	cursor  int
	literal bool
}

// Drafts belong to this TUI session, so an SSH reconnect cannot discard them.
// Switch before processing more input, rather than waiting for a redraw.
func (a *App) syncInputDraft() {
	b := a.state.CurrentInfo()
	key := ""
	if b != nil {
		key = model.Key(b.Server, b.Target)
	}
	if a.inputKey == key || a.input == nil {
		return
	}
	if a.inputDrafts == nil {
		a.inputDrafts = make(map[string]inputDraft)
	}
	if a.inputKey != "" {
		a.inputDrafts[a.inputKey] = inputDraft{a.input.Text, a.input.Cursor, a.pasteLiteral}
		d := a.inputDrafts[key]
		a.input.Text, a.input.Cursor, a.pasteLiteral = d.text, d.cursor, d.literal
	}
	a.inputKey = key
	a.resetNickCompletion()
}

func (a *App) selectBuffer(server, target string) {
	a.syncInputDraft()
	a.state.Select(server, target)
	a.syncInputDraft()
}

func (a *App) selectBufferKey(key string) bool {
	a.syncInputDraft()
	ok := a.state.SelectKey(key)
	a.syncInputDraft()
	return ok
}
