package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// DeclareStruct takes a name of a struct and a slice of fields representing
// the members and adds the struct to the package.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		structName - name of the struct to declare.
//	 structFields - fields of the struct to be added.
func DeclareStruct(prgrm *ast.CXProgram, structName string, structFields []*ast.CXArgument) {
	// Make sure we are inside a package.
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		// FIXME: Should give a relevant error message
		panic(err)
	}

	// Make sure a struct with the same name is not yet defined.
	strct, err := prgrm.GetStruct(prgrm.CXProgramNameIntern(structName), pkg.Name)
	if err != nil {
		// FIXME: Should give a relevant error message
		panic(err)
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
	// Now a dropped object, site S1208.
	strct.Fields = nil
	var structSize types.Pointer
	for _, field := range structFields {
		fieldSize, fits := declarationArgSize(field)
		if !fits || !declarationSizeSumFits(structSize, fieldSize) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDeclarationTooLarge, field.ArgDetails, ast.CompileErrorOperands{int64(field.Name)})
			prgrm.ReleaseCXArgBuilder(field)
		} else if _, err := strct.GetField(prgrm, field.Name); err == nil {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructFieldMultiplyDefined, field.ArgDetails, ast.CompileErrorOperands{int64(field.Name)})
			prgrm.ReleaseCXArgBuilder(field)
		} else if inner := declareStructArgValueStruct(field); inner != nil && declareStructContains(prgrm, inner, strct) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructRecursive, field.ArgDetails, ast.CompileErrorOperands{int64(strct.Name)})
			prgrm.ReleaseCXArgBuilder(field)
		} else {
			structSize += fieldSize
			strct.AddField(prgrm, field)
		}
	}
}

// declareStructArgValueStruct returns the struct arg holds as a plain value,
// or nil for no struct or one behind a pointer, slice or array (issue-233).
func declareStructArgValueStruct(arg *ast.CXArgument) *ast.CXStruct {
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER || decl == constants.DECL_SLICE || decl == constants.DECL_ARRAY {
			return nil
		}
	}
	return arg.StructType
}

// declareStructContains reports whether a value of strct holds target by
// value. Declared structs never hold themselves, so the walk ends.
func declareStructContains(prgrm *ast.CXProgram, strct *ast.CXStruct, target *ast.CXStruct) bool {
	if strct == target {
		return true
	}
	for _, fieldIdx := range strct.Fields {
		field := prgrm.GetCXTypeSignatureFromArray(fieldIdx)
		var inner *ast.CXStruct
		switch field.Type {
		case ast.TYPE_STRUCT:
			inner = prgrm.GetCXTypeSignatureStructFromArray(field.Meta).StructType
		case ast.TYPE_CXARGUMENT_DEPRECATE:
			inner = declareStructArgValueStruct(prgrm.GetCXArgFromArray(ast.CXArgumentIndex(field.Meta)))
		}
		if inner != nil && declareStructContains(prgrm, inner, target) {
			return true
		}
	}
	return false
}
