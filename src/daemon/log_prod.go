//go:build !debug
package main

func debugLog(format string, a ...any) {
	// Do nothing in production
}
