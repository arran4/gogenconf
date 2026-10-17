package codegen

import (
	"fmt"
	"strings"
	"unicode"

	m "github.com/arran4/gogenconf"
)

type parserDefinition struct {
	Name   string
	Symbol string
	GoType string
}

func parserDefinitions(s m.Schema) ([]parserDefinition, error) {
	seen := map[string]parserDefinition{}
	add := func(f m.Field) error {
		if f.Parser == "" || f.InputOnly || f.RuntimeOnly {
			return nil
		}
		symbol, err := parserSymbol(f.Parser)
		if err != nil {
			return err
		}
		goType, _ := m.ParseType(f.GoType)
		goType = strings.TrimLeft(goType, "*")
		def := parserDefinition{Name: f.Parser, Symbol: symbol, GoType: goType}
		if previous, ok := seen[symbol]; ok {
			if previous.Name != def.Name || previous.GoType != def.GoType {
				return fmt.Errorf("parser %q has incompatible generated adapter types %s and %s", f.Parser, previous.GoType, def.GoType)
			}
			return nil
		}
		seen[symbol] = def
		return nil
	}
	for _, f := range s.RootFields {
		if err := add(f); err != nil {
			return nil, err
		}
	}
	var walk func(m.SectionDefinition) error
	walk = func(sec m.SectionDefinition) error {
		for _, f := range sec.Fields {
			if err := add(f); err != nil {
				return err
			}
		}
		for _, child := range sec.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, sec := range s.Sections {
		if err := walk(sec); err != nil {
			return nil, err
		}
	}
	defs := make([]parserDefinition, 0, len(seen))
	for _, def := range seen {
		defs = append(defs, def)
	}
	// Stable output is part of generated API compatibility.
	for i := range defs {
		for j := i + 1; j < len(defs); j++ {
			if defs[j].Symbol < defs[i].Symbol {
				defs[i], defs[j] = defs[j], defs[i]
			}
		}
	}
	return defs, nil
}

// parserSymbol maps a configuration-facing parser name to the exported Go
// symbol used by generated adapters. Parser names deliberately are not Go
// identifiers: they are schema data, while generated names are implementation
// details. The mapping is deterministic and callers must reject collisions.
func parserSymbol(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("empty parser name")
	}
	var b strings.Builder
	upper := true
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if upper {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(r)
			}
			upper = false
			continue
		}
		upper = true
	}
	s := b.String()
	if s == "" || unicode.IsDigit(rune(s[0])) {
		return "", fmt.Errorf("parser name %q has no usable Go symbol", name)
	}
	return s, nil
}
