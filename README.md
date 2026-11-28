# CX Playground

The CX Playground runs programs in Skycoin's programming language, CX, in the
browser. Try it at https://skycoin.github.io/skycoin-cx-playground/

# In the browser

The page needs no server. `make site` builds it into `./_site`:

- `cx.wasm` is the `cx` command of [skycoin-cx](https://github.com/skycoin/skycoin-cx)
  built for js/wasm with the default tags (cipher, cxos and regexp in, cxfx out).
  It is a `tool` dependency in web/go.mod, so its version is pinned there.
- `jsfs.js`, `fsbridge.js`, `proc.js`, `vnet.js`, `coi-sw.js` and
  `coi-register.js` come from [bottle](https://github.com/0magnet/bottle), written out by `web/cmd/site`
  from the bottle version pinned in web/go.mod (currently commit 02b522e).
- `wasm_exec.js` is copied from the Go toolchain that built `cx.wasm`.
- `cxcorefile.wasm` is `web/cmd/cxcorefile`, which runs the CX IR form: the
  serialized program that `cx build` appends to a file, with no parser. It is
  the `cx-core` of skycoin-cx for js/wasm, where a core cannot read its own
  file, so the file is its first argument. It links the core opcodes only.

Each run writes the editor contents to `/home/cx/main.cx` in bottle's in-memory
filesystem and starts `cx /home/cx/main.cx` as a new bottle process. The CX
compiler keeps global state and exits the process on a compile error, so a
fresh process per run keeps runs apart. The process runs in a Worker, so a
program that never ends is stopped after 20 seconds or by the stop button.

"run with input" runs the program on the page thread instead, because the
stdin of a Worker process reads EOF. Each line typed into the input box goes
to the program's standard input, which CX `read()` returns. A program run
this way that never ends can be stopped only between two reads.

A Worker needs the page to be cross-origin isolated. GitHub Pages cannot send
the headers, so `coi-sw.js` adds them and the page reloads once on the first
visit. Without isolation the program runs on the page thread and an endless
loop cannot be stopped.

CX programs can use the `net` library in the browser only on loopback inside
one process: Go's js/wasm network is an in-memory one, and a browser gives a
page no TCP or UDP socket.

`.github/workflows/pages.yml` builds the site and publishes it on every push
to main.

# Browser tests

`make browser-test` builds the site and the cx-forth REPL page into
`_site/forth`, then runs every folder of `browsertest/tests` in a new tab of a
headless Brave (`BROWSERTEST_BROWSER` names another Chrome) through the
DevTools protocol. The page is served with the cross-origin isolation
headers. Results go to `browsertest/results/<time>/<test>/`, with
`summary.txt` for the run. `browsertest` is its own Go module, which pins
skycoin-cx-forth and the `microagents` module of skycoin-cx-playground-android
whose tapes the tests replay. `BROWSERTEST_FLAGS="-cx PATH"` names the native
cx that host steps run.

A test folder holds `steps.txt` and `expect.txt`, one step or check per line.
`$test`, `$results` and `$microagents` expand to the test folder, its results
folder and the microagents folder.

| Step | Does |
| --- | --- |
| `open PATH` | loads the page and waits until it is complete |
| `wait-text SELECTOR TEXT`, `wait-eval JS` | waits up to 60 s for the text in the element, or for the expression to be true |
| `click SELECTOR` | presses and releases the left mouse button at the centre of the element |
| `key NAME...` | Enter, Tab, Backspace, Escape, the arrow keys or ctrl+a to ctrl+z, as key events |
| `type TEXT`, `type-file FILE` | one key event per character, ENTER for a newline |
| `files PATH FOLDER` | copies a host file or folder into bottle's filesystem |
| `spawn NAME CWD ARGV...` | runs a bottle process in a Worker, saves `NAME.stdout`, `NAME.stderr` and `NAME.exit` |
| `pull FOLDER NAME` | copies a folder of bottle's filesystem to `NAME/` in the results |
| `grid SELECTOR NAME` | saves the element's text as `NAME.grid.txt`, one line per row, and its size in character cells as `NAME.grid-size.txt` |
| `screenshot NAME` | saves the 1024x768 page as `NAME.png` |
| `host NAME CWD COMMAND` | runs a shell command on the host in a results folder, with `$CX` set |
| `shell NAME CWD COMMAND` | runs a command line in a shell of the workstation page (`desk.html`), saves `NAME.stdout`, `NAME.stderr` and `NAME.exit` |
| `eval JS`, `wait MS` | runs JavaScript in the page, or pauses |

| Check | Passes when |
| --- | --- |
| `exit NAME CODE` | the process or host command exited with CODE |
| `file-equals RESULT FILE` | the saved file equals the file |
| `contains RESULT TEXT`, `contains-file RESULT FILE` | the saved file contains the text, or the whole file |
| `png NAME` | the screenshot is a 1024x768 PNG |
| `differs RESULT RESULT` | the two saved files differ |
| `same RESULT RESULT` | the two saved files are the same |

| Test | Checks |
| --- | --- |
| `cx-source-typed-run` | a program typed into the editor key by key runs, the output and editor grids and two screenshots |
| `ui-example-select-keyboard` | TAB and ARROWDOWN pick an example from the list, and it runs |
| `cx-stdin-typed-lines` | lines typed into the input box reach CX `read()` of a running program, in order |
| `cx-ir-build-and-run` | `cx build` in the browser and on the host, and `cxcorefile` runs both IR files in the browser |
| `cx-net-tcp-loopback`, `cx-net-udp-loopback` | the TCP and UDP tests of skycoin-cx run live on loopback and from their replay records |
| `forth-repl-typed-lines` | lines typed into the cx-forth REPL, ARROWUP history, and `CX-RUN fib` equal to the host cxforth |
| `microagent-<agent>-replay` | two replays of the femtoclaw, mimiclaw and zclaw CX ports print their expected output |
| `codingbench-<agent>-<task>` | the scripted-stub tape of each codingbench task replays with `++apply=1`, and the task's `check.sh` accepts the files |
| `workstation-desk` | the workstation runs cx from websh: several files and packages, a pipe, exit codes, file IO, an image, zip and unzip, and a CX HTTP server that a CX client, the page and the netscrape window reach over vnet |
| `workstation-native-compare` | programs of skycoin-cx tests/ give the same output, errors and exit status in the workstation shell as the native cx |
| `workstation-worker-interrupt-stdin` | cx runs in a Worker: an endless loop leaves the page working and Ctrl+C stops it with status 130, and lines typed in the terminal reach `read()` until Ctrl+D |

`browsertest/microagent-tests.sh` writes the micro agent folders from the
pinned microagents module.

# Workstation

`desk.html` is a workstation in the same page set, the shape of
[shipyard](https://github.com/0magnet/shipyard): a [desk](https://github.com/0magnet/desk)
of windows with [websh](https://github.com/0magnet/websh) terminals, a file
manager, websh's editor and the [netscrape](https://github.com/0magnet/netscrape)
browser, all on bottle's jsfs. It is `web/cmd/cxdesk`, built into
`cxdesk.wasm`, with `cx-core.wasm` and `cx-core-image.wasm`, the cores of
skycoin-cx that run what `cx build` makes.

- `cx`, `cx-core`, `cx-core-image` and `cxcorefile` are programs in `/bin`,
  fetched the first time they run. `cx main.cx lib.cx`, `cx folder`, `cx
  build --target=image`, stdin, pipes and exit codes work as in a native
  shell, and each run is a new process.
- The home folder, `/home/user`, is kept in IndexedDB between visits. The
  first visit copies the examples and `web/seed` there.
  `project-save DIR` downloads a folder as a zip file and `project-load`
  brings a zip file, files or a folder back. `zip`, `unzip`, `download` and
  `upload` do it file by file.
- The `os` library of cx reads and writes jsfs. TCP on 127.0.0.1 is bottle's
  vnet, shared by every process in the tab, so a CX server started with `cx
  --enable=net http-server.cx &` is dialed by another CX process and opened
  by `browse 127.0.0.1:8080`. `netscrape.wasm` is fetched when a browser
  window first opens.
- `edit FILE` edits in the terminal, `open FILE` and a double click in the
  file manager open an editor window.

cx and its cores run in a Worker, so a program that never ends leaves the
page working and Ctrl+C stops it. Lines typed into the terminal and pipes
reach its stdin. A run with `--enable=net` runs on the page thread instead,
where vnet is, and a net program that never waits there holds the page.

# Server

The server is the root module and still builds against skycoin-cx
v0.9.0-alpha1, whose package names it uses. The pages are built from the
`web` module, which pins the skycoin-cx they run.

- Run `make run`. It builds the site and serves it with the API below:

```
Starting web service for CX playground on http://127.0.0.1:5336/
```

- In another terminal, run this as a hello world example:

```
curl -H "Content-Type: application/json" -X POST -d '{"code": "package main; import \"sys/string\"; func main() {string.PrintLine(\"Hello, world!\");}"}' http://localhost:5336/eval
```

- Get example file list
```
curl -H "Content-Type: application/json" -X GET http://localhost:5336/playground/examples
```

- Get example file content
```
curl -H "Content-Type: application/json" -X POST  -d '{"examplename": "example1.cx"}' http://localhost:5336/playground/examples/code
```

Each `/eval` and `/showast` request compiles and runs the program in a child
process, which is this binary started with `-cx-eval` or `-cx-ast`. The CX
compiler keeps process wide state, so reusing one process gave wrong results
from the second program on. A child that runs longer than 20 seconds is
killed and the reply ends with `Timed out.`

The Dockerfile predates the move to skycoin-cx and has not been rebuilt. It
needs skycoin-cx to be published, or both repositories in the build context.
