package codegen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

func TestGeneratedNamedParserAdapter(t *testing.T) {
	doc, err := m.Parse(strings.NewReader("config for generated version 1\n\nrule CustomerParser(4 > a && b <= 9)\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := doc.Entry("rule"); !ok {
		t.Fatal("root parser expression was not retained as a root entry")
	}
	schema := m.Schema{Version: 1, RootFields: []m.Field{{
		Key: "rule", GoName: "Rule", GoType: "RuleExpression", Parser: "custom_rule_ast", Sensitive: true,
	}}}
	generated, err := codegen.GenerateAppAPI(schema, codegen.Options{Package: "appconfig", RuntimeImport: "example.test/generated/nativeconfig"})
	if err != nil {
		t.Fatal(err)
	}
	src := string(generated)
	for _, want := range []string{"type CustomRuleAstFunc func", "func WithCustomRuleAst", "func FormatCustomRuleAst", "missing required parser CustomRuleAstFunc"} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated adapter missing %q", want)
		}
	}
	for _, want := range []string{"decodeRootRule(e, options.CustomRuleAst)", "func decodeRootRule(e nativeconfig.Expr, parse CustomRuleAstFunc)"} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated root dispatch missing %q", want)
		}
	}
	if strings.Contains(src, "reflect.") {
		t.Fatal("generated parser adapter uses reflection")
	}
	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{Package: "nativeconfig", Capabilities: codegen.FullRuntimeCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/generated\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "appconfig")
	if err := os.MkdirAll(app, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "config_generated.go"), generated, 0644); err != nil {
		t.Fatal(err)
	}
	const extension = `package appconfig

import "example.test/generated/nativeconfig"

type RuleExpression interface { Text() string }
type rule string
func (r rule) Text() string { return string(r) }
func (r rule) FormatConfig() (string, error) { return string(r), nil }
func ParseRule(raw string, span nativeconfig.Span) (RuleExpression, error) { return rule(raw), nil }
`
	if err := os.WriteFile(filepath.Join(app, "rule.go"), []byte(extension), 0644); err != nil {
		t.Fatal(err)
	}
	const main = `package main
import (
 "context"
 "fmt"
 "strings"
 "example.test/generated/appconfig"
 "example.test/generated/nativeconfig"
)
func main() {
	parse := appconfig.ParseRule
	input := []byte("config for generated version 1\n\nrule CustomerParser(4 > a && b <= 9)\n")
	if _, _, err := appconfig.LoadBytes(context.Background(), input); err == nil { panic("missing parser accepted") }
	var nilParse appconfig.CustomRuleAstFunc
	if _, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(nilParse)); err == nil { panic("nil parser accepted") }
	if _, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(parse), appconfig.WithCustomRuleAst(parse)); err == nil { panic("duplicate parser accepted") }
	fail := func(raw string, span nativeconfig.Span) (appconfig.RuleExpression, error) { return nil, fmt.Errorf("secret expression %s", raw) }
	if _, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(fail)); err == nil || strings.Contains(err.Error(), "secret expression") || strings.Contains(err.Error(), "4 > a") { panic(fmt.Sprintf("sensitive parser error leaked: %v", err)) }
	cfg, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(parse))
	if err != nil { panic(err) }
	formatted, err := appconfig.FormatCustomRuleAst(cfg.Rule)
	if err != nil || formatted != "4 > a && b <= 9" { panic(fmt.Sprintf("format = %q, %v", formatted, err)) }
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated parser module failed: %v\n%s", err, output)
	}
}

func TestParserNamesNeedNotBeGoIdentifiers(t *testing.T) {
	schema := m.Schema{Version: 1, Sections: []m.SectionDefinition{{Name: "x", GoName: "X", GoType: "XConfig", Fields: []m.Field{{Key: "value", GoName: "Value", GoType: "Value", Parser: "custom-rule_ast"}}}}}
	if _, err := codegen.GenerateAppAPI(schema, codegen.Options{Package: "x"}); err != nil {
		t.Fatalf("descriptive parser name rejected: %v", err)
	}
	schema.Sections[0].Fields = append(schema.Sections[0].Fields, m.Field{Key: "other", GoName: "Other", GoType: "Value", Parser: "custom_rule_ast"})
	if _, err := codegen.GenerateAppAPI(schema, codegen.Options{Package: "x"}); err == nil {
		t.Fatal("ambiguous generated parser symbol accepted")
	}
}
