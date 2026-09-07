// THROWAWAY. Persistent arming is the lifecycle behavior under investigation.
chrome.runtime.onInstalled.addListener(async ({reason}) => {
  if (reason === "install") await chrome.storage.local.set({armed: false, stage: "installed"});
});
chrome.runtime.onStartup.addListener(async () => {
  const {armed} = await chrome.storage.local.get("armed");
  if (!armed) return;
  await chrome.storage.local.set({stage: "startup_received"});
  try {
    const existing = await chrome.windows.getAll({windowTypes: ["normal"]});
    if (existing.length !== 0) {
      await chrome.storage.local.set({stage: "existing_window_preserved"});
      return;
    }
    await chrome.storage.local.set({stage: "creation_requested"});
    const window = await chrome.windows.create({
      url: "about:blank#coupangctl-bootstrap-startup", type: "normal",
      state: "minimized", focused: false
    });
    await chrome.storage.local.set({stage: "creation_callback", windowId: window.id,
      reportedState: window.state, reportedFocused: window.focused});
  } catch {
    // Unknown outcome is not retried. No URLs, exception messages or page data.
    await chrome.storage.local.set({stage: "creation_failed_or_uncertain"});
  }
});
