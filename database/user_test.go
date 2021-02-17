package database

import "testing"

// TestNumChunks ensures that numChunks works as expected.
func TestNumChunks(t *testing.T) {
	tests := []struct {
		size   uint64
		result uint64
	}{
		{size: 0 * MiB, result: 0},
		{size: 1 * MiB, result: 0},
		{size: 4 * MiB, result: 0},
		{size: 5 * MiB, result: 1},
		{size: 50 * MiB, result: 2},
		{size: 500 * MiB, result: 13},
	}
	for _, tt := range tests {
		res := numChunks(tt.size)
		if res != tt.result {
			t.Errorf("Expected a %d MiB file to result into %d chunks, got %d.", tt.size/MiB, tt.result, res)
		}
	}
}

// TestStorageUsed ensures that StorageUsed works as expected.
func TestStorageUsed(t *testing.T) {
	tests := []struct {
		size   uint64
		result uint64
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
			t.Errorf("Expected a %d MiB file to result into %d MBs used for upload storage, got %d MiB.", tt.size/MiB, tt.result/MiB, res/MiB)
		}
	}
}
