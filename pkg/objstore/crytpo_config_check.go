package objstore

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/fsctl/tless/pkg/cryptography"
	"github.com/fsctl/tless/pkg/util"
)

var (
	ErrMultipleSalts       = errors.New("there is more than one SALT-xxxx file on server")
	ErrCantDecryptSaltFile = errors.New("the salt file could not be decrypted with your master password")
	ErrNoSaltOnServer      = errors.New("there is no salt file on server")
	ErrMismatchedSalts     = errors.New("the local salt does not match the salt saved on cloud server")
)

// Here is the basic logic of the server SALT-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx file:
//
// - The SALT-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx file should have encrypted AES-GCM contents equal
// to some random bytes. Their value is not important.
// - On startup, try to retrieve all SALT-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx files
//     - If there's more than one, warn user of multiple salts.
//     - If there's exactly one, make sure the salt string matches the config
//         - If not, warn the user that there's a mismatch between the config and the server.
//		     Only a --force flag can allow the requested operation to proceed.
//         - If yes, download that one, matching SALT-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx file and
//         try to decrypt it with master password.
//             - If it does not decrypt+authenticate, warn the user that password has changed
//			     in config file.  Only a --force flag can allow the requested operation to proceed.
//             - If it does decrypt and authenticate, then everything is ok between config file
//			     and obj store.  Proceed as planned, no warnings.
func (os *ObjStore) CheckCryptoConfigMatchesServer(ctx context.Context, key []byte, bucket string, expectedSalt string, isVerbose bool) bool {
	// Try to read the salt, warning user on common failure cases ErrMultipleSalts and
	// ErrCantDecryptSaltFile.  On ErrNoSaltOnServer, try to save the config salt to the server.
	salt, err := os.tryReadSalt(ctx, key, bucket, isVerbose, nil)
	if err != nil {
		if errors.Is(err, ErrMultipleSalts) {
			fmt.Println(`warning: there are multiple SALT-xxxx files in the bucket. You need to delete
the wrong one(s) manually. Use --force to override this check and use the salt
in your config.toml.`)
			return false
		} else if errors.Is(err, ErrCantDecryptSaltFile) {
			fmt.Println(`warning: cannot decrypt the SALT-xxxx files in the bucket. Did you change your
master password in config.toml? Note: use --force to override this check and 
use the master password in your config.toml, but doing so may corrupt the 
backup.`)
			return false
		} else if errors.Is(err, ErrNoSaltOnServer) {
			if isVerbose {
				fmt.Println("warning: no SALT-xxxx file on server; saving the salt from your config.toml")
			}
			if err = os.tryWriteSalt(ctx, key, bucket, expectedSalt); err != nil {
				log.Printf("warning: failed to write salt to server for backup: %v\n", err)
			}
			return true
		}
	} else {
		// Warn if salt on server != salt in config file
		if salt != expectedSalt {
			log.Printf("warning: local salt =/= server saved salt; --force to use local config salt\n('%s') and ignore server salt\n('%s')\n", expectedSalt, salt)
			return false
		}
	}

	return true
}

// Same as CheckCryptoConfigMatchesServer() except printing and error return designed for daemon mode
func (os *ObjStore) CheckCryptoConfigMatchesServerDaemon(ctx context.Context, key []byte, bucket string, expectedSalt string, vlog *util.VLog) (bool, error) {
	salt, err := os.tryReadSalt(ctx, key, bucket, false, vlog)
	if err != nil {
		if errors.Is(err, ErrMultipleSalts) {
			log.Println(`warning: there are multiple SALT-xxxx files in the bucket. You need to delete
the wrong one(s) manually.`)
			return false, err
		} else if errors.Is(err, ErrCantDecryptSaltFile) {
			log.Println(`warning: cannot decrypt the SALT-xxxx files in the bucket. Did you change your
master password in config.toml?`)
			return false, err
		} else if errors.Is(err, ErrNoSaltOnServer) {
			log.Println("warning: no SALT-xxxx file on server; saving the salt from your config.toml")
			if err = os.tryWriteSalt(ctx, key, bucket, expectedSalt); err != nil {
				log.Printf("warning: failed to write salt to server for backup: %v\n", err)
			}
			return true, nil
		}
	} else {
		// Warn if salt on server != salt in config file
		if salt != expectedSalt {
			log.Printf("warning: local salt ('%s') =/= server saved salt ('%s')", expectedSalt, salt)
			return false, ErrMismatchedSalts
		}
	}

	return true, nil
}

func (os *ObjStore) tryReadSalt(ctx context.Context, key []byte, bucket string, isVerbose bool, vlog *util.VLog) (string, error) {
	// Try to fetch all objects starting with "SALT-"
	m, err := os.GetObjList(ctx, bucket, "SALT-", vlog)
	if err != nil {
		return "", err
	}

	// Check if there's >1 SALT-xxxx file and warn user if so
	var saltObjName string
	if len(m) > 1 {
		for k := range m {
			msg := fmt.Sprintf("warning: found salt: '%s' in bucket\n", k)
			if vlog != nil {
				vlog.Println(msg)
			} else {
				log.Println(msg)
			}
		}
		log.Printf("warning: there are multiple SALT-xxxx files on the server; you need to manually delete the wrong one(s)")
		return "", ErrMultipleSalts
	} else if len(m) == 0 {
		// There is no SALT-xxxx file.
		return "", ErrNoSaltOnServer
	} else {
		// There is only one salt; get its value
		for k := range m {
			saltObjName = k
			msg := fmt.Sprintf("found salt file '%s' in bucket\n", saltObjName)
			if isVerbose {
				fmt.Println(msg)
			}
			if vlog != nil {
				vlog.Println(msg)
			}
		}
	}

	// Try to decrypt the salt file with specified master password
	saltFileContents, err := os.DownloadObjToBuffer(ctx, bucket, saltObjName)
	if err != nil {
		return "", err
	}
	_, err = cryptography.DecryptBuffer(key, saltFileContents)
	if err != nil {
		return "", ErrCantDecryptSaltFile
	}

	// Return the salt and no error
	salt := strings.TrimPrefix(saltObjName, "SALT-")
	return salt, nil
}

func (os *ObjStore) tryWriteSalt(ctx context.Context, key []byte, bucket string, salt string) error {
	// We fill the salt file with Enc(8 random bytes).  The bytes are not important since we
	// validate using the AES-GCM tag, not by checking the bytes themselves.
	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		log.Printf("error: could not generate random bytes")
		return err
	}

	ciphertextBuf, err := cryptography.EncryptBuffer(key, buf)
	if err != nil {
		log.Printf("error: could not encrypt empty buffer")
		return err
	}

	err = os.UploadObjFromBuffer(ctx, bucket, "SALT-"+salt, ciphertextBuf, ComputeETag(ciphertextBuf))
	return err
}
