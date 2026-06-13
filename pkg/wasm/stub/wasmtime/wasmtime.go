package wasmtime

// Config holds WebAssembly engine configuration.
type Config struct{}

// NewConfig creates a new Config.
func NewConfig() *Config {
	return &Config{}
}

// SetConsumeFuel enables or disables fuel consumption.
func (c *Config) SetConsumeFuel(consume bool) {}

// SetEpochInterruption enables or disables epoch-based interruption.
func (c *Config) SetEpochInterruption(enable bool) {}

// Engine is a WebAssembly compilation and execution engine.
type Engine struct{}

// NewEngine creates a new Engine with default config.
func NewEngine() *Engine {
	return &Engine{}
}

// NewEngineWithConfig creates a new Engine with the given Config.
func NewEngineWithConfig(config *Config) *Engine {
	return &Engine{}
}

// IncrementEpoch increments the engine's epoch counter.
func (e *Engine) IncrementEpoch() {}

// Store represents a WebAssembly store.
type Store struct{}

// NewStore creates a new Store.
func NewStore(engine *Engine) *Store {
	return &Store{}
}

// Limiter sets memory/table limits on the store.
func (s *Store) Limiter(memorySize, tableElements, instances, tables, memories int64) {}

// SetEpochDeadline sets the epoch deadline.
func (s *Store) SetEpochDeadline(deadline int64) {}

// AddFuel adds fuel to the store for execution metering.
func (s *Store) AddFuel(fuel uint64) error {
	return nil
}

// Module is a compiled WebAssembly module.
type Module struct{}

// NewModule compiles WebAssembly bytes into a Module.
func NewModule(engine *Engine, bytes []byte) (*Module, error) {
	return &Module{}, nil
}

// Exports returns the module's exports.
func (m *Module) Exports() []Export {
	return nil
}

// Export describes a module export.
type Export struct {
	name string
}

// Name returns the export's name.
func (e Export) Name() string {
	return e.name
}

// Instance is a WebAssembly instance.
type Instance struct{}

// NewInstance creates a new Instance from a Module.
func NewInstance(store *Store, module *Module, imports []AsExtern) (*Instance, error) {
	return &Instance{}, nil
}

// GetFunc retrieves an exported function by name.
func (i *Instance) GetFunc(store *Store, name string) *Func {
	return &Func{}
}

// Func is an exported WebAssembly function.
type Func struct{}

// Call invokes the function with the given arguments.
func (f *Func) Call(store *Store, args ...interface{}) (interface{}, error) {
	return nil, nil
}

// AsExtern is an interface for types that can be used as imports.
type AsExtern interface {
	asExtern()
}

// Val is a WebAssembly value.
type Val struct {
	kind  ValKind
	value interface{}
}

// ValKind represents the type of a Val.
type ValKind int

const (
	KindI32 ValKind = iota
	KindI64
	KindF32
	KindF64
)

// ValI32 creates an i32 WebAssembly value.
func ValI32(v int32) Val {
	return Val{kind: KindI32, value: v}
}

// ValI64 creates an i64 WebAssembly value.
func ValI64(v int64) Val {
	return Val{kind: KindI64, value: v}
}

// ValF32 creates an f32 WebAssembly value.
func ValF32(v float32) Val {
	return Val{kind: KindF32, value: v}
}

// ValF64 creates an f64 WebAssembly value.
func ValF64(v float64) Val {
	return Val{kind: KindF64, value: v}
}

// Wat2Wasm compiles WAT text format to WASM binary.
func Wat2Wasm(wat string) ([]byte, error) {
	return nil, nil
}
