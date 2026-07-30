import AppKit
import SwiftUI

enum MainWindowChrome {
    static let minSize = NSSize(width: 900, height: 560)

    static func apply(to window: NSWindow?) {
        guard let window else { return }
        // Do not use .fullSizeContentView — it often breaks hit-testing in SwiftUI toolbars.
        window.styleMask.insert([.titled, .closable, .miniaturizable, .resizable])
        window.collectionBehavior.insert([.fullScreenPrimary, .managed])
        window.minSize = minSize
        window.contentMinSize = minSize
        window.isReleasedWhenClosed = false
        window.setFrameAutosaveName("XCAutokitStudio.Main")
    }

    static func applyToKeyWindows() {
        for window in NSApp.windows where window.isVisible && !(window is NSPanel) {
            // Skip Settings / utility panels.
            if String(describing: type(of: window)).contains("Settings") { continue }
            apply(to: window)
        }
    }
}

extension View {
    /// Resize + full-screen support without inserting an AppKit view into the SwiftUI hierarchy
    /// (an NSViewRepresentable background was eating clicks and forcing an I-beam cursor).
    func macOSMainWindowChrome() -> some View {
        self
            .frame(minWidth: MainWindowChrome.minSize.width, minHeight: MainWindowChrome.minSize.height)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .onAppear {
                DispatchQueue.main.async {
                    MainWindowChrome.applyToKeyWindows()
                }
            }
            .onReceive(NotificationCenter.default.publisher(for: NSWindow.didBecomeKeyNotification)) { note in
                MainWindowChrome.apply(to: note.object as? NSWindow)
            }
    }
}
