package picker

import (
	"testing"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

func TestLabelSelectorFilter_StringAttr(t *testing.T) {
	f, err := LabelSelectorFilter("system.ip=127.0.0.1")
	if err != nil {
		t.Fatalf("LabelSelectorFilter parse: %v", err)
	}

	info := SubConnInfo{Address: resolver.Address{Attributes: attributes.New("system.ip", "127.0.0.1")}}
	if !f(info) {
		t.Fatalf("expected match")
	}

	info2 := SubConnInfo{Address: resolver.Address{Attributes: attributes.New("system.ip", "127.0.0.2")}}
	if f(info2) {
		t.Fatalf("expected not match")
	}
}

func TestLabelSelectorFilter_SliceAttr(t *testing.T) {
	f, err := LabelSelectorFilter("tags in (b,c)")
	if err != nil {
		t.Fatalf("LabelSelectorFilter parse: %v", err)
	}

	info := SubConnInfo{Address: resolver.Address{Attributes: attributes.New("tags", []string{"a", "b"})}}
	if !f(info) {
		t.Fatalf("expected match")
	}

	info2 := SubConnInfo{Address: resolver.Address{Attributes: attributes.New("tags", []string{"a"})}}
	if f(info2) {
		t.Fatalf("expected not match")
	}
}

func TestLabelSelectorFilter_NumberAttr(t *testing.T) {
	f, err := LabelSelectorFilter("x_customize_weight>2")
	if err != nil {
		t.Fatalf("LabelSelectorFilter parse: %v", err)
	}

	info := SubConnInfo{Address: resolver.Address{Attributes: attributes.New(WeightAttributeKey, int32(3))}}
	if !f(info) {
		t.Fatalf("expected match")
	}

	info2 := SubConnInfo{Address: resolver.Address{Attributes: attributes.New(WeightAttributeKey, int32(1))}}
	if f(info2) {
		t.Fatalf("expected not match")
	}
}

func TestLabelSelectorFilter_EmptySelectorWithNilAttrs(t *testing.T) {
	f, err := LabelSelectorFilter("")
	if err != nil {
		t.Fatalf("LabelSelectorFilter parse: %v", err)
	}
	info := SubConnInfo{Address: resolver.Address{Attributes: nil}}
	if !f(info) {
		t.Fatalf("expected match")
	}
}

func TestLabelSelectorFilter_MetadataMapFallback(t *testing.T) {
	f, err := LabelSelectorFilter("system.ip=127.0.0.1")
	if err != nil {
		t.Fatalf("LabelSelectorFilter parse: %v", err)
	}

	attrs := attributes.New(MetadataFilterKey, map[string]string{"system.ip": "127.0.0.1"})
	info := SubConnInfo{Address: resolver.Address{Attributes: attrs}}
	if !f(info) {
		t.Fatalf("expected match")
	}
}

func TestLabelSelectorFilter_MoreSelectors(t *testing.T) {
	type tc struct {
		name     string
		selector string
		attrs    *attributes.Attributes
		want     bool
	}

	base := attributes.New("system.ip", "127.0.0.1").
		WithValue("env", "prod").
		WithValue("tags", []string{"a", "b"}).
		WithValue("id", "foo123barZ").
		WithValue("v", "2.1.1").
		WithValue("a", "0xff").
		WithValue(WeightAttributeKey, int32(3))

	cases := []tc{
		{"exact match", "system.ip=127.0.0.1", base, true},
		{"exists", "system.ip", base, true},
		{"does not exist", "!system.ip", base, false},
		{"in (single)", "env in (prod,staging)", base, true},
		{"not equals", "env!=staging", base, true},
		{"notin when present", "env notin (dev,staging)", base, true},
		{"notin when missing", "missing notin (x,y)", base, true},
		{"slice in", "tags in (b,c)", base, true},
		{"pattern", "id~=foo*bar?", base, true},
		{"semver", "v@^1.1.0 || >=2", base, true},
		{"numeric decimal from int32", "x_customize_weight>2", base, true},
		{"numeric hex string", "a>0xfe", base, true},
		{"and", "system.ip=127.0.0.1, env=prod, tags in (b)", base, true},
		{"and fail", "system.ip=127.0.0.1, env=staging", base, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			f, err := LabelSelectorFilter(c.selector)
			if err != nil {
				t.Fatalf("LabelSelectorFilter(%q) parse: %v", c.selector, err)
			}
			info := SubConnInfo{Address: resolver.Address{Attributes: c.attrs}}
			got := f(info)
			if got != c.want {
				t.Fatalf("selector %q got=%v want=%v", c.selector, got, c.want)
			}
		})
	}
}
