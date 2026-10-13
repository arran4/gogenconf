package gogenconf_test

import (
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
)

func TestParseNestedSections(t *testing.T) {
	input := `config for myapp version 1

section server
    host "127.0.0.1"
    port 8080

    section tls
        enabled true
        cert_file "/etc/ssl/cert.pem"

        section advanced
            cipher "ECDHE-RSA-AES128-GCM-SHA256"
        end
    end

    section logging
        level "debug"
    end
end

section worker
    concurrency 10

    section tls
        enabled false
    end
end
`

	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "test.conf")
	if err != nil {
		t.Fatalf("ParseNamed failed: %v", err)
	}

	if doc.Identity != "myapp" || doc.Version != 1 {
		t.Fatalf("unexpected header: %s v%d", doc.Identity, doc.Version)
	}

	// Root sections
	server := doc.Section("server")
	if server == nil {
		t.Fatal("missing server section")
	}
	worker := doc.Section("worker")
	if worker == nil {
		t.Fatal("missing worker section")
	}

	// Path lookup
	tlsSec := doc.SectionPath("server", "tls")
	if tlsSec == nil {
		t.Fatal("missing server.tls section")
	}

	advSec := doc.SectionPath("server", "tls", "advanced")
	if advSec == nil {
		t.Fatal("missing server.tls.advanced section")
	}

	// PathValue lookup
	val := doc.PathValue("server", "host")
	if lit, ok := val.(gogenconf.Literal); !ok || lit.Value != "127.0.0.1" {
		t.Errorf("expected server.host '127.0.0.1', got %v", val)
	}

	cipherVal := doc.PathValue("server", "tls", "advanced", "cipher")
	if lit, ok := cipherVal.(gogenconf.Literal); !ok || lit.Value != "ECDHE-RSA-AES128-GCM-SHA256" {
		t.Errorf("expected server.tls.advanced.cipher, got %v", cipherVal)
	}

	// Distinct sub-sections with same name in different scopes
	serverTLS := doc.PathValue("server", "tls", "enabled")
	if lit, ok := serverTLS.(gogenconf.Literal); !ok || lit.Value != "true" {
		t.Errorf("expected server.tls.enabled true, got %v", serverTLS)
	}
	workerTLS := doc.PathValue("worker", "tls", "enabled")
	if lit, ok := workerTLS.(gogenconf.Literal); !ok || lit.Value != "false" {
		t.Errorf("expected worker.tls.enabled false, got %v", workerTLS)
	}

	// Non-existent paths return nil
	if doc.SectionPath("server", "missing") != nil {
		t.Error("expected nil for non-existent section path")
	}
	if doc.PathValue("server", "tls", "nonexistent") != nil {
		t.Error("expected nil for non-existent path value")
	}
}

func TestNestedSectionDuplicatePreservedInAST(t *testing.T) {
	input := `config for myapp version 1

section server
    section tls
        enabled true
    end
    section tls
        enabled false
    end
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "dup.conf")
	if err != nil {
		t.Fatalf("unexpected error parsing duplicate nested sections: %v", err)
	}
	server := doc.Section("server")
	if server == nil {
		t.Fatal("missing server section")
	}
	tlsSections := server.SectionsNamed("tls")
	if len(tlsSections) != 2 {
		t.Fatalf("expected 2 duplicate 'tls' sections preserved in AST, got %d", len(tlsSections))
	}
	if lit, ok := tlsSections[0].Value("enabled").(gogenconf.Literal); !ok || lit.Value != "true" {
		t.Errorf("tls 0 enabled mismatch: %v", tlsSections[0].Value("enabled"))
	}
	if lit, ok := tlsSections[1].Value("enabled").(gogenconf.Literal); !ok || lit.Value != "false" {
		t.Errorf("tls 1 enabled mismatch: %v", tlsSections[1].Value("enabled"))
	}
}

func TestNestedSectionUnterminated(t *testing.T) {
	input := `config for myapp version 1

section server
    section tls
        enabled true
end
`
	_, err := gogenconf.ParseNamed(strings.NewReader(input), "unterminated.conf")
	if err == nil {
		t.Fatal("expected error on unterminated nested section")
	}
	if !strings.Contains(err.Error(), "unterminated section") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNestedSectionUnexpectedEnd(t *testing.T) {
	input := `config for myapp version 1

section server
end
end
`
	_, err := gogenconf.ParseNamed(strings.NewReader(input), "extra_end.conf")
	if err == nil {
		t.Fatal("expected error on unexpected end")
	}
	if !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestNestedSectionClone(t *testing.T) {
	input := `config for myapp version 1

section server
    section tls
        cert "old.pem"
    end
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "test.conf")
	if err != nil {
		t.Fatal(err)
	}

	cloned := doc.Clone()

	// Mutate original
	origTLS := doc.SectionPath("server", "tls")
	origTLS.Set("cert", gogenconf.Literal{Value: "new.pem"})

	// Cloned should remain untouched
	clonedVal := cloned.PathValue("server", "tls", "cert")
	if lit, ok := clonedVal.(gogenconf.Literal); !ok || lit.Value != "old.pem" {
		t.Fatalf("clone was mutated: expected old.pem, got %v", clonedVal)
	}
}

func TestNestedSectionFormatRoundTrip(t *testing.T) {
	input := `config for myapp version 1

section server
    host 127.0.0.1
    section tls
        enabled true
        section advanced
            cipher "AES-GCM"
        end
    end
end
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "test.conf")
	if err != nil {
		t.Fatal(err)
	}

	formatted := gogenconf.Format(doc)

	doc2, err := gogenconf.ParseNamed(strings.NewReader(formatted), "formatted.conf")
	if err != nil {
		t.Fatalf("parsing formatted output failed: %v\nFormatted content:\n%s", err, formatted)
	}

	v1 := doc.PathValue("server", "tls", "advanced", "cipher")
	v2 := doc2.PathValue("server", "tls", "advanced", "cipher")
	lit1, ok1 := v1.(gogenconf.Literal)
	lit2, ok2 := v2.(gogenconf.Literal)
	if !ok1 || !ok2 || lit1.Value != lit2.Value {
		t.Fatalf("round-trip mismatch: %v vs %v", v1, v2)
	}

	// Check indentation in formatted output
	if !strings.Contains(formatted, "    section tls\n") {
		t.Errorf("expected 4 spaces indentation for nested section tls:\n%s", formatted)
	}
	if !strings.Contains(formatted, "        section advanced\n") {
		t.Errorf("expected 8 spaces indentation for doubly nested section advanced:\n%s", formatted)
	}
	if !strings.Contains(formatted, "        end\n") {
		t.Errorf("expected 8 spaces indentation for advanced end:\n%s", formatted)
	}
	if !strings.Contains(formatted, "    end\n") {
		t.Errorf("expected 4 spaces indentation for tls end:\n%s", formatted)
	}
	if !strings.Contains(formatted, "end\n") {
		t.Errorf("expected 0 spaces indentation for server end:\n%s", formatted)
	}
}

func TestCanonicalBraceSyntax(t *testing.T) {
	input := `config for myapp version 1

project lxa {
    module_path github.com/arran4/lxa

    build {
        target ./cmd/lxa
    }
}
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "brace.conf")
	if err != nil {
		t.Fatalf("ParseNamed failed: %v", err)
	}

	modVal := doc.PathValue("project", "lxa", "module_path")
	if lit, ok := modVal.(gogenconf.Literal); !ok || lit.Value != "github.com/arran4/lxa" {
		t.Errorf("expected module_path github.com/arran4/lxa, got %v", modVal)
	}

	targetVal := doc.PathValue("project", "lxa", "build", "target")
	if lit, ok := targetVal.(gogenconf.Literal); !ok || lit.Value != "./cmd/lxa" {
		t.Errorf("expected build target ./cmd/lxa, got %v", targetVal)
	}
}

func TestCompactStructuralPathsEquivalence(t *testing.T) {
	compactInput := `config for testapp version 1

a b c {
    target linux
}
`
	expandedInput := `config for testapp version 1

a {
    b {
        c {
            target linux
        }
    }
}
`
	compactDoc, err := gogenconf.ParseNamed(strings.NewReader(compactInput), "compact.conf")
	if err != nil {
		t.Fatalf("compact parse failed: %v", err)
	}

	expandedDoc, err := gogenconf.ParseNamed(strings.NewReader(expandedInput), "expanded.conf")
	if err != nil {
		t.Fatalf("expanded parse failed: %v", err)
	}

	compactVal := compactDoc.PathValue("a", "b", "c", "target")
	expandedVal := expandedDoc.PathValue("a", "b", "c", "target")

	lit1, ok1 := compactVal.(gogenconf.Literal)
	lit2, ok2 := expandedVal.(gogenconf.Literal)
	if !ok1 || !ok2 || lit1.Value != lit2.Value || lit1.Value != "linux" {
		t.Fatalf("structural equivalence failure: compact=%v, expanded=%v", compactVal, expandedVal)
	}
}

func TestLiteralQuotedPathComponents(t *testing.T) {
	input := `config for testapp version 1

package "sys-apps/portage" {
    slot 0
}

a "b c" d {
    target "value with spaces"
}
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "quoted.conf")
	if err != nil {
		t.Fatalf("ParseNamed failed: %v", err)
	}

	slotVal := doc.PathValue("package", "sys-apps/portage", "slot")
	if lit, ok := slotVal.(gogenconf.Literal); !ok || lit.Value != "0" {
		t.Errorf("expected slot 0, got %v", slotVal)
	}

	targetVal := doc.PathValue("a", "b c", "d", "target")
	if lit, ok := targetVal.(gogenconf.Literal); !ok || lit.Value != "value with spaces" {
		t.Errorf("expected target 'value with spaces', got %v", targetVal)
	}
}

func TestAnonymousNestedObjects(t *testing.T) {
	input := `config for testapp version 1

server {
    {
        listen 8080
    }
}
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "anon.conf")
	if err != nil {
		t.Fatalf("ParseNamed failed: %v", err)
	}

	server := doc.Section("server")
	if server == nil {
		t.Fatal("missing server section")
	}
	if len(server.Sections) != 1 {
		t.Fatalf("expected 1 child section, got %d", len(server.Sections))
	}
	anon := server.Sections[0]
	if anon.Name != "" {
		t.Fatalf("expected anonymous section with empty name, got %q", anon.Name)
	}
	listenVal := anon.Value("listen")
	if lit, ok := listenVal.(gogenconf.Literal); !ok || lit.Value != "8080" {
		t.Errorf("expected listen 8080, got %v", listenVal)
	}
}

func TestBraceSyntaxRoundTrip(t *testing.T) {
	input := `config for myapp version 1

project lxa {
    module_path github.com/arran4/lxa

    build {
        target ./cmd/lxa
    }
}
`
	doc, err := gogenconf.ParseNamed(strings.NewReader(input), "brace.conf")
	if err != nil {
		t.Fatal(err)
	}

	formatted := gogenconf.Format(doc)
	doc2, err := gogenconf.ParseNamed(strings.NewReader(formatted), "formatted.conf")
	if err != nil {
		t.Fatalf("re-parse formatted failed: %v\nFormatted:\n%s", err, formatted)
	}

	v1 := doc.PathValue("project", "lxa", "build", "target")
	v2 := doc2.PathValue("project", "lxa", "build", "target")
	lit1, ok1 := v1.(gogenconf.Literal)
	lit2, ok2 := v2.(gogenconf.Literal)
	if !ok1 || !ok2 || lit1.Value != lit2.Value {
		t.Fatalf("round trip mismatch: %v vs %v", v1, v2)
	}
}

func TestMalformedBracesQuotesAndNesting(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "mismatched brace with end",
			input:   "project lxa {\n    end\n",
			wantErr: "unexpected 'end', expected '}'",
		},
		{
			name:    "mismatched section with brace",
			input:   "section server\n}\n",
			wantErr: "unexpected '}', expected 'end'",
		},
		{
			name:    "unexpected brace at root",
			input:   "}\n",
			wantErr: "unexpected '}'",
		},
		{
			name:    "unterminated brace block",
			input:   "project lxa {\n    module_path test\n",
			wantErr: "unterminated block, missing '}'",
		},
		{
			name:    "unterminated quoted path component",
			input:   "project \"unclosed {\n    module test\n}\n",
			wantErr: "unterminated quote",
		},
		{
			name:    "unexpected tokens after brace",
			input:   "project lxa {\n} extra\n",
			wantErr: "unexpected token after '}'",
		},
		{
			name:    "invalid path component",
			input:   "a @ b {\n}\n",
			wantErr: "invalid path component",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gogenconf.ParseNamed(strings.NewReader(tc.input), "test.conf")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDeclarationAttachedMutationSyntax(t *testing.T) {
	// Canonical declaration-attached mutation syntax per Issue #25:
	// - project[~] lxa { ... }
	// - target[=] ./cmd/lxa
	// - arch[+] [ "riscv64" ]
	// - deprecated_field[-]
	canonicalInput := `config for lxa version 1

project[~] lxa {
    build {
        target[=] ./cmd/lxa
    }
    arch[+] "riscv64"
    deprecated[-]
}

`

	doc, err := gogenconf.ParseNamed(strings.NewReader(canonicalInput), "test_canonical.conf")
	if err != nil {
		t.Fatalf("failed to parse canonical attached mutation syntax: %v", err)
	}

	// A compact declaration path targets its leaf. `project[~] lxa` must
	// mutate lxa, not the project namespace and its sibling declarations.
	proj := doc.Section("project")
	if proj == nil {
		t.Fatal("expected section 'project'")
	}
	if proj.Mutation != gogenconf.MutationNone {
		t.Fatalf("expected project namespace to remain unmodified, got %v", proj.Mutation)
	}

	// Verify lxa section inside project
	lxa := proj.Section("lxa")
	if lxa == nil {
		t.Fatal("expected section 'lxa' inside 'project'")
	}
	if lxa.Mutation != gogenconf.MutationMutate {
		t.Fatalf("expected lxa mutation to be MutationMutate ([~]), got %v", lxa.Mutation)
	}

	// Verify target entry mutation
	build := lxa.Section("build")
	if build == nil {
		t.Fatal("expected section 'build' inside 'lxa'")
	}
	targetEntry, _, ok := build.Entry("target")
	if !ok {
		t.Fatal("expected entry 'target' in 'build'")
	}
	if targetEntry.Mutation != gogenconf.MutationReplace {
		t.Fatalf("expected target mutation to be MutationReplace ([=]), got %v", targetEntry.Mutation)
	}

	// Verify arch entry mutation
	archEntry, _, ok := lxa.Entry("arch")
	if !ok {
		t.Fatal("expected entry 'arch' in 'lxa'")
	}
	if archEntry.Mutation != gogenconf.MutationAdd {
		t.Fatalf("expected arch mutation to be MutationAdd ([+]), got %v", archEntry.Mutation)
	}

	// Verify deprecated entry mutation
	depEntry, _, ok := lxa.Entry("deprecated")
	if !ok {
		t.Fatal("expected entry 'deprecated' in 'lxa'")
	}
	if depEntry.Mutation != gogenconf.MutationDelete {
		t.Fatalf("expected deprecated mutation to be MutationDelete ([-]), got %v", depEntry.Mutation)
	}
	if depEntry.Value != nil {
		t.Fatalf("expected deprecated entry value to be nil, got %v", depEntry.Value)
	}

	// Verify alternative prefix notation produces equivalent AST
	prefixInput := `config for lxa version 1

[~] project lxa {
    build {
        [=] target ./cmd/lxa
    }
    [+] arch "riscv64"
    [-] deprecated
}
`
	prefixDoc, err := gogenconf.ParseNamed(strings.NewReader(prefixInput), "test_prefix.conf")
	if err != nil {
		t.Fatalf("failed to parse prefix mutation syntax: %v", err)
	}

	prefixProj := prefixDoc.Section("project")
	if prefixProj == nil || prefixProj.Mutation != gogenconf.MutationNone {
		t.Fatalf("expected prefix doc project namespace to remain unmodified, got %v", prefixProj)
	}
	if prefixLXA := prefixProj.Section("lxa"); prefixLXA == nil || prefixLXA.Mutation != gogenconf.MutationMutate {
		t.Fatalf("expected prefix doc lxa mutation MutationMutate, got %v", prefixLXA)
	}
	prefixTarget, _, _ := prefixDoc.SectionPath("project", "lxa", "build").Entry("target")
	if prefixTarget.Mutation != gogenconf.MutationReplace {
		t.Fatalf("expected prefix doc target mutation MutationReplace, got %v", prefixTarget.Mutation)
	}
	prefixArch, _, _ := prefixDoc.SectionPath("project", "lxa").Entry("arch")
	if prefixArch.Mutation != gogenconf.MutationAdd {
		t.Fatalf("expected prefix doc arch mutation MutationAdd, got %v", prefixArch.Mutation)
	}
	prefixDep, _, _ := prefixDoc.SectionPath("project", "lxa").Entry("deprecated")
	if prefixDep.Mutation != gogenconf.MutationDelete {
		t.Fatalf("expected prefix doc deprecated mutation MutationDelete, got %v", prefixDep.Mutation)
	}

	// Verify format canonicalization: prefix notation formats as canonical declaration-attached syntax
	formattedFromPrefix := gogenconf.Format(prefixDoc)
	if !strings.Contains(formattedFromPrefix, "lxa[~] {") {
		t.Errorf("expected formatted output to retain leaf mutation, got:\n%s", formattedFromPrefix)
	}
	if !strings.Contains(formattedFromPrefix, "target[=] ./cmd/lxa") {
		t.Errorf("expected formatted output to contain canonical 'target[=] ./cmd/lxa', got:\n%s", formattedFromPrefix)
	}
	if !strings.Contains(formattedFromPrefix, "deprecated[-]") {
		t.Errorf("expected formatted output to contain canonical 'deprecated[-]', got:\n%s", formattedFromPrefix)
	}

	// Round-trip re-parse formatted output
	docRoundTrip, err := gogenconf.ParseNamed(strings.NewReader(formattedFromPrefix), "roundtrip.conf")
	if err != nil {
		t.Fatalf("failed to re-parse formatted output: %v", err)
	}
	rtProj := docRoundTrip.Section("project")
	if rtProj == nil || rtProj.Section("lxa") == nil || rtProj.Section("lxa").Mutation != gogenconf.MutationMutate {
		t.Fatalf("roundtrip mutation mismatch on lxa: got %v", rtProj)
	}
}

func TestParseEmptyCompactBraceDeclaration(t *testing.T) {
	doc, err := gogenconf.ParseNamed(strings.NewReader("config for go2gentoo version 1\n\nproject[-] lxa {}\n"), "delete.conf")
	if err != nil {
		t.Fatalf("parse empty compact declaration: %v", err)
	}
	project := doc.Section("project")
	if project == nil || project.Section("lxa") == nil || project.Section("lxa").Mutation != gogenconf.MutationDelete {
		t.Fatalf("empty compact declaration did not target lxa deletion: %#v", project)
	}
}
