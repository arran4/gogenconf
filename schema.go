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
	for _, def := range s.Sections {
		names := []string{def.Name}
		if def.Repeated {
			names = nil
			for _, name := range def.ExampleNames {
				names = append(names, def.Name+" "+name)
			}
		}
		for _, name := range names {
			section := &Section{Name: name}
			for _, text := range def.Documentation {
				section.Items = append(section.Items, Comment{Kind: DocumentationComment, Text: text})
			}
			section.Items = append(section.Items, Blank{})
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

type Schema struct {
	Version  int
	Sections []SectionDefinition
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
	for _, def := range s.Sections {
		var sections []*Section
		if def.Repeated {
			for _, section := range d.Sections {
				if strings.HasPrefix(section.Name, def.Name+" ") {
					sections = append(sections, section)
				}
			}
		} else {
			section := d.Section(def.Name)
			if section == nil {
				section = &Section{Name: def.Name, Origin: OriginEnriched}
				if len(d.Sections) > 0 {
					d.Items = append(d.Items, Blank{})
				}
				d.AddSection(section)
			}
			sections = append(sections, section)
		}
		for _, section := range sections {
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
		}
	}
	return nil
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
	for _, def := range s.Sections {
		if def.Repeated {
			for _, section := range d.Sections {
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
							Path:   section.Name + "." + f.Key,
							Source: section.Span,
							Err:    fmt.Errorf("required field %q is missing", f.Key),
						}
					}
				}
			}
		} else {
			section := d.Section(def.Name)
			if section == nil {
				for _, f := range def.Fields {
					if f.Required {
						return &ConfigError{
							Stage: StageValidate,
							Path:  def.Name + "." + f.Key,
							Err:   fmt.Errorf("required section %q is missing", def.Name),
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
						Path:   def.Name + "." + f.Key,
						Source: section.Span,
						Err:    fmt.Errorf("required field %q is missing", f.Key),
					}
				}
			}
		}
	}
	return nil
}

// Field looks up accepted persisted input, including InputOnly compatibility
// fields. Readers, migrations, inspection, and removal may use this lookup.
func (s Schema) Field(section, key string) (Field, bool) {
	for _, sec := range s.Sections {
		if sec.Name == section || sec.Repeated && strings.HasPrefix(section, sec.Name+" ") {
			for _, f := range sec.Fields {
				if f.Key == key && !f.RuntimeOnly {
					return f, true
				}
			}
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
