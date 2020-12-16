package metafetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NebulousLabs/skynet-accounts/database"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// maxAttempts defines the maximum number of attempts we are going to make to
// process a message.
const maxAttempts = 3

// Message is the format we use to tell the MetaFetcher to download
// the metadata for a given skylink and then add its size to the used space of
// a given user.
type Message struct {
	UserID    primitive.ObjectID
	SkylinkID primitive.ObjectID
	Attempts  uint8
}

// MetaFetcher is a background task that listens for messages on its queue and
// then processes them.
type MetaFetcher struct {
	Queue chan Message

	db     *database.DB
	portal string
	logger *logrus.Logger
}

// New returns a new MetaFetcher instance and starts its internal queue watcher.
func New(ctx context.Context, db *database.DB, portal string, logger *logrus.Logger) *MetaFetcher {
	if logger == nil {
		logger = logrus.New()
	}
	mf := MetaFetcher{
		Queue:  make(chan Message, 1000),
		db:     db,
		portal: portal,
		logger: logger,
	}

	go mf.threadedStartQueueWatcher(ctx)

	return &mf
}

// threadedStartQueueWatcher starts a loop over the Queue that processes each
// incoming message in a separate goroutine.
func (mf *MetaFetcher) threadedStartQueueWatcher(ctx context.Context) {
	for m := range mf.Queue {
		// Process each message in a separate goroutine because fetching the
		// meta might take a long time (30 seconds) and we don't want to block
		// the queue.
		go mf.processMessage(ctx, m)
	}
}

// processMessage tries to download the metadata for the given skylink and
// update the skylink's record in the database. It will also update the user's
// used storage. If it fails to download it will put the message back in the
// queue and retry it later a maximum of maxAttempts times.
func (mf *MetaFetcher) processMessage(ctx context.Context, m Message) {
	sl, err := mf.db.SkylinkByID(ctx, m.SkylinkID)
	if err != nil {
		logrus.Tracef("Failed to fetch skylink from DB. Skylink ID: %v, error: %v\n", m.SkylinkID, err)
		if m.Attempts >= maxAttempts {
			mf.logger.Debugf("Message exceeded its maximum number of attempts, dropping: %v\n", m)
			return
		}
		m.Attempts++
		mf.Queue <- m
		return
	}
	r, err := http.Head(fmt.Sprintf("%s/%s", mf.portal, sl.Skylink))
	if err != nil || r.StatusCode > 399 {
		mf.logger.Tracef("Failed to fetch skyfile. Skylink: %s, error: %v\n", sl.Skylink, err)
		if m.Attempts >= maxAttempts {
			mf.logger.Debugf("Message exceeded its maximum number of attempts, dropping: %v\n", m)
			return
		}
		m.Attempts++
		mf.Queue <- m
		return
	}
	mhs, ok := r.Header["Skynet-File-Metadata"]
	if !ok {
		mf.logger.Debugf("Skyfile doesn't have metadata: %s. Headers: %v\n", sl.Skylink, r.Header)
		return
	}
	var meta struct {
		Filename string `json:"filename"`
		Length   int64  `json:"length"`
	}
	err = json.Unmarshal([]byte(mhs[0]), &meta)
	if err != nil {
		mf.logger.Debugf("Failed to parse skyfile metadata: %s\n", err)
		return
	}
	mf.logger.Tracef("Successfully fetched metdata for skylink %v %s: %v\n", sl.ID, sl.Skylink, meta)
	err = mf.db.SkylinkUpdate(ctx, m.SkylinkID, meta.Filename, meta.Length)
	if err != nil {
		mf.logger.Debugf("Failed to update skyfile metadata: %s\n", err)
		// We don't return here because we want to perform the next operation
		// regardless of the success of the current one.
	}
	err = mf.db.UserUpdateUsedStorage(ctx, m.UserID, meta.Length)
	if err != nil {
		mf.logger.Debugf("Failed to update user's used storage: %s\n", err)
		// This return might be redundant but it's better to have it than to
		// forget to add it when we add more code below.
		return
	}
	mf.logger.Tracef("Successfully updated skylink %v and user %v.\n", m.SkylinkID, m.UserID)
}
