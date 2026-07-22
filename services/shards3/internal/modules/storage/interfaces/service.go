package interfaces

import (
	"fmt"
)

var availableBackends []BackendType

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
	File
	// Used for testing purposes only. Do not use in production.
	File2
	File3
	File4
	File5
	File1Fail
	File2Fail
	File3Fail
	File4Fail
	File5Fail
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
	case File, File2, File3, File4, File5:
		return &FileService{fail: false}, nil
	case File1Fail, File2Fail, File3Fail, File4Fail, File5Fail:
		return &FileService{fail: true}, nil
	default:
		return nil, fmt.Errorf("unsupported backend type")
	}
}

func SetAvailableBackends(backends []BackendType) {
	availableBackends = backends
}

func GetAvailableBackends() []BackendType {
	return availableBackends
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
	location, err := service.PutObject(data)
	if err != nil {
		return "", err
	}
	return location, nil
}

func DeleteShard(backendType BackendType, location string) error {
	service, err := getService(backendType)
	if err != nil {
		return err
	}
	return service.DeleteObject(location)
}
