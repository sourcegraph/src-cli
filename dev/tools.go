//go:build tools
// +build tools

package main

import (
	// go-mockgen is used to codegen mockable interfaces
	_ "github.com/derision-test/go-mockgen/cmd/go-mockgen"
)
