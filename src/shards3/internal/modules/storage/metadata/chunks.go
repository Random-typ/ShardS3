package metadata

import (
	"database/sql"
	"fmt"
	"shards3/internal/modules/storage/chunker"
	"shards3/internal/modules/storage/encryption"
	"shards3/internal/modules/storage/object"
	"shards3/internal/platform/db"

	"github.com/google/uuid"
)

type chunkRow struct {
	id                  string
	encodedShardSize    int
	encodedDataShards   int
	encodedParityShards int
	encryptionType      int
	kmsKeyID            sql.NullInt64
	size                int64
}

func scanChunkRows(rows *sql.Rows) ([]chunkRow, error) {
	var chunkRows []chunkRow
	for rows.Next() {
		var cr chunkRow
		if err := rows.Scan(&cr.id, &cr.encodedShardSize, &cr.encodedDataShards, &cr.encodedParityShards, &cr.encryptionType, &cr.kmsKeyID, &cr.size); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunkRows = append(chunkRows, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunks: %w", err)
	}
	return chunkRows, nil
}

func chunkRowsToChunks(database *db.DB, chunkRows []chunkRow) ([]chunker.Chunk, error) {
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

		var kmsKeyID encryption.KeyID
		if cr.kmsKeyID.Valid {
			kmsKeyID = encryption.KeyID(cr.kmsKeyID.Int64)
		}

		chunks = append(chunks, chunker.Chunk{
			Id:                  id,
			Size:                cr.size,
			EncodedShardSize:    cr.encodedShardSize,
			EncodedDataShards:   cr.encodedDataShards,
			EncodedParityShards: cr.encodedParityShards,
			Encryption:          chunker.Encryption{Type: encryption.EncryptionType(cr.encryptionType), KeyId: kmsKeyID},
			Shards:              shards,
		})
	}

	return chunks, nil
}

func getChunks(database *db.DB, location object.ObjectLocation) ([]chunker.Chunk, error) {
	rows, err := database.Query(`
		SELECT id, EncodingShardSize, EncodingDataShards, EncodingParityShards, encryption_type, kms_key_id, size
		FROM chunks WHERE bucket = ? AND object_key = ? ORDER BY ordinal ASC`,
		string(location.Bucket.Name), string(location.Key),
	)
	if err != nil {
		return nil, fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	chunkRows, err := scanChunkRows(rows)
	if err != nil {
		return nil, err
	}
	return chunkRowsToChunks(database, chunkRows)
}

// getPartChunks returns the ordered chunks (including shards) recorded for a
// single part of an in-progress multipart upload.
func getPartChunks(database *db.DB, uploadID string, partNumber int) ([]chunker.Chunk, error) {
	rows, err := database.Query(`
		SELECT id, EncodingShardSize, EncodingDataShards, EncodingParityShards, encryption_type, kms_key_id, size
		FROM chunks WHERE upload_id = ? AND part_number = ? ORDER BY ordinal ASC`,
		uploadID, partNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("query part chunks: %w", err)
	}
	defer rows.Close()

	chunkRows, err := scanChunkRows(rows)
	if err != nil {
		return nil, err
	}
	return chunkRowsToChunks(database, chunkRows)
}

// insertChunk inserts a single chunk row (owned by either an object or a
// multipart part, never both) along with its shards.
func insertChunk(tx *sql.Tx, bucket string, objectKey, uploadID sql.NullString, partNumber sql.NullInt64, ordinal int, c chunker.Chunk) error {
	var kmsKeyID sql.NullInt64
	if c.Encryption.Type != encryption.None {
		kmsKeyID = sql.NullInt64{Int64: int64(c.Encryption.KeyId), Valid: true}
	}

	if _, err := tx.Exec(`
		INSERT INTO chunks (id, bucket, object_key, upload_id, part_number, ordinal, EncodingShardSize, EncodingDataShards, EncodingParityShards, encryption_type, kms_key_id, size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Id.String(), bucket, objectKey, uploadID, partNumber, ordinal,
		c.EncodedShardSize, c.EncodedDataShards, c.EncodedParityShards, int(c.Encryption.Type), kmsKeyID, c.Size,
	); err != nil {
		return fmt.Errorf("insert chunk %s: %w", c.Id, err)
	}

	return insertShards(tx, c.Id.String(), c.Shards)
}

func insertChunks(tx *sql.Tx, location object.ObjectLocation, chunks []chunker.Chunk) error {
	objectKey := sql.NullString{String: string(location.Key), Valid: true}
	for ordinal, c := range chunks {
		if err := insertChunk(tx, string(location.Bucket.Name), objectKey, sql.NullString{}, sql.NullInt64{}, ordinal, c); err != nil {
			return err
		}
	}
	return nil
}

// insertPartChunks inserts the chunks produced for a single part of an
// in-progress multipart upload.
func insertPartChunks(tx *sql.Tx, bucket string, uploadID string, partNumber int, chunks []chunker.Chunk) error {
	uploadIDArg := sql.NullString{String: uploadID, Valid: true}
	partNumberArg := sql.NullInt64{Int64: int64(partNumber), Valid: true}
	for ordinal, c := range chunks {
		if err := insertChunk(tx, bucket, sql.NullString{}, uploadIDArg, partNumberArg, ordinal, c); err != nil {
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
