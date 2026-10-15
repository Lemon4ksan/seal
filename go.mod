module github.com/lemon4ksan/seal

go 1.27.0

require (
	github.com/lemon4ksan/ctrl v0.0.0-00010101000000-000000000000
	github.com/lemon4ksan/foundation v0.2.0
	github.com/lemon4ksan/przm v0.0.0
	github.com/nwaples/rardecode/v2 v2.4.1
	golang.org/x/net v0.61.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
)

require golang.org/x/crypto v0.58.0 // indirect

replace (
	github.com/lemon4ksan/ctrl => ../ctrl
	github.com/lemon4ksan/foundation => ../foundation
	github.com/lemon4ksan/przm => ../przm
)
