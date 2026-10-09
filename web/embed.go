// Package web embeds the static frontend.
package web

import "embed"

//go:embed *.html css js vendor
var FS embed.FS
