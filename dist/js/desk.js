// The workstation page: it restores the files kept in IndexedDB, puts cx and
// the cores in /bin, seeds the home folder on the first visit and starts
// cxdesk.wasm, the desk with its terminals, files, editor and browser.

const boot = document.getElementById("boot");
const bootLog = (line) => { boot.textContent += line + "\n"; };
const fileExists = (path) => {
    try {
        return jsfs.readFile(path) !== null;
    } catch (e) {
        return false;
    }
};

// netscrape fetches a loopback address over bottle's vnet, where a CX server
// in this tab listens, and everything else with the page's fetch.
window.__netscrapeFetch = (url) => {
    try {
        const u = new URL(url.indexOf("://") < 0 ? "http://" + url : url);
        const loopback = ["127.0.0.1", "localhost", "[::1]", "::1"].includes(u.hostname);
        const port = parseInt(u.port || "80", 10);
        if (loopback && window.vnet) {
            return vnet.httpFetch(port, "GET", (u.pathname || "/") + (u.search || ""), null, {})
                .then((r) => new Response(r.body, {status: r.status || 200, headers: r.headers || {}}));
        }
    } catch (e) {
        // not a URL, fetch says why
    }
    return fetch(url);
};
window.__netscrapeStartLinks = [{label: "CX server on 127.0.0.1:8080", url: "http://127.0.0.1:8080/"}];
// The browser is fetched the first time a window opens it.
proc.registerURL("/opt/netscrape/browser.wasm", "netscrape.wasm");

(async () => {
    const seedMarker = "/home/user/.workstation-seeded";
    const sizes = fetch("wasm-sizes.txt").then((r) => r.text());
    const seedFiles = fetch("workstation-seed.json").then((r) => r.json());
    const deskModule = fetch("cxdesk.wasm").then((r) => r.arrayBuffer());

    // The home folder survives a reload. /bin is written again on every
    // visit, so a new cx.wasm reaches a returning visitor, and /tmp is not kept.
    const restored = await jsfs.persist.enable("cx-workstation", {
        exclude: (p) => p === "/tmp" || p.startsWith("/tmp/") || p === "/bin" || p.startsWith("/bin/"),
    });
    bootLog(restored.restored ? "restored your files" : "first visit");
    jsfs.mkdirp("/bin");
    jsfs.mkdirp("/tmp");
    jsfs.mkdirp("/home/user");
    // The programs are fetched the first time they run.
    for (const line of (await sizes).split("\n")) {
        const [file, size] = line.split(" ");
        if (file && size) {
            jsfs.writeLazy("/bin/" + file.replace(/\.wasm$/, ""), parseInt(size, 10), file);
        }
    }
    bootLog("cx, cx-core, cx-core-image and cxcorefile are in /bin");
    if (!fileExists(seedMarker)) {
        const seed = await seedFiles;
        for (const p of Object.keys(seed)) {
            jsfs.mkdirp(p.replace(/\/[^/]*$/, ""));
            if (!fileExists(p)) {
                jsfs.writeFile(p, seed[p]);
            }
        }
        jsfs.writeFile(seedMarker, "the examples and demo were copied here once\n");
        bootLog("copied " + Object.keys(seed).length + " example files to /home/user");
    }
    await jsfs.persist.flush();

    const ready = new Promise((resolve) => { window.__cxdeskReady = resolve; });
    const go = new Go();
    const {instance} = await WebAssembly.instantiate(await deskModule, go.importObject);
    go.run(instance);
    await ready;
    bootLog("ready");
    window.cxdeskReady = true;
})().catch((e) => {
    bootLog("ERROR " + e + "\n" + ((e && e.stack) || ""));
});
