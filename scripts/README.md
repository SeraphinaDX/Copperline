# Copperline example Lua scripts

This directory contains small example scripts for Copperline's embedded Lua API.
They are **examples in the source tree** and are not loaded automatically from
this directory.

Copperline normally loads scripts from:

```text
~/.config/copperline/scripts/
```

Copy only the examples you want to use, for example:

```sh
mkdir -p ~/.config/copperline/scripts
cp scripts/wave.lua ~/.config/copperline/scripts/
```

Then reload Lua inside Copperline:

```text
/lua reload
```

Use `/lua list` to see which files loaded successfully.

## Included examples

- `hello.lua` — adds `/hello`; demonstrates a local custom command and `copperline.print`.
- `wave.lua` — adds `/wave`; demonstrates sending a message to the active channel/query.
- `mentions.lua` — adds `/mentioncount`; demonstrates message/action hooks and session state.
- `away.lua` — adds `/brb [reason]` and `/back`; demonstrates `copperline.raw`.
- `connection.lua` — reacts to numeric `001`; demonstrates a raw IRC numeric hook.
- `debug-events.lua` — demonstrates the catch-all `irc` hook. Debug output is disabled by default because it can be very noisy.
- `trout.lua` — adds `/trout nickname`; sends the classic large-trout CTCP ACTION.
- `blackjack.lua` — hosts per-user blackjack in channels/queries with `!blackjack`, `!hit`, and `!stand`.

For the complete API and event reference, see [`../SCRIPTING.md`](../SCRIPTING.md).
