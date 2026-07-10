package shard

import (
	"fmt"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/platform/config"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/klauspost/reedsolomon"
)

type Shard struct {
	Order int

	Backend interfaces.BackendType

	// string used by the backend to identify the shard.
	Location string

	// the last time the checksum was verified. This is used to determine if the shard needs to be re-verified.
	lastVerified time.Time

	Checksum int64
}

type RawShard struct {
	Order int
	data  []byte
}

func (s Shard) VerifyChecksum() bool {
	data, err := interfaces.GetShard(s.Backend, s.Location)
	if err != nil {
		return false
	}
	if s.Checksum != GenerateChecksum(data) {
		return false
	}
	s.lastVerified = time.Now()
	return true
}

func GenerateChecksum(data []byte) int64 {
	return int64(xxhash.Sum64(data))
}

func getShardParityCount(backendCount int, dataShardsCount int) int {
	for parity := 0; ; parity++ {
		total := dataShardsCount + parity

		perBackend := total / backendCount
		extra := total % backendCount

		// worst case: fill the failed backends first
		lost := config.Cfg.FailureTolerance * perBackend
		if config.Cfg.FailureTolerance < extra {
			lost += config.Cfg.FailureTolerance
		} else {
			lost += extra
		}

		if parity >= lost {
			return parity
		}
	}
}

// put a list of shards into the specified backend
func putShards(shards []RawShard, backend interfaces.BackendType, storedShards []Shard, wg *sync.WaitGroup) error {
	defer wg.Done()
	for _, shard := range shards {
		location, err := interfaces.PutShard(backend, shard.data)
		if err != nil {
			return err
		}
		storedShards = append(storedShards, Shard{
			Order:    shard.Order,
			Backend:  backend,
			Location: location,
			Checksum: GenerateChecksum(shard.data),
		})
	}
	return nil
}

func encodeParityShards(data []byte, backendCount int, minShardSize int) ([][]byte, int /*data shards count*/, error) {
	var dataCount = len(data) / minShardSize
	var parityCount = getShardParityCount(backendCount, dataCount)
	var totalShards = dataCount + parityCount

	// Create parity and data shards using Reed-Solomon encoding
	enc, err := reedsolomon.New(dataCount, parityCount)
	if err != nil {
		return nil, 0, err
	}
	parityData := make([][]byte, totalShards)
	// Create all shards, size them at minShardSize each and fill the data shard with data
	for i := range parityData {
		parityData[i] = make([]byte, minShardSize)
		if i < dataCount {
			copy(parityData[i], data[i*minShardSize:(i+1)*minShardSize])
		}
	}
	// Encode the parity shards
	err = enc.Encode(parityData)
	if err != nil {
		return nil, 0, err
	}

	return parityData, dataCount, nil
}

// Splits data into shards, encodes parity shards, and distributes them across the specified backends.
func ShardData(data []byte, backend []interfaces.BackendType) ([]Shard, int /*data shards count*/, int /*encoding shard size*/, error) {
	var shards = make(map[interfaces.BackendType][]Shard)
	var minShardSize = int(^uint(0) >> 1) // Initialize to max int
	for _, b := range backend {
		shardSize, err := interfaces.GetMaxShardSize(b)
		if err != nil {
			return nil, 0, 0, err
		}
		minShardSize = min(minShardSize, shardSize)
	}
	// find optimal placement
	// Each backend cannot have more than config.Cfg.Parity number of parity/data shards
	// parity/data shards are merged for each backend up to its max shard size -> less shards

	// Encode the data and add parity shards using Reed-Solomon encoding
	parityData, dataShardsCount, err := encodeParityShards(data, len(backend), minShardSize)
	if err != nil {
		return nil, 0, 0, err
	}

	// Distributing the shards evenly between the backends
	var rawShards = make(map[interfaces.BackendType][]RawShard)
	for i, shardData := range parityData {
		backendIndex := i % len(backend)
		rawShards[backend[backendIndex]] = append(rawShards[backend[backendIndex]], RawShard{
			Order: i,
			data:  shardData,
		})
	}

	// Merge shards for each backend up to its max shard size
	var finalRawShards = make(map[interfaces.BackendType][]RawShard)
	for _, b := range backend {
		shardSize, err := interfaces.GetMaxShardSize(b)
		if err != nil {
			return nil, 0, 0, err
		}
		for i := 0; i < len(rawShards[b]); {
			var mergedData []byte
			var j int
			for j = i; j < len(rawShards[b]); j++ {
				if len(mergedData)+len(rawShards[b][j].data) > shardSize {
					break
				}
				mergedData = append(mergedData, rawShards[b][j].data...)
			}
			finalRawShards[b] = append(finalRawShards[b], RawShard{
				Order: i,
				data:  mergedData,
			})
			i = j
		}
	}

	// put the shards for each backend
	var wg sync.WaitGroup
	for _, b := range backend {
		wg.Add(1)
		go putShards(finalRawShards[b], b, shards[b], &wg)
	}
	wg.Wait()

	// collect all shards from different backends into a single slice
	finalShards := []Shard{}
	for _, b := range backend {
		finalShards = append(finalShards, shards[b]...)
	}
	return finalShards, dataShardsCount, minShardSize, nil
}

func collectBackendObject(shards []Shard, backendType interfaces.BackendType, fulfilledShardCount *atomic.Int64, encodedDataShardCount int, encodedShardSize int, encodedData [][]byte, wg *sync.WaitGroup) error {
	defer wg.Done()

	var currentShardOffset int = 0
	var iShard int = 0
	for _, shard := range shards {
		if fulfilledShardCount.Load() >= int64(encodedDataShardCount) {
			return nil
		}
		if shard.Backend != backendType {
			return nil
		}
		data, err := interfaces.GetShard(shard.Backend, shard.Location)
		if err != nil {
			return fmt.Errorf("failed to get shard from backend %v: %w", shard.Backend, err)
		}
		// split backend shard into encoded shards and copy them into the encodedData slice
		for offset := 0; offset < len(data); {
			remaining := len(data) - offset
			toCopy := min(min(remaining, encodedShardSize), encodedShardSize-currentShardOffset)
			if currentShardOffset == 0 {
				encodedData[iShard] = make([]byte, encodedShardSize)
			}
			copy(encodedData[iShard][currentShardOffset:currentShardOffset+toCopy], data[offset:offset+toCopy])
			currentShardOffset += toCopy
			offset += toCopy
			if currentShardOffset == encodedShardSize {
				iShard++
				currentShardOffset = 0
				fulfilledShardCount.Add(1)
			}
		}
	}

	return nil
}

// CollectShards collects the shards and reconstructs the original data.
func CollectShards(shards []Shard, encodedShardSize int, encodedDataShardCount int) ([]byte, error) {
	var data []byte

	// Sort shards by backend
	sort.Slice(shards, func(i, j int) bool {
		return shards[i].Backend < shards[j].Backend
	})

	// Collect shards from each backend concurrently
	// We try to make the least amount of requests necessary.
	// Only the minimum required number of shards is collected and the data is reconstructed.
	// encodedDataShardCount is the minimum number of shards required to reconstruct the data.
	var encodedShards = make(map[interfaces.BackendType][][]byte)
	fulfilledShardCount := atomic.Int64{}
	var wg sync.WaitGroup
	for i := 0; i < len(shards); {

		wg.Add(1)
		go collectBackendObject(shards[i:], shards[i].Backend, &fulfilledShardCount, encodedDataShardCount, encodedShardSize, encodedShards[shards[i].Backend], &wg)

		// go to the next backend
		for i < len(shards)-1 && shards[i].Backend == shards[i+1].Backend {
			i++
		}
	}
	wg.Wait()
	if fulfilledShardCount.Load() < int64(encodedDataShardCount) {
		return nil, fmt.Errorf("not enough shards collected to reconstruct the data")
	}
	// Order data by shard order
	var parityData [][]byte
	for backend, backendShards := range encodedShards {
		sort.Slice(backendShards, func(i, j int) bool {
			return shards[i].Order < shards[j].Order
		})
	}

	// Reconstruct the original data using Reed-Solomon decoding
	enc, err := reedsolomon.New(encodedDataShardCount, len(shards)-encodedDataShardCount)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func DestroyShards(shards []Shard) error {
	for _, shard := range shards {
		err := interfaces.DeleteShard(shard.Backend, shard.Location)
		if err != nil {
			return err
		}
	}
	return nil
}
