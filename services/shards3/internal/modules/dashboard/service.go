package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/metadata"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/platform/db"
)

type Service struct {
	database *db.DB
}

type Health struct {
	Message string
}

type Bucket struct {
	Name        string
	Size        int64
	ObjectCount int64
}

type ObjectEntry struct {
	Name         string
	Path         string
	IsDir        bool
	LastModified time.Time
	Size         int64
	ChunkCount   int
	ShardCount   int
}

type BackendStats struct {
	Name            string
	Configured      bool
	TotalShards     int64
	TotalBytes      int64
	TotalChunks     int64
	TotalObjects    int64
	TotalBuckets    int64
	BytesShare      float64
	ShardsShare     float64
	LastVerified    time.Time
	HasLastVerified bool
	MaxShardSize    int
	MaxShardSizeErr string
}

func NewService(database *db.DB) *Service {
	return &Service{database: database}
}

func (s *Service) Health(ctx context.Context) (Health, error) {
	if s.database == nil {
		return Health{}, errors.New("dashboard database not configured")
	}

	if err := s.database.PingContext(ctx); err != nil {
		return Health{}, fmt.Errorf("database health check: %w", err)
	}

	return Health{Message: "ok"}, nil
}

func (s *Service) ListBuckets() []Bucket {
	stats, err := metadata.ListBucketsWithStats()
	if err != nil {
		buckets, _, err := metadata.ListBuckets("", 1, 100)
		if err != nil {
			return nil
		}
		result := make([]Bucket, 0, len(buckets))
		for _, bucket := range buckets {
			result = append(result, Bucket{Name: bucket.Name})
		}

		return result
	}

	result := make([]Bucket, 0, len(stats))
	for _, bucket := range stats {
		result = append(result, Bucket{
			Name:        bucket.Bucket.Name,
			Size:        bucket.TotalSize,
			ObjectCount: bucket.TotalObjects,
		})
	}

	return result
}

func (s *Service) ListBucketsWithStats() ([]object.BucketStats, error) {
	buckets, err := metadata.ListBucketsWithStats()
	if err != nil {
		return nil, fmt.Errorf("list buckets with stats: %w", err)
	}
	return buckets, nil
}

func (s *Service) ListObjects(bucketName string, prefix string) ([]object.Object, bool, error) {
	return metadata.ListObjects(object.Bucket{Name: bucketName}, prefix, "/", "", 1, 100)
}

// demoBackendStats returns fabricated backend usage for screenshots. Remove to restore live stats.
func demoBackendStats() []BackendStats {
	const gib = int64(1) << 30
	const mib = int64(1) << 20

	now := time.Now()
	result := []BackendStats{
		{
			Name:            "telegram",
			Configured:      true,
			TotalShards:     48213,
			TotalBytes:      812*gib + 344*mib,
			TotalChunks:     16071,
			TotalObjects:    9284,
			TotalBuckets:    7,
			LastVerified:    now.Add(-11 * time.Minute),
			HasLastVerified: true,
			MaxShardSize:    int(2 * gib),
		},
		{
			Name:            "youtube",
			Configured:      true,
			TotalShards:     379,
			TotalBytes:      623*gib + 902*mib,
			TotalChunks:     12646,
			TotalObjects:    7115,
			TotalBuckets:    6,
			LastVerified:    now.Add(-38 * time.Minute),
			HasLastVerified: true,
			MaxShardSize:    int(2 * gib),
		},
		{
			Name:            "discord",
			Configured:      true,
			TotalShards:     52887,
			TotalBytes:      471*gib + 118*mib,
			TotalChunks:     17629,
			TotalObjects:    8402,
			TotalBuckets:    7,
			LastVerified:    now.Add(-4 * time.Minute),
			HasLastVerified: true,
			MaxShardSize:    int(10 * mib),
		},
	}

	var maxBytes, maxShards int64 = 1, 1
	for _, entry := range result {
		if entry.TotalBytes > maxBytes {
			maxBytes = entry.TotalBytes
		}
		if entry.TotalShards > maxShards {
			maxShards = entry.TotalShards
		}
	}
	for i := range result {
		result[i].BytesShare = percentOf(result[i].TotalBytes, maxBytes)
		result[i].ShardsShare = percentOf(result[i].TotalShards, maxShards)
	}

	return result
}

func (s *Service) ListBackendStats() ([]BackendStats, error) {
	return demoBackendStats(), nil
}

func (s *Service) listBackendStatsLive() ([]BackendStats, error) {
	rawStats, err := metadata.ListBackendStats()
	if err != nil {
		return nil, fmt.Errorf("list backend stats: %w", err)
	}

	statsByName := make(map[string]object.BackendStats, len(rawStats))
	for _, stat := range rawStats {
		statsByName[stat.Backend] = stat
	}

	configuredSet := map[string]struct{}{}
	configured := interfaces.GetAvailableBackends()
	result := make([]BackendStats, 0, len(rawStats)+len(configured))

	var maxBytes int64 = 1
	var maxShards int64 = 1

	for _, backendID := range configured {
		name := string(backendID)
		configuredSet[name] = struct{}{}
		entry := BackendStats{Name: name, Configured: true}

		if stat, ok := statsByName[name]; ok {
			entry.TotalShards = stat.TotalShards
			entry.TotalBytes = stat.TotalBytes
			entry.TotalChunks = stat.TotalChunks
			entry.TotalObjects = stat.TotalObjects
			entry.TotalBuckets = stat.TotalBuckets
			entry.LastVerified = stat.LastVerified
			entry.HasLastVerified = !stat.LastVerified.IsZero()
		}

		maxSize, sizeErr := interfaces.GetMaxShardSize(backendID)
		if sizeErr != nil {
			entry.MaxShardSizeErr = sizeErr.Error()
		} else {
			entry.MaxShardSize = maxSize
		}

		if entry.TotalBytes > maxBytes {
			maxBytes = entry.TotalBytes
		}
		if entry.TotalShards > maxShards {
			maxShards = entry.TotalShards
		}

		result = append(result, entry)
	}

	for _, stat := range rawStats {
		if _, ok := configuredSet[stat.Backend]; ok {
			continue
		}

		entry := BackendStats{
			Name:            stat.Backend,
			Configured:      false,
			TotalShards:     stat.TotalShards,
			TotalBytes:      stat.TotalBytes,
			TotalChunks:     stat.TotalChunks,
			TotalObjects:    stat.TotalObjects,
			TotalBuckets:    stat.TotalBuckets,
			LastVerified:    stat.LastVerified,
			HasLastVerified: !stat.LastVerified.IsZero(),
		}

		if entry.TotalBytes > maxBytes {
			maxBytes = entry.TotalBytes
		}
		if entry.TotalShards > maxShards {
			maxShards = entry.TotalShards
		}

		result = append(result, entry)
	}

	for i := range result {
		result[i].BytesShare = percentOf(result[i].TotalBytes, maxBytes)
		result[i].ShardsShare = percentOf(result[i].TotalShards, maxShards)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Configured != result[j].Configured {
			return result[i].Configured
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

func (s *Service) BrowseObjects(bucketName string, prefix string) ([]ObjectEntry, error) {
	prefix = normalizePrefix(prefix)

	objects, _, err := s.ListObjects(bucketName, prefix)
	if err != nil {
		return nil, err
	}

	entries := make([]ObjectEntry, 0)
	dirs := make(map[string]struct{})

	for _, obj := range objects {
		key := string(obj.Location.Key)
		remainder := strings.TrimPrefix(key, prefix)
		if remainder == "" {
			continue
		}

		if slash := strings.Index(remainder, "/"); slash >= 0 {
			dirName := remainder[:slash]
			dirPath := prefix + dirName + "/"
			if _, exists := dirs[dirPath]; !exists {
				dirs[dirPath] = struct{}{}
				entries = append(entries, ObjectEntry{
					Name:  dirName,
					Path:  dirPath,
					IsDir: true,
				})
			}
			continue
		}

		chunks, err := metadata.GetChunks(obj.Location)
		if err != nil {
			return nil, fmt.Errorf("get chunks for %s: %w", key, err)
		}

		shardCount := 0
		for _, chunk := range chunks {
			shardCount += len(chunk.Shards)
		}

		entries = append(entries, ObjectEntry{
			Name:         remainder,
			Path:         key,
			IsDir:        false,
			LastModified: obj.LastModified,
			Size:         obj.Size,
			ChunkCount:   len(chunks),
			ShardCount:   shardCount,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries, nil
}

func (s *Service) CreateBucket(name string) error {
	if name == "" {
		return errors.New("bucket name is required")
	}

	return metadata.CreateBucket(object.Bucket{Name: name})
}

func (s *Service) DeleteBucket(name string) error {
	if name == "" {
		return errors.New("bucket name is required")
	}

	return metadata.DeleteBucket(object.Bucket{Name: name})
}

func (s *Service) bucketExists(name string) bool {
	for _, bucket := range s.ListBuckets() {
		if bucket.Name == name {
			return true
		}
	}

	return false
}

func (s *Service) deleteIfExists(name string) error {
	if !s.bucketExists(name) {
		return sql.ErrNoRows
	}

	return s.DeleteBucket(name)
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix == "" {
		return ""
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return prefix
}

func percentOf(value int64, max int64) float64 {
	if value <= 0 || max <= 0 {
		return 0
	}
	p := (float64(value) / float64(max)) * 100
	return math.Round(p*100) / 100
}
