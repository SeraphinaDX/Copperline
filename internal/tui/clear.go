package tui

import "copperline/internal/model"

func (a *App) clearCurrent() {
	a.rebuildCurrent()
	b := a.state.CurrentInfo()
	if b == nil {
		return
	}
	// Keep model history for relay replay de-duplication. Only the rendered
	// window moves forward; logs and other attached clients are unaffected.
	a.transcript = a.newTranscriptList()
	a.transcriptStart, a.transcriptTotal = b.Total, b.Total
	a.transcriptFromLog = true // prevent a revisit from reloading older rows
	a.transcriptBacklogRows = 0
	a.search = searchState{}
	a.follow = true
	delete(a.transcriptCaches, model.Key(b.Server, b.Target))
	a.state.MarkReadThrough(b.Server, b.Target, b.Total)
}
