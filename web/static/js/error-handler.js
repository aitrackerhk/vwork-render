// 手機端 JavaScript 錯誤顯示工具
// 用於在手機上查看和調試 JavaScript 錯誤

(function() {
    'use strict';

    // 錯誤存儲
    const errorLog = [];
    const MAX_ERRORS = 50; // 最多保存 50 個錯誤

    // 創建錯誤顯示面板
    function createErrorPanel() {
        // 檢查是否已存在
        if (document.getElementById('js-error-panel')) {
            return;
        }

        const panel = document.createElement('div');
        panel.id = 'js-error-panel';
        panel.innerHTML = `
            <div class="error-panel-header">
                <span class="error-panel-title">
                    <i class="bi bi-exclamation-triangle-fill"></i>
                    JavaScript 錯誤 (<span id="error-count">0</span>)
                </span>
                <div class="error-panel-controls">
                    <button id="error-panel-clear" class="error-btn-clear" title="清除錯誤">
                        <i class="bi bi-trash"></i>
                    </button>
                    <button id="error-panel-toggle" class="error-btn-toggle" title="展開/收起">
                        <i class="bi bi-chevron-up"></i>
                    </button>
                    <button id="error-panel-close" class="error-btn-close" title="關閉面板">
                        <i class="bi bi-x"></i>
                    </button>
                </div>
            </div>
            <div class="error-panel-body" id="error-panel-body">
                <div class="error-list" id="error-list"></div>
            </div>
        `;

        // 添加樣式
        const style = document.createElement('style');
        style.textContent = `
            #js-error-panel {
                position: fixed;
                bottom: 0;
                left: 0;
                right: 0;
                background: #1e1e1e;
                color: #fff;
                font-family: 'Courier New', monospace;
                font-size: 12px;
                z-index: 99999;
                max-height: 50vh;
                display: flex;
                flex-direction: column;
                box-shadow: 0 -2px 10px rgba(0,0,0,0.3);
                border-top: 2px solid #dc3545;
                transition: transform 0.3s ease;
            }

            #js-error-panel.collapsed {
                transform: translateY(calc(100% - 40px));
            }

            #js-error-panel.hidden {
                display: none;
            }

            .error-panel-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                padding: 8px 12px;
                background: #2d2d2d;
                border-bottom: 1px solid #444;
                cursor: pointer;
                user-select: none;
            }

            .error-panel-title {
                display: flex;
                align-items: center;
                gap: 8px;
                font-weight: bold;
                color: #ff6b6b;
            }

            .error-panel-title i {
                color: #dc3545;
            }

            .error-panel-controls {
                display: flex;
                gap: 8px;
                align-items: center;
            }

            .error-btn-clear,
            .error-btn-toggle,
            .error-btn-close {
                background: transparent;
                border: none;
                color: #fff;
                cursor: pointer;
                padding: 4px 8px;
                border-radius: 4px;
                font-size: 14px;
                transition: background 0.2s;
            }

            .error-btn-clear:hover,
            .error-btn-toggle:hover,
            .error-btn-close:hover {
                background: #444;
            }

            .error-panel-body {
                overflow-y: auto;
                flex: 1;
                max-height: calc(50vh - 40px);
            }

            .error-list {
                padding: 8px;
            }

            .error-item {
                background: #2d2d2d;
                border-left: 3px solid #dc3545;
                padding: 10px;
                margin-bottom: 8px;
                border-radius: 4px;
                word-break: break-word;
            }

            .error-item-header {
                display: flex;
                justify-content: space-between;
                align-items: flex-start;
                margin-bottom: 6px;
            }

            .error-message {
                color: #ff6b6b;
                font-weight: bold;
                flex: 1;
                margin-right: 8px;
            }

            .error-time {
                color: #888;
                font-size: 10px;
                white-space: nowrap;
            }

            .error-source {
                color: #4ec9b0;
                font-size: 11px;
                margin-top: 4px;
            }

            .error-stack {
                color: #ce9178;
                font-size: 11px;
                margin-top: 6px;
                padding: 6px;
                background: #1e1e1e;
                border-radius: 3px;
                white-space: pre-wrap;
                max-height: 150px;
                overflow-y: auto;
            }

            .error-stack-toggle {
                color: #888;
                cursor: pointer;
                font-size: 10px;
                margin-top: 4px;
                text-decoration: underline;
            }

            .error-stack-toggle:hover {
                color: #fff;
            }

            /* 手機適配 */
            @media (max-width: 768px) {
                #js-error-panel {
                    font-size: 11px;
                }

                .error-panel-header {
                    padding: 6px 10px;
                }

                .error-item {
                    padding: 8px;
                }

                .error-message {
                    font-size: 11px;
                }

                .error-stack {
                    font-size: 10px;
                    max-height: 100px;
                }
            }

            /* 桌面端可以調整位置 */
            @media (min-width: 769px) {
                #js-error-panel {
                    max-width: 600px;
                    left: auto;
                    right: 0;
                    border-radius: 8px 8px 0 0;
                }
            }
        `;

        document.head.appendChild(style);
        document.body.appendChild(panel);

        // 綁定事件
        setupErrorPanelEvents();
    }

    // 設置錯誤面板事件
    function setupErrorPanelEvents() {
        const panel = document.getElementById('js-error-panel');
        const toggleBtn = document.getElementById('error-panel-toggle');
        const closeBtn = document.getElementById('error-panel-close');
        const clearBtn = document.getElementById('error-panel-clear');
        const header = document.querySelector('.error-panel-header');

        // 切換展開/收起
        if (toggleBtn) {
            toggleBtn.addEventListener('click', function(e) {
                e.stopPropagation();
                panel.classList.toggle('collapsed');
                const icon = toggleBtn.querySelector('i');
                if (icon) {
                    icon.className = panel.classList.contains('collapsed') 
                        ? 'bi bi-chevron-down' 
                        : 'bi bi-chevron-up';
                }
            });
        }

        // 點擊標題也可以切換
        if (header) {
            header.addEventListener('click', function(e) {
                if (e.target.closest('.error-panel-controls')) return;
                panel.classList.toggle('collapsed');
                const icon = toggleBtn?.querySelector('i');
                if (icon) {
                    icon.className = panel.classList.contains('collapsed') 
                        ? 'bi bi-chevron-down' 
                        : 'bi bi-chevron-up';
                }
            });
        }

        // 關閉面板
        if (closeBtn) {
            closeBtn.addEventListener('click', function(e) {
                e.stopPropagation();
                panel.classList.add('hidden');
                // 可以選擇保存到 localStorage，下次訪問時不顯示
                localStorage.setItem('js-error-panel-hidden', 'true');
            });
        }

        // 清除錯誤
        if (clearBtn) {
            clearBtn.addEventListener('click', function(e) {
                e.stopPropagation();
                errorLog.length = 0;
                updateErrorDisplay();
            });
        }

        // 檢查是否之前被隱藏
        if (localStorage.getItem('js-error-panel-hidden') === 'true') {
            panel.classList.add('hidden');
        }
    }

    // 更新錯誤顯示
    function updateErrorDisplay() {
        const errorList = document.getElementById('error-list');
        const errorCount = document.getElementById('error-count');
        
        if (!errorList) return;

        // 更新計數
        if (errorCount) {
            errorCount.textContent = errorLog.length;
        }

        // 清空列表
        errorList.innerHTML = '';

        // 顯示錯誤（最新的在前）
        const errorsToShow = [...errorLog].reverse();
        errorsToShow.forEach((error, index) => {
            const errorItem = document.createElement('div');
            errorItem.className = 'error-item';
            
            const time = new Date(error.timestamp).toLocaleTimeString('zh-TW');
            const source = error.source || '未知來源';
            
            let stackHtml = '';
            if (error.stack) {
                const stackLines = error.stack.split('\n').slice(0, 10); // 只顯示前 10 行
                stackHtml = `
                    <div class="error-stack" style="display: none;" id="stack-${index}">
                        ${stackLines.map(line => escapeHtml(line)).join('\n')}
                    </div>
                    <div class="error-stack-toggle" onclick="this.previousElementSibling.style.display = this.previousElementSibling.style.display === 'none' ? 'block' : 'none'; this.textContent = this.previousElementSibling.style.display === 'none' ? '顯示堆疊' : '隱藏堆疊';">
                        顯示堆疊
                    </div>
                `;
            }

            errorItem.innerHTML = `
                <div class="error-item-header">
                    <div class="error-message">${escapeHtml(error.message)}</div>
                    <div class="error-time">${time}</div>
                </div>
                <div class="error-source">${escapeHtml(source)}</div>
                ${stackHtml}
            `;

            errorList.appendChild(errorItem);
        });

        // 如果沒有錯誤，顯示提示
        if (errorLog.length === 0) {
            errorList.innerHTML = '<div style="text-align: center; color: #888; padding: 20px;">暫無錯誤</div>';
        }
    }

    // HTML 轉義
    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // 提取錯誤訊息（處理各種類型的錯誤）
    function extractErrorMessage(error) {
        if (!error) {
            return '未知錯誤';
        }

        // 如果是 Error 實例，直接使用 message
        if (error instanceof Error) {
            return error.message || error.toString();
        }

        // 如果是字符串，直接返回
        if (typeof error === 'string') {
            return error;
        }

        // 如果是數字或布林值
        if (typeof error !== 'object') {
            return String(error);
        }

        // 如果是 null（typeof null === 'object'）
        if (error === null) {
            return 'null';
        }

        // 如果是對象，嘗試提取訊息
        if (typeof error === 'object') {
            // 優先使用 message 屬性
            if (error.message && typeof error.message === 'string' && error.message.trim()) {
                return error.message;
            }

            // 嘗試使用 error 屬性（某些庫會這樣做）
            if (error.error) {
                if (typeof error.error === 'string') {
                    return error.error;
                }
                // 如果 error 是對象，遞歸提取
                if (typeof error.error === 'object') {
                    const nestedMsg = extractErrorMessage(error.error);
                    if (nestedMsg && nestedMsg !== '[object Object]') {
                        return `錯誤: ${nestedMsg}`;
                    }
                }
            }

            // 嘗試使用 reason 屬性（Promise rejection 可能使用）
            if (error.reason) {
                if (typeof error.reason === 'string') {
                    return error.reason;
                }
                // 如果 reason 是對象，遞歸提取
                if (typeof error.reason === 'object') {
                    const nestedMsg = extractErrorMessage(error.reason);
                    if (nestedMsg && nestedMsg !== '[object Object]') {
                        return `原因: ${nestedMsg}`;
                    }
                }
            }

            // 嘗試使用其他常見的錯誤屬性
            const commonErrorProps = ['msg', 'description', 'detail', 'details', 'text', 'statusText'];
            for (const prop of commonErrorProps) {
                if (error[prop] && typeof error[prop] === 'string' && error[prop].trim()) {
                    return error[prop];
                }
            }

            // 如果有 toString 方法且不是默認的
            if (error.toString && error.toString !== Object.prototype.toString) {
                const str = error.toString();
                if (str && str !== '[object Object]' && str.trim()) {
                    return str;
                }
            }

            // 收集對象的所有屬性值（排除函數和 undefined）
            const props = [];
            const keys = Object.keys(error);
            
            for (const key of keys) {
                const value = error[key];
                // 跳過內部屬性（stack, name 等會在後面單獨處理）
                if (key === 'stack' || key === 'name' || key === 'message') {
                    continue;
                }
                if (value !== undefined && value !== null) {
                    if (typeof value === 'string' && value.trim()) {
                        props.push(`${key}: ${value}`);
                    } else if (typeof value !== 'function' && typeof value !== 'object') {
                        props.push(`${key}: ${value}`);
                    }
                }
            }

            // 構建友好的錯誤訊息
            const parts = [];
            
            // 如果有 name，顯示類型
            if (error.name && typeof error.name === 'string') {
                parts.push(error.name);
            }
            
            // 如果有其他屬性，顯示它們
            if (props.length > 0) {
                parts.push(props.join(', '));
            }
            
            // 如果只有 name 和 stack（且 stack 為 null），顯示簡化訊息
            if (parts.length === 0 && keys.length <= 2 && keys.includes('name') && (keys.includes('stack') || keys.length === 1)) {
                return error.name || '未知錯誤';
            }
            
            // 如果有部分信息，組合顯示
            if (parts.length > 0) {
                return parts.join(' - ');
            }

            // 最後嘗試 JSON.stringify（但限制長度）
            try {
                const jsonStr = JSON.stringify(error, null, 2);
                // 如果 JSON 只有很少內容（如只有 {"stack": null, "name": "Error"}），顯示簡化訊息
                if (jsonStr.length < 100 && keys.length <= 3) {
                    const meaningfulKeys = keys.filter(k => {
                        const v = error[k];
                        return v !== null && v !== undefined && (typeof v !== 'string' || v.trim());
                    });
                    if (meaningfulKeys.length === 0) {
                        return error.name || '未知錯誤';
                    }
                    return `錯誤: ${meaningfulKeys.map(k => `${k}=${error[k]}`).join(', ')}`;
                }
                // 如果 JSON 太長，截斷
                if (jsonStr.length > 500) {
                    return jsonStr.substring(0, 500) + '... (已截斷)';
                }
                return jsonStr;
            } catch (e) {
                // JSON.stringify 失敗（可能是循環引用）
                if (keys.length > 0) {
                    return `[對象錯誤: ${keys.join(', ')}]`;
                }
                return '[空對象錯誤]';
            }
        }

        // 其他情況
        return String(error);
    }

    // 記錄錯誤
    function logError(error, source) {
        const errorMessage = extractErrorMessage(error);
        
        // 改進 source 信息的提取
        let errorSource = source;
        if (!errorSource || errorSource === 'undefined:undefined:undefined') {
            if (error && error.filename) {
                errorSource = `${error.filename}:${error.lineno || '?'}:${error.colno || '?'}`;
            } else if (error && error.url) {
                errorSource = error.url;
            } else if (error && typeof error === 'object') {
                // 嘗試從錯誤對象中提取來源信息
                const sourceProps = ['source', 'file', 'filepath', 'url', 'endpoint'];
                for (const prop of sourceProps) {
                    if (error[prop]) {
                        errorSource = String(error[prop]);
                        break;
                    }
                }
            }
            if (!errorSource || errorSource === 'undefined:undefined:undefined') {
                errorSource = '未知來源';
            }
        }
        
        const errorInfo = {
            message: errorMessage,
            source: errorSource,
            stack: (error && error.stack) ? error.stack : null,
            timestamp: new Date().toISOString(),
            type: (error && error.name) ? error.name : 'Error'
        };

        // 添加到錯誤日誌
        errorLog.push(errorInfo);

        // 限制錯誤數量
        if (errorLog.length > MAX_ERRORS) {
            errorLog.shift();
        }

        // 更新顯示
        updateErrorDisplay();

        // 同時輸出到控制台（方便開發時查看）
        console.error('❌ JavaScript Error:', errorInfo);

        // 可選：發送到服務器
        // sendErrorToServer(errorInfo);
    }

    // 發送錯誤到服務器（可選功能）
    function sendErrorToServer(errorInfo) {
        // 只在生產環境或需要時啟用
        if (typeof App !== 'undefined' && App.apiRequest) {
            try {
                App.apiRequest('/api/v1/errors', {
                    method: 'POST',
                    body: JSON.stringify({
                        type: 'javascript_error',
                        error: errorInfo
                    })
                }).catch(err => {
                    // 靜默失敗，不影響用戶體驗
                    console.warn('Failed to send error to server:', err);
                });
            } catch (e) {
                // 忽略
            }
        }
    }

    // 全局錯誤處理器
    window.addEventListener('error', function(event) {
        // 構建來源信息，處理 undefined 值
        let source = '未知來源';
        if (event.filename || event.lineno || event.colno) {
            const parts = [];
            if (event.filename) parts.push(event.filename);
            if (event.lineno !== undefined) parts.push(event.lineno);
            if (event.colno !== undefined) parts.push(event.colno);
            if (parts.length > 0) {
                source = parts.join(':');
            }
        }
        
        // 從 event.error 中提取更多信息（如果 event.message 為空）
        let errorMessage = event.message;
        let errorName = 'Error';
        let errorStack = null;
        
        if (event.error) {
            if (event.error instanceof Error) {
                errorMessage = event.error.message || errorMessage || '未知錯誤';
                errorName = event.error.name || errorName;
                errorStack = event.error.stack || null;
            } else if (typeof event.error === 'object') {
                // 如果 error 是對象但不是 Error 實例
                errorMessage = event.error.message || extractErrorMessage(event.error) || errorMessage || '未知錯誤';
                errorName = event.error.name || errorName;
                errorStack = event.error.stack || null;
            } else {
                errorMessage = extractErrorMessage(event.error) || errorMessage || '未知錯誤';
            }
        }
        
        // 如果仍然沒有消息，嘗試從其他來源獲取
        if (!errorMessage || errorMessage === '未知錯誤') {
            // 嘗試從 target 中獲取信息
            if (event.target && event.target.tagName) {
                const tagName = event.target.tagName.toLowerCase();
                let targetInfo = '';
                
                // 根據不同元素類型提取信息
                if (tagName === 'img') {
                    // 對於圖片，優先從 HTML 屬性獲取原始值（避免瀏覽器解析後的 URL）
                    let src = event.target.getAttribute('src') || '';
                    const currentHref = window.location.href;
                    
                    // 如果原始屬性為空，檢查 src 屬性（可能是動態設置的）
                    if (!src) {
                        src = event.target.src || '';
                        // 如果 src 是當前頁面 URL，說明原始 src 可能是空的或無效的
                        if (src === currentHref) {
                            src = '';
                        }
                    }
                    
                    // 如果是相對路徑，轉換為絕對路徑
                    if (src && !src.startsWith('http') && !src.startsWith('//') && !src.startsWith('data:') && !src.startsWith('blob:')) {
                        try {
                            src = new URL(src, window.location.origin).href;
                        } catch (e) {
                            // 如果轉換失敗，使用原始值
                        }
                    }
                    
                    const alt = event.target.alt || event.target.getAttribute('alt') || '';
                    
                    // 檢查圖片是否已經有錯誤處理器（避免重複報告已處理的錯誤）
                    // 方法1: 檢查是否有 onerror 屬性（HTML 字符串中設置的）
                    const hasOnErrorAttr = event.target.getAttribute('onerror') !== null;
                    // 方法2: 檢查是否有 onerror 函數（通過 JavaScript 設置的）
                    const hasOnErrorFunc = event.target.onerror !== null && event.target.onerror !== undefined;
                    // 方法3: 檢查是否已經被標記為已處理（通過 data 屬性）
                    const isErrorHandled = event.target.getAttribute('data-error-handled') === 'true';
                    // 方法4: 檢查圖片是否已經被隱藏（說明 onerror 已經處理了）
                    const isHidden = event.target.style.display === 'none' || 
                                   window.getComputedStyle(event.target).display === 'none';
                    
                    // 如果圖片已經有錯誤處理器或已被處理，靜默處理，不記錄為錯誤
                    if (hasOnErrorAttr || hasOnErrorFunc || isErrorHandled || isHidden) {
                        // 靜默處理，不記錄為錯誤（因為已經有 onerror 處理）
                        return; // 直接返回，不記錄錯誤
                    }
                    
                    // 如果 src 仍然是當前頁面 URL 或空，說明可能是無效的 src
                    if (!src || src === currentHref) {
                        // 檢查圖片是否被隱藏（說明是預期行為，不應記錄為錯誤）
                        const isHidden = event.target.style.display === 'none' || 
                                       window.getComputedStyle(event.target).display === 'none';
                        const parentHidden = event.target.parentElement && 
                                           (event.target.parentElement.style.display === 'none' ||
                                            window.getComputedStyle(event.target.parentElement).display === 'none');
                        
                        // 如果圖片或父元素被隱藏，且 src 無效，這是預期行為，不記錄為錯誤
                        if ((isHidden || parentHidden) && (!src || src === currentHref)) {
                            return; // 靜默處理，不記錄為錯誤
                        }
                        
                        // 嘗試從其他屬性獲取
                        const dataSrc = event.target.getAttribute('data-src') || event.target.dataset?.src || '';
                        if (dataSrc) {
                            src = dataSrc;
                            if (!src.startsWith('http') && !src.startsWith('//') && !src.startsWith('data:') && !src.startsWith('blob:')) {
                                try {
                                    src = new URL(src, window.location.origin).href;
                                } catch (e) {
                                    // 忽略
                                }
                            }
                        }
                    }
                    
                    targetInfo = `圖片加載失敗`;
                    if (src && src !== currentHref) {
                        // 只顯示文件名，避免太長
                        try {
                            const url = new URL(src);
                            const fileName = url.pathname.split('/').pop() || url.pathname || src;
                            targetInfo += `: ${fileName}`;
                        } catch (e) {
                            // 如果不是有效 URL，直接使用
                            const fileName = src.split('/').pop() || src;
                            targetInfo += `: ${fileName}`;
                        }
                        // 更新 source 為圖片路徑（使用完整路徑）
                        if (source === '未知來源') {
                            source = src.length > 80 ? src.substring(0, 80) + '...' : src;
                        }
                    } else if (!src || src === currentHref) {
                        // src 無效或為空，但只有在圖片可見時才記錄為錯誤
                        const isVisible = event.target.style.display !== 'none' && 
                                        window.getComputedStyle(event.target).display !== 'none';
                        const parentVisible = !event.target.parentElement || 
                                            (event.target.parentElement.style.display !== 'none' &&
                                             window.getComputedStyle(event.target.parentElement).display !== 'none');
                        
                        // 如果圖片不可見，不記錄為錯誤（這是預期行為）
                        if (!isVisible || !parentVisible) {
                            return; // 靜默處理，不記錄為錯誤
                        }
                        
                        targetInfo += ` (src 屬性無效或為空)`;
                        // 嘗試從其他來源獲取信息
                        const imgId = event.target.id || '';
                        const imgClass = event.target.className || '';
                        if (imgId) {
                            targetInfo += ` [ID: ${imgId}]`;
                        } else if (imgClass) {
                            const firstClass = imgClass.split(' ')[0];
                            targetInfo += ` [Class: ${firstClass}]`;
                        }
                        // 如果 source 仍然是未知，使用元素信息
                        if (source === '未知來源') {
                            source = `IMG 元素${imgId ? ' #' + imgId : ''}${imgClass ? ' .' + imgClass.split(' ')[0] : ''}`;
                        }
                    }
                    if (alt) {
                        targetInfo += ` (${alt})`;
                    }
                } else if (tagName === 'script') {
                    let src = event.target.src || '';
                    if (!src || src === window.location.href) {
                        src = event.target.getAttribute('src') || '';
                        if (src && !src.startsWith('http') && !src.startsWith('//') && !src.startsWith('data:')) {
                            try {
                                src = new URL(src, window.location.origin).href;
                            } catch (e) {
                                // 忽略
                            }
                        }
                    }
                    targetInfo = `腳本加載失敗`;
                    if (src) {
                        try {
                            const url = new URL(src);
                            const fileName = url.pathname.split('/').pop() || url.pathname || src;
                            targetInfo += `: ${fileName}`;
                        } catch (e) {
                            const fileName = src.split('/').pop() || src;
                            targetInfo += `: ${fileName}`;
                        }
                        if (source === '未知來源') {
                            source = src.length > 80 ? src.substring(0, 80) + '...' : src;
                        }
                    }
                } else if (tagName === 'link') {
                    let href = event.target.href || '';
                    if (!href || href === window.location.href) {
                        href = event.target.getAttribute('href') || '';
                        if (href && !href.startsWith('http') && !href.startsWith('//') && !href.startsWith('data:')) {
                            try {
                                href = new URL(href, window.location.origin).href;
                            } catch (e) {
                                // 忽略
                            }
                        }
                    }
                    targetInfo = `資源加載失敗`;
                    if (href) {
                        try {
                            const url = new URL(href);
                            const fileName = url.pathname.split('/').pop() || url.pathname || href;
                            targetInfo += `: ${fileName}`;
                        } catch (e) {
                            const fileName = href.split('/').pop() || href;
                            targetInfo += `: ${fileName}`;
                        }
                        if (source === '未知來源') {
                            source = href.length > 80 ? href.substring(0, 80) + '...' : href;
                        }
                    }
                } else {
                    targetInfo = `${tagName.toUpperCase()} 元素發生錯誤`;
                }
                
                errorMessage = targetInfo;
            }
        }
        
        // 添加調試信息到控制台（幫助開發者識別錯誤來源）
        if (console && console.debug) {
            let targetSrc = '';
            let targetSrcAttr = '';
            let targetInfo = {};
            
            if (event.target) {
                const tagName = event.target.tagName ? event.target.tagName.toLowerCase() : '';
                if (tagName === 'img') {
                    // 優先從屬性獲取原始值
                    targetSrcAttr = event.target.getAttribute('src') || '';
                    targetSrc = event.target.src || '';
                    // 如果 src 是當前頁面 URL，說明原始 src 可能是空的
                    if (targetSrc === window.location.href && !targetSrcAttr) {
                        targetSrc = '(空或無效的 src)';
                    }
                    // 收集圖片元素的詳細信息
                    targetInfo = {
                        id: event.target.id || '',
                        className: event.target.className || '',
                        alt: event.target.alt || event.target.getAttribute('alt') || '',
                        dataSrc: event.target.getAttribute('data-src') || event.target.dataset?.src || '',
                        naturalWidth: event.target.naturalWidth || 0,
                        naturalHeight: event.target.naturalHeight || 0,
                        complete: event.target.complete || false
                    };
                } else if (tagName === 'script') {
                    targetSrcAttr = event.target.getAttribute('src') || '';
                    targetSrc = event.target.src || '';
                    targetInfo = {
                        id: event.target.id || '',
                        type: event.target.type || '',
                        async: event.target.async || false,
                        defer: event.target.defer || false
                    };
                } else if (tagName === 'link') {
                    targetSrcAttr = event.target.getAttribute('href') || '';
                    targetSrc = event.target.href || '';
                    targetInfo = {
                        id: event.target.id || '',
                        rel: event.target.rel || '',
                        type: event.target.type || ''
                    };
                } else {
                    targetInfo = {
                        id: event.target.id || '',
                        className: event.target.className || '',
                        tagName: event.target.tagName || ''
                    };
                }
            }
            
            console.debug('全局錯誤詳情:', {
                // 標準錯誤事件屬性（資源加載錯誤時可能為 undefined）
                message: event.message,
                filename: event.filename,
                lineno: event.lineno,
                colno: event.colno,
                error: event.error,
                // 目標元素信息
                target: event.target,
                targetTagName: event.target ? event.target.tagName : '',
                targetSrc: targetSrc,
                targetSrcAttr: targetSrcAttr, // 原始 HTML 屬性值
                targetInfo: targetInfo, // 元素的詳細信息
                // 環境信息
                currentHref: window.location.href,
                // 錯誤類型判斷
                isResourceError: !event.error && !event.message && event.target !== null,
                isScriptError: !!event.error || !!event.message
            });
        }
        
        logError({
            message: errorMessage || '未知錯誤',
            filename: event.filename,
            lineno: event.lineno,
            colno: event.colno,
            stack: errorStack,
            name: errorName
        }, source);
    }, true);

    // Promise 錯誤處理器
    window.addEventListener('unhandledrejection', function(event) {
        const error = event.reason;
        
        // 構建錯誤對象，確保包含所有有用信息
        let errorObj;
        let source = 'Promise Rejection';
        
        // 處理 null/undefined
        if (error === null || error === undefined) {
            errorObj = {
                message: `Promise 被拒絕 (${error === null ? 'null' : 'undefined'})`,
                name: 'UnhandledRejection',
                originalReason: error
            };
        } else if (error instanceof Error) {
            // 如果是 Error 實例，直接使用
            errorObj = {
                message: error.message || 'Promise 被拒絕',
                stack: error.stack || null,
                name: error.name || 'UnhandledRejection',
                originalError: error
            };
            
            // 嘗試從 stack 中提取文件名
            if (error.stack) {
                const stackMatch = error.stack.match(/at\s+(.+?):(\d+):(\d+)/);
                if (stackMatch) {
                    source = `Promise Rejection (${stackMatch[1]}:${stackMatch[2]})`;
                }
            }
        } else if (error && typeof error === 'object') {
            // 如果是對象，合併屬性
            errorObj = {
                ...error,
                message: error.message || extractErrorMessage(error),
                name: error.name || 'UnhandledRejection',
                originalReason: error
            };
            
            // 嘗試從錯誤對象中提取更多信息
            if (error.url || error.status) {
                source = `Promise Rejection (${error.status || ''} ${error.url || ''})`.trim();
            } else if (error.filename || error.file) {
                source = `Promise Rejection (${error.filename || error.file})`;
            }
        } else {
            // 其他類型（字符串、數字等）
            const errorMsg = extractErrorMessage(error);
            errorObj = {
                message: errorMsg || `Promise 被拒絕: ${String(error)}`,
                name: 'UnhandledRejection',
                originalReason: error
            };
        }
        
        // 添加調試信息到控制台（幫助開發者識別錯誤來源）
        if (console && console.debug) {
            console.debug('Promise Rejection 詳情:', {
                reason: error,
                reasonType: typeof error,
                isError: error instanceof Error,
                event: event
            });
        }
        
        logError(errorObj, source);
    });

    // 初始化：創建錯誤面板
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', createErrorPanel);
    } else {
        createErrorPanel();
    }

    // 提供全局 API 供手動觸發錯誤顯示
    window.JSErrorHandler = {
        show: function() {
            const panel = document.getElementById('js-error-panel');
            if (panel) {
                panel.classList.remove('hidden');
                panel.classList.remove('collapsed');
                localStorage.removeItem('js-error-panel-hidden');
            }
        },
        hide: function() {
            const panel = document.getElementById('js-error-panel');
            if (panel) {
                panel.classList.add('hidden');
                localStorage.setItem('js-error-panel-hidden', 'true');
            }
        },
        clear: function() {
            errorLog.length = 0;
            updateErrorDisplay();
        },
        getErrors: function() {
            return [...errorLog];
        }
    };

    // 在控制台輸出提示
    console.log('%c📱 JavaScript 錯誤面板已啟用', 'color: #4ec9b0; font-weight: bold;');
    console.log('使用 JSErrorHandler.show() 顯示錯誤面板');
    console.log('使用 JSErrorHandler.hide() 隱藏錯誤面板');
    console.log('使用 JSErrorHandler.clear() 清除錯誤記錄');
})();

