// vWork 主應用 JavaScript

// ─── Product-aware redirect URLs (shared across all pages) ─────────────
// Maps vlogin_product → landing page after login / onboarding.
// auth.js also references this via App.getProductRedirectUrl().
const PRODUCT_REDIRECT_URLS = {
    vwork:   'https://www.vworkai.com/dashboard',
    vai:     'https://vai.vsysai.com/vai-chat',
    voffice: 'https://voffice.vsysai.com/voffice-download',
};

// Maps each known domain to the product it belongs to, plus the
// relative path the user should land on after login.
// This allows login on any product domain to redirect locally
// (same-origin) instead of always bouncing to vworkai.com.
const DOMAIN_PRODUCT_MAP = {
    'www.vworkai.com':    { product: 'vwork',   path: '/dashboard' },
    'vworkai.com':        { product: 'vwork',   path: '/dashboard' },
    'vai.vsysai.com':     { product: 'vai',     path: '/vai-chat' },
    'voffice.vsysai.com': { product: 'voffice', path: '/voffice-download' },
};

/**
 * Detect which product the current domain belongs to.
 * Returns an object { product, path } or null if unknown.
 */
function detectCurrentDomainProduct() {
    const host = window.location.hostname.toLowerCase();
    // Exact match first
    if (DOMAIN_PRODUCT_MAP[host]) return DOMAIN_PRODUCT_MAP[host];
    // Try with port stripped (local dev uses localhost:3001)
    const hostPort = window.location.host.toLowerCase();
    if (DOMAIN_PRODUCT_MAP[hostPort]) return DOMAIN_PRODUCT_MAP[hostPort];
    return null;
}

/**
 * Return the correct post-login URL based on the product the user
 * selected in the login product-selector (stored in localStorage).
 *
 * Key improvement: if the user is already on the target product's
 * domain, return a relative path so no cross-domain redirect is needed
 * (the cookie stays valid on the current origin).
 *
 * Default: /dashboard (vWork) on current domain.
 */
function getProductRedirectUrl() {
    const product = (localStorage.getItem('vlogin_product') || 'vwork').toLowerCase();
    const host = window.location.hostname.toLowerCase();

    // v00(2026-03-09): Fix for local dev multi-product redirect.
    // If on localhost, use relative paths based on selected product.
    if (host === 'localhost' || host === '127.0.0.1') {
        const localPaths = {
            'vwork':   '/dashboard',
            'vai':     '/vai-chat',
            'voffice': '/voffice-download',
        };
        return localPaths[product] || '/dashboard';
    }

    const currentDomain = detectCurrentDomainProduct();

    // If user is already on the domain that matches their selected product,
    // redirect locally (relative path) — avoids cross-domain cookie issues.
    if (currentDomain && currentDomain.product === product) {
        return currentDomain.path;
    }

    // If no product was explicitly selected (default vwork) AND the current
    // domain has a known landing page, stay on the current domain.
    if (product === 'vwork' && currentDomain) {
        return currentDomain.path;
    }

    // Otherwise, the user explicitly chose a different product than the
    // domain they're on — return the full cross-domain URL (will need SSO).
    return PRODUCT_REDIRECT_URLS[product] || '/dashboard';
}

/**
 * Return the /setup-tenant URL with ?product= query parameter
 * based on the product the user selected at login (stored in localStorage).
 * Only appends the param for non-vwork products.
 */
function getSetupTenantUrl() {
    const product = (localStorage.getItem('vlogin_product') || 'vwork').toLowerCase();
    if (product && product !== 'vwork') {
        return '/setup-tenant?product=' + encodeURIComponent(product);
    }
    return '/setup-tenant';
}

/**
 * Redirect the browser to the product-specific landing page.
 * This is a convenience wrapper used by onboarding pages that
 * don't load auth.js.
 *
 * If the URL is a relative path (same-origin), redirect directly.
 * If it is cross-domain, use SSO.navigateTo() when available so the
 * auth cookie/token is carried across domains via an SSO ticket.
 */
function redirectToProductPage() {
    const url = getProductRedirectUrl();
    // Relative path → same-origin, just navigate
    if (url.startsWith('/')) {
        window.location.href = url;
        return;
    }
    // Cross-domain: use SSO if available
    if (typeof SSO !== 'undefined' && SSO.navigateTo) {
        SSO.navigateTo(url);
    } else {
        window.location.href = url;
    }
}

// API 基礎 URL
const API_BASE = '/api/v1';

// 工具函數
const App = {
    // 將常見的中文提示訊息在英文介面下轉成英文（避免大量舊代碼未 i18n 化）
    // 注意：這只處理純文字訊息（包含 HTML 的訊息不做改寫，以免破壞結構）
    normalizeAlertMessage: function(message) {
        try {
            if (typeof message !== 'string') return message;
            if (message.includes('<')) return message;
            if (typeof I18n === 'undefined' || !I18n.currentLang) return message;
            if (I18n.currentLang !== 'en') return message;

            let m = message.trim();

            // Prefix replacements (keep the detail suffix)
            const prefixRules = [
                [/^載入數據失敗[:：]?\s*/u, 'Failed to load data: '],
                [/^載入失敗[:：]?\s*/u, 'Load failed: '],
                [/^載入使用記錄失敗[:：]?\s*/u, 'Failed to load usage records: '],
                [/^保存失敗[:：]?\s*/u, 'Save failed: '],
                [/^更新失敗[:：]?\s*/u, 'Update failed: '],
                [/^批核失敗[:：]?\s*/u, 'Approval failed: '],
                [/^拒絕失敗[:：]?\s*/u, 'Rejection failed: '],
                [/^轉換失敗[:：]?\s*/u, 'Conversion failed: '],
                [/^圖片上傳失敗[:：]?\s*/u, 'Image upload failed: '],
                [/^附件上傳失敗[:：]?\s*/u, 'Attachment upload failed: '],
                [/^顯示裁剪窗口失敗[:：]?\s*/u, 'Failed to open cropper: '],
                [/^更新匯率失敗[:：]?\s*/u, 'Failed to update exchange rates: '],
                [/^生成佣金支出失敗[:：]?\s*/u, 'Failed to generate commission expenses: '],
                [/^保存條件時發生錯誤[:：]?\s*/u, 'Failed to save conditions: '],
                [/^配置錯誤[:：]?\s*/u, 'Configuration error: '],
                [/^ID 錯誤[:：]?\s*/u, 'ID error: '],
                [/^資料更新成功[:：]?\s*/u, 'Data updated successfully'],
                [/^服務單保存成功[:：]?\s*/u, 'Service order saved successfully'],
            ];
            for (const [re, replacement] of prefixRules) {
                if (re.test(m)) {
                    m = m.replace(re, replacement);
                    return m;
                }
            }

            // Exact replacements
            const exact = new Map([
                ['保存成功', 'Saved successfully'],
                ['保存失敗', 'Save failed'],
                ['更新成功', 'Updated successfully'],
                ['更新失敗', 'Update failed'],
                ['創建成功', 'Created successfully'],
                ['創建失敗', 'Create failed'],
                ['刪除成功', 'Deleted successfully'],
                ['刪除失敗', 'Delete failed'],
                ['載入失敗', 'Load failed'],
                ['載入中...', 'Loading...'],
                ['請填寫必填字段', 'Please fill in required fields'],
                ['請選擇圖片文件', 'Please select an image file'],
                ['圖片大小不能超過 5MB', 'Image size cannot exceed 5MB'],
                ['頭像上傳成功', 'Avatar uploaded successfully'],
                ['已拒絕', 'Rejected'],
                ['已批核並產生支出', 'Approved and expense created'],
                ['已成功轉成訂單', 'Converted successfully'],
                ['轉換失敗', 'Conversion failed'],
                ['此優惠券尚未被使用', 'This coupon has not been used yet'],
                ['未設定頁面名稱，無法載入草稿', 'Page name is not set. Unable to load draft.'],
                ['載入的數據為空', 'Loaded data is empty'],
                ['圖片裁剪功能未載入，請刷新頁面重試', 'Image cropper is not loaded. Please refresh and try again.'],
                ['地址已標記為刪除，將在保存客戶時生效', 'Address marked for deletion. It will take effect when you save the customer.'],
                ['系統未初始化，請重新整理頁面', 'System is not initialized. Please refresh the page.'],
                ['導出功能未載入，請重新整理頁面', 'Export feature is not loaded. Please refresh the page.'],
            ]);
            if (exact.has(m)) return exact.get(m);

            // Handle common variants with punctuation or suffix
            const normalizePunc = (s) => s.replace(/[!！。．…]+$/u, '').trim();
            const m2 = normalizePunc(m);
            if (exact.has(m2)) return exact.get(m2);

            // Special long message variants seen in forms
            if (m.startsWith('保存成功，但重新加載數據失敗')) {
                return 'Saved successfully, but failed to reload data. Please refresh the page to view the latest data.';
            }
            if (m2 === '服務單創建成功') {
                return 'Service order created successfully';
            }

            return message;
        } catch (_e) {
            return message;
        }
    },

    // Determine the service home URL based on current page/product context.
    // Used by logout to redirect back to the relevant service landing page.
    getServiceHomeUrl: function() {
        try {
            var path = window.location.pathname;
            // Help pages → stay on respective help section
            if (path.startsWith('/help/vwork')) return '/help/vwork';
            if (path.startsWith('/help/voffice')) return '/help/voffice';
            if (path.startsWith('/help')) return '/help';
            if (path.startsWith('/contact')) return '/contact';

            // Product detection via query param or hostname
            var params = new URLSearchParams(window.location.search);
            var domain = params.get('domain');
            var hostname = window.location.hostname.toLowerCase();
            var isLocal = hostname === 'localhost' || hostname === '127.0.0.1';

            // vai
            if (domain === 'vai' || hostname.includes('vai.')) {
                return isLocal ? '/?domain=vai' : '/';
            }
            // voffice
            if (domain === 'voffice' || hostname.includes('voffice.')) {
                return isLocal ? '/?domain=voffice' : '/';
            }
            // vsys
            if (domain === 'vsys' || hostname.includes('vsysai.com')) {
                return isLocal ? '/?domain=vsys' : '/';
            }
            // vwork (explicit domain param or production hostname)
            if (domain === 'vwork' || hostname.includes('vwork')) {
                return '/';
            }
        } catch (_e) { /* ignore */ }
        // Default: root (vWork on localhost, or whichever product the hostname resolves to)
        return '/';
    },

    // 顯示提示訊息
    showAlert: function(message, type = 'info', containerId = 'alertContainer') {
        const container = document.getElementById(containerId);
        if (!container) return;

        message = this.normalizeAlertMessage(message);

        const alert = document.createElement('div');
        alert.className = `alert alert-${type} alert-dismissible fade show`;
        alert.innerHTML = `
            ${message}
            <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
        `;
        container.innerHTML = '';
        container.appendChild(alert);

        // 3秒後自動關閉
        setTimeout(() => {
            alert.remove();
        }, 3000);
    },

    // 兼容舊代碼：顯示錯誤訊息（danger）
    showError: function(message, containerId = 'alertContainer') {
        return this.showAlert(message, 'danger', containerId);
    },

    // 兼容舊代碼：顯示成功訊息（success）
    showSuccess: function(message, containerId = 'alertContainer') {
        return this.showAlert(message, 'success', containerId);
    },

    // API 請求
    apiRequest: async function(url, options = {}) {
        const defaultOptions = {
            headers: {
                'Content-Type': 'application/json',
            },
        };

        // 添加租戶信息（如果存在）
        let tenantSubdomain = localStorage.getItem('tenant_subdomain');
        if (!tenantSubdomain && typeof window !== 'undefined' && window.tenantSubdomain) {
            tenantSubdomain = window.tenantSubdomain;
            try { localStorage.setItem('tenant_subdomain', tenantSubdomain); } catch (e) {}
        }
        if (tenantSubdomain) {
            defaultOptions.headers['X-Tenant-Subdomain'] = tenantSubdomain;
        }

        // 添加認證 token（如果存在）
        const token = localStorage.getItem('auth_token');
        if (token) {
            defaultOptions.headers['Authorization'] = `Bearer ${token}`;
        }

        // 始終帶 credentials，讓 browser 自動發送 HTTPOnly cookie
        // 當 localStorage auth_token 不存在時，AuthMiddleware 會 fallback 到 cookie
        defaultOptions.credentials = 'same-origin';

        const finalOptions = {
            ...defaultOptions,
            ...options,
            headers: {
                ...defaultOptions.headers,
                ...options.headers,
            },
        };

        // 如果是 FormData，上傳 multipart 時不要手動設置 Content-Type
        // 讓瀏覽器自動帶 boundary，否則後端會拿不到檔案（FormFile("file") 失敗）
        if (finalOptions.body && (finalOptions.body instanceof FormData)) {
            if (finalOptions.headers) {
                delete finalOptions.headers['Content-Type'];
                delete finalOptions.headers['content-type'];
            }
        }

        try {
            let requestUrl = url;
            // 統一 API 前綴處理：
            // 1) 已是絕對網址 -> 直接使用
            // 2) 已包含 /api/ 開頭 -> 直接使用
            // 3) 其他相對路徑 -> 補上 /api/v1 前綴
            if (!/^https?:\/\//.test(url)) {
                if (!url.startsWith('/api/')) {
                    // 確保只有一個斜線
                    requestUrl = API_BASE + (url.startsWith('/') ? url : '/' + url);
                }
            }

            // 記錄請求詳情
            // console.log('📤 API Request:', {
            //     url: requestUrl,
            //     method: finalOptions.method || 'GET',
            //     headers: finalOptions.headers,
            //     body: finalOptions.body ? (typeof finalOptions.body === 'string' ? JSON.parse(finalOptions.body) : finalOptions.body) : null
            // });

            const response = await fetch(requestUrl, finalOptions);
            
            // 直接输出完整的 response 对象供调试
            // console.log('📥 ========== COMPLETE RESPONSE OBJECT ==========');
            // console.log('Response status:', response.status);
            // console.log('Response statusText:', response.statusText);
            // console.log('Response ok:', response.ok);
            // console.log('Response headers:', response.headers);
            // console.log('Response url:', response.url);
            // console.log('Response type:', response.type);
            // console.log('Response redirected:', response.redirected);
            
            // 输出所有 headers
            const headersObj = {};
            response.headers.forEach((value, key) => {
                headersObj[key] = value;
            });
            // console.log('Response headers (object):', headersObj);
            
            let data;
            const contentType = response.headers.get('content-type') || '';
            // console.log('Content-Type:', contentType);
            // console.log('Request URL:', requestUrl);
            
            try {
                const text = await response.text();
                // console.log('📥 Response text (full length):', text.length);
                // console.log('📥 Response text (full content):', text);
                // console.log('📥 Response text (first 1000 chars):', text.substring(0, 1000));
                // console.log('📥 Response text (is empty?):', text.length === 0);
                // console.log('📥 Response text (starts with < ?):', text.trim().startsWith('<'));
                // console.log('📥 Response text (starts with { ?):', text.trim().startsWith('{'));
                // console.log('📥 Response text (starts with [ ?):', text.trim().startsWith('['));
                
                if (text) {
                    try {
                        data = JSON.parse(text);
                    } catch (e) {
                        console.error('❌ API Response is not valid JSON');
                        console.error('❌ Response status:', response.status, response.statusText);
                        console.error('❌ Content-Type:', contentType);
                        console.error('❌ Response text (full):', text);
                        console.error('❌ Response text (first 500 chars):', text.substring(0, 500));
                        // 如果响应是 HTML，提取可能的错误信息
                        let errorMsg = `Server returned invalid JSON (status ${response.status})`;
                        if (text.trim().startsWith('<')) {
                            // 尝试从 HTML 中提取错误信息
                            const titleMatch = text.match(/<title>(.*?)<\/title>/i);
                            const h1Match = text.match(/<h1[^>]*>(.*?)<\/h1>/i);
                            const errorMatch = text.match(/error[^>]*>([^<]+)/i);
                            if (titleMatch) errorMsg += `: ${titleMatch[1]}`;
                            else if (h1Match) errorMsg += `: ${h1Match[1]}`;
                            else if (errorMatch) errorMsg += `: ${errorMatch[1]}`;
                            else errorMsg += `: HTML response received (likely an error page)`;
                        } else {
                            errorMsg += `: ${text.substring(0, 200)}`;
                        }
                        throw new Error(errorMsg);
                    }
                } else {
                    // 空响应，根据状态码决定返回什么
                    if (response.ok) {
                        data = { success: true };
                    } else {
                        data = { error: `Server error (${response.status})` };
                    }
                }
            } catch (e) {
                console.error('❌ Failed to parse response:', e);
                console.error('❌ Error type:', typeof e);
                console.error('❌ Error message:', e.message);
                console.error('❌ Error stack:', e.stack);
                console.error('❌ Response status:', response.status, response.statusText);
                console.error('❌ Content-Type:', contentType);
                // 如果是网络错误或其他错误，使用原始错误消息
                if (e.message && (e.message.includes('Server') || e.message.includes('invalid JSON'))) {
                    throw e; // 重新抛出已经格式化的错误
                }
                throw new Error(`Failed to parse response (status ${response.status}): ${e.message || 'Unknown error'}`);
            }
            
            // 記錄響應詳情
            if (response.ok) {
                // console.log('📥 API Response:', {
                //     status: response.status,
                //     statusText: response.statusText,
                //     url: requestUrl,
                //     data: data
                // });
            } else {
                console.error('📥 API Error Response:', {
                    status: response.status,
                    statusText: response.statusText,
                    url: requestUrl,
                    error: data.error,
                    fullData: JSON.stringify(data, null, 2)
                });
            }
            
            if (!response.ok) {
                // 付款/訂閱導流：若後端回傳 redirect，直接跳轉，避免彈出錯誤訊息干擾用戶
                // 但「公開頁」（/、/contact、/help 等）不應被強制導走，即使已過期也要能正常瀏覽
                const currentPath = window.location && window.location.pathname ? window.location.pathname : '';
                const _hostname = window.location.hostname.toLowerCase();
                const _isVMarketDomain = _hostname === 'www.vmarketai.com' || _hostname === 'vmarketai.com';
                const isPublicPageForRedirect = _isVMarketDomain ||
                    currentPath === '/' ||
                    currentPath === '/login' ||
                    currentPath === '/reset-password' ||
                    (currentPath.startsWith && (currentPath.startsWith('/help') || currentPath.startsWith('/contact') || currentPath.startsWith('/static/') || currentPath.startsWith('/vwork-blog') || currentPath.startsWith('/vmarket') || currentPath.startsWith('/co/')));

                if (!isPublicPageForRedirect && (response.status === 403 || response.status === 402) && data && data.redirect) {
                    try {
                        const target = String(data.redirect || '').trim();
                        if (target && window.location && window.location.pathname !== target) {
                            if (typeof Router !== 'undefined' && Router.go) {
                                Router.go(target);
                            } else {
                                window.location.href = target;
                            }
                            // 阻止後續 then/catch 造成 alert；頁面即將跳轉
                            return new Promise(() => {});
                        }
                    } catch (_e) {
                        // ignore and fallback to normal error handling
                    }
                }

                // 如果是 401 未授權錯誤，檢查是否在公開頁面
                if (response.status === 401) {
                    const path = window.location.pathname;
                    const isPublicPage = path === '/help' || path === '/contact' || path === '/' || path === '/login' || path === '/reset-password' || path === '/sales-partner' || path === '/enterprise-custom' ||
                                       (path.startsWith && (path.startsWith('/help/') || path.startsWith('/static/') || path.startsWith('/sales-partner') || path.startsWith('/vwork-blog') || path.startsWith('/vwork-events') || path.startsWith('/industry/') || path.startsWith('/custom/') || path.startsWith('/co/')));
                    
                    // 區分真正的 session 失效 vs 非致命 401：
                    // 只有明確的認證失敗才清除 session 並重定向；
                    // Tenant mismatch 或其他非 token 問題不應登出。
                    const errMsg = (data && (data.error || data.message || '')).toLowerCase();
                    const isSessionInvalid =
                        errMsg.includes('invalid or expired token') ||
                        errMsg.includes('session expired') ||
                        errMsg.includes('authorization header') ||
                        errMsg.includes('incomplete session') ||
                        errMsg.includes('account is inactive') ||
                        errMsg.includes('user not found') ||
                        errMsg.includes('tenant not found') ||
                        errMsg.includes('tenant is inactive');
                    
                    // 只有在非公開頁面 且 確認是 session 失效時才重定向
                    if (!isPublicPage && isSessionInvalid) {
                        console.warn('⚠️ 收到 401 未授權錯誤 (session 失效)，清除會話並跳轉到首頁:', errMsg);
                        App.clearSessionAndRedirect();
                    } else if (!isPublicPage) {
                        console.warn('⚠️ 收到 401 未授權錯誤，但非 session 失效，不登出:', errMsg);
                    } else {
                        console.warn('⚠️ 收到 401 未授權錯誤，但在公開頁面，不重定向');
                    }
                    throw new Error('Unauthorized');
                }
                
                const errorMessage = data.error || data.message || (
                    (typeof I18n !== 'undefined' && I18n.t)
                        ? I18n.t('common.requestFailed') + ` (${response.status})`
                        : `Request failed (${response.status})`
                );
                console.error('❌ API Error:', {
                    url: requestUrl,
                    status: response.status,
                    statusText: response.statusText,
                    error: errorMessage,
                    fullResponse: JSON.stringify(data, null, 2),
                    requestBody: finalOptions.body ? (typeof finalOptions.body === 'string' ? JSON.parse(finalOptions.body) : finalOptions.body) : null
                });
                // 如果错误信息包含更多详情，也显示出来
                if (data.details) {
                    console.error('Error Details:', data.details);
                }
                throw new Error(errorMessage);
            }
            
            return data;
        } catch (error) {
            console.error('❌ API 請求錯誤:', {
                url: url,
                error: error.message,
                stack: error.stack,
                fullError: error
            });
            throw error;
        }
    },

    // 檢查登錄狀態
    checkAuth: function() {
        // 如果是登錄頁或首頁，不檢查認證
        const path = window.location.pathname;
        const hostname = window.location.hostname.toLowerCase();
        // vmarketai.com 上所有公開市集頁面不需要認證
        const isVMarketDomain = hostname === 'www.vmarketai.com' || hostname === 'vmarketai.com';
        // 公開頁面：不需要認證即可訪問
        if (isVMarketDomain ||
            path === '/login' || 
            path === '/' || 
            path === '/reset-password' ||
            path === '/subscription-required' ||
            path === '/sales-partner' ||
            path === '/enterprise-custom' ||
            path === '/setup-tenant' ||
            path === '/profile-guide' ||
            path.startsWith('/static/') || 
            path.startsWith('/help') || 
            path.startsWith('/contact') ||
            path.startsWith('/sales-partner') ||
            path.startsWith('/vwork-blog') ||
            path.startsWith('/vwork-events') ||
            path.startsWith('/industry/') ||
            path.startsWith('/custom/') ||
            path.startsWith('/co/') ||
            path.startsWith('/vmarket')) {
            return true; // 提前返回，不執行後續的認證檢查和重定向
        }
        
        const token = localStorage.getItem('auth_token');
        const user = localStorage.getItem('user');
        const tenantSubdomain = localStorage.getItem('tenant_subdomain');
        const cookieAuth = sessionStorage.getItem('_cookie_auth');
        
        // 如果 cookie-based auth 已恢復過 session（user 已設定），
        // 則不需要 localStorage auth_token — apiRequest 會用 cookie
        if (!token && user && cookieAuth === '1') {
            return true; // cookie-based auth，允許繼續
        }
        
        // 如果 token 或 user 缺失，嘗試透過 HTTPOnly cookie 恢復 session
        // (server-side AuthMiddleware 可能已經驗證了 cookie，只是 localStorage 被清除了)
        if (!token || !user) {
            // 防止重複恢復：如果已經在恢復中，跳過
            if (window._checkAuthRecovering) {
                return false;
            }
            window._checkAuthRecovering = true;
            // 非同步嘗試恢復 — 用 credentials:'same-origin' 讓 browser 帶 HTTPOnly cookie
            fetch('/api/v1/user/me', {
                headers: { 'Accept': 'application/json' },
                credentials: 'same-origin'
            }).then(function(resp) {
                if (!resp.ok) throw new Error('not ok');
                return resp.json();
            }).then(function(userData) {
                if (userData && userData.id && userData.email) {
                    // Cookie 有效！恢復 localStorage
                    try {
                        localStorage.setItem('user', JSON.stringify(userData));
                        if (userData.tenant && userData.tenant.subdomain) {
                            localStorage.setItem('tenant_subdomain', String(userData.tenant.subdomain));
                        }
                        // 標記為 cookie-based auth（無法取得 HTTPOnly cookie 值）
                        // apiRequest 已加 credentials:'same-origin'，所以後續 API 呼叫
                        // 會自動帶 cookie，不需要 localStorage auth_token
                        sessionStorage.setItem('_cookie_auth', '1');
                    } catch (_e) { /* ignore */ }
                    // 重新載入頁面以正常初始化所有元件
                    // （reload 後 sessionStorage._cookie_auth 會被檢測到，避免再次進入此分支）
                    window.location.reload();
                } else {
                    // Cookie 無效或用戶資料不完整
                    window._checkAuthRecovering = false;
                    App.clearSessionAndRedirect();
                }
            }).catch(function() {
                // API 失敗（無 cookie 或 cookie 無效）
                window._checkAuthRecovering = false;
                App.clearSessionAndRedirect();
            });
            return false; // 暫時返回 false，等待非同步恢復
        }
        
        // 檢查 user 數據是否完整（至少應該有 id、email 和 name）
        try {
            const userData = JSON.parse(user);
            if (!userData) {
                // 用戶數據為空
                this.clearSessionAndRedirect();
                return false;
            }
            
            // 檢查必要的字段：id 和 email 是必須的，name 可選
            // （name 在某些 OAuth 流程中可能為空，不應因此登出用戶）
            const hasId = userData.id && String(userData.id).trim() !== '';
            const hasEmail = userData.email && String(userData.email).trim() !== '';
            
            // 如果缺少必要的字段，視為會話不完整
            if (!hasId || !hasEmail) {
                console.warn('⚠️ 用戶會話信息不完整:', {
                    hasId: hasId,
                    hasEmail: hasEmail,
                    userData: userData
                });
                this.clearSessionAndRedirect();
                return false;
            }
        } catch (e) {
            // user 數據無法解析，視為不完整
            console.error('⚠️ 無法解析用戶數據:', e);
            this.clearSessionAndRedirect();
            return false;
        }
        
        return true;
    },

    // 初始化租戶切換按鈕
    initTenantSwitcher: async function() {
        // console.log('[TenantSwitcher] 開始初始化...');
        const btn = document.getElementById('tenantSwitchBtn');
        const nameEl = document.getElementById('tenantSwitchName');
        const modalEl = document.getElementById('tenantSwitchModal');
        const listEl = document.getElementById('tenantSwitchList');
        const confirmBtn = document.getElementById('tenantSwitchConfirm');
        const addBtn = document.getElementById('tenantSwitchAddTenant');
        const errEl = document.getElementById('tenantSwitchError');

        // console.log('[TenantSwitcher] Elements:', { btn: !!btn, modalEl: !!modalEl, listEl: !!listEl, confirmBtn: !!confirmBtn });
        if (!btn || !modalEl || !listEl || !confirmBtn) return;

        let tenants = [];
        try {
            const resp = await this.apiRequest('/user/tenants');
            // console.log('[TenantSwitcher] API response:', resp);
            tenants = resp.data || [];
        } catch (e) {
            console.error('[TenantSwitcher] API error:', e);
            // API 失敗時，嘗試從 localStorage 獲取當前租戶
            const currentTenantId = localStorage.getItem('tenant_id');
            const currentTenantName = localStorage.getItem('tenant_name');
            const currentTenantSubdomain = localStorage.getItem('tenant_subdomain');
            if (currentTenantId) {
                // console.log('[TenantSwitcher] Using localStorage tenant as fallback');
                tenants = [{
                    id: currentTenantId,
                    name: currentTenantName || 'Tenant',
                    subdomain: currentTenantSubdomain || ''
                }];
            } else {
                btn.style.display = 'none';
                return;
            }
        }

        // console.log('[TenantSwitcher] Tenants from API:', tenants);
        if (!Array.isArray(tenants)) {
            // console.log('[TenantSwitcher] tenants is not array, hiding button');
            btn.style.display = 'none';
            return;
        }

        const currentTenantId = localStorage.getItem('tenant_id');
        const currentTenantName = localStorage.getItem('tenant_name');
        const currentTenantSubdomain = localStorage.getItem('tenant_subdomain');
        // console.log('[TenantSwitcher] localStorage:', { currentTenantId, currentTenantName, currentTenantSubdomain });
        if (currentTenantId) {
            const exists = tenants.some(t => String(t.id) === String(currentTenantId));
            // console.log('[TenantSwitcher] Current tenant exists in list?', exists);
            if (!exists) {
                // console.log('[TenantSwitcher] Adding current tenant to list');
                tenants.unshift({
                    id: currentTenantId,
                    name: currentTenantName || 'Tenant',
                    subdomain: currentTenantSubdomain || ''
                });
            }
        }

        // console.log('[TenantSwitcher] Final tenants count:', tenants.length);
        if (tenants.length === 0) {
            // console.log('[TenantSwitcher] No tenants, hiding button');
            btn.style.display = 'none';
            return;
        }

        btn.style.display = '';
        const currentTenant = tenants.find(t => String(t.id) === String(currentTenantId)) || tenants[0];
        if (nameEl && currentTenant) {
            nameEl.textContent = currentTenant.name || currentTenant.subdomain || '企業';
        }
        // Keep localStorage in sync with the latest API data
        if (currentTenant) {
            try {
                if (currentTenant.name && currentTenant.name !== currentTenantName) {
                    localStorage.setItem('tenant_name', currentTenant.name);
                }
                if (currentTenant.subdomain && currentTenant.subdomain !== currentTenantSubdomain) {
                    localStorage.setItem('tenant_subdomain', currentTenant.subdomain);
                }
            } catch (e) {}
        }

        const renderTenantList = () => {
            const currentId = localStorage.getItem('tenant_id');
            // 如果沒有選擇過租戶，默認選擇第一個
            const defaultId = currentId || (tenants.length > 0 ? String(tenants[0].id) : null);
            listEl.innerHTML = tenants.map(tn => {
                const checked = defaultId && String(tn.id) === String(defaultId);
                return `
                    <label class="list-group-item d-flex align-items-center gap-2">
                        <input class="form-check-input" type="radio" name="tenantSwitch" value="${tn.id}" ${checked ? 'checked' : ''}>
                        <div>
                            <div class="fw-bold">${tn.name || tn.subdomain || tn.id}</div>
                            <div class="text-muted small">${tn.subdomain || ''}</div>
                        </div>
                    </label>
                `;
            }).join('');
        };

        // Refresh tenant list from API and re-render
        const refreshAndRender = async () => {
            try {
                const resp = await this.apiRequest('/user/tenants');
                const fresh = resp.data || [];
                if (Array.isArray(fresh) && fresh.length > 0) {
                    tenants.length = 0;
                    fresh.forEach(t => tenants.push(t));
                }
            } catch (e) {
                console.warn('[TenantSwitcher] refresh failed, using cached list', e);
            }
            renderTenantList();
        };

        btn.addEventListener('click', () => {
            if (errEl) errEl.style.display = 'none';
            refreshAndRender();
            if (!btn.getAttribute('data-bs-toggle') && typeof bootstrap !== 'undefined') {
                const modal = bootstrap.Modal.getOrCreateInstance(modalEl);
                modal.show();
            }
        });

        modalEl.addEventListener('show.bs.modal', () => {
            if (errEl) errEl.style.display = 'none';
            refreshAndRender();
        });

        const handleAddTenant = () => {
            const modalInstance = typeof bootstrap !== 'undefined'
                ? bootstrap.Modal.getInstance(modalEl)
                : null;
            if (modalInstance) modalInstance.hide();
            if (typeof Router !== 'undefined' && Router.go) { Router.go('/setup-tenant'); } else { window.location.href = '/setup-tenant'; }
        };

        if (addBtn) {
            addBtn.addEventListener('click', handleAddTenant);
        }

        modalEl.addEventListener('shown.bs.modal', () => {
            const modalAddBtn = document.getElementById('tenantSwitchAddTenant');
            if (modalAddBtn && !modalAddBtn.dataset.bound) {
                modalAddBtn.addEventListener('click', handleAddTenant);
                modalAddBtn.dataset.bound = 'true';
            }
        });

        confirmBtn.onclick = async () => {
            const selected = modalEl.querySelector('input[name="tenantSwitch"]:checked');
            if (!selected) {
                if (errEl) {
                    errEl.textContent = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('tenantSwitch.pleaseSelect') : '請選擇企業';
                    errEl.style.display = '';
                }
                return;
            }
            try {
                const resp = await this.apiRequest('/user/select-tenant', {
                    method: 'POST',
                    body: JSON.stringify({ tenant_id: selected.value })
                });
                if (resp.token) {
                    localStorage.setItem('auth_token', resp.token);
                }
                if (resp.tenant) {
                    if (resp.tenant.subdomain) localStorage.setItem('tenant_subdomain', resp.tenant.subdomain);
                    if (resp.tenant.id) localStorage.setItem('tenant_id', resp.tenant.id);
                    if (resp.tenant.name) localStorage.setItem('tenant_name', resp.tenant.name);
                }
                if (nameEl && resp.tenant) {
                    nameEl.textContent = resp.tenant.name || resp.tenant.subdomain || ((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('nav.enterprises') : '企業');
                }
                const modalInstance = bootstrap.Modal.getInstance(modalEl);
                if (modalInstance) modalInstance.hide();
                window.location.reload();
            } catch (e) {
                if (errEl) {
                    errEl.textContent = e?.message || ((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('tenantSwitch.switchFailed') : '切換租戶失敗');
                    errEl.style.display = '';
                }
            }
        };
    },

    // 清除會話並跳轉到首頁
    clearSessionAndRedirect: function() {
        // 保留語系設定
        const preserved = {};
        const preserveKeys = ['u-nai_lang', 'language', 'nwork_lang'];
        try {
            preserveKeys.forEach((k) => {
                const v = localStorage.getItem(k);
                if (v !== null && v !== undefined && String(v).trim() !== '') {
                    preserved[k] = String(v);
                }
            });
        } catch (_e) {
            // ignore
        }

        // 清除所有 cookies
        const cookies = document.cookie.split(';');
        for (let i = 0; i < cookies.length; i++) {
            const cookie = cookies[i];
            const eqPos = cookie.indexOf('=');
            const name = eqPos > -1 ? cookie.substr(0, eqPos).trim() : cookie.trim();
            if (name === 'auth_token' || name.startsWith('auth_token')) {
                document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/';
                document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;domain=' + window.location.hostname;
                document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;SameSite=Lax';
                document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;SameSite=None;Secure';
            }
        }
        
        // 清除 localStorage
        localStorage.clear();
        // 清除 cookie-auth session 標記
        try { sessionStorage.removeItem('_cookie_auth'); } catch (_e) {}

        // 還原語系
        try {
            Object.keys(preserved).forEach((k) => {
                localStorage.setItem(k, preserved[k]);
            });
        } catch (_e) {
            // ignore
        }
        
        // 跳轉到服務首頁（會話不完整時）
        var homeUrl = App.getServiceHomeUrl();
        window.location.replace(homeUrl + (homeUrl.includes('?') ? '&' : '?') + 'logged_out=1&t=' + Date.now());
    },

    // 登出
    logout: function() {
        // 保留語系設定（登出不要刪除語系）
        // - I18n 使用 u-nai_lang
        // - CMS 歷史上也會用 language
        // - dynamic-list 等少數地方也會讀 nwork_lang
        const preserved = {};
        const preserveKeys = ['u-nai_lang', 'language', 'nwork_lang'];
        try {
            preserveKeys.forEach((k) => {
                const v = localStorage.getItem(k);
                if (v !== null && v !== undefined && String(v).trim() !== '') {
                    preserved[k] = String(v);
                }
            });
        } catch (_e) {
            // ignore
        }

        // Determine redirect URL: service home page + logged_out flag
        var homeUrl = App.getServiceHomeUrl();
        var redirectUrl = homeUrl + (homeUrl.includes('?') ? '&' : '?') + 'logged_out=1&t=' + Date.now();

        // Update app-switcher links to logged-out state before redirect
        if (typeof App.updateAppSwitcherLinks === 'function') {
            App.updateAppSwitcherLinks(false);
        }

        // Helper: clear client-side state and redirect
        function finishLogout() {
            // 先清除所有可能的 cookies（多種方式確保清除）
            const cookies = document.cookie.split(';');
            for (let i = 0; i < cookies.length; i++) {
                const cookie = cookies[i];
                const eqPos = cookie.indexOf('=');
                const name = eqPos > -1 ? cookie.substr(0, eqPos).trim() : cookie.trim();
                if (name === 'auth_token' || name.startsWith('auth_token')) {
                    // 清除 cookie（多種路徑和域名）
                    document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/';
                    document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;domain=' + window.location.hostname;
                    document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;SameSite=Lax';
                    document.cookie = name + '=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;SameSite=None;Secure';
                }
            }
            
            // 清除 localStorage
            localStorage.clear();
            // 清除 cookie-auth session 標記
            try { sessionStorage.removeItem('_cookie_auth'); } catch (_e) {}

            // 還原語系
            try {
                Object.keys(preserved).forEach((k) => {
                    localStorage.setItem(k, preserved[k]);
                });
            } catch (_e) {
                // ignore
            }
            
            // 強制跳轉到服務首頁，使用 replace 避免返回，並添加時間戳防止緩存
            window.location.replace(redirectUrl);
        }

        // Call server-side logout to clear HTTPOnly cookie, then finish client-side cleanup
        try {
            fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' })
                .then(function() { finishLogout(); })
                .catch(function() { finishLogout(); });
        } catch (_e) {
            finishLogout();
        }
    },

    // 顯示 loading 提示
    showLoading: function(message = null) {
        if (!message) {
            message = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...';
        }
        // 檢查是否已經存在 loading overlay
        let loadingOverlay = document.getElementById('appLoadingOverlay');
        if (loadingOverlay) {
            // 更新消息
            const messageEl = loadingOverlay.querySelector('.loading-message');
            if (messageEl) {
                messageEl.textContent = message;
            }
            loadingOverlay.style.display = 'flex';
            return;
        }

        // 創建 loading overlay
        loadingOverlay = document.createElement('div');
        loadingOverlay.id = 'appLoadingOverlay';
        loadingOverlay.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.5);
            display: flex;
            justify-content: center;
            align-items: center;
            z-index: 9999;
            flex-direction: column;
        `;
        loadingOverlay.innerHTML = `
            <div style="background: white; padding: 20px; border-radius: 8px; text-align: center; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);">
                <div class="spinner-border text-primary" role="status" style="width: 3rem; height: 3rem;">
                    <span class="visually-hidden">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : 'Loading...'}</span>
                </div>
                <p class="mt-3 mb-0 loading-message">${message}</p>
            </div>
        `;
        
        document.body.appendChild(loadingOverlay);
    },

    // 隱藏 loading 提示
    hideLoading: function() {
        const loadingOverlay = document.getElementById('appLoadingOverlay');
        if (loadingOverlay) {
            loadingOverlay.style.display = 'none';
        }
    },

    // 初始化所有 readonly 和 disabled 字段的 tooltip
    initTooltips: function() {
        if (typeof bootstrap === 'undefined' || !bootstrap.Tooltip) {
            return;
        }
        
        // 在整個文檔中查找所有 readonly 和 disabled 字段
        const readonlyElements = document.querySelectorAll('[readonly], [disabled]');
        readonlyElements.forEach(el => {
            // 跳過按鈕和不需要 tooltip 的元素
            if (el.type === 'button' || el.tagName === 'BUTTON') {
                return;
            }
            if (el.id && (el.id.endsWith('_uploadBtn') || el.id.endsWith('_removeBtn'))) {
                return;
            }
            
            // 如果字段是 readonly 或 disabled，但還沒有 tooltip 屬性，則添加
            if ((el.hasAttribute('readonly') || el.hasAttribute('disabled')) && !el.hasAttribute('data-bs-toggle')) {
                const message = el.hasAttribute('readonly') ? '此欄位為只讀，無法編輯' : '此欄位已禁用，無法編輯';
                el.setAttribute('data-bs-toggle', 'tooltip');
                el.setAttribute('data-bs-placement', 'top');
                el.setAttribute('title', message);
            }
            
            // 如果元素有 tooltip 屬性，初始化 tooltip
            if (el.hasAttribute('data-bs-toggle') && el.getAttribute('data-bs-toggle') === 'tooltip') {
                // 如果已經有 tooltip 實例，先銷毀
                const existingTooltip = bootstrap.Tooltip.getInstance(el);
                if (existingTooltip) {
                    existingTooltip.dispose();
                }
                // 創建新的 tooltip 實例
                try {
                    new bootstrap.Tooltip(el);
                } catch (e) {
                    console.warn('初始化 tooltip 失敗:', e);
                }
            }
        });
    }
};

// 頁面加載時檢查認證（僅在需要認證的頁面）
// 公開頁面：/, /login, /help, /contact, /subscription-required 不需要認證
// 需要認證的頁面：/dashboard 等
(function() {
    const publicPages = ['/', '/login', '/reset-password', '/subscription-required', '/tutorial/run', '/sales-partner', '/enterprise-custom', '/setup-tenant', '/profile-guide'];
    const publicPagePrefixes = ['/help', '/contact', '/tutorial', '/sales-partner', '/vwork-blog', '/vwork-events', '/industry/', '/custom/', '/co/', '/vmarket'];
    const currentPath = window.location.pathname;
    const hostname = window.location.hostname.toLowerCase();
    // vmarketai.com 的公開頁面（不帶 /vmarket 前綴）不需要認證
    const isVMarketDomain = hostname === 'www.vmarketai.com' || hostname === 'vmarketai.com';

    // 檢查是否是公開頁面（完全匹配或前綴匹配）
    const isPublicPage = isVMarketDomain ||
                         publicPages.includes(currentPath) || 
                         publicPagePrefixes.some(prefix => currentPath.startsWith(prefix));

    // 只在非公開頁面時才檢查認證
    if (!isPublicPage) {
        document.addEventListener('DOMContentLoaded', function() {
            // 再次檢查路徑（防止在 DOMContentLoaded 時路徑已改變）
            const path = window.location.pathname;
            if (path === '/reset-password' || path === '/subscription-required' || path === '/enterprise-custom' || path === '/setup-tenant' || path === '/profile-guide' || path.startsWith('/help') || path.startsWith('/contact') || path.startsWith('/tutorial') || path.startsWith('/sales-partner') || path.startsWith('/vwork-blog') || path.startsWith('/vwork-events') || path.startsWith('/industry/') || path.startsWith('/custom/') || path.startsWith('/co/') || path.startsWith('/vmarket') || path === '/' || path === '/login') {
                return; // 公開頁面，不檢查認證
            }
            if (typeof App !== 'undefined' && App.checkAuth) {
                App.checkAuth();
            }
            // 初始化所有 readonly 和 disabled 字段的 tooltip
            if (typeof App !== 'undefined' && App.initTooltips) {
                App.initTooltips();
            }
            // 初始化租戶切換
            if (typeof App !== 'undefined' && App.initTenantSwitcher) {
                App.initTenantSwitcher();
            }
        });
    }
})();

// 全局初始化 tooltip（適用於所有頁面，包括公開頁面）
document.addEventListener('DOMContentLoaded', function() {
    // 延遲初始化，確保所有動態內容都已加載
    setTimeout(() => {
        App.initTooltips();
    }, 500);
    
    // 使用 MutationObserver 監聽 DOM 變化，當有新元素添加時自動初始化 tooltip
    const observer = new MutationObserver(function(mutations) {
        let shouldInit = false;
        mutations.forEach(function(mutation) {
            if (mutation.addedNodes.length > 0) {
                mutation.addedNodes.forEach(function(node) {
                    if (node.nodeType === 1) { // Element node
                        // 檢查是否有 readonly 或 disabled 屬性
                        if (node.hasAttribute && (node.hasAttribute('readonly') || node.hasAttribute('disabled'))) {
                            shouldInit = true;
                        }
                        // 檢查子元素
                        if (node.querySelectorAll) {
                            const readonlyChildren = node.querySelectorAll('[readonly], [disabled]');
                            if (readonlyChildren.length > 0) {
                                shouldInit = true;
                            }
                        }
                    }
                });
            }
        });
        
        if (shouldInit) {
            // 防抖：避免頻繁初始化
            clearTimeout(window.tooltipInitTimeout);
            window.tooltipInitTimeout = setTimeout(() => {
                App.initTooltips();
            }, 200);
        }
    });
    
    // 開始觀察整個文檔的變化
    observer.observe(document.body, {
        childList: true,
        subtree: true,
        attributes: true,
        attributeFilter: ['readonly', 'disabled']
    });
});

// 全局函數：顯示訂閱彈窗（可在 console 調用）
window.showSubscriptionPopup = function() {
    // 如果已經在訂閱頁面，刷新即可
    if (window.location.pathname === '/subscription-required') {
        window.location.reload();
    } else {
        if (typeof Router !== 'undefined' && Router.go) { Router.go('/subscription-required'); } else { window.location.href = '/subscription-required'; }
    }
};

// Tools section - no longer uses carousel

function openDiningMenuPreview(event) {
    if (event && event.preventDefault) event.preventDefault();
    const subdomain = (typeof window !== 'undefined' && window.tenantSubdomain) ? window.tenantSubdomain : null;
    const storedSubdomain = localStorage.getItem('tenant_subdomain');
    const finalSubdomain = subdomain || storedSubdomain;
    if (!finalSubdomain) {
        if (typeof App !== 'undefined' && App.showError) {
            App.showError('請先完成企業子域名設定');
        }
        return false;
    }
    const url = `/co/${encodeURIComponent(finalSubdomain)}/menu/`;
    window.open(url, '_blank');
    return false;
}

window.openDiningMenuPreview = openDiningMenuPreview;

// 页面加载时执行
document.addEventListener('DOMContentLoaded', function() {
    // Detect ?logged_out=1 param and show alert
    try {
        var params = new URLSearchParams(window.location.search);
        if (params.get('logged_out') === '1') {
            // Clean up the URL (remove logged_out and t params)
            params.delete('logged_out');
            params.delete('t');
            var cleanUrl = window.location.pathname + (params.toString() ? '?' + params.toString() : '') + window.location.hash;
            window.history.replaceState({}, '', cleanUrl);

            // Show alert — use App.showAlert if alertContainer exists, otherwise create one
            var container = document.getElementById('alertContainer');
            if (!container) {
                container = document.createElement('div');
                container.id = 'alertContainer';
                container.style.cssText = 'position:fixed;top:24px;left:50%;transform:translateX(-50%);z-index:9999;max-width:500px;width:90%;';
                document.body.appendChild(container);
            }
            var msg = '已成功登出';
            if (typeof I18n !== 'undefined' && I18n.currentLang === 'en') {
                msg = 'You have been logged out';
            }
            if (typeof App !== 'undefined' && App.showAlert) {
                App.showAlert(msg, 'success');
            } else {
                var alertEl = document.createElement('div');
                alertEl.className = 'alert alert-success alert-dismissible fade show';
                alertEl.innerHTML = msg + '<button type="button" class="btn-close" data-bs-dismiss="alert"></button>';
                container.innerHTML = '';
                container.appendChild(alertEl);
                setTimeout(function() { alertEl.remove(); }, 3000);
            }
        }
    } catch (_e) { /* ignore */ }
});

// ─── App Switcher: update links based on auth state & environment ─────
// On production: replace relative /?domain=xxx links with actual product domains
//                and add data-sso-link for cross-domain SSO.
// On localhost:   when logged in, point to product backend pages;
//                 when logged out, point to product homepages.
// This function is also called by checkAuthAndUpdateNav() in each layout
// template, so app-switcher links stay in sync after login/logout.
App.updateAppSwitcherLinks = function(isLoggedIn) {
    try {
        var hostname = window.location.hostname.toLowerCase();
        var isLocal = hostname === 'localhost' || hostname === '127.0.0.1' ||
                      hostname.startsWith('localhost:') || hostname.startsWith('127.0.0.1:');

        // Auto-detect login state if not explicitly provided
        if (typeof isLoggedIn === 'undefined') {
            var token = localStorage.getItem('auth_token');
            isLoggedIn = !!(token && token !== 'temp_token');
        }

        if (isLocal) {
            // Localhost: all products share the same domain.
            // When logged in → link to backend pages directly.
            // When logged out → link to product homepages.
            var localLoggedInMap = {
                'vai':     '/vai-chat',
                'vwork':   '/dashboard',
                'vmarket': '/?domain=vmarket',
                'voffice': '/?domain=voffice'
            };
            var localLoggedOutMap = {
                'vai':     '/?domain=vai',
                'vwork':   '/',
                'vmarket': '/?domain=vmarket',
                'voffice': '/?domain=voffice'
            };
            var localMap = isLoggedIn ? localLoggedInMap : localLoggedOutMap;

            document.querySelectorAll('a[data-app]').forEach(function(link) {
                var app = link.getAttribute('data-app');
                if (localMap[app]) {
                    link.href = localMap[app];
                    // No SSO needed on localhost (same domain)
                    link.removeAttribute('data-sso-link');
                }
            });
        } else {
            // Production: cross-domain SSO links
            // vWork/vAI: logged in → backend page; logged out → homepage.
            // vOffice/vMarket: always homepage (the homepage IS the product).
            var prodLoggedInMap = {
                'vai':     'https://vai.vsysai.com/vai-chat',
                'vwork':   'https://www.vworkai.com/dashboard',
                'vmarket': 'https://www.vmarketai.com',
                'voffice': 'https://voffice.vsysai.com'
            };
            var prodLoggedOutMap = {
                'vai':     'https://vai.vsysai.com',
                'vwork':   'https://www.vworkai.com',
                'vmarket': 'https://www.vmarketai.com',
                'voffice': 'https://voffice.vsysai.com'
            };
            var prodUrlMap = isLoggedIn ? prodLoggedInMap : prodLoggedOutMap;

            document.querySelectorAll('a[data-app]').forEach(function(link) {
                var app = link.getAttribute('data-app');
                if (prodUrlMap[app]) {
                    link.href = prodUrlMap[app];
                    link.setAttribute('data-sso-link', '');
                }
            });
        }

        // Footer links: logo & website → vsys homepage
        var vsysUrl = isLocal ? 'http://localhost:3001/?domain=vsys' : 'https://www.vsysai.com';
        var footerWebsite = document.getElementById('vworkFooterWebsiteLink');
        if (footerWebsite) { footerWebsite.href = vsysUrl; footerWebsite.setAttribute('data-sso-link', ''); }
        var footerLogo = document.getElementById('footerLogoLink');
        if (footerLogo) { footerLogo.href = vsysUrl; footerLogo.setAttribute('data-sso-link', ''); }

        // Footer product links: use the same URL map as the app-switcher
        var footerUrlMap = isLocal ? {
            'vai':     'http://localhost:3001/?domain=vai',
            'vwork':   'http://localhost:3001/?domain=vwork',
            'vmarket': 'http://localhost:3001/?domain=vmarket',
            'voffice': 'http://localhost:3001/?domain=voffice'
        } : {
            'vai':     'https://vai.vsysai.com',
            'vwork':   'https://www.vworkai.com',
            'vmarket': 'https://www.vmarketai.com',
            'voffice': 'https://voffice.vsysai.com'
        };
        var footerIds = {
            'vai':     'footerProductVAI',
            'vwork':   'footerProductVWork',
            'vmarket': 'footerProductVMarket',
            'voffice': 'footerProductVOffice'
        };
        for (var key in footerIds) {
            var el = document.getElementById(footerIds[key]);
            if (el) { el.href = footerUrlMap[key]; if (!isLocal) el.setAttribute('data-sso-link', ''); }
        }

        // Additional footer/brand/nav link IDs used by some pages
        var extraVsysIds = ['topnavLogoLink', 'footerWebsiteLink', 'navHomeLink', 'sidebarHomeLink', 'footerInfoHome', 'footerHomeLink'];
        for (var i = 0; i < extraVsysIds.length; i++) {
            var el2 = document.getElementById(extraVsysIds[i]);
            if (el2) { el2.href = vsysUrl; if (!isLocal) el2.setAttribute('data-sso-link', ''); }
        }
        // Links pointing to vsys homepage #about section
        var aboutIds = ['navAboutLink', 'sidebarAboutLink', 'footerInfoAbout', 'footerAboutLink'];
        for (var ai = 0; ai < aboutIds.length; ai++) {
            var ael = document.getElementById(aboutIds[ai]);
            if (ael) { ael.href = vsysUrl + '#about'; if (!isLocal) ael.setAttribute('data-sso-link', ''); }
        }
        var extraProductIds = {
            'footerVWorkLink': footerUrlMap['vwork'],
            'vmarketBrandLink': footerUrlMap['vmarket'],
            'navBrandLink': footerUrlMap['vai']
        };
        for (var id in extraProductIds) {
            var el3 = document.getElementById(id);
            if (el3) { el3.href = extraProductIds[id]; if (!isLocal) el3.setAttribute('data-sso-link', ''); }
        }

        // Product card links (vsys homepage)
        var cardIds = {
            'productCardVai':     footerUrlMap['vai'],
            'productCardVwork':   footerUrlMap['vwork'],
            'productCardVmarket': footerUrlMap['vmarket'],
            'productCardVoffice': footerUrlMap['voffice']
        };
        for (var cid in cardIds) {
            var cel = document.getElementById(cid);
            if (cel) { cel.href = cardIds[cid]; if (!isLocal) cel.setAttribute('data-sso-link', ''); }
        }
    } catch (_e) { /* ignore */ }
};

// Initial update on page load
document.addEventListener('DOMContentLoaded', function() {
    App.updateAppSwitcherLinks();
});

// Cross-tab logout detection: when another tab clears auth_token from localStorage,
// this tab should also redirect to the login page (only on protected pages).
window.addEventListener('storage', function(e) {
    // Skip redirect on public pages — they handle auth state via their own UI updates
    var path = window.location.pathname;
    var _host = window.location.hostname;
    var isVMarketDomain = _host === 'www.vmarketai.com' || _host === 'vmarketai.com';
    var isPublicPage = isVMarketDomain ||
                       path === '/' || path === '/login' || path === '/reset-password' ||
                       path === '/subscription-required' || path === '/sales-partner' || path === '/enterprise-custom' ||
                       (path.startsWith && (path.startsWith('/help') || path.startsWith('/contact') ||
                        path.startsWith('/static/') || path.startsWith('/sales-partner') ||
                        path.startsWith('/vwork-blog') || path.startsWith('/vwork-events') || path.startsWith('/vmarket') || path.startsWith('/co/') ||
                        path.startsWith('/industry/') || path.startsWith('/custom/')));
    if (isPublicPage) {
        return; // Public pages manage their own nav state; no redirect needed
    }

    if (e.key === 'auth_token' && !e.newValue) {
        // auth_token was removed in another tab — redirect to service home
        // Call server-side logout to also clear the HTTPOnly cookie
        var homeUrl = (typeof App !== 'undefined' && App.getServiceHomeUrl) ? App.getServiceHomeUrl() : '/';
        var redir = homeUrl + (homeUrl.includes('?') ? '&' : '?') + 'logged_out=1&t=' + Date.now();
        try {
            fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' })
                .finally(function() {
                    window.location.replace(redir);
                });
        } catch (_e) {
            window.location.replace(redir);
        }
    }
    // Also handle localStorage.clear() — key will be null
    if (e.key === null) {
        var homeUrl2 = (typeof App !== 'undefined' && App.getServiceHomeUrl) ? App.getServiceHomeUrl() : '/';
        var redir2 = homeUrl2 + (homeUrl2.includes('?') ? '&' : '?') + 'logged_out=1&t=' + Date.now();
        try {
            fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' })
                .finally(function() {
                    window.location.replace(redir2);
                });
        } catch (_e) {
            window.location.replace(redir2);
        }
    }
});

// ─── Invite modal (global, usable from dashboard + /users page) ──────
(function() {
    function _t(key, fallback) {
        try {
            if (typeof I18n === 'undefined' || !I18n.t) return (fallback ?? key);
            var v = I18n.t(key);
            if (!v || v === key) return (fallback ?? key);
            return v;
        } catch (_e) { return (fallback ?? key); }
    }

    function _parseEmails(text) {
        return text.split(/[\n,;]+/)
            .map(function(e) { return e.trim().toLowerCase(); })
            .filter(function(e) { return e && e.includes('@') && e.includes('.'); });
    }

    window.openInviteModal = function openInviteModal() {
        var existing = document.getElementById('inviteModal');
        if (existing) existing.remove();

        var modalHTML =
            '<div class="modal fade" id="inviteModal" tabindex="-1" aria-hidden="true">' +
            '<div class="modal-dialog modal-dialog-centered">' +
            '<div class="modal-content">' +
            '<div class="modal-header">' +
            '<h5 class="modal-title"><i class="bi bi-envelope-plus me-1"></i> ' + _t('invitePopup.title', '邀請成員加入團隊') + '</h5>' +
            '<button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>' +
            '</div>' +
            '<div class="modal-body">' +
            '<p class="text-muted small">' + _t('invitePopup.description', '輸入電子郵件地址，每行一個或用逗號分隔（最多 20 個）。') + '</p>' +
            '<textarea class="form-control" id="inviteEmails" rows="5" placeholder="' + _t('invitePopup.emailPlaceholder', 'email1@example.com') + '"></textarea>' +
            '<div class="text-muted small mt-1" id="inviteEmailCount">0 / 20</div>' +
            '<div id="inviteResults" class="mt-3" style="display:none;"></div>' +
            '</div>' +
            '<div class="modal-footer">' +
            '<button type="button" class="btn btn-secondary" data-bs-dismiss="modal">' + _t('common.cancel', '取消') + '</button>' +
            '<button type="button" class="btn btn-primary" id="inviteSendBtn"><i class="bi bi-send me-1"></i> ' + _t('invitePopup.sendBtn', '發送邀請') + '</button>' +
            '</div></div></div></div>';

        document.body.insertAdjacentHTML('beforeend', modalHTML);

        var modalEl = document.getElementById('inviteModal');
        var bsModal = new bootstrap.Modal(modalEl);

        var textarea = document.getElementById('inviteEmails');
        var counter = document.getElementById('inviteEmailCount');
        textarea.addEventListener('input', function() {
            var emails = _parseEmails(textarea.value);
            counter.textContent = emails.length + ' / 20';
            counter.className = emails.length > 20 ? 'text-danger small mt-1' : 'text-muted small mt-1';
        });

        document.getElementById('inviteSendBtn').addEventListener('click', async function() {
            var emails = _parseEmails(textarea.value);
            if (emails.length === 0) {
                App.showAlert(_t('invitePopup.noEmails', '請輸入至少一個電子郵件地址'), 'warning');
                return;
            }
            if (emails.length > 20) {
                App.showAlert(_t('invitePopup.maxEmails', '每次最多邀請 20 人'), 'warning');
                return;
            }

            var btn = document.getElementById('inviteSendBtn');
            var origHTML = btn.innerHTML;
            btn.disabled = true;
            btn.innerHTML = '<span class="spinner-border spinner-border-sm"></span>';

            try {
                var data = await App.apiRequest('/tenant-invitations', {
                    method: 'POST',
                    body: JSON.stringify({ emails: emails })
                });
                btn.disabled = false;
                btn.innerHTML = origHTML;

                var resultsDiv = document.getElementById('inviteResults');
                if (data.results && data.results.length > 0) {
                    var statusIcons = {
                        'sent': '<i class="bi bi-check-circle-fill text-success me-1"></i>',
                        'already_member': '<i class="bi bi-info-circle-fill text-info me-1"></i>',
                        'error': '<i class="bi bi-x-circle-fill text-danger me-1"></i>'
                    };
                    var statusTexts = {
                        'sent': _t('invitePopup.resultSent', '已發送'),
                        'already_member': _t('invitePopup.resultAlreadyMember', '已是成員'),
                        'error': _t('invitePopup.resultError', '發送失敗')
                    };
                    resultsDiv.innerHTML = data.results.map(function(r) {
                        var icon = statusIcons[r.status] || '';
                        var text = statusTexts[r.status] || r.status;
                        return '<div class="d-flex justify-content-between align-items-center py-1 border-bottom">' +
                            '<span class="small">' + r.email + '</span>' +
                            '<span class="small">' + icon + text + '</span></div>';
                    }).join('');
                    resultsDiv.style.display = '';
                    textarea.value = '';
                    counter.textContent = '0 / 20';
                }
            } catch (err) {
                btn.disabled = false;
                btn.innerHTML = origHTML;
                App.showAlert(err.message || _t('invitePopup.sendFailed', '發送失敗'), 'danger');
            }
        });

        modalEl.addEventListener('hidden.bs.modal', function() { modalEl.remove(); });
        bsModal.show();
    };
})();
