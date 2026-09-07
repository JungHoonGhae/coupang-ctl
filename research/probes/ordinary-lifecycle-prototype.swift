// THROWAWAY: launch experiment, not a production lifecycle manager.
// Only a newly created scratch profile is allowed. Never reads page titles/content.
import AppKit
import ApplicationServices

let args = CommandLine.arguments
guard args.count == 3, ["hidden", "hidden-minimized", "no-window", "prepare-minimized", "restore-minimized", "visible-control", "make-minimized", "make-invisible-minimized", "make-invisible"].contains(args[1]),
      args[2].hasPrefix("/tmp/coupangctl-lifecycle-profile."),
      FileManager.default.fileExists(atPath: args[2]) else { exit(2) }
let mode = args[1], profile = args[2]
let workspace = NSWorkspace.shared
let before = Set(workspace.runningApplications.map { $0.processIdentifier })
let initialFront = workspace.frontmostApplication?.processIdentifier
let start = Date()
var launched: NSRunningApplication?
var launchError = false, terminateAccepted = false
var records = [[String: Any]](), activations = [[String: Any]]()
var lastSample = start, maxGap = 0.0
func age() -> Double { Date().timeIntervalSince(start) }
func emit(_ value: [String: Any]) {
    let data = try! JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
    print(String(data: data, encoding: .utf8)!)
    fflush(stdout)
}
let token = workspace.notificationCenter.addObserver(forName: NSWorkspace.didActivateApplicationNotification, object: nil, queue: .main) { n in
    if let app = n.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication {
        activations.append(["at": age(), "pid": Int(app.processIdentifier)])
    }
}
let sampler = Timer.scheduledTimer(withTimeInterval: 0.03, repeats: true) { _ in
    maxGap = max(maxGap, Date().timeIntervalSince(lastSample)); lastSample = Date()
    let apps = workspace.runningApplications.filter { $0.bundleIdentifier == "com.google.Chrome" && !before.contains($0.processIdentifier) }
    let windows = (CGWindowListCopyWindowInfo([.optionAll, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]]) ?? []
    for app in apps {
        let own = windows.filter { ($0[kCGWindowOwnerPID as String] as? Int) == Int(app.processIdentifier) && ($0[kCGWindowLayer as String] as? Int) == 0 }
        let visible = own.filter {
            let bounds = $0[kCGWindowBounds as String] as? [String: Double] ?? [:]
            return ($0[kCGWindowIsOnscreen as String] as? Bool) == true && (bounds["Width"] ?? 0) > 0 && (bounds["Height"] ?? 0) > 0
        }
        let onScreen = visible.count
        var value: CFTypeRef?
        let ax = AXUIElementCopyAttributeValue(AXUIElementCreateApplication(app.processIdentifier), kAXWindowsAttribute as CFString, &value)
        var minimized = [Bool]()
        if ax == .success, let ws = value as? [AXUIElement] {
            for w in ws {
                var v: CFTypeRef?
                if AXUIElementCopyAttributeValue(w, kAXMinimizedAttribute as CFString, &v) == .success, let b = v as? Bool { minimized.append(b) }
            }
        }
        records.append(["at": age(), "pid": Int(app.processIdentifier), "onscreen": onScreen,
                        "cg_windows": own.count, "hidden": app.isHidden,
                        "active": app.isActive, "ax_status": ax.rawValue, "minimized": minimized,
                        "visible_sizes": visible.map { w -> [String: Double] in
                            let b = w[kCGWindowBounds as String] as? [String: Double] ?? [:]
                            return ["width": b["Width"] ?? 0, "height": b["Height"] ?? 0]
                        }])
    }
}
DispatchQueue.main.asyncAfter(deadline: .now() + 1) {
    let config = NSWorkspace.OpenConfiguration()
    config.activates = mode == "visible-control"
    config.hides = mode != "visible-control"
    config.createsNewApplicationInstance = true
    config.arguments = ["--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check", "--disable-sync"]
    if mode == "hidden-minimized" || mode == "restore-minimized" { config.arguments.append("--start-minimized") }
    if mode == "no-window" || mode.hasPrefix("make-") { config.arguments.append("--no-startup-window") }
    else if mode == "restore-minimized" { config.arguments.append("--restore-last-session") }
    else { config.arguments.append("about:blank") }
    workspace.openApplication(at: URL(fileURLWithPath: "/Applications/Google Chrome.app"), configuration: config) { app, error in
        launched = app; launchError = error != nil
        emit(["stage": "launch_callback", "at": age(), "pid": app.map { Int($0.processIdentifier) } ?? 0, "error": launchError])
    }
}
DispatchQueue.main.asyncAfter(deadline: .now() + 4) {
    if mode.hasPrefix("make-"), let app = launched, !before.contains(app.processIdentifier) {
        let command = Process(), output = Pipe()
        command.executableURL = URL(fileURLWithPath: FileManager.default.currentDirectoryPath + "/ordinary-window-create-prototype")
        command.arguments = [String(app.processIdentifier), profile, mode]
        command.standardOutput = output; command.standardError = FileHandle.nullDevice
        command.terminationHandler = { p in
            let data = output.fileHandleForReading.readDataToEndOfFile()
            let result = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] ?? ["status": "invalid_helper_result"]
            DispatchQueue.main.async { emit(["stage": "window_creation", "at": age(), "exit": p.terminationStatus, "result": result]) }
        }
        do { try command.run() } catch { emit(["stage": "window_creation", "error": true]) }
        DispatchQueue.main.asyncAfter(deadline: .now() + 4) { if command.isRunning { command.terminate() } }
    }
    if mode == "prepare-minimized", let app = launched, !before.contains(app.processIdentifier) {
        var value: CFTypeRef?
        let result = AXUIElementCopyAttributeValue(AXUIElementCreateApplication(app.processIdentifier), kAXWindowsAttribute as CFString, &value)
        if result == .success, let windows = value as? [AXUIElement] {
            let results = windows.map { AXUIElementSetAttributeValue($0, kAXMinimizedAttribute as CFString, kCFBooleanTrue).rawValue }
            emit(["stage": "initial_scratch_setup_minimize", "results": results])
        } else { emit(["stage": "initial_scratch_setup_minimize", "ax_status": result.rawValue]) }
    }
}
DispatchQueue.main.asyncAfter(deadline: .now() + 9) {
    // Only the fresh instance returned by our launch is eligible for termination.
    if let app = launched, !before.contains(app.processIdentifier) { terminateAccepted = app.terminate() }
}
DispatchQueue.main.asyncAfter(deadline: .now() + 12) {
    sampler.invalidate(); workspace.notificationCenter.removeObserver(token)
    let pid = launched.map { Int($0.processIdentifier) } ?? 0
    let own = records.filter { ($0["pid"] as? Int) == pid }
    let frontEvents = activations.filter { ($0["pid"] as? Int) == pid }
    emit(["stage": "summary", "mode": mode, "launch_error": launchError, "pid": pid,
          "samples": own.count, "sample_max_gap_seconds": maxGap, "ax_trusted": AXIsProcessTrusted(),
          "screen_capture_preflight": CGPreflightScreenCaptureAccess(),
          "onscreen_samples": own.filter { ($0["onscreen"] as? Int ?? 0) > 0 }.count,
          "active_samples": own.filter { ($0["active"] as? Bool) == true }.count,
          "activation_events": frontEvents, "first": own.first ?? [:], "last": own.last ?? [:],
          "first_onscreen": own.first { ($0["onscreen"] as? Int ?? 0) > 0 } ?? [:],
          "unminimized_samples": own.filter { ($0["minimized"] as? [Bool] ?? []).contains(false) }.count,
          "terminate_accepted": terminateAccepted, "terminated": launched?.isTerminated ?? false,
          "final_front_same": workspace.frontmostApplication?.processIdentifier == initialFront])
    exit(0)
}
RunLoop.main.run()
