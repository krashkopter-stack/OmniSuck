const DEF = { quality: "1080", format: "video", folder: "~/Downloads" };
const HOST = "com.omnisuck.helper";

chrome.runtime.onMessage.addListener((m, _s, reply) => {
  if (m.type === "download") { start(m).then(reply); return true; }
  if (m.type === "pick") pick();
});

async function start({ tabId, url }) {
  const s = await chrome.storage.local.get(DEF);
  let job = null;
  // Instagram post: take all photos/videos of the post (carousel too)
  if (/instagram\.com\/(.+\/)?(p|reel|reels|tv)\//.test(url) && s.format === "video") {
    try {
      const [res] = await chrome.scripting.executeScript({ target: { tabId }, func: igItems });
      if (res.result && res.result.length) job = { action: "files", items: res.result, folder: s.folder };
    } catch (e) { /* fallback to yt-dlp */ }
  }
  if (!job) job = { action: "video", url, quality: s.quality, format: s.format, folder: s.folder };
  setJob("loading", "Скачиваю…");
  run(job);
  return { msg: job.action === "files"
    ? `Скачиваю файлов: ${job.items.length}. Окно можно закрыть.`
    : "Скачивание началось. Окно можно закрыть — придёт уведомление." };
}

let busy = false;

function run(job) {
  const port = chrome.runtime.connectNative(HOST);
  let done = false;
  busy = true;
  iconSpin(true);
  port.onMessage.addListener(r => {
    if (r.progress !== undefined) {
      const label = r.stage === "convert" ? "Перекодирую в MP4" : "Скачиваю";
      setJob("loading", `${label}… ${r.progress}%`, r.progress);
      return;
    }
    done = true;
    finish(r.ok ? "done" : "error", r.ok ? r.text : r.error);
    port.disconnect();
  });
  port.onDisconnect.addListener(() => {
    if (done) return;
    const e = (chrome.runtime.lastError && chrome.runtime.lastError.message) || "";
    finish("error", /not found|forbidden|not exist/i.test(e)
      ? "Помощник не установлен. Запустите установщик OmniSuck ещё раз."
      : e || "Помощник неожиданно закрылся.");
  });
  port.postMessage(job);
}

function finish(state, text) {
  busy = false;
  iconSpin(false);
  drawIcon(state);
  setJob(state, text, 100);
}

let lastPct = -1;
function setJob(state, text, pct = 0) {
  if (state === "loading" && Math.floor(pct) === lastPct) return;
  lastPct = state === "loading" ? Math.floor(pct) : -1;
  chrome.storage.local.set({ job: { state, text: String(text).slice(0, 250), pct, t: Date.now() } });
}

// ---- toolbar icon: spinner while downloading, check / cross when finished ----
const cv = new OffscreenCanvas(32, 32), ctx = cv.getContext("2d");
let spinTimer = null, angle = 0;

function drawIcon(kind) {
  ctx.clearRect(0, 0, 32, 32);
  ctx.lineCap = "round";
  if (kind === "spin") {
    ctx.lineWidth = 4;
    ctx.strokeStyle = "rgba(229,57,53,.25)";
    ctx.beginPath(); ctx.arc(16, 16, 12, 0, Math.PI * 2); ctx.stroke();
    ctx.strokeStyle = "#e53935";
    ctx.beginPath(); ctx.arc(16, 16, 12, angle, angle + Math.PI * 0.6); ctx.stroke();
  } else {
    const ok = kind === "done";
    ctx.fillStyle = ok ? "#22c55e" : "#ef4444";
    ctx.beginPath(); ctx.arc(16, 16, 15, 0, Math.PI * 2); ctx.fill();
    ctx.strokeStyle = "#fff"; ctx.lineWidth = 3.5; ctx.lineJoin = "round";
    ctx.beginPath();
    if (ok) { ctx.moveTo(9, 16.5); ctx.lineTo(14, 21.5); ctx.lineTo(23, 11.5); }
    else { ctx.moveTo(11, 11); ctx.lineTo(21, 21); ctx.moveTo(21, 11); ctx.lineTo(11, 21); }
    ctx.stroke();
  }
  chrome.action.setIcon({ imageData: ctx.getImageData(0, 0, 32, 32) });
}

function iconSpin(on) {
  clearInterval(spinTimer);
  if (on) spinTimer = setInterval(() => { angle += 0.35; drawIcon("spin"); }, 70);
}

function resetIcon() { if (!busy) chrome.action.setIcon({ path: "icon.png" }); }
chrome.tabs.onActivated.addListener(resetIcon);
chrome.tabs.onUpdated.addListener((_id, ch, tab) => { if (ch.url && tab.active) resetIcon(); });

// Runs inside the Instagram page (uses the user's own login)
async function igItems() {
  const m = location.pathname.match(/\/(?:p|reel|reels|tv)\/([A-Za-z0-9_-]+)/);
  if (!m) return [];
  const A = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
  let id = 0n;
  for (const c of m[1].slice(0, 11)) id = id * 64n + BigInt(A.indexOf(c));
  const r = await fetch(`/api/v1/media/${id}/info/`, { headers: { "x-ig-app-id": "936619743392459" }, credentials: "include" });
  const it = (await r.json()).items[0];
  return (it.carousel_media || [it]).map((x, i) => {
    const v = x.video_versions && x.video_versions[0] && x.video_versions[0].url;
    return { url: v || x.image_versions2.candidates[0].url, name: `instagram_${m[1]}_${i + 1}.${v ? "mp4" : "jpg"}` };
  });
}

function pick() {
  const port = chrome.runtime.connectNative(HOST);
  port.onMessage.addListener(r => {
    if (r.ok && r.folder) {
      chrome.storage.local.set({ folder: r.folder });
    }
    port.disconnect();
  });
  port.onDisconnect.addListener(() => chrome.runtime.lastError);
  port.postMessage({ action: "pick" });
}
