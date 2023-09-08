package main

import (
	"context"
	"io"
	"log"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/metadata"
)

var addrs = []string{"10.181.22.31:50051", "10.181.22.31:50052"}

func callUnaryEcho(c pb.EchoClient, message string) {
	pctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	md := metadata.Pairs()
	ctx := metadata.NewOutgoingContext(pctx, md)
	r, err := c.UnaryEcho(ctx, &pb.EchoRequest{Message: message})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Println(r.Message)
}

func makeRPCs(cc *grpc.ClientConn, n int) {
	hwc := pb.NewEchoClient(cc)
	for i := 0; i < n; i++ {
		callUnaryEcho(hwc, "this is examples/load_balancing")
		time.Sleep(3 * time.Second)
	}
}

func serverStream(cc *grpc.ClientConn) {
	hwc := pb.NewEchoClient(cc)
	pctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	md := metadata.Pairs()
	ctx := metadata.NewOutgoingContext(pctx, md)
	stream, err := hwc.ServerStreamingEcho(ctx, &pb.EchoRequest{Message: "load_balancing server stream"})
	if err != nil {
		panic(err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}

		log.Printf(resp.Message)
	}
}

func main() {
	attrs := make(map[string]*attributes.Attributes)
	for i, addr := range addrs {
		attrs[addr] = attributes.New(picker.WeightAttributeKey, int32(i+1))
	}

	// https://mdnice.com/writing/5631c3f1ac4047a381daadc81b08f546
	grpcCfg := &grpclient.Config{
		Endpoints:   addrs,
		BalanceName: balancer.RoundRobinBalanceName,
		Attributes:  attrs,

		DialTimeout: 10 * time.Second,

		// 如果没有 activity， 则每隔 10s 发送一个 ping 包
		DialKeepAliveTime: 10 * time.Second,
		// 如果 ping ack 1s 之内未返回则认为连接已断开
		DialKeepAliveTimeout: 2 * time.Second,
		// 如果没有 active 的 stream， 是否允许发送 ping
		PermitWithoutStream: true,
	}

	grpclient, err := grpclient.NewClient(grpcCfg)
	if err != nil {
		log.Panic(err)
	}

	defer grpclient.Close()

	for i := 0; i < 50; i++ {
		cc := grpclient.ActiveConnection()
		hwc := pb.NewEchoClient(cc)
		callUnaryEcho(hwc, "this is examples/load_balancing")
		time.Sleep(3 * time.Second)
	}

	// makeRPCs(grpclient.ActiveConnection(), 100)
	serverStream(grpclient.ActiveConnection())
}
