//go:generate go run ./internal/generate

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arran4/gogenconf/examples/generated/appconfig/appconfig"
	"github.com/arran4/gogenconf/examples/generated/appconfig/nativeconfig"
)

func main() {
	ctx := context.Background()

	// Set environment for resolution
	if err := os.Setenv("API_KEY", "secret-service-token-12345"); err != nil {
		panic(err)
	}

	// 1. Transactional application migration setup (v1 to v2)
	migrations := appconfig.NewMigrations()
	migrations.Register(1, func(d *nativeconfig.Document) error {
		// Version 1 used legacy_token; migrate to api_key declarer expression
		sec := d.Section("service")
		if sec != nil {
			if entry, _, ok := sec.Entry("legacy_token"); ok {
				sec.Delete("legacy_token")
				sec.Set("api_key", entry.Value)
			}
		}
		return nil
	})

	// Input document from a v1 configuration file
	rawV1Config := `# Production configuration
config_version 1

section service
    legacy_token from_env(API_KEY)
end

section worker worker-a
    concurrency 8
end
`

	// 2. Load and bind using schema-specialized application API
	cfg, doc, err := appconfig.LoadBytes(ctx, []byte(rawV1Config),
		appconfig.WithMigrations(migrations),
		appconfig.WithStrict(true),
	)
	if err != nil {
		panic(fmt.Sprintf("load failed: %v", err))
	}

	// 3. Business logic accesses concrete, strongly-typed fields
	endpoint := cfg.Service.Endpoint
	apiKey := cfg.Service.APIKey
	if endpoint != "https://api.internal.net" {
		panic(fmt.Sprintf("unexpected default endpoint: %s", endpoint))
	}
	if apiKey != "secret-service-token-12345" {
		panic("resolved API key did not match environment")
	}
	if len(cfg.Workers) != 1 || cfg.Workers[0].Name != "worker-a" || cfg.Workers[0].Concurrency != "8" {
		panic(fmt.Sprintf("unexpected worker config: %+v", cfg.Workers))
	}

	// 4. Safe inspection and logging: sensitive credentials are automatically redacted
	redactedView := appconfig.InspectString(doc)
	if strings.Contains(redactedView, "secret-service-token-12345") {
		panic("diagnostic leakage: sensitive API key appeared in inspected string")
	}
	if !strings.Contains(redactedView, "[REDACTED]") {
		panic("expected [REDACTED] marker in inspected view")
	}

	// 5. Config editing preserves comments and document provenance
	doc.Section("service").Set("endpoint", nativeconfig.Literal{Value: "https://api.custom.internal"})
	tmpFile := filepath.Join(os.TempDir(), "appconfig_example_saved.conf")
	defer os.Remove(tmpFile)
	if err := appconfig.SaveFile(tmpFile, doc); err != nil {
		panic(err)
	}

	// 6. Reload from file to verify persistence
	reloadedCfg, _, err := appconfig.LoadFile(ctx, tmpFile)
	if err != nil {
		panic(err)
	}
	if reloadedCfg.Service.Endpoint != "https://api.custom.internal" {
		panic("persisted endpoint did not round-trip")
	}

	fmt.Printf("appconfig example succeeded: endpoint=%s, workers=%d\n",
		reloadedCfg.Service.Endpoint, len(reloadedCfg.Workers))
}
