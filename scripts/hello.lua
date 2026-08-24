-- hello.lua
--
-- A minimal custom command. This writes only to Copperline; it does not send
-- anything to IRC.

copperline.command("hello", function(ctx)
    local target = ctx.target

    if target == "" or target == "*server*" then
        copperline.print("Hello from Lua!", ctx.server, "*server*")
        return
    end

    copperline.print(
        "Hello from Lua in " .. target .. "!",
        ctx.server,
        target
    )
end)
