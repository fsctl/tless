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
	"time"

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

func (os *ObjStore) IsReachableWithRetries(ctx context.Context, maxWaitSeconds int, bucket string, vlog *util.VLog) (bool, error) {
	waitSeconds := 1
	var err error
	for waitSeconds < maxWaitSeconds {
		if _, err = os.GetObjList(ctx, bucket, "doesnotexist", vlog); err != nil {
			if vlog == nil {
				log.Printf("warning: server unreachable: %v\n", err)
				log.Printf("trying again in %d seconds...\n", waitSeconds)
			} else {
				vlog.Printf("warning: server unreachable: %v\n", err)
				vlog.Printf("trying again in %d seconds...\n", waitSeconds)
			}
			time.Sleep(time.Duration(waitSeconds * 1e9))
			waitSeconds *= 2
		} else {
			return true, nil
		}
	}
	return false, err
}

func (os *ObjStore) UploadObjFromBuffer(ctx context.Context, bucket string, objectName string, buffer []byte, expectedETag string) error {
	// Upload the file
	contentType := "application/octet-stream"

	// convert byte slice to io.Reader
	reader := bytes.NewReader(buffer)

	// Upload the file with FPutObject
	info, err := os.minioClient.PutObject(ctx, bucket, objectName, reader, int64(len(buffer)), minio.PutObjectOptions{
		ContentType: contentType,
		PartSize:    ObjStoreMultiPartUploadPartSize})
	if err != nil {
		log.Printf("error: UploadObjFromBuffer (%s): %v", objectName, err)
		return err
	}
	//             remove this:  /-------------------\
	if info.ETag != expectedETag && expectedETag != "" {
		log.Printf("error: UploadObjFromBuffer: ETag returned was '%s', expected '%s'", info.ETag, expectedETag)
		return ErrUploadCorrupted
	}

	//log.Printf("Successfully uploaded buffer of size %d\n", info.Size)

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

	return ret, nil
}

func (os *ObjStore) GetObjList(ctx context.Context, bucket string, prefix string, vlog *util.VLog) (map[string]int64, error) {
	mAllObjects := make(map[string]int64, 0)

	opts := minio.ListObjectsOptions{
		Recursive: true,
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
		mAllObjects[object.Key] = object.Size
	}

	return mAllObjects, nil
}

// Gets only the first two levels of the full object list, i.e., all backup_name/snapshot_name
// but none of the objects within a snapshot
func (os *ObjStore) GetObjListTopTwoLevels(ctx context.Context, bucket string, excludeTopLevelWithPrefixes []string) (map[string][]string, error) {
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
			mObjects[topLevelName] = append(mObjects[topLevelName], util.StripTrailingSlashes(subObjKey))
		}
	}

	return mObjects, nil
}

func (os *ObjStore) DeleteObj(ctx context.Context, bucket string, objectName string) error {
	opts := minio.RemoveObjectOptions{}
	err := os.minioClient.RemoveObject(context.Background(), bucket, objectName, opts)
	return err
}

func (os *ObjStore) RenameObj(ctx context.Context, bucket string, objectNameSrc string, objectNameDst string) error {
	srcOpts := minio.CopySrcOptions{
		Bucket: bucket,
		Object: objectNameSrc,
	}
	dstOpts := minio.CopyDestOptions{
		Bucket: bucket,
		Object: objectNameDst,
	}
	_, err := os.minioClient.CopyObject(ctx, dstOpts, srcOpts)
	if err != nil {
		return err
	}

	err = os.DeleteObj(ctx, bucket, objectNameSrc)
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
