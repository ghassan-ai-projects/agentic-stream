//go:build tools

// Package tools pins the versions of dev-only tools used by the Makefile
// and CI. The //go:build tools constraint ensures these dependencies never
// leak into production binaries — they are only present in go.mod for
// reproducible installs via `go install <package>@<version>`.
package tools

import (
	_ "golang.org/x/tools/cmd/deadcode"
	_ "golang.org/x/tools/cmd/goimports"
	_ "golang.org/x/vuln/cmd/govulncheck"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
