package purekit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	// ErrCapabilityNotFound is returned when calling an unregistered capability.
	ErrCapabilityNotFound = errors.New("purekit: capability not found")
	// ErrCapabilityExists is returned when registering an already registered capability.
	ErrCapabilityExists = errors.New("purekit: capability already registered")
)

// CapabilityHandler handles synchronous or RPC-style requests on the bus.
type CapabilityHandler func(ctx context.Context, req any) (any, error)

// Bus is the thread-safe Capability Bus connecting decoupled plugins without
// direct package imports.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string]CapabilityHandler
	versions map[string]uint64
	nextVer  uint64
}

// NewBus constructs an empty Capability Bus.
func NewBus() *Bus {
	return &Bus{
		handlers: map[string]CapabilityHandler{},
		versions: map[string]uint64{},
	}
}

// Register binds a handler to a unique capability name (e.g. "image.preview").
func (b *Bus) Register(name string, handler CapabilityHandler) error {
	if name == "" {
		return fmt.Errorf("purekit: empty capability name")
	}
	if handler == nil {
		return fmt.Errorf("purekit: nil capability handler for %q", name)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.handlers == nil {
		b.handlers = map[string]CapabilityHandler{}
	}
	if b.versions == nil {
		b.versions = map[string]uint64{}
	}
	if _, exists := b.handlers[name]; exists {
		return fmt.Errorf("%w: %s", ErrCapabilityExists, name)
	}
	b.nextVer++
	b.handlers[name] = handler
	b.versions[name] = b.nextVer
	return nil
}

// Unregister removes a capability handler if present.
func (b *Bus) Unregister(name string) {
	b.mu.Lock()
	delete(b.handlers, name)
	delete(b.versions, name)
	b.mu.Unlock()
}

// Version returns the registration generation for name, or 0 if unregistered.
// Subscribers can compare versions across dynamic plugin restarts.
func (b *Bus) Version(name string) uint64 {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	v := b.versions[name]
	b.mu.RUnlock()
	return v
}

// Has reports whether a capability is currently registered.
func (b *Bus) Has(name string) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	_, ok := b.handlers[name]
	b.mu.RUnlock()
	return ok
}

// List returns sorted names of all currently registered capabilities.
func (b *Bus) List() []string {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	out := make([]string, 0, len(b.handlers))
	for k := range b.handlers {
		out = append(out, k)
	}
	b.mu.RUnlock()
	sort.Strings(out)
	return out
}

// Call invokes the handler registered for name without holding the bus lock.
func (b *Bus) Call(ctx context.Context, name string, req any) (any, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityNotFound, name)
	}
	b.mu.RLock()
	h := b.handlers[name]
	b.mu.RUnlock()
	if h == nil {
		return nil, fmt.Errorf("%w: %s", ErrCapabilityNotFound, name)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return h(ctx, req)
}
