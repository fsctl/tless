package backup

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/pkg/xattr"
)

func serializeXAttrsToHex(path string) (string, error) {
	xattrHex := ""

	var err error
	var list []string
	if list, err = xattr.List(path); err != nil {
		log.Println("could not list xattrs: ", err)
		return "", err
	}

	for _, xa := range list {
		var data []byte
		if data, err = xattr.Get(path, xa); err != nil {
			log.Println("could not get xattr value: ", err)
			return "", err
		}

		xattrHex += fmt.Sprintf("%s=%x\n", xa, data)
	}

	// Get rid of the trailing \n
	xattrHex = strings.TrimSuffix(xattrHex, "\n")

	return xattrHex, nil
}

func deserializeAndSetXAttrs(path string, xattrsHex string) error {
	if xattrsHex == "" {
		return nil
	}

	xattrHexLines := strings.Split(xattrsHex, "\n")
	for _, xattrHexLine := range xattrHexLines {
		xattrHexLineParts := strings.Split(xattrHexLine, "=")
		if len(xattrHexLineParts) != 2 {
			return fmt.Errorf("'k=v' was malformed in hex serialization: ")
		}
		k := xattrHexLineParts[0]
		v := xattrHexLineParts[1]
		vBytes, err := hex.DecodeString(v)
		if err != nil {
			log.Println("could not deser xattr value from hex: ", err)
			return err
		}
		if err := xattr.Set(path, k, vBytes); err != nil {
			log.Printf("could not set xattr '%s': %v", k, err)
			return err
		}
	}
	return nil
}
