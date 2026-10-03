package configmodel

// DisplayPolicy declares which call arguments identify sources rather than
// contain content. Unknown declarers fail closed. This is independent of typed
// resolver registration: registering a resolver does not authorize disclosure.
// Applications may explicitly add rules for their own declarers.
type DisplayPolicy map[string]func([]Expr, DisplayPolicy) bool

func (p DisplayPolicy) SafeSource(e Expr) bool {
	c, ok := e.(Call)
	if !ok {
		return false
	}
	rule := p[c.Name]
	return rule != nil && rule(c.Args, p)
}

func StandardDisplayPolicy() DisplayPolicy {
	locator := func(e Expr, p DisplayPolicy) bool {
		if v, ok := e.(Literal); ok {
			return v.Value != ""
		}
		return p.SafeSource(e)
	}
	literalArg := func(args []Expr, _ DisplayPolicy) bool {
		if len(args) != 1 {
			return false
		}
		v, ok := args[0].(Literal)
		return ok && v.Value != ""
	}
	return DisplayPolicy{
		"from_env":      literalArg,
		"from_env_file": literalArg,
		"from_file":     func(args []Expr, p DisplayPolicy) bool { return len(args) == 1 && locator(args[0], p) },
		"from_json_file": func(args []Expr, p DisplayPolicy) bool {
			if len(args) != 2 || !locator(args[0], p) {
				return false
			}
			v, ok := args[1].(Literal)
			return ok && v.Value != ""
		},
	}
}

// Inspect returns a declaration, never a resolved value. Sensitive literals
// and calls without an explicit source-only display contract are redacted.
func (s Schema) Inspect(d *Document, section, key string, p DisplayPolicy) (string, bool) {
	f, known := s.Field(section, key)
	e := d.Value(section, key)
	if e == nil {
		e = f.Default
	}
	if f.Sensitive && !p.SafeSource(e) {
		return "<redacted>", true
	}
	if e == nil {
		return "", known
	}
	return FormatExpr(e), true
}

// InspectionDocument is a redacted display copy, not a persistence round trip.
// Format/Save the original document to preserve authored declarations.
func (s Schema) InspectionDocument(d *Document, p DisplayPolicy) *Document {
	copy := d.Clone()
	for _, section := range copy.Sections {
		for i, item := range section.Items {
			if entry, ok := item.(Entry); ok {
				if f, known := s.Field(section.Name, entry.Key); known && f.Sensitive && !p.SafeSource(entry.Value) {
					entry.Value = Literal{Value: "<redacted>"}
					section.Items[i] = entry
				}
			}
		}
	}
	return copy
}
