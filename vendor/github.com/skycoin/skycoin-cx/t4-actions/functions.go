package actions

import (
	"errors"
	"fmt"

	"github.com/jinzhu/copier"
	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// SymbolsDataKey names a symbol of a scope by its package name and its name.
type SymbolsDataKey struct {
	SymbolsDataKeyPackage stringpool.StringGid
	SymbolsDataKeyName    stringpool.StringGid
}

type SymbolsData struct {
	varCount     int
	symbols      []ast.CXTypeSignatureIndex
	symbolsIndex []map[SymbolsDataKey]int
	// functionSymbols are the signatures lookupSymbol made for function names,
	// released with their arguments at the end of FunctionDeclaration.
	functionSymbols []ast.CXTypeSignatureIndex
}

// FunctionHeader takes a function name ('fnName') and either creates the
// function if it's not known before or returns the already existing function
// if it is.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fnName - name of the function.
//	 receiver - the receiver of the method if the function is a method.
//				   i.e. func (sampleArg i32) SomeMethod(), sampleArg is the receiver.
//	 isMethod - If the function is a method (isMethod = true), then it adds the object that
//				   it's called on as the first argument.
func FunctionHeader(prgrm *ast.CXProgram, fnName string, receiver []*ast.CXArgument, isMethod bool) ast.CXFunctionIndex {
	if isMethod {
		if len(receiver) > 1 {
			panic("method has multiple receivers")
		}
		if pkg, err := prgrm.GetCurrentPackage(); err == nil {
			receiverName := receiver[0].StructType.Name
			methodName := prgrm.CXProgramNameIntern(fnName)

			if fnIdx, ok := pkg.Methods[ast.CXPackageMethodKey{CXPackageMethodKeyStruct: receiverName, CXPackageMethodKeyMethod: methodName}]; ok {
				fn := prgrm.GetFunctionFromArray(fnIdx)
				fn.AddInput(prgrm, receiver[0])
				pkg.CurrentFunction = fnIdx
				return fnIdx
			} else {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a constructor call, site S0723.
				fn := ast.MakeFunction(prgrm, prgrm.CXProgramMethodNameIntern(receiverName, methodName), CurrentFile, LineNo)
				fn.AddInput(prgrm, receiver[0])
				_, fnIdx := pkg.AddMethod(prgrm, fn, receiverName, methodName)
				pkg.CurrentFunction = fnIdx

				return fnIdx
			}
		} else {
			panic(err)
		}
	} else {
		if pkg, err := prgrm.GetCurrentPackage(); err == nil {
			fnNameGid := prgrm.CXProgramNameIntern(fnName)
			if fn, err := prgrm.GetFunction(fnNameGid, pkg.Name); err == nil {
				pkg.CurrentFunction = ast.CXFunctionIndex(fn.Index)
				return ast.CXFunctionIndex(fn.Index)
			} else {
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now a constructor call, site S0724.
				fn := ast.MakeFunction(prgrm, fnNameGid, CurrentFile, LineNo)
				_, fnIdx := pkg.AddFunction(prgrm, fn)
				pkg.CurrentFunction = fnIdx

				return fnIdx
			}
		} else {
			panic(err)
		}
	}
}

// FunctionAddParameters adds the function's input and output parameters.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fnIdx - the index of the function in the main CXFunction array.
//	 inputs - parameters to be added in the function as input.
//	 outputs - parameters to be added in the function as output.
func FunctionAddParameters(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, inputs, outputs []*ast.CXArgument) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0725.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	fnInputs := fn.GetInputs(prgrm)
	if len(fnInputs) != len(inputs) {
		// it must be a method declaration
		// so we save the first input
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
		// Now a dropped object, site S0726.
		fn.Inputs.Fields = fn.Inputs.Fields[:1]
	} else {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
		// Now a dropped object, site S0727.
		fn.Inputs.Fields = nil
	}

	// we need to wipe the inputs recognized in the first pass
	// as these don't have all the fields correctly
	fn.Outputs = nil

	for _, inp := range inputs {
		fn.AddLocalVariableName(inp.Name)
		fn.AddInput(prgrm, inp)
	}

	for _, out := range outputs {
		fn.AddLocalVariableName(out.Name)
		fn.AddOutput(prgrm, out)
	}
}

func isParseOp(prgrm *ast.CXProgram, expr *ast.CXExpression) bool {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0728.
	exprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0729.
	exprAtomicOpOperator := prgrm.GetFunctionFromArray(exprAtomicOp.Operator)
	if exprAtomicOpOperator != nil && exprAtomicOpOperator.AtomicOPCode > constants.START_PARSE_OPS && exprAtomicOpOperator.AtomicOPCode < constants.END_PARSE_OPS {
		return true
	}
	return false
}

// CheckUndValidTypes checks if an expression with a generic operator (operators that
// accept `cxcore.TYPE_UNDEFINED` arguments) is receiving arguments of valid types. For example,
// the expression `sa + sb` is not valid if they are struct instances.
func CheckUndValidTypes(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expressionIdx := exprs[currIndex].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0730.
	expressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[expressionIdx].Operator)
	if expressionOperator != nil && ast.IsOperator(expressionOperator.AtomicOPCode) && !IsAllArgsBasicTypes(prgrm, &exprs[currIndex]) {
		file, line := exprLine(prgrm, exprs, currIndex)
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeOperatorArgumentTypes, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.OpNames[expressionOperator.AtomicOPCode])})
	}
}

// ProcessFunctionParameters processes the scoping, offsets, and final size
// of the parameter args of the function.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		symbols - a slice of string-CXArg map which corresponds to
//				  the scoping of the  CXArguments. Each element in the slice
//				  corresponds to a different scope. The innermost scope is
//				  the last element of the slice.
//	 offset - offset to use by statements (excluding inputs, outputs
//				 and receiver).
//	 fnIdx - the index of the function in the main CXFunction array.
//	 params - function parameters to be processed.
func ProcessFunctionParameters(prgrm *ast.CXProgram, symbolsData *SymbolsData, offset *types.Pointer, fnIdx ast.CXFunctionIndex, params []ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0731.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	for _, paramIdx := range params {
		UpdateSymbolsTable(prgrm, symbolsData, paramIdx, offset, false)
		// We remove it from local var array since it is already added to the symbols
		// if fn.IsLocalVariable(typeSignatureName) {
		// 	err := fn.RemoveLocalVariableFromArray(typeSignatureName)
		// 	if err != nil {
		// 		panic(err)
		// 	}
		// }

		GiveOffset(prgrm, symbolsData, paramIdx)
		AddPointer(prgrm, fn, paramIdx)

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0732.
		typeSig := prgrm.GetCXTypeSignatureFromArray(paramIdx)
		var param *ast.CXArgument
		if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0733.
			param = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
		} else {
			continue
		}

		// as these are declarations, they should not have any dereference operations
		param.DereferenceOperations = nil
	}
}

// FunctionDeclaration completes the function's composition.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fnIdx - the index of the function in the main CXFunction array.
//	 inputs - parameters to be added in the function as input.
//	 outputs - parameters to be added in the function as output.
//	 exprs - expression statements inside the function.
func FunctionDeclaration(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, inputs, outputs []*ast.CXArgument, exprs []ast.CXExpression) {
	// getting offset to use by statements (excluding inputs, outputs and receiver).
	var offset types.Pointer

	symIndex := make([]map[SymbolsDataKey]int, 0)
	symIndex = append(symIndex, make(map[SymbolsDataKey]int))

	symbolsData := &SymbolsData{
		varCount: 1,

		// symbols is a slice of string-int map which corresponds to
		// the scoping of the local variables. Each element in the slice
		// corresponds to a different scope. The innermost scope is
		// the last element of the slice.
		symbolsIndex: symIndex,
		symbols:      make([]ast.CXTypeSignatureIndex, 0),
	}

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	pkg.CurrentFunction = fnIdx
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0735.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	FunctionAddParameters(prgrm, fnIdx, inputs, outputs)
	ProcessGoTos(prgrm, exprs)
	AddExprsToFunction(prgrm, fnIdx, exprs)
	ProcessFunctionParameters(prgrm, symbolsData, &offset, fnIdx, fn.GetInputs(prgrm))
	ProcessFunctionParameters(prgrm, symbolsData, &offset, fnIdx, fn.GetOutputs(prgrm))

	for i, expr := range fn.Expressions {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0736.
		exprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		for s := uint8(0); s < expr.ExpressionScopeOpenCount; s++ {
			symbolsData.symbolsIndex = append(symbolsData.symbolsIndex, make(map[SymbolsDataKey]int))
		}

		ProcessMethodCall(prgrm, &expr, symbolsData)

		ProcessExpressionArguments(prgrm, symbolsData, &offset, fnIdx, exprAtomicOp.GetInputs(prgrm), &expr, true)
		ProcessExpressionArguments(prgrm, symbolsData, &offset, fnIdx, exprAtomicOp.GetOutputs(prgrm), &expr, false)

		ProcessPointerStructs(prgrm, &expr)
		ProcessTempVariable(prgrm, &expr)
		ProcessSliceAssignment(prgrm, &expr)
		ProcessStringAssignment(prgrm, &expr)
		// ProcessReferenceAssignment(prgrm, &expr)
		ProcessShortDeclaration(prgrm, &expr, fn.Expressions, i)
		processTestExpression(prgrm, fn.Expressions, i)

		CheckTypes(prgrm, fn.Expressions, i)
		CheckUndValidTypes(prgrm, fn.Expressions, i)
		checkOperatorOperands(prgrm, fn.Expressions, i)
		ProcessTypedOperator(prgrm, fn.Expressions, i)
		assert.AssertInvariantTrue(len(symbolsData.symbolsIndex) > int(expr.ExpressionScopeCloseCount), "FunctionDeclaration: scope close without a matching open")
		symbolsData.symbolsIndex = symbolsData.symbolsIndex[:len(symbolsData.symbolsIndex)-int(expr.ExpressionScopeCloseCount)]
	}

	fn.LineCount = len(fn.Expressions)
	fn.Size = offset

	releaseFunctionSymbols(prgrm, symbolsData)
}

// ProcessTypedOperator gets the proper typed operator for the expression.
// i.e. add -> i32.add
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	exprs, currIndex - the expressions and the index of the one to process.
func ProcessTypedOperator(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expr := &exprs[currIndex]
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0737.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0738.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil {
		return
	}
	if ast.IsOperator(expressionOperator.AtomicOPCode) {
		atomicType := operatorOperandType(prgrm, expression.GetInputs(prgrm)[0])

		typedOp := ast.GetTypedOperator(atomicType, expressionOperator.AtomicOPCode)
		if typedOp == nil {
			if ast.OpcodeHandlers[expressionOperator.AtomicOPCode] == nil {
				file, line := exprLine(prgrm, exprs, currIndex)
				prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeOperatorArgumentTypes, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.OpNames[expressionOperator.AtomicOPCode])})
			}
			return
		}

		expressionOperator.AtomicOPCode = typedOp.AtomicOPCode

	}
}

// operatorOperandType returns the type an operator dispatches on for one
// operand. A pointer that is not dereferenced dispatches as types.POINTER.
func operatorOperandType(prgrm *ast.CXProgram, sigIdx ast.CXTypeSignatureIndex) types.Code {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0739.
	sig := prgrm.GetCXTypeSignatureFromArray(sigIdx)
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		if sig.Type == ast.TYPE_POINTER_ATOMIC {
			return types.POINTER
		}
		return sig.GetType(prgrm)
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S0740.
	arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(sig.Meta))
	if declaredAsPointer(arg.GetAssignmentElement(prgrm).DeclarationSpecifiers) {
		return types.POINTER
	}
	return arg.GetType(prgrm)
}

// declaredAsPointer reports whether the outermost declaration specifier left
// after dereferences and indexing is a pointer.
func declaredAsPointer(specs []int) bool {
	var stack []int
	for _, spec := range specs {
		switch spec {
		case constants.DECL_DEREF, constants.DECL_INDEXING:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, spec)
		}
	}
	return len(stack) > 0 && stack[len(stack)-1] == constants.DECL_POINTER
}

var operatorSymbols = map[int]string{
	constants.OP_EQUAL:    "==",
	constants.OP_UNEQUAL:  "!=",
	constants.OP_LT:       "<",
	constants.OP_GT:       ">",
	constants.OP_LTEQ:     "<=",
	constants.OP_GTEQ:     ">=",
	constants.OP_BITAND:   "&",
	constants.OP_BITOR:    "|",
	constants.OP_BITXOR:   "^",
	constants.OP_BITCLEAR: "&^",
	constants.OP_BITSHL:   "<<",
	constants.OP_BITSHR:   ">>",
	constants.OP_MUL:      "*",
	constants.OP_DIV:      "/",
	constants.OP_MOD:      "%",
	constants.OP_ADD:      "+",
	constants.OP_SUB:      "-",
	constants.OP_NEG:      "-",
}

// checkOperatorOperands rejects the operators that language revision 1
// removed: every operator on str, and ordering on pointers.
func checkOperatorOperands(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, currIndex)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0741.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil || !ast.IsOperator(expressionOperator.AtomicOPCode) || len(expression.GetInputs(prgrm)) == 0 {
		return
	}

	opCode := expressionOperator.AtomicOPCode
	sym := operatorSymbols[opCode]
	code := ast.CompileErrorCodeNil
	switch operatorOperandType(prgrm, expression.GetInputs(prgrm)[0]) {
	case types.STR:
		switch opCode {
		case constants.OP_EQUAL, constants.OP_UNEQUAL:
			code = ast.CompileErrorCodeStrEqualityOperator
		case constants.OP_LT, constants.OP_GT, constants.OP_LTEQ, constants.OP_GTEQ:
			code = ast.CompileErrorCodeStrOrderingOperator
		case constants.OP_ADD:
			code = ast.CompileErrorCodeStrConcatenation
		default:
			code = ast.CompileErrorCodeStrArithmeticOperator
		}
	case types.POINTER:
		switch opCode {
		case constants.OP_LT, constants.OP_GT, constants.OP_LTEQ, constants.OP_GTEQ:
			code = ast.CompileErrorCodePointerOrderingOperator
		}
	}
	if code == ast.CompileErrorCodeNil {
		return
	}

	file, line := exprLine(prgrm, exprs, currIndex)
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0742.
	if sig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0]); sig.ArgDetails != ast.CXArgumentDebugGidNil {
		if sigFile, sigLine := ast.CXArgumentDebugFileGidLine(prgrm, sig.ArgDetails); sigLine > 0 {
			file, line = sigFile, sigLine
		}
	}
	prgrm.CXProgramCompileErrorAdd(code, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(sym)})
}

// FunctionCall completes the array of expressions for postfix function call.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		exprs - the array of expressions.
//	 args - the input arg expressions to the function.
func FunctionCall(prgrm *ast.CXProgram, exprs []ast.CXExpression, args []ast.CXExpression) []ast.CXExpression {
	expr := exprs[len(exprs)-1]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0743.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0744.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	if expressionOperator == nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0745.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		// A possible method call has fields, an atomic output has none.
		fieldless := true
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0747.
			fieldless = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta)).Fields == nil
		}

		opName := expressionOutputTypeSig.Name
		opPkgIdx := expressionOutputTypeSig.Package

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0749.
		opPkg, err := prgrm.GetPackageFromArray(opPkgIdx)
		if err != nil {
			panic(err)
		}

		if op, err := prgrm.GetFunction(opName, opPkg.Name); err == nil {
			expression.Operator = ast.CXFunctionIndex(op.Index)
		} else if fieldless {
			// then it's not a possible method call
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, CurrentFile, LineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
			prgrm.CXProgramCompileErrorExit()
		} else {
			exprs[len(exprs)-1].ExpressionType = ast.CXEXPR_METHOD_CALL
		}

		if len(expression.GetOutputs(prgrm)) > 0 && fieldless {
			expression.Outputs = nil
		}
	}

	var nestedExprs []ast.CXExpression
	for i, inpExpr := range args {
		if inpExpr.Type == ast.CX_LINE {
			continue
		}

		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0750.
		inpExprAtomicOp, err := prgrm.GetCXAtomicOp(inpExpr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0751.
		inpExprAtomicOpOperator := prgrm.GetFunctionFromArray(inpExprAtomicOp.Operator)

		inpExprCXLine, _ := prgrm.GetPreviousCXLine(args, i)

		if inpExprAtomicOpOperator == nil {
			typeSigIdx := inpExprAtomicOp.GetOutputs(prgrm)[0]

			// typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)

			// // TODO: This is still to be improved
			// // Get its CX argument if its type cx arg deprecate
			// var typeSigArg *ast.CXArgument = &ast.CXArgument{}
			// if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// 	typeSigArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))

			// 	// If an atomic type, convert cx type signature to type atomic/
			// 	if ast.IsTypeAtomic(typeSigArg) {
			// 		typeSig.Name = typeSigArg.Name
			// 		typeSig.Type = ast.TYPE_ATOMIC
			// 		typeSig.Meta = int(typeSigArg.Type)
			// 		typeSig.Offset = typeSigArg.Offset
			// 		typeSig.Package = typeSigArg.Package
			// 	}
			// } else if typeSig.Type == ast.TYPE_ATOMIC || typeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// 	// do nothing
			// }

			// then it's a literal
			expression.AddInput(prgrm, typeSigIdx)
		} else {
			// then it's a function call
			if len(inpExprAtomicOp.GetOutputs(prgrm)) < 1 {
				var out *ast.CXArgument

				inpExprAtomicOpOperatorOutputs := inpExprAtomicOpOperator.GetOutputs(prgrm)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0753.
				inpExprAtomicOpOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(inpExprAtomicOpOperatorOutputs[0])
				var inpExprAtomicOpOperatorOutputArg *ast.CXArgument
				if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0755.
					inpExprAtomicOpOperatorOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inpExprAtomicOpOperatorOutputTypeSig.Meta))

					var typeSigIdx ast.CXTypeSignatureIndex

					// if undefined type, then adopt argument's type
					if inpExprAtomicOpOperatorOutputArg.Type == types.UNDEFINED {
						// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
						// Now an arena getter, site S0756.
						inpExprAtomicOpInputTypeSig := prgrm.GetCXTypeSignatureFromArray(inpExprAtomicOp.GetInputs(prgrm)[0])

						var inpExprAtomicOpInputArg *ast.CXArgument
						if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
							// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
							// Now an arena getter, site S0758.
							inpExprAtomicOpInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inpExprAtomicOpInputTypeSig.Meta))

							// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
							// Now a constructor call, site S0759.
							_, out = ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, inpExprCXLine.LineNumber)
							out.SetType(inpExprAtomicOpInputArg.Type)
							out.StructType = inpExprAtomicOpInputArg.StructType
							out.Size = inpExprAtomicOpInputArg.Size
							out.Type = inpExprAtomicOpInputArg.Type
							out.PointerTargetType = inpExprAtomicOpInputArg.PointerTargetType
							out.PreviouslyDeclared = true

							out.Package = inpExprAtomicOp.Package

							typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
						} else if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_ATOMIC {
							argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
							argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
							argDetails.FileName = CurrentFile
							argDetails.FileLine = inpExprCXLine.LineNumber
							_, outTypeSig := prgrm.AllocCXTypeSignature()
							outTypeSig.ArgDetails = argDetailsGid
							outTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
							outTypeSig.Type = ast.TYPE_ATOMIC
							outTypeSig.Meta = inpExprAtomicOpInputTypeSig.Meta
							outTypeSig.Package = inpExprAtomicOpInputTypeSig.Package
							outTypeSig.Offset = inpExprAtomicOpInputTypeSig.Offset

							typeSigIdx = outTypeSig.Index
						} else if inpExprAtomicOpInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
							argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
							argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
							argDetails.FileName = CurrentFile
							argDetails.FileLine = inpExprCXLine.LineNumber
							_, outTypeSig := prgrm.AllocCXTypeSignature()
							outTypeSig.ArgDetails = argDetailsGid
							outTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
							outTypeSig.Type = ast.TYPE_POINTER_ATOMIC
							outTypeSig.Meta = inpExprAtomicOpInputTypeSig.Meta
							outTypeSig.Package = inpExprAtomicOpInputTypeSig.Package
							outTypeSig.Offset = inpExprAtomicOpInputTypeSig.Offset

							typeSigIdx = outTypeSig.Index
						} else {
							panic("type is not known")
						}

					} else {
						// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
						// Now a constructor call, site S0767.
						_, out = ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, inpExprCXLine.LineNumber)
						out.SetType(inpExprAtomicOpOperatorOutputArg.Type)
						out.DeclarationSpecifiers = inpExprAtomicOpOperatorOutputArg.DeclarationSpecifiers
						if ast.IsTypeArrayAtomic(inpExprAtomicOpOperatorOutputArg) {
							out.Lengths = inpExprAtomicOpOperatorOutputArg.Lengths
						}

						out.StructType = inpExprAtomicOpOperatorOutputArg.StructType

						if inpExprAtomicOpOperatorOutputArg.StructType != nil {
							// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
							// Now an arena getter, site S0768.
							inpExprPkg, err := prgrm.GetPackageFromArray(inpExprAtomicOp.Package)
							if err != nil {
								panic(err)
							}
							if strct, err := inpExprPkg.GetStruct(prgrm, inpExprAtomicOpOperatorOutputArg.StructType.Name); err == nil {
								out.Size = strct.GetStructSize(prgrm)
							}
						} else {
							out.Size = inpExprAtomicOpOperatorOutputArg.Size
						}

						out.Type = inpExprAtomicOpOperatorOutputArg.Type
						out.PointerTargetType = inpExprAtomicOpOperatorOutputArg.PointerTargetType
						out.PreviouslyDeclared = true
						out.Package = inpExprAtomicOp.Package

						typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
					}

					inpExprAtomicOp.AddOutput(prgrm, typeSigIdx)
					expression.AddInput(prgrm, typeSigIdx)
				} else if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(inpExprAtomicOpOperatorOutputTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = inpExprAtomicOp.Package
					newTypeSigIdx := newTypeSig.Index

					inpExprAtomicOp.AddOutput(prgrm, newTypeSigIdx)
					expression.AddInput(prgrm, newTypeSigIdx)
				} else if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(inpExprAtomicOpOperatorOutputTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = inpExprAtomicOp.Package
					newTypeSigIdx := newTypeSig.Index

					inpExprAtomicOp.AddOutput(prgrm, newTypeSigIdx)
					expression.AddInput(prgrm, newTypeSigIdx)
				} else if inpExprAtomicOpOperatorOutputTypeSig.Type == ast.TYPE_STRUCT {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(inpExprAtomicOpOperatorOutputTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = inpExprAtomicOp.Package
					newTypeSigIdx := newTypeSig.Index

					inpExprAtomicOp.AddOutput(prgrm, newTypeSigIdx)
					expression.AddInput(prgrm, newTypeSigIdx)
				} else {
					panic("type is not known")
				}

			}
			if len(inpExprAtomicOp.GetOutputs(prgrm)) > 0 && inpExpr.IsArrayLiteral() {
				typeSig := inpExprAtomicOp.GetOutputs(prgrm)[0]
				expression.AddInput(prgrm, typeSig)
			}
			nestedExprs = append(nestedExprs, inpExpr)
		}
	}

	return append(nestedExprs, exprs...)
}

// checkSameNativeType checks if all the inputs of an expression are of the same type.
// It is used mainly to prevent implicit castings in arithmetic operations
func checkSameNativeType(prgrm *ast.CXProgram, expr *ast.CXExpression) error {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0776.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	if len(expression.GetInputs(prgrm)) < 1 {
		return errors.New("cannot perform arithmetic without operands")
	}

	var typeCode types.Code
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0777.
	expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
	if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0778.
		expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

		typeCode = expressionInputArg.Type
		if expressionInputArg.Type == types.POINTER {
			typeCode = expressionInputArg.PointerTargetType
		}
	} else {
		typeCode = expressionInputTypeSig.GetType(prgrm)
	}

	for _, inputIdx := range expression.GetInputs(prgrm) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0779.
		input := prgrm.GetCXTypeSignatureFromArray(inputIdx)

		var inpType types.Code
		if input.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0780.
			inp := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(input.Meta))

			inpType = inp.Type
			if inp.Type == types.POINTER {
				inpType = inp.PointerTargetType
			}
		} else {
			inpType = input.GetType(prgrm)
		}

		if inpType != typeCode {
			return errors.New(fmt.Sprintf("operands are not of the same type: %v!=%v", inpType, typeCode))
		}

		typeCode = inpType
	}

	return nil
}

// ProcessOperatorExpression checks if the inputs of an expression
// has the same type to prevent implicit castings in arithmetic operations.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	expr - the operator expression.
func ProcessOperatorExpression(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0781.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0782.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	if expressionOperator != nil && ast.IsOperator(expressionOperator.AtomicOPCode) {
		if err := checkSameNativeType(prgrm, expr); err != nil {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, CurrentFile, LineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
		}
	}
	if expr.IsUndType(prgrm) {
		for _, outputIdx := range expression.GetOutputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0783.
			output := prgrm.GetCXTypeSignatureFromArray(outputIdx)

			var outIdx ast.CXArgumentIndex
			if output.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				outIdx = ast.CXArgumentIndex(output.Meta)

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0784.
				out := prgrm.GetCXArgFromArray(outIdx)

				size := types.Pointer(1)
				if !ast.IsComparisonOperator(expressionOperator.AtomicOPCode) {
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S0785.
					expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])

					var expressionInputArg *ast.CXArgument
					if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S0787.
						expressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

						size = ast.GetArgSize(prgrm, expressionInputArg.GetAssignmentElement(prgrm))
					} else {
						size = expressionInputTypeSig.GetSize(prgrm, false)
					}
				}

				out.Size = size
			} else if output.Type == ast.TYPE_ATOMIC || output.Type == ast.TYPE_POINTER_ATOMIC {
				continue
			} else if output.Type == ast.TYPE_STRUCT {
				continue
			} else {
				panic("type is not known")
			}
		}
	}
}

// ProcessPointerStructs checks and processes if any args is a pointer struct.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	expr - the operator expression.
func ProcessPointerStructs(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0788.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	for _, argTypeSigIdx := range append(expression.GetInputs(prgrm), expression.GetOutputs(prgrm)...) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0789.
		argTypeSig := prgrm.GetCXTypeSignatureFromArray(argTypeSigIdx)

		var arg *ast.CXArgument
		if argTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0791.
			arg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argTypeSig.Meta))
		} else if argTypeSig.Type == ast.TYPE_ATOMIC || argTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
			continue
		} else if argTypeSig.Type == ast.TYPE_STRUCT {
			continue
		} else if argTypeSig.Type == ast.TYPE_POINTER_STRUCT {
			// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
			// Now an arena getter, site S0792.
			structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta)
			if len(structDetails.Fields) > 0 {
				argTypeSig.IsDeref = true

				for _, fldIdx := range structDetails.Fields {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0793.
					fld := prgrm.GetCXArgFromArray(fldIdx)
					doesFieldHaveDerefPointer := false
					for _, deref := range fld.DereferenceOperations {
						if deref == constants.DEREF_POINTER {
							doesFieldHaveDerefPointer = true
						}
					}
					if fld.IsPointer() && !doesFieldHaveDerefPointer {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1677.
						prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).DereferenceOperations, constants.DEREF_POINTER)
					}
				}
			}
			continue
		} else {
			panic("type is not known")
		}

		for _, fldIdx := range arg.Fields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0794.
			fld := prgrm.GetCXArgFromArray(fldIdx)
			doesFieldHaveDerefPointer := false
			for _, deref := range fld.DereferenceOperations {
				if deref == constants.DEREF_POINTER {
					doesFieldHaveDerefPointer = true
				}
			}
			if fld.IsPointer() && !doesFieldHaveDerefPointer {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1678.
				prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(fldIdx)).DereferenceOperations, constants.DEREF_POINTER)
			}
		}

		doesArgHaveDerefPointer := false
		for _, deref := range arg.DereferenceOperations {
			if deref == constants.DEREF_POINTER {
				doesArgHaveDerefPointer = true
			}
		}
		if arg.IsStruct() && arg.IsPointer() && len(arg.Fields) > 0 && !doesArgHaveDerefPointer {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1679.
			prgrm.GetCXArg(ast.CXArgumentIndex(arg.Index)).DereferenceOperations = append(arg.DereferenceOperations, constants.DEREF_POINTER)
		}
	}
}

// processTestExpression checks for the special case of test calls. `assert`, `test`, `panic` are operators where
// their first input's type needs to be the same as its second input's type. This can't be handled by
// `checkSameNativeType` because these test functions' third input parameter is always a `str`.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	exprs - the function's expressions.
//	currIndex - the index of the operator expression in exprs.
func processTestExpression(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0795.
	expression, err := prgrm.GetCXAtomicOp(exprs[currIndex].Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0796.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	if expressionOperator != nil {
		opCode := expressionOperator.AtomicOPCode
		if opCode == constants.OP_ASSERT || opCode == constants.OP_TEST || opCode == constants.OP_PANIC {
			var inp1Type, inp2Type string

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0797.
			expressionInputFirstTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
			inp1Type = ast.GetFormattedType(prgrm, expressionInputFirstTypeSig)

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0798.
			expressionInputSecondTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[1])
			inp2Type = ast.GetFormattedType(prgrm, expressionInputSecondTypeSig)
			if inp1Type != inp2Type {
				file, line := exprLine(prgrm, exprs, currIndex)
				prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeInputTypesNotEqual, file, line, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(ast.OpNames[expressionOperator.AtomicOPCode]), prgrm.CXProgramCompileErrorText(inp1Type), prgrm.CXProgramCompileErrorText(inp2Type)})
			}
		}
	}
}

// checkIndexType throws an error if the type of `idx` is not `i32` or `i64`.
func checkIndexType(prgrm *ast.CXProgram, idxIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0799.
	idxTypeSig := prgrm.GetCXTypeSignatureFromArray(idxIdx)
	var idxType string

	idxType = ast.GetFormattedType(prgrm, idxTypeSig)
	if idxType != "i32" && idxType != "i64" {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIndexType, idxTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(idxType)})
	}
}

// ProcessExpressionArguments performs a series of checks and processes to an expresion's inputs and outputs.
// Some of these checks are: checking if a an input has not been declared, assign a relative offset to the argument,
// and calculate the correct size of the argument.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		symbols - a slice of string-CXArg map which corresponds to
//				  the scoping of the  CXArguments. Each element in the slice
//				  corresponds to a different scope. The innermost scope is
//				  the last element of the slice.
//	 offset - offset to use by statements (excluding inputs, outputs
//				 and receiver).
//	 fnIdx - the index of the function in the main CXFunction array.
//	 args - the expression arguments.
//	 expr - the expression.
//	 isInput - true if args are input arguments, false if they are output args.
func ProcessExpressionArguments(prgrm *ast.CXProgram, symbolsData *SymbolsData, offset *types.Pointer, fnIdx ast.CXFunctionIndex, args []ast.CXTypeSignatureIndex, expr *ast.CXExpression, isInput bool) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0800.
	fn := prgrm.GetFunctionFromArray(fnIdx)

	for _, typeSignatureIdx := range args {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0801.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		var argIdx ast.CXArgumentIndex
		if typeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			argIdx = ast.CXArgumentIndex(typeSignature.Meta)
		} else {
			argIdx = -1
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0802.
		arg := prgrm.GetCXArgFromArray(argIdx)

		if !isInput {
			CheckRedeclared(prgrm, symbolsData, expr, typeSignatureIdx)
		}

		if !isInput {
			ProcessOperatorExpression(prgrm, expr)
		}

		// TODO: Check how to remove PreviouslyDeclared field
		// maybe its only used by temp variables
		// TempVar always have PreviouslyDeclared=true
		typeSignatureName := typeSignature.Name
		isLocalVar := fn.IsLocalVariable(typeSignatureName)
		if (arg != nil && arg.PreviouslyDeclared) || IsTempVar(prgrm, typeSignatureName) || (arg == nil && isLocalVar) {
			UpdateSymbolsTable(prgrm, symbolsData, typeSignatureIdx, offset, false)
		} else {
			UpdateSymbolsTable(prgrm, symbolsData, typeSignatureIdx, offset, true)
		}

		GiveOffset(prgrm, symbolsData, typeSignatureIdx)
		ProcessSlice(prgrm, argIdx)

		if arg != nil {
			for _, idxIdx := range arg.Indexes {
				UpdateSymbolsTable(prgrm, symbolsData, idxIdx, offset, true)
				GiveOffset(prgrm, symbolsData, idxIdx)
				checkIndexType(prgrm, idxIdx)
			}
			for _, fldIdx := range arg.Fields {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0803.
				fld := prgrm.GetCXArgFromArray(fldIdx)
				for _, idxIdx := range fld.Indexes {
					UpdateSymbolsTable(prgrm, symbolsData, idxIdx, offset, true)
					GiveOffset(prgrm, symbolsData, idxIdx)
				}
			}
		}
		AddPointer(prgrm, fn, typeSignatureIdx)
	}
}

// isPointerAdded checks if `sym` has already been added to `fn.ListOfPointers`.
func isPointerAdded(prgrm *ast.CXProgram, fn *ast.CXFunction, sym *ast.CXArgument) (found bool) {
	for _, ptr := range fn.ListOfPointers {
		if sym.Name == ptr.Name {
			if len(sym.Fields) == 0 && len(ptr.Fields) == 0 {
				found = true
				break
			}

			// Checking if we're referring to the same symbol and
			// same fields being accessed. For instance, `foo.bar` !=
			// `foo.car` as these will have different memory offset to
			// be considered by the garbage collector.
			if len(sym.Fields) > 0 &&
				len(sym.Fields) == len(ptr.Fields) &&
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now arena getters, sites S1680 S1681.
				prgrm.GetCXArg(ast.CXArgumentIndex(sym.Fields[len(sym.Fields)-1])).Name == prgrm.GetCXArg(ast.CXArgumentIndex(ptr.Fields[len(ptr.Fields)-1])).Name {
				found = true
				break
			}
		}
	}

	return found
}

// AddPointer checks if `sym` or its last field, if a struct, behaves like a
// pointer (slice, pointer, string). If this is the case, `sym` is added to
// `fn.ListOfPointers` so the CX runtime does not have to determine this.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		fn - the function the sym belongs.
//	 typeSigIdx - the index of the type signature from the main CXTypeSignature array.
func AddPointer(prgrm *ast.CXProgram, fn *ast.CXFunction, typeSigIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0804.
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
	var sym *ast.CXArgument
	if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0806.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
	} else {
		// panic("type is not cx arg deprecate\n\n)")
		// TODO: atomic type for now doesnt have pointers
		return
	}

	// Ignore if it's a global variable.
	if sym.Offset > prgrm.Stack.Size {
		return
	}
	// We first need to check if we're going to add `sym` with fields.
	// If `sym` has fields, then we `return` and we don't add the root `sym`.
	// If `sym` has no fields, then we check if `sym` is a pointer and
	// we add it if it is.

	// Field symbol:
	// Checking if it is a pointer candidate and if it was already
	// added to the list.
	if len(sym.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0807.
		field := prgrm.GetCXArgFromArray(sym.Fields[len(sym.Fields)-1])
		if field.IsPointer() && !isPointerAdded(prgrm, fn, sym) {
			fn.ListOfPointers = append(fn.ListOfPointers, sym)
		}
	}
	// Root symbol:
	// Checking if it is a pointer candidate and if it was already
	// added to the list.
	if sym.IsPointer() && !isPointerAdded(prgrm, fn, sym) {
		if len(sym.Fields) > 0 {
			// A copy of sym without its fields, owned by fn.
			_, tmp := prgrm.AllocCXArgument()
			tmpIndex := tmp.Index
			copier.Copy(tmp, sym)
			tmp.Index = tmpIndex
			// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
			// Now a dropped object, site S0809.
			tmp.Fields = nil
			fn.ListOfPointers = append(fn.ListOfPointers, tmp)
		} else {
			fn.ListOfPointers = append(fn.ListOfPointers, sym)
		}
	}
}

// CheckRedeclared checks if `expr` represents a variable declaration and then checks if an
// instance of that variable has already been declared.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		symbols - a slice of string-CXArg map which corresponds to
//				  the scoping of the  CXArguments. Each element in the slice
//				  corresponds to a different scope. The innermost scope is
//				  the last element of the slice.
//	 expr - the expression.
//	 typeSigIdx - the index of the type signature from the main CXTypeSignature array.
func CheckRedeclared(prgrm *ast.CXProgram, symbolsData *SymbolsData, expr *ast.CXExpression, typeSigIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0810.
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0811.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0812.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	if expressionOperator == nil && len(expression.GetOutputs(prgrm)) > 0 && len(expression.GetInputs(prgrm)) == 0 {
		assert.AssertInvariantTrue(len(symbolsData.symbolsIndex) > 0, "CheckRedeclared: symbol scope stack is empty")
		lastIdx := len(symbolsData.symbolsIndex) - 1

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0813.
		symPkg, err := prgrm.GetPackageFromArray(typeSig.Package)
		if err != nil {
			panic(err)
		}

		_, found := (symbolsData.symbolsIndex)[lastIdx][SymbolsDataKey{SymbolsDataKeyPackage: symPkg.Name, SymbolsDataKeyName: typeSig.Name}]
		if found {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeRedeclared, typeSig.ArgDetails, ast.CompileErrorOperands{int64(typeSig.Name)})
		}
	}
}

// ProcessGoTos sets the ThenLines value if the expression is a goto.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 exprs - slice of expressions.
func ProcessGoTos(prgrm *ast.CXProgram, exprs []ast.CXExpression) {
	for i, expr := range exprs {
		expression, _, err := prgrm.GetOperation(&expr)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0814.
		expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
		expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
		expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

		opGotoFn := ast.Natives[constants.OP_GOTO]
		isOpGoto := false
		if expressionOperator != nil && expressionOperator.AtomicOPCode == opGotoFn.AtomicOPCode && len(expressionOperatorInputs) == len(opGotoFn.Inputs) && len(expressionOperatorOutputs) == len(opGotoFn.Outputs) {
			isOpGoto = true
		}
		if isOpGoto {
			// then it's a goto
			for j, e := range exprs {
				expressionToGoTo, _, err := prgrm.GetOperation(&e)
				if err != nil {
					panic(err)
				}

				if expressionToGoTo.Label == expression.Label && i != j {
					// ElseLines is used because arg's default val is false
					expression.ThenLines = j - i - 1
					break
				}
			}
		}
	}
}

// AddExprsToFunction add all expressions to the function.
func AddExprsToFunction(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, exprs []ast.CXExpression) {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0815.
	fn := prgrm.GetFunctionFromArray(fnIdx)
	for _, expr := range exprs {
		fn.AddExpression(prgrm, &expr)
	}
}

func checkMatchParamTypes(prgrm *ast.CXProgram, expr *ast.CXExpression, expected, received []ast.CXTypeSignatureIndex, isInputs bool) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0816.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0817.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	for i, expectedTypeSigIdx := range expected {
		var receivedType, expectedType string

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0818.
		expectedTypeSig := prgrm.GetCXTypeSignatureFromArray(expectedTypeSigIdx)
		// An atomic expected type is neither a pointer argument nor undefined.
		var expectedPointer, expectedUndefined bool
		if expectedTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0820.
			expectedArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expectedTypeSig.Meta))
			expectedPointer = expectedArg.IsPointer()
			expectedUndefined = expectedArg.Type == types.UNDEFINED
		}
		expectedType = ast.GetFormattedType(prgrm, expectedTypeSig)

		if expr.IsMethodCall() && (expectedPointer || expectedTypeSig.Type == ast.TYPE_POINTER_STRUCT) && i == 0 {
			// if method receiver is pointer, remove *
			if expectedType[0] == '*' {
				// we need to check if it's not an `str`
				// otherwise we end up removing the `s` instead of a `*`
				expectedType = expectedType[1:]
			}
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0821.
		receivedTypeSig := prgrm.GetCXTypeSignatureFromArray(received[i])
		receivedType = ast.GetFormattedType(prgrm, receivedTypeSig)

		if expectedType != receivedType && !expectedUndefined {
			var opName string
			if expressionOperator.IsBuiltIn() {
				opName = ast.OpNames[expressionOperator.AtomicOPCode]
			} else {
				opName = prgrm.CXProgramNameText(expressionOperator.Name)
			}

			if isInputs {
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeFunctionInputType, receivedTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})
			} else {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0822.
				expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[i])
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeFunctionReceiverType, expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})

			}

		}

		var expressionOutputTypeSig *ast.CXTypeSignature
		if expressionOperator.AtomicOPCode == constants.OP_IDENTITY {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0823.
			expressionOutputTypeSig = prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		}
		// In the case of assignment we need to check that the input's type matches the output's type.
		// FIXME: There are some expressions added by the cxgo where temporary variables are used.
		// These temporary variables' types are not properly being set. That's why we use !cxcore.IsTempVar to
		// exclude these cases for now.
		if expressionOperator.AtomicOPCode == constants.OP_IDENTITY && !IsTempVar(prgrm, expressionOutputTypeSig.Name) {
			var outType, inpType string

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0824.
			expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
			inpType = ast.GetFormattedType(prgrm, expressionInputTypeSig)

			var expressionOutputArg *ast.CXArgument
			expressionOutputTypeSigName := expressionOutputTypeSig.Name
			if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0826.
				expressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
				expressionOutputTypeSigName = expressionOutputArg.GetAssignmentElement(prgrm).Name
			}

			outType = ast.GetFormattedType(prgrm, expressionOutputTypeSig)

			// We use `isInputs` to only print the error once.
			// Otherwise we'd print the error twice: once for the input and again for the output
			if inpType != outType && isInputs {
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeAssignValueType, expressionInputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(inpType), int64(expressionOutputTypeSigName), prgrm.CXProgramCompileErrorText(outType)})
			}

		}
	}
}

func checkMatchExprNumOfInputs(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator, expressionOperatorInputs []ast.CXTypeSignatureIndex, opName string, exprCXLine *ast.CXLine, builtIn bool) {
	// checking if number of inputs is not the same as the required number of inputs
	if len(expression.GetInputs(prgrm)) == len(expressionOperatorInputs) {
		return
	}

	// built-ins without declared inputs, such as break and goto, take a label
	if builtIn && len(expressionOperatorInputs) == 0 {
		return
	}

	// if the last input is of type undefined then it might be a variadic function, such as printf
	if len(expressionOperatorInputs) > 0 && len(expression.GetInputs(prgrm)) > len(expressionOperatorInputs) {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0827.
		lastTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorInputs[len(expressionOperatorInputs)-1])
		switch lastTypeSig.Type {
		case ast.TYPE_CXARGUMENT_DEPRECATE:
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0828.
			if prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastTypeSig.Meta)).Type == types.UNDEFINED {
				return
			}
		case ast.TYPE_ATOMIC:
			if types.Code(lastTypeSig.Meta) == types.UNDEFINED {
				return
			}
		}
	}

	// then we need to be strict in the number of inputs
	prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeOperatorInputCount, exprCXLine.FileName, exprCXLine.LineNumber, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName), int64(len(expressionOperatorInputs)), int64(len(expression.GetInputs(prgrm)))})
	prgrm.CXProgramCompileErrorExit()
}

func checkMatchExprNumOfOutputs(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator, expressionOperatorOutputs []ast.CXTypeSignatureIndex, opName string, exprCXLine *ast.CXLine) {
	// checking if number of outputs is not the same as the required number of outputs
	if len(expression.GetOutputs(prgrm)) != len(expressionOperatorOutputs) {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeOperatorOutputCount, exprCXLine.FileName, exprCXLine.LineNumber, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(opName), int64(len(expressionOperatorOutputs)), int64(len(expression.GetOutputs(prgrm)))})
		prgrm.CXProgramCompileErrorExit()
	}
}

// CheckTypes checks if the expected types are provided and outputted correctly.
func CheckTypes(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) {
	expression, err := prgrm.GetCXAtomicOpFromExpressions(exprs, currIndex)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0829.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	exprCXLine, _ := prgrm.GetPreviousCXLine(exprs, currIndex)

	if expressionOperator != nil {
		opName := expression.GetOperatorName(prgrm)
		expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
		expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

		// checking if number of inputs is not the same as the required number of inputs
		checkMatchExprNumOfInputs(prgrm, expression, expressionOperatorInputs, opName, exprCXLine, expressionOperator.IsBuiltIn())
		// checking if number of outputs is not the same as the required number of outputs
		checkMatchExprNumOfOutputs(prgrm, expression, expressionOperatorOutputs, opName, exprCXLine)
	}

	if expressionOperator != nil && expressionOperator.IsBuiltIn() && expressionOperator.AtomicOPCode == constants.OP_IDENTITY {
		for i := range expression.GetInputs(prgrm) {
			var expectedType string
			var receivedType string

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0830.
			expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[i])
			if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0831.
				expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

				if expressionInputArg.GetAssignmentElement(prgrm).StructType != nil {
					// then it's custom type
					receivedType = prgrm.CXProgramNameText(expressionInputArg.GetAssignmentElement(prgrm).StructType.Name)
				} else {
					// then it's native type
					receivedType = expressionInputArg.GetAssignmentElement(prgrm).Type.Name()

					if expressionInputArg.GetAssignmentElement(prgrm).Type == types.POINTER {
						receivedType = expressionInputArg.GetAssignmentElement(prgrm).PointerTargetType.Name()
					}
				}

			} else {
				receivedType = expressionInputTypeSig.GetTypeString(prgrm)
			}

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0832.
			expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[i])
			var expressionOutputArg *ast.CXArgument
			if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0834.
				expressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
				expressionOutputArg = expressionOutputArg.GetAssignmentElement(prgrm)

				if expressionOutputArg.StructType != nil {
					// then it's custom type
					expectedType = prgrm.CXProgramNameText(expressionOutputArg.StructType.Name)
				} else {
					// then it's native type
					expectedType = expressionOutputArg.Type.Name()

					if expressionOutputArg.Type == types.POINTER {
						expectedType = expressionOutputArg.PointerTargetType.Name()
					}
				}
			} else {
				expectedType = expressionOutputTypeSig.GetTypeString(prgrm)
			}

			if receivedType != expectedType {
				if exprs[currIndex].IsStructLiteral() {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0835.
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructLiteralFieldType, expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{int64(prgrm.GetCXArgFromArray(expressionOutputArg.Fields[0]).Name), int64(expressionOutputArg.StructType.Name), prgrm.CXProgramCompileErrorText(expectedType), prgrm.CXProgramCompileErrorText(receivedType)})
				} else {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeAssignSymbolType, expressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(receivedType), int64(expressionOutputTypeSig.Name), prgrm.CXProgramCompileErrorText(expectedType)})
				}
			}

		}
	}

	// then it's a function call and not a declaration
	if expressionOperator != nil {
		// checking inputs matching operator's inputs
		checkMatchParamTypes(prgrm, &exprs[currIndex], expressionOperator.GetInputs(prgrm), expression.GetInputs(prgrm), true)

		// checking outputs matching operator's outputs
		checkMatchParamTypes(prgrm, &exprs[currIndex], expressionOperator.GetOutputs(prgrm), expression.GetOutputs(prgrm), false)
	}
}

// ProcessStringAssignment sets the args PassBy to PASSBY_VALUE if the type is string.
func ProcessStringAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0836.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0837.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

	opIdentFn := ast.Natives[constants.OP_IDENTITY]
	isOpIdent := false
	if expressionOperator != nil && expressionOperator.AtomicOPCode == opIdentFn.AtomicOPCode && len(expressionOperatorInputs) == len(opIdentFn.Inputs) && len(expressionOperatorOutputs) == len(opIdentFn.Outputs) {
		isOpIdent = true
	}
	if isOpIdent {
		for i, outputIdx := range expression.GetOutputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0838.
			output := prgrm.GetCXTypeSignatureFromArray(outputIdx)

			// Only a CXArgument can hold a str or aff value, typed signatures are never strings.
			if output.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
				continue
			}
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0839.
			out := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(output.Meta))

			if len(expression.GetInputs(prgrm)) > i {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0840.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[i])
				if expressionInputTypeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
					continue
				}
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0841.
				expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

				out = out.GetAssignmentElement(prgrm)
				inp := expressionInputArg.GetAssignmentElement(prgrm)

				if (out.Type == types.STR || out.Type == types.AFF) && out.Name != stringpool.StringGidNil &&
					(inp.Type == types.STR || inp.Type == types.AFF) && inp.Name != stringpool.StringGidNil {
					out.PassBy = constants.PASSBY_VALUE
				}
			}
		}
	}
}

// ProcessReferenceAssignment checks if the reference of a symbol can be assigned to the expression's output.
// For example: `var foo i32; var bar i32; bar = &foo` is not valid.
// func ProcessReferenceAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression) {
// 	expression, err := prgrm.GetCXAtomicOp(expr.Index)
// 	if err != nil {
// 		panic(err)
// 	}

// 	for _, outIdx := range expression.Outputs {
// 		out := prgrm.GetCXArgFromArray(outIdx)
// 		elt := out.GetAssignmentElement(prgrm)
// 		if elt.PassBy == constants.PASSBY_REFERENCE &&
// 			!hasDeclSpec(elt, constants.DECL_POINTER) &&
// 			elt.PointerTargetType != types.STR && elt.Type != types.STR && !elt.IsSlice {
// 			println(ast.CompilationError(prgrm.CXProgramSourceFileText(CurrentFile), LineNo), "invalid reference assignment", elt.Name)
// 		}
// 	}

// }

// ProcessShortDeclaration sets proper values if the expr is a short declaration.
func ProcessShortDeclaration(prgrm *ast.CXProgram, expr *ast.CXExpression, expressions []ast.CXExpression, idx int) {
	if len(expressions) <= 1 {
		return
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0842.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	var expressionOutputArgType types.Code
	if len(expression.GetOutputs(prgrm)) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0843.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0844.
			expressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
			expressionOutputArgType = expressionOutputArg.Type
		} else {
			expressionOutputArgType = expressionOutputTypeSig.GetType(prgrm)
		}
	}

	// process short declaration
	if len(expression.GetOutputs(prgrm)) > 0 && len(expression.GetInputs(prgrm)) > 0 && expressionOutputArgType == types.IDENTIFIER && !expr.IsStructLiteral() && !isParseOp(prgrm, expr) {
		prevExpression, err := prgrm.GetPreviousCXAtomicOpFromExpressions(expressions, idx-1)
		if err != nil {
			return
		}

		var argType, argPointerTargetType types.Code
		var argSize types.Pointer
		if expr.IsMethodCall() {

			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0845.
			expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
			expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0846.
			expressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorOutputs[0])

			if expressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0847.
				expressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOperatorOutputTypeSig.Meta))
				argType = expressionOperatorOutputArg.Type
				argPointerTargetType = expressionOperatorOutputArg.PointerTargetType
				argSize = expressionOperatorOutputArg.Size
			} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				argType = types.Code(expressionOperatorOutputTypeSig.Meta)
				argSize = types.Code(expressionOperatorOutputTypeSig.Meta).Size()
			} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				argType = types.Code(expressionOperatorOutputTypeSig.Meta)
				argSize = types.Code(expressionOperatorOutputTypeSig.Meta).Size()
			} else {
				panic("type is not known")
			}
		} else {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0848.
			expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
			if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0849.
				expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
				argType = expressionInputArg.Type
				argPointerTargetType = expressionInputArg.PointerTargetType
				argSize = expressionInputArg.Size
			} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
				argType = types.Code(expressionInputTypeSig.Meta)
				argSize = types.Code(expressionInputTypeSig.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				argType = types.Code(expressionInputTypeSig.Meta)
				argSize = types.Code(expressionInputTypeSig.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0850.
				arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
				argType = types.Code(arrDetails.Meta)
				argSize = types.Code(arrDetails.Meta).Size()
			} else if expressionInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0851.
				sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
				argType = types.Code(sliceDetails.Meta)
				argSize = types.Code(sliceDetails.Meta).Size()
			} else {
				panic("type is not known")
			}
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0852.
		prevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prevExpression.GetOutputs(prgrm)[0])
		var prevExpressionOutputIdx ast.CXArgumentIndex
		if prevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			prevExpressionOutputIdx = ast.CXArgumentIndex(prevExpressionOutputTypeSig.Meta)

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1682.
			prgrm.GetCXArg(ast.CXArgumentIndex(prevExpressionOutputIdx)).Type = argType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1683.
			prgrm.GetCXArg(ast.CXArgumentIndex(prevExpressionOutputIdx)).PointerTargetType = argPointerTargetType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1684.
			prgrm.GetCXArg(ast.CXArgumentIndex(prevExpressionOutputIdx)).Size = argSize
		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			prevExpressionOutputTypeSig.Meta = int(argType)
		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			prevExpressionOutputTypeSig.Meta = int(argType)
		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0853.
			arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(prevExpressionOutputTypeSig.Meta)
			arrDetails.Meta = int(argType)

		} else if prevExpressionOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0854.
			sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(prevExpressionOutputTypeSig.Meta)
			sliceDetails.Meta = int(argType)

		} else {
			panic("type is not known")
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0855.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			expressionOutputIdx := ast.CXArgumentIndex(expressionOutputTypeSig.Meta)

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1685.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Type = argType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1686.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).PointerTargetType = argPointerTargetType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1687.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Size = argSize
		} else if expressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionOutputTypeSig.Meta = int(argType)
		} else if expressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionOutputTypeSig.Meta = int(argType)
		} else if expressionOutputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0856.
			arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionOutputTypeSig.Meta)
			arrDetails.Meta = int(argType)
		} else if expressionOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0857.
			sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionOutputTypeSig.Meta)
			sliceDetails.Meta = int(argType)
		} else {
			panic("type is not known")
		}
	}
}

// ProcessSlice sets DereferenceOperations if the arg is a slice.
func ProcessSlice(prgrm *ast.CXProgram, inpIdx ast.CXArgumentIndex) {
	if inpIdx == -1 {
		return
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S0858.
	inp := prgrm.GetCXArgFromArray(inpIdx)

	var elt *ast.CXArgument
	if len(inp.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0860.
		elt = prgrm.GetCXArgFromArray(inp.Fields[len(inp.Fields)-1])
	} else {
		elt = inp
	}

	if elt.IsSlice() && len(elt.DereferenceOperations) > 0 && elt.DereferenceOperations[len(elt.DereferenceOperations)-1] == constants.DEREF_POINTER {
		elt.DereferenceOperations = elt.DereferenceOperations[:len(elt.DereferenceOperations)-1]
		return
	}

	if elt.IsSlice() && len(elt.DereferenceOperations) > 0 && len(inp.Fields) == 0 {
		return
		// elt.DereferenceOperations = append([]int{cxcore.DEREF_POINTER}, elt.DereferenceOperations...)
	}
}

// ProcessSliceAssignent sets correct values for slice assignment expressions.
func ProcessSliceAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0861.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0862.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

	opIdentFn := ast.Natives[constants.OP_IDENTITY]
	isOpIdent := false
	if expressionOperator != nil && expressionOperator.AtomicOPCode == opIdentFn.AtomicOPCode && len(expressionOperatorInputs) == len(opIdentFn.Inputs) && len(expressionOperatorOutputs) == len(opIdentFn.Outputs) {
		isOpIdent = true
	}

	if isOpIdent {
		var inp *ast.CXArgument
		var out *ast.CXArgument

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0865.
		expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
		var expressionInputArg *ast.CXArgument
		if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0867.
			expressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
		} else {
			// panic("type is not cx argument deprecate\n\n")
			return
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0868.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		var expressionOutputArg *ast.CXArgument
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0870.
			expressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
		} else {
			// panic("type is not cx argument deprecate\n\n")
			return
		}

		inp = expressionInputArg.GetAssignmentElement(prgrm)
		out = expressionOutputArg.GetAssignmentElement(prgrm)

		if inp.IsSlice() && out.IsSlice() && len(inp.Indexes) == 0 && len(out.Indexes) == 0 {
			out.PassBy = constants.PASSBY_VALUE
		}
	}
	if expressionOperator != nil && !expressionOperator.IsBuiltIn() {
		// then it's a function call
		for _, inputIdx := range expression.GetInputs(prgrm) {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0871.
			input := prgrm.GetCXTypeSignatureFromArray(inputIdx)

			// Only a CXArgument slice needs its PassBy changed here.
			if input.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
				continue
			}
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0872.
			inp := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(input.Meta))

			assignElt := inp.GetAssignmentElement(prgrm)

			// we want to pass by value if we're sending the slice as a whole (no indexing)
			// unless it's a pointer to the slice
			if assignElt.IsSlice() && len(assignElt.Indexes) == 0 && !hasDeclSpec(assignElt, constants.DECL_POINTER) {
				assignElt.PassBy = constants.PASSBY_VALUE
			}
		}
	}
}

// lookupSymbol searches for `ident` in `symbols`, starting from the innermost scope.
func lookupSymbol(prgrm *ast.CXProgram, pkgName stringpool.StringGid, ident stringpool.StringGid, symbolsData *SymbolsData) (*ast.CXTypeSignature, error) {
	fullName := SymbolsDataKey{SymbolsDataKeyPackage: pkgName, SymbolsDataKeyName: StripNameNumber(prgrm, ident)}
	for c := len(symbolsData.symbolsIndex) - 1; c >= 0; c-- {
		if symIdx, found := (symbolsData.symbolsIndex)[c][fullName]; found {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S1776.
			return prgrm.GetCXTypeSignatureFromArray(symbolsData.symbols[symIdx]), nil
		}
	}

	// Checking if `ident` refers to a function.
	pkg, err := prgrm.GetPackage(pkgName)
	if err != nil {
		return nil, err
	}

	notFound := errors.New("identifier '" + prgrm.CXProgramNameText(ident) + "' does not exist")
	// We're not checking for that error
	fn, err := pkg.GetFunction(prgrm, ident)
	if err != nil {
		return nil, errors.New(err.Error() + ":" + notFound.Error() + fmt.Sprintf("--fullName=%s.%s", prgrm.CXProgramNameText(pkgName), prgrm.CXProgramNameText(fullName.SymbolsDataKeyName)))
	}
	// Then we found a function by that name. Let's create a `cxcore.CXArgument` of
	// type `func` with that name.
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0873.
	_, fnArg := ast.MakeArgument(prgrm, ident, fn.FileName, fn.FileLine)
	fnArg.SetType(types.FUNC)
	fnArg.Package = ast.CXPackageIndex(pkg.Index)

	fnArgIdx := ast.CXArgumentIndex(fnArg.Index)
	_, fnArgTypeSig := prgrm.AllocCXTypeSignature()
	fnArgTypeSig.Name = fnArg.Name
	fnArgTypeSig.Offset = fnArg.Offset
	fnArgTypeSig.Package = fnArg.Package
	fnArgTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
	fnArgTypeSig.Meta = int(fnArgIdx)
	fnArgTypeSig.ArgDetails = fnArg.ArgDetails
	symbolsData.functionSymbols = append(symbolsData.functionSymbols, fnArgTypeSig.Index)

	return fnArgTypeSig, nil
}

// UpdateSymbolsTable adds `sym` to the innermost scope (last element of slice) in `symbols`.
func UpdateSymbolsTable(prgrm *ast.CXProgram, symbolsData *SymbolsData, typeSigIdx ast.CXTypeSignatureIndex, offset *types.Pointer, shouldAlreadyExist bool) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0876.
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)

	var sym *ast.CXArgument
	if typeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0878.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
		if sym.Type == types.UNDEFINED && !IsTempVar(prgrm, typeSig.Name) {
			return
		}
	} else {
		// panic("type is not cx arg deprecate\n\n")
		// TODO: temporary return only
		sym = nil
	}

	if typeSig.Name != stringpool.StringGidNil {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0879.
		typeSigPkg, err := prgrm.GetPackageFromArray(typeSig.Package)
		if err != nil {
			panic(err)
		}

		currFn, err := prgrm.GetCurrentFunction()
		if err != nil {
			panic("error getting current function")
		}

		if !currFn.IsLocalVariable(typeSig.Name) {
			GetGlobalSymbol(prgrm, symbolsData, typeSigPkg, typeSig.Name)
		}

		assert.AssertInvariantTrue(len(symbolsData.symbolsIndex) > 0, "UpdateSymbolsTable: symbol scope stack is empty")
		lastIdx := len(symbolsData.symbolsIndex) - 1
		fullName := SymbolsDataKey{SymbolsDataKeyPackage: typeSigPkg.Name, SymbolsDataKeyName: StripNameNumber(prgrm, typeSig.Name)}

		// outerSym, err := lookupSymbol(sym.Package.Name, sym.Name, symbols)
		_, err = lookupSymbol(prgrm, typeSigPkg.Name, typeSig.Name, symbolsData)
		_, found := (symbolsData.symbolsIndex)[lastIdx][fullName]

		// then it wasn't found in any scope
		if err != nil && shouldAlreadyExist {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIdentifierMissing, typeSig.ArgDetails, ast.CompileErrorOperands{int64(typeSig.Name)})
		}

		// then it was already added in the innermost scope
		if found {
			return
		}

		// then it is a new declaration
		if !shouldAlreadyExist && !found {
			// We remove it from local var array since it is already added to the symbols
			err := currFn.RemoveLocalVariableFromArray(typeSig.Name)
			if err != nil {
				panic(err)
			}

			if !IsTempVar(prgrm, typeSig.Name) {
				// Change name to n:varname format
				typeSig.Name = prgrm.CXProgramNameIntern(fmt.Sprintf("%v:%s", symbolsData.varCount, prgrm.CXProgramNameText(typeSig.Name)))
				symbolsData.varCount++
			}

			symbolsData.symbols = append(symbolsData.symbols, typeSigIdx)
			// add name to symbols
			symbolsData.symbolsIndex[lastIdx][fullName] = len(symbolsData.symbols) - 1

			if sym != nil {
				sym.Name = typeSig.Name
				sym.Offset = *offset
				updateSymbolsTableOffsetAdd(prgrm, typeSig, offset, ast.GetArgSize(prgrm, sym))
			} else {
				typeSig.Offset = *offset
				updateSymbolsTableOffsetAdd(prgrm, typeSig, offset, typeSig.GetSize(prgrm, true))
			}
		}
	}
}

// updateSymbolsTableOffsetAdd moves the frame offset past size bytes of
// typeSig, recording a frame that does not fit declarationSizeLimit.
func updateSymbolsTableOffsetAdd(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature, offset *types.Pointer, size types.Pointer) {
	if !declarationSizeSumFits(*offset, size) {
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDeclarationTooLarge, typeSig.ArgDetails, ast.CompileErrorOperands{int64(StripNameNumber(prgrm, typeSig.Name))})
	}
	*offset += size
}

// ProcessMethodCall completes the method call expression's inputs and outputs.
//
// Input arguments description:
// 	prgrm - a CXProgram that contains all the data and arrays of the program.
//  expr - the method call expression.
// 	symbols - a slice of string-CXArg map which corresponds to
// 			  the scoping of the  CXArguments. Each element in the slice
// 			  corresponds to a different scope. The innermost scope is
// 			  the last element of the slice.

// TODO: improve structure ASAP
func ProcessMethodCall(prgrm *ast.CXProgram, expr *ast.CXExpression, symbolsData *SymbolsData) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0880.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	if !expr.IsMethodCall() {
		return
	}

	var inpIdx ast.CXArgumentIndex = -1
	var outIdx ast.CXArgumentIndex = -1

	if len(expression.GetInputs(prgrm)) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0881.
		expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
		if expressionInputTypeSig.Name != stringpool.StringGidNil && expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			inpIdx = ast.CXArgumentIndex(expressionInputTypeSig.Meta)
		} else if expressionInputTypeSig.Name != stringpool.StringGidNil && expressionInputTypeSig.Type == ast.TYPE_STRUCT {
			println("went here type struct for processMethodCall")
		}
	}

	if len(expression.GetOutputs(prgrm)) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0882.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		if expressionOutputTypeSig.Name != stringpool.StringGidNil && expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			outIdx = ast.CXArgumentIndex(expressionOutputTypeSig.Meta)
		}
	}

	if inpIdx != -1 {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0883. Also CXArgumentGet, phase 5 #327, site S1689.
		inpPkg, err := prgrm.GetPackageFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Package)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1690.
		if argInpTypeSignature, err := lookupSymbol(prgrm, inpPkg.Name, prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Name, symbolsData); err == nil {
			var argInpStructType *ast.CXStruct
			if argInpTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0884.
				argInp := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argInpTypeSignature.Meta))
				argInpStructType = argInp.StructType
			} else if argInpTypeSignature.Type == ast.TYPE_ATOMIC || argInpTypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
			} else if argInpTypeSignature.Type == ast.TYPE_STRUCT {
				// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
				// Now an arena getter, site S0885.
				structDetails := prgrm.GetCXTypeSignatureStructFromArray(argInpTypeSignature.Meta)
				argInpStructType = structDetails.StructType
			} else {
				panic("type is not known")
			}

			// then we found an input
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1691.
			if len(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields) > 0 && argInpStructType != nil {
				strct := argInpStructType

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1692.
				for _, fldIdx := range prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0886.
					field := prgrm.GetCXArgFromArray(fldIdx)
					if inFld, err := strct.GetField(prgrm, field.Name); err == nil {
						if inFld.GetStructType(prgrm) != nil {
							strct = inFld.GetStructType(prgrm)
						}
					}
				}

				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0887.
				strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0888. Also CXArgumentGet, phase 5 #327, site S1694.
				if fnIdx, err := strctPkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields)-1]).Name); err == nil {
					prgrm.CXAtomicOps[expr.Index].Operator = fnIdx
				} else {
					processMethodCallMissing(prgrm, strct, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields)-1]))
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
				// Now a dropped object, site S0889. Also CXArgumentGet, phase 5 #327, site S1696.
				prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields = prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields[:len(prgrm.GetCXArg(ast.CXArgumentIndex(inpIdx)).Fields)-1]
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1697.
			} else if len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields) > 0 {
				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0890. Also CXArgumentGet, phase 5 #327, site S1699.
				outPkg, err := prgrm.GetPackageFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Package)
				if err != nil {
					panic(err)
				}
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1700.
				argOutTypeSignature, err := lookupSymbol(prgrm, outPkg.Name, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Name, symbolsData)
				if err != nil {
					panic(err)
				}

				// An atomic output has no struct type.
				var strct *ast.CXStruct
				var argOutType string
				if argOutTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0892.
					argOut := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argOutTypeSignature.Meta))
					argOutType = argOut.Type.Name()
					strct = argOut.StructType
				} else {
					argOutType = argOutTypeSignature.GetTypeString(prgrm)
				}

				if strct == nil {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodePrimitiveFieldAccess, argOutTypeSignature.ArgDetails, ast.CompileErrorOperands{int64(argOutTypeSignature.Name), prgrm.CXProgramCompileErrorText(argOutType)})
					prgrm.CXProgramCompileErrorExit()
				}

				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
				// Now a dropped object, site S0893.
				prgrm.CXAtomicOps[expr.Index].Inputs.Fields = append(prgrm.CXAtomicOps[expr.Index].Outputs.Fields[:1], prgrm.CXAtomicOps[expr.Index].Inputs.Fields...)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
				// Now a dropped object, site S0894.
				prgrm.CXAtomicOps[expr.Index].Outputs.Fields = prgrm.CXAtomicOps[expr.Index].Outputs.Fields[:len(prgrm.CXAtomicOps[expr.Index].Outputs.Fields)-1]

				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0895.
				strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0896. Also CXArgumentGet, phase 5 #327, site S1702.
				if fnIdx, err := strctPkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]).Name); err == nil {
					prgrm.CXAtomicOps[expr.Index].Operator = fnIdx
				} else {
					processMethodCallMissing(prgrm, strct, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]))
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
				// Now a dropped object, site S0897. Also CXArgumentGet, phase 5 #327, site S1704.
				prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields = prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[:len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]
			}
		} else {
			if outIdx == -1 {
				panic("")
			}

			// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
			// Now an arena getter, site S0898. Also CXArgumentGet, phase 5 #327, site S1706.
			outPkg, err := prgrm.GetPackageFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Package)
			if err != nil {
				panic(err)
			}

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1707.
			argOutTypeSignature, err := lookupSymbol(prgrm, outPkg.Name, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Name, symbolsData)
			if err != nil {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1708.
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIdentifierMissing, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).ArgDetails, ast.CompileErrorOperands{int64(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Name)})
				prgrm.CXProgramCompileErrorExit()
			}
			// then we found an output
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1709.
			if len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields) > 0 {
				var argOut *ast.CXArgument
				if argOutTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0900.
					argOut = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argOutTypeSignature.Meta))
				} else {
					panic("type is cx argument deprecate\n\n")
				}

				strct := argOut.StructType
				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0901.
				strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0902. Also CXArgumentGet, phase 5 #327, site S1711.
				if fnIdx, err := strctPkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]).Name); err == nil {
					prgrm.CXAtomicOps[expr.Index].Operator = fnIdx
				} else {
					processMethodCallMissing(prgrm, strct, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]))
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0903.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)))
				prgrm.CXAtomicOps[expr.Index].Inputs.Fields = append([]ast.CXTypeSignatureIndex{typeSigIdx}, prgrm.CXAtomicOps[expr.Index].Inputs.Fields...)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
				// Now a dropped object, site S0905.
				prgrm.CXAtomicOps[expr.Index].Outputs.Fields = prgrm.CXAtomicOps[expr.Index].Outputs.Fields[1:]
				// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
				// Now a dropped object, site S0906. Also CXArgumentGet, phase 5 #327, site S1713.
				prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields = prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[:len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]
			}
		}
	} else {
		if outIdx == -1 {
			panic("")
		}

		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0907. Also CXArgumentGet, phase 5 #327, site S1715.
		outPkg, err := prgrm.GetPackageFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Package)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1716.
		argOutTypeSignature, err := lookupSymbol(prgrm, outPkg.Name, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Name, symbolsData)
		if err != nil {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1717.
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeIdentifierMissing, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).ArgDetails, ast.CompileErrorOperands{int64(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Name)})
			prgrm.CXProgramCompileErrorExit()
		}

		var argOut *ast.CXArgument
		if argOutTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0909.
			argOut = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argOutTypeSignature.Meta))

			// then we found an output
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1718.
			if len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields) > 0 {
				strct := argOut.StructType

				if strct == nil {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodePrimitiveFieldAccess, argOutTypeSignature.ArgDetails, ast.CompileErrorOperands{int64(argOutTypeSignature.Name), prgrm.CXProgramCompileErrorText(argOut.Type.Name())})
					prgrm.CXProgramCompileErrorExit()
				}

				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0910.
				strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0911. Also CXArgumentGet, phase 5 #327, site S1720.
				if fnIdx, err := strctPkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]).Name); err == nil {
					prgrm.CXAtomicOps[expr.Index].Operator = fnIdx
				} else {
					processMethodCallMissing(prgrm, strct, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]))
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0912.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)))
				// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
				// Now a heap literal, site S0914.
				newInputs := &ast.CXStruct{Fields: []ast.CXTypeSignatureIndex{typeSigIdx}}
				if prgrm.CXAtomicOps[expr.Index].Inputs != nil {
					for _, typeSig := range prgrm.CXAtomicOps[expr.Index].Inputs.Fields {
						newInputs.AddField_CXAtomicOps(prgrm, typeSig)
					}
				}
				prgrm.CXAtomicOps[expr.Index].Inputs = newInputs
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
				// Now a dropped object, site S0915.
				prgrm.CXAtomicOps[expr.Index].Outputs.Fields = prgrm.CXAtomicOps[expr.Index].Outputs.Fields[1:]
				// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
				// Now a dropped object, site S0916. Also CXArgumentGet, phase 5 #327, site S1722.
				prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields = prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[:len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]
			}
		} else if argOutTypeSignature.Type == ast.TYPE_ATOMIC || argOutTypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
			// TODO: improve
			// do nothing for now since len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields) > 0
			// of type atomics and pointers are always zero
		} else if argOutTypeSignature.Type == ast.TYPE_STRUCT || argOutTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
			// TODO: improve this ASAP
			// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
			// Now an arena getter, site S0917.
			structDetails := prgrm.GetCXTypeSignatureStructFromArray(argOutTypeSignature.Meta)

			// then we found an output
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1723.
			if len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields) > 0 {
				strct := structDetails.StructType

				if strct == nil {
					// TODO: add proper println for error
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodePrimitiveFieldAccess, argOutTypeSignature.ArgDetails, ast.CompileErrorOperands{int64(argOutTypeSignature.Name), prgrm.CXProgramCompileErrorText(argOutTypeSignature.GetTypeString(prgrm))})
					prgrm.CXProgramCompileErrorExit()
				}

				// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
				// Now an arena getter, site S0918.
				strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0919. Also CXArgumentGet, phase 5 #327, site S1725.
				if fnIdx, err := strctPkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]).Name); err == nil {
					prgrm.CXAtomicOps[expr.Index].Operator = fnIdx
				} else {
					processMethodCallMissing(prgrm, strct, prgrm.GetCXArgFromArray(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]))
				}

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0920.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)))
				// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
				// Now a heap literal, site S0922.
				newInputs := &ast.CXStruct{Fields: []ast.CXTypeSignatureIndex{typeSigIdx}}
				if prgrm.CXAtomicOps[expr.Index].Inputs != nil {
					for _, typeSig := range prgrm.CXAtomicOps[expr.Index].Inputs.Fields {
						newInputs.AddField_CXAtomicOps(prgrm, typeSig)
					}
				}
				prgrm.CXAtomicOps[expr.Index].Inputs = newInputs
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
				// Now a dropped object, site S0923.
				prgrm.CXAtomicOps[expr.Index].Outputs.Fields = prgrm.CXAtomicOps[expr.Index].Outputs.Fields[1:]
				// OBJMODEL: state.CXArgumentMng.CXArgumentRelease(gid), phase 4 #326.
				// Now a dropped object, site S0924. Also CXArgumentGet, phase 5 #327, site S1727.
				prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields = prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields[:len(prgrm.GetCXArg(ast.CXArgumentIndex(outIdx)).Fields)-1]
			}
		} else {
			panic("type is not known")
		}

	}
}

// processMethodCallMissing records that strct has no method named by field
// and exits, as the call cannot be built without it.
func processMethodCallMissing(prgrm *ast.CXProgram, strct *ast.CXStruct, field *ast.CXArgument) {
	prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMethodMissing, field.ArgDetails, ast.CompileErrorOperands{int64(strct.Name), int64(field.Name)})
	prgrm.CXProgramCompileErrorExit()
}

// GiveOffset
func GiveOffset(prgrm *ast.CXProgram, symbolsData *SymbolsData, typeSigIdx ast.CXTypeSignatureIndex) {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0925.
	symTypeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
	symTypeSigName := StripNameNumber(prgrm, symTypeSig.Name)
	if symTypeSigName != stringpool.StringGidNil {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0926.
		symPkg, err := prgrm.GetPackageFromArray(symTypeSig.Package)
		if err != nil {
			panic(err)
		}

		currFn, err := prgrm.GetCurrentFunction()
		if err != nil {
			// TODO: improve error handling
			panic("error getting current function")
		}

		if !currFn.IsLocalVariable(symTypeSigName) {
			GetGlobalSymbol(prgrm, symbolsData, symPkg, symTypeSigName)
		}

		argTypeSignature, err := lookupSymbol(prgrm, symPkg.Name, symTypeSigName, symbolsData)
		if err == nil {
			ProcessSymbolFields(prgrm, symTypeSig, argTypeSignature)
			CopyArgFields(prgrm, symTypeSig, argTypeSignature)
		}
	}
}

// ProcessTempVariable completes the temp variable expression.
func ProcessTempVariable(prgrm *ast.CXProgram, expr *ast.CXExpression) {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0927.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0928.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorInputs := expressionOperator.GetInputs(prgrm)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)

	opIdentFn := ast.Natives[constants.OP_IDENTITY]
	isOpIdent := false
	if expressionOperator != nil && expressionOperator.AtomicOPCode == opIdentFn.AtomicOPCode && len(expressionOperatorInputs) == len(opIdentFn.Inputs) && len(expressionOperatorOutputs) == len(opIdentFn.Outputs) {
		isOpIdent = true
	}

	if expressionOperator != nil && (isOpIdent || ast.IsArithmeticOperator(expressionOperator.AtomicOPCode)) && len(expression.GetOutputs(prgrm)) > 0 && len(expression.GetInputs(prgrm)) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0929.
		outputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		if IsTempVar(prgrm, outputTypeSig.Name) {
			if outputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				outputArgIdx := ast.CXArgumentIndex(outputTypeSig.Meta)

				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0930.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
				if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0931.
					expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))

					expressionInput := expressionInputArg
					// then it's a temporary variable and it needs to adopt its input's type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1728.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInput.Type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1729.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).PointerTargetType = expressionInput.PointerTargetType
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1730.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = expressionInput.Size
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1731.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).PreviouslyDeclared = true
				} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1732.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1733.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = types.Code(expressionInputTypeSig.Meta).Size()
				} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1734.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1735.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = types.Code(expressionInputTypeSig.Meta).Size()
				} else if expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0932.
					arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1736.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1737.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = types.Code(arrDetails.Meta).Size()
				} else if expressionInputTypeSig.Type == ast.TYPE_STRUCT || expressionInputTypeSig.Type == ast.TYPE_POINTER_STRUCT {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1738.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Type = expressionInputTypeSig.GetType(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1739.
					prgrm.GetCXArg(ast.CXArgumentIndex(outputArgIdx)).Size = expressionInputTypeSig.GetSize(prgrm, false)
				} else {
					panic("type is not known")
				}

			} else if outputTypeSig.Type == ast.TYPE_ATOMIC || outputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0933.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
				if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0934.
					expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
					outputTypeSig.Meta = int(expressionInputArg.Type)
				} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
					outputTypeSig.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					outputTypeSig.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0935.
					sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
					outputTypeSig.Meta = sliceDetails.Meta
				} else {
					panic("type is not known")
				}
			} else if outputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0936.
				outputArrDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputTypeSig.Meta)
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0937.
				expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])
				if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0938.
					expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
					outputArrDetails.Meta = int(expressionInputArg.Type)
				} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
					outputArrDetails.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					outputArrDetails.Meta = expressionInputTypeSig.Meta
				} else if expressionInputTypeSig.Type == ast.TYPE_ARRAY_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0939.
					arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(expressionInputTypeSig.Meta)
					outputArrDetails.Meta = arrDetails.Meta
				} else {
					panic("type is not known")
				}
			} else {
				panic("type is not known")
			}
		}
	}
}

// CopyArgFields copies 'arg' fields to 'sym' fields.
// copyArgFieldsIndexesCheck records indexes past the dimensions of the type
// and drops them, so later passes never index past its lengths.
func copyArgFieldsIndexesCheck(prgrm *ast.CXProgram, sym *ast.CXArgument, typeSig *ast.CXTypeSignature) {
	dimensions := 0
	switch typeSig.Type {
	case ast.TYPE_ATOMIC:
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		dimensions = len(prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta).Lengths)
	default:
		return
	}
	if len(sym.Indexes) <= dimensions {
		return
	}
	prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
	sym.Indexes = sym.Indexes[:dimensions]
}

func CopyArgFields(prgrm *ast.CXProgram, symTypeSignature, argTypeSignature *ast.CXTypeSignature) {
	var sym *ast.CXArgument
	if symTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0941.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(symTypeSignature.Meta))
	} else {
		sym = nil
	}

	var arg *ast.CXArgument
	if argTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0943.
		arg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argTypeSignature.Meta))
	} else {
		arg = nil
	}

	if sym != nil && arg == nil {
		copyArgFieldsIndexesCheck(prgrm, sym, argTypeSignature)
	}

	// TODO: check if this needs a change
	if sym == nil && arg == nil {
		if !(symTypeSignature.Type == ast.TYPE_SLICE_ATOMIC && argTypeSignature.Type == ast.TYPE_SLICE_ATOMIC) && !(symTypeSignature.Type == ast.TYPE_STRUCT && argTypeSignature.Type == ast.TYPE_STRUCT) && !(symTypeSignature.Type == ast.TYPE_POINTER_STRUCT && argTypeSignature.Type == ast.TYPE_POINTER_STRUCT) {
			// Only copy meta if sym and arg are not type slice atomic.
			symTypeSignature.Meta = argTypeSignature.Meta
		}

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Offset = argTypeSignature.Offset

		return
	} else if sym != nil && ast.IsTypePointerAtomic(sym) && argTypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = argTypeSignature.Meta
		symTypeSignature.Offset = argTypeSignature.Offset

		return

	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		sym.Name = argTypeSignature.Name
		sym.Package = argTypeSignature.Package
		sym.Type = types.Code(argTypeSignature.Meta)

		sym.Offset = argTypeSignature.Offset

		declSpec := []int{constants.DECL_BASIC, constants.DECL_POINTER}
		for _, spec := range sym.DeclarationSpecifiers {
			// checking if we need to remove or add cxcore.DECL_POINTERs
			// also we could be removing
			switch spec {
			case constants.DECL_INDEXING:
				prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
			case constants.DECL_DEREF:
				if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
					declSpec = declSpec[:len(declSpec)-1]
				} else {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
				}

			}
		}
		sym.DeclarationSpecifiers = declSpec

		return
	} else if sym != nil && arg == nil && (len(sym.DeclarationSpecifiers) > 0 || len(sym.DereferenceOperations) > 0) && argTypeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0944.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		return
	} else if sym != nil && arg == nil && (len(sym.DeclarationSpecifiers) > 0 || len(sym.DereferenceOperations) > 0) && argTypeSignature.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0947.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		for _, decl := range sym.DeclarationSpecifiers {
			if decl == constants.DECL_DEREF {
				symTypeSignature.IsDeref = true
			}
		}

		return
	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0950.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		for _, decl := range sym.DeclarationSpecifiers {
			switch decl {
			case constants.DECL_DEREF, constants.DEREF_SLICE, constants.DEREF_ARRAY:
				symTypeSignature.IsDeref = true
			}
		}

		for _, deref := range sym.DereferenceOperations {
			switch deref {
			case constants.DEREF_SLICE:
				symTypeSignature.IsDeref = true
			}
		}

		return
	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0953.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSignature.Meta)
		newArrDetails := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		newArrDetails.Meta = arrDetails.Meta
		newArrDetails.Lengths = arrDetails.Lengths
		newArrDetails.Indexes = sym.Indexes
		newArrDetailsIdx := int(objmodel.GidIndex(int64(arrayGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newArrDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		for _, decl := range sym.DeclarationSpecifiers {
			switch decl {
			case constants.DECL_DEREF, constants.DEREF_SLICE, constants.DEREF_ARRAY:
				symTypeSignature.IsDeref = true
			}
		}

		for _, deref := range sym.DereferenceOperations {
			switch deref {
			case constants.DEREF_SLICE:
				symTypeSignature.IsDeref = true
			}
		}

		return
	} else if sym != nil && arg == nil && argTypeSignature.Type == ast.TYPE_STRUCT || argTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		structGid := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructAlloc()
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0956.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSignature.Meta)
		newStructDetails := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructGet(structGid)
		newStructDetails.StructType = structDetails.StructType
		newStructDetails.Fields = sym.Fields
		newStructDetailsIdx := int(objmodel.GidIndex(int64(structGid)))

		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = newStructDetailsIdx
		symTypeSignature.Offset = argTypeSignature.Offset
		symTypeSignature.PassBy = sym.PassBy

		if argTypeSignature.Type == ast.TYPE_POINTER_STRUCT && len(sym.Fields) == 0 {
			for _, decl := range sym.DeclarationSpecifiers {
				if decl == constants.DECL_DEREF {
					symTypeSignature.IsDeref = true
				}
			}
		}

		if len(sym.Fields) > 0 {
			elt := sym.GetAssignmentElement(prgrm)

			declSpec := []int{}
			for c := 0; c < len(elt.DeclarationSpecifiers); c++ {
				switch elt.DeclarationSpecifiers[c] {
				case constants.DECL_INDEXING:
					if declSpec[len(declSpec)-1] == constants.DECL_ARRAY || declSpec[len(declSpec)-1] == constants.DECL_SLICE {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_DEREF:
					if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				default:
					declSpec = append(declSpec, elt.DeclarationSpecifiers[c])
				}
			}

			elt.DeclarationSpecifiers = declSpec
		}

		return
	} else if sym != nil && arg == nil && (len(sym.DeclarationSpecifiers) > 1 || len(sym.DereferenceOperations) > 1) {
		sym.Name = argTypeSignature.Name
		sym.Package = argTypeSignature.Package
		sym.Type = types.Code(argTypeSignature.Meta)
		sym.Offset = argTypeSignature.Offset

		return
	} else if sym != nil && arg == nil {
		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = argTypeSignature.Meta
		symTypeSignature.Offset = argTypeSignature.Offset
		return
	} else if sym == nil && arg != nil {
		symTypeSignature.Name = argTypeSignature.Name
		symTypeSignature.Package = argTypeSignature.Package
		symTypeSignature.Type = argTypeSignature.Type
		symTypeSignature.Meta = argTypeSignature.Meta
		symTypeSignature.Offset = argTypeSignature.Offset

		return
	}

	// This is for test-pointers when IsTypeAtomic includes types.IDENTIFIER
	// else if sym == nil && arg != nil && symTypeSignature.PassBy != 0 && arg.Type.IsPrimitive() {
	// 	symTypeSignature.Meta = int(arg.Type)
	// 	symTypeSignature.Offset = arg.Offset

	// 	return
	// }

	sym.Name = arg.Name
	sym.Offset = arg.Offset
	sym.Type = arg.Type

	_, symLine := ast.CXArgumentDebugFileLine(prgrm, sym.ArgDetails)
	_, argLine := ast.CXArgumentDebugFileLine(prgrm, arg.ArgDetails)
	if symLine != argLine {
		// FIXME Maybe we can unify this later.
		if len(sym.Fields) > 0 {
			elt := sym.GetAssignmentElement(prgrm)

			declSpec := []int{}
			for c := 0; c < len(elt.DeclarationSpecifiers); c++ {
				switch elt.DeclarationSpecifiers[c] {
				case constants.DECL_INDEXING:
					if declSpec[len(declSpec)-1] == constants.DECL_ARRAY || declSpec[len(declSpec)-1] == constants.DECL_SLICE {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_DEREF:
					if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				default:
					declSpec = append(declSpec, elt.DeclarationSpecifiers[c])
				}
			}

			elt.DeclarationSpecifiers = declSpec
		} else {
			declSpec := make([]int, len(arg.DeclarationSpecifiers))

			for i, spec := range arg.DeclarationSpecifiers {
				declSpec[i] = spec
			}

			for _, spec := range sym.DeclarationSpecifiers {
				// checking if we need to remove or add cxcore.DECL_POINTERs
				// also we could be removing
				switch spec {
				case constants.DECL_INDEXING:
					if declSpec[len(declSpec)-1] == constants.DECL_ARRAY || declSpec[len(declSpec)-1] == constants.DECL_SLICE {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndexing, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_DEREF:
					if declSpec[len(declSpec)-1] == constants.DECL_POINTER {
						declSpec = declSpec[:len(declSpec)-1]
					} else {
						prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeInvalidIndirection, sym.ArgDetails, ast.CompileErrorOperands{})
					}
				case constants.DECL_POINTER:
					if symLine != argLine {
						// This function is also called so it assigns offset and other fields to signature parameters
						//
						declSpec = append(declSpec, constants.DECL_POINTER)
					}
				}
			}

			sym.DeclarationSpecifiers = declSpec
		}
	} else {
		sym.DeclarationSpecifiers = arg.DeclarationSpecifiers
	}

	sym.StructType = arg.StructType

	// FIXME: In other processes like ProcessSymbolFields the symbol is assigned with lengths.
	// If we already have some lengths, we skip this. This needs to be fixed in the redesign of the cxgo.
	if len(sym.Lengths) == 0 {
		sym.Lengths = arg.Lengths
	}

	// sym.Lengths = arg.Lengths
	sym.Package = arg.Package
	sym.Size = arg.Size

	// Checking if it's a slice struct field. We'll do the same process as
	// below (as in the `arg.IsSlice` check), but the process differs in the
	// case of a slice struct field.
	assignElement := sym.GetAssignmentElement(prgrm)
	if (!arg.IsSlice() || hasDerefOp(sym, constants.DEREF_ARRAY)) && arg.StructType != nil && assignElement.IsSlice() && assignElement != sym {
		for i, deref := range assignElement.DereferenceOperations {
			// The cxgo when reading `foo[5]` in postfix.go does not know if `foo`
			// is a slice or an array. At this point we now know it's a slice and we need
			// to change those dereferences to cxcore.DEREF_SLICE.
			if deref == constants.DEREF_ARRAY {
				assignElement.DereferenceOperations[i] = constants.DEREF_SLICE
			}
		}

		if len(assignElement.DereferenceOperations) > 0 && assignElement.DereferenceOperations[0] == constants.DEREF_POINTER {
			assignElement.DereferenceOperations = assignElement.DereferenceOperations[1:]
		}
	}

	if arg.IsSlice() {
		if !hasDerefOp(sym, constants.DEREF_ARRAY) {
			// Then we're handling the slice itself, and we need to dereference it.
			sym.DereferenceOperations = append([]int{constants.DEREF_POINTER}, sym.DereferenceOperations...)
		} else {
			for i, deref := range sym.DereferenceOperations {
				// The cxgo when reading `foo[5]` in postfix.go does not know if `foo`
				// is a slice or an array. At this point we now know it's a slice and we need
				// to change those dereferences to cxcore.DEREF_SLICE.
				if deref == constants.DEREF_ARRAY {
					sym.DereferenceOperations[i] = constants.DEREF_SLICE
				}
			}
		}
	}

	if len(sym.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0959.
		symField := prgrm.GetCXArgFromArray(sym.Fields[len(sym.Fields)-1])
		if sym.Type == types.POINTER && arg.StructType != nil {
			sym.PointerTargetType = symField.Type
		} else {
			sym.Type = symField.Type
			sym.PointerTargetType = symField.PointerTargetType
		}
	} else {
		sym.Type = arg.Type
		sym.PointerTargetType = arg.PointerTargetType
	}
}

// ProcessSymbolFields copies the correct field values for the sym.Fields from their struct fields.
func ProcessSymbolFields(prgrm *ast.CXProgram, symTypeSignature, argTypeSignature *ast.CXTypeSignature) {
	var argStructType *ast.CXStruct
	if argTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0960.
		arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(argTypeSignature.Meta))
		argStructType = arg.StructType
	} else if argTypeSignature.Type == ast.TYPE_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0961.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSignature.Meta)
		argStructType = structDetails.StructType
	} else if argTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0962.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSignature.Meta)
		argStructType = structDetails.StructType
	} else {
		// panic("type is type cxargument deprecate\n\n")
		// ProcessSymbolFields is only needed for struct types
		// So we return from here
		return
	}

	// nil for a struct signature.
	var sym *ast.CXArgument
	var symFields []ast.CXArgumentIndex
	if symTypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0964.
		sym = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(symTypeSignature.Meta))
		symFields = sym.Fields
	} else if symTypeSignature.Type == ast.TYPE_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0965.
		symStructDetails := prgrm.GetCXTypeSignatureStructFromArray(symTypeSignature.Meta)
		symFields = symStructDetails.Fields
	} else if symTypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0966.
		symStructDetails := prgrm.GetCXTypeSignatureStructFromArray(symTypeSignature.Meta)
		symFields = symStructDetails.Fields
	} else {
		// panic("type is type cxargument deprecate\n\n")
		// ProcessSymbolFields is only needed for struct types
		// So we return from here
		return
	}

	if len(symFields) > 0 {
		if argStructType == nil || len(argStructType.Fields) == 0 {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeNoFields, symTypeSignature.ArgDetails, ast.CompileErrorOperands{int64(symTypeSignature.Name)})
			return
		}

		// checking if fields do exist in their StructType
		// and assigning that StructType to the sym.Field
		strct := argStructType
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0967.
		strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
		if err != nil {
			panic(err)
		}

		for _, fldIdx := range symFields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0968.
			field := prgrm.GetCXArgFromArray(fldIdx)
			if inFld, err := strct.GetField(prgrm, field.Name); err == nil {
				if inFld.GetStructType(prgrm) != nil {
					field.StructType = strct
					strct = inFld.GetStructType(prgrm)
				}
			} else {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0969.
				methodName := prgrm.GetCXArgFromArray(symFields[len(symFields)-1]).Name
				receiverType := strct.Name

				if methodIdx, methodErr := strctPkg.GetMethod(prgrm, receiverType, methodName); methodErr == nil {
					// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
					// Now an arena getter, site S0970.
					method := prgrm.GetFunctionFromArray(methodIdx)
					methodOutputs := method.GetOutputs(prgrm)
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S0971.
					methodOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(methodOutputs[0])

					var methodOutputArg *ast.CXArgument
					if methodOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S0973.
						methodOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(methodOutputTypeSig.Meta))

						field.Type = methodOutputArg.Type
						field.PointerTargetType = methodOutputArg.PointerTargetType
					} else {
						field.Type = methodOutputTypeSig.GetType(prgrm)
					}
				} else {
					prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeMessage, field.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
				}
			}
		}

		strct = argStructType
		// then we copy all the type struct fields
		// to the respective symFields
		for _, nameFldIdx := range symFields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0974.
			nameField := prgrm.GetCXArgFromArray(nameFldIdx)
			if nameField.StructType != nil {
				strct = nameField.StructType
			}

			for _, typeSignatureIdx := range strct.Fields {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0975.
				typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)

				if typeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					fieldIdx := typeSignature.Meta
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1740.
					field := prgrm.GetCXArg(ast.CXArgumentIndex(fieldIdx))

					if nameField.Name == field.Name {
						nameField.Type = field.Type
						nameField.Lengths = field.Lengths
						nameField.Size = field.Size
						nameField.PointerTargetType = field.PointerTargetType
						nameField.StructType = field.StructType

						if sym != nil {
							sym.Lengths = field.Lengths
						}

						if len(nameField.DeclarationSpecifiers) > 0 {
							nameField.DeclarationSpecifiers = append(field.DeclarationSpecifiers, nameField.DeclarationSpecifiers[1:]...)
						} else {
							nameField.DeclarationSpecifiers = field.DeclarationSpecifiers
						}

						if field.IsSlice() {
							nameField.DereferenceOperations = append([]int{constants.DEREF_POINTER}, nameField.DereferenceOperations...)
						}

						nameField.PassBy = field.PassBy

						if field.StructType != nil {
							strct = field.StructType
						}

						break
					}
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_ATOMIC {
					nameField.Type = types.Code(typeSignature.Meta)
					nameField.StructType = nil
					nameField.Size = typeSignature.GetSize(prgrm, false)

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_BASIC}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_BASIC}
					}

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_POINTER_ATOMIC {
					nameField.Type = types.Code(typeSignature.Meta)
					nameField.StructType = nil
					nameField.Size = types.Code(typeSignature.Meta).Size()

					nameField.DereferenceOperations = append([]int{constants.DECL_BASIC, constants.DEREF_POINTER}, nameField.DereferenceOperations...)

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0976.
					typeSignatureArray := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
					nameField.Type = types.Code(typeSignatureArray.Meta)
					nameField.StructType = nil
					nameField.Size = types.Code(typeSignatureArray.Meta).Size()
					nameField.Lengths = typeSignatureArray.Lengths
					if sym != nil {
						sym.Lengths = typeSignatureArray.Lengths
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_BASIC, constants.DECL_ARRAY}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_BASIC}
					}
					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_SLICE_ATOMIC {
					// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
					// Now an arena getter, site S0977.
					sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
					nameField.Type = types.Code(sliceDetails.Meta)
					nameField.StructType = nil
					nameField.Size = nameField.Type.Size()
					nameField.Lengths = sliceDetails.Lengths
					if sym != nil {
						sym.Lengths = sliceDetails.Lengths
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_BASIC, constants.DECL_SLICE}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_BASIC}
					}

					nameField.DereferenceOperations = append([]int{constants.DEREF_POINTER}, nameField.DereferenceOperations...)

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_STRUCT {
					// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
					// Now an arena getter, site S0978.
					structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
					nameField.Type = types.STRUCT
					nameField.Size = typeSignature.GetSize(prgrm, false)
					nameField.StructType = structDetails.StructType
					if nameField.StructType != nil {
						strct = nameField.StructType
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_STRUCT}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
					}

					break
				} else if nameField.Name == typeSignature.Name && typeSignature.Type == ast.TYPE_POINTER_STRUCT {
					// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
					// Now an arena getter, site S0979.
					structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
					nameField.Type = types.POINTER
					nameField.Size = typeSignature.GetSize(prgrm, false)
					nameField.StructType = structDetails.StructType
					if nameField.StructType != nil {
						strct = nameField.StructType
					}

					// TODO: this should not be needed.
					if len(nameField.DeclarationSpecifiers) > 0 {
						nameField.DeclarationSpecifiers = append([]int{constants.DECL_STRUCT, constants.DECL_POINTER}, nameField.DeclarationSpecifiers[1:]...)
					} else {
						nameField.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
					}

					break
				}

				nameField.Offset += typeSignature.Offset
			}
		}
	}
}

// GetGlobalSymbol tries to retrieve `ident` from `symPkg`'s globals if `ident` is not found in the local scope.
func GetGlobalSymbol(prgrm *ast.CXProgram, symbolsData *SymbolsData, symPkg *ast.CXPackage, ident stringpool.StringGid) {
	_, err := lookupSymbol(prgrm, symPkg.Name, ident, symbolsData)
	if err != nil {
		if glbl, err := symPkg.GetGlobal(prgrm, ident); err == nil {
			lastIdx := len(*&symbolsData.symbolsIndex) - 1

			// add name to symbols
			symbolsData.symbols = append(symbolsData.symbols, glbl.Index)
			(symbolsData.symbolsIndex)[lastIdx][SymbolsDataKey{SymbolsDataKeyPackage: symPkg.Name, SymbolsDataKeyName: ident}] = len(symbolsData.symbols) - 1
		}
	}
}

// exprLine returns the file and line of the CX_LINE marker before exprs[currIndex].
// It falls back to the parser position, which is the end of the function once the body is parsed.
func exprLine(prgrm *ast.CXProgram, exprs []ast.CXExpression, currIndex int) (ast.SourceFileGid, int) {
	if cxLine, err := prgrm.GetPreviousCXLine(exprs, currIndex); err == nil && cxLine.LineNumber > 0 {
		return cxLine.FileName, cxLine.LineNumber
	}
	return CurrentFile, LineNo
}

// releaseFunctionSymbols releases the signatures that lookupSymbol made for
// function names, with the argument each names and its debug record. Only
// the lookup that made one ever reads it.
func releaseFunctionSymbols(prgrm *ast.CXProgram, symbolsData *SymbolsData) {
	for _, typeSigIdx := range symbolsData.functionSymbols {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1777.
		typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
		argDetails := typeSig.ArgDetails
		prgrm.ReleaseCXArg(ast.CXArgumentIndex(typeSig.Meta))
		prgrm.ReleaseCXTypeSignature(typeSigIdx)
		prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugRelease(argDetails)
	}
	symbolsData.functionSymbols = nil
}
