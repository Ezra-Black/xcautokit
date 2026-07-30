package server

import (
	"fmt"

	"github.com/xcautokit/xcautokit/internal/session"
	"github.com/xcautokit/xcautokit/internal/sim"
)

func (a *App) resolveUDID(simulatorUuid string) (string, error) {
	if simulatorUuid != "" {
		return sim.ResolveUDID(simulatorUuid)
	}
	d := a.Session.Get()
	if d.SimulatorUdid != "" {
		return sim.ResolveUDID(d.SimulatorUdid)
	}
	return sim.ResolveUDID("booted")
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
