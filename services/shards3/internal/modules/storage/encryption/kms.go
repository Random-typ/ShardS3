package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"shards3/services/shards3/internal/platform/config"
	"shards3/services/shards3/internal/platform/db"
)

const (
	defaultKMSFilePath        = "kms.enc.json"
	defaultKMSPasswordKeyPath = "kms.key"
	generatedKeySize          = 32
)

type kmsStorage struct {
	Version int        `json:"version"`
	NextID  KeyID      `json:"nextId"`
	Keys    []kmsEntry `json:"keys"`
}

type kmsEntry struct {
	ID      KeyID  `json:"id"`
	KeyB64  string `json:"keyB64"`
	Deleted bool   `json:"deleted,omitempty"`
}

type encryptedPayload struct {
	SaltB64       string `json:"saltB64"`
	NonceB64      string `json:"nonceB64"`
	CiphertextB64 string `json:"ciphertextB64"`
}

type KMS struct {
	db          *db.DB
	passwordKey string
}

var (
	defaultKMSMu sync.RWMutex
	defaultKMS   *KMS
)

func ConfigureKMS(database *db.DB) error {
	kms, err := NewKMS(database)
	if err != nil {
		return err
	}

	defaultKMSMu.Lock()
	defer defaultKMSMu.Unlock()
	defaultKMS = kms
	return nil
}

func CreateKey() (KeyID, error) {
	kms, err := getDefaultKMS()
	if err != nil {
		return 0, err
	}
	return kms.CreateKey()
}

func ListKeys() ([]KeyID, error) {
	kms, err := getDefaultKMS()
	if err != nil {
		return nil, err
	}
	return kms.ListKeys(), nil
}

func DeleteKey(keyID KeyID) error {
	kms, err := getDefaultKMS()
	if err != nil {
		return err
	}
	return kms.DeleteKey(keyID)
}

func NewKMS(database *db.DB) (*KMS, error) {
	passwordKeyPath := strings.TrimSpace(config.Cfg.KMSPasswordKeyPath)
	if passwordKeyPath == "" {
		passwordKeyPath = defaultKMSPasswordKeyPath
	}

	kms := &KMS{
		db:          database,
		passwordKey: passwordKeyPath,
	}

	if err := kms.ensurePasswordFile(); err != nil {
		return nil, err
	}

	return kms, nil
}

func (k *KMS) CreateKey() (KeyID, error) {
	keyData := make([]byte, generatedKeySize)
	if _, err := rand.Read(keyData); err != nil {
		return 0, fmt.Errorf("generate key: %w", err)
	}

	password, err := k.readPassword()
	if err != nil {
		return 0, err
	}

	encBytes, err := encryptJSON(keyData, password)
	if err != nil {
		return 0, err
	}

	var id int64
	err = k.db.QueryRow(`
		INSERT INTO kms_keys (key_ciphertext, created_at)
		VALUES (?, ?) RETURNING id`,
		encBytes, time.Now().UTC(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert key: %w", err)
	}

	return KeyID(id), nil
}

func (k *KMS) GetKey(keyID KeyID) ([]byte, error) {
	var encBytes []byte
	err := k.db.QueryRow(`
		SELECT key_ciphertext FROM kms_keys 
		WHERE id = ? AND deleted_at IS NULL`,
		keyID,
	).Scan(&encBytes)
	if err != nil {
		return nil, fmt.Errorf("key not found for keyId %d: %w", keyID, err)
	}

	password, err := k.readPassword()
	if err != nil {
		return nil, err
	}

	plain, err := decryptJSON(encBytes, password)
	if err != nil {
		return nil, err
	}

	return plain, nil
}

func (k *KMS) ListKeys() []KeyID {
	rows, err := k.db.Query(`SELECT id FROM kms_keys WHERE deleted_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []KeyID
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, KeyID(id))
		}
	}
	return ids
}

func (k *KMS) DeleteKey(keyID KeyID) error {
	res, err := k.db.Exec(`UPDATE kms_keys SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, time.Now().UTC(), keyID)
	if err != nil {
		return fmt.Errorf("delete key: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return fmt.Errorf("key not found for keyId %d", keyID)
	}

	return nil
}

func getDefaultKMS() (*KMS, error) {
	defaultKMSMu.RLock()
	if defaultKMS != nil {
		kms := defaultKMS
		defaultKMSMu.RUnlock()
		return kms, nil
	}
	defaultKMSMu.RUnlock()

	defaultKMSMu.Lock()
	defer defaultKMSMu.Unlock()

	if defaultKMS != nil {
		return defaultKMS, nil
	}

	return nil, errors.New("KMS not initialized. Call ConfigureKMS first.")
}

func (k *KMS) ensurePasswordFile() error {
	_, err := os.Stat(k.passwordKey)
	if errors.Is(err, os.ErrNotExist) {
		password := make([]byte, 32)
		if _, readErr := rand.Read(password); readErr != nil {
			return fmt.Errorf("generate kms password: %w", readErr)
		}

		encoded := base64.StdEncoding.EncodeToString(password)
		if dirErr := os.MkdirAll(filepath.Dir(k.passwordKey), 0o755); dirErr != nil && filepath.Dir(k.passwordKey) != "." {
			return fmt.Errorf("create key directory: %w", dirErr)
		}

		if writeErr := os.WriteFile(k.passwordKey, []byte(encoded), 0o600); writeErr != nil {
			return fmt.Errorf("write key file: %w", writeErr)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("stat key file: %w", err)
	}

	return nil
}

func (k *KMS) readPassword() ([]byte, error) {
	passwordRaw, err := os.ReadFile(k.passwordKey)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	passwordStr := strings.TrimSpace(string(passwordRaw))
	if passwordStr == "" {
		return nil, errors.New("key file is empty")
	}

	decoded, decodeErr := base64.StdEncoding.DecodeString(passwordStr)
	if decodeErr != nil {
		return nil, fmt.Errorf("decode key file: %w", decodeErr)
	}

	return decoded, nil
}

func encryptJSON(plain []byte, password []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plain, nil)

	payload := encryptedPayload{
		SaltB64:       base64.StdEncoding.EncodeToString(salt),
		NonceB64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
	}

	enc, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal encrypted payload: %w", err)
	}

	return enc, nil
}

func decryptJSON(enc []byte, password []byte) ([]byte, error) {
	var payload encryptedPayload
	if err := json.Unmarshal(enc, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal encrypted payload: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(payload.SaltB64)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(payload.NonceB64)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(payload.CiphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt kms file: %w", err)
	}

	return plain, nil
}

func deriveKey(password []byte, salt []byte) []byte {
	hash := sha256.Sum256(append(salt, password...))
	return hash[:]
}
