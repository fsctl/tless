# Security

This system is designed for a threat model in which you want to back up a folder on a local machine to an AWS S3-compatible cloud provider, but once you do so you have no control over (or visibility into) what happens to the data.  You're concerned that the cloud provider could be hacked, and your data subject to brute force decryption attempts.

## Recommendations

Considering these factors, I recommend:

 - Use a 10-word Diceware passphrase.  This will give you a passphrase with about 129 bits of entropy, which should be resistant to brute force decryption well into the middle of this century.

 - Try to use as secure and trustworthy of a cloud provider as you can.  While this program aims to prevent against the scenario where your cloud provider is hacked and your (encrypted) data is leaked to the public Internet, you're still better off with a provider where this is unlikely to happen in the first place.

## What Information Is Protected or Revealed

Here is what is encrypted client-side and protected against disclosure:

 - The contents of each file.

 - All of the metadata about a file, including file (and directory) names, file size, modification times, permissions and extended attributes.

 - The names of your backups and snapshots.

Here is what is visible to an attacker who can obtain a one-time clone of your S3 bucket:

 - The times snapshots were made.

 - Encrypted representations of your encryption key and HMAC key.  See [cryptographic design](CRYPTOGRAPHY.md) document.

 - The key derivation salt (see Cryptography Design below) is stored in plaintext on the server since it is not intended to be kept secret nor memorized.

## Cryptographic Design

See [CRYPTOGRAPHY.md](CRYPTOGRAPHY.md).