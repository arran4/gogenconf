package codegen

import (
	"bytes"
	"fmt"
	m "github.com/arran4/gogenconf"
	"strings"
	"unicode"
)

// GenerateAppAPI emits a complete schema-specialized application configuration API,
// including typed loading, saving, validation, enrichment, safe inspection,
// schema metadata, and migration scaffolding.
func GenerateAppAPI(s m.Schema, o Options) ([]byte, error) {
	o.EmitAppAPI = true
	model := o.ConfigImport == ""
	return generate(s, o, model, true)
}

func emitAppAPI(b *bytes.Buffer, s m.Schema, o Options, qual, rtAlias string) error {
	p := func(f string, a ...any) { fmt.Fprintf(b, f, a...) }
	configIdentity := strings.TrimSuffix(s.Identity, "-schema")

	p("// SchemaVersion is the current supported configuration version for this schema.\n")
	p("const SchemaVersion = %d\n\n", s.Version)

	p(`type LoadOptions struct {
	Registry   *%s.Registry
	LookupEnv  %s.LookupEnv
	Migrations *Migrations
	Strict     bool
	SkipEnrich bool
}

type LoadOption func(*LoadOptions)

// WithRegistry overrides the declarer registry used during resolution.
func WithRegistry(r *%s.Registry) LoadOption {
	return func(o *LoadOptions) { o.Registry = r }
}

// WithLookupEnv overrides the environment lookup function used during resolution.
func WithLookupEnv(lookup %s.LookupEnv) LoadOption {
	return func(o *LoadOptions) { o.LookupEnv = lookup }
}

// WithMigrations supplies application migrations applied before validation and binding.
func WithMigrations(m *Migrations) LoadOption {
	return func(o *LoadOptions) { o.Migrations = m }
}

// WithStrict enables strict schema checking, rejecting unknown sections or fields.
func WithStrict(strict bool) LoadOption {
	return func(o *LoadOptions) { o.Strict = strict }
}

// WithSkipEnrich bypasses schema default enrichment during Load.
func WithSkipEnrich(skip bool) LoadOption {
	return func(o *LoadOptions) { o.SkipEnrich = skip }
}

// Load binds an unresolved Document into typed application Config, applying migrations,
// enrichment, and validation according to options.
func Load(ctx context.Context, d *%s.Document, opts ...LoadOption) (%sConfig, error) {
	if d == nil {
		return %sConfig{}, fmt.Errorf("nil document")
	}
	var options LoadOptions
	for _, opt := range opts {
		opt(&options)
	}
	working := d.Clone()
	if options.Migrations != nil {
		if err := options.Migrations.Apply(working, SchemaVersion); err != nil {
			return %sConfig{}, fmt.Errorf("migration failed: %%w", err)
		}
	}
	if !options.SkipEnrich {
		if err := Enrich(working); err != nil {
			return %sConfig{}, err
		}
	}
	if err := Validate(working, options.Strict); err != nil {
		return %sConfig{}, err
	}
	r := options.Registry
	if r == nil {
		var err error
		r, err = %s.NewStandardRegistry()
		if err != nil {
			return %sConfig{}, fmt.Errorf("standard registry: %%w", err)
		}
	}
	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	cfg, err := Resolve(ctx, working, r, lookup)
	if err != nil {
		return %sConfig{}, err
	}
	*d = *working
	return cfg, nil
}

// LoadReader parses native configuration from r and loads it into typed Config.
func LoadReader(ctx context.Context, r io.Reader, opts ...LoadOption) (%sConfig, *%s.Document, error) {
	d, err := %s.Parse(r)
	if err != nil {
		return %sConfig{}, nil, err
	}
	c, err := Load(ctx, d, opts...)
	if err != nil {
		return %sConfig{}, d, err
	}
	return c, d, nil
}

// LoadFile reads and parses a native configuration file and loads it into typed Config.
func LoadFile(ctx context.Context, path string, opts ...LoadOption) (%sConfig, *%s.Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return %sConfig{}, nil, err
	}
	defer f.Close()
	return LoadReader(ctx, f, opts...)
}

// LoadBytes parses native configuration bytes and loads it into typed Config.
func LoadBytes(ctx context.Context, data []byte, opts ...LoadOption) (%sConfig, *%s.Document, error) {
	return LoadReader(ctx, bytes.NewReader(data), opts...)
}

// Save formats an unresolved Document and writes it to w.
func Save(w io.Writer, d *%s.Document) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	_, err := io.WriteString(w, %s.Format(d))
	return err
}

// SaveFile formats an unresolved Document and atomically writes it to path.
func SaveFile(path string, d *%s.Document) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	content := %s.Format(d)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Seed returns an initial Document populated with schema defaults and version metadata.
func Seed() *%s.Document {
	d := &%s.Document{
		Version: SchemaVersion,
		HasVersion: true,
		Items: []%s.Item{%s.Version{}, %s.Blank{}},
	}
	_ = Enrich(d)
	return d
}
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, qual, qual, qual, qual, qual, rtAlias, qual, qual, qual, rtAlias, rtAlias, qual, qual, qual, rtAlias, qual, qual, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)

	// Enrich function
	p(`// Enrich idempotently adds missing default fields and sections to d,
// preserving existing values, user comments, and unknown content.
func Enrich(d *%s.Document) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	if d.Version > SchemaVersion {
		return &%s.ConfigError{Stage: %s.StageValidate, Source: d.HeaderSpan, Err: fmt.Errorf("configuration version %%d is newer than supported version %%d", d.Version, SchemaVersion)}
	}
	if !d.HasVersion || d.Version != SchemaVersion {
		return &%s.ConfigError{Stage: %s.StageValidate, Source: d.HeaderSpan, Err: fmt.Errorf("configuration must be migrated to version %%d before enrichment", SchemaVersion)}
	}
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)
	for _, f := range s.RootFields {
		if f.InputOnly || f.RuntimeOnly || f.Default == nil {
			continue
		}
		ex, err := expressionWithPkg(f.Default, rtAlias)
		if err != nil {
			return err
		}
		p(`	if _, _, ok := d.Entry(%q); !ok {
		d.Items = append(d.Items, %s.Entry{Key: %q, Value: %s, Defaulted: true})
	}
`, f.Key, rtAlias, f.Key, ex)
	}

	for _, sec := range s.Sections {
		if sec.Repeated {
			p(`	for _, s := range d.Sections {
		if strings.HasPrefix(s.Name, %q) {
			enrichSection_%s(s)
		}
	}
	if group := d.Section(%q); group != nil && group.Brace {
		for _, s := range group.Sections {
			enrichSection_%s(s)
		}
	}
`, sec.Name+" ", sec.GoName, sec.Name, sec.GoName)
		} else {
			p(`	{
		s := d.Section(%q)
		if s == nil {
			s = &%s.Section{Name: %q}
			if len(d.Sections) > 0 {
				d.Items = append(d.Items, %s.Blank{})
			}
			d.AddSection(s)
		}
		enrichSection_%s(s)
	}
`, sec.Name, rtAlias, sec.Name, rtAlias, sec.GoName)
		}
	}
	p(`	return nil
}
`)

	for _, sec := range s.Sections {
		if err := emitSectionEnricher(b, sec, sec.GoName, rtAlias); err != nil {
			return err
		}
	}

	allSecs := collectAllSections(s.Sections, "")

	p(`func validateRepeatedIdentity(rawName, prefix, pathPrefix string, span %s.Span) (string, error) {
	ident := strings.TrimSpace(strings.TrimPrefix(rawName, prefix+" "))
	if ident == "" || strings.ContainsAny(ident, " \t") {
		return "", &%s.ConfigError{Stage: %s.StageValidate, Path: pathPrefix, Source: span, Err: fmt.Errorf("malformed repeated section identity in %%q", rawName)}
	}
	return ident, nil
}
`, rtAlias, rtAlias, rtAlias)

	p("func isDeclaredChild_Root(name string) bool {\n\tswitch name {\n")
	for _, sec := range s.Sections {
		p("\tcase %q:\n\t\treturn true\n", sec.Name)
	}
	p("\tdefault:\n\t\treturn false\n\t}\n}\n\n")

	for _, sec := range s.Sections {
		emitSectionValidator(b, sec, sec.GoName, qual, rtAlias)
	}

	p(`// Validate checks that d conforms to the schema version and required fields.
// When strict is true, unknown sections and fields are rejected.
func Validate(d *%s.Document, strict bool) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	if d.Version < 0 {
		return &%s.ConfigError{Stage: %s.StageValidate, Source: d.HeaderSpan, Err: fmt.Errorf("invalid configuration version")}
	}
	if d.Version > SchemaVersion {
		return &%s.ConfigError{Stage: %s.StageValidate, Source: d.HeaderSpan, Err: fmt.Errorf("configuration version %%d is newer than supported version %%d", d.Version, SchemaVersion)}
	}
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)
	if configIdentity != "" {
		p(`	if d.Identity != %q {
		return &%s.ConfigError{Stage: %s.StageValidate, Source: d.HeaderSpan, Err: fmt.Errorf("expected config identity %%q, got %%q", %q, d.Identity)}
	}
`, configIdentity, rtAlias, rtAlias, configIdentity)
	}

	p(`	rootSeen := map[string]bool{}
	for _, item := range d.Items {
		entry, ok := item.(%s.Entry)
		if !ok { continue }
		switch entry.Key {
`, rtAlias)
	for _, f := range s.RootFields {
		if f.RuntimeOnly {
			p("\t\tcase %q:\n\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: entry.Key, Source: entry.Span, Err: fmt.Errorf(\"field %%q is runtime-only and cannot be set in configuration\", entry.Key)}\n", runtimeFieldKey(f), rtAlias, rtAlias)
		}
	}
	p(`		}
		if strict {
			if rootSeen[entry.Key] {
				return &%s.ConfigError{Stage: %s.StageBind, Path: entry.Key, Source: entry.Span, Err: fmt.Errorf("duplicate unresolved field")}
			}
			rootSeen[entry.Key] = true
			switch entry.Key {
`, rtAlias, rtAlias)
	for _, f := range s.RootFields {
		if !f.RuntimeOnly {
			p("\t\t\tcase %q:\n", f.Key)
		}
	}
	p(`			default:
				return &%s.ConfigError{Stage: %s.StageValidate, Path: entry.Key, Source: entry.Span, Err: fmt.Errorf("unknown root field %%q", entry.Key)}
			}
		}
	}
`, rtAlias, rtAlias)

	for _, f := range s.RootFields {
		if f.Required {
			p(`	if e, _, ok := d.Entry(%q); !ok || e.Value == nil {
		return &%s.ConfigError{Stage: %s.StageValidate, Path: %q, Source: d.HeaderSpan, Err: fmt.Errorf("missing required root field %%q", %q)}
	}
`, f.Key, rtAlias, rtAlias, f.Key, f.Key)
		}
	}

	for _, sec := range s.Sections {
		if sec.Repeated {
			p("\tseenIdentities_%s := map[string]bool{}\n", sec.GoName)
		} else {
			p("\tfound_%s := false\n", sec.GoName)
		}
	}

	p(`	for _, sNode := range d.Sections {
		switch {
`)
	for _, sec := range s.Sections {
		if sec.Repeated {
			p("\t\tcase strings.HasPrefix(sNode.Name, %q):\n", sec.Name+" ")
			p("\t\t\tident, err := validateRepeatedIdentity(sNode.Name, %q, %q, sNode.Span)\n\t\t\tif err != nil { return err }\n", sec.Name, sec.Name)
			p("\t\t\tsecPath := %q + \".\" + ident\n", sec.Name)
			p("\t\t\tif strict && seenIdentities_%s[ident] {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: secPath, Source: sNode.Span, Err: fmt.Errorf(\"duplicate %s identity %%q\", ident)}\n\t\t\t}\n", sec.GoName, rtAlias, rtAlias, sec.Name)
			p("\t\t\tseenIdentities_%s[ident] = true\n", sec.GoName)
			p("\t\t\tif err := validateSection_%s(sNode, secPath, strict); err != nil { return err }\n", sec.GoName)

			p("\t\tcase sNode.Name == %q:\n", sec.Name)
			p("\t\t\tif !sNode.Brace {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q, Source: sNode.Span, Err: fmt.Errorf(\"missing repeated section identity for %%q\", %q)}\n\t\t\t}\n", rtAlias, rtAlias, sec.Name, sec.Name)
			p("\t\t\tfor _, item := range sNode.Items {\n\t\t\t\tif entry, ok := item.(%s.Entry); ok {\n\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q + \".\" + entry.Key, Source: entry.Span, Err: fmt.Errorf(\"repeated section %%q requires a named instance before field %%q\", %q, entry.Key)}\n\t\t\t\t}\n\t\t\t}\n", rtAlias, rtAlias, rtAlias, sec.Name, sec.Name)
			p("\t\t\tif len(sNode.Sections) == 0 {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q, Source: sNode.Span, Err: fmt.Errorf(\"repeated section %%q requires a named instance\", %q)}\n\t\t\t}\n", rtAlias, rtAlias, sec.Name, sec.Name)
			p("\t\t\tfor _, inst := range sNode.Sections {\n")
			p("\t\t\t\tif inst.Name == \"\" || strings.ContainsAny(inst.Name, \" \\t\") {\n\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q, Source: inst.Span, Err: fmt.Errorf(\"malformed repeated section identity %%q for %%q\", inst.Name, %q)}\n\t\t\t\t}\n", rtAlias, rtAlias, sec.Name, sec.Name)
			p("\t\t\t\tfor _, sub := range inst.Sections {\n")
			p("\t\t\t\t\tif sub.Span.Line == inst.Span.Line && inst.Span.Line > 0 && !isDeclaredChild_%s(sub.Name) {\n\t\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q + \".\" + inst.Name, Source: sub.Span, Err: fmt.Errorf(\"malformed repeated section identity %%q\", %q+\" \"+inst.Name+\" \"+sub.Name)}\n\t\t\t\t\t}\n\t\t\t\t}\n", sec.GoName, rtAlias, rtAlias, sec.Name, sec.Name)
			p("\t\t\t\tsecPath := %q + \".\" + inst.Name\n", sec.Name)
			p("\t\t\t\tif strict && seenIdentities_%s[inst.Name] {\n\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: secPath, Source: inst.Span, Err: fmt.Errorf(\"duplicate %s identity %%q\", inst.Name)}\n\t\t\t\t}\n", sec.GoName, rtAlias, rtAlias, sec.Name)
			p("\t\t\t\tseenIdentities_%s[inst.Name] = true\n", sec.GoName)
			p("\t\t\tif err := validateSection_%s(inst, secPath, strict); err != nil { return err }\n", sec.GoName)
			p("\t\t\t}\n")
		} else {
			p("\t\tcase sNode.Name == %q:\n", sec.Name)
			p("\t\t\tif strict && found_%s {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: %q, Source: sNode.Span, Err: fmt.Errorf(\"duplicate section %%q\", %q)}\n\t\t\t}\n", sec.GoName, rtAlias, rtAlias, sec.Name, sec.Name)
			p("\t\t\tfound_%s = true\n", sec.GoName)
			p("\t\t\tif err := validateSection_%s(sNode, %q, strict); err != nil { return err }\n", sec.GoName, sec.Name)
			p("\t\tcase strings.HasPrefix(sNode.Name, %q):\n", sec.Name+" ")
			p("\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q, Source: sNode.Span, Err: fmt.Errorf(\"section %%q does not accept a named identity\", %q)}\n", rtAlias, rtAlias, sec.Name, sec.Name)
		}
	}
	p(`		default:
			if strict {
				return &%s.ConfigError{Stage: %s.StageValidate, Path: sNode.Name, Source: sNode.Span, Err: fmt.Errorf("unknown section %%q", sNode.Name)}
			}
		}
	}
`, rtAlias, rtAlias)

	for _, sec := range s.Sections {
		if !sec.Repeated {
			if reqRelPath, isReq := sectionRequiresPresence(sec); isReq {
				p("\tif !found_%s {\n\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: %q + \".\" + %q, Source: d.HeaderSpan, Err: fmt.Errorf(\"missing required section %%q\", %q)}\n\t}\n", sec.GoName, rtAlias, rtAlias, sec.Name, reqRelPath, sec.Name)
			}
		}
	}

	p(`	return nil
}
`)

	// Inspect functions
	p(`// Inspect returns a redacted copy of d where sensitive literals and calls are redacted.
func Inspect(d *%s.Document) *%s.Document {
	if d == nil {
		return nil
	}
	cloned := d.Clone()
	for i, item := range cloned.Items {
		if entry, ok := item.(%s.Entry); ok && IsSensitive("", entry.Key) {
			entry.Value = %s.Literal{Value: "[REDACTED]"}
			cloned.Items[i] = entry
		}
	}
	var inspectSection func(s *%s.Section, path string)
	inspectSection = func(s *%s.Section, path string) {
		for i, item := range s.Items {
			if entry, ok := item.(%s.Entry); ok && IsSensitive(path, entry.Key) {
				entry.Value = %s.Literal{Value: "[REDACTED]"}
				s.Items[i] = entry
			}
		}
		for _, child := range s.Sections {
			childPath := path
			if child.Name != "" {
				if childPath != "" {
					childPath += "." + child.Name
				} else {
					childPath = child.Name
				}
			}
			inspectSection(child, childPath)
		}
	}
	for _, s := range cloned.Sections {
		inspectSection(s, s.Name)
	}
	return cloned
}

// InspectString formats the redacted document as a string suitable for logging.
func InspectString(d *%s.Document) string {
	if d == nil {
		return ""
	}
	return %s.Format(Inspect(d))
}
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)

	// Metadata helpers
	for _, s := range allSecs {
		if len(s.childNames) > 0 {
			p("func isChildOf_%s(name string) bool {\n\tswitch name {\n", strings.ReplaceAll(s.canonicalPath, ".", "_"))
			for _, chName := range s.childNames {
				p("\tcase %q:\n\t\treturn true\n", chName)
			}
			p("\tdefault:\n\t\treturn false\n\t}\n}\n\n")
		}
	}

	emitNormalizeSectionPath(b, allSecs)

	p(`// FieldMeta describes runtime schema metadata for a configuration field.
type FieldMeta struct {
	Key        string
	Section    string
	Required   bool
	Sensitive  bool
	InputOnly  bool
	Writable   bool
	Deprecated bool
}

// Field looks up field metadata for section and key.
func Field(section, key string) (FieldMeta, bool) {
	switch normalizeSectionPath(section) {
`)
	if len(s.RootFields) > 0 {
		p(`	case "":
		switch key {
`)
		for _, f := range s.RootFields {
			p(`		case %q:
			return FieldMeta{
				Key: %q,
				Section: "",
				Required: %t,
				Sensitive: %t,
				InputOnly: %t,
				Writable: %t,
				Deprecated: %t,
			}, true
`, f.Key, f.Key, f.Required, f.Sensitive, f.InputOnly, !f.InputOnly && !f.RuntimeOnly, f.Deprecated)
		}
		p(`		}
`)
	}
	for _, secMeta := range allSecs {
		p("\tcase %q:\n\t\tswitch key {\n", secMeta.canonicalPath)
		for _, f := range secMeta.fields {
			p(`		case %q:
			return FieldMeta{
				Key: %q,
				Section: %q,
				Required: %t,
				Sensitive: %t,
				InputOnly: %t,
				Writable: %t,
				Deprecated: %t,
			}, true
`, f.Key, f.Key, secMeta.canonicalPath, f.Required, f.Sensitive, f.InputOnly, !f.InputOnly && !f.RuntimeOnly, f.Deprecated)
		}
		p("\t\t}\n")
	}
	p(`	}
	return FieldMeta{}, false
}

func IsSensitive(section, key string) bool {
	f, ok := Field(section, key)
	return ok && f.Sensitive
}

func IsRequired(section, key string) bool {
	f, ok := Field(section, key)
	return ok && f.Required
}

func IsInputOnly(section, key string) bool {
	f, ok := Field(section, key)
	return ok && f.InputOnly
}

func IsWritable(section, key string) bool {
	f, ok := Field(section, key)
	return ok && f.Writable
}
`)

	// Migrations
	p(`// MigrationFunc executes an explicit, transactional migration step on an unresolved Document.
type MigrationFunc func(d *%s.Document) error

// Migrations provides typed, versioned migration dispatch.
type Migrations struct {
	steps map[int]MigrationFunc
}

func NewMigrations() *Migrations {
	return &Migrations{steps: make(map[int]MigrationFunc)}
}

func (m *Migrations) Register(fromVersion int, fn MigrationFunc) {
	m.steps[fromVersion] = fn
}

func (m *Migrations) Apply(d *%s.Document, targetVersion int) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	if !d.HasVersion {
		d.Version = 0
		d.HasVersion = true
	}
	for d.Version < targetVersion {
		fn, ok := m.steps[d.Version]
		if !ok {
			return fmt.Errorf("no migration registered from version %%d to %%d", d.Version, d.Version+1)
		}
		cloned := d.Clone()
		if err := fn(cloned); err != nil {
			return fmt.Errorf("migration step %%d -> %%d failed: %%w", d.Version, d.Version+1, err)
		}
		cloned.Version = d.Version + 1
		*d = *cloned
	}
	return nil
}
`, rtAlias, rtAlias)

	// Sample function
	p(`// Sample returns a sample Document illustrating defaults as active entries
// and examples as documentation comments.
func Sample() *%s.Document {
	d := &%s.Document{
		Version: SchemaVersion,
		HasVersion: true,
		Items: []%s.Item{
			%s.Version{},
			%s.Blank{},
			%s.Comment{Kind: %s.UserComment, Text: "Single # comments belong to you; ## documentation is schema-managed."},
			%s.Comment{Kind: %s.DocumentationComment, Text: "Examples below are comments, not active values or defaults."},
			%s.Blank{},
		},
	}
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)

	for _, f := range s.RootFields {
		if f.InputOnly || f.RuntimeOnly || f.Deprecated {
			continue
		}
		for _, doc := range f.Documentation {
			p(`		d.Items = append(d.Items, %s.Comment{Kind: %s.DocumentationComment, Text: %q})
`, rtAlias, rtAlias, doc)
		}
		if len(f.Examples) > 0 {
			p(`		d.Items = append(d.Items, %s.Comment{Kind: %s.DocumentationComment, Text: "Examples (inactive):"})
`, rtAlias, rtAlias)
			for _, ex := range f.Examples {
				formatted := m.FormatExpr(ex)
				p(`		d.Items = append(d.Items, %s.Comment{Kind: %s.DocumentationComment, Text: "  " + %q + " " + %q})
`, rtAlias, rtAlias, f.Key, formatted)
			}
		}
		if f.Default != nil {
			ex, err := expressionWithPkg(f.Default, rtAlias)
			if err != nil {
				return err
			}
			p(`		d.Set(%q, %s)
`, f.Key, ex)
		}
		p(`		d.Items = append(d.Items, %s.Blank{})
`, rtAlias)
	}

	for _, sec := range s.Sections {
		names := []string{sec.Name}
		if sec.Repeated {
			names = nil
			for _, exName := range sec.ExampleNames {
				names = append(names, sec.Name+" "+exName)
			}
			if len(names) == 0 {
				names = append(names, sec.Name+" default")
			}
		}
		for _, name := range names {
			p(`	{
		sec := &%s.Section{Name: %q}
`, rtAlias, name)
			if err := emitSampleSection(p, sec, "sec", rtAlias); err != nil {
				return err
			}
			if sec.Repeated {
				p(`		tmp := &%s.Document{}
		tmp.AddSection(sec)
		for _, line := range strings.Split(strings.TrimSuffix(%s.Format(tmp), "\n"), "\n") {
			text := ""
			if line != "" {
				text = "  " + line
			}
			d.Items = append(d.Items, %s.Comment{Kind: %s.DocumentationComment, Text: text})
		}
		d.Items = append(d.Items, %s.Blank{})
	}
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)
			} else {
				p(`		d.AddSection(sec)
		d.Items = append(d.Items, %s.Blank{})
	}
`, rtAlias)
			}
		}
	}
	p(`	return d
}
`)

	return nil
}

func sectionRequiresPresence(sec m.SectionDefinition) (string, bool) {
	for _, f := range sec.Fields {
		if f.Required {
			return f.Key, true
		}
	}
	for _, child := range sec.Children {
		if !child.Repeated {
			if req, ok := sectionRequiresPresence(child); ok {
				return child.Name + "." + req, true
			}
		}
	}
	return "", false
}

func runtimeFieldKey(f m.Field) string {
	if f.Key != "" {
		return f.Key
	}
	var b strings.Builder
	for i, r := range f.GoName {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type schemaSectionMeta struct {
	canonicalPath string
	name          string
	repeated      bool
	childNames    []string
	fields        []m.Field
}

func collectAllSections(sections []m.SectionDefinition, prefix string) []schemaSectionMeta {
	var result []schemaSectionMeta
	for _, sec := range sections {
		cPath := sec.Name
		if prefix != "" {
			cPath = prefix + "." + sec.Name
		}
		var childNames []string
		for _, ch := range sec.Children {
			childNames = append(childNames, ch.Name)
		}
		result = append(result, schemaSectionMeta{
			canonicalPath: cPath,
			name:          sec.Name,
			repeated:      sec.Repeated,
			childNames:    childNames,
			fields:        sec.Fields,
		})
		result = append(result, collectAllSections(sec.Children, cPath)...)
	}
	return result
}

func emitSectionValidator(b *bytes.Buffer, sec m.SectionDefinition, prefix, qual, rtAlias string) {
	p := func(f string, a ...any) { fmt.Fprintf(b, f, a...) }

	p("func isDeclaredChild_%s(name string) bool {\n\tswitch name {\n", prefix)
	for _, child := range sec.Children {
		p("\tcase %q:\n\t\treturn true\n", child.Name)
	}
	p("\tdefault:\n\t\treturn false\n\t}\n}\n\n")

	p("func validateSection_%s(sNode *%s.Section, path string, strict bool) error {\n", prefix, rtAlias)
	p("\tseenEntries := map[string]bool{}\n")
	p("\tfor _, item := range sNode.Items {\n")
	p("\t\tentry, ok := item.(%s.Entry)\n\t\tif !ok { continue }\n", rtAlias)
	p("\t\tentryPath := path + \".\" + entry.Key\n")
	p("\t\tswitch entry.Key {\n")
	for _, f := range sec.Fields {
		if f.RuntimeOnly {
			p("\t\tcase %q:\n\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: entryPath, Source: entry.Span, Err: fmt.Errorf(\"field %%q is runtime-only and cannot be set in configuration\", entry.Key)}\n", runtimeFieldKey(f), rtAlias, rtAlias)
		}
	}
	p("\t\t}\n")
	p("\t\tif strict {\n")
	p("\t\t\tif seenEntries[entry.Key] {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: entryPath, Source: entry.Span, Err: fmt.Errorf(\"duplicate unresolved field\")}\n\t\t\t}\n", rtAlias, rtAlias)
	p("\t\t\tseenEntries[entry.Key] = true\n")
	p("\t\t\tswitch entry.Key {\n")
	for _, f := range sec.Fields {
		if !f.RuntimeOnly {
			p("\t\t\tcase %q:\n", f.Key)
		}
	}
	p("\t\t\tdefault:\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: entryPath, Source: entry.Span, Err: fmt.Errorf(\"unknown field %%q in section %%q\", entry.Key, path)}\n", rtAlias, rtAlias)
	p("\t\t\t}\n")
	p("\t\t}\n")
	p("\t}\n")

	for _, f := range sec.Fields {
		if f.Required {
			p("\tif e, _, ok := sNode.Entry(%q); !ok || e.Value == nil {\n\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path + \".\" + %q, Source: sNode.Span, Err: fmt.Errorf(\"missing required field %%s.%%s\", path, %q)}\n\t}\n", f.Key, rtAlias, rtAlias, f.Key, f.Key)
		}
	}

	for _, child := range sec.Children {
		childPrefix := prefix + child.GoName
		if child.Repeated {
			p("\tseenIdentities_%s := map[string]bool{}\n", childPrefix)
		} else {
			p("\tfound_%s := false\n", childPrefix)
		}
	}

	p("\tfor _, childSec := range sNode.Sections {\n")
	p("\t\tswitch {\n")
	for _, child := range sec.Children {
		childPrefix := prefix + child.GoName
		if child.Repeated {
			p("\t\tcase strings.HasPrefix(childSec.Name, %q):\n", child.Name+" ")
			p("\t\t\tident, err := validateRepeatedIdentity(childSec.Name, %q, path+\".\"+%q, childSec.Span)\n\t\t\tif err != nil { return err }\n", child.Name, child.Name)
			p("\t\t\tchildPath := path + \".\" + %q + \".\" + ident\n", child.Name)
			p("\t\t\tif strict && seenIdentities_%s[ident] {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: childPath, Source: childSec.Span, Err: fmt.Errorf(\"duplicate %s identity %%q\", ident)}\n\t\t\t}\n", childPrefix, rtAlias, rtAlias, child.Name)
			p("\t\t\tseenIdentities_%s[ident] = true\n", childPrefix)
			p("\t\t\tif err := validateSection_%s(childSec, childPath, strict); err != nil { return err }\n", childPrefix)

			p("\t\tcase childSec.Name == %q:\n", child.Name)
			p("\t\t\tif !childSec.Brace {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path+\".\"+%q, Source: childSec.Span, Err: fmt.Errorf(\"missing repeated section identity for %%q\", %q)}\n\t\t\t}\n", rtAlias, rtAlias, child.Name, child.Name)
			p("\t\t\tfor _, item := range childSec.Items {\n\t\t\t\tif entry, ok := item.(%s.Entry); ok {\n\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path + \".\" + %q + \".\" + entry.Key, Source: entry.Span, Err: fmt.Errorf(\"repeated section %%q requires a named instance before field %%q\", %q, entry.Key)}\n\t\t\t\t}\n\t\t\t}\n", rtAlias, rtAlias, rtAlias, child.Name, child.Name)
			p("\t\t\tif len(childSec.Sections) == 0 {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path+\".\"+%q, Source: childSec.Span, Err: fmt.Errorf(\"repeated section %%q requires a named instance\", %q)}\n\t\t\t}\n", rtAlias, rtAlias, child.Name, child.Name)
			p("\t\t\tfor _, inst := range childSec.Sections {\n")
			p("\t\t\t\tif inst.Name == \"\" || strings.ContainsAny(inst.Name, \" \\t\") {\n\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path+\".\"+%q, Source: inst.Span, Err: fmt.Errorf(\"malformed repeated section identity %%q for %%q\", inst.Name, %q)}\n\t\t\t\t}\n", rtAlias, rtAlias, child.Name, child.Name)
			p("\t\t\t\tfor _, sub := range inst.Sections {\n")
			p("\t\t\t\t\tif sub.Span.Line == inst.Span.Line && inst.Span.Line > 0 && !isDeclaredChild_%s(sub.Name) {\n\t\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path + \".\" + %q + \".\" + inst.Name, Source: sub.Span, Err: fmt.Errorf(\"malformed repeated section identity %%q\", %q+\" \"+inst.Name+\" \"+sub.Name)}\n\t\t\t\t\t}\n\t\t\t\t}\n", childPrefix, rtAlias, rtAlias, child.Name, child.Name)
			p("\t\t\t\tchildPath := path + \".\" + %q + \".\" + inst.Name\n", child.Name)
			p("\t\t\t\tif strict && seenIdentities_%s[inst.Name] {\n\t\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: childPath, Source: inst.Span, Err: fmt.Errorf(\"duplicate %s identity %%q\", inst.Name)}\n\t\t\t\t}\n", childPrefix, rtAlias, rtAlias, child.Name)
			p("\t\t\t\tseenIdentities_%s[inst.Name] = true\n", childPrefix)
			p("\t\t\tif err := validateSection_%s(inst, childPath, strict); err != nil { return err }\n", childPrefix)
			p("\t\t\t}\n")
		} else {
			p("\t\tcase childSec.Name == %q:\n", child.Name)
			p("\t\t\tif strict && found_%s {\n\t\t\t\treturn &%s.ConfigError{Stage: %s.StageBind, Path: path+\".\"+%q, Source: childSec.Span, Err: fmt.Errorf(\"duplicate section %%q\", %q)}\n\t\t\t}\n", childPrefix, rtAlias, rtAlias, child.Name, child.Name)
			p("\t\t\tfound_%s = true\n", childPrefix)
			p("\t\t\tif err := validateSection_%s(childSec, path+\".\"+%q, strict); err != nil { return err }\n", childPrefix, child.Name)
			p("\t\tcase strings.HasPrefix(childSec.Name, %q):\n", child.Name+" ")
			p("\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path+\".\"+%q, Source: childSec.Span, Err: fmt.Errorf(\"section %%q does not accept a named identity\", %q)}\n", rtAlias, rtAlias, child.Name, child.Name)
		}
	}
	p("\t\tdefault:\n")
	p("\t\t\tif strict {\n")
	p("\t\t\t\tunknownPath := path + \".\" + childSec.Name\n")
	p("\t\t\t\tif childSec.Name == \"\" { unknownPath = path }\n")
	p("\t\t\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: unknownPath, Source: childSec.Span, Err: fmt.Errorf(\"unknown section %%q in %%q\", childSec.Name, path)}\n", rtAlias, rtAlias)
	p("\t\t\t}\n")
	p("\t\t}\n")
	p("\t}\n")

	for _, child := range sec.Children {
		if !child.Repeated {
			if reqRelPath, isReq := sectionRequiresPresence(child); isReq {
				childPrefix := prefix + child.GoName
				p("\tif !found_%s {\n\t\treturn &%s.ConfigError{Stage: %s.StageValidate, Path: path + \".\" + %q + \".\" + %q, Source: sNode.Span, Err: fmt.Errorf(\"missing required section %%q\", %q)}\n\t}\n", childPrefix, rtAlias, rtAlias, child.Name, reqRelPath, child.Name)
			}
		}
	}

	p("\treturn nil\n}\n\n")

	for _, child := range sec.Children {
		emitSectionValidator(b, child, prefix+child.GoName, qual, rtAlias)
	}
}

func emitNormalizeSectionPath(b *bytes.Buffer, allSecs []schemaSectionMeta) {
	p := func(f string, a ...any) { fmt.Fprintf(b, f, a...) }
	p(`func normalizeSectionPath(section string) string {
	if section == "" {
		return ""
	}
	parts := strings.FieldsFunc(section, func(r rune) bool { return r == ' ' || r == '.' })
	if len(parts) == 0 {
		return ""
	}
	var out []string
	idx := 0
	scope := "root"
	for idx < len(parts) {
		tok := parts[idx]
		switch scope {
		case "root":
			switch tok {
`)
	for _, s := range allSecs {
		if !strings.Contains(s.canonicalPath, ".") {
			p("\t\t\tcase %q:\n\t\t\t\tout = append(out, %q)\n\t\t\t\tscope = %q\n\t\t\t\tidx++\n", s.name, s.name, s.name)
			if s.repeated {
				if len(s.childNames) > 0 {
					p("\t\t\t\tif idx < len(parts) && !isChildOf_%s(parts[idx]) {\n\t\t\t\t\tidx++\n\t\t\t\t}\n", strings.ReplaceAll(s.canonicalPath, ".", "_"))
				} else {
					p("\t\t\t\tif idx < len(parts) {\n\t\t\t\t\tidx++\n\t\t\t\t}\n")
				}
			}
		}
	}
	p(`			default:
				out = append(out, tok)
				idx++
			}
`)
	for _, s := range allSecs {
		if len(s.childNames) > 0 {
			p("\t\tcase %q:\n\t\t\tswitch tok {\n", s.canonicalPath)
			for _, childName := range s.childNames {
				childPath := s.canonicalPath + "." + childName
				var childMeta *schemaSectionMeta
				for i := range allSecs {
					if allSecs[i].canonicalPath == childPath {
						childMeta = &allSecs[i]
						break
					}
				}
				p("\t\t\tcase %q:\n\t\t\t\tout = append(out, %q)\n\t\t\t\tscope = %q\n\t\t\t\tidx++\n", childName, childName, childPath)
				if childMeta != nil && childMeta.repeated {
					if len(childMeta.childNames) > 0 {
						p("\t\t\t\tif idx < len(parts) && !isChildOf_%s(parts[idx]) {\n\t\t\t\t\tidx++\n\t\t\t\t}\n", strings.ReplaceAll(childMeta.canonicalPath, ".", "_"))
					} else {
						p("\t\t\t\tif idx < len(parts) {\n\t\t\t\t\tidx++\n\t\t\t\t}\n")
					}
				}
			}
			p(`			default:
				out = append(out, tok)
				idx++
			}
`)
		}
	}
	p(`		default:
			out = append(out, tok)
			idx++
		}
	}
	return strings.Join(out, ".")
}
`)
}

func emitSectionEnricher(b *bytes.Buffer, sec m.SectionDefinition, prefix, rtAlias string) error {
	p := func(f string, a ...any) { fmt.Fprintf(b, f, a...) }
	p("func enrichSection_%s(s *%s.Section) {\n", prefix, rtAlias)
	for _, f := range sec.Fields {
		if f.InputOnly || f.RuntimeOnly || f.Default == nil {
			continue
		}
		ex, err := expressionWithPkg(f.Default, rtAlias)
		if err != nil {
			return err
		}
		p("\tif _, _, ok := s.Entry(%q); !ok {\n\t\ts.Items = append(s.Items, %s.Entry{Key: %q, Value: %s, Defaulted: true})\n\t}\n", f.Key, rtAlias, f.Key, ex)
	}
	for _, child := range sec.Children {
		childPrefix := prefix + child.GoName
		if child.Repeated {
			p("\tfor _, ch := range s.Sections {\n\t\tif strings.HasPrefix(ch.Name, %q) {\n\t\t\tenrichSection_%s(ch)\n\t\t}\n\t}\n", child.Name+" ", childPrefix)
			p("\tif group := s.Section(%q); group != nil && group.Brace {\n\t\tfor _, ch := range group.Sections {\n\t\t\tenrichSection_%s(ch)\n\t\t}\n\t}\n", child.Name, childPrefix)
		} else {
			p("\tif ch := s.Section(%q); ch != nil {\n\t\tenrichSection_%s(ch)\n\t}\n", child.Name, childPrefix)
		}
	}
	p("}\n\n")
	for _, child := range sec.Children {
		if err := emitSectionEnricher(b, child, prefix+child.GoName, rtAlias); err != nil {
			return err
		}
	}
	return nil
}

func emitSampleSection(p func(string, ...any), sec m.SectionDefinition, secVar, rtAlias string) error {
	for _, doc := range sec.Documentation {
		p("\t\t%s.Items = append(%s.Items, %s.Comment{Kind: %s.DocumentationComment, Text: %q})\n", secVar, secVar, rtAlias, rtAlias, doc)
	}
	if len(sec.Documentation) > 0 {
		p("\t\t%s.Items = append(%s.Items, %s.Blank{})\n", secVar, secVar, rtAlias)
	}
	for _, f := range sec.Fields {
		if f.InputOnly || f.RuntimeOnly || f.Deprecated {
			continue
		}
		for _, doc := range f.Documentation {
			p("\t\t%s.Items = append(%s.Items, %s.Comment{Kind: %s.DocumentationComment, Text: %q})\n", secVar, secVar, rtAlias, rtAlias, doc)
		}
		if len(f.Examples) > 0 {
			p("\t\t%s.Items = append(%s.Items, %s.Comment{Kind: %s.DocumentationComment, Text: \"Examples (inactive):\"})\n", secVar, secVar, rtAlias, rtAlias)
			for _, ex := range f.Examples {
				formatted := m.FormatExpr(ex)
				p("\t\t%s.Items = append(%s.Items, %s.Comment{Kind: %s.DocumentationComment, Text: \"  \" + %q + \" \" + %q})\n", secVar, secVar, rtAlias, rtAlias, f.Key, formatted)
			}
		}
		if f.Default != nil {
			ex, err := expressionWithPkg(f.Default, rtAlias)
			if err != nil {
				return err
			}
			p("\t\t%s.Set(%q, %s)\n", secVar, f.Key, ex)
		}
		p("\t\t%s.Items = append(%s.Items, %s.Blank{})\n", secVar, secVar, rtAlias)
	}
	for _, child := range sec.Children {
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
		for _, name := range names {
			childVar := "ch_" + child.GoName
			p("\t\t{\n\t\t\t%s := &%s.Section{Name: %q}\n", childVar, rtAlias, name)
			if err := emitSampleSection(p, child, childVar, rtAlias); err != nil {
				return err
			}
			p("\t\t\t%s.AddSection(%s)\n\t\t\t%s.Items = append(%s.Items, %s.Blank{})\n\t\t}\n", secVar, childVar, secVar, secVar, rtAlias)
		}
	}
	return nil
}
