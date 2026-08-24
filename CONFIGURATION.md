# Copperline Configuration Reference

Copperline is configured with TOML. This document describes every configuration option supported by the current build.

## Configuration file location

By default Copperline loads:

```text
~/.config/copperline/config.toml
```

If `XDG_CONFIG_HOME` is set, Copperline instead uses:

```text
$XDG_CONFIG_HOME/copperline/config.toml
```

A different configuration file can be selected at startup:

```bash
Copperline -config=/path/to/config.toml
```

Copperline requires at least one `[[server]]` entry. Every server must have a unique non-empty `name` and a non-empty `host`.

## Complete example

```toml
[general]
nick = "myNick"
user = "myNick"
real_name = "Copperline User"
logging = true
log_dir = "~/.local/state/copperline/logs"
timestamp = "15:04"
mouse = true
show_typing = true
send_typing = true
history_lines = 2000
reconnect_seconds = 10

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
nick_colors = [
    "#ff7b72", "#f6c177", "#8bd49c", "#64d8cb",
    "#72c7ef", "#d9a7ff", "#ff9ecb", "#d99058",
]

[dcc]
enabled = true
download_dir = "~/Downloads"
listen_addr = "0.0.0.0:0"
advertise_ip = "192.0.2.10"

[gotify]
enabled = false
url = "https://push.example.com"
token_env = "COPPERLINE_GOTIFY_TOKEN"
mentions = true
private_messages = true
priority = 5
timeout_seconds = 5

[[server]]
name = "libera"
host = "irc.libera.chat"
port = 6697
tls = true
skip_verify = false
auto_connect = true
nick = "myNick"
user = "myNick"
real_name = "Copperline User"
channels = ["#go-nuts", "#linux"]
caps = []

# Optional traditional IRC PASS authentication.
# password = "secret"
# password_env = "LIBERA_SERVER_PASSWORD"

# Optional SASL authentication for the server entry immediately above.
[server.sasl]
mechanism = "plain"
username = "myNick"
password_env = "LIBERA_IRC_PASSWORD"

[[server]]
name = "oftc"
host = "irc.oftc.net"
tls = true
auto_connect = false
channels = ["#debian"]
```

---

# `[general]`

Global defaults and client-wide behavior.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `nick` | string | `"copperline"` | Default IRC nickname. Individual servers may override it. |
| `user` | string | value of `nick` | Default IRC username sent during registration. Individual servers may override it. |
| `real_name` | string | `"Copperline IRC Client"` | Default IRC real-name/GECOS field. Individual servers may override it. |
| `logging` | bool | `true` | Enables or disables all on-disk IRC logging. In-memory buffer history is unaffected. |
| `log_dir` | string | `"~/.local/state/copperline/logs"` | Root directory for IRC logs when logging is enabled. `~` and `~/...` are expanded. |
| `timestamp` | string | `"15:04"` | Go time-layout string used for displayed messages and logs. |
| `mouse` | bool | `false` | Enables gotui mouse handling, including clickable buffers/nicks and mouse-wheel scrollback. |
| `show_typing` | bool | `true` | Shows incoming IRCv3 `+typing` indicators in the message input title when the server supports `message-tags`. |
| `send_typing` | bool | `true` | Sends your IRCv3 `+typing` state to compatible clients. Set to `false` if you do not want to reveal when you are composing a message. Slash commands never generate typing notifications. |
| `show_join_messages` | bool | `true` | Shows `nick joined` lines in channel buffers. Set to `false` to hide JOIN messages while still tracking channel membership normally. |
| `history_lines` | integer | `1000` | Maximum number of messages retained in each in-memory buffer. Values `<= 0` are reset to `1000`. |
| `reconnect_seconds` | integer | `10` | Delay between reconnect attempts. Values `<= 0` are reset to `10`. |

## IRCv3 typing indicators

Copperline supports the IRCv3 `+typing` client tag when the network negotiates `message-tags`. Incoming typing state is shown in the message input title, for example `Message — Alice is typing…`.

```toml
[general]
show_typing = true
send_typing = true
```

`show_typing = false` hides other users' indicators. `send_typing = false` is the privacy control that prevents Copperline from advertising when you are composing a message. Copperline never sends typing state while the input is a slash command. Active notifications are throttled and refreshed according to the IRCv3 timing rules; stale incoming indicators expire automatically.

## Join messages

Channel JOIN lines can be hidden without changing Copperline's membership tracking:

```toml
[general]
show_join_messages = false
```

When disabled, Copperline still processes JOIN events internally, updates nick/channel state, and confirms membership in the sidebar; it simply does not add `nick joined` lines to channel buffers. PART, QUIT, KICK, and topic messages are unaffected.

## Identity inheritance

Server identity settings inherit from `[general]` when omitted:

```toml
[general]
nick = "Britney"
user = "britney"
real_name = "Britney"

[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true

[[server]]
name = "othernet"
host = "irc.example.net"
tls = true
nick = "DifferentNick"
```

`libera` uses the global identity. `othernet` overrides only the nickname and still inherits the global `user` and `real_name`.

## Timestamp format

Copperline uses Go's reference-time format. Common examples:

```toml
# 24-hour hours and minutes
timestamp = "15:04"

# Hours, minutes, seconds
timestamp = "15:04:05"

# Date and time
timestamp = "2006-01-02 15:04:05"
```

## Logging behavior

Logs are stored beneath `log_dir`, grouped by server and target:

```text
~/.local/state/copperline/logs/
├── libera/
│   ├── #linux.log
│   ├── #go-nuts.log
│   └── SomeNick.log
└── oftc/
    └── #debian.log
```

Server directories are created with mode `0700`; log files are created with mode `0600`. Copperline appends to existing logs rather than replacing them.

Logging is enabled by default. To disable all disk logging:

```toml
[general]
logging = false
```

When `logging = false`, Copperline does not create or append log files. Normal in-memory buffer history still works and is controlled separately by `history_lines`. Existing log files are left untouched.

`log_dir` is ignored while logging is disabled. Per-server and per-channel logging overrides are not currently supported.

---

# `[theme]`

The entire `[theme]` section is optional. Any omitted theme field receives Copperline's built-in default.

Colors may be supplied as true-color hexadecimal values:

```toml
border = "#36506b"
```

or as color names understood by gotui, such as `red`, `green`, `yellow`, `blue`, `cyan`, `magenta`, `white`, `grey`, `orange`, `purple`, `pink`, `gold`, `teal`, `turquoise`, `violet`, `lightblue`, `lightgreen`, or `skyblue`.

Use known names or `#RRGGBB` values; invalid color strings are not recommended.

| Option | Default | Controls |
| --- | --- | --- |
| `background` | `#090d16` | Base/background color used around panel borders. |
| `panel` | `#101827` | Main panel backgrounds. |
| `foreground` | `#d8dee9` | Normal foreground text. |
| `muted` | `#77839a` | Muted UI elements, including configured-but-not-joined buffer indicators. |
| `border` | `#36506b` | Panel borders. |
| `title` | `#64d8cb` | Panel titles. |
| `selected_fg` | `#071018` | Foreground of the selected sidebar row. |
| `selected_bg` | `#d99058` | Background of the selected sidebar row. |
| `timestamp` | `#77839a` | Message timestamps. |
| `server` | `#64d8cb` | Server indicator/color in the sidebar. |
| `channel` | `#8bd49c` | Joined-channel indicator/color. |
| `query` | `#d9a7ff` | Private-query indicator/color. |
| `mention` | `#ff9ecb` | Incoming messages/actions that mention your current nick. |
| `unread` | `#f6c177` | Unread-count indicator. |
| `notice` | `#f6c177` | IRC NOTICE sender marker. |
| `action` | `#d9a7ff` | `/me`/CTCP ACTION marker. |
| `system` | `#72c7ef` | System messages such as joins, parts, connection events, and informational replies. |
| `error` | `#ff6b81` | Error messages. |
| `dcc` | `#8bd49c` | DCC messages and transfer events. |
| `topic` | `#f2cc8f` | Channel topic text. |
| `input` | `#eef1f6` | Text in the input box. |
| `cursor_fg` | `#071018` | Input cursor foreground. |
| `cursor_bg` | `#64d8cb` | Input cursor background. |
| `status_fg` | `#071018` | Status-bar foreground. |
| `status_bg` | `#64d8cb` | Status-bar background. |
| `nick_colors` | eight-color built-in palette | Palette used to assign stable colors to nicknames. |

## Nickname colors

Copperline hashes each nickname into `nick_colors`, so a nick is assigned a stable palette entry rather than receiving a random color every time it is drawn.

Default palette:

```toml
nick_colors = [
    "#ff7b72",
    "#f6c177",
    "#8bd49c",
    "#64d8cb",
    "#72c7ef",
    "#d9a7ff",
    "#ff9ecb",
    "#d99058",
]
```

You may supply any number of colors. If the list is omitted or empty, Copperline restores the default palette.

---

# `[dcc]`

Controls Direct Client-to-Client support.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `enabled` | bool | `false` | Enables DCC handling. When false, incoming DCC CTCP offers are not registered and outgoing `/dcc send` is rejected. |
| `download_dir` | string | `"~/Downloads"` | Destination for received DCC SEND files. `~` and `~/...` are expanded. |
| `listen_addr` | string | `"0.0.0.0:0"` | Local TCP address used for outgoing DCC SEND. Port `0` asks the OS to choose a free port. |
| `advertise_ip` | string | empty | IP address advertised to the peer for outgoing DCC SEND. |

Example:

```toml
[dcc]
enabled = true
download_dir = "~/Downloads"
listen_addr = "0.0.0.0:0"
advertise_ip = "192.168.1.50"
```

## `listen_addr`

The value is a Go TCP listen address. Examples:

```toml
# Any IPv4 interface, automatically chosen port
listen_addr = "0.0.0.0:0"

# Specific local interface, automatically chosen port
listen_addr = "192.168.1.50:0"

# Specific interface and port
listen_addr = "192.168.1.50:50000"
```

## `advertise_ip`

With the default wildcard listener (`0.0.0.0:0`), outgoing DCC SEND needs `advertise_ip`, because `0.0.0.0` is not an address another client can connect to.

If `advertise_ip` is empty and `listen_addr` resolves to a specific non-wildcard IP, Copperline uses that listener IP.

For peers outside your LAN, the advertised address must actually be reachable. NAT/firewall configuration may therefore be necessary.

DCC traffic is a direct peer-to-peer TCP connection. IRC TLS does not encrypt the DCC transfer itself.

---

# `[gotify]`

Optional Gotify push notifications. Copperline sends notifications using Gotify's application message API. Create an **application** in the Gotify WebUI and use that application's token; a Gotify client token is not the token used for sending messages.

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `enabled` | bool | `false` | Enables Gotify notifications. When false, Copperline makes no Gotify HTTP requests. |
| `url` | string | none | Gotify server base URL, for example `https://push.example.com` or `https://example.com/gotify`. A URL ending in `/message` is also accepted. Required when enabled. |
| `token` | string | none | Gotify application token stored directly in TOML. Supported, but `token_env` is preferable for secrets. |
| `token_env` | string | none | Environment variable containing the Gotify application token. If the variable is set, it takes precedence over `token`. |
| `mentions` | bool | `true` | Send Gotify notifications for incoming channel messages/actions that mention your current nick. |
| `private_messages` | bool | `true` | Send Gotify notifications for incoming private messages/actions. |
| `priority` | integer | `5` | Gotify message priority. Gotify clients use priority when deciding how prominently to present a notification. |
| `timeout_seconds` | integer | `5` | HTTP timeout for a Gotify request. Values `<= 0` are reset to `5`. |

Example:

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

Copperline posts JSON to Gotify's `/message` endpoint and authenticates with the application token. Notifications are sent on a background queue so a slow or offline Gotify server does not block IRC processing or the TUI. If the queue fills while Gotify is unavailable, new notification work is dropped rather than allowing unbounded memory growth.

A private message which also mentions your nick produces only one notification. Mention notifications take precedence. Your own outgoing/echoed messages never trigger Gotify. Notices, server numerics, joins/parts, and other system events do not trigger Gotify in the current build.

## Gotify application token

In the Gotify WebUI, create an application for Copperline and copy its application token. With Gotify 3.x, application tokens are shown when created or rotated, so save it somewhere appropriate at that time.

For fish, a session-only exported variable can be set with:

```fish
set -x COPPERLINE_GOTIFY_TOKEN 'your-gotify-application-token'
./Copperline
```

Or enter it without echoing it on screen:

```fish
read -s -P 'Gotify application token: ' COPPERLINE_GOTIFY_TOKEN
set -x COPPERLINE_GOTIFY_TOKEN $COPPERLINE_GOTIFY_TOKEN
./Copperline
```

Do not use `set -Ux` for the token unless you intentionally want fish to persist it in universal-variable storage.

## Testing Gotify

Once Copperline is running:

```text
/gotify status
/gotify test
```

`/gotify status` reports whether the integration is enabled. `/gotify test` sends a real test notification and reports an HTTP/authentication error back in the current Copperline buffer if it fails.

---

# `[[server]]`

Each IRC network gets its own `[[server]]` table. Multiple server tables may be configured simultaneously.

```toml
[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
auto_connect = true
channels = ["#linux", "#go-nuts"]

[[server]]
name = "oftc"
host = "irc.oftc.net"
tls = true
auto_connect = true
channels = ["#debian"]
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `name` | string | **required** | Unique name Copperline uses to identify this server/network. |
| `host` | string | **required** | IRC server hostname or address. |
| `port` | integer | `6697` with TLS, otherwise `6667` | TCP port. The default is selected after reading `tls`. |
| `tls` | bool | `false` | Connect using TLS. |
| `skip_verify` | bool | `false` | Disable TLS certificate verification. Strongly discouraged except for controlled testing. |
| `password` | string | empty | Traditional IRC `PASS` password. This is not SASL. |
| `password_env` | string | empty | Name of an environment variable containing the IRC `PASS` password. |
| `nick` | string | `[general].nick` | Per-server nickname override. |
| `user` | string | `[general].user` | Per-server IRC username override. |
| `real_name` | string | `[general].real_name` | Per-server real-name/GECOS override. |
| `auto_connect` | bool | `false` | Connect to this network when Copperline starts. |
| `channels` | array of strings | empty | Channels to join after IRC registration completes. |
| `caps` | array of strings | empty | Additional IRCv3 capabilities to request in addition to Copperline's built-in capability set. |

## TLS and ports

Typical TLS configuration:

```toml
[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
port = 6697
```

Because `6697` is already the default when `tls = true`, this is equivalent:

```toml
[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
```

Without TLS, an omitted port defaults to `6667`.

When TLS is enabled, Copperline requires TLS 1.2 or newer.

### `skip_verify`

```toml
skip_verify = true
```

turns off TLS certificate verification. This makes man-in-the-middle attacks possible and should normally remain `false`.

## Automatic connection and reconnection

```toml
auto_connect = true
```

causes Copperline to connect when the application starts. A configured server with `auto_connect = false` remains available in the UI and can be connected manually with `/connect`.

Once a connection is desired, Copperline reconnects after a disconnect using `[general].reconnect_seconds`. `/disconnect` clears that desired-connection state and stops reconnecting until `/connect` is used again.

## Autojoin channels

```toml
channels = ["#linux", "#go-nuts", "#copperline"]
```

Copperline sends JOIN after the IRC connection has completed registration.

A channel being listed in the sidebar does not by itself mean the server accepted the JOIN:

- `· #channel` — configured/open buffer, but not confirmed joined
- `✓ #channel` — JOIN confirmed by server state

Channel keys are not currently configurable in the `channels` array.

---

# Traditional IRC `PASS`

`password` and `password_env` configure the IRC `PASS` command sent during server registration. This is separate from SASL.

Inline password:

```toml
[[server]]
name = "private"
host = "irc.example.net"
tls = true
password = "secret"
```

Environment variable:

```toml
password_env = "PRIVATE_IRC_PASSWORD"
```

For fish:

```fish
set -x PRIVATE_IRC_PASSWORD 'secret'
```

For bash/zsh:

```bash
export PRIVATE_IRC_PASSWORD='secret'
```

If `password_env` names a non-empty environment variable, that value takes precedence over `password`. If the variable is missing or empty, Copperline falls back to the inline `password` value.

Environment variables are preferable to storing passwords directly in TOML.

---

# `[server.sasl]`

SASL is optional and configured separately for each `[[server]]` entry.

If no `[server.sasl]` table is present, Copperline does not configure SASL for that server.

## SASL PLAIN

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

Available SASL fields:

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `mechanism` | string | empty, or inferred as `plain` in some cases | SASL mechanism. Copperline recognizes `plain` and `external`. |
| `username` | string | empty | Authentication username for SASL PLAIN. |
| `password` | string | empty | Inline SASL PLAIN password. |
| `password_env` | string | empty | Environment variable containing the SASL PLAIN password. |
| `identity` | string | empty | SASL EXTERNAL authorization identity. |

If `mechanism` is omitted but any of `username`, `password`, or `password_env` is present, Copperline automatically selects `plain`.

For SASL PLAIN, `password_env` takes precedence over `password` when the named environment variable is non-empty.

SASL PLAIN should be used over TLS so the authentication exchange is protected in transit.

## No SASL

Simply omit the SASL table:

```toml
[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
auto_connect = true
channels = ["#linux"]
```

TLS and SASL are independent. `tls = true` does not require SASL.

## SASL EXTERNAL

Copperline recognizes:

```toml
[server.sasl]
mechanism = "external"
identity = "myIdentity"
```

However, the current TOML schema does **not** expose TLS client-certificate/key paths. Therefore SASL EXTERNAL is recognized by the IRC layer but is not yet fully configurable for the common client-certificate workflow. SASL PLAIN is the fully configurable SASL mechanism in the current release.

Unknown SASL mechanism names are not configured.

---

# IRCv3 capability configuration

Copperline has a built-in set of IRCv3 capabilities it is willing to negotiate. A server's `caps` array adds additional names to that set.

Example:

```toml
[[server]]
name = "example"
host = "irc.example.net"
tls = true
caps = [
    "example.org/custom-cap",
    "draft/something-new",
]
```

`caps` does **not** replace the built-in list. Duplicates are automatically collapsed.

The current built-in request set is:

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

A capability in this list is only negotiated if the IRC server advertises/supports it.

The current config format can add capabilities but cannot disable individual built-in capability names.

Use `/caps` inside Copperline to see the capabilities actually negotiated on the selected network.

---

# Path expansion

Copperline expands `~` and paths beginning with `~/` for:

- `[general].log_dir`
- `[dcc].download_dir`
- file paths passed to DCC SEND through Copperline

Example:

```toml
log_dir = "~/IRCLogs"
download_dir = "~/Downloads/IRC"
```

Other string fields are used as written.

---

# Minimal configurations

## Libera.Chat without SASL

```toml
[general]
nick = "myNick"
mouse = true

[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
auto_connect = true
channels = ["#linux"]
```

## Libera.Chat with SASL PLAIN

```toml
[general]
nick = "myNick"
mouse = true

[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
auto_connect = true
channels = ["#linux"]

[server.sasl]
mechanism = "plain"
username = "myNick"
password_env = "LIBERA_IRC_PASSWORD"
```

## Multiple networks

```toml
[general]
nick = "myNick"
mouse = true

[[server]]
name = "libera"
host = "irc.libera.chat"
tls = true
auto_connect = true
channels = ["#linux", "#go-nuts"]

[[server]]
name = "oftc"
host = "irc.oftc.net"
tls = true
auto_connect = true
channels = ["#debian"]
```

## Logging disabled

Logging defaults to enabled. To run Copperline without writing IRC logs to disk:

```toml
[general]
logging = false
```

This does not disable the live in-memory scrollback buffer.

## DCC disabled

DCC defaults to disabled, so either omit `[dcc]` entirely or write:

```toml
[dcc]
enabled = false
```

---

# Current configuration limitations

The following are not currently configurable in TOML:

- keybindings
- per-channel logging enable/disable
- per-channel log directories
- log rotation
- proxy/SOCKS settings
- TLS client certificate/key files
- channel keys in the autojoin list
- disabling individual built-in IRCv3 capabilities
- UI panel sizes/layout

These are potential future configuration additions rather than undocumented options.
