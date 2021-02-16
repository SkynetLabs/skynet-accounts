package metafetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
	UploaderID primitive.ObjectID
	SkylinkID  primitive.ObjectID
	Attempts   uint8
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
// update the skylink's record in the database. If it fails to download it will
// put the message back in the queue and retry it later a maximum of maxAttempts
// times.
func (mf *MetaFetcher) processMessage(ctx context.Context, m Message) {
	sl, err := mf.db.SkylinkByID(ctx, m.SkylinkID)
	if err != nil {
		logrus.Tracef("Failed to fetch skylink from DB. Skylink ID: %v, error: %v", m.SkylinkID, err)
		if m.Attempts >= maxAttempts {
			mf.logger.Debugf("Message exceeded its maximum number of attempts, dropping: %v", m)
			return
		}
		m.Attempts++
		mf.Queue <- m
		return
	}
	// Check if we have already fetched the size of this skylink and skip the
	// HTTP call if we have.
	if sl.Size != 0 {
		// Update the uploading user, if needed.
		if !m.UploaderID.IsZero() {
			err = mf.db.UserUpdateUsedStorage(ctx, m.UploaderID, sl.Size)
			if err != nil {
				mf.logger.Debugf("Failed to update user's used storage: %s", err)
				// This return might be redundant but it's better to have it than to
				// forget to add it when we add more code below.
				return
			}
			mf.logger.Tracef("Successfully incremented the used storage of user %v with the size of skyfile %v.", m.UploaderID, m.SkylinkID)
		}
		return
	}
	// Make a HEAD request directly to the local `sia` container. We do that, so
	// we don't get rate-limited by nginx in case we need to make many requests.
	skylinkURL, err := url.Parse(fmt.Sprintf("http://sia:9980/skynet/skylink/%s", sl.Skylink))
	if err != nil {
		mf.logger.Debugf("Error while forming skylink URL for skylink %s. Error: %v", sl.Skylink, err)
		return
	}
	req := http.Request{
		Method: http.MethodHead,
		URL:    skylinkURL,
		Header: http.Header{"User-Agent": []string{"Sia-Agent"}},
	}
	client := http.Client{}
	res, err := client.Do(&req)
	if err != nil || res.StatusCode > 399 {
		var statusCode int
		if res != nil {
			statusCode = res.StatusCode
		}
		mf.logger.Tracef("Failed to fetch skyfile. Skylink: %s, status: %v, error: %v", sl.Skylink, statusCode, err)
		if m.Attempts >= maxAttempts {
			mf.logger.Debugf("Message exceeded its maximum number of attempts, dropping: %v", m)
			return
		}
		m.Attempts++
		mf.Queue <- m
		return
	}
	mhs, ok := res.Header["Skynet-File-Metadata"]
	if !ok {
		mf.logger.Debugf("Skyfile doesn't have metadata: %s. Headers: %v", sl.Skylink, res.Header)
		return
	}
	var meta struct {
		Filename string `json:"filename"`
		Length   int64  `json:"length"`
	}
	err = json.Unmarshal([]byte(mhs[0]), &meta)
	if err != nil {
		mf.logger.Debugf("Failed to parse skyfile metadata: %s", err)
		return
	}
	mf.logger.Tracef("Successfully fetched metdata for skylink %v %s: %v", sl.ID, sl.Skylink, meta)
	err = mf.db.SkylinkUpdate(ctx, m.SkylinkID, meta.Filename, meta.Length)
	if err != nil {
		mf.logger.Debugf("Failed to update skyfile metadata: %s", err)
		// We don't return here because we want to perform the next operations
		// regardless of the success of the current one.
	}
	err = mf.db.SkylinkDownloadsUpdate(ctx, m.SkylinkID, meta.Length)
	if err != nil {
		mf.logger.Debugf("Failed to update skyfile downloads: %s", err)
		// We don't return here because we want to perform the next operations
		// regardless of the success of the current one.
	}
	if !m.UploaderID.IsZero() {
		err = mf.db.UserUpdateUsedStorage(ctx, m.UploaderID, meta.Length)
		if err != nil {
			mf.logger.Debugf("Failed to update user's used storage: %s", err)
			// This return might be redundant but it's better to have it than to
			// forget to add it when we add more code below.
			return
		}
	}
	mf.logger.Tracef("Successfully updated skylink %v.", m.SkylinkID)
}
