package database

import (
	"context"
	"sync"
	"time"

	"github.com/SkynetLabs/skynet-accounts/skynet"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type (
	// UserStats contains statistical information about the user.
	// "Total" is a prefix in JSON form because of backwards compatibility.
	UserStats struct {
		NumRegReads       int64 `json:"numRegReads"`
		NumRegReadsTotal  int64 `json:"numRegReadsTotal"`
		NumRegWrites      int64 `json:"numRegWrites"`
		NumRegWritesTotal int64 `json:"numRegWritesTotal"`
		NumUploads        int64 `json:"numUploads"`
		NumUploadsTotal   int64 `json:"numUploadsTotal"`
		NumDownloads      int64 `json:"numDownloads"`
		NumDownloadsTotal int64 `json:"numDownloadsTotal"`

		BandwidthUploads        int64 `json:"bwUploads"`
		BandwidthUploadsTotal   int64 `json:"bwUploadsTotal"`
		BandwidthDownloads      int64 `json:"bwDownloads"`
		BandwidthDownloadsTotal int64 `json:"bwDownloadsTotal"`
		BandwidthRegReads       int64 `json:"bwRegReads"`
		BandwidthRegReadsTotal  int64 `json:"bwRegReadsTotal"`
		BandwidthRegWrites      int64 `json:"bwRegWrites"`
		BandwidthRegWritesTotal int64 `json:"bwRegWritesTotal"`

		RawStorageUsed      int64 `json:"rawStorageUsed"`
		RawStorageUsedTotal int64 `json:"rawStorageUsedTotal"`
		UploadsSize         int64 `json:"uploadsSize"`
		UploadsSizeTotal    int64 `json:"uploadsSizeTotal"`
		DownloadsSize       int64 `json:"downloadsSize"`
		DownloadsSizeTotal  int64 `json:"downloadsSizeTotal"`

		// These are here for backwards compatibility.
		TotalUploadsSize   int64 `json:"totalUploadsSize"`
		TotalDownloadsSize int64 `json:"totalDownloadsSize"`
	}
	// UserStatsUpload reports the upload stats of a given user. It holds
	// the stats for the current period, as well as the total stats.
	UserStatsUpload struct {
		Count               int64
		CountTotal          int64
		Size                int64
		SizeTotal           int64
		RawStorageUsed      int64
		RawStorageUsedTotal int64
		Bandwidth           int64
		BandwidthTotal      int64
	}
	// UserStatsDownload reports the download stats of a given user. It holds
	// the stats for the current period, as well as the total stats.
	UserStatsDownload struct {
		Count          int64
		CountTotal     int64
		Size           int64
		SizeTotal      int64
		Bandwidth      int64
		BandwidthTotal int64
	}
	// UserStatsRegReads reports the number of registry reads for a given user.
	// It holds the stats for the current period, as well as the total stats.
	UserStatsRegReads struct {
		Count          int64
		CountTotal     int64
		Bandwidth      int64
		BandwidthTotal int64
	}
	// UserStatsRegWrites reports the number of registry writes for a given user.
	// It holds the stats for the current period, as well as the total stats.
	UserStatsRegWrites struct {
		Count          int64
		CountTotal     int64
		Bandwidth      int64
		BandwidthTotal int64
	}
)

// UserStats returns statistical information about the user.
func (db *DB) UserStats(ctx context.Context, user User) (*UserStats, error) {
	return db.userStats(ctx, user)
}

// userStats reports statistical information about the user.
func (db *DB) userStats(ctx context.Context, user User) (*UserStats, error) {
	stats := UserStats{}
	var errs []error
	var errsMux sync.Mutex
	regErr := func(msg string, e error) {
		db.staticLogger.Infoln(msg, e)
		errsMux.Lock()
		errs = append(errs, e)
		errsMux.Unlock()
	}
	startOfMonth := monthStart(user.SubscribedUntil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		upStats, err := db.UserStatsUpload(ctx, user.ID, startOfMonth)
		if err != nil {
			regErr("Failed to get user's upload stats:", err)
			return
		}
		stats.NumUploads = upStats.Count
		stats.NumUploadsTotal = upStats.CountTotal
		stats.UploadsSize = upStats.Size
		stats.UploadsSizeTotal = upStats.SizeTotal
		stats.TotalUploadsSize = upStats.SizeTotal
		stats.BandwidthUploads = upStats.Bandwidth
		stats.BandwidthUploadsTotal = upStats.BandwidthTotal
		stats.RawStorageUsed = upStats.RawStorageUsed
		stats.RawStorageUsedTotal = upStats.RawStorageUsedTotal
		db.staticLogger.Tracef("User %s upload stats: %v", user.ID.Hex(), upStats)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		downStats, err := db.userDownloadStats(ctx, user.ID, startOfMonth)
		if err != nil {
			regErr("Failed to get user's download stats:", err)
			return
		}
		stats.NumDownloads = downStats.Count
		stats.NumDownloadsTotal = downStats.CountTotal
		stats.DownloadsSize = downStats.Size
		stats.DownloadsSizeTotal = downStats.SizeTotal
		stats.TotalDownloadsSize = downStats.SizeTotal
		stats.BandwidthDownloads = downStats.Bandwidth
		stats.BandwidthDownloadsTotal = downStats.BandwidthTotal
		db.staticLogger.Tracef("User %s download stats: %v", user.ID.Hex(), downStats)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		rwStats, err := db.userRegistryWriteStats(ctx, user.ID, startOfMonth)
		if err != nil {
			regErr("Failed to get user's registry write bandwidth used:", err)
			return
		}
		stats.NumRegWrites = rwStats.Count
		stats.NumRegWritesTotal = rwStats.CountTotal
		stats.BandwidthRegWrites = rwStats.Bandwidth
		stats.BandwidthRegWrites = rwStats.BandwidthTotal
		db.staticLogger.Tracef("User %s registry write stats: %v", user.ID.Hex(), rwStats)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		rrStats, err := db.userRegistryReadStats(ctx, user.ID, startOfMonth)
		if err != nil {
			regErr("Failed to get user's registry read bandwidth used:", err)
			return
		}
		stats.NumRegReads = rrStats.Count
		stats.NumRegReadsTotal = rrStats.CountTotal
		stats.BandwidthRegReads = rrStats.Bandwidth
		stats.BandwidthRegReadsTotal = rrStats.BandwidthTotal
		db.staticLogger.Tracef("User %s registry read stats: %v", user.ID.Hex(), rrStats)
	}()

	wg.Wait()
	if len(errs) > 0 {
		return nil, errors.Compose(errs...)
	}
	return &stats, nil
}

// UserStatsUpload reports on the user's uploads - count, total size and total
// bandwidth used. It uses the total size of the uploaded skyfiles as basis.
func (db *DB) UserStatsUpload(ctx context.Context, id primitive.ObjectID, since time.Time) (stats UserStatsUpload, err error) {
	matchStage := bson.D{{"$match", bson.M{"user_id": id}}}
	lookupStage := bson.D{
		{"$lookup", bson.D{
			{"from", "skylinks"},
			{"localField", "skylink_id"},
			{"foreignField", "_id"},
			{"as", "skylink_data"},
		}},
	}
	replaceStage := bson.D{
		{"$replaceRoot", bson.D{
			{"newRoot", bson.D{
				{"$mergeObjects", bson.A{
					bson.D{{"$arrayElemAt", bson.A{"$skylink_data", 0}}}, "$$ROOT"},
				},
			}},
		}},
	}
	// These are the fields we don't need.
	projectStage := bson.D{{"$project", bson.D{
		{"_id", 0},
		{"user_id", 0},
		{"skylink_data", 0},
		{"name", 0},
		{"skylink_id", 0},
	}}}

	pipeline := mongo.Pipeline{matchStage, lookupStage, replaceStage, projectStage}
	c, err := db.staticUploads.Aggregate(ctx, pipeline)
	if err != nil {
		return
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			db.staticLogger.Traceln("Error on closing DB cursor.", errDef)
		}
	}()

	// We need this struct, so we can safely decode both int32 and int64.
	result := struct {
		Size      int64     `bson:"size"`
		Skylink   string    `bson:"skylink"`
		Unpinned  bool      `bson:"unpinned"`
		Timestamp time.Time `bson:"timestamp"`
	}{}
	processedSkylinks := make(map[string]bool)
	for c.Next(ctx) {
		if err = c.Decode(&result); err != nil {
			err = errors.AddContext(err, "failed to decode DB data")
			return
		}
		// All bandwidth is counted, regardless of unpinned status and
		// uniqueness.
		stats.BandwidthTotal += skynet.BandwidthUploadCost(result.Size)
		if result.Timestamp.After(since) {
			stats.Bandwidth += skynet.BandwidthUploadCost(result.Size)
		}
		// Only count unique  uploads that are still pinned towards total count,
		// size and storage used.
		if result.Unpinned {
			continue
		}
		stats.CountTotal++
		if !processedSkylinks[result.Skylink] {
			stats.SizeTotal += result.Size
			stats.RawStorageUsedTotal += skynet.RawStorageUsed(result.Size)
		}
		// Check against the time threshold before continuing with the period
		// counts.
		if result.Timestamp.After(since) {
			stats.Count++
			if !processedSkylinks[result.Skylink] {
				stats.Size += result.Size
				stats.RawStorageUsed += skynet.RawStorageUsed(result.Size)
			}
		}
		processedSkylinks[result.Skylink] = true
	}
	return stats, nil
}

// userDownloadStats reports on the user's downloads - count, total size and
// total bandwidth used. It uses the actual bandwidth used, as reported by nginx.
func (db *DB) userDownloadStats(ctx context.Context, id primitive.ObjectID, since time.Time) (stats UserStatsDownload, err error) {
	matchStage := bson.D{{"$match", bson.D{
		{"user_id", id},
	}}}
	lookupStage := bson.D{
		{"$lookup", bson.D{
			{"from", "skylinks"},
			{"localField", "skylink_id"}, // field in the downloads collection
			{"foreignField", "_id"},      // field in the skylinks collection
			{"as", "fromSkylinks"},
		}},
	}
	replaceStage := bson.D{
		{"$replaceRoot", bson.D{
			{"newRoot", bson.D{
				{"$mergeObjects", bson.A{
					bson.D{{"$arrayElemAt", bson.A{"$fromSkylinks", 0}}}, "$$ROOT"},
				},
			}},
		}},
	}
	// This stage checks if the download has a non-zero `bytes` field and if so,
	// it takes it as the download's size. Otherwise, it reports the full
	// skylink's size as download's size.
	projectStage := bson.D{{"$project", bson.D{
		{"size", bson.D{
			{"$cond", bson.A{
				bson.D{{"$gt", bson.A{"$bytes", 0}}}, // if
				"$bytes",                             // then
				"$size",                              // else
			}},
		}},
		{"created_at", "$created_at"},
	}}}

	pipeline := mongo.Pipeline{matchStage, lookupStage, replaceStage, projectStage}
	c, err := db.staticDownloads.Aggregate(ctx, pipeline)
	if err != nil {
		err = errors.AddContext(err, "DB query failed")
		return
	}
	defer func() {
		if errDef := c.Close(ctx); errDef != nil {
			db.staticLogger.Traceln("Error on closing DB cursor.", errDef)
		}
	}()

	// We need this struct, so we can safely decode both int32 and int64.
	result := struct {
		Size      int64     `bson:"size"`
		CreatedAt time.Time `bson:"created_at"`
	}{}
	for c.Next(ctx) {
		if err = c.Decode(&result); err != nil {
			err = errors.AddContext(err, "failed to decode DB data")
			return
		}
		stats.CountTotal++
		stats.SizeTotal += result.Size
		stats.BandwidthTotal += skynet.BandwidthDownloadCost(result.Size)
		if result.CreatedAt.After(since) {
			stats.Count++
			stats.Size += result.Size
			stats.Bandwidth += skynet.BandwidthDownloadCost(result.Size)
		}
	}
	return stats, nil
}

// userRegistryWriteStats reports the number of registry writes by the user and
// the bandwidth used.
func (db *DB) userRegistryWriteStats(ctx context.Context, userID primitive.ObjectID, since time.Time) (stats UserStatsRegWrites, err error) {
	matchStage := bson.D{{"$match", bson.D{
		{"user_id", userID},
		{"timestamp", bson.D{{"$gt", since}}},
	}}}
	writes, err := db.count(ctx, db.staticRegistryWrites, matchStage)
	if err != nil {
		return stats, errors.AddContext(err, "failed to fetch registry write bandwidth")
	}
	matchStage = bson.D{{"$match", bson.D{
		{"user_id", userID},
	}}}
	writesTotal, err := db.count(ctx, db.staticRegistryWrites, matchStage)
	if err != nil {
		return stats, errors.AddContext(err, "failed to fetch registry write bandwidth")
	}
	stats.Count = writes
	stats.CountTotal = writesTotal
	stats.Bandwidth = writes * skynet.CostBandwidthRegistryWrite
	stats.BandwidthTotal = writesTotal * skynet.CostBandwidthRegistryWrite
	return stats, nil
}

// userRegistryReadsStats reports the number of registry reads by the user and
// the bandwidth used.
func (db *DB) userRegistryReadStats(ctx context.Context, userID primitive.ObjectID, monthStart time.Time) (stats UserStatsRegReads, err error) {
	matchStage := bson.D{{"$match", bson.D{
		{"user_id", userID},
		{"timestamp", bson.D{{"$gt", monthStart}}},
	}}}
	reads, err := db.count(ctx, db.staticRegistryReads, matchStage)
	if err != nil {
		return stats, errors.AddContext(err, "failed to fetch registry read bandwidth")
	}
	matchStage = bson.D{{"$match", bson.D{
		{"user_id", userID},
	}}}
	readsTotal, err := db.count(ctx, db.staticRegistryReads, matchStage)
	if err != nil {
		return stats, errors.AddContext(err, "failed to fetch registry read bandwidth")
	}
	stats.Count = reads
	stats.CountTotal = readsTotal
	stats.Bandwidth = reads * skynet.CostBandwidthRegistryRead
	stats.BandwidthTotal = readsTotal * skynet.CostBandwidthRegistryRead
	return stats, nil
}
