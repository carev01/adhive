const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const rules = require('@duckduckgo/autoconsent');
const { PlaywrightBlocker } = require('@ghostery/adblocker-playwright');
const fetch = require('cross-fetch');

// ─── Version Auto-Detection ─────────────────────────────────────────────────

// Derive the Chrome version from Playwright's bundled browsers.json so
// User-Agent, sec-ch-ua, and all other fingerprint signals stay in sync
// with the actual Chromium binary. No more hardcoded version strings.
function detectBundledChromeVersion() {
  try {
    const browsersPath = path.join(
      path.dirname(require.resolve('playwright-core')),
      'browsers.json'
    );
    const data = JSON.parse(fs.readFileSync(browsersPath, 'utf8'));
    const chromium = data.browsers?.find((b) => b.name === 'chromium');
    if (chromium?.browserVersion) {
      // Extract major version (e.g. "145.0.7632.6" → "145")
      return chromium.browserVersion.split('.')[0];
    }
  } catch {}
  return '145'; // safe fallback
}

const CHROME_MAJOR = detectBundledChromeVersion();

// Build consistent fingerprint strings from the detected version.
function buildUserAgent(majorVersion) {
  return `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/${majorVersion}.0.0.0 Safari/537.36`;
}

function buildSecChUa(majorVersion) {
  return `"Google Chrome";v="${majorVersion}", "Chromium";v="${majorVersion}", "Not.A/Brand";v="24"`;
}

// ─── Realistic Viewport Pool ────────────────────────────────────────────────

// Common desktop resolutions from StatCounter global stats.
// Picked once per browser context (not per navigation) to match real behavior.
const VIEWPORT_POOL = [
  { width: 1920, height: 1080 },
  { width: 1366, height: 768 },
  { width: 1536, height: 864 },
  { width: 1440, height: 900 },
  { width: 1280, height: 720 },
  { width: 1600, height: 900 },
];

function pickViewport(configWidth, configHeight) {
  if (configWidth && configHeight) {
    return { width: configWidth, height: configHeight };
  }
  return VIEWPORT_POOL[Math.floor(Math.random() * VIEWPORT_POOL.length)];
}

// ─── WebGL Renderer Pool ────────────────────────────────────────────────────

// Small pool of plausible GPU renderer strings to reduce cross-session
// fingerprint linkage. Picked once per browser context.
const WEBGL_RENDERERS = [
  { vendor: 'Google Inc. (NVIDIA)', renderer: 'ANGLE (NVIDIA, NVIDIA GeForce GTX 1650 Direct3D11 vs_5_0 ps_5_0, D3D11)' },
  { vendor: 'Google Inc. (NVIDIA)', renderer: 'ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)' },
  { vendor: 'Google Inc. (Intel)', renderer: 'ANGLE (Intel, Intel(R) UHD Graphics 630 Direct3D11 vs_5_0 ps_5_0, D3D11)' },
  { vendor: 'Google Inc. (AMD)', renderer: 'ANGLE (AMD, AMD Radeon RX 580 Direct3D11 vs_5_0 ps_5_0, D3D11)' },
  { vendor: 'Google Inc. (Intel)', renderer: 'ANGLE (Intel, Intel(R) Iris Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)' },
];

// New Autoconsent approach
async function attachAutoconsent(context, options = {}) {
  // Autoconsent uses 'optIn' instead of 'accept' internally
  const action = options.action || 'optIn'; 
  const timeout = options.timeout || 10000;

  await context.addInitScript({
    content: `
      window.__autoconsent_action = '${action}';
      window.__autoconsent_timeout = ${timeout};
      ${rules.bundledScript}
    `
  });

  context.on('page', (page) => {
    page.on('console', (msg) => {
      if (msg.text().startsWith('[autoconsent]')) console.log(msg.text());
    });
  });
}

let adblockerCache = null;
async function fetchEasyListCookie() {
  if (adblockerCache) return adblockerCache;
  // Automatically fetches and parses the Adblock syntax into proper Playwright interceptors and CSS hiding rules
  adblockerCache = await PlaywrightBlocker.fromLists(fetch, [
    'https://secure.fanboy.co.nz/fanboy-cookiemonster.txt'
  ]);
  return adblockerCache;
}

// ─── Error Classification ───────────────────────────────────────────────────

function classifyErrorType(message) {
  const lower = (message || '').toLowerCase();
  if (lower.includes('timeout')) return 'timeout';
  if (lower.includes('net::') || lower.includes('network')) return 'network_error';
  if (lower.includes('econnrefused') || lower.includes('enotfound')) return 'connection_error';
  if (lower.includes('ssl') || lower.includes('cert')) return 'ssl_error';
  if (lower.includes('blocked') || lower.includes('forbidden') || lower.includes('captcha')) return 'blocked';
  return 'unknown';
}

// ─── Full-Screen Overlay Removal ────────────────────────────────────────────

// Removes age gates, consent modals, and other full-screen overlays using
// computed styles — catches overlays that CSS selectors alone would miss
// (e.g., styles applied via external stylesheets or JS frameworks).
async function removeFullScreenOverlays(page) {
  return await page.evaluate(() => {
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    // ── Keyword lists (mirrors Go HeuristicConfig) ──

    const ATTR_KEYWORDS = [
      'age-gate', 'agegate', 'age-verif', 'age_verif', 'age-check',
      'age-modal', 'age-overlay', 'age-warning', 'age-popup',
      'adult-modal', 'adult-warning', 'nsfw-gate',
      'over-18', 'over18', 'modal-adulto',
      'cookie-consent', 'cookie-banner', 'cookie-notice',
      'consent-banner', 'consent-modal', 'consent-popup',
      'gdpr-banner', 'lgpd-banner',
    ];

    const TEXT_KEYWORDS = [
      'are you 18', 'verify your age', 'age verification',
      'you must be 18', '18 years of age', 'adult content',
      '18 anos', 'maiores de 18', 'conteúdo adulto',
      'contenido adulto', 'mayor de edad',
      'we use cookies', 'cookie policy',
    ];

    const THRESHOLD = 4.0;
    const MAX_TEXT_LEN = 2000;

    // ── Scoring ──

    function scoreElement(el) {
      let score = 0;
      const reasons = [];
      const add = (w, reason) => { score += w; reasons.push(reason); };

      const s = getComputedStyle(el);

      // --- Layout signals (from computed styles — no CSS cascade blindness) ---
      const isFixed = s.position === 'fixed' || s.position === 'sticky';
      if (isFixed) {
        add(1.5, 'computed:fixed/sticky');
      }

      const r = el.getBoundingClientRect();
      if (isFixed && r.width >= vw * 0.4 && r.height >= vh * 0.4) {
        add(1.5, 'computed:fullscreen');
      }

      const z = parseInt(s.zIndex, 10);
      if (z > 999) {
        add(1.0, `computed:z-index:${z}`);
      }

      // --- Attribute keywords (id, class, data-*) ---
      const attrText = [
        el.id || '',
        typeof el.className === 'string' ? el.className : '',
        ...Array.from(el.attributes)
          .filter(a => a.name.startsWith('data-'))
          .map(a => a.value),
      ].join(' ').toLowerCase();

      for (const kw of ATTR_KEYWORDS) {
        if (attrText.includes(kw)) {
          add(3.0, `attr:${kw}`);
          break;
        }
      }

      // --- ARIA / role ---
      if (el.getAttribute('role') === 'dialog' ||
          el.getAttribute('aria-modal') === 'true') {
        add(1.5, 'aria:dialog/modal');
      }

      // --- Backdrop detection (low-opacity fixed overlay behind modal) ---
      if (isFixed && z > 999) {
        const opacity = parseFloat(s.opacity);
        if (opacity < 0.95 && r.width >= vw * 0.9 && r.height >= vh * 0.9) {
          add(2.0, 'computed:backdrop');
        }
      }

      // --- Text keywords (only if already some signal — avoid cost) ---
      if (score >= 1.0) {
        const text = (el.innerText || '').toLowerCase();
        for (const kw of TEXT_KEYWORDS) {
          if (text.includes(kw)) {
            add(2.0, `text:${kw}`);
            break;
          }
        }
        // Penalty: too much text means it's probably real content
        if (text.length > MAX_TEXT_LEN) {
          add(-3.0, 'penalty:large-text');
        }
      }

      return { score, reasons };
    }

    // ── Collect candidates ──
    // Walk top-down; if a parent is removed, skip its children.

    const toRemove = [];
    const removedSet = new Set();

    for (const el of document.querySelectorAll('body *')) {
      // Skip if an ancestor is already marked
      let dominated = false;
      for (let p = el.parentElement; p; p = p.parentElement) {
        if (removedSet.has(p)) { dominated = true; break; }
      }
      if (dominated) continue;

      // Skip invisible elements (nothing to remove)
      const s = getComputedStyle(el);
      if (s.display === 'none' || s.visibility === 'hidden') continue;

      const { score, reasons } = scoreElement(el);
      if (score >= THRESHOLD) {
        toRemove.push({
          el,
          tag: el.tagName.toLowerCase(),
          id: el.id || '',
          classes: (typeof el.className === 'string'
            ? el.className : '').substring(0, 200),
          score,
          reasons,
        });
        removedSet.add(el);
      }
    }

    // ── Remove ──
    const removed = [];
    for (const entry of toRemove) {
      entry.el.remove();
      removed.push({
        tag: entry.tag,
        id: entry.id,
        classes: entry.classes,
        score: entry.score,
        reasons: entry.reasons,
      });
    }

    // ── Unlock scroll ──
    let scrollUnlocked = false;
    for (const root of [document.body, document.documentElement]) {
      if (root && getComputedStyle(root).overflow === 'hidden') {
        root.style.overflow = '';
        scrollUnlocked = true;
      }
    }

    return {
      removed_count: removed.length,
      removed_elements: removed,
      scroll_unlocked: scrollUnlocked,
    };
  }).catch(() => ({
    removed_count: 0,
    removed_elements: [],
    scroll_unlocked: false,
  }));
}

// ─── Challenge Detection ────────────────────────────────────────────────────

// FIX: WEAK_SIGNALS now includes 'recaptcha' to match what detectChallengeSignals
// considers weak. Previously only 'cloudflare' was listed here, causing 'recaptcha'
// to be treated as a strong signal during manual-mode post-checks.
const WEAK_SIGNALS = ['cloudflare', 'recaptcha'];

function hasVisibleChallengeElement(html) {
  if (!html) return false;
  // FIX: removed redundant .toLowerCase() — the regex already uses the /i flag.
  const lowerHtml = String(html);

  const patterns = [
    /<iframe[^>]*class=["'][^"']*(g-recaptcha|h-captcha|recaptcha)[^"']*["'][^>]*>/i,
    /class=["'][^"']*(cf-challenge|challenge-platform|challenge-container|challenge-wrapper|challenge-form|challenge-stage)["']/i,
    /<(?:div|main|body)[^>]*(?:id|class)=["'][^"']*(cf-challenge|cf-browser-verification|cf-ic-s|cf-ic)[^"']*["']/i,
    /<(?:div|section|h1|h2|p|span)[^>]*>[^<]*(?:just a moment|checking your browser|please verify you are human|access denied|bot check|security check)[^<]*<\/(?:div|section|h1|h2|p|span)>/i,
  ];

  return patterns.some((regex) => regex.test(lowerHtml));
}

// FIX: Helper to detect actual reCAPTCHA presence (not just CSS class names).
// Pages may have CSS classes like "g-recaptcha-cookies" that contain "recaptcha"
// but are not actual challenge elements. Real reCAPTCHA requires:
// - A script tag loading recaptcha/api.js or recaptcha/enterprise
// - OR an iframe with g-recaptcha/h-captcha class (not just a CSS reference)
function hasActualRecaptcha(html) {
  if (!html) return false;
  const lowerHtml = html.toLowerCase();
  
  // Check for actual reCAPTCHA script inclusion
  if (lowerHtml.includes('<script') && 
      (lowerHtml.includes('recaptcha/api.js') || lowerHtml.includes('recaptcha/enterprise'))) {
    return true;
  }
  
  // Check for actual reCAPTCHA iframe (not CSS class declarations)
  // Look for iframe tags with recaptcha-related class attributes
  const iframeMatch = lowerHtml.match(/<iframe[^>]*class=["'][^"']*["']/gi);
  if (iframeMatch) {
    for (const match of iframeMatch) {
      if (match.includes('g-recaptcha') || match.includes('h-captcha')) {
        // Verify it's not just a CSS class reference in a style block
        if (!match.includes('class="g-recaptcha-') && !match.includes("class='g-recaptcha-")) {
          return true;
        }
      }
    }
  }
  
  return false;
}

function detectChallengeSignals(html, finalURL, resourceCount, statusCode, redirectChain = []) {
  // FIX: Search HTML and URL separately to avoid cross-contamination of matches
  // (e.g., a URL fragment matching an HTML-only pattern or vice versa).
  const lowerHtml = (html || '').toLowerCase();
  const lowerUrl = (finalURL || '').toLowerCase();

  // FIX: Lowered threshold from 50 to 25. Pages with 25+ resources are
  // substantial enough to warrant stronger evidence for challenge detection.
  // The original threshold of 50 was too high, causing false positives
  // on pages with legitimate content but fewer resources (e.g., 26 resources).
  const resourceThreshold = 25;
  const hasHighResourceCount = resourceCount >= resourceThreshold;
  const isSuccessfulLoad = statusCode >= 200 && statusCode < 400;
  const requiresStrongerEvidence = hasHighResourceCount && isSuccessfulLoad;

  const strongSignals = [
    'hcaptcha', 'access denied', 'verify you are human',
    'verifique se voce e humano', 'please enable javascript and cookies',
    'checking your browser', 'attention required', 'bot challenge',
    'cf-challenge'
  ];

  // FIX: "just a moment" and "ray id" moved to context-dependent signals.
  // "just a moment" is common in legitimate content; "ray id" appears in
  // standard Cloudflare response headers on non-blocked pages. They should
  // only count as strong signals when paired with Cloudflare-specific context.
  const contextSignals = [
    { text: 'just a moment', requires: ['cloudflare', 'cf-'] },
    { text: 'ray id', requires: ['cloudflare', 'cf-', '/cdn-cgi/'] },
    { 
        text: 'cdn-cgi/challenge', 
        // Only count as strong if the page didn't load successfully (e.g., HTTP 403)
        // or if a visible challenge element was detected
        requiresContext: () => !isSuccessfulLoad || hasVisibleChallengeElement(html) 
    }
  ];
  
  const weakSignals = ['recaptcha', 'cloudflare'];

  // FIX: Only use final URL for challenge signal detection, not redirect chain.
  // Redirect chain URLs can contain challenge patterns (e.g., /cdn-cgi/challenge) even when
  // the page loads successfully. We only want to check the final loaded URL.
  const searchText = lowerHtml;

  const foundStrong = strongSignals.filter((s) => searchText.includes(s));

  // Context signals: only count as strong if their required context is present
  for (const ctx of contextSignals) {
    if (searchText.includes(ctx.text)) {
      let hasContext = false;
      if (ctx.requiresContext) {
        hasContext = ctx.requiresContext();
      } else if (ctx.requires) {
        hasContext = ctx.requires.some(req => searchText.includes(req));
      }
      if (hasContext) foundStrong.push(ctx.text);
    }
  }

  // FIX: Filter weak signals more carefully. For 'recaptcha', only count it
  // if there's an actual reCAPTCHA script/iframe, not just CSS class names.
  const foundWeak = weakSignals.filter((s) => {
    if (s === 'recaptcha') {
      // Only count recaptcha if there's actual reCAPTCHA implementation
      return hasActualRecaptcha(html);
    }
    return searchText.includes(s);
  });

  const urlLooksChallenge =
    lowerUrl.includes('/cdn-cgi/challenge') ||
    lowerUrl.includes('/cdn-cgi/l/chk_jschl') ||
    lowerUrl.includes('/challenge');

  const hasVisibleChallenge = hasVisibleChallengeElement(html);
  if (hasVisibleChallenge) {
    foundStrong.push('visible_challenge_element');
  }

  if (foundStrong.length > 0 || urlLooksChallenge) {
    const validWeakSignals =
      requiresStrongerEvidence && !hasVisibleChallenge ? [] : foundWeak;
    return Array.from(new Set([...foundStrong, ...validWeakSignals]));
  }

  if (foundWeak.length >= 2 && !requiresStrongerEvidence) {
    // FIX: Don't treat weak signals as challenge evidence if:
    // - The page loaded successfully (HTTP 200-399)
    // - No visible challenge element in HTML
    // - Substantial content exists (10+ resources OR 50KB+ HTML)
    // This prevents false positives from CSS class names like "g-recaptcha-cookies"
    // or CDN references like "cloudflare" that appear in legitimate page content.
    if (isSuccessfulLoad && !hasVisibleChallenge && (resourceCount >= 10 || lowerHtml.length > 50000)) {
      return [];  // False positive: weak signals are CSS class names or CDN refs
    }
    return foundWeak;
  }

  return [];
}

function isCrossDomainRedirect(originalUrl, finalUrl) {
  try {
    return new URL(originalUrl).hostname !== new URL(finalUrl).hostname;
  } catch {
    return false;
  }
}

function isSocialMediaRedirect(finalUrl) {
  const socialDomains = [
    'instagram.com', 'facebook.com', 'twitter.com', 'x.com',
    'tiktok.com', 'youtube.com', 'whatsapp.com', 'telegram.org',
    't.me', 'linkedin.com',
  ];
  try {
    // FIX: escaped the dot in the regex (was `www.`, now `www\.`)
    const hostname = new URL(finalUrl).hostname.replace(/^www\./, '');
    return socialDomains.some((d) => hostname === d || hostname.endsWith('.' + d));
  } catch {
    return false;
  }
}

// ─── Stealth Setup ──────────────────────────────────────────────────────────

// FIX: Extracted stealth into a dedicated factory function with proper mocks.
// Accepts webglProfile for per-context renderer rotation.
function getStealthInitScript(config = {}) {
  const platform = config.platform || 'Win32';
  const languages = config.languages || ['pt-BR', 'pt', 'en'];
  const webglVendor = config.webglVendor || 'Google Inc. (NVIDIA)';
  const webglRenderer = config.webglRenderer || 'ANGLE (NVIDIA, NVIDIA GeForce GTX 1650 Direct3D11 vs_5_0 ps_5_0, D3D11)';

  return () => {
    // 1. Remove webdriver flag — also delete from prototype chain
    Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
    try {
      delete Object.getPrototypeOf(navigator).webdriver;
    } catch {}

    // 2. FIX (CRITICAL): navigator.plugins must return a PluginArray-like object.
    //    The original code returned [1, 2, 3, 4, 5] — an array of plain numbers —
    //    which is trivially detectable: navigator.plugins[0].name === undefined
    //    instantly reveals automation.
    const makePlugin = (name, description, filename, mimeType) => {
      const mime = {
        type: mimeType,
        suffixes: '',
        description,
        enabledPlugin: null,
      };
      const plugin = {
        name,
        description,
        filename,
        length: 1,
        0: mime,
        item: (i) => (i === 0 ? mime : null),
        namedItem: (n) => (n === mimeType ? mime : null),
      };
      mime.enabledPlugin = plugin;
      return plugin;
    };

    const plugins = [
      makePlugin(
        'Chrome PDF Plugin',
        'Portable Document Format',
        'internal-pdf-viewer',
        'application/x-google-chrome-pdf'
      ),
      makePlugin(
        'Chrome PDF Viewer',
        '',
        'mhjfbmdgcfjbbpaeojofohoefgiehjai',
        'application/pdf'
      ),
      makePlugin(
        'Native Client',
        '',
        'internal-nacl-plugin',
        'application/x-nacl'
      ),
    ];

    const pluginArray = Object.create(PluginArray.prototype);
    plugins.forEach((p, i) => {
      pluginArray[i] = p;
    });
    Object.defineProperty(pluginArray, 'length', {
      get: () => plugins.length,
    });
    pluginArray.item = (i) => plugins[i] || null;
    pluginArray.namedItem = (name) =>
      plugins.find((p) => p.name === name) || null;
    pluginArray.refresh = () => {};

    Object.defineProperty(navigator, 'plugins', { get: () => pluginArray });

    // 3. Languages (match locale config)
    Object.defineProperty(navigator, 'languages', {
      get: () => [...languages],
    });

    // 4. Platform must match User-Agent
    Object.defineProperty(navigator, 'platform', { get: () => platform });

    // 5. Hardware fingerprint (realistic values)
    Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 });
    if ('deviceMemory' in navigator) {
      Object.defineProperty(navigator, 'deviceMemory', { get: () => 8 });
    }

    // 6. FIX: Comprehensive window.chrome mock (original only had `runtime: {}`)
    window.chrome = {
      runtime: {
        connect: () => {},
        sendMessage: () => {},
        onMessage: { addListener: () => {}, removeListener: () => {} },
        onConnect: { addListener: () => {}, removeListener: () => {} },
        id: undefined,
      },
      csi: () => ({
        startE: Date.now(),
        onloadT: Date.now() + 100,
        pageT: 300,
        tran: 15,
      }),
      loadTimes: () => ({
        commitLoadTime: Date.now() / 1000,
        connectionInfo: 'h2',
        finishDocumentLoadTime: Date.now() / 1000 + 0.1,
        finishLoadTime: Date.now() / 1000 + 0.2,
        firstPaintAfterLoadTime: 0,
        firstPaintTime: Date.now() / 1000 + 0.05,
        navigationType: 'Other',
        npnNegotiatedProtocol: 'h2',
        requestTime: Date.now() / 1000 - 0.5,
        startLoadTime: Date.now() / 1000 - 0.4,
        wasAlternateProtocolAvailable: false,
        wasFetchedViaSpdy: true,
        wasNpnNegotiated: true,
      }),
    };

    // 7. Permissions API override — return 'prompt' for all permission types
    //    in headless mode. Real fresh Chrome profiles return 'prompt' for
    //    everything. The original only handled 'notifications'; anti-bot
    //    scripts also probe clipboard-read, camera, microphone, geolocation.
    const originalQuery = Permissions.prototype.query;
    Permissions.prototype.query = function (parameters) {
      return Promise.resolve({
        state: 'prompt',
        onchange: null,
      });
    };

    // 8. WebGL vendor/renderer spoofing — values are rotated per browser
    //    context (passed via config) to reduce cross-session fingerprint
    //    linkage. Headless Chrome's default "Google SwiftShader" is a
    //    strong automation signal.
    const _glVendor = webglVendor;
    const _glRenderer = webglRenderer;
    const getParameterProto =
      WebGLRenderingContext.prototype.getParameter;
    WebGLRenderingContext.prototype.getParameter = function (param) {
      if (param === 0x9245) return _glVendor;
      if (param === 0x9246) return _glRenderer;
      return getParameterProto.call(this, param);
    };

    if (typeof WebGL2RenderingContext !== 'undefined') {
      const getParameter2Proto =
        WebGL2RenderingContext.prototype.getParameter;
      WebGL2RenderingContext.prototype.getParameter = function (param) {
        if (param === 0x9245) return _glVendor;
        if (param === 0x9246) return _glRenderer;
        return getParameter2Proto.call(this, param);
      };
    }

    // 9. FIX: Remove known automation framework indicators
    const automationGlobals = [
      '__nightmare',
      '_phantom',
      'callPhantom',
      '_selenium',
      '__webdriver_evaluate',
      '__webdriver_unwrap',
      '__driver_evaluate',
      '__driver_unwrap',
      '__fxdriver_evaluate',
      '__fxdriver_unwrap',
    ];
    for (const prop of automationGlobals) {
      try {
        delete window[prop];
      } catch {}
    }
  };
}

// ─── Lazy-Load Scrolling ────────────────────────────────────────────────────

async function autoScrollForLazyLoadedImages(page, options = {}) {
  const maxRounds = Math.min(
    Math.max(Number(options.maxRounds ?? 12) || 12, 1),
    50
  );
  const stepPx = Math.min(
    Math.max(Number(options.stepPx ?? 800) || 800, 100),
    2500
  );
  const idleRoundsToStop = Math.min(
    Math.max(Number(options.idleRoundsToStop ?? 2) || 2, 1),
    8
  );
  const settleMs = Math.min(
    Math.max(Number(options.settleMs ?? 300) || 300, 50),
    2000
  );

  const defaultMetrics = {
    skipped: false,
    rounds: 0,
    max_scroll_y: 0,
    document_height: 0,
    images_before: 0,
    images_after: 0,
    loaded_images_before: 0,
    loaded_images_after: 0,
    lazy_candidates_before: 0,
    lazy_candidates_after: 0,
  };

  const metrics = await page
    .evaluate(
      async ({ maxRounds, stepPx, idleRoundsToStop, settleMs }) => {
        const wait = (ms) =>
          new Promise((resolve) => setTimeout(resolve, ms));
        const docHeight = () =>
          Math.max(
            (document.scrollingElement ||
              document.documentElement ||
              document.body)?.scrollHeight || 0,
            document.documentElement?.scrollHeight || 0,
            document.body?.scrollHeight || 0
          );

        const countImages = () => {
          const images = Array.from(document.images || []);
          const loaded = images.filter((img) => {
            const hasSource = Boolean(img.currentSrc || img.src);
            return hasSource && (img.complete || img.naturalWidth > 0);
          }).length;
          const lazyCandidates = images.filter(
            (img) =>
              !img.complete ||
              img.loading === 'lazy' ||
              img.hasAttribute('data-src') ||
              img.hasAttribute('data-original') ||
              img.hasAttribute('data-lazy-src') ||
              img.hasAttribute('data-lazy')
          ).length;
          return { total: images.length, loaded, lazyCandidates };
        };

        const before = countImages();
        let rounds = 0;
        let idleRounds = 0;
        let maxScrollY = window.scrollY || 0;
        let previousHeight = docHeight();

        for (let i = 0; i < maxRounds; i++) {
          const currentHeight = docHeight();
          const targetY = Math.min(
            (window.scrollY || 0) + stepPx,
            Math.max(currentHeight - window.innerHeight, 0)
          );

          window.scrollTo(0, targetY);
          rounds++;
          maxScrollY = Math.max(maxScrollY, window.scrollY || 0);
          window.dispatchEvent(new Event('scroll'));
          await wait(settleMs);

          const newHeight = docHeight();
          const reachedBottom =
            window.scrollY + window.innerHeight >= newHeight - 4;
          const grew = newHeight > currentHeight + 8;

          if (
            reachedBottom &&
            !grew &&
            Math.abs(newHeight - previousHeight) < 8
          ) {
            idleRounds++;
          } else {
            idleRounds = 0;
          }

          previousHeight = newHeight;
          if (idleRounds >= idleRoundsToStop) break;
        }

        window.scrollTo(0, 0);
        await wait(Math.min(250, settleMs));
        const after = countImages();

        return {
          skipped: false,
          rounds,
          max_scroll_y: maxScrollY,
          document_height: docHeight(),
          images_before: before.total,
          images_after: after.total,
          loaded_images_before: before.loaded,
          loaded_images_after: after.loaded,
          lazy_candidates_before: before.lazyCandidates,
          lazy_candidates_after: after.lazyCandidates,
        };
      },
      { maxRounds, stepPx, idleRoundsToStop, settleMs }
    )
    .catch(() => ({ ...defaultMetrics }));

  return { ...defaultMetrics, ...metrics };
}

// ─── DOM Asset Extraction ───────────────────────────────────────────────────

async function extractDomAssetUrls(page) {
  const domAssetUrls = await page
    .evaluate(() => {
      const urls = new Set();

      const addUrl = (raw) => {
        if (!raw) return;
        const value = String(raw).trim();
        if (!value || value === 'about:blank') return;
        if (/^(?:data:|blob:|javascript:|mailto:|tel:|#)/i.test(value))
          return;
        try {
          const absolute = new URL(value, document.baseURI);
          if (
            absolute.protocol !== 'http:' &&
            absolute.protocol !== 'https:'
          )
            return;
          absolute.hash = '';
          urls.add(absolute.toString());
        } catch {
          /* malformed URL */
        }
      };

      const srcsetCandidates = (srcset = '') =>
        srcset
          .split(',')
          .map((c) => c.trim().split(/\s+/)[0])
          .filter(Boolean);

      // FIX: Create a fresh regex inside the function to avoid lastIndex leaking
      // across calls when an error interrupts the while loop mid-iteration.
      const addCssUrls = (cssText = '') => {
        const regex = /url\(\s*(['"]?)([^'")]+)\1\s*\)/g;
        let match;
        while ((match = regex.exec(cssText)) !== null) addUrl(match[2]);
      };

      document.querySelectorAll('img').forEach((el) => {
        addUrl(el.getAttribute('src'));
        addUrl(el.currentSrc);
        srcsetCandidates(el.getAttribute('srcset') || '').forEach(addUrl);
      });

      document.querySelectorAll('source').forEach((el) => {
        addUrl(el.getAttribute('src'));
        srcsetCandidates(el.getAttribute('srcset') || '').forEach(addUrl);
      });

      document
        .querySelectorAll('script[src]')
        .forEach((el) => addUrl(el.getAttribute('src')));

      document.querySelectorAll('link[href]').forEach((el) => {
        const rel = (el.getAttribute('rel') || '').toLowerCase();
        const as = (el.getAttribute('as') || '').toLowerCase();
        if (
          /stylesheet|preload|prefetch|modulepreload/.test(rel) ||
          /style|script|font|image/.test(as)
        ) {
          addUrl(el.getAttribute('href'));
        }
      });

      document.querySelectorAll('video, audio').forEach((el) => {
        addUrl(el.getAttribute('src'));
        addUrl(el.getAttribute('poster'));
      });

      document
        .querySelectorAll('track[src]')
        .forEach((el) => addUrl(el.getAttribute('src')));

      document
        .querySelectorAll('[style]')
        .forEach((el) => addCssUrls(el.getAttribute('style') || ''));
      document
        .querySelectorAll('style')
        .forEach((el) => addCssUrls(el.textContent || ''));

      return Array.from(urls);
    })
    .catch(() => []);

  return Array.isArray(domAssetUrls) ? domAssetUrls : [];
}

// ─── Manual Intervention ────────────────────────────────────────────────────

// FIX: Now accepts resourceCount and statusCode instead of hardcoding 0 and 200.
// The original code called detectChallengeSignals(html, url, 0, 200) which
// bypassed the resource-based heuristic (requiresStrongerEvidence was always
// false when resourceCount=0), causing false challenge detections in manual mode.
async function waitForManualIntervention(
  page,
  timeoutMs,
  resourceCount,
  statusCode
) {
  const start = Date.now();
  const maxWait = Number(timeoutMs || 120000);
  while (Date.now() - start < maxWait) {
    await page.waitForTimeout(1000);
    const html = await page.content().catch(() => '');
    const signals = detectChallengeSignals(
      html,
      page.url(),
      resourceCount,
      statusCode
    );
    if (signals.length === 0) {
      return { resolved: true, waitedMs: Date.now() - start };
    }
  }
  return { resolved: false, waitedMs: Date.now() - start };
}

// ─── Main Scrape Function ───────────────────────────────────────────────────

// FIX: Added configurable retry logic with exponential backoff for transient
// failures (network errors, timeouts, challenge blocks).
async function scrape(config) {
  const maxRetries = Math.min(
    Math.max(Number(config.retries ?? 0) || 0, 0),
    3
  );
  let lastError = null;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const result = await _scrapeAttempt(config, attempt);

      // If blocked and we have retries left, retry with backoff
      if (result.challenge_detected && attempt < maxRetries) {
        lastError = new Error('Challenge detected, retrying...');
        await new Promise((r) =>
          setTimeout(r, 2000 * (attempt + 1))
        );
        continue;
      }

      console.log(JSON.stringify(result));
      return;
    } catch (error) {
      lastError = error;
      if (attempt < maxRetries) {
        await new Promise((r) =>
          setTimeout(r, 2000 * (attempt + 1))
        );
        continue;
      }
    }
  }

  // All retries exhausted
  console.log(
    JSON.stringify({
      html: '',
      status_code: 0,
      error: lastError?.message || 'Unknown error',
      resource_urls: [],
      dom_asset_urls: [],
      cookies: [],
      redirect_chain: [],
      challenge_detected: false,
      challenge_signals: [],
      capture_mode: config.manualMode ? 'manual' : 'auto',
      timeout_stage: lastError?.timeoutStage || '',
      error_type: classifyErrorType(lastError?.message),
    })
  );
}

async function _scrapeAttempt(config, attemptNumber) {
  // Playwright uses bundled Chromium by default on Debian-based images.
  // The executablePath config is kept for backward compatibility with Alpine
  // deployments that need to use system Chromium (PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH).
  const executablePath = config.executablePath || process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || null;
  
  const launchOptions = {
    headless: config.headless !== false,
    args: [
      '--disable-blink-features=AutomationControlled',
      '--disable-features=IsolateOrigins,site-per-process',
      '--no-sandbox',
      '--disable-setuid-sandbox',
      '--disable-dev-shm-usage',
      '--disable-accelerated-2d-canvas',
      '--no-first-run',
      '--no-zygote',
      '--disable-gpu',
    ],
  };
  
  // If using system Chromium, specify the executable path
  if (executablePath) {
    launchOptions.executablePath = executablePath;
  }
  
  const browser = await chromium.launch(launchOptions);

  // All fingerprint signals derived from one source of truth: CHROME_MAJOR.
  const effectiveUA = config.userAgent || buildUserAgent(CHROME_MAJOR);
  const effectiveSecChUa = buildSecChUa(CHROME_MAJOR);

  // Viewport: pick once from realistic resolution pool (not per-page).
  const viewport = pickViewport(config.viewportWidth, config.viewportHeight);

  // WebGL: pick a random renderer for this context to reduce cross-session linkage.
  const webglProfile = WEBGL_RENDERERS[Math.floor(Math.random() * WEBGL_RENDERERS.length)];

  const context = await browser.newContext({
    userAgent: effectiveUA,
    viewport,
    locale: config.locale || 'pt-BR',
    timezoneId: config.timezoneId || 'America/Sao_Paulo',
    colorScheme: 'light',
    // All sec-ch-ua headers derived from the same Chrome version.
    extraHTTPHeaders: {
      'Accept-Language': 'pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7',
      Accept:
        'text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8',
      'sec-ch-ua': effectiveSecChUa,
      'sec-ch-ua-mobile': '?0',
      'sec-ch-ua-platform': '"Windows"',
    },
  });

  // Stealth init script — pass WebGL profile for per-context rotation.
  await context.addInitScript(
    getStealthInitScript({
      platform: 'Win32',
      languages: (config.locale || 'pt-BR').startsWith('pt')
        ? ['pt-BR', 'pt', 'en']
        : ['en-US', 'en'],
      webglVendor: webglProfile.vendor,
      webglRenderer: webglProfile.renderer,
    })
  );
  
  // --- HYBRID CONSENT: LAYERS 1 & 2 ---
  await attachAutoconsent(context, { action: 'optIn' }); 
  const blocker = await fetchEasyListCookie(); 

  const page = await context.newPage();
  
  // Applies EasyList rules (hides remaining banners and blocks consent scripts)
  await blocker.enableBlockingInPage(page); 
  
  const captureMode = config.manualMode ? 'manual' : 'auto';
  const resourceSet = new Set();
  const redirectChain = [];

  page.on('request', (req) => {
    const url = req.url();
    if (url) resourceSet.add(url);
  });

  page.on('response', (res) => {
    const req = res.request();
    if (req?.redirectedFrom()) redirectChain.push(req.redirectedFrom().url());
    const url = res.url();
    if (url) resourceSet.add(url);
  });

  let timeoutStage = '';

  try {
    timeoutStage = 'goto';
    const response = await page.goto(config.url, {
      waitUntil: config.waitFor || 'domcontentloaded',
      timeout: config.timeout || 30000,
    });

    // FIX: Use waitForLoadState instead of bare waitForTimeout for post-load settle
    timeoutStage = 'post_load_settle';
    await page.waitForLoadState('domcontentloaded').catch(() => {});
    await page.waitForTimeout(500);

	// --- HYBRID CONSENT: LAYER 3 (API FALLBACK) ---
    timeoutStage = 'cmp_api_fallback';
    await page.evaluate(() => {
      try {
        if (window.OneTrust) window.OneTrust.AllowAll();
        if (window.Cookiebot) window.Cookiebot.submitCustomConsent(1,1,1);
        if (window.__cmpapi) window.__cmpapi('setConsentStatus', {allConsent:true});
      } catch(e) {}
    }).catch(() => {});

    if (config.waitForSelector) {
      timeoutStage = 'wait_for_selector';
      await page
        .waitForSelector(config.waitForSelector, { timeout: 5000 })
        .catch(() => {});
    }
 
    // ── NEW: remove surviving overlays before scroll/capture ──
    timeoutStage = 'overlay_removal';
    const overlayResult = await removeFullScreenOverlays(page);
 
    // Lazy scroll
    timeoutStage = 'lazy_scroll';
    const lazyScroll = config.autoScroll !== false ? await autoScrollForLazyLoadedImages(page, {
      maxRounds: config.autoScrollMaxRounds,
      stepPx: config.autoScrollStepPx,
      idleRoundsToStop: config.autoScrollIdleRounds,
      settleMs: config.autoScrollSettleMs,
    }) : {
            skipped: true,
            rounds: 0,
            max_scroll_y: 0,
            document_height: 0,
            images_before: 0,
            images_after: 0,
            loaded_images_before: 0,
            loaded_images_after: 0,
            lazy_candidates_before: 0,
            lazy_candidates_after: 0,
          };

    // FIX: Use waitForLoadState('networkidle') instead of arbitrary 2000ms timeout
    // to wait for actual network activity to settle.
    await page.waitForLoadState('networkidle').catch(() => {});
    await page.waitForTimeout(1000);

    // Extract assets
    timeoutStage = 'dom_asset_inventory';
    const domAssetUrls = await extractDomAssetUrls(page);

    // Capture cookies
    timeoutStage = 'cookie_capture';
    const cookies = await context.cookies().catch(() => []);
    const persistedCookies = (Array.isArray(cookies) ? cookies : [])
      .map((c) => ({
        name: c?.name || '',
        value: c?.value || '',
        domain: c?.domain || '',
        path: c?.path || '/',
        expires: Number.isFinite(c?.expires) ? c.expires : 0,
        httpOnly: Boolean(c?.httpOnly),
        secure: Boolean(c?.secure),
        sameSite: c?.sameSite || '',
      }))
      .filter((c) => c.name && c.value);

    // FIX: Capture page content ONCE. The original code called page.content()
    // twice — once for challenge detection and again for the result when no
    // challenge was detected. This was wasteful and could return inconsistent
    // content if the page changed between calls.
    const html = await page.content();
    const finalURL = page.url();

    // Screenshot
    let screenshot = '';
    if (config.screenshot) {
      const buffer = await page.screenshot({ fullPage: false });
      screenshot = 'data:image/png;base64,' + buffer.toString('base64');
    }

    // Response headers
    const headers = {};
    if (response) {
      Object.entries(response.headers()).forEach(([k, v]) => {
        headers[k] = v;
      });
    }

    // Challenge detection
    const resourceCount = resourceSet.size;
    const statusCode = response ? response.status() : 200;
    let challengeSignals = detectChallengeSignals(
      html,
      finalURL,
      resourceCount,
      statusCode,
      redirectChain
    );
    let challengeDetected = challengeSignals.length > 0;

    // Cross-domain redirect check
    const crossDomainRedirect = isCrossDomainRedirect(config.url, finalURL);
    const socialMediaRedirect =
      crossDomainRedirect && isSocialMediaRedirect(finalURL);

    if (crossDomainRedirect) {
      challengeDetected = false;
      challengeSignals = [];
    }

    // Manual mode: wait for human to solve challenge
    if (challengeDetected && config.manualMode) {
      // FIX: Pass actual resourceCount and statusCode instead of hardcoded 0, 200
      const manualWait = await waitForManualIntervention(
        page,
        config.manualTimeoutMs || 120000,
        resourceCount,
        statusCode
      );
      if (manualWait.resolved) {
        const postHtml = await page.content().catch(() => html);
        const postSignals = detectChallengeSignals(
          postHtml,
          page.url(),
          resourceCount,
          statusCode,
          redirectChain
        );
        // FIX: Use the corrected WEAK_SIGNALS list that includes 'recaptcha'
        const strongRemain = postSignals.filter(
          (s) => !WEAK_SIGNALS.includes(s)
        );
        challengeDetected = strongRemain.length > 0;
        challengeSignals = challengeDetected ? postSignals : [];
      }
    }

    return {
      html,
      status_code: statusCode,
      final_url: finalURL,
      screenshot,
      headers,
      resource_urls: Array.from(resourceSet),
      dom_asset_urls: domAssetUrls,
      cookies: persistedCookies,
      redirect_chain: redirectChain,
      challenge_detected: challengeDetected,
      challenge_signals: challengeSignals,
      cross_domain_redirect: crossDomainRedirect,
      social_media_redirect: socialMediaRedirect,
      capture_mode: captureMode,
      // Removed: privacypopupclicks, consentredirecthandled, consentredirectresult
      lazy_scroll: lazyScroll,
      overlay_removal: overlayResult,
      timeout_stage: '',
      error_type: '',
      attempt: attemptNumber,
    };
  } catch (error) {
    // FIX: Attach context to error for better debugging upstream
    error.timeoutStage = timeoutStage;
    error.partialResourceUrls = Array.from(resourceSet);
    error.partialRedirectChain = redirectChain;
    throw error;
  } finally {
    await browser.close();
  }
}

module.exports = {
  detectChallengeSignals,
  classifyErrorType,
  scrape,
};

if (require.main === module) {
  const config = JSON.parse(process.argv[2]);
  scrape(config);
}
