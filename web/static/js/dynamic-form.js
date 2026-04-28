// 動態表單頁面 JavaScript

class DynamicForm {
    constructor(config) {
        this.config = config;
        this.isEdit = false;
        this.itemId = null;
        this.pageName = null;
        this.activeStores = []; // users 子表所需的店舖列表
        this.loadedItem = null;  // 保存已載入的數據（如 users 的 extra_fields）
        this.currentDraftId = null;
        this.hasUnsavedChanges = false;
        this.originalImageUrl = null; // 存儲原始圖片 URL（編輯時使用）
        this.saveTimer = null; // 草稿保存定時器
        this.isSubmitting = false; // 防止重複提交
        this.autoSaveDisabled = false; // 標記自動保存是否已禁用
        this._formInitializing = true; // 初始化期間禁止自動保存草稿（避免預設值觸發 change 導致假草稿）
        this.referrerQueryParams = this.captureReferrerParams(); // 保存來源 URL 的查詢參數
        this.fieldSettings = null; // 欄位設定（顯示/隱藏、順序、額外欄位）
        this._fieldSettingsDirty = false; // 用戶在 modal 中修改了 extraFields，防止背景 API 覆蓋
        this.init();
    }

    // 捕獲從列表頁帶來的查詢參數（如 source_type=dining）
    captureReferrerParams() {
        try {
            const params = new URLSearchParams(window.location.search || '');
            // 保存用於列表篩選的參數（排除 id, draft_id 等表單專用參數）
            const filterParams = new URLSearchParams();
            const excludeKeys = ['id', 'draft_id', 'edit', 'new'];
            for (const [key, value] of params.entries()) {
                if (!excludeKeys.includes(key)) {
                    filterParams.set(key, value);
                }
            }
            return filterParams.toString();
        } catch (e) {
            return '';
        }
    }

    async init() {
        if (!App.checkAuth()) { return; }

        // Wait for i18n translations to be loaded before rendering,
        // otherwise I18n.t() calls during render() return raw keys
        // instead of translated text (race condition on page refresh).
        if (typeof I18n !== 'undefined' && I18n.whenReady) {
            await I18n.whenReady(3000);
        }

        this.checkEditMode();
        this.detectPageName();
        await this.prepareVMarketProductFields();
        this.render();
        // Payment-methods: 這頁的互動邏輯必須「一定綁上」（避免任何時序問題導致沒反應）
        this.setupPaymentMethodsBehavior();
        // Logistics-companies: 國家/地區（多選）聯動清理
        this.setupLogisticsCompaniesBehavior();
        // Shipment-items: 初始化產品明細搜索和添加功能
        this.setupShipmentItemsBehavior();
        this.bindEvents();
        
        // 等待 DOM 更新完成，确保所有元素都已渲染
        await new Promise(resolve => setTimeout(resolve, 100));
        
        // 确保加载屏幕已显示（特别是对于产品表单，loadRelationOptions 可能很慢）
        // 在 loadRelationOptions 开始前显示，避免用户看到空白页面
        this.showLoadingOverlay();
        
        // 等待 DOM 更新和 relation options 載入完成
        // 对于产品表单，这可能需要较长时间（加载 product-types, brands, services 等）
        try {
            await this.loadRelationOptions();
        } catch (error) {
            console.error('加载关联选项失败:', error);
            // 即使失败也要隐藏加载屏幕
            this.hideLoadingOverlay();
            throw error;
        }

        // 初始化 users 的所屬店舖子表
        if (this.pageName === 'users') {
            await this.loadUserStoresSubTable();
        }
        
        if (this.isEdit && this.itemId) {
            await this.loadItemData();
        } else {
            // 新表單：先嘗試載入草稿（如果有的話，草稿中的編號會被保留）
            await this.loadDraft();
            // 然後載入已預留的編號（僅當沒有從草稿加載編號時）
            await this.loadReservedNumbers();
            // 從 URL 參數預填充字段
            this.populateFromURLParams();
            // 套用預設：相關人員=目前登入者、預設付款/收款帳戶
            await this.applyDefaultCurrentUserAndDefaultBankAccount();
            // 如果草稿載入後編號被清空，重新載入預留編號
            await this.ensureReservedNumbers();
            
            // 数据加载完成后立即隐藏 loading overlay，不等待 Select2 初始化
            // Select2 初始化可以在后台进行，不影响用户体验
            this.hideLoadingOverlay();
            
            // 在后台确保所有 Select2 字段已初始化（不阻塞 UI）
            // 1. 检查并初始化所有 Select2 字段
            const select2Fields = this.config.formFields.filter(f => f.type === 'select2' || f.type === 'select2-multi');
            const select2InitPromises = [];
            for (const field of select2Fields) {
                const fieldId = field._uniqueId || `field_${field.key}`;
                const input = document.getElementById(fieldId);
                if (input) {
                    const initPromise = (async () => {
                        // 等待 jQuery 和 Select2 加载
                        let retries = 0;
                        const maxRetries = 30;
                        while ((typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') && retries < maxRetries) {
                            await new Promise(resolve => setTimeout(resolve, 100));
                            retries++;
                        }
                        
                        if (typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') {
                            console.warn(`jQuery 或 Select2 未加载，无法初始化字段 ${field.key}`);
                            return;
                        }
                        
                        // 检查是否已经初始化
                        if ($(input).hasClass('select2-hidden-accessible')) {
                            return; // 已经初始化，跳过
                        }
                        
                        // 如果还没有初始化，实际调用 initSelect2 来初始化
                        try {
                            await this.initSelect2(field);
                        } catch (error) {
                            console.error(`初始化 Select2 字段失败 (${field.key}):`, error);
                        }
                    })();
                    select2InitPromises.push(initPromise);
                }
            }
            
            // 2. 在后台等待 Select2 初始化完成（不阻塞 UI）
            if (select2InitPromises.length > 0) {
                Promise.all(select2InitPromises).catch(err => {
                    console.warn('部分 Select2 字段初始化失败:', err);
                });
            }

            // 薪資表單：選擇員工時自動帶薪資，並預填供款 5%/5%
            if (this.pageName === 'payrolls') {
                const userSelect = document.getElementById('field_user_id');
                if (userSelect && typeof $ !== 'undefined') {
                    $(userSelect).on('select2:select', async (e) => {
                        const userId = e.params.data.id;
                        if (!userId) return;
                        try {
                            const user = await App.apiRequest(`/users/${userId}`);
                            if (user && user.salary != null) {
                                const salaryInput = document.getElementById('field_base_salary');
                                if (salaryInput) salaryInput.value = user.salary;
                            }
                        } catch (err) {
                            console.warn('載入員工薪資失敗', err);
                        }
                    });
                }
            }
        }  // end else (!this.isEdit)

        // 電話區號表單：選擇區碼時自動填充名稱
        if (this.isEdit && this.pageName === 'phone-country-codes') {
            setTimeout(() => {
                const codeSelect = document.getElementById('field_code');
                const nameInput = document.getElementById('field_name');
                if (codeSelect && nameInput) {
                    // 支持 Select2 和普通 select
                    const handleCodeChange = () => {
                        let selectedCode = '';
                        if (typeof $ !== 'undefined' && $(codeSelect).hasClass('select2-hidden-accessible')) {
                            selectedCode = $(codeSelect).val() || '';
                        } else {
                            selectedCode = codeSelect.value || '';
                        }
                        if (selectedCode && typeof COUNTRY_PHONE_CODES !== 'undefined' && COUNTRY_PHONE_CODES[selectedCode]) {
                            const englishName = COUNTRY_PHONE_CODES[selectedCode];
                            nameInput.dataset.submitValue = englishName;
                            if (typeof I18n !== 'undefined' && I18n.t) {
                                const k = `phoneCountryCodes.names.${selectedCode}`;
                                const translated = I18n.t(k);
                                nameInput.value = (translated && translated !== k) ? translated : englishName;
                            } else {
                                nameInput.value = englishName;
                            }
                        } else {
                            nameInput.value = '';
                            delete nameInput.dataset.submitValue;
                        }
                    };
                    // 使用 jQuery 监听 change 事件（支持 Select2）
                    if (typeof $ !== 'undefined') {
                        $(codeSelect).on('change', handleCodeChange);
                    } else {
                        codeSelect.addEventListener('change', handleCodeChange);
                    }
                }
            }, 500);
        }

        // dependency: 顯示/隱藏依賴字段（例如：users 的佣金率/佣金實額、expenses 的關聯欄位）
        this.setupFieldDependencies();
        this.applyFieldDependencies();

        // 應用欄位設定（隱藏/顯示、預設值）- 新建模式下套用
        // 使用獨立模塊處理，先從 API 載入最新設定，再應用到 DOM
        // 注意：這是 fire-and-forget（不 await），因為 loadFieldSettingsFromAPI 是異步 API 調用
        // 當 Promise resolve 時，setTimeout 已不在 Router 的 tracked patch 範圍內（_uninstallPatches 已恢復原生 setTimeout）
        // 因此必須：1) 檢查 this 是否仍是活躍實例 2) 將 timeout ID 存在實例上以便清理
        if (!this.isEdit) {
            console.log('[DynamicForm][DEBUG] 新建模式，開始載入欄位設定...');
            const self = this;
            this._fieldSettingsTimeoutIds = [];
            // 先從 API 載入最新的欄位設定（確保 defaultValue 正確）
            this.loadFieldSettingsFromAPI().then(() => {
                // 防護：如果此實例已被 SPA 導航銷毀，不要操作新頁面的 DOM
                if (window.dynamicForm !== self) {
                    console.log('[DynamicForm] 欄位設定回調：實例已過期（SPA 已導航離開），跳過');
                    return;
                }
                
                // ── 動態注入缺失的額外欄位 DOM 元素 ──
                // 當 API 回傳的 extraFields 在初次渲染時不在 cache 中，需要動態插入
                if (self.fieldSettings && self.fieldSettings.extraFields && self.fieldSettings.extraFields.length > 0) {
                    const baseFieldKeys = new Set(self.config.formFields.map(f => f.key));
                    const extraFields = self.fieldSettings.extraFields.filter(ef => !baseFieldKeys.has(ef.key));
                    
                    // Build field order map for visibility check
                    const fieldOrderMap = {};
                    if (self.fieldSettings.fields) {
                        self.fieldSettings.fields.forEach(sf => { fieldOrderMap[sf.key] = sf; });
                    }
                    
                    extraFields.forEach(ef => {
                        const sfSetting = fieldOrderMap[ef.key];
                        if (sfSetting && sfSetting.visible === false) return;
                        
                        const fieldId = `field_${ef.key}`;
                        if (document.getElementById(fieldId)) return; // Already in DOM
                        
                        // Render the field HTML and inject before form buttons
                        const fieldHtml = self.renderField(ef);
                        if (!fieldHtml) return;
                        
                        const buttonsContainer = document.querySelector('#dynamicForm .form-buttons-container');
                        if (buttonsContainer) {
                            const wrapper = document.createElement('div');
                            wrapper.setAttribute('data-extra-field-key', ef.key);
                            wrapper.innerHTML = fieldHtml;
                            buttonsContainer.parentNode.insertBefore(wrapper, buttonsContainer);
                            console.log(`[DynamicForm] 動態注入額外欄位: ${ef.key}`);
                        }
                    });
                    
                    // Re-initialize Select2 for any dynamically injected select-type extra fields
                    const selectExtraFields = extraFields.filter(ef => ef.type === 'select' || ef.type === 'select2');
                    if (selectExtraFields.length > 0) {
                        const t0 = setTimeout(async () => {
                            if (window.dynamicForm !== self) return;
                            for (const ef of selectExtraFields) {
                                const fieldId = `field_${ef.key}`;
                                const input = document.getElementById(fieldId);
                                if (input && typeof self.initSelect2 === 'function') {
                                    try {
                                        await self.initSelect2(ef);
                                    } catch (e) {
                                        console.warn(`[DynamicForm] initSelect2 failed for extra field ${ef.key}:`, e);
                                    }
                                }
                            }
                        }, 100);
                        self._fieldSettingsTimeoutIds.push(t0);
                    }
                }
                
                // API 載入完成後，等待 DOM 穩定再應用
                const t1 = setTimeout(() => {
                    if (window.dynamicForm !== self) return; // 再次檢查
                    if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.applyFieldSettingsToDOM) {
                        FormFieldSettingsHandler.applyFieldSettingsToDOM(self);
                    }
                }, 300);
                // 針對 Select2 欄位，需要更長的延遲
                const t2 = setTimeout(() => {
                    if (window.dynamicForm !== self) return; // 再次檢查
                    if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.applyDefaultValuesAfterSelect2Init) {
                        FormFieldSettingsHandler.applyDefaultValuesAfterSelect2Init(self);
                    }
                }, 900);
                self._fieldSettingsTimeoutIds.push(t1, t2);
            }).catch(err => {
                console.warn('[DynamicForm] 載入欄位設定失敗，使用預設:', err);
                if (window.dynamicForm !== self) return; // 防護
                // 即使 API 失敗也嘗試用快取的設定
                const t3 = setTimeout(() => {
                    if (window.dynamicForm !== self) return;
                    if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.applyFieldSettingsToDOM) {
                        FormFieldSettingsHandler.applyFieldSettingsToDOM(self);
                    }
                }, 300);
                self._fieldSettingsTimeoutIds.push(t3);
            });
        } else {
            // 編輯模式：也需要從 API 載入最新欄位設定，動態注入缺失的額外欄位
            const self = this;
            this._fieldSettingsTimeoutIds = [];
            this.loadFieldSettingsFromAPI().then(() => {
                if (window.dynamicForm !== self) return;
                
                // 動態注入缺失的額外欄位 DOM 元素
                if (self.fieldSettings && self.fieldSettings.extraFields && self.fieldSettings.extraFields.length > 0) {
                    const baseFieldKeys = new Set(self.config.formFields.map(f => f.key));
                    const extraFields = self.fieldSettings.extraFields.filter(ef => !baseFieldKeys.has(ef.key));
                    
                    const fieldOrderMap = {};
                    if (self.fieldSettings.fields) {
                        self.fieldSettings.fields.forEach(sf => { fieldOrderMap[sf.key] = sf; });
                    }
                    
                    let injected = false;
                    extraFields.forEach(ef => {
                        const sfSetting = fieldOrderMap[ef.key];
                        if (sfSetting && sfSetting.visible === false) return;
                        
                        const fieldId = `field_${ef.key}`;
                        if (document.getElementById(fieldId)) return; // Already in DOM
                        
                        const fieldHtml = self.renderField(ef);
                        if (!fieldHtml) return;
                        
                        const buttonsContainer = document.querySelector('#dynamicForm .form-buttons-container');
                        if (buttonsContainer) {
                            const wrapper = document.createElement('div');
                            wrapper.setAttribute('data-extra-field-key', ef.key);
                            wrapper.innerHTML = fieldHtml;
                            buttonsContainer.parentNode.insertBefore(wrapper, buttonsContainer);
                            injected = true;
                            console.log(`[DynamicForm][Edit] 動態注入額外欄位: ${ef.key}`);
                        }
                    });
                    
                    // Re-initialize Select2 for dynamically injected select-type extra fields
                    const selectExtraFields = extraFields.filter(ef => ef.type === 'select' || ef.type === 'select2');
                    if (selectExtraFields.length > 0) {
                        const t0 = setTimeout(async () => {
                            if (window.dynamicForm !== self) return;
                            for (const ef of selectExtraFields) {
                                const fieldId = `field_${ef.key}`;
                                const input = document.getElementById(fieldId);
                                if (input && typeof self.initSelect2 === 'function') {
                                    try {
                                        await self.initSelect2(ef);
                                    } catch (e) {
                                        console.warn(`[DynamicForm] initSelect2 failed for extra field ${ef.key}:`, e);
                                    }
                                }
                            }
                        }, 100);
                        self._fieldSettingsTimeoutIds.push(t0);
                    }
                    
                    // 如果有注入新欄位且有 loadedItem，填充額外欄位的值
                    if (injected && self.loadedItem && self.loadedItem.extra_fields) {
                        const t1 = setTimeout(() => {
                            if (window.dynamicForm !== self) return;
                            extraFields.forEach(ef => {
                                const fieldId = `field_${ef.key}`;
                                const input = document.getElementById(fieldId);
                                if (!input) return;
                                
                                const value = self.loadedItem.extra_fields[ef.key];
                                if (value === undefined || value === null) return;
                                
                                if (ef.type === 'checkbox') {
                                    input.checked = value === true || value === 'true' || value === 1 || value === '1';
                                } else if (ef.type === 'select' && typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                                    $(input).val(String(value)).trigger('change');
                                } else if (ef.type === 'select') {
                                    input.value = String(value);
                                } else if (ef.type === 'number') {
                                    input.value = value;
                                } else if (ef.type === 'date' && typeof value === 'string') {
                                    const date = new Date(value);
                                    if (!isNaN(date.getTime())) {
                                        const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
                                        input.value = localDate.toISOString().slice(0, 10);
                                    } else {
                                        input.value = value;
                                    }
                                } else if (ef.type === 'datetime-local' && typeof value === 'string') {
                                    const date = new Date(value);
                                    if (!isNaN(date.getTime())) {
                                        const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
                                        input.value = localDate.toISOString().slice(0, 16);
                                    } else {
                                        input.value = value;
                                    }
                                } else {
                                    input.value = String(value);
                                }
                            });
                        }, 200);
                        self._fieldSettingsTimeoutIds.push(t1);
                    }
                }
                
                // 編輯模式也應用欄位可見性設定
                const t2 = setTimeout(() => {
                    if (window.dynamicForm !== self) return;
                    if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.applyFieldSettingsToDOM) {
                        FormFieldSettingsHandler.applyFieldSettingsToDOM(self);
                    }
                }, 300);
                self._fieldSettingsTimeoutIds.push(t2);
            }).catch(err => {
                console.warn('[DynamicForm][Edit] 載入欄位設定失敗:', err);
            });
        }

        // 設置自動保存
        this.setupAutoSave();
        
        // 新建模式：初始化草稿指示器
        if (!this.isEdit) {
            setTimeout(() => {
                this.updateDraftIndicator();
            }, 200);
            
            // 初始化完成後才允許自動保存（延遲 800ms，等待所有預設值 trigger 完成）
            // 避免 phone_country_code、member_level 等初始預設值的 trigger('change') 誤觸發草稿儲存
            setTimeout(() => {
                this._formInitializing = false;
            }, 800);
        }
        
        // 新建模式：確保按鈕欄一定會顯示
        // 注意：hideLoadingOverlay() 已經會在 150ms 後調用 setupFormButtonBar()
        // 但為了確保按鈕欄一定會顯示，我們在這裡也設置多個備用調用（使用 forceShow）
        if (!this.isEdit) {
             // 這些 setTimeout 會調用 setupFormButtonBar(true)，這會強制重新創建按鈕欄
             // 如果 cms_layout.html 中的 setupFormButtonBar 邏輯是 "如果有就清空並重新加"，這可能導致閃爍
             // 特別是如果兩者在短時間內連續觸發。
             // 既然我們已經在 cms_layout.html 中使用了 MutationObserver，這裡的多次嘗試可能造成干擾
             // 讓我們減少這些嘗試，或者只保留一個延遲較長的，作為最後的保障
            
            // 延遲到 600ms，避開 observer 的 500ms
            setTimeout(() => {
                if (typeof setupFormButtonBar === 'function') {
                    const buttonBar = document.querySelector('.form-button-bar');
                    // 只有當按鈕欄真的不存在或者隱藏時才嘗試修復
                    if (!buttonBar || !buttonBar.classList.contains('show') || buttonBar.style.display === 'none') {
                        console.log('setupFormButtonBar: 保障機制觸發設置按鈕欄');
                        setupFormButtonBar(true);
                    }
                }
            }, 600);
        }
    }

    // Payment methods：確保 payment_type/name/is_online_payment + gateway 欄位的互動一定會生效
    setupPaymentMethodsBehavior() {
        if (this.pageName !== 'payment-methods') return;
        if (this._paymentMethodsBehaviorReady) return;

        const tryBind = () => {
            const paymentTypeField = document.getElementById('field_payment_type');
            if (!paymentTypeField) return false;

            if (paymentTypeField.dataset && paymentTypeField.dataset.pmBound === '1') {
                // 已綁定，但仍補一次初始化，確保畫面狀態正確
                try { this.handlePaymentTypeChange(); } catch (e) { console.warn('handlePaymentTypeChange failed', e); }
                this._paymentMethodsBehaviorReady = true;
                return true;
            }

            // 初始化 + 綁定
            try { this.handlePaymentTypeChange(); } catch (e) { console.warn('handlePaymentTypeChange failed', e); }
            paymentTypeField.addEventListener('change', () => {
                try { this.handlePaymentTypeChange(); } catch (e) { console.warn('handlePaymentTypeChange failed', e); }
            });

            if (paymentTypeField.dataset) paymentTypeField.dataset.pmBound = '1';
            this._paymentMethodsBehaviorReady = true;
            return true;
        };

        if (tryBind()) return;

        // 後備：少量重試（某些頁面可能因 DOM 尚未 ready 而取不到欄位）
        let attempts = 0;
        const maxAttempts = 30;
        const timer = setInterval(() => {
            attempts++;
            if (tryBind() || attempts >= maxAttempts) {
                clearInterval(timer);
            }
        }, 100);
    }

    setupLogisticsCompaniesBehavior() {
        if (this.pageName !== 'logistics-companies') return;
        const tryBind = (retries = 0) => {
            const countryEl = document.getElementById('field_allowed_country_codes');
            const regionEl = document.getElementById('field_allowed_region_keys');
            if (!countryEl || !regionEl) {
                if (retries < 20) setTimeout(() => tryBind(retries + 1), 100);
                return;
            }

            const clearRegions = () => {
                if (typeof $ !== 'undefined' && $(regionEl).hasClass('select2-hidden-accessible')) {
                    $(regionEl).val(null).trigger('change');
                } else {
                    Array.from(regionEl.options || []).forEach(o => (o.selected = false));
                }
            };

            // 避免重複綁定
            if (countryEl.dataset._lcBound === '1') return;
            countryEl.dataset._lcBound = '1';

            if (typeof $ !== 'undefined' && $(countryEl).hasClass('select2-hidden-accessible')) {
                $(countryEl).on('change', () => clearRegions());
            } else {
                countryEl.addEventListener('change', () => clearRegions());
            }
        };
        tryBind();
    }

    // Shipment-items 搜索和添加功能
    setupShipmentItemsBehavior() {
        // 延遲執行，確保 DOM 已渲染
        setTimeout(async () => {
            const itemsField = this.config.formFields.find(f => f.type === 'shipment-items');
            if (!itemsField) return;

            const fieldId = `field_${itemsField.key}`;
            const addBtn = document.getElementById(`${fieldId}_add_btn`);
            const tableBody = document.getElementById(`${fieldId}_table`);
            const hiddenInput = document.getElementById(fieldId);

            if (!tableBody || !hiddenInput) return;

            // 存儲當前項目和產品列表
            this.shipmentItems = this.shipmentItems || [];
            this.shipmentProducts = [];
            
            // 載入產品列表
            try {
                const products = await App.apiRequest('/products?limit=500');
                this.shipmentProducts = (products && products.data) ? products.data : (Array.isArray(products) ? products : []);
            } catch (error) {
                console.error('載入產品列表失敗:', error);
            }

            // 點擊添加產品按鈕
            if (addBtn) {
                addBtn.addEventListener('click', () => {
                    this.addShipmentItemRow(fieldId);
                });
            }
        }, 200);
    }

    // 添加產品行（使用下拉選單）
    addShipmentItemRow(fieldId) {
        const tableBody = document.getElementById(`${fieldId}_table`);
        const hiddenInput = document.getElementById(fieldId);
        if (!tableBody || !hiddenInput) return;

        // 移除空白提示行
        const emptyRow = tableBody.querySelector('.shipment-item-empty');
        if (emptyRow) emptyRow.remove();

        // 添加到內部數據
        const itemIndex = this.shipmentItems.length;
        const newItem = {
            product_id: '',
            product_name: '',
            sku: '',
            quantity: 1
        };
        this.shipmentItems.push(newItem);

        // 創建表格行
        const row = document.createElement('tr');
        row.dataset.index = itemIndex;
        
        // 構建產品選項
        const productOptions = this.shipmentProducts.map(p => 
            `<option value="${p.id}">${this.escapeHtml(p.name)}${p.sku ? ` (${p.sku})` : ''}</option>`
        ).join('');
        
        row.innerHTML = `
            <td>
                <select class="form-select form-select-sm shipment-product-select" data-index="${itemIndex}">
                    <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('shipmentsPage.selectProduct') : '請選擇產品'}</option>
                    ${productOptions}
                </select>
            </td>
            <td>
                <input type="number" class="form-control form-control-sm text-end" value="1" data-field="quantity" min="1" step="1">
            </td>
            <td class="text-center">
                <button type="button" class="btn btn-sm btn-outline-danger" title="${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.delete') : '刪除'}">
                    <i class="bi bi-trash"></i>
                </button>
            </td>
        `;

        // 綁定產品選擇事件
        const productSelect = row.querySelector('.shipment-product-select');
        productSelect.addEventListener('change', () => {
            const productId = productSelect.value;
            const product = this.shipmentProducts.find(p => p.id === productId);
            if (product) {
                newItem.product_id = product.id;
                newItem.product_name = product.name;
                newItem.sku = product.sku || '';
            } else {
                newItem.product_id = '';
                newItem.product_name = '';
                newItem.sku = '';
            }
            this.syncShipmentItemsToHidden(fieldId);
            this.hasUnsavedChanges = true;
        });

        // 綁定數量輸入事件
        const quantityInput = row.querySelector('input[data-field="quantity"]');
        quantityInput.addEventListener('input', () => {
            newItem.quantity = parseInt(quantityInput.value) || 1;
            this.syncShipmentItemsToHidden(fieldId);
            this.hasUnsavedChanges = true;
        });

        // 綁定刪除按鈕
        row.querySelector('button').addEventListener('click', () => {
            this.removeShipmentItemRow(row, fieldId);
        });

        tableBody.appendChild(row);
        
        // 初始化 Select2（如果可用）
        if (typeof $ !== 'undefined' && $.fn.select2) {
            $(productSelect).select2({
                placeholder: (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('shipmentsPage.selectProduct') : '請選擇產品',
                allowClear: true,
                width: '100%'
            }).on('change', function() {
                const productId = $(this).val();
                const product = window.dynamicForm.shipmentProducts.find(p => p.id === productId);
                if (product) {
                    newItem.product_id = product.id;
                    newItem.product_name = product.name;
                    newItem.sku = product.sku || '';
                } else {
                    newItem.product_id = '';
                    newItem.product_name = '';
                    newItem.sku = '';
                }
                window.dynamicForm.syncShipmentItemsToHidden(fieldId);
                window.dynamicForm.hasUnsavedChanges = true;
            });
        }

        this.syncShipmentItemsToHidden(fieldId);
        this.hasUnsavedChanges = true;
    }

    // 刪除產品行
    removeShipmentItemRow(row, fieldId) {
        const index = parseInt(row.dataset.index);
        if (isNaN(index)) return;

        // 銷毀 Select2（如果有）
        const select = row.querySelector('.shipment-product-select');
        if (select && typeof $ !== 'undefined' && $(select).hasClass('select2-hidden-accessible')) {
            $(select).select2('destroy');
        }

        // 從數組中移除
        this.shipmentItems.splice(index, 1);
        row.remove();

        // 重新索引
        const tableBody = document.getElementById(`${fieldId}_table`);
        if (tableBody) {
            Array.from(tableBody.querySelectorAll('tr[data-index]')).forEach((tr, i) => {
                tr.dataset.index = i;
                const sel = tr.querySelector('.shipment-product-select');
                if (sel) sel.dataset.index = i;
            });
        }

        // 如果沒有項目，顯示空白提示
        if (this.shipmentItems.length === 0) {
            tableBody.innerHTML = `<tr class="shipment-item-empty"><td colspan="3" class="text-center text-muted">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.noData') : '暫無資料'}</td></tr>`;
        }

        this.syncShipmentItemsToHidden(fieldId);
        this.hasUnsavedChanges = true;
    }

    // 同步數據到隱藏字段
    syncShipmentItemsToHidden(fieldId) {
        const hiddenInput = document.getElementById(fieldId);
        if (hiddenInput) {
            hiddenInput.value = JSON.stringify(this.shipmentItems);
        }
    }

    // HTML 轉義
    escapeHtml(str) {
        if (!str) return '';
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    // 載入現有的 shipment items（編輯模式）
    async loadExistingShipmentItems(items, fieldId) {
        const tableBody = document.getElementById(`${fieldId}_table`);
        if (!tableBody) return;

        // 確保產品列表已載入
        if (!this.shipmentProducts || this.shipmentProducts.length === 0) {
            try {
                const products = await App.apiRequest('/products?limit=500');
                this.shipmentProducts = (products && products.data) ? products.data : (Array.isArray(products) ? products : []);
            } catch (error) {
                console.error('載入產品列表失敗:', error);
            }
        }

        // 初始化 shipmentItems 數組
        this.shipmentItems = [];

        // 移除空白提示行
        const emptyRow = tableBody.querySelector('.shipment-item-empty');
        if (emptyRow) emptyRow.remove();

        // 為每個現有項目添加行
        items.forEach((it, idx) => {
            const itemData = {
                product_id: it.product_id || '',
                product_name: it.product_name || it.name || it.title || '',
                sku: it.sku || it.product_sku || '',
                quantity: (it.quantity !== undefined && it.quantity !== null) ? parseInt(it.quantity) : (it.qty || 1)
            };
            
            this.shipmentItems.push(itemData);

            // 創建表格行
            const row = document.createElement('tr');
            row.dataset.index = idx;
            
            // 構建產品選項
            const productOptions = this.shipmentProducts.map(p => {
                const selected = (p.id === itemData.product_id) ? 'selected' : '';
                return `<option value="${p.id}" ${selected}>${this.escapeHtml(p.name)}${p.sku ? ` (${p.sku})` : ''}</option>`;
            }).join('');
            
            row.innerHTML = `
                <td>
                    <select class="form-select form-select-sm shipment-product-select" data-index="${idx}">
                        <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('shipmentsPage.selectProduct') : '請選擇產品'}</option>
                        ${productOptions}
                    </select>
                </td>
                <td>
                    <input type="number" class="form-control form-control-sm text-end" value="${itemData.quantity}" data-field="quantity" min="1" step="1">
                </td>
                <td class="text-center">
                    <button type="button" class="btn btn-sm btn-outline-danger" title="${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.delete') : '刪除'}">
                        <i class="bi bi-trash"></i>
                    </button>
                </td>
            `;

            // 綁定產品選擇事件
            const productSelect = row.querySelector('.shipment-product-select');
            productSelect.addEventListener('change', () => {
                const productId = productSelect.value;
                const product = this.shipmentProducts.find(p => p.id === productId);
                if (product) {
                    itemData.product_id = product.id;
                    itemData.product_name = product.name;
                    itemData.sku = product.sku || '';
                } else {
                    itemData.product_id = '';
                    itemData.product_name = '';
                    itemData.sku = '';
                }
                this.syncShipmentItemsToHidden(fieldId);
                this.hasUnsavedChanges = true;
            });

            // 綁定數量輸入事件
            const quantityInput = row.querySelector('input[data-field="quantity"]');
            quantityInput.addEventListener('input', () => {
                itemData.quantity = parseInt(quantityInput.value) || 1;
                this.syncShipmentItemsToHidden(fieldId);
                this.hasUnsavedChanges = true;
            });

            // 綁定刪除按鈕
            row.querySelector('button').addEventListener('click', () => {
                this.removeShipmentItemRow(row, fieldId);
            });

            tableBody.appendChild(row);
            
            // 初始化 Select2（如果可用）
            if (typeof $ !== 'undefined' && $.fn.select2) {
                $(productSelect).select2({
                    placeholder: (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('shipmentsPage.selectProduct') : '請選擇產品',
                    allowClear: true,
                    width: '100%'
                }).on('change', function() {
                    const productId = $(this).val();
                    const product = window.dynamicForm.shipmentProducts.find(p => p.id === productId);
                    if (product) {
                        itemData.product_id = product.id;
                        itemData.product_name = product.name;
                        itemData.sku = product.sku || '';
                    } else {
                        itemData.product_id = '';
                        itemData.product_name = '';
                        itemData.sku = '';
                    }
                    window.dynamicForm.syncShipmentItemsToHidden(fieldId);
                    window.dynamicForm.hasUnsavedChanges = true;
                });
            }
        });

        this.syncShipmentItemsToHidden(fieldId);
    }

    // ===== dependency (show/hide fields) =====
    setupFieldDependencies() {
        try {
            if (!this.config || !Array.isArray(this.config.formFields)) return;
            // 綁定一次即可
            if (this._depsBound) return;
            this._depsBound = true;

            const deps = this.config.formFields.filter(f => f && f.dependency && f.dependency.field);
            const watched = new Set();
            deps.forEach(f => watched.add(String(f.dependency.field)));

            watched.forEach(depKey => {
                const depEl = document.getElementById(`field_${depKey}`);
                if (!depEl) return;
                const handler = () => {
                    // 延迟一点确保值已更新
                    setTimeout(() => {
                        this.applyFieldDependencies();
                    }, 50);
                };

                // Select2 也會觸發 change，但保險起見加 select2 event
                depEl.addEventListener('change', handler);
                if (typeof $ !== 'undefined' && $(depEl).hasClass('select2-hidden-accessible')) {
                    $(depEl).on('select2:select select2:clear', handler);
                }
            });
        } catch (e) {
            console.warn('setupFieldDependencies failed', e);
        }
    }

    applyFieldDependencies() {
        try {
            if (!this.config || !Array.isArray(this.config.formFields)) return;
            const deps = this.config.formFields.filter(f => f && f.dependency && f.dependency.field);

            const getValue = (key) => {
                const el = document.getElementById(`field_${key}`);
                if (!el) return '';
                if (el.type === 'checkbox') return el.checked ? 'true' : 'false';
                // Select2 uses the underlying select value
                if (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) {
                    const v = $(el).val();
                    if (Array.isArray(v)) return v.map(String);
                    return (v == null) ? '' : String(v);
                }
                if (el.tagName === 'SELECT' && el.multiple) {
                    return Array.from(el.selectedOptions).map(o => String(o.value));
                }
                return (el.value == null) ? '' : String(el.value);
            };

            const setContainerVisible = (fieldKey, visible) => {
                const container = document.getElementById(`field_container_${fieldKey}`);
                if (!container) return;
                container.style.display = visible ? '' : 'none';

                // hidden 時 disable 內部控件，避免 required/提交問題
                const controls = container.querySelectorAll('input, select, textarea, button');
                controls.forEach(ctrl => {
                    // 不要 disable label/button bar
                    if (ctrl && ctrl.id && ctrl.id.endsWith('_uploadBtn')) return;
                    if (ctrl && ctrl.id && ctrl.id.endsWith('_removeBtn')) return;
                    if (ctrl && ctrl.type === 'button') return;
                    ctrl.disabled = !visible;
                });
            };

            deps.forEach(f => {
                const dep = f.dependency;
                const cur = getValue(dep.field);
                // 支持 dep.value (单数) 或 dep.values (复数)
                const allowed = dep.values ? dep.values.map(String) : (dep.value ? [String(dep.value)] : []);
                let ok = false;

                // 特殊处理：address_region_code 字段始终显示（不依赖国家字段的值）
                if (f.key === 'address_region_code' && dep.field === 'address_country_code') {
                    // 地区字段始终显示，不依赖国家字段的值
                    ok = true;
                } else if (allowed.length === 0) {
                    // 如果没有指定允许的值，只要有值就显示
                    if (Array.isArray(cur)) {
                        ok = cur.length > 0 && cur.some(v => v !== '' && v != null);
                    } else {
                        ok = cur !== '' && cur != null && cur !== undefined;
                    }
                } else {
                    // 正常逻辑：检查值是否在允许列表中
                    if (Array.isArray(cur)) {
                        ok = cur.some(v => allowed.includes(String(v)));
                    } else {
                        ok = allowed.includes(String(cur));
                    }
                }
                
                // 為多個相同 key 的字段生成唯一容器 ID
                let containerId = `field_container_${f.key}`;
                if (f.key === 'reference_id' && f.dependency && f.dependency.values && f.dependency.values.length > 0) {
                    const depValue = f.dependency.values[0];
                    containerId = `field_container_${f.key}_${depValue}`;
                }
                
                const container = document.getElementById(containerId);
                if (container) {
                    container.style.display = ok ? '' : 'none';
                    // hidden 時 disable 內部控件，避免 required/提交問題
                    const controls = container.querySelectorAll('input, select, textarea, button');
                    controls.forEach(ctrl => {
                        // 不要 disable label/button bar
                        if (ctrl && ctrl.id && ctrl.id.endsWith('_uploadBtn')) return;
                        if (ctrl && ctrl.id && ctrl.id.endsWith('_removeBtn')) return;
                        if (ctrl && ctrl.type === 'button') return;
                        ctrl.disabled = !ok;
                        
                        // 如果是 Select2，需要重新啟用
                        if (ok && typeof $ !== 'undefined' && $(ctrl).hasClass('select2-hidden-accessible')) {
                            $(ctrl).prop('disabled', false);
                        }
                    });
                    
                    // 同時處理父級 row 容器的顯示/隱藏
                    // 找到包含此容器的 row（結構：row > col > field_container）
                    const colParent = container.closest('.col-12, .col-md-6');
                    if (colParent) {
                        const rowParent = colParent.closest('.row');
                        if (rowParent) {
                            // 檢查該 row 內所有的 field_container 是否都被隱藏
                            const allContainers = rowParent.querySelectorAll('[id^="field_container_"]');
                            const allHidden = Array.from(allContainers).every(c => c.style.display === 'none');
                            rowParent.style.display = allHidden ? 'none' : '';
                        }
                    }
                    
                    // 如果是 service_package_service_id 字段且应该显示，确保 Select2 初始化
                    if (ok && f.key === 'service_package_service_id' && (f.type === 'select2' || f.type === 'select2-multi')) {
                        // 延迟初始化，确保 DOM 已更新
                        setTimeout(async () => {
                            const select = document.getElementById(`field_${f.key}`);
                            if (select && container.style.display !== 'none') {
                                // 如果字段显示，确保 Select2 已初始化
                                if (typeof $ !== 'undefined' && !$(select).hasClass('select2-hidden-accessible')) {
                                    try {
                                        await this.loadRelationFieldOptions(f);
                                        await this.initSelect2(f);
                                    } catch (error) {
                                        console.error('初始化 service_package_service_id Select2 失敗:', error);
                                    }
                                }
                            }
                        }, 200);
                    }
                    
                    // 通用：dependency 顯示的 select2 靜態選項字段，延遲初始化 Select2
                    if (ok && (f.type === 'select2' || f.type === 'select2-multi') && f.options && !f.relationApi) {
                        setTimeout(() => {
                            const select = document.getElementById(`field_${f.key}`);
                            if (select && container.style.display !== 'none') {
                                if (typeof $ !== 'undefined' && !$(select).hasClass('select2-hidden-accessible')) {
                                    try {
                                        $(select).select2({ theme: 'bootstrap-5', width: '100%', allowClear: true, placeholder: (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇' });
                                    } catch (error) {
                                        console.warn('初始化靜態 Select2 失敗:', error);
                                    }
                                }
                            }
                        }, 200);
                    }
                }
            });
        } catch (e) {
            console.warn('applyFieldDependencies failed', e);
        }
    }

    checkEditMode() {
        const pathParts = window.location.pathname.split('/').filter(p => p);
        // 例如: /suppliers/2903149e-b217-4705-a5fb-4c0691ca7a9b/edit
        // pathParts = ['suppliers', '2903149e-b217-4705-a5fb-4c0691ca7a9b', 'edit']
        this.isEdit = pathParts.length === 3 && pathParts[2] === 'edit';
        this.itemId = this.isEdit ? pathParts[1] : null;
    }

    detectPageName() {
        const pathParts = window.location.pathname.split('/').filter(p => p);
        // 例如 /customers/new -> customers, /orders/123/edit -> orders
        if (pathParts.length >= 1) {
            this.pageName = pathParts[0];
        }
    }

    async prepareVMarketProductFields() {
        if (this.pageName !== 'products' && this.pageName !== 'services') return;
        try {
            const resp = await App.apiRequest('/api/v1/tenant/vmarket-settings');
            const joined = !!(resp && resp.data && resp.data.vmarket_joined);
            this.vmarketJoined = joined;
            if (this.config && Array.isArray(this.config.formFields)) {
                if (!joined) {
                    // 如果未加入 VMarket，隱藏 VMarket 相關選項
                    this.config.formFields = this.config.formFields.filter(field => field.key !== 'show_on_vmarket' && !(field.key === 'category' && field.dependency && field.dependency.field === 'show_on_vmarket'));
                } else {
                    // 已加入 VMarket，預設開啟顯示
                    const vmField = this.config.formFields.find(field => field.key === 'show_on_vmarket');
                    if (vmField) {
                        vmField.defaultValue = true;
                    }
                }
            }
        } catch (error) {
            console.warn('載入 VMarket 狀態失敗，沿用預設欄位設定', error);
            this.vmarketJoined = false;
        }
    }
    
    // 將頁面名稱轉換為翻譯鍵（將連字符轉為駝峰命名）
    getTranslationKey() {
        if (!this.pageName) return '';
        // 將 member-levels 轉為 memberLevels, expense-requests 轉為 expenseRequests
        return this.pageName.replace(/-([a-z])/g, (match, letter) => letter.toUpperCase());
    }

    // 欄位設定權限檢查（無權限則不顯示設定按鈕）
    canConfigureFieldSettings() {
        try {
            const raw = localStorage.getItem('user');
            if (!raw) return true;
            const user = JSON.parse(raw);
            const perms = [];
            const normalize = (v) => String(v || '').trim().toLowerCase().replace(/_/g, '-');
            const pushPerm = (v) => {
                const k = normalize(v);
                if (k) perms.push(k);
            };
            const collect = (src) => {
                if (!src) return;
                if (Array.isArray(src)) {
                    src.forEach(pushPerm);
                    return;
                }
                if (typeof src === 'string') {
                    try {
                        const parsed = JSON.parse(src);
                        collect(parsed);
                        return;
                    } catch (_e) {
                        pushPerm(src);
                        return;
                    }
                }
                if (typeof src === 'object') {
                    Object.keys(src).forEach((k) => {
                        const val = src[k];
                        if (val === true || val === 1 || val === '1') {
                            pushPerm(k);
                        } else {
                            pushPerm(val);
                        }
                    });
                }
            };

            collect(user && user.level && user.level.permissions);
            collect(user && user.role && user.role.permissions);

            const uniquePerms = Array.from(new Set(perms)).filter(Boolean);
            if (uniquePerms.length === 0) return true;
            return uniquePerms.includes('field-settings');
        } catch (e) {
            return true;
        }
    }

    // 翻譯選項 label：
    // - opt.labelKey（完全自定義）
    // - options.<page>.<field>.<value>（同一個 value 在不同頁可以不同翻譯）
    // - options.<field>.<value>（通用翻譯）
    getOptionLabel(fieldKey, opt) {
        if (!opt) return '';

        const rawLabel = opt.label != null ? String(opt.label) : '';
        const rawValue = opt.value != null ? String(opt.value) : '';

        if (typeof I18n === 'undefined' || !I18n.t) {
            return rawLabel || rawValue;
        }

        if (opt.labelKey) {
            const t = I18n.t(opt.labelKey);
            if (t && t !== opt.labelKey) return t;
        }

        const pageKey = this.pageName || this.getTranslationKey() || '';
        if (pageKey && fieldKey && rawValue) {
            const k1 = `options.${pageKey}.${fieldKey}.${rawValue}`;
            const t1 = I18n.t(k1);
            if (t1 && t1 !== k1) return t1;
        }

        if (fieldKey && rawValue) {
            const k2 = `options.${fieldKey}.${rawValue}`;
            const t2 = I18n.t(k2);
            if (t2 && t2 !== k2) return t2;
        }

        // 布林通用翻譯：options.boolean.true/false
        if (rawValue === 'true' || rawValue === 'false' || rawValue === '1' || rawValue === '0') {
            const normalized = (rawValue === '1') ? 'true' : (rawValue === '0' ? 'false' : rawValue);
            const k3 = `options.boolean.${normalized}`;
            const t3 = I18n.t(k3);
            if (t3 && t3 !== k3) return t3;
        }

        return rawLabel || rawValue;
    }

    // 翻譯 placeholder（支援 placeholders.<page>.<field> / placeholders.<field>）
    getPlaceholder(fieldKey, rawPlaceholder) {
        const fallback = rawPlaceholder != null ? String(rawPlaceholder) : '';
        if (!fallback) return '';
        if (typeof I18n === 'undefined' || !I18n.t) return fallback;

        const pageKey = this.pageName || this.getTranslationKey() || '';
        if (pageKey && fieldKey) {
            const k1 = `placeholders.${pageKey}.${fieldKey}`;
            const t1 = I18n.t(k1);
            if (t1 && t1 !== k1) return t1;
        }
        if (fieldKey) {
            const k2 = `placeholders.${fieldKey}`;
            const t2 = I18n.t(k2);
            if (t2 && t2 !== k2) return t2;
        }
        // 若沒有對應 placeholder 翻譯，且 fallback 看起來是中文，則在英文語系時避免顯示中文
        // 改用 "Please select ..." 的統一提示
        try {
            if (I18n && I18n.currentLang === 'en' && /[\u3400-\u9FFF]/.test(fallback)) {
                const selectKey = 'common.pleaseSelect';
                const selectText = (typeof I18n !== 'undefined' && I18n.t && I18n.t(selectKey) !== selectKey) ? I18n.t(selectKey) : 'Please select';
                return selectText;
            }
        } catch (e) {
            // ignore
        }
        return fallback;
    }

    // 翻譯欄位 label：預設優先用 field.key -> fields.<key>，再退回用 label（含中文 label）
    // 但某些頁面（例如 payment-methods 的 is_default）同一個 key 在不同資源代表不同語意，
    // 這時可在 config.formFields 裡設置 preferLabel=true 來跳過 fields.<key> 覆蓋。
    getFieldLabel(field) {
        const rawLabel = field && field.label != null ? String(field.label) : '';
        const rawKey = field && field.key != null ? String(field.key) : '';
        if (typeof I18n === 'undefined' || !I18n.t) return rawLabel;

        // 優先使用 field.labelKey（頁面特定翻譯鍵，如 stampSettingsPage.fields.productStampEnabled）
        if (field && field.labelKey) {
            try {
                const tLabel = I18n.t(field.labelKey);
                if (tLabel && tLabel !== field.labelKey) return tLabel;
            } catch (e) {
                // ignore
            }
        }

        // 若 field.label 本身是 i18n key（例如 paymentMethods.defaultCustomer），直接翻譯
        if (rawLabel) {
            try {
                if (rawLabel.includes('.') && !/[\u3400-\u9FFF]/.test(rawLabel)) {
                    const tLabel = I18n.t(rawLabel);
                    if (tLabel && tLabel !== rawLabel) return tLabel;
                }
            } catch (e) {
                // ignore
            }
        }

        const preferLabel = !!(field && (field.preferLabel || field.disableKeyTranslation));

        // 1) 先用 key 查 fields.<key>（最穩）
        if (!preferLabel && rawKey) {
            // 關係 key（如 user.name）：嘗試 camelCase、第一段、最後一段
            if (rawKey.includes('.')) {
                const parts = rawKey.split('.');
                const camelCaseKey = parts.map((p, i) => i === 0 ? p : p.charAt(0).toUpperCase() + p.slice(1)).join('');
                const k0 = `fields.${camelCaseKey}`;
                const t0 = I18n.t(k0);
                if (t0 && t0 !== k0) return t0;

                const firstPart = parts[0];
                const firstPartCamel = firstPart.split('_').map((p, i) => i === 0 ? p : p.charAt(0).toUpperCase() + p.slice(1)).join('');
                const k1 = `fields.${firstPartCamel}`;
                const t1 = I18n.t(k1);
                if (t1 && t1 !== k1) return t1;

                const lastPart = parts[parts.length - 1];
                const k2 = `fields.${lastPart}`;
                const t2 = I18n.t(k2);
                if (t2 && t2 !== k2) return t2;
            }

            const k3 = `fields.${rawKey}`;
            const t3 = I18n.t(k3);
            if (t3 && t3 !== k3) return t3;
            
            // 如果 rawKey 包含下划线，尝试转换为驼峰命名（phone_country_code -> phoneCountryCode）
            if (rawKey.includes('_')) {
                const camelCaseKey = rawKey.split('_').map((part, index) => 
                    index === 0 ? part : part.charAt(0).toUpperCase() + part.slice(1)
                ).join('');
                const k4 = `fields.${camelCaseKey}`;
                const t4 = I18n.t(k4);
                if (t4 && t4 !== k4) return t4;
            }
        }

        // 2) 再用 label 做 fallback（支援 fields.<中文label>）
        if (rawLabel) {
            const normalized = rawLabel.toLowerCase().replace(/\s+/g, '').replace(/[()（）]/g, '');
            if (normalized) {
                const k4 = `fields.${normalized}`;
                const t4 = I18n.t(k4);
                if (t4 && t4 !== k4) return t4;
            }
            if (/[\u3400-\u9FFF]/.test(rawLabel)) {
                const k5 = `fields.${rawLabel}`;
                const t5 = I18n.t(k5);
                if (t5 && t5 !== k5) return t5;
            }
        }

        return rawLabel;
    }

    render() {
        const container = document.getElementById('dynamicFormContainer');
        if (!container) return;

        // 使用 i18n 獲取翻譯
        const getText = (key) => {
            if (typeof I18n !== 'undefined' && I18n.t) {
                return I18n.t(key);
            }
            // 後備方案
            const fallback = {
                'common.edit': '編輯',
                'common.add': '新增',
                'common.backToList': '返回列表',
                'common.fieldSettings': '欄位設定',
                'common.save': '保存',
                'common.cancel': '取消',
                'common.required': '必填',
                'common.optional': '選填'
            };
            return fallback[key] || key;
        };
        
        const translationKey = this.getTranslationKey();

        // i18n: form 標題優先使用單數實體名稱（common.product/common.customer...），避免英文出現 "Edit Products"
        const tryTranslate = (key) => {
            if (typeof I18n === 'undefined' || !I18n.t) return null;
            const t = I18n.t(key);
            return (t && t !== key) ? t : null;
        };

        // 僅對簡單英文複數做單數化（例如 products -> product, customers -> customer）
        const singularCandidate = (/^[a-z]+s$/.test(translationKey)) ? translationKey.slice(0, -1) : translationKey;
        const pageTitle =
            tryTranslate(`common.${singularCandidate}`) ||
            tryTranslate(`menu.${translationKey}`) ||
            this.config.title;

        const editText = getText('common.edit');
        const addText = getText('common.add');

        // 自動決定是否需要在前綴與標題之間加入空格（英文需要，中文通常不需要）
        const joinTitle = (prefix, subject) => {
            const a = (prefix == null ? '' : String(prefix)).trimEnd();
            const b = (subject == null ? '' : String(subject)).trimStart();
            if (!a) return b;
            if (!b) return a;
            const needSpace = /[A-Za-z0-9]$/.test(a) && /^[A-Za-z0-9]/.test(b);
            return needSpace ? `${a} ${b}` : `${a}${b}`;
        };

        const prefixText = this.isEdit ? editText : addText;
        const title = joinTitle(prefixText, pageTitle);
        // Determine whether a space is needed between the action prefix and entity name
        const needTitleSpace = /[A-Za-z0-9]$/.test(String(prefixText).trimEnd()) && /^[A-Za-z0-9]/.test(String(pageTitle).trimStart());
        const titleSep = needTitleSpace ? ' ' : '';
        const icon = this.config.icon ? `<i class="bi ${this.config.icon} me-2"></i>` : '';
        
        // 欄位設定按鈕文字
        const fieldSettingsText = getText('common.fieldSettings') || '欄位設定';
        const showFieldSettings = this.canConfigureFieldSettings();
        const fieldSettingsButton = showFieldSettings ? `
                    <button type="button" class="btn btn-outline-primary" id="fieldSettingsBtn" onclick="window.dynamicForm && window.dynamicForm.openFieldSettingsModal()">
                        <i class="bi bi-gear"></i> <span data-i18n="common.fieldSettings">${fieldSettingsText}</span>
                    </button>
        ` : '';
        
        const header = `
            <div class="d-flex justify-content-between align-items-center mb-4">
                <h3 id="pageTitle">${icon}<span data-i18n="${this.isEdit ? 'common.edit' : 'common.add'}">${prefixText}</span>${titleSep}${pageTitle}</h3>
                <div class="d-flex gap-2">
                    <button type="button" class="btn btn-outline-secondary" id="backToListBtn" onclick="return window.dynamicForm ? window.dynamicForm.handleCancel(event) : false;" data-i18n="common.backToList">
                        <i class="bi bi-arrow-left"></i> <span data-i18n="common.backToList">${getText('common.backToList')}</span>
                    </button>
                    ${fieldSettingsButton}
                </div>
            </div>
        `;

        // 特殊處理：客戶表單和優惠券表單的字段整合成兩行顯示
        let formFields = '';
        
        // 載入欄位設定（確保所有排版分支都能存取 fieldSettings，包含額外欄位）
        this.loadFieldSettings();
        
        if (this.pageName === 'customers') {
            const customerBasicFields = ['name', 'last_name', 'email', 'phone'];
            let otherFields = [];
            let basicFields = [];
            let codeField = null;
            
            this.config.formFields.forEach(field => {
                if (field.key === 'code') {
                    codeField = field;
                } else if (customerBasicFields.includes(field.key)) {
                    basicFields.push(field);
                } else if (field.key === 'phone_country_code') {
                    // 已在電話行內自訂排版，避免重複顯示
                    return;
                } else {
                    otherFields.push(field);
                }
            });
            
            // 編號字段放在最頂部
            if (codeField) {
                formFields += this.renderField(codeField);
            }
            
            // 第一行：名稱和姓氏
            const nameField = basicFields.find(f => f.key === 'name');
            const lastNameField = basicFields.find(f => f.key === 'last_name');
            if (nameField || lastNameField) {
                // 名稱行：不要使用 mb-3（用 g-2 保留 mobile 堆疊時的間距）
                formFields += '<div class="row g-2">';
                if (nameField) {
                    let fieldHtml = this.renderField(nameField);
                    // 替換最外層的 mb-3 為 mb-0
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (lastNameField) {
                    let fieldHtml = this.renderField(lastNameField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                formFields += '</div>';
            }
            
            // 頭像放在姓氏下一行
            const profilePicField = this.config.formFields.find(f => f.key === 'profile_pic');
            if (profilePicField) {
                formFields += this.renderField(profilePicField);
            }
            
            // 第二行：郵箱和電話（含區號）
            const emailField = basicFields.find(f => f.key === 'email');
            const phoneField = basicFields.find(f => f.key === 'phone');
            const phoneCodeField = this.config.formFields.find(f => f.key === 'phone_country_code');
            if (emailField || phoneField) {
                formFields += '<div class="row mb-3">';
                if (emailField) {
                    let fieldHtml = this.renderField(emailField);
                    // 郵箱外層保留 mb-3（不要改成 mb-0）
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (phoneField) {
                    // 區號 + 電話並排（微調比例）
                    formFields += '<div class="col-md-6"><div class="row g-2 mb-0">';
                    if (phoneCodeField) {
                        let codeHtml = this.renderField(phoneCodeField);
                        codeHtml = codeHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        // 與 /enterprises 一致：mobile 堆疊、md 以上並排
                        formFields += '<div class="col-12 col-md-4">' + codeHtml + '</div>';
                    }
                    let phoneHtml = this.renderField(phoneField);
                    phoneHtml = phoneHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    // 與 /enterprises 一致：mobile 堆疊、md 以上並排
                    formFields += '<div class="col-12 col-md-8">' + phoneHtml + '</div>';
                    formFields += '</div></div>';
                }
                formFields += '</div>';
            }
            
            // 出生日期和性别放在同一行
            const birthDateField = otherFields.find(f => f.key === 'birth_date');
            const genderField = otherFields.find(f => f.key === 'gender');
            if (birthDateField || genderField) {
                formFields += '<div class="row mb-3">';
                if (birthDateField) {
                    let fieldHtml = this.renderField(birthDateField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-0">/, '<div class="mb-3">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (genderField) {
                    let fieldHtml = this.renderField(genderField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-0" id="field_container_gender"/, '<div class="mb-3" id="field_container_gender"');
                    fieldHtml = fieldHtml.replace(/<div class="mb-0">/, '<div class="mb-3">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                formFields += '</div>';
            }
            
            // 其他字段正常顯示（排除已處理的出生日期、性别和頭像）
            otherFields.forEach(field => {
                if (field.key !== 'birth_date' && field.key !== 'gender' && field.key !== 'profile_pic') {
                formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'stores') {
            // 店舖表單：電話區號和電話放在一起
            const storeBasicFields = ['name', 'address', 'contact_person', 'email', 'phone'];
            let otherFields = [];
            let basicFields = [];
            let codeField = null;
            
            this.config.formFields.forEach(field => {
                if (field.key === 'code') {
                    codeField = field;
                } else if (storeBasicFields.includes(field.key)) {
                    basicFields.push(field);
                } else if (field.key === 'phone_country_code') {
                    // 已在電話行內自訂排版，避免重複顯示
                    return;
                } else {
                    otherFields.push(field);
                }
            });
            
            // 編號字段放在最頂部
            if (codeField) {
                formFields += this.renderField(codeField);
            }
            
            // 名稱字段
            const nameField = basicFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
            }
            
            // 圖片獨立一行（在名稱下一行）
            const imageField = this.config.formFields.find(f => f.key === 'image_url');
            if (imageField) {
                formFields += this.renderField(imageField);
            }
            
            // 國家和地區（同一行）
            const countryField = this.config.formFields.find(f => f.key === 'address_country_code');
            const regionField = this.config.formFields.find(f => f.key === 'address_region_code');
            if (countryField || regionField) {
                formFields += '<div class="row mb-3">';
                if (countryField) {
                    let fieldHtml = this.renderField(countryField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (regionField) {
                    let fieldHtml = this.renderField(regionField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                formFields += '</div>';
            }
            
            // 地址
            const addressField = basicFields.find(f => f.key === 'address');
            if (addressField) {
                formFields += this.renderField(addressField);
            }
            
            // 聯絡人和電話（含區號）在同一行
            const contactPersonField = basicFields.find(f => f.key === 'contact_person');
            const phoneField = basicFields.find(f => f.key === 'phone');
            const phoneCodeField = this.config.formFields.find(f => f.key === 'phone_country_code');
            if (contactPersonField || phoneField) {
                formFields += '<div class="row mb-3">';
                if (contactPersonField) {
                    let fieldHtml = this.renderField(contactPersonField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (phoneField) {
                    // 區號 + 電話並排（微調比例）
                    formFields += '<div class="col-md-6"><div class="row g-2 mb-0">';
                    if (phoneCodeField) {
                        let codeHtml = this.renderField(phoneCodeField);
                        codeHtml = codeHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        // 與 /enterprises 一致：mobile 堆疊、md 以上並排
                        formFields += '<div class="col-12 col-md-4">' + codeHtml + '</div>';
                    }
                    let phoneHtml = this.renderField(phoneField);
                    phoneHtml = phoneHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    // 與 /enterprises 一致：mobile 堆疊、md 以上並排
                    formFields += '<div class="col-12 col-md-8">' + phoneHtml + '</div>';
                    formFields += '</div></div>';
                }
                formFields += '</div>';
            }
            
            // 郵箱
            const emailField = basicFields.find(f => f.key === 'email');
            if (emailField) {
                formFields += this.renderField(emailField);
            }
            
            // 其他字段正常顯示（跳过已处理的 address_country_code、address_region_code 和 image_url）
            otherFields.forEach(field => {
                if (field.key !== 'address_country_code' && 
                    field.key !== 'address_region_code' && 
                    field.key !== 'image_url') {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'rooms') {
            // 房間：圖片獨立一行（在名稱下一行），樣式與 products 一致
            const processedFields = new Set();

            // 編號字段放在最頂部
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }

            // 名稱字段
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }

            // 圖片獨立一行（在名稱下一行）
            const imageField = this.config.formFields.find(f => f.key === 'image_url');
            if (imageField) {
                formFields += this.renderField(imageField);
                processedFields.add(imageField.key);
            }

            // 其他字段維持原本單欄順序
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'equipments') {
            // 設備：圖片獨立一行（在名稱下一行），樣式與 products 一致
            const processedFields = new Set();

            // 編號字段放在最頂部
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }

            // 名稱字段
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }

            // 圖片獨立一行（在名稱下一行）
            const imageField = this.config.formFields.find(f => f.key === 'image_url');
            if (imageField) {
                formFields += this.renderField(imageField);
                processedFields.add(imageField.key);
            }

            // 其他字段維持原本單欄順序
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'products') {
            // 產品：圖片獨立一行（在名稱下一行）
            const processedFields = new Set();

            // 編號字段放在最頂部
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }

            // 名稱字段
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }

            // 圖片獨立一行（在名稱下一行）
            const imageField = this.config.formFields.find(f => f.key === 'image_url');
            if (imageField) {
                formFields += this.renderField(imageField);
                processedFields.add(imageField.key);
            }

            // 其他字段維持原本單欄順序
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'vehicles') {
            // 車輛：圖片獨立一行（在名稱下一行）
            const processedFields = new Set();

            // 編號字段放在最頂部
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }

            // 名稱字段
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }

            // 圖片獨立一行（在名稱下一行）
            const imageField = this.config.formFields.find(f => f.key === 'image_url');
            if (imageField) {
                formFields += this.renderField(imageField);
                processedFields.add(imageField.key);
            }

            // 其他字段維持原本單欄順序
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'coupons') {
            // 優惠券表單：整合同類字段成兩列
            const couponRowFields = [
                ['code', 'name'],                    // 代碼和名稱
                ['coupon_type', 'discount_value'],    // 類型和折扣值
                ['max_discount', 'min_purchase'],    // 最大折扣和最低消費
                ['min_product_quantity', 'min_product_amount'], // 最低產品數量和金額
                ['valid_from', 'valid_to'],          // 有效期開始和結束
                ['usage_limit', 'customer_limit']    // 使用次數限制和每客戶限制
            ];
            
            const singleRowFields = ['description', 'member_level_id', 'status']; // 單獨顯示的字段
            
            let processedFields = new Set();
            let otherFields = [];
            
            // 分類字段
            this.config.formFields.forEach(field => {
                const isInRow = couponRowFields.some(row => row.includes(field.key));
                const isSingle = singleRowFields.includes(field.key);
                if (!isInRow && !isSingle) {
                    otherFields.push(field);
                }
            });
            
            // 渲染兩列字段
            couponRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'incomes') {
            // 收入表單：標題和類別獨立一行，其他字段兩列布局
            let processedFields = new Set();
            
            // 標題獨立一行（fullWidth）
            const descriptionField = this.config.formFields.find(f => f.key === 'description');
            if (descriptionField) {
                formFields += this.renderField(descriptionField);
                processedFields.add(descriptionField.key);
            }
            
            // 類別獨立一行（fullWidth）
            const categoryField = this.config.formFields.find(f => f.key === 'category');
            if (categoryField) {
                formFields += this.renderField(categoryField);
                processedFields.add(categoryField.key);
            }
            
            // 關聯 ID 獨立一行（根據類別動態顯示，可能有多個）
            const referenceIdFields = this.config.formFields.filter(f => f.key === 'reference_id');
            referenceIdFields.forEach(referenceIdField => {
                formFields += this.renderField(referenceIdField);
                processedFields.add(referenceIdField.key);
            });
            
            // 其他字段使用兩列布局
            const incomeRowFields = [
                ['amount', 'income_date'],              // 金額和日期
                ['payment_method', 'reference_number'], // 付款方法和參考號碼
                ['bank_account_id', 'attachment']       // 收款賬戶和附件
            ];
            
            // 渲染兩列字段
            incomeRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 其他字段（notes 等）正常顯示
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'expenses') {
            // 支出表單：欄位次序/排版對齊 incomes/new
            let processedFields = new Set();

            // 標題獨立一行（fullWidth）
            const descriptionField = this.config.formFields.find(f => f.key === 'description');
            if (descriptionField) {
                formFields += this.renderField(descriptionField);
                processedFields.add(descriptionField.key);
            }

            // 相關人員獨立一行（fullWidth）
            const relatedUserField = this.config.formFields.find(f => f.key === 'related_user_id');
            if (relatedUserField) {
                formFields += this.renderField(relatedUserField);
                processedFields.add(relatedUserField.key);
            }

            // 類別獨立一行（fullWidth）
            const categoryField = this.config.formFields.find(f => f.key === 'category');
            if (categoryField) {
                formFields += this.renderField(categoryField);
                processedFields.add(categoryField.key);
            }

            // 關聯欄位獨立一行（依類別動態顯示）
            const projectIdField = this.config.formFields.find(f => f.key === 'project_id');
            if (projectIdField) {
                formFields += this.renderField(projectIdField);
                processedFields.add(projectIdField.key);
            }
            // 處理所有 reference_id 字段（可能有多個，根據不同的 category 顯示）
            const referenceIdFields = this.config.formFields.filter(f => f.key === 'reference_id');
            referenceIdFields.forEach(referenceIdField => {
                formFields += this.renderField(referenceIdField);
                processedFields.add(referenceIdField.key);
            });

            // 其他字段使用兩列布局
            const expenseRowFields = [
                ['amount', 'expense_date'],              // 金額和日期
                ['payment_method', 'reference_number'],  // 支付方式和參考號碼
                ['bank_account_id', 'attachment']        // 付款賬戶和附件
            ];

            expenseRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);

                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });

            // 其他字段（notes 等）正常顯示
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'expense-requests') {
            // 支出申請表單：整合同類字段
            const expenseRequestRowFields = [
                ['title', 'amount'],                    // 標題和金額
                ['request_date', 'attachment']           // 申請日期和附件
            ];
            const singleRowFields = ['description']; // 單獨顯示的字段
            
            let processedFields = new Set();
            
            // 渲染兩列字段
            expenseRequestRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段和其他字段
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'brands') {
            // 品牌：圖片獨立一行（在名稱下一行），樣式與 products 一致
            const processedFields = new Set();

            // 名稱字段
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }

            // 圖片獨立一行（在名稱下一行），logo_url 對應圖片字段
            const imageField = this.config.formFields.find(f => f.key === 'logo_url');
            if (imageField) {
                formFields += this.renderField(imageField);
                processedFields.add(imageField.key);
            }

            // 其他字段維持原本單欄順序
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'phone-country-codes' || this.pageName === 'product-attributes' || this.pageName === 'points' || this.pageName === 'product-types' || this.pageName === 'order-labels' || this.pageName === 'service-types' || this.pageName === 'services') {
            // 電話區號、產品屬性、積分、產品類型、訂單標籤、服務種類、服務表單：每個字段獨自一行
            this.config.formFields.forEach(field => {
                formFields += this.renderField(field);
            });
        } else if (this.pageName === 'holidays') {
            // 假期表單：名稱和狀態字段 full width，其他字段保持原有佈局
            let processedFields = new Set();
            
            // 名稱字段獨自一行（full width）
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 狀態字段獨自一行（full width）
            const statusField = this.config.formFields.find(f => f.key === 'status');
            if (statusField) {
                formFields += this.renderField(statusField);
                processedFields.add(statusField.key);
            }
            
            // 其他字段使用兩列布局
            const holidaysRowFields = [
                ['start_date', 'end_date'],           // 開始日期和結束日期
                ['is_recurring']                      // 每年重複
            ];
            const singleRowFields = ['description']; // 描述單獨顯示（全寬，因為是 textarea）
            
            // 渲染兩列字段
            holidaysRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（描述）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'warehouses') {
            // 倉庫表單：編號和名稱獨自一行，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 編號字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 第一行：名稱 *, 聯絡人
            const nameField = this.config.formFields.find(f => f.key === 'name');
            const contactPersonField = this.config.formFields.find(f => f.key === 'contact_person');
            if (nameField || contactPersonField) {
                formFields += '<div class="row mb-3">';
                if (nameField) {
                    let fieldHtml = this.renderField(nameField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(nameField.key);
                }
                if (contactPersonField) {
                    let fieldHtml = this.renderField(contactPersonField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(contactPersonField.key);
                }
                formFields += '</div>';
            }
            
            // 第二行：郵箱, 電話區號+電話
            const emailField = this.config.formFields.find(f => f.key === 'email');
            const phoneCodeField = this.config.formFields.find(f => f.key === 'phone_country_code');
            const phoneField = this.config.formFields.find(f => f.key === 'phone');
            if (emailField || phoneField) {
                formFields += '<div class="row mb-3">';
                if (emailField) {
                    let fieldHtml = this.renderField(emailField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(emailField.key);
                }
                if (phoneField) {
                    // 區號 + 電話並排
                    formFields += '<div class="col-md-6"><div class="row g-2 mb-0">';
                    if (phoneCodeField) {
                        let codeHtml = this.renderField(phoneCodeField);
                        codeHtml = codeHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        // 與 /enterprises 一致：mobile 堆疊、md 以上並排
                        formFields += '<div class="col-12 col-md-4">' + codeHtml + '</div>';
                        processedFields.add(phoneCodeField.key);
                    }
                    let phoneHtml = this.renderField(phoneField);
                    phoneHtml = phoneHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    // 與 /enterprises 一致：mobile 堆疊、md 以上並排
                    formFields += '<div class="col-12 col-md-8">' + phoneHtml + '</div>';
                    processedFields.add(phoneField.key);
                    formFields += '</div></div>';
                }
                formFields += '</div>';
            }
            
            // 狀態單獨一行
            const statusField = this.config.formFields.find(f => f.key === 'status');
            if (statusField) {
                formFields += this.renderField(statusField);
                processedFields.add(statusField.key);
            }
            
            // 國家和地區（同一行，在地址之前）- 一開始就顯示，不依賴國家選擇
            const countryField = this.config.formFields.find(f => f.key === 'address_country_code');
            const regionField = this.config.formFields.find(f => f.key === 'address_region_code');
            if (countryField || regionField) {
                formFields += '<div class="row mb-3">';
                if (countryField) {
                    let fieldHtml = this.renderField(countryField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(countryField.key);
                }
                if (regionField) {
                    let fieldHtml = this.renderField(regionField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(regionField.key);
                }
                formFields += '</div>';
            }
            
            // 地址字段單獨顯示（全寬）
            const addressField = this.config.formFields.find(f => f.key === 'address');
            if (addressField) {
                formFields += this.renderField(addressField);
                processedFields.add(addressField.key);
            }
            
            // 其他字段正常顯示（如果有的話）
            this.config.formFields.forEach(field => {
                // 跳过已处理的字段，以及 address_country_code 和 address_region_code（已在上面单独处理）
                if (!processedFields.has(field.key) && field.key !== 'address_country_code' && field.key !== 'address_region_code') {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'appointments') {
            // 預約表單：房間、車輛、設備、備註放在表單最底部
            let processedFields = new Set();
            const bottomFields = ['room_ids', 'vehicle_ids', 'equipment_ids', 'notes'];
            
            // 先渲染非底部字段
            this.config.formFields.forEach(field => {
                if (!bottomFields.includes(field.key)) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 最後渲染底部字段（房間、車輛、設備、備註）
            bottomFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
        } else if (this.pageName === 'leave-requests') {
            // 請假申請表單：請假類型一行，開始日期和結束日期同一行
            let processedFields = new Set();
            
            // 請假類型獨自一行
            const leaveTypeField = this.config.formFields.find(f => f.key === 'leave_type');
            if (leaveTypeField) {
                formFields += this.renderField(leaveTypeField);
                processedFields.add(leaveTypeField.key);
            }
            
            // 開始日期和結束日期同一行
            const startDateField = this.config.formFields.find(f => f.key === 'start_date');
            const endDateField = this.config.formFields.find(f => f.key === 'end_date');
            if (startDateField || endDateField) {
                formFields += '<div class="row mb-3">';
                if (startDateField) {
                    let fieldHtml = this.renderField(startDateField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(startDateField.key);
                }
                if (endDateField) {
                    let fieldHtml = this.renderField(endDateField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    processedFields.add(endDateField.key);
                }
                formFields += '</div>';
            }
            
            // 其他字段正常顯示
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'reminders') {
            // 提醒表單：所有字段各一行
            this.config.formFields.forEach(field => {
                formFields += this.renderField(field);
            });
        } else if (this.pageName === 'notes') {
            // 備忘表單：標題 full width，內容 html-editor
            this.config.formFields.forEach(field => {
                formFields += this.renderField(field);
            });
        } else if (this.pageName === 'logistics-companies') {
            // 物流公司表單：名稱和代碼獨自一行，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 名稱字段獨自一行
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 代碼字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 其他字段使用兩列布局
            const logisticsRowFields = [
                ['base_fee', 'per_item_fee'],      // 預設定額和件價
                ['per_weight_fee', 'per_area_fee'], // 重量價和面積價
                ['allowed_country_codes', 'allowed_region_keys'] // 國家/地區限制
            ];
            const singleRowFields = ['status']; // 狀態單獨顯示（全寬）
            
            // 渲染兩列字段
            logisticsRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（狀態）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'payment-methods') {
            // 付款方式表單：付款形式在最頂，然後是名稱和代碼，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 付款形式字段獨自一行（最頂）
            const paymentTypeField = this.config.formFields.find(f => f.key === 'payment_type');
            if (paymentTypeField) {
                formFields += this.renderField(paymentTypeField);
                processedFields.add(paymentTypeField.key);
            }
            
            // 名稱字段獨自一行（如果是 gateway 類型，會變成 dropdown）
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 代碼字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 其他字段使用兩列布局
            const paymentRowFields = [
                ['is_default', 'is_default_expense']  // 系統預設客戶付款方法和系統預設支出付款方法在同一行
            ];
            
            // 渲染兩列字段
            paymentRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = rowPair[1] ? this.config.formFields.find(f => f.key === rowPair[1]) : null;
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 其他字段正常顯示（如果有的話），但排除 status
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key) && field.key !== 'status') {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 狀態字段單獨在最後一行，全寬
            const statusField = this.config.formFields.find(f => f.key === 'status');
            if (statusField) {
                formFields += this.renderField(statusField);
                processedFields.add(statusField.key);
            }
        } else if (this.pageName === 'shipping-methods') {
            // 運送方式表單：名稱和代碼獨自一行，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 名稱字段獨自一行
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 代碼字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 其他字段使用兩列布局
            const shippingRowFields = [
                ['requires_shipping', 'is_default']  // 需要送貨和設為系統預設
            ];
            const singleRowFields = ['status']; // 狀態單獨顯示（全寬）
            
            // 渲染兩列字段
            shippingRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（狀態）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'service-staffs') {
            // 服務員表單：員工編號獨自一行（full width），其他字段使用兩列布局
            let processedFields = new Set();
            
            // 員工編號字段獨自一行（full width）
            const employeeNumberField = this.config.formFields.find(f => f.key === 'employee_number');
            if (employeeNumberField) {
                formFields += this.renderField(employeeNumberField);
                processedFields.add(employeeNumberField.key);
            }
            
            // 其他字段使用兩列布局
            const staffRowFields = [
                ['name', 'phone'],           // 姓名和電話
                ['hourly_rate', 'status']    // 時薪和狀態
            ];
            const singleRowFields = ['specialization']; // 專長單獨顯示（全寬，因為是 textarea）
            
            // 渲染兩列字段
            staffRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（專長）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'rooms') {
            // 房間表單：編號和名稱獨自一行，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 編號字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 名稱字段獨自一行
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 其他字段使用兩列布局
            const roomRowFields = [
                ['capacity', 'status']  // 容量和狀態
            ];
            const singleRowFields = ['description', 'notes']; // 描述和備註單獨顯示（全寬，因為是 textarea）
            
            // 渲染兩列字段
            roomRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（描述、備註、允許重複使用）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話，如 allow_overlap）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'equipments') {
            // 設備表單：編號和名稱獨自一行，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 編號字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 名稱字段獨自一行
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 其他字段使用兩列布局
            const equipmentRowFields = [
                ['equipment_type', 'status']  // 設備類型和狀態
            ];
            const singleRowFields = ['notes']; // 備註單獨顯示（全寬，因為是 textarea）
            
            // 渲染兩列字段
            equipmentRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（備註、允許重複使用）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話，如 allow_overlap）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'member-levels') {
            // 會員等級表單：名稱和代碼獨自一行，其他字段使用兩列布局
            let processedFields = new Set();
            
            // 名稱字段獨自一行
            const nameField = this.config.formFields.find(f => f.key === 'name');
            if (nameField) {
                formFields += this.renderField(nameField);
                processedFields.add(nameField.key);
            }
            
            // 代碼字段獨自一行
            const codeField = this.config.formFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 其他字段使用兩列布局
            const memberLevelRowFields = [
                ['level_order', 'discount_rate'],           // 順序和折扣率
                ['min_points', 'min_purchase_amount'],      // 最低積分和最低購物金額
                ['is_default', 'auto_upgrade']              // 設為系統預設和自動會員升級
            ];
            const singleRowFields = ['description']; // 福利單獨顯示（全寬，因為是 textarea）
            
            // 渲染兩列字段
            memberLevelRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（福利）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示（如果有的話）
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'bank-accounts') {
            // 銀行賬戶表單：整合同類字段
            const bankAccountRowFields = [
                ['name', 'bank_name'],                  // 賬戶名稱和銀行名稱
                ['account_number', 'account_holder'],   // 賬戶號碼和戶名
                ['currency', 'status'],                 // 貨幣和狀態
                ['is_default_receiving', 'is_default_payment'] // 預設收款和付款
            ];
            const singleRowFields = ['notes']; // 單獨顯示的字段
            
            let processedFields = new Set();
            
            // 渲染兩列字段
            bankAccountRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段和其他字段
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'calendars') {
            // 日曆表單：整合同類字段
            const calendarRowFields = [
                ['title', 'event_type'],                // 標題和事件類型
                ['start_time', 'end_time']              // 開始時間和結束時間
            ];
            const singleRowFields = ['description', 'all_day']; // 描述和全天事件單獨顯示（全寬）
            
            let processedFields = new Set();
            
            // 渲染兩列字段
            calendarRowFields.forEach(rowPair => {
                const field1 = this.config.formFields.find(f => f.key === rowPair[0]);
                const field2 = this.config.formFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                }
            });
            
            // 單獨顯示的字段（描述）
            singleRowFields.forEach(key => {
                const field = this.config.formFields.find(f => f.key === key);
                if (field) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 其他字段正常顯示
            this.config.formFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'products') {
            // 產品表單：整合同類字段
            const productFields = [];
            const productFieldKeys = new Set();
            this.config.formFields.forEach(field => {
                if (!field || productFieldKeys.has(field.key)) return;
                productFieldKeys.add(field.key);
                productFields.push(field);
            });

            const productRowFields = [
                ['name', 'image_url'],                   // 名稱和圖片
                ['sku', 'barcode'],                     // SKU和條碼
                ['product_type_id', 'brand_id'],         // 產品類型和品牌
                ['price', 'cost'],                       // 價格和成本
                ['unit', 'is_service_package']          // 單位和服務套票
            ];
            const singleRowFields = ['code', 'description']; // 單獨顯示的字段（移除 service_package_service_id，它會在 is_service_package 之後單獨顯示）
            
            let processedFields = new Set();
            
            // 編號字段放在最頂部
            const codeField = productFields.find(f => f.key === 'code');
            if (codeField) {
                formFields += this.renderField(codeField);
                processedFields.add(codeField.key);
            }
            
            // 渲染兩列字段
            productRowFields.forEach(rowPair => {
                const field1 = productFields.find(f => f.key === rowPair[0]);
                const field2 = productFields.find(f => f.key === rowPair[1]);
                
                if (field1 || field2) {
                    formFields += '<div class="row mb-3">';
                    if (field1) {
                        let fieldHtml = this.renderField(field1);
                        // SKU 字段保持 mb-3，其他字段改为 mb-0
                        if (field1.key !== 'sku') {
                            fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        }
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field1.key);
                    }
                    if (field2) {
                        let fieldHtml = this.renderField(field2);
                        // SKU 字段保持 mb-3，其他字段改为 mb-0
                        if (field2.key !== 'sku') {
                            fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        }
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                        processedFields.add(field2.key);
                    }
                    formFields += '</div>';
                    
                    // 如果這一行包含 is_service_package，立即在下一行顯示 service_package_service_id（full width）
                    if (rowPair.includes('is_service_package')) {
                        const serviceField = productFields.find(f => f.key === 'service_package_service_id');
                        if (serviceField) {
                            formFields += this.renderField(serviceField);
                            processedFields.add(serviceField.key);
                        }
                    }
                }
            });
            
            // 處理 fullWidth 字段（獨立一行顯示）
            productFields.forEach(field => {
                if (!processedFields.has(field.key) && field.fullWidth) {
                    formFields += this.renderField(field);
                    processedFields.add(field.key);
                }
            });
            
            // 單獨顯示的字段和其他字段
            productFields.forEach(field => {
                if (!processedFields.has(field.key)) {
                    formFields += this.renderField(field);
                }
            });
        } else if (this.pageName === 'suppliers') {
            // 供應商表單特殊處理（類似客戶表單）
            const supplierBasicFields = ['name', 'last_name', 'email', 'phone'];
            let otherFields = [];
            let basicFields = [];
            let codeField = null;
            
            this.config.formFields.forEach(field => {
                if (field.key === 'code') {
                    codeField = field;
                } else if (supplierBasicFields.includes(field.key)) {
                    basicFields.push(field);
                } else if (field.key === 'phone_country_code') {
                    // 已在電話行內自訂排版，避免重複顯示
                    return;
                } else {
                    otherFields.push(field);
                }
            });
            
            // 編號字段放在最頂部
            if (codeField) {
                formFields += this.renderField(codeField);
            }
            
            // 第一行：名稱和姓氏
            const nameField = basicFields.find(f => f.key === 'name');
            const lastNameField = basicFields.find(f => f.key === 'last_name');
            if (nameField || lastNameField) {
                formFields += '<div class="row mb-3">';
                if (nameField) {
                    let fieldHtml = this.renderField(nameField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (lastNameField) {
                    let fieldHtml = this.renderField(lastNameField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                formFields += '</div>';
            }
            
            // 第二行：郵箱和電話（含區號）
            const emailField = basicFields.find(f => f.key === 'email');
            const phoneField = basicFields.find(f => f.key === 'phone');
            const phoneCodeField = this.config.formFields.find(f => f.key === 'phone_country_code');
            if (emailField || phoneField) {
                formFields += '<div class="row mb-3">';
                if (emailField) {
                    let fieldHtml = this.renderField(emailField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                }
                if (phoneField) {
                    // 區號 + 電話並排（微調比例）
                    formFields += '<div class="col-md-6"><div class="row g-2 mb-0">';
                    if (phoneCodeField) {
                        let codeHtml = this.renderField(phoneCodeField);
                        codeHtml = codeHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-4">' + codeHtml + '</div>';
                    }
                    let phoneHtml = this.renderField(phoneField);
                    phoneHtml = phoneHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-8">' + phoneHtml + '</div>';
                    formFields += '</div></div>';
                }
                formFields += '</div>';
            }
            
            // 國家和地區（同一行，在地址之前）- 一開始就顯示，不依賴國家選擇
            const countryField = otherFields.find(f => f.key === 'address_country_code');
            const regionField = otherFields.find(f => f.key === 'address_region_code');
            const addressField = otherFields.find(f => f.key === 'address');
            
            if (countryField || regionField) {
                formFields += '<div class="row mb-3">';
                if (countryField) {
                    let fieldHtml = this.renderField(countryField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    // 從 otherFields 中移除，避免重複顯示
                    otherFields = otherFields.filter(f => f.key !== 'address_country_code');
                }
                if (regionField) {
                    let fieldHtml = this.renderField(regionField);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    // 從 otherFields 中移除，避免重複顯示
                    otherFields = otherFields.filter(f => f.key !== 'address_region_code');
                }
                formFields += '</div>';
            }
            
            // 地址字段（在國家和地區之後）
            if (addressField) {
                formFields += this.renderField(addressField);
                // 從 otherFields 中移除，避免重複顯示
                otherFields = otherFields.filter(f => f.key !== 'address');
            }
            
            // 其他字段正常顯示
            otherFields.forEach(field => {
                formFields += this.renderField(field);
            });
        } else {
            // 其他頁面正常渲染
            // NOTE:
            // 之前 fullWidth 欄位會被「提到最上面」渲染，導致欄位順序被打亂（例如 rooms/equipments 的圖片跑到最前）。
            // 這裡改為：保持欄位設定的順序（如有）；遇到 fullWidth/textarea 則獨佔一行。
            
            // 載入欄位設定
            this.loadFieldSettings();
            
            // 獲取有效的欄位列表（已排序和過濾，含 extra fields）
            const effectiveFields = this.getEffectiveFormFields();
            // 標記：generic 分支已透過 getEffectiveFormFields() 包含 extra fields，
            // 不需要再由後面的 extra fields block 重複渲染
            this._extraFieldsRenderedByEffective = true;

            const processedFields = new Set();

            // 先處理 sameRow 字段（維持原邏輯：同組字段並排）
            const sameRowGroups = {};
            effectiveFields.forEach(field => {
                if (!processedFields.has(field.key) && field.sameRow) {
                    const rowKey = field.sameRow;
                    if (!sameRowGroups[rowKey]) {
                        sameRowGroups[rowKey] = [];
                    }
                    sameRowGroups[rowKey].push(field);
                    processedFields.add(field.key);
                }
            });

            Object.keys(sameRowGroups).forEach(rowKey => {
                const group = sameRowGroups[rowKey];
                const otherField = effectiveFields.find(f => f.key === rowKey && !processedFields.has(f.key));
                if (otherField) {
                    group.push(otherField);
                    processedFields.add(otherField.key);
                }

                if (group.length > 0) {
                    formFields += '<div class="row mb-3">';
                    group.forEach(f => {
                        let fieldHtml = this.renderField(f);
                        fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                        formFields += '<div class="col-md-6">' + fieldHtml + '</div>';
                    });
                    formFields += '</div>';
                }
            });

            // 修改 flushHalfRow：如果只有一個欄位，使用 full width
            const flushHalfRow = (rowFields) => {
                if (!rowFields || rowFields.length === 0) return;
                formFields += '<div class="row mb-3">';
                rowFields.forEach(f => {
                    let fieldHtml = this.renderField(f);
                    fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                    // 如果同一行只有一個欄位，使用 full width
                    const colClass = rowFields.length === 1 ? 'col-12' : 'col-md-6';
                    formFields += `<div class="${colClass}">` + fieldHtml + '</div>';
                });
                formFields += '</div>';
            };

            const flushFullRow = (field) => {
                let fieldHtml = this.renderField(field);
                fieldHtml = fieldHtml.replace(/<div class="mb-3">/, '<div class="mb-0">');
                formFields += '<div class="row mb-3"><div class="col-12">' + fieldHtml + '</div></div>';
            };

            let currentRow = [];
            effectiveFields.forEach(field => {
                if (processedFields.has(field.key)) return;

                // fullWidth / textarea：獨佔一行，但保持順序
                if (field.fullWidth || field.type === 'textarea') {
                    flushHalfRow(currentRow);
                    currentRow = [];
                    flushFullRow(field);
                    processedFields.add(field.key);
                    return;
                }

                // 其他字段：兩欄
                currentRow.push(field);
                processedFields.add(field.key);
                if (currentRow.length === 2) {
                    flushHalfRow(currentRow);
                    currentRow = [];
                }
            });

            // 處理剩餘的單個字段（會自動使用 full width）
            flushHalfRow(currentRow);
        }

        // ── 渲染額外欄位（extra fields from field settings）──
        // 對於自訂排版的頁面（如 customers, stores 等），
        // 在基礎欄位之後渲染已配置的額外欄位。
        // generic 分支已透過 getEffectiveFormFields() 包含 extra fields，不需重複渲染。
        if (!this._extraFieldsRenderedByEffective && this.fieldSettings && this.fieldSettings.extraFields && this.fieldSettings.extraFields.length > 0) {
            const baseFieldKeys = new Set(this.config.formFields.map(f => f.key));
            const extraFields = this.fieldSettings.extraFields.filter(ef => !baseFieldKeys.has(ef.key));
            
            if (extraFields.length > 0) {
                // 按 fields 設定中的 order 排序
                const fieldOrderMap = {};
                if (this.fieldSettings.fields) {
                    this.fieldSettings.fields.forEach(sf => {
                        fieldOrderMap[sf.key] = sf;
                    });
                }
                
                const sortedExtraFields = [...extraFields].sort((a, b) => {
                    const orderA = fieldOrderMap[a.key] ? fieldOrderMap[a.key].order : 9999;
                    const orderB = fieldOrderMap[b.key] ? fieldOrderMap[b.key].order : 9999;
                    return orderA - orderB;
                });
                
                // 只渲染 visible 的額外欄位，包裹 data-extra-field-key 以便後續識別
                sortedExtraFields.forEach(ef => {
                    const sfSetting = fieldOrderMap[ef.key];
                    if (sfSetting && sfSetting.visible === false) return;
                    formFields += `<div data-extra-field-key="${ef.key}">${this.renderField(ef)}</div>`;
                });
            }
        }

        const form = `
            <div class="card">
                <div class="card-body">
                    <form id="dynamicForm" onsubmit="return false;">
                        <input type="hidden" id="itemId" value="${this.itemId || ''}">
                        ${this.pageName === 'customers' ? `
                            <ul class="nav nav-tabs mb-4" id="customerFormTabs" role="tablist">
                                <li class="nav-item" role="presentation">
                                    <button class="nav-link active" id="data-tab" data-bs-toggle="tab" data-bs-target="#data-pane" type="button" role="tab" aria-controls="data-pane" aria-selected="true">${tryTranslate('customers.form.tabs.data') || '資料'}</button>
                                </li>
                                <li class="nav-item" role="presentation">
                                    <button class="nav-link" id="address-tab" data-bs-toggle="tab" data-bs-target="#address-pane" type="button" role="tab" aria-controls="address-pane" aria-selected="false">${tryTranslate('customers.form.tabs.address') || '地址'}</button>
                                </li>
                            </ul>
                            <div class="tab-content" id="customerFormTabContent">
                                <div class="tab-pane fade show active" id="data-pane" role="tabpanel" aria-labelledby="data-tab">
                                    ${formFields}
                                </div>
                                <div class="tab-pane fade" id="address-pane" role="tabpanel" aria-labelledby="address-tab">
                                    <div id="customerAddressManagementContainer"></div>
                                </div>
                            </div>
                        ` : `
                        ${formFields}
                        ${this.pageName === 'users' ? this.renderUserStoreSubTable() : ''}
                            <div id="customerAddressManagementContainer"></div>
                        `}
                        <div class="d-flex gap-2 mt-4 form-buttons-container">
                            <button type="submit" class="btn btn-primary" data-i18n="common.save">
                                <i class="bi bi-save"></i> <span data-i18n="common.save">${getText('common.save')}</span>
                            </button>
                            <button type="button" class="btn btn-secondary" id="cancelBtn" onclick="return window.dynamicForm ? window.dynamicForm.handleCancel(event) : false;" data-i18n="common.cancel">
                                <i class="bi bi-x"></i> <span data-i18n="common.cancel">${getText('common.cancel')}</span>
                            </button>
                        </div>
                    </form>
                </div>
            </div>
        `;

        container.innerHTML = header + form;

        // render 完成後，立即嘗試設置按鈕欄（新建模式）
        if (!this.isEdit) {
            // 使用 setTimeout 確保 DOM 已更新
            setTimeout(() => {
                if (typeof setupFormButtonBar === 'function') {
                    setupFormButtonBar(true); // 強制顯示
                }
            }, 50);
        }

        // payment-methods: render 後立刻初始化一次（避免任何事件綁定時序問題）
        if (this.pageName === 'payment-methods') {
            setTimeout(() => {
                try { this.handlePaymentTypeChange(); } catch (e) { console.warn('handlePaymentTypeChange failed', e); }
            }, 0);
        }
        
        // 產品表單：新建時強制設置服務套票/允許缺貨訂購預設為 no
        if (this.pageName === 'products' && !this.isEdit) {
            const svc = document.getElementById('field_is_service_package');
            const backorder = document.getElementById('field_allow_backorder');
            if (svc && !svc.value) svc.value = 'false';
            if (backorder && !backorder.value) backorder.value = 'false';
        }
        // 倉庫表單：新建時自動生成編號（英文字前綴）和設置狀態默認值
        if (this.pageName === 'warehouses' && !this.isEdit) {
            const codeInput = document.getElementById('field_code');
            if (codeInput && (!codeInput.value || codeInput.value.trim() === '')) {
                const today = new Date();
                const dateStr = today.toISOString().slice(0,10).replace(/-/g,'');
                const randomSeq = String(Math.floor(Math.random()*999)).padStart(3,'0');
                codeInput.value = `WH-${dateStr}-${randomSeq}`;
            }
            // 設置狀態默認值
            const statusInput = document.getElementById('field_status');
            if (statusInput) {
                const statusField = this.config.formFields.find(f => f.key === 'status');
                if (statusField) {
                    const defaultValue = statusField.defaultValue || statusField.default;
                    if (defaultValue && (!statusInput.value || statusInput.value === '')) {
                        statusInput.value = defaultValue;
                    }
                }
            }
        }
        
        // 產品表單：設置狀態默認值
        if (this.pageName === 'products' && !this.isEdit) {
            const statusInput = document.getElementById('field_status');
            if (statusInput) {
                const statusField = this.config.formFields.find(f => f.key === 'status');
                if (statusField) {
                    const defaultValue = statusField.defaultValue || statusField.default;
                    if (defaultValue && (!statusInput.value || statusInput.value === '')) {
                        statusInput.value = defaultValue;
                    }
                }
            }
        }
        
        // 在表單渲染後顯示 loading overlay（新建和編輯模式都需要）
            // 注意：这里不立即显示，而是在 init() 中的 loadRelationOptions() 之前显示
            // 这样可以确保加载屏幕在真正需要的时候显示，避免闪烁
            // 加载屏幕会在 init() 方法中，loadRelationOptions() 调用前显示
        
        // 如果是優惠券表單，添加條件管理區域
        if (this.pageName === 'coupons') {
            this.renderCouponConditions();
        }
        
        // 如果是產品表單，添加產品屬性管理區域
        if (this.pageName === 'products') {
            this.renderProductAttributes();
        }

        // 如果是薪資表單，添加供款明細區域
        if (this.pageName === 'payrolls') {
            this.renderPayrollContributions();
        }
        
        // 如果是客戶表單，初始化地址管理
        if (this.pageName === 'customers') {
            this.customerAddresses = [];
            // 確保地址管理 UI 已添加到 DOM 後再初始化
            setTimeout(() => {
                this.initCustomerAddresses();
            }, 100);
        }
        
        // 如果是推廣表單，設置發送方式切換邏輯
        if (this.pageName === 'promotions') {
            this.setupPromotionScheduleToggle();
        }
        
        // 如果是產品表單，設置服務套票切換邏輯（在 setupDependencies 之後）
        if (this.pageName === 'products') {
            // 延遲執行，確保 setupDependencies 已完成
            setTimeout(() => {
                this.setupServicePackageToggle();
                // 強制檢查並初始化對應服務字段（如果應該顯示）
                const servicePackageSelect = document.getElementById('field_is_service_package');
                const serviceSelectContainer = document.getElementById('field_container_service_package_service_id');
                if (servicePackageSelect && serviceSelectContainer) {
                    if (servicePackageSelect.value === 'true') {
                        // 如果已經是 true，強制顯示並初始化
                        this.toggleServicePackageService(servicePackageSelect);
                    }
                }
            }, 300);
        }
        
        // 初始化 HTML 編輯器（Quill.js）
        this.initHtmlEditors();
        
        // 初始化 button-group 字段的事件綁定
        setTimeout(() => {
            this.initButtonGroups();
        }, 100);
        
        // 注意：loadRelationOptions() 現在在 init() 中調用，確保在載入草稿和預留編號之前完成
        // 觸發表單按鈕欄設置（延遲執行，確保 DOM 已更新）
        // 注意：在編輯模式下，需要等待數據加載完成後再設置按鈕欄
        if (this.isEdit && this.itemId) {
            // 編輯模式：等待數據加載完成後再設置按鈕欄
            // 這將在 loadItemData() 完成後通過 populateForm() 觸發
        } else {
            // 新建模式：等待所有初始化完成後再設置按鈕欄
            // 這將在 init() 完成後通過 setTimeout 觸發
        }
    }

    // 客戶地址管理
    initCustomerAddresses() {
        if (this.pageName !== 'customers') return;
        
        const t = (key, fallback) => {
            try {
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const v = I18n.t(key);
                    if (v && v !== key) return v;
                }
            } catch (e) {
                // ignore
            }
            return fallback;
        };
        
        // 渲染地址管理 UI
        const container = document.getElementById('customerAddressManagementContainer');
        if (!container) {
            console.error(t('customers.addressManagement.containerMissing', '地址管理容器不存在'));
            return;
        }
        
        container.innerHTML = '<div class="mb-4">' +
            '<div class="d-flex justify-content-between align-items-center mb-3">' +
            `<h5 class="mb-0">${t('customers.addressManagement.title','地址管理')}</h5>` +
            '<button type="button" class="btn btn-sm btn-primary" onclick="window.dynamicForm && window.dynamicForm.addCustomerAddress()">' +
            `<i class="bi bi-plus"></i> ${t('customers.addressManagement.add','新增地址')}` +
            '</button>' +
            '</div>' +
            '<div class="table-responsive">' +
            '<table class="table table-bordered" id="customerAddressesTable">' +
            '<thead>' +
            '<tr>' +
            `<th>${t('fields.country','國家')}</th>` +
            `<th>${t('fields.region','地區')}</th>` +
            `<th>${t('fields.postalCode','郵政編碼')}</th>` +
            `<th>${t('customers.addressManagement.columns.address','地址')}</th>` +
            `<th>${t('customers.addressManagement.columns.default','默認')}</th>` +
            `<th class="customer-address-actions-header">${t('common.actions','操作')}</th>` +
            '</tr>' +
            '</thead>' +
            '<tbody id="customerAddressesTbody">' +
            '<tr>' +
            `<td colspan="6" class="text-center text-muted">${t('customers.addressManagement.empty','暫無地址')}</td>` +
            '</tr>' +
            '</tbody>' +
            '</table>' +
            '</div>' +
            '</div>';
        
        // 如果是編輯模式，載入現有地址
        if (this.isEdit && this.itemId) {
            this.loadCustomerAddresses();
        } else {
            // 新建模式，初始化空數組
            this.customerAddresses = [];
            this.renderCustomerAddresses();
        }
        
        // 綁定事件
        const tbody = document.getElementById('customerAddressesTbody');
        if (tbody) {
            tbody.addEventListener('click', (e) => {
                if (e.target.closest('.btn-edit-address')) {
                    const index = parseInt(e.target.closest('tr').dataset.index);
                    this.editCustomerAddress(index);
                } else if (e.target.closest('.btn-delete-address')) {
                    const index = parseInt(e.target.closest('tr').dataset.index);
                    this.deleteCustomerAddress(index);
                } else if (e.target.closest('.btn-set-default-address')) {
                    const index = parseInt(e.target.closest('tr').dataset.index);
                    this.setDefaultAddress(index);
                }
            });
        }
    }

    // 自動計算「客戶地址子表」操作列寬度（mobile/desktop 都可用）
    calculateCustomerAddressActionColumnWidth() {
        // 等待 DOM 更新完成
        setTimeout(() => {
            const table = document.getElementById('customerAddressesTable');
            if (!table) return;

            const actionCells = table.querySelectorAll('td.customer-address-actions-cell');
            if (!actionCells || actionCells.length === 0) return;

            let maxWidth = 0;
            actionCells.forEach(cell => {
                const buttons = cell.querySelectorAll('button, a.btn');
                if (!buttons || buttons.length === 0) return;

                let totalButtonWidth = 0;
                buttons.forEach(btn => {
                    void btn.offsetWidth;
                    totalButtonWidth += btn.offsetWidth || btn.scrollWidth || 0;
                });
                const buttonSpacing = buttons.length * 10;
                const cellWidth = totalButtonWidth + buttonSpacing;
                if (cellWidth > maxWidth) maxWidth = cellWidth;
            });

            const calculatedWidth = maxWidth || 0;
            const actionHeader = table.querySelector('thead th.customer-address-actions-header') || table.querySelector('thead th:last-child');
            if (actionHeader) {
                actionHeader.style.width = calculatedWidth + 'px';
                actionHeader.style.maxWidth = calculatedWidth + 'px';
            }
            actionCells.forEach(cell => {
                cell.style.width = calculatedWidth + 'px';
            });
        }, 150);
    }

    // 自動計算「產品屬性子表」操作列寬度（與 list 的操作欄呈現一致：不固定百分比，必要時自動撐開）
    calculateProductAttributeActionColumnWidth() {
        setTimeout(() => {
            const tbody = document.getElementById('productAttributesList');
            if (!tbody) return;
            const table = tbody.closest('table');
            if (!table) return;

            const actionCells = table.querySelectorAll('td.product-attr-actions-cell');
            if (!actionCells || actionCells.length === 0) return;

            let maxWidth = 0;
            actionCells.forEach(cell => {
                const buttons = cell.querySelectorAll('button, a.btn');
                if (!buttons || buttons.length === 0) return;
                let total = 0;
                buttons.forEach(btn => {
                    void btn.offsetWidth;
                    total += btn.offsetWidth || btn.scrollWidth || 0;
                });
                const spacing = buttons.length * 10;
                const w = total + spacing;
                if (w > maxWidth) maxWidth = w;
            });

            const calculatedWidth = maxWidth || 0;
            const actionHeader = table.querySelector('thead th.product-attr-actions-header') || table.querySelector('thead th:last-child');
            if (actionHeader && calculatedWidth > 0) {
                actionHeader.style.width = calculatedWidth + 'px';
                actionHeader.style.maxWidth = calculatedWidth + 'px';
            }
            actionCells.forEach(cell => {
                if (calculatedWidth > 0) cell.style.width = calculatedWidth + 'px';
            });
        }, 150);
    }
    
    async loadCustomerAddresses() {
        if (!this.itemId) return;
        
        try {
            const response = await App.apiRequest(`/api/v1/customers/${this.itemId}/addresses`);
            this.customerAddresses = response.data || [];
            // 預載國家/地區顯示名（依語系），確保列表顯示為翻譯後文字
            this.preloadAddressDisplayMaps().then(() => this.renderCustomerAddresses());
            this.renderCustomerAddresses();
        } catch (error) {
            console.error('載入地址失敗:', error);
            this.customerAddresses = [];
            this.renderCustomerAddresses();
        }
    }

    // 預先載入國家/地區顯示映射（顯示用），保存仍使用英文 name_en
    async preloadAddressDisplayMaps() {
        if (this.pageName !== 'customers') return;
        const lang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
        this._countriesByLang = this._countriesByLang || {};
        this._regionsByLang = this._regionsByLang || {};

        // Countries
        if (!this._countriesByLang[lang]) {
            try {
                // Must use API v1 endpoint (supports local country_translations.json)
                const res = await App.apiRequest(`/api/v1/countries?lang=${encodeURIComponent(lang)}&limit=300`);
                const m = {};
                (res.data || []).forEach(it => {
                    if (it && it.code) m[it.code] = it.name || it.code;
                });
                this._countriesByLang[lang] = m;
            } catch (e) {
                this._countriesByLang[lang] = {};
            }
        }

        // Regions: load only needed countries
        const needed = Array.from(new Set((this.customerAddresses || []).map(a => (a && a.country_code) ? String(a.country_code) : '').filter(Boolean)));
        this._regionsByLang[lang] = this._regionsByLang[lang] || {};

        await Promise.all(needed.map(async (cc) => {
            if (this._regionsByLang[lang][cc]) return;
            try {
                // Must use API v1 endpoint (supports local region_translations.json)
                const res = await App.apiRequest(`/api/v1/country-regions?country_code=${encodeURIComponent(cc)}&lang=${encodeURIComponent(lang)}&limit=1000`);
                const m = {};
                (res.data || []).forEach(r => {
                    if (r && r.code) m[r.code] = r.name || r.code;
                });
                this._regionsByLang[lang][cc] = m;
            } catch (e) {
                this._regionsByLang[lang][cc] = {};
            }
        }));
    }
    
    renderCustomerAddresses() {
        const tbody = document.getElementById('customerAddressesTbody');
        if (!tbody) return;

        const t = (key, fallback) => {
            try {
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const v = I18n.t(key);
                    if (v && v !== key) return v;
                }
            } catch (e) {
                // ignore
            }
            return fallback;
        };
        
        // 過濾掉標記為刪除的地址
        const visibleAddresses = (this.customerAddresses || []).filter(addr => !addr._deleted);
        
        if (visibleAddresses.length === 0) {
            tbody.innerHTML = `<tr><td colspan="6" class="text-center text-muted">${t('customers.addressManagement.empty','暫無地址')}</td></tr>`;
            this.calculateCustomerAddressActionColumnWidth();
            return;
        }
        
        const currentLang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
        const countryMap = this._countriesByLang && this._countriesByLang[currentLang] ? this._countriesByLang[currentLang] : null;
        const regionsByLang = this._regionsByLang && this._regionsByLang[currentLang] ? this._regionsByLang[currentLang] : null;

        const getCountryDisplay = (addr) => {
            const code = (addr && addr.country_code) ? String(addr.country_code) : '';
            if (countryMap && code && countryMap[code]) return countryMap[code];
            return addr.country_name || code || '';
        };
        const getRegionDisplay = (addr) => {
            const cc = (addr && addr.country_code) ? String(addr.country_code) : '';
            const rc = (addr && addr.region_code) ? String(addr.region_code) : '';
            if (regionsByLang && cc && rc && regionsByLang[cc] && regionsByLang[cc][rc]) return regionsByLang[cc][rc];
            return addr.region_name || rc || '';
        };

        tbody.innerHTML = visibleAddresses.map((addr, visibleIndex) => {
            // 找到原始索引
            const originalIndex = this.customerAddresses.findIndex(a => a === addr);
            return `
            <tr data-index="${originalIndex}" ${addr._deleted ? 'style="opacity: 0.5; text-decoration: line-through;"' : ''}>
                <td>${getCountryDisplay(addr)}</td>
                <td>${getRegionDisplay(addr)}</td>
                <td>${addr.postal_code || ''}</td>
                <td>${addr.address_line1 || ''} ${addr.address_line2 || ''}</td>
                <td>
                    ${addr.is_default ? `<span class="badge bg-primary">${t('customers.addressManagement.badges.default','默認')}</span>` : `<span class="text-muted">${t('common.no','否')}</span>`}
                    ${addr._deleted ? `<span class="badge bg-danger">${t('customers.addressManagement.badges.pendingDelete','待刪除')}</span>` : ''}
                </td>
                <td class="customer-address-actions-cell actions-cell" style="white-space: nowrap;">
                    <button type="button" class="btn btn-sm btn-outline-primary btn-edit-address" title="${t('common.edit','編輯')}" ${addr._deleted ? 'disabled' : ''}>
                        <i class="bi bi-pencil"></i>
                    </button>
                    <button type="button" class="btn btn-sm btn-outline-success btn-set-default-address" title="${t('customers.addressManagement.setDefault','設為默認')}" ${addr.is_default || addr._deleted ? 'disabled' : ''}>
                        <i class="bi bi-star"></i>
                    </button>
                    <button type="button" class="btn btn-sm btn-outline-danger btn-delete-address" title="${t('common.delete','刪除')}" ${addr._deleted ? 'disabled' : ''}>
                        <i class="bi bi-trash"></i>
                    </button>
                </td>
            </tr>
        `;
        }).join('');

        this.calculateCustomerAddressActionColumnWidth();
    }
    
    addCustomerAddress() {
        this.showAddressModal();
    }
    
    editCustomerAddress(index) {
        // 確保索引是數字
        const addressIndex = parseInt(index);
        if (isNaN(addressIndex) || addressIndex < 0 || addressIndex >= this.customerAddresses.length) {
            console.error('無效的地址索引:', index);
            return;
        }
        const address = this.customerAddresses[addressIndex];
        if (!address || address._deleted) {
            console.error('地址不存在或已標記為刪除');
            return;
        }
        this.showAddressModal(address, addressIndex);
    }
    
    deleteCustomerAddress(index) {
        const confirmText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('customers.addressManagement.confirmDelete') : '確定要刪除這個地址嗎？';
        if (!confirm(confirmText)) return;
        
        const address = this.customerAddresses[index];
        if (address.id && this.itemId) {
            // 已保存的地址，標記為待刪除（在保存客戶時真正刪除）
            address._deleted = true;
            this.renderCustomerAddresses();
            const msg = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('customers.addressManagement.markedForDelete') : '地址已標記為刪除，將在保存客戶時生效';
            App.showAlert(msg, 'info');
        } else {
            // 未保存的地址，直接從數組中刪除
            this.customerAddresses.splice(index, 1);
            this.renderCustomerAddresses();
        }
        
        // 觸發草稿保存（使用與表單相同的延遲機制）
        if (this.pageName === 'customers' && !this.isEdit) {
            this.hasUnsavedChanges = true;
            const saveInterval = 2000; // 2秒延遲
            clearTimeout(this.saveTimer);
            this.saveTimer = setTimeout(() => {
                if (this.hasUnsavedChanges) {
                    this.saveDraft();
                }
            }, saveInterval);
        }
    }
    
    setDefaultAddress(index) {
        // 確保索引是數字
        const addressIndex = parseInt(index);
        if (isNaN(addressIndex) || addressIndex < 0 || addressIndex >= this.customerAddresses.length) {
            console.error('無效的地址索引:', index);
            return;
        }
        const address = this.customerAddresses[addressIndex];
        if (!address || address._deleted) {
            console.error('地址不存在或已標記為刪除');
            return;
        }
        
        // 將所有地址設為非默認
        this.customerAddresses.forEach(addr => {
            if (!addr._deleted) {
                addr.is_default = false;
            }
        });
        // 設置當前地址為默認
        address.is_default = true;
        this.renderCustomerAddresses();
        // 不立即保存到數據庫，等待保存客戶時一起保存
        
        // 觸發草稿保存（使用與表單相同的延遲機制）
        if (this.pageName === 'customers' && !this.isEdit) {
            this.hasUnsavedChanges = true;
            const saveInterval = 2000; // 2秒延遲
            clearTimeout(this.saveTimer);
            this.saveTimer = setTimeout(() => {
                if (this.hasUnsavedChanges) {
                    this.saveDraft();
                }
            }, saveInterval);
        }
    }
    
    showAddressModal(address = null, index = null) {
        // 創建或顯示地址編輯模態框
        let modal = document.getElementById('customerAddressModal');
        let bsModal = null;
        let loadRegionsForCountry = null;

        const t = (key, fallback) => {
            if (typeof I18n !== 'undefined' && I18n.t) {
                const v = I18n.t(key);
                if (v && v !== key) return v;
            }
            return fallback;
        };
        
        if (!modal) {
            modal = document.createElement('div');
            modal.className = 'modal fade';
            modal.id = 'customerAddressModal';
            modal.innerHTML = `
                <div class="modal-dialog modal-lg">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title">${address ? t('customers.addressModal.editTitle','編輯地址') : t('customers.addressModal.addTitle','新增地址')}</h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                        </div>
                        <div class="modal-body">
                            <div class="mb-3">
                                <label class="form-label">${t('fields.country','國家')} <span class="text-danger">*</span></label>
                                <select class="form-select" id="addressCountry" style="width: 100%;" required>
                                    <option value="">${t('customers.addressModal.selectCountry','請選擇國家')}</option>
                                </select>
                            </div>
                            <div class="row mb-3">
                                <div class="col-md-6">
                                    <label class="form-label">${t('fields.region','地區')}</label>
                                    <select class="form-select" id="addressRegion" style="width: 100%;">
                                        <option value="">${t('customers.addressModal.selectRegion','請選擇地區')}</option>
                                    </select>
                                </div>
                                <div class="col-md-6">
                                    <label class="form-label">${t('fields.postalCode','郵政編碼')}</label>
                                    <input type="text" class="form-control" id="addressPostalCode">
                                </div>
                            </div>
                            <div class="mb-3">
                                <label class="form-label">${t('customers.addressModal.addressLine1','地址第一行')} <span class="text-danger">*</span></label>
                                <input type="text" class="form-control" id="addressLine1" required>
                            </div>
                            <div class="mb-3">
                                <label class="form-label">${t('customers.addressModal.addressLine2','地址第二行')}</label>
                                <input type="text" class="form-control" id="addressLine2">
                            </div>
                            <div class="mb-3">
                                <div class="form-check">
                                    <input class="form-check-input" type="checkbox" id="addressIsDefault">
                                    <label class="form-check-label" for="addressIsDefault">${t('customers.addressModal.setDefault','設為默認地址')}</label>
                                </div>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">${t('common.cancel','取消')}</button>
                            <button type="button" class="btn btn-primary" id="saveAddressBtn" data-address-index="${index !== null ? index : ''}">${t('common.save','保存')}</button>
                        </div>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
            
            const uiLang = () => {
                try {
                    return (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
                } catch (e) {
                    return 'zh';
                }
            };

            // 載入地區的函數（顯示依語系；保存用英文 name_en）
            loadRegionsForCountry = (countryCode, currentRegionCode = null, currentRegionNameEn = null) => {
                const regionSelect = $('#addressRegion');
                if (!countryCode) {
                    regionSelect.empty().append(`<option value="">${t('customers.addressModal.selectCountryFirst','請先選擇國家')}</option>`).prop('disabled', true).trigger('change');
                    return;
                }
                
                regionSelect.prop('disabled', false);
                regionSelect.empty().append(`<option value="">${t('common.loading','載入中...')}</option>`).trigger('change');
                
                // 從 API 獲取地區
                fetch(`/api/v1/country-regions?country_code=${encodeURIComponent(countryCode)}&lang=${encodeURIComponent(uiLang())}`)
                    .then(res => res.json())
                    .then(data => {
                        regionSelect.empty();
                        regionSelect.append(`<option value="">${t('customers.addressModal.selectRegion','請選擇地區')}</option>`);
                        
                        // 添加預定義的地區
                        const seen = new Set();
                        if (data.data && data.data.length > 0) {
                            data.data.forEach(region => {
                                const opt = new Option(region.name || region.code, region.code);
                                // 保存用英文：優先 name_en，否則 fallback region.name
                                if (opt && opt.dataset) {
                                    opt.dataset.nameEn = region.name_en || region.name || '';
                                }
                                regionSelect.append(opt);
                                seen.add(String(region.code || ''));
                            });
                        }

                        // 若當前地址的 region_code 不在列表內，補一個（顯示/保存都用原值）
                        if (currentRegionCode && !seen.has(String(currentRegionCode))) {
                            const fallbackText = currentRegionNameEn || currentRegionCode;
                            const cur = new Option(fallbackText, currentRegionCode, true, true);
                            if (cur && cur.dataset) cur.dataset.nameEn = currentRegionNameEn || fallbackText;
                            regionSelect.append(cur);
                        }
                        
                        // 如果沒有預定義地區且沒有當前地區，顯示提示
                        if ((!data.data || data.data.length === 0) && !currentRegionCode) {
                            regionSelect.append(`<option value="">${t('customers.addressModal.noPredefinedRegions','該國家無預定義地區')}</option>`);
                        }
                        
                        // 如果有當前地區代碼，設置選中值
                        if (currentRegionCode) {
                            regionSelect.val(currentRegionCode).trigger('change');
                        } else {
                            regionSelect.trigger('change');
                        }
                        regionSelect.trigger('change');
                    })
                    .catch(error => {
                        console.error('載入地區失敗:', error);
                        regionSelect.empty();
                        regionSelect.append(`<option value="">${t('customers.addressModal.selectRegion','請選擇地區')}</option>`);
                        // 即使載入失敗，也添加當前地區（如果有的話）
                        if (currentRegionCode) {
                            const fallbackText = currentRegionNameEn || currentRegionCode;
                            const cur = new Option(fallbackText, currentRegionCode, true, true);
                            if (cur && cur.dataset) cur.dataset.nameEn = currentRegionNameEn || fallbackText;
                            regionSelect.append(cur);
                        } else {
                            regionSelect.append(`<option value="">${t('common.loadError','載入失敗')}</option>`);
                        }
                        regionSelect.trigger('change');
                    });
            };
            
            // 創建 Bootstrap Modal 實例
            bsModal = new bootstrap.Modal(modal);
            
            // 監聽模態框完全顯示後再初始化 Select2
            modal.addEventListener('shown.bs.modal', function() {
                // 初始化國家選擇器
                $('#addressCountry').select2({
                    theme: 'bootstrap-5',
                    placeholder: t('customers.addressModal.searchCountry','搜索國家...'),
                    allowClear: true,
                    dropdownParent: $('#customerAddressModal'), // 確保下拉菜單在模態框內顯示
                    width: '100%',
                    ajax: {
                        url: (function() {
                            const lang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
                            return `/api/v1/countries?lang=${encodeURIComponent(lang)}`;
                        })(),
                        dataType: 'json',
                        delay: 250,
                        data: function(params) {
                            return { search: params.term || '', page: params.page || 1, limit: 250 };
                        },
                        processResults: function(data, params) {
                            params.page = params.page || 1;
                            return {
                                results: (data.data || []).map(item => ({
                                    id: item.code,
                                    text: item.name || item.code,
                                    name_en: item.name_en || '',
                                    name_zh: item.name_zh || ''
                                })),
                                pagination: {
                                    // 如果返回了 250 條記錄，且總數大於當前頁 * 250，則還有更多數據
                                    more: (data.data || []).length === 250 && (data.total || 0) > params.page * 250
                                }
                            };
                        },
                        cache: true
                    },
                    minimumInputLength: 0
                }).on('change', function() {
                    // 當選擇國家時，載入對應的地區
                    const countryCode = $(this).val();
                    if (loadRegionsForCountry) {
                        loadRegionsForCountry(countryCode);
                    }
                });
                
                // 初始化地區選擇器
                $('#addressRegion').select2({
                    theme: 'bootstrap-5',
                    placeholder: t('customers.addressModal.selectCountryFirst','請先選擇國家'),
                    allowClear: true,
                    dropdownParent: $('#customerAddressModal'), // 確保下拉菜單在模態框內顯示
                    width: '100%',
                    disabled: true
                });
            }, { once: true });
        } else {
            // 如果模態框已存在，獲取現有的 Modal 實例
            bsModal = bootstrap.Modal.getInstance(modal) || new bootstrap.Modal(modal);
            
            // 載入地區的函數（需要在這裡定義，因為模態框已存在）
            loadRegionsForCountry = (countryCode, currentRegionCode = null, currentRegionNameEn = null) => {
                const regionSelect = $('#addressRegion');
                if (!countryCode) {
                    regionSelect.empty().append(`<option value="">${t('customers.addressModal.selectCountryFirst','請先選擇國家')}</option>`).prop('disabled', true).trigger('change');
                    return;
                }
                
                regionSelect.prop('disabled', false);
                regionSelect.empty().append(`<option value="">${t('common.loading','載入中...')}</option>`).trigger('change');
                
                // 從 API 獲取地區
                const lang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
                fetch(`/api/v1/country-regions?country_code=${encodeURIComponent(countryCode)}&lang=${encodeURIComponent(lang)}`)
                    .then(res => res.json())
                    .then(data => {
                        regionSelect.empty();
                        regionSelect.append(`<option value="">${t('customers.addressModal.selectRegion','請選擇地區')}</option>`);
                        
                        // 添加預定義的地區
                        const seen = new Set();
                        if (data.data && data.data.length > 0) {
                            data.data.forEach(region => {
                                const opt = new Option(region.name || region.code, region.code);
                                if (opt && opt.dataset) opt.dataset.nameEn = region.name_en || region.name || '';
                                regionSelect.append(opt);
                                seen.add(String(region.code || ''));
                            });
                        }

                        if (currentRegionCode && !seen.has(String(currentRegionCode))) {
                            const fallbackText = currentRegionNameEn || currentRegionCode;
                            const cur = new Option(fallbackText, currentRegionCode, true, true);
                            if (cur && cur.dataset) cur.dataset.nameEn = currentRegionNameEn || fallbackText;
                            regionSelect.append(cur);
                        }
                        
                        // 如果沒有預定義地區且沒有當前地區，顯示提示
                        if ((!data.data || data.data.length === 0) && !currentRegionCode) {
                            regionSelect.append(`<option value="">${t('customers.addressModal.noPredefinedRegions','該國家無預定義地區')}</option>`);
                        }
                        
                        // 如果有當前地區代碼，設置選中值
                        if (currentRegionCode) {
                            regionSelect.val(currentRegionCode).trigger('change');
                        } else {
                            regionSelect.trigger('change');
                        }
                    })
                    .catch(error => {
                        console.error('載入地區失敗:', error);
                        regionSelect.empty();
                        regionSelect.append(`<option value="">${t('customers.addressModal.selectRegion','請選擇地區')}</option>`);
                        // 即使載入失敗，也添加當前地區（如果有的話）
                        if (currentRegionCode) {
                            const fallbackText = currentRegionNameEn || currentRegionCode;
                            const cur = new Option(fallbackText, currentRegionCode, true, true);
                            if (cur && cur.dataset) cur.dataset.nameEn = currentRegionNameEn || fallbackText;
                            regionSelect.val(currentRegionCode).trigger('change');
                        } else {
                            regionSelect.append(`<option value="">${t('common.loadError','載入失敗')}</option>`);
                            regionSelect.trigger('change');
                        }
                    });
            };
            
            // 確保 Select2 已初始化（如果還沒有）
            const initSelect2 = () => {
                // 檢查國家選擇器是否已初始化
                if (!$('#addressCountry').hasClass('select2-hidden-accessible')) {
                    $('#addressCountry').select2({
                        theme: 'bootstrap-5',
                        placeholder: t('customers.addressModal.searchCountry','搜索國家...'),
                        allowClear: true,
                        dropdownParent: $('#customerAddressModal'),
                        width: '100%',
                        ajax: {
                            url: (function() {
                                const lang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
                                return `/api/v1/countries?lang=${encodeURIComponent(lang)}`;
                            })(),
                            dataType: 'json',
                            delay: 250,
                            data: function(params) {
                                return { search: params.term || '', page: params.page || 1, limit: 250 };
                            },
                            processResults: function(data) {
                                return {
                                    results: (data.data || []).map(item => ({
                                        id: item.code,
                                        text: item.name || item.code,
                                        name_en: item.name_en || '',
                                        name_zh: item.name_zh || ''
                                    }))
                                };
                            },
                            cache: true
                        },
                        minimumInputLength: 0
                    }).on('change', function() {
                        const countryCode = $(this).val();
                        if (loadRegionsForCountry) {
                            loadRegionsForCountry(countryCode);
                        }
                    });
                }
                
                // 檢查地區選擇器是否已初始化
                if (!$('#addressRegion').hasClass('select2-hidden-accessible')) {
                    $('#addressRegion').select2({
                        theme: 'bootstrap-5',
                        placeholder: t('customers.addressModal.selectCountryFirst','請先選擇國家'),
                        allowClear: true,
                        dropdownParent: $('#customerAddressModal'),
                        width: '100%',
                        disabled: true
                    });
                }
            };
            
            // 如果模態框已經顯示，立即初始化；否則等待顯示後初始化
            if (modal.classList.contains('show')) {
                initSelect2();
            } else {
                modal.addEventListener('shown.bs.modal', initSelect2, { once: true });
            }
            
            // 更新保存按鈕的索引（每次打開模態框時更新）
            const saveBtn = document.getElementById('saveAddressBtn');
            if (saveBtn) {
                saveBtn.setAttribute('data-address-index', index !== null ? index : '');
            }
        }
        
        // 設置保存按鈕的點擊事件（只在第一次創建時設置）
        if (!modal.hasAttribute('data-save-listener')) {
            modal.setAttribute('data-save-listener', 'true');
            modal.addEventListener('click', (e) => {
                if (e.target && e.target.id === 'saveAddressBtn') {
                    const saveBtn = e.target;
                    const addressIndex = saveBtn.getAttribute('data-address-index');
                    const indexValue = addressIndex !== null && addressIndex !== '' ? parseInt(addressIndex) : null;
                    if (window.dynamicForm) {
                        window.dynamicForm.saveAddressModal(indexValue);
                    }
                }
            });
        } else {
            // 如果監聽器已設置，只需要更新索引
            const saveBtn = document.getElementById('saveAddressBtn');
            if (saveBtn) {
                saveBtn.setAttribute('data-address-index', index !== null ? index : '');
            }
        }
        
        // 填充數據的函數
        const populateData = () => {
            if (address) {
                // 確保 Select2 已初始化
                if (!$('#addressCountry').hasClass('select2-hidden-accessible')) {
                    setTimeout(populateData, 100);
                    return;
                }
                
                // 設置國家（需要先添加選項）
                if ($('#addressCountry').find(`option[value="${address.country_code}"]`).length === 0) {
                    // 顯示用：依語系取 name；保存用：dataset.nameEn 固定英文
                    const lang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
                    const fallbackText = address.country_name || address.country_code;
                    const countryOption = new Option(fallbackText, address.country_code, true, true);
                    if (countryOption && countryOption.dataset) {
                        countryOption.dataset.nameEn = address.country_name || '';
                    }
                    // 盡量拉一次翻譯（不阻塞）
                    fetch(`/api/v1/countries?lang=${encodeURIComponent(lang)}&search=${encodeURIComponent(address.country_code)}&limit=5`)
                        .then(r => r.json())
                        .then(d => {
                            const hit = (d.data || []).find(x => x.code === address.country_code);
                            if (hit) {
                                countryOption.text = hit.name || fallbackText;
                                if (countryOption.dataset) countryOption.dataset.nameEn = hit.name_en || (address.country_name || '');
                                $('#addressCountry').trigger('change.select2');
                            }
                        })
                        .catch(() => {});
                    $('#addressCountry').append(countryOption);
                }
                $('#addressCountry').val(address.country_code).trigger('change');
                
                // 等待國家選擇器更新後再載入地區
                setTimeout(() => {
                    if (address.country_code && loadRegionsForCountry) {
                        // 傳遞當前地址的地區信息，確保即使該國家沒有預定義地區也能顯示
                        loadRegionsForCountry(address.country_code, address.region_code, address.region_name);
                    }
                }, 200);
                
                document.getElementById('addressPostalCode').value = address.postal_code || '';
                document.getElementById('addressLine1').value = address.address_line1 || '';
                document.getElementById('addressLine2').value = address.address_line2 || '';
                document.getElementById('addressIsDefault').checked = address.is_default || false;
            } else {
                $('#addressCountry').val('').trigger('change');
                $('#addressRegion').val('').trigger('change');
                document.getElementById('addressPostalCode').value = '';
                document.getElementById('addressLine1').value = '';
                document.getElementById('addressLine2').value = '';
                document.getElementById('addressIsDefault').checked = false;
            }
        };
        
        // 等待模態框顯示後再填充數據
        modal.addEventListener('shown.bs.modal', () => {
            setTimeout(populateData, 100);
        }, { once: true });
        
        // 顯示模態框
        bsModal.show();
    }
    
    async saveAddressModal(index) {
        const countryCode = $('#addressCountry').val();
        const countryData = $('#addressCountry').select2('data')[0] || {};
        const countryName =
            countryData.name_en ||
            (countryData.element && countryData.element.dataset ? countryData.element.dataset.nameEn : '') ||
            countryData.text ||
            '';
        const regionCode = $('#addressRegion').val() || '';
        const regionData = $('#addressRegion').select2('data')[0] || {};
        const regionName =
            (regionData.element && regionData.element.dataset ? (regionData.element.dataset.nameEn || '') : '') ||
            regionData.name_en ||
            regionData.text ||
            '';
        const postalCode = document.getElementById('addressPostalCode').value;
        const addressLine1 = document.getElementById('addressLine1').value;
        const addressLine2 = document.getElementById('addressLine2').value;
        const isDefault = document.getElementById('addressIsDefault').checked;
        
        if (!countryCode || !addressLine1) {
            App.showAlert('請填寫必填字段', 'warning');
            return;
        }
        
        const addressData = {
            country_code: countryCode,
            country_name: countryName,
            region_code: regionCode,
            region_name: regionName,
            postal_code: postalCode,
            address_line1: addressLine1,
            address_line2: addressLine2,
            is_default: isDefault
        };
        
        // 處理索引：可能是字符串 'null'、數字或 undefined
        let addressIndex = null;
        if (index !== null && index !== 'null' && index !== undefined && index !== '') {
            addressIndex = parseInt(index);
            if (isNaN(addressIndex)) {
                addressIndex = null;
            }
        }
        
        // 如果設置為默認，先將其他地址設為非默認
        if (isDefault) {
            this.customerAddresses.forEach((addr, i) => {
                if (i !== addressIndex && !addr._deleted) {
                    addr.is_default = false;
                }
            });
        }
        
        // 檢查是否為編輯模式：索引有效且存在對應的地址
        if (addressIndex !== null && addressIndex >= 0 && addressIndex < this.customerAddresses.length) {
            // 編輯模式：更新現有地址
            const existing = this.customerAddresses[addressIndex];
            if (existing && !existing._deleted) {
                // 保留原有的 id（如果存在）
                if (existing.id) {
                    addressData.id = existing.id;
                }
                // 使用 Object.assign 合併屬性，而不是完全替換，避免丟失其他屬性
                Object.assign(this.customerAddresses[addressIndex], addressData);
                // 確保 _deleted 標記被清除（如果用戶編輯了已標記為刪除的地址）
                delete this.customerAddresses[addressIndex]._deleted;
            } else {
                // 如果地址不存在或已刪除，當作新增處理
                this.customerAddresses.push(addressData);
            }
        } else {
            // 新增模式
            this.customerAddresses.push(addressData);
        }
        
        this.renderCustomerAddresses();
        
        // 關閉模態框
        bootstrap.Modal.getInstance(document.getElementById('customerAddressModal')).hide();
        // 不顯示"已保存"提示，因為地址只在保存客戶時才真正保存到數據庫
        
        // 觸發草稿保存（使用與表單相同的延遲機制）
        if (this.pageName === 'customers' && !this.isEdit) {
            this.hasUnsavedChanges = true;
            const saveInterval = 2000; // 2秒延遲
            clearTimeout(this.saveTimer);
            this.saveTimer = setTimeout(() => {
                if (this.hasUnsavedChanges) {
                    this.saveDraft();
                }
            }, saveInterval);
        }

        // 更新顯示用翻譯快取（不影響保存值）
        this.preloadAddressDisplayMaps().then(() => this.renderCustomerAddresses());
    }
    
    // 在保存客戶時處理地址
    async saveCustomerAddresses(customerId) {
        if (!customerId || !this.customerAddresses || this.customerAddresses.length === 0) return;
        
        // 先處理刪除：刪除標記為 _deleted 的地址
        for (const address of this.customerAddresses) {
            if (address._deleted && address.id) {
                try {
                    await App.apiRequest(`/api/v1/customers/${customerId}/addresses/${address.id}`, {
                        method: 'DELETE'
                    });
                } catch (error) {
                    console.error('刪除地址失敗:', error);
                    throw error; // 如果刪除失敗，拋出錯誤
                }
            }
        }
        
        // 處理更新和創建：過濾掉已刪除的地址
        const addressesToSave = this.customerAddresses.filter(addr => !addr._deleted);
        
        for (const address of addressesToSave) {
            if (address.id) {
                // 已存在的地址，更新
                try {
                    await App.apiRequest(`/api/v1/customers/${customerId}/addresses/${address.id}`, {
                        method: 'PUT',
                        body: JSON.stringify({
                            country_code: address.country_code,
                            country_name: address.country_name,
                            region_code: address.region_code,
                            region_name: address.region_name,
                            postal_code: address.postal_code,
                            address_line1: address.address_line1,
                            address_line2: address.address_line2,
                            is_default: address.is_default
                        })
                    });
                } catch (error) {
                    console.error('更新地址失敗:', error);
                    throw error; // 如果更新失敗，拋出錯誤
                }
            } else {
                // 新地址，創建
                try {
                    const created = await App.apiRequest(`/api/v1/customers/${customerId}/addresses`, {
                        method: 'POST',
                        body: JSON.stringify({
                            country_code: address.country_code,
                            country_name: address.country_name,
                            region_code: address.region_code,
                            region_name: address.region_name,
                            postal_code: address.postal_code,
                            address_line1: address.address_line1,
                            address_line2: address.address_line2,
                            is_default: address.is_default
                        })
                    });
                    // 更新本地地址的 ID
                    address.id = created.id;
                } catch (error) {
                    console.error('創建地址失敗:', error);
                    throw error; // 如果創建失敗，拋出錯誤
                }
            }
        }
    }

    // users 所屬店舖子表渲染
    renderUserStoreSubTable() {
        if (this.pageName !== 'users') return '';
        return `
            <div class="card mt-4">
                <div class="card-header d-flex align-items-center">
                    <h5 class="mb-0" data-i18n="users.stores.title">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.title') : '所屬店舖'}</h5>
                </div>
                <div class="card-body">
                    <div class="table-responsive">
                        <table class="table table-hover align-middle" id="userStoreSubTable">
                            <thead>
                                <tr>
                                    <th data-i18n="users.stores.columns.storeName">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.columns.storeName') : '店舖名稱'}</th>
                                    <th data-i18n="users.stores.columns.code">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.columns.code') : '編號'}</th>
                                    <th style="width:120px;" data-i18n="users.stores.columns.belongs">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.columns.belongs') : '屬於店舖'}</th>
                                    <th style="width:120px;" data-i18n="users.stores.columns.default">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.columns.default') : '系統預設'}</th>
                                </tr>
                            </thead>
                            <tbody id="userStoreSubTableBody">
                                <tr><td colspan="4" class="text-center text-muted" data-i18n="common.loading">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</td></tr>
                            </tbody>
                        </table>
                    </div>
                    <small class="text-muted" data-i18n="users.stores.hint">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.hint') : '僅顯示活躍店舖，「系統預設」需先勾選「屬於店舖」後才可選。'}</small>
                </div>
            </div>
        `;
    }

    async loadUserStoresSubTable() {
        if (this.pageName !== 'users') return;
        try {
            // 取得活躍店舖（不影響 /stores API，只讀取）
            const res = await App.apiRequest('/api/v1/stores?status=active&limit=1000');
            this.activeStores = res.data || [];
        } catch (err) {
            console.warn('載入店舖列表失敗', err);
            this.activeStores = [];
        }

        const tbody = document.getElementById('userStoreSubTableBody');
        if (!tbody) return;

        if (!this.activeStores.length) {
            const empty = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.empty') : '沒有可用的店舖';
            tbody.innerHTML = `<tr><td colspan="4" class="text-center text-muted" data-i18n="users.stores.empty">${empty}</td></tr>`;
            return;
        }

        const rows = this.activeStores.map(store => {
            const belongId = `userstore_belong_${store.id}`;
            const defaultId = `userstore_default_${store.id}`;
            return `
                <tr data-store-id="${store.id}">
                    <td>${store.name || ''}</td>
                    <td>${store.code || ''}</td>
                    <td>
                        <div class="form-check">
                            <input class="form-check-input userstore-belong" type="checkbox" id="${belongId}" data-store-id="${store.id}" />
                            <label class="form-check-label" for="${belongId}" data-i18n="users.stores.columns.belongs">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.columns.belongs') : '屬於店舖'}</label>
                        </div>
                    </td>
                    <td>
                        <div class="form-check">
                            <input class="form-check-input userstore-default" type="radio" name="userstore_default" id="${defaultId}" data-store-id="${store.id}" disabled />
                            <label class="form-check-label" for="${defaultId}" data-i18n="users.stores.columns.default">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('users.stores.columns.default') : '系統預設'}</label>
                        </div>
                    </td>
                </tr>
            `;
        }).join('');

        tbody.innerHTML = rows;

        // 綁定互斥/依賴邏輯
        this.activeStores.forEach(store => {
            const belongCheckbox = document.getElementById(`userstore_belong_${store.id}`);
            const defaultRadio = document.getElementById(`userstore_default_${store.id}`);
            if (belongCheckbox && defaultRadio) {
                const syncState = () => {
                    if (belongCheckbox.checked) {
                        defaultRadio.disabled = false;
                        defaultRadio.closest('td').style.opacity = '1';
                    } else {
                        defaultRadio.checked = false;
                        defaultRadio.disabled = true;
                        defaultRadio.closest('td').style.opacity = '0.5';
                    }
                };
                belongCheckbox.addEventListener('change', syncState);
                syncState();
            }
        });
    }

    applyUserStoreSelections(item) {
        if (this.pageName !== 'users') return;
        if (!item || !item.extra_fields) return;
        const stores = item.extra_fields.stores || [];
        if (!Array.isArray(stores)) return;

        stores.forEach(s => {
            const belongCheckbox = document.getElementById(`userstore_belong_${s.store_id}`);
            const defaultRadio = document.getElementById(`userstore_default_${s.store_id}`);
            if (belongCheckbox) {
                belongCheckbox.checked = true;
                belongCheckbox.dispatchEvent(new Event('change'));
            }
            if (defaultRadio && s.is_default) {
                defaultRadio.checked = true;
            }
        });
    }

    collectUserStoreData() {
        if (this.pageName !== 'users') return [];
        const tbody = document.getElementById('userStoreSubTableBody');
        if (!tbody) return [];
        const rows = tbody.querySelectorAll('tr[data-store-id]');
        const result = [];
        rows.forEach(row => {
            const storeId = row.getAttribute('data-store-id');
            const belong = row.querySelector('.userstore-belong');
            const def = row.querySelector('.userstore-default');
            const belongsToStore = belong && belong.checked;
            const isDefault = def && def.checked; // radio button 的 checked 属性
            if (belongsToStore) {
                result.push({
                    store_id: storeId,
                    is_default: !!isDefault
                });
            }
        });
        return result;
    }
    
    // 設置服務套票切換邏輯
    setupServicePackageToggle() {
        const servicePackageSelect = document.getElementById('field_is_service_package');
        const serviceSelectContainer = document.getElementById('field_container_service_package_service_id');
        
        if (!servicePackageSelect || !serviceSelectContainer) {
            console.warn('服務套票字段未找到，可能尚未初始化');
            return;
        }
        
        // 初始狀態：根據 is_service_package 的值顯示/隱藏對應服務
        this.toggleServicePackageService(servicePackageSelect);
        
        // 綁定監聽器（使用 once: false 允許重複綁定，但實際只會觸發一次）
        servicePackageSelect.addEventListener('change', () => {
            this.toggleServicePackageService(servicePackageSelect);
        });
    }
    
    // 切換服務套票對應服務顯示
    async toggleServicePackageService(selectElement) {
        const serviceSelectContainer = document.getElementById('field_container_service_package_service_id');
        const serviceSelect = document.getElementById('field_service_package_service_id');
        
        if (!serviceSelectContainer || !selectElement) {
            console.warn('服務套票相關字段未找到');
            return;
        }
        
        const isServicePackage = selectElement.value === 'true' || selectElement.value === true;
        
        if (isServicePackage) {
            // 顯示字段
            serviceSelectContainer.style.display = 'block';
            // 確保字段可見（移除任何隱藏樣式）
            serviceSelectContainer.style.visibility = 'visible';
            serviceSelectContainer.style.opacity = '1';
            
            // 如果字段顯示後，確保 Select2 已初始化
            if (serviceSelect) {
                // 查找字段配置
                const fieldConfig = this.config.formFields.find(f => f.key === 'service_package_service_id');
                if (fieldConfig) {
                    console.log('服務套票字段顯示，開始初始化 Select2', fieldConfig);
                    // 等待一下確保 DOM 已更新，並且容器已顯示
                    setTimeout(async () => {
                        try {
                            // 確保容器已顯示
                            if (serviceSelectContainer.style.display === 'none') {
                                serviceSelectContainer.style.display = 'block';
                            }
                            
                            // 先載入選項（如果還沒有載入）
                            if (serviceSelect.options.length <= 1) {
                                await this.loadRelationFieldOptions(fieldConfig);
                                console.log('選項載入完成，選項數量:', serviceSelect.options.length);
                            }
                            
                            // 檢查是否已初始化 Select2
                            if (typeof $ !== 'undefined' && $(serviceSelect).hasClass('select2-hidden-accessible')) {
                                console.log('Select2 已初始化，重新初始化');
                                // 如果已經初始化，先銷毀再重新初始化
                                try {
                                    $(serviceSelect).select2('destroy');
                                } catch (e) {
                                    console.warn('銷毀 Select2 失敗:', e);
                                }
                            }
                            
                            // 初始化 Select2
                            await this.initSelect2(fieldConfig);
                            console.log('Select2 初始化完成');
                            
                            // 再次確保 Select2 容器可見
                            setTimeout(() => {
                                const select2Container = $(serviceSelect).next('.select2-container');
                                if (select2Container.length > 0) {
                                    select2Container.css({
                                        'display': 'block',
                                        'visibility': 'visible',
                                        'opacity': '1',
                                        'width': '100%'
                                    });
                                }
                                // 確保 select 元素本身也可見
                                $(serviceSelect).css({
                                    'display': 'block',
                                    'visibility': 'visible',
                                    'opacity': '1'
                                });
                            }, 100);
                        } catch (error) {
                            console.error('初始化 Select2 失敗:', error);
                        }
                    }, 300);
                } else {
                    console.warn('找不到 service_package_service_id 字段配置');
                }
            } else {
                console.warn('找不到 service_package_service_id 字段元素');
            }
        } else {
            // 隱藏字段
            serviceSelectContainer.style.display = 'none';
            if (serviceSelect) {
                // 如果是 Select2，需要清除
                if (typeof $ !== 'undefined' && $(serviceSelect).hasClass('select2-hidden-accessible')) {
                    $(serviceSelect).val(null).trigger('change');
                } else {
                    serviceSelect.value = '';
                }
            }
        }
    }
    
    // 設置推廣排程切換邏輯
    setupPromotionScheduleToggle() {
        const sendTypeSelect = document.getElementById('field_send_type');
        const scheduledAtContainer = document.getElementById('field_container_scheduled_at');
        
        if (!sendTypeSelect || !scheduledAtContainer) return;
        
        // 初始狀態：根據 send_type 的值顯示/隱藏排程時間
        this.togglePromotionSchedule(sendTypeSelect);
        
        // 監聽變化
        sendTypeSelect.addEventListener('change', () => {
            this.togglePromotionSchedule(sendTypeSelect);
        });
    }
    
    // 切換推廣排程顯示
    togglePromotionSchedule(selectElement) {
        const scheduledAtContainer = document.getElementById('field_container_scheduled_at');
        const scheduledAtInput = document.getElementById('field_scheduled_at');
        
        if (!scheduledAtContainer || !selectElement) return;
        
        const sendType = selectElement.value;
        
        if (sendType === 'scheduled') {
            scheduledAtContainer.style.display = 'block';
            if (scheduledAtInput) {
                scheduledAtInput.setAttribute('required', 'required');
            }
        } else {
            scheduledAtContainer.style.display = 'none';
            if (scheduledAtInput) {
                scheduledAtInput.removeAttribute('required');
                scheduledAtInput.value = '';
            }
        }
    }
    
    // 初始化 HTML 編輯器（Quill.js）
    initHtmlEditors() {
        if (typeof Quill === 'undefined') {
            console.warn('Quill.js 未加載，無法初始化 HTML 編輯器');
            return;
        }
        
        this.config.formFields.forEach(field => {
            if (field.type === 'html-editor') {
                const fieldId = `field_${field.key}`;
                const editorId = `${fieldId}_editor`;
                const editorElement = document.getElementById(editorId);
                const textarea = document.getElementById(fieldId);
                
                if (editorElement && !editorElement.quill) {
                    // 初始化 Quill 編輯器
                    const rawPlaceholder = field.placeholder || '輸入內容...';
                    const placeholder = this.getPlaceholder(field.key, rawPlaceholder) || rawPlaceholder;
                    const quill = new Quill(editorElement, {
                        theme: 'snow',
                        modules: {
                            toolbar: [
                                [{ 'header': [1, 2, 3, false] }],
                                ['bold', 'italic', 'underline', 'strike'],
                                [{ 'list': 'ordered'}, { 'list': 'bullet' }],
                                [{ 'color': [] }, { 'background': [] }],
                                ['link', 'image'],
                                ['clean']
                            ]
                        },
                        placeholder
                    });
                    
                    // 將 Quill 實例存儲到元素上，方便後續訪問
                    editorElement.quill = quill;
                    
                    // 監聽內容變化，同步到隱藏的 textarea
                    quill.on('text-change', () => {
                        if (textarea) {
                            const html = quill.root.innerHTML;
                            textarea.value = html === '<p><br></p>' || html === '<p></p>' ? '' : html;
                        }
                    });
                    
                    // 如果有初始值，設置到編輯器
                    if (textarea && textarea.value) {
                        quill.root.innerHTML = textarea.value;
                    }
                }
            }
        });
    }
    
    // 渲染優惠券條件管理區域
    renderCouponConditions() {
        const form = document.getElementById('dynamicForm');
        if (!form) return;
        
        const conditionsSection = `
            <div class="card mt-4">
                <div class="card-header d-flex justify-content-between align-items-center">
                    <h5 class="mb-0" data-i18n="coupons.conditions.title">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.title') : '多條件匹配'}</h5>
                    <button type="button" class="btn btn-sm btn-primary" onclick="window.dynamicForm.addCondition()">
                        <i class="bi bi-plus-circle"></i> <span data-i18n="coupons.conditions.addCondition">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.addCondition') : '添加條件'}</span>
                    </button>
                </div>
                <div class="card-body">
                    <div id="conditionsList">
                        <p class="text-muted" data-i18n="common.noConditions">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.noConditions') : '暫無條件'}</p>
                    </div>
                </div>
            </div>
        `;
        
        form.insertAdjacentHTML('beforeend', conditionsSection);
        
        // 如果是編輯模式，載入現有條件
        if (this.isEdit && this.itemId) {
            this.loadConditions();
        }
    }
    
    // 添加條件
    addCondition() {
        const conditionsList = document.getElementById('conditionsList');
        if (!conditionsList) return;
        
        // 清除"暫無條件"提示
        if (conditionsList.querySelector('p.text-muted')) {
            conditionsList.innerHTML = '';
        }
        
        const conditionId = 'condition_' + Date.now();
        const conditionHtml = `
            <div class="card mb-3 condition-item" data-condition-id="${conditionId}">
                <div class="card-body">
                    <div class="row">
                        <div class="col-md-3 mb-3">
                            <label class="form-label" data-i18n="coupons.conditions.conditionType">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.conditionType') : '條件類型'}</label>
                            <select class="form-select condition-type" onchange="window.dynamicForm.updateConditionFields('${conditionId}')">
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇'}</option>
                                <option value="product_quantity" data-i18n="coupons.conditions.type.product_quantity">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.type.product_quantity') : '特定產品數量'}</option>
                                <option value="product_amount" data-i18n="coupons.conditions.type.product_amount">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.type.product_amount') : '特定產品金額'}</option>
                                <option value="customer" data-i18n="coupons.conditions.type.customer">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.type.customer') : '特定客戶'}</option>
                            </select>
                        </div>
                        <div class="col-md-3 mb-3 condition-field-product" style="display: none;">
                            <label class="form-label" data-i18n="common.product">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.product') : '產品'}</label>
                            <select class="form-select condition-product" id="condition-product-${conditionId}">
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                        </div>
                        <div class="col-md-2 mb-3 condition-field-quantity" style="display: none;">
                            <label class="form-label" data-i18n="common.quantity">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.quantity') : '數量'}</label>
                            <input type="number" class="form-control condition-quantity" min="1">
                        </div>
                        <div class="col-md-2 mb-3 condition-field-amount" style="display: none;">
                            <label class="form-label" data-i18n="common.amount">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.amount') : '金額'}</label>
                            <input type="number" class="form-control condition-amount" step="0.01" min="0">
                        </div>
                        <div class="col-md-3 mb-3 condition-field-customer" style="display: none;">
                            <label class="form-label" data-i18n="common.customer">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.customer') : '客戶'}</label>
                            <select class="form-select condition-customer" id="condition-customer-${conditionId}">
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                        </div>
                        <div class="col-md-2 mb-3">
                            <label class="form-label" data-i18n="common.matchType">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.matchType') : '匹配方式'}</label>
                            <select class="form-select condition-match-type">
                                <option value="and" data-i18n="common.andMatch">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.andMatch') : 'AND (全部滿足)'}</option>
                                <option value="or" data-i18n="common.orMatch">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.orMatch') : 'OR (任一滿足)'}</option>
                            </select>
                        </div>
                        <div class="col-md-1 mb-3 d-flex align-items-end">
                            <button type="button" class="btn btn-danger btn-sm" onclick="window.dynamicForm.removeCondition('${conditionId}')">
                                <i class="bi bi-trash"></i>
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        conditionsList.insertAdjacentHTML('beforeend', conditionHtml);
        
        // 載入產品、會員等級、客戶選項
        this.loadConditionOptions(conditionId);
    }
    
    // 更新條件字段顯示
    async updateConditionFields(conditionId) {
        const item = document.querySelector(`[data-condition-id="${conditionId}"]`);
        if (!item) return;
        
        const type = item.querySelector('.condition-type').value;
        
        // 隱藏所有字段
        item.querySelectorAll('.condition-field-product, .condition-field-quantity, .condition-field-amount, .condition-field-member-level, .condition-field-customer').forEach(el => {
            el.style.display = 'none';
        });
        
        // 根據類型顯示相應字段
        if (type === 'product_quantity') {
            const productField = item.querySelector('.condition-field-product');
            if (productField) {
                productField.style.display = 'block';
                // 初始化產品 Select2
                const productSelect = item.querySelector('.condition-product');
                if (productSelect && !$(productSelect).hasClass('select2-hidden-accessible')) {
                    const k = 'common.product';
                    const label = (typeof I18n !== 'undefined' && I18n.t && I18n.t(k) !== k)
                        ? I18n.t(k)
                        : (((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Product' : '產品'));
                    await this.initConditionSelect2(productSelect, '/products', label);
                }
            }
            item.querySelector('.condition-field-quantity').style.display = 'block';
        } else if (type === 'product_amount') {
            const productField = item.querySelector('.condition-field-product');
            if (productField) {
                productField.style.display = 'block';
                // 初始化產品 Select2
                const productSelect = item.querySelector('.condition-product');
                if (productSelect && !$(productSelect).hasClass('select2-hidden-accessible')) {
                    const k = 'common.product';
                    const label = (typeof I18n !== 'undefined' && I18n.t && I18n.t(k) !== k)
                        ? I18n.t(k)
                        : (((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Product' : '產品'));
                    await this.initConditionSelect2(productSelect, '/products', label);
                }
            }
            item.querySelector('.condition-field-amount').style.display = 'block';
        } else if (type === 'customer') {
            const customerField = item.querySelector('.condition-field-customer');
            if (customerField) {
                customerField.style.display = 'block';
                // 初始化客戶 Select2
                const customerSelect = item.querySelector('.condition-customer');
                if (customerSelect && !$(customerSelect).hasClass('select2-hidden-accessible')) {
                    const k = 'common.customer';
                    const label = (typeof I18n !== 'undefined' && I18n.t && I18n.t(k) !== k)
                        ? I18n.t(k)
                        : (((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Customer' : '客戶'));
                    await this.initConditionSelect2(customerSelect, '/customers', label);
                }
            }
        }
    }
    
    // 載入條件選項
    async loadConditionOptions(conditionId) {
        const item = document.querySelector(`[data-condition-id="${conditionId}"]`);
        if (!item) return;
        
        // 載入產品選項（使用 Select2）
        const productSelect = item.querySelector('.condition-product');
        if (productSelect) {
            const k = 'common.product';
            const label = (typeof I18n !== 'undefined' && I18n.t && I18n.t(k) !== k)
                ? I18n.t(k)
                : (((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Product' : '產品'));
            await this.initConditionSelect2(productSelect, '/products', label);
        }
        
        // 載入客戶選項（使用 Select2）
        const customerSelect = item.querySelector('.condition-customer');
        if (customerSelect) {
            const k = 'common.customer';
            const label = (typeof I18n !== 'undefined' && I18n.t && I18n.t(k) !== k)
                ? I18n.t(k)
                : (((typeof I18n !== 'undefined' && I18n.currentLang === 'en') ? 'Customer' : '客戶'));
            await this.initConditionSelect2(customerSelect, '/customers', label);
        }
    }
    
    // 初始化條件中的 Select2 下拉框
    async initConditionSelect2(select, apiPath, label) {
        if (!select) return;
        
        // 如果已經初始化過 Select2，先銷毀
        if (typeof $ !== 'undefined' && $(select).hasClass('select2-hidden-accessible')) {
            $(select).select2('destroy');
        }
        
        // 等待 jQuery 和 Select2 加載
        if (typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') {
            console.warn('jQuery 或 Select2 未加載，無法初始化 Select2');
            return;
        }
        
        // 檢查是否為多選（通過 multiple 屬性）
        const isMulti = select.hasAttribute('multiple') || select.multiple;
        
        // 初始化 Select2
        const getSearchOrSelectText = () => {
            const lang = (typeof I18n !== 'undefined' && I18n.currentLang) ? I18n.currentLang : 'zh';
            const fallbackTpl = (lang === 'en') ? 'Search or select {label}...' : '搜索或選擇{label}...';
            try {
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const k = 'common.searchOrSelect';
                    const t = I18n.t(k);
                    const tpl = (t && t !== k) ? t : fallbackTpl;
                    return tpl.replace('{label}', label || '');
                }
            } catch {}
            return fallbackTpl.replace('{label}', label || '');
        };

        $(select).select2({
            theme: 'bootstrap-5',
            placeholder: getSearchOrSelectText(),
            allowClear: true,
            multiple: isMulti,
            ajax: {
                url: '/api/v1' + apiPath,
                dataType: 'json',
                delay: 250,
                headers: {
                    'Authorization': 'Bearer ' + (localStorage.getItem('auth_token') || ''),
                    'X-Tenant-Subdomain': localStorage.getItem('tenant_subdomain') || ''
                },
                data: function (params) {
                    return {
                        search: params.term || '',
                        limit: 50,
                        page: params.page || 1
                    };
                },
                processResults: function (data, params) {
                    params.page = params.page || 1;
                    return {
                        results: (data.data || []).map(function(item) {
                            return {
                                id: item.id,
                                text: item.name || item.code || item.employee_number || item.id
                            };
                        }),
                        pagination: {
                            // 如果返回了 50 條記錄，且總數大於當前頁 * 50，則還有更多數據
                            more: (data.data || []).length === 50 && (data.total || 0) > params.page * 50
                        }
                    };
                },
                cache: true
            },
            minimumInputLength: 0,
            closeOnSelect: !isMulti, // 多選模式下選擇後不關閉下拉框，方便連續選擇
            language: {
                noResults: function() {
                    return (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.noResults') : '未找到結果';
                },
                searching: function() {
                    return (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.searching') : '搜索中...';
                }
            }
        });
    }
    
    // 移除條件
    removeCondition(conditionId) {
        const item = document.querySelector(`[data-condition-id="${conditionId}"]`);
        if (item) {
            item.remove();
        }
        
        // 如果沒有條件了，顯示提示
        const conditionsList = document.getElementById('conditionsList');
        if (conditionsList && conditionsList.children.length === 0) {
            conditionsList.innerHTML = `<p class="text-muted" data-i18n="common.noConditions">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.noConditions') : '暫無條件'}</p>`;
        }
    }
    
    // 載入現有條件
    async loadConditions() {
        if (!this.itemId) return;
        
        try {
            const coupon = await App.apiRequest(`${this.config.apiPath}/${this.itemId}`);
            if (coupon.conditions && coupon.conditions.length > 0) {
                const conditionsList = document.getElementById('conditionsList');
                if (conditionsList) {
                    conditionsList.innerHTML = '';
                    for (const condition of coupon.conditions) {
                        await this.addConditionFromData(condition);
                    }
                }
            }
        } catch (error) {
            console.error('Failed to load conditions:', error);
        }
    }
    
    // 從數據添加條件
    async addConditionFromData(condition) {
        const conditionsList = document.getElementById('conditionsList');
        if (!conditionsList) return;
        
        if (conditionsList.querySelector('p.text-muted')) {
            conditionsList.innerHTML = '';
        }
        
        const conditionId = condition.id || 'condition_' + Date.now();
        const conditionHtml = `
            <div class="card mb-3 condition-item" data-condition-id="${conditionId}" data-original-id="${condition.id || ''}">
                <div class="card-body">
                    <div class="row">
                        <div class="col-md-3 mb-3">
                            <label class="form-label" data-i18n="coupons.conditions.conditionType">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.conditionType') : '條件類型'}</label>
                            <select class="form-select condition-type" onchange="window.dynamicForm.updateConditionFields('${conditionId}')">
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇'}</option>
                                <option value="product_quantity" ${condition.condition_type === 'product_quantity' ? 'selected' : ''} data-i18n="coupons.conditions.type.product_quantity">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.type.product_quantity') : '特定產品數量'}</option>
                                <option value="product_amount" ${condition.condition_type === 'product_amount' ? 'selected' : ''} data-i18n="coupons.conditions.type.product_amount">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.type.product_amount') : '特定產品金額'}</option>
                                <option value="customer" ${condition.condition_type === 'customer' ? 'selected' : ''} data-i18n="coupons.conditions.type.customer">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('coupons.conditions.type.customer') : '特定客戶'}</option>
                            </select>
                        </div>
                        <div class="col-md-3 mb-3 condition-field-product" style="display: ${condition.condition_type === 'product_quantity' || condition.condition_type === 'product_amount' ? 'block' : 'none'};">
                            <label class="form-label" data-i18n="common.product">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.product') : '產品'}</label>
                            <select class="form-select condition-product" id="condition-product-${conditionId}">
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                        </div>
                        <div class="col-md-2 mb-3 condition-field-quantity" style="display: ${condition.condition_type === 'product_quantity' ? 'block' : 'none'};">
                            <label class="form-label" data-i18n="common.quantity">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.quantity') : '數量'}</label>
                            <input type="number" class="form-control condition-quantity" min="1" value="${condition.quantity || ''}">
                        </div>
                        <div class="col-md-2 mb-3 condition-field-amount" style="display: ${condition.condition_type === 'product_amount' ? 'block' : 'none'};">
                            <label class="form-label" data-i18n="common.amount">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.amount') : '金額'}</label>
                            <input type="number" class="form-control condition-amount" step="0.01" min="0" value="${condition.amount || ''}">
                        </div>
                        <div class="col-md-3 mb-3 condition-field-customer" style="display: ${condition.condition_type === 'customer' ? 'block' : 'none'};">
                            <label class="form-label" data-i18n="common.customer">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.customer') : '客戶'}</label>
                            <select class="form-select condition-customer" id="condition-customer-${conditionId}">
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                        </div>
                        <div class="col-md-2 mb-3">
                            <label class="form-label" data-i18n="common.matchType">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.matchType') : '匹配方式'}</label>
                            <select class="form-select condition-match-type">
                                <option value="and" ${condition.match_type === 'and' ? 'selected' : ''} data-i18n="common.andMatch">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.andMatch') : 'AND (全部滿足)'}</option>
                                <option value="or" ${condition.match_type === 'or' ? 'selected' : ''} data-i18n="common.orMatch">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.orMatch') : 'OR (任一滿足)'}</option>
                            </select>
                        </div>
                        <div class="col-md-1 mb-3 d-flex align-items-end">
                            <button type="button" class="btn btn-danger btn-sm" onclick="window.dynamicForm.removeCondition('${conditionId}')">
                                <i class="bi bi-trash"></i>
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        conditionsList.insertAdjacentHTML('beforeend', conditionHtml);
        
        // 載入選項並設置值
        this.loadConditionOptions(conditionId).then(() => {
            const item = document.querySelector(`[data-condition-id="${conditionId}"]`);
            if (item) {
                if (condition.product_id) {
                    const productSelect = item.querySelector('.condition-product');
                    if (productSelect) productSelect.value = condition.product_id;
                }
                if (condition.customer_id) {
                    const customerSelect = item.querySelector('.condition-customer');
                    if (customerSelect) customerSelect.value = condition.customer_id;
                }
            }
        });
    }
    
    // 收集條件數據
    collectConditions() {
        const conditions = [];
        const items = document.querySelectorAll('.condition-item');
        
        items.forEach(item => {
            const type = item.querySelector('.condition-type').value;
            if (!type) return;
            
            const condition = {
                condition_type: type,
                match_type: item.querySelector('.condition-match-type').value || 'and'
            };
            
            if (type === 'product_quantity' || type === 'product_amount') {
                const productSelect = item.querySelector('.condition-product');
                let productId = '';
                if (productSelect) {
                    if (typeof $ !== 'undefined' && $(productSelect).hasClass('select2-hidden-accessible')) {
                        productId = $(productSelect).val() || '';
                    } else {
                        productId = productSelect.value || '';
                    }
                }
                if (productId) condition.product_id = productId;
                
                if (type === 'product_quantity') {
                    const quantity = item.querySelector('.condition-quantity').value;
                    if (quantity) condition.quantity = parseInt(quantity);
                } else {
                    const amount = item.querySelector('.condition-amount').value;
                    if (amount) condition.amount = parseFloat(amount);
                }
            } else if (type === 'customer') {
                const customerSelect = item.querySelector('.condition-customer');
                let customerId = '';
                if (customerSelect) {
                    if (typeof $ !== 'undefined' && $(customerSelect).hasClass('select2-hidden-accessible')) {
                        customerId = $(customerSelect).val() || '';
                    } else {
                        customerId = customerSelect.value || '';
                    }
                }
                if (customerId) condition.customer_id = customerId;
            }
            
            const originalId = item.getAttribute('data-original-id');
            if (originalId) condition.id = originalId;
            
            conditions.push(condition);
        });
        
        return conditions;
    }
    
    async loadRelationOptions() {
        // 确保 jQuery 和 Select2 已加载
        if (typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') {
            console.warn('等待 jQuery 和 Select2 加载...');
            let retries = 0;
            while ((typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') && retries < 30) {
                await new Promise(resolve => setTimeout(resolve, 100));
                retries++;
            }
            if (typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') {
                console.error('jQuery 或 Select2 加载超时，无法初始化 Select2');
                return;
            }
        }
        
        // 收集所有需要加载的字段，分为串行和并行两类
        const serialFields = []; // 需要串行处理的字段（如依赖字段、特殊字段）
        const parallelFields = []; // 可以并行处理的字段
        
        for (const field of this.config.formFields) {
            // 对于 warehouses, suppliers, stores 页面的 address_country_code 和 address_region_code，确保正确初始化
            if ((this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') && 
                (field.key === 'address_country_code' || field.key === 'address_region_code') &&
                field.type === 'select2') {
                serialFields.push({ field, type: 'select2-init' });
                continue;
            }
            
            // 如果是依赖字段，检查是否应该显示
            if (field.dependency) {
                const dependencyField = document.getElementById(`field_${field.dependency.field}`);
                if (dependencyField) {
                    const dependencyValue = dependencyField.type === 'checkbox' ? String(dependencyField.checked) : dependencyField.value;
                    const allowedValues = Array.isArray(field.dependency.values) ? field.dependency.values : [field.dependency.value];
                    const shouldShow = allowedValues.includes(dependencyValue);
                    
                    if (!shouldShow) {
                        // 依赖字段隐藏，只加载选项，不初始化 Select2
                        if (field.relationApi && (field.type === 'select2' || field.type === 'select2-multi')) {
                            serialFields.push({ field, type: 'load-options' });
                        }
                        continue; // 跳过初始化，等显示后再初始化
                    } else {
                        // 依赖字段显示，需要串行处理（先等依赖字段加载完成）
                        serialFields.push({ field, type: field.type === 'select2' || field.type === 'select2-multi' ? 'select2-init' : 'load-options' });
                        continue;
                    }
                }
            }
            
            // 正常字段可以并行处理
            if (field.type === 'select2' || field.type === 'select2-multi') {
                parallelFields.push({ field, type: 'select2-init' });
            } else if ((field.type === 'select' && field.relationApi) || field.type === 'relation-select' || field.type === 'multi-select') {
                parallelFields.push({ field, type: 'load-options' });
            }
        }
        
        // 并行加载所有可以并行的字段
        const parallelPromises = parallelFields.map(async ({ field, type }) => {
            try {
                if (type === 'select2-init') {
                    await this.initSelect2(field);
                } else {
                    await this.loadRelationFieldOptions(field);
                }
            } catch (error) {
                console.error(`加载字段失败 (${field.key}):`, error);
            }
        });
        
        // 串行处理需要串行的字段
        for (const { field, type } of serialFields) {
            try {
                if (type === 'select2-init') {
                    await this.initSelect2(field);
                    await new Promise(resolve => setTimeout(resolve, 50));
                } else {
                    await this.loadRelationFieldOptions(field);
                    await new Promise(resolve => setTimeout(resolve, 50));
                }
            } catch (error) {
                console.error(`加载字段失败 (${field.key}):`, error);
            }
        }
        
        // 等待所有并行操作完成
        await Promise.all(parallelPromises);
        
        // 额外等待，确保所有 Select2 和选项都完全加载完成
        // 对于产品表单，可能需要更长时间（因为有很多关联字段：product-types, brands, services 等）
        const extraWaitTime = this.pageName === 'products' ? 200 : 100;
        await new Promise(resolve => setTimeout(resolve, extraWaitTime));
    }
    
    async loadRelationFieldOptions(field) {
        try {
            // 為多個相同 key 的字段生成唯一 ID
            let fieldId = field._uniqueId || `field_${field.key}`;
            if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0 && !field._uniqueId) {
                const depValue = field.dependency.values[0];
                fieldId = `field_${field.key}_${depValue}`;
            }
            const select = document.getElementById(fieldId);
            if (!select) return;
            
            const apiPath = field.api || field.relationApi;
            if (!apiPath) return;
            
            // 如果是地區字段且需要 country_code，檢查是否有國家值
            if (field.key === 'address_region_code' && field.dependency && field.dependency.field === 'address_country_code') {
                const countryFieldId = `field_${field.dependency.field}`;
                const countryField = document.getElementById(countryFieldId);
                if (countryField) {
                    let countryCode = '';
                    if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                        countryCode = $(countryField).val() || '';
                    } else {
                        countryCode = countryField.value || '';
                    }
                    if (!countryCode) {
                        // 沒有國家值，只顯示提示，不載入地區
                        if (field.type === 'multi-select') {
                            select.innerHTML = '';
                        } else {
                            const txt = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelectCountryFirst') : '請先選擇國家';
                            select.innerHTML = `<option value="">${txt}</option>`;
                        }
                        select.disabled = true;
                        return;
                    }
                    // 有國家值，在 API 請求中添加 country_code 參數
                    const data = await App.apiRequest(`${apiPath}?country_code=${countryCode}&limit=1000`);
                    const items = data.data || [];
                    // 多選不需要空選項
                    if (field.type === 'multi-select') {
                        select.innerHTML = '';
                    } else {
                        const txt = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelectRegion') : '請選擇地區';
                        select.innerHTML = `<option value="">${txt}</option>`;
                    }
                    // 存儲所有選項數據以便搜索
                    select._allOptions = items;
                    items.forEach(item => {
                        const valueKey = field.relationValueKey || field.relationKey || 'id';
                        const labelKey = field.relationLabelKey || field.relationLabel || 'name';
                        const value = item[valueKey];
                        const label = item[labelKey] || item.name || item.code || value;
                        select.innerHTML += `<option value="${value}" data-label="${label}">${label}</option>`;
                    });
                    select.disabled = false;
                    return;
                }
            }
            
            const data = await App.apiRequest(apiPath + '?limit=1000');
            const items = data.data || [];
            
            // 多選不需要空選項
            if (field.type === 'multi-select') {
                select.innerHTML = '';
            } else {
                const txt = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇';
                select.innerHTML = `<option value="">${txt}</option>`;
            }
            
            // 存儲所有選項數據以便搜索
            select._allOptions = items;
            
            items.forEach(item => {
                const valueKey = field.relationValueKey || field.relationKey || 'id';
                const labelKey = field.relationLabelKey || field.relationLabel || 'name';
                const value = item[valueKey];
                const label = item[labelKey] || item.name || item.code || value;
                select.innerHTML += `<option value="${value}" data-label="${label}">${label}</option>`;
            });
            
            // 為 relation-select 添加搜索功能
            if ((field.type === 'relation-select' || field.relationApi) && field.type !== 'multi-select') {
                this.setupSelectSearch(fieldId, field);
            }
        } catch (error) {
            console.error(`載入 ${field.label} 選項失敗:`, error);
            const fieldId = `field_${field.key}`;
            const select = document.getElementById(fieldId);
            if (select) {
                if (field.type === 'multi-select') {
                    select.innerHTML = `<option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loadError') : '載入失敗'}</option>`;
                } else {
                    select.innerHTML = `<option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loadError') : '載入失敗'}</option>`;
                }
            }
        }
    }
    
    setupSelectSearch(fieldId, field) {
        const select = document.getElementById(fieldId);
        const searchInput = document.getElementById(fieldId + '_search');
        if (!select || !searchInput) return;
        
        // 顯示搜索框
        searchInput.style.display = 'block';
        searchInput.style.marginBottom = '5px';
        try {
            const labelKey = field && field.key ? `fields.${field.key}` : '';
            const labelText = (typeof I18n !== 'undefined' && I18n.t && labelKey) ? (I18n.t(labelKey) !== labelKey ? I18n.t(labelKey) : (field.label || '')) : (field.label || '');
            const searchKey = 'common.search';
            const searchText = (typeof I18n !== 'undefined' && I18n.t) ? (I18n.t(searchKey) !== searchKey ? I18n.t(searchKey) : '搜索') : '搜索';
            searchInput.placeholder = `${searchText}${labelText ? ` ${labelText}` : ''}...`;
        } catch (e) {
            searchInput.placeholder = `搜索${field.label || ''}...`;
        }
        
        // 移除舊的事件監聽器（如果有的話）
        const newSearchInput = searchInput.cloneNode(true);
        searchInput.parentNode.replaceChild(newSearchInput, searchInput);
        
        // 搜索功能
        newSearchInput.addEventListener('input', (e) => {
            const searchTerm = e.target.value.toLowerCase();
            const options = select.querySelectorAll('option');
            
            options.forEach(option => {
                const label = option.textContent.toLowerCase();
                if (label.includes(searchTerm) || option.value === '') {
                    option.style.display = '';
                } else {
                    option.style.display = 'none';
                }
            });
        });
        
        // 當選擇改變時，清空搜索
        select.addEventListener('change', () => {
            newSearchInput.value = '';
            const options = select.querySelectorAll('option');
            options.forEach(option => {
                option.style.display = '';
            });
        });
    }
    
    // 初始化 Select2 下拉框
    async initSelect2(field) {
        // 為多個相同 key 的字段生成唯一 ID
        let fieldId = field._uniqueId || `field_${field.key}`;
        if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0 && !field._uniqueId) {
            const depValue = field.dependency.values[0];
            fieldId = `field_${field.key}_${depValue}`;
        }
        
        // 等待 DOM 元素存在
        let select = document.getElementById(fieldId);
        let retries = 0;
        while (!select && retries < 10) {
            await new Promise(resolve => setTimeout(resolve, 100));
            select = document.getElementById(fieldId);
            retries++;
        }
        
        if (!select) {
            console.warn(`Select2 初始化失敗：找不到元素 ${fieldId}`);
            return;
        }
        
        // 等待 jQuery 和 Select2 加載
        let retries2 = 0;
        while ((typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') && retries2 < 20) {
            await new Promise(resolve => setTimeout(resolve, 100));
            retries2++;
        }
        
        if (typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') {
            console.error('jQuery 或 Select2 未加載，無法初始化 Select2');
            return;
        }
        
        // 如果已經初始化過 Select2，先銷毀
        if ($(select).hasClass('select2-hidden-accessible')) {
            try {
                $(select).select2('destroy');
            } catch (e) {
                console.warn('銷毀 Select2 失敗:', e);
            }
        }
        
        const apiPath = field.api || field.relationApi;
        const isMulti = field.type === 'select2-multi';

        // 統一 Select2 placeholder / language，避免在不同語言下出現英文預設文案
        const getSelect2Placeholder = () => {
            try {
                // 1) 若 field.placeholder 有設定，優先走 i18n placeholders
                if (field && field.placeholder) {
                    return this.getPlaceholder(field.key, field.placeholder) || String(field.placeholder);
                }

                // 2) 否則組合「Please select + Field label」
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const selectKey = 'common.pleaseSelect';
                    const selectText = (I18n.t(selectKey) !== selectKey) ? I18n.t(selectKey) : '請選擇';
                    const labelKey = field && field.key ? `fields.${field.key}` : '';
                    const labelText = labelKey && I18n.t(labelKey) !== labelKey ? I18n.t(labelKey) : (this.getFieldLabel(field) || field.label || '');
                    return `${selectText}${labelText ? ` ${labelText}` : ''}`.trim();
                }
            } catch (e) {
                // ignore
            }
            return field.label ? `請選擇 ${field.label}` : '請選擇';
        };

        const getSelect2Language = () => {
            const fallback = {
                noResults: () => '未找到結果',
                searching: () => '搜索中...'
            };
            if (typeof I18n === 'undefined' || !I18n.t) return fallback;
            return {
                noResults: () => {
                    const k = 'common.noResults';
                    const t = I18n.t(k);
                    return (t && t !== k) ? t : fallback.noResults();
                },
                searching: () => {
                    const k = 'common.searching';
                    const t = I18n.t(k);
                    return (t && t !== k) ? t : fallback.searching();
                }
            };
        };
        
        // 如果有固定選項，使用固定選項初始化 Select2
        if (field.options && !apiPath) {
            try {
            const isPhoneCountryCodePage = (this.pageName === 'phone-country-codes');
            const isPhoneCodeField = (field && field.key === 'code');
            const phoneCodeFormatter = (data) => {
                try {
                    // placeholder / empty
                    if (!data || data.loading) return data && data.text ? data.text : '';
                    const code = String(data.id || data.text || '').trim();
                    if (!code) return data && data.text ? data.text : '';

                    // Prefer i18n (Chinese) display name, fallback to COUNTRY_PHONE_CODES (English), then code
                    let name = '';
                    if (typeof I18n !== 'undefined' && I18n.t) {
                        const k = `phoneCountryCodes.names.${code}`;
                        const t = I18n.t(k);
                        if (t && t !== k) name = t;
                    }
                    if (!name && typeof COUNTRY_PHONE_CODES !== 'undefined' && COUNTRY_PHONE_CODES && COUNTRY_PHONE_CODES[code]) {
                        name = COUNTRY_PHONE_CODES[code];
                    }

                    // 中文在前：名稱 (區號)
                    return name ? `${name} (${code})` : code;
                } catch (e) {
                    return data && data.text ? data.text : '';
                }
            };

            const cfg = {
                theme: 'bootstrap-5',
                placeholder: getSelect2Placeholder(),
                allowClear: true,
                multiple: isMulti,
                closeOnSelect: !isMulti,
                language: getSelect2Language()
            };

            // phone-country-codes：區號下拉顯示「中文名稱 (區號)」，避免第一眼仍是英文或只有 code
            if (isPhoneCountryCodePage && isPhoneCodeField) {
                cfg.templateResult = phoneCodeFormatter;
                cfg.templateSelection = phoneCodeFormatter;
            }

            $(select).select2(cfg);
            if (this.pageName === 'dining-queues' && field.key === 'area_id') {
                this.attachDiningQueueAreaWatcher();
                this.refreshDiningQueueTicketNumber().catch((e) => console.warn('refresh dining ticket failed', e));
            }
            } catch (error) {
                console.error(`初始化 Select2 失败 (${field.key}, 固定选项):`, error);
            }
            return;
        }
        
        // 如果有 relationApi，使用 AJAX 加載
        if (!apiPath) return;
        
        // 如果是新建模式且是 member_level_id 字段，先獲取預設值
        let defaultMemberLevel = null;
        if (!this.isEdit && field.key === 'member_level_id' && apiPath === '/member-levels') {
            try {
                const memberLevels = await App.apiRequest('/member-levels?limit=1000');
                if (memberLevels && memberLevels.data && Array.isArray(memberLevels.data) && memberLevels.data.length > 0) {
                    // API 已按 is_default DESC 排序，第一個就是默認值，但還是要檢查 is_default 字段
                    defaultMemberLevel = memberLevels.data.find(level => level && (level.is_default === true || level.is_default === 'true' || level.is_default === 1));
                    // 如果沒找到，使用第一個（因為 API 已排序，第一個應該就是默認值）
                    if (!defaultMemberLevel && memberLevels.data.length > 0) {
                        defaultMemberLevel = memberLevels.data[0];
                    }
                    if (!defaultMemberLevel) {
                        console.log('未找到系統預設的會員等級，將使用空值');
                    }
                } else {
                    console.log('會員等級列表為空，將使用空值');
                }
            } catch (error) {
                console.warn('獲取預設會員等級失敗，將繼續使用空值:', error);
                // 即使獲取失敗，也不影響 Select2 的正常初始化
                defaultMemberLevel = null;
            }
        }
        
        // 如果是新建模式且是 currency 字段（bank-accounts），先獲取預設值
        let defaultCurrency = null;
        if (!this.isEdit && field.key === 'currency' && apiPath === '/currencies') {
            try {
                const currencies = await App.apiRequest('/currencies?limit=1000');
                if (currencies && currencies.data && Array.isArray(currencies.data) && currencies.data.length > 0) {
                    // API 已按 is_default DESC 排序，第一個就是默認值，但還是要檢查 is_default 字段
                    defaultCurrency = currencies.data.find(curr => curr && (curr.is_default === true || curr.is_default === 'true' || curr.is_default === 1));
                    // 如果沒找到，使用第一個（因為 API 已排序，第一個應該就是默認值）
                    if (!defaultCurrency && currencies.data.length > 0) {
                        defaultCurrency = currencies.data[0];
                    }
                    if (!defaultCurrency) {
                        console.log('未找到系統預設的貨幣，將使用空值');
                    }
                } else {
                    console.log('貨幣列表為空，將使用空值');
                }
            } catch (error) {
                console.warn('獲取預設貨幣失敗，將繼續使用空值:', error);
                // 即使獲取失敗，也不影響 Select2 的正常初始化
                defaultCurrency = null;
            }
        }

        // 如果是電話區號字段，獲取系統預設（適用於 customers、suppliers、warehouses 頁面）
        // 新建模式：使用系統預設；編輯模式：如果沒有現有值，使用系統預設
        let defaultPhoneCode = null;
        if (field.key === 'phone_country_code' && (this.pageName === 'customers' || this.pageName === 'suppliers' || this.pageName === 'warehouses')) {
            try {
                const resp = await App.apiRequest('/api/v1/phone-country-codes');
                const codes = (resp && resp.data) ? resp.data : [];
                // API 已按 is_default DESC 排序，第一個就是默認值，但還是要檢查 is_default 字段
                defaultPhoneCode = codes.find(c => c && (c.is_default === true || c.is_default === 'true' || c.is_default === 1));
                // 如果沒找到，使用第一個（因為 API 已排序，第一個應該就是默認值）
                if (!defaultPhoneCode && codes.length > 0) {
                    defaultPhoneCode = codes[0];
                }
            } catch (error) {
                console.warn('獲取預設電話區號失敗，將繼續使用空值:', error);
                defaultPhoneCode = null;
            }
        }
        
        // 初始化 Select2（支持多選模式）
        try {
        const select2Config = {
            theme: 'bootstrap-5',
            placeholder: getSelect2Placeholder(),
            allowClear: true,
            multiple: isMulti,
            closeOnSelect: !isMulti, // 多選模式下選擇後不關閉下拉框
            minimumInputLength: 0, // 允许在打开下拉框时就加载选项（必须在 ajax 之前设置）
            language: getSelect2Language(),
            ajax: {
                url: apiPath.startsWith('/api/v1') ? apiPath : '/api/v1' + apiPath,
                dataType: 'json',
                delay: 250,
                headers: {
                    'Authorization': 'Bearer ' + (localStorage.getItem('auth_token') || ''),
                    'X-Tenant-Subdomain': localStorage.getItem('tenant_subdomain') || ''
                },
                data: function (params) {
                    const requestData = {
                        search: params.term || '',
                        limit: 50,
                        page: params.page || 1
                    };
                    
                    // 如果是地區字段，需要從國家字段獲取 country_code
                    if (field.key === 'address_region_code') {
                        const countryFieldId = `field_address_country_code`;
                        const countryField = document.getElementById(countryFieldId);
                        if (countryField) {
                            let countryCode = '';
                            if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                                countryCode = $(countryField).val() || '';
                            } else {
                                countryCode = countryField.value || '';
                            }
                            if (countryCode) {
                                requestData.country_code = countryCode;
                            } else {
                                // 沒有國家代碼，返回空結果
                                return {
                                    results: [],
                                    pagination: { more: false }
                                };
                            }
                        } else {
                            // 找不到國家字段，返回空結果
                            return {
                                results: [],
                                pagination: { more: false }
                            };
                        }
                    }

                    // Logistics companies：地區限制（多選國家）
                    if (field.key === 'allowed_region_keys') {
                        const countryFieldId = `field_allowed_country_codes`;
                        const countryField = document.getElementById(countryFieldId);
                        let selected = [];
                        if (countryField) {
                            if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                                const v = $(countryField).val();
                                if (Array.isArray(v)) selected = v;
                                else if (v) selected = [v];
                            } else if (countryField.multiple) {
                                selected = Array.from(countryField.selectedOptions).map(o => o.value).filter(Boolean);
                            } else if (countryField.value) {
                                selected = [countryField.value];
                            }
                        }
                        selected = (selected || []).map(s => String(s).trim()).filter(Boolean);
                        if (selected.length > 0) {
                            requestData.country_codes = selected.join(',');
                        }
                    }
                    
                    return requestData;
                },
                processResults: function (data, params) {
                    params.page = params.page || 1;
                    const labelKey = field.relationLabelKey || field.relationLabel || 'name';
                        // 如果定义了自定义显示格式，使用它
                        const displayFormat = field.relationDisplayFormat;
                        const labelFields = Array.isArray(field.relationLabelFields) ? field.relationLabelFields : null;
                    return {
                        results: (data.data || []).map(function(item) {
                            const valueKey = field.relationValueKey || field.relationKey || 'id';
                                let displayText;
                                if (labelFields && labelFields.length > 0) {
                                    const parts = labelFields.map(k => (item && item[k] != null ? String(item[k]).trim() : '')).filter(Boolean);
                                    if (parts.length >= 2 && String(labelFields[1]) === 'account_number') {
                                        displayText = `${parts[0]} (${parts.slice(1).join(' ')})`;
                                    } else {
                                        displayText = parts.join(' - ');
                                    }
                                } else if (displayFormat === 'code-name' && item.code && item.name) {
                                    // 货币格式：代码 - 名称
                                    displayText = `${item.code} - ${item.name}`;
                                } else if (displayFormat === 'employee_number-name' && item.employee_number && item.name) {
                                    // 員工編號格式：員工編號 - 名稱
                                    displayText = `${item.employee_number} - ${item.name}`;
                                } else if (displayFormat && typeof displayFormat === 'function') {
                                    displayText = displayFormat(item);
                                } else {
                                    displayText = item[labelKey] || item.name || item.code || item.employee_number || item.id;
                                }
                            return {
                                id: item[valueKey],
                                    text: displayText
                            };
                        }),
                        pagination: {
                            // 如果返回了 50 條記錄，且總數大於當前頁 * 50，則還有更多數據
                            more: (data.data || []).length === 50 && (data.total || 0) > (params.page || 1) * 50
                        }
                    };
                },
                cache: true
            },
            minimumInputLength: 0,
            language: {
                noResults: function() {
                    return (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.noResults') : '未找到結果';
                },
                searching: function() {
                    return (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.searching') : '搜索中...';
                }
            }
        };
        
        // 如果有預設的會員等級，添加到初始數據中
        if (defaultMemberLevel && defaultMemberLevel.id) {
            const labelKey = field.relationLabelKey || field.relationLabel || 'name';
            const valueKey = field.relationValueKey || field.relationKey || 'id';
            const defaultId = defaultMemberLevel[valueKey] || defaultMemberLevel.id;
            const defaultText = defaultMemberLevel[labelKey] || defaultMemberLevel.name || '預設會員等級';
            
            if (defaultId) {
                select2Config.data = [{
                    id: defaultId,
                    text: defaultText
                }];
            }
        }
        
        // 如果有預設的貨幣，添加到初始數據中
        if (defaultCurrency && (defaultCurrency.id || defaultCurrency.code)) {
            const labelKey = field.relationLabelKey || field.relationLabel || 'code';
            const valueKey = field.relationValueKey || field.relationKey || 'code';
            const defaultCode = defaultCurrency[valueKey] || defaultCurrency.code;
            let defaultText = '';
            
            // 根據顯示格式生成文本
            if (field.relationDisplayFormat === 'code-name' && defaultCurrency.code && defaultCurrency.name) {
                defaultText = `${defaultCurrency.code} - ${defaultCurrency.name}`;
            } else {
                defaultText = defaultCurrency[labelKey] || defaultCurrency.code || defaultCurrency.name || '預設貨幣';
            }
            
            if (defaultCode) {
                select2Config.data = [{
                    id: defaultCode,
                    text: defaultText
                }];
            }
        }
        
        // 如果有預設的電話區號，添加到初始數據中
        if (defaultPhoneCode && defaultPhoneCode.code && field.key === 'phone_country_code') {
            const valueKey = field.relationValueKey || field.relationKey || 'code';
            const defaultCode = defaultPhoneCode[valueKey] || defaultPhoneCode.code;
            // 電話區號只顯示 code（例如：+852），不顯示名稱
            const defaultText = defaultPhoneCode.code || '預設電話區號';
            
            if (defaultCode) {
                // 將默認電話區號添加到初始數據中，這樣 Select2 初始化時就會有這個選項
                select2Config.data = [{
                    id: defaultCode,
                    text: defaultText
                }];
            }
        }
        
        $(select).select2(select2Config);
        if (this.pageName === 'dining-queues' && field.key === 'area_id') {
            this.attachDiningQueueAreaWatcher();
            this.refreshDiningQueueTicketNumber().catch((e) => console.warn('refresh dining ticket failed', e));
        }
        
        // 对于 address_country_code 字段，确保在打开下拉框时能加载选项
        if ((this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') && 
            field.key === 'address_country_code' && field.type === 'select2') {
            // 确保 Select2 打开时能加载选项（minimumInputLength: 0 已设置，但需要触发查询）
            $(select).on('select2:open', function() {
                const $select = $(this);
                // 触发查询以加载选项（如果还没有加载）
                setTimeout(() => {
                    if ($select.data('select2')) {
                        $select.data('select2').trigger('query', { term: '' });
                    }
                }, 100);
            });
        }
        
        // 如果是地區字段，監聽國家字段的變化，重新加載地區（參考 customers/new 地址 popup 的實現）
        if (field.key === 'address_region_code') {
            const countryFieldId = `field_address_country_code`;
            const countryField = document.getElementById(countryFieldId);
            if (countryField) {
                // 初始化時檢查國家字段是否有值
                let initialCountryCode = '';
                if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                    initialCountryCode = $(countryField).val() || '';
                } else {
                    initialCountryCode = countryField.value || '';
                }
                
                // 如果有国家代码，启用地区字段
                if (initialCountryCode) {
                    $(select).prop('disabled', false);
                }
                
                // 初始化时加载地区选项（如果有国家代码）
                const loadRegions = () => {
                    let countryCode = '';
                    if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                        countryCode = $(countryField).val() || '';
                    } else {
                        countryCode = countryField.value || '';
                    }
                    
                    if (countryCode) {
                        // 啟用地區字段並清空當前選項
                        $(select).prop('disabled', false);
                        $(select).empty();
                        $(select).append('<option value="">載入中...</option>');
                        $(select).val('').trigger('change');
                        
                        // 使用 AJAX 加載地區
                        fetch(`/api/v1/country-regions?country_code=${countryCode}`)
                            .then(res => res.json())
                            .then(data => {
                                $(select).empty();
                                $(select).append('<option value="">請選擇地區</option>');
                                if (data.data && data.data.length > 0) {
                                    data.data.forEach(region => {
                                        const option = new Option(region.name, region.code, false, false);
                                        $(select).append(option);
                                    });
                                } else {
                                    $(select).append('<option value="">該國家無預定義地區</option>');
                                }
                                $(select).trigger('change');
                            })
                            .catch(error => {
                                console.error('載入地區失敗:', error);
                                $(select).empty();
                                $(select).append('<option value="">載入失敗</option>');
                            });
                    } else {
                        // 沒有選擇國家，清空地區選項並禁用
                        $(select).empty();
                        $(select).append('<option value="">請先選擇國家</option>');
                        $(select).prop('disabled', true);
                        $(select).trigger('change');
                    }
                };
                
                // 監聽國家字段變化（包括原生 change 和 Select2 事件）
                countryField.addEventListener('change', loadRegions);
                if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                    $(countryField).on('select2:select select2:clear', loadRegions);
                }
                
                // 初始化时立即加载一次（如果有国家代码则加载地区，否则显示"請先選擇國家"）
                setTimeout(() => {
                    loadRegions();
                }, 300);
            }
        }
        
        // 如果是 appointments 頁面的房間/設備/車輛字段，添加衝突檢查
        if (this.pageName === 'appointments' && (field.key === 'room_ids' || field.key === 'equipment_ids' || field.key === 'vehicle_ids')) {
            $(select).on('select2:select', async (e) => {
                await this.checkAppointmentConflict(field.key, e.params.data.id);
            });
        }
        
        // 如果有預設的電話區號，在 Select2 初始化後立即設置值
        if (defaultPhoneCode && defaultPhoneCode.code && field.key === 'phone_country_code') {
            const valueKey = field.relationValueKey || field.relationKey || 'code';
            const defaultCode = defaultPhoneCode[valueKey] || defaultPhoneCode.code;
            if (defaultCode) {
                // 立即設置值（因為已經添加到初始數據中）
                $(select).val(defaultCode).trigger('change');
                console.log('已設置預設電話區號（初始化時）:', defaultCode);
            }
        }
        
        // 設置預設值（僅當有有效的預設會員等級時）
        if (defaultMemberLevel && defaultMemberLevel.id) {
            try {
                const valueKey = field.relationValueKey || field.relationKey || 'id';
                const defaultId = defaultMemberLevel[valueKey] || defaultMemberLevel.id;
                
                if (defaultId) {
                    await new Promise(resolve => setTimeout(resolve, 200));
                    $(select).val(defaultId).trigger('change');
                    console.log('已設置預設會員等級:', defaultMemberLevel.name || '未知', defaultId);
                }
            } catch (error) {
                console.warn('設置預設會員等級值失敗，但不影響正常使用:', error);
            }
        }
        
        // 設置預設值（僅當有有效的預設貨幣時）
        if (defaultCurrency && (defaultCurrency.id || defaultCurrency.code)) {
            try {
                const valueKey = field.relationValueKey || field.relationKey || 'code';
                const defaultCode = defaultCurrency[valueKey] || defaultCurrency.code;
                
                if (defaultCode) {
                    await new Promise(resolve => setTimeout(resolve, 200));
                    $(select).val(defaultCode).trigger('change');
                    console.log('已設置預設貨幣:', defaultCurrency.name || defaultCurrency.code || '未知', defaultCode);
                }
            } catch (error) {
                console.warn('設置預設貨幣值失敗，但不影響正常使用:', error);
            }
        }

        // 設置預設值（電話區號）- 適用於 customers 和 suppliers 頁面
        if (defaultPhoneCode && defaultPhoneCode.code && field.key === 'phone_country_code') {
            try {
                await new Promise(resolve => setTimeout(resolve, 300));
                const valueKey = field.relationValueKey || field.relationKey || 'code';
                const defaultCode = defaultPhoneCode[valueKey] || defaultPhoneCode.code;
                
                if (defaultCode) {
                    // 確保 Select2 已初始化
                    if ($(select).hasClass('select2-hidden-accessible')) {
                        // 如果選項不存在，先添加（電話區號只顯示 code，例如：+852）
                        if ($(select).find(`option[value="${defaultCode}"]`).length === 0) {
                            const newOption = new Option(defaultPhoneCode.code, defaultCode, true, true);
                            $(select).append(newOption);
                        }
                        $(select).val(defaultCode).trigger('change');
                        console.log('已設置預設電話區號:', defaultCode);
                    } else {
                        // Select2 還未初始化，等待一下再試
                        setTimeout(async () => {
                            try {
                                if ($(select).find(`option[value="${defaultCode}"]`).length === 0) {
                                    const newOption = new Option(defaultPhoneCode.code, defaultCode, true, true);
                                    $(select).append(newOption);
                                }
                                $(select).val(defaultCode).trigger('change');
                                console.log('已設置預設電話區號（延遲）:', defaultCode);
                            } catch (err) {
                                console.warn('延遲設置預設電話區號失敗:', err);
                            }
                        }, 500);
                    }
                }
            } catch (error) {
                console.warn('設置預設電話區號失敗，但不影響正常使用:', error);
            }
        }
        } catch (error) {
            console.error(`初始化 Select2 失败 (${field.key}, AJAX):`, error);
        }
    }

    // 生成 tooltip 屬性（用於 readonly 或 disabled 字段）
    getTooltipAttr(field) {
        if (field.readonly || field.disabled) {
            const message = field.readonly ? '此欄位為只讀，無法編輯' : '此欄位已禁用，無法編輯';
            return `data-bs-toggle="tooltip" data-bs-placement="top" title="${message}"`;
        }
        return '';
    }

    renderField(field) {
        const t = (key, fallback) => {
            try {
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const v = I18n.t(key);
                    if (v && v !== key) return v;
                }
            } catch (e) {
                // ignore
            }
            return fallback;
        };

        const fieldLabel = this.getFieldLabel(field);
        const required = field.required ? '<span class="text-danger">*</span>' : '';
        // Resolve helpText: support helpTextKey for i18n, fall back to field.helpText
        const _resolvedHelpText = (() => {
            if (field.helpTextKey && typeof I18n !== 'undefined' && I18n.t) {
                const v = I18n.t(field.helpTextKey);
                if (v && v !== field.helpTextKey) return v;
            }
            return field.helpText || '';
        })();
        const helpTextHtml = _resolvedHelpText ? `<small class="form-text text-muted">${_resolvedHelpText.replace(/\n/g, '<br>')}</small>` : '';
        // 為多個相同 key 的字段生成唯一 ID（例如：多個 reference_id 字段）
        let fieldId = `field_${field.key}`;
        if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0) {
            // 使用依賴值來區分不同的 reference_id 字段
            const depValue = field.dependency.values[0]; // 使用第一個依賴值作為後綴
            fieldId = `field_${field.key}_${depValue}`;
        }
        const tooltipAttr = this.getTooltipAttr(field);
        // 統一：依賴顯示/隱藏需要 container id（很多舊欄位沒有，導致 dependency 沒反應）
        // 注意：reference_id 可能有多個實例，需要唯一容器 id
        let defaultContainerId = `field_container_${field.key}`;
        if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0) {
            const depValue = field.dependency.values[0];
            defaultContainerId = `field_container_${field.key}_${depValue}`;
        }

        switch (field.type) {
            case 'shipment-items': {
                const helpText = _resolvedHelpText ? `<small class="form-text text-muted">${_resolvedHelpText.replace(/\n/g, '<br>')}</small>` : '';
                const t = (key, fallback) => (typeof I18n !== 'undefined' && I18n.t) ? I18n.t(key) : fallback;
                return `
                    <div class="mb-3" id="${defaultContainerId}">
                        <label class="form-label">${fieldLabel} ${required}</label>
                        <div class="d-flex justify-content-between align-items-center mb-2">
                            <span class="text-muted small">${helpText ? '' : t('shipmentsPage.fields.itemsHelpText', '添加產品到此配送單')}</span>
                            <button type="button" class="btn btn-sm btn-outline-primary" id="${fieldId}_add_btn">
                                <i class="bi bi-plus-circle"></i> ${t('shipmentsPage.addProduct', '添加產品')}
                            </button>
                        </div>
                        <div class="table-responsive">
                            <table class="table table-sm table-bordered">
                                <thead>
                                    <tr>
                                        <th>${t('shipmentsPage.productColumn', '產品')}</th>
                                        <th class="text-end" style="width: 100px;">${t('shipmentsPage.quantityColumn', '數量')}</th>
                                        <th style="width: 60px;"></th>
                                    </tr>
                                </thead>
                                <tbody id="${fieldId}_table">
                                    <tr class="shipment-item-empty"><td colspan="3" class="text-center text-muted">${t('common.noData', '暫無資料')}</td></tr>
                                </tbody>
                            </table>
                        </div>
                        <input type="hidden" id="${fieldId}" name="${field.key}" value="[]">
                        ${helpText}
                    </div>
                `;
            }
            case 'default-include-single': {
                // 單一 yes/no：以 dropdown 呈現（符合 /product-taxes/new、/service-taxes/new 需求）
                // 實際送出時會轉為 default_include: ["..."] 或 []
                const helpText = field.helpText ? `<small class="form-text text-muted">${field.helpText.replace(/\n/g, '<br>')}</small>` : '';
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <select class="form-select" id="${fieldId}">
                            <option value="true">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('options.boolean.true') : '是'}</option>
                            <option value="false">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('options.boolean.false') : '否'}</option>
                        </select>
                        ${helpText}
                    </div>
                `;
            }
            case 'checkbox': {
                const helpText = field.helpText ? `<small class="form-text text-muted">${field.helpText.replace(/\n/g, '<br>')}</small>` : '';
                const readonlyAttr = field.readonly ? 'disabled' : '';
                const checkedAttr = (field.defaultValue === true || field.defaultValue === 'true') ? 'checked' : '';
                return `
                    <div class="mb-3 form-check" id="${defaultContainerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <input class="form-check-input" type="checkbox" id="${fieldId}" name="${field.key}" ${checkedAttr} ${readonlyAttr} ${tooltipAttr}>
                        <label class="form-check-label" for="${fieldId}" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        ${helpText}
                    </div>
                `;
            }
            case 'html-editor':
                // HTML 編輯器（使用 Quill.js）
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <div id="${fieldId}_editor" style="min-height: 200px;"></div>
                        <textarea id="${fieldId}" name="${field.key}" style="display: none;" ${field.required ? 'required' : ''}></textarea>
                    </div>
                `;
            
            case 'textarea':
                const textareaRows = field.rows || 3;
                const textareaPlaceholder = field.placeholder ? `placeholder="${this.getPlaceholder(field.key, field.placeholder)}"` : '';
                const textareaReadonly = field.readonly ? 'readonly' : '';
                const textareaReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                const textareaRequired = field.required ? 'required' : '';
                const textareaMaxlength = field.maxlength ? `maxlength="${field.maxlength}"` : '';
                const helpText = field.helpText ? `<small class="form-text text-muted">${field.helpText.replace(/\n/g, '<br>')}</small>` : '';
                return `
                    <div class="mb-3" id="${defaultContainerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <textarea class="form-control ${textareaReadonlyClass}" id="${fieldId}" name="${field.key}" rows="${textareaRows}" ${textareaPlaceholder} ${textareaReadonly} ${textareaRequired} ${textareaMaxlength} ${tooltipAttr}></textarea>
                        ${helpText}
                    </div>
                `;
            
            case 'multi-select':
            case 'multiselect':
                // 多選下拉框（支持 relationApi）
                const multiReadonlyAttr = field.readonly ? 'disabled' : '';
                const multiReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                if (field.relationApi) {
                    return `
                        <div class="mb-3">
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <select class="form-select ${multiReadonlyClass}" id="${fieldId}" multiple ${field.required ? 'required' : ''} size="5" ${multiReadonlyAttr} ${tooltipAttr}>
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                            <small class="text-muted" data-i18n="common.multiSelectHint">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.multiSelectHint') : '按住 Ctrl (Windows) 或 Cmd (Mac) 鍵進行多選'}</small>
                        </div>
                    `;
                }
                // 靜態多選選項
                const multiOptions = field.options?.map(opt => 
                    `<option value="${opt.value}">${this.getOptionLabel(field.key, opt)}</option>`
                ).join('') || '';
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <select class="form-select ${multiReadonlyClass}" id="${fieldId}" multiple ${field.required ? 'required' : ''} size="10" ${multiReadonlyAttr} ${tooltipAttr}>
                            ${multiOptions}
                        </select>
                        <small class="text-muted">按住 Ctrl (Windows) 或 Cmd (Mac) 鍵進行多選</small>
                    </div>
                `;
            
            case 'select2':
            case 'select2-multi':
                // Select2 可搜索下拉框（支持單選和多選）
                const select2ReadonlyAttr = field.readonly ? 'disabled' : '';
                const select2ReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                if (field.relationApi) {
                    const multiple = field.type === 'select2-multi' ? 'multiple' : '';
                    // 為多個相同 key 的字段生成唯一容器 ID
                    let containerId = `field_container_${field.key}`;
                    if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0) {
                        const depValue = field.dependency.values[0];
                        containerId = `field_container_${field.key}_${depValue}`;
                    }
                    return `
                        <div class="mb-3" id="${containerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <select class="form-select ${select2ReadonlyClass}" id="${fieldId}" ${multiple} ${field.required ? 'required' : ''} ${select2ReadonlyAttr} ${tooltipAttr}>
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                            ${helpTextHtml}
                        </div>
                    `;
                }
                // 支持固定選項的 select2
                if (field.options) {
                    const multiple = field.type === 'select2-multi' ? 'multiple' : '';
                    const options = field.options.map(opt =>
                        `<option value="${opt.value}">${this.getOptionLabel(field.key, opt)}</option>`
                    ).join('');
                    let select2StaticContainerId = `field_container_${field.key}`;
                    return `
                        <div class="mb-3" id="${select2StaticContainerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <select class="form-select ${select2ReadonlyClass}" id="${fieldId}" ${multiple} ${field.required ? 'required' : ''} ${select2ReadonlyAttr} ${tooltipAttr}>
                                ${field.options[0]?.value !== '' ? `<option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇'}</option>` : ''}
                                ${options}
                            </select>
                            ${helpTextHtml}
                        </div>
                    `;
                }
                break;
            case 'relation-select':
            case 'select':
                // 如果是 relation-select 或帶 relationApi 的 select，需要動態載入選項
                if (field.type === 'relation-select' || field.relationApi) {
                    // 返回一個帶搜索功能的 select
                    const readonlyAttr = field.readonly ? 'disabled' : '';
                    const readonlySelectClass = field.readonly ? 'bg-light text-muted' : '';
                    // 為多個相同 key 的字段生成唯一容器 ID
                    let containerId = `field_container_${field.key}`;
                    if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0) {
                        const depValue = field.dependency.values[0];
                        containerId = `field_container_${field.key}_${depValue}`;
                    }
                    return `
                        <div class="mb-3" id="${containerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <input type="text" class="form-control mb-2" id="${fieldId}_search" placeholder="${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.search') : '搜索'}..." style="display: none;">
                            <select class="form-select ${readonlySelectClass}" id="${fieldId}" ${field.required ? 'required' : ''} ${readonlyAttr} ${tooltipAttr}>
                                <option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.loading') : '載入中...'}</option>
                            </select>
                        </div>
                    `;
                }
                // 靜態選項
                const defaultValue = field.defaultValue !== undefined ? field.defaultValue : (field.default !== undefined ? field.default : undefined);
                const options = field.options?.map(opt => {
                    const isSelected = defaultValue !== undefined && String(opt.value) === String(defaultValue) ? 'selected' : '';
                    return `<option value="${opt.value}" ${isSelected}>${this.getOptionLabel(field.key, opt)}</option>`;
                }).join('') || '';
                // payment-methods: 付款形式必須立即觸發（避免任何綁定失敗造成「完全沒反應」）
                const pmOnChange = (this.pageName === 'payment-methods' && field.key === 'payment_type')
                    ? `onchange="if(window.dynamicForm && window.dynamicForm.handlePaymentTypeChange){window.dynamicForm.handlePaymentTypeChange();}"`
                    : '';
                const onChangeAttr = field.onChange
                    ? `onchange="${field.onChange}(this)"`
                    : pmOnChange;
                const readonlyAttr = field.readonly ? 'disabled' : '';
                const readonlySelectClass = field.readonly ? 'bg-light text-muted' : '';
                return `
                    <div class="mb-3" id="field_container_${field.key}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <select class="form-select ${readonlySelectClass}" id="${fieldId}" ${field.required ? 'required' : ''} ${onChangeAttr} ${readonlyAttr} ${tooltipAttr}>
                            ${field.options?.[0]?.value !== '' ? `<option value="">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇'}</option>` : ''}
                            ${options}
                        </select>
                    </div>
                `;
            
            case 'number':
                const numberReadonly = field.readonly ? 'readonly' : '';
                const numberReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                // 為多個相同 key 的字段生成唯一容器 ID
                let numberContainerId = `field_container_${field.key}`;
                if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0) {
                    const depValue = field.dependency.values[0];
                    numberContainerId = `field_container_${field.key}_${depValue}`;
                }
                return `
                    <div class="mb-3" id="${numberContainerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="number" class="form-control ${numberReadonlyClass}" id="${fieldId}" 
                               step="${field.step || '1'}" ${field.required ? 'required' : ''} ${numberReadonly} ${tooltipAttr}>
                    </div>
                `;
            
            case 'color':
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <div class="input-group">
                            <input type="color" class="form-control form-control-color" id="${fieldId}" 
                                   value="${field.default || '#007bff'}" ${field.required ? 'required' : ''}>
                            <input type="text" class="form-control" id="${fieldId}_text" 
                                   value="${field.default || '#007bff'}" placeholder="#007bff" 
                                   onchange="document.getElementById('${fieldId}').value = this.value">
                        </div>
                        <small class="text-muted" data-i18n="common.colorHelp">${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.colorHelp') : '點擊顏色塊或輸入十六進制顏色代碼'}</small>
                    </div>
                `;
            
            case 'date':
                const dateReadonly = field.readonly ? 'readonly' : '';
                const dateReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="date" class="form-control ${dateReadonlyClass}" id="${fieldId}" ${field.required ? 'required' : ''} ${dateReadonly} ${tooltipAttr}>
                    </div>
                `;
            
            case 'button-group':
                // Button group 字段類型（用於性別等選項）
                const buttonGroupReadonly = field.readonly ? 'disabled' : '';
                const buttonGroupReadonlyClass = field.readonly ? 'disabled' : '';
                const buttonGroupOptions = field.options || [];
                const buttonGroupDefaultValue = field.defaultValue || field.default || '';
                const buttonGroupOptionsHtml = buttonGroupOptions.map(opt => {
                    const isSelected = buttonGroupDefaultValue === opt.value ? 'active' : '';
                    // 优先使用 labelKey，否则使用 getOptionLabel
                    let optLabel = this.getOptionLabel(field.key, opt);
                    if (opt.labelKey && typeof I18n !== 'undefined' && I18n.t) {
                        const translated = I18n.t(opt.labelKey);
                        if (translated && translated !== opt.labelKey) {
                            optLabel = translated;
                        }
                    }
                    return `
                        <input type="radio" class="btn-check" name="${fieldId}_radio" id="${fieldId}_${opt.value}" value="${opt.value}" autocomplete="off" ${buttonGroupDefaultValue === opt.value ? 'checked' : ''} ${buttonGroupReadonly}>
                        <label class="btn btn-outline-primary flex-fill ${isSelected}" for="${fieldId}_${opt.value}" ${opt.labelKey ? `data-i18n="${opt.labelKey}"` : ''}>${optLabel}</label>
                    `;
                }).join('');
                return `
                    <div class="mb-3" id="field_container_${field.key}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label d-block mb-2" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <div class="btn-group w-100 ${buttonGroupReadonlyClass}" role="group" id="${fieldId}_group">
                            ${buttonGroupOptionsHtml}
                        </div>
                        <input type="hidden" id="${fieldId}" name="${field.key}" value="${buttonGroupDefaultValue}" ${field.required ? 'required' : ''}>
                    </div>
                `;
            
            case 'time':
                const timeReadonly = field.readonly ? 'readonly' : '';
                const timeReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                const timeDefaultValue = field.defaultValue || field.default || '';
                return `
                    <div class="mb-3" id="field_container_${field.key}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="time" class="form-control ${timeReadonlyClass}" id="${fieldId}" value="${timeDefaultValue}" ${field.required ? 'required' : ''} ${timeReadonly} ${tooltipAttr}>
                    </div>
                `;
            
            case 'datetime-local':
                const datetimeReadonly = field.readonly ? 'readonly' : '';
                const datetimeReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                // 如果是 reminder_time，添加刪除按鈕
                if (field.key === 'reminder_time') {
                    // 為多個相同 key 的字段生成唯一容器 ID
                    let containerId = `field_container_${field.key}`;
                    if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0) {
                        const depValue = field.dependency.values[0];
                        containerId = `field_container_${field.key}_${depValue}`;
                    }
                    return `
                        <div class="mb-3" id="${containerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <div class="input-group">
                                <input type="datetime-local" class="form-control ${datetimeReadonlyClass}" id="${fieldId}" ${field.required ? 'required' : ''} ${datetimeReadonly} ${tooltipAttr}>
                                <button type="button" class="btn btn-outline-secondary" onclick="document.getElementById('${fieldId}').value = '';" title="清除">
                                    <i class="bi bi-x"></i>
                                </button>
                            </div>
                        </div>
                    `;
                }
                return `
                    <div class="mb-3" id="field_container_${field.key}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="datetime-local" class="form-control ${datetimeReadonlyClass}" id="${fieldId}" ${field.required ? 'required' : ''} ${datetimeReadonly} ${tooltipAttr}>
                    </div>
                `;
            
            case 'email':
                const emailReadonly = field.readonly ? 'readonly' : '';
                const emailReadonlyClass = field.readonly ? 'bg-light text-muted' : '';
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="email" class="form-control ${emailReadonlyClass}" id="${fieldId}" ${field.required ? 'required' : ''} ${emailReadonly} ${tooltipAttr}>
                    </div>
                `;
            
            case 'profile-image':
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <div class="d-flex align-items-center gap-3 mb-2">
                            <div class="profile-pic-preview" style="width: 80px; height: 80px; border-radius: 50%; overflow: hidden; border: 2px solid #dee2e6; background-color: #f8f9fa; display: flex; align-items: center; justify-content: center;">
                                <img id="${fieldId}_preview" src="" alt="${t('pages.personalData.avatar','頭像')}" style="width: 100%; height: 100%; object-fit: cover; display: none;">
                                <i class="bi bi-person fs-1 text-muted" id="${fieldId}_placeholder"></i>
                            </div>
                            <div class="flex-grow-1">
                                <input type="file" class="form-control" id="${fieldId}" accept="image/*" style="display: none;">
                                <input type="hidden" id="${fieldId}_url" name="${field.key}">
                                <button type="button" class="btn btn-sm btn-outline-primary" id="${fieldId}_uploadBtn">
                                    <i class="bi bi-upload"></i> ${t('pages.personalData.uploadAvatar','上傳頭像')}
                                </button>
                                <button type="button" class="btn btn-sm btn-outline-danger" id="${fieldId}_removeBtn" style="display: none;">
                                    <i class="bi bi-trash"></i> ${t('pages.personalData.removeAvatar','移除頭像')}
                                </button>
                            </div>
                        </div>
                        <small class="text-muted">${t('pages.personalData.avatarHint','支持 JPG、PNG、GIF 格式，建議尺寸 200x200 像素')}</small>
                    </div>
                `;
            
            case 'file':
                const acceptAttr = field.accept ? `accept="${field.accept}"` : '';
                // 對於 products, vehicles, equipments, services, projects, stores 的 image_url 字段，以及 brands 的 logo_url 字段，使用類似 profile-image 的界面
                if ((field.key === 'image_url' && (this.pageName === 'products' || this.pageName === 'vehicles' || this.pageName === 'equipments' || this.pageName === 'services' || this.pageName === 'projects' || this.pageName === 'stores' || this.pageName === 'rooms' || this.pageName === 'brands' || this.pageName === 'product-types')) ||
                    (field.key === 'logo_url' && this.pageName === 'brands') ||
                    (field.key === 'featured_image' && this.pageName === 'blogs')) {
                    return `
                        <div class="mb-3">
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <div class="d-flex align-items-center gap-3 mb-2">
                                ${(this.pageName === 'projects' || (this.pageName === 'blogs' && field.key === 'featured_image')) ? 
                                    `<div class="image-preview" style="width: 142px; height: 80px; border-radius: 8px; overflow: hidden; border: 2px solid #dee2e6; background-color: #f8f9fa; display: flex; align-items: center; justify-content: center;">
                                        <img id="${fieldId}_preview" src="" alt="圖片預覽" style="width: 100%; height: 100%; object-fit: cover; display: none;">
                                        <i class="bi bi-image fs-1 text-muted" id="${fieldId}_placeholder"></i>
                                    </div>` :
                                    `<div class="image-preview" style="width: 80px; height: 80px; border-radius: 8px; overflow: hidden; border: 2px solid #dee2e6; background-color: #f8f9fa; display: flex; align-items: center; justify-content: center;">
                                        <img id="${fieldId}_preview" src="" alt="圖片預覽" style="width: 100%; height: 100%; object-fit: cover; display: none;">
                                        <i class="bi bi-image fs-1 text-muted" id="${fieldId}_placeholder"></i>
                                    </div>`
                                }
                                <div class="flex-grow-1">
                                    <input type="file" class="form-control" id="${fieldId}" accept="image/*" style="display: none;">
                                    <input type="hidden" id="${fieldId}_url" name="${field.key}">
                                    <button type="button" class="btn btn-sm btn-outline-primary" id="${fieldId}_uploadBtn">
                                        <i class="bi bi-upload"></i> ${t('common.uploadImage', '上傳圖片')}
                                    </button>
                                    <button type="button" class="btn btn-sm btn-outline-danger" id="${fieldId}_removeBtn" style="display: none;">
                                        <i class="bi bi-trash"></i> ${t('common.removeImage', '移除圖片')}
                                    </button>
                                </div>
                            </div>
                            <small class="text-muted">${(this.pageName === 'projects' || (this.pageName === 'blogs' && field.key === 'featured_image')) ? t('common.imageHintWide', '支持 JPG、PNG、GIF 格式，建議尺寸 1920x1080 像素（16:9）') : t('common.imageHint', '支持 JPG、PNG、GIF 格式，建議尺寸 200x200 像素')}</small>
                        </div>
                    `;
                }
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="file" class="form-control" id="${fieldId}" ${acceptAttr} ${field.required ? 'required' : ''}>
                        ${field.key === 'image_url' && this.pageName === 'stores' ? '<div id="imagePreview" class="mt-3" style="display: none !important; position: relative; width: 67px; margin-top: 0.75rem;"><img id="previewImg" alt="預覽" style="max-width: 67px; max-height: 67px; width: 67px; height: 67px; object-fit: cover; border: 1px solid #ddd; border-radius: 4px; cursor: pointer;" onerror="this.style.display=\'none\'; this.setAttribute(\'data-error-handled\',\'true\'); this.onerror=null;"><button type="button" id="deleteImageBtn" class="btn btn-sm btn-danger" style="position: absolute; top: -8px; right: -8px; width: 20px; height: 20px; padding: 0; border-radius: 50%; display: flex; align-items: center; justify-content: center; z-index: 10; box-shadow: 0 2px 4px rgba(0,0,0,0.2);" onclick="if(window.dynamicForm) window.dynamicForm.deleteImage(); return false;" title="刪除圖片"><i class="bi bi-x" style="font-size: 12px;"></i></button></div>' : (field.key === 'image_url' ? '<div id="imagePreview" class="mt-3" style="display: none; position: relative; display: inline-block; width: 67px; margin-top: 0.75rem;"><img id="previewImg" alt="預覽" style="max-width: 67px; max-height: 67px; width: 67px; height: 67px; object-fit: cover; border: 1px solid #ddd; border-radius: 4px; cursor: pointer;" onerror="this.style.display=\'none\'; this.setAttribute(\'data-error-handled\',\'true\'); this.onerror=null;"><button type="button" id="deleteImageBtn" class="btn btn-sm btn-danger" style="position: absolute; top: -8px; right: -8px; width: 20px; height: 20px; padding: 0; border-radius: 50%; display: flex; align-items: center; justify-content: center; z-index: 10; box-shadow: 0 2px 4px rgba(0,0,0,0.2);" onclick="if(window.dynamicForm) window.dynamicForm.deleteImage(); return false;" title="刪除圖片"><i class="bi bi-x" style="font-size: 12px;"></i></button></div>' : '')}
                    </div>
                `;
            
            case 'password':
                const passwordPlaceholderAttr = field.placeholder ? `placeholder="${this.getPlaceholder(field.key, field.placeholder)}"` : '';
                const passwordHintText = (field.minlength && field.maxlength)
                    ? `${field.minlength}-${field.maxlength} ${t('common.charactersUnit', '個字符')}`
                    : '';
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="password" class="form-control" id="${fieldId}" autocomplete="${this.isEdit ? 'new-password' : 'new-password'}" 
                               ${field.required ? 'required' : ''} 
                               ${field.minlength ? `minlength="${field.minlength}"` : ''} 
                               ${field.maxlength ? `maxlength="${field.maxlength}"` : ''} ${passwordPlaceholderAttr}>
                        ${passwordHintText ? `<small class="text-muted">${passwordHintText}</small>` : ''}
                    </div>
                `;
            
            case 'checkbox-group':
                // 支持按 session 分组的 checkbox
                if (field.sessions) {
                    let sessionsHtml = '';
                    field.sessions.forEach(session => {
                        const sessionLabel = typeof I18n !== 'undefined' && I18n.t ? I18n.t(`menu.${session.key}`) : session.label;
                        let checkboxesHtml = '';
                        session.options.forEach(opt => {
                            const optLabel = typeof I18n !== 'undefined' && I18n.t ? I18n.t(`menu.${opt.value}`) : opt.label;
                            checkboxesHtml += `
                                <div class="col-md-6 col-lg-4">
                                    <div class="form-check">
                                        <input class="form-check-input" type="checkbox" value="${opt.value}" id="${fieldId}_${opt.value}" name="${field.key}[]">
                                        <label class="form-check-label" for="${fieldId}_${opt.value}">
                                            ${optLabel}
                                        </label>
                                    </div>
                                </div>
                            `;
                        });
                        sessionsHtml += `
                            <div class="mb-4">
                                <h6 class="mb-3 fw-bold text-primary border-bottom pb-2">${sessionLabel}</h6>
                                <div class="row g-2">
                                    ${checkboxesHtml}
                                </div>
                            </div>
                        `;
                    });
                    return `
                        <div class="mb-4">
                            <label class="form-label fw-bold mb-3" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <div class="border rounded p-3 bg-light">
                                ${sessionsHtml}
                            </div>
                        </div>
                    `;
                }
                // 如果没有 sessions，使用简单的 checkbox 列表
                const checkboxOptions = field.options?.map(opt => {
                    const optLabel = typeof I18n !== 'undefined' && I18n.t ? I18n.t(`menu.${opt.value}`) : opt.label;
                    return `
                        <div class="form-check">
                            <input class="form-check-input" type="checkbox" value="${opt.value}" id="${fieldId}_${opt.value}" name="${field.key}[]">
                            <label class="form-check-label" for="${fieldId}_${opt.value}">
                                ${optLabel}
                            </label>
                        </div>
                    `;
                }).join('') || '';
                return `
                    <div class="mb-3">
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <div class="ps-3">
                            ${checkboxOptions}
                        </div>
                    </div>
                `;
            
            case 'custom':
                if (field.render && typeof field.render === 'function') {
                    return field.render();
                }
                return '';
            
            default: // text
                // 特殊處理：my_referral_code 在未生成時不顯示
                if (field.key === 'my_referral_code') {
                    const placeholderAttr = field.placeholder ? `placeholder="${this.getPlaceholder(field.key, field.placeholder)}"` : '';
                    return `
                        <div class="mb-3" id="my_referral_code_container" style="display: none;">
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <input type="text" class="form-control bg-light text-muted" id="${fieldId}" readonly 
                                   ${placeholderAttr}
                                   ${tooltipAttr}>
                        </div>
                    `;
                }
                // 特殊處理：barcode 字段添加預覽鏈接
                if (field.key === 'barcode') {
                    const readonly = field.readonly ? 'readonly' : '';
                    const readonlyClass = field.readonly ? 'bg-light text-muted' : '';
                    const placeholderAttr = field.placeholder ? `placeholder="${this.getPlaceholder(field.key, field.placeholder)}"` : '';
                    return `
                        <div class="mb-3">
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <div class="input-group">
                                <input type="text" class="form-control ${readonlyClass}" id="${fieldId}" 
                                       ${field.required ? 'required' : ''} ${readonly} 
                                       ${placeholderAttr}
                                       onchange="updateBarcodePreview('${fieldId}')" ${tooltipAttr}>
                                <button type="button" class="btn btn-outline-secondary" id="${fieldId}_preview_btn" 
                                        onclick="showBarcodePreview('${fieldId}')" style="display: none;">
                                    <i class="bi bi-upc-scan"></i> ${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.previewBarcode') : '預覽條碼'}
                                </button>
                            </div>
                        </div>
                    `;
                }
                // 特殊處理：sku 字段添加 QR code 預覽
                if (field.key === 'sku') {
                    const readonly = field.readonly ? 'readonly' : '';
                    const readonlyClass = field.readonly ? 'bg-light text-muted' : '';
                    const placeholderAttr = field.placeholder ? `placeholder="${this.getPlaceholder(field.key, field.placeholder)}"` : '';
                    return `
                        <div class="mb-3">
                            <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                            <div class="input-group">
                                <input type="text" class="form-control ${readonlyClass}" id="${fieldId}" 
                                       ${field.required ? 'required' : ''} ${readonly} 
                                       ${placeholderAttr}
                                       onchange="updateQRCodePreview('${fieldId}')" ${tooltipAttr}>
                                <button type="button" class="btn btn-outline-secondary" id="${fieldId}_preview_btn" 
                                        onclick="showQRCodePreview('${fieldId}')" style="display: none;">
                                    <i class="bi bi-qr-code-scan"></i> ${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.previewQrCode') : '預覽 QR Code'}
                                </button>
                            </div>
                        </div>
                    `;
                }
                const readonly = field.readonly ? 'readonly' : '';
                const readonlyClass = field.readonly ? 'bg-light text-muted' : '';
                const placeholderAttr = field.placeholder ? `placeholder="${this.getPlaceholder(field.key, field.placeholder)}"` : '';
                return `
                    <div class="mb-3" id="${defaultContainerId}" ${field.dependency ? 'style="display: none;"' : ''}>
                        <label class="form-label" data-i18n-field="${field.key}">${fieldLabel} ${required}</label>
                        <input type="text" class="form-control ${readonlyClass}" id="${fieldId}" 
                               ${field.required ? 'required' : ''} ${readonly} 
                               ${placeholderAttr} ${tooltipAttr}>
                        ${helpTextHtml}
                    </div>
                `;
        }
    }

    // ==================== 欄位設定功能 ====================
    
    // 獲取欄位設定 API 路徑
    getFieldSettingsApiPath() {
        return `/user/form-field-settings/${this.pageName}`;
    }
    
    // 載入欄位設定（同步版本，從快取讀取）
    loadFieldSettings() {
        // 如果已經載入過，直接使用快取
        if (this.fieldSettings) {
            return;
        }
        
        // 嘗試從快取讀取（首次渲染時使用）
        try {
            const cacheKey = `form_field_settings_cache_${this.pageName}`;
            const cached = sessionStorage.getItem(cacheKey);
            if (cached) {
                this.fieldSettings = JSON.parse(cached);
                return;
            }
        } catch (e) {
            console.warn('讀取欄位設定快取失敗:', e);
        }
        
        // 如果沒有快取，初始化預設設定
        this.fieldSettings = {
            fields: this.config.formFields.map((f, idx) => ({
                key: f.key,
                visible: true,
                order: idx
            })),
            extraFields: []
        };
    }
    
    // 從 API 載入欄位設定（異步）
    async loadFieldSettingsFromAPI() {
        // 防護：如果用戶正在 modal 中編輯 extraFields，不要覆蓋
        if (this._fieldSettingsDirty) {
            console.log('[DynamicForm][GUARD] _fieldSettingsDirty=true, 跳過 loadFieldSettingsFromAPI 避免覆蓋用戶修改');
            return;
        }
        try {
            const response = await App.apiRequest(this.getFieldSettingsApiPath());
            // 再次檢查：await 期間用戶可能已修改 extraFields
            if (this._fieldSettingsDirty) {
                console.log('[DynamicForm][GUARD] _fieldSettingsDirty=true (post-await), 跳過覆蓋');
                return;
            }
            console.log('[DynamicForm][DEBUG] API 回應原始數據:', JSON.stringify(response));
            if (response && response.field_config) {
                const config = response.field_config;
                // Debug: extraFields 狀態
                console.log('[DynamicForm][DEBUG] field_config.extraFields:', JSON.stringify(config.extraFields || 'undefined'));
                console.log('[DynamicForm][DEBUG] field_config.fields count:', (config.fields || []).length);
                
                if (config.fields && config.fields.length > 0) {
                    this.fieldSettings = config;
                    // Debug: 顯示載入的欄位設定（含 defaultValue）
                    const fieldsWithDefaults = config.fields.filter(f => f.defaultValue !== undefined && f.defaultValue !== '');
                    if (fieldsWithDefaults.length > 0) {
                        console.log('[DynamicForm] 載入欄位設定，含預設值的欄位:', fieldsWithDefaults.map(f => `${f.key}=${f.defaultValue}`));
                    }
                    console.log('[DynamicForm][DEBUG] 載入後 this.fieldSettings.extraFields count:', (this.fieldSettings.extraFields || []).length);
                } else {
                    // 初始化預設設定
                    console.log('[DynamicForm][DEBUG] fields 為空，使用預設設定（保留 extraFields）');
                    this.fieldSettings = {
                        fields: this.config.formFields.map((f, idx) => ({
                            key: f.key,
                            visible: true,
                            order: idx
                        })),
                        extraFields: config.extraFields || []
                    };
                }
            } else {
                // 初始化預設設定
                console.log('[DynamicForm][DEBUG] 無 field_config，使用預設設定');
                this.fieldSettings = {
                    fields: this.config.formFields.map((f, idx) => ({
                        key: f.key,
                        visible: true,
                        order: idx
                    })),
                    extraFields: []
                };
            }
            
            // 快取到 sessionStorage
            const cacheKey = `form_field_settings_cache_${this.pageName}`;
            sessionStorage.setItem(cacheKey, JSON.stringify(this.fieldSettings));
            
        } catch (e) {
            console.warn('從 API 載入欄位設定失敗:', e);
            // 使用預設設定
            this.fieldSettings = {
                fields: this.config.formFields.map((f, idx) => ({
                    key: f.key,
                    visible: true,
                    order: idx
                })),
                extraFields: []
            };
        }
    }
    
    // 保存欄位設定到 API
    async saveFieldSettings() {
        try {
            // 保存前清理 readonly 欄位的預設值
            if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.sanitizeFieldSettingsBeforeSave) {
                this.fieldSettings = FormFieldSettingsHandler.sanitizeFieldSettingsBeforeSave(this.fieldSettings, this);
            }
            
            // Debug: 確認保存前的 extraFields 狀態
            console.log('[DynamicForm][DEBUG] 保存前 extraFields count:', (this.fieldSettings.extraFields || []).length);
            console.log('[DynamicForm][DEBUG] 保存前 extraFields:', JSON.stringify(this.fieldSettings.extraFields || []));
            console.log('[DynamicForm][DEBUG] 保存前 fields count:', (this.fieldSettings.fields || []).length);
            
            const response = await App.apiRequest(this.getFieldSettingsApiPath(), {
                method: 'POST',
                body: JSON.stringify({
                    field_config: this.fieldSettings
                })
            });
            
            // Debug: 確認 API 回應
            console.log('[DynamicForm][DEBUG] Save API 回應:', JSON.stringify(response));
            
            // Debug: 確認保存的欄位設定含 defaultValue
            const savedDefaults = (this.fieldSettings.fields || []).filter(f => f.defaultValue !== undefined && f.defaultValue !== '');
            if (savedDefaults.length > 0) {
                console.log('[DynamicForm] 已保存欄位設定，含預設值:', savedDefaults.map(f => `${f.key}=${f.defaultValue}`));
            }
            
            // 更新快取
            const cacheKey = `form_field_settings_cache_${this.pageName}`;
            sessionStorage.setItem(cacheKey, JSON.stringify(this.fieldSettings));
            
            // 保存成功後清除 dirty flag（DB 已同步）
            this._fieldSettingsDirty = false;
            
            return response;
        } catch (e) {
            console.error('保存欄位設定失敗:', e);
            throw e;
        }
    }
    
    // 獲取有效的欄位列表（根據設定排序和過濾）
    getEffectiveFormFields() {
        if (!this.fieldSettings) {
            this.loadFieldSettings();
        }
        
        const settings = this.fieldSettings;
        if (!settings || !settings.fields) {
            return this.config.formFields;
        }
        
        // 建立欄位 map
        const fieldMap = {};
        this.config.formFields.forEach(f => {
            fieldMap[f.key] = f;
        });
        
        // 加入額外欄位
        if (settings.extraFields) {
            settings.extraFields.forEach(ef => {
                fieldMap[ef.key] = ef;
            });
        }

        // 套用預設值（欄位設定優先）
        if (settings.fields) {
            settings.fields.forEach(sf => {
                if (sf.defaultValue !== undefined && fieldMap[sf.key]) {
                    fieldMap[sf.key].defaultValue = sf.defaultValue;
                }
            });
        }
        
        // 根據設定排序和過濾
        const result = [];
        const sortedFields = [...settings.fields].sort((a, b) => a.order - b.order);
        
        sortedFields.forEach(sf => {
            if (sf.visible && fieldMap[sf.key]) {
                result.push(fieldMap[sf.key]);
            }
        });
        
        // 添加任何新增但不在設定中的欄位（可能是配置更新後新增的）
        this.config.formFields.forEach(f => {
            const inSettings = settings.fields.some(sf => sf.key === f.key);
            if (!inSettings) {
                result.push(f);
            }
        });
        
        return result;
    }
    
    // 打開欄位設定 Modal
    async openFieldSettingsModal() {
        console.log('[DynamicForm] openFieldSettingsModal called');
        try {
            // 重置 dirty flag，允許 loadFieldSettingsFromAPI 正常載入
            this._fieldSettingsDirty = false;
            // 從 API 載入當前設定
            await this.loadFieldSettingsFromAPI();
            console.log('[DynamicForm] loadFieldSettingsFromAPI done');
            
            const getText = (key, fallback) => {
            if (typeof I18n !== 'undefined' && I18n.t) {
                const t = I18n.t(key);
                return (t && t !== key) ? t : fallback;
            }
            return fallback;
        };
        
        // 建立 Modal HTML
        const modalId = 'fieldSettingsModal';
        let existingModal = document.getElementById(modalId);
        if (existingModal) {
            existingModal.remove();
        }
        
        // 建立欄位 map 用於獲取標籤
        const fieldMap = {};
        this.config.formFields.forEach(f => {
            fieldMap[f.key] = f;
        });
        
        // 加入額外欄位
        if (this.fieldSettings.extraFields) {
            this.fieldSettings.extraFields.forEach(ef => {
                fieldMap[ef.key] = ef;
            });
        }
        
        // 獲取欄位標籤
        const getFieldLabel = (fieldKey) => {
            const field = fieldMap[fieldKey];
            if (!field) return fieldKey;
            
            // 嘗試翻譯
            const translationKey = `fields.${fieldKey}`;
            let label = getText(translationKey, null);
            if (!label || label === translationKey) {
                label = field.label || fieldKey;
            }
            return label;
        };

        const getFieldTypeLabel = (fieldKey) => {
            const field = fieldMap[fieldKey];
            if (!field) return 'text';
            return field.type || 'text';
        };

        const getTypeBadge = (typeLabel) => {
            const typeMap = {
                text: getText('common.fieldTypeText', '文字'),
                number: getText('common.fieldTypeNumber', '數字'),
                date: getText('common.fieldTypeDate', '日期'),
                datetime: getText('common.fieldTypeDate', '日期'),
                'datetime-local': getText('common.fieldTypeDate', '日期'),
                textarea: getText('common.fieldTypeTextarea', '多行文字'),
                email: getText('common.fieldTypeEmail', '郵箱'),
                select: getText('common.fieldTypeSelect', '下拉選單'),
                'select2': getText('common.fieldTypeSelect', '下拉選單'),
                'select2-multi': getText('common.fieldTypeSelect', '下拉選單'),
                password: getText('common.fieldTypePassword', '密碼'),
                file: getText('common.fieldTypeFile', '檔案'),
                'html-editor': getText('common.fieldTypeTextarea', '多行文字'),
                'button-group': getText('common.fieldTypeSelect', '下拉選單')
            };
            const label = typeMap[typeLabel] || typeLabel;
            return `<span class="badge bg-secondary ms-2">${label}</span>`;
        };
        
        // 排序欄位列表
        const sortedFields = [...this.fieldSettings.fields].sort((a, b) => a.order - b.order);
        
        const getDefaultValueInputHtml = (sf) => {
            const field = fieldMap[sf.key];
            const typeLabel = getFieldTypeLabel(sf.key);
            const rawDefault = sf.defaultValue !== undefined && sf.defaultValue !== null ? sf.defaultValue : '';
            const defaultValue = String(rawDefault);

            const escapeHtml = (val) => String(val)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;');

            const normalizeSelected = (value, multiple) => {
                if (Array.isArray(value)) {
                    return value.map(v => String(v));
                }
                if (!value) return [];
                if (multiple && String(value).includes(',')) {
                    return String(value).split(',').map(v => v.trim()).filter(Boolean);
                }
                return [String(value)];
            };

            if (['select', 'relation-select', 'select2', 'select2-multi', 'button-group'].includes(typeLabel)) {
                const isMultiple = typeLabel === 'select2-multi';
                const apiPath = field?.api || field?.relationApi;
                const hasOptions = Array.isArray(field?.options) && field.options.length > 0;
                const placeholder = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇';
                
                // 如果有 relationApi 但沒有固定 options，使用 Select2 remote data
                if (apiPath && !hasOptions && (typeLabel === 'select2' || typeLabel === 'select2-multi')) {
                    // 生成帶有特殊標記的 select，後續會在 modal 顯示後初始化 Select2 remote
                    const dataAttrs = `data-field-key="${sf.key}" data-field-type="${typeLabel}" data-api-path="${escapeHtml(apiPath)}"`;
                    const relationLabelKey = field?.relationLabelKey || field?.relationLabel || 'name';
                    const relationValueKey = field?.relationValueKey || field?.relationKey || 'id';
                    const relationDisplayFormat = field?.relationDisplayFormat || '';
                    const relationLabelFields = (Array.isArray(field?.relationLabelFields) ? field.relationLabelFields.join(',') : '') || '';
                    const extraAttrs = `data-relation-label-key="${escapeHtml(relationLabelKey)}" data-relation-value-key="${escapeHtml(relationValueKey)}" data-relation-display-format="${escapeHtml(relationDisplayFormat)}" data-relation-label-fields="${escapeHtml(relationLabelFields)}" data-default-value="${escapeHtml(defaultValue)}"`;
                    return `<select class="form-select form-select-sm field-default-input field-default-select2-remote" ${dataAttrs} ${extraAttrs} ${isMultiple ? 'multiple' : ''} style="width: 100%;"></select>`;
                }
                
                // 有固定 options 的情況，使用普通 select 或 Select2 static
                const selectedValues = new Set(normalizeSelected(rawDefault, isMultiple));
                const options = Array.isArray(field?.options) ? field.options : [];
                let optionsHtml = isMultiple ? '' : `<option value="">${escapeHtml(placeholder)}</option>`;

                options.forEach(opt => {
                    const value = opt && opt.value != null ? String(opt.value) : String(opt || '');
                    const label = opt && typeof opt === 'object' ? this.getOptionLabel(sf.key, opt) : value;
                    const selected = selectedValues.has(value) ? 'selected' : '';
                    optionsHtml += `<option value="${escapeHtml(value)}" ${selected}>${escapeHtml(label)}</option>`;
                });

                return `<select class="form-select form-select-sm field-default-input" data-field-key="${sf.key}" data-field-type="${typeLabel}" ${isMultiple ? 'multiple' : ''}>${optionsHtml}</select>`;
            }

            if (typeLabel === 'checkbox') {
                const checked = defaultValue === 'true' || defaultValue === '1' ? 'checked' : '';
                return `<div class="form-check"><input type="checkbox" class="form-check-input field-default-input" data-field-key="${sf.key}" data-field-type="checkbox" ${checked}><label class="form-check-label small">${getText('common.defaultChecked', '預設勾選')}</label></div>`;
            }

            if (typeLabel === 'date' || typeLabel === 'datetime' || typeLabel === 'datetime-local') {
                return `<input type="${typeLabel === 'date' ? 'date' : 'datetime-local'}" class="form-control form-control-sm field-default-input" data-field-key="${sf.key}" value="${escapeHtml(defaultValue)}">`;
            }

            if (typeLabel === 'number') {
                return `<input type="number" class="form-control form-control-sm field-default-input" data-field-key="${sf.key}" value="${escapeHtml(defaultValue)}">`;
            }

            if (typeLabel === 'textarea' || typeLabel === 'html-editor') {
                return `<textarea class="form-control form-control-sm field-default-input" data-field-key="${sf.key}" rows="2">${escapeHtml(defaultValue)}</textarea>`;
            }

            // 圖片/檔案類型 - 支援上傳預設圖片
            if (typeLabel === 'file' || typeLabel === 'profile-image') {
                const previewStyle = defaultValue ? '' : 'display: none;';
                const previewSrc = defaultValue || '';
                return `
                    <div class="field-default-image-upload" data-field-key="${sf.key}">
                        <div class="d-flex align-items-center gap-2">
                            <div class="field-default-image-preview" style="width: 60px; height: 60px; border: 1px dashed #dee2e6; border-radius: 6px; overflow: hidden; display: flex; align-items: center; justify-content: center; background: #fff; ${previewStyle}">
                                <img src="${escapeHtml(previewSrc)}" style="max-width: 100%; max-height: 100%; object-fit: cover;">
                            </div>
                            <div class="flex-grow-1">
                                <input type="file" class="form-control form-control-sm field-default-image-file" accept="image/*" style="display: none;">
                                <input type="hidden" class="field-default-input" data-field-key="${sf.key}" value="${escapeHtml(defaultValue)}">
                                <button type="button" class="btn btn-sm btn-outline-secondary field-default-image-btn">
                                    <i class="bi bi-upload me-1"></i>${getText('common.uploadImage', '上傳圖片')}
                                </button>
                                <button type="button" class="btn btn-sm btn-outline-danger field-default-image-clear ms-1" style="${defaultValue ? '' : 'display: none;'}">
                                    <i class="bi bi-x"></i>
                                </button>
                            </div>
                        </div>
                    </div>
                `;
            }

            return `<input type="text" class="form-control form-control-sm field-default-input" data-field-key="${sf.key}" value="${escapeHtml(defaultValue)}">`;
        };

        // 檢查欄位是否為 readonly
        const isFieldReadonly = (fieldKey) => {
            const field = fieldMap[fieldKey];
            // 只有明確設定 readonly: true 才認定為 readonly
            const result = field && field.readonly === true;
            if (result) {
                console.log(`[FieldSettings] 欄位 ${fieldKey} 是 readonly`);
            }
            return result;
        };

        // 生成欄位列表 HTML
        const fieldsListHtml = sortedFields.map((sf, idx) => {
            const isExtraField = this.fieldSettings.extraFields?.some(ef => ef.key === sf.key);
            const typeLabel = getFieldTypeLabel(sf.key);
            const isReadonly = isFieldReadonly(sf.key);
            const defaultValueInputHtml = getDefaultValueInputHtml(sf);
            
            // Readonly 欄位顯示標籤
            const readonlyBadge = isReadonly ? '<span class="badge bg-warning ms-2">Readonly</span>' : '';
            
            // 預設值區域：readonly 欄位顯示提示，非 readonly 欄位顯示輸入
            const defaultValueBoxHtml = isReadonly 
                ? `<div class="field-default-value-box d-none" style="background: #fff8e1; border: 1px solid #ffe082; border-top: none; border-radius: 0 0 6px 6px; padding: 12px 16px; margin-bottom: 8px;">
                        <div class="text-muted small"><i class="bi bi-lock me-1"></i>${getText('common.readonlyFieldNoDefault', 'Readonly 欄位無法設定預設值')}</div>
                    </div>` 
                : `<div class="field-default-value-box d-none" style="background: #f8f9fa; border: 1px solid #dee2e6; border-top: none; border-radius: 0 0 6px 6px; padding: 12px 16px; margin-bottom: 8px;">
                        <label class="form-label small text-muted mb-2"><i class="bi bi-pencil-square me-1"></i>${getText('common.defaultValue', '預設值')}</label>
                        ${defaultValueInputHtml}
                    </div>`;
            
            return `
                <div class="field-settings-wrapper" data-field-key="${sf.key}" ${isReadonly ? 'data-readonly="true"' : ''}>
                    <div class="field-settings-item" draggable="true">
                        <div class="field-drag-handle">
                            <i class="bi bi-grip-vertical"></i>
                        </div>
                        <div class="field-info">
                            <span class="field-label">${getFieldLabel(sf.key)}</span>
                            <span class="field-key text-muted">(${sf.key})</span>
                            ${isExtraField ? '<span class="badge bg-info ms-2">額外</span>' : ''}
                            ${readonlyBadge}
                            ${getTypeBadge(typeLabel)}
                        </div>
                        <div class="field-actions">
                            <button type="button" class="btn btn-sm btn-link field-visibility-toggle p-0" data-visible="${sf.visible ? 'true' : 'false'}" data-field-key="${sf.key}">
                                <i class="bi ${sf.visible ? 'bi-eye text-success' : 'bi-eye-slash text-muted'}" style="font-size: 1.2rem;"></i>
                            </button>
                            ${isExtraField ? `<button type="button" class="btn btn-sm btn-outline-danger delete-extra-field ms-2" data-key="${sf.key}"><i class="bi bi-trash"></i></button>` : ''}
                        </div>
                    </div>
                    ${defaultValueBoxHtml}
                </div>
            `;
        }).join('');
        
        const modalHtml = `
            <div class="modal fade" id="${modalId}" tabindex="-1" aria-labelledby="${modalId}Label" aria-hidden="true">
                <div class="modal-dialog modal-lg">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title" id="${modalId}Label">
                                <i class="bi bi-gear me-2"></i>${getText('common.fieldSettings', '欄位設定')}
                            </h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                        </div>
                        <div class="modal-body">
                            <div class="mb-3">
                                <p class="text-muted mb-2">
                                    <i class="bi bi-info-circle me-1"></i>
                                    ${getText('common.fieldSettingsHelp', '拖動欄位可調整順序，點擊眼睛圖標控制顯示/隱藏')}
                                </p>
                            </div>
                            <div class="field-settings-list" id="fieldSettingsList">
                                ${fieldsListHtml}
                            </div>
                            <hr class="my-3">
                            <div class="add-extra-field-section">
                                <h6><i class="bi bi-plus-circle me-2"></i>${getText('common.addExtraField', '添加額外欄位')}</h6>
                                <div class="row g-2">
                                    <div class="col-md-4">
                                        <input type="text" class="form-control" id="extraFieldKey" placeholder="${getText('common.fieldKey', '欄位 Key')}">
                                    </div>
                                    <div class="col-md-4">
                                        <input type="text" class="form-control" id="extraFieldLabel" placeholder="${getText('common.fieldLabel', '欄位標籤')}">
                                    </div>
                                    <div class="col-md-3">
                                        <select class="form-select" id="extraFieldType">
                                            <option value="text">${getText('common.fieldTypeText', '文字')}</option>
                                            <option value="number">${getText('common.fieldTypeNumber', '數字')}</option>
                                            <option value="date">${getText('common.fieldTypeDate', '日期')}</option>
                                            <option value="textarea">${getText('common.fieldTypeTextarea', '多行文字')}</option>
                                            <option value="email">${getText('common.fieldTypeEmail', '郵箱')}</option>
                                            <option value="select">${getText('common.fieldTypeSelect', '下拉選單')}</option>
                                        </select>
                                    </div>
                                    <div class="col-md-1">
                                        <button type="button" class="btn btn-primary w-100" id="addExtraFieldBtn">
                                            <i class="bi bi-plus"></i>
                                        </button>
                                    </div>
                                </div>
                                <div class="row mt-2" id="selectOptionsRow" style="display: none;">
                                    <div class="col-12">
                                        <select class="form-select" id="extraFieldOptions" multiple style="width: 100%;"></select>
                                        <small class="text-muted">${getText('common.fieldTypeSelectOptions', '選項（用逗號分隔或按 Enter）')}</small>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">
                                ${getText('common.cancel', '取消')}
                            </button>
                            <button type="button" class="btn btn-primary" id="saveFieldSettingsBtn">
                                <i class="bi bi-save me-1"></i>${getText('common.save', '保存')}
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        document.body.insertAdjacentHTML('beforeend', modalHtml);
        
        const modal = new bootstrap.Modal(document.getElementById(modalId));
        modal.show();
        
        // 綁定事件
        this.bindFieldSettingsEvents();
        
        // Modal 顯示後初始化 Select2 remote 欄位，並禁用 readonly 欄位的預設值
        const modalEl = document.getElementById(modalId);
        if (modalEl) {
            modalEl.addEventListener('shown.bs.modal', () => {
                this.initFieldSettingsSelect2Remote();
                // 禁用 readonly 欄位的預設值輸入
                if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.disableDefaultValueForReadonlyFields) {
                    FormFieldSettingsHandler.disableDefaultValueForReadonlyFields(this);
                }
            }, { once: true });
        }
        } catch (e) {
            console.error('[DynamicForm] openFieldSettingsModal failed:', e);
            App.showError('無法打開欄位設定');
        }
    }
    
    // 初始化欄位設定 Modal 中的 Select2 remote 欄位
    async initFieldSettingsSelect2Remote() {
        const remoteSelects = document.querySelectorAll('.field-default-select2-remote');
        if (!remoteSelects.length) return;
        
        // 等待 jQuery 和 Select2 加載
        let retries = 0;
        while ((typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') && retries < 20) {
            await new Promise(resolve => setTimeout(resolve, 100));
            retries++;
        }
        
        if (typeof $ === 'undefined' || typeof $.fn.select2 === 'undefined') {
            console.error('jQuery 或 Select2 未加載，無法初始化欄位設定 Select2');
            return;
        }
        
        const getSelect2Language = () => {
            const fallback = {
                noResults: () => '未找到結果',
                searching: () => '搜索中...'
            };
            if (typeof I18n === 'undefined' || !I18n.t) return fallback;
            return {
                noResults: () => {
                    const k = 'common.noResults';
                    const t = I18n.t(k);
                    return (t && t !== k) ? t : fallback.noResults();
                },
                searching: () => {
                    const k = 'common.searching';
                    const t = I18n.t(k);
                    return (t && t !== k) ? t : fallback.searching();
                }
            };
        };
        
        for (const selectEl of remoteSelects) {
            const apiPath = selectEl.dataset.apiPath;
            const fieldKey = selectEl.dataset.fieldKey;
            const fieldType = selectEl.dataset.fieldType;
            const relationLabelKey = selectEl.dataset.relationLabelKey || 'name';
            const relationValueKey = selectEl.dataset.relationValueKey || 'id';
            const relationDisplayFormat = selectEl.dataset.relationDisplayFormat || '';
            const relationLabelFields = selectEl.dataset.relationLabelFields ? selectEl.dataset.relationLabelFields.split(',').filter(Boolean) : null;
            const defaultValue = selectEl.dataset.defaultValue || '';
            const isMulti = fieldType === 'select2-multi';
            
            if (!apiPath) continue;
            
            const placeholder = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇';
            
            // 初始化 Select2 with AJAX
            const select2Config = {
                theme: 'bootstrap-5',
                placeholder: placeholder,
                allowClear: true,
                multiple: isMulti,
                closeOnSelect: !isMulti,
                minimumInputLength: 0,
                language: getSelect2Language(),
                dropdownParent: $('#fieldSettingsModal'),
                width: '100%',
                ajax: {
                    url: apiPath.startsWith('/api/v1') ? apiPath : '/api/v1' + apiPath,
                    dataType: 'json',
                    delay: 250,
                    headers: {
                        'Authorization': 'Bearer ' + (localStorage.getItem('auth_token') || ''),
                        'X-Tenant-Subdomain': localStorage.getItem('tenant_subdomain') || ''
                    },
                    data: function (params) {
                        return {
                            search: params.term || '',
                            limit: 50,
                            page: params.page || 1
                        };
                    },
                    processResults: function (data, params) {
                        params.page = params.page || 1;
                        return {
                            results: (data.data || []).map(function(item) {
                                let displayText;
                                if (relationLabelFields && relationLabelFields.length > 0) {
                                    const parts = relationLabelFields.map(k => (item && item[k] != null ? String(item[k]).trim() : '')).filter(Boolean);
                                    if (parts.length >= 2 && String(relationLabelFields[1]) === 'account_number') {
                                        displayText = `${parts[0]} (${parts.slice(1).join(' ')})`;
                                    } else {
                                        displayText = parts.join(' - ');
                                    }
                                } else if (relationDisplayFormat === 'code-name' && item.code && item.name) {
                                    displayText = `${item.code} - ${item.name}`;
                                } else if (relationDisplayFormat === 'employee_number-name' && item.employee_number && item.name) {
                                    displayText = `${item.employee_number} - ${item.name}`;
                                } else {
                                    displayText = item[relationLabelKey] || item.name || item.code || item.employee_number || item.id;
                                }
                                return {
                                    id: item[relationValueKey],
                                    text: displayText
                                };
                            }),
                            pagination: {
                                more: (data.data || []).length === 50 && (data.total || 0) > (params.page || 1) * 50
                            }
                        };
                    },
                    cache: true
                }
            };
            
            $(selectEl).select2(select2Config);
            
            // 如果有預設值，需要加載並設置
            if (defaultValue) {
                try {
                    const defaultValues = isMulti ? defaultValue.split(',').map(v => v.trim()).filter(Boolean) : [defaultValue];
                    const fullApiPath = apiPath.startsWith('/api/v1') ? apiPath : '/api/v1' + apiPath;
                    
                    // 嘗試從 API 獲取預設值的顯示文本
                    const resp = await App.apiRequest(fullApiPath + '?limit=1000');
                    const allItems = resp?.data || [];
                    
                    for (const val of defaultValues) {
                        const item = allItems.find(i => String(i[relationValueKey]) === String(val));
                        if (item) {
                            let displayText;
                            if (relationLabelFields && relationLabelFields.length > 0) {
                                const parts = relationLabelFields.map(k => (item && item[k] != null ? String(item[k]).trim() : '')).filter(Boolean);
                                displayText = parts.join(' - ');
                            } else if (relationDisplayFormat === 'code-name' && item.code && item.name) {
                                displayText = `${item.code} - ${item.name}`;
                            } else {
                                displayText = item[relationLabelKey] || item.name || item.code || item.id;
                            }
                            
                            // 添加選項並選中
                            const option = new Option(displayText, item[relationValueKey], true, true);
                            $(selectEl).append(option);
                        }
                    }
                    $(selectEl).trigger('change');
                } catch (error) {
                    console.warn('載入預設值失敗:', error);
                }
            }
            
            // 監聽變化事件，更新 fieldSettings
            $(selectEl).on('change', (e) => {
                const sf = this.fieldSettings.fields.find(f => f.key === fieldKey);
                if (!sf) return;
                
                let value = '';
                if (isMulti) {
                    const selected = $(selectEl).val() || [];
                    value = selected.join(',');
                } else {
                    value = $(selectEl).val() || '';
                }
                
                if (value === '') {
                    delete sf.defaultValue;
                } else {
                    sf.defaultValue = value;
                }
            });
        }
    }
    
    // 綁定欄位設定 Modal 事件
    bindFieldSettingsEvents(rebindListOnly = false) {
        const listContainer = document.getElementById('fieldSettingsList');
        if (!listContainer) return;
        
        // 定義 getText 輔助函數
        const getText = (key, fallback) => {
            if (typeof I18n !== 'undefined' && I18n.t) {
                const t = I18n.t(key);
                return (t && t !== key) ? t : fallback;
            }
            return fallback;
        };
        
        // Drag & Drop 排序
        let draggedItem = null;
        
        listContainer.querySelectorAll('.field-settings-wrapper').forEach(wrapper => {
            const item = wrapper.querySelector('.field-settings-item');
            if (!item) return;
            
            item.addEventListener('dragstart', (e) => {
                draggedItem = wrapper;
                wrapper.classList.add('dragging');
                e.dataTransfer.effectAllowed = 'move';
            });
            
            item.addEventListener('dragend', () => {
                wrapper.classList.remove('dragging');
                listContainer.querySelectorAll('.field-settings-wrapper').forEach(w => {
                    w.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
                });
                draggedItem = null;
            });
            
            item.addEventListener('dragover', (e) => {
                e.preventDefault();
                e.dataTransfer.dropEffect = 'move';
                
                if (draggedItem && draggedItem !== wrapper) {
                    const rect = item.getBoundingClientRect();
                    const midY = rect.top + rect.height / 2;
                    
                    listContainer.querySelectorAll('.field-settings-wrapper').forEach(w => {
                        w.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
                    });
                    
                    if (e.clientY < midY) {
                        wrapper.classList.add('drag-over-top');
                    } else {
                        wrapper.classList.add('drag-over-bottom');
                    }
                }
            });
            
            item.addEventListener('drop', (e) => {
                e.preventDefault();
                if (draggedItem && draggedItem !== wrapper) {
                    const rect = item.getBoundingClientRect();
                    const midY = rect.top + rect.height / 2;
                    
                    if (e.clientY < midY) {
                        listContainer.insertBefore(draggedItem, wrapper);
                    } else {
                        listContainer.insertBefore(draggedItem, wrapper.nextSibling);
                    }
                }
                
                listContainer.querySelectorAll('.field-settings-wrapper').forEach(w => {
                    w.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
                });
            });

            item.addEventListener('click', (e) => {
                if (e.target.closest('.field-actions') || e.target.closest('.field-drag-handle') || e.target.closest('button') || e.target.closest('input') || e.target.closest('select') || e.target.closest('textarea') || e.target.closest('.select2-container')) {
                    return;
                }
                const defaultBox = wrapper.querySelector('.field-default-value-box');
                if (defaultBox) {
                    const wasHidden = defaultBox.classList.contains('d-none');
                    defaultBox.classList.toggle('d-none');
                    
                    // 如果剛展開且有未初始化的 Select2 remote，初始化它
                    if (wasHidden) {
                        const remoteSelect = defaultBox.querySelector('.field-default-select2-remote');
                        if (remoteSelect && typeof $ !== 'undefined' && !$(remoteSelect).hasClass('select2-hidden-accessible')) {
                            // 延遲一下讓 DOM 更新完成
                            setTimeout(() => {
                                this.initFieldSettingsSelect2Remote();
                            }, 50);
                        }
                    }
                }
            });
        });
        
        // 顯示/隱藏切換 - 使用眼睛圖標按鈕
        listContainer.querySelectorAll('.field-visibility-toggle').forEach(toggle => {
            toggle.addEventListener('click', (e) => {
                const btn = e.currentTarget;
                const fieldKey = btn.dataset.fieldKey;
                const isVisible = btn.dataset.visible === 'true';
                const newVisible = !isVisible;
                
                // 更新設定
                const sf = this.fieldSettings.fields.find(f => f.key === fieldKey);
                if (sf) {
                    sf.visible = newVisible;
                }
                
                // 更新按鈕狀態
                btn.dataset.visible = newVisible ? 'true' : 'false';
                const icon = btn.querySelector('i');
                if (newVisible) {
                    icon.className = 'bi bi-eye text-success';
                } else {
                    icon.className = 'bi bi-eye-slash text-muted';
                }
            });
        });
        
        // 刪除額外欄位
        listContainer.querySelectorAll('.delete-extra-field').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const fieldKey = btn.dataset.key;
                
                // 從 extraFields 中移除
                this.fieldSettings.extraFields = this.fieldSettings.extraFields.filter(ef => ef.key !== fieldKey);
                
                // 從 fields 設定中移除
                this.fieldSettings.fields = this.fieldSettings.fields.filter(f => f.key !== fieldKey);
                
                // 標記 dirty，防止背景 loadFieldSettingsFromAPI 覆蓋
                this._fieldSettingsDirty = true;
                
                // 從 DOM 中移除 - 需要移除整個 wrapper
                btn.closest('.field-settings-wrapper').remove();
            });
        });

        // 預設值設定 — 同時綁定 change 和 input 事件
        // change: 適用於 select、checkbox（以及 blur 時的 text）
        // input: 適用於 text/number/textarea，每次按鍵即時同步
        const _syncDefaultValue = (e) => {
            const el = e.currentTarget;
            const fieldKey = el.dataset.fieldKey;
            const sf = this.fieldSettings.fields.find(f => f.key === fieldKey);
            if (!sf) return;

            let value = '';
            if (el.type === 'checkbox') {
                value = el.checked ? 'true' : 'false';
            } else if (el.tagName === 'SELECT' && el.multiple) {
                const selected = Array.from(el.selectedOptions)
                    .map(o => String(o.value))
                    .filter(v => v !== '');
                value = selected.join(',');
            } else {
                value = (el.value || '').trim();
            }

            if (value === '') {
                delete sf.defaultValue;
            } else {
                sf.defaultValue = value;
            }
        };
        listContainer.querySelectorAll('.field-default-input').forEach(input => {
            input.addEventListener('change', _syncDefaultValue);
            // 對 text/number/textarea 加上 input 事件，即時同步
            if (input.tagName !== 'SELECT' && input.type !== 'checkbox') {
                input.addEventListener('input', _syncDefaultValue);
            }
        });

        // 圖片上傳預設值
        listContainer.querySelectorAll('.field-default-image-upload').forEach(container => {
            const fieldKey = container.dataset.fieldKey;
            const fileInput = container.querySelector('.field-default-image-file');
            const hiddenInput = container.querySelector('.field-default-input');
            const uploadBtn = container.querySelector('.field-default-image-btn');
            const clearBtn = container.querySelector('.field-default-image-clear');
            const previewContainer = container.querySelector('.field-default-image-preview');
            const previewImg = previewContainer?.querySelector('img');

            if (uploadBtn && fileInput) {
                uploadBtn.addEventListener('click', () => {
                    fileInput.click();
                });

                fileInput.addEventListener('change', async (e) => {
                    const file = e.target.files[0];
                    if (!file) return;

                    // 顯示上傳中狀態
                    uploadBtn.disabled = true;
                    uploadBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span>' + getText('common.uploading', '上傳中...');

                    try {
                        // 上傳圖片
                        const formData = new FormData();
                        formData.append('file', file);

                        const response = await fetch('/api/v1/upload', {
                            method: 'POST',
                            headers: {
                                'Authorization': 'Bearer ' + (localStorage.getItem('auth_token') || ''),
                                'X-Tenant-Subdomain': localStorage.getItem('tenant_subdomain') || ''
                            },
                            body: formData
                        });

                        if (!response.ok) {
                            throw new Error(getText('common.uploadFailed', '上傳失敗'));
                        }

                        const result = await response.json();
                        const imageUrl = result.url || result.file_url || '';

                        if (imageUrl) {
                            // 更新預覽
                            if (previewImg) {
                                previewImg.src = imageUrl;
                                previewContainer.style.display = 'flex';
                            }
                            // 更新隱藏輸入
                            hiddenInput.value = imageUrl;
                            hiddenInput.dispatchEvent(new Event('change', { bubbles: true }));
                            // 顯示清除按鈕
                            if (clearBtn) {
                                clearBtn.style.display = '';
                            }
                        }
                    } catch (error) {
                        console.error('Upload image failed:', error);
                        App.showError(getText('common.uploadImageFailed', '上傳圖片失敗'));
                    } finally {
                        uploadBtn.disabled = false;
                        uploadBtn.innerHTML = '<i class="bi bi-upload me-1"></i>' + getText('common.uploadImage', '上傳圖片');
                        fileInput.value = '';
                    }
                });
            }

            if (clearBtn) {
                clearBtn.addEventListener('click', () => {
                    // 清除預覽
                    if (previewImg) {
                        previewImg.src = '';
                        previewContainer.style.display = 'none';
                    }
                    // 清除值
                    hiddenInput.value = '';
                    hiddenInput.dispatchEvent(new Event('change', { bubbles: true }));
                    // 隱藏清除按鈕
                    clearBtn.style.display = 'none';
                });
            }
        });
        
        // 初始化 Select2 for options 和類型變更事件
        const typeSelect = document.getElementById('extraFieldType');
        const optionsRow = document.getElementById('selectOptionsRow');
        const optionsSelect = document.getElementById('extraFieldOptions');
        
        if (typeSelect && optionsRow && optionsSelect) {
            try {
                // 初始化 Select2 tags 模式
                if (typeof $ !== 'undefined' && $.fn.select2) {
                    $(optionsSelect).select2({
                        tags: true,
                        tokenSeparators: [',', '\n'],
                        placeholder: getText('common.fieldTypeSelectOptions', '選項（用逗號分隔或按 Enter）'),
                        allowClear: true,
                        width: '100%'
                    });
                } else {
                    console.warn('[DynamicForm] jQuery or Select2 not loaded, falling back to simple select');
                }
                
                // 類型變更時顯示/隱藏選項輸入
                typeSelect.addEventListener('change', () => {
                    if (typeSelect.value === 'select') {
                        optionsRow.style.display = 'block';
                    } else {
                        optionsRow.style.display = 'none';
                        // 清空選項
                        if (typeof $ !== 'undefined' && $.fn.select2) {
                            $(optionsSelect).val(null).trigger('change');
                        } else {
                            optionsSelect.value = '';
                        }
                    }
                });
            } catch (e) {
                console.error('[DynamicForm] Select2 initialization failed:', e);
            }
        }
        
        // ── 以下是 Add / Save 按鈕綁定，只在初次呼叫時執行 ──
        // rebindListOnly=true 時跳過，避免重複綁定導致多次 API 呼叫
        console.log('[DynamicForm][DEBUG] bindFieldSettingsEvents rebindListOnly=', rebindListOnly);
        if (rebindListOnly) return;
        
        // 添加額外欄位 — 使用 onclick 覆蓋，避免事件疊加
        const addBtn = document.getElementById('addExtraFieldBtn');
        console.log('[DynamicForm][DEBUG] addBtn found:', !!addBtn);
        if (addBtn) {
            addBtn.onclick = () => {
                const keyInput = document.getElementById('extraFieldKey');
                const labelInput = document.getElementById('extraFieldLabel');
                const typeSelectEl = document.getElementById('extraFieldType');
                const optionsSelectEl = document.getElementById('extraFieldOptions');
                
                const key = keyInput.value.trim();
                const label = labelInput.value.trim();
                const type = typeSelectEl.value;
                
                if (!key) {
                    App.showError('請輸入欄位 Key');
                    return;
                }
                
                // 檢查 key 是否已存在
                const exists = this.fieldSettings.fields.some(f => f.key === key);
                if (exists) {
                    App.showError('欄位 Key 已存在');
                    return;
                }
                
                // 添加到 extraFields
                const newField = {
                    key: key,
                    label: label || key,
                    type: type,
                    required: false,
                    isExtra: true
                };
                
                // 如果是 select 類型，添加選項
                if (type === 'select' && optionsSelectEl) {
                    const options = $(optionsSelectEl).val() || [];
                    newField.options = options;
                }
                
                if (!this.fieldSettings.extraFields) {
                    this.fieldSettings.extraFields = [];
                }
                this.fieldSettings.extraFields.push(newField);
                // 標記 dirty，防止背景 loadFieldSettingsFromAPI 覆蓋
                this._fieldSettingsDirty = true;
                console.log('[DynamicForm][ADD] extraField pushed, key=', key, 'extraFields count=', this.fieldSettings.extraFields.length, '_fieldSettingsDirty=true');
                
                // 添加到 fields 設定
                const maxOrder = Math.max(...this.fieldSettings.fields.map(f => f.order), -1);
                this.fieldSettings.fields.push({
                    key: key,
                    visible: true,
                    order: maxOrder + 1
                });
                
                // 生成類型 badge
                let typeBadge = '';
                const typeMap = {
                    text: getText('common.fieldTypeText', '文字'),
                    number: getText('common.fieldTypeNumber', '數字'),
                    date: getText('common.fieldTypeDate', '日期'),
                    textarea: getText('common.fieldTypeTextarea', '多行文字'),
                    email: getText('common.fieldTypeEmail', '郵箱'),
                    select: getText('common.fieldTypeSelect', '下拉選單')
                };
                const typeBadgeLabel = typeMap[type] || type;
                if (type === 'select') {
                    const optionCount = newField.options ? newField.options.length : 0;
                    typeBadge = `<span class="badge bg-secondary ms-1">${typeBadgeLabel} (${optionCount})</span>`;
                } else {
                    typeBadge = `<span class="badge bg-secondary ms-2">${typeBadgeLabel}</span>`;
                }
                
                // 生成預設值 input HTML（根據欄位類型）
                let defaultValueInputHtml = '';
                if (type === 'number') {
                    defaultValueInputHtml = `<input type="number" class="form-control form-control-sm field-default-input" data-field-key="${key}" value="">`;
                } else if (type === 'date') {
                    defaultValueInputHtml = `<input type="date" class="form-control form-control-sm field-default-input" data-field-key="${key}" value="">`;
                } else if (type === 'textarea') {
                    defaultValueInputHtml = `<textarea class="form-control form-control-sm field-default-input" data-field-key="${key}" rows="2"></textarea>`;
                } else if (type === 'select' && newField.options && newField.options.length > 0) {
                    const placeholder = getText('common.pleaseSelect', '請選擇');
                    const optionsHtml = newField.options.map(opt => `<option value="${opt}">${opt}</option>`).join('');
                    defaultValueInputHtml = `<select class="form-select form-select-sm field-default-input" data-field-key="${key}"><option value="">${placeholder}</option>${optionsHtml}</select>`;
                } else {
                    defaultValueInputHtml = `<input type="text" class="form-control form-control-sm field-default-input" data-field-key="${key}" value="">`;
                }
                
                // 添加到列表 DOM - 使用 wrapper 結構
                const newItemHtml = `
                    <div class="field-settings-wrapper" data-field-key="${key}">
                        <div class="field-settings-item" draggable="true">
                            <div class="field-drag-handle">
                                <i class="bi bi-grip-vertical"></i>
                            </div>
                            <div class="field-info">
                                <span class="field-label">${label || key}</span>
                                <span class="field-key text-muted">(${key})</span>
                                <span class="badge bg-info ms-2">額外</span>
                                ${typeBadge}
                            </div>
                            <div class="field-actions">
                                <button type="button" class="btn btn-sm btn-link field-visibility-toggle p-0" data-visible="true" data-field-key="${key}">
                                    <i class="bi bi-eye text-success" style="font-size: 1.2rem;"></i>
                                </button>
                                <button type="button" class="btn btn-sm btn-outline-danger delete-extra-field ms-2" data-key="${key}"><i class="bi bi-trash"></i></button>
                            </div>
                        </div>
                        <div class="field-default-value-box d-none" style="background: #f8f9fa; border: 1px solid #dee2e6; border-top: none; border-radius: 0 0 6px 6px; padding: 12px 16px; margin-bottom: 8px;">
                            <label class="form-label small text-muted mb-2"><i class="bi bi-pencil-square me-1"></i>${getText('common.defaultValue', '預設值')}</label>
                            ${defaultValueInputHtml}
                        </div>
                    </div>
                `;
                
                document.getElementById('fieldSettingsList').insertAdjacentHTML('beforeend', newItemHtml);
                
                // 清空輸入
                keyInput.value = '';
                labelInput.value = '';
                typeSelectEl.selectedIndex = 0;
                if (optionsSelectEl && typeof $ !== 'undefined' && $.fn.select2) {
                    $(optionsSelectEl).val(null).trigger('change');
                } else if (optionsSelectEl) {
                    optionsSelectEl.value = '';
                }
                document.getElementById('selectOptionsRow').style.display = 'none';
                
                // 重新綁定列表項目事件（不重新綁定 Add/Save 按鈕）
                this.bindFieldSettingsEvents(true);
            };
        }
        



        
        // 保存按鈕
        const saveBtn = document.getElementById('saveFieldSettingsBtn');
        console.log('[DynamicForm][DEBUG] saveBtn found:', !!saveBtn);
        if (saveBtn) {
            // 使用 onclick 覆蓋，避免事件疊加
            saveBtn.onclick = async () => {
                // Check if user has typed in extra field key but forgot to press +
                const extraFieldKeyInput = document.getElementById('extraFieldKey');
                if (extraFieldKeyInput && extraFieldKeyInput.value.trim()) {
                    const confirmSave = confirm(getText('common.extraFieldNotAdded', '你在「添加額外欄位」中已輸入內容但尚未按「+」新增，是否仍要保存？'));
                    if (!confirmSave) return;
                }

                console.log('[DynamicForm][SAVE] Save 按鈕被點擊');
                console.log('[DynamicForm][SAVE] this.fieldSettings.extraFields:', JSON.stringify(this.fieldSettings.extraFields || []));
                console.log('[DynamicForm][SAVE] this.fieldSettings.extraFields count:', (this.fieldSettings.extraFields || []).length);
                console.log('[DynamicForm][SAVE] this._fieldSettingsDirty:', this._fieldSettingsDirty);
                console.log('[DynamicForm][SAVE] this === window.dynamicForm:', this === window.dynamicForm);
                console.log('[DynamicForm][SAVE] fieldSettings ref id:', this.fieldSettings._debugId || 'none');
                console.log('[DynamicForm][SAVE] window._debugExtraFieldsRef same?:', this.fieldSettings.extraFields === window._debugExtraFieldsRef);
                console.log('[DynamicForm][SAVE] window._debugFieldSettingsRef same?:', this.fieldSettings === window._debugFieldSettingsRef);
                if (window._debugDynamicFormRef && window._debugDynamicFormRef !== this) {
                    console.warn('[DynamicForm][SAVE] WARNING: this !== _debugDynamicFormRef! ADD 和 SAVE 用了不同的 instance!');
                    console.log('[DynamicForm][SAVE] _debugDynamicFormRef.fieldSettings.extraFields:', JSON.stringify(window._debugDynamicFormRef.fieldSettings.extraFields || []));
                }
                
                // 更新順序 - 使用 wrapper 來獲取 fieldKey
                const listContainer = document.getElementById('fieldSettingsList');
                const wrappers = listContainer.querySelectorAll('.field-settings-wrapper');
                wrappers.forEach((wrapper, idx) => {
                    const fieldKey = wrapper.dataset.fieldKey;
                    const sf = this.fieldSettings.fields.find(f => f.key === fieldKey);
                    if (sf) {
                        sf.order = idx;
                    }
                });

                // 保存前主動從 DOM 收集所有 defaultValue（避免 change 事件未觸發導致遺漏）
                listContainer.querySelectorAll('.field-default-input').forEach(el => {
                    const fieldKey = el.dataset.fieldKey;
                    if (!fieldKey) return;
                    const sf = this.fieldSettings.fields.find(f => f.key === fieldKey);
                    if (!sf) return;

                    let value = '';
                    if (el.type === 'checkbox') {
                        value = el.checked ? 'true' : 'false';
                    } else if (el.tagName === 'SELECT' && el.multiple) {
                        // 支援 Select2 multiple
                        if (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) {
                            const vals = $(el).val();
                            value = Array.isArray(vals) ? vals.filter(v => v !== '').join(',') : '';
                        } else {
                            value = Array.from(el.selectedOptions).map(o => String(o.value)).filter(v => v !== '').join(',');
                        }
                    } else if (el.tagName === 'SELECT') {
                        // 支援 Select2 single
                        if (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) {
                            value = ($(el).val() || '').toString().trim();
                        } else {
                            value = (el.value || '').trim();
                        }
                    } else {
                        value = (el.value || '').trim();
                    }

                    if (value === '') {
                        delete sf.defaultValue;
                    } else {
                        sf.defaultValue = value;
                    }
                });
                
                // 禁用按鈕防止重複點擊
                saveBtn.disabled = true;
                saveBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span>保存中...';
                
                try {
                    // 保存設定到 API
                    await this.saveFieldSettings();
                    
                    // 關閉 Modal
                    const modal = bootstrap.Modal.getInstance(document.getElementById('fieldSettingsModal'));
                    if (modal) {
                        modal.hide();
                    }
                    
                    // Save 後不重新渲染表單
                    if (typeof FormFieldSettingsHandler !== 'undefined' && FormFieldSettingsHandler.applyFieldSettingsToDOM) {
                        FormFieldSettingsHandler.applyFieldSettingsToDOM(this, { skipDefaults: true });
                    }

                    
                    // ── 動態注入/移除額外欄位 DOM 元素 ──
                    // 保存後，需要在表單上新增或移除 extra fields，不然用戶看不到新欄位
                    if (this.fieldSettings) {
                        const baseFieldKeys = new Set(this.config.formFields.map(f => f.key));
                        const currentExtraFields = (this.fieldSettings.extraFields || []).filter(ef => !baseFieldKeys.has(ef.key));
                        const currentExtraKeys = new Set(currentExtraFields.map(ef => ef.key));
                        
                        // Build field order map for visibility check
                        const fieldOrderMap = {};
                        if (this.fieldSettings.fields) {
                            this.fieldSettings.fields.forEach(sf => { fieldOrderMap[sf.key] = sf; });
                        }
                        
                        // 1) Remove DOM elements for extra fields that were deleted
                        const existingExtraContainers = document.querySelectorAll('#dynamicForm [data-extra-field-key]');
                        existingExtraContainers.forEach(container => {
                            const key = container.getAttribute('data-extra-field-key');
                            if (!currentExtraKeys.has(key)) {
                                container.remove();
                                console.log(`[DynamicForm] 已移除被刪除的額外欄位 DOM: ${key}`);
                            }
                        });
                        
                        // 2) Inject DOM elements for newly added extra fields
                        const self = this;
                        const selectFieldsToInit = [];
                        currentExtraFields.forEach(ef => {
                            const sfSetting = fieldOrderMap[ef.key];
                            if (sfSetting && sfSetting.visible === false) return;
                            
                            const fieldId = `field_${ef.key}`;
                            if (document.getElementById(fieldId)) return; // Already in DOM
                            
                            const fieldHtml = self.renderField(ef);
                            if (!fieldHtml) return;
                            
                            const buttonsContainer = document.querySelector('#dynamicForm .form-buttons-container');
                            if (buttonsContainer) {
                                const wrapper = document.createElement('div');
                                wrapper.setAttribute('data-extra-field-key', ef.key);
                                wrapper.innerHTML = fieldHtml;
                                buttonsContainer.parentNode.insertBefore(wrapper, buttonsContainer);
                                console.log(`[DynamicForm] 動態注入新額外欄位: ${ef.key}`);
                                
                                if (ef.type === 'select' || ef.type === 'select2') {
                                    selectFieldsToInit.push(ef);
                                }
                            }
                        });
                        
                        // 3) Re-initialize Select2 for newly injected select fields
                        if (selectFieldsToInit.length > 0) {
                            setTimeout(async () => {
                                for (const ef of selectFieldsToInit) {
                                    const fieldId = `field_${ef.key}`;
                                    const input = document.getElementById(fieldId);
                                    if (input && typeof self.initSelect2 === 'function') {
                                        try {
                                            await self.initSelect2(ef);
                                        } catch (e) {
                                            console.warn(`[DynamicForm] initSelect2 failed for extra field ${ef.key}:`, e);
                                        }
                                    }
                                }
                            }, 100);
                        }
                    }
                    
                    App.showSuccess('欄位設定已保存');
                } catch (e) {
                    App.showError('保存欄位設定失敗');
                } finally {
                    saveBtn.disabled = false;
                    saveBtn.innerHTML = '<i class="bi bi-save me-1"></i>保存';
                }
            };
        }
    }

    // ── 判斷字段是否為圖片上傳類型 ──
    // 返回 true 如果該字段使用 Modern Path A 渲染（帶 _uploadBtn/_removeBtn/_preview）
    _isImageUploadField(field) {
        if (field.type === 'profile-image') return true;
        if (field.type !== 'file') return false;
        // image_url on supported pages
        if (field.key === 'image_url' && ['products', 'vehicles', 'equipments', 'services', 'projects', 'stores', 'rooms', 'brands', 'product-types'].includes(this.pageName)) return true;
        // logo_url on brands
        if (field.key === 'logo_url' && this.pageName === 'brands') return true;
        // featured_image on blogs
        if (field.key === 'featured_image' && this.pageName === 'blogs') return true;
        return false;
    }

    // ── 取得圖片裁剪比例 ──
    _getImageAspectRatio(fieldKey) {
        // projects 的 image_url 和 blogs 的 featured_image 使用 16:9
        if ((this.pageName === 'projects' && fieldKey === 'image_url') ||
            (this.pageName === 'blogs' && fieldKey === 'featured_image')) {
            return 16 / 9;
        }
        // 其餘全部 1:1（profile-image、products/vehicles/equipments/stores/rooms/brands/product-types 的 image_url、brands 的 logo_url）
        return 1;
    }

    // ── 事件委託：統一處理所有 upload/remove 按鈕和 file input change ──
    // 綁定在 document.body（或 .main-content）上，不依賴 setTimeout，
    // 無論 SPA Router 何時替換 DOM 都能正常工作。
    _setupImageUploadDelegation() {
        const container = document.querySelector('.main-content') || document.body;

        // 避免重複綁定（如果 Router 重用了同一個容器）
        if (container._imageUploadDelegated) return;
        container._imageUploadDelegated = true;

        // ── 1) Upload 按鈕：點擊觸發對應 file input ──
        container.addEventListener('click', (e) => {
            const uploadBtn = e.target.closest('[id$="_uploadBtn"]');
            if (!uploadBtn) return;
            e.preventDefault();
            // field_xxx_uploadBtn → field_xxx
            const fieldId = uploadBtn.id.replace('_uploadBtn', '');
            const fileInput = document.getElementById(fieldId);
            if (fileInput) fileInput.click();
        });

        // ── 2) Remove 按鈕：清除圖片 ──
        container.addEventListener('click', (e) => {
            const removeBtn = e.target.closest('[id$="_removeBtn"]');
            if (!removeBtn) return;
            e.preventDefault();
            const fieldId = removeBtn.id.replace('_removeBtn', '');
            const fileInput = document.getElementById(fieldId);
            const urlInput = document.getElementById(`${fieldId}_url`);
            const preview = document.getElementById(`${fieldId}_preview`);
            const placeholder = document.getElementById(`${fieldId}_placeholder`);

            if (fileInput) fileInput.value = '';
            if (urlInput) {
                urlInput.value = '';
                urlInput.dispatchEvent(new Event('change', { bubbles: true }));
            }
            if (preview) {
                preview.src = '';
                preview.style.display = 'none';
            }
            if (placeholder) {
                placeholder.style.display = 'block';
            }
            removeBtn.style.display = 'none';

            // 觸發自動保存草稿
            if (window.dynamicForm && window.dynamicForm.saveDraft && !window.dynamicForm.isEdit) {
                window.dynamicForm.hasUnsavedChanges = true;
                setTimeout(() => { window.dynamicForm.saveDraft(); }, 100);
            }
        });

        // ── 3) File input change：裁剪 → 上傳 → 預覽 ──
        container.addEventListener('change', (e) => {
            const fileInput = e.target;
            // 只處理 hidden file input 且 id 以 field_ 開頭（排除普通 file input）
            if (fileInput.tagName !== 'INPUT' || fileInput.type !== 'file') return;
            if (!fileInput.id || !fileInput.id.startsWith('field_')) return;
            // 排除非圖片的 file input（attachment 等）— 只處理有 _uploadBtn 兄弟的
            const fieldId = fileInput.id;
            const uploadBtn = document.getElementById(`${fieldId}_uploadBtn`);
            if (!uploadBtn) return; // 不是 modern image upload field

            const file = fileInput.files[0];
            if (!file) return;

            // 驗證文件類型
            if (!file.type.startsWith('image/')) {
                App.showAlert('請選擇圖片文件', 'warning');
                fileInput.value = '';
                return;
            }
            // 驗證文件大小（最大 10MB）
            if (file.size > 10 * 1024 * 1024) {
                App.showAlert('圖片大小不能超過 10MB', 'warning');
                fileInput.value = '';
                return;
            }

            // 確保 ImageCropper 已載入
            if (typeof ImageCropper === 'undefined') {
                console.error('ImageCropper is not defined');
                App.showAlert('圖片裁剪功能未載入，請刷新頁面重試', 'warning');
                fileInput.value = '';
                return;
            }

            // 從 fieldId 推算 field key（field_image_url → image_url）
            const fieldKey = fieldId.replace(/^field_/, '');
            const dynForm = window.dynamicForm;
            const aspectRatio = dynForm ? dynForm._getImageAspectRatio(fieldKey) : 1;
            const isProfile = fileInput.closest('.profile-pic-preview') !== null ||
                              document.getElementById(`${fieldId}_placeholder`)?.classList.contains('bi-person');

            try {
                const cropper = new ImageCropper({
                    aspectRatio: aspectRatio,
                    viewMode: 1
                });

                cropper.showCropModal(file, async (croppedBlob) => {
                    try {
                        const formData = new FormData();
                        formData.append('file', croppedBlob, 'cropped.jpg');

                        const token = localStorage.getItem('auth_token');
                        const uploadResponse = await fetch('/api/v1/upload', {
                            method: 'POST',
                            headers: { 'Authorization': 'Bearer ' + token },
                            body: formData
                        });

                        if (!uploadResponse.ok) {
                            const errorData = await uploadResponse.json();
                            throw new Error(errorData.error || '上傳失敗');
                        }

                        const uploadData = await uploadResponse.json();
                        let imageUrl = uploadData.url;
                        if (imageUrl && !imageUrl.startsWith('http') && !imageUrl.startsWith('/')) {
                            imageUrl = '/' + imageUrl;
                        }

                        // 更新隱藏字段
                        const urlInput = document.getElementById(`${fieldId}_url`);
                        if (urlInput) urlInput.value = imageUrl;

                        // 更新預覽
                        const preview = document.getElementById(`${fieldId}_preview`);
                        const placeholder = document.getElementById(`${fieldId}_placeholder`);
                        const removeBtn = document.getElementById(`${fieldId}_removeBtn`);

                        if (preview) {
                            preview.src = imageUrl;
                            preview.style.display = 'block';
                            preview.style.cursor = 'pointer';
                            preview.onclick = () => {
                                if (dynForm) dynForm.showImageLightbox(imageUrl);
                            };
                            preview.onerror = function () {
                                this.style.display = 'none';
                                if (placeholder) placeholder.style.display = 'block';
                                if (removeBtn) removeBtn.style.display = 'none';
                                console.warn('Failed to load image:', imageUrl);
                            };
                        }
                        if (placeholder) placeholder.style.display = 'none';
                        if (removeBtn) removeBtn.style.display = 'inline-block';

                        App.showAlert(isProfile ? '頭像上傳成功' : '圖片上傳成功', 'success');
                    } catch (error) {
                        console.error('Upload error:', error);
                        App.showAlert((isProfile ? '頭像' : '圖片') + '上傳失敗: ' + error.message, 'danger');
                    } finally {
                        fileInput.value = '';
                    }
                }, {
                    onCancel: () => { fileInput.value = ''; }
                });
            } catch (error) {
                console.error('Error showing crop modal:', error);
                App.showAlert('顯示裁剪窗口失敗: ' + error.message, 'danger');
                fileInput.value = '';
            }
        });
    }

    bindEvents() {
        // ── 事件委託：upload/remove 按鈕和 file input ──
        // 使用事件委託（delegation）綁定在容器上，避免 SPA 導航時 setTimeout timing 問題
        // 這些 handler 不依賴 setTimeout，無論 DOM 何時渲染都能正常工作
        this._setupImageUploadDelegation();
        
        // 使用 setTimeout 確保 DOM 已更新
        setTimeout(() => {
            const form = document.getElementById('dynamicForm');
            if (!form) {
                console.error('找不到 dynamicForm 表單元素');
                return;
            }

            // 移除 onsubmit="return false;" 屬性，避免阻止事件
            form.removeAttribute('onsubmit');

            form.addEventListener('submit', async (e) => {
                e.preventDefault();
                try {
                    await this.submitForm();
                } catch (error) {
                    // 錯誤已在 submitForm 中處理，這裡只是防止未捕獲的錯誤
                    console.error('表單提交錯誤:', error);
                }
            });
            
            // Payment methods 特殊逻辑：根据付款形式动态调整字段
            if (this.pageName === 'payment-methods') {
                const paymentTypeField = document.getElementById('field_payment_type');
                if (paymentTypeField) {
                    // 初始化
                    this.handlePaymentTypeChange();
                    
                    // 监听变化
                    paymentTypeField.addEventListener('change', () => {
                        this.handlePaymentTypeChange();
                    });
                }
            }
        }, 0);
        
        // 初始化所有 readonly 和 disabled 欄位的 tooltip
        setTimeout(() => {
            this.initTooltips();
        }, 200);
        
        // Profile image 字段處理 - 移動端按鈕文字優化 + 預覽初始化
        setTimeout(() => {
            // 移動端按鈕文字優化
            const updateProfileImageButtonText = () => {
                if (window.innerWidth <= 768) {
                    this.config.formFields.forEach(field => {
                        if (field.type === 'profile-image') {
                            const fieldId = `field_${field.key}`;
                            const uploadBtn = document.getElementById(`${fieldId}_uploadBtn`);
                            const removeBtn = document.getElementById(`${fieldId}_removeBtn`);
                            
                            if (uploadBtn) {
                                const uploadText = uploadBtn.textContent.trim();
                                if (uploadText.includes('頭像')) {
                                    uploadBtn.innerHTML = uploadBtn.innerHTML.replace('上傳頭像', '上傳').replace('上传头像', '上传');
                                }
                            }
                            
                            if (removeBtn) {
                                const removeText = removeBtn.textContent.trim();
                                if (removeText.includes('頭像')) {
                                    removeBtn.innerHTML = removeBtn.innerHTML.replace('移除頭像', '移除').replace('移除头像', '移除');
                                }
                            }
                        }
                    });
                }
            };
            
            updateProfileImageButtonText();
            window.addEventListener('resize', updateProfileImageButtonText);
            
            // 初始化已有圖片的預覽（image_url/logo_url/featured_image）
            this.config.formFields.forEach(field => {
                if (this._isImageUploadField(field)) {
                    const fieldId = `field_${field.key}`;
                    const urlInput = document.getElementById(`${fieldId}_url`);
                    // 如果有現有圖片 URL，顯示預覽
                    if (urlInput && urlInput.value) {
                        const currentPreview = document.getElementById(`${fieldId}_preview`);
                        const currentPlaceholder = document.getElementById(`${fieldId}_placeholder`);
                        const currentRemoveBtn = document.getElementById(`${fieldId}_removeBtn`);
                        
                        if (currentPreview && urlInput.value) {
                            // 確保 URL 是完整的路徑
                            let imageUrl = urlInput.value;
                            if (!imageUrl.startsWith('http') && !imageUrl.startsWith('/')) {
                                imageUrl = '/' + imageUrl;
                            }
                            currentPreview.src = imageUrl;
                            currentPreview.style.display = 'block';
                            // 添加點擊事件以放大圖片
                            currentPreview.style.cursor = 'pointer';
                            currentPreview.onclick = () => {
                                this.showImageLightbox(imageUrl);
                            };
                            if (currentPlaceholder) {
                                currentPlaceholder.style.display = 'none';
                            }
                            if (currentRemoveBtn) {
                                currentRemoveBtn.style.display = 'inline-block';
                            }
                        }
                    }
                }
            });
        }, 200);

        // 取消按鈕事件（需要確認）
        const cancelBtn = document.getElementById('cancelBtn');
        if (cancelBtn) {
            cancelBtn.addEventListener('click', (e) => {
                e.preventDefault();
                this.handleCancelWithConfirm(e);
            });
        }
        // 返回列表按鈕事件（不需要確認）
        const backBtn = document.getElementById('backToListBtn');
        if (backBtn) {
            backBtn.addEventListener('click', (e) => {
                e.preventDefault();
                this.handleCancel(e);
            });
        }

        // 監聽表單變化
        const form = document.getElementById('dynamicForm');
        if (form) {
            form.addEventListener('input', () => {
                this.hasUnsavedChanges = true;
            });
            form.addEventListener('change', () => {
                this.hasUnsavedChanges = true;
            });
        }
    }

    handleCancel(event) {
        if (event) event.preventDefault();
        // 返回列表時不再詢問，直接返回（草稿會自動保存）
        const listPath = this.getListPathWithParams();
        if (typeof Router !== 'undefined' && Router.go) {
            Router.go(listPath);
        } else {
            window.location.href = listPath;
        }
        return false;
    }

    handleCancelWithConfirm(event) {
        if (event) event.preventDefault();
        // 取消按鈕需要確認
        const listPath = this.getListPathWithParams();
        if (this.hasUnsavedChanges || this.currentDraftId) {
            const confirmText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.unsavedChanges') : '您有未保存的更改，是否捨棄草稿並返回？';
            if (confirm(confirmText)) {
                // 刪除當前草稿
                if (this.currentDraftId && this.pageName) {
                    draftManager.deleteDraft(this.pageName, this.currentDraftId);
                }
                if (typeof Router !== 'undefined' && Router.go) {
                    Router.go(listPath);
                } else {
                    window.location.href = listPath;
                }
            }
        } else {
            if (typeof Router !== 'undefined' && Router.go) {
                Router.go(listPath);
            } else {
                window.location.href = listPath;
            }
        }
        return false;
    }

    // 獲取帶有來源篩選參數的列表路徑
    getListPathWithParams() {
        const basePath = this.config.listPath;
        if (!this.referrerQueryParams) return basePath;
        const separator = basePath.includes('?') ? '&' : '?';
        return `${basePath}${separator}${this.referrerQueryParams}`;
    }

    // 更新 URL 的 draft_id 參數（不刷新頁面）
    updateURLWithDraftId(draftId) {
        if (!draftId) return;
        
        const url = new URL(window.location.href);
        url.searchParams.set('draft_id', draftId);
        
        // 使用 replaceState 更新 URL，不刷新頁面
        window.history.replaceState({}, '', url.toString());
    }

    setupAutoSave() {
        if (this.isEdit) return; // 編輯模式不自動保存草稿

        const form = document.getElementById('dynamicForm');
        if (!form) return;

        const saveInterval = 2000; // 2秒後保存

        const autoSave = () => {
            // 初始化期間忽略（預設值設置會觸發 change，不應視為用戶輸入）
            if (this._formInitializing) return;

            // 只要有輸入就標記為有未保存的更改
            this.hasUnsavedChanges = true;

            clearTimeout(this.saveTimer);
            this.saveTimer = setTimeout(() => {
                this.saveDraft();
            }, saveInterval);
        };

        form.addEventListener('input', autoSave);
        form.addEventListener('change', autoSave);
        
        // 監聽 Select2 變化
        this.config.formFields.forEach(field => {
            if (field.type === 'select2' || field.type === 'select2-multi') {
                const fieldId = `field_${field.key}`;
                const input = document.getElementById(fieldId);
                if (input && typeof $ !== 'undefined') {
                    // 等待 Select2 初始化完成
                    setTimeout(() => {
                        if ($(input).hasClass('select2-hidden-accessible')) {
                            $(input).on('change', autoSave);
                        }
                    }, 500);
                }
            }
        });
        
        // 監聽子表格變化（如產品屬性）
        if (this.pageName === 'products') {
            const attributesList = document.getElementById('productAttributesList');
            if (attributesList) {
                attributesList.addEventListener('change', autoSave);
                attributesList.addEventListener('input', autoSave);
            }
        }
    }

    saveDraft() {
        if (!this.pageName || this.isEdit) return;
        
        // 初始化期間不保存草稿（避免預設值觸發 change 導致假草稿）
        if (this._formInitializing) return;
        
        // 檢查是否已禁用自動保存（例如正在提交表單時）
        if (this.autoSaveDisabled) {
            return; // 如果自動保存已禁用，直接返回
        }

        const formData = this.collectFormData();
        if (!formData) return;
        
        // 如果是產品表單，收集產品屬性數據
        if (this.pageName === 'products') {
            const attributes = this.collectProductAttributes();
            formData.product_attributes = attributes;
        }
        
        // 如果是客戶表單，收集地址數據
        if (this.pageName === 'customers' && this.customerAddresses) {
            formData.customer_addresses = this.customerAddresses;
        }
        
        // 檢查是否有任何非空值（只要有輸入就保存）
        // 注意：customer_addresses 數組即使為空也應該保存（因為用戶可能刪除了所有地址）
        const hasData = Object.keys(formData).some(key => {
            const value = formData[key];
            if (value === null || value === undefined) return false;
            if (typeof value === 'string' && value.trim() === '') return false;
            // 地址數組特殊處理：即使為空也認為有數據（因為用戶可能刪除了所有地址）
            if (key === 'customer_addresses' && Array.isArray(value)) return true;
            if (Array.isArray(value) && value.length === 0) return false;
            return true;
        });
        
        if (!hasData) return;

        // 找出 key field（編號類字段）
        const keyField = this.config.formFields.find(f => 
            f.key === 'order_number' || 
            f.key === 'code' || 
            f.key === 'invoice_number' ||
            f.key === 'sale_number' ||
            (f.key && f.key.includes('number') && f.readonly)
        )?.key;

        // 如果有 currentDraftId，使用它來更新現有草稿；否則創建新草稿
        const draftId = draftManager.saveDraft(this.pageName, formData, keyField, this.currentDraftId);
        if (draftId) {
            const isNewDraft = !this.currentDraftId;
            this.currentDraftId = draftId;
            this.hasUnsavedChanges = false;
            this.updateDraftIndicator();
            
            // 如果是新草稿，更新 URL 的 draft_id 參數（不刷新頁面）
            if (isNewDraft) {
                this.updateURLWithDraftId(draftId);
            }
        }
    }
    
    // 更新草稿指示器
    updateDraftIndicator() {
        if (!this.pageName || this.isEdit) return;
        
        const drafts = draftManager.getDraftsForPage(this.pageName);
        const draftCount = drafts.length;
        const currentDraft = this.currentDraftId ? drafts.find(d => d.id === this.currentDraftId) : null;
        
        // 查找或創建草稿指示器容器
        let indicatorContainer = document.getElementById('draftIndicatorContainer');
        if (!indicatorContainer) {
            // 在 dynamicFormContainer 中查找第一個 card，在其之前插入
            const container = document.getElementById('dynamicFormContainer');
            if (container) {
                // 查找第一個 card（主表單的 card）
                const firstCard = container.querySelector('.card');
                if (firstCard) {
                    indicatorContainer = document.createElement('div');
                    indicatorContainer.id = 'draftIndicatorContainer';
                    indicatorContainer.className = 'mb-3';
                    container.insertBefore(indicatorContainer, firstCard);
                } else {
                    // 如果找不到 card，在 container 的開頭插入
                    indicatorContainer = document.createElement('div');
                    indicatorContainer.id = 'draftIndicatorContainer';
                    indicatorContainer.className = 'mb-3';
                    container.insertBefore(indicatorContainer, container.firstChild);
                }
            } else {
                // 如果找不到 container，在表單開頭插入
                const form = document.getElementById('dynamicForm');
                if (form) {
                    indicatorContainer = document.createElement('div');
                    indicatorContainer.id = 'draftIndicatorContainer';
                    indicatorContainer.className = 'mb-3';
                    form.insertBefore(indicatorContainer, form.firstChild);
                }
            }
        }
        
        if (draftCount === 0) {
            indicatorContainer.innerHTML = '';
            return;
        }
        
        const draftText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.draft') : '草稿';
        const draftNumber = currentDraft?.keyField || currentDraft?.data?.order_number || currentDraft?.data?.code || '';
        const draftNumberText = draftNumber ? ` (${draftNumber})` : '';
        
        indicatorContainer.innerHTML = `
            <div class="alert alert-info d-flex justify-content-between align-items-center mb-0">
                <div>
                    <i class="bi bi-file-earmark-text"></i> 
                    <strong>${draftText}${draftNumberText}</strong>
                    <span class="badge bg-primary ms-2">${draftCount}</span>
                </div>
                <button type="button" class="btn btn-sm btn-outline-primary" onclick="window.dynamicForm.showDraftList()">
                    <i class="bi bi-list"></i> ${(typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.viewDrafts') : '查看草稿'}
                </button>
            </div>
        `;
    }
    
    // 顯示草稿列表
    showDraftList() {
        if (!this.pageName || this.isEdit) return;
        
        const drafts = draftManager.getDraftsForPage(this.pageName);
        const draftText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.draft') : '草稿';
        const loadText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.load') : '載入';
        const deleteText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.delete') : '刪除';
        const noDraftsText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.noDrafts') : '目前沒有草稿';
        
        const listContent = drafts.length > 0
            ? drafts.map(d => {
                const code = d.keyField || d.data?.order_number || d.data?.code || '';
                const name = d.data?.name || '';
                const createdAt = new Date(d.createdAt).toLocaleString();
                
                return `
                    <div class="list-group-item d-flex justify-content-between align-items-center">
                        <div class="flex-grow-1">
                            ${code ? `<div class="fw-bold">${code}</div>` : ''}
                            ${name ? `<div>${name}</div>` : ''}
                            <small class="text-muted">${createdAt}</small>
                        </div>
                        <div class="d-flex gap-2">
                            <button class="btn btn-sm btn-primary" onclick="window.dynamicForm.loadDraftFromModal('${d.id}')">${loadText}</button>
                            <button class="btn btn-sm btn-outline-danger" onclick="window.dynamicForm.deleteDraftFromModal('${d.id}', this)">${deleteText}</button>
                        </div>
                    </div>
                `;
            }).join('')
            : `<p class="text-muted mb-0">${noDraftsText}</p>`;

        const existingModal = document.getElementById('draftListModal');
        if (existingModal) existingModal.remove();

        const modal = document.createElement('div');
        modal.className = 'modal fade';
        modal.id = 'draftListModal';
        modal.innerHTML = `
            <div class="modal-dialog">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">${draftText}</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                    </div>
                    <div class="modal-body">
                        <div class="list-group">
                            ${listContent}
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(modal);
        const modalEl = new bootstrap.Modal(modal);
        modalEl.show();
        
        modal.addEventListener('hidden.bs.modal', () => {
            modal.remove();
        });
    }
    
    // 從模態框載入草稿
    async loadDraftFromModal(draftId) {
        const draft = draftManager.getDraft(this.pageName, draftId);
        if (draft) {
            this.currentDraftId = draftId;
            
            // 先填充表單數據（包括編號）
            await this.populateForm(draft.data);
            
            // 如果是產品表單，載入產品屬性
            if (this.pageName === 'products' && draft.data.product_attributes) {
                const attributesList = document.getElementById('productAttributesList');
                if (attributesList) {
                    attributesList.innerHTML = '';
                    for (const attr of draft.data.product_attributes) {
                        await this.addProductAttribute(attr.attribute_id, attr.value || '');
                    }
                }
            }
            
            // 如果是客戶表單，載入地址數據
            if (this.pageName === 'customers' && draft.data.customer_addresses) {
                this.customerAddresses = draft.data.customer_addresses;
                this.renderCustomerAddresses();
            }
            
            // 如果草稿中有編號字段，確保這些編號被預留（防止被其他表單使用）
            const numberFields = this.config.formFields.filter(f => 
                (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
                 f.key === 'sale_number' || f.key === 'purchase_order_number' ||
                 (f.key && f.key.includes('number'))) && f.readonly
            );
            
            for (const field of numberFields) {
                const fieldId = `field_${field.key}`;
                const input = document.getElementById(fieldId);
                if (input && input.value && input.value.trim() !== '') {
                    // 預留這個編號
                    try {
                        await this.reserveNumber(field.key, input.value);
                    } catch (err) {
                        console.warn(`預留編號失敗（可能已預留）: ${field.key} = ${input.value}`, err);
                    }
                }
            }
            
            App.showAlert((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.draftLoaded') : '已載入草稿', 'info');
            
            // 關閉模態框
            const modal = bootstrap.Modal.getInstance(document.getElementById('draftListModal'));
            if (modal) modal.hide();
            
            // 更新指示器
            this.updateDraftIndicator();
        }
    }
    
    // 從模態框刪除草稿
    deleteDraftFromModal(draftId, button) {
        if (!confirm((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.confirmDeleteDraft') : '確定要刪除此草稿嗎？')) {
            return;
        }
        
        draftManager.deleteDraft(this.pageName, draftId);
        
        // 如果刪除的是當前草稿，清除當前草稿 ID
        if (this.currentDraftId === draftId) {
            this.currentDraftId = null;
        }
        
        // 移除列表項
        button.closest('.list-group-item').remove();
        
        // 更新指示器
        this.updateDraftIndicator();
        
        // 如果沒有草稿了，關閉模態框
        const drafts = draftManager.getDraftsForPage(this.pageName);
        if (drafts.length === 0) {
            const modal = bootstrap.Modal.getInstance(document.getElementById('draftListModal'));
            if (modal) modal.hide();
        }
    }

    async loadDraft() {
        if (!this.pageName || this.isEdit) return;

        // 檢查 URL 參數是否有 draft_id（僅透過明確參數載入，不自動提示）
        const urlParams = new URLSearchParams(window.location.search);
        const draftId = urlParams.get('draft_id');

        if (draftId) {
            const draft = draftManager.getDraft(this.pageName, draftId);
            if (draft) {
                this.currentDraftId = draftId;
                
                // 先填充表單數據（包括編號）
                await this.populateForm(draft.data);
                
                // 如果是產品表單，載入產品屬性
                if (this.pageName === 'products' && draft.data.product_attributes) {
                    const attributesList = document.getElementById('productAttributesList');
                    if (attributesList) {
                        attributesList.innerHTML = '';
                        for (const attr of draft.data.product_attributes) {
                            await this.addProductAttribute(attr.attribute_id, attr.value || '');
                        }
                    }
                }
                
                // 如果是客戶表單，載入地址數據
                if (this.pageName === 'customers' && draft.data.customer_addresses) {
                    this.customerAddresses = draft.data.customer_addresses;
                    this.renderCustomerAddresses();
                }
                
                // 如果草稿中有編號字段，確保這些編號被預留（防止被其他表單使用）
                const numberFields = this.config.formFields.filter(f => 
                    (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
                     f.key === 'sale_number' || f.key === 'purchase_order_number' || f.key === 'employee_number' ||
                     (f.key && f.key.includes('number'))) && f.readonly
                );
                
                for (const field of numberFields) {
                    const fieldId = `field_${field.key}`;
                    const input = document.getElementById(fieldId);
                    if (input && input.value && input.value.trim() !== '') {
                        // 預留這個編號
                        try {
                            await this.reserveNumber(field.key, input.value);
                        } catch (err) {
                            console.warn(`預留編號失敗（可能已預留）: ${field.key} = ${input.value}`, err);
                        }
                    }
                }
                
                App.showAlert((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.draftLoaded') : '已載入草稿', 'info');
            }
        }
    }

    populateFromURLParams() {
        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.size === 0) return;

        this.config.formFields.forEach(field => {
            const paramValue = urlParams.get(field.key);
            if (paramValue !== null && paramValue !== undefined && paramValue !== '') {
                const fieldId = `field_${field.key}`;
                const input = document.getElementById(fieldId);
                if (input) {
                    if (field.type === 'checkbox') {
                        input.checked = paramValue === 'true' || paramValue === '1';
                    } else if ((field.type === 'select2' || field.type === 'select2-multi') && typeof $ !== 'undefined') {
                        // select2：補 option 再 set，避免沒有選項時無法顯示
                        try {
                            if ($(input).find(`option[value="${paramValue}"]`).length === 0) {
                                const opt = new Option(paramValue, paramValue, true, true);
                                $(input).append(opt);
                            }
                            // 等待 select2 初始化後再觸發 change
                            setTimeout(() => {
                                try { $(input).val(paramValue).trigger('change'); } catch (e) {}
                            }, 300);
                        } catch (e) {
                            input.value = paramValue;
                        }
                    } else {
                        input.value = paramValue;
                    }
                }
            }
        });
    }

    async applyDefaultCurrentUserAndDefaultBankAccount() {
        if (this.isEdit) return;
        if (!this.pageName) return;

        // 1) default current user for related_user_id
        try {
            const relatedField = this.config.formFields.find(f => f.key === 'related_user_id');
            if (relatedField) {
                const el = document.getElementById('field_related_user_id');
                if (el) {
                    const currentVal = (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) ? ($(el).val() || '') : (el.value || '');
                    if (!currentVal) {
                        const me = await App.apiRequest('/user/me');
                        if (me && me.id) {
                            if (typeof $ !== 'undefined') {
                                if ($(el).find(`option[value="${me.id}"]`).length === 0) {
                                    const opt = new Option(me.name || me.email || me.id, me.id, true, true);
                                    $(el).append(opt);
                                }
                                setTimeout(() => { try { $(el).val(me.id).trigger('change'); } catch (e) {} }, 100);
                            } else {
                                el.value = me.id;
                            }
                        }
                    }
                }
            }
        } catch (e) {
            console.warn('default related_user_id failed', e);
        }

        // 2) default bank account for incomes/expenses
        try {
            const el = document.getElementById('field_bank_account_id');
            if (!el) return;
            const currentVal = (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) ? ($(el).val() || '') : (el.value || '');
            if (currentVal) return;

            let url = null;
            if (this.pageName === 'expenses') {
                url = '/bank-accounts?status=active&is_default_payment=true';
            } else if (this.pageName === 'incomes') {
                url = '/bank-accounts?status=active&is_default_receiving=true';
            }
            if (!url) return;

            const res = await App.apiRequest(url);
            const list = (res && res.data) ? res.data : [];
            if (!list.length) return;
            const acc = list[0];
            if (!acc || !acc.id) return;

            if (typeof $ !== 'undefined') {
                if ($(el).find(`option[value="${acc.id}"]`).length === 0) {
                    const label = acc.account_number ? `${acc.name || ''} (${acc.account_number})`.trim() : (acc.name || acc.id);
                    const opt = new Option(label, acc.id, true, true);
                    $(el).append(opt);
                }
                setTimeout(() => { try { $(el).val(acc.id).trigger('change'); } catch (e) {} }, 100);
            } else {
                el.value = acc.id;
            }
        } catch (e) {
            console.warn('default bank_account_id failed', e);
        }

        // 3) default payment method for incomes/expenses
        try {
            const paymentMethodField = this.config.formFields.find(f => f.key === 'payment_method');
            if (!paymentMethodField) return;
            
            const el = document.getElementById('field_payment_method');
            if (!el) return;
            const currentVal = (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) ? ($(el).val() || '') : (el.value || '');
            if (currentVal) return;

            let url = null;
            if (this.pageName === 'incomes') {
                // 收入：獲取系統預設客戶付款方法
                url = '/payment-methods?status=active&is_default=true';
            } else if (this.pageName === 'expenses') {
                // 支出：獲取系統預設支出付款方法
                url = '/payment-methods?status=active&is_default_expense=true';
            }
            if (!url) return;

            const res = await App.apiRequest(url);
            const list = (res && res.data) ? res.data : [];
            if (!list.length) return;
            const paymentMethod = list[0];
            if (!paymentMethod || !paymentMethod.id) return;

            if (typeof $ !== 'undefined') {
                if ($(el).find(`option[value="${paymentMethod.id}"]`).length === 0) {
                    const label = paymentMethod.name || paymentMethod.code || paymentMethod.id;
                    const opt = new Option(label, paymentMethod.id, true, true);
                    $(el).append(opt);
                }
                setTimeout(() => { 
                    try { 
                        $(el).val(paymentMethod.id).trigger('change'); 
                    } catch (e) {
                        console.warn('設置默認付款方法失敗:', e);
                    }
                }, 200);
            } else {
                el.value = paymentMethod.id;
            }
        } catch (e) {
            console.warn('default payment_method failed', e);
        }

        // 4) default shift for users
        if (this.pageName === 'users') {
            try {
                const shiftField = this.config.formFields.find(f => f.key === 'shift_id');
                if (!shiftField) return;
                
                const el = document.getElementById('field_shift_id');
                if (!el) return;
                const currentVal = (typeof $ !== 'undefined' && $(el).hasClass('select2-hidden-accessible')) ? ($(el).val() || '') : (el.value || '');
                if (currentVal) return;

                // 獲取系統預設工作時段
                const res = await App.apiRequest('/shifts?is_default=true&limit=1');
                const list = (res && res.data) ? res.data : [];
                if (!list.length) return;
                const defaultShift = list[0];
                if (!defaultShift || !defaultShift.id) return;

                if (typeof $ !== 'undefined') {
                    if ($(el).find(`option[value="${defaultShift.id}"]`).length === 0) {
                        const label = defaultShift.name || defaultShift.id;
                        const opt = new Option(label, defaultShift.id, true, true);
                        $(el).append(opt);
                    }
                    setTimeout(() => { 
                        try { 
                            $(el).val(defaultShift.id).trigger('change'); 
                        } catch (e) {
                            console.warn('設置默認工作時段失敗:', e);
                        }
                    }, 200);
                } else {
                    el.value = defaultShift.id;
                }
            } catch (e) {
                console.warn('default shift_id failed', e);
            }
        }
    }

    getDiningQueueAreaId() {
        const areaEl = document.getElementById('field_area_id');
        if (!areaEl) return '';
        if (typeof $ !== 'undefined' && $(areaEl).hasClass('select2-hidden-accessible')) {
            return $(areaEl).val() || '';
        }
        return areaEl.value || '';
    }

    buildReservedNumberNextUrl(fieldKey) {
        let url = `/api/v1/reserved-numbers/next?field_name=${fieldKey}&page_name=${this.pageName}`;
        if (this.pageName === 'dining-queues' && fieldKey === 'ticket_number') {
            const areaId = this.getDiningQueueAreaId();
            if (areaId) {
                url += `&area_id=${encodeURIComponent(areaId)}`;
            }
        }
        return url;
    }

    async refreshDiningQueueTicketNumber() {
        if (this.pageName !== 'dining-queues' || this.isEdit) return;
        const input = document.getElementById('field_ticket_number');
        if (!input) return;
        const areaId = this.getDiningQueueAreaId();
        if (!areaId) {
            input.value = '';
            input.setAttribute('placeholder', '請先選擇桌區');
            input.disabled = true;
            return;
        }
        input.disabled = false;
        const response = await App.apiRequest(this.buildReservedNumberNextUrl('ticket_number'));
        if (response && response.next_number) {
            input.value = response.next_number;
            await this.reserveNumber('ticket_number', response.next_number);
        }
    }

    attachDiningQueueAreaWatcher() {
        if (this.pageName !== 'dining-queues') return;
        const areaEl = document.getElementById('field_area_id');
        if (!areaEl) return;
        const handler = () => {
            this.refreshDiningQueueTicketNumber().catch((e) => console.warn('refresh dining ticket failed', e));
        };
        if (typeof $ !== 'undefined' && $(areaEl).hasClass('select2-hidden-accessible')) {
            $(areaEl).off('.diningQueueTicket');
            $(areaEl).on('change.diningQueueTicket', handler);
            $(areaEl).on('select2:select.diningQueueTicket', handler);
            $(areaEl).on('select2:clear.diningQueueTicket', handler);
        } else {
            areaEl.removeEventListener('change', this._diningQueueAreaHandler || handler);
            areaEl.addEventListener('change', handler);
        }
        this._diningQueueAreaHandler = handler;
        this._diningQueueAreaWatcherAttached = true;
        if (this.getDiningQueueAreaId()) {
            handler();
        }
    }

    async loadReservedNumbers() {
        if (!this.pageName || this.isEdit) return;

        // 找出所有 readonly 的編號字段
        const numberFields = this.config.formFields.filter(f => 
            (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
             f.key === 'sale_number' || f.key === 'purchase_order_number' || f.key === 'employee_number' ||
             (f.key && f.key.includes('number'))) && f.readonly
        );

        console.log('loadReservedNumbers - 找到的編號字段:', numberFields.map(f => f.key)); // 調試用

        for (const field of numberFields) {
            const fieldId = `field_${field.key}`;
            // shipment-items 現在由 populateForm 處理，跳過
            if (field.type === 'shipment-items') {
                continue;
            }

            const input = document.getElementById(fieldId);
            if (!input) {
                console.warn(`loadReservedNumbers - 找不到字段: ${fieldId}`); // 調試用
                continue;
            }

            // 先檢查輸入框當前值（可能來自草稿）
            const currentValue = input.value;
            console.log(`loadReservedNumbers - ${field.key} 當前值:`, currentValue); // 調試用
            
            // 如果已經有值（來自草稿），檢查是否已預留
            if (currentValue && currentValue.trim() !== '') {
                try {
                    const response = await App.apiRequest(
                        `/api/v1/reserved-numbers/check?field_name=${field.key}&field_value=${encodeURIComponent(currentValue)}&page_name=${this.pageName}`
                    );
                    if (response.reserved) {
                        // 顯示已預留提示（products 页面不显示）
                        input.setAttribute('readonly', 'true');
                        input.classList.add('bg-light');
                        // 在字段下方顯示提示（如果還沒有），但 products 页面不显示
                        if (this.pageName !== 'products' && !input.parentElement.querySelector('.reserved-hint')) {
                            const helpText = document.createElement('small');
                            helpText.className = 'text-muted d-block mt-1 reserved-hint';
                            helpText.textContent = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.reservedNumber') : '此編號已預留';
                            input.parentElement.appendChild(helpText);
                        }
                        console.log(`loadReservedNumbers - ${field.key} 已預留:`, currentValue); // 調試用
                    }
                } catch (error) {
                    console.warn(`Failed to check reserved number for ${field.key}:`, error);
                }
            } else {
                // 如果沒有值，自動獲取下一個編號（含重試機制，防止 SPA 導航時 API 被中斷）
                try {
                    console.log(`loadReservedNumbers - 獲取下一個編號: ${field.key}`); // 調試用
                    if (this.pageName === 'dining-queues' && field.key === 'ticket_number') {
                        const areaId = this.getDiningQueueAreaId();
                        if (!areaId) {
                            this.attachDiningQueueAreaWatcher();
                            continue;
                        }
                    }
                    let response = null;
                    let lastError = null;
                    const maxRetries = 3;
                    for (let attempt = 0; attempt <= maxRetries; attempt++) {
                        try {
                            response = await App.apiRequest(this.buildReservedNumberNextUrl(field.key));
                            break; // success
                        } catch (retryError) {
                            lastError = retryError;
                            if (attempt < maxRetries) {
                                console.warn(`loadReservedNumbers - ${field.key} attempt ${attempt + 1} failed, retrying...`);
                                await new Promise(resolve => setTimeout(resolve, 300 * (attempt + 1)));
                            }
                        }
                    }
                    console.log(`loadReservedNumbers - API 響應:`, response); // 調試用
                    if (response && response.next_number) {
                        input.value = response.next_number;
                        console.log(`loadReservedNumbers - 設置 ${field.key} 為:`, response.next_number); // 調試用
                        // 自動預留這個編號
                        await this.reserveNumber(field.key, response.next_number);
                        console.log(`loadReservedNumbers - 已預留 ${field.key}:`, response.next_number); // 調試用
                    } else {
                        console.warn(`loadReservedNumbers - 沒有獲取到編號 for ${field.key}`, lastError || ''); // 調試用
                    }
                } catch (error) {
                    console.error(`Failed to get next number for ${field.key}:`, error); // 調試用
                }
            }
        }

        this.attachDiningQueueAreaWatcher();
    }
    
    async ensureReservedNumbers() {
        // 確保所有編號字段都有值（如果被草稿清空，重新獲取）
        if (!this.pageName || this.isEdit) return;

        const numberFields = this.config.formFields.filter(f => 
            (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
             f.key === 'sale_number' || f.key === 'purchase_order_number' || f.key === 'employee_number' ||
             (f.key && f.key.includes('number'))) && f.readonly
        );

        for (const field of numberFields) {
            const fieldId = `field_${field.key}`;
            // shipment-items 由 populateForm 處理，跳過
            if (field.type === 'shipment-items') continue;
            const input = document.getElementById(fieldId);
            if (!input) continue;

            // 如果字段為空，重新獲取編號（含重試機制）
            if (!input.value || input.value.trim() === '') {
                if (this.pageName === 'dining-queues' && field.key === 'ticket_number') {
                    const areaId = this.getDiningQueueAreaId();
                    if (!areaId) {
                        this.attachDiningQueueAreaWatcher();
                        continue;
                    }
                }
                const maxRetries = 3;
                for (let attempt = 0; attempt <= maxRetries; attempt++) {
                    try {
                        const response = await App.apiRequest(this.buildReservedNumberNextUrl(field.key));
                        if (response && response.next_number) {
                            input.value = response.next_number;
                            await this.reserveNumber(field.key, response.next_number);
                            break; // success
                        }
                    } catch (error) {
                        console.warn(`ensureReservedNumbers - ${field.key} attempt ${attempt + 1} failed:`, error);
                        if (attempt < maxRetries) {
                            await new Promise(resolve => setTimeout(resolve, 500 * (attempt + 1)));
                        }
                    }
                }
            }
        }
    }

    /**
     * 提交前確保所有編號字段都有值（最後一道防線）
     * 如果任何 readonly 編號字段為空，嘗試重新獲取；仍然為空則阻止提交
     * @returns {boolean} true = all numbers OK, false = some numbers missing
     */
    async ensureAllNumbersBeforeSubmit() {
        if (!this.pageName || this.isEdit) return true;

        const numberFields = this.config.formFields.filter(f => 
            (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
             f.key === 'sale_number' || f.key === 'purchase_order_number' || f.key === 'employee_number' ||
             (f.key && f.key.includes('number'))) && f.readonly
        );

        const missingFields = [];

        for (const field of numberFields) {
            const fieldId = `field_${field.key}`;
            if (field.type === 'shipment-items') continue;
            const input = document.getElementById(fieldId);
            if (!input) continue;

            if (!input.value || input.value.trim() === '') {
                // Last-resort attempt: try to fetch & reserve the number
                if (this.pageName === 'dining-queues' && field.key === 'ticket_number') {
                    const areaId = this.getDiningQueueAreaId();
                    if (!areaId) continue; // area not selected, skip
                }
                try {
                    const response = await App.apiRequest(this.buildReservedNumberNextUrl(field.key));
                    if (response && response.next_number) {
                        input.value = response.next_number;
                        await this.reserveNumber(field.key, response.next_number);
                    }
                } catch (e) {
                    console.error(`ensureAllNumbersBeforeSubmit - failed to fetch number for ${field.key}:`, e);
                }

                // After the attempt, check again
                if (!input.value || input.value.trim() === '') {
                    missingFields.push(field);
                }
            }
        }

        if (missingFields.length > 0) {
            const names = missingFields.map(f => this.getFieldLabel(f) || f.label || f.key).join(', ');
            const msg = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.numberGenerationFailed') : '編號生成失敗，請重新整理頁面再試';
            App.showAlert(`${msg} (${names})`, 'danger');
            return false;
        }
        return true;
    }
    
    async reserveNumber(fieldName, fieldValue) {
        try {
            await App.apiRequest(`/api/v1/reserved-numbers`, {
                method: 'POST',
                body: JSON.stringify({
                    field_name: fieldName,
                    field_value: fieldValue,
                    page_name: this.pageName
                })
            });
        } catch (error) {
            console.warn(`Failed to reserve number ${fieldValue}:`, error);
        }
    }

    collectFormData() {
        const formData = {};
        this.config.formFields.forEach(field => {
            // 為多個相同 key 的字段生成唯一 ID
            let fieldId = field._uniqueId || `field_${field.key}`;
            if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0 && !field._uniqueId) {
                const depValue = field.dependency.values[0];
                fieldId = `field_${field.key}_${depValue}`;
            }
            const input = document.getElementById(fieldId);
            if (!input) return;
            
            // 檢查字段是否可見（對於依賴字段，只收集可見字段的值）
            if (field.dependency) {
                const dependencyField = document.getElementById(`field_${field.dependency.field}`);
                if (dependencyField) {
                    const dependencyValue = dependencyField.type === 'checkbox' ? String(dependencyField.checked) : dependencyField.value;
                    const allowedValues = Array.isArray(field.dependency.values) ? field.dependency.values : [field.dependency.value];
                    if (!allowedValues.includes(dependencyValue)) {
                        // 字段不可見，跳過收集
                        return;
                    }
                }
            }

            // 跳過 my_referral_code 字段（只讀顯示用）
            if (field.key === 'my_referral_code') {
                return;
            }

            // shipment-items：收集用戶添加的產品明細
            if (field.type === 'shipment-items') {
                if (this.shipmentItems && this.shipmentItems.length > 0) {
                    formData[field.key] = this.shipmentItems;
                }
                return;
            }

            let value;
            if (field.type === 'default-include-single') {
                // default_include 單一 yes/no（產品稅=order、服務稅=service_order）
                const includeValue = field.includeValue || '';
                const checked = String(input.value) === 'true';
                value = checked && includeValue ? [includeValue] : [];
            } else if (field.type === 'checkbox-group') {
                // checkbox-group：獲取所有選中的 checkbox 值作為數組
                const checkboxes = document.querySelectorAll(`input[name="${field.key}[]"]:checked`);
                value = Array.from(checkboxes).map(cb => cb.value).filter(v => v);
                if (value.length === 0) {
                    value = null;
                }
            } else if (field.type === 'multi-select' || field.type === 'multiselect') {
                // 多選：獲取所有選中的值作為數組
                const selectedOptions = Array.from(input.selectedOptions);
                value = selectedOptions.map(option => option.value).filter(v => v);
                if (value.length === 0) {
                    value = null;
                }
            } else if (field.type === 'select2' || field.type === 'select2-multi') {
                // Select2 字段：使用 jQuery 獲取值
                if (typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                    value = $(input).val();
                    // 多選模式返回數組，單選返回字符串或 null
                    if (field.type === 'select2-multi') {
                        if (Array.isArray(value) && value.length === 0) {
                            value = null;
                        }
                    } else {
                        // 單選模式：如果是空字符串或 null，設為 null
                        if (value === '' || value === null || value === undefined) {
                            value = null;
                        } else if (typeof value === 'string' && value.trim() === '') {
                            value = null;
                        }
                    }
                } else {
                    value = input.value;
                    if (field.type === 'select2-multi') {
                        // 多選：獲取所有選中的值作為數組
                        const selectedOptions = Array.from(input.selectedOptions);
                        value = selectedOptions.map(option => option.value).filter(v => v);
                        if (value.length === 0) {
                            value = null;
                        }
                    } else {
                        // 單選：如果是空字符串，設為 null
                        if (value === '' || value === null || value === undefined) {
                            value = null;
                        }
                    }
                }
                
                // 如果字段是 UUID 類型（如 member_level_id, department_id, level_id），確保值格式正確
                if (value && (field.key.includes('_id') || field.relationApi)) {
                    // 確保值是有效的 UUID 字符串
                    if (typeof value === 'string' && value.trim() !== '') {
                        value = value.trim();
                    }
                }
            } else if (field.type === 'number') {
                // 處理數字字段
                if (input.value === '') {
                    // 空字符串的處理
                    if (this.isEdit && !field.required) {
                        // 編輯模式下，非必填數字字段為空時，發送 0 以清除字段
                        // 這樣後端會更新字段為 0（因為後端使用指針類型，null 會被忽略）
                        value = 0;
                    } else {
                        // 新建模式或必填字段：設為 null
                        value = null;
                    }
                } else if (input.value === '0') {
                    value = 0;
                } else {
                    const parsed = parseFloat(input.value);
                    value = isNaN(parsed) ? null : parsed;
                }
            } else if (field.type === 'checkbox') {
                value = input.checked;
            } else if (field.type === 'html-editor') {
                // HTML 編輯器：從 Quill 實例獲取 HTML 內容
                const editorId = `${fieldId}_editor`;
                const editorElement = document.getElementById(editorId);
                if (editorElement && editorElement.quill) {
                    value = editorElement.quill.root.innerHTML;
                    if (value === '<p><br></p>' || value === '<p></p>') {
                        value = null; // 空內容設為 null
                    }
                } else {
                    value = null;
                }
            } else if (field.type === 'time') {
                // time 類型：直接使用 HH:MM 格式
                value = input.value || null;
            } else if (field.type === 'datetime-local') {
                // datetime-local 改送 RFC3339，避免後端解析失敗
                if (input.value) {
                    const date = new Date(input.value);
                    value = isNaN(date.getTime()) ? null : date.toISOString();
                } else {
                    value = null;
                }
            } else if (field.key === 'phone' && (this.pageName === 'customers' || this.pageName === 'suppliers' || this.pageName === 'warehouses')) {
                // 保持電話為純號碼，區號在 phone_country_code 字段單獨保存
                value = input.value || '';
            } else if (field.type === 'select' && (field.key === 'auto_upgrade' || field.key === 'auto_apply_discount')) {
                // 處理 auto_upgrade 字段：將 'true'/'false' 字符串轉換為布爾值
                value = input.value === 'true' ? true : (input.value === 'false' ? false : null);
            } else if (field.type === 'select' && field.key === 'is_required') {
                // 處理 is_required 字段：保持 'yes'/'no' 字符串格式（後端會處理轉換）
                value = input.value || 'no';
            } else if (field.type === 'select' && (field.key === 'is_service_package' || field.key === 'allow_backorder')) {
                // 處理 is_service_package 和 allow_backorder 字段：將 'true'/'false' 字符串轉換為布爾值
                value = input.value === 'true' ? true : (input.value === 'false' ? false : false);
            } else if (field.type === 'select' && (field.key === 'requires_shipping' || field.key === 'is_default' || field.key === 'is_default_expense' || field.key === 'is_online_payment' || field.key === 'is_default_receiving' || field.key === 'is_default_payment')) {
                // 處理 boolean select 字段：將 'true'/'false' 字符串轉換為布爾值
                value = input.value === 'true' ? true : (input.value === 'false' ? false : false);
            } else if (field.type === 'select' && field.key === 'allow_overlap') {
                // 處理 allow_overlap（允許重複使用）：將 'true'/'false' 字符串轉換為布爾值
                value = input.value === 'true' ? true : (input.value === 'false' ? false : false);
            } else if (field.type === 'select' && field.key === 'all_day') {
                // 處理 all_day 字段：將 'yes'/'no' 字符串轉換為布爾值
                value = input.value === 'yes' ? true : (input.value === 'no' ? false : false);
            } else if (field.type === 'file') {
                // 文件字段：對於 image_url/logo_url（products, vehicles, equipments, services, stores, rooms, brands），從隱藏的 URL input 獲取值
                if ((field.key === 'image_url' || field.key === 'logo_url') && 
                    (this.pageName === 'products' || this.pageName === 'vehicles' || this.pageName === 'equipments' || 
                     this.pageName === 'services' || this.pageName === 'stores' || this.pageName === 'rooms' || this.pageName === 'brands')) {
                    const urlInput = document.getElementById(`${fieldId}_url`);
                    if (urlInput) {
                        value = urlInput.value || '';
                    } else {
                        value = '';
                    }
                } else {
                    // 其他文件字段：返回文件對象（稍後在 submitForm 中處理）
                value = input.files && input.files.length > 0 ? input.files[0] : null;
                }
            } else if (field.type === 'color') {
                // 顏色字段：優先使用 color picker 的值，如果沒有則使用 text input
                const colorInput = document.getElementById(fieldId);
                const textInput = document.getElementById(`${fieldId}_text`);
                value = colorInput ? colorInput.value : (textInput ? textInput.value : field.default || '#007bff');
            } else if (field.type === 'button-group') {
                // button-group 類型：從隱藏的 input 獲取值
                value = input.value || null;
            } else if (field.type === 'password' && input.value === '') {
                // 密碼字段為空時，不包含在數據中（編輯時不更新密碼）
                return;
            } else if (field.type === 'profile-image') {
                // Profile image 字段：從隱藏的 URL input 獲取值
                const urlInput = document.getElementById(`${fieldId}_url`);
                if (urlInput) {
                    value = urlInput.value || '';
                } else {
                    value = '';
                }
            } else {
                value = input.value;
                // 特殊處理：options 字段（產品屬性的選項，用逗號分隔）
                if (field.key === 'options' && value) {
                    // 將逗號分隔的字符串轉換為數組
                    if (typeof value === 'string' && value.trim() !== '') {
                        value = value.split(',').map(opt => opt.trim()).filter(opt => opt !== '');
                    } else {
                        value = null;
                    }
                    // 如果是空數組，設為 null
                    if (Array.isArray(value) && value.length === 0) {
                        value = null;
                    }
                } else {
                // 編輯模式下，空字符串應該發送空字符串，而不是 null
                // 這樣後端才能正確更新字段為空值
                // 只有密碼字段在編輯時為空才不發送（不更新密碼）
                if (value === '' && field.type === 'password' && this.isEdit) {
                    return; // 跳過密碼字段
                }
                // 編輯模式下，對於所有文本類型的字段，空字符串應該發送空字符串
                // 這樣後端才能知道要清空該字段（因為後端使用指針類型，null 會被忽略）
                // 新建模式下，空字符串可以轉為 null（因為字段本來就是空的）
                if (this.isEdit && value === '') {
                    // 編輯模式：發送空字符串，讓後端知道要清空該字段
                    // 不轉換為 null，保持為空字符串
                    value = '';
                } else if (!this.isEdit && value === '') {
                    // 新建模式：空字符串可以轉為 null
                    value = null;
                    }
                }
            }

            // phone-country-codes：name 永遠提交英文（顯示可翻譯，但 DB 要一致）
            if (this.pageName === 'phone-country-codes' && field.key === 'name') {
                const codeEl = document.getElementById('field_code');
                let codeVal = '';
                if (codeEl) {
                    if (typeof $ !== 'undefined' && $(codeEl).hasClass('select2-hidden-accessible')) {
                        codeVal = $(codeEl).val() || '';
                    } else {
                        codeVal = codeEl.value || '';
                    }
                }
                if (codeVal && typeof COUNTRY_PHONE_CODES !== 'undefined' && COUNTRY_PHONE_CODES[codeVal]) {
                    value = COUNTRY_PHONE_CODES[codeVal];
                } else if (input && input.dataset && input.dataset.submitValue) {
                    value = input.dataset.submitValue;
                }
            }

            formData[field.key] = value;
        });
        
        // ── 收集額外欄位（extra fields from field settings）──
        // 額外欄位的值存入 extra_fields JSON 物件中
        if (this.fieldSettings && this.fieldSettings.extraFields && this.fieldSettings.extraFields.length > 0) {
            const baseFieldKeys = new Set(this.config.formFields.map(f => f.key));
            const extraFieldsToCollect = this.fieldSettings.extraFields.filter(ef => !baseFieldKeys.has(ef.key));
            
            if (extraFieldsToCollect.length > 0) {
                const existingExtraFields = (this.loadedItem && this.loadedItem.extra_fields)
                    ? JSON.parse(JSON.stringify(this.loadedItem.extra_fields))
                    : {};
                const extraFieldsData = formData.extra_fields || existingExtraFields || {};
                
                extraFieldsToCollect.forEach(ef => {
                    const fieldId = `field_${ef.key}`;
                    const input = document.getElementById(fieldId);
                    if (!input) return;
                    
                    let value;
                    if (ef.type === 'number') {
                        value = input.value === '' ? null : (isNaN(parseFloat(input.value)) ? null : parseFloat(input.value));
                    } else if (ef.type === 'select' && typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                        value = $(input).val() || null;
                    } else if (ef.type === 'checkbox') {
                        value = input.checked;
                    } else if (ef.type === 'textarea' || ef.type === 'html-editor') {
                        value = input.value || null;
                    } else {
                        value = input.value || null;
                    }
                    
                    extraFieldsData[ef.key] = value;
                });
                
                formData.extra_fields = extraFieldsData;
            }
        }
        
        // Payment methods 特殊处理：将 payment_type 和网关连接字段保存到 extra_fields
        if (this.pageName === 'payment-methods') {
            const extraFields = formData.extra_fields || {};
            
            // 強制規則（避免只靠 UI 隱藏）
            // - normal: is_online_payment=false，且不保存任何 gateway 欄位
            // - gateway: is_online_payment=true，且只保存選中 gateway 對應欄位
            const paymentType = (formData.payment_type || (this.loadedItem && this.loadedItem.extra_fields && this.loadedItem.extra_fields.payment_type) || 'normal');
            const code = String(formData.code || '').toLowerCase();
            const isGateway = paymentType === 'gateway';
            const isStripeConnect = paymentType === 'stripe_connect';
            const stripeGatewayCodes = ['stripe', 'alipay', 'wechat_pay', 'apple_pay', 'google_pay'];
            const paypalGatewayCodes = ['paypal'];
            const qfpayGatewayCodes = ['fps', 'payme', 'alipay_hk', 'wechat_hk', 'boc_pay', 'octopus', 'unionpay'];
            const useStripeFields = stripeGatewayCodes.includes(code);
            const usePayPalFields = paypalGatewayCodes.includes(code);
            const useQFPayFields = qfpayGatewayCodes.includes(code);
            if (isStripeConnect) {
                // Stripe Connect: code = stripe_connect, is_online_payment = true, no API keys needed
                formData.is_online_payment = true;
                formData.code = 'stripe_connect';
                formData.name = 'Stripe Connect';
                delete formData.stripe_api_key;
                delete formData.stripe_secret_key;
                delete formData.paypal_client_id;
                delete formData.paypal_secret;
                delete formData.qfpay_app_code;
                delete formData.qfpay_client_key;
                delete formData.qfpay_base_url;
                delete formData.currency;
            } else if (!isGateway) {
                formData.is_online_payment = false;
                delete formData.stripe_api_key;
                delete formData.stripe_secret_key;
                delete formData.paypal_client_id;
                delete formData.paypal_secret;
                delete formData.qfpay_app_code;
                delete formData.qfpay_client_key;
                delete formData.qfpay_base_url;
                delete formData.currency;
            } else {
                formData.is_online_payment = true;
                if (useStripeFields) {
                    delete formData.paypal_client_id;
                    delete formData.paypal_secret;
                    delete formData.qfpay_app_code;
                    delete formData.qfpay_client_key;
                    delete formData.qfpay_base_url;
                } else if (usePayPalFields) {
                    delete formData.stripe_api_key;
                    delete formData.stripe_secret_key;
                    delete formData.qfpay_app_code;
                    delete formData.qfpay_client_key;
                    delete formData.qfpay_base_url;
                } else if (useQFPayFields) {
                    delete formData.stripe_api_key;
                    delete formData.stripe_secret_key;
                    delete formData.paypal_client_id;
                    delete formData.paypal_secret;
                } else {
                    // 未選擇 gateway：不保存任何 gateway 欄位
                    delete formData.stripe_api_key;
                    delete formData.stripe_secret_key;
                    delete formData.paypal_client_id;
                    delete formData.paypal_secret;
                    delete formData.qfpay_app_code;
                    delete formData.qfpay_client_key;
                    delete formData.qfpay_base_url;
                    delete formData.currency;
                }
            }
            
            // 保存 payment_type
            if (formData.payment_type) {
                extraFields.payment_type = formData.payment_type;
                delete formData.payment_type; // 从主数据中删除，只保存在 extra_fields
            }
            
            // 保存网关连接字段
            if (formData.stripe_api_key) {
                extraFields.stripe_api_key = formData.stripe_api_key;
                delete formData.stripe_api_key;
            }
            if (formData.stripe_secret_key) {
                extraFields.stripe_secret_key = formData.stripe_secret_key;
                delete formData.stripe_secret_key;
            }
            if (formData.paypal_client_id) {
                extraFields.paypal_client_id = formData.paypal_client_id;
                delete formData.paypal_client_id;
            }
            if (formData.paypal_secret) {
                extraFields.paypal_secret = formData.paypal_secret;
                delete formData.paypal_secret;
            }
            if (formData.qfpay_app_code) {
                extraFields.qfpay_app_code = formData.qfpay_app_code;
                delete formData.qfpay_app_code;
            }
            if (formData.qfpay_client_key) {
                extraFields.qfpay_client_key = formData.qfpay_client_key;
                delete formData.qfpay_client_key;
            }
            if (formData.qfpay_base_url) {
                extraFields.qfpay_base_url = formData.qfpay_base_url;
                delete formData.qfpay_base_url;
            }
            if (formData.currency) {
                extraFields.currency = formData.currency;
                delete formData.currency;
            }
            
            // 如果有 extra_fields 数据，设置到 formData
            if (Object.keys(extraFields).length > 0) {
                formData.extra_fields = extraFields;
            }
        }
        
        // Warehouses, Suppliers, Stores 特殊处理：将 address_country_code 和 address_region_code 保存到 extra_fields
        if (this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') {
            // 获取现有的 extra_fields（如果有）
            const existingExtraFields = this.loadedItem && this.loadedItem.extra_fields ? 
                JSON.parse(JSON.stringify(this.loadedItem.extra_fields)) : {};
            const extraFields = formData.extra_fields || existingExtraFields || {};
            
            // 保存地址国家代码（即使为 null 也要保存，以便清空字段）
            if (formData.address_country_code !== undefined) {
                if (formData.address_country_code === null || formData.address_country_code === '') {
                    extraFields.address_country_code = null;
                } else {
                extraFields.address_country_code = formData.address_country_code;
                }
                delete formData.address_country_code;
            }
            
            // 保存地址地区代码（即使为 null 也要保存，以便清空字段）
            if (formData.address_region_code !== undefined) {
                if (formData.address_region_code === null || formData.address_region_code === '') {
                    extraFields.address_region_code = null;
                } else {
                extraFields.address_region_code = formData.address_region_code;
                }
                delete formData.address_region_code;
            }
            
            // 合并其他 extra_fields 字段（如果有）
            if (formData.extra_fields) {
                Object.keys(formData.extra_fields).forEach(key => {
                    if (key !== 'address_country_code' && key !== 'address_region_code') {
                        extraFields[key] = formData.extra_fields[key];
                    }
                });
            }
            
            // 设置 extra_fields 到 formData（即使某些字段为 null，也要保存以更新数据库）
                formData.extra_fields = extraFields;
            }

        // Logistics companies：将 allowed_country_codes / allowed_region_keys 保存到 extra_fields
        if (this.pageName === 'logistics-companies') {
            const existingExtraFields = this.loadedItem && this.loadedItem.extra_fields ?
                JSON.parse(JSON.stringify(this.loadedItem.extra_fields)) : {};
            const extraFields = formData.extra_fields || existingExtraFields || {};

            // 多選空值代表「全允許」：依需求仍可保存空陣列（或 null）
            if (formData.allowed_country_codes !== undefined) {
                extraFields.allowed_country_codes = formData.allowed_country_codes;
                delete formData.allowed_country_codes;
            }
            if (formData.allowed_region_keys !== undefined) {
                extraFields.allowed_region_keys = formData.allowed_region_keys;
                delete formData.allowed_region_keys;
            }

            // 合併其他 extra_fields
            if (formData.extra_fields) {
                Object.keys(formData.extra_fields).forEach(key => {
                    if (key !== 'allowed_country_codes' && key !== 'allowed_region_keys') {
                        extraFields[key] = formData.extra_fields[key];
                    }
                });
            }

            formData.extra_fields = extraFields;
        }

        // Products / Services：將 show_on_vmarket 存入 extra_fields
        if (this.pageName === 'products' || this.pageName === 'services') {
            const existingExtraFields = this.loadedItem && this.loadedItem.extra_fields ?
                JSON.parse(JSON.stringify(this.loadedItem.extra_fields)) : {};
            const extraFields = formData.extra_fields || existingExtraFields || {};

            if (formData.show_on_vmarket !== undefined) {
                extraFields.show_on_vmarket = !!formData.show_on_vmarket;
                delete formData.show_on_vmarket;
            } else if (this.vmarketJoined === true && extraFields.show_on_vmarket === undefined) {
                extraFields.show_on_vmarket = true;
            }

            formData.extra_fields = extraFields;
        }
        
        return formData;
    }

    async loadItemData() {
        try {
            if (!this.config || !this.config.apiPath) {
                console.error('配置錯誤: config 或 apiPath 未設置', this.config);
                App.showAlert('配置錯誤: 無法載入數據', 'danger');
                return;
            }
            if (!this.itemId) {
                console.error('ID 錯誤: itemId 未設置', this.itemId);
                App.showAlert('ID 錯誤: 無法載入數據', 'danger');
                return;
            }
            
            // 等待 DOM 完全渲染
            await new Promise(resolve => setTimeout(resolve, 50));
            
            const apiUrl = `${this.config.apiPath}/${this.itemId}`;
            console.log('載入數據 - API URL:', apiUrl, 'itemId:', this.itemId, 'apiPath:', this.config.apiPath); // 調試用
            const item = await App.apiRequest(apiUrl);
            console.log('載入的數據:', item); // 調試用
            console.log('載入的數據 - extra_fields:', item ? item.extra_fields : 'null'); // 調試用
            if (item && item.extra_fields) {
                console.log('載入的數據 - extra_fields.address_country_code:', item.extra_fields.address_country_code);
                console.log('載入的數據 - extra_fields.address_region_code:', item.extra_fields.address_region_code);
            }
            
            if (!item) {
                console.error('載入的數據為空');
                App.showAlert('載入的數據為空', 'danger');
                return;
            }
            this.loadedItem = item;
            
            // 先載入所有 relation-select 和 select2 字段的選項
            for (const field of this.config.formFields) {
                if (field.type === 'select2' || field.type === 'select2-multi') {
                    await this.initSelect2(field);
                    // 等待 Select2 初始化完成
                    await new Promise(resolve => setTimeout(resolve, 50));
                } else if ((field.type === 'relation-select' || (field.type === 'select' && field.relationApi)) && field.type !== 'multi-select') {
                    await this.loadRelationFieldOptions(field);
                    // 等待選項載入完成
                    await new Promise(resolve => setTimeout(resolve, 50));
                }
            }
            
            // 再次等待，確保所有字段都已準備好
            await new Promise(resolve => setTimeout(resolve, 100));
            
            // 然後填充表單
            await this.populateForm(item);

            // 填充 users 所屬店舖子表
            if (this.pageName === 'users') {
                this.applyUserStoreSelections(item);
            }
            
            // Payment methods: 填充完成后触发处理
            if (this.pageName === 'payment-methods') {
                setTimeout(() => {
                    this.handlePaymentTypeChange();
                }, 200);
            }
            
            // 填充完成後，再次檢查並重試填充（處理可能遺漏的字段）
                await this.retryPopulateForm(item);
            
            // 额外等待，确保所有异步操作（包括 Select2 选项加载、国家/地区字段加载）都完成
            await new Promise(resolve => setTimeout(resolve, 500));
            
            // 所有數據加載完成後，隱藏 loading overlay（這會觸發顯示 button bar）
            this.hideLoadingOverlay();
        } catch (error) {
            console.error('載入數據失敗:', error);
            this.hideLoadingOverlay();
            App.showAlert('載入數據失敗: ' + error.message, 'danger');
            setTimeout(() => {
                const listPath = this.getListPathWithParams();
                if (typeof Router !== 'undefined' && Router.go) {
                    Router.go(listPath);
                } else {
                    window.location.href = listPath;
                }
            }, 2000);
        }
    }
    
    // 顯示 loading overlay
    showLoadingOverlay() {
        // 查找 main-content 容器
        const mainContent = document.querySelector('.main-content');
        if (!mainContent) {
            // 如果找不到 main-content，等待一下再試
            setTimeout(() => this.showLoadingOverlay(), 100);
            return;
        }
        
        // 隱藏 button bar（確保在顯示 loading 時隱藏）
        const buttonBar = document.querySelector('.form-button-bar');
        if (buttonBar) {
            buttonBar.classList.remove('show');
            // 標記為正在 loading，防止 MutationObserver 觸發顯示
            buttonBar.setAttribute('data-loading', 'true');
        }
        
        // 檢查是否已經存在 overlay
        let overlay = document.getElementById('formLoadingOverlay');
        if (overlay) {
            overlay.style.display = 'flex';
            return;
        }
        
        // 確保 main-content 有相對定位
        const originalPosition = window.getComputedStyle(mainContent).position;
        if (originalPosition === 'static') {
            mainContent.style.position = 'relative';
        }
        
        // 創建 overlay
        overlay = document.createElement('div');
        overlay.id = 'formLoadingOverlay';
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
            </div>
        `;
        
        mainContent.appendChild(overlay);
    }
    
    // 隱藏 loading overlay
    hideLoadingOverlay() {
        const overlay = document.getElementById('formLoadingOverlay');
        if (!overlay) {
            // 即使 overlay 不存在，也要確保按鈕欄顯示
            setTimeout(() => {
                const buttonBar = document.querySelector('.form-button-bar');
                if (buttonBar) {
                    buttonBar.removeAttribute('data-loading');
                }
                if (typeof setupFormButtonBar === 'function') {
                    setupFormButtonBar(true); // 強制顯示
                }
            }, 150);
            return;
        }
        
        // 先隱藏 overlay
        overlay.style.display = 'none';
        
        // 確保 button bar 在 loading 完全隱藏後才顯示
        // 使用 requestAnimationFrame 確保 DOM 更新完成
        requestAnimationFrame(() => {
            // 再次確認 overlay 已隱藏且不在 DOM 中可見
            const checkOverlay = document.getElementById('formLoadingOverlay');
            const isOverlayHidden = !checkOverlay || 
                                   checkOverlay.style.display === 'none' || 
                                   window.getComputedStyle(checkOverlay).display === 'none';
            
            // 無論 overlay 是否隱藏，都要確保按鈕欄顯示（避免某些邊緣情況）
            // 移除 loading 標記
            const buttonBar = document.querySelector('.form-button-bar');
            if (buttonBar) {
                buttonBar.removeAttribute('data-loading');
            }
            
            // 延遲一下確保所有 DOM 更新完成
                setTimeout(() => {
                    // 顯示 button bar（如果表單已準備好）
                    // 使用 forceShow=true 確保按鈕欄一定會顯示
                    if (typeof setupFormButtonBar === 'function') {
                        setupFormButtonBar(true);
                    }
                }, 150);
        });
    }
    
    // 重試填充表單（處理可能遺漏的字段）
    async retryPopulateForm(item) {
        let retryCount = 0;
        const maxRetries = 3;
        
        while (retryCount < maxRetries) {
            let hasEmptyFields = false;
            
            // 檢查是否有字段應該有值但為空
            for (const field of this.config.formFields) {
                const fieldId = `field_${field.key}`;
                const input = document.getElementById(fieldId);
                if (!input) continue;
                
                const value = this.getNestedValue(item, field.key);
                if (value !== null && value !== undefined && value !== '') {
                    // 如果數據中有值，但輸入框為空，需要重新填充
                    if (field.type === 'text' || field.type === 'textarea' || field.type === 'email' || field.type === 'number') {
                        if (!input.value || input.value.trim() === '') {
                            console.log(`重試填充字段 ${field.key}:`, value);
                            input.value = value || '';
                            hasEmptyFields = true;
                        }
                    } else if (field.type === 'select' && !field.relationApi) {
                        // 特殊處理 is_required、allow_backorder、is_service_package、requires_shipping、is_default、is_default_receiving、is_default_payment 和 all_day 字段
                        let expectedValue = String(value);
                        if (field.key === 'is_required' || field.key === 'allow_backorder' || field.key === 'is_service_package' || field.key === 'requires_shipping' || field.key === 'is_default' || field.key === 'is_default_receiving' || field.key === 'is_default_payment') {
                            if (value === true || value === 'true' || value === 'yes' || value === 1 || value === '1') {
                                expectedValue = field.key === 'is_required' ? 'yes' : 'true';
                            } else {
                                expectedValue = field.key === 'is_required' ? 'no' : 'false';
                            }
                        } else if (field.key === 'all_day') {
                            if (value === true || value === 'true' || value === 'yes' || value === 1 || value === '1') {
                                expectedValue = 'yes';
                            } else {
                                expectedValue = 'no';
                            }
                        }
                        if (input.value !== expectedValue) {
                            console.log(`重試填充 select 字段 ${field.key}:`, value);
                            input.value = expectedValue;
                            hasEmptyFields = true;
                        }
                    }
                }
            }
            
            if (!hasEmptyFields) {
                break; // 所有字段都已填充，退出重試
            }
            
            retryCount++;
            if (retryCount < maxRetries) {
                await new Promise(resolve => setTimeout(resolve, 100));
            }
        }
    }

    async populateForm(item) {
        console.log('[DEBUG populateForm] 开始填充表单，pageName:', this.pageName);
        console.log('[DEBUG populateForm] item:', item);
        console.log('[DEBUG populateForm] item.extra_fields:', item ? item.extra_fields : 'null');
        if (item && item.extra_fields) {
            console.log('[DEBUG populateForm] extra_fields.address_country_code:', item.extra_fields.address_country_code);
            console.log('[DEBUG populateForm] extra_fields.address_region_code:', item.extra_fields.address_region_code);
        }
        
        // 收集所有异步操作的 Promise
        const asyncPromises = [];
        
        // 如果是優惠券，載入條件
        if (this.pageName === 'coupons' && item.conditions) {
            const conditionsList = document.getElementById('conditionsList');
            if (conditionsList) {
                conditionsList.innerHTML = '';
                for (const condition of item.conditions) {
                    await this.addConditionFromData(condition);
                }
            }
        }
        
        // 如果是推廣表單，根據 scheduled_at 設置 send_type
        if (this.pageName === 'promotions') {
            const sendTypeSelect = document.getElementById('field_send_type');
            if (sendTypeSelect) {
                if (item.scheduled_at) {
                    sendTypeSelect.value = 'scheduled';
                } else {
                    sendTypeSelect.value = 'immediate';
                }
                // 觸發 change 事件以更新排程時間字段的顯示
                sendTypeSelect.dispatchEvent(new Event('change'));
            }
        }
        
        // 載入 shipment-items 產品明細（編輯模式）
        if (this.pageName === 'shipments' && item) {
            const itemsField = this.config.formFields.find(f => f.type === 'shipment-items');
            if (itemsField) {
                const fieldId = `field_${itemsField.key}`;
                const items = (item.extra_fields && Array.isArray(item.extra_fields.items))
                    ? item.extra_fields.items
                    : (Array.isArray(item.items) ? item.items : []);
                
                if (items && items.length > 0) {
                    // 延遲添加，確保 DOM 已渲染且產品列表已載入
                    setTimeout(() => {
                        this.loadExistingShipmentItems(items, fieldId);
                    }, 500);
                }
            }
        }
        
        // 如果是產品表單，載入產品屬性
        if (this.pageName === 'products' && this.isEdit && this.itemId) {
            // 延遲加載，確保產品屬性區域已渲染
            setTimeout(() => {
                this.loadProductAttributes();
            }, 300);
        }
        
        // 如果是產品表單，根據 is_service_package 設置服務套票選項
        if (this.pageName === 'products') {
            const servicePackageSelect = document.getElementById('field_is_service_package');
            if (servicePackageSelect) {
                // 將 boolean 值轉換為 true/false 字符串
                if (item.is_service_package === true || item.is_service_package === 'true' || item.is_service_package === true) {
                    servicePackageSelect.value = 'true';
                } else {
                    servicePackageSelect.value = 'false';
                }
                // 延遲觸發 change 事件以確保依賴字段已初始化
                setTimeout(async () => {
                    // 先觸發 change 事件
                    servicePackageSelect.dispatchEvent(new Event('change'));
                    // 然後手動調用切換函數確保顯示
                    if (this.toggleServicePackageService) {
                        await this.toggleServicePackageService(servicePackageSelect);
                    }
                    // 同時調用 setupDependencies 確保依賴字段正確顯示
                    if (this.setupDependencies) {
                        this.setupDependencies();
                    }
                }, 500);
            }
            
            // 處理 allow_backorder 字段
            const allowBackorderSelect = document.getElementById('field_allow_backorder');
            if (allowBackorderSelect) {
                // 將 boolean 值轉換為 true/false 字符串
                if (item.allow_backorder === true || item.allow_backorder === 'true' || item.allow_backorder === true) {
                    allowBackorderSelect.value = 'true';
                } else {
                    allowBackorderSelect.value = 'false';
                }
            }
        }
        
        // 特殊處理：appointments 的 rooms, equipments, vehicles 數組
        if (this.pageName === 'appointments') {
            // 處理 room_ids
            if (item.rooms && Array.isArray(item.rooms)) {
                const roomIds = item.rooms.map(room => room.id || room);
                const roomInput = document.getElementById('field_room_ids');
                if (roomInput && typeof $ !== 'undefined' && $(roomInput).hasClass('select2-hidden-accessible')) {
                    $(roomInput).val(roomIds).trigger('change');
                }
            }
            // 處理 equipment_ids
            if (item.equipments && Array.isArray(item.equipments)) {
                const equipmentIds = item.equipments.map(equipment => equipment.id || equipment);
                const equipmentInput = document.getElementById('field_equipment_ids');
                if (equipmentInput && typeof $ !== 'undefined' && $(equipmentInput).hasClass('select2-hidden-accessible')) {
                    $(equipmentInput).val(equipmentIds).trigger('change');
                }
            }
            // 處理 vehicle_ids
            if (item.vehicles && Array.isArray(item.vehicles)) {
                const vehicleIds = item.vehicles.map(vehicle => vehicle.id || vehicle);
                const vehicleInput = document.getElementById('field_vehicle_ids');
                if (vehicleInput && typeof $ !== 'undefined' && $(vehicleInput).hasClass('select2-hidden-accessible')) {
                    $(vehicleInput).val(vehicleIds).trigger('change');
                }
            }
        }
        
        // 特殊處理：stamp-settings 的 earning_products, earning_services 和 redeemable_products 數組
        if (this.pageName === 'stamp-settings') {
            // 處理 earning_products (JSON 格式是小寫)
            const earningProducts = item.earning_products || item.EarningProducts;
            if (earningProducts && Array.isArray(earningProducts)) {
                const productIds = earningProducts.map(ep => ep.product_id || ep.ProductID || (ep.product && ep.product.id) || (ep.Product && ep.Product.id));
                const productInput = document.getElementById('field_earning_products');
                if (productInput && typeof $ !== 'undefined' && $(productInput).hasClass('select2-hidden-accessible')) {
                    $(productInput).val(productIds.filter(id => id)).trigger('change');
                }
            }
            // 處理 earning_services (JSON 格式是小寫)
            const earningServices = item.earning_services || item.EarningServices;
            if (earningServices && Array.isArray(earningServices)) {
                const serviceIds = earningServices.map(es => es.service_id || es.ServiceID || (es.service && es.service.id) || (es.Service && es.Service.id));
                const serviceInput = document.getElementById('field_earning_services');
                if (serviceInput && typeof $ !== 'undefined' && $(serviceInput).hasClass('select2-hidden-accessible')) {
                    $(serviceInput).val(serviceIds.filter(id => id)).trigger('change');
                }
            }
            // 處理 redeemable_products (JSON 格式是小寫)
            const redeemableProducts = item.redeemable_products || item.RedeemableProducts;
            if (redeemableProducts && Array.isArray(redeemableProducts)) {
                const redeemableProductIds = redeemableProducts.map(rp => rp.product_id || rp.ProductID || (rp.product && rp.product.id) || (rp.Product && rp.Product.id));
                const redeemableInput = document.getElementById('field_redeemable_products');
                if (redeemableInput && typeof $ !== 'undefined' && $(redeemableInput).hasClass('select2-hidden-accessible')) {
                    $(redeemableInput).val(redeemableProductIds.filter(id => id)).trigger('change');
                }
            }
        }
        
        this.config.formFields.forEach(field => {
            // 為多個相同 key 的字段生成唯一 ID
            let fieldId = field._uniqueId || `field_${field.key}`;
            if (field.key === 'reference_id' && field.dependency && field.dependency.values && field.dependency.values.length > 0 && !field._uniqueId) {
                const depValue = field.dependency.values[0];
                fieldId = `field_${field.key}_${depValue}`;
            }
            
            // 调试：检查 address_country_code 和 address_region_code 字段
            if ((this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') && 
                (field.key === 'address_country_code' || field.key === 'address_region_code')) {
                console.log(`[DEBUG] 找到字段 ${field.key}, fieldId: ${fieldId}`);
            }
            
            const input = document.getElementById(fieldId);
            if (!input) {
                // 调试：如果找不到 input，记录一下
                if ((this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') && 
                    (field.key === 'address_country_code' || field.key === 'address_region_code')) {
                    console.warn(`[DEBUG] 找不到输入元素 ${fieldId} for field ${field.key}`);
                }
                return;
            }
            
            // 對於依賴字段，檢查是否應該顯示（基於當前依賴字段的值或數據中的值）
            if (field.dependency) {
                const dependencyField = document.getElementById(`field_${field.dependency.field}`);
                let dependencyValue = '';
                if (dependencyField) {
                    // checkbox 使用 checked 狀態
                    if (dependencyField.type === 'checkbox') {
                        dependencyValue = dependencyField.checked ? 'true' : 'false';
                    } else {
                        dependencyValue = dependencyField.value;
                    }
                } else {
                    // 如果依賴字段還沒有值，從數據中獲取
                    dependencyValue = this.getNestedValue(item, field.dependency.field) || '';
                    // 也嘗試從 extra_fields 獲取
                    if (!dependencyValue && item && item.extra_fields && item.extra_fields[field.dependency.field] !== undefined) {
                        dependencyValue = String(item.extra_fields[field.dependency.field]);
                    }
                }
                const allowedValues = Array.isArray(field.dependency.values) ? field.dependency.values : [field.dependency.value];
                if (!allowedValues.includes(dependencyValue)) {
                    // 字段不可見，跳過填充
                    return;
                }
            }
            
            // 特殊處理：對於 reference_id 字段，需要根據 reference_type 來決定填充哪個字段
            if (field.key === 'reference_id' && field.dependency && field.dependency.field === 'category') {
                // 獲取當前的 reference_type 或 category
                const referenceType = this.getNestedValue(item, 'reference_type') || this.getNestedValue(item, 'category') || '';
                const category = this.getNestedValue(item, 'category') || '';
                
                // 檢查這個 reference_id 字段是否對應當前的 reference_type/category
                const allowedValues = Array.isArray(field.dependency.values) ? field.dependency.values : [field.dependency.value];
                const shouldFill = allowedValues.includes(category) || 
                    (referenceType === 'order' && allowedValues.includes('order')) ||
                    (referenceType === 'service_order' && allowedValues.includes('service_order')) ||
                    (referenceType === 'purchase_order' && allowedValues.includes('purchase'));
                
                if (!shouldFill) {
                    // 這個 reference_id 字段不對應當前的類型，跳過填充
                    return;
                }
            }

            // 特殊處理：如果是 readonly 的編號字段，且當前已有值（來自預留編號），則保留當前值
            // 也防止 null/undefined 的草稿數據清空已預留的編號
            const isNumberField = (field.key === 'order_number' || field.key === 'code' || field.key === 'invoice_number' || 
                                   field.key === 'sale_number' || field.key === 'purchase_order_number' || field.key === 'employee_number' ||
                                   (field.key && field.key.includes('number'))) && field.readonly;
            if (isNumberField) {
                if (input.value && input.value.trim() !== '') {
                    // 保留已預留的編號，不覆蓋
                    console.log(`populateForm - 保留已預留的編號 ${field.key}:`, input.value); // 調試用
                    return;
                }
                // If the incoming value is null/undefined/empty, don't clear the field —
                // loadReservedNumbers() or ensureReservedNumbers() will populate it later
                const incomingValue = this.getNestedValue(item, field.key);
                if (incomingValue === null || incomingValue === undefined || incomingValue === '') {
                    console.log(`populateForm - 跳過空值的編號字段 ${field.key}，等待 loadReservedNumbers 填充`); // 調試用
                    return;
                }
            }

            // 特殊處理：my_referral_code 應該顯示客戶自己的 code（編號）
            let value = this.getNestedValue(item, field.key);

            // Logistics companies：allowed_country_codes / allowed_region_keys 來自 extra_fields
            if (this.pageName === 'logistics-companies' && (value === null || value === undefined || value === '') && item && item.extra_fields) {
                if ((field.key === 'allowed_country_codes' || field.key === 'allowed_region_keys') && item.extra_fields[field.key] !== undefined) {
                    value = item.extra_fields[field.key];
                }
            }

            // Products / Services：show_on_vmarket 來自 extra_fields
            if ((this.pageName === 'products' || this.pageName === 'services') && (value === null || value === undefined || value === '') && item && item.extra_fields) {
                if (field.key === 'show_on_vmarket' && item.extra_fields.show_on_vmarket !== undefined) {
                    value = item.extra_fields.show_on_vmarket;
                }
            }
            
            // 特殊處理：users 的佣金設置可能來自 extra_fields
            if (this.pageName === 'users') {
                // 訂單佣金設置
                if (field.key === 'order_commission_mode') {
                    if (!value && item.extra_fields && item.extra_fields.order_commission_mode) {
                        value = item.extra_fields.order_commission_mode;
                    }
                    // 如果仍然沒有值，使用默認值 'percent'
                    if (!value || value === '') {
                        value = 'percent';
                    }
                }
                if (field.key === 'order_commission_rate' && (value === null || value === undefined || value === '') && item.extra_fields && item.extra_fields.order_commission_rate !== undefined) {
                    value = item.extra_fields.order_commission_rate;
                }
                if (field.key === 'order_commission_fixed' && (value === null || value === undefined || value === '') && item.extra_fields && item.extra_fields.order_commission_fixed !== undefined) {
                    value = item.extra_fields.order_commission_fixed;
                }
                // 服務單佣金設置
                if (field.key === 'service_order_commission_mode') {
                    if (!value && item.extra_fields && item.extra_fields.service_order_commission_mode) {
                        value = item.extra_fields.service_order_commission_mode;
                    }
                    // 如果仍然沒有值，使用默認值 'percent'
                    if (!value || value === '') {
                        value = 'percent';
                    }
                }
                if (field.key === 'service_order_commission_rate' && (value === null || value === undefined || value === '') && item.extra_fields && item.extra_fields.service_order_commission_rate !== undefined) {
                    value = item.extra_fields.service_order_commission_rate;
                }
                if (field.key === 'service_order_commission_fixed' && (value === null || value === undefined || value === '') && item.extra_fields && item.extra_fields.service_order_commission_fixed !== undefined) {
                    value = item.extra_fields.service_order_commission_fixed;
                }
                // 向後兼容：舊的通用佣金設置
                if (field.key === 'commission_mode' && !value && item.extra_fields && item.extra_fields.commission_mode) {
                    value = item.extra_fields.commission_mode;
                }
                if (field.key === 'commission_fixed' && (value === null || value === undefined || value === '') && item.extra_fields && item.extra_fields.commission_fixed !== undefined) {
                    value = item.extra_fields.commission_fixed;
                }
            }
            
            if (field.key === 'my_referral_code') {
                value = item.code || ''; // 使用客戶自己的編號作為推薦碼
                // 如果沒有編號，隱藏該字段
                const container = document.getElementById('my_referral_code_container');
                if (container) {
                    container.style.display = value ? 'block' : 'none';
                }
            }
            
            // 特殊處理：member_level_id 可能來自 member_level 對象
            if (field.key === 'member_level_id' && !value && item.member_level) {
                value = item.member_level.id || item.member_level_id;
            }
            
            // 特殊處理：customer_id 可能來自 customer 對象
            if (field.key === 'customer_id' && !value && item.customer) {
                value = item.customer.id || item.customer_id;
            }
            
            // 特殊處理：product_type_id 可能來自 product_type 對象
            if (field.key === 'product_type_id' && !value && item.product_type) {
                value = item.product_type.id || item.product_type_id;
            }
            
            // 特殊處理：brand_id 可能來自 brand 對象
            if (field.key === 'brand_id' && !value && item.brand) {
                value = item.brand.id || item.brand_id;
            }
            
            // 特殊處理：default_supplier_id 可能來自 default_supplier 對象
            if (field.key === 'default_supplier_id' && !value && item.default_supplier) {
                value = item.default_supplier.id || item.default_supplier_id;
            }
            
            // 特殊處理：default_warehouse_id 可能來自 default_warehouse 對象
            if (field.key === 'default_warehouse_id' && !value && item.default_warehouse) {
                value = item.default_warehouse.id || item.default_warehouse_id;
            }
            
            // 特殊處理：default_warehouse_zone_id 可能來自 default_warehouse_zone 對象
            if (field.key === 'default_warehouse_zone_id' && !value && item.default_warehouse_zone) {
                value = item.default_warehouse_zone.id || item.default_warehouse_zone_id;
            }
            
            // 特殊處理：salesperson_id 可能來自 salesperson 對象
            if (field.key === 'salesperson_id' && !value && item.salesperson) {
                value = item.salesperson.id || item.salesperson_id;
            }
            
            // 特殊處理：order_id 可能來自 order 對象
            if (field.key === 'order_id' && !value && item.order) {
                value = item.order.id || item.order_id;
            }
            
            // default_include 單一 yes/no
            if (field.type === 'default-include-single') {
                const arr = Array.isArray(value) ? value : [];
                const includeValue = field.includeValue || '';
                input.value = (includeValue && arr.includes(includeValue)) ? 'true' : 'false';
                return;
            }
            // 單一 checkbox
            if (field.type === 'checkbox') {
                input.checked = value === true || value === 'true' || value === 1 || value === '1';
                return;
            }
            // 處理 checkbox-group 字段
            if (field.type === 'checkbox-group') {
                // 如果是數組，提取值（可能是對象數組或字符串數組）
                let values = [];
                if (Array.isArray(value)) {
                    values = value.map(item => {
                        // 如果是對象，提取 id 字段或直接使用字符串值
                        if (typeof item === 'object' && item !== null) {
                            return item.id || item.value || String(item);
                        }
                        // 如果是字符串或數字，直接使用
                        return String(item);
                    });
                } else if (value) {
                    // 如果是單個值（可能是對象或字符串）
                    if (typeof value === 'object' && value !== null) {
                        values = [String(value.id || value.value || value)];
                    } else {
                        values = [String(value)];
                    }
                }
                
                // 設置所有相關的 checkbox 為選中/未選中
                const checkboxes = document.querySelectorAll(`input[name="${field.key}[]"]`);
                checkboxes.forEach(checkbox => {
                    checkbox.checked = values.includes(checkbox.value);
                });
                return;
            }
            
            // 處理多選字段
            if (field.type === 'multi-select' || field.type === 'multiselect') {
                // 如果是數組，提取 ID（可能是對象數組或 ID 數組）
                let ids = [];
                if (Array.isArray(value)) {
                    ids = value.map(item => {
                        // 如果是對象，提取 id 字段
                        if (typeof item === 'object' && item !== null) {
                            return item.id || item[field.relationValueKey || 'id'];
                        }
                        // 如果是字符串或數字，直接使用
                        return item;
                    });
                } else if (value) {
                    // 如果是單個值（可能是對象或 ID）
                    if (typeof value === 'object' && value !== null) {
                        ids = [value.id || value[field.relationValueKey || 'id']];
                    } else {
                        ids = [value];
                    }
                }
                
                // 設置選項為選中
                Array.from(input.options).forEach(option => {
                    option.selected = ids.includes(option.value);
                });
                return;
            }
            
            // 特殊處理：Warehouses, Suppliers, Stores 的 address_country_code 和 address_region_code
            // 必須在 select2 處理之前，因為這些字段是 select2 類型
            // 注意：只处理国家字段，地区字段会在国家字段完成后单独处理
            if ((this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') && 
                field.key === 'address_country_code') {
                // 先尝试从 extra_fields 获取值
                let addressValue = null;
                if (item.extra_fields && item.extra_fields.address_country_code) {
                    addressValue = item.extra_fields.address_country_code;
                } else if (value) {
                    addressValue = value;
                }
                
                if (!addressValue) {
                    return;
                }
                
                // 先直接设置到 input.value
                input.value = addressValue;
                
                // 等待 Select2 初始化后，使用标准流程处理（和电话区号一样）
                const setCountryValue = async () => {
                    let retries = 0;
                    while (retries < 50) {
                        if (typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                            // 使用标准的 Select2 处理流程（和电话区号一样）
                            // 检查选项是否存在
                            if ($(input).find(`option[value="${addressValue}"]`).length === 0) {
                                // 选项不存在，通过 API 加载并添加
                                try {
                                    const apiPath = field.relationApi || '/api/v1/countries';
                                    const itemData = await App.apiRequest(`${apiPath}?limit=1000&search=${encodeURIComponent(addressValue)}`);
                                    if (itemData && itemData.data && Array.isArray(itemData.data)) {
                                        const valueKey = field.relationValueKey || 'code';
                                        const foundItem = itemData.data.find(item => {
                                            const itemValue = item[valueKey] || item.code || item.id;
                                            return String(itemValue) === String(addressValue);
                                        });
                                        if (foundItem) {
                                            const labelKey = field.relationLabel || 'name';
                                            const displayText = foundItem[labelKey] || foundItem.name || foundItem.code || String(addressValue);
                                            const newOption = new Option(displayText, addressValue, true, true);
                                            $(input).append(newOption);
                                            $(input).val(addressValue).trigger('change');
                                            
                                            // 国家字段完成后，处理地区字段
                                            await this.setAddressRegionCode(item);
                                            return;
                                        }
                                    }
                                    // 如果搜索不到，尝试直接获取
                                    const singleItem = await App.apiRequest(`${apiPath}/${addressValue}`);
                                    if (singleItem) {
                                        const labelKey = field.relationLabel || 'name';
                                        const displayText = singleItem[labelKey] || singleItem.name || singleItem.code || String(addressValue);
                                        const newOption = new Option(displayText, addressValue, true, true);
                                        $(input).append(newOption);
                                        $(input).val(addressValue).trigger('change');
                                        
                                        // 国家字段完成后，处理地区字段
                                        await this.setAddressRegionCode(item);
                                        return;
                                    }
                                } catch (err) {
                                    console.warn(`无法获取国家选项数据:`, err);
                                }
                                // 即使获取失败，也添加一个基本选项
                                const newOption = new Option(String(addressValue), addressValue, true, true);
                                $(input).append(newOption);
                                $(input).val(addressValue).trigger('change');
                                
                                // 国家字段完成后，处理地区字段
                                await this.setAddressRegionCode(item);
                            } else {
                                // 选项已存在，直接设置值
                                $(input).val(addressValue).trigger('change');
                                
                                // 国家字段完成后，处理地区字段
                                await this.setAddressRegionCode(item);
                            }
                            return;
                        }
                        await new Promise(resolve => setTimeout(resolve, 100));
                        retries++;
                    }
                    // 即使超时，也尝试处理地区字段
                    await this.setAddressRegionCode(item);
                };
                // 将国家字段加载添加到异步 Promise 列表
                asyncPromises.push(setCountryValue());
                return;
            }
            
            // 地区字段单独处理（在国家字段完成后调用）
            if ((this.pageName === 'warehouses' || this.pageName === 'suppliers' || this.pageName === 'stores') && 
                field.key === 'address_region_code') {
                // 地区字段会在国家字段完成后通过 setAddressRegionCode 方法处理
                // 这里先跳过，避免重复处理
                return;
            }
            
            // 處理 Select2 字段（單選和多選）
            if (field.type === 'select2' || field.type === 'select2-multi') {
                const valueKey = field.relationValueKey || field.relationKey || 'id';
                
                if (field.type === 'select2-multi') {
                    // 多選模式：處理數組
                    let ids = [];
                    if (Array.isArray(value)) {
                        ids = value.map(item => {
                            if (typeof item === 'object' && item !== null) {
                                return item.id || item[valueKey] || item;
                            }
                            return item;
                        });
                    } else if (value) {
                        if (typeof value === 'object' && value !== null) {
                            ids = [value.id || value[valueKey]];
                        } else {
                            ids = [value];
                        }
                    }
                    
                    // 設置 Select2 多選的值
                    if (typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible') && ids.length > 0) {
                        // 檢查哪些 ID 缺少 <option> 元素（AJAX 模式下通常都缺少）
                        const missingIds = ids.filter(id => $(input).find(`option[value="${id}"]`).length === 0);
                        
                        if (missingIds.length > 0 && field.relationApi) {
                            // 需要從 API 獲取顯示文本並創建 <option> 元素
                            const loadMultiOptions = async () => {
                                try {
                                    const apiPath = field.relationApi;
                                    const labelKey = field.relationLabelKey || field.relationLabel || 'name';
                                    const rValueKey = field.relationValueKey || field.relationKey || 'id';
                                    const displayFormat = field.relationDisplayFormat;
                                    const labelFields = Array.isArray(field.relationLabelFields) ? field.relationLabelFields : null;
                                    // 批量獲取所有缺失的項目（一次 API 請求，大 limit）
                                    const resp = await App.apiRequest(`${apiPath}?limit=1000`);
                                    const allItems = (resp && resp.data && Array.isArray(resp.data)) ? resp.data : [];
                                    
                                    for (const id of missingIds) {
                                        let displayText = String(id);
                                        const foundItem = allItems.find(item => {
                                            const itemValue = item[rValueKey] || item.id;
                                            return String(itemValue) === String(id);
                                        });
                                        
                                        if (foundItem) {
                                            if (labelFields && labelFields.length > 0) {
                                                const parts = labelFields.map(k => (foundItem && foundItem[k] != null ? String(foundItem[k]).trim() : '')).filter(Boolean);
                                                displayText = parts.join(' - ');
                                            } else if (displayFormat === 'code-name' && foundItem.code && foundItem.name) {
                                                displayText = `${foundItem.code} - ${foundItem.name}`;
                                            } else if (displayFormat && typeof displayFormat === 'function') {
                                                displayText = displayFormat(foundItem);
                                            } else {
                                                displayText = foundItem[labelKey] || foundItem.name || foundItem.code || String(id);
                                            }
                                        } else {
                                            // 項目在列表中找不到，嘗試單獨獲取
                                            try {
                                                const singleItem = await App.apiRequest(`${apiPath}/${id}`);
                                                if (singleItem) {
                                                    if (labelFields && labelFields.length > 0) {
                                                        const parts = labelFields.map(k => (singleItem && singleItem[k] != null ? String(singleItem[k]).trim() : '')).filter(Boolean);
                                                        displayText = parts.join(' - ');
                                                    } else if (displayFormat === 'code-name' && singleItem.code && singleItem.name) {
                                                        displayText = `${singleItem.code} - ${singleItem.name}`;
                                                    } else if (displayFormat && typeof displayFormat === 'function') {
                                                        displayText = displayFormat(singleItem);
                                                    } else {
                                                        displayText = singleItem[labelKey] || singleItem.name || singleItem.code || String(id);
                                                    }
                                                }
                                            } catch (err) {
                                                console.warn(`無法獲取 ${field.key} 的項目 ${id}:`, err);
                                            }
                                        }
                                        
                                        const newOption = new Option(displayText, id, true, true);
                                        $(input).append(newOption);
                                    }
                                    
                                    // 所有選項都已創建，現在設置值
                                    $(input).val(ids.map(String)).trigger('change');
                                } catch (err) {
                                    console.warn(`無法獲取 ${field.key} 的多選選項數據:`, err);
                                    // 即使獲取失敗，也添加基本選項
                                    for (const id of missingIds) {
                                        if ($(input).find(`option[value="${id}"]`).length === 0) {
                                            const newOption = new Option(String(id), id, true, true);
                                            $(input).append(newOption);
                                        }
                                    }
                                    $(input).val(ids.map(String)).trigger('change');
                                }
                            };
                            // 將異步操作添加到 Promise 列表
                            asyncPromises.push(loadMultiOptions());
                        } else if (missingIds.length > 0) {
                            // 沒有 relationApi（固定選項模式），直接添加基本選項
                            for (const id of missingIds) {
                                const newOption = new Option(String(id), id, true, true);
                                $(input).append(newOption);
                            }
                            $(input).val(ids.map(String)).trigger('change');
                        } else {
                            // 所有選項都已存在，直接設置值
                            $(input).val(ids.map(String)).trigger('change');
                        }
                    } else if (typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                        // ids 為空，清空選擇
                        $(input).val([]).trigger('change');
                    } else {
                        // 如果 Select2 還沒初始化，先設置值
                        Array.from(input.options).forEach(option => {
                            option.selected = ids.includes(option.value);
                        });
                    }
                } else {
                    // 單選模式
                    if (typeof value === 'object' && value !== null) {
                        value = value[valueKey] || value.id;
                    }
                    if (value === null || value === undefined) {
                        value = '';
                    }
                    value = String(value);
                    
                    // 如果 Select2 已初始化，需要確保選項存在
                    if (typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible') && value) {
                        // 檢查選項是否存在
                        if ($(input).find(`option[value="${value}"]`).length === 0) {
                            // 選項不存在，需要從 API 獲取並添加
                            const apiPath = field.relationApi;
                            if (apiPath) {
                                try {
                                    // 使用 Promise 處理異步操作
                                    const loadOption = async () => {
                                        try {
                                            // 如果是 address_region_code 字段，需要检查是否有 country_code
                                            if (field.key === 'address_region_code' && field.relationApi && field.relationApi.includes('country-regions')) {
                                                const countryFieldId = 'field_address_country_code';
                                                const countryField = document.getElementById(countryFieldId);
                                                let countryCode = '';
                                                if (countryField) {
                                                    if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                                                        countryCode = $(countryField).val() || '';
                                                    } else {
                                                        countryCode = countryField.value || '';
                                                    }
                                                }
                                                // 如果没有 country_code，不加载 region
                                                if (!countryCode) {
                                                    const newOption = new Option(String(value), value, true, true);
                                                    $(input).append(newOption);
                                                    $(input).val(value).trigger('change');
                                                    return;
                                                }
                                                // 有 country_code，在 API 请求中添加
                                                const itemData = await App.apiRequest(`${apiPath}?country_code=${countryCode}&limit=1000&search=${encodeURIComponent(value)}`);
                                                if (itemData && itemData.data && Array.isArray(itemData.data)) {
                                                    const foundItem = itemData.data.find(item => {
                                                        const itemValue = item[valueKey] || item.code || item.id;
                                                        return String(itemValue) === String(value);
                                                    });
                                                    if (foundItem) {
                                                        const labelKey = field.relationLabel || 'name';
                                                        const displayText = foundItem[labelKey] || foundItem.name || foundItem.code || String(value);
                                                        const newOption = new Option(displayText, value, true, true);
                                                        $(input).append(newOption);
                                                        $(input).val(value).trigger('change');
                                                        return;
                                                    }
                                                }
                                            } else {
                                                const itemData = await App.apiRequest(`${apiPath}?limit=1000&search=${encodeURIComponent(value)}`);
                                                if (itemData && itemData.data && Array.isArray(itemData.data)) {
                                                    const foundItem = itemData.data.find(item => {
                                                        const itemValue = item[valueKey] || item.id;
                                                        return String(itemValue) === String(value);
                                                    });
                                                    if (foundItem) {
                                                        const labelKey = field.relationLabel || 'name';
                                                        const displayText = foundItem[labelKey] || foundItem.name || foundItem.code || String(value);
                                                        const newOption = new Option(displayText, value, true, true);
                                                        $(input).append(newOption);
                                                        $(input).val(value).trigger('change');
                                                        return;
                                                    }
                                                }
                                                // 如果搜索不到，嘗試直接獲取該 ID 的數據
                                                const singleItem = await App.apiRequest(`${apiPath}/${value}`);
                                                if (singleItem) {
                                                    const labelKey = field.relationLabel || 'name';
                                                    const displayText = singleItem[labelKey] || singleItem.name || singleItem.code || String(value);
                                                    const newOption = new Option(displayText, value, true, true);
                                                    $(input).append(newOption);
                                                    $(input).val(value).trigger('change');
                                                    return;
                                                }
                                            }
                                        } catch (err) {
                                            console.warn(`無法獲取 ${field.key} 的選項數據:`, err);
                                        }
                                        // 即使獲取失敗，也添加一個基本選項
                                        const newOption = new Option(String(value), value, true, true);
                                        $(input).append(newOption);
                                        $(input).val(value).trigger('change');
                                    };
                                    // 異步加載選項，但不等待完成
                                    loadOption();
                                } catch (err) {
                                    console.warn(`無法獲取 ${field.key} 的選項數據:`, err);
                                    // 即使獲取失敗，也添加一個基本選項
                                    const newOption = new Option(String(value), value, true, true);
                                    $(input).append(newOption);
                                    $(input).val(value).trigger('change');
                                }
                            } else {
                                // 沒有 relationApi，直接添加選項
                                const newOption = new Option(String(value), value, true, true);
                                $(input).append(newOption);
                                $(input).val(value).trigger('change');
                            }
                        } else {
                            // 選項已存在，直接設置值
                            $(input).val(value).trigger('change');
                        }
                    } else if (typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                        // Select2 已初始化，選項已存在
                        $(input).val(value).trigger('change');
                    } else {
                        // 如果 Select2 還沒初始化，先設置值
                        input.value = value;
                    }
                }
                return;
            }
            
            // 處理 relation select（例如 supplier_id, member_level_id）
            if ((field.type === 'select' && field.relationApi) || field.type === 'relation-select') {
                // 如果是 relation，可能需要從關聯對象獲取 ID
                const valueKey = field.relationValueKey || field.relationKey || 'id';
                if (typeof value === 'object' && value !== null) {
                    value = value[valueKey] || value.id;
                }
                // 如果 value 是 null 或 undefined，設置為空字符串
                if (value === null || value === undefined) {
                    value = '';
                }
                // 確保值是字符串格式（因為 select 的 value 是字符串）
                value = String(value);
            }
            
            if (field.type === 'time' && value) {
                // 处理时间格式：从后端返回的 SQLTime（字符串格式，如 "09:00:00" 或 "09:00"）转换为 HH:MM 格式
                if (typeof value === 'string') {
                    // SQLTime 已经是字符串格式，提取前5个字符（HH:MM）
                    if (value.includes(':')) {
                        value = value.slice(0, 5); // 取 HH:MM，去掉秒数部分
                    }
                } else if (value instanceof Date) {
                    // 如果是 Date 对象（向后兼容），转换为 HH:MM
                    const hours = String(value.getHours()).padStart(2, '0');
                    const minutes = String(value.getMinutes()).padStart(2, '0');
                    value = `${hours}:${minutes}`;
                }
            }
            
            if ((field.type === 'date' || field.type === 'datetime-local') && value) {
                // 处理日期时间格式
                if (typeof value === 'string') {
                    // 如果是 ISO 格式字符串，转换为本地时间
                    const date = new Date(value);
                    if (!isNaN(date.getTime())) {
                        const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
                        if (field.type === 'date') {
                            value = localDate.toISOString().slice(0, 10);
                        } else {
                            value = localDate.toISOString().slice(0, 16);
                        }
                    }
                } else if (field.type === 'date') {
                    value = formatDateForInput(value);
                }
            }
            
            if (input.type === 'checkbox') {
                input.checked = value;
            } else if (field.key === 'code' && this.pageName === 'phone-country-codes') {
                // 電話區號表單：填充 code 時自動填充 name
                input.value = value || '';
                // 觸發 change 事件以自動填充名稱
                if (value && typeof COUNTRY_PHONE_CODES !== 'undefined' && COUNTRY_PHONE_CODES[value]) {
                    const nameInput = document.getElementById('field_name');
                    if (nameInput) {
                        const englishName = COUNTRY_PHONE_CODES[value];
                        nameInput.dataset.submitValue = englishName;
                        if (typeof I18n !== 'undefined' && I18n.t) {
                            const k = `phoneCountryCodes.names.${value}`;
                            const translated = I18n.t(k);
                            nameInput.value = (translated && translated !== k) ? translated : englishName;
                        } else {
                            nameInput.value = englishName;
                        }
                    }
                }
                return;
            } else if (field.key === 'phone' && (this.pageName === 'customers' || this.pageName === 'suppliers' || this.pageName === 'warehouses')) {
                // 拆分區號與電話
                const phoneVal = String(value || '');
                const ccSelect = document.getElementById('field_phone_country_code');
                // 優先使用 formData 中的 phone_country_code
                if (this.formData && this.formData.phone_country_code && ccSelect) {
                    const phoneCode = this.formData.phone_country_code;
                    // 如果是 Select2，需要正確設置
                    if (typeof $ !== 'undefined' && $(ccSelect).hasClass('select2-hidden-accessible')) {
                        // 如果選項不存在，先添加
                        if ($(ccSelect).find(`option[value="${phoneCode}"]`).length === 0) {
                            const newOption = new Option(phoneCode, phoneCode, true, true);
                            $(ccSelect).append(newOption);
                        }
                        $(ccSelect).val(phoneCode).trigger('change');
                    } else {
                        ccSelect.value = phoneCode;
                    }
                }
                const parts = phoneVal.trim().split(' ');
                if (parts.length > 1 && parts[0].startsWith('+')) {
                    if (ccSelect && !ccSelect.value) {
                        const phoneCode = parts[0];
                        // 如果是 Select2，需要正確設置
                        if (typeof $ !== 'undefined' && $(ccSelect).hasClass('select2-hidden-accessible')) {
                            if ($(ccSelect).find(`option[value="${phoneCode}"]`).length === 0) {
                                const newOption = new Option(phoneCode, phoneCode, true, true);
                                $(ccSelect).append(newOption);
                            }
                            $(ccSelect).val(phoneCode).trigger('change');
                        } else {
                            ccSelect.value = phoneCode;
                        }
                    }
                    input.value = parts.slice(1).join(' ');
                } else {
                    input.value = phoneVal;
                }
            } else if (field.type === 'select' && (field.key === 'auto_upgrade' || field.key === 'auto_apply_discount')) {
                // 處理 auto_upgrade 字段：將布爾值轉換為 'true'/'false' 字符串
                input.value = value === true ? 'true' : (value === false ? 'false' : '');
            } else if (field.type === 'select' && field.key === 'is_required') {
                // 處理 is_required 字段：將布爾值轉換為 'yes'/'no' 字符串
                if (value === true || value === 'true' || value === 'yes') {
                    input.value = 'yes';
                } else if (value === false || value === 'false' || value === 'no') {
                    input.value = 'no';
                } else {
                    input.value = field.defaultValue || 'no';
                }
            } else if (field.type === 'select' && (field.key === 'is_service_package' || field.key === 'allow_backorder')) {
                // 處理 is_service_package 和 allow_backorder 字段：將布爾值轉換為 'true'/'false' 字符串
                if (value === true || value === 'true' || value === 1 || value === '1') {
                    input.value = 'true';
                } else if (value === false || value === 'false' || value === 'no' || value === 0 || value === '0') {
                    input.value = 'false';
                } else {
                    input.value = field.defaultValue || 'false';
                }
            } else if (field.type === 'select' && (field.key === 'requires_shipping' || field.key === 'is_default' || field.key === 'is_default_receiving' || field.key === 'is_default_payment')) {
                // 處理 requires_shipping、is_default、is_default_receiving 和 is_default_payment 字段：將布爾值轉換為 'true'/'false' 字符串
                if (value === true || value === 'true' || value === 1 || value === '1') {
                    input.value = 'true';
                } else if (value === false || value === 'false' || value === 0 || value === '0') {
                    input.value = 'false';
                } else {
                    input.value = field.defaultValue || 'false';
                }
            } else if (field.type === 'select' && field.key === 'all_day') {
                // 處理 all_day 字段：將布爾值轉換為 'yes'/'no' 字符串
                if (value === true || value === 'true' || value === 1 || value === '1') {
                    input.value = 'yes';
                } else if (value === false || value === 'false' || value === 0 || value === '0' || value === 'no') {
                    input.value = 'no';
                } else {
                    input.value = field.defaultValue || 'no';
                }
            } else if (field.type === 'profile-image') {
                // Profile image 字段：設置預覽和隱藏字段
                const urlInput = document.getElementById(`${fieldId}_url`);
                const preview = document.getElementById(`${fieldId}_preview`);
                const placeholder = document.getElementById(`${fieldId}_placeholder`);
                const removeBtn = document.getElementById(`${fieldId}_removeBtn`);
                
                // 確保 URL 是完整的路徑
                let imageUrl = value || '';
                if (imageUrl && !imageUrl.startsWith('http') && !imageUrl.startsWith('/')) {
                    imageUrl = '/' + imageUrl;
                }
                
                if (imageUrl && urlInput) {
                    urlInput.value = imageUrl;
                }
                if (imageUrl && preview) {
                    preview.src = imageUrl;
                    preview.style.display = 'block';
                    // 添加點擊事件以放大圖片
                    preview.style.cursor = 'pointer';
                    preview.onclick = () => {
                        this.showImageLightbox(imageUrl);
                    };
                    preview.onerror = function() {
                        // 如果圖片加載失敗，顯示占位符
                        this.style.display = 'none';
                        this.setAttribute('data-error-handled', 'true');
                        if (placeholder) {
                            placeholder.style.display = 'block';
                        }
                        if (removeBtn) {
                            removeBtn.style.display = 'none';
                        }
                        this.onerror = null; // 防止重複觸發
                        console.warn('Failed to load profile picture:', imageUrl);
                    };
                }
                if (imageUrl && placeholder) {
                    placeholder.style.display = 'none';
                }
                if (imageUrl && removeBtn) {
                    removeBtn.style.display = 'inline-block';
                }
                if (!imageUrl && preview) {
                    preview.src = '';
                    preview.style.display = 'none';
                }
                if (!imageUrl && placeholder) {
                    placeholder.style.display = 'block';
                }
                if (!imageUrl && removeBtn) {
                    removeBtn.style.display = 'none';
                }
            } else if (field.type === 'html-editor') {
                // HTML 編輯器：設置 Quill 編輯器的內容
                const editorId = `${fieldId}_editor`;
                const editorElement = document.getElementById(editorId);
                const textarea = document.getElementById(fieldId);
                
                if (editorElement && editorElement.quill) {
                    // Quill 已初始化，直接設置內容
                    const htmlValue = value || '';
                    editorElement.quill.root.innerHTML = htmlValue;
                    if (textarea) {
                        textarea.value = htmlValue;
                    }
                } else if (textarea) {
                    // Quill 還未初始化，先設置到 textarea，等待初始化
                    textarea.value = value || '';
                    // 等待 Quill 初始化完成後再設置
                    setTimeout(() => {
                        if (editorElement && editorElement.quill) {
                            editorElement.quill.root.innerHTML = value || '';
                        }
                    }, 100);
                }
                return;
            } else if (field.type === 'color') {
                // 顏色字段：同步 color picker 和 text input
                if (input) {
                    input.value = value || field.default || '#007bff';
                    const textInput = document.getElementById(`${fieldId}_text`);
                    if (textInput) {
                        textInput.value = value || field.default || '#007bff';
                    // 監聽 color picker 變化，同步到 text input
                    input.addEventListener('input', function() {
                            textInput.value = this.value;
                    });
                    // 監聽 text input 變化，同步到 color picker
                        textInput.addEventListener('input', function() {
                            if (/^#[0-9A-F]{6}$/i.test(this.value)) {
                                input.value = this.value;
                            }
                        });
                    }
                }
                return;
            } else if (field.type === 'file' && (field.key === 'image_url' || field.key === 'logo_url') && 
                       (this.pageName === 'products' || this.pageName === 'vehicles' || this.pageName === 'equipments' || 
                        this.pageName === 'services' || this.pageName === 'projects' || this.pageName === 'stores' || 
                        this.pageName === 'rooms' || this.pageName === 'brands')) {
                // image_url/logo_url 字段（products, vehicles, equipments, services, projects, stores, rooms, brands）：設置預覽和隱藏字段
                const urlInput = document.getElementById(`${fieldId}_url`);
                const preview = document.getElementById(`${fieldId}_preview`);
                const placeholder = document.getElementById(`${fieldId}_placeholder`);
                const removeBtn = document.getElementById(`${fieldId}_removeBtn`);
                
                // 確保 URL 是完整的路徑
                let imageUrl = value || '';
                if (imageUrl && !imageUrl.startsWith('http') && !imageUrl.startsWith('/')) {
                    imageUrl = '/' + imageUrl;
                }
                
                if (imageUrl && urlInput) {
                    urlInput.value = imageUrl;
                }
                if (imageUrl && preview) {
                    preview.src = imageUrl;
                    preview.style.display = 'block';
                    // 添加點擊事件以放大圖片
                    preview.style.cursor = 'pointer';
                    preview.onclick = () => {
                        this.showImageLightbox(imageUrl);
                    };
                    preview.onerror = function() {
                        // 如果圖片加載失敗，顯示占位符
                        this.style.display = 'none';
                        this.setAttribute('data-error-handled', 'true');
                        if (placeholder) {
                            placeholder.style.display = 'block';
                        }
                        if (removeBtn) {
                            removeBtn.style.display = 'none';
                        }
                        this.onerror = null; // 防止重複觸發
                        console.warn('Failed to load image:', imageUrl);
                    };
                }
                if (imageUrl && placeholder) {
                    placeholder.style.display = 'none';
                }
                if (imageUrl && removeBtn) {
                    removeBtn.style.display = 'inline-block';
                }
                if (!imageUrl && preview) {
                    preview.src = '';
                    preview.style.display = 'none';
                }
                if (!imageUrl && placeholder) {
                    placeholder.style.display = 'block';
                }
                if (!imageUrl && removeBtn) {
                    removeBtn.style.display = 'none';
                }
                return;
            } else if (field.type === 'file' && field.key === 'image_url') {
                // 文件字段：顯示現有圖片（如果有）
                const preview = document.getElementById('imagePreview');
                const previewImg = document.getElementById('previewImg');
                if (value && typeof value === 'string' && value.trim() !== '') {
                    // 保存原始圖片 URL（編輯時使用）
                    this.originalImageUrl = value;
                    if (preview && previewImg) {
                        previewImg.src = value;
                        preview.style.display = 'inline-block';
                        // 添加點擊事件以放大圖片
                        previewImg.style.cursor = 'pointer';
                        previewImg.onclick = () => {
                            this.showImageLightbox(value);
                        };
                    }
                } else {
                    // 沒有圖片時，完全隱藏 imagePreview
                    if (preview) {
                        preview.style.display = 'none';
                    }
                    if (previewImg) {
                        // 移除 src 屬性而不是設置為空字符串，避免觸發錯誤
                        previewImg.removeAttribute('src');
                        previewImg.setAttribute('data-error-handled', 'true');
                        previewImg.onclick = null;
                    }
                    this.originalImageUrl = null;
                }
            } else if (field.type === 'button-group') {
                // button-group 類型：設置選中的 radio button
                const value = this.getNestedValue(item, field.key);
                if (value && input) {
                    const radioId = `${fieldId}_${value}`;
                    const radio = document.getElementById(radioId);
                    if (radio) {
                        radio.checked = true;
                        input.value = value;
                        // 更新按鈕的 active 狀態
                        const group = document.getElementById(`${fieldId}_group`);
                        if (group) {
                            group.querySelectorAll('label').forEach(label => label.classList.remove('active'));
                            const label = document.querySelector(`label[for="${radioId}"]`);
                            if (label) label.classList.add('active');
                        }
                    }
                }
            } else if (field.key === 'payment_type' && this.pageName === 'payment-methods') {
                // Payment methods: 从 extra_fields 加载 payment_type
                let paymentTypeValue = value;
                if (!paymentTypeValue && item.extra_fields && item.extra_fields.payment_type) {
                    paymentTypeValue = item.extra_fields.payment_type;
                }
                if (paymentTypeValue && input) {
                    input.value = paymentTypeValue;
                }
                // 设置付款形式后触发处理
                setTimeout(() => {
                    this.handlePaymentTypeChange();
                }, 100);
            } else if (this.pageName === 'payment-methods' && (field.key === 'stripe_api_key' || field.key === 'stripe_secret_key' || field.key === 'paypal_client_id' || field.key === 'paypal_secret' || field.key === 'qfpay_app_code' || field.key === 'qfpay_client_key' || field.key === 'qfpay_base_url' || field.key === 'currency')) {
                // Payment methods: 从 extra_fields 加载网关连接字段
                let gatewayValue = value;
                if (!gatewayValue && item.extra_fields && item.extra_fields[field.key]) {
                    gatewayValue = item.extra_fields[field.key];
                }
                if (gatewayValue && input) {
                    input.value = gatewayValue;
                }
            } else {
                // 特殊處理：options 字段（產品屬性的選項，用逗號分隔）
                if (field.key === 'options' && value) {
                    if (Array.isArray(value)) {
                        // 如果是數組，轉換為逗號分隔的字符串
                        input.value = value.join(', ');
                    } else if (typeof value === 'string') {
                        // 如果已經是字符串，直接使用
                        input.value = value;
                    } else {
                        // 其他類型，轉換為字符串
                        input.value = String(value);
                }
            } else {
                // 如果沒有值且有默認值，使用默認值（支持 default 和 defaultValue）
                const defaultValue = field.default !== undefined ? field.default : (field.defaultValue !== undefined ? field.defaultValue : undefined);
                // 對於 order_commission_mode 和 service_order_commission_mode，即使在編輯模式下，如果值為空也應用默認值
                const shouldApplyDefault = (value === null || value === undefined || value === '') && defaultValue !== undefined && 
                    (!this.isEdit || field.key === 'order_commission_mode' || field.key === 'service_order_commission_mode');
                if (shouldApplyDefault) {
                    input.value = defaultValue;
                } else {
                    input.value = value || '';
                    }
                }
            }
        });
        
        // ── 填充額外欄位（extra fields from field settings）──
        // 編輯時，將 item.extra_fields 中的值填入對應的額外欄位 DOM 元素
        if (this.fieldSettings && this.fieldSettings.extraFields && this.fieldSettings.extraFields.length > 0 && item && item.extra_fields) {
            const baseFieldKeys = new Set(this.config.formFields.map(f => f.key));
            const extraFieldsToPopulate = this.fieldSettings.extraFields.filter(ef => !baseFieldKeys.has(ef.key));
            
            extraFieldsToPopulate.forEach(ef => {
                const fieldId = `field_${ef.key}`;
                const input = document.getElementById(fieldId);
                if (!input) return;
                
                const value = item.extra_fields[ef.key];
                if (value === undefined || value === null) return;
                
                if (ef.type === 'checkbox') {
                    input.checked = value === true || value === 'true' || value === 1 || value === '1';
                } else if (ef.type === 'select' && typeof $ !== 'undefined' && $(input).hasClass('select2-hidden-accessible')) {
                    // Select2 field
                    $(input).val(String(value)).trigger('change');
                } else if (ef.type === 'select') {
                    input.value = String(value);
                } else if (ef.type === 'number') {
                    input.value = value;
                } else if (ef.type === 'date' && value) {
                    // Handle date format: ensure it's YYYY-MM-DD
                    if (typeof value === 'string') {
                        const date = new Date(value);
                        if (!isNaN(date.getTime())) {
                            const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
                            input.value = localDate.toISOString().slice(0, 10);
                        } else {
                            input.value = value;
                        }
                    } else {
                        input.value = value;
                    }
                } else if (ef.type === 'datetime-local' && value) {
                    // Handle datetime format: ensure it's YYYY-MM-DDTHH:MM
                    if (typeof value === 'string') {
                        const date = new Date(value);
                        if (!isNaN(date.getTime())) {
                            const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
                            input.value = localDate.toISOString().slice(0, 16);
                        } else {
                            input.value = value;
                        }
                    } else {
                        input.value = value;
                    }
                } else {
                    // text, textarea, etc.
                    input.value = String(value);
                }
            });
        }
        
        // 設置依賴字段的顯示/隱藏邏輯
        this.setupDependencies();
        
        // 應用依賴字段的顯示/隱藏（在數據填充後）
        setTimeout(() => {
            this.applyFieldDependencies();
        }, 100);
        
        // 特殊處理：users 頁面載入後，再次確保佣金相關字段正確顯示/隱藏
        if (this.pageName === 'users') {
            setTimeout(() => {
                this.applyFieldDependencies();
            }, 300);
        }
        
        // 如果是推廣表單且是新表單，設置狀態默認值
        if (this.pageName === 'promotions' && !this.isEdit) {
            const statusSelect = document.getElementById('field_status');
            if (statusSelect && field.default !== undefined) {
                const statusField = this.config.formFields.find(f => f.key === 'status');
                if (statusField && statusField.default) {
                    statusSelect.value = statusField.default;
                }
            }
        }
        
        // 填充完成後，更新條碼和 QR Code 預覽按鈕狀態
        setTimeout(() => {
            if (typeof updateBarcodePreview === 'function') {
                updateBarcodePreview('field_barcode');
            }
            if (typeof updateQRCodePreview === 'function') {
                updateQRCodePreview('field_sku');
            }
            // 初始化所有 readonly 和 disabled 欄位的 tooltip
            this.initTooltips();
            // 初始化 button-group 字段的事件綁定
            this.initButtonGroups();
        }, 100);
        
        // 等待所有异步操作完成（包括国家/地区字段的加载）
        if (asyncPromises.length > 0) {
            await Promise.all(asyncPromises);
            // 额外等待一小段时间，确保所有 DOM 更新完成
            await new Promise(resolve => setTimeout(resolve, 200));
        }
    }

    // 初始化 button-group 字段的事件綁定
    initButtonGroups() {
        this.config.formFields.forEach(field => {
            if (field.type === 'button-group') {
                const fieldId = field._uniqueId || `field_${field.key}`;
                const radioButtons = document.querySelectorAll(`input[name="${fieldId}_radio"]`);
                const hiddenInput = document.getElementById(fieldId);
                
                if (radioButtons.length > 0 && hiddenInput) {
                    radioButtons.forEach(radio => {
                        // 移除舊的事件監聽器（如果有的話）
                        const newRadio = radio.cloneNode(true);
                        radio.parentNode.replaceChild(newRadio, radio);
                        
                        newRadio.addEventListener('change', function() {
                            if (this.checked && hiddenInput) {
                                hiddenInput.value = this.value;
                                // 更新按鈕的 active 狀態
                                const group = document.getElementById(`${fieldId}_group`);
                                if (group) {
                                    group.querySelectorAll('label').forEach(label => label.classList.remove('active'));
                                    const label = document.querySelector(`label[for="${this.id}"]`);
                                    if (label) label.classList.add('active');
                                }
                            }
                        });
                    });
                }
            }
        });
    }

    // 初始化所有 readonly 和 disabled 欄位的 tooltip
    initTooltips() {
        if (typeof bootstrap === 'undefined' || !bootstrap.Tooltip) {
            return;
        }
        
        // 在整個文檔中查找所有 readonly 和 disabled 字段
        const readonlyElements = document.querySelectorAll('[readonly], [disabled]');
        readonlyElements.forEach(el => {
            // 跳過按鈕和不需要 tooltip 的元素
            if (el.type === 'button' || el.tagName === 'BUTTON' || el.tagName === 'LABEL') {
                return;
            }
            // 跳過 button-group 中的 radio button 和 label
            if (el.closest('.btn-group') || el.closest('[role="group"]')) {
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
    
    // 設置依賴字段的顯示/隱藏邏輯
    setupDependencies() {
        this.config.formFields.forEach(field => {
            if (field.dependency) {
                const dependencyField = document.getElementById(`field_${field.dependency.field}`);
                const dependentFieldContainer = document.getElementById(`field_container_${field.key}`);
                
                if (dependencyField && dependentFieldContainer) {
                    const checkDependency = async () => {
                        const dependencyValue = dependencyField.type === 'checkbox' ? String(dependencyField.checked) : dependencyField.value;
                        // 支持單個值或多個值（數組）
                        const allowedValues = Array.isArray(field.dependency.values) ? field.dependency.values : [field.dependency.value];
                        const shouldShow = allowedValues.includes(dependencyValue);
                        
                        if (shouldShow) {
                            // 顯示字段
                            dependentFieldContainer.style.display = 'block';
                            dependentFieldContainer.style.visibility = 'visible';
                            dependentFieldContainer.style.opacity = '1';
                            
                            // 如果字段顯示後，確保 Select2 已初始化（僅對 select2 類型）
                            const dependentField = document.getElementById(`field_${field.key}`);
                            if (dependentField && (field.type === 'select2' || field.type === 'select2-multi')) {
                                // 等待一下確保 DOM 已更新且容器已顯示
                                setTimeout(async () => {
                                    try {
                                        // 確保容器已顯示
                                        if (dependentFieldContainer.style.display === 'none') {
                                            dependentFieldContainer.style.display = 'block';
                                        }
                                        
                                        // 先載入選項（如果還沒有載入）
                                        if (field.relationApi && dependentField.options.length <= 1) {
                                            await this.loadRelationFieldOptions(field);
                                        }
                                        
                                        // 檢查是否已初始化 Select2
                                        if (typeof $ !== 'undefined' && $(dependentField).hasClass('select2-hidden-accessible')) {
                                            // 已經初始化，確保容器可見
                                            const select2Container = $(dependentField).next('.select2-container');
                                            if (select2Container.length > 0) {
                                                select2Container.css({
                                                    'display': 'block',
                                                    'visibility': 'visible',
                                                    'opacity': '1'
                                                });
        }
                                        } else {
                                            // 尚未初始化，初始化 Select2
                                            if (field.relationApi) {
                                                await this.loadRelationFieldOptions(field);
                                            }
                                            await this.initSelect2(field);
                                            
                                            // 確保 Select2 容器可見
                                            setTimeout(() => {
                                                const select2Container = $(dependentField).next('.select2-container');
                                                if (select2Container.length > 0) {
                                                    select2Container.css({
                                                        'display': 'block',
                                                        'visibility': 'visible',
                                                        'opacity': '1',
                                                        'width': '100%'
                                                    });
                                                }
                                            }, 100);
                                        }
                                    } catch (error) {
                                        console.error(`初始化依赖字段 Select2 失败 (${field.key}):`, error);
                                    }
                                }, 200);
                            }
                        } else {
                            dependentFieldContainer.style.display = 'none';
                            // 清空依賴字段的值
                            const dependentField = document.getElementById(`field_${field.key}`);
                            if (dependentField) {
                                // 如果是 Select2，需要清除
                                if (typeof $ !== 'undefined' && $(dependentField).hasClass('select2-hidden-accessible')) {
                                    $(dependentField).val(null).trigger('change');
                                } else {
                                    dependentField.value = '';
                                }
                            }
                        }
                    };
                    
                    // 初始檢查
                    checkDependency();
                    
                    // 監聽依賴字段的變化
                    dependencyField.addEventListener('change', checkDependency);
                }
            }
        });
        
        // 特殊處理：incomes 頁面的自動生成標題
        if (this.pageName === 'incomes') {
            const descriptionField = document.getElementById('field_description');
            const categoryField = document.getElementById('field_category');
            const paymentMethodField = document.getElementById('field_payment_method');
            const referenceNumberField = document.getElementById('field_reference_number');
            
            const updateDescription = async () => {
                if (!descriptionField || !categoryField) return;
                
                const category = categoryField.value;
                let description = '';
                
                // 根據類別獲取對應的 reference_id 字段
                let referenceIdField = null;
                if (category === 'order') {
                    referenceIdField = document.getElementById('field_reference_id_order');
                } else if (category === 'service_order') {
                    referenceIdField = document.getElementById('field_reference_id_service_order');
                }
                
                if (category === 'order' && referenceIdField) {
                    const orderId = typeof $ !== 'undefined' && $(referenceIdField).hasClass('select2-hidden-accessible') 
                        ? $(referenceIdField).val() 
                        : referenceIdField.value;
                    
                    if (orderId) {
                        try {
                            const order = await App.apiRequest(`/orders/${orderId}`);
                            const invoiceNumber = referenceNumberField ? referenceNumberField.value : '';
                            const orderNumber = order.order_number || '';
                            
                            if (invoiceNumber) {
                                description = `${invoiceNumber} (${orderNumber})`;
                            } else {
                                description = `訂單 ${orderNumber}`;
                            }
                        } catch (error) {
                            console.error('載入訂單失敗:', error);
                        }
                    }
                } else if (category === 'service_order' && referenceIdField) {
                    const serviceOrderId = typeof $ !== 'undefined' && $(referenceIdField).hasClass('select2-hidden-accessible') 
                        ? $(referenceIdField).val() 
                        : referenceIdField.value;
                    
                    if (serviceOrderId) {
                        try {
                            const serviceOrder = await App.apiRequest(`/service-orders/${serviceOrderId}`);
                            const invoiceNumber = referenceNumberField ? referenceNumberField.value : '';
                            const serviceOrderNumber = serviceOrder.order_number || '';
                            
                            if (invoiceNumber) {
                                description = `${invoiceNumber} (${serviceOrderNumber})`;
                            } else {
                                description = `服務單 ${serviceOrderNumber}`;
                            }
                        } catch (error) {
                            console.error('載入服務單失敗:', error);
                        }
                    }
                }
                
                if (description && descriptionField.value === '') {
                    descriptionField.value = description;
                }
            };
            
            // 監聽相關字段變化
            if (categoryField) {
                categoryField.addEventListener('change', () => {
                    updateDescription();
                    // 觸發依賴字段的顯示/隱藏
                    this.applyFieldDependencies();
                });
        }
        
            // 監聽兩個 reference_id 字段的變化
            const orderReferenceField = document.getElementById('field_reference_id_order');
            const serviceOrderReferenceField = document.getElementById('field_reference_id_service_order');
                    
            if (orderReferenceField) {
                orderReferenceField.addEventListener('change', updateDescription);
                if (typeof $ !== 'undefined' && $(orderReferenceField).hasClass('select2-hidden-accessible')) {
                    $(orderReferenceField).on('select2:select', updateDescription);
                }
            }
            if (serviceOrderReferenceField) {
                serviceOrderReferenceField.addEventListener('change', updateDescription);
                if (typeof $ !== 'undefined' && $(serviceOrderReferenceField).hasClass('select2-hidden-accessible')) {
                    $(serviceOrderReferenceField).on('select2:select', updateDescription);
                    }
                }
                
                if (referenceNumberField) {
                    referenceNumberField.addEventListener('input', updateDescription);
            }
        }
    }

    // 处理地区字段（在国家字段完成后调用）
    async setAddressRegionCode(item) {
        if (this.pageName !== 'warehouses' && this.pageName !== 'suppliers' && this.pageName !== 'stores') {
            return;
        }
        
        const regionInput = document.getElementById('field_address_region_code');
        if (!regionInput) {
            return;
        }
        
        // 获取地区字段配置
        const regionField = this.config.formFields.find(f => f.key === 'address_region_code');
        if (!regionField) {
            return;
        }
        
        // 获取地区值
        let regionValue = null;
        if (item.extra_fields && item.extra_fields.address_region_code) {
            regionValue = item.extra_fields.address_region_code;
        }
        
        if (!regionValue) {
            return;
        }
        
        // 先设置到 input.value
        regionInput.value = regionValue;
        
        // 获取国家代码
        let countryCode = '';
        const countryField = document.getElementById('field_address_country_code');
        if (countryField) {
            if (typeof $ !== 'undefined' && $(countryField).hasClass('select2-hidden-accessible')) {
                countryCode = $(countryField).val() || '';
            } else {
                countryCode = countryField.value || '';
            }
        }
        
        // 如果国家代码还没有，从 extra_fields 获取
        if (!countryCode && item.extra_fields && item.extra_fields.address_country_code) {
            countryCode = item.extra_fields.address_country_code;
        }
        
        if (!countryCode) {
            return;
        }
        
        // 等待 Select2 初始化后，使用标准流程处理（和电话区号一样）
        let retries = 0;
        while (retries < 50) {
            if (typeof $ !== 'undefined' && $(regionInput).hasClass('select2-hidden-accessible')) {
                // 确保地区字段已启用（如果有国家代码）
                if (countryCode) {
                    $(regionInput).prop('disabled', false);
                }
                
                // 使用标准的 Select2 处理流程（和电话区号一样）
                // 检查选项是否存在
                if ($(regionInput).find(`option[value="${regionValue}"]`).length === 0) {
                    // 选项不存在，通过 API 加载并添加
                    try {
                        const apiPath = regionField.relationApi || '/api/v1/country-regions';
                        const itemData = await App.apiRequest(`${apiPath}?country_code=${countryCode}&limit=1000&search=${encodeURIComponent(regionValue)}`);
                        if (itemData && itemData.data && Array.isArray(itemData.data)) {
                            const valueKey = regionField.relationValueKey || 'code';
                            const foundItem = itemData.data.find(item => {
                                const itemValue = item[valueKey] || item.code || item.id;
                                return String(itemValue) === String(regionValue);
                            });
                            if (foundItem) {
                                const labelKey = regionField.relationLabel || 'name';
                                const displayText = foundItem[labelKey] || foundItem.name || foundItem.code || String(regionValue);
                                const newOption = new Option(displayText, regionValue, true, true);
                                $(regionInput).append(newOption);
                                $(regionInput).val(regionValue).trigger('change');
                                return;
                            }
                        }
                        // 如果搜索不到，尝试直接获取
                        const singleItem = await App.apiRequest(`${apiPath}/${regionValue}`);
                        if (singleItem) {
                            const labelKey = regionField.relationLabel || 'name';
                            const displayText = singleItem[labelKey] || singleItem.name || singleItem.code || String(regionValue);
                            const newOption = new Option(displayText, regionValue, true, true);
                            $(regionInput).append(newOption);
                            $(regionInput).val(regionValue).trigger('change');
                            return;
                        }
                    } catch (err) {
                        console.warn(`无法获取地区选项数据:`, err);
                    }
                    // 即使获取失败，也添加一个基本选项
                    const newOption = new Option(String(regionValue), regionValue, true, true);
                    $(regionInput).append(newOption);
                    $(regionInput).val(regionValue).trigger('change');
                } else {
                    // 选项已存在，直接设置值
                    $(regionInput).val(regionValue).trigger('change');
                }
                return;
            }
            await new Promise(resolve => setTimeout(resolve, 100));
            retries++;
        }
    }

    getNestedValue(obj, path) {
        return path.split('.').reduce((current, key) => current?.[key], obj);
    }

    async submitForm() {
        // 防止重複提交
        if (this.isSubmitting) {
            console.log('正在提交表單，跳過重複調用');
            return;
        }
        this.isSubmitting = true;
        
        try { // 外層 try-finally 確保 isSubmitting 在任何退出路徑都會被重置
        
        // 立即清除自動保存草稿的定時器，防止在提交時同時生成草稿
        if (this.saveTimer) {
            clearTimeout(this.saveTimer);
            this.saveTimer = null;
        }
        // 禁用自動保存功能，防止在提交過程中觸發
        this.autoSaveDisabled = true;
        
        const data = this.collectFormData();
        console.log('提交的數據:', data); // 調試用

        // users：附加所屬店舖子表數據到 extra_fields，並處理佣金相關字段
        if (this.pageName === 'users') {
            const storeData = this.collectUserStoreData();
            if (!data.extra_fields) data.extra_fields = {};
            data.extra_fields.stores = storeData;
            
            // 將訂單佣金設置保存到 extra_fields
            if (data.order_commission_mode !== undefined && data.order_commission_mode !== null && data.order_commission_mode !== '') {
                data.extra_fields.order_commission_mode = data.order_commission_mode;
            }
            if (data.order_commission_rate !== undefined && data.order_commission_rate !== null && data.order_commission_rate !== '') {
                data.extra_fields.order_commission_rate = parseFloat(data.order_commission_rate) || 0;
            }
            if (data.order_commission_fixed !== undefined && data.order_commission_fixed !== null && data.order_commission_fixed !== '') {
                data.extra_fields.order_commission_fixed = parseFloat(data.order_commission_fixed) || 0;
            }
            // 將服務單佣金設置保存到 extra_fields
            if (data.service_order_commission_mode !== undefined && data.service_order_commission_mode !== null && data.service_order_commission_mode !== '') {
                data.extra_fields.service_order_commission_mode = data.service_order_commission_mode;
            }
            if (data.service_order_commission_rate !== undefined && data.service_order_commission_rate !== null && data.service_order_commission_rate !== '') {
                data.extra_fields.service_order_commission_rate = parseFloat(data.service_order_commission_rate) || 0;
            }
            if (data.service_order_commission_fixed !== undefined && data.service_order_commission_fixed !== null && data.service_order_commission_fixed !== '') {
                data.extra_fields.service_order_commission_fixed = parseFloat(data.service_order_commission_fixed) || 0;
            }
            // 向後兼容：保留舊的 commission_mode, commission_rate, commission_fixed（用於訂單和服務單的默認值）
            // 如果沒有設置新的字段，使用舊字段作為默認值
            if (!data.extra_fields.order_commission_mode && data.commission_mode !== undefined && data.commission_mode !== null && data.commission_mode !== '') {
                data.extra_fields.order_commission_mode = data.commission_mode;
                data.extra_fields.service_order_commission_mode = data.commission_mode;
            }
            if (!data.extra_fields.order_commission_rate && data.commission_rate !== undefined && data.commission_rate !== null && data.commission_rate !== '') {
                data.extra_fields.order_commission_rate = parseFloat(data.commission_rate) || 0;
                data.extra_fields.service_order_commission_rate = parseFloat(data.commission_rate) || 0;
            }
            if (!data.extra_fields.order_commission_fixed && data.commission_fixed !== undefined && data.commission_fixed !== null && data.commission_fixed !== '') {
                data.extra_fields.order_commission_fixed = parseFloat(data.commission_fixed) || 0;
                data.extra_fields.service_order_commission_fixed = parseFloat(data.commission_fixed) || 0;
            }
        }
        
        // 如果是推廣表單，處理發送方式
        if (this.pageName === 'promotions') {
            const sendType = data.send_type;
            if (sendType === 'immediate') {
                // 即時發送：清空排程時間
                data.scheduled_at = null;
            } else if (sendType === 'scheduled' && !data.scheduled_at) {
                // 排程發送但沒有設置時間，提示錯誤
                App.showAlert((typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelectScheduleTime') : '請選擇排程時間', 'warning');
                throw new Error('Scheduled time is required');
            }
            // 移除 send_type 字段（後端不需要）
            delete data.send_type;
        }
        
        // 如果是產品表單，處理服務套票
        if (this.pageName === 'products') {
            const isServicePackage = data.is_service_package;
            // 將 true/false 字符串轉換為 boolean
            if (isServicePackage === 'true' || isServicePackage === true) {
                data.is_service_package = true;
            } else {
                data.is_service_package = false;
                // 如果不是服務套票，清空對應服務
                data.service_package_service_id = null;
            }
            
            // 處理 allow_backorder：將 true/false 字符串轉換為 boolean
            if (data.allow_backorder === 'true' || data.allow_backorder === true) {
                data.allow_backorder = true;
            } else {
                data.allow_backorder = false;
            }
        }
        
        // 如果是假期表單，處理每年重複
        if (this.pageName === 'holidays') {
            // 將 true/false 字符串轉換為 boolean
            if (data.is_recurring === 'true' || data.is_recurring === true) {
                data.is_recurring = true;
            } else {
                data.is_recurring = false;
            }
        }
        
        // 如果是優惠券表單，收集條件數據
        if (this.pageName === 'coupons') {
            const conditions = this.collectConditions();
            this.pendingConditions = conditions;
        }
        
        // 如果是產品表單，收集產品屬性數據
        if (this.pageName === 'products') {
            const attributes = this.collectProductAttributes();
            data.product_attributes = attributes;
        }
        if (this.pageName === 'payrolls') {
            data.contributions = this.collectPayrollContributions();
        }
        
        // 檢查是否有文件上傳
        const imageFile = data.image_url;
        let imageUrl = null;
        
        // 如果有圖片文件，先上傳
        if (imageFile instanceof File) {
            try {
                const uploadResult = await this.uploadFile(imageFile);
                if (uploadResult && uploadResult.url) {
                    imageUrl = uploadResult.url;
                    data.image_url = imageUrl;
                }
            } catch (error) {
                App.showAlert('圖片上傳失敗: ' + error.message, 'danger');
                return;
            }
        } else if (this.isEdit && !imageFile && this.originalImageUrl) {
            // 編輯模式：如果沒有選擇新文件，保留原始圖片 URL
            data.image_url = this.originalImageUrl;
        } else if (!imageFile) {
            // 新建模式：如果沒有選擇文件，設為 null
            data.image_url = null;
        }
        
        // 處理 incomes 的 attachment 字段（存儲在 ExtraFields 中）
        if (this.pageName === 'incomes' && data.attachment) {
            const attachmentFile = data.attachment;
            if (attachmentFile instanceof File) {
                try {
                    const uploadResult = await this.uploadFile(attachmentFile);
                    if (uploadResult && uploadResult.url) {
                        // 將附件 URL 存儲在 ExtraFields 中
                        if (!data.extra_fields) {
                            data.extra_fields = {};
                        }
                        data.extra_fields.attachment_url = uploadResult.url;
                        data.extra_fields.attachment_name = attachmentFile.name;
                    }
                } catch (error) {
                    App.showAlert('附件上傳失敗: ' + error.message, 'danger');
                    return;
                }
            }
            // 移除 attachment 字段（後端不需要）
            delete data.attachment;
        }
        
        // 驗證必填字段
        for (const field of this.config.formFields) {
            if (field.required) {
                // payment-methods：payment_type 存在 extra_fields（collectFormData 會搬移並刪除主欄位）
                const value = (this.pageName === 'payment-methods' && field.key === 'payment_type')
                    ? (data && data.extra_fields ? data.extra_fields.payment_type : undefined)
                    : data[field.key];
                // 對於數字類型，0 是有效值，不應該被視為空值
                if (field.type === 'number') {
                    if (value === null || value === undefined || value === '') {
                        const fieldName = this.getFieldLabel(field) || field.label || field.key;
                        const msgKey = 'common.pleaseFill';
                        const prefix = (typeof I18n !== 'undefined' && I18n.t && I18n.t(msgKey) !== msgKey) ? I18n.t(msgKey) : '請填寫';
                        App.showAlert(`${prefix}${fieldName ? ` ${fieldName}` : ''}`.trim(), 'warning');
                        throw new Error('Validation failed');
                    }
                } else if (!value || (Array.isArray(value) && value.length === 0)) {
                    const fieldName = this.getFieldLabel(field) || field.label || field.key;
                    const msgKey = 'common.pleaseFill';
                    const prefix = (typeof I18n !== 'undefined' && I18n.t && I18n.t(msgKey) !== msgKey) ? I18n.t(msgKey) : '請填寫';
                    App.showAlert(`${prefix}${fieldName ? ` ${fieldName}` : ''}`.trim(), 'warning');
                    throw new Error('Validation failed');
                }
            }
        }
        
        // 新建模式：確保所有 readonly 編號字段都有值（最後防線，防止提交空編號）
        if (!this.isEdit) {
            const numbersOK = await this.ensureAllNumbersBeforeSubmit();
            if (!numbersOK) {
                throw new Error('Validation failed');
            }
            // Re-collect form data to pick up any numbers that were just fetched
            const refreshedData = this.collectFormData();
            const numberFieldKeys = this.config.formFields
                .filter(f => (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
                    f.key === 'sale_number' || f.key === 'purchase_order_number' || f.key === 'employee_number' ||
                    (f.key && f.key.includes('number'))) && f.readonly)
                .map(f => f.key);
            for (const key of numberFieldKeys) {
                if (refreshedData[key] && (!data[key] || data[key].trim() === '')) {
                    data[key] = refreshedData[key];
                }
            }
        }

        // 客戶表單：檢查電郵或電話是否重複（僅新建模式）
        if (this.pageName === 'customers' && !this.isEdit) {
            if (data.email || (data.phone && data.phone_country_code)) {
                try {
                    const checkResult = await App.apiRequest('/customers/check-duplicate', {
                        method: 'POST',
                        body: JSON.stringify({
                            email: data.email || '',
                            phone: data.phone || '',
                            phone_country_code: data.phone_country_code || '',
                            exclude_id: ''
                        })
                    });
                    
                    if (checkResult.has_duplicate && checkResult.duplicates && checkResult.duplicates.length > 0) {
                        // 構建重複信息提示
                        let duplicateMessages = [];
                        checkResult.duplicates.forEach(dup => {
                            const customer = dup.customer;
                            const customerName = customer.last_name 
                                ? `${customer.name} ${customer.last_name}` 
                                : customer.name;
                            if (dup.type === 'email') {
                                duplicateMessages.push(`電郵 ${dup.value} 已被客戶 ${customerName} (${customer.code}) 使用`);
                            } else if (dup.type === 'phone') {
                                duplicateMessages.push(`電話 ${dup.value} 已被客戶 ${customerName} (${customer.code}) 使用`);
                            }
                        });
                        
                        const message = `發現重複記錄：\n\n${duplicateMessages.join('\n')}\n\n是否仍要繼續保存？`;
                        
                        // 顯示確認對話框
                        const confirmed = confirm(message);
                        if (!confirmed) {
                            return; // 用戶取消，不提交表單
                        }
                    }
                } catch (error) {
                    console.warn('檢查重複記錄失敗:', error);
                    // 檢查失敗不阻止提交，繼續執行
                }
            }
        }

        let couponId = this.itemId;
        let result;
            
        if (this.isEdit && this.itemId) {
                console.log('提交更新數據:', data); // 調試用
                console.log('API 路徑:', `${this.config.apiPath}/${this.itemId}`); // 調試用
                
                try {
                    result = await App.apiRequest(`${this.config.apiPath}/${this.itemId}`, {
                        method: 'PUT',
                        body: JSON.stringify(data)
                    });
                    console.log('更新結果:', result); // 調試用
                    
                    // 檢查是否有錯誤
                    if (result && result.error) {
                        throw new Error(result.error);
                    }
                    
                    App.showAlert('更新成功', 'success');
                    
                    // 如果是客戶表單，保存地址
                    if (this.pageName === 'customers' && this.itemId) {
                        try {
                            await this.saveCustomerAddresses(this.itemId);
                        } catch (error) {
                            console.error('保存客戶地址失敗:', error);
                            // 不阻止表單提交，只記錄錯誤
                        }
                    }
                    
                    // 更新模式：也刪除草稿（如果存在）
                    if (this.currentDraftId && this.pageName) {
                        try {
                            draftManager.deleteDraft(this.pageName, this.currentDraftId);
                            console.log('已刪除草稿:', this.currentDraftId);
                        } catch (draftError) {
                            console.warn('刪除草稿失敗:', draftError);
                        }
                    }
                } catch (apiError) {
                    console.error('API 請求錯誤:', apiError); // 調試用
                    throw apiError; // 重新拋出錯誤，讓外層 catch 處理
                }
            } else {
                // 提交前預留編號
                await this.reserveNumbers(data);
                
                result = await App.apiRequest(this.config.apiPath, {
                    method: 'POST',
                    body: JSON.stringify(data)
                });
                
                // 檢查是否有錯誤
                if (result.error) {
                    throw new Error(result.error);
                }
                
                App.showAlert('創建成功', 'success');
                
                // 如果是優惠券，獲取新創建的 ID
                if (this.pageName === 'coupons' && result.id) {
                    couponId = result.id;
                }
                
                // 如果是客戶表單，保存地址
                if (this.pageName === 'customers' && result.id) {
                    try {
                        await this.saveCustomerAddresses(result.id);
                    } catch (error) {
                        console.error('保存客戶地址失敗:', error);
                        // 不阻止表單提交，只記錄錯誤
                    }
                }
                
                // 刪除草稿（創建成功後）
                if (this.currentDraftId && this.pageName) {
                    try {
                    draftManager.deleteDraft(this.pageName, this.currentDraftId);
                        console.log('已刪除草稿:', this.currentDraftId);
                    } catch (draftError) {
                        console.warn('刪除草稿失敗:', draftError);
                    }
                }
            }
            
            // 如果是優惠券，保存條件
            if (this.pageName === 'coupons' && this.pendingConditions && couponId) {
                await this.saveConditions(couponId);
            }
            
            // 只有在成功後才跳轉（使用 replace 避免瀏覽器歷史記錄問題）
            setTimeout(() => {
                const listPath = this.getListPathWithParams();
                if (typeof Router !== 'undefined' && Router.replace) {
                    Router.replace(listPath);
                } else {
                    window.location.replace(listPath);
                }
            }, 1000);
        } catch (error) {
            console.error('保存失敗:', error); // 調試用
            console.error('錯誤詳情:', {
                message: error.message,
                stack: error.stack,
                error: error
            }); // 調試用
            // Validation failed 的提示已在驗證時顯示，不再重複顯示
            if (error.message !== 'Validation failed') {
                const errorMessage = error.message || error.toString() || '未知錯誤';
                App.showAlert('保存失敗: ' + errorMessage, 'danger');
            }
            
            // 保存失敗時，重新啟用自動保存
            this.autoSaveDisabled = false;
            // 發生錯誤時不跳轉，讓用戶有機會修正錯誤
            return; // 明確返回，不繼續執行
        } finally {
            this.isSubmitting = false; // 重置提交狀態
        }
    }

    async uploadFile(file) {
        const formData = new FormData();
        formData.append('file', file);
        
        try {
            const tenantSubdomain = localStorage.getItem('tenant_subdomain');
            const token = localStorage.getItem('auth_token');
            
            const headers = {};
            if (tenantSubdomain) {
                headers['X-Tenant-Subdomain'] = tenantSubdomain;
            }
            if (token) {
                headers['Authorization'] = `Bearer ${token}`;
            }
            
            const response = await fetch('/api/v1/upload', {
                method: 'POST',
                headers: headers,
                body: formData
            });
            
            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || '文件上傳失敗');
            }
            
            return await response.json();
        } catch (error) {
            console.error('文件上傳失敗:', error);
            throw error;
        }
    }

    async reserveNumbers(data) {
        // 找出需要預留的編號字段
        const numberFields = this.config.formFields.filter(f => 
            (f.key === 'order_number' || f.key === 'code' || f.key === 'invoice_number' || 
             f.key === 'sale_number' || f.key === 'purchase_order_number' || f.key === 'employee_number' ||
             (f.key && f.key.includes('number'))) &&
            data[f.key] && data[f.key].trim() !== ''
        );

        if (numberFields.length === 0) return;

        // 調用後端 API 預留編號
        for (const field of numberFields) {
            try {
                await App.apiRequest(`/api/v1/reserved-numbers`, {
                    method: 'POST',
                    body: JSON.stringify({
                        field_name: field.key,
                        field_value: data[field.key],
                        page_name: this.pageName
                    })
                });
            } catch (error) {
                console.warn(`Failed to reserve number for ${field.key}:`, error);
            }
        }
    }
    
    // 渲染產品屬性管理區域
    renderProductAttributes() {
        const form = document.getElementById('dynamicForm');
        if (!form) return;

        const t = (key, fallback) => {
            try {
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const v = I18n.t(key);
                    if (v && v !== key) return v;
                }
            } catch (e) {
                // ignore
            }
            return fallback;
        };
        const titleText = t('products.attributes.title', '產品屬性');
        const addText = t('products.attributes.add', '添加屬性');
        const thAttrText = t('products.attributes.attribute', '屬性');
        const thValueText = t('products.attributes.value', '值');
        const thActionsText = t('products.attributes.actions', '操作');
        const emptyText = t('products.attributes.empty', '暫無屬性');
        
        const attributesSection = `
            <div class="card mt-4">
                <div class="card-header d-flex justify-content-between align-items-center">
                    <h5 class="mb-0" data-i18n="products.attributes.title">${titleText}</h5>
                    <button type="button" class="btn btn-sm btn-primary" onclick="window.dynamicForm.addProductAttribute()">
                        <i class="bi bi-plus-circle"></i> <span data-i18n="products.attributes.add">${addText}</span>
                    </button>
                </div>
                <div class="card-body">
                    <div class="table-responsive">
                        <table class="table table-bordered">
                            <thead>
                                <tr>
                                    <!-- 前兩欄不要固定寬度，讓瀏覽器自動分配 -->
                                    <th data-i18n="products.attributes.attribute">${thAttrText}</th>
                                    <th data-i18n="products.attributes.value">${thValueText}</th>
                                    <!-- 操作欄：保持與 list 一致（不固定寬度，nowrap，必要時再用 JS 自動計算） -->
                                    <th class="product-attr-actions-header text-nowrap" data-i18n="products.attributes.actions">${thActionsText}</th>
                                </tr>
                            </thead>
                            <tbody id="productAttributesList">
                                <tr>
                                    <td colspan="3" class="text-center text-muted" data-i18n="products.attributes.empty">${emptyText}</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        `;
        
        form.insertAdjacentHTML('beforeend', attributesSection);

        // 初始化時也嘗試計算一次操作列寬度（若目前已有 row）
        this.calculateProductAttributeActionColumnWidth?.();
        
        // 如果是編輯模式，載入現有屬性
        // 注意：這裡不立即調用，因為數據可能還沒加載完成
        // 將在 populateForm 中調用
    }

    // 薪資供款明細（多行）
    renderPayrollContributions() {
        const form = document.getElementById('dynamicForm');
        if (!form) return;
        const fieldset = document.createElement('div');
        fieldset.className = 'card mt-4';
        fieldset.innerHTML = `
            <div class="card-header d-flex justify-content-between align-items-center">
                <h5 class="mb-0">款項明細</h5>
                <button type="button" class="btn btn-sm btn-primary" onclick="window.dynamicForm.addPayrollContributionRow()">
                    <i class="bi bi-plus-circle"></i> 添加
                </button>
            </div>
            <div class="card-body p-0">
                <table class="table mb-0">
                    <thead>
                        <tr>
                            <th style="width: 20%">款項名稱</th>
                            <th style="width: 15%">扣/加方</th>
                            <th style="width: 15%">方式</th>
                            <th style="width: 20%" id="rateColumnHeader">比例 (%)</th>
                            <th style="width: 20%" id="amountColumnHeader" style="display: none;">固定金額</th>
                            <th style="width: 10%">操作</th>
                        </tr>
                    </thead>
                    <tbody id="payrollContribBody">
                        <tr class="text-center text-muted" data-empty><td colspan="6">暫無供款</td></tr>
                    </tbody>
                </table>
            </div>
        `;
        form.appendChild(fieldset);
    }

    addPayrollContributionRow(data = {}) {
        const tbody = document.getElementById('payrollContribBody');
        if (!tbody) return;
        const rowId = 'contrib_' + Date.now() + Math.floor(Math.random() * 1000);
        if (tbody.querySelector('[data-empty]')) tbody.innerHTML = '';
        const name = data.name || '';
        const payer = data.payer || 'employee';
        const mode = data.mode || 'percent';
        const rate = data.rate != null ? data.rate : '';
        const amount = data.amount != null ? data.amount : '';
        const row = document.createElement('tr');
        row.setAttribute('data-id', rowId);
        const rateCellStyle = mode === 'percent' ? '' : 'display: none;';
        const amountCellStyle = mode === 'fixed' ? '' : 'display: none;';
        row.innerHTML = `
            <td>
                <input type="text" class="form-control" id="${rowId}_name" value="${name}" placeholder="款項名稱">
            </td>
            <td>
                <select class="form-select" id="${rowId}_payer">
                    <option value="employee" ${payer === 'employee' ? 'selected' : ''}>員工扣除</option>
                    <option value="employer" ${payer === 'employer' ? 'selected' : ''}>雇主另加</option>
                </select>
            </td>
            <td>
                <select class="form-select" id="${rowId}_mode" onchange="window.dynamicForm.togglePayrollContributionMode('${rowId}')">
                    <option value="percent" ${mode === 'percent' ? 'selected' : ''}>百分比</option>
                    <option value="fixed" ${mode === 'fixed' ? 'selected' : ''}>定額</option>
                </select>
            </td>
            <td id="${rowId}_rateCell" style="${rateCellStyle}">
                <input type="number" step="0.01" class="form-control" id="${rowId}_rate" value="${rate}" placeholder="如 5 = 5%">
            </td>
            <td id="${rowId}_amountCell" style="${amountCellStyle}">
                <input type="number" step="0.01" class="form-control" id="${rowId}_amount" value="${amount}" placeholder="固定金額">
            </td>
            <td><button type="button" class="btn btn-sm btn-danger" onclick="window.dynamicForm.removePayrollContributionRow('${rowId}')"><i class="bi bi-trash"></i></button></td>
        `;
        tbody.appendChild(row);
        // 更新表头显示
        this.updatePayrollContributionHeaders();
    }

    togglePayrollContributionMode(rowId) {
        const modeSelect = document.getElementById(`${rowId}_mode`);
        const rateCell = document.getElementById(`${rowId}_rateCell`);
        const amountCell = document.getElementById(`${rowId}_amountCell`);
        if (!modeSelect || !rateCell || !amountCell) return;
        
        const mode = modeSelect.value;
        if (mode === 'percent') {
            rateCell.style.display = '';
            amountCell.style.display = 'none';
        } else {
            rateCell.style.display = 'none';
            amountCell.style.display = '';
        }
        // 更新表头显示
        this.updatePayrollContributionHeaders();
    }

    updatePayrollContributionHeaders() {
        const tbody = document.getElementById('payrollContribBody');
        if (!tbody) return;
        const rows = tbody.querySelectorAll('tr[data-id]');
        let hasPercent = false;
        let hasFixed = false;
        
        rows.forEach(row => {
            const rowId = row.getAttribute('data-id');
            const modeSelect = document.getElementById(`${rowId}_mode`);
            if (modeSelect) {
                if (modeSelect.value === 'percent') hasPercent = true;
                if (modeSelect.value === 'fixed') hasFixed = true;
            }
        });
        
        const rateHeader = document.getElementById('rateColumnHeader');
        const amountHeader = document.getElementById('amountColumnHeader');
        if (rateHeader && amountHeader) {
            if (hasPercent && hasFixed) {
                // 两种方式都有，都显示
                rateHeader.style.display = '';
                amountHeader.style.display = '';
            } else if (hasPercent) {
                rateHeader.style.display = '';
                amountHeader.style.display = 'none';
            } else if (hasFixed) {
                rateHeader.style.display = 'none';
                amountHeader.style.display = '';
            } else {
                // 默认显示比例
                rateHeader.style.display = '';
                amountHeader.style.display = 'none';
            }
        }
    }

    removePayrollContributionRow(rowId) {
        const row = document.querySelector(`tr[data-id="${rowId}"]`);
        if (row) row.remove();
        const tbody = document.getElementById('payrollContribBody');
        if (tbody && tbody.children.length === 0) {
            tbody.innerHTML = `<tr class="text-center text-muted" data-empty><td colspan="6">暫無供款</td></tr>`;
        }
        // 更新表头显示
        this.updatePayrollContributionHeaders();
    }

    collectPayrollContributions() {
        const tbody = document.getElementById('payrollContribBody');
        if (!tbody) return [];
        const rows = tbody.querySelectorAll('tr[data-id]');
        const list = [];
        rows.forEach(row => {
            const id = row.getAttribute('data-id');
            const name = document.getElementById(`${id}_name`)?.value || '';
            const payer = document.getElementById(`${id}_payer`)?.value || 'employee';
            const mode = document.getElementById(`${id}_mode`)?.value || 'percent';
            const rateVal = document.getElementById(`${id}_rate`)?.value;
            const amountVal = document.getElementById(`${id}_amount`)?.value;
            const rate = rateVal ? parseFloat(rateVal) / 100 : 0; // UI 輸入百分比，轉小數
            const amount = amountVal ? parseFloat(amountVal) : 0;
            if (!name) return; // 必須有款項名稱
            if (mode === 'percent' && rate === 0) return;
            if (mode === 'fixed' && amount === 0) return;
            list.push({
                name,
                payer,
                mode,
                rate,
                amount
            });
        });
        return list;
    }

    loadPayrollContributions(data) {
        if (!data || !data.contributions) return;
        const list = data.contributions;
        if (!list || list.length === 0) return;
        list.forEach(item => {
            this.addPayrollContributionRow({
                name: item.name || '',
                payer: item.payer,
                mode: item.mode,
                rate: item.mode === 'percent' ? (item.rate * 100) : '',
                amount: item.amount
            });
        });
    }
    
    // 添加產品屬性行
    async addProductAttribute(selectedAttributeId = null, value = '') {
        const attributesList = document.getElementById('productAttributesList');
        if (!attributesList) return;
        
        // 清除"暫無屬性"提示
        if (attributesList.querySelector('td[colspan]')) {
            attributesList.innerHTML = '';
        }
        
        // 載入產品屬性選項
        const selectAttrText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelectAttribute') : '請選擇屬性';
        let attributeOptions = `<option value="">${selectAttrText}</option>`;
        let attributeMap = {};
        try {
            const attributes = await App.apiRequest('/product-attributes?limit=1000');
            const items = attributes.data || [];
            items.forEach(attr => {
                attributeOptions += `<option value="${attr.id}" data-type="${attr.attribute_type || 'text'}" data-options='${JSON.stringify(attr.options || [])}'>${attr.name}</option>`;
                attributeMap[attr.id] = attr;
            });
        } catch (error) {
            console.error('Failed to load product attributes:', error);
        }
        
        const attributeId = 'attr_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
        const selectedAttr = selectedAttributeId ? attributeMap[selectedAttributeId] : null;
        const isDropdown = selectedAttr && (selectedAttr.attribute_type === 'select' || selectedAttr.attribute_type === 'dropdown');
        let options = selectedAttr && selectedAttr.options ? selectedAttr.options : [];
        
        // 如果 options 是字符串（逗號分隔），轉換為數組
        if (typeof options === 'string' && options.trim() !== '') {
            options = options.split(',').map(opt => opt.trim()).filter(opt => opt !== '');
        }
        
        let valueField = '';
        if (isDropdown && options.length > 0) {
            // Dropdown 類型 - 使用 multi-select，默認全選
            let optionHtml = '';
            // 處理 value 參數：可能是字符串（逗號分隔）或數組
            let selectedValues = [];
            if (value) {
                if (Array.isArray(value)) {
                    selectedValues = value.map(v => String(v).trim()).filter(v => v);
                } else {
                    const valueStr = String(value).trim();
                    selectedValues = valueStr ? valueStr.split(',').map(v => v.trim()).filter(v => v) : [];
                }
            }
            // 默認全選所有選項（如果沒有指定值）
            const shouldSelectAll = selectedValues.length === 0;
            
            console.log(`渲染屬性值選項: selectedValues=${JSON.stringify(selectedValues)}, shouldSelectAll=${shouldSelectAll}`); // 調試用
            
            options.forEach(opt => {
                const optValue = typeof opt === 'string' ? opt : (opt.value || opt.label || opt);
                const optLabel = typeof opt === 'string' ? opt : (opt.label || opt.value || opt);
                const optValueStr = String(optValue).trim();
                // 使用精確匹配，確保值正確匹配
                const isSelected = shouldSelectAll || selectedValues.some(sv => String(sv).trim() === optValueStr);
                optionHtml += `<option value="${optValueStr}" ${isSelected ? 'selected' : ''}>${optLabel}</option>`;
            });
            valueField = `<select class="form-select attribute-value" id="attr_value_${attributeId}" multiple>${optionHtml}</select>`;
        } else {
            // Input 類型
            const valuePh = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('products.attributes.valuePlaceholder') : '輸入屬性值';
            valueField = `<input type="text" class="form-control attribute-value" id="attr_value_${attributeId}" value="${value}" placeholder="${valuePh}" data-i18n-placeholder="products.attributes.valuePlaceholder">`;
        }
        
        const row = `
            <tr data-attribute-id="${attributeId}">
                <td>
                    <select class="form-select attribute-field" id="attr_field_${attributeId}" onchange="window.dynamicForm.updateAttributeValueField('${attributeId}', this.value)">
                        ${attributeOptions}
                    </select>
                </td>
                <td id="attr_value_container_${attributeId}">
                    ${valueField}
                </td>
                <td class="product-attr-actions-cell text-nowrap">
                    <button type="button" class="btn btn-sm btn-danger" onclick="window.dynamicForm.removeProductAttribute('${attributeId}')">
                        <i class="bi bi-trash"></i>
                    </button>
                </td>
            </tr>
        `;
        
        attributesList.insertAdjacentHTML('beforeend', row);

        // 新增 row 後調整操作列寬度
        this.calculateProductAttributeActionColumnWidth?.();
        
        // 如果指定了選中的屬性，設置選中狀態
        // 注意：如果已經有值，不要觸發 change 事件，因為會導致值丟失
        if (selectedAttributeId) {
            const select = document.getElementById(`attr_field_${attributeId}`);
            if (select) {
                select.value = selectedAttributeId;
                // 只有在沒有值時才觸發 change 事件（讓 updateAttributeValueField 處理）
                // 如果有值，直接調用 updateAttributeValueField 並傳遞當前值
                if (!value || value === '') {
                    select.dispatchEvent(new Event('change'));
                } else {
                    // 如果有值，直接更新值字段，保留當前值
                    this.updateAttributeValueField(attributeId, selectedAttributeId, value);
                }
            }
        }
        
        // 如果是 multi-select，初始化 Select2（如果可用）
        if (isDropdown && options.length > 0) {
            const valueSelect = document.getElementById(`attr_value_${attributeId}`);
            if (valueSelect && typeof $ !== 'undefined') {
                // 延遲初始化，確保 DOM 已更新
                setTimeout(() => {
                    try {
                        // 確保選中狀態正確設置（在初始化 Select2 之前）
                        let selectedValues = [];
                        if (value) {
                            if (Array.isArray(value)) {
                                selectedValues = value.map(v => String(v).trim()).filter(v => v);
                            } else {
                                const valueStr = String(value).trim();
                                selectedValues = valueStr ? valueStr.split(',').map(v => v.trim()).filter(v => v) : [];
                            }
                        }
                        
                        console.log(`初始化 Select2: attributeId=${attributeId}, selectedValues=${JSON.stringify(selectedValues)}`); // 調試用
                        
                        // 如果已經初始化過 Select2，先銷毀
                        if ($(valueSelect).hasClass('select2-hidden-accessible')) {
                            $(valueSelect).select2('destroy');
                        }
                        
                        // 先設置選中狀態（在初始化 Select2 之前）
                        if (selectedValues.length > 0) {
                            // 先清空所有選中狀態
                            Array.from(valueSelect.options).forEach(option => {
                                option.selected = false;
                            });
                            // 然後設置選中狀態
                            Array.from(valueSelect.options).forEach(option => {
                                const optValue = String(option.value).trim();
                                const isSelected = selectedValues.some(sv => String(sv).trim() === optValue);
                                if (isSelected) {
                                    option.selected = true;
                                }
                            });
                            const selectedCount = Array.from(valueSelect.selectedOptions).length;
                            console.log(`設置選中狀態後，選中的選項數量: ${selectedCount}`, Array.from(valueSelect.selectedOptions).map(opt => opt.value)); // 調試用
                        }
                        
                        // 初始化 Select2（確保支持多選）
                        $(valueSelect).select2({
                            theme: 'bootstrap-5',
                            placeholder: (typeof I18n !== 'undefined' && I18n.t && I18n.t('common.pleaseSelectValue') !== 'common.pleaseSelectValue')
                                ? I18n.t('common.pleaseSelectValue')
                                : '選擇屬性值（可多選）',
                            width: '100%',
                            closeOnSelect: false,
                            multiple: true  // 明確指定多選模式
                        });
                        
                        // Select2 初始化後，再次設置值（確保正確顯示）
                        if (selectedValues.length > 0) {
                            // 確保所有值都是字符串格式，並且與選項值匹配
                            const stringValues = selectedValues.map(v => String(v).trim()).filter(v => {
                                // 驗證值是否存在於選項中
                                return Array.from(valueSelect.options).some(opt => String(opt.value).trim() === v);
                            });
                            console.log(`Select2 初始化後設置值:`, stringValues); // 調試用
                            console.log(`所有可用選項值:`, Array.from(valueSelect.options).map(opt => String(opt.value).trim())); // 調試用
                            
                            if (stringValues.length > 0) {
                                $(valueSelect).val(stringValues).trigger('change');
                                
                                // 驗證設置是否成功
                                setTimeout(() => {
                                    const currentVal = $(valueSelect).val();
                                    console.log(`Select2 當前值:`, currentVal, `期望值:`, stringValues); // 調試用
                                    if (!Array.isArray(currentVal) || currentVal.length !== stringValues.length) {
                                        console.warn('Select2 值設置可能失敗，嘗試重新設置');
                                        $(valueSelect).val(stringValues).trigger('change');
                                    }
                                }, 200);
                            } else {
                                console.warn('沒有找到匹配的選項值');
                            }
                        } else {
                            $(valueSelect).trigger('change');
                        }
                    } catch (error) {
                        console.error('Failed to initialize Select2 for attribute value:', error);
                    }
                }, 300);
            }
        }
    }
    
    async updateAttributeValueField(rowId, attributeId) {
        if (!attributeId) {
            // 清空值字段
            const container = document.getElementById(`attr_value_container_${rowId}`);
            if (container) {
                container.innerHTML = '<input type="text" class="form-control attribute-value" id="attr_value_' + rowId + '" placeholder="輸入屬性值">';
            }
            return;
        }
        
        try {
            const attribute = await App.apiRequest(`/product-attributes/${attributeId}`);
            const isDropdown = attribute.attribute_type === 'select' || attribute.attribute_type === 'dropdown';
            let options = attribute.options || [];
            
            // 如果 options 是字符串（逗號分隔），轉換為數組
            if (typeof options === 'string' && options.trim() !== '') {
                options = options.split(',').map(opt => opt.trim()).filter(opt => opt !== '');
            }
            
            const container = document.getElementById(`attr_value_container_${rowId}`);
            if (!container) return;
            
            // 保存當前的值（如果有）
            let currentValue = '';
            const currentValueInput = container.querySelector('.attribute-value');
            if (currentValueInput) {
                // 如果是 multi-select，使用 Select2 的值（如果已初始化）
                if (currentValueInput.multiple && typeof $ !== 'undefined' && $(currentValueInput).hasClass('select2-hidden-accessible')) {
                    const select2Values = $(currentValueInput).val();
                    if (Array.isArray(select2Values)) {
                        currentValue = select2Values.filter(v => v).join(',');
                    } else if (select2Values) {
                        currentValue = String(select2Values);
                    }
                } else if (currentValueInput.multiple) {
                    // 如果是 multi-select 但 Select2 未初始化，使用原生方式
                    const selectedOptions = Array.from(currentValueInput.selectedOptions);
                    currentValue = selectedOptions.map(opt => opt.value).filter(v => v).join(',');
                } else {
                    currentValue = currentValueInput.value || '';
                }
            }
            
            if (isDropdown && options.length > 0) {
                // Dropdown 類型 - 使用 multi-select，默認全選
                let optionHtml = '';
                // 處理 currentValue：可能是字符串（逗號分隔）或數組
                let selectedValues = [];
                if (currentValue) {
                    if (Array.isArray(currentValue)) {
                        selectedValues = currentValue.map(v => String(v).trim()).filter(v => v);
                    } else {
                        const valueStr = String(currentValue).trim();
                        selectedValues = valueStr ? valueStr.split(',').map(v => v.trim()).filter(v => v) : [];
                    }
                }
                // 默認全選所有選項（如果沒有指定值）
                const shouldSelectAll = selectedValues.length === 0;
                
                options.forEach(opt => {
                    const optValue = typeof opt === 'string' ? opt : (opt.value || opt.label || opt);
                    const optLabel = typeof opt === 'string' ? opt : (opt.label || opt.value || opt);
                    const optValueStr = String(optValue);
                    const isSelected = shouldSelectAll || selectedValues.some(sv => String(sv).trim() === optValueStr);
                    optionHtml += `<option value="${optValueStr}" ${isSelected ? 'selected' : ''}>${optLabel}</option>`;
                });
                container.innerHTML = `<select class="form-select attribute-value" id="attr_value_${rowId}" multiple>${optionHtml}</select>`;
                
                // 初始化 Select2（如果可用）
                const valueSelect = document.getElementById(`attr_value_${rowId}`);
                if (valueSelect && typeof $ !== 'undefined') {
                    setTimeout(() => {
                        try {
                            // 如果已經初始化過 Select2，先銷毀
                            if ($(valueSelect).hasClass('select2-hidden-accessible')) {
                                $(valueSelect).select2('destroy');
                            }
                            
                            // 確保選中狀態正確設置（在初始化 Select2 之前）
                            let selectedValues = [];
                            if (currentValue) {
                                if (Array.isArray(currentValue)) {
                                    selectedValues = currentValue.map(v => String(v).trim()).filter(v => v);
                                } else {
                                    const valueStr = String(currentValue).trim();
                                    selectedValues = valueStr ? valueStr.split(',').map(v => v.trim()).filter(v => v) : [];
                                }
                            }
                            
                            if (selectedValues.length > 0) {
                                // 設置選中狀態（使用精確匹配）
                                Array.from(valueSelect.options).forEach(option => {
                                    const optValue = String(option.value).trim();
                                    const isSelected = selectedValues.some(sv => String(sv).trim() === optValue);
                                    option.selected = isSelected;
                                });
                            }
                            
                            $(valueSelect).select2({
                                theme: 'bootstrap-5',
                                placeholder: (typeof I18n !== 'undefined' && I18n.t && I18n.t('common.pleaseSelectValue') !== 'common.pleaseSelectValue')
                                    ? I18n.t('common.pleaseSelectValue')
                                    : '選擇屬性值（可多選）',
                                width: '100%',
                                closeOnSelect: false,
                                multiple: true  // 明確指定多選模式
                            });
                            
                            // Select2 初始化後，再次設置值
                            if (selectedValues.length > 0) {
                                // 確保所有值都是字符串格式，並且與選項值匹配
                                const stringValues = selectedValues.map(v => String(v).trim()).filter(v => {
                                    return Array.from(valueSelect.options).some(opt => String(opt.value).trim() === v);
                                });
                                console.log(`updateAttributeValueField - Select2 初始化後設置值:`, stringValues); // 調試用
                                if (stringValues.length > 0) {
                                    $(valueSelect).val(stringValues).trigger('change');
                                    // 驗證設置是否成功
                                    setTimeout(() => {
                                        const currentVal = $(valueSelect).val();
                                        console.log(`updateAttributeValueField - Select2 當前值:`, currentVal); // 調試用
                                    }, 100);
                                }
                            } else {
                                $(valueSelect).trigger('change');
                            }
                            
                            // 觸發 change 事件以更新 Select2 顯示
                            $(valueSelect).trigger('change');
                        } catch (error) {
                            console.error('Failed to initialize Select2 for attribute value:', error);
                        }
                    }, 300);
                }
            } else {
                // Input 類型
                container.innerHTML = `<input type="text" class="form-control attribute-value" id="attr_value_${rowId}" value="${currentValue}" placeholder="輸入屬性值">`;
            }
        } catch (error) {
            console.error('Failed to load attribute details:', error);
            // 默認使用 input
            const container = document.getElementById(`attr_value_container_${rowId}`);
            if (container) {
                const currentValueInput = container.querySelector('.attribute-value');
                const currentValue = currentValueInput ? currentValueInput.value : '';
                const valuePh = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('products.attributes.valuePlaceholder') : '輸入屬性值';
                container.innerHTML = '<input type="text" class="form-control attribute-value" id="attr_value_' + rowId + '" value="' + currentValue + '" placeholder="' + valuePh + '" data-i18n-placeholder="products.attributes.valuePlaceholder">';
            }
        }
    }
    
    // 刪除產品屬性行
    removeProductAttribute(attributeId) {
        // 使用更具體的選擇器，確保找到正確的行
        const row = document.querySelector(`#productAttributesList tr[data-attribute-id="${attributeId}"]`);
        if (row) {
            row.remove();
        } else {
            // 如果找不到，嘗試通過按鈕的父元素找到行
            const button = document.querySelector(`button[onclick*="removeProductAttribute('${attributeId}')"]`);
            if (button) {
                const tr = button.closest('tr');
                if (tr) {
                    tr.remove();
                }
            }
        }
        
        // 如果沒有屬性了，顯示提示
        const attributesList = document.getElementById('productAttributesList');
        if (attributesList) {
            const remainingRows = attributesList.querySelectorAll('tr[data-attribute-id]');
            if (remainingRows.length === 0) {
            const emptyText = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('products.attributes.empty') : '暫無屬性';
            attributesList.innerHTML = '<tr><td colspan="3" class="text-center text-muted" data-i18n="products.attributes.empty">' + emptyText + '</td></tr>';
            }
        }

        // 重新計算操作列寬度
        this.calculateProductAttributeActionColumnWidth?.();
    }
    
    // 收集產品屬性數據
    collectProductAttributes() {
        const attributes = [];
        const rows = document.querySelectorAll('#productAttributesList tr[data-attribute-id]');
        
        rows.forEach(row => {
            const attributeId = row.getAttribute('data-attribute-id');
            const fieldSelect = document.getElementById(`attr_field_${attributeId}`);
            const valueInput = document.getElementById(`attr_value_${attributeId}`);
            
            if (fieldSelect && fieldSelect.value) {
                // 確保 attribute_id 是有效的 UUID 字符串
                const attrId = fieldSelect.value.trim();
                if (attrId && attrId !== '') {
                    let value = '';
                    if (valueInput) {
                        // 如果是 multi-select，收集所有選中的值
                        if (valueInput.multiple) {
                            // 優先使用 Select2 的值（如果已初始化）
                            if (typeof $ !== 'undefined' && $(valueInput).hasClass('select2-hidden-accessible')) {
                                const select2Values = $(valueInput).val();
                                if (Array.isArray(select2Values)) {
                                    value = select2Values.filter(v => v).join(',');
                                } else if (select2Values) {
                                    value = String(select2Values);
                                }
                            } else {
                                // 如果 Select2 未初始化，使用原生方式
                                const selectedOptions = Array.from(valueInput.selectedOptions);
                                value = selectedOptions.map(opt => opt.value).filter(v => v).join(',');
                            }
                        } else {
                            // 單選或 input
                            value = valueInput.value ? valueInput.value.trim() : '';
                        }
                    }
                    
                    // 只有當有值時才添加（multi-select 可能沒有選中任何項）
                    if (value) {
                    attributes.push({
                        attribute_id: attrId,
                            value: value
                    });
                    }
                }
            }
        });
        
        return attributes;
    }
    
    // 載入現有產品屬性（編輯模式）
    async loadProductAttributes() {
        if (!this.itemId) return;
        
        // 防止重複加載
        if (this._loadingProductAttributes) return;
        this._loadingProductAttributes = true;
        
        try {
            const product = await App.apiRequest(`${this.config.apiPath}/${this.itemId}`);
            
            // 從產品數據中獲取屬性值
            if (product.product_attribute_values && product.product_attribute_values.length > 0) {
                const attributesList = document.getElementById('productAttributesList');
                if (attributesList) {
                    // 清空現有內容（包括所有行，包括"暫無屬性"提示）
                    attributesList.innerHTML = '';
                    
                    // 先載入所有產品屬性選項
                    const attributes = await App.apiRequest('/product-attributes?limit=1000');
                    const attributeMap = {};
                    (attributes.data || []).forEach(attr => {
                        attributeMap[attr.id] = attr.name;
                    });
                    
                    // 為每個屬性值創建一行（使用 Map 按 attribute_id 分組，因為一個屬性可能有多個值）
                    const attributeValueMap = new Map();
                    for (const attrValue of product.product_attribute_values) {
                        const attrId = attrValue.attribute_id;
                        if (!attributeValueMap.has(attrId)) {
                            attributeValueMap.set(attrId, []);
                        }
                        // 如果值是逗號分隔的字符串，拆分為數組
                        if (attrValue.value) {
                            const valueStr = String(attrValue.value);
                            const values = valueStr.includes(',') 
                                ? valueStr.split(',').map(v => v.trim()).filter(v => v)
                                : [valueStr.trim()];
                            // 只添加非空值
                            values.forEach(v => {
                                if (v) {
                                    attributeValueMap.get(attrId).push(v);
                                }
                            });
                        }
                    }
                    
                    // 為每個屬性創建一行（合併多個值）
                    for (const [attrId, values] of attributeValueMap.entries()) {
                        const attr = (attributes.data || []).find(a => a.id === attrId);
                        if (attr) {
                            // 去重並合併值
                            const uniqueValues = [...new Set(values)];
                            const valueString = uniqueValues.join(',');
                            console.log(`加載產品屬性: ${attr.name}, 值: ${valueString}`); // 調試用
                            await this.addProductAttribute(attrId, valueString);
                            // 等待 Select2 初始化完成
                            await new Promise(resolve => setTimeout(resolve, 200));
                        }
                    }
                }
            }
        } catch (error) {
            console.error('Failed to load product attributes:', error);
        } finally {
            this._loadingProductAttributes = false;
        }
    }
    
    // 保存優惠券條件
    async saveConditions(couponId) {
        if (!this.pendingConditions || this.pendingConditions.length === 0) return;
        
        try {
            // 先獲取現有條件
            const existingConditions = await App.apiRequest(`/coupons/${couponId}/conditions`);
            const existingIds = (existingConditions || []).map(c => c.id);
            
            // 刪除不再存在的條件
            const newIds = this.pendingConditions.filter(c => c.id).map(c => c.id);
            const toDelete = existingIds.filter(id => !newIds.includes(id));
            
            for (const id of toDelete) {
                await App.apiRequest(`/coupons/${couponId}/conditions/${id}`, {
                    method: 'DELETE'
                });
            }
            
            // 保存或更新條件
            for (const condition of this.pendingConditions) {
                if (condition.id) {
                    // 更新現有條件
                    await App.apiRequest(`/coupons/${couponId}/conditions/${condition.id}`, {
                        method: 'PUT',
                        body: JSON.stringify(condition)
                    });
                } else {
                    // 創建新條件
                    await App.apiRequest(`/coupons/${couponId}/conditions`, {
                        method: 'POST',
                        body: JSON.stringify(condition)
                    });
                }
            }
        } catch (error) {
            console.error('保存條件失敗:', error);
            App.showAlert('保存條件時發生錯誤: ' + error.message, 'warning');
        }
    }
    
    // 顯示圖片放大彈窗（參考 dynamic-list.js）
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
    
    // 刪除圖片
    deleteImage() {
        if (!confirm('確定要刪除這張圖片嗎？')) {
            return;
        }
        
        const imageInput = document.getElementById('field_image_url');
        const preview = document.getElementById('imagePreview');
        const previewImg = document.getElementById('previewImg');
        
        // 清空文件輸入
        if (imageInput) {
            imageInput.value = '';
        }
        
        // 完全隱藏預覽（不顯示 imagePreview）
        if (preview) {
            preview.style.display = 'none';
        }
        
        // 清空圖片源
        if (previewImg) {
            // 移除 src 屬性而不是設置為空字符串，避免觸發錯誤
            previewImg.removeAttribute('src');
            previewImg.setAttribute('data-error-handled', 'true');
            previewImg.onclick = null;
        }
        
        // 標記圖片已刪除（用於提交表單時）
        this.imageDeleted = true;
        this.originalImageUrl = null;
    }
    
    // 檢查預約衝突
    async checkAppointmentConflict(fieldKey, selectedId) {
        if (this.pageName !== 'appointments') return;
        
        // 獲取開始和結束時間
        const startTimeInput = document.getElementById('field_start_time');
        const endTimeInput = document.getElementById('field_end_time');
        
        if (!startTimeInput || !endTimeInput || !startTimeInput.value || !endTimeInput.value) {
            return; // 如果時間未設置，不檢查衝突
        }
        
        const startTime = new Date(startTimeInput.value);
        const endTime = new Date(endTimeInput.value);
        
        // 獲取當前選擇的所有資源ID
        const roomIds = this.getSelectedIds('room_ids');
        const equipmentIds = this.getSelectedIds('equipment_ids');
        const vehicleIds = this.getSelectedIds('vehicle_ids');
        
        // 如果當前選擇的資源不在對應的數組中，添加它
        if (fieldKey === 'room_ids' && !roomIds.includes(selectedId)) {
            roomIds.push(selectedId);
        } else if (fieldKey === 'equipment_ids' && !equipmentIds.includes(selectedId)) {
            equipmentIds.push(selectedId);
        } else if (fieldKey === 'vehicle_ids' && !vehicleIds.includes(selectedId)) {
            vehicleIds.push(selectedId);
        }
        
        // 獲取當前預約ID（如果是編輯模式）
        const excludeId = this.isEdit && this.itemId ? this.itemId : null;
        
        try {
            const response = await App.apiRequest('/api/v1/appointments/check-conflict', {
                method: 'POST',
                body: JSON.stringify({
                    start_time: startTime.toISOString(),
                    end_time: endTime.toISOString(),
                    room_ids: roomIds,
                    equipment_ids: equipmentIds,
                    vehicle_ids: vehicleIds,
                    exclude_id: excludeId
                })
            });
            
            if (response.has_conflict && response.conflicts && response.conflicts.length > 0) {
                // 檢查選中的資源是否允許重複使用
                const resourceType = fieldKey === 'room_ids' ? 'room' : (fieldKey === 'equipment_ids' ? 'equipment' : 'vehicle');
                const resourceApi = fieldKey === 'room_ids' ? '/api/v1/rooms' : (fieldKey === 'equipment_ids' ? '/api/v1/equipments' : '/api/v1/vehicles');
                
                try {
                    const resource = await App.apiRequest(`${resourceApi}/${selectedId}`);
                    if (resource && resource.allow_overlap) {
                        // 允許重複使用，不阻止選擇
                        return;
                    }
                } catch (error) {
                    console.warn('無法獲取資源信息:', error);
                }
                
                // 有衝突且不允許重複使用，阻止選擇並顯示警告
                const conflictMessages = response.conflicts.join('\n');
                App.showAlert(`時間衝突：\n${conflictMessages}`, 'warning');
                
                // 移除剛選擇的項目
                const select = document.getElementById(`field_${fieldKey}`);
                if (select && typeof $ !== 'undefined') {
                    const currentValues = $(select).val() || [];
                    const newValues = currentValues.filter(id => id !== selectedId);
                    $(select).val(newValues).trigger('change');
                }
            }
        } catch (error) {
            console.error('檢查衝突失敗:', error);
            // 不阻止選擇，但記錄錯誤
        }
    }
    
    // 獲取選中的ID數組
    getSelectedIds(fieldKey) {
        const select = document.getElementById(`field_${fieldKey}`);
        if (!select) return [];
        
        if (typeof $ !== 'undefined' && $(select).hasClass('select2-hidden-accessible')) {
            return $(select).val() || [];
        }
        
        const selected = [];
        for (const option of select.options) {
            if (option.selected) {
                selected.push(option.value);
            }
        }
        return selected;
    }
    
    // Payment methods 特殊逻辑：根据付款形式动态调整字段
    handlePaymentTypeChange() {
        const paymentTypeField = document.getElementById('field_payment_type');
        const nameField = document.getElementById('field_name');
        const nameFieldContainer = nameField ? nameField.closest('.mb-3') : null;
        const isOnlinePaymentField = document.getElementById('field_is_online_payment');
        const codeField = document.getElementById('field_code');
        
        if (!paymentTypeField) return;
        
        const paymentType = paymentTypeField.value;
        const isGateway = paymentType === 'gateway';
        const isStripeConnect = paymentType === 'stripe_connect';

        const gatewayOptions = [
            { code: 'stripe', label: 'Stripe' },
            { code: 'paypal', label: 'PayPal' },
            { code: 'alipay', label: 'Alipay' },
            { code: 'wechat_pay', label: 'WeChat Pay' },
            { code: 'apple_pay', label: 'Apple Pay' },
            { code: 'google_pay', label: 'Google Pay' },
            { code: 'fps', label: 'FPS' },
            { code: 'payme', label: 'PayMe' },
            { code: 'alipay_hk', label: 'Alipay HK' },
            { code: 'wechat_hk', label: 'WeChat Pay HK' },
            { code: 'boc_pay', label: 'BoC Pay' },
            { code: 'octopus', label: 'Octopus' },
            { code: 'unionpay', label: 'UnionPay' }
        ];

        const stripeGatewayCodes = ['stripe', 'alipay', 'wechat_pay', 'apple_pay', 'google_pay'];
        const paypalGatewayCodes = ['paypal'];
        const qfpayGatewayCodes = ['fps', 'payme', 'alipay_hk', 'wechat_hk', 'boc_pay', 'octopus', 'unionpay'];
        
        // Remove any existing Stripe Connect info banner
        const existingBanner = document.getElementById('stripe_connect_info');
        if (existingBanner) existingBanner.remove();

        // 線上閘道 or Stripe Connect：code 必須只讀
        if (codeField) {
            if (isGateway || isStripeConnect) {
                codeField.readOnly = true;
                codeField.classList.add('bg-light', 'text-muted');
            } else {
                codeField.readOnly = false;
                codeField.classList.remove('bg-light', 'text-muted');
            }
        }

        // Stripe Connect: 自動設定 name、code、is_online_payment，並顯示 link
        if (isStripeConnect) {
            // Auto-fill fields
            if (nameField) {
                // Restore to text input if it was a dropdown
                if (nameField.tagName === 'SELECT') {
                    const input = document.createElement('input');
                    input.type = 'text';
                    input.id = 'field_name';
                    input.className = 'form-control bg-light text-muted';
                    input.readOnly = true;
                    input.value = 'Stripe Connect';
                    nameField.replaceWith(input);
                } else {
                    nameField.value = 'Stripe Connect';
                    nameField.readOnly = true;
                    nameField.classList.add('bg-light', 'text-muted');
                }
            }
            if (codeField) {
                codeField.value = 'stripe_connect';
            }
            if (isOnlinePaymentField) {
                isOnlinePaymentField.value = 'true';
                isOnlinePaymentField.disabled = true;
                isOnlinePaymentField.classList.add('bg-light', 'text-muted');
            }

            // Hide all gateway fields
            const gatewayFieldIds = ['field_stripe_api_key', 'field_stripe_secret_key', 'field_paypal_client_id', 'field_paypal_secret', 'field_qfpay_app_code', 'field_qfpay_client_key', 'field_qfpay_base_url', 'field_currency'];
            gatewayFieldIds.forEach(id => {
                const field = document.getElementById(id);
                if (field) {
                    const container = field.closest('.mb-3');
                    if (container) container.style.display = 'none';
                    field.value = '';
                }
            });

            // Hide card terminal fields
            ['field_extra_fields.use_card_terminal', 'field_extra_fields.card_terminal_id', 'field_extra_fields.card_terminal_type'].forEach(id => {
                const field = document.getElementById(id);
                if (field) {
                    const container = field.closest('.mb-3');
                    if (container) container.style.display = 'none';
                    field.value = '';
                }
            });

            // Show info banner with link to Stripe Connect setup page
            const banner = document.createElement('div');
            banner.id = 'stripe_connect_info';
            banner.className = 'alert alert-info mt-3';
            const _t = (key) => (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('pages.stripeConnect.' + key) : key;
            const setupText = _t('infoBannerSetup')
                .replace('{linkStart}', '<a href="/stripe-connect" class="alert-link">')
                .replace('{linkEnd}', '</a>');
            banner.innerHTML = `
                <i class="bi bi-info-circle me-2"></i>
                <strong>${_t('infoBannerTitle')}</strong> — ${_t('infoBannerDesc')}
                ${setupText}
                ${_t('infoBannerDone')}
            `;
            const form = paymentTypeField.closest('form') || paymentTypeField.closest('.card-body');
            if (form) form.appendChild(banner);
            return;
        }
        
        // 線上閘道：code 必須只讀（避免被手動改壞）
        if (codeField) {
            if (isGateway) {
                codeField.readOnly = true;
                codeField.classList.add('bg-light', 'text-muted');
            } else {
                codeField.readOnly = false;
                codeField.classList.remove('bg-light', 'text-muted');
            }
        }
        
        // 如果是線上閘道，将名称字段变成 dropdown（paypal/stripe）
        if (isGateway && nameFieldContainer && nameField) {
            // 检查是否已经是 dropdown
            if (nameField.tagName !== 'SELECT') {
                // 保存当前值
                const currentValue = nameField.value || '';
                
                // 创建新的 select 元素
                const select = document.createElement('select');
                select.id = 'field_name';
                select.className = 'form-select';
                select.required = true;
                const pleaseSelect = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('common.pleaseSelect') : '請選擇';
                const currentCode = (codeField && codeField.value ? String(codeField.value).toLowerCase() : '').trim();
                const fallbackFromName = String(currentValue || '').trim().toLowerCase();
                const inferredCode = currentCode || (fallbackFromName === 'paypal' ? 'paypal' : (fallbackFromName === 'stripe' ? 'stripe' : ''));

                select.innerHTML = `<option value="">${pleaseSelect}</option>` + gatewayOptions.map(opt => {
                    const selected = inferredCode === opt.code ? 'selected' : '';
                    return `<option value="${opt.code}" data-label="${opt.label}" ${selected}>${opt.label}</option>`;
                }).join('');
                
                // 替换 input
                nameField.replaceWith(select);
                
                // 更新 code 字段
                if (codeField && !codeField.value) {
                    codeField.value = inferredCode;
                }
                
                // 监听名称变化，自动更新 code
                select.addEventListener('change', () => {
                    if (codeField) {
                        codeField.value = select.value.toLowerCase() || '';
                    }
                    const selectedOption = select.options[select.selectedIndex];
                    if (selectedOption && selectedOption.dataset && selectedOption.dataset.label) {
                        select.dataset.selectedLabel = selectedOption.dataset.label;
                    }
                    // 重新调用以更新网关字段显示
                    this.handlePaymentTypeChange();
                });
            }
        } else if (!isGateway && nameFieldContainer && nameField) {
            // 如果是普通類型，將 dropdown 變回 text input，並移除 readOnly
            if (nameField.tagName === 'SELECT') {
                const selectedOption = nameField.options[nameField.selectedIndex];
                const currentLabel = (selectedOption && selectedOption.dataset && selectedOption.dataset.label)
                    ? selectedOption.dataset.label
                    : (nameField.dataset.selectedLabel || '');
                const input = document.createElement('input');
                input.type = 'text';
                input.id = 'field_name';
                input.className = 'form-control';
                input.required = true;
                input.value = currentLabel;
                nameField.replaceWith(input);
            } else {
                nameField.readOnly = false;
                nameField.classList.remove('bg-light', 'text-muted');
            }
        }
        
        // 如果是線上閘道，将網店付款方式设置为 yes (readonly)
        if (isGateway && isOnlinePaymentField) {
            isOnlinePaymentField.value = 'true';
            isOnlinePaymentField.disabled = true;
            isOnlinePaymentField.classList.add('bg-light', 'text-muted');
        } else if (!isGateway && isOnlinePaymentField) {
            // 普通：強制 default 否
            isOnlinePaymentField.value = 'false';
            isOnlinePaymentField.disabled = false;
            isOnlinePaymentField.classList.remove('bg-light', 'text-muted');
        }
        
        // 显示/隐藏 Stripe 和 PayPal 连接字段
        const stripeApiKeyField = document.getElementById('field_stripe_api_key');
        const stripeSecretKeyField = document.getElementById('field_stripe_secret_key');
        const paypalClientIdField = document.getElementById('field_paypal_client_id');
        const paypalSecretField = document.getElementById('field_paypal_secret');
        const qfpayAppCodeField = document.getElementById('field_qfpay_app_code');
        const qfpayClientKeyField = document.getElementById('field_qfpay_client_key');
        const qfpayBaseURLField = document.getElementById('field_qfpay_base_url');
        const currencyField = document.getElementById('field_currency');
        
        const gatewayFields = [
            stripeApiKeyField, stripeSecretKeyField,
            paypalClientIdField, paypalSecretField,
            qfpayAppCodeField, qfpayClientKeyField, qfpayBaseURLField, currencyField
        ];
        // 普通：全隱藏，並清空值，避免誤保存
        if (!isGateway) {
        gatewayFields.forEach(field => {
                if (!field) return;
                const container = field.closest('.mb-3');
                if (container) container.style.display = 'none';
                field.value = '';
        });
            return;
        }
        
        // 根据选择的名称显示对应的连接字段
        if (isGateway) {
            const currentNameEl = document.getElementById('field_name');
            const selectedCode = currentNameEl ? String(currentNameEl.value || '').toLowerCase() : '';
            const selectedOption = currentNameEl && currentNameEl.tagName === 'SELECT'
                ? currentNameEl.options[currentNameEl.selectedIndex]
                : null;
            const selectedLabel = selectedOption && selectedOption.dataset ? selectedOption.dataset.label : '';
            
            // 先全部隱藏
            gatewayFields.forEach(field => {
                if (!field) return;
                const container = field.closest('.mb-3');
                if (container) container.style.display = 'none';
            });

            if (codeField) {
                codeField.value = selectedCode;
            }
            const currentNameInput = document.getElementById('field_name');
            if (currentNameInput && currentNameInput.tagName === 'SELECT' && selectedLabel) {
                currentNameInput.dataset.selectedLabel = selectedLabel;
            }
            
            const useStripeFields = stripeGatewayCodes.includes(selectedCode);
            const usePayPalFields = paypalGatewayCodes.includes(selectedCode);
            const useQFPayFields = qfpayGatewayCodes.includes(selectedCode);

            if (useStripeFields && stripeApiKeyField && stripeSecretKeyField) {
                const stripeContainer = stripeApiKeyField.closest('.mb-3');
                if (stripeContainer) {
                    stripeContainer.style.display = 'block';
                }
                const stripeSecretContainer = stripeSecretKeyField.closest('.mb-3');
                if (stripeSecretContainer) {
                    stripeSecretContainer.style.display = 'block';
                }
            }
            if (usePayPalFields && paypalClientIdField && paypalSecretField) {
                const paypalContainer = paypalClientIdField.closest('.mb-3');
                if (paypalContainer) {
                    paypalContainer.style.display = 'block';
                }
                const paypalSecretContainer = paypalSecretField.closest('.mb-3');
                if (paypalSecretContainer) {
                    paypalSecretContainer.style.display = 'block';
                }
            }
            if (useQFPayFields) {
                [qfpayAppCodeField, qfpayClientKeyField, qfpayBaseURLField].forEach(field => {
                    if (!field) return;
                    const container = field.closest('.mb-3');
                    if (container) container.style.display = 'block';
                });
            }
            if (currencyField && selectedCode) {
                const currencyContainer = currencyField.closest('.mb-3');
                if (currencyContainer) currencyContainer.style.display = 'block';
            }

            // 清空非當前 gateway 的欄位，避免誤保存
            if (!useStripeFields) {
                if (stripeApiKeyField) stripeApiKeyField.value = '';
                if (stripeSecretKeyField) stripeSecretKeyField.value = '';
            }
            if (!usePayPalFields) {
                if (paypalClientIdField) paypalClientIdField.value = '';
                if (paypalSecretField) paypalSecretField.value = '';
            }
            if (!useQFPayFields) {
                if (qfpayAppCodeField) qfpayAppCodeField.value = '';
                if (qfpayClientKeyField) qfpayClientKeyField.value = '';
                if (qfpayBaseURLField) qfpayBaseURLField.value = '';
            }
            if (!selectedCode && currencyField) {
                currencyField.value = '';
            }
        }
        
        // 卡機相關欄位顯示/隱藏
        const isCardTerminal = paymentType === 'card_terminal';
        const cardTerminalFields = [
            'field_extra_fields.use_card_terminal',
            'field_extra_fields.card_terminal_id',
            'field_extra_fields.card_terminal_type'
        ];
        
        cardTerminalFields.forEach(fieldId => {
            const field = document.getElementById(fieldId);
            if (!field) return;
            const container = field.closest('.mb-3');
            if (container) {
                container.style.display = isCardTerminal ? 'block' : 'none';
            }
            // 非卡機模式時清空值
            if (!isCardTerminal) {
                field.value = '';
            }
        });
        
        // 卡機模式：自動設置 use_card_terminal 為 true
        if (isCardTerminal) {
            const useTerminalField = document.getElementById('field_extra_fields.use_card_terminal');
            if (useTerminalField && !useTerminalField.value) {
                useTerminalField.value = 'true';
            }
        }
    }
}

// 全局函數：切換服務套票對應服務顯示
function toggleServicePackageService(selectElement) {
    if (window.dynamicForm && typeof window.dynamicForm.toggleServicePackageService === 'function') {
        window.dynamicForm.toggleServicePackageService(selectElement);
    }
}
