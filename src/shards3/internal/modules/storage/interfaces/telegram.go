package interfaces

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

func init() {
	RegisterKind("telegram", newTelegramService)
}

const telegramMaxObjectSize = 2 * 1024 * 1024 * 1024 // 2 GiB

// newTelegramService reads the "chat_id" and optional "tdlib_workdir"
// settings plus required secrets "bot_token", "api_id", and "api_hash"
// (env SHARDS3_BACKEND_<ID>_BOT_TOKEN/API_ID/API_HASH) for the given
// instance.
func newTelegramService(id BackendType, settings map[string]any, secrets SecretResolver) (Service, error) {
	chatID, err := readInt64Setting(settings, "chat_id")
	if err != nil {
		return nil, fmt.Errorf("telegram backend %q: %w", id, err)
	}

	botToken, ok := secrets("bot_token")
	if !ok || botToken == "" {
		return nil, fmt.Errorf("telegram backend %q: missing required secret \"bot_token\"", id)
	}

	apiID, err := readInt32SettingOrSecret(settings, secrets, "api_id")
	if err != nil {
		return nil, fmt.Errorf("telegram backend %q: %w", id, err)
	}

	apiHash, err := readStringSettingOrSecret(settings, secrets, "api_hash")
	if err != nil {
		return nil, fmt.Errorf("telegram backend %q: %w", id, err)
	}

	tdlibWorkDir, _ := settings["tdlib_workdir"].(string)

	return &TelegramService{
		chatID:       chatID,
		botToken:     botToken,
		apiID:        apiID,
		apiHash:      apiHash,
		tdlibWorkDir: tdlibWorkDir,
	}, nil
}

type TelegramService struct {
	chatID       int64
	botToken     string
	apiID        int32
	apiHash      string
	tdlibWorkDir string

	mu      sync.Mutex
	remote  telegramRemote
	objects map[string][]byte
}

func (s *TelegramService) GetMaxObjectSize() int {
	return telegramMaxObjectSize
}

func (s *TelegramService) GetObject(location string) ([]byte, error) {
	if !s.isConfigured() {
		return s.getObjectInMemory(location)
	}

	remote, err := s.getRemote()
	if err != nil {
		return nil, err
	}

	return remote.GetObject(location)
}

func (s *TelegramService) PutObject(data []byte) (string, error) {
	if len(data) > s.GetMaxObjectSize() {
		return "", fmt.Errorf("telegram backend: object too large (%d bytes > %d bytes)", len(data), s.GetMaxObjectSize())
	}

	if !s.isConfigured() {
		return s.putObjectInMemory(data)
	}

	remote, err := s.getRemote()
	if err != nil {
		return "", err
	}

	return remote.PutObject(data)
}

func (s *TelegramService) DeleteObject(location string) error {
	if !s.isConfigured() {
		return s.deleteObjectInMemory(location)
	}

	remote, err := s.getRemote()
	if err != nil {
		return err
	}

	return remote.DeleteObject(location)
}

func (s *TelegramService) isConfigured() bool {
	return s.chatID != 0 && s.botToken != "" && s.apiID != 0 && s.apiHash != ""
}

func (s *TelegramService) getRemote() (telegramRemote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.remote != nil {
		return s.remote, nil
	}

	remote, err := newTelegramRemote(s)
	if err != nil {
		return nil, err
	}

	s.remote = remote
	return s.remote, nil
}

func (s *TelegramService) ensureMemoryStoreLocked() {
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
}

func (s *TelegramService) getObjectInMemory(location string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMemoryStoreLocked()
	data, ok := s.objects[location]
	if !ok {
		return nil, fmt.Errorf("telegram backend: object %q not found", location)
	}

	cloned := make([]byte, len(data))
	copy(cloned, data)
	return cloned, nil
}

func (s *TelegramService) putObjectInMemory(data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMemoryStoreLocked()
	location := "telegram-" + uuid.New().String()
	cloned := make([]byte, len(data))
	copy(cloned, data)
	s.objects[location] = cloned
	return location, nil
}

func (s *TelegramService) deleteObjectInMemory(location string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMemoryStoreLocked()
	if _, ok := s.objects[location]; !ok {
		return fmt.Errorf("telegram backend: object %q not found", location)
	}
	delete(s.objects, location)
	return nil
}

type telegramRemote interface {
	GetObject(location string) ([]byte, error)
	PutObject(data []byte) (string, error)
	DeleteObject(location string) error
}

func readInt64Setting(settings map[string]any, key string) (int64, error) {
	raw, ok := settings[key]
	if !ok {
		return 0, fmt.Errorf("missing required setting %q", key)
	}

	switch v := raw.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, fmt.Errorf("invalid setting %q: value out of range", key)
		}
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("setting %q must not be empty", key)
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid setting %q: %w", key, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid setting %q type %T", key, raw)
	}
}

func readInt32SettingOrSecret(settings map[string]any, secrets SecretResolver, key string) (int32, error) {
	if raw, ok := settings[key]; ok {
		parsed, err := parseInt32Value(key, raw)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	}

	v, ok := secrets(key)
	if !ok || strings.TrimSpace(v) == "" {
		return 0, fmt.Errorf("missing required setting or secret %q", key)
	}

	parsed64, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid secret %q: %w", key, err)
	}

	return int32(parsed64), nil
}

func parseInt32Value(key string, raw any) (int32, error) {
	switch v := raw.(type) {
	case int:
		return int32(v), nil
	case int8:
		return int32(v), nil
	case int16:
		return int32(v), nil
	case int32:
		return v, nil
	case int64:
		return int32(v), nil
	case uint:
		return int32(v), nil
	case uint8:
		return int32(v), nil
	case uint16:
		return int32(v), nil
	case uint32:
		return int32(v), nil
	case uint64:
		return int32(v), nil
	case float64:
		return int32(v), nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, fmt.Errorf("setting %q must not be empty", key)
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid setting %q: %w", key, err)
		}
		return int32(parsed), nil
	default:
		return 0, fmt.Errorf("invalid setting %q type %T", key, raw)
	}
}

func readStringSettingOrSecret(settings map[string]any, secrets SecretResolver, key string) (string, error) {
	if raw, ok := settings[key]; ok {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), nil
		}
		return "", fmt.Errorf("setting %q must be a non-empty string", key)
	}

	v, ok := secrets(key)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("missing required setting or secret %q", key)
	}

	return strings.TrimSpace(v), nil
}
