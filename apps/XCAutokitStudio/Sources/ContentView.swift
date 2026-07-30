import AppKit
import SwiftUI

struct ContentView: View {
    @EnvironmentObject var monitor: SimulatorMonitor
    @EnvironmentObject var library: LibraryStore
    @EnvironmentObject var replay: ReplayEngine
    @EnvironmentObject var capture: InteractionCapture
    @EnvironmentObject var video: VideoRecorder

    @State private var selection: SidebarItem? = .blocks
    @State private var blockName = "Untitled Block"
    @State private var inspectorPresented = true
    @State private var columnVisibility: NavigationSplitViewVisibility = .all
    @AppStorage("studio.coldStartRestore") private var coldStartRestore = true

    var body: some View {
        NavigationSplitView(columnVisibility: $columnVisibility) {
            sidebar
        } detail: {
            detail
        }
        .navigationSplitViewStyle(.balanced)
        .inspector(isPresented: $inspectorPresented) {
            inspector
                .inspectorColumnWidth(min: 240, ideal: 280, max: 360)
        }
        .toolbar { toolbarContent }
        .onAppear {
            columnVisibility = .all
            purgeLegacyUIChangeSpam()
            if selection == nil || !selectionIsValid {
                selection = monitor.simulators.first.map { .simulator($0.udid) } ?? .blocks
            }
        }
        .onChange(of: monitor.simulators) { _, sims in
            if case .simulator(let udid) = selection, !sims.contains(where: { $0.udid == udid }) {
                selection = sims.first.map { .simulator($0.udid) } ?? .blocks
            } else if selection == nil {
                selection = sims.first.map { .simulator($0.udid) } ?? .blocks
            }
        }
        .onDisappear {
            video.stopAll()
            capture.stop()
        }
        .alert(
            "Accessibility Required",
            isPresented: Binding(
                get: { capture.lastError != nil && !capture.accessibilityTrusted },
                set: { if !$0 { capture.lastError = nil } }
            )
        ) {
            Button("Open System Settings") {
                if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
                    NSWorkspace.shared.open(url)
                }
            }
            Button("OK", role: .cancel) {}
        } message: {
            Text(capture.lastError ?? "Grant Accessibility access so Studio can record Simulator interactions.")
        }
    }

    private var selectionIsValid: Bool {
        switch selection {
        case .simulator(let udid):
            return monitor.simulators.contains(where: { $0.udid == udid })
        case .blocks, .buildingBlocks, .sets, .none:
            return selection != nil
        }
    }

    /// Drop leftover fingerprint spam from older builds.
    private func purgeLegacyUIChangeSpam() {
        for sim in monitor.simulators {
            let steps = monitor.timelines[sim.udid] ?? []
            if !steps.isEmpty, steps.allSatisfy({ $0.kind == .uiChange }) {
                monitor.clearTimeline(udid: sim.udid)
            }
        }
    }

    // MARK: - Sidebar

    private var sidebar: some View {
        List(selection: $selection) {
            Section("Simulators") {
                if monitor.simulators.isEmpty {
                    Text("No booted devices")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(monitor.simulators) { sim in
                        HStack(spacing: 8) {
                            Image(systemName: video.isRecording(udid: sim.udid) ? "record.circle.fill" : "iphone")
                                .foregroundStyle(video.isRecording(udid: sim.udid) ? .red : .secondary)
                                .frame(width: 18)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(sim.name)
                                    .lineLimit(1)
                                Text(sim.runtimeShort)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                        }
                        .tag(SidebarItem.simulator(sim.udid))
                    }
                }
            }

            Section("Library") {
                ForEach(LibraryKind.allCases) { kind in
                    HStack {
                        Label(kind.rawValue, systemImage: kind.systemImage)
                        Spacer(minLength: 8)
                        if libraryBadge(for: kind) > 0 {
                            Text("\(libraryBadge(for: kind))")
                                .font(.caption.monospacedDigit())
                                .foregroundStyle(.secondary)
                        }
                    }
                    .tag(kind.sidebarItem)
                }
            }
        }
        .listStyle(.sidebar)
        .navigationTitle("XCAutokit")
        .navigationSplitViewColumnWidth(min: 200, ideal: 240, max: 320)
    }

    private func libraryBadge(for kind: LibraryKind) -> Int {
        switch kind {
        case .blocks: return library.library.blocks.count
        case .buildingBlocks: return library.library.buildingBlocks.count
        case .sets: return library.library.sets.count
        }
    }

    // MARK: - Detail

    @ViewBuilder
    private var detail: some View {
        switch selection {
        case .simulator(let udid):
            if let sim = monitor.simulators.first(where: { $0.udid == udid }) {
                SimulatorDetailView(sim: sim, blockName: $blockName)
            } else {
                ContentUnavailableView("Simulator Unavailable", systemImage: "iphone.slash")
            }
        case .blocks:
            BlocksLibraryView()
        case .buildingBlocks:
            BuildingBlocksLibraryView()
        case .sets:
            SetsLibraryView()
        case .none:
            ContentUnavailableView(
                "Select a Simulator",
                systemImage: "sidebar.left",
                description: Text("Choose a booted simulator from the sidebar, or open Library to manage blocks.")
            )
        }
    }

    // MARK: - Inspector

    private var inspector: some View {
        Form {
            Section {
                LabeledContent("Devices") {
                    Text(monitor.isRunning ? "Watching" : "Paused")
                }
                LabeledContent("Input recording") {
                    Text(capture.isCapturing ? "On" : "Off")
                }
                if capture.isCapturing || capture.lastError != nil {
                    Text(capture.liveStatus)
                        .font(.caption)
                        .foregroundStyle(capture.lastError == nil ? Color.secondary : Color.red)
                }
            } header: {
                Text("Status")
            } footer: {
                Text("1) Record Input  2) Click the Simulator window so it’s frontmost  3) Tap the phone screen. Grant Accessibility if prompted.")
            }

            Section("When replaying") {
                Picker("System alerts", selection: $replay.interruptPolicy) {
                    ForEach(InterruptPolicy.allCases) { policy in
                        Text(policy.label).tag(policy)
                    }
                }
                Toggle("Relaunch app from scratch", isOn: $coldStartRestore)
                if replay.isExecuting {
                    Label("Running…", systemImage: "play.circle.fill")
                        .foregroundStyle(.secondary)
                }
            }

            if case .simulator(let udid) = selection,
               let restore = monitor.restorePointForSelection(udid: udid) {
                Section("Before this run") {
                    Text("Reset to \(restore.summary)")
                        .foregroundStyle(.secondary)
                }
            }

            Section("Run log") {
                if replay.lastLog.isEmpty {
                    Text("Nothing executed yet.")
                        .foregroundStyle(.secondary)
                } else {
                    Text(replay.lastLog)
                        .font(.system(.caption, design: .monospaced))
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
        .formStyle(.grouped)
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        ToolbarItemGroup(placement: .navigation) {
            Button {
                inspectorPresented.toggle()
            } label: {
                Label("Inspector", systemImage: "sidebar.trailing")
            }
            .help("Show or hide the inspector")
        }

        ToolbarItemGroup(placement: .primaryAction) {
            Toggle(isOn: Binding(
                get: { capture.isCapturing },
                set: { on in
                    if on { capture.start(monitor: monitor) } else { capture.stop() }
                }
            )) {
                Label(
                    capture.isCapturing ? "Recording Input" : "Record Input",
                    systemImage: capture.isCapturing ? "record.circle.fill" : "hand.tap"
                )
            }
            .toggleStyle(.button)
            .help("Record mouse and keyboard actions from Simulator.app (Accessibility required)")
            .tint(capture.isCapturing ? .red : nil)
        }
    }
}
