package skynet

const (
	// KiB kilobyte
	KiB = 1024
	// MiB megabyte
	MiB = 1024 * KiB

	// SizeBaseSector is the size of a base sector.
	SizeBaseSector = 4 * MiB
	// SizeChunk is the size of a chunk.
	SizeChunk = 40 * MiB

	// PriceBandwidthRegistryWrite the bandwidth cost of a single registry write
	PriceBandwidthRegistryWrite = 5 * MiB
	// PriceBandwidthRegistryRead the bandwidth cost of a single registry read
	PriceBandwidthRegistryRead = MiB

	// PriceBandwidthUploadBase is the baseline bandwidth price for each upload.
	// This is the cost of uploading the base sector.
	PriceBandwidthUploadBase = 40 * MiB
	// PriceBandwidthUploadIncrement is the bandwidth price per 40MB uploaded
	// data, beyond the base sector (beyond the first 4MB). Rounded up.
	PriceBandwidthUploadIncrement = 120 * MiB
	// PriceBandwidthDownloadBase is the baseline bandwidth price for each Download.
	PriceBandwidthDownloadBase = 200 * KiB
	// PriceBandwidthDownloadIncrement is the bandwidth price per 64B. Rounded up.
	PriceBandwidthDownloadIncrement = 64

	// PriceStorageUploadBase is the baseline storage price for each upload.
	// This is the cost of uploading the base sector.
	PriceStorageUploadBase = 4 * MiB
	// PriceStorageUploadIncrement is the storage price for each 40MB beyond
	// the base sector (beyond the first 4MB). Rounded up.
	PriceStorageUploadIncrement = 40 * MiB
)

// BandwidthUploadCost calculates the bandwidth cost of an upload with the given
// size. The base sector is uploaded with 10x redundancy. Each chunk is uploaded
// with 3x redundancy.
func BandwidthUploadCost(size int64) int64 {
	return PriceBandwidthUploadBase + numChunks(size)*PriceBandwidthUploadIncrement
}

// BandwidthDownloadCost calculates the bandwidth cost of a download with the
// given size.
func BandwidthDownloadCost(size int64) int64 {
	chunks := size / 64
	if size%64 > 0 {
		chunks++
	}
	return PriceBandwidthDownloadBase + chunks*PriceBandwidthDownloadIncrement
}

// StorageUsed calculates how much storage an upload with a given size actually
// uses.
func StorageUsed(uploadSize int64) int64 {
	return PriceStorageUploadBase + numChunks(uploadSize)*PriceStorageUploadIncrement
}

// numChunks returns the number of 40MB chunks a file of this size uses, beyond
// the 4MB in the base sector.
func numChunks(size int64) int64 {
	if size <= SizeBaseSector {
		return 0
	}
	chunksBeyondBase := (size - SizeBaseSector) / SizeChunk
	if (size-SizeBaseSector)%SizeChunk > 0 {
		chunksBeyondBase++
	}
	return chunksBeyondBase
}
