package metadata

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"time"

	"shards3/internal/modules/storage/chunker"
	"shards3/internal/modules/storage/compression"
	"shards3/internal/modules/storage/object"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
)

// generate unique upload id with bucket name and uuid
// in normal AWS S3, the upload id is always paired with a bucket,
// thats why we put the bucket name in the upload id
// this is not really required, since uuid are unique enough.
func generateUploadID(bucket object.Bucket) string {
	return bucket.Name + "-" + uuid.New().String()
}

// CreateMultipartUpload registers a new in-progress multipart upload and
// returns its generated upload ID.
func CreateMultipartUpload(location object.ObjectLocation, comp compression.Compression) (string, error) {
	database, err := getDB()
	if err != nil {
		return "", err
	}

	uploadID := generateUploadID(location.Bucket)
	now := time.Now().UTC()

	tx, err := database.Begin()
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO buckets (name, created_at, updated_at) VALUES (?, ?, ?)`,
		string(location.Bucket.Name), now, now,
	); err != nil {
		return "", fmt.Errorf("ensure bucket: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO multipart_uploads (upload_id, bucket, object_key, compression_type, compression_level, initiated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uploadID, string(location.Bucket.Name), string(location.Key), int(comp.Type), comp.Level, now,
	); err != nil {
		return "", fmt.Errorf("create multipart upload: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit transaction: %w", err)
	}
	return uploadID, nil
}

func GetMultipartUploadByLocation(location object.ObjectLocation) (object.MultipartUpload, error) {
	database, err := getDB()
	if err != nil {
		return object.MultipartUpload{}, err
	}

	var bucket, key string
	var compType, compLevel int
	var initiated time.Time
	err = database.QueryRow(`
		SELECT bucket, object_key, compression_type, compression_level, initiated_at
		FROM multipart_uploads WHERE bucket = ? AND object_key = ?`,
		string(location.Bucket.Name), string(location.Key),
	).Scan(&bucket, &key, &compType, &compLevel, &initiated)
	if errors.Is(err, sql.ErrNoRows) {
		return object.MultipartUpload{}, fmt.Errorf("multipart upload not found for location: %s/%s", location.Bucket.Name, location.Key)
	}
	if err != nil {
		return object.MultipartUpload{}, fmt.Errorf("get multipart upload by location: %w", err)
	}

	return object.MultipartUpload{
		UploadID:    generateUploadID(location.Bucket), // This might need to be fetched from the database if stored
		Location:    object.ObjectLocation{Bucket: object.Bucket{Name: bucket}, Key: key},
		Compression: compression.Compression{Type: compression.CompressionType(compType), Level: compLevel},
		Initiated:   initiated,
	}, nil
}

func GetMultipartUpload(uploadID string) (object.MultipartUpload, error) {
	database, err := getDB()
	if err != nil {
		return object.MultipartUpload{}, err
	}

	var bucket, key string
	var compType, compLevel int
	var initiated time.Time
	err = database.QueryRow(`
		SELECT bucket, object_key, compression_type, compression_level, initiated_at
		FROM multipart_uploads WHERE upload_id = ?`,
		uploadID,
	).Scan(&bucket, &key, &compType, &compLevel, &initiated)
	if errors.Is(err, sql.ErrNoRows) {
		return object.MultipartUpload{}, fmt.Errorf("multipart upload not found: %s", uploadID)
	}
	if err != nil {
		return object.MultipartUpload{}, fmt.Errorf("get multipart upload: %w", err)
	}

	return object.MultipartUpload{
		UploadID:    uploadID,
		Location:    object.ObjectLocation{Bucket: object.Bucket{Name: bucket}, Key: key},
		Compression: compression.Compression{Type: compression.CompressionType(compType), Level: compLevel},
		Initiated:   initiated,
	}, nil
}

// ListMultipartUploads returns the in-progress uploads for a bucket, ordered
// by initiation time.
func ListMultipartUploads(bucket object.Bucket, prefix string, delim string, keyMarker string, uploadIDMarker string, max int) ([]object.MultipartUpload, bool, error) {
	database, err := getDB()
	if err != nil {
		return nil, false, err
	}

	rows, err := database.Query(`
		SELECT upload_id, object_key, compression_type, compression_level, initiated_at
		FROM multipart_uploads WHERE bucket = ? and object_key LIKE ? and object_key > ? and upload_id > ? ORDER BY bucket, object_key, upload_id ASC LIMIT ?`,
		string(bucket.Name), prefix+"%", keyMarker, uploadIDMarker, max+1,
	)
	if err != nil {
		return nil, false, fmt.Errorf("query multipart uploads: %w", err)
	}
	defer rows.Close()

	var uploads []object.MultipartUpload
	for rows.Next() {
		var uploadID, key string
		var compType, compLevel int
		var initiated time.Time
		if err := rows.Scan(&uploadID, &key, &compType, &compLevel, &initiated); err != nil {
			return nil, false, fmt.Errorf("scan multipart upload: %w", err)
		}
		uploads = append(uploads, object.MultipartUpload{
			UploadID:    uploadID,
			Location:    object.ObjectLocation{Bucket: bucket, Key: key},
			Compression: compression.Compression{Type: compression.CompressionType(compType), Level: compLevel},
			Initiated:   initiated,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate multipart uploads: %w", err)
	}
	hasMore := len(uploads) == max+1
	if len(uploads) > 0 && hasMore {
		uploads = uploads[:len(uploads)-1] // remove the extra item used to check for more
	}
	return uploads, hasMore, nil
}

// AbortMultipartUpload discards an in-progress upload; cascading foreign
// keys remove its parts, chunks and shards.
func AbortMultipartUpload(uploadID string) error {
	database, err := getDB()
	if err != nil {
		return err
	}

	res, err := database.Exec(`DELETE FROM multipart_uploads WHERE upload_id = ?`, uploadID)
	if err != nil {
		return fmt.Errorf("abort multipart upload: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("abort multipart upload: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("multipart upload not found: %s", uploadID)
	}
	return nil
}

// PutPart stores (or replaces, on re-upload) a single part's chunks for an
// in-progress multipart upload.
func PutPart(part object.MultipartPart) error {
	database, err := getDB()
	if err != nil {
		return err
	}

	upload, err := GetMultipartUpload(part.UploadID)
	if err != nil {
		return err
	}

	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Replace any previously stored chunks/shards for this part (re-upload).
	if _, err := tx.Exec(`DELETE FROM chunks WHERE upload_id = ? AND part_number = ?`, part.UploadID, part.PartNumber); err != nil {
		return fmt.Errorf("clear old part chunks: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO multipart_parts (upload_id, part_number, ETag, size, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(upload_id, part_number) DO UPDATE SET
			ETag = excluded.ETag,
			size = excluded.size,
			created_at = excluded.created_at`,
		part.UploadID, part.PartNumber, strconv.FormatUint(part.ETag, 10), part.Size, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("upsert multipart part: %w", err)
	}

	if err := insertPartChunks(tx, string(upload.Location.Bucket.Name), part.UploadID, part.PartNumber, part.Chunks); err != nil {
		return err
	}

	return tx.Commit()
}

func GetPart(uploadID string, partNumber int) (object.MultipartPart, error) {
	database, err := getDB()
	if err != nil {
		return object.MultipartPart{}, err
	}

	var etagStr string
	var size int64
	var created time.Time
	err = database.QueryRow(`
		SELECT ETag, size, created_at FROM multipart_parts WHERE upload_id = ? AND part_number = ?`,
		uploadID, partNumber,
	).Scan(&etagStr, &size, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return object.MultipartPart{}, fmt.Errorf("part not found: upload %s part %d", uploadID, partNumber)
	}
	if err != nil {
		return object.MultipartPart{}, fmt.Errorf("get multipart part: %w", err)
	}

	etag, err := strconv.ParseUint(etagStr, 10, 64)
	if err != nil {
		return object.MultipartPart{}, fmt.Errorf("parse part ETag: %w", err)
	}

	chunks, err := getPartChunks(database, uploadID, partNumber)
	if err != nil {
		return object.MultipartPart{}, err
	}

	return object.MultipartPart{
		UploadID:   uploadID,
		PartNumber: partNumber,
		ETag:       etag,
		Size:       size,
		CreatedAt:  created,
		Chunks:     chunks,
	}, nil
}

// ListParts returns the parts uploaded so far for an upload, ordered by part
// number, without loading their chunks.
func ListParts(uploadID string, max int, firstPartNumber int) ([]object.MultipartPart, bool, error) {
	database, err := getDB()
	if err != nil {
		return nil, false, err
	}

	rows, err := database.Query(`
		SELECT part_number, ETag, size, created_at FROM multipart_parts
		WHERE upload_id = ? AND part_number >= ? ORDER BY part_number ASC LIMIT ?`,
		uploadID, firstPartNumber, max+1,
	)
	if err != nil {
		return nil, false, fmt.Errorf("query multipart parts: %w", err)
	}
	defer rows.Close()

	var parts []object.MultipartPart
	for rows.Next() {
		var partNumber int
		var etagStr string
		var size int64
		var created time.Time
		if err := rows.Scan(&partNumber, &etagStr, &size, &created); err != nil {
			return nil, false, fmt.Errorf("scan multipart part: %w", err)
		}
		etag, err := strconv.ParseUint(etagStr, 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parse part ETag: %w", err)
		}
		parts = append(parts, object.MultipartPart{
			UploadID:   uploadID,
			PartNumber: partNumber,
			ETag:       etag,
			Size:       size,
			CreatedAt:  created,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate multipart parts: %w", err)
	}
	hasMore := len(parts) == max+1
	if len(parts) > 0 && hasMore {
		parts = parts[:len(parts)-1] // remove the extra part used to check for more
	}

	return parts, hasMore, nil
}

// CompleteMultipartUpload finalizes an upload into a regular object by
// re-parenting each part's already-uploaded chunks onto the final object -
// no shard data is re-uploaded or moved between backends.
func CompleteMultipartUpload(uploadID string) (object.Object, error) {
	database, err := getDB()
	if err != nil {
		return object.Object{}, err
	}

	upload, err := GetMultipartUpload(uploadID)
	if err != nil {
		return object.Object{}, err
	}

	parts, _, err := ListParts(uploadID, 10000, 1)
	if err != nil {
		return object.Object{}, err
	}
	if len(parts) == 0 {
		return object.Object{}, fmt.Errorf("multipart upload %s has no parts", uploadID)
	}

	// Combined ETag is the XXHASH64 of the concatenated part ETags, mirroring
	// how a regular object's ETag is a running hash over its chunk data.
	var totalSize int64
	digest := xxhash.New()
	partChunks := make([][]chunker.Chunk, len(parts))
	for i, part := range parts {
		totalSize += part.Size

		var etagBytes [8]byte
		binary.BigEndian.PutUint64(etagBytes[:], part.ETag)
		digest.Write(etagBytes[:])

		chunks, err := getPartChunks(database, uploadID, part.PartNumber)
		if err != nil {
			return object.Object{}, err
		}
		partChunks[i] = chunks
	}
	combinedETag := digest.Sum64()

	tx, err := database.Begin()
	if err != nil {
		return object.Object{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO buckets (name, created_at, updated_at) VALUES (?, ?, ?)`,
		string(upload.Location.Bucket.Name), now, now,
	); err != nil {
		return object.Object{}, fmt.Errorf("ensure bucket: %w", err)
	}

	// Insert the object row before reassigning chunks to it: chunks.object_key
	// has a foreign key to objects(bucket, object_key).
	if _, err := tx.Exec(`
		INSERT INTO objects (bucket, object_key, ETag, size, compression_type, compression_level, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(bucket, object_key) DO UPDATE SET
			ETag = excluded.ETag,
			size = excluded.size,
			compression_type = excluded.compression_type,
			compression_level = excluded.compression_level,
			updated_at = excluded.updated_at`,
		string(upload.Location.Bucket.Name), string(upload.Location.Key), strconv.FormatUint(combinedETag, 10), totalSize,
		int(upload.Compression.Type), upload.Compression.Level, now, now,
	); err != nil {
		return object.Object{}, fmt.Errorf("finalize object: %w", err)
	}

	// Any chunks previously stored under this object_key (from an object
	// being overwritten by this completion) are replaced by the part chunks.
	if _, err := tx.Exec(`DELETE FROM chunks WHERE bucket = ? AND object_key = ?`,
		string(upload.Location.Bucket.Name), string(upload.Location.Key),
	); err != nil {
		return object.Object{}, fmt.Errorf("clear old object chunks: %w", err)
	}

	ordinal := 0
	for _, chunks := range partChunks {
		for _, c := range chunks {
			if _, err := tx.Exec(`
				UPDATE chunks SET object_key = ?, upload_id = NULL, part_number = NULL, ordinal = ?
				WHERE id = ?`,
				string(upload.Location.Key), ordinal, c.Id.String(),
			); err != nil {
				return object.Object{}, fmt.Errorf("reassign chunk %s: %w", c.Id, err)
			}
			ordinal++
		}
	}

	if _, err := tx.Exec(`DELETE FROM multipart_uploads WHERE upload_id = ?`, uploadID); err != nil {
		return object.Object{}, fmt.Errorf("delete multipart upload: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return object.Object{}, fmt.Errorf("commit transaction: %w", err)
	}

	return GetObject(upload.Location)
}
