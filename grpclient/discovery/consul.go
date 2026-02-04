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

package discovery

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/xkeyideal/grpcbalance/grpclient/picker"
)

// ConsulDiscovery implements Discovery interface using Consul as service registry
type ConsulDiscovery struct {
	client      *api.Client
	serviceName string
	tags        []string
	passingOnly bool
	mu          sync.RWMutex
	lastIndex   uint64
}

// ConsulDiscoveryConfig is the configuration for ConsulDiscovery
type ConsulDiscoveryConfig struct {
	// Address is the Consul agent address (e.g., "127.0.0.1:8500")
	Address string
	// ServiceName is the name of the service to discover
	ServiceName string
	// Tags are optional tags to filter services
	Tags []string
	// PassingOnly if true, only returns healthy services
	PassingOnly bool
	// Token is the ACL token (optional)
	Token string
	// Datacenter is the datacenter to query (optional)
	Datacenter string
}

// NewConsulDiscovery creates a new ConsulDiscovery
func NewConsulDiscovery(cfg ConsulDiscoveryConfig) (*ConsulDiscovery, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("service name is required")
	}

	consulCfg := api.DefaultConfig()
	if cfg.Address != "" {
		consulCfg.Address = cfg.Address
	}
	if cfg.Token != "" {
		consulCfg.Token = cfg.Token
	}
	if cfg.Datacenter != "" {
		consulCfg.Datacenter = cfg.Datacenter
	}

	client, err := api.NewClient(consulCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %v", err)
	}

	return &ConsulDiscovery{
		client:      client,
		serviceName: cfg.ServiceName,
		tags:        cfg.Tags,
		passingOnly: cfg.PassingOnly,
	}, nil
}

// Watch implements Discovery interface
// Consul supports long polling through blocking queries
func (c *ConsulDiscovery) Watch(ctx context.Context) (<-chan Event, error) {
	ch := make(chan Event, 1)

	// Get initial endpoints
	eps, err := c.GetEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	// Send initial event
	ch <- Event{Type: EventTypeUpdate, Endpoints: eps}

	// Start watching using blocking queries
	go func() {
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Use blocking query with WaitIndex
			c.mu.RLock()
			lastIndex := c.lastIndex
			c.mu.RUnlock()

			queryOpts := &api.QueryOptions{
				WaitIndex: lastIndex,
				WaitTime:  api.DefaultConfig().WaitTime,
			}

			var services []*api.ServiceEntry
			var meta *api.QueryMeta
			var err error

			if len(c.tags) > 0 {
				// Use first tag for filtering, Consul API limitation
				services, meta, err = c.client.Health().ServiceMultipleTags(
					c.serviceName,
					c.tags,
					c.passingOnly,
					queryOpts.WithContext(ctx),
				)
			} else {
				services, meta, err = c.client.Health().Service(
					c.serviceName,
					"",
					c.passingOnly,
					queryOpts.WithContext(ctx),
				)
			}

			if err != nil {
				if ctx.Err() != nil {
					return
				}
				select {
				case ch <- Event{Type: EventTypeError, Err: err}:
				case <-ctx.Done():
					return
				}
				continue
			}

			// Update lastIndex for next blocking query
			if meta.LastIndex > lastIndex {
				c.mu.Lock()
				c.lastIndex = meta.LastIndex
				c.mu.Unlock()

				endpoints := c.parseServices(services)
				select {
				case ch <- Event{Type: EventTypeUpdate, Endpoints: endpoints}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// GetEndpoints implements Discovery interface
func (c *ConsulDiscovery) GetEndpoints(ctx context.Context) ([]Endpoint, error) {
	queryOpts := &api.QueryOptions{}

	var services []*api.ServiceEntry
	var meta *api.QueryMeta
	var err error

	if len(c.tags) > 0 {
		services, meta, err = c.client.Health().ServiceMultipleTags(
			c.serviceName,
			c.tags,
			c.passingOnly,
			queryOpts.WithContext(ctx),
		)
	} else {
		services, meta, err = c.client.Health().Service(
			c.serviceName,
			"",
			c.passingOnly,
			queryOpts.WithContext(ctx),
		)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get services from consul: %v", err)
	}

	c.mu.Lock()
	c.lastIndex = meta.LastIndex
	c.mu.Unlock()

	return c.parseServices(services), nil
}

// parseServices converts Consul service entries to Endpoints
func (c *ConsulDiscovery) parseServices(services []*api.ServiceEntry) []Endpoint {
	endpoints := make([]Endpoint, 0, len(services))

	for _, svc := range services {
		addr := svc.Service.Address
		if addr == "" {
			addr = svc.Node.Address
		}

		port := svc.Service.Port
		endpoint := Endpoint{
			Addr:     fmt.Sprintf("%s:%d", addr, port),
			Weight:   1,
			Metadata: make(map[string]string),
		}

		// Extract weight from service meta if present
		if weightStr, ok := svc.Service.Meta[picker.WeightAttributeKey]; ok {
			if w, err := strconv.ParseInt(weightStr, 10, 32); err == nil && w > 0 {
				endpoint.Weight = int32(w)
			}
		}

		// Copy service meta to endpoint metadata
		for k, v := range svc.Service.Meta {
			if k == picker.WeightAttributeKey {
				continue
			}
			endpoint.Metadata[k] = v
		}

		// Add node info to metadata
		endpoint.Metadata["node"] = svc.Node.Node
		endpoint.Metadata["datacenter"] = svc.Node.Datacenter

		endpoints = append(endpoints, endpoint)
	}

	return endpoints
}

// Close implements Discovery interface
func (c *ConsulDiscovery) Close() error {
	// Consul client doesn't need explicit closing
	return nil
}

// Register registers a service instance to Consul
// This is a helper method for service registration
func (c *ConsulDiscovery) Register(ctx context.Context, id, addr string, port int, meta map[string]string, ttl string) error {
	registration := &api.AgentServiceRegistration{
		ID:      id,
		Name:    c.serviceName,
		Address: addr,
		Port:    port,
		Tags:    c.tags,
		Meta:    meta,
	}

	if ttl != "" {
		registration.Check = &api.AgentServiceCheck{
			TTL:                            ttl,
			DeregisterCriticalServiceAfter: "1m",
		}
	}

	return c.client.Agent().ServiceRegister(registration)
}

// Unregister removes a service instance from Consul
func (c *ConsulDiscovery) Unregister(ctx context.Context, id string) error {
	return c.client.Agent().ServiceDeregister(id)
}

// PassTTL updates the TTL check to passing state
func (c *ConsulDiscovery) PassTTL(checkID string, note string) error {
	return c.client.Agent().PassTTL(checkID, note)
}
