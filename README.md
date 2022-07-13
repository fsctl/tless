# tless

Cloud backup for people who don't trust cloud provider security.  Features:

 - Cloud backup utility that encrypts files (and filenames) entirely on the client
 - No plaintext is ever seen by the cloud provider server that you're backing up to
 - Incremental backups
 - Snapshots
 - Compression
 - Preserves xattrs, file mode, symlinks
 - Clear security guarantees (see [SECURITY.md](docs/SECURITY.md))

There are plenty of cloud-based backup solutions, many with more elaborate features than this one.  However, I do not know of any other utility like this that leaks zero unencrypted information to the cloud provider.  This means that if your cloud provider is hacked or your cloud data is otherwise leaked onto the Internet, you can have strong confidence that it will not be readable by anyone who does not know your passphrase.  

If you follow the [security recommendations](docs/SECURITY.md), your data is likely to be secure through at least the middle of this century.

## Installation & Usage

### Prerequisites

#### 1.  Set up a bucket on an S3-compatible object store and get read-write access credentials for it

The simplest path is to use an AWS or Digital Ocean account that has access to S3.  Just create a bucket and a Access Key Id + Secret Access Key pair that can read/write the bucket.  Here's a [tutorial on setting up S3 on Digital Ocean](docs/DO-S3-TUTORIAL.md).

For testing on your LAN, I recommend running [Minio](https://docs.min.io/docs/minio-quickstart-guide.html) on your local machine.  Make sure to enable TLS.  You can use a self-signed certificate, but the Minio documentation for how to make this work is sparse, so see my [instructions here](docs/MINIO-TLS.md).

Whatever object store you use, make note of these pieces of information:

 - `host:port` for your S3-compatible object store
 - Access Key Id and Secret Access Key for your object store acccount
 - The name of the empty bucket, which you should create in advance
 
#### 2.  Install these prerequisites to be able to compile

 - [Protocol Buffer Compiler (protoc)](https://grpc.io/docs/protoc-installation/)
 - The Go Protocol Buffer and GRPC plugins:

```
$ go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
$ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
```

Make sure the plugins are in your PATH so `protoc` can find them:

```
    export PATH="$PATH:$(go env GOPATH)/bin"
```

### Setting Up `tless`

#### 1.  Clone this repo and run `make` to build the executable

```
make
```

Note: Go 1.18 or higher is required.

#### 2.  Run the program as `tless backup` to generate a template config file

It will be in `$HOME/.tless/config.toml`

#### 3.  Edit the config file, following the instructions in the comments

This is where you provide your object store endpoint, bucket name and credentials.  If you are using a self-signed TLS certificate (rather than a commercial S3-compatible provider like AWS or Digital Ocean), make sure that `trust_self_signed_certs` is set to `true`.

A high-entropy Diceware password is generated for you, though you can change it if you like.  

The config file also specifies what directory tree(s) to back up.  For example, you may want to back up `/home/<your username>` on Linux or `/Users/<your username>/Documents` on macOS.

####  4.  Test your config file 

```
tless extras check-conn
``` 

This command will tell you whether your object store is reachable.

####  5.  Run your first backup

Run this command to create a backup:

```
tless backup
```

Change (or `touch`) some files and run that command again to create an incremental snapshot.

#### 6.  Use `tless cloudls` to see the snapshots you have accumulated on your cloud server

```
tless cloudls
```

Snapshots are all named after the directory being backed up and the time the snapshot was made.  Pick one to restore from the list and run:

```
tless restore Documents/2022-05-22_11:52:01 /tmp/restore-here
```

You should get all your files back in `/tmp/restore-here`.

#### 7.  Connect to your bucket via its web interface and observe that everything is encrypted

This includes file and directory names, file metadata and file contents.
