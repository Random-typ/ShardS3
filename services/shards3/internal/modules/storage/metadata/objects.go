package metadata

import (
	"database/sql"
	"errors"
	"fmt"
	"shards3/services/shards3/internal/modules/storage/compression"
	"shards3/services/shards3/internal/modules/storage/object"
	"strconv"
	"time"
)

func UpdateObject(obj object.Object) error {
	return upsertObject(obj)
}

func DeleteObject(location object.ObjectLocation) error {
	database, err := getDB()
	if err != nil {
		return err
	}

	res, err := database.Exec(`DELETE FROM objects WHERE bucket = ? AND object_key = ?`,
		string(location.Bucket.Name), string(location.Key))
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("object not found: %s/%s", location.Bucket.Name, location.Key)
	}
	return nil
}

func ListObjects(bucket object.Bucket, prefix string, delim string, startAfter string, page int, max int) ([]object.Object, bool, error) {
	database, err := getDB()
	if err != nil {
		return nil, false, err
	}

	rows, err := database.Query(`
		SELECT object_key, size, compression_type, compression_level, created_at
		FROM objects WHERE bucket = ? AND object_key LIKE ? AND object_key > ? ORDER BY object_key ASC LIMIT ? OFFSET ?`,
		string(bucket.Name), string(prefix)+"%", startAfter, max+1, (page-1)*max,
	)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var objects []object.Object
	for rows.Next() {
		var key string
		var size int64
		var compType int
		var compLevel int
		var created time.Time
		if err := rows.Scan(&key, &size, &compType, &compLevel, &created); err != nil {
			return nil, false, err
		}

		objects = append(objects, object.Object{
			Location:     object.ObjectLocation{Bucket: bucket, Key: key},
			Size:         size,
			Compression:  compression.Compression{Type: compression.CompressionType(compType), Level: compLevel},
			LastModified: created,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasMore := len(objects) == max+1
	if len(objects) > 0 && hasMore {
		objects = objects[:len(objects)-1] // remove the extra item used to check for more
	}
	return objects, hasMore, nil
}

// upsertObject inserts or fully replaces the object row, along with its
// chunks and shards, inside a single transaction.
func upsertObject(obj object.Object) error {
	database, err := getDB()
	if err != nil {
		return err
	}

	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO buckets (name, created_at, updated_at) VALUES (?, ?, ?)`,
		string(obj.Location.Bucket.Name), now, now,
	); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO objects (bucket, object_key, ETag, size, compression_type, compression_level, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(bucket, object_key) DO UPDATE SET
			ETag = excluded.ETag,
			size = excluded.size,
			compression_type = excluded.compression_type,
			compression_level = excluded.compression_level,
			updated_at = excluded.updated_at`,
		obj.Location.Bucket.Name, obj.Location.Key, strconv.FormatUint(obj.ETag, 10), obj.Size,
		int(obj.Compression.Type), obj.Compression.Level, now, now,
	); err != nil {
		return fmt.Errorf("upsert object: %w", err)
	}

	// Replace any previously stored chunks/shards for this object.
	if _, err := tx.Exec(`DELETE FROM chunks WHERE bucket = ? AND object_key = ?`,
		string(obj.Location.Bucket.Name), string(obj.Location.Key),
	); err != nil {
		return fmt.Errorf("clear old chunks: %w", err)
	}

	if err := insertChunks(tx, obj.Location, obj.Chunks); err != nil {
		return err
	}

	return tx.Commit()
}

func PutObject(obj object.Object) error {
	return upsertObject(obj)
}

func GetObject(location object.ObjectLocation) (object.Object, error) {
	database, err := getDB()
	if err != nil {
		return object.Object{}, err
	}

	var eTag string
	var size int64
	var compType int
	var compLevel int
	var created time.Time
	err = database.QueryRow(`
		SELECT ETag, size, compression_type, compression_level, created_at
		FROM objects WHERE bucket = ? AND object_key = ?`,
		string(location.Bucket.Name), string(location.Key),
	).Scan(&eTag, &size, &compType, &compLevel, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return object.Object{}, fmt.Errorf("object not found: %s/%s", location.Bucket.Name, location.Key)
	}
	if err != nil {
		return object.Object{}, fmt.Errorf("get object: %w", err)
	}

	chunks, err := getChunks(database, location)
	if err != nil {
		return object.Object{}, err
	}

	eTagUint, err := strconv.ParseUint(eTag, 10, 64)
	if err != nil {
		return object.Object{}, fmt.Errorf("parse ETag: %w", err)
	}

	return object.Object{
		Location:     location,
		ETag:         eTagUint,
		Size:         size,
		Compression:  compression.Compression{Type: compression.CompressionType(compType), Level: compLevel},
		LastModified: created,
		Chunks:       chunks,
	}, nil
}
