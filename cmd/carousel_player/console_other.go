//go:build !windows

package main

func hideConsoleWindow() {
	// no-op on non-Windows
}
