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

// CompositionWarning describes a warning encountered during composition with lifecycle stage,
// target path, mutation operation, message, and source span.
type CompositionWarning struct {
	Stage     Stage
	Path      string
	Operation string
	Message   string
	Span      Span
}

func (w CompositionWarning) String() string {
	if w.Operation != "" {
		return fmt.Sprintf("[%s] %s %s: %s (at %v)", w.Stage, w.Operation, w.Path, w.Message, w.Span)
	}
	return fmt.Sprintf("[%s] %s: %s (at %v)", w.Stage, w.Path, w.Message, w.Span)
}

// CompositionOptions specifies compatibility constraints and parameters for composition.
type CompositionOptions struct {
	ExpectedIdentity string
	MaxVersion       int
	WarnCollector    func(warning CompositionWarning)
}

func (opts CompositionOptions) warn(path, operation, msg string, span Span) {
	if opts.WarnCollector != nil {
		opts.WarnCollector(CompositionWarning{
			Stage:     StageCompose,
			Path:      path,
			Operation: operation,
			Message:   msg,
			Span:      span,
		})
	}
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

	// Reject duplicate (Priority, Order) source-layer pairs
	seenPriorityOrder := make(map[[2]int]string, len(layers))
	for _, l := range layers {
		key := [2]int{int(l.Priority), l.Order}
		if prev, exists := seenPriorityOrder[key]; exists {
			return nil, fmt.Errorf("duplicate layer priority/order (%d, %d) between layer %q and %q", l.Priority, l.Order, prev, l.Name)
		}
		seenPriorityOrder[key] = l.Name
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
		mergeDocumentInto(effective, layer.Document, opts)
	}

	return effective, nil
}

func mergeDocumentInto(target, source *Document, opts CompositionOptions) {
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
			applyEntryMutation(&target.Items, e, e.Key, opts, target.Delete, target.DeleteSection)
		}
	}

	// Merge sections
	for _, sec := range source.Sections {
		mergeSectionIntoContainer(&target.Items, &target.Sections, sec, sec.Name, opts, target.DeleteSection, target.AddSection)
	}
}

func mergeSection(target, source *Section, currentPath string, opts CompositionOptions) {
	if target == nil || source == nil {
		return
	}

	for _, item := range source.Items {
		switch e := item.(type) {
		case Entry:
			applyEntryMutation(&target.Items, e, joinPath(currentPath, e.Key), opts, target.Delete, target.DeleteSection)
		}
	}

	// Recursively merge child sections
	for _, sub := range source.Sections {
		mergeSectionIntoContainer(&target.Items, &target.Sections, sub, joinPath(currentPath, sub.Name), opts, target.DeleteSection, target.AddSection)
	}
}

func mergeSectionIntoContainer(
	items *[]Item,
	sections *[]*Section,
	sec *Section,
	path string,
	opts CompositionOptions,
	deleteSec func(string) bool,
	addSec func(*Section),
) {
	findSec := func(name string) *Section {
		for _, s := range *sections {
			if s.Name == name {
				return s
			}
		}
		return nil
	}

	switch sec.Mutation {
	case MutationDelete:
		if !deleteSec(sec.Name) {
			opts.warn(path, "[-]", fmt.Sprintf("deletion target section %q not found", sec.Name), sec.Span)
		}
	case MutationReplace:
		deleteSec(sec.Name)
		cloned := sec.Clone()
		cloned.Mutation = MutationNone
		addSec(cloned)
	case MutationAdd:
		targetSec := findSec(sec.Name)
		if targetSec == nil {
			cloned := sec.Clone()
			cloned.Mutation = MutationNone
			addSec(cloned)
		} else {
			mergeSection(targetSec, sec, path, opts)
		}
	case MutationMutate:
		targetSec := findSec(sec.Name)
		if targetSec == nil {
			opts.warn(path, "[~]", fmt.Sprintf("mutation target section %q not found; constructing section", sec.Name), sec.Span)
			cloned := sec.Clone()
			cloned.Mutation = MutationNone
			addSec(cloned)
		} else {
			mergeSection(targetSec, sec, path, opts)
		}
	case MutationNone:
		targetSec := findSec(sec.Name)
		if targetSec == nil {
			targetSec = &Section{
				Name:   sec.Name,
				Brace:  sec.Brace,
				Span:   sec.Span,
				Origin: sec.Origin,
			}
			addSec(targetSec)
		}
		mergeSection(targetSec, sec, path, opts)
	}
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func applyEntryMutation(items *[]Item, e Entry, path string, opts CompositionOptions, deleteEntry func(string) bool, deleteSec func(string) bool) {
	switch e.Mutation {
	case MutationDelete:
		// [-] delete / remove
		delEntry := deleteEntry(e.Key)
		delSec := false
		if deleteSec != nil {
			delSec = deleteSec(e.Key)
		}
		if !delEntry && !delSec {
			opts.warn(path, "[-]", fmt.Sprintf("deletion target %q not found", e.Key), e.Span)
		}
	case MutationReplace:
		// [=] replace / overwrite
		setOrReplaceItem(items, Entry{
			Key:      e.Key,
			Mutation: MutationNone,
			Value:    CloneExpr(e.Value),
			Span:     e.Span,
			Origin:   OriginAuthored,
		})
	case MutationAdd:
		// [+] add / append / insert
		if idx := findEntryIndex(*items, e.Key); idx >= 0 {
			oldEntry := (*items)[idx].(Entry)
			(*items)[idx] = Entry{
				Key:      e.Key,
				Mutation: MutationNone,
				Value:    addValues(oldEntry.Value, e.Value, e.Span),
				Span:     e.Span,
				Origin:   OriginAuthored,
			}
		} else {
			// Automatic upsert
			*items = append(*items, Entry{
				Key:      e.Key,
				Mutation: MutationNone,
				Value:    CloneExpr(e.Value),
				Span:     e.Span,
				Origin:   OriginAuthored,
			})
		}
	case MutationMutate:
		// [~] mutate / inherit (NOT prepend!)
		if idx := findEntryIndex(*items, e.Key); idx >= 0 {
			oldEntry := (*items)[idx].(Entry)
			(*items)[idx] = Entry{
				Key:      e.Key,
				Mutation: MutationNone,
				Value:    mutateValues(oldEntry.Value, e.Value, e.Span),
				Span:     e.Span,
				Origin:   OriginAuthored,
			}
		} else {
			// For missing [~] targets, warn and construct value
			opts.warn(path, "[~]", fmt.Sprintf("mutation target %q not found; constructing value", e.Key), e.Span)
			*items = append(*items, Entry{
				Key:      e.Key,
				Mutation: MutationNone,
				Value:    CloneExpr(e.Value),
				Span:     e.Span,
				Origin:   OriginAuthored,
			})
		}
	case MutationNone:
		// Bare declaration
		if e.Value == nil {
			// Bare value-less entry is treated as deletion
			delEntry := deleteEntry(e.Key)
			delSec := false
			if deleteSec != nil {
				delSec = deleteSec(e.Key)
			}
			if !delEntry && !delSec {
				opts.warn(path, "[-]", fmt.Sprintf("deletion target %q not found", e.Key), e.Span)
			}
		} else if idx := findEntryIndex(*items, e.Key); idx >= 0 {
			oldEntry := (*items)[idx].(Entry)
			oldMap, oldIsMap := oldEntry.Value.(Map)
			newMap, newIsMap := e.Value.(Map)
			if oldIsMap && newIsMap {
				(*items)[idx] = Entry{
					Key:      e.Key,
					Mutation: MutationNone,
					Value:    mergeMaps(oldMap, newMap),
					Span:     e.Span,
					Origin:   OriginAuthored,
				}
			} else {
				// Bare scalar replaces!
				(*items)[idx] = Entry{
					Key:      e.Key,
					Mutation: MutationNone,
					Value:    CloneExpr(e.Value),
					Span:     e.Span,
					Origin:   OriginAuthored,
				}
			}
		} else {
			*items = append(*items, Entry{
				Key:      e.Key,
				Mutation: MutationNone,
				Value:    CloneExpr(e.Value),
				Span:     e.Span,
				Origin:   OriginAuthored,
			})
		}
	}
}

func findEntryIndex(items []Item, key string) int {
	for i, it := range items {
		if e, ok := it.(Entry); ok && e.Key == key {
			return i
		}
	}
	return -1
}

func setOrReplaceItem(items *[]Item, entry Entry) {
	if idx := findEntryIndex(*items, entry.Key); idx >= 0 {
		(*items)[idx] = entry
	} else {
		*items = append(*items, entry)
	}
}

func addValues(base, extra Expr, span Span) Expr {
	if base == nil {
		return CloneExpr(extra)
	}
	if extra == nil {
		return CloneExpr(base)
	}

	baseList, baseIsList := base.(List)
	extraList, extraIsList := extra.(List)

	if baseIsList && extraIsList {
		out := List{
			Items:  make([]Expr, 0, len(baseList.Items)+len(extraList.Items)),
			Span:   span,
			Origin: OriginAuthored,
		}
		for _, it := range baseList.Items {
			out.Items = append(out.Items, CloneExpr(it))
		}
		for _, it := range extraList.Items {
			out.Items = append(out.Items, CloneExpr(it))
		}
		return out
	}
	if baseIsList {
		out := List{
			Items:  make([]Expr, 0, len(baseList.Items)+1),
			Span:   span,
			Origin: OriginAuthored,
		}
		for _, it := range baseList.Items {
			out.Items = append(out.Items, CloneExpr(it))
		}
		out.Items = append(out.Items, CloneExpr(extra))
		return out
	}

	baseMap, baseIsMap := base.(Map)
	extraMap, extraIsMap := extra.(Map)
	if baseIsMap && extraIsMap {
		return mergeMaps(baseMap, extraMap)
	}

	// For scalars or incompatible types, replace with extra
	return CloneExpr(extra)
}

func mutateValues(base, override Expr, span Span) Expr {
	if base == nil {
		return CloneExpr(override)
	}
	if override == nil {
		return CloneExpr(base)
	}

	baseMap, baseIsMap := base.(Map)
	overrideMap, overrideIsMap := override.(Map)
	if baseIsMap && overrideIsMap {
		return mergeMaps(baseMap, overrideMap)
	}

	baseList, baseIsList := base.(List)
	overrideList, overrideIsList := override.(List)
	if baseIsList && overrideIsList {
		// Mutate/inherit elements by index: override specified indices, inherit remaining base items
		maxLen := len(baseList.Items)
		if len(overrideList.Items) > maxLen {
			maxLen = len(overrideList.Items)
		}
		out := List{
			Items:  make([]Expr, maxLen),
			Span:   span,
			Origin: OriginAuthored,
		}
		for i := 0; i < maxLen; i++ {
			if i < len(overrideList.Items) {
				out.Items[i] = CloneExpr(overrideList.Items[i])
			} else {
				out.Items[i] = CloneExpr(baseList.Items[i])
			}
		}
		return out
	}

	// Scalar inheritance: override replaces base value
	return CloneExpr(override)
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
