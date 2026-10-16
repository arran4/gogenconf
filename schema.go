package gogenconf

import (
	"errors"
	"fmt"
	"strings"
)

type ResolutionPolicy string

const (
	Eager          ResolutionPolicy = ""
	ProviderBacked ResolutionPolicy = "provider"
)

// CollectionKind identifies whether a schema field represents a scalar,
// an ordered collection ([]T), or a string-keyed map ([$]T).
type CollectionKind string

const (
	CollectionNone    CollectionKind = ""
	CollectionOrdered CollectionKind = "list" // []T
	CollectionKeyed   CollectionKind = "map"  // [$]T: string-keyed map ($ is shorthand for string key)
)

// ParseType inspects a schema type signature and classifies its collection kind
// and element type. In gogenconf schema conventions:
//   - "[]T" indicates an ordered collection of T
//   - "[$]T" or "map[string]T" indicates a string-keyed map of T ($ is shorthand for string key)
func ParseType(goType string) (elemType string, kind CollectionKind) {
	trimmed := strings.TrimSpace(goType)
	if strings.HasPrefix(trimmed, "[$]") {
		return strings.TrimSpace(trimmed[3:]), CollectionKeyed
	}
	if strings.HasPrefix(trimmed, "map[string]") {
		return strings.TrimSpace(trimmed[len("map[string]"):len(trimmed)]), CollectionKeyed
	}
	if strings.HasPrefix(trimmed, "[]") && !strings.HasPrefix(trimmed, "[]byte") {
		return strings.TrimSpace(trimmed[2:]), CollectionOrdered
	}
	return trimmed, CollectionNone
}

// CanonicalType formats an element type and collection kind into standard gogenconf notation.
func CanonicalType(elemType string, kind CollectionKind) string {
	switch kind {
	case CollectionOrdered:
		return "[]" + elemType
	case CollectionKeyed:
		return "[$]" + elemType
	default:
		return elemType
	}
}

// Field describes content and static Go mapping. RuntimeOnly fields belong to
// the generated struct but cannot be configured through the document.
type Field struct {
	Key, GoName, GoType string
	Default             Expr
	// Examples are documentation only: never defaults or runtime inputs.
	Examples                                     []Expr
	Documentation                                []string
	Required, Sensitive, RuntimeOnly, Deprecated bool
	// InputOnly fields are accepted persisted inputs for compatibility/migration,
	// not writable editing targets, runtime members, or ordinary binding targets.
	InputOnly bool
	Policy    ResolutionPolicy
	// Environment is a compatibility fallback below an explicit document value.
	Environment         string
	EnvironmentNonEmpty bool
}

// IsCollection reports whether the field is an ordered list or keyed map.
func (f Field) IsCollection() bool {
	_, kind := ParseType(f.GoType)
	return kind != CollectionNone
}

// CollectionKind returns the collection kind of this field.
func (f Field) CollectionKind() CollectionKind {
	_, kind := ParseType(f.GoType)
	return kind
}

// ElementType returns the element type if this field is a collection.
func (f Field) ElementType() string {
	elem, _ := ParseType(f.GoType)
	return elem
}

// ResolvedGoType returns the standard Go type representation for code generation.
func (f Field) ResolvedGoType() string {
	elem, kind := ParseType(f.GoType)
	switch kind {
	case CollectionKeyed:
		return "map[string]" + elem
	case CollectionOrdered:
		return "[]" + elem
	default:
		return f.GoType
	}
}

type SectionDefinition struct {
	Name, GoName, GoType string
	Documentation        []string
	Fields               []Field
	// Children describes schema-defined nested native objects.  It is a
	// recursive structural declaration, not a flattened dotted path: each
	// child is bound against the corresponding Document.Section subtree.
	Children []SectionDefinition
	// Repeated sections use "Name instance", generating a slice and Name field.
	Repeated bool
	// ExampleNames illustrate repeated sections without creating runtime defaults.
	ExampleNames []string
}

// Sample renders defaults as active entries and examples as managed comments.
// Unlike Seed it also illustrates optional fields and repeated sections, but
// does not activate their examples. All content comes from schema metadata.
func (s Schema) Sample() *Document {
	d := &Document{Version: s.Version, HasVersion: true, Items: []Item{Version{}, Blank{},
		Comment{Kind: UserComment, Text: "Single # comments belong to you; ## documentation is schema-managed."},
		Comment{Kind: DocumentationComment, Text: "Examples below are comments, not active values or defaults."}, Blank{}}}
	for _, f := range s.RootFields {
		if f.RuntimeOnly || f.InputOnly || f.Deprecated {
			continue
		}
		for _, text := range f.Documentation {
			d.Items = append(d.Items, Comment{Kind: DocumentationComment, Text: text})
		}
		if len(f.Examples) > 0 {
			d.Items = append(d.Items, Comment{Kind: DocumentationComment, Text: "Examples (inactive):"})
			for _, e := range f.Examples {
				d.Items = append(d.Items, Comment{Kind: DocumentationComment, Text: "  " + f.Key + " " + FormatExpr(e)})
			}
		}
		if f.Default != nil {
			d.Set(f.Key, CloneExpr(f.Default))
		}
		d.Items = append(d.Items, Blank{})
	}
	for _, def := range s.Sections {
		names := []string{def.Name}
		if def.Repeated {
			names = nil
			for _, name := range def.ExampleNames {
				names = append(names, def.Name+" "+name)
			}
			if len(names) == 0 {
				names = append(names, def.Name+" default")
			}
		}
		for _, name := range names {
			section := sampleSectionDefinition(def, name)
			if def.Repeated {
				// Comment the entire repeated section: examples must not create
				// an empty provider/config instance just by running the sample.
				tmp := &Document{}
				tmp.AddSection(section)
				for _, line := range strings.Split(strings.TrimSuffix(Format(tmp), "\n"), "\n") {
					text := ""
					if line != "" {
						text = "  " + line
					}
					d.Items = append(d.Items, Comment{Kind: DocumentationComment, Text: text})
				}
			} else {
				d.AddSection(section)
			}
			d.Items = append(d.Items, Blank{})
		}
	}
	if len(d.Items) > 0 {
		if _, ok := d.Items[len(d.Items)-1].(Blank); ok {
			d.Items = d.Items[:len(d.Items)-1]
		}
	}
	return d
}

func sampleSectionDefinition(def SectionDefinition, name string) *Section {
	section := &Section{Name: name}
	for _, text := range def.Documentation {
		section.Items = append(section.Items, Comment{Kind: DocumentationComment, Text: text})
	}
	if len(def.Documentation) > 0 {
		section.Items = append(section.Items, Blank{})
	}
	for _, f := range def.Fields {
		if f.RuntimeOnly || f.InputOnly || f.Deprecated {
			continue
		}
		for _, text := range f.Documentation {
			section.Items = append(section.Items, Comment{Kind: DocumentationComment, Text: text})
		}
		if len(f.Examples) > 0 {
			section.Items = append(section.Items, Comment{Kind: DocumentationComment, Text: "Examples (inactive):"})
			for _, e := range f.Examples {
				section.Items = append(section.Items, Comment{Kind: DocumentationComment, Text: "  " + f.Key + " " + FormatExpr(e)})
			}
		}
		if f.Default != nil {
			section.Set(f.Key, CloneExpr(f.Default))
		}
		section.Items = append(section.Items, Blank{})
	}
	for _, child := range def.Children {
		names := []string{child.Name}
		if child.Repeated {
			names = nil
			for _, exName := range child.ExampleNames {
				names = append(names, child.Name+" "+exName)
			}
			if len(names) == 0 {
				names = append(names, child.Name+" default")
			}
		}
		for _, chName := range names {
			childSec := sampleSectionDefinition(child, chName)
			section.AddSection(childSec)
			section.Items = append(section.Items, Blank{})
		}
	}
	return section
}

type Schema struct {
	Identity string
	Version  int
	// RootFields are scalar/collection declarations stored directly on the
	// Document rather than in a Section. They deliberately use the same Field
	// contract as section fields.
	RootFields []Field
	Sections   []SectionDefinition
}

func (s Schema) Seed() *Document {
	d := &Document{Version: s.Version, HasVersion: true, Items: []Item{Version{}, Blank{}}}
	_ = s.Enrich(d)
	return d
}

// refreshDocs replaces only managed comments within the associated comment
// block. User comments are retained even when interspersed with managed docs.
func refreshDocs(items []Item, start, end int, docs []string) []Item {
	block := make([]Item, 0, end-start+len(docs))
	for _, i := range items[start:end] {
		if c, ok := i.(Comment); !ok || c.Kind == UserComment {
			block = append(block, i)
		}
	}
	for _, text := range docs {
		block = append(block, Comment{Kind: DocumentationComment, Text: text})
	}
	result := append([]Item{}, items[:start]...)
	result = append(result, block...)
	return append(result, items[end:]...)
}

func (s Schema) Enrich(d *Document) error {
	if err := s.CheckVersion(d); err != nil {
		return err
	}
	if !d.HasVersion || d.Version != s.Version {
		return &ConfigError{
			Stage: StageValidate,
			Err:   fmt.Errorf("configuration must be migrated to version %d before enrichment", s.Version),
		}
	}
	// Enrich RootFields
	for _, f := range s.RootFields {
		if f.RuntimeOnly || f.InputOnly || f.Deprecated {
			continue
		}
		if _, idx, exists := d.Entry(f.Key); exists {
			start := idx
			for start > 0 {
				if _, ok := d.Items[start-1].(Comment); !ok {
					break
				}
				start--
			}
			d.Items = refreshDocs(d.Items, start, idx, f.Documentation)
		} else if f.Default != nil {
			for _, text := range f.Documentation {
				d.Items = append(d.Items, Comment{Kind: DocumentationComment, Text: text})
			}
			d.Items = append(d.Items, Entry{
				Key:       f.Key,
				Value:     WithOrigin(CloneExpr(f.Default), OriginDefaulted),
				Defaulted: true,
				Origin:    OriginDefaulted,
			})
		}
	}
	enrichSectionDefinitions(s.Sections, d.Sections, d)
	return nil
}

func enrichSection(def SectionDefinition, section *Section) {
	// Section docs occupy the initial block ending at a blank line. This
	// delimiter disambiguates them from the first field's documentation.
	end := 0
	for end < len(section.Items) {
		if _, ok := section.Items[end].(Comment); !ok {
			break
		}
		end++
	}
	hasBoundary := end < len(section.Items)
	if hasBoundary {
		_, hasBoundary = section.Items[end].(Blank)
	}
	if hasBoundary {
		section.Items = refreshDocs(section.Items, 0, end, def.Documentation)
	} else if len(def.Documentation) > 0 {
		prefix := []Item{}
		for _, text := range def.Documentation {
			prefix = append(prefix, Comment{Kind: DocumentationComment, Text: text})
		}
		prefix = append(prefix, Blank{})
		section.Items = append(prefix, section.Items...)
	}
	for _, f := range def.Fields {
		if f.RuntimeOnly || f.InputOnly || f.Deprecated {
			continue
		}
		if _, idx, exists := section.Entry(f.Key); exists {
			start := idx
			for start > 0 {
				if _, ok := section.Items[start-1].(Comment); !ok {
					break
				}
				start--
			}
			section.Items = refreshDocs(section.Items, start, idx, f.Documentation)
		} else if f.Default != nil {
			for _, text := range f.Documentation {
				section.Items = append(section.Items, Comment{Kind: DocumentationComment, Text: text})
			}
			section.Items = append(section.Items, Entry{
				Key:       f.Key,
				Value:     WithOrigin(CloneExpr(f.Default), OriginDefaulted),
				Defaulted: true,
				Origin:    OriginDefaulted,
			})
		}
	}
	if len(def.Children) > 0 {
		enrichSectionDefinitions(def.Children, section.Sections, nil)
	}
}

func enrichSectionDefinitions(defs []SectionDefinition, sections []*Section, doc *Document) {
	for _, def := range defs {
		if def.Repeated {
			// Legacy repeated sections: "def.Name instance"
			for _, section := range sections {
				if strings.HasPrefix(section.Name, def.Name+" ") {
					enrichSection(def, section)
				}
			}
			// Canonical brace container: container "def.Name" with children
			for _, section := range sections {
				if section.Name == def.Name && section.Brace {
					for _, inst := range section.Sections {
						enrichSection(def, inst)
					}
				}
			}
		} else {
			var section *Section
			for _, s := range sections {
				if s.Name == def.Name {
					section = s
					break
				}
			}
			if section == nil && doc != nil {
				section = &Section{Name: def.Name, Origin: OriginEnriched}
				if len(doc.Sections) > 0 {
					doc.Items = append(doc.Items, Blank{})
				}
				doc.AddSection(section)
			}
			if section != nil {
				enrichSection(def, section)
			}
		}
	}
}

func (s Schema) CheckVersion(d *Document) error {
	if d == nil {
		return &ConfigError{
			Stage: StageValidate,
			Err:   errors.New("nil configuration document"),
		}
	}
	if d.Version < 0 {
		return &ConfigError{
			Stage: StageValidate,
			Err:   errors.New("invalid configuration version"),
		}
	}
	if d.HasVersion && d.Version > s.Version {
		return &ConfigError{
			Stage: StageValidate,
			Err:   fmt.Errorf("configuration version %d is newer than supported version %d", d.Version, s.Version),
		}
	}
	return nil
}

// Validate verifies that the document conforms to the schema's version and required fields,
// returning structured ConfigErrors with StageValidate, Path, and Source span when available.
func (s Schema) Validate(d *Document) error {
	if err := s.CheckVersion(d); err != nil {
		return err
	}
	for _, f := range s.RootFields {
		if f.Required {
			entry, _, exists := d.Entry(f.Key)
			if !exists || entry.Value == nil {
				return &ConfigError{
					Stage:  StageValidate,
					Path:   f.Key,
					Source: d.HeaderSpan,
					Err:    fmt.Errorf("required field %q is missing", f.Key),
				}
			}
		}
	}
	return validateSectionDefinitions(s.Sections, d.Sections, "", d.HeaderSpan)
}

func validateSectionDefinitions(defs []SectionDefinition, sections []*Section, pathPrefix string, parentSpan Span) error {
	for _, def := range defs {
		if def.Repeated {
			// Legacy repeated sections: "def.Name instance"
			for _, section := range sections {
				if !strings.HasPrefix(section.Name, def.Name+" ") {
					continue
				}
				for _, f := range def.Fields {
					if !f.Required {
						continue
					}
					entry, _, exists := section.Entry(f.Key)
					if !exists || entry.Value == nil {
						return &ConfigError{
							Stage:  StageValidate,
							Path:   pathPrefix + section.Name + "." + f.Key,
							Source: section.Span,
							Err:    fmt.Errorf("required field %q is missing", f.Key),
						}
					}
				}
				if len(def.Children) > 0 {
					if err := validateSectionDefinitions(def.Children, section.Sections, pathPrefix+section.Name+".", section.Span); err != nil {
						return err
					}
				}
			}
			// Canonical brace repeated sections: container "def.Name" with children
			for _, section := range sections {
				if section.Name != def.Name || !section.Brace {
					continue
				}
				for _, inst := range section.Sections {
					instPath := pathPrefix + def.Name + "." + inst.Name
					for _, f := range def.Fields {
						if !f.Required {
							continue
						}
						entry, _, exists := inst.Entry(f.Key)
						if !exists || entry.Value == nil {
							return &ConfigError{
								Stage:  StageValidate,
								Path:   instPath + "." + f.Key,
								Source: inst.Span,
								Err:    fmt.Errorf("required field %q is missing", f.Key),
							}
						}
					}
					if len(def.Children) > 0 {
						if err := validateSectionDefinitions(def.Children, inst.Sections, instPath+".", inst.Span); err != nil {
							return err
						}
					}
				}
			}
		} else {
			// Non-repeated section: search in sections
			var section *Section
			for _, s := range sections {
				if s.Name == def.Name {
					section = s
					break
				}
			}
			if section == nil {
				for _, f := range def.Fields {
					if f.Required {
						return &ConfigError{
							Stage:  StageValidate,
							Path:   pathPrefix + def.Name + "." + f.Key,
							Source: parentSpan,
							Err:    fmt.Errorf("required section %q is missing", def.Name),
						}
					}
				}
				if len(def.Children) > 0 {
					for _, ch := range def.Children {
						for _, f := range ch.Fields {
							if f.Required {
								return &ConfigError{
									Stage:  StageValidate,
									Path:   pathPrefix + def.Name + "." + ch.Name + "." + f.Key,
									Source: parentSpan,
									Err:    fmt.Errorf("required section %q is missing", def.Name),
								}
							}
						}
					}
				}
				continue
			}
			for _, f := range def.Fields {
				if !f.Required {
					continue
				}
				entry, _, exists := section.Entry(f.Key)
				if !exists || entry.Value == nil {
					return &ConfigError{
						Stage:  StageValidate,
						Path:   pathPrefix + def.Name + "." + f.Key,
						Source: section.Span,
						Err:    fmt.Errorf("required field %q is missing", f.Key),
					}
				}
			}
			if len(def.Children) > 0 {
				if err := validateSectionDefinitions(def.Children, section.Sections, pathPrefix+def.Name+".", section.Span); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s Schema) findSectionDef(section string) *SectionDefinition {
	var tokens []string
	for _, part := range strings.Split(section, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for _, sub := range strings.Fields(part) {
			tokens = append(tokens, sub)
		}
	}
	if len(tokens) == 0 {
		return nil
	}
	return matchSectionDef(s.Sections, tokens)
}

func matchSectionDef(defs []SectionDefinition, tokens []string) *SectionDefinition {
	if len(tokens) == 0 {
		return nil
	}
	for i := range defs {
		def := &defs[i]
		if def.Name == tokens[0] {
			if len(tokens) == 1 {
				return def
			}
			rest := tokens[1:]
			if def.Repeated && len(rest) > 0 {
				isChild := false
				for _, ch := range def.Children {
					if ch.Name == rest[0] {
						isChild = true
						break
					}
				}
				if !isChild {
					rest = rest[1:]
				}
			}
			if len(rest) == 0 {
				return def
			}
			if childMatch := matchSectionDef(def.Children, rest); childMatch != nil {
				return childMatch
			}
		}
	}
	return nil
}

// Field looks up accepted persisted input, including InputOnly compatibility
// fields. Readers, migrations, inspection, and removal may use this lookup.
func (s Schema) Field(section, key string) (Field, bool) {
	if section == "" || section == "." {
		for _, f := range s.RootFields {
			if f.Key == key && !f.RuntimeOnly {
				return f, true
			}
		}
		return Field{}, false
	}
	def := s.findSectionDef(section)
	if def == nil {
		return Field{}, false
	}
	for _, f := range def.Fields {
		if f.Key == key && !f.RuntimeOnly {
			return f, true
		}
	}
	return Field{}, false
}

// WritableField identifies ordinary editing targets. InputOnly fields remain
// readable but cannot be introduced or replaced by current-schema editors.
func (s Schema) WritableField(section, key string) (Field, bool) {
	f, ok := s.Field(section, key)
	return f, ok && !f.InputOnly
}

func (s Schema) IsSensitive(section, key string) bool {
	f, ok := s.Field(section, key)
	return ok && f.Sensitive
}

func (s Schema) IsRequired(section, key string) bool {
	f, ok := s.Field(section, key)
	return ok && f.Required
}

func (s Schema) IsInputOnly(section, key string) bool {
	f, ok := s.Field(section, key)
	return ok && f.InputOnly
}

func (s Schema) IsWritable(section, key string) bool {
	_, ok := s.WritableField(section, key)
	return ok
}
