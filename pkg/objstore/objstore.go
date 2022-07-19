package objstore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsctl/tless/pkg/database"
	"github.com/fsctl/tless/pkg/util"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	ObjStoreMultiPartUploadPartSize = 16 * 1024 * 1024
)

type ObjStore struct {
	minioClient *minio.Client
}

var (
	ErrUploadCorrupted = errors.New("error: upload corrupted in transit, bad etag returned")
)

var (
	// These are always accessed atomically, so no sync.Mutex to protect them
	uploadBytes   int64 = 0
	downloadBytes int64 = 0
)

func NewObjStore(ctx context.Context, endpoint string, accessKeyId string, secretAccessKey string, isTrustSelfSignedCerts bool) *ObjStore {
	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyId, secretAccessKey, ""),
		Secure: true,
		Transport: &http.Transport{
			DisableCompression: true,
			TLSClientConfig:    &tls.Config{InsecureSkipVerify: isTrustSelfSignedCerts},
		},
	})
	if err != nil {
		log.Fatalln("error: NewObjStore: ", err)
	}

	return &ObjStore{
		minioClient: minioClient,
	}
}

func (os *ObjStore) IsReachable(ctx context.Context, bucket string, vlog *util.VLog) (bool, error) {
	var err error
	_, err = os.GetObjList(ctx, bucket, "doesnotexist", false, vlog)
	if err != nil {
		return false, err
	} else {
		return true, nil
	}
}

func (os *ObjStore) UploadObjFromBuffer(ctx context.Context, bucket string, objectName string, buffer []byte, expectedETag string) error {
	// Upload the file
	contentType := "application/octet-stream"

	backoffSec := 5
	maxBackoffSec := 5 * 60

	for {
		reader := bytes.NewReader(buffer)
		info, err := os.minioClient.PutObject(ctx, bucket, objectName, reader, int64(len(buffer)), minio.PutObjectOptions{
			ContentType: contentType,
			PartSize:    ObjStoreMultiPartUploadPartSize})
		if err != nil {
			// If network became unreachable, try an exponential backoff rather than just erroring out
			if strings.Contains(err.Error(), "network is unreachable") {
				log.Printf("UploadObjFromBuffer: pausing because network became unreachable (%d sec)", backoffSec)
				time.Sleep(time.Second * time.Duration(backoffSec))
				backoffSec *= 2
				if backoffSec > maxBackoffSec {
					backoffSec = maxBackoffSec
				}
				continue
			} else {
				log.Printf("error: UploadObjFromBuffer (%s): %v", objectName, err)
				return err
			}
		} else {
			// Log the download bandwidth
			atomic.AddInt64(&uploadBytes, int64(len(buffer)))
			//log.Printf("notice: UploadObjFromBuffer: added %d bytes to upload byte counter", int64(len(buffer)))
		}
		if info.ETag != expectedETag {
			log.Printf("error: UploadObjFromBuffer: ETag returned was '%s', expected '%s'", info.ETag, expectedETag)
			return ErrUploadCorrupted
		}
		break
	}

	return nil
}

func (os *ObjStore) DownloadObjToBuffer(ctx context.Context, bucket string, objectName string) ([]byte, error) {
	// Upload the file with FPutObject
	reader, err := os.minioClient.GetObject(ctx, bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		log.Println("error: DownloadObjToBuffer (GetObject): ", err)
		return nil, err
	}
	defer reader.Close()

	// Copy reader to byte array
	var b bytes.Buffer
	bufWriter := bufio.NewWriter(&b)

	stat, err := reader.Stat()
	if err != nil {
		log.Printf("error: DownloadObjToBuffer (Stat on '%s'): %v", objectName, err)
		return nil, err
	}

	n, err := io.CopyN(bufWriter, reader, stat.Size)
	if err != nil {
		log.Println("error: DownloadObjToBuffer (CopyN): ", err)
		return nil, err
	}
	ret := b.Bytes()

	_ = n
	//log.Printf("Successfully downloaded %d (=%d) bytes to buffer\n", n, len(ret))

	// Log the download bandwidth
	atomic.AddInt64(&downloadBytes, int64(len(ret)))
	//log.Printf("notice: DownloadObjToBuffer: added %d bytes to download byte counter", int64(len(ret)))

	return ret, nil
}

func (os *ObjStore) GetObjList(ctx context.Context, bucket string, prefix string, recursive bool, vlog *util.VLog) (map[string]int64, error) {
	mObjects := make(map[string]int64, 0)

	opts := minio.ListObjectsOptions{
		Recursive: recursive,
		Prefix:    prefix,
	}

	for object := range os.minioClient.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			msg := fmt.Sprintf("warning: GetObjList (ListObjects): %v", object.Err)
			if vlog != nil {
				vlog.Println(msg)
			} else {
				log.Println(msg)
			}
			return nil, object.Err
		}
		mObjects[object.Key] = object.Size
	}

	return mObjects, nil
}

// Gets only the top levels objects, i.e., all backup_name directories
func (os *ObjStore) GetObjListTopLevel(ctx context.Context, bucket string, excludePrefixes []string) ([]string, error) {
	objects := make([]string, 0)

	opts := minio.ListObjectsOptions{
		Recursive: false,
		Prefix:    "",
	}
	for object := range os.minioClient.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			log.Printf("warning: GetObjListTopTwoLevels: %v", object.Err)
			return nil, object.Err
		}

		skip := false
		for _, exclPrefix := range excludePrefixes {
			if strings.HasPrefix(object.Key, exclPrefix) {
				skip = true
			}
		}
		if !skip {
			objects = append(objects, util.StripTrailingSlashes(object.Key))
		}
	}

	return objects, nil
}

// Gets only the first two levels of the full object list, i.e., all backup_name/snapshot_name
// but none of the objects within a snapshot
func (os *ObjStore) GetObjListTopTwoLevels(ctx context.Context, bucket string, excludeTopLevelWithPrefixes []string, excludeSecondLevelWithPrefix []string) (map[string][]string, error) {
	mObjects := make(map[string][]string, 0)

	// Get the top level
	opts := minio.ListObjectsOptions{
		Recursive: false,
		Prefix:    "",
	}
	for object := range os.minioClient.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			log.Printf("warning: GetObjListTopTwoLevels: %v", object.Err)
			return nil, object.Err
		}

		skip := false
		for _, exclPrefix := range excludeTopLevelWithPrefixes {
			if strings.HasPrefix(object.Key, exclPrefix) {
				skip = true
			}
		}
		if !skip {
			mObjects[util.StripTrailingSlashes(object.Key)] = make([]string, 0)
		}
	}

	// Loop over all top level objects and get everything at the next level
	for topLevelName := range mObjects {
		subOpts := minio.ListObjectsOptions{
			Recursive: false,
			Prefix:    topLevelName + "/",
		}
		for subObject := range os.minioClient.ListObjects(ctx, bucket, subOpts) {
			if subObject.Err != nil {
				log.Printf("warning: GetObjListTopTwoLevels: %v", subObject.Err)
				return nil, subObject.Err
			}
			subObjKey := strings.TrimPrefix(subObject.Key, topLevelName+"/")

			skip := false
			for _, exclPrefix := range excludeSecondLevelWithPrefix {
				if strings.HasPrefix(subObjKey, exclPrefix) {
					skip = true
				}
			}
			if !skip {
				mObjects[topLevelName] = append(mObjects[topLevelName], util.StripTrailingSlashes(subObjKey))
			}
		}
	}

	return mObjects, nil
}

func (os *ObjStore) DeleteObj(ctx context.Context, bucket string, objectName string) error {
	opts := minio.RemoveObjectOptions{}
	err := os.minioClient.RemoveObject(context.Background(), bucket, objectName, opts)
	return err
}

func (objst *ObjStore) RenameObj(ctx context.Context, bucket string, objectNameSrc string, objectNameDst string) error {
	// Bandwidth logging: determine the size of the object
	var objByteCnt int64 = 0
	m, err := objst.GetObjList(ctx, bucket, objectNameSrc, false, nil)
	if err != nil {
		log.Println("error: RenameObj: cannot get size of source object; bandwidth won't be counted")
	} else {
		if len(m) != 1 {
			msg := fmt.Sprintf("error: RenameObj: there are %d objects named %s, expected 1 object with that name", len(m), objectNameSrc)
			log.Println(msg)
			return fmt.Errorf(msg)
		}
		for _, byteCnt := range m {
			objByteCnt = byteCnt
			break
		}
	}

	srcOpts := minio.CopySrcOptions{
		Bucket: bucket,
		Object: objectNameSrc,
	}
	dstOpts := minio.CopyDestOptions{
		Bucket: bucket,
		Object: objectNameDst,
	}
	_, err = objst.minioClient.CopyObject(ctx, dstOpts, srcOpts)
	if err != nil {
		return err
	} else {
		// Count copy as upload and download bytes; not clear this is correct for all providers
		atomic.AddInt64(&uploadBytes, objByteCnt)
		atomic.AddInt64(&downloadBytes, objByteCnt)
		log.Printf("notice: RenameObj: added %d, %d bytes to up/download byte counters", objByteCnt, objByteCnt)
	}

	err = objst.DeleteObj(ctx, bucket, objectNameSrc)
	return err
}

func (os *ObjStore) ListBuckets(ctx context.Context) ([]string, error) {
	buckets, err := os.minioClient.ListBuckets(ctx)
	if err != nil {
		return []string{}, err
	}
	ret := make([]string, 0)
	for _, bucketInfo := range buckets {
		ret = append(ret, bucketInfo.Name)
	}
	sort.Strings(ret)
	return ret, nil
}

func (os *ObjStore) MakeBucket(ctx context.Context, bucketName string, region string) error {
	err := os.minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{Region: region, ObjectLocking: false})
	if err != nil {
		return err
	}
	return nil
}

// Computes the expected ETag for the entire buffer buf
// Ref: https://stackoverflow.com/questions/12186993/what-is-the-algorithm-to-compute-the-amazon-s3-etag-for-a-file-larger-than-5gb#answer-19896823
func ComputeETag(buf []byte) string {
	md5s := make([][16]byte, 0)
	bufStartPos := 0
	for {
		var bufPart []byte
		bufEndPos := bufStartPos + ObjStoreMultiPartUploadPartSize
		if len(buf) > bufEndPos {
			bufPart = buf[bufStartPos:bufEndPos]
			md5s = append(md5s, md5.Sum(bufPart))
			bufStartPos = bufEndPos
		} else {
			bufPart = buf[bufStartPos:]
			md5s = append(md5s, md5.Sum(bufPart))
			break
		}
	}

	// If there's just one md5 then we merely return it
	var eTag string
	if len(md5s) == 1 {
		eTag = fmt.Sprintf("%x", md5s[0])
	} else {
		// Otherwise, concatenate the md5s into a single []byte, and md5 that
		concatMd5s := make([]byte, 0)
		for _, md5Val := range md5s {
			for i := 0; i < 16; i++ {
				concatMd5s = append(concatMd5s, md5Val[i])
			}
		}
		eTag = fmt.Sprintf("%x-%d", md5.Sum(concatMd5s), len(md5s))
	}

	return eTag
}

func (objst *ObjStore) PersistBandwidthUsage(dbLock *sync.Mutex, db *database.DB, vlog *util.VLog) error {
	// Get the upload and download bandwidth values and atomically reset those accumulators to zero
	upBytes := int64(0)
	downBytes := int64(0)
	var ok bool
	for {
		upBytes = atomic.LoadInt64(&uploadBytes)
		if ok = atomic.CompareAndSwapInt64(&uploadBytes, upBytes, 0); ok {
			break
		} else {
			vlog.Println("warning: objst.PersistBandwidthUsage: CAS failed")
		}
	}
	for {
		downBytes = atomic.LoadInt64(&downloadBytes)
		if ok = atomic.CompareAndSwapInt64(&downloadBytes, downBytes, 0); ok {
			break
		} else {
			vlog.Println("warning: objst.PersistBandwidthUsage: CAS failed")
		}
	}

	// Persist the retrieved values to database
	if upBytes+downBytes > 0 {
		util.LockIf(dbLock)
		err := db.AddBandwidthUsageReport(time.Now().Unix(), upBytes+downBytes)
		util.UnlockIf(dbLock)
		if err != nil {
			log.Println("error: objst.PersistBandwidthUsage: AddBandwidthUsageReport failed (putting byte counts back): ", err)
			atomic.AddInt64(&uploadBytes, upBytes)
			atomic.AddInt64(&uploadBytes, downBytes)
			return err
		}
	}

	return nil
}
