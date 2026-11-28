package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// stepRunner runs the steps and checks of one test against one tab.
type stepRunner struct {
	ctx     context.Context
	conn    *cdpConnection
	base    string
	dir     string
	out     string
	options options
	vars    map[string]string
	exits   map[string]int
}

const stepWaitTimeout = 60 * time.Second

func (r *stepRunner) expand(s string) string {
	for k, v := range r.vars {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}

// testPath resolves a file name of a step against the test folder, unless it
// is absolute after expansion.
func (r *stepRunner) testPath(name string) string {
	name = r.expand(name)
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(r.dir, name)
}

// resultPath resolves a saved file name against the results folder.
func (r *stepRunner) resultPath(name string) string {
	return filepath.Join(r.out, name)
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (r *stepRunner) stepRun(line string) error {
	verb, rest, _ := strings.Cut(line, " ")
	args := strings.Fields(rest)
	switch verb {
	case "open":
		return r.stepOpen(rest)
	case "wait-eval":
		return r.waitEval(rest)
	case "wait-text":
		if len(args) < 2 {
			return errors.New("want wait-text SELECTOR TEXT")
		}
		text := strings.TrimSpace(strings.TrimPrefix(rest, args[0]))
		return r.waitEval(fmt.Sprintf("(document.querySelector(%s)?.textContent || '').includes(%s)", jsString(args[0]), jsString(text)))
	case "eval":
		return r.conn.cdpEvaluate(r.ctx, r.expand(rest), nil)
	case "click":
		return r.stepClick(rest)
	case "key":
		for _, k := range args {
			if err := r.keyPress(k); err != nil {
				return err
			}
		}
		return nil
	case "type":
		return r.typeText(rest)
	case "type-file":
		b, err := os.ReadFile(r.testPath(rest))
		if err != nil {
			return err
		}
		return r.typeText(string(b))
	case "files":
		if len(args) != 2 {
			return errors.New("want files HOST-PATH JSFS-FOLDER")
		}
		return r.stepFiles(r.testPath(args[0]), args[1])
	case "spawn":
		if len(args) < 3 {
			return errors.New("want spawn NAME CWD ARGV...")
		}
		return r.stepSpawn(args[0], args[1], args[2:])
	case "shell":
		if len(args) < 3 {
			return errors.New("want shell NAME CWD COMMAND...")
		}
		return r.stepShell(args[0], args[1], strings.TrimSpace(strings.SplitN(rest, " ", 3)[2]))
	case "pull":
		if len(args) != 2 {
			return errors.New("want pull JSFS-FOLDER NAME")
		}
		return r.stepPull(args[0], args[1])
	case "grid":
		if len(args) != 2 {
			return errors.New("want grid SELECTOR NAME")
		}
		return r.stepGrid(args[0], args[1])
	case "screenshot":
		return r.stepScreenshot(rest)
	case "host":
		if len(args) < 3 {
			return errors.New("want host NAME CWD COMMAND...")
		}
		return r.stepHost(args[0], args[1], strings.Join(args[2:], " "))
	case "wait":
		ms, err := strconv.Atoi(rest)
		if err != nil {
			return err
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil
	}
	return fmt.Errorf("unknown step %q", verb)
}

func (r *stepRunner) stepOpen(path string) error {
	if err := r.conn.cdpCall(r.ctx, "Page.navigate", map[string]any{"url": r.base + path}, nil); err != nil {
		return err
	}
	return r.waitEval("document.readyState === 'complete'")
}

// waitEval polls a JavaScript expression until it is true.
func (r *stepRunner) waitEval(expression string) error {
	deadline := time.Now().Add(stepWaitTimeout)
	var last error
	for time.Now().Before(deadline) {
		var ok bool
		last = r.conn.cdpEvaluate(r.ctx, "Boolean("+expression+")", &ok)
		if last == nil && ok {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last != nil {
		return fmt.Errorf("not true in %v: %w", stepWaitTimeout, last)
	}
	return fmt.Errorf("not true in %v", stepWaitTimeout)
}

// stepClick presses and releases the left mouse button at the centre of the
// element, as a user's click does.
func (r *stepRunner) stepClick(selector string) error {
	var p struct{ X, Y float64 }
	err := r.conn.cdpEvaluate(r.ctx, fmt.Sprintf(`(() => { const e = document.querySelector(%s);
		e.scrollIntoView({block: "center"}); const b = e.getBoundingClientRect();
		return {X: b.left + b.width / 2, Y: b.top + b.height / 2}; })()`, jsString(selector)), &p)
	if err != nil {
		return err
	}
	for _, kind := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		if err := r.conn.cdpCall(r.ctx, "Input.dispatchMouseEvent", map[string]any{
			"type": kind, "x": p.X, "y": p.Y, "button": "left", "clickCount": 1,
		}, nil); err != nil {
			return err
		}
	}
	return nil
}

type keyDefinition struct {
	key, code, text string
	virtualCode     int
}

var keyDefinitions = map[string]keyDefinition{
	"Enter":      {"Enter", "Enter", "\r", 13},
	"Tab":        {"Tab", "Tab", "", 9},
	"Backspace":  {"Backspace", "Backspace", "", 8},
	"Escape":     {"Escape", "Escape", "", 27},
	"ArrowLeft":  {"ArrowLeft", "ArrowLeft", "", 37},
	"ArrowUp":    {"ArrowUp", "ArrowUp", "", 38},
	"ArrowRight": {"ArrowRight", "ArrowRight", "", 39},
	"ArrowDown":  {"ArrowDown", "ArrowDown", "", 40},
}

// keyPress sends one key down and up. NAME is a key of keyDefinitions, or
// ctrl+a to select all.
func (r *stepRunner) keyPress(name string) error {
	if name == "ctrl+a" {
		return r.keyEvents(map[string]any{"key": "a", "code": "KeyA", "windowsVirtualKeyCode": 65,
			"modifiers": 2, "commands": []string{"selectAll"}})
	}
	if letter, ok := strings.CutPrefix(name, "ctrl+"); ok && len(letter) == 1 && letter[0] >= 'a' && letter[0] <= 'z' {
		upper := strings.ToUpper(letter)
		return r.keyEvents(map[string]any{"key": letter, "code": "Key" + upper,
			"windowsVirtualKeyCode": int(upper[0]), "modifiers": 2})
	}
	d, ok := keyDefinitions[name]
	if !ok {
		return fmt.Errorf("unknown key %q", name)
	}
	ev := map[string]any{"key": d.key, "code": d.code, "windowsVirtualKeyCode": d.virtualCode}
	if d.text != "" {
		ev["text"] = d.text
	}
	return r.keyEvents(ev)
}

func (r *stepRunner) keyEvents(ev map[string]any) error {
	down := map[string]any{"type": "keyDown"}
	if _, ok := ev["text"]; !ok {
		down["type"] = "rawKeyDown"
	}
	up := map[string]any{"type": "keyUp"}
	for k, v := range ev {
		down[k] = v
		if k != "text" && k != "commands" {
			up[k] = v
		}
	}
	if err := r.conn.cdpCall(r.ctx, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	return r.conn.cdpCall(r.ctx, "Input.dispatchKeyEvent", up, nil)
}

// typeText sends one key event per character, ENTER for a newline and TAB
// for a tab.
func (r *stepRunner) typeText(text string) error {
	for _, c := range text {
		var err error
		switch c {
		case '\n':
			err = r.keyPress("Enter")
		case '\t':
			err = r.keyPress("Tab")
		case '\r':
			continue
		default:
			s := string(c)
			err = r.keyEvents(map[string]any{"key": s, "text": s, "unmodifiedText": s})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// stepFiles copies a host file or folder into bottle's jsfs at folder.
func (r *stepRunner) stepFiles(host, folder string) error {
	type file struct {
		Path string `json:"path"`
		Data string `json:"data"`
	}
	var batch []file
	size := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		b, _ := json.Marshal(batch)
		batch, size = nil, 0
		return r.conn.cdpEvaluate(r.ctx, `(() => { for (const f of `+string(b)+`) {
			jsfs.writeFile(f.path, Uint8Array.from(atob(f.data), (c) => c.charCodeAt(0))); } return true; })()`, nil)
	}
	st, err := os.Stat(host)
	if err != nil {
		return err
	}
	root := host
	if !st.IsDir() {
		root = filepath.Dir(host)
	}
	err = filepath.WalkDir(host, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		batch = append(batch, file{Path: strings.TrimSuffix(folder, "/") + "/" + filepath.ToSlash(rel), Data: base64.StdEncoding.EncodeToString(data)})
		size += len(data)
		if size > 4<<20 {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	return flush()
}

// stepSpawn runs argv as a bottle process in a Worker with cwd and saves its
// standard output, standard error and exit status as NAME.stdout,
// NAME.stderr and NAME.exit.
func (r *stepRunner) stepSpawn(name, cwd string, argv []string) error {
	for i := range argv {
		argv[i] = r.expand(argv[i])
	}
	a, _ := json.Marshal(argv)
	var res struct {
		Code   int    `json:"code"`
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	err := r.conn.cdpEvaluate(r.ctx, `(async () => {
		jsfs.mkdirp(`+jsString(cwd)+`);
		const out = [], err = [];
		const h = proc.spawnWorker({argv: `+string(a)+`, cwd: `+jsString(cwd)+`,
			env: {PATH: "/bin", HOME: "/home/cx", USER: "cx", CXPATH: "/home/cx/cx"},
			stdout: (b) => out.push(b.slice()), stderr: (b) => err.push(b.slice())});
		const timer = setTimeout(() => h.kill(true), 300000);
		const code = await h.exited;
		clearTimeout(timer);
		await new Promise((res) => setTimeout(res, 0));
		const join = (parts) => {
			let s = "";
			for (const p of parts) for (let i = 0; i < p.length; i += 32768) s += String.fromCharCode.apply(null, p.subarray(i, i + 32768));
			return btoa(s);
		};
		return {code, stdout: join(out), stderr: join(err)};
	})()`, &res)
	if err != nil {
		return err
	}
	r.exits[name] = res.Code
	for ext, data := range map[string]string{".stdout": res.Stdout, ".stderr": res.Stderr} {
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(r.resultPath(name+ext), b, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(r.resultPath(name+".exit"), []byte(strconv.Itoa(res.Code)+"\n"), 0o644)
}

// stepPull copies a jsfs folder to NAME/ in the results.
func (r *stepRunner) stepPull(folder, name string) error {
	var files map[string]string
	err := r.conn.cdpEvaluate(r.ctx, `(() => {
		const out = {};
		const walk = (dir) => {
			for (const n of jsfs.sync.readdir(dir)) {
				const p = dir + "/" + n;
				if ((jsfs.sync.stat(p).mode & 0o170000) === 0o040000) { walk(p); continue; }
				const d = jsfs.readFile(p) || new Uint8Array();
				let s = "";
				for (let i = 0; i < d.length; i += 32768) s += String.fromCharCode.apply(null, d.subarray(i, i + 32768));
				out[p.slice(`+jsString(strings.TrimSuffix(folder, "/"))+`.length + 1)] = btoa(s);
			}
		};
		walk(`+jsString(strings.TrimSuffix(folder, "/"))+`);
		return out;
	})()`, &files)
	if err != nil {
		return err
	}
	for rel, data := range files {
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return err
		}
		p := filepath.Join(r.resultPath(name), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// stepGrid saves the text of an element as the terminal grid it shows, one
// line per row, as NAME.grid.txt, and its size in character cells, from the
// element size and the advance and line height of its font, as
// NAME.grid-size.txt.
func (r *stepRunner) stepGrid(selector, name string) error {
	var g struct {
		Text       string  `json:"text"`
		Columns    int     `json:"columns"`
		Rows       int     `json:"rows"`
		CellWidth  float64 `json:"cellWidth"`
		CellHeight float64 `json:"cellHeight"`
	}
	err := r.conn.cdpEvaluate(r.ctx, fmt.Sprintf(`(() => {
		const e = document.querySelector(%s);
		const probe = document.createElement("span");
		probe.style.font = getComputedStyle(e).font;
		probe.style.whiteSpace = "pre";
		probe.style.position = "absolute";
		probe.textContent = "M".repeat(100);
		document.body.appendChild(probe);
		const b = probe.getBoundingClientRect();
		probe.remove();
		const cellWidth = b.width / 100, cellHeight = b.height;
		const text = "value" in e && e.tagName !== "PRE" ? e.value : e.textContent;
		return {text, cellWidth, cellHeight,
			columns: Math.floor(e.clientWidth / cellWidth), rows: Math.round(e.scrollHeight / cellHeight)};
	})()`, jsString(selector)), &g)
	if err != nil {
		return err
	}
	size := fmt.Sprintf("columns %d\nrows %d\ncell %.2f x %.2f px\n", g.Columns, g.Rows, g.CellWidth, g.CellHeight)
	if err := os.WriteFile(r.resultPath(name+".grid-size.txt"), []byte(size), 0o644); err != nil {
		return err
	}
	return os.WriteFile(r.resultPath(name+".grid.txt"), []byte(g.Text), 0o644)
}

func (r *stepRunner) stepScreenshot(name string) error {
	var s struct {
		Data string `json:"data"`
	}
	if err := r.conn.cdpCall(r.ctx, "Page.captureScreenshot", map[string]any{"format": "png"}, &s); err != nil {
		return err
	}
	b, err := base64.StdEncoding.DecodeString(s.Data)
	if err != nil {
		return err
	}
	return os.WriteFile(r.resultPath(name+".png"), b, 0o644)
}

// stepHost runs a shell command on the host in cwd, a folder of the results,
// with $CX set, and saves its output and exit status like spawn.
func (r *stepRunner) stepHost(name, cwd, command string) error {
	cmd := exec.Command("/bin/sh", "-c", r.expand(command))
	cmd.Dir = r.resultPath(cwd)
	cmd.Env = append(os.Environ(), "CX="+r.options.cx)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return err
		}
		code = exit.ExitCode()
	}
	r.exits[name] = code
	os.WriteFile(r.resultPath(name+".stdout"), stdout.Bytes(), 0o644)
	os.WriteFile(r.resultPath(name+".stderr"), stderr.Bytes(), 0o644)
	return os.WriteFile(r.resultPath(name+".exit"), []byte(strconv.Itoa(code)+"\n"), 0o644)
}

func (r *stepRunner) checkRun(line string) error {
	verb, rest, _ := strings.Cut(line, " ")
	args := strings.Fields(rest)
	switch verb {
	case "exit":
		if len(args) != 2 {
			return errors.New("want exit NAME CODE")
		}
		want, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		got, ok := r.exits[args[0]]
		if !ok {
			return fmt.Errorf("no process %s ran", args[0])
		}
		if got != want {
			return fmt.Errorf("exit status %d, want %d, stderr: %s", got, want, r.tail(args[0]+".stderr"))
		}
		return nil
	case "file-equals":
		if len(args) != 2 {
			return errors.New("want file-equals RESULT FILE")
		}
		got, err := os.ReadFile(r.resultPath(args[0]))
		if err != nil {
			return err
		}
		want, err := os.ReadFile(r.testPath(args[1]))
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("%d bytes differ from the %d bytes of %s", len(got), len(want), args[1])
		}
		return nil
	case "contains":
		if len(args) < 2 {
			return errors.New("want contains RESULT TEXT")
		}
		got, err := os.ReadFile(r.resultPath(args[0]))
		if err != nil {
			return err
		}
		text := strings.TrimSpace(strings.TrimPrefix(rest, args[0]))
		if !strings.Contains(string(got), text) {
			return fmt.Errorf("%q not found", text)
		}
		return nil
	case "contains-file":
		if len(args) != 2 {
			return errors.New("want contains-file RESULT FILE")
		}
		got, err := os.ReadFile(r.resultPath(args[0]))
		if err != nil {
			return err
		}
		want, err := os.ReadFile(r.testPath(args[1]))
		if err != nil {
			return err
		}
		if len(want) == 0 || !bytes.Contains(got, want) {
			return fmt.Errorf("the %d bytes of %s are not in it", len(want), args[1])
		}
		return nil
	case "png":
		f, err := os.Open(r.resultPath(rest + ".png"))
		if err != nil {
			return err
		}
		defer f.Close()
		c, err := png.DecodeConfig(f)
		if err != nil {
			return err
		}
		if c.Width != 1024 || c.Height != 768 {
			return fmt.Errorf("screenshot is %dx%d, want 1024x768", c.Width, c.Height)
		}
		return nil
	case "same":
		if len(args) != 2 {
			return errors.New("want same RESULT RESULT")
		}
		a, err := os.ReadFile(r.resultPath(args[0]))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(r.resultPath(args[1]))
		if err != nil {
			return err
		}
		if !bytes.Equal(a, b) {
			return fmt.Errorf("%s and %s differ: %q and %q", args[0], args[1], clip(a), clip(b))
		}
		return nil
	case "differs":
		if len(args) != 2 {
			return errors.New("want differs RESULT RESULT")
		}
		a, err := os.ReadFile(r.resultPath(args[0]))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(r.resultPath(args[1]))
		if err != nil {
			return err
		}
		if bytes.Equal(a, b) {
			return errors.New("the files are the same")
		}
		return nil
	}
	return fmt.Errorf("unknown check %q", verb)
}

func (r *stepRunner) tail(name string) string {
	b, _ := os.ReadFile(r.resultPath(name))
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	return s
}

// stepShell runs a command line in a shell of the workstation page, which
// runs programs from /bin as websh does at its prompt, and saves its output
// and exit status as stepSpawn does.
func (r *stepRunner) stepShell(name, cwd, command string) error {
	var res struct {
		Code   int    `json:"code"`
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	err := r.conn.cdpEvaluate(r.ctx, `(async () => {
		jsfs.mkdirp(`+jsString(cwd)+`);
		const r = await window.__cxdeskExec(`+jsString(cwd)+`, `+jsString(r.expand(command))+`, "");
		const b64 = (s) => {
			const p = new TextEncoder().encode(s);
			let t = "";
			for (let i = 0; i < p.length; i += 32768) t += String.fromCharCode.apply(null, p.subarray(i, i + 32768));
			return btoa(t);
		};
		return {code: r.code, stdout: b64(r.stdout), stderr: b64(r.stderr)};
	})()`, &res)
	if err != nil {
		return err
	}
	r.exits[name] = res.Code
	for ext, data := range map[string]string{".stdout": res.Stdout, ".stderr": res.Stderr} {
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(r.resultPath(name+ext), b, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(r.resultPath(name+".exit"), []byte(strconv.Itoa(res.Code)+"\n"), 0o644)
}

// clip shortens b for a message.
func clip(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "..."
	}
	return string(b)
}
