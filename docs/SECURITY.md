# Security

This system is designed for a threat model in which you want to back up a folder on a local machine to an AWS S3-compatible cloud provider, but once you do so you have no control over (or visibility into) what happens to the data.  You're concerned that the cloud provider could be hacked, and your data subject to brute force decryption attempts for years or even decades.

## Recommendations

Considering these factors, I recommend:

 - Use a 10-word Diceware passphrase.  This will give you a passphrase with about 129 bits of entropy, which should be resistant to brute force decryption well into the middle of this century.

 - Try to use as secure and trustworthy of a cloud provider as you can.  While this program aims to prevent against the scenario where your cloud provider is hacked and your (encrypted) data is leaked to the public Internet, you're still better off with a provider where this is unlikely to happen in the first place.

## What Information Is Protected or Revealed

Here is what is protected:

 - All of the metadata about a file, including file (and directory) names, modification times, permissions and extended attributes are serialized, encrypted client side and stored in the file stream.
 
 - The actual file contents are client side encrypted.

 Here is what is visible to an attacker who can obtain a one-time clone of your S3 bucket:

 - Upload times (which correlate with file change times, though an attacker cannot learn which file changed)

 - Approximate total size of each file
 
 - The key derivation salt (see Cryptography Design below) is stored in plaintext on the server since it is not intended to be kept secret nor memorized.
 
 - Because of the incremental nature of the backup process, it will be clear if some file is changing regularly over time (growing, being modified), though obviously what that file is and what the changes are will not be discoverable.

## Cryptographic Design

See [CRYPTOGRAPHY.md](docs/CRYPTOGRAPHY.md).