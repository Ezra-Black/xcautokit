package server

import (
	"context"
	"fmt"

	"github.com/xcautokit/xcautokit/internal/session"
	"github.com/xcautokit/xcautokit/internal/sim"
)

func (a *App) resolveUDID(simulatorUuid string) (string, error) {
	return a.resolveUDIDContext(context.Background(), simulatorUuid)
}

type deviceContextKey struct{}

func (a *App) resolveUDIDContext(ctx context.Context, simulatorUuid string) (string, error) {
	if pinned, ok := ctx.Value(deviceContextKey{}).(string); ok {
		return pinned, nil
	}
	if simulatorUuid != "" {
		return sim.ResolveUDIDContext(ctx, simulatorUuid)
	}
	d := a.Session.Get()
	if d.SimulatorUdid != "" {
		return sim.ResolveUDIDContext(ctx, d.SimulatorUdid)
	}
	if d.SimulatorName != "" {
		return sim.ResolveUDIDContext(ctx, d.SimulatorName)
	}
	return sim.ResolveUDIDContext(ctx, "booted")
}

func (a *App) projectOpts(project, scheme, configuration, destination string) (string, string, string, string, session.Defaults, error) {
	d := a.Session.Get()
	if project == "" {
		project = d.ProjectPath
	}
	if scheme == "" {
		scheme = d.Scheme
	}
	if configuration == "" {
		configuration = d.Configuration
	}
	if configuration == "" {
		configuration = "Debug"
	}
	if project == "" {
		return "", "", "", "", d, fmt.Errorf("project path required (pass project or session_set_defaults)")
	}
	if scheme == "" {
		return "", "", "", "", d, fmt.Errorf("scheme required (pass scheme or session_set_defaults)")
	}
	return project, scheme, configuration, destination, d, nil
}
