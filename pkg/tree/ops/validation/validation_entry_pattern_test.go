package validation

import (
	"regexp"
	"sync"
	"testing"
)

var benchPatterns = []struct{ pattern, value string }{
	{`[a-zA-Z0-9_\-]+`, "ethernet-1_15"},
	{`\i\c*`, "mgmt0.100"},
	{`(\d{1,3}\.){3}\d{1,3}`, "192.168.100.254"},
	{`^[a-z]+$`, "abcdef"},
}

// BenchmarkPatternBaseline measures the previous behaviour: translate and
// compile on every value.
func BenchmarkPatternBaseline(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp := benchPatterns[i%len(benchPatterns)]
		if _, err := regexp.MatchString(yangPatternToGo(bp.pattern), bp.value); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPatternCached measures compilePattern followed by a match.
func BenchmarkPatternCached(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bp := benchPatterns[i%len(benchPatterns)]
		cp := compilePattern(bp.pattern)
		if cp.err != nil {
			b.Fatal(cp.err)
		}
		cp.re.MatchString(bp.value)
	}
}

func TestCompilePattern_Matches(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{`[a-z]+`, "abc", true},
		{`[a-z]+`, "abc1", false}, // implicit anchoring
		{`^[a-z]+$`, "abc", true}, // explicit anchors trimmed
		{`a|b`, "b", true},
		{`a|b`, "ab", false}, // alternation stays inside the anchors
		{`\i\c*`, "x.y-1", true},
	}
	for _, tt := range tests {
		cp := compilePattern(tt.pattern)
		if cp.err != nil {
			t.Fatalf("%q: unexpected error: %v", tt.pattern, cp.err)
		}
		if cp.goPattern != yangPatternToGo(tt.pattern) {
			t.Errorf("%q: goPattern %q, want %q", tt.pattern, cp.goPattern, yangPatternToGo(tt.pattern))
		}
		if got := cp.re.MatchString(tt.value); got != tt.want {
			t.Errorf("%q vs %q: got %t want %t", tt.pattern, tt.value, got, tt.want)
		}
	}
}

func TestCompilePattern_CacheHit(t *testing.T) {
	a := compilePattern(`[a-z]{3}`)
	b := compilePattern(`[a-z]{3}`)
	if a != b {
		t.Fatal("expected the same cached entry for identical pattern text")
	}
	if c := compilePattern(`[a-z]{4}`); c == a {
		t.Fatal("different pattern text must not share an entry")
	}
}

func TestCompilePattern_InvalidIsCachedWithSameError(t *testing.T) {
	const bad = `([a-z`
	first := compilePattern(bad)
	if first.err == nil {
		t.Fatal("expected compile error")
	}
	if first.goPattern != yangPatternToGo(bad) {
		t.Fatalf("goPattern %q, want %q", first.goPattern, yangPatternToGo(bad))
	}
	if _, err := regexp.Compile(first.goPattern); err == nil || err.Error() != first.err.Error() {
		t.Fatalf("cached error %v differs from a fresh compile error %v", first.err, err)
	}
	for i := 0; i < 3; i++ {
		again := compilePattern(bad)
		if again != first || again.err != first.err {
			t.Fatal("invalid pattern must return the identical cached outcome")
		}
	}
}

func TestCompilePattern_Concurrent(t *testing.T) {
	patterns := []string{`[a-z]+`, `\i\c*`, `([a-z`, `[0-9]{2}`}
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				p := patterns[i%len(patterns)]
				cp := compilePattern(p)
				if (cp.err != nil) != (p == `([a-z`) {
					t.Errorf("%q: unexpected err state %v", p, cp.err)
					return
				}
				if cp.err == nil {
					cp.re.MatchString("abc")
				}
			}
		}()
	}
	wg.Wait()
}
