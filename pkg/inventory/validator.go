package inventory

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Validate(inv *Inventory) error {
	if inv.SchemaVersion != "1.0" {
		return fmt.Errorf("unsupported schema version: %s", inv.SchemaVersion)
	}

	if inv.MinCLIVersion != "" {
		minVer, err := semver.NewVersion(inv.MinCLIVersion)
		if err != nil {
			return fmt.Errorf("invalid min_cli_version: %w", err)
		}

		currentVer := semver.MustParse("0.1.0")
		if currentVer.LessThan(minVer) {
			return fmt.Errorf("CLI version 0.1.0 is below minimum required %s",
				inv.MinCLIVersion)
		}
	}

	if inv.MaxCLIVersion != "" {
		maxVer, err := semver.NewVersion(inv.MaxCLIVersion)
		if err != nil {
			return fmt.Errorf("invalid max_cli_version: %w", err)
		}

		currentVer := semver.MustParse("0.1.0")
		if currentVer.GreaterThan(maxVer) {
			return fmt.Errorf("CLI version 0.1.0 exceeds maximum supported %s",
				inv.MaxCLIVersion)
		}
	}

	resourceNames := make(map[string]bool)
	for _, res := range inv.Resources {
		if resourceNames[res.Name] {
			return fmt.Errorf("duplicate resource name: %s", res.Name)
		}
		resourceNames[res.Name] = true
	}

	for _, res := range inv.Resources {
		opNames := make(map[string]bool)
		for _, op := range res.Operations {
			if opNames[op.Name] {
				return fmt.Errorf("duplicate operation name '%s' in resource '%s'", op.Name, res.Name)
			}
			opNames[op.Name] = true
		}
	}

	for _, mw := range inv.Middleware {
		if mw.Stage != "request" && mw.Stage != "response" && mw.Stage != "error" {
			return fmt.Errorf("invalid middleware stage '%s' for module '%s'", mw.Stage, mw.Name)
		}
	}

	return nil
}
