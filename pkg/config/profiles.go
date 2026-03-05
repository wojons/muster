package config

import (
	"fmt"
)

type ProfileManager struct {
	cm *ConfigManager
}

func NewProfileManager(cm *ConfigManager) *ProfileManager {
	return &ProfileManager{cm: cm}
}

func (pm *ProfileManager) Create(name string, profile Profile) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	if err := pm.cm.AddProfile(name, profile); err != nil {
		return err
	}

	if pm.cm.GetDefaultProfile() == "" {
		return pm.cm.SetDefaultProfile(name)
	}

	return nil
}

func (pm *ProfileManager) Get(name string) (*Profile, error) {
	return pm.cm.GetProfile(name)
}

func (pm *ProfileManager) Update(name string, profile Profile) error {
	return pm.cm.UpdateProfile(name, profile)
}

func (pm *ProfileManager) Delete(name string) error {
	if pm.cm.GetDefaultProfile() == name {
		return fmt.Errorf("cannot delete default profile '%s'", name)
	}
	return pm.cm.DeleteProfile(name)
}

func (pm *ProfileManager) List() []*Profile {
	profiles := pm.cm.ListProfiles()
	result := make([]*Profile, len(profiles))
	for i := range profiles {
		result[i] = &profiles[i]
	}
	return result
}

func (pm *ProfileManager) Exists(name string) bool {
	_, err := pm.cm.GetProfile(name)
	return err == nil
}

func (pm *ProfileManager) SetDefault(name string) error {
	return pm.cm.SetDefaultProfile(name)
}

func (pm *ProfileManager) GetDefault() (*Profile, error) {
	name := pm.cm.GetDefaultProfile()
	if name == "" {
		return nil, fmt.Errorf("no default profile set")
	}
	return pm.Get(name)
}

func (pm *ProfileManager) Current() (*Profile, error) {
	return pm.GetDefault()
}

func (p *Profile) MergeWith(other *Profile) *Profile {
	merged := &Profile{
		BaseURL:      p.BaseURL,
		AuthType:     p.AuthType,
		AuthConfig:   make(map[string]string),
		DefaultFlags: make(map[string]string),
		Headers:      make(map[string]string),
	}

	if other.DefaultFlags != nil {
		for k, v := range other.DefaultFlags {
			merged.DefaultFlags[k] = v
		}
	}

	if p.DefaultFlags != nil {
		for k, v := range p.DefaultFlags {
			merged.DefaultFlags[k] = v
		}
	}

	if other.Headers != nil {
		for k, v := range other.Headers {
			merged.Headers[k] = v
		}
	}

	if p.Headers != nil {
		for k, v := range p.Headers {
			merged.Headers[k] = v
		}
	}

	if other.AuthConfig != nil {
		for k, v := range other.AuthConfig {
			merged.AuthConfig[k] = v
		}
	}

	if p.AuthConfig != nil {
		for k, v := range p.AuthConfig {
			merged.AuthConfig[k] = v
		}
	}

	if other.BaseURL != "" {
		merged.BaseURL = other.BaseURL
	}

	if other.AuthType != "" {
		merged.AuthType = other.AuthType
	}

	return merged
}
