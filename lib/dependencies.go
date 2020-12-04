package lib

// Dependencies is a mechanism for simulating different kinds of errors.
type Dependencies interface {
	// Disrupt returns true when the disruption with name disruptionName should
	// result in an error in the current context.
	Disrupt(disruptionName string) bool
}

// ProductionDependencies is the Dependency for production runs which must never
// result in an error.
type ProductionDependencies struct{}

// Disrupt does nothing in production.
func (pd *ProductionDependencies) Disrupt(_ string) bool {
	return false
}
