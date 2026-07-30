import Foundation

struct ColumnRecording: Identifiable, Hashable {
    var id: String { udid }
    var udid: String
    var path: URL
    var startedAt: Date
}

/// Per-simulator `simctl io recordVideo` sessions.
@MainActor
final class VideoRecorder: ObservableObject {
    @Published var active: [String: ColumnRecording] = [:]
    @Published var lastError: String?

    private var processes: [String: Process] = [:]

    private var recordingsDir: URL {
        let dir = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Pictures/xcautokit/studio", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir
    }

    func isRecording(udid: String) -> Bool { active[udid] != nil }

    func start(udid: String, deviceName: String) {
        guard active[udid] == nil else { return }
        let stamp = ISO8601DateFormatter().string(from: Date()).replacingOccurrences(of: ":", with: "-")
        let safe = deviceName.replacingOccurrences(of: " ", with: "_")
        let path = recordingsDir.appendingPathComponent("\(safe)_\(udid.prefix(8))_\(stamp).mp4")

        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/usr/bin/xcrun")
        proc.arguments = ["simctl", "io", udid, "recordVideo", "--codec=h264", path.path]
        proc.standardOutput = Pipe()
        proc.standardError = Pipe()
        do {
            try proc.run()
            processes[udid] = proc
            active[udid] = ColumnRecording(udid: udid, path: path, startedAt: Date())
            lastError = nil
        } catch {
            lastError = error.localizedDescription
        }
    }

    func stop(udid: String) {
        guard let proc = processes[udid] else { return }
        // simctl recordVideo finalizes the file on SIGINT.
        proc.interrupt()
        DispatchQueue.global().async {
            proc.waitUntilExit()
        }
        processes.removeValue(forKey: udid)
        active.removeValue(forKey: udid)
    }

    func stopAll() {
        for udid in Array(processes.keys) {
            stop(udid: udid)
        }
    }
}
