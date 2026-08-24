package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	General   GeneralConfig   `toml:"general"`
	Theme     ThemeConfig     `toml:"theme"`
	DCC       DCCConfig       `toml:"dcc"`
	Gotify    GotifyConfig    `toml:"gotify"`
	Scripting ScriptingConfig `toml:"scripting"`
	Servers   []ServerConfig  `toml:"server"`
}

type GeneralConfig struct {
	Nick             string `toml:"nick"`
	User             string `toml:"user"`
	RealName         string `toml:"real_name"`
	Logging          *bool  `toml:"logging"`
	LogDir           string `toml:"log_dir"`
	Timestamp        string `toml:"timestamp"`
	Mouse            bool   `toml:"mouse"`
	ShowTyping       *bool  `toml:"show_typing"`
	SendTyping       *bool  `toml:"send_typing"`
	ShowJoinMessages *bool  `toml:"show_join_messages"`
	HistoryLines     int    `toml:"history_lines"`
	LogBacklogLines  *int   `toml:"log_backlog_lines"`
	ReconnectSecs    int    `toml:"reconnect_seconds"`
}

func (g GeneralConfig) LoggingEnabled() bool {
	return g.Logging == nil || *g.Logging
}

func (g GeneralConfig) ShowTypingEnabled() bool {
	return g.ShowTyping == nil || *g.ShowTyping
}

func (g GeneralConfig) SendTypingEnabled() bool {
	return g.SendTyping == nil || *g.SendTyping
}

func (g GeneralConfig) ShowJoinMessagesEnabled() bool {
	return g.ShowJoinMessages == nil || *g.ShowJoinMessages
}

func (g GeneralConfig) LogBacklogLinesValue() int {
	if g.LogBacklogLines == nil {
		return 10
	}
	if *g.LogBacklogLines < 0 {
		return 0
	}
	return *g.LogBacklogLines
}

type ThemeConfig struct {
	Background string   `toml:"background"`
	Panel      string   `toml:"panel"`
	Foreground string   `toml:"foreground"`
	Muted      string   `toml:"muted"`
	Border     string   `toml:"border"`
	Title      string   `toml:"title"`
	SelectedFG string   `toml:"selected_fg"`
	SelectedBG string   `toml:"selected_bg"`
	Timestamp  string   `toml:"timestamp"`
	Server     string   `toml:"server"`
	Channel    string   `toml:"channel"`
	Query      string   `toml:"query"`
	Unread     string   `toml:"unread"`
	Mention    string   `toml:"mention"`
	Notice     string   `toml:"notice"`
	Action     string   `toml:"action"`
	System     string   `toml:"system"`
	Error      string   `toml:"error"`
	DCC        string   `toml:"dcc"`
	Topic      string   `toml:"topic"`
	Input      string   `toml:"input"`
	CursorFG   string   `toml:"cursor_fg"`
	CursorBG   string   `toml:"cursor_bg"`
	StatusFG   string   `toml:"status_fg"`
	StatusBG   string   `toml:"status_bg"`
	NickColors []string `toml:"nick_colors"`
}

type DCCConfig struct {
	Enabled     bool   `toml:"enabled"`
	DownloadDir string `toml:"download_dir"`
	ListenAddr  string `toml:"listen_addr"`
	AdvertiseIP string `toml:"advertise_ip"`
}

type GotifyConfig struct {
	Enabled         bool   `toml:"enabled"`
	URL             string `toml:"url"`
	Token           string `toml:"token"`
	TokenEnv        string `toml:"token_env"`
	Mentions        *bool  `toml:"mentions"`
	PrivateMessages *bool  `toml:"private_messages"`
	Priority        *int   `toml:"priority"`
	TimeoutSeconds  int    `toml:"timeout_seconds"`
}

func (g GotifyConfig) MentionsEnabled() bool {
	return g.Mentions == nil || *g.Mentions
}

func (g GotifyConfig) PrivateMessagesEnabled() bool {
	return g.PrivateMessages == nil || *g.PrivateMessages
}

func (g GotifyConfig) PriorityValue() int {
	if g.Priority == nil {
		return 5
	}
	return *g.Priority
}

type ScriptingConfig struct {
	Enabled *bool  `toml:"enabled"`
	Dir     string `toml:"dir"`
}

func (s ScriptingConfig) EnabledValue() bool {
	return s.Enabled == nil || *s.Enabled
}

type ServerConfig struct {
	Name        string   `toml:"name"`
	Host        string   `toml:"host"`
	Port        int      `toml:"port"`
	TLS         bool     `toml:"tls"`
	SkipVerify  bool     `toml:"skip_verify"`
	Password    string   `toml:"password"`
	PasswordEnv string   `toml:"password_env"`
	Nick        string   `toml:"nick"`
	User        string   `toml:"user"`
	RealName    string   `toml:"real_name"`
	AutoConnect bool     `toml:"auto_connect"`
	Channels    []string `toml:"channels"`
	Caps        []string `toml:"caps"`
	SASL        SASL     `toml:"sasl"`
}

type SASL struct {
	Mechanism   string `toml:"mechanism"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	PasswordEnv string `toml:"password_env"`
	Identity    string `toml:"identity"`
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "copperline", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "copperline", "config.toml")
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.General.Nick == "" {
		c.General.Nick = "copperline"
	}
	if c.General.User == "" {
		c.General.User = c.General.Nick
	}
	if c.General.RealName == "" {
		c.General.RealName = "Copperline IRC Client"
	}
	if c.General.Timestamp == "" {
		c.General.Timestamp = "15:04"
	}
	if c.General.HistoryLines <= 0 {
		c.General.HistoryLines = 1000
	}
	if c.General.ReconnectSecs <= 0 {
		c.General.ReconnectSecs = 10
	}
	if c.General.LogDir == "" {
		c.General.LogDir = "~/.local/state/copperline/logs"
	}
	c.Theme.applyDefaults()
	if c.DCC.DownloadDir == "" {
		c.DCC.DownloadDir = "~/Downloads"
	}
	if c.DCC.ListenAddr == "" {
		c.DCC.ListenAddr = "0.0.0.0:0"
	}
	if c.Gotify.TimeoutSeconds <= 0 {
		c.Gotify.TimeoutSeconds = 5
	}
	if c.Scripting.Dir == "" {
		c.Scripting.Dir = "~/.config/copperline/scripts"
	}
	for i := range c.Servers {
		s := &c.Servers[i]
		if s.Port == 0 {
			if s.TLS {
				s.Port = 6697
			} else {
				s.Port = 6667
			}
		}
		if s.Nick == "" {
			s.Nick = c.General.Nick
		}
		if s.User == "" {
			s.User = c.General.User
		}
		if s.RealName == "" {
			s.RealName = c.General.RealName
		}
		if s.SASL.Mechanism == "" && (s.SASL.Username != "" || s.SASL.Password != "" || s.SASL.PasswordEnv != "") {
			s.SASL.Mechanism = "plain"
		}
	}
}

func (t *ThemeConfig) applyDefaults() {
	set := func(dst *string, value string) {
		if *dst == "" {
			*dst = value
		}
	}

	// Copperline's built-in theme: dark navy panels with copper/aqua accents.
	set(&t.Background, "#090d16")
	set(&t.Panel, "#101827")
	set(&t.Foreground, "#d8dee9")
	set(&t.Muted, "#77839a")
	set(&t.Border, "#36506b")
	set(&t.Title, "#64d8cb")
	set(&t.SelectedFG, "#071018")
	set(&t.SelectedBG, "#d99058")
	set(&t.Timestamp, "#77839a")
	set(&t.Server, "#64d8cb")
	set(&t.Channel, "#8bd49c")
	set(&t.Query, "#d9a7ff")
	set(&t.Unread, "#f6c177")
	set(&t.Mention, "#ff9ecb")
	set(&t.Notice, "#f6c177")
	set(&t.Action, "#d9a7ff")
	set(&t.System, "#72c7ef")
	set(&t.Error, "#ff6b81")
	set(&t.DCC, "#8bd49c")
	set(&t.Topic, "#f2cc8f")
	set(&t.Input, "#eef1f6")
	set(&t.CursorFG, "#071018")
	set(&t.CursorBG, "#64d8cb")
	set(&t.StatusFG, "#071018")
	set(&t.StatusBG, "#64d8cb")
	if len(t.NickColors) == 0 {
		t.NickColors = []string{
			"#ff7b72", "#f6c177", "#8bd49c", "#64d8cb",
			"#72c7ef", "#d9a7ff", "#ff9ecb", "#d99058",
		}
	}
}

func (c *Config) Validate() error {
	if c.Gotify.Enabled {
		if strings.TrimSpace(c.Gotify.URL) == "" {
			return errors.New("gotify is enabled but [gotify].url is empty")
		}
		if strings.TrimSpace(Secret(c.Gotify.Token, c.Gotify.TokenEnv)) == "" {
			return errors.New("gotify is enabled but no application token is available; set token or token_env")
		}
	}
	if len(c.Servers) == 0 {
		return errors.New("configuration contains no [[server]] entries")
	}
	seen := map[string]bool{}
	for _, s := range c.Servers {
		if strings.TrimSpace(s.Name) == "" {
			return errors.New("every server needs a name")
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate server name %q", s.Name)
		}
		seen[s.Name] = true
		if strings.TrimSpace(s.Host) == "" {
			return fmt.Errorf("server %q has no host", s.Name)
		}
	}
	return nil
}

func ExpandPath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func Secret(value, envName string) string {
	if envName != "" {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return value
}
