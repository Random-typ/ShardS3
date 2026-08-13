//go:build !linux && !darwin

package interfaces

import "fmt"

func newTelegramRemote(s *TelegramService) (telegramRemote, error) {
	return nil, fmt.Errorf("telegram backend: go-tdlib transport is only supported on linux/darwin builds")
}
