const helloWorld = `package main

import "sys/string"

func main() {
    string.PrintLine("Hello, World!")
}`;

// A run longer than this is killed, compile included.
const timeoutMs = 20000;

const $ = (id) => document.getElementById(id);
const code = $("code");
const output = $("output");
const status = $("status");
code.value = helloWorld;

// Tab inserts four spaces instead of leaving the text area.
code.addEventListener("keydown", (e) => {
    if (e.key !== "Tab") {
        return;
    }
    e.preventDefault();
    code.setRangeText("    ", code.selectionStart, code.selectionEnd, "end");
});

fetch("examples/index.json").then((r) => r.json()).then((names) => {
    for (const n of names) {
        $("list").add(new Option(n));
    }
});

$("list").addEventListener("change", (e) => {
    const name = e.target.value;
    if (name === "Hello, World") {
        code.value = helloWorld;
        return;
    }
    fetch("examples/" + encodeURIComponent(name)).then((r) => r.text()).then((t) => { code.value = t; });
});

// cx runs as a bottle process: a fresh wasm instance per run, so the
// compiler's global state and its os.Exit on errors end with the run.
proc.registerURL("/bin/cx", "cx.wasm");
// cxcorefile runs the CX IR form, a file cx build made, with no parser.
proc.registerURL("/bin/cxcorefile", "cxcorefile.wasm");
jsfs.mkdirp("/home/cx");
const env = {PATH: "/bin", HOME: "/home/cx", USER: "cx", CXPATH: "/home/cx/cx"};
const offThread = globalThis.crossOriginIsolated === true;
status.textContent = offThread ? "Ready." :
    "Ready. The page is not cross-origin isolated, so a program that never ends cannot be stopped.";

let current = null;

function setBusy(busy) {
    $("run").disabled = busy;
    $("runinput").disabled = busy;
    $("ast").disabled = busy;
    $("stop").disabled = !busy;
}

// run starts cx with args and the editor contents as /home/cx/main.cx. The
// result is also kept on window.cxLast. With withInput the program runs on
// the page thread, since a Worker child's stdin reads EOF, and each line of
// the input box goes to its stdin, which CX read() returns.
function run(args, withInput) {
    if (current) {
        return current;
    }
    jsfs.writeFile("/home/cx/main.cx", code.value + "\n");
    output.textContent = "";
    const sink = () => {
        const dec = new TextDecoder();
        return (b) => { output.textContent += dec.decode(b, {stream: true}); };
    };
    const spawn = offThread && !withInput ? proc.spawnWorker : proc.spawn;
    const start = performance.now();
    const h = spawn({argv: ["cx", ...args, "/home/cx/main.cx"], env, cwd: "/home/cx", stdout: sink(), stderr: sink(),
        stdin: withInput ? "pipe" : undefined});
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; h.kill(true); }, timeoutMs);
    $("stop").onclick = () => h.kill(true);
    const input = $("input");
    if (withInput) {
        input.disabled = false;
        input.focus();
        input.onkeydown = (e) => {
            if (e.key !== "Enter") {
                return;
            }
            e.preventDefault();
            output.textContent += input.value + "\n";
            h.stdin.write(new TextEncoder().encode(input.value + "\n"));
            input.value = "";
        };
    }
    setBusy(true);
    status.textContent = "Running...";
    current = h.exited.then((c) => new Promise((res) => setTimeout(() => res(c), 0))).then((c) => {
        clearTimeout(timer);
        const ms = Math.round(performance.now() - start);
        status.textContent = timedOut ? "Stopped after " + timeoutMs / 1000 + " s." :
            "Exit status " + c + ", " + ms + " ms.";
        setBusy(false);
        input.disabled = true;
        input.onkeydown = null;
        current = null;
        window.cxLast = {exit: c, timedOut, ms, output: output.textContent};
        return window.cxLast;
    });
    return current;
}

$("run").addEventListener("click", () => run([]));
$("runinput").addEventListener("click", () => run([], true));
$("ast").addEventListener("click", () => run(["-ast"]));
