package objstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Salt                string
	Version             int
	EncryptedEncKeyB64  string
	EncryptedHmacKeyB64 string
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

func (objst *ObjStore) readBucketMetadataFile(ctx context.Context, bucket string, masterPassword string, vlog *util.VLog) (bMdataPtr *BucketMetadata, encKey []byte, hmacKey []byte, err error) {
	buf, err := objst.DownloadObjToBuffer(ctx, bucket, MetadataObjName)
	if err != nil {
		log.Println("error: readBucketMetadataFile: cannot download bucket metadata file: ", err)
		return nil, nil, nil, err
	}

	var bMdata BucketMetadata
	if err = json.Unmarshal(buf, &bMdata); err != nil {
		log.Println("error: readBucketMetadataFile: cannot unmarshall bucket metadata json: ", err)
		return nil, nil, nil, err
	}

	// Is salt valid?  Needed for pbKey derivation in next step
	if len(bMdata.Salt) < util.SaltLen {
		err = fmt.Errorf("error: readBucketMetadataFile: the bucket salt retrieved is too short (%d chars) to be valid (%d chars required): ", len(bMdata.Salt), util.SaltLen)
		return nil, nil, nil, err
	}

	// Derive the password-derived key (pdKey) which we'll need to decrypt the two encrypted keys
	// in bucket
	pdKey, err := cryptography.DeriveKey(bMdata.Salt, masterPassword)
	if err != nil {
		e := fmt.Errorf("error: readBucketMetadataFile: could not derive pdKey: %v", err)
		vlog.Println(e.Error())
		return nil, nil, nil, e
	}

	// Un-base64 the two encrypted keys from bucket
	encryptedEncKey, err := base64.URLEncoding.DecodeString(bMdata.EncryptedEncKeyB64)
	if err != nil {
		vlog.Printf("error: readBucketMetadataFile: could not base64 decode encrypted encryption key base64 string: %v", err)
		return nil, nil, nil, err
	}
	encryptedHmacKey, err := base64.URLEncoding.DecodeString(bMdata.EncryptedHmacKeyB64)
	if err != nil {
		vlog.Printf("error: readBucketMetadataFile: could not base64 decode encrypted HMAC key base64 string: %v", err)
		return nil, nil, nil, err
	}

	// Decrypt the encrypted keys in bucket using pdKey
	encKey, err = cryptography.DecryptBuffer(pdKey, encryptedEncKey)
	if err != nil {
		vlog.Printf("error: readBucketMetadataFile: could not decrypt encrypted encryption key: %v", err)
		return nil, nil, nil, err
	}
	hmacKey, err = cryptography.DecryptBuffer(pdKey, encryptedHmacKey)
	if err != nil {
		vlog.Printf("error: readBucketMetadataFile: could not decrypt encrypted HMAC key: %v", err)
		return nil, nil, nil, err
	}

	return &bMdata, encKey, hmacKey, nil
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

func (objst *ObjStore) GetOrCreateBucketMetadata(ctx context.Context, bucket string, masterPassword string, vlog *util.VLog) (salt string, version int, encKey []byte, hmacKey []byte, err error) {
	// Does the bucket have a metadata file?  If so, read it in and extract salt, decrypted keys
	// and version.
	// If not, create one (checking first that bucket is actually empty and warning user if not.)
	var bMdata *BucketMetadata = nil
	mObjs, err := objst.GetObjList(ctx, bucket, MetadataObjName, false, vlog)
	if err != nil {
		log.Println("error: GetOrCreateBucketMetadata: failed while searching for bucket metadata file: ", err)
		return "", 0, nil, nil, err
	}
	for objName := range mObjs {
		if objName == MetadataObjName {
			bMdata, encKey, hmacKey, err = objst.readBucketMetadataFile(ctx, bucket, masterPassword, vlog)
			if err != nil {
				if strings.Contains(err.Error(), "connect") {
					return "", 0, nil, nil, ErrCantConnect
				} else {
					log.Println("error: GetOrCreateBucketMetadata: cannot read bucket metadata file: ", err)
					return "", 0, nil, nil, err
				}
			}
		}
	}
	if bMdata != nil && len(encKey) == 32 && len(hmacKey) == 32 {
		return bMdata.Salt, bMdata.Version, encKey, hmacKey, nil
	} else {
		isEmpty, err := objst.isBucketEmpty(ctx, bucket, vlog)
		if err != nil {
			log.Println("error: GetOrCreateBucketMetadata: cannot determine if bucket is empty: ", err)
			return "", 0, nil, nil, err
		}
		if isEmpty {
			// generate the new salt we're going to use when writing new bucket
			salt = util.GenerateRandomSalt()

			// derive pdKey using new salt
			pdKey, err := cryptography.DeriveKey(salt, masterPassword)
			if err != nil {
				e := fmt.Errorf("error: readBucketMetadataFile: could not derive pdKey: %v", err)
				vlog.Println(e.Error())
				return "", 0, nil, nil, e
			}

			// generate two random keys and encrypt+base64 both with pdKey
			encKey = make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, encKey); err != nil {
				return "", 0, nil, nil, err
			}
			encryptedEncKey, err := cryptography.EncryptBuffer(pdKey, encKey)
			encryptedEncKeyB64 := base64.URLEncoding.EncodeToString(encryptedEncKey)

			hmacKey = make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, hmacKey); err != nil {
				return "", 0, nil, nil, err
			}
			encryptedHmacKey, err := cryptography.EncryptBuffer(pdKey, hmacKey)
			encryptedHmacKeyB64 := base64.URLEncoding.EncodeToString(encryptedHmacKey)

			bMdata = &BucketMetadata{
				Salt:                salt,
				Version:             1,
				EncryptedEncKeyB64:  encryptedEncKeyB64,
				EncryptedHmacKeyB64: encryptedHmacKeyB64,
			}
			if err := objst.writeBucketMetadataFile(ctx, bucket, bMdata, vlog); err != nil {
				log.Println("error: GetOrCreateBucketMetadata: cannot write new bucket metadata file: ", err)
				return "", 0, nil, nil, err
			}
			bMdata, encKey, hmacKey, err = objst.readBucketMetadataFile(ctx, bucket, masterPassword, vlog)
			if err != nil {
				log.Println("error: GetOrCreateBucketMetadata: cannot read bucket metadata file that we just wrote: ", err)
				return "", 0, nil, nil, err
			}
			return bMdata.Salt, bMdata.Version, encKey, hmacKey, nil
		} else {
			return "", 0, nil, nil, ErrNoMetadataButNotEmpty
		}
	}
}

// Verifies the keys by trying to decrypt bucket metadata and seeing if we get the same result.
// Further verify encKey by trying to decrypt any snapshot names in bucket.
func (objst *ObjStore) VerifyKeys(ctx context.Context, bucket string, masterPassword string, encKey []byte, hmacKey []byte, vlog *util.VLog) error {
	_, encKeyReadBack, hmacKeyReadBack, err := objst.readBucketMetadataFile(ctx, bucket, masterPassword, vlog)
	if err != nil {
		log.Printf("error: VerifyKeysAndSalt: could not read bucket metadata: %v", err)
		return err
	}

	// Are they byte-for-byte equal to expected keys?
	if !bytes.Equal(encKey, encKeyReadBack) || !bytes.Equal(hmacKey, hmacKeyReadBack) {
		err = fmt.Errorf("error: VerifyKeysAndSalt: keys did not match re-derived keys")
		return err
	}

	// Further verify encKey by decrypting top level objects if there are any
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
