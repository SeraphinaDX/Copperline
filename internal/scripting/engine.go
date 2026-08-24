package scripting

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type Context struct {
	Server string
	Target string
	Nick   string
}

type Event struct {
	Time    time.Time
	Server  string
	Command string
	Source  string
	Params  []string
	Tags    map[string]string
	Raw     bool
}

type Host struct {
	Print       func(server, target, text string)
	Message     func(server, target, text string) error
	Notice      func(server, target, text string) error
	Raw         func(server, line string) error
	Active      func() Context
	CurrentNick func(server string) string
}

type result struct {
	value any
	err   error
}

type request struct {
	fn   func(*runtime) (any, error)
	resp chan result
}

type runtime struct {
	L        *lua.LState
	host     Host
	commands map[string]*lua.LFunction
	hooks    map[string][]*lua.LFunction
	scripts  []string
}

type Engine struct {
	dir      string
	host     Host
	requests chan request
	events   chan Event
	done     chan struct{}
	close    sync.Once
}

func New(dir string, host Host) *Engine {
	e := &Engine{
		dir:      dir,
		host:     host,
		requests: make(chan request),
		events:   make(chan Event, 512),
		done:     make(chan struct{}),
	}
	go e.loop()
	return e
}

func (e *Engine) loop() {
	r := newRuntime(e.host)
	defer func() {
		r.L.Close()
		close(e.done)
	}()
	for {
		select {
		case req, ok := <-e.requests:
			if !ok {
				return
			}
			v, err := req.fn(r)
			req.resp <- result{value: v, err: err}
		case ev := <-e.events:
			r.dispatchEvent(ev)
		}
	}
}

func newRuntime(host Host) *runtime {
	r := &runtime{
		L:        lua.NewState(),
		host:     host,
		commands: make(map[string]*lua.LFunction),
		hooks:    make(map[string][]*lua.LFunction),
	}
	r.installAPI()
	return r
}

func (e *Engine) call(fn func(*runtime) (any, error)) (any, error) {
	resp := make(chan result, 1)
	select {
	case e.requests <- request{fn: fn, resp: resp}:
	case <-e.done:
		return nil, fmt.Errorf("Lua engine is closed")
	}
	select {
	case out := <-resp:
		return out.value, out.err
	case <-e.done:
		return nil, fmt.Errorf("Lua engine is closed")
	}
}

func (e *Engine) Close() {
	e.close.Do(func() {
		close(e.requests)
		<-e.done
	})
}

func (e *Engine) Reload() ([]string, error) {
	v, err := e.call(func(r *runtime) (any, error) {
		candidate := newRuntime(r.host)
		keepCandidate := false
		defer func() {
			if !keepCandidate {
				candidate.L.Close()
			}
		}()

		entries, readErr := os.ReadDir(e.dir)
		if os.IsNotExist(readErr) {
			if err := os.MkdirAll(e.dir, 0o700); err != nil {
				return append([]string(nil), r.scripts...), err
			}
			entries = nil
			readErr = nil
		}
		if readErr != nil {
			return append([]string(nil), r.scripts...), readErr
		}

		var names []string
		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".lua" {
				continue
			}
			names = append(names, entry.Name())
		}
		sort.Strings(names)

		var loadErrs []string
		for _, name := range names {
			path := filepath.Join(e.dir, name)
			if err := candidate.L.DoFile(path); err != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			candidate.scripts = append(candidate.scripts, name)
		}
		if len(loadErrs) > 0 {
			// A failed reload never installs a half-loaded VM. The previous
			// working script set remains active until every file loads cleanly.
			return append([]string(nil), r.scripts...), fmt.Errorf("%s", strings.Join(loadErrs, "; "))
		}

		candidate.dispatchNamed("startup", map[string]lua.LValue{})
		r.L.Close()
		*r = *candidate
		keepCandidate = true
		return append([]string(nil), r.scripts...), nil
	})
	if v == nil {
		return nil, err
	}
	return v.([]string), err
}

func (e *Engine) List() []string {
	v, err := e.call(func(r *runtime) (any, error) {
		return append([]string(nil), r.scripts...), nil
	})
	if err != nil || v == nil {
		return nil
	}
	return v.([]string)
}

func (e *Engine) Eval(code string) error {
	_, err := e.call(func(r *runtime) (any, error) {
		return nil, r.L.DoString(code)
	})
	return err
}

func (e *Engine) Command(name, args string, ctx Context) (bool, error) {
	v, err := e.call(func(r *runtime) (any, error) {
		fn := r.commands[strings.ToLower(name)]
		if fn == nil {
			return false, nil
		}
		tbl := r.contextTable(ctx)
		tbl.RawSetString("command", lua.LString(name))
		tbl.RawSetString("args", lua.LString(args))
		err := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, tbl)
		return true, err
	})
	if v == nil {
		return false, err
	}
	return v.(bool), err
}

func (e *Engine) EmitEvent(ev Event) {
	select {
	case e.events <- ev:
	default:
		// IRC must never be blocked by a slow or broken Lua script. If scripts
		// fall far enough behind to fill the queue, drop hook events rather than
		// stalling the network client.
	}
}

func (r *runtime) installAPI() {
	mod := r.L.NewTable()
	r.L.SetFuncs(mod, map[string]lua.LGFunction{
		"on":      r.luaOn,
		"command": r.luaCommand,
		"print":   r.luaPrint,
		"message": r.luaMessage,
		"notice":  r.luaNotice,
		"raw":     r.luaRaw,
		"active":  r.luaActive,
		"nick":    r.luaNick,
	})
	r.L.SetGlobal("copperline", mod)
}

func (r *runtime) luaOn(L *lua.LState) int {
	name := strings.ToLower(strings.TrimSpace(L.CheckString(1)))
	fn := L.CheckFunction(2)
	if name == "" {
		L.ArgError(1, "event name cannot be empty")
		return 0
	}
	r.hooks[name] = append(r.hooks[name], fn)
	return 0
}

func (r *runtime) luaCommand(L *lua.LState) int {
	name := strings.ToLower(strings.TrimSpace(L.CheckString(1)))
	fn := L.CheckFunction(2)
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		L.ArgError(1, "command must be one word")
		return 0
	}
	r.commands[name] = fn
	return 0
}

func (r *runtime) luaPrint(L *lua.LState) int {
	text := L.CheckString(1)
	ctx := r.host.Active()
	server := L.OptString(2, ctx.Server)
	target := L.OptString(3, ctx.Target)
	if r.host.Print != nil {
		r.host.Print(server, target, text)
	}
	return 0
}

func (r *runtime) luaMessage(L *lua.LState) int {
	target := L.CheckString(1)
	text := L.CheckString(2)
	server := L.OptString(3, r.host.Active().Server)
	if r.host.Message != nil {
		if err := r.host.Message(server, target, text); err != nil {
			L.RaiseError("message: %v", err)
		}
	}
	return 0
}

func (r *runtime) luaNotice(L *lua.LState) int {
	target := L.CheckString(1)
	text := L.CheckString(2)
	server := L.OptString(3, r.host.Active().Server)
	if r.host.Notice != nil {
		if err := r.host.Notice(server, target, text); err != nil {
			L.RaiseError("notice: %v", err)
		}
	}
	return 0
}

func (r *runtime) luaRaw(L *lua.LState) int {
	line := L.CheckString(1)
	server := L.OptString(2, r.host.Active().Server)
	if r.host.Raw != nil {
		if err := r.host.Raw(server, line); err != nil {
			L.RaiseError("raw: %v", err)
		}
	}
	return 0
}

func (r *runtime) luaActive(L *lua.LState) int {
	L.Push(r.contextTable(r.host.Active()))
	return 1
}

func (r *runtime) luaNick(L *lua.LState) int {
	ctx := r.host.Active()
	server := L.OptString(1, ctx.Server)
	if r.host.CurrentNick == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(r.host.CurrentNick(server)))
	return 1
}

func (r *runtime) contextTable(ctx Context) *lua.LTable {
	t := r.L.NewTable()
	t.RawSetString("server", lua.LString(ctx.Server))
	t.RawSetString("target", lua.LString(ctx.Target))
	t.RawSetString("nick", lua.LString(ctx.Nick))
	return t
}

func (r *runtime) dispatchEvent(ev Event) {
	fields := map[string]lua.LValue{
		"server":  lua.LString(ev.Server),
		"command": lua.LString(strings.ToLower(ev.Command)),
		"source":  lua.LString(ev.Source),
		"time":    lua.LNumber(ev.Time.Unix()),
		"raw":     lua.LBool(ev.Raw),
	}
	t := r.L.NewTable()
	for key, value := range fields {
		t.RawSetString(key, value)
	}
	params := r.L.NewTable()
	for _, p := range ev.Params {
		params.Append(lua.LString(p))
	}
	t.RawSetString("params", params)
	if len(ev.Params) > 0 {
		t.RawSetString("target", lua.LString(ev.Params[0]))
	}
	if len(ev.Params) > 1 {
		t.RawSetString("text", lua.LString(ev.Params[len(ev.Params)-1]))
	}
	tags := r.L.NewTable()
	for k, v := range ev.Tags {
		tags.RawSetString(k, lua.LString(v))
	}
	t.RawSetString("tags", tags)

	if ev.Raw {
		r.callHooks("irc", t)
	}
	name := strings.ToLower(ev.Command)
	if name != "irc" {
		r.callHooks(name, t)
	}
}

func (r *runtime) dispatchNamed(name string, fields map[string]lua.LValue) {
	t := r.L.NewTable()
	for key, value := range fields {
		t.RawSetString(key, value)
	}
	r.callHooks(strings.ToLower(name), t)
}

func (r *runtime) callHooks(name string, event *lua.LTable) {
	for _, fn := range r.hooks[name] {
		if err := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, event); err != nil {
			ctx := r.host.Active()
			if r.host.Print != nil {
				r.host.Print(ctx.Server, ctx.Target, fmt.Sprintf("Lua %s hook error: %v", name, err))
			}
		}
	}
}
