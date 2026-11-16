// swift-tools-version:5.5

import PackageDescription

let package = Package(
  name: "tauri-plugin-kiestoon-call",
  platforms: [
    .iOS(.v16)
  ],
  products: [
    .library(
      name: "tauri-plugin-kiestoon-call",
      type: .static,
      targets: ["tauri-plugin-kiestoon-call"])
  ],
  dependencies: [
    .package(name: "Tauri", path: "../.tauri/tauri-api")
  ],
  targets: [
    .target(
      name: "tauri-plugin-kiestoon-call",
      dependencies: [
        .byName(name: "Tauri")
      ],
      path: "Sources")
  ]
)
