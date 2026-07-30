import Foundation

@MainActor
final class LibraryStore: ObservableObject {
    @Published var library = StudioLibrary()

    private var url: URL {
        let dir = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent(".xcautokit/studio", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("library.json")
    }

    func load() {
        guard let data = try? Data(contentsOf: url) else { return }
        let dec = JSONDecoder()
        dec.dateDecodingStrategy = .iso8601
        if let decoded = try? dec.decode(StudioLibrary.self, from: data) {
            library = decoded
        }
    }

    func save() {
        let enc = JSONEncoder()
        enc.outputFormatting = [.prettyPrinted, .sortedKeys]
        enc.dateEncodingStrategy = .iso8601
        guard let data = try? enc.encode(library) else { return }
        try? data.write(to: url, options: .atomic)
    }

    func addBlock(_ block: Block) {
        library.blocks.insert(block, at: 0)
        save()
    }

    func addBuildingBlock(name: String, blockIDs: [UUID]) {
        let restore = blockIDs.compactMap { id in library.blocks.first(where: { $0.id == id })?.restore }.first
        library.buildingBlocks.insert(
            BuildingBlock(name: name, blockIDs: blockIDs, restore: restore),
            at: 0
        )
        save()
    }

    func addSet(name: String, buildingBlockIDs: [UUID]) {
        let restore = buildingBlockIDs.compactMap { id in library.buildingBlocks.first(where: { $0.id == id })?.restore }.first
        library.sets.insert(
            AutomationSet(name: name, buildingBlockIDs: buildingBlockIDs, restore: restore),
            at: 0
        )
        save()
    }

    func deleteBlock(id: UUID) {
        library.blocks.removeAll { $0.id == id }
        save()
    }

    func steps(for buildingBlock: BuildingBlock) -> [InteractionStep] {
        buildingBlock.blockIDs.flatMap { id in
            library.blocks.first(where: { $0.id == id })?.steps ?? []
        }
    }

    func steps(for set: AutomationSet) -> [InteractionStep] {
        set.buildingBlockIDs.flatMap { id -> [InteractionStep] in
            guard let bb = library.buildingBlocks.first(where: { $0.id == id }) else { return [] }
            return steps(for: bb)
        }
    }
}
