# AnsiChat Release Notes

[Back to README](README.md)

## Fixed in 0.2.38

Unread counts now exclude messages discarded by the local history limit, so relay catch-up and long-running buffers cannot show more unread messages than they retain. Update the client to receive this fix; client and relay history limits remain independent.

## Added in 0.2.37

Connect to an unconfigured IRC server with `/quick irc.example.org`. TLS with
certificate verification is the default (port 6697). Use
`/quick irc.example.org 6697 MyNick` or `/quick irc.example.org:6697 MyNick`
for a custom port/nickname, or `/quick -plain irc.example.org 6667` for plaintext.
Without a nickname override, the connection uses the IRC-owning process's
`[general]` identity. Network passwords, SASL, client certificates, and autojoin
channels are not inherited.

The command selects the new server's status buffer; use `/join #channel` next.
Sessions remain available for `/connect` and `/disconnect` until the process
exits, without writing to TOML. Repeating the same endpoint and nickname reuses
the session. In relay mode the connection is made on the relay and shared by
attached clients; update both client and relay to 0.2.37 to use `/quick`.
The wire protocol remains version 2; older relays reject the new command.

## Fixed in 0.2.36

Mentions now highlight channels you are not currently viewing in the terminal
and graphical interfaces. The highlight clears when you open the channel or
mark the mention read.

## Fixed in 0.2.35

Reduce relay-client memory overhead by releasing the large startup history decoder buffer, avoiding full server-state copies for routine lookups, and caching visible transcript cells. Relay checkpoints now stream message JSON to reduce temporary allocations. Add opt-in `-memprofile` diagnostics. History limits remain configurable per buffer and independently on each client and relay.

## Fixed in 0.2.34

Fixed a terminal-input decoder loop when a UTF-8 character is split across TTY reads. Even a short website paste containing curly quotes could stall before its paste-end marker reached the editor. UTF-8 input is now decoded as a continuous stream, so incomplete characters wait for the next read instead of spinning. This complements the large-input performance changes in 0.2.33. Update the terminal client; the relay does not need an update.

## Fixed in 0.2.33

Terminal input bursts are inserted in small batches, including paste methods that do not send bracketed-paste markers. The input renders a bounded window around the cursor while retaining the complete editable draft. This avoids character-by-character copying and full-draft redraws for large website pastes. Cursor movement, configured shortcuts, and bracketed-paste line handling remain available.

Long ordinary chat drafts also use the existing background paste sender when Enter is pressed, so waiting for IRC delivery does not block typing. Slash commands keep their normal behavior. This is a terminal-client update; the relay protocol remains unchanged.

## Fixed in 0.2.31

`/flex` keeps longer GPU model names within its existing GPU field budget instead
of cutting each model at 64 bytes. Linux uses compact AMD, NVIDIA, and Intel vendor
labels, preserving the full reported device description and Radeon model suffix.
The complete message remains bounded to one IRC line.

## New in 0.2.30

`/flex` now includes GPU information when available and leaves off load averages.
Linux uses local `lspci` model names, with PCI vendor/device IDs as a fallback if
pciutils is unavailable. Windows uses native display-adapter APIs; macOS uses
the local System Profiler. Android reads GPU model files exposed by Adreno/Mali
drivers when permitted. Distinct GPU names are included on systems with multiple
adapters. Missing or restricted GPU information is omitted. Hardware queries
have bounded execution time and output, and summaries remain a single IRC line.

## Fixed in 0.2.29

IRC reconnects now rejoin channels successfully joined during the current session,
including channels outside the configured autojoin list. Keys supplied through
`/join #channel key` are kept in memory for reconnects. Explicit `/part`, `/leave`,
and `/close` requests, and kicks of your own nickname, remove the channel from the
session's reconnect list. Failed joins and merely open buffers are not treated
as joined channels.

The shared IRC backend handles this in direct mode and on the relay server, so
TUI, desktop GUI, and Android GUI clients benefit. In relay mode, update the relay
server too. This session state does not change the TOML autojoin list or persist
across a process restart.

## New in 0.2.28

Channel lists sort alphabetically by default in the TUI, desktop GUI, and Android
GUI. Sorting ignores letter case, keeps each network's status buffer first, and
also orders private queries. TUI numbered jumps and next/previous/unread navigation
follow the displayed order. Network order, selected conversations, and drafts are
preserved.

To retain creation order, configure this locally on each client:

```toml
[general]
sort_channels_alphabetically = false
```

Omitting the setting or using `true` enables alphabetical sorting.

## New in 0.2.27

`/flex` posts a compact system summary to the current channel or query in every
frontend: TUI, desktop GUI, and Android GUI. Command names are case-insensitive,
so `/Flex` works too. The summary includes AnsiChat version, OS/release and
architecture, CPU model and logical CPU count, GPU, RAM, and system uptime when
available. Android also includes the device model when available.
Windows uses native system/registry APIs; macOS reports total RAM. Other platforms
retain the portable OS, architecture, and CPU-count baseline. Missing/restricted
stats are omitted.

The statistics describe the device running AnsiChat, including in relay mode.
The command takes no arguments and uses the normal message delivery path, so a
failed send keeps `/flex` in the TUI input or restores the GUI draft. It does not
include hostname, username, IP addresses, serial numbers, or account details.

Example (values vary by device):

```text
AnsiChat 0.2.34 | OS: CachyOS (Linux 6.18)/amd64 | CPU: Intel Core i5 — 8 logical CPUs | GPU: Intel UHD Graphics 620 | RAM: 4.2/15.5 GiB used | Up: 2d 3h 12m
```

## Fixed in 0.2.26

Android sending keeps the composer focused and the keyboard open, whether using
the on-screen Enter key or the Send control inside the message box. Opening the
composer scrolls to the latest messages and keeps them above the keyboard as the
viewport changes. Navigation controls hide while typing to give chat more room.

## New in 0.2.25

`/map` requests the IRC network topology from the active server in every AnsiChat frontend: the terminal UI, desktop GUI, and Android GUI. MAP replies are shown in the network's server/status buffer. The command accepts no arguments and uses the relay's existing raw IRC path, so no relay protocol bump is required.

## Fixed in 0.2.18

PART visibility is now configurable independently from JOIN visibility. Use
`show_part_messages = false` under `[general]` to hide `nick left` lines
without affecting membership tracking, typing cleanup, or unread activity.
The older `show_parts` spelling is also accepted for compatibility, while
`show_part_messages` takes precedence if both appear.

## Fixed in 0.2.17

`/close [reason]` now sends PART before closing a channel buffer, including when
attached to a relay. A failed leave request keeps the buffer open and displays
the error. PART echoes, queued messages, and relay replay cannot reopen the
closed channel; use `/join #channel` to open it again. In relay mode, leaving
affects the shared IRC connection and therefore all attached clients. Closing
a private conversation still only closes its local buffer; a new PM can reopen it.
Configured autojoin channels are unchanged.

## Fixed in 0.2.16

Restored the configurable CTCP VERSION response, which was accidentally removed
in the later slash-command update along with its configuration field. The newer
slash commands remain available. Rebuild and restart the IRC-owning instance
(the relay server in relay mode) to apply the fix; existing `ctcp_version`
settings work again. The binary version is corrected to `0.2.16`.

## New in 0.2.15

Customize the text returned to `/ctcp yourNick VERSION` in your existing TOML configuration:

```toml
[general]
ctcp_version = "My IRC client"
```

Omit `ctcp_version` to retain `AnsiChat by Britney Lozza`, or set it to `""` to disable VERSION replies. Leading/trailing whitespace is trimmed. The value must be a single line without NUL or CTCP delimiter characters. This changes only the CTCP VERSION response, not `--version` or other CTCP replies. Requests still appear in the conversation even when replies are disabled.

In direct mode, configure your local client. In relay mode, configure the **relay server**, which sends the IRC replies; changing only an attached client's config has no effect. Restart the relevant instance after changing this setting.

## New in 0.2.14

- **Per-buffer drafts:** channels, private conversations, and server buffers each retain their own unsent text, cursor position, and multiline paste state when you switch away. Drafts stay on the current machine for the current session and survive relay reconnects. Sending clears that buffer's draft; `/close` discards it.
- **`/clear`:** clears the current buffer's visible scrollback and unread marker. Other buffers, your input draft, log files, and relay history remain intact. Cleared rows stay hidden across buffer switches and relay reconnects in the same session. Restarting AnsiChat can load history again.
- **Custom highlights:** add literal words or phrases alongside your nickname. Matches are case-insensitive, respect word boundaries, and apply to other users' messages and `/me` actions. They use the existing mention color and Gotify mention setting; your own messages do not trigger alerts.
- **URL grabber:** optionally collect HTTP(S) links from messages, actions, and notices, including your own. Configure it on the relay server to collect while clients are disconnected, or on your local machine in direct mode.

Add these settings to the existing sections of your TOML config:

```toml
[general]
highlight_words = ["AnsiChat", "alternateNick", "server down"]

[urls]
file = "~/.local/state/ansichat/urls.jsonl"
```

For relay-owned Gotify alerts, set `highlight_words` on the relay. Clients receive those highlight flags and can add local highlight words for display. Client-only words cannot trigger notifications owned by the relay. Restart the relevant instance after changing the config.

URL collection is disabled when `urls.file` is blank or omitted. The file is append-only JSON Lines, with `time`, `server`, `target`, `nick`, and `url` on each line; timestamps include their timezone. Parent directories are created automatically. Links are never opened or fetched. Ignored messages and relay attachment replay are not collected. Each URL is saved only once across all buffers, keeping its first sender/channel/time. The saved file is indexed at startup, so duplicate suppression survives reconnects and restarts, including repeated IRC history. Scheme and hostname casing are ignored; paths, queries, and fragments remain distinct. Use a separate file for each collector process.

URL writes run in a bounded background queue. Slow or failing storage cannot block IRC delivery; queue overflow drops links, and the first collection error appears in the first configured network's server buffer. Later messages retry writes. If an existing file cannot be read or contains malformed records, collection is disabled until the file is fixed and AnsiChat restarted, so it cannot append duplicates unknowingly. `/clear` does not erase collected URLs.

## Fixed in 0.2.13

Nicknames containing IRC-valid square brackets now render literally in the user list. In particular, a nickname such as `[aruna]` no longer loses its closing bracket to gotui's inline-style parser.

## Fixed in 0.2.12

IRC server connections now prefer IPv6 regardless of the DNS resolver's address ordering. AnsiChat starts with the server's IPv6 addresses, then begins an IPv4 fallback after 250 milliseconds so IPv4-only servers and networks with unavailable or broken IPv6 still connect promptly. An explicit IPv4 or IPv6 address in `host` continues to use that address family directly.

## New in 0.2.11

`/ignore` provides a persistent, relay-aware ignore list with network, channel, and global scopes. Plain masks support case-insensitive `*` wildcards; advanced rules may use Go/RE2 expressions prefixed with `re:`. Ignored chat, actions, notices, typing indicators, CTCP lines, and DCC offers are removed before history and notifications, while membership and moderation state remains intact. Command results are displayed in the buffer where `/ignore` was entered.

Common forms are `/ignore add Alice`, `/ignore add Alice --channel`, `/ignore add Alice --global`, `/ignore list`, `/ignore remove Alice`, `/ignore remove 2`, and `/ignore clear`. Rules live in `[general].ignore_file`, which defaults to `~/.config/ansichat/ignores.toml`; relay users configure this on the relay server.

## Fixed in 0.2.10

`/whois nick` now displays readable WHOIS details in the buffer where the command was entered. Responses work in direct and relay-client modes, including account, idle time, channels, secure-connection status, and CertFP information. Raw WHOIS events remain available to Lua scripts.

## New in 0.2.9

Per-server `tls_cert_file` and `tls_key_file` settings enable TLS client certificates, CertFP, and SASL EXTERNAL. The two settings may point to the same combined PEM file. Invalid or incomplete certificate configuration is rejected with a clear server-specific error; see [CONFIGURATION.md](CONFIGURATION.md#liberachat-with-certfp-and-sasl-external) for the Libera.Chat setup and registration procedure.

## Fixed in 0.2.8

JOIN, PART, and QUIT notices remain visible in the transcript but no longer increment the unread activity number beside a buffer. Other events and messages continue to count normally. Update clients and the relay so this metadata survives relay delivery and retained-history restoration.

## Fixed in 0.2.7

Channel buffer names now ignore capitalization: `#C` and `#c` share one transcript, unread count, and sidebar entry. This also applies when restoring relay history. Update and restart clients; update the relay as well so its retained history uses the same channel identity.

## Fixed in 0.2.6

Multiline paste is collected as a single draft instead of interpreting each pasted newline as Enter. Newlines appear as `↵` in the input; press Enter to send the draft as ordinary chat text. Delivery runs in the background, one bounded message at a time, with progress in the status bar. You can keep editing a new draft, switch channels, or use `/paste cancel` to stop after the current line. IRC flood protection still limits transmission speed.

A stopped or failed paste retains unconfirmed/unsent text. `/paste restore` puts that remainder back into the input and selects its original target; check the last attempted line before retrying because delivery can be uncertain. `/paste discard` removes the retained remainder, and `/paste status` shows progress. Pasted slash-prefixed lines in a multiline draft are sent as text. Long lines are split on UTF-8 boundaries and empty lines are skipped. Large single-line pastes use the same background sender. Paste collection is limited to 1 MiB; an oversized paste is rejected as a whole. This requires terminal bracketed-paste support.

The relay also rejects overlapping message sends promptly rather than accumulating rate-limit waits against its shared IRC connection. Update clients for paste handling and the relay for the send guard; protocol remains 2.

## New in 0.2.5

`Alt+B` toggles the channel list and `Alt+U` toggles the user list independently. Chat uses the freed space, and typing and live updates continue. Visibility is temporary for the current session; the user list still automatically hides on narrow terminals. Rebind these with `[keybindings].toggle_channel_list` and `toggle_user_list`.

## Fixed in 0.2.4

When Gotify is enabled on the relay, the relay sends eligible mention/private-message alerts whether or not clients are attached. Client alerts are suppressed to avoid duplicates. If relay Gotify is disabled, the oldest eligible attached client remains responsible for notifications.

In relay-client mode, `/gotify status` and `/gotify test` now query/test the relay's notifier. Status includes HTTP delivery successes, failures, queue drops, and the last attempt/success/error. Counters reset when AnsiChat restarts; HTTP success means Gotify accepted the alert, not that a phone displayed it. Failed alerts are not automatically retried. Update the relay and clients for remote diagnostics; protocol remains 2.

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
