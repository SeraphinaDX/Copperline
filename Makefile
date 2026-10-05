APP_ID := ca.cerberusgames.copperline
GUI_BIN := copperline-gui
PREFIX ?= $(HOME)/.local
BINDIR := $(PREFIX)/bin
APPLICATIONS_DIR := $(PREFIX)/share/applications
ICON_DIR := $(PREFIX)/share/icons/hicolor/512x512/apps

.PHONY: gui tui install-gui uninstall-gui

gui:
	go build -o $(GUI_BIN) ./cmd/copperline-gui

tui:
	go build -o copperline ./cmd/copperline

install-gui: gui
	install -Dm755 $(GUI_BIN) $(BINDIR)/$(GUI_BIN)
	install -Dm644 cmd/copperline-gui/Icon.png $(ICON_DIR)/$(APP_ID).png
	install -Dm644 packaging/linux/$(APP_ID).desktop $(APPLICATIONS_DIR)/$(APP_ID).desktop
	@command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database $(APPLICATIONS_DIR) || true
	@command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -f -t $(PREFIX)/share/icons/hicolor >/dev/null 2>&1 || true
	@echo "Installed Copperline GUI desktop integration under $(PREFIX)."
	@echo "On KDE/Wayland, fully close any running Copperline window and launch it again."

uninstall-gui:
	rm -f $(BINDIR)/$(GUI_BIN)
	rm -f $(ICON_DIR)/$(APP_ID).png
	rm -f $(APPLICATIONS_DIR)/$(APP_ID).desktop
	@command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database $(APPLICATIONS_DIR) || true
	@command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -f -t $(PREFIX)/share/icons/hicolor >/dev/null 2>&1 || true
