package checksum

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"

	"github.com/cespare/xxhash/v2"
	"github.com/minio/crc64nvme"
	"github.com/zeebo/xxh3"
)

type ChecksumType int

const (
	ChecksumCRC32 ChecksumType = iota
	ChecksumCRC32C
	ChecksumCRC64NVME
	ChecksumSHA1
	ChecksumSHA256
	ChecksumSHA512
	ChecksumMD5
	ChecksumMD5_AltHeader
	ChecksumXXHASH64
	ChecksumXXHASH3
	ChecksumXXHASH128
)

var checksumHeaders = map[ChecksumType]string{
	ChecksumCRC32:         "x-amz-checksum-crc32",
	ChecksumCRC32C:        "x-amz-checksum-crc32c",
	ChecksumCRC64NVME:     "x-amz-checksum-crc64nvme",
	ChecksumSHA1:          "x-amz-checksum-sha1",
	ChecksumSHA256:        "x-amz-checksum-sha256",
	ChecksumSHA512:        "x-amz-checksum-sha512",
	ChecksumMD5:           "x-amz-checksum-md5",
	ChecksumMD5_AltHeader: "Content-MD5",
	ChecksumXXHASH64:      "x-amz-checksum-xxhash64",
	ChecksumXXHASH3:       "x-amz-checksum-xxhash3",
	ChecksumXXHASH128:     "x-amz-checksum-xxhash128",
}

type Checksum struct {
	Type ChecksumType
	// Base64 encoded
	Value []byte
}

func GetChecksums(w http.ResponseWriter, r *http.Request) ([]Checksum, error) {
	var checksums []Checksum
	for ctype, header := range checksumHeaders {
		if value := r.Header.Get(header); value != "" {
			checksums = append(checksums, Checksum{
				Type:  ctype,
				Value: []byte(value),
			})
		}
	}

	return checksums, nil
}

func VerifyChecksums(w http.ResponseWriter, r *http.Request, reader io.Reader) ([]Checksum, error) {
	checksums, err := GetChecksums(w, r)
	if err != nil {
		return nil, err
	}

	type hashWriter struct {
		Hash any
		Type ChecksumType
	}

	hashWriters := make(map[ChecksumType]hashWriter)
	for _, checksum := range checksums {
		var hash any
		switch checksum.Type {
		case ChecksumCRC32:
			hash = crc32.New(crc32.IEEETable)
		case ChecksumCRC32C:
			hash = crc32.New(crc32.MakeTable(crc32.Castagnoli))
		case ChecksumCRC64NVME:
			hash = crc64nvme.New()
		case ChecksumSHA1:
			hash = sha1.New()
		case ChecksumSHA256:
			hash = sha256.New()
		case ChecksumSHA512:
			hash = sha512.New()
		case ChecksumMD5:
			hash = md5.New()
		case ChecksumMD5_AltHeader:
			hash = md5.New()
		case ChecksumXXHASH64:
			hash = xxhash.New()
		case ChecksumXXHASH3:
			hash = xxh3.New()
		case ChecksumXXHASH128:
			hash = xxh3.New128()
		default:
			continue
		}
		hashWriters[checksum.Type] = hashWriter{
			Hash: hash,
			Type: checksum.Type,
		}
	}

	var writers []io.Writer
	for _, hw := range hashWriters {
		writers = append(writers, hw.Hash.(io.Writer))
	}
	writer := io.MultiWriter(writers...)

	if _, err := io.Copy(writer, reader); err != nil {
		return nil, err
	}

	for _, checksum := range checksums {
		if hw, ok := hashWriters[checksum.Type]; ok {
			result := hw.Hash.(interface{ Sum([]byte) []byte }).Sum(nil)
			encoded := make([]byte, base64.StdEncoding.EncodedLen(len(result)))
			base64.StdEncoding.Encode(encoded, result)
			if !equal(encoded, checksum.Value) {
				return nil, fmt.Errorf("checksum mismatch for type %v", checksum.Type)
			}
		} else {
			return nil, fmt.Errorf("no hash writer found for checksum type %v", checksum.Type)
		}
	}

	return checksums, nil
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func AddChecksumHeaders(w http.ResponseWriter, r *http.Request) error {
	checksums, err := GetChecksums(w, r)
	if err != nil {
		return err
	}
	for _, checksum := range checksums {
		if checksum.Type == ChecksumMD5_AltHeader {
			continue // Content-MD5 header is not a response header
		}
		if header, ok := checksumHeaders[checksum.Type]; ok {
			w.Header().Set(header, string(checksum.Value))
		}
	}
	return nil
}
