package shard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/platform/config"
	"testing"
)

func runLifecycleTest(t *testing.T, payloadSize int, testStorageDir string, backends ...interfaces.BackendType) {
	data := make([]byte, payloadSize)
	for i := range data {
		data[i] = byte(i % 251)
	}

	shards, encodedDataShardCount, encodedShardSize, err := ShardData(data, backends)
	if err != nil {
		t.Fatalf("ShardData() error: %v", err)
	}
	if len(shards) == 0 {
		t.Fatal("ShardData() returned no shard metadata")
	}

	maxShardIndex := 0
	for _, shard := range shards {
		if shard.Last > maxShardIndex {
			maxShardIndex = shard.Last
		}
	}
	encodedParityShardCount := maxShardIndex + 1 - encodedDataShardCount

	reconstructed, err := CollectShards(shards, encodedShardSize, encodedDataShardCount, encodedParityShardCount)
	if err != nil {
		t.Fatalf("CollectShards() error: %v", err)
	}
	if len(reconstructed) < len(data) {
		t.Fatalf("reconstructed data too short: got=%d want-at-least=%d", len(reconstructed), len(data))
	}

	for i := range data {
		if reconstructed[i] != data[i] {
			t.Fatalf("reconstructed data mismatch at byte %d: got=%d want=%d", i, reconstructed[i], data[i])
		}
	}

	if err := DestroyShards(shards); err != nil {
		t.Fatalf("DestroyShards() error: %v", err)
	}

	for _, shard := range shards {
		location := filepath.Join(testStorageDir, shard.Location)
		_, statErr := os.Stat(location)
		if !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("expected shard file to be removed at %q, got stat error: %v", location, statErr)
		}
	}

}

type lifecycleTestCase struct {
	backends         []interfaces.BackendType
	failureTolerance int
}

func generateSucceedingLifecycleTestCases(maxBackendCount int) []lifecycleTestCase {
	healthy := interfaces.RegisterFileTestBackends(5)

	if maxBackendCount > len(healthy) {
		maxBackendCount = len(healthy)
	}

	cases := make([]lifecycleTestCase, 0)
	for backendCount := 1; backendCount <= maxBackendCount; backendCount++ {
		for failureTolerance := 0; failureTolerance < backendCount; failureTolerance++ {
			backendsAllHealthy := append([]interfaces.BackendType(nil), healthy[:backendCount]...)
			cases = append(cases, lifecycleTestCase{
				backends:         backendsAllHealthy,
				failureTolerance: failureTolerance,
			})
		}
	}

	return cases
}

func TestLifecycle_FileBackendOnly(t *testing.T) {
	testStorageDir := filepath.Join(".", "testdata")
	if err := os.MkdirAll(testStorageDir, 0o755); err != nil {
		t.Fatalf("failed to create test storage directory: %v", err)
	}

	testCases := generateSucceedingLifecycleTestCases(5)

	shardSize, err := interfaces.GetMaxShardSize(testCases[0].backends[0])
	if err != nil {
		t.Fatalf("failed to get max shard size for file backend: %v", err)
	}

	payloadSizes := []int{shardSize / 2, shardSize, shardSize * 2}

	for _, tc := range testCases {
		config.Cfg.FailureTolerance = tc.failureTolerance
		for _, payloadSize := range payloadSizes {
			testName := fmt.Sprintf("Backends_%d_Tolerance_%d_PayloadSize_%d", len(tc.backends), tc.failureTolerance, payloadSize)
			t.Run(testName, func(t *testing.T) {
				runLifecycleTest(t, payloadSize, testStorageDir, tc.backends...)
			})
		}
	}
}
