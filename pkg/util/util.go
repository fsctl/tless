// Package util contains common helper functions used in multiple places
package util

import (
	"crypto/rand"
	"log"
	"math/big"
	"strings"

	"github.com/sethvargo/go-diceware/diceware"
)

// Returns s without any trailing slashes if it has any; otherwise return s unchanged.
func StripTrailingSlashes(s string) string {
	if s == "/" {
		return s
	}

	for {
		stripped := strings.TrimSuffix(s, "/")
		if stripped == s {
			return stripped
		} else {
			s = stripped
		}
	}
}

func GenerateConfigTemplate() string {
	template := `[objectstore]
# Customize this section with the real host:port of your S3-compatible object 
# store, your credentials for the object store, and a bucket you have ALREADY 
# created for storing backups.
endpoint = "127.0.0.1:9000"
access_key_id = "<your object store user id>"
access_secret = "<your object store password>"
bucket = "<name of an empty bucket you have created on object store>"

[backups]
# You can specify as many directories to back up as you want. All paths 
# should be absolute paths. 
# Example (Linux): /home/<yourname>/Documents
# Example (macOS): /Users/<yourname>/Documents
dirs = [ "<absolute path to directory>", "<optional additional directory>" ]

# Specify as many exclusion paths as you want. Excludes can be entire 
# directories or single files. All paths should be absolute paths. 
# Example (Linux): /home/<yourname>/Documents/MyJournal
# Example (macOS): /Users/<yourname>/Documents/MyJournal
excludes = [ "<absolute path to exclude>", "<optional additional exclude path>" ]

# The 10-word Diceware passphrase below has been randomly generated for you. 
# It has ~128 bits of entropy and thus is very resistant to brute force 
# cracking through at least the middle of this century.
#
# Note that your passphrase resides in this file but never leaves this machine.
master_password = "`

	template += GenerateRandomPassphrase(10)

	template += `"

# This salt has been randomly generated for you; there's no need to change it.
# The salt does not need to be kept secret. In fact, a backup copy is stored 
# on the object store server as 'SALT-[salt_string]' in the bucket root.
salt = "`
	template += GenerateRandomSalt() + "\"\n"
	return template
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
