# Cryptography Overview

The key theme of the cryptography used here is simple and standard. No custom cryptographic constructs were invented for this program. The author has seen too many security vulnerabilities introduced when software designers, even experts, attempt to devise novel cryptographic constructs.

To ensure security of the user's data in the event of a security breach at the cloud provider, all uploaded information is first encrypted on the client.  This file explains what cryptographic algorithms are used and how they are used.

### Key Derivation

The Argon2id key derivation function is used to derive a single, 256-bit symmetric encryption key from the user's passphrase and a random (but not secret) salt.  Passphrases are encouraged to have at least 128 bits of entropy, and the program will recommend a randomly generated passphrase meeting this requirement when first run.  The program will also generate a randomly generated 32 ASCII character key derivation salt.

The randomly generated salt is a 32-byte ASCII string consisting of the characters \[A-Za-z0-9\].  This process can generate approximately 10^57 possible salts, which is adequate since the purpose of the salt is to provide global uniqueness, not additional secrecy. It is stored plainly in the bucket's `metadata` file.

The passphrase-derived key is used to decrypt two keys that are stored encrypted (AES-256-GCM) in the S3 bucket's `metadata` file: the encryption key and the HMAC key. The encryption key is used to encrypt all file contents and metadata like filenames as described below. (The HMAC key is not presently used.) Both keys are initially generated randomly on the client machine using 32 bytes (each) from Go's CSPRNG. They are never written to the bucket in unencrypted form.

### Encryption of File Contents and Metadata

File contents and metadata (like permissions and xattrs) are concatenated into a single stream and encrypted on the client using AES-256-GCM with no additional data. Snapshot index files, which store the file names, are encrypted the same way. (See [on-bucket layout](bucket-layout.md) for further details on snapshot index files.)

The AES-GCM implementation is from the Go standard library [crypto/cipher](https://pkg.go.dev/crypto/cipher) and [crypto/aes](https://pkg.go.dev/crypto/aes) packages.  This is a non-constant time AES (and GHASH) implementation.

### Encryption of Backup Names

Backup names need to encrypt deterministically so that they can be used as parent "directories" for all the snapshot index files created in that backup. Therefore, AES-GCM-SIV with the same 256-bit key is used to encrypt backup names.

(AES-GCM-SIV is a mode of encryption that does not leak information when the same nonce is reused with the same key.  In order to preserve the mapping of backup name plaintext to cloud object store names, the same nonce is always used for a given backup.)

Obviously, this leaks the fact that a particular encrypted string corresponds to _some_ unchanging backup name, but this alone is unlikely to be useful to an attacker since (1) it reveals nothing about the backup name (which is probably just some word like "Documents" or "home"; (2) it is assumed that an attacker already knows you are using this program for making backups.

The AES-GCM-SIV implementation is the [siv-go](github.com/secure-io/siv-go) open source library.  It is not a constant time implementation.

### Passphrase Recommendation

When the program first starts, if no config.toml file exists, the program will generate an example config file that the user can fill in with the required values.  Each time an example file like this is generated, a new randomly generated passphrase is embedded in it.  This is intended as a suggestion to the user to employ a strong passphrase, but the user is of course free to override this suggestion.

The randomly generated passphrase is a 10-word Diceware passphrase.  Since each word is drawn randomly from a list of 7,776 words by a CSPRNG, a 10-word passphrase has entropy of 10*log2(7776) ~= 129 bits, which should be secure against brute force cracking until at least the middle of this century.
