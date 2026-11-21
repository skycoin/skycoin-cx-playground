package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin/src/cipher/encoder"
)

// ReturnExpressions stores the `Size` of the return arguments represented by `Expressions`.
// For example: `return foo() + bar()` is a set of 3 expressions and they represent a single return argument
type ReturnExpressions struct {
	Size int
	// OBJMODEL: []CXExpressionGid or CXExpressionGid, phase 5 #347.
	// Now objects held by value, site S0607.
	Expressions []ast.CXExpression
}

// IterationExpressions creates series of expressions that will create
// a for loop condition.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		initializeExprs - contains the initialization of the variables used
//						  for the condition of the iteration expression.
//	 conditionExprs - contains the condition of the iteration expression.
//	 incrementExprs - contains the increment expr for the variable used
//						 in the condition.
//	 statementExprs - contains the statements inside the iteration expression.
func IterationExpressions(prgrm *ast.CXProgram,
	initializeExprs []ast.CXExpression,
	conditionExprs []ast.CXExpression,
	incrementExprs []ast.CXExpression,
	statementExprs []ast.CXExpression) []ast.CXExpression {
	jmpFn := ast.Natives[constants.OP_JMP]

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0608.
	upExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0609.
	upExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	upExpressionIdx := upExpr.Index

	prgrm.CXAtomicOps[upExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	// -2 for the cx line expression addition for up and down expr
	upLines := ((len(statementExprs) + len(incrementExprs) + len(conditionExprs) + 2) * -1) - 2
	downLines := 0

	trueArg := WritePrimary(prgrm, types.BOOL, encoder.Serialize(true), false)
	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, trueArg)
	prgrm.CXAtomicOps[upExpressionIdx].AddInput(prgrm, typeSigIdx)
	prgrm.CXAtomicOps[upExpressionIdx].ThenLines = upLines
	prgrm.CXAtomicOps[upExpressionIdx].ElseLines = downLines

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0611.
	downExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0612.
	downExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	downExpressionIdx := downExpr.Index

	prgrm.CXAtomicOps[downExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	lastCondExpressionIdx := conditionExprs[len(conditionExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0613.
	lastCondExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastCondExpressionIdx].Operator)
	lastCondExpressionOperatorOutputs := lastCondExpressionOperator.GetOutputs(prgrm)
	if len(prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)) < 1 {
		if len(lastCondExpressionOperatorOutputs) == 0 {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeLoopConditionNoValue, CurrentFile, LineNo, ast.CompileErrorOperands{})
			prgrm.CXProgramCompileErrorExit()
		}
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0614.
		lastCondExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastCondExpressionOperatorOutputs[0])

		var lastCondExpressionOperatorOutputType types.Code
		var typeSigIdx ast.CXTypeSignatureIndex
		if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0615.
			lastCondExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastCondExpressionOperatorOutputTypeSig.Meta))
			lastCondExpressionOperatorOutputType = lastCondExpressionOperatorOutputArg.Type
			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0616.
			_, predicate := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			predicate.SetType(lastCondExpressionOperatorOutputType)
			predicate.Package = ast.CXPackageIndex(pkg.Index)
			predicate.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, predicate)
		} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(lastCondExpressionOperatorOutputTypeSig)
			newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
			newTypeSig.Offset = types.Pointer(0)
			typeSigIdx = newTypeSig.Index
		} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(lastCondExpressionOperatorOutputTypeSig)
			newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
			newTypeSig.Offset = types.Pointer(0)
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[lastCondExpressionIdx].AddOutput(prgrm, typeSigIdx)
		prgrm.CXAtomicOps[downExpressionIdx].AddInput(prgrm, typeSigIdx)
	} else {
		predicateTypeSigIdx := prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)[0]
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0624.
		predicateTypeSig := prgrm.GetCXTypeSignatureFromArray(predicateTypeSigIdx)
		var predicateIdx ast.CXArgumentIndex
		if predicateTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			predicateIdx = ast.CXArgumentIndex(predicateTypeSig.Meta)
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1675.
			prgrm.GetCXArg(ast.CXArgumentIndex(predicateIdx)).Package = ast.CXPackageIndex(pkg.Index)
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1676.
			prgrm.GetCXArg(ast.CXArgumentIndex(predicateIdx)).PreviouslyDeclared = true
		} else if predicateTypeSig.Type == ast.TYPE_ATOMIC || predicateTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// do nothing
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[downExpressionIdx].AddInput(prgrm, predicateTypeSigIdx)
	}

	thenLines := 0
	// + 1 for the cx line expression addition for the down expr jmp
	elseLines := len(incrementExprs) + len(statementExprs) + 1 + 1

	// processing possible breaks
	for i, statementExpr := range statementExprs {
		if statementExpr.IsBreak(prgrm) {
			statementExpressionIdx := statementExpr.Index
			prgrm.CXAtomicOps[statementExpressionIdx].ThenLines = elseLines - i - 1
		}
	}

	// processing possible continues
	for i, statementExpr := range statementExprs {
		if statementExpr.IsContinue(prgrm) {
			statementExpressionIdx := statementExpr.Index
			prgrm.CXAtomicOps[statementExpressionIdx].ThenLines = len(statementExprs) - i - 1
		}
	}

	prgrm.CXAtomicOps[downExpressionIdx].ThenLines = thenLines
	prgrm.CXAtomicOps[downExpressionIdx].ElseLines = elseLines

	exprs := initializeExprs
	exprs = append(exprs, conditionExprs...)
	exprs = append(exprs, *downExprCXLine, *downExpr)
	exprs = append(exprs, statementExprs...)
	exprs = append(exprs, incrementExprs...)
	exprs = append(exprs, *upExprCXLine, *upExpr)

	DefineNewScope(exprs)

	return exprs
}

// trueJmpExpressions makes a true jump expression for BREAKS and CONTINUES.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		opcode - the opcode of the expression. Currently used as either a
//	 		 BREAK or CONTINUE expression.
func trueJmpExpressions(prgrm *ast.CXProgram, opcode int) []ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0625.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0626.
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[opcode])
	expressionIdx := expr.Index

	trueArg := WritePrimary(prgrm, types.BOOL, encoder.Serialize(true), false)
	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, trueArg)
	prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S0628.
	return []ast.CXExpression{*exprCXLine, *expr}
}

// BreakExpressions makes a true jump expression for BREAKS.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
func BreakExpressions(prgrm *ast.CXProgram) []ast.CXExpression {
	exprs := trueJmpExpressions(prgrm, constants.OP_BREAK)
	return exprs
}

// ContinueExpressions makes a true jump expression for CONTINUES.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
func ContinueExpressions(prgrm *ast.CXProgram) []ast.CXExpression {
	exprs := trueJmpExpressions(prgrm, constants.OP_CONTINUE)
	return exprs
}

// SelectionExpressions creates series of expressions that will create
// an if else condition.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 conditionExprs - contains the condition of the selection expression.
//		thenExprs - contains the statements if condition is true.
//	 elseExprs - contains the statements if condition is false.
func SelectionExpressions(prgrm *ast.CXProgram, conditionExprs []ast.CXExpression, thenExprs []ast.CXExpression, elseExprs []ast.CXExpression) []ast.CXExpression {
	DefineNewScope(thenExprs)
	DefineNewScope(elseExprs)

	jmpFn := ast.Natives[constants.OP_JMP]
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0629.
	ifExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0630.
	ifExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	ifExpressionIdx := ifExpr.Index
	prgrm.CXAtomicOps[ifExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	lastCondExpressionIdx := conditionExprs[len(conditionExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0631.
	lastCondExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastCondExpressionIdx].Operator)

	var predicateTypeSigIdx ast.CXTypeSignatureIndex
	if lastCondExpressionOperator == nil && !conditionExprs[len(conditionExprs)-1].IsMethodCall() {
		// then it's a literal
		predicateTypeSigIdx = prgrm.CXAtomicOps[lastCondExpressionIdx].GetOutputs(prgrm)[0]
	} else {
		// then it's an expression
		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0632.
		_, predicate := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
		if conditionExprs[len(conditionExprs)-1].IsMethodCall() {
			// we'll change this once we have access to method's types in
			// ProcessMethodCall
			predicate.SetType(types.BOOL)
			prgrm.CXAtomicOps[lastCondExpressionIdx].Inputs.Fields = append(prgrm.CXAtomicOps[lastCondExpressionIdx].Outputs.Fields, prgrm.CXAtomicOps[lastCondExpressionIdx].Inputs.Fields...)
			prgrm.CXAtomicOps[lastCondExpressionIdx].Outputs = nil
		} else {
			lastCondExpressionOperatorOutputs := lastCondExpressionOperator.GetOutputs(prgrm)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0633.
			lastCondExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastCondExpressionOperatorOutputs[0])

			var lastCondExpressionOperatorOutputArgType types.Code
			if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0634.
				lastCondExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastCondExpressionOperatorOutputTypeSig.Meta))
				lastCondExpressionOperatorOutputArgType = lastCondExpressionOperatorOutputArg.Type
			} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				lastCondExpressionOperatorOutputArgType = types.Code(lastCondExpressionOperatorOutputTypeSig.Meta)
			} else if lastCondExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				lastCondExpressionOperatorOutputArgType = types.Code(lastCondExpressionOperatorOutputTypeSig.Meta)
			} else {
				panic("type is not known")
			}
			predicate.SetType(lastCondExpressionOperatorOutputArgType)
		}
		predicate.PreviouslyDeclared = true
		predicate.Package = ast.CXPackageIndex(pkg.Index)

		predicateTypeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, predicate)
		prgrm.CXAtomicOps[lastCondExpressionIdx].AddOutput(prgrm, predicateTypeSigIdx)
	}
	prgrm.CXAtomicOps[ifExpressionIdx].AddInput(prgrm, predicateTypeSigIdx)

	thenLines := 0
	// + 1 for cx line expression addition
	elseLines := len(thenExprs) + 1 + 1

	prgrm.CXAtomicOps[ifExpressionIdx].ThenLines = thenLines
	prgrm.CXAtomicOps[ifExpressionIdx].ElseLines = elseLines

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0636.
	skipExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0637.
	skipExpr := ast.MakeAtomicOperatorExpression(prgrm, jmpFn)
	skipExpressionIdx := skipExpr.Index

	prgrm.CXAtomicOps[skipExpressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	skipLines := len(elseExprs)

	trueArg := WritePrimary(prgrm, types.BOOL, encoder.Serialize(true), false)
	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, trueArg)
	prgrm.CXAtomicOps[skipExpressionIdx].AddInput(prgrm, typeSigIdx)
	prgrm.CXAtomicOps[skipExpressionIdx].ThenLines = skipLines
	prgrm.CXAtomicOps[skipExpressionIdx].ElseLines = 0

	var exprs []ast.CXExpression
	if lastCondExpressionOperator != nil || conditionExprs[len(conditionExprs)-1].IsMethodCall() {
		exprs = append(exprs, conditionExprs...)
	}

	exprs = append(exprs, *ifExprCXLine, *ifExpr)
	exprs = append(exprs, thenExprs...)
	exprs = append(exprs, *skipExprCXLine, *skipExpr)
	exprs = append(exprs, elseExprs...)

	return exprs
}

// resolveTypeForUnd tries to determine the type that will be returned from an expression
func resolveTypeForUnd(prgrm *ast.CXProgram, expr *ast.CXExpression) types.Code {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0639.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0640.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

	expressionInputs := expression.GetInputs(prgrm)
	expressionOutputs := expression.GetOutputs(prgrm)
	if len(expressionInputs) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0641.
		expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionInputs[0])

		var expressionInputArgType types.Code
		if expressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0642.
			expressionInputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionInputTypeSig.Meta))
			expressionInputArgType = expressionInputArg.Type
		} else if expressionInputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionInputArgType = types.Code(expressionInputTypeSig.Meta)
		} else if expressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionInputArgType = types.Code(expressionInputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		// it's a literal
		return expressionInputArgType
	}

	if len(expressionOutputs) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0643.
		expressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOutputs[0])
		var expressionOutputArgType types.Code
		if expressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0644.
			expressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOutputTypeSig.Meta))
			expressionOutputArgType = expressionOutputArg.Type
		} else if expressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionOutputArgType = types.Code(expressionOutputTypeSig.Meta)
		} else if expressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionOutputArgType = types.Code(expressionOutputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		// it's an expression with an output
		return expressionOutputArgType
	}

	if expressionOperator == nil {
		// the expression doesn't return anything
		return -1
	}
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)
	if len(expressionOperatorOutputs) > 0 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0645.
		expressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorOutputs[0])

		var expressionOperatorOutputArgType types.Code
		if expressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0646.
			expressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOperatorOutputTypeSig.Meta))
			expressionOperatorOutputArgType = expressionOperatorOutputArg.Type
		} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			expressionOperatorOutputArgType = types.Code(expressionOperatorOutputTypeSig.Meta)
		} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			expressionOperatorOutputArgType = types.Code(expressionOperatorOutputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		// always return first output's type
		return expressionOperatorOutputArgType
	}

	// error
	return -1
}

// IsTempVar ...
// TODO: Delete this function; only called by next function
func IsTempVar(prgrm *ast.CXProgram, nameGid stringpool.StringGid) bool {
	name := prgrm.CXProgramNameText(nameGid)
	if len(name) >= len(constants.LOCAL_PREFIX) && name[:len(constants.LOCAL_PREFIX)] == constants.LOCAL_PREFIX {
		return true
	}
	return false
}

// OperatorExpression creates an expression for an expression OP expression instance.
// i.e. ((expr+expr) * (expr-expr))
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 leftExprs - exprs on the left side of the operator expression
//		rightExprs - exprs on the right side of the operator expression
//		opcode - the opcode of the operator.
func OperatorExpression(prgrm *ast.CXProgram, leftExprs []ast.CXExpression, rightExprs []ast.CXExpression, opcode int) (out []ast.CXExpression) {
	// An empty side failed to compile and its error is recorded.
	if len(leftExprs) == 0 || len(rightExprs) == 0 {
		return nil
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	lastLeftExpressionIdx := leftExprs[len(leftExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0647.
	lastLeftExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastLeftExpressionIdx].Operator)
	lastLeftExpressionOperatorOutputs := lastLeftExpressionOperator.GetOutputs(prgrm)
	if len(prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)) < 1 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0648.
		lastLeftExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastLeftExpressionOperatorOutputs[0])

		var typeSigIdx ast.CXTypeSignatureIndex
		if lastLeftExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0649.
			lastLeftExpressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastLeftExpressionOperatorOutputTypeSig.Meta))

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0650.
			_, out := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			out.SetType(resolveTypeForUnd(prgrm, &leftExprs[len(leftExprs)-1]))
			out.Size = lastLeftExpressionOperatorOutputArg.Size
			out.Type = lastLeftExpressionOperatorOutputArg.Type
			out.PointerTargetType = lastLeftExpressionOperatorOutputArg.PointerTargetType
			out.Package = ast.CXPackageIndex(pkg.Index)
			out.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
		} else if lastLeftExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
			argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
			argDetails.FileName = CurrentFile
			argDetails.FileLine = LineNo
			_, typeSig := prgrm.AllocCXTypeSignature()
			typeSig.ArgDetails = argDetailsGid
			typeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			typeSig.Package = lastLeftExpressionOperatorOutputTypeSig.Package
			typeSig.Type = ast.TYPE_ATOMIC
			typeSig.Meta = lastLeftExpressionOperatorOutputTypeSig.Meta
			typeSig.Offset = lastLeftExpressionOperatorOutputTypeSig.Offset

			typeSigIdx = typeSig.Index
		} else if lastLeftExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
			argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
			argDetails.FileName = CurrentFile
			argDetails.FileLine = LineNo
			_, typeSig := prgrm.AllocCXTypeSignature()
			typeSig.ArgDetails = argDetailsGid
			typeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			typeSig.Package = lastLeftExpressionOperatorOutputTypeSig.Package
			typeSig.Type = ast.TYPE_POINTER_ATOMIC
			typeSig.Meta = lastLeftExpressionOperatorOutputTypeSig.Meta
			typeSig.Offset = lastLeftExpressionOperatorOutputTypeSig.Offset

			typeSigIdx = typeSig.Index
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[lastLeftExpressionIdx].AddOutput(prgrm, typeSigIdx)
	}

	lastRightExpressionIdx := rightExprs[len(rightExprs)-1].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0658.
	lastRightExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[lastRightExpressionIdx].Operator)
	lastRightExpressionOperatorOutputs := lastRightExpressionOperator.GetOutputs(prgrm)
	if len(prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)) < 1 {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0659.
		lastRightExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastRightExpressionOperatorOutputs[0])
		var lastRightExpressionOperatorOutputArg *ast.CXArgument

		var typeSigIdx ast.CXTypeSignatureIndex
		if lastRightExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0661.
			lastRightExpressionOperatorOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastRightExpressionOperatorOutputTypeSig.Meta))

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0662.
			_, out := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			out.SetType(resolveTypeForUnd(prgrm, &rightExprs[len(rightExprs)-1]))
			out.Size = lastRightExpressionOperatorOutputArg.Size
			out.Type = lastRightExpressionOperatorOutputArg.Type
			out.PointerTargetType = lastRightExpressionOperatorOutputArg.PointerTargetType
			out.Package = ast.CXPackageIndex(pkg.Index)
			out.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
		} else if lastRightExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
			argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
			argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
			argDetails.FileName = CurrentFile
			argDetails.FileLine = LineNo
			_, typeSig := prgrm.AllocCXTypeSignature()
			typeSig.ArgDetails = argDetailsGid
			typeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			typeSig.Package = lastRightExpressionOperatorOutputTypeSig.Package
			typeSig.Type = ast.TYPE_ATOMIC
			typeSig.Meta = lastRightExpressionOperatorOutputTypeSig.Meta
			typeSig.Offset = lastRightExpressionOperatorOutputTypeSig.Offset

			typeSigIdx = typeSig.Index
		} else if lastRightExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
			argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
			argDetails.FileName = CurrentFile
			argDetails.FileLine = LineNo
			_, typeSig := prgrm.AllocCXTypeSignature()
			typeSig.ArgDetails = argDetailsGid
			typeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			typeSig.Package = lastRightExpressionOperatorOutputTypeSig.Package
			typeSig.Type = ast.TYPE_POINTER_ATOMIC
			typeSig.Meta = lastRightExpressionOperatorOutputTypeSig.Meta
			typeSig.Offset = lastRightExpressionOperatorOutputTypeSig.Offset

			typeSigIdx = typeSig.Index
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[lastRightExpressionIdx].AddOutput(prgrm, typeSigIdx)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0670.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0671.
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[opcode])
	expressionIdx := expr.Index

	// we can't know the type until we compile the full function
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0672.
	lastLeftExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)[0])
	// nil for an atomic output.
	var lastLeftExpressionOutputArg *ast.CXArgument
	if lastLeftExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0674.
		lastLeftExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastLeftExpressionOutputTypeSig.Meta))
	} else if lastLeftExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || lastLeftExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		// An atomic output has no indexes.
	} else {
		panic("type is not known")
	}

	if lastLeftExpressionOutputArg != nil && len(lastLeftExpressionOutputArg.Indexes) > 0 || lastLeftExpressionOperator != nil {
		typeSigIdx := prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)[0]
		// then it's a function call or an array access
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)

		if IsTempVar(prgrm, lastLeftExpressionOutputTypeSig.Name) {
			out = append(out, leftExprs...)
		} else {
			out = append(out, leftExprs[:len(leftExprs)-1]...)
		}
	} else {
		typeSigIdx := prgrm.CXAtomicOps[lastLeftExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0676.
	lastRightExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0])
	// nil for an atomic output.
	var lastRightExpressionOutputArg *ast.CXArgument
	if lastRightExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0678.
		lastRightExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastRightExpressionOutputTypeSig.Meta))
	} else if lastRightExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || lastRightExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		// An atomic output has no indexes.
	} else {
		panic("type is not known")
	}

	if lastRightExpressionOutputArg != nil && len(lastRightExpressionOutputArg.Indexes) > 0 || lastRightExpressionOperator != nil {
		typeSigIdx := prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0]
		// then it's a function call or an array access
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)

		if IsTempVar(prgrm, lastRightExpressionOutputTypeSig.Name) {
			out = append(out, rightExprs...)
		} else {
			out = append(out, rightExprs[:len(rightExprs)-1]...)
		}
	} else {
		typeSigIdx := prgrm.CXAtomicOps[lastRightExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
	}

	out = append(out, *exprCXLine, *expr)

	return
}

// UnaryExpression creates an expression for unary operations,
// operations that only need one input, '*', '&', '!', and '-'.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 op - the unary operator, '*', '&', '!', and '-'.
//	 prevExprs - the array of expressions the unary expression belongs.
func UnaryExpression(prgrm *ast.CXProgram, op string, prevExprs []ast.CXExpression) []ast.CXExpression {
	lastPrevExpression, err := prgrm.GetCXAtomicOpFromExpressions(prevExprs, len(prevExprs)-1)
	if err != nil {
		panic(err)
	}

	if len(lastPrevExpression.GetOutputs(prgrm)) == 0 {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeInvalidIndirection, CurrentFile, LineNo, ast.CompileErrorOperands{})
		// needs to be stopped immediately
		prgrm.CXProgramCompileErrorExit()
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0680.
	lastPrevExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastPrevExpression.GetOutputs(prgrm)[0])
	var lastPrevExpressionOutputArg *ast.CXArgument
	if lastPrevExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0682.
		lastPrevExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastPrevExpressionOutputTypeSig.Meta))

		// Some properties need to be read from the base argument
		// due to how we calculate dereferences at the moment.
		lastPrevExpressionOutput := lastPrevExpressionOutputArg
		baseOut := lastPrevExpressionOutput
		exprOut := lastPrevExpressionOutput.GetAssignmentElement(prgrm)
		switch op {
		case "*":
			exprOut.DereferenceOperations = append(exprOut.DereferenceOperations, constants.DEREF_POINTER)
			exprOut.DeclarationSpecifiers = append(exprOut.DeclarationSpecifiers, constants.DECL_DEREF)
		case "&":
			baseOut.PassBy = constants.PASSBY_REFERENCE

			exprOut.DeclarationSpecifiers = append(exprOut.DeclarationSpecifiers, constants.DECL_POINTER)
		case "!":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0683.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BOOL_NOT])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0684.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)

				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0685.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArgFromArray(ast.CXArgumentIndex(exprOut.Index)))
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0687.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		case "-":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0688.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_NEG])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0689.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0690.
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, prgrm.GetCXArgFromArray(ast.CXArgumentIndex(exprOut.Index)))
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0692.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		}
	} else if lastPrevExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
		switch op {
		case "*":
			// TODO: what is the most efficient alternative for this
			// exprOut.DereferenceOperations = append(exprOut.DereferenceOperations, constants.DEREF_POINTER)
			// exprOut.DeclarationSpecifiers = append(exprOut.DeclarationSpecifiers, constants.DECL_DEREF)

			// a pointer type that we want to get its value
			lastPrevExpressionOutputTypeSig.IsDeref = true
		case "&":
			lastPrevExpressionOutputTypeSig.PassBy = constants.PASSBY_REFERENCE
		case "!":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0693.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BOOL_NOT])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0694.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)

				typeSigIdx := lastPrevExpressionOutputTypeSig.Index
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0695.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		case "-":
			if pkg, err := prgrm.GetCurrentPackage(); err == nil {
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now a constructor call, site S0696.
				expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_NEG])
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0697.
				expression, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.Package = ast.CXPackageIndex(pkg.Index)

				typeSigIdx := lastPrevExpressionOutputTypeSig.Index
				expression.AddInput(prgrm, typeSigIdx)
				// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
				// Now an object copy, site S0698.
				prevExprs[len(prevExprs)-1] = *expr
			} else {
				panic(err)
			}
		}

	} else {
		panic("type is not cx argument deprecate\n\n")
	}

	return prevExprs
}

// AssociateReturnExpressions associate the output of `retExprs` to the
// `idx`th output parameter of the current function.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 idx - referring to the nth output parameter of the current function.
//		retExprs - the return expressions to be associated or linked to the
//				   output parameter of the current function.
func AssociateReturnExpressions(prgrm *ast.CXProgram, idx int, retExprs []ast.CXExpression) []ast.CXExpression {
	var pkg *ast.CXPackage
	var fn *ast.CXFunction
	var err error

	pkg, err = prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	fn, err = pkg.GetCurrentFunction(prgrm)
	if err != nil {
		panic(err)
	}

	fnOutputs := fn.GetOutputs(prgrm)
	// AddJmpToReturnExpressions reports a return count that does not match.
	if idx >= len(fnOutputs) || len(retExprs) == 0 {
		return retExprs
	}
	lastExpr := retExprs[len(retExprs)-1]

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0699.
	fnOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(fnOutputs[idx])
	var outParam *ast.CXArgument
	var typeSigIdx ast.CXTypeSignatureIndex
	if fnOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0701.
		outParam = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fnOutputTypeSig.Meta))

		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0702.
		_, out := ast.MakeArgument(prgrm, outParam.Name, CurrentFile, LineNo)
		out.SetType(outParam.Type)
		out.StructType = outParam.StructType
		out.PreviouslyDeclared = true

		typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
	} else if fnOutputTypeSig.Type == ast.TYPE_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		_, newCXTypeSig := prgrm.AllocCXTypeSignature()
		newCXTypeSig.ArgDetails = argDetailsGid
		newCXTypeSig.Name = fnOutputTypeSig.Name
		newCXTypeSig.Package = fnOutputTypeSig.Package
		newCXTypeSig.Type = ast.TYPE_ATOMIC
		newCXTypeSig.Meta = int(fnOutputTypeSig.Type)
		newCXTypeSig.Offset = fnOutputTypeSig.Offset
		typeSigIdx = newCXTypeSig.Index
	} else if fnOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		_, newCXTypeSig := prgrm.AllocCXTypeSignature()
		newCXTypeSig.ArgDetails = argDetailsGid
		newCXTypeSig.Name = fnOutputTypeSig.Name
		newCXTypeSig.Package = fnOutputTypeSig.Package
		newCXTypeSig.Type = ast.TYPE_POINTER_ATOMIC
		newCXTypeSig.Meta = int(fnOutputTypeSig.Type)
		newCXTypeSig.Offset = fnOutputTypeSig.Offset
		typeSigIdx = newCXTypeSig.Index
	} else if fnOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
		argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
		argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
		argDetails.FileName = CurrentFile
		argDetails.FileLine = LineNo
		_, newCXTypeSig := prgrm.AllocCXTypeSignature()
		newCXTypeSig.ArgDetails = argDetailsGid
		newCXTypeSig.Name = fnOutputTypeSig.Name
		newCXTypeSig.Package = fnOutputTypeSig.Package
		newCXTypeSig.Type = ast.TYPE_SLICE_ATOMIC
		newCXTypeSig.Meta = fnOutputTypeSig.Meta
		newCXTypeSig.Offset = fnOutputTypeSig.Offset
		typeSigIdx = newCXTypeSig.Index
	} else {
		panic("type is not known")
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0713.
	lastExpression, err := prgrm.GetCXAtomicOp(lastExpr.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0714.
	lastExpressionOperator := prgrm.GetFunctionFromArray(lastExpression.Operator)
	if lastExpressionOperator == nil {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S0715.
		opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
		lastExpression.Operator = opIdx

		lastExpression.Inputs = lastExpression.Outputs
		lastExpression.Outputs = nil

		lastExpression.AddOutput(prgrm, typeSigIdx)
		return retExprs
	} else if len(lastExpression.GetOutputs(prgrm)) > 0 {
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0716.
		exprCXLine := MakeCXLineExpressionCurrent(prgrm)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0717.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0718.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		exprOutTypeSig := lastExpression.GetOutputs(prgrm)[0]
		expression.AddInput(prgrm, exprOutTypeSig)
		expression.AddOutput(prgrm, typeSigIdx)

		return append(retExprs, *exprCXLine, *expr)
	} else {
		lastExpression.AddOutput(prgrm, typeSigIdx)
		return retExprs
	}
}

// AddJmpToReturnExpressions adds an jump expression that makes a function stop its execution.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 exprs - contains the array of return expressions where the jump expr will be added.
func AddJmpToReturnExpressions(prgrm *ast.CXProgram, exprs ReturnExpressions) []ast.CXExpression {
	var pkg *ast.CXPackage
	var fn *ast.CXFunction
	var err error

	pkg, err = prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	fn, err = pkg.GetCurrentFunction(prgrm)
	if err != nil {
		panic(err)
	}

	retExprs := exprs.Expressions
	fnOutputs := fn.GetOutputs(prgrm)
	if len(fnOutputs) != exprs.Size && exprs.Expressions != nil {
		// lastExpr := retExprs[len(retExprs)-1]
		file, line := CurrentFile, LineNo
		if lastExprCXLine, err := prgrm.GetPreviousCXLine(retExprs, len(retExprs)-1); err == nil {
			file, line = lastExprCXLine.FileName, lastExprCXLine.LineNumber
		}
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeFunctionReturnCount, file, line, ast.CompileErrorOperands{int64(fn.Name), int64(len(fnOutputs)), int64(exprs.Size)})
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0719.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// expression to jump to the end of the embedding function
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0720.
	expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_GOTO])
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0721.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	// simulating a label so it gets executed without evaluating a predicate
	expression.Label = generateTempVarName(prgrm, constants.LABEL_PREFIX)
	expression.ThenLines = types.MAX_INT32
	expression.Package = ast.CXPackageIndex(pkg.Index)

	retExprs = append(retExprs, *exprCXLine, *expr)

	return retExprs
}
