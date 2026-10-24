// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Package buildinfo holds the fork's attribution constants. They are plain
// constants so every binary (desktop, CLI, daemon) prints the same credit
// without build-time injection.
package buildinfo

const (
	// Author is the maintainer of this fork.
	Author = "Nguyễn Dương Gia Thuận"
	// Repository is the public home of this fork.
	Repository = "https://github.com/ndgthuan/tdrive"
	// Upstream names the project this fork is based on.
	Upstream = "TDrive by Jeetraj"
	// UpstreamURL is the original project, licensed under MIT.
	UpstreamURL = "https://github.com/jeetrex17/TDrive"
)

// Attribution is the one-line credit printed by the CLI and daemon.
func Attribution() string {
	return "Made by " + Author + ", based on " + Upstream + " (MIT)"
}
