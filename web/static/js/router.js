// vWork SPA Router — AJAX-based page navigation
// Intercepts internal CMS link clicks, fetches page HTML via AJAX,
// and swaps only the .main-content region without full page reload.

var Router = {
    // Whether the router is active
    _active: false,
    // Current navigation AbortController (to cancel in-flight fetches)
    _abortController: null,
    // Pages that should NOT use AJAX navigation (full reload instead)
    _excludedPaths: [
        '/login', '/logout', '/reset-password', '/',
        '/pos', '/subscription-required', '/setup-tenant',
        '/subscribe-now', '/tutorial/run', '/accept-invite'
    ],
    // Prefixes that should NOT use AJAX navigation
    _excludedPrefixes: [
        '/co/', '/help', '/contact', '/static/', '/api/',
        '/vmarket', '/sales-partner', '/tutorial', '/vai-', '/voffice-'
    ],
    // Track page-specific cleanup functions
    _cleanupFns: [],
    // Loading indicator element
    _loadingBar: null,
    // Scroll positions keyed by URL for back/forward
    _scrollPositions: {},
    // Phase 2: Auto-tracked page timers (setInterval/setTimeout IDs)
    _trackedIntervals: [],
    _trackedTimeouts: [],
    // Phase 2: Auto-tracked page event listeners [{target, type, fn, options}]
    _trackedListeners: [],
    // Phase 2: Body class/data snapshot before page swap
    _bodyClassSnapshot: null,
    _bodyDataSnapshot: {},
    // Phase 2: Original native functions (saved for monkey-patch restore)
    _origSetInterval: null,
    _origSetTimeout: null,
    _origWindowAddEventListener: null,
    _origDocAddEventListener: null,

    // ─── Initialization ───────────────────────────────────────────
    init: function() {
        if (this._active) return;
        this._active = true;

        // Create loading bar
        this._createLoadingBar();

        // Intercept clicks on links
        document.addEventListener('click', this._onLinkClick.bind(this), true);

        // Handle browser back/forward
        window.addEventListener('popstate', this._onPopState.bind(this));

        // Mark initial page in history state
        history.replaceState(
            { spa: true, url: window.location.pathname + window.location.search },
            '',
            window.location.href
        );

        // Take initial body snapshot so _restoreBody works on first SPA navigation
        // (e.g. navigating back from a full-reload excluded page like /pos)
        this._snapshotBody();

        // Set sidemenu active state on initial page load (full reload / refresh)
        this._updateSidemenuActive(window.location.href);
    },

    // ─── Loading Bar (removed) ──────────────────────────────────
    _createLoadingBar: function() {},
    _showLoading: function() {},
    _hideLoading: function() {},

    // ─── Should this URL be handled by SPA router? ────────────────
    _shouldIntercept: function(url) {
        try {
            var parsed = new URL(url, window.location.origin);
            if (parsed.origin !== window.location.origin) return false;
            var path = parsed.pathname;

            for (var i = 0; i < this._excludedPaths.length; i++) {
                if (path === this._excludedPaths[i]) return false;
            }
            for (var j = 0; j < this._excludedPrefixes.length; j++) {
                if (path.startsWith(this._excludedPrefixes[j])) return false;
            }
            // Skip file downloads (but allow .html)
            if (path.match(/\.\w+$/) && !path.endsWith('.html')) return false;

            return true;
        } catch (e) {
            return false;
        }
    },

    // ─── Link Click Handler ───────────────────────────────────────
    _onLinkClick: function(e) {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        if (e.button !== 0) return;

        var link = e.target.closest('a');
        if (!link) return;

        if (link.target === '_blank' || link.hasAttribute('download')) return;
        var href = link.getAttribute('href');
        if (!href || href.startsWith('javascript:') || href === '#') return;
        if (link.hasAttribute('onclick')) return;
        if (link.hasAttribute('data-spa-ignore')) return;
        // Don't intercept links inside modals
        if (link.closest('.modal')) return;

        var url;
        try {
            url = new URL(href, window.location.origin);
        } catch (e) {
            return;
        }
        if (!this._shouldIntercept(url.href)) return;

        // Same page — do nothing
        if (url.pathname === window.location.pathname &&
            url.search === window.location.search) {
            e.preventDefault();
            return;
        }

        e.preventDefault();
        this.navigate(url.pathname + url.search + url.hash);
    },

    // ─── Programmatic Navigation ──────────────────────────────────
    navigate: function(url, opts) {
        opts = opts || {};
        var pushState = opts.pushState !== false;
        var replaceState = opts.replace === true;
        var scrollTop = opts.scrollTop !== false;
        var restoreScroll = opts.restoreScroll === true;
        var self = this;

        // Save scroll position
        var scrollKey = window.location.pathname + window.location.search;
        var wrapper = document.querySelector('.main-content-wrapper');
        this._scrollPositions[scrollKey] = wrapper ? wrapper.scrollTop : window.scrollY;

        // Cancel in-flight request
        if (this._abortController) {
            try { this._abortController.abort(); } catch (e) {}
        }
        this._abortController = new AbortController();

        this._showLoading();

        // Close mobile sidebar if open
        try {
            var sidebar = document.getElementById('sidebar');
            var overlay = document.getElementById('sidebarOverlay');
            if (sidebar) sidebar.classList.remove('show');
            if (overlay) overlay.classList.remove('show');
        } catch (e) {}

        fetch(url, {
            signal: this._abortController.signal,
            headers: { 'X-SPA-Request': '1' },
            credentials: 'same-origin'
        })
        .then(function(response) {
            // Auth redirect detection: if server returned 401 or redirected
            // to login/sso page, do a full page reload instead of SPA swap.
            // NOTE: 403 is NOT treated as auth failure — it may come from
            // TrialExpiredMiddleware or RequireRole and should not trigger logout.
            var respUrl = response.url || '';
            var redirectedToAuth = /\/(login|sso)(\/|$|\?)/.test(respUrl);
            if (response.status === 401 || redirectedToAuth) {
                self._hideLoading();
                window.location.href = redirectedToAuth ? respUrl : url;
                return;
            }
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.text();
        })
        .then(function(html) {
            if (!html) return; // Auth redirect handled above
            if (replaceState) {
                history.replaceState({ spa: true, url: url }, '', url);
            } else if (pushState) {
                history.pushState({ spa: true, url: url }, '', url);
            }
            self._swapContent(html, url, scrollTop, restoreScroll);
            self._hideLoading();
        })
        .catch(function(err) {
            if (err.name === 'AbortError') return;
            console.error('[Router] Navigation failed:', err);
            self._hideLoading();
            window.location.href = url;
        });
    },

    // ─── Content Swap ─────────────────────────────────────────────
    _swapContent: function(html, url, scrollTop, restoreScroll) {
        // 1. Cleanup current page
        this._runCleanup();

        // 1.5 Strip page-specific body classes that full-reload pages may have added
        // (e.g. /pos adds 'pos-page', /dining-queue-board adds 'kiosk-mode')
        var pageClasses = ['pos-page', 'kiosk-mode', 'page-editor'];
        for (var pc = 0; pc < pageClasses.length; pc++) {
            document.body.classList.remove(pageClasses[pc]);
        }

        // 1.6 Reset layout inline styles that may have been set by excluded pages
        // (e.g. POS or kiosk mode pages set margin-left:0, display:none, etc.)
        var sidebar = document.getElementById('sidebar');
        if (sidebar) {
            sidebar.style.transform = '';
            sidebar.style.display = '';
        }
        var mainContentEl = document.querySelector('.main-content');
        if (mainContentEl) mainContentEl.style.marginLeft = '';
        var mainWrapper = document.querySelector('.main-content-wrapper');
        if (mainWrapper) mainWrapper.style.marginLeft = '';
        var adSidebar = document.getElementById('adSidebar');
        if (adSidebar) adSidebar.style.display = 'none';
        document.body.classList.remove('has-ad-sidebar');
        // Also hide mobile banner ad — will be re-evaluated after scripts
        var mobileBanner = document.getElementById('mobileBannerAd');
        if (mobileBanner) mobileBanner.style.display = 'none';
        document.body.classList.remove('has-mobile-banner-ad');
        var cmsFooter = document.querySelector('.cms-footer');
        if (cmsFooter) cmsFooter.style.marginLeft = '';

        // 2. Snapshot body class & data before swap (P2-4)
        this._snapshotBody();

        // 3. Parse fetched HTML
        var parser = new DOMParser();
        var doc = parser.parseFromString(html, 'text/html');

        // 4. Extract .main-content from response
        var newMainContent = doc.querySelector('.main-content');
        if (!newMainContent) {
            console.warn('[Router] .main-content not found in response, falling back');
            window.location.href = url;
            return;
        }

        // 5. Update page title
        var newTitle = doc.querySelector('title');
        if (newTitle) document.title = newTitle.textContent;

        // 6. Update page-name meta tag
        var newMeta = doc.querySelector('meta[name="page-name"]');
        var oldMeta = document.querySelector('meta[name="page-name"]');
        if (newMeta) {
            if (oldMeta) {
                oldMeta.setAttribute('content', newMeta.getAttribute('content'));
            } else {
                var m = document.createElement('meta');
                m.name = 'page-name';
                m.content = newMeta.getAttribute('content');
                document.head.appendChild(m);
            }
        } else if (oldMeta) {
            oldMeta.remove();
        }

        // 7. Replace .main-content
        var currentMain = document.querySelector('.main-content');
        if (!currentMain) {
            window.location.href = url;
            return;
        }

        var alertHtml = '<div id="alertContainer" class="position-fixed start-50 translate-middle-x" ' +
            'style="z-index: 9999; max-width: 500px; top: 60px; margin-top: 10px;"></div>';

        // Build new inner HTML, skipping old alertContainer
        var newInner = '';
        var children = newMainContent.childNodes;
        for (var i = 0; i < children.length; i++) {
            var child = children[i];
            if (child.nodeType === 1 && child.id === 'alertContainer') continue;
            if (child.nodeType === 1) {
                newInner += child.outerHTML;
            } else if (child.nodeType === 3 && child.textContent.trim()) {
                newInner += child.textContent;
            }
        }

        currentMain.innerHTML = alertHtml + newInner;

        // 8. Inject <link> stylesheets from the fetched page content into <head>
        this._injectStylesheets(currentMain);

        // 9. Install monkey-patches before executing page scripts (P2-1,2,3)
        this._installPatches();

        // 10. Execute scripts (async — waits for external scripts to load)
        var self = this;
        this._executeScripts(currentMain).then(function() {
            // 11. Uninstall monkey-patches — restore native functions
            self._uninstallPatches();

            // 12. Update sidemenu active state
            self._updateSidemenuActive(url);

            // 13. Re-apply i18n
            if (typeof I18n !== 'undefined' && I18n.updatePage) {
                try {
                    if (I18n.currentLang && I18n.currentLang !== 'zh') {
                        document.body.classList.add('i18n-loading');
                    }
                    requestAnimationFrame(function() {
                        try {
                            I18n.updatePage();
                        } catch (e) {
                            console.warn('[Router] i18n updatePage error:', e);
                            if (document.body) document.body.classList.remove('i18n-loading');
                        }
                    });
                } catch (e) {
                    console.warn('[Router] i18n update failed:', e);
                }
            }

            // 14. Re-init tooltips
            if (typeof App !== 'undefined' && App.initTooltips) {
                setTimeout(function() { App.initTooltips(); }, 300);
            }

            // 14.5 Re-evaluate ad sidebar / mobile banner for the new page
            if (typeof window.initAdSidebar === 'function') {
                try { window.initAdSidebar(); } catch (e) {
                    console.warn('[Router] initAdSidebar error:', e);
                }
            }
            if (typeof window.initMobileBannerAd === 'function') {
                try { window.initMobileBannerAd(); } catch (e) {
                    console.warn('[Router] initMobileBannerAd error:', e);
                }
            }

            // 15. Scroll
            if (restoreScroll) {
                var parsedUrl = new URL(url, window.location.origin);
                var scrollKey = parsedUrl.pathname + parsedUrl.search;
                var savedPos = self._scrollPositions[scrollKey];
                if (savedPos != null) {
                    var w = document.querySelector('.main-content-wrapper');
                    if (w) { w.scrollTop = savedPos; } else { window.scrollTo(0, savedPos); }
                }
            } else if (scrollTop) {
                var w = document.querySelector('.main-content-wrapper');
                if (w) w.scrollTop = 0;
                window.scrollTo(0, 0);
            }

            // 16. Dispatch event
            window.dispatchEvent(new CustomEvent('spa:navigation', { detail: { url: url } }));
        });
    },

    // ─── Phase 2: Snapshot body class & data attributes ─────────
    _snapshotBody: function() {
        this._bodyClassSnapshot = document.body.className;
        this._bodyDataSnapshot = {};
        var attrs = document.body.attributes;
        for (var i = 0; i < attrs.length; i++) {
            if (attrs[i].name.startsWith('data-')) {
                this._bodyDataSnapshot[attrs[i].name] = attrs[i].value;
            }
        }
    },

    _restoreBody: function() {
        // Restore class
        document.body.className = this._bodyClassSnapshot;
        // Remove any data-* attributes added by the page
        var currentAttrs = Array.from(document.body.attributes);
        for (var i = 0; i < currentAttrs.length; i++) {
            var name = currentAttrs[i].name;
            if (name.startsWith('data-') && !(name in this._bodyDataSnapshot)) {
                document.body.removeAttribute(name);
            }
        }
        // Restore original data-* values
        for (var key in this._bodyDataSnapshot) {
            if (this._bodyDataSnapshot.hasOwnProperty(key)) {
                document.body.setAttribute(key, this._bodyDataSnapshot[key]);
            }
        }
    },

    // ─── Phase 2: Install monkey-patches before script execution ──
    _installPatches: function() {
        var self = this;

        // Reset tracking arrays for new page
        this._trackedIntervals = [];
        this._trackedTimeouts = [];
        this._trackedListeners = [];

        // Save original native functions BEFORE patching
        this._origDocAddEventListener = document.addEventListener;
        this._origWindowAddEventListener = window.addEventListener;
        this._origSetInterval = window.setInterval;
        this._origSetTimeout = window.setTimeout;

        // --- P2-1: Patch DOMContentLoaded ---
        // Since we're in SPA mode, document.readyState is already 'complete'.
        // Page scripts that do addEventListener('DOMContentLoaded', fn) would
        // never fire. We intercept and invoke immediately.
        var savedDocAEL = this._origDocAddEventListener;
        document.addEventListener = function(type, fn, options) {
            if (type === 'DOMContentLoaded') {
                // Execute immediately since DOM is already loaded
                try { fn(); } catch (e) {
                    console.warn('[Router] DOMContentLoaded callback error:', e);
                }
                return;
            }
            // Track the listener for cleanup (P2-3)
            self._trackedListeners.push({ target: document, type: type, fn: fn, options: options });
            return savedDocAEL.call(document, type, fn, options);
        };

        // --- P2-2: Patch setInterval / setTimeout ---
        var savedSetInterval = this._origSetInterval;
        var savedSetTimeout = this._origSetTimeout;

        window.setInterval = function() {
            var id = savedSetInterval.apply(window, arguments);
            self._trackedIntervals.push(id);
            return id;
        };
        window.setTimeout = function() {
            var id = savedSetTimeout.apply(window, arguments);
            self._trackedTimeouts.push(id);
            return id;
        };

        // --- P2-3: Patch window.addEventListener ---
        var savedWinAEL = this._origWindowAddEventListener;
        window.addEventListener = function(type, fn, options) {
            self._trackedListeners.push({ target: window, type: type, fn: fn, options: options });
            return savedWinAEL.call(window, type, fn, options);
        };
    },

    // ─── Phase 2: Uninstall monkey-patches after script execution ─
    _uninstallPatches: function() {
        // Restore all native functions from saved references
        if (this._origDocAddEventListener) {
            document.addEventListener = this._origDocAddEventListener;
        }
        if (this._origSetInterval) {
            window.setInterval = this._origSetInterval;
        }
        if (this._origSetTimeout) {
            window.setTimeout = this._origSetTimeout;
        }
        if (this._origWindowAddEventListener) {
            window.addEventListener = this._origWindowAddEventListener;
        }
        this._origDocAddEventListener = null;
        this._origSetInterval = null;
        this._origSetTimeout = null;
        this._origWindowAddEventListener = null;
    },

    // ─── Inject <link> stylesheets from SPA-fetched content ────────
    // Scans the swapped content for <link rel="stylesheet"> tags and injects
    // them into <head> if not already present (prevents duplicate loads).
    _injectStylesheets: function(container) {
        var links = container.querySelectorAll('link[rel="stylesheet"]');
        for (var i = 0; i < links.length; i++) {
            var href = links[i].getAttribute('href');
            if (!href) continue;
            // Check if this stylesheet is already in <head>
            var existing = document.querySelector(
                'head link[href="' + href + '"]'
            );
            if (existing) {
                links[i].remove();
                continue;
            }
            // Move the <link> from content to <head>
            var newLink = document.createElement('link');
            newLink.rel = 'stylesheet';
            newLink.href = href;
            newLink.setAttribute('data-spa-injected', 'true');
            // Copy other attributes (e.g. media, crossorigin)
            var attrs = links[i].attributes;
            for (var a = 0; a < attrs.length; a++) {
                if (attrs[a].name !== 'rel' && attrs[a].name !== 'href') {
                    newLink.setAttribute(attrs[a].name, attrs[a].value);
                }
            }
            document.head.appendChild(newLink);
            links[i].remove();
        }
    },

    // ─── Execute inline <script> tags ─────────────────────────────
    // Returns a Promise that resolves when all scripts (including async external ones) have loaded
    _executeScripts: function(container) {
        var scripts = Array.from(container.querySelectorAll('script'));
        var chain = Promise.resolve();

        for (var i = 0; i < scripts.length; i++) {
            (function(oldScript) {
                chain = chain.then(function() {
                    var newScript = document.createElement('script');

                    // Copy attributes
                    var attrs = Array.from(oldScript.attributes);
                    for (var a = 0; a < attrs.length; a++) {
                        newScript.setAttribute(attrs[a].name, attrs[a].value);
                    }

                    if (oldScript.src) {
                        // External script — skip if already loaded globally (outside this container)
                        var relSrc = oldScript.getAttribute('src');
                        var existingAll = document.querySelectorAll(
                            'script[src="' + relSrc + '"], script[src="' + oldScript.src + '"]'
                        );
                        // Check if any match is NOT the oldScript itself (i.e. truly loaded elsewhere)
                        var alreadyLoaded = false;
                        for (var e = 0; e < existingAll.length; e++) {
                            if (existingAll[e] !== oldScript) {
                                alreadyLoaded = true;
                                break;
                            }
                        }
                        if (alreadyLoaded) {
                            oldScript.remove();
                            return Promise.resolve();
                        }
                        // Load external script and WAIT for it before continuing
                        return new Promise(function(resolve) {
                            newScript.src = relSrc;
                            newScript.onload = function() {
                                resolve();
                            };
                            newScript.onerror = function() {
                                console.warn('[Router] Failed to load script:', relSrc);
                                resolve(); // Continue even on error
                            };
                            if (oldScript.parentNode) {
                                oldScript.parentNode.replaceChild(newScript, oldScript);
                            } else {
                                document.head.appendChild(newScript);
                            }
                        });
                    } else {
                        // Inline script execution for SPA navigation.
                        //
                        // Problem: When a page is first loaded as a full page load, the browser
                        // executes <script> in the global lexical environment. Any `let`/`const`
                        // declarations create global lexical bindings that CANNOT be redeclared
                        // (not even with `var`). On subsequent SPA navigation to the same page,
                        // re-executing the script via DOM insertion would fail with:
                        //   "SyntaxError: Identifier 'X' has already been declared"
                        //
                        // Solution:
                        // 1. Replace let/const with var (var allows redeclaration in global scope)
                        // 2. Use indirect eval `(0, eval)(code)` instead of DOM insertion.
                        //    Indirect eval executes in global scope (same as <script>), but
                        //    errors can be caught with try/catch (unlike DOM script insertion
                        //    where SyntaxError is uncatchable).
                        // 3. If eval still fails (e.g. conflicting global lexical binding from
                        //    a previous full-page load), fall back to a full page reload to
                        //    get a clean global lexical environment.
                        var scriptText = oldScript.textContent
                            .replace(/\blet\s+/g, 'var ')
                            .replace(/\bconst\s+/g, 'var ');

                        // Remove the inert old <script> tag so it doesn't accumulate
                        if (oldScript.parentNode) {
                            oldScript.remove();
                        }

                        try {
                            // Indirect eval — executes in global scope
                            (0, eval)(scriptText);
                        } catch (evalErr) {
                            // If eval fails due to a global lexical binding conflict
                            // (e.g. a previous full-page load declared `const X` which
                            // cannot be redeclared even as `var X`), the only reliable
                            // fix is a full page reload to get a clean global environment.
                            console.warn('[Router] Inline script eval failed:', evalErr.message,
                                '— falling back to full page reload');
                            window.location.reload();
                            return Promise.resolve();
                        }
                        return Promise.resolve();
                    }
                });
            })(scripts[i]);
        }

        return chain;
    },

    // ─── Update Sidemenu Active State ─────────────────────────────
    _updateSidemenuActive: function(url) {
        try {
            var parsed = new URL(url, window.location.origin);
            var currentPath = parsed.pathname;
            var normalizedPath = currentPath === '/' ? '/' : currentPath.replace(/\/+$/, '');
            var queryParams = new URLSearchParams(parsed.search || '');
            var sourceType = (queryParams.get('source_type') || '').toLowerCase();
            var serviceTypeCode = (queryParams.get('service_type_code') || '').toLowerCase();

            var sidebar = document.getElementById('sidebar');
            if (!sidebar) return;

            // Remove all active classes
            sidebar.querySelectorAll('.nav-link.active').forEach(function(el) {
                el.classList.remove('active');
            });

            // NOTE: We no longer blindly collapse ALL open submenus here.
            // Instead, after finding the matched nav item, we only collapse
            // submenus that are NOT the parent of the new active item.
            // This prevents the flash of submenu closing then re-opening
            // when navigating within the same submenu group.

            // Navigation item mapping
            var navItems = {
                '/dashboard': 'nav-dashboard',
                '/business-goals': 'nav-business-goals',
                '/customers': 'nav-customers',
                '/customer-labels': 'nav-customer-labels',
                '/customer-analysis-report': 'nav-customer-analysis-report',
                '/project-analysis-report': 'nav-project-analysis-report',
                '/suppliers': 'nav-suppliers',
                '/warehouses': 'nav-warehouses',
                '/products': 'nav-products',
                '/product-taxes': 'nav-product-taxes',
                '/orders': 'nav-orders',
                '/invoices': 'nav-invoices',
                '/order-reports': 'nav-order-reports',
                '/member-levels': 'nav-member-levels',
                '/points': 'nav-points',
                '/points-history': 'nav-points-history',
                '/point-settings': 'nav-point-settings',
                '/stamp-settings': 'nav-stamp-settings',
                '/stamp-records': 'nav-stamp-records',
                '/referrals': 'nav-referrals',
                '/coupons': 'nav-coupons',
                '/product-types': 'nav-product-types',
                '/product-attributes': 'nav-product-attributes',
                '/brands': 'nav-brands',
                '/product-labels': 'nav-product-labels',
                '/service-types': 'nav-service-types',
                '/services': 'nav-services',
                '/service-taxes': 'nav-service-taxes',
                '/appointments': 'nav-appointments',
                '/service-orders': 'nav-service-orders',
                '/service-staffs': 'nav-service-staffs',
                '/rooms': 'nav-rooms',
                '/equipments': 'nav-equipments',
                '/vehicles': 'nav-vehicles',
                '/resource-usage-calendar': 'nav-resource-usage-calendar',
                '/rental-orders': 'nav-rental-orders',
                '/rental-order-labels': 'nav-rental-order-labels',
                '/projects': 'nav-projects',
                '/project-types': 'nav-project-types',
                '/pages': 'nav-pages',
                '/blogs': 'nav-blogs',
                '/website-theme': 'nav-website-theme',
                '/website-settings': 'nav-website-settings',
                '/website-domains': 'nav-website-domains',
                '/website-page-views': 'nav-website-page-views',
                '/personal-data': 'nav-personal-data',
                '/pos-settings': 'nav-pos-settings',
                '/notification-settings': 'nav-notification-settings',
                '/calendars': 'nav-calendars',
                '/reminders': 'nav-reminders',
                '/notifications': 'nav-notifications',
                '/messages': 'nav-messages',
                '/notes': 'nav-notes',
                '/users': 'nav-users',
                '/enterprises': 'nav-enterprises',
                '/google-ads': 'nav-google-ads',
                '/departments': 'nav-departments',
                '/roles': 'nav-roles',
                '/levels': 'nav-levels',
                '/regions': 'nav-regions',
                '/currencies': 'nav-currencies',
                '/purchase-orders': 'nav-purchase-orders',
                '/quotations': 'nav-quotations',
                '/support-communications': 'nav-support-communications',
                '/promotions': 'nav-promotions',
                '/lead-finder': 'nav-lead-finder',
                '/lead-finder/results': 'nav-lead-list',
                '/auto-outreach': 'nav-auto-outreach',
                '/ad-positions': 'nav-ad-positions',
                '/pos': 'nav-pos',
                '/order-labels': 'nav-order-labels',
                '/service-order-labels': 'nav-service-order-labels',
                '/purchase-order-labels': 'nav-purchase-order-labels',
                '/payment-methods': 'nav-payment-methods',
                '/stripe-connect': 'nav-stripe-connect',
                '/shipping-methods': 'nav-shipping-methods',
                '/logistics-companies': 'nav-logistics-companies',
                '/shipments': 'nav-shipments',
                '/shipping-integrations': 'nav-shipping-integrations',
                '/document-settings': 'nav-document-settings',
                '/document-auto-settings': 'nav-document-auto-settings',
                '/product-sync-settings': 'nav-product-sync-settings',
                '/accounting': 'nav-accounting',
                '/accounting/reports': 'nav-accounting-reports',
                '/accounts': 'nav-accounts',
                '/journal-entries': 'nav-journal-entries',
                '/tax-configs': 'nav-tax-configs',
                '/posting-rules': 'nav-posting-rules',
                '/accounting/posting-rules': 'nav-posting-rules',
                '/incomes': 'nav-incomes',
                '/expenses': 'nav-expenses',
                '/expense-requests': 'nav-expense-requests',
                '/bank-accounts': 'nav-bank-accounts',
                '/inventory-movements': 'nav-inventory-movements',
                '/inventory-processing': 'nav-inventory-processing',
                '/inventory-adjustments': 'nav-inventory-adjustments',
                '/inventory-counts': 'nav-inventory-counts',
                '/inventory/low-stock': 'nav-low-stock',
                '/inventory-settings': 'nav-inventory-settings',
                '/attendance/clock': 'nav-attendance-clock',
                '/attendances': 'nav-attendances',
                '/attendance-reports': 'nav-attendance-reports',
                '/leave-requests': 'nav-leave-requests',
                '/holidays': 'nav-holidays',
                '/shifts': 'nav-shifts',
                '/payrolls': 'nav-payrolls',
                '/payroll-adjustment-presets': 'nav-payroll-adjustment-presets',
                '/job-vacancies': 'nav-job-vacancies',
                '/job-applicants': 'nav-job-applicants',
                '/job-hires': 'nav-job-hires',
                '/phone-country-codes': 'nav-phone-country-codes',
                '/stores': 'nav-stores',
                '/dining-areas': 'nav-dining-areas',
                '/dining-tables/board': 'nav-dining-tables-board',
                '/dining-tables': 'nav-dining-tables',
                '/dining-queue/board': 'nav-dining-queue-board',
                '/dining-queues': 'nav-dining-queues',
                '/delivery-integrations': 'nav-delivery-integrations',
                '/delivery-orders': 'nav-delivery-orders',
                '/dining-settings': 'nav-dining-settings',
                '/dining-ticket-settings': 'nav-dining-ticket-settings',
                '/diningorder-ticket-settings': 'nav-diningorder-ticket-settings',
                '/guide/dining-flow': 'nav-guide-dining-flow',
                '/guide/order-flow': 'nav-guide-order-flow',
                '/printer-settings': 'nav-printer-settings',
                '/pos-ticket-settings': 'nav-pos-ticket-settings',
                '/card-terminal-settings': 'nav-card-terminal-settings',
                '/api-tokens': 'nav-api-tokens',
                '/billing': 'nav-billing',
                '/vcoins': 'nav-ai-coins',
                '/hardware-purchase': 'nav-hardware-purchase'
            };

            var matchedId = null;
            var matchedPathLength = -1;

            if (normalizedPath === '/orders' && sourceType === 'dining') {
                matchedId = 'nav-dining-orders';
                matchedPathLength = normalizedPath.length;
            } else if (serviceTypeCode === 'course') {
                var courseNavMap = {
                    '/service-orders': 'nav-course-orders',
                    '/service-staffs': 'nav-course-instructors',
                    '/appointment-calendar': 'nav-course-calendar',
                    '/appointments': 'nav-course-appointments'
                };
                if (courseNavMap[normalizedPath]) {
                    matchedId = courseNavMap[normalizedPath];
                    matchedPathLength = normalizedPath.length;
                }
            }

            if (!matchedId) {
                if (navItems[normalizedPath]) {
                    matchedId = navItems[normalizedPath];
                    matchedPathLength = normalizedPath.length;
                } else {
                    var paths = Object.keys(navItems).sort(function(a, b) { return b.length - a.length; });
                    for (var i = 0; i < paths.length; i++) {
                        var path = paths[i];
                        if (normalizedPath.startsWith(path + '/')) {
                            matchedPathLength = path.length;
                            matchedId = navItems[path];
                            break;
                        }
                    }
                }
            }

            if (matchedId) {
                var navItem = document.getElementById(matchedId);
                if (navItem) {
                    navItem.classList.add('active');
                    var targetSubmenu = navItem.closest('.collapse');

                    // Collapse all open submenus EXCEPT the one containing the new active item
                    sidebar.querySelectorAll('.collapse.show').forEach(function(el) {
                        if (targetSubmenu && el.id === targetSubmenu.id) return; // skip — this one stays open
                        var toggle = document.querySelector('[data-bs-target="#' + el.id + '"]');
                        if (toggle) toggle.setAttribute('aria-expanded', 'false');
                        if (typeof bootstrap !== 'undefined') {
                            var bsc = bootstrap.Collapse.getInstance(el);
                            if (bsc) bsc.hide();
                        }
                    });

                    // Expand the target submenu if not already open
                    if (targetSubmenu && typeof bootstrap !== 'undefined') {
                        var bsCollapse = bootstrap.Collapse.getOrCreateInstance(targetSubmenu, { toggle: false });
                        bsCollapse.show();
                        var toggle = document.querySelector('[data-bs-target="#' + targetSubmenu.id + '"]');
                        if (toggle) toggle.setAttribute('aria-expanded', 'true');
                    }
                }
            } else {
                // No matched nav item — collapse all open submenus
                sidebar.querySelectorAll('.collapse.show').forEach(function(el) {
                    var toggle = document.querySelector('[data-bs-target="#' + el.id + '"]');
                    if (toggle) toggle.setAttribute('aria-expanded', 'false');
                    if (typeof bootstrap !== 'undefined') {
                        var bsc = bootstrap.Collapse.getInstance(el);
                        if (bsc) bsc.hide();
                    }
                });
            }

            // Auto-scroll sidebar to active item
            setTimeout(function() {
                try {
                    if (!sidebar) return;
                    var activeLink = sidebar.querySelector('.nav-link.active');
                    var target = null;
                    if (activeLink) {
                        var collapse = activeLink.closest('.collapse');
                        if (collapse && collapse.id) {
                            target = sidebar.querySelector('[data-bs-target="#' + collapse.id + '"]');
                        }
                    }
                    if (!target) target = sidebar.querySelector('.nav-group-toggle[aria-expanded="true"]');
                    if (!target) target = activeLink;
                    if (target && typeof target.scrollIntoView === 'function') {
                        target.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'nearest' });
                    }
                } catch (e) {}
            }, 300);
        } catch (e) {
            console.warn('[Router] Failed to update sidemenu:', e);
        }
    },

    // ─── Page Cleanup ─────────────────────────────────────────────
    _runCleanup: function() {
        // Run registered cleanup functions
        while (this._cleanupFns.length > 0) {
            try { this._cleanupFns.pop()(); } catch (e) {
                console.warn('[Router] Cleanup error:', e);
            }
        }

        // ── P2-2: Clear all tracked intervals & timeouts ──
        var i;
        for (i = 0; i < this._trackedIntervals.length; i++) {
            try { clearInterval(this._trackedIntervals[i]); } catch (e) {}
        }
        this._trackedIntervals = [];

        for (i = 0; i < this._trackedTimeouts.length; i++) {
            try { clearTimeout(this._trackedTimeouts[i]); } catch (e) {}
        }
        this._trackedTimeouts = [];

        // ── P2-3: Remove all tracked event listeners ──
        for (i = 0; i < this._trackedListeners.length; i++) {
            var entry = this._trackedListeners[i];
            try {
                // Use the native removeEventListener (via prototype) to ensure it works
                // even if our patch is still somehow active
                EventTarget.prototype.removeEventListener.call(
                    entry.target, entry.type, entry.fn, entry.options
                );
            } catch (e) {}
        }
        this._trackedListeners = [];

        // ── P2-4: Restore body class & data attributes ──
        // Only restore if snapshot was taken (check if _snapshotBody was called)
        if (this._bodyClassSnapshot !== null) {
            this._restoreBody();
        }

        // Destroy DynamicList
        if (window.dynamicList) {
            try {
                var lc = document.getElementById('dynamicListContainer');
                if (lc && typeof $ !== 'undefined' && $.fn.select2) {
                    $(lc).find('select.select2-hidden-accessible').each(function() {
                        try { $(this).select2('destroy'); } catch (e) {}
                    });
                }
            } catch (e) {}
            window.dynamicList = null;
        }

        // Destroy DynamicForm
        if (window.dynamicForm) {
            try {
                if (window.dynamicForm.saveTimer) clearTimeout(window.dynamicForm.saveTimer);
                // Clear fire-and-forget field settings timeouts (not tracked by Router patches
                // because they are created after _uninstallPatches restores native setTimeout)
                if (window.dynamicForm._fieldSettingsTimeoutIds) {
                    for (var t = 0; t < window.dynamicForm._fieldSettingsTimeoutIds.length; t++) {
                        try { clearTimeout(window.dynamicForm._fieldSettingsTimeoutIds[t]); } catch (e) {}
                    }
                    window.dynamicForm._fieldSettingsTimeoutIds = [];
                }
                var fc = document.getElementById('dynamicFormContainer');
                if (fc && typeof $ !== 'undefined' && $.fn.select2) {
                    $(fc).find('select.select2-hidden-accessible').each(function() {
                        try { $(this).select2('destroy'); } catch (e) {}
                    });
                }
            } catch (e) {}
            window.dynamicForm = null;
        }

        // Remove fixed form-button-bar (appended to document.body by setupFormButtonBar).
        // It will be re-created by MutationObserver + setupFormButtonBar if the new page has forms.
        var formButtonBar = document.querySelector('.form-button-bar');
        if (formButtonBar) {
            formButtonBar.remove();
        }

        // ── P2-5: Destroy FullCalendar instances ──
        var calendarNames = ['calendar', 'apptCalendar', 'resourceCalendar'];
        for (i = 0; i < calendarNames.length; i++) {
            if (window[calendarNames[i]]) {
                try { window[calendarNames[i]].destroy(); } catch (e) {}
                window[calendarNames[i]] = null;
            }
        }
        // Clean calendar filter globals
        try { delete window.calendarFilter; } catch (e) {}
        try { delete window.apptCalFilters; } catch (e) {}
        try { delete window.resourceUsageCategoryFilter; } catch (e) {}
        try { delete window.resourceUsageTypeFilter; } catch (e) {}

        // Destroy Chart.js instances (check window globals + Chart.js registry)
        var chartNames = ['monthlyChart', 'accountingChart'];
        for (var c = 0; c < chartNames.length; c++) {
            // Try via window global first
            if (window[chartNames[c]]) {
                try { window[chartNames[c]].destroy(); } catch (e) {}
                window[chartNames[c]] = null;
            }
            // Fallback: use Chart.js registry by canvas ID
            var cvs = document.getElementById(chartNames[c]);
            if (cvs && typeof Chart !== 'undefined' && Chart.getChart) {
                var inst = Chart.getChart(cvs);
                if (inst) {
                    try { inst.destroy(); } catch (e) {}
                }
            }
        }
        // Clean business-goals page globals
        try { delete window.BGoals; } catch (e) {}

        // Clean dashboard/accounting globals
        var dashGlobals = [
            '__lastMonthlyChartData', '__lastAccountingChartData',
            '__lastPlanStats', '__planClickBound',
            'dashboardLoadingStart'
        ];
        for (var d = 0; d < dashGlobals.length; d++) {
            try { delete window[dashGlobals[d]]; } catch (e) {}
        }

        // ── P2-5: Destroy billing/subscription globals ──
        var billingGlobals = ['subscribePlan', 'cancelSubscription'];
        for (i = 0; i < billingGlobals.length; i++) {
            try { delete window[billingGlobals[i]]; } catch (e) {}
        }

        // ── P2-5: Clean messages page globals ──
        var messagesGlobals = [
            'messageConversations', 'messageCurrentConversation',
            'messagePollingInterval', 'messageUnreadCount',
            'messageSendMessage', 'messageMarkAsRead',
            'messageDeleteConversation', 'messageSearchUsers',
            'messageLoadConversation', 'messageLoadConversations',
            'messageInit'
        ];
        for (i = 0; i < messagesGlobals.length; i++) {
            try { delete window[messagesGlobals[i]]; } catch (e) {}
        }

        // ── P2-5: Clean delivery_orders/notifications polling globals ──
        var pollingGlobals = [
            'loadOrders', 'loadNotifications', 'loadStatus',
            'deliveryOrders', 'notificationsList'
        ];
        for (i = 0; i < pollingGlobals.length; i++) {
            try { delete window[pollingGlobals[i]]; } catch (e) {}
        }

        // Destroy all Select2 in .main-content
        try {
            var mc = document.querySelector('.main-content');
            if (mc && typeof $ !== 'undefined' && $.fn.select2) {
                $(mc).find('select.select2-hidden-accessible').each(function() {
                    try { $(this).select2('destroy'); } catch (e) {}
                });
            }
        } catch (e) {}

        // Destroy Bootstrap tooltips (entire document, not just .main-content)
        try {
            if (typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
                // Dispose all tooltip instances on any element with data-bs-toggle="tooltip"
                document.querySelectorAll('[data-bs-toggle="tooltip"]').forEach(function(el) {
                    var tip = bootstrap.Tooltip.getInstance(el);
                    if (tip) try { tip.dispose(); } catch (e) {}
                });
                // Remove any orphaned tooltip DOM elements left in <body>
                document.querySelectorAll('.tooltip.bs-tooltip-auto, .tooltip.bs-tooltip-top, .tooltip.bs-tooltip-bottom, .tooltip.bs-tooltip-start, .tooltip.bs-tooltip-end, .tooltip.show').forEach(function(el) {
                    el.remove();
                });
            }
        } catch (e) {}

        // Destroy Bootstrap modals in .main-content
        try {
            var mc3 = document.querySelector('.main-content');
            if (mc3 && typeof bootstrap !== 'undefined' && bootstrap.Modal) {
                mc3.querySelectorAll('.modal').forEach(function(el) {
                    var modal = bootstrap.Modal.getInstance(el);
                    if (modal) try { modal.dispose(); } catch (e) {}
                });
            }
            document.querySelectorAll('.modal-backdrop').forEach(function(el) { el.remove(); });
            document.body.classList.remove('modal-open');
            document.body.style.removeProperty('overflow');
            document.body.style.removeProperty('padding-right');
        } catch (e) {}

        // Remove loading overlay
        try {
            var ol = document.getElementById('appLoadingOverlay');
            if (ol) ol.remove();
        } catch (e) {}

        // Remove any dynamically injected <script> tags in <head> from page scripts
        try {
            document.querySelectorAll('head script[data-spa-injected]').forEach(function(el) {
                el.remove();
            });
        } catch (e) {}
    },

    // ─── Register cleanup function for current page ───────────────
    onCleanup: function(fn) {
        if (typeof fn === 'function') {
            this._cleanupFns.push(fn);
        }
    },

    // ─── PopState Handler (back/forward) ──────────────────────────
    _onPopState: function(e) {
        var url = window.location.pathname + window.location.search + window.location.hash;
        if (!this._shouldIntercept(window.location.href)) {
            window.location.reload();
            return;
        }
        this.navigate(url, { pushState: false, scrollTop: false, restoreScroll: true });
    },

    // ─── Utility: Drop-in for window.location.href ────────────────
    go: function(url) {
        try {
            if (this._shouldIntercept(new URL(url, window.location.origin).href)) {
                this.navigate(url);
            } else {
                window.location.href = url;
            }
        } catch (e) {
            window.location.href = url;
        }
    },

    // ─── Utility: Drop-in for window.location.replace() ──────────
    replace: function(url) {
        try {
            if (this._shouldIntercept(new URL(url, window.location.origin).href)) {
                this.navigate(url, { replace: true });
            } else {
                window.location.replace(url);
            }
        } catch (e) {
            window.location.replace(url);
        }
    }
};

// Auto-initialize on CMS pages only
(function() {
    var path = window.location.pathname;
    var publicPages = ['/', '/login', '/reset-password', '/subscription-required', '/subscribe-now', '/pos', '/accept-invite'];
    var publicPrefixes = ['/help', '/contact', '/co/', '/static/', '/sales-partner', '/tutorial'];

    var isPublic = publicPages.indexOf(path) !== -1;
    if (!isPublic) {
        for (var i = 0; i < publicPrefixes.length; i++) {
            if (path.startsWith(publicPrefixes[i])) {
                isPublic = true;
                break;
            }
        }
    }

    if (!isPublic) {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', function() { Router.init(); });
        } else {
            Router.init();
        }
    }
})();
