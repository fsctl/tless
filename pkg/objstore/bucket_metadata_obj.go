package objstore

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/util"
)

const (
	MetadataObjName string = "metadata"
)

var (
	ErrCantConnect           = errors.New("cannot connect to cloud provider")
	ErrNoMetadataButNotEmpty = errors.New("the bucket does not contain a metadata file but is also not empty")
)

type BucketMetadata struct {
	Salt    string
	Version int
}

func (objst *ObjStore) isBucketEmpty(ctx context.Context, bucket string, vlog *util.VLog) (bool, error) {
	mTopLevelObjs, err := objst.GetObjList(ctx, bucket, "", false, vlog)
	if err != nil {
		log.Println("error: isBucketEmpty: cannot get bucket object list: ", err)
		return false, err
	}
	if len(mTopLevelObjs) == 0 {
		return true, nil
	} else {
		return false, nil
	}
}

func (objst *ObjStore) readBucketMetadataFile(ctx context.Context, bucket string, vlog *util.VLog) (*BucketMetadata, error) {
	buf, err := objst.DownloadObjToBuffer(ctx, bucket, MetadataObjName)
	if err != nil {
		log.Println("error: readBucketMetadataFile: cannot download bucket metadata file: ", err)
		return nil, err
	}

	var bMdata BucketMetadata
	if err = json.Unmarshal(buf, &bMdata); err != nil {
		log.Println("error: readBucketMetadataFile: cannot unmarshall bucket metadata json: ", err)
		return nil, err
	}

	return &bMdata, nil
}

func (objst *ObjStore) writeBucketMetadataFile(ctx context.Context, bucket string, bMdata *BucketMetadata, vlog *util.VLog) error {
	buf, err := json.Marshal(bMdata)
	if err != nil {
		log.Println("error: writeBucketMetadataFile: cannot marshall bucket metadata to json: ", err)
		return err
	}

	if err = objst.UploadObjFromBuffer(ctx, bucket, MetadataObjName, buf, ComputeETag(buf)); err != nil {
		log.Println("error: writeBucketMetadataFile: cannot upload metadata to bucket: ", err)
		return err
	}

	return nil
}

func (objst *ObjStore) GetOrCreateBucketMetadata(ctx context.Context, bucket string, vlog *util.VLog) (salt string, version int, err error) {
	// Does the bucket have a metadata file?  If so, read it in and extract salt and version.
	// If not, create one (checking first that bucket is actually empty and warning user if not.)
	var bMdata *BucketMetadata = nil
	mObjs, err := objst.GetObjList(ctx, bucket, MetadataObjName, false, vlog)
	if err != nil {
		log.Println("error: GetOrCreateBucketMetadata: failed while searching for bucket metadata file: ", err)
		return "", 0, err
	}
	for objName := range mObjs {
		if objName == MetadataObjName {
			bMdata, err = objst.readBucketMetadataFile(ctx, bucket, vlog)
			if err != nil {
				if strings.Contains(err.Error(), "connect") {
					return "", 0, ErrCantConnect
				} else {
					log.Println("error: GetOrCreateBucketMetadata: cannot read bucket metadata file: ", err)
					return "", 0, err
				}
			}
		}
	}
	if bMdata != nil {
		return bMdata.Salt, bMdata.Version, nil
	} else {
		isEmpty, err := objst.isBucketEmpty(ctx, bucket, vlog)
		if err != nil {
			log.Println("error: GetOrCreateBucketMetadata: cannot determine if bucket is empty: ", err)
			return "", 0, err
		}
		if isEmpty {
			bMdata = &BucketMetadata{
				Salt:    util.GenerateRandomSalt(),
				Version: 1,
			}
			if err := objst.writeBucketMetadataFile(ctx, bucket, bMdata, vlog); err != nil {
				log.Println("error: GetOrCreateBucketMetadata: cannot write new bucket metadata file: ", err)
				return "", 0, err
			}
			bMdata, err = objst.readBucketMetadataFile(ctx, bucket, vlog)
			if err != nil {
				log.Println("error: GetOrCreateBucketMetadata: cannot read bucket metadata file that we just wrote: ", err)
				return "", 0, err
			}
			return bMdata.Salt, bMdata.Version, nil
		} else {
			return "", 0, ErrNoMetadataButNotEmpty
		}
	}
}

// Verifies the key by decrypting first top-level bucket AES-GCM auth enc
func (objst *ObjStore) VerifyKeyAndSalt(ctx context.Context, bucket string, encKey []byte) error {
	topLevelObjs, err := objst.GetObjListTopLevel(ctx, bucket, []string{"metadata", "chunks"})
	if err != nil {
		log.Printf("error: could not get top level bucket objects '%s': %v", topLevelObjs[0], err)
		return err
	}
	if len(topLevelObjs) > 0 {
		decObjName, err := cryptography.DecryptFilename(encKey, topLevelObjs[0])
		if err != nil {
			log.Printf("Could not decrypt '%s': key is probably wrong (%v)", topLevelObjs[0], err)
			return err
		}
		if len(decObjName) == 0 {
			log.Fatalf("Could not decrypt '%s': key is probably wrong (%v)", decObjName, err)
			return err
		}
	}
	return nil
}
