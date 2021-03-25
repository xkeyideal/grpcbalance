## gRPC-go load balancing

The gRPC-go Require

* Go 1.15+
* gRPC 1.36.0

### How it works

The gRPC client-side load balancing to work need to main components, the [naming resolver](https://github.com/grpc/grpc/blob/master/doc/naming.md) and the [load balancing policy](https://github.com/grpc/grpc/blob/master/doc/load-balancing.md)

![load balancing work image](https://github.com/xkeyideal/grpcbalance/blob/master/examples/balancer.png)

The infra image source [itnext.io](https://itnext.io/on-grpc-load-balancing-683257c5b7b3)

### gRPC naming resolver & load balancing working principle

[On gRPC Load Balancing](https://itnext.io/on-grpc-load-balancing-683257c5b7b3)


### Running the Example Application

The gRPC client and server applications used in the example are based on the [proto/echo]((https://github.com/grpc/grpc-go/blob/master/examples/features/proto/echo/echo.proto)) & [load_balancing](https://github.com/grpc/grpc-go/blob/master/examples/features/load_balancing/README.md) examples found on the **gRPC-go examples** with the following modifications:

* The [server](https://github.com/xkeyideal/grpcbalance/blob/master/examples/server/server.go) running with port args
* The [client](https://github.com/xkeyideal/grpcbalance/blob/master/examples/client/client.go) used customized balance

### Support Balance Strategy

* round robin [balancer.RoundRobinBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/roundrobin.go#L11)
* weighted round robin [balancer.WeightedRobinBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/weightedroundrobin.go#L11)
* random weighted round robin [balancer.RandomWeightedRobinBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/randomweightedroundrobin.go#L11)
* minimum connection number [balancer.MinConnectBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/minconnect.go#L11)
* minimum response consume [balancer.MinRespTimeBalanceName](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/balancer/minresptime.go#L11), keep 10 response consume time, remove maximum and minimum and then take the average value.


### Customize Advanced Balancing Strategy

1. Modify [naming resolver](https://github.com/xkeyideal/grpcbalance/blob/master/grpclient/resolver/resolver.go) with your requirements, first set [attributes.Attributes](https://github.com/grpc/grpc-go/blob/master/attributes/attributes.go) for per endpoint address, second when one endpoint attributes.Attributes changed then update subConn state.

2. Implement yourself balancer & picker function, then based on [attributes.Attributes](https://github.com/grpc/grpc-go/blob/master/attributes/attributes.go) picker subConn in `Pick(balancer.PickInfo) (balancer.PickResult, error)`