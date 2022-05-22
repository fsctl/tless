// Package objstorefs is a filesystem-like abstraction on top of the objstore package that
// presents the remote, encrypted key-value pairs as a listing of files.  It can read .DELETED
// files to reconstruct the full history of a snapshot, and upload or download entire files
// to a snapshot by transparently constructing a file from/to numbered chunks.
package objstorefs
