package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only AssertFailed, RegisterOpcodes; no depth gain

	"github.com/skycoin/skycoin-cx/t0-util" // thin: only CXCreateFile, CXLogFile, CXOpenFile; no depth gain
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t1-profiling"
	repl "github.com/skycoin/skycoin-cx/t10-cxrepl" // thin: only Repl, ReplTargetFn; only user of cxrepl, could merge it here, max tier 11 to 10
	"github.com/skycoin/skycoin-cx/t2-globals"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-actions"
	"github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"
	cxinit "github.com/skycoin/skycoin-cx/t7-init"             // thin: only MakeProgram, RegisterPackages; no depth gain
	cxgo "github.com/skycoin/skycoin-cx/t8-cxparsingcompletor" // thin: only Tokenize; no depth gain
	cxparsing "github.com/skycoin/skycoin-cx/t9-cxparsing"     // thin: only AddInitFunction, ParseSourceCode; no depth gain
)

func parseProgram(options cxCmdFlags, fileNames []string, sourceCode []*os.File) bool {

	stopCPUProfile, err := profiling.StartCPUProfile("parse", profiling.DebugProfileRate)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		stopCPUProfile = func() error { return nil }
	}
	defer func() {
		if err := stopCPUProfile(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	if profiling.DebugProfile {
		defer func() {
			if err := profiling.DumpMemProfile("parse"); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}()
	}

	profiling.StartProfile("parse")
	defer profiling.StopProfile("parse")
	defer func() { printer.PrinterCompileErrorsPrint(actions.AST) }()

	if actions.AST == nil {
		// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
		// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0056 S0057.
		actions.AST = cxinit.MakeProgram()
		cxinit.RegisterPackages(actions.AST)
	}

	// Parsing all the source code files sent as CLI arguments to CX.
	cxparsing.ParseSourceCode(sourceCode, fileNames)

	// Checking if a main package exists. If not, create and add it to `AST`.
	if _, err := actions.AST.GetFunction(actions.AST.CxIrState.CxIrStateNameMain, actions.AST.CxIrState.CxIrStateNameMain); err != nil {
		initMainPkg(actions.AST)
	}

	// Setting what function to start in if using the REPL.
	repl.ReplTargetFn = constants.MAIN_FUNC

	// Adding *init function that initializes all the global variables.
	err = cxparsing.AddInitFunction(actions.AST)
	if err != nil {
		panic(fmt.Sprintf("error initializing function: %v", err))
	}

	actions.LineNo = 0

	if globals.FoundCompileErrors {
		printer.PrinterCompileErrorsPrint(actions.AST)
		profiling.CleanupAndExit(types.CX_COMPILATION_ERROR)
	}

	return true
}

// initMainPkg adds a `main` package with an empty `main` function to `prgrm`.
func initMainPkg(prgrm *ast.CXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S0058.
	mod := ast.MakePackage(prgrm.CxIrState.CxIrStateNameMain)
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S0059.
	modIdx := prgrm.AddPackage(mod)
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0060.
	modPkg, err := prgrm.GetPackageFromArray(modIdx)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now a constructor call, site S0061.
	fn := ast.MakeFunction(prgrm, prgrm.CxIrState.CxIrStateNameMain, actions.CurrentFile, actions.LineNo)
	modPkg.AddFunction(prgrm, fn)
}

// optionTokenize checks if the user wants to use CX to generate the lexer tokens
func printTokenize(options cxCmdFlags, fileNames []string) {
	var r *os.File
	var w *os.File
	var err error
	var lines []string
	var sourceFilename string

	if len(fileNames) == 0 {
		r = os.Stdin
	} else {
		sourceFilename = fileNames[0]
		if len(fileNames) > 1 {
			fmt.Fprintln(os.Stderr, "Multiple source files detected. Ignoring all except", sourceFilename)
		}
		r, err = util.CXOpenFile(sourceFilename)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ProgramError reading:", sourceFilename, err)
			return
		}
		defer r.Close()

		bytesRead, err := ioutil.ReadFile(sourceFilename)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ProgramError reading:", sourceFilename, err)
			return
		}

		file_content := string(bytesRead)
		lines = strings.Split(file_content, "\n")
	}

	if options.lexerOutput == "" {
		w = os.Stdout
	} else {
		tokenFilename := options.lexerOutput
		w, err = util.CXCreateFile(tokenFilename)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ProgramError writing:", tokenFilename, err)
			return
		}
		defer w.Close()
	}

	cxgo.Tokenize(r, w, lines, sourceFilename)
}

func printProgramAST(options cxCmdFlags, cxArgs []string, sourceCode []*os.File) {
	profiling.StartProfile("run")
	defer profiling.StopProfile("run")

	if options.replMode || len(sourceCode) == 0 {
		repl.Repl()
		return
	}

	// Print CX program.
	actions.AST.PrintProgram()

	if opcodes.AssertFailed() {
		os.Exit(types.CX_ASSERT)
	}
}

// Used for the -heap-initial, -heap-max and -stack-size flags.
// This function parses, for example, "1M" to 1048576 (the corresponding number of bytes)
// Possible suffixes are: G or g (gigabytes), M or m (megabytes), K or k (kilobytes)
func parseMemoryString(s string) types.Pointer {
	suffix := s[len(s)-1]
	_, notSuffix := strconv.ParseFloat(string(suffix), 64)

	if notSuffix == nil {
		// then we don't have a suffix
		num, err := strconv.ParseInt(s, 10, 64)

		if err != nil {
			// malformed size
			return types.InvalidPointer
		}

		return types.Cast_i64_to_ptr(num)
	} else {
		// then we have a suffix
		num, err := strconv.ParseFloat(s[:len(s)-1], 64)

		if err != nil {
			// malformed size
			return types.InvalidPointer
		}

		// The user can use suffixes to give as input gigabytes, megabytes or kilobytes.
		switch suffix {
		case 'G', 'g':
			return types.Cast_f64_to_ptr(num * 1073741824)
		case 'M', 'm':
			return types.Cast_f64_to_ptr(num * 1048576)
		case 'K', 'k':
			return types.Cast_f64_to_ptr(num * 1024)
		default:
			return types.InvalidPointer
		}
	}
}

type SourceCode struct {
	Code string //Unused?
}

// GetCXPath sets the bin, pkg and src paths from -cxpath, else CXPATH, else
// $HOME/cx. It does not create the directories.
func GetCXPath(options cxCmdFlags) {
	var err error
	CXPATH := ""
	if os.Getenv("CXPATH") != "" {
		CXPATH = os.Getenv("CXPATH")
	}
	// `options.cxpath` overrides `os.Getenv("CXPATH")`
	if options.cxpath != "" {
		CXPATH, err = filepath.Abs(options.cxpath)
		if err != nil {
			panic(err)
		}
	}
	if os.Getenv("CXPATH") == "" && options.cxpath == "" {
		usr, err := user.Current()
		if err != nil {
			panic(err)
		}

		CXPATH = usr.HomeDir + "/cx/"
	}
	globals.BINPATH = filepath.Join(CXPATH, "bin/")
	globals.PKGPATH = filepath.Join(CXPATH, "pkg/")
	globals.SRCPATH = filepath.Join(CXPATH, "src/")
}
