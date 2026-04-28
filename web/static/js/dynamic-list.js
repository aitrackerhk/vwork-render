// 動態列表頁面 JavaScript

class DynamicList {
    constructor(config) {
        this.config = this.normalizeConfig(config);
        this.currentPage = 1;
        this.totalItems = 0;
        this.totalPages = 1;
        this.pageSize = 20;
        this.selectedItems = new Set(); // 存儲選中的項目ID
        this.labelOptions = [];
        this.selectedLabelIds = new Set();
        this.trashMode = false; // 垃圾筒模式
        this.init();
    }

    normalizeConfig(config) {
        const cfg = config || {};
        const filters = Array.isArray(cfg.filters) ? [...cfg.filters] : [];
        const formFields = Array.isArray(cfg.formFields) ? cfg.formFields : [];
        const hasStatusFilter = filters.some(f => f && f.key === 'status');
        const statusField = formFields.find(f => f && f.key === 'status');

        if (statusField && !hasStatusFilter) {
            const label = statusField.label || '狀態';
            const options = Array.isArray(statusField.options) ? statusField.options : [];
            const hasAll = options.some(opt => opt && (opt.value === '' || opt.value === null || opt.value === undefined));
            const allOption = { value: '', label: '全部', labelKey: 'common.all' };
            if (options.length > 0) {
                filters.unshift({
                    key: 'status',
                    label,
                    type: 'select',
                    options: hasAll ? options : [allOption, ...options]
                });
            } else {
                filters.unshift({
                    key: 'status',
                    label,
                    type: 'text'
                });
            }
        }

        cfg.filters = filters;
        return cfg;
    }

    async init() {
        // 檢查 App 是否已載入
        if (typeof App === 'undefined' || !App.checkAuth()) {
            // 如果 App 未載入，等待一下再重試
            if (typeof App === 'undefined') {
                setTimeout(() => this.init(), 100);
                return;
            }
            return;
        }

        // Wait for i18n translations to be loaded before rendering,
        // otherwise I18n.t() calls during render() return raw keys
        // instead of translated text (race condition on page refresh).
        if (typeof I18n !== 'undefined' && I18n.whenReady) {
            await I18n.whenReady(3000);
        }
        
        // 根據 URL 參數動態調整配置（如 orders 頁面的 source_type）
        this.applyURLParamsToConfig();
        
        this.render();
        this.applyDocumentAutoSettingsToToolbar();
        this.bindEvents();
        this.applyURLParamsToFilters();
        this.initLabelFilter();
        this.loadData();
    }

    // 取得 menu 翻譯 key（允許由 config 覆寫）
    getMenuTitleKey() {
        return this.config.titleKey || `menu.${this.getTranslationKey()}`;
    }

    // 取得 menu 顯示文字（優先 i18n，其次 fallback 到 config.title）
    getMenuTitle() {
        const titleKey = this.getMenuTitleKey();
        if (typeof I18n !== 'undefined' && I18n.t) {
            const t = I18n.t(titleKey);
            if (t && t !== titleKey) return t;
        }
        return this.config.title || '';
    }
    
    // 根據 URL 參數調整頁面配置（標題、圖標等）
    applyURLParamsToConfig() {
        try {
            const params = new URLSearchParams(window.location.search || '');
            const pageName = this.getPageName();
            
            // 訂單頁面根據 source_type 動態調整標題和圖標
            if (pageName === 'orders') {
                const sourceType = params.get('source_type');
                if (sourceType) {
                    const sourceTypeConfigs = {
                        dining: { 
                            title: '餐飲訂單', 
                            // 與側邊欄菜單一致：/orders?source_type=dining
                            icon: 'bi-receipt',
                            titleKey: 'menu.diningOrders'
                        },
                        pos: { 
                            title: 'POS 訂單', 
                            icon: 'bi-cash-stack',
                            titleKey: 'menu.posOrders'
                        },
                        webstore: { 
                            title: '網店訂單', 
                            icon: 'bi-globe',
                            titleKey: 'menu.webstoreOrders'
                        },
                        erp: { 
                            title: 'ERP 訂單', 
                            icon: 'bi-building',
                            titleKey: 'menu.erpOrders'
                        }
                    };
                    
                    const sourceConfig = sourceTypeConfigs[sourceType];
                    if (sourceConfig) {
                        this.config.title = sourceConfig.title;
                        this.config.icon = sourceConfig.icon;
                        this.config.titleKey = sourceConfig.titleKey;
                    }
                }
            }

            // 服務單/服務員/預約頁面根據 service_type_code 動態調整標題和圖標
            const serviceTypeCode = params.get('service_type_code');
            if (serviceTypeCode) {
                const serviceTypeOverrides = {
                    'service-orders': {
                        course: { title: '課程', icon: 'bi-journal-bookmark', titleKey: 'menu.courseOrders' }
                    },
                    'service-staffs': {
                        course: { title: '導師', icon: 'bi-person-video3', titleKey: 'menu.courseInstructors' }
                    },
                    'appointments': {
                        course: { title: '課程預約', icon: 'bi-calendar-check', titleKey: 'menu.courseAppointments' }
                    }
                };
                const pageOverrides = serviceTypeOverrides[pageName];
                if (pageOverrides && pageOverrides[serviceTypeCode]) {
                    const cfg = pageOverrides[serviceTypeCode];
                    this.config.title = cfg.title;
                    this.config.icon = cfg.icon;
                    this.config.titleKey = cfg.titleKey;
                }
            }
        } catch (e) {
            console.warn('applyURLParamsToConfig error:', e);
        }
    }

    async applyDocumentAutoSettingsToToolbar() {
        try {
            if (this.getPageName() !== 'expenses') return;
            const btn = document.getElementById('generateMonthlyCommissionsBtn');
            if (!btn) return;
            const auto = await App.apiRequest('/document-auto-settings');
            const enabled = auto && auto.auto_generate_commission !== false;
            if (enabled) {
                btn.style.display = 'none';
            }
        } catch (e) {
            // 忽略：工具列不影響列表正常使用
        }
    }

    // 支援從 URL query 初始化 filters/search（例如 /expenses?project_id=...）
    applyURLParamsToFilters() {
        try {
            const params = new URLSearchParams(window.location.search || '');
            if (!params || Array.from(params.keys()).length === 0) return;

            // search
            const search = params.get('search');
            const searchInput = document.getElementById('searchInput');
            if (search && searchInput) {
                searchInput.value = search;
            }

            if (this.config.labelFilter) {
                const labelKey = this.config.labelFilter.paramKey || 'label_id';
                const labelParam = params.get(labelKey) || params.get('label_ids');
                if (labelParam) {
                    const ids = labelParam.split(',').map(v => v.trim()).filter(Boolean);
                    ids.forEach(id => this.selectedLabelIds.add(id));
                }
            }

            if (!this.config.filters) return;
            const labelFilterKey = this.config.labelFilter ? (this.config.labelFilter.paramKey || 'label_id') : '';
            this.config.filters.forEach(filter => {
                if (labelFilterKey && filter.key === labelFilterKey) return;
                let v = params.get(filter.key);

                // Support _code variant: e.g. URL has service_type_code=course → resolve to service_type_id
                if (!v && filter.key.endsWith('_id') && filter.relationApi) {
                    const codeKey = filter.key.replace(/_id$/, '_code');
                    const codeVal = params.get(codeKey);
                    if (codeVal) {
                        this._resolveCodeToId(filter, codeVal);
                        return;
                    }
                }

                if (!v) return;
                const el = document.getElementById(`filter_${filter.key}`);
                if (!el) return;

                // 對於 select2：先嘗試直接 set value；若 option 不存在則補一個 placeholder option
                if (filter.relationApi && typeof $ !== 'undefined') {
                    // 先補 option（避免 select2 沒值）
                    if ($(el).find(`option[value="${v}"]`).length === 0) {
                        const opt = new Option(v, v, true, true);
                        $(el).append(opt);
                    }
                    // 初始化完成後再 set
                    setTimeout(() => {
                        try { $(el).val(v).trigger('change'); } catch {}
                    }, 300);
                } else {
                    el.value = v;
                }
            });
        } catch (e) {
            console.warn('applyURLParamsToFilters failed', e);
        }
    }

    // Resolve a code value (e.g. "course") to an ID via the filter's relationApi,
    // then set it on the filter select2 element and reload data.
    async _resolveCodeToId(filter, codeVal) {
        try {
            const resp = await App.apiRequest(`${filter.relationApi}?code=${encodeURIComponent(codeVal)}&limit=1`);
            const items = resp && resp.data ? resp.data : (Array.isArray(resp) ? resp : []);
            if (items.length === 0) return;
            const item = items[0];
            const id = item.id;
            const label = item[filter.relationLabel || 'name'] || codeVal;
            const el = document.getElementById(`filter_${filter.key}`);
            if (!el) return;
            if (typeof $ !== 'undefined') {
                if ($(el).find(`option[value="${id}"]`).length === 0) {
                    const opt = new Option(label, id, true, true);
                    $(el).append(opt);
                }
                setTimeout(() => {
                    try {
                        $(el).val(id).trigger('change');
                    } catch {}
                }, 300);
            } else {
                el.value = id;
            }
        } catch (e) {
            console.warn('_resolveCodeToId failed for', filter.key, codeVal, e);
        }
    }

    // 獲取翻譯的輔助函數
    getText(key) {
        if (typeof I18n !== 'undefined' && I18n.t) {
            return I18n.t(key);
        }
        // 後備方案
        const fallback = {
            'common.draft': '草稿',
            'common.add': '新增',
            'common.excel': 'Excel',
            'common.pdf': 'PDF',
            'common.search': '搜索',
            'common.searchPlaceholder': '輸入關鍵字搜索...',
            'common.edit': '編輯',
            'common.delete': '刪除',
            'common.cancel': '取消',
            'common.cancelTrashMode': '取消垃圾筒模式',
            'common.actions': '操作',
            'common.confirmDelete': '確定要刪除這個',
            'common.deleteSuccess': '刪除成功',
            'common.deleteError': '刪除失敗',
            'common.cancelled': '已取消',
            'common.completed': '已完成',
            'common.markAsCancelled': '標記為已取消',
            'common.markAsCompleted': '標記為已完成',
            'common.approve': '批核',
            'common.reject': '拒絕',
            'common.autoGenerateMonthlyPayrolls': '自動生成本月薪資',
            'common.autoGenerateMonthlyCommissions': '自動生成本月所有佣金',
            'common.noData': '暫無數據',
            'common.trash': '垃圾筒',
            'common.viewTrash': '查看垃圾筒',
            'common.backToList': '返回列表',
            'common.restore': '還原',
            'common.permanentDelete': '永久刪除',
            'common.restoreSuccess': '還原成功',
            'common.restoreError': '還原失敗',
            'common.confirmTrash': '確定要刪除這個',
            'common.trashInfo': '資料將移至垃圾筒，7 日後自動刪除',
            'common.confirmPermanentDelete': '確定要永久刪除這個',
            'common.permanentDeleteWarning': '此操作無法復原！',
            'common.confirmRestore': '確定要還原這個',
            'common.trashNotSupported': '此頁面不支援垃圾筒功能',
            'common.labels': '標籤',
            'common.noLabels': '暫無標籤',
            'common.selectLabels': '選擇標籤',
            'common.modify': '更改'
        };
        return fallback[key] || key;
    }

    render() {
        const container = document.getElementById('dynamicListContainer');
        if (!container) return;

        // 翻譯 filter label（支援 labelKey / label 本身是 i18n key / fields.<key> / fields.<中文label>）
        const resolveFilterLabel = (filter) => {
            const rawLabel = (filter && filter.label != null) ? String(filter.label) : '';
            const rawKey = (filter && filter.key != null) ? String(filter.key) : '';
            if (typeof I18n === 'undefined' || !I18n.t) return rawLabel;

            if (filter && filter.labelKey) {
                const t0 = I18n.t(filter.labelKey);
                if (t0 && t0 !== filter.labelKey) return t0;
            }

            // label 本身就是 key：例如 fields.startDate
            if (rawLabel && rawLabel.includes('.') && !/[\u3400-\u9FFF]/.test(rawLabel)) {
                const t1 = I18n.t(rawLabel);
                if (t1 && t1 !== rawLabel) return t1;
            }

            if (rawKey) {
                const k2 = `fields.${rawKey}`;
                const t2 = I18n.t(k2);
                if (t2 && t2 !== k2) return t2;
            }

            if (rawLabel && /[\u3400-\u9FFF]/.test(rawLabel)) {
                const k3 = `fields.${rawLabel}`;
                const t3 = I18n.t(k3);
                if (t3 && t3 !== k3) return t3;
            }

            return rawLabel;
        };

        // 匯出（Excel/PDF）：不再在「無資料」時 disabled
        // 改為：仍可點擊，但在 exportToExcel/exportToPDF 內提示「沒有資料」。

        // 獲取草稿數量
        const draftCount = draftManager.getDraftCount(this.getPageName());

        // 渲染標題和操作按鈕
        // 工具列
        let toolbarButtons = '';
            toolbarButtons += `
            <button id="exportExcelBtn" class="btn btn-success" onclick="window.dynamicList.exportToExcel()" data-i18n="common.excel">
                    <i class="bi bi-file-earmark-excel"></i> <span class="d-none d-md-inline">${this.getText('common.excel')}</span>
                </button>
            <button id="exportPdfBtn" class="btn btn-danger" onclick="window.dynamicList.exportToPDF()" data-i18n="common.pdf">
                    <i class="bi bi-file-earmark-pdf"></i> <span class="d-none d-md-inline">${this.getText('common.pdf')}</span>
                </button>
            `;
        // payrolls 專屬生成按鈕（在 draft button 之前）
        if (this.getPageName() === 'payrolls') {
            toolbarButtons += `
                <button id="generatePayrollsBtn" class="btn btn-outline-secondary" data-i18n="common.autoGenerateMonthlyPayrolls">
                    ${this.getText('common.autoGenerateMonthlyPayrolls')}
                </button>
            `;
        }
        // expenses 專屬生成按鈕
        if (this.getPageName() === 'expenses') {
            toolbarButtons += `
                <button id="generateMonthlyCommissionsBtn" class="btn btn-outline-primary" onclick="window.dynamicList.generateMonthlyCommissions()" data-i18n="common.autoGenerateMonthlyCommissions">
                    <i class="bi bi-cash-coin"></i> ${this.getText('common.autoGenerateMonthlyCommissions')}
                </button>
            `;
        }
        // currencies 專屬更新匯率按鈕
        if (this.getPageName() === 'currencies') {
            toolbarButtons += `
                <button class="btn btn-outline-success" onclick="window.dynamicList.updateCurrencyRates()" data-i18n="common.updateExchangeRates">
                    <i class="bi bi-arrow-repeat"></i> ${this.getText('common.updateExchangeRates') || '自動更新匯率'}
                </button>
            `;
        }
        // users 專屬邀請成員按鈕
        if (this.getPageName() === 'users') {
            toolbarButtons += `
                <button class="btn btn-outline-primary" onclick="if(window.openInviteModal)window.openInviteModal()" data-i18n="invitePopup.title">
                    <i class="bi bi-envelope-plus"></i> <span class="d-none d-md-inline">${this.getText('invitePopup.title') || '邀請成員'}</span>
                </button>
            `;
        }
        // 只有當配置允許時才顯示草稿按鈕
        if (this.config.showDraftButton !== false) {
            toolbarButtons += `
                <div class="position-relative">
                    <button class="btn btn-outline-secondary" onclick="window.dynamicList.showDraftList()" title="${this.getText('common.draft')}" data-i18n="common.draft">
                        <i class="bi bi-file-earmark-text"></i> <span data-i18n="common.draft">${this.getText('common.draft')}</span>
                        ${draftCount > 0 ? ` <span class="badge bg-danger">${draftCount}</span>` : ''}
                    </button>
                </div>
            `;
        }
        if ((this.config.formFields && this.config.formFields.length > 0) || this.config.showAddButton) {
            // 保留當前 URL 的 query params（如 source_type）
            const currentParams = new URLSearchParams(window.location.search);
            const addUrl = currentParams.toString() 
                ? `${this.config.listPath}/new?${currentParams.toString()}`
                : `${this.config.listPath}/new`;
            toolbarButtons += `
                <a href="${addUrl}" class="btn btn-primary" data-i18n="common.add">
                    <i class="bi bi-plus-circle"></i> <span data-i18n="common.add">${this.getText('common.add')}</span>
                </a>
            `;
        }

        // 獲取標題翻譯（如果有的話）
        const titleKey = this.getMenuTitleKey();
        const title = this.getMenuTitle();
        const icon = this.config.icon ? `<i class="bi ${this.config.icon} me-2"></i>` : '';
        
        const header = `
            <div class="d-flex justify-content-between align-items-center mb-4">
                <h3 class="d-flex align-items-center mb-0">
                    ${icon ? icon : ''}
                    <span data-i18n="${titleKey}">${title}</span>
                </h3>
                <div class="d-flex gap-2 align-items-center">
                    ${toolbarButtons}
                </div>
            </div>
        `;

        // 渲染搜索和過濾器
        const labelFilterKey = this.config.labelFilter ? (this.config.labelFilter.paramKey || 'label_id') : '';
        const visibleFilters = this.config.filters ? this.config.filters.filter(filter => !(labelFilterKey && filter.key === labelFilterKey)) : [];
        
        // 動態計算欄寬，確保搜索框和所有過濾器在同一行
        // 總共 12 欄，根據過濾器數量分配
        const filterCount = visibleFilters.length;
        let searchCol, filterCol;
        if (filterCount === 0) {
            searchCol = '12';
            filterCol = '12';
        } else if (filterCount === 1) {
            searchCol = '6';
            filterCol = '6';
        } else if (filterCount === 2) {
            searchCol = '4';
            filterCol = '4';
        } else if (filterCount === 3) {
            searchCol = '3';
            filterCol = '3';
        } else {
            // 4 個以上過濾器，搜索框佔 2 欄，過濾器平分剩餘空間
            searchCol = '2';
            filterCol = String(Math.floor(10 / filterCount));
        }
        
        const filters = `
            <div class="mb-4">
                <div class="row g-3 align-items-end">
                    <div class="col-md-${searchCol}">
                        <div class="search-box-wrapper">
                            <div class="input-group search-box">
                                <input type="text" class="form-control" id="searchInput" 
                                       placeholder="${(this.config.searchPlaceholderKey && typeof I18n !== 'undefined' && I18n.t) ? this.getText(this.config.searchPlaceholderKey) : this.getText('common.searchPlaceholder')}" data-i18n-placeholder="${this.config.searchPlaceholderKey || 'common.searchPlaceholder'}" 
                                       onkeydown="window.dynamicList.handleSearchKey(event)">
                                <button class="btn btn-outline-secondary border-start-0 border-end-0" type="button" id="clearSearchBtn" style="display: none;" onclick="document.getElementById('searchInput').value=''; window.dynamicList.loadData(); this.style.display='none';">
                                    <i class="bi bi-x-lg"></i>
                                </button>
                                <button class="btn btn-outline-secondary border-0" type="button" id="searchBtn" onclick="window.dynamicList.submitSearch()">
                                    <i class="bi bi-search"></i>
                                </button>
                            </div>
                        </div>
                    </div>
                    ${visibleFilters.map(filter => {
                        const col = filterCol;
                        const defaultVal = (filter.defaultValue !== undefined && filter.defaultValue !== null) ? String(filter.defaultValue) : '';

                        // relationApi 一律用 select2
                        if (filter.relationApi) {
                            const filterLabel = resolveFilterLabel(filter);
                            return `
                                <div class="col-md-${col}">
                                    <label class="form-label fw-semibold text-muted mb-2">${filterLabel}</label>
                                    <select class="form-select filter-select" id="filter_${filter.key}" onchange="window.dynamicList.loadData()">
                                        ${filter.options ? filter.options.map(opt => {
                                            const raw = opt.label != null ? String(opt.label) : '';
                                            const v = opt.value != null ? String(opt.value) : '';
                                            let label = raw || v;
                                            let i18nKey = opt.labelKey ? String(opt.labelKey) : '';

                                            // 若未提供 labelKey，對常見的 dropdown（status/leave_type/boolean）
                                            // 直接補上 i18n key，讓語言檔晚於列表渲染載入時也能更新。
                                            // （避免使用「t(key) !== key」來判斷，因為語言檔可能尚未載入）
                                            if (!i18nKey && filter && filter.key && v) {
                                                if (filter.key === 'status') i18nKey = `options.status.${v}`;
                                                if (filter.key === 'leave_type') i18nKey = `options.leave_type.${v}`;
                                            }
                                            if (!i18nKey && (v === 'true' || v === 'false' || v === '1' || v === '0')) {
                                                const normalized = (v === '1') ? 'true' : (v === '0' ? 'false' : v);
                                                i18nKey = `options.boolean.${normalized}`;
                                            }

                                            // 嘗試在渲染時翻譯（若語言檔已載入），同時保留 data-i18n 供後續 updatePage 使用。
                                            if (typeof I18n !== 'undefined' && I18n.t) {
                                                if (i18nKey) {
                                                    const t = I18n.t(i18nKey);
                                                    if (t && t !== i18nKey) label = t;
                                                } else if (this.getPageName() && filter.key && v) {
                                                    const k1 = `options.${this.getPageName()}.${filter.key}.${v}`;
                                                    const t1 = I18n.t(k1);
                                                    if (t1 && t1 !== k1) label = t1;
                                                } else if (filter.key && v) {
                                                    const k2 = `options.${filter.key}.${v}`;
                                                    const t2 = I18n.t(k2);
                                                    if (t2 && t2 !== k2) label = t2;
                                                }
                                            }

                                            const dataI18nAttr = i18nKey ? ` data-i18n="${i18nKey}"` : '';
                                            return `<option value="${opt.value}"${dataI18nAttr}>${label}</option>`;
                                        }).join('') : ''}
                                    </select>
                                </div>
                            `;
                        }

                        // 支援 date / month / text
                        if (filter.type === 'date') {
                            const filterLabel = resolveFilterLabel(filter);
                            return `
                                <div class="col-md-${col}">
                                    <label class="form-label fw-semibold text-muted mb-2">${filterLabel}</label>
                                    <input type="date" class="form-control filter-input" id="filter_${filter.key}" value="${defaultVal}" onchange="window.dynamicList.loadData()">
                                </div>
                            `;
                        }
                        if (filter.type === 'month') {
                            const filterLabel = resolveFilterLabel(filter);
                            return `
                                <div class="col-md-${col}">
                                    <label class="form-label fw-semibold text-muted mb-2">${filterLabel}</label>
                                    <input type="month" class="form-control filter-input" id="filter_${filter.key}" value="${defaultVal}" onchange="window.dynamicList.loadData()">
                                </div>
                            `;
                        }
                        if (filter.type === 'text') {
                            const filterLabel = resolveFilterLabel(filter);
                            const placeholderKey = this.getPageName() ? `placeholders.${this.getPageName()}.${filter.key}` : '';
                            let placeholder = filter.placeholder || '';
                            if (placeholder && typeof I18n !== 'undefined' && I18n.t) {
                                if (placeholderKey) {
                                    const t1 = I18n.t(placeholderKey);
                                    if (t1 && t1 !== placeholderKey) placeholder = t1;
                                }
                                const k2 = `placeholders.${filter.key}`;
                                const t2 = I18n.t(k2);
                                if (t2 && t2 !== k2) placeholder = t2;
                            }
                            return `
                                <div class="col-md-${col}">
                                    <label class="form-label fw-semibold text-muted mb-2">${filterLabel}</label>
                                     <input type="text" class="form-control filter-input" id="filter_${filter.key}" value="${defaultVal}" placeholder="${placeholder}"
                                         onkeydown="window.dynamicList.handleFilterKey(event)">
                                </div>
                            `;
                        }

                        // default：select
                        const filterLabel = resolveFilterLabel(filter);
                        return `
                            <div class="col-md-${col}">
                                <label class="form-label fw-semibold text-muted mb-2">${filterLabel}</label>
                                <select class="form-select filter-select" id="filter_${filter.key}" onchange="window.dynamicList.loadData()">
                                    ${(filter.options || []).map(opt => {
                                        const raw = opt.label != null ? String(opt.label) : '';
                                        const v = opt.value != null ? String(opt.value) : '';
                                        let label = raw || v;
                                        let i18nKey = opt.labelKey ? String(opt.labelKey) : '';

                                        if (typeof I18n !== 'undefined' && I18n.t) {
                                            if (!i18nKey && filter && filter.key && v) {
                                                if (filter.key === 'status') i18nKey = `options.status.${v}`;
                                                if (filter.key === 'leave_type') i18nKey = `options.leave_type.${v}`;
                                            }
                                            if (!i18nKey && (v === 'true' || v === 'false' || v === '1' || v === '0')) {
                                                const normalized = (v === '1') ? 'true' : (v === '0' ? 'false' : v);
                                                i18nKey = `options.boolean.${normalized}`;
                                            }

                                            if (i18nKey) {
                                                const t = I18n.t(i18nKey);
                                                if (t && t !== i18nKey) label = t;
                                            } else if (this.getPageName() && filter.key && v) {
                                                const k1 = `options.${this.getPageName()}.${filter.key}.${v}`;
                                                const t1 = I18n.t(k1);
                                                if (t1 && t1 !== k1) label = t1;
                                            } else if (filter.key && v) {
                                                const k2 = `options.${filter.key}.${v}`;
                                                const t2 = I18n.t(k2);
                                                if (t2 && t2 !== k2) label = t2;
                                            }
                                        }

                                        const dataI18nAttr = i18nKey ? ` data-i18n="${i18nKey}"` : '';
                                        return `<option value="${opt.value}"${dataI18nAttr}>${label}</option>`;
                                    }).join('')}
                                </select>
                            </div>
                        `;
                    }).join('')}
                </div>
            </div>
        `;

        // 渲染表格 - 添加字段標籤翻譯支持
        const getFieldLabel = (col) => {
            // 優先使用 col.labelKey（頁面特定翻譯鍵，如 stampSettingsPage.columns.productStampEnabled）
            if (typeof I18n !== 'undefined' && I18n.t && col && col.labelKey) {
                try {
                    const tLabel = I18n.t(col.labelKey);
                    if (tLabel && tLabel !== col.labelKey) return tLabel;
                } catch (e) {
                    // ignore
                }
            }
            // 若 col.label 本身就是 i18n key（例如 attendanceReports.totalDays、fields.employee），直接翻譯它
            if (typeof I18n !== 'undefined' && I18n.t && col && col.label) {
                try {
                    const rawLabel = String(col.label);
                    // 排除明顯是中文的情況，避免把中文當 key 查找造成多餘計算
                    if (rawLabel.includes('.') && !/[\u3400-\u9FFF]/.test(rawLabel)) {
                        const tLabel = I18n.t(rawLabel);
                        if (tLabel && tLabel !== rawLabel) return tLabel;
                    }
                } catch (e) {
                    // ignore
                }
            }

            // 優先使用 col.key 來查找翻譯（因為這是標準的字段鍵）
            if (typeof I18n !== 'undefined' && I18n.t) {
                // 某些頁面同一個 key 在不同資源代表不同語意（例如 payment-methods 的 is_default）
                // 若 config.columns 設定 preferLabel=true，則跳過 fields.<key> 覆蓋，直接用 col.label（必要時可在上方 i18n key 分支翻譯）。
                if (col && col.preferLabel && col.label) {
                    return col.label;
                }

                // 處理關係字段（如 member_level.name）
                let fieldKey = col.key;
                if (fieldKey.includes('.')) {
                    // 對於關係字段，優先嘗試將整個鍵轉換為駝峰命名（如 member_level.name -> memberLevelName）
                    const parts = fieldKey.split('.');
                    const camelCaseKey = parts.map((p, i) => i === 0 ? p : p.charAt(0).toUpperCase() + p.slice(1)).join('');
                    let translated = I18n.t(`fields.${camelCaseKey}`);
                    if (translated && translated !== `fields.${camelCaseKey}`) {
                        return translated;
                    }
                    
                    // 如果找不到，嘗試使用第一部分轉換為駝峰（如 member_level.name -> memberLevel）
                    if (parts.length >= 2) {
                        const firstPartCamel = parts[0].split('_').map((p, i) => i === 0 ? p : p.charAt(0).toUpperCase() + p.slice(1)).join('');
                        translated = I18n.t(`fields.${firstPartCamel}`);
                        if (translated && translated !== `fields.${firstPartCamel}`) {
                            return translated;
                        }
                    }
                    
                    // 如果還是找不到，嘗試使用最後一部分（如 member_level.name -> name）
                    const lastPart = parts[parts.length - 1];
                    translated = I18n.t(`fields.${lastPart}`);
                    if (translated && translated !== `fields.${lastPart}`) {
                        return translated;
                    }
                    
                    // 如果還是找不到，使用 label
                    fieldKey = lastPart;
                } else {
                    // 直接使用 col.key 查找 fields.${col.key}
                    const translated = I18n.t(`fields.${fieldKey}`);
                    if (translated && translated !== `fields.${fieldKey}`) {
                        return translated;
                    }
                }
                
                // 如果 col.key 找不到，嘗試使用 label 轉換（先嘗試直接匹配，再嘗試轉換）
                if (col.label) {
                    // 先嘗試直接使用 label 作為鍵（處理已經翻譯過的 label）
                    const directLabelKey = col.label.toLowerCase().replace(/\s+/g, '').replace(/[()（）]/g, '');
                    let translatedByLabel = I18n.t(`fields.${directLabelKey}`);
                    if (translatedByLabel && translatedByLabel !== `fields.${directLabelKey}`) {
                        return translatedByLabel;
                    }

                    // 如果 label 是中文（或包含中文），直接嘗試 fields.<中文label>
                    if (/[\u3400-\u9FFF]/.test(col.label)) {
                        const k = `fields.${col.label}`;
                        const t = I18n.t(k);
                        if (t && t !== k) {
                            return t;
                        }
                    }
                    
                    // 如果 label 是中文，嘗試從中文翻譯文件中查找對應的英文鍵
                    // 這是一個後備方案，用於處理配置中使用中文 label 的情況
                    // 但優先使用 col.key 的翻譯
                }
            }
            // 如果找不到翻譯，返回原始 label（這可能是中文，但至少能顯示）
            return col.label || col.key;
        };
        
        const tableHeaders = this.config.columns.map(col => {
            const translatedLabel = getFieldLabel(col);
            // 如果是图片类型，设置固定宽度为 60px
            const widthStyle = col.type === 'image' ? ' style="width: 60px;"' : '';
            // If col has a labelKey, add data-i18n so updatePage() can re-translate on language switch
            const i18nAttr = col.labelKey ? ` data-i18n="${col.labelKey}"` : '';
            return `<th data-i18n-field="${col.key}"${i18nAttr}${widthStyle}>${translatedLabel}</th>`;
        }).join('');
        
        const actionsText = this.getText('common.actions');
        const loadingText = this.getText('common.loading');
        
        // 檢查是否顯示操作列
        const showActions = this.config.showActions !== false; // 默認顯示，除非明確設置為 false
        
        // 更多操作下拉菜单和确认删除按钮（i18n）
        const moreActionsText = this.getText('common.moreActions');
        const bulkDeleteText = this.getText('common.bulkDelete');
        const confirmDeleteSelectedText = this.getText('common.confirmDeleteSelected');

        // 額外更多動作（由 page-configs.js 提供；若未提供，則使用預設模板動作）
        // 注意：匯入按鈕改為獨立顯示在「更多動作」旁，不放在下拉選單內
        const apiPath = this.config?.apiPath || '';
        const canImport = !!apiPath && this.config?.enableImportActions !== false;
        const defaultMoreActions = canImport ? [
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（不含資料）', icon: 'bi bi-download', arg: false },
            { id: 'downloadImportTemplate', label: '下載匯入 Excel（含資料）', icon: 'bi bi-download', arg: true },
            { type: 'divider' }
        ] : [];
        const extraMoreActionsRaw = Array.isArray(this.config.moreActions) ? this.config.moreActions : defaultMoreActions;
        // 避免有人在 page-configs.js 仍配置了 importFromExcel，這裡直接忽略（避免重複）
        const extraMoreActions = extraMoreActionsRaw.filter(a => !(a && a.id === 'importFromExcel'));
        const extraMoreActionsHtml = extraMoreActions.map((a) => {
            if (!a) return '';
            if (a.type === 'divider') {
                return `<li><hr class="dropdown-divider"></li>`;
            }
            const id = a.id || '';
            const label = a.labelKey ? this.getText(a.labelKey) : (a.label || id);
            const icon = a.icon ? `<i class="${a.icon}"></i> ` : '';
            const cls = a.className ? a.className : 'dropdown-item';
            // 目前只支援：呼叫 DynamicList.handleMoreAction(actionId, arg)
            const arg = (typeof a.arg === 'undefined') ? 'null' : JSON.stringify(a.arg);
            return `
                <li>
                    <a class="${cls}" href="#" onclick="window.dynamicList.handleMoreAction('${id}', ${arg}); return false;">
                        ${icon}${label}
                    </a>
                </li>
            `;
        }).join('');

        const importBtnHtml = canImport ? `
                <button class="btn btn-light" id="importExcelBtn" onclick="window.dynamicList.importFromExcel();" style="background: white; border: 1px solid #dee2e6;">
                    <i class="bi bi-upload"></i> ${this.getText('common.importExcel')}
                </button>
        ` : '';

        const viewTrashText = this.getText('common.viewTrash');
        const allowDeleteActions = this.config.enableDeleteActions !== false;
        const deleteActionsHtml = allowDeleteActions ? `
                    <li>
                        <a class="dropdown-item" href="#" onclick="window.dynamicList.toggleTrashMode(); return false;">
                            <i class="bi bi-trash3"></i> ${viewTrashText}
                        </a>
                    </li>
                    <li><hr class="dropdown-divider"></li>
                    <li>
                        <a class="dropdown-item text-danger" href="#" onclick="window.dynamicList.enableBulkDelete(); return false;">
                            <i class="bi bi-trash"></i> ${bulkDeleteText}
                        </a>
                    </li>
        ` : '';

        const moreActionsDropdown = `
            ${importBtnHtml}
            <div class="dropdown" style="position: relative;">
                <button class="btn btn-light dropdown-toggle" type="button" id="moreActionsBtn" data-bs-toggle="dropdown" aria-expanded="false" style="background: white; border: 1px solid #dee2e6;">
                    <i class="bi bi-three-dots-vertical"></i> ${moreActionsText}
                </button>
                <ul class="dropdown-menu" aria-labelledby="moreActionsBtn">
                    ${extraMoreActionsHtml}
                    ${deleteActionsHtml}
                </ul>
            </div>
            <button class="btn btn-danger" id="confirmDeleteBtn" style="display: none;" onclick="window.dynamicList.bulkDelete();">
                <i class="bi bi-trash"></i> ${confirmDeleteSelectedText} (<span id="selectedCount">0</span>)
            </button>
            <button class="btn btn-outline-secondary" id="cancelBulkDeleteBtn" style="display: none;" onclick="window.dynamicList.cancelBulkDelete();">
                <i class="bi bi-x-circle"></i> ${this.getText('common.cancel')}
            </button>
            <button class="btn btn-outline-secondary" id="exitTrashModeBtn" style="display: ${allowDeleteActions && this.trashMode ? 'inline-block' : 'none'};" onclick="window.dynamicList.toggleTrashMode();">
                <i class="bi bi-x-circle"></i> ${this.getText('common.cancelTrashMode')}
            </button>
        `;

        const labelFilterHtml = this.config.labelFilter ? `
            <div id="labelFilterContainer" class="d-flex flex-wrap align-items-center gap-2">
                <span class="text-muted small fw-semibold">${this.getText('common.labels')}</span>
                <div class="d-flex flex-wrap align-items-center gap-2" id="labelFilterBar"></div>
                <div class="dropdown">
                    <button class="btn btn-sm btn-light dropdown-toggle" type="button" id="labelFilterManageBtn" data-bs-toggle="dropdown" aria-expanded="false">
                        <span>${this.getText('common.modify')}</span>
                    </button>
                    <div class="dropdown-menu p-2" id="labelFilterDropdown" style="min-width: 220px; max-height: 260px; overflow: auto;"></div>
                </div>
            </div>
        ` : '';

        const toolbarRow = `
            <div class="d-flex justify-content-between align-items-start flex-wrap gap-3 mb-3">
                <div class="flex-grow-1">
                    ${labelFilterHtml}
                </div>
                <div class="d-flex align-items-center gap-2 ms-auto">
                    ${moreActionsDropdown}
                </div>
            </div>
        `;

        const table = `
            <div class="card">
                <div class="card-body">
                    ${filters}
                    <div id="alertContainer"></div>
                    ${toolbarRow}
                    <div class="table-responsive">
                        <table class="table table-hover">
                            <thead class="table-light">
                                <tr>
                                    <th id="checkboxHeader" style="width: 40px; display: none;">
                                        <input type="checkbox" id="selectAllCheckbox" onchange="window.dynamicList.toggleSelectAll(this.checked)">
                                    </th>
                                    ${tableHeaders}
                                    ${showActions ? `<th style="white-space: nowrap;" data-i18n="common.actions">${actionsText}</th>` : ''}
                                </tr>
                            </thead>
                            <tbody id="dataTable">
                                <tr><td colspan="${this.config.columns.length + 1 + (showActions ? 1 : 0)}" class="text-center" data-i18n="common.loading">${loadingText}</td></tr>
                            </tbody>
                        </table>
                    </div>
                    <div id="paginationContainer" class="mt-3"></div>
                </div>
            </div>
        `;

        container.innerHTML = header + table;
        
        // 添加搜索框清除按钮的显示/隐藏逻辑
        setTimeout(() => {
            const searchInput = document.getElementById('searchInput');
            const clearBtn = document.getElementById('clearSearchBtn');
            if (searchInput && clearBtn) {
                searchInput.addEventListener('input', function() {
                    clearBtn.style.display = this.value ? 'block' : 'none';
                });
            }
        }, 100);
        
        // 初始化有 relationApi 的過濾器 Select2
        this.initFilterSelect2();
    }
    
    // 初始化過濾器 Select2（對於有 relationApi 的過濾器）
    async initFilterSelect2() {
        if (!this.config.filters) return;
        
        for (const filter of this.config.filters) {
            if (filter.relationApi) {
                const selectId = `filter_${filter.key}`;
                const select = document.getElementById(selectId);
                if (!select) continue;
                
                // 如果已經初始化過 Select2，先銷毀
                if ($(select).hasClass('select2-hidden-accessible')) {
                    $(select).select2('destroy');
                }
                
                // 構建 API 路徑
                let apiPath = filter.relationApi;
                if (!apiPath.startsWith('/api/v1') && !apiPath.startsWith('http')) {
                    apiPath = `/api/v1${apiPath.startsWith('/') ? '' : '/'}${apiPath}`;
                }
                
                // 初始化 Select2
                $(select).select2({
                    theme: 'bootstrap-5',
                    width: '100%',
                    placeholder: (() => {
                        // 預設中文 fallback（避免 i18n 尚未載入時顯示 key）
                        let fallback = `請選擇${filter.label || ''}`;
                        if (typeof I18n === 'undefined' || !I18n.t) return fallback;

                        const selectKey = 'common.select';
                        const selectText = I18n.t(selectKey);
                        // 取欄位名稱：優先 filter.labelKey，其次 fields.<filter.key>，否則用 filter.label
                        let labelText = filter.label || '';
                        if (filter.labelKey) {
                            const t0 = I18n.t(filter.labelKey);
                            if (t0 && t0 !== filter.labelKey) labelText = t0;
                        }
                        if (labelText === (filter.label || '')) {
                            const fieldKey = `fields.${filter.key}`;
                            const fieldText = I18n.t(fieldKey);
                            if (fieldText && fieldText !== fieldKey) labelText = fieldText;
                        }

                        if (selectText && selectText !== selectKey) {
                            const combined = `${selectText} ${labelText}`.trim();
                            return combined || fallback;
                        }
                        return fallback;
                    })(),
                    allowClear: true,
                    ajax: {
                        url: apiPath,
                        dataType: 'json',
                        delay: 250,
                        headers: {
                            'Authorization': 'Bearer ' + (localStorage.getItem('auth_token') || ''),
                            'X-Tenant-Subdomain': localStorage.getItem('tenant_subdomain') || ''
                        },
                        data: function (params) {
                            return {
                                search: params.term || '',
                                page: params.page || 1,
                                limit: 50
                            };
                        },
                        processResults: function (data) {
                            const items = (data.data || data || []);
                            return {
                                results: items.map(item => ({
                                    id: item.id,
                                    text: item[filter.relationLabel || 'name'] || item.name || item.id
                                }))
                            };
                        },
                        cache: true
                    },
                    minimumInputLength: 0
                }).on('change', () => {
                    this.loadData();
                });
                
                // 如果有預設選項，先添加它們
                if (filter.options && filter.options.length > 0) {
                    filter.options.forEach(opt => {
                        if (opt.value !== undefined && opt.value !== null) {
                            const option = new Option(opt.label, opt.value, false, false);
                            $(select).append(option);
                        }
                    });
                    // 設置第一個選項為選中（通常是"全部"）
                    // 注意：不要立即觸發 change 事件，因為 dynamicList 可能還沒初始化
                    if (filter.options.length > 0 && filter.options[0].value === '') {
                        $(select).val('');
                        // 延遲觸發 change，確保 dynamicList 已初始化
                        setTimeout(() => {
                            if (window.dynamicList && typeof window.dynamicList.loadData === 'function') {
                                $(select).trigger('change');
                            }
                        }, 100);
                    }
                }
            }
        }
    }

    // 處理更多動作
    handleMoreAction(actionId, arg) {
        try {
            switch (actionId) {
                case 'downloadImportTemplate':
                    this.downloadImportTemplate(!!arg);
                    return;
                case 'importFromExcel':
                    this.importFromExcel();
                    return;
                default:
                    App.showAlert(`未知動作：${actionId}`, 'warning');
                    return;
            }
        } catch (e) {
            console.error('handleMoreAction error:', e);
            App.showAlert('執行動作失敗：' + (e?.message || e), 'danger');
        }
    }

    // 下載匯入模板（Excel）
    async downloadImportTemplate(withData) {
        const apiPath = this.config?.apiPath || '';
        if (!apiPath) {
            App.showAlert('頁面未設定 apiPath，無法下載模板', 'warning');
            return;
        }
        const pageName = (typeof this.getPageName === 'function') ? this.getPageName() : (apiPath || 'import');
        const filename = `${pageName}_import_template_${withData ? 'with_data' : 'blank'}.xlsx`;

        // 先嘗試後端（若該模組有專屬模板）
        try {
            const qs = `with_data=${withData ? 'true' : 'false'}`;
            const url = `/api/v1${apiPath}/import-template/excel?${qs}`;
            if (typeof downloadApiFile === 'function') {
                await downloadApiFile(url, filename);
                return;
            }
        } catch (e) {
            // 後端未支援則 fallback 到前端模板
            console.warn('Server import-template not available, fallback to client template:', e);
        }

        // 前端生成模板（適用所有 DynamicList）
        if (typeof ensureXlsxLib === 'function') {
            await ensureXlsxLib();
        }
        if (typeof XLSX === 'undefined') {
            App.showAlert('模板功能未載入（XLSX），請重新整理頁面', 'warning');
            return;
        }

        // Header 優先用 formFields（避免漏掉必填欄位），否則用 columns
        const fields = (this.config.formFields || []).filter(f => !!f && !!f.key);
        const cols = (this.config.columns || []).filter(c => !!c && !!c.key);
        const headerKeys = (fields.length ? fields.map(f => f.key) : cols.map(c => c.key));
        const header = Array.from(new Set(headerKeys)); // 去重

        let body = [];
        if (withData) {
            const { rows } = await _fetchAllDynamicListRows(this);
            // 對齊 header key
            body = rows.map(r => header.map(k => _formatExportValue(this, r, { key: k, type: 'text' })));
        }

        const aoa = [header, ...body];
        const ws = XLSX.utils.aoa_to_sheet(aoa);
        const wb = XLSX.utils.book_new();
        const sheetName = (this.config.title || 'Import').slice(0, 31);
        XLSX.utils.book_append_sheet(wb, ws, sheetName);
        XLSX.writeFile(wb, filename);
    }

    // 匯入 Excel（上傳 xlsx）
    async importFromExcel() {
        const apiPath = this.config?.apiPath || '';
        if (!apiPath) {
            App.showAlert('頁面未設定 apiPath，無法匯入', 'warning');
            return;
        }

        const input = document.createElement('input');
        input.type = 'file';
        input.accept = '.xlsx';
        input.style.display = 'none';
        document.body.appendChild(input);

        input.onchange = async () => {
            try {
                const file = input.files && input.files[0];
                if (!file) return;

                // 1) 先嘗試走後端專屬匯入（orders/service-orders/purchase-orders）
                try {
                    const formData = new FormData();
                    formData.append('file', file);
                    const url = `${apiPath}/import/excel`;
                    const resp = await App.apiRequest(url, { method: 'POST', body: formData });
                    const created = resp.created_orders || 0;
                    const updated = resp.updated_orders || 0;
                    const items = resp.updated_items || 0;
                    App.showAlert(`匯入完成：新增 ${created}，更新 ${updated}，明細 ${items}`, 'success');
                    if (resp.warnings && resp.warnings.length) {
                        console.warn('Import warnings:', resp.warnings);
                        App.showAlert(`匯入完成（有 ${resp.warnings.length} 個警告，請看 console）`, 'warning');
                    }
                    await this.loadData();
                    return;
                } catch (e) {
                    console.warn('Server import not available, fallback to client import:', e);
                }

                // 2) 通用匯入（前端解析 Excel -> 逐行呼叫既有 REST API）
                await ensureXlsxLib();
                if (typeof XLSX === 'undefined') {
                    App.showAlert('匯入功能未載入（XLSX），請重新整理頁面', 'warning');
                    return;
                }

                const buf = await file.arrayBuffer();
                const wb = XLSX.read(buf, { type: 'array' });
                const sheetName = wb.SheetNames?.[0];
                if (!sheetName) {
                    App.showAlert('Excel 沒有工作表', 'warning');
                    return;
                }
                const ws = wb.Sheets[sheetName];
                const aoa = XLSX.utils.sheet_to_json(ws, { header: 1, raw: false });
                if (!aoa || aoa.length < 2) {
                    App.showAlert('Excel 沒有資料列', 'warning');
                    return;
                }
                const header = (aoa[0] || []).map(h => String(h || '').trim()).filter(Boolean);
                if (header.length === 0) {
                    App.showAlert('Excel 第一行沒有欄位名稱', 'warning');
                    return;
                }

                // roles 匯入：admin 角色不能被清除/更改（或重建）
                const isRolesImport = apiPath === '/roles';
                const normalizeRoleName = (name) => String(name || '').trim().toLowerCase();
                const isAdminRoleName = (name) => normalizeRoleName(name) === 'admin';
                let existingRoleById = null;
                let existingRoleByName = null;
                if (isRolesImport) {
                    try {
                        const { rows } = await _fetchAllDynamicListRows(this);
                        existingRoleById = new Map();
                        existingRoleByName = new Map();
                        (rows || []).forEach(r => {
                            if (!r) return;
                            const id = (r.id !== undefined && r.id !== null) ? String(r.id) : '';
                            const nm = normalizeRoleName(r.name);
                            if (id) existingRoleById.set(id, r);
                            if (nm) existingRoleByName.set(nm, r);
                        });
                    } catch (e) {
                        console.warn('Failed to prefetch roles for import admin-protection:', e);
                        // 若無法預載 roles，仍可用 Excel 內的 name 做基本阻擋
                        existingRoleById = new Map();
                        existingRoleByName = new Map();
                    }
                }

                // 構建欄位定義（用於必填驗證與型別轉換）
                const fieldDefs = {};
                const formFields = Array.isArray(this.config?.formFields) ? this.config.formFields : [];
                for (const f of formFields) {
                    if (f && f.key) fieldDefs[f.key] = f;
                }
                const requiredKeys = formFields.filter(f => f && f.key && f.required).map(f => f.key);

                const parseBool = (v) => {
                    const s = String(v || '').trim().toLowerCase();
                    return s === '1' || s === 'true' || s === 'yes' || s === 'y' || s === 'on';
                };
                const parseNum = (v) => {
                    const s = String(v || '').trim().replace(/,/g, '');
                    if (!s) return null;
                    const n = Number(s);
                    return Number.isFinite(n) ? n : null;
                };
                const parseMulti = (v) => {
                    const s = String(v || '').trim();
                    if (!s) return [];
                    return s.split(',').map(x => x.trim()).filter(Boolean);
                };

                // 預驗證：缺必填 / 觸發保護規則就 alert（不送 API）
                const validationErrors = [];
                const adminViolations = [];
                const rowsToImport = [];

                for (let r = 1; r < aoa.length; r++) {
                    const row = aoa[r] || [];
                    const obj = {};
                    let allEmpty = true;
                    for (let c = 0; c < header.length; c++) {
                        const key = header[c];
                        const v = (row[c] === undefined || row[c] === null) ? '' : String(row[c]).trim();
                        if (v !== '') allEmpty = false;
                        const def = fieldDefs[key];
                        if (def && def.type) {
                            switch (def.type) {
                                case 'number': {
                                    const n = parseNum(v);
                                    obj[key] = (n === null) ? '' : n;
                                    break;
                                }
                                case 'checkbox':
                                case 'boolean': {
                                    obj[key] = parseBool(v);
                                    break;
                                }
                                case 'select2-multi': {
                                    obj[key] = parseMulti(v);
                                    break;
                                }
                                default:
                                    obj[key] = v;
                            }
                        } else {
                            obj[key] = v;
                        }
                    }
                    if (allEmpty) continue;

                    // id 有值 -> PUT；否則 POST
                    const idVal = (obj.id || '').trim();

                    // roles：admin 不能被匯入更新/清除，也不能建立名為 admin 的角色
                    if (isRolesImport) {
                        const nameVal = normalizeRoleName(obj.name);
                        const existing = idVal ? existingRoleById?.get(idVal) : (nameVal ? existingRoleByName?.get(nameVal) : null);
                        const wouldTouchAdmin = (existing && isAdminRoleName(existing.name)) || isAdminRoleName(nameVal);
                        if (wouldTouchAdmin) {
                            adminViolations.push({ row: r + 1, id: idVal || '', name: obj.name || '' });
                        }
                    }

                    // 必填驗證（若 requiredKeys 有定義）
                    if (requiredKeys.length) {
                        for (const k of requiredKeys) {
                            const val = obj[k];
                            const isEmpty = (val === undefined || val === null || val === '' || (Array.isArray(val) && val.length === 0));
                            if (isEmpty) {
                                validationErrors.push({ row: r + 1, field: k });
                            }
                        }
                    }
                    rowsToImport.push({ rowNum: r + 1, idVal, obj });
                }

                if (validationErrors.length) {
                    const top = validationErrors.slice(0, 10);
                    console.warn('Import validation errors:', validationErrors);
                    App.showAlert(`Excel 必填欄位未填（共 ${validationErrors.length} 個）。例如：${top.map(e => `第${e.row}行缺 ${e.field}`).join('；')}`, 'warning');
                    return;
                }

                if (adminViolations.length) {
                    const top = adminViolations.slice(0, 10);
                    console.warn('Import blocked: admin role violation:', adminViolations);
                    App.showAlert(`匯入已中止：admin 角色不能被清除或更改。請移除/修改 Excel 中相關資料列，例如：${top.map(v => `第${v.row}行`).join('、')}`, 'warning');
                    return;
                }

                let created = 0;
                let updated = 0;
                for (const it of rowsToImport) {
                    if (it.idVal) {
                        await App.apiRequest(`${apiPath}/${encodeURIComponent(it.idVal)}`, { method: 'PUT', body: JSON.stringify(it.obj) });
                        updated++;
                    } else {
                        await App.apiRequest(`${apiPath}`, { method: 'POST', body: JSON.stringify(it.obj) });
                        created++;
                    }
                }

                App.showAlert(`匯入完成：新增 ${created}，更新 ${updated}`, 'success');
                await this.loadData();
            } catch (e) {
                console.error('Import excel error:', e);
                App.showAlert('匯入失敗：' + (e?.message || e), 'danger');
            } finally {
                input.remove();
            }
        };

        input.click();
    }

    getPageName() {
        if (this.config.pageName) return this.config.pageName;
        const apiPath = this.config.apiPath || '';
        const pageName = apiPath.replace(/^\/api\/v1\//, '').replace(/^\//, '');
        // 處理連字符，將 member-levels 轉換為 member-levels（保持原樣）
        return pageName;
    }
    
    // 將頁面名稱轉換為翻譯鍵（將連字符轉為駝峰命名）
    getTranslationKey() {
        const pageName = this.getPageName();
        // 將 member-levels 轉為 memberLevels, expense-requests 轉為 expenseRequests
        return pageName.replace(/-([a-z])/g, (match, letter) => letter.toUpperCase());
    }

    bindEvents() {
        // 事件綁定已在 HTML 中完成
        // payrolls 自動生成
        const genBtn = document.getElementById('generatePayrollsBtn');
        if (genBtn) {
            genBtn.addEventListener('click', async () => {
                genBtn.disabled = true;
                genBtn.textContent = this.getText('common.generating');
                try {
                    const result = await App.apiRequest('/payrolls/generate-current-month', { method: 'POST' });
                    let message = `已生成 ${result.created || 0} 條薪資記錄`;
                    if (result.with_salary !== undefined) {
                        message += `<br>有薪資員工：${result.with_salary} 人`;
                    }
                    if (result.monthly !== undefined) {
                        message += `<br>月薪：${result.monthly} 人`;
                    }
                    if (result.hourly !== undefined) {
                        message += `<br>時薪：${result.hourly} 人`;
                    }
                    App.showAlert(message, 'success');
                    this.loadData();
                } catch (err) {
                    console.error(err);
                    App.showAlert(this.getText('common.generateError') + ': ' + (err.message || err), 'danger');
                } finally {
                    genBtn.disabled = false;
                    genBtn.textContent = this.getText('common.autoGenerateMonthlyPayrolls');
                }
            });
        }
    }

    async initLabelFilter() {
        if (!this.config.labelFilter) return;
        const container = document.getElementById('labelFilterContainer');
        if (!container) return;

        const apiPath = this.normalizeApiPath(this.config.labelFilter.apiPath || this.config.labelFilter.api || '');
        if (!apiPath) return;

        try {
            const res = await App.apiRequest(`${apiPath}?page=1&limit=200`);
            this.labelOptions = Array.isArray(res?.data) ? res.data : [];
        } catch (e) {
            console.warn('load labels failed', e);
            this.labelOptions = [];
        }

        this.renderLabelFilter();

        const bar = document.getElementById('labelFilterBar');
        if (bar) {
            bar.addEventListener('click', (e) => {
                const btn = e.target.closest('[data-label-id]');
                if (!btn) return;
                const labelId = btn.getAttribute('data-label-id');
                if (!labelId) return;
                this.toggleLabelSelection(labelId);
            });
        }

        const dropdown = document.getElementById('labelFilterDropdown');
        if (dropdown) {
            dropdown.addEventListener('change', (e) => {
                const input = e.target;
                if (!input || input.type !== 'checkbox') return;
                const labelId = input.getAttribute('data-label-id');
                if (!labelId) return;
                if (input.checked) {
                    this.selectedLabelIds.add(labelId);
                } else {
                    this.selectedLabelIds.delete(labelId);
                }
                this.applyLabelFilter();
            });

            dropdown.addEventListener('click', (e) => {
                const btn = e.target.closest('[data-action="clear-labels"]');
                if (!btn) return;
                e.preventDefault();
                this.selectedLabelIds.clear();
                this.applyLabelFilter();
            });
        }
    }

    normalizeApiPath(apiPath) {
        if (!apiPath) return '';
        if (apiPath.startsWith('http')) return apiPath;
        if (apiPath.startsWith('/api/v1')) return apiPath;
        return `/api/v1${apiPath.startsWith('/') ? '' : '/'}${apiPath}`;
    }

    renderLabelFilter() {
        const bar = document.getElementById('labelFilterBar');
        const dropdown = document.getElementById('labelFilterDropdown');
        if (!bar || !dropdown) return;

        if (!this.labelOptions || this.labelOptions.length === 0) {
            bar.innerHTML = '<span class="text-muted small">' + this.getText('common.noLabels') + '</span>';
            dropdown.innerHTML = '<div class="text-muted small px-2">' + this.getText('common.noLabels') + '</div>';
            return;
        }

        const showLimit = (this.config.labelFilter && this.config.labelFilter.defaultShow) ? this.config.labelFilter.defaultShow : 4;
        const selectedIds = Array.from(this.selectedLabelIds);
        const selectedLabels = this.labelOptions.filter(l => this.selectedLabelIds.has(String(l.id)));
        const displayLabels = selectedLabels.length > 0 ? selectedLabels.slice(0, showLimit) : this.labelOptions.slice(0, showLimit);

        const chips = displayLabels.map(label => {
            const id = String(label.id || '');
            const name = String(label.name || '');
            const color = (label.color || '#64748b').toString();
            const active = this.selectedLabelIds.has(id);
            const cls = active ? 'cms-label-chip active' : 'cms-label-chip';
            return `<button type="button" class="${cls}" data-label-id="${id}" style="--label-color: ${color};">${name}</button>`;
        }).join('');

        const extraCount = selectedIds.length > showLimit ? `<span class="badge text-bg-light border">+${selectedIds.length - showLimit}</span>` : '';
        bar.innerHTML = chips + extraCount;

        const dropdownItems = this.labelOptions.map(label => {
            const id = String(label.id || '');
            const name = String(label.name || '');
            const color = (label.color || '#64748b').toString();
            const checked = this.selectedLabelIds.has(id) ? 'checked' : '';
            return `
                <div class="form-check mb-1">
                    <input class="form-check-input" type="checkbox" ${checked} data-label-id="${id}" id="label-filter-${id}">
                    <label class="form-check-label d-flex align-items-center gap-2" for="label-filter-${id}">
                        <span class="cms-label-dot" style="--label-color: ${color};"></span>
                        <span>${name}</span>
                    </label>
                </div>
            `;
        }).join('');

        dropdown.innerHTML = `
            <div class="d-flex align-items-center justify-content-between mb-2">
                <span class="text-muted small">${this.getText('common.selectLabels')}</span>
                <button class="btn btn-link btn-sm text-decoration-none" data-action="clear-labels">${this.getText('common.clear')}</button>
            </div>
            ${dropdownItems}
        `;
    }

    toggleLabelSelection(labelId) {
        if (this.selectedLabelIds.has(labelId)) {
            this.selectedLabelIds.delete(labelId);
        } else {
            this.selectedLabelIds.add(labelId);
        }
        this.applyLabelFilter();
    }

    applyLabelFilter() {
        this.renderLabelFilter();
        this.currentPage = 1;
        this.loadData();
    }

    handleSearchKey(event) {
        if (!event) return;
        if (event.key === 'Enter') {
            event.preventDefault();
            this.submitSearch();
        }
    }

    handleFilterKey(event) {
        if (!event) return;
        if (event.key === 'Enter') {
            event.preventDefault();
            this.loadData();
        }
    }

    submitSearch() {
        this.currentPage = 1;
        this.loadData();
    }

    // 獲取垃圾筒資源名稱（從 apiPath 轉換）
    getTrashResourceName() {
        const apiPath = this.config.apiPath || '';
        // /orders -> orders
        // /api/v1/orders -> orders
        // /service-orders -> service-orders
        // 先嘗試匹配 /api/v1/xxx 格式
        let match = apiPath.match(/\/api\/v1\/([^/?]+)/);
        if (match) return match[1];
        // 否則直接取最後一段路徑
        match = apiPath.match(/\/([^/?]+)$/);
        return match ? match[1] : 'items';
    }

    // 切換垃圾筒模式
    toggleTrashMode() {
        this.trashMode = !this.trashMode;
        this.currentPage = 1;
        this.selectedItems.clear();
        
        // 更新 UI
        const exitBtn = document.getElementById('exitTrashModeBtn');
        const labelFilter = document.getElementById('labelFilterContainer');
        const filtersContainer = document.getElementById('filtersContainer');
        
        if (this.trashMode) {
            if (exitBtn) exitBtn.style.display = 'inline-block';
            if (labelFilter) labelFilter.style.display = 'none';
            if (filtersContainer) filtersContainer.style.display = 'none';
        } else {
            if (exitBtn) exitBtn.style.display = 'none';
            if (labelFilter) labelFilter.style.display = '';
            if (filtersContainer) filtersContainer.style.display = '';
        }
        
        this.loadData();
    }

    async loadData() {
        try {
            this.loadingStart = Date.now();
            this.showLoadingOverlay();
            const search = document.getElementById('searchInput')?.value || '';
            
            // 垃圾筒模式使用不同的 API
            let url;
            if (this.trashMode) {
                const resourceName = this.getTrashResourceName();
                url = `/api/v1/trash/${resourceName}?page=${this.currentPage}&limit=20`;
            } else {
                url = `${this.config.apiPath}?page=${this.currentPage}&limit=20`;
            }
            
            if (search) {
                url += `&search=${encodeURIComponent(search)}`;
            }

            // 添加標籤過濾（多選）- 僅在非垃圾筒模式下
            if (!this.trashMode && this.config.labelFilter) {
                const labelKey = this.config.labelFilter.paramKey || 'label_id';
                const labelIds = Array.from(this.selectedLabelIds);
                if (labelIds.length > 0) {
                    url += `&${labelKey}=${encodeURIComponent(labelIds.join(','))}`;
                }
            }

            // 添加過濾器 - 僅在非垃圾筒模式下
            if (!this.trashMode && this.config.filters) {
                const labelFilterKey = this.config.labelFilter ? (this.config.labelFilter.paramKey || 'label_id') : '';
                this.config.filters.forEach(filter => {
                    if (labelFilterKey && filter.key === labelFilterKey) return;
                    const filterSelect = document.getElementById(`filter_${filter.key}`);
                    if (!filterSelect) return;
                    
                    // 如果是 Select2，使用 jQuery 獲取值；否則使用原生 value
                    let filterValue = '';
                    if (filter.relationApi && typeof $ !== 'undefined' && $(filterSelect).hasClass('select2-hidden-accessible')) {
                        filterValue = $(filterSelect).val() || '';
                    } else {
                        filterValue = filterSelect.value || '';
                    }
                    
                    if (filterValue) {
                        url += `&${filter.key}=${encodeURIComponent(filterValue)}`;
                    }
                });
            }

            // Pass through URL _code params to API (e.g. service_type_code=course)
            // so backend can filter even before the select2 preset resolves.
            // Skip if the corresponding _id filter already has a value.
            if (!this.trashMode) {
                try {
                    const urlParams = new URLSearchParams(window.location.search || '');
                    urlParams.forEach((val, key) => {
                        if (key.endsWith('_code') && val && !url.includes(`&${key}=`)) {
                            // Check if the corresponding _id filter already has a value in the URL
                            const idKey = key.replace(/_code$/, '_id');
                            if (!url.includes(`&${idKey}=`)) {
                                url += `&${key}=${encodeURIComponent(val)}`;
                            }
                        }
                    });
                } catch (_) {}
            }

            if (typeof App === 'undefined') {
                throw new Error('App 未載入');
            }
            const data = await App.apiRequest(url);
            // 調試：檢查數據是否包含 is_default
            if (data.data && data.data.length > 0) {
                const pageName = this.getPageName();
                if (pageName === 'warehouses' || pageName === 'logistics-companies' || pageName === 'currencies') {
                    console.log(`${pageName} list data sample:`, data.data[0]);
                    console.log(`First ${pageName} is_default:`, data.data[0].is_default, typeof data.data[0].is_default);
                }
            }
            // 更新分页信息
            this.totalItems = data.total || 0;
            this.totalPages = Math.max(1, Math.ceil(this.totalItems / this.pageSize));
            this.currentPage = data.page || 1;
            this.renderTable(data.data || []);
            this.renderPagination();
            this.updateExportButtons();
        } catch (error) {
            // 垃圾筒模式遇到 Invalid resource type 錯誤時，自動退出並提示
            if (this.trashMode && error.message && error.message.includes('Invalid resource type')) {
                this.trashMode = false;
                const exitBtn = document.getElementById('exitTrashModeBtn');
                if (exitBtn) exitBtn.style.display = 'none';
                const labelFilter = document.getElementById('labelFilterContainer');
                const filtersContainer = document.getElementById('filtersContainer');
                if (labelFilter) labelFilter.style.display = '';
                if (filtersContainer) filtersContainer.style.display = '';
                App.showAlert(this.getText('common.trashNotSupported'), 'warning');
                this.loadData();
                return;
            }
            const tbody = document.getElementById('dataTable');
            if (tbody) {
                const showActions = this.config.showActions !== false;
                const colSpan = this.config.columns.length + 1 + (showActions ? 1 : 0);
                const loadFailed = this.getText('common.loadError');
                tbody.innerHTML = `<tr><td colspan="${colSpan}" class="text-center text-danger">${loadFailed}: ${error.message}</td></tr>`;
            }
            this.totalItems = 0;
            this.updateExportButtons();
        } finally {
            const elapsed = Date.now() - (this.loadingStart || Date.now());
            const delay = elapsed < 100 ? 100 - elapsed : 0;
            setTimeout(() => this.hideLoadingOverlay(), delay);
        }
    }

    updateExportButtons() {
        const excelBtn = document.getElementById('exportExcelBtn');
        const pdfBtn = document.getElementById('exportPdfBtn');
        // 不要因為沒資料而 disabled；點擊時會在 exportToExcel/exportToPDF 內提示
        if (excelBtn) excelBtn.disabled = false;
        if (pdfBtn) pdfBtn.disabled = false;
    }

    renderTable(items) {
        const tbody = document.getElementById('dataTable');
        if (!tbody) return;

        // 保存當前數據（供 inline actions/checkbox 更新用）
        this.data = Array.isArray(items) ? items : [];

        // 檢查是否顯示操作列
        const showActions = this.config.showActions !== false; // 默認顯示，除非明確設置為 false
        const colSpan = this.config.columns.length + 1 + (showActions ? 1 : 0); // +1 for checkbox column

        if (!this.data || this.data.length === 0) {
            const noDataText = this.getText('common.noData');
            tbody.innerHTML = `<tr><td colspan="${colSpan}" class="text-center empty-data-row" data-i18n="common.noData">${noDataText}</td></tr>`;
            this.updateExportButtons();
            return;
        }

        tbody.innerHTML = this.data.map(item => {
            const itemId = String(item.id || '');
            const isChecked = this.selectedItems.has(itemId);
            const safeItemId = itemId.replace(/'/g, "\\'").replace(/"/g, '&quot;');
            const checkboxCell = `<td class="checkbox-cell" style="display: none;"><input type="checkbox" class="item-checkbox" data-item-id="${safeItemId}" onchange="window.dynamicList.toggleItemSelection('${safeItemId}', this.checked)" ${isChecked ? 'checked' : ''}></td>`;
            
            const cells = this.config.columns.map(col => {
                return `<td>${this.renderCell(item, col)}</td>`;
            }).join('');

            // 轉義 item.id 以防止 XSS
            const editPath = this.config.editPath || '';

            // 特殊處理：頁面列表添加預覽按鈕
            let extraActions = '';
            if (this.config.apiPath === '/pages') {
                const slug = item.slug || '';
                const safeSlug = String(slug).replace(/'/g, "\\'").replace(/"/g, '&quot;');
                const tenantSubdomain = window.tenantSubdomain || localStorage.getItem('tenant_subdomain') || 'test';
                const previewUrl = `/co/${tenantSubdomain}/${safeSlug}/`;
                const previewText = this.getText('common.preview');
                extraActions = `
                    <a href="${previewUrl}" target="_blank" class="btn btn-sm btn-outline-info" title="${previewText}" data-i18n-title="common.preview">
                        <i class="bi bi-eye"></i> <span data-i18n="common.preview">${previewText}</span>
                    </a>
                `;
            }
            
            // 特殊處理：優惠券列表添加使用記錄按鈕
            if (this.config.apiPath === '/coupons') {
                extraActions = `
                    <button class="btn btn-sm btn-outline-info" onclick="window.dynamicList.viewCouponUsage('${safeItemId}')" title="查看使用記錄">
                        <i class="bi bi-list-ul"></i> 使用記錄
                    </button>
                `;
            }

            // 特殊處理：餐桌管理加入點餐連結
            if (this.config.apiPath === '/dining-tables') {
                const tenantSubdomain = window.tenantSubdomain || localStorage.getItem('tenant_subdomain') || 'test';
                const tableId = encodeURIComponent(item.id || '');
                const tableCode = encodeURIComponent(item.code || '');
                const storeId = encodeURIComponent(item.store_id || '');
                const qs = new URLSearchParams();
                if (tableId) qs.set('table_id', tableId);
                if (tableCode) qs.set('table_code', tableCode);
                if (storeId) qs.set('store_id', storeId);
                const orderUrl = `/co/${tenantSubdomain}/dining/order/` + (qs.toString() ? `?${qs.toString()}` : '');
                extraActions = `
                    <a href="${orderUrl}" target="_blank" class="btn btn-sm btn-outline-primary" title="點餐連結">
                        <i class="bi bi-qr-code"></i> 點餐連結
                    </a>
                `;
            }

            // 候位排隊：不提供候位連結
            
            // 特殊處理：預約列表添加取消和完成按鈕
            if (this.config.apiPath === '/appointments') {
                const status = item.status || '';
                // 轉義 status 以防止 XSS
                const safeStatus = String(status).replace(/'/g, "\\'").replace(/"/g, '&quot;');
                if (safeStatus !== 'cancelled' && safeStatus !== 'completed') {
                    extraActions += `
                        <button class="btn btn-sm btn-outline-warning" onclick="window.dynamicList.updateAppointmentStatus('${safeItemId}', 'cancelled')" title="${this.getText('common.markAsCancelled')}" data-i18n="common.markAsCancelled">
                            <i class="bi bi-x-circle"></i> <span data-i18n="common.cancelled">${this.getText('common.cancelled')}</span>
                        </button>
                        <button class="btn btn-sm btn-outline-success" onclick="window.dynamicList.updateAppointmentStatus('${safeItemId}', 'completed')" title="${this.getText('common.markAsCompleted')}" data-i18n="common.markAsCompleted">
                            <i class="bi bi-check-circle"></i> <span data-i18n="common.completed">${this.getText('common.completed')}</span>
                        </button>
                    `;
                }
            }

            // 支出申請：只允許批核/拒絕，無編輯
            if (this.config.apiPath === '/expense-requests') {
                extraActions = `
                    <button class="btn btn-sm btn-outline-success" onclick="window.dynamicList.approveExpenseRequest('${safeItemId}')" ${item.status !== 'pending' ? 'disabled' : ''} data-i18n="common.approve">
                        <span data-i18n="common.approve">${this.getText('common.approve')}</span>
                    </button>
                    <button class="btn btn-sm btn-outline-danger" onclick="window.dynamicList.rejectExpenseRequest('${safeItemId}')" ${item.status !== 'pending' ? 'disabled' : ''} data-i18n="common.reject">
                        <span data-i18n="common.reject">${this.getText('common.reject')}</span>
                    </button>
                `;
            }
            
            // 訂單管理：報價單顯示「轉成訂單」按鈕
            if (this.config.apiPath === '/orders' && item.status === 'quotation') {
                extraActions += `
                    <button class="btn btn-sm btn-outline-success" onclick="window.dynamicList.convertQuotationToOrder('${safeItemId}')" title="轉成訂單">
                        <i class="bi bi-arrow-repeat"></i> 轉成訂單
                    </button>
                `;
            }
            
            // 報價單管理：所有報價單都顯示「轉成訂單」按鈕
            if (this.config.apiPath === '/quotations') {
                extraActions += `
                    <button class="btn btn-sm btn-outline-success" onclick="window.dynamicList.convertQuotationToOrder('${safeItemId}')" title="轉成訂單">
                        <i class="bi bi-arrow-repeat"></i> 轉成訂單
                    </button>
                `;
            }

            // 訂單管理：非已完成/已取消的訂單顯示「生成結帳連結」按鈕
            if (this.config.apiPath === '/orders' && item.status !== 'completed' && item.status !== 'cancelled') {
                extraActions += `
                    <button class="btn btn-sm btn-outline-info" onclick="window.dynamicList.generatePaymentLink('${safeItemId}')" title="生成結帳連結">
                        <i class="bi bi-link-45deg"></i> 結帳連結
                    </button>
                `;
            }
            
            // 處理自定義操作（從配置中讀取）
            if (this.config.customActions && Array.isArray(this.config.customActions)) {
                this.config.customActions.forEach(action => {
                    if (action.condition && typeof action.condition === 'function') {
                        if (action.condition(item)) {
                            extraActions += `
                                <button class="btn btn-sm ${action.className || 'btn-outline-primary'}" onclick="window.dynamicList.handleCustomAction('${action.action}', '${safeItemId}')" title="${action.label || ''}">
                                    ${action.icon ? `<i class="bi ${action.icon}"></i> ` : ''}${action.label || ''}
                                </button>
                            `;
                        }
                    } else {
                        extraActions += `
                            <button class="btn btn-sm ${action.className || 'btn-outline-primary'}" onclick="window.dynamicList.handleCustomAction('${action.action}', '${safeItemId}')" title="${action.label || ''}">
                                ${action.icon ? `<i class="bi ${action.icon}"></i> ` : ''}${action.label || ''}
                            </button>
                        `;
                    }
                });
            }
            
            // 檢查是否顯示操作列
            const showActions = this.config.showActions !== false; // 默認顯示，除非明確設置為 false
            
            // 垃圾筒模式下的操作按鈕
            let actionsCell = '';
            if (showActions) {
                const allowDeleteActions = this.config.enableDeleteActions !== false;
                if (this.trashMode) {
                    // 垃圾筒模式：只顯示還原和永久刪除
                    actionsCell = allowDeleteActions ? `
                        <td class="actions-cell" style="white-space: nowrap;">
                            <button class="btn btn-sm btn-outline-success" onclick="window.dynamicList.restoreItem('${safeItemId}')" title="${this.getText('common.restore')}">
                                <i class="bi bi-arrow-counterclockwise"></i> <span>${this.getText('common.restore')}</span>
                            </button>
                            <button class="btn btn-sm btn-outline-danger" onclick="window.dynamicList.permanentDeleteItem('${safeItemId}')" title="${this.getText('common.permanentDelete')}">
                                <i class="bi bi-x-circle"></i> <span>${this.getText('common.permanentDelete')}</span>
                            </button>
                        </td>
                    ` : '';
                } else {
                    // 正常模式：編輯和刪除
                    actionsCell = `
                        <td class="actions-cell" style="white-space: nowrap;">
                            ${this.config.apiPath === '/expense-requests' ? '' : `
                                <a href="${editPath}/${safeItemId}/edit" class="btn btn-sm btn-outline-primary" data-i18n="common.edit">
                                    <i class="bi bi-pencil"></i> <span data-i18n="common.edit">${this.getText('common.edit')}</span>
                                </a>
                                ${(!allowDeleteActions || (this.config.apiPath === '/roles' && (item.name === 'admin' || item.name === 'Admin' || item.name === 'ADMIN'))) ? '' : `
                                <button class="btn btn-sm btn-outline-danger" onclick="window.dynamicList.deleteItem('${safeItemId}')" data-i18n="common.delete">
                                    <i class="bi bi-trash"></i> <span data-i18n="common.delete">${this.getText('common.delete')}</span>
                                </button>
                                `}
                            `}
                            ${extraActions}
                        </td>
                    `;
                }
            }
            
            return `
                <tr>
                    ${checkboxCell}
                    ${cells}
                    ${actionsCell}
                </tr>
            `;
        }).join('');
        
        // 自動計算操作列寬度
        if (showActions) {
            this.calculateActionColumnWidth();
        }
        
        // 更新批量刪除按鈕和全選checkbox狀態
        this.updateBulkDeleteButton();
        this.updateSelectAllCheckbox();
    }

    showLoadingOverlay() {
        const mainContent = document.querySelector('.main-content');
        if (!mainContent) {
            setTimeout(() => this.showLoadingOverlay(), 50);
            return;
        }
        let overlay = document.getElementById('listLoadingOverlay');
        if (overlay) {
            overlay.style.display = 'flex';
            return;
        }
        const originalPosition = window.getComputedStyle(mainContent).position;
        if (originalPosition === 'static') {
            mainContent.style.position = 'relative';
        }
        overlay = document.createElement('div');
        overlay.id = 'listLoadingOverlay';
        overlay.className = 'form-loading-overlay';

        const loadingText = (typeof I18n !== 'undefined' && I18n.t && I18n.t('common.loading') !== 'common.loading')
            ? I18n.t('common.loading')
            : '載入中...';
        const loadingDataText = (typeof I18n !== 'undefined' && I18n.t && I18n.t('common.loadingData') !== 'common.loadingData')
            ? I18n.t('common.loadingData')
            : '正在載入數據...';
        overlay.innerHTML = `
            <div class="loading-content">
                <div class="spinner-border text-primary" role="status" style="width: 3rem; height: 3rem;">
                    <span class="visually-hidden">${loadingText}</span>
                </div>
                <p class="mt-3 text-muted">${loadingDataText}</p>
            </div>`;
        mainContent.appendChild(overlay);
    }

    hideLoadingOverlay() {
        const overlay = document.getElementById('listLoadingOverlay');
        if (overlay) {
            overlay.style.display = 'none';
        }
    }
    
    calculateActionColumnWidth() {
        // 等待 DOM 更新完成
        setTimeout(() => {
            const actionCells = document.querySelectorAll('#dynamicListContainer .actions-cell');
            if (actionCells.length === 0) {
                // 如果找不到，再試一次
                setTimeout(() => this.calculateActionColumnWidth(), 200);
                return;
            }
            
            let maxWidth = 0;
            
            // 遍歷所有操作單元格，找到最寬的一個
            actionCells.forEach(cell => {
                // 獲取該單元格內的所有按鈕（包括 a 標籤和 button 標籤）
                const buttons = cell.querySelectorAll('button, a.btn');
                
                if (buttons.length === 0) return;
                
                // 計算所有按鈕的寬度總和
                let totalButtonWidth = 0;
                buttons.forEach(btn => {
                    // 強制重新計算佈局以獲取實際寬度
                    void btn.offsetWidth;
                    totalButtonWidth += btn.offsetWidth || btn.scrollWidth || 0;
                });
                
                // 按鈕數量 * 10px（按鈕之間的間距）
                const buttonSpacing = buttons.length * 10;
                
                // 總寬度 = 按鈕寬度總和 + 按鈕間距
                const cellWidth = totalButtonWidth + buttonSpacing;
                
                if (cellWidth > maxWidth) {
                    maxWidth = cellWidth;
                }
            });
            
            // 使用計算出的寬度（不設置最小寬度限制）
            const calculatedWidth = maxWidth || 0; // 如果沒有按鈕，寬度為 0
            
            // 設置表頭操作列的寬度
            const table = document.querySelector('#dynamicListContainer table');
            if (table) {
                const thead = table.querySelector('thead');
                if (thead) {
                    const actionHeader = thead.querySelector('th:last-child');
                    if (actionHeader) {
                        // 檢查是否是操作列（通過文本或位置判斷）
                        const headerText = actionHeader.textContent.trim();
                        const actionsText = this.getText('common.actions');
                        if (headerText === actionsText || actionHeader === thead.querySelector('th:last-child')) {
                            actionHeader.style.width = calculatedWidth + 'px';
                            actionHeader.style.maxWidth = calculatedWidth + 'px';
                            
                            // 同時設置所有操作單元格的寬度（不設置 minWidth，允許自動調整）
                            actionCells.forEach(cell => {
                                cell.style.width = calculatedWidth + 'px';
                            });
                        }
                    }
                }
            }
        }, 150);
    }

    renderCell(item, col) {
        let value = this.getNestedValue(item, col.key);
        const pageName = this.getPageName();

        // phone-country-codes：name 顯示依語系翻譯（存值仍為英文）
        if (pageName === 'phone-country-codes' && col && col.key === 'name') {
            try {
                const code = this.getNestedValue(item, 'code') || item.code || '';
                if (code && typeof I18n !== 'undefined' && I18n.t) {
                    const k = `phoneCountryCodes.names.${code}`;
                    const t = I18n.t(k);
                    if (t && t !== k) value = t;
                }
            } catch {}
        }

        switch (col.type) {
            case 'text-with-avatar':
                // 在名稱前顯示頭像
                let profilePic = this.getNestedValue(item, 'profile_pic') || '';
                const nameValue = value || '';
                let avatarHtml = '';
                
                // 確保 URL 是完整的路徑
                if (profilePic && !profilePic.startsWith('http') && !profilePic.startsWith('/')) {
                    profilePic = '/' + profilePic;
                }
                
                if (profilePic) {
                    // 確保 URL 是完整的路徑
                    let fullProfileUrl = profilePic;
                    if (!fullProfileUrl.startsWith('http') && !fullProfileUrl.startsWith('/')) {
                        fullProfileUrl = '/' + fullProfileUrl;
                    }
                    // 轉義 URL 以防止 XSS
                    const safeProfileUrl = fullProfileUrl.replace(/'/g, "\\'").replace(/"/g, '&quot;');
                    avatarHtml = `<img src="${fullProfileUrl}" alt="頭像" style="width: 32px; height: 32px; border-radius: 50%; object-fit: cover; border: 2px solid #dee2e6; margin-right: 8px; cursor: pointer;" onclick="window.dynamicList.showImageLightbox('${safeProfileUrl}')" onerror="this.style.display='none'; this.setAttribute('data-error-handled','true'); if(this.nextElementSibling) this.nextElementSibling.style.display='flex'; this.onerror=null;">
                        <div class="avatar-circle" style="width: 32px; height: 32px; border-radius: 50%; border: 2px solid #dee2e6; background: transparent; color: #6c757d; display: none; align-items: center; justify-content: center; margin-right: 8px; flex-shrink: 0;"><i class="bi bi-person"></i></div>`;
                } else {
                    avatarHtml = `<div class="avatar-circle" style="width: 32px; height: 32px; border-radius: 50%; border: 2px solid #dee2e6; background: transparent; color: #6c757d; display: inline-flex; align-items: center; justify-content: center; margin-right: 8px; flex-shrink: 0;"><i class="bi bi-person"></i></div>`;
                }
                return `<div class="d-flex align-items-center">${avatarHtml}<span>${nameValue}</span></div>`;
            
            case 'profile-image':
                const profileUrl = value || '';
                if (profileUrl) {
                    // 確保 URL 是完整的路徑
                    let fullProfileUrl = profileUrl;
                    if (!fullProfileUrl.startsWith('http') && !fullProfileUrl.startsWith('/')) {
                        fullProfileUrl = '/' + fullProfileUrl;
                    }
                    // 轉義 URL 以防止 XSS
                    const safeProfileUrl = fullProfileUrl.replace(/'/g, "\\'").replace(/"/g, '&quot;');
                    return `<img src="${fullProfileUrl}" alt="頭像" style="width: 40px; height: 40px; border-radius: 50%; object-fit: cover; border: 2px solid #dee2e6; cursor: pointer;" onclick="window.dynamicList.showImageLightbox('${safeProfileUrl}')" onerror="this.style.display='none'; this.setAttribute('data-error-handled','true'); if(this.nextElementSibling) this.nextElementSibling.style.display='flex'; this.onerror=null;">
                        <div class="avatar-circle" style="width: 40px; height: 40px; border-radius: 50%; border: 2px solid #dee2e6; background: transparent; color: #6c757d; display: none; align-items: center; justify-content: center;"><i class="bi bi-person"></i></div>`;
                } else {
                    return `<div class="avatar-circle" style="width: 40px; height: 40px; border-radius: 50%; border: 2px solid #dee2e6; background: transparent; color: #6c757d; display: flex; align-items: center; justify-content: center;"><i class="bi bi-person"></i></div>`;
                }
            
            case 'image':
                let imageUrl = value || col.default || '/static/product.jpg';
                // 確保 URL 是完整的路徑
                if (imageUrl && !imageUrl.startsWith('http') && !imageUrl.startsWith('/')) {
                    imageUrl = '/' + imageUrl;
                }
                return `<img src="${imageUrl}" alt="${col.key}" class="product-image-thumb" style="width: 60px; height: 60px; object-fit: cover; cursor: pointer; border: 1px solid #e5e7eb;" onclick="window.dynamicList.showImageLightbox('${imageUrl}')">`;
            
            case 'badge':
                // 特殊處理：currencies、member-levels、shipping-methods、payment-methods、phone-country-codes、warehouses、logistics-companies、pages：
                // - is_default / is_homepage 顯示為可點擊的單選框
                // - payment-methods 額外支援 is_default_expense 也用單選框
                // 以及 bank-accounts 頁面的 is_default_receiving 和 is_default_payment 字段
                const pageName = this.getPageName();
                if ((pageName === 'currencies' || pageName === 'member-levels' || pageName === 'shipping-methods' || pageName === 'payment-methods' || pageName === 'phone-country-codes' || pageName === 'warehouses' || pageName === 'logistics-companies' || pageName === 'pages' || pageName === 'shifts') && (col.key === 'is_default' || col.key === 'is_homepage' || (pageName === 'payment-methods' && col.key === 'is_default_expense'))) {
                    // 獲取 item ID，API 返回的是 id (小寫)
                    const itemId = item.id || item.ID || '';
                    if (!itemId) {
                        return '<span class="text-muted">-</span>';
                    }
                    // 檢查值，支持多種格式：true, 'true', 1, '1'
                    const fieldValue = item[col.key] !== undefined ? item[col.key] : (value !== undefined && value !== null ? value : false);
                    const isChecked = fieldValue === true || fieldValue === 'true' || fieldValue === 1 || fieldValue === '1' || String(fieldValue).toLowerCase() === 'true';
                    console.log('Radio button render:', {
                        itemId, 
                        value: value, 
                        field: col.key,
                        fieldValue,
                        isChecked, 
                        'item keys': Object.keys(item)
                    });
                    // 轉義 ID 以防止 XSS
                    const safeId = String(itemId).replace(/'/g, "\\'").replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    // 根據頁面名稱設置不同的 radio name 和處理函數
                    let radioName, dataAttrName, radioId;
                    if (pageName === 'currencies') {
                        radioName = 'default_currency';
                        dataAttrName = 'data-currency-id';
                        radioId = `default_currency_${safeId}`;
                    } else if (pageName === 'member-levels') {
                        radioName = 'default_member_level';
                        dataAttrName = 'data-member-level-id';
                        radioId = `default_member_level_${safeId}`;
                    } else if (pageName === 'shipping-methods') {
                        radioName = 'default_shipping_method';
                        dataAttrName = 'data-shipping-method-id';
                        radioId = `default_shipping_method_${safeId}`;
                    } else if (pageName === 'payment-methods') {
                        if (col.key === 'is_default_expense') {
                            radioName = 'default_payment_method_expense';
                            dataAttrName = 'data-payment-method-expense-id';
                            radioId = `default_payment_method_expense_${safeId}`;
                        } else {
                            radioName = 'default_payment_method';
                            dataAttrName = 'data-payment-method-id';
                            radioId = `default_payment_method_${safeId}`;
                        }
                    } else if (pageName === 'phone-country-codes') {
                        radioName = 'default_phone_country_code';
                        dataAttrName = 'data-phone-code-id';
                        radioId = `default_phone_code_${safeId}`;
                    } else if (pageName === 'warehouses') {
                        radioName = 'default_warehouse';
                        dataAttrName = 'data-warehouse-id';
                        radioId = `default_warehouse_${safeId}`;
                    } else if (pageName === 'logistics-companies') {
                        radioName = 'default_logistics_company';
                        dataAttrName = 'data-logistics-company-id';
                        radioId = `default_logistics_company_${safeId}`;
                    } else if (pageName === 'pages') {
                        radioName = 'default_homepage';
                        dataAttrName = 'data-page-id';
                        radioId = `default_homepage_${safeId}`;
                    } else if (pageName === 'shifts') {
                        radioName = 'default_shift';
                        dataAttrName = 'data-shift-id';
                        radioId = `default_shift_${safeId}`;
                    }
                    // 使用 onclick 事件，並確保 dynamicList 實例存在
                    // 注意：使用 data 屬性存儲 ID，避免在 onclick 中轉義問題
                    return `
                        <div class="form-check" style="padding: 0;">
                            <input class="form-check-input" type="radio" name="${radioName}" 
                                   id="${radioId}" 
                                   value="${safeId}"
                                   ${dataAttrName}="${safeId}"
                                   ${isChecked ? 'checked' : ''} 
                                   style="cursor: pointer; margin: 0;"
                                   onchange="(function() { const itemId = this.getAttribute('${dataAttrName}'); console.log('Radio changed, itemId:', itemId, 'pageName:', '${pageName}', 'field:', '${col.key}'); if (window.dynamicList && typeof window.dynamicList.setDefaultItem === 'function') { window.dynamicList.setDefaultItem(itemId, true, '${col.key}'); } else { console.error('dynamicList not available:', window.dynamicList); } }).call(this);">
                        </div>
                    `;
                }
                
                // 特殊處理：bank-accounts 頁面的 is_default_receiving 和 is_default_payment 字段顯示為可點擊的單選框
                if (this.getPageName() === 'bank-accounts' && (col.key === 'is_default_receiving' || col.key === 'is_default_payment')) {
                    const itemId = item.id || item.ID || '';
                    if (!itemId) {
                        return '<span class="text-muted">-</span>';
                    }
                    const fieldValue = item[col.key] !== undefined ? item[col.key] : (value !== undefined && value !== null ? value : false);
                    const isChecked = fieldValue === true || fieldValue === 'true' || fieldValue === 1 || fieldValue === '1' || String(fieldValue).toLowerCase() === 'true';
                    const safeId = String(itemId).replace(/'/g, "\\'").replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    const radioName = col.key === 'is_default_receiving' ? 'default_receiving_account' : 'default_payment_account';
                    const dataAttrName = col.key === 'is_default_receiving' ? 'data-receiving-account-id' : 'data-payment-account-id';
                    const radioId = `${radioName}_${safeId}`;
                    
                    return `
                        <div class="form-check" style="padding: 0;">
                            <input class="form-check-input" type="radio" name="${radioName}" 
                                   id="${radioId}" 
                                   value="${safeId}"
                                   ${dataAttrName}="${safeId}"
                                   ${isChecked ? 'checked' : ''} 
                                   style="cursor: pointer; margin: 0;"
                                   onchange="(function() { const accountId = this.getAttribute('${dataAttrName}'); console.log('Radio changed, accountId:', accountId, 'field:', '${col.key}'); if (window.dynamicList && typeof window.dynamicList.setDefaultBankAccount === 'function') { window.dynamicList.setDefaultBankAccount(accountId, '${col.key}', true); } else { console.error('dynamicList not available:', window.dynamicList); } }).call(this);">
                        </div>
                    `;
                }
                
                // 處理 undefined 值
                if (value === undefined || value === null) {
                    return '<span class="text-muted">-</span>';
                }
                
                const badgeClass = (col.options && col.options[value]) ? col.options[value] : 'secondary';
                let label = (col.labels && col.labels[value]) ? col.labels[value] : value;
                // 嘗試從 options 翻譯鍵獲取翻譯（優先），再退回 fields
                if (typeof I18n !== 'undefined' && I18n.t) {
                    // labels 也允許放 i18n key（例如 options.boolean.true）
                    if (typeof label === 'string' && label.includes('.') && !/[\u3400-\u9FFF]/.test(label)) {
                        const tLabel = I18n.t(label);
                        if (tLabel && tLabel !== label) {
                            label = tLabel;
                        }
                    }
                    const pageName = this.getPageName();
                    const rawValue = String(value);
                    if (pageName && col.key) {
                        const k1 = `options.${pageName}.${col.key}.${rawValue}`;
                        const t1 = I18n.t(k1);
                        if (t1 && t1 !== k1) {
                            label = t1;
                        }
                    }
                    if (label === value && col.key) {
                        const k2 = `options.${col.key}.${rawValue}`;
                        const t2 = I18n.t(k2);
                        if (t2 && t2 !== k2) {
                            label = t2;
                        }
                    }
                    if (label === value && (rawValue === 'true' || rawValue === 'false' || rawValue === '1' || rawValue === '0')) {
                        const normalized = (rawValue === '1') ? 'true' : (rawValue === '0' ? 'false' : rawValue);
                        const k3 = `options.boolean.${normalized}`;
                        const t3 = I18n.t(k3);
                        if (t3 && t3 !== k3) {
                            label = t3;
                        }
                    }
                    if (label === value) {
                        const fieldKey = String(label).toLowerCase().replace(/\s+/g, '');
                        const translated = I18n.t(`fields.${fieldKey}`);
                        if (translated && translated !== `fields.${fieldKey}`) {
                            label = translated;
                        }
                    }
                }
                // 轉義 HTML 特殊字符
                const safeLabel = String(label).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                return `<span class="badge bg-${badgeClass}">${safeLabel}</span>`;
            
            case 'currency':
                return `$${parseFloat(value || 0).toFixed(2)}`;

            case 'color': {
                const rawColor = (value || '').toString().trim();
                if (!rawColor) {
                    return '<span class="text-muted">-</span>';
                }
                const safeColor = rawColor.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                return `
                    <span class="d-inline-flex align-items-center" title="${safeColor}">
                        <span style="width: 18px; height: 18px; border-radius: 50%; background: ${safeColor}; border: 1px solid #e5e7eb;"></span>
                    </span>
                `;
            }
            
            case 'date':
                if (typeof formatDate === 'undefined') {
                    return value || '-';
                }
                return formatDate(value);
            
            case 'datetime':
                if (typeof formatDateTime === 'undefined') {
                    return value || '-';
                }
                return formatDateTime(value);
            
            case 'relation':
                // 对于 user.name 这样的关系字段，value 应该是 user 对象
                if (col.relationKey) {
                    const formatNameWithLastName = (obj) => {
                        if (!obj || typeof obj !== 'object') return '-';
                        const first = String(obj.name || '').trim();
                        const last = String(obj.last_name || '').trim();
                        if (!first) return last || '-';
                        if (!last) return first;
                        const hasCJK = /[\u3400-\u9FFF]/.test(first) || /[\u3400-\u9FFF]/.test(last);
                        return hasCJK ? `${last}${first}` : `${first} ${last}`;
                    };

                    // customer/supplier：若有 last_name，依語系樣式顯示
                    if (col.relationKey === 'name' && value && typeof value === 'object' && (value.last_name || value.lastName)) {
                        // value 可能是 snake/camel
                        const obj = {
                            name: value.name,
                            last_name: value.last_name || value.lastName
                        };
                        return formatNameWithLastName(obj);
                    }

                    const relationValue = value?.[col.relationKey] || '-';
                    return relationValue;
                }
                // 如果没有 relationKey，尝试从 value 中获取 name
                // 如果 value 是对象，尝试获取 name 属性
                if (value && typeof value === 'object') {
                    return value.name || value.code || value.id || '-';
                }
                return value || '-';
            
            case 'tags':
                // 簡單字串陣列，渲染為 info badge
                if (!value || !Array.isArray(value) || value.length === 0) {
                    return '<span class="text-muted">-</span>';
                }
                return value.map(tag => `<span class="badge bg-info text-dark" style="margin-right: 4px;">${tag}</span>`).join('');

            case 'labels':
                if (!value || !Array.isArray(value) || value.length === 0) {
                    return '<span class="text-muted">-</span>';
                }
                return value.map(label => {
                    const color = label.color || '#007bff';
                    const name = label.name || '';
                    return `<span class="badge" style="background-color: ${color}; cursor: pointer; margin-right: 4px;" 
                                 onclick="window.dynamicList.filterByLabel('${label.id}')" title="點擊過濾">${name}</span>`;
                }).join('');
            
            case 'number':
                // 特殊處理：考勤記錄的工作時長和加班時長，將分鐘轉換為小時:分鐘格式
                if (this.getPageName() === 'attendances' && (col.key === 'work_duration' || col.key === 'ot_duration')) {
                    const minutes = parseInt(value) || 0;
                    if (minutes === 0) return '0分鐘';
                    const hours = Math.floor(minutes / 60);
                    const mins = minutes % 60;
                    if (hours > 0 && mins > 0) {
                        return `${hours}小時${mins}分鐘`;
                    } else if (hours > 0) {
                        return `${hours}小時`;
                    } else {
                        return `${mins}分鐘`;
                    }
                }
                return value || 0;

            case 'default-include': {
                // 用於 product-taxes / service-taxes：在列表直接勾選預設包含的單據類型
                const pageName = this.getPageName();
                if (pageName !== 'product-taxes' && pageName !== 'service-taxes') {
                    return (Array.isArray(value) ? value.join(', ') : (value || '-'));
                }
                const itemId = item.id || item.ID || '';
                if (!itemId) return '<span class="text-muted">-</span>';
                const selected = Array.isArray(value) ? value.map(String) : [];
                const opts = (col.options && Array.isArray(col.options)) ? col.options : [
                    { value: 'order', label: '訂單' },
                    { value: 'service_order', label: '服務單' }
                ];
                const safeId = String(itemId).replace(/'/g, "\\'").replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
                const html = opts.map((o, idx) => {
                    const checked = selected.includes(String(o.value));
                    const checkboxId = `di_${pageName}_${safeId}_${idx}`;
                    const safeVal = String(o.value).replace(/'/g, "\\'");
                    return `
                        <div class="form-check form-check-inline" style="margin: 0 8px 0 0;">
                            <input class="form-check-input" type="checkbox" id="${checkboxId}"
                                   ${checked ? 'checked' : ''}
                                   data-item-id="${safeId}"
                                   data-include-val="${safeVal}"
                                   onclick="event.stopPropagation();"
                                   onmousedown="event.stopPropagation();"
                                   onchange="(function(){ 
                                       const itemId = this.getAttribute('data-item-id'); 
                                       const inc = this.getAttribute('data-include-val'); 
                                       const checked = this.checked; 
                                       if (window.dynamicList && typeof window.dynamicList.updateTaxDefaultInclude === 'function') {
                                           window.dynamicList.updateTaxDefaultInclude(itemId, inc, checked);
                                       }
                                   }).call(this);">
                            ${o.label ? `<label class="form-check-label" for="${checkboxId}" style="cursor:pointer;" onclick="event.stopPropagation();" onmousedown="event.stopPropagation();">${o.label}</label>` : ''}
                        </div>
                    `;
                }).join('');
                return `<div style="white-space: nowrap;">${html}</div>`;
            }
            
            default:
                // 不再在 description 字段中添加訂單/服務單/採購單編號
                return value || '-';
        }
    }

    async updateTaxDefaultInclude(itemId, includeVal, checked) {
        const pageName = this.getPageName();
        if (pageName !== 'product-taxes' && pageName !== 'service-taxes') return;
        try {
            // 找回目前 item
            const row = (this.data || []).find(x => String(x.id || x.ID) === String(itemId));
            if (!row) return;
            const current = Array.isArray(row.default_include) ? row.default_include.map(String) : [];
            const val = String(includeVal);
            let next = current.slice();
            if (checked) {
                if (!next.includes(val)) next.push(val);
            } else {
                next = next.filter(v => v !== val);
            }

            const path = pageName === 'product-taxes' ? '/product-taxes' : '/service-taxes';
            await App.apiRequest(`${path}/${itemId}`, {
                method: 'PUT',
                body: JSON.stringify({ default_include: next })
            });
            // 更新前端記憶並保持 UI
            row.default_include = next;
            App.showAlert('已更新系統預設包含', 'success');
            // 重新載入，確保 checked 狀態與後端 sanitize 後一致
            this.loadData();
        } catch (e) {
            App.showAlert('更新失敗: ' + (e.message || e), 'danger');
            // reload 以回復 checkbox 狀態
            this.loadData();
        }
    }

    filterByLabel(labelId) {
        const filterSelect = document.getElementById('filter_label_id');
        if (filterSelect) {
            filterSelect.value = labelId;
            this.loadData();
        }
    }

    async approveExpenseRequest(id) {
        try {
            await App.apiRequest(`/expense-requests/${id}/approve`, { method: 'POST' });
            App.showAlert('已批核並產生支出', 'success');
            this.loadData();
        } catch (err) {
            App.showAlert('批核失敗: ' + (err.message || err), 'danger');
        }
    }

    async rejectExpenseRequest(id) {
        try {
            await App.apiRequest(`/expense-requests/${id}/reject`, { method: 'POST' });
            App.showAlert('已拒絕', 'info');
            this.loadData();
        } catch (err) {
            App.showAlert('拒絕失敗: ' + (err.message || err), 'danger');
        }
    }
    
    async convertQuotationToOrder(id) {
        if (!confirm('確定要將此報價單轉成訂單嗎？')) {
            return;
        }
        try {
            await App.apiRequest(`/orders/${id}/convert-to-order`, { method: 'POST' });
            App.showAlert('已成功轉成訂單', 'success');
            this.loadData();
        } catch (err) {
            App.showAlert(err.message || '轉換失敗', 'danger');
        }
    }

    async generatePaymentLink(id) {
        try {
            const data = await App.apiRequest(`/orders/${id}/payment-link`, { method: 'POST' });
            const link = data.payment_link;
            if (!link || !link.url) {
                App.showAlert('無法取得付款連結', 'danger');
                return;
            }
            // Show modal with link
            const existingModal = document.getElementById('paymentLinkListModal');
            if (existingModal) existingModal.remove();

            const modal = document.createElement('div');
            modal.className = 'modal fade';
            modal.id = 'paymentLinkListModal';
            modal.setAttribute('tabindex', '-1');
            modal.innerHTML = `
                <div class="modal-dialog modal-dialog-centered">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title"><i class="bi bi-link-45deg me-1"></i> 結帳連結</h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                        </div>
                        <div class="modal-body">
                            <div class="mb-3">
                                <label class="form-label fw-semibold">付款連結</label>
                                <div class="input-group">
                                    <input type="text" class="form-control" id="plListUrl" value="${link.url}" readonly>
                                    <button class="btn btn-outline-primary" type="button" id="plListCopyBtn">
                                        <i class="bi bi-clipboard me-1"></i> 複製
                                    </button>
                                </div>
                            </div>
                            <div class="d-flex gap-2">
                                <a href="${link.url}" target="_blank" class="btn btn-sm btn-outline-secondary">
                                    <i class="bi bi-box-arrow-up-right me-1"></i> 開啟連結
                                </a>
                                <button type="button" class="btn btn-sm btn-outline-danger" id="plListRevokeBtn">
                                    <i class="bi bi-x-circle me-1"></i> 撤銷連結
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);

            // Copy button
            modal.querySelector('#plListCopyBtn').addEventListener('click', () => {
                const input = modal.querySelector('#plListUrl');
                input.select();
                navigator.clipboard.writeText(input.value).then(() => {
                    const btn = modal.querySelector('#plListCopyBtn');
                    btn.innerHTML = '<i class="bi bi-check me-1"></i> 已複製';
                    btn.classList.replace('btn-outline-primary', 'btn-success');
                    setTimeout(() => {
                        btn.innerHTML = '<i class="bi bi-clipboard me-1"></i> 複製';
                        btn.classList.replace('btn-success', 'btn-outline-primary');
                    }, 2000);
                });
            });

            // Revoke button
            modal.querySelector('#plListRevokeBtn').addEventListener('click', async () => {
                if (!confirm('確定要撤銷此結帳連結嗎？')) return;
                try {
                    await App.apiRequest(`/orders/${id}/payment-link`, { method: 'DELETE' });
                    bootstrap.Modal.getInstance(modal).hide();
                    App.showAlert('結帳連結已撤銷', 'success');
                } catch (err) {
                    App.showAlert('撤銷失敗: ' + (err.message || ''), 'danger');
                }
            });

            // Clean up on close
            modal.addEventListener('hidden.bs.modal', () => modal.remove());

            new bootstrap.Modal(modal).show();
        } catch (err) {
            App.showAlert('生成結帳連結失敗: ' + (err.message || ''), 'danger');
        }
    }

    async handleCustomAction(action, id) {
        if (action === 'convertToOrder') {
            await this.convertQuotationToOrder(id);
        } else {
            console.warn('Unknown custom action:', action);
        }
    }

    showImageLightbox(imageUrl) {
        // 轉義 URL 以防止 XSS
        const safeImageUrl = String(imageUrl).replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        
        // 創建 lightbox modal
        const modal = document.createElement('div');
        modal.className = 'modal fade';
        modal.id = 'imageLightbox';
        modal.setAttribute('tabindex', '-1');
        modal.setAttribute('data-bs-backdrop', 'static');
        modal.innerHTML = `
            <div class="modal-dialog modal-dialog-centered" style="max-width: 90vw; max-height: 90vh;">
                <div class="modal-content" style="border: none; box-shadow: none;">
                    <div class="modal-header" style="border: none; padding: 0.5rem; justify-content: flex-end;">
                        <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal" aria-label="關閉" style="opacity: 1;"></button>
                    </div>
                    <div class="modal-body text-center" style="padding: 0;">
                        <img src="${safeImageUrl}" class="img-fluid" alt="圖片預覽" style="max-width: 100%; max-height: 85vh; object-fit: contain; border-radius: 8px; box-shadow: 0 10px 40px rgba(0,0,0,0.3);">
                    </div>
                </div>
            </div>
        `;
        
        // 添加背景樣式
        const style = document.createElement('style');
        style.textContent = `
            #imageLightbox .modal-backdrop {
                background-color: rgba(0, 0, 0, 0.85) !important;
                backdrop-filter: blur(4px);
            }
            #imageLightbox .modal-content {
                animation: fadeInScale 0.3s ease-out;
            }
            @keyframes fadeInScale {
                from {
                    opacity: 0;
                    transform: scale(0.9);
                }
                to {
                    opacity: 1;
                    transform: scale(1);
                }
            }
        `;
        document.head.appendChild(style);
        
        document.body.appendChild(modal);
        const bsModal = new bootstrap.Modal(modal, {
            backdrop: true,
            keyboard: true
        });
        bsModal.show();
        
        // 點擊背景關閉
        modal.addEventListener('click', function(e) {
            if (e.target === modal || e.target.classList.contains('modal-dialog')) {
                bsModal.hide();
            }
        });
        
        // ESC 鍵關閉
        document.addEventListener('keydown', function escHandler(e) {
            if (e.key === 'Escape' && document.getElementById('imageLightbox')) {
                bsModal.hide();
                document.removeEventListener('keydown', escHandler);
            }
        });
        
        modal.addEventListener('hidden.bs.modal', () => {
            modal.remove();
            if (style.parentNode) {
                style.parentNode.removeChild(style);
            }
        });
    }

    getNestedValue(obj, path) {
        return path.split('.').reduce((current, key) => current?.[key], obj);
    }

    // 切換單個項目的選擇狀態
    toggleItemSelection(itemId, checked) {
        if (checked) {
            this.selectedItems.add(itemId);
        } else {
            this.selectedItems.delete(itemId);
        }
        this.updateBulkDeleteButton();
        this.updateSelectAllCheckbox();
    }

    // 切換全選
    toggleSelectAll(checked) {
        const checkboxes = document.querySelectorAll('.item-checkbox');
        checkboxes.forEach(checkbox => {
            const itemId = checkbox.getAttribute('data-item-id');
            checkbox.checked = checked;
            if (checked) {
                this.selectedItems.add(itemId);
            } else {
                this.selectedItems.delete(itemId);
            }
        });
        this.updateBulkDeleteButton();
    }

    // 全選
    selectAll() {
        const checkboxes = document.querySelectorAll('.item-checkbox');
        checkboxes.forEach(checkbox => {
            const itemId = checkbox.getAttribute('data-item-id');
            checkbox.checked = true;
            this.selectedItems.add(itemId);
        });
        this.updateBulkDeleteButton();
        this.updateSelectAllCheckbox();
    }

    // 取消全選
    deselectAll() {
        const checkboxes = document.querySelectorAll('.item-checkbox');
        checkboxes.forEach(checkbox => {
            const itemId = checkbox.getAttribute('data-item-id');
            checkbox.checked = false;
            this.selectedItems.delete(itemId);
        });
        this.updateBulkDeleteButton();
        this.updateSelectAllCheckbox();
    }

    // 更新全選checkbox狀態
    updateSelectAllCheckbox() {
        const selectAllCheckbox = document.getElementById('selectAllCheckbox');
        if (!selectAllCheckbox) return;
        
        const checkboxes = document.querySelectorAll('.item-checkbox');
        const checkedCount = document.querySelectorAll('.item-checkbox:checked').length;
        
        if (checkboxes.length === 0) {
            selectAllCheckbox.checked = false;
            selectAllCheckbox.indeterminate = false;
        } else if (checkedCount === checkboxes.length) {
            selectAllCheckbox.checked = true;
            selectAllCheckbox.indeterminate = false;
        } else if (checkedCount > 0) {
            selectAllCheckbox.checked = false;
            selectAllCheckbox.indeterminate = true;
        } else {
            selectAllCheckbox.checked = false;
            selectAllCheckbox.indeterminate = false;
        }
    }

    // 啟用批量刪除模式（顯示 checkbox 和確認刪除按鈕）
    enableBulkDelete() {
        // 顯示表頭的 checkbox
        const checkboxHeader = document.getElementById('checkboxHeader');
        if (checkboxHeader) {
            checkboxHeader.style.display = '';
        }
        
        // 顯示所有行的 checkbox
        const checkboxCells = document.querySelectorAll('.checkbox-cell');
        checkboxCells.forEach(cell => {
            cell.style.display = '';
        });
        
        // 顯示確認刪除按鈕
        const confirmDeleteBtn = document.getElementById('confirmDeleteBtn');
        if (confirmDeleteBtn) {
            confirmDeleteBtn.style.display = 'inline-block';
        }

        const cancelBulkDeleteBtn = document.getElementById('cancelBulkDeleteBtn');
        if (cancelBulkDeleteBtn) {
            cancelBulkDeleteBtn.style.display = 'inline-block';
        }
        
        // 更新選中數量
        this.updateBulkDeleteButton();
    }

    // 更新批量刪除按鈕顯示狀態
    updateBulkDeleteButton() {
        const confirmDeleteBtn = document.getElementById('confirmDeleteBtn');
        const selectedCount = document.getElementById('selectedCount');
        
        if (confirmDeleteBtn && confirmDeleteBtn.style.display !== 'none') {
            if (this.selectedItems.size > 0) {
                confirmDeleteBtn.style.display = 'inline-block';
                if (selectedCount) {
                    selectedCount.textContent = this.selectedItems.size;
                }
            } else {
                // 如果沒有選中項目，仍然顯示按鈕（因為已經進入批量刪除模式）
                if (selectedCount) {
                    selectedCount.textContent = '0';
                }
            }
        }
    }

    cancelBulkDelete() {
        this.selectedItems.clear();

        const checkboxHeader = document.getElementById('checkboxHeader');
        if (checkboxHeader) {
            checkboxHeader.style.display = 'none';
        }

        const checkboxCells = document.querySelectorAll('.checkbox-cell');
        checkboxCells.forEach(cell => {
            cell.style.display = 'none';
        });

        const confirmDeleteBtn = document.getElementById('confirmDeleteBtn');
        if (confirmDeleteBtn) {
            confirmDeleteBtn.style.display = 'none';
        }

        const cancelBulkDeleteBtn = document.getElementById('cancelBulkDeleteBtn');
        if (cancelBulkDeleteBtn) {
            cancelBulkDeleteBtn.style.display = 'none';
        }

        this.updateBulkDeleteButton();
        this.updateSelectAllCheckbox();
    }

    // 批量刪除
    async bulkDelete() {
        if (this.selectedItems.size === 0) {
            App.showAlert(this.getText('common.selectItemsToDelete'), 'warning');
            return;
        }

        const count = this.selectedItems.size;
        const pageName = this.config.title || '項目';
        const confirmTpl = this.getText('common.bulkDeleteConfirm'); // uses {count} {page}
        const confirmMsg = confirmTpl
            .replace('{count}', String(count))
            .replace('{page}', String(pageName || ''));
        if (!confirm(confirmMsg)) {
            return;
        }

        try {
            const deletePromises = Array.from(this.selectedItems).map(id => {
                return App.apiRequest(`${this.config.apiPath}/${id}`, { method: 'DELETE' });
            });

            await Promise.all(deletePromises);
            
            const okTpl = this.getText('common.bulkDeleteSuccess'); // uses {count} {page}
            App.showAlert(okTpl.replace('{count}', String(count)).replace('{page}', String(pageName || '')), 'success');
            this.selectedItems.clear();
            
            // 隱藏 checkbox 和確認刪除按鈕
            const checkboxHeader = document.getElementById('checkboxHeader');
            if (checkboxHeader) {
                checkboxHeader.style.display = 'none';
            }
            const checkboxCells = document.querySelectorAll('.checkbox-cell');
            checkboxCells.forEach(cell => {
                cell.style.display = 'none';
            });
            const confirmDeleteBtn = document.getElementById('confirmDeleteBtn');
            if (confirmDeleteBtn) {
                confirmDeleteBtn.style.display = 'none';
            }

            const cancelBulkDeleteBtn = document.getElementById('cancelBulkDeleteBtn');
            if (cancelBulkDeleteBtn) {
                cancelBulkDeleteBtn.style.display = 'none';
            }
            
            this.updateBulkDeleteButton();
            this.updateSelectAllCheckbox();
            this.loadData();
        } catch (error) {
            console.error('批量刪除失敗:', error);
            App.showAlert(this.getText('common.bulkDeleteFailed') + ': ' + (error.message || error), 'danger');
        }
    }

    async deleteItem(id) {
        const title = this.getMenuTitle();
        const confirmText = this.getText('common.confirmDelete');
        const trashInfo = this.getText('common.trashInfo');
        if (!confirm(`${confirmText}${title}嗎？\n\n${trashInfo}`)) return;
        if (typeof App === 'undefined') {
            App.showAlert((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'System is not initialized. Please refresh the page.' : '系統未初始化，請重新整理頁面', 'warning');
            return;
        }
        try {
            await App.apiRequest(`${this.config.apiPath}/${id}`, { method: 'DELETE' });
            const successText = this.getText('common.deleteSuccess');
            App.showAlert(successText + ' - ' + trashInfo, 'success');
            this.loadData();
        } catch (error) {
            const errorText = this.getText('common.deleteError');
            let errorMessage = error.message || error.error || '未知錯誤';
            // 如果是租戶主帳號，顯示 i18n 提示
            if (errorMessage.includes('tenant owner') || errorMessage.includes('Cannot delete tenant owner')) {
                errorMessage = this.getText('common.cannotDeleteTenantOwner');
            }
            // 如果是系統預設資料
            if (errorMessage.includes('system default')) {
                errorMessage = this.getText('common.cannotDeleteSystemDefault') || '無法刪除系統預設資料';
            }
            App.showAlert(errorText + ': ' + errorMessage, 'danger');
        }
    }

    // 從垃圾筒還原項目
    async restoreItem(id) {
        const title = this.getMenuTitle();
        const confirmText = this.getText('common.confirmRestore');
        if (!confirm(`${confirmText}${title}嗎？`)) return;
        
        try {
            const resourceName = this.getTrashResourceName();
            await App.apiRequest(`/api/v1/trash/${resourceName}/${id}/restore`, { method: 'POST' });
            const successText = this.getText('common.restoreSuccess');
            App.showAlert(successText, 'success');
            this.loadData();
        } catch (error) {
            const errorText = this.getText('common.restoreError');
            App.showAlert(errorText + ': ' + (error.message || error.error || '未知錯誤'), 'danger');
        }
    }

    // 永久刪除項目
    async permanentDeleteItem(id) {
        const title = this.getMenuTitle();
        const confirmText = this.getText('common.confirmPermanentDelete');
        const warningText = this.getText('common.permanentDeleteWarning');
        if (!confirm(`${confirmText}${title}嗎？\n\n⚠️ ${warningText}`)) return;
        
        try {
            const resourceName = this.getTrashResourceName();
            await App.apiRequest(`/api/v1/trash/${resourceName}/${id}`, { method: 'DELETE' });
            App.showAlert('永久刪除成功', 'success');
            this.loadData();
        } catch (error) {
            App.showAlert('永久刪除失敗: ' + (error.message || error.error || '未知錯誤'), 'danger');
        }
    }

    exportToExcel() {
        if ((this.totalItems || 0) <= 0) {
            App.showAlert((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'No data to export' : '沒有資料，不能匯出', 'warning');
            return;
        }
        if (typeof exportToExcel === 'undefined') {
            App.showAlert((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Export feature is not loaded. Please refresh the page.' : '導出功能未載入，請重新整理頁面', 'warning');
            return;
        }
        const apiPath = this.config.apiPath || '';
        const moduleName = apiPath.replace(/^\/api\/v1\//, '').replace(/^\//, '');
        exportToExcel(moduleName);
    }

    exportToPDF() {
        if ((this.totalItems || 0) <= 0) {
            App.showAlert((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'No data to export' : '沒有資料，不能匯出', 'warning');
            return;
        }
        if (typeof exportToPDF === 'undefined') {
            App.showAlert((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Export feature is not loaded. Please refresh the page.' : '導出功能未載入，請重新整理頁面', 'warning');
            return;
        }
        const apiPath = this.config.apiPath || '';
        const moduleName = apiPath.replace(/^\/api\/v1\//, '').replace(/^\//, '');
        exportToPDF(moduleName);
    }

    async setDefaultItem(itemId, isDefault, fieldName = 'is_default') {
        console.log('setDefaultItem called:', itemId, isDefault, fieldName);
        
        if (!itemId) {
            console.error('Item ID is missing');
            return;
        }

        const pageName = this.getPageName();
        console.log('Page name:', pageName, 'API path:', this.config.apiPath);
        let itemType = '項目';
        if (pageName === 'currencies') {
            itemType = '貨幣';
        } else if (pageName === 'member-levels') {
            itemType = '會員等級';
        } else if (pageName === 'shipping-methods') {
            itemType = '運送方式';
        } else if (pageName === 'payment-methods') {
            itemType = '付款方式';
        } else if (pageName === 'phone-country-codes') {
            itemType = '電話區號';
        } else if (pageName === 'warehouses') {
            itemType = '倉庫';
        } else if (pageName === 'logistics-companies') {
            itemType = '物流公司';
        } else if (pageName === 'pages') {
            itemType = '頁面';
        } else if (pageName === 'shifts') {
            itemType = '工作時段';
        }

        try {
            App.showLoading('正在設置...');
            
            // 獲取當前項目信息
            const item = await App.apiRequest(`${this.config.apiPath}/${itemId}`);
            console.log(`${itemType} data:`, item);
            
            if (!item) {
                throw new Error(`無法獲取${itemType}信息`);
            }
            
            // 更新為默認項目（後端會自動取消其他默認項目）
            const updateData = {};
            if (pageName === 'currencies') {
                updateData.code = item.code;
                updateData.name = item.name;
                updateData.symbol = item.symbol || null;
                updateData.exchange_rate = item.exchange_rate || 1.0;
                updateData.is_default = true;
                updateData.status = item.status || 'active';
                console.log('Currency updateData:', updateData);
            } else if (pageName === 'member-levels') {
                updateData.name = item.name;
                updateData.code = item.code;
                updateData.level_order = item.level_order;
                updateData.min_points = item.min_points;
                updateData.min_purchase_amount = item.min_purchase_amount;
                updateData.discount_rate = item.discount_rate;
                updateData.is_default = true;
                updateData.auto_upgrade = item.auto_upgrade;
                updateData.description = item.description;
                updateData.status = item.status || 'active';
            } else if (pageName === 'shipping-methods') {
                updateData.name = item.name;
                updateData.code = item.code;
                updateData.requires_shipping = item.requires_shipping;
                updateData.is_default = true;
                updateData.status = item.status || 'active';
            } else if (pageName === 'payment-methods') {
                updateData.name = item.name;
                updateData.code = item.code;
                // IMPORTANT: UpdatePaymentMethod 會直接覆蓋 bool 欄位，因此要保留另一個 default 欄位的現值，避免被意外清掉。
                const curIsDefault = item.is_default === true || item.is_default === 'true' || item.is_default === 1 || item.is_default === '1' || String(item.is_default).toLowerCase() === 'true';
                const curIsDefaultExpense = item.is_default_expense === true || item.is_default_expense === 'true' || item.is_default_expense === 1 || item.is_default_expense === '1' || String(item.is_default_expense).toLowerCase() === 'true';
                updateData.is_default = (fieldName === 'is_default') ? true : curIsDefault;
                updateData.is_default_expense = (fieldName === 'is_default_expense') ? true : curIsDefaultExpense;
                updateData.status = item.status || 'active';
            } else if (pageName === 'phone-country-codes') {
                updateData.code = item.code;
                updateData.name = item.name;
                updateData.is_default = true;
            } else if (pageName === 'warehouses') {
                updateData.code = item.code;
                updateData.name = item.name;
                updateData.address = item.address || null;
                updateData.contact_person = item.contact_person || null;
                updateData.phone = item.phone || null;
                updateData.email = item.email || null;
                updateData.is_default = true;
                updateData.status = item.status || 'active';
            } else if (pageName === 'logistics-companies') {
                updateData.name = item.name;
                updateData.code = item.code;
                updateData.base_fee = item.base_fee || null;
                updateData.per_item_fee = item.per_item_fee || null;
                updateData.per_weight_fee = item.per_weight_fee || null;
                updateData.per_area_fee = item.per_area_fee || null;
                updateData.is_default = true;
                updateData.status = item.status || 'active';
            } else if (pageName === 'pages') {
                updateData.name = item.name;
                updateData.slug = item.slug;
                updateData.title = item.title || null;
                updateData.description = item.description || null;
                updateData.status = item.status || 'published';
                updateData.is_homepage = true;
            } else if (pageName === 'shifts') {
                updateData.name = item.name;
                // 格式化时间为 HH:MM（SQLTime 已经是字符串格式，如 "09:00:00" 或 "09:00"）
                let startTime = item.start_time || '09:00:00';
                let endTime = item.end_time || '18:00:00';
                // 如果是字符串，提取 HH:MM 部分（去掉秒数）
                if (typeof startTime === 'string') {
                    startTime = startTime.slice(0, 5); // 取前5个字符 "HH:MM"
                }
                if (typeof endTime === 'string') {
                    endTime = endTime.slice(0, 5); // 取前5个字符 "HH:MM"
                }
                updateData.start_time = startTime;
                updateData.end_time = endTime;
                updateData.is_default = true;
            }
            
            console.log('Sending update request:', {
                url: `${this.config.apiPath}/${itemId}`,
                method: 'PUT',
                data: updateData,
                'updateData.is_default': updateData.is_default,
                'updateData JSON': JSON.stringify(updateData)
            });
            
            const response = await App.apiRequest(`${this.config.apiPath}/${itemId}`, {
                method: 'PUT',
                body: JSON.stringify(updateData)
            });
            
            console.log('Update response:', response);
            console.log('Update response is_default:', response.is_default, typeof response.is_default);
            console.log('Full response object keys:', Object.keys(response || {}));
            console.log('Full response:', JSON.stringify(response, null, 2));

            // 檢查響應是否成功（即使 is_default 未定義，也重新載入列表）
            if (response && response.error) {
                throw new Error(response.error);
            }

            App.hideLoading();
            if (pageName === 'payment-methods') {
                const label = fieldName === 'is_default_expense' ? '系統預設支出付款方法' : '系統預設客戶付款方法';
                App.showAlert(`已設置為${label}`, 'success');
            } else {
                App.showAlert(`已設置為系統預設${itemType}`, 'success');
            }
            
            // 重新載入列表以更新所有單選框狀態（無論響應中是否有 is_default）
            await this.loadData();
        } catch (error) {
            App.hideLoading();
            console.error(`設置默認${itemType}失敗:`, error);
            console.error('Error details:', {
                url: `${this.config.apiPath}/${itemId}`,
                method: 'PUT',
                data: updateData,
                error: error
            });
            const errorMessage = error.message || error || '未知錯誤';
            App.showAlert(`設置失敗: ${errorMessage}`, 'danger');
            // 重新載入數據以恢復正確的選中狀態
            await this.loadData();
        }
    }

    async setDefaultBankAccount(accountId, fieldName, isDefault) {
        console.log('setDefaultBankAccount called:', accountId, fieldName, isDefault);
        
        if (!accountId) {
            console.error('Account ID is missing');
            return;
        }

        const fieldLabel = fieldName === 'is_default_receiving' ? '收款帳號' : '付款帳號';

        try {
            App.showLoading('正在設置...');
            
            // 獲取當前賬戶信息
            const account = await App.apiRequest(`${this.config.apiPath}/${accountId}`);
            console.log('銀行賬戶 data:', account);
            
            if (!account) {
                throw new Error(`無法獲取銀行賬戶信息`);
            }
            
            // 更新為默認賬戶（後端會自動取消其他默認賬戶）
            // 注意：保留另一個字段的當前值，允許同時設置為默認收款帳號和默認付款帳號
            const updateData = {
                name: account.name,
                bank_name: account.bank_name,
                account_number: account.account_number,
                account_holder: account.account_holder || null,
                currency: account.currency,
                status: account.status,
                notes: account.notes || null,
                // 保留另一個字段的當前值
                is_default_receiving: fieldName === 'is_default_receiving' ? true : (account.is_default_receiving || false),
                is_default_payment: fieldName === 'is_default_payment' ? true : (account.is_default_payment || false)
            };
            
            console.log('Sending update request:', {
                url: `${this.config.apiPath}/${accountId}`,
                method: 'PUT',
                data: updateData
            });
            
            const response = await App.apiRequest(`${this.config.apiPath}/${accountId}`, {
                method: 'PUT',
                body: JSON.stringify(updateData)
            });
            
            console.log('Update response:', response);

            App.hideLoading();
            App.showAlert(`已設置為系統預設${fieldLabel}`, 'success');
            
            // 重新載入列表以更新所有單選框狀態
            await this.loadData();
        } catch (error) {
            App.hideLoading();
            console.error(`設置默認${fieldLabel}失敗:`, error);
            const errorMessage = error.message || error || '未知錯誤';
            App.showAlert(`設置失敗: ${errorMessage}`, 'danger');
            // 重新載入數據以恢復正確的選中狀態
            await this.loadData();
        }
    }

    async updateCurrencyRates() {
        if (!confirm('確定要自動更新所有貨幣的匯率嗎？這將從外部 API 獲取最新匯率並更新數據庫。')) {
            return;
        }

        try {
            App.showLoading('正在更新匯率...');
            const response = await App.apiRequest('/currencies/update-rates', {
                method: 'POST'
            });

            App.hideLoading();
            App.showAlert(`成功更新 ${response.updated || 0} 個貨幣的匯率。基礎貨幣：${response.base || 'N/A'}，更新日期：${response.date || 'N/A'}`, 'success');
            
            // 重新載入列表
            this.loadData();
        } catch (error) {
            App.hideLoading();
            App.showAlert('更新匯率失敗: ' + (error.message || error), 'danger');
        }
    }

    async generateMonthlyCommissions() {
        if (!confirm('確定要自動生成本月所有訂單的佣金支出嗎？')) {
            return;
        }

        try {
            const response = await App.apiRequest('/expenses/generate-monthly-commissions', {
                method: 'POST',
                body: JSON.stringify({})
            });

            App.showAlert(`成功生成 ${response.count || 0} 筆佣金支出`, 'success');
            
            // 重新載入列表
            this.loadData();
        } catch (error) {
            console.error('Failed to generate monthly commissions:', error);
            App.showAlert('生成佣金支出失敗: ' + error.message, 'danger');
        }
    }

    async viewCouponUsage(couponId) {
        try {
            const data = await App.apiRequest(`${this.config.apiPath}/${couponId}/usage?limit=100`);
            const orders = data.data || [];
            
            if (orders.length === 0) {
                App.showAlert('此優惠券尚未被使用', 'info');
                return;
            }
            
            // 創建模態框顯示使用記錄
            const modal = document.createElement('div');
            modal.className = 'modal fade';
            modal.innerHTML = `
                <div class="modal-dialog modal-lg">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title">優惠券使用記錄</h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                        </div>
                        <div class="modal-body">
                            <div class="table-responsive">
                                <table class="table table-hover">
                                    <thead>
                                        <tr>
                                            <th>訂單號</th>
                                            <th>客戶</th>
                                            <th>訂單金額</th>
                                            <th>折扣金額</th>
                                            <th>使用時間</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        ${orders.map(order => `
                                            <tr>
                                                <td>${order.order_number || '-'}</td>
                                                <td>${order.customer ? order.customer.name : '-'}</td>
                                                <td>$${parseFloat(order.total_amount || 0).toFixed(2)}</td>
                                                <td>$${parseFloat(order.coupon_discount || 0).toFixed(2)}</td>
                                                <td>${new Date(order.created_at).toLocaleString('zh-TW')}</td>
                                            </tr>
                                        `).join('')}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">關閉</button>
                        </div>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
            const bsModal = new bootstrap.Modal(modal);
            bsModal.show();
            modal.addEventListener('hidden.bs.modal', () => modal.remove());
        } catch (error) {
            App.showAlert('載入使用記錄失敗: ' + error.message, 'danger');
        }
    }

    showDraftList() {
        const pageName = this.getPageName();
        if (!pageName) {
            App.showAlert('未設定頁面名稱，無法載入草稿', 'warning');
            return;
        }

        const drafts = draftManager.getDraftsForPage(pageName);
        const currentLang = localStorage.getItem('nwork_lang') || (navigator.language?.startsWith('zh') ? 'zh' : 'en');
        const draftText = currentLang === 'zh' ? '草稿' : 'Drafts';

        const listContent = drafts.length > 0
            ? drafts.map(d => {
                // 獲取編號和名稱
                const code = d.keyField || d.data?.order_number || d.data?.code || '';
                const name = d.data?.name || '';
                const createdAt = new Date(d.createdAt).toLocaleString('zh-TW');
                
                return `
                    <div class="list-group-item d-flex justify-content-between align-items-center">
                        <div class="flex-grow-1">
                            ${code ? `<div class="fw-bold">${code}</div>` : ''}
                            ${name ? `<div>${name}</div>` : ''}
                            <small class="text-muted">${createdAt}</small>
                        </div>
                        <div class="d-flex gap-2">
                            <button class="btn btn-sm btn-primary draft-action-btn" onclick="window.dynamicList.loadDraftFromModal('${d.id}')">
                                <i class="bi bi-box-arrow-in-right"></i> <span class="d-none d-md-inline">${currentLang === 'zh' ? '載入' : 'Load'}</span>
                            </button>
                            <button class="btn btn-sm btn-outline-danger draft-action-btn" onclick="window.dynamicList.deleteDraftFromModal('${d.id}', this)">
                                <i class="bi bi-trash"></i> <span class="d-none d-md-inline">${currentLang === 'zh' ? '刪除' : 'Delete'}</span>
                            </button>
                        </div>
                    </div>
                `;
            }).join('')
            : `<p class="text-muted mb-0">${currentLang === 'zh' ? '目前沒有草稿' : 'No drafts yet'}</p>`;

        const existingModal = document.getElementById('draftListModal');
        if (existingModal) existingModal.remove();

        const modal = document.createElement('div');
        modal.className = 'modal fade';
        modal.id = 'draftListModal';
        modal.setAttribute('tabindex', '-1');
        modal.innerHTML = `
            <div class="modal-dialog modal-dialog-scrollable">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title"><i class="bi bi-file-earmark-text me-2"></i>${draftText}</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                    </div>
                    <div class="modal-body">
                        <div class="list-group">
                            ${listContent}
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">${currentLang === 'zh' ? '關閉' : 'Close'}</button>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(modal);
        const bsModal = new bootstrap.Modal(modal);
        bsModal.show();
        modal.addEventListener('hidden.bs.modal', () => modal.remove());
    }

    loadDraftFromModal(draftId) {
        const modalEl = document.getElementById('draftListModal');
        if (modalEl) {
            const instance = bootstrap.Modal.getInstance(modalEl);
            instance?.hide();
        }
        const target = `${this.config.listPath || ''}/new?draft_id=${encodeURIComponent(draftId)}`;
        if (typeof Router !== 'undefined' && Router.go) {
            Router.go(target);
        } else {
            window.location.href = target;
        }
    }

    deleteDraftFromModal(draftId, btn) {
        const pageName = this.getPageName();
        draftManager.deleteDraft(pageName, draftId);

        const item = btn?.closest('.list-group-item');
        if (item) item.remove();

        this.updateDraftBadge(draftManager.getDraftCount(pageName));

        const listGroup = document.querySelector('#draftListModal .list-group');
        if (listGroup && listGroup.children.length === 0) {
            listGroup.innerHTML = `<p class="text-muted mb-0">目前沒有草稿</p>`;
        }
    }

    updateDraftBadge(count) {
        const draftButton = document.querySelector('#dynamicListContainer button[onclick="window.dynamicList.showDraftList()"]');
        if (!draftButton) return;

        let badge = draftButton.querySelector('.badge');
        if (count > 0) {
            if (!badge) {
                badge = document.createElement('span');
                badge.className = 'badge bg-danger ms-1';
                draftButton.appendChild(badge);
            }
            badge.textContent = count;
        } else if (badge) {
            badge.remove();
        }
    }

    async updateAppointmentStatus(appointmentId, status) {
        const getStatusLabel = (s) => {
            if (typeof I18n !== 'undefined' && I18n.t) {
                return I18n.t(`common.${s}`);
            }
            const labels = {
                'cancelled': this.getText('common.cancelled'),
                'completed': this.getText('common.completed')
            };
            return labels[s] || s;
        };
        const statusLabel = getStatusLabel(status);
        
        if (!confirm(`確定要將此預約標記為「${statusLabel}」嗎？`)) return;
        
        if (typeof App === 'undefined') {
            App.showAlert((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'System is not initialized. Please refresh the page.' : '系統未初始化，請重新整理頁面', 'warning');
            return;
        }
        
        try {
            await App.apiRequest(`${this.config.apiPath}/${appointmentId}`, {
                method: 'PUT',
                body: JSON.stringify({ status: status })
            });
            App.showAlert(`預約已標記為「${statusLabel}」`, 'success');
            this.loadData();
        } catch (error) {
            App.showAlert('更新失敗: ' + error.message, 'danger');
        }
    }

    renderPagination() {
        const pagination = document.getElementById('paginationContainer');
        if (!pagination) return;
        
        // 即使只有一页也显示分页控件
        const prevText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.previous') : '上一页';
        const nextText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.next') : '下一页';
        const showingText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.showing') : '显示';
        const ofText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.of') : '共';
        const itemsText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.items') : '条';
        
        const startItem = this.totalItems === 0 ? 0 : ((this.currentPage - 1) * this.pageSize + 1);
        const endItem = Math.min(this.currentPage * this.pageSize, this.totalItems);
        
        let paginationHTML = `
            <div class="d-flex justify-content-between align-items-center">
                <div class="text-muted small">
                    ${showingText} ${startItem}-${endItem} ${ofText} ${this.totalItems} ${itemsText}
                </div>
                <nav>
                    <ul class="pagination pagination-sm mb-0">
        `;
        
        // 上一页按钮
        paginationHTML += `
            <li class="page-item ${this.currentPage === 1 ? 'disabled' : ''}">
                <a class="page-link" href="#" onclick="event.preventDefault(); window.dynamicList.goToPage(${this.currentPage - 1}); return false;" ${this.currentPage === 1 ? 'tabindex="-1" aria-disabled="true"' : ''}>
                    ${prevText}
                </a>
            </li>
        `;
        
        // 页码按钮
        const maxPagesToShow = 7;
        let startPage = Math.max(1, this.currentPage - Math.floor(maxPagesToShow / 2));
        let endPage = Math.min(this.totalPages, startPage + maxPagesToShow - 1);
        
        if (endPage - startPage < maxPagesToShow - 1) {
            startPage = Math.max(1, endPage - maxPagesToShow + 1);
        }
        
        // 第一页
        if (startPage > 1) {
            paginationHTML += `
                <li class="page-item">
                    <a class="page-link" href="#" onclick="event.preventDefault(); window.dynamicList.goToPage(1); return false;">1</a>
                </li>
            `;
            if (startPage > 2) {
                paginationHTML += `<li class="page-item disabled"><span class="page-link">...</span></li>`;
            }
        }
        
        // 页码
        for (let i = startPage; i <= endPage; i++) {
            paginationHTML += `
                <li class="page-item ${i === this.currentPage ? 'active' : ''}">
                    <a class="page-link" href="#" onclick="event.preventDefault(); window.dynamicList.goToPage(${i}); return false;">${i}</a>
                </li>
            `;
        }
        
        // 最后一页
        if (endPage < this.totalPages) {
            if (endPage < this.totalPages - 1) {
                paginationHTML += `<li class="page-item disabled"><span class="page-link">...</span></li>`;
            }
            paginationHTML += `
                <li class="page-item">
                    <a class="page-link" href="#" onclick="event.preventDefault(); window.dynamicList.goToPage(${this.totalPages}); return false;">${this.totalPages}</a>
                </li>
            `;
        }
        
        // 下一页按钮
        paginationHTML += `
            <li class="page-item ${this.currentPage === this.totalPages ? 'disabled' : ''}">
                <a class="page-link" href="#" onclick="event.preventDefault(); window.dynamicList.goToPage(${this.currentPage + 1}); return false;" ${this.currentPage === this.totalPages ? 'tabindex="-1" aria-disabled="true"' : ''}>
                    ${nextText}
                </a>
            </li>
        `;
        
        paginationHTML += `
                    </ul>
                </nav>
            </div>
        `;
        
        pagination.innerHTML = paginationHTML;

        // mobile：標記第一/最後可見 page-item（避免圓角被 ... 或 disabled/hidden 影響）
        try {
            const ul = pagination.querySelector('ul.pagination');
            if (ul) {
                const items = Array.from(ul.querySelectorAll('.page-item'))
                    .filter(li => {
                        const style = window.getComputedStyle(li);
                        return style && style.display !== 'none' && style.visibility !== 'hidden';
                    });
                items.forEach(li => li.classList.remove('first-visible', 'last-visible'));
                if (items.length > 0) {
                    items[0].classList.add('first-visible');
                    items[items.length - 1].classList.add('last-visible');
                }
            }
        } catch (e) {
            // ignore
        }
    }

    goToPage(page) {
        if (page < 1 || page > this.totalPages || page === this.currentPage) {
            return;
        }
        this.currentPage = page;
        this.loadData();
        // 滚动到表格顶部
        const table = document.querySelector('.table-responsive');
        if (table) {
            table.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
    }
}

