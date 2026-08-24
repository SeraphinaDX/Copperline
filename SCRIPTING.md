# Copperline Lua Scripting

Copperline embeds Lua through [GopherLua](https://github.com/yuin/gopher-lua). Scripts can add slash commands, react to IRC events, send messages and notices, issue raw IRC commands, and write informational lines into Copperline buffers.

Lua support is intended for personal automation and client customization without requiring Copperline itself to be rebuilt.

## Script directory

Scripts are loaded from:

```text
~/.config/copperline/scripts/
```

All files ending in `.lua` are loaded in alphabetical filename order when Copperline starts.

The directory can be changed in `config.toml`:

```toml
[scripting]
enabled = true
dir = "~/.config/copperline/scripts"
```

Scripting is enabled by default. If the directory does not exist, Copperline creates it with mode `0700`. An empty directory has no effect on normal client behavior.

To disable Lua completely:

```toml
[scripting]
enabled = false
```

## Lua commands inside Copperline

Copperline provides a built-in `/lua` command:

```text
/lua list
/lua reload
/lua eval <code>
```

`/lua list` lists the scripts that loaded successfully.

`/lua reload` discards the current Lua VM, creates a fresh one, and reloads every `.lua` file from the script directory. This means removed globals, commands, and hooks do not linger from an older version of a script.

`/lua eval` evaluates a short piece of Lua in the current VM. It is mainly useful while developing scripts, for example:

```text
/lua eval copperline.print("hello from Lua")
```

## The `copperline` API

Copperline exposes one global Lua table named `copperline`.

### `copperline.command(name, callback)`

Registers a new slash command.

```lua
copperline.command("hello", function(ctx)
    copperline.print("Hello from Lua, " .. ctx.nick)
end)
```

The command is then available as:

```text
/hello
```

The callback receives a context table:

```text
ctx.server   current server name
ctx.target   current channel/query/server buffer
ctx.nick     your current nickname on that server
ctx.command  command name
ctx.args     everything typed after the command
```

Example using arguments:

```lua
copperline.command("slap", function(ctx)
    if ctx.args == "" then
        copperline.print("usage: /slap nick")
        return
    end

    copperline.message(
        ctx.target,
        ctx.args .. " gets slapped with a large trout",
        ctx.server
    )
end)
```

Copperline's built-in slash commands take precedence over Lua commands. A script cannot replace `/join`, `/msg`, `/quit`, `/lua`, or another built-in command.

### `copperline.on(event, callback)`

Registers an event hook.

There are two layers of hooks. `irc` receives every raw IRC event:

```lua
copperline.on("irc", function(ev)
    -- receives every raw IRC event
end)
```

A hook may also target a particular raw IRC command:

```lua
copperline.on("privmsg", function(ev)
    -- receives PRIVMSG only
end)
```

IRC command names are lower-case in Lua. Examples include:

```text
privmsg
notice
join
part
quit
kick
nick
topic
tagmsg
001
433
```

Numeric replies can therefore be hooked by their three-digit command name.

Copperline also emits semantic client events for message types that are more convenient after parsing:

```text
message
action
dcc
startup
```

For example, an IRC CTCP ACTION is available as `action`, so a script does not need to decode the CTCP payload itself:

```lua
copperline.on("action", function(ev)
    copperline.print(ev.source .. " performed /me: " .. ev.text)
end)
```

Each event table contains:

```text
ev.server   Copperline server name
ev.command  lower-case IRC command
ev.source   nickname/server source
ev.params   Lua array containing IRC parameters
ev.tags     table of IRCv3 message tags
ev.time     Unix timestamp
ev.target   first IRC parameter, when present
ev.text     final IRC parameter when the event has at least two parameters
ev.raw      true for raw IRC events, false for semantic Copperline events
```

For example:

```lua
copperline.on("privmsg", function(ev)
    if ev.text and ev.text:lower():match("hello copperline") then
        copperline.print(
            "Lua noticed a greeting from " .. ev.source,
            ev.server,
            ev.target
        )
    end
end)
```

There is also a `startup` event. It runs after all scripts have been loaded or reloaded:

```lua
copperline.on("startup", function()
    copperline.print("My scripts are loaded")
end)
```

### `copperline.print(text [, server [, target]])`

Writes a Copperline system line to a buffer without sending anything to IRC.

```lua
copperline.print("script loaded")
```

If `server` and `target` are omitted, the currently selected buffer is used.

Explicit destination:

```lua
copperline.print("hello #go-nuts", "libera", "#go-nuts")
```

Lines written with `copperline.print` go through Copperline's normal buffer and logging path.

### `copperline.message(target, text [, server])`

Sends an IRC `PRIVMSG`.

```lua
copperline.message("#go-nuts", "hello from Lua")
```

The current server is used when the third argument is omitted.

```lua
copperline.message("#debian", "hello OFTC", "oftc")
```

### `copperline.notice(target, text [, server])`

Sends an IRC `NOTICE`.

```lua
copperline.notice("SomeNick", "hello", "libera")
```

### `copperline.raw(line [, server])`

Sends a raw IRC protocol line.

```lua
copperline.raw("WHO #go-nuts")
```

or:

```lua
copperline.raw("MODE #go-nuts +b", "libera")
```

This is deliberately powerful. Invalid raw IRC can disconnect you, trigger server flood protection, or perform destructive IRC operations. Scripts using `raw` should be treated with the same care as commands typed manually with `/raw`.

### `copperline.active()`

Returns information about the currently selected buffer:

```lua
local ctx = copperline.active()

copperline.print(
    "server=" .. ctx.server ..
    " target=" .. ctx.target ..
    " nick=" .. ctx.nick
)
```

The returned table contains:

```text
server
target
nick
```

### `copperline.nick([server])`

Returns your current nickname on a server.

```lua
local mynick = copperline.nick()
```

or:

```lua
local libera_nick = copperline.nick("libera")
```

## Complete example

Save this as:

```text
~/.config/copperline/scripts/example.lua
```

```lua
copperline.on("startup", function()
    copperline.print("example.lua loaded")
end)

copperline.command("wave", function(ctx)
    if ctx.target == "*server*" then
        copperline.print("Select a channel or query first")
        return
    end

    copperline.message(ctx.target, "o/", ctx.server)
end)

copperline.on("privmsg", function(ev)
    local mynick = copperline.nick(ev.server)

    if ev.text and mynick ~= "" and ev.text:lower():match(mynick:lower()) then
        copperline.print(
            "Lua mention hook: " .. ev.source .. " mentioned you",
            ev.server,
            ev.target
        )
    end
end)
```

Then reload without restarting Copperline:

```text
/lua reload
```

and use:

```text
/wave
```

## Concurrency model

Copperline's IRC connections, DCC work, notifications, timers, and UI already use Go concurrency. The Lua VM is intentionally different: all Lua callbacks are serialized through one scripting goroutine.

This is important because one GopherLua state should not be called concurrently from unrelated IRC goroutines. Copperline therefore queues IRC hook events and executes Lua callbacks one at a time.

IRC processing itself is not allowed to block waiting for Lua. The event queue is bounded; if a script falls extremely far behind, Copperline drops scripting hook events rather than stalling the IRC connection.

Custom slash commands are executed synchronously from the UI because their result is user initiated. A badly written command callback that never returns can therefore make the UI appear stuck even though the IRC networking goroutines continue to run.

## Script errors

A script that fails while loading is reported in Copperline as a Lua error. Copperline never installs a half-loaded replacement VM: if `/lua reload` fails, the previous working set of scripts remains active. On initial startup, a failed load leaves scripting with no newly loaded script set until the files are corrected and reloaded.

Errors raised by an event hook are shown in the active Copperline buffer and do not stop later IRC processing.

For custom commands, the error is shown as a normal Copperline error line.

After correcting a script, run:

```text
/lua reload
```

## Security

Lua scripts run locally as part of Copperline and should be considered trusted code. They can send IRC messages and raw protocol commands using your connected identity.

Do not install random scripts without reading them first.

The scripting API is intended as a convenience boundary, not as a security sandbox.

## Lua compatibility

Copperline uses GopherLua, which implements Lua 5.1 semantics with selected additions. Scripts should generally be written as Lua 5.1 code.
