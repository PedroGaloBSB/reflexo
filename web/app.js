// Reflexo UI.
//
// Deliberately dependency-free and tiny: the whole point of ADR-0002 is that
// this runs in the browser the user already has, on a machine that may have no
// webview runtime, no package manager and no admin rights.
//
// There is no user-facing text in this file (ADR-0006). Every word comes from
// the state payload, translated server-side by internal/i18n, and is applied
// through the data-i18n attributes in index.html. If you find yourself adding
// a Portuguese or English literal here, it belongs in internal/i18n/catalogs.go
// instead — TestNoUserFacingStringsInWeb asserts this.

"use strict";

const $ = (id) => document.getElementById(id);

// Resolved from the state payload. Empty until the first poll succeeds, which
// is why every lookup goes through t() rather than reading it directly.
let text = {};

// t looks up a translated string, falling back to the key so a gap shows up as
// an obviously broken label rather than an empty button.
function t(key) {
  return text[key] || key;
}

// applyStatic fills every [data-i18n] element from the payload. Called once per
// render so a SetLang on the server is picked up without a page reload.
function applyStatic() {
  for (const el of document.querySelectorAll("[data-i18n]")) {
    el.textContent = t(el.dataset.i18n);
  }
}

let starting = false;

// Reads the session token the Go server put in the URL. Keeping it means a
// refresh or a reload of the page keeps working.
const token = new URLSearchParams(location.search).get("t") || "";

async function poll() {
  try {
    const res = await fetch("api/state?t=" + encodeURIComponent(token), {
      cache: "no-store",
    });
    if (!res.ok) throw new Error("HTTP " + res.status);
    render(await res.json());
  } catch {
    // The payload never arrived, so there is no translation to show. The best
    // available fallback is whatever the HTML shipped with.
    showFatal(t("ui.conn_lost"));
  }
}

function render(s) {
  if (s.text) text = s.text;

  if (s.fatal) {
    showFatal(s.fatal);
    return;
  }

  // Before anything dynamic: applyStatic fills the placeholders, and the
  // assignments below deliberately overwrite the elements that hold live data.
  applyStatic();

  $("meta").textContent = [s.platform, "scrcpy " + s.scrcpyVersion]
    .filter(Boolean)
    .join(" · ");

  renderSetup(s);
  renderMain(s);
}

function renderSetup(s) {
  if (s.setup === "ready") {
    $("setup").hidden = true;
    return;
  }

  $("setup").hidden = false;
  $("main").hidden = true;

  const failed = s.setup === "error";
  $("setup-title").textContent = failed
    ? t("setup.failed_title")
    : t("setup.preparing");
  $("setup-msg").textContent = s.setupMsg || "…";
  $("setup-msg").className = failed ? "detail error" : "detail";

  // The bar only makes sense while bytes are actually moving; a stalled
  // connection should not look like a slow but healthy download.
  const showBar = !failed && s.setupPct > 0 && s.setupPct < 1;
  $("setup-bar").hidden = !showBar;
  if (showBar) $("setup-fill").style.width = Math.round(s.setupPct * 100) + "%";
}

function renderMain(s) {
  $("setup").hidden = true;
  $("main").hidden = false;

  // The label comes from the catalog via the key the server sent. The numeric
  // severity is used only to pick a colour; the browser does not know the
  // meaning of the numbers and must not guess.
  $("badge").textContent = s.severityKey ? t(s.severityKey) : "";
  $("badge").dataset.sev = String(s.severity);
  $("headline").textContent = s.headline || "…";
  $("detail").textContent = s.detail || "";

  const ol = $("steps");
  ol.textContent = "";
  for (const step of s.steps || []) {
    const li = document.createElement("li");
    li.textContent = step;
    ol.appendChild(li);
  }

  const btn = $("start");
  const label = t(s.running ? "ui.running" : "ui.start");
  if (btn.textContent !== label) btn.textContent = label;

  btn.disabled = starting || s.running || !s.canStart;
  $("hint").textContent = s.running ? t("ui.close_to_stop") : "";

  // The demonstration has to announce itself. Its whole purpose is to show the
  // product working, and it is worthless — worse than nothing — if the reader
  // mistakes it for a report about their own phone.
  $("demo-banner").hidden = !s.demo;

  // While the daemon is still coming up there is no verdict, so show a moving
  // bar instead of a frozen, empty screen.
  $("waiting-bar").hidden = !s.waiting;

  // The button doubles as the way out, so its label flips with the state.
  const demoBtn = $("demo");
  demoBtn.hidden = !s.demo && !s.canDemo;
  const demoLabel = t(s.demo ? "ui.demo_stop" : "ui.demo_try");
  if (demoBtn.textContent !== demoLabel) demoBtn.textContent = demoLabel;
}

function showFatal(msg) {
  $("setup").hidden = false;
  $("main").hidden = true;
  $("setup-title").textContent = t("ui.unavailable");
  $("setup-msg").textContent = msg;
  $("setup-msg").className = "detail error";
  $("setup-bar").hidden = true;
}

$("start").addEventListener("click", async () => {
  if (starting) return;
  starting = true;
  $("start").disabled = true;

  try {
    const res = await fetch("api/start?t=" + encodeURIComponent(token), {
      method: "POST",
    });
    if (!res.ok) {
      // The server's own text is in the user's language; only fall back to a
      // generic phrase when it sent nothing.
      $("hint").textContent = (await res.text()) || t("ui.start_failed");
    }
  } catch {
    $("hint").textContent = t("ui.start_conn_failed");
  } finally {
    // Release the button only after the next poll reports the real state.
    setTimeout(() => {
      starting = false;
      poll();
    }, 800);
  }
});

$("demo").addEventListener("click", async () => {
  $("demo").disabled = true;
  try {
    await fetch("api/demo?t=" + encodeURIComponent(token), { method: "POST" });
  } catch {
    // The next poll restores whatever the server actually believes, so there is
    // nothing to repair here beyond not leaving a button stuck.
  } finally {
    $("demo").disabled = false;
    poll();
  }
});

// Poll while the tab is visible. A hidden tab does not need 2s updates, and this
// keeps a laptop from waking up in a bag.
let timer = null;
function schedule(ms) {
  clearTimeout(timer);
  if (!document.hidden) timer = setTimeout(tick, ms);
}
function tick() {
  poll().finally(() => schedule(2000));
}

document.addEventListener("visibilitychange", () => {
  if (document.hidden) {
    clearTimeout(timer);
  } else {
    poll().finally(() => schedule(2000));
  }
});

poll().finally(() => schedule(2000));