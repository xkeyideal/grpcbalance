package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	pb "github.com/xkeyideal/grpcbalance/examples/proto/echo"
)

var (
	addrs = []string{":50051", ":50052", ":50053", ":50054"}
)

type ecServer struct {
	pb.UnimplementedEchoServer
	addr string
}

func (s *ecServer) UnaryEcho(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	return &pb.EchoResponse{Message: fmt.Sprintf("%s (from %s)", req.Message, s.addr)}, nil
}

func (s *ecServer) ServerStreamingEcho(req *pb.EchoRequest, stream pb.Echo_ServerStreamingEchoServer) error {
	for i := 0; i < 5; i++ {
		err := stream.Send(&pb.EchoResponse{
			Message: fmt.Sprintf("%s (from %s)", req.Message, s.addr),
		})
		if err != nil {
			return err
		}

		time.Sleep(2 * time.Second)
	}

	return nil
}

func startServer(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	kaep := keepalive.EnforcementPolicy{
		// 如果客户端两次 ping 的间隔小于 5s，则关闭连接
		MinTime:             5 * time.Second,
		PermitWithoutStream: true, // 即使没有 active stream, 也允许 ping keepalive.ServerParameters
	}

	kasp := keepalive.ServerParameters{
		// 如果一个 client 空闲超过 10s, 则发送一个 ping 请求
		Time: 10 * time.Second,
		// 如果 ping 请求 2s 内未收到回复, 则认为该连接已断开
		Timeout:               2 * time.Second,
		MaxConnectionIdle:     15 * time.Second, // 如果一个 client 空闲超过 15s, 发送一个 GOAWAY, 为了防止同一时间发送大量 GOAWAY, 会在 15s 时间间隔上下浮动 15*10%, 即 15+1.5 或者 15-1.5
		MaxConnectionAge:      30 * time.Second, // 如果任意连接存活时间超过 30s, 发送一个 GOAWAY
		MaxConnectionAgeGrace: 5 * time.Second,  // 在强制关闭连接之间, 允许有 5s 的时间完成 pending 的 rpc 请求
	}

	gopts := []grpc.ServerOption{
		grpc.KeepaliveEnforcementPolicy(kaep),
		grpc.KeepaliveParams(kasp),
	}

	s := grpc.NewServer(gopts...)

	pb.RegisterEchoServer(s, &ecServer{addr: addr})
	log.Printf("serving on %s\n", addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func main() {
	if len(os.Args) <= 1 {
		log.Fatal("input arg $1 port")
	}

	port, err := strconv.ParseUint(os.Args[1], 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	startServer(fmt.Sprintf(":%d", port))
}
