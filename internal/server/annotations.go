package server

import "github.com/modelcontextprotocol/go-sdk/mcp"

func boolPtr(b bool) *bool { return &b }

// Read-only / inspect tools.
func annRO() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:   true,
		OpenWorldHint:  boolPtr(false),
		IdempotentHint: true,
	}
}

// Mutating but non-destructive (tap, type, launch, build, record start).
func annWrite() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}
}

// Destructive / hard to undo (shutdown, terminate, clean, clear defaults).
func annDestructive() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(true),
		OpenWorldHint:   boolPtr(false),
	}
}

func toolMeta(name, title, description string, ann *mcp.ToolAnnotations) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: ann,
	}
}
