// docgen keeps the navigable site derived from the primary README manual.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	b, err := os.ReadFile("README.md")
	if err != nil {
		return err
	}
	source := string(b)
	links := regexp.MustCompile(`\]\(([^)#]+)\)`)
	source = links.ReplaceAllStringFunc(source, func(s string) string {
		target := s[2 : len(s)-1]
		if strings.Contains(target, "://") {
			return s
		}
		return "](https://github.com/arran4/gogenconf/blob/main/" + target + ")"
	})
	sections := map[string]string{}
	for _, part := range strings.Split(source, "\n## ")[1:] {
		title, _, _ := strings.Cut(part, "\n")
		sections[title] = "## " + part
	}
	pages := []struct {
		slug, title string
		sections    []string
	}{
		{"introduction", "Introduction", []string{"Purpose and boundaries", "Provenance and stability"}},
		{"quick-start", "Quick Start", []string{"Quick start", "Configuration stays declarative"}},
		{"language", "Configuration Language", []string{"Native language reference", "Defaults, examples, comments, and evolution"}},
		{"schemas", "Schemas", []string{"Application schema and generated Go", "Defaults, examples, comments, and evolution"}},
		{"code-generation", "Code Generation", []string{"Application schema and generated Go"}},
		{"resolvers", "Resolvers and Declarers", []string{"Configuration stays declarative", "Native language reference"}},
		{"migrations", "Migrations and Enrichment", []string{"Migration walkthrough", "Defaults, examples, comments, and evolution"}},
		{"sensitive-values", "Sensitive Values", []string{"Sensitive inspection and providers"}},
		{"providers", "Providers", []string{"Sensitive inspection and providers", "Runnable examples"}},
		{"cli", "CLI Reference", []string{"CLI reference", "Installation and man pages"}},
		{"packages", "Go API and Package Guide", []string{"Packages and development"}},
		{"examples", "Examples", []string{"Runnable examples"}},
		{"releases", "Release and Installation", []string{"Installation and man pages", "Release and recovery"}},
	}
	write := func(name, title, body string, weight int) error {
		return os.WriteFile(filepath.Join("docs/content", name+".md"), []byte(fmt.Sprintf("---\ntitle: %q\nweight: %d\n---\n\n<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->\n\n%s", title, weight, body)), 0644)
	}
	if err := os.MkdirAll("docs/content", 0755); err != nil {
		return err
	}
	if err := write("_index", "gogenconf Manual", source, 0); err != nil {
		return err
	}
	for i, p := range pages {
		var body strings.Builder
		for _, title := range p.sections {
			s, ok := sections[title]
			if !ok {
				return fmt.Errorf("missing README section %s", title)
			}
			body.WriteString(s)
		}
		if err := write(p.slug, p.title, body.String(), i+1); err != nil {
			return err
		}
	}
	return nil
}
