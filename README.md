# CX Playground

The CX Playground runs programs in Skycoin's programming language, CX, in the
browser. Try it at https://skycoin.github.io/skycoin-cx-playground/

# In the browser

The page needs no server. `make site` builds it into `./_site`:

- `cx.wasm` is the `cx` command of [skycoin-cx](https://github.com/skycoin/skycoin-cx)
  built for js/wasm with the default tags (cipher, cxos and regexp in, cxfx out).
  It is a `tool` dependency in go.mod, so its version is pinned there.
- `jsfs.js`, `fsbridge.js`, `proc.js`, `coi-sw.js` and `coi-register.js` come
  from [bottle](https://github.com/0magnet/bottle), written out by `cmd/site`
  from the bottle version pinned in go.mod (currently commit 2b922f2).
- `wasm_exec.js` is copied from the Go toolchain that built `cx.wasm`.

Each run writes the editor contents to `/home/cx/main.cx` in bottle's in-memory
filesystem and starts `cx /home/cx/main.cx` as a new bottle process. The CX
compiler keeps global state and exits the process on a compile error, so a
fresh process per run keeps runs apart. The process runs in a Worker, so a
program that never ends is stopped after 20 seconds or by the stop button.

A Worker needs the page to be cross-origin isolated. GitHub Pages cannot send
the headers, so `coi-sw.js` adds them and the page reloads once on the first
visit. Without isolation the program runs on the page thread and an endless
loop cannot be stopped.

`.github/workflows/pages.yml` builds the site and publishes it on every push
to main.

# Server

- Run `make run`. It builds the site and serves it with the API below:

```
Starting web service for CX playground on http://127.0.0.1:5336/
```

- In another terminal, run this as a hello world example:

```
curl -H "Content-Type: application/json" -X POST -d '{"code": "package main; func main() {str.print(\"Hello, world!\");}"}' http://localhost:5336/eval
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
