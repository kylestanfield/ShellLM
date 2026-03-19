//go:build debug
package main

import "fmt"

func debugLog(format string, a ...any) {
	fmt.Printf("[DEBUG] "+format+"\n", a...)
}
