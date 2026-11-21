package playground

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/skycoin/skycoin-cx/t4-execute"

	"github.com/skycoin/skycoin-cx/t4-actions"
	parsingcompletor "github.com/skycoin/skycoin-cx/t8-cxparsingcompletor"
	cxparsing "github.com/skycoin/skycoin-cx/t9-cxparsing"

	"github.com/skycoin/skycoin-cx/t0-types"
	globals "github.com/skycoin/skycoin-cx/t2-globals"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
)

var (
	exampleCollection map[string]string
	exampleNames      []string
	examplesDir       string
)

type ExampleContent struct {
	ExampleName string
}

var InitPlayground = func(workingDir string) error {
	types.Allocator = cxast.AllocateSeq

	examplesDir = filepath.Join(workingDir, "./examples")
	exampleCollection = make(map[string]string)

	exampleInfoList, err := ioutil.ReadDir(examplesDir)
	if err != nil {
		fmt.Printf("Fail to get file list under examples dir: %s\n", err.Error())
		return err
	}
	for _, exp := range exampleInfoList {
		if exp.IsDir() {
			continue
		}
		path := filepath.Join(examplesDir, exp.Name())
		bytes, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Printf("Fail to read example file %s\n", path)
			// coninue if fail to read the current example file
			continue
		}

		exampleName := exp.Name()
		exampleNames = append(exampleNames, exampleName)
		exampleCollection[exampleName] = string(bytes)
	}

	return nil
}

func timeTrack(start time.Time, w http.ResponseWriter) {
	elapsed := time.Since(start)
	fmt.Fprintf(w, "%s", "\n")
	fmt.Fprintf(w, "%s", elapsed)
}

func GetExampleFileList(w http.ResponseWriter, r *http.Request) {

	exampleNamesBytes, err := json.Marshal(exampleNames)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprint(w, string(exampleNamesBytes))
}

func GetExampleFileContent(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var (
		b   []byte
		err error
	)
	if b, err = ioutil.ReadAll(r.Body); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var example ExampleContent
	if err = json.Unmarshal(b, &example); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	content, err := getExampleFileContent(example.ExampleName)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprint(w, content)
}

var getExampleFileContent = func(exampleName string) (string, error) {
	exampleContent, ok := exampleCollection[exampleName]
	if !ok {
		err := fmt.Errorf("example name %s not found", exampleName)

		return "", err
	}
	return exampleContent, nil
}

type SourceCode struct {
	Code string `json:"code,omitempty"`
}

func RunProgram(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var b []byte
	var err error
	if b, err = ioutil.ReadAll(r.Body); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var source SourceCode
	if err := json.Unmarshal(b, &source); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	defer timeTrack(time.Now(), w)
	fmt.Fprintf(w, "%s", eval(source.Code+"\n"))
}

func ShowAst(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var b []byte
	var err error
	if b, err = ioutil.ReadAll(r.Body); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var source SourceCode
	if err := json.Unmarshal(b, &source); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), 500)
	}
	fmt.Fprintf(w, "%s", ast(source.Code+"\n"))
}

// evalTimeout bounds one program run, compile included.
const evalTimeout = 20 * time.Second

// The CX compiler keeps process wide state that is not reset between
// programs, so each request compiles and runs in a fresh child process.
// The child is this same binary started with ChildEvalArg or ChildASTArg.
const (
	ChildEvalArg = "-cx-eval"
	ChildASTArg  = "-cx-ast"
)

// RunChild handles the child modes. It reads CX source on stdin, writes
// the program output or AST to stdout and returns the exit code. ok is
// false when args do not select a child mode.
func RunChild(args []string) (code int, ok bool) {
	if len(args) < 2 || (args[1] != ChildEvalArg && args[1] != ChildASTArg) {
		return 0, false
	}
	types.Allocator = cxast.AllocateSeq
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Println(err)
		return 1, true
	}
	if err := compile(string(src) + "\n"); err != nil {
		fmt.Println(err)
		return 1, true
	}
	if args[1] == ChildASTArg {
		actions.AST.PrintProgram()
		return 0, true
	}
	if err := execute.RunCompiled(actions.AST, 0, nil); err != nil {
		fmt.Println(err)
		return 1, true
	}
	return 0, true
}

func runChild(mode, code string) string {
	self, err := os.Executable()
	if err != nil {
		return err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, self, mode)
	cmd.Stdin = strings.NewReader(code)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out) + "Timed out."
	}
	if err != nil && len(out) == 0 {
		return err.Error()
	}
	return string(out)
}

func eval(code string) string { return runChild(ChildEvalArg, code) }

func ast(code string) string { return runChild(ChildASTArg, code) }

func GetMemStatus(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(w, "%s", strconv.FormatUint(m.HeapAlloc/1049000, 10)+"Mb Allocated / "+strconv.FormatUint(m.Sys/1049000, 10)+"Mb Reserved")
}

// compile parses code into a fresh actions.AST the same way cmd/cx does.
func compile(code string) error {
	parsingcompletor.InitCXCore()
	globals.FoundCompileErrors = false
	actions.LineNo = 0

	if err := cxparsing.ParseSourceStrings([]string{code}, []string{"main.cx"}); err != nil {
		return err
	}
	if _, err := actions.AST.GetFunction(actions.AST.CxIrState.CxIrStateNameMain, actions.AST.CxIrState.CxIrStateNameMain); err != nil {
		return fmt.Errorf("no main function in package main")
	}
	return cxparsing.AddInitFunction(actions.AST)
}
