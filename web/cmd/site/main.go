// Command site writes the static playground of the repository into a
// directory: the pages from dist, bottle's scripts, the top level examples with an index, and the
// files the workstation page seeds. The Makefile adds the wasm programs and
// wasm_exec.js.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0magnet/bottle"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: site <repository> <outdir>")
		os.Exit(2)
	}
	out, err := filepath.Abs(os.Args[2])
	if err == nil {
		err = os.Chdir(os.Args[1])
	}
	if err == nil {
		err = build(out)
	}
	if err != nil {
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
		"vnet.js":         bottle.VNetJS(),
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
	if err := os.WriteFile(filepath.Join(out, "examples", "index.json"), idx, 0o644); err != nil {
		return err
	}
	return workstationSeedWrite(filepath.Join(out, "workstation-seed.json"))
}

// workstationSeedHome is the home folder of the workstation page.
const workstationSeedHome = "/home/user"

// workstationSeedWrite writes the files the workstation puts in its home
// folder on the first visit, as a JSON object of path and text: the
// examples under examples and web/seed at the top. Folders of
// pictures and fonts, which only the graphics examples use, are left out.
func workstationSeedWrite(out string) error {
	seed := map[string]string{}
	add := func(root, under string) error {
		return filepath.WalkDir(root, func(p string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "textures" || entry.Name() == "fonts" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".cx", ".md", ".txt":
			default:
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			seed[path.Join(under, filepath.ToSlash(rel))] = string(b)
			return nil
		})
	}
	if err := add("examples", workstationSeedHome+"/examples"); err != nil {
		return err
	}
	if err := add(filepath.Join("web", "seed"), workstationSeedHome); err != nil {
		return err
	}
	b, err := json.Marshal(seed)
	if err != nil {
		return err
	}
	return os.WriteFile(out, b, 0o644)
}
