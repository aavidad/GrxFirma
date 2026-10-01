// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build windows

package clockdiagnostic

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func inspectLocalClock() localSnapshot {
	snapshot := localSnapshot{
		observedAtUTC:   timeNowUTC(),
		timeService:     "unknown",
		serviceObserved: true,
	}
	manager, err := windows.OpenSCManager(
		nil,
		nil,
		windows.SC_MANAGER_CONNECT,
	)
	if err != nil {
		return snapshot
	}
	defer windows.CloseServiceHandle(manager)

	serviceName, err := windows.UTF16PtrFromString("W32Time")
	if err != nil {
		return snapshot
	}
	service, err := windows.OpenService(
		manager,
		serviceName,
		windows.SERVICE_QUERY_STATUS,
	)
	if err != nil {
		return snapshot
	}
	defer windows.CloseServiceHandle(service)

	var status windows.SERVICE_STATUS_PROCESS
	var bytesNeeded uint32
	err = windows.QueryServiceStatusEx(
		service,
		windows.SC_STATUS_PROCESS_INFO,
		(*byte)(unsafe.Pointer(&status)),
		uint32(unsafe.Sizeof(status)),
		&bytesNeeded,
	)
	if err != nil {
		return snapshot
	}
	if status.CurrentState == windows.SERVICE_RUNNING {
		snapshot.timeService = "running"
	} else if status.CurrentState == windows.SERVICE_STOPPED {
		snapshot.timeService = "stopped"
	}
	return snapshot
}
