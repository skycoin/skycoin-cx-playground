// Command site writes the static playground into a directory: the page from
// dist, bottle's scripts, and the top level examples with an index. The
// Makefile adds cx.wasm and wasm_exec.js.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0magnet/bottle"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: site <outdir>")
		os.Exit(2)
	}
	if err := build(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(out string) error {
	if err := os.CopyFS(out, os.DirFS("dist")); err != nil {
		return err
	}
	scripts := map[string][]byte{
		"jsfs.js":         bottle.JSFS(),
		"fsbridge.js":     bottle.FSBridgeJS(),
		"proc.js":         bottle.ProcJS(),
		"coi-sw.js":       bottle.COISWJS(),
		"coi-register.js": bottle.COIRegisterJS(),
	}
	for name, b := range scripts {
		if err := os.WriteFile(filepath.Join(out, name), b, 0o644); err != nil {
			return err
		}
	}

	ents, err := os.ReadDir("examples")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(out, "examples"), 0o755); err != nil {
		return err
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cx") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("examples", e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "examples", e.Name()), b, 0o644); err != nil {
			return err
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	idx, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "examples", "index.json"), idx, 0o644)
}
