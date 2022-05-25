#!/bin/bash

# Set to '-v' for verbose output on all commands
VERBOSE=

# Clean up from last run
rm trustlessbak-state.db
rm -rf /tmp/test-backup-src
rm -rf /tmp/test-restore-dst
./trustlessbak extras wipe-server

# Create backup source file hierarchy
mkdir -p /tmp/test-backup-src/emptydir
mkdir -p /tmp/test-backup-src/subdir1
# Test file with non-standard mode bits
echo "Hello, world!" > /tmp/test-backup-src/subdir1/file.txt
chmod 750 /tmp/test-backup-src/subdir1/file.txt
mkdir -p /tmp/test-backup-src/subdir2
dd if=/dev/random of=/tmp/test-backup-src/subdir2/bigfile.bin bs=$((1024*1024)) count=512 2>/dev/null
# Tests really long path name:
mkdir -p /tmp/test-backup-src/really/long/path/Xcode.app.Contents.Developer.Platforms.iPhoneOS.platform.Library.Developer.CoreSimulator.Profiles.Runtimes.iOS.simruntime.Contents.Resources.RuntimeRoot.System.Library.Assistant.UIPlugins.GeneralKnowledge.siriUIBundle.en_AU.lproj
echo "Small file" > /tmp/test-backup-src/really/long/path/Xcode.app.Contents.Developer.Platforms.iPhoneOS.platform.Library.Developer.CoreSimulator.Profiles.Runtimes.iOS.simruntime.Contents.Resources.RuntimeRoot.System.Library.Assistant.UIPlugins.GeneralKnowledge.siriUIBundle.en_AU.lproj/small.txt
dd if=/dev/random of=/tmp/test-backup-src/really/long/path/Xcode.app.Contents.Developer.Platforms.iPhoneOS.platform.Library.Developer.CoreSimulator.Profiles.Runtimes.iOS.simruntime.Contents.Resources.RuntimeRoot.System.Library.Assistant.UIPlugins.GeneralKnowledge.siriUIBundle.en_AU.lproj/big.bin bs=$((1024*1024)) count=130 2>/dev/null
# Tests symlinks to directories and to files
mkdir -p /tmp/test-backup-src/subdir3
CWD=`pwd`
cd /tmp/test-backup-src/subdir3
ln -s ../subdir1 subdir1link
ln -s ../subdir1/file.txt file.txt.link
cd $CWD
# Test xattrs
mkdir -p /tmp/test-backup-src/xattrs
echo "Hello" > /tmp/test-backup-src/xattrs/xattr-file
xattr -w xattr.name xattr-file /tmp/test-backup-src/xattrs/xattr-file
xattr -w xattr.name xattrs /tmp/test-backup-src/xattrs

# Backup /tmp/test-backup-src to cloud.
echo "🧪 Testing initial backup..."
./trustlessbak backup -d /tmp/test-backup-src $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

# Get the snapshot names to specify in restore
echo "🧪 Testing cloudls..."
SNAPSHOT_NAME=`./trustlessbak cloudls --grep | tail -n -1`

# Restore to /tmp/test-restore-dst/
echo "🧪 Testing restore of initial backup..."
./trustlessbak restore $SNAPSHOT_NAME /tmp/test-restore-dst/ $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

# 'diff -r' to make sure they match exactly
diff -r /tmp/test-backup-src /tmp/test-restore-dst/$SNAPSHOT_NAME
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "ERROR: source and restore directories do not match!"
    exit 1
fi

# Compare the mode bits on subdir1/file to make sure mode was set correctly
FILE_MODE_BITS_SRC=`ls -la /tmp/test-backup-src/subdir1/file.txt | cut -c 1-10`
FILE_MODE_BITS_DST=`ls -la /tmp/test-restore-dst/$SNAPSHOT_NAME/subdir1/file.txt | cut -c 1-10`
if [[ "$FILE_MODE_BITS_SRC" != "$FILE_MODE_BITS_DST" ]]; then
    echo "ERROR: mode bits do not match on subdir1/file"
    exit 1
fi

# Compare xattrs on xattrs and xattrs/xattr-file
XATTRS_FILE_SRC=`xattr -p xattr.name /tmp/test-backup-src/xattrs/xattr-file`
XATTRS_FILE_DST=`xattr -p xattr.name /tmp/test-restore-dst/$SNAPSHOT_NAME/xattrs/xattr-file`
if [[ "$XATTRS_FILE_SRC" != "$XATTRS_FILE_DST" ]]; then
    echo "ERROR: xattrs do not match on xattrs/xattr-file"
    exit 1
fi
XATTRS_DIR_SRC=`xattr -p xattr.name /tmp/test-backup-src/xattrs`
XATTRS_DIR_DST=`xattr -p xattr.name /tmp/test-restore-dst/$SNAPSHOT_NAME/xattrs`
if [[ "$XATTRS_DIR_SRC" != "$XATTRS_DIR_DST" ]]; then
    echo "ERROR: xattrs do not match on directory xattrs"
    exit 1
fi

# Now delete a file and repeat the whole test process
rm -rf /tmp/test-backup-src/subdir2

# Incremental backup of /tmp/test-backup-src
echo "🧪 Testing incremental backup with deleted paths..."
./trustlessbak backup -d /tmp/test-backup-src $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

# Get the new snapshot name to specify in next restore
SNAPSHOT_NAME=`./trustlessbak cloudls --grep | tail -n -1`

# Restore to /tmp/test-restore-dst/
echo "🧪 Testing restore of snapshot with deleted paths..."
./trustlessbak restore $SNAPSHOT_NAME /tmp/test-restore-dst/ $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

# 'diff -r' to make sure they match exactly
diff -r /tmp/test-backup-src /tmp/test-restore-dst/$SNAPSHOT_NAME
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "ERROR: source and restore directories do not match!"
    exit 1
fi

echo ""
echo "ALL TESTS SUCCEEDED!"
