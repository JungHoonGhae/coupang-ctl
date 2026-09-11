// Diagnostic helper for chrome-apple-events.mjs; not a production/browser API.
// Use ScriptingBridge's PID address, not JXA Application(pid), which can resolve
// another instance of the same app. This file runs under osascript, not Node.
ObjC.import("ScriptingBridge");
function run(argv) {
    try {
        const input = JSON.parse(argv[0]);
        const app = $.SBApplication.applicationWithProcessIdentifier(input.pid);
        // ScriptingBridge uses 60 Hz ticks, not seconds, for reply timeouts.
        app.timeout = 8 * 60;
        function check(object) {
            const error = object.lastError;
            if (error && Number(error.code)) {
                const code = Number(error.code);
                throw {reason:code === 12 ? "javascript_permission_required" : "apple_event_failed", code};
            }
        }
        function get(object, key) {
            const value = object.valueForKey(key); check(object); return value;
        }
        const windows = get(app, "windows");
        if (Number(windows.count) !== 1) throw {reason:"expected_one_window"};
        const window = windows.objectAtIndex(0);
        const tabs = get(window, "tabs");
        if (Number(tabs.count) !== 1) throw {reason:"expected_one_tab"};
        const tab = tabs.objectAtIndex(0);
        function state() {
            return {window_id:ObjC.unwrap(get(window,"id")), tab_id:ObjC.unwrap(get(tab,"id")),
                minimized:Boolean(ObjC.unwrap(get(window,"minimized")))};
        }
        const before = state();
        if (input.window_id && (before.window_id !== input.window_id || before.tab_id !== input.tab_id)) {
            throw {reason:"target_changed"};
        }
        const url = ObjC.unwrap(get(tab,"URL"));
        if (url !== "about:blank" && !/^https:\/\/www\.coupang\.com\/np\/search\?/.test(url)) {
            throw {reason:"unrelated_tab"};
        }
        switch (input.operation) {
        case "status": return JSON.stringify({status:"ok",...before});
        case "minimize":
            window.setValueForKey($.NSNumber.numberWithBool(true),"minimized"); check(window);
            return JSON.stringify({status:"ok",...state()});
        case "permission": {
            const value = tab.performSelectorWithObject("executeJavascript:",$("JSON.stringify({probe:4})"));
            check(tab);
            return JSON.stringify({status:ObjC.unwrap(value) === '{"probe":4}' ? "ok" : "javascript_unavailable",...state()});
        }
        case "navigate":
            if (!before.minimized) throw {reason:"window_not_minimized"};
            if (!/^https:\/\/www\.coupang\.com\/np\/search\?/.test(input.url)) throw {reason:"invalid_operation"};
            // Keep diagnostic navigation aligned with the quiet production path.
            // The native URL setter restored the window in live verification.
            const navigation = tab.performSelectorWithObject("executeJavascript:",
                $("(function(){window.location.assign("+JSON.stringify(input.url)+");return 'navigation_requested';})()"));
            check(tab);
            if (ObjC.unwrap(navigation) !== "navigation_requested") throw {reason:"invalid_result"};
            return JSON.stringify({status:"ok",...state()});
        case "read": {
            if (!before.minimized) throw {reason:"window_not_minimized"};
            const value = tab.performSelectorWithObject("executeJavascript:",$(input.script)); check(tab);
            const result = ObjC.unwrap(value);
            if (typeof result !== "string" || result.length > 128000) throw {reason:"invalid_result"};
            return JSON.stringify({status:"ok",...state(),result:JSON.parse(result)});
        }
        default: throw {reason:"invalid_operation"};
        }
    } catch (error) {
        // Never return native descriptions, page bodies, or source snippets.
        return JSON.stringify({status:"unavailable",reason:error.reason || "transport_failed",code:error.code || 0});
    }
}
