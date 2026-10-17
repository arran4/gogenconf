package benchmark_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/arran4/gogenconf"
	hist_basic_bind "github.com/arran4/gogenconf/examples/generated/basic/binding"
	hist_rep_bind "github.com/arran4/gogenconf/examples/generated/repeated/binding"

	basic_native "benchmark.test/basic/nativeconfig"
	basic_app "benchmark.test/basic/staticapp"
	rep_native "benchmark.test/repeated/nativeconfig"
	rep_app "benchmark.test/repeated/staticapp"
)

// ============================================================================
// 1. Basic Scalar Workload (Matching Schema & Identical Authoring)
// Schema:
//   section service
//     endpoint string (default: https://example.test)
//     credential []byte (required, sensitive)
//   end
// ============================================================================

const basicInput = `config_version 1
section service
    endpoint "https://custom.service.test"
    credential "secret-token-12345"
end
`

var basicInputBytes = []byte(basicInput)

func BenchmarkParse_Basic_Historical(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(basicInputBytes)))
	for i := 0; i < b.N; i++ {
		doc, err := gogenconf.Parse(strings.NewReader(basicInput))
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

func BenchmarkParse_Basic_Static(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(basicInputBytes)))
	for i := 0; i < b.N; i++ {
		doc, err := basic_native.Parse(strings.NewReader(basicInput))
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

func BenchmarkDecodeOnly_Basic_Historical(b *testing.B) {
	doc, err := gogenconf.Parse(strings.NewReader(basicInput))
	if err != nil {
		b.Fatal(err)
	}
	reg, err := gogenconf.NewStandardRegistry()
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfg, err := hist_basic_bind.Resolve(ctx, doc, reg, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkDecodeOnly_Basic_Static(b *testing.B) {
	doc, err := basic_native.Parse(strings.NewReader(basicInput))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfg, err := basic_app.Resolve(ctx, doc, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkEndToEnd_Basic_Historical(b *testing.B) {
	reg, err := gogenconf.NewStandardRegistry()
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.SetBytes(int64(len(basicInputBytes)))

	for i := 0; i < b.N; i++ {
		doc, err := gogenconf.Parse(strings.NewReader(basicInput))
		if err != nil {
			b.Fatal(err)
		}
		cfg, err := hist_basic_bind.Resolve(ctx, doc, reg, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkEndToEnd_Basic_Static(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.SetBytes(int64(len(basicInputBytes)))

	for i := 0; i < b.N; i++ {
		cfg, _, err := basic_app.LoadBytes(ctx, basicInputBytes)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

// ============================================================================
// 2. Repeated Section Workload (50 Repeated Workers, Matching Schema & Input)
// Schema:
//   section worker <name>
//     label string (required)
//   end
// ============================================================================

func makeRepeatedInput(count int) string {
	var sb strings.Builder
	sb.WriteString("config_version 1\n")
	for i := 0; i < count; i++ {
		sb.WriteString(fmt.Sprintf("section worker worker-%02d\n    label \"worker-label-%02d\"\nend\n", i, i))
	}
	return sb.String()
}

var repeatedInput = makeRepeatedInput(50)
var repeatedInputBytes = []byte(repeatedInput)

func BenchmarkParse_Repeated_Historical(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(repeatedInputBytes)))
	for i := 0; i < b.N; i++ {
		doc, err := gogenconf.Parse(strings.NewReader(repeatedInput))
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

func BenchmarkParse_Repeated_Static(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(repeatedInputBytes)))
	for i := 0; i < b.N; i++ {
		doc, err := rep_native.Parse(strings.NewReader(repeatedInput))
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

func BenchmarkDecodeOnly_Repeated_Historical(b *testing.B) {
	doc, err := gogenconf.Parse(strings.NewReader(repeatedInput))
	if err != nil {
		b.Fatal(err)
	}
	reg, err := gogenconf.NewStandardRegistry()
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfg, err := hist_rep_bind.Resolve(ctx, doc, reg, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkDecodeOnly_Repeated_Static(b *testing.B) {
	doc, err := rep_native.Parse(strings.NewReader(repeatedInput))
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfg, err := rep_app.Resolve(ctx, doc, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkEndToEnd_Repeated_Historical(b *testing.B) {
	reg, err := gogenconf.NewStandardRegistry()
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.SetBytes(int64(len(repeatedInputBytes)))

	for i := 0; i < b.N; i++ {
		doc, err := gogenconf.Parse(strings.NewReader(repeatedInput))
		if err != nil {
			b.Fatal(err)
		}
		cfg, err := hist_rep_bind.Resolve(ctx, doc, reg, nil)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkEndToEnd_Repeated_Static(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.SetBytes(int64(len(repeatedInputBytes)))

	for i := 0; i < b.N; i++ {
		cfg, _, err := rep_app.LoadBytes(ctx, repeatedInputBytes)
		if err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}
