package cryptography

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncryptDecryptFilenameAesGcm256Siv(t *testing.T) {
	key := []byte{0x47, 0x0e, 0x0b, 0x8b, 0xee, 0x2c, 0x22, 0x07, 0x58, 0x00, 0xf3, 0x33, 0x42, 0xd9, 0x2e, 0x34, 0xf7, 0x1f, 0x20, 0xff, 0xb7, 0x98, 0xa2, 0x5c, 0x2c, 0x6a, 0xfc, 0x79, 0x36, 0x8f, 0x62, 0xba}
	filename := "dir1/dir2/myfile.txt"
	expectedEncFilename := "zpkzlh2JJMJ1Z3h9VNRxRh036PDUCSVNTZTi39Mb_0DO2QyoWb_Tb_BPpmbcQVKUsSMqCrbbZbcQ0YK-"

	encFilename, err := EncryptFilename(key, filename)
	assert.NoError(t, err)
	assert.Equal(t, expectedEncFilename, encFilename)

	recoveredFilename, err := DecryptFilename(key, encFilename)
	assert.NoError(t, err)
	assert.Equal(t, filename, recoveredFilename)
}

func TestEncryptDecryptFilenameAesGcm256SivLongFilename(t *testing.T) {
	key := []byte{0x47, 0x0e, 0x0b, 0x8b, 0xee, 0x2c, 0x22, 0x07, 0x58, 0x00, 0xf3, 0x33, 0x42, 0xd9, 0x2e, 0x34, 0xf7, 0x1f, 0x20, 0xff, 0xb7, 0x98, 0xa2, 0x5c, 0x2c, 0x6a, 0xfc, 0x79, 0x36, 0x8f, 0x62, 0xba}
	filename := "longdirname012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789/long-subdir-name-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-abcdefghijklmnopqrstuvwxyz-aaaaaaaaaaaaaaaaaaaaa/another-long-subdir-0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789/file"

	encFilename, err := EncryptFilename(key, filename)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(encFilename), 300)

	recoveredFilename, err := DecryptFilename(key, encFilename)
	assert.NoError(t, err)
	assert.Equal(t, filename, recoveredFilename)
}

func TestAppendEntireFileToBuffer(t *testing.T) {
	buf := []byte{0x01, 0x02, 0x03} // initial bytes in buffer

	fileContents := "abc"

	// create a temporary file with fileContents
	file, err := ioutil.TempFile("", "testfile.*.bin")
	assert.NoError(t, err)
	defer os.Remove(file.Name())
	_, err = file.Write([]byte(fileContents))
	assert.NoError(t, err)
	file.Close()

	// append the file to buf
	buf, err = AppendEntireFileToBuffer(file.Name(), buf)
	assert.NoError(t, err)

	// check if buf is correct
	expected := []byte{0x01, 0x02, 0x03, 'a', 'b', 'c'}
	assert.Equal(t, expected, buf)
}

func TestEncryptDecryptBuffer(t *testing.T) {
	key := []byte{0x47, 0x0e, 0x0b, 0x8b, 0xee, 0x2c, 0x22, 0x07, 0x58, 0x00, 0xf3, 0x33, 0x42, 0xd9, 0x2e, 0x34, 0xf7, 0x1f, 0x20, 0xff, 0xb7, 0x98, 0xa2, 0x5c, 0x2c, 0x6a, 0xfc, 0x79, 0x36, 0x8f, 0x62, 0xba}
	buf := []byte{0x01, 0x02, 0x03} // initial bytes in buffer

	fileContents := "abc"

	// create a temporary file with fileContents
	file, err := ioutil.TempFile("", "testfile.*.bin")
	assert.NoError(t, err)
	defer os.Remove(file.Name())
	_, err = file.Write([]byte(fileContents))
	assert.NoError(t, err)
	file.Close()

	// append the file to buf
	buf, err = AppendEntireFileToBuffer(file.Name(), buf)
	assert.NoError(t, err)

	// check if buf is correct
	expected := []byte{0x01, 0x02, 0x03, 'a', 'b', 'c'}
	assert.Equal(t, expected, buf)

	// encrypt
	ciphertextBuf, err := EncryptBuffer(key, buf)
	assert.NoError(t, err)

	// decrypt
	recoveredPlaintextBuf, err := DecryptBuffer(key, ciphertextBuf)
	assert.NoError(t, err)
	assert.Equal(t, buf, recoveredPlaintextBuf)
}

func TestEncryptDecryptBufferWithNonceFunctions(t *testing.T) {
	key := []byte{0x47, 0x0e, 0x0b, 0x8b, 0xee, 0x2c, 0x22, 0x07, 0x58, 0x00, 0xf3, 0x33, 0x42, 0xd9, 0x2e, 0x34, 0xf7, 0x1f, 0x20, 0xff, 0xb7, 0x98, 0xa2, 0x5c, 0x2c, 0x6a, 0xfc, 0x79, 0x36, 0x8f, 0x62, 0xba}
	nonce := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}
	buf := []byte{0x01, 0x02, 0x03} // initial bytes in buffer

	// encrypt buffer
	ciphertextBuf, err := EncryptBufferWithNonce(key, buf, nonce)
	assert.NoError(t, err)

	// decrypt buffer
	recoveredPlaintextBuf, recoveredNonce, err := DecryptBufferReturningNonce(key, ciphertextBuf)
	assert.NoError(t, err)
	assert.Equal(t, buf, recoveredPlaintextBuf)
	assert.Equal(t, nonce, recoveredNonce)
}
