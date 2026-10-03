package configmodel

import (
	"fmt"
	"strings"
)

type ResolutionPolicy string

const (
	Eager          ResolutionPolicy = ""
	ProviderBacked ResolutionPolicy = "provider"
)

// Field describes content and static Go mapping. RuntimeOnly fields belong to
// the generated struct but cannot be configured through the document.
type Field struct {
	Key, GoName, GoType                          string
	Default                                      Expr
	Documentation                                []string
	Required, Sensitive, RuntimeOnly, Deprecated bool
	Policy                                       ResolutionPolicy
	// Environment is a compatibility fallback below an explicit document value.
	Environment         string
	EnvironmentNonEmpty bool
}
type SectionDefinition struct {
	Name, GoName, GoType string
	Documentation        []string
	Fields               []Field
	// Repeated sections use "Name instance", generating a slice and Name field.
	Repeated bool
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
		return fmt.Errorf("configuration must be migrated to version %d before enrichment", s.Version)
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
				section = &Section{Name: def.Name}
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
				if f.RuntimeOnly || f.Deprecated {
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
					section.Items = append(section.Items, Entry{Key: f.Key, Value: CloneExpr(f.Default), Defaulted: true})
				}
			}
		}
	}
	return nil
}

func (s Schema) CheckVersion(d *Document) error {
	if d == nil {
		return fmt.Errorf("nil configuration document")
	}
	if d.Version < 0 {
		return fmt.Errorf("invalid configuration version")
	}
	if d.HasVersion && d.Version > s.Version {
		return fmt.Errorf("configuration version %d is newer than supported version %d", d.Version, s.Version)
	}
	return nil
}
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
