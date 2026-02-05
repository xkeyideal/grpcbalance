// Copyright 2016 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Example: Service Discovery with gRPC client
//
// This example demonstrates how to use the service discovery feature
// to dynamically update endpoints without restarting the client.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient"
	"github.com/xkeyideal/grpcbalance/grpclient/balancer"
	"github.com/xkeyideal/grpcbalance/grpclient/discovery"
)

func main() {
	// Example 1: Static Discovery (for testing)
	staticDiscoveryExample()

	// Example 2: Custom Discovery with polling
	customDiscoveryExample()

	// Note: Etcd and Consul discovery examples are in separate files
	// with build tags. To use them:
	//
	// For etcd:
	//   go get go.etcd.io/etcd/client/v3
	//   go build -tags etcd ./...
	//
	// For consul:
	//   go get github.com/hashicorp/consul/api
	//   go build -tags consul ./...
}

// staticDiscoveryExample demonstrates using StaticDiscovery
func staticDiscoveryExample() {
	fmt.Println("=== Static Discovery Example ===")

	// Create a static discovery with initial endpoints
	staticDiscovery := discovery.NewStaticDiscovery([]string{
		"127.0.0.1:8081",
		"127.0.0.1:8082",
		"127.0.0.1:8083",
	})

	cfg := &grpclient.Config{
		BalanceName:          balancer.RoundRobinBalanceName,
		DialTimeout:          5 * time.Second,
		DialKeepAliveTime:    30 * time.Second,
		DialKeepAliveTimeout: 10 * time.Second,
		EnableHealthCheck:    true,
		Discovery:            staticDiscovery,
		OnEndpointsUpdate: func(endpoints []discovery.Endpoint) {
			log.Printf("Endpoints updated: %v", discovery.EndpointsToAddrs(endpoints))
		},
	}

	client, err := grpclient.NewClient(cfg)
	if err != nil {
		log.Printf("Failed to create client (expected if no server): %v", err)
		return
	}
	defer client.Close()

	fmt.Printf("Client created with endpoints: %v\n", client.Endpoints())
}

// customDiscoveryExample demonstrates using a custom discovery function with polling
func customDiscoveryExample() {
	fmt.Println("\n=== Custom Discovery Example ===")

	// Simulate a custom service registry
	registeredEndpoints := []discovery.Endpoint{
		{Addr: "127.0.0.1:8081", Weight: 1},
		{Addr: "127.0.0.1:8082", Weight: 2},
	}

	// Create a custom discovery using DiscoveryFunc
	customDiscovery := discovery.DiscoveryFunc(func(ctx context.Context) ([]discovery.Endpoint, error) {
		// In real scenarios, this would fetch from your service registry
		// e.g., database, file, HTTP API, etc.
		return registeredEndpoints, nil
	})

	// Wrap with polling for automatic updates
	pollingDiscovery := discovery.NewPollingDiscovery(customDiscovery, 10*time.Second)

	cfg := &grpclient.Config{
		BalanceName:          balancer.WeightedRobinBalanceName,
		DialTimeout:          5 * time.Second,
		DialKeepAliveTime:    30 * time.Second,
		DialKeepAliveTimeout: 10 * time.Second,
		Discovery:            pollingDiscovery,
		EnableHealthCheck:    true,
		OnEndpointsUpdate: func(endpoints []discovery.Endpoint) {
			log.Printf("Endpoints updated via polling: %v", endpoints)
		},
	}

	client, err := grpclient.NewClient(cfg)
	if err != nil {
		log.Printf("Failed to create client (expected if no server): %v", err)
		return
	}
	defer client.Close()

	fmt.Printf("Client created with endpoints: %v\n", client.Endpoints())
}

// HTTPDiscovery is a simple HTTP-based discovery implementation
type HTTPDiscovery struct {
	url string
}

func NewHTTPDiscovery(url string) *HTTPDiscovery {
	return &HTTPDiscovery{url: url}
}

func (h *HTTPDiscovery) Watch(ctx context.Context) (<-chan discovery.Event, error) {
	// HTTP doesn't support native watching, return nil to use polling
	return nil, nil
}

func (h *HTTPDiscovery) GetEndpoints(ctx context.Context) ([]discovery.Endpoint, error) {
	// In real implementation, make HTTP request to fetch endpoints
	// Example:
	// resp, err := http.Get(h.url)
	// if err != nil {
	//     return nil, err
	// }
	// defer resp.Body.Close()
	// var endpoints []discovery.Endpoint
	// json.NewDecoder(resp.Body).Decode(&endpoints)
	// return endpoints, nil

	// Placeholder
	return []discovery.Endpoint{
		{Addr: "127.0.0.1:8081", Weight: 1},
	}, nil
}

func (h *HTTPDiscovery) Close() error {
	return nil
}
