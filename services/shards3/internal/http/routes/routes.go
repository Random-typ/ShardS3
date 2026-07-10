package routes

const (
	BucketCollection = "/buckets"
	BucketItem       = "/buckets/{bucket}"

	ObjectCollection = "/buckets/{bucket}/objects"
	ObjectItem       = "/buckets/{bucket}/objects/{key}"

	ListObjectsV2 = "/buckets/{bucket}/objects:list"

	MultipartCreate = "/buckets/{bucket}/multipart"
	MultipartPart   = "/buckets/{bucket}/multipart/{uploadId}/parts/{partNumber}"
	MultipartCommit = "/buckets/{bucket}/multipart/{uploadId}:complete"
	MultipartAbort  = "/buckets/{bucket}/multipart/{uploadId}:abort"

	HeadBucket = "/buckets/{bucket}:head"
	HeadObject = "/buckets/{bucket}/objects/{key}:head"

	SigV4Verify = "/auth/sigv4:verify"
	PresignURL  = "/presign"
)
