package dependencies

import "github.com/SkynetLabs/skynet-accounts/lib"

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

// NewDependencyUserPutMongoDelay returns a new DependencyUserPutMongoDelay
// which causes the `PUT /user` endpoint to add a delay before writing to
// Mongo.
func NewDependencyUserPutMongoDelay() lib.Dependencies {
	return &DependencyUserPutMongoDelay{}
}
