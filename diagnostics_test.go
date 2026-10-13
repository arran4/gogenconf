package gogenconf_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
)

func TestParseRecordsSpans(t *testing.T) {
	input := `config_version 1

section server
    host "127.0.0.1"
    port env("PORT", 8080)
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "test.conf")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	sec := doc.Section("server")
	if sec == nil {
		t.Fatal("expected section server")
	}
	if sec.Span.Source != "test.conf" || sec.Span.Line != 3 || sec.Span.Column != 1 {
		t.Errorf("unexpected section span: %+v", sec.Span)
	}
	if sec.Origin != gogenconf.OriginAuthored {
		t.Errorf("expected OriginAuthored for section, got %v", sec.Origin)
	}

	hostEntry, _, ok := sec.Entry("host")
	if !ok {
		t.Fatal("expected entry host")
	}
	if hostEntry.Span.Source != "test.conf" || hostEntry.Span.Line != 4 {
		t.Errorf("unexpected host entry span: %+v", hostEntry.Span)
	}
	if hostEntry.Origin != gogenconf.OriginAuthored {
		t.Errorf("expected OriginAuthored for host entry, got %v", hostEntry.Origin)
	}
	if lit, ok := hostEntry.Value.(gogenconf.Literal); !ok || lit.Value != "127.0.0.1" {
		t.Errorf("unexpected host value: %v", hostEntry.Value)
	} else if lit.Span.Source != "test.conf" || lit.Span.Line != 4 {
		t.Errorf("unexpected literal span: %+v", lit.Span)
	}

	portEntry, _, ok := sec.Entry("port")
	if !ok {
		t.Fatal("expected entry port")
	}
	if call, ok := portEntry.Value.(gogenconf.Call); !ok || call.Name != "env" {
		t.Fatalf("unexpected port value: %v", portEntry.Value)
	} else {
		if call.Span.Source != "test.conf" || call.Span.Line != 5 {
			t.Errorf("unexpected call span: %+v", call.Span)
		}
		if len(call.Args) != 2 {
			t.Fatalf("expected 2 call args, got %d", len(call.Args))
		}
		arg0Span := gogenconf.SpanOf(call.Args[0])
		if arg0Span.Source != "test.conf" || arg0Span.Line != 5 {
			t.Errorf("unexpected arg 0 span: %+v", arg0Span)
		}
	}
}

func TestParseErrorsProduceConfigError(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantLine   int
		wantColumn int
		wantSubstr string
	}{
		{
			name:       "invalid version",
			input:      "config_version abc\n",
			wantLine:   1,
			wantColumn: 1,
			wantSubstr: "invalid config_version",
		},
		{
			name:       "duplicate section",
			input:      "section a\nend\nsection a\nend\n",
			wantLine:   3,
			wantColumn: 1,
			wantSubstr: "duplicate section",
		},
		{
			name:       "invalid expression syntax",
			input:      "section a\n    key env(\nend\n",
			wantLine:   2,
			wantColumn: 9,
			wantSubstr: "invalid expression",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gogenconf.ParseNamed(strings.NewReader(tc.input), "error.conf")
			if err == nil {
				t.Fatal("expected parse error, got nil")
			}
			var cfgErr *gogenconf.ConfigError
			if !errors.As(err, &cfgErr) {
				t.Fatalf("expected *gogenconf.ConfigError, got %T: %v", err, err)
			}
			if cfgErr.Stage != gogenconf.StageParse {
				t.Errorf("expected StageParse, got %v", cfgErr.Stage)
			}
			if cfgErr.Source.Line != tc.wantLine {
				t.Errorf("expected line %d, got %d", tc.wantLine, cfgErr.Source.Line)
			}
			if tc.wantColumn > 0 && cfgErr.Source.Column != tc.wantColumn {
				t.Errorf("expected column %d, got %d", tc.wantColumn, cfgErr.Source.Column)
			}
			if !strings.Contains(cfgErr.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not contain %q", cfgErr.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestProvenancePreservationAndDistinction(t *testing.T) {
	input := `config_version 1

section database
    host "db.internal"
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "db.conf")
	if err != nil {
		t.Fatal(err)
	}

	sec := doc.Section("database")
	entry, _, _ := sec.Entry("host")
	if entry.Origin != gogenconf.OriginAuthored {
		t.Errorf("expected OriginAuthored, got %v", entry.Origin)
	}

	// Schema with default field and new section
	schema := gogenconf.Schema{
		Version: 1,
		Sections: []gogenconf.SectionDefinition{
			{
				Name: "database",
				Fields: []gogenconf.Field{
					{Key: "host", GoType: "string"},
					{Key: "port", GoType: "int", Default: gogenconf.Literal{Value: "5432"}},
				},
			},
			{
				Name: "metrics",
				Fields: []gogenconf.Field{
					{Key: "enabled", GoType: "bool", Default: gogenconf.Literal{Value: "true"}},
				},
			},
		},
	}

	if err := schema.Enrich(doc); err != nil {
		t.Fatalf("enrich failed: %v", err)
	}

	// Enriched default field in existing section
	portEntry, _, ok := sec.Entry("port")
	if !ok {
		t.Fatal("expected port entry from default")
	}
	if portEntry.Origin != gogenconf.OriginDefaulted {
		t.Errorf("expected OriginDefaulted for port entry, got %v", portEntry.Origin)
	}
	if !portEntry.Span.IsZero() {
		t.Errorf("defaulted entry should have zero span, got %+v", portEntry.Span)
	}
	if gogenconf.OriginOf(portEntry.Value) != gogenconf.OriginDefaulted {
		t.Errorf("expected OriginDefaulted on default expression, got %v", gogenconf.OriginOf(portEntry.Value))
	}

	// Enriched section created during enrich
	metricsSec := doc.Section("metrics")
	if metricsSec == nil {
		t.Fatal("expected metrics section from enrich")
	}
	if metricsSec.Origin != gogenconf.OriginEnriched {
		t.Errorf("expected OriginEnriched for metrics section, got %v", metricsSec.Origin)
	}

	// Modification via Set marks OriginMigrated and preserves original span
	sec.Set("host", gogenconf.Literal{Value: "newdb.internal"})
	updatedHost, _, _ := sec.Entry("host")
	if updatedHost.Origin != gogenconf.OriginMigrated {
		t.Errorf("expected OriginMigrated after Set, got %v", updatedHost.Origin)
	}
	if updatedHost.Span != entry.Span {
		t.Errorf("expected span preservation across Set, got %+v vs %+v", updatedHost.Span, entry.Span)
	}

	// Clone preserves spans and origins
	cloned := doc.Clone()
	clonedSec := cloned.Section("database")
	clonedHost, _, _ := clonedSec.Entry("host")
	if clonedHost.Span != updatedHost.Span || clonedHost.Origin != updatedHost.Origin {
		t.Errorf("cloned entry metadata mismatch: %+v vs %+v", clonedHost, updatedHost)
	}
}

func TestSchemaValidationDiagnostics(t *testing.T) {
	input := `config_version 1

section server
    host "localhost"
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "app.conf")
	if err != nil {
		t.Fatal(err)
	}

	schema := gogenconf.Schema{
		Version: 1,
		Sections: []gogenconf.SectionDefinition{
			{
				Name: "server",
				Fields: []gogenconf.Field{
					{Key: "host", GoType: "string"},
					{Key: "port", GoType: "int", Required: true},
				},
			},
		},
	}

	err = schema.Validate(doc)
	if err == nil {
		t.Fatal("expected schema validation error for missing required field")
	}

	var cfgErr *gogenconf.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Stage != gogenconf.StageValidate {
		t.Errorf("expected StageValidate, got %v", cfgErr.Stage)
	}
	if cfgErr.Path != "server.port" {
		t.Errorf("expected path server.port, got %q", cfgErr.Path)
	}
	if cfgErr.Source.Line != 3 {
		t.Errorf("expected source line 3 (section server span), got %d", cfgErr.Source.Line)
	}
}

var errCustomResolution = errors.New("custom resolution failure")

func TestResolveFieldDiagnosticsAndSensitiveRedaction(t *testing.T) {
	ctx := context.Background()
	reg := gogenconf.NewRegistry()
	reg.Register("failing", "", func(ctx context.Context, r *gogenconf.Registry, args []gogenconf.Expr) (any, error) {
		return nil, errCustomResolution
	})

	expr := gogenconf.WithSpan(gogenconf.Call{Name: "failing"}, gogenconf.Span{
		Source: "prod.conf",
		Line:   10,
		Column: 12,
	})

	// Non-sensitive resolution failure: preserves underlying error for errors.Is / errors.As
	_, err := gogenconf.ResolveField[string](ctx, reg, expr, "service.token", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cfgErr *gogenconf.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	if cfgErr.Stage != gogenconf.StageResolve {
		t.Errorf("expected StageResolve, got %v", cfgErr.Stage)
	}
	if cfgErr.Path != "service.token" {
		t.Errorf("expected path service.token, got %q", cfgErr.Path)
	}
	if cfgErr.Source.String() != "prod.conf:10:12" {
		t.Errorf("expected source prod.conf:10:12, got %q", cfgErr.Source.String())
	}
	if !errors.Is(err, errCustomResolution) {
		t.Errorf("expected errors.Is to match errCustomResolution through Unwrap")
	}

	// Sensitive resolution failure: redacts underlying error
	secretExpr := gogenconf.WithSpan(gogenconf.Call{Name: "failing"}, gogenconf.Span{
		Source: "secret.conf",
		Line:   42,
		Column: 5,
	})
	_, sensitiveErr := gogenconf.ResolveField[string](ctx, reg, secretExpr, "service.password", true)
	if sensitiveErr == nil {
		t.Fatal("expected error for sensitive field, got nil")
	}

	var sensitiveCfgErr *gogenconf.ConfigError
	if !errors.As(sensitiveErr, &sensitiveCfgErr) {
		t.Fatalf("expected *ConfigError, got %T", sensitiveErr)
	}
	if errors.Is(sensitiveErr, errCustomResolution) {
		t.Error("sensitive resolution should redact underlying error and not match custom error")
	}
	if strings.Contains(sensitiveCfgErr.Error(), "custom resolution failure") {
		t.Errorf("sensitive diagnostic leaked underlying message: %s", sensitiveCfgErr.Error())
	}
	if !strings.Contains(sensitiveCfgErr.Error(), "source resolution failed") {
		t.Errorf("expected 'source resolution failed' in error, got: %s", sensitiveCfgErr.Error())
	}
}

func TestSpanAndOriginFormatting(t *testing.T) {
	s1 := gogenconf.Span{Source: "foo.conf", Line: 12, Column: 4}
	if s1.String() != "foo.conf:12:4" {
		t.Errorf("unexpected span string: %s", s1.String())
	}
	s2 := gogenconf.Span{Line: 5, Column: 2}
	if s2.String() != "line 5:2" {
		t.Errorf("unexpected span string: %s", s2.String())
	}
	s3 := gogenconf.Span{Source: "bar.conf"}
	if s3.String() != "bar.conf" {
		t.Errorf("unexpected span string: %s", s3.String())
	}

	origins := []struct {
		origin gogenconf.Origin
		want   string
	}{
		{gogenconf.OriginAuthored, "authored"},
		{gogenconf.OriginDefaulted, "defaulted"},
		{gogenconf.OriginEnriched, "enriched"},
		{gogenconf.OriginMigrated, "migrated"},
	}
	for _, o := range origins {
		if o.origin.String() != o.want {
			t.Errorf("origin %v String() = %q, want %q", o.origin, o.origin.String(), o.want)
		}
	}
}
