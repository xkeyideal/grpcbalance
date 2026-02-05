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
	"sync"

	"github.com/xkeyideal/grpcbalance/grpclient/endpoint"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const (
	Scheme = "x-customize-endpoints" // must be not conflict with other registered resolver schemes
)

// CustomizeResolver is a Resolver (and resolver.Builder) that can be updated
// using SetEndpoints. It's similar to grpc-go's manual resolver but with
// endpoint-specific attribute support.
//
// Note: Every instance of the CustomizeResolver may only ever be used with a
// single grpc.ClientConn. Otherwise, bad things will happen.
type CustomizeResolver struct {
	// mu guards access to cc and lastSeenState fields.
	mu sync.Mutex
	cc resolver.ClientConn
	// Storing the most recent state update makes this resolver resilient to
	// restarts, which is possible with channel idleness.
	lastSeenState *resolver.State

	endpoints  []string
	attributes map[string]*attributes.Attributes
}

func NewCustomizeResolver(endpoints []string, attributes map[string]*attributes.Attributes) *CustomizeResolver {
	dr := &CustomizeResolver{
		endpoints:  endpoints,
		attributes: attributes,
	}
	// NOTE: Do NOT call resolver.Register here.
	// grpc-go's resolver registry is intended to be mutated only at init time and
	// is not thread-safe. This resolver is expected to be passed via
	// grpc.WithResolvers(...), which makes global registration unnecessary.
	return dr
}

func (r *CustomizeResolver) Scheme() string {
	return Scheme
}

// Build returns itself for Resolver, because it's both a builder and a resolver.
func (r *CustomizeResolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cc = cc
	// If we have a previously stored state (e.g., from InitialState or SetEndpoints
	// before Build was called), apply it now.
	if r.lastSeenState != nil {
		r.cc.UpdateState(*r.lastSeenState)
	} else {
		// Otherwise, build state from current endpoints.
		r.updateStateLocked()
	}

	return r, nil
}

// SetEndpoints updates the resolver's endpoints and pushes the new state to
// the ClientConn. If Build has not been called yet, the state is stored for
// later use when Build is called.
func (r *CustomizeResolver) SetEndpoints(endpoints []string, attrs map[string]*attributes.Attributes) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = endpoints
	r.attributes = attrs
	r.updateStateLocked()
}

// updateStateLocked builds the resolver state from endpoints and updates the
// ClientConn. Must be called with r.mu held.
func (r *CustomizeResolver) updateStateLocked() {
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

	// Store the state for resilience to channel idleness restarts.
	r.lastSeenState = &state

	// Only update if cc is set (Build has been called).
	if r.cc != nil {
		r.cc.UpdateState(state)
	}
}

// ResolveNow is a noop for this resolver.
// When balancer.UpdateClientConnState returns ErrBadResolverState,
// gRPC will call resolver.ResolveNow to trigger re-resolution.
func (r *CustomizeResolver) ResolveNow(o resolver.ResolveNowOptions) {
	// For static resolver, we can simply re-push the last known state.
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cc != nil && r.lastSeenState != nil {
		r.cc.UpdateState(*r.lastSeenState)
	}
}

// Close cleans up the resolver.
func (r *CustomizeResolver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cc = nil
}
