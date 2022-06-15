# # Cryptography Overview

To ensure security of the user's data in the event of a security breach at the cloud provider, all uploaded information is first encrypted on the client.  This file explains what cryptographic algorithms are used and how they are used.

### Key Derivation

The Argon2id key derivation function is used to derive a single, 256-bit symmetric encryption key from the user's passphrase and a random (but not secret) salt.  Passphrases are recommended to have at least 128 bits of entropy, and the program will recommend a randomly generated passphrase meeting this requirement when first run.  The program will also recommend a randomly generated 32 ASCII character salt.  (See _Passphrase and Salt Recommendations_ below.)

### Encryption of File Contents and Metadata

File contents and metadata (like permissions and xattrs) are concatenated into a single stream and encrypted on the client using AES-GCM with a 256-bit key.  The key is derived as described above.

The AES-GCM implementation is from the Go standard library [crypto/cipher](https://pkg.go.dev/crypto/cipher) and [crypto/aes](https://pkg.go.dev/crypto/aes) packages.  This is not a constant time AES (or GHASH) implementation.

### Encryption of File and Directory Names

File and directory names are encrypted on the client using AES-GCM-SIV with a 256-bit key.  This key is the same as the one used to encrypt the file contents and is derived as described above.

AES-GCM-SIV is used in this application because it does not leak information when the same nonce is reused with the same key.  In order to preserve the mapping of plaintext file names to cloud object store keys, the same nonce is always used for a given file.

(Obviously, this leaks the fact that a particular encrypted string corresponds to _some_ unchanging filesystem path name if it appears in multiple snapshots, but this alone is unlikely to be useful to an attacker since there's still no way to know what the plaintext filesystem path is.  At best the attacker can say that there exists some unknown file that is periodically edited.)

The AES-GCM-SIV implementation is the [siv-go](github.com/secure-io/siv-go) open source library.  It is not a constant time implementation.

### Passphrase and Salt Recommendations

When the program first starts, if no config.toml file exists, the program will generate an example config file that the user can fill in with the required values.  Each time an example file like this is generated, a new randomly generated passphrase and salt string are embedded in it.  These are intended as suggestions to the user to employ a strong passphrase and unique salt, but the user is of course free to override these suggested values.

The randomly generated passphrase is a 10-word Diceware passphrase.  Since each word is drawn randomly from a list of 7,776 words by a CSPRNG, a 10-word passphrase has entropy of 10*log2(7776) ~= 129 bits, which should be secure against brute force cracking until at least the middle of this century.

The randomly generated salt is a 32-byte ASCII string consisting of the characters \[A-Za-z0-9\].  This process can generate approximately 10^57 possible salts, which is adequate since the purpose of the salt is to provide global uniqueness, not additional secrecy.