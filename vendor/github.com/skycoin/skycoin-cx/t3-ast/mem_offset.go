package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

type DereferenceStruct struct {
	isPointer     bool
	baseOffset    types.Pointer
	sizeofElement types.Pointer
	derefPointer  bool
	idxCounter    int

	// OBJMODEL: no change, the struct lives on the stack of one GetFinalOffset call.
	// Site S0366.
	arg         *CXArgument
	finalOffset types.Pointer
	fp          types.Pointer
	// fault, when set, records a bad index instead of a panic.
	fault *bool
}

// GetSize ...
func GetArgSize(prgrm *CXProgram, arg *CXArgument) types.Pointer {
	if len(arg.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0367.
		return GetArgSize(prgrm, prgrm.GetCXArgFromArray(arg.Fields[len(arg.Fields)-1]))
	}

	derefCount := len(arg.DereferenceOperations)
	if derefCount > 0 {
		deref := arg.DereferenceOperations[derefCount-1]

		switch deref {
		case constants.DEREF_SLICE, constants.DEREF_ARRAY:
			return arg.Size
		case constants.DEREF_POINTER:
			return arg.Type.Size()
		}
	}

	for _, decl := range arg.DeclarationSpecifiers {
		switch decl {
		case constants.DECL_ARRAY:
			return arg.Size * types.TotalLength(arg.Lengths)
		case constants.DECL_POINTER:
			return arg.Size
		case constants.DECL_SLICE:
			return types.POINTER_SIZE
		}
	}

	if arg.StructType != nil {
		return arg.StructType.GetStructSize(prgrm)
	}

	return arg.Size
}

func GetNativeSize(prgrm *CXProgram, arg *CXArgument) types.Pointer {
	derefCount := len(arg.DereferenceOperations)
	if derefCount > 0 {
		deref := arg.DereferenceOperations[derefCount-1]
		switch deref {
		case constants.DEREF_SLICE, constants.DEREF_ARRAY:
			return arg.Size
		case constants.DEREF_POINTER:
			return arg.Type.Size()
		}
	}

	for _, decl := range arg.DeclarationSpecifiers {
		switch decl {
		case constants.DECL_ARRAY:
			return arg.Size * types.TotalLength(arg.Lengths)
		case constants.DECL_POINTER:
			return arg.Size
		case constants.DECL_SLICE:
			return types.POINTER_SIZE
		}
	}

	if arg.StructType != nil {
		return arg.StructType.GetStructSize(prgrm)
	}

	return arg.Size
}

// GetDerefSize ...
func GetDerefSize(prgrm *CXProgram, arg *CXArgument, index int, derefPointer bool, derefArray bool) types.Pointer {
	if !derefArray && len(arg.Lengths) > 1 && ((index + 1) < len(arg.Lengths)) {
		return types.POINTER_SIZE
	}
	if arg.StructType != nil {
		return arg.StructType.GetStructSize(prgrm)
	}
	if derefPointer {
		return arg.Type.Size()
	}
	return arg.Size
}

// GetDerefSizeSlice ...
func GetDerefSizeSlice(prgrm *CXProgram, typeSignature *CXTypeSignature) types.Pointer {
	if typeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0368.
		arg := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSignature.Meta))
		arg = arg.GetAssignmentElement(prgrm)
		if len(arg.Lengths) > 1 && (len(arg.Lengths)-len(arg.Indexes)) > 1 {
			return types.POINTER_SIZE
		}
		if arg.StructType != nil {
			return arg.StructType.GetStructSize(prgrm)
		}
		return arg.Size
	} else if typeSignature.Type == TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0369.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		if len(sliceDetails.Lengths) > 1 && (len(sliceDetails.Lengths)-len(sliceDetails.Indexes)) > 1 {
			return types.POINTER_SIZE
		}
		return types.Code(sliceDetails.Meta).Size()
	} else if typeSignature.Type == TYPE_POINTER_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0370.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		if len(sliceDetails.Lengths) > 1 && (len(sliceDetails.Lengths)-len(sliceDetails.Indexes)) > 1 {
			return types.POINTER_SIZE
		}

		return types.Code(sliceDetails.Meta).Size()
	}

	return typeSignature.GetSize(prgrm, false)
}

// GetFinalOffset returns the address of the value argTypeSig names in the
// frame at fp. A negative or out of range index panics with its CX error.
func GetFinalOffset(prgrm *CXProgram, fp types.Pointer, oldArg *CXArgument, argTypeSig *CXTypeSignature) types.Pointer {
	return finalOffsetCompute(prgrm, fp, oldArg, argTypeSig, nil)
}

// FinalOffsetChecked is GetFinalOffset that returns false where it would
// panic, so the call stack of a runtime error can print the other values.
func FinalOffsetChecked(prgrm *CXProgram, fp types.Pointer, argTypeSig *CXTypeSignature) (types.Pointer, bool) {
	var fault bool
	offset := finalOffsetCompute(prgrm, fp, nil, argTypeSig, &fault)
	return offset, !fault
}

// offsetIndexFromI32 is types.Cast_i32_to_ptr for an index. With fault set a
// negative index sets *fault and counts as 0 instead of panicking.
func offsetIndexFromI32(value int32, fault *bool) types.Pointer {
	if value < 0 && fault != nil {
		*fault = true
		return 0
	}
	return types.Cast_i32_to_ptr(value)
}

// offsetSliceIndexCheck panics with CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE when
// an index is out of range, or sets *fault when fault is set.
func offsetSliceIndexCheck(valid bool, fault *bool) {
	if valid {
		return
	}
	if fault != nil {
		*fault = true
		return
	}
	panic(types.CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE)
}

// finalOffsetCompute is GetFinalOffset. With fault set it records a bad index
// in *fault instead of panicking.
func finalOffsetCompute(prgrm *CXProgram, fp types.Pointer, oldArg *CXArgument, argTypeSig *CXTypeSignature, fault *bool) types.Pointer {
	var finalOffset types.Pointer

	var arg *CXArgument
	if oldArg != nil {
		arg = oldArg
	} else if argTypeSig.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0372.
		arg = prgrm.GetCXArg(CXArgumentIndex(argTypeSig.Meta))
	} else if argTypeSig.Type == TYPE_ATOMIC {
		argTypeSigOffset := argTypeSig.Offset

		//Todo: find way to eliminate this check
		if argTypeSigOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			argTypeSigOffset += fp
		}

		return argTypeSigOffset
	} else if argTypeSig.Type == TYPE_POINTER_ATOMIC {
		finalOffset = argTypeSig.Offset

		if argTypeSig.IsDeref {
			finalOffset = types.Read_ptr(prgrm.Memory, finalOffset)
		}

		//Todo: find way to eliminate this check
		if finalOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			finalOffset += fp
		}

		if finalOffset.IsValid() && finalOffset >= prgrm.Heap.StartsAt {
			// then it's an object
			finalOffset += types.OBJECT_HEADER_SIZE
		}

		return finalOffset
	} else if argTypeSig.Type == TYPE_ARRAY_ATOMIC {
		argTypeSigOffset := argTypeSig.Offset

		//Todo: find way to eliminate this check
		if argTypeSigOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			argTypeSigOffset += fp
		}

		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0373.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSig.Meta)
		indexesLen := len(arrDetails.Indexes)

		for i := 0; i < indexesLen; i++ {
			var subSize = types.Pointer(1)
			for _, len := range arrDetails.Lengths[i+1:] {
				subSize *= len
			}

			sizeToUse := types.Code(arrDetails.Meta).Size()
			sizeOfElement := subSize * sizeToUse
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0374.
			argTypeSigOffset += offsetIndexFromI32(types.Read_i32(prgrm.Memory, finalOffsetCompute(prgrm, fp, nil, prgrm.GetCXTypeSignatureFromArray(arrDetails.Indexes[i]), fault)), fault) * sizeOfElement
		}

		return argTypeSigOffset
	} else if argTypeSig.Type == TYPE_POINTER_ARRAY_ATOMIC {
		finalOffset = argTypeSig.Offset

		//Todo: find way to eliminate this check
		if finalOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			finalOffset += fp
		}

		if argTypeSig.IsDeref {
			finalOffset = types.Read_ptr(prgrm.Memory, finalOffset)

			// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
			// Now an arena getter, site S0375.
			arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSig.Meta)
			indexesLen := len(arrDetails.Indexes)

			for i := 0; i < indexesLen; i++ {
				var subSize = types.Pointer(1)
				for _, len := range arrDetails.Lengths[i+1:] {
					subSize *= len
				}

				sizeToUse := types.Code(arrDetails.Meta).Size()
				sizeOfElement := subSize * sizeToUse
				// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
				// Now an arena getter, site S0376.
				finalOffset += offsetIndexFromI32(types.Read_i32(prgrm.Memory, finalOffsetCompute(prgrm, fp, nil, prgrm.GetCXTypeSignatureFromArray(arrDetails.Indexes[i]), fault)), fault) * sizeOfElement
			}
		}

		if finalOffset.IsValid() && finalOffset >= prgrm.Heap.StartsAt {
			// then it's an object
			finalOffset += types.OBJECT_HEADER_SIZE
		}

		return finalOffset
	} else if argTypeSig.Type == TYPE_SLICE_ATOMIC {
		argTypeSigOffset := argTypeSig.Offset
		//Todo: find way to eliminate this check
		if argTypeSigOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			argTypeSigOffset += fp
		}

		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0377.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSig.Meta)
		indexesLen := len(arrDetails.Indexes)
		eltSize := types.Code(arrDetails.Meta).Size()

		for i := 0; i < indexesLen; i++ {
			// The elements of an outer slice are pointers to the inner slices.
			sizeToUse := eltSize
			if i+1 < len(arrDetails.Lengths) {
				sizeToUse = types.POINTER_SIZE
			}

			argTypeSigOffset = types.Read_ptr(prgrm.Memory, argTypeSigOffset)
			baseOffset := argTypeSigOffset

			argTypeSigOffset += types.OBJECT_HEADER_SIZE
			argTypeSigOffset += constants.SLICE_HEADER_SIZE

			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0378.
			indexOffset := finalOffsetCompute(prgrm, fp, nil, prgrm.GetCXTypeSignatureFromArray(arrDetails.Indexes[i]), fault)
			indexValue := types.Read_i32(prgrm.Memory, indexOffset)

			argTypeSigOffset += offsetIndexFromI32(indexValue, fault) * sizeToUse // TODO:PTR Use ptr/Read_ptr, array/slice indexing only works with i32.

			offsetSliceIndexCheck(IsValidSliceIndex(prgrm, baseOffset, argTypeSigOffset, sizeToUse), fault)
		}

		return argTypeSigOffset
	} else if argTypeSig.Type == TYPE_POINTER_SLICE_ATOMIC {
		argTypeSigOffset := argTypeSig.Offset

		//Todo: find way to eliminate this check
		if argTypeSigOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			argTypeSigOffset += fp
		}

		if argTypeSig.IsDeref {
			argTypeSigOffset = types.Read_ptr(prgrm.Memory, argTypeSigOffset)

			if argTypeSigOffset.IsValid() && argTypeSigOffset >= prgrm.Heap.StartsAt {
				// then it's an object
				argTypeSigOffset += types.OBJECT_HEADER_SIZE
				argTypeSigOffset += constants.SLICE_HEADER_SIZE
			}
		}

		return argTypeSigOffset
	} else if argTypeSig.Type == TYPE_STRUCT {
		argTypeSigOffset := argTypeSig.Offset

		//Todo: find way to eliminate this check
		if argTypeSigOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			argTypeSigOffset += fp
		}

		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0379.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta)
		for _, fldIdx := range structDetails.Fields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0380.
			fld := prgrm.GetCXArgFromArray(fldIdx)
			argTypeSigOffset += fld.Offset
			argTypeSigOffset = calculateDereferencesCompute(prgrm, fld, argTypeSigOffset, fp, fault)
		}

		return argTypeSigOffset
	} else if argTypeSig.Type == TYPE_POINTER_STRUCT {
		argTypeSigOffset := argTypeSig.Offset

		//Todo: find way to eliminate this check
		if argTypeSigOffset < prgrm.Stack.Size {
			// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
			argTypeSigOffset += fp
		}

		if argTypeSig.IsDeref {
			argTypeSigOffset = types.Read_ptr(prgrm.Memory, argTypeSigOffset)
			if argTypeSigOffset.IsValid() && argTypeSigOffset >= prgrm.Heap.StartsAt {
				// then it's an object
				argTypeSigOffset += types.OBJECT_HEADER_SIZE
			}
		}

		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0381.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta)
		for _, fldIdx := range structDetails.Fields {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0382.
			fld := prgrm.GetCXArgFromArray(fldIdx)
			argTypeSigOffset += fld.Offset
			argTypeSigOffset = calculateDereferencesCompute(prgrm, fld, argTypeSigOffset, fp, fault)
		}

		return argTypeSigOffset
	}

	if arg == nil {
		// A signature of another type has no offset or dereferences of its own.
		if prgrm.Stack.Size > 0 {
			return fp
		}
		return 0
	}

	finalOffset = arg.Offset

	//Todo: find way to eliminate this check
	if finalOffset < prgrm.Stack.Size {
		// Then it's in the stack, not in data or heap and we need to consider the frame pointer.
		finalOffset += fp
	}

	// elt = arg
	//TODO: Eliminate all op codes with more than one return type
	//TODO: Eliminate this loop
	//Q: How can CalculateDereferences change offset?
	//Why is finalOffset fed in as a pointer?

	finalOffset = calculateDereferencesCompute(prgrm, arg, finalOffset, fp, fault)
	for _, fldIdx := range arg.Fields {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0383.
		fld := prgrm.GetCXArgFromArray(fldIdx)
		// elt = fld
		finalOffset += fld.Offset
		finalOffset = calculateDereferencesCompute(prgrm, fld, finalOffset, fp, fault)
	}

	return finalOffset
}

func CalculateDereference_Slice(prgrm *CXProgram, drfsStruct *DereferenceStruct) {
	drfsStruct.isPointer = false
	drfsStruct.finalOffset = types.Read_ptr(prgrm.Memory, drfsStruct.finalOffset)
	drfsStruct.baseOffset = drfsStruct.finalOffset

	drfsStruct.finalOffset += types.OBJECT_HEADER_SIZE
	drfsStruct.finalOffset += constants.SLICE_HEADER_SIZE

	//TODO: delete
	sizeToUse := GetDerefSize(prgrm, drfsStruct.arg, drfsStruct.idxCounter, drfsStruct.derefPointer, false) //TODO: is always arg.Size unless arg.StructType != nil
	drfsStruct.derefPointer = false

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0384.
	indexOffset := finalOffsetCompute(prgrm, drfsStruct.fp, nil, prgrm.GetCXTypeSignatureFromArray(drfsStruct.arg.Indexes[drfsStruct.idxCounter]), drfsStruct.fault)
	indexValue := types.Read_i32(prgrm.Memory, indexOffset)

	drfsStruct.finalOffset += offsetIndexFromI32(indexValue, drfsStruct.fault) * sizeToUse // TODO:PTR Use ptr/Read_ptr, array/slice indexing only works with i32.

	offsetSliceIndexCheck(IsValidSliceIndex(prgrm, drfsStruct.baseOffset, drfsStruct.finalOffset, sizeToUse), drfsStruct.fault)

	drfsStruct.idxCounter++
}

func CalculateDereference_Array(prgrm *CXProgram, drfsStruct *DereferenceStruct) {
	if drfsStruct.fault != nil && drfsStruct.idxCounter >= len(drfsStruct.arg.Lengths) {
		*drfsStruct.fault = true
		return
	}
	var subSize = types.Pointer(1)
	for _, len := range drfsStruct.arg.Lengths[drfsStruct.idxCounter+1:] {
		subSize *= len
	}

	//TODO: Delete
	sizeToUse := GetDerefSize(prgrm, drfsStruct.arg, drfsStruct.idxCounter, drfsStruct.derefPointer, true) //TODO: is always arg.Size unless arg.StructType != nil
	drfsStruct.derefPointer = false
	drfsStruct.baseOffset = drfsStruct.finalOffset
	drfsStruct.sizeofElement = subSize * sizeToUse

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0385.
	drfsStruct.finalOffset += offsetIndexFromI32(types.Read_i32(prgrm.Memory, finalOffsetCompute(prgrm, drfsStruct.fp, nil, prgrm.GetCXTypeSignatureFromArray(drfsStruct.arg.Indexes[drfsStruct.idxCounter]), drfsStruct.fault)), drfsStruct.fault) * drfsStruct.sizeofElement // TODO:PTR Use Read_ptr
	drfsStruct.idxCounter++
}

func CalculateDereference_Pointer(prgrm *CXProgram, drfsStruct *DereferenceStruct) {
	drfsStruct.isPointer = true
	drfsStruct.finalOffset = types.Read_ptr(prgrm.Memory, drfsStruct.finalOffset)
	drfsStruct.derefPointer = true
}

// CalculateDereferences applies the index and pointer dereferences of arg to
// finalOffset.
func CalculateDereferences(prgrm *CXProgram, arg *CXArgument, finalOffset types.Pointer, fp types.Pointer) types.Pointer {
	return calculateDereferencesCompute(prgrm, arg, finalOffset, fp, nil)
}

// calculateDereferencesCompute is CalculateDereferences, see finalOffsetCompute
// for fault.
func calculateDereferencesCompute(prgrm *CXProgram, arg *CXArgument, finalOffset types.Pointer, fp types.Pointer, fault *bool) types.Pointer {
	drfsStruct := &DereferenceStruct{
		idxCounter:  0,
		arg:         arg,
		finalOffset: finalOffset,
		fp:          fp,
		fault:       fault,
	}

	for _, op := range drfsStruct.arg.DereferenceOperations {
		if len(drfsStruct.arg.Indexes) == 0 && op != constants.DEREF_POINTER {
			continue
		}

		switch op {
		case constants.DEREF_SLICE:
			CalculateDereference_Slice(prgrm, drfsStruct)
		case constants.DEREF_ARRAY:
			CalculateDereference_Array(prgrm, drfsStruct)
		case constants.DEREF_POINTER:
			CalculateDereference_Pointer(prgrm, drfsStruct)
		}
	}

	if drfsStruct.finalOffset.IsValid() && drfsStruct.finalOffset >= prgrm.Heap.StartsAt && drfsStruct.isPointer {
		// then it's an object
		drfsStruct.finalOffset += types.OBJECT_HEADER_SIZE
		if drfsStruct.arg.IsSlice() {
			drfsStruct.finalOffset += constants.SLICE_HEADER_SIZE
			offsetSliceIndexCheck(IsValidSliceIndex(prgrm, drfsStruct.baseOffset, drfsStruct.finalOffset, drfsStruct.sizeofElement), drfsStruct.fault)
		}
	}

	return drfsStruct.finalOffset
}
