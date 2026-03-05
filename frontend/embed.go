// Package frontend embeds the built React app into the Go binary.
package frontend

import "embed"

//go:embed dist
var DistFS embed.FS
