package main

// Version is set at build time via:
//
//	go build -ldflags="-X main.Version=v1.2.3"
//
// Falls back to "dev" when built without ldflags (local development).
var Version = "dev"
