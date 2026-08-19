package metadata

import (
	"fmt"
	"shards3/internal/modules/storage/object"
	"time"
)

func GetBucket(bucket object.Bucket) (object.Bucket, error) {
	database, err := getDB()
	if err != nil {
		return object.Bucket{}, err
	}
	err = database.QueryRow(`SELECT name, created_at FROM buckets WHERE name = ?`, string(bucket.Name)).Scan(&bucket.Name, &bucket.CreatedAt)
	if err != nil {
		return object.Bucket{}, fmt.Errorf("get bucket %s: %w", bucket.Name, err)
	}
	return bucket, nil
}

func CreateBucket(bucket object.Bucket) error {
	database, err := getDB()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err := database.Exec(`
		INSERT INTO buckets (name, created_at, updated_at) VALUES (?, ?, ?)`,
		string(bucket.Name), now, now,
	); err != nil {
		return fmt.Errorf("create bucket %s: %w", bucket.Name, err)
	}
	return nil
}

func ListBucketsWithStats() ([]object.BucketStats, error) {
	database, err := getDB()
	if err != nil {
		return nil, err
	}

	rows, err := database.Query(`
		SELECT b.name, b.created_at, COALESCE(COUNT(o.object_key), 0), COALESCE(SUM(o.size), 0)
		FROM buckets b
		LEFT JOIN objects o ON o.bucket = b.name
		GROUP BY b.name
		ORDER BY b.name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query bucket stats: %w", err)
	}
	defer rows.Close()

	var stats []object.BucketStats
	for rows.Next() {
		var s object.BucketStats
		var bucket object.Bucket
		if err := rows.Scan(&bucket.Name, &bucket.CreatedAt, &s.TotalObjects, &s.TotalSize); err != nil {
			return nil, fmt.Errorf("scan bucket stats: %w", err)
		}
		s.Bucket = bucket
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bucket stats: %w", err)
	}

	return stats, nil
}

func DeleteBucket(bucket object.Bucket) error {
	database, err := getDB()
	if err != nil {
		return err
	}

	res, err := database.Exec(`DELETE FROM buckets WHERE name = ?`, string(bucket.Name))
	if err != nil {
		return fmt.Errorf("delete bucket %s: %w", bucket.Name, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bucket %s: %w", bucket.Name, err)
	}
	if affected == 0 {
		return fmt.Errorf("bucket not found: %s", bucket.Name)
	}
	return nil
}

func BucketCount() (int, error) {
	database, err := getDB()
	if err != nil {
		return 0, err
	}

	var count int
	err = database.QueryRow(`SELECT COUNT(*) FROM buckets`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count buckets: %w", err)
	}
	return count, nil
}

func ListBuckets(prefix string, page int, max int) ([]object.Bucket, bool, error) {
	database, err := getDB()
	if err != nil {
		return nil, false, fmt.Errorf("get DB: %w", err)
	}
	// we request one extra bucket to determine if there are more buckets available after the current page. It is removed later if it exists.
	rows, err := database.Query(`SELECT name, created_at FROM buckets WHERE name LIKE ? ORDER BY name ASC LIMIT ? OFFSET ?`, prefix+"%", max+1, (page-1)*max)
	if err != nil {
		return nil, false, fmt.Errorf("query buckets: %w", err)
	}
	defer rows.Close()

	var buckets []object.Bucket
	for rows.Next() {
		var name string
		var createdAt time.Time
		if err := rows.Scan(&name, &createdAt); err != nil {
			return nil, false, fmt.Errorf("scan bucket: %w", err)
		}
		buckets = append(buckets, object.Bucket{Name: name, CreatedAt: createdAt})
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate buckets: %w", err)
	}

	hasMore := len(buckets) == max+1
	if len(buckets) > 0 && hasMore {
		buckets = buckets[:len(buckets)-1] // remove the extra item used to check for more
	}
	return buckets, hasMore, nil
}
