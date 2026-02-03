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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdDiscovery implements Discovery interface using etcd as service registry
type EtcdDiscovery struct {
	client    *clientv3.Client
	keyPrefix string
	lease     clientv3.Lease
}

// EtcdDiscoveryConfig is the configuration for EtcdDiscovery
type EtcdDiscoveryConfig struct {
	// Endpoints is the list of etcd endpoints
	Endpoints []string
	// KeyPrefix is the prefix for service keys (e.g., "/services/myapp/")
	KeyPrefix string
	// DialTimeout is the timeout for connecting to etcd
	DialTimeout time.Duration
	// Username for etcd authentication (optional)
	Username string
	// Password for etcd authentication (optional)
	Password string
}

// NewEtcdDiscovery creates a new EtcdDiscovery
// KeyPrefix should be in format "/services/{service-name}/"
func NewEtcdDiscovery(cfg EtcdDiscoveryConfig) (*EtcdDiscovery, error) {
	if len(cfg.Endpoints) == 0 {
		return nil, fmt.Errorf("at least one etcd endpoint is required")
	}

	if cfg.KeyPrefix == "" {
		return nil, fmt.Errorf("key prefix is required")
	}

	dialTimeout := cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	etcdCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: dialTimeout,
	}

	if cfg.Username != "" {
		etcdCfg.Username = cfg.Username
		etcdCfg.Password = cfg.Password
	}

	client, err := clientv3.New(etcdCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}

	return &EtcdDiscovery{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
	}, nil
}

// EtcdEndpointValue is the JSON structure stored in etcd for each endpoint
type EtcdEndpointValue struct {
	Addr     string            `json:"addr"`
	Weight   int               `json:"weight,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Watch implements Discovery interface
// Supports automatic reconnection on watch disconnection
func (e *EtcdDiscovery) Watch(ctx context.Context) (<-chan Event, error) {
	ch := make(chan Event, 1)

	// Get initial endpoints
	eps, err := e.GetEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	// Send initial event
	ch <- Event{Type: EventTypeUpdate, Endpoints: eps}

	// Start watching with reconnection support
	go func() {
		defer close(ch)

		var (
			watchCh       clientv3.WatchChan
			retryInterval = time.Second
			maxRetry      = 30 * time.Second
		)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Create or recreate watch channel
			if watchCh == nil {
				watchCh = e.client.Watch(ctx, e.keyPrefix, clientv3.WithPrefix())
			}

			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					// Watch channel closed, attempt reconnection
					watchCh = nil

					// Notify about potential error
					select {
					case ch <- Event{Type: EventTypeError, Err: fmt.Errorf("watch channel closed, reconnecting")}:
					case <-ctx.Done():
						return
					}

					// Wait before retry with exponential backoff
					select {
					case <-time.After(retryInterval):
						retryInterval = minDuration(retryInterval*2, maxRetry)
					case <-ctx.Done():
						return
					}

					// Fetch current endpoints after reconnection
					eps, err := e.GetEndpoints(ctx)
					if err != nil {
						continue
					}

					select {
					case ch <- Event{Type: EventTypeUpdate, Endpoints: eps}:
						// Reset retry interval on success
						retryInterval = time.Second
					case <-ctx.Done():
						return
					}
					continue
				}

				// Check for watch errors (e.g., compaction, connection issues)
				if resp.Err() != nil {
					// Handle specific errors
					if resp.Canceled {
						// Watch was canceled, need to recreate
						watchCh = nil
					}

					select {
					case ch <- Event{Type: EventTypeError, Err: resp.Err()}:
					case <-ctx.Done():
						return
					}

					// If watch channel is still valid but had an error, continue
					if watchCh != nil {
						continue
					}

					// Wait before retry
					select {
					case <-time.After(retryInterval):
						retryInterval = minDuration(retryInterval*2, maxRetry)
					case <-ctx.Done():
						return
					}
					continue
				}

				// Reset retry interval on successful response
				retryInterval = time.Second

				// Fetch all current endpoints on any change
				// This is simpler and more reliable than incremental updates
				eps, err := e.GetEndpoints(ctx)
				if err != nil {
					select {
					case ch <- Event{Type: EventTypeError, Err: err}:
					case <-ctx.Done():
						return
					}
					continue
				}

				select {
				case ch <- Event{Type: EventTypeUpdate, Endpoints: eps}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// minDuration returns the smaller of two durations
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// GetEndpoints implements Discovery interface
func (e *EtcdDiscovery) GetEndpoints(ctx context.Context) ([]Endpoint, error) {
	resp, err := e.client.Get(ctx, e.keyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints from etcd: %v", err)
	}

	endpoints := make([]Endpoint, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var val EtcdEndpointValue
		if err := json.Unmarshal(kv.Value, &val); err != nil {
			// Try to parse as plain address
			addr := strings.TrimSpace(string(kv.Value))
			if addr != "" {
				endpoints = append(endpoints, Endpoint{
					Addr:   addr,
					Weight: 1,
				})
			}
			continue
		}

		if val.Addr == "" {
			continue
		}

		weight := val.Weight
		if weight <= 0 {
			weight = 1
		}

		endpoints = append(endpoints, Endpoint{
			Addr:     val.Addr,
			Weight:   weight,
			Metadata: val.Metadata,
		})
	}

	return endpoints, nil
}

// Close implements Discovery interface
func (e *EtcdDiscovery) Close() error {
	return e.client.Close()
}

// Register registers an endpoint to etcd with optional TTL
// This is a helper method for service registration
func (e *EtcdDiscovery) Register(ctx context.Context, endpoint Endpoint, ttl int64) error {
	key := e.keyPrefix + endpoint.Addr

	val := EtcdEndpointValue{
		Addr:     endpoint.Addr,
		Weight:   endpoint.Weight,
		Metadata: endpoint.Metadata,
	}

	data, err := json.Marshal(val)
	if err != nil {
		return err
	}

	if ttl > 0 {
		// Create lease
		leaseResp, err := e.client.Grant(ctx, ttl)
		if err != nil {
			return err
		}

		_, err = e.client.Put(ctx, key, string(data), clientv3.WithLease(leaseResp.ID))
		if err != nil {
			return err
		}

		// Keep lease alive
		keepAliveCh, err := e.client.KeepAlive(ctx, leaseResp.ID)
		if err != nil {
			return err
		}

		// Drain the keep alive responses in background
		go func() {
			for range keepAliveCh {
			}
		}()
	} else {
		_, err = e.client.Put(ctx, key, string(data))
		if err != nil {
			return err
		}
	}

	return nil
}

// Unregister removes an endpoint from etcd
func (e *EtcdDiscovery) Unregister(ctx context.Context, addr string) error {
	key := e.keyPrefix + addr
	_, err := e.client.Delete(ctx, key)
	return err
}
