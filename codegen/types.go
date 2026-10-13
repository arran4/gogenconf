package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"strconv"
	"strings"
	"time"
)

// TypeKind identifies the category of Go type configured in the schema.
type TypeKind string

const (
	TypeKindString   TypeKind = "string"
	TypeKindBool     TypeKind = "bool"
	TypeKindInt      TypeKind = "int"
	TypeKindUint     TypeKind = "uint"
	TypeKindFloat    TypeKind = "float"
	TypeKindDuration TypeKind = "duration"
	TypeKindBytes    TypeKind = "bytes"
	TypeKindCustom   TypeKind = "custom"
)

// TypeInfo provides parsed metadata about a schema field's Go type.
type TypeInfo struct {
	GoType   string
	Kind     TypeKind
	ElemType string
	IsSlice  bool
	IsPtr    bool
}

// ClassifyType inspects and categorizes a Go type expression for code generation.
func ClassifyType(goType string) (TypeInfo, error) {
	expr, err := parser.ParseExpr(goType)
	if err != nil {
		return TypeInfo{}, fmt.Errorf("invalid Go type %q: %w", goType, err)
	}

	var isPtr bool
	if star, ok := expr.(*ast.StarExpr); ok {
		isPtr = true
		expr = star.X
	}

	var isSlice bool
	if arr, ok := expr.(*ast.ArrayType); ok {
		if arr.Len != nil {
			return TypeInfo{}, fmt.Errorf("unsupported fixed-length array %q", goType)
		}
		// Special case: []byte
		if ident, ok := arr.Elt.(*ast.Ident); ok && ident.Name == "byte" {
			if isPtr {
				return TypeInfo{GoType: goType, Kind: TypeKindBytes, ElemType: "byte", IsPtr: true}, nil
			}
			return TypeInfo{GoType: goType, Kind: TypeKindBytes, ElemType: "byte"}, nil
		}
		isSlice = true
		expr = arr.Elt
	}

	switch node := expr.(type) {
	case *ast.Ident:
		switch node.Name {
		case "string":
			return TypeInfo{GoType: goType, Kind: TypeKindString, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		case "bool":
			return TypeInfo{GoType: goType, Kind: TypeKindBool, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		case "int", "int8", "int16", "int32", "int64":
			return TypeInfo{GoType: goType, Kind: TypeKindInt, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
			return TypeInfo{GoType: goType, Kind: TypeKindUint, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		case "float32", "float64":
			return TypeInfo{GoType: goType, Kind: TypeKindFloat, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		case "byte":
			return TypeInfo{GoType: goType, Kind: TypeKindUint, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		default:
			if !ast.IsExported(node.Name) {
				return TypeInfo{}, fmt.Errorf("unsupported type mapping %s", goType)
			}
			// Named application type identifier
			return TypeInfo{GoType: goType, Kind: TypeKindCustom, IsSlice: isSlice, IsPtr: isPtr, ElemType: node.Name}, nil
		}

	case *ast.SelectorExpr:
		if pkg, ok := node.X.(*ast.Ident); ok {
			if pkg.Name == "time" && node.Sel.Name == "Duration" {
				return TypeInfo{GoType: goType, Kind: TypeKindDuration, IsSlice: isSlice, IsPtr: isPtr, ElemType: "time.Duration"}, nil
			}
			// Qualified custom type e.g. mypkg.MyType
			return TypeInfo{GoType: goType, Kind: TypeKindCustom, IsSlice: isSlice, IsPtr: isPtr, ElemType: pkg.Name + "." + node.Sel.Name}, nil
		}
		return TypeInfo{}, fmt.Errorf("unsupported qualified type %q", goType)

	default:
		return TypeInfo{}, fmt.Errorf("unsupported type mapping %q", goType)
	}
}

// ValidateTypeDefault checks that a literal default matches the syntax expected by the type.
func ValidateTypeDefault(ti TypeInfo, val string) error {
	if ti.IsSlice {
		if val == "" {
			return nil
		}
		parts := strings.Split(val, ",")
		elemTi := TypeInfo{Kind: ti.Kind, GoType: ti.ElemType}
		for _, p := range parts {
			if err := ValidateTypeDefault(elemTi, strings.TrimSpace(p)); err != nil {
				return err
			}
		}
		return nil
	}

	switch ti.Kind {
	case TypeKindBool:
		if _, err := strconv.ParseBool(val); err != nil {
			return fmt.Errorf("invalid boolean default %q: %w", val, err)
		}
	case TypeKindInt:
		if _, err := strconv.ParseInt(val, 0, typeBitSize(ti)); err != nil {
			return fmt.Errorf("invalid integer default %q: %w", val, err)
		}
	case TypeKindUint:
		if _, err := strconv.ParseUint(val, 0, typeBitSize(ti)); err != nil {
			return fmt.Errorf("invalid unsigned integer default %q: %w", val, err)
		}
	case TypeKindFloat:
		if _, err := strconv.ParseFloat(val, typeBitSize(ti)); err != nil {
			return fmt.Errorf("invalid float default %q: %w", val, err)
		}
	case TypeKindDuration:
		if _, err := time.ParseDuration(val); err != nil {
			return fmt.Errorf("invalid duration default %q: %w", val, err)
		}
	}
	return nil
}

func typeBitSize(ti TypeInfo) int {
	t := ti.ElemType
	if t == "" {
		t = ti.GoType
	}
	switch t {
	case "int8", "uint8", "byte":
		return 8
	case "int16", "uint16":
		return 16
	case "int32", "uint32", "float32":
		return 32
	case "int64", "uint64", "float64":
		return 64
	default:
		return strconv.IntSize
	}
}
