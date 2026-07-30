import Foundation

// MARK: - Navigation

enum SidebarItem: Hashable {
    case simulator(String) // udid
    case blocks
    case buildingBlocks
    case sets
}

enum LibraryKind: String, CaseIterable, Identifiable {
    case blocks = "Blocks"
    case buildingBlocks = "Building Blocks"
    case sets = "Sets"

    var id: String { rawValue }

    var systemImage: String {
        switch self {
        case .blocks: return "rectangle.stack"
        case .buildingBlocks: return "square.stack.3d.up"
        case .sets: return "function"
        }
    }

    var sidebarItem: SidebarItem {
        switch self {
        case .blocks: return .blocks
        case .buildingBlocks: return .buildingBlocks
        case .sets: return .sets
        }
    }
}

// MARK: - Restore

/// Simulator context at the first recorded step — used to reset before Execute.
struct RestorePoint: Codable, Hashable {
    var udid: String
    var bundleId: String?
    var onSpringBoard: Bool
    var coldStart: Bool
    var capturedAt: Date

    static func springBoard(udid: String, at date: Date = Date()) -> RestorePoint {
        RestorePoint(udid: udid, bundleId: nil, onSpringBoard: true, coldStart: false, capturedAt: date)
    }

    static func app(udid: String, bundleId: String, coldStart: Bool = true, at date: Date = Date()) -> RestorePoint {
        RestorePoint(udid: udid, bundleId: bundleId, onSpringBoard: false, coldStart: coldStart, capturedAt: date)
    }

    static func derived(from steps: [InteractionStep]) -> RestorePoint? {
        guard let first = steps.first(where: \.isReplayable) else { return nil }
        if first.kind == .appLaunch, let bid = first.payload["bundleId"], !bid.isEmpty {
            return .app(udid: first.udid, bundleId: bid, at: first.timestamp)
        }
        if let bid = first.payload["foregroundBundleId"], !bid.isEmpty,
           !ForegroundProbe.isSpringBoard(bundleId: bid) {
            return .app(udid: first.udid, bundleId: bid, at: first.timestamp)
        }
        if first.payload["onSpringBoard"] == "1" {
            return .springBoard(udid: first.udid, at: first.timestamp)
        }
        return RestorePoint(
            udid: first.udid,
            bundleId: first.payload["foregroundBundleId"],
            onSpringBoard: first.payload["onSpringBoard"] == "1" || first.payload["foregroundBundleId"] == nil,
            coldStart: true,
            capturedAt: first.timestamp
        )
    }

    var summary: String {
        if onSpringBoard || bundleId == nil { return "SpringBoard (home)" }
        let id = bundleId ?? ""
        return coldStart ? "cold launch \(id)" : "launch \(id)"
    }
}

// MARK: - Steps & library

struct InteractionStep: Identifiable, Codable, Hashable {
    var id: UUID = UUID()
    var udid: String
    var deviceName: String
    var timestamp: Date
    var kind: StepKind
    var summary: String
    var payload: [String: String]
    var axFingerprint: String?

    var isReplayable: Bool {
        switch kind {
        case .uiChange, .note: return false
        default: return true
        }
    }

    enum StepKind: String, Codable, CaseIterable {
        case tap, swipe, typeText, longPress, button
        case appLaunch, openURL, interruptDismiss, note, uiChange

        var systemImage: String {
            switch self {
            case .tap: return "hand.tap"
            case .swipe: return "hand.draw"
            case .longPress: return "hand.point.up.left"
            case .typeText: return "keyboard"
            case .button: return "button.programmable"
            case .appLaunch: return "arrow.up.app"
            case .openURL: return "link"
            case .interruptDismiss: return "exclamationmark.bubble"
            case .uiChange: return "rectangle.on.rectangle"
            case .note: return "note.text"
            }
        }
    }
}

struct Block: Identifiable, Codable, Hashable {
    var id: UUID = UUID()
    var name: String
    var createdAt: Date = Date()
    var udid: String?
    var steps: [InteractionStep]
    var restore: RestorePoint?
}

struct BuildingBlock: Identifiable, Codable, Hashable {
    var id: UUID = UUID()
    var name: String
    var createdAt: Date = Date()
    var blockIDs: [UUID]
    var restore: RestorePoint?
}

struct AutomationSet: Identifiable, Codable, Hashable {
    var id: UUID = UUID()
    var name: String
    var createdAt: Date = Date()
    var buildingBlockIDs: [UUID]
    var restore: RestorePoint?
}

struct StudioLibrary: Codable {
    var blocks: [Block] = []
    var buildingBlocks: [BuildingBlock] = []
    var sets: [AutomationSet] = []
}

struct BootedSimulator: Identifiable, Hashable {
    var id: String { udid }
    var udid: String
    var name: String
    var runtime: String
    var state: String

    var runtimeShort: String {
        runtime.split(separator: ".").last.map(String.init) ?? runtime
    }
}
