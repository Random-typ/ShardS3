package stats

type Service interface {
	GetSummary() error
	GetThroughput() error
	GetStorageUsed() error
}
