// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "XCAutokitStudio",
    platforms: [.macOS(.v14)],
    products: [
        .executable(name: "XCAutokitStudio", targets: ["XCAutokitStudio"]),
    ],
    targets: [
        .executableTarget(
            name: "XCAutokitStudio",
            path: "Sources"
        ),
    ]
)
