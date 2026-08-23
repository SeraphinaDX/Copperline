# Copperline

Copperline is a multi-server, multi-channel terminal IRC client written in Go using `github.com/metaspartan/gotui/v5`.

![Copperline main interface](screenshots/screenshot1.avif)

![Copperline themes](screenshots/screenshot2.avif)

![Copperline Gotify notifications](screenshots/screenshot3.avif)

This is a usable first implementation with a deliberately separated IRC core, TUI, buffer model, logging layer, configuration loader, and DCC manager so the client can grow without becoming one giant `main.go`.

## Features

- Multiple IRC servers connected at the same time
- Multiple channels and private-message/query buffers
- Numbered buffers with WeeChat-style `F6` direct jumping
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
- Optional local per-network/per-buffer logs
- Optional Gotify notifications for incoming mentions and private messages
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

For the complete TOML reference, defaults, inheritance rules, theme options, DCC, SASL, and IRCv3 capability settings, see [`CONFIGURATION.md`](CONFIGURATION.md).

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

Logging is enabled by default. Disable all on-disk IRC logs while keeping normal in-memory scrollback with:

```toml
[general]
logging = false
```

See `CONFIGURATION.md` for log paths, permissions, and related options.

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

## Layout

The left panel contains every configured server and its channel/query buffers. Every visible buffer is numbered in sidebar order so it can be selected directly. The middle is the current conversation. Wide terminals also get a nick list on the right. The input line is at the bottom.

Keyboard controls:

- `Ctrl-N`: next buffer
- `Ctrl-P`: previous buffer
- `F6`, number, `Enter`: jump directly to a numbered buffer
- `Escape`: cancel an active F6 buffer jump
- `Tab`: complete a nickname at the cursor; repeated Tab cycles matches
- `PageUp` / `PageDown`: scroll transcript
- `End`: return to following the newest messages
- `Ctrl-U`: clear input
- `Ctrl-C`: quit

Nickname completion is channel-aware. For example, typing `ali` at the start of the input and pressing `Tab` can produce `Alice: `. If more than one nickname matches, press `Tab` repeatedly to cycle through them. When the partial nick appears later in a message, Copperline completes only the nick and does not add the reply colon.

Buffer numbers are shown in the left sidebar and follow the same order used by `Ctrl-N` and `Ctrl-P`. To jump directly, press `F6`, type the displayed number, and press `Enter`. For example, if `#golang` is buffer `7`, use `F6`, `7`, `Enter`. Press `Escape` to cancel. Buffer numbers can change when buffers are opened or closed, so the sidebar is the source of truth. `Tab` remains dedicated to nickname completion.

Mouse controls are enabled when `general.mouse = true`. Click a server/channel/query in the sidebar, click a nick to open a query, and use the wheel over the transcript to scroll.

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
internal/tui/         gotui interface and commands
```


## Mention highlighting

Copperline highlights incoming messages and `/me` actions that mention your current nickname. The mention color is configurable with `theme.mention` (default `#ff9ecb`). Your own outgoing messages are never highlighted as mentions.