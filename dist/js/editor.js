const helloWorld = `package main

func main() {
    str.print("Hello, World!")
}`;

const $ = (id) => document.getElementById(id);
const code = $("code");
const output = $("output");
code.value = helloWorld;

// Tab inserts four spaces instead of leaving the text area.
code.addEventListener("keydown", (e) => {
    if (e.key !== "Tab") {
        return;
    }
    e.preventDefault();
    code.setRangeText("    ", code.selectionStart, code.selectionEnd, "end");
});

const post = (url, body) => fetch(url, {
    method: "POST",
    headers: {"Content-Type": "application/json;charset=utf-8"},
    body: JSON.stringify(body),
}).then((r) => r.text());

const show = (p) => p.then((t) => { output.textContent = t; }, (e) => { output.textContent = String(e); });

fetch("playground/examples").then((r) => r.json()).then((names) => {
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
    post("playground/examples/code", {examplename: name}).then((t) => { code.value = t; });
});

$("run").addEventListener("click", () => {
    show(post("eval", {code: code.value}));
    fetch("mem", {method: "POST"}).then((r) => r.text()).then((t) => { $("memory").textContent = t; });
});

$("ast").addEventListener("click", () => {
    show(post("showast", {code: code.value}));
});
