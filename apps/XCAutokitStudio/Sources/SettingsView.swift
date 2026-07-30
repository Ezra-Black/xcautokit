import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var replay: ReplayEngine
    @AppStorage("studio.coldStartRestore") private var coldStartRestore = true
    @AppStorage("studio.autoStartCapture") private var autoStartCapture = false
    @AppStorage("studio.autoStartMonitor") private var autoStartMonitor = true

    var body: some View {
        TabView {
            Form {
                Section {
                    Picker("Interrupt handling", selection: $replay.interruptPolicy) {
                        ForEach(InterruptPolicy.allCases) { policy in
                            Text(policy.label).tag(policy)
                        }
                    }
                    .help("What to do when a system alert appears during replay")

                    Toggle("Cold-start app when restoring", isOn: $coldStartRestore)
                        .help("Terminate and relaunch the app so Execute starts from a clean process")
                } header: {
                    Text("Replay")
                } footer: {
                    Text("Before replaying, Studio returns to SpringBoard and restores the app context from the first recorded step.")
                }

                Section("Launch") {
                    Toggle("Start input recording automatically", isOn: $autoStartCapture)
                    Toggle("Watch simulators automatically", isOn: $autoStartMonitor)
                }
            }
            .formStyle(.grouped)
            .padding()
            .tabItem {
                Label("General", systemImage: "gearshape")
            }
            .frame(width: 480, height: 280)

            Form {
                Section("Permissions") {
                    Text("Input recording requires Accessibility access for XCAutokit Studio.")
                        .foregroundStyle(.secondary)
                    Button("Open Accessibility Settings…") {
                        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
                            NSWorkspace.shared.open(url)
                        }
                    }
                }
                Section("Storage") {
                    LabeledContent("Library") {
                        Text("~/.xcautokit/studio/library.json")
                            .textSelection(.enabled)
                            .font(.caption)
                    }
                    LabeledContent("Videos") {
                        Text("~/Pictures/xcautokit/studio/")
                            .textSelection(.enabled)
                            .font(.caption)
                    }
                }
            }
            .formStyle(.grouped)
            .padding()
            .tabItem {
                Label("Advanced", systemImage: "slider.horizontal.3")
            }
            .frame(width: 480, height: 280)
        }
    }
}
