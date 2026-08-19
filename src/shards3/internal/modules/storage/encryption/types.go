package encryption

/*
*
*
*
*
*
 */

type EncryptionType int
type KeyID int64

const (
	None EncryptionType = iota
	AES_256_GCM
	ChaCha20_Poly1305
)

type Encryption struct {
	Type  EncryptionType
	KeyId KeyID
}
