// Package util contains common helper functions used in multiple places
package util

import (
	"strings"
)

// Returns s without any trailing slashes if it has any; otherwise return s unchanged.
func StripTrailingSlashes(s string) string {
	if s == "/" {
		return s
	}

	for {
		stripped := strings.TrimSuffix(s, "/")
		if stripped == s {
			return stripped
		} else {
			s = stripped
		}
	}
}
