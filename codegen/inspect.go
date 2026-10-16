package codegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	m "github.com/arran4/gogenconf"
)

// SchemaFieldInfo describes a single configuration field within an inspected schema section.
type SchemaFieldInfo struct {
	Key           string   `json:"key"`
	GoName        string   `json:"go_name"`
	GoType        string   `json:"go_type"`
	Default       string   `json:"default,omitempty"`
	Examples      []string `json:"examples,omitempty"`
	Required      bool     `json:"required,omitempty"`
	Sensitive     bool     `json:"sensitive,omitempty"`
	InputOnly     bool     `json:"input_only,omitempty"`
	RuntimeOnly   bool     `json:"runtime_only,omitempty"`
	Deprecated    bool     `json:"deprecated,omitempty"`
	Policy        string   `json:"policy,omitempty"`
	Environment   string   `json:"environment,omitempty"`
	Documentation []string `json:"documentation,omitempty"`
}

// SchemaSectionInfo describes an inspected configuration section.
type SchemaSectionInfo struct {
	Name          string              `json:"name"`
	GoName        string              `json:"go_name"`
	GoType        string              `json:"go_type"`
	Repeated      bool                `json:"repeated,omitempty"`
	ExampleNames  []string            `json:"example_names,omitempty"`
	Documentation []string            `json:"documentation,omitempty"`
	Fields        []SchemaFieldInfo   `json:"fields"`
	Children      []SchemaSectionInfo `json:"children,omitempty"`
}

// SchemaInfo captures the full inspected schema metadata in a structured model.
type SchemaInfo struct {
	Identity   string              `json:"identity"`
	Version    int                 `json:"version"`
	RootFields []SchemaFieldInfo   `json:"root_fields,omitempty"`
	Sections   []SchemaSectionInfo `json:"sections"`
}

func inspectFieldInfo(f m.Field) SchemaFieldInfo {
	defStr := ""
	if f.Default != nil {
		defStr = m.FormatExpr(f.Default)
	}

	var exStrs []string
	for _, ex := range f.Examples {
		if ex != nil {
			exStrs = append(exStrs, m.FormatExpr(ex))
		}
	}

	return SchemaFieldInfo{
		Key:           f.Key,
		GoName:        f.GoName,
		GoType:        f.ResolvedGoType(),
		Default:       defStr,
		Examples:      exStrs,
		Required:      f.Required,
		Sensitive:     f.Sensitive,
		InputOnly:     f.InputOnly,
		RuntimeOnly:   f.RuntimeOnly,
		Deprecated:    f.Deprecated,
		Policy:        string(f.Policy),
		Environment:   f.Environment,
		Documentation: f.Documentation,
	}
}

func inspectSectionInfo(sec m.SectionDefinition) SchemaSectionInfo {
	secInfo := SchemaSectionInfo{
		Name:          sec.Name,
		GoName:        sec.GoName,
		GoType:        sec.GoType,
		Repeated:      sec.Repeated,
		ExampleNames:  sec.ExampleNames,
		Documentation: sec.Documentation,
	}

	for _, f := range sec.Fields {
		secInfo.Fields = append(secInfo.Fields, inspectFieldInfo(f))
	}
	for _, ch := range sec.Children {
		secInfo.Children = append(secInfo.Children, inspectSectionInfo(ch))
	}
	return secInfo
}

// InspectSchema extracts structured introspection metadata from a parsed Schema.
func InspectSchema(s *m.Schema) *SchemaInfo {
	if s == nil {
		return nil
	}

	info := &SchemaInfo{
		Identity: s.Identity,
		Version:  s.Version,
	}

	for _, f := range s.RootFields {
		info.RootFields = append(info.RootFields, inspectFieldInfo(f))
	}

	for _, sec := range s.Sections {
		info.Sections = append(info.Sections, inspectSectionInfo(sec))
	}

	return info
}

// InspectSchemaFile parses and validates a schema document, returning its inspection metadata.
func InspectSchemaFile(path string) (*SchemaInfo, error) {
	s, err := LoadAndValidateSchema(path)
	if err != nil {
		return nil, err
	}
	return InspectSchema(s), nil
}

// LoadAndValidateSchema loads a declarative schema document and verifies generator constraints.
func LoadAndValidateSchema(path string) (*m.Schema, error) {
	if path == "" {
		return nil, fmt.Errorf("schema file path required")
	}

	var s *m.Schema
	var err error
	if path == "-" {
		s, err = m.ParseSchemaNamed(os.Stdin, "stdin")
		if err != nil {
			return nil, fmt.Errorf("parse schema from stdin: %w", err)
		}
	} else {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schema file %s: %w", path, err)
		}
		s, err = m.ParseSchemaNamed(bytes.NewReader(data), path)
		if err != nil {
			return nil, fmt.Errorf("parse schema %s: %w", path, err)
		}
	}

	if err := Validate(*s); err != nil {
		return nil, fmt.Errorf("validate schema: %w", err)
	}

	return s, nil
}

func formatFieldText(b *strings.Builder, f SchemaFieldInfo, indent string) {
	var flags []string
	if f.Required {
		flags = append(flags, "required")
	}
	if f.Sensitive {
		flags = append(flags, "sensitive")
	}
	if f.RuntimeOnly {
		flags = append(flags, "runtime_only")
	}
	if f.InputOnly {
		flags = append(flags, "input_only")
	}
	if f.Deprecated {
		flags = append(flags, "deprecated")
	}
	if f.Policy != "" {
		flags = append(flags, "policy="+f.Policy)
	}
	if f.Environment != "" {
		flags = append(flags, "env="+f.Environment)
	}

	flagStr := ""
	if len(flags) > 0 {
		flagStr = " [" + strings.Join(flags, ", ") + "]"
	}

	defStr := ""
	if f.Default != "" {
		defStr = fmt.Sprintf(" (default: %s)", f.Default)
	}

	fmt.Fprintf(b, "%s• %s -> %s: %s%s%s\n", indent, f.Key, f.GoName, f.GoType, defStr, flagStr)

	for _, doc := range f.Documentation {
		fmt.Fprintf(b, "%s    ## %s\n", indent, doc)
	}
	for _, ex := range f.Examples {
		fmt.Fprintf(b, "%s    example: %s\n", indent, ex)
	}
}

func formatSectionText(b *strings.Builder, sec SchemaSectionInfo, indent string) {
	rep := ""
	if sec.Repeated {
		rep = " [repeated]"
	}
	fmt.Fprintf(b, "%sSection: %s (Go: %s %s)%s\n", indent, sec.Name, sec.GoName, sec.GoType, rep)
	for _, doc := range sec.Documentation {
		fmt.Fprintf(b, "%s  ## %s\n", indent, doc)
	}

	for _, f := range sec.Fields {
		formatFieldText(b, f, indent+"  ")
	}
	for _, ch := range sec.Children {
		formatSectionText(b, ch, indent+"  ")
	}
	if indent == "" {
		fmt.Fprintln(b)
	}
}

// FormatText formats the schema metadata into a concise, readable textual outline.
func (info *SchemaInfo) FormatText() string {
	if info == nil {
		return ""
	}

	var b strings.Builder
	id := info.Identity
	if id == "" {
		id = "(anonymous)"
	}
	fmt.Fprintf(&b, "Schema: %s (version %d)\n\n", id, info.Version)

	if len(info.RootFields) > 0 {
		fmt.Fprintf(&b, "Root Fields:\n")
		for _, f := range info.RootFields {
			formatFieldText(&b, f, "  ")
		}
		fmt.Fprintln(&b)
	}

	for _, sec := range info.Sections {
		formatSectionText(&b, sec, "")
	}

	return b.String()
}

// JSON returns an indented JSON representation of the schema metadata.
func (info *SchemaInfo) JSON() ([]byte, error) {
	return json.MarshalIndent(info, "", "  ")
}
