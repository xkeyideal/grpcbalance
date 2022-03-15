// Copyright 2021 The etcd Authors
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

package resolver

import (
	"github.com/xkeyideal/grpcbalance/grpclient/endpoint"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const (
	Scheme = "customize-endpoints"
)

// DefineResolver is a Resolver (and resolver.Builder) that can be updated
// using SetEndpoints.
type CustomizeResolver struct {
	endpoints  []string
	attributes map[string]*attributes.Attributes
	cc         resolver.ClientConn
}

func NewCustomizeResolver(endpoints []string, attributes map[string]*attributes.Attributes) *CustomizeResolver {
	dr := &CustomizeResolver{
		endpoints:  endpoints,
		attributes: attributes,
	}
	resolver.Register(dr)
	return dr
}

func (r *CustomizeResolver) Scheme() string {
	return Scheme
}

// Build returns itself for Resolver, because it's both a builder and a resolver.
func (r *CustomizeResolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	// Populates endpoints stored in r into ClientConn (cc).
	r.cc = cc
	r.updateState()

	return r, nil
}

func (r *CustomizeResolver) SetEndpoints(endpoints []string, attributes map[string]*attributes.Attributes) {
	r.endpoints = endpoints
	r.attributes = attributes
	r.updateState()
}

func (r *CustomizeResolver) updateState() {
	addresses := make([]resolver.Address, len(r.endpoints))
	for i, ep := range r.endpoints {
		addr, serverName := endpoint.Interpret(ep)
		addresses[i] = resolver.Address{
			Addr:       addr,
			ServerName: serverName,
			Attributes: r.attributes[ep],
		}
	}
	state := resolver.State{
		Addresses: addresses,
	}

	r.cc.UpdateState(state)
}

// ResolveNow is a noop for Resolver.
// When balancer.UpdateClientConnState error just returned ErrBadResolverState will call resolver.ResolveNow
func (r *CustomizeResolver) ResolveNow(o resolver.ResolveNowOptions) {

}

// Close is a noop for Resolver.
func (r *CustomizeResolver) Close() {}
