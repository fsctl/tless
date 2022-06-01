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

type CfgSettings struct {
	Endpoint        string
	AccessKeyId     string
	SecretAccessKey string
	Bucket          string
	MasterPassword  string
	Salt            string
	Dirs            []string
	ExcludePaths    []string
}

func GenerateConfigTemplate(configValues *CfgSettings) string {
	template := `[objectstore]
# Customize this section with the real host:port of your S3-compatible object 
# store, your credentials for the object store, and a bucket you have ALREADY 
# created for storing backups.
endpoint = "`

	if configValues != nil && configValues.Endpoint != "" {
		template += configValues.Endpoint
	} else {
		template += "127.0.0.1:9000"
	}

	template += `"
access_key_id = "`

	if configValues != nil && configValues.AccessKeyId != "" {
		template += configValues.AccessKeyId
	} else {
		template += "<your object store user id>"
	}

	template += `"
access_secret = "`

	if configValues != nil && configValues.SecretAccessKey != "" {
		template += configValues.SecretAccessKey
	} else {
		template += "<your object store password>"
	}

	template += `"
bucket = "`

	if configValues != nil && configValues.Bucket != "" {
		template += configValues.Bucket
	} else {
		template += "<name of an empty bucket you have created on object store>"
	}

	template += `"

[backups]
# You can specify as many directories to back up as you want. All paths 
# should be absolute paths. 
# Example (Linux): /home/<yourname>/Documents
# Example (macOS): /Users/<yourname>/Documents
dirs = [ `

	if configValues != nil && len(configValues.Dirs) > 0 {
		template += sliceToCommaSeparatedString(configValues.Dirs)
	} else {
		template += "\"<absolute path to directory>\", \"<optional additional directory>\""
	}

	template += ` ]

# Specify as many exclusion paths as you want. Excludes can be entire 
# directories or single files. All paths should be absolute paths. 
# Example (Linux): /home/<yourname>/Documents/MyJournal
# Example (macOS): /Users/<yourname>/Documents/MyJournal
excludes = [ `

	if configValues != nil && len(configValues.ExcludePaths) > 0 {
		template += sliceToCommaSeparatedString(configValues.ExcludePaths)
	} else {
		template += "\"<absolute path to exclude>\", \"<optional additional exclude path>\""
	}

	template += ` ]

# The 10-word Diceware passphrase below has been randomly generated for you. 
# It has ~128 bits of entropy and thus is very resistant to brute force 
# cracking through at least the middle of this century.
#
# Note that your passphrase resides in this file but never leaves this machine.
master_password = "`

	if configValues != nil && configValues.MasterPassword != "" {
		template += configValues.MasterPassword
	} else {
		template += GenerateRandomPassphrase(10)
	}

	template += `"

# This salt has been randomly generated for you; there's no need to change it.
# The salt does not need to be kept secret. In fact, a backup copy is stored 
# on the object store server as 'SALT-[salt_string]' in the bucket root.
salt = "`

	if configValues != nil && configValues.Salt != "" {
		template += configValues.Salt + "\"\n"
	} else {
		template += GenerateRandomSalt() + "\"\n"
	}

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

// Turns a slice of strings into a toml format array, i.e.:
// "string1", "string2", "string3"
func sliceToCommaSeparatedString(s []string) string {
	ret := ""
	for i := 0; i < len(s); i++ {
		ret += "\"" + s[i] + "\""
		if i+1 < len(s) {
			ret += ", "
		}
	}
	return ret
}

// Accepts a string of any length and returns '********' of same length.
func MakeLogSafe(s string) string {
	ret := ""
	for range s {
		ret += "*"
	}
	return ret
}
