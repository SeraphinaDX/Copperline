-- away.lua
--
-- Adds simple away aliases using raw IRC.
--
--   /brb
--   /brb making tea
--   /back

copperline.command("brb", function(ctx)
    if ctx.server == "" then
        copperline.print("/brb: no active server")
        return
    end

    local reason = ctx.args
    if reason == "" then
        reason = "Away"
    end

    copperline.raw("AWAY :" .. reason, ctx.server)
    copperline.print("Away: " .. reason, ctx.server, "*server*")
end)

copperline.command("back", function(ctx)
    if ctx.server == "" then
        copperline.print("/back: no active server")
        return
    end

    copperline.raw("AWAY", ctx.server)
    copperline.print("Away status cleared", ctx.server, "*server*")
end)
