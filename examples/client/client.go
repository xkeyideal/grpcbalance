package main

import (
	"context"
	"log"
	"time"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
)

var addrs = []string{"localhost:50051", "localhost:50052", "localhost:50053", "localhost:50054"}

func callUnaryEcho(c pb.EchoClient, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
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

func main() {
	attrs := make(map[string]*attributes.Attributes)
	for i, addr := range addrs {
		attrs[addr] = attributes.New(picker.WeightAttributeKey, int32(i+1))
	}

	grpcCfg := &grpclient.Config{
		Endpoints:            addrs,
		BalanceName:          balancer.RoundRobinBalanceName,
		Attributes:           attrs,
		DialTimeout:          2 * time.Second,
		DialKeepAliveTime:    2 * time.Second,
		DialKeepAliveTimeout: 2 * time.Second,
		PermitWithoutStream:  false,
	}

	grpclient, err := grpclient.NewClient(grpcCfg)
	if err != nil {
		log.Panic(err)
	}

	defer grpclient.Close()

	makeRPCs(grpclient.ActiveConnection(), 100)
}
