package shard

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/platform/config"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/klauspost/reedsolomon"
	"golang.org/x/sync/errgroup"
)

type Shard struct {
	// The first and last index of the shard in the original data. This is used to reconstruct the original data from the shards.
	First int
	Last  int

	Backend interfaces.BackendType

	// string used by the backend to identify the shard.
	Location string

	// the last time the checksum was verified. This is used to determine if the shard needs to be re-verified.
	lastVerified time.Time

	Checksum int64
}

type RawShard struct {
	// The first and last index of the shard in the original data. This is used to reconstruct the original data from the shards.
	First int
	Last  int
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
func putShards(shards []RawShard, backend interfaces.BackendType) ([]Shard, error) {
	storedShards := make([]Shard, 0, len(shards))
	for _, shard := range shards {
		log.Printf("trace shard_upload begin backend=%d range=[%d,%d] bytes=%d", backend, shard.First, shard.Last, len(shard.data))
		location, err := interfaces.PutShard(backend, shard.data)
		if err != nil {
			log.Printf("trace shard_upload failed backend=%d range=[%d,%d] err=%v", backend, shard.First, shard.Last, err)
			return nil, err
		}
		log.Printf("trace shard_upload done backend=%d range=[%d,%d] location=%s", backend, shard.First, shard.Last, location)
		storedShards = append(storedShards, Shard{
			First:    shard.First,
			Last:     shard.Last,
			Backend:  backend,
			Location: location,
			Checksum: GenerateChecksum(shard.data),
		})
	}
	return storedShards, nil
}

func encodeParityShards(data []byte, backendCount int, minShardSize int) ([][]byte, int /*data shards count*/, error) {
	dataCount := int(math.Ceil(float64(len(data)) / float64(minShardSize)))
	parityCount := getShardParityCount(backendCount, dataCount)
	totalShards := dataCount + parityCount

	// Create parity and data shards using Reed-Solomon encoding
	enc, err := reedsolomon.New(dataCount, parityCount)
	if err != nil {
		return nil, 0, err
	}
	parityData := make([][]byte, totalShards)
	for i := range parityData {
		parityData[i] = make([]byte, minShardSize)
		if i < dataCount {
			start := i * minShardSize
			end := min((i+1)*minShardSize, len(data))
			copy(parityData[i], data[start:end])
		}
	}

	if err := enc.Encode(parityData); err != nil {
		return nil, 0, err
	}

	return parityData, dataCount, nil
}

// Splits data into shards, encodes parity shards, and distributes them across the specified backends.
func ShardData(data []byte, backend []interfaces.BackendType) ([]Shard, int /*data shards count*/, int /*encoding shard size*/, error) {
	if len(backend) == 0 {
		return nil, 0, 0, fmt.Errorf("at least one backend is required")
	}
	if len(data) == 0 {
		return nil, 0, 0, fmt.Errorf("cannot shard empty data")
	}

	minShardSize := int(^uint(0) >> 1) // Initialize to max int
	for _, b := range backend {
		shardSize, err := interfaces.GetMaxShardSize(b)
		if err != nil {
			return nil, 0, 0, err
		}
		minShardSize = min(minShardSize, shardSize)
	}

	if len(data) > minShardSize {
		const maxDivide = 10
		const minCandidateSize = 1024 * 50 // 50KiB
		bestShardSize := minShardSize
		leastLeftover := len(data) % bestShardSize
		for i := 1; i <= maxDivide; i++ {
			candidate := minShardSize >> i // divide by 2^i
			if candidate < minCandidateSize {
				break
			}

			leftover := len(data) % candidate
			if leftover < leastLeftover {
				bestShardSize = candidate
				leastLeftover = leftover
			}

			if leftover == 0 {
				break
			}
		}
		minShardSize = bestShardSize
	}

	parityData, dataShardsCount, err := encodeParityShards(data, len(backend), minShardSize)
	if err != nil {
		return nil, 0, 0, err
	}

	// Distribute encoded shards evenly across backends. Each backend gets a
	// contiguous block of encoded shard indices (rather than an interleaved
	// round-robin assignment), since merged shards are later addressed by
	// their [First,Last] index range - that range must map to a contiguous
	// run of encoded shards for reconstruction to work. Block sizes still
	// differ by at most one shard across backends, so the failure-tolerance
	// math in getShardParityCount (which assumes an even split) still holds.
	rawShards := make(map[interfaces.BackendType][]RawShard)
	totalEncodedShards := len(parityData)
	baseCount := totalEncodedShards / len(backend)
	extraCount := totalEncodedShards % len(backend)
	nextIndex := 0
	for backendIndex, b := range backend {
		count := baseCount
		if backendIndex < extraCount {
			count++
		}
		for c := 0; c < count; c++ {
			rawShards[b] = append(rawShards[b], RawShard{
				First: nextIndex,
				Last:  nextIndex,
				data:  parityData[nextIndex],
			})
			nextIndex++
		}
	}

	// Merge shards for each backend up to its max shard size.
	finalRawShards := make(map[interfaces.BackendType][]RawShard)
	for _, b := range backend {
		shardSize, err := interfaces.GetMaxShardSize(b)
		if err != nil {
			return nil, 0, 0, err
		}

		for i := 0; i < len(rawShards[b]); {
			mergedData := make([]byte, 0)
			j := i
			for ; j < len(rawShards[b]); j++ {
				if len(mergedData)+len(rawShards[b][j].data) > shardSize {
					break
				}
				mergedData = append(mergedData, rawShards[b][j].data...)
			}

			finalRawShards[b] = append(finalRawShards[b], RawShard{
				First: rawShards[b][i].First,
				Last:  rawShards[b][j-1].Last,
				data:  mergedData,
			})
			i = j
		}
	}

	// Upload each backend's shards concurrently - the backends are
	// independent of one another, so there is no reason to wait for one
	// backend's (potentially slow, network-bound) uploads to finish before
	// starting the next. Results are written into disjoint, preallocated
	// slice indices so no synchronization is needed beyond errgroup.Wait().
	perBackendShards := make([][]Shard, len(backend))
	var g errgroup.Group
	for i, b := range backend {
		i, b := i, b
		g.Go(func() error {
			storedShards, err := putShards(finalRawShards[b], b)
			if err != nil {
				return err
			}
			perBackendShards[i] = storedShards
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, 0, err
	}

	finalShards := make([]Shard, 0, len(parityData))
	for _, shards := range perBackendShards {
		finalShards = append(finalShards, shards...)
	}

	return finalShards, dataShardsCount, minShardSize, nil
}

// CollectShards collects the shards and reconstructs the original data.
func CollectShards(shards []Shard, encodedShardSize int, encodedDataShardCount int, encodedParityShardCount int) ([]byte, error) {
	parityData := make([][]byte, encodedDataShardCount+encodedParityShardCount)
	collectedShardCount := 0

	for _, shard := range shards {
		data, err := interfaces.GetShard(shard.Backend, shard.Location)
		if err != nil {
			continue
		}

		for shardIndex := shard.First; shardIndex <= shard.Last; shardIndex++ {
			segmentIndex := shardIndex - shard.First
			start := segmentIndex * encodedShardSize
			end := start + encodedShardSize
			if end > len(data) || shardIndex >= len(parityData) {
				return nil, fmt.Errorf("invalid shard payload for shard range [%d,%d]", shard.First, shard.Last)
			}

			if parityData[shardIndex] == nil {
				parityData[shardIndex] = make([]byte, encodedShardSize)
				copy(parityData[shardIndex], data[start:end])
				collectedShardCount++
			}
		}
	}

	if collectedShardCount < encodedDataShardCount {
		return nil, fmt.Errorf("not enough shards collected to reconstruct the data")
	}

	enc, err := reedsolomon.New(encodedDataShardCount, encodedParityShardCount)
	if err != nil {
		return nil, err
	}
	if err := enc.Reconstruct(parityData); err != nil {
		return nil, err
	}

	var restored bytes.Buffer
	for i := 0; i < encodedDataShardCount; i++ {
		if parityData[i] == nil {
			return nil, fmt.Errorf("missing reconstructed data shard at index %d", i)
		}
		restored.Write(parityData[i])
	}

	return restored.Bytes(), nil
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
