package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"copperline/internal/config"
	ircclient "copperline/internal/irc"
	"copperline/internal/logging"
	"copperline/internal/model"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

func TestFirstChannelVisitKeepsCurrentSessionMessagesLive(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "libera")
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(serverDir, "#later.log"),
		[]byte("2026-08-26 12:00:00 <alice> persisted context\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loggingEnabled := true
	backlogLines := 10
	cfg := &config.Config{General: config.GeneralConfig{
		Logging:         &loggingEnabled,
		LogDir:          dir,
		Timestamp:       "2006-01-02 15:04:05",
		HistoryLines:    100,
		LogBacklogLines: &backlogLines,
	}}
	logger := logging.New(true, dir, cfg.General.Timestamp)
	state := model.New(100)
	msg := model.Message{
		Time:   time.Date(2026, 8, 27, 20, 0, 0, 0, time.Local),
		Server: "libera",
		Target: "#later",
		Nick:   "carol",
		Text:   "current session before first view",
		Kind:   model.KindMessage,
	}
	state.Add(msg)
	state.Select("libera", "#later")
	if err := logger.Write(msg); err != nil {
		t.Fatal(err)
	}

	app := &App{
		cfg:        cfg,
		state:      state,
		logger:     logger,
		transcript: &transcriptList{List: widgets.NewList()},
	}
	if !app.loadConversationLogBacklog(state.CurrentInfo()) {
		t.Fatal("loadConversationLogBacklog returned false; expected persisted context")
	}
	if app.transcriptTotal != 0 || app.transcriptStart != 0 {
		t.Fatalf("first-view live cursor = start %d total %d, want 0/0", app.transcriptStart, app.transcriptTotal)
	}
	if app.transcriptBacklogRows != 1 {
		t.Fatalf("backlog rows = %d, want 1 pre-session row", app.transcriptBacklogRows)
	}

	window := state.CurrentWindow(app.transcriptTotal)
	if window == nil || len(window.Messages) != 1 || window.Messages[0].Text != msg.Text {
		t.Fatalf("current-session window = %#v, want the message received before first view", window)
	}
}

func TestFirstPrivateMessageVisitKeepsCurrentSessionMessagesLive(t *testing.T) {
	dir := t.TempDir()
	serverDir := filepath.Join(dir, "libera")
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(serverDir, "alice.log"),
		[]byte("2026-09-06 12:00:00 <alice> persisted private context\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	loggingEnabled := true
	backlogLines := 10
	cfg := &config.Config{General: config.GeneralConfig{
		Logging:         &loggingEnabled,
		LogDir:          dir,
		Timestamp:       "2006-01-02 15:04:05",
		HistoryLines:    100,
		LogBacklogLines: &backlogLines,
	}}
	logger := logging.New(true, dir, cfg.General.Timestamp)
	state := model.New(100)
	msg := model.Message{
		Time:   time.Date(2026, 9, 7, 0, 40, 0, 0, time.Local),
		Server: "libera",
		Target: "alice",
		Nick:   "alice",
		Text:   "current session before opening query",
		Kind:   model.KindMessage,
	}

	// Match App.onMessage ordering: pin the log boundary before exposing the PM
	// to state, then append it to the persistent log.
	if err := logger.BeginBuffer(msg.Server, msg.Target); err != nil {
		t.Fatal(err)
	}
	state.Add(msg)
	if err := logger.Write(msg); err != nil {
		t.Fatal(err)
	}
	state.Select("libera", "alice")

	app := &App{
		cfg:        cfg,
		state:      state,
		logger:     logger,
		transcript: &transcriptList{List: widgets.NewList()},
	}
	if !app.loadConversationLogBacklog(state.CurrentInfo()) {
		t.Fatal("loadConversationLogBacklog returned false for PM query")
	}
	if app.transcriptBacklogRows != 1 {
		t.Fatalf("PM backlog rows = %d, want 1 pre-session row", app.transcriptBacklogRows)
	}
	if app.transcriptTotal != 0 || app.transcriptStart != 0 {
		t.Fatalf("PM first-view live cursor = start %d total %d, want 0/0", app.transcriptStart, app.transcriptTotal)
	}

	window := state.CurrentWindow(app.transcriptTotal)
	if window == nil || len(window.Messages) != 1 || window.Messages[0].Text != msg.Text {
		t.Fatalf("PM current-session window = %#v, want delayed live PM", window)
	}

	// The persisted seed must be muted, but the current-session PM must be
	// formatted through the normal live-message path when rebuildCurrent appends
	// the window above it.
	if len(app.transcript.Rows) != 1 || app.transcript.Rows[0] == msg.Text {
		t.Fatalf("PM persisted seed = %#v, want one separate backlog row", app.transcript.Rows)
	}
}

func TestServerBufferDoesNotLoadPersistentConversationBacklog(t *testing.T) {
	loggingEnabled := true
	backlogLines := 10
	cfg := &config.Config{General: config.GeneralConfig{
		Logging:         &loggingEnabled,
		LogDir:          t.TempDir(),
		Timestamp:       "2006-01-02 15:04:05",
		LogBacklogLines: &backlogLines,
	}}
	app := &App{cfg: cfg, logger: logging.New(true, cfg.General.LogDir, cfg.General.Timestamp), transcript: &transcriptList{List: widgets.NewList()}}
	if app.loadConversationLogBacklog(&model.BufferInfo{Server: "libera", Target: "*server*"}) {
		t.Fatal("server buffer unexpectedly loaded conversational backlog")
	}
}

func TestScrollTranscriptBottomEmptyListIsSafe(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	app := &App{transcript: list}

	app.scrollTranscriptBottom()

	if list.SelectedRow != 0 {
		t.Fatalf("SelectedRow = %d, want 0 for empty transcript", list.SelectedRow)
	}
}

func TestScrollTranscriptBottomSelectsLastRow(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	list.Rows = []string{"one", "two", "three"}
	app := &App{transcript: list}

	app.scrollTranscriptBottom()

	if list.SelectedRow != 2 {
		t.Fatalf("SelectedRow = %d, want 2", list.SelectedRow)
	}
}

func TestTranscriptCachePreservesBacklogAndLiveRows(t *testing.T) {
	list := &transcriptList{List: widgets.NewList()}
	list.Rows = []string{
		"[old persisted line](fg:#77839a)",
		"live message one",
		"live message two",
	}
	app := &App{
		transcript:       list,
		transcriptCaches: make(map[string]transcriptCache),
	}

	key := "libera\x00#copperline"
	app.saveTranscriptCache(key, transcriptCache{
		rows:        list.Rows,
		start:       12,
		total:       14,
		fromLog:     true,
		backlogRows: 1,
	})

	// Mutating the current widget after saving must not mutate the cache.
	app.transcript.Rows[1] = "changed elsewhere"
	app.transcript.Rows = nil
	app.transcriptStart = 0
	app.transcriptTotal = 0
	app.transcriptFromLog = false
	app.transcriptBacklogRows = 0

	if !app.restoreTranscriptCache(key) {
		t.Fatal("restoreTranscriptCache returned false")
	}
	want := []string{
		"[old persisted line](fg:#77839a)",
		"live message one",
		"live message two",
	}
	if len(app.transcript.Rows) != len(want) {
		t.Fatalf("restored %d rows, want %d", len(app.transcript.Rows), len(want))
	}
	for i := range want {
		if app.transcript.Rows[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, app.transcript.Rows[i], want[i])
		}
	}
	if app.transcriptStart != 12 || app.transcriptTotal != 14 {
		t.Fatalf("restored window = start %d total %d, want 12/14", app.transcriptStart, app.transcriptTotal)
	}
	if !app.transcriptFromLog || app.transcriptBacklogRows != 1 {
		t.Fatalf("restored backlog metadata = fromLog %v rows %d, want true/1", app.transcriptFromLog, app.transcriptBacklogRows)
	}
}

func TestKeyBindingMatchesRuneFallbackForModernCtrlEvents(t *testing.T) {
	if !keyBindingMatchesRuneFallback("Ctrl+Y", "y", true, false) {
		t.Fatal("Ctrl+Y did not match KeyRune y carrying Ctrl modifier")
	}
	if !keyBindingMatchesRuneFallback("Ctrl+Y", string(rune(25)), true, false) {
		t.Fatal("Ctrl+Y did not match ASCII Ctrl+Y control byte fallback")
	}
	if keyBindingMatchesRuneFallback("Alt+Y", "y", true, false) {
		t.Fatal("Ctrl rune incorrectly matched Alt+Y")
	}
	if !keyBindingMatchesRuneFallback("Alt+N", "n", false, true) {
		t.Fatal("Alt+N did not match KeyRune n carrying Alt modifier")
	}
}

func TestUserListScrollKeyDeltaConfigurable(t *testing.T) {
	keys := config.KeybindingsConfig{UserListDown: "Alt+N", UserListUp: "Alt+P"}
	tests := []struct {
		name string
		id   string
		want int
	}{
		{name: "alt n meta form", id: "<M-n>", want: 1},
		{name: "alt p meta form", id: "<M-p>", want: -1},
		{name: "alt n alternate form", id: "<A-n>", want: 1},
		{name: "alt p long form", id: "<Alt-p>", want: -1},
		{name: "plain ctrl n remains buffer navigation", id: "<C-n>", want: 0},
		{name: "plain ctrl p remains buffer navigation", id: "<C-p>", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userListScrollKeyDelta(tt.id, keys)
			if got != tt.want {
				t.Fatalf("userListScrollKeyDelta(%q) = %d, want %d", tt.id, got, tt.want)
			}
		})
	}

	keys.UserListDown = "Ctrl+J"
	if got := userListScrollKeyDelta("<C-j>", keys); got != 1 {
		t.Fatalf("custom Ctrl+J user-list binding = %d, want 1", got)
	}
	if got := userListScrollKeyDelta("<M-n>", keys); got != 0 {
		t.Fatalf("old Alt+N binding still active after rebind: %d", got)
	}
}

func TestClampUserScroll(t *testing.T) {
	tests := []struct {
		scroll  int
		total   int
		visible int
		want    int
	}{
		{scroll: -3, total: 50, visible: 10, want: 0},
		{scroll: 5, total: 50, visible: 10, want: 5},
		{scroll: 99, total: 50, visible: 10, want: 40},
		{scroll: 7, total: 5, visible: 10, want: 0},
		{scroll: 7, total: 50, visible: 0, want: 0},
	}
	for _, tt := range tests {
		if got := clampUserScroll(tt.scroll, tt.total, tt.visible); got != tt.want {
			t.Fatalf("clampUserScroll(%d, %d, %d) = %d, want %d", tt.scroll, tt.total, tt.visible, got, tt.want)
		}
	}
}

func TestUserNickAtVisibleRowUsesScrollOffset(t *testing.T) {
	users := widgets.NewList()
	users.Rows = []string{"Carol", "Dave"}
	app := &App{
		users:      users,
		userNicks:  []string{"Alice", "Bob", "Carol", "Dave"},
		userScroll: 2,
	}

	nick, ok := app.userNickAtVisibleRow(1)
	if !ok {
		t.Fatal("userNickAtVisibleRow returned !ok")
	}
	if nick != "Dave" {
		t.Fatalf("nick = %q, want Dave", nick)
	}
}

func newInputHistoryTestApp() *App {
	state := model.New(100)
	state.Ensure("test", "#one")
	return &App{
		state:        state,
		input:        widgets.NewInput(),
		inputHistory: make(map[string]*inputHistoryState),
	}
}

func TestInputHistoryRecallsMessagesCommandsAndDraft(t *testing.T) {
	app := newInputHistoryTestApp()
	app.addInputHistory("hello channel")
	app.addInputHistory("/whois Alice")
	app.input.Text = "unfinished draft"

	app.inputHistoryPrevious()
	if app.input.Text != "/whois Alice" {
		t.Fatalf("first previous = %q, want newest command", app.input.Text)
	}
	app.inputHistoryPrevious()
	if app.input.Text != "hello channel" {
		t.Fatalf("second previous = %q, want earlier message", app.input.Text)
	}
	app.inputHistoryNext()
	if app.input.Text != "/whois Alice" {
		t.Fatalf("first next = %q, want newer command", app.input.Text)
	}
	app.inputHistoryNext()
	if app.input.Text != "unfinished draft" {
		t.Fatalf("next past newest = %q, want saved draft", app.input.Text)
	}
}

func TestInputHistoryIsPerBuffer(t *testing.T) {
	app := newInputHistoryTestApp()
	app.addInputHistory("public message")

	app.state.Ensure("test", "Alice")
	app.state.Select("test", "Alice")
	app.addInputHistory("private message")
	app.input.Text = ""
	app.inputHistoryPrevious()
	if app.input.Text != "private message" {
		t.Fatalf("query history = %q, want private message", app.input.Text)
	}

	app.state.Select("test", "#one")
	app.resetInputHistoryNavigation()
	app.input.Text = ""
	app.inputHistoryPrevious()
	if app.input.Text != "public message" {
		t.Fatalf("channel history = %q, want public message", app.input.Text)
	}
}

func TestInputHistorySkipsConsecutiveDuplicates(t *testing.T) {
	app := newInputHistoryTestApp()
	app.addInputHistory("same")
	app.addInputHistory("same")
	h := app.inputHistoryCurrent()
	if h == nil || len(h.entries) != 1 {
		t.Fatalf("history entries = %#v, want one consecutive duplicate", h)
	}
}

func TestInputHistoryDefaultsToTenEntriesPerBuffer(t *testing.T) {
	app := newInputHistoryTestApp()
	for i := 0; i < 15; i++ {
		app.addInputHistory(fmt.Sprintf("line %d", i))
	}
	h := app.inputHistoryCurrent()
	if h == nil || len(h.entries) != 10 {
		t.Fatalf("history entries = %#v, want 10 entries", h)
	}
	if h.entries[0] != "line 5" || h.entries[9] != "line 14" {
		t.Fatalf("history range = %#v, want lines 5..14", h.entries)
	}
}

func TestInputHistoryConfiguredLimit(t *testing.T) {
	limit := 3
	app := newInputHistoryTestApp()
	app.cfg = &config.Config{General: config.GeneralConfig{InputHistoryLimit: &limit}}
	for i := 0; i < 5; i++ {
		app.addInputHistory(fmt.Sprintf("line %d", i))
	}
	h := app.inputHistoryCurrent()
	if h == nil || len(h.entries) != 3 {
		t.Fatalf("history entries = %#v, want 3 entries", h)
	}
	if h.entries[0] != "line 2" || h.entries[2] != "line 4" {
		t.Fatalf("history range = %#v, want lines 2..4", h.entries)
	}
}

func TestInputHistoryCanBeDisabled(t *testing.T) {
	limit := 0
	app := newInputHistoryTestApp()
	app.cfg = &config.Config{General: config.GeneralConfig{InputHistoryLimit: &limit}}
	app.addInputHistory("not stored")
	h := app.inputHistoryCurrent()
	if h == nil {
		t.Fatal("inputHistoryCurrent returned nil")
	}
	if len(h.entries) != 0 {
		t.Fatalf("history entries = %#v, want none", h.entries)
	}
}

func TestPrintableInputTextAllowsLiteralLessThan(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "literal less than", id: "<", want: "<"},
		{name: "literal greater than", id: ">", want: ">"},
		{name: "ordinary text", id: "x", want: "x"},
		{name: "special key", id: "<Enter>", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := printableInputText(ui.Event{ID: tt.id}); got != tt.want {
				t.Fatalf("printableInputText(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestStartupProgressAllReady(t *testing.T) {
	if !(startupProgress{}).allReady() {
		t.Fatal("empty startup progress should be ready")
	}
	if (startupProgress{connectedServers: 1, totalServers: 2}).allReady() {
		t.Fatal("startup with a disconnected server reported ready")
	}
	if (startupProgress{connectedServers: 1, totalServers: 1, joinedChannels: 1, totalChannels: 2}).allReady() {
		t.Fatal("startup with an unjoined channel reported ready")
	}
	if !(startupProgress{connectedServers: 2, totalServers: 2, joinedChannels: 3, totalChannels: 3}).allReady() {
		t.Fatal("fully connected startup did not report ready")
	}
}

func TestInputDisplayStateShowsConnectingBeforeChatIsReady(t *testing.T) {
	cfg := &config.Config{
		General: config.GeneralConfig{HistoryLines: 10},
		Servers: []config.ServerConfig{{
			Name:        "testnet",
			Host:        "irc.example.invalid",
			Port:        6697,
			TLS:         true,
			Nick:        "tester",
			User:        "tester",
			AutoConnect: true,
			Channels:    []string{"#one"},
		}},
	}
	app := New(cfg)
	title, placeholder := app.inputDisplayState(&model.BufferInfo{Server: "testnet", Target: "#one"})
	if title != "Connecting - please wait" {
		t.Fatalf("title = %q, want connecting state", title)
	}
	if placeholder != "Waiting for testnet..." {
		t.Fatalf("placeholder = %q, want server wait message", placeholder)
	}
	if reason := app.chatWaitReason("testnet", "#one"); reason == "" {
		t.Fatal("chatWaitReason allowed chat before connection")
	}
}

func TestResetUIAfterScriptReloadForcesCleanTranscriptWidget(t *testing.T) {
	app := &App{redraw: make(chan struct{}, 1)}
	if app.transcriptReset {
		t.Fatal("transcriptReset unexpectedly true before reset")
	}
	if app.forceScreenSync {
		t.Fatal("forceScreenSync unexpectedly true before reset")
	}

	// No gotui screen is initialized in this unit test; the helper must still
	// mark both halves of the repair. The next real render rebuilds the List and
	// then performs a physical tcell Sync so stale terminal cells cannot linger.
	app.resetUIAfterScriptReload()
	if !app.transcriptReset {
		t.Fatal("script reload did not request a clean transcript rebuild")
	}
	if !app.forceScreenSync {
		t.Fatal("script reload did not request a physical screen resync")
	}
	select {
	case <-app.redraw:
	default:
		t.Fatal("script reload did not request a redraw")
	}
}

type failingSendBackend struct {
	ircclient.Backend
}

func (f *failingSendBackend) IsConnected(string) bool      { return true }
func (f *failingSendBackend) WantsConnection(string) bool  { return true }
func (f *failingSendBackend) IsJoined(string, string) bool { return true }
func (f *failingSendBackend) SendMessage(string, string, string) error {
	return errors.New("relay unavailable")
}
func (f *failingSendBackend) SendAction(string, string, string) error {
	return errors.New("relay unavailable")
}
func (f *failingSendBackend) Notice(string, string, string) error {
	return errors.New("relay unavailable")
}
func (f *failingSendBackend) SendTyping(string, string, string) (bool, error) {
	return false, nil
}

func TestFailedChatSendKeepsInputText(t *testing.T) {
	for _, line := range []string{"do not lose me", "/msg alice do not lose me", "/me waves", "/notice alice do not lose me"} {
		t.Run(line, func(t *testing.T) {
			cfg := &config.Config{Relay: config.RelayConfig{Mode: "client"}}
			state := model.New(100)
			state.Select("testnet", "#chat")
			input := widgets.NewInput()
			input.Text = line
			input.Cursor = len([]rune(input.Text))
			app := &App{
				cfg:          cfg,
				state:        state,
				irc:          &failingSendBackend{},
				logger:       logging.New(false, "", "2006-01-02 15:04:05"),
				redraw:       make(chan struct{}, 1),
				input:        input,
				transcript:   &transcriptList{List: widgets.NewList()},
				inputHistory: make(map[string]*inputHistoryState),
			}

			app.handleKey(ui.Event{Type: ui.KeyboardEvent, ID: "<Enter>"})
			if app.input.Text != line {
				t.Fatalf("failed send cleared input: got %q", app.input.Text)
			}
		})
	}

}

func TestRelayReconnectLabelAdvertisesConfiguredKey(t *testing.T) {
	cfg := &config.Config{
		Relay:       config.RelayConfig{Mode: "client"},
		Keybindings: config.KeybindingsConfig{RelayReconnect: "Alt+R"},
	}
	app := &App{
		cfg:            cfg,
		relayReconnect: func() (ircclient.Backend, error) { return nil, nil },
	}
	if got, want := app.relayReconnectLabel(), "[⟳ RECONNECT (Alt+R)]"; got != want {
		t.Fatalf("relay reconnect label = %q, want %q", got, want)
	}
	if got, want := app.relayReconnectControlWidth(), len([]rune("[⟳ RECONNECT (Alt+R)]"))+2; got != want {
		t.Fatalf("relay reconnect control width = %d, want %d", got, want)
	}
}

type knownTargetsBackend struct {
	ircclient.Backend
	servers map[string][]string
}

func (b *knownTargetsBackend) ServerNames() []string {
	out := make([]string, 0, len(b.servers))
	for server := range b.servers {
		out = append(out, server)
	}
	return out
}
func (b *knownTargetsBackend) KnownTargets(server string) []string {
	return append([]string(nil), b.servers[server]...)
}
func (b *knownTargetsBackend) CurrentNick(string) string { return "britney" }

func TestClosedBufferIsNotResurrectedByBackendSnapshot(t *testing.T) {
	state := model.New(100)
	state.Ensure("testnet", "*server*")
	state.Select("testnet", "#later")
	backend := &knownTargetsBackend{servers: map[string][]string{"testnet": {"#later"}}}
	app := &App{
		state:         state,
		irc:           backend,
		closedBuffers: make(map[string]bool),
		redraw:        make(chan struct{}, 1),
	}

	app.closeBufferLocally("testnet", "#later")
	state.Close("testnet", "#later")
	app.onBackendUpdate()

	if got := state.Find("testnet", "#later"); got != nil {
		t.Fatalf("backend snapshot resurrected closed channel: %#v", got)
	}
}

func TestLiveMessageReopensLocallyClosedBuffer(t *testing.T) {
	state := model.New(100)
	state.Ensure("testnet", "*server*")
	backend := &knownTargetsBackend{servers: map[string][]string{"testnet": {"#later"}}}
	app := &App{
		cfg:           &config.Config{},
		state:         state,
		irc:           backend,
		logger:        logging.New(false, "", "2006-01-02 15:04:05"),
		closedBuffers: make(map[string]bool),
		redraw:        make(chan struct{}, 1),
	}

	app.closeBufferLocally("testnet", "#later")
	app.onMessage(model.Message{
		Time:   time.Now(),
		Server: "testnet",
		Target: "#later",
		Nick:   "alice",
		Text:   "new traffic",
		Kind:   model.KindMessage,
	})

	if app.isBufferClosed("testnet", "#later") {
		t.Fatal("live message did not clear local closed-buffer tombstone")
	}
	if got := state.Find("testnet", "#later"); got == nil || len(got.Messages) != 1 || got.Messages[0].Text != "new traffic" {
		t.Fatalf("live message did not reopen channel correctly: %#v", got)
	}
}
