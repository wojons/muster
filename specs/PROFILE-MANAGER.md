# Profile Manager Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Configuration

## Overview

Complete specification of **profile manager** for CRUD operations on profiles.

## 1. Manager Structure

```go
type ProfileManager struct {
    configManager *ConfigManager
    profiles      map[string]*Profile
}

func NewProfileManager(cm *ConfigManager) *ProfileManager {
    return &ProfileManager{
        configManager: cm,
        profiles:      make(map[string]*Profile),
    }
}
```

## 2. CRUD Operations

```go
func (pm *ProfileManager) Create(name string, profile *Profile) error {
    if _, exists := pm.profiles[name]; exists {
        return fmt.Errorf("profile '%s' already exists", name)
    }
    
    profile.Name = name
    pm.profiles[name] = profile
    
    return pm.save()
}

func (pm *ProfileManager) Get(name string) (*Profile, error) {
    profile, exists := pm.profiles[name]
    if !exists {
        return nil, fmt.Errorf("profile '%s' not found", name)
    }
    
    return profile, nil
}

func (pm *ProfileManager) Update(name string, profile *Profile) error {
    if _, exists := pm.profiles[name]; !exists {
        return fmt.Errorf("profile '%s' not found", name)
    }
    
    profile.Name = name
    pm.profiles[name] = profile
    
    return pm.save()
}

func (pm *ProfileManager) Delete(name string) error {
    if name == "default" {
        return errors.New("cannot delete default profile")
    }
    
    if _, exists := pm.profiles[name]; !exists {
        return fmt.Errorf("profile '%s' not found", name)
    }
    
    delete(pm.profiles, name)
    return pm.save()
}
```

## 3. Listing

```go
func (pm *ProfileManager) List() []*Profile {
    list := make([]*Profile, 0, len(pm.profiles))
    
    for _, profile := range pm.profiles {
        list = append(list, profile)
    }
    
    // Sort by name
    sort.Slice(list, func(i, j int) bool {
        return list[i].Name < list[j].Name
    })
    
    return list
}

func (pm *ProfileManager) Exists(name string) bool {
    _, exists := pm.profiles[name]
    return exists
}
```

## 4. Current Profile

```go
func (pm *ProfileManager) SetCurrent(name string) error {
    if _, exists := pm.profiles[name]; !exists {
        return fmt.Errorf("profile '%s' not found", name)
    }
    
    return pm.configManager.SetDefaultProfile(name)
}

func (pm *ProfileManager) GetCurrent() (*Profile, error) {
    current := pm.configManager.GetDefaultProfile()
    return pm.Get(current)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
