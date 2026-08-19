package interfaces

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
)

func init() {
	RegisterKind("discord", newDiscordService)
}

const discordMaxObjectSize = 25 * 1024 * 1024 // 25 MiB

// newDiscordService reads the "channel_id" setting and the "bot_token" secret
// (env SHARDS3_BACKEND_<ID>_BOT_TOKEN) for the given instance.
func newDiscordService(id BackendType, settings map[string]any, secrets SecretResolver) (Service, error) {
	channelID, _ := settings["channel_id"].(string)
	if strings.TrimSpace(channelID) == "" {
		return nil, fmt.Errorf("discord backend %q: missing required setting \"channel_id\"", id)
	}

	botToken, ok := secrets("bot_token")
	if !ok || botToken == "" {
		return nil, fmt.Errorf("discord backend %q: missing required secret \"bot_token\"", id)
	}

	return &DiscordService{channelID: channelID, botToken: botToken}, nil
}

type DiscordService struct {
	channelID string
	botToken  string

	mu      sync.Mutex
	session *discordgo.Session
	objects map[string][]byte
}

func (s *DiscordService) GetMaxObjectSize() int {
	return discordMaxObjectSize
}

func (s *DiscordService) GetObject(location string) ([]byte, error) {
	if !s.isConfigured() {
		return s.getObjectInMemory(location)
	}

	session, err := s.getSession()
	if err != nil {
		return nil, err
	}

	message, err := session.ChannelMessage(s.channelID, location)
	if err != nil {
		return nil, fmt.Errorf("discord backend: get message %s: %w", location, err)
	}
	if len(message.Attachments) == 0 {
		return nil, fmt.Errorf("discord backend: message %s has no attachment", location)
	}

	attachmentURL := message.Attachments[0].URL
	if attachmentURL == "" {
		attachmentURL = message.Attachments[0].ProxyURL
	}
	if attachmentURL == "" {
		return nil, fmt.Errorf("discord backend: attachment URL is empty for message %s", location)
	}

	body, err := session.RequestWithBucketID("GET", attachmentURL, nil, attachmentURL)
	if err != nil {
		return nil, fmt.Errorf("discord backend: download attachment for message %s: %w", location, err)
	}

	return body, nil
}

func (s *DiscordService) PutObject(data []byte) (string, error) {
	if len(data) > s.GetMaxObjectSize() {
		return "", fmt.Errorf("discord backend: object too large (%d bytes > %d bytes)", len(data), s.GetMaxObjectSize())
	}

	if !s.isConfigured() {
		return s.putObjectInMemory(data)
	}

	session, err := s.getSession()
	if err != nil {
		return "", err
	}

	message, err := session.ChannelMessageSendComplex(s.channelID, &discordgo.MessageSend{
		Files: []*discordgo.File{{
			Name:   "shard.bin",
			Reader: bytes.NewReader(data),
		}},
	})
	if err != nil {
		return "", fmt.Errorf("discord backend: upload shard: %w", err)
	}

	return message.ID, nil
}

func (s *DiscordService) DeleteObject(location string) error {
	if !s.isConfigured() {
		return s.deleteObjectInMemory(location)
	}

	session, err := s.getSession()
	if err != nil {
		return err
	}

	if err := session.ChannelMessageDelete(s.channelID, location); err != nil {
		return fmt.Errorf("discord backend: delete message %s: %w", location, err)
	}

	return nil
}

func (s *DiscordService) isConfigured() bool {
	return strings.TrimSpace(s.channelID) != "" && strings.TrimSpace(s.botToken) != ""
}

func (s *DiscordService) getSession() (*discordgo.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		return s.session, nil
	}

	token := strings.TrimSpace(s.botToken)
	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}

	session, err := discordgo.New(token)
	if err != nil {
		return nil, fmt.Errorf("discord backend: create session: %w", err)
	}

	s.session = session
	return s.session, nil
}

func (s *DiscordService) ensureMemoryStoreLocked() {
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
}

func (s *DiscordService) getObjectInMemory(location string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMemoryStoreLocked()
	data, ok := s.objects[location]
	if !ok {
		return nil, fmt.Errorf("discord backend: object %q not found", location)
	}

	cloned := make([]byte, len(data))
	copy(cloned, data)
	return cloned, nil
}

func (s *DiscordService) putObjectInMemory(data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMemoryStoreLocked()
	location := "discord-" + uuid.New().String()
	cloned := make([]byte, len(data))
	copy(cloned, data)
	s.objects[location] = cloned
	return location, nil
}

func (s *DiscordService) deleteObjectInMemory(location string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMemoryStoreLocked()
	if _, ok := s.objects[location]; !ok {
		return fmt.Errorf("discord backend: object %q not found", location)
	}
	delete(s.objects, location)
	return nil
}
