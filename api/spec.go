// Package api embeds the public OpenAPI contract.
package api

import "embed"

// Files makes the API contract available without a runtime filesystem dependency.
//
//go:embed openapi.json
var Files embed.FS
