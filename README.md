# smerkle

A weekend project exploring Merkle trees in Go.

## what we have

- Content-addressable object store with git-style sharding (`objects/ab/cd...`)
- SHA-256 hashing for blobs and trees
- Index with caching (avoids rehashing unchanged files via size/modTime checks)
- Atomic writes via temp files
- Binary serialization for blobs, trees, and index
- Directory walker that builds Merkle trees from filesystem
- Ignore file support (gitignore-style patterns)
- Tree diffing to compare two trees and report changes (added/deleted/modified/type changes)
