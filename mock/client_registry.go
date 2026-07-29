package mock

//fusa:req REQ-MCR-001
//fusa:req REQ-MCR-002
//fusa:req REQ-MCR-003
//fusa:req REQ-MCR-004
//fusa:req REQ-MCR-005

import (
	"fmt"
	"sync"
)

// ClientRegistry is a caller-keyed collection of *Client, mirroring
// udp.Registry's own "re-key by a caller-chosen label" shape (ROADMAP.md
// Milestone 54) for this package's in-process fakes.
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]*Client
	closed  bool
}

// NewClientRegistry returns an empty ClientRegistry.
func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{clients: make(map[string]*Client)}
}

// Register adds client under key. It returns ErrAlreadyExists if key is
// already registered, or ErrClosed if the registry has been closed.
func (r *ClientRegistry) Register(key string, client *Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("rcp/mock: client registry: %w", ErrClosed)
	}
	if _, ok := r.clients[key]; ok {
		return fmt.Errorf("rcp/mock: client registry key %s: %w", key, ErrAlreadyExists)
	}
	r.clients[key] = client
	return nil
}

// Deregister closes and removes the *Client registered under key.
func (r *ClientRegistry) Deregister(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clients[key]
	if !ok {
		return fmt.Errorf("rcp/mock: client registry key %s: %w", key, ErrNotFound)
	}
	delete(r.clients, key)
	return c.Close()
}

// Lookup returns the *Client registered under key.
func (r *ClientRegistry) Lookup(key string) (*Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, fmt.Errorf("rcp/mock: client registry: %w", ErrClosed)
	}
	c, ok := r.clients[key]
	if !ok {
		return nil, fmt.Errorf("rcp/mock: client registry key %s: %w", key, ErrNotFound)
	}
	return c, nil
}

// Clients returns every currently registered *Client.
func (r *ClientRegistry) Clients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Client, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, c)
	}
	return out
}

// Close closes every registered *Client and marks the registry closed to
// further Register calls. Safe to call multiple times.
func (r *ClientRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var last error
	for key, c := range r.clients {
		if err := c.Close(); err != nil {
			last = err
		}
		delete(r.clients, key)
	}
	return last
}
