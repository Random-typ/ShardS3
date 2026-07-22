package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

func EncryptionTypeFromString(s string) (EncryptionType, error) {
	switch s {
	case "None":
		return None, nil
	case "AES-256-GCM":
		return AES_256_GCM, nil
	case "ChaCha20-Poly1305":
		return ChaCha20_Poly1305, nil
	default:
		return 0, fmt.Errorf("unsupported encryption type: %s", s)
	}
}

func Encrypt(data []byte, encryption EncryptionType) ([]byte, KeyID, error) {
	switch encryption {
	case None:
		return append([]byte(nil), data...), 0, nil
	case AES_256_GCM:
		return encryptAES256GCM(data)
	case ChaCha20_Poly1305:
		return encryptChaCha20Poly1305(data)
	default:
		return nil, 0, fmt.Errorf("unsupported encryption type: %d", encryption)
	}
}

func Decrypt(data []byte, encryption Encryption) ([]byte, error) {
	switch encryption.Type {
	case None:
		return append([]byte(nil), data...), nil
	case AES_256_GCM:
		return decryptAES256GCM(data, encryption.KeyId)
	case ChaCha20_Poly1305:
		return decryptChaCha20Poly1305(data, encryption.KeyId)
	default:
		return nil, fmt.Errorf("unsupported encryption type: %d", encryption)
	}
}

func getKey(keyId KeyID) ([]byte, error) {
	kms, err := getDefaultKMS()
	if err != nil {
		return nil, err
	}

	return kms.GetKey(keyId)
}

func encryptAES256GCM(data []byte) ([]byte, KeyID, error) {
	keyId, err := CreateKey()
	if err != nil {
		return nil, 0, fmt.Errorf("create key: %w", err)
	}

	key, err := getKey(keyId)
	if err != nil {
		return nil, 0, err
	}

	if len(key) != 32 {
		return nil, 0, fmt.Errorf("invalid key size for AES-256-GCM: %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, 0, fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, 0, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, 0, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)
	sealed := append(nonce, ciphertext...)

	return sealed, keyId, nil
}

func decryptAES256GCM(data []byte, keyId KeyID) ([]byte, error) {
	if keyId <= 0 {
		return nil, errors.New("keyId is required for AES-256-GCM decryption")
	}

	key, err := getKey(keyId)
	if err != nil {
		return nil, err
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key size for AES-256-GCM: %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt aes-256-gcm data: %w", err)
	}

	return plain, nil
}

func encryptChaCha20Poly1305(data []byte) ([]byte, KeyID, error) {
	keyId, err := CreateKey()
	if err != nil {
		return nil, 0, fmt.Errorf("create key: %w", err)
	}

	key, err := getKey(keyId)
	if err != nil {
		return nil, 0, err
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, 0, fmt.Errorf("create chacha20-poly1305: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, 0, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, data, nil)
	sealed := append(nonce, ciphertext...)

	return sealed, keyId, nil
}

func decryptChaCha20Poly1305(data []byte, keyId KeyID) ([]byte, error) {
	if keyId <= 0 {
		return nil, errors.New("keyId is required for ChaCha20-Poly1305 decryption")
	}

	key, err := getKey(keyId)
	if err != nil {
		return nil, err
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create chacha20-poly1305: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt chacha20-poly1305 data: %w", err)
	}

	return plain, nil
}
