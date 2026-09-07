# Copperline SSH Relay

Copperline can keep IRC connections alive on one machine and let other Copperline clients attach to that session over SSH. The SSH transport is implemented **inside Copperline** with Go's `golang.org/x/crypto/ssh` package. Copperline does not invoke the system `ssh` client or `sshd`.

The relay has three modes:

- `direct` — normal Copperline. The TUI connects directly to IRC, exactly as before.
- `server` — headless Copperline relay. It owns the IRC connections, retains bounded in-memory scrollback, writes the normal logs, and listens for Copperline clients on its embedded SSH server.
- `client` — local Copperline TUI. It connects only to a Copperline relay over SSH; it does not open IRC connections itself.

## Why SSH

SSH gives the relay encrypted transport, public-key authentication and server host-key verification without deploying a TLS certificate or depending on a certificate authority.

Copperline relay builds require **Go 1.26+** and `golang.org/x/crypto v0.56.0` or newer. The v0.56.0 release contains September 2026 SSH denial-of-service fixes, so the relay intentionally does not use an older x/crypto release just to preserve the former Go 1.24 build requirement.

Copperline's relay SSH endpoint is deliberately narrow. It accepts only the private `copperline-relay@copperline` SSH channel used by the relay protocol. It does **not** provide a shell, PTY, command execution, TCP forwarding, SFTP or a general-purpose SSH login.

Relay authentication is public-key-only. By default Copperline keeps both the server host key and relay-client key under `~/.config/copperline/`, separate from OpenSSH configuration.

## 1. Configure the relay server

The relay server keeps the ordinary `[[server]]` IRC sections because it is the machine that actually connects to IRC.

```toml
[relay]
mode = "server"
listen = "0.0.0.0:2222"
user = "copperline"
host_key = "~/.config/copperline/relay_host_ed25519"
authorized_keys = "~/.config/copperline/relay_authorized_keys"

[general]
nick = "myNick"
history_lines = 2000
logging = true

[[server]]
name = "libera"
host = "irc.libera.chat"
port = 6697
tls = true
auto_connect = true
channels = ["#go-nuts", "#linux"]
```

Start it normally:

```sh
./Copperline -config=relay-server.toml
```

On first start Copperline creates an Ed25519 SSH host key automatically and prints a line similar to:

```text
Copperline relay host key fingerprint: SHA256:...
```

Save that fingerprint for the client configuration.

The `authorized_keys` file is also created if it does not exist. Copperline rereads it on every authentication attempt, so you can add a client key while the relay is running; no relay restart is required.

The default relay listen address is `127.0.0.1:2222` for safety. Set an externally reachable address such as `0.0.0.0:2222` only when you intend to accept remote clients, and protect that port with the host firewall as appropriate.

## 2. Configure a relay client

A relay client may omit every `[[server]]` section. Server/channel state comes from the relay.

```toml
[relay]
mode = "client"
address = "relay.example.com:2222"
user = "copperline"
private_key = "~/.config/copperline/relay_client_ed25519"
host_key_fingerprint = "SHA256:the-fingerprint-printed-by-the-server"

[general]
nick = "myNick"
history_lines = 2000
```

The first time a relay client runs, if `private_key` does not exist Copperline creates an Ed25519 keypair itself:

```text
~/.config/copperline/relay_client_ed25519
~/.config/copperline/relay_client_ed25519.pub
```

The first connection will be rejected until that public key is authorized. Copy the contents of the `.pub` file into the relay server's configured `authorized_keys` file, then start the client again. Because the relay server reloads authorized keys for each login, it does not need to be restarted.

If you deliberately want to reuse another SSH private key, point `private_key` at it. Encrypted private keys are supported with an environment variable:

```toml
private_key = "/path/to/key"
private_key_passphrase_env = "COPPERLINE_RELAY_KEY_PASSPHRASE"
```

Copperline does not use `ssh-agent`; this is intentional so relay behavior does not depend on a system SSH setup.

## Host-key verification

The relay client pins the SHA-256 fingerprint of the relay server host key:

```toml
host_key_fingerprint = "SHA256:..."
```

If the fingerprint changes unexpectedly, Copperline refuses the connection. This protects against connecting to the wrong relay or an active man-in-the-middle attack without requiring a certificate authority.

For temporary testing only, host-key verification can be disabled explicitly:

```toml
insecure_skip_host_key_check = true
```

Do not use that setting for a normal remote relay.


## Force reconnect from the TUI

Relay-client mode adds a dedicated reconnect control to the bottom-right of the TUI. It is rendered separately from ordinary status text, so startup progress, jump mode, connection notices, or a long status line cannot overwrite it:

```text
[⟳ RECONNECT (Alt+R)]
```

It is both clickable and keyboard-accessible. The default binding is configurable:

```toml
[keybindings]
relay_reconnect = "Alt+R"
```

Both paths call the same reconnect operation. Copperline closes only the local SSH attachment and opens a fresh connection to the relay; the relay process and its IRC network connections are left running. Retained relay history is de-duplicated on reattach, while messages received during the disconnected gap are still accepted.

The relay client also sends a lightweight protocol heartbeat every five seconds. If a heartbeat or another relay request cannot be acknowledged within ten seconds, Copperline closes that SSH attachment and marks the relay unavailable instead of trusting the last cached IRC snapshot. The reconnect control changes to a warning form while the attachment is down.

### Message safety during a relay failure

For ordinary channel/query chat, Copperline does **not** clear the input field until the relay has acknowledged the send request. If the SSH attachment fails or times out, the exact text remains in the input and Copperline reports that delivery was not confirmed. Because a transport can fail after the relay receives a message but before its acknowledgement reaches the client, check the channel before manually retrying an unconfirmed message to avoid a duplicate.

## What stays on the relay server

In `server` mode the relay owns:

- IRC TCP/TLS connections and reconnect loops
- SASL credentials and IRC server passwords
- joined-channel state and nick lists
- IRCv3 capability state
- bounded in-memory message history
- the normal persistent Copperline logs
- DCC network connections and received files

`[general].history_lines` on the relay server controls the maximum retained in-memory message history per buffer that can be replayed to a newly attached client.

## What stays on the relay client

In `client` mode the local Copperline process owns the presentation layer:

- the TUI and terminal behavior
- theme and keybindings
- local input history
- local Lua scripts
- local Gotify notifications while that client is attached

When a client attaches, the relay sends its retained in-memory messages first, then switches to live streaming. Replayed messages are displayed as normal conversation text but are not re-logged or re-notified by the client, preventing duplicate logs/notifications every time a client starts.

Multiple Copperline clients can attach to the same relay simultaneously. Messages and IRC state updates are broadcast to every attached client, and commands from any authorized client operate the shared IRC session.

## Commands and detach behavior

Most existing Copperline commands work unchanged because the relay client exposes the same IRC backend operations as direct mode.

Important distinction:

- `/quit` in a relay **client** detaches that local Copperline process. It does **not** stop the relay's IRC connections.
- `/disconnect [server]` deliberately tells the relay to disconnect that IRC network for the shared session.
- `/connect [server]` tells the relay to connect/reconnect that IRC network.

This makes the relay behave like a persistent IRC bouncer rather than like a remote terminal session.

## DCC notes

DCC is owned by the relay server because that is where the IRC/network backend runs. Incoming DCC SEND files are therefore saved in the relay server's configured `download_dir`.

DCC CHAT lines are forwarded back to the requesting Copperline client. For DCC SEND initiated from a relay client, the path currently refers to the **relay server's filesystem**; transferring an arbitrary local client file to the relay before DCC SEND is not part of relay protocol version 1.

## Protocol

The application protocol is versioned independently from SSH. Version 1 uses newline-delimited JSON frames over the private SSH channel. It carries:

- retained and live `model.Message` records
- IRC connection/event records
- coalesced server/channel/nick-list/topic/capability snapshots
- request/response frames for IRC commands
- DCC CHAT lines

Large IRC state bursts such as NAMES/WHO are coalesced before snapshots are sent so they do not cause a flood of SSH round trips or redraws.
