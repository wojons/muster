# Domain: WASM Module

## Overview

WASM module entities define compiled WebAssembly modules with their metadata, signatures, and capabilities.

## Entities

### Module

Core WASM module entity.

```go
type Module struct {
    ID            string           `json:"id" validate:"required,uuid4"`
    InventoryID   string           `json:"inventory_id" validate:"required,uuid4"`
    Name          string           `json:"name" validate:"required,min=3,max=100,alphanum_underscore"`
    Description   string           `json:"description" validate:"max=500"`
    Version       string           `json:"version" validate:"required,semver"`
    Runtime       ModuleRuntime    `json:"runtime" validate:"required"`
    Binary        ModuleBinary     `json:"binary" validate:"required"`
    Signature     ModuleSignature  `json:"signature,omitempty"`
    Capabilities  []Capability     `json:"capabilities" validate:"dive"`
    Interfaces    []Interface      `json:"interfaces" validate:"dive"`
    Metadata      ModuleMetadata   `json:"metadata"`
    Status        ModuleStatus     `json:"status" validate:"required"`
    CreatedAt     time.Time        `json:"created_at"`
    UpdatedAt     time.Time        `json:"updated_at"`
}

type ModuleStatus string

const (
    ModuleStatusPending     ModuleStatus = "pending"
    ModuleStatusCompiled    ModuleStatus = "compiled"
    ModuleStatusValidated   ModuleStatus = "validated"
    ModuleStatusSigned      ModuleStatus = "signed"
    ModuleStatusActive      ModuleStatus = "active"
    ModuleStatusDeprecated  ModuleStatus = "deprecated"
    ModuleStatusRevoked     ModuleStatus = "revoked"
)
```

### ModuleRuntime

Runtime environment configuration.

```go
type ModuleRuntime struct {
    Engine      RuntimeEngine `json:"engine" validate:"required,oneof=wasmer wasmtime wazero"`
    Version     string        `json:"version" validate:"required,semver"`
    Memory      MemoryConfig  `json:"memory"`
    WASI        bool          `json:"wasi"`
    Threads     bool          `json:"threads"`
    SIMD        bool          `json:"simd"`
    BulkMemory  bool          `json:"bulk_memory"`
    ReferenceTypes bool       `json:"reference_types"`
}

type RuntimeEngine string

const (
    RuntimeEngineWasmer  RuntimeEngine = "wasmer"
    RuntimeEngineWasmtime RuntimeEngine = "wasmtime"
    RuntimeEngineWazero  RuntimeEngine = "wazero"
)

type MemoryConfig struct {
    MinPages    uint32 `json:"min_pages" validate:"gte=1,lte=65536"`
    MaxPages    uint32 `json:"max_pages" validate:"gtefield=MinPages,lte=65536"`
    Shared      bool   `json:"shared"`
}
```

### ModuleBinary

Binary artifact metadata.

```go
type ModuleBinary struct {
    URL         string           `json:"url" validate:"required,url"`
    Size        int64            `json:"size" validate:"required,gt=0,lt=52428800"`
    Format      BinaryFormat     `json:"format" validate:"required,oneof=wasm wat"`
    Checksum    BinaryChecksum   `json:"checksum" validate:"required"`
    Compression CompressionType  `json:"compression,omitempty" validate:"omitempty,oneof=gzip brotli zstd"`
}

type BinaryFormat string

const (
    BinaryFormatWASM BinaryFormat = "wasm"
    BinaryFormatWAT  BinaryFormat = "wat"
)

type BinaryChecksum struct {
    Algorithm string `json:"algorithm" validate:"required,oneof=sha256 sha384 sha512"`
    Value     string `json:"value" validate:"required,len=64|len=96|len=128"`
}

type CompressionType string

const (
    CompressionGzip   CompressionType = "gzip"
    CompressionBrotli CompressionType = "brotli"
    CompressionZstd   CompressionType = "zstd"
)
```

### ModuleSignature

Cryptographic signature for module verification.

```go
type ModuleSignature struct {
    Algorithm   SigAlgorithm     `json:"algorithm" validate:"required,oneof=ed25519 ecdsa rsa"`
    PublicKey   string           `json:"public_key" validate:"required,base64"`
    Signature   string           `json:"signature" validate:"required,base64"`
    Timestamp   time.Time        `json:"timestamp"`
    Signer      string           `json:"signer" validate:"required,max=200"`
    KeyID       string           `json:"key_id" validate:"required,max=100"`
}

type SigAlgorithm string

const (
    SigAlgorithmEd25519 SigAlgorithm = "ed25519"
    SigAlgorithmECDSA   SigAlgorithm = "ecdsa"
    SigAlgorithmRSA     SigAlgorithm = "rsa"
)
```

### Capability

Module capability declarations.

```go
type Capability struct {
    ID          string            `json:"id" validate:"required,uuid4"`
    Type        CapabilityType    `json:"type" validate:"required"`
    Name        string            `json:"name" validate:"required,max=100"`
    Description string            `json:"description" validate:"max=500"`
    Schema      json.RawMessage   `json:"schema,omitempty"`
    Required    bool              `json:"required"`
}

type CapabilityType string

const (
    CapabilityTypeHTTP     CapabilityType = "http"
    CapabilityTypeFile     CapabilityType = "file"
    CapabilityTypeNetwork  CapabilityType = "network"
    CapabilityTypeCrypto   CapabilityType = "crypto"
    CapabilityTypeEnv      CapabilityType = "env"
    CapabilityTypeStdio    CapabilityType = "stdio"
    CapabilityTypeClock    CapabilityType = "clock"
    CapabilityTypeRandom   CapabilityType = "random"
)
```

### Interface

Module interface definitions for host interactions.

```go
type Interface struct {
    ID          string           `json:"id" validate:"required,uuid4"`
    Name        string           `json:"name" validate:"required,max=100"`
    Version     string           `json:"version" validate:"required,semver"`
    Functions   []Function       `json:"functions" validate:"dive"`
    Imports     []Import         `json:"imports" validate:"dive"`
    Exports     []Export         `json:"exports" validate:"dive"`
}

type Function struct {
    Name        string           `json:"name" validate:"required,max=100"`
    Params      []Param          `json:"params" validate:"dive"`
    Returns     []Param          `json:"returns" validate:"dive"`
    Async       bool             `json:"async"`
}

type Param struct {
    Name        string           `json:"name" validate:"required,max=100"`
    Type        string           `json:"type" validate:"required,oneof=i32 i64 f32 f64 externref funcref"`
    Optional    bool             `json:"optional"`
}

type Import struct {
    Module      string           `json:"module" validate:"required,max=100"`
    Name        string           `json:"name" validate:"required,max=100"`
    Type        string           `json:"type" validate:"required"`
}

type Export struct {
    Name        string           `json:"name" validate:"required,max=100"`
    Type        string           `json:"type" validate:"required,oneof=function memory table global"`
}
```

### ModuleMetadata

Module metadata for organization.

```go
type ModuleMetadata struct {
    Language    string           `json:"language" validate:"max=50"`
    Compiler    string           `json:"compiler" validate:"max=100"`
    BuildTime   time.Time        `json:"build_time"`
    SDKVersion  string           `json:"sdk_version" validate:"omitempty,semver"`
    Target      string           `json:"target" validate:"max=100"`
    Features    []string         `json:"features" validate:"dive,max=50"`
}
```

## Signature Validation

```go
func (m *Module) ValidateSignature() error {
    if m.Signature.Algorithm == "" {
        return errors.New("module requires signature for activation")
    }
    
    pubKey, err := base64.StdEncoding.DecodeString(m.Signature.PublicKey)
    if err != nil {
        return fmt.Errorf("invalid public key encoding: %w", err)
    }
    
    sig, err := base64.StdEncoding.DecodeString(m.Signature.Signature)
    if err != nil {
        return fmt.Errorf("invalid signature encoding: %w", err)
    }
    
    data := m.canonicalData()
    
    switch m.Signature.Algorithm {
    case SigAlgorithmEd25519:
        return m.verifyEd25519(pubKey, sig, data)
    case SigAlgorithmECDSA:
        return m.verifyECDSA(pubKey, sig, data)
    case SigAlgorithmRSA:
        return m.verifyRSA(pubKey, sig, data)
    default:
        return fmt.Errorf("unsupported signature algorithm: %s", m.Signature.Algorithm)
    }
}
```
