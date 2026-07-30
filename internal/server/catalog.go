package server

type ToolInfo struct {
	Name        string
	Category    string
	Description string
}

func ToolCatalog() []ToolInfo {
	return []ToolInfo{
		{Name: "status", Category: "device", Description: "Simulator status"},
		{Name: "device_list", Category: "device", Description: "List simulators"},
		{Name: "device_boot", Category: "device", Description: "Boot simulator"},
		{Name: "device_shutdown", Category: "device", Description: "Shutdown simulator"},
		{Name: "open_sim", Category: "device", Description: "Foreground Simulator.app"},
		{Name: "tap", Category: "input", Description: "Tap coordinates"},
		{Name: "swipe", Category: "input", Description: "Swipe gesture"},
		{Name: "gesture", Category: "input", Description: "Semantic gestures"},
		{Name: "long_press", Category: "input", Description: "Long press"},
		{Name: "button", Category: "input", Description: "Hardware button"},
		{Name: "type_text", Category: "input", Description: "Type text"},
		{Name: "key_press", Category: "input", Description: "Press keycode"},
		{Name: "key_sequence", Category: "input", Description: "Keycode sequence"},
		{Name: "ui_describe", Category: "ui", Description: "Full accessibility tree"},
		{Name: "ui_find", Category: "ui", Description: "Find elements"},
		{Name: "ui_search", Category: "ui", Description: "Search UI text"},
		{Name: "ui_summary", Category: "ui", Description: "Compact UI summary (includes interrupts preview)"},
		{Name: "ui_point", Category: "ui", Description: "Element at point"},
		{Name: "ui_check_interrupt", Category: "ui", Description: "Detect alerts/sheets/permissions/banners"},
		{Name: "ui_dismiss_interrupt", Category: "ui", Description: "Dismiss interrupt with explicit action"},
		{Name: "screenshot", Category: "capture", Description: "Capture screenshot"},
		{Name: "record_start", Category: "capture", Description: "Start recording (returns ticket)"},
		{Name: "record_stop", Category: "capture", Description: "Stop recording by ticket"},
		{Name: "start_sim_log_cap", Category: "capture", Description: "Start log capture (returns ticket)"},
		{Name: "stop_sim_log_cap", Category: "capture", Description: "Stop log capture by ticket"},
		{Name: "app_install", Category: "app", Description: "Install .app"},
		{Name: "app_launch", Category: "app", Description: "Launch by bundle ID"},
		{Name: "app_terminate", Category: "app", Description: "Terminate app"},
		{Name: "open_url", Category: "app", Description: "Open URL scheme"},
		{Name: "discover_projects", Category: "project", Description: "Find Xcode projects"},
		{Name: "list_schemes", Category: "project", Description: "List schemes"},
		{Name: "show_build_settings", Category: "project", Description: "Show build settings"},
		{Name: "get_app_bundle_id", Category: "project", Description: "Resolve bundle ID"},
		{Name: "get_sim_app_path", Category: "project", Description: "Installed app path"},
		{Name: "session_set_defaults", Category: "session", Description: "Set session defaults"},
		{Name: "session_show_defaults", Category: "session", Description: "Show session defaults"},
		{Name: "session_clear_defaults", Category: "session", Description: "Clear session defaults"},
		{Name: "build_sim", Category: "build", Description: "Build for simulator"},
		{Name: "build_run_sim", Category: "build", Description: "Build and run"},
		{Name: "test_sim", Category: "build", Description: "Run tests"},
		{Name: "clean", Category: "build", Description: "Clean build"},
		{Name: "launch_app_logs_sim", Category: "build", Description: "Launch with logs hint"},
		{Name: "xcode_windows", Category: "xcode", Description: "List Xcode windows (mcpbridge)"},
		{Name: "xcode_issues", Category: "xcode", Description: "Issue Navigator"},
		{Name: "xcode_build_log", Category: "xcode", Description: "Build log"},
		{Name: "xcode_preview", Category: "xcode", Description: "SwiftUI preview"},
		{Name: "docs_search", Category: "xcode", Description: "Apple docs search"},
		{Name: "swift_snippet", Category: "xcode", Description: "Execute Swift snippet"},
		{Name: "run_some_tests", Category: "xcode", Description: "Run selected tests"},
	}
}
