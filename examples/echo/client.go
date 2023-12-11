package main

import (
	"context"
	"log"
	"math"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

var grpcOptions = []grpc.DialOption{
	grpc.WithTransportCredentials(insecure.NewCredentials()), // grpc.WithInsecure(),
	// grpc.WithBlock(),
	grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor([]grpc_retry.CallOption{
		grpc_retry.WithMax(1),
		grpc_retry.WithBackoff(grpc_retry.BackoffExponential(100 * time.Millisecond)),
		grpc_retry.WithCodes(codes.Canceled, codes.Internal, codes.Unavailable),
	}...)),
	grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                15 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}),
}

var callOptions = []grpc.CallOption{
	grpc.WaitForReady(true),
	grpc.MaxCallSendMsgSize(20 * 1024 * 1024),
	grpc.MaxCallRecvMsgSize(math.MaxInt32),
}

func callUnaryEcho(c pb.EchoClient, message string) {
	pctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	md := metadata.Pairs()
	ctx := metadata.NewOutgoingContext(pctx, md)
	r, err := c.UnaryEcho(ctx, &pb.EchoRequest{Message: message}, callOptions...)
	if err != nil {
		log.Println("could not greet: %v", err)
		return
	}
	log.Println(r.Message)
}

func dial(addr string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpcOptions...)

	return conn, err
}

func main() {
	conn, err := dial("127.0.0.1:50053")
	if err != nil {
		panic(err)
	}
	log.Println("state:", conn.GetState().String())

	hwc := pb.NewEchoClient(conn)

	st := time.Now()
	callUnaryEcho(hwc, "this is examples/load_balancing")
	log.Println("cost:", time.Now().Sub(st).Milliseconds())
}
