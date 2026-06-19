package importer

import sdcpb "github.com/sdcio/sdc-protos/sdcpb"

// InferUnionMemberFromTypedValue returns the effective union branch type for a
// unmarshaled TypedValue and declared union schema.
//
// Resolution follows RFC 7950 §9.12: union member types are considered in schema
// declaration order (UnionTypes order). Lexically, the first branch for which
// TVFromStringWithType(tv.ToString()) succeeds and the result is TypedValue.Equal
// to tv wins (same narrowing as validating XML text against each member in order).
//
// If no branch matches lexically (for example pattern/length checks fail during
// lexical parse even though the wire TypedValue was produced from a structured
// union member), a structural fallback runs: among branches whose declared type
// matches tv's protobuf oneof shape (see tvShapeCompatibleWithBranch), the first
// such member in declaration order wins.
func InferUnionMemberFromTypedValue(tv *sdcpb.TypedValue, declared *sdcpb.SchemaLeafType) *sdcpb.SchemaLeafType {
	if tv == nil || declared == nil || declared.Type != "union" {
		return nil
	}
	if m := inferUnionLexicalEqual(tv, declared); m != nil {
		return m
	}
	return inferUnionStructuralUnique(tv, declared)
}

// inferUnionLexicalEqual narrows the union by re-parsing tv's canonical string
// through each branch type in declaration order. The first branch for which
// TVFromStringWithType succeeds and the result is Equal to tv wins (RFC 7950 §9.12).
// If none match, callers may fall back to structural inference.
func inferUnionLexicalEqual(tv *sdcpb.TypedValue, declared *sdcpb.SchemaLeafType) *sdcpb.SchemaLeafType {
	lexical := tv.ToString()
	ts := tv.GetTimestamp() // forwarded for branches whose lexical parse depends on time context
	for _, branch := range declared.UnionTypes {
		if branch == nil {
			continue
		}
		refTV, matched, err := sdcpb.TVFromStringWithType(branch, lexical, ts)
		if err != nil {
			continue
		}
		if !refTV.Equal(tv) {
			continue
		}
		return matched
	}
	return nil
}

// inferUnionStructuralUnique picks a branch when lexical equality cannot decide:
// it considers branches whose declared type is compatible with tv's protobuf oneof
// "shape" (see tvShapeCompatibleWithBranch). Nested union branches recurse until
// a concrete leaf type matches or inner inference returns nil. The first
// compatible member in union declaration order wins (RFC 7950 §9.12).
func inferUnionStructuralUnique(tv *sdcpb.TypedValue, declared *sdcpb.SchemaLeafType) *sdcpb.SchemaLeafType {
	for _, branch := range declared.UnionTypes {
		if branch == nil || !tvShapeCompatibleWithBranch(tv, branch) {
			continue
		}
		if branch.Type == "union" {
			if inner := InferUnionMemberFromTypedValue(tv, branch); inner != nil {
				return inner
			}
			continue
		}
		return branch
	}
	return nil
}

// tvShapeCompatibleWithBranch is a coarse wire-type filter: it checks whether the
// TypedValue's protobuf variant could plausibly hold a value of branch's YANG type.
// It is not full validation (ranges, patterns, identities). Unknown or unmapped
// combinations return false; leafref recurses to the target type.
func tvShapeCompatibleWithBranch(tv *sdcpb.TypedValue, branch *sdcpb.SchemaLeafType) bool {
	if tv == nil || branch == nil {
		return false
	}
	if branch.Type == "union" {
		for _, sub := range branch.UnionTypes {
			if tvShapeCompatibleWithBranch(tv, sub) {
				return true
			}
		}
		return false
	}
	switch tv.Value.(type) {
	case *sdcpb.TypedValue_StringVal:
		switch branch.Type {
		case "string", "enumeration", "bits", "binary", "identityref", "instance-identifier":
			return true
		case "leafref":
			return branch.LeafrefTargetType != nil && tvShapeCompatibleWithBranch(tv, branch.LeafrefTargetType)
		default:
			return false
		}
	case *sdcpb.TypedValue_UintVal:
		switch branch.Type {
		case "uint8", "uint16", "uint32", "uint64":
			return true
		default:
			return false
		}
	case *sdcpb.TypedValue_IntVal:
		switch branch.Type {
		case "int8", "int16", "int32", "int64":
			return true
		default:
			return false
		}
	case *sdcpb.TypedValue_BoolVal:
		return branch.Type == "boolean"
	case *sdcpb.TypedValue_DecimalVal:
		return branch.Type == "decimal64"
	case *sdcpb.TypedValue_EmptyVal:
		return branch.Type == "empty"
	case *sdcpb.TypedValue_IdentityrefVal:
		return branch.Type == "identityref"
	default:
		// e.g. JsonVal, JsonIetfVal, LeaflistVal — not handled by this wire-shape union filter.
		return false
	}
}
