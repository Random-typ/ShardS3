package head

type Service interface {
	HeadBucket(name string) error
	HeadObject(bucket string, key string) error
}
