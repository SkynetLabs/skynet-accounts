package database

import "testing"

// TestNumChunks ensures that numChunks works as expected.
func TestNumChunks(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0 * MiB, result: 0},
		{size: 1 * MiB, result: 0},
		{size: 4 * MiB, result: 0},
		{size: 5 * MiB, result: 1},
		{size: 50 * MiB, result: 2},
		{size: 500 * MiB, result: 13},
	}
	for _, tt := range tests {
		res := NumChunks(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MiB file to result into %d chunks, got %d.",
				tt.size/MiB, tt.result, res)
		}
	}
}

// TestStorageUsed ensures that StorageUsed works as expected.
func TestStorageUsed(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0, result: 4 * MiB},
		{size: 1 * MiB, result: 4 * MiB},
		{size: 4 * MiB, result: 4 * MiB},
		{size: 5 * MiB, result: (4 + 40) * MiB},
		{size: 50 * MiB, result: (4 + 2*40) * MiB},
		{size: 500 * MiB, result: (4 + 13*40) * MiB},
	}
	for _, tt := range tests {
		res := StorageUsed(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MiB file to result into %d MiB used for upload storage, got %d MiB.",
				tt.size/MiB, tt.result/MiB, res/MiB)
		}
	}
}

// TestBandwidthUploadCost ensures BandwidthUploadCost works as expected.
func TestBandwidthUploadCost(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0, result: 40 * MiB},
		{size: 1 * MiB, result: 40 * MiB},
		{size: 4 * MiB, result: 40 * MiB},
		{size: 5 * MiB, result: (40 + 120) * MiB},
		{size: 50 * MiB, result: (40 + 2*120) * MiB},
		{size: 500 * MiB, result: (40 + 13*120) * MiB},
	}
	for _, tt := range tests {
		res := BandwidthUploadCost(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MiB file to result into %d MiB upload bandwidth, got %d MiB.",
				tt.size/MiB, tt.result/MiB, res/MiB)
		}
	}
}

// TestBandwidthDownloadCost ensures BandwidthDownloadCost works as expected.
func TestBandwidthDownloadCost(t *testing.T) {
	tests := []struct {
		size   int64
		result int64
	}{
		{size: 0, result: 200 * KiB},
		{size: 1 * MiB, result: 200*KiB + 1*MiB},
		{size: 1*MiB + 1, result: 200*KiB + 1*MiB + 64},
		{size: 4 * MiB, result: 200*KiB + 4*MiB},
		{size: 4*MiB + 1, result: 200*KiB + 4*MiB + 64},
		{size: 50 * MiB, result: 200*KiB + 50*MiB},
		{size: 500*MiB + 1, result: 200*KiB + 500*MiB + 64},
	}
	for _, tt := range tests {
		res := BandwidthDownloadCost(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %dB file to result into %dB upload bandwidth, got %dB.",
				tt.size, tt.result, res)
		}
	}
}
