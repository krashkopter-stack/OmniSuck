const $ = id => document.getElementById(id);
const DEF = { quality: "1080", format: "video", folder: "~/Downloads", job: null };
const short = f => f.replace(/^(\/Users\/[^/]+|[A-Za-z]:\\Users\\[^\\]+)/, "~");

const ICON = {
  done: '<svg class="mark" viewBox="0 0 40 40"><circle cx="20" cy="20" r="18" fill="#1f3a26"/><path d="M12 20.5l5.5 5.5L28.5 14" fill="none" stroke="#4ade80" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round"/></svg>',
  error: '<svg class="mark" viewBox="0 0 40 40"><circle cx="20" cy="20" r="18" fill="#3a1f1f"/><path d="M14 14l12 12M26 14L14 26" fill="none" stroke="#f87171" stroke-width="3.5" stroke-linecap="round"/></svg>'
};

function status(state, text, pct) {
  // text on top; below it: progress bar while loading, then a check (or cross) in its place
  $("status").innerHTML = "<div class=\"t\"></div>" +
    (state === "loading" ? `<div class="bar"><i style="width:${pct || 0}%"></i></div>` : (ICON[state] || ""));
  $("status").querySelector(".t").textContent = text || "";
  $("dl").disabled = state === "loading";
}
function showFolder(f) { $("fname").textContent = short(f); $("folder").title = f; }

chrome.storage.local.get(DEF, s => {
  for (const k of ["quality", "format"]) {
    $(k).value = s[k];
    $(k).onchange = () => chrome.storage.local.set({ [k]: $(k).value });
  }
  showFolder(s.folder);
  // show running download, or a fresh result (last 2 minutes)
  if (s.job && (s.job.state === "loading" || Date.now() - s.job.t < 120000)) status(s.job.state, s.job.text, s.job.pct);
});

chrome.storage.onChanged.addListener(ch => {
  if (ch.folder) { showFolder(ch.folder.newValue); status("done", "Папка выбрана"); }
  if (ch.job && ch.job.newValue) status(ch.job.newValue.state, ch.job.newValue.text, ch.job.newValue.pct);
});

$("set").onclick = () => ($("settings").hidden = !$("settings").hidden);
$("folder").onclick = () => {
  status("", "Откроется окно выбора папки…");
  chrome.runtime.sendMessage({ type: "pick" });
};

$("dl").onclick = async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab || !/^https?:/.test(tab.url)) return status("error", "Откройте страницу с видео.");
  status("loading", "Готовлю…");
  await chrome.runtime.sendMessage({ type: "download", tabId: tab.id, url: tab.url });
};
