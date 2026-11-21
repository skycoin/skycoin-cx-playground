package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// assignStructLiteralFields converts a struct literal to a series of struct field assignments.
// For example, `foo = Item{x: 10, y: 20}` is converted to: `foo.x = 10; foo.y = 20;`.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs - toExprs are the array of expressions that contains the data needed
//			  to construct the series of struct field assignments.
//	fromExprs - fromExprs are the array of expressions that will contain the
//			    series of struct field assignments.
//	structLiteralName - name of the struct, in the example above this is "foo".
func assignStructLiteralFields(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, structLiteralName stringpool.StringGid) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	for _, exprInfo := range fromExprs {
		if exprInfo.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0512.
		expression, err := prgrm.GetCXAtomicOp(exprInfo.Index)
		if err != nil {
			panic(err)
		}

		expressionOutputTypeSigIdx := expression.GetOutputs(prgrm)[0]
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0513.
		expressionOutput := prgrm.GetCXTypeSignatureFromArray(expressionOutputTypeSigIdx)

		if expressionOutput.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			expressionOutputIdx := ast.CXArgumentIndex(expressionOutput.Meta)

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1634.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Name = structLiteralName
			expressionOutput.Name = structLiteralName

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0514.
			toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
			// nil for an atomic output.
			var toExpressionOutput *ast.CXArgument
			if toExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0516.
				toExpressionOutput = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(toExpressionOutputTypeSig.Meta))
			} else if toExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || toExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// TODO: Improve when possible
				// Do nothing for now since len(toExpressionOutput.Indexes) > 0
				// of type atomics and type pointer atomics are always 0
			} else {
				panic("type is not known")
			}

			if toExpressionOutput != nil && len(toExpressionOutput.Indexes) > 0 {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1635.
				prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Lengths = toExpressionOutput.Lengths
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1636.
				prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Indexes = toExpressionOutput.Indexes
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1637.
				prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations, constants.DEREF_ARRAY)
			}

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1638.
			prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations, constants.DEREF_FIELD)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}
	}

	return fromExprs
}

// StructLiteralAssignment handles struct literals, e.g. `Item{x: 10, y: 20}`, and references to
// struct literals, e.g. `&Item{x: 10, y: 20}` in assignment expressions.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs - toExprs are the array of expressions that contains the data needed
//			  to construct the series of struct field assignments.
//	fromExprs - fromExprs are the array of expressions that will contain the
//				series of struct field assignments.
func StructLiteralAssignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression) []ast.CXExpression {
	lastFromExpr := fromExprs[len(fromExprs)-1]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0517.
	lastFromExpression, err := prgrm.GetCXAtomicOp(lastFromExpr.Index)
	if err != nil {
		panic(err)
	}

	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	lastFromCXLine, _ := prgrm.GetPreviousCXLine(fromExprs, len(fromExprs)-1)

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0518.
	lastFromExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(lastFromExpression.GetOutputs(prgrm)[0])
	var lastFromExpressionOutputArg *ast.CXArgument
	if lastFromExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0520.
		lastFromExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(lastFromExpressionOutputTypeSig.Meta))
	} else {
		panic("type is not cx argument deprecate\n\n")
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0521.
	toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])

	// If the last expression in `fromExprs` is declared as pointer
	// then it means the whole struct literal needs to be passed by reference.
	if !hasDeclSpec(lastFromExpressionOutputArg.GetAssignmentElement(prgrm), constants.DECL_POINTER) {
		return assignStructLiteralFields(prgrm, toExprs, fromExprs, toExpressionOutputTypeSig.Name)
	} else {
		// And we also need an auxiliary variable to point to,
		// otherwise we'd be trying to assign the fields to a nil value.
		outField := lastFromExpressionOutputArg
		auxName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)
		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0522.
		_, aux := ast.MakeArgument(prgrm, auxName, lastFromCXLine.FileName, lastFromCXLine.LineNumber)
		aux.SetType(outField.Type)
		aux.DeclarationSpecifiers = append(aux.DeclarationSpecifiers, constants.DECL_POINTER)
		aux.StructType = outField.StructType
		aux.Size = outField.Size
		aux.PreviouslyDeclared = true
		aux.Package = lastFromExpression.Package

		auxTypeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, aux)

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0524.
		declExprCXLine := ast.MakeCXLineExpression(prgrm, lastFromCXLine.FileName, lastFromCXLine.LineNumber, lastFromCXLine.LineStr)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0525.
		declExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
		declExpressionIdx := declExpr.Index

		prgrm.CXAtomicOps[declExpressionIdx].Package = lastFromExpression.Package
		prgrm.CXAtomicOps[declExpressionIdx].AddOutput(prgrm, auxTypeSigIdx)

		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0526.
		_, out := ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, lastFromCXLine.FileName, lastFromCXLine.LineNumber)
		out.Package = lastFromExpression.Package
		outTypeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)

		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0528.
		assignExprCXLine := ast.MakeCXLineExpression(prgrm, lastFromCXLine.FileName, lastFromCXLine.LineNumber, lastFromCXLine.LineStr)
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0529.
		assignExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
		assignExpressionIdx := assignExpr.Index

		prgrm.CXAtomicOps[assignExpressionIdx].Package = lastFromExpression.Package
		prgrm.CXAtomicOps[assignExpressionIdx].AddOutput(prgrm, outTypeSigIdx)
		prgrm.CXAtomicOps[assignExpressionIdx].AddInput(prgrm, auxTypeSigIdx)

		fromExprs = assignStructLiteralFields(prgrm, toExprs, fromExprs, auxName)
		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S0530.
		fromExprs = append([]ast.CXExpression{*declExprCXLine, *declExpr}, fromExprs...)

		return append(fromExprs, *assignExprCXLine, *assignExpr)
	}
}

// ArrayLiteralAssignment handles array literals.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs - toExprs are the array of expressions that contains the data needed
//			  to construct the series of array literals.
//	fromExprs - fromExprs are the array of expressions that will contain the
//				series of array literal assignments.
func ArrayLiteralAssignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0531.
	toExpressionOutput := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])

	for _, expr := range fromExprs {
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0532.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0533.
		expressionOutput := prgrm.GetCXTypeSignatureFromArray(expression.GetOutputs(prgrm)[0])
		var expressionOutputIdx ast.CXArgumentIndex
		if expressionOutput.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			expressionOutputIdx = ast.CXArgumentIndex(expressionOutput.Meta)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1639.
		prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).Name = toExpressionOutput.Name
		expressionOutput.Name = toExpressionOutput.Name
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1640.
		prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations = append(prgrm.GetCXArg(ast.CXArgumentIndex(expressionOutputIdx)).DereferenceOperations, constants.DEREF_ARRAY)
	}

	return fromExprs
}

// ShortAssignment handles short assignments for ">>=","<<=",
// "+=","-=","*=","/=","%=","&=","^=", and "|=" operators.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	expr - the expression for the short assignment.
//	exprCXLine - the cx line or the line string of the short assignment expression.
//	toExprs - Contains the output cx arg to be added to the expression.
//	fromExprs - Contains the output cx arg to be added to the expression.
//	pkg - the package the expression belongs.
func ShortAssignment(prgrm *ast.CXProgram, expr *ast.CXExpression, exprCXLine *ast.CXExpression, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, pkg *ast.CXPackage) []ast.CXExpression {
	expressionIdx := expr.Index

	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	fromExpressionIdx := fromExprs[len(fromExprs)-1].Index

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0534.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	typeSig := toExpression.GetOutputs(prgrm)[0]

	prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSig)
	prgrm.CXAtomicOps[expressionIdx].AddOutput(prgrm, typeSig)

	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)

	if fromExpressionOperator == nil {
		typeSig := prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0]
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSig)
	} else {
		var fromExpressionInputArg *ast.CXArgument
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0536.
		fromExpressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetInputs(prgrm)[0])
		var typeSigIdx ast.CXTypeSignatureIndex
		if fromExpressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0537.
			fromExpressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionInputTypeSig.Meta))

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0538.
			_, sym := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), CurrentFile, LineNo)
			sym.SetType(fromExpressionInputArg.Type)
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, sym)
		} else if fromExpressionInputTypeSig.Type == ast.TYPE_ATOMIC || fromExpressionInputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(fromExpressionInputTypeSig)
			newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
			newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[fromExpressionIdx].AddOutput(prgrm, typeSigIdx)
		prgrm.CXAtomicOps[expressionIdx].AddInput(prgrm, typeSigIdx)
	}

	//must check if from expression is naked previously declared variable
	if len(fromExprs) == 1 && fromExpressionOperator == nil && len(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)) > 0 && len(prgrm.CXAtomicOps[fromExpressionIdx].GetInputs(prgrm)) == 0 {
		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S0543.
		return []ast.CXExpression{*exprCXLine, *expr}
	} else {
		return append(fromExprs, *exprCXLine, *expr)
	}
}

// getOutputType tries to determine the expr's type that should be
// returned by a function call.
// This function is needed because CX has some standard library functions that return cxcore.TYPE_UNDEFINED
// arguments. In these cases, the output type depends on its input arguments' type. In the rest of
// the cases, we can simply use the function's return type.
func getOutputType(prgrm *ast.CXProgram, expr *ast.CXExpression) *ast.CXTypeSignature {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0544.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0545.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	expressionOperatorOutputs := expressionOperator.GetOutputs(prgrm)
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0546.
	expressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(expressionOperatorOutputs[0])
	if expressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0547.
		expressionOperatorOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(expressionOperatorOutputTypeSig.Meta))

		if expressionOperatorOutputArg.Type != types.UNDEFINED {
			return expressionOperatorOutputTypeSig
		}
	} else if expressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC || expressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
		return expressionOperatorOutputTypeSig
	} else {
		panic("type is not known")
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0548.
	expressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(expression.GetInputs(prgrm)[0])

	return expressionInputTypeSig
}

// ShortDeclarationAssignment handles short assignments for ":=" operators.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	pkg - the package the expression belongs.
//	toExprs - Contains the output cx arg to be added to the expression.
//	fromExprs - Contains the output cx arg to be added to the expression.
func shortDeclarationAssignment(prgrm *ast.CXProgram, pkg *ast.CXPackage, toExprs []ast.CXExpression, fromExprs []ast.CXExpression) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	lastFromExpressionIdx := len(fromExprs) - 1
	fromExpressionIdx := fromExprs[lastFromExpressionIdx].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0549.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0550.
	expr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	expressionIdx := expr.Index
	prgrm.CXAtomicOps[expressionIdx].Package = ast.CXPackageIndex(pkg.Index)
	// nil for an atomic output.
	var sym *ast.CXArgument

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0552.
	toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])

	fn, err := prgrm.GetCurrentFunction()
	if err != nil {
		panic(err)
	}

	// Add name in fn.LocalVariables since its a new local declaration
	err = fn.AddLocalVariableName(toExpressionOutputTypeSig.Name)
	if err != nil {
		panic(err)
	}

	var typeSigIdx ast.CXTypeSignatureIndex
	if fromExpressionOperator == nil {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0553.
		fromExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0])
		var fromExpressionOutputArg *ast.CXArgument
		if fromExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0555.
			fromExpressionOutputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOutputTypeSig.Meta))

			// then it's a literal
			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0556.
			_, sym = ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, CurrentFile, LineNo)
			sym.SetType(fromExpressionOutputArg.Type)
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(fromExpressionOutputTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(fromExpressionOutputTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

	} else {
		outTypeSig := getOutputType(prgrm, &fromExprs[lastFromExpressionIdx])

		if outTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0564.
			outTypeArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outTypeSig.Meta))
			outType := outTypeArg.Type
			outTypeIsSlice := outTypeArg.IsSlice()

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0565.
			_, sym = ast.MakeArgument(prgrm, toExpressionOutputTypeSig.Name, CurrentFile, LineNo)
			sym.SetType(outType)

			if fromExprs[lastFromExpressionIdx].IsArrayLiteral() {
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0566.
				fromExpressionInputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetInputs(prgrm)[0])
				var fromExpressionInputArg *ast.CXArgument
				if fromExpressionInputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0568.
					fromExpressionInputArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionInputTypeSig.Meta))
				} else {
					panic("type is not cx argument deprecate\n\n")
				}

				fromCXAtomicOpInputs := fromExpressionInputArg
				sym.Size = fromCXAtomicOpInputs.Size
				sym.Lengths = fromCXAtomicOpInputs.Lengths
			}
			if outTypeIsSlice {
				sym.Lengths = append([]types.Pointer{0}, sym.Lengths...)
				sym.DeclarationSpecifiers = append(sym.DeclarationSpecifiers, constants.DECL_SLICE)
			} else if ast.IsTypeArrayAtomic(outTypeArg) {
				sym.Lengths = outTypeArg.Lengths
				sym.DeclarationSpecifiers = append([]int(nil), outTypeArg.DeclarationSpecifiers...)
			}

			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true
			sym.Offset = outTypeArg.Offset

			typeSigIdx = ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
		} else if outTypeSig.Type == ast.TYPE_ATOMIC {
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(outTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else if outTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// TODO: recheck if this should be pointer
			// or the atomic type its pointing to.
			_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(outTypeSig)
			newTypeSig.Name = toExpressionOutputTypeSig.Name
			typeSigIdx = newTypeSig.Index
		} else {
			panic("type is not known")
		}

	}

	prgrm.CXAtomicOps[expressionIdx].AddOutput(prgrm, typeSigIdx)

	// The type an atomic output gives its receivers is zero.
	var symType, symPointerTargetType types.Code
	var symSize types.Pointer
	if sym != nil {
		symType = sym.Type
		symPointerTargetType = sym.PointerTargetType
		symSize = sym.Size
		prgrm.ReleaseCXArgUnlessNamed(sym, typeSigIdx)
	}

	for _, toExpr := range toExprs {
		if toExpr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0576.
		toExprAtomicOp, err := prgrm.GetCXAtomicOp(toExpr.Index)
		if err != nil {
			panic(err)
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0577.
		toExprAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExprAtomicOp.GetOutputs(prgrm)[0])
		var toExprAtomicOpOutputIdx ast.CXArgumentIndex
		if toExprAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			toExprAtomicOpOutputIdx = ast.CXArgumentIndex(toExprAtomicOpOutputTypeSig.Meta)
		} else {
			panic("type is not type cx argument deprecate\n\n")
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1641.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).PreviouslyDeclared = true

		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1642.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).Type = symType
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1643.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).PointerTargetType = symPointerTargetType
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1644.
		prgrm.GetCXArg(ast.CXArgumentIndex(toExprAtomicOpOutputIdx)).Size = symSize
	}

	// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
	// Now a slice of objects, site S0578.
	return append([]ast.CXExpression{*expr}, toExprs...)
}

func processAssignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, fromExprs []ast.CXExpression, toExpression *ast.CXAtomicOperator) []ast.CXExpression {
	lastFromExpressionIdx := len(fromExprs) - 1
	fromExpressionIdx := fromExprs[lastFromExpressionIdx].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0579.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	toLastExpr, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, len(toExprs)-1)
	if err != nil {
		panic(err)
	}

	if fromExpressionOperator == nil {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S0580.
		opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
		prgrm.CXAtomicOps[fromExpressionIdx].Operator = opIdx

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0581.
		toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
		var toExpressionOutputIdx ast.CXArgumentIndex = -1
		if toExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			toExpressionOutputIdx = ast.CXArgumentIndex(toExpressionOutputTypeSig.Meta)
		} else if toExpressionOutputTypeSig.Type != ast.TYPE_ATOMIC && toExpressionOutputTypeSig.Type != ast.TYPE_POINTER_ATOMIC {
			panic("type is not type cx argument deprecate\n\n")
		}

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0582.
		fromExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(prgrm.CXAtomicOps[fromExpressionIdx].GetOutputs(prgrm)[0])
		if toExpressionOutputIdx == -1 {
			// A typed target, such as a package global, keeps its type.
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0583.
			fromExpressionOutputArg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOutputTypeSig.Meta))

			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1645.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = fromExpressionOutputArg.Size
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1646.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = fromExpressionOutputArg.Type
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1647.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = fromExpressionOutputArg.PointerTargetType
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1648.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = fromExpressionOutputArg.Lengths
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1649.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PassBy = fromExpressionOutputArg.PassBy
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1650.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = types.Code(fromExpressionOutputTypeSig.Meta).Size()
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1651.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOutputTypeSig.Meta)
		} else if fromExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1652.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = types.Code(fromExpressionOutputTypeSig.Meta).Size()
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1653.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.POINTER
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1654.
			prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = types.Code(fromExpressionOutputTypeSig.Meta)
		} else {
			panic("type is not known")
		}

		if fromExprs[lastFromExpressionIdx].IsMethodCall() {
			newInputs := prgrm.CXAtomicOps[fromExpressionIdx].Outputs
			if prgrm.CXAtomicOps[fromExpressionIdx].Inputs != nil {
				for _, typeSig := range prgrm.CXAtomicOps[fromExpressionIdx].Inputs.Fields {
					newInputs.AddField_CXAtomicOps(prgrm, typeSig)
				}
			}
			prgrm.CXAtomicOps[fromExpressionIdx].Inputs = newInputs
		} else {
			prgrm.CXAtomicOps[fromExpressionIdx].Inputs = prgrm.CXAtomicOps[fromExpressionIdx].Outputs
		}

		prgrm.CXAtomicOps[fromExpressionIdx].Outputs = toLastExpr.Outputs

		return append(toExprs[:len(toExprs)-1], fromExprs...)
	} else {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0584.
		toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpression.GetOutputs(prgrm)[0])
		var toExpressionOutputIdx ast.CXArgumentIndex
		if toExpressionOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			toExpressionOutputIdx = ast.CXArgumentIndex(toExpressionOutputTypeSig.Meta)

			fromExpressionOperatorOutputs := fromExpressionOperator.GetOutputs(prgrm)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0585.
			fromExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(fromExpressionOperatorOutputs[0])
			if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0586.
				fromCXAtomicOpOperatorOutput := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOperatorOutputTypeSig.Meta))

				if fromExpressionOperator.IsBuiltIn() {
					// only assigning as if the operator had only one output defined
					if fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
						// it's a short variable declaration
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1655.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = fromCXAtomicOpOperatorOutput.Size
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1656.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = fromCXAtomicOpOperatorOutput.Type
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1657.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = fromCXAtomicOpOperatorOutput.PointerTargetType
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1658.
						prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = fromCXAtomicOpOperatorOutput.Lengths
					}

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1659.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PassBy = fromCXAtomicOpOperatorOutput.PassBy
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1660.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Size = fromCXAtomicOpOperatorOutput.Size
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1661.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = fromCXAtomicOpOperatorOutput.Type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1662.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = fromCXAtomicOpOperatorOutput.PointerTargetType
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1663.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = fromCXAtomicOpOperatorOutput.Lengths
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1664.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PassBy = fromCXAtomicOpOperatorOutput.PassBy
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1665.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1666.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1667.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1668.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.POINTER
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1669.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).PointerTargetType = types.Code(fromExpressionOperatorOutputTypeSig.Meta)
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_SLICE_ATOMIC {
				// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
				// Now an arena getter, site S0587.
				sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(fromExpressionOperatorOutputTypeSig.Meta)

				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1670.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(sliceDetails.Meta)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1671.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = sliceDetails.Lengths
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1672.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.Code(sliceDetails.Meta)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1673.
					prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Lengths = sliceDetails.Lengths
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_STRUCT {
				// it's a short variable declaration
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S1674.
				prgrm.GetCXArg(ast.CXArgumentIndex(toExpressionOutputIdx)).Type = types.STRUCT
			} else {
				panic("type is not known")
			}
		} else if toExpressionOutputTypeSig.Type == ast.TYPE_ATOMIC || toExpressionOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
			fromExpressionOperatorOutputs := fromExpressionOperator.GetOutputs(prgrm)
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0588.
			fromExpressionOperatorOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(fromExpressionOperatorOutputs[0])
			if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0589.
				fromCXAtomicOpOperatorOutput := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(fromExpressionOperatorOutputTypeSig.Meta))
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					toExpressionOutputTypeSig.Meta = int(fromCXAtomicOpOperatorOutput.Type)
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					toExpressionOutputTypeSig.Meta = int(fromCXAtomicOpOperatorOutput.Type)
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				}
			} else if fromExpressionOperatorOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
				// only assigning as if the operator had only one output defined
				if fromExpressionOperator.IsBuiltIn() && fromExpressionOperator.AtomicOPCode != constants.OP_IDENTITY {
					// it's a short variable declaration
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				} else {
					// we'll delegate multiple-value returns to the 'expression' grammar rule
					// only assigning as if the operator had only one output defined
					toExpressionOutputTypeSig.Meta = fromExpressionOperatorOutputTypeSig.Meta
				}
			} else {
				panic("type is not known")
			}
		} else {
			panic("type is not known")
		}

		prgrm.CXAtomicOps[fromExpressionIdx].Outputs = toLastExpr.Outputs

		return append(toExprs[:len(toExprs)-1], fromExprs...)
	}
}

func checkIfFunctionCallNeedsAnOutput(prgrm *ast.CXProgram, fromExpressionOperator *ast.CXFunction, toExpression *ast.CXAtomicOperator) {
	fromExpressionOperatorOutputs := fromExpressionOperator.GetOutputs(prgrm)

	// Checking if we're trying to assign stuff from a function call
	// And if that function call actually returns something. If not, throw an error.
	if fromExpressionOperator != nil && len(fromExpressionOperatorOutputs) == 0 {
		toExpressionOutputs := toExpression.GetOutputs(prgrm)
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0590.
		toExpressionOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(toExpressionOutputs[0])

		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeAssignOutputlessOperator, toExpressionOutputTypeSig.ArgDetails, ast.CompileErrorOperands{})
		prgrm.CXProgramCompileErrorExit()
	}
}

// Assignment handles assignment statements with different operators,
// like =, :=, +=, *=.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and array of the program.
//	toExprs, fromExprs - array of expressions where the assingment
//			  		  	 expression will be added.
//	assignOp - the assignment operator, "=", ":=", ">>=","<<=",
//			   "+=","-=","*=","/=","%=","&=","^=", and "|=".
func Assignment(prgrm *ast.CXProgram, toExprs []ast.CXExpression, assignOp string, fromExprs []ast.CXExpression) []ast.CXExpression {
	toExpression, err := prgrm.GetCXAtomicOpFromExpressions(toExprs, 0)
	if err != nil {
		panic(err)
	}

	lastFromExpressionIdx := len(fromExprs) - 1
	fromExpressionIdx := fromExprs[lastFromExpressionIdx].Index
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0591.
	fromExpressionOperator := prgrm.GetFunctionFromArray(prgrm.CXAtomicOps[fromExpressionIdx].Operator)

	checkIfFunctionCallNeedsAnOutput(prgrm, fromExpressionOperator, toExpression)

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0592.
	exprCXLine := MakeCXLineExpressionCurrent(prgrm)
	switch assignOp {
	case ":=":
		shortDeclExprs := shortDeclarationAssignment(prgrm, pkg, toExprs, fromExprs)
		// OBJMODEL: state.CXExpressionMng (manager storage) or []CXExpressionGid, phase 4 #346.
		// Now a slice of objects, site S0593.
		toExprs = append([]ast.CXExpression{*exprCXLine}, shortDeclExprs...)
	case ">>=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0594.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BITSHR])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "<<=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0595.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BITSHL])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "+=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0596.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_ADD])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "-=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0597.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_SUB])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "*=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0598.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_MUL])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "/=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0599.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_DIV])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "%=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0600.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_MOD])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "&=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0601.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BITAND])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "^=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0602.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BITXOR])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	case "|=":
		// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
		// Now a constructor call, site S0603.
		expr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_BITOR])
		return ShortAssignment(prgrm, expr, exprCXLine, toExprs, fromExprs, pkg)
	}

	return processAssignment(prgrm, toExprs, fromExprs, toExpression)
}
