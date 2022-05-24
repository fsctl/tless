# trustlessbak

Cloud backup for people who don't trust cloud provider security.  Features:

 - Cloud backup utility that encrypts files (and filenames) entirely on the client
 - No plaintext is ever seen by the cloud provider server that you're backing up to
 - Incremental backups
 - Snapshots
 - Preserves xattrs (extended attributes)
 - Clear security guarantees (see [SECURITY.md](blob/main/SECURITY.md))

There are plenty of cloud-based backup solutions, many with more elaborate features than this one.  However, I do not know of any other utility like this that leaks zero unencrypted information to the cloud provider.  This means that if your cloud provider is hacked or your cloud data is otherwise leaked onto the Internet, you can have strong confidence that it will not be readable by anyone who does not know your passphrase.  

If you follow the [security recommendations](blob/main/SECURITY.md), your data is likely to be secure through at least the middle of this century.

## Installation & Usage

1.  Set up a bucket on an S3-compatible blob store and get read-write access credentials for it.  For testing, I recommend running [Minio](https://docs.min.io/docs/minio-quickstart-guide.html) on your local machine.

Whatever blobstore you use, make note of these pieces of information:

 - `host:port` for your S3-compatible blobstore
 - Access Key and Secret Key for your blobstore acccount
 - The name of the empty bucket, which you should create in advance

2.  Clone this repo and run `make` to build the executable

Note: Go 1.18 or higher is required

3.  Run the program as `trustlessbak backup` to generate a template config file in `$HOME/.trustlessbak/config.toml`

4.  Edit the config file, following the instructions in the comments.  This is where you provide your blobstore credentials.  A high-entropy Diceware password and a strong salt are generated for you, though you can change them if you like.  

The config file also specifies what directory tree(s) to back up, e.g., `/home/<your name>` on Linux or `/Users/<your name>/Documents` on macOS.

5.  Test your config file by running `trustlessbak check-conn`. This will report on whether your blob store is reachable.

6.  Run your first backup:  `trustlessbak backup`.  Change (or `touch`) some files and run that command again to create an incremental snapshot.

7.  Use `trustlessbak cloudls` to see the snapshots you have accumulated on your cloud server.  They are all named after the directory being backed up and the time the snapshot was made.  Pick one to restore from the list and run:

```
trustlessbak restore Documents/2022-05-22_11:52:01 /tmp/restore-here
```

8.  Connect to your bucket via its web interface and observe that everything is encrypted:  file and directory names, file metadata, file contents.

