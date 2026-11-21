package ast

import (
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only FoundCompileErrors; core globals, keep
)

// CompileErrorCode names the message format of a compile error. The codes
// take the compiler block of docs/ErrorHandling.md, and the printer package
// holds the format of each one in a table indexed by the code.
type CompileErrorCode int32

const (
	CompileErrorCodeNil CompileErrorCode = 7000 + iota
	// CompileErrorCodeMessage prints its operand, the text of a Go error
	// or a lexer message, which has no code of its own.
	CompileErrorCodeMessage
	CompileErrorCodeAssignOutputlessOperator
	CompileErrorCodeAssignSymbolType
	CompileErrorCodeAssignValueType
	CompileErrorCodeBuiltinPackageNotImported
	CompileErrorCodeFunctionInputType
	CompileErrorCodeFunctionMissing
	CompileErrorCodeFunctionReceiverType
	CompileErrorCodeFunctionRedeclaration
	CompileErrorCodeFunctionReturnCount
	CompileErrorCodeIdentifierMissing
	CompileErrorCodeIndexType
	CompileErrorCodeInputTypesNotEqual
	CompileErrorCodeInvalidExpression
	CompileErrorCodeInvalidIndexing
	CompileErrorCodeInvalidIndirection
	CompileErrorCodeLoopConditionNoValue
	CompileErrorCodeNoFields
	CompileErrorCodeNoPackageDefined
	CompileErrorCodeOpcodeRenamed
	CompileErrorCodeOpcodeRenamedImport
	CompileErrorCodeOperatorArgumentTypes
	CompileErrorCodeOperatorInputCount
	CompileErrorCodeOperatorOutputCount
	CompileErrorCodePackageIdentifierMissing
	CompileErrorCodePointerOrderingOperator
	CompileErrorCodePrimitiveFieldAccess
	CompileErrorCodeRedeclared
	CompileErrorCodeStrArithmeticOperator
	CompileErrorCodeStrConcatenation
	CompileErrorCodeStrEqualityOperator
	CompileErrorCodeStrOrderingOperator
	CompileErrorCodeStructFieldDuplicate
	CompileErrorCodeStructFieldMultiplyDefined
	CompileErrorCodeStructLiteralFieldType
	CompileErrorCodeMethodMissing
	CompileErrorCodeDeclarationTooLarge
	CompileErrorCodeStructRecursive
	// CompileErrorCodeEnd is one past the last code.
	CompileErrorCodeEnd
)

// CompileErrorOperands are the operands of a compile error, each a
// stringpool.StringGid or an integer as the format of its code says.
type CompileErrorOperands [4]int64

// CompileErrorRecord is a compile error as data, see #413. The compiler
// records it without building text and the printer package prints it.
type CompileErrorRecord struct {
	CompileErrorRecordCode     CompileErrorCode
	CompileErrorRecordFile     SourceFileGid
	CompileErrorRecordLine     int
	CompileErrorRecordOperands CompileErrorOperands
}

// CompileErrorList holds the compile errors of a program in the order they
// were found. CompileErrorListPrinted counts the records already printed.
type CompileErrorList struct {
	CompileErrorListRecords []CompileErrorRecord
	CompileErrorListPrinted int
}

// CompileErrorListInit sets up an empty list with room for capacity records.
func (list *CompileErrorList) CompileErrorListInit(capacity int) {
	list.CompileErrorListRecords = make([]CompileErrorRecord, 0, capacity)
	list.CompileErrorListPrinted = 0
}

// CXProgramCompileErrorAdd records a compile error at file and line, and
// records that the program has a compile error.
func (prgrm *CXProgram) CXProgramCompileErrorAdd(code CompileErrorCode, file SourceFileGid, line int, operands CompileErrorOperands) {
	globals.FoundCompileErrors = true
	list := &prgrm.CxIrState.CompileErrorList
	list.CompileErrorListRecords = append(list.CompileErrorListRecords, CompileErrorRecord{
		CompileErrorRecordCode:     code,
		CompileErrorRecordFile:     file,
		CompileErrorRecordLine:     line,
		CompileErrorRecordOperands: operands,
	})
}

// CXProgramCompileErrorText interns text for a StringGid operand. Only the
// error path calls it, for names that are not in the pool already.
func (prgrm *CXProgram) CXProgramCompileErrorText(text string) int64 {
	return int64(prgrm.CxIrState.StringPool.StringPoolIntern([]byte(text)))
}

// CXProgramCompileErrorExit prints the compile errors not printed yet and
// ends the process, for a compile error the compiler cannot go on after.
func (prgrm *CXProgram) CXProgramCompileErrorExit() {
	if prgrm.CompileErrorPrint != nil {
		prgrm.CompileErrorPrint(prgrm)
	}
	os.Exit(types.CX_COMPILATION_ERROR)
}

// CXArgumentDebugFileGidLine returns the source file and line of the debug
// record gid, SourceFileGidNil and -1 for CXArgumentDebugGidNil.
func CXArgumentDebugFileGidLine(prgrm *CXProgram, gid CXArgumentDebugGid) (SourceFileGid, int) {
	if gid == CXArgumentDebugGidNil {
		return SourceFileGidNil, -1
	}
	details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(gid)
	return details.FileName, details.FileLine
}

// CXProgramCompileErrorAddDebug records a compile error at the source file
// and line of the debug record details.
func (prgrm *CXProgram) CXProgramCompileErrorAddDebug(code CompileErrorCode, details CXArgumentDebugGid, operands CompileErrorOperands) {
	file, line := CXArgumentDebugFileGidLine(prgrm, details)
	prgrm.CXProgramCompileErrorAdd(code, file, line, operands)
}
