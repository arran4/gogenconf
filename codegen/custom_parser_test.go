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

func TestGeneratedCollectionParserAdapters(t *testing.T) {
	schema := m.Schema{
		Version: 1,
		RootFields: []m.Field{
			{Key: "rules", GoName: "Rules", GoType: "[]RuleExpression", Parser: "custom_rule_ast"},
			{Key: "mapped_rules", GoName: "MappedRules", GoType: "map[string]RuleExpression", Parser: "custom_rule_ast"},
			{Key: "secret_rules", GoName: "SecretRules", GoType: "[]RuleExpression", Parser: "custom_rule_ast", Sensitive: true},
			{Key: "bracket_rules", GoName: "BracketRules", GoType: "[$]RuleExpression", Parser: "custom_rule_ast"},
		},
		Sections: []m.SectionDefinition{
			{
				Name:   "policy",
				GoName: "Policy",
				GoType: "PolicyConfig",
				Fields: []m.Field{
					{Key: "rule_list", GoName: "RuleList", GoType: "[]RuleExpression", Parser: "custom_rule_ast"},
					{Key: "rule_map", GoName: "RuleMap", GoType: "map[string]RuleExpression", Parser: "custom_rule_ast"},
				},
			},
		},
	}

	generated, err := codegen.GenerateAppAPI(schema, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/generated/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI: %v", err)
	}
	src := string(generated)

	// Verify generated types and functions
	for _, want := range []string{
		"type CustomRuleAstFunc func(raw string, span nativeconfig.Span) (RuleExpression, error)",
		"func WithCustomRuleAst(parse CustomRuleAstFunc) LoadOption",
		"func FormatCustomRuleAst(value RuleExpression) (string, error)",
		"func FormatCustomRuleAstSlice(values []RuleExpression) ([]string, error)",
		"func FormatCustomRuleAstMap(values map[string]RuleExpression) (map[string]string, error)",
		"decodeRootRulesElement",
		"decodeRootMappedRulesElement",
		"decodeRootSecretRulesElement",
		"decodeRootBracketRulesElement",
		"decodePolicyRuleListElement",
		"decodePolicyRuleMapElement",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated code missing %q", want)
		}
	}

	if strings.Contains(src, "reflect.") {
		t.Fatal("generated parser adapter uses reflection")
	}

	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/generated\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(dir, "appconfig")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "config_generated.go"), generated, 0644); err != nil {
		t.Fatal(err)
	}

	const extension = `package appconfig

import "example.test/generated/nativeconfig"

type RuleExpression interface {
	Text() string
}

type rule string

func (r rule) Text() string { return string(r) }
func (r rule) FormatConfig() (string, error) { return string(r), nil }

func ParseRule(raw string, span nativeconfig.Span) (RuleExpression, error) {
	return rule(raw), nil
}
`
	if err := os.WriteFile(filepath.Join(appDir, "rule.go"), []byte(extension), 0644); err != nil {
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

	validInput := []byte("config for generated version 1\n\n" +
		"rules [ CustomerParser(4 > a && b <= 9), CustomerParser(x == 1) ]\n" +
		"mapped_rules { first: CustomerParser(a > 1), second: CustomerParser(b < 2) }\n" +
		"secret_rules [ CustomerParser(secret_token_123) ]\n" +
		"bracket_rules { item: CustomerParser(y > 5) }\n" +
		"policy {\n" +
		"  rule_list [ CustomerParser(p > 0) ]\n" +
		"  rule_map { k: CustomerParser(q > 0) }\n" +
		"}\n")

	// 1. Missing required parser
	if _, _, err := appconfig.LoadBytes(context.Background(), validInput); err == nil {
		panic("missing parser accepted")
	}

	// 2. Nil parser option
	var nilParse appconfig.CustomRuleAstFunc
	if _, _, err := appconfig.LoadBytes(context.Background(), validInput, appconfig.WithCustomRuleAst(nilParse)); err == nil {
		panic("nil parser accepted")
	}

	// 3. Duplicate parser registration
	if _, _, err := appconfig.LoadBytes(context.Background(), validInput, appconfig.WithCustomRuleAst(parse), appconfig.WithCustomRuleAst(parse)); err == nil {
		panic("duplicate parser accepted")
	}

	// 4. Successful load
	cfg, _, err := appconfig.LoadBytes(context.Background(), validInput, appconfig.WithCustomRuleAst(parse))
	if err != nil {
		panic(fmt.Sprintf("load failed: %v", err))
	}

	if len(cfg.Rules) != 2 || cfg.Rules[0].Text() != "4 > a && b <= 9" || cfg.Rules[1].Text() != "x == 1" {
		panic(fmt.Sprintf("unexpected cfg.Rules: %v", cfg.Rules))
	}
	if len(cfg.MappedRules) != 2 || cfg.MappedRules["first"].Text() != "a > 1" || cfg.MappedRules["second"].Text() != "b < 2" {
		panic(fmt.Sprintf("unexpected cfg.MappedRules: %v", cfg.MappedRules))
	}
	if len(cfg.SecretRules) != 1 || cfg.SecretRules[0].Text() != "secret_token_123" {
		panic(fmt.Sprintf("unexpected cfg.SecretRules: %v", cfg.SecretRules))
	}
	if len(cfg.BracketRules) != 1 || cfg.BracketRules["item"].Text() != "y > 5" {
		panic(fmt.Sprintf("unexpected cfg.BracketRules: %v", cfg.BracketRules))
	}
	if len(cfg.Policy.RuleList) != 1 || cfg.Policy.RuleList[0].Text() != "p > 0" {
		panic(fmt.Sprintf("unexpected cfg.Policy.RuleList: %v", cfg.Policy.RuleList))
	}
	if len(cfg.Policy.RuleMap) != 1 || cfg.Policy.RuleMap["k"].Text() != "q > 0" {
		panic(fmt.Sprintf("unexpected cfg.Policy.RuleMap: %v", cfg.Policy.RuleMap))
	}

	// 5. Parser/formatter round trips
	formattedSlice, err := appconfig.FormatCustomRuleAstSlice(cfg.Rules)
	if err != nil {
		panic(fmt.Sprintf("FormatCustomRuleAstSlice failed: %v", err))
	}
	if len(formattedSlice) != 2 || formattedSlice[0] != "4 > a && b <= 9" || formattedSlice[1] != "x == 1" {
		panic(fmt.Sprintf("unexpected formattedSlice: %v", formattedSlice))
	}

	formattedMap, err := appconfig.FormatCustomRuleAstMap(cfg.MappedRules)
	if err != nil {
		panic(fmt.Sprintf("FormatCustomRuleAstMap failed: %v", err))
	}
	if len(formattedMap) != 2 || formattedMap["first"] != "a > 1" || formattedMap["second"] != "b < 2" {
		panic(fmt.Sprintf("unexpected formattedMap: %v", formattedMap))
	}

	// 6. Parser error inside collection element
	failParse := func(raw string, span nativeconfig.Span) (appconfig.RuleExpression, error) {
		if raw == "bad_element" {
			return nil, fmt.Errorf("invalid rule syntax: %s", raw)
		}
		return appconfig.ParseRule(raw, span)
	}
	errInput := []byte("config for generated version 1\n\nrules [ CustomerParser(good), CustomerParser(bad_element) ]\n")
	_, _, err = appconfig.LoadBytes(context.Background(), errInput, appconfig.WithCustomRuleAst(failParse))
	if err == nil {
		panic("expected error for bad collection element")
	}
	if !strings.Contains(err.Error(), "bad_element") {
		panic(fmt.Sprintf("expected error to mention bad_element: %v", err))
	}

	// 7. Sensitive collection failure redaction
	failSensitiveParse := func(raw string, span nativeconfig.Span) (appconfig.RuleExpression, error) {
		return nil, fmt.Errorf("top_secret_token_leaked")
	}
	sensInput := []byte("config for generated version 1\n\nsecret_rules [ CustomerParser(secret_value) ]\n")
	_, _, err = appconfig.LoadBytes(context.Background(), sensInput, appconfig.WithCustomRuleAst(failSensitiveParse))
	if err == nil {
		panic("expected error for sensitive parser failure")
	}
	if strings.Contains(err.Error(), "top_secret_token_leaked") || strings.Contains(err.Error(), "secret_value") {
		panic(fmt.Sprintf("sensitive error leaked: %v", err))
	}
	if !strings.Contains(err.Error(), "value conversion failed") {
		panic(fmt.Sprintf("expected value conversion failed: %v", err))
	}

	// 8. Single element fallback for slices
	singleInput := []byte("config for generated version 1\n\nrules CustomerParser(single_val)\n")
	cfgSingle, _, err := appconfig.LoadBytes(context.Background(), singleInput, appconfig.WithCustomRuleAst(parse))
	if err != nil {
		panic(fmt.Sprintf("single element load failed: %v", err))
	}
	if len(cfgSingle.Rules) != 1 || cfgSingle.Rules[0].Text() != "single_val" {
		panic(fmt.Sprintf("unexpected single rules: %v", cfgSingle.Rules))
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated collection parser module failed: %v\n%s", err, output)
	}
}
