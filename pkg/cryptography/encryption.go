package cryptography

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	siv "github.com/secure-io/siv-go"
)

// EncryptFileName uses AES-GCM-256-SIV mode of encryption with fixed
// nonce so that the nonce reuse does not leak any information except
// that the same plaintext was encrypted.
func EncryptFilename(key []byte, filename string) (string, error) {
	// gzip filename to shorten it if it's long
	filenameGz := gz(filename)

	aessiv, err := siv.NewGCM(key)
	if err != nil {
		// TODO: don't Fatalln here, just return the error
		log.Fatalln("error: EncryptFilename: ", err)
		return "", err
	}

	// Use a fixed nonce (all zeros) to makes AES-GCM-SIV a deterministic AEAD
	// scheme (same plaintext && additional data produces the same ciphertext).
	nonce := make([]byte, aessiv.NonceSize())

	ciphertext := aessiv.Seal(nil, nonce, []byte(filenameGz), nil)
	ciphertextBase64 := base64.URLEncoding.EncodeToString(ciphertext)

	return ciphertextBase64, nil
}

// Decrypts filenames encrypted with EncryptFilename.
func DecryptFilename(key []byte, encryptedFilenameB64 string) (string, error) {
	encryptedFilename, err := base64.URLEncoding.DecodeString(encryptedFilenameB64)
	if err != nil {
		log.Printf("warning: DecryptFilename failed on '%s' (partial decode: '%x'): %v", encryptedFilenameB64, encryptedFilename, err)
		return "", err
	}

	aessiv, err := siv.NewGCM(key)
	if err != nil {
		// TODO: don't Fatalf here, just return the error?
		log.Fatalf("error: DecryptFilename on '%s' ('%x'): %v", encryptedFilenameB64, encryptedFilename, err)
		return "", err
	}

	// Use the same fixed nonce (all zeros) as in encryption
	nonce := make([]byte, aessiv.NonceSize())

	decryptedFilename, err := aessiv.Open(nil, nonce, encryptedFilename, nil)
	if err != nil {
		// TODO: don't Fatalln here, just return the error
		log.Fatalf("error: DecryptFilename on '%s' ('%x'): %v", encryptedFilenameB64, encryptedFilename, err)
		return "", err
	}

	// gunzip decrypted filename
	filename := string(decryptedFilename)
	filename = gunzip(filename)

	return filename, nil
}

func gz(input string) string {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(input))
	if err != nil {
		log.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.String()
}

func gunzip(gzinput string) string {
	var buf bytes.Buffer = *bytes.NewBuffer([]byte(gzinput))
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		log.Fatal(err)
	}
	sb := new(strings.Builder)
	if _, err := io.Copy(sb, zr); err != nil {
		log.Fatal(err)
	}
	if err := zr.Close(); err != nil {
		log.Fatal(err)
	}
	return sb.String()
}

func AppendEntireFileToBuffer(path string, buf []byte) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		// There are valid reasons why a file might not be openable (eg might have disappeared since we traversed the dir)
		log.Printf("AppendEntireFileToBuffer: %v", err)
		return nil, err
	}
	defer file.Close()

	fileinfo, err := file.Stat()
	if err != nil {
		log.Printf("AppendEntireFileToBuffer: %v", err)
		return nil, err
	}

	filesize := fileinfo.Size()
	buffer := make([]byte, filesize)

	bytesRead, err := file.Read(buffer)
	if err != nil {
		log.Printf("AppendEntireFileToBuffer: %v", err)
		return nil, err
	}

	if bytesRead != int(filesize) {
		log.Printf("AppendEntireFileToBuffer: only read %d bytes (file size: %d bytes)", bytesRead, filesize)
		return nil, errors.New("failed to read entire file")
	}

	buf = append(buf, buffer...)

	return buf, nil
}

func EncryptBuffer(key []byte, plaintext []byte) ([]byte, error) {
	// gzip the plaintext
	plaintext = []byte(gz(string(plaintext)))

	// do AES-GCM encryption of plaintext buffer
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)
	noncePrefixedCipherText := append(nonce, ciphertext...)

	return noncePrefixedCipherText, nil
}

func DecryptBuffer(key []byte, ciphertext []byte) ([]byte, error) {
	// ciphertext must be >12 bytes b/c at a minimum there is the 12-byte nonce and some ciphertext
	if len(ciphertext) <= 12 {
		return nil, fmt.Errorf("error: DecryptBuffer: ciphertext too short to be valid")
	}

	// Extract the nonce (first 12 bytes of ciphertext)
	nonce := ciphertext[0:12]
	ciphertext = ciphertext[12:]

	// Decrypt the ciphertext buffer
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// gunzip the plaintext
	plaintext = []byte(gunzip(string(plaintext)))

	return plaintext, nil
}

func EncryptBufferWithNonce(key []byte, plaintext []byte, nonce []byte) ([]byte, error) {
	// do AES-GCM encryption of plaintext buffer using nonce
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)
	noncePrefixedCipherText := append(nonce, ciphertext...)

	return noncePrefixedCipherText, nil
}

func DecryptBufferReturningNonce(key []byte, ciphertext []byte) (plaintext []byte, nonce []byte, err error) {
	// Extract the nonce (first 12 bytes of ciphertext)
	nonce = ciphertext[0:12]
	ciphertext = ciphertext[12:]

	// Decrypt the ciphertext buffer
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	plaintext, err = aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, nil, err
	}

	return plaintext, nonce, nil
}
