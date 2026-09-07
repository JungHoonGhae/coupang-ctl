async function render() {
  const state = await chrome.storage.local.get(["armed", "stage", "reportedState", "reportedFocused"]);
  document.getElementById("state").textContent = JSON.stringify(state, null, 2);
}
document.getElementById("arm").onclick = async () => {
  await chrome.storage.local.set({armed: true, stage: "armed_for_next_startup"}); await render();
};
document.getElementById("disarm").onclick = async () => {
  await chrome.storage.local.set({armed: false, stage: "disarmed"}); await render();
};
render();
