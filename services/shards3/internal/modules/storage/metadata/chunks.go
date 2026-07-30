package metadata

import (
	"database/sql"
	"fmt"
	"shards3/services/shards3/internal/modules/storage/chunker"
	"shards3/services/shards3/internal/modules/storage/encryption"
	"shards3/services/shards3/internal/modules/storage/object"
	"shards3/services/shards3/internal/platform/db"

	"github.com/google/uuid"
)

func getChunks(database *db.DB, location object.ObjectLocation) ([]chunker.Chunk, error) {
	rows, err := database.Query(`
		SELECT id, EncodingShardSize, EncodingDataShards, EncodingParityShards, encryption_type, key_id, size
		FROM chunks WHERE bucket = ? AND object_key = ? ORDER BY ordinal ASC`,
		string(location.Bucket.Name), string(location.Key),
	)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	type chunkRow struct {
		id                  string
		encodedShardSize    int
		encodedDataShards   int
		encodedParityShards int
		encryptionType      int
		keyID               sql.NullInt64
		size                int64
	}

	var chunkRows []chunkRow
	for rows.Next() {
		var cr chunkRow
		if err := rows.Scan(&cr.id, &cr.encodedShardSize, &cr.encodedDataShards, &cr.encodedParityShards, &cr.encryptionType, &cr.keyID, &cr.size); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunkRows = append(chunkRows, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunks: %w", err)
	}

	chunks := make([]chunker.Chunk, 0, len(chunkRows))
	for _, cr := range chunkRows {
		id, err := uuid.Parse(cr.id)
		if err != nil {
			return nil, fmt.Errorf("parse chunk id %s: %w", cr.id, err)
		}

		shards, err := getShards(database, cr.id)
		if err != nil {
			return nil, err
		}

		var keyID encryption.KeyID
		if cr.keyID.Valid {
			keyID = encryption.KeyID(cr.keyID.Int64)
		}

		chunks = append(chunks, chunker.Chunk{
			Id:                  id,
			Size:                cr.size,
			EncodedShardSize:    cr.encodedShardSize,
			EncodedDataShards:   cr.encodedDataShards,
			EncodedParityShards: cr.encodedParityShards,
			Encryption:          chunker.Encryption{Type: encryption.EncryptionType(cr.encryptionType), KeyId: keyID},
			Shards:              shards,
		})
	}

	return chunks, nil
}

func insertChunks(tx *sql.Tx, location object.ObjectLocation, chunks []chunker.Chunk) error {
	for ordinal, c := range chunks {
		var keyID sql.NullInt64
		if c.Encryption.Type != encryption.None {
			keyID = sql.NullInt64{Int64: int64(c.Encryption.KeyId), Valid: true}
		}

		if _, err := tx.Exec(`
			INSERT INTO chunks (id, bucket, object_key, ordinal, EncodingShardSize, EncodingDataShards, EncodingParityShards, encryption_type, key_id, size)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.Id.String(), string(location.Bucket.Name), string(location.Key), ordinal,
			c.EncodedShardSize, c.EncodedDataShards, c.EncodedParityShards, int(c.Encryption.Type), keyID, c.Size,
		); err != nil {
			return fmt.Errorf("insert chunk %s: %w", c.Id, err)
		}

		if err := insertShards(tx, c.Id.String(), c.Shards); err != nil {
			return err
		}
	}
	return nil
}

// GetChunks returns the ordered chunks (including their shards) recorded for
// the given object. Intended for read-only consumers such as stats/dashboard
// APIs that don't need the full Object.
func GetChunks(location object.ObjectLocation) ([]chunker.Chunk, error) {
	database, err := getDB()
	if err != nil {
		return nil, err
	}
	return getChunks(database, location)
}
