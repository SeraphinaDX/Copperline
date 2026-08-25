-- trout.lua
--
-- Adds the classic IRC /trout command.
--
-- Usage:
--   /trout nickname
--
-- This sends a real CTCP ACTION, equivalent to typing:
--   /me *slaps nickname around a bit with a large trout*.

copperline.command("trout", function(ctx)
    if ctx.server == "" then
        copperline.print("/trout: no active server")
        return
    end

    if ctx.target == "" or ctx.target == "*server*" then
        copperline.print("/trout: select a channel or private query first")
        return
    end

    local nick = ctx.args:match("^%s*(%S+)%s*$")
    if not nick then
        copperline.print("usage: /trout nickname")
        return
    end

    local action = "*slaps " .. nick .. " around a bit with a large trout*."

    -- CTCP ACTION is what IRC clients send for /me. Copperline's Lua API does
    -- not need a special helper for it; the raw IRC form is straightforward.
    copperline.raw(
        "PRIVMSG " .. ctx.target .. " :\001ACTION " .. action .. "\001",
        ctx.server
    )
end)
