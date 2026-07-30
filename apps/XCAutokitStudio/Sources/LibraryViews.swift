import SwiftUI

struct BlocksLibraryView: View {
    @EnvironmentObject var library: LibraryStore
    @EnvironmentObject var monitor: SimulatorMonitor
    @EnvironmentObject var replay: ReplayEngine
    @State private var selection = Set<UUID>()
    @State private var buildingBlockName = "Untitled Building Block"

    var body: some View {
        Group {
            if library.library.blocks.isEmpty {
                ContentUnavailableView(
                    "No Blocks",
                    systemImage: "rectangle.stack",
                    description: Text("Select steps in a simulator timeline and choose Save Block.")
                )
            } else {
                List(selection: $selection) {
                    ForEach(library.library.blocks) { block in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(block.name)
                            Text("\(block.steps.count) steps · \(block.restore?.summary ?? "No restore point")")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        .tag(block.id)
                        .contextMenu {
                            Button("Execute") { run(block) }
                            Divider()
                            Button("Delete", role: .destructive) {
                                library.deleteBlock(id: block.id)
                                selection.remove(block.id)
                            }
                        }
                    }
                    .onDelete { indexSet in
                        for i in indexSet {
                            library.deleteBlock(id: library.library.blocks[i].id)
                        }
                    }
                }
            }
        }
        .navigationTitle("Blocks")
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                Button {
                    guard let id = selection.first,
                          let block = library.library.blocks.first(where: { $0.id == id }) else { return }
                    run(block)
                } label: {
                    Label("Execute", systemImage: "play.fill")
                }
                .disabled(selection.count != 1 || replay.isExecuting)

                Button {
                    library.addBuildingBlock(name: buildingBlockName, blockIDs: Array(selection))
                    selection.removeAll()
                } label: {
                    Label("New Building Block", systemImage: "square.stack.3d.up.badge.a")
                }
                .disabled(selection.isEmpty)
            }
        }
        .safeAreaInset(edge: .bottom) {
            if !selection.isEmpty {
                HStack {
                    Text("\(selection.count) selected")
                        .foregroundStyle(.secondary)
                    Spacer()
                    TextField("Building Block name", text: $buildingBlockName)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 260)
                }
                .padding()
                .background(.bar)
            }
        }
    }

    private func run(_ block: Block) {
        Task {
            await replay.execute(
                steps: block.steps,
                bin: monitor.xcautokitPath,
                restore: block.restore ?? RestorePoint.derived(from: block.steps)
            )
        }
    }
}

struct BuildingBlocksLibraryView: View {
    @EnvironmentObject var library: LibraryStore
    @EnvironmentObject var monitor: SimulatorMonitor
    @EnvironmentObject var replay: ReplayEngine
    @State private var selection = Set<UUID>()
    @State private var setName = "Untitled Set"

    var body: some View {
        Group {
            if library.library.buildingBlocks.isEmpty {
                ContentUnavailableView(
                    "No Building Blocks",
                    systemImage: "square.stack.3d.up",
                    description: Text("Select one or more Blocks, then create a Building Block.")
                )
            } else {
                List(selection: $selection) {
                    ForEach(library.library.buildingBlocks) { bb in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(bb.name)
                            Text("\(bb.blockIDs.count) blocks")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        .tag(bb.id)
                    }
                }
            }
        }
        .navigationTitle("Building Blocks")
        .toolbar {
            ToolbarItemGroup(placement: .primaryAction) {
                Button {
                    guard let id = selection.first,
                          let bb = library.library.buildingBlocks.first(where: { $0.id == id }) else { return }
                    let steps = library.steps(for: bb)
                    Task {
                        await replay.execute(
                            steps: steps,
                            bin: monitor.xcautokitPath,
                            restore: bb.restore ?? RestorePoint.derived(from: steps)
                        )
                    }
                } label: {
                    Label("Execute", systemImage: "play.fill")
                }
                .disabled(selection.count != 1 || replay.isExecuting)

                Button {
                    library.addSet(name: setName, buildingBlockIDs: Array(selection))
                    selection.removeAll()
                } label: {
                    Label("New Set", systemImage: "function")
                }
                .disabled(selection.isEmpty)
            }
        }
        .safeAreaInset(edge: .bottom) {
            if !selection.isEmpty {
                HStack {
                    Text("\(selection.count) selected")
                        .foregroundStyle(.secondary)
                    Spacer()
                    TextField("Set name", text: $setName)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 260)
                }
                .padding()
                .background(.bar)
            }
        }
    }
}

struct SetsLibraryView: View {
    @EnvironmentObject var library: LibraryStore
    @EnvironmentObject var monitor: SimulatorMonitor
    @EnvironmentObject var replay: ReplayEngine
    @State private var selection: UUID?

    var body: some View {
        Group {
            if library.library.sets.isEmpty {
                ContentUnavailableView(
                    "No Sets",
                    systemImage: "function",
                    description: Text("Compose Building Blocks into a Set to create a callable function.")
                )
            } else {
                List(library.library.sets, selection: $selection) { set in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(set.name)
                        Text("\(set.buildingBlockIDs.count) building blocks")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .tag(set.id)
                }
            }
        }
        .navigationTitle("Sets")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    guard let selection,
                          let set = library.library.sets.first(where: { $0.id == selection }) else { return }
                    let steps = library.steps(for: set)
                    Task {
                        await replay.execute(
                            steps: steps,
                            bin: monitor.xcautokitPath,
                            restore: set.restore ?? RestorePoint.derived(from: steps)
                        )
                    }
                } label: {
                    Label("Execute Set", systemImage: "play.fill")
                }
                .disabled(selection == nil || replay.isExecuting)
            }
        }
    }
}
