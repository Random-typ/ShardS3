package metadata

import "shards3/services/shards3/internal/modules/storage/object"

/* Manages Metadata for objects
*
* Stores the map of objects → chunks → locations
*
*
*
 */

// Objects
func PutObject(object object.Object) error {
	return nil
}

func GetObject(object object.ObjectLocation) (object.Object, error) {
	return object.Object{}, nil
}

func UpdateObject(object object.Object) error {
	return nil
}

func DeleteObject(object object.ObjectLocation) error {
	return nil
}

func ListObjects(object object.ObjectLocation) []object.Object {
	return nil
}

// Buckets
func CreateBucket(object object.Object) error {
	return nil
}

//func GetBucket(object object.ObjectLocation) (object.Bucket, error) {
//	return object.Object{}, nil
//}

//func UpdateBucket(object object.Object) error {
//return nil
//}

func DeleteBucket(object object.ObjectLocation) error {
	return nil
}

func ListBuckets(object object.ObjectLocation) []object.Object {
	return nil
}
