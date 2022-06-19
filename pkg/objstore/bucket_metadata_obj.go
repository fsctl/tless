package objstore

import (
	"context"
	"errors"

	"github.com/fsctl/tless/pkg/util"
)

//
// WIP - to replace CryptoConfigCheck(Daemon) with general metadata file that contains bucket version, encrypted keys and KDF salt
//

var (
	ErrCantConnect       = errors.New("cannot connect to cloud provider")
	ErrCorruptedMetadata = errors.New("the bucket metadata file is corrupt")
)

// type bucketMetadata struct {
// }
//
// func (objst *ObjStore) isBucketEmpty(ctx context.Context, bucket string) (bool, error) {
// 	return false, nil
// }
//
// func (objst *ObjStore) readBucketMetadataFile(ctx context.Context, bucket string) (bool, error) {
// 	return false, nil
// }
//
// func (objst *ObjStore) writeBucketMetadataFile(ctx context.Context, bucket string, salt string, version int) (bool, error) {
// 	return false, nil
// }

func (objst *ObjStore) GetOrCreateBucketMetadata(ctx context.Context, bucket string, vlog *util.VLog) (salt string, version int, err error) {
	return "", 0, nil
}
