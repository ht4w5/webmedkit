package filter

import (
	"fmt"
	"testing"

	"github.com/HT4w5/webmedkit/bandage/internal/domain"
)

// benchMatch sinks Match results so the calls cannot be elided.
var benchMatch bool

// knownSize gives an event a known Filesize: a size comparison matches
// nothing without one (size 0 means unknown).
func knownSize(e *domain.Event) { e.SizeBytes = 1 << 20 }

// leafCases pairs each leaf filter type with sources that do and do not match
// the case's event - baseEvent, with mut adjusting it where the leaves need
// more than its defaults. Each case mutates only its own event, so an
// adjustment cannot skew any other benchmark's baseline; only the size leaves
// set and read SizeBytes.
var leafCases = []struct {
	name        string
	match, miss string
	mut         func(*domain.Event) // applied to baseEvent
}{
	{"remote-addr", `src 203.0.113.0/24`, `src 198.51.100.0/24`, nil},
	{"method", `method GET`, `method POST`, nil},
	{"path", `path "/releases/24.04.iso"`, `path "/releases/other.iso"`, nil},
	{"path-prefix", `path prefix "/releases/"`, `path prefix "/other/"`, nil},
	{"path-suffix", `path suffix ".iso"`, `path suffix ".img"`, nil},
	{"path-keyword", `path keyword "24.04"`, `path keyword "debian"`, nil},
	{"path-regex", `path regex "\\.iso$"`, `path regex "\\.deb$"`, nil},
	{"agent", `agent "curl/8.5.0"`, `agent "Wget/1.21"`, nil},
	{"agent-keyword", `agent keyword "curl"`, `agent keyword "Wget"`, nil},
	{"agent-regex", `agent regex "curl"`, `agent regex "Wget"`, nil},
	{"agent-prefix", `agent prefix "curl/"`, `agent prefix "Wget/"`, nil},
	{"agent-suffix", `agent suffix ".5.0"`, `agent suffix ".deb"`, nil},
	{"status", `status 200`, `status 404`, nil},
	{"status-class", `status class 2`, `status class 5`, nil},
	{"sent-at-least", `sent >= 1k`, `sent >= 10m`, nil},
	{"sent-at-most", `sent <= 10k`, `sent <= 100`, nil},
	{"size-at-least", `size >= 1k`, `size >= 10m`, knownSize},
	{"size-at-most", `size <= 10m`, `size <= 1k`, knownSize},
}

// benchFilter parses src or fails the benchmark: fixture construction is
// setup, never a silent zero filter.
func benchFilter(b *testing.B, src string) Filter {
	b.Helper()
	f, err := Parse(src)
	if err != nil {
		b.Fatalf("Parse(%q): %v", src, err)
	}
	return f
}

// benchChain builds n realistic rule filters (site CIDR, method, path prefix,
// minimum size); the last matches baseEvent exactly when matchLast is set.
func benchChain(b *testing.B, n int, matchLast bool) []Filter {
	b.Helper()
	filters := make([]Filter, n)
	for i := range filters {
		prefix := fmt.Sprintf("/mirror/%d/", i)
		if matchLast && i == n-1 {
			prefix = "/releases/"
		}
		src := fmt.Sprintf(`src 203.0.113.0/24 and method GET and path prefix "%s" and sent >= 1k`, prefix)
		filters[i] = benchFilter(b, src)
	}
	return filters
}

func BenchmarkFilterMatch(b *testing.B) {
	b.Run("leaf", func(b *testing.B) {
		for _, leaf := range leafCases {
			for _, path := range []struct {
				name string
				src  string
			}{{"match", leaf.match}, {"no-match", leaf.miss}} {
				b.Run(leaf.name+"/"+path.name, func(b *testing.B) {
					f := benchFilter(b, path.src)
					ev := baseEvent()
					if leaf.mut != nil {
						leaf.mut(&ev)
					}
					for b.Loop() {
						benchMatch = f.Match(&ev)
					}
				})
			}
		}
	})
	b.Run("chain", func(b *testing.B) {
		ev := baseEvent()
		for _, n := range []int{1, 10, 100} {
			for _, path := range []struct {
				name      string
				matchLast bool
			}{{"match", true}, {"no-match", false}} {
				filters := benchChain(b, n, path.matchLast)
				b.Run(fmt.Sprintf("%d-rules/%s", n, path.name), func(b *testing.B) {
					for b.Loop() {
						matched := false
						for _, f := range filters {
							if f.Match(&ev) {
								matched = true
								break // terminal rule: chain traversal ends
							}
						}
						benchMatch = matched
					}
				})
			}
		}
	})
}
