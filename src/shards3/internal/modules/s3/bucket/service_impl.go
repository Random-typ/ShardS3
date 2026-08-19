package bucket

import (
	"shards3/internal/modules/storage/metadata"
	"shards3/internal/modules/storage/object"
)

type service struct{}

func NewService() Service {
	return service{}
}

func (service) ListBuckets(prefix string, page int, max int) ([]object.Bucket, bool, error) {
	return metadata.ListBuckets(prefix, page, max)
}

func (service) CreateBucket(name string) error {
	return metadata.CreateBucket(object.Bucket{Name: name})
}

func (service) GetBucket(name string) (object.Bucket, error) {
	return metadata.GetBucket(object.Bucket{Name: name})
}

func (service) DeleteBucket(name string) error {
	return metadata.DeleteBucket(object.Bucket{Name: name})
}
