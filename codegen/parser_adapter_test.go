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
	schema := m.Schema{Version: 1, Sections: []m.SectionDefinition{{
		Name: "policy", GoName: "Policy", GoType: "PolicyConfig", Fields: []m.Field{{
			Key: "rule", GoName: "Rule", GoType: "RuleExpression", Parser: "custom_rule_ast",
		}},
	}}}
	generated, err := codegen.GenerateAppAPI(schema, codegen.Options{Package: "appconfig", RuntimeImport: "example.test/generated/nativeconfig"})
	if err != nil {
		t.Fatal(err)
	}
	src := string(generated)
	for _, want := range []string{"type CustomRuleAstFunc func", "func WithCustomRuleAst", "missing required parser CustomRuleAstFunc"} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated adapter missing %q", want)
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

type RuleExpression interface { Text() string }
type rule string
func (r rule) Text() string { return string(r) }
`
	if err := os.WriteFile(filepath.Join(app, "rule.go"), []byte(extension), 0644); err != nil {
		t.Fatal(err)
	}
	const main = `package main
import (
 "context"
 "fmt"
 "example.test/generated/appconfig"
 "example.test/generated/nativeconfig"
)
func main() {
 parse := func(raw string, span nativeconfig.Span) (appconfig.RuleExpression, error) {
  if raw != "4 > a && b <= 9" || span.Line == 0 { return nil, fmt.Errorf("unexpected parser input %q", raw) }
  return nil, nil
 }
	input := []byte("config for generated version 1\n\npolicy {\n rule CustomerParser(4 > a && b <= 9)\n}\n")
	if _, _, err := appconfig.LoadBytes(context.Background(), input); err == nil { panic("missing parser accepted") }
	var nilParse appconfig.CustomRuleAstFunc
	if _, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(nilParse)); err == nil { panic("nil parser accepted") }
	if _, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(parse), appconfig.WithCustomRuleAst(parse)); err == nil { panic("duplicate parser accepted") }
	_, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithCustomRuleAst(parse))
	if err != nil { panic(err) }
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
