package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t0-util"    // thin: only CXCreateFile, CXLogFile, CXOpenFile, ProgramSandboxRootOpen; no depth gain
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only AssertFailed, RegisterOpcodes; no depth gain

	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t1-profiling"
	repl "github.com/skycoin/skycoin-cx/t10-cxrepl" // thin: only Repl, ReplTargetFn; only user of cxrepl, could merge it here, max tier 11 to 10
	"github.com/skycoin/skycoin-cx/t2-globals"      // thin: only DBG_GOLANG_STACK_TRACE; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-actions"
	"github.com/skycoin/skycoin-cx/t4-execute" // thin: only RunCompiled; no depth gain
	"github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"
	"github.com/skycoin/skycoin-cx/t5-cxos"        // thin: only InitConstants; no depth gain
	"github.com/skycoin/skycoin-cx/t6-cxfx"        // thin: only InitConstants; no depth gain
	cxinit "github.com/skycoin/skycoin-cx/t7-init" // thin: only MakeProgram, RegisterPackages; no depth gain
)

func main() {
	Run(os.Args[1:])
}

func Run(args []string) {
	defer operationCountPrint()
	options, cxArgs, sourceCode, fileNames, start := runStart(args)
	if !start {
		return
	}

	if run := parseProgram(options, fileNames, sourceCode); run {

		if options.printAST || checkAST(args) {
			printProgramAST(options, cxArgs, sourceCode)
			return
		}

		if options.tokenizeMode {
			printTokenize(options, fileNames)
			return
		}

		//if strings.Contains(args[0], "tokenize") {
		if checktokenizeMode(args) {
			printTokenize(options, fileNames)
			return
		}

		runProgram(options, cxArgs, sourceCode)
	}
}

func runProgram(options cxCmdFlags, cxArgs []string, sourceCode []*os.File) {
	root, fileError := util.ProgramSandboxRootOpen(options.sandbox)
	if fileError != nil {
		fmt.Fprintf(os.Stderr, "cannot open the sandbox: %s\n", fileError.FileErrorMessage)
		os.Exit(types.CX_COMPILATION_ERROR)
	}
	defer root.Close()
	actions.AST.ProgramSandboxRoot = root
	actions.AST.ProgramStdinInit(os.Stdin)

	profiling.StartProfile("run")

	defer profiling.StopProfile("run")

	if options.executor != executorAuto && options.executor != executorOld && options.executor != executorVm {
		fmt.Fprintf(os.Stderr, "unknown --executor %q, want auto, old or vm\n", options.executor)
		os.Exit(types.CX_COMPILATION_ERROR)
	}

	if options.replMode || len(sourceCode) == 0 {
		repl.Repl()
		return
	}

	actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
	actions.AST.CxIrState.StringPool.StringPoolFreeze()

	if options.executor != executorOld {
		image, _, outside := executorLower(actions.AST)
		switch {
		case outside == "":
			if code := executorVmRun(actions.AST, &image); code != types.CX_SUCCESS {
				os.Exit(int(code))
			}
			return
		case options.executor == executorVm:
			fmt.Fprintf(os.Stderr, "cannot run on vm: %s\n", outside)
			os.Exit(types.CX_RUNTIME_NOT_IMPLEMENTED)
		}
	}

	err := execute.RunCompiled(actions.AST, 0, cxArgs)

	if err != nil {
		panic(err)
	}
	if actions.AST.RuntimeError.RuntimeErrorRecordRaised {
		os.Exit(printer.PrinterRuntimeErrorPrint(actions.AST))
	}

	if opcodes.AssertFailed() {
		os.Exit(types.CX_ASSERT)
	}
}

// runStart reads the flags and sets up the program state for args, the part
// of Run before parsing. It returns false when there is nothing to run.
func runStart(args []string) (options cxCmdFlags, cxArgs []string, sourceCode []*os.File, fileNames []string, start bool) {
	types.Allocator = ast.AllocateSeq
	runtime.LockOSThread()

	runtime.GOMAXPROCS(2)

	options = defaultCmdFlags()

	parseFlags(&options, args)

	/*
		Checking if CXPATH is set, either by setting an environment variable
		 or by setting the `--cxpath` flag.
	*/
	GetCXPath(options)

	/*
		checkHelp checks  for command line argument string "help"
		if exits print help
		$cx help
	*/
	if checkHelp(args) {
		commandLine.PrintDefaults()
		return options, nil, nil, nil, false
	}

	/*
		options.printHelp checks for flags string "help"
		if exits print help
		$cx --help
		$cx -help
	*/
	if options.printHelp {
		printHelp()
		return options, nil, nil, nil, false
	}

	/*
		options.printVersion checks for flags string "version"
		if exits print cx version
		$cx --version
		$cx -v
	*/
	if options.printVersion {
		printVersion()
		return options, nil, nil, nil, false
	}

	/*
		checkversion checks  for command line argument string "version"
		if exits print version
		$cx version
	*/
	if checkversion(args) {
		printVersion()
		return options, nil, nil, nil, false
	}

	/*
		options.printEnv checks for flags string "env"
		if exits print cx env
		$cx --env
	*/
	if options.printEnv {
		printEnv()
		return options, nil, nil, nil, false
	}

	/*
		checkenv checks  for command line argument string "env"
		if exits print env
		$cx env
	*/
	if checkenv(args) {
		printEnv()
		return options, nil, nil, nil, false
	}

	/*
		options.debugFileAccess prints open/close file events"
		$cx --debug-file-access
		$cx -Df
	*/
	globals.DBG_GOLANG_STACK_TRACE = options.debugGoStack
	if options.debugFileAccess {
		util.CXLogFile(true)
	}

	/*
		options.initialHeap checks for flags string "heap-initial"
		$cx --heap-initial
	*/
	if options.initialHeap != "" {
		constants.INIT_HEAP_SIZE = parseMemoryString(options.initialHeap)
	}

	/*
		options.maxHeap checks for flags string "maxHeap"
		$cx --maxHeap
	*/
	if options.maxHeap != "" {
		constants.MAX_HEAP_SIZE = parseMemoryString(options.maxHeap)
		if constants.MAX_HEAP_SIZE < constants.INIT_HEAP_SIZE {
			// Then MAX_HEAP_SIZE overrides INIT_HEAP_SIZE's value.
			constants.INIT_HEAP_SIZE = constants.MAX_HEAP_SIZE
		}
	}

	if options.stackSize != "" {
		constants.STACK_SIZE = parseMemoryString(options.stackSize)
	}
	// Memory stays below constants.ESCAPE_OFFSET_FLAG.
	constants.MAX_HEAP_SIZE = min(constants.MAX_HEAP_SIZE, constants.ESCAPE_OFFSET_FLAG/2)
	constants.INIT_HEAP_SIZE = min(constants.INIT_HEAP_SIZE, constants.MAX_HEAP_SIZE)
	constants.STACK_SIZE = min(constants.STACK_SIZE, constants.ESCAPE_OFFSET_FLAG/4)

	/*
		options.minHeapFreeRatio checks for flags string "--min-heap-free"
		$cx --min-heap-free
	*/
	if options.minHeapFreeRatio != float64(0) {
		constants.MIN_HEAP_FREE_RATIO = float32(options.minHeapFreeRatio)
	}

	/*
		options.maxHeapFreeRatio checks for flags string "--max-heap-free"
		$cx --max-heap-free
	*/
	if options.maxHeapFreeRatio != float64(0) {
		constants.MAX_HEAP_FREE_RATIO = float32(options.maxHeapFreeRatio)
	}

	// options, file pointers, filenames
	cxArgs, sourceCode, fileNames, err := util.ParseArgsForCX(commandLine.Args(), true)
	if err != nil {
		println(err.Error())
		os.Exit(types.CX_COMPILATION_ERROR)
	}

	// Propagate some options out to other packages.
	profiling.DebugProfileRate = options.debugProfile

	profiling.DebugProfile = profiling.DebugProfileRate > 0

	// Init 4-9 run here, after the flags, so the heap flags size the program.
	// Init 1-3 are package vars (see the VIOLATION comments).
	// Init 4: cx.* constants in actions.ConstCodes
	actions.InitConstants()
	// Init 5: os.* constants (empty with nocxos)
	cxos.InitConstants()
	// Init 6: gl, glfw and al constants (empty without cxfx)
	cxfx.InitConstants()
	// Init 7: actions.AST, the program state
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Also state.CXProgramMng.CXProgramRelease(gid), phase 4 #371. Site S0062 S0063.
	actions.AST = cxinit.MakeProgram()
	// Init 8: native packages in the program and the global opcode tables
	cxinit.RegisterPackages(actions.AST)
	// Init 9: built-in opcode and operator tables
	opcodes.RegisterOpcodes(actions.AST)

	return options, cxArgs, sourceCode, fileNames, true
}
