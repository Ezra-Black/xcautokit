import Foundation

enum InterruptPolicy: String, CaseIterable, Identifiable {
    case declinePermissions = "decline"
    case acceptPermissions = "accept"
    case pauseOnInterrupt = "pause"

    var id: String { rawValue }

    var label: String {
        switch self {
        case .declinePermissions: return "Auto-decline permissions"
        case .acceptPermissions: return "Auto-accept permissions"
        case .pauseOnInterrupt: return "Pause on interrupt"
        }
    }
}

/// Replays blocks / sets with interrupt handling via `xcautokit interrupt`.
@MainActor
final class ReplayEngine: ObservableObject {
    @Published var isExecuting = false
    @Published var lastLog = ""
    @Published var interruptPolicy: InterruptPolicy = .declinePermissions
    @Published var pausedForInterrupt = false

    var onExecutingChange: ((Bool) -> Void)?

    func execute(steps: [InteractionStep], bin: String, restore: RestorePoint? = nil) async {
        guard !isExecuting else { return }
        isExecuting = true
        onExecutingChange?(true)
        pausedForInterrupt = false
        defer {
            isExecuting = false
            onExecutingChange?(false)
        }

        var lines: [String] = []
        func log(_ line: String) {
            lines.append(line)
            lastLog = lines.joined(separator: "\n")
        }

        var actionable = steps.filter(\.isReplayable)
        let point = restore ?? RestorePoint.derived(from: steps)

        if let point {
            log(await restoreToFirstStepState(point))
            if let first = actionable.first,
               first.kind == .appLaunch,
               first.payload["bundleId"] == point.bundleId {
                actionable.removeFirst()
                log("restore: skipped duplicate first appLaunch")
            }
            if let handled = await handleInterrupts(udid: point.udid, bin: bin) {
                log(handled)
                if pausedForInterrupt { return }
            }
        } else {
            log("restore: no restore point (running steps as-is)")
        }

        for step in actionable {
            if step.kind != .interruptDismiss,
               let handled = await handleInterrupts(udid: step.udid, bin: bin) {
                log(handled)
                if pausedForInterrupt { return }
            }
            log(await runStep(step, bin: bin))
            try? await Task.sleep(nanoseconds: 120_000_000)
        }

        if let last = actionable.last,
           let handled = await handleInterrupts(udid: last.udid, bin: bin) {
            log(handled)
        }
    }

    private func restoreToFirstStepState(_ point: RestorePoint) async -> String {
        let preferColdStart = UserDefaults.standard.object(forKey: "studio.coldStartRestore") as? Bool ?? true
        var parts = ["restore → \(point.summary)"]
        parts.append(await ProcessRunner.run(["axe", "button", "home", "--udid", point.udid]))
        try? await Task.sleep(nanoseconds: 400_000_000)

        if point.onSpringBoard || point.bundleId == nil || ForegroundProbe.isSpringBoard(bundleId: point.bundleId) {
            parts.append("restore: at SpringBoard")
            return parts.joined(separator: "\n")
        }

        let bid = point.bundleId!
        if point.coldStart && preferColdStart {
            _ = await ProcessRunner.run(["xcrun", "simctl", "terminate", point.udid, bid])
            try? await Task.sleep(nanoseconds: 250_000_000)
        }
        parts.append(await ProcessRunner.run(["xcrun", "simctl", "launch", point.udid, bid]))
        try? await Task.sleep(nanoseconds: 900_000_000)
        parts.append("restore: app ready")
        return parts.joined(separator: "\n")
    }

    private func handleInterrupts(udid: String, bin: String) async -> String? {
        guard let check = await ProcessRunner.jsonObject([bin, "interrupt", "check", "--udid", udid]),
              (check["hasInterrupt"] as? Bool) == true else { return nil }

        let interrupts = check["interrupts"] as? [[String: Any]] ?? []
        let kinds = interrupts.compactMap { $0["kind"] as? String }
        if kinds.allSatisfy({ $0 == "springboard" }) {
            return "interrupt: springboard (continue)"
        }

        switch interruptPolicy {
        case .pauseOnInterrupt:
            pausedForInterrupt = true
            return "PAUSED: interrupt \(kinds) — dismiss manually, then re-run"
        case .declinePermissions, .acceptPermissions:
            let action = interruptPolicy == .acceptPermissions ? "accept" : "decline"
            if let dismissed = await ProcessRunner.jsonObject([bin, "interrupt", "dismiss", "--udid", udid, "--action", action]),
               (dismissed["dismissed"] as? Bool) == true {
                return "interrupt: \(action) → \(dismissed["kind"] ?? kinds)"
            }
            if let dismissed = await ProcessRunner.jsonObject([bin, "interrupt", "dismiss", "--udid", udid, "--action", "dismiss"]),
               (dismissed["dismissed"] as? Bool) == true {
                return "interrupt: dismiss fallback"
            }
            return "interrupt: unresolved \(kinds)"
        }
    }

    private func runStep(_ step: InteractionStep, bin: String) async -> String {
        switch step.kind {
        case .tap:
            guard let x = step.payload["x"], let y = step.payload["y"] else {
                return "skip tap (no coords): \(step.summary)"
            }
            return await ProcessRunner.run(["axe", "tap", "-x", x, "-y", y, "--udid", step.udid])
        case .typeText:
            return await ProcessRunner.run(["axe", "type", step.payload["text"] ?? "", "--udid", step.udid])
        case .appLaunch:
            return await ProcessRunner.run(["xcrun", "simctl", "launch", step.udid, step.payload["bundleId"] ?? ""])
        case .button:
            return await ProcessRunner.run(["axe", "button", step.payload["button"] ?? "home", "--udid", step.udid])
        case .longPress:
            let x = step.payload["x"] ?? "200"
            let y = step.payload["y"] ?? "400"
            let dur = Double(step.payload["duration"] ?? "1") ?? 1
            return await ProcessRunner.run([
                "axe", "tap", "-x", x, "-y", y,
                "--pre-delay", String(format: "%.2f", dur / 2),
                "--post-delay", String(format: "%.2f", dur / 2),
                "--udid", step.udid,
            ])
        case .swipe:
            return await ProcessRunner.run([
                "axe", "swipe",
                "--start-x", step.payload["x1"] ?? "200",
                "--start-y", step.payload["y1"] ?? "600",
                "--end-x", step.payload["x2"] ?? "200",
                "--end-y", step.payload["y2"] ?? "200",
                "--udid", step.udid,
            ])
        case .interruptDismiss:
            let action = step.payload["action"] ?? "decline"
            _ = await ProcessRunner.jsonObject([bin, "interrupt", "dismiss", "--udid", step.udid, "--action", action])
            return "interruptDismiss \(action)"
        case .openURL:
            return await ProcessRunner.run(["xcrun", "simctl", "openurl", step.udid, step.payload["url"] ?? ""])
        case .uiChange, .note:
            return "note: \(step.summary)"
        }
    }
}
