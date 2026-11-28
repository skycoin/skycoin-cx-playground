//go:build js && wasm

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"syscall/js"

	"github.com/0magnet/afero"
	"github.com/0magnet/sh/v3/interp"
	"github.com/0magnet/websh/shell"
)

// A project is a folder. project-save downloads one as a zip file and
// project-load brings a zip file, some files or a folder back in. zip and
// unzip do the same between the folder and a zip file in the filesystem.

func projectAppletsRegister() {
	shell.RegisterApplet("zip", "zip ARCHIVE PATH...: put files and folders in a zip file", zipApplet)
	shell.RegisterApplet("unzip", "unzip ARCHIVE [-d DIR]: extract a zip file", unzipApplet)
	shell.RegisterApplet("project-save", "project-save [DIR]: download a folder as DIR.zip", projectSaveApplet)
	shell.RegisterApplet("project-load", "project-load [--folder] [DIR]: upload a zip file, files or a folder into DIR", projectLoadApplet)
}

// zipArchiveWrite writes every file under each of roots into a zip archive.
// A file is named by its path relative to base.
func zipArchiveWrite(fileSystem afero.Fs, base string, roots []string, output io.Writer) (int, error) {
	archive := zip.NewWriter(output)
	count := 0
	for _, root := range roots {
		err := afero.Walk(fileSystem, root, func(filePath string, entry fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			name := strings.TrimPrefix(strings.TrimPrefix(filePath, base), "/")
			if name == "" {
				return nil
			}
			if entry.IsDir() {
				_, err := archive.Create(name + "/")
				return err
			}
			content, err := afero.ReadFile(fileSystem, filePath)
			if err != nil {
				return err
			}
			header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: entry.ModTime()}
			header.SetMode(entry.Mode())
			writer, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			_, err = writer.Write(content)
			count++
			return err
		})
		if err != nil {
			return count, err
		}
	}
	return count, archive.Close()
}

// zipArchiveExtract writes the files of a zip archive under directory.
func zipArchiveExtract(fileSystem afero.Fs, content []byte, directory string) (int, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, member := range archive.File {
		name := path.Clean("/" + member.Name)
		if name == "/" {
			continue
		}
		target := path.Join(directory, name)
		if member.FileInfo().IsDir() {
			if err := fileSystem.MkdirAll(target, 0o755); err != nil {
				return count, err
			}
			continue
		}
		if err := fileSystem.MkdirAll(path.Dir(target), 0o755); err != nil {
			return count, err
		}
		reader, err := member.Open()
		if err != nil {
			return count, err
		}
		body, err := io.ReadAll(reader)
		reader.Close() //nolint:errcheck // read only
		if err != nil {
			return count, err
		}
		mode := member.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := afero.WriteFile(fileSystem, target, body, mode); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func zipApplet(_ context.Context, s *shell.Shell, hc *interp.HandlerContext, args []string) int {
	var operands []string
	for _, argument := range args {
		if argument != "-r" {
			operands = append(operands, argument)
		}
	}
	if len(operands) < 2 {
		shell.Println(hc.Stderr, "usage: zip ARCHIVE PATH...")
		return 2
	}
	var roots []string
	for _, operand := range operands[1:] {
		roots = append(roots, shell.Resolve(hc, operand))
	}
	var archive bytes.Buffer
	count, err := zipArchiveWrite(s.FS, hc.Dir, roots, &archive)
	if err != nil {
		shell.Printf(hc.Stderr, "zip: %v\n", err)
		return 1
	}
	if _, err := shell.WriteFile(s, hc, operands[0], archive.Bytes()); err != nil {
		shell.Printf(hc.Stderr, "zip: %v\n", err)
		return 1
	}
	shell.Printf(hc.Stdout, "%s: %d files, %d bytes\n", operands[0], count, archive.Len())
	return 0
}

func unzipApplet(_ context.Context, s *shell.Shell, hc *interp.HandlerContext, args []string) int {
	directory := hc.Dir
	var archiveName string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-d" && i+1 < len(args):
			i++
			directory = shell.Resolve(hc, args[i])
		case archiveName == "":
			archiveName = args[i]
		}
	}
	if archiveName == "" {
		shell.Println(hc.Stderr, "usage: unzip ARCHIVE [-d DIR]")
		return 2
	}
	content, err := shell.ReadFile(s, hc, archiveName)
	if err != nil {
		shell.Printf(hc.Stderr, "unzip: %v\n", err)
		return 1
	}
	count, err := zipArchiveExtract(s.FS, content, directory)
	if err != nil {
		shell.Printf(hc.Stderr, "unzip: %v\n", err)
		return 1
	}
	shell.Printf(hc.Stdout, "%d files into %s\n", count, directory)
	return 0
}

// projectDownload zips directory, its own name as the top folder, and hands
// the browser the zip file to save.
func projectDownload(fileSystem afero.Fs, directory string) (string, int, error) {
	directory = path.Clean(directory)
	information, err := fileSystem.Stat(directory)
	if err != nil {
		return "", 0, err
	}
	if !information.IsDir() {
		return "", 0, fmt.Errorf("%s is not a folder", directory)
	}
	name := path.Base(directory)
	if name == "/" || name == "." {
		name = "project"
	}
	var archive bytes.Buffer
	count, err := zipArchiveWrite(fileSystem, path.Dir(directory), []string{directory}, &archive)
	if err != nil {
		return "", 0, err
	}
	browserDownload(name+".zip", archive.Bytes())
	return name + ".zip", count, nil
}

// browserDownload hands the browser bytes to save as name.
func browserDownload(name string, content []byte) {
	payload := js.Global().Get("Uint8Array").New(len(content))
	js.CopyBytesToJS(payload, content)
	parts := js.Global().Get("Array").New()
	parts.Call("push", payload)
	blob := js.Global().Get("Blob").New(parts, map[string]any{"type": "application/zip"})
	address := js.Global().Get("URL").Call("createObjectURL", blob)
	anchor := js.Global().Get("document").Call("createElement", "a")
	anchor.Set("href", address)
	anchor.Set("download", name)
	anchor.Call("click")
	js.Global().Call("setTimeout", js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Get("URL").Call("revokeObjectURL", address)
		return nil
	}), 10000)
}

func projectSaveApplet(_ context.Context, s *shell.Shell, hc *interp.HandlerContext, args []string) int {
	directory := hc.Dir
	if len(args) > 0 {
		directory = shell.Resolve(hc, args[0])
	}
	name, count, err := projectDownload(s.FS, directory)
	if err != nil {
		shell.Printf(hc.Stderr, "project-save: %v\n", err)
		return 1
	}
	shell.Printf(hc.Stdout, "downloading %s, %d files\n", name, count)
	return 0
}

// pickedFile is one file the person picked, with the folder path the
// browser gives for a picked folder.
type pickedFile struct {
	name    string
	content []byte
}

// filePickerOpen shows the browser's file picker. It must run while the
// page still has the click or key that asked for it. The files arrive on
// the returned channel, nil when the person cancels.
func filePickerOpen(folder bool) <-chan []pickedFile {
	picked := make(chan []pickedFile, 1)
	input := js.Global().Get("document").Call("createElement", "input")
	input.Set("type", "file")
	if folder {
		input.Set("webkitdirectory", true)
	} else {
		input.Set("multiple", true)
	}
	var onChange, onCancel js.Func
	release := func() {
		onChange.Release()
		onCancel.Release()
	}
	onChange = js.FuncOf(func(js.Value, []js.Value) any {
		list := input.Get("files")
		go func() {
			defer release()
			var result []pickedFile
			for i := 0; i < list.Get("length").Int(); i++ {
				file := list.Index(i)
				buffer, err := promiseAwait(file.Call("arrayBuffer"))
				if err != nil {
					continue
				}
				bytesView := js.Global().Get("Uint8Array").New(buffer)
				content := make([]byte, bytesView.Get("length").Int())
				js.CopyBytesToGo(content, bytesView)
				name := file.Get("name").String()
				if relative := file.Get("webkitRelativePath"); relative.Type() == js.TypeString && relative.String() != "" {
					name = relative.String()
				}
				result = append(result, pickedFile{name: name, content: content})
			}
			picked <- result
		}()
		return nil
	})
	onCancel = js.FuncOf(func(js.Value, []js.Value) any {
		release()
		picked <- nil
		return nil
	})
	input.Set("onchange", onChange)
	input.Set("oncancel", onCancel)
	input.Call("click")
	return picked
}

// promiseAwait waits for a JavaScript promise.
func promiseAwait(promise js.Value) (js.Value, error) {
	settled := make(chan struct{})
	var value js.Value
	var failure error
	var onResolve, onReject js.Func
	onResolve = js.FuncOf(func(_ js.Value, arguments []js.Value) any {
		value = arguments[0]
		close(settled)
		return nil
	})
	onReject = js.FuncOf(func(_ js.Value, arguments []js.Value) any {
		failure = errors.New(js.Global().Get("String").Invoke(arguments[0]).String())
		close(settled)
		return nil
	})
	promise.Call("then", onResolve, onReject)
	<-settled
	onResolve.Release()
	onReject.Release()
	return value, failure
}

// projectFilesStore writes picked files under directory. A zip file is
// extracted, every other file is copied by its name.
func projectFilesStore(fileSystem afero.Fs, files []pickedFile, directory string) (int, error) {
	count := 0
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file.name), ".zip") && !strings.Contains(file.name, "/") {
			extracted, err := zipArchiveExtract(fileSystem, file.content, directory)
			count += extracted
			if err != nil {
				return count, fmt.Errorf("%s: %w", file.name, err)
			}
			continue
		}
		target := path.Join(directory, path.Clean("/"+file.name))
		if err := fileSystem.MkdirAll(path.Dir(target), 0o755); err != nil {
			return count, err
		}
		if err := afero.WriteFile(fileSystem, target, file.content, 0o644); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func projectLoadApplet(ctx context.Context, s *shell.Shell, hc *interp.HandlerContext, args []string) int {
	directory := hc.Dir
	folder := false
	for _, argument := range args {
		if argument == "--folder" || argument == "-folder" {
			folder = true
			continue
		}
		directory = shell.Resolve(hc, argument)
	}
	picked := filePickerOpen(folder)
	shell.Println(hc.Stdout, "pick a zip file, some files or a folder... (Ctrl+C to cancel)")
	var files []pickedFile
	select {
	case files = <-picked:
	case <-ctx.Done():
		return 130
	}
	if len(files) == 0 {
		shell.Println(hc.Stderr, "project-load: nothing picked")
		return 1
	}
	count, err := projectFilesStore(s.FS, files, directory)
	if err != nil {
		shell.Printf(hc.Stderr, "project-load: %v\n", err)
		return 1
	}
	shell.Printf(hc.Stdout, "%d files into %s\n", count, directory)
	return 0
}

// projectSaveFromMenu asks which folder to download and downloads it.
func projectSaveFromMenu() {
	answer := js.Global().Call("prompt", "Folder to download as a zip file", workstationHome)
	if !answer.Truthy() || strings.TrimSpace(answer.String()) == "" {
		return
	}
	if _, _, err := projectDownload(afero.NewOsFs(), strings.TrimSpace(answer.String())); err != nil {
		js.Global().Call("alert", "save project: "+err.Error())
	}
}

// projectLoadFromMenu picks a zip file or files and stores them in the home
// folder. It runs in the menu's click, which the file picker needs.
func projectLoadFromMenu() {
	picked := filePickerOpen(false)
	go func() {
		files := <-picked
		if len(files) == 0 {
			return
		}
		count, err := projectFilesStore(afero.NewOsFs(), files, workstationHome)
		if err != nil {
			js.Global().Call("alert", "load project: "+err.Error())
			return
		}
		js.Global().Get("console").Call("log", fmt.Sprintf("load project: %d files into %s", count, workstationHome))
	}()
}
