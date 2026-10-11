package gogenconf

import (
	"fmt"
	"sort"
)

// LayerPriority defines standard layer priorities for document composition.
type LayerPriority int

const (
	PriorityDefault LayerPriority = 100
	PrioritySystem  LayerPriority = 200
	PriorityUser    LayerPriority = 300
	PriorityCLI     LayerPriority = 400
)

// SourceLayer encapsulates an input document layer to be composed.
type SourceLayer struct {
	Name     string        // e.g. "00-defaults.conf", "10-system.conf", "99-user.conf"
	Document *Document     // parsed native document AST
	Priority LayerPriority // higher priority layers override lower priority layers
	Order    int           // stable order within the same priority
	Trusted  bool          // whether this layer is caller-certified as trusted
}

// CompositionOptions specifies compatibility constraints and parameters for composition.
type CompositionOptions struct {
	ExpectedIdentity string
	MaxVersion       int
}

// Compose combines multiple ordered source layers into an effective document.
// Input source documents are never mutated. Provenance (Span and Source) is preserved
// on individual effective AST items.
func Compose(layers []SourceLayer, opts CompositionOptions) (*Document, error) {
	if len(layers) == 0 {
		return &Document{
			Identity:   opts.ExpectedIdentity,
			Version:    opts.MaxVersion,
			HasVersion: opts.MaxVersion > 0,
		}, nil
	}

	// Stable sort by Priority ascending, then Order ascending
	sorted := make([]SourceLayer, len(layers))
	copy(sorted, layers)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].Order < sorted[j].Order
	})

	// Verify compatibility constraints across all input layers
	for _, l := range sorted {
		if l.Document == nil {
			return nil, fmt.Errorf("layer %q has nil document", l.Name)
		}
		if opts.ExpectedIdentity != "" || opts.MaxVersion > 0 {
			if err := l.Document.CheckCompatibility(opts.ExpectedIdentity, opts.MaxVersion); err != nil {
				return nil, fmt.Errorf("layer %q compatibility error: %w", l.Name, err)
			}
		}
	}

	first := sorted[0].Document
	effective := &Document{
		Identity:   first.Identity,
		Version:    first.Version,
		HasVersion: first.HasVersion,
		HeaderSpan: first.HeaderSpan,
	}

	for _, layer := range sorted {
		mergeDocumentInto(effective, layer.Document)
	}

	return effective, nil
}

func mergeDocumentInto(target, source *Document) {
	if target == nil || source == nil {
		return
	}

	// Header metadata updates from later layers if target was unversioned
	if !target.HasVersion && source.HasVersion {
		target.Identity = source.Identity
		target.Version = source.Version
		target.HasVersion = true
		target.HeaderSpan = source.HeaderSpan
	}

	// Merge root entries
	for _, item := range source.Items {
		switch e := item.(type) {
		case Entry:
			if e.Value == nil {
				target.Delete(e.Key)
			} else if oldEntry, idx, found := target.Entry(e.Key); found {
				oldMap, oldIsMap := oldEntry.Value.(Map)
				newMap, newIsMap := e.Value.(Map)
				if oldIsMap && newIsMap {
					target.Items[idx] = Entry{
						Key:    e.Key,
						Value:  mergeMaps(oldMap, newMap),
						Span:   e.Span,
						Origin: OriginAuthored,
					}
				} else {
					target.Items[idx] = Entry{
						Key:    e.Key,
						Value:  CloneExpr(e.Value),
						Span:   e.Span,
						Origin: OriginAuthored,
					}
				}
			} else {
				target.Items = append(target.Items, Entry{
					Key:    e.Key,
					Value:  CloneExpr(e.Value),
					Span:   e.Span,
					Origin: OriginAuthored,
				})
			}
		}
	}

	// Merge sections
	for _, sec := range source.Sections {
		targetSec := target.Section(sec.Name)
		if targetSec == nil {
			target.AddSection(sec.Clone())
		} else {
			mergeSection(targetSec, sec)
		}
	}
}

func mergeSection(target, source *Section) {
	if target == nil || source == nil {
		return
	}

	for _, item := range source.Items {
		switch e := item.(type) {
		case Entry:
			if e.Value == nil {
				target.Delete(e.Key)
			} else if oldEntry, idx, found := target.Entry(e.Key); found {
				oldMap, oldIsMap := oldEntry.Value.(Map)
				newMap, newIsMap := e.Value.(Map)
				if oldIsMap && newIsMap {
					target.Items[idx] = Entry{
						Key:    e.Key,
						Value:  mergeMaps(oldMap, newMap),
						Span:   e.Span,
						Origin: OriginAuthored,
					}
				} else {
					target.Items[idx] = Entry{
						Key:    e.Key,
						Value:  CloneExpr(e.Value),
						Span:   e.Span,
						Origin: OriginAuthored,
					}
				}
			} else {
				target.Items = append(target.Items, Entry{
					Key:    e.Key,
					Value:  CloneExpr(e.Value),
					Span:   e.Span,
					Origin: OriginAuthored,
				})
			}
		}
	}

	// Recursively merge child sections
	for _, sub := range source.Sections {
		targetSub := target.Section(sub.Name)
		if targetSub == nil {
			target.AddSection(sub.Clone())
		} else {
			mergeSection(targetSub, sub)
		}
	}
}

func mergeMaps(base, override Map) Map {
	out := Map{
		Span:   override.Span,
		Origin: OriginAuthored,
	}
	keyIdx := map[string]int{}

	for _, en := range base.Entries {
		out.Entries = append(out.Entries, MapEntry{
			Key:     en.Key,
			Value:   CloneExpr(en.Value),
			KeySpan: en.KeySpan,
			Span:    en.Span,
			Origin:  en.Origin,
		})
		keyIdx[en.Key] = len(out.Entries) - 1
	}

	for _, en := range override.Entries {
		if idx, exists := keyIdx[en.Key]; exists {
			out.Entries[idx] = MapEntry{
				Key:     en.Key,
				Value:   CloneExpr(en.Value),
				KeySpan: en.KeySpan,
				Span:    en.Span,
				Origin:  en.Origin,
			}
		} else {
			out.Entries = append(out.Entries, MapEntry{
				Key:     en.Key,
				Value:   CloneExpr(en.Value),
				KeySpan: en.KeySpan,
				Span:    en.Span,
				Origin:  en.Origin,
			})
			keyIdx[en.Key] = len(out.Entries) - 1
		}
	}

	return out
}
