// Package codegen turns a validated schema into static Go types and binders,
// and generates application-owned, dependency-free native configuration runtimes.
// It has no knowledge of application-specific fields or packages.
package codegen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strconv"
	"strings"

	m "github.com/arran4/gogenconf"
)

type Options struct {
	// Package is the destination package name.
	Package string
	// LibraryImport is the configuration library path in generated code.
	// Empty uses github.com/arran4/gogenconf.
	LibraryImport string
	// ConfigImport selects the separately generated application model package.
	ConfigImport string
	// RuntimeImport selects the separately generated application native runtime package.
	// When provided, generated model and binder import RuntimeImport instead of LibraryImport,
	// eliminating all runtime dependency on gogenconf.
	RuntimeImport string
	// EmitAppAPI enables emission of the schema-specialized application configuration API
	// (loading, saving, validation, enrichment, inspection, metadata, and migrations).
	EmitAppAPI bool
}

func Validate(s m.Schema) error {
	if s.Version < 1 {
		return fmt.Errorf("schema requires a positive version")
	}
	paths, names, typesSeen := map[string]bool{}, map[string]bool{}, map[string]bool{"Config": true}
	rootKeys := map[string]bool{}
	for _, f := range s.RootFields {
		for _, e := range f.Examples {
			if e == nil {
				return fmt.Errorf("nil example for root.%s", f.Key)
			}
			if _, err := expression(e); err != nil {
				return fmt.Errorf("invalid example for root.%s: %w", f.Key, err)
			}
			if _, err := m.ParseExpr(m.FormatExpr(e)); err != nil {
				return fmt.Errorf("invalid example syntax for root.%s: %w", f.Key, err)
			}
		}
		if f.InputOnly {
			if f.Key == "" || rootKeys[f.Key] || f.RuntimeOnly || f.Policy != m.Eager || f.GoName != "" || f.GoType != "" {
				return fmt.Errorf("invalid input-only root field %s", f.Key)
			}
			rootKeys[f.Key] = true
			continue
		}
		if !exported(f.GoName) || names[f.GoName] {
			return fmt.Errorf("invalid or duplicate root field mapping %s", f.GoName)
		}
		names[f.GoName] = true
		if !f.RuntimeOnly {
			if f.Key == "" || rootKeys[f.Key] {
				return fmt.Errorf("duplicate or empty root config path %s", f.Key)
			}
			rootKeys[f.Key] = true
		}
		ti, err := ClassifyType(f.GoType)
		if err != nil {
			return err
		}
		if f.Default != nil {
			if lit, ok := f.Default.(m.Literal); ok {
				if err := ValidateTypeDefault(ti, lit.Value); err != nil {
					return fmt.Errorf("root.%s: %w", f.Key, err)
				}
			}
		}
		if f.Policy != m.Eager && f.Policy != m.ProviderBacked {
			return fmt.Errorf("unsupported resolution policy")
		}
		if f.RuntimeOnly && (f.Default != nil || f.Environment != "" || f.Policy != m.Eager || f.Key != "") {
			return fmt.Errorf("runtime-only root field has persisted policy")
		}
		if _, err := expression(f.Default); err != nil {
			return fmt.Errorf("root.%s: %w", f.Key, err)
		}
	}
	for _, sec := range s.Sections {
		if sec.Name == "" || paths[sec.Name] {
			return fmt.Errorf("duplicate or empty section %q", sec.Name)
		}
		paths[sec.Name] = true
		if !exported(sec.GoName) || names[sec.GoName] {
			return fmt.Errorf("invalid or ambiguous section mapping %q", sec.GoName)
		}
		names[sec.GoName] = true
		if !exported(sec.GoType) || typesSeen[sec.GoType] {
			return fmt.Errorf("invalid or ambiguous type %q", sec.GoType)
		}
		typesSeen[sec.GoType] = true
		keys, fields := map[string]bool{}, map[string]bool{}
		if sec.Repeated {
			fields["Name"] = true
		}
		for _, f := range sec.Fields {
			for _, e := range f.Examples {
				if e == nil {
					return fmt.Errorf("nil example for %s.%s", sec.Name, f.Key)
				}
				if _, err := expression(e); err != nil {
					return fmt.Errorf("invalid example for %s.%s: %w", sec.Name, f.Key, err)
				}
				if _, err := m.ParseExpr(m.FormatExpr(e)); err != nil {
					return fmt.Errorf("invalid example syntax for %s.%s", sec.Name, f.Key)
				}
			}
			if f.InputOnly {
				if f.Key == "" || keys[f.Key] || f.RuntimeOnly || f.Policy != m.Eager || f.GoName != "" || f.GoType != "" {
					return fmt.Errorf("invalid input-only field %s.%s", sec.Name, f.Key)
				}
				keys[f.Key] = true
				continue
			}
			if !exported(f.GoName) || fields[f.GoName] {
				return fmt.Errorf("invalid or duplicate field mapping %s.%s", sec.GoName, f.GoName)
			}
			fields[f.GoName] = true
			if !f.RuntimeOnly {
				if f.Key == "" || keys[f.Key] {
					return fmt.Errorf("duplicate or empty config path %s.%s", sec.Name, f.Key)
				}
				keys[f.Key] = true
			}
			ti, err := ClassifyType(f.GoType)
			if err != nil {
				return err
			}
			if f.Default != nil {
				if lit, ok := f.Default.(m.Literal); ok {
					if err := ValidateTypeDefault(ti, lit.Value); err != nil {
						return fmt.Errorf("%s.%s: %w", sec.Name, f.Key, err)
					}
				}
			}
			if f.Policy != m.Eager && f.Policy != m.ProviderBacked {
				return fmt.Errorf("unsupported resolution policy")
			}
			if f.RuntimeOnly && (f.Default != nil || f.Environment != "" || f.Policy != m.Eager || f.Key != "") {
				return fmt.Errorf("runtime-only field has persisted policy")
			}
			if _, err := expression(f.Default); err != nil {
				return fmt.Errorf("%s.%s: %w", sec.Name, f.Key, err)
			}
		}
		if err := validateChildDefinitions(sec, sec.Name, typesSeen); err != nil {
			return err
		}
	}
	return nil
}

// validateChildDefinitions applies the same static constraints to every
// nested generated struct. Type names are global to the generated package;
// field and section identities are scoped to their immediate parent.
func validateChildDefinitions(parent m.SectionDefinition, path string, typesSeen map[string]bool) error {
	childNames := map[string]bool{}
	memberNames := map[string]bool{}
	if parent.Repeated {
		memberNames["Name"] = true
	}
	for _, f := range parent.Fields {
		if !f.InputOnly {
			memberNames[f.GoName] = true
		}
	}
	for _, child := range parent.Children {
		childPath := path + "." + child.Name
		if child.Name == "" || childNames[child.Name] {
			return fmt.Errorf("duplicate or empty nested section %q", childPath)
		}
		childNames[child.Name] = true
		if !exported(child.GoName) || memberNames[child.GoName] {
			return fmt.Errorf("invalid or ambiguous nested field mapping %q", childPath)
		}
		memberNames[child.GoName] = true
		if !exported(child.GoType) || typesSeen[child.GoType] {
			return fmt.Errorf("invalid or ambiguous nested type %q", child.GoType)
		}
		typesSeen[child.GoType] = true
		keys, fields := map[string]bool{}, map[string]bool{}
		if child.Repeated {
			fields["Name"] = true
		}
		for _, f := range child.Fields {
			if f.InputOnly {
				if f.Key == "" || keys[f.Key] || f.RuntimeOnly || f.Policy != m.Eager || f.GoName != "" || f.GoType != "" {
					return fmt.Errorf("invalid input-only field %s.%s", childPath, f.Key)
				}
				keys[f.Key] = true
				continue
			}
			if !exported(f.GoName) || fields[f.GoName] {
				return fmt.Errorf("invalid or duplicate field mapping %s.%s", childPath, f.GoName)
			}
			fields[f.GoName] = true
			if !f.RuntimeOnly {
				if f.Key == "" || keys[f.Key] {
					return fmt.Errorf("duplicate or empty config path %s.%s", childPath, f.Key)
				}
				keys[f.Key] = true
			}
			ti, err := ClassifyType(f.GoType)
			if err != nil {
				return err
			}
			if f.Default != nil {
				if lit, ok := f.Default.(m.Literal); ok {
					if err := ValidateTypeDefault(ti, lit.Value); err != nil {
						return fmt.Errorf("%s.%s: %w", childPath, f.Key, err)
					}
				}
			}
			if f.Policy != m.Eager && f.Policy != m.ProviderBacked {
				return fmt.Errorf("unsupported resolution policy")
			}
			if f.RuntimeOnly && (f.Default != nil || f.Environment != "" || f.Policy != m.Eager || f.Key != "") {
				return fmt.Errorf("runtime-only field has persisted policy")
			}
			if _, err := expression(f.Default); err != nil {
				return fmt.Errorf("%s.%s: %w", childPath, f.Key, err)
			}
			for _, ex := range f.Examples {
				if ex == nil {
					return fmt.Errorf("nil example for %s.%s", childPath, f.Key)
				}
				if _, err := expression(ex); err != nil {
					return fmt.Errorf("invalid example for %s.%s: %w", childPath, f.Key, err)
				}
			}
		}
		if err := validateChildDefinitions(child, childPath, typesSeen); err != nil {
			return err
		}
	}
	return nil
}

func supportedGeneratedType(goType string) bool {
	elem, kind := m.ParseType(goType)
	if kind != m.CollectionNone {
		return supportedGeneratedType(elem)
	}
	switch strings.TrimSpace(goType) {
	case "string", "[]byte", "bool",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return true
	default:
		return false
	}
}

func validateGeneratedDefault(goType string, expr m.Expr) error {
	if expr == nil {
		return nil
	}
	elem, kind := m.ParseType(goType)
	if kind == m.CollectionOrdered {
		switch v := expr.(type) {
		case m.List:
			for _, item := range v.Items {
				if err := validateGeneratedDefault(elem, item); err != nil {
					return err
				}
			}
			return nil
		case *m.List:
			if v == nil {
				return nil
			}
			return validateGeneratedDefault(goType, *v)
		case m.Literal:
			for _, item := range strings.Split(v.Value, ",") {
				if err := validateGeneratedDefault(elem, m.Literal{Value: item}); err != nil {
					return err
				}
			}
			return nil
		case *m.Literal:
			if v == nil {
				return nil
			}
			return validateGeneratedDefault(goType, *v)
		default:
			return fmt.Errorf("ordered default requires a list")
		}
	}
	if kind == m.CollectionKeyed {
		switch v := expr.(type) {
		case m.Map:
			for _, entry := range v.Entries {
				if err := validateGeneratedDefault(elem, entry.Value); err != nil {
					return err
				}
			}
			return nil
		case *m.Map:
			if v == nil {
				return nil
			}
			return validateGeneratedDefault(goType, *v)
		default:
			return fmt.Errorf("keyed default requires a map")
		}
	}
	lit, ok := expr.(m.Literal)
	if !ok {
		if ptr, ok := expr.(*m.Literal); ok && ptr != nil {
			lit = *ptr
		} else {
			return nil // resolver expressions are validated at load time.
		}
	}
	_, err := parseLiteral(goType, lit.Value)
	return err
}

// parseLiteral validates parser widths during generation. It deliberately
// returns only an error: emitted code performs the corresponding conversion
// so generated applications do not depend on this package at runtime.
func parseLiteral(goType, value string) (any, error) {
	switch goType {
	case "string", "[]byte":
		return value, nil
	case "bool":
		return strconv.ParseBool(value)
	case "int":
		return strconv.ParseInt(value, 0, strconv.IntSize)
	case "int8":
		return strconv.ParseInt(value, 0, 8)
	case "int16":
		return strconv.ParseInt(value, 0, 16)
	case "int32":
		return strconv.ParseInt(value, 0, 32)
	case "int64":
		return strconv.ParseInt(value, 0, 64)
	case "uint":
		return strconv.ParseUint(value, 0, strconv.IntSize)
	case "uint8":
		return strconv.ParseUint(value, 0, 8)
	case "uint16":
		return strconv.ParseUint(value, 0, 16)
	case "uint32":
		return strconv.ParseUint(value, 0, 32)
	case "uint64":
		return strconv.ParseUint(value, 0, 64)
	case "float32":
		return strconv.ParseFloat(value, 32)
	case "float64":
		return strconv.ParseFloat(value, 64)
	default:
		return nil, fmt.Errorf("unsupported generated type %q", goType)
	}
}

// emitGeneratedDefault emits only static conversion code. Keeping this in the
// generated binder makes the normal application path independent from the
// generic library's reflection-based resolver.
func emitGeneratedDefault(p func(string, ...any), dest, goType string, expr m.Expr, path string, sensitive bool) {
	elem, kind := m.ParseType(goType)
	switch kind {
	case m.CollectionOrdered:
		switch v := expr.(type) {
		case m.List:
			p("%s=make(%s,0,%d)\n", dest, goType, len(v.Items))
			for _, item := range v.Items {
				p("%s=append(%s,*new(%s))\n", dest, dest, elem)
				emitGeneratedDefault(p, dest+"[len("+dest+")-1]", elem, item, path, sensitive)
			}
		case *m.List:
			if v != nil {
				emitGeneratedDefault(p, dest, goType, *v, path, sensitive)
			}
		case m.Literal:
			emitGeneratedEnvironment(p, dest, goType, strconv.Quote(v.Value), path, sensitive)
		case *m.Literal:
			if v != nil {
				emitGeneratedEnvironment(p, dest, goType, strconv.Quote(v.Value), path, sensitive)
			}
		}
	case m.CollectionKeyed:
		switch v := expr.(type) {
		case m.Map:
			p("%s=make(%s,%d)\n", dest, m.Field{GoType: goType}.ResolvedGoType(), len(v.Entries))
			for _, entry := range v.Entries {
				p("var defaultValue %s\n", elem)
				emitGeneratedDefault(p, "defaultValue", elem, entry.Value, path+"."+entry.Key, sensitive)
				p("%s[%q]=defaultValue\n", dest, entry.Key)
			}
		case *m.Map:
			if v != nil {
				emitGeneratedDefault(p, dest, goType, *v, path, sensitive)
			}
		}
	default:
		if v, ok := literalValue(expr); ok {
			emitGeneratedScalar(p, dest, goType, strconv.Quote(v), path, sensitive)
		}
	}
}

func emitGeneratedEnvironment(p func(string, ...any), dest, goType, raw, path string, sensitive bool) {
	elem, kind := m.ParseType(goType)
	switch kind {
	case m.CollectionOrdered:
		p("{defaultItems:=strings.Split(%s,\",\");%s=make(%s,0,len(defaultItems));for _,defaultItem:=range defaultItems{%s=append(%s,*new(%s));", raw, dest, goType, dest, dest, elem)
		emitGeneratedScalar(p, dest+"[len("+dest+")-1]", elem, "defaultItem", path, sensitive)
		p("}}")
	case m.CollectionKeyed:
		// There is no unambiguous legacy environment encoding for keyed maps.
		// A schema may still use static map defaults and document values.
		p("_=%s", raw)
	default:
		emitGeneratedScalar(p, dest, goType, raw, path, sensitive)
	}
}

func literalValue(expr m.Expr) (string, bool) {
	switch v := expr.(type) {
	case m.Literal:
		return v.Value, true
	case *m.Literal:
		if v != nil {
			return v.Value, true
		}
	}
	return "", false
}

func emitGeneratedScalar(p func(string, ...any), dest, goType, raw, path string, sensitive bool) {
	pathLit := strconv.Quote(path)
	sensitiveLit := strconv.FormatBool(sensitive)
	switch goType {
	case "string":
		p("%s=%s", dest, raw)
	case "[]byte":
		p("%s=[]byte(%s)", dest, raw)
	case "bool":
		p("parsed,err:=strconv.ParseBool(%s);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=parsed", raw, pathLit, sensitiveLit, dest)
	case "int":
		emitGeneratedInteger(p, dest, raw, pathLit, sensitiveLit, "ParseInt", "strconv.IntSize", "int")
	case "int8", "int16", "int32", "int64":
		emitGeneratedInteger(p, dest, raw, pathLit, sensitiveLit, "ParseInt", goType[3:], goType)
	case "uint":
		emitGeneratedInteger(p, dest, raw, pathLit, sensitiveLit, "ParseUint", "strconv.IntSize", "uint")
	case "uint8", "uint16", "uint32", "uint64":
		emitGeneratedInteger(p, dest, raw, pathLit, sensitiveLit, "ParseUint", goType[4:], goType)
	case "float32":
		emitGeneratedFloat(p, dest, raw, pathLit, sensitiveLit, "32", goType)
	case "float64":
		emitGeneratedFloat(p, dest, raw, pathLit, sensitiveLit, "64", goType)
	}
}

func emitGeneratedInteger(p func(string, ...any), dest, raw, path, sensitive, parser, bits, cast string) {
	p("parsed,err:=strconv.%s(%s,0,%s);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=%s(parsed)", parser, raw, bits, path, sensitive, dest, cast)
}

func emitGeneratedFloat(p func(string, ...any), dest, raw, path, sensitive, bits, cast string) {
	p("parsed,err:=strconv.ParseFloat(%s,%s);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=%s(parsed)", raw, bits, path, sensitive, dest, cast)
}

// emitTypedDefaultValue emits field-specific conversion rather than relying on
// the generic resolver. Every fallible conversion returns its error, while
// defaultValueError redacts sensitive values before they reach diagnostics.
func emitTypedDefaultValue(p func(string, ...any), dest string, ti TypeInfo, raw, path string, sensitive bool) {
	if !ti.IsPtr && !ti.IsSlice && (ti.Kind == TypeKindString || ti.Kind == TypeKindBytes) {
		emitTypedDefaultValueBody(p, dest, ti, raw, path, sensitive)
		p("\n")
		return
	}
	if ti.IsSlice && ti.Kind == TypeKindString && ti.ElemType == "string" {
		p("%s=strings.FieldsFunc(%s,func(r rune)bool{return r==','})\n", dest, raw)
		return
	}
	p("{")
	emitTypedDefaultValueBody(p, dest, ti, raw, path, sensitive)
	p("}\n")
}

func emitTypedDefaultValueBody(p func(string, ...any), dest string, ti TypeInfo, raw, path string, sensitive bool) {
	pathLit := strconv.Quote(path)
	sensitiveLit := strconv.FormatBool(sensitive)
	if ti.IsPtr {
		valueType := ti.ElemType
		valueInfo := ti
		valueInfo.GoType, valueInfo.IsPtr = valueType, false
		p("var defaultValue %s;", valueType)
		emitTypedDefaultValueBody(p, "defaultValue", valueInfo, raw, path, sensitive)
		p("%s=&defaultValue;", dest)
		return
	}
	if ti.IsSlice {
		itemInfo := ti
		itemInfo.GoType, itemInfo.IsSlice = ti.ElemType, false
		p("defaultItems:=strings.Split(%s,\",\");%s=make(%s,0,len(defaultItems));for _,defaultItem:=range defaultItems{var defaultValue %s;", raw, dest, ti.GoType, ti.ElemType)
		emitTypedDefaultValueBody(p, "defaultValue", itemInfo, "defaultItem", path, sensitive)
		p("%s=append(%s,defaultValue)};", dest, dest)
		return
	}
	switch ti.Kind {
	case TypeKindString:
		p("%s=%s;", dest, raw)
	case TypeKindBytes:
		p("%s=[]byte(%s);", dest, raw)
	case TypeKindBool:
		p("parsed,err:=strconv.ParseBool(%s);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=parsed;", raw, pathLit, sensitiveLit, dest)
	case TypeKindInt:
		p("parsed,err:=strconv.ParseInt(%s,0,%d);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=%s(parsed);", raw, typeBitSize(ti), pathLit, sensitiveLit, dest, ti.GoType)
	case TypeKindUint:
		p("parsed,err:=strconv.ParseUint(%s,0,%d);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=%s(parsed);", raw, typeBitSize(ti), pathLit, sensitiveLit, dest, ti.GoType)
	case TypeKindFloat:
		p("parsed,err:=strconv.ParseFloat(%s,%d);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=%s(parsed);", raw, typeBitSize(ti), pathLit, sensitiveLit, dest, ti.GoType)
	case TypeKindDuration:
		p("parsed,err:=time.ParseDuration(%s);if err!=nil{return c,defaultValueError(%s,%s,err)};%s=parsed;", raw, pathLit, sensitiveLit, dest)
	case TypeKindCustom:
		p("if dec,ok:=any(&%s).(interface{DecodeConfig(string)error});ok{if err:=dec.DecodeConfig(%s);err!=nil{return c,defaultValueError(%s,%s,err)}}else if dec,ok:=any(&%s).(interface{UnmarshalText([]byte)error});ok{if err:=dec.UnmarshalText([]byte(%s));err!=nil{return c,defaultValueError(%s,%s,err)}}else{return c,fmt.Errorf(\"default %%s: unsupported custom decoder\",%s)};", dest, raw, pathLit, sensitiveLit, dest, raw, pathLit, sensitiveLit, pathLit)
	}
}

func exported(s string) bool { return token.IsIdentifier(s) && ast.IsExported(s) }

func expression(e m.Expr) (string, error) {
	return expressionWithPkg(e, "gogenconf")
}

func expressionWithPkg(e m.Expr, pkg string) (string, error) {
	switch v := e.(type) {
	case nil:
		return "nil", nil
	case m.Literal:
		return pkg + ".Literal{Value:" + strconv.Quote(v.Value) + "}", nil
	case *m.Literal:
		return expressionWithPkg(*v, pkg)
	case m.Call:
		args := []string{}
		for _, a := range v.Args {
			r, err := expressionWithPkg(a, pkg)
			if err != nil {
				return "", err
			}
			args = append(args, r)
		}
		return pkg + ".Call{Name:" + strconv.Quote(v.Name) + ",Args:[]" + pkg + ".Expr{" + strings.Join(args, ",") + "}}", nil
	case *m.Call:
		return expressionWithPkg(*v, pkg)
	case m.List:
		items := []string{}
		for _, it := range v.Items {
			r, err := expressionWithPkg(it, pkg)
			if err != nil {
				return "", err
			}
			items = append(items, r)
		}
		return pkg + ".List{Items:[]" + pkg + ".Expr{" + strings.Join(items, ",") + "}}", nil
	case *m.List:
		if v == nil {
			return "nil", nil
		}
		return expressionWithPkg(*v, pkg)
	case m.Map:
		entries := []string{}
		for _, en := range v.Entries {
			r, err := expressionWithPkg(en.Value, pkg)
			if err != nil {
				return "", err
			}
			entries = append(entries, pkg+".MapEntry{Key:"+strconv.Quote(en.Key)+",Value:"+r+"}")
		}
		return pkg + ".Map{Entries:[]" + pkg + ".MapEntry{" + strings.Join(entries, ",") + "}}", nil
	case *m.Map:
		if v == nil {
			return "nil", nil
		}
		return expressionWithPkg(*v, pkg)
	default:
		return "", fmt.Errorf("unsupported default expression")
	}
}

// Generate validates before producing any output. Go names/types are checked
// at generation time; the Go compiler checks the emitted typed resolver calls.
func Generate(s m.Schema, o Options) ([]byte, error) {
	return generate(s, o, true, true)
}

// GenerateModel emits only concrete application-owned types.
func GenerateModel(s m.Schema, o Options) ([]byte, error) {
	return generate(s, o, true, false)
}

// GenerateBinder emits one-way typed resolution into a separate model package.
func GenerateBinder(s m.Schema, o Options) ([]byte, error) {
	if o.ConfigImport == "" {
		return nil, fmt.Errorf("runtime model import is required")
	}
	return generate(s, o, false, true)
}

func generate(s m.Schema, o Options, model, binder bool) ([]byte, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}
	if !token.IsIdentifier(o.Package) || o.Package == "_" {
		return nil, fmt.Errorf("invalid package")
	}
	if o.LibraryImport == "" {
		o.LibraryImport = "github.com/arran4/gogenconf"
	}
	rtImport := o.LibraryImport
	rtAlias := "gogenconf"
	if o.RuntimeImport != "" {
		rtImport = o.RuntimeImport
		rtAlias = "nativeconfig"
	}

	var b bytes.Buffer
	p := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	p("// Code generated by gogenconf. DO NOT EDIT.\n\npackage %s\n", o.Package)
	qual := ""
	if binder {
		if !model {
			p("import runtimeconfig %q\n", o.ConfigImport)
			qual = "runtimeconfig."
		}
		p("import(\"bytes\";\"context\";\"errors\";\"fmt\";\"io\";\"os\";\"strconv\";\"strings\";\"time\"; %s %q)\n", rtAlias, rtImport)
		p("var _ = bytes.NewReader\nvar _ = io.WriteString\nvar _ = os.Open\nvar _ = strconv.Itoa\nvar _ = time.Second\n")
		p("var _ = strings.Join\nvar _ = fmt.Errorf\nvar _ = errors.New\nvar _ = strconv.IntSize\n")
	} else {
		usesProvider := false
		for _, f := range s.RootFields {
			usesProvider = usesProvider || !f.InputOnly && f.Policy == m.ProviderBacked
		}
		for _, sec := range s.Sections {
			for _, f := range sec.Fields {
				usesProvider = usesProvider || !f.InputOnly && f.Policy == m.ProviderBacked
			}
		}
		if usesProvider {
			p("import %s %q\n", rtAlias, rtImport)
		}
	}
	if model {
		p("type Config struct {\n")
		for _, f := range s.RootFields {
			if f.InputOnly {
				continue
			}
			typ := f.ResolvedGoType()
			if f.Policy == m.ProviderBacked {
				typ = rtAlias + ".Provider[" + typ + "]"
			}
			p("%s %s\n", f.GoName, typ)
		}
		for _, sec := range s.Sections {
			prefix := ""
			if sec.Repeated {
				prefix = "[]"
			}
			p("%s %s%s\n", sec.GoName, prefix, sec.GoType)
		}
		p("}\n")
		for _, sec := range s.Sections {
			emitModelSection(p, sec, rtAlias)
		}
	}
	if !binder {
		return format.Source(b.Bytes())
	}
	p("var ErrInvalidDefault=errors.New(\"invalid generated default\")\nfunc defaultValueError(path string,sensitive bool,err error)error{if sensitive{return fmt.Errorf(\"default %%s: %%w\",path,ErrInvalidDefault)};return fmt.Errorf(\"default %%s: %%w\",path,err)}\n")
	p("// Defaults returns concrete static defaults and optional legacy environment overrides.\n// Invalid static or environment values are errors; they are never treated as omission.\nfunc Defaults(lookup %s.LookupEnv) (%sConfig,error) { var c %sConfig\n", rtAlias, qual, qual)
	for _, f := range s.RootFields {
		if f.InputOnly || f.RuntimeOnly || f.Policy == m.ProviderBacked {
			continue
		}
		ti, _ := ClassifyType(f.GoType)
		if v, ok := f.Default.(m.Literal); ok {
			emitTypedDefaultValue(p, "c."+f.GoName, ti, strconv.Quote(v.Value), f.Key, f.Sensitive)
		}
		if f.Environment != "" {
			p("if lookup!=nil{if v,ok:=lookup(%q);ok", f.Environment)
			if f.EnvironmentNonEmpty {
				p("&&v!=\"\"")
			}
			p("{")
			emitTypedDefaultValueBody(p, "c."+f.GoName, ti, "v", f.Key, f.Sensitive)
			p("}}\n")
		}
	}
	for _, sec := range s.Sections {
		if sec.Repeated {
			continue
		}
		for _, f := range sec.Fields {
			if f.InputOnly || f.RuntimeOnly || f.Policy == m.ProviderBacked {
				continue
			}
			dest := "c." + sec.GoName + "." + f.GoName
			ti, _ := ClassifyType(f.GoType)
			if v, ok := f.Default.(m.Literal); ok {
				emitTypedDefaultValue(p, dest, ti, strconv.Quote(v.Value), sec.Name+"."+f.Key, f.Sensitive)
			}
			if f.Environment != "" {
				p("if lookup!=nil{if v,ok:=lookup(%q);ok", f.Environment)
				if f.EnvironmentNonEmpty {
					p("&&v!=\"\"")
				}
				p("{")
				emitTypedDefaultValueBody(p, dest, ti, "v", sec.Name+"."+f.Key, f.Sensitive)
				p("}}\n")
			}
		}
	}
	p("return c,nil}\n")
	p("func decodeValueError(path string,sensitive bool,span %s.Span,err error)error{if sensitive{err=errors.New(\"value conversion failed\")};return &%s.ConfigError{Stage:%s.StageConvert,Path:path,Source:span,Err:err}}\n", rtAlias, rtAlias, rtAlias)
	for _, f := range s.RootFields {
		if f.InputOnly || f.RuntimeOnly || f.Policy == m.ProviderBacked {
			continue
		}
		ti, err := ClassifyType(f.GoType)
		if err != nil {
			return nil, err
		}
		emitStaticDecoder(p, staticDecoderName("Root", f.GoName), f.ResolvedGoType(), ti, rtAlias, f.Key, f.Sensitive)
	}
	for _, sec := range s.Sections {
		if err := emitSectionStaticDecoders(p, sec, sec.GoName, sec.Name, rtAlias); err != nil {
			return nil, err
		}
	}
	p("// Resolve constructs ordinary Go fields; provider policy alone defers resolution.\nfunc Resolve(ctx context.Context,d *%s.Document,r *%s.Registry,lookup %s.LookupEnv)(%sConfig,error){\nvar c %sConfig\nif d==nil{return c,fmt.Errorf(\"nil document\")}\nif d.Version>%d{return c,fmt.Errorf(\"unsupported future configuration version\")}\n", rtAlias, rtAlias, rtAlias, qual, qual, s.Version)
	for _, f := range s.RootFields {
		if f.InputOnly || f.RuntimeOnly {
			continue
		}
		ex, _ := expressionWithPkg(f.Default, rtAlias)
		p("{e:=%s.BindingValue(d,\"\",%q,%s,%q,%t,lookup)\n", rtAlias, f.Key, ex, f.Environment, f.EnvironmentNonEmpty)
		if f.Required {
			p("if e==nil{return %sConfig{},fmt.Errorf(%q)}\n", qual, "missing required field "+f.Key)
		}
		p("if e!=nil{")
		if f.Policy == m.ProviderBacked {
			p("c.%s=%s.FieldProvider[%s](r,e,%q,%t)\n", f.GoName, rtAlias, f.ResolvedGoType(), f.Key, f.Sensitive)
		} else {
			p("value,err:=decodeRoot%s(ctx,r,e);if err!=nil{return %sConfig{},err};c.%s=value\n", f.GoName, qual, f.GoName)
		}
		p("}}\n")
	}
	for _, sec := range s.Sections {
		section := strconv.Quote(sec.Name)
		dest := "c." + sec.GoName
		if sec.Repeated {
			p("for _,section:=range d.Sections{if !strings.HasPrefix(section.Name,%q){continue};var v %s%s;v.Name=strings.TrimPrefix(section.Name,%q)\n", sec.Name+" ", qual, sec.GoType, sec.Name+" ")
			section = "section.Name"
			dest = "v"
		}
		for _, f := range sec.Fields {
			if f.InputOnly || f.RuntimeOnly {
				continue
			}
			ex, _ := expressionWithPkg(f.Default, rtAlias)
			p("{e:=%s.BindingValue(d,%s,%q,%s,%q,%t,lookup)\n", rtAlias, section, f.Key, ex, f.Environment, f.EnvironmentNonEmpty)
			if f.Required {
				p("if e==nil{return %sConfig{},fmt.Errorf(%q)}\n", qual, "missing required field "+sec.Name+"."+f.Key)
			}
			p("if e!=nil{\n")
			if f.Policy == m.ProviderBacked {
				p("%s.%s=%s.FieldProvider[%s](r,e,%q,%t)\n", dest, f.GoName, rtAlias, f.ResolvedGoType(), sec.Name+"."+f.Key, f.Sensitive)
			} else {
				ti, _ := ClassifyType(f.GoType)
				p("value,err:=%s(ctx,r,e)\n", staticDecoderCall(staticDecoderName(sec.GoName, f.GoName), ti))
				p("if err!=nil{return %sConfig{},err}\n", qual)
				if f.Sensitive {
					p("if s,ok:=any(&value).(interface{SanitizeConfig()error});ok{if err:=s.SanitizeConfig();err!=nil{return %sConfig{},fmt.Errorf(\"sanitize %%s: failed\", %q)}}\n", qual, sec.Name+"."+f.Key)
					p("if v,ok:=any(&value).(interface{ValidateConfig()error});ok{if err:=v.ValidateConfig();err!=nil{return %sConfig{},fmt.Errorf(\"validate %%s: failed\", %q)}}\n", qual, sec.Name+"."+f.Key)
				} else {
					p("if s,ok:=any(&value).(interface{SanitizeConfig()error});ok{if err:=s.SanitizeConfig();err!=nil{return %sConfig{},fmt.Errorf(\"sanitize %%s: %%w\", %q, err)}}\n", qual, sec.Name+"."+f.Key)
					p("if v,ok:=any(&value).(interface{ValidateConfig()error});ok{if err:=v.ValidateConfig();err!=nil{return %sConfig{},fmt.Errorf(\"validate %%s: %%w\", %q, err)}}\n", qual, sec.Name+"."+f.Key)
				}
				p("%s.%s=value\n", dest, f.GoName)
			}
			p("}}\n")
		}
		parent := fmt.Sprintf("d.Section(%q)", sec.Name)
		if sec.Repeated {
			parent = "section"
		}
		if err := emitNestedSectionResolve(p, sec, parent, dest, sec.GoName, sec.Name, qual, rtAlias); err != nil {
			return nil, err
		}
		p("if s,ok:=any(&%s).(interface{SanitizeConfig()error});ok{if err:=s.SanitizeConfig();err!=nil{return %sConfig{},fmt.Errorf(\"sanitize %%s: %%w\", %q, err)}}\n", dest, qual, sec.Name)
		p("if v,ok:=any(&%s).(interface{ValidateConfig()error});ok{if err:=v.ValidateConfig();err!=nil{return %sConfig{},fmt.Errorf(\"validate %%s: %%w\", %q, err)}}\n", dest, qual, sec.Name)
		if sec.Repeated {
			p("c.%s=append(c.%s,v)}\n", sec.GoName, sec.GoName)
			// Brace syntax represents `project lxa {}` as a structural
			// container named project with a child named lxa. Keep legacy
			// `section project lxa` support above, then bind canonical named
			// children without flattening their identity into a string key.
			p("if group:=d.Section(%q);group!=nil{for _,section:=range group.Sections{var v %s%s;v.Name=section.Name\n", sec.Name, qual, sec.GoType)
			if err := emitSectionValueResolve(p, sec, "section", "v", sec.GoName, sec.Name, qual, rtAlias); err != nil {
				return nil, err
			}
			p("c.%s=append(c.%s,v)}}\n", sec.GoName, sec.GoName)
		}
	}
	p("if s,ok:=any(&c).(interface{SanitizeConfig()error});ok{if err:=s.SanitizeConfig();err!=nil{return %sConfig{},fmt.Errorf(\"sanitize config: %%w\", err)}}\n", qual)
	p("if v,ok:=any(&c).(interface{ValidateConfig()error});ok{if err:=v.ValidateConfig();err!=nil{return %sConfig{},fmt.Errorf(\"validate config: %%w\", err)}}\n", qual)
	p("return c,nil}\n")
	if o.EmitAppAPI {
		if err := emitAppAPI(&b, s, o, qual, rtAlias); err != nil {
			return nil, err
		}
	}
	return format.Source(b.Bytes())
}

func emitModelSection(p func(string, ...any), sec m.SectionDefinition, rtAlias string) {
	p("type %s struct {\n", sec.GoType)
	if sec.Repeated {
		p("Name string\n")
	}
	for _, f := range sec.Fields {
		if f.InputOnly {
			continue
		}
		typ := f.ResolvedGoType()
		if f.Policy == m.ProviderBacked {
			typ = rtAlias + ".Provider[" + typ + "]"
		}
		p("%s %s\n", f.GoName, typ)
	}
	for _, child := range sec.Children {
		typ := child.GoType
		if child.Repeated {
			typ = "[]" + typ
		}
		p("%s %s\n", child.GoName, typ)
	}
	p("}\n")
	for _, child := range sec.Children {
		emitModelSection(p, child, rtAlias)
	}
}

func emitSectionStaticDecoders(p func(string, ...any), sec m.SectionDefinition, prefix, path, rtAlias string) error {
	for _, f := range sec.Fields {
		if f.InputOnly || f.RuntimeOnly || f.Policy == m.ProviderBacked {
			continue
		}
		ti, err := ClassifyType(f.GoType)
		if err != nil {
			return err
		}
		emitStaticDecoder(p, staticDecoderName(prefix, f.GoName), f.ResolvedGoType(), ti, rtAlias, path+"."+f.Key, f.Sensitive)
	}
	for _, child := range sec.Children {
		if err := emitSectionStaticDecoders(p, child, prefix+child.GoName, path+"."+child.Name, rtAlias); err != nil {
			return err
		}
	}
	return nil
}

// emitNestedSectionResolve emits direct binding for schema-defined child
// objects. It deliberately walks Document.Section pointers, preserving the
// native object tree instead of manufacturing flattened keys or reflection
// metadata at runtime.
func emitNestedSectionResolve(p func(string, ...any), parent m.SectionDefinition, parentExpr, dest, prefix, path, qual, rtAlias string) error {
	for _, child := range parent.Children {
		childVar := "nested" + prefix + child.GoName
		childDest := dest + "." + child.GoName
		if child.Repeated {
			p("if %s!=nil{for _,%s:=range %s.Sections{if !strings.HasPrefix(%s.Name,%q){continue};var value %s%s;value.Name=strings.TrimPrefix(%s.Name,%q)\n", parentExpr, childVar, parentExpr, childVar, child.Name+" ", qual, child.GoType, childVar, child.Name+" ")
			if err := emitSectionValueResolve(p, child, childVar, "value", prefix+child.GoName, path+"."+child.Name, qual, rtAlias); err != nil {
				return err
			}
			p("%s=append(%s,value)}}\n", childDest, childDest)
			continue
		}
		p("if %s!=nil{if %s:=%s.Section(%q);%s!=nil{var value %s%s\n", parentExpr, childVar, parentExpr, child.Name, childVar, qual, child.GoType)
		if err := emitSectionValueResolve(p, child, childVar, "value", prefix+child.GoName, path+"."+child.Name, qual, rtAlias); err != nil {
			return err
		}
		p("%s=value}}\n", childDest)
	}
	return nil
}

func emitSectionValueResolve(p func(string, ...any), sec m.SectionDefinition, sectionVar, dest, prefix, path, qual, rtAlias string) error {
	for _, f := range sec.Fields {
		if f.InputOnly || f.RuntimeOnly {
			continue
		}
		ex, err := expressionWithPkg(f.Default, rtAlias)
		if err != nil {
			return err
		}
		p("{e:=%s.SectionBindingValue(%s,%q,%s,%q,%t,lookup)\n", rtAlias, sectionVar, f.Key, ex, f.Environment, f.EnvironmentNonEmpty)
		if f.Required {
			p("if e==nil{return %sConfig{},fmt.Errorf(%q)}\n", qual, "missing required field "+path+"."+f.Key)
		}
		p("if e!=nil{")
		if f.Policy == m.ProviderBacked {
			p("%s.%s=%s.FieldProvider[%s](r,e,%q,%t)\n", dest, f.GoName, rtAlias, f.ResolvedGoType(), path+"."+f.Key, f.Sensitive)
		} else {
			p("decoded,err:=decode%s%s(ctx,r,e);if err!=nil{return %sConfig{},err};%s.%s=decoded\n", prefix, f.GoName, qual, dest, f.GoName)
		}
		p("}}\n")
	}
	if err := emitNestedSectionResolve(p, sec, sectionVar, dest, prefix, path, qual, rtAlias); err != nil {
		return err
	}
	p("if s,ok:=any(&%s).(interface{SanitizeConfig()error});ok{if err:=s.SanitizeConfig();err!=nil{return %sConfig{},fmt.Errorf(\"sanitize %%s: %%w\",%q,err)}}\n", dest, qual, path)
	p("if v,ok:=any(&%s).(interface{ValidateConfig()error});ok{if err:=v.ValidateConfig();err!=nil{return %sConfig{},fmt.Errorf(\"validate %%s: %%w\",%q,err)}}\n", dest, qual, path)
	return nil
}
