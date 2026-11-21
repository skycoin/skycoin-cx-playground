package main

import (
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
	lower "github.com/skycoin/skycoin-cx/t4-cx-compiler-lower"
	"github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"
)

// The --executor values. auto runs a program on vm when every function
// lowers and on the old loop otherwise, step 5 of #408.
const (
	executorAuto = "auto"
	executorOld  = "old"
	executorVm   = "vm"
)

// executorOutputSize is the output buffer of a vm run.
const executorOutputSize = 64 << 10

// executorLower lowers prgrm. It returns "" when the program can run on vm:
// every function lowered, there is a main function, and the frames of *init
// and main fit the stack, which the old loop checks only for main. Otherwise
// it returns why not.
func executorLower(prgrm *ast.CXProgram) (image ir.Image, failures []lower.Failure, outside string) {
	image, failures = lower.Program(prgrm)
	switch {
	case len(failures) > 0:
		return image, failures, executorFailureDescribe(prgrm, failures[0])
	case image.ImageEntryMain == ir.ImageFuncNone:
		return image, failures, "no main function"
	}
	entries := uint64(image.ImageFuncs[image.ImageEntryMain].FuncFrameSize)
	if image.ImageEntryInit != ir.ImageFuncNone {
		entries += uint64(image.ImageFuncs[image.ImageEntryInit].FuncFrameSize)
	}
	if entries > uint64(image.ImageStackSize) {
		return image, failures, "the frames of *init and main do not fit the stack"
	}
	return image, failures, ""
}

// executorFailureDescribe names the reason and function of a failure.
func executorFailureDescribe(prgrm *ast.CXProgram, failure lower.Failure) string {
	description := lower.ReasonNames[failure.FailureReason]
	if failure.FailureReason == lower.ReasonOpcodeOutsideSubset {
		if row, found := constants.OpcodeLifecycleLiveByNumber(int(failure.FailureOpcode)); found {
			description += " " + row.Name
		}
	}
	return description + " in " + prgrm.CXProgramNameText(prgrm.CXFunctions[failure.FailureFunction].Name)
}

// executorVmRun runs image on vm as execute.RunCompiled and runProgram run
// prgrm, writing to os.Stdout. It returns the exit code: CX_SUCCESS,
// CX_ASSERT after a failed test, or the code of a runtime error, which it
// prints like printer.PrinterRuntimeErrorPrint.
func executorVmRun(prgrm *ast.CXProgram, image *ir.Image) int32 {
	return executorVmRunMachine(prgrm, vm.MachineNew(image, executorOutputSize))
}

// executorVmRunMachine runs the image of machine from a reset machine, see
// executorVmRun.
func executorVmRunMachine(prgrm *ast.CXProgram, machine *vm.Machine) int32 {
	image := machine.MachineImage
	machine.Reset()
	framePointer := uint32(0)
	if image.ImageEntryInit != ir.ImageFuncNone {
		if code := executorVmRunEntry(prgrm, machine, image.ImageEntryInit, 0); code != types.CX_SUCCESS {
			return code
		}
		framePointer = image.ImageFuncs[image.ImageEntryInit].FuncFrameSize
	}
	if code := executorVmRunEntry(prgrm, machine, image.ImageEntryMain, framePointer); code != types.CX_SUCCESS {
		return code
	}
	if machine.MachineTestFailures > 0 {
		return types.CX_ASSERT
	}
	return types.CX_SUCCESS
}

// executorVmRunEntry runs one entry function, serving the resumable stops.
func executorVmRunEntry(prgrm *ast.CXProgram, machine *vm.Machine, entry uint32, framePointer uint32) int32 {
	stop, _ := machine.Run(entry, framePointer)
	for {
		executorOutputFlush(machine)
		switch {
		case stop == vm.StopReturn:
			return types.CX_SUCCESS
		case stop == vm.StopTestFailed:
			printer.PrinterVmTestFailurePrint(machine)
		case !vm.Resumable(stop):
			printer.PrinterVmErrorPrint(prgrm, machine, stop)
			return vm.StopExitCodes[stop]
		}
		stop, _ = machine.Resume()
	}
}

// executorOutputFlush writes and empties the output of machine.
func executorOutputFlush(machine *vm.Machine) {
	if len(machine.MachineOutput) == 0 {
		return
	}
	// A failed write to stdout is ignored, as fmt.Println in the old loop
	// ignores it.
	os.Stdout.Write(machine.MachineOutput)
	machine.MachineOutput = machine.MachineOutput[:0]
}
