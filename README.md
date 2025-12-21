# smerkle

A weekend project exploring Merkle trees in Go.

## what we have

- Content-addressable object store with git-style sharding (`objects/ab/cd...`)
- SHA-256 hashing for blobs and trees
- Index with caching (avoids rehashing unchanged files via size/modTime checks)
- Atomic writes via temp files
- Binary serialization for blobs, trees, and index

## TODO

- [ ] `.smerkleignore` - gitignore-style pattern matching for excluding files
- [ ] Directory walker - recursively walk a directory to build the tree
- [ ] Tree diffing - compare two trees and output changes (added/modified/deleted)
