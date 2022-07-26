package dependencies

import (
	"sync"

	"github.com/SkynetLabs/skynet-accounts/lib"
)

var (
	// DependencyMongoWriteConflictNMessage is the error message with which the
	// DependencyMongoWriteConflictN will cause a function to fail.
	DependencyMongoWriteConflictNMessage = "(WriteConflict) This is a WriteConflict caused by DependencyMongoWriteConflictN."
)

type (
	// DependencyMongoWriteConflictN causes the caller to fail with a
	// WriteConflict N times in a row.
	DependencyMongoWriteConflictN struct {
		remainingFailures uint
		mu                sync.Mutex
	}
	// DependencyUserPutMongoDelay causes the `PUT /user` endpoint to add a delay before
	// writing to Mongo.
	DependencyUserPutMongoDelay struct{}
)

// Disrupt causes the `PUT /user` endpoint to add a delay before writing to
// Mongo.
func (d *DependencyMongoWriteConflictN) Disrupt(s string) bool {
	if s == "DependencyMongoWriteConflictN" && d.remainingFailures > 0 {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.remainingFailures--
		return true
	}
	return false
}

// NewDependencyMongoWriteConflictN returns a new DependencyMongoWriteConflictN
// which causes the caller to fail with a WriteConflict N times in a row.
func NewDependencyMongoWriteConflictN(n uint) lib.Dependencies {
	return &DependencyMongoWriteConflictN{remainingFailures: n}
}

// Disrupt causes the `PUT /user` endpoint to add a delay before writing to
// Mongo.
func (d *DependencyUserPutMongoDelay) Disrupt(s string) bool {
	return s == "DependencyUserPutMongoDelay"
}

// NewDependencyUserPutMongoDelay returns a new DependencyUserPutMongoDelay
// which causes the `PUT /user` endpoint to add a delay before writing to
// Mongo.
func NewDependencyUserPutMongoDelay() lib.Dependencies {
	return &DependencyMongoWriteConflictN{}
}
