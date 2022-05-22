package cmd

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/fsctl/trustlessbak/pkg/cryptography"
	"github.com/fsctl/trustlessbak/pkg/objstore"
	"github.com/spf13/cobra"
)

var (
	// Command
	cloudlsCmd = &cobra.Command{
		Use:   "cloudls",
		Short: "Lists all data stored in cloud",
		Long: `Lists everything you have stored in the cloud. File and directory names are automatically
decrypted in the display.

Example:

	trustlessbak cloudls
`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			cloudlsMain()
		},
	}
)

func init() {
	rootCmd.AddCommand(cloudlsCmd)
}

func cloudlsMain() {
	ctx := context.Background()
	objst := objstore.NewObjStore(ctx, cfgEndpoint, cfgAccessKeyId, cfgSecretAccessKey)

	groupedObjects, err := getGroupedObjects(ctx, objst, cfgBucket)
	if err != nil {
		log.Fatalf("Could not get grouped objects: %v", err)
	}

	// print out each backup name group
	if len(groupedObjects) == 0 {
		fmt.Println("No objects found in cloud")
	}
	for groupName := range groupedObjects {
		fmt.Printf("Backup '%s':\n", groupName)
		for _, relPath := range groupedObjects[groupName] {
			fmt.Printf("  %s\n", relPath)
		}
	}
}

func getGroupedObjects(ctx context.Context, objst *objstore.ObjStore, bucket string) (map[string][]string, error) {
	allObjects, err := objst.GetObjList(ctx, bucket, "")
	if err != nil {
		log.Printf("GetObjList failed: %v", err)
		return nil, err
	}

	reDot, _ := regexp.Compile(`\.`)
	reDot000, _ := regexp.Compile(`\.000`)

	// Iterate over the list removing all .001, .002, etc.  Keep only .000 for each chunked
	// rel path (but strip out the .000 part from the name).
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

		groupedObjects[backupName] = append(groupedObjects[backupName], relPath)
	}

	// sort the relpath strings within each backup name group
	for groupName := range groupedObjects {
		sort.Strings(groupedObjects[groupName])
	}

	return groupedObjects, nil
}
