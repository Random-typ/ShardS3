package interfaces

var availableBackends []BackendType

// BackendType identifies one configured backend instance (e.g. "telegram",
// "file-0"), as assigned in backends.yaml. Distinct from Kind, which
// identifies the driver implementation used to construct the instance.
type BackendType string

type Service interface {
	GetObject(location string) ([]byte, error)
	PutObject(data []byte) (string, error)
	DeleteObject(location string) error

	GetMaxObjectSize() int
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
