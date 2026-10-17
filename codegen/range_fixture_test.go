package codegen_test

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
)

// boundedRange is deliberately application-owned fixture code. Its `...`
// spelling means an inclusive start and exclusive end, so 0...100 represents
// 100 values without materialising them during configuration loading.
type boundedRange struct{ Start, End int64 }

func parseBoundedRange(raw string, _ m.Span) (boundedRange, error) {
	parts := strings.Split(raw, "...")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return boundedRange{}, fmt.Errorf("range must be start...end")
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return boundedRange{}, fmt.Errorf("range start: %w", err)
	}
	end, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return boundedRange{}, fmt.Errorf("range end: %w", err)
	}
	if end < start {
		return boundedRange{}, fmt.Errorf("range end precedes start")
	}
	return boundedRange{Start: start, End: end}, nil
}

func (r boundedRange) FormatConfig() (string, error) {
	if r.End < r.Start {
		return "", fmt.Errorf("range end precedes start")
	}
	return fmt.Sprintf("%d...%d", r.Start, r.End), nil
}

func (r boundedRange) Materialize(limit int) ([]int64, error) {
	if limit < 0 || r.End < r.Start {
		return nil, fmt.Errorf("invalid range materialisation")
	}
	if r.End-r.Start > int64(limit) {
		return nil, fmt.Errorf("range has %d values, limit is %d", r.End-r.Start, limit)
	}
	values := make([]int64, 0, int(r.End-r.Start))
	for v := r.Start; v < r.End; v++ {
		values = append(values, v)
	}
	return values, nil
}

func TestBoundedRangeFixture(t *testing.T) {
	r, err := parseBoundedRange("0...100", m.Span{Source: "fixture", Line: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r.Start != 0 || r.End != 100 {
		t.Fatalf("range = %#v", r)
	}
	formatted, err := r.FormatConfig()
	if err != nil || formatted != "0...100" {
		t.Fatalf("format = %q, %v", formatted, err)
	}
	roundTrip, err := parseBoundedRange(formatted, m.Span{})
	if err != nil || roundTrip != r {
		t.Fatalf("round trip = %#v, %v", roundTrip, err)
	}
	if _, err := r.Materialize(99); err == nil {
		t.Fatal("oversize range materialised")
	}
	values, err := r.Materialize(100)
	if err != nil || len(values) != 100 || values[99] != 99 {
		t.Fatalf("materialise = %v, %v", values, err)
	}
	for _, raw := range []string{"", "0...", "...1", "3...2", "x...1", "0..." + strconv.FormatUint(uint64(math.MaxInt64)+1, 10)} {
		if _, err := parseBoundedRange(raw, m.Span{}); err == nil {
			t.Fatalf("invalid range %q accepted", raw)
		}
	}
}
