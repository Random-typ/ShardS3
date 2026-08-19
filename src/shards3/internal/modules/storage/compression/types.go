package compression

/*
*
*
*
*
*
 */

type CompressionType int

const (
	None CompressionType = iota
	Zstd
)

type Compression struct {
	Type  CompressionType
	Level int
}
