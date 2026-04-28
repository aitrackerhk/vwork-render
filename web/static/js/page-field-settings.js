/**
 * Page Field Settings - 靜態頁面欄位設定功能
 * 用於 orders_new, service_orders_new, purchase_orders_new 等頁面
 * 
 * 使用方式:
 * 1. 在頁面引入此 JS 文件
 * 2. 調用 PageFieldSettings.init({ pageName: 'orders_new', containerSelector: '#data-pane', insertAfterSelector: '#backToListBtn' })
 */

window.PageFieldSettings = (function() {
    'use strict';
    
    let currentSettings = null;
    let pageName = '';
    let containerSelector = '';
    let fieldDefinitions = [];
    
    // 從 App 獲取翻譯
    function getText(key, fallback) {
        if (window.App && window.App.getTranslation) {
            return window.App.getTranslation(key, fallback);
        }
        return fallback;
    }

    // 欄位設定權限檢查（無權限則不顯示設定按鈕）
    function canConfigureFieldSettings() {
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
    
    // 從 API 載入欄位設定
    async function loadFieldSettingsFromAPI() {
        try {
            const response = await App.apiRequest(`/user/form-field-settings/${encodeURIComponent(pageName)}`);
            if (response && response.field_config) {
                return response.field_config;
            }
        } catch (error) {
            console.log('No existing field settings found, using defaults');
        }
        return null;
    }
    
    // 保存欄位設定到 API
    async function saveFieldSettingsToAPI(settings) {
        try {
            await App.apiRequest(`/user/form-field-settings/${encodeURIComponent(pageName)}`, {
                method: 'POST',
                body: JSON.stringify({ field_config: settings })
            });
            return true;
        } catch (error) {
            console.error('Failed to save field settings:', error);
            return false;
        }
    }
    
    // 收集頁面上的欄位定義
    function collectFieldDefinitions(containerEl) {
        const fields = [];
        const processedKeys = new Set();
        
        // 找所有有 id 的表單元素
        containerEl.querySelectorAll('input[id], select[id], textarea[id]').forEach(el => {
            const key = el.id;
            if (processedKeys.has(key)) return;
            if (key.startsWith('_') || key.includes('Modal') || key.includes('modal')) return;
            
            // 找對應的 label
            let label = key;
            const labelEl = containerEl.querySelector(`label[for="${key}"]`);
            if (labelEl) {
                label = labelEl.textContent.trim().replace(/\s*\*\s*$/, '');
            } else {
                // 嘗試找同一個 mb-3 容器內的 label
                const wrapper = el.closest('.mb-3');
                if (wrapper) {
                    const wrapperLabel = wrapper.querySelector('.form-label');
                    if (wrapperLabel) {
                        label = wrapperLabel.textContent.trim().replace(/\s*\*\s*$/, '');
                    }
                }
            }
            
            processedKeys.add(key);
            fields.push({
                key: key,
                label: label,
                element: el
            });
        });
        
        return fields;
    }
    
    // 初始化默認設定
    function initDefaultSettings(fields) {
        return {
            fields: fields.map((f, idx) => ({
                key: f.key,
                visible: true,
                order: idx
            })),
            extraFields: [],
            categories: []
        };
    }
    
    // 應用欄位設定到頁面
    function applyFieldSettings() {
        if (!currentSettings || !currentSettings.fields) return;
        
        const containerEl = document.querySelector(containerSelector);
        if (!containerEl) return;
        
        currentSettings.fields.forEach(sf => {
            // 找到欄位元素
            const el = containerEl.querySelector(`#${CSS.escape(sf.key)}`);
            if (!el) return;
            
            // 找到欄位的容器（通常是 .mb-3 或 .col-md-*）
            const fieldWrapper = el.closest('.mb-3') || el.closest('.col-md-6') || el.closest('.col-md-12');
            if (!fieldWrapper) return;
            
            // 應用顯示/隱藏
            if (sf.visible === false) {
                fieldWrapper.style.display = 'none';
                fieldWrapper.setAttribute('data-field-hidden', 'true');
                // 如果欄位是 required，暫時移除以避免提交失敗
                if (el.hasAttribute('required')) {
                    el.removeAttribute('required');
                    el.setAttribute('data-was-required', 'true');
                }
            } else {
                fieldWrapper.style.display = '';
                fieldWrapper.removeAttribute('data-field-hidden');
                // 恢復 required 屬性
                if (el.hasAttribute('data-was-required')) {
                    el.setAttribute('required', '');
                    el.removeAttribute('data-was-required');
                }
            }

            // 應用預設值（跳過 readonly 欄位）
            if (sf.defaultValue !== undefined && sf.defaultValue !== null) {
                // 檢查是否為 readonly 欄位
                const isReadonly = el.hasAttribute('readonly') || el.hasAttribute('disabled');
                if (isReadonly) {
                    console.log(`[PageFieldSettings] 欄位 ${sf.key} 是 readonly，跳過預設值設定`);
                    return;
                }
                
                const value = String(sf.defaultValue);
                if (el.tagName === 'SELECT') {
                    if (el.multiple) {
                        const values = value.split(',').map(v => v.trim()).filter(Boolean);
                        Array.from(el.options).forEach(opt => {
                            opt.selected = values.includes(opt.value);
                        });
                    } else if (!el.value) {
                        el.value = value;
                    }
                } else if (el.type === 'checkbox' || el.type === 'radio') {
                    if (el.type === 'checkbox') {
                        el.checked = value === 'true' || value === '1';
                    }
                } else if (!el.value) {
                    el.value = value;
                }
                
                // 觸發 change 事件
                el.dispatchEvent(new Event('change', { bubbles: true }));
            }
        });
    }
    
    // 創建欄位設定按鈕
    function createSettingsButton(insertAfterEl) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'btn btn-outline-primary ms-2';
        btn.id = 'pageFieldSettingsBtn';
        btn.innerHTML = `<i class="bi bi-gear"></i> <span data-i18n="common.fieldSettings">${getText('common.fieldSettings', '欄位設定')}</span>`;
        btn.addEventListener('click', openFieldSettingsModal);
        
        insertAfterEl.parentNode.insertBefore(btn, insertAfterEl.nextSibling);
        return btn;
    }
    
    // 打開欄位設定 Modal
    function openFieldSettingsModal() {
        const modalId = 'pageFieldSettingsModal';
        
        // 移除舊的 Modal
        const existingModal = document.getElementById(modalId);
        if (existingModal) {
            existingModal.remove();
        }
        
        // 排序欄位
        const sortedFields = [...currentSettings.fields].sort((a, b) => a.order - b.order);
        
        // 找欄位標籤
        const getFieldLabel = (key) => {
            const def = fieldDefinitions.find(f => f.key === key);
            return def ? def.label : key;
        };
        
        const getFieldTypeLabel = (key) => {
            const extraFieldDef = currentSettings.extraFields?.find(ef => ef.key === key);
            if (extraFieldDef?.type) {
                return extraFieldDef.type;
            }
            const def = fieldDefinitions.find(f => f.key === key);
            const el = def?.element;
            if (!el) return 'text';
            if (el.tagName === 'SELECT') return 'select';
            if (el.tagName === 'TEXTAREA') return 'textarea';
            if (el.tagName === 'INPUT') {
                return el.type || 'text';
            }
            return 'text';
        };

        const getTypeBadge = (typeLabel, key) => {
            const typeMap = {
                text: getText('common.fieldTypeText', '文字'),
                number: getText('common.fieldTypeNumber', '數字'),
                date: getText('common.fieldTypeDate', '日期'),
                textarea: getText('common.fieldTypeTextarea', '多行文字'),
                email: getText('common.fieldTypeEmail', '郵箱'),
                select: getText('common.fieldTypeSelect', '下拉選單'),
                password: getText('common.fieldTypePassword', '密碼')
            };
            const label = typeMap[typeLabel] || typeLabel;
            return `<span class="badge bg-secondary ms-2">${label}</span>`;
        };

        // 生成預設值輸入 HTML（根據欄位類型）
        const getDefaultValueInputHtml = (sf) => {
            const fieldKey = sf.key;
            const defaultValue = sf.defaultValue !== undefined && sf.defaultValue !== null ? sf.defaultValue : '';
            const typeLabel = getFieldTypeLabel(fieldKey);
            const def = fieldDefinitions.find(f => f.key === fieldKey);
            const el = def?.element;
            const extraFieldDef = currentSettings.extraFields?.find(ef => ef.key === fieldKey);
            
            // 針對 select 類型，複製原始選項
            if (typeLabel === 'select') {
                let optionsHtml = '<option value="">-- ' + getText('common.selectPlaceholder', '請選擇') + ' --</option>';
                
                if (el && el.tagName === 'SELECT') {
                    // 從原始 select 元素複製所有選項
                    Array.from(el.options).forEach(opt => {
                        // 跳過空白或佔位符選項
                        if (opt.value === '' && opt.textContent.includes('--')) {
                            return;
                        }
                        const selected = opt.value === defaultValue ? 'selected' : '';
                        const escapedValue = opt.value.replace(/"/g, '&quot;');
                        const escapedText = opt.textContent.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                        optionsHtml += '<option value="' + escapedValue + '" ' + selected + '>' + escapedText + '</option>';
                    });
                } else if (extraFieldDef?.options) {
                    // 額外欄位的選項
                    extraFieldDef.options.forEach(optVal => {
                        const selected = optVal === defaultValue ? 'selected' : '';
                        const escapedValue = optVal.replace(/"/g, '&quot;');
                        optionsHtml += '<option value="' + escapedValue + '" ' + selected + '>' + escapedValue + '</option>';
                    });
                }
                
                return '<select class="form-select form-select-sm field-default-input" data-field-key="' + fieldKey + '" data-field-type="select">' + optionsHtml + '</select>';
            }
            
            // 針對 checkbox/radio
            if (el && (el.type === 'checkbox' || el.type === 'radio')) {
                const checked = defaultValue === 'true' || defaultValue === '1' ? 'checked' : '';
                return '<div class="form-check"><input type="checkbox" class="form-check-input field-default-input" data-field-key="' + fieldKey + '" data-field-type="checkbox" ' + checked + '><label class="form-check-label small">' + getText('common.defaultChecked', '預設勾選') + '</label></div>';
            }
            
            // 針對 date
            if (typeLabel === 'date') {
                const escapedValue = defaultValue.replace(/"/g, '&quot;');
                return '<input type="date" class="form-control form-control-sm field-default-input" data-field-key="' + fieldKey + '" value="' + escapedValue + '">';
            }
            
            // 針對 number
            if (typeLabel === 'number') {
                const escapedValue = defaultValue.replace(/"/g, '&quot;');
                return '<input type="number" class="form-control form-control-sm field-default-input" data-field-key="' + fieldKey + '" value="' + escapedValue + '">';
            }
            
            // 針對 textarea
            if (typeLabel === 'textarea') {
                const escapedValue = defaultValue.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                return '<textarea class="form-control form-control-sm field-default-input" data-field-key="' + fieldKey + '" rows="2">' + escapedValue + '</textarea>';
            }
            
            // 預設使用 text input
            const escapedValue = String(defaultValue).replace(/"/g, '&quot;');
            return '<input type="text" class="form-control form-control-sm field-default-input" data-field-key="' + fieldKey + '" value="' + escapedValue + '">';
        };

        // 生成欄位列表 HTML
        const fieldsListHtml = sortedFields.map(sf => {
            const isExtraField = currentSettings.extraFields?.some(ef => ef.key === sf.key);
            const typeLabel = getFieldTypeLabel(sf.key);
            const defaultValueInputHtml = getDefaultValueInputHtml(sf);
            const hasDefaultValue = sf.defaultValue !== undefined && sf.defaultValue !== null && sf.defaultValue !== '';
            return `
                <div class="field-settings-item" data-field-key="${sf.key}" draggable="true">
                    <div class="field-drag-handle">
                        <i class="bi bi-grip-vertical"></i>
                    </div>
                    <div class="field-info">
                        <span class="field-label">${getFieldLabel(sf.key)}</span>
                        <span class="field-key text-muted">(${sf.key})</span>
                        ${isExtraField ? '<span class="badge bg-info ms-2">額外</span>' : ''}
                        ${getTypeBadge(typeLabel, sf.key)}
                        ${hasDefaultValue ? '<span class="badge bg-success ms-1" title="' + getText('common.hasDefaultValue', '已設定預設值') + '"><i class="bi bi-check-circle-fill"></i></span>' : ''}
                        <div class="field-default-value mt-2 d-none">
                            <label class="form-label small text-muted mb-1">
                                <i class="bi bi-pencil-square me-1"></i>${getText('common.defaultValue', '預設值')}
                            </label>
                            ${defaultValueInputHtml}
                        </div>
                    </div>
                    <div class="field-actions">
                        <button type="button" class="btn btn-sm btn-link field-visibility-toggle p-0" data-visible="${sf.visible ? 'true' : 'false'}" data-field-key="${sf.key}">
                            <i class="bi ${sf.visible ? 'bi-eye text-success' : 'bi-eye-slash text-muted'}" style="font-size: 1.2rem;"></i>
                        </button>
                        ${isExtraField ? `<button type="button" class="btn btn-sm btn-outline-danger delete-extra-field ms-2" data-key="${sf.key}"><i class="bi bi-trash"></i></button>` : ''}
                    </div>
                </div>
            `;
        }).join('');
        
        const modalHtml = `
            <div class="modal fade" id="${modalId}" tabindex="-1" aria-hidden="true">
                <div class="modal-dialog modal-lg">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title">
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
                            <div class="field-settings-list" id="pageFieldSettingsList">
                                ${fieldsListHtml}
                            </div>
                            <hr class="my-3">
                            <div class="add-extra-field-section">
                                <h6><i class="bi bi-plus-circle me-2"></i>${getText('common.addExtraField', '添加額外欄位')}</h6>
                                <div class="row g-2">
                                    <div class="col-md-4">
                                        <input type="text" class="form-control" id="pageExtraFieldKey" placeholder="${getText('common.fieldKey', '欄位 Key')}">
                                    </div>
                                    <div class="col-md-4">
                                        <input type="text" class="form-control" id="pageExtraFieldLabel" placeholder="${getText('common.fieldLabel', '欄位標籤')}">
                                    </div>
                                    <div class="col-md-3">
                                        <select class="form-select" id="pageExtraFieldType">
                                            <option value="text">${getText('common.fieldTypeText', '文字')}</option>
                                            <option value="number">${getText('common.fieldTypeNumber', '數字')}</option>
                                            <option value="date">${getText('common.fieldTypeDate', '日期')}</option>
                                            <option value="textarea">${getText('common.fieldTypeTextarea', '多行文字')}</option>
                                            <option value="email">${getText('common.fieldTypeEmail', '郵箱')}</option>
                                            <option value="select">${getText('common.fieldTypeSelect', '下拉選單')}</option>
                                        </select>
                                    </div>
                                    <div class="col-md-1">
                                        <button type="button" class="btn btn-primary w-100" id="pageAddExtraFieldBtn">
                                            <i class="bi bi-plus"></i>
                                        </button>
                                    </div>
                                </div>
                                <div class="row mt-2" id="pageSelectOptionsRow" style="display: none;">
                                    <div class="col-12">
                                        <select class="form-select" id="pageExtraFieldOptions" multiple style="width: 100%;"></select>
                                        <small class="text-muted">${getText('common.fieldTypeSelectOptions', '選項（用逗號分隔或按 Enter）')}</small>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">
                                ${getText('common.cancel', '取消')}
                            </button>
                            <button type="button" class="btn btn-primary" id="savePageFieldSettingsBtn">
                                <i class="bi bi-check-lg me-1"></i>${getText('common.save', '保存')}
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;
        
        document.body.insertAdjacentHTML('beforeend', modalHtml);
        
        const modal = new bootstrap.Modal(document.getElementById(modalId));
        bindModalEvents();
        modal.show();
        
        // Modal 顯示後禁用 readonly 欄位的預設值輸入
        document.getElementById(modalId).addEventListener('shown.bs.modal', () => {
            disableDefaultValueForReadonlyFields();
        }, { once: true });
    }
    
    // 禁用 readonly 欄位的預設值輸入
    function disableDefaultValueForReadonlyFields() {
        const listContainer = document.getElementById('pageFieldSettingsList');
        if (!listContainer) return;
        
        const containerEl = document.querySelector(containerSelector);
        if (!containerEl) return;
        
        fieldDefinitions.forEach(fd => {
            const el = fd.element;
            if (!el) return;
            
            // 檢查是否為 readonly 欄位
            const isReadonly = el.hasAttribute('readonly') || el.hasAttribute('disabled');
            if (!isReadonly) return;
            
            // 找到對應的欄位設定項目
            const item = listContainer.querySelector(`[data-field-key="${fd.key}"]`);
            if (!item) return;
            
            // 找到預設值輸入區域
            const defaultValueArea = item.querySelector('.field-default-value');
            if (!defaultValueArea) return;
            
            // 替換預設值區域內容為提示文字
            defaultValueArea.innerHTML = `
                <div class="text-muted small mt-1">
                    <i class="bi bi-lock me-1"></i>${getText('common.readonlyFieldNoDefault', 'Readonly 欄位無法設定預設值')}
                </div>
            `;
            
            // 在欄位設定項目上添加 Readonly 標記
            const fieldInfo = item.querySelector('.field-info');
            if (fieldInfo && !fieldInfo.querySelector('.badge.bg-warning')) {
                const badge = document.createElement('span');
                badge.className = 'badge bg-warning ms-2';
                badge.textContent = 'Readonly';
                // 插入到 field-default-value 之前
                const defaultDiv = fieldInfo.querySelector('.field-default-value');
                if (defaultDiv) {
                    fieldInfo.insertBefore(badge, defaultDiv);
                } else {
                    fieldInfo.appendChild(badge);
                }
            }
        });
    }
    
    // 保存前清理 readonly 欄位的預設值
    function sanitizeSettingsBeforeSave() {
        if (!currentSettings || !currentSettings.fields) return;
        
        const containerEl = document.querySelector(containerSelector);
        if (!containerEl) return;
        
        // 找出所有 readonly 欄位
        const readonlyFieldKeys = new Set();
        fieldDefinitions.forEach(fd => {
            const el = fd.element;
            if (el && (el.hasAttribute('readonly') || el.hasAttribute('disabled'))) {
                readonlyFieldKeys.add(fd.key);
            }
        });
        
        // 移除 readonly 欄位的預設值
        currentSettings.fields.forEach(sf => {
            if (readonlyFieldKeys.has(sf.key) && sf.defaultValue !== undefined) {
                console.log(`[PageFieldSettings] 移除 readonly 欄位 ${sf.key} 的預設值`);
                delete sf.defaultValue;
            }
        });
    }
    
    // 綁定 Modal 內的事件
    function bindModalEvents() {
        const listContainer = document.getElementById('pageFieldSettingsList');
        if (!listContainer) return;
        
        // Drag & Drop
        let draggedItem = null;
        
        listContainer.querySelectorAll('.field-settings-item').forEach(item => {
            item.addEventListener('dragstart', (e) => {
                draggedItem = item;
                item.classList.add('dragging');
                e.dataTransfer.effectAllowed = 'move';
            });
            
            item.addEventListener('dragend', () => {
                if (draggedItem) {
                    draggedItem.classList.remove('dragging');
                }
                listContainer.querySelectorAll('.field-settings-item').forEach(i => {
                    i.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
                });
                draggedItem = null;
            });
            
            item.addEventListener('dragover', (e) => {
                e.preventDefault();
                if (draggedItem && draggedItem !== item) {
                    const rect = item.getBoundingClientRect();
                    const midY = rect.top + rect.height / 2;
                    item.classList.remove('drag-over-top', 'drag-over-bottom');
                    if (e.clientY < midY) {
                        item.classList.add('drag-over-top');
                    } else {
                        item.classList.add('drag-over-bottom');
                    }
                }
            });
            
            item.addEventListener('dragleave', () => {
                item.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
            });
            
            item.addEventListener('drop', (e) => {
                e.preventDefault();
                if (draggedItem && draggedItem !== item) {
                    const rect = item.getBoundingClientRect();
                    const midY = rect.top + rect.height / 2;
                    if (e.clientY < midY) {
                        item.parentNode.insertBefore(draggedItem, item);
                    } else {
                        item.parentNode.insertBefore(draggedItem, item.nextSibling);
                    }
                }
                listContainer.querySelectorAll('.field-settings-item').forEach(i => {
                    i.classList.remove('drag-over', 'drag-over-top', 'drag-over-bottom');
                });
            });

            item.addEventListener('click', (e) => {
                if (e.target.closest('.field-actions') || e.target.closest('.field-drag-handle') || e.target.closest('button') || e.target.closest('input') || e.target.closest('select') || e.target.closest('textarea')) {
                    return;
                }
                const defaultRow = item.querySelector('.field-default-value');
                if (defaultRow) {
                    defaultRow.classList.toggle('d-none');
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
                const sf = currentSettings.fields.find(f => f.key === fieldKey);
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
            btn.addEventListener('click', () => {
                const fieldKey = btn.dataset.key;
                currentSettings.extraFields = currentSettings.extraFields.filter(ef => ef.key !== fieldKey);
                currentSettings.fields = currentSettings.fields.filter(f => f.key !== fieldKey);
                btn.closest('.field-settings-item').remove();
            });
        });

        // 預設值設定 - 支援多種輸入類型
        listContainer.querySelectorAll('.field-default-input').forEach(input => {
            input.addEventListener('change', (e) => {
                const el = e.currentTarget;
                const fieldKey = el.dataset.fieldKey;
                const fieldType = el.dataset.fieldType;
                const sf = currentSettings.fields.find(f => f.key === fieldKey);
                if (!sf) return;
                
                let value;
                if (fieldType === 'checkbox' || el.type === 'checkbox') {
                    value = el.checked ? 'true' : '';
                } else if (el.tagName === 'SELECT') {
                    value = el.value;
                } else if (el.tagName === 'TEXTAREA') {
                    value = el.value.trim();
                } else {
                    value = el.value.trim();
                }
                
                if (value === '') {
                    delete sf.defaultValue;
                    // 更新 badge
                    const item = el.closest('.field-settings-item');
                    const badge = item?.querySelector('.badge.bg-success');
                    if (badge) badge.remove();
                } else {
                    sf.defaultValue = value;
                    // 添加或保持 badge
                    const item = el.closest('.field-settings-item');
                    const fieldInfo = item?.querySelector('.field-info');
                    if (fieldInfo && !fieldInfo.querySelector('.badge.bg-success')) {
                        const typeBadge = fieldInfo.querySelector('.badge.bg-secondary');
                        if (typeBadge) {
                            typeBadge.insertAdjacentHTML('afterend', ' <span class="badge bg-success ms-1" title="' + getText('common.hasDefaultValue', '已設定預設值') + '"><i class="bi bi-check-circle-fill"></i></span>');
                        }
                    }
                }
            });
        });
        
        // 初始化 Select2 for options 和類型變更事件
        const typeSelect = document.getElementById('pageExtraFieldType');
        const optionsRow = document.getElementById('pageSelectOptionsRow');
        const optionsSelect = document.getElementById('pageExtraFieldOptions');
        
        if (typeSelect && optionsRow && optionsSelect) {
            // 初始化 Select2 tags 模式
            $(optionsSelect).select2({
                tags: true,
                tokenSeparators: [',', '\n'],
                placeholder: getText('common.fieldTypeSelectOptions', '選項（用逗號分隔或按 Enter）'),
                allowClear: true,
                width: '100%'
            });
            
            // 類型變更時顯示/隱藏選項輸入
            typeSelect.addEventListener('change', () => {
                if (typeSelect.value === 'select') {
                    optionsRow.style.display = 'block';
                } else {
                    optionsRow.style.display = 'none';
                    // 清空選項
                    $(optionsSelect).val(null).trigger('change');
                }
            });
        }
        
        // 添加額外欄位
        const addBtn = document.getElementById('pageAddExtraFieldBtn');
        if (addBtn) {
            addBtn.addEventListener('click', () => {
                const keyInput = document.getElementById('pageExtraFieldKey');
                const labelInput = document.getElementById('pageExtraFieldLabel');
                const typeSelectEl = document.getElementById('pageExtraFieldType');
                const optionsSelectEl = document.getElementById('pageExtraFieldOptions');
                
                const key = keyInput.value.trim();
                const label = labelInput.value.trim();
                const type = typeSelectEl.value;
                
                if (!key) {
                    App.showError(getText('common.pleaseEnterFieldKey', '請輸入欄位 Key'));
                    return;
                }
                
                // 檢查 key 是否已存在
                const exists = currentSettings.fields.some(f => f.key === key);
                if (exists) {
                    App.showError(getText('common.fieldKeyExists', '欄位 Key 已存在'));
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
                
                if (!currentSettings.extraFields) {
                    currentSettings.extraFields = [];
                }
                currentSettings.extraFields.push(newField);
                
                // 添加到 fields 設定
                const maxOrder = Math.max(...currentSettings.fields.map(f => f.order), -1);
                currentSettings.fields.push({
                    key: key,
                    visible: true,
                    order: maxOrder + 1
                });
                
                // 生成類型 badge
                let typeBadge = '';
                if (type === 'select') {
                    const optionCount = newField.options ? newField.options.length : 0;
                    typeBadge = `<span class="badge bg-secondary ms-1">${getText('common.fieldTypeSelect', '下拉選單')} (${optionCount})</span>`;
                }
                
                // 添加到列表 DOM
                const newItemHtml = `
                    <div class="field-settings-item" data-field-key="${key}" draggable="true">
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
                `;
                
                document.getElementById('pageFieldSettingsList').insertAdjacentHTML('beforeend', newItemHtml);
                
                // 清空輸入
                keyInput.value = '';
                labelInput.value = '';
                typeSelectEl.selectedIndex = 0;
                if (optionsSelectEl) {
                    $(optionsSelectEl).val(null).trigger('change');
                }
                document.getElementById('pageSelectOptionsRow').style.display = 'none';
                
                // 重新綁定事件
                bindModalEvents();
            });
        }
        
        // 輔助函數：更新類別列表顯示
        function updateCategoriesList() {
            const container = document.getElementById('categoriesList');
            if (!container) return;
            container.innerHTML = (currentSettings.categories || []).map(cat => 
                `<span class="badge bg-secondary me-1 mb-1 category-badge" data-category="${cat}">${cat} <i class="bi bi-x ms-1" style="cursor:pointer;"></i></span>`
            ).join('');
        }
        
        // 輔助函數：更新所有 Select2 的選項
        function updateAllCategorySelects() {
            if (typeof $ === 'undefined' || !$.fn.select2) return;
            $('.field-categories-select').each(function() {
                const $select = $(this);
                const fieldKey = $select.data('field-key');
                const sf = currentSettings.fields.find(f => f.key === fieldKey);
                const selectedCats = sf?.categories || [];
                
                // 重建選項
                $select.empty();
                (currentSettings.categories || []).forEach(cat => {
                    const option = new Option(cat, cat, selectedCats.includes(cat), selectedCats.includes(cat));
                    $select.append(option);
                });
                $select.trigger('change.select2');
            });
        }
        
        // 保存按鈕
        const saveBtn = document.getElementById('savePageFieldSettingsBtn');
        if (saveBtn) {
            saveBtn.addEventListener('click', async () => {
                // 更新順序
                const items = listContainer.querySelectorAll('.field-settings-item');
                items.forEach((item, idx) => {
                    const fieldKey = item.dataset.fieldKey;
                    const sf = currentSettings.fields.find(f => f.key === fieldKey);
                    if (sf) {
                        sf.order = idx;
                    }
                });
                
                // 保存前清理 readonly 欄位的預設值
                sanitizeSettingsBeforeSave();
                
                // 禁用按鈕
                saveBtn.disabled = true;
                saveBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span>保存中...';
                
                const success = await saveFieldSettingsToAPI(currentSettings);
                
                if (success) {
                    App.showSuccess(getText('common.saveSuccess', '保存成功'));
                    applyFieldSettings();
                    bootstrap.Modal.getInstance(document.getElementById('pageFieldSettingsModal')).hide();
                } else {
                    App.showError(getText('common.saveFailed', '保存失敗'));
                }
                
                saveBtn.disabled = false;
                saveBtn.innerHTML = `<i class="bi bi-check-lg me-1"></i>${getText('common.save', '保存')}`;
            });
        }
    }
    
    // 初始化函數
    async function init(options) {
        pageName = options.pageName;
        containerSelector = options.containerSelector || 'body';
        
        const insertAfterEl = document.querySelector(options.insertAfterSelector);
        if (!insertAfterEl) {
            console.warn('PageFieldSettings: insertAfterSelector not found:', options.insertAfterSelector);
            return;
        }
        
        const containerEl = document.querySelector(containerSelector);
        if (!containerEl) {
            console.warn('PageFieldSettings: containerSelector not found:', containerSelector);
            return;
        }
        
        // 收集欄位定義
        fieldDefinitions = collectFieldDefinitions(containerEl);
        
        // 載入或創建設定
        const savedSettings = await loadFieldSettingsFromAPI();
        if (savedSettings) {
            currentSettings = savedSettings;
            // 確保新欄位被加入
            fieldDefinitions.forEach(fd => {
                if (!currentSettings.fields.some(f => f.key === fd.key)) {
                    const maxOrder = Math.max(...currentSettings.fields.map(f => f.order), -1);
                    currentSettings.fields.push({
                        key: fd.key,
                        visible: true,
                        order: maxOrder + 1
                    });
                }
            });
        } else {
            currentSettings = initDefaultSettings(fieldDefinitions);
        }
        
        // 創建按鈕（需要權限）
        if (canConfigureFieldSettings()) {
            createSettingsButton(insertAfterEl);
        }
        
        // 應用設定
        applyFieldSettings();
    }
    
    return {
        init: init
    };
})();
