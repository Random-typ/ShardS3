package compression

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

func Compress(data []byte, compression Compression) ([]byte, error) {
	switch compression.Type {
	case None:
		return data, nil
	case Zstd:
		encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(compression.Level)))
		if err != nil {
			return nil, fmt.Errorf("create zstd encoder: %w", err)
		}
		defer encoder.Close()

		return encoder.EncodeAll(data, nil), nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %d", compression.Type)
	}
}

func Decompress(data []byte, compression Compression) ([]byte, error) {
	switch compression.Type {
	case None:
		return data, nil
	case Zstd:
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return nil, fmt.Errorf("create zstd decoder: %w", err)
		}
		defer decoder.Close()

		decoded, err := decoder.DecodeAll(data, nil)
		if err != nil {
			return nil, fmt.Errorf("decode zstd data: %w", err)
		}

		return decoded, nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %d", compression.Type)
	}
}
