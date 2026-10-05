package gui

import (
	"copperline/internal/config"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"image/color"
	"strconv"
	"strings"
)

type copperTheme struct {
	cfg     config.ThemeConfig
	compact bool
}

func (t copperTheme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	value := ""
	switch name {
	case theme.ColorNameBackground:
		value = t.cfg.Background
	case theme.ColorNameForeground:
		value = t.cfg.Foreground
	case theme.ColorNameInputBackground, theme.ColorNameButton:
		value = t.cfg.Panel
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		value = t.cfg.Title
	case theme.ColorNameSelection:
		value = t.cfg.SelectedBG
	case theme.ColorNameDisabled:
		value = t.cfg.Muted
	case theme.ColorNameError:
		value = t.cfg.Error
	case "mention":
		value = t.cfg.Mention
	case "action":
		value = t.cfg.Action
	case "system":
		value = t.cfg.System
	case "notice":
		value = t.cfg.Notice
	}
	if len(value) == 7 && strings.HasPrefix(value, "#") {
		if n, err := strconv.ParseUint(value[1:], 16, 32); err == nil {
			return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
		}
	}
	return theme.DefaultTheme().Color(name, v)
}
func (t copperTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (t copperTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (t copperTheme) Size(n fyne.ThemeSizeName) float32 {
	if !t.compact {
		return theme.DefaultTheme().Size(n)
	}
	switch n {
	case theme.SizeNamePadding:
		return 3
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameScrollBar:
		return 9
	case theme.SizeNameScrollBarSmall:
		return 2
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameButtonRadius:
		return 10
	case theme.SizeNameSelectionRadius:
		return 7
	}
	return theme.DefaultTheme().Size(n)
}
