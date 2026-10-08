# Copperline GUI 0.2.28

One Fyne frontend for desktop and Android, using Copperline's existing SSH relay
client. This milestone is a **relay client**. Start an existing Copperline relay
server using [RELAY.md](RELAY.md); the GUI does not connect directly to IRC.

## First connection

1. Launch the GUI. Its **Settings** screen edits the relay configuration as TOML.
2. Set `address`, `user`, and `host_key_fingerprint`. Copy the fingerprint printed
   by your relay server at startup. Host key verification is required.
3. Click **Copy public key**, append that key to the relay server's configured
   `authorized_keys` file, and restart the relay server.
4. Click **Save and connect**. The next launch connects using this saved profile.

The GUI generates its own Ed25519 identity in app storage. Android needs no shared
storage permission. The key remains the same across launches; clearing app data
or uninstalling removes it. Every device has its own key. The GUI uses this
app-owned identity even if the desktop TOML has a `private_key` field.

Example profile:

```toml
[general]
history_lines = 1000
timestamp = "15:04"
sort_channels_alphabetically = true

[relay]
mode = "client"
address = "relay.example.com:2222"
user = "copperline"
host_key_fingerprint = "SHA256:REPLACE_WITH_YOUR_RELAY_FINGERPRINT"
```

An IPv6 address with a port must have brackets, for example
`address = "[2001:db8::10]:21337"`. Your phone needs a route to the relay address,
including access to the relay's VPN/overlay if it uses one.

The GUI uses Copperline's existing `[theme]` colors. Set JOIN/PART/QUIT visibility
on the **relay server** with `show_join_messages`, `show_part_messages`, and
`show_quit_messages` under `[general]`; these notices do not increment unread
counts. The server controls which notices are generated for attached clients.

## Using chat

Channels and private queries sort alphabetically by name within each network,
ignoring letter case, with the network's status buffer first. This applies to
desktop lists and the Android **Channels** panel. Sorting defaults to on; set
`sort_channels_alphabetically = false` under `[general]` in this client's Settings
to retain creation order.

On a wide desktop window, channels, chat, and users are visible together. Narrow
windows and phones use **Channels** and **Users** panels. Each buffer has its own
in-memory draft. Clicking a user opens a query. The title shows network and target;
channel topics and nick prefixes come from the relay's current snapshot. In a
channel, the topic is an editable field: press **Enter** in it or tap **Set** to
submit a new topic. An empty field clears the topic. The IRC server still decides
whether your nickname has permission to change it. On desktop, the topic editor
occupies a dedicated full-width header row, so its size stays consistent whether
the channel topic is empty, short, or long.

Live messages and relay history appear in the same transcript. Reconnecting
preserves the retained transcript without duplicating replayed messages. Scrolling
up stops automatic scrolling; switching buffers or sending resumes it. Mentions,
actions, notices, and errors use the configured theme colors. Server-status
buffers show connection and WHOIS output.

On desktop, **Tab** in the message box completes nicknames from the current channel. At the start of a message it uses the IRC reply form `Nick: `, and repeated Tab presses cycle matching nicknames.

Enter sends one line; **Send** does the same. A failed or uncertain send restores
the draft, or offers its text for copying if a newer draft already occupies that
buffer. Copperline never automatically retries an uncertain send.

`/flex` posts one compact message with Copperline version, OS/release, architecture,
CPU model and logical CPU count, RAM, uptime, and load averages where available.
It reports this client device, including Android, even when using an SSH relay.
On Android the device model is included when available; macOS reports total RAM.
Restricted or unavailable stats are omitted. Select a channel or query first.

Supported slash commands:

| Command | Usage |
| --- | --- |
| `/join` | `/join #channel [key]` |
| `/part` | `/part [#channel] [reason]` |
| `/msg` | `/msg nick message` |
| `/me` | `/me action` |
| `/notice` | `/notice target message` |
| `/nick` | `/nick nickname` |
| `/topic` | `/topic new topic` in the current channel |
| `/whois` | `/whois nickname` |
| `/flex` | `/flex` — post this device's OS and system stats to the current channel/query |
| `/map` | `/map` — request the IRC network server map; replies appear in the server/status buffer |
| `/raw` | `/raw IRC command` |
| `/clear` | Clear the displayed transcript for this buffer; retain history for replay deduplication |

`/msg`, `/me`, `/notice`, `/flex`, and `/map` use the same frontend-independent dispatcher as the
TUI. Other TUI commands, Lua scripting, DCC controls, text selection, and native
Android notifications are outside this first GUI milestone.

On Android, **Enter** on the on-screen keyboard and the **Send** control inside the
message box both send while keeping the keyboard open. Opening the composer shows
the latest messages above the keyboard; navigation controls are hidden while typing
to leave more room for the conversation. Tap outside the composer or use Android
Back to dismiss the keyboard.

On Android, leaving the foreground detaches the SSH client. Returning reconnects
and replays retained relay history. The relay remains connected to IRC, and its
Gotify notifications continue according to the server configuration. Closing the
GUI never shuts down the relay's IRC sessions. **Reconnect** also works manually;
a dropped connection is shown as disconnected, and unsent messages are not retried.

## Desktop build

Use Go 1.26 or newer. Fyne requires a C compiler and platform graphics development
libraries. On CachyOS/Arch, install `base-devel`, `pkgconf`, `libglvnd`, `libxcursor`,
`libxrandr`, `libxinerama`, `libxi`, `wayland`, and `libxkbcommon`. On Debian/Ubuntu, install `gcc`, `pkg-config`,
`libgl1-mesa-dev`, `xorg-dev`, `libwayland-dev`, and `libxkbcommon-dev`.

From the repository root, a plain development build still works:

```sh
make gui
./copperline-gui -version
./copperline-gui
```

On Linux desktops, especially KDE/Wayland, install the GUI desktop integration
once so the compositor can resolve Copperline's Wayland app ID to the correct
window/taskbar icon:

```sh
make install-gui
```

This installs `copperline-gui` to `~/.local/bin`, the launcher entry as
`~/.local/share/applications/ca.cerberusgames.copperline.desktop`, and the same
`Icon.png` used by Android under the matching hicolor icon name. Fully close any
running Copperline window and launch it again after installing. KWin matches the
Fyne Wayland app ID `ca.cerberusgames.copperline` to that desktop entry; a raw
binary without the desktop entry may otherwise show the generic Wayland icon in
both the taskbar and window decoration.

The GitHub Linux artifact includes the same desktop integration files. After
extracting that artifact, install the prebuilt copy with:

```sh
make install-gui-prebuilt
```

By default, configuration and the key are in Fyne's private application storage
for `ca.cerberusgames.copperline`, separate from the TUI's default configuration.
To edit a particular relay-client TOML file on desktop:

```sh
./copperline-gui -config=/path/to/relay-client.toml
```

The settings Save button updates that specified file. It still uses the GUI's own
key, so authorize the key shown in GUI setup. The `-config=` syntax also works in
fish. The TUI remains buildable independently:

```sh
make tui
```

Windows and macOS use the same `cmd/copperline-gui` entry point with Fyne's normal
platform build prerequisites. For a mobile-shaped desktop window:

```sh
go run -tags mobile ./cmd/copperline-gui
```

## Android APK

Install a JDK, Android SDK and NDK, and make `sdkmanager`/`adb` available. The
repository's **GUI and Android** workflow builds an arm64 debug APK on PRs, main
pushes, and manual runs, and uploads a `Copperline-Android-arm64` artifact. The
same `cmd/copperline-gui/Icon.png` is embedded as the running desktop window icon
and used by Fyne for desktop/Android packaging, so launcher and runtime icons stay
in sync. It also
builds a Linux desktop binary and runs headless race tests. The checked-in
`cmd/copperline-gui/AndroidManifest.xml` explicitly requests `adjustResize` so the
keyboard reduces the chat viewport rather than covering the latest lines. Keep
its version fields aligned with `FyneApp.toml` when bumping releases.

For the same local build, set `ANDROID_HOME` to the SDK directory and
`ANDROID_NDK_HOME` to the installed NDK directory, then run:

```sh
go install fyne.io/tools/cmd/fyne@v1.7.3
cd cmd/copperline-gui
fyne package -os android/arm64 -app-id ca.cerberusgames.copperline -name Copperline -icon Icon.png
adb install -r Copperline.apk
```

Keep the app ID fixed. This is a development APK; store distribution and release
signing are later work. To build other supported Android architectures, change
`android/arm64` to the desired target or `android` for the default architecture set.

Fyne's official [mobile packaging](https://docs.fyne.io/started/mobile/) and
[compilation](https://docs.fyne.io/explore/compiling/) documentation covers SDK/NDK
setup and platform requirements.

## Verification

```sh
go test -race -tags ci ./...
go build ./cmd/copperline-gui
```

The `ci` tag uses Fyne's headless driver for checks on machines without a display.
Tests cover app identity/config persistence, corrupt-key handling, narrow/wide
layouts, buffer drafts, command targets, cancellation during SSH handshake, live
messages arriving before sink installation, and the existing SSH history replay.
Android device testing should cover first-time key authorization, opening a
channel, sending, nick/topic updates, phone suspension/resumption, and reconnect.
