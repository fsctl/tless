package cmd

import (
	"context"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/fsctl/trustlessbak/pkg/backup"
	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/spf13/cobra"
)

var (
	restoreCmd = &cobra.Command{
		Use:   "restore [name] [/restore/into/dir]",
		Short: "Restores a backup into a specified directory",
		Long: `Restores the named backup into the specified path. Usage:

trustlessbak restore [name] [/restore/into/dir]

(Use 'trustlessbak cloudls' to get the names of your stored backups.)

Example:

	trustlessbak restore Documents /Users/myname/Recovered-Documents
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			restoreMain(args[0], args[1])
		},
	}
)

func init() {
	rootCmd.AddCommand(restoreCmd)
}

func restoreMain(backupName string, pathToRestoreInto string) {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)

	// encrypt backupName argument so we can use it as a key into object list
	encryptedBackupName, err := cryptography.EncryptFilename(encKey, backupName)
	if err != nil {
		log.Fatalf("Could not compute encryption of '%s'", backupName)
	}

	// get all the encrypted objects grouped by (encrypted) backup name
	groupedObjects, err := getObjectsWithoutChunkPieces(ctx, objst, cfgBucket, true)
	if err != nil {
		log.Fatalf("Could not get grouped objects: %v", err)
	}

	// Restore each path
	re, _ := regexp.Compile(`\.`)
	if encryptedRelPaths, ok := groupedObjects[encryptedBackupName]; ok {
		for _, encryptedRelPath := range encryptedRelPaths {
			encryptedRelPathParts := re.Split(encryptedRelPath, 2)
			if len(encryptedRelPathParts) == 2 {
				// We're guaranteed to see only the .000 in this enumeration.  Other .NNN were stripped by
				// getObjectsWithoutChunkPieces.
				// We need to find all the other objects with this same rel path base name, assemble into an ordered slice,
				// and then pass the slice to RestoreDirEntryFromChunks().
				relPathChunks, err := getAllObjectsWithRelPathBaseName(ctx, objst, cfgBucket, encryptedRelPathParts[0])
				if err != nil {
					log.Printf("skipping %s because we couldn't get the other chunks", encryptedRelPath)
					continue
				}
				err = backup.RestoreDirEntryFromChunks(ctx, encKey, pathToRestoreInto, encryptedBackupName, relPathChunks, objst, cfgBucket)
				if err != nil {
					log.Printf("error: could not restore a dir entry '%s'", encryptedRelPath)
				}
			} else {
				err = backup.RestoreDirEntry(ctx, encKey, pathToRestoreInto, encryptedBackupName, encryptedRelPath, objst, cfgBucket)
				if err != nil {
					log.Printf("error: could not restore a dir entry '%s'", encryptedRelPath)
				}
			}
		}
	} else {
		log.Fatalf("No backup on server called '%s'", backupName)
	}
}

func getAllObjectsWithRelPathBaseName(ctx context.Context, objst *objstore.ObjStore, bucket string, relPathBaseName string) (relPathChunks []string, err error) {
	allObjects, err := objst.GetObjList(ctx, bucket, "")
	if err != nil {
		log.Printf("error: getAllObjectsWithRelPathBaseName: GetObjList failed: %v", err)
		return nil, err
	}

	chunks := make([]string, 0)
	for objName := range allObjects {
		if strings.Contains(objName, relPathBaseName) {
			objNameParts := strings.Split(objName, "/")
			chunks = append(chunks, objNameParts[1])
		}
	}

	sort.Strings(chunks)

	return chunks, nil
}

func getObjectsWithoutChunkPieces(ctx context.Context, objst *objstore.ObjStore, bucket string, useRawNames bool) (map[string][]string, error) {
	allObjects, err := objst.GetObjList(ctx, bucket, "")
	if err != nil {
		log.Printf("GetObjList failed: %v", err)
		return nil, err
	}

	// Iterate over the list removing all .001, .002, etc.  Keep only .000 for each chunked
	// rel path (but strip out the .000 part from the name).
	reDot, _ := regexp.Compile(`\.`)
	reDot000, _ := regexp.Compile(`\.000`)
	for objName := range allObjects {
		hasDot := reDot.FindAllString(objName, -1) != nil
		hasDot000 := reDot000.FindAllString(objName, -1) != nil
		if hasDot && !hasDot000 {
			delete(allObjects, objName)
		}
	}

	// remove the SALT-[salt string] object from allObjects since it's the
	// only file that does not fit the pattern x/y/z
	for objName := range allObjects {
		if objName[:5] == "SALT-" {
			delete(allObjects, objName)
		}
	}

	// split at slash to group by backup name
	groupedObjects := make(map[string][]string, 0)
	for objName := range allObjects {
		parts := strings.Split(objName, "/")
		backupName := parts[0]
		relPath := parts[1]

		// decrypt the two strings backupName and relPath unless raw mode was specified
		if !useRawNames {
			backupName, err = cryptography.DecryptFilename(encKey, backupName)
			if err != nil {
				log.Printf("WARNING: skipping b/c could not decrypt '%s'", backupName)
				continue
			}
			if reDot000.FindAllString(relPath, -1) != nil {
				relPath = relPath[0 : len(relPath)-4]
			}
			relPath, err = cryptography.DecryptFilename(encKey, relPath)
			if err != nil {
				log.Printf("WARNING: skipping b/c could not decrypt '%s'", relPath)
				continue
			}
		}

		groupedObjects[backupName] = append(groupedObjects[backupName], relPath)
	}

	// sort the relpath strings within each backup name group
	for groupName := range groupedObjects {
		sort.Strings(groupedObjects[groupName])
	}

	return groupedObjects, nil
}
