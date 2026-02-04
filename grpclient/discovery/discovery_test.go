package discovery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/picker"
)

func TestEndpointToAttrs_WeightKeyNotOverriddenByMetadata(t *testing.T) {
	ep := Endpoint{
		Addr:   "127.0.0.1:1",
		Weight: 7,
		Metadata: map[string]string{
			picker.WeightAttributeKey: "999", // should be ignored
			"k":                       "v",
		},
	}
	attrs := EndpointToAttrs(ep)
	if got := attrs.Value(picker.WeightAttributeKey); got != int32(7) {
		t.Fatalf("weight attr=%T(%v), want int32(7)", got, got)
	}
	if got := attrs.Value("k"); got != "v" {
		t.Fatalf("metadata attr=%T(%v), want %q", got, got, "v")
	}
}

func TestStaticDiscovery_SnapshotsAreIsolated(t *testing.T) {
	sd := NewStaticDiscoveryWithEndpoints([]Endpoint{{
		Addr:   "a",
		Weight: 1,
		Metadata: map[string]string{
			"k": "v",
		},
	}})

	ctx := context.Background()
	eps1, err := sd.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints error: %v", err)
	}
	if len(eps1) != 1 {
		t.Fatalf("len(eps1)=%d, want 1", len(eps1))
	}

	// Mutate returned slice and map; internal state must not change.
	eps1[0].Addr = "mutated"
	eps1[0].Metadata["k"] = "mutated"
	eps1 = append(eps1, Endpoint{Addr: "extra"})

	eps2, err := sd.GetEndpoints(ctx)
	if err != nil {
		t.Fatalf("GetEndpoints error: %v", err)
	}
	if len(eps2) != 1 {
		t.Fatalf("len(eps2)=%d, want 1", len(eps2))
	}
	if eps2[0].Addr != "a" {
		t.Fatalf("Addr=%q, want %q", eps2[0].Addr, "a")
	}
	if got := eps2[0].Metadata["k"]; got != "v" {
		t.Fatalf("Metadata[k]=%q, want %q", got, "v")
	}
}

func TestStaticDiscovery_WatchReturnsSnapshot(t *testing.T) {
	sd := NewStaticDiscoveryWithEndpoints([]Endpoint{{
		Addr:   "a",
		Weight: 1,
		Metadata: map[string]string{
			"k": "v",
		},
	}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := sd.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	ev := <-ch
	if ev.Type != EventTypeUpdate {
		t.Fatalf("event type=%v, want Update", ev.Type)
	}
	if len(ev.Endpoints) != 1 {
		t.Fatalf("len(ev.Endpoints)=%d, want 1", len(ev.Endpoints))
	}

	// Mutate event snapshot; internal state must not change.
	ev.Endpoints[0].Addr = "mutated"
	ev.Endpoints[0].Metadata["k"] = "mutated"

	eps, err := sd.GetEndpoints(context.Background())
	if err != nil {
		t.Fatalf("GetEndpoints error: %v", err)
	}
	if eps[0].Addr != "a" {
		t.Fatalf("Addr=%q, want %q", eps[0].Addr, "a")
	}
	if got := eps[0].Metadata["k"]; got != "v" {
		t.Fatalf("Metadata[k]=%q, want %q", got, "v")
	}
}

func TestStaticDiscovery_ConcurrentAccess(t *testing.T) {
	sd := NewStaticDiscoveryWithEndpoints([]Endpoint{{Addr: "a", Weight: 1}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Readers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = sd.GetEndpoints(ctx)
				}
			}
		}()
	}

	// Updaters
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n := 0
			for {
				select {
				case <-stop:
					return
				default:
					n++
					sd.UpdateEndpoints([]Endpoint{{Addr: "a", Weight: int32(n)}, {Addr: "b", Weight: int32(i + 1)}})
				}
			}
		}(i)
	}

	// Watchers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					cctx, ccancel := context.WithCancel(ctx)
					ch, err := sd.Watch(cctx)
					if err == nil {
						<-ch
					}
					ccancel()
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}
