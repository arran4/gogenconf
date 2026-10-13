package codegen

import (
	"bytes"
	"fmt"
	m "github.com/arran4/gogenconf"
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
	if options.Migrations != nil {
		if err := options.Migrations.Apply(d, SchemaVersion); err != nil {
			return %sConfig{}, fmt.Errorf("migration failed: %%w", err)
		}
	}
	if !options.SkipEnrich {
		if err := Enrich(d); err != nil {
			return %sConfig{}, fmt.Errorf("enrichment failed: %%w", err)
		}
	}
	if err := Validate(d, options.Strict); err != nil {
		return %sConfig{}, fmt.Errorf("validation failed: %%w", err)
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
	return Resolve(ctx, d, r, lookup)
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
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, qual, qual, qual, qual, qual, rtAlias, qual, qual, rtAlias, rtAlias, qual, qual, qual, rtAlias, qual, qual, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)

	// Enrich function
	p(`// Enrich idempotently adds missing default fields and sections to d,
// preserving existing values, user comments, and unknown content.
func Enrich(d *%s.Document) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	if !d.HasVersion || d.Version != SchemaVersion {
		return fmt.Errorf("configuration must be migrated to version %%d before enrichment", SchemaVersion)
	}
`, rtAlias)

	for _, sec := range s.Sections {
		if sec.Repeated {
			p(`	for _, s := range d.Sections {
		if !strings.HasPrefix(s.Name, %q) {
			continue
		}
`, sec.Name+" ")
			for _, f := range sec.Fields {
				if f.InputOnly || f.RuntimeOnly || f.Default == nil {
					continue
				}
				ex, err := expressionWithPkg(f.Default, rtAlias)
				if err != nil {
					return err
				}
				p(`		if _, _, ok := s.Entry(%q); !ok {
			s.Items = append(s.Items, %s.Entry{Key: %q, Value: %s, Defaulted: true})
		}
`, f.Key, rtAlias, f.Key, ex)
			}
			p(`	}
`)
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
`, sec.Name, rtAlias, sec.Name, rtAlias)
			for _, f := range sec.Fields {
				if f.InputOnly || f.RuntimeOnly || f.Default == nil {
					continue
				}
				ex, err := expressionWithPkg(f.Default, rtAlias)
				if err != nil {
					return err
				}
				p(`		if _, _, ok := s.Entry(%q); !ok {
			s.Items = append(s.Items, %s.Entry{Key: %q, Value: %s, Defaulted: true})
		}
`, f.Key, rtAlias, f.Key, ex)
			}
			p(`	}
`)
		}
	}
	p(`	return nil
}
`)

	// Validate function
	p(`// Validate checks that d conforms to the schema version and required fields.
// When strict is true, unknown sections and fields are rejected.
func Validate(d *%s.Document, strict bool) error {
	if d == nil {
		return fmt.Errorf("nil document")
	}
	if d.Version > SchemaVersion {
		return fmt.Errorf("configuration version %%d is newer than supported version %%d", d.Version, SchemaVersion)
	}
`, rtAlias)

	for _, sec := range s.Sections {
		hasRequired := false
		for _, f := range sec.Fields {
			if f.Required {
				hasRequired = true
				break
			}
		}
		if !hasRequired {
			continue
		}
		if sec.Repeated {
			p(`	for _, s := range d.Sections {
		if !strings.HasPrefix(s.Name, %q) {
			continue
		}
`, sec.Name+" ")
			for _, f := range sec.Fields {
				if f.Required {
					p(`		if e, _, ok := s.Entry(%q); !ok || e.Value == nil {
			return fmt.Errorf("missing required field %%s.%%s", s.Name, %q)
		}
`, f.Key, f.Key)
				}
			}
			p(`	}
`)
		} else {
			p(`	{
		s := d.Section(%q)
		if s == nil {
			return fmt.Errorf("missing required section %%q", %q)
		}
`, sec.Name, sec.Name)
			for _, f := range sec.Fields {
				if f.Required {
					p(`		if e, _, ok := s.Entry(%q); !ok || e.Value == nil {
			return fmt.Errorf("missing required field %%s.%%s", %q, %q)
		}
`, f.Key, sec.Name, f.Key)
				}
			}
			p(`	}
`)
		}
	}

	p(`	if strict {
		for _, s := range d.Sections {
			switch {
`)
	for _, sec := range s.Sections {
		if sec.Repeated {
			p(`			case strings.HasPrefix(s.Name, %q):
`, sec.Name+" ")
		} else {
			p(`			case s.Name == %q:
`, sec.Name)
		}
	}
	p(`			default:
				return fmt.Errorf("unknown section %%q", s.Name)
			}
			for _, item := range s.Items {
				if entry, ok := item.(%s.Entry); ok {
					if _, ok := Field(s.Name, entry.Key); !ok {
						return fmt.Errorf("unknown field %%q in section %%q", entry.Key, s.Name)
					}
				}
			}
		}
	}
	return nil
}
`, rtAlias)

	// Inspect functions
	p(`// Inspect returns a redacted copy of d where sensitive literals and calls are redacted.
func Inspect(d *%s.Document) *%s.Document {
	if d == nil {
		return nil
	}
	cloned := d.Clone()
	for _, s := range cloned.Sections {
		for i, item := range s.Items {
			if entry, ok := item.(%s.Entry); ok {
				if IsSensitive(s.Name, entry.Key) {
					entry.Value = %s.Literal{Value: "[REDACTED]"}
					s.Items[i] = entry
				}
			}
		}
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
`, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias, rtAlias)

	// FieldMeta and metadata queries
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
	baseSection := section
`)
	for _, sec := range s.Sections {
		if sec.Repeated {
			p(`	if strings.HasPrefix(section, %q) {
		baseSection = %q
	}
`, sec.Name+" ", sec.Name)
		}
	}
	p(`	switch baseSection {
`)
	for _, sec := range s.Sections {
		p(`	case %q:
		switch key {
`, sec.Name)
		for _, f := range sec.Fields {
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
`, f.Key, f.Key, sec.Name, f.Required, f.Sensitive, f.InputOnly, !f.InputOnly && !f.RuntimeOnly, f.Deprecated)
		}
		p(`		}
`)
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
	p("// Sample returns a sample Document illustrating defaults as active entries\n" +
		"// and examples as documentation comments.\n" +
		"func Sample() *" + rtAlias + ".Document {\n" +
		"	d := &" + rtAlias + ".Document{\n" +
		"		Version: SchemaVersion,\n" +
		"		HasVersion: true,\n" +
		"		Items: []" + rtAlias + ".Item{\n" +
		"			" + rtAlias + ".Version{},\n" +
		"			" + rtAlias + ".Blank{},\n" +
		"			" + rtAlias + ".Comment{Kind: " + rtAlias + ".UserComment, Text: \"Single # comments belong to you; ## documentation is schema-managed.\"},\n" +
		"			" + rtAlias + ".Comment{Kind: " + rtAlias + ".DocumentationComment, Text: \"Examples below are comments, not active values or defaults.\"},\n" +
		"			" + rtAlias + ".Blank{},\n" +
		"		},\n" +
		"	}\n")

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
			for _, doc := range sec.Documentation {
				p(`		sec.Items = append(sec.Items, %s.Comment{Kind: %s.DocumentationComment, Text: %q})
`, rtAlias, rtAlias, doc)
			}
			if len(sec.Documentation) > 0 {
				p(`		sec.Items = append(sec.Items, %s.Blank{})
`, rtAlias)
			}
			for _, f := range sec.Fields {
				if f.InputOnly || f.RuntimeOnly || f.Deprecated {
					continue
				}
				for _, doc := range f.Documentation {
					p(`		sec.Items = append(sec.Items, %s.Comment{Kind: %s.DocumentationComment, Text: %q})
`, rtAlias, rtAlias, doc)
				}
				if len(f.Examples) > 0 {
					p(`		sec.Items = append(sec.Items, %s.Comment{Kind: %s.DocumentationComment, Text: "Examples (inactive):"})
`, rtAlias, rtAlias)
					for _, ex := range f.Examples {
						formatted := m.FormatExpr(ex)
						p(`		sec.Items = append(sec.Items, %s.Comment{Kind: %s.DocumentationComment, Text: "  " + %q + " " + %q})
`, rtAlias, rtAlias, f.Key, formatted)
					}
				}
				if f.Default != nil {
					ex, err := expressionWithPkg(f.Default, rtAlias)
					if err != nil {
						return err
					}
					p(`		sec.Set(%q, %s)
`, f.Key, ex)
				}
				p(`		sec.Items = append(sec.Items, %s.Blank{})
`, rtAlias)
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
