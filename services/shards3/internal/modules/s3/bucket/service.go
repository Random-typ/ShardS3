package bucket

type Service interface {
	CreateBucket(name string) error
	GetBucket(name string) error
	UpdateBucket(name string) error
	DeleteBucket(name string) error
}
