/**
 * sso.js — 跨域單點登入 (SSO) 前端工具
 *
 * 功能：
 *  1. 頁面載入時自動偵測 URL 中的 ?sso_ticket=xxx，向後端驗證並完成登入
 *  2. 提供 SSO.navigateTo(url) 方法，用於跨域導航時自動夾帶 SSO ticket
 *  3. 自動攔截含有 data-sso-link 屬性的連結
 *
 * 用法：
 *  <script src="/static/js/sso.js"></script>
 *
 *  <!-- 自動攔截：點擊時會先取 ticket 再跳轉 -->
 *  <a href="https://other-domain.com" data-sso-link>跳轉</a>
 *
 *  <!-- 手動呼叫 -->
 *  <script>SSO.navigateTo('https://other-domain.com/page');</script>
 */
(function () {
    'use strict';

    const SSO = {};

    // ─── 常量 ─────────────────────────────────────
    const TICKET_PARAM = 'sso_ticket';

    // ─── 工具函數 ─────────────────────────────────
    function getAuthToken() {
        return localStorage.getItem('auth_token');
    }

    function getTenantSubdomain() {
        return localStorage.getItem('tenant_subdomain');
    }

    /**
     * 判斷一個 URL 是否跨域（不同 origin）
     */
    function isCrossDomain(url) {
        try {
            const target = new URL(url, window.location.origin);
            return target.origin !== window.location.origin;
        } catch (e) {
            return false;
        }
    }

    /**
     * 判斷是否處於本地開發環境
     * 本地開發時所有 domain 都是 localhost:3001，不需要 SSO
     */
    function isLocalDev() {
        const h = window.location.hostname.toLowerCase();
        return h === 'localhost' || h === '127.0.0.1';
    }

    // ─── 核心功能 ───────────────────────────────────

    /**
     * 向後端申請一次性 SSO ticket
     * @returns {Promise<string|null>} ticket 字串，失敗則為 null
     */
    SSO.requestTicket = async function () {
        const token = getAuthToken();
        if (!token || token === 'temp_token') return null;

        try {
            const headers = {
                'Accept': 'application/json',
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token,
            };
            const subdomain = getTenantSubdomain();
            if (subdomain) headers['X-Tenant-Subdomain'] = subdomain;

            const resp = await fetch('/api/v1/sso/ticket', {
                method: 'POST',
                headers: headers,
            });
            if (!resp.ok) return null;
            const data = await resp.json();
            return data.ticket || null;
        } catch (e) {
            console.error('[SSO] Failed to request ticket:', e);
            return null;
        }
    };

    /**
     * 驗證 SSO ticket（在目標域名上呼叫）
     * @param {string} ticket
     * @returns {Promise<object|null>} { token, user, tenant } 或 null
     */
    SSO.validateTicket = async function (ticket) {
        try {
            const resp = await fetch('/api/v1/sso/validate', {
                method: 'POST',
                headers: {
                    'Accept': 'application/json',
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ ticket: ticket }),
            });
            if (!resp.ok) return null;
            return await resp.json();
        } catch (e) {
            console.error('[SSO] Failed to validate ticket:', e);
            return null;
        }
    };

    /**
     * 跨域導航：先取 ticket，再跳轉到目標 URL
     * 如果是本地開發（同域），直接跳轉不需要 ticket
     * @param {string} url 目標完整 URL
     */
    SSO.navigateTo = async function (url) {
        // 本地開發或同域：直接跳轉
        if (!isCrossDomain(url) || isLocalDev()) {
            window.location.href = url;
            return;
        }

        // 未登入：直接跳轉（無 ticket）
        const token = getAuthToken();
        if (!token || token === 'temp_token') {
            window.location.href = url;
            return;
        }

        // 已登入 + 跨域：取 ticket 並附加到 URL
        const ticket = await SSO.requestTicket();
        if (ticket) {
            const targetUrl = new URL(url);
            targetUrl.searchParams.set(TICKET_PARAM, ticket);
            window.location.href = targetUrl.toString();
        } else {
            // ticket 取得失敗，仍然跳轉（用戶需要在目標域名重新登入）
            window.location.href = url;
        }
    };

    /**
     * 頁面載入時自動處理 SSO ticket
     * 偵測 URL 中的 ?sso_ticket=xxx，驗證後儲存登入狀態
     */
    SSO.handleIncomingTicket = async function () {
        const params = new URLSearchParams(window.location.search);
        const ticket = params.get(TICKET_PARAM);
        if (!ticket) return false;

        // 立即從 URL 移除 sso_ticket 參數（避免重新載入時重複使用）
        params.delete(TICKET_PARAM);
        const cleanUrl = window.location.pathname +
            (params.toString() ? '?' + params.toString() : '') +
            window.location.hash;
        window.history.replaceState({}, '', cleanUrl);

        // 驗證 ticket
        const result = await SSO.validateTicket(ticket);
        if (!result || !result.token) {
            console.warn('[SSO] Ticket validation failed');
            return false;
        }

        // 儲存到 localStorage
        localStorage.setItem('auth_token', result.token);
        if (result.user) {
            localStorage.setItem('user', JSON.stringify(result.user));
        }
        if (result.tenant) {
            if (result.tenant.subdomain) localStorage.setItem('tenant_subdomain', result.tenant.subdomain);
            if (result.tenant.id) localStorage.setItem('tenant_id', result.tenant.id);
            if (result.tenant.name) localStorage.setItem('tenant_name', result.tenant.name);
        }

        console.log('[SSO] Cross-domain login successful');

        // Reload the page so the normal auth-check logic (which runs on
        // DOMContentLoaded) picks up the newly-saved token from localStorage.
        // Without this, there is a race condition: the page's auth check runs
        // before the async ticket validation completes, so the first visit
        // shows the "not logged in" state.
        // We use replaceState above to strip the sso_ticket param, so the
        // reload will NOT re-trigger ticket validation.
        window.location.reload();
        // Return a never-resolving promise so no further code runs before reload
        return new Promise(function () {});
    };

    // ─── 自動初始化 ─────────────────────────────────

    // 1. 頁面載入時處理傳入的 SSO ticket
    SSO.handleIncomingTicket();

    // 2. 自動攔截帶有 data-sso-link 屬性的連結
    document.addEventListener('click', function (e) {
        const link = e.target.closest('a[data-sso-link]');
        if (!link) return;

        const href = link.getAttribute('href');
        if (!href) return;

        e.preventDefault();
        SSO.navigateTo(href);
    });

    // 掛載到 window
    window.SSO = SSO;
})();
