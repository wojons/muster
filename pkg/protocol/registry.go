package protocol

import (
	stderrors "errors"

	"github.com/wojons/muster/pkg/errors"
)

// ErrUnknownAdapter is returned by Registry.Adapter when no adapter is
// registered for the requested protocol.
var ErrUnknownAdapter = stderrors.New("no adapter registered for protocol")

// Registry resolves operations and executors by protocol. The default
// OpenAPI adapter is registered at startup; new adapters register alongside
// it and never change its behavior (specs/PROTOCOLS.md §1.3).
type Registry struct {
	adapters map[Protocol]Adapter
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[Protocol]Adapter)}
}

// Register adds an adapter to the registry. Registering a duplicate
// Protocol returns an error.
func (r *Registry) Register(a Adapter) error {
	if a == nil {
		return errors.New(errors.CodeValidation, "cannot register nil adapter")
	}
	if r.adapters == nil {
		r.adapters = make(map[Protocol]Adapter)
	}
	if _, dup := r.adapters[a.Protocol()]; dup {
		return errors.Errorf(errors.CodeValidation, "adapter for protocol %q already registered", a.Protocol())
	}
	r.adapters[a.Protocol()] = a
	return nil
}

// Adapter returns the adapter registered for the given protocol, or an
// AppError with CodeNotFound when none is registered.
func (r *Registry) Adapter(p Protocol) (Adapter, error) {
	a, ok := r.adapters[p]
	if !ok {
		return nil, errors.Wrap(ErrUnknownAdapter, errors.CodeNotFound,
			"no adapter registered for protocol "+string(p))
	}
	return a, nil
}
