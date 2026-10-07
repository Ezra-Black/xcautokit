package server

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/config"
	"github.com/xcautokit/xcautokit/internal/devicelease"
	"github.com/xcautokit/xcautokit/internal/session"
	"github.com/xcautokit/xcautokit/internal/ticket"
	"github.com/xcautokit/xcautokit/internal/xcodebridge"
)

type App struct {
	Cfg     config.Config
	Session *session.Store
	Bridge  *xcodebridge.Client
	Tickets *ticket.Store
	Leases  *devicelease.Store
}

func New() *App {
	return &App{
		Cfg:     config.Default(),
		Session: session.New(),
		Bridge:  xcodebridge.New(xcodebridge.ModeFromEnv()),
		Tickets: ticket.New(),
		Leases:  devicelease.New(filepath.Join(stateDir(), "locks")),
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
	a.registerCoordinationTools(srv)
	a.registerWorkflowTools(srv)
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
		a.registerUIWorkflowTools(srv)
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
	return errors.Join(a.Bridge.Close(), a.Tickets.Close(), a.Leases.Close())
}

func (a *App) Run(ctx context.Context) error {
	srv := a.Server()
	defer a.Close()
	return srv.Run(ctx, &mcp.StdioTransport{})
}
