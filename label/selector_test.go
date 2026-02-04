package label

import "testing"

func TestSelectors_Match(t *testing.T) {
	type args struct {
		labels Labels
	}
	tests := []struct {
		name     string
		selector string
		args     []string
		result   bool
	}{
		{"semver", "v@=1.1.0-rc4", []string{"v=1.1.0-rc4"}, true},
		{"semver", "v@=1.1.0-rc5", []string{"v=1.1.0-rc4"}, false},
		{"semver", "v@^1.1.0", []string{"v=1.1.0-rc4"}, false},
		{"semver", "v@^1.1.0", []string{"v=1.1.1"}, true},
		{"semver", "v@^1.1.0", []string{"v=1.2.1"}, true},
		{"semver", "v@~1.1.0", []string{"v=1.1.1"}, true},
		{"semver", "v@~1.1.0", []string{"v=1.2.1"}, false},
		{"semver", "v@^1.1.0 || >2", []string{"v=3.1.1"}, true},
		{"semver", "v@^1.1.0 || >2", []string{"v=2.1.1"}, false},
		{"semver", "v@^1.1.0 || >=2", []string{"v=2.1.1"}, true},
		{"and", "v@^1.1.0 || >=2, a=1", []string{"v=2.1.1", "a=1"}, true},
		{"and", "v@^1.1.0 || >=2, a=1", []string{"v=2.1.1"}, false},
		{"wildcard", "a~=123456*", []string{"a=1234567890"}, true},
		{"wildcard", "a~=123456789?", []string{"a=1234567890"}, true},
		{"wildcard", "a~=1234?67890", []string{"a=1234567890"}, true},
		{"wildcard", "a~=12345?67890", []string{"a=1234567890"}, false},
		{"wildcard", "a~=1234567890?", []string{"a=1234567890"}, false},
		{"wildcard", "a~=124?67890", []string{"a=1234567890"}, false},
		{"wildcard", "a~=1234*67890", []string{"a=1234567890"}, true},
		{"wildcard", "a~=12345*67890", []string{"a=1234567890"}, true},
		{"equal", "a==1234567890", []string{"a=1234567890"}, true},
		{"equal", "a==234567890", []string{"a=1234567890"}, false},
		{"equal", "a==123456789", []string{"a=1234567890"}, false},
		{"equal", "a==123457890", []string{"a=1234567890"}, false},
		{"no-equal", "a!=1234567890", []string{"a=1234567890"}, false},
		{"no-equal", "a!=234567890", []string{"a=1234567890"}, true},
		{"no-equal", "a!=123456789", []string{"a=1234567890"}, true},
		{"no-equal", "a!=123457890", []string{"a=1234567890"}, true},
		{"exist", "a", []string{"a=1234567890"}, true},
		{"exist", "aa", []string{"a=1234567890"}, false},
		{"no-exist", "!a", []string{"a=1234567890"}, false},
		{"no-exist", "!aa", []string{"a=1234567890"}, true},
		{"in", "a in (a, bb, ccc)", []string{"a=aa"}, false},
		{"in", "a in (a, bb, ccc)", []string{"a=b"}, false},
		{"in", "a in (a, bb, ccc)", []string{"a=cc"}, false},
		{"in", "a in (a, bb, ccc)", []string{"a=a"}, true},
		{"in", "a in (a, bb, ccc)", []string{"a=bb"}, true},
		{"in", "a in (a, bb, ccc)", []string{"a=ccc"}, true},
		{"in", "a notin (a, bb, ccc)", []string{"a=a"}, false},
		{"in", "a notin (a, bb, ccc)", []string{"a=bb"}, false},
		{"in", "a notin (a, bb, ccc)", []string{"a=ccc"}, false},
		{"number", "a > 1", []string{"a=2"}, true},
		{"number", "a > 1", []string{"a=2", "a=1"}, true},
		{"number", "a > 1", []string{"a=1"}, false},
		{"number", "a>1", []string{"a=2"}, true},
		{"number", "a<10,a>0", []string{"a=2"}, true},
		{"number", "a<10,a>0", []string{"a=20"}, false},
		{"number", "a<10,a>5", []string{"a=1"}, false},
		{"number", "a<-10", []string{"a=1"}, false},
		{"number", "a>0xfe", []string{"a=0xff"}, true},
		{"number", "a>0b01", []string{"a=0b10"}, true},
		{"number", "a>0o10", []string{"a=0x9"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector, err := ParseSelector(tt.selector)
			if err != nil {
				t.Fatal(err)
			}
				labels, err := ParseLabels(tt.args)
				if err != nil {
					t.Fatal(err)
				}
			if got := selector.Matches(labels); got != tt.result {
				t.Errorf("Match() = %v, want %v", got, tt.result)
			}
		})
	}
}

func TestEqISelectors(t *testing.T) {
	s1, err := ParseSelectors([]string{"a=1", "b=2"})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := ParseSelectors([]string{"a=1", "b=2"})
	if err != nil {
		t.Fatal(err)
	}
	s3, err := ParseSelectors([]string{"a=1"})
	if err != nil {
		t.Fatal(err)
	}

	if !EqISelectors(s1, s2) {
		t.Fatalf("EqISelectors(s1,s2)=false, want true")
	}
	if EqISelectors(s1, s3) {
		t.Fatalf("EqISelectors(s1,s3)=true, want false")
	}
	if EqISelectors(nil, s1) {
		t.Fatalf("EqISelectors(nil,s1)=true, want false")
	}
	if !EqISelectors(nil, nil) {
		t.Fatalf("EqISelectors(nil,nil)=false, want true")
	}
}
