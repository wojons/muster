package protocol

import (
	"fmt"
	"sync"

	"github.com/wojons/muster/pkg/errors"
)

// Registry resolves operations and executors by protocol. The default
// OpenAPI adapter is registered at startup; new adapters register alongside
// it and never change its behavior.
type Registry struct {
	mu       sync.RWMutex
	adapters map[Protocol]Adapter
}

// NewRegistry creates an empty protocol adapter Registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[Protocol]Adapter),
	}
}

// Register adds an adapter to the registry. Registering two adapters for
// the same protocol family is an error.
func (r *Registry) Register(a Adapter) error {
	if a == nil {
		return errors.New(errors.CodeValidation, "protocol: cannot register nil adapter")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[a.Protocol()]; exists {
		return errors.New(errors.CodeValidation, fmt.Sprintf("protocol: adapter for %q already registered", a.Protocol()))
	}
	if r.adapters == nil {
		r.adapters = make(map[Protocol]Adapter)
	}
	r.adapters[a.Protocol()] = a
	return nil
}

// Adapter returns the registered adapter for the given protocol family,
// or an AppError with CodeNotFound when none is registered.
func (r *Registry) Adapter(p Protocol) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[p]
	if !ok {
		return nil, errors.New(errors.CodeNotFound, fmt.Sprintf("protocol: no adapter registered for %q", p))
	}
	return a, nil
}
