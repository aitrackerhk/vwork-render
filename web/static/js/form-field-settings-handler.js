/**
 * Form Field Settings Handler
 * 處理表單欄位設定功能：隱藏/顯示欄位、預設值應用、readonly 欄位限制
 * 
 * 從 dynamic-form.js 分離出來，使代碼更易維護
 */

window.FormFieldSettingsHandler = (function() {
    'use strict';

    /**
     * 翻譯輔助函數
     */
    function getText(key, fallback) {
        if (typeof I18n !== 'undefined' && I18n.t) {
            const t = I18n.t(key);
            return (t && t !== key) ? t : fallback;
        }
        return fallback;
    }

    /**
     * 從 fieldSettings 中應用欄位可見性和預設值到 DOM
     * @param {Object} dynamicFormInstance - DynamicForm 實例
     * @param {Object} [options] - 選項
     * @param {boolean} [options.skipDefaults=false] - 若為 true，只應用 visibility，不應用預設值
     */
    function applyFieldSettingsToDOM(dynamicFormInstance, options) {
        const skipDefaults = options && options.skipDefaults === true;
        console.log('[FormFieldSettingsHandler][DEBUG] applyFieldSettingsToDOM 被呼叫, skipDefaults:', skipDefaults);
        if (!dynamicFormInstance || !dynamicFormInstance.fieldSettings) {
            console.log('[FormFieldSettingsHandler][DEBUG] 無 fieldSettings，跳過');
            return;
        }

        const settings = dynamicFormInstance.fieldSettings;
        const isEditMode = dynamicFormInstance.isEdit;

        if (!settings.fields || !Array.isArray(settings.fields)) {
            return;
        }

        settings.fields.forEach(sf => {
            const fieldKey = sf.key;

            // 1. 應用可見性設定
            // 但永遠不隱藏 readonly 欄位（如 code, order_number 等系統自動產生的編號欄位）
            // 這些欄位由系統控制，不應該被用戶的欄位設定隱藏
            const fieldConfig = (dynamicFormInstance.config && dynamicFormInstance.config.formFields)
                ? dynamicFormInstance.config.formFields.find(f => f.key === fieldKey)
                : null;
            const isReadonly = fieldConfig && fieldConfig.readonly === true;

            if (isReadonly && sf.visible === false) {
                applyFieldVisibility(fieldKey, true);
            } else {
                applyFieldVisibility(fieldKey, sf.visible);
            }

            // 2. 應用預設值（僅在新建模式 + 未 skipDefaults 時）
            if (skipDefaults) return;

            let effectiveDefault = sf.defaultValue;
            if (effectiveDefault === undefined || effectiveDefault === null || effectiveDefault === '') {
                if (fieldConfig) {
                    effectiveDefault = fieldConfig.defaultValue !== undefined ? fieldConfig.defaultValue
                                     : fieldConfig.default !== undefined ? fieldConfig.default
                                     : undefined;
                }
            }
            if (!isEditMode && effectiveDefault !== undefined && effectiveDefault !== null && effectiveDefault !== '') {
                applyFieldDefaultValue(fieldKey, effectiveDefault, dynamicFormInstance);
            }
        });
    }

    /**
     * 應用欄位可見性
     * @param {string} fieldKey - 欄位 key
     * @param {boolean} visible - 是否可見
     */
    function applyFieldVisibility(fieldKey, visible) {
        // 嘗試找到欄位容器
        const possibleContainerIds = [
            `field_container_${fieldKey}`,
            `container_${fieldKey}`,
            `${fieldKey}_container`
        ];

        let container = null;
        for (const containerId of possibleContainerIds) {
            container = document.getElementById(containerId);
            if (container) break;
        }

        // 如果沒有找到容器，嘗試通過欄位元素找到其父容器
        if (!container) {
            const fieldElement = document.getElementById(`field_${fieldKey}`);
            if (fieldElement) {
                // 找到最近的 .mb-3 或 .col-md-* 或 .row 容器
                container = fieldElement.closest('.mb-3') || 
                           fieldElement.closest('.col-md-6') || 
                           fieldElement.closest('.col-md-12') ||
                           fieldElement.closest('.col-12');
            }
        }

        if (!container) {
            // console.warn(`[FormFieldSettingsHandler] 找不到欄位容器: ${fieldKey}`);
            return;
        }

        if (visible === false) {
            container.style.display = 'none';
            container.setAttribute('data-field-hidden', 'true');
            
            // 如果欄位是 required，暫時移除 required 屬性以避免提交失敗
            const fieldElement = container.querySelector(`#field_${fieldKey}`) || 
                                container.querySelector(`[name="${fieldKey}"]`);
            if (fieldElement && fieldElement.hasAttribute('required')) {
                fieldElement.removeAttribute('required');
                fieldElement.setAttribute('data-was-required', 'true');
            }
        } else {
            container.style.display = '';
            container.removeAttribute('data-field-hidden');
            
            // 恢復 required 屬性
            const fieldElement = container.querySelector(`#field_${fieldKey}`) || 
                                container.querySelector(`[name="${fieldKey}"]`);
            if (fieldElement && fieldElement.hasAttribute('data-was-required')) {
                fieldElement.setAttribute('required', '');
                fieldElement.removeAttribute('data-was-required');
            }
        }
    }

    /**
     * 應用欄位預設值
     * @param {string} fieldKey - 欄位 key
     * @param {*} defaultValue - 預設值
     * @param {Object} dynamicFormInstance - DynamicForm 實例
     */
    function applyFieldDefaultValue(fieldKey, defaultValue, dynamicFormInstance) {
        const fieldId = `field_${fieldKey}`;
        const fieldElement = document.getElementById(fieldId);

        if (!fieldElement) {
            // console.warn(`[FormFieldSettingsHandler] 找不到欄位元素: ${fieldId}`);
            return;
        }

        // 檢查欄位是否為 readonly - readonly 欄位不應用預設值
        if (isFieldReadonly(fieldElement, fieldKey, dynamicFormInstance)) {
            console.log(`[FormFieldSettingsHandler] 欄位 ${fieldKey} 是 readonly，跳過預設值設定`);
            return;
        }

        // 如果欄位已經有值，不覆蓋
        if (hasExistingValue(fieldElement)) {
            return;
        }

        const tagName = fieldElement.tagName.toUpperCase();
        const inputType = (fieldElement.type || '').toLowerCase();

        try {
            if (tagName === 'SELECT') {
                applySelectDefaultValue(fieldElement, defaultValue, fieldKey);
            } else if (tagName === 'INPUT') {
                applyInputDefaultValue(fieldElement, inputType, defaultValue);
            } else if (tagName === 'TEXTAREA') {
                fieldElement.value = String(defaultValue);
            }

            // 觸發 change 事件以確保其他綁定的邏輯能夠執行
            fieldElement.dispatchEvent(new Event('change', { bubbles: true }));

            console.log(`[FormFieldSettingsHandler] 已設定欄位 ${fieldKey} 的預設值: ${defaultValue}`);
        } catch (error) {
            console.error(`[FormFieldSettingsHandler] 設定欄位 ${fieldKey} 預設值失敗:`, error);
        }
    }

    /**
     * 檢查欄位是否為 readonly
     */
    function isFieldReadonly(fieldElement, fieldKey, dynamicFormInstance) {
        // 檢查 DOM 屬性
        if (fieldElement.hasAttribute('readonly') || fieldElement.hasAttribute('disabled')) {
            return true;
        }

        // 檢查 field 配置
        if (dynamicFormInstance && dynamicFormInstance.config && dynamicFormInstance.config.formFields) {
            const fieldConfig = dynamicFormInstance.config.formFields.find(f => f.key === fieldKey);
            if (fieldConfig && fieldConfig.readonly) {
                return true;
            }
        }

        return false;
    }

    /**
     * 檢查欄位是否已有值
     */
    function hasExistingValue(fieldElement) {
        const tagName = fieldElement.tagName.toUpperCase();

        if (tagName === 'SELECT') {
            // 對於 select，檢查是否有非空選中值
            if (fieldElement.multiple) {
                return fieldElement.selectedOptions.length > 0 && 
                       Array.from(fieldElement.selectedOptions).some(opt => opt.value !== '');
            }
            return fieldElement.value !== '' && fieldElement.value !== null;
        }

        if (fieldElement.type === 'checkbox' || fieldElement.type === 'radio') {
            // checkbox/radio 總是可以設定預設值
            return false;
        }

        return fieldElement.value !== '' && fieldElement.value !== null;
    }

    /**
     * 應用 SELECT 元素的預設值
     */
    function applySelectDefaultValue(selectElement, defaultValue, fieldKey) {
        const isMultiple = selectElement.multiple;
        const values = isMultiple 
            ? String(defaultValue).split(',').map(v => v.trim()).filter(Boolean)
            : [String(defaultValue)];

        // 檢查是否使用 Select2
        const isSelect2 = typeof $ !== 'undefined' && $(selectElement).hasClass('select2-hidden-accessible');

        if (isMultiple) {
            // 多選
            Array.from(selectElement.options).forEach(opt => {
                opt.selected = values.includes(opt.value);
            });
        } else {
            // 單選
            const targetValue = values[0];
            const optionExists = Array.from(selectElement.options).some(opt => opt.value === targetValue);
            if (optionExists) {
                selectElement.value = targetValue;
            } else {
                console.warn(`[FormFieldSettingsHandler] 欄位 ${fieldKey} 的預設值 "${targetValue}" 不在選項中`);
            }
        }

        // 如果是 Select2，觸發更新
        if (isSelect2) {
            $(selectElement).trigger('change.select2');
        }
    }

    /**
     * 應用 INPUT 元素的預設值
     */
    function applyInputDefaultValue(inputElement, inputType, defaultValue) {
        switch (inputType) {
            case 'checkbox':
                inputElement.checked = defaultValue === true || 
                                       defaultValue === 'true' || 
                                       defaultValue === '1';
                break;
            case 'radio':
                inputElement.checked = inputElement.value === String(defaultValue);
                break;
            case 'number':
                inputElement.value = defaultValue;
                break;
            case 'date':
            case 'datetime-local':
            case 'time':
                inputElement.value = String(defaultValue);
                break;
            default:
                inputElement.value = String(defaultValue);
        }
    }

    /**
     * 在欄位設定 Modal 中隱藏 readonly 欄位的預設值區域
     * 注意：現在 readonly 欄位的預設值區域在生成 HTML 時就不會被建立，
     * 此函數作為備用機制，確保任何遺漏的 readonly 欄位預設值區域被隱藏
     * @param {Object} dynamicFormInstance - DynamicForm 實例
     */
    function disableDefaultValueForReadonlyFields(dynamicFormInstance) {
        if (!dynamicFormInstance || !dynamicFormInstance.config || !dynamicFormInstance.config.formFields) {
            return;
        }

        const readonlyFieldKeys = dynamicFormInstance.config.formFields
            .filter(f => f.readonly === true)
            .map(f => f.key);

        if (readonlyFieldKeys.length === 0) {
            return;
        }

        const listContainer = document.getElementById('fieldSettingsList') || 
                             document.getElementById('pageFieldSettingsList');
        if (!listContainer) {
            return;
        }

        readonlyFieldKeys.forEach(fieldKey => {
            // 找到對應的欄位設定項目
            const wrapper = listContainer.querySelector(`[data-field-key="${fieldKey}"]`);
            if (!wrapper) return;

            // 標記為 readonly
            wrapper.setAttribute('data-readonly', 'true');

            // 找到預設值輸入區域，如果存在則完全隱藏
            const defaultValueBox = wrapper.querySelector('.field-default-value-box') ||
                                   wrapper.querySelector('.field-default-value');
            if (defaultValueBox) {
                // 完全隱藏預設值區域（不只是 d-none）
                defaultValueBox.style.display = 'none';
                defaultValueBox.classList.add('d-none');
                defaultValueBox.classList.remove('show');
            }

            // 確保欄位設定項目有 Readonly 標記
            const fieldInfo = wrapper.querySelector('.field-info');
            if (fieldInfo && !fieldInfo.querySelector('.badge.bg-warning')) {
                const badge = document.createElement('span');
                badge.className = 'badge bg-warning ms-2';
                badge.textContent = 'Readonly';
                fieldInfo.appendChild(badge);
            }
        });

        console.log(`[FormFieldSettingsHandler] 已隱藏 ${readonlyFieldKeys.length} 個 readonly 欄位的預設值區域`);
    }

    /**
     * 增強版保存前驗證：移除 readonly 欄位的預設值
     * @param {Object} fieldSettings - 欄位設定對象
     * @param {Object} dynamicFormInstance - DynamicForm 實例
     */
    function sanitizeFieldSettingsBeforeSave(fieldSettings, dynamicFormInstance) {
        if (!fieldSettings || !fieldSettings.fields || !dynamicFormInstance) {
            return fieldSettings;
        }

        const readonlyFieldKeys = new Set(
            (dynamicFormInstance.config?.formFields || [])
                .filter(f => f.readonly === true)
                .map(f => f.key)
        );

        fieldSettings.fields.forEach(sf => {
            if (readonlyFieldKeys.has(sf.key) && sf.defaultValue !== undefined) {
                console.log(`[FormFieldSettingsHandler] 移除 readonly 欄位 ${sf.key} 的預設值`);
                delete sf.defaultValue;
            }
        });

        return fieldSettings;
    }

    /**
     * 延遲應用欄位設定（等待 DOM 和 Select2 初始化完成）
     * @param {Object} dynamicFormInstance - DynamicForm 實例
     * @param {number} delay - 延遲時間（毫秒）
     */
    function applyFieldSettingsWithDelay(dynamicFormInstance, delay = 500) {
        setTimeout(() => {
            applyFieldSettingsToDOM(dynamicFormInstance);
        }, delay);
    }

    /**
     * 監聽 Select2 初始化完成後應用預設值
     * @param {Object} dynamicFormInstance - DynamicForm 實例
     */
    function applyDefaultValuesAfterSelect2Init(dynamicFormInstance) {
        if (!dynamicFormInstance || !dynamicFormInstance.fieldSettings) {
            return;
        }

        const settings = dynamicFormInstance.fieldSettings;
        const isEditMode = dynamicFormInstance.isEdit;

        if (isEditMode || !settings.fields) {
            return;
        }

        // 找出所有需要設定預設值的 Select2 欄位
        settings.fields.forEach(sf => {
            // 優先使用 API 設定中的 defaultValue，如果沒有則 fallback 到 page-configs.js 的靜態預設值
            let effectiveDefault = sf.defaultValue;
            if (effectiveDefault === undefined || effectiveDefault === null || effectiveDefault === '') {
                const fieldConfig = (dynamicFormInstance.config && dynamicFormInstance.config.formFields)
                    ? dynamicFormInstance.config.formFields.find(f => f.key === sf.key)
                    : null;
                if (fieldConfig) {
                    effectiveDefault = fieldConfig.defaultValue !== undefined ? fieldConfig.defaultValue
                                     : fieldConfig.default !== undefined ? fieldConfig.default
                                     : undefined;
                }
            }

            if (effectiveDefault === undefined || effectiveDefault === null || effectiveDefault === '') {
                return;
            }

            const fieldId = `field_${sf.key}`;
            const fieldElement = document.getElementById(fieldId);
            if (!fieldElement || fieldElement.tagName !== 'SELECT') {
                return;
            }

            // 檢查是否為 Select2
            if (typeof $ === 'undefined') return;

            const $el = $(fieldElement);
            if (!$el.hasClass('select2-hidden-accessible')) {
                return;
            }

            // 檢查 readonly
            if (isFieldReadonly(fieldElement, sf.key, dynamicFormInstance)) {
                return;
            }

            // 如果已有值，跳過
            if (hasExistingValue(fieldElement)) {
                return;
            }

            // 設定 Select2 的值
            try {
                const isMultiple = fieldElement.multiple;
                const values = isMultiple 
                    ? String(effectiveDefault).split(',').map(v => v.trim()).filter(Boolean)
                    : String(effectiveDefault);

                $el.val(values).trigger('change');
                console.log(`[FormFieldSettingsHandler] Select2 欄位 ${sf.key} 預設值已設定: ${effectiveDefault}`);
            } catch (error) {
                console.error(`[FormFieldSettingsHandler] 設定 Select2 欄位 ${sf.key} 預設值失敗:`, error);
            }
        });
    }

    // 公開 API
    return {
        applyFieldSettingsToDOM: applyFieldSettingsToDOM,
        applyFieldVisibility: applyFieldVisibility,
        applyFieldDefaultValue: applyFieldDefaultValue,
        isFieldReadonly: isFieldReadonly,
        disableDefaultValueForReadonlyFields: disableDefaultValueForReadonlyFields,
        sanitizeFieldSettingsBeforeSave: sanitizeFieldSettingsBeforeSave,
        applyFieldSettingsWithDelay: applyFieldSettingsWithDelay,
        applyDefaultValuesAfterSelect2Init: applyDefaultValuesAfterSelect2Init
    };
})();
