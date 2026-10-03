package codegen_test

import (
	"strings"
	"testing"

	gogenconf "github.com/arran4/gogenconf"
	rf "github.com/arran4/gogenconf/codegen/internal/runtimefixture"
)

func TestConformanceExpressions(t *testing.T) {
	validExpressions := []string{
		"8080",
		`""`,
		`" \t leading/trailing\n "`,
		`"a,b(c) \"quoted\" \\path"`,
		`"\x00\xff"`,
		"from_env(CONFIG)",
		`from_json_file(from_env(CONFIG), ".nested.key")`,
		`from_json_file(path, .object.key)`,
		"nested(inner(x), \"a,b\")",
		"multi(one, two, three)",
		"deep(l1(l2(l3(l4(l5(value))))))",
		"empty_args()",
		"spaced( arg1 , arg2 )",
		`escaped("line1\nline2\ttab")`,
	}

	for _, exprStr := range validExpressions {
		t.Run("valid/"+exprStr, func(t *testing.T) {
			canonExpr, canonErr := gogenconf.ParseExpr(exprStr)
			if canonErr != nil {
				t.Fatalf("canonical ParseExpr failed on %q: %v", exprStr, canonErr)
			}
			genExpr, genErr := rf.ParseExpr(exprStr)
			if genErr != nil {
				t.Fatalf("runtime ParseExpr failed on %q: %v", exprStr, genErr)
			}

			canonFormatted := gogenconf.FormatExpr(canonExpr)
			genFormatted := rf.FormatExpr(genExpr)
			if canonFormatted != genFormatted {
				t.Fatalf("formatted mismatch for %q:\ncanonical: %s\ngenerated: %s", exprStr, canonFormatted, genFormatted)
			}

			// Roundtrip parse and format again
			canonAgain, err := gogenconf.ParseExpr(canonFormatted)
			if err != nil {
				t.Fatalf("canonical roundtrip parse failed: %v", err)
			}
			genAgain, err := rf.ParseExpr(genFormatted)
			if err != nil {
				t.Fatalf("runtime roundtrip parse failed: %v", err)
			}

			if gogenconf.FormatExpr(canonAgain) != canonFormatted {
				t.Fatalf("canonical roundtrip format unstable for %q", exprStr)
			}
			if rf.FormatExpr(genAgain) != genFormatted {
				t.Fatalf("runtime roundtrip format unstable for %q", exprStr)
			}

			// Verify structural AST equivalence
			assertASTMatch(t, canonExpr, genExpr)
		})
	}

	invalidExpressions := []string{
		`"unterminated`,
		`"bad\q"`,
		"f(,a)",
		"f(a,)",
		"f(a",
		"f(a))",
		`f("a" "b")`,
		"f(a(b) c)",
		"literal(",
		"a)",
		"1bad(x)",
		strings.Repeat("f(", 70) + "x" + strings.Repeat(")", 70), // nesting > 64
	}

	for _, bad := range invalidExpressions {
		t.Run("invalid/"+bad, func(t *testing.T) {
			_, canonErr := gogenconf.ParseExpr(bad)
			_, genErr := rf.ParseExpr(bad)
			if canonErr == nil {
				t.Errorf("canonical accepted invalid expression %q", bad)
			}
			if genErr == nil {
				t.Errorf("runtime accepted invalid expression %q", bad)
			}
		})
	}
}

func assertASTMatch(t *testing.T, canon gogenconf.Expr, gen rf.Expr) {
	t.Helper()
	switch c := canon.(type) {
	case gogenconf.Literal:
		g, ok := gen.(rf.Literal)
		if !ok {
			t.Fatalf("type mismatch: canonical Literal, generated %T", gen)
		}
		if c.Value != g.Value {
			t.Fatalf("literal value mismatch: canonical %q, generated %q", c.Value, g.Value)
		}
	case *gogenconf.Literal:
		g, ok := gen.(*rf.Literal)
		if !ok {
			t.Fatalf("type mismatch: canonical *Literal, generated %T", gen)
		}
		if c.Value != g.Value {
			t.Fatalf("literal value mismatch: canonical %q, generated %q", c.Value, g.Value)
		}
	case gogenconf.Call:
		g, ok := gen.(rf.Call)
		if !ok {
			t.Fatalf("type mismatch: canonical Call, generated %T", gen)
		}
		if c.Name != g.Name {
			t.Fatalf("call name mismatch: canonical %q, generated %q", c.Name, g.Name)
		}
		if len(c.Args) != len(g.Args) {
			t.Fatalf("call arg count mismatch for %q: canonical %d, generated %d", c.Name, len(c.Args), len(g.Args))
		}
		for i := range c.Args {
			assertASTMatch(t, c.Args[i], g.Args[i])
		}
	case *gogenconf.Call:
		g, ok := gen.(*rf.Call)
		if !ok {
			t.Fatalf("type mismatch: canonical *Call, generated %T", gen)
		}
		if c.Name != g.Name {
			t.Fatalf("call name mismatch: canonical %q, generated %q", c.Name, g.Name)
		}
		if len(c.Args) != len(g.Args) {
			t.Fatalf("call arg count mismatch for %q: canonical %d, generated %d", c.Name, len(c.Args), len(g.Args))
		}
		for i := range c.Args {
			assertASTMatch(t, c.Args[i], g.Args[i])
		}
	default:
		if gen != nil {
			t.Fatalf("expected nil generated expr for %T", canon)
		}
	}
}

func TestConformanceDocuments(t *testing.T) {
	documents := []string{
		"# before version\nconfig_version 1\n\nsection custom\n    ## managed description\n    # user description\n    unknown from_json_file(from_env(CONFIG), \".nested.key\")\n\n    empty \"\"\nend\n# between sections\n\nsection another\n    value \"a,b(c) \\\\ \\\"quoted\\\"\"\nend\n# trailing comment\n",
		"section simple\n    key value\nend\n",
		"config_version 2\nsection a\n    k1 v1\nend\nsection b\n    k2 v2\nend\n",
		"# named / repeated sections\nconfig_version 1\nsection worker worker-1\n    concurrency 4\nend\nsection worker worker-2\n    concurrency 8\nend\n",
		"section tabbed\n\tkey\tvalue\nend\n",
		"section valueless\n    empty_key\nend\n",
	}

	for _, input := range documents {
		t.Run("doc", func(t *testing.T) {
			canonDoc, canonErr := gogenconf.Parse(strings.NewReader(input))
			if canonErr != nil {
				t.Fatalf("canonical Parse failed: %v", canonErr)
			}
			genDoc, genErr := rf.Parse(strings.NewReader(input))
			if genErr != nil {
				t.Fatalf("runtime Parse failed: %v", genErr)
			}

			canonFormatted := gogenconf.Format(canonDoc)
			genFormatted := rf.Format(genDoc)
			if canonFormatted != genFormatted {
				t.Fatalf("formatted output mismatch:\n=== Canonical ===\n%s\n=== Generated ===\n%s", canonFormatted, genFormatted)
			}

			// Parse again and check stability
			canonAgain, err := gogenconf.Parse(strings.NewReader(canonFormatted))
			if err != nil {
				t.Fatalf("canonical re-parse failed: %v", err)
			}
			genAgain, err := rf.Parse(strings.NewReader(genFormatted))
			if err != nil {
				t.Fatalf("runtime re-parse failed: %v", err)
			}

			if gogenconf.Format(canonAgain) != canonFormatted {
				t.Fatal("canonical format not idempotent")
			}
			if rf.Format(genAgain) != genFormatted {
				t.Fatal("runtime format not idempotent")
			}

			// Verify section and entry counts
			if len(canonDoc.Sections) != len(genDoc.Sections) {
				t.Fatalf("section count mismatch: canonical %d, runtime %d", len(canonDoc.Sections), len(genDoc.Sections))
			}
			for i := range canonDoc.Sections {
				cs := canonDoc.Sections[i]
				gs := genDoc.Sections[i]
				if cs.Name != gs.Name {
					t.Fatalf("section %d name mismatch: %q vs %q", i, cs.Name, gs.Name)
				}
				if len(cs.Items) != len(gs.Items) {
					t.Fatalf("section %s item count mismatch: canonical %d, runtime %d", cs.Name, len(cs.Items), len(gs.Items))
				}
			}
		})
	}
}

func TestConformanceCommentOwnership(t *testing.T) {
	versions := []string{"", "config_version 0\n", "config_version 1\n"}
	for _, version := range versions {
		t.Run("version="+version, func(t *testing.T) {
			input := "# ordinary legacy note\n## legacy heading\n### another legacy heading\n#### fourth heading\n" + version + "section service\n    ## field heading\n    endpoint custom\nend\n"

			canonDoc, err := gogenconf.Parse(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			genDoc, err := rf.Parse(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}

			isV1OrHigher := genDoc.Version > 0
			for i := 0; i < 4; i++ {
				cc := canonDoc.Items[i].(gogenconf.Comment)
				gc := genDoc.Items[i].(rf.Comment)

				if cc.Text != gc.Text {
					t.Fatalf("comment %d text mismatch: canonical %q, generated %q", i, cc.Text, gc.Text)
				}

				wantKind := rf.UserComment
				if isV1OrHigher && i > 0 {
					wantKind = rf.DocumentationComment
				}
				if gc.Kind != wantKind {
					t.Fatalf("comment %d kind mismatch for %q: got %v, want %v", i, version, gc.Kind, wantKind)
				}
				if int(cc.Kind) != int(gc.Kind) {
					t.Fatalf("comment %d kind mismatch with canonical: canonical %v, generated %v", i, cc.Kind, gc.Kind)
				}
			}

			// Verify section comment ownership
			cSectionComment := canonDoc.Section("service").Items[0].(gogenconf.Comment)
			gSectionComment := genDoc.Section("service").Items[0].(rf.Comment)
			if int(cSectionComment.Kind) != int(gSectionComment.Kind) {
				t.Fatalf("section comment kind mismatch: canonical %v, generated %v", cSectionComment.Kind, gSectionComment.Kind)
			}

			// Format check
			canonOut := gogenconf.Format(canonDoc)
			genOut := rf.Format(genDoc)
			if canonOut != genOut {
				t.Fatalf("format mismatch for version %q:\ncanonical: %s\ngenerated: %s", version, canonOut, genOut)
			}
			if !isV1OrHigher && genOut != input {
				t.Fatalf("v0/unversioned parse/format changed spelling:\n%s", genOut)
			}
		})
	}
}

func TestConformanceMalformedDocuments(t *testing.T) {
	malformed := []string{
		"config_version 1 garbage\n",
		"config_version -1\n",
		"config_version 1\nconfig_version 1\n",
		"section x\n",
		"end\n",
		"section x\nsection y\nend\n",
		"section x\n a v\n a w\nend\n",
		"section x\nend\nsection x\nend\n",
		"section x\n config_version 1\nend\n",
		"section x\nend junk\n",
		"1bad key\n",
		"section\nend\n",
		"section x\n key bad(\nend\n",
	}

	for _, input := range malformed {
		t.Run("malformed/"+input, func(t *testing.T) {
			_, canonErr := gogenconf.Parse(strings.NewReader(input))
			_, genErr := rf.Parse(strings.NewReader(input))
			if canonErr == nil {
				t.Errorf("canonical accepted malformed document: %q", input)
			}
			if genErr == nil {
				t.Errorf("runtime accepted malformed document: %q", input)
			}
		})
	}
}

func TestConformanceCloningAndEditing(t *testing.T) {
	input := "config_version 1\nsection custom\n    endpoint https://example.test\n    empty \"\"\nend\n"

	canonDoc, err := gogenconf.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	genDoc, err := rf.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}

	canonClone := canonDoc.Clone()
	genClone := genDoc.Clone()

	// Mutate the clones
	canonClone.Section("custom").Set("endpoint", gogenconf.Literal{Value: "replacement"})
	canonClone.Section("custom").Delete("empty")

	genClone.Section("custom").Set("endpoint", rf.Literal{Value: "replacement"})
	genClone.Section("custom").Delete("empty")

	// Verify original documents are untouched
	if gogenconf.Format(canonDoc) != rf.Format(genDoc) {
		t.Fatal("original documents diverged")
	}
	if gogenconf.Format(canonDoc) != input {
		t.Fatal("mutation of clone altered original canonical document")
	}
	if rf.Format(genDoc) != input {
		t.Fatal("mutation of clone altered original generated document")
	}

	// Verify clones match each other
	if gogenconf.Format(canonClone) != rf.Format(genClone) {
		t.Fatalf("clones mismatch:\ncanonical: %s\ngenerated: %s", gogenconf.Format(canonClone), rf.Format(genClone))
	}
}

func TestConformanceCloneExpr(t *testing.T) {
	exprs := []struct {
		canon gogenconf.Expr
		gen   rf.Expr
	}{
		{gogenconf.Literal{Value: "val"}, rf.Literal{Value: "val"}},
		{&gogenconf.Literal{Value: "val"}, &rf.Literal{Value: "val"}},
		{gogenconf.Call{Name: "fn", Args: []gogenconf.Expr{gogenconf.Literal{Value: "arg"}}}, rf.Call{Name: "fn", Args: []rf.Expr{rf.Literal{Value: "arg"}}}},
		{&gogenconf.Call{Name: "fn", Args: []gogenconf.Expr{gogenconf.Literal{Value: "arg"}}}, &rf.Call{Name: "fn", Args: []rf.Expr{rf.Literal{Value: "arg"}}}},
	}

	for _, tc := range exprs {
		canonCloned := gogenconf.CloneExpr(tc.canon)
		genCloned := rf.CloneExpr(tc.gen)
		assertASTMatch(t, canonCloned, genCloned)
	}

	if rf.CloneExpr(nil) != nil {
		t.Fatal("CloneExpr(nil) != nil")
	}
}
