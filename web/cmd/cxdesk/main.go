//go:build js && wasm

// Command cxdesk is the CX workstation: a desk with websh terminals, a file
// manager, the websh text editor and a lazily loaded netscrape browser, all
// on bottle's jsfs. cx and cxcorefile are wasm programs in /bin that the page
// writes there, so `cx main.cx lib.cx` runs the real cx.wasm as a process,
// with stdin, pipes and exit codes. A CX server listening on 127.0.0.1 is on
// bottle's vnet, where another process or the browser window dials it.
package main

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/0magnet/afero"
	"github.com/0magnet/desk"
	"github.com/0magnet/desk/panes/files"
	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/shell"
	webshbrowser "github.com/0magnet/websh/shell/browser"
	"github.com/0magnet/websh/web"
	winbox "github.com/0magnet/winbox-go"
)

// workstationHome is where the shell starts and the file manager opens.
const workstationHome = "/home/user"

// workstationEnvironment is the environment of every shell.
var workstationEnvironment = []string{
	"PATH=/bin",
	"HOME=" + workstationHome,
	"USER=user",
	"HOSTNAME=cx",
	"CXPATH=" + workstationHome + "/.cx",
	"EDITOR=edit",
}

// workstationGreeting is written once in each new terminal.
const workstationGreeting = "CX workstation. cx is on the PATH: try  cd examples && cx hello-world.cx\r\n" +
	"cat ~/README.txt for the commands, edit FILE to edit, browse URL to open the browser.\r\n\r\n"

// terminalPane is a websh shell in a desk window.
type terminalPane struct {
	session *web.Session
	// command runs once the shell is up, and onExit closes the window when
	// the shell exits. The editor window uses both.
	command string
	onExit  func()
}

// terminalPanes is every open terminal, oldest first, for __cxdeskSubmit.
var terminalPanes []*terminalPane

func (pane *terminalPane) Mount(element js.Value) error {
	greeting := workstationGreeting
	if pane.command != "" {
		greeting = ""
	}
	session, err := web.NewSession(element, web.Options{
		FS:        afero.NewOsFs(),
		Host:      "cx",
		Greeting:  greeting,
		Env:       workstationEnvironment,
		NoWebGL:   true,
		OffThread: programOffThread,
		OnExit: func() {
			if pane.onExit != nil {
				pane.onExit()
			}
		},
	})
	if err != nil {
		return err
	}
	pane.session = session
	terminalPanes = append(terminalPanes, pane)
	if pane.command != "" {
		session.Submit(pane.command)
	}
	return nil
}

func (pane *terminalPane) Close() {
	for i, open := range terminalPanes {
		if open == pane {
			terminalPanes = append(terminalPanes[:i], terminalPanes[i+1:]...)
			break
		}
	}
	if pane.session != nil {
		pane.session.Close()
		pane.session = nil
	}
}

// shellQuote quotes text as one word for the shell.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// editorPaneOpening is the editor pane being opened, so the launcher can
// hand it its window once desk.LaunchOpts returns one.
var editorPaneOpening *terminalPane

// editorOpen opens filePath in websh's editor in a window of its own, which
// closes when the editor quits.
func editorOpen(filePath string) error {
	window, err := desk.LaunchOpts("editor-window", desk.Options{Title: "edit " + path.Base(filePath)}, filePath)
	if err != nil {
		return err
	}
	pane := editorPaneOpening
	editorPaneOpening = nil
	if pane != nil && window != nil {
		pane.onExit = func() { go window.Close(true) }
	}
	return nil
}

// browserMountCount numbers the browser windows' mount elements.
var browserMountCount int

// browserPane is a netscrape window. netscrape.wasm is a program of its own,
// fetched the first time a browser window opens.
type browserPane struct {
	startURL string
	process  js.Value
}

func (pane *browserPane) Mount(element js.Value) error {
	browserMountCount++
	mountID := "cxdesk-browser-" + strconv.Itoa(browserMountCount)
	mount := js.Global().Get("document").Call("createElement", "div")
	mount.Set("id", mountID)
	mount.Get("style").Set("cssText", "position:absolute;inset:0")
	element.Call("appendChild", mount)
	if pane.startURL != "" {
		js.Global().Set("__netscrapeStart", pane.startURL)
	} else {
		js.Global().Delete("__netscrapeStart")
	}
	environment := js.Global().Get("Object").New()
	environment.Set("NETSCRAPE_MOUNT", mountID)
	arguments := js.Global().Get("Array").New()
	arguments.Call("push", "/opt/netscrape/browser.wasm")
	options := js.Global().Get("Object").New()
	options.Set("argv", arguments)
	options.Set("cwd", workstationHome)
	options.Set("env", environment)
	pane.process = js.Global().Get("proc").Call("spawn", options)
	return nil
}

func (pane *browserPane) Close() {
	if pane.process.Truthy() && pane.process.Get("kill").Type() == js.TypeFunction {
		pane.process.Call("kill", true)
	}
	pane.process = js.Value{}
}

// browseApplet opens the browser window at a URL: browse [URL].
func browseApplet(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, args []string) int {
	start := ""
	if len(args) > 0 {
		start = args[0]
		if !strings.Contains(start, "://") {
			start = "http://" + start
		}
	}
	if _, err := desk.LaunchOpts("browser", desk.Options{Title: "netscrape " + start}, start); err != nil {
		shell.Printf(hc.Stderr, "browse: %v\n", err)
		return 1
	}
	return 0
}

// openApplet opens a file in an editor window of its own: open FILE. The
// command edit edits in the terminal it is typed in.
func openApplet(_ context.Context, _ *shell.Shell, hc *interp.HandlerContext, args []string) int {
	if len(args) != 1 {
		shell.Println(hc.Stderr, "usage: open FILE")
		return 2
	}
	if err := editorOpen(shell.Resolve(hc, args[0])); err != nil {
		shell.Printf(hc.Stderr, "open: %v\n", err)
		return 1
	}
	return 0
}

// shellRunResult is what __cxdeskExec resolves with.
type shellRunResult struct {
	code           int
	stdout, stderr strings.Builder
}

// shellRunHeadless runs one command line in a new shell with no terminal,
// in directory, with stdin as its input. A job it starts with & keeps
// running, as it would in a terminal.
func shellRunHeadless(directory, line, stdin string) *shellRunResult {
	result := &shellRunResult{}
	headless, err := shell.New(afero.NewOsFs(), strings.NewReader(stdin), &result.stdout, &result.stderr, workstationEnvironment...)
	if err != nil {
		result.stderr.WriteString(err.Error())
		result.code = 126
		return result
	}
	headless.OffThread = programOffThread
	if directory != "" {
		line = "cd " + shellQuote(directory) + " && " + line
	}
	_, err = headless.Run(context.Background(), line)
	var status interp.ExitStatus
	switch {
	case err == nil:
	case errors.As(err, &status):
		result.code = int(status)
	default:
		result.stderr.WriteString(err.Error() + "\n")
		result.code = 2
	}
	return result
}

func main() {
	document := js.Global().Get("document")
	winbox.InjectCSS()
	if root := document.Call("getElementById", "desktop"); root.Truthy() {
		desk.SetRoot(root)
	}
	desk.NewPanel()

	webshbrowser.Register()
	shell.RegisterApplet("browse", "open the netscrape browser, at a URL such as 127.0.0.1:8080", browseApplet)
	shell.RegisterApplet("open", "open a file in an editor window", openApplet)
	projectAppletsRegister()

	desk.Register(desk.App{
		Name:   "shell",
		Title:  "terminal",
		Help:   "a websh shell with cx on its PATH",
		Width:  820,
		Height: 500,
		NewTab: true,
		Open:   func([]string) (desk.Pane, error) { return &terminalPane{}, nil },
	})
	desk.Register(desk.App{
		Name:   "files",
		Title:  "files",
		Help:   "the files in this tab, double click to edit",
		Width:  520,
		Height: 440,
		Open: func(args []string) (desk.Pane, error) {
			directory := workstationHome
			if len(args) > 0 {
				directory = args[0]
			}
			return files.New(afero.NewOsFs(), directory), nil
		},
	})
	// The file manager opens a file through the app named viewer.
	desk.Register(desk.App{
		Name:     "viewer",
		Unlisted: true,
		Run: func(args []string) error {
			if len(args) == 0 {
				return errors.New("viewer: no file")
			}
			return editorOpen(args[0])
		},
	})
	desk.Register(desk.App{
		Name:     "editor-window",
		Title:    "edit",
		Unlisted: true,
		Width:    760,
		Height:   520,
		Open: func(args []string) (desk.Pane, error) {
			pane := &terminalPane{command: "edit " + shellQuote(args[0]) + "; exit"}
			editorPaneOpening = pane
			return pane, nil
		},
	})
	desk.Register(desk.App{
		Name:   "browser",
		Title:  "netscrape",
		Help:   "a web browser, it opens servers on 127.0.0.1 in this tab",
		Width:  760,
		Height: 520,
		Open: func(args []string) (desk.Pane, error) {
			pane := &browserPane{}
			if len(args) > 0 {
				pane.startURL = args[0]
			}
			return pane, nil
		},
	})
	desk.Register(desk.App{
		Name:  "project-save",
		Title: "save project",
		Help:  "download a folder as a zip file",
		Run: func([]string) error {
			go projectSaveFromMenu()
			return nil
		},
	})
	desk.Register(desk.App{
		Name:  "project-load",
		Title: "load project",
		Help:  "upload a zip file or a folder into ~",
		Run: func([]string) error {
			projectLoadFromMenu()
			return nil
		},
	})

	if _, err := desk.LaunchOpts("files", desk.Options{X: 20, Y: 20, Width: 360, Height: 420}); err != nil {
		js.Global().Get("console").Call("error", "cxdesk: "+err.Error())
	}
	if _, err := desk.LaunchOpts("shell", desk.Options{X: 400, Y: 20}); err != nil {
		js.Global().Get("console").Call("error", "cxdesk: "+err.Error())
		return
	}

	js.Global().Set("__cxdeskSubmit", js.FuncOf(func(_ js.Value, arguments []js.Value) any {
		for _, pane := range terminalPanes {
			if pane.session != nil && pane.command == "" && len(arguments) > 0 {
				pane.session.Submit(arguments[0].String())
				break
			}
		}
		return nil
	}))
	// __cxdeskExec(directory, line, stdin) runs a line in a shell with no
	// terminal and resolves {code, stdout, stderr}, for tests.
	js.Global().Set("__cxdeskExec", js.FuncOf(func(_ js.Value, arguments []js.Value) any {
		text := func(index int) string {
			if index < len(arguments) && arguments[index].Type() == js.TypeString {
				return arguments[index].String()
			}
			return ""
		}
		directory, line, stdin := text(0), text(1), text(2)
		promiseConstructor := js.Global().Get("Promise")
		var executor js.Func
		executor = js.FuncOf(func(_ js.Value, settle []js.Value) any {
			resolve := settle[0]
			go func() {
				result := shellRunHeadless(directory, line, stdin)
				resolve.Invoke(map[string]any{
					"code":   result.code,
					"stdout": result.stdout.String(),
					"stderr": result.stderr.String(),
				})
			}()
			executor.Release()
			return nil
		})
		return promiseConstructor.New(executor)
	}))
	js.Global().Set("__cxdeskOpen", js.FuncOf(func(_ js.Value, arguments []js.Value) any {
		if len(arguments) == 0 {
			return nil
		}
		var launchArguments []string
		for _, argument := range arguments[1:] {
			launchArguments = append(launchArguments, argument.String())
		}
		go func() {
			if _, err := desk.Launch(arguments[0].String(), launchArguments...); err != nil {
				js.Global().Get("console").Call("error", "cxdesk: "+err.Error())
			}
		}()
		return nil
	}))

	if ready := js.Global().Get("__cxdeskReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

// programOffThread says which programs run in a Worker: cx and its cores,
// so an endless loop cannot hold the page and Ctrl+C stops it. A run that
// enables the net library stays on the page thread, where bottle's vnet is.
func programOffThread(args []string) bool {
	switch path.Base(args[0]) {
	case "cx", "cx-core", "cx-core-image", "cxcorefile":
	default:
		return false
	}
	for i, argument := range args {
		if !strings.HasPrefix(argument, "--enable") && !strings.HasPrefix(argument, "-enable") {
			continue
		}
		value := argument
		if !strings.Contains(argument, "=") && i+1 < len(args) {
			value = args[i+1]
		}
		if strings.Contains(value, "net") {
			return false
		}
	}
	return true
}
