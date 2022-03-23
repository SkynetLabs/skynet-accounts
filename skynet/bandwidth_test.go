package skynet

import (
	"testing"
)

const (
	// baseSectorTotalSize is the total amount of storage used by a base sector,
	// including its redundancy.
	baseSectorTotalSize = SizeBaseSector * RedundancyBaseSector
	// chunkTotalSize is the total amount of storage used by a chunk, including
	// its redundancy.
	chunkTotalSize = SizeChunk * RedundancyChunk
)

// TestNumChunks ensures that numChunks works as expected.
func TestNumChunks(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0 * MB, result: 0},
		{size: 1 * MB, result: 0},
		{size: 4 * MB, result: 0},
		{size: 5 * MB, result: 1},
		{size: 50 * MB, result: 2},
		{size: 500 * MB, result: 13},
	}
	for _, tt := range tests {
		res := numChunks(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MB file to result into %d chunks, got %d.",
				tt.size/MB, tt.result, res)
		}
	}
}

// TestRawStorageUsed ensures that RawStorageUsed works as expected.
func TestRawStorageUsed(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0, result: baseSectorTotalSize},
		{size: 1 * MB, result: baseSectorTotalSize},
		{size: 4 * MB, result: baseSectorTotalSize},
		// 4MB base sector + 1MB overflow which fits in a single 40MB chunk.
		{size: 5 * MB, result: baseSectorTotalSize + chunkTotalSize},
		// 4MB base sector + 46MB overflow which fit in two 40MB chunks.
		{size: 50 * MB, result: baseSectorTotalSize + 2*chunkTotalSize},
		// 4MB base sector + 496MB overflow which fit in math.Ceil(496 / 40.0) = 13 chunks.
		{size: 500 * MB, result: baseSectorTotalSize + 13*chunkTotalSize},
	}
	for _, tt := range tests {
		res := RawStorageUsed(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MB file to result into %d MB used for upload storage, got %d MB.",
				tt.size/MB, tt.result/MB, res/MB)
		}
	}
}

// TestBandwidthUploadCost ensures BandwidthUploadCost works as expected.
func TestBandwidthUploadCost(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0, result: RedundancyBaseSector * SizeBaseSector},
		{size: 1 * MB, result: RedundancyBaseSector * SizeBaseSector},
		{size: 4 * MB, result: RedundancyBaseSector * SizeBaseSector},
		{size: 5 * MB, result: RedundancyBaseSector*SizeBaseSector + RedundancyChunk*SizeChunk},
		{size: 50 * MB, result: RedundancyBaseSector*SizeBaseSector + 2*RedundancyChunk*SizeChunk},
		// 4MB base sector + 496MB overflow which fit in math.Ceil(496 / 40.0) = 13 chunks.
		{size: 500 * MB, result: RedundancyBaseSector*SizeBaseSector + 13*RedundancyChunk*SizeChunk},
	}
	for _, tt := range tests {
		res := BandwidthUploadCost(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MB file to result into %d MB upload bandwidth, got %d MB.",
				tt.size/MB, tt.result/MB, res/MB)
		}
	}
}

// TestBandwidthDownloadCost ensures BandwidthDownloadCost works as expected.
func TestBandwidthDownloadCost(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0, result: 200 * KB},
		{size: 1 * MB, result: 200*KB + 1*MB},
		{size: 1*MB + 1, result: 200*KB + 1*MB + 64},
		{size: 4 * MB, result: 200*KB + 4*MB},
		{size: 4*MB + 1, result: 200*KB + 4*MB + 64},
		{size: 50 * MB, result: 200*KB + 50*MB},
		{size: 500*MB + 1, result: 200*KB + 500*MB + 64},
	}
	for _, tt := range tests {
		res := BandwidthDownloadCost(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %dB file to result into %dB upload bandwidth, got %dB.",
				tt.size, tt.result, res)
		}
	}
}
