# Copperline

Copperline is my modern terminal IRC client: a multi-server, multi-channel client written in **Go** with `github.com/metaspartan/gotui/v5`. I wanted something that still feels like a traditional IRC client while taking advantage of a modern language, a responsive TUI, and the parts of IRCv3 that make IRC nicer to use today.

Go is a particularly good fit for Copperline. It compiles to a fast native executable, has a **garbage-collected runtime**, and is designed for concurrency. Copperline uses goroutines for work that should happen independently—IRC connections, reconnect handling, DCC transfers, notifications, and timers—while Go's **multithreaded runtime** can execute that work across OS threads and CPU cores. The result is a client that can be doing several things at once without turning the UI architecture into a maze.

Copperline is not intended to be a tiny IRC demo. It has grown into a full everyday client with multiple networks, IRCv3, SASL, DCC, scrollback and logging, Gotify notifications, typing indicators, mouse support, true-color themes, nickname completion, direct buffer jumping, CTCP, and the usual IRC commands I actually want to use.

![Copperline main interface](screenshots/screenshot1.avif)

![Copperline themes](screenshots/screenshot2.avif)

![Copperline Gotify notifications](screenshots/screenshot3.avif)

The code is deliberately split into an IRC core, TUI, buffer model, logging layer, configuration loader, notification client, and DCC manager. Go makes that separation pleasantly straightforward, and keeps Copperline from becoming one giant `main.go` as it grows.

## Features

- Clear startup feedback: while auto-connect servers and configured channels are
  still loading, Copperline shows an animated `STARTING` status with server and
  channel progress, marks connecting/joining entries in the sidebar, and changes
  the input title to `Connecting - please wait` or `Joining - please wait`.
  Messages submitted too early are kept in the input box instead of being lost.

- **Native Go application** with garbage collection, goroutines, and a multithreaded runtime
- **Multiple IRC networks at once**, each with multiple channels and private-message/query buffers
- **Native Copperline SSH relay/bouncer mode** — keep IRC sessions alive on a headless Copperline server and attach one or more local Copperline TUIs over an embedded, public-key-authenticated SSH transport; no system `ssh`, `sshd`, TLS certificate, or CA is required
- **Modern IRCv3 support** with CAP negotiation, message tags, server-time, echo-message, typing indicators, CHATHISTORY, account/away/chghost state, and more
- **TLS and SASL** with PLAIN and EXTERNAL authentication
- **Real DCC support** for sending and receiving files, plus incoming DCC CHAT
- **Gotify notifications** for mentions and private messages without blocking the IRC client
- **Persistent local logs** organized per network and buffer, with configurable timestamps, a muted 10-line conversation backlog on first open, and an option to disable logging entirely
- **True-color theming** for nicks, message types, buffer state, borders, input, status bars, mentions, and more
- **Traditional IRC usability** with nick lists, channel topics, CTCP, `/whois`, `/me`, `/notice`, `/raw`, and familiar slash commands
- **Fast buffer navigation** with numbered buffers, `F6` direct jumping, and `Ctrl-N` / `Ctrl-P`
- **Nickname completion** with channel-aware Tab completion and match cycling
- **Mouse support** for scrollback, buffer selection, and opening private queries from the nick list
- **Automatic reconnect** and independent per-server connection handling
- **Configurable JOIN-message noise**, typing privacy, logging, mouse behavior, notifications, and other day-to-day preferences
- **TOML configuration** with environment-variable support for passwords and tokens
- **Embedded Lua scripting** for custom slash commands, IRC event hooks, automation, and raw protocol extensions
- Raw IRC access remains available for network-specific commands and newer extensions that do not yet have dedicated UI

## Requirements

Copperline now requires Go 1.26 or newer. Relay mode uses the September 2026 SSH security fixes in golang.org/x/crypto v0.56.0.

## Build

```sh
go mod tidy
go build -o Copperline ./cmd/copperline
```

## Configuration

For the complete TOML reference, defaults, inheritance rules, theme options, DCC, SASL, and IRCv3 capability settings, see [`CONFIGURATION.md`](CONFIGURATION.md).

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

For passwords, environment variables are preferable to putting secrets directly in TOML. Copperline reads the variable named by `password_env` in your server or SASL configuration.

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

### Native SSH relay mode

Copperline can run as a headless IRC relay and keep the IRC connections alive while local Copperline TUIs attach over Copperline's own embedded SSH server. The SSH endpoint accepts only Copperline's private relay channel; it does not expose a shell or port forwarding.

Select the role in TOML:

```toml
[relay]
mode = "direct" # direct, server, or client
```

A relay server still contains the normal `[[server]]` IRC definitions. A relay client may omit them entirely because it receives the server/channel state from the relay. See [`RELAY.md`](RELAY.md) for setup.

Relay-client mode also exposes a clickable **⟳ RECONNECT** control in the status bar, with **Alt+R** as the default configurable shortcut (`[keybindings].relay_reconnect`). It reconnects only the SSH attachment, not the relay's IRC sessions.

### Fish shell passwords

With fish, export the password in the shell before starting Copperline:

```fish
set -x LIBERA_IRC_PASSWORD 'your-password-here'
./Copperline
```

`set -x` exports the variable so Copperline can read it. The variable exists only for the current fish session unless you deliberately make it persistent. Start Copperline from that same shell.

For a password containing spaces or shell metacharacters, keep it quoted:

```fish
set -x LIBERA_IRC_PASSWORD 'a password with spaces & symbols!'
```

To avoid typing the password visibly on the command line, fish can prompt for it without echoing the characters:

```fish
read -s -P 'Libera IRC password: ' LIBERA_IRC_PASSWORD
set -x LIBERA_IRC_PASSWORD $LIBERA_IRC_PASSWORD
./Copperline
```

To verify that the variable is exported without printing the password itself:

```fish
set -q LIBERA_IRC_PASSWORD; and echo 'LIBERA_IRC_PASSWORD is set'
```

To remove it from the current shell afterward:

```fish
set -e LIBERA_IRC_PASSWORD
```

You *can* make an exported fish variable persistent with `set -Ux`, but that stores the value in fish's universal-variable data on disk. For passwords, a session-only `set -x` (or the hidden `read -s` approach above) is generally preferable.

For bash/zsh, the equivalent is:

```sh
export LIBERA_IRC_PASSWORD='your-password-here'
./Copperline
```

The same pattern works for traditional IRC `PASS` authentication: point the server's `password_env` at a variable name and export that variable before starting Copperline.

You can also select another configuration:

```sh
./Copperline -config=./my-config.toml
```

### Logging

Logging is enabled by default. The first time you open a channel during a Copperline session, Copperline shows up to the last 10 meaningful persisted lines from **before the current Copperline session** in the muted theme color, then shows every retained message received during the current run with normal live styling. This remains true even if you do not open that channel until much later. Switching away and back preserves that channel's live transcript; the log preview is not reloaded. Configure or disable it with `general.log_backlog_lines`.

Disable all on-disk IRC logs while keeping normal in-memory scrollback with:

```toml
[general]
logging = false
```

See `CONFIGURATION.md` for log paths, permissions, backlog behavior, and related options.

### Gotify notifications


Copperline can push incoming mentions and private messages to a Gotify server. Create a Gotify **application** and use its application token. Environment variables are recommended instead of storing the token directly in TOML.

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

With fish:

```fish
read -s -P 'Gotify application token: ' COPPERLINE_GOTIFY_TOKEN
set -x COPPERLINE_GOTIFY_TOKEN $COPPERLINE_GOTIFY_TOKEN
./Copperline
```

Test the setup inside Copperline with:

```text
/gotify test
```

Gotify requests run asynchronously, so an unavailable notification server does not freeze IRC or the TUI. Your own messages do not generate Gotify notifications. See `CONFIGURATION.md` for every Gotify option and the exact trigger behavior.

### Theme


Copperline has a built-in dark navy/copper/aqua theme. Add a `[theme]` section only when you want to override it. Colors may be `#RRGGBB` true-color values or gotui color names such as `cyan`, `orange`, `purple`, `skyblue`, and `lightgreen`.

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

Nick colors are chosen deterministically from `nick_colors`, so the same nickname keeps the same color. Theme changes affect only the TUI; log files remain plain text. See `config.example.toml` for the full default palette.

## Lua scripting

Copperline embeds Lua for client-side automation and customization. Scripts load from `~/.config/copperline/scripts/*.lua` by default and can register slash commands, hook IRC events, send messages/notices, write to buffers, and use raw IRC. Use `/lua reload` to reload scripts without restarting the client.

The complete API lives in [`SCRIPTING.md`](SCRIPTING.md), and the source tree includes ready-to-copy examples in [`scripts/`](scripts/).

## Layout

The left panel contains every configured server and its channel/query buffers. Every visible buffer is numbered in sidebar order so it can be selected directly. The middle is the current conversation. Wide terminals also get a nick list on the right. The input line is at the bottom.

Keyboard controls (all of these action bindings are configurable under `[keybindings]`):

- `Ctrl-N`: next buffer
- `Ctrl-P`: previous buffer
- `Alt-N`: scroll the right-hand user list down
- `Alt-P`: scroll the right-hand user list up
- `F6`, number, `Enter`: jump directly to a numbered buffer
- `Escape`: cancel an active buffer jump
- `Tab`: complete a nickname at the cursor; repeated Tab cycles matches
- `Up` / `Down`: recall older/newer input history for the current buffer (messages and slash commands)
- `PageUp` / `PageDown`: scroll transcript by page
- `Alt-K` / `Alt-J`: scroll transcript up/down by one line
- `Alt-L`: toggle bare/copy mode for easy terminal text selection
- `End`: return to following the newest messages
- `Ctrl-U`: clear input
- `Ctrl-C`: quit

For example, to move nick-list scrolling from `Alt-N` / `Alt-P` to other keys:

```toml
[keybindings]
user_list_down = "Alt+N"
user_list_up = "Alt+P"
history_previous = "Up"
history_next = "Down"
```

Bindings use readable names such as `Ctrl+N`, `Alt+P`, `F6`, `PageUp`, `Escape`, and `Tab`. Existing configs do not need a `[keybindings]` section; omitted values keep the defaults. Copperline rejects invalid or conflicting action bindings at startup so a typo cannot silently disable a shortcut. Configured Ctrl/Alt bindings are matched from both gotui event IDs and the underlying tcell modifier data for compatibility with modern terminal keyboard protocols.

Input history is kept in memory for the current Copperline session and is separate for each buffer. The default limit is 10 entries per buffer and can be changed with `input_history_limit` in `[general]` (`0` disables it). It records both ordinary messages and slash commands. When you press `Up` while a draft is in the input box, Copperline saves that draft; pressing `Down` past the newest recalled item restores it. Consecutive duplicate entries are stored only once. Older configs that explicitly used the old `Up`/`Down` transcript-line defaults are migrated automatically to `Alt+K`/`Alt+J`.

Nickname completion is channel-aware. For example, typing `ali` at the start of the input and pressing `Tab` can produce `Alice: `. If more than one nickname matches, press `Tab` repeatedly to cycle through them. When the partial nick appears later in a message, Copperline completes only the nick and does not add the reply colon.

Buffer numbers are shown in the left sidebar and follow the same order used by `Ctrl-N` and `Ctrl-P`. To jump directly, press `F6`, type the displayed number, and press `Enter`. For example, if `#golang` is buffer `7`, use `F6`, `7`, `Enter`. Press `Escape` to cancel. Buffer numbers can change when buffers are opened or closed, so the sidebar is the source of truth. `Tab` remains dedicated to nickname completion.

Mouse controls are enabled when `general.mouse = true`. Click a server/channel/query in the sidebar, double-click a nick to open a private query, use the wheel over the transcript to scroll messages, and use the wheel over the right-hand user list to scroll nicknames.

By default, `Alt-N` / `Alt-P` scroll the nick list without changing the current buffer while `Ctrl-N` / `Ctrl-P` navigate buffers. Those keyboard bindings can be changed independently in `[keybindings]`; mouse-wheel nick scrolling remains available when the pointer is over the Users pane.

Press `Alt-L` for a WeeChat-style bare/copy view. Copperline temporarily hides the sidebar, topic, nick list, input, status bar, and transcript borders; disables terminal mouse reporting; and freezes redraws so you can select and copy text with the terminal normally. IRC connections, logging, DCC, Gotify notifications, and incoming message state continue in the background. Press `Alt-L` again to restore the full interface and catch up.

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

CTCP requests can be sent directly to another user. For example:

```text
/ctcp Alice VERSION
/ctcp Alice TIME
/ctcp Alice PING 123456789
```

Replies are displayed as system lines in that user's query buffer.

## IRCv3

Copperline asks for a broad modern capability set and lets the server decide what is actually enabled. The negotiated set is visible with `/caps`.

The underlying girc library already implements IRCv3 CAP handling, message tags, SASL PLAIN/EXTERNAL, server-time integration, account-notify, away-notify, chghost, extended-join and user/channel state tracking. Copperline additionally requests and understands enough of the wire protocol to use CHATHISTORY, batches/message tags, echo-message, standard replies and read-marker commands while retaining `/raw` for newer extensions.

Copperline also implements the IRCv3 `+typing` client tag. When another compatible user is typing, the input title changes to something like `Message — Alice is typing…`. Copperline sends `active`, `paused`, and `done` typing state for ordinary message composition, but never for `/slash commands`. Both directions are configurable:

```toml
[general]
show_typing = true
send_typing = true
```

Set `send_typing = false` if you prefer not to advertise your typing state. Typing tags depend on the negotiated `message-tags` capability; `+typing` itself is a client tag, not a separate capability name.

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

The IRCv3 ecosystem is still evolving. Not every requested draft capability has a dedicated Copperline UI action yet; unsupported incoming commands still pass through the event handling/logging path where girc considers them displayable, and `/raw` can be used to experiment with network-specific commands.

## Logging

Logs are written by default below:

```text
~/.local/state/copperline/logs/<server>/<buffer>.log
```

Log files are created with mode `0600`; directories use `0700`.

## DCC notes

DCC is intentionally configurable and can be disabled. Traditional DCC creates a direct TCP connection between IRC users. It is not protected by the TLS connection to the IRC server and can reveal IP addresses.

For outgoing DCC SEND, configure `dcc.advertise_ip` to an address the other user can reach. NAT/router port forwarding may be necessary. Incoming filenames are reduced to their basename, downloads use exclusive file creation, and existing files are not overwritten.

This version implements normal active DCC SEND and accepts DCC CHAT. Passive/reverse DCC SEND (`port 0`) and DCC RESUME are good next additions.

## Project layout

```text
cmd/copperline/       program entry point
internal/config/      TOML configuration
internal/model/       buffers and messages
internal/irc/         multi-server IRC + IRCv3 manager
internal/logging/     local logs
internal/gotify/      Gotify notification client
internal/dcc/         DCC parsing and transfers
internal/scripting/   embedded Lua runtime and plugin API
scripts/              example Lua scripts
internal/tui/         gotui interface and commands
```


## Mention highlighting

Copperline highlights incoming messages and `/me` actions that mention your current nickname. The mention color is configurable with `theme.mention` (default `#ff9ecb`). Your own outgoing messages are never highlighted as mentions.