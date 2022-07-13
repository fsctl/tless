# Setting Up TLS on Minio

You can use `tless` without setting up your own object store.  For instance, just use AWS S3.  However, if you want to run the open source object store Minio for yourself, you will need to configure it with TLS.

The basic configuration of Minio is covered in their [guide](https://docs.min.io/docs/minio-quickstart-guide.html).  However, the explanation for how to set up TLS with a self-signed certificate is a little sparse.  Here are the steps I followed to achieve it.

1.  Download their program [certgen](https://github.com/minio/certgen) for your platform.  This is a simple Go program that generates self-signed certificates.

2.  For convenience, make sure the downloaded binary is somewhere in your PATH.  We'll assume you saved it in `/usr/local/bin`.

3.  Make it executable:

```
$ chmod +x /usr/local/bin/certgen-darwin-amd64
```

(The exact filename will vary if you downloaded the binary for a different platform.)

4.  MacOS users only:  in order to run an unsigned binary, you need to remove the quarantine extended attributes on the binary.

```
$ xattr -d com.apple.metadata:kMDItemWhereFroms /usr/local/bin/certgen-darwin-amd64
$ xattr -d com.apple.quarantine /usr/local/bin/certgen-darwin-amd64
```

5.  Change into the `$HOME/.minio/certs` directory.  If it doesn't exist, create it.  This should be in the $HOME of the user who will run the Minio server; probably your normal interactive user account.

6.  Run `certgen`:

```
$ certgen-darwin-amd64 -no-ca -host "127.0.0.1,10.0.0.9"
```

Again, your binary will be called something different from `certgen-darwin-amd64` unless you are also running on Intel macOS.  And for the `-host` argument, put whatever hostname(s) you plan to use to access your Minio instance.  In my case, I want to access it as localhost (127.0.0.1) and at a fixed IP on my LAN (10.0.0.9).

7.  Make sure the two generated files (`private.key` and `public.crt`) are owned by the user that is going to run minio, e.g.,

```
$ chown <yourusername> ~/.minio/certs/*
```

8.  Refer to [`scripts/start-minio.sh`](../scripts/start-minio.sh) in this repo to see how to start the Minio server pointed at the correct certs directory.