package codegen_test

import (
	"embed"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/internal/testutil"
	"golang.org/x/tools/txtar"
)

//go:embed testdata/generate/*.txtar
var generationCases embed.FS

var updateGenerationGoldens = flag.Bool("update", false, "update codegen txtar expected output")

func TestGenerationFixtures(t *testing.T) {
	cases, err := testutil.LoadCases(generationCases, "testdata/generate")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no generation fixtures discovered")
	}
	for _, tc := range cases {
		t.Run(strings.TrimSuffix(filepath.Base(tc.Path), ".txtar"), func(t *testing.T) {
			name := fixtureName(t, tc.Archive)
			actual := generateFixture(t, name)
			if *updateGenerationGoldens {
				updateFixture(t, tc.Path, tc.Archive, actual)
				return
			}
			want, err := testutil.Tree(tc.Archive, "expected")
			if err != nil {
				t.Fatal(err)
			}
			testutil.CompareTree(t, want, actual)
		})
	}
}

func fixtureName(t *testing.T, ar *txtar.Archive) string {
	t.Helper()
	for _, f := range ar.Files {
		if f.Name == "options.json" {
			var options struct {
				Scenario string `json:"scenario"`
			}
			if err := json.Unmarshal(f.Data, &options); err != nil {
				t.Fatal(err)
			}
			if options.Scenario == "" {
				t.Fatal("options.json has no scenario")
			}
			return options.Scenario
		}
	}
	t.Fatal("fixture has no options.json")
	return ""
}

func generateFixture(t *testing.T, name string) map[string][]byte {
	t.Helper()
	if name == "invalid-type" {
		_, err := codegen.Generate(m.Schema{Version: 1, Sections: []m.SectionDefinition{{Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []m.Field{{Key: "bad", GoName: "Bad", GoType: "complex128"}}}}}, codegen.Options{Package: "config"})
		if err == nil {
			t.Fatal("invalid fixture unexpectedly generated output")
		}
		return map[string][]byte{"error.txt": []byte(err.Error() + "\n")}
	}
	s := fixtureSchema(name)
	model, err := codegen.GenerateModel(s, codegen.Options{Package: "appconfig"})
	if err != nil {
		t.Fatal(err)
	}
	binder, err := codegen.GenerateBinder(s, codegen.Options{Package: "binding", ConfigImport: "example.test/appconfig"})
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"model/config_generated.go": model, "binding/config_generated.go": binder}
}

func fixtureSchema(name string) m.Schema {
	switch name {
	case "basic":
		return m.Schema{Version: 1, Sections: []m.SectionDefinition{{Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []m.Field{{Key: "endpoint", GoName: "Endpoint", GoType: "string", Default: m.Literal{Value: "https://example.test"}}, {Key: "key", GoName: "Key", GoType: "[]byte", Required: true, Sensitive: true}}}}}
	case "features":
		return m.Schema{Version: 2, Sections: []m.SectionDefinition{
			{Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []m.Field{
				{Key: "endpoint", GoName: "Endpoint", GoType: "string", Default: m.Literal{Value: "https://default.example.test"}, Environment: "SERVICE_ENDPOINT", EnvironmentNonEmpty: true},
				{Key: "secret", GoName: "Secret", GoType: "[]byte", Required: true, Sensitive: true},
				{Key: "tags", GoName: "Tags", GoType: "[]string", Default: m.Literal{Value: "one,two"}},
				{Key: "content", GoName: "Content", GoType: "[]byte", Required: true, Policy: m.ProviderBacked},
				{Key: "legacy_location", InputOnly: true, Deprecated: true},
				{GoName: "Enabled", GoType: "bool", RuntimeOnly: true},
			}},
			{Name: "worker", GoName: "Workers", GoType: "WorkerConfig", Repeated: true, Fields: []m.Field{{Key: "args", GoName: "Args", GoType: "[]string", Default: m.Literal{Value: "a,b"}}}},
		}}
	default:
		panic("unknown fixture schema " + name)
	}
}

func updateFixture(t *testing.T, embeddedPath string, ar *txtar.Archive, actual map[string][]byte) {
	t.Helper()
	files := ar.Files[:0]
	for _, f := range ar.Files {
		if !strings.HasPrefix(f.Name, "expected/") {
			files = append(files, f)
		}
	}
	paths := make([]string, 0, len(actual))
	for name := range actual {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		files = append(files, txtar.File{Name: "expected/" + name, Data: actual[name]})
	}
	ar.Files = files
	path := strings.TrimPrefix(embeddedPath, "testdata/")
	if err := os.WriteFile(filepath.Join("testdata", path), txtar.Format(ar), 0644); err != nil {
		t.Fatal(err)
	}
}
