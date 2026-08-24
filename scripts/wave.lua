-- wave.lua
--
-- Adds /wave. In a channel or private query it sends a small greeting.

copperline.command("wave", function(ctx)
    if ctx.server == "" then
        copperline.print("/wave: no active server")
        return
    end

    if ctx.target == "" or ctx.target == "*server*" then
        copperline.print("/wave: select a channel or private query first")
        return
    end

    copperline.message(ctx.target, "o/", ctx.server)
end)
