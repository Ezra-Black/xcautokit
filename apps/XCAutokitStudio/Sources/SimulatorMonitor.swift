import CoreGraphics
import Foundation

/// Polls booted simulators and owns per-device interaction timelines.
@MainActor
final class SimulatorMonitor: ObservableObject {
    @Published var simulators: [BootedSimulator] = []
    @Published var timelines: [String: [InteractionStep]] = [:]
    @Published var selectionAnchor: [String: UUID] = [:]
    @Published var selectionEnd: [String: UUID] = [:]
    @Published var statusMessage = "Starting…"
    @Published var isRunning = false
    @Published var screenSizes: [String: CGSize] = [:]
    @Published var foregroundBundleId: [String: String] = [:]

    weak var capture: InteractionCapture?

    private var pollTask: Task<Void, Never>?

    var xcautokitPath: String {
        if let env = ProcessInfo.processInfo.environment["XCAUTOKIT_BIN"],
           FileManager.default.isExecutableFile(atPath: env) {
            return env
        }
        let local = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Developer/Autokit/xcautokit").path
        return FileManager.default.isExecutableFile(atPath: local) ? local : "xcautokit"
    }

    func start() {
        guard !isRunning else { return }
        isRunning = true
        statusMessage = "Monitoring…"
        pollTask = Task { [weak self] in
            while let self, !Task.isCancelled, self.isRunning {
                await self.tick()
                try? await Task.sleep(nanoseconds: 1_500_000_000)
            }
        }
    }

    func stop() {
        isRunning = false
        pollTask?.cancel()
        pollTask = nil
        statusMessage = "Stopped"
    }

    private func tick() async {
        do {
            let booted = try await Self.listBooted()
            simulators = booted
            for sim in booted {
                if let bid = await ForegroundProbe.bundleId(udid: sim.udid) {
                    foregroundBundleId[sim.udid] = bid
                }
                await refreshScreenSize(sim)
                if timelines[sim.udid] == nil { timelines[sim.udid] = [] }
            }
            let recording = capture?.isCapturing == true ? " · recording" : ""
            statusMessage = "\(booted.count) simulator(s)\(recording)"
        } catch {
            statusMessage = "Error: \(error.localizedDescription)"
        }
    }

    private func refreshScreenSize(_ sim: BootedSimulator) async {
        let size = await Self.screenSize(udid: sim.udid)
        guard size.width > 0 else { return }
        screenSizes[sim.udid] = size
        capture?.updateScreenSize(udid: sim.udid, size: size)
    }

    func clearTimeline(udid: String) {
        timelines[udid] = []
        selectionAnchor[udid] = nil
        selectionEnd[udid] = nil
    }

    func appendManualStep(_ step: InteractionStep) {
        var stamped = step
        if stamped.payload["foregroundBundleId"] == nil {
            if let bid = foregroundBundleId[step.udid] {
                stamped.payload["foregroundBundleId"] = bid
                stamped.payload["onSpringBoard"] = ForegroundProbe.isSpringBoard(bundleId: bid) ? "1" : "0"
            } else {
                stamped.payload["onSpringBoard"] = "1"
            }
        }
        var list = timelines[stamped.udid] ?? []
        list.append(stamped)
        if list.count > 800 { list.removeFirst(list.count - 800) }
        timelines[stamped.udid] = list
    }

    func restorePointForSelection(udid: String) -> RestorePoint? {
        RestorePoint.derived(from: selectedReplayableSteps(udid: udid))
    }

    func selectedSteps(udid: String) -> [InteractionStep] {
        let list = timelines[udid] ?? []
        guard let a = selectionAnchor[udid],
              let b = selectionEnd[udid] ?? selectionAnchor[udid],
              let i = list.firstIndex(where: { $0.id == a }),
              let j = list.firstIndex(where: { $0.id == b }) else { return [] }
        return Array(list[min(i, j)...max(i, j)])
    }

    func selectedReplayableSteps(udid: String) -> [InteractionStep] {
        selectedSteps(udid: udid).filter(\.isReplayable)
    }

    func selectClick(udid: String, stepID: UUID, shift: Bool) {
        if shift, selectionAnchor[udid] != nil {
            selectionEnd[udid] = stepID
        } else {
            selectionAnchor[udid] = stepID
            selectionEnd[udid] = stepID
        }
    }

    static func listBooted() async throws -> [BootedSimulator] {
        try await Task.detached {
            let proc = Process()
            proc.executableURL = URL(fileURLWithPath: "/usr/bin/xcrun")
            proc.arguments = ["simctl", "list", "devices", "booted", "--json"]
            let out = Pipe()
            proc.standardOutput = out
            proc.standardError = Pipe()
            try proc.run()
            proc.waitUntilExit()
            let data = out.fileHandleForReading.readDataToEndOfFile()
            guard let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let devices = json["devices"] as? [String: Any] else { return [] }
            var result: [BootedSimulator] = []
            for (runtime, list) in devices {
                guard let arr = list as? [[String: Any]] else { continue }
                for d in arr {
                    guard let udid = d["udid"] as? String,
                          let name = d["name"] as? String,
                          let state = d["state"] as? String,
                          state == "Booted" else { continue }
                    result.append(BootedSimulator(udid: udid, name: name, runtime: runtime, state: state))
                }
            }
            return result.sorted { $0.name < $1.name }
        }.value
    }

    static func screenSize(udid: String) async -> CGSize {
        await Task.detached {
            let axe = Process()
            axe.executableURL = URL(fileURLWithPath: "/usr/bin/env")
            axe.arguments = ["axe", "describe-ui", "--udid", udid]
            let pipe = Pipe()
            axe.standardOutput = pipe
            axe.standardError = Pipe()
            do {
                try axe.run()
                axe.waitUntilExit()
                let data = pipe.fileHandleForReading.readDataToEndOfFile()
                guard !data.isEmpty else { return .zero }
                if let json = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]],
                   let root = json.first {
                    return parseFrameSize(root["frame"] ?? root["AXFrame"])
                }
                if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    return parseFrameSize(json["frame"] ?? json["AXFrame"])
                }
                return .zero
            } catch {
                return .zero
            }
        }.value
    }

    nonisolated private static func parseFrameSize(_ any: Any?) -> CGSize {
        if let dict = any as? [String: Any] {
            let w = (dict["width"] as? Double) ?? (dict["Width"] as? Double) ?? 0
            let h = (dict["height"] as? Double) ?? (dict["Height"] as? Double) ?? 0
            return CGSize(width: w, height: h)
        }
        if let s = any as? String {
            let nums = s.replacingOccurrences(of: "{", with: "")
                .replacingOccurrences(of: "}", with: "")
                .split(separator: ",")
                .compactMap { Double($0.trimmingCharacters(in: .whitespaces)) }
            if nums.count >= 4 { return CGSize(width: nums[2], height: nums[3]) }
        }
        return .zero
    }
}
