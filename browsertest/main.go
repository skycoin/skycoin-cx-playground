// Command browsertest runs the browser tests of the playground in a headless
// Brave or Chrome through the DevTools protocol. Each folder under tests/ is
// one test: steps.txt drives the page with key and mouse events and runs
// programs in bottle, expect.txt checks the files the steps saved. See
// "Browser tests" in the README of the repository.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type options struct {
	browser     string
	site        string
	tests       string
	results     string
	run         string
	microagents string
	cxtests     string
	cx          string
	keep        bool
}

func main() {
	var o options
	flag.StringVar(&o.browser, "browser", envOr("BROWSERTEST_BROWSER", "/opt/brave.com/brave-nightly/brave"), "Chrome or Brave binary, run headless")
	flag.StringVar(&o.site, "site", "../_site", "the site make site built, served at /")
	flag.StringVar(&o.tests, "tests", "tests", "folder of test folders")
	flag.StringVar(&o.results, "results", "results", "results go to <results>/<time>/<test>/")
	flag.StringVar(&o.run, "run", "*", "glob of the test folders to run")
	flag.StringVar(&o.microagents, "microagents", "", "the microagents folder of skycoin-cx-playground-android, $microagents in steps")
	flag.StringVar(&o.cxtests, "cxtests", "", "the tests folder of the skycoin-cx that built cx.wasm, $cxtests in steps")
	flag.StringVar(&o.cx, "cx", envOr("CX", "cx"), "native cx for host steps, $CX in their environment")
	flag.BoolVar(&o.keep, "keep", false, "keep the browser profile folder")
	flag.Parse()
	failed, err := suiteRun(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "browsertest:", err)
		os.Exit(2)
	}
	if failed > 0 {
		fmt.Printf("%d test(s) failed\n", failed)
		os.Exit(1)
	}
	fmt.Println("all tests passed")
}

func envOr(name, value string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return value
}

// suiteRun serves the site, starts the browser and runs every test folder in
// its own new tab. It returns the number of failed tests.
func suiteRun(o options) (int, error) {
	for _, p := range []*string{&o.site, &o.tests, &o.results, &o.microagents, &o.cxtests, &o.cx} {
		// A bare cx is looked up in PATH, every other value is a path.
		if *p == "" || (p == &o.cx && !strings.Contains(*p, "/")) {
			continue
		}
		a, err := filepath.Abs(*p)
		if err != nil {
			return 0, err
		}
		*p = a
	}
	dirs, err := filepath.Glob(filepath.Join(o.tests, o.run))
	if err != nil {
		return 0, err
	}
	sort.Strings(dirs)
	base, stopServer, err := siteServe(o.site)
	if err != nil {
		return 0, err
	}
	defer stopServer()
	browser, err := browserStart(o)
	if err != nil {
		return 0, err
	}
	defer browser.browserStop()
	resultsRoot := filepath.Join(o.results, time.Now().UTC().Format("20060102-150405"))
	summary := []string{"browser " + browser.version, "site " + o.site}
	failed := 0
	for _, dir := range dirs {
		if st, err := os.Stat(filepath.Join(dir, "steps.txt")); err != nil || st.IsDir() {
			continue
		}
		name := filepath.Base(dir)
		out := filepath.Join(resultsRoot, name)
		if err := os.MkdirAll(out, 0o755); err != nil {
			return failed, err
		}
		start := time.Now()
		detail, err := testRun(o, browser, base, dir, out)
		line := fmt.Sprintf("PASS %s: %s (%.1f s)", name, detail, time.Since(start).Seconds())
		if err != nil {
			failed++
			line = fmt.Sprintf("FAIL %s: %v (%.1f s)", name, err, time.Since(start).Seconds())
		}
		fmt.Println(line)
		summary = append(summary, line)
		if err := os.WriteFile(filepath.Join(out, "result.txt"), []byte(line+"\n"), 0o644); err != nil {
			return failed, err
		}
	}
	fmt.Println("results in", resultsRoot)
	return failed, os.WriteFile(filepath.Join(resultsRoot, "summary.txt"), []byte(strings.Join(summary, "\n")+"\n"), 0o644)
}

// siteServe serves dir on a free loopback port with the headers that make a
// page cross-origin isolated, so bottle runs programs in Workers at once.
func siteServe(dir string) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	files := http.FileServer(http.Dir(dir))
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})}
	go server.Serve(listener)
	return "http://" + listener.Addr().String(), func() { server.Close() }, nil
}

type browserProcess struct {
	cmd     *exec.Cmd
	profile string
	keep    bool
	debug   string
	version string
}

// browserStart starts the browser headless with a new profile and reads the
// DevTools port it chose.
func browserStart(o options) (*browserProcess, error) {
	profile, err := os.MkdirTemp("", "browsertest-profile-")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(o.browser, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "--disable-extensions", "--window-size=1024,768",
		"--hide-scrollbars", "about:blank")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		os.RemoveAll(profile)
		return nil, err
	}
	b := &browserProcess{cmd: cmd, profile: profile, keep: o.keep}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			if port, _, ok := strings.Cut(string(data), "\n"); ok && port != "" {
				b.debug = "http://127.0.0.1:" + port
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if b.debug == "" {
		b.browserStop()
		return nil, errors.New("the browser wrote no DevToolsActivePort in 20 s")
	}
	var v struct {
		Browser string `json:"Browser"`
	}
	if err := httpJSON("GET", b.debug+"/json/version", &v); err != nil {
		b.browserStop()
		return nil, err
	}
	b.version = v.Browser
	return b, nil
}

func (b *browserProcess) browserStop() {
	if b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd.Wait()
	}
	if !b.keep {
		os.RemoveAll(b.profile)
	}
}

type browserTab struct {
	ID  string `json:"id"`
	URL string `json:"webSocketDebuggerUrl"`
}

func (b *browserProcess) browserTabOpen() (*browserTab, error) {
	var t browserTab
	return &t, httpJSON("PUT", b.debug+"/json/new?about:blank", &t)
}

func (b *browserProcess) browserTabClose(t *browserTab) {
	httpJSON("GET", b.debug+"/json/close/"+t.ID, nil)
}

func httpJSON(method, url string, result any) error {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s", method, url, resp.Status)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// testRun runs the steps and then the checks of the test in dir in a new
// tab, and returns a one line summary.
func testRun(o options, b *browserProcess, base, dir, out string) (string, error) {
	tab, err := b.browserTabOpen()
	if err != nil {
		return "", err
	}
	defer b.browserTabClose(tab)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	conn, err := cdpConnect(ctx, tab.URL)
	if err != nil {
		return "", err
	}
	defer conn.cdpClose()
	if err := conn.cdpCall(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
		"width": 1024, "height": 768, "deviceScaleFactor": 1, "mobile": false,
	}, nil); err != nil {
		return "", err
	}
	r := &stepRunner{
		ctx: ctx, conn: conn, base: base, dir: dir, out: out, options: o,
		vars:  map[string]string{"$test": dir, "$results": out, "$microagents": o.microagents, "$cxtests": o.cxtests},
		exits: map[string]int{},
	}
	steps, err := lines(filepath.Join(dir, "steps.txt"))
	if err != nil {
		return "", err
	}
	for i, s := range steps {
		if err := r.stepRun(s); err != nil {
			return "", fmt.Errorf("step %d %q: %w", i+1, s, err)
		}
	}
	checks, err := lines(filepath.Join(dir, "expect.txt"))
	if err != nil {
		return "", err
	}
	for _, c := range checks {
		if err := r.checkRun(c); err != nil {
			return "", fmt.Errorf("check %q: %w", c, err)
		}
	}
	return fmt.Sprintf("%d steps, %d checks", len(steps), len(checks)), nil
}

// lines returns the lines of a file that are not blank or # comments.
func lines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		t := strings.TrimSpace(s.Text())
		if t != "" && !strings.HasPrefix(t, "#") {
			out = append(out, t)
		}
	}
	return out, s.Err()
}
