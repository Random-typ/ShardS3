package metadata

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"shards3/services/shards3/internal/modules/storage/chunker"
	"shards3/services/shards3/internal/modules/storage/compression"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/interfaces"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/modules/storage/shard"
	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"

	"github.com/google/uuid"
)

// setupTestDB configures the metadata package against a fresh, temporary
// SQLite database for the duration of the test.
func setupTestDB(t *testing.T) {
	t.Helper()

	config.Cfg.SQLitePath = filepath.Join(t.TempDir(), "metadata_test.db")
	config.Cfg.SQLiteBusyTimeoutMS = 5000
	config.Cfg.SQLiteMaxOpenConns = 1

	database, err := db.New()
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
	})

	Configure(database)

	// sampleChunk() references a fixed kms_key_id of 7; back it with a real
	// row so the chunks.kms_key_id foreign key is satisfied.
	if _, err := database.Exec(`INSERT INTO kms_keys (id, key_ciphertext) VALUES (7, X'00')`); err != nil {
		t.Fatalf("seed kms key: %v", err)
	}
}

// sampleChunk builds a deterministic chunk with two shards, using ordinal to
// keep shard locations unique across chunks within a test.
func sampleChunk(ordinal int) chunker.Chunk {
	return chunker.Chunk{
		Id:                uuid.New(),
		Size:              2000,
		EncodedShardSize:  1000,
		EncodedDataShards: 2,
		Encryption:        chunker.Encryption{Type: encryption.AES_256_GCM, KeyId: encryption.KeyID(7)},
		Shards: []shard.Shard{
			{First: 0, Last: 999, Backend: interfaces.BackendType("file"), Location: fmt.Sprintf("chunk-%d-shard-0", ordinal), Checksum: 111},
			{First: 1000, Last: 1999, Backend: interfaces.BackendType("file2"), Location: fmt.Sprintf("chunk-%d-shard-1", ordinal), Checksum: 222},
		},
	}
}

func TestObjectLifecycle(t *testing.T) {
	setupTestDB(t)

	location := object.ObjectLocation{Bucket: object.Bucket{Name: "test-bucket"}, Key: "path/to/object.txt"}

	chunk0 := sampleChunk(0)
	chunk1 := sampleChunk(1)

	obj := object.Object{
		Location:    location,
		Size:        4000,
		Compression: compression.Compression{Type: compression.Zstd, Level: 3},
		Chunks:      []chunker.Chunk{chunk0, chunk1},
	}

	if err := PutObject(obj); err != nil {
		t.Fatalf("PutObject() error: %v", err)
	}

	got, err := GetObject(location)
	if err != nil {
		t.Fatalf("GetObject() error: %v", err)
	}
	if got.Size != obj.Size {
		t.Fatalf("Size mismatch: got=%d want=%d", got.Size, obj.Size)
	}
	if got.Compression != obj.Compression {
		t.Fatalf("Compression mismatch: got=%+v want=%+v", got.Compression, obj.Compression)
	}
	if got.LastModified.IsZero() {
		t.Fatal("expected LastModified to be set")
	}
	if !reflect.DeepEqual(got.Chunks, obj.Chunks) {
		t.Fatalf("GetObject().Chunks mismatch:\ngot=%+v\nwant=%+v", got.Chunks, obj.Chunks)
	}

	chunks, err := GetChunks(location)
	if err != nil {
		t.Fatalf("GetChunks() error: %v", err)
	}
	if !reflect.DeepEqual(chunks, obj.Chunks) {
		t.Fatalf("GetChunks() mismatch:\ngot=%+v\nwant=%+v", chunks, obj.Chunks)
	}

	shards, err := GetShards(chunk0.Id)
	if err != nil {
		t.Fatalf("GetShards() error: %v", err)
	}
	if !reflect.DeepEqual(shards, chunk0.Shards) {
		t.Fatalf("GetShards() mismatch:\ngot=%+v\nwant=%+v", shards, chunk0.Shards)
	}

	list, hasMore, err := ListObjects(location.Bucket, "", "/", "", 1, 100)
	if err != nil {
		t.Fatalf("ListObjects() error: %v", err)
	}
	if len(list) != 1 || list[0].Location.Key != location.Key {
		t.Fatalf("ListObjects() mismatch: %+v", list)
	}
	if hasMore {
		t.Fatalf("ListObjects() unexpected hasMore: %v", hasMore)
	}

	// Update with a new set of chunks; the old chunks (and their shards) must
	// be replaced entirely.
	newChunk := sampleChunk(2)
	updatedObj := obj
	updatedObj.Size = 2000
	updatedObj.Chunks = []chunker.Chunk{newChunk}

	if err := UpdateObject(updatedObj); err != nil {
		t.Fatalf("UpdateObject() error: %v", err)
	}

	got, err = GetObject(location)
	if err != nil {
		t.Fatalf("GetObject() after update error: %v", err)
	}
	if got.Size != updatedObj.Size {
		t.Fatalf("Size after update mismatch: got=%d want=%d", got.Size, updatedObj.Size)
	}
	if !reflect.DeepEqual(got.Chunks, updatedObj.Chunks) {
		t.Fatalf("Chunks after update mismatch:\ngot=%+v\nwant=%+v", got.Chunks, updatedObj.Chunks)
	}

	// The previous chunks' shards must be gone too (cascade delete).
	oldShards, err := GetShards(chunk0.Id)
	if err != nil {
		t.Fatalf("GetShards() for removed chunk error: %v", err)
	}
	if len(oldShards) != 0 {
		t.Fatalf("expected no shards for removed chunk, got %+v", oldShards)
	}

	oldChunks, err := GetChunks(location)
	if err != nil {
		t.Fatalf("GetChunks() after update error: %v", err)
	}
	if len(oldChunks) != 1 || oldChunks[0].Id != newChunk.Id {
		t.Fatalf("expected only the new chunk after update, got %+v", oldChunks)
	}

	if err := DeleteObject(location); err != nil {
		t.Fatalf("DeleteObject() error: %v", err)
	}

	if _, err := GetObject(location); err == nil {
		t.Fatal("expected error getting deleted object")
	}

	remainingChunks, err := GetChunks(location)
	if err != nil {
		t.Fatalf("GetChunks() after delete error: %v", err)
	}
	if len(remainingChunks) != 0 {
		t.Fatalf("expected no chunks after object delete, got %+v", remainingChunks)
	}

	remainingShards, err := GetShards(newChunk.Id)
	if err != nil {
		t.Fatalf("GetShards() after delete error: %v", err)
	}
	if len(remainingShards) != 0 {
		t.Fatalf("expected no shards after object delete, got %+v", remainingShards)
	}

	if err := DeleteObject(location); err == nil {
		t.Fatal("expected error deleting an already-deleted object")
	}
}

func TestBucketLifecycle(t *testing.T) {
	setupTestDB(t)

	bucket := object.Bucket{Name: "lifecycle-bucket"}

	if err := CreateBucket(bucket); err != nil {
		t.Fatalf("CreateBucket() error: %v", err)
	}

	if err := CreateBucket(bucket); err == nil {
		t.Fatal("expected error creating a duplicate bucket")
	}

	buckets, _, err := ListBuckets("", 1, 100)
	if err != nil {
		t.Fatalf("ListBuckets() error: %v", err)
	}
	if !containsBucket(buckets, bucket) {
		t.Fatalf("expected bucket %q in ListBuckets(), got %+v", bucket.Name, buckets)
	}

	if err := DeleteBucket(bucket); err != nil {
		t.Fatalf("DeleteBucket() error: %v", err)
	}

	buckets, _, err = ListBuckets("", 1, 100)
	if err != nil {
		t.Fatalf("ListBuckets() error: %v", err)
	}
	if containsBucket(buckets, bucket) {
		t.Fatalf("expected bucket %q to be removed, got %+v", bucket.Name, buckets)
	}

	if err := DeleteBucket(bucket); err == nil {
		t.Fatal("expected error deleting an already-deleted bucket")
	}
}

func TestListBackendStats(t *testing.T) {
	setupTestDB(t)

	bucket := object.Bucket{Name: "backend-stats-bucket"}
	if err := CreateBucket(bucket); err != nil {
		t.Fatalf("CreateBucket() error: %v", err)
	}

	obj := object.Object{
		Location:    object.ObjectLocation{Bucket: bucket, Key: "obj.bin"},
		Size:        4000,
		Compression: compression.Compression{Type: compression.None, Level: 0},
		Chunks:      []chunker.Chunk{sampleChunk(0), sampleChunk(1)},
	}
	if err := PutObject(obj); err != nil {
		t.Fatalf("PutObject() error: %v", err)
	}

	stats, err := ListBackendStats()
	if err != nil {
		t.Fatalf("ListBackendStats() error: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 backend stats entries, got %d", len(stats))
	}

	byBackend := map[string]object.BackendStats{}
	for _, stat := range stats {
		byBackend[stat.Backend] = stat
	}

	fileStat, ok := byBackend["file"]
	if !ok {
		t.Fatalf("missing file backend stats: %+v", stats)
	}
	if fileStat.TotalShards != 2 || fileStat.TotalBytes != 2000 || fileStat.TotalChunks != 2 || fileStat.TotalObjects != 1 || fileStat.TotalBuckets != 1 {
		t.Fatalf("unexpected file backend stats: %+v", fileStat)
	}
	if fileStat.LastVerified.IsZero() {
		t.Fatalf("expected file backend LastVerified to be set")
	}

	file2Stat, ok := byBackend["file2"]
	if !ok {
		t.Fatalf("missing file2 backend stats: %+v", stats)
	}
	if file2Stat.TotalShards != 2 || file2Stat.TotalBytes != 2000 || file2Stat.TotalChunks != 2 || file2Stat.TotalObjects != 1 || file2Stat.TotalBuckets != 1 {
		t.Fatalf("unexpected file2 backend stats: %+v", file2Stat)
	}
	if file2Stat.LastVerified.IsZero() {
		t.Fatalf("expected file2 backend LastVerified to be set")
	}
}

func containsBucket(buckets []object.Bucket, name object.Bucket) bool {
	for _, b := range buckets {
		if b == name {
			return true
		}
	}
	return false
}
