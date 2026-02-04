package grpclient

import (
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
)

func TestDialTimeout_TimesOutWhenUnreachable(t *testing.T) {
	start := time.Now()
	cfg := &Config{
		Endpoints:   []string{"127.0.0.1:1"},
		DialTimeout: 200 * time.Millisecond,
		BalanceName: balancer.RoundRobinBalanceName,
	}
	c, err := NewClient(cfg)
	if err == nil {
		_ = c.Close()
		t.Fatalf("expected dial timeout error, got nil")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("dial took too long: %v", d)
	}
}

func TestDialTimeout_SucceedsWhenServerAccepts(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	s := grpc.NewServer()
	defer s.Stop()
	go func() {
		_ = s.Serve(lis)
	}()

	cfg := &Config{
		Endpoints:   []string{lis.Addr().String()},
		DialTimeout: 2 * time.Second,
		BalanceName: balancer.RoundRobinBalanceName,
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	_ = c.Close()
}
