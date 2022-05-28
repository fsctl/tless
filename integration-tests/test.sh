#!/bin/bash

#
# Set this to '-v' for verbose output on all commands
#
VERBOSE=

#
# Can't use tmpfs on Linux because it does not allow user-defined xattrs
#
UNAME=`uname`
if [[ $UNAME == "Linux" ]]; then
    TEMPDIR=/home/mike/temp
elif [[ $UNAME == "Darwin" ]]; then
    TEMPDIR=/tmp
else
    # Unknown OS, try /tmp
    TEMPDIR=/tmp
fi

#
# Clean up from last run
#
rm trustlessbak-state.db
rm -rf $TEMPDIR/test-backup-src
rm -rf $TEMPDIR/test-restore-dst
./trustlessbak extras wipe-server --force
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test because could not wipe server (exit code $EXITCODE)"
    exit 1
fi

#
# Create backup source file hierarchy
#
mkdir -p $TEMPDIR/test-backup-src/emptydir
mkdir -p $TEMPDIR/test-backup-src/subdir1
# Test file with non-standard mode bits
echo "Hello, world!" > $TEMPDIR/test-backup-src/subdir1/file.txt
chmod 750 $TEMPDIR/test-backup-src/subdir1/file.txt
mkdir -p $TEMPDIR/test-backup-src/subdir2
dd if=/dev/urandom of=$TEMPDIR/test-backup-src/subdir2/bigfile.bin bs=$((1024*1024)) count=256 2>/dev/null
# Tests really long path name:
mkdir -p $TEMPDIR/test-backup-src/really/long/path/Xcode.app.Contents.Developer.Platforms.iPhoneOS.platform.Library.Developer.CoreSimulator.Profiles.Runtimes.iOS.simruntime.Contents.Resources.RuntimeRoot.System.Library.Assistant.UIPlugins.GeneralKnowledge.siriUIBundle.en_AU.lproj
echo "Small file" > $TEMPDIR/test-backup-src/really/long/path/Xcode.app.Contents.Developer.Platforms.iPhoneOS.platform.Library.Developer.CoreSimulator.Profiles.Runtimes.iOS.simruntime.Contents.Resources.RuntimeRoot.System.Library.Assistant.UIPlugins.GeneralKnowledge.siriUIBundle.en_AU.lproj/small.txt
dd if=/dev/urandom of=$TEMPDIR/test-backup-src/really/long/path/Xcode.app.Contents.Developer.Platforms.iPhoneOS.platform.Library.Developer.CoreSimulator.Profiles.Runtimes.iOS.simruntime.Contents.Resources.RuntimeRoot.System.Library.Assistant.UIPlugins.GeneralKnowledge.siriUIBundle.en_AU.lproj/big.bin bs=$((1024*1024)) count=130 2>/dev/null
# Tests symlinks to directories and to files
mkdir -p $TEMPDIR/test-backup-src/subdir3
CWD=`pwd`
cd $TEMPDIR/test-backup-src/subdir3
ln -s ../subdir1 subdir1link
ln -s ../subdir1/file.txt file.txt.link
cd $CWD
# Test xattrs
mkdir -p $TEMPDIR/test-backup-src/xattrs
echo "Hello" > $TEMPDIR/test-backup-src/xattrs/xattr-file
xattr -w user.xattr-name xattr-file $TEMPDIR/test-backup-src/xattrs/xattr-file
xattr -w user.xattr-name xattrs $TEMPDIR/test-backup-src/xattrs

#
# Backup $TEMPDIR/test-backup-src to cloud.
#
echo "🧪 Testing initial backup..."
./trustlessbak backup -d $TEMPDIR/test-backup-src $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

#
# Get the snapshot names to specify in restore
#
echo "🧪 Testing cloudls..."
SNAPSHOT_NAME=`./trustlessbak cloudls --grep | tail -n -1`

#
# Partial restores to $TEMPDIR/test-restore-dst/
#
echo "🧪 Testing partial dir restore of initial backup..."
./trustlessbak restore $SNAPSHOT_NAME $TEMPDIR/test-restore-dst/ $VERBOSE --partial emptydir
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test due to partial restore failure exit code ($EXITCODE)"
    exit 1
fi
F=`find $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME -type f | wc -l | sed -e 's/ //g'`
D=`find $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME -type d | wc -l | sed -e 's/ //g'`
if [[ $F != 0 ]]; then
    echo "$0: ERROR: got wrong number of dir entries on count of $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME"
    echo "$0: (expected 0 files, got $F files)"
    exit 1
fi
if [[ $D != 2 ]]; then
    echo "$0: ERROR: got wrong number of dirs on count of $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME"
    echo "$0: (expected 2 dirs, got $D dirs)"
    exit 1
fi
rm -rf $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME

echo "🧪 Testing single file partial restore of initial backup..."
./trustlessbak restore $SNAPSHOT_NAME $TEMPDIR/test-restore-dst/ $VERBOSE --partial subdir1/file.txt
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test due to partial restore failure exit code ($EXITCODE)"
    exit 1
fi
F=`find $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME -type f | wc -l | sed -e 's/ //g'`
D=`find $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME -type d | wc -l | sed -e 's/ //g'`
if [[ $F != 1 ]]; then
    echo "$0: ERROR: got wrong number of dir entries on count of $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME"
    echo "$0: (expected 1 file, got $F files)"
    exit 1
fi
if [[ $D != 2 ]]; then
    echo "$0: ERROR: got wrong number of dirs on count of $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME"
    echo "$0: (expected 2 dirs, got $D dirs)"
    exit 1
fi
rm -rf $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME

#
# Full restore to $TEMPDIR/test-restore-dst/
#
echo "🧪 Testing full restore of initial backup..."
./trustlessbak restore $SNAPSHOT_NAME $TEMPDIR/test-restore-dst/ $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

#
# 'diff -r' to make sure they match exactly
#
diff -r $TEMPDIR/test-backup-src $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: ERROR: source and restore directories do not match!"
    exit 1
fi

#
# Compare the mode bits on subdir1/file to make sure mode was set correctly
#
FILE_MODE_BITS_SRC=`ls -la $TEMPDIR/test-backup-src/subdir1/file.txt | cut -c 1-10`
FILE_MODE_BITS_DST=`ls -la $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME/subdir1/file.txt | cut -c 1-10`
if [[ "$FILE_MODE_BITS_SRC" != "$FILE_MODE_BITS_DST" ]]; then
    echo "$0: ERROR: mode bits do not match on subdir1/file"
    exit 1
fi

#
# Compare xattrs on xattrs and xattrs/xattr-file
#
XATTRS_FILE_SRC=`xattr -p user.xattr-name $TEMPDIR/test-backup-src/xattrs/xattr-file`
XATTRS_FILE_DST=`xattr -p user.xattr-name $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME/xattrs/xattr-file`
if [[ "$XATTRS_FILE_SRC" != "$XATTRS_FILE_DST" ]]; then
    echo "$0: ERROR: xattrs do not match on xattrs/xattr-file"
    exit 1
fi
XATTRS_DIR_SRC=`xattr -p user.xattr-name $TEMPDIR/test-backup-src/xattrs`
XATTRS_DIR_DST=`xattr -p user.xattr-name $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME/xattrs`
if [[ "$XATTRS_DIR_SRC" != "$XATTRS_DIR_DST" ]]; then
    echo "$0: ERROR: xattrs do not match on directory xattrs"
    exit 1
fi

#
# Now delete a file and repeat the whole test process
#
rm -rf $TEMPDIR/test-backup-src/subdir2

#
# Incremental backup of $TEMPDIR/test-backup-src
#
echo "🧪 Testing incremental backup with deleted paths..."
./trustlessbak backup -d $TEMPDIR/test-backup-src $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

#
# Get the new snapshot name to specify in next restore
#
SNAPSHOT_NAME=`./trustlessbak cloudls --grep | tail -n -1`

#
# Restore to $TEMPDIR/test-restore-dst/
#
echo "🧪 Testing restore of snapshot with deleted paths..."
./trustlessbak restore $SNAPSHOT_NAME $TEMPDIR/test-restore-dst/ $VERBOSE
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: Halting test due to failure exit code ($EXITCODE)"
    exit 1
fi

#
# 'diff -r' to make sure they match exactly
#
diff -r $TEMPDIR/test-backup-src $TEMPDIR/test-restore-dst/$SNAPSHOT_NAME
EXITCODE=$?
if [[ $EXITCODE != 0 ]]; then
    echo "$0: ERROR: source and restore directories do not match!"
    exit 1
fi

echo ""
echo "ALL TESTS SUCCEEDED!"
