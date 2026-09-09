# Copperline

**Copperline is a modern terminal IRC client and native SSH relay/bouncer written in Go.** It is built for people who still love IRC, but do not want to give up a fast native application, modern IRCv3 features, rich terminal UI, scripting, notifications, persistent sessions, or strong customization.

Copperline is not a toy IRC example and it is not just a thin wrapper around an IRC library. It is an everyday multi-network client with its own buffer model, responsive TUI, persistent logging, DCC, Gotify notifications, Lua scripting, configurable keyboard and mouse controls, true-color themes, IRCv3 support, and an **embedded SSH relay system that can keep your IRC sessions alive when your local client is gone**.

One Copperline installation can connect directly to IRC, run headlessly as a persistent relay, or act as a local TUI attached to a remote Copperline relay. The relay transport is built directly into Copperline using Go's SSH libraries: **no system `ssh`, no `sshd`, no web service, no TLS certificate, and no certificate authority are required.**

![Copperline main interface](screenshots/screenshot1.avif)

![Copperline themes](screenshots/screenshot2.avif)

![Copperline Gotify notifications](screenshots/screenshot3.avif)

## LLM Code Policy

This project does not discriminate against the use of LLM generated code. The project already does contain LLM generated code. Just make sure the code compiles and does not introduce new bugs or cause it not to pass tests.

This project also chose the language Go precisely for its memory safety because of those guardrails for LLM generated code.

This code is daily driven by the author. All bugs are eliminated in a prompt manner by someone terminally online.

## Why Copperline is different

Copperline deliberately combines the things people expect from a traditional IRC client with features that are usually split across a client, a bouncer, shell tools, and notification services.

- **A real native terminal IRC client** — built in Go, compiled to a native executable, and designed around concurrent network work without making the UI wait on it.
- **A built-in SSH relay/bouncer** — run Copperline on a server, keep IRC connected there, and attach Copperline clients from other machines.
- **No external SSH daemon required** — relay mode has its own embedded SSH server and private Copperline channel protocol.
- **No certificate-authority hassle** — relay clients authenticate with SSH public keys and pin the relay's SSH host-key fingerprint.
- **Safer relay message delivery** — chat text is not cleared from the input field until the relay acknowledges the request. If the transport dies or a send cannot be confirmed, the text stays in the input instead of silently disappearing.
- **Relay health monitoring** — the client heartbeats the SSH transport and exposes an always-visible reconnect control with a configurable shortcut.
- **Multiple attached clients** — more than one Copperline client can attach to the same relay while the relay maintains the IRC connections.
- **Modern IRCv3 without abandoning classic IRC** — server-time, message tags, echo-message, typing indicators, CHATHISTORY, account/away state, SASL, CTCP, DCC, `/raw`, and the familiar slash-command workflow all live together.
- **A TUI meant to be lived in** — channel topics, nick lists, mouse support, buffer numbers, direct jumps, nickname completion, per-buffer input history, copy mode, startup progress, and configurable keybindings.
- **Lua scripting built in** — add commands, react to IRC events, automate repetitive work, and extend Copperline without recompiling it.
- **Notifications without freezing IRC** — Gotify requests run asynchronously and can alert on mentions and private messages.
- **Everything is themeable** — true-color palettes cover buffers, nicks, mentions, message types, borders, input, cursor, status areas, and more.

Copperline is designed so the IRC connection, UI, logging, scripting, notifications, relay transport, and DCC code remain separate pieces rather than collapsing into one giant `main.go`.

## Feature highlights

### Native Copperline SSH relay

Copperline can be its own IRC bouncer.

Run one Copperline instance in relay-server mode on an always-on machine. It maintains the real IRC connections, retains bounded in-memory conversation history, tracks channel state, and accepts Copperline clients over its **embedded SSH server**.

A relay client runs the normal Copperline TUI locally while the remote relay stays connected to IRC. Closing the local client does not make the relay leave your networks and channels.

Relay mode includes:

- public-key-only authentication;
- automatically generated Ed25519 host and client keys;
- SHA-256 host-key fingerprint pinning;
- a Copperline-specific `authorized_keys` file;
- multiple attached Copperline clients;
- retained message replay when a client attaches again;
- synchronized server, channel, topic, nick-list, capability, typing, and connection state;
- suppression of huge WHO/NAMES housekeeping floods such as `352`, `353`, and `354` from the relay transport;
- an SSH heartbeat so stale connections stop pretending to be healthy;
- a dedicated clickable **RECONNECT** control in relay-client mode;
- **Alt+R** as the default configurable force-reconnect shortcut;
- safe force reconnect that replaces only the local SSH attachment and leaves IRC connected on the relay;
- relay send acknowledgement before Copperline clears normal chat text from the input field.

The embedded SSH endpoint is intentionally narrow. It does **not** provide a shell, PTY, SFTP, remote command execution, or SSH port forwarding. It accepts Copperline's private relay channel only.

See [`RELAY.md`](RELAY.md) for complete setup instructions.

### Modern IRCv3

Copperline requests a broad modern capability set while still behaving like a recognizable IRC client. Depending on what the IRC network supports, Copperline can use:

- CAP negotiation;
- message tags;
- server-time;
- echo-message;
- account-notify and account-tag;
- away-notify;
- chghost;
- extended-join;
- multi-prefix;
- userhost-in-names;
- labeled-response;
- standard replies;
- CHATHISTORY;
- event playback;
- read markers;
- IRCv3 `+typing` client tags;
- SASL PLAIN and EXTERNAL.

The negotiated capability set is visible with `/caps`, and `/raw` remains available for network-specific features and experiments.

### A responsive, practical TUI

Copperline's interface is designed for long-running everyday use rather than a minimal demo.

- multiple IRC networks at once;
- channels and private-query buffers;
- numbered buffers;
- direct buffer jumping with `F6`;
- next/previous buffer shortcuts;
- channel topics with wrapping;
- scrollable nick list;
- mouse wheel support for transcripts and nick lists;
- double-click a nick to open a private query;
- channel-aware nickname completion with Tab cycling;
- `Up` / `Down` per-buffer message and command history;
- configurable input-history limit, defaulting to 10 entries per buffer;
- page and single-line transcript scrolling;
- WeeChat-style bare/copy mode for easy terminal text selection;
- visible startup progress while servers connect and channels join;
- drafts kept in the input when Copperline is not ready to send them;
- configurable keybindings with collision and validation checks;
- compatibility handling for modern terminal Ctrl/Alt key event forms.

### Lua scripting

Copperline embeds Lua for client-side extension and automation. Scripts can:

- register custom slash commands;
- receive IRC event hooks;
- send messages and notices;
- write local buffer text;
- send raw IRC commands;
- inspect the active server, target, and nickname;
- reload without restarting Copperline.

Use `/lua reload` to reload scripts. Copperline performs a physical terminal resync after script reloads to keep the TUI clean.

See [`SCRIPTING.md`](SCRIPTING.md) for the complete scripting API and [`scripts/`](scripts/) for examples.

### DCC, Gotify, logging, and themes

Copperline also includes features that are often missing from smaller terminal IRC clients:

- active DCC SEND;
- incoming DCC CHAT;
- asynchronous Gotify notifications for mentions and private messages;
- per-network/per-buffer persistent logs;
- configurable date/time stamps;
- muted pre-session conversation backlog without confusing current-session messages with old log text;
- configurable mention highlighting;
- deterministic per-nick colors;
- full true-color theme customization.

## Features

- **Native Go application** with garbage collection, goroutines, and a multithreaded runtime
- **Multiple IRC networks at once**, each with multiple channels and private-message/query buffers
- **Native Copperline SSH relay/bouncer mode** with embedded SSH and public-key authentication
- **Relay message safety** with send acknowledgement, heartbeat health detection, and force reconnect
- **Modern IRCv3 support** with CAP negotiation, message tags, server-time, echo-message, typing indicators, CHATHISTORY, account/away/chghost state, and more
- **TLS and SASL** with PLAIN and EXTERNAL authentication
- **Real DCC support** for sending and receiving files, plus incoming DCC CHAT
- **Gotify notifications** for mentions and private messages without blocking IRC
- **Persistent local logs** organized per network and buffer
- **True-color theming** for nicks, message types, buffer state, borders, input, status areas, mentions, and more
- **Traditional IRC usability** with nick lists, channel topics, CTCP, `/whois`, `/me`, `/notice`, `/raw`, and familiar slash commands
- **Fast buffer navigation** with numbered buffers, `F6` direct jumping, and configurable next/previous shortcuts
- **Nickname completion** with channel-aware Tab completion and match cycling
- **Mouse support** for scrollback, buffer selection, nick-list scrolling, private-query opening, and relay reconnect
- **Automatic IRC reconnect** and independent per-server connection handling
- **Clear startup feedback** with connecting/joining states and progress information
- **Configurable JOIN-message noise**, typing privacy, logging, mouse behavior, notifications, history limits, and other day-to-day preferences
- **TOML configuration** with environment-variable support for passwords and tokens
- **Embedded Lua scripting** for custom slash commands, IRC event hooks, automation, and raw protocol extensions
- **Raw IRC access** for network-specific commands and newer extensions without dedicated UI yet

## New in 0.2.5

`Alt+B` toggles the channel list and `Alt+U` toggles the user list independently. Chat uses the freed space, and typing and live updates continue. Visibility is temporary for the current session; the user list still automatically hides on narrow terminals. Rebind these with `[keybindings].toggle_channel_list` and `toggle_user_list`.

## Fixed in 0.2.4

When Gotify is enabled on the relay, the relay sends eligible mention/private-message alerts whether or not clients are attached. Client alerts are suppressed to avoid duplicates. If relay Gotify is disabled, the oldest eligible attached client remains responsible for notifications.

In relay-client mode, `/gotify status` and `/gotify test` now query/test the relay's notifier. Status includes HTTP delivery successes, failures, queue drops, and the last attempt/success/error. Counters reset when Copperline restarts; HTTP success means Gotify accepted the alert, not that a phone displayed it. Failed alerts are not automatically retried. Update the relay and clients for remote diagnostics; protocol remains 2.

## Fixed in 0.2.3

A sleeping or stalled relay client can no longer block shared IRC broadcasts while its SSH channel is being closed. Slow attachments are removed immediately, with socket cleanup in the background and a timeout on outgoing channel writes. Other attached machines remain independent of that cleanup. Typing notifications now run outside the keyboard handler, with at most one in flight, so an unresponsive relay cannot freeze draft editing through typing updates.

Update and restart the relay server to apply the attachment isolation fix; update desktop/laptop clients for responsive typing. Relay protocol remains 2.

## Fixed in 0.2.2

Reconnect readiness now waits for IRC registration. Chat, `/msg`, `/me`, and `/notice` retain their input if a send cannot be confirmed. A unique IRC PING/PONG checks that the server read past the text before reporting success; it does not guarantee recipient delivery or override channel permissions. If confirmation is lost, check the channel before retrying to avoid duplicates. Messages are not automatically resent.

SSH request timeouts now include blocked writes, and stale attachments close their underlying socket first. IRC keepalive detection is faster. Update both relay servers and clients to get all fixes; relay protocol remains 2.

## Fixed in 0.2.1

Channel selection once again clears activity indicators and follows the newest messages. IRC formatting controls and their color parameters no longer appear as stray digits in messages or restored log previews. Relay protocol remains 2.

## New in 0.2.0

- Relay-side Gotify alerts for mentions and private messages while no clients are attached; one eligible attached client owns live alerts to avoid duplicates.
- `/search text` searches retained messages in the current buffer, with literal, case-insensitive highlighting. `F3` / `F4` or `/searchnext` / `/searchprev` move between matching messages and wrap around. `/search` with no text clears the search.
- `Alt+A` or `/unread` jumps to the next unread buffer. Selecting a buffer clears its activity indicator and resumes following the newest messages. Messages arriving while you scroll back remain unread, with a **New messages below** divider; `End` or `/markread` acknowledges them.
- Optional `[relay].history_file` restores bounded structured replay history after a relay restart. Stable message IDs distinguish identical messages and deduplicate reconnect replay.
- Smaller TUI source files for commands, keyboard, mouse, navigation, rendering, reconnect, search, and catch-up behavior.

**Upgrade relay servers and clients together:** this release uses relay protocol 2. Search covers retained messages, not older disk logs. Read positions are currently local to each running client; persisting and synchronizing them is a future increment.

## Requirements

Copperline requires **Go 1.26 or newer**. Relay mode uses the September 2026 SSH security fixes in `golang.org/x/crypto v0.56.0`.

## Build

```sh
go mod tidy
go build -o Copperline ./cmd/copperline
```

Check the version with:

```sh
./Copperline -version
```

## Configuration

For the complete TOML reference, defaults, theme options, DCC, SASL, IRCv3 capability settings, keybindings, logging, and relay settings, see [`CONFIGURATION.md`](CONFIGURATION.md).

For Lua scripts, custom commands, event hooks, and the complete scripting API, see [`SCRIPTING.md`](SCRIPTING.md).

For the native SSH relay/bouncer mode, including server/client TOML examples and key setup, see [`RELAY.md`](RELAY.md).

Copperline defaults to:

```text
~/.config/copperline/config.toml
```

or `$XDG_CONFIG_HOME/copperline/config.toml` when `XDG_CONFIG_HOME` is set.

Start with `config.example.toml`:

```sh
mkdir -p ~/.config/copperline
cp config.example.toml ~/.config/copperline/config.toml
```

A different configuration file can be selected with:

```sh
./Copperline -config=./my-config.toml
```

### Passwords and secrets

For passwords and tokens, environment variables are preferable to storing secrets directly in TOML. Copperline reads the variable named by `password_env`, `token_env`, or the relevant secret setting.

For example, a Libera SASL configuration can use:

```toml
[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true

[server.sasl]
mechanism = "plain"
username = "myNick"
password_env = "LIBERA_IRC_PASSWORD"
```

With fish:

```fish
read -s -P 'Libera IRC password: ' LIBERA_IRC_PASSWORD
set -x LIBERA_IRC_PASSWORD $LIBERA_IRC_PASSWORD
./Copperline
```

For bash or zsh:

```sh
export LIBERA_IRC_PASSWORD='your-password-here'
./Copperline
```

### Native SSH relay mode

Select the Copperline role in TOML:

```toml
[relay]
mode = "direct" # direct, server, or client
```

A relay server contains the normal `[[server]]` IRC definitions. A relay client may omit them completely because it receives IRC network and channel state from the relay.

Relay-client mode has a dedicated always-visible reconnect control. By default:

```toml
[keybindings]
relay_reconnect = "Alt+R"
```

Force reconnect tears down only the client's Copperline SSH attachment. It does not tell the relay to disconnect from IRC.

See [`RELAY.md`](RELAY.md) for host-key fingerprint setup, generated client keys, `authorized_keys`, server/client configuration, retained history, and security notes.

### Logging

Logging is enabled by default. Copperline can show a small persisted conversation preview from before the current process/session while keeping current-session messages in normal live styling.

Disable all on-disk IRC logs while keeping normal in-memory scrollback with:

```toml
[general]
logging = false
```

The default log location is:

```text
~/.local/state/copperline/logs/<server>/<buffer>.log
```

Log files are created with mode `0600`; directories use `0700`.

### Gotify notifications

Copperline can push incoming mentions and private messages to Gotify. Gotify delivery is asynchronous so a slow or unavailable notification service does not freeze IRC or the TUI. When configured on the relay server, alerts continue with clients attached or detached. Use `/gotify status` on an updated relay client to inspect the relay's delivery counters and last error; `/gotify test` sends a test from the relay. In direct mode these commands inspect the local notifier.

```toml
[gotify]
enabled = true
url = "https://push.example.com"
token_env = "COPPERLINE_GOTIFY_TOKEN"
mentions = true
private_messages = true
priority = 5
timeout_seconds = 5
```

Test it from Copperline with:

```text
/gotify test
```

### Theme

Copperline ships with a dark navy/copper/aqua theme, but the entire TUI palette can be overridden with `[theme]`.

```toml
[theme]
background = "#090d16"
panel = "#101827"
foreground = "#d8dee9"
muted = "#77839a"
border = "#36506b"
title = "#64d8cb"
selected_fg = "#071018"
selected_bg = "#d99058"
timestamp = "#77839a"
server = "#64d8cb"
channel = "#8bd49c"
query = "#d9a7ff"
unread = "#f6c177"
mention = "#ff9ecb"
notice = "#f6c177"
action = "#d9a7ff"
system = "#72c7ef"
error = "#ff6b81"
dcc = "#8bd49c"
topic = "#f2cc8f"
input = "#eef1f6"
cursor_fg = "#071018"
cursor_bg = "#64d8cb"
status_fg = "#071018"
status_bg = "#64d8cb"
nick_colors = ["#ff7b72", "#f6c177", "#8bd49c", "#64d8cb", "#72c7ef", "#d9a7ff"]
```

Nick colors are chosen deterministically from `nick_colors`, so the same nickname keeps the same color.

## Lua scripting

Copperline embeds Lua for client-side automation and customization. Scripts load from `~/.config/copperline/scripts/*.lua` by default and can register slash commands, hook IRC events, send messages/notices, write to buffers, and use raw IRC.

Use:

```text
/lua reload
```

to reload scripts without restarting Copperline.

The complete API lives in [`SCRIPTING.md`](SCRIPTING.md), and the source tree includes ready-to-copy examples in [`scripts/`](scripts/).

## Layout and controls

The left panel contains servers and channel/query buffers. Every visible buffer is numbered. The middle panel contains the current conversation. Wide terminals also get a nick list on the right. The topic sits above the transcript, and the message input and status/control area sit at the bottom.

All action bindings are configurable under `[keybindings]`. Default controls include:

- `Ctrl-N`: next buffer
- `Ctrl-P`: previous buffer
- `Alt-A`: next unread buffer
- `F3` / `F4`: next / previous search match
- `Alt-N`: scroll the right-hand user list down
- `Alt-P`: scroll the right-hand user list up
- `F6`, number, `Enter`: jump directly to a numbered buffer
- `Escape`: cancel buffer jump
- `Tab`: nickname completion; repeated Tab cycles matches
- `Up` / `Down`: older/newer input history for the current buffer
- `PageUp` / `PageDown`: transcript by page
- `Alt-K` / `Alt-J`: transcript by one line
- `Alt-L`: bare/copy mode for terminal text selection
- `End`: return to/follow newest messages
- `Alt-R`: force reconnect to a Copperline relay while in relay-client mode
- `Ctrl-U`: clear input
- `Ctrl-C`: quit

For example:

```toml
[keybindings]
user_list_down = "Ctrl+Y"
user_list_up = "Ctrl+U"
history_previous = "Up"
history_next = "Down"
relay_reconnect = "Alt+R"
```

If you reassign a binding such as `Ctrl+U`, move any conflicting action to another key. Copperline validates configurable action keys so invalid or conflicting bindings fail clearly instead of silently doing something unexpected.

Input history is session-only and separate for each buffer. It stores both ordinary messages and slash commands. The default is 10 entries per buffer and is configurable with:

```toml
[general]
input_history_limit = 10
```

Set it to `0` to disable input history.

Press `Alt+B` to show/hide the channel list, or `Alt+U` to show/hide the user list. Both toggles keep chat live and your draft intact.

Press `Alt-L` for bare/copy mode. Copperline temporarily hides the normal UI chrome and disables terminal mouse reporting so the terminal can perform normal text selection. IRC, logging, Gotify, DCC, scripting, and incoming message state continue in the background.

## Commands

```text
/help
/buffer number
/server [name]
/connect [server]
/disconnect [server] [reason]
/join #channel [key]
/part [#channel] [reason]
/query nick
/msg nick message
/me action
/notice target message
/ctcp nick command [text]
/nick newnick
/topic new topic
/whois nick
/raw IRC COMMAND HERE
/history [count]
/search [text]
/searchnext
/searchprev
/unread
/markread
/caps
/dcc list
/dcc accept nick
/dcc send nick /path/to/file
/gotify status
/gotify test
/lua list
/lua reload
/lua eval <code>
/close
/quit [reason]
```

CTCP examples:

```text
/ctcp Alice VERSION
/ctcp Alice TIME
/ctcp Alice PING 123456789
```

## IRCv3 capability set

Capabilities currently requested by default include:

```text
account-notify
account-tag
away-notify
batch
cap-notify
chghost
echo-message
extended-join
invite-notify
labeled-response
message-tags
multi-prefix
server-time
setname
standard-replies
userhost-in-names
draft/chathistory
draft/event-playback
draft/extended-monitor
draft/message-redaction
draft/multiline
draft/no-implicit-names
draft/read-marker
```

Not every requested draft capability has a dedicated UI action yet. `/raw` remains available for experimenting with network-specific IRCv3 commands and extensions.

## DCC notes

Traditional DCC establishes a direct TCP connection between IRC users. It is not protected by the TLS connection to the IRC server and can reveal IP addresses.

For outgoing DCC SEND, configure `dcc.advertise_ip` to an address the other user can reach. NAT/router port forwarding may be necessary. Incoming filenames are reduced to their basename, downloads use exclusive file creation, and existing files are not overwritten.

Copperline currently implements normal active DCC SEND and incoming DCC CHAT. Passive/reverse DCC SEND (`port 0`) and DCC RESUME are possible future additions.

## Project layout

```text
cmd/copperline/       program entry point
internal/config/      TOML configuration
internal/model/       buffers and messages
internal/irc/         direct multi-server IRC + IRCv3 backend
internal/relay/       embedded SSH relay server/client and protocol
internal/logging/     persistent logs and pre-session backlog
internal/gotify/      Gotify notification client
internal/dcc/         DCC parsing and transfers
internal/scripting/   embedded Lua runtime and scripting API
scripts/              example Lua scripts
internal/tui/         gotui interface, navigation, input, and commands
```

## Mention highlighting

Copperline highlights incoming messages and `/me` actions that mention your current nickname. The mention color is configurable with `theme.mention`. Your own outgoing messages are never highlighted as mentions.

## License

Copperline is licensed under the **GNU General Public License version 3 or later (GPL-3.0-or-later)**. See [`LICENSE`](LICENSE).
