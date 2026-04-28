(function () {
  function getEl(id) {
    const el = document.getElementById(id);
    if (!el) throw new Error(`Element not found: #${id}`);
    return el;
  }

  function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  function rand(min, max) {
    return min + Math.random() * (max - min);
  }

  function clamp(n, min, max) {
    return Math.max(min, Math.min(max, n));
  }

  function lerp(start, end, t) {
    return start + (end - start) * t;
  }

  function toNumber(v, fallback) {
    if (typeof v === "string" && v.trim() === "") return fallback;
    const n = Number(v);
    return Number.isFinite(n) ? n : fallback;
  }

  function roundPx(n) {
    return Math.round(n);
  }

  const CURSOR_HOTSPOT_X = 12;
  const CURSOR_HOTSPOT_Y = 8;
  let lastCursorPos = { x: 12, y: 12 };

  function isInputLike(el) {
    if (!el) return false;
    const tag = String(el.tagName || "").toLowerCase();
    if (tag === "input" || tag === "textarea" || tag === "select") return true;
    if (el.isContentEditable) return true;
    return false;
  }

  function getTargetPoint(el) {
    const rect = el.getBoundingClientRect();
    if (isInputLike(el)) {
      const x = rect.left + Math.min(22, rect.width * 0.18);
      const y = rect.top + rect.height * 0.58;
      return { x, y };
    }
    return { x: rect.left + rect.width * 0.5, y: rect.top + rect.height * 0.5 };
  }

  function isInViewport(frame, el, marginPx) {
    const win = frame.contentWindow;
    if (!win) return true;
    const rect = el.getBoundingClientRect();
    const m = Math.max(0, Number(marginPx ?? 12));
    return (
      rect.top >= m &&
      rect.left >= m &&
      rect.bottom <= win.innerHeight - m &&
      rect.right <= win.innerWidth - m
    );
  }

  async function waitForScrollStable(win, timeoutMs, ctx, onTick) {
    if (!win) return;
    const speed = ctx && ctx.speed ? ctx.speed : 1;
    const maxMs = clamp(Number(timeoutMs ?? 700) / speed, 120, 1400);
    const startedAt = performance.now();
    const docEl = win.document && win.document.documentElement;
    const getPos = () => ({
      x: Number(win.scrollX ?? win.pageXOffset ?? 0),
      y: Number(win.scrollY ?? win.pageYOffset ?? (docEl ? docEl.scrollTop : 0) ?? 0)
    });
    let last = getPos();
    let stableFrames = 0;
    if (typeof onTick === "function") {
      try { onTick(); } catch (_) {}
    }
    while (performance.now() - startedAt < maxMs) {
      await new Promise((resolve) => win.requestAnimationFrame(() => resolve()));
      if (typeof onTick === "function") {
        try { onTick(); } catch (_) {}
      }
      const cur = getPos();
      const dx = Math.abs(cur.x - last.x);
      const dy = Math.abs(cur.y - last.y);
      last = cur;
      if (dx < 0.5 && dy < 0.5) stableFrames += 1;
      else stableFrames = 0;
      if (stableFrames >= 6) return;
    }
  }

  async function ensureVisible(frame, el, ctx, onScrollTick) {
    const win = frame.contentWindow;
    if (!win) return;
    if (typeof onScrollTick === "function") {
      try { onScrollTick(); } catch (_) {}
    }
    await waitForScrollStable(win, 650, ctx, onScrollTick);
    if (isInViewport(frame, el, 18)) return;
    const doc = frame.contentDocument;
    const docEl = doc && doc.documentElement;
    const body = doc && doc.body;
    const prevHtml = docEl ? docEl.style.scrollBehavior : "";
    const prevBody = body ? body.style.scrollBehavior : "";
    try {
      if (docEl) docEl.style.scrollBehavior = "auto";
      if (body) body.style.scrollBehavior = "auto";
      if (typeof onScrollTick === "function") {
        try { onScrollTick(); } catch (_) {}
      }
      el.scrollIntoView({ block: "center", inline: "nearest" });
    } catch (_) {}
    await waitForScrollStable(win, 900, ctx, onScrollTick);
    try {
      if (docEl) docEl.style.scrollBehavior = prevHtml;
      if (body) body.style.scrollBehavior = prevBody;
    } catch (_) {}
  }

  function ensureCursor(frame) {
    const doc = frame.contentDocument;
    if (!doc) return null;
    let el = doc.getElementById("__tutorial_cursor__");
    if (el) return el;
    el = doc.createElement("div");
    el.id = "__tutorial_cursor__";
    el.style.position = "fixed";
    el.style.left = "-200px";
    el.style.top = "-200px";
    el.style.width = "56px";
    el.style.height = "56px";
    el.style.zIndex = "2147483647";
    el.style.pointerEvents = "none";
    el.style.transition = "none"; // Ensure no CSS transition interferes with JS animation
    el.style.opacity = "0";
    el.innerHTML =
      '<svg width="39" height="39" viewBox="0 0 28 28" xmlns="http://www.w3.org/2000/svg">' +
      '<path d="M6 4 L6 22 L10.2 18.1 L13.4 24.4 L16 23.2 L12.9 16.9 L18.1 16.7 Z" fill="rgba(0,0,0,0.92)" stroke="rgba(255,255,255,0.35)" stroke-width="1.2" />' +
      "</svg>";
    el.style.filter = "drop-shadow(0 8px 12px rgba(0,0,0,0.35))";
    el.dataset.x = String(lastCursorPos.x);
    el.dataset.y = String(lastCursorPos.y);
    el.style.left = `${lastCursorPos.x - CURSOR_HOTSPOT_X}px`;
    el.style.top = `${lastCursorPos.y - CURSOR_HOTSPOT_Y}px`;
    doc.documentElement.appendChild(el);
    try {
      const style = doc.createElement("style");
      style.id = "__tutorial_cursor_hide_style__";
      style.textContent = `
        html, body { cursor: none !important; }
        * { cursor: none !important; }
      `;
      doc.head ? doc.head.appendChild(style) : doc.documentElement.appendChild(style);
    } catch (_) {}
    return el;
  }

  function ensureParentCursorHidden(frame) {
    try {
      frame.style.cursor = "none";
    } catch (_) {}
    try {
      const id = "__tutorial_parent_cursor_hide_style__";
      if (!document.getElementById(id)) {
        const style = document.createElement("style");
        style.id = id;
        style.textContent = `
          html, body { cursor: none !important; }
          iframe { cursor: none !important; }
        `;
        (document.head || document.documentElement).appendChild(style);
      }
      document.documentElement.style.cursor = "none";
      if (document.body) document.body.style.cursor = "none";
    } catch (_) {}
  }

  async function moveCursorTo(frame, x, y, durationMs, onTick) {
    const cursor = ensureCursor(frame);
    const win = frame.contentWindow;
    if (!cursor || !win) return;
    if (cursor.style.opacity !== "1") cursor.style.opacity = "1";
    
    // Switch to transform if not already (reset left/top)
    if (cursor.style.left !== "0px" || cursor.style.top !== "0px") {
      cursor.style.left = "0px";
      cursor.style.top = "0px";
    }

    const startX = Number(cursor.dataset.x ?? lastCursorPos.x);
    const startY = Number(cursor.dataset.y ?? lastCursorPos.y);
    const endX = clamp(x, 2, win.innerWidth - 2);
    const endY = clamp(y, 2, win.innerHeight - 2);
    const ms = clamp(Number(durationMs ?? 140), 40, 1400);
    const startedAt = performance.now();
    await new Promise((resolve) => {
      const tick = (now) => {
        const t = clamp((now - startedAt) / ms, 0, 1);
        const ease = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
        const cx = startX + (endX - startX) * ease;
        const cy = startY + (endY - startY) * ease;
        
        // Use translate3d for hardware acceleration and sub-pixel rendering
        const tx = cx - CURSOR_HOTSPOT_X;
        const ty = cy - CURSOR_HOTSPOT_Y;
        cursor.style.transform = `translate3d(${tx}px, ${ty}px, 0)`;
        
        // Store integer position for logical consistency, but use float for display
        cursor.dataset.x = String(cx);
        cursor.dataset.y = String(cy);
        lastCursorPos = { x: cx, y: cy };
        
        if (typeof onTick === "function") {
          try { onTick({ x: cx, y: cy, t, ease }); } catch (_) {}
        }
        if (t >= 1) return resolve();
        win.requestAnimationFrame(tick);
      };
      win.requestAnimationFrame(tick);
    });
  }

  async function moveCursorToElement(frame, el, opts) {
    const rect = el.getBoundingClientRect();
    const x = rect.left + rect.width * 0.5 + rand(-2, 2);
    const y = rect.top + rect.height * 0.5 + rand(-2, 2);
    const durationMs = opts && opts.moveMs != null ? opts.moveMs : 320;
    await moveCursorTo(frame, x, y, durationMs);
  }

  function dispatchMouse(frame, el, type, clientX, clientY) {
    const win = frame.contentWindow;
    if (!win) return;
    const evt = new win.MouseEvent(type, {
      bubbles: true,
      cancelable: true,
      composed: true,
      view: win,
      clientX,
      clientY,
      screenX: clientX,
      screenY: clientY,
      button: 0
    });
    el.dispatchEvent(evt);
  }

  function reportTutorialClick(ctx, frame, x, y) {
    try {
      if (!ctx || !ctx.cbPort || !ctx.token) return;
      const win = frame.contentWindow;
      if (!win) return;
      const docEl = win.document && win.document.documentElement;
      const vw = (docEl && docEl.clientWidth) ? docEl.clientWidth : win.innerWidth;
      const vh = (docEl && docEl.clientHeight) ? docEl.clientHeight : win.innerHeight;
      
      let nx = x / Math.max(1, vw);
      let ny = y / Math.max(1, vh);

      // Heuristic: If we are recording the whole window (vCapture usually does),
      // we need to normalize against outer dimensions to account for chrome (title bar).
      // If outer dimensions are significantly larger, apply correction.
      if (win.outerWidth && win.outerHeight && win.outerWidth > vw && win.outerHeight > vh) {
          const ow = win.outerWidth;
          const oh = win.outerHeight;
          const diffH = oh - vh;
          // Only apply if the difference looks like chrome (e.g. > 20px)
          if (diffH > 20) {
              // Assume most of the height difference is the top title bar.
              // We map y (client) to y_outer.
              // y_outer ~= y + diffH (approx).
              // We ignore horizontal offset (usually small borders).
              nx = x / ow; 
              ny = (y + diffH) / oh;
          }
      }

      nx = clamp(nx, 0, 1);
      ny = clamp(ny, 0, 1);

      const url =
        `http://127.0.0.1:${encodeURIComponent(String(ctx.cbPort))}` +
        `/tutorial/click?nx=${encodeURIComponent(String(nx))}` +
        `&ny=${encodeURIComponent(String(ny))}` +
        `&t=${encodeURIComponent(String(Date.now()))}` +
        `&token=${encodeURIComponent(String(ctx.token))}`;
      fetch(url, { method: "GET", mode: "cors", cache: "no-store" }).catch(() => {});
    } catch (_) {}
  }

  async function humanClick(frame, el, opts, ctx) {
    await ensureVisible(frame, el, ctx);
    try {
      if (typeof el.focus === "function") el.focus({ preventScroll: true });
    } catch (_) {}
    const pt = getTargetPoint(el);
    const x = pt.x;
    const y = pt.y;
    await moveCursorTo(frame, x, y, opts && opts.moveMs != null ? opts.moveMs : 120);
    dispatchMouse(frame, el, "mousemove", x, y);
    dispatchMouse(frame, el, "mousedown", x, y);
    await sleep(opts && opts.downUpDelayMs != null ? opts.downUpDelayMs : rand(12, 30));
    dispatchMouse(frame, el, "mouseup", x, y);
    dispatchMouse(frame, el, "click", x, y);
    reportTutorialClick(ctx, frame, x, y);
    if (el && el.tagName === "A") {
      const hrefAttr = el.getAttribute("href");
      const targetAttr = el.getAttribute("target");
      const hrefLower = String(hrefAttr || "").trim().toLowerCase();
      const isJsHref = hrefLower.startsWith("javascript:");
      const isPhoneHref = hrefLower.startsWith("tel:");
      const isMailHref = hrefLower.startsWith("mailto:");
      if (hrefAttr && hrefAttr !== "#" && targetAttr !== "_blank" && !isJsHref && !isPhoneHref && !isMailHref) {
        try {
          frame.location.assign(el.href);
        } catch (_) {}
      }
    }
  }

  async function humanType(frame, el, value, opts, ctx) {
    const input = el;
    await humanClick(frame, input, opts, ctx);
    const clear = Boolean(opts && opts.clear);
    if (clear) {
      input.value = "";
      input.dispatchEvent(new Event("input", { bubbles: true }));
      await sleep(rand(10, 30));
    }
    const speed = ctx && ctx.speed ? ctx.speed : 1;
    const baseDelay = (opts && opts.charDelayMs != null) ? Number(opts.charDelayMs) : 14;
    const jitter = (opts && opts.charJitterMs != null) ? Number(opts.charJitterMs) : 8;
    for (const ch of String(value ?? "")) {
      input.value = String(input.value ?? "") + ch;
      input.dispatchEvent(new Event("input", { bubbles: true }));
      const ms = clamp(rand(baseDelay - jitter, baseDelay + jitter) / speed, 4, 45);
      await sleep(ms);
    }
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  function parseVars(input) {
    const out = {};
    if (!input || typeof input !== "object") return out;
    for (const [k, v] of Object.entries(input)) out[k] = String(v ?? "");
    return out;
  }

  function interpolate(value, vars) {
    if (typeof value !== "string") return value;
    return value.replace(/\$\{([a-zA-Z0-9_]+)\}/g, (_, key) => (vars[key] ?? ""));
  }

  function normalizeLang(raw) {
    const v = String(raw ?? "").trim();
    if (!v) return "zh";
    const lower = v.toLowerCase();
    if (lower === "zh-cn" || lower.startsWith("zh-cn")) return "zh-CN";
    if (lower === "zh-tw" || lower.startsWith("zh-tw")) return "zh";
    if (lower.startsWith("zh")) return "zh";
    if (lower.startsWith("en")) return "en";
    return "zh";
  }

  function t(key, lang) {
    const L = normalizeLang(lang);
    const dict = {
      readyTitle: { zh: "準備就緒", "zh-CN": "准备就绪", en: "Ready" },
      readyDesc: {
        zh: "請先至 vCapture 選擇此視窗並開始錄影<br>然後點擊下方按鈕開始教學",
        "zh-CN": "请先至 vCapture 选择此窗口并开始录影<br>然后点击下方按钮开始教学",
        en: "Select this window in vCapture and start recording.<br>Then click the button below to start."
      },
      startTutorial: { zh: "開始 Tutorial", "zh-CN": "开始 Tutorial", en: "Start Tutorial" },
      startingSoon: { zh: "即將開始…", "zh-CN": "即将开始…", en: "Starting soon…" },
      preloadScenario: { zh: "預載入 Scenario…", "zh-CN": "预载入 Scenario…", en: "Preloading scenario…" },
      preloadPagePrefix: { zh: "預載入頁面：", "zh-CN": "预载入页面：", en: "Preloading page: " },
      waitingRecording: { zh: "等待錄影開始…", "zh-CN": "等待录影开始…", en: "Waiting for recording…" },
      notDetected: {
        zh: "尚未偵測到錄影開始，請先在 vCapture 開始錄影",
        "zh-CN": "尚未侦测到录影开始，请先在 vCapture 开始录影",
        en: "Recording not detected. Start recording in vCapture first."
      },
      doneSending: { zh: "完成，送出回呼…", "zh-CN": "完成，送出回呼…", en: "Done. Sending callback…" },
      done: { zh: "完成", "zh-CN": "完成", en: "Done" },
    };
    const row = dict[key];
    if (!row) return String(key);
    return row[L] ?? row.zh ?? String(key);
  }

  function resolveStepText(value, vars) {
    if (value == null) return "";
    if (typeof value === "string") return interpolate(value, vars);
    if (typeof value === "object") {
      const lang = normalizeLang(vars && vars.lang ? vars.lang : "");
      const table = value;
      const picked = (table && (table[lang] ?? table.zh ?? table["zh-CN"] ?? table.en)) ?? "";
      return interpolate(String(picked ?? ""), vars);
    }
    return interpolate(String(value), vars);
  }

  function withTimeout(promise, timeoutMs, label) {
    if (!timeoutMs || timeoutMs <= 0) return promise;
    let timer;
    const timeoutPromise = new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(`Timeout: ${label}`)), timeoutMs);
    });
    return Promise.race([promise, timeoutPromise]).finally(() => clearTimeout(timer));
  }

  async function waitForFrameLoad(frame, timeoutMs) {
    await withTimeout(
      new Promise((resolve) => {
        const onLoad = () => {
          frame.removeEventListener("load", onLoad);
          resolve();
        };
        frame.addEventListener("load", onLoad);
      }),
      timeoutMs ?? 15000,
      "frame load"
    );
  }

  async function waitForSelector(frame, selector, timeoutMs) {
    const start = Date.now();
    while (true) {
      const doc = frame.contentDocument;
      if (doc) {
        const el = doc.querySelector(selector);
        if (el) return el;
      }
      if (Date.now() - start > (timeoutMs ?? 15000)) {
        throw new Error(`waitForSelector timeout: ${selector}`);
      }
      await sleep(30);
    }
  }

  async function waitForPathNot(frame, path, timeoutMs) {
    const start = Date.now();
    while (true) {
      const win = frame.contentWindow;
      if (win && win.location && win.location.pathname !== path) return;
      if (Date.now() - start > (timeoutMs ?? 15000)) {
        throw new Error(`waitForPathNot timeout: ${path}`);
      }
      await sleep(50);
    }
  }

  function ensureHighlighter(frame) {
    const doc = frame.contentDocument;
    if (!doc) return null;
    let root = doc.getElementById("__tutorial_highlight_root__");
    if (root) return root;
    root = doc.createElement("div");
    root.id = "__tutorial_highlight_root__";
    root.style.position = "fixed";
    root.style.left = "0";
    root.style.top = "0";
    root.style.width = "0";
    root.style.height = "0";
    root.style.zIndex = "2147483646"; // Lower than cursor (2147483647)
    root.style.pointerEvents = "none";

    const box = doc.createElement("div");
    box.id = "__tutorial_highlight_box__";
    box.style.position = "fixed";
    box.style.border = "3px solid rgba(59, 130, 246, 0.95)";
    box.style.borderRadius = "10px";
    box.style.boxShadow = "0 0 0 9999px rgba(0,0,0,0.18)";
    box.style.transition = "all 120ms ease";
    box.style.pointerEvents = "none";

    const label = doc.createElement("div");
    label.id = "__tutorial_highlight_label__";
    label.style.position = "fixed";
    label.style.padding = "8px 10px";
    label.style.borderRadius = "10px";
    label.style.background = "rgba(0,0,0,0.78)";
    label.style.color = "#fff";
    label.style.fontSize = "13px";
    label.style.maxWidth = "min(520px, calc(100vw - 24px))";
    label.style.pointerEvents = "none";

    root.appendChild(box);
    root.appendChild(label);
    doc.documentElement.appendChild(root);
    return root;
  }

  function createHighlightUpdater(frame) {
    const doc = frame.contentDocument;
    if (!doc) return null;
    ensureHighlighter(frame);
    const box = doc.getElementById("__tutorial_highlight_box__");
    const label = doc.getElementById("__tutorial_highlight_label__");
    if (!box || !label) return null;

    const win = frame.contentWindow;
    let lastText = null;

    // Reset layout properties to allow transform to work relative to 0,0
    box.style.left = "0px";
    box.style.top = "0px";
    // Ensure initial transform is set to avoid jump
    // We cannot know the initial pos here easily without rect, but the first call to update will fix it.

    return (rect, text, opts) => {
      if (opts && opts.disableTransition) {
        box.style.transition = "none";
      } else {
        box.style.transition = "all 120ms ease";
      }

      const pad = 6;
      // Use translate3d for position (GPU accelerated)
      // Note: We still need to update width/height which triggers layout, 
      // but separating position helps.
      const tx = Math.max(0, rect.left - pad);
      const ty = Math.max(0, rect.top - pad);
      box.style.transform = `translate3d(${tx}px, ${ty}px, 0)`;
      box.style.width = `${Math.max(0, rect.width + pad * 2)}px`;
      box.style.height = `${Math.max(0, rect.height + pad * 2)}px`;

      if (text !== lastText) {
        label.textContent = text || "";
        lastText = text;
      }
      
      const iw = win ? win.innerWidth : window.innerWidth;
      const ih = win ? win.innerHeight : window.innerHeight;
      const margin = 12;
      const gap = 10;
      const lr = label.getBoundingClientRect();
      const lw = lr.width;
      const lh = lr.height;
      const lx = Math.min(iw - margin - lw, Math.max(margin, rect.left));
      const below = rect.bottom + gap;
      const above = rect.top - gap - lh;
      const preferredY = below + lh <= ih - margin ? below : above;
      const ly = Math.min(ih - margin - lh, Math.max(margin, preferredY));
      label.style.left = "0px";
      label.style.top = "0px";
      label.style.transform = `translate3d(${lx}px, ${ly}px, 0)`;
    };
  }

  function drawHighlight(frame, rect, text, opts) {
    const doc = frame.contentDocument;
    if (!doc) return;
    ensureHighlighter(frame);
    const box = doc.getElementById("__tutorial_highlight_box__");
    const label = doc.getElementById("__tutorial_highlight_label__");
    if (!box || !label) return;

    if (opts && opts.disableTransition) {
      box.style.transition = "none";
    } else {
      box.style.transition = "all 120ms ease";
    }

    const pad = 6;
    // For single draw calls, we can also use transform
    box.style.left = "0px";
    box.style.top = "0px";
    const tx = Math.max(0, rect.left - pad);
    const ty = Math.max(0, rect.top - pad);
    box.style.transform = `translate3d(${tx}px, ${ty}px, 0)`;
    box.style.width = `${Math.max(0, rect.width + pad * 2)}px`;
    box.style.height = `${Math.max(0, rect.height + pad * 2)}px`;

    label.textContent = text || "";
    const win = frame.contentWindow;
    const iw = win ? win.innerWidth : window.innerWidth;
    const ih = win ? win.innerHeight : window.innerHeight;
    const margin = 12;
    const gap = 10;
    const lr = label.getBoundingClientRect();
    const lw = lr.width;
    const lh = lr.height;
    const lx = Math.min(iw - margin - lw, Math.max(margin, rect.left));
    const below = rect.bottom + gap;
    const above = rect.top - gap - lh;
    const preferredY = below + lh <= ih - margin ? below : above;
    const ly = Math.min(ih - margin - lh, Math.max(margin, preferredY));
    label.style.left = "0px";
    label.style.top = "0px";
    label.style.transform = `translate3d(${lx}px, ${ly}px, 0)`;
  }

  function setHighlight(frame, targetEl, text, opts) {
    const rect = targetEl.getBoundingClientRect();
    drawHighlight(frame, rect, text, opts);
  }

  function clearHighlight(frame) {
    const doc = frame.contentDocument;
    if (!doc) return;
    const root = doc.getElementById("__tutorial_highlight_root__");
    if (root) root.remove();
  }

  async function doType(el, value, clear) {
    const input = el;
    if (clear) input.value = "";
    input.focus();
    input.value = value;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  async function notifyDone(cbPort, scenario, token, error) {
    if (!cbPort) return;
    let url = `http://127.0.0.1:${encodeURIComponent(cbPort)}/tutorial/done?scenario=${encodeURIComponent(scenario)}&token=${encodeURIComponent(token || "")}`;
    if (error) {
      url += `&error=${encodeURIComponent(error)}`;
    }
    
    // Retry a few times to ensure vCapture receives the signal
    for (let i = 0; i < 3; i++) {
      try {
        await fetch(url, { method: "GET", mode: "cors", cache: "no-store" });
        return;
      } catch (e) {
        console.warn(`Notify done failed (attempt ${i + 1}):`, e);
        await new Promise((r) => setTimeout(r, 500));
      }
    }
  }

  async function runScenario({ scenario, vars, frame, statusEl, cbPort, token, ctx, scenarioData }) {
    if (!scenarioData) {
      statusEl.textContent = `載入 Scenario：${scenario}…`;
      const scenarioUrl = `/static/tutorials/${encodeURIComponent(scenario)}.json`;
      const res = await fetch(scenarioUrl, { cache: "no-store" });
      if (!res.ok) throw new Error(`Failed to load scenario: ${scenarioUrl}`);
      scenarioData = await res.json();
    }
    const stepList = Array.isArray(scenarioData.steps) ? scenarioData.steps : [];

    const scenarioVars = {
      ...parseVars(scenarioData.vars),
      ...parseVars(vars)
    };

    for (let i = 0; i < stepList.length; i++) {
      const step = stepList[i] || {};
      const action = String(step.action || "");
      const startedAt = performance.now();
      statusEl.textContent = `[${i + 1}/${stepList.length}] ${action}`;

      if (action === "goto") {
        const url = interpolate(step.url, scenarioVars);
        const win = frame.contentWindow;
        const current = win && win.location ? `${win.location.pathname}${win.location.search || ""}` : "";
        const target = String(url || "");
        if (current !== target) {
          statusEl.textContent = `[${i + 1}/${stepList.length}] goto ${target} (loading…)`;
          const loadPromise = waitForFrameLoad(frame, step.timeoutMs ?? 20000);
          frame.src = target;
          await loadPromise;
        }
        clearHighlight(frame);
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        if (ctx && ctx.stepDelayMs != null && ctx.stepDelayMs > 0) await sleep(ctx.stepDelayMs);
        continue;
      }

      if (action === "waitFor") {
        const selector = interpolate(step.selector, scenarioVars);
        statusEl.textContent = `[${i + 1}/${stepList.length}] waitFor ${selector}`;
        await waitForSelector(frame, selector, step.timeoutMs ?? 15000);
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        continue;
      }

      if (action === "waitForPathNot") {
        const path = interpolate(step.path, scenarioVars);
        statusEl.textContent = `[${i + 1}/${stepList.length}] waitForPathNot ${path}`;
        await waitForPathNot(frame, path, step.timeoutMs ?? 15000);
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        continue;
      }

      if (action === "highlight") {
        const selector = interpolate(step.selector, scenarioVars);
        const el = await waitForSelector(frame, selector, step.timeoutMs ?? 15000);
        const text = resolveStepText(step.text || "", scenarioVars);
        
        await ensureVisible(frame, el, ctx, () => {
          if (ctx && ctx.guide !== false) setHighlight(frame, el, text, { disableTransition: true });
        });

        const startRect = (function() {
          try {
            const doc = frame.contentDocument;
            const box = doc && doc.getElementById("__tutorial_highlight_box__");
            if (!box || box.style.display === "none") return null;
            const r = box.getBoundingClientRect();
            return {
              left: r.left + 6,
              top: r.top + 6,
              width: r.width - 12,
              height: r.height - 12
            };
          } catch (_) { return null; }
        })();

        const updateHighlight = createHighlightUpdater(frame);

        const pt = getTargetPoint(el);
        const x = pt.x;
        const y = pt.y;
        
        // Pre-calculate target rect to avoid thrashing in loop
        const targetRect = el.getBoundingClientRect();
        
        // Capture start cursor for reference (not used for interpolation anymore)
        // const startCursor = { ...lastCursorPos };

        await moveCursorTo(frame, x, y, step.moveMs ?? 90, (tick) => {
          if (ctx && ctx.guide !== false) {
            if (startRect && tick && typeof tick.ease === "number") {
              const progress = tick.ease;
              
              // Direct interpolation of rects ensures the box moves exactly in sync with the cursor's easing.
              // We avoid anchor interpolation which can cause visual speed discrepancies.
              const curRect = {
                left: lerp(startRect.left, targetRect.left, progress),
                top: lerp(startRect.top, targetRect.top, progress),
                width: lerp(startRect.width, targetRect.width, progress),
                height: lerp(startRect.height, targetRect.height, progress),
              };
              
              // Recalculate precise bottom for label positioning
              curRect.bottom = curRect.top + curRect.height;
              
              if (updateHighlight) {
                updateHighlight(curRect, text, { disableTransition: true });
              } else {
                drawHighlight(frame, curRect, text, { disableTransition: true });
              }
            } else {
              setHighlight(frame, el, text, { disableTransition: true });
            }
          }
        });
        dispatchMouse(frame, el, "mousemove", x, y);
        if (ctx && ctx.guide !== false) {
          setHighlight(frame, el, text);
        } else {
          clearHighlight(frame);
        }
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        if (ctx && ctx.stepDelayMs != null && ctx.stepDelayMs > 0) await sleep(ctx.stepDelayMs);
        continue;
      }

      if (action === "click") {
        const selector = interpolate(step.selector, scenarioVars);
        const el = await waitForSelector(frame, selector, step.timeoutMs ?? 15000);
        if (step.humanize === false) {
          el.click();
        } else {
          await humanClick(frame, el, step, ctx);
        }
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        if (ctx && ctx.stepDelayMs != null && ctx.stepDelayMs > 0) await sleep(ctx.stepDelayMs);
        continue;
      }

      if (action === "type") {
        const selector = interpolate(step.selector, scenarioVars);
        const el = await waitForSelector(frame, selector, step.timeoutMs ?? 15000);
        const value = interpolate(step.value ?? "", scenarioVars);
        if (step.humanize === false) {
          await doType(el, value, Boolean(step.clear));
        } else {
          await humanType(frame, el, value, { ...step, clear: Boolean(step.clear) }, ctx);
        }
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        if (ctx && ctx.stepDelayMs != null && ctx.stepDelayMs > 0) await sleep(ctx.stepDelayMs);
        continue;
      }

      if (action === "sleep") {
        const speed = ctx && ctx.speed ? ctx.speed : 1;
        const raw = Number(step.ms ?? 0);
        const scaled = raw / speed;
        const ms = step.force === true ? scaled : Math.min(scaled, 350);
        await sleep(ms);
        statusEl.textContent = `[${i + 1}/${stepList.length}] ${action} (${Math.round(performance.now() - startedAt)}ms)`;
        continue;
      }

      if (action === "done") {
        statusEl.textContent = t("doneSending", (vars && vars.lang) || "");
        // Ensure a small delay before stopping to catch the final state
        await sleep(1000);
        
        if (cbPort) {
          await notifyDone(cbPort, scenario, token);
        }
        
        clearHighlight(frame);
        statusEl.textContent = t("done", (vars && vars.lang) || "");
        
        // Auto close window if running in batch mode (indicated by cbPort/token presence)
        if (cbPort && token) {
            await sleep(500);
            window.close();
        }
        return;
      }

      throw new Error(`Unknown action: ${action}`);
    }

    clearHighlight(frame);
    statusEl.textContent = t("doneSending", (vars && vars.lang) || "");
    await sleep(1000);
    if (cbPort) {
      await notifyDone(cbPort, scenario, token);
    }
    statusEl.textContent = t("done", (vars && vars.lang) || "");
    
    // Auto close window if running in batch mode
    if (cbPort && token) {
        await sleep(500);
        window.close();
    }
  }

  function readQueryVars() {
    const params = new URLSearchParams(window.location.search);
    const vars = {};
    for (const [k, v] of params.entries()) {
      if (k === "scenario" || k === "cbPort" || k === "token") continue;
      vars[k] = v;
    }
    return vars;
  }

  function createStartOverlay(onStart) {
    const vars = readQueryVars();
    const lang = normalizeLang(vars.lang);
    const overlay = document.createElement("div");
    overlay.id = "__tutorial_start_overlay__";
    overlay.style.position = "fixed";
    overlay.style.inset = "0";
    overlay.style.zIndex = "20000";
    overlay.style.backgroundColor = "rgba(0,0,0,0.85)";
    overlay.style.display = "flex";
    overlay.style.flexDirection = "column";
    overlay.style.alignItems = "center";
    overlay.style.justifyContent = "center";
    overlay.style.color = "#fff";
    overlay.style.fontFamily = "system-ui, sans-serif";

    const title = document.createElement("h2");
    title.textContent = t("readyTitle", lang);
    title.style.marginBottom = "16px";
    
    const desc = document.createElement("p");
    desc.innerHTML = t("readyDesc", lang);
    desc.style.marginBottom = "24px";
    desc.style.textAlign = "center";
    desc.style.lineHeight = "1.5";
    desc.style.opacity = "0.9";
    
    const btn = document.createElement("button");
    btn.textContent = t("startTutorial", lang);
    btn.style.padding = "12px 24px";
    btn.style.fontSize = "18px";
    btn.style.fontWeight = "bold";
    btn.style.color = "#fff";
    btn.style.backgroundColor = "#0079d3";
    btn.style.border = "none";
    btn.style.borderRadius = "8px";
    btn.style.cursor = "pointer";
    btn.onclick = () => {
      overlay.remove();
      onStart();
    };

    overlay.appendChild(title);
    overlay.appendChild(desc);
    overlay.appendChild(btn);
    document.body.appendChild(overlay);

    return { overlay, title, desc, btn };
  }

  function createCountdownOverlay(delayMs, onDone) {
    const vars = readQueryVars();
    const lang = normalizeLang(vars.lang);
    const overlay = document.createElement("div");
    overlay.id = "__tutorial_countdown_overlay__";
    overlay.style.position = "fixed";
    overlay.style.inset = "0";
    overlay.style.zIndex = "20000";
    overlay.style.backgroundColor = "rgba(0,0,0,0.85)";
    overlay.style.display = "flex";
    overlay.style.flexDirection = "column";
    overlay.style.alignItems = "center";
    overlay.style.justifyContent = "center";
    overlay.style.color = "#fff";
    overlay.style.fontFamily = "system-ui, sans-serif";

    const title = document.createElement("div");
    title.style.fontSize = "18px";
    title.style.opacity = "0.92";
    title.style.marginBottom = "14px";
    title.textContent = t("startingSoon", lang);

    const counter = document.createElement("div");
    counter.style.fontSize = "56px";
    counter.style.fontWeight = "800";
    counter.style.letterSpacing = "0.02em";
    counter.textContent = "";

    overlay.appendChild(title);
    overlay.appendChild(counter);
    document.body.appendChild(overlay);

    const startedAt = Date.now();
    const tick = () => {
      const left = Math.max(0, delayMs - (Date.now() - startedAt));
      const sec = Math.ceil(left / 1000);
      counter.textContent = String(sec);
      if (left <= 0) {
        overlay.remove();
        onDone();
        return;
      }
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }

  window.vworkTutorialRunner = {
    start: async function start(opts) {
      const frameId = opts.frameId || "tutorialFrame";
      const statusId = opts.statusId || "tutorialStatus";
      const scenario = opts.scenario || "login_demo";
      const cbPort = opts.cbPort || "";
      const token = opts.token || "";
      const autostart = opts.autostart !== false && opts.autostart !== "false";
      const delayMs = Number(opts.autostartDelayMs ?? 0);
      const speed = clamp(toNumber(opts.speed, 3), 0.5, 3);
      const stepDelayMs = clamp(toNumber(opts.stepDelayMs ?? opts.stepDelay ?? 500, 500), 0, 2000);
      const guide = opts.guide !== false && opts.guide !== "false";
      const startOverlay = opts.startOverlay !== false && opts.startOverlay !== "false";

      const frame = getEl(frameId);
      const statusEl = getEl(statusId);
      ensureParentCursorHidden(frame);
      if (!guide) {
        try { statusEl.style.display = "none"; } catch (_) {}
      }

      const vars = {
        ...readQueryVars(),
        ...parseVars(opts)
      };
      const lang = normalizeLang(vars.lang);
      // Sync lang to localStorage immediately so that any same-origin iframe
      // loaded by this tutorial session picks up the correct language via i18n.js.
      // Without this, a previous batch task may have written a different lang to
      // localStorage, causing the iframe's vWork UI to appear in the wrong language
      // even though the URL has the correct lang param.
      if (lang) {
        try { localStorage.setItem('u-nai_lang', lang); } catch (_) {}
        try { localStorage.setItem('language', lang); } catch (_) {}
      }
      const waitRecording =
        opts.waitRecording === true ||
        opts.waitRecording === "true" ||
        vars.waitRecording === true ||
        vars.waitRecording === "true";

      let scenarioData = null;
      try {
        statusEl.textContent = t("preloadScenario", lang);
        const scenarioUrl = `/static/tutorials/${encodeURIComponent(scenario)}.json`;
        const res = await fetch(scenarioUrl, { cache: "no-store" });
        if (res.ok) scenarioData = await res.json();
      } catch (_) {}

      try {
        const steps = scenarioData && Array.isArray(scenarioData.steps) ? scenarioData.steps : [];
        const first = steps.length > 0 ? steps[0] : null;
        if (first && String(first.action || "") === "goto") {
          const preUrl = interpolate(first.url, { ...parseVars(scenarioData.vars), ...vars });
          if (preUrl) {
            statusEl.textContent = `${t("preloadPagePrefix", lang)}${preUrl}`;
            const loadPromise = waitForFrameLoad(frame, 20000);
            frame.src = preUrl;
            await loadPromise;
          }
        }
      } catch (_) {}

      const waitForRecordingReady = async () => {
        if (!waitRecording) return "ready";
        if (!cbPort || !token) return "ready";
        const deadline = Date.now() + 20000;
        const firstAttemptAt = Date.now();
        let sawOk = false;
        while (Date.now() < deadline) {
          try {
            const url =
              `http://127.0.0.1:${encodeURIComponent(String(cbPort))}` +
              `/tutorial/ready?token=${encodeURIComponent(String(token))}`;
            const res = await fetch(url, { method: "GET", mode: "cors", cache: "no-store" });
            if (res.status === 401) return "unreachable";
            if (res.ok) {
              sawOk = true;
              const text = (await res.text()).trim();
              if (text === "1") return "ready";
            }
          } catch (_) {}
          if (!sawOk && Date.now() - firstAttemptAt > 8000) return "unreachable";
          await sleep(150);
        }
        return "timeout";
      };

      const startRun = async () => {
        try {
          if (waitRecording) statusEl.textContent = t("waitingRecording", lang);
          const readyState = await waitForRecordingReady();
          if (waitRecording && readyState === "timeout") {
            statusEl.textContent = t("notDetected", lang);
            createStartOverlay(async () => {
              await sleep(stepDelayMs);
              await runScenario({
                scenario,
                vars,
                frame,
                statusEl,
                cbPort,
                token,
                ctx: { speed, cbPort, token, stepDelayMs, guide },
                scenarioData
              });
            });
            return;
          }
          await sleep(stepDelayMs);
          await runScenario({
            scenario,
            vars,
            frame,
            statusEl,
            cbPort,
            token,
            ctx: { speed, cbPort, token, stepDelayMs, guide },
            scenarioData
          });
        } catch (e) {
          console.error("Scenario Error:", e);
          statusEl.textContent = `錯誤：${e && e.message ? e.message : String(e)}`;
          statusEl.style.color = "#ff4444";
          statusEl.style.fontWeight = "bold";
          try {
            clearHighlight(frame);
          } catch (_) {
          }
          
          if (cbPort) {
            await notifyDone(cbPort, scenario, token, e && e.message ? e.message : String(e));
          }
        }
      };

      if (delayMs > 0) {
        statusEl.textContent = "等待開始…";
        if (startOverlay) {
          createCountdownOverlay(delayMs, startRun);
        } else {
          setTimeout(() => startRun(), delayMs);
        }
      } else if (autostart) {
        startRun();
      } else {
        statusEl.textContent = "等待開始…";
        if (startOverlay) createStartOverlay(startRun);
      }
    }
  };
})();
