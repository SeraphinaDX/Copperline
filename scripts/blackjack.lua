-- blackjack.lua
--
-- A small per-user IRC blackjack game hosted by Copperline.
--
-- Anyone in a channel (or a private query) can play independently:
--   !blackjack   start a new hand
--   !hit         take another card
--   !stand       stand and let the dealer finish
--   !bjhelp      show the commands
--
-- Game state is kept in memory and resets when Lua is reloaded or Copperline
-- exits. Each game is keyed by server + channel/query + nickname, so several
-- users can play at the same time without sharing a deck or hand.
--
-- Rules are intentionally simple: no betting, split, insurance, or double
-- down. The dealer stands on 17.

local games = {}

local suits = { "♠", "♥", "♦", "♣" }
local ranks = {
    { name = "A",  value = 11 },
    { name = "2",  value = 2 },
    { name = "3",  value = 3 },
    { name = "4",  value = 4 },
    { name = "5",  value = 5 },
    { name = "6",  value = 6 },
    { name = "7",  value = 7 },
    { name = "8",  value = 8 },
    { name = "9",  value = 9 },
    { name = "10", value = 10 },
    { name = "J",  value = 10 },
    { name = "Q",  value = 10 },
    { name = "K",  value = 10 },
}

math.randomseed(os.time())

local function trim(s)
    return (s:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function card_name(card)
    return card.rank .. card.suit
end

local function hand_name(hand)
    local out = {}
    for i, card in ipairs(hand) do
        out[i] = card_name(card)
    end
    return "[" .. table.concat(out, " ") .. "]"
end

local function hand_value(hand)
    local total = 0
    local aces = 0

    for _, card in ipairs(hand) do
        total = total + card.value
        if card.rank == "A" then
            aces = aces + 1
        end
    end

    while total > 21 and aces > 0 do
        total = total - 10
        aces = aces - 1
    end

    return total
end

local function is_blackjack(hand)
    return #hand == 2 and hand_value(hand) == 21
end

local function new_deck()
    local deck = {}

    for _, suit in ipairs(suits) do
        for _, rank in ipairs(ranks) do
            deck[#deck + 1] = {
                rank = rank.name,
                value = rank.value,
                suit = suit,
            }
        end
    end

    -- Fisher-Yates shuffle.
    for i = #deck, 2, -1 do
        local j = math.random(i)
        deck[i], deck[j] = deck[j], deck[i]
    end

    return deck
end

local function draw(game, hand)
    local card = table.remove(game.deck)
    hand[#hand + 1] = card
    return card
end

local function game_key(server, target, nick)
    return server:lower() .. "\031" .. target:lower() .. "\031" .. nick:lower()
end

local function send(server, target, text)
    copperline.message(target, text, server)
end

local function finish_game(key, game, nick, server, target, prefix)
    while hand_value(game.dealer) < 17 do
        draw(game, game.dealer)
    end

    local player_total = hand_value(game.player)
    local dealer_total = hand_value(game.dealer)
    local result

    if dealer_total > 21 then
        result = "dealer busts — you win!"
    elseif player_total > dealer_total then
        result = "you win!"
    elseif player_total < dealer_total then
        result = "dealer wins."
    else
        result = "push."
    end

    local intro = ""
    if prefix and prefix ~= "" then
        intro = prefix .. " "
    end

    send(
        server,
        target,
        nick .. ": " .. intro ..
        "you " .. hand_name(game.player) .. " = " .. tostring(player_total) ..
        "; dealer " .. hand_name(game.dealer) .. " = " .. tostring(dealer_total) ..
        " — " .. result
    )

    games[key] = nil
end

local function start_game(key, nick, server, target)
    if games[key] then
        local game = games[key]
        send(
            server,
            target,
            nick .. ": you already have a hand: " .. hand_name(game.player) ..
            " = " .. tostring(hand_value(game.player)) .. ". Use !hit or !stand."
        )
        return
    end

    local game = {
        deck = new_deck(),
        player = {},
        dealer = {},
    }

    -- Deal in normal blackjack order.
    draw(game, game.player)
    draw(game, game.dealer)
    draw(game, game.player)
    draw(game, game.dealer)

    local player_blackjack = is_blackjack(game.player)
    local dealer_blackjack = is_blackjack(game.dealer)

    if player_blackjack or dealer_blackjack then
        local result
        if player_blackjack and dealer_blackjack then
            result = "both have blackjack — push."
        elseif player_blackjack then
            result = "BLACKJACK — you win!"
        else
            result = "dealer has blackjack."
        end

        send(
            server,
            target,
            nick .. ": you " .. hand_name(game.player) .. " = " ..
            tostring(hand_value(game.player)) .. "; dealer " ..
            hand_name(game.dealer) .. " = " .. tostring(hand_value(game.dealer)) ..
            " — " .. result
        )
        return
    end

    games[key] = game

    send(
        server,
        target,
        nick .. ": you " .. hand_name(game.player) .. " = " ..
        tostring(hand_value(game.player)) .. "; dealer shows [" ..
        card_name(game.dealer[1]) .. " ??]. Use !hit or !stand."
    )
end

local function hit(key, nick, server, target)
    local game = games[key]
    if not game then
        send(server, target, nick .. ": no active hand. Use !blackjack to start one.")
        return
    end

    local card = draw(game, game.player)
    local total = hand_value(game.player)

    if total > 21 then
        send(
            server,
            target,
            nick .. " hits " .. card_name(card) .. ": " .. hand_name(game.player) ..
            " = " .. tostring(total) .. " — bust. Dealer wins."
        )
        games[key] = nil
        return
    end

    if total == 21 then
        finish_game(
            key,
            game,
            nick,
            server,
            target,
            "hit " .. card_name(card) .. " and made 21."
        )
        return
    end

    send(
        server,
        target,
        nick .. " hits " .. card_name(card) .. ": " .. hand_name(game.player) ..
        " = " .. tostring(total) .. "."
    )
end

local function stand(key, nick, server, target)
    local game = games[key]
    if not game then
        send(server, target, nick .. ": no active hand. Use !blackjack to start one.")
        return
    end

    finish_game(key, game, nick, server, target, "stands.")
end

copperline.on("message", function(ev)
    local text = trim(ev.text or ""):lower()
    if text ~= "!blackjack" and text ~= "!hit" and text ~= "!stand" and text ~= "!bjhelp" then
        return
    end

    local server = ev.server or ""
    local target = ev.target or ""
    local nick = ev.source or ""

    if server == "" or target == "" or nick == "" then
        return
    end

    if text == "!bjhelp" then
        send(server, target, nick .. ": blackjack commands: !blackjack, !hit, !stand")
        return
    end

    local key = game_key(server, target, nick)

    if text == "!blackjack" then
        start_game(key, nick, server, target)
    elseif text == "!hit" then
        hit(key, nick, server, target)
    elseif text == "!stand" then
        stand(key, nick, server, target)
    end
end)
