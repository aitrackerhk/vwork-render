// 認證相關 JavaScript

// ─── Product-aware redirect (SSO to vOffice / vMarket) ─────────────
// Use the shared PRODUCT_REDIRECT_URLS from app.js when available;
// fall back to a local copy so auth.js still works standalone.
const PRODUCT_URLS = (typeof PRODUCT_REDIRECT_URLS !== 'undefined')
    ? PRODUCT_REDIRECT_URLS
    : {
        vwork:   'https://www.vworkai.com/dashboard',
        vai:     'https://vai.vsysai.com/vai-chat',
        voffice: 'https://voffice.vsysai.com/voffice-download',
        vmarket: 'https://www.vmarketai.com/vmarket-search',
    };

/**
 * Redirect to the product the user selected in the login product-selector.
 *
 * Key design: if the user is already on the target product's domain,
 * redirect with a relative path (same-origin) so that the auth cookie
 * set during login stays valid.  Only use a cross-domain absolute URL
 * (via SSO) when the user explicitly picked a different product than
 * the domain they're on.
 *
 * vOffice Desktop App flow:
 *   If sessionStorage contains 'voffice_redirect_uri' (set by login.html when
 *   ?redirect_uri=http://localhost:PORT/oauth/callback is present), we generate
 *   an SSO ticket and redirect to that URI with ?ticket=xxx so the desktop app's
 *   local HTTP server can pick it up.
 */
function redirectToProduct() {
    // Check for vOffice desktop app redirect_uri
    const redirectUri = sessionStorage.getItem('voffice_redirect_uri');
    if (redirectUri) {
        sessionStorage.removeItem('voffice_redirect_uri');
        _handleDesktopAppRedirect(redirectUri);
        return;
    }

    // Use the domain-aware helper from app.js when available
    const url = (typeof getProductRedirectUrl === 'function')
        ? getProductRedirectUrl()
        : _legacyProductUrl();

    // Relative path → same-origin, just navigate
    if (url.startsWith('/')) {
        window.location.href = url;
        return;
    }

    // Cross-domain: use SSO.navigateTo() if available
    if (typeof SSO !== 'undefined' && SSO.navigateTo) {
        SSO.navigateTo(url);
    } else {
        window.location.href = url;
    }
}

/**
 * Fallback when getProductRedirectUrl is not available (app.js not loaded).
 * Tries to stay on the current domain if possible.
 */
function _legacyProductUrl() {
    const product = (localStorage.getItem('vlogin_product') || 'vwork').toLowerCase();
    const host = window.location.hostname.toLowerCase();

    // v00(2026-03-09): Fix for local dev multi-product redirect.
    // If on localhost, use relative paths based on selected product.
    if (host === 'localhost' || host === '127.0.0.1') {
        const localPaths = {
            'vwork':   '/dashboard',
            'vai':     '/vai-chat',
            'voffice': '/voffice-download',
            'vmarket': '/vmarket-search',
        };
        return localPaths[product] || '/dashboard';
    }

    // If we can detect we're already on the right domain, use relative path
    const domainMap = {
        'www.vworkai.com':    { product: 'vwork',   path: '/dashboard' },
        'vworkai.com':        { product: 'vwork',   path: '/dashboard' },
        'vai.vsysai.com':     { product: 'vai',     path: '/vai-chat' },
        'voffice.vsysai.com': { product: 'voffice', path: '/voffice-download' },
        'www.vmarketai.com':  { product: 'vmarket', path: '/vmarket-search' },
        'vmarketai.com':      { product: 'vmarket', path: '/vmarket-search' },
    };

    const current = domainMap[host];
    if (current) {
        // User is on a known domain — stay here if product matches or default
        if (current.product === product || product === 'vwork') {
            return current.path;
        }
    }

    // Fall back to absolute URL for cross-domain
    return PRODUCT_URLS[product] || '/dashboard';
}

/**
 * Handle redirect back to vOffice desktop app via SSO ticket.
 * Generates a one-time SSO ticket and redirects to the desktop app's
 * local HTTP server callback URL with the ticket as a query parameter.
 *
 * @param {string} redirectUri - The desktop app's local callback URL
 *                               (e.g. http://localhost:9876/oauth/callback)
 */
async function _handleDesktopAppRedirect(redirectUri) {
    const token = localStorage.getItem('auth_token');
    if (!token || token === 'temp_token') {
        // No valid token, fall back to normal redirect
        window.location.href = '/dashboard';
        return;
    }

    try {
        // Request SSO ticket from backend
        const subdomain = localStorage.getItem('tenant_subdomain') || '';
        const resp = await fetch('/api/v1/sso/ticket', {
            method: 'POST',
            headers: {
                'Authorization': 'Bearer ' + token,
                'Content-Type': 'application/json',
                'X-Tenant-Subdomain': subdomain,
            },
        });

        if (!resp.ok) {
            console.error('[vOffice] Failed to generate SSO ticket:', resp.status);
            window.location.href = '/dashboard';
            return;
        }

        const data = await resp.json();
        const ticket = data.ticket;

        if (!ticket) {
            console.error('[vOffice] Empty SSO ticket');
            window.location.href = '/dashboard';
            return;
        }

        // Redirect to desktop app's local server with ticket
        const sep = redirectUri.includes('?') ? '&' : '?';
        const callbackUrl = redirectUri + sep + 'ticket=' + encodeURIComponent(ticket);

        // Show success message briefly before redirecting
        if (typeof App !== 'undefined' && App.showAlert) {
            App.showAlert('登入成功！正在返回 vOffice...', 'success');
        }

        setTimeout(() => {
            window.location.href = callbackUrl;
        }, 500);
    } catch (err) {
        console.error('[vOffice] SSO ticket error:', err);
        window.location.href = '/dashboard';
    }
}

document.addEventListener('DOMContentLoaded', function() {
    // i18n helper (I18n.t returns key when missing)
    const t = (key, fallback) => {
        try {
            if (typeof I18n === 'undefined' || !I18n.t) return (fallback ?? key);
            const v = I18n.t(key);
            if (!v || v === key) return (fallback ?? key);
            return v;
        } catch (_e) {
            return (fallback ?? key);
        }
    };

    // Helper: reveal the login page (remove the CSS opacity:0 cloak)
    function revealLoginPage() {
        const wrapper = document.querySelector('.vlogin-wrapper');
        if (wrapper) wrapper.classList.add('vlogin-ready');
    }

    // Helper: clear auth-related localStorage keys (preserve preferences like language)
    function clearAuthStorage() {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('user');
        localStorage.removeItem('tenant_id');
        localStorage.removeItem('tenant_subdomain');
        localStorage.removeItem('tenant_name');
        localStorage.removeItem('industry_template_selected');
        localStorage.removeItem('industry_template_skipped');
    }

    // Helper: decode JWT payload and check if it's still valid (not expired).
    // Returns true if the token has a valid exp claim that is in the future.
    // This is a LOCAL-ONLY check — no network round-trip, so no flash/delay.
    function isTokenLocallyValid(token) {
        try {
            // JWT = header.payload.signature — we only need the payload
            const parts = token.split('.');
            if (parts.length !== 3) return false;
            // base64url → base64 → decode
            const payload = JSON.parse(atob(parts[1].replace(/-/g, '+').replace(/_/g, '/')));
            if (!payload || typeof payload.exp !== 'number') return false;
            // exp is in seconds; give 30s buffer to avoid edge-case race
            return payload.exp > (Date.now() / 1000) + 30;
        } catch (e) {
            return false;
        }
    }

    // 如果已經登錄，重定向到 dashboard
    // 注意：使用 JWT 本地 exp 判斷，不做網路請求，避免等待造成閃爍
    if (window.location.pathname === '/login') {
        const token = localStorage.getItem('auth_token');
        const user = localStorage.getItem('user');

        let shouldRedirect = false;

        if (token && user && token !== 'temp_token') {
            try {
                const userObj = JSON.parse(user);
                if (userObj && userObj.id && isTokenLocallyValid(token)) {
                    // Token 本地未過期，直接重定向（不顯示登入頁，零閃爍）
                    shouldRedirect = true;
                } else {
                    // Token 過期或 user 資料無效 → 清除，顯示登入頁
                    clearAuthStorage();
                }
            } catch (e) {
                clearAuthStorage();
            }
        } else if (token === 'temp_token') {
            localStorage.removeItem('auth_token');
        }

        if (shouldRedirect) {
            // 頁面保持 opacity:0，直接跳走
            checkIndustryTemplateAndRedirect();
            // 不 return — 下面的表單事件綁定仍需執行（以防 redirect 失敗或需要回退）
        } else {
            revealLoginPage();
        }
    } else {
        revealLoginPage();
    }
    const loginForm = document.getElementById('loginForm');
    const registerForm = document.getElementById('registerForm');
    const registerFormData = document.getElementById('registerFormData');
    const showRegister = document.getElementById('showRegister');
    const showLogin = document.getElementById('showLogin');
    const togglePassword = document.getElementById('togglePassword');
    const eyeIcon = document.getElementById('eyeIcon');

    // 切換顯示/隱藏密碼
    if (togglePassword) {
        togglePassword.addEventListener('click', function() {
            const passwordInput = document.getElementById('password');
            const type = passwordInput.getAttribute('type') === 'password' ? 'text' : 'password';
            passwordInput.setAttribute('type', type);
            eyeIcon.classList.toggle('bi-eye');
            eyeIcon.classList.toggle('bi-eye-slash');
        });
    }

    // 更新頂部按鈕顯示狀態
    function updateTopButtons(isRegisterVisible) {
        const topRegisterBtn = document.getElementById('topRegisterBtn');
        const topLoginBtn = document.getElementById('topLoginBtn');
        if (isRegisterVisible) {
            // 顯示註冊表單時，顯示登錄按鈕
            if (topRegisterBtn) topRegisterBtn.style.display = 'none';
            if (topLoginBtn) topLoginBtn.style.display = 'inline-block';
        } else {
            // 顯示登錄表單時，顯示註冊按鈕
            if (topRegisterBtn) topRegisterBtn.style.display = 'inline-block';
            if (topLoginBtn) topLoginBtn.style.display = 'none';
        }
    }

    // 顯示註冊表單
    if (showRegister) {
        showRegister.addEventListener('click', function(e) {
            e.preventDefault();
            // 跳轉到註冊 URL
            window.location.href = '/login?reg=1';
        });
    }

    // 顯示登錄表單
    if (showLogin) {
        showLogin.addEventListener('click', function(e) {
            e.preventDefault();
            // 跳轉回登錄 URL (移除 reg 參數)
            window.location.href = '/login';
        });
    }

    // 頂部註冊按鈕點擊事件
    const topRegisterBtn = document.getElementById('topRegisterBtn');
    if (topRegisterBtn && showRegister) {
        topRegisterBtn.addEventListener('click', function(e) {
            e.preventDefault();
            showRegister.click(); // 觸發原有的顯示註冊表單邏輯
        });
    }

    // 頂部登錄按鈕點擊事件
    const topLoginBtn = document.getElementById('topLoginBtn');
    if (topLoginBtn && showLogin) {
        topLoginBtn.addEventListener('click', function(e) {
            e.preventDefault();
            showLogin.click(); // 觸發原有的顯示登錄表單邏輯
        });
    }

    // 初始化頂部按鈕狀態（默認顯示登錄表單，所以顯示註冊按鈕）
    updateTopButtons(false);

    // 自動切換到註冊介面 (如果 URL 包含 ?reg=1)
    const urlParams = new URLSearchParams(window.location.search);
    if ((urlParams.get('reg') === '1' || urlParams.has('reg'))) {
        const loginHeader = document.getElementById('loginHeader');
        const registerHeader = document.getElementById('registerHeader');
        if (loginHeader) loginHeader.style.display = 'none';
        if (registerHeader) registerHeader.style.display = 'block';
        if (loginForm) loginForm.style.display = 'none';
        if (registerForm) registerForm.style.display = 'block';
        updateTopButtons(true); // 顯示註冊表單
    }

    function buildTenantSelectionModal() {
        const modalId = 'tenantSelectionModal';
        let modalEl = document.getElementById(modalId);
        if (modalEl) return modalEl;

        modalEl = document.createElement('div');
        modalEl.id = modalId;
        modalEl.className = 'modal fade';
        modalEl.tabIndex = -1;
        modalEl.innerHTML = `
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">${t('login.selectTenant', '選擇租戶')}</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                    </div>
                    <div class="modal-body">
                        <div class="text-muted mb-2">${t('login.selectTenantHint', '請選擇要進入的租戶')}</div>
                        <div id="tenantSelectionList" class="list-group"></div>
                        <div class="text-danger small mt-2" id="tenantSelectionError" style="display:none;"></div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">${t('common.cancel', '取消')}</button>
                        <button type="button" class="btn btn-primary" id="tenantSelectionConfirm">${t('common.confirm', '確認')}</button>
                    </div>
                </div>
            </div>
        `;
        document.body.appendChild(modalEl);
        return modalEl;
    }

    async function selectTenantAndProceed(tenantId, afterSelect) {
        try {
            const resp = await fetch('/api/v1/user/select-tenant', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${localStorage.getItem('auth_token') || ''}`
                },
                body: JSON.stringify({ tenant_id: tenantId })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                throw new Error(data.error || data.message || '選擇租戶失敗');
            }
            if (data.token) {
                localStorage.setItem('auth_token', data.token);
            }
            if (data.tenant) {
                if (data.tenant.subdomain) localStorage.setItem('tenant_subdomain', data.tenant.subdomain);
                if (data.tenant.id) localStorage.setItem('tenant_id', data.tenant.id);
                if (data.tenant.name) localStorage.setItem('tenant_name', data.tenant.name);
            }
            if (typeof afterSelect === 'function') afterSelect();
        } catch (e) {
            const errEl = document.getElementById('tenantSelectionError');
            if (errEl) {
                errEl.textContent = e?.message || '選擇租戶失敗';
                errEl.style.display = '';
            } else {
                App.showAlert(e?.message || '選擇租戶失敗', 'danger');
            }
        }
    }

    function openTenantSelectionModal(tenants, lastTenantId, afterSelect) {
        const modalEl = buildTenantSelectionModal();
        const listEl = modalEl.querySelector('#tenantSelectionList');
        const errEl = modalEl.querySelector('#tenantSelectionError');
        if (errEl) errEl.style.display = 'none';
        if (!listEl) return;

        const itemsHtml = (tenants || []).map(tn => {
            const isActive = lastTenantId && String(tn.id) === String(lastTenantId);
            return `
                <label class="list-group-item d-flex align-items-center gap-2">
                    <input class="form-check-input" type="radio" name="tenantSelection" value="${tn.id}" ${isActive ? 'checked' : ''}>
                    <div>
                        <div class="fw-bold">${tn.name || tn.subdomain || tn.id}</div>
                        <div class="text-muted small">${tn.subdomain || ''}</div>
                    </div>
                </label>
            `;
        }).join('');
        listEl.innerHTML = itemsHtml || `<div class="text-muted">${t('login.noTenants', '沒有可用租戶')}</div>`;

        const confirmBtn = modalEl.querySelector('#tenantSelectionConfirm');
        if (confirmBtn) {
            confirmBtn.onclick = async () => {
                const selected = modalEl.querySelector('input[name="tenantSelection"]:checked');
                if (!selected) {
                    if (errEl) {
                        errEl.textContent = t('login.selectTenantRequired', '請選擇租戶');
                        errEl.style.display = '';
                    }
                    return;
                }
                await selectTenantAndProceed(selected.value, () => {
                    const modalInstance = bootstrap.Modal.getInstance(modalEl);
                    if (modalInstance) modalInstance.hide();
                    if (typeof afterSelect === 'function') afterSelect();
                });
            };
        }

        const modal = new bootstrap.Modal(modalEl);
        modal.show();
    }

    // 登錄表單提交
    if (loginForm) {
        loginForm.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const submitBtn = loginForm.querySelector('button[type="submit"]');
            const originalText = submitBtn.innerHTML;
            submitBtn.disabled = true;
            submitBtn.innerHTML = '<span class="spinner-border spinner-border-sm"></span> 登錄中...';

            const emailInput = document.getElementById('email');
            const passwordInput = document.getElementById('password');
            
            if (!emailInput || !passwordInput) {
                App.showAlert('表單元素未找到，請刷新頁面重試', 'error');
                submitBtn.disabled = false;
                submitBtn.innerHTML = originalText;
                return;
            }
            
            const formData = {
                email: emailInput.value.trim(),
                password: passwordInput.value
            };
            
            if (!formData.email || !formData.password) {
                App.showAlert('請填寫完整的登錄信息', 'warning');
                submitBtn.disabled = false;
                submitBtn.innerHTML = originalText;
                return;
            }

            try {
                const response = await fetch('/api/v1/auth/login', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(formData)
                });

                let data;
                try {
                    data = await response.json();
                } catch (e) {
                    // 如果响应不是 JSON
                    const text = await response.text();
                    const errorMsg = text || ((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('login.errors.loginFailed', '登錄失敗，請檢查網絡連接') : '登錄失敗，請檢查網絡連接');
                    App.showAlert(errorMsg, 'danger');
                    submitBtn.disabled = false;
                    submitBtn.innerHTML = originalText;
                    return;
                }

                if (response.ok) {
                    // 保存認證信息
                    localStorage.setItem('auth_token', data.token || 'temp_token');
                    localStorage.setItem('user', JSON.stringify(data.user));
                    // Signal CMS to show vAi greet bubble after redirect
                    localStorage.setItem('vai_just_logged_in', '1');
                    // 從響應中獲取 tenant subdomain（如果有的話）
                    if (data.tenant && data.tenant.subdomain) {
                        localStorage.setItem('tenant_subdomain', data.tenant.subdomain);
                        if (data.tenant.id) localStorage.setItem('tenant_id', data.tenant.id);
                        if (data.tenant.name) localStorage.setItem('tenant_name', data.tenant.name);
                    } else if (data.user && data.user.tenant_subdomain) {
                        localStorage.setItem('tenant_subdomain', data.user.tenant_subdomain);
                    }
                    
                    App.showAlert(t('login.loginSuccessRedirecting', '登錄成功！正在跳轉...'), 'success');
                    
                    // 檢查個人資料是否完整
                    const profileComplete = data.profile && data.profile.complete;
                    const profileSkipped = data.profile && data.profile.skipped;
                    const hasPhone = data.profile && data.profile.hasPhone;
                    
                    // 如果需要設置租戶，跳轉到設置頁面
                    // vAi 產品不需要租戶，直接跳轉到產品頁面
                    if (data.requires_setup) {
                        const currentProduct = (localStorage.getItem('vlogin_product') || 'vwork').toLowerCase();
                        if (currentProduct === 'vai') {
                            // vAi 不需要租戶設置，直接進入 vai-chat
                            window.location.href = '/vai-chat';
                            return;
                        }
                        // 如果個人資料不完整且未跳過，先跳轉到個人資料頁面
                        if (!profileComplete && !profileSkipped) {
                            window.location.href = '/profile-guide';
                        } else {
                            window.location.href = getSetupTenantUrl();
                        }
                        return;
                    }

                    // 多租戶：需要選擇租戶
                    if (data.requires_tenant_selection && Array.isArray(data.tenants)) {
                        const lastTenantId = localStorage.getItem('tenant_id') || data.last_tenant_id;
                        const hasLast = lastTenantId && data.tenants.some(tn => String(tn.id) === String(lastTenantId));
                        const afterSelect = () => {
                            if (!profileComplete && !profileSkipped) {
                                window.location.href = '/profile-guide';
                                return;
                            }
                            checkIndustryTemplateAndRedirect();
                        };
                        if (hasLast) {
                            await selectTenantAndProceed(lastTenantId, afterSelect);
                        } else {
                            openTenantSelectionModal(data.tenants, lastTenantId, afterSelect);
                        }
                        return;
                    }
                    
                    // 如果個人資料不完整且未跳過，跳轉到個人資料頁面
                    if (!profileComplete && !profileSkipped) {
                        window.location.href = '/profile-guide';
                        return;
                    }
                    
                    // 檢查是否已選擇行業模板
                    checkIndustryTemplateAndRedirect();
                } else {
                    // 显示更详细的错误信息，并进行翻译
                    let errorMsg = data.error || data.message || (typeof I18n !== 'undefined' && I18n.t ? I18n.t('login.errors.loginFailed', '登錄失敗，請檢查電子郵件和密碼') : '登錄失敗，請檢查電子郵件和密碼');
                    // 翻译常见错误消息
                    if (errorMsg && typeof I18n !== 'undefined' && I18n.t) {
                        const errorKey = getErrorTranslationKey(errorMsg);
                        if (errorKey) {
                            errorMsg = I18n.t(`login.errors.${errorKey}`, errorMsg);
                        }
                    }
                    App.showAlert(errorMsg, 'danger');
                    submitBtn.disabled = false;
                    submitBtn.innerHTML = originalText;
                }
            } catch (error) {
                const errorMsg = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('login.errors.networkError', '網絡錯誤，請稍後再試') : '網絡錯誤，請稍後再試';
                App.showAlert(errorMsg, 'danger');
                submitBtn.disabled = false;
                submitBtn.innerHTML = originalText;
            }
        });
    }

    // 註冊表單提交
    if (registerFormData) {
        registerFormData.addEventListener('submit', async function(e) {
            e.preventDefault();
            
            const submitBtn = registerFormData.querySelector('button[type="submit"]');
            const originalText = submitBtn.innerHTML;
            submitBtn.disabled = true;
            submitBtn.innerHTML = '<span class="spinner-border spinner-border-sm"></span> ' + t('login.registering', '註冊中...');

            const phoneCodeSelect = document.getElementById('regPhoneCode');
            const phoneCode = (typeof $ !== 'undefined' && $(phoneCodeSelect).hasClass('select2-hidden-accessible')) 
                ? $(phoneCodeSelect).val() 
                : (phoneCodeSelect?.value || '');
            const phoneNumber = document.getElementById('regPhone')?.value.trim() || '';
            
            // 電話是可選的，不需要驗證
            const phone = phoneNumber ? `${phoneCode || ''} ${phoneNumber}`.trim() : '';
            
            const formData = {
                name: document.getElementById('regName')?.value || '',
                email: document.getElementById('regEmail')?.value || '',
                phone: phone || null,
                phone_country_code: phoneCode || null,
                birth_date: document.getElementById('regBirthDate')?.value || null,
                password: document.getElementById('regPassword')?.value || ''
            };

            try {
                const response = await fetch('/api/v1/auth/register', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(formData)
                });

                const data = await response.json();

                if (response.ok) {
                    // 註冊成功，後端已經創建了 session 並返回 token
                    const token = data.token;
                    
                    if (token) {
                        // 保存認證信息到 localStorage
                        localStorage.setItem('auth_token', token);
                        localStorage.setItem('user', JSON.stringify(data.user || {
                            id: data.user?.id,
                            email: data.user?.email,
                            name: data.user?.name
                        }));
                        
                        App.showAlert(t('login.registerSuccessRedirecting', '註冊成功！正在跳轉...'), 'success');
                        
                        // 檢查個人資料是否完整
                        const profileComplete = data.profile && data.profile.complete;
                        const hasPhone = data.profile && data.profile.hasPhone;
                        
                        // 如果個人資料不完整，先跳轉到個人資料頁面
                        setTimeout(() => {
                            if (!profileComplete) {
                                window.location.href = '/profile-guide';
                            } else {
                                window.location.href = getSetupTenantUrl();
                            }
                        }, 500);
                    } else {
                        // 如果沒有 token，嘗試自動登錄
                        const loginData = {
                            email: formData.email,
                            password: formData.password
                        };
                        
                        const loginResponse = await fetch('/api/v1/auth/login', {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json',
                            },
                            body: JSON.stringify(loginData)
                        });
                        
                        const loginResult = await loginResponse.json();
                        
                        if (loginResponse.ok && loginResult.token) {
                            localStorage.setItem('auth_token', loginResult.token);
                            localStorage.setItem('user', JSON.stringify(loginResult.user || {}));
                            App.showAlert(t('login.registerSuccessRedirecting', '註冊成功！正在跳轉...'), 'success');
                            
                            // 檢查個人資料是否完整
                            const profileComplete = loginResult.profile && loginResult.profile.complete;
                            
                            setTimeout(() => {
                                if (!profileComplete) {
                                    window.location.href = '/profile-guide';
                                } else {
                                    window.location.href = getSetupTenantUrl();
                                }
                            }, 500);
                        } else {
                            App.showAlert(t('login.registerSuccessPleaseLogin', '註冊成功！請登錄'), 'success');
                            // 自動填充登錄表單
                            const emailInput = document.getElementById('email');
                            if (emailInput) {
                                emailInput.value = formData.email;
                            }
                            const loginHeader = document.getElementById('loginHeader');
                            const registerHeader = document.getElementById('registerHeader');
                            if (registerHeader) registerHeader.style.display = 'none';
                            if (loginHeader) loginHeader.style.display = 'block';
                            if (registerForm) registerForm.style.display = 'none';
                            if (loginForm) loginForm.style.display = 'block';
                        }
                    }
                } else {
                    // 翻译错误消息
                    let errorMsg = data.error || (typeof I18n !== 'undefined' && I18n.t ? I18n.t('login.errors.registerFailed', '註冊失敗') : '註冊失敗');
                    if (data.error && typeof I18n !== 'undefined' && I18n.t) {
                        const errorKey = getErrorTranslationKey(data.error);
                        if (errorKey) {
                            errorMsg = I18n.t(`login.errors.${errorKey}`, data.error);
                        } else {
                            errorMsg = data.error;
                        }
                    }
                    App.showAlert(errorMsg, 'danger');
                }
                
                submitBtn.disabled = false;
                submitBtn.innerHTML = originalText;
            } catch (error) {
                const errorMsg = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('login.errors.networkError', '網絡錯誤，請稍後再試') : '網絡錯誤，請稍後再試';
                App.showAlert(errorMsg, 'danger');
                submitBtn.disabled = false;
                submitBtn.innerHTML = originalText;
            }
        });
    }

    // Google OAuth 配置
    let googleOAuthConfig = null;
    let googleClientID = null;

    // 獲取 Google OAuth 配置
    async function loadGoogleOAuthConfig() {
        try {
            console.log('Loading Google OAuth config...');
            const response = await fetch('/api/v1/auth/google/config');
            if (response.ok) {
                const data = await response.json();
                console.log('Google OAuth config loaded:', data);
                googleOAuthConfig = data;
                googleClientID = data.client_id;
                
                // 如果已啟用，初始化 Google Sign-In
                if (data.enabled && data.client_id) {
                    // 檢查是否已登錄，如果已登錄則不初始化（避免自動彈出選擇賬號窗口）
                    const token = localStorage.getItem('auth_token');
                    if (token && token !== 'temp_token') {
                        console.log('User already logged in, skipping Google Sign-In initialization to prevent auto-popup');
                        return;
                    }
                    
                    // 等待 Google SDK 加載完成
                    const waitForGoogleSDK = (maxAttempts = 20, attempt = 0) => {
                        if (window.google && window.google.accounts && window.google.accounts.id) {
                            console.log('Google SDK loaded, initializing...');
                            initializeGoogleSignIn();
                        } else if (attempt < maxAttempts) {
                            console.log(`Waiting for Google SDK... (${attempt + 1}/${maxAttempts})`);
                            setTimeout(() => waitForGoogleSDK(maxAttempts, attempt + 1), 500);
                        } else {
                            console.error('Google SDK failed to load after', maxAttempts, 'attempts');
                            App.showAlert('Google 登錄 SDK 加載失敗，請刷新頁面重試', 'warning');
                        }
                    };
                    
                    // 開始等待 SDK 加載
                    waitForGoogleSDK();
                } else {
                    console.log('Google OAuth not enabled or client ID missing');
                }
            } else {
                console.error('Failed to load Google OAuth config, status:', response.status);
            }
        } catch (error) {
            console.error('Failed to load Google OAuth config:', error);
        }
    }

    // 初始化 Google Sign-In（僅初始化，不自動顯示）
    function initializeGoogleSignIn() {
        if (!googleClientID) {
            console.warn('Google Client ID not available');
            return;
        }
        
        console.log('Initializing Google Sign-In with Client ID:', googleClientID);
        console.log('Current origin:', window.location.origin);
        
        try {
            window.google.accounts.id.initialize({
                client_id: googleClientID,
                callback: handleGoogleSignIn,
                auto_select: false, // 禁用自動選擇
                cancel_on_tap_outside: true,
                itp_support: true,
                use_fedcm_for_prompt: false // 禁用自動提示
            });
            
            console.log('Google Sign-In initialized successfully (auto-prompting disabled)');

            // 明確禁用自動顯示 One Tap（無論是否登錄都禁用）
            try {
                window.google.accounts.id.disableAutoSelect();
                console.log('Auto-select disabled to prevent auto-popup');
            } catch (e) {
                console.warn('Failed to disable auto-select:', e);
            }
            
            // 檢查是否已登錄，如果已登錄則完全禁用 One Tap
            const token = localStorage.getItem('auth_token');
            if (token && token !== 'temp_token') {
                try {
                    window.google.accounts.id.cancel(); // 取消任何正在顯示的提示
                    console.log('One Tap cancelled because user is already logged in');
                } catch (e) {
                    console.warn('Failed to cancel One Tap:', e);
                }
            }
        } catch (error) {
            console.error('Failed to initialize Google Sign-In:', error);
            // 如果初始化失败，可能是因为域名未授权
            const currentOrigin = window.location.origin;
            console.error(`Google OAuth 初始化錯誤：${error.message}`);
            console.error(`當前域名：${currentOrigin}`);
            console.error(`請確認在 Google Cloud Console 中添加 "${currentOrigin}" 到「授權的 JavaScript 來源」`);
        }
    }

    // 處理 Google 登錄回調
    async function handleGoogleSignIn(response) {
        // 重置觸發標記
        isGoogleSignInTriggered = false;
        
        try {
            console.log('Google sign-in response received:', response);
            
            // 立即取消/關閉 One Tap 提示，避免重複顯示
            try {
                if (window.google && window.google.accounts && window.google.accounts.id) {
                    window.google.accounts.id.cancel();
                    console.log('One Tap cancelled after sign-in');
                }
            } catch (cancelError) {
                console.warn('Failed to cancel One Tap:', cancelError);
            }
            
            if (!response.credential) {
                console.error('No credential in response:', response);
                throw new Error('No credential received from Google');
            }

            console.log('Sending token to backend, token length:', response.credential.length);
            
            // 發送到後端驗證
            const apiResponse = await fetch('/api/v1/auth/google', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ token: response.credential })
            });

            console.log('Backend response status:', apiResponse.status);
            
            const data = await apiResponse.json();
            console.log('Backend response data:', data);

            if (!apiResponse.ok) {
                console.error('Backend returned error:', data);
                throw new Error(data.error || 'Google login failed');
            }

            // 保存 token 和用戶信息
            if (data.token) {
                localStorage.setItem('auth_token', data.token);
            }
            if (data.user) {
                localStorage.setItem('user', JSON.stringify(data.user));
            }
            // Signal CMS to show vAi greet bubble after redirect
            localStorage.setItem('vai_just_logged_in', '1');

            // 立即並徹底禁用 Google One Tap，防止登錄後再次彈出選擇賬號窗口
            try {
                if (window.google && window.google.accounts && window.google.accounts.id) {
                    // 1. 取消當前的提示
                    window.google.accounts.id.cancel();
                    
                    // 2. 禁用自動選擇
                    window.google.accounts.id.disableAutoSelect();
                    
                    // 3. 存儲憑證，告訴 Google 用戶已經登錄（這會阻止後續的自動彈出）
                    try {
                        window.google.accounts.id.storeCredential({
                            id: response.credential,
                            password: '' // One Tap 不需要密碼
                        });
                        console.log('Credential stored to prevent future auto-popup');
                    } catch (storeError) {
                        // storeCredential 可能不存在或失敗，忽略
                        console.warn('Could not store credential (may not be available):', storeError);
                    }
                    
                    console.log('One Tap fully disabled after successful login');
                }
            } catch (cancelError) {
                console.warn('Error cancelling/disabling One Tap:', cancelError);
            }

            // 立即移除所有 Google 相關的 DOM 元素（強制清理）
            try {
                const googleElements = document.querySelectorAll('iframe[src*="accounts.google.com"], iframe[src*="gsi"], [id*="google"], [class*="google-one-tap"]');
                googleElements.forEach(el => {
                    try {
                        if (el && el.parentNode) {
                            el.style.display = 'none';
                            el.style.visibility = 'hidden';
                            el.remove();
                        }
                    } catch (e) {
                        // 忽略錯誤
                    }
                });
            } catch (e) {
                console.warn('Error removing Google elements:', e);
            }

            // 檢查個人資料是否完整
            const profileComplete = data.profile && data.profile.complete;
            const profileSkipped = data.profile && data.profile.skipped;

            // 不顯示成功消息，直接重定向（避免延遲和彈窗繼續顯示）
            // 如果用戶需要設置租戶，重定向到設置頁面
            // vAi 產品不需要租戶，直接跳轉到產品頁面
            if (data.requires_setup) {
                const currentProduct = (localStorage.getItem('vlogin_product') || 'vwork').toLowerCase();
                if (currentProduct === 'vai') {
                    // vAi 不需要租戶設置，直接進入 vai-chat
                    window.location.replace('/vai-chat');
                    return;
                }
                // 如果個人資料不完整且未跳過，先跳轉到個人資料頁面
                if (!profileComplete && !profileSkipped) {
                    window.location.replace('/profile-guide');
                } else {
                    window.location.replace(getSetupTenantUrl()); // 使用 replace 而不是 href，避免返回登錄頁
                }
                return;
            }

            // 如果個人資料不完整且未跳過，跳轉到個人資料頁面
            if (!profileComplete && !profileSkipped) {
                window.location.replace('/profile-guide');
                return;
            }

            // 否則重定向到目標產品（vWork → /dashboard, vOffice/vMarket → SSO redirect）
            redirectToProduct();

        } catch (error) {
            console.error('Google login error:', error);
            
            // 錯誤時也取消 One Tap
            try {
                if (window.google && window.google.accounts && window.google.accounts.id) {
                    window.google.accounts.id.cancel();
                }
            } catch (cancelError) {
                // 忽略錯誤
            }
            
            App.showAlert(error.message || t('login.googleLoginFailed', 'Google 登錄失敗，請稍後再試'), 'danger');
        }
    }

    // 防止重複觸發的標記
    let isGoogleSignInTriggered = false;
    let googleSignInTimeout = null;

    // 觸發 Google 登錄
    function triggerGoogleSignIn() {
        // 防止重複觸發（在短時間內）
        if (isGoogleSignInTriggered) {
            console.log('Google sign-in already triggered, ignoring duplicate call');
            return;
        }
        
        console.log('Triggering Google Sign-In...');
        console.log('Config:', googleOAuthConfig);
        console.log('Client ID:', googleClientID);
        console.log('Google SDK:', window.google);
        
        if (!googleOAuthConfig || !googleOAuthConfig.enabled) {
            console.warn('Google OAuth not enabled');
            App.showAlert(t('login.googleOAuthNotEnabled', 'Google 登錄功能未啟用'), 'warning');
            return;
        }

        if (!googleClientID) {
            console.warn('Google Client ID not configured');
            App.showAlert(t('login.googleOAuthNotConfigured', 'Google 登錄未配置'), 'warning');
            return;
        }

        if (!window.google || !window.google.accounts || !window.google.accounts.id) {
            console.error('Google SDK not loaded. Retrying...');
            App.showAlert(t('login.googleSDKNotLoaded', 'Google 登錄 SDK 未加載，請稍候再試'), 'warning');
            
            // 重新嘗試加載配置和初始化
            setTimeout(() => {
                if (window.google && window.google.accounts) {
                    initializeGoogleSignIn();
                    // 再次嘗試觸發
                    setTimeout(() => triggerGoogleSignIn(), 500);
                } else {
                    loadGoogleOAuthConfig();
                }
            }, 1000);
            return;
        }

        try {
            // 設置標記，防止重複觸發
            isGoogleSignInTriggered = true;
            
            // 設置超時自動重置標記（允許用戶在取消後再次點擊）
            if (googleSignInTimeout) {
                clearTimeout(googleSignInTimeout);
            }
            googleSignInTimeout = setTimeout(() => {
                if (isGoogleSignInTriggered) {
                    console.log('Resetting Google sign-in trigger flag after timeout');
                    isGoogleSignInTriggered = false;
                }
            }, 5000); // 5秒後自動重置
            
            console.log('Triggering Google Sign-In...');
            
            // 先取消任何正在顯示的 One Tap，防止多個彈窗同時出現
            try {
                if (window.google && window.google.accounts && window.google.accounts.id) {
                    window.google.accounts.id.cancel();
                    window.google.accounts.id.disableAutoSelect();
                    console.log('Cancelled any existing One Tap prompts');
                }
            } catch (e) {
                console.warn('Failed to cancel existing prompts:', e);
            }
            
            // 等待一小段時間確保 One Tap 已取消，然後再使用備選方案（renderButton）
            // 直接使用 renderButton，因為它更穩定且不會與 One Tap 衝突
            setTimeout(() => {
                if (isGoogleSignInTriggered) {
                    console.log('Using renderButton method (more stable)');
                    tryRenderButtonFallback();
                }
            }, 100);
            
        } catch (error) {
            isGoogleSignInTriggered = false; // 重置標記
            console.error('Google sign-in error:', error);
            console.error('Error details:', error.stack);
            App.showAlert(error.message || t('login.googleLoginFailed', 'Google 登錄失敗，請稍後再試'), 'danger');
            
            // 嘗試備選方案
            tryRenderButtonFallback();
        }
    }
    
    // 使用 renderButton（主要方法，更穩定且不會與 One Tap 衝突）
    function tryRenderButtonFallback() {
        try {
            // 先確保取消任何正在顯示的 One Tap
            try {
                if (window.google && window.google.accounts && window.google.accounts.id) {
                    window.google.accounts.id.cancel();
                    window.google.accounts.id.disableAutoSelect();
                }
            } catch (e) {
                console.warn('Failed to cancel One Tap before renderButton:', e);
            }
            
            console.log('Using renderButton method...');
            
            // 創建一個隱藏的容器
            let container = document.getElementById('google-signin-button-hidden');
            if (!container) {
                container = document.createElement('div');
                container.id = 'google-signin-button-hidden';
                container.style.display = 'none';
                container.style.position = 'absolute';
                container.style.top = '-9999px';
                container.style.left = '-9999px';
                document.body.appendChild(container);
            }
            
            // 清空容器
            container.innerHTML = '';
            
            // 等待一小段時間確保 DOM 已準備好
            setTimeout(() => {
                try {
                    // 渲染 Google 按鈕
                    window.google.accounts.id.renderButton(container, {
                        theme: 'outline',
                        size: 'large',
                        text: 'signin_with',
                        width: 300,
                        type: 'standard'
                    });
                    
                    // 等待按鈕渲染完成後點擊
                    setTimeout(() => {
                        const button = container.querySelector('div[role="button"]');
                        if (button) {
                            console.log('Found Google button, clicking...');
                            button.click();
                        } else {
                            console.error('Google button not found after render');
                            isGoogleSignInTriggered = false; // 重置標記
                            App.showAlert('無法啟動 Google 登錄，請檢查瀏覽器控制台獲取詳細錯誤信息', 'danger');
                        }
                    }, 200);
                } catch (renderError) {
                    console.error('Failed to render button:', renderError);
                    isGoogleSignInTriggered = false; // 重置標記
                    App.showAlert('Google 登錄暫時無法使用。請檢查瀏覽器控制台的錯誤信息。', 'danger');
                }
            }, 100);
        } catch (error) {
            isGoogleSignInTriggered = false; // 重置標記
            console.error('renderButton method failed:', error);
            App.showAlert('Google 登錄暫時無法使用。請檢查瀏覽器控制台的錯誤信息。', 'danger');
        }
    }

    // Google 登錄按鈕
    const btnGoogleLogin = document.getElementById('btnGoogleLogin');
    if (btnGoogleLogin) {
        btnGoogleLogin.addEventListener('click', triggerGoogleSignIn);
    }

    // Google 註冊按鈕（與登錄功能相同）
    const btnGoogleRegister = document.getElementById('btnGoogleRegister');
    if (btnGoogleRegister) {
        btnGoogleRegister.addEventListener('click', triggerGoogleSignIn);
    }

    // 頁面加載時獲取 Google OAuth 配置
    loadGoogleOAuthConfig();
});

// 获取错误消息的翻译键
function getErrorTranslationKey(errorMsg) {
    if (!errorMsg) return null;
    
    const errorMsgLower = errorMsg.toLowerCase();
    
    // 匹配常见错误消息
    if (errorMsgLower.includes('email already registered') || errorMsgLower.includes('already registered')) {
        return 'emailAlreadyRegistered';
    }
    if (errorMsgLower.includes('email or password') || errorMsgLower.includes('電子郵件或密碼') || errorMsgLower.includes('电子邮件或密码')) {
        return 'invalidEmailOrPassword';
    }
    if (errorMsgLower.includes('account has been deactivated') || errorMsgLower.includes('帳戶已被停用') || errorMsgLower.includes('帐户已被停用')) {
        return 'accountInactive';
    }
    if (errorMsgLower.includes('enterprise does not exist') || errorMsgLower.includes('企業不存在') || errorMsgLower.includes('企业不存在')) {
        return 'tenantNotFound';
    }
    if (errorMsgLower.includes('enterprise is not activated') || errorMsgLower.includes('企業未激活') || errorMsgLower.includes('企业未激活')) {
        return 'tenantInactive';
    }
    if (errorMsgLower.includes('name is required') || errorMsgLower.includes('姓名為必填') || errorMsgLower.includes('姓名为必填')) {
        return 'nameRequired';
    }
    if (errorMsgLower.includes('name must be at most') || errorMsgLower.includes('姓名最多')) {
        return 'nameTooLong';
    }
    if (errorMsgLower.includes('password must be at least') || errorMsgLower.includes('密碼至少') || errorMsgLower.includes('密码至少')) {
        return 'passwordTooShort';
    }
    if (errorMsgLower.includes('password must be at most') || errorMsgLower.includes('密碼最多') || errorMsgLower.includes('密码最多')) {
        return 'passwordTooLong';
    }
    if (errorMsgLower.includes('email is required') || errorMsgLower.includes('電子郵件為必填') || errorMsgLower.includes('电子邮件为必填')) {
        return 'emailRequired';
    }
    if (errorMsgLower.includes('password is required') || errorMsgLower.includes('密碼為必填') || errorMsgLower.includes('密码为必填')) {
        return 'passwordRequired';
    }
    if (errorMsgLower.includes('invalid request') || errorMsgLower.includes('無效的請求') || errorMsgLower.includes('无效的请求')) {
        return 'invalidRequest';
    }
    
    return null;
}

// 檢查行業模板並重定向
async function checkIndustryTemplateAndRedirect() {
    try {
        const token = localStorage.getItem('auth_token');
        if (!token || token === 'temp_token') {
            redirectToProduct();
            return;
        }

        // 先檢查個人資料是否完整
        try {
            // 先從 localStorage 獲取用戶信息（可能包含最新狀態）
            const userStr = localStorage.getItem('user');
            const user = userStr ? JSON.parse(userStr) : null;
            const profileSkipped = user && user.extra_fields && user.extra_fields.profile_guide_skipped;
            
            // 檢查個人資料完整性
            const profileResponse = await fetch('/api/v1/user/profile/check', {
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json'
                }
            });
            
            if (profileResponse.ok) {
                const profileData = await profileResponse.json();
                // 如果個人資料不完整且未跳過，先跳轉到個人資料頁面
                // profileData.complete 檢查 name 和 email 是否存在
                if (!profileData.complete) {
                    // 如果已跳過，繼續檢查行業模板
                    if (profileSkipped) {
                        // 已跳過，繼續檢查行業模板
                    } else {
                        // 未跳過，跳轉到個人資料頁面（onboarding stays on vWork）
                        window.location.href = '/profile-guide';
                        return;
                    }
                }
            }
        } catch (error) {
            console.warn('檢查個人資料失敗，繼續檢查行業模板:', error);
        }

        // 檢查是否已經選擇或跳過過（避免重複彈出）
        const hasSelected = localStorage.getItem('industry_template_selected') === 'true';
        const hasSkipped = localStorage.getItem('industry_template_skipped') === 'true';
        
        if (hasSelected || hasSkipped) {
            redirectToProduct();
            return;
        }

        // 檢查租戶是否已選擇行業模板
        const response = await fetch('/api/v1/tenant/industry-template', {
            headers: {
                'Authorization': `Bearer ${token}`,
                'Content-Type': 'application/json'
            }
        });

        // 處理訂閱過期的情況
        if (response.status === 403) {
            try {
                const errorData = await response.json();
                if (errorData.error === 'subscription_required' && errorData.redirect) {
                    window.location.href = errorData.redirect;
                    return;
                }
            } catch (e) {
                // 如果無法解析錯誤，繼續正常流程
            }
        }

        if (response.ok) {
            const data = await response.json();
            // 如果沒有選擇模板，重定向到選擇頁面（第一次登錄，onboarding stays on vWork）
            if (!data.data || !data.data.id) {
                window.location.href = '/industry-template-selector';
                return;
            } else {
                // 已選擇模板，記錄標記
                localStorage.setItem('industry_template_selected', 'true');
            }
        }
        
        // 已選擇模板或檢查失敗，重定向到目標產品
        redirectToProduct();
    } catch (error) {
        console.warn('檢查行業模板失敗，重定向到目標產品:', error);
        redirectToProduct();
    }
}

