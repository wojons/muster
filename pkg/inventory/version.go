package inventory

import (
	"fmt"
	"time"

	"github.com/blang/semver"
)

type VersionManager struct {
	history map[string][]VersionRecord
}

func NewVersionManager() *VersionManager {
	return &VersionManager{
		history: make(map[string][]VersionRecord),
	}
}

func (vm *VersionManager) Track(host string, inv *Inventory) {
	now := time.Now()

	record := VersionRecord{
		Version:    inv.APIVersion,
		FirstSeen:  now,
		LastUsed:   now,
		Compatible: vm.isCompatible(inv),
	}

	history := vm.history[host]
	for i, r := range history {
		if r.Version == inv.APIVersion {
			history[i].LastUsed = now
			history[i].Compatible = record.Compatible
			return
		}
	}

	vm.history[host] = append(history, record)
}

func (vm *VersionManager) GetHistory(host string) ([]VersionRecord, error) {
	history, ok := vm.history[host]
	if !ok {
		return nil, fmt.Errorf("no history found for host: %s", host)
	}
	return history, nil
}

func (vm *VersionManager) IsVersionCompatible(host, version string) bool {
	history, ok := vm.history[host]
	if !ok {
		return false
	}

	for _, r := range history {
		if r.Version == version {
			return r.Compatible
		}
	}

	return false
}

func (vm *VersionManager) isCompatible(inv *Inventory) bool {
	if inv.MinCLIVersion == "" && inv.MaxCLIVersion == "" {
		return true
	}

	currentVer := semver.MustParse("0.1.0")

	if inv.MinCLIVersion != "" {
		minVer, err := semver.Parse(inv.MinCLIVersion)
		if err != nil {
			return false
		}
		if currentVer.LT(minVer) {
			return false
		}
	}

	if inv.MaxCLIVersion != "" {
		maxVer, err := semver.Parse(inv.MaxCLIVersion)
		if err != nil {
			return false
		}
		if currentVer.GT(maxVer) {
			return false
		}
	}

	return true
}
