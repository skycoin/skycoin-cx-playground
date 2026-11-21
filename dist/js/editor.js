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
jsfs.mkdirp("/home/cx");
const env = {PATH: "/bin", HOME: "/home/cx", USER: "cx", CXPATH: "/home/cx/cx"};
const offThread = globalThis.crossOriginIsolated === true;
status.textContent = offThread ? "Ready." :
    "Ready. The page is not cross-origin isolated, so a program that never ends cannot be stopped.";

let current = null;

function setBusy(busy) {
    $("run").disabled = busy;
    $("ast").disabled = busy;
    $("stop").disabled = !busy;
}

// run starts cx with args and the editor contents as /home/cx/main.cx. The
// result is also kept on window.cxLast.
function run(args) {
    if (current) {
        return current;
    }
    jsfs.writeFile("/home/cx/main.cx", code.value + "\n");
    output.textContent = "";
    const sink = () => {
        const dec = new TextDecoder();
        return (b) => { output.textContent += dec.decode(b, {stream: true}); };
    };
    const spawn = offThread ? proc.spawnWorker : proc.spawn;
    const start = performance.now();
    const h = spawn({argv: ["cx", ...args, "/home/cx/main.cx"], env, cwd: "/home/cx", stdout: sink(), stderr: sink()});
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; h.kill(true); }, timeoutMs);
    $("stop").onclick = () => h.kill(true);
    setBusy(true);
    status.textContent = "Running...";
    current = h.exited.then((c) => new Promise((res) => setTimeout(() => res(c), 0))).then((c) => {
        clearTimeout(timer);
        const ms = Math.round(performance.now() - start);
        status.textContent = timedOut ? "Stopped after " + timeoutMs / 1000 + " s." :
            "Exit status " + c + ", " + ms + " ms.";
        setBusy(false);
        current = null;
        window.cxLast = {exit: c, timedOut, ms, output: output.textContent};
        return window.cxLast;
    });
    return current;
}

$("run").addEventListener("click", () => run([]));
$("ast").addEventListener("click", () => run(["-ast"]));
