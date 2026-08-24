-- debug-events.lua
--
-- Demonstrates the catch-all raw IRC hook. It is disabled by default because
-- printing every IRC event into the active buffer is extremely noisy.
-- Change this to true while developing a script, then /lua reload.

local DEBUG = false

copperline.on("irc", function(ev)
    if not DEBUG then
        return
    end

    local source = ev.source or ""
    local target = ev.target or ""
    local text = ev.text or ""

    copperline.print(
        string.format(
            "IRC %s source=%s target=%s text=%s",
            ev.command or "?",
            source,
            target,
            text
        )
    )
end)
