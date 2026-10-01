module github.com/lemon4ksan/seal

go 1.27.0

require (
	github.com/klauspost/compress v1.20.0
	github.com/lemon4ksan/foundation v0.0.0-20260924183451-121e9f1d1f93
	github.com/lemon4ksan/przm v0.0.0
	github.com/lemon4ksan/sein v0.0.0-00010101000000-000000000000
	github.com/nwaples/rardecode/v2 v2.4.1
	golang.org/x/net v0.59.0
)

require (
	github.com/lemon4ksan/mach v0.0.0-20260924165838-4be719b602ef // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace (
	github.com/lemon4ksan/foundation => ../foundation
	github.com/lemon4ksan/przm => ../przm
)

replace github.com/lemon4ksan/sein => ../server/sein
