import ApplicationServices
import AppKit
import CoreGraphics
import Foundation

private enum ProbeError: Error {
    case invalidPID
    case inaccessibleApplication
    case windowMissing
    case controlMissing
    case actionFailed(Int32)
    case phoneUnavailable
    case valueSetFailed(Int32)
    case valueReadbackMismatch
    case outcomeUnverified
    case otpUnavailable
    case windowBoundsMissing
    case submitControlMissing
    case submitDisabled
}

private func attribute(_ element: AXUIElement, _ name: String) -> CFTypeRef? {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success else {
        return nil
    }
    return value
}

private func stringAttribute(_ element: AXUIElement, _ name: String) -> String {
    attribute(element, name) as? String ?? ""
}

private func children(_ element: AXUIElement) -> [AXUIElement] {
    attribute(element, kAXChildrenAttribute) as? [AXUIElement] ?? []
}

private func walk(_ element: AXUIElement, depth: Int = 0, visit: (AXUIElement) -> Void) {
    guard depth <= 16 else { return }
    visit(element)
    for child in children(element) {
        walk(child, depth: depth + 1, visit: visit)
    }
}

private func find(_ root: AXUIElement, where predicate: (AXUIElement) -> Bool) -> AXUIElement? {
    var match: AXUIElement?
    walk(root) { element in
        if match == nil, predicate(element) {
            match = element
        }
    }
    return match
}

private func largestWindow(_ windows: [AXUIElement]) -> AXUIElement? {
    windows.max { lhs, rhs in
        func area(_ element: AXUIElement) -> CGFloat {
            guard let rawSize = attribute(element, kAXSizeAttribute) else { return 0 }
            let sizeValue = rawSize as! AXValue
            var size = CGSize.zero
            guard AXValueGetValue(sizeValue, .cgSize, &size) else { return 0 }
            return size.width * size.height
        }
        return area(lhs) < area(rhs)
    }
}

private func press(_ element: AXUIElement) throws {
    let result = AXUIElementPerformAction(element, kAXPressAction as CFString)
    guard result == .success else { throw ProbeError.actionFailed(result.rawValue) }
}

private func click(_ element: AXUIElement, pid: pid_t) throws {
    guard let rawPosition = attribute(element, kAXPositionAttribute),
          let rawSize = attribute(element, kAXSizeAttribute) else {
        throw ProbeError.controlMissing
    }
    let positionValue = rawPosition as! AXValue
    let sizeValue = rawSize as! AXValue
    var position = CGPoint.zero
    var size = CGSize.zero
    guard AXValueGetValue(positionValue, .cgPoint, &position),
          AXValueGetValue(sizeValue, .cgSize, &size),
          size.width > 0, size.height > 0 else {
        throw ProbeError.controlMissing
    }
    _ = NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
    Thread.sleep(forTimeInterval: 0.15)
    let point = CGPoint(x: position.x + size.width / 2, y: position.y + size.height / 2)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: point, mouseButton: .left)?.post(tap: .cghidEventTap)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: point, mouseButton: .left)?.post(tap: .cghidEventTap)
}

private func classify(_ element: AXUIElement) -> String? {
    let label = [
        stringAttribute(element, kAXTitleAttribute),
        stringAttribute(element, kAXDescriptionAttribute),
        stringAttribute(element, kAXHelpAttribute),
        stringAttribute(element, kAXPlaceholderValueAttribute),
        stringAttribute(element, kAXValueAttribute),
    ].joined(separator: " ")

    if label.contains("휴대폰번호 로그인") { return "phone_login" }
    if label.contains("인증번호 발송") { return "send_otp" }
    if label.contains("재발송") { return "resend_otp" }
    if label.contains("만료") { return "expired" }
    if label.contains("올바르지") || label.contains("일치하지") || label.contains("확인해") { return "verification_error" }
    if label.contains("자동입력 방지문자") { return "human_challenge" }
    if label.contains("시스템 오류") { return "system_error" }
    if label == "닫기" || label.contains("창 닫기") { return "challenge_close" }
    if label.contains("QR코드 로그인") || label.contains("QR로그인") { return "qr_login" }
    if label.contains("휴대폰 카메라로 QR코드를 스캔") || label.contains("선택하면 로그인") || label.contains("남은시간") { return "qr_ready" }
    if label.contains("인증번호") { return "otp_step" }
    if label.contains("휴대폰") || label.contains("전화번호") { return "phone_step" }
    if label.contains("이메일") { return "email_step" }
    return nil
}

private func label(_ element: AXUIElement) -> String {
    [
        stringAttribute(element, kAXTitleAttribute),
        stringAttribute(element, kAXDescriptionAttribute),
        stringAttribute(element, kAXHelpAttribute),
    ].joined(separator: " ").trimmingCharacters(in: .whitespacesAndNewlines)
}

private func digitsOnly(_ value: String) -> String {
    String(value.filter(\.isNumber))
}

private let digitKeyCodes: [Character: CGKeyCode] = [
    "0": 29, "1": 18, "2": 19, "3": 20, "4": 21,
    "5": 23, "6": 22, "7": 26, "8": 28, "9": 25,
]

private func typeDigits(_ digits: String, pid: pid_t) throws {
    for digit in digits {
        guard let keyCode = digitKeyCodes[digit] else { throw ProbeError.valueReadbackMismatch }
        CGEvent(keyboardEventSource: nil, virtualKey: keyCode, keyDown: true)?.postToPid(pid)
        CGEvent(keyboardEventSource: nil, virtualKey: keyCode, keyDown: false)?.postToPid(pid)
        Thread.sleep(forTimeInterval: 0.12)
    }
}

private func navigate(_ url: String, pid: pid_t) {
    _ = NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
    Thread.sleep(forTimeInterval: 0.15)
    let selectAddressDown = CGEvent(keyboardEventSource: nil, virtualKey: 37, keyDown: true)
    selectAddressDown?.flags = .maskCommand
    selectAddressDown?.postToPid(pid)
    let selectAddressUp = CGEvent(keyboardEventSource: nil, virtualKey: 37, keyDown: false)
    selectAddressUp?.flags = .maskCommand
    selectAddressUp?.postToPid(pid)
    Thread.sleep(forTimeInterval: 0.1)
    var characters = Array(url.utf16)
    let typeURL = CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: true)
    typeURL?.keyboardSetUnicodeString(stringLength: characters.count, unicodeString: &characters)
    typeURL?.postToPid(pid)
    CGEvent(keyboardEventSource: nil, virtualKey: 36, keyDown: true)?.postToPid(pid)
    CGEvent(keyboardEventSource: nil, virtualKey: 36, keyDown: false)?.postToPid(pid)
}

private func frontWindowBounds(pid: pid_t) -> CGRect? {
    guard let raw = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] else {
        return nil
    }
    for item in raw {
        guard item[kCGWindowOwnerPID as String] as? Int == Int(pid),
              item[kCGWindowLayer as String] as? Int == 0,
              let bounds = item[kCGWindowBounds as String] as? NSDictionary,
              let rect = CGRect(dictionaryRepresentation: bounds as CFDictionary) else {
            continue
        }
        return rect
    }
    return nil
}

private func enterOTP(_ otp: String, pid: pid_t) throws {
    guard let bounds = frontWindowBounds(pid: pid) else { throw ProbeError.windowBoundsMissing }
    guard let running = NSRunningApplication(processIdentifier: pid) else { throw ProbeError.inaccessibleApplication }
    running.activate(options: [.activateAllWindows])
    Thread.sleep(forTimeInterval: 0.25)

    let inputPoint = CGPoint(x: bounds.minX + bounds.width * 0.48, y: bounds.minY + bounds.height * 0.352)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: inputPoint, mouseButton: .left)?.post(tap: .cghidEventTap)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: inputPoint, mouseButton: .left)?.post(tap: .cghidEventTap)
    Thread.sleep(forTimeInterval: 0.15)

    try typeDigits(otp, pid: pid)
}

private func main() throws {
    guard (2...3).contains(CommandLine.arguments.count), let rawPID = Int32(CommandLine.arguments[1]) else {
        throw ProbeError.invalidPID
    }
    let mode = CommandLine.arguments.count == 3 ? CommandLine.arguments[2] : "inspect"
    let app = AXUIElementCreateApplication(pid_t(rawPID))
    var actualPID: pid_t = 0
    guard AXUIElementGetPid(app, &actualPID) == .success, actualPID == rawPID else {
        throw ProbeError.inaccessibleApplication
    }
    guard let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let window = windows.first else {
        throw ProbeError.windowMissing
    }

    if mode == "open-phone" {
        let control = find(window) { classify($0) == "phone_login" }
        guard let control else { throw ProbeError.controlMissing }
        try press(control)
        Thread.sleep(forTimeInterval: 1.5)
    }

    if mode == "open-qr" {
        if find(window, where: { classify($0) == "human_challenge" }) != nil {
            navigate("https://login.coupang.com/login/login.pang", pid: pid_t(rawPID))
        }
        Thread.sleep(forTimeInterval: 1.2)
        guard let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let currentWindow = windows.first else {
            throw ProbeError.windowMissing
        }
        guard let qrLogin = find(currentWindow, where: { classify($0) == "qr_login" }) else {
            throw ProbeError.controlMissing
        }
        try click(qrLogin, pid: pid_t(rawPID))
        var ready = false
        for _ in 0..<10 {
            Thread.sleep(forTimeInterval: 0.3)
            guard let refreshed = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let first = refreshed.first else {
                continue
            }
            if find(first, where: { classify($0) == "qr_ready" }) != nil {
                ready = true
                break
            }
        }
        guard ready else { throw ProbeError.outcomeUnverified }
    }

    if mode == "request-otp" {
        guard let phone = ProcessInfo.processInfo.environment["COUPANG_PHONE"], !phone.isEmpty else {
            throw ProbeError.phoneUnavailable
        }
        var currentWindow = window
        if find(currentWindow, where: { classify($0) == "send_otp" }) == nil {
            guard let phoneLogin = find(currentWindow, where: { classify($0) == "phone_login" }) else {
                throw ProbeError.controlMissing
            }
            try press(phoneLogin)
            Thread.sleep(forTimeInterval: 1.5)
            guard let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let first = windows.first else {
                throw ProbeError.windowMissing
            }
            currentWindow = first
        }

        let phoneField = find(currentWindow, where: {
            let role = stringAttribute($0, kAXRoleAttribute)
            return role == kAXTextFieldRole && classify($0) == "phone_step"
        }) ?? find(currentWindow, where: {
            let role = stringAttribute($0, kAXRoleAttribute)
            let description = stringAttribute($0, kAXDescriptionAttribute)
            return role == kAXTextFieldRole && !description.contains("주소") && !description.contains("검색")
        })
        guard let phoneField else {
            throw ProbeError.controlMissing
        }
        guard let running = NSRunningApplication(processIdentifier: pid_t(rawPID)) else {
            throw ProbeError.inaccessibleApplication
        }
        running.activate(options: [.activateAllWindows])
        let focusResult = AXUIElementSetAttributeValue(phoneField, kAXFocusedAttribute as CFString, kCFBooleanTrue)
        guard focusResult == .success else {
            throw ProbeError.valueSetFailed(focusResult.rawValue)
        }
        let selectDown = CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: true)
        selectDown?.flags = .maskCommand
        selectDown?.postToPid(pid_t(rawPID))
        let selectUp = CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: false)
        selectUp?.flags = .maskCommand
        selectUp?.postToPid(pid_t(rawPID))
        Thread.sleep(forTimeInterval: 0.15)
        try typeDigits(digitsOnly(phone), pid: pid_t(rawPID))
        Thread.sleep(forTimeInterval: 0.2)
        guard digitsOnly(stringAttribute(phoneField, kAXValueAttribute)) == digitsOnly(phone) else {
            throw ProbeError.valueReadbackMismatch
        }
        guard let send = find(currentWindow, where: { classify($0) == "send_otp" }) else {
            throw ProbeError.controlMissing
        }
        guard attribute(send, kAXEnabledAttribute) as? Bool == true else {
            throw ProbeError.submitDisabled
        }
        try click(send, pid: pid_t(rawPID))

        var verified = false
        for _ in 0..<12 {
            Thread.sleep(forTimeInterval: 0.5)
            guard let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let first = windows.first else {
                continue
            }
            if find(first, where: { classify($0) == "resend_otp" || classify($0) == "otp_step" }) != nil {
                verified = true
                break
            }
        }
        guard verified else { throw ProbeError.outcomeUnverified }
    }

    if mode == "resend-otp" {
        guard let resend = find(window, where: { classify($0) == "resend_otp" }) else {
            throw ProbeError.controlMissing
        }
        try press(resend)
        var verified = false
        for _ in 0..<12 {
            Thread.sleep(forTimeInterval: 0.5)
            guard let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let first = windows.first else {
                continue
            }
            let stillOnOTP = find(first, where: { classify($0) == "resend_otp" }) != nil
            let expired = find(first, where: { classify($0) == "expired" }) != nil
            if stillOnOTP && !expired {
                verified = true
                break
            }
        }
        guard verified else { throw ProbeError.outcomeUnverified }
    }

    if mode == "submit-otp" {
        guard let line = readLine(), line.range(of: #"^[0-9]{6}$"#, options: .regularExpression) != nil else {
            throw ProbeError.otpUnavailable
        }
        try enterOTP(line, pid: pid_t(rawPID))
        Thread.sleep(forTimeInterval: 0.5)
        guard let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let currentWindow = windows.first else {
            throw ProbeError.windowMissing
        }
        guard let submit = find(currentWindow, where: {
            stringAttribute($0, kAXRoleAttribute) == kAXButtonRole && label($0) == "로그인"
        }) else {
            throw ProbeError.submitControlMissing
        }
        guard attribute(submit, kAXEnabledAttribute) as? Bool == true else {
            throw ProbeError.submitDisabled
        }
        try press(submit)
        Thread.sleep(forTimeInterval: 2)
    }

    guard let refreshedWindows = attribute(app, kAXWindowsAttribute) as? [AXUIElement], let refreshedWindow = largestWindow(refreshedWindows) else {
        throw ProbeError.windowMissing
    }
    var counts: [String: Int] = [:]
    var roleCounts: [String: Int] = [:]
    var settableValueRoleCounts: [String: Int] = [:]
    var elementCount = 0
    var pageTextFields = 0
    var settablePageTextFields = 0
    var submitEnabled: Bool?
    var loginSurface = false
    var coupangSurface = false
    var orderSurface = false
    var myCoupangSurface = false
    var orderHostSurface = false
    var phoneFieldCount = 0
    var phoneValueDigitCount = 0
    walk(refreshedWindow) { element in
        elementCount += 1
        let role = stringAttribute(element, kAXRoleAttribute)
        if !role.isEmpty {
            roleCounts[role, default: 0] += 1
            var valueSettable = DarwinBoolean(false)
            if AXUIElementIsAttributeSettable(element, kAXValueAttribute as CFString, &valueSettable) == .success, valueSettable.boolValue {
                settableValueRoleCounts[role, default: 0] += 1
            }
        }
        let description = stringAttribute(element, kAXDescriptionAttribute)
        if role == kAXTextFieldRole, description.contains("주소") || description.contains("검색") {
            let address = stringAttribute(element, kAXValueAttribute)
            loginSurface = address.contains("login.coupang.com")
            coupangSurface = address.contains("coupang.com")
            orderSurface = address.contains("mc.coupang.com/ssr/desktop/order/list")
            myCoupangSurface = address.contains("www.coupang.com/np/mycoupang")
            orderHostSurface = address.contains("mc.coupang.com")
        }
        if role == kAXTextFieldRole, !description.contains("주소") && !description.contains("검색") {
            pageTextFields += 1
            var settable = DarwinBoolean(false)
            if AXUIElementIsAttributeSettable(element, kAXValueAttribute as CFString, &settable) == .success, settable.boolValue {
                settablePageTextFields += 1
            }
        }
        if role == kAXTextFieldRole, classify(element) == "phone_step" {
            phoneFieldCount += 1
            phoneValueDigitCount = max(phoneValueDigitCount, digitsOnly(stringAttribute(element, kAXValueAttribute)).count)
        }
        if role == kAXButtonRole, label(element) == "로그인" {
            submitEnabled = attribute(element, kAXEnabledAttribute) as? Bool
        }
        if let kind = classify(element) {
            counts[kind, default: 0] += 1
        }
    }
    let result: [String: Any] = [
        "pid_verified": true,
        "window_count": refreshedWindows.count,
        "element_count": elementCount,
        "page_text_fields": pageTextFields,
        "settable_page_text_fields": settablePageTextFields,
        "submit_present": submitEnabled != nil,
        "submit_enabled": submitEnabled ?? false,
        "login_surface": loginSurface,
        "coupang_surface": coupangSurface,
        "order_surface": orderSurface,
        "mycoupang_surface": myCoupangSurface,
        "order_host_surface": orderHostSurface,
        "phone_field_count": phoneFieldCount,
        "phone_value_digit_count": phoneValueDigitCount,
        "mode": mode,
        "requested": mode == "request-otp" || mode == "resend-otp",
        "submitted": mode == "submit-otp",
        "ui_transition_verified": mode == "request-otp" || mode == "resend-otp",
        "states": counts,
        "role_counts": roleCounts,
        "settable_value_role_counts": settableValueRoleCounts,
    ]
    let data = try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys])
    print(String(decoding: data, as: UTF8.self))
}

do {
    try main()
} catch {
    let result: [String: Any] = ["ok": false, "error": String(describing: error)]
    let data = try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys])
    FileHandle.standardError.write(data)
    exit(1)
}
