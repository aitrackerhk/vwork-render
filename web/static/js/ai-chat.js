// AI Chat 全局功能（支持多對話）
let aiChatHistory = [];
let aiEndpoint = '';
let aiApiKey = ''; // 不再从前端存储，由后端处理
let aiModel = 'gpt-3.5-turbo';
let aiCurrentUserId = null; // 使用不同的变量名避免冲突
let aiConversations = []; // 所有對話列表
let aiCurrentConversationId = null; // 當前選中的對話 ID
const PENDING_CONV_ID = '__pending__'; // 尚未建立的暫存對話 ID
let aiWebSearchEnabled = false; // Web Search (Google grounding) toggle state
let aiSpeechRecognition = null;
let aiVoiceListening = false;
let aiVoiceRecording = false;
let aiMediaRecorder = null;
let aiRecordingStream = null;
let aiRecordedChunks = [];
let aiSpeechBaseText = '';
let aiRecordingTimerInterval = null;
let aiRecordingSeconds = 0;
let aiRecordingDiscarded = false; // flag to skip auto-send on discard

// Track active document polling intervals: { docId -> intervalId }
const _aiDocPollers = {};

// Track active video polling intervals: { operationId -> intervalId }
const _aiVideoPollers = {};

// Start polling for a generating video. Updates the DOM card and message
// extra_fields when the video completes (or fails).
function startVideoGenerationPolling(operationId, messageId) {
    if (_aiVideoPollers[operationId]) return; // Already polling

    console.log('[vAi] Starting video poll for operation:', operationId, 'msgId:', messageId);

    let pollCount = 0;
    const maxPolls = 120; // 10 min max (5s intervals)

    const pollInterval = setInterval(async () => {
        pollCount++;

        if (pollCount > maxPolls) {
            clearInterval(pollInterval);
            delete _aiVideoPollers[operationId];
            // Update card to error state
            const card = document.querySelector(`.ai-generated-video-card[data-operation-id="${operationId}"]`);
            if (card) {
                updateVideoCardDOM(card, { status: 'error', errorMsg: I18n.t('vai.chat.generationTimeout') });
            }
            return;
        }

        // Update progress text on card
        const statusText = document.querySelector(`.ai-generated-video-card[data-operation-id="${operationId}"] .ai-video-card-status`);
        if (statusText) {
            statusText.innerHTML = '<i class="bi bi-hourglass-split me-1"></i>' + I18n.t('vai.chat.generatingProgress').replace('{seconds}', pollCount * 5);
        }

        try {
            // Ensure operationId slashes are not percent-encoded
            const decodedOpId = decodeURIComponent(operationId);
            const resp = await App.apiRequest('/llm/video/' + decodedOpId);

            if (resp.done) {
                clearInterval(pollInterval);
                delete _aiVideoPollers[operationId];

                // Extract video URL from result
                let videoUrl = null;
                if (resp.result) {
                    // Veo API structure: result.generateVideoResponse.generatedSamples[].video.uri
                    const genResp = resp.result.generateVideoResponse || resp.result;
                    const samples = genResp.generatedSamples || (Array.isArray(genResp) ? genResp : []);
                    for (let i = 0; i < samples.length; i++) {
                        const sample = samples[i];
                        if (sample.video) {
                            if (sample.video.uri) {
                                videoUrl = sample.video.uri;
                            } else if (sample.video.bytesBase64Encoded) {
                                const mime = sample.video.mimeType || 'video/mp4';
                                videoUrl = 'data:' + mime + ';base64,' + sample.video.bytesBase64Encoded;
                            }
                            break;
                        }
                    }
                }

                const card = document.querySelector(`.ai-generated-video-card[data-operation-id="${operationId}"]`);
                if (card) {
                    updateVideoCardDOM(card, {
                        status: videoUrl ? 'done' : 'error',
                        videoUrl: videoUrl,
                        errorMsg: videoUrl ? null : I18n.t('vai.chat.cannotExtractVideo')
                    });
                }

                // Update message extra_fields in DB
                if (messageId) {
                    try {
                        const msgData = await App.apiRequest('/messages/' + messageId);
                        if (msgData && msgData.extra_fields) {
                            const updatedExtra = { ...msgData.extra_fields };
                            updatedExtra.video_info = {
                                ...updatedExtra.video_info,
                                status: videoUrl ? 'done' : 'error',
                                video_url: videoUrl || null
                            };
                            await App.apiRequest('/messages/' + messageId, {
                                method: 'PATCH',
                                body: JSON.stringify({ extra_fields: updatedExtra })
                            });
                            console.log('[vAi] Updated message extra_fields for video', operationId);
                        }
                    } catch (e) {
                        console.warn('[vAi] Failed to update message extra_fields for video:', e);
                    }
                }

            } else if (resp.error) {
                clearInterval(pollInterval);
                delete _aiVideoPollers[operationId];
                const errMsg = typeof resp.error === 'object' ? (resp.error.message || 'Generation failed') : (resp.error || 'Generation failed');
                const card = document.querySelector(`.ai-generated-video-card[data-operation-id="${operationId}"]`);
                if (card) {
                    updateVideoCardDOM(card, { status: 'error', errorMsg: errMsg });
                }
            }
        } catch (e) {
            console.warn('[vAi] Video poll error for', operationId, ':', e);
        }
    }, 5000); // Poll every 5 seconds

    _aiVideoPollers[operationId] = pollInterval;
}

// Update a video card DOM element with completed/failed/done state
function updateVideoCardDOM(card, data) {
    card.setAttribute('data-video-status', data.status);
    const prompt = card.getAttribute('data-prompt') || I18n.t('vai.chat.aiVideo');
    const aspect = card.getAttribute('data-aspect') || '16:9';
    const duration = card.getAttribute('data-duration') || '8s';

    if (data.status === 'done' && data.videoUrl) {
        card.innerHTML = `
            <div class="ai-video-card-player">
                <video controls preload="metadata" style="width: 100%; border-radius: 8px;">
                    <source src="${data.videoUrl}" type="video/mp4">
                    ${I18n.t('vai.chat.browserNoVideoSupport')}
                </video>
            </div>
            <div class="ai-video-card-footer">
                <div class="ai-video-card-info">
                    <div class="ai-video-card-title">${escapeHtml(prompt)}</div>
                    <div class="ai-video-card-meta">
                        <span class="ai-video-card-spec">${aspect} | ${duration}</span>
                    </div>
                </div>
                <a href="${data.videoUrl}" download="vai-video-${Date.now()}.mp4" class="btn btn-sm btn-primary ai-video-card-download" title="${I18n.t('vai.chat.downloadVideo')}">
                    <i class="bi bi-download me-1"></i>${I18n.t('vai.common.download')}
                </a>
            </div>`;
    } else if (data.status === 'error') {
        card.innerHTML = `
            <div class="ai-video-card-icon" style="color: #dc3545;">
                <i class="bi bi-exclamation-triangle"></i>
            </div>
            <div class="ai-video-card-info">
                <div class="ai-video-card-title">${escapeHtml(prompt)}</div>
                <div class="ai-video-card-meta">
                    <span class="ai-video-card-spec">${aspect} | ${duration}</span>
                    <span class="ai-video-card-status" style="color: #dc3545;">${escapeHtml(data.errorMsg || I18n.t('vai.chat.generationFailed'))}</span>
                </div>
            </div>`;
    }
}

// Start polling for a generating document. Updates the DOM card and message
// extra_fields when the document completes (or fails).
function startDocGenerationPolling(docId, messageId) {
    if (_aiDocPollers[docId]) return; // Already polling

    console.log('[vAi] Starting poll for doc:', docId, 'msgId:', messageId);

    const pollInterval = setInterval(async () => {
        try {
            const doc = await App.apiRequest(`/ai/documents/${docId}`);
            console.log('[vAi] Poll result for doc', docId, ':', doc.status);

            if (doc.status === 'completed' || doc.status === 'failed') {
                clearInterval(pollInterval);
                delete _aiDocPollers[docId];

                // Update the card in the DOM
                const card = document.querySelector(`.ai-generated-doc-card[data-doc-id="${docId}"]`);
                if (card) {
                    updateDocCardDOM(card, doc);
                }

                // Update the message extra_fields in DB so reload shows correct state
                if (messageId) {
                    try {
                        // Load current message to get existing extra_fields
                        const msgData = await App.apiRequest(`/messages/${messageId}`);
                        if (msgData && msgData.extra_fields) {
                            const updatedExtra = { ...msgData.extra_fields };
                            updatedExtra.doc_info = {
                                id: doc.id,
                                title: doc.title,
                                doc_type: doc.doc_type,
                                status: doc.status,
                                file_url: doc.file_url || `/api/v1/ai/documents/${docId}/download`,
                                file_size: doc.file_size || 0
                            };
                            await App.apiRequest(`/messages/${messageId}`, {
                                method: 'PATCH',
                                body: JSON.stringify({ extra_fields: updatedExtra })
                            });
                            console.log('[vAi] Updated message extra_fields for doc', docId);
                        }
                    } catch (e) {
                        console.warn('[vAi] Failed to update message extra_fields:', e);
                    }
                }

                // Also update content text if completed
                if (doc.status === 'completed' && messageId) {
                    try {
                        const typeLabels = { docx: 'Word', xlsx: 'Excel', pptx: 'PowerPoint', pdf: 'PDF' };
                        const label = typeLabels[doc.doc_type] || doc.doc_type;
                        await App.apiRequest(`/messages/${messageId}`, {
                            method: 'PATCH',
                            body: JSON.stringify({ content: I18n.t('vai.chat.docGenerated').replace('{label}', label).replace('{title}', doc.title) })
                        });
                    } catch (e) { /* non-critical */ }
                }
            }
        } catch (e) {
            console.warn('[vAi] Poll error for doc', docId, ':', e);
        }
    }, 3000); // Poll every 3 seconds

    _aiDocPollers[docId] = pollInterval;
}

// Update a document card DOM element with completed/failed state
function updateDocCardDOM(card, doc) {
    const docTypeIcons = {
        docx: { icon: 'bi-file-earmark-word', color: '#2b579a', label: 'Word' },
        xlsx: { icon: 'bi-file-earmark-excel', color: '#217346', label: 'Excel' },
        pptx: { icon: 'bi-file-earmark-ppt', color: '#d24726', label: 'PowerPoint' },
        pdf:  { icon: 'bi-file-earmark-pdf', color: '#dc3545', label: 'PDF' }
    };
    const typeInfo = docTypeIcons[doc.doc_type] || { icon: 'bi-file-earmark', color: '#6c757d', label: doc.doc_type };

    card.setAttribute('data-doc-status', doc.status);

    if (doc.status === 'completed') {
        const fileSizeStr = doc.file_size ? formatFileSize(doc.file_size) : '';
        const downloadUrl = doc.file_url || `/api/v1/ai/documents/${doc.id}/download`;
        card.innerHTML = `
            <div class="ai-doc-card-icon" style="color: ${typeInfo.color};">
                <i class="bi ${typeInfo.icon}"></i>
            </div>
            <div class="ai-doc-card-info">
                <div class="ai-doc-card-title">${escapeHtml(doc.title || I18n.t('vai.chat.document'))}</div>
                <div class="ai-doc-card-meta">
                    <span class="ai-doc-card-type">${typeInfo.label} (.${doc.doc_type})</span>
                    ${fileSizeStr ? `<span class="ai-doc-card-size">${fileSizeStr}</span>` : ''}
                </div>
            </div>
            <a href="${downloadUrl}" class="btn btn-sm btn-primary ai-doc-card-download" title="${I18n.t('vai.chat.downloadDocument')}" download>
                <i class="bi bi-download me-1"></i>${I18n.t('vai.common.download')}
            </a>`;
    } else if (doc.status === 'failed') {
        card.innerHTML = `
            <div class="ai-doc-card-icon" style="color: #dc3545;">
                <i class="bi bi-exclamation-triangle"></i>
            </div>
            <div class="ai-doc-card-info">
                <div class="ai-doc-card-title">${escapeHtml(doc.title || I18n.t('vai.chat.document'))}</div>
                <div class="ai-doc-card-meta">
                    <span class="ai-doc-card-type">${typeInfo.label} (.${doc.doc_type})</span>
                    <span class="ai-doc-card-size" style="color: #dc3545;">${I18n.t('vai.chat.generationFailed')}</span>
                </div>
            </div>`;
    }
}

// File upload state
let aiPendingFiles = []; // Array of { file: File, id: string }

// [DEPRECATED] Image generation keyword detection — now handled by backend tool calling.
// Kept for reference; no longer used in sendAIMessage flow.
const AI_IMAGE_GEN_KEYWORDS = [
    '生成圖片', '生成圖像', '生成一張圖', '生成一幅',
    '畫一張', '畫一幅', '畫一個', '幫我畫', '畫圖',
    '產生圖片', '產生圖像', '製作圖片',
    'generate image', 'generate a image', 'generate an image',
    'create image', 'create a image', 'create an image',
    'draw a', 'draw an', 'draw me',
    'make image', 'make a image', 'make an image',
    '幫我生成', '請畫', '請生成',
];

// Check if user message is an image generation request
function isImageGenRequest(text) {
    const lower = text.toLowerCase().trim();
    return AI_IMAGE_GEN_KEYWORDS.some(kw => lower.includes(kw.toLowerCase()));
}

// Extract prompt from image generation request (remove trigger keywords)
function extractImagePrompt(text) {
    let prompt = text.trim();
    // Remove common prefixes to get the actual image description
    const prefixes = [
        '幫我生成一張', '幫我生成一幅', '幫我生成',
        '生成一張', '生成一幅', '生成圖片', '生成圖像', '生成一張圖',
        '畫一張', '畫一幅', '畫一個', '幫我畫一張', '幫我畫一幅', '幫我畫',
        '產生圖片', '產生圖像', '製作圖片',
        '請畫一張', '請畫一幅', '請畫', '請生成一張', '請生成一幅', '請生成',
        'generate an image of', 'generate a image of', 'generate image of',
        'generate an image', 'generate a image', 'generate image',
        'create an image of', 'create a image of', 'create image of',
        'create an image', 'create a image', 'create image',
        'draw me a', 'draw me an', 'draw me',
        'draw a', 'draw an',
        'make an image of', 'make a image of', 'make image of',
        'make an image', 'make a image', 'make image',
        '畫圖',
    ];
    // Sort by length descending so longer prefixes match first
    const sorted = prefixes.slice().sort((a, b) => b.length - a.length);
    for (const pfx of sorted) {
        if (prompt.toLowerCase().startsWith(pfx.toLowerCase())) {
            prompt = prompt.substring(pfx.length).trim();
            break;
        }
    }
    // Remove leading punctuation/colon
    prompt = prompt.replace(/^[:：,，\s]+/, '');
    return prompt || text.trim();
}

// Call image generation API
async function callImageGenAPI(prompt) {
    const response = await App.apiRequest('/llm/image', {
        method: 'POST',
        body: JSON.stringify({
            prompt: prompt,
            aspect_ratio: '1:1'
        })
    });
    return response;
}

// 初始化 AI Chat
document.addEventListener('DOMContentLoaded', async function() {
    if (!App.checkAuth()) return;
    
    // Show loading overlay immediately during init
    showChatLoadingOverlay();
    
    // 獲取當前用戶ID
    await loadCurrentUser();
    
    // 載入 AI 設置（從後端獲取）
    await loadAISettings();
    
    // 初始化 AI Chat 輸入框
    const aiMessageInput = document.getElementById('aiMessageInput');
    if (aiMessageInput) {
        aiMessageInput.addEventListener('input', function() {
            this.style.height = 'auto';
            this.style.height = Math.min(this.scrollHeight, 120) + 'px';
            updateAIMicSendToggle();
        });
        
        aiMessageInput.addEventListener('keydown', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendAIMessage(e);
            }
        });
    }
    
    // Initialize mic/send button toggle (send hidden by default, mic visible)
    updateAIMicSendToggle();
    
    // Initialize file upload - change listener (button triggers via <label for="aiFileInput">)
    const aiFileInput = document.getElementById('aiFileInput');
    if (aiFileInput) {
        aiFileInput.addEventListener('change', function(e) {
            if (this.files.length > 0) {
                handleAIFileSelect(e);
            }
        });
    }

    updateAIMicButtonState();
    
    // 為 AI floating button 添加移動端觸摸事件支持
    const vaiBtn = document.getElementById('aiChatFloatingBtn');
    if (vaiBtn) {
        // 隱藏 tooltip 的輔助函數
        const hideTooltip = () => {
            if (typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
                const tooltip = bootstrap.Tooltip.getInstance(vaiBtn);
                if (tooltip) {
                    tooltip.hide();
                }
            }
        };
        
        // 添加觸摸事件（移動端）
        vaiBtn.addEventListener('touchstart', hideTooltip, { passive: true });
        vaiBtn.addEventListener('touchend', hideTooltip, { passive: true });
    }
});

// 載入當前用戶信息
async function loadCurrentUser() {
    try {
        const response = await App.apiRequest('/user/me');
        aiCurrentUserId = response.id;
    } catch (error) {
        console.error('Failed to load current user:', error);
        // 如果失敗，嘗試從token解析（備用方案）
        try {
            const token = localStorage.getItem('auth_token');
            if (token) {
                const payload = JSON.parse(atob(token.split('.')[1]));
                if (payload.user_id) {
                    aiCurrentUserId = payload.user_id;
                }
            }
        } catch (e) {
            console.error('Failed to parse token:', e);
        }
    }
}

// 顯示 AI Chat 模態框
function showAIChatModal() {
    // 隱藏 AI floating button 的 tooltip
    const vaiBtn = document.getElementById('aiChatFloatingBtn');
    if (vaiBtn && typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
        const tooltip = bootstrap.Tooltip.getInstance(vaiBtn);
        if (tooltip) {
            tooltip.hide();
        }
    }
    
    const modalElement = document.getElementById('aiChatModal');
    if (!modalElement) return;
    
    // 使用 getOrCreateInstance 避免重複實例化導致錯誤
    const modal = bootstrap.Modal.getOrCreateInstance(modalElement);
    modal.show();
    
    // 載入對話列表
    loadAiConversations();
    
    // 滾動到底部（只綁定一次）
    if (!modalElement._aiChatListenerAdded) {
        modalElement.addEventListener('shown.bs.modal', function() {
            setTimeout(() => {
                const aiChatMessages = document.getElementById('aiChatMessages');
                if (aiChatMessages) {
                    aiChatMessages.scrollTop = aiChatMessages.scrollHeight;
                }
            }, 200);
        });
        modalElement._aiChatListenerAdded = true;
    }
}

// 載入 AI 設置（從後端獲取）
async function loadAISettings() {
    try {
        const response = await App.apiRequest('/llm/config');
        aiEndpoint = response.enabled ? (response.endpoint || 'gemini') : '';
        aiModel = response.model || 'gpt-3.5-turbo';
    } catch (error) {
        console.error('Failed to load LLM config:', error);
        aiEndpoint = '';
        aiModel = 'gpt-3.5-turbo';
    }
}

// ============================================
// 對話列表管理
// ============================================

// 載入所有 AI 對話
async function loadAiConversations() {
    // Show loading overlay during initial conversation load
    if (!aiCurrentConversationId) {
        showChatLoadingOverlay();
    }

    try {
        const response = await App.apiRequest('/ai/conversations');
        aiConversations = response.data || [];
        console.log('[vAi] Loaded', aiConversations.length, 'conversations');
        renderAiConversationList();
        
        // 如果有對話且沒有選中的，選中最新的
        // 保留暫存對話（如果有的話）
        const pendingConv = aiConversations.find(c => c.id === PENDING_CONV_ID);
        aiConversations = response.data || [];
        if (pendingConv) {
            aiConversations.unshift(pendingConv);
        }

        if (aiCurrentConversationId === PENDING_CONV_ID) {
            // 正在使用暫存對話，保持不動
            hideChatLoadingOverlay();
            renderAiConversationList();
        } else if (aiConversations.length > 0 && !aiCurrentConversationId) {
            // 有對話但沒選中 — 不自動切換，由頁面初始化決定（layout 會自動建新對話）
            hideChatLoadingOverlay();
            renderAiConversationList();
        } else if (aiConversations.length === 0) {
            // 沒有對話，自動建立一個新對話
            hideChatLoadingOverlay();
            createNewAiConversation();
        } else if (aiCurrentConversationId) {
            // 已有選中的對話，確認它仍存在
            const exists = aiConversations.find(c => c.id === aiCurrentConversationId);
            if (exists) {
                // 只更新列表 active 狀態，不重新載入消息（避免重複）
                hideChatLoadingOverlay();
                renderAiConversationList();
            } else {
                // 對話已不存在，切到第一個
                switchToConversation(aiConversations[0].id);
            }
        }
    } catch (error) {
        console.error('Failed to load AI conversations:', error);
        hideChatLoadingOverlay();
    }
}

// 渲染對話列表
function renderAiConversationList() {
    const container = document.getElementById('aiConversationList');
    if (!container) return;

    if (aiConversations.length === 0) {
        container.innerHTML = `
            <div class="text-center text-muted p-3" style="font-size: 0.8rem;">
                <i class="bi bi-chat-dots mb-2 d-block" style="font-size: 1.5rem;"></i>
                ${I18n.t('vai.chat.noConversations')}<br>${I18n.t('vai.chat.noConversationsHint')}
            </div>
        `;
        return;
    }

    container.innerHTML = aiConversations.map(conv => {
        const isActive = conv.id === aiCurrentConversationId;
        const timeStr = formatConvTime(conv.updated_at);
        const preview = conv.last_message || '';
        return `
            <div class="ai-conversation-item ${isActive ? 'active' : ''}" onclick="switchToConversation('${conv.id}')" data-conv-id="${conv.id}">
                <i class="bi bi-chat-text conv-icon"></i>
                <div class="conv-info">
                    <div class="conv-title">${escapeHtml(conv.title)}</div>
                    ${preview ? `<div class="conv-preview">${escapeHtml(preview)}</div>` : ''}
                </div>
                <span class="conv-time">${timeStr}</span>
            </div>
        `;
    }).join('');
}

// 格式化對話時間
function formatConvTime(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    const now = new Date();
    const diff = now - date;
    const oneDay = 86400000;
    
    if (diff < oneDay && date.getDate() === now.getDate()) {
        return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
    } else if (diff < 7 * oneDay) {
        return date.toLocaleDateString(undefined, { weekday: 'short' });
    } else {
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    }
}

// 切換到指定對話
async function switchToConversation(convId) {
    aiCurrentConversationId = convId;
    
    // 更新列表 active 狀態
    document.querySelectorAll('.ai-conversation-item').forEach(el => {
        el.classList.toggle('active', el.dataset.convId === convId);
    });
    
    // 更新標題列
    updateConvHeader();
    
    // 載入該對話的消息
    await loadAIChatHistory();
    
    // 手機端：關閉 sidebar
    closeMobileSidebar();
}

// 更新對話標題列
function updateConvHeader() {
    const header = document.getElementById('aiChatConvHeader');
    const titleEl = document.getElementById('aiChatConvTitle');
    if (!header || !titleEl) return;

    if (aiCurrentConversationId) {
        header.classList.remove('d-none');
        header.classList.add('d-flex');
        const conv = aiConversations.find(c => c.id === aiCurrentConversationId);
        titleEl.textContent = conv ? conv.title : I18n.t('vai.chat.conversation');
    } else {
        header.classList.add('d-none');
        header.classList.remove('d-flex');
    }
}

// 顯示歡迎畫面（無對話時）
function showWelcomeScreen() {
    const container = document.getElementById('aiChatMessages');
    if (!container) return;
    container.innerHTML = `
        <div class="text-center p-4 text-muted" style="margin-top: 2rem;">
            <img src="/static/vai.png" alt="vAi" style="width: 120px; height: 120px; object-fit: contain; margin-bottom: 1rem;">
            <p class="mt-2">${I18n.t('vai.chat.startChat')}</p>
            <button class="btn btn-primary btn-sm mt-2" onclick="createNewAiConversation()">
                <i class="bi bi-plus-lg me-1"></i>${I18n.t('vai.chat.newChat')}
            </button>
            <div class="mt-4">
                <p class="text-muted mb-2" style="font-size: 0.85rem;">${I18n.t('vai.chat.suggestionsLabel')}</p>
                <div class="d-flex flex-wrap justify-content-center gap-2">
                    <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickGenerateImage'))">
                        <i class="bi bi-image me-1"></i>${I18n.t('vai.chat.quickGenerateImage')}
                    </button>
                    <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickGenerateVideo'))">
                        <i class="bi bi-camera-video me-1"></i>${I18n.t('vai.chat.quickGenerateVideo')}
                    </button>
                    <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickGeneratePptx'))">
                        <i class="bi bi-file-earmark-slides me-1"></i>${I18n.t('vai.chat.quickGeneratePptx')}
                    </button>
                    <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickQueryOrders'))">
                        <i class="bi bi-search me-1"></i>${I18n.t('vai.chat.quickQueryOrders')}
                    </button>
                </div>
            </div>
        </div>
    `;
}

// 新建對話（延遲建立：只在前端建立暫存，等第一條消息才寫入 DB）
function createNewAiConversation() {
    // 如果已有 pending 對話，直接切換過去
    if (aiConversations.some(c => c.id === PENDING_CONV_ID)) {
        switchToConversation(PENDING_CONV_ID);
        return;
    }

    const chatTitle = I18n.t('vai.chat.newConversation');
    const pendingConv = {
        id: PENDING_CONV_ID,
        title: (!chatTitle || chatTitle === 'vai.chat.newConversation') ? '新對話' : chatTitle,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        _pending: true
    };

    aiConversations.unshift(pendingConv);
    renderAiConversationList();
    switchToConversation(PENDING_CONV_ID);
}

// 重命名當前對話
async function renameCurrentConversation() {
    if (!aiCurrentConversationId) return;
    
    const conv = aiConversations.find(c => c.id === aiCurrentConversationId);
    if (!conv) return;
    
    const newTitle = prompt(I18n.t('vai.chat.renamePrompt'), conv.title);
    if (!newTitle || newTitle.trim() === '' || newTitle === conv.title) return;

    // 暫存對話只在前端更新
    if (aiCurrentConversationId === PENDING_CONV_ID) {
        conv.title = newTitle.trim();
        renderAiConversationList();
        updateConvHeader();
        return;
    }
    
    try {
        await App.apiRequest(`/ai/conversations/${aiCurrentConversationId}`, {
            method: 'PUT',
            body: JSON.stringify({ title: newTitle.trim() })
        });
        
        conv.title = newTitle.trim();
        renderAiConversationList();
        updateConvHeader();
    } catch (error) {
        console.error('Failed to rename conversation:', error);
        App.showAlert(I18n.t('vai.chat.renameFailed'), 'danger');
    }
}

// 刪除當前對話
async function deleteCurrentConversation() {
    if (!aiCurrentConversationId) return;

    // 暫存對話直接移除，不需要呼叫 API
    if (aiCurrentConversationId === PENDING_CONV_ID) {
        aiConversations = aiConversations.filter(c => c.id !== PENDING_CONV_ID);
        aiCurrentConversationId = null;
        renderAiConversationList();
        if (aiConversations.length > 0) {
            switchToConversation(aiConversations[0].id);
        } else {
            showWelcomeScreen();
            updateConvHeader();
        }
        return;
    }
    
    if (!confirm(I18n.t('vai.chat.deleteConfirm'))) return;
    
    try {
        await App.apiRequest(`/ai/conversations/${aiCurrentConversationId}`, {
            method: 'DELETE'
        });
        
        aiConversations = aiConversations.filter(c => c.id !== aiCurrentConversationId);
        aiCurrentConversationId = null;
        renderAiConversationList();
        
        // 切換到下一個對話或顯示歡迎
        if (aiConversations.length > 0) {
            switchToConversation(aiConversations[0].id);
        } else {
            showWelcomeScreen();
            updateConvHeader();
        }
    } catch (error) {
        console.error('Failed to delete conversation:', error);
        App.showAlert(I18n.t('vai.chat.deleteFailed'), 'danger');
    }
}

// 手機端 sidebar 開關
function toggleAiSidebar() {
    const sidebar = document.getElementById('aiChatSidebar');
    if (!sidebar) return;
    sidebar.classList.toggle('show');
    const overlay = document.getElementById('aiChatSidebarOverlay');
    if (overlay) {
        overlay.classList.toggle('show', sidebar.classList.contains('show'));
    }
}

function closeMobileSidebar() {
    const sidebar = document.getElementById('aiChatSidebar');
    if (sidebar) sidebar.classList.remove('show');
    const overlay = document.getElementById('aiChatSidebarOverlay');
    if (overlay) overlay.classList.remove('show');
}

// HTML escape 工具
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Sanitize HTML: escape everything, then restore safe <a> tags and auto-linkify URLs
function sanitizeHtml(text) {
    if (!text) return '';

    // If marked.js + DOMPurify are available, render Markdown properly.
    if (typeof marked !== 'undefined' && typeof DOMPurify !== 'undefined') {
        // Backend convertLinksToHTML() may have already converted URLs to <a> tags.
        // marked.js will pass them through as raw HTML, so they are preserved.
        // Configure marked for safe rendering with line breaks.
        const html = marked.parse(text, { breaks: true, gfm: true });
        return DOMPurify.sanitize(html, { ADD_ATTR: ['target'] });
    }

    // Fallback: no markdown library — escape HTML and restore <a> tags.
    let escaped = escapeHtml(text);
    // Restore <a href="..." ...>...</a> that were escaped.
    escaped = escaped.replace(
        /&lt;a\s+href=(["']|&quot;|&#039;|&apos;)(https?:\/\/.*?)\1[^]*?&gt;([\s\S]*?)&lt;\/a&gt;/gi,
        function(match, quote, url, content) {
            var cleanUrl = url.replace(/&amp;/g, '&');
            return '<a href="' + cleanUrl + '" target="_blank" rel="noopener noreferrer">' + content + '</a>';
        }
    );
    // Auto-linkify plain URLs that are not already inside an <a> tag.
    escaped = escaped.replace(
        /(?<!href=")(?<!noreferrer">)(https?:\/\/[^\s<&]+)/g,
        '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>'
    );
    return escaped;
}

// Build a payment link card DOM element with copy button
function buildPaymentLinkCard(paymentUrl) {
    const card = document.createElement('div');
    card.className = 'ai-payment-link-card mt-2';
    const safeUrl = escapeHtml(paymentUrl);
    const copyLabel = I18n.t('vai.chat.copyPaymentLink') || 'Copy Link';
    const openLabel = I18n.t('vai.chat.openPaymentLink') || 'Open';
    card.innerHTML = `
        <div class="d-flex align-items-center gap-2 p-2" style="background: #f8f9fa; border: 1px solid #dee2e6; border-radius: 8px;">
            <i class="bi bi-credit-card" style="font-size: 1.1rem; color: #0d6efd;"></i>
            <span class="text-truncate flex-grow-1" style="font-size: 0.85rem; color: #495057; max-width: 200px;" title="${safeUrl}">${safeUrl}</span>
            <button class="btn btn-sm btn-primary d-inline-flex align-items-center gap-1" onclick="copyPaymentLink(this, '${safeUrl}')" style="white-space: nowrap;">
                <i class="bi bi-clipboard"></i>
                <span>${copyLabel}</span>
            </button>
            <a href="${safeUrl}" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-outline-secondary d-inline-flex align-items-center gap-1" style="white-space: nowrap; text-decoration: none;">
                <i class="bi bi-box-arrow-up-right"></i>
                <span>${openLabel}</span>
            </a>
        </div>
    `;
    return card;
}

// Copy payment link to clipboard and show feedback
function copyPaymentLink(btn, url) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(url).then(() => {
            const iconEl = btn.querySelector('i');
            const textEl = btn.querySelector('span');
            const origIcon = iconEl.className;
            const origText = textEl.textContent;
            iconEl.className = 'bi bi-check-lg';
            textEl.textContent = I18n.t('vai.chat.copied') || 'Copied!';
            btn.classList.remove('btn-primary');
            btn.classList.add('btn-success');
            setTimeout(() => {
                iconEl.className = origIcon;
                textEl.textContent = origText;
                btn.classList.remove('btn-success');
                btn.classList.add('btn-primary');
            }, 2000);
        }).catch(() => {
            fallbackCopyPaymentLink(url);
        });
    } else {
        fallbackCopyPaymentLink(url);
    }
}

// Fallback copy using textarea
function fallbackCopyPaymentLink(url) {
    const textarea = document.createElement('textarea');
    textarea.value = url;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
        document.execCommand('copy');
        if (typeof App !== 'undefined' && App.showAlert) {
            App.showAlert(I18n.t('vai.chat.copied') || 'Copied!', 'success');
        }
    } catch (e) {
        console.error('Copy failed:', e);
    }
    document.body.removeChild(textarea);
}

// Format file size to human-readable string
function formatFileSize(bytes) {
    if (!bytes || bytes === 0) return '';
    const units = ['B', 'KB', 'MB', 'GB'];
    let i = 0;
    let size = bytes;
    while (size >= 1024 && i < units.length - 1) {
        size /= 1024;
        i++;
    }
    return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

// ============================================
// Chat Loading Overlay
// ============================================

// Show loading overlay on aiChatMessages (similar to CMS form-loading-overlay)
function showChatLoadingOverlay() {
    const container = document.getElementById('aiChatMessages');
    if (!container) return;

    // Ensure container is positioned for absolute overlay
    if (window.getComputedStyle(container).position === 'static') {
        container.style.position = 'relative';
    }

    let overlay = document.getElementById('aiChatLoadingOverlay');
    if (overlay) {
        overlay.style.display = 'flex';
        return;
    }

    // Resolve loading text: try vai.chat.loadingChat first, fallback to common.loadingData, then hardcoded
    const getLoadingText = () => {
        if (typeof I18n === 'undefined' || !I18n.t) return '';
        const vaiText = I18n.t('vai.chat.loadingChat');
        if (vaiText && vaiText !== 'vai.chat.loadingChat') return vaiText;
        const commonText = I18n.t('common.loadingData');
        if (commonText && commonText !== 'common.loadingData') return commonText;
        return '';
    };

    overlay = document.createElement('div');
    overlay.id = 'aiChatLoadingOverlay';
    overlay.className = 'ai-chat-loading-overlay';
    overlay.innerHTML = `
        <div class="loading-content">
            <div class="spinner-border text-primary" role="status" style="width: 3rem; height: 3rem;">
                <span class="visually-hidden">Loading...</span>
            </div>
            <p class="mt-3 text-muted" id="aiChatLoadingText">${getLoadingText()}</p>
        </div>`;
    container.appendChild(overlay);

    // If i18n is not ready yet, update the text once it becomes ready
    if (typeof I18n !== 'undefined' && typeof I18n.whenReady === 'function' && !I18n._ready) {
        I18n.whenReady(5000).then(() => {
            const el = document.getElementById('aiChatLoadingText');
            if (el) el.textContent = getLoadingText();
        }).catch(() => { /* timeout, ignore */ });
    }
}

// Hide loading overlay on aiChatMessages
function hideChatLoadingOverlay() {
    const overlay = document.getElementById('aiChatLoadingOverlay');
    if (overlay) {
        overlay.style.display = 'none';
    }
}

// ============================================
// 消息功能
// ============================================

// 載入 AI 對話歷史
async function loadAIChatHistory() {
    if (!aiCurrentConversationId) {
        aiChatHistory = [];
        hideChatLoadingOverlay();
        showWelcomeScreen();
        return;
    }

    // 暫存對話尚無消息，直接顯示空聊天介面
    if (aiCurrentConversationId === PENDING_CONV_ID) {
        aiChatHistory = [];
        hideChatLoadingOverlay();
        renderAIChatMessages([]);
        return;
    }

    // Show loading overlay while fetching messages
    showChatLoadingOverlay();

    try {
        const response = await App.apiRequest(`/ai/conversations/${aiCurrentConversationId}/messages`);
        const messages = response.data || [];
        aiChatHistory = messages;
        hideChatLoadingOverlay();
        renderAIChatMessages(messages);

        // Restart polling for any messages with doc_info.status === "generating"
        messages.forEach(msg => {
            if (msg.extra_fields && msg.extra_fields.doc_info &&
                msg.extra_fields.doc_info.status === 'generating' &&
                msg.extra_fields.doc_info.id) {
                startDocGenerationPolling(msg.extra_fields.doc_info.id, msg.id);
            }
            // Restart polling for any messages with video_info.status === "generating"
            if (msg.extra_fields && msg.extra_fields.video_info &&
                msg.extra_fields.video_info.status === 'generating' &&
                msg.extra_fields.video_info.operation_id) {
                startVideoGenerationPolling(msg.extra_fields.video_info.operation_id, msg.id);
            }
        });
        
        // 滾動到底部
        setTimeout(() => {
            const aiChatMessages = document.getElementById('aiChatMessages');
            if (aiChatMessages) {
                aiChatMessages.scrollTop = aiChatMessages.scrollHeight;
            }
        }, 100);
    } catch (error) {
        console.error('Failed to load AI chat history:', error);
        hideChatLoadingOverlay();
    }
}

// 顯示 AI 思考中的指示器
function showAIThinking() {
    const container = document.getElementById('aiChatMessages');
    if (!container) return;
    
    const existingThinking = container.querySelector('.ai-thinking-indicator');
    if (existingThinking) existingThinking.remove();
    
    const thinkingDiv = document.createElement('div');
    thinkingDiv.className = 'ai-thinking-indicator d-flex justify-content-start mb-2';
    thinkingDiv.innerHTML = `
        <div class="avatar-circle me-2 flex-shrink-0" style="width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;">
            <i class="bi bi-robot"></i>
        </div>
        <div class="message-bubble ai position-relative" style="max-width: 70%;">
            <div class="d-flex align-items-center">
                <span class="thinking-dots">
                    <span class="dot"></span>
                    <span class="dot"></span>
                    <span class="dot"></span>
                </span>
                <span class="ms-2" style="opacity: 0.7;">${I18n.t('vai.chat.aiThinking')}</span>
            </div>
        </div>
    `;
    
    container.appendChild(thinkingDiv);
    setTimeout(() => { container.scrollTop = container.scrollHeight; }, 100);
}

// 隱藏 AI 思考中的指示器
function hideAIThinking() {
    const container = document.getElementById('aiChatMessages');
    if (!container) return;
    const el = container.querySelector('.ai-thinking-indicator');
    if (el) el.remove();
}

// 渲染 AI Chat 消息
function renderAIChatMessages(messages) {
    const container = document.getElementById('aiChatMessages');
    if (!container) return;
    
    hideAIThinking();
    
    if (messages.length === 0) {
        container.innerHTML = `
            <div class="text-center p-4 text-muted">
                <i class="bi bi-robot fs-1"></i>
                <p class="mt-2">${I18n.t('vai.chat.startChat')}</p>
                ${!aiEndpoint ? `<small class="text-muted d-block mt-2">${I18n.t('vai.chat.llmNotConfigured')}</small>` : ''}
            </div>
        `;
        renderQuickActionSuggestions();
        return;
    }

    container.innerHTML = messages.map(msg => {
        let isUser = false;
        if (msg.extra_fields && msg.extra_fields.role) {
            isUser = msg.extra_fields.role === 'user';
        } else if (msg.from_user_id && aiCurrentUserId) {
            isUser = msg.from_user_id === aiCurrentUserId;
        }
        
        const timeStr = new Date(msg.created_at).toLocaleString(undefined, {
            month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
        });

        // Build attachment cards HTML if present
        let attachmentHtml = '';
        if (msg.extra_fields && msg.extra_fields.attachments && msg.extra_fields.attachments.length > 0) {
            attachmentHtml = '<div class="ai-msg-attachments">' +
                msg.extra_fields.attachments.map(att => {
                    const iconClass = getFileIconClass(att.filename);
                    const displayName = att.filename.length > 30 ? att.filename.substring(0, 27) + '...' : att.filename;
                    const fileUrl = att.file_url || '';
                    const isImage = /\.(jpg|jpeg|png)$/i.test(att.filename);
                    const isAudio = /\.(webm|wav|mp3|m4a|ogg|opus)$/i.test(att.filename);
                    if (isAudio && att.data) {
                        // Inline audio player for voice messages (base64 data)
                        const mimeType = att.mime_type || 'audio/webm';
                        const dataUrl = 'data:' + mimeType + ';base64,' + att.data;
                        return `<div class="ai-voice-message-player">
                            <audio controls preload="metadata" src="${dataUrl}" style="height:36px; max-width:260px;"></audio>
                        </div>`;
                    }
                    if (isAudio && fileUrl) {
                        // Inline audio player for voice messages (URL)
                        return `<div class="ai-voice-message-player">
                            <audio controls preload="metadata" src="${escapeHtml(fileUrl)}" style="height:36px; max-width:260px;"></audio>
                        </div>`;
                    }
                    if (fileUrl) {
                        if (isImage) {
                            return `<div class="ai-msg-attachment-card" title="${escapeHtml(att.filename)}" onclick="openAiGeneratedImage('${escapeHtml(fileUrl)}')" role="button">
                                <i class="${iconClass}"></i>
                                <span>${escapeHtml(displayName)}</span>
                            </div>`;
                        } else {
                            return `<a href="${escapeHtml(fileUrl)}" download="${escapeHtml(att.filename)}" class="ai-msg-attachment-card" title="${escapeHtml(att.filename)}" style="text-decoration:none;color:inherit;">
                                <i class="${iconClass}"></i>
                                <span>${escapeHtml(displayName)}</span>
                            </a>`;
                        }
                    }
                    // Audio without URL — try base64 data if available
                    if (isAudio && att.data) {
                        const mimeType = att.mime_type || 'audio/webm';
                        const dataUrl = 'data:' + mimeType + ';base64,' + att.data;
                        return `<div class="ai-voice-message-player">
                            <audio controls preload="metadata" src="${dataUrl}" style="height:36px; max-width:260px;"></audio>
                        </div>`;
                    }
                    return `<div class="ai-msg-attachment-card" title="${escapeHtml(att.filename)}">
                        <i class="${iconClass}"></i>
                        <span>${escapeHtml(displayName)}</span>
                    </div>`;
                }).join('') +
            '</div>';
        }

        // Strip [附件: ...] suffix from display content (already shown as cards)
        let displayContent = (msg.content || '');
        if (attachmentHtml) {
            displayContent = displayContent.replace(/\n?\n?\[附件: [^\]]*\]$/, '').trim();
        }
        const contentHtml = displayContent ? `<div class="ai-msg-content">${sanitizeHtml(displayContent)}</div>` : '';

        // Build generated image HTML if present
        let generatedImageHtml = '';
        if (msg.extra_fields && msg.extra_fields.image_urls && msg.extra_fields.image_urls.length > 0) {
            generatedImageHtml = '<div class="ai-generated-images">' +
                msg.extra_fields.image_urls.map((url, idx) => {
                    return `<div class="ai-generated-image-item">
                        <img src="${url}" alt="AI Generated Image ${idx + 1}" loading="lazy" onclick="openAiGeneratedImage(this.src)">
                        <div class="ai-generated-image-actions">
                            <a href="${url}" download="vai-image-${Date.now()}-${idx}.png" class="btn btn-sm btn-outline-secondary" title="${I18n.t('vai.common.download')}">
                                <i class="bi bi-download"></i>
                            </a>
                        </div>
                    </div>`;
                }).join('') +
            '</div>';
        }

        // Build generated document card HTML if present
        let generatedDocHtml = '';
        if (msg.extra_fields && msg.extra_fields.doc_info) {
            const doc = msg.extra_fields.doc_info;
            const docTypeIcons = {
                docx: { icon: 'bi-file-earmark-word', color: '#2b579a', label: 'Word' },
                xlsx: { icon: 'bi-file-earmark-excel', color: '#217346', label: 'Excel' },
                pptx: { icon: 'bi-file-earmark-ppt', color: '#d24726', label: 'PowerPoint' },
                pdf:  { icon: 'bi-file-earmark-pdf', color: '#dc3545', label: 'PDF' }
            };
            const typeInfo = docTypeIcons[doc.doc_type] || { icon: 'bi-file-earmark', color: '#6c757d', label: doc.doc_type };
            const docTitle = doc.title || I18n.t('vai.chat.document');

            if (doc.deleted) {
                // Record was deleted from vai-docs
                generatedDocHtml = `
                    <div class="ai-generated-doc-card" data-doc-id="${doc.id}" data-doc-status="deleted" style="opacity: 0.6;">
                        <div class="ai-doc-card-icon" style="color: #adb5bd;">
                            <i class="bi bi-file-earmark-x"></i>
                        </div>
                        <div class="ai-doc-card-info">
                            <div class="ai-doc-card-title" style="text-decoration: line-through; color: #adb5bd;">${escapeHtml(docTitle)}</div>
                            <div class="ai-doc-card-meta">
                                <span class="ai-doc-card-type" style="color: #adb5bd;">${typeInfo.label} (.${doc.doc_type})</span>
                                <span class="ai-doc-card-size" style="color: #adb5bd;"><i class="bi bi-trash3 me-1"></i>${I18n.t('vai.chat.recordDeleted')}</span>
                            </div>
                        </div>
                    </div>`;
            } else if (doc.status === 'generating') {
                // Spinner card for in-progress document generation
                generatedDocHtml = `
                    <div class="ai-generated-doc-card" data-doc-id="${doc.id}" data-doc-status="generating">
                        <div class="ai-doc-card-icon" style="color: ${typeInfo.color};">
                            <i class="bi ${typeInfo.icon}"></i>
                        </div>
                        <div class="ai-doc-card-info">
                            <div class="ai-doc-card-title">${escapeHtml(docTitle)}</div>
                            <div class="ai-doc-card-meta">
                                <span class="ai-doc-card-type">${typeInfo.label} (.${doc.doc_type})</span>
                                <span class="ai-doc-card-size" style="color: #f0ad4e;"><i class="bi bi-hourglass-split me-1"></i>${I18n.t('vai.chat.docGeneratingStatus')}</span>
                            </div>
                        </div>
                        <div class="spinner-border spinner-border-sm text-primary ai-doc-card-spinner" role="status">
                            <span class="visually-hidden">Loading...</span>
                        </div>
                    </div>`;
            } else if (doc.status === 'failed') {
                // Failed card
                generatedDocHtml = `
                    <div class="ai-generated-doc-card" data-doc-id="${doc.id}" data-doc-status="failed">
                        <div class="ai-doc-card-icon" style="color: #dc3545;">
                            <i class="bi bi-exclamation-triangle"></i>
                        </div>
                        <div class="ai-doc-card-info">
                            <div class="ai-doc-card-title">${escapeHtml(docTitle)}</div>
                            <div class="ai-doc-card-meta">
                                <span class="ai-doc-card-type">${typeInfo.label} (.${doc.doc_type})</span>
                                <span class="ai-doc-card-size" style="color: #dc3545;">${I18n.t('vai.chat.generationFailed')}</span>
                            </div>
                        </div>
                    </div>`;
            } else {
                // Completed card (existing behavior)
                const fileSizeStr = doc.file_size ? formatFileSize(doc.file_size) : '';
                const downloadUrl = doc.file_url || `/api/v1/ai/documents/${doc.id}/download`;

                generatedDocHtml = `
                    <div class="ai-generated-doc-card" data-doc-id="${doc.id}" data-doc-status="completed">
                        <div class="ai-doc-card-icon" style="color: ${typeInfo.color};">
                            <i class="bi ${typeInfo.icon}"></i>
                        </div>
                        <div class="ai-doc-card-info">
                            <div class="ai-doc-card-title">${escapeHtml(docTitle)}</div>
                            <div class="ai-doc-card-meta">
                                <span class="ai-doc-card-type">${typeInfo.label} (.${doc.doc_type})</span>
                                ${fileSizeStr ? `<span class="ai-doc-card-size">${fileSizeStr}</span>` : ''}
                            </div>
                        </div>
                        <a href="${downloadUrl}" class="btn btn-sm btn-primary ai-doc-card-download" title="${I18n.t('vai.chat.downloadDocument')}" download>
                            <i class="bi bi-download me-1"></i>${I18n.t('vai.common.download')}
                        </a>
                    </div>`;
            }
        }
        
        // Build generated video card HTML if present
        let generatedVideoHtml = '';
        if (msg.extra_fields && msg.extra_fields.video_info) {
            const video = msg.extra_fields.video_info;
            const videoPrompt = video.prompt || I18n.t('vai.chat.video');
            const truncatedPrompt = videoPrompt.length > 60 ? videoPrompt.substring(0, 60) + '...' : videoPrompt;

            if (video.deleted) {
                // Record was deleted from vai-video
                generatedVideoHtml = `
                    <div class="ai-generated-video-card" data-video-status="deleted" style="opacity: 0.6;">
                        <div class="ai-video-card-icon" style="color: #adb5bd;">
                            <i class="bi bi-camera-video"></i>
                        </div>
                        <div class="ai-video-card-info">
                            <div class="ai-video-card-title" style="text-decoration: line-through; color: #adb5bd;">${escapeHtml(truncatedPrompt)}</div>
                            <div class="ai-video-card-meta">
                                <span class="ai-video-card-spec" style="color: #adb5bd;">${video.aspect_ratio || '16:9'} | ${video.duration || 8}s</span>
                                <span class="ai-video-card-status" style="color: #adb5bd;"><i class="bi bi-trash3 me-1"></i>${I18n.t('vai.chat.recordDeleted')}</span>
                            </div>
                        </div>
                    </div>`;
            } else if (video.status === 'generating') {
                // Spinner card for in-progress video generation
                generatedVideoHtml = `
                    <div class="ai-generated-video-card" data-operation-id="${video.operation_id}" data-video-status="generating" data-prompt="${escapeHtml(truncatedPrompt)}" data-aspect="${video.aspect_ratio || '16:9'}" data-duration="${video.duration || 8}s">
                        <div class="ai-video-card-icon">
                            <i class="bi bi-camera-video"></i>
                        </div>
                        <div class="ai-video-card-info">
                            <div class="ai-video-card-title">${escapeHtml(truncatedPrompt)}</div>
                            <div class="ai-video-card-meta">
                                <span class="ai-video-card-spec">${video.aspect_ratio || '16:9'} | ${video.duration || 8}s</span>
                                <span class="ai-video-card-status" style="color: #f0ad4e;"><i class="bi bi-hourglass-split me-1"></i>${I18n.t('vai.chat.videoGeneratingStatus')}</span>
                            </div>
                        </div>
                        <div class="spinner-border spinner-border-sm text-primary ai-video-card-spinner" role="status">
                            <span class="visually-hidden">Loading...</span>
                        </div>
                        <div class="ai-video-card-hint"><small class="text-muted"><i class="bi bi-lightbulb me-1"></i>${I18n.t('vai.chat.videoCardHint')}</small></div>
                    </div>`;
            } else if (video.status === 'error' || video.status === 'failed') {
                // Error card
                generatedVideoHtml = `
                    <div class="ai-generated-video-card" data-operation-id="${video.operation_id}" data-video-status="error">
                        <div class="ai-video-card-icon" style="color: #dc3545;">
                            <i class="bi bi-exclamation-triangle"></i>
                        </div>
                        <div class="ai-video-card-info">
                            <div class="ai-video-card-title">${escapeHtml(truncatedPrompt)}</div>
                            <div class="ai-video-card-meta">
                                <span class="ai-video-card-spec">${video.aspect_ratio || '16:9'} | ${video.duration || 8}s</span>
                                <span class="ai-video-card-status" style="color: #dc3545;">${I18n.t('vai.chat.generationFailed')}</span>
                            </div>
                        </div>
                        <div class="ai-video-card-hint"><small class="text-muted"><i class="bi bi-lightbulb me-1"></i>${I18n.t('vai.chat.videoCardHint')}</small></div>
                    </div>`;
            } else if (video.status === 'done') {
                // Completed card with video player
                const videoUrl = video.video_url || '';
                generatedVideoHtml = `
                    <div class="ai-generated-video-card" data-operation-id="${video.operation_id}" data-video-status="done">
                        <div class="ai-video-card-player">
                            <video controls preload="metadata" style="width: 100%; border-radius: 8px;">
                                <source src="${videoUrl}" type="video/mp4">
                                ${I18n.t('vai.chat.browserNoVideoSupport')}
                            </video>
                        </div>
                        <div class="ai-video-card-footer">
                            <div class="ai-video-card-info">
                                <div class="ai-video-card-title">${escapeHtml(truncatedPrompt)}</div>
                                <div class="ai-video-card-meta">
                                    <span class="ai-video-card-spec">${video.aspect_ratio || '16:9'} | ${video.duration || 8}s</span>
                                </div>
                            </div>
                            <a href="${videoUrl}" download="vai-video-${Date.now()}.mp4" class="btn btn-sm btn-primary ai-video-card-download" title="${I18n.t('vai.chat.downloadVideo')}">
                                <i class="bi bi-download me-1"></i>${I18n.t('vai.common.download')}
                            </a>
                        </div>
                        <div class="ai-video-card-hint"><small class="text-muted"><i class="bi bi-lightbulb me-1"></i>${I18n.t('vai.chat.videoCardHint')}</small></div>
                    </div>`;
            }
        }
        
        // Build grounding sources HTML if present (web search results persisted in extra_fields)
        let groundingHtml = '';
        if (!isUser && msg.extra_fields && msg.extra_fields.grounding &&
            msg.extra_fields.grounding.sources && msg.extra_fields.grounding.sources.length > 0) {
            const sourcesLabel = I18n.t('vai.chat.webSearchSources') || 'Sources';
            groundingHtml = '<div class="ai-grounding-sources mt-2">' +
                '<small class="text-muted d-block mb-1"><i class="bi bi-globe2 me-1"></i>' + sourcesLabel + '</small>' +
                msg.extra_fields.grounding.sources.map(function(src) {
                    if (!src.url) return '';
                    const title = src.title || (function() { try { return new URL(src.url).hostname; } catch(e) { return src.url; } })();
                    const tooltip = src.text || src.title || src.url;
                    return '<a href="' + escapeHtml(src.url) + '" target="_blank" rel="noopener noreferrer" ' +
                        'class="badge bg-light text-dark text-decoration-none me-1 mb-1 d-inline-block" ' +
                        'style="font-size:0.75rem;font-weight:normal;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" ' +
                        'title="' + escapeHtml(tooltip) + '">' + escapeHtml(title) + '</a>';
                }).join('') +
                '</div>';
        }

        // Build navigate link card HTML if persisted in extra_fields
        let navigateLinkHtml = '';
        if (!isUser && msg.extra_fields && msg.extra_fields.navigate_url && typeof window._vaiHandleNavigate !== 'function') {
            const navUrl = msg.extra_fields.navigate_url;
            const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
            const vworkBase = isLocal ? window.location.origin : 'https://www.vworkai.com';
            const navFullUrl = vworkBase + (navUrl.startsWith('/') ? navUrl : '/' + navUrl);
            const navPageName = navUrl.replace(/^\//, '').replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
            const navLabel = escapeHtml(I18n.t('vai.chat.openInVWork') || 'Open in vWork');
            navigateLinkHtml = `
                <div class="ai-navigate-link-card mt-2">
                    <a href="${encodeURI(navFullUrl)}" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-outline-primary d-inline-flex align-items-center gap-1" style="text-decoration: none;">
                        <i class="bi bi-box-arrow-up-right"></i>
                        <span>${navLabel}: ${escapeHtml(navPageName)}</span>
                    </a>
                </div>`;
        }

        // Build payment link card HTML if persisted in extra_fields
        let paymentLinkHtml = '';
        if (!isUser && msg.extra_fields && msg.extra_fields.payment_link_url) {
            const payUrl = escapeHtml(msg.extra_fields.payment_link_url);
            const copyLabel = escapeHtml(I18n.t('vai.chat.copyPaymentLink') || 'Copy Link');
            const openLabel = escapeHtml(I18n.t('vai.chat.openPaymentLink') || 'Open');
            paymentLinkHtml = `
                <div class="ai-payment-link-card mt-2">
                    <div class="d-flex align-items-center gap-2 p-2" style="background: #f8f9fa; border: 1px solid #dee2e6; border-radius: 8px;">
                        <i class="bi bi-credit-card" style="font-size: 1.1rem; color: #0d6efd;"></i>
                        <span class="text-truncate flex-grow-1" style="font-size: 0.85rem; color: #495057; max-width: 200px;" title="${payUrl}">${payUrl}</span>
                        <button class="btn btn-sm btn-primary d-inline-flex align-items-center gap-1" onclick="copyPaymentLink(this, '${payUrl}')" style="white-space: nowrap;">
                            <i class="bi bi-clipboard"></i>
                            <span>${copyLabel}</span>
                        </button>
                        <a href="${payUrl}" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-outline-secondary d-inline-flex align-items-center gap-1" style="white-space: nowrap; text-decoration: none;">
                            <i class="bi bi-box-arrow-up-right"></i>
                            <span>${openLabel}</span>
                        </a>
                    </div>
                </div>`;
        }

        return `
            <div class="d-flex ${isUser ? 'justify-content-end' : 'justify-content-start'} mb-2 message-item" data-message-id="${msg.id}">
                ${!isUser ? `
                    <div class="avatar-circle me-2 flex-shrink-0" style="width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;">
                        <i class="bi bi-robot"></i>
                    </div>
                ` : ''}
                <div class="message-bubble ${isUser ? 'user' : 'ai'} position-relative" style="max-width: 70%;">
                    ${contentHtml}
                    ${generatedDocHtml}
                    ${generatedVideoHtml}
                    ${generatedImageHtml}
                    ${attachmentHtml}
                    ${groundingHtml}
                    ${navigateLinkHtml}
                    ${paymentLinkHtml}
                    <div class="d-flex justify-content-between align-items-center mt-1">
                        <div class="message-time" style="font-size: 0.75rem; opacity: 0.7;">${timeStr}</div>
                        <div class="d-flex align-items-center">
                            ${ /* TTS temporarily hidden — uncomment to re-enable:
                            !isUser ? `<button class="btn btn-sm btn-link p-0 me-2 ai-tts-btn" onclick="toggleAiTTS(this)" title="Read aloud" style="font-size: 0.75rem; text-decoration: none; opacity: 0.7; line-height: 1; color: #666;">
                                <i class="bi bi-volume-up"></i>
                            </button>` : ''
                            */ ''}
                            <button class="btn btn-sm btn-link p-0" onclick="deleteAIMessage('${msg.id}')" title="${I18n.t('vai.chat.deleteMessageTitle')}" style="font-size: 0.75rem; text-decoration: none; opacity: 0.7; line-height: 1; ${isUser ? 'color: white;' : 'color: #666;'}">
                                <i class="bi bi-trash"></i>
                            </button>
                        </div>
                    </div>
                </div>
                ${isUser ? `
                    <div class="avatar-circle ms-2 flex-shrink-0" style="width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;">
                        <i class="bi bi-person"></i>
                    </div>
                ` : ''}
            </div>
        `;
    }).join('');
}

// 發送 AI 消息
async function sendAIMessage(event) {
    if (event && event.preventDefault) event.preventDefault();
    
    if (!aiEndpoint) {
        App.showAlert(I18n.t('vai.chat.llmNotConfigured'), 'warning');
        return;
    }

    const aiMessageInput = document.getElementById('aiMessageInput');
    const content = aiMessageInput ? aiMessageInput.value.trim() : '';
    const hasFiles = aiPendingFiles.length > 0;
    if (!content && !hasFiles) return;

    // Upload files first (before creating conversation)
    let attachments = [];
    if (hasFiles) {
        attachments = await uploadAIFiles();
        if (attachments.length === 0 && !content) {
            // All uploads failed and no text
            return;
        }
    }

    // 如果沒有當前對話 或 是暫存對話，先建立真正的 DB 記錄
    if (!aiCurrentConversationId || aiCurrentConversationId === PENDING_CONV_ID) {
        try {
            // 如果使用者已為暫存對話重命名，使用該名稱；否則用訊息內容作為標題
            const pendingConv = aiConversations.find(c => c.id === PENDING_CONV_ID);
            const pendingTitle = pendingConv && pendingConv.title !== I18n.t('vai.chat.newConversation') ? pendingConv.title : null;
            const displayContent = content || (attachments.length > 0 ? `[${attachments.map(a => a.filename).join(', ')}]` : I18n.t('vai.chat.newConversation'));
            const autoTitle = pendingTitle || (displayContent.length > 20 ? displayContent.substring(0, 20) + '...' : displayContent);
            const conv = await App.apiRequest('/ai/conversations', {
                method: 'POST',
                body: JSON.stringify({ title: autoTitle })
            });
            // 移除暫存對話（如果有）
            aiConversations = aiConversations.filter(c => c.id !== PENDING_CONV_ID);
            aiConversations.unshift(conv);
            aiCurrentConversationId = conv.id;
            renderAiConversationList();
            updateConvHeader();
        } catch (err) {
            console.error('Failed to auto-create conversation:', err);
            App.showAlert(I18n.t('vai.chat.createFailed'), 'danger');
            return;
        }
    }

    const aiSendButton = document.getElementById('aiSendButton');
    if (aiSendButton) aiSendButton.disabled = true;
    if (aiMessageInput) aiMessageInput.disabled = true;

    // Clear pending files and preview
    aiPendingFiles = [];
    renderAIFilePreview();

    try {
        // Build user message content (include file names if attached)
        let userMessageContent = content;
        if (attachments.length > 0) {
            const fileNames = attachments.map(a => a.filename).join(', ');
            if (content) {
                userMessageContent = content + '\n\n[附件: ' + fileNames + ']';
            } else {
                userMessageContent = '[附件: ' + fileNames + ']';
            }
        }

        // 先保存用戶消息到數據庫
        const userExtraFields = { ai_chat: true, role: 'user' };
        if (attachments.length > 0) {
            userExtraFields.attachments = attachments.map(a => ({
                filename: a.filename,
                mime_type: a.mime_type,
                file_url: a.file_url || null
            }));
        }
        const userMessageData = {
            content: userMessageContent,
            subject: 'AI Chat',
            message_type: 'ai_chat',
            conversation_id: aiCurrentConversationId,
            extra_fields: userExtraFields
        };

        await App.apiRequest('/messages', {
            method: 'POST',
            body: JSON.stringify(userMessageData)
        });

        // 清空輸入框
        if (aiMessageInput) {
            aiMessageInput.value = '';
            aiMessageInput.style.height = 'auto';
            aiMessageInput.disabled = false;
        }
        updateAIMicSendToggle();

        // 重新載入消息以顯示用戶消息
        await loadAIChatHistory();

        // 如果對話標題是「新對話」，用第一條消息自動命名
        const conv = aiConversations.find(c => c.id === aiCurrentConversationId);
        if (conv && conv.title === I18n.t('vai.chat.newConversation')) {
            const displayContent = content || (attachments.length > 0 ? `[${attachments.map(a => a.filename).join(', ')}]` : I18n.t('vai.chat.newConversation'));
            const autoTitle = displayContent.length > 20 ? displayContent.substring(0, 20) + '...' : displayContent;
            try {
                await App.apiRequest(`/ai/conversations/${aiCurrentConversationId}`, {
                    method: 'PUT',
                    body: JSON.stringify({ title: autoTitle })
                });
                conv.title = autoTitle;
                renderAiConversationList();
                updateConvHeader();
            } catch (e) { /* 自動命名失敗不影響主流程 */ }
        }

        // 顯示思考中的視覺效果
        showAIThinking();
        
        // Check if this is a document generation request (keyword-based, kept for now)
        let llmResponse;
        let navigateUrl = null;
        let generatedImageUrls = null; // For image generation results (via backend tool calling)
        let generatedDocInfo = null;   // For document generation results (via backend tool calling)
        let generatedVideoInfo = null; // For video generation results (via backend tool calling)
        let groundingData = null;      // For web search grounding sources
        let paymentLinkUrl = null;     // For payment link results (via backend tool calling)
        
        // --- Unified LLM Chat Path (all tool calling handled by backend) ---
        try {
            const llmResult = await callLLMAPI(content || I18n.t('vai.chat.analyzeAttachments'), attachments);
            console.log('[vAi] LLM result keys:', Object.keys(llmResult));
            console.log('[vAi] LLM result.doc_info:', llmResult.doc_info);
            llmResponse = llmResult.content;
            navigateUrl = llmResult.navigate_url;
            // Check if backend returned image_urls from generate_image tool call
            if (llmResult.image_urls && llmResult.image_urls.length > 0) {
                generatedImageUrls = llmResult.image_urls;
                console.log('[vAi] Image generation via tool calling, count:', generatedImageUrls.length);
            }
            // Check if backend returned doc_info from generate_document tool call
            if (llmResult.doc_info) {
                generatedDocInfo = llmResult.doc_info;
                console.log('[vAi] Document generation via tool calling:', generatedDocInfo.doc_type, generatedDocInfo.title);
            }
            // Check if backend returned video_info from generate_video tool call
            if (llmResult.video_info) {
                generatedVideoInfo = llmResult.video_info;
                console.log('[vAi] Video generation via tool calling:', generatedVideoInfo.operation_id, generatedVideoInfo.status);
            }
            // Check if backend returned grounding metadata from web search
            if (llmResult.grounding) {
                groundingData = llmResult.grounding;
                console.log('[vAi] Web search grounding sources:', groundingData.sources ? groundingData.sources.length : 0);
            }
            // Check if backend returned payment_link_url from send_payment_link or get_payment_link tool call
            if (llmResult.payment_link_url) {
                paymentLinkUrl = llmResult.payment_link_url;
                console.log('[vAi] Payment link URL:', paymentLinkUrl);
            }
            // Allow empty content if navigate_url, image_urls, doc_info, video_info, or payment_link_url is present
            if ((!llmResponse || llmResponse.trim() === '') && !navigateUrl && !generatedImageUrls && !generatedDocInfo && !generatedVideoInfo && !paymentLinkUrl) {
                throw new Error(I18n.t('vai.chat.aiNoResponse'));
            }
            // Provide fallback message for navigate-only responses
            if ((!llmResponse || llmResponse.trim() === '') && navigateUrl) {
                llmResponse = I18n.t('vai.chat.navigating');
            }
            // Provide fallback message for image-only responses
            if ((!llmResponse || llmResponse.trim() === '') && generatedImageUrls) {
                llmResponse = I18n.t('vai.chat.imageGenerated');
            }
            // Provide fallback message for document-only responses
            if ((!llmResponse || llmResponse.trim() === '') && generatedDocInfo) {
                const typeLabels = { docx: 'Word', xlsx: 'Excel', pptx: 'PowerPoint', pdf: 'PDF' };
                const label = typeLabels[generatedDocInfo.doc_type] || generatedDocInfo.doc_type;
                if (generatedDocInfo.status === 'generating') {
                    llmResponse = I18n.t('vai.chat.docGenerating').replace('{label}', label).replace('{title}', generatedDocInfo.title || '');
                } else {
                    llmResponse = I18n.t('vai.chat.docGenerated').replace('{label}', label);
                }
            }
            // Provide fallback message for video-only responses
            if ((!llmResponse || llmResponse.trim() === '') && generatedVideoInfo) {
                if (generatedVideoInfo.status === 'generating') {
                    llmResponse = I18n.t('vai.chat.videoGenerating');
                } else {
                    llmResponse = I18n.t('vai.chat.videoGenerated');
                }
            }
        } catch (llmError) {
            console.error('LLM API error:', llmError);
            hideAIThinking();
            const errorMsg = llmError.message || (llmError.error && (llmError.error.message || llmError.error)) || I18n.t('vai.chat.aiResponseFailed');
            App.showAlert(I18n.t('vai.chat.aiResponseFailed') + ': ' + errorMsg, 'danger');
            if (aiMessageInput) aiMessageInput.disabled = false;
            if (aiSendButton) aiSendButton.disabled = false;
            return;
        }
        
        hideAIThinking();
        
        // 保存 AI 回復到數據庫
        const aiExtraFields = { ai_chat: true, role: 'assistant' };
        if (generatedImageUrls) {
            aiExtraFields.image_urls = generatedImageUrls;
        }
        if (generatedDocInfo) {
            aiExtraFields.doc_info = {
                id: generatedDocInfo.id,
                title: generatedDocInfo.title,
                doc_type: generatedDocInfo.doc_type,
                status: generatedDocInfo.status || 'completed',
                file_url: generatedDocInfo.file_url,
                file_size: generatedDocInfo.file_size
            };
        }
        if (generatedVideoInfo) {
            aiExtraFields.video_info = {
                operation_id: generatedVideoInfo.operation_id,
                status: generatedVideoInfo.status || 'generating',
                prompt: generatedVideoInfo.prompt,
                aspect_ratio: generatedVideoInfo.aspect_ratio,
                duration: generatedVideoInfo.duration
            };
        }
        if (groundingData && groundingData.sources && groundingData.sources.length > 0) {
            aiExtraFields.grounding = groundingData;
        }
        if (navigateUrl) {
            aiExtraFields.navigate_url = navigateUrl;
        }
        if (paymentLinkUrl) {
            aiExtraFields.payment_link_url = paymentLinkUrl;
        }
        const aiMessageData = {
            content: llmResponse,
            subject: 'AI Chat',
            message_type: 'ai_chat',
            conversation_id: aiCurrentConversationId,
            extra_fields: aiExtraFields
        };

        const savedAiMsg = await App.apiRequest('/messages', {
            method: 'POST',
            body: JSON.stringify(aiMessageData)
        });

        // 重新載入消息
        await loadAIChatHistory();

        // Render grounding sources on the last AI message (web search results)
        if (groundingData && groundingData.sources && groundingData.sources.length > 0) {
            const msgContainer = document.getElementById('aiChatMessages');
            if (msgContainer) {
                const allBubbles = msgContainer.querySelectorAll('.message-bubble.ai');
                if (allBubbles.length > 0) {
                    const lastBubble = allBubbles[allBubbles.length - 1];
                    renderGroundingSources(groundingData, lastBubble);
                }
            }
        }

        // If document is still generating, start polling for completion
        if (generatedDocInfo && generatedDocInfo.status === 'generating' && generatedDocInfo.id) {
            const savedMsgId = savedAiMsg && savedAiMsg.id ? savedAiMsg.id : null;
            startDocGenerationPolling(generatedDocInfo.id, savedMsgId);
        }
        
        // If video is still generating, start polling for completion
        if (generatedVideoInfo && generatedVideoInfo.status === 'generating' && generatedVideoInfo.operation_id) {
            const savedMsgId = savedAiMsg && savedAiMsg.id ? savedAiMsg.id : null;
            startVideoGenerationPolling(generatedVideoInfo.operation_id, savedMsgId);
        }
        
        // Handle navigate_url from AI response (goto page)
        if (navigateUrl) {
            if (typeof window._vaiHandleNavigate === 'function') {
                // Inside CMS: perform SPA navigation directly
                setTimeout(() => {
                    window._vaiHandleNavigate(navigateUrl, llmResponse);
                }, 800);
            } else {
                // Outside CMS (e.g. vai-chat standalone page): display a clickable vWork link
                // Build the full vWork URL for the target page
                const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
                const vworkBase = isLocal ? window.location.origin : 'https://www.vworkai.com';
                const fullUrl = vworkBase + (navigateUrl.startsWith('/') ? navigateUrl : '/' + navigateUrl);
                // Find the last AI message bubble and append a navigation link card
                setTimeout(() => {
                    const msgContainer = document.getElementById('aiChatMessages');
                    if (msgContainer) {
                        const allBubbles = msgContainer.querySelectorAll('.message-bubble.ai');
                        if (allBubbles.length > 0) {
                            const lastBubble = allBubbles[allBubbles.length - 1];
                            const pageName = navigateUrl.replace(/^\//, '').replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
                            const linkCard = document.createElement('div');
                            linkCard.className = 'ai-navigate-link-card mt-2';
                            const safeUrl = encodeURI(fullUrl);
                            const safePage = escapeHtml(pageName);
                            const label = escapeHtml(I18n.t('vai.chat.openInVWork') || 'Open in vWork');
                            linkCard.innerHTML = `
                                <a href="${safeUrl}" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-outline-primary d-inline-flex align-items-center gap-1" style="text-decoration: none;">
                                    <i class="bi bi-box-arrow-up-right"></i>
                                    <span>${label}: ${safePage}</span>
                                </a>
                            `;
                            // Insert before the date/delete row so it doesn't push the footer down
                            const footerRow = lastBubble.querySelector('.d-flex.justify-content-between.align-items-center.mt-1');
                            if (footerRow) {
                                lastBubble.insertBefore(linkCard, footerRow);
                            } else {
                                lastBubble.appendChild(linkCard);
                            }
                            // Scroll to bottom to show the link
                            if (msgContainer) msgContainer.scrollTop = msgContainer.scrollHeight;
                        }
                    }
                }, 300);
            }
        }
        
        // Handle payment_link_url from AI response (show copy card)
        if (paymentLinkUrl) {
            setTimeout(() => {
                const msgContainer = document.getElementById('aiChatMessages');
                if (msgContainer) {
                    const allBubbles = msgContainer.querySelectorAll('.message-bubble.ai');
                    if (allBubbles.length > 0) {
                        const lastBubble = allBubbles[allBubbles.length - 1];
                        const linkCard = buildPaymentLinkCard(paymentLinkUrl);
                        const footerRow = lastBubble.querySelector('.d-flex.justify-content-between.align-items-center.mt-1');
                        if (footerRow) {
                            lastBubble.insertBefore(linkCard, footerRow);
                        } else {
                            lastBubble.appendChild(linkCard);
                        }
                        if (msgContainer) msgContainer.scrollTop = msgContainer.scrollHeight;
                    }
                }
            }, 300);
        }
        
        // 刷新對話列表以更新 last_message 和排序
        try {
            const resp = await App.apiRequest('/ai/conversations');
            aiConversations = resp.data || [];
            renderAiConversationList();
        } catch (e) { /* 不影響主流程 */ }
        
        if (aiSendButton) aiSendButton.disabled = false;
        
        // 滾動到底部
        setTimeout(() => {
            const aiChatMessages = document.getElementById('aiChatMessages');
            if (aiChatMessages) aiChatMessages.scrollTop = aiChatMessages.scrollHeight;
        }, 100);
    } catch (error) {
        console.error('Failed to send AI message:', error);
        const errorMsg = error.message || (error.error && (error.error.message || error.error)) || I18n.t('vai.chat.sendFailed');
        App.showAlert(I18n.t('vai.chat.sendFailed') + ': ' + errorMsg, 'danger');
        if (aiMessageInput) aiMessageInput.disabled = false;
        if (aiSendButton) aiSendButton.disabled = false;
    }
}

// 調用 LLM API（通過後端代理）
async function callLLMAPI(userMessage, attachments) {
    if (!aiEndpoint) throw new Error(I18n.t('vai.chat.llmNotConfigured'));

    const messages = [];
    
    // 從歷史消息中提取對話上下文（最多保留最近 10 條）
    const recentHistory = aiChatHistory
        .filter(msg => msg.message_type === 'ai_chat')
        .slice(-10);
    
    recentHistory.forEach(msg => {
        let role = 'assistant';
        if (msg.extra_fields && msg.extra_fields.role) {
            role = msg.extra_fields.role;
        } else if (msg.from_user_id && aiCurrentUserId && msg.from_user_id === aiCurrentUserId) {
            role = 'user';
        }
        const content = msg.content || '';
        if (content.trim()) messages.push({ role, content });
    });
    
    messages.push({ role: 'user', content: userMessage });

    const hasAttachments = attachments && attachments.length > 0;

    // Detect page context: 'cms' when inside vWork CMS (has SPA router), 'vai-chat' otherwise
    const pageContext = (typeof window._vaiHandleNavigate === 'function') ? 'cms' : 'vai-chat';

    const requestBody = {
        model: aiModel,
        messages: messages,
        temperature: 0.7,
        max_tokens: hasAttachments ? 4000 : 1000,
        conversation_id: aiCurrentConversationId || null,
        web_search: aiWebSearchEnabled,
        page_context: pageContext
    };

    // Attach file data if any
    if (hasAttachments) {
        requestBody.attachments = attachments;
    }

    const response = await App.apiRequest('/llm/chat', {
        method: 'POST',
        body: JSON.stringify(requestBody)
    });

    console.log('[vAi] Raw LLM API response keys:', Object.keys(response));
    console.log('[vAi] Raw LLM API response.doc_info:', response.doc_info);

    // Extract navigate_url if backend returned one (from navigate_to_page function call)
    const navigateUrl = response.navigate_url || null;
    // Extract image_urls if backend returned them (from generate_image function call)
    const imageUrls = response.image_urls || null;
    // Extract doc_info if backend returned it (from generate_document function call)
    const docInfo = response.doc_info || null;
    // Extract video_info if backend returned it (from generate_video function call)
    const videoInfo = response.video_info || null;
    // Extract grounding metadata if web search was used
    const grounding = response.grounding || null;

    let textContent = null;
    if (response.choices && response.choices.length > 0) {
        const msg = response.choices[0].message;
        // Use nullish coalescing — empty string "" is a valid response
        textContent = msg?.content ?? msg?.text ?? null;
    }
    if (textContent === null && response.content != null) textContent = response.content;
    if (textContent === null && response.text != null) textContent = response.text;
    if (textContent === null && response.error) throw new Error(response.error.message || response.error || I18n.t('vai.chat.llmApiError'));
    // If content is still null but we have a navigate_url, use a placeholder message
    if (textContent === null && navigateUrl) {
        textContent = I18n.t('vai.chat.navigating');
    }
    // If content is still null but we have image_urls, use a placeholder
    if (textContent === null && imageUrls && imageUrls.length > 0) {
        textContent = I18n.t('vai.chat.imageGenerated');
    }
    // If content is still null but we have doc_info, use a placeholder
    if (textContent === null && docInfo) {
        const typeLabels = { docx: 'Word', xlsx: 'Excel', pptx: 'PowerPoint', pdf: 'PDF' };
        const label = typeLabels[docInfo.doc_type] || docInfo.doc_type;
        if (docInfo.status === 'generating') {
            textContent = I18n.t('vai.chat.docGenerating').replace('{label}', label).replace('{title}', docInfo.title || '');
        } else {
            textContent = I18n.t('vai.chat.docGenerated').replace('{label}', label);
        }
    }
    // If content is still null but we have video_info, use a placeholder
    if (textContent === null && videoInfo) {
        if (videoInfo.status === 'generating') {
            textContent = I18n.t('vai.chat.videoGenerating');
        } else if (videoInfo.status === 'done') {
            textContent = I18n.t('vai.chat.videoGenerated');
        } else {
            textContent = I18n.t('vai.chat.videoGeneratingStatus');
        }
    }
    if (textContent === null) {
        console.error('Unexpected LLM response format:', response);
        throw new Error(I18n.t('vai.chat.llmParseFailed'));
    }
    // Return object with content, optional navigate_url, image_urls, doc_info, video_info, and grounding
    return { content: textContent, navigate_url: navigateUrl, image_urls: imageUrls, doc_info: docInfo, video_info: videoInfo, grounding: grounding };
}

// Toggle Web Search (Google grounding) on/off
function toggleWebSearch() {
    aiWebSearchEnabled = !aiWebSearchEnabled;
    const btn = document.getElementById('aiWebSearchToggle');
    if (btn) {
        if (aiWebSearchEnabled) {
            btn.classList.add('active', 'btn-primary');
            btn.classList.remove('ai-upload-btn');
        } else {
            btn.classList.remove('active', 'btn-primary');
            btn.classList.add('ai-upload-btn');
        }
    }
}

function toggleAIVoiceInput() {
    if (aiVoiceListening || aiVoiceRecording) {
        stopAIVoiceInput();
        return;
    }

    // Always show recording bar with delete/send controls.
    // Also start STT in parallel for live transcription if supported.
    startAIAudioRecording();
    _startParallelSTT();
}

function stopAIVoiceInput() {
    // Stop parallel STT if running
    if (aiVoiceListening && aiSpeechRecognition) {
        aiSpeechRecognition.stop();
    }
    if (aiVoiceRecording && aiMediaRecorder) {
        // Normal stop (send) — not discard
        aiRecordingDiscarded = false;
        aiMediaRecorder.stop();
    }
}

// Discard recording — stop without sending
function discardAIAudioRecording() {
    // Stop parallel STT
    if (aiVoiceListening && aiSpeechRecognition) {
        try { aiSpeechRecognition.abort(); } catch(_) {}
        aiVoiceListening = false;
        aiSpeechRecognition = null;
    }
    if (aiVoiceRecording && aiMediaRecorder) {
        aiRecordingDiscarded = true;
        aiMediaRecorder.stop();
    }
    // Clear any transcribed text from parallel STT
    const inp = document.getElementById('aiMessageInput');
    if (inp) { inp.value = aiSpeechBaseText || ''; }
}

// Send recording — stop and auto-send
function sendAIAudioRecording() {
    // Stop parallel STT first
    if (aiVoiceListening && aiSpeechRecognition) {
        try { aiSpeechRecognition.abort(); } catch(_) {}
        aiVoiceListening = false;
        aiSpeechRecognition = null;
    }
    if (aiVoiceRecording && aiMediaRecorder) {
        aiRecordingDiscarded = false;
        aiMediaRecorder.stop();
    }
}

// Start STT in parallel with audio recording for live transcription
function _startParallelSTT() {
    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognition) return; // No STT support — audio-only mode

    const input = document.getElementById('aiMessageInput');
    if (!input) return;

    aiSpeechRecognition = new SpeechRecognition();
    aiSpeechRecognition.lang = getAIVoiceLang();
    aiSpeechRecognition.interimResults = true;
    aiSpeechRecognition.continuous = true;
    aiSpeechBaseText = input.value ? input.value.trim() : '';

    aiSpeechRecognition.onstart = function() {
        aiVoiceListening = true;
    };

    aiSpeechRecognition.onresult = function(event) {
        let transcript = '';
        for (let i = event.resultIndex; i < event.results.length; i++) {
            transcript += event.results[i][0].transcript;
        }
        input.value = (aiSpeechBaseText + ' ' + transcript).trim();
        input.style.height = 'auto';
        input.style.height = Math.min(input.scrollHeight, 120) + 'px';
    };

    aiSpeechRecognition.onerror = function() {
        // Silently ignore — audio recording is the primary mode
    };

    aiSpeechRecognition.onend = function() {
        aiVoiceListening = false;
        aiSpeechRecognition = null;
        // Do NOT auto-send here — recording bar controls handle that
    };

    try {
        aiSpeechRecognition.start();
    } catch (e) {
        // STT failed to start — no problem, audio recording continues
    }
}

function getAIVoiceLang() {
    const htmlLang = (document.documentElement.getAttribute('lang') || '').toLowerCase();
    if (htmlLang.startsWith('zh-cn')) return 'zh-CN';
    if (htmlLang.startsWith('zh')) return 'zh-TW';
    return 'en-US';
}

function startAIWebSpeech() {
    const input = document.getElementById('aiMessageInput');
    if (!input) return;
    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognition) {
        App.showAlert(I18n.t('vai.chat.voiceInputNotSupported'), 'warning');
        return;
    }

    aiSpeechRecognition = new SpeechRecognition();
    aiSpeechRecognition.lang = getAIVoiceLang();
    aiSpeechRecognition.interimResults = true;
    aiSpeechRecognition.continuous = true;
    aiSpeechBaseText = input.value ? input.value.trim() : '';

    aiSpeechRecognition.onstart = function() {
        aiVoiceListening = true;
        updateAIMicButtonState();
    };

    aiSpeechRecognition.onresult = function(event) {
        let transcript = '';
        for (let i = event.resultIndex; i < event.results.length; i++) {
            transcript += event.results[i][0].transcript;
        }
        input.value = (aiSpeechBaseText + ' ' + transcript).trim();
        input.style.height = 'auto';
        input.style.height = Math.min(input.scrollHeight, 120) + 'px';
    };

    aiSpeechRecognition.onerror = function(event) {
        if (event.error === 'not-allowed' || event.error === 'service-not-allowed') {
            App.showAlert(I18n.t('vai.chat.voiceInputPermissionDenied'), 'warning');
        } else if (event.error === 'no-speech') {
            App.showAlert(I18n.t('vai.chat.voiceInputNoSpeech'), 'warning');
        } else if (event.error !== 'aborted') {
            App.showAlert(I18n.t('vai.chat.voiceInputFailed').replace('{error}', event.error), 'warning');
        }
    };

    aiSpeechRecognition.onend = function() {
        aiVoiceListening = false;
        aiSpeechRecognition = null;
        updateAIMicButtonState();
        // Auto-send after speech recognition ends if there is text
        const inp = document.getElementById('aiMessageInput');
        if (inp && inp.value.trim()) {
            sendAIMessage();
        }
    };

    try {
        aiSpeechRecognition.start();
    } catch (e) {
        aiSpeechRecognition = null;
        aiVoiceListening = false;
        updateAIMicButtonState();
        App.showAlert(I18n.t('vai.chat.voiceInputFailed').replace('{error}', e.message || 'unknown'), 'warning');
    }
}

async function startAIAudioRecording() {
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        App.showAlert(I18n.t('vai.chat.voiceInputNotSupported'), 'warning');
        return;
    }

    try {
        aiRecordingStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        const preferredTypes = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4'];
        let selectedMime = '';
        for (const t of preferredTypes) {
            if (window.MediaRecorder && MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t)) {
                selectedMime = t;
                break;
            }
        }

        aiRecordedChunks = [];
        aiRecordingDiscarded = false;
        aiMediaRecorder = selectedMime ? new MediaRecorder(aiRecordingStream, { mimeType: selectedMime }) : new MediaRecorder(aiRecordingStream);
        aiMediaRecorder.ondataavailable = function(e) {
            if (e.data && e.data.size > 0) {
                aiRecordedChunks.push(e.data);
            }
        };
        aiMediaRecorder.onstart = function() {
            aiVoiceRecording = true;
            aiRecordingSeconds = 0;
            updateAIMicButtonState();
            showAIRecordingBar();
            // Start timer
            aiRecordingTimerInterval = setInterval(function() {
                aiRecordingSeconds++;
                updateAIRecordingTimer();
            }, 1000);
        };
        aiMediaRecorder.onstop = async function() {
            aiVoiceRecording = false;
            clearInterval(aiRecordingTimerInterval);
            aiRecordingTimerInterval = null;
            hideAIRecordingBar();
            updateAIMicButtonState();

            if (aiRecordingDiscarded) {
                // User chose to delete — discard audio
                cleanupAIAudioRecorder();
                return;
            }

            // Check if parallel STT transcribed text into the input
            const inp = document.getElementById('aiMessageInput');
            const sttText = inp ? inp.value.trim() : '';

            if (sttText && sttText !== (aiSpeechBaseText || '').trim()) {
                // STT produced text — send as text message (no audio attachment)
                cleanupAIAudioRecorder();
                sendAIMessage();
            } else {
                // No STT text — check if audio has meaningful content
                const blob = new Blob(aiRecordedChunks, { type: aiMediaRecorder.mimeType || 'audio/webm' });
                if (aiRecordingSeconds < 1 || blob.size < 1024) {
                    // Too short or too small — likely empty/silent
                    cleanupAIAudioRecorder();
                    App.showAlert('錄音沒有辨識到內容，請檢查錄音裝置或重錄', 'warning');
                } else {
                    // Audio has content (e.g. music, environment sound) — send as attachment
                    try {
                        await attachRecordedAudioToPendingFiles();
                        if (aiPendingFiles.length > 0) {
                            sendAIMessage();
                        }
                    } catch (e) {
                        App.showAlert(I18n.t('vai.chat.voiceInputFailed').replace('{error}', e.message || 'unknown'), 'warning');
                    }
                    cleanupAIAudioRecorder();
                }
            }
        };
        aiMediaRecorder.onerror = function(e) {
            aiVoiceRecording = false;
            clearInterval(aiRecordingTimerInterval);
            aiRecordingTimerInterval = null;
            hideAIRecordingBar();
            updateAIMicButtonState();
            App.showAlert(I18n.t('vai.chat.voiceInputFailed').replace('{error}', e.error?.message || 'recording_error'), 'warning');
            cleanupAIAudioRecorder();
        };

        aiMediaRecorder.start();
    } catch (e) {
        if (e && (e.name === 'NotAllowedError' || e.name === 'SecurityError')) {
            App.showAlert(I18n.t('vai.chat.voiceInputPermissionDenied'), 'warning');
        } else {
            App.showAlert(I18n.t('vai.chat.voiceInputFailed').replace('{error}', e.message || 'unknown'), 'warning');
        }
        cleanupAIAudioRecorder();
    }
}

// ─── WhatsApp-style recording bar ───────────────────────────────────

function showAIRecordingBar() {
    // Hide the normal input group, show recording bar
    const form = document.getElementById('aiChatForm');
    if (!form) return;

    // Create recording bar if not exists
    let bar = document.getElementById('aiRecordingBar');
    if (!bar) {
        bar = document.createElement('div');
        bar.id = 'aiRecordingBar';
        bar.className = 'ai-recording-bar';
        bar.innerHTML = `
            <button type="button" class="btn ai-recording-btn-delete" onclick="discardAIAudioRecording()" title="Delete recording">
                <i class="bi bi-trash3"></i>
            </button>
            <span class="ai-recording-indicator"></span>
            <span class="ai-recording-timer" id="aiRecordingTimer">00:00</span>
            <div class="ai-recording-wave">
                ${Array.from({length: 20}, () => '<span class="ai-recording-wave-bar"></span>').join('')}
            </div>
            <button type="button" class="btn ai-recording-btn-send" onclick="sendAIAudioRecording()" title="Send recording">
                <i class="bi bi-send-fill"></i>
            </button>
        `;
        form.parentNode.insertBefore(bar, form);
    }

    form.style.display = 'none';
    bar.style.display = 'flex';
}

function hideAIRecordingBar() {
    const bar = document.getElementById('aiRecordingBar');
    const form = document.getElementById('aiChatForm');
    if (bar) bar.style.display = 'none';
    if (form) form.style.display = '';
}

function updateAIRecordingTimer() {
    const el = document.getElementById('aiRecordingTimer');
    if (!el) return;
    const m = String(Math.floor(aiRecordingSeconds / 60)).padStart(2, '0');
    const s = String(aiRecordingSeconds % 60).padStart(2, '0');
    el.textContent = m + ':' + s;
}

async function attachRecordedAudioToPendingFiles() {
    if (!aiRecordedChunks || aiRecordedChunks.length === 0) {
        App.showAlert(I18n.t('vai.chat.voiceInputNoSpeech'), 'warning');
        return;
    }

    const blobType = (aiMediaRecorder && aiMediaRecorder.mimeType) ? aiMediaRecorder.mimeType : (aiRecordedChunks[0].type || 'audio/webm');
    const ext = blobType.includes('ogg') ? 'ogg' : (blobType.includes('mp4') ? 'm4a' : 'webm');
    const filename = 'voice-' + Date.now() + '.' + ext;
    const file = new File(aiRecordedChunks, filename, { type: blobType });
    if (file.size > AI_MAX_FILE_SIZE) {
        App.showAlert(I18n.t('vai.chat.fileTooLarge').replace('{name}', filename), 'warning');
        return;
    }

    aiPendingFiles.push({
        file: file,
        id: 'f_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6)
    });
    renderAIFilePreview();
    App.showAlert(I18n.t('vai.chat.voiceInputAttached'), 'success');
}

function cleanupAIAudioRecorder() {
    if (aiRecordingStream) {
        aiRecordingStream.getTracks().forEach(function(track) { track.stop(); });
    }
    aiMediaRecorder = null;
    aiRecordingStream = null;
    aiRecordedChunks = [];
}

function updateAIMicButtonState() {
    const btn = document.getElementById('aiMicButton');
    if (!btn) return;
    const icon = btn.querySelector('i');
    if (aiVoiceListening || aiVoiceRecording) {
        btn.classList.add('recording');
        if (icon) {
            icon.classList.remove('bi-mic');
            icon.classList.add('bi-mic-fill');
        }
        btn.title = I18n.t('vai.chat.voiceInputRecording');
    } else {
        btn.classList.remove('recording');
        if (icon) {
            icon.classList.remove('bi-mic-fill');
            icon.classList.add('bi-mic');
        }
        btn.title = I18n.t('vai.chat.voiceInput');
    }
    // Also update mic/send toggle state
    updateAIMicSendToggle();
}

// WhatsApp-style toggle: no text → show mic, hide send; has text → show send, hide mic
function updateAIMicSendToggle() {
    const input = document.getElementById('aiMessageInput');
    const micBtn = document.getElementById('aiMicButton');
    const sendBtn = document.getElementById('aiSendButton');
    if (!micBtn || !sendBtn) return;
    const hasText = input && input.value.trim().length > 0;
    // Also check if file attachments are pending
    const hasFiles = typeof aiPendingFiles !== 'undefined' && aiPendingFiles.length > 0;
    if (hasText || hasFiles || aiVoiceListening || aiVoiceRecording) {
        micBtn.style.display = 'none';
        sendBtn.style.display = '';
    } else {
        micBtn.style.display = '';
        sendBtn.style.display = 'none';
    }
}

// Render grounding sources (web search results) below an AI message
function renderGroundingSources(grounding, containerEl) {
    if (!grounding || !grounding.sources || grounding.sources.length === 0) return;
    const sourcesDiv = document.createElement('div');
    sourcesDiv.className = 'ai-grounding-sources mt-2';
    const label = document.createElement('small');
    label.className = 'text-muted d-block mb-1';
    label.innerHTML = '<i class="bi bi-globe2 me-1"></i>' + (I18n.t('vai.chat.webSearchSources') || 'Sources');
    sourcesDiv.appendChild(label);
    grounding.sources.forEach(function(src) {
        if (!src.url) return;
        const link = document.createElement('a');
        link.href = src.url;
        link.target = '_blank';
        link.rel = 'noopener noreferrer';
        link.className = 'badge bg-light text-dark text-decoration-none me-1 mb-1 d-inline-block';
        link.style.fontSize = '0.75rem';
        link.style.fontWeight = 'normal';
        link.style.maxWidth = '200px';
        link.style.overflow = 'hidden';
        link.style.textOverflow = 'ellipsis';
        link.style.whiteSpace = 'nowrap';
        link.title = src.text || src.title || src.url;
        link.textContent = src.title || new URL(src.url).hostname;
        sourcesDiv.appendChild(link);
    });
    containerEl.appendChild(sourcesDiv);
}

// 刪除 AI 消息
async function deleteAIMessage(messageId) {
    if (!messageId) return;
    if (!confirm(I18n.t('vai.chat.deleteMessageConfirm'))) return;
    
    try {
        await App.apiRequest(`/messages/${messageId}`, { method: 'DELETE' });
        await loadAIChatHistory();
        App.showAlert(I18n.t('vai.chat.messageDeleted'), 'success');
    } catch (error) {
        console.error('Failed to delete message:', error);
        App.showAlert(I18n.t('vai.common.deleteFailed') + ': ' + (error.message || error), 'danger');
    }
}

// ============================================
// 快捷動作（純聊天式，WhatsApp 兼容）
// ============================================

// 快捷發送：預填文字到輸入框並自動發送
async function vaiQuickSend(text) {
    const input = document.getElementById('aiMessageInput');
    if (!input) return;

    // 如果沒有當前對話，建立暫存對話（sendAIMessage 會處理真正的 DB 建立）
    if (!aiCurrentConversationId) {
        createNewAiConversation();
    }

    input.value = text;
    sendAIMessage(new Event('submit'));
}

// 渲染快捷動作建議（空對話時顯示在消息區底部）
function renderQuickActionSuggestions() {
    const container = document.getElementById('aiChatMessages');
    if (!container) return;

    // 如果已有消息，不顯示建議
    if (aiChatHistory.length > 0) return;

    const existing = container.querySelector('.vai-suggestions');
    if (existing) return;

    const div = document.createElement('div');
    div.className = 'vai-suggestions text-center mt-3';
    div.innerHTML = `
        <p class="text-muted mb-2" style="font-size: 0.8rem;">${I18n.t('vai.chat.suggestionsLabel')}</p>
        <div class="d-flex flex-wrap justify-content-center gap-2">
            <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickGenerateImage'))" style="font-size: 0.8rem;">
                <i class="bi bi-image me-1"></i>${I18n.t('vai.chat.quickGenerateImage')}
            </button>
            <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickGenerateVideo'))" style="font-size: 0.8rem;">
                <i class="bi bi-camera-video me-1"></i>${I18n.t('vai.chat.quickGenerateVideo')}
            </button>
            <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickGeneratePptx'))" style="font-size: 0.8rem;">
                <i class="bi bi-file-earmark-slides me-1"></i>${I18n.t('vai.chat.quickGeneratePptx')}
            </button>
            <button class="btn btn-outline-secondary btn-sm" onclick="vaiQuickSend(I18n.t('vai.chat.quickQueryOrders'))" style="font-size: 0.8rem;">
                <i class="bi bi-search me-1"></i>${I18n.t('vai.chat.quickQueryOrders')}
            </button>
        </div>
    `;
    container.appendChild(div);
}

// ============================================
// 文件上傳功能
// ============================================

const AI_ALLOWED_EXTENSIONS = ['xls', 'xlsx', 'doc', 'docx', 'ppt', 'pptx', 'pdf', 'txt', 'jpg', 'jpeg', 'png', 'webm', 'wav', 'mp3', 'm4a', 'ogg'];
const AI_MAX_FILE_SIZE = 20 * 1024 * 1024; // 20MB per file

// Get file icon class based on extension
function getFileIconClass(filename) {
    const ext = (filename.split('.').pop() || '').toLowerCase();
    if (['jpg', 'jpeg', 'png'].includes(ext)) return 'bi bi-file-image file-icon file-img';
    if (['webm', 'wav', 'mp3', 'm4a', 'ogg'].includes(ext)) return 'bi bi-file-earmark-music file-icon';
    if (ext === 'pdf') return 'bi bi-file-pdf file-icon file-pdf';
    if (['doc', 'docx'].includes(ext)) return 'bi bi-file-word file-icon file-doc';
    if (['xls', 'xlsx'].includes(ext)) return 'bi bi-file-excel file-icon file-xls';
    if (['ppt', 'pptx'].includes(ext)) return 'bi bi-file-ppt file-icon file-ppt';
    if (ext === 'txt') return 'bi bi-file-text file-icon file-txt';
    return 'bi bi-file-earmark file-icon';
}

// Handle file selection from input
function handleAIFileSelect(event) {
    const input = event.target || event.srcElement || document.getElementById('aiFileInput');
    const files = input ? input.files : null;
    if (!files || files.length === 0) return;

    for (const file of files) {
        // Validate extension
        const ext = (file.name.split('.').pop() || '').toLowerCase();
        if (!AI_ALLOWED_EXTENSIONS.includes(ext)) {
            App.showAlert(I18n.t('vai.chat.unsupportedFormat').replace('{ext}', ext), 'warning');
            continue;
        }
        // Validate size
        if (file.size > AI_MAX_FILE_SIZE) {
            App.showAlert(I18n.t('vai.chat.fileTooLarge').replace('{name}', file.name), 'warning');
            continue;
        }
        // Add to pending files
        aiPendingFiles.push({
            file: file,
            id: 'f_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6)
        });
    }

    // Reset file input so same file can be selected again
    event.target.value = '';

    // Close the attachment picker modal if open
    if (typeof _aiChatAttachPickerModal !== 'undefined' && _aiChatAttachPickerModal) {
        _aiChatAttachPickerModal.hide();
    }

    renderAIFilePreview();
}

// Remove a pending file
function removeAIPendingFile(fileId) {
    aiPendingFiles = aiPendingFiles.filter(f => f.id !== fileId);
    renderAIFilePreview();
}

// Render file preview area
function renderAIFilePreview() {
    const previewArea = document.getElementById('aiFilePreviewArea');
    if (!previewArea) return;

    if (aiPendingFiles.length === 0) {
        previewArea.style.display = 'none';
        previewArea.innerHTML = '';
        updateAIMicSendToggle();
        return;
    }

    previewArea.style.display = 'flex';
    previewArea.innerHTML = aiPendingFiles.map(f => {
        const fname = f.file ? f.file.name : 'image';
        const name = fname.length > 25 ? fname.substring(0, 22) + '...' : fname;
        const isUrlImage = f._isUrlImage && f._imageUrl;

        if (isUrlImage) {
            return `
                <div class="ai-file-preview-item vai-attach-thumb" title="${escapeHtml(fname)}">
                    <img src="${escapeHtml(f._imageUrl)}" alt="${escapeHtml(name)}" class="vai-attach-thumb-img">
                    <span class="remove-file" onclick="removeAIPendingFile('${f.id}')">&times;</span>
                </div>
            `;
        }

        const iconClass = getFileIconClass(fname);
        return `
            <div class="ai-file-preview-item" title="${escapeHtml(fname)}">
                <i class="${iconClass}"></i>
                <span class="file-name">${escapeHtml(name)}</span>
                <span class="remove-file" onclick="removeAIPendingFile('${f.id}')">&times;</span>
            </div>
        `;
    }).join('');
    updateAIMicSendToggle();
}

// Upload files to server and get attachment data for LLM
async function uploadAIFiles() {
    if (aiPendingFiles.length === 0) return [];

    const attachments = [];
    for (const pf of aiPendingFiles) {
        try {
            // URL-based image (from sketch/product picker) — no upload needed
            if (pf._isUrlImage && pf._imageUrl) {
                const fname = pf.file ? pf.file.name : 'image.jpg';
                const mtype = pf.file ? pf.file.type : 'image/jpeg';
                attachments.push({
                    filename: fname,
                    mime_type: mtype,
                    data: null,
                    file_url: pf._imageUrl
                });
                continue;
            }

            const formData = new FormData();
            formData.append('file', pf.file);

            const token = localStorage.getItem('auth_token');
            const headers = {};
            if (token && token !== 'temp_token') {
                headers['Authorization'] = 'Bearer ' + token;
            }
            const tenantSubdomain = localStorage.getItem('tenant_subdomain');
            if (tenantSubdomain) headers['X-Tenant-Subdomain'] = tenantSubdomain;

            const resp = await fetch('/api/v1/ai/upload-file', {
                method: 'POST',
                headers: headers,
                body: formData
            });

            if (!resp.ok) {
                const errData = await resp.json().catch(() => ({}));
                throw new Error(errData.error || `Upload failed (${resp.status})`);
            }

            const data = await resp.json();
            attachments.push({
                filename: pf.file.name,
                mime_type: data.mime_type,
                data: data.data, // base64
                file_url: data.file_url || null
            });
        } catch (err) {
            console.error('Failed to upload file:', pf.file.name, err);
            App.showAlert(I18n.t('vai.chat.uploadFailed').replace('{name}', pf.file.name).replace('{error}', err.message), 'danger');
        }
    }

    return attachments;
}

// ============================================
// AI Generated Image Viewer
// ============================================

// Open generated image in a lightbox overlay
function openAiGeneratedImage(src) {
    // Remove existing overlay if any
    const existing = document.getElementById('aiImageLightbox');
    if (existing) existing.remove();

    const overlay = document.createElement('div');
    overlay.id = 'aiImageLightbox';
    overlay.className = 'ai-image-lightbox';
    overlay.onclick = function(e) {
        if (e.target === overlay) overlay.remove();
    };
    overlay.innerHTML = `
        <div class="ai-image-lightbox-content">
            <img src="${src}" alt="AI Generated Image">
            <div class="ai-image-lightbox-actions">
                <a href="${src}" download="vai-image-${Date.now()}.png" class="btn btn-sm btn-light">
                    <i class="bi bi-download me-1"></i>${I18n.t('vai.common.download')}
                </a>
                <button class="btn btn-sm btn-light" onclick="this.closest('.ai-image-lightbox').remove()">
                    <i class="bi bi-x-lg me-1"></i>${I18n.t('vai.common.close')}
                </button>
            </div>
        </div>
    `;
    document.body.appendChild(overlay);
}

// ============================================
// AI Text-to-Speech (TTS) using Web Speech API
// ============================================

let _aiTtsCurrentBtn = null; // currently speaking button element

/**
 * Toggle TTS on an AI message bubble.
 * @param {HTMLElement} btn - The speaker button element
 */
function toggleAiTTS(btn) {
    const synth = window.speechSynthesis;
    if (!synth) return;

    // If this button is currently speaking, stop it
    if (_aiTtsCurrentBtn === btn && synth.speaking) {
        synth.cancel();
        _aiTtsResetBtn(btn);
        _aiTtsCurrentBtn = null;
        return;
    }

    // Stop any other ongoing speech
    if (synth.speaking) {
        synth.cancel();
        if (_aiTtsCurrentBtn) {
            _aiTtsResetBtn(_aiTtsCurrentBtn);
        }
    }

    // Find the message bubble that contains this button
    const bubble = btn.closest('.message-bubble');
    if (!bubble) return;

    // Extract text content, stripping HTML and code blocks
    let text = bubble.innerText || bubble.textContent || '';
    // Remove the time and button text at the bottom
    const timeEl = bubble.querySelector('.message-time');
    if (timeEl) text = text.replace(timeEl.textContent, '');
    text = text.trim();
    if (!text) return;

    // Auto-detect language: if mostly CJK characters, use Chinese
    const cjkCount = (text.match(/[\u4e00-\u9fff\u3400-\u4dbf]/g) || []).length;
    const lang = cjkCount > text.length * 0.15 ? 'zh-TW' : 'en-US';

    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = lang;
    utterance.rate = 1.0;
    utterance.pitch = 1.0;

    // Update button to show "stop" icon
    const icon = btn.querySelector('i');
    if (icon) {
        icon.className = 'bi bi-stop-circle';
    }
    btn.style.opacity = '1';
    btn.style.color = '#0d6efd'; // primary blue
    _aiTtsCurrentBtn = btn;

    utterance.onend = () => {
        _aiTtsResetBtn(btn);
        _aiTtsCurrentBtn = null;
    };
    utterance.onerror = () => {
        _aiTtsResetBtn(btn);
        _aiTtsCurrentBtn = null;
    };

    synth.speak(utterance);
}

/**
 * Reset a TTS button back to its default state.
 */
function _aiTtsResetBtn(btn) {
    if (!btn) return;
    const icon = btn.querySelector('i');
    if (icon) {
        icon.className = 'bi bi-volume-up';
    }
    btn.style.opacity = '0.7';
    btn.style.color = '#666';
}
