// Package web embeds the browser playground and learning guide.
package web

import "embed"

// Files contains only the assets served by the application, excluding tests.
//
//go:embed *.html *.css app.js api.js
var Files embed.FS
