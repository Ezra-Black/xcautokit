package server

import (
	"context"

	"github.com/xcautokit/xcautokit/internal/config"
	"github.com/xcautokit/xcautokit/internal/session"
	"github.com/xcautokit/xcautokit/internal/ticket"
	"github.com/xcautokit/xcautokit/internal/xcodebridge"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type App struct {
	Cfg     config.Config
	Session *session.Store
	Bridge  *xcodebridge.Client
	Tickets *ticket.Store
}

func New() *App {
	return &App{
		Cfg:     config.Default(),
		Session: session.New(),
		Bridge:  xcodebridge.New(xcodebridge.ModeFromEnv()),
		Tickets: ticket.New(),
	}
}

func (a *App) Server() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "xcautokit",
		Version: config.Version,
	}, &mcp.ServerOptions{
		Instructions: AgentInstructions,
	})
	wf := workflowsFromEnv()
	a.registerResources(srv)
	a.registerPrompts(srv)
	if wf.enabled("device") {
		a.registerDeviceTools(srv)
	}
	if wf.enabled("input") {
		a.registerInputTools(srv)
	}
	if wf.enabled("ui") {
		a.registerUITools(srv)
	}
	if wf.enabled("app") {
		a.registerAppTools(srv)
	}
	if wf.enabled("capture") {
		a.registerCaptureTools(srv)
	}
	if wf.enabled("project") || wf.enabled("build") || wf.enabled("session") {
		a.registerProjectTools(srv)
	}
	if wf.enabled("xcode") {
		a.registerXcodeTools(srv)
	}
	return srv
}

func (a *App) Close() error {
	return a.Bridge.Close()
}

func (a *App) Run(ctx context.Context) error {
	srv := a.Server()
	defer a.Close()
	return srv.Run(ctx, &mcp.StdioTransport{})
}
