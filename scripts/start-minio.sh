#!/bin/bash

if [[ $MINIO_ROOT_USER == "" || $MINIO_ROOT_PASSWORD == "" || $MINIO_HOST_PORT == "" ]]; then
    echo "Set MINIO_ROOT_USER, MINIO_ROOT_PASSWORD and MINIO_HOST_PORT to your"
    echo "desired admin credentials before running this script:"
    echo ""
    echo "  export MINIO_ROOT_USER=minioadmin"
    echo "  export MINIO_ROOT_PASSWORD=minioadmin"
    echo "  export MINIO_HOST_PORT=127.0.0.1:9000"
    echo "  ./scripts/start-minio.sh"
    echo ""
    exit 1
fi
/usr/local/opt/minio/bin/minio server --certs-dir ~/.minio/certs --config-dir=/usr/local/etc/minio --address=$MINIO_HOST_PORT --console-address=127.0.0.1:9001 /usr/local/var/minio

