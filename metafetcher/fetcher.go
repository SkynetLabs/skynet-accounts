package metafetcher

import (
	"context"
	"fmt"
	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/http"
)

/*
TODO
 - decide what data to keep about skylinks
 - how to store info about the user's uploads?
 - background worker pool?
*/

// maxAttempts defines the maximum number of attempts we are going to make to
// process a message.
const maxAttempts = 3

type MetaFetcherMessage struct {
	UserID    primitive.ObjectID
	SkylinkID primitive.ObjectID
	Attempts  uint8
}

type MetaFetcher struct {
	Queue chan MetaFetcherMessage
	//workers []*worker...

	db     *database.DB
	portal string

	//tg *threadgroup.ThreadGroup
}

// start the pool at service start (in main)
// make it available to the api
// send a struct on its receiving channel with {skylink_id, user_id, attempts=0}
// on fetching the metadata failure, resend with attempts+1 until you reach 3
// on success update the size of the skylink in the collection and increment
// the user's used storage OR update the user's used storage based on the
// current state of the uploads collection

func New(ctx context.Context, db *database.DB, portal string) *MetaFetcher {
	mf := MetaFetcher{
		Queue:  make(chan MetaFetcherMessage),
		db:     db,
		portal: portal,
	}

	go mf.threadedStartQueueWatcher(ctx)

	return &mf
}

// threadedStartQueueWatcher starts a loop over the Queue that processes each
// incoming message in a separate goroutine.
func (mf *MetaFetcher) threadedStartQueueWatcher(ctx context.Context) {
	for m := range mf.Queue {
		go mf.processMessage(ctx, m)
	}
}

func (mf *MetaFetcher) processMessage(ctx context.Context, m MetaFetcherMessage) {
	sl, err := mf.db.SkylinkByID(ctx, m.SkylinkID)
	if err != nil {
		logrus.Tracef("Failed to fetch skylink from DB. Skylink ID: %v, error: %v\n", m.SkylinkID, err)
		if m.Attempts >= maxAttempts {
			logrus.Debugf("Message exceeded its maximum number of attempts, dropping: %v\n", m)
			return
		}
		m.Attempts++
		mf.Queue <- m
		return
	}
	r, err := http.Head(fmt.Sprintf("%s/%s", mf.portal, sl))
	if err!=nil {
		logrus.Tracef("Failed to fetch skyfile. Skylink: %s, error: %v\n", sl, err)
		if m.Attempts >= maxAttempts {
			logrus.Debugf("Message exceeded its maximum number of attempts, dropping: %v\n", m)
			return
		}
		m.Attempts++
		mf.Queue <- m
		return
	}
	fmt.Println(r)
}
