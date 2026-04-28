(() => {
  const modalEl = document.getElementById('tenantChatModal');
  const messagesEl = document.getElementById('tenantChatMessages');
  const formEl = document.getElementById('tenantChatForm');
  const inputEl = document.getElementById('tenantChatInput');
  const sendBtn = document.getElementById('tenantChatSendBtn');
  if (!modalEl || !messagesEl || !formEl || !inputEl) return;

  const subdomain = (window.location.pathname.split('/')[2] || '').trim();
  if (!subdomain) return;

  const storageKey = `public_chat_visitor_id_${subdomain}`;
  const nameKey = `public_chat_visitor_name_${subdomain}`;

  const ensureVisitorId = () => {
    let id = '';
    try { id = (localStorage.getItem(storageKey) || '').trim(); } catch {}
    if (!id) {
      // UUID v4-ish
      id = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
        const r = Math.random() * 16 | 0;
        const v = c === 'x' ? r : (r & 0x3 | 0x8);
        return v.toString(16);
      });
      try { localStorage.setItem(storageKey, id); } catch {}
    }
    return id;
  };

  const t = (key, fallback) => {
    if (typeof I18n !== 'undefined' && I18n.t) {
      const val = I18n.t(key);
      // I18n.t returns the key itself when translation is not found/not loaded yet
      return (val && val !== key) ? val : (fallback || key);
    }
    return fallback || key;
  };

  const ensureVisitorName = (visitorId) => {
    let name = '';
    try { name = (localStorage.getItem(nameKey) || '').trim(); } catch {}
    // Fix corrupted names: if stored value looks like an untranslated i18n key, clear it
    if (name && name.startsWith('publicSite.')) {
      name = '';
      try { localStorage.removeItem(nameKey); } catch {}
    }
    if (!name) {
      const suffix = visitorId.slice(-6);
      name = `${t('publicSite.chat.visitor', '訪客')} ${suffix}`;
      try { localStorage.setItem(nameKey, name); } catch {}
    }
    return name;
  };

  const visitorId = ensureVisitorId();
  let currentVisitorId = visitorId;
  let visitorName = ensureVisitorName(visitorId);

  // If I18n translations were not ready during initial ensureVisitorName,
  // retry once they are loaded to fix the visitor name
  if (typeof I18n !== 'undefined' && I18n.whenReady) {
    I18n.whenReady().then(() => {
      const stored = (() => { try { return (localStorage.getItem(nameKey) || '').trim(); } catch { return ''; } })();
      // Re-check: if name still looks like an untranslated key, regenerate
      if (stored.startsWith('publicSite.') || !stored) {
        const suffix = visitorId.slice(-6);
        visitorName = `${t('publicSite.chat.visitor', '訪客')} ${suffix}`;
        try { localStorage.setItem(nameKey, visitorName); } catch {}
      }
    });
  }

  const apiBase = `/api/v1/public/${encodeURIComponent(subdomain)}/chat`;

  let pollTimer = null;
  let lastRenderCount = 0;

  const escapeHtml = (s) =>
    String(s || '').replace(/[&<>"']/g, (m) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[m]));

  const scrollToBottom = () => {
    messagesEl.scrollTop = messagesEl.scrollHeight;
  };

  const renderMessages = (list) => {
    const items = Array.isArray(list) ? list : [];
    if (items.length === 0) {
      messagesEl.innerHTML = `<div class="text-center p-4 text-muted"><p class="mt-2">${escapeHtml(t('publicSite.chat.startChat', '開始與我們對話'))}</p></div>`;
      lastRenderCount = 0;
      return;
    }

    messagesEl.innerHTML = items.map((m) => {
      const isMe = !m.from_user_id; // visitor side: from_user_id null 表示訪客自己送出
      const content = escapeHtml(m.content || '').replace(/\n/g, '<br>');
      
      // 格式化時間
      const timeLocale = (typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'en-US' : (typeof I18n !== 'undefined' && I18n.currentLang === 'zh-CN') ? 'zh-CN' : 'zh-TW';
      const timeStr = m.created_at ? new Date(m.created_at).toLocaleString(timeLocale, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      }) : '';
      
      // 使用與 aiChatMessages 一致的結構和樣式
      return `
        <div class="d-flex ${isMe ? 'justify-content-end' : 'justify-content-start'} mb-2 message-item" data-message-id="${m.id || ''}">
          ${!isMe ? `
            <div class="avatar-circle me-2 flex-shrink-0" style="width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;">
              <i class="bi bi-person-circle"></i>
            </div>
          ` : ''}
          <div class="message-bubble ${isMe ? 'user' : 'ai'} position-relative" style="max-width: 70%;">
            <div style="white-space: pre-wrap; word-wrap: break-word;">${content}</div>
            ${timeStr ? `<div class="message-time" style="font-size: 0.75rem; opacity: 0.7; margin-top: 0.25rem;">${timeStr}</div>` : ''}
          </div>
          ${isMe ? `
            <div class="avatar-circle ms-2 flex-shrink-0" style="width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;">
              <i class="bi bi-person"></i>
            </div>
          ` : ''}
        </div>
      `;
    }).join('');

    if (items.length !== lastRenderCount) {
      lastRenderCount = items.length;
      setTimeout(scrollToBottom, 50);
    }
  };

  const loadConversation = async () => {
    const url = `${apiBase}/conversation?visitor_id=${encodeURIComponent(currentVisitorId)}&limit=200`;
    const resp = await fetch(url, { method: 'GET' });
    const data = await resp.json().catch(() => ({}));
    renderMessages((data && data.data) ? data.data : []);
  };

  const sendMessage = async (content) => {
    const resp = await fetch(`${apiBase}/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        visitor_id: currentVisitorId,
        visitor_name: visitorName,
        content
      })
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) {
      const errorMsg = (data && data.error) ? data.error : t('publicSite.chat.sendFailed', '發送失敗');
      const details = (data && data.details) ? ` (${data.details})` : '';
      console.error('Public chat send failed:', errorMsg, details, data);
      throw new Error(errorMsg + details);
    }
    // 若後端生成了 visitor_id（缺省情況），同步回本地
    const returnedVisitorId = data?.data?.visitor_id;
    if (returnedVisitorId && returnedVisitorId !== currentVisitorId) {
      currentVisitorId = returnedVisitorId;
      try { localStorage.setItem(storageKey, returnedVisitorId); } catch {}
    }
  };

  // textarea auto-grow
  inputEl.addEventListener('input', function () {
    this.style.height = 'auto';
    this.style.height = Math.min(this.scrollHeight, 120) + 'px';
  });
  inputEl.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      formEl.dispatchEvent(new Event('submit', { cancelable: true }));
    }
  });

  // Show inline error inside modal instead of window.alert (which steals focus and can close the modal)
  const showInlineError = (msg) => {
    let errEl = formEl.querySelector('.tenant-chat-inline-error');
    if (!errEl) {
      errEl = document.createElement('div');
      errEl.className = 'tenant-chat-inline-error text-danger small mt-1';
      formEl.appendChild(errEl);
    }
    errEl.textContent = msg;
    setTimeout(() => { errEl.textContent = ''; }, 5000);
  };

  formEl.addEventListener('submit', async (e) => {
    e.preventDefault();
    e.stopPropagation();
    const content = (inputEl.value || '').trim();
    if (!content) return;

    if (sendBtn) sendBtn.disabled = true;
    inputEl.disabled = true;
    try {
      await sendMessage(content);
      inputEl.value = '';
      inputEl.style.height = 'auto';
      await loadConversation();
    } catch (err) {
      showInlineError(err && err.message ? err.message : t('publicSite.chat.sendFailed', '發送失敗'));
    } finally {
      inputEl.disabled = false;
      if (sendBtn) sendBtn.disabled = false;
      inputEl.focus();
    }
  });

  modalEl.addEventListener('shown.bs.modal', async () => {
    try {
      await loadConversation();
    } catch (e) {
      messagesEl.innerHTML = `<div class="text-center text-danger py-4">${escapeHtml(t('publicSite.chat.loadFailed', '載入失敗'))}</div>`;
    }
    inputEl.focus();

    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(() => {
      loadConversation().catch(() => {});
    }, 4000);
  });

  modalEl.addEventListener('hidden.bs.modal', () => {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = null;
  });
})();


