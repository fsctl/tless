#!/bin/bash

# Clean up from last run
rm trustlessbak-state.db
rm -rf /tmp/test-restore

# Backup ~/Documents/backedup and ~/Documents/backedup2 to cloud
# Those dirs are specified in config.toml.
./trustlessbak backup
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

# Restore to /tmp/test-restore/backedup and /tmp/test-restore/backedup2
./trustlessbak restore backedup /tmp/test-restore/
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

./trustlessbak restore backedup2 /tmp/test-restore/
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

# 'diff -r' to make sure they match exactly
diff -r ~/Documents/backedup /tmp/test-restore/backedup
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "ERROR: source and restore directories do not match! (/tmp/test-restore/backedup)"
    exit 1
fi

diff -r ~/Documents/backedup2 /tmp/test-restore/backedup2
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "ERROR: source and restore directories do not match! (/tmp/test-restore/backedup2)"
    exit 1
fi

echo ""
echo "TEST SUCCEEDED!"
