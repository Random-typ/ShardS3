package bucket

import "shards3/services/shards3/internal/modules/storage/object"

type Service interface {
	ListBuckets(prefix string, page int, max int) ([]object.Bucket, bool, error)
	CreateBucket(name string) error
	GetBucket(name string) (object.Bucket, error)
	//UpdateBucket(name string) error
	DeleteBucket(name string) error
}
