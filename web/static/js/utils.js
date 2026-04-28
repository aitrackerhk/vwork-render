// 通用工具函數

// 格式化日期（顯示用）
function formatDate(dateStr) {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleDateString('zh-TW');
}

// 格式化日期（表單輸入用，格式：YYYY-MM-DD）
function formatDateForInput(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}

// 格式化日期時間
function formatDateTime(dateStr) {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleString('zh-TW');
}

// =========================
// 匯出（Excel / PDF）
// - 優先使用「前端匯出」，確保所有 list 都能 work
// - 若非 DynamicList 頁面，才回退到舊的後端 export endpoint
// =========================

function _loadScriptOnce(id, src) {
    return new Promise((resolve, reject) => {
        const existing = document.getElementById(id);
        if (existing) {
            existing.addEventListener('load', () => resolve());
            existing.addEventListener('error', () => reject(new Error('script load failed')));
            // 可能已載入
            if (existing.getAttribute('data-loaded') === 'true') resolve();
            return;
        }
        const s = document.createElement('script');
        s.id = id;
        s.src = src;
        s.async = true;
        s.onload = () => {
            s.setAttribute('data-loaded', 'true');
            resolve();
        };
        s.onerror = () => reject(new Error(`Failed to load ${src}`));
        document.head.appendChild(s);
    });
}

async function ensureXlsxLib() {
    if (typeof XLSX !== 'undefined') return;
    await _loadScriptOnce('sheetjs-lib-local', '/static/vendor/xlsx.full.min.js');
}

async function ensureJsPdfLib() {
    // jsPDF 本體
    if (typeof window.jspdf === 'undefined' || !window.jspdf.jsPDF) {
        await _loadScriptOnce('jspdf-lib-local', '/static/vendor/jspdf.umd.min.js');
    }
    // autoTable plugin
    if (typeof window.jspdf !== 'undefined' && window.jspdf.jsPDF && (!window.jspdf.jsPDF.API || !window.jspdf.jsPDF.API.autoTable)) {
        await _loadScriptOnce('jspdf-autotable-lib-local', '/static/vendor/jspdf.plugin.autotable.min.js');
    }
}

function _downloadBlob(blob, filename) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
}

// 下載 API 檔案（帶 Authorization / Tenant header），避免 window.location 無法帶 header 的問題
async function downloadApiFile(url, filename) {
    const headers = {};

    const tenantSubdomain = localStorage.getItem('tenant_subdomain');
    if (tenantSubdomain) {
        headers['X-Tenant-Subdomain'] = tenantSubdomain;
    }
    const token = localStorage.getItem('auth_token');
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }

    const resp = await fetch(url, { method: 'GET', headers });
    if (!resp.ok) {
        let msg = `下載失敗 (${resp.status})`;
        try {
            const t = await resp.text();
            if (t) msg += `: ${t}`;
        } catch (_e) {}
        throw new Error(msg);
    }
    const blob = await resp.blob();
    _downloadBlob(blob, filename || 'download.xlsx');
}

function _formatExportValue(dynamicList, item, col) {
    const getNested = (obj, path) => {
        if (dynamicList && typeof dynamicList.getNestedValue === 'function') return dynamicList.getNestedValue(obj, path);
        return (path || '').split('.').reduce((cur, key) => cur?.[key], obj);
    };

    let value = getNested(item, col.key);
    switch (col.type) {
        case 'currency':
            return parseFloat(value || 0).toFixed(2);
        case 'date':
            return (typeof formatDate === 'function') ? formatDate(value) : (value || '');
        case 'datetime':
            return (typeof formatDateTime === 'function') ? formatDateTime(value) : (value || '');
        case 'relation':
            if (col.relationKey) return value?.[col.relationKey] || '';
            if (value && typeof value === 'object') return value.name || value.code || value.id || '';
            return value || '';
        case 'labels':
            if (!Array.isArray(value)) return '';
            return value.map(l => l?.name).filter(Boolean).join(', ');
        case 'image':
        case 'profile-image':
        case 'text-with-avatar':
        case 'badge':
            // 這些在 list 是 HTML 呈現，匯出只要純值
            if (col.type === 'badge' && (value === undefined || value === null)) return '';
            return (value !== undefined && value !== null) ? String(value) : '';
        default:
            return (value !== undefined && value !== null) ? String(value) : '';
    }
}

async function _fetchAllDynamicListRows(dynamicList) {
    const cfg = dynamicList?.config;
    if (!cfg?.apiPath) throw new Error('未找到列表配置');

    const pageSize = 1000;
    const maxRows = 20000; // 保護：避免匯出超大資料卡死

    const buildUrl = (page) => {
        const search = document.getElementById('searchInput')?.value || '';
        let url = `${cfg.apiPath}?page=${page}&limit=${pageSize}`;
        if (search) url += `&search=${encodeURIComponent(search)}`;

        if (cfg.filters) {
            cfg.filters.forEach(filter => {
                const el = document.getElementById(`filter_${filter.key}`);
                if (!el) return;
                let v = '';
                if (filter.relationApi && typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) {
                    v = $(el).val() || '';
                } else {
                    v = el.value || '';
                }
                if (v) url += `&${filter.key}=${encodeURIComponent(v)}`;
            });
        }
        return url;
    };

    const first = await App.apiRequest(buildUrl(1));
    const total = first.total || 0;
    let rows = first.data || [];
    const totalPages = Math.ceil(total / pageSize);

    for (let p = 2; p <= totalPages; p++) {
        if (rows.length >= maxRows) break;
        const res = await App.apiRequest(buildUrl(p));
        rows = rows.concat(res.data || []);
    }

    const truncated = total > rows.length;
    return { rows, truncated, total };
}

function _getExportFilename(prefix, ext) {
    const d = new Date();
    const pad = (n) => String(n).padStart(2, '0');
    const ts = `${d.getFullYear()}${pad(d.getMonth()+1)}${pad(d.getDate())}_${pad(d.getHours())}${pad(d.getMinutes())}`;
    return `${prefix || 'export'}_${ts}.${ext}`;
}

// 導出到 Excel（DynamicList：前端生成；其他：回退到後端）
async function exportToExcel(type) {
    try {
        if (window.dynamicList && window.dynamicList.config && window.dynamicList.config.columns) {
            await ensureXlsxLib();
            const { rows, truncated, total } = await _fetchAllDynamicListRows(window.dynamicList);
            const cols = (window.dynamicList.config.columns || []).filter(c => !!c && !!c.key);
            const header = cols.map(c => {
                const raw = c.label || c.key;
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const t = I18n.t(raw);
                    if (t && t !== raw) return t;
                }
                return raw;
            });
            const body = rows.map(r => cols.map(c => _formatExportValue(window.dynamicList, r, c)));
            const aoa = [header, ...body];

            const ws = XLSX.utils.aoa_to_sheet(aoa);
            const wb = XLSX.utils.book_new();
            let sheetName = window.dynamicList.config.title || 'Sheet1';
            if (typeof I18n !== 'undefined' && I18n.t) {
                const t = I18n.t(sheetName);
                if (t && t !== sheetName) sheetName = t;
            }
            sheetName = sheetName.slice(0, 31);
            XLSX.utils.book_append_sheet(wb, ws, sheetName);

            const filename = _getExportFilename(type || window.dynamicList.getPageName?.() || 'list', 'xlsx');
            XLSX.writeFile(wb, filename);
            if (truncated) {
                App.showAlert(`已匯出 ${rows.length}/${total} 筆（數據過大已截斷）`, 'warning');
            } else {
                App.showAlert(`已匯出 ${rows.length} 筆`, 'success');
            }
            return;
        }
    } catch (e) {
        console.warn('前端 Excel 匯出失敗：', e);
        App.showAlert('Excel 匯出失敗：' + (e?.message || e), 'danger');
        return;
    }

    // fallback（舊）
    window.location.href = `/api/v1/${type}/export/excel`;
}

// 導出到 PDF（DynamicList：前端生成；其他：回退到後端）
async function exportToPDF(type) {
    try {
        if (window.dynamicList && window.dynamicList.config && window.dynamicList.config.columns) {
            await ensureJsPdfLib();
            const { rows, truncated, total } = await _fetchAllDynamicListRows(window.dynamicList);
            const cols = (window.dynamicList.config.columns || []).filter(c => !!c && !!c.key);
            const head = [cols.map(c => {
                const raw = c.label || c.key;
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const t = I18n.t(raw);
                    if (t && t !== raw) return t;
                }
                return raw;
            })];
            const body = rows.map(r => cols.map(c => _formatExportValue(window.dynamicList, r, c)));

            const colCount = cols.length;
            const orientation = colCount > 6 ? 'landscape' : 'portrait';
            const doc = new window.jspdf.jsPDF({ orientation, unit: 'pt', format: 'a4' });

            let title = window.dynamicList.config.title || (type || 'Export');
            if (typeof I18n !== 'undefined' && I18n.t) {
                const t = I18n.t(title);
                if (t && t !== title) title = t;
            }
            doc.setFontSize(14);
            doc.text(String(title), 40, 40);

            doc.autoTable({
                head,
                body,
                startY: 60,
                styles: { fontSize: 8, cellPadding: 4 },
                headStyles: { fillColor: [33, 37, 41] },
                theme: 'grid'
            });

            const blob = doc.output('blob');
            const filename = _getExportFilename(type || window.dynamicList.getPageName?.() || 'list', 'pdf');
            _downloadBlob(blob, filename);

            if (truncated) {
                App.showAlert(`已匯出 ${rows.length}/${total} 筆（數據過大已截斷）`, 'warning');
            } else {
                App.showAlert(`已匯出 ${rows.length} 筆`, 'success');
            }
            return;
        }
    } catch (e) {
        console.warn('前端 PDF 匯出失敗：', e);
        App.showAlert('PDF 匯出失敗：' + (e?.message || e), 'danger');
        return;
    }

    // fallback（舊）
    window.location.href = `/api/v1/${type}/export/pdf`;
}

// 格式化金額
function formatCurrency(amount) {
    if (amount === null || amount === undefined) return '$0.00';
    return '$' + parseFloat(amount).toLocaleString('zh-TW', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

// 格式化數字
function formatNumber(num) {
    if (num === null || num === undefined) return '0';
    return parseFloat(num).toLocaleString('zh-TW');
}

// 更新 QR Code 預覽按鈕顯示狀態
function updateQRCodePreview(fieldId) {
    const input = document.getElementById(fieldId);
    const previewBtn = document.getElementById(`${fieldId}_preview_btn`);
    if (input && previewBtn) {
        if (input.value && input.value.trim() !== '') {
            previewBtn.style.display = 'inline-block';
        } else {
            previewBtn.style.display = 'none';
        }
    }
}

// 動態載入 QRCode 庫，確保存在 toCanvas
function ensureQRCodeLib(callback) {
    if (typeof QRCode !== 'undefined' && typeof QRCode.toCanvas === 'function') {
        callback();
        return;
    }

    const existing = document.getElementById('qrcode-lib');
    if (existing) {
        existing.onload = () => callback();
        existing.onerror = () => App.showAlert('QR Code 庫載入失敗', 'danger');
        return;
    }

    const script = document.createElement('script');
    script.id = 'qrcode-lib';
    script.src = '/static/js/qrcode.min.js';
    script.onload = () => callback();
    script.onerror = () => App.showAlert('QR Code 庫載入失敗', 'danger');
    document.head.appendChild(script);
}

// 顯示 QR Code 預覽
function showQRCodePreview(fieldId) {
    const input = document.getElementById(fieldId);
    if (!input || !input.value || input.value.trim() === '') {
        App.showAlert('請先輸入內容', 'warning');
        return;
    }
    
    const text = input.value.trim();
    
    // 創建模態框
    const modalId = 'qrCodePreviewModal';
    let modal = document.getElementById(modalId);
    if (!modal) {
        modal = document.createElement('div');
        modal.id = modalId;
        modal.className = 'modal fade';
        modal.innerHTML = `
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">QR Code 預覽</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                    </div>
                    <div class="modal-body text-center">
                        <div id="qrCodeCanvasContainer"></div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">關閉</button>
                    </div>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
    }
    
    // 清空容器
    const container = document.getElementById('qrCodeCanvasContainer');
    container.innerHTML = '';
    
    // 創建 canvas
    const canvas = document.createElement('canvas');
    container.appendChild(canvas);
    
    const renderQRCode = () => {
        try {
            if (typeof QRCode !== 'undefined' && typeof QRCode.toCanvas === 'function') {
                QRCode.toCanvas(canvas, text, {
                    width: 256,
                    margin: 2,
                    color: { dark: '#000000', light: '#FFFFFF' }
                }, function (error) {
                    if (error) {
                        console.error('QR Code 生成錯誤:', error);
                        App.showAlert('QR Code 生成失敗: ' + error.message, 'danger');
                        container.innerHTML = '<p class="text-danger">QR Code 生成失敗</p>';
                    }
                });
            } else if (typeof QRCode !== 'undefined') {
                // 後備：若無 toCanvas，使用 QRCode 實例化方式
                new QRCode(container, {
                    text,
                    width: 256,
                    height: 256,
                    colorDark: '#000000',
                    colorLight: '#ffffff',
                    correctLevel: QRCode.CorrectLevel.H
                });
            } else {
                App.showAlert('QR Code 庫未載入', 'danger');
            }
        } catch (error) {
            console.error('QR Code 生成錯誤:', error);
            App.showAlert('QR Code 生成失敗: ' + error.message, 'danger');
            container.innerHTML = '<p class="text-danger">QR Code 生成失敗</p>';
        }
    };

    // 確保庫載入後再渲染
    ensureQRCodeLib(renderQRCode);
    
    // 顯示模態框
    const bsModal = new bootstrap.Modal(modal);
    bsModal.show();
}

// ─── VaiSTT: Reusable Speech-to-Text for prompt inputs ─────────────
// Usage: VaiSTT.toggle('textareaId', buttonElement)
var VaiSTT = (function() {
    'use strict';

    var _recognition = null;
    var _listening = false;
    var _baseText = '';
    var _targetId = null;
    var _activeBtn = null;

    function _getLang() {
        var htmlLang = (document.documentElement.getAttribute('lang') || '').toLowerCase();
        if (htmlLang.startsWith('zh-cn')) return 'zh-CN';
        if (htmlLang.startsWith('zh')) return 'zh-TW';
        return 'en-US';
    }

    function _updateBtnState(btn, listening) {
        if (!btn) return;
        var icon = btn.querySelector('i');
        if (!icon) return;
        // Detect original button style (btn-primary or btn-outline-secondary)
        var isPrimary = btn.classList.contains('btn-primary') || btn.dataset.btnStyle === 'primary';
        if (listening) {
            icon.className = 'bi bi-mic-fill';
            btn.classList.add('btn-danger');
            if (isPrimary) {
                btn.dataset.btnStyle = 'primary';
                btn.classList.remove('btn-primary');
            } else {
                btn.classList.remove('btn-outline-secondary');
            }
        } else {
            icon.className = 'bi bi-mic';
            btn.classList.remove('btn-danger');
            if (btn.dataset.btnStyle === 'primary') {
                btn.classList.add('btn-primary');
            } else {
                btn.classList.add('btn-outline-secondary');
            }
        }
    }

    function stop() {
        if (_recognition) {
            try { _recognition.abort(); } catch(_) {}
            _recognition = null;
        }
        _listening = false;
        _updateBtnState(_activeBtn, false);
        _activeBtn = null;
        _targetId = null;
    }

    function toggle(textareaId, btn) {
        // If already listening on this target, stop
        if (_listening && _targetId === textareaId) {
            stop();
            return;
        }
        // If listening on a different target, stop first
        if (_listening) stop();

        var SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
        if (!SpeechRecognition) {
            if (typeof App !== 'undefined' && App.showAlert) {
                App.showAlert(typeof I18n !== 'undefined' ? I18n.t('vai.chat.voiceInputNotSupported') : 'Speech recognition not supported', 'warning');
            }
            return;
        }

        var input = document.getElementById(textareaId);
        if (!input) return;

        _targetId = textareaId;
        _activeBtn = btn;
        _baseText = input.value ? input.value.trim() : '';

        _recognition = new SpeechRecognition();
        _recognition.lang = _getLang();
        _recognition.interimResults = true;
        _recognition.continuous = true;

        _recognition.onstart = function() {
            _listening = true;
            _updateBtnState(_activeBtn, true);
        };

        _recognition.onresult = function(event) {
            var transcript = '';
            for (var i = event.resultIndex; i < event.results.length; i++) {
                transcript += event.results[i][0].transcript;
            }
            input.value = (_baseText + ' ' + transcript).trim();
            // Auto-resize textarea
            input.style.height = 'auto';
            input.style.height = Math.min(input.scrollHeight, 200) + 'px';
            // Dispatch input event so mic/send toggles can react
            input.dispatchEvent(new Event('input', { bubbles: true }));
        };

        _recognition.onerror = function(event) {
            if (event.error === 'not-allowed' || event.error === 'service-not-allowed') {
                if (typeof App !== 'undefined' && App.showAlert) {
                    App.showAlert(typeof I18n !== 'undefined' ? I18n.t('vai.chat.voiceInputPermissionDenied') : 'Microphone permission denied', 'warning');
                }
            }
            // For other errors (no-speech, aborted), silently ignore
        };

        _recognition.onend = function() {
            _listening = false;
            _recognition = null;
            _updateBtnState(_activeBtn, false);
            // Dispatch input event on target so mic/send toggles update
            if (input) input.dispatchEvent(new Event('input', { bubbles: true }));
            _activeBtn = null;
            _targetId = null;
        };

        try {
            _recognition.start();
        } catch (e) {
            _recognition = null;
            _listening = false;
            _updateBtnState(_activeBtn, false);
            _activeBtn = null;
            _targetId = null;
        }
    }

    return {
        toggle: toggle,
        stop: stop,
        isListening: function() { return _listening; }
    };
})();
