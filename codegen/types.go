package codegen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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
	IsMap    bool
	IsPtr    bool
	// Element retains the complete type-directed shape of a collection member.
	// It is nil for scalar values and avoids flattening nested lists/maps into
	// an accidental scalar conversion path.
	Element *TypeInfo
}

// ClassifyType inspects and categorizes a Go type expression for code generation.
func ClassifyType(goType string) (TypeInfo, error) {
	// [$]T is the schema spelling for map[string]T. Generated Go uses the
	// ordinary Go spelling, but classification keeps one type-directed path.
	if elem, ok := keyedElementType(goType); ok {
		goType = "map[string]" + elem
	}
	expr, err := parser.ParseExpr(goType)
	if err != nil {
		return TypeInfo{}, fmt.Errorf("invalid Go type %q: %w", goType, err)
	}

	return classifyExpr(expr, goType)
}

func classifyExpr(expr ast.Expr, goType string) (TypeInfo, error) {
	if star, ok := expr.(*ast.StarExpr); ok {
		child, err := classifyExpr(star.X, typeExprString(star.X))
		if err != nil {
			return TypeInfo{}, err
		}
		child.GoType, child.IsPtr = goType, true
		return child, nil
	}
	if arr, ok := expr.(*ast.ArrayType); ok {
		if arr.Len != nil {
			return TypeInfo{}, fmt.Errorf("unsupported fixed-length array %q", goType)
		}
		if ident, ok := arr.Elt.(*ast.Ident); ok && ident.Name == "byte" {
			return TypeInfo{GoType: goType, Kind: TypeKindBytes, ElemType: "byte"}, nil
		}
		elem, err := classifyExpr(arr.Elt, typeExprString(arr.Elt))
		if err != nil {
			return TypeInfo{}, err
		}
		return TypeInfo{GoType: goType, Kind: elem.Kind, ElemType: elem.GoType, IsSlice: true, Element: &elem}, nil
	}
	if mp, ok := expr.(*ast.MapType); ok {
		key, ok := mp.Key.(*ast.Ident)
		if !ok || key.Name != "string" {
			return TypeInfo{}, fmt.Errorf("only string-keyed maps are supported: %q", goType)
		}
		elem, err := classifyExpr(mp.Value, typeExprString(mp.Value))
		if err != nil {
			return TypeInfo{}, err
		}
		return TypeInfo{GoType: goType, Kind: elem.Kind, ElemType: elem.GoType, IsMap: true, Element: &elem}, nil
	}

	switch node := expr.(type) {
	case *ast.Ident:
		switch node.Name {
		case "string":
			return TypeInfo{GoType: goType, Kind: TypeKindString, ElemType: node.Name}, nil
		case "bool":
			return TypeInfo{GoType: goType, Kind: TypeKindBool, ElemType: node.Name}, nil
		case "int", "int8", "int16", "int32", "int64":
			return TypeInfo{GoType: goType, Kind: TypeKindInt, ElemType: node.Name}, nil
		case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
			return TypeInfo{GoType: goType, Kind: TypeKindUint, ElemType: node.Name}, nil
		case "float32", "float64":
			return TypeInfo{GoType: goType, Kind: TypeKindFloat, ElemType: node.Name}, nil
		case "byte":
			return TypeInfo{GoType: goType, Kind: TypeKindUint, ElemType: node.Name}, nil
		default:
			if !ast.IsExported(node.Name) {
				return TypeInfo{}, fmt.Errorf("unsupported type mapping %s", goType)
			}
			// Named application type identifier
			return TypeInfo{GoType: goType, Kind: TypeKindCustom, ElemType: node.Name}, nil
		}

	case *ast.SelectorExpr:
		if pkg, ok := node.X.(*ast.Ident); ok {
			if pkg.Name == "time" && node.Sel.Name == "Duration" {
				return TypeInfo{GoType: goType, Kind: TypeKindDuration, ElemType: "time.Duration"}, nil
			}
			// Qualified custom type e.g. mypkg.MyType
			return TypeInfo{GoType: goType, Kind: TypeKindCustom, ElemType: pkg.Name + "." + node.Sel.Name}, nil
		}
		return TypeInfo{}, fmt.Errorf("unsupported qualified type %q", goType)

	default:
		return TypeInfo{}, fmt.Errorf("unsupported type mapping %q", goType)
	}
}

func typeExprString(expr ast.Expr) string {
	var b bytes.Buffer
	if err := format.Node(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return b.String()
}

func keyedElementType(goType string) (string, bool) {
	trimmed := strings.TrimSpace(goType)
	if strings.HasPrefix(trimmed, "[$]") {
		return strings.TrimSpace(trimmed[3:]), true
	}
	return "", false
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
