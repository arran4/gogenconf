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
		Version: schemaVersion,
	}

	for _, sec := range d.Sections {
		secDef, err := parseSectionDefinition(sec)
		if err != nil {
			return nil, err
		}
		s.Sections = append(s.Sections, secDef)
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
				if v.Value == nil {
					secDef.Repeated = true
				} else if lit, ok := v.Value.(Literal); ok {
					secDef.Repeated = lit.Value == "true" || lit.Value == ""
				}
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

	fields := strings.Fields(name)
	if len(fields) < 2 {
		return Field{}, &ConfigError{
			Stage:  StageValidate,
			Source: sec.Span,
			Err:    fmt.Errorf("field declaration requires key and Go type: %q", sec.Name),
		}
	}

	key := fields[0]
	goType := fields[1]

	f := Field{
		Key:    key,
		GoName: exportIdentifier(key),
		GoType: goType,
	}

	for _, item := range sec.Items {
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
			case "go_type":
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
				f.Required = parseBoolExpr(v.Value)
			case "sensitive":
				f.Sensitive = parseBoolExpr(v.Value)
			case "runtime_only":
				f.RuntimeOnly = parseBoolExpr(v.Value)
			case "input_only":
				f.InputOnly = parseBoolExpr(v.Value)
			case "deprecated":
				f.Deprecated = parseBoolExpr(v.Value)
			case "policy":
				if lit, ok := v.Value.(Literal); ok {
					f.Policy = ResolutionPolicy(lit.Value)
				}
			case "env":
				if lit, ok := v.Value.(Literal); ok {
					f.Environment = lit.Value
				}
			case "env_non_empty":
				f.EnvironmentNonEmpty = parseBoolExpr(v.Value)
			}
		}
	}

	return f, nil
}

func parseBoolExpr(e Expr) bool {
	if e == nil {
		return true
	}
	if lit, ok := e.(Literal); ok {
		return lit.Value == "true" || lit.Value == "1" || lit.Value == ""
	}
	return true
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
