package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin/src/cipher/encoder"
)

// SliceLiteralExpression handles literal expressions by converting it to a series of `append` expressions.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 typeCode - type code of the slice.
//	 exprs - array of expressions that make up the slice literal expression.
func SliceLiteralExpression(prgrm *ast.CXProgram, typeCode types.Code, exprs []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	symName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0980.
	slcVarExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// adding the declaration
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S0981.
	slcVarExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	slcVarExprAtomicOpIdx := slcVarExpr.Index

	prgrm.CXAtomicOps[slcVarExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0982.
	_, slcVar := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	slcVar.SetType(typeCode)
	slcVar = DeclarationSpecifiers(slcVar, []types.Pointer{0}, constants.DECL_SLICE)
	slcVar.Package = ast.CXPackageIndex(pkg.Index)
	slcVar.PreviouslyDeclared = true

	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, slcVar)
	prgrm.CXAtomicOps[slcVarExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

	result = append(result, *slcVarExprCXLine, *slcVarExpr)

	var endPointsCounter int
	for i, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0984.
		exprAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0985.
		exprAtomicOpOperator := prgrm.GetFunctionFromArray(exprAtomicOp.Operator)

		exprCXLine, _ := prgrm.GetPreviousCXLine(exprs, i)

		if expr.IsArrayLiteral() {
			expr.ExpressionType = ast.CXEXPR_UNUSED

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0986.
			_, symInp := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
			symInp.SetType(typeCode)
			symInp.Package = ast.CXPackageIndex(pkg.Index)

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S0987.
			_, symOut := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
			symOut.SetType(typeCode)
			symOut.Package = ast.CXPackageIndex(pkg.Index)

			endPointsCounter++

			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S0988.
			symExprExprCXLine := MakeCXLineExpressionCurrent(prgrm)
			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S0989.
			symExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
			symExprAtomicOpIdx := symExpr.Index

			prgrm.CXAtomicOps[symExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
			typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symOut)
			prgrm.CXAtomicOps[symExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

			if exprAtomicOpOperator == nil {
				// then it's a literal
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S0991.
				opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_APPEND])
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = opIdx

				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = nil
				typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInp)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, typeSigIdx)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs.Fields = append(prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs.Fields, exprAtomicOp.Outputs.Fields...)
			} else {
				// We need to create a temporary variable to hold the result of the
				// nested expressions. Then use that variable as part of the slice literal.

				outTypeSig := getOutputType(prgrm, &expr)
				var outArg *ast.CXArgument
				var outTypeSigIdx ast.CXTypeSignatureIndex
				if outTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S0993.
					outArg = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outTypeSig.Meta))

					// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
					// Now a constructor call, site S0994.
					_, out := ast.MakeArgument(prgrm, generateTempVarName(prgrm, constants.LOCAL_PREFIX), exprCXLine.FileName, exprCXLine.LineNumber)
					out.SetType(outArg.Type)
					out.PointerTargetType = outArg.PointerTargetType
					out.StructType = outArg.StructType
					out.Size = outArg.Size
					out.PreviouslyDeclared = true
					out.Package = ast.CXPackageIndex(pkg.Index)

					outTypeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, out)
				} else if outTypeSig.Type == ast.TYPE_ATOMIC || outTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					_, newTypeSig := prgrm.AllocCXTypeSignatureCopy(outTypeSig)
					newTypeSig.Name = generateTempVarName(prgrm, constants.LOCAL_PREFIX)
					newTypeSig.Package = ast.CXPackageIndex(pkg.Index)
					outTypeSigIdx = newTypeSig.Index
				} else {
					panic("type is not known")
				}

				// MakeAtomicOperatorExpression above may have grown CXAtomicOps, so refetch.
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S0999.
				exprAtomicOp, err = prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}

				exprAtomicOp.Outputs = nil

				exprAtomicOp.AddOutput(prgrm, outTypeSigIdx)

				result = append(result, expr)
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S1000.
				opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_APPEND])
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = opIdx

				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = nil
				typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInp)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, typeSigIdx)
				prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, outTypeSigIdx)
			}
			result = append(result, *symExprExprCXLine, *symExpr)
		} else {
			result = append(result, expr)
		}
	}

	symNameOutput := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1002.
	_, symOutput := ast.MakeArgument(prgrm, symNameOutput, CurrentFile, LineNo)
	symOutput.SetType(typeCode)
	symOutput.Package = ast.CXPackageIndex(pkg.Index)
	symOutput.PreviouslyDeclared = true

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1003.
	_, symInput := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	symInput.SetType(typeCode)
	symInput.Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1004.
	symExprExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1005.
	symExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1006.
	symExprAtomicOp, err := prgrm.GetCXAtomicOp(symExpr.Index)
	if err != nil {
		panic(err)
	}

	symExprAtomicOp.Package = ast.CXPackageIndex(pkg.Index)
	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symOutput)
	symExprAtomicOp.AddOutput(prgrm, typeSigIdx)
	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInput)
	symExprAtomicOp.AddInput(prgrm, typeSigIdx)

	// marking the output so multidimensional arrays identify the expressions
	result = append(result, *symExprExprCXLine, *symExpr)

	return result
}

// PrimaryStructLiteral creates a struct literal expression.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 structName - name of the struct.
//	 structFields - fields of the struct.
func PrimaryStructLiteral(prgrm *ast.CXProgram, structName string, structFields []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression
	structNameGid := prgrm.CXProgramNameIntern(structName)

	if pkg, err := prgrm.GetCurrentPackage(); err == nil {
		if strct, err := prgrm.GetStruct(structNameGid, pkg.Name); err == nil {
			for _, expr := range structFields {
				if expr.Type == ast.CX_LINE {
					continue
				}
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S1009.
				cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}

				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S1010.
				cxAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(cxAtomicOp.GetOutputs(prgrm)[0])
				if cxAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
					cxAtomicOpOutputIdx := cxAtomicOpOutputTypeSig.Meta
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1741.
					name := prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name

					// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
					// Now a constructor call, site S1011.
					_, field := ast.MakeArgument(prgrm, name, CurrentFile, LineNo)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1742.
					field.Type = prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Type
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1743.
					field.PointerTargetType = prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).PointerTargetType
					field.StructType = strct

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1744.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Package = ast.CXPackageIndex(pkg.Index)

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1745.
					if prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).StructType == nil {
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1746.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).StructType = strct
					}

					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1747.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Size = strct.GetStructSize(prgrm)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1748.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name = structNameGid
					cxAtomicOpOutputTypeSig.Name = structNameGid

					fieldIdx := ast.CXArgumentIndex(field.Index)
					// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
					// Now an arena getter, site S1749.
					prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields = append(prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields, fieldIdx)
				} else if cxAtomicOpOutputTypeSig.Type == ast.TYPE_ATOMIC {
					// TODO: give proper change when we implement type_structs
					// Looks like we have to convert the arg to type cx arg deprecate again
					argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
					argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
					argDetails.FileName = CurrentFile
					argDetails.FileLine = LineNo
					_, newCXArg := prgrm.AllocCXArgument()
					newCXArg.ArgDetails = argDetailsGid
					newCXArg.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					newCXArg.Package = ast.CXPackageIndex(pkg.Index)
					newCXArg.Offset = cxAtomicOpOutputTypeSig.Offset

					newCXArg.StructType = strct
					newCXArg.Size = strct.GetStructSize(prgrm)
					newCXArg.Name = structNameGid

					// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
					// Now a constructor call, site S1015.
					_, field := ast.MakeArgument(prgrm, cxAtomicOpOutputTypeSig.Name, CurrentFile, LineNo)
					field.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					field.StructType = strct
					fieldIdx := ast.CXArgumentIndex(field.Index)

					newCXArg.Fields = append(newCXArg.Fields, fieldIdx)
					newCXArgIdx := ast.CXArgumentIndex(newCXArg.Index)

					cxAtomicOpOutputTypeSig.Name = structNameGid
					cxAtomicOpOutputTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
					cxAtomicOpOutputTypeSig.Meta = int(newCXArgIdx)
				} else if cxAtomicOpOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
					// TODO: give proper change when we implement type_structs
					// Looks like we have to convert the arg to type cx arg deprecate again
					argDetailsGid := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugAlloc()
					argDetails := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(argDetailsGid)
					argDetails.FileName = CurrentFile
					argDetails.FileLine = LineNo
					_, newCXArg := prgrm.AllocCXArgument()
					newCXArg.ArgDetails = argDetailsGid
					newCXArg.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					newCXArg.Package = ast.CXPackageIndex(pkg.Index)
					newCXArg.Offset = cxAtomicOpOutputTypeSig.Offset
					newCXArg.DeclarationSpecifiers = []int{constants.DECL_BASIC, constants.DEREF_POINTER}

					newCXArg.StructType = strct
					newCXArg.Size = strct.GetStructSize(prgrm)
					newCXArg.Name = structNameGid

					// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
					// Now a constructor call, site S1020.
					_, field := ast.MakeArgument(prgrm, cxAtomicOpOutputTypeSig.Name, CurrentFile, LineNo)
					field.Type = types.Code(cxAtomicOpOutputTypeSig.Meta)
					field.StructType = strct
					fieldIdx := ast.CXArgumentIndex(field.Index)

					newCXArg.Fields = append(newCXArg.Fields, fieldIdx)
					newCXArgIdx := ast.CXArgumentIndex(newCXArg.Index)

					cxAtomicOpOutputTypeSig.Name = structNameGid
					cxAtomicOpOutputTypeSig.Type = ast.TYPE_CXARGUMENT_DEPRECATE
					cxAtomicOpOutputTypeSig.Meta = int(newCXArgIdx)
				} else {
					panic("type is not known")
				}

				expr.ExpressionType = ast.CXEXPR_STRUCT_LITERAL
				result = append(result, expr)
			}
		} else {
			panic("type '" + structName + "' does not exist")
		}
	} else {
		panic(err)
	}

	return result
}

// PrimaryStructLiteralExternal creates a struct literal expression after a postfix expression.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 importName - name of the import.
//	 structName - name of the struct.
//	 structFields - fields of the struct.
func PrimaryStructLiteralExternal(prgrm *ast.CXProgram, importName string, structName string, structFields []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression
	structNameGid := prgrm.CXProgramNameIntern(structName)
	importNameGid := prgrm.CXProgramNameIntern(importName)
	if pkg, err := prgrm.GetCurrentPackage(); err == nil {
		if _, err := pkg.GetImport(prgrm, importNameGid); err == nil {
			if strct, err := prgrm.GetStruct(structNameGid, importNameGid); err == nil {
				for _, expr := range structFields {
					if expr.Type == ast.CX_LINE {
						continue
					}
					// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
					// Now an arena getter, site S1023.
					cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
					if err != nil {
						panic(err)
					}

					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S1024.
					cxAtomicOpOutputTypeSig := prgrm.GetCXTypeSignatureFromArray(cxAtomicOp.GetOutputs(prgrm)[0])
					if cxAtomicOpOutputTypeSig.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
						cxAtomicOpOutputIdx := cxAtomicOpOutputTypeSig.Meta
						// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
						// Now a constructor call, site S1025.
						_, field := ast.MakeArgument(prgrm, stringpool.StringGidNil, CurrentFile, LineNo)
						field.SetType(types.IDENTIFIER)
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1750.
						field.Name = prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name

						expr.ExpressionType = ast.CXEXPR_STRUCT_LITERAL

						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1751.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Package = ast.CXPackageIndex(pkg.Index)
						// expr.ProgramOutput[0].Program = prgrm

						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1752.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).StructType = strct
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1753.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Size = strct.GetStructSize(prgrm)
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1754.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Name = structNameGid
						cxAtomicOpOutputTypeSig.Name = structNameGid

						fieldIdx := ast.CXArgumentIndex(field.Index)
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S1755.
						prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields = append(prgrm.GetCXArg(ast.CXArgumentIndex(cxAtomicOpOutputIdx)).Fields, fieldIdx)
					} else if cxAtomicOpOutputTypeSig.Type == ast.TYPE_ATOMIC || cxAtomicOpOutputTypeSig.Type == ast.TYPE_POINTER_ATOMIC {
						panic("type signature is type atomic")
					} else {
						panic("type is not known")
					}

					result = append(result, expr)
				}
			} else {
				panic("type '" + structName + "' does not exist")
			}
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}

	return result
}

// ArrayLiteralExpression creates an array literal expression.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//	 arraySizes - len(arraySizes) is the length of the array while contents of
//					 arraySizes define the size of each dimension.
//	 typeCode - type code of the array.
//	 exprs - the array of expressions composing the array literal expression.
func ArrayLiteralExpression(prgrm *ast.CXProgram, arraySizes []types.Pointer, typeCode types.Code, exprs []ast.CXExpression) []ast.CXExpression {
	var result []ast.CXExpression

	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	symName := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1027.
	arrVarExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1028.
	arrVarExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	arrVarExprAtomicOpIdx := arrVarExpr.Index

	prgrm.CXAtomicOps[arrVarExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1029.
	_, arrVar := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	arrVar.SetType(typeCode)
	arrVar = DeclarationSpecifiers(arrVar, arraySizes, constants.DECL_ARRAY)
	arrVar.Package = ast.CXPackageIndex(pkg.Index)
	arrVar.PreviouslyDeclared = true

	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, arrVar)
	prgrm.CXAtomicOps[arrVarExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

	result = append(result, *arrVarExprCXLine, *arrVarExpr)

	var endPointsCounter int
	for _, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S1031.
		expression, err := prgrm.GetCXAtomicOp(expr.Index)
		if err != nil {
			panic(err)
		}
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S1032.
		expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)

		if expr.IsArrayLiteral() {
			expr.ExpressionType = ast.CXEXPR_UNUSED

			// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
			// Now a constructor call, site S1033.
			_, sym := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
			sym.SetType(typeCode)
			sym.Package = ast.CXPackageIndex(pkg.Index)
			sym.PreviouslyDeclared = true

			idxArg := WritePrimary(prgrm, types.I32, encoder.Serialize(int32(endPointsCounter)), false)
			// index is always i32
			// hence, always an atomic type

			_, indexTypeSignature := prgrm.AllocCXTypeSignature()
			indexTypeSignature.Name = idxArg.Name
			indexTypeSignature.Type = ast.TYPE_ATOMIC
			indexTypeSignature.Meta = int(idxArg.Type)
			indexTypeSignature.Offset = idxArg.Offset // important for this
			indexTypeSignature.Package = idxArg.Package
			indexTypeSignature.ArgDetails = idxArg.ArgDetails
			// The signature keeps the debug record of the dropped literal argument.
			prgrm.ReleaseCXArg(ast.CXArgumentIndex(idxArg.Index))

			indexTypeSignatureIdx := indexTypeSignature.Index
			endPointsCounter++

			sym.Indexes = append(sym.Indexes, indexTypeSignatureIdx)
			sym.DereferenceOperations = append(sym.DereferenceOperations, constants.DEREF_ARRAY)
			sym.Lengths = arraySizes

			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S1036.
			symExprCXLine := MakeCXLineExpressionCurrent(prgrm)
			// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
			// Now a constructor call, site S1037.
			symExpr := ast.MakeAtomicOperatorExpression(prgrm, nil)
			symExprAtomicOpIdx := symExpr.Index

			typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
			prgrm.CXAtomicOps[symExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

			if expressionOperator == nil {
				// then it's a literal
				// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
				// Now an arena insert, site S1039.
				opIdx := prgrm.AddNativeFunctionInArray(ast.Natives[constants.OP_IDENTITY])
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = opIdx
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = expression.Outputs
			} else {
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Operator = expression.Operator
				prgrm.CXAtomicOps[symExprAtomicOpIdx].Inputs = expression.Inputs

				typeSigIdx := ast.AllocCXTypeSignatureOfCXArg(prgrm, sym)
				// hack to get the correct lengths below
				// MakeAtomicOperatorExpression above may have grown CXAtomicOps, so refetch.
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
				// Now an arena getter, site S1041.
				expression, err = prgrm.GetCXAtomicOp(expr.Index)
				if err != nil {
					panic(err)
				}
				expression.AddOutput(prgrm, typeSigIdx)
			}
			prgrm.ReleaseCXArgUnlessNamed(sym, typeSigIdx)
			result = append(result, *symExprCXLine, *symExpr)
		} else {
			result = append(result, expr)
		}
	}

	symNameOutput := generateTempVarName(prgrm, constants.LOCAL_PREFIX)

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1042.
	_, symOutput := ast.MakeArgument(prgrm, symNameOutput, CurrentFile, LineNo)
	symOutput.SetType(typeCode)
	// symOutput.Lengths = append(symOutput.Lengths, arrSizes[len(arrSizes)-1])
	symOutput.Lengths = arraySizes
	symOutput.Package = ast.CXPackageIndex(pkg.Index)
	symOutput.PreviouslyDeclared = true

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1043.
	_, symInput := ast.MakeArgument(prgrm, symName, CurrentFile, LineNo)
	symInput.SetType(typeCode)
	// symInput.Lengths = append(symInput.Lengths, arrSizes[len(arrSizes)-1])
	symInput.Lengths = arraySizes
	symInput.Package = ast.CXPackageIndex(pkg.Index)
	symInput.PreviouslyDeclared = true

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1044.
	symExprCXLine := MakeCXLineExpressionCurrent(prgrm)
	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1045.
	symExpr := ast.MakeAtomicOperatorExpression(prgrm, ast.Natives[constants.OP_IDENTITY])
	symExprAtomicOpIdx := symExpr.Index
	prgrm.CXAtomicOps[symExprAtomicOpIdx].Package = ast.CXPackageIndex(pkg.Index)

	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symOutput)
	prgrm.CXAtomicOps[symExprAtomicOpIdx].AddOutput(prgrm, typeSigIdx)

	typeSigIdx = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, symInput)
	prgrm.CXAtomicOps[symExprAtomicOpIdx].AddInput(prgrm, typeSigIdx)

	// symOutput.SynonymousTo = symInput.Name

	// marking the output so multidimensional arrays identify the expressions
	symExpr.ExpressionType = ast.CXEXPR_ARRAY_LITERAL
	result = append(result, *symExprCXLine, *symExpr)

	return result
}

// StructLiteralFields creates an initial expression for the
// struct literal expression.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	structName - name of the struct.
func StructLiteralFields(prgrm *ast.CXProgram, structName string) ast.CXExpression {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1048.
	_, arg := ast.MakeArgument(prgrm, stringpool.StringGidNil, CurrentFile, LineNo)
	arg.SetType(types.IDENTIFIER)
	arg.Name = prgrm.CXProgramNameIntern(structName)
	arg.Package = ast.CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXExpressionMng.CXExpressionAlloc(), phase 4 #346.
	// Now a constructor call, site S1049.
	expr := ast.MakeAtomicOperatorExpression(prgrm, nil)
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1050.
	expression, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	typeSigIdx := ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, arg)
	expression.AddOutput(prgrm, typeSigIdx)

	expression.Package = ast.CXPackageIndex(pkg.Index)
	return *expr
}
