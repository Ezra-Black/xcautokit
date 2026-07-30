import Foundation

enum ForegroundProbe {
    /// Best-effort frontmost app bundle ID on a booted simulator.
    static func bundleId(udid: String) async -> String? {
        await Task.detached {
            let proc = Process()
            proc.executableURL = URL(fileURLWithPath: "/usr/bin/xcrun")
            proc.arguments = ["simctl", "spawn", udid, "launchctl", "list"]
            let out = Pipe()
            proc.standardOutput = out
            proc.standardError = Pipe()
            do {
                try proc.run()
                proc.waitUntilExit()
                let text = String(data: out.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
                return pickForeground(from: text)
            } catch {
                return nil
            }
        }.value
    }

    /// Running `UIKitApplication:<bundle>` rows; prefer non-system with highest PID.
    static func pickForeground(from text: String) -> String? {
        struct Row { var pid: Int; var bundleId: String }
        var rows: [Row] = []
        // "93531	0	UIKitApplication:com.pennywise.ios[408c][rb-legacy]"
        let pattern = #"^(\d+)\s+\S+\s+UIKitApplication:([a-zA-Z0-9.\-]+)\["#
        guard let re = try? NSRegularExpression(pattern: pattern, options: [.anchorsMatchLines]) else {
            return nil
        }
        let range = NSRange(text.startIndex..<text.endIndex, in: text)
        re.enumerateMatches(in: text, range: range) { match, _, _ in
            guard let match, match.numberOfRanges > 2,
                  let pr = Range(match.range(at: 1), in: text),
                  let br = Range(match.range(at: 2), in: text),
                  let pid = Int(text[pr]) else { return }
            rows.append(Row(pid: pid, bundleId: String(text[br])))
        }
        guard !rows.isEmpty else { return nil }

        let systemPrefixes = ["com.apple."]
        let user = rows.filter { row in !systemPrefixes.contains { row.bundleId.hasPrefix($0) } }
        let pool = user.isEmpty ? rows : user
        // Highest PID among running UIKit apps is usually the most recently foregrounded.
        return pool.max(by: { $0.pid < $1.pid })?.bundleId
    }

    static func isSpringBoard(bundleId: String?) -> Bool {
        guard let bundleId else { return true }
        return bundleId == "com.apple.springboard" || bundleId.isEmpty
    }
}
