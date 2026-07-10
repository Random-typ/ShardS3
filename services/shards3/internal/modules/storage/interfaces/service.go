package interfaces

import (
	"fmt"
)

/*
*
*
*
*
*
 */

type BackendType int

const (
	Telegram BackendType = iota
	Discord
)

type Service interface {
	GetObject(location string) ([]byte, error)
	PutObject(data []byte) (string, error)
	DeleteObject(location string) error

	GetMaxObjectSize() int
}

func getService(backendType BackendType) (Service, error) {
	switch backendType {
	case Telegram:
		return &TelegramService{}, nil
	case Discord:
		return &DiscordService{}, nil
	default:
		return nil, fmt.Errorf("unsupported backend type")
	}
}

func GetMaxShardSize(backendType BackendType) (int, error) {
	service, err := getService(backendType)
	if err != nil {
		return 0, err
	}
	return service.GetMaxObjectSize(), nil
}

func GetShard(backendType BackendType, location string) ([]byte, error) {
	service, err := getService(backendType)
	if err != nil {
		return nil, err
	}
	return service.GetObject(location)
}

func PutShard(backendType BackendType, data []byte) (string, error) {
	service, err := getService(backendType)
	if err != nil {
		return "", err
	}
	return service.PutObject(data)
}

func DeleteShard(backendType BackendType, location string) error {
	service, err := getService(backendType)
	if err != nil {
		return err
	}
	return service.DeleteObject(location)
}
