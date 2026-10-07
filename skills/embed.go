// Package skills distributes the same host-side guidance with the CLI and npm package.
package skills

import _ "embed"

// Companion is installed into a project's .agents/skills directory by xcautokit init.
//
//go:embed xcautokit/SKILL.md
var Companion string
