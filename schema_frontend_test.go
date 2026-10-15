package gogenconf_test

import (
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

func TestParseSchemaValid(t *testing.T) {
	input := `config for downloadsd-schema version 1

version 1

section service
    go_type ServiceConfig
    ## Service configuration and daemon settings

    section field name string
        ## Service instance name
        required true
    end

    section field priority int
        default 10
        ## Task scheduling priority
    end

    section field tags []string
        default [ "default", "fast" ]
        ## Selection tags
    end

    section field destinations [$]string
        default { primary: "https://torbox.app", fallback: "https://backup.internal" }
        ## Destination endpoints
    end
end

section worker
    repeated true
    example_names [ "worker-1", "worker-2" ]
    ## Background download workers

    section field concurrency int
        default 4
        required true
    end

    section field api_token string
        sensitive true
        required true
        env DOWNLOADSD_WORKER_TOKEN
    end
end
`

	s, err := gogenconf.ParseSchemaNamed(strings.NewReader(input), "downloadsd.schema")
	if err != nil {
		t.Fatalf("ParseSchemaNamed failed: %v", err)
	}

	if s.Version != 1 {
		t.Fatalf("expected schema version 1, got %d", s.Version)
	}

	if len(s.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(s.Sections))
	}

	// Verify service section
	srv := s.Sections[0]
	if srv.Name != "service" || srv.GoType != "ServiceConfig" {
		t.Errorf("service section mismatch: %s / %s", srv.Name, srv.GoType)
	}
	if len(srv.Documentation) == 0 || srv.Documentation[0] != "Service configuration and daemon settings" {
		t.Errorf("service documentation mismatch: %v", srv.Documentation)
	}
	if len(srv.Fields) != 4 {
		t.Fatalf("expected 4 service fields, got %d", len(srv.Fields))
	}

	// Verify scalar required field
	fName := srv.Fields[0]
	if fName.Key != "name" || fName.GoType != "string" || !fName.Required {
		t.Errorf("fName mismatch: %+v", fName)
	}

	// Verify priority default
	fPrio := srv.Fields[1]
	if fPrio.Key != "priority" || fPrio.GoType != "int" {
		t.Errorf("fPrio mismatch: %+v", fPrio)
	}
	if lit, ok := fPrio.Default.(gogenconf.Literal); !ok || lit.Value != "10" {
		t.Errorf("fPrio default mismatch: %v", fPrio.Default)
	}

	// Verify ordered collection field
	fTags := srv.Fields[2]
	if fTags.Key != "tags" || fTags.GoType != "[]string" {
		t.Errorf("fTags mismatch: %+v", fTags)
	}
	if list, ok := fTags.Default.(gogenconf.List); !ok || len(list.Items) != 2 {
		t.Errorf("fTags default list mismatch: %v", fTags.Default)
	}

	// Verify keyed collection field
	fDests := srv.Fields[3]
	if fDests.Key != "destinations" || fDests.GoType != "[$]string" {
		t.Errorf("fDests mismatch: %+v", fDests)
	}
	if mp, ok := fDests.Default.(gogenconf.Map); !ok || len(mp.Entries) != 2 {
		t.Errorf("fDests default map mismatch: %v", fDests.Default)
	}

	// Verify worker section
	worker := s.Sections[1]
	if !worker.Repeated {
		t.Error("expected worker section to be repeated")
	}
	if len(worker.ExampleNames) != 2 || worker.ExampleNames[0] != "worker-1" {
		t.Errorf("worker example names mismatch: %v", worker.ExampleNames)
	}

	// Verify sensitive field with env
	fToken := worker.Fields[1]
	if fToken.Key != "api_token" || !fToken.Sensitive || fToken.Environment != "DOWNLOADSD_WORKER_TOKEN" {
		t.Errorf("fToken mismatch: %+v", fToken)
	}
}

func TestParseSchemaRootFields(t *testing.T) {
	s, err := gogenconf.ParseSchema(strings.NewReader(`config for example-schema version 1

version 1

section root
    section field name string
        required true
    end
end
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.RootFields) != 1 || s.RootFields[0].Key != "name" || !s.RootFields[0].Required {
		t.Fatalf("root fields = %#v", s.RootFields)
	}
	generated, err := codegen.GenerateAppAPI(*s, codegen.Options{Package: "appconfig", LibraryImport: "github.com/arran4/gogenconf"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Name string", `BindingValue(d, "", "name"`, "decodeRootName"} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("generated root decoder missing %q", want)
		}
	}
}

func TestSchemaFrontendCodeGeneration(t *testing.T) {
	input := `config for myapp-schema version 1

version 1

section server
    go_type ServerSettings
    ## Server network settings

    section field host string
        default "0.0.0.0"
        required true
    end

    section field aliases []string
        default [ "localhost", "127.0.0.1" ]
    end
end
`

	s, err := gogenconf.ParseSchema(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSchema failed: %v", err)
	}

	// Feed parsed Schema directly into codegen
	bytes, err := codegen.Generate(*s, codegen.Options{
		Package: "appcfg",
	})
	if err != nil {
		t.Fatalf("codegen.Generate with frontend Schema failed: %v", err)
	}

	content := string(bytes)
	if !strings.Contains(content, "type ServerSettings struct") {
		t.Error("generated code missing expected ServerSettings struct")
	}
}

func TestFormatSchemaRoundTrip(t *testing.T) {
	origSchema := &gogenconf.Schema{
		Version: 1,
		Sections: []gogenconf.SectionDefinition{
			{
				Name:          "database",
				GoName:        "Database",
				GoType:        "DatabaseConfig",
				Documentation: []string{"Primary database connection settings"},
				Fields: []gogenconf.Field{
					{
						Key:           "dsn",
						GoName:        "DSN",
						GoType:        "string",
						Required:      true,
						Sensitive:     true,
						Documentation: []string{"Data source name connection string"},
					},
					{
						Key:     "max_connections",
						GoName:  "MaxConnections",
						GoType:  "int",
						Default: gogenconf.Literal{Value: "25"},
					},
				},
			},
		},
	}

	formatted := gogenconf.FormatSchema("db-schema", origSchema)
	reparsed, err := gogenconf.ParseSchema(strings.NewReader(formatted))
	if err != nil {
		t.Fatalf("parsing formatted schema failed: %v\nFormatted output:\n%s", err, formatted)
	}

	if reparsed.Version != origSchema.Version {
		t.Errorf("version mismatch: %d vs %d", reparsed.Version, origSchema.Version)
	}
	if len(reparsed.Sections) != len(origSchema.Sections) {
		t.Fatalf("section count mismatch: %d vs %d", len(reparsed.Sections), len(origSchema.Sections))
	}

	sec := reparsed.Sections[0]
	if sec.Name != "database" || len(sec.Fields) != 2 {
		t.Fatalf("reparsed section mismatch: %+v", sec)
	}
	if !sec.Fields[0].Sensitive || !sec.Fields[0].Required {
		t.Errorf("field 0 flags not preserved: %+v", sec.Fields[0])
	}
}

func TestParseSchemaErrors(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		errSub string
	}{
		{
			name: "missing identity header",
			input: `version 1
section server
end
`,
			errSub: "missing required 'config for <identity>-schema version 1'",
		},
		{
			name: "non-schema identity",
			input: `config for myapp version 1
version 1
section server
end
`,
			errSub: "invalid schema document identity",
		},
		{
			name: "unsupported schema version",
			input: `config for myapp-schema version 99
version 99
section server
end
`,
			errSub: "unsupported schema specification version",
		},
		{
			name: "malformed field without type",
			input: `config for myapp-schema version 1
version 1
section server
    section field host
    end
end
`,
			errSub: "field declaration requires key and Go type",
		},
		{
			name: "invalid boolean literal in field",
			input: `config for myapp-schema version 1
version 1
section server
    section field host string
        required "maybe"
    end
end
`,
			errSub: "invalid boolean value",
		},
		{
			name: "unsupported boolean expression shape in field",
			input: `config for myapp-schema version 1
version 1
section server
    section field host string
        required call("env")
    end
end
`,
			errSub: "expected boolean literal, got",
		},
		{
			name: "unknown section metadata",
			input: `config for myapp-schema version 1
version 1
section server
    invalid_section_metadata true
end
`,
			errSub: "unknown section metadata",
		},
		{
			name: "unknown field metadata",
			input: `config for myapp-schema version 1
version 1
section server
    section field host string
        bogus_key "val"
    end
end
`,
			errSub: "unknown field metadata",
		},
		{
			name: "invalid Go type in field",
			input: `config for myapp-schema version 1
version 1
section server
    section field host 123invalid
    end
end
`,
			errSub: "invalid Go type",
		},
		{
			name: "invalid numeric default",
			input: `config for myapp-schema version 1
version 1
section server
    section field port int
        default "not_a_number"
    end
end
`,
			errSub: "invalid numeric default",
		},
		{
			name: "invalid policy value",
			input: `config for myapp-schema version 1
version 1
section server
    section field host string
        policy "unsupported_policy"
    end
end
`,
			errSub: "invalid resolution policy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gogenconf.ParseSchema(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("expected error containing %q, got %v", tc.errSub, err)
			}
		})
	}
}

func TestParseSchemaBraceSyntax(t *testing.T) {
	input := `config for app-schema version 1

version 1

server {
    go_type ServerConfig
    ## Server configuration

    host string {
        default "0.0.0.0"
        required true
    }

    port int {
        default 8080
    }

    tags {
        go_type "[]string"
        default [ "web", "api" ]
    }

    routes "[$]string" {
        default { home: "/index", api: "/v1" }
    }
}
`
	s, err := gogenconf.ParseSchema(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSchema failed for brace syntax: %v", err)
	}

	if len(s.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(s.Sections))
	}
	sec := s.Sections[0]
	if sec.Name != "server" || sec.GoType != "ServerConfig" {
		t.Errorf("section mismatch: %s / %s", sec.Name, sec.GoType)
	}
	if len(sec.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(sec.Fields))
	}

	fHost := sec.Fields[0]
	if fHost.Key != "host" || fHost.GoType != "string" || !fHost.Required {
		t.Errorf("fHost mismatch: %+v", fHost)
	}
	if lit, ok := fHost.Default.(gogenconf.Literal); !ok || lit.Value != "0.0.0.0" {
		t.Errorf("fHost default mismatch: %v", fHost.Default)
	}

	fPort := sec.Fields[1]
	if fPort.Key != "port" || fPort.GoType != "int" {
		t.Errorf("fPort mismatch: %+v", fPort)
	}
	if lit, ok := fPort.Default.(gogenconf.Literal); !ok || lit.Value != "8080" {
		t.Errorf("fPort default mismatch: %v", fPort.Default)
	}

	fTags := sec.Fields[2]
	if fTags.Key != "tags" || fTags.GoType != "[]string" {
		t.Errorf("fTags mismatch: %+v", fTags)
	}

	fRoutes := sec.Fields[3]
	if fRoutes.Key != "routes" || fRoutes.GoType != "[$]string" {
		t.Errorf("fRoutes mismatch: %+v", fRoutes)
	}
}
