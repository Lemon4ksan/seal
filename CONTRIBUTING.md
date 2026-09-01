# Contributing to Seal

Thank you for your interest in contributing to `seal`!

## Code Standards

1. **Zero External Dependencies**: Core packages strictly rely on the Go standard library and `github.com/lemon4ksan/foundation`.
2. **Performance Mindset**: Zero heap allocations on critical streaming pathways; use `internal/pool` and sized byte buffers.
3. **Documentation Integrity**: All exported types and functions must have descriptive, technically precise Go doc comments in English matching the standard Go style and BSD headers (`// Copyright (c) 2026 Lemon4ksan All rights reserved.`).
4. **Testing**: Every new feature or bugfix must be accompanied by comprehensive tests and benchmark validations.

## Running Tests

```bash
# Run unit tests with race detection
go test -v -race ./...

# Run benchmarks
go test -bench=. -benchmem ./...
```
