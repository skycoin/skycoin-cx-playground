package actions

import (
	"math/bits"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// DeclarationSpecifiers is called to build a type of a variable or parameter.
//
// It is called repeatedly while the type is parsed.
//
// Returns the new type build from `declSpec` and `opTyp`.
//
// Input arguments description:
//
//	declSpec:     The incoming type
//	arrayLengths: The lengths of the array if `opTyp` = cxcore.DECL_ARRAY
//	opTyp:        The type of modification to `declSpec` (array of, pointer to, ...)
func DeclarationSpecifiers(declSpec *ast.CXArgument, arrayLengths []types.Pointer, opTyp int) *ast.CXArgument {
	switch opTyp {
	case constants.DECL_POINTER:
		declSpec.DeclarationSpecifiers = append(declSpec.DeclarationSpecifiers, constants.DECL_POINTER)
		declSpec.Size = types.POINTER_SIZE

		if declSpec.Type == types.STR || declSpec.StructType != nil {
			declSpec.PointerTargetType = declSpec.Type
			declSpec.Type = types.POINTER
		}

		return declSpec
	case constants.DECL_ARRAY:
		for range arrayLengths {
			declSpec.DeclarationSpecifiers = append(declSpec.DeclarationSpecifiers, constants.DECL_ARRAY)
		}
		arg := declSpec
		arg.Lengths = arrayLengths

		return arg
	case constants.DECL_SLICE:
		arg := declSpec
		arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_SLICE)
		arg.Lengths = append([]types.Pointer{0}, arg.Lengths...)

		return arg
	case constants.DECL_BASIC:
		arg := declSpec

		return arg
	case constants.DECL_FUNC:
		// Creating this case if additional operations are needed in the
		// future.
		return declSpec
	}

	return nil
}

// DeclarationSpecifiersBasic() returns a type specifier created from one of the builtin types.
func DeclarationSpecifiersBasic(prgrm *ast.CXProgram, typeCode types.Code) *ast.CXArgument {
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0605.
	_, arg := ast.MakeArgument(prgrm, stringpool.StringGidNil, CurrentFile, LineNo)
	arg.SetType(typeCode)
	if typeCode == types.AFF {
		// equivalent to slice of strings
		return DeclarationSpecifiers(arg, []types.Pointer{0}, constants.DECL_SLICE)
	}

	return DeclarationSpecifiers(arg, []types.Pointer{0}, constants.DECL_BASIC)
}

// DeclarationSpecifiersStruct() declares a struct
func DeclarationSpecifiersStruct(prgrm *ast.CXProgram, ident string, pkgName string,
	isExternal bool, currentFile ast.SourceFileGid, lineNo int) *ast.CXArgument {
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}

	identGid := prgrm.CXProgramNameIntern(ident)
	var strct *ast.CXStruct
	if isExternal {
		// custom type in an imported package
		imp, err := pkg.GetImport(prgrm, prgrm.CXProgramNameIntern(pkgName))
		if err != nil {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, currentFile, lineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})

			return declarationSpecifiersPlaceholder(prgrm, currentFile, lineNo)
		}

		strct, err = prgrm.GetStruct(identGid, imp.Name)
		if err != nil {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, currentFile, lineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})

			return declarationSpecifiersPlaceholder(prgrm, currentFile, lineNo)
		}
	} else {
		// custom type in the current package
		strct, err = prgrm.GetStruct(identGid, pkg.Name)
		if err != nil {
			prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, currentFile, lineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})

			return declarationSpecifiersPlaceholder(prgrm, currentFile, lineNo)
		}
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0606.
	_, arg := ast.MakeArgument(prgrm, stringpool.StringGidNil, currentFile, lineNo)
	arg.Type = types.STRUCT
	arg.StructType = strct
	arg.Size = strct.GetStructSize(prgrm)
	arg.Package = ast.CXPackageIndex(pkg.Index)
	arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_STRUCT)

	return arg
}

// declarationSpecifiersPlaceholder stands in for a type that failed to resolve
// after its compile error is recorded, so parsing goes on without a nil.
func declarationSpecifiersPlaceholder(prgrm *ast.CXProgram, currentFile ast.SourceFileGid, lineNo int) *ast.CXArgument {
	_, arg := ast.MakeArgument(prgrm, stringpool.StringGidNil, currentFile, lineNo)
	arg.SetType(types.UNDEFINED)
	arg.Size = 0
	return arg
}

// declarationSizeLimit caps a declaration, a frame and the data segment at the
// range of 32 bit pointers, so offsets never wrap in either build.
const declarationSizeLimit = uint64(types.MAX_UINT32)

// declarationArgSize returns the bytes a declaration of arg takes, and false
// when they pass declarationSizeLimit. It follows ast.GetArgSize.
func declarationArgSize(arg *ast.CXArgument) (types.Pointer, bool) {
	for _, decl := range arg.DeclarationSpecifiers {
		switch decl {
		case constants.DECL_ARRAY:
			total := uint64(arg.Size)
			for _, length := range arg.Lengths {
				high, low := bits.Mul64(total, uint64(length))
				if high != 0 || low > declarationSizeLimit {
					return 0, false
				}
				total = low
			}
			return types.Pointer(total), true
		case constants.DECL_POINTER:
			return arg.Size, true
		case constants.DECL_SLICE:
			return types.POINTER_SIZE, true
		}
	}
	return arg.Size, uint64(arg.Size) <= declarationSizeLimit
}

// declarationSizeSumFits reports whether size bytes more after used stay
// within declarationSizeLimit.
func declarationSizeSumFits(used types.Pointer, size types.Pointer) bool {
	return uint64(used)+uint64(size) <= declarationSizeLimit
}
