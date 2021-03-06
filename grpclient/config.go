package grpclient

import (
	"context"
	"math"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
)

var (
	// WaitForReady configures the action to take when an RPC is attempted on broken
	// connections or unreachable servers. If waitForReady is false, the RPC will fail
	// immediately. Otherwise, the RPC client will block the call until a
	// connection is available (or the call is canceled or times out) and will
	// retry the call if it fails due to a transient error.  gRPC will not retry if
	// data was written to the wire unless the server indicates it did not process
	// the data.  Please refer to
	// https://github.com/grpc/grpc/blob/master/doc/wait-for-ready.md.
	defaultWaitForReady = grpc.WaitForReady(false)

	// client-side request send limit, gRPC default is math.MaxInt32
	// Make sure that "client-side send limit < server-side default send/recv limit"
	// Same value as "embed.DefaultMaxRequestBytes" plus gRPC overhead bytes
	defaultMaxCallSendMsgSize = grpc.MaxCallSendMsgSize(2 * 1024 * 1024)

	// client-side response receive limit, gRPC default is 4MB
	// Make sure that "client-side receive limit >= server-side default send/recv limit"
	// because range response can easily exceed request send limits
	// Default to math.MaxInt32; writes exceeding server-side send limit fails anyway
	defaultMaxCallRecvMsgSize = grpc.MaxCallRecvMsgSize(math.MaxInt32)

	// client-side non-streaming retry limit, only applied to requests where server responds with
	// a error code clearly indicating it was unable to process the request such as codes.Unavailable.
	// If set to 0, retry is disabled.
	defaultUnaryMaxRetries uint = 100

	// client-side streaming retry limit, only applied to requests where server responds with
	// a error code clearly indicating it was unable to process the request such as codes.Unavailable.
	// If set to 0, retry is disabled.
	defaultStreamMaxRetries = ^uint(0) // max uint

	// client-side retry backoff wait between requests.
	defaultBackoffWaitBetween = 25 * time.Millisecond

	// client-side retry backoff default jitter fraction.
	defaultBackoffJitterFraction = 0.10
)

// defaultCallOpts defines a list of default "gRPC.CallOption".
// Some options are exposed to "clientv3.Config".
// Defaults will be overridden by the settings in "clientv3.Config".
var defaultCallOpts = []grpc.CallOption{defaultWaitForReady, defaultMaxCallSendMsgSize, defaultMaxCallRecvMsgSize}

type Config struct {
	// Endpoints is a list of URLs.
	Endpoints []string

	// Endpoints address attributes
	// see: https://github.com/grpc/grpc-go/blob/v1.36.0/attributes/attributes.go#L30
	Attributes map[string]*attributes.Attributes

	// load balance name, current support balancer.RoundRobinBalanceName, balancer.WeightedRobinBalanceName
	BalanceName string

	// AutoSyncInterval is the interval to update endpoints with its latest members.
	// 0 disables auto-sync. By default auto-sync is disabled.
	AutoSyncInterval time.Duration

	// DialTimeout is the timeout for failing to establish a connection.
	DialTimeout time.Duration

	// DialKeepAliveTime is the time after which client pings the server to see if
	// transport is alive.
	DialKeepAliveTime time.Duration

	// DialKeepAliveTimeout is the time that the client waits for a response for the
	// keep-alive probe. If the response is not received in this time, the connection is closed.
	DialKeepAliveTimeout time.Duration

	// MaxCallSendMsgSize is the client-side request send limit in bytes.
	// If 0, it defaults to 2.0 MiB (2 * 1024 * 1024).
	// Make sure that "MaxCallSendMsgSize" < server-side default send/recv limit.
	// ("--max-request-bytes" flag to etcd or "embed.Config.MaxRequestBytes").
	MaxCallSendMsgSize int

	// MaxCallRecvMsgSize is the client-side response receive limit.
	// If 0, it defaults to "math.MaxInt32", because range response can
	// easily exceed request send limits.
	// Make sure that "MaxCallRecvMsgSize" >= server-side default send/recv limit.
	// ("--max-request-bytes" flag to etcd or "embed.Config.MaxRequestBytes").
	MaxCallRecvMsgSize int

	// DialOptions is a list of dial options for the grpc client (e.g., for interceptors).
	// For example, pass "grpc.WithBlock()" to block until the underlying connection is up.
	// Without this, Dial returns immediately and connecting the server happens in background.
	DialOptions []grpc.DialOption

	// Context is the default client context; it can be used to cancel grpc dial out and
	// other operations that do not have an explicit context.
	Context context.Context

	PermitWithoutStream bool
}
