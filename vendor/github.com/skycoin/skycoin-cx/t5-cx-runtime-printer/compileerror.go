package printer

import (
	"os"
	"strconv"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// printerCompileErrorFormat is the text of one compile error code. In
// Format, %H is the "error: file:line" header, %s the next operand as a
// StringGid and %d the next as an integer. %+ is "s" when the last integer
// is above 1, and %~ is " was" when it is 1 and "s were" otherwise.
// Stdout sends the text to stdout instead of stderr.
type printerCompileErrorFormat struct {
	Format string
	Stdout bool
}

var printerCompileErrorFormats = [ast.CompileErrorCodeEnd - ast.CompileErrorCodeNil]printerCompileErrorFormat{
	ast.CompileErrorCodeMessage - ast.CompileErrorCodeNil:                    {Format: "%H %s\n"},
	ast.CompileErrorCodeAssignOutputlessOperator - ast.CompileErrorCodeNil:   {Format: "%H trying to use an outputless operator in an assignment\n"},
	ast.CompileErrorCodeAssignSymbolType - ast.CompileErrorCodeNil:           {Format: "%H trying to assign argument of type '%s' to symbol '%s' of type '%s'\n"},
	ast.CompileErrorCodeAssignValueType - ast.CompileErrorCodeNil:            {Format: "%H cannot assign value of type '%s' to identifier '%s' of type '%s'\n"},
	ast.CompileErrorCodeBuiltinPackageNotImported - ast.CompileErrorCodeNil:  {Format: "%H built-in package %s is used without importing it, add import \"" + cxpackages.SysPrefix + "%s\"\n"},
	ast.CompileErrorCodeFunctionInputType - ast.CompileErrorCodeNil:          {Format: "%H function '%s' expected input argument of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeFunctionMissing - ast.CompileErrorCodeNil:            {Format: "%H function '%s' does not exist\n"},
	ast.CompileErrorCodeFunctionReceiverType - ast.CompileErrorCodeNil:       {Format: "%H function '%s' expected receiving variable of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeFunctionRedeclaration - ast.CompileErrorCodeNil:      {Format: "%H function redeclaration\n"},
	ast.CompileErrorCodeFunctionReturnCount - ast.CompileErrorCodeNil:        {Format: "%H function '%s' expects to return %d argument%+, but %d output argument%~ provided\n"},
	ast.CompileErrorCodeIdentifierMissing - ast.CompileErrorCodeNil:          {Format: "%H identifier '%s' does not exist\n"},
	ast.CompileErrorCodeIndexType - ast.CompileErrorCodeNil:                  {Format: "%H wrong index type; expected either 'i32' or 'i64', got '%s'\n"},
	ast.CompileErrorCodeInputTypesNotEqual - ast.CompileErrorCodeNil:         {Format: "%H first and second input arguments' types are not equal in '%s' call ('%s' != '%s')\n"},
	ast.CompileErrorCodeInvalidExpression - ast.CompileErrorCodeNil:          {Format: "%H invalid expression\n"},
	ast.CompileErrorCodeInvalidIndexing - ast.CompileErrorCodeNil:            {Format: "%H invalid indexing\n"},
	ast.CompileErrorCodeInvalidIndirection - ast.CompileErrorCodeNil:         {Format: "%H invalid indirection\n"},
	ast.CompileErrorCodeLoopConditionNoValue - ast.CompileErrorCodeNil:       {Format: "%H loop condition has no value\n"},
	ast.CompileErrorCodeNoFields - ast.CompileErrorCodeNil:                   {Format: "%H '%s' has no fields\n"},
	ast.CompileErrorCodeNoPackageDefined - ast.CompileErrorCodeNil:           {Format: "%H No package defined\n"},
	ast.CompileErrorCodeOpcodeRenamed - ast.CompileErrorCodeNil:              {Format: "%H %s was renamed to %s in language version %d\n"},
	ast.CompileErrorCodeOpcodeRenamedImport - ast.CompileErrorCodeNil:        {Format: "%H %s was renamed to %s in language version %d, import \"" + cxpackages.SysPrefix + "%s\"\n"},
	ast.CompileErrorCodeOperatorArgumentTypes - ast.CompileErrorCodeNil:      {Format: "%H invalid argument types for '%s' operator\n"},
	ast.CompileErrorCodeOperatorInputCount - ast.CompileErrorCodeNil:         {Format: "%H operator '%s' expects %d input/s, but %d input argument\n were provided\n"},
	ast.CompileErrorCodeOperatorOutputCount - ast.CompileErrorCodeNil:        {Format: "%H operator '%s' expects to return %d output/s, but %d receiving argument/s were provided\n"},
	ast.CompileErrorCodePackageIdentifierMissing - ast.CompileErrorCodeNil:   {Format: "%H identifier '%s.%s' does not exist\n"},
	ast.CompileErrorCodePointerOrderingOperator - ast.CompileErrorCodeNil:    {Format: "%H pointer %s pointer is not allowed, pointers support only == and !=\n"},
	ast.CompileErrorCodePrimitiveFieldAccess - ast.CompileErrorCodeNil:       {Format: "%H illegal method call or field access on identifier '%s' of primitive type '%s'\n"},
	ast.CompileErrorCodeRedeclared - ast.CompileErrorCodeNil:                 {Format: "%H '%s' redeclared\n"},
	ast.CompileErrorCodeStrArithmeticOperator - ast.CompileErrorCodeNil:      {Format: "%H operator %s is not allowed on str, strings have no arithmetic operators\n"},
	ast.CompileErrorCodeStrConcatenation - ast.CompileErrorCodeNil:           {Format: "%H str + str is not allowed, use string.Concatenation\n"},
	ast.CompileErrorCodeStrEqualityOperator - ast.CompileErrorCodeNil:        {Format: "%H str %s str is not allowed, use string.EqualityByByteValue or string.EqualityByHeapPointer\n"},
	ast.CompileErrorCodeStrOrderingOperator - ast.CompileErrorCodeNil:        {Format: "%H str %s str is not allowed, use string.OrderingByByteValue\n"},
	ast.CompileErrorCodeStructFieldDuplicate - ast.CompileErrorCodeNil:       {Format: "%H : duplicate field", Stdout: true},
	ast.CompileErrorCodeStructFieldMultiplyDefined - ast.CompileErrorCodeNil: {Format: "%H Multiply defined struct field: %s\n"},
	ast.CompileErrorCodeStructLiteralFieldType - ast.CompileErrorCodeNil:     {Format: "%H field '%s' in struct literal of type '%s' expected argument of type '%s'; '%s' was provided\n"},
	ast.CompileErrorCodeMethodMissing - ast.CompileErrorCodeNil:              {Format: "%H type '%s' has no method '%s'\n"},
	ast.CompileErrorCodeDeclarationTooLarge - ast.CompileErrorCodeNil:        {Format: "%H '%s' is too large\n"},
	ast.CompileErrorCodeStructRecursive - ast.CompileErrorCodeNil:            {Format: "%H invalid recursive type '%s'\n"},
}

// PrinterCompileErrorsRegister makes prgrm print its compile errors through
// this package when the compiler exits on one.
func PrinterCompileErrorsRegister(prgrm *ast.CXProgram) {
	prgrm.CompileErrorPrint = PrinterCompileErrorsPrint
}

// PrinterCompileErrorsPrint prints the compile errors of prgrm that are not
// printed yet, in the order they were found.
func PrinterCompileErrorsPrint(prgrm *ast.CXProgram) {
	list := &prgrm.CxIrState.CompileErrorList
	for ; list.CompileErrorListPrinted < len(list.CompileErrorListRecords); list.CompileErrorListPrinted++ {
		record := &list.CompileErrorListRecords[list.CompileErrorListPrinted]
		format := printerCompileErrorFormats[record.CompileErrorRecordCode-ast.CompileErrorCodeNil]
		text := PrinterCompileErrorAppend(nil, prgrm, record)
		if format.Stdout {
			os.Stdout.Write(text)
		} else {
			os.Stderr.Write(text)
		}
	}
}

// PrinterCompileErrorAppend appends the text of record to text.
func PrinterCompileErrorAppend(text []byte, prgrm *ast.CXProgram, record *ast.CompileErrorRecord) []byte {
	format := printerCompileErrorFormats[record.CompileErrorRecordCode-ast.CompileErrorCodeNil].Format
	operand := 0
	var lastInteger int64
	for index := 0; index < len(format); index++ {
		if format[index] != '%' || index+1 == len(format) {
			text = append(text, format[index])
			continue
		}
		index++
		switch format[index] {
		case 'H':
			text = append(text, "error: "...)
			text = append(text, prgrm.CXProgramSourceFileText(record.CompileErrorRecordFile)...)
			text = append(text, ':')
			text = strconv.AppendInt(text, int64(record.CompileErrorRecordLine), 10)
		case 's':
			text = append(text, prgrm.CXProgramNameBytes(stringpool.StringGid(record.CompileErrorRecordOperands[operand]))...)
			operand++
		case 'd':
			lastInteger = record.CompileErrorRecordOperands[operand]
			text = strconv.AppendInt(text, lastInteger, 10)
			operand++
		case '+':
			if lastInteger > 1 {
				text = append(text, 's')
			}
		case '~':
			if lastInteger == 1 {
				text = append(text, " was"...)
			} else {
				text = append(text, "s were"...)
			}
		default:
			text = append(text, '%', format[index])
		}
	}
	return text
}
