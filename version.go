package main

import "fmt"

// 编译时通过 ldflags 注入（例如: -X main.Version=v0.9.15 -X main.Commit=abc1234）
var (
	Version = "dev"
	Commit  = "none"
)

func GetVersion() string {
	return fmt.Sprintf("vortex %s (commit: %s)", Version, Commit)
}
