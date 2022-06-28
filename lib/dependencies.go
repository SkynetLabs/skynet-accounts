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

/* Test dependencies */

type (
	// DependencyUserPutMongoDelay causes the `PUT /user` endpoint to add a delay before
	// writing to Mongo.
	DependencyUserPutMongoDelay struct{}
)

// Disrupt causes the `PUT /user` endpoint to add a delay before writing to
// Mongo.
func (d *DependencyUserPutMongoDelay) Disrupt(s string) bool {
	return s == "DependencyUserPutMongoDelay"
}

// NewDependencyUserPutMongoDelay returns a new DependencyUserPutMongoDelay.
func NewDependencyUserPutMongoDelay() Dependencies {
	return &DependencyUserPutMongoDelay{}
}
