// Package rules embeds the rules that every agent run starts with.
package rules

import _ "embed"

//go:embed AGENTS.md
var Text string
