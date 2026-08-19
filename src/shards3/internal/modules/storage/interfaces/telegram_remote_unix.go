//go:build linux || darwin

package interfaces

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tdlib "github.com/zelenin/go-tdlib/client"
)

type telegramTDLibRemote struct {
	chatID int64
	client *tdlib.Client
}

func newTelegramRemote(s *TelegramService) (telegramRemote, error) {
	workDir := strings.TrimSpace(s.tdlibWorkDir)
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), fmt.Sprintf("shards3-tdlib-%d", s.chatID))
	}

	databaseDir := filepath.Join(workDir, "database")
	filesDir := filepath.Join(workDir, "files")
	if err := os.MkdirAll(databaseDir, 0o755); err != nil {
		return nil, fmt.Errorf("telegram backend: create tdlib database directory: %w", err)
	}
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return nil, fmt.Errorf("telegram backend: create tdlib files directory: %w", err)
	}

	authorizer := tdlib.BotAuthorizer(&tdlib.SetTdlibParametersRequest{
		UseTestDc:           false,
		DatabaseDirectory:   databaseDir,
		FilesDirectory:      filesDir,
		UseFileDatabase:     true,
		UseChatInfoDatabase: true,
		UseMessageDatabase:  true,
		UseSecretChats:      false,
		ApiId:               s.apiID,
		ApiHash:             s.apiHash,
		SystemLanguageCode:  "en",
		DeviceModel:         "ShardS3",
		SystemVersion:       "1.0.0",
		ApplicationVersion:  "1.0.0",
	}, s.botToken)

	client, err := tdlib.NewClient(authorizer, tdlib.WithLogVerbosity(&tdlib.SetLogVerbosityLevelRequest{NewVerbosityLevel: 0}))
	if err != nil {
		return nil, fmt.Errorf("telegram backend: initialize tdlib client: %w", err)
	}

	if _, err := client.OpenChat(&tdlib.OpenChatRequest{ChatId: s.chatID}); err != nil {
		return nil, fmt.Errorf("telegram backend: open chat %d: %w", s.chatID, err)
	}

	return &telegramTDLibRemote{chatID: s.chatID, client: client}, nil
}

func (r *telegramTDLibRemote) GetObject(location string) ([]byte, error) {
	messageID, err := parseTelegramMessageID(location)
	if err != nil {
		return nil, err
	}

	message, err := r.client.GetMessage(&tdlib.GetMessageRequest{
		ChatId:    r.chatID,
		MessageId: messageID,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram backend: get message %s: %w", location, err)
	}

	content, ok := message.Content.(*tdlib.MessageDocument)
	if !ok || content.Document == nil || content.Document.Document == nil {
		return nil, fmt.Errorf("telegram backend: message %s does not contain a document", location)
	}

	file, err := r.client.DownloadFile(&tdlib.DownloadFileRequest{
		FileId:      content.Document.Document.Id,
		Priority:    32,
		Offset:      0,
		Limit:       0,
		Synchronous: true,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram backend: download file for message %s: %w", location, err)
	}

	if file.Local == nil || !file.Local.IsDownloadingCompleted || file.Local.Path == "" {
		return nil, fmt.Errorf("telegram backend: file for message %s is not fully downloaded", location)
	}

	data, err := os.ReadFile(file.Local.Path)
	if err != nil {
		return nil, fmt.Errorf("telegram backend: read downloaded file for message %s: %w", location, err)
	}
	return data, nil
}

func (r *telegramTDLibRemote) PutObject(data []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "shards3-telegram-shard-*.bin")
	if err != nil {
		return "", fmt.Errorf("telegram backend: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("telegram backend: write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("telegram backend: close temp file: %w", err)
	}

	message, err := r.client.SendMessage(&tdlib.SendMessageRequest{
		ChatId: r.chatID,
		InputMessageContent: &tdlib.InputMessageDocument{
			Document:                    &tdlib.InputFileLocal{Path: tmpPath},
			DisableContentTypeDetection: true,
		},
	})
	if err != nil {
		return "", fmt.Errorf("telegram backend: send document: %w", err)
	}

	return strconv.FormatInt(message.Id, 10), nil
}

func (r *telegramTDLibRemote) DeleteObject(location string) error {
	messageID, err := parseTelegramMessageID(location)
	if err != nil {
		return err
	}

	_, err = r.client.DeleteMessages(&tdlib.DeleteMessagesRequest{
		ChatId:     r.chatID,
		MessageIds: []int64{messageID},
		Revoke:     true,
	})
	if err != nil {
		return fmt.Errorf("telegram backend: delete message %s: %w", location, err)
	}
	return nil
}

func parseTelegramMessageID(location string) (int64, error) {
	trimmed := strings.TrimSpace(location)
	if trimmed == "" {
		return 0, fmt.Errorf("telegram backend: location must not be empty")
	}

	messageID, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telegram backend: invalid location %q: %w", location, err)
	}
	if messageID == 0 {
		return 0, fmt.Errorf("telegram backend: invalid location %q", location)
	}
	return messageID, nil
}
