#!/bin/bash

if [[ $MINIO_ROOT_USER == "" || $MINIO_ROOT_PASSWORD == "" ]]; then
    echo "Set MINIO_ROOT_USER and MINIO_ROOT_PASSWORD to your desired admin credentials"
    echo "before running this script:"
    echo ""
    echo "  export MINIO_ROOT_USER=minioadmin"
    echo "  export MINIO_ROOT_PASSWORD=minioadmin"
    echo "  ./scripts/start-minio.sh"
    echo ""
    exit 1
fi
/usr/local/opt/minio/bin/minio server --config-dir=/usr/local/etc/minio --address=127.0.0.1:9000 --console-address=127.0.0.1:9001 /usr/local/var/minio

