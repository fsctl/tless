// Package cryptography provides cryptography functions for
// operations like key derivation, file name encryption/decryption,
// and file contents encryption/decryption.
package cryptography

import (
	"crypto/rand"
	"errors"
	"log"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/sethvargo/go-diceware/diceware"
)

var (
	// ErrorBadSalt is returned by DeriveKey if salt is empty or too short
	ErrorBadSalt = errors.New("salt was empty or too short")
)

// DeriveKey derives a 32-byte (256-bit) encryption key from the user's
// master password and a non-secret but random salt string
func DeriveKey(salt string, masterPassword string) ([]byte, error) {
	if len(salt) < 8 {
		return nil, ErrorBadSalt
	}
	key := argon2.IDKey([]byte(masterPassword), []byte(salt), 3, 1024*1024, 4, 32)
	return key, nil
}

func GenerateRandomSalt() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	lettersLen := big.NewInt(int64(len(letters)))

	salt := ""

	for i := 0; i < 32; i++ {
		n, err := rand.Int(rand.Reader, lettersLen)
		if err != nil {
			log.Fatalf("error: generateRandomSalt: %v", err)
		}
		randLetter := letters[n.Int64()]
		salt += string(randLetter)
	}

	return salt
}

func GenerateRandomPassphrase(numDicewareWords int) string {
	list, err := diceware.Generate(numDicewareWords)
	if err != nil {
		log.Fatalln("error: could not generate random diceware passphrase", err)
	}
	return strings.Join(list, "-")
}
