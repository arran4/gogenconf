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
	for _, sec := range s.Sections {
		for _, f := range sec.Fields {
			if f.Parser == "" || f.InputOnly || f.RuntimeOnly {
				continue
			}
			symbol, err := parserSymbol(f.Parser)
			if err != nil {
				return nil, err
			}
			def := parserDefinition{Name: f.Parser, Symbol: symbol, GoType: f.ResolvedGoType()}
			if previous, ok := seen[symbol]; ok {
				if previous.Name != def.Name || previous.GoType != def.GoType {
					return nil, fmt.Errorf("parser %q has incompatible generated adapter types %s and %s", f.Parser, previous.GoType, def.GoType)
				}
				continue
			}
			seen[symbol] = def
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
