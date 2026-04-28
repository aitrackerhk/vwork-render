// 草稿管理器
class DraftManager {
    constructor() {
        this.storageKey = 'u-nai_drafts';
        this.defaultRetentionDays = 7;
        this.retentionDays = this.loadRetentionDays();
        this.apiPath = '/drafts';
        this.isSyncing = false;
    }

    // 從用戶設置載入保留天數
    loadRetentionDays() {
        try {
            const settings = localStorage.getItem('u-nai_user_settings');
            if (settings) {
                const parsed = JSON.parse(settings);
                return parsed.draft_retention_days || this.defaultRetentionDays;
            }
        } catch (e) {
            console.error('Failed to load retention days:', e);
        }
        return this.defaultRetentionDays;
    }

    // 設置保留天數
    setRetentionDays(days) {
        this.retentionDays = days;
        try {
            let settings = {};
            const existing = localStorage.getItem('nwork_user_settings');
            if (existing) {
                settings = JSON.parse(existing);
            }
            settings.draft_retention_days = days;
            localStorage.setItem('nwork_user_settings', JSON.stringify(settings));
        } catch (e) {
            console.error('Failed to save retention days:', e);
        }
    }

    // 獲取所有草稿
    getAllDrafts() {
        try {
            const drafts = JSON.parse(localStorage.getItem(this.storageKey) || '{}');
            this.cleanExpiredDrafts(drafts);
            return drafts;
        } catch (e) {
            console.error('Failed to load drafts:', e);
            return {};
        }
    }

    // 從伺服器同步草稿（非阻塞）
    async syncFromServer(pageName = null) {
        if (this.isSyncing) return;
        if (typeof App === 'undefined' || !App.apiRequest) return;

        this.isSyncing = true;
        try {
            const url = pageName
                ? `${this.apiPath}?page_name=${encodeURIComponent(pageName)}`
                : this.apiPath;
            const res = await App.apiRequest(url);
            if (!res || !Array.isArray(res.drafts)) return;

            const draftsByPage = {};
            for (const d of res.drafts) {
                const page = d.page_name || d.pageName;
                if (!page) continue;
                if (!draftsByPage[page]) draftsByPage[page] = [];
                draftsByPage[page].push(this.normalizeDraftFromServer(d));
            }

            const allDrafts = this.getAllDrafts();
            if (pageName) {
                allDrafts[pageName] = draftsByPage[pageName] || [];
            } else {
                Object.keys(allDrafts).forEach(k => delete allDrafts[k]);
                Object.assign(allDrafts, draftsByPage);
            }

            localStorage.setItem(this.storageKey, JSON.stringify(allDrafts));
        } catch (e) {
            console.warn('Failed to sync drafts from server:', e);
        } finally {
            this.isSyncing = false;
        }
    }

    normalizeDraftFromServer(draft) {
        return {
            id: draft.id,
            data: draft.data || {},
            keyField: draft.key_field_value || draft.keyField || null,
            createdAt: draft.created_at || draft.createdAt || new Date().toISOString(),
            updatedAt: draft.updated_at || draft.updatedAt || new Date().toISOString()
        };
    }

    // 獲取特定頁面的草稿列表
    getDraftsForPage(pageName) {
        const allDrafts = this.getAllDrafts();
        const pageDrafts = allDrafts[pageName] || [];
        // 按時間排序，最新的在前
        return pageDrafts.sort((a, b) => new Date(b.createdAt) - new Date(a.createdAt));
    }

    // 獲取特定草稿
    getDraft(pageName, draftId) {
        const drafts = this.getDraftsForPage(pageName);
        return drafts.find(d => d.id === draftId);
    }

    // 保存草稿
    saveDraft(pageName, formData, keyField = null, draftId = null) {
        try {
            const allDrafts = this.getAllDrafts();
            if (!allDrafts[pageName]) {
                allDrafts[pageName] = [];
            }

            let existingIndex = -1;
            const hasDraftId = !!draftId;
            
            // 優先使用傳入的 draftId 來查找現有草稿
            if (hasDraftId) {
                existingIndex = allDrafts[pageName].findIndex(d => d.id === draftId);
            }
            
            // 如果沒有找到，檢查是否已存在相同 keyField 的草稿
            if (existingIndex === -1) {
                const keyFieldValue = keyField && formData[keyField] ? formData[keyField] : null;
                if (keyFieldValue) {
                    existingIndex = allDrafts[pageName].findIndex(d => {
                        // 檢查 keyField 值是否匹配
                        if (d.keyField === keyFieldValue) return true;
                        // 也檢查 data 中的值（向後兼容）
                        if (d.data && d.data[keyField] === keyFieldValue) return true;
                        return false;
                    });
                }
            }

            const keyFieldValue = keyField && formData[keyField] ? formData[keyField] : null;
            const existingId = existingIndex >= 0 ? allDrafts[pageName][existingIndex].id : null;
            const normalizedId = existingId && this.isUUID(existingId) ? existingId : null;
            const resolvedDraftId = normalizedId || (hasDraftId && this.isUUID(draftId) ? draftId : this.generateDraftId());
            const draft = {
                id: existingIndex >= 0 ? resolvedDraftId : (hasDraftId && this.isUUID(draftId) ? draftId : this.generateDraftId()),
                data: formData,
                keyField: keyFieldValue, // 存儲 keyField 的值，而不是字段名
                createdAt: existingIndex >= 0 ? allDrafts[pageName][existingIndex].createdAt : new Date().toISOString(),
                updatedAt: new Date().toISOString()
            };

            if (existingIndex >= 0) {
                // 更新現有草稿
                allDrafts[pageName][existingIndex] = draft;
            } else {
                // 添加新草稿
                allDrafts[pageName].push(draft);
            }

            localStorage.setItem(this.storageKey, JSON.stringify(allDrafts));

            // 非阻塞同步到伺服器
            this.saveDraftToServer(draft, pageName, keyField);
            return draft.id;
        } catch (e) {
            console.error('Failed to save draft:', e);
            return null;
        }
    }

    // 刪除草稿
    deleteDraft(pageName, draftId) {
        try {
            const allDrafts = this.getAllDrafts();
            if (allDrafts[pageName]) {
                allDrafts[pageName] = allDrafts[pageName].filter(d => d.id !== draftId);
                if (allDrafts[pageName].length === 0) {
                    delete allDrafts[pageName];
                }
                localStorage.setItem(this.storageKey, JSON.stringify(allDrafts));
            }

            // 同步刪除伺服器草稿
            this.deleteDraftFromServer(draftId);
        } catch (e) {
            console.error('Failed to delete draft:', e);
        }
    }

    // 清理過期草稿
    cleanExpiredDrafts(drafts = null) {
        if (!drafts) {
            drafts = this.getAllDrafts();
        }
        
        const now = new Date();
        const retentionMs = this.retentionDays * 24 * 60 * 60 * 1000;
        let hasChanges = false;
        const removedIds = [];

        for (const pageName in drafts) {
            drafts[pageName] = drafts[pageName].filter(draft => {
                const draftDate = new Date(draft.createdAt);
                const isExpired = (now - draftDate) > retentionMs;
                if (isExpired) {
                    hasChanges = true;
                    if (draft.id) removedIds.push(draft.id);
                }
                return !isExpired;
            });

            if (drafts[pageName].length === 0) {
                delete drafts[pageName];
                hasChanges = true;
            }
        }

        if (hasChanges) {
            localStorage.setItem(this.storageKey, JSON.stringify(drafts));
        }

        if (removedIds.length > 0) {
            removedIds.forEach(id => this.deleteDraftFromServer(id));
        }
    }

    // 生成草稿 ID
    generateDraftId() {
        if (typeof crypto !== 'undefined') {
            if (crypto.randomUUID) return crypto.randomUUID();
            if (crypto.getRandomValues) {
                const buf = new Uint8Array(16);
                crypto.getRandomValues(buf);
                // RFC4122 v4
                buf[6] = (buf[6] & 0x0f) | 0x40;
                buf[8] = (buf[8] & 0x3f) | 0x80;
                const hex = Array.from(buf).map(b => b.toString(16).padStart(2, '0'));
                return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10, 16).join('')}`;
            }
        }
        return 'draft_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
    }

    isUUID(value) {
        return typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
    }

    // 獲取草稿數量
    getDraftCount(pageName) {
        return this.getDraftsForPage(pageName).length;
    }

    // 獲取所有頁面的草稿總數
    getTotalDraftCount() {
        const allDrafts = this.getAllDrafts();
        let total = 0;
        for (const pageName in allDrafts) {
            total += allDrafts[pageName].length;
        }
        return total;
    }

    async saveDraftToServer(draft, pageName, keyField) {
        if (typeof App === 'undefined' || !App.apiRequest) return;
        try {
            const payload = {
                draft_id: draft.id,
                page_name: pageName,
                key_field: keyField,
                key_field_value: draft.keyField || null,
                data: draft.data || {}
            };
            const res = await App.apiRequest(this.apiPath, {
                method: 'POST',
                body: JSON.stringify(payload)
            });

            if (res && res.id && res.id !== draft.id) {
                this.replaceDraftId(pageName, draft.id, res.id);
            }
        } catch (e) {
            console.warn('Failed to save draft to server:', e);
        }
    }

    async deleteDraftFromServer(draftId) {
        if (!draftId) return;
        if (!this.isUUID(draftId)) return;
        if (typeof App === 'undefined' || !App.apiRequest) return;
        try {
            await App.apiRequest(`${this.apiPath}/${encodeURIComponent(draftId)}`, {
                method: 'DELETE'
            });
        } catch (e) {
            console.warn('Failed to delete draft from server:', e);
        }
    }

    replaceDraftId(pageName, oldId, newId) {
        if (!oldId || !newId || oldId === newId) return;
        const allDrafts = this.getAllDrafts();
        if (!allDrafts[pageName]) return;
        const idx = allDrafts[pageName].findIndex(d => d.id === oldId);
        if (idx === -1) return;
        allDrafts[pageName][idx].id = newId;
        localStorage.setItem(this.storageKey, JSON.stringify(allDrafts));
    }
}

// 全局實例
const draftManager = new DraftManager();

// 頁面載入時清理過期草稿
if (typeof window !== 'undefined') {
    window.addEventListener('load', () => {
        draftManager.cleanExpiredDrafts();
        draftManager.syncFromServer();
    });
}

