# Security

This system is designed for a threat model in which you want to back up a folder on a local machine to an AWS S3-compatible cloud provider, but once you do so you have no control over (or visibility into) what happens to the data.  It could be exfiltrated due to a hack of the cloud provider, or due to a malicious employee of the cloud provider, or other scenarios.  Once that happens, the data could be subject to brute force decryption attempts for years or decades, depending on how long the data is valuable to an attacker.

## Recommendations

Considering these factors, I recommend:

 - Use a 10-word Diceware passphrase.  This will give you a passphrase with about 129 bits of entropy, which should be resistant to brute force decryption well into the middle of this century.

 - Try to use as secure and trustworthy of a cloud provider as you can.  While this program aims to prevent against even insider attacks by disclosing nothing to the cloud provider, you're still better off if your data is not exfiltrated in the first place.

 - If your data is so sensitive that even a small chance of disclosure is unacceptable, consider not putting it in the cloud in the first place.  No system can be guaranteed to be impenetrable.

## What Information Is Protected or Revealed

Here is what is protected:

 - All of the metadata about a file, including file (and directory) names, modification times, and extended attributes are serialized, encrypted client side and stored in the file stream.
 - The actual file contents are client side encrypted.

 Here is what is visible to an attacker who can obtain a one-time clone of your S3 bucket:

 - Upload times (which correlate with correspond with file change times)

 - Approximate total size of each file
 
 - The key derivation salt (see Cryptography Design below) is stored in plaintext on the server since it is not intended to be kept secret nor memorized.
 
 - Because of the incremental nature of the backup process, it will be clear if one file is changing regularly over time (growing, being modified), though obviously what that file is will not be knowable.

## Cryptographic Design

 - Argon2id is used to derive a 256-bit AES key from your passphrase and a non-secret, randomly-generated 32-byte salt.

 - AES-GCM-256 is used to encrypt file contents, including the metadata that is prepended to the file stream before encryption.

 - AES-GCM-256-SIV is used to deterministically encrypt file and directory names. This is done by always using an all-zero nonce, and SIV mode was chosen specifically because it makes security guarantees even when nonces are reused. In this case, the same local path (like dir1/subdir1/myfile.txt) will always encrypt to the same object name on the server.