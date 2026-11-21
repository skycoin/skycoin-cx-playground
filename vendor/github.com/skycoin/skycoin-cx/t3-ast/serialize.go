package ast

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-lz4codec"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin/src/cipher/encoder"
)

func deserializeRaw(byts []byte, offset types.Pointer, size types.Pointer, item interface{}) {
	_, err := encoder.DeserializeRaw(byts[offset:offset+size], item)
	if err != nil {
		panic(err)
	}
}

func serializeString(name string, s *SerializedCXProgram) (int64, int64) {
	if name == "" {
		return int64(-1), int64(-1)
	}

	size := encoder.Size(name)

	off, found := s.StringsMap[name]
	if found {
		return int64(off), int64(size)
	}
	off = int64(len(s.Strings))
	s.Strings = append(s.Strings, encoder.Serialize(name)...)
	s.StringsMap[name] = off

	return int64(off), int64(size)
}

func indexPackage(prgrm *CXProgram, pkg *CXPackage, s *SerializedCXProgram) {
	if _, found := s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]; !found {
		s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)] = int64(len(s.PackagesMap))
	} else {
		panic("duplicated package in serialization process")
	}
}

func indexStruct(prgrm *CXProgram, strct *CXStruct, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0389.
	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		panic(err)
	}

	strctName := prgrm.CXProgramNameText(strctPkg.Name) + "." + prgrm.CXProgramNameText(strct.Name)
	if _, found := s.StructsMap[strctName]; !found {
		s.StructsMap[strctName] = int64(len(s.StructsMap))
	} else {
		panic("duplicated struct in serialization process")
	}
}

func indexFunction(prgrm *CXProgram, fn *CXFunction, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0390.
	fnPkg, err := prgrm.GetPackageFromArray(fn.Package)
	if err != nil {
		panic(err)
	}
	fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(fn.Name)
	if _, found := s.FunctionsMap[fnName]; !found {
		s.FunctionsMap[fnName] = int64(len(s.FunctionsMap))
	} else {
		panic("duplicated function in serialization process")
	}
}

func serializeBoolean(val bool) int64 {
	if val {
		return 1
	}
	return 0
}

func serializePointers(pointers []types.Pointer, s *SerializedCXProgram) (int64, int64) {
	if len(pointers) == 0 {
		return int64(-1), int64(-1)
	}
	off := len(s.Integers)
	l := len(pointers)

	ints := make([]int64, l)
	for i, pointer := range pointers {
		ints[i] = int64(pointer)
	}

	s.Integers = append(s.Integers, ints...)

	return int64(off), int64(l)
}

func serializeIntegers(ints []int, s *SerializedCXProgram) (int64, int64) {
	if len(ints) == 0 {
		return int64(-1), int64(-1)
	}
	off := len(s.Integers)
	l := len(ints)

	ints32 := make([]int64, l)
	for i, int := range ints {
		ints32[i] = int64(int)
	}

	s.Integers = append(s.Integers, ints32...)

	return int64(off), int64(l)
}

func serializeArgument(prgrm *CXProgram, arg *CXArgument, s *SerializedCXProgram) int {
	s.Arguments = append(s.Arguments, serializedArgument{})
	argOff := len(s.Arguments) - 1

	sNil := int64(-1)

	s.Arguments[argOff].NameOffset, s.Arguments[argOff].NameSize = serializeString(prgrm.CXProgramNameText(arg.Name), s)

	s.Arguments[argOff].Type = int64(arg.Type)

	if arg.StructType == nil {
		s.Arguments[argOff].StructTypeOffset = sNil
	} else {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0391.
		strctPkg, err := prgrm.GetPackageFromArray(arg.StructType.Package)
		if err != nil {
			panic(err)
		}

		strctName := prgrm.CXProgramNameText(strctPkg.Name) + "." + prgrm.CXProgramNameText(arg.StructType.Name)
		if strctOff, found := s.StructsMap[strctName]; found {
			s.Arguments[argOff].StructTypeOffset = int64(strctOff)
		} else {
			panic("struct reference not found")
		}
	}

	s.Arguments[argOff].Size = int64(arg.Size)
	s.Arguments[argOff].Offset = int64(arg.Offset)

	s.Arguments[argOff].DeclarationSpecifiersOffset,
		s.Arguments[argOff].DeclarationSpecifiersSize = serializeIntegers(arg.DeclarationSpecifiers, s)

	s.Arguments[argOff].PreviouslyDeclared = serializeBoolean(arg.PreviouslyDeclared)

	s.Arguments[argOff].PassBy = int64(arg.PassBy)

	s.Arguments[argOff].LengthsOffset, s.Arguments[argOff].LengthsSize = serializePointers(arg.Lengths, s)
	// TODO: include Indexes of type CTypeSignature
	// s.Arguments[argOff].IndexesOffset, s.Arguments[argOff].IndexesSize = serializeSliceOfArguments(prgrm, prgrm.ConvertIndexArgsToPointerArgs(arg.Indexes), s)
	s.Arguments[argOff].FieldsOffset, s.Arguments[argOff].FieldsSize = serializeSliceOfArguments(prgrm, prgrm.ConvertIndexArgsToPointerArgs(arg.Fields), s)
	s.Arguments[argOff].InputsOffset, s.Arguments[argOff].InputsSize = serializeSliceOfArguments(prgrm, prgrm.ConvertIndexArgsToPointerArgs(arg.Inputs), s)
	s.Arguments[argOff].OutputsOffset, s.Arguments[argOff].OutputsSize = serializeSliceOfArguments(prgrm, prgrm.ConvertIndexArgsToPointerArgs(arg.Outputs), s)

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0392.
	argPkg, err := prgrm.GetPackageFromArray(arg.Package)
	if err != nil {
		panic(err)
	}

	if _, found := s.PackagesMap[prgrm.CXProgramNameText(argPkg.Name)]; found {
		s.Arguments[argOff].PackageName = prgrm.CXProgramNameText(argPkg.Name)
	} else {
		panic("package reference not found")
	}

	return argOff
}

func serializeSliceOfArguments(prgrm *CXProgram, args []*CXArgument, s *SerializedCXProgram) (int64, int64) {
	if len(args) == 0 {
		return int64(-1), int64(-1)
	}
	idxs := make([]int, len(args))
	for i, arg := range args {
		idxs[i] = serializeArgument(prgrm, arg, s)
	}
	return serializeIntegers(idxs, s)
}

func serializeCalls(prgrm *CXProgram, calls []CXCall, s *SerializedCXProgram) (int64, int64) {
	if len(calls) == 0 {
		return int64(-1), int64(-1)
	}
	idxs := make([]int, len(calls))
	for i, call := range calls {
		idxs[i] = serializeCall(prgrm, &call, s)
	}
	return serializeIntegers(idxs, s)

}

func serializeExpression(prgrm *CXProgram, expr *CXExpression, s *SerializedCXProgram) int {
	s.Expressions = append(s.Expressions, serializedExpression{})
	exprOff := len(s.Expressions) - 1
	sExpr := &s.Expressions[exprOff]

	sNil := int64(-1)

	sExpr.Type = int64(expr.Type)
	switch expr.Type {
	case CX_LINE:
		_, cxLine, err := prgrm.GetOperation(expr)
		if err != nil {
			panic(err)
		}

		sExpr.ExpressionType = int64(expr.ExpressionType)
		sExpr.FileName = prgrm.CXProgramSourceFileText(cxLine.FileName)
		sExpr.LineNumber = int64(cxLine.LineNumber)
		sExpr.LineStr = prgrm.CXProgramNameText(cxLine.LineStr)

	case CX_ATOMIC_OPERATOR:
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0393.
		cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0394.
		cxAtomicOpOperator := prgrm.GetFunctionFromArray(cxAtomicOp.Operator)

		if cxAtomicOpOperator == nil {
			// then it's a declaration
			sExpr.OperatorOffset = sNil
			sExpr.IsNative = serializeBoolean(false)
			sExpr.OpCode = int64(-1)
		} else if cxAtomicOpOperator.IsBuiltIn() {
			sExpr.OperatorOffset = sNil
			sExpr.IsNative = serializeBoolean(true)
			sExpr.OpCode = int64(cxAtomicOpOperator.AtomicOPCode)
		} else {
			sExpr.IsNative = serializeBoolean(false)
			sExpr.OpCode = sNil

			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S0395.
			opPkg, err := prgrm.GetPackageFromArray(cxAtomicOpOperator.Package)
			if err != nil {
				panic(err)
			}
			opName := prgrm.CXProgramNameText(opPkg.Name) + "." + prgrm.CXProgramNameText(cxAtomicOpOperator.Name)
			if opOff, found := s.FunctionsMap[opName]; found {
				sExpr.OperatorOffset = int64(opOff)
			}
		}

		inputCXArgs := prgrm.ConvertIndexTypeSignaturesToPointerArgs(cxAtomicOp.GetInputs(prgrm))
		sExpr.InputsOffset, sExpr.InputsSize = serializeSliceOfArguments(prgrm, inputCXArgs, s)
		outputCXArgs := prgrm.ConvertIndexTypeSignaturesToPointerArgs(cxAtomicOp.GetOutputs(prgrm))
		sExpr.OutputsOffset, sExpr.OutputsSize = serializeSliceOfArguments(prgrm, outputCXArgs, s)

		sExpr.LabelOffset, sExpr.LabelSize = serializeString(prgrm.CXProgramNameText(cxAtomicOp.Label), s)
		sExpr.ThenLines = int64(cxAtomicOp.ThenLines)
		sExpr.ElseLines = int64(cxAtomicOp.ElseLines)

		sExpr.ExpressionType = int64(expr.ExpressionType)

		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0396.
		cxAtomicOpFunction := prgrm.GetFunctionFromArray(cxAtomicOp.Function)

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0397.
		fnPkg, err := prgrm.GetPackageFromArray(cxAtomicOpFunction.Package)
		if err != nil {
			panic(err)
		}
		fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(cxAtomicOpFunction.Name)
		if fnOff, found := s.FunctionsMap[fnName]; found {
			sExpr.FunctionOffset = int64(fnOff)
		} else {
			panic("function reference not found")
		}

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0398.
		cxAtomicOpPkg, err := prgrm.GetPackageFromArray(cxAtomicOp.Package)
		if err != nil {
			panic(err)
		}

		if _, found := s.PackagesMap[prgrm.CXProgramNameText(cxAtomicOpPkg.Name)]; found {
			sExpr.PackageName = prgrm.CXProgramNameText(cxAtomicOpPkg.Name)
		} else {
			panic("package reference not found")
		}

	}
	return exprOff
}

func serializeCall(prgrm *CXProgram, call *CXCall, s *SerializedCXProgram) int {
	s.Calls = append(s.Calls, serializedCall{})
	callOff := len(s.Calls) - 1
	serializedCall := &s.Calls[callOff]

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0399.
	opPkg, err := prgrm.GetPackageFromArray(call.Operator.Package)
	if err != nil {
		panic(err)
	}

	opName := prgrm.CXProgramNameText(opPkg.Name) + "." + prgrm.CXProgramNameText(call.Operator.Name)
	if opOff, found := s.FunctionsMap[opName]; found {
		serializedCall.OperatorOffset = int64(opOff)
		serializedCall.Line = int64(call.Line)
		serializedCall.FramePointer = int64(call.FramePointer)
	} else {
		panic("function reference not found")
	}

	return callOff
}

func serializeStructArguments(prgrm *CXProgram, strct *CXStruct, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0400.
	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		panic(err)
	}

	strctName := prgrm.CXProgramNameText(strctPkg.Name) + "." + prgrm.CXProgramNameText(strct.Name)
	if strctOff, found := s.StructsMap[strctName]; found {
		sStrct := &s.Structs[strctOff]
		sStrct.FieldsOffset, sStrct.FieldsSize = serializeSliceOfArguments(prgrm, prgrm.ConvertIndexTypeSignaturesToPointerArgs(strct.Fields), s)
	} else {
		panic("struct reference not found")
	}
}

func serializeFunctionArguments(prgrm *CXProgram, fn *CXFunction, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0401.
	fnPkg, err := prgrm.GetPackageFromArray(fn.Package)
	if err != nil {
		panic(err)
	}
	fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(fn.Name)
	if fnOff, found := s.FunctionsMap[fnName]; found {
		sFn := &s.Functions[fnOff]

		arrCXArgs := prgrm.ConvertIndexTypeSignaturesToPointerArgs(fn.GetInputs(prgrm))
		sFn.InputsOffset, sFn.InputsSize = serializeSliceOfArguments(prgrm, arrCXArgs, s)
		outputCXArgs := prgrm.ConvertIndexTypeSignaturesToPointerArgs(fn.GetOutputs(prgrm))
		sFn.OutputsOffset, sFn.OutputsSize = serializeSliceOfArguments(prgrm, outputCXArgs, s)
		sFn.ListOfPointersOffset, sFn.ListOfPointersSize = serializeSliceOfArguments(prgrm, fn.ListOfPointers, s)
	} else {
		panic("function reference not found")
	}
}

func serializePackageName(prgrm *CXProgram, pkg *CXPackage, s *SerializedCXProgram) {
	sPkg := &s.Packages[s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]]
	sPkg.NameOffset, sPkg.NameSize = serializeString(prgrm.CXProgramNameText(pkg.Name), s) //Change Name to String
}

func serializeStructName(prgrm *CXProgram, strct *CXStruct, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0402.
	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		panic(err)
	}

	strctName := prgrm.CXProgramNameText(strctPkg.Name) + "." + prgrm.CXProgramNameText(strct.Name)
	sStrct := &s.Structs[s.StructsMap[strctName]]
	sStrct.NameOffset, sStrct.NameSize = serializeString(prgrm.CXProgramNameText(strct.Name), s) //Change Name to String
}

func serializeFunctionName(prgrm *CXProgram, fn *CXFunction, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0403.
	fnPkg, err := prgrm.GetPackageFromArray(fn.Package)
	if err != nil {
		panic(err)
	}
	fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(fn.Name)
	if off, found := s.FunctionsMap[fnName]; found {
		sFn := &s.Functions[off]
		sFn.NameOffset, sFn.NameSize = serializeString(prgrm.CXProgramNameText(fn.Name), s) //Change Name to String
	} else {
		panic("function reference not found")
	}
}

func serializePackageGlobals(prgrm *CXProgram, pkg *CXPackage, s *SerializedCXProgram) {
	if pkgOff, found := s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]; found {
		sPkg := &s.Packages[pkgOff]

		var glblArgs []*CXArgument
		for _, glblFldIdx := range pkg.Globals.Fields {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0404.
			glblFld := prgrm.GetCXTypeSignatureFromArray(glblFldIdx)
			// Assuming only all are TYPE_CXARGUMENT_DEPRECATE
			// TODO: To be replaced
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0405.
			glbl := prgrm.GetCXArg(CXArgumentIndex(glblFld.Meta))
			glblArgs = append(glblArgs, glbl)
		}
		sPkg.GlobalsOffset, sPkg.GlobalsSize = serializeSliceOfArguments(prgrm, glblArgs, s)
	} else {
		panic("package reference not found")
	}
}

func serializePackageImports(prgrm *CXProgram, pkg *CXPackage, s *SerializedCXProgram) {
	l := len(pkg.Imports)
	if l == 0 {
		s.Packages[s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]].ImportsOffset = int64(-1)
		s.Packages[s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]].ImportsSize = int64(-1)
		return
	}
	imps := make([]int64, l)
	count := 0
	for _, impIdx := range importsByName(prgrm, pkg) {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0406.
		impPkg, err := prgrm.GetPackageFromArray(impIdx)
		if err != nil {
			panic(err)
		}
		if idx, found := s.PackagesMap[prgrm.CXProgramNameText(impPkg.Name)]; found {
			imps[count] = int64(idx)
		} else {
			panic("import package reference not found")
		}

		count++
	}

	s.Packages[s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]].ImportsOffset = int64(len(s.Integers))
	s.Packages[s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]].ImportsSize = int64(l)
	s.Integers = append(s.Integers, imps...)
}

func serializeStructPackage(prgrm *CXProgram, strct *CXStruct, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0407.
	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		panic(err)
	}

	strctName := prgrm.CXProgramNameText(strctPkg.Name) + "." + prgrm.CXProgramNameText(strct.Name)
	if _, found := s.PackagesMap[prgrm.CXProgramNameText(strctPkg.Name)]; found {
		if off, found := s.StructsMap[strctName]; found {
			sStrct := &s.Structs[off]
			sStrct.PackageName = prgrm.CXProgramNameText(strctPkg.Name)
		} else {
			panic("struct reference not found")
		}
	} else {
		panic("struct's package reference not found")
	}
}

func serializeFunctionPackage(prgrm *CXProgram, fn *CXFunction, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0408.
	fnPkg, err := prgrm.GetPackageFromArray(fn.Package)
	if err != nil {
		panic(err)
	}
	fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(fn.Name)
	if _, found := s.PackagesMap[prgrm.CXProgramNameText(fnPkg.Name)]; found {
		if off, found := s.FunctionsMap[fnName]; found {
			sFn := &s.Functions[off]
			sFn.PackageName = prgrm.CXProgramNameText(fnPkg.Name)
		} else {
			panic("function reference not found")
		}
	} else {
		panic("function's package reference not found")
	}
}

func serializePackageIntegers(prgrm *CXProgram, pkg *CXPackage, s *SerializedCXProgram) {
	if pkgOff, found := s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]; found {
		sPkg := &s.Packages[pkgOff]

		if pkg.CurrentFunction == -1 {
			// package has no functions
			sPkg.CurrentFunctionName = ""
		} else {
			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0409.
			currFn := prgrm.GetFunctionFromArray(pkg.CurrentFunction)

			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S0410.
			currFnPkg, err := prgrm.GetPackageFromArray(currFn.Package)
			if err != nil {
				panic(err)
			}
			currFnName := prgrm.CXProgramNameText(currFnPkg.Name) + "." + prgrm.CXProgramNameText(currFn.Name)

			if _, found := s.FunctionsMap[currFnName]; found {
				sPkg.CurrentFunctionName = currFnName
			} else {
				panic("function reference not found")
			}
		}

	} else {
		panic("package reference not found")
	}
}

// func serializeStructIntegers(prgrm *CXProgram, strct *CXStruct, s *SerializedCXProgram) {
// 	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
// 	if err != nil {
// 		panic(err)
// 	}

// 	strctName := strctPkg.Name + "." + strct.Name
// 	if off, found := s.StructsMap[strctName]; found {
// 		sStrct := &s.Structs[off]
// 		sStrct.Size = int64(strct.Size)
// 	} else {
// 		panic("struct reference not found")
// 	}
// }

func serializeFunctionIntegers(prgrm *CXProgram, fn *CXFunction, s *SerializedCXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0411.
	fnPkg, err := prgrm.GetPackageFromArray(fn.Package)
	if err != nil {
		panic(err)
	}
	fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(fn.Name)
	if off, found := s.FunctionsMap[fnName]; found {
		sFn := &s.Functions[off]
		sFn.Size = int64(fn.Size)
		sFn.Length = int64(fn.LineCount)
	} else {
		panic("function reference not found")
	}
}

// initSerialization initializes the
// container for our serialized cx program.
// Program memory is also added here to our container
// if memory is to be included.
func initSerialization(prgrm *CXProgram, s *SerializedCXProgram, includeDataMemory, wholeMemory bool) {
	s.PackagesMap = make(map[string]int64)
	s.StructsMap = make(map[string]int64)
	s.FunctionsMap = make(map[string]int64)
	s.StringsMap = make(map[string]int64)

	s.Calls = make([]serializedCall, prgrm.CallCounter)
	s.Packages = make([]serializedPackage, len(prgrm.Packages))

	// Compressed serialization passes wholeMemory and stores all memory.
	// Otherwise includeDataMemory stores only the data segment.
	if wholeMemory {
		s.Memory = prgrm.Memory
	} else if includeDataMemory && len(prgrm.Memory) != 0 {
		s.DataSegmentMemory = prgrm.Memory[prgrm.Data.StartsAt : prgrm.Data.StartsAt+prgrm.Data.Size]
	}

	var numStrcts int
	var numFns int

	for _, pkg := range packagesByName(prgrm) {

		numStrcts += len(pkg.Structs)
		numFns += len(pkg.Functions)
	}

	s.Structs = make([]serializedStruct, numStrcts)
	s.Functions = make([]serializedFunction, numFns)
	// args and exprs need to be appended as they are found
}

// serializeProgram serializes
// program of cx program.
func serializeProgram(prgrm *CXProgram, s *SerializedCXProgram) {
	s.Program = serializedProgram{}
	sPrgrm := &s.Program
	sPrgrm.PackagesOffset = int64(0)
	sPrgrm.PackagesSize = int64(len(prgrm.Packages))

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0412.
	currPkg, err := prgrm.GetPackageFromArray(prgrm.CurrentPackage)
	if err != nil {
		panic(err)
	}
	if _, found := s.PackagesMap[prgrm.CXProgramNameText(currPkg.Name)]; found {
		sPrgrm.CurrentPackageName = prgrm.CXProgramNameText(currPkg.Name)
	} else {
		panic("package reference not found")
	}

	// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
	// Now a slice of objects, site S0413.
	args := []*CXArgument{}
	for _, argIdx := range prgrm.ProgramInput {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0414.
		arg := prgrm.GetCXArg(argIdx)
		args = append(args, arg)
	}
	sPrgrm.InputsOffset, sPrgrm.InputsSize = serializeSliceOfArguments(prgrm, args, s)
	//sPrgrm.OutputsOffset, sPrgrm.OutputsSize = serializeSliceOfArguments(prgrm.ProgramOutput, s)

	sPrgrm.CallStackOffset, sPrgrm.CallStackSize = serializeCalls(prgrm, prgrm.CallStack[:prgrm.CallCounter], s)

	sPrgrm.CallCounter = int64(prgrm.CallCounter)

	sPrgrm.MemoryOffset = int64(0)
	sPrgrm.MemorySize = int64(len(prgrm.Memory))

	sPrgrm.HeapPointer = int64(prgrm.Heap.Pointer)
	sPrgrm.StackPointer = int64(prgrm.Stack.Pointer)
	sPrgrm.StackSize = int64(prgrm.Stack.Size)
	sPrgrm.DataSegmentSize = int64(prgrm.Data.Size)
	sPrgrm.DataSegmentStartsAt = int64(prgrm.Data.StartsAt)
	sPrgrm.HeapSize = int64(prgrm.Heap.Size)
	sPrgrm.HeapStartsAt = int64(prgrm.Heap.StartsAt)

	sPrgrm.Terminated = serializeBoolean(prgrm.Terminated)
	sPrgrm.VersionOffset, sPrgrm.VersionSize = serializeString(strconv.FormatInt(prgrm.Version, 10), s)
}

// serializeCXProgramElements is used serializing CX program's
// elements (packages, structs, functions, etc.).
func serializeCXProgramElements(prgrm *CXProgram, s *SerializedCXProgram) {
	var fnCounter int64
	var strctCounter int64

	// indexing packages and serializing their names
	for _, pkg := range packagesByName(prgrm) {

		indexPackage(prgrm, pkg, s)
		serializePackageName(prgrm, pkg, s)
	}
	// we first needed to populate references to all packages
	// now we add the imports' references
	for _, pkg := range packagesByName(prgrm) {

		serializePackageImports(prgrm, pkg, s)
	}

	// structs
	for _, pkg := range packagesByName(prgrm) {

		for _, strctIdx := range structsByName(prgrm, pkg) {
			// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
			// Now a pointer into an arena, site S0415.
			strct := &prgrm.CXStructs[strctIdx]
			indexStruct(prgrm, strct, s)
			serializeStructName(prgrm, strct, s)
			serializeStructPackage(prgrm, strct, s)
			// serializeStructIntegers(prgrm, strct, s)
		}
	}
	// we first needed to populate references to all structs
	// now we add fields
	for _, pkg := range packagesByName(prgrm) {

		for _, strctIdx := range structsByName(prgrm, pkg) {
			// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
			// Now a pointer into an arena, site S0416.
			strct := &prgrm.CXStructs[strctIdx]
			serializeStructArguments(prgrm, strct, s)
		}
	}

	// globals
	for _, pkg := range packagesByName(prgrm) {

		serializePackageGlobals(prgrm, pkg, s)
	}

	// functions
	for _, pkg := range packagesByName(prgrm) {

		for _, fnIdx := range functionsByName(prgrm, pkg) {
			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0417.
			fn := prgrm.GetFunctionFromArray(fnIdx)

			indexFunction(prgrm, fn, s)
			serializeFunctionName(prgrm, fn, s)
			serializeFunctionPackage(prgrm, fn, s)
			serializeFunctionIntegers(prgrm, fn, s)
			serializeFunctionArguments(prgrm, fn, s)
		}
	}

	// package elements' offsets and sizes
	for _, pkg := range packagesByName(prgrm) {

		if pkgOff, found := s.PackagesMap[prgrm.CXProgramNameText(pkg.Name)]; found {
			sPkg := &s.Packages[pkgOff]

			if len(pkg.Structs) == 0 {
				sPkg.StructsOffset = int64(-1)
				sPkg.StructsSize = int64(-1)
			} else {
				sPkg.StructsOffset = strctCounter
				lenStrcts := int64(len(pkg.Structs))
				sPkg.StructsSize = lenStrcts
				strctCounter += lenStrcts
			}

			if len(pkg.Functions) == 0 {
				sPkg.FunctionsOffset = int64(-1)
				sPkg.FunctionsSize = int64(-1)
			} else {
				sPkg.FunctionsOffset = fnCounter
				lenFns := int64(len(pkg.Functions))
				sPkg.FunctionsSize = lenFns
				fnCounter += lenFns
			}
		} else {
			panic("package reference not found")
		}
	}

	// package integers
	// we needed the references to all functions and structs first
	for _, pkg := range packagesByName(prgrm) {

		serializePackageIntegers(prgrm, pkg, s)
	}

	// expressions
	for _, pkg := range packagesByName(prgrm) {

		for _, fnIdx := range functionsByName(prgrm, pkg) {
			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0418.
			fn := prgrm.GetFunctionFromArray(fnIdx)

			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S0419.
			fnPkg, err := prgrm.GetPackageFromArray(fn.Package)
			if err != nil {
				panic(err)
			}
			fnName := prgrm.CXProgramNameText(fnPkg.Name) + "." + prgrm.CXProgramNameText(fn.Name)
			if fnOff, found := s.FunctionsMap[fnName]; found {
				sFn := &s.Functions[fnOff]

				if len(fn.Expressions) == 0 {
					sFn.ExpressionsOffset = int64(-1)
					sFn.ExpressionsSize = int64(-1)
					sFn.CurrentExpressionOffset = int64(-1)
				} else {
					exprs := make([]int, len(fn.Expressions))
					for i, expr := range fn.Expressions {
						exprIdx := serializeExpression(prgrm, &expr, s)
						exprs[i] = exprIdx
					}

					sFn.ExpressionsOffset, sFn.ExpressionsSize = serializeIntegers(exprs, s)
				}
			} else {
				panic("function reference not found")
			}
		}
	}
}

// SerializeCXProgram translates cx program to slice of bytes that we can save.
// These slice of bytes can then be deserialize in the future and
// be translated back to cx program.
// A nil codec leaves the bytes uncompressed. A non-nil codec must be
// initialized, and the bytes then include the whole memory and are compressed.
func SerializeCXProgram(prgrm *CXProgram, includeDataMemory bool, codec *lz4codec.Codec) ([]byte, error) {
	b, err := encodeCXProgram(prgrm, includeDataMemory, codec != nil)
	if err != nil || codec == nil {
		return b, err
	}
	return compressCopy(codec, b)
}

// encodeCXProgram serializes prgrm without compression. wholeMemory is what
// compressed serialization stores, see initSerialization.
func encodeCXProgram(prgrm *CXProgram, includeDataMemory, wholeMemory bool) ([]byte, error) {
	s := SerializedCXProgram{}
	initSerialization(prgrm, &s, includeDataMemory, wholeMemory)

	// serialize cx program's packages,
	// structs, functions, etc.
	serializeCXProgramElements(prgrm, &s)

	// serialize cx program's program
	serializeProgram(prgrm, &s)

	// The name maps only index the slices while serializing. The generated
	// encoder writes a map in range order, so leave them out of the bytes.
	s.PackagesMap, s.StructsMap, s.FunctionsMap, s.StringsMap = nil, nil, nil, nil

	// serializing everything
	return EncodeSerializedCXProgram(&s)
}

// compressCopy compresses b with codec and copies the result out of the
// codec's buffer.
func compressCopy(codec *lz4codec.Codec, b []byte) ([]byte, error) {
	out, err := codec.Compress(b)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), out...), nil
}

// decompressCopy decompresses b with codec and copies the result out of the
// codec's buffer.
func decompressCopy(codec *lz4codec.Codec, b []byte) ([]byte, error) {
	out, err := codec.Decompress(b)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), out...), nil
}

// SerializeDebugInfo returns the byte size of each serialized segment.
// wholeMemory gives the sizes of the bytes that compressed serialization
// compresses.
func SerializeDebugInfo(prgrm *CXProgram, includeMemory, wholeMemory bool) (SerializedDataSize, error) {
	idxSize := encoder.Size(serializedCXProgramIndex{})
	var s SerializedCXProgram

	bytes, err := encodeCXProgram(prgrm, includeMemory, wholeMemory)
	if err != nil {
		return SerializedDataSize{}, err
	}
	deserializeRaw(bytes, 0, types.Cast_ui64_to_ptr(idxSize), &s.Index)

	data := &SerializedDataSize{
		Program:     len(bytes[s.Index.ProgramOffset:s.Index.CallsOffset]),
		Calls:       len(bytes[s.Index.CallsOffset:s.Index.PackagesOffset]),
		Packages:    len(bytes[s.Index.PackagesOffset:s.Index.StructsOffset]),
		Structs:     len(bytes[s.Index.StructsOffset:s.Index.FunctionsOffset]),
		Functions:   len(bytes[s.Index.FunctionsOffset:s.Index.ExpressionsOffset]),
		Expressions: len(bytes[s.Index.ExpressionsOffset:s.Index.ArgumentsOffset]),
		Arguments:   len(bytes[s.Index.ArgumentsOffset:s.Index.IntegersOffset]),
		Integers:    len(bytes[s.Index.IntegersOffset:s.Index.StringsOffset]),
		Strings:     len(bytes[s.Index.StringsOffset:s.Index.MemoryOffset]),
		Memory:      len(bytes[s.Index.MemoryOffset:]),
	}

	return *data, nil
}

func deserializeString(off int64, size int64, s *SerializedCXProgram) string {
	if size < 1 {
		return ""
	}

	var name string
	deserializeRaw(s.Strings, types.Cast_i64_to_ptr(off), types.Cast_i64_to_ptr(size), &name)

	return name
}

func deserializePackages(s *SerializedCXProgram, prgrm *CXProgram) {
	var fnCounter int64
	var strctCounter int64

	for _, sPkg := range s.Packages {
		// initializing packages with their names,
		// empty functions, structs, imports and globals
		// and current function and struct
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now a heap literal, site S0420.
		pkg := &CXPackage{}
		pkg.Name = prgrm.CXProgramNameIntern(deserializeString(sPkg.NameOffset, sPkg.NameSize, s))
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now an arena insert, site S0421.
		pkgIdx := prgrm.AddPackage(pkg)
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0422.
		pkg, _ = prgrm.GetPackageFromArray(pkgIdx)

		if sPkg.ImportsSize > 0 {
			pkg.Imports = make(map[stringpool.StringGid]CXPackageIndex, sPkg.ImportsSize)
		}

		if sPkg.FunctionsSize > 0 {
			pkg.Functions = make(map[stringpool.StringGid]CXFunctionIndex, sPkg.FunctionsSize)
			pkg.Methods = make(map[CXPackageMethodKey]CXFunctionIndex)

			for _, sFn := range s.Functions[sPkg.FunctionsOffset : sPkg.FunctionsOffset+sPkg.FunctionsSize] {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a local object, site S0423.
				var fn CXFunction
				fn.Name = prgrm.CXProgramNameIntern(deserializeString(sFn.NameOffset, sFn.NameSize, s))

				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S0424.
				fnIdx := prgrm.AddFunctionInArray(&fn)
				pkg.Functions[fn.Name] = fnIdx
				deserializeMethodKey(prgrm, pkg, fn.Name, fnIdx)
			}
		}

		if sPkg.StructsSize > 0 {
			pkg.Structs = make(map[stringpool.StringGid]CXStructIndex, sPkg.StructsSize)

			for _, sStrct := range s.Structs[sPkg.StructsOffset : sPkg.StructsOffset+sPkg.StructsSize] {
				// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
				// Now a local object, site S0425.
				var strct CXStruct
				strct.Name = prgrm.CXProgramNameIntern(deserializeString(sStrct.NameOffset, sStrct.NameSize, s))
				// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
				// Now an arena insert, site S0426.
				strctIdx := prgrm.AddStructInArray(&strct)
				pkg.Structs[strct.Name] = strctIdx
			}
		}

		// if sPkg.GlobalsSize > 0 {
		// 	pkg.Globals = make([]CXArgumentIndex, sPkg.GlobalsSize)
		// }

		// CurrentFunction
		if sPkg.FunctionsSize > 0 {
			pkg.CurrentFunction = pkg.Functions[prgrm.CXProgramNameIntern(sPkg.CurrentFunctionName)]
		}

		fnCounter += sPkg.FunctionsSize
		strctCounter += sPkg.StructsSize

		// imports
		if sPkg.ImportsSize > 0 {
			// getting indexes of imports
			idxs := deserializeIntegers(sPkg.ImportsOffset, sPkg.ImportsSize, s)

			for _, idx := range idxs {
				impPkg := deserializePackageImport(&s.Packages[idx], s, prgrm)
				pkg.Imports[impPkg.Name] = CXPackageIndex(impPkg.Index)
			}
		}

		// globals
		if sPkg.GlobalsSize > 0 {
			glblArgs := deserializeArguments(sPkg.GlobalsOffset, sPkg.GlobalsSize, s, prgrm)
			var glblArgsIdxs []CXArgumentIndex
			for _, glbl := range glblArgs {
				// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
				// Now an arena insert, site S0427.
				glblIdx := prgrm.AddCXArgInArray(glbl)
				glblArgsIdxs = append(glblArgsIdxs, CXArgumentIndex(glblIdx))
			}
			// pkg.Globals = glblArgsIdxs
			for _, glblIdx := range glblArgsIdxs {
				pkg.Globals.AddField_Globals_CXAtomicOps(prgrm, glblIdx)
			}
		}

		// structs
		if sPkg.StructsSize > 0 {
			for _, sStrct := range s.Structs[sPkg.StructsOffset : sPkg.StructsOffset+sPkg.StructsSize] {
				strctName := deserializeString(sStrct.NameOffset, sStrct.NameSize, s)
				// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
				// Now a pointer into an arena, site S0428.
				strct := &prgrm.CXStructs[pkg.Structs[prgrm.CXProgramNameIntern(strctName)]]
				deserializeStruct(&sStrct, strct, s, prgrm)
			}
		}

		// functions
		if sPkg.FunctionsSize > 0 {
			for _, sFn := range s.Functions[sPkg.FunctionsOffset : sPkg.FunctionsOffset+sPkg.FunctionsSize] {
				fnName := deserializeString(sFn.NameOffset, sFn.NameSize, s)
				// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
				// Now an arena getter, site S0429.
				pkgFn := prgrm.GetFunctionFromArray(pkg.Functions[prgrm.CXProgramNameIntern(fnName)])

				deserializeFunction(&sFn, pkgFn, s, prgrm)
			}
		}
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now an object copy, site S0430.
		prgrm.CXPackages[pkgIdx] = *pkg
	}

	// current package
	prgrm.CurrentPackage = prgrm.Packages[prgrm.CXProgramNameIntern(s.Program.CurrentPackageName)]
}

func deserializeStruct(sStrct *serializedStruct, strct *CXStruct, s *SerializedCXProgram, prgrm *CXProgram) {
	strct.Name = prgrm.CXProgramNameIntern(deserializeString(sStrct.NameOffset, sStrct.NameSize, s))
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
	// Now an arena insert, site S0431.
	strct.Fields = prgrm.AddPointerArgsToTypeSignaturesArray(deserializeArguments(sStrct.FieldsOffset, sStrct.FieldsSize, s, prgrm))

	strct.Package = prgrm.Packages[prgrm.CXProgramNameIntern(sStrct.PackageName)]
}

func deserializeArguments(off int64, size int64, s *SerializedCXProgram, prgrm *CXProgram) []*CXArgument {
	if size < 1 {
		return nil
	}

	// getting indexes of arguments
	idxs := deserializeIntegers(off, size, s)

	// sArgs := s.Arguments[off : off + size]
	// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
	// Now a slice of objects, site S0432.
	args := make([]*CXArgument, size)
	for i, idx := range idxs {
		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0433.
		args[i] = deserializeArgument(&s.Arguments[idx], s, prgrm)
	}
	return args
}

// func getStructType(sArg *serializedArgument, s *SerializedCXProgram, prgrm *CXProgram) *CXStruct {
// 	if sArg.StructTypeOffset < 0 {
// 		return nil
// 	}

// 	//structTypePkg := prgrm.Packages[s.Structs[sArg.StructTypeOffset].PackageOffset]
// 	sStrct := s.Structs[sArg.StructTypeOffset]
// 	structTypeName := deserializeString(sStrct.NameOffset, sStrct.NameSize, s)

// 	for _, strct := range structTypePkg.Structs {
// 		if strct.Name == structTypeName {
// 			return strct
// 		}
// 	}

// 	return nil
// }

// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S0434.
func deserializeArgument(sArg *serializedArgument, s *SerializedCXProgram, prgrm *CXProgram) *CXArgument {
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a local object, site S0435.
	var arg CXArgument
	arg.ArgDetails = prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
	arg.Name = prgrm.CXProgramNameIntern(deserializeString(sArg.NameOffset, sArg.NameSize, s))
	arg.Type = types.Code(sArg.Type)

	//arg.StructType = getStructType(sArg, s, prgrm)

	arg.Size = types.Cast_i64_to_ptr(sArg.Size)
	arg.Offset = types.Cast_i64_to_ptr(sArg.Offset)
	arg.PassBy = int(sArg.PassBy)

	arg.DeclarationSpecifiers = deserializeIntegers(sArg.DeclarationSpecifiersOffset, sArg.DeclarationSpecifiersSize, s)

	// arg.IsPointer = deserializeBool(sArg.IsPointer)
	// arg.IsReference = deserializeBool(sArg.IsReference)

	arg.PreviouslyDeclared = deserializeBool(sArg.PreviouslyDeclared)

	arg.Lengths = deserializePointers(sArg.LengthsOffset, sArg.LengthsSize, s)
	// TODO: include serializing of CXTypeSignature for indexes
	// arg.Indexes = prgrm.AddPointerArgsToCXArgsArray(deserializeArguments(sArg.IndexesOffset, sArg.IndexesSize, s, prgrm))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now an arena insert, site S0437.
	arg.Fields = prgrm.AddPointerArgsToCXArgsArray(deserializeArguments(sArg.FieldsOffset, sArg.FieldsSize, s, prgrm))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now an arena insert, site S0438.
	arg.Inputs = prgrm.AddPointerArgsToCXArgsArray(deserializeArguments(sArg.InputsOffset, sArg.InputsSize, s, prgrm))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now an arena insert, site S0439.
	arg.Outputs = prgrm.AddPointerArgsToCXArgsArray(deserializeArguments(sArg.OutputsOffset, sArg.OutputsSize, s, prgrm))
	arg.Package = -1
	if pkgIdx, ok := prgrm.Packages[prgrm.CXProgramNameIntern(sArg.PackageName)]; ok {
		arg.Package = pkgIdx
	}

	return &arg
}

func deserializeOperator(sExpr *serializedExpression, s *SerializedCXProgram, prgrm *CXProgram) *CXFunction {
	if sExpr.OperatorOffset < 0 {
		return nil
	}

	opPkgIdx := prgrm.Packages[prgrm.CXProgramNameIntern(s.Functions[sExpr.OperatorOffset].PackageName)]
	sOp := s.Functions[sExpr.OperatorOffset]
	opName := deserializeString(sOp.NameOffset, sOp.NameSize, s)

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0440.
	opPkg, err := prgrm.GetPackageFromArray(opPkgIdx)
	if err != nil {
		panic(err)
	}

	for _, fnIdx := range opPkg.Functions {
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0441.
		fn := prgrm.GetFunctionFromArray(fnIdx)

		if prgrm.CXProgramNameText(fn.Name) == opName {
			return fn
		}
	}

	return nil
}

func deserializePackageImport(sImp *serializedPackage, s *SerializedCXProgram, prgrm *CXProgram) *CXPackage {
	impName := deserializeString(sImp.NameOffset, sImp.NameSize, s)

	for _, pkgIdx := range prgrm.Packages {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0442.
		pkg, err := prgrm.GetPackageFromArray(pkgIdx)
		if err != nil {
			panic(err)
		}

		if prgrm.CXProgramNameText(pkg.Name) == impName {
			return pkg
		}
	}

	return nil
}

func deserializeExpressionFunction(sExpr *serializedExpression, s *SerializedCXProgram, prgrm *CXProgram) CXFunctionIndex {
	if sExpr.FunctionOffset < 0 {
		return -1
	}

	fnPkgIdx := prgrm.Packages[prgrm.CXProgramNameIntern(s.Functions[sExpr.FunctionOffset].PackageName)]
	sFn := s.Functions[sExpr.FunctionOffset]
	fnName := deserializeString(sFn.NameOffset, sFn.NameSize, s)

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0443.
	fnPkg, err := prgrm.GetPackageFromArray(fnPkgIdx)
	if err != nil {
		panic(err)
	}
	for _, fnIdx := range fnPkg.Functions {
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0444.
		fn := prgrm.GetFunctionFromArray(fnIdx)

		if prgrm.CXProgramNameText(fn.Name) == fnName {
			return fnIdx
		}
	}

	return -1
}

func deserializeExpressions(off int64, size int64, s *SerializedCXProgram, prgrm *CXProgram) []CXExpression {
	if size < 1 {
		return nil
	}

	// getting indexes of expressions
	idxs := deserializeIntegers(off, size, s)

	// sExprs := s.Expressions[off : off + size]
	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S0445.
	exprs := make([]CXExpression, size)
	for i, idx := range idxs {
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0446.
		exprs[i] = deserializeExpression(&s.Expressions[idx], s, prgrm)
	}
	return exprs
}

// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
// Now a constructor, site S0447.
func deserializeExpression(sExpr *serializedExpression, s *SerializedCXProgram, prgrm *CXProgram) CXExpression {
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a local object, site S0448.
	var expr CXExpression

	expr.ExpressionType = CXEXPR_TYPE(sExpr.ExpressionType)
	switch sExpr.Type {
	case int64(CX_LINE):
		lineGid := prgrm.CxIrState.CXLineMng.CXLineAlloc()
		cxLine := prgrm.CxIrState.CXLineMng.CXLineGet(lineGid)
		cxLine.FileName = prgrm.CXProgramSourceFileIntern(sExpr.FileName)
		cxLine.LineNumber = int(sExpr.LineNumber)
		cxLine.LineStr = prgrm.CXProgramNameIntern(sExpr.LineStr)
		expr.Index = int(objmodel.GidIndex(int64(lineGid)))
		expr.Type = CX_LINE
	case int64(CX_ATOMIC_OPERATOR):
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now a heap literal, site S0451 S0452.
		cxAtomicOpOperatorIdx := prgrm.AddFunctionInArray(&CXFunction{})
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
		// Now a heap literal, site S0453.
		cxAtomicOp := &CXAtomicOperator{Operator: cxAtomicOpOperatorIdx}

		if deserializeBool(sExpr.IsNative) {
			// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
			// Now an arena insert, site S0454.
			opIdx := prgrm.AddNativeFunctionInArray(Natives[int(sExpr.OpCode)])
			cxAtomicOp.Operator = opIdx
		} else {
			// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
			// Now an arena insert, site S0455.
			opIdx := prgrm.AddFunctionInArray(deserializeOperator(sExpr, s, prgrm))
			cxAtomicOp.Operator = opIdx
		}

		inputCXArgsArray := deserializeArguments(sExpr.InputsOffset, sExpr.InputsSize, s, prgrm)
		for _, inputCXArg := range inputCXArgsArray {
			typeSignature := GetCXTypeSignatureRepresentationOfCXArg(prgrm, inputCXArg)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
			// Now an arena insert, site S0456.
			typeSignatureIdx := prgrm.AddCXTypeSignatureInArray(typeSignature)
			cxAtomicOp.AddInput(prgrm, typeSignatureIdx)
		}

		outputCXArgsArray := deserializeArguments(sExpr.OutputsOffset, sExpr.OutputsSize, s, prgrm)
		for _, outputCXArg := range outputCXArgsArray {
			typeSignature := GetCXTypeSignatureRepresentationOfCXArg(prgrm, outputCXArg)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
			// Now an arena insert, site S0457.
			typeSignatureIdx := prgrm.AddCXTypeSignatureInArray(typeSignature)
			cxAtomicOp.AddOutput(prgrm, typeSignatureIdx)
		}

		cxAtomicOp.Label = prgrm.CXProgramNameIntern(deserializeString(sExpr.LabelOffset, sExpr.LabelSize, s))

		cxAtomicOp.ThenLines = int(sExpr.ThenLines)
		cxAtomicOp.ElseLines = int(sExpr.ElseLines)
		cxAtomicOp.Package = prgrm.Packages[prgrm.CXProgramNameIntern(sExpr.PackageName)]
		cxAtomicOp.Function = deserializeExpressionFunction(sExpr, s, prgrm)

		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
		// Now an arena insert, site S0458.
		index := prgrm.AddCXAtomicOp(cxAtomicOp)
		expr.Index = index
		expr.Type = CX_ATOMIC_OPERATOR
	}

	return expr
}

func deserializeFunction(sFn *serializedFunction, fn *CXFunction, s *SerializedCXProgram, prgrm *CXProgram) {
	fn.Name = prgrm.CXProgramNameIntern(deserializeString(sFn.NameOffset, sFn.NameSize, s))

	inputCXArgsArray := deserializeArguments(sFn.InputsOffset, sFn.InputsSize, s, prgrm)
	for _, inputCXArg := range inputCXArgsArray {
		fn.AddInput(prgrm, inputCXArg)
	}

	outputCXArgsArray := deserializeArguments(sFn.OutputsOffset, sFn.OutputsSize, s, prgrm)
	for _, outputCXArg := range outputCXArgsArray {
		fn.AddOutput(prgrm, outputCXArg)
	}

	fn.ListOfPointers = deserializeArguments(sFn.ListOfPointersOffset, sFn.ListOfPointersSize, s, prgrm)
	fn.Package = prgrm.Packages[prgrm.CXProgramNameIntern(sFn.PackageName)]
	fn.Expressions = deserializeExpressions(sFn.ExpressionsOffset, sFn.ExpressionsSize, s, prgrm)
	fn.Size = types.Cast_i64_to_ptr(sFn.Size)
	fn.LineCount = int(sFn.Length)
	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now an object copy, site S0459.
	prgrm.CXFunctions[fn.Index] = *fn
}

func deserializeBool(val int64) bool {
	return val == 1
}

func deserializePointers(off int64, size int64, s *SerializedCXProgram) []types.Pointer {
	if size < 1 {
		return nil
	}
	ints := s.Integers[off : off+size]
	res := make([]types.Pointer, len(ints))
	for i, in := range ints {
		res[i] = types.Cast_i64_to_ptr(in)
	}

	return res
}

func deserializeIntegers(off int64, size int64, s *SerializedCXProgram) []int {
	if size < 1 {
		return nil
	}
	ints := s.Integers[off : off+size]
	res := make([]int, len(ints))
	for i, in := range ints {
		res[i] = int(in)
	}

	return res
}

// initDeserialization initializes the CXProgram fields that represent a CX program. This should be refactored, as the names Deserialize and initDeserialization create some naming conflict.
func initDeserialization(prgrm *CXProgram, s *SerializedCXProgram) {
	prgrm.Memory = s.Memory
	prgrm.Packages = make(map[stringpool.StringGid]CXPackageIndex, len(s.Packages))
	// OBJMODEL: state.CXCallMng (manager storage) or []CXCallGid, phase 4 #366.
	// Now a slice of objects, site S0460.
	prgrm.CallStack = make([]CXCall, constants.CALLSTACK_SIZE)
	prgrm.Heap.StartsAt = types.Cast_i64_to_ptr(s.Program.HeapStartsAt)
	prgrm.Heap.Pointer = types.Cast_i64_to_ptr(s.Program.HeapPointer)
	prgrm.Stack.Size = types.Cast_i64_to_ptr(s.Program.StackSize)
	prgrm.Data.Size = types.Cast_i64_to_ptr(s.Program.DataSegmentSize)
	prgrm.Data.StartsAt = types.Cast_i64_to_ptr(s.Program.DataSegmentStartsAt)
	prgrm.Heap.Size = types.Cast_i64_to_ptr(s.Program.HeapSize)
	prgrm.Version = deserializeVersion(s)

	// This means reinstantiate memory and add DataSegmentMemory
	if len(s.DataSegmentMemory) > 0 && len(s.Memory) == 0 {
		minHeapSize := MinHeapSize()
		prgrm.Memory = make([]byte, constants.STACK_SIZE+minHeapSize)
		y := 0
		for i := prgrm.Data.StartsAt; i < prgrm.Data.StartsAt+prgrm.Data.Size; i++ {
			prgrm.Memory[i] = s.DataSegmentMemory[y]
			y++
		}
	}
	deserializePackages(s, prgrm)
}

// deserializeVersion returns the stored language version, 0 when it is not
// an integer, as in programs from before language versions.
func deserializeVersion(s *SerializedCXProgram) int64 {
	v, err := strconv.ParseInt(deserializeString(s.Program.VersionOffset, s.Program.VersionSize, s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// Deserialize deserializes a serialized CX program back to its golang struct representation.
// A nil codec reads b as uncompressed. A non-nil codec must be initialized.
// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
// Now a constructor, site S0461.
func Deserialize(b []byte, codec *lz4codec.Codec) (*CXProgram, error) {
	if codec != nil {
		var err error
		if b, err = decompressCopy(codec, b); err != nil {
			return nil, err
		}
	}
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Now a heap literal, site S0462.
	prgrm := &CXProgram{CxIrState: &CxIrState{}}
	prgrm.CxIrState.CxIrStateInit()
	var s SerializedCXProgram
	if _, err := DecodeSerializedCXProgram(b, &s); err != nil {
		return nil, err
	}
	if v := deserializeVersion(&s); v != constants.LanguageVersionCurrent {
		return nil, fmt.Errorf("serialized program is language version %d, this CX implements version %d, recompile it from source", v, constants.LanguageVersionCurrent)
	}
	initDeserialization(prgrm, &s)

	return prgrm, nil
}

// packagesByName returns the program's packages in name order. The serialized
// offsets depend on every walk using the same order, so all of them use this.
func packagesByName(prgrm *CXProgram) []*CXPackage {
	// OBJMODEL: state.CXPackageMng (manager storage) or []CXPackageGid, phase 4 #361.
	// Now a slice of objects, site S0463.
	pkgs := make([]*CXPackage, 0, len(prgrm.Packages))
	for _, name := range sortedKeys(prgrm, prgrm.Packages) {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0464.
		pkg, err := prgrm.GetPackageFromArray(prgrm.Packages[name])
		if err != nil {
			panic(err)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func structsByName(prgrm *CXProgram, pkg *CXPackage) []CXStructIndex {
	idxs := make([]CXStructIndex, 0, len(pkg.Structs))
	for _, name := range sortedKeys(prgrm, pkg.Structs) {
		idxs = append(idxs, pkg.Structs[name])
	}
	return idxs
}

func functionsByName(prgrm *CXProgram, pkg *CXPackage) []CXFunctionIndex {
	idxs := make([]CXFunctionIndex, 0, len(pkg.Functions))
	for _, name := range sortedKeys(prgrm, pkg.Functions) {
		idxs = append(idxs, pkg.Functions[name])
	}
	return idxs
}

func importsByName(prgrm *CXProgram, pkg *CXPackage) []CXPackageIndex {
	idxs := make([]CXPackageIndex, 0, len(pkg.Imports))
	for _, name := range sortedKeys(prgrm, pkg.Imports) {
		idxs = append(idxs, pkg.Imports[name])
	}
	return idxs
}

// sortedKeys returns the keys of m in the byte order of their names.
func sortedKeys[V any](prgrm *CXProgram, m map[stringpool.StringGid]V) []stringpool.StringGid {
	keys := make([]stringpool.StringGid, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(prgrm.CXProgramNameBytes(keys[i]), prgrm.CXProgramNameBytes(keys[j])) < 0
	})
	return keys
}

// deserializeMethodKey adds the function fnIdx named "Struct.method" to the
// methods of pkg. A function name has a dot only when it is a method.
func deserializeMethodKey(prgrm *CXProgram, pkg *CXPackage, fnName stringpool.StringGid, fnIdx CXFunctionIndex) {
	name := prgrm.CXProgramNameBytes(fnName)
	dot := bytes.IndexByte(name, '.')
	if dot < 0 {
		return
	}
	receiverName := prgrm.CxIrState.StringPool.StringPoolIntern(name[:dot])
	methodName := prgrm.CxIrState.StringPool.StringPoolIntern(name[dot+1:])
	pkg.Methods[CXPackageMethodKey{CXPackageMethodKeyStruct: receiverName, CXPackageMethodKeyMethod: methodName}] = fnIdx
}
