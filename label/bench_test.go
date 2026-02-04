package label

import (
	"testing"
)

func BenchmarkMatch_Simple(b *testing.B) {
	pattern := "abc?ef"
	name := "abcdef"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Match(pattern, name)
	}
}

func BenchmarkMatch_StarBacktrack(b *testing.B) {
	pattern := "a*b*c*d*e*f*g*h*i*j*k*l*m*n*o*p*q*r*s*t*u*v*w*x*y*z"
	name := "abcdefghijklmnopqrstuvwxyz"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Match(pattern, name)
	}
}

func BenchmarkParseSelector(b *testing.B) {
	selector := "v@^1.1.0 || >=2, a in (x,y,z), b!=1, c~=foo*bar?baz"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ParseSelector(selector)
	}
}

func BenchmarkSelectorMatches(b *testing.B) {
	sel, err := ParseSelector("v@^1.1.0 || >=2, a in (x,y,z), b!=1, c~=foo*bar?baz")
	if err != nil {
		b.Fatal(err)
	}
	labels, err := ParseLabels([]string{"v=2.1.1", "a=y", "b=2", "c=foo---barXbaz"})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sel.Matches(labels)
	}
}

func BenchmarkParseLabels(b *testing.B) {
	raw := []string{"a=1", "a=2", "b=x", "c=foo-bar", "d=1234567890", "e=2021-10-18T10:20:41+08:00"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ParseLabels(raw)
	}
}
