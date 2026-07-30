import AppKit
import SwiftUI

struct SimulatorDetailView: View {
    let sim: BootedSimulator
    @Binding var blockName: String

    @EnvironmentObject var monitor: SimulatorMonitor
    @EnvironmentObject var library: LibraryStore
    @EnvironmentObject var replay: ReplayEngine
    @EnvironmentObject var video: VideoRecorder
    @EnvironmentObject var capture: InteractionCapture

    private var steps: [InteractionStep] {
        monitor.timelines[sim.udid] ?? []
    }

    private var selectedCount: Int {
        monitor.selectedReplayableSteps(udid: sim.udid).count
    }

    var body: some View {
        VStack(spacing: 0) {
            if steps.isEmpty {
                emptyState
            } else {
                List(steps) { step in
                    StepRow(step: step, selected: isSelected(step.id))
                        .tag(step.id)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            let shift = NSEvent.modifierFlags.contains(.shift)
                            monitor.selectClick(udid: sim.udid, stepID: step.id, shift: shift)
                        }
                        .listRowBackground(isSelected(step.id) ? Color.accentColor.opacity(0.12) : nil)
                }
                .listStyle(.inset)
                .alternatingRowBackgrounds(.enabled)
            }
        }
        .navigationTitle(sim.name)
        .navigationSubtitle(capture.isCapturing ? capture.liveStatus : "Idle")
        .toolbar { detailToolbar }
        .safeAreaInset(edge: .bottom) {
            if selectedCount > 0 {
                selectionBar
            }
        }
    }

    private var emptyState: some View {
        ContentUnavailableView {
            Label(
                capture.isCapturing ? "Waiting for Simulator taps" : "Nothing recorded",
                systemImage: capture.isCapturing ? "hand.tap" : "record.circle"
            )
        } description: {
            if let err = capture.lastError {
                Text(err)
            } else if capture.isCapturing {
                Text("\(capture.liveStatus)\n\nBring Simulator to the front, then tap on the phone screen (not the Mac chrome).")
            } else {
                Text("Press Record Input, focus the Simulator, then tap. You’ll need Accessibility permission for XCAutokit Studio.")
            }
        } actions: {
            if !capture.isCapturing {
                Button("Record Input") {
                    capture.start(monitor: monitor)
                }
                .buttonStyle(.borderedProminent)
            } else if !capture.accessibilityTrusted {
                Button("Open Accessibility Settings") {
                    if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
                        NSWorkspace.shared.open(url)
                    }
                }
            }
        }
    }

    @ToolbarContentBuilder
    private var detailToolbar: some ToolbarContent {
        ToolbarItemGroup(placement: .primaryAction) {
            ControlGroup {
                Button {
                    Task { await executeSelection() }
                } label: {
                    Label("Execute", systemImage: "play.fill")
                }
                .disabled(selectedCount == 0 || replay.isExecuting)
                .help("Reset to the first step’s app state, then replay")

                Button {
                    saveBlock()
                } label: {
                    Label("Save Block", systemImage: "rectangle.badge.plus")
                }
                .disabled(selectedCount == 0)
                .help("Save the selected steps as a reusable Block")
            }

            Button {
                monitor.clearTimeline(udid: sim.udid)
            } label: {
                Label("Clear", systemImage: "trash")
            }
            .disabled(steps.isEmpty)
            .help("Clear this timeline")

            if video.isRecording(udid: sim.udid) {
                Button(role: .destructive) {
                    video.stop(udid: sim.udid)
                } label: {
                    Label("Stop Video", systemImage: "stop.circle.fill")
                }
            } else {
                Button {
                    video.start(udid: sim.udid, deviceName: sim.name)
                } label: {
                    Label("Video", systemImage: "video")
                }
                .help("Record a video of this simulator")
            }
        }
    }

    private var selectionBar: some View {
        HStack {
            Text("\(selectedCount) steps selected")
                .foregroundStyle(.secondary)
            if let restore = monitor.restorePointForSelection(udid: sim.udid) {
                Text("·")
                    .foregroundStyle(.tertiary)
                Label(restore.summary, systemImage: "arrow.counterclockwise")
                    .foregroundStyle(.secondary)
            }
            Spacer()
            TextField("Block name", text: $blockName)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 220)
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(.bar)
    }

    private func isSelected(_ id: UUID) -> Bool {
        monitor.selectedSteps(udid: sim.udid).contains { $0.id == id }
    }

    private func executeSelection() async {
        let steps = monitor.selectedReplayableSteps(udid: sim.udid)
        await replay.execute(
            steps: steps,
            bin: monitor.xcautokitPath,
            restore: RestorePoint.derived(from: steps)
        )
    }

    private func saveBlock() {
        let steps = monitor.selectedReplayableSteps(udid: sim.udid)
        guard !steps.isEmpty else { return }
        library.addBlock(Block(
            name: blockName,
            udid: sim.udid,
            steps: steps,
            restore: RestorePoint.derived(from: steps)
        ))
        blockName = "Untitled Block"
    }
}

struct StepRow: View {
    let step: InteractionStep
    let selected: Bool

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Image(systemName: step.kind.systemImage)
                .foregroundStyle(selected ? Color.accentColor : .secondary)
                .frame(width: 20)
            VStack(alignment: .leading, spacing: 2) {
                Text(step.summary)
                    .lineLimit(2)
                Text(step.kind.rawValue)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 8)
            Text(step.timestamp, style: .time)
                .font(.caption)
                .foregroundStyle(.tertiary)
                .monospacedDigit()
        }
        .padding(.vertical, 2)
    }
}
