package irc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"copperline/internal/config"
	"copperline/internal/model"

	"github.com/BurntSushi/toml"
)

const (
	maxIgnoreRules = 256
	maxIgnoreMask  = 512
)

// ignoreRule stores user-entered scope and mask separately from its compiled
// matcher. Empty Server means all networks; empty Channel means all targets,
// including private messages. The unexported matcher is not written to TOML.
type ignoreRule struct {
	Mask    string `toml:"mask"`
	Server  string `toml:"server,omitempty"`
	Channel string `toml:"channel,omitempty"`
	matcher *regexp.Regexp
}

type ignoreFile struct {
	Rules []ignoreRule `toml:"ignore"`
}

// compileIgnoreMask treats ordinary masks literally except for '*'. Only the
// explicit re: prefix enables regular-expression syntax and partial matching.
func compileIgnoreMask(mask string) (*regexp.Regexp, error) {
	if len(mask) == 0 {
		return nil, errors.New("ignore mask cannot be empty")
	}
	if len(mask) > maxIgnoreMask {
		return nil, fmt.Errorf("ignore mask is longer than %d bytes", maxIgnoreMask)
	}
	pattern := ""
	if strings.HasPrefix(strings.ToLower(mask), "re:") {
		pattern = mask[3:]
		if pattern == "" {
			return nil, errors.New("regular expression cannot be empty")
		}
	} else {
		pattern = "^" + strings.ReplaceAll(regexp.QuoteMeta(mask), `\*`, ".*") + "$"
	}
	matcher, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid ignore expression: %w", err)
	}
	return matcher, nil
}

// loadIgnoreRules validates the whole file before making any rule available.
// A missing file is a new list; an invalid file is an error, not an empty list.
func loadIgnoreRules(path string) ([]ignoreRule, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	var stored ignoreFile
	if _, err := toml.DecodeFile(path, &stored); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	if len(stored.Rules) > maxIgnoreRules {
		return nil, fmt.Errorf("load %s: too many rules (maximum %d)", path, maxIgnoreRules)
	}
	for i := range stored.Rules {
		matcher, err := compileIgnoreMask(stored.Rules[i].Mask)
		if err != nil {
			return nil, fmt.Errorf("load %s: rule %d: %w", path, i+1, err)
		}
		stored.Rules[i].matcher = matcher
	}
	return stored.Rules, nil
}

// saveIgnoreRules writes beside the destination so rename can replace it
// atomically. Callers hold ignoreMu and restore their previous in-memory rules
// on failure, keeping acknowledged edits consistent with persisted state.
func saveIgnoreRules(path string, rules []ignoreRule) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create ignore-list directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".ignores-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary ignore list: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("secure temporary ignore list: %w", err)
	}
	if err := toml.NewEncoder(tmp).Encode(ignoreFile{Rules: rules}); err != nil {
		cleanup()
		return fmt.Errorf("encode ignore list: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync ignore list: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close ignore list: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace ignore list: %w", err)
	}
	return nil
}

// ManageIgnore executes the text after /ignore using the invoking buffer as
// the default scope. Returning lines instead of broadcasting them lets direct
// and relay clients display results only in the requesting client's buffer.
func (m *Manager) ManageIgnore(server, channel, command string) ([]string, error) {
	if m.ignoreLoadErr != nil {
		return nil, m.ignoreLoadErr
	}
	fields := strings.Fields(command)
	if len(fields) == 0 || strings.EqualFold(fields[0], "list") {
		return m.ignoreList(), nil
	}

	switch strings.ToLower(fields[0]) {
	case "add", "addreplace":
		if len(fields) < 2 {
			return nil, errors.New("usage: /ignore add mask [--channel] [--global]")
		}
		mask := fields[1]
		matcher, err := compileIgnoreMask(mask)
		if err != nil {
			return nil, err
		}
		rule := ignoreRule{Mask: mask, Server: server, matcher: matcher}
		global, explicitScope := false, false
		for i := 2; i < len(fields); i++ {
			switch {
			case fields[i] == "--global":
				global = true
				rule.Server, rule.Channel = "", ""
			case fields[i] == "--channel":
				explicitScope = true
				if !model.IsChannel(channel) {
					return nil, errors.New("--channel requires a channel buffer")
				}
				rule.Channel = channel
			case strings.HasPrefix(fields[i], "--channel="):
				explicitScope = true
				rule.Channel = strings.TrimPrefix(fields[i], "--channel=")
				if !model.IsChannel(rule.Channel) {
					return nil, errors.New("--channel value must be a channel name")
				}
			case strings.HasPrefix(fields[i], "--server="):
				explicitScope = true
				rule.Server = strings.TrimPrefix(fields[i], "--server=")
				if rule.Server == "" {
					return nil, errors.New("--server value cannot be empty")
				}
			default:
				return nil, fmt.Errorf("unknown ignore option %q", fields[i])
			}
		}
		if global && explicitScope {
			return nil, errors.New("--global cannot be combined with --server or --channel")
		}
		if rule.Channel != "" && rule.Server == "" {
			return nil, errors.New("a channel-scoped ignore also needs a server scope")
		}
		replace := strings.EqualFold(fields[0], "addreplace")
		return m.addIgnoreRule(rule, replace)

	case "remove", "del":
		if len(fields) != 2 {
			return nil, errors.New("usage: /ignore remove number|mask")
		}
		return m.removeIgnoreRule(server, fields[1])

	case "clear":
		if len(fields) != 1 {
			return nil, errors.New("usage: /ignore clear")
		}
		return m.clearIgnoreRules()

	default:
		return nil, errors.New("usage: /ignore [list|add|remove|clear]")
	}
}

func (m *Manager) ignoreList() []string {
	m.ignoreMu.RLock()
	defer m.ignoreMu.RUnlock()
	if len(m.ignoreRules) == 0 {
		return []string{"ignore list is empty"}
	}
	lines := []string{fmt.Sprintf("ignore rules (%d):", len(m.ignoreRules))}
	for i, rule := range m.ignoreRules {
		scope := "all networks"
		if rule.Server != "" {
			scope = rule.Server + " / all channels"
		}
		if rule.Channel != "" {
			scope = rule.Server + " / " + rule.Channel
		}
		lines = append(lines, fmt.Sprintf("%d. %s [%s]", i+1, rule.Mask, scope))
	}
	return lines
}

func (m *Manager) addIgnoreRule(rule ignoreRule, replace bool) ([]string, error) {
	m.ignoreMu.Lock()
	defer m.ignoreMu.Unlock()
	previous := append([]ignoreRule(nil), m.ignoreRules...)
	for i := len(m.ignoreRules) - 1; i >= 0; i-- {
		existing := m.ignoreRules[i]
		if strings.EqualFold(existing.Mask, rule.Mask) && strings.EqualFold(existing.Server, rule.Server) && strings.EqualFold(existing.Channel, rule.Channel) {
			if !replace {
				return nil, errors.New("that ignore rule already exists")
			}
			m.ignoreRules = append(m.ignoreRules[:i], m.ignoreRules[i+1:]...)
		}
	}
	if len(m.ignoreRules) >= maxIgnoreRules {
		m.ignoreRules = previous
		return nil, fmt.Errorf("ignore list is full (maximum %d rules)", maxIgnoreRules)
	}
	m.ignoreRules = append(m.ignoreRules, rule)
	if err := saveIgnoreRules(config.ExpandPath(m.cfg.General.IgnoreFile), m.ignoreRules); err != nil {
		m.ignoreRules = previous
		return nil, err
	}
	return []string{"ignored " + rule.Mask + " [" + ignoreScope(rule) + "]"}, nil
}

func (m *Manager) removeIgnoreRule(server, selector string) ([]string, error) {
	m.ignoreMu.Lock()
	defer m.ignoreMu.Unlock()
	index := -1
	if n, err := strconv.Atoi(selector); err == nil {
		if n < 1 || n > len(m.ignoreRules) {
			return nil, fmt.Errorf("no ignore rule numbered %d", n)
		}
		index = n - 1
	} else {
		for i, rule := range m.ignoreRules {
			if strings.EqualFold(rule.Mask, selector) && (rule.Server == "" || strings.EqualFold(rule.Server, server)) {
				if index != -1 {
					return nil, errors.New("more than one rule has that mask; remove it by number")
				}
				index = i
			}
		}
		if index == -1 {
			return nil, fmt.Errorf("no ignore rule matches %q", selector)
		}
	}
	previous := append([]ignoreRule(nil), m.ignoreRules...)
	removed := m.ignoreRules[index]
	m.ignoreRules = append(m.ignoreRules[:index], m.ignoreRules[index+1:]...)
	if err := saveIgnoreRules(config.ExpandPath(m.cfg.General.IgnoreFile), m.ignoreRules); err != nil {
		m.ignoreRules = previous
		return nil, err
	}
	return []string{"removed ignore rule for " + removed.Mask}, nil
}

func (m *Manager) clearIgnoreRules() ([]string, error) {
	m.ignoreMu.Lock()
	defer m.ignoreMu.Unlock()
	if len(m.ignoreRules) == 0 {
		return []string{"ignore list is already empty"}, nil
	}
	previous := append([]ignoreRule(nil), m.ignoreRules...)
	m.ignoreRules = nil
	if err := saveIgnoreRules(config.ExpandPath(m.cfg.General.IgnoreFile), nil); err != nil {
		m.ignoreRules = previous
		return nil, err
	}
	return []string{fmt.Sprintf("cleared %d ignore rules", len(previous))}, nil
}

func ignoreScope(rule ignoreRule) string {
	if rule.Server == "" {
		return "all networks"
	}
	if rule.Channel == "" {
		return rule.Server + " / all channels"
	}
	return rule.Server + " / " + rule.Channel
}

// shouldIgnore filters conversation content, not membership/moderation state.
// It does not erase messages already retained before a rule was added.
func (m *Manager) shouldIgnore(msg model.Message) bool {
	if msg.Nick == "" {
		return false
	}
	switch msg.Kind {
	case model.KindMessage, model.KindAction, model.KindNotice, model.KindDCC:
	case model.KindSystem:
		if !strings.HasPrefix(msg.Text, "CTCP ") {
			return false
		}
	default:
		return false
	}
	return m.matchesIgnoreIdentity(msg.Server, msg.Target, msg.Nick, msg.User, msg.Host)
}

func (m *Manager) matchesIgnoreIdentity(server, channel, nick, user, host string) bool {
	candidates := []string{nick}
	if user != "" || host != "" {
		candidates = append(candidates, user+"@"+host, nick+"!"+user+"@"+host)
	}
	if host != "" {
		candidates = append(candidates, host)
	}
	m.ignoreMu.RLock()
	defer m.ignoreMu.RUnlock()
	for _, rule := range m.ignoreRules {
		if rule.Server != "" && !strings.EqualFold(rule.Server, server) {
			continue
		}
		if rule.Channel != "" && !strings.EqualFold(rule.Channel, channel) {
			continue
		}
		for _, candidate := range candidates {
			if rule.matcher != nil && rule.matcher.MatchString(candidate) {
				return true
			}
		}
	}
	return false
}
