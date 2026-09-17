module github.com/lemon4ksan/seal

go 1.27.0

require (
	github.com/klauspost/compress v1.20.0
	github.com/lemon4ksan/foundation v0.0.0-20260912143841-ae415cdfd72d
	github.com/lemon4ksan/przm v0.0.0
	github.com/nwaples/rardecode/v2 v2.4.1
	golang.org/x/net v0.59.0
)

require (
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace (
	github.com/lemon4ksan/foundation => ../foundation
	github.com/lemon4ksan/przm => ../przm
)
