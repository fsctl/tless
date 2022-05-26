package objstore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	ObjStoreMultiPartUploadPartSize = 129 * 1024 * 1024
)

type ObjStore struct {
	endpoint        string
	accessKeyId     string
	secretAccessKey string
	minioClient     *minio.Client
}

var (
	ErrUploadCorrupted = errors.New("error: upload corrupted in transit, bad etag returned")
)

func NewObjStore(ctx context.Context, endpoint string, accessKeyId string, secretAccessKey string) *ObjStore {
	useSSL := !true

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyId, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalln("error: NewObjStore: ", err)
	}

	return &ObjStore{
		endpoint:        endpoint,
		accessKeyId:     accessKeyId,
		secretAccessKey: secretAccessKey,
		minioClient:     minioClient,
	}
}

func (os *ObjStore) IsReachableWithRetries(ctx context.Context, maxWaitSeconds int, bucket string) bool {
	waitSeconds := 1
	for waitSeconds < maxWaitSeconds {
		if _, err := os.GetObjList(ctx, bucket, ""); err != nil {
			log.Printf("warning: server unreachable: %v\n", err)
			log.Printf("trying again in %d seconds...\n", waitSeconds)
			time.Sleep(time.Duration(waitSeconds * 1e9))
			waitSeconds *= 2
		} else {
			return true
		}
	}
	return false
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

func (os *ObjStore) GetObjList(ctx context.Context, bucket string, prefix string) (map[string]int64, error) {
	mAllObjects := make(map[string]int64, 0)

	opts := minio.ListObjectsOptions{
		Recursive: true,
		Prefix:    prefix,
	}

	for object := range os.minioClient.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			log.Printf("warning: GetObjList (ListObjects): %v", object.Err)
			return nil, object.Err
		}
		mAllObjects[object.Key] = object.Size
	}

	return mAllObjects, nil
}

func (os *ObjStore) DeleteObj(ctx context.Context, bucket string, objectName string) error {
	opts := minio.RemoveObjectOptions{
		GovernanceBypass: true,
	}
	err := os.minioClient.RemoveObject(context.Background(), bucket, objectName, opts)
	return err
}

func (os *ObjStore) TryReadSalt(ctx context.Context, bucket string) (string, error) {
	if m, err := os.GetObjList(ctx, bucket, "SALT-"); err == nil {
		for key := range m {
			salt := strings.TrimPrefix(key, "SALT-")
			return salt, nil
		}
	} else {
		return "", err
	}

	return "", nil
}

func (os *ObjStore) TryWriteSalt(ctx context.Context, bucket string, salt string) error {
	err := os.UploadObjFromBuffer(ctx, bucket, "SALT-"+salt, []byte(""), ComputeETag([]byte{}))
	return err
}

// Computes the expected ETag for the entire buffer buf
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

	// if len(buf) was <= ObjStoreMultiPartUploadPartSize then we
	// just return the md5
	var eTag string
	if len(md5s) == 1 {
		eTag = fmt.Sprintf("%x", md5s[0])
	} else {
		// Concatenate the md5s into a single []byte, and md5 that
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
