package grpclient

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/discovery"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testEchoServer struct {
	pb.UnimplementedEchoServer
	port string
}

func (s *testEchoServer) UnaryEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	_ = ctx
	return &pb.EchoResponse{Message: fmt.Sprintf("%s (from :%s)", req.GetMessage(), s.port)}, nil
}

type memDiscovery struct {
	mu        sync.RWMutex
	endpoints []discovery.Endpoint
	watchers  map[*memWatcher]struct{}
	closed    bool
}

type memWatcher struct {
	ch     chan discovery.Event
	closed bool
}

func newMemDiscovery(endpoints []discovery.Endpoint) *memDiscovery {
	d := &memDiscovery{watchers: make(map[*memWatcher]struct{})}
	d.endpoints = cloneDiscoveryEndpointsForTest(endpoints)
	return d
}

func (d *memDiscovery) Watch(ctx context.Context) (<-chan discovery.Event, error) {
	w := &memWatcher{ch: make(chan discovery.Event, 8)}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		close(w.ch)
		w.closed = true
		return w.ch, nil
	}
	d.watchers[w] = struct{}{}
	snap := cloneDiscoveryEndpointsForTest(d.endpoints)
	d.mu.Unlock()

	// initial snapshot
	w.ch <- discovery.Event{Type: discovery.EventTypeUpdate, Endpoints: snap}

	go func() {
		<-ctx.Done()
		d.mu.Lock()
		d.closeWatcherLocked(w)
		d.mu.Unlock()
	}()

	return w.ch, nil
}

func (d *memDiscovery) GetEndpoints(ctx context.Context) ([]discovery.Endpoint, error) {
	_ = ctx
	d.mu.RLock()
	defer d.mu.RUnlock()
	return cloneDiscoveryEndpointsForTest(d.endpoints), nil
}

func (d *memDiscovery) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	for w := range d.watchers {
		d.closeWatcherLocked(w)
	}
	return nil
}

func (d *memDiscovery) closeWatcherLocked(w *memWatcher) {
	if w == nil || w.closed {
		return
	}
	delete(d.watchers, w)
	close(w.ch)
	w.closed = true
}

func (d *memDiscovery) UpdateEndpoints(endpoints []discovery.Endpoint) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	d.endpoints = cloneDiscoveryEndpointsForTest(endpoints)
	snap := cloneDiscoveryEndpointsForTest(d.endpoints)
	for w := range d.watchers {
		if w.closed {
			continue
		}
		select {
		case w.ch <- discovery.Event{Type: discovery.EventTypeUpdate, Endpoints: snap}:
		default:
		}
	}
}

func cloneDiscoveryEndpointsForTest(endpoints []discovery.Endpoint) []discovery.Endpoint {
	if len(endpoints) == 0 {
		return nil
	}
	out := make([]discovery.Endpoint, len(endpoints))
	copy(out, endpoints)
	for i := range out {
		if out[i].Metadata == nil {
			continue
		}
		m2 := make(map[string]string, len(out[i].Metadata))
		for k, v := range out[i].Metadata {
			m2[k] = v
		}
		out[i].Metadata = m2
	}
	return out
}

func TestDiscoveryMetadataUpdate_RefreshesPickerAttributes(t *testing.T) {
	// Start two echo servers.
	startServer := func() (addr string, stop func()) {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		port := strings.Split(lis.Addr().String(), ":")[1]
		s := grpc.NewServer()
		pb.RegisterEchoServer(s, &testEchoServer{port: port})
		go func() { _ = s.Serve(lis) }()
		return lis.Addr().String(), func() {
			s.Stop()
			_ = lis.Close()
		}
	}

	addr1, stop1 := startServer()
	defer stop1()
	addr2, stop2 := startServer()
	defer stop2()

	d := newMemDiscovery([]discovery.Endpoint{
		{Addr: addr1, Weight: 1, Metadata: map[string]string{"env": "prod", "lane": "canary"}},
		{Addr: addr2, Weight: 1, Metadata: map[string]string{"env": "prod", "lane": "stable"}},
	})
	defer d.Close()

	cfg := &Config{
		BalanceName:      balancer.RoundRobinBalanceName,
		EnableNodeFilter: true,
		Discovery:        d,
		DialTimeout:      2 * time.Second,
	}

	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer c.Close()

	echo := pb.NewEchoClient(c.ActiveConnection())

	fCanary, err := picker.LabelSelectorFilter("env=prod, lane=canary")
	if err != nil {
		t.Fatalf("LabelSelectorFilter: %v", err)
	}

	ctx1 := picker.WithNodeFilter(context.Background(), fCanary)
	ctx1, cancel := context.WithTimeout(ctx1, 2*time.Second)
	defer cancel()
	resp, err := echo.UnaryEcho(ctx1, &pb.EchoRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("UnaryEcho before update: %v", err)
	}
	if !strings.Contains(resp.GetMessage(), addr1[strings.LastIndex(addr1, ":")+1:]) {
		// Only addr1 is canary.
		t.Fatalf("expected canary server response, got %q", resp.GetMessage())
	}

	// Update: flip canary -> stable on addr1, so selector should match none.
	d.UpdateEndpoints([]discovery.Endpoint{
		{Addr: addr1, Weight: 1, Metadata: map[string]string{"env": "prod", "lane": "stable"}},
		{Addr: addr2, Weight: 1, Metadata: map[string]string{"env": "prod", "lane": "stable"}},
	})
	// Allow watcher -> resolver -> balancer/picker to refresh.
	time.Sleep(200 * time.Millisecond)

	ctx2 := picker.WithNodeFilter(context.Background(), fCanary)
	ctx2, cancel2 := context.WithTimeout(ctx2, 2*time.Second)
	defer cancel2()
	_, err = echo.UnaryEcho(ctx2, &pb.EchoRequest{Message: "hello2"})
	if err == nil {
		t.Fatalf("expected error after update (no canary endpoints), got nil")
	}
	st, ok := status.FromError(err)
	if ok {
		// Different gRPC paths may map "no subconn" differently; accept Unavailable/DeadlineExceeded.
		if st.Code() != codes.Unavailable && st.Code() != codes.DeadlineExceeded {
			t.Fatalf("unexpected status code after update: %s err=%v", st.Code(), err)
		}
	}
}
