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

// Package discovery provides service discovery interfaces and implementations
// for dynamic endpoint management in gRPC client.
package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/xkeyideal/grpcbalance/grpclient/picker"

	"google.golang.org/grpc/attributes"
)

// Endpoint represents a service endpoint with optional metadata
type Endpoint struct {
	// Addr is the address of the endpoint (e.g., "192.168.1.1:8080")
	Addr string `json:"addr"`
	// Weight is the weight of the endpoint for weighted load balancing
	Weight int32 `json:"weight,omitempty"`
	// Metadata contains additional endpoint metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Event represents a service discovery event
type Event struct {
	// Type is the type of event
	Type EventType
	// Endpoints is the list of endpoints after this event
	Endpoints []Endpoint
	// Err contains the error if Type is EventTypeError
	Err error
}

// EventType represents the type of service discovery event
type EventType int

const (
	// EventTypeUpdate indicates endpoints have been updated
	EventTypeUpdate EventType = iota
	// EventTypeDelete indicates endpoints have been deleted
	EventTypeDelete
	// EventTypeError indicates an error occurred
	EventTypeError
)

func (t EventType) String() string {
	switch t {
	case EventTypeUpdate:
		return "Update"
	case EventTypeDelete:
		return "Delete"
	case EventTypeError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Discovery is the interface for service discovery
// Implement this interface to integrate with different service discovery systems
// such as etcd, Consul, Nacos, Kubernetes, etc.
type Discovery interface {
	// Watch starts watching for endpoint changes and sends events to the channel
	// The channel will be closed when the context is canceled or an unrecoverable error occurs
	Watch(ctx context.Context) (<-chan Event, error)

	// GetEndpoints returns the current list of endpoints
	// This can be used for initial endpoint loading
	GetEndpoints(ctx context.Context) ([]Endpoint, error)

	// Close closes the discovery client and releases resources
	Close() error
}

// DiscoveryFunc is a function type that implements Discovery interface for simple cases
type DiscoveryFunc func(ctx context.Context) ([]Endpoint, error)

// Watch implements Discovery interface - returns nil channel as DiscoveryFunc doesn't support watching
func (f DiscoveryFunc) Watch(ctx context.Context) (<-chan Event, error) {
	return nil, nil
}

// GetEndpoints implements Discovery interface
func (f DiscoveryFunc) GetEndpoints(ctx context.Context) ([]Endpoint, error) {
	return f(ctx)
}

// Close implements Discovery interface
func (f DiscoveryFunc) Close() error {
	return nil
}

// PollingDiscovery wraps a Discovery implementation with polling-based watching
// This is useful for discovery systems that don't support native watching
type PollingDiscovery struct {
	discovery Discovery
	interval  time.Duration
	mu        sync.RWMutex
	lastEps   []Endpoint
}

// NewPollingDiscovery creates a new PollingDiscovery
func NewPollingDiscovery(discovery Discovery, interval time.Duration) *PollingDiscovery {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &PollingDiscovery{
		discovery: discovery,
		interval:  interval,
	}
}

// Watch implements Discovery interface with polling
func (p *PollingDiscovery) Watch(ctx context.Context) (<-chan Event, error) {
	ch := make(chan Event, 1)

	// Get initial endpoints
	eps, err := p.discovery.GetEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.lastEps = eps
	p.mu.Unlock()

	// Send initial event
	ch <- Event{Type: EventTypeUpdate, Endpoints: eps}

	// Start polling goroutine
	go func() {
		defer close(ch)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				eps, err := p.discovery.GetEndpoints(ctx)
				if err != nil {
					select {
					case ch <- Event{Type: EventTypeError, Endpoints: nil, Err: err}:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Check if endpoints changed
				if p.hasChanged(eps) {
					p.mu.Lock()
					p.lastEps = eps
					p.mu.Unlock()

					select {
					case ch <- Event{Type: EventTypeUpdate, Endpoints: eps}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

// hasChanged checks if the endpoints have changed
func (p *PollingDiscovery) hasChanged(newEps []Endpoint) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(newEps) != len(p.lastEps) {
		return true
	}

	oldMap := make(map[string]Endpoint)
	for _, ep := range p.lastEps {
		oldMap[ep.Addr] = ep
	}

	for _, ep := range newEps {
		old, ok := oldMap[ep.Addr]
		if !ok {
			return true
		}
		if old.Weight != ep.Weight {
			return true
		}
	}

	return false
}

// GetEndpoints implements Discovery interface
func (p *PollingDiscovery) GetEndpoints(ctx context.Context) ([]Endpoint, error) {
	return p.discovery.GetEndpoints(ctx)
}

// Close implements Discovery interface
func (p *PollingDiscovery) Close() error {
	return p.discovery.Close()
}

// StaticDiscovery is a simple discovery implementation with static endpoints
type StaticDiscovery struct {
	mu        sync.RWMutex
	endpoints []Endpoint
}

// NewStaticDiscovery creates a new StaticDiscovery
func NewStaticDiscovery(addrs []string) *StaticDiscovery {
	eps := make([]Endpoint, len(addrs))
	for i, addr := range addrs {
		eps[i] = Endpoint{Addr: addr, Weight: 1}
	}
	return &StaticDiscovery{endpoints: eps}
}

// NewStaticDiscoveryWithEndpoints creates a new StaticDiscovery with full Endpoint objects
func NewStaticDiscoveryWithEndpoints(endpoints []Endpoint) *StaticDiscovery {
	return &StaticDiscovery{endpoints: cloneEndpoints(endpoints)}
}

// Watch implements Discovery interface - static discovery doesn't support watching
func (s *StaticDiscovery) Watch(ctx context.Context) (<-chan Event, error) {
	ch := make(chan Event, 1)
	s.mu.RLock()
	snap := cloneEndpoints(s.endpoints)
	s.mu.RUnlock()
	ch <- Event{Type: EventTypeUpdate, Endpoints: snap}
	// Keep channel open but don't send more events
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// GetEndpoints implements Discovery interface
func (s *StaticDiscovery) GetEndpoints(ctx context.Context) ([]Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneEndpoints(s.endpoints), nil
}

// Close implements Discovery interface
func (s *StaticDiscovery) Close() error {
	return nil
}

// UpdateEndpoints allows updating endpoints in StaticDiscovery (for testing)
func (s *StaticDiscovery) UpdateEndpoints(endpoints []Endpoint) {
	s.mu.Lock()
	s.endpoints = cloneEndpoints(endpoints)
	s.mu.Unlock()
}

func cloneEndpoints(endpoints []Endpoint) []Endpoint {
	if len(endpoints) == 0 {
		return nil
	}
	out := make([]Endpoint, len(endpoints))
	copy(out, endpoints)
	for i := range out {
		if out[i].Metadata == nil {
			continue
		}
		m2 := make(map[string]string, len(out[i].Metadata))
		for k, v := range out[i].Metadata {
			m2[k] = v
		}
		out[i].Metadata = m2
	}
	return out
}

// EndpointToAttrs converts discovery.Endpoint to attributes.Attributes
func EndpointToAttrs(ep Endpoint) *attributes.Attributes {
	// Use int32 for weight to match picker expectations
	attrs := attributes.New(picker.WeightAttributeKey, int32(ep.Weight))

	// Attach full metadata map for filters that expect it (e.g., picker.MetadataFilter).
	// Also keep per-key attributes below for convenience and selector matching.
	if len(ep.Metadata) > 0 {
		mm := make(map[string]string, len(ep.Metadata))
		for k, v := range ep.Metadata {
			// Prevent overriding reserved keys with user metadata.
			if k == picker.WeightAttributeKey || k == picker.MetadataFilterKey {
				continue
			}
			mm[k] = v
		}
		if len(mm) > 0 {
			attrs = attrs.WithValue(picker.MetadataFilterKey, mm)
		}
	}
	for k, v := range ep.Metadata {
		// Prevent overriding reserved keys with a string value.
		if k == picker.WeightAttributeKey || k == picker.MetadataFilterKey {
			continue
		}
		attrs = attrs.WithValue(k, v)
	}
	return attrs
}

// EndpointsToAddrs extracts addresses from endpoints
func EndpointsToAddrs(endpoints []Endpoint) []string {
	addrs := make([]string, len(endpoints))
	for i, ep := range endpoints {
		addrs[i] = ep.Addr
	}
	return addrs
}

// EndpointsToAttrsMap converts endpoints to address-to-attributes map
func EndpointsToAttrsMap(endpoints []Endpoint) map[string]*attributes.Attributes {
	m := make(map[string]*attributes.Attributes)
	for _, ep := range endpoints {
		m[ep.Addr] = EndpointToAttrs(ep)
	}
	return m
}
