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
	a.registerResources(srv)
	a.registerPrompts(srv)
	a.registerDeviceTools(srv)
	a.registerInputTools(srv)
	a.registerUITools(srv)
	a.registerAppTools(srv)
	a.registerCaptureTools(srv)
	a.registerProjectTools(srv)
	a.registerXcodeTools(srv)
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
