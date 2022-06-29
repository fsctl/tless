// Package util contains common helper functions used in multiple places
package util

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sethvargo/go-diceware/diceware"
)

const (
	SaltLen int = 32
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
	Endpoint             string
	AccessKeyId          string
	SecretAccessKey      string
	Bucket               string
	TrustSelfSignedCerts bool
	MasterPassword       string
	Salt                 string
	Dirs                 []string
	ExcludePaths         []string
	VerboseDaemon        bool
}

func GenerateConfigTemplate(configValues *CfgSettings) string {
	template := `[objectstore]
# Customize this section with the real host:port of your S3-compatible object 
# store, your credentials for the object store, and a bucket you have ALREADY 
# created for storing backups.
#
# You can leave access_key_id blank; you then will need to supply it on each
# run of the program.
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
trust_self_signed_certs = `

	if configValues != nil {
		if configValues.TrustSelfSignedCerts {
			template += "true"
		} else {
			template += "false"
		}
	} else {
		template += "true"
	}

	template += `

[backups]
# You can specify as many directories to back up as you want. All paths 
# should be absolute paths. 
# Example (Linux): /home/<yourname>/Documents
# Example (macOS): /Users/<yourname>/Documents
dirs = [ `

	if configValues != nil {
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

	if configValues != nil {
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
#
# You can leave this field blank; you will need to supply your passphrase on
# each run of the program.
master_password = "`

	if configValues != nil && configValues.MasterPassword != "" {
		template += configValues.MasterPassword
	} else {
		template += GenerateRandomPassphrase(10)
	}

	template += `"

[daemon]
# This section affects only the daemon.
verbose = `

	if configValues != nil {
		if configValues.VerboseDaemon {
			template += "true"
		} else {
			template += "false"
		}
	} else {
		template += "true"
	}

	template += `
`

	return template
}

func GenerateRandomSalt() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	lettersLen := big.NewInt(int64(len(letters)))

	salt := ""

	for i := 0; i < SaltLen; i++ {
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

// Makes the directory $HOME/.tless, where $HOME is either for current user or overridden with
// arguments (if both are non-empty strings).  The correct owner and group are set for the new dir.
// Returns path to new dir $HOME/.tless
func MkdirUserConfig(username string, userHomeDir string) (string, error) {
	if username == "" || userHomeDir == "" {
		if u, err := user.Current(); err != nil {
			return "", fmt.Errorf("error: could not lookup current user: %v", err)
		} else {
			username = u.Username
			userHomeDir = u.HomeDir
		}
	}

	// get the user's numeric uid and primary group's gid
	uid, gid, err := GetUidGid(username)
	if err != nil {
		return "", err
	}

	// make the config file dir
	configDir := filepath.Join(userHomeDir, ".tless")
	if err := os.Mkdir(configDir, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("error: mkdir failed: %v", err)
	}
	if err := os.Chmod(configDir, 0755); err != nil {
		return "", fmt.Errorf("error: chmod on created config dir failed: %v", err)
	}
	if err := os.Chown(configDir, uid, gid); err != nil {
		return "", fmt.Errorf("error: could not chown dir to '%d/%d': %v", uid, gid, err)
	}

	return configDir, nil
}

// Get the specified user's numeric uid and primary group's numeric gid
func GetUidGid(username string) (uid int, gid int, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, 0, fmt.Errorf("error: could not lookup user '%s': %v", username, err)
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("error: could not convert uid string '%s' to int: %v", u.Uid, err)
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("error: could not convert gid string '%s' to int: %v", u.Gid, err)
	}
	return uid, gid, nil
}

func GetUnixTimeFromSnapshotName(snapshotName string) int64 {
	tm, err := time.Parse("2006-01-02_15.04.05", snapshotName)
	if err != nil {
		log.Fatalln("error: getUnixTimeFromSnapshotName: ", err)
	}
	return tm.Unix()
}

// Acquires mutex if it is not nil. If mutex is nil, then this function is a no-op.
func LockIf(lock *sync.Mutex) {
	if lock != nil {
		lock.Lock()
	}
}

// Releases mutex if it is not nil. If mutex is nil, then this function is a no-op.
func UnlockIf(lock *sync.Mutex) {
	if lock != nil {
		lock.Unlock()
	}
}

// Returns a formatted string representing the number of bytes in a sensible unit,
// such as "36.0 mb" for input of 37748736.  Ranges from "b" to "gb".
func FormatBytesAsString(bcount int) string {
	if bcount < 1024 {
		return fmt.Sprintf("%d bytes", bcount)
	} else if bcount < 1024*1024 {
		return fmt.Sprintf("%01f kb", float64(bcount)/1024)
	} else if bcount < 1024*1024*1024 {
		return fmt.Sprintf("%01f mb", float64(bcount)/float64(1024*1024))
	} else if bcount < 1024*1024*1024*1024 {
		return fmt.Sprintf("%01f gb", float64(bcount)/float64(1024*1024*1024))
	} else {
		// default to just printing number of bytes
		return fmt.Sprintf("%d bytes", bcount)
	}
}
