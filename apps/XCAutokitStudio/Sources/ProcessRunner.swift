import Foundation

enum ProcessRunner {
    /// Run a command via `/usr/bin/env` and return a short status line.
    static func run(_ args: [String]) async -> String {
        await Task.detached {
            let p = Process()
            p.executableURL = URL(fileURLWithPath: "/usr/bin/env")
            p.arguments = args
            let out = Pipe()
            let err = Pipe()
            p.standardOutput = out
            p.standardError = err
            do {
                try p.run()
                p.waitUntilExit()
                let o = String(data: out.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
                let e = String(data: err.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
                let snippet = (o + e).trimmingCharacters(in: .whitespacesAndNewlines)
                let clipped = snippet.prefix(80)
                return "\(args.joined(separator: " ")) → \(p.terminationStatus)\(clipped.isEmpty ? "" : " \(clipped)")"
            } catch {
                return "error: \(error.localizedDescription)"
            }
        }.value
    }

    static func data(_ args: [String]) async -> Data? {
        await Task.detached {
            let p = Process()
            p.executableURL = URL(fileURLWithPath: "/usr/bin/env")
            p.arguments = args
            let out = Pipe()
            p.standardOutput = out
            p.standardError = Pipe()
            do {
                try p.run()
                p.waitUntilExit()
                return out.fileHandleForReading.readDataToEndOfFile()
            } catch {
                return nil
            }
        }.value
    }

    static func jsonObject(_ args: [String]) async -> [String: Any]? {
        guard let data = await data(args), !data.isEmpty else { return nil }
        return (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
    }
}
