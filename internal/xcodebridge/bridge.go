package xcodebridge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Mode string

const (
	ModeAuto Mode = "auto"
	ModeOn   Mode = "on"
	ModeOff  Mode = "off"
)

type Status struct {
	Available     bool   `json:"available"`
	Mode          string `json:"mode"`
	BridgePath    string `json:"bridgePath,omitempty"`
	Connected     bool   `json:"connected"`
	TabIdentifier string `json:"tabIdentifier,omitempty"`
	WorkspacePath string `json:"workspacePath,omitempty"`
	Message       string `json:"message,omitempty"`
	Error         string `json:"error,omitempty"`
}

type Client struct {
	mu      sync.Mutex
	mode    Mode
	session *mcp.ClientSession
	client  *mcp.Client
	status  Status
}

func ModeFromEnv() Mode {
	raw := os.Getenv("XCAUTOKIT_XCODE_BACKEND")
	if raw == "" {
		raw = os.Getenv("AUTOKIT_XCODE_BACKEND") // legacy alias
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "1", "true":
		return ModeOn
	case "off", "0", "false":
		return ModeOff
	default:
		return ModeAuto
	}
}

func New(mode Mode) *Client {
	if mode == "" {
		mode = ModeFromEnv()
	}
	return &Client{
		mode: mode,
		status: Status{
			Mode: string(mode),
		},
	}
}

func FindBridge() (string, error) {
	out, err := exec.Command("xcrun", "--find", "mcpbridge").CombinedOutput()
	path := strings.TrimSpace(string(out))
	if err != nil || path == "" {
		return "", fmt.Errorf("mcpbridge not found (requires Xcode 26.3+): %v %s", err, path)
	}
	return path, nil
}

func (c *Client) Mode() Mode { return c.mode }

func (c *Client) Status(ctx context.Context) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.status
	st.Mode = string(c.mode)
	if c.mode == ModeOff {
		st.Available = false
		st.Message = "Xcode backend disabled (XCAUTOKIT_XCODE_BACKEND=off)"
		return st
	}
	path, err := FindBridge()
	if err != nil {
		st.Available = false
		st.Error = err.Error()
		st.Message = "Install Xcode 26.3+ and enable Intelligence → Allow external agents to use Xcode tools"
		return st
	}
	st.Available = true
	st.BridgePath = path
	if c.session != nil {
		st.Connected = true
	}
	return st
}

func (c *Client) Enabled() bool {
	if c.mode == ModeOff {
		return false
	}
	_, err := FindBridge()
	if err != nil {
		return c.mode == ModeOn // on forces attempt; auto stays off if missing
	}
	return true
}

func (c *Client) Ensure(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode == ModeOff {
		return fmt.Errorf("xcode backend disabled")
	}
	if c.session != nil {
		return nil
	}
	path, err := FindBridge()
	if err != nil {
		return err
	}
	cmd := exec.Command("xcrun", "mcpbridge")
	if pid := os.Getenv("MCP_XCODE_PID"); pid != "" {
		cmd.Env = append(os.Environ(), "MCP_XCODE_PID="+pid)
	}
	if sid := os.Getenv("MCP_XCODE_SESSION_ID"); sid != "" {
		cmd.Env = append(cmd.Env, "MCP_XCODE_SESSION_ID="+sid)
	}
	_ = path
	client := mcp.NewClient(&mcp.Implementation{Name: "xcautokit-xcode-bridge", Version: "1.0.0"}, nil)
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, err := client.Connect(connectCtx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		c.status.Error = err.Error()
		return fmt.Errorf("connect mcpbridge: %w", err)
	}
	c.client = client
	c.session = session
	c.status.Connected = true
	c.status.Available = true
	c.status.BridgePath = path
	c.status.Message = "connected to Xcode mcpbridge"
	return nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		err := c.session.Close()
		c.session = nil
		c.client = nil
		c.status.Connected = false
		return err
	}
	return nil
}

func (c *Client) Call(ctx context.Context, name string, args map[string]any) (string, map[string]any, error) {
	if err := c.Ensure(ctx); err != nil {
		return "", nil, err
	}
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return "", nil, fmt.Errorf("no mcpbridge session")
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		// Drop session so next call reconnects.
		_ = c.Close()
		return "", nil, err
	}
	if res.IsError {
		return textFromResult(res), asMap(res.StructuredContent), fmt.Errorf("tool error: %s", textFromResult(res))
	}
	return textFromResult(res), asMap(res.StructuredContent), nil
}

func (c *Client) ListWindows(ctx context.Context) (string, error) {
	text, _, err := c.Call(ctx, "XcodeListWindows", map[string]any{})
	if err != nil {
		return "", err
	}
	tab, workspace := parseWindowInfo(text)
	c.mu.Lock()
	c.status.TabIdentifier = tab
	c.status.WorkspacePath = workspace
	c.mu.Unlock()
	return text, nil
}

func (c *Client) ResolveTab(ctx context.Context, tab string) (string, error) {
	if tab != "" {
		return tab, nil
	}
	c.mu.Lock()
	cached := c.status.TabIdentifier
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	text, err := c.ListWindows(ctx)
	if err != nil {
		return "", err
	}
	tab, _ = parseWindowInfo(text)
	if tab == "" {
		return "", fmt.Errorf("no Xcode tabIdentifier found; open a project in Xcode")
	}
	return tab, nil
}

func (c *Client) BuildProject(ctx context.Context, tab, scheme, configuration string) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	args := map[string]any{"tabIdentifier": tab}
	if scheme != "" {
		args["scheme"] = scheme
	}
	if configuration != "" {
		args["configuration"] = configuration
	}
	return c.Call(ctx, "BuildProject", args)
}

func (c *Client) RunAllTests(ctx context.Context, tab, scheme string) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	args := map[string]any{"tabIdentifier": tab}
	if scheme != "" {
		args["scheme"] = scheme
	}
	return c.Call(ctx, "RunAllTests", args)
}

func (c *Client) RunSomeTests(ctx context.Context, tab string, tests []string, scheme string) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	args := map[string]any{"tabIdentifier": tab, "tests": tests}
	if scheme != "" {
		args["scheme"] = scheme
	}
	return c.Call(ctx, "RunSomeTests", args)
}

func (c *Client) GetBuildLog(ctx context.Context, tab, severity string) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	args := map[string]any{"tabIdentifier": tab}
	if severity != "" {
		args["severity"] = severity
	}
	return c.Call(ctx, "GetBuildLog", args)
}

func (c *Client) ListNavigatorIssues(ctx context.Context, tab string) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	return c.Call(ctx, "XcodeListNavigatorIssues", map[string]any{"tabIdentifier": tab})
}

func (c *Client) RenderPreview(ctx context.Context, tab, filePath, previewName string) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	args := map[string]any{"tabIdentifier": tab, "filePath": filePath}
	if previewName != "" {
		args["previewName"] = previewName
	}
	return c.Call(ctx, "RenderPreview", args)
}

func (c *Client) DocumentationSearch(ctx context.Context, query string) (string, map[string]any, error) {
	return c.Call(ctx, "DocumentationSearch", map[string]any{"query": query})
}

func (c *Client) ExecuteSnippet(ctx context.Context, tab, code string, timeout float64) (string, map[string]any, error) {
	tab, err := c.ResolveTab(ctx, tab)
	if err != nil {
		return "", nil, err
	}
	args := map[string]any{"tabIdentifier": tab, "code": code}
	if timeout > 0 {
		args["timeout"] = timeout
	}
	return c.Call(ctx, "ExecuteSnippet", args)
}

func textFromResult(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func parseWindowInfo(text string) (tab, workspace string) {
	// Expected-ish: "tabIdentifier: windowtab1, workspacePath: /path"
	lower := text
	for _, line := range strings.Split(lower, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(strings.ToLower(line), "tabidentifier:"); i >= 0 {
			rest := strings.TrimSpace(line[i+len("tabIdentifier:"):])
			// handle "windowtab1," or "windowtab1 "
			rest = strings.TrimPrefix(rest, " ")
			for _, sep := range []string{",", " "} {
				if j := strings.Index(rest, sep); j >= 0 {
					tab = strings.TrimSpace(rest[:j])
					break
				}
			}
			if tab == "" {
				tab = strings.TrimSpace(rest)
			}
		}
		if i := strings.Index(strings.ToLower(line), "workspacepath:"); i >= 0 {
			rest := strings.TrimSpace(line[i+len("workspacePath:"):])
			workspace = strings.TrimSpace(rest)
		}
	}
	// Also try single-line comma form.
	if tab == "" {
		if i := strings.Index(strings.ToLower(text), "tabidentifier:"); i >= 0 {
			rest := text[i+len("tabIdentifier:"):]
			fields := strings.Split(rest, ",")
			if len(fields) > 0 {
				tab = strings.TrimSpace(fields[0])
			}
		}
	}
	return tab, workspace
}
