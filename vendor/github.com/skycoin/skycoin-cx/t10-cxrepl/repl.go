package repl

//put the repo command stuff here

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/skycoin/skycoin-cx/t2-globals"            // thin: only FoundCompileErrors; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"                // thin: only InREPL, SourceFileGidNil; no depth gain
	"github.com/skycoin/skycoin-cx/t4-execute"            // thin: only RunCompiled; no depth gain
	"github.com/skycoin/skycoin-cx/t5-cx-runtime-printer" // thin: only PrinterCompileErrorsPrint, PrinterRuntimeErrorPrint; no depth gain
	cxinit "github.com/skycoin/skycoin-cx/t7-init"        // thin: only MakeProgram, RegisterPackages; no depth gain

	"github.com/skycoin/skycoin-cx/t4-actions"                               // thin: only AST, LineNo; no depth gain
	cxpartialparsing "github.com/skycoin/skycoin-cx/t5-cxpartialparsing"     // thin: only CurrentFile, Parse, Program; no depth gain
	cxparsingcompletor "github.com/skycoin/skycoin-cx/t8-cxparsingcompletor" // thin: only Lexer, NewLexer, Parse; no depth gain
	cxparsering "github.com/skycoin/skycoin-cx/t9-cxparsing"                 // thin: only AddInitFunction, ErrCompilation; could move AddInitFunction to t4-actions
)

const VERSION = "0.8.0"

// VIOLATION: state outside State. Owner: t10-cxrepl; moves to ReplMng in phase 2.
var ReplTargetFn string = ""

// VIOLATION: state outside State. Owner: t10-cxrepl; moves to ReplMng in phase 2.
var ReplTargetStrct string = ""

// VIOLATION: state outside State. Owner: t10-cxrepl; moves to ReplMng in phase 2.
var ReplTargetMod string = ""

func unsafeEval(code string) (out string) {
	var lexer *cxparsingcompletor.Lexer

	// storing strings sent to standard output
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Sprintf("%s", err)
	}
	// Drain the pipe while the program runs, or output larger than the
	// pipe buffer blocks it.
	outC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		r.Close()
		outC <- buf.String()
	}()
	os.Stdout = w

	restored := false
	restore := func() string {
		if restored {
			return ""
		}
		restored = true
		w.Close()
		os.Stdout = old // restoring the real stdout
		return <-outC
	}

	defer func() {
		if r := recover(); r != nil {
			printer.PrinterCompileErrorsPrint(actions.AST)
			restore()
			out = fmt.Sprintf("%v", r)
			// lexer.Stop()
		}
	}()

	actions.LineNo = 0
	// Eval code has no file, and a file Gid of an earlier program is stale.
	actions.CurrentFile = ast.SourceFileGidNil
	cxpartialparsing.CurrentFile = ast.SourceFileGidNil
	globals.FoundCompileErrors = false

	// VIOLATION: init chaining. Init 7-8 again, a fresh program on REPL reset.
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0001 S0002.
	actions.AST = cxinit.MakeProgram()
	cxinit.RegisterPackages(actions.AST)
	cxpartialparsing.Program = actions.AST

	parseErrors := cxpartialparsing.Parse(code)

	actions.AST = cxpartialparsing.Program

	lexer = cxparsingcompletor.NewLexer(bytes.NewBufferString(code))
	parseErrors += cxparsingcompletor.Parse(lexer)
	printer.PrinterCompileErrorsPrint(actions.AST)
	if parseErrors > 0 || globals.FoundCompileErrors {
		restore()
		// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
		// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0003 S0004.
		actions.AST = cxinit.MakeProgram()
		cxinit.RegisterPackages(actions.AST)
		return cxparsering.ErrCompilation.Error()
	}
	err = cxparsering.AddInitFunction(actions.AST)
	if err != nil {
		restore()
		return fmt.Sprintf("%s", err)
	}

	actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
	actions.AST.CxIrState.StringPool.StringPoolFreeze()
	//err = actions.AST.RunCompiled(0, nil);
	err = execute.RunCompiled(actions.AST, 0, nil)
	if actions.AST.RuntimeError.RuntimeErrorRecordRaised {
		os.Exit(printer.PrinterRuntimeErrorPrint(actions.AST))
	}
	if err != nil {
		restore()
		// VIOLATION: init chaining. Init 7-8 again, a fresh program on REPL reset.
		// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
		// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0005 S0006.
		actions.AST = cxinit.MakeProgram()
		cxinit.RegisterPackages(actions.AST)
		return fmt.Sprintf("%s", err)
	}
	//Tod: If error equals nill?

	out = restore()

	// VIOLATION: init chaining. Init 7-8 again, a fresh program on REPL reset.
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0007 S0008.
	actions.AST = cxinit.MakeProgram()
	cxinit.RegisterPackages(actions.AST)
	return out
}

// Eval runs code as a CX program and returns its output. The caller must
// first run the init in t11-cx Run: InitConstants, MakeProgram, RegisterPackages, RegisterOpcodes.
func Eval(code string) string {
	runtime.GOMAXPROCS(2)
	ch := make(chan string, 1)

	var result string

	go func() {
		result = unsafeEval(code)
		ch <- result
	}()

	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()

	select {
	case <-ch:
		return result
	case <-timer.C:
		// VIOLATION: init chaining. Init 7-8 again, a fresh program on REPL reset.
		// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
		// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0009 S0010.
		actions.AST = cxinit.MakeProgram()
		cxinit.RegisterPackages(actions.AST)
		return "Timed out."
	}
}

func Repl() {
	fmt.Println("CX", VERSION)
	fmt.Println("More information about CX is available at https://github.com/skycoin/skycoin-cx")

	ast.InREPL = true

	// fi := bufio.NewReader(os.NewFile(0, "stdin"))
	fi := bufio.NewReader(os.Stdin)
	actions.AST.ProgramStdinInit(fi)
	// scanner := bufio.NewScanner(os.Stdin)

	for {
		actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
		var inp string
		var ok bool

		printPrompt()

		if inp, ok = readline(fi); ok {
			if ReplTargetFn != "" {
				inp = fmt.Sprintf(":func %s {\n%s\n}\n", ReplTargetFn, inp)
			}
			if ReplTargetMod != "" {
				inp = fmt.Sprintf("%s", inp)
			}
			if ReplTargetStrct != "" {
				inp = fmt.Sprintf(":struct %s {\n%s\n}\n", ReplTargetStrct, inp)
			}

			b := bytes.NewBufferString(inp)

			cxparsingcompletor.Parse(cxparsingcompletor.NewLexer(b))
			printer.PrinterCompileErrorsPrint(actions.AST)
			//yyParse(NewLexer(b))
		} else {
			if ReplTargetFn != "" {
				ReplTargetFn = ""
				continue
			}

			if ReplTargetStrct != "" {
				ReplTargetStrct = ""
				continue
			}

			if ReplTargetMod != "" {
				ReplTargetMod = ""
				continue
			}

			fmt.Printf("\nBye!\n")
			break
		}
	}
}

func printPrompt() {
	if ReplTargetMod != "" {
		fmt.Println(fmt.Sprintf(":package %s ...", ReplTargetMod))
		fmt.Printf("* ")
	} else if ReplTargetFn != "" {
		fmt.Println(fmt.Sprintf(":func %s {...", ReplTargetFn))
		fmt.Printf("\t* ")
	} else if ReplTargetStrct != "" {
		fmt.Println(fmt.Sprintf(":struct %s {...", ReplTargetStrct))
		fmt.Printf("\t* ")
	} else {
		fmt.Printf("* ")
	}
}

// ----------------------------------------------------------------
//                     Utility functions

func readline(fi *bufio.Reader) (string, bool) {
	s, err := fi.ReadString('\n')

	s = strings.Replace(s, "\n", "", -1)
	s = strings.Replace(s, "\r", "", -1)

	for _, ch := range s {
		if ch == rune(4) {
			err = io.EOF
			break
		}
	}

	if err != nil {
		return "", false
	}

	return s, true
}
