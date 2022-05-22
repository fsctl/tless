#!/bin/bash

export MINIO_ROOT_USER=minioadmin
export MINIO_ROOT_PASSWORD=rootroot
/usr/local/opt/minio/bin/minio server --config-dir=/usr/local/etc/minio --address=127.0.0.1:9000 --console-address=127.0.0.1:9001 /usr/local/var/minio

