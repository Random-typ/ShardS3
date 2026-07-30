package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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
