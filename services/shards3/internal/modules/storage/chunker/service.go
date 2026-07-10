package chunker

type Service interface {
	PutObject(bucket string, key string) error
	GetObject(bucket string, key string) error
	UpdateObject(bucket string, key string) error
	DeleteObject(bucket string, key string) error
}
