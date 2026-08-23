# Copperline

Copperline is a multi-server, multi-channel terminal IRC client written in Go using `github.com/metaspartan/gotui/v5`.

This is a usable first implementation with a deliberately separated IRC core, TUI, buffer model, logging layer, configuration loader, and DCC manager so the client can grow without becoming one giant `main.go`.

## Features

- Multiple IRC servers connected at the same time
- Multiple channels and private-message/query buffers
- TOML configuration
- Configurable true-color theme with colored nicks, message types, buffer state, borders, input and status bars
- TLS IRC connections
- SASL PLAIN and EXTERNAL through girc
- IRCv3 CAP negotiation and message tags
- IRCv3 server-time timestamps
- IRCv3 echo-message handling
- IRCv3 account/away/chghost/extended-join state through girc
- Requests modern capabilities including batch, labeled-response, standard-replies, chathistory, read-marker, multiline and related draft caps
- `/history` support using IRCv3 CHATHISTORY
- Local per-network/per-buffer logs
- DCC SEND receive/send
- Incoming DCC CHAT acceptance
- Mouse wheel scrollback
- Mouse channel/query selection
- Mouse nick selection to open a query
- Nick list
- Channel topics
- Automatic reconnect
- Raw IRC command access for extensions not yet given a dedicated command

## Requirements

Current gotui v5 requires Go 1.24 or newer.

## Build

```sh
go mod tidy
go build -o Copperline ./cmd/copperline
```

## Configuration

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

For passwords, environment variables are preferable to putting secrets directly in TOML:

```sh
export LIBERA_IRC_PASSWORD='...'
./Copperline
```

You can also select another configuration:

```sh
./Copperline -config=./my-config.toml
```

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

## Layout

The left panel contains every configured server and its channel/query buffers. The middle is the current conversation. Wide terminals also get a nick list on the right. The input line is at the bottom.

Keyboard controls:

- `Ctrl-N` / `Tab`: next buffer
- `Ctrl-P`: previous buffer
- `PageUp` / `PageDown`: scroll transcript
- `End`: return to following the newest messages
- `Ctrl-U`: clear input
- `Ctrl-C`: quit

Mouse controls are enabled when `general.mouse = true`. Click a server/channel/query in the sidebar, click a nick to open a query, and use the wheel over the transcript to scroll.

## Commands

```text
/help
/server [name]
/connect [server]
/disconnect [server] [reason]
/join #channel [key]
/part [#channel] [reason]
/query nick
/msg nick message
/me action
/notice target message
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
/close
/quit [reason]
```

## IRCv3

Copperline asks for a broad modern capability set and lets the server decide what is actually enabled. The negotiated set is visible with `/caps`.

The underlying girc library already implements IRCv3 CAP handling, message tags, SASL PLAIN/EXTERNAL, server-time integration, account-notify, away-notify, chghost, extended-join and user/channel state tracking. Copperline additionally requests and understands enough of the wire protocol to use CHATHISTORY, batches/message tags, echo-message, standard replies and read-marker commands while retaining `/raw` for newer extensions.

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
internal/dcc/         DCC parsing and transfers
internal/tui/         gotui interface and commands
```
