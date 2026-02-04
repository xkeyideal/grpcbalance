package endpoint

import "testing"

func TestInterpret_NoPanic(t *testing.T) {
	cases := []string{
		"",
		"127.0.0.1:1",
		"http://127.0.0.1:2379",
		"https://example.com:443",
		"unix:///tmp/abc.sock",
		"unix://tmp/abc.sock",
		"unix:tmp/abc.sock",
		"unixs:///tmp/abc.sock",
		"unixs://tmp/abc.sock",
		"unixs:tmp/abc.sock",
		"bad://",
		"://",
		"http://",
		"http://:bad",
	}

	for _, ep := range cases {
		ep := ep
		t.Run(ep, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Interpret panicked for %q: %v", ep, r)
				}
			}()
			_, _ = Interpret(ep)
		})
	}
}

func TestInterpret_KnownMappings(t *testing.T) {
	addr, serverName := Interpret("http://127.0.0.1:2379")
	if addr != "127.0.0.1:2379" {
		t.Fatalf("addr=%q, want %q", addr, "127.0.0.1:2379")
	}
	if serverName != "127.0.0.1" {
		t.Fatalf("serverName=%q, want %q", serverName, "127.0.0.1")
	}

	addr, serverName = Interpret("unix:///tmp/abc.sock")
	if addr != "unix:///tmp/abc.sock" {
		t.Fatalf("addr=%q, want %q", addr, "unix:///tmp/abc.sock")
	}
	if serverName != "abc.sock" {
		t.Fatalf("serverName=%q, want %q", serverName, "abc.sock")
	}
}
