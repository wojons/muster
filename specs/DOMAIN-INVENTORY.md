# Domain: Inventory

## Overview

Inventory entities manage the discovery, storage, and versioning of API specifications and WASM modules.

## Entities

### Inventory

Core inventory container for organizing specifications and modules.

```go
type Inventory struct {
    ID          string                 `json:"id" validate:"required,uuid4"`
    Name        string                 `json:"name" validate:"required,min=3,max=100"`
    Description string                 `json:"description" validate:"max=500"`
    Type        InventoryType          `json:"type" validate:"required,oneof=specification module hybrid"`
    Source      InventorySource        `json:"source" validate:"required"`
    Versions    []Version              `json:"versions" validate:"dive"`
    Metadata    InventoryMetadata      `json:"metadata"`
    Status      InventoryStatus        `json:"status" validate:"required"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
    Tags        []string               `json:"tags" validate:"dive,min=1,max=50"`
    Labels      map[string]string      `json:"labels" validate:"dive,keys,max=50,endkeys,max=100"`
}

type InventoryType string

const (
    InventoryTypeSpec      InventoryType = "specification"
    InventoryTypeModule    InventoryType = "module"
    InventoryTypeHybrid    InventoryType = "hybrid"
)

type InventoryStatus string

const (
    InventoryStatusDraft     InventoryStatus = "draft"
    InventoryStatusActive    InventoryStatus = "active"
    InventoryStatusArchived  InventoryStatus = "archived"
    InventoryStatusError     InventoryStatus = "error"
)
```

### InventorySource

Source configuration for inventory discovery.

```go
type InventorySource struct {
    Type     SourceType `json:"type" validate:"required,oneof=local git http s3"`
    URI      string     `json:"uri" validate:"required,uri"`
    Branch   string     `json:"branch,omitempty"`
    Path     string     `json:"path,omitempty" validate:"max=500"`
    Auth     SourceAuth `json:"auth,omitempty"`
    Schedule string     `json:"schedule,omitempty" validate:"omitempty,cron"`
}

type SourceType string

const (
    SourceTypeLocal SourceType = "local"
    SourceTypeGit   SourceType = "git"
    SourceTypeHTTP  SourceType = "http"
    SourceTypeS3    SourceType = "s3"
)

type SourceAuth struct {
    Type   AuthType `json:"type" validate:"oneof=basic token ssh oauth"`
    Secret string   `json:"-" validate:"max=4096"`
}
```

### Version

Version entity tracking inventory iterations.

```go
type Version struct {
    ID          string          `json:"id" validate:"required,uuid4"`
    InventoryID string          `json:"inventory_id" validate:"required,uuid4"`
    Semantic    string          `json:"semantic" validate:"required,semver"`
    Commit      string          `json:"commit" validate:"omitempty,len=40"`
    Message     string          `json:"message" validate:"max=500"`
    Author      string          `json:"author" validate:"max=100"`
    Artifacts   []Artifact      `json:"artifacts" validate:"dive"`
    Status      VersionStatus   `json:"status" validate:"required"`
    CreatedAt   time.Time       `json:"created_at"`
    PublishedAt *time.Time      `json:"published_at,omitempty"`
}

type VersionStatus string

const (
    VersionStatusPending    VersionStatus = "pending"
    VersionStatusValidating VersionStatus = "validating"
    VersionStatusValid      VersionStatus = "valid"
    VersionStatusInvalid    VersionStatus = "invalid"
    VersionStatusPublished  VersionStatus = "published"
)

type Artifact struct {
    ID        string         `json:"id" validate:"required,uuid4"`
    Type      ArtifactType   `json:"type" validate:"required,oneof=openapi wasm schema"`
    Path      string         `json:"path" validate:"required,max=500"`
    Size      int64          `json:"size" validate:"gte=0"`
    Checksum  string         `json:"checksum" validate:"required,len=64"`
    MimeType  string         `json:"mime_type" validate:"required"`
}

type ArtifactType string

const (
    ArtifactTypeOpenAPI ArtifactType = "openapi"
    ArtifactTypeWASM    ArtifactType = "wasm"
    ArtifactTypeSchema  ArtifactType = "schema"
)
```

### InventoryMetadata

Metadata for inventory organization and discovery.

```go
type InventoryMetadata struct {
    Title       string            `json:"title" validate:"max=200"`
    Summary     string            `json:"summary" validate:"max=500"`
    Category    string            `json:"category" validate:"max=50"`
    Provider    string            `json:"provider" validate:"max=100"`
    Contact     ContactInfo       `json:"contact"`
    License     string            `json:"license" validate:"max=50"`
    HomePage    string            `json:"home_page" validate:"omitempty,url"`
    Repository  string            `json:"repository" validate:"omitempty,url"`
    Keywords    []string          `json:"keywords" validate:"dive,max=30"`
}

type ContactInfo struct {
    Name  string `json:"name" validate:"max=100"`
    Email string `json:"email" validate:"omitempty,email"`
    URL   string `json:"url" validate:"omitempty,url"`
}
```

## Validation Rules

```go
func (i *Inventory) Validate() error {
    if err := validator.Validate(i); err != nil {
        return fmt.Errorf("inventory validation failed: %w", err)
    }
    
    if len(i.Versions) == 0 && i.Status == InventoryStatusActive {
        return errors.New("active inventory must have at least one version")
    }
    
    if i.Type == InventoryTypeHybrid && len(i.Versions) > 0 {
        hasSpec := false
        hasModule := false
        for _, v := range i.Versions {
            for _, a := range v.Artifacts {
                if a.Type == ArtifactTypeOpenAPI {
                    hasSpec = true
                }
                if a.Type == ArtifactTypeWASM {
                    hasModule = true
                }
            }
        }
        if !hasSpec || !hasModule {
            return errors.New("hybrid inventory must contain both spec and module artifacts")
        }
    }
    
    return nil
}

func (v *Version) Validate() error {
    if err := validator.Validate(v); err != nil {
        return fmt.Errorf("version validation failed: %w", err)
    }
    
    if v.Status == VersionStatusPublished && v.PublishedAt == nil {
        return errors.New("published version must have published_at timestamp")
    }
    
    return nil
}
```

## Domain Invariants

1. **Unique Naming**: Inventory names must be unique within organization scope
2. **Version Immutability**: Once published, version artifacts cannot be modified
3. **Semantic Versioning**: All versions must follow SemVer 2.0.0 specification
4. **Artifact Integrity**: All artifacts must have valid SHA-256 checksums
5. **Source Validity**: Active inventories must have reachable sources
6. **Status Transitions**: Draft → Active → Archived (error can occur from any state)
