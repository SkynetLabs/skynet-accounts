package test

import (
	"github.com/NebulousLabs/skynet-accounts/lib"
)

type (
	// DependencyHashPassword causes the contractor to use default
	// settings when renewing a contract.
	DependencyHashPassword struct {
		lib.ProductionDependencies
	}
)

// Disrupt causes the contractor to use default host settings
// when renewing a contract.
func (d *DependencyHashPassword) Disrupt(s string) bool {
	return s == "DependencyHashPassword"
}
