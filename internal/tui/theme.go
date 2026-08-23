package tui

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"copperline/internal/config"
	"copperline/internal/model"

	ui "github.com/metaspartan/gotui/v5"
	"github.com/metaspartan/gotui/v5/widgets"
)

type uiTheme struct {
	cfg config.ThemeConfig

	background ui.Color
	panel      ui.Color
	foreground ui.Color
	muted      ui.Color
	border     ui.Color
	title      ui.Color
	selectedFG ui.Color
	selectedBG ui.Color
	topic      ui.Color
	input      ui.Color
	cursorFG   ui.Color
	cursorBG   ui.Color
	statusFG   ui.Color
	statusBG   ui.Color
}

func newUITheme(cfg config.ThemeConfig) uiTheme {
	return uiTheme{
		cfg:        cfg,
		background: colorSpec(cfg.Background, ui.ColorBlack),
		panel:      colorSpec(cfg.Panel, ui.ColorBlack),
		foreground: colorSpec(cfg.Foreground, ui.ColorWhite),
		muted:      colorSpec(cfg.Muted, ui.ColorGrey),
		border:     colorSpec(cfg.Border, ui.ColorDarkCyan),
		title:      colorSpec(cfg.Title, ui.ColorLightCyan),
		selectedFG: colorSpec(cfg.SelectedFG, ui.ColorBlack),
		selectedBG: colorSpec(cfg.SelectedBG, ui.ColorOrange),
		topic:      colorSpec(cfg.Topic, ui.ColorWheat),
		input:      colorSpec(cfg.Input, ui.ColorWhite),
		cursorFG:   colorSpec(cfg.CursorFG, ui.ColorBlack),
		cursorBG:   colorSpec(cfg.CursorBG, ui.ColorLightCyan),
		statusFG:   colorSpec(cfg.StatusFG, ui.ColorBlack),
		statusBG:   colorSpec(cfg.StatusBG, ui.ColorLightCyan),
	}
}

func colorSpec(spec string, fallback ui.Color) ui.Color {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if c, ok := ui.StyleParserColorMap[spec]; ok {
		return c
	}
	if len(spec) == 7 && spec[0] == '#' {
		r, errR := strconv.ParseInt(spec[1:3], 16, 32)
		g, errG := strconv.ParseInt(spec[3:5], 16, 32)
		b, errB := strconv.ParseInt(spec[5:7], 16, 32)
		if errR == nil && errG == nil && errB == nil {
			return ui.NewRGBColor(int32(r), int32(g), int32(b))
		}
	}
	return fallback
}

func (t uiTheme) applyBlock(w *widgets.List) {
	w.BackgroundColor = t.panel
	w.BorderStyle = ui.Style{Fg: t.border, Bg: t.background}
	w.TitleStyle = ui.Style{Fg: t.title, Bg: t.panel, Modifier: ui.ModifierBold}
	w.TextStyle = ui.Style{Fg: t.foreground, Bg: t.panel}
	w.SelectedStyle = ui.Style{Fg: t.selectedFG, Bg: t.selectedBG, Modifier: ui.ModifierBold}
}

func (t uiTheme) applyParagraph(w *widgets.Paragraph, fg ui.Color) {
	w.BackgroundColor = t.panel
	w.BorderStyle = ui.Style{Fg: t.border, Bg: t.background}
	w.TitleStyle = ui.Style{Fg: t.title, Bg: t.panel, Modifier: ui.ModifierBold}
	w.TextStyle = ui.Style{Fg: fg, Bg: t.panel}
}

func (t uiTheme) applyInput(w *widgets.Input) {
	w.BackgroundColor = t.panel
	w.BorderStyle = ui.Style{Fg: t.border, Bg: t.background}
	w.TitleStyle = ui.Style{Fg: t.title, Bg: t.panel, Modifier: ui.ModifierBold}
	w.TextStyle = ui.Style{Fg: t.input, Bg: t.panel}
	w.CursorStyle = ui.Style{Fg: t.cursorFG, Bg: t.cursorBG, Modifier: ui.ModifierBold}
}

func (t uiTheme) applyStatus(w *widgets.Paragraph) {
	w.BackgroundColor = t.statusBG
	w.TextStyle = ui.Style{Fg: t.statusFG, Bg: t.statusBG, Modifier: ui.ModifierBold}
}

func styled(text, color string) string {
	if text == "" {
		return ""
	}
	// gotui uses [text](fg:color) markup. Do not wrap strings containing
	// brackets, because IRC nicknames are allowed to contain them.
	if strings.ContainsAny(text, "[]") {
		return text
	}
	return fmt.Sprintf("[%s](fg:%s)", text, color)
}

func (t uiTheme) nickColor(nick string) string {
	if len(t.cfg.NickColors) == 0 {
		return t.cfg.Foreground
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(nick)))
	return t.cfg.NickColors[int(h.Sum32())%len(t.cfg.NickColors)]
}

func (t uiTheme) formatMessage(m model.Message, layout string) string {
	stamp := styled(m.Time.Local().Format(layout), t.cfg.Timestamp)
	switch m.Kind {
	case model.KindAction:
		return fmt.Sprintf("%s %s %s", stamp, styled("* "+m.Nick, t.cfg.Action), m.Text)
	case model.KindNotice:
		return fmt.Sprintf("%s %s %s", stamp, styled("-"+m.Nick+"-", t.cfg.Notice), m.Text)
	case model.KindSystem:
		return fmt.Sprintf("%s %s %s", stamp, styled("***", t.cfg.System), m.Text)
	case model.KindError:
		return fmt.Sprintf("%s %s %s", stamp, styled("!!!", t.cfg.Error), m.Text)
	case model.KindDCC:
		return fmt.Sprintf("%s %s %s", stamp, styled("DCC", t.cfg.DCC), m.Text)
	default:
		nick := styled("<"+m.Nick+">", t.nickColor(m.Nick))
		return fmt.Sprintf("%s %s %s", stamp, nick, m.Text)
	}
}
