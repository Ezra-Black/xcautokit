import AppKit
import ApplicationServices
import Foundation

/// Records mouse/keyboard actions from Simulator.app into the timeline.
@MainActor
final class InteractionCapture: ObservableObject {
    @Published var isCapturing = false
    @Published var accessibilityTrusted = false
    @Published var lastError: String?
    @Published var liveStatus = "Off"

    weak var monitor: SimulatorMonitor?
    var suppressCapture = false

    private var monitors: [Any] = []
    private var dragStart: (hit: SimulatorWindowHit, point: CGPoint, time: Date)?
    private var typingBuffer: (udid: String, deviceName: String, text: String, last: Date)?
    private var flushTask: Task<Void, Never>?
    private var screenSizes: [String: CGSize] = [:]
    private var eventCount = 0

    func requestAccess() {
        let opts = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
        accessibilityTrusted = AXIsProcessTrustedWithOptions(opts)
    }

    func start(monitor: SimulatorMonitor) {
        self.monitor = monitor
        requestAccess()
        guard accessibilityTrusted else {
            isCapturing = false
            liveStatus = "Needs Accessibility permission"
            lastError = "Grant Accessibility access to “XCAutokit Studio” in System Settings → Privacy & Security → Accessibility, then click Record Input again."
            return
        }

        stopMonitors()
        guard let monitorRef = NSEvent.addGlobalMonitorForEvents(
            matching: [.leftMouseDown, .leftMouseUp, .keyDown],
            handler: { [weak self] event in
                Task { @MainActor in self?.handle(event) }
            }
        ) else {
            lastError = "Could not start input monitor. Check Accessibility permission for XCAutokit Studio."
            liveStatus = "Failed to start"
            isCapturing = false
            return
        }

        monitors = [monitorRef]
        isCapturing = true
        lastError = nil
        eventCount = 0
        liveStatus = "Listening — click in Simulator"
    }

    func stop() {
        isCapturing = false
        liveStatus = "Off"
        stopMonitors()
        flushTyping(force: true)
        dragStart = nil
    }

    func updateScreenSize(udid: String, size: CGSize) {
        if size.width > 0, size.height > 0 { screenSizes[udid] = size }
    }

    private func stopMonitors() {
        monitors.forEach { NSEvent.removeMonitor($0) }
        monitors = []
    }

    private func handle(_ event: NSEvent) {
        guard isCapturing, !suppressCapture, let monitor else { return }
        guard isSimulatorFrontmost() else {
            liveStatus = "Waiting for Simulator to be frontmost"
            return
        }

        let hits = SimulatorWindowMap.hits(for: monitor.simulators, screenSizes: screenSizes)
        guard !hits.isEmpty else {
            liveStatus = "Can't see Simulator window (is it open?)"
            return
        }

        switch event.type {
        case .leftMouseDown:
            let loc = NSEvent.mouseLocation
            if let hit = SimulatorWindowMap.hitContaining(cocoaPoint: loc, hits: hits),
               let pt = SimulatorWindowMap.devicePoint(globalCocoa: loc, hit: hit) {
                dragStart = (hit, pt, Date())
                liveStatus = String(format: "Down (%.0f, %.0f)", pt.x, pt.y)
            } else {
                dragStart = nil
                liveStatus = "Click missed device area — try the phone screen"
            }

        case .leftMouseUp:
            defer { dragStart = nil }
            guard let start = dragStart else { return }
            let loc = NSEvent.mouseLocation
            guard let end = SimulatorWindowMap.devicePoint(globalCocoa: loc, hit: start.hit) else {
                liveStatus = "Release outside device — ignored"
                return
            }
            let dt = Date().timeIntervalSince(start.time)
            let dist = hypot(end.x - start.point.x, end.y - start.point.y)
            flushTyping(force: true)

            if dist < 14, dt < 0.55 {
                emit(tap: end, from: start, kind: .tap, summary: String(format: "tap (%.0f, %.0f)", end.x, end.y), extra: [:])
            } else if dist < 14 {
                emit(
                    tap: end, from: start, kind: .longPress,
                    summary: String(format: "long press (%.0f, %.0f)", end.x, end.y),
                    extra: ["duration": String(format: "%.2f", dt)]
                )
            } else {
                emit(InteractionStep(
                    udid: start.hit.udid,
                    deviceName: start.hit.deviceName,
                    timestamp: Date(),
                    kind: .swipe,
                    summary: String(format: "swipe (%.0f,%.0f)→(%.0f,%.0f)", start.point.x, start.point.y, end.x, end.y),
                    payload: [
                        "x1": "\(Int(start.point.x.rounded()))",
                        "y1": "\(Int(start.point.y.rounded()))",
                        "x2": "\(Int(end.x.rounded()))",
                        "y2": "\(Int(end.y.rounded()))",
                    ]
                ))
            }

        case .keyDown:
            guard let chars = event.charactersIgnoringModifiers, !chars.isEmpty else { return }
            let hit = SimulatorWindowMap.hitContaining(cocoaPoint: NSEvent.mouseLocation, hits: hits) ?? hits.first
            guard let hit else { return }
            if chars == "\r" || chars == "\n" {
                flushTyping(force: true)
                return
            }
            if chars == "\u{7f}" {
                if var buf = typingBuffer, buf.udid == hit.udid, !buf.text.isEmpty {
                    buf.text.removeLast()
                    buf.last = Date()
                    typingBuffer = buf
                }
                return
            }
            if typingBuffer?.udid == hit.udid {
                typingBuffer?.text += chars
                typingBuffer?.last = Date()
            } else {
                flushTyping(force: true)
                typingBuffer = (hit.udid, hit.deviceName, chars, Date())
            }
            scheduleTypingFlush()
            liveStatus = "Typing…"

        default:
            break
        }
    }

    private func emit(
        tap end: CGPoint,
        from start: (hit: SimulatorWindowHit, point: CGPoint, time: Date),
        kind: InteractionStep.StepKind,
        summary: String,
        extra: [String: String]
    ) {
        var payload = [
            "x": "\(Int(end.x.rounded()))",
            "y": "\(Int(end.y.rounded()))",
        ]
        extra.forEach { payload[$0.key] = $0.value }
        emit(InteractionStep(
            udid: start.hit.udid,
            deviceName: start.hit.deviceName,
            timestamp: Date(),
            kind: kind,
            summary: summary,
            payload: payload
        ))
    }

    private func isSimulatorFrontmost() -> Bool {
        guard let front = NSWorkspace.shared.frontmostApplication?.bundleIdentifier else { return false }
        return front == "com.apple.iphonesimulator"
            || front == "com.apple.CoreSimulator.SimulatorTrampoline"
    }

    private func scheduleTypingFlush() {
        flushTask?.cancel()
        flushTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 700_000_000)
            flushTyping(force: false)
        }
    }

    private func flushTyping(force: Bool) {
        guard let buf = typingBuffer else { return }
        if !force, Date().timeIntervalSince(buf.last) < 0.65 { return }
        typingBuffer = nil
        let text = buf.text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        emit(InteractionStep(
            udid: buf.udid,
            deviceName: buf.deviceName,
            timestamp: Date(),
            kind: .typeText,
            summary: "type \"\(text.prefix(40))\"",
            payload: ["text": text]
        ))
    }

    private func emit(_ step: InteractionStep) {
        eventCount += 1
        liveStatus = "Recorded \(eventCount) action(s)"
        monitor?.appendManualStep(step)
    }
}
