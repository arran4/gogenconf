package gogenconf

import (
	"fmt"
	"strings"
)

// CommentKind records ownership rather than inferring it from prose.
type CommentKind uint8

const (
	UserComment CommentKind = iota
	DocumentationComment
)

// Origin distinguishes where a configuration node or value originated.
type Origin uint8

const (
	OriginAuthored Origin = iota
	OriginDefaulted
	OriginEnriched
	OriginMigrated
)

func (o Origin) String() string {
	switch o {
	case OriginAuthored:
		return "authored"
	case OriginDefaulted:
		return "defaulted"
	case OriginEnriched:
		return "enriched"
	case OriginMigrated:
		return "migrated"
	default:
		return "unknown"
	}
}

// Span records source text position.
type Span struct {
	Source string // file path, fragment name, or input descriptor
	Line   int    // 1-indexed line
	Column int    // 1-indexed column
	Offset int    // byte offset from start
}

func (s Span) String() string {
	if s.Source != "" {
		if s.Line > 0 && s.Column > 0 {
			return fmt.Sprintf("%s:%d:%d", s.Source, s.Line, s.Column)
		}
		if s.Line > 0 {
			return fmt.Sprintf("%s:%d", s.Source, s.Line)
		}
		return s.Source
	}
	if s.Line > 0 && s.Column > 0 {
		return fmt.Sprintf("line %d:%d", s.Line, s.Column)
	}
	if s.Line > 0 {
		return fmt.Sprintf("line %d", s.Line)
	}
	return ""
}

func (s Span) IsZero() bool {
	return s.Source == "" && s.Line == 0 && s.Column == 0 && s.Offset == 0
}

type Stage string

const (
	StageParse    Stage = "parse"
	StageMigrate  Stage = "migrate"
	StageResolve  Stage = "resolve"
	StageConvert  Stage = "convert"
	StageSanitize Stage = "sanitize"
	StageValidate Stage = "validate"
	StageBind     Stage = "bind"
)

// ConfigError wraps a configuration failure with lifecycle stage, field path,
// and source span/provenance without leaking sensitive values.
type ConfigError struct {
	Stage  Stage
	Path   string
	Source Span
	Err    error
}

func (e *ConfigError) Error() string {
	var parts []string
	if e.Stage != "" {
		parts = append(parts, string(e.Stage))
	}
	if e.Path != "" {
		parts = append(parts, e.Path)
	}
	if !e.Source.IsZero() {
		parts = append(parts, e.Source.String())
	}
	prefix := "configuration error"
	if len(parts) > 0 {
		prefix = fmt.Sprintf("configuration %s", strings.Join(parts, " "))
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", prefix, e.Err)
	}
	return prefix
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}

// Item preserves ordered document structure. Adapters may add Raw items for
// syntax they intentionally do not interpret.
type Item interface{ item() }

type Comment struct {
	Kind CommentKind
	Text string
}

func (Comment) item() {}

type Blank struct{}

func (Blank) item() {}

type Entry struct {
	Key    string
	Value  Expr
	Span   Span
	Origin Origin
	// Defaulted is transient provenance for runtime precedence, never serialized.
	Defaulted bool
}

func (Entry) item() {}

type Raw struct{ Text string }

func (Raw) item() {}

type Section struct {
	Name   string
	Span   Span
	Origin Origin
	Items  []Item
}

func (*Section) item() {}

type Version struct{}

func (Version) item() {}

type Document struct {
	Version    int
	HasVersion bool
	Items      []Item
	Sections   []*Section
}

func (d *Document) Section(name string) *Section {
	for _, s := range d.Sections {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (s *Section) Entry(key string) (Entry, int, bool) {
	if s == nil {
		return Entry{}, -1, false
	}
	for i, item := range s.Items {
		if e, ok := item.(Entry); ok && e.Key == key {
			return e, i, true
		}
	}
	return Entry{}, -1, false
}

func (d *Document) AddSection(s *Section) {
	seen := map[*Section]bool{}
	for _, item := range d.Items {
		if section, ok := item.(*Section); ok {
			seen[section] = true
		}
	}
	for _, section := range d.Sections {
		if !seen[section] {
			d.Items = append(d.Items, section)
		}
	}
	d.Sections = append(d.Sections, s)
	d.Items = append(d.Items, s)
}

func (d *Document) Value(section, key string) Expr {
	e, _, ok := d.Section(section).Entry(key)
	if !ok {
		return nil
	}
	return e.Value
}

func CloneExpr(e Expr) Expr {
	switch e := e.(type) {
	case nil:
		return nil
	case Literal:
		return e
	case *Literal:
		if e == nil {
			return nil
		}
		c := *e
		return &c
	case Call:
		args := make([]Expr, len(e.Args))
		for i, a := range e.Args {
			args[i] = CloneExpr(a)
		}
		return Call{Name: e.Name, Args: args, Span: e.Span, Origin: e.Origin}
	case *Call:
		if e == nil {
			return nil
		}
		args := make([]Expr, len(e.Args))
		for i, a := range e.Args {
			args[i] = CloneExpr(a)
		}
		return &Call{Name: e.Name, Args: args, Span: e.Span, Origin: e.Origin}
	default:
		return nil
	}
}

// Clone owns all slices and expression nodes, so migrations and providers do
// not share mutable declarations with the editing document.
func (d *Document) Clone() *Document {
	out := &Document{Version: d.Version, HasVersion: d.HasVersion}
	sections := map[*Section]*Section{}
	cloneItems := func(items []Item) []Item {
		result := make([]Item, 0, len(items))
		for _, item := range items {
			if e, ok := item.(Entry); ok {
				e.Value = CloneExpr(e.Value)
				item = e
			}
			result = append(result, item)
		}
		return result
	}
	for _, s := range d.Sections {
		c := &Section{Name: s.Name, Span: s.Span, Origin: s.Origin, Items: cloneItems(s.Items)}
		sections[s] = c
		out.Sections = append(out.Sections, c)
	}
	out.Items = cloneItems(d.Items)
	for i, item := range out.Items {
		if s, ok := item.(*Section); ok {
			out.Items[i] = sections[s]
		}
	}
	return out
}

func (s *Section) Set(key string, value Expr) {
	if old, i, ok := s.Entry(key); ok {
		s.Items[i] = Entry{Key: key, Value: value, Span: old.Span, Origin: OriginMigrated}
		return
	}
	s.Items = append(s.Items, Entry{Key: key, Value: value, Origin: OriginMigrated})
}

func (s *Section) Delete(key string) bool {
	if _, i, ok := s.Entry(key); ok {
		s.Items = append(s.Items[:i], s.Items[i+1:]...)
		return true
	}
	return false
}
