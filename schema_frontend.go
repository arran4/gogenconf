package gogenconf

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// ParseSchema reads a declarative schema document and produces a canonical Schema.
func ParseSchema(r io.Reader) (*Schema, error) {
	return ParseSchemaNamed(r, "")
}

// ParseSchemaNamed reads a declarative schema document with source provenance.
func ParseSchemaNamed(r io.Reader, sourceName string) (*Schema, error) {
	doc, err := ParseNamed(r, sourceName)
	if err != nil {
		return nil, err
	}
	return ParseSchemaDocument(doc)
}

// ParseSchemaDocument constructs a Schema from a parsed Document AST.
func ParseSchemaDocument(d *Document) (*Schema, error) {
	if d == nil {
		return nil, fmt.Errorf("nil document")
	}

	// Schema document compatibility assertion:
	// The document identity must identify a schema contract, e.g.:
	//   config for <app>-schema version 1
	//   config for gogenconf-schema version 1
	if d.Identity == "" {
		if !d.HasVersion {
			return nil, &ConfigError{
				Stage:  StageValidate,
				Source: d.HeaderSpan,
				Err:    fmt.Errorf("schema document missing required 'config for <identity>-schema version 1' header"),
			}
		}
	} else if !strings.HasSuffix(d.Identity, "-schema") && d.Identity != "gogenconf-schema" && d.Identity != "schema" {
		return nil, &ConfigError{
			Stage:  StageValidate,
			Source: d.HeaderSpan,
			Err:    fmt.Errorf("invalid schema document identity %q: expected '<app>-schema' or 'gogenconf-schema'", d.Identity),
		}
	}

	if d.Version > 1 {
		return nil, &ConfigError{
			Stage:  StageValidate,
			Source: d.HeaderSpan,
			Err:    fmt.Errorf("unsupported schema specification version %d: expected <= 1", d.Version),
		}
	}

	schemaVersion := d.Version
	if schemaVersion == 0 {
		schemaVersion = 1
	}

	// Document root version can override target configuration version
	if verExpr := d.Value("", "version"); verExpr != nil {
		if lit, ok := verExpr.(Literal); ok {
			if v, err := strconv.Atoi(lit.Value); err == nil && v > 0 {
				schemaVersion = v
			}
		}
	}

	s := &Schema{
		Identity: d.Identity,
		Version:  schemaVersion,
	}

	for _, sec := range d.Sections {
		secDef, err := parseSectionDefinition(sec)
		if err != nil {
			return nil, err
		}
		if sec.Name == "root" {
			if secDef.Repeated || len(secDef.Documentation) != 0 || secDef.GoName != "Root" || secDef.GoType != "RootConfig" {
				return nil, &ConfigError{Stage: StageValidate, Source: sec.Span, Err: fmt.Errorf("root schema declaration supports fields only")}
			}
			s.RootFields = append(s.RootFields, secDef.Fields...)
		} else {
			s.Sections = append(s.Sections, secDef)
		}
	}

	return s, nil
}

func parseSectionDefinition(sec *Section) (SectionDefinition, error) {
	secDef := SectionDefinition{
		Name:   sec.Name,
		GoName: exportIdentifier(sec.Name),
		GoType: exportIdentifier(sec.Name) + "Config",
	}

	for _, item := range sec.Items {
		switch v := item.(type) {
		case Comment:
			if v.Kind == DocumentationComment && v.Text != "" {
				secDef.Documentation = append(secDef.Documentation, v.Text)
			}
		case Entry:
			switch v.Key {
			case "go_name":
				if lit, ok := v.Value.(Literal); ok {
					secDef.GoName = lit.Value
				}
			case "go_type":
				if lit, ok := v.Value.(Literal); ok {
					secDef.GoType = lit.Value
				}
			case "repeated":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return secDef, err
				}
				secDef.Repeated = b
			case "doc":
				if lit, ok := v.Value.(Literal); ok {
					secDef.Documentation = append(secDef.Documentation, lit.Value)
				}
			case "example_names":
				if list, ok := v.Value.(List); ok {
					for _, item := range list.Items {
						if lit, ok := item.(Literal); ok {
							secDef.ExampleNames = append(secDef.ExampleNames, lit.Value)
						}
					}
				} else if lit, ok := v.Value.(Literal); ok {
					for _, name := range strings.Split(lit.Value, ",") {
						trimmed := strings.TrimSpace(name)
						if trimmed != "" {
							secDef.ExampleNames = append(secDef.ExampleNames, trimmed)
						}
					}
				}
			case "field":
				// Compact field declaration: field <key> <go_type>
				if lit, ok := v.Value.(Literal); ok {
					fields := strings.Fields(lit.Value)
					if len(fields) >= 2 {
						key := fields[0]
						goType := fields[1]
						secDef.Fields = append(secDef.Fields, Field{
							Key:    key,
							GoName: exportIdentifier(key),
							GoType: goType,
						})
					}
				}
			default:
				// Compact field declaration: <key> <go_type>
				if lit, ok := v.Value.(Literal); ok && identifier(v.Key) && isValidGoType(lit.Value) {
					secDef.Fields = append(secDef.Fields, Field{
						Key:    v.Key,
						GoName: exportIdentifier(v.Key),
						GoType: lit.Value,
					})
				} else {
					return secDef, &ConfigError{
						Stage:  StageValidate,
						Source: v.Span,
						Err:    fmt.Errorf("unknown section metadata %q", v.Key),
					}
				}
			}
		case *Section:
			field, err := parseFieldDefinition(v)
			if err != nil {
				return secDef, err
			}
			secDef.Fields = append(secDef.Fields, field)
		}
	}

	return secDef, nil
}

func parseFieldDefinition(sec *Section) (Field, error) {
	name := sec.Name
	if strings.HasPrefix(name, "field ") {
		name = strings.TrimPrefix(name, "field ")
	}

	var key, goType string
	items := make([]Item, len(sec.Items))
	copy(items, sec.Items)

	fields := strings.Fields(name)
	if len(fields) >= 2 {
		key = fields[0]
		goType = fields[1]
	} else if len(fields) == 1 {
		if fields[0] == "field" && len(sec.Sections) > 0 {
			child := sec.Sections[0]
			childFields := strings.Fields(child.Name)
			if len(childFields) >= 2 {
				key = childFields[0]
				goType = childFields[1]
				items = append(items, child.Items...)
			} else if len(childFields) == 1 {
				key = childFields[0]
				if len(child.Sections) > 0 && isValidGoType(child.Sections[0].Name) {
					goType = child.Sections[0].Name
					items = append(items, child.Sections[0].Items...)
				}
				items = append(items, child.Items...)
			}
		} else {
			key = fields[0]
			if len(sec.Sections) > 0 && isValidGoType(sec.Sections[0].Name) {
				goType = sec.Sections[0].Name
				items = append(items, sec.Sections[0].Items...)
			}
		}
	}

	f := Field{
		Key:    key,
		GoName: exportIdentifier(key),
		GoType: goType,
	}

	for _, item := range items {
		switch v := item.(type) {
		case Comment:
			if v.Kind == DocumentationComment && v.Text != "" {
				f.Documentation = append(f.Documentation, v.Text)
			}
		case Entry:
			switch v.Key {
			case "go_name":
				if lit, ok := v.Value.(Literal); ok {
					f.GoName = lit.Value
				}
			case "go_type", "type":
				if lit, ok := v.Value.(Literal); ok {
					f.GoType = lit.Value
				}
			case "default":
				f.Default = CloneExpr(v.Value)
			case "example":
				f.Examples = append(f.Examples, CloneExpr(v.Value))
			case "doc":
				if lit, ok := v.Value.(Literal); ok {
					f.Documentation = append(f.Documentation, lit.Value)
				}
			case "required":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return f, err
				}
				f.Required = b
			case "sensitive":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return f, err
				}
				f.Sensitive = b
			case "runtime_only":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return f, err
				}
				f.RuntimeOnly = b
			case "input_only":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return f, err
				}
				f.InputOnly = b
			case "deprecated":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return f, err
				}
				f.Deprecated = b
			case "policy":
				if lit, ok := v.Value.(Literal); ok {
					if lit.Value != "" && lit.Value != string(ProviderBacked) {
						return f, &ConfigError{
							Stage:  StageValidate,
							Source: v.Span,
							Err:    fmt.Errorf("invalid resolution policy %q: expected %q", lit.Value, ProviderBacked),
						}
					}
					f.Policy = ResolutionPolicy(lit.Value)
				}
			case "env":
				if lit, ok := v.Value.(Literal); ok {
					f.Environment = lit.Value
				}
			case "env_non_empty":
				b, err := parseBoolExpr(v.Value, v.Span)
				if err != nil {
					return f, err
				}
				f.EnvironmentNonEmpty = b
			default:
				return f, &ConfigError{
					Stage:  StageValidate,
					Source: v.Span,
					Err:    fmt.Errorf("unknown field metadata %q", v.Key),
				}
			}
		}
	}

	if f.Key == "" || f.GoType == "" {
		return Field{}, &ConfigError{
			Stage:  StageValidate,
			Source: sec.Span,
			Err:    fmt.Errorf("field declaration requires key and Go type: %q", sec.Name),
		}
	}
	if !isValidGoType(f.GoType) {
		return Field{}, &ConfigError{
			Stage:  StageValidate,
			Source: sec.Span,
			Err:    fmt.Errorf("invalid Go type %q for field %q", f.GoType, f.Key),
		}
	}
	if f.Default != nil {
		if err := validateFieldDefault(f, sec.Span); err != nil {
			return f, err
		}
	}

	return f, nil
}

func validateFieldDefault(f Field, span Span) error {
	if f.Default == nil {
		return nil
	}
	elem, kind := ParseType(f.GoType)
	switch kind {
	case CollectionOrdered:
		switch v := f.Default.(type) {
		case List:
			for _, item := range v.Items {
				if err := validateScalarDefault(elem, item, span); err != nil {
					return err
				}
			}
		case Literal:
			return validateScalarDefault(elem, v, span)
		default:
			return &ConfigError{
				Stage:  StageValidate,
				Source: span,
				Err:    fmt.Errorf("invalid default for ordered collection %q: expected list, got %T", f.Key, f.Default),
			}
		}
	case CollectionKeyed:
		switch v := f.Default.(type) {
		case Map:
			for _, entry := range v.Entries {
				if err := validateScalarDefault(elem, entry.Value, entry.Span); err != nil {
					return err
				}
			}
		default:
			return &ConfigError{
				Stage:  StageValidate,
				Source: span,
				Err:    fmt.Errorf("invalid default for keyed collection %q: expected map, got %T", f.Key, f.Default),
			}
		}
	default:
		return validateScalarDefault(f.GoType, f.Default, span)
	}
	return nil
}

func validateScalarDefault(goType string, val Expr, span Span) error {
	lit, ok := val.(Literal)
	if !ok {
		return nil
	}
	switch goType {
	case "int", "int64", "int32", "uint", "uint64", "uint32":
		if _, err := strconv.ParseInt(lit.Value, 10, 64); err != nil {
			if _, err2 := strconv.ParseUint(lit.Value, 10, 64); err2 != nil {
				return &ConfigError{
					Stage:  StageValidate,
					Source: span,
					Err:    fmt.Errorf("invalid numeric default %q for type %s: %w", lit.Value, goType, err),
				}
			}
		}
	case "bool":
		if _, err := parseBoolExpr(lit, span); err != nil {
			return &ConfigError{
				Stage:  StageValidate,
				Source: span,
				Err:    fmt.Errorf("invalid boolean default %q: %w", lit.Value, err),
			}
		}
	case "float64", "float32":
		if _, err := strconv.ParseFloat(lit.Value, 64); err != nil {
			return &ConfigError{
				Stage:  StageValidate,
				Source: span,
				Err:    fmt.Errorf("invalid float default %q for type %s: %w", lit.Value, goType, err),
			}
		}
	}
	return nil
}

func parseBoolExpr(e Expr, span Span) (bool, error) {
	if e == nil {
		return true, nil
	}
	lit, ok := e.(Literal)
	if !ok {
		return false, &ConfigError{
			Stage:  StageValidate,
			Source: span,
			Err:    fmt.Errorf("expected boolean literal, got %T", e),
		}
	}
	switch strings.ToLower(lit.Value) {
	case "true", "1", "":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, &ConfigError{
			Stage:  StageValidate,
			Source: span,
			Err:    fmt.Errorf("invalid boolean value %q", lit.Value),
		}
	}
}

func exportIdentifier(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

// FormatSchema converts a Schema definition into canonical declarative gogenconf schema document syntax.
func FormatSchema(identity string, s *Schema) string {
	var b strings.Builder
	if identity == "" {
		identity = "gogenconf"
	}
	if !strings.HasSuffix(identity, "-schema") && identity != "gogenconf-schema" {
		identity = identity + "-schema"
	}

	fmt.Fprintf(&b, "config for %s version 1\n\n", identity)
	fmt.Fprintf(&b, "version %d\n\n", s.Version)

	for _, sec := range s.Sections {
		fmt.Fprintf(&b, "section %s\n", sec.Name)
		if sec.GoName != "" && sec.GoName != exportIdentifier(sec.Name) {
			fmt.Fprintf(&b, "    go_name %s\n", sec.GoName)
		}
		if sec.GoType != "" && sec.GoType != exportIdentifier(sec.Name)+"Config" {
			fmt.Fprintf(&b, "    go_type %s\n", sec.GoType)
		}
		if sec.Repeated {
			b.WriteString("    repeated true\n")
		}
		if len(sec.ExampleNames) > 0 {
			quoted := make([]string, len(sec.ExampleNames))
			for i, name := range sec.ExampleNames {
				quoted[i] = strconv.Quote(name)
			}
			fmt.Fprintf(&b, "    example_names [%s]\n", strings.Join(quoted, ", "))
		}
		for _, doc := range sec.Documentation {
			fmt.Fprintf(&b, "    ## %s\n", doc)
		}

		for _, f := range sec.Fields {
			b.WriteString("\n")
			fmt.Fprintf(&b, "    section field %s %s\n", f.Key, f.GoType)
			if f.GoName != "" && f.GoName != exportIdentifier(f.Key) {
				fmt.Fprintf(&b, "        go_name %s\n", f.GoName)
			}
			if f.Default != nil {
				fmt.Fprintf(&b, "        default %s\n", FormatExpr(f.Default))
			}
			for _, ex := range f.Examples {
				fmt.Fprintf(&b, "        example %s\n", FormatExpr(ex))
			}
			for _, doc := range f.Documentation {
				fmt.Fprintf(&b, "        ## %s\n", doc)
			}
			if f.Required {
				b.WriteString("        required true\n")
			}
			if f.Sensitive {
				b.WriteString("        sensitive true\n")
			}
			if f.RuntimeOnly {
				b.WriteString("        runtime_only true\n")
			}
			if f.InputOnly {
				b.WriteString("        input_only true\n")
			}
			if f.Deprecated {
				b.WriteString("        deprecated true\n")
			}
			if f.Policy != "" {
				fmt.Fprintf(&b, "        policy %s\n", f.Policy)
			}
			if f.Environment != "" {
				fmt.Fprintf(&b, "        env %s\n", f.Environment)
			}
			if f.EnvironmentNonEmpty {
				b.WriteString("        env_non_empty true\n")
			}
			b.WriteString("    end\n")
		}
		b.WriteString("end\n\n")
	}

	return b.String()
}

func isValidGoType(t string) bool {
	if t == "true" || t == "false" || t == "" {
		return false
	}
	elem, _ := ParseType(t)
	if elem == "" || elem == "true" || elem == "false" {
		return false
	}
	first := rune(elem[0])
	return first == '*' || unicode.IsLetter(first) || first == '_'
}
