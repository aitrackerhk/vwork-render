// vWork 國際化支持

// localStorage 在某些環境（隱私模式/限制第三方 cookie/iframe）可能直接 throw
// 用安全包裝避免 i18n.init() 中斷造成整頁「不出字/卡住」
const SafeStorage = {
    get: function(key) {
        try { return localStorage.getItem(key); } catch (e) { return null; }
    },
    set: function(key, value) {
        try { localStorage.setItem(key, value); } catch (e) { /* ignore */ }
    }
};

const I18n = {
    currentLang: 'zh',
    translations: {},
    _ready: false,
    _readyPromise: null,
    _readyResolve: null,
    _initStarted: false,
    _unlockTimer: null,
    // 未 ready 時呼叫 t(key, el) 所記錄的 pending patches
    // 格式：[{ el, key, attr }]（attr: null=textContent, 'placeholder', 'title'）
    _pendingPatches: [],

    // Ensure _readyPromise exists before init() is called,
    // so that whenReady() called early (e.g. from DynamicList)
    // actually waits instead of resolving immediately via
    // the `|| Promise.resolve()` fallback.
    _ensureReadyPromise: function() {
        if (!this._readyPromise) {
            this._readyPromise = new Promise((resolve) => { this._readyResolve = resolve; });
        }
    },

    // 初始化
    init: function() {
        // 避免被模板/其他腳本重複呼叫造成多次載入與狀態互相覆寫
        if (this._initStarted) return;
        this._initStarted = true;

        // 初始化 ready promise（讓任何地方都能 await i18n 載入完成，避免 popup 先顯示預設中文再跳英文）
        this._ensureReadyPromise();
        this._ready = false;

        // 保底解鎖：即使 i18n 載入卡住（fetch 無限等待/例外中斷），也不要讓整頁一直被 i18n-loading 隱藏
        // 多數模板也有 5 秒解鎖，但這裡做成全域保護，避免漏網頁面
        try {
            if (this._unlockTimer) clearTimeout(this._unlockTimer);
            this._unlockTimer = setTimeout(() => {
                try {
                    if (document.body && document.body.classList.contains('i18n-loading')) {
                        console.warn('i18n: unlock timeout reached, removing i18n-loading to prevent blank UI');
                        document.body.classList.remove('i18n-loading');
                    }
                } catch (e) { /* ignore */ }
            }, 5000);
        } catch (e) { /* ignore */ }

        // 從 localStorage 獲取語言，或根據瀏覽器語言自動選擇
        // 唯一 source of truth: u-nai_lang（不再讀寫歷史遺留的 localStorage.language）
        const savedLang = SafeStorage.get('u-nai_lang');
        const browserLang = navigator.language || navigator.userLanguage;
        
        // 前台（/co/...）可由模板注入租戶預設語言，避免依瀏覽器語言自動切換
        let publicDefaultLang = null;
        try {
            publicDefaultLang = (typeof window !== 'undefined' && window.__PUBLIC_DEFAULT_LANG) ? String(window.__PUBLIC_DEFAULT_LANG) : null;
        } catch (e) {
            publicDefaultLang = null;
        }
        const isSupportedLang = (l) => l === 'zh' || l === 'zh-CN' || l === 'en';

        // 模板可能注入 window.__INITIAL_LANG__（例如 CMS layout），用來指定本頁應採用的語言
        let initialLang = null;
        try {
            initialLang = (typeof window !== 'undefined' && window.__INITIAL_LANG__) ? String(window.__INITIAL_LANG__) : null;
        } catch (e) {
            initialLang = null;
        }
        
        const isPublicCompanySite = (typeof window !== 'undefined' && window.location && typeof window.location.pathname === 'string')
            ? window.location.pathname.startsWith('/co/')
            : false;

        // 清除歷史遺留的 localStorage.language，避免其他殘留邏輯讀到舊值
        try { localStorage.removeItem('language'); } catch (e) { /* ignore */ }

        // Public company site (/co/...): ALWAYS use tenant's configured language
        // regardless of localStorage (admin preference should NOT affect public pages)
        if (isPublicCompanySite && isSupportedLang(publicDefaultLang)) {
            this.currentLang = publicDefaultLang;
            // Do NOT overwrite savedLang — it remains the user's admin preference
        } else if (savedLang && isSupportedLang(savedLang)) {
            this.currentLang = savedLang;
        } else if (!isPublicCompanySite && isSupportedLang(initialLang)) {
            this.currentLang = initialLang;
            SafeStorage.set('u-nai_lang', initialLang);
        } else {
            // 檢測瀏覽器語言
            if (browserLang.startsWith('zh-CN') || browserLang.startsWith('zh-Hans')) {
                this.currentLang = 'zh-CN';
            } else if (browserLang.startsWith('zh')) {
                this.currentLang = 'zh';
            } else {
                this.currentLang = 'en';
            }
            SafeStorage.set('u-nai_lang', this.currentLang);
        }

        // 載入語言文件
        this.loadLanguage(this.currentLang);
        this.bindLangSwitcher();
    },

    // 等待語言檔載入完成（可用於 modal/popup：先翻譯再顯示，避免閃爍）
    whenReady: function(timeoutMs = 3000) {
        if (this._ready) return Promise.resolve();
        this._ensureReadyPromise();
        const p = this._readyPromise;
        if (!timeoutMs || timeoutMs <= 0) return p;
        return Promise.race([
            p,
            new Promise((resolve) => setTimeout(resolve, timeoutMs))
        ]);
    },

    // 綁定語言切換按鈕 / 下拉
    bindLangSwitcher: function() {
        document.querySelectorAll('[data-lang]').forEach(el => {
            el.addEventListener('click', (e) => {
                e.preventDefault();
                const lang = el.getAttribute('data-lang');
                if (lang) {
                    this.switchLanguage(lang);
                }
            });
        });
    },

    // 載入語言文件
    loadLanguage: async function(lang) {
        try {
            // 檢查是否為 vsys / vmarket / voffice / vai 頁面
            // Help 頁面根據產品載入對應翻譯檔：/help/vai → vai, /help/vmarket → vmarket, /help/voffice → voffice, /help → help_hub
            const pathname = window.location.pathname;
            const isHelpHubPage = pathname === '/help' || pathname === '/help/';
            const isHelpVaiPage = pathname.startsWith('/help/vai');
            const isHelpVmarketPage = pathname.startsWith('/help/vmarket');
            const isHelpVofficePage = pathname.startsWith('/help/voffice');

            const hostname = window.location.hostname;
            const isVmarketPage = isHelpVmarketPage ||
                                  (pathname === '/' && window.location.search.includes('domain=vmarket')) ||
                                  pathname.startsWith('/vmarket') ||
                                  hostname.includes('vmarketai.com');
            const isVofficePage = isHelpVofficePage ||
                                  (pathname === '/' && window.location.search.includes('domain=voffice')) ||
                                  pathname.startsWith('/voffice-') ||
                                  hostname.includes('voffice.vsysai.com');
            const isVaiPage = isHelpVaiPage ||
                              (pathname === '/' && window.location.search.includes('domain=vai')) ||
                              pathname.startsWith('/vai-') ||
                              hostname.includes('vai.vsysai.com');
            // isVsysPage must be checked AFTER more specific subdomains (vai, voffice, vmarket)
            // because 'vsysai.com' matches all subdomains like vai.vsysai.com, voffice.vsysai.com, etc.
            // Only match www.vsysai.com or bare vsysai.com (not subdomains)
            const isVsysPage = (pathname === '/' && window.location.search.includes('domain=vsys')) || 
                              (hostname === 'vsysai.com' || hostname === 'www.vsysai.com');
            
            // 將 zh-CN 映射到 zh-CN.json，zh 映射到 zh.json
            const langFile = lang === 'zh-CN' ? 'zh-CN' : (lang === 'zh' ? 'zh' : lang);
            let fileName;
            if (isHelpHubPage) {
                fileName = `help_hub_${langFile}.json`;
            } else if (isVaiPage) {
                fileName = `vai_${langFile}.json`;
            } else if (isVmarketPage) {
                fileName = `vmarket_${langFile}.json`;
            } else if (isVofficePage) {
                fileName = `voffice_${langFile}.json`;
            } else if (isVsysPage) {
                fileName = `vsys_${langFile}.json`;
            } else {
                fileName = `${langFile}.json`;
            }
            const url = `/static/locales/${fileName}`;

            // Determine if this is a product-specific file that needs base fallback
            const isProductFile = isVaiPage || isVmarketPage || isVofficePage || isVsysPage || isHelpHubPage;
            const baseFileName = `${langFile}.json`;
            const baseUrl = `/static/locales/${baseFileName}`;

            // fetch 超時保護（避免網路卡住 Promise 永遠 pending，導致 i18n 一直不 ready）
            // 不使用 cache: 'no-store'，讓瀏覽器可以利用 HTTP Cache-Control（後端設定 max-age=3600）
            // 這樣第二次及後續頁面載入可直接從本地 cache 讀取，大幅降低翻譯 load 不到的機率
            const fetchWithTimeout = async (u, timeoutMs = 4000) => {
                // AbortController 在舊瀏覽器可能不存在；不存在時仍可依賴全域解鎖 timer
                if (typeof AbortController === 'undefined') {
                    return await fetch(u);
                }
                const controller = new AbortController();
                const timer = setTimeout(() => {
                    try { controller.abort(); } catch (e) { /* ignore */ }
                }, timeoutMs);
                try {
                    return await fetch(u, { signal: controller.signal });
                } finally {
                    clearTimeout(timer);
                }
            };

            // JSON 解析超時保護（避免大文件或格式問題導致卡住）
            const parseJsonWithTimeout = async (resp, timeoutMs = 3000) => {
                const text = await resp.text();
                return new Promise((resolve, reject) => {
                    const timer = setTimeout(() => {
                        reject(new Error('JSON parse timeout'));
                    }, timeoutMs);
                    try {
                        const json = JSON.parse(text);
                        clearTimeout(timer);
                        resolve(json);
                    } catch (e) {
                        clearTimeout(timer);
                        reject(e);
                    }
                });
            };

            // Deep merge: base object is overridden by product-specific values
            const deepMerge = (base, override) => {
                const result = Object.assign({}, base);
                for (const key of Object.keys(override)) {
                    if (
                        result[key] && typeof result[key] === 'object' && !Array.isArray(result[key]) &&
                        override[key] && typeof override[key] === 'object' && !Array.isArray(override[key])
                    ) {
                        result[key] = deepMerge(result[key], override[key]);
                    } else {
                        result[key] = override[key];
                    }
                }
                return result;
            };

            let translations;

            if (isProductFile) {
                // Load base + common + product translations in parallel, merge them
                // Load order: base → common → product (each layer overrides previous)
                const commonFileName = `common_${langFile}.json`;
                const commonUrl = `/static/locales/${commonFileName}`;

                const [baseResp, commonResp, productResp] = await Promise.all([
                    fetchWithTimeout(baseUrl, 4000).catch(() => null),
                    fetchWithTimeout(commonUrl, 4000).catch(() => null),
                    fetchWithTimeout(url, 4000)
                ]);

                if (!productResp || !productResp.ok) {
                    throw new Error(`Failed to load ${fileName}: ${productResp ? productResp.status : 'network error'}`);
                }

                const productTranslations = await parseJsonWithTimeout(productResp, 3000);

                let baseTranslations = {};
                if (baseResp && baseResp.ok) {
                    try {
                        baseTranslations = await parseJsonWithTimeout(baseResp, 3000);
                    } catch (e) {
                        console.warn('i18n: Failed to parse base translations, using product-only:', e);
                    }
                }

                let commonTranslations = {};
                if (commonResp && commonResp.ok) {
                    try {
                        commonTranslations = await parseJsonWithTimeout(commonResp, 3000);
                    } catch (e) {
                        console.warn('i18n: Failed to parse common translations, skipping:', e);
                    }
                }

                // Merge: base → common → product (each layer overrides previous)
                translations = deepMerge(deepMerge(baseTranslations, commonTranslations), productTranslations);
            } else {
                const response = await fetchWithTimeout(url, 4000);
                if (!response.ok) {
                    throw new Error(`Failed to load ${langFile}.json: ${response.status}`);
                }
                translations = await parseJsonWithTimeout(response, 3000);
            }
            
            this.translations = translations;
            this.currentLang = lang;
            SafeStorage.set('u-nai_lang', lang);
            this._ready = true;
            if (typeof this._readyResolve === 'function') {
                try { this._readyResolve(); } catch (e) { /* ignore */ }
            }
            // 触发自定义事件，通知其他组件语言已切换
            if (typeof window !== 'undefined' && window.dispatchEvent) {
                window.dispatchEvent(new CustomEvent('languageChanged', { detail: { lang: lang } }));
            }
            // 補跑 pending patches：在翻譯 ready 前呼叫過 t(key, el) 的 element，自動補上翻譯
            this._flushPendingPatches();
            // 使用 requestAnimationFrame 异步更新页面，避免阻塞
            requestAnimationFrame(() => {
                try {
                    this.updatePage();
                } catch (e) {
                    console.error('i18n updatePage error:', e);
                    // 即使更新失败，也要移除 loading 类
                    if (document.body) {
                        document.body.classList.remove('i18n-loading');
                    }
                }
            });
        } catch (error) {
            console.error('Failed to load language file:', error);
            // 如果載入失敗，嘗試載入英文
            if (lang !== 'en') {
                await this.loadLanguage('en');
            } else {
                // 如果英文也載入失敗，至少移除 i18n-loading 類，顯示原始內容
                // 這樣即使沒有翻譯文件，用戶也能看到頁面內容
                if (document.body) {
                    document.body.classList.remove('i18n-loading');
                }
                // 即使載入失敗，也嘗試更新頁面（使用空的翻譯對象）
                this.translations = {};
                this._ready = true;
                if (typeof this._readyResolve === 'function') {
                    try { this._readyResolve(); } catch (e) { /* ignore */ }
                }
                // 使用 requestAnimationFrame 异步更新，避免阻塞
                requestAnimationFrame(() => {
                    try {
                        this.updatePage();
                    } catch (e) {
                        console.error('i18n fallback updatePage error:', e);
                    }
                });
            }
        } finally {
            // 不管成功/失敗，都確保不會永遠卡在 loading 狀態
            try {
                if (this._unlockTimer) {
                    clearTimeout(this._unlockTimer);
                    this._unlockTimer = null;
                }
                if (document.body && document.body.classList.contains('i18n-loading')) {
                    document.body.classList.remove('i18n-loading');
                }
            } catch (e) { /* ignore */ }
        }
    },

    // 切換語言
    switchLanguage: function(lang) {
        this.currentLang = lang;
        SafeStorage.set('u-nai_lang', lang);
        // 刷新頁面以確保所有內容都正確更新
        window.location.reload();
    },

    // 補跑 pending patches：翻譯 ready 後，自動更新那些「ready 前就呼叫過 t(key, el)」的 elements
    _flushPendingPatches: function() {
        if (!this._pendingPatches || this._pendingPatches.length === 0) return;
        const patches = this._pendingPatches;
        this._pendingPatches = [];
        try {
            for (const { el, key, attr } of patches) {
                try {
                    if (!el || !el.nodeType) continue;
                    const translated = this.t(key);
                    if (!translated || translated === key) continue;
                    if (attr === 'placeholder') {
                        el.placeholder = translated;
                    } else if (attr === 'title') {
                        el.setAttribute('title', translated);
                    } else {
                        // 預設更新 textContent，但若 element 有子節點（icon 等）則只更新 text node
                        const hasElementChildren = Array.from(el.childNodes || []).some(n => n.nodeType === Node.ELEMENT_NODE);
                        if (hasElementChildren) {
                            // 找第一個純文字 text node 更新，避免破壞圖標結構
                            for (const node of el.childNodes) {
                                if (node.nodeType === Node.TEXT_NODE && node.textContent.trim()) {
                                    node.textContent = translated;
                                    break;
                                }
                            }
                        } else {
                            el.textContent = translated;
                        }
                    }
                } catch (e) { /* 單個 patch 失敗不影響其他 */ }
            }
        } catch (e) {
            console.warn('i18n _flushPendingPatches error:', e);
        }
    },

    // 獲取翻譯
    // el（可選）：若翻譯未 ready，記錄此 element，ready 後自動補更新
    // attr（可選）：要更新的屬性，null=textContent, 'placeholder', 'title'
    t: function(key, el, attr) {
        const keys = key.split('.');
        let value = this.translations;
        
        for (const k of keys) {
            if (value && typeof value === 'object') {
                value = value[k];
            } else {
                // 如果找不到，返回 key（但如果是 menu.blogs，尝试直接查找 menu 对象）
                if (key === 'menu.blogs' && this.translations.menu && this.translations.menu.blogs) {
                    return this.translations.menu.blogs;
                }
                // 翻譯未 ready 時若有傳入 element，記錄起來等 ready 後補更新
                if (!this._ready && el && el.nodeType) {
                    this._pendingPatches.push({ el, key, attr: attr || null });
                }
                return key; // 如果找不到，返回 key
            }
        }
        
        // 翻譯 key 存在但翻譯未 ready（translations 還是 {}）時，
        // value 會等於 undefined，也要 queue 起來
        if (!this._ready && el && el.nodeType && (value === undefined || value === null)) {
            this._pendingPatches.push({ el, key, attr: attr || null });
        }
        return value || key;
    },

    // Apply translations to a specific DOM subtree (for dynamically shown elements like ad-promo-card)
    applyTranslations: function(rootEl) {
        if (!rootEl || !rootEl.querySelectorAll) return;

        const toText = (translation, fallbackText) => {
            if (typeof translation === 'string') return translation;
            if (typeof translation === 'number' || typeof translation === 'boolean') return String(translation);
            if (translation && typeof translation === 'object') {
                const candidates = ['text', 'label', 'title', 'value', 'name', 'default'];
                for (const k of candidates) {
                    if (typeof translation[k] === 'string' && translation[k].trim() !== '') return translation[k];
                }
                return fallbackText || '';
            }
            return fallbackText || '';
        };

        try {
            rootEl.querySelectorAll('[data-i18n]').forEach(el => {
                try {
                    const key = el.getAttribute('data-i18n');
                    const translation = this.t(key);
                    if (translation && translation !== key) {
                        el.textContent = toText(translation, el.textContent || '');
                    }
                } catch (e) { /* ignore */ }
            });
            rootEl.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
                try {
                    const key = el.getAttribute('data-i18n-placeholder');
                    const translation = this.t(key);
                    if (translation && translation !== key) {
                        el.placeholder = toText(translation, el.placeholder || '');
                    }
                } catch (e) { /* ignore */ }
            });
            rootEl.querySelectorAll('[data-i18n-title]').forEach(el => {
                try {
                    const key = el.getAttribute('data-i18n-title');
                    const translation = this.t(key);
                    if (translation && translation !== key) {
                        el.setAttribute('title', toText(translation, ''));
                    }
                } catch (e) { /* ignore */ }
            });
        } catch (e) {
            console.warn('i18n applyTranslations error:', e);
        }
    },

    // 更新頁面內容
    updatePage: function() {
        // Public page（/co/... 或 custom domain）暫時不做任何自動翻譯，保持原始內容
        // 即使用戶在 vWork 後台已登入，前台頁面也不應被 i18n 改動
        // 偵測方式：/co/ 路徑 或 模板注入的 __PUBLIC_DEFAULT_LANG（涵蓋 custom domain）
        const isPublicPage = (typeof window !== 'undefined')
            ? (window.location && window.location.pathname && window.location.pathname.startsWith('/co/'))
              || (typeof window.__PUBLIC_DEFAULT_LANG !== 'undefined' && window.__PUBLIC_DEFAULT_LANG !== '')
            : false;
        if (isPublicPage) {
            // 只移除 loading 狀態，不翻譯任何內容
            try {
                if (document.body) {
                    document.body.classList.remove('i18n-loading');
                }
            } catch (e) { /* ignore */ }
            return;
        }

        // 將翻譯結果轉成可安全顯示的字串；若拿到物件，優先取常見欄位，否則保留原文字避免顯示 [object Object]
        const toText = (translation, fallbackText = '') => {
            if (typeof translation === 'string') return translation;
            if (typeof translation === 'number' || typeof translation === 'boolean') return String(translation);
            if (translation && typeof translation === 'object') {
                const candidates = ['text', 'label', 'title', 'value', 'name', 'default'];
                for (const k of candidates) {
                    if (typeof translation[k] === 'string' && translation[k].trim() !== '') return translation[k];
                }
                return fallbackText || '';
            }
            return fallbackText || '';
        };

        // 使用 try-catch 包裹所有 DOM 操作，避免單個元素錯誤導致整個更新中斷
        const safeUpdate = (fn) => {
            try {
                fn();
            } catch (e) {
                console.warn('i18n update element error:', e);
            }
        };

        // 更新所有帶有 data-i18n-placeholder 屬性的元素
        try {
            document.querySelectorAll('[data-i18n-placeholder]').forEach(element => {
                safeUpdate(() => {
                    const key = element.getAttribute('data-i18n-placeholder');
                    const translation = this.t(key);
                    if (element.tagName === 'INPUT' || element.tagName === 'TEXTAREA') {
                        const fallback = element.placeholder || '';
                        const translationStr = (translation === key) ? fallback : toText(translation, fallback);
                        element.placeholder = translationStr;
                    }
                });
            });
        } catch (e) {
            console.warn('i18n updatePlaceholder error:', e);
        }
        
        // 更新所有帶有 data-i18n 屬性的元素
        try {
            document.querySelectorAll('[data-i18n]').forEach(element => {
                safeUpdate(() => {
                    const key = element.getAttribute('data-i18n');
                    const translation = this.t(key);
                    const fallback = element.textContent || '';
                    const translationStr = (translation === key) ? fallback : toText(translation, fallback);
                    
                    // 如果是 SPAN 元素，直接更新文本（保留父元素的其他内容）
                    if (element.tagName === 'SPAN') {
                        element.textContent = translationStr;
                        return;
                    }
                    
                    if (element.tagName === 'INPUT' && element.type !== 'submit' && element.type !== 'button') {
                        // 如果沒有 data-i18n-placeholder，才更新 placeholder
                        if (!element.hasAttribute('data-i18n-placeholder')) {
                            element.placeholder = translationStr;
                        } else {
                            // 如果有 data-i18n-placeholder，更新其他屬性（如 value）
                            if (element.type === 'text' || element.type === 'email' || element.type === 'tel' || element.type === 'url' || element.type === 'search') {
                                // 對於文本輸入框，如果沒有 value，可以更新 placeholder（但已經在上面處理了）
                            }
                        }
                    } else if (element.tagName === 'IMG' && element.hasAttribute('alt')) {
                        element.alt = translationStr;
                    } else if (element.tagName === 'A' && element.hasAttribute('href')) {
                        // 檢查是否有子元素包含圖標或其他元素
                        const icon = element.querySelector('i');
                        const span = element.querySelector('span[data-i18n]');
                        
                        // 如果有 data-i18n 的 span，只更新那個 span
                        if (span) {
                            span.textContent = translationStr;
                        } else if (icon) {
                            // 保留圖標，更新文字
                            const otherElements = Array.from(element.childNodes).filter(node => 
                                node.nodeType === Node.ELEMENT_NODE && node.tagName !== 'I'
                            );
                            element.innerHTML = icon.outerHTML + ' ' + translationStr;
                            // 保留其他非圖標元素（如下拉箭頭）
                            otherElements.forEach(el => {
                                if (el.tagName !== 'SPAN' || !el.hasAttribute('data-i18n')) {
                                    element.appendChild(el);
                                }
                            });
                        } else {
                            element.textContent = translationStr;
                        }
                    } else if (element.tagName === 'BUTTON') {
                        // 按鈕可能包含圖標（i 標籤）或 SVG 或 span
                        const icon = element.querySelector('i');
                        const span = element.querySelector('span[data-i18n]');
                        const svg = element.querySelector('svg');
                        
                        if (span) {
                            // 如果有 span[data-i18n]，只更新 span 的文本
                            span.textContent = translationStr;
                        } else if (icon) {
                            // 如果有 i 圖標，保留圖標並更新文字
                            element.innerHTML = icon.outerHTML + ' ' + translationStr;
                        } else if (svg) {
                            // 如果有 SVG，保留 SVG 並在後面添加翻譯文字
                            const svgHTML = svg.outerHTML;
                            element.innerHTML = svgHTML + ' ' + translationStr;
                        } else {
                            // 沒有圖標，直接更新整個按鈕文本
                            element.textContent = translationStr;
                        }
                    } else {
                        element.textContent = translationStr;
                    }
                });
            });
        } catch (e) {
            console.warn('i18n updateElements error:', e);
        }

        // 更新頁面標題（只更新 <title> 標籤的 data-i18n-title）
        // 若翻譯失敗（t() 回傳 key 本身），保留 <title> 的原始文字，避免顯示 raw key（如 "login.title"）
        try {
            const pageTitle = document.querySelector('title[data-i18n-title]');
            if (pageTitle) {
                const titleKey = pageTitle.getAttribute('data-i18n-title');
                const translated = this.t(titleKey);
                if (translated && translated !== titleKey) {
                    document.title = translated;
                }
                // else: keep original <title> text (e.g. "Login - V-sys")
            }
        } catch (e) {
            console.warn('i18n updateTitle error:', e);
        }
        
        // 自動翻譯頁面標題（當 <title> 沒有 data-i18n-title 時）
        // 目標：讓所有頁面的「xx - vWork」在切換語言後可自動更新
        try {
            const pageTitle = document.querySelector('title[data-i18n-title]');
            if (!pageTitle) {
                const trySetTitle = () => {
                try {
                    const siteName = 'vWork';
                    const rawTitle = (document.title || '').trim();
                    if (!rawTitle) return;
                    
                    // 避免覆蓋 login.html 這種自帶 data-i18n-title 的頁面（上面已處理）
                    // 只處理包含 vWork 的 title（常見格式：<Title> - vWork）
                    const hasSite = rawTitle.includes(siteName);
                    if (!hasSite) return;
                    
                    // 解析 pageName：優先 meta[name="page-name"]，否則從 URL 推斷
                    const metaPageName = document.querySelector('meta[name="page-name"]');
                    let pageName = metaPageName ? (metaPageName.getAttribute('content') || '').trim() : '';
                    if (!pageName) {
                        const parts = window.location.pathname.split('/').filter(Boolean);
                        if (parts.length === 0) return;
                        
                        const last = parts[parts.length - 1];
                        const secondLast = parts.length >= 2 ? parts[parts.length - 2] : '';
                        const thirdLast = parts.length >= 3 ? parts[parts.length - 3] : '';
                        
                        const looksLikeUUID = (s) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s);
                        const looksLikeId = (s) => /^[0-9]+$/.test(s) || looksLikeUUID(s);
                        
                        if (last === 'new') {
                            pageName = secondLast || last;
                        } else if (last === 'edit') {
                            // e.g. /currencies/:id/edit -> currencies
                            pageName = thirdLast || secondLast || last;
                        } else if (parts.length >= 2 && parts[0] === 'inventory' && parts[1] === 'low-stock') {
                            // 特殊路徑：/inventory/low-stock
                            pageName = 'inventory/lowStock';
                        } else if (looksLikeId(last)) {
                            // e.g. /customers/:id -> customers
                            pageName = secondLast || last;
                        } else {
                            pageName = last;
                        }
                    }
                    
                    // 將 pageName（kebab/snake）轉 menu key（camelCase）
                    const toMenuKey = (name) => {
                        if (!name) return '';
                        // 保留包含 "/" 的情況（例如 inventory/lowStock）
                        if (name.includes('/')) return name;
                        const parts = name.split(/[-_]/g).filter(Boolean);
                        if (parts.length === 0) return name;
                        return parts.map((p, i) => i === 0 ? p : (p.charAt(0).toUpperCase() + p.slice(1))).join('');
                    };
                    
                    const candidates = [];
                    // 1) 允許直接用 menu.<pageName>（某些 key 可能本來就是 camelCase）
                    candidates.push(`menu.${pageName}`);
                    // 2) 將 pageName 轉成 camelCase 再查 menu
                    candidates.push(`menu.${toMenuKey(pageName)}`);
                    // 3) 若是 inventory/lowStock，再嘗試 menu.lowStock
                    if (pageName === 'inventory/lowStock') candidates.push('menu.lowStock');
                    
                    // 找到第一個可用翻譯
                    let translated = '';
                    for (const key of candidates) {
                        const t = this.t(key);
                        if (t && t !== key) {
                            translated = t;
                            break;
                        }
                    }
                    if (!translated) return;
                    
                    document.title = `${translated} - ${siteName}`;
                } catch (e) {
                    // 不要影響整體 i18n 更新
                    console.warn('Auto title i18n failed:', e);
                }
            };
            
                trySetTitle();
            }
        } catch (e) {
            console.warn('i18n autoTitle error:', e);
        }
        
        // 更新所有元素的 title 屬性
        try {
            document.querySelectorAll('[data-i18n-title]').forEach(element => {
                safeUpdate(() => {
                    const key = element.getAttribute('data-i18n-title');
                    const translation = this.t(key);
                    element.setAttribute('title', translation);
                });
            });
        } catch (e) {
            console.warn('i18n updateTitleAttr error:', e);
        }
        
        // 更新字段標籤（表單和列表）
        try {
            document.querySelectorAll('[data-i18n-field]').forEach(element => {
                safeUpdate(() => {
                    const fieldKey = element.getAttribute('data-i18n-field');
                    // 嘗試從 fields 翻譯鍵獲取翻譯
                    const fieldTranslation = this.t(`fields.${fieldKey}`);
                    if (fieldTranslation && fieldTranslation !== `fields.${fieldKey}`) {
                        // 保留必填標記（*）
                        const requiredMark = element.innerHTML.match(/<span class="text-danger">\*<\/span>/);
                        element.innerHTML = fieldTranslation + (requiredMark ? ' ' + requiredMark[0] : '');
                    }
                });
            });
        } catch (e) {
            console.warn('i18n updateFieldLabels error:', e);
        }

        // 後備：自動翻譯「未加 data-i18n」但文本是中文的短 label/button/header（用 fields.<中文>）
        // 目標：快速覆蓋像 quotations_new.html 這類歷史頁面，避免整頁都不翻譯
        // 使用分批處理避免大量 DOM 操作阻塞
        try {
            const candidates = Array.from(document.querySelectorAll('label, th, button, h1, h2, h3, h4, h5, h6, option, small, span'));
            const batchSize = 50; // 每批處理 50 個元素
            let index = 0;
            
            const processBatch = () => {
                const end = Math.min(index + batchSize, candidates.length);
                for (let i = index; i < end; i++) {
                    const el = candidates[i];
                    if (!el) continue;
                    safeUpdate(() => {
                        if (el.hasAttribute('data-i18n') || el.hasAttribute('data-i18n-field') || el.hasAttribute('data-i18n-placeholder')) return;

                        const raw = (el.textContent || '').trim();
                        if (!raw) return;
                        // 只處理含中文且長度不太長的文本，避免翻譯段落內容
                        if (!/[\u3400-\u9FFF]/.test(raw)) return;
                        if (raw.length > 30) return;

                        // 若元素包含多個子元素（例如 required *、icon），跳過避免破壞結構
                        const hasNonTrivialChildren = Array.from(el.childNodes || []).some(n => n && n.nodeType === Node.ELEMENT_NODE);
                        if (hasNonTrivialChildren) {
                            // 允許唯一子元素是 required * 的情況
                            const requiredMark = el.innerHTML && el.innerHTML.match(/<span class="text-danger">\*<\/span>/);
                            if (!requiredMark) return;

                            // 嘗試只翻譯純文本部分
                            const textOnly = raw.replace(/\*/g, '').trim();
                            const k0 = `fields.${textOnly}`;
                            const t0 = this.t(k0);
                            if (t0 && t0 !== k0) {
                                el.innerHTML = t0 + ' ' + requiredMark[0];
                            }
                            return;
                        }

                        const k = `fields.${raw}`;
                        const translated = this.t(k);
                        if (translated && translated !== k) {
                            el.textContent = translated;
                        }
                    });
                }
                
                index = end;
                if (index < candidates.length) {
                    // 使用 requestAnimationFrame 繼續處理下一批，避免阻塞
                    requestAnimationFrame(processBatch);
                }
            };
            
            if (candidates.length > 0) {
                processBatch();
            }
        } catch (e) {
            console.warn('i18n autoTranslate error:', e);
        }

        // 更新語言切換按鈕
        try {
            document.querySelectorAll('[data-lang]').forEach(btn => {
                safeUpdate(() => {
                    if (btn.getAttribute('data-lang') === this.currentLang) {
                        btn.classList.add('active');
                    } else {
                        btn.classList.remove('active');
                    }
                });
            });
        } catch (e) {
            console.warn('i18n updateLangButtons error:', e);
        }
        
        // 重新渲染動態列表（如果存在）- 使用 setTimeout 避免阻塞
        try {
            if (window.dynamicList && typeof window.dynamicList.render === 'function') {
                setTimeout(() => {
                    try {
                        window.dynamicList.render();
                        // 重新載入數據以更新內容
                        if (typeof window.dynamicList.loadData === 'function') {
                            window.dynamicList.loadData();
                        }
                    } catch (e) {
                        console.warn('i18n dynamicList render error:', e);
                    }
                }, 0);
            }
        } catch (e) {
            console.warn('i18n dynamicList error:', e);
        }
        
        // Update dynamic form labels only (do NOT re-render).
        // Calling render() would wipe all populated form values because
        // render() does container.innerHTML = ... which destroys existing
        // inputs and their values.  The data-i18n / data-i18n-field
        // processing above already translates labels, buttons, and
        // headings, so a full re-render is unnecessary.
        // (Previous code called window.dynamicForm.render() here which
        //  caused the business-goals edit-page SPA bug: values appeared
        //  briefly then disappeared.)
        
        // 移除 i18n-loading 類，顯示翻譯後的內容
        try {
            if (document.body) {
                document.body.classList.remove('i18n-loading');
            }
        } catch (e) {
            console.warn('i18n removeLoadingClass error:', e);
        }
    }
};

// 頁面加載時初始化
document.addEventListener('DOMContentLoaded', function() {
    I18n.init();
});
