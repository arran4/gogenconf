package codegen_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	appconfig_static "github.com/arran4/gogenconf/examples/generated/appconfig/appconfig"
	nativeconfig_static "github.com/arran4/gogenconf/examples/generated/appconfig/nativeconfig"
)

// ============================================================================
// 1. Production Generated Application API Benchmarks
//
// These benchmarks measure the actual compiled production code generated for
// examples/generated/appconfig (no handwritten stand-ins):
// - nativeconfig_static.Parse: standalone zero-dependency parser
// - appconfig_static.Resolve: static typed decoder (decode-only given AST)
// - appconfig_static.LoadBytes: complete configuration loading (parse + decode + validate)
// ============================================================================

const prodScalarInput = `config_version 2

service {
    endpoint "https://api.internal.net"
    api_key "secret-service-token-12345"
}
`

var prodScalarBytes = []byte(prodScalarInput)

func makeProdRepeatedInput(count int) string {
	var sb strings.Builder
	sb.WriteString("config_version 2\n\nservice {\n    endpoint \"https://api.internal.net\"\n    api_key \"secret-service-token-12345\"\n}\n")
	for i := 0; i < count; i++ {
		sb.WriteString(fmt.Sprintf("worker worker-%02d {\n    concurrency \"8\"\n}\n", i))
	}
	return sb.String()
}

var prodRepeatedInput = makeProdRepeatedInput(50)
var prodRepeatedBytes = []byte(prodRepeatedInput)

// BenchmarkProductionStatic_Parse_Scalars measures standalone native parser throughput on scalar sections.
func BenchmarkProductionStatic_Parse_Scalars(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(prodScalarBytes)))
	for i := 0; i < b.N; i++ {
		doc, err := nativeconfig_static.Parse(strings.NewReader(prodScalarInput))
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

// BenchmarkProductionStatic_DecodeOnly_Scalars measures static typed decoding from pre-parsed AST.
func BenchmarkProductionStatic_DecodeOnly_Scalars(b *testing.B) {
	doc, err := nativeconfig_static.Parse(strings.NewReader(prodScalarInput))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfg, err := appconfig_static.Resolve(ctx, doc, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

// BenchmarkProductionStatic_EndToEnd_Scalars measures complete configuration loading including
// parsing, transactional document copy, and validation.
func BenchmarkProductionStatic_EndToEnd_Scalars(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.SetBytes(int64(len(prodScalarBytes)))

	for i := 0; i < b.N; i++ {
		cfg, _, err := appconfig_static.LoadBytes(ctx, prodScalarBytes)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

// BenchmarkProductionStatic_Parse_Repeated_50 measures parsing of 50 repeated worker sections.
func BenchmarkProductionStatic_Parse_Repeated_50(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(prodRepeatedBytes)))
	for i := 0; i < b.N; i++ {
		doc, err := nativeconfig_static.Parse(strings.NewReader(prodRepeatedInput))
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

// BenchmarkProductionStatic_DecodeOnly_Repeated_50 measures static typed decoding of 50 repeated objects.
func BenchmarkProductionStatic_DecodeOnly_Repeated_50(b *testing.B) {
	doc, err := nativeconfig_static.Parse(strings.NewReader(prodRepeatedInput))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfg, err := appconfig_static.Resolve(ctx, doc, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

// BenchmarkProductionStatic_EndToEnd_Repeated_50 measures complete configuration loading of 50 repeated objects.
func BenchmarkProductionStatic_EndToEnd_Repeated_50(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.SetBytes(int64(len(prodRepeatedBytes)))

	for i := 0; i < b.N; i++ {
		cfg, _, err := appconfig_static.LoadBytes(ctx, prodRepeatedBytes)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

// ============================================================================
// 2. Reproducible Historical vs Static Benchmark Runner
//
// Directly executes the isolated benchmark suite located in testdata/benchmark,
// comparing:
// - Actual generated historical binder from frozen revision d000175b840edf42166bf2f441b7dbce3153a7de
// - Actual generated static decoder from current codebase
// Across identical schemas, identical inputs, and matching lifecycle stages.
//
// Can also be executed independently via:
//   cd codegen/testdata/benchmark && go test -bench=. -benchmem
// ============================================================================

// TestBenchmarkHarnessIntegrity verifies that the isolated benchmark suite compiles
// and passes without error across all historical and static targets.
func TestBenchmarkHarnessIntegrity(t *testing.T) {
	benchDir, err := filepath.Abs("testdata/benchmark")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "go", "test", "-bench=.", "-benchmem", "-benchtime=50ms", ".")
	cmd.Dir = benchDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated benchmark harness failed: %v\n%s", err, out)
	}
}

// BenchmarkActualGeneratedHistoricalVsStatic executes the isolated historical vs static benchmark harness.
func BenchmarkActualGeneratedHistoricalVsStatic(b *testing.B) {
	benchDir, err := filepath.Abs("testdata/benchmark")
	if err != nil {
		b.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "go", "test", "-bench=.", "-benchmem", "-benchtime=300ms", ".")
	cmd.Dir = benchDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		b.Fatalf("benchmark execution failed: %v\n%s", err, out)
	}
	b.Logf("\n%s", out)
}
