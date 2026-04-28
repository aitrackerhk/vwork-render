// vAI Docs — AI Document Generation (docx, xlsx, pptx)
// Uses App.apiRequest() for authenticated API calls

var VaiDocs = (function() {
    'use strict';

    // ─── State ────────────────────────────────────────────────────
    var documents = [];
    var chatDocuments = [];
    var currentDocId = null;
    var isGenerating = false;
    var sidebarOpen = false;
    var sidebarTab = 'docs'; // 'docs' or 'chat'

    // ─── Initialization ───────────────────────────────────────────
    function init() {
        loadDocuments();
        initDocTypeSelector();
    }

    function initDocTypeSelector() {
        document.querySelectorAll('.vai-doc-type-option').forEach(function(opt) {
            opt.addEventListener('click', function() {
                document.querySelectorAll('.vai-doc-type-option').forEach(function(o) {
                    o.classList.remove('active');
                });
                opt.classList.add('active');
                opt.querySelector('input[type="radio"]').checked = true;
            });
        });
    }

    // ─── API Calls ────────────────────────────────────────────────

    function loadDocuments() {
        // Load user-created documents (source=docs)
        App.apiRequest('/ai/documents?source=docs').then(function(resp) {
            documents = resp.data || resp || [];
            renderDocList();
            updateEmptyState();
        }).catch(function(err) {
            console.error('Failed to load documents:', err);
            documents = [];
            renderDocList();
            updateEmptyState();
        });
    }

    function loadChatDocuments() {
        var container = document.getElementById('vaiChatDocsList');
        if (container) {
            container.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><span class="spinner-border spinner-border-sm me-1"></span><span data-i18n="vai.common.loading">' + I18n.t('vai.common.loading') + '</span></div>';
        }
        App.apiRequest('/ai/documents?source=chat').then(function(resp) {
            chatDocuments = resp.data || resp || [];
            renderChatDocList();
        }).catch(function(err) {
            console.error('Failed to load chat documents:', err);
            chatDocuments = [];
            renderChatDocList();
        });
    }

    function generate() {
        if (isGenerating) return;

        var prompt = document.getElementById('vaiDocPrompt').value.trim();
        if (!prompt) {
            showAlert('warning', I18n.t('vai.docs.enterDescription'));
            return;
        }

        var title = document.getElementById('vaiDocTitle').value.trim();
        var docType = document.querySelector('input[name="docType"]:checked').value;

        isGenerating = true;
        var btn = document.getElementById('vaiDocGenBtn');
        var originalHtml = btn.innerHTML;
        btn.disabled = true;
        btn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.docs.generatingBtn');

        App.apiRequest('/ai/doc-generate', {
            method: 'POST',
            body: JSON.stringify({
                prompt: prompt,
                doc_type: docType,
                title: title
            })
        }).then(function(resp) {
            isGenerating = false;
            btn.disabled = false;
            btn.innerHTML = originalHtml;

            // Add to local list and show detail
            documents.unshift(resp);
            renderDocList();
            showDocDetail(resp.id || resp.ID);
            hideNewDocForm();

            // Clear form
            document.getElementById('vaiDocPrompt').value = '';
            document.getElementById('vaiDocTitle').value = '';

            showAlert('success', I18n.t('vai.docs.generateSuccess'));
        }).catch(function(err) {
            isGenerating = false;
            btn.disabled = false;
            btn.innerHTML = originalHtml;

            var errorMsg = I18n.t('vai.docs.generateFailed');
            if (err && err.error) {
                errorMsg = err.error;
            } else if (err && err.message) {
                errorMsg = err.message;
            }
            showAlert('danger', errorMsg);

            // Refresh list in case a partial record was created
            loadDocuments();
        });
    }

    function deleteCurrent() {
        if (!currentDocId) return;
        deleteDocById(currentDocId);
    }

    function deleteDocById(id) {
        if (!confirm(I18n.t('vai.docs.deleteConfirm'))) return;

        App.apiRequest('/ai/documents/' + id, {
            method: 'DELETE'
        }).then(function() {
            documents = documents.filter(function(d) { return (d.id || d.ID) !== id; });
            chatDocuments = chatDocuments.filter(function(d) { return (d.id || d.ID) !== id; });
            if (currentDocId === id) {
                currentDocId = null;
                showEmptyState();
            }
            renderDocList();
            renderChatDocList();
            showAlert('success', I18n.t('vai.docs.deleted'));
        }).catch(function(err) {
            console.error('Failed to delete document:', err);
            showAlert('danger', I18n.t('vai.common.deleteFailed'));
        });
    }

    // ─── Rendering ────────────────────────────────────────────────

    function renderDocList() {
        var container = document.getElementById('vaiDocsList');
        if (!container) return;

        if (documents.length === 0) {
            container.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><span data-i18n="vai.docs.noDocuments">' + I18n.t('vai.docs.noDocuments') + '</span></div>';
            return;
        }

        container.innerHTML = documents.map(function(d) {
            var id = d.id || d.ID;
            var isActive = id === currentDocId;
            var icon = getDocTypeIcon(d.doc_type);
            var statusBadge = getStatusBadgeHtml(d.status);
            var timeStr = formatTime(d.created_at);

            return '<div class="vai-docs-list-item ' + (isActive ? 'active' : '') + '" onclick="VaiDocs.showDocDetail(\'' + id + '\')" data-doc-id="' + id + '">' +
                '<div class="d-flex align-items-center gap-2">' +
                    '<i class="' + icon.class + ' ' + icon.color + '" style="font-size: 1.3rem;"></i>' +
                    '<div class="flex-grow-1 min-width-0">' +
                        '<div class="doc-title text-truncate">' + escapeHtml(d.title || I18n.t('vai.common.unnamed')) + '</div>' +
                        '<div class="doc-meta">' + statusBadge + ' <span class="text-muted">' + timeStr + '</span></div>' +
                    '</div>' +
                    '<button class="btn btn-sm btn-link p-0 vai-history-delete" ' +
                        'onclick="event.stopPropagation(); VaiDocs.deleteDocById(\'' + id + '\')" ' +
                        'title="' + I18n.t('vai.common.delete') + '"><i class="bi bi-trash3" style="font-size: 0.75rem;"></i></button>' +
                '</div>' +
            '</div>';
        }).join('');
    }

    function renderChatDocList() {
        var container = document.getElementById('vaiChatDocsList');
        if (!container) return;

        if (chatDocuments.length === 0) {
            container.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' +
                '<i class="bi bi-chat-dots d-block mb-2" style="font-size: 1.5rem;"></i>' +
                '<span data-i18n="vai.docs.noChatDocs">' + I18n.t('vai.docs.noChatDocs') + '</span><br><small data-i18n="vai.docs.noChatDocsHint">' + I18n.t('vai.docs.noChatDocsHint') + '</small></div>';
            return;
        }

        container.innerHTML = chatDocuments.map(function(d) {
            var id = d.id || d.ID;
            var isActive = id === currentDocId;
            var icon = getDocTypeIcon(d.doc_type);
            var statusBadge = getStatusBadgeHtml(d.status);
            var timeStr = formatTime(d.created_at);

            return '<div class="vai-docs-list-item ' + (isActive ? 'active' : '') + '" onclick="VaiDocs.showDocDetail(\'' + id + '\', true)" data-doc-id="' + id + '">' +
                '<div class="d-flex align-items-center gap-2">' +
                    '<i class="' + icon.class + ' ' + icon.color + '" style="font-size: 1.3rem;"></i>' +
                    '<div class="flex-grow-1 min-width-0">' +
                        '<div class="doc-title text-truncate">' + escapeHtml(d.title || I18n.t('vai.common.unnamed')) + '</div>' +
                        '<div class="doc-meta">' + statusBadge + ' <span class="text-muted">' + timeStr + '</span></div>' +
                    '</div>' +
                    '<button class="btn btn-sm btn-link p-0 vai-history-delete" ' +
                        'onclick="event.stopPropagation(); VaiDocs.deleteDocById(\'' + id + '\')" ' +
                        'title="' + I18n.t('vai.common.delete') + '"><i class="bi bi-trash3" style="font-size: 0.75rem;"></i></button>' +
                '</div>' +
            '</div>';
        }).join('');
    }

    // ─── Sidebar Tab Switching ────────────────────────────────────
    function switchSidebarTab(tab) {
        sidebarTab = tab;
        var docList = document.getElementById('vaiDocsList');
        var chatList = document.getElementById('vaiChatDocsList');
        var tabs = document.querySelectorAll('#vaiDocsSidebar .vai-sidebar-tab');
        tabs.forEach(function(t) {
            t.classList.toggle('active', t.getAttribute('data-tab') === tab);
        });
        if (tab === 'docs') {
            if (docList) docList.style.display = '';
            if (chatList) chatList.style.display = 'none';
        } else {
            if (docList) docList.style.display = 'none';
            if (chatList) chatList.style.display = '';
            loadChatDocuments();
        }
    }

    function showDocDetail(id, fromChat) {
        var doc = documents.find(function(d) { return (d.id || d.ID) === id; });
        if (!doc) {
            doc = chatDocuments.find(function(d) { return (d.id || d.ID) === id; });
        }
        if (!doc) return;

        currentDocId = id;

        // Highlight in sidebar
        document.querySelectorAll('.vai-docs-list-item').forEach(function(el) {
            el.classList.toggle('active', el.getAttribute('data-doc-id') === id);
        });

        // Hide other views
        hideEl('vaiDocsEmpty');
        hideEl('vaiDocsNoSelection');
        hideEl('vaiDocsForm');
        showEl('vaiDocsDetail');

        // Populate detail
        document.getElementById('vaiDocDetailTitle').textContent = doc.title || I18n.t('vai.common.unnamed');
        document.getElementById('vaiDocDetailPrompt').textContent = doc.prompt || '';
        document.getElementById('vaiDocDetailDate').textContent = formatTime(doc.created_at);

        // Type badge
        var typeBadge = document.getElementById('vaiDocDetailTypeBadge');
        var typeInfo = getDocTypeInfo(doc.doc_type);
        typeBadge.className = 'badge ' + typeInfo.badgeClass;
        typeBadge.textContent = typeInfo.label;

        // Status badge
        var statusBadge = document.getElementById('vaiDocDetailStatusBadge');
        var statusInfo = getStatusInfo(doc.status);
        statusBadge.className = 'badge ' + statusInfo.badgeClass;
        statusBadge.textContent = statusInfo.label;

        // Show appropriate area based on status
        hideEl('vaiDocDetailDownload');
        hideEl('vaiDocDetailError');
        hideEl('vaiDocDetailGenerating');

        if (doc.status === 'completed') {
            showEl('vaiDocDetailDownload');
            var downloadBtn = document.getElementById('vaiDocDetailDownloadBtn');
            downloadBtn.href = doc.file_url || '/api/v1/ai/documents/' + id + '/download';
            var fileSizeEl = document.getElementById('vaiDocDetailFileSize');
            fileSizeEl.textContent = formatFileSize(doc.file_size || 0);
        } else if (doc.status === 'failed') {
            showEl('vaiDocDetailError');
            document.getElementById('vaiDocDetailErrorMsg').textContent = doc.error_message || I18n.t('vai.docs.generationFailed');
        } else if (doc.status === 'generating' || doc.status === 'pending') {
            showEl('vaiDocDetailGenerating');
        }

        // Close mobile sidebar
        if (window.innerWidth < 768) {
            closeSidebar();
        }
    }

    // ─── UI State Management ──────────────────────────────────────

    function showNewDocForm() {
        hideEl('vaiDocsEmpty');
        hideEl('vaiDocsNoSelection');
        hideEl('vaiDocsDetail');
        showEl('vaiDocsForm');
        currentDocId = null;
        // Deselect sidebar items
        document.querySelectorAll('.vai-docs-list-item').forEach(function(el) {
            el.classList.remove('active');
        });
        // Focus prompt
        setTimeout(function() {
            var prompt = document.getElementById('vaiDocPrompt');
            if (prompt) prompt.focus();
        }, 100);
    }

    function hideNewDocForm() {
        hideEl('vaiDocsForm');
        updateEmptyState();
    }

    function showEmptyState() {
        hideEl('vaiDocsForm');
        hideEl('vaiDocsDetail');
        if (documents.length === 0) {
            showEl('vaiDocsEmpty');
            hideEl('vaiDocsNoSelection');
        } else {
            hideEl('vaiDocsEmpty');
            showEl('vaiDocsNoSelection');
        }
    }

    function updateEmptyState() {
        var formVisible = document.getElementById('vaiDocsForm').style.display !== 'none';
        var detailVisible = document.getElementById('vaiDocsDetail').style.display !== 'none';
        if (!currentDocId && !formVisible && !detailVisible) {
            if (documents.length === 0) {
                showEl('vaiDocsEmpty');
                hideEl('vaiDocsNoSelection');
            } else {
                hideEl('vaiDocsEmpty');
                showEl('vaiDocsNoSelection');
            }
        } else {
            hideEl('vaiDocsEmpty');
            hideEl('vaiDocsNoSelection');
        }
    }

    function toggleSidebar() {
        var sidebar = document.getElementById('vaiDocsSidebar');
        if (!sidebar) return;
        sidebarOpen = !sidebarOpen;
        sidebar.classList.toggle('show', sidebarOpen);
    }

    function closeSidebar() {
        var sidebar = document.getElementById('vaiDocsSidebar');
        if (sidebar) sidebar.classList.remove('show');
        sidebarOpen = false;
    }

    // ─── Helpers ──────────────────────────────────────────────────

    function getDocTypeIcon(docType) {
        switch (docType) {
            case 'docx': return { class: 'bi bi-file-earmark-word', color: 'text-primary' };
            case 'xlsx': return { class: 'bi bi-file-earmark-excel', color: 'text-success' };
            case 'pptx': return { class: 'bi bi-file-earmark-ppt', color: 'text-danger' };
            case 'pdf': return { class: 'bi bi-file-earmark-pdf', color: 'text-danger' };
            default: return { class: 'bi bi-file-earmark', color: 'text-muted' };
        }
    }

    function getDocTypeInfo(docType) {
        switch (docType) {
            case 'docx': return { label: 'Word', badgeClass: 'bg-primary' };
            case 'xlsx': return { label: 'Excel', badgeClass: 'bg-success' };
            case 'pptx': return { label: 'PowerPoint', badgeClass: 'bg-danger' };
            case 'pdf': return { label: 'PDF', badgeClass: 'bg-danger' };
            default: return { label: docType, badgeClass: 'bg-secondary' };
        }
    }

    function getStatusInfo(status) {
        switch (status) {
            case 'completed': return { label: I18n.t('vai.docs.statusCompleted'), badgeClass: 'bg-success' };
            case 'generating': return { label: I18n.t('vai.docs.statusGenerating'), badgeClass: 'bg-warning text-dark' };
            case 'pending': return { label: I18n.t('vai.docs.statusPending'), badgeClass: 'bg-secondary' };
            case 'failed': return { label: I18n.t('vai.docs.statusFailed'), badgeClass: 'bg-danger' };
            default: return { label: status, badgeClass: 'bg-secondary' };
        }
    }

    function getStatusBadgeHtml(status) {
        var info = getStatusInfo(status);
        return '<span class="badge ' + info.badgeClass + '" style="font-size: 0.65rem;">' + info.label + '</span>';
    }

    function formatTime(dateStr) {
        if (!dateStr) return '';
        var d = new Date(dateStr);
        var now = new Date();
        var diffMs = now - d;
        var diffMin = Math.floor(diffMs / 60000);
        if (diffMin < 1) return I18n.t('vai.common.justNow');
        if (diffMin < 60) return diffMin + I18n.t('vai.common.minutesAgo');
        var diffHr = Math.floor(diffMin / 60);
        if (diffHr < 24) return diffHr + I18n.t('vai.common.hoursAgo');
        return d.getFullYear() + '/' + (d.getMonth() + 1) + '/' + d.getDate();
    }

    function formatFileSize(bytes) {
        if (!bytes || bytes === 0) return '';
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / 1048576).toFixed(1) + ' MB';
    }

    function escapeHtml(text) {
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(text));
        return div.innerHTML;
    }

    function showAlert(type, msg) {
        var container = document.getElementById('alertContainer');
        if (!container) return;
        var id = 'alert-' + Date.now();
        container.innerHTML = '<div id="' + id + '" class="alert alert-' + type + ' alert-dismissible fade show" role="alert">' +
            msg + '<button type="button" class="btn-close" data-bs-dismiss="alert"></button></div>';
        setTimeout(function() {
            var el = document.getElementById(id);
            if (el) el.remove();
        }, 5000);
    }

    function showEl(id) {
        var el = document.getElementById(id);
        if (el) el.style.display = '';
    }

    function hideEl(id) {
        var el = document.getElementById(id);
        if (el) el.style.display = 'none';
    }

    // ─── Public API ───────────────────────────────────────────────
    return {
        init: init,
        generate: generate,
        showNewDocForm: showNewDocForm,
        hideNewDocForm: hideNewDocForm,
        showDocDetail: showDocDetail,
        deleteCurrent: deleteCurrent,
        deleteDocById: deleteDocById,
        toggleSidebar: toggleSidebar,
        switchSidebarTab: switchSidebarTab
    };

})();
