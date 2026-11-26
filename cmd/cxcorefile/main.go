// Command cxcorefile runs the serialized CX program appended to a file that
// cx build made, the CX IR form, with no parser. It is the cx-core of
// skycoin-cx t6-cx-core-run for js/wasm, where a core cannot read its own
// file, so the file is the first argument and the rest are os.Args. It links
// the core opcodes only, so a program that calls a system library is refused.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-cx-runtime-printer"
	core "github.com/skycoin/skycoin-cx/t5-cx-runtime-core"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cxcorefile FILE-MADE-BY-CX-BUILD [ARGS...]")
		os.Exit(2)
	}
	os.Exit(int(coreFileRun(os.Args[1], os.Args[2:])))
}

// coreFileRun loads the program appended to path and runs it with args as
// os.Args. It returns the exit code.
func coreFileRun(path string, args []string) int32 {
	core.ExecutorMemoryLimitsClamp()
	program, fileError := core.CoreTrailerRead(path)
	if fileError != nil {
		fmt.Fprintf(os.Stderr, "cxcorefile: %s: %s\n", fileError.FileErrorPath, fileError.FileErrorMessage)
		return types.CX_COMPILATION_ERROR
	}
	if program == nil {
		fmt.Fprintf(os.Stderr, "cxcorefile: no CX program is appended to %s, make one with cx build\n", path)
		return types.CX_COMPILATION_ERROR
	}
	core.CoreRegister(core.CoreProgramMake())
	ast.OpcodeDispatch = core.CoreOpcodeDispatch
	prgrm, err := ast.Deserialize(program, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cxcorefile: %s\n", err)
		return types.CX_COMPILATION_ERROR
	}
	if missing := core.CoreOpcodesMissing(prgrm); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "cxcorefile: the program uses opcodes this core does not include: %s\n", strings.Join(missing, ", "))
		return types.CX_COMPILATION_ERROR
	}
	core.CoreLibrariesEnabledResolve(prgrm)
	printer.PrinterCompileErrorsRegister(prgrm)
	prgrm.ProgramStdinInit(os.Stdin)
	return core.ExecutorRun(prgrm, core.ExecutorAuto, args)
}
