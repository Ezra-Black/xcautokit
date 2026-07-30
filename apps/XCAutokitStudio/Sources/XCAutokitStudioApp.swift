import SwiftUI

@main
struct XCAutokitStudioApp: App {
    @StateObject private var monitor = SimulatorMonitor()
    @StateObject private var library = LibraryStore()
    @StateObject private var replay = ReplayEngine()
    @StateObject private var capture = InteractionCapture()
    @StateObject private var video = VideoRecorder()

    @AppStorage("studio.autoStartCapture") private var autoStartCapture = false
    @AppStorage("studio.autoStartMonitor") private var autoStartMonitor = true

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(monitor)
                .environmentObject(library)
                .environmentObject(replay)
                .environmentObject(capture)
                .environmentObject(video)
                .macOSMainWindowChrome()
                .onAppear(perform: wireServices)
        }
        .defaultSize(width: 1100, height: 720)
        .windowResizability(.contentMinSize)
        .commands {
            CommandGroup(replacing: .newItem) {}

            CommandMenu("Record") {
                Button(capture.isCapturing ? "Stop Recording Input" : "Record Input") {
                    if capture.isCapturing {
                        capture.stop()
                    } else {
                        capture.start(monitor: monitor)
                    }
                }
                .keyboardShortcut("r", modifiers: [.command, .shift])
            }

            CommandMenu("Replay") {
                ForEach(InterruptPolicy.allCases) { policy in
                    Button {
                        replay.interruptPolicy = policy
                    } label: {
                        HStack {
                            Text(policy.label)
                            if replay.interruptPolicy == policy {
                                Image(systemName: "checkmark")
                            }
                        }
                    }
                }
            }
        }

        Settings {
            SettingsView()
                .environmentObject(replay)
        }
    }

    private func wireServices() {
        monitor.capture = capture
        replay.onExecutingChange = { executing in
            capture.suppressCapture = executing
        }
        library.load()
        capture.stop()
        if autoStartMonitor, !monitor.isRunning {
            monitor.start()
        }
        if autoStartCapture {
            capture.start(monitor: monitor)
        }
    }
}
