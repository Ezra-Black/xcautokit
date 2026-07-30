import AppKit
import CoreGraphics
import Foundation

struct SimulatorWindowHit: Sendable {
    var udid: String
    var deviceName: String
    var windowBounds: CGRect
    /// Device screen rect inside the Simulator window (Quartz / top-left global coords).
    var contentBounds: CGRect
    var deviceWidth: CGFloat
    var deviceHeight: CGFloat
}

enum SimulatorWindowMap {
    /// Build hits for booted sims by matching Simulator windows to devices.
    static func hits(for sims: [BootedSimulator], screenSizes: [String: CGSize]) -> [SimulatorWindowHit] {
        let options: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
        guard let info = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] else {
            return []
        }

        struct Candidate {
            var title: String
            var bounds: CGRect
            var area: CGFloat
        }
        var candidates: [Candidate] = []
        for win in info {
            guard let owner = win[kCGWindowOwnerName as String] as? String,
                  owner == "Simulator" || owner == "iOS Simulator" else { continue }
            guard let boundsDict = win[kCGWindowBounds as String] as? [String: Any] else { continue }
            let bounds = CGRect(
                x: cgFloat(boundsDict["X"]),
                y: cgFloat(boundsDict["Y"]),
                width: cgFloat(boundsDict["Width"]),
                height: cgFloat(boundsDict["Height"])
            )
            // Ignore tiny chrome / menu windows.
            guard bounds.width > 200, bounds.height > 200 else { continue }
            let title = (win[kCGWindowName as String] as? String) ?? ""
            candidates.append(Candidate(title: title, bounds: bounds, area: bounds.width * bounds.height))
        }
        // Prefer largest windows first (the device frame, not inspectors).
        candidates.sort { $0.area > $1.area }

        var hits: [SimulatorWindowHit] = []
        var usedUDIDs = Set<String>()
        for candidate in candidates {
            guard let sim = matchSim(title: candidate.title, sims: sims, excluding: usedUDIDs) else { continue }
            usedUDIDs.insert(sim.udid)
            let size = screenSizes[sim.udid] ?? CGSize(width: 393, height: 852)
            let content = contentRect(window: candidate.bounds, device: size)
            hits.append(SimulatorWindowHit(
                udid: sim.udid,
                deviceName: sim.name,
                windowBounds: candidate.bounds,
                contentBounds: content,
                deviceWidth: size.width,
                deviceHeight: size.height
            ))
        }

        // Last resort: one booted sim + one large Simulator window with empty title
        // (window titles require Screen Recording permission).
        if hits.isEmpty, sims.count == 1, let best = candidates.first {
            let sim = sims[0]
            let size = screenSizes[sim.udid] ?? CGSize(width: 393, height: 852)
            let content = contentRect(window: best.bounds, device: size)
            hits.append(SimulatorWindowHit(
                udid: sim.udid,
                deviceName: sim.name,
                windowBounds: best.bounds,
                contentBounds: content,
                deviceWidth: size.width,
                deviceHeight: size.height
            ))
        }
        return hits
    }

    private static func cgFloat(_ any: Any?) -> CGFloat {
        if let n = any as? CGFloat { return n }
        if let n = any as? Double { return CGFloat(n) }
        if let n = any as? Int { return CGFloat(n) }
        if let n = any as? NSNumber { return CGFloat(truncating: n) }
        return 0
    }

    private static func matchSim(title: String, sims: [BootedSimulator], excluding: Set<String>) -> BootedSimulator? {
        let available = sims.filter { !excluding.contains($0.udid) }
        guard !available.isEmpty else { return nil }
        if !title.isEmpty {
            for sim in available where title.localizedCaseInsensitiveContains(sim.name) {
                return sim
            }
        }
        if available.count == 1 { return available[0] }
        return nil
    }

    private static func contentRect(window: CGRect, device: CGSize) -> CGRect {
        // Generous insets — better to accept chrome clicks than miss device taps.
        let inset = NSEdgeInsets(top: 40, left: 8, bottom: 8, right: 8)
        let available = CGRect(
            x: window.minX + inset.left,
            y: window.minY + inset.top,
            width: max(1, window.width - inset.left - inset.right),
            height: max(1, window.height - inset.top - inset.bottom)
        )
        let deviceAspect = device.width / max(device.height, 1)
        let availAspect = available.width / max(available.height, 1)
        if availAspect > deviceAspect {
            let h = available.height
            let w = h * deviceAspect
            return CGRect(x: available.midX - w / 2, y: available.minY, width: w, height: h)
        } else {
            let w = available.width
            let h = w / deviceAspect
            return CGRect(x: available.minX, y: available.midY - h / 2, width: w, height: h)
        }
    }

    /// Convert a global mouse point (Cocoa bottom-left OR Quartz top-left) into device points.
    static func devicePoint(globalCocoa: CGPoint? = nil, globalQuartz: CGPoint? = nil, hit: SimulatorWindowHit) -> CGPoint? {
        let quartz: CGPoint
        if let globalQuartz {
            quartz = globalQuartz
        } else if let globalCocoa {
            quartz = cocoaToQuartz(globalCocoa)
        } else {
            return nil
        }
        // Prefer precise content rect; fall back to full window so taps aren't dropped.
        let rect = hit.contentBounds.contains(quartz) ? hit.contentBounds : hit.windowBounds
        guard rect.contains(quartz), rect.width > 0, rect.height > 0 else { return nil }
        let nx = (quartz.x - rect.minX) / rect.width
        let ny = (quartz.y - rect.minY) / rect.height
        return CGPoint(
            x: min(max(nx, 0), 1) * hit.deviceWidth,
            y: min(max(ny, 0), 1) * hit.deviceHeight
        )
    }

    static func cocoaToQuartz(_ p: CGPoint) -> CGPoint {
        let screenH = NSScreen.screens.map(\.frame.maxY).max() ?? 0
        return CGPoint(x: p.x, y: screenH - p.y)
    }

    static func hitContaining(cocoaPoint: CGPoint, hits: [SimulatorWindowHit]) -> SimulatorWindowHit? {
        let q = cocoaToQuartz(cocoaPoint)
        return hits.first(where: { $0.contentBounds.contains(q) || $0.windowBounds.contains(q) })
    }
}
