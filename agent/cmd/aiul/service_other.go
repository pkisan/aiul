//go:build !windows

package main

// runAsWindowsService is only meaningful on Windows.
func runAsWindowsService() bool { return false }
