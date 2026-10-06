package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// The Android surface remains touch-first. Desktop uses permanent, compact
// sidebars when space allows and falls back to lightweight navigation controls
// in narrow windows.
type responsiveLayout struct {
	chat            fyne.CanvasObject
	channels, users fyne.CanvasObject
	mobile          bool
	scroll          *container.Scroll
	editing         func() bool
}

func (l *responsiveLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if l.mobile {
		// Do not impose a desktop-height minimum on the keyboard's viewport.
		return fyne.NewSize(320, l.chat.MinSize().Height)
	}
	return fyne.NewSize(320, 400)
}

func (l *responsiveLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	toolbar := objects[0]
	desktopSidebars := !l.mobile && size.Width >= 800
	editing := l.mobile && l.editing != nil && l.editing()
	keepLatest := l.mobile && l.scroll != nil && (editing ||
		l.scroll.Offset.Y+l.scroll.Size().Height >= l.scroll.Content.Size().Height-24)
	var oldScrollSize fyne.Size
	if l.scroll != nil {
		oldScrollSize = l.scroll.Size()
	}

	y := float32(0)
	if (l.mobile || !desktopSidebars) && !editing {
		toolbar.Show()
		toolbar.Move(fyne.NewPos(0, 0))
		toolbar.Resize(fyne.NewSize(size.Width, toolbar.MinSize().Height))
		y = toolbar.MinSize().Height + 4
	} else {
		toolbar.Hide()
	}

	height := max(float32(0), size.Height-y)
	if desktopSidebars {
		const (
			channelWidth = float32(210)
			userWidth    = float32(150)
			gap          = float32(8)
		)
		l.channels.Show()
		l.users.Show()
		l.channels.Move(fyne.NewPos(0, y))
		l.channels.Resize(fyne.NewSize(channelWidth, height))
		l.users.Move(fyne.NewPos(size.Width-userWidth, y))
		l.users.Resize(fyne.NewSize(userWidth, height))
		l.chat.Move(fyne.NewPos(channelWidth+gap, y))
		l.chat.Resize(fyne.NewSize(size.Width-channelWidth-userWidth-(gap*2), height))
		return
	}

	l.channels.Hide()
	l.users.Hide()
	l.chat.Move(fyne.NewPos(0, y))
	l.chat.Resize(fyne.NewSize(size.Width, height))
	if keepLatest && oldScrollSize != l.scroll.Size() {
		// Scroll after nested layouts have resized the transcript. Keeping the
		// old offset would leave the newest lines underneath the open keyboard.
		l.scroll.ScrollToBottom()
	}
}
