// vAI Video Generation Tool — v5.0
// Multi-shot video generation using AI video API (native audio, no TTS/lip-sync/BGM)
// Features: project management, shot management, image-to-video, prompt-based generation, multi-shot, combine

var VaiVideo = (function() {
    'use strict';

    // ─── State ────────────────────────────────────────────────────
    var projects = [];           // loaded from backend history (vai-video source only)
    var chatProjects = [];       // loaded from backend history (vai-chat source)
    var currentProject = null;   // currently selected project message object
    var currentProjectId = null; // id of the current project
    var shots = [];              // current project's shots (client-side state with refImage)
    var currentShotIndex = -1;
    var pollingTimers = {};      // operationId -> timer
    var imagePickerModal = null;
    var imagePickerMode = 'refImage'; // 'refImage' (shot editor), 'attachment' (chat input), or 'character' (storyboard character)
    var _editCharIdx = -1;   // index of character being edited (for 'character' mode image picker)
    var saveTimer = null;        // debounced save timer
    var isReadOnly = false;      // true for vai-chat sourced videos
    var sidebarTab = 'projects'; // 'projects' or 'chat'
    var viewMode = 'chat';       // 'chat' or 'advanced' — default to chat-first UX
    var currentStoryboard = null; // last AI-generated storyboard
    var chatGenerating = false;   // true while AI is generating storyboard
    var batchGenerating = false;  // true while batch-generating all shots
    var chatHistory = [];          // multi-turn conversation history [{role,content,attachments?}]
    var videoChatPendingFiles = []; // pending file attachments for video chat input
    var projectReferenceImages = []; // reference images from chat attachments (for image-to-video)
    var PENDING_PROJECT_ID = '__pending__'; // sentinel for lazy project creation (like vai-chat)
    var bgTasks = {};  // projectId -> { projectId, shots, chatHistory, storyboard, progressId, pollTimer, taskId, status, result }

    // ─── Multi-language Detection ─────────────────────────────────
    // Detects the primary language of text for prompt localization.
    // Returns a BCP-47 locale string.
    function detectLanguage(text) {
        if (!text) return 'yue-HK';
        // Cantonese-specific characters (粵語特徵字)
        var cantoneseChars = /[嘅嗰咗冇佢哋啲嚟噉咁喺唔係嘢嗮啦囉喎噃呢啊吖]/;
        // CJK Unified Ideographs (Chinese characters)
        var cjk = /[\u4e00-\u9fff\u3400-\u4dbf]/;
        // Japanese-specific (Hiragana + Katakana)
        var japanese = /[\u3040-\u309f\u30a0-\u30ff]/;
        // Korean (Hangul)
        var korean = /[\uac00-\ud7af\u1100-\u11ff]/;
        // Thai
        var thai = /[\u0e00-\u0e7f]/;
        // Arabic
        var arabic = /[\u0600-\u06ff\u0750-\u077f]/;
        // Devanagari (Hindi)
        var hindi = /[\u0900-\u097f]/;

        // Count character classes
        var chars = text.replace(/\s+/g, '');
        if (chars.length === 0) return 'yue-HK';

        // Check Cantonese first (highest priority for our users)
        if (cantoneseChars.test(text) && cjk.test(text)) return 'yue-HK';
        // Japanese (check before generic CJK because Kanji overlaps)
        if (japanese.test(text)) return 'ja-JP';
        // Korean
        if (korean.test(text)) return 'ko-KR';
        // Thai
        if (thai.test(text)) return 'th-TH';
        // Arabic
        if (arabic.test(text)) return 'ar-XA';
        // Hindi
        if (hindi.test(text)) return 'hi-IN';
        // Generic Chinese (no Cantonese markers)
        if (cjk.test(text)) {
            // Count CJK ratio — use global flag to count ALL matches
            var cjkGlobal = /[\u4e00-\u9fff\u3400-\u4dbf]/g;
            var cjkCount = (text.match(cjkGlobal) || []).length;
            if (cjkCount / chars.length > 0.1) return 'zh-TW';
        }
        // Default: English
        return 'en-US';
    }

    // ─── Initialization ───────────────────────────────────────────
    function init() {
        console.log('[VaiVideo] v4.1 Initializing (lang fix: no JSON mode)...');
        // 等 i18n 翻譯載入完成再 render，避免 I18n.t() 在 translations 未 ready 時
        // 回傳 raw key（如 "vai.video.noProjects"）而非翻譯文字
        if (typeof I18n !== 'undefined' && typeof I18n.whenReady === 'function') {
            I18n.whenReady().then(function() {
                loadHistory();
            });
        } else {
            loadHistory();
        }

        // 語言切換時重新 render sidebar lists（updatePage 只處理 data-i18n，不處理 JS 動態 innerHTML）
        window.addEventListener('languageChanged', function() {
            renderHistoryList();
            renderChatVideoList();
        });

        // Chat input: auto-resize + mic/send toggle + keydown (same as vai-chat)
        var chatInput = document.getElementById('vaiChatInput');
        if (chatInput) {
            chatInput.addEventListener('input', function() {
                this.style.height = 'auto';
                this.style.height = Math.min(this.scrollHeight, 120) + 'px';
                updateVideoMicSendToggle();
            });
            chatInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    sendChatMessage();
                }
            });
        }
        // Initialize mic/send toggle (send hidden by default, mic visible)
        updateVideoMicSendToggle();

        // SPA cleanup: stop polling when navigating away from video page
        if (typeof Router !== 'undefined' && typeof Router.registerCleanup === 'function') {
            Router.registerCleanup(function() {
                stopAllPolling();
            });
        }
    }

    // ─── History / Backend ────────────────────────────────────────
    function loadHistory() {
        App.apiRequest('/llm/video/history?page=1&limit=50').then(function(resp) {
            var all = resp.data || [];
            projects = [];
            chatProjects = [];
            for (var i = 0; i < all.length; i++) {
                var extra = all[i].extra_fields || {};
                if (extra.source === 'vai-video') {
                    projects.push(all[i]);
                } else {
                    chatProjects.push(all[i]);
                }
            }
            renderHistoryList();
            renderChatVideoList();
            console.log('[VaiVideo] Loaded', projects.length, 'projects,', chatProjects.length, 'chat videos');

            // On init: try to restore a project with active generation,
            // otherwise always create a new pending project (like vai-chat/vai-sketch).
            if (!currentProjectId) {
                var restored = false;

                // 1) Check for projects with a bgTask (SPA navigation back: running, done, or error)
                for (var bi = 0; bi < projects.length; bi++) {
                    if (bgTasks[projects[bi].id]) {
                        selectProjectData(projects[bi]);
                        restored = true;
                        console.log('[VaiVideo] Restored project with bgTask (' + bgTasks[projects[bi].id].status + '):', projects[bi].id);
                        break;
                    }
                }

                // 2) Check for projects with polling shots (page refresh during generation)
                if (!restored) {
                    for (var pi = 0; pi < projects.length; pi++) {
                        var pExtra = projects[pi].extra_fields || {};
                        if (Array.isArray(pExtra.shots)) {
                            for (var si = 0; si < pExtra.shots.length; si++) {
                                if (pExtra.shots[si].status === 'polling' && pExtra.shots[si].operationId) {
                                    selectProjectData(projects[pi]);
                                    restored = true;
                                    console.log('[VaiVideo] Restored project with polling shots:', projects[pi].id);
                                    break;
                                }
                            }
                        }
                        if (restored) break;
                    }
                }

                // 3) Always create a new pending project (consistent with vai-chat/vai-sketch)
                // Previously this restored from sessionStorage, but users expect a fresh project on refresh.
                // Old projects are still accessible from the sidebar.
                if (!restored) {
                    createProject();
                }
            }
        }).catch(function(err) {
            console.error('[VaiVideo] Failed to load history:', err);
            App.showAlert(I18n.t('vai.video.loadHistoryFailed'), 'danger');
        });
    }

    function renderHistoryList() {
        var container = document.getElementById('vaiHistoryList');
        if (!container) return;

        if (projects.length === 0) {
            container.innerHTML =
                '<div class="text-center text-muted p-4" style="font-size: 0.85rem;">' +
                    '<i class="bi bi-camera-reels d-block mb-2" style="font-size: 2rem;"></i>' +
                    I18n.t('vai.video.noProjects') + '<br>' +
                    '<small>' + I18n.t('vai.video.noProjectsHint') + '</small>' +
                '</div>';
            return;
        }

        var html = '';
        for (var i = 0; i < projects.length; i++) {
            var proj = projects[i];
            var extra = proj.extra_fields || {};
            var isActive = currentProjectId && String(proj.id) === String(currentProjectId);

            var title = proj.content || getProjectTitleFromShots(extra.shots);
            if (title.length > 50) title = title.substring(0, 50) + '...';

            // Shot stats
            var shotCount = 0;
            var doneCount = 0;
            if (Array.isArray(extra.shots)) {
                shotCount = extra.shots.length;
                for (var j = 0; j < extra.shots.length; j++) {
                    if (extra.shots[j].status === 'done') doneCount++;
                }
            }

            var timeStr = formatRelativeTime(proj.created_at || proj.CreatedAt);

            html += '<div class="vai-history-item' + (isActive ? ' active' : '') + '" ' +
                'onclick="VaiVideo.selectProject(\'' + proj.id + '\')" data-project-id="' + proj.id + '">';
            html += '<div class="d-flex align-items-start gap-2">';
            html += '<div class="flex-grow-1" style="min-width: 0;">';
            html += '<div class="vai-history-title text-truncate" style="font-size: 0.85rem; font-weight: 500;">' + escapeHtml(title) + '</div>';
            html += '<div class="d-flex align-items-center gap-2 mt-1">';
            html += '<small class="text-muted" style="font-size: 0.7rem;">' +
                '<i class="bi bi-camera-video me-1"></i>' + shotCount + I18n.t('vai.video.shots');
            if (doneCount > 0) {
                html += ' · ' + doneCount + I18n.t('vai.video.completed');
            }
            html += '</small>';
            html += '</div>';
            html += '<small class="text-muted d-block mt-1" style="font-size: 0.65rem;">' + escapeHtml(timeStr) + '</small>';
            html += '</div>';
            // Show spinner for projects with active background generation
            var hasBgTask = bgTasks[proj.id] && bgTasks[proj.id].status === 'running';
            if (hasBgTask) {
                html += '<div class="spinner-border spinner-border-sm text-primary flex-shrink-0" role="status" style="width: 16px; height: 16px; border-width: 2px;"><span class="visually-hidden">Generating...</span></div>';
            }
            html += '<button class="btn btn-sm btn-link vai-history-delete p-0" ' +
                'onclick="event.stopPropagation(); VaiVideo.deleteProject(\'' + proj.id + '\')" ' +
                'title="' + I18n.t('vai.video.deleteProject') + '"><i class="bi bi-trash3" style="font-size: 0.8rem;"></i></button>';
            html += '</div>';
            html += '</div>';
        }

        container.innerHTML = html;
    }

    function renderChatVideoList() {
        var container = document.getElementById('vaiChatVideoList');
        if (!container) return;

        if (chatProjects.length === 0) {
            container.innerHTML =
                '<div class="text-center text-muted p-4" style="font-size: 0.85rem;">' +
                    '<i class="bi bi-chat-dots d-block mb-2" style="font-size: 2rem;"></i>' +
                    I18n.t('vai.video.noChatVideos') + '<br>' +
                    '<small>' + I18n.t('vai.video.noChatVideosHint') + '</small>' +
                '</div>';
            return;
        }

        var html = '';
        for (var i = 0; i < chatProjects.length; i++) {
            var proj = chatProjects[i];
            var extra = proj.extra_fields || {};
            var isActive = currentProjectId && String(proj.id) === String(currentProjectId);

            var title = '';
            if (extra.video_info) {
                title = extra.video_info.prompt || I18n.t('vai.video.videoFromChat');
            } else {
                title = proj.content || I18n.t('vai.video.videoFromChat');
            }
            if (title.length > 50) title = title.substring(0, 50) + '...';

            var hasDone = extra.video_info && extra.video_info.video_url;
            var timeStr = formatRelativeTime(proj.created_at || proj.CreatedAt);

            html += '<div class="vai-history-item' + (isActive ? ' active' : '') + '" ' +
                'onclick="VaiVideo.selectProject(\'' + proj.id + '\')" data-project-id="' + proj.id + '">';
            html += '<div class="d-flex align-items-start gap-2">';
            html += '<div class="flex-grow-1" style="min-width: 0;">';
            html += '<div class="vai-history-title text-truncate" style="font-size: 0.85rem; font-weight: 500;">' + escapeHtml(title) + '</div>';
            html += '<div class="d-flex align-items-center gap-2 mt-1">';
            html += '<small class="text-muted" style="font-size: 0.7rem;">' +
                '<i class="bi bi-camera-video me-1"></i>1' + I18n.t('vai.video.shots');
            if (hasDone) html += ' · 1' + I18n.t('vai.video.completed');
            html += '</small>';
            html += '</div>';
            html += '<small class="text-muted d-block mt-1" style="font-size: 0.65rem;">' + escapeHtml(timeStr) + '</small>';
            html += '</div>';
            html += '<button class="btn btn-sm btn-link vai-history-delete p-0" ' +
                'onclick="event.stopPropagation(); VaiVideo.deleteChatVideo(\'' + proj.id + '\')" ' +
                'title="' + I18n.t('vai.video.deleteRecord') + '"><i class="bi bi-trash3" style="font-size: 0.8rem;"></i></button>';
            html += '</div>';
            html += '</div>';
        }

        container.innerHTML = html;
    }

    // ─── Sidebar Tab Switching ────────────────────────────────────
    function switchSidebarTab(tab) {
        sidebarTab = tab;
        var projectList = document.getElementById('vaiHistoryList');
        var chatList = document.getElementById('vaiChatVideoList');
        var tabs = document.querySelectorAll('#vaiVideoSidebar .vai-sidebar-tab');
        tabs.forEach(function(t) {
            t.classList.toggle('active', t.getAttribute('data-tab') === tab);
        });
        if (tab === 'projects') {
            if (projectList) projectList.style.display = '';
            if (chatList) chatList.style.display = 'none';
        } else {
            if (projectList) projectList.style.display = 'none';
            if (chatList) chatList.style.display = '';
        }
    }

    function getProjectTitleFromShots(shotsArr) {
        if (!Array.isArray(shotsArr) || shotsArr.length === 0) return I18n.t('vai.video.unnamedProject');
        var firstPrompt = shotsArr[0].prompt;
        if (firstPrompt && firstPrompt.trim()) {
            return firstPrompt.length > 40 ? firstPrompt.substring(0, 40) + '...' : firstPrompt;
        }
        return I18n.t('vai.video.unnamedProject');
    }

    // Helper: get aspect ratio & duration from current shot (or first shot as fallback)
    function getCurrentShotSettings() {
        var s = null;
        if (currentShotIndex >= 0 && currentShotIndex < shots.length) {
            s = shots[currentShotIndex];
        } else if (shots.length > 0) {
            s = shots[0];
        }
        return {
            aspectRatio: s ? (s.aspectRatio || '9:16') : '9:16',
            duration: s ? (s.duration || '10s') : '10s'
        };
    }

    // ─── Project Management ───────────────────────────────────────
    function createProject() {
        // If already have a pending project, just switch to it
        for (var i = 0; i < projects.length; i++) {
            if (projects[i].id === PENDING_PROJECT_ID) {
                selectProjectData(projects[i]);
                return;
            }
        }

        // Create a front-end-only pending project (no API call, no prompt())
        // Start with 0 shots — shots will be created by AI storyboard or manually
        var now = new Date();
        var pendingProj = {
            id: PENDING_PROJECT_ID,
            content: I18n.t('vai.chat.newConversation') || '新對話',
            created_at: now.toISOString(),
            updated_at: now.toISOString(),
            extra_fields: { shots: [], source: 'vai-video', chat_history: [] },
            _pending: true
        };

        projects.unshift(pendingProj);
        renderHistoryList();
        selectProjectData(pendingProj);
    }

    function selectProject(id) {
        // Find the project in our list
        var proj = null;
        for (var i = 0; i < projects.length; i++) {
            if (String(projects[i].id) === String(id)) {
                proj = projects[i];
                break;
            }
        }
        if (!proj) {
            console.warn('[VaiVideo] Project not found:', id);
            return;
        }
        selectProjectData(proj);
    }

    function selectProjectData(proj) {
        // Stop single-shot polling timers only (NOT background generation tasks)
        stopAllPolling();

        // Reset batchGenerating flag for the UI — the real generation state lives in bgTasks
        batchGenerating = false;

        currentProject = proj;
        currentProjectId = proj.id;

        // Remember last active project for page refresh recovery
        try {
            if (proj.id && proj.id !== PENDING_PROJECT_ID) {
                sessionStorage.setItem('vai_video_last_project', String(proj.id));
            }
        } catch(e) {}
        currentShotIndex = -1;

        var extra = proj.extra_fields || {};
        var isVaiChatSource = extra.source !== 'vai-video';
        isReadOnly = isVaiChatSource;

        // Check if this project has an active background task
        var bgTask = bgTasks[proj.id];

        // Load shots
        shots = [];
        if (bgTask && bgTask.status === 'running') {
            // Project has active background generation — use the task's live shots
            shots = bgTask.shots;
            batchGenerating = true;
        } else if (bgTask && (bgTask.status === 'done' || bgTask.status === 'error')) {
            // Background task finished while we were away — use its final shots
            shots = bgTask.shots;
        } else if (isVaiChatSource && extra.video_info) {
            // Create a virtual read-only shot from vai-chat video_info
            var vi = extra.video_info;
            shots = [{
                id: 'chat_' + proj.id,
                prompt: vi.prompt || '',
                refImage: null,
                aspectRatio: vi.aspect_ratio || '9:16',
                duration: vi.duration || '10s',
                status: vi.video_url ? 'done' : (vi.status === 'processing' ? 'polling' : 'draft'),
                videoUrl: vi.video_url || null,
                operationId: vi.operation_id || null,
                errorMsg: null
            }];
        } else if (Array.isArray(extra.shots)) {
            // Load shots from project data
            shots = extra.shots.map(function(s) {
                // Recover transient states: error/generating don't survive refresh
                var loadStatus = s.status || 'draft';
                if (loadStatus === 'error' || loadStatus === 'generating') {
                    loadStatus = 'draft';
                }
                return {
                    id: s.id || 'shot_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6),
                    prompt: s.prompt || '',
                    refImage: s.refImage || null,
                    aspectRatio: s.aspectRatio || '9:16',
                    duration: s.duration || '10s',
                    status: loadStatus,
                    videoUrl: s.videoUrl || null,
                    operationId: s.operationId || null,
                    errorMsg: null
                };
            });
        }

        // Update project title
        var titleEl = document.getElementById('vaiProjectTitle');
        if (titleEl) {
            var displayTitle = proj.content || getProjectTitleFromShots(shots);
            titleEl.textContent = displayTitle;
            titleEl.title = displayTitle;
        }

        // Load chat history from project — use bgTask's chat if it has a completed/running task
        if (bgTask && bgTask.chatHistory) {
            chatHistory = bgTask.chatHistory;
            // Also load storyboard from bgTask
            currentStoryboard = bgTask.storyboard || null;
        } else {
            loadChatHistory(proj);
            // Load storyboard from extra_fields if available
            var extra2 = proj.extra_fields || {};
            if (extra2.storyboard) {
                currentStoryboard = extra2.storyboard;
            } else {
                currentStoryboard = null;
            }
        }

        // Clear reference images from previous project
        projectReferenceImages = [];

        // Show settings area
        var chatSettingsEl = document.getElementById('vaiChatSettings');
        if (chatSettingsEl) chatSettingsEl.style.display = isReadOnly ? 'none' : '';

        // Render shot strip
        renderShotStrip();

        // Render history (highlight active)
        renderHistoryList();

        // Refresh generation history panel if it's open (so it shows this project's history)
        if (videoGenHistoryOpen) loadVideoGenHistory(true);

        // Always open in chat mode when selecting a project
        switchToChatMode();
        // Always restore chat bubbles (this clears old DOM first, then replays chatHistory)
        restoreChatBubbles();

        // If returning to a project with a completed background task, show the result bubble
        if (bgTask && bgTask.status === 'done') {
            _showBgTaskCompletionBubble(bgTask);
            delete bgTasks[proj.id];
        } else if (bgTask && bgTask.status === 'error') {
            _showBgTaskCompletionBubble(bgTask);
            delete bgTasks[proj.id];
        } else if (bgTask && bgTask.status === 'running') {
            // Re-attach live progress DOM for active background task
            _reattachBgProgressBubble(bgTask);
        }

        // Resume polling for any shots in polling state (single-shot polling only)
        resumePolling();

        // If recovering from page refresh: shots are polling but no bgTask exists.
        // Show a recovery progress bubble so user knows generation is still running.
        if (!bgTask) {
            var pollingShots = shots.filter(function(s) { return s.status === 'polling' && s.operationId; });
            if (pollingShots.length > 0) {
                batchGenerating = true;
                _showRecoveryProgressBubble(pollingShots.length);
            }
        }
    }

    // Show completion bubble when switching back to a project whose bgTask finished
    function _showBgTaskCompletionBubble(task) {
        // The result was already saved to chatHistory by the background task,
        // and restoreChatBubbles() above already rendered it.
        // Just scroll to bottom.
        var chatContainer = document.getElementById('vaiChatMessages');
        if (chatContainer) chatContainer.scrollTop = chatContainer.scrollHeight;
    }

    // Re-attach a live progress bubble for an active background task
    function _reattachBgProgressBubble(task) {
        var chatContainer = document.getElementById('vaiChatMessages');
        if (!chatContainer) return;

        // Check if progress bubble already exists (from restoreChatBubbles)
        var existing = document.getElementById(task.progressId);
        if (existing) return;

        // Create a new progress bubble for the running task
        var wrapper = document.createElement('div');
        wrapper.className = 'd-flex justify-content-start mb-2';
        wrapper.id = task.progressId;
        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
        wrapper.innerHTML =
            '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                '<i class="bi bi-robot"></i>' +
            '</div>' +
            '<div class="message-bubble ai position-relative" style="max-width: 70%;">' +
                '<div class="vai-batch-progress">' +
                    '<div class="d-flex align-items-center justify-content-between mb-1">' +
                        '<span class="fw-semibold"><i class="bi bi-play-circle me-1"></i>' + (I18n.t('vai.video.generatingVideo') || '影片生成中...') + '</span>' +
                        '<button class="btn btn-sm btn-link text-muted p-0 ms-2 vai-progress-close-btn" onclick="VaiVideo.dismissProgress(\'' + task.progressId + '\')" title="' + (I18n.t('vai.common.close') || '關閉') + '" style="font-size: 0.85rem; line-height: 1;">&times;</button>' +
                    '</div>' +
                    '<div class="vai-batch-progress-bar"><div class="vai-batch-progress-fill" style="width: ' + (task.lastPct || 0) + '%"></div></div>' +
                    '<div class="vai-batch-progress-text small text-muted mt-1">' +
                        (task.lastStatusText || (I18n.t('vai.video.aiProcessing') || 'AI 影片生成中...')) +
                    '</div>' +
                '</div>' +
            '</div>';
        chatContainer.appendChild(wrapper);
        chatContainer.scrollTop = chatContainer.scrollHeight;
    }

    // Show a recovery progress bubble when restoring a project with polling shots after page refresh
    // (no bgTask exists — it was lost on refresh, but shots still have operationId for polling)
    function _showRecoveryProgressBubble(pollingShotCount) {
        var chatContainer = document.getElementById('vaiChatMessages');
        if (!chatContainer) return;

        var recoveryId = 'recovery_' + Date.now();
        var wrapper = document.createElement('div');
        wrapper.className = 'd-flex justify-content-start mb-2';
        wrapper.id = recoveryId;
        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
        wrapper.innerHTML =
            '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                '<i class="bi bi-robot"></i>' +
            '</div>' +
            '<div class="message-bubble ai position-relative" style="max-width: 70%;">' +
                '<div class="vai-batch-progress">' +
                    '<div class="d-flex align-items-center justify-content-between mb-1">' +
                        '<span class="fw-semibold"><i class="bi bi-arrow-repeat me-1"></i>' + (I18n.t('vai.video.generationResumed') || '已恢復生成進度') + '</span>' +
                        '<button class="btn btn-sm btn-link text-muted p-0 ms-2 vai-progress-close-btn" onclick="VaiVideo.dismissRecovery()" title="' + (I18n.t('vai.common.close') || '關閉') + '" style="font-size: 0.85rem; line-height: 1;">&times;</button>' +
                    '</div>' +
                    '<div class="vai-batch-progress-bar"><div class="vai-batch-progress-fill" style="width: 50%"></div></div>' +
                    '<div class="vai-batch-progress-text small text-muted mt-1">' +
                        (I18n.t('vai.video.pollingResumed') || '正在繼續追蹤生成狀態...') +
                    '</div>' +
                '</div>' +
            '</div>';
        chatContainer.appendChild(wrapper);
        chatContainer.scrollTop = chatContainer.scrollHeight;

        // Store the recovery bubble ID so we can update/remove it when polling completes
        _recoveryBubbleId = recoveryId;
    }
    var _recoveryBubbleId = null;

    // Dismiss recovery progress bubble — stop polling, reset shots, persist to DB
    function dismissRecovery() {
        // Stop all single-shot polling timers
        stopAllPolling();

        // Reset polling shots to draft, clear operationId
        for (var ri = 0; ri < shots.length; ri++) {
            if (shots[ri].status === 'polling') {
                shots[ri].status = 'draft';
                shots[ri].operationId = null;
                shots[ri].errorMsg = null;
            }
        }

        batchGenerating = false;

        // Persist to DB so refresh won't resume
        if (currentProjectId && currentProjectId !== PENDING_PROJECT_ID) {
            saveProject();
        }

        // Add cancellation to chat history
        chatHistory.push({
            role: 'assistant',
            content: '[generation_cancelled]',
            generation_result: { success: false, message: 'Cancelled by user' }
        });
        saveChatHistory();

        // Remove the recovery bubble
        if (_recoveryBubbleId) {
            var el = document.getElementById(_recoveryBubbleId);
            if (el) el.remove();
            _recoveryBubbleId = null;
        }

        // Update UI
        renderShotStrip();
        renderHistoryList();
        console.log('[VaiVideo] Recovery generation dismissed by user');
    }

    function deleteProject(id) {
        if (!confirm(I18n.t('vai.video.deleteProjectConfirm'))) return;

        // Pending project: just remove from front-end, no API call
        if (id === PENDING_PROJECT_ID) {
            projects = projects.filter(function(p) { return p.id !== PENDING_PROJECT_ID; });
            if (currentProjectId === PENDING_PROJECT_ID) {
                currentProject = null;
                currentProjectId = null;
                currentShotIndex = -1;
                shots = [];
                isReadOnly = false;
                chatHistory = [];
                currentStoryboard = null;
                showEmptyState();
                clearShotStrip();
                var titleEl = document.getElementById('vaiProjectTitle');
                if (titleEl) titleEl.textContent = I18n.t('vai.video.toolTitle');
            }
            renderHistoryList();
            App.showAlert(I18n.t('vai.video.projectDeleted'), 'success');
            return;
        }

        App.apiRequest('/llm/video/history/' + id, {
            method: 'DELETE'
        }).then(function() {
            // Remove from local list
            projects = projects.filter(function(p) { return String(p.id) !== String(id); });

            // If this was the current project, clear state
            if (currentProjectId && String(currentProjectId) === String(id)) {
                stopAllPolling();
                currentProject = null;
                currentProjectId = null;
                currentShotIndex = -1;
                shots = [];
                isReadOnly = false;
                chatHistory = [];
                currentStoryboard = null;
                showEmptyState();
                clearShotStrip();
                var titleEl = document.getElementById('vaiProjectTitle');
                if (titleEl) titleEl.textContent = I18n.t('vai.video.toolTitle');
            }

            renderHistoryList();
            App.showAlert(I18n.t('vai.video.projectDeleted'), 'success');
        }).catch(function(err) {
            console.error('[VaiVideo] Failed to delete project:', err);
            App.showAlert(I18n.t('vai.video.deleteProjectFailed'), 'danger');
        });
    }

    function renameProject() {
        if (!currentProjectId) return;

        var proj = currentProject;
        if (!proj) return;

        var currentTitle = proj.content || I18n.t('vai.chat.newConversation') || '新對話';
        var newTitle = prompt(I18n.t('vai.chat.renamePrompt') || '請輸入新的名稱：', currentTitle);
        if (!newTitle || newTitle.trim() === '' || newTitle.trim() === currentTitle) return;
        newTitle = newTitle.trim();

        // Pending project: just update front-end
        if (currentProjectId === PENDING_PROJECT_ID) {
            proj.content = newTitle;
            renderHistoryList();
            var titleEl = document.getElementById('vaiProjectTitle');
            if (titleEl) { titleEl.textContent = newTitle; titleEl.title = newTitle; }
            return;
        }

        // Real project: update via API
        App.apiRequest('/llm/video/history/' + currentProjectId, {
            method: 'PATCH',
            body: JSON.stringify({ title: newTitle })
        }).then(function(resp) {
            proj.content = newTitle;
            if (resp.data) {
                var idx = findProjectIndex(currentProjectId);
                if (idx >= 0) {
                    projects[idx].content = newTitle;
                }
            }
            renderHistoryList();
            var titleEl = document.getElementById('vaiProjectTitle');
            if (titleEl) { titleEl.textContent = newTitle; titleEl.title = newTitle; }
        }).catch(function(err) {
            console.error('[VaiVideo] Failed to rename project:', err);
            App.showAlert(I18n.t('vai.chat.renameFailed') || '重新命名失敗', 'danger');
        });
    }

    function deleteChatVideo(id) {
        if (!confirm(I18n.t('vai.video.deleteChatVideoConfirm'))) return;

        App.apiRequest('/llm/video/history/' + id + '/mark-deleted', {
            method: 'PATCH'
        }).then(function() {
            // Remove from chatProjects
            chatProjects = chatProjects.filter(function(p) { return String(p.id) !== String(id); });

            // If this was the current project, clear state
            if (currentProjectId && String(currentProjectId) === String(id)) {
                stopAllPolling();
                currentProject = null;
                currentProjectId = null;
                currentShotIndex = -1;
                shots = [];
                isReadOnly = false;
                chatHistory = [];
                currentStoryboard = null;
                showEmptyState();
                clearShotStrip();
                var titleEl = document.getElementById('vaiProjectTitle');
                if (titleEl) titleEl.textContent = I18n.t('vai.video.toolTitle');
            }

            renderChatVideoList();
            App.showAlert(I18n.t('vai.video.videoRecordDeleted'), 'success');
        }).catch(function(err) {
            console.error('[VaiVideo] Failed to delete chat video:', err);
            App.showAlert(I18n.t('vai.video.deleteVideoRecordFailed'), 'danger');
        });
    }

    // ─── Auto-save (debounced) ────────────────────────────────────
    function scheduleSave() {
        if (!currentProjectId || isReadOnly) return;
        clearTimeout(saveTimer);
        saveTimer = setTimeout(function() {
            saveProject();
        }, 500);
    }

    function saveProject() {
        if (!currentProjectId || isReadOnly) return;
        if (currentProjectId === PENDING_PROJECT_ID) return; // pending — not in DB yet

        // Build shots array for backend
        // Include refImage only if it's a URL path (not base64) to avoid huge payloads
        // Transient states (error, generating) are saved as 'draft' so they don't persist across refresh
        var savableShots = shots.map(function(s) {
            var persistStatus = s.status;
            if (persistStatus === 'error' || persistStatus === 'generating' || persistStatus === 'polling') {
                persistStatus = 'draft';
            }
            var obj = {
                id: s.id,
                prompt: s.prompt,
                aspectRatio: s.aspectRatio,
                duration: s.duration,
                status: persistStatus,
                videoUrl: s.videoUrl,
                operationId: s.operationId
            };
            // Save refImage if it's a short URL path (e.g. /static/img/...); skip large base64 data
            if (s.refImage && !s.refImage.startsWith('data:')) {
                obj.refImage = s.refImage;
            }
            return obj;
        });

        var body = { shots: savableShots };

        // Also update title if we have a project title element
        var titleEl = document.getElementById('vaiProjectTitle');
        if (titleEl && titleEl.textContent && currentProject) {
            body.title = titleEl.textContent;
        }

        App.apiRequest('/llm/video/history/' + currentProjectId, {
            method: 'PATCH',
            body: JSON.stringify(body)
        }).then(function(resp) {
            // Update local project data
            if (resp.data) {
                var idx = findProjectIndex(currentProjectId);
                if (idx >= 0) {
                    projects[idx] = resp.data;
                    currentProject = resp.data;
                }
            }
            console.log('[VaiVideo] Project saved');
        }).catch(function(err) {
            console.error('[VaiVideo] Failed to save project:', err);
        });
    }

    function findProjectIndex(id) {
        for (var i = 0; i < projects.length; i++) {
            if (String(projects[i].id) === String(id)) return i;
        }
        return -1;
    }

    // ─── Chat History Persistence ─────────────────────────────────
    function saveChatHistory() {
        console.log('[VaiVideo] saveChatHistory called: projectId=' + currentProjectId + ', readOnly=' + isReadOnly + ', historyLen=' + chatHistory.length);
        if (!currentProjectId || isReadOnly) return;
        // Skip saving for pending projects (not yet in DB)
        if (currentProjectId === PENDING_PROJECT_ID) {
            console.log('[VaiVideo] saveChatHistory skipped: project is pending');
            return;
        }
        // Save chat_history + storyboard into the project's extra_fields via PATCH
        var saveData = { chat_history: chatHistory };
        if (currentStoryboard) {
            saveData.storyboard = currentStoryboard;
        }
        App.apiRequest('/llm/video/history/' + currentProjectId, {
            method: 'PATCH',
            body: JSON.stringify(saveData)
        }).then(function() {
            console.log('[VaiVideo] Chat history + storyboard saved OK, items=' + chatHistory.length);
        }).catch(function(err) {
            console.error('[VaiVideo] Failed to save chat history:', err);
        });
    }

    // Ensure a project exists before saving chat. If pending, auto-create in DB.
    // IMPORTANT: Does NOT call selectProjectData (which would clear chatHistory and DOM).
    // Only sets currentProject/currentProjectId and adds to sidebar list.
    // Returns a Promise that resolves when currentProjectId is a real DB id.
    function ensureProject(titleHint) {
        // Already have a real (non-pending) project
        if (currentProjectId && currentProjectId !== PENDING_PROJECT_ID) {
            return Promise.resolve();
        }

        // Build title from hint or default
        var title = titleHint || (I18n.t('vai.video.projectPrefix') || 'vAi Video ');
        // Truncate to reasonable length (vai-chat style: 20 chars + ...)
        if (title.length > 20) title = title.substring(0, 20) + '...';

        return App.apiRequest('/llm/video/history', {
            method: 'POST',
            body: JSON.stringify({ title: title, shots: [], status: 'active' })
        }).then(function(resp) {
            var newProject = resp.data || resp;
            if (resp.id && !newProject.id) newProject.id = resp.id;

            // Remove the pending project from list (if any)
            projects = projects.filter(function(p) { return p.id !== PENDING_PROJECT_ID; });

            // Set project state WITHOUT clearing chat (no selectProjectData)
            currentProject = newProject;
            currentProjectId = newProject.id;
            isReadOnly = false;

            // Remember this project for page refresh recovery
            try { sessionStorage.setItem('vai_video_last_project', String(newProject.id)); } catch(e) {}

            // Preserve shots if we had any; auto-create shot 1 if empty
            if (!shots || shots.length === 0) {
                var arEl = document.getElementById('vaiVideoAspectRatio');
                var durEl = document.getElementById('vaiVideoDuration');
                shots = [{
                    id: 'shot_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6),
                    prompt: '',
                    refImage: null,
                    aspectRatio: arEl ? arEl.value : '9:16',
                    duration: durEl ? durEl.value : '10s',
                    status: 'draft',
                    videoUrl: null,
                    operationId: null,
                    errorMsg: null
                }];
            }

            // Add real project to sidebar and highlight
            projects.unshift(newProject);
            renderHistoryList();

            // Show aspect ratio in toolbar (now that we have a project)
            renderShotStrip();

            // Update title
            var titleEl = document.getElementById('vaiProjectTitle');
            if (titleEl) {
                titleEl.textContent = title;
                titleEl.title = title;
            }

            console.log('[VaiVideo] Auto-created project:', newProject.id, 'title:', title, 'chatHistory preserved:', chatHistory.length);
        });
    }

    function loadChatHistory(proj) {
        chatHistory = [];
        var extra = proj.extra_fields || {};
        if (Array.isArray(extra.chat_history)) {
            chatHistory = extra.chat_history;
        }
    }

    function restoreChatBubbles() {
        // Restore chat bubbles from chatHistory into the DOM
        var container = document.getElementById('vaiChatMessages');
        if (!container) return;
        // Clear ALL existing chat bubbles and indicators
        var bubbles = container.querySelectorAll('.d-flex.mb-2, .ai-thinking-indicator');
        for (var i = 0; i < bubbles.length; i++) {
            bubbles[i].remove();
        }

        // If no history, show welcome message
        if (chatHistory.length === 0) {
            container.innerHTML =
                '<div class="d-flex justify-content-start mb-2">' +
                    '<div class="avatar-circle me-2 flex-shrink-0" style="width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;">' +
                        '<i class="bi bi-robot"></i>' +
                    '</div>' +
                    '<div class="message-bubble ai position-relative" style="max-width: 70%;">' +
                        '<div class="ai-msg-content">' +
                            '<p class="mb-2 fw-semibold">' + I18n.t('vai.video.chatWelcome') + '</p>' +
                            '<p class="mb-1 small">' + I18n.t('vai.video.chatWelcomeDesc') + '</p>' +
                            '<div class="vai-chat-suggestions">' +
                                '<button class="vai-chat-suggestion" onclick="VaiVideo.useSuggestion(this)">' + I18n.t('vai.video.chatSuggest1') + '</button>' +
                                '<button class="vai-chat-suggestion" onclick="VaiVideo.useSuggestion(this)">' + I18n.t('vai.video.chatSuggest2') + '</button>' +
                                '<button class="vai-chat-suggestion" onclick="VaiVideo.useSuggestion(this)">' + I18n.t('vai.video.chatSuggest3') + '</button>' +
                            '</div>' +
                        '</div>' +
                    '</div>' +
                '</div>';
            return;
        }

        // Replay history
        for (var j = 0; j < chatHistory.length; j++) {
            var msg = chatHistory[j];
            if (msg.role === 'user') {
                appendChatBubble('user', msg.content || '', msg.attachments || []);
            } else if (msg.role === 'assistant') {
                if (msg.content === '[storyboard generated]' && currentStoryboard) {
                    // Re-render the storyboard bubble
                    var ss = getCurrentShotSettings();
                    appendStoryboardBubble(currentStoryboard, ss.aspectRatio, ss.duration);
                } else if (msg.content === '[generation_success]') {
                    // Re-render generation success bubble
                    _restoreGenerationResultBubble(true, msg.generation_result ? msg.generation_result.message : null);
                } else if (msg.content === '[generation_error]') {
                    // Re-render generation error bubble
                    _restoreGenerationResultBubble(false, msg.generation_result ? msg.generation_result.message : null);
                } else if (msg.content === '[playback]') {
                    // Re-render playback bubble (if shots have video URLs)
                    var completedShots = shots.filter(function(s) { return s.status === 'done' && s.videoUrl; });
                    if (completedShots.length > 0) {
                        _showCombinedChatBubble(completedShots);
                    }
                } else if (msg.content && msg.content !== '[storyboard generated]') {
                    appendChatBubble('ai', msg.content);
                }
            }
        }
    }

    // Restore a generation result bubble (used by restoreChatBubbles)
    function _restoreGenerationResultBubble(success, msg) {
        var container = document.getElementById('vaiChatMessages');
        if (!container) return;

        var wrapper = document.createElement('div');
        wrapper.className = 'd-flex justify-content-start mb-2';
        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';

        var innerHtml;
        if (success) {
            innerHtml =
                '<div class="vai-batch-progress">' +
                    '<div class="fw-semibold text-success"><i class="bi bi-check-circle-fill me-1"></i>' + (msg || (I18n.t('vai.video.allShotsComplete') || '影片已生成完成！')) + '</div>' +
                    '<div class="mt-2">' +
                        '<button class="btn btn-primary btn-sm me-2" onclick="VaiVideo.combineAll()"><i class="bi bi-film me-1"></i>' + (I18n.t('vai.video.play') || '播放') + '</button>' +
                        '<button class="btn btn-outline-secondary btn-sm" onclick="VaiVideo.downloadComplete()"><i class="bi bi-download me-1"></i>' + (I18n.t('vai.common.download') || '下載') + '</button>' +
                    '</div>' +
                '</div>';
        } else {
            innerHtml =
                '<div class="vai-batch-progress">' +
                    '<div class="fw-semibold text-danger"><i class="bi bi-exclamation-triangle-fill me-1"></i>' + escapeHtml(msg || (I18n.t('vai.video.batchFailed') || 'Generation failed')) + '</div>' +
                    '<div class="mt-1 small text-muted">' + (I18n.t('vai.video.tryAgainHint') || '你可以修改分鏡後重新生成') + '</div>' +
                '</div>';
        }

        wrapper.innerHTML =
            '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                '<i class="bi bi-robot"></i>' +
            '</div>' +
            '<div class="message-bubble ai position-relative" style="max-width: 70%;">' +
                innerHtml +
            '</div>';

        container.appendChild(wrapper);
    }

    // ─── Shot Strip ───────────────────────────────────────────────
    function renderShotStrip() {
        // Shot strip removed — show aspect ratio in toolbar when a project is active
        var arGroup = document.getElementById('vaiAspectRatioGroup');
        if (arGroup) {
            arGroup.style.display = currentProjectId ? 'flex' : 'none';
        }
    }

    function clearShotStrip() {
        var arGroup = document.getElementById('vaiAspectRatioGroup');
        if (arGroup) arGroup.style.display = 'none';
    }

    // ─── Shot Management ──────────────────────────────────────────

    var MAX_TOTAL_DURATION_SEC = 15;
    var _recalcEditTotalDuration = function() {}; // set by editStoryboard()

    // Parse duration string like "5s", "10s", "15" → integer seconds
    function _parseDurationSec(durStr) {
        if (!durStr) return 5;
        var n = parseInt(String(durStr).replace(/s$/i, ''), 10);
        return isNaN(n) || n < 3 ? 5 : n;
    }

    // Calculate total duration of a storyboard's shots in seconds
    function _calcTotalDurationSec(sbOrShots) {
        var arr = Array.isArray(sbOrShots) ? sbOrShots : (sbOrShots && sbOrShots.shots ? sbOrShots.shots : []);
        var total = 0;
        for (var i = 0; i < arr.length; i++) {
            total += _parseDurationSec(arr[i].duration);
        }
        return total;
    }

    // Clamp a storyboard so total duration <= maxSec.
    // Strategy: proportionally scale down shot durations, min 3s each.
    // Modifies storyboard.shots in-place and returns total seconds after clamping.
    function _clampStoryboardDuration(storyboard, maxSec) {
        if (!storyboard || !storyboard.shots || storyboard.shots.length === 0) return 0;
        var total = _calcTotalDurationSec(storyboard);
        if (total <= maxSec) return total;

        console.warn('[VaiVideo] Storyboard total ' + total + 's exceeds ' + maxSec + 's, clamping...');
        var sbShots = storyboard.shots;
        var ratio = maxSec / total;
        var newTotal = 0;
        for (var i = 0; i < sbShots.length; i++) {
            var orig = _parseDurationSec(sbShots[i].duration);
            var scaled = Math.max(3, Math.round(orig * ratio));
            // Don't exceed remaining budget
            var remaining = maxSec - newTotal;
            if (scaled > remaining) scaled = Math.max(3, remaining);
            if (newTotal + scaled > maxSec && i < sbShots.length - 1) {
                scaled = Math.max(3, maxSec - newTotal);
            }
            sbShots[i].duration = scaled + 's';
            newTotal += scaled;
            if (newTotal >= maxSec) {
                // Truncate remaining shots
                sbShots.splice(i + 1);
                break;
            }
        }
        console.log('[VaiVideo] Clamped to ' + newTotal + 's (' + sbShots.length + ' shots)');
        return newTotal;
    }

    // Sync shots[] from a storyboard response.
    // - Preserves existing shots that already have video (status === 'done')
    // - Updates draft shots with new storyboard descriptions
    // - Adds/removes shots to match storyboard length
    function syncShotsFromStoryboard(storyboard, aspectRatio, duration) {
        if (!storyboard || !storyboard.shots || storyboard.shots.length === 0) return;

        // Auto-clamp total duration to 15s if AI returned more
        _clampStoryboardDuration(storyboard, MAX_TOTAL_DURATION_SEC);

        var sbShots = storyboard.shots;
        var newShots = [];

        for (var i = 0; i < sbShots.length; i++) {
            var sbShot = sbShots[i];
            if (i < shots.length) {
                // Existing shot — update only if it's still a draft (no video generated yet)
                var existing = shots[i];
                if (existing.status === 'done' && existing.videoUrl) {
                    // Keep the existing shot (already has video), just update prompt for reference
                    existing.prompt = sbShot.description || existing.prompt;
                    newShots.push(existing);
                } else {
                    // Draft shot — overwrite with storyboard data
                    existing.prompt = sbShot.description || '';
                    existing.duration = sbShot.duration || duration;
                    existing.aspectRatio = aspectRatio;
                    newShots.push(existing);
                }
            } else {
                // New shot from storyboard
                newShots.push({
                    id: 'shot_' + Date.now() + '_' + i + '_' + Math.random().toString(36).substr(2, 4),
                    prompt: sbShot.description || '',
                    refImage: null,
                    aspectRatio: aspectRatio,
                    duration: sbShot.duration || duration,
                    status: 'draft',
                    videoUrl: null,
                    operationId: null,
                    errorMsg: null
                });
            }
        }

        shots = newShots;
        renderShotStrip();
        scheduleSave();
        console.log('[VaiVideo] Synced ' + shots.length + ' shots from storyboard');
    }

    function addShot() {
        if (isReadOnly) return;
        if (!currentProjectId) {
            App.showAlert(I18n.t('vai.video.selectProjectFirst'), 'warning');
            return;
        }

        var arEl = document.getElementById('vaiVideoAspectRatio');
        var durEl = document.getElementById('vaiVideoDuration');

        var shot = {
            id: 'shot_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6),
            prompt: '',
            refImage: null,
            aspectRatio: arEl ? arEl.value : '9:16',
            duration: durEl ? durEl.value : '10s',
            status: 'draft',
            videoUrl: null,
            operationId: null,
            errorMsg: null
        };

        shots.push(shot);
        renderShotStrip();
        selectShot(shots.length - 1);
        scheduleSave();
    }

    function selectShot(index) {
        if (index < 0 || index >= shots.length) return;
        currentShotIndex = index;
        var shot = shots[index];

        // Switch to advanced (editor) mode — hide chat, show editor
        viewMode = 'advanced';
        var chatEl = document.getElementById('vaiVideoChat');
        var emptyEl = document.getElementById('vaiVideoEmpty');
        var editorEl = document.getElementById('vaiShotEditor');
        if (chatEl) chatEl.style.display = 'none';
        if (emptyEl) emptyEl.style.display = 'none';
        if (editorEl) editorEl.style.display = 'block';

        // Load shot data into editor
        var promptEl = document.getElementById('vaiShotPrompt');
        if (promptEl) {
            promptEl.value = shot.prompt || '';
            promptEl.disabled = isReadOnly;
        }

        // Aspect ratio & duration
        var arEl = document.getElementById('vaiVideoAspectRatio');
        if (arEl) {
            arEl.value = shot.aspectRatio || '9:16';
            arEl.disabled = isReadOnly;
        }
        var durEl = document.getElementById('vaiVideoDuration');
        if (durEl) {
            durEl.value = shot.duration || '10s';
            durEl.disabled = isReadOnly;
        }

        // Reference image
        updateRefImageDisplay(shot);

        // Video preview
        updateVideoPreview(shot);

        // Generate button state
        updateGenerateButtonState(shot);

        // Progress bars
        showProgress(shot.status === 'generating' || shot.status === 'polling');

        // Update shot strip highlight
        renderShotStrip();

        // Hide combined result when switching shots
        closeCombinedResult();

        // Update "use prev shot frame" button visibility
        updatePrevFrameButton();

        // Apply previous-shot continuity reference (auto-capture last frame)
        if (index > 0) {
            applyPrevShotContinuity(index);
        } else {
            hidePrevShotBanner();
        }
    }

    function deleteShot() {
        if (isReadOnly) return;
        if (currentShotIndex < 0 || currentShotIndex >= shots.length) return;
        if (!confirm(I18n.t('vai.video.deleteShotConfirm').replace('{num}', currentShotIndex + 1))) return;

        // Stop polling if active
        var shot = shots[currentShotIndex];
        if (shot.operationId && pollingTimers[shot.operationId]) {
            clearInterval(pollingTimers[shot.operationId]);
            delete pollingTimers[shot.operationId];
        }

        shots.splice(currentShotIndex, 1);

        if (shots.length > 0) {
            var newIndex = currentShotIndex >= shots.length ? shots.length - 1 : currentShotIndex;
            currentShotIndex = -1; // reset before selecting
            renderShotStrip();
            selectShot(newIndex);
        } else {
            currentShotIndex = -1;
            renderShotStrip();
            showEmptyState();
        }

        scheduleSave();
    }

    function showEmptyState() {
        // Show chat mode instead of old empty state
        showChatMode();
    }

    function showChatMode() {
        viewMode = 'chat';
        var chatEl = document.getElementById('vaiVideoChat');
        var emptyEl = document.getElementById('vaiVideoEmpty');
        var editorEl = document.getElementById('vaiShotEditor');
        if (chatEl) chatEl.style.display = 'flex';
        if (emptyEl) emptyEl.style.display = 'none';
        if (editorEl) editorEl.style.display = 'none';
        closeCombinedResult();

        // Deselect current shot in strip
        currentShotIndex = -1;
        renderShotStrip();
    }

    function showAdvancedMode() {
        viewMode = 'advanced';
        var chatEl = document.getElementById('vaiVideoChat');
        if (chatEl) chatEl.style.display = 'none';

        // Show standard shot strip and editor
        renderShotStrip();
        if (shots.length > 0) {
            selectShot(0);
        }
    }

    function switchToAdvancedMode() {
        if (!currentProjectId || shots.length === 0) {
            App.showAlert(I18n.t('vai.video.noStoryboardYet') || 'Please generate a storyboard first', 'warning');
            return;
        }
        showAdvancedMode();
    }

    function switchToChatMode() {
        showChatMode();
    }

    // ─── Status Icon ──────────────────────────────────────────────
    function getStatusIcon(status) {
        switch (status) {
            case 'generating':
            case 'polling':
                return '<span class="spinner-border spinner-border-sm text-primary" style="width: 12px; height: 12px;"></span>';
            case 'done':
                return '<i class="bi bi-check-circle-fill text-success" style="font-size: 0.75rem;"></i>';
            case 'error':
                return '<i class="bi bi-exclamation-circle-fill text-danger" style="font-size: 0.75rem;"></i>';
            default:
                return '<i class="bi bi-circle text-muted" style="font-size: 0.65rem;"></i>';
        }
    }

    // ─── Shot Settings ────────────────────────────────────────────
    function updateShotSetting(key, value) {
        if (isReadOnly) return;
        if (key === 'aspectRatio') {
            // Aspect ratio is a project-level setting — apply to ALL shots
            for (var i = 0; i < shots.length; i++) {
                shots[i][key] = value;
            }
        } else {
            if (currentShotIndex < 0 || currentShotIndex >= shots.length) return;
            shots[currentShotIndex][key] = value;
        }
        renderShotStrip();
        scheduleSave();
    }

    // ─── Reference Image ──────────────────────────────────────────
    function handleImageUpload(event) {
        var file = event.target.files[0];
        if (!file) return;
        event.target.value = '';

        // Close modal if open
        if (imagePickerModal) imagePickerModal.hide();

        if (imagePickerMode === 'attachment') {
            // In attachment mode: add file as a chat attachment (upload it)
            videoChatPendingFiles.push({
                file: file,
                id: 'vf_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6),
                uploaded: false,
                url: null,
                name: file.name
            });
            uploadVideoChatPendingFiles();
            renderVideoChatFilePreview();
            return;
        }

        if (imagePickerMode === 'character') {
            // In character mode: upload file and set as character image
            var reader = new FileReader();
            reader.onload = function(e) {
                setCharacterImage(e.target.result);
            };
            reader.readAsDataURL(file);
            return;
        }

        // In refImage mode: set as shot reference image
        var reader = new FileReader();
        reader.onload = function(e) {
            if (currentShotIndex >= 0 && currentShotIndex < shots.length) {
                shots[currentShotIndex].refImage = e.target.result;
                updateRefImageDisplay(shots[currentShotIndex]);
                renderShotStrip();
                detectAndSetAspectRatio(e.target.result, currentShotIndex);
            }
        };
        reader.readAsDataURL(file);
    }

    // Dispatch image selection based on current picker mode
    function onImagePickerSelect(url, name) {
        if (imagePickerMode === 'attachment') {
            addImageAsAttachment(url, name || 'image');
        } else if (imagePickerMode === 'character') {
            setCharacterImage(url);
        } else {
            setRefImage(url);
        }
    }

    // Add a URL-based image as a chat attachment (pending file)
    function addImageAsAttachment(url, name) {
        var ext = (url.split('.').pop() || 'jpg').split('?')[0].toLowerCase();
        if (['jpg', 'jpeg', 'png', 'gif', 'webp'].indexOf(ext) < 0) ext = 'jpg';
        var filename = (name || 'image') + (name && name.indexOf('.') >= 0 ? '' : '.' + ext);
        var mimeType = 'image/' + (ext === 'jpg' ? 'jpeg' : ext);

        var fileId = 'vf_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6);
        videoChatPendingFiles.push({
            file: null,           // no File object — it's a URL-based attachment
            id: fileId,
            uploaded: true,       // already accessible via URL
            url: url,
            name: filename,
            mime_type: mimeType,
            base64: null,
            _isUrlImage: true     // flag to render as thumbnail instead of icon
        });

        if (imagePickerModal) imagePickerModal.hide();
        renderVideoChatFilePreview();
    }

    function setRefImage(dataUrl) {
        if (currentShotIndex >= 0 && currentShotIndex < shots.length) {
            shots[currentShotIndex].refImage = dataUrl;
            updateRefImageDisplay(shots[currentShotIndex]);
            renderShotStrip();
            detectAndSetAspectRatio(dataUrl, currentShotIndex);
        }
        if (imagePickerModal) imagePickerModal.hide();
    }

    // Set image for a storyboard character (used by image picker in 'character' mode)
    function setCharacterImage(imageUrl) {
        if (_editCharIdx < 0) return;
        var charCard = document.querySelector('.vai-edit-char-card[data-char-idx="' + _editCharIdx + '"]');
        if (charCard) {
            // Update data attribute to store the new image URL
            charCard.setAttribute('data-image-url', imageUrl);
            // Update the visual preview (preserve the pencil edit icon overlay)
            var imgContainer = charCard.querySelector('.vai-edit-char-img');
            if (imgContainer) {
                imgContainer.innerHTML =
                    '<img src="' + escapeAttr(imageUrl) + '" style="width:48px;height:48px;object-fit:cover;border-radius:6px;">' +
                    '<div style="position:absolute;bottom:-2px;right:-2px;width:18px;height:18px;border-radius:50%;background:#0d6efd;color:#fff;display:flex;align-items:center;justify-content:center;font-size:10px;border:2px solid #fff;">' +
                        '<i class="bi bi-pencil-fill"></i>' +
                    '</div>';
            }
        }
        _editCharIdx = -1;
        if (imagePickerModal) imagePickerModal.hide();
    }

    function removeRefImage() {
        if (currentShotIndex >= 0 && currentShotIndex < shots.length) {
            shots[currentShotIndex].refImage = null;
            updateRefImageDisplay(shots[currentShotIndex]);
            renderShotStrip();
        }
    }

    // Auto-detect aspect ratio from reference image and update shot + dropdown
    function detectAndSetAspectRatio(imageSrc, shotIndex) {
        if (!imageSrc || shotIndex < 0 || shotIndex >= shots.length) return;
        var img = new Image();
        img.onload = function() {
            var w = img.naturalWidth;
            var h = img.naturalHeight;
            if (!w || !h) return;
            var ratio = w / h;
            var closest;
            if (ratio < 0.75) {
                closest = '9:16';
            } else {
                closest = '16:9';
            }
            if (shotIndex < shots.length) {
                shots[shotIndex].aspectRatio = closest;
                // Update dropdown if this shot is currently selected
                if (currentShotIndex === shotIndex) {
                    var arEl = document.getElementById('vaiVideoAspectRatio');
                    if (arEl) arEl.value = closest;
                }
                scheduleSave();
            }
        };
        img.onerror = function() {
            // Detection failed — leave aspect ratio unchanged
        };
        img.src = imageSrc;
    }

    function updateRefImageDisplay(shot) {
        var previewEl = document.getElementById('vaiRefPreview');
        var emptyEl = document.getElementById('vaiRefEmpty');
        var wrapperEl = document.getElementById('vaiRefImgWrapper');
        var imgEl = document.getElementById('vaiRefImg');

        if (shot && shot.refImage) {
            if (emptyEl) emptyEl.style.display = 'none';
            if (wrapperEl) wrapperEl.style.display = 'block';
            if (imgEl) imgEl.src = shot.refImage;
        } else {
            if (emptyEl) emptyEl.style.display = 'flex';
            if (wrapperEl) wrapperEl.style.display = 'none';
            if (imgEl) imgEl.src = '';
        }
    }

    // ─── Image Picker Modal ───────────────────────────────────────
    // mode: 'refImage' (default, for shot editor) or 'attachment' (for chat input)
    function openImagePicker(mode) {
        if (mode !== 'attachment' && isReadOnly) return;
        imagePickerMode = mode || 'refImage';

        if (!imagePickerModal) {
            var el = document.getElementById('vaiVideoImagePickerModal');
            if (el && typeof bootstrap !== 'undefined') {
                imagePickerModal = new bootstrap.Modal(el);
            }
        }
        if (imagePickerModal) {
            // Update modal title based on mode
            var titleEl = document.querySelector('#vaiVideoImagePickerModal .modal-title');
            if (titleEl) {
                if (imagePickerMode === 'attachment') {
                    titleEl.textContent = I18n.t('vai.video.addAttachment') || '新增附件';
                } else if (imagePickerMode === 'character') {
                    titleEl.textContent = I18n.t('vai.video.selectCharImage') || '選擇角色圖片';
                } else {
                    titleEl.textContent = I18n.t('vai.video.selectRefImage') || '選擇參考圖片';
                }
            }
            // Reset to first tab (Upload) when opening
            var firstTab = document.querySelector('#vaiVideoImagePickerModal .nav-link:first-child');
            if (firstTab && typeof bootstrap !== 'undefined') {
                var tab = new bootstrap.Tab(firstTab);
                tab.show();
            }
            imagePickerModal.show();
            loadProductImages();
            loadSketchImages();
        }
    }

    function loadProductImages(search) {
        var grid = document.getElementById('vaiVidProductImgGrid');
        if (!grid) return;

        grid.innerHTML =
            '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' +
                '<span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.video.loadingProducts') +
            '</div>';

        var url = '/products?limit=50';
        if (search && search.trim()) url += '&search=' + encodeURIComponent(search.trim());

        App.apiRequest(url).then(function(resp) {
            var products = resp.data || resp || [];
            renderProductImageGrid(products, grid);
        }).catch(function() {
            grid.innerHTML =
                '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' + I18n.t('vai.video.loadFailed') + '</div>';
        });
    }

    var productSearchTimer = null;
    function searchProductImages(query) {
        clearTimeout(productSearchTimer);
        productSearchTimer = setTimeout(function() {
            loadProductImages(query);
        }, 300);
    }

    function renderProductImageGrid(products, grid) {
        var withImages = products.filter(function(p) { return p.image_url && p.image_url.trim(); });

        if (withImages.length === 0) {
            grid.innerHTML =
                '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' +
                    '<i class="bi bi-image d-block mb-1" style="font-size: 1.5rem;"></i>' +
                    I18n.t('vai.video.noProductImages') +
                '</div>';
            return;
        }

        grid.innerHTML = withImages.map(function(p) {
            var safeUrl = escapeHtml(p.image_url).replace(/'/g, "\\'");
            var safeName = escapeHtml(p.name || '').replace(/'/g, "\\'");
            return '<div class="vai-product-img-item" onclick="VaiVideo.onImagePickerSelect(\'' + safeUrl + '\', \'' + safeName + '\')" ' +
                'title="' + escapeHtml(p.name) + '">' +
                '<img src="' + escapeHtml(p.image_url) + '" alt="' + escapeHtml(p.name) + '" loading="lazy">' +
                '<div class="vai-product-img-name">' + escapeHtml(p.name) + '</div>' +
            '</div>';
        }).join('');
    }

    function loadSketchImages() {
        var grid = document.getElementById('vaiVidSketchImgGrid');
        if (!grid) return;

        grid.innerHTML =
            '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' +
                '<span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.video.loadingSketches') +
            '</div>';

        App.apiRequest('/ai/sketches').then(function(resp) {
            var sketches = resp.data || resp || [];
            renderSketchImageGrid(sketches, grid);
        }).catch(function() {
            grid.innerHTML =
                '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' + I18n.t('vai.video.loadFailed') + '</div>';
        });
    }

    function renderSketchImageGrid(sketches, grid) {
        var withThumbs = sketches.filter(function(s) { return s.thumbnail && s.thumbnail.trim(); });

        if (withThumbs.length === 0) {
            grid.innerHTML =
                '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' +
                    '<i class="bi bi-brush d-block mb-1" style="font-size: 1.5rem;"></i>' +
                    I18n.t('vai.video.noSketchImages') +
                '</div>';
            return;
        }

        grid.innerHTML = withThumbs.map(function(s) {
            var safeUrl = escapeHtml(s.thumbnail).replace(/'/g, "\\'");
            var title = escapeHtml(s.title || I18n.t('vai.video.imageFallbackTitle'));
            var safeName = title.replace(/'/g, "\\'");
            return '<div class="vai-product-img-item" onclick="VaiVideo.onImagePickerSelect(\'' + safeUrl + '\', \'' + safeName + '\')" ' +
                'title="' + title + '">' +
                '<img src="' + escapeHtml(s.thumbnail) + '" alt="' + title + '" loading="lazy">' +
                '<div class="vai-product-img-name">' + title + '</div>' +
            '</div>';
        }).join('');
    }

    // Use last frame of previous shot as reference image (button in form)
    function usePrevShotFrame() {
        if (currentShotIndex <= 0 || currentShotIndex >= shots.length) return;

        var prevShot = shots[currentShotIndex - 1];
        if (!prevShot || prevShot.status !== 'done' || !prevShot.videoUrl) return;

        var shotIdx = currentShotIndex;
        var btn = document.getElementById('vaiUsePrevFrameBtn');
        if (btn) {
            btn.disabled = true;
            btn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.video.prevShotContinuity');
        }

        captureVideoLastFrame(prevShot.videoUrl, function(frameDataUrl) {
            // Re-enable button
            if (btn) {
                btn.disabled = false;
                btn.innerHTML = '<i class="bi bi-link-45deg me-1"></i>' + I18n.t('vai.video.prevShotContinuity');
            }

            if (!frameDataUrl) {
                App.showAlert(I18n.t('vai.video.captureFrameFailed') || 'Failed to capture frame', 'warning');
                return;
            }

            if (shotIdx < shots.length) {
                shots[shotIdx].refImage = frameDataUrl;
                shots[shotIdx]._prevShotRef = true;
                if (currentShotIndex === shotIdx) {
                    updateRefImageDisplay(shots[shotIdx]);
                    showPrevShotBanner(frameDataUrl);
                }
                renderShotStrip();
                detectAndSetAspectRatio(frameDataUrl, shotIdx);
            }
        });
    }

    // Show/hide the "use prev shot frame" button based on prev shot state
    function updatePrevFrameButton() {
        var btn = document.getElementById('vaiUsePrevFrameBtn');
        if (!btn) return;

        var show = false;
        if (!isReadOnly && currentShotIndex > 0 && currentShotIndex < shots.length) {
            var prevShot = shots[currentShotIndex - 1];
            var currentShot = shots[currentShotIndex];
            // Show only if prev shot is done with video, and current shot doesn't already have a prev-shot ref
            if (prevShot && prevShot.status === 'done' && prevShot.videoUrl && !currentShot._prevShotRef) {
                show = true;
            }
        }
        btn.style.display = show ? '' : 'none';
    }

    // ─── Previous Shot Continuity (Capture Last Frame) ────────────
    // Captures the last frame of a video element and returns a data URL
    function captureVideoLastFrame(videoUrl, callback) {
        var video = document.createElement('video');
        video.crossOrigin = 'anonymous';
        video.preload = 'auto';
        video.muted = true;
        video.playsInline = true;

        video.onloadedmetadata = function() {
            // Seek to last frame (duration - small epsilon)
            var targetTime = Math.max(0, video.duration - 0.05);
            video.currentTime = targetTime;
        };

        video.onseeked = function() {
            try {
                var canvas = document.createElement('canvas');
                canvas.width = video.videoWidth;
                canvas.height = video.videoHeight;
                var ctx = canvas.getContext('2d');
                ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
                var dataUrl = canvas.toDataURL('image/jpeg', 0.85);
                callback(dataUrl);
            } catch (e) {
                console.warn('[VaiVideo] Failed to capture last frame:', e);
                callback(null);
            }
            // Clean up
            video.src = '';
            video.load();
        };

        video.onerror = function() {
            console.warn('[VaiVideo] Failed to load video for frame capture');
            callback(null);
        };

        video.src = videoUrl;
        video.load();
    }

    // Check if previous shot has a completed video; if so, capture last frame
    // and set as reference for the current shot
    function applyPrevShotContinuity(shotIndex) {
        if (shotIndex <= 0 || shotIndex >= shots.length) {
            hidePrevShotBanner();
            return;
        }

        var prevShot = shots[shotIndex - 1];
        var currentShot = shots[shotIndex];

        // Only apply if previous shot is done with a video URL
        if (prevShot.status !== 'done' || !prevShot.videoUrl) {
            hidePrevShotBanner();
            return;
        }

        // If current shot already has a manually set refImage, don't override
        if (currentShot.refImage && !currentShot._prevShotRef) {
            hidePrevShotBanner();
            return;
        }

        // Capture last frame from previous shot's video
        captureVideoLastFrame(prevShot.videoUrl, function(frameDataUrl) {
            if (!frameDataUrl) {
                hidePrevShotBanner();
                return;
            }

            // Set as reference image (mark as auto-applied from prev shot)
            currentShot.refImage = frameDataUrl;
            currentShot._prevShotRef = true;

            // Update display if this shot is still selected
            if (currentShotIndex === shotIndex) {
                updateRefImageDisplay(currentShot);
                showPrevShotBanner(frameDataUrl);

                // Auto-prepend continuity hint to prompt if empty or if not already hinted
                var promptEl = document.getElementById('vaiShotPrompt');
                if (promptEl) {
                    var continuityHint = I18n.t('vai.video.prevShotPromptHint');
                    if (!promptEl.value.includes(continuityHint)) {
                        var prefix = continuityHint + '\n';
                        promptEl.value = prefix + promptEl.value;
                        currentShot.prompt = promptEl.value;
                    }
                }
            }
        });
    }

    function showPrevShotBanner(thumbUrl) {
        var banner = document.getElementById('vaiPrevShotBanner');
        var thumb = document.getElementById('vaiPrevShotThumb');
        if (banner) banner.style.display = '';
        if (thumb && thumbUrl) thumb.src = thumbUrl;
    }

    function hidePrevShotBanner() {
        var banner = document.getElementById('vaiPrevShotBanner');
        if (banner) banner.style.display = 'none';
    }

    function removePrevShotRef() {
        if (currentShotIndex >= 0 && currentShotIndex < shots.length) {
            var shot = shots[currentShotIndex];
            shot.refImage = null;
            shot._prevShotRef = false;
            updateRefImageDisplay(shot);
            hidePrevShotBanner();

            // Remove continuity hint from prompt
            var promptEl = document.getElementById('vaiShotPrompt');
            if (promptEl) {
                var hint = I18n.t('vai.video.prevShotPromptHint');
                promptEl.value = promptEl.value.replace(hint + '\n', '').replace(hint, '');
                shot.prompt = promptEl.value;
            }
        }
    }

    // ─── Video Generation ─────────────────────────────────────────

    // Convert a URL path (e.g. /static/img/...) to a base64 data URL via fetch + canvas
    function urlToBase64(url) {
        return new Promise(function(resolve, reject) {
            var img = new Image();
            img.crossOrigin = 'anonymous';
            img.onload = function() {
                var canvas = document.createElement('canvas');
                canvas.width = img.naturalWidth;
                canvas.height = img.naturalHeight;
                var ctx = canvas.getContext('2d');
                ctx.drawImage(img, 0, 0);
                resolve(canvas.toDataURL('image/jpeg', 0.92));
            };
            img.onerror = function() {
                reject(new Error('Failed to load image: ' + url));
            };
            img.src = url;
        });
    }

    // ─── TTS (Text-to-Speech) API Helper ──────────────────────────
    // ─── TTS / Lip-sync functions removed ──────────────────────────
    // AI video handles all audio natively (voiceover, dialogue, sound effects).
    // No separate TTS, D-ID lip-sync, or BGM generation needed.

    function generateShot() {
        if (isReadOnly) return;
        if (currentShotIndex < 0 || currentShotIndex >= shots.length) return;

        var shot = shots[currentShotIndex];

        // Save current prompt from textarea
        var promptEl = document.getElementById('vaiShotPrompt');
        if (promptEl) shot.prompt = promptEl.value.trim();

        if (!shot.prompt) {
            App.showAlert(I18n.t('vai.video.enterDescription'), 'warning');
            return;
        }

        // Update UI state
        shot.status = 'generating';
        shot.errorMsg = null;
        shot.videoUrl = null;
        shot.operationId = null;
        updateGenerateButtonState(shot);
        showProgress(true);
        renderShotStrip();

        var shotIdx = currentShotIndex;

        // SCENE SHOT: AI video generation (single shot mode)
        // Resolve refImage: if it's a URL path, convert to base64 first
        var imagePromise;
        if (shot.refImage && !shot.refImage.startsWith('data:')) {
            imagePromise = urlToBase64(shot.refImage);
        } else {
            imagePromise = Promise.resolve(shot.refImage || null);
        }

        imagePromise.then(function(imageData) {
            // Build single-shot request (backend wraps it as multi-shot for API)
            var reqBody = {
                prompt: shot.prompt,
                aspect_ratio: shot.aspectRatio || '9:16',
                duration: shot.duration || '10s'
            };

            if (imageData) {
                reqBody.image = imageData;
            }

            return App.apiRequest('/llm/video', {
                method: 'POST',
                body: JSON.stringify(reqBody)
            });
        }).then(function(resp) {
            if (resp.task_id) {
                shot.operationId = resp.task_id;
                shot.status = 'polling';
                renderShotStrip();
                scheduleSave();
                startPolling(shotIdx, resp.task_id);
            } else if (resp.done && resp.result) {
                handleVideoResult(shotIdx, resp);
            } else {
                shot.status = 'error';
                shot.errorMsg = resp.error || I18n.t('vai.video.unknownError');
                updateGenerateButtonState(shot);
                showProgress(false);
                renderShotStrip();
                scheduleSave();
                App.showAlert(I18n.t('vai.video.generationFailed') + shot.errorMsg, 'danger');
            }
        }).catch(function(err) {
            shot.status = 'error';
            shot.errorMsg = (err && err.message) || I18n.t('vai.video.requestFailed');
            updateGenerateButtonState(shot);
            showProgress(false);
            renderShotStrip();
            scheduleSave();
            App.showAlert(I18n.t('vai.video.generationFailed') + shot.errorMsg, 'danger');
        });
    }

    function regenerateShot() {
        if (isReadOnly) return;
        if (currentShotIndex < 0 || currentShotIndex >= shots.length) return;

        var shot = shots[currentShotIndex];

        // Stop existing polling if any
        if (shot.operationId && pollingTimers[shot.operationId]) {
            clearInterval(pollingTimers[shot.operationId]);
            delete pollingTimers[shot.operationId];
        }

        shot.videoUrl = null;
        shot.operationId = null;
        shot.status = 'draft';
        shot.errorMsg = null;
        updateVideoPreview(shot);
        generateShot();
    }

    // ─── Video Polling ──────────────────────────────────────────────
    function startPolling(shotIndex, taskId) {
        var pollCount = 0;
        var maxPolls = 60; // 15 min max (15s intervals)
        var consecutiveErrors = 0;

        var timer = setInterval(function() {
            pollCount++;

            if (pollCount > maxPolls) {
                clearInterval(timer);
                delete pollingTimers[taskId];
                if (shotIndex < shots.length) {
                    shots[shotIndex].status = 'error';
                    shots[shotIndex].errorMsg = I18n.t('vai.video.generationTimeout');
                    if (currentShotIndex === shotIndex) {
                        updateGenerateButtonState(shots[shotIndex]);
                        showProgress(false);
                    }
                    renderShotStrip();
                    scheduleSave();
                    _checkRecoveryComplete();
                }
                return;
            }

            // Update progress bar if viewing this shot
            if (currentShotIndex === shotIndex) {
                var pct = Math.min(95, (pollCount / maxPolls) * 100);
                var bar = document.getElementById('vaiGenProgressBar');
                if (bar) bar.style.width = pct + '%';
                var text = document.getElementById('vaiGenProgressText');
                if (text) text.textContent = I18n.t('vai.video.generatingProgress').replace('{seconds}', pollCount * 15);
            }

            // Poll the video API via backend
            App.apiRequest('/llm/video/' + taskId).then(function(resp) {
                consecutiveErrors = 0; // reset on success

                if (resp.done && resp.error) {
                    // Done with error
                    clearInterval(timer);
                    delete pollingTimers[taskId];
                    if (shotIndex < shots.length) {
                        shots[shotIndex].status = 'error';
                        var errMsg = extractErrorMessage(resp.error);
                        shots[shotIndex].errorMsg = errMsg;
                        if (currentShotIndex === shotIndex) {
                            updateGenerateButtonState(shots[shotIndex]);
                            showProgress(false);
                        }
                        renderShotStrip();
                        scheduleSave();
                        App.showAlert(I18n.t('vai.video.generationFailed') + errMsg, 'danger');
                        _checkRecoveryComplete();
                    }
                } else if (resp.done) {
                    clearInterval(timer);
                    delete pollingTimers[taskId];
                    handleVideoResult(shotIndex, resp);
                } else if (resp.error) {
                    clearInterval(timer);
                    delete pollingTimers[taskId];
                    if (shotIndex < shots.length) {
                        shots[shotIndex].status = 'error';
                        var errMsg = extractErrorMessage(resp.error);
                        shots[shotIndex].errorMsg = errMsg;
                        if (currentShotIndex === shotIndex) {
                            updateGenerateButtonState(shots[shotIndex]);
                            showProgress(false);
                        }
                        renderShotStrip();
                        scheduleSave();
                        App.showAlert(I18n.t('vai.video.generationFailed') + errMsg, 'danger');
                        _checkRecoveryComplete();
                    }
                }
                // else still processing, keep polling
            }).catch(function(err) {
                consecutiveErrors++;
                console.warn('[VaiVideo] Poll error (' + consecutiveErrors + '/3):', err);
                if (consecutiveErrors >= 3) {
                    clearInterval(timer);
                    delete pollingTimers[taskId];
                    if (shotIndex < shots.length) {
                        var errMsg = (err && err.message) || 'Poll failed';
                        shots[shotIndex].status = 'error';
                        shots[shotIndex].errorMsg = errMsg;
                        if (currentShotIndex === shotIndex) {
                            updateGenerateButtonState(shots[shotIndex]);
                            showProgress(false);
                        }
                        renderShotStrip();
                        scheduleSave();
                        App.showAlert(I18n.t('vai.video.generationFailed') + errMsg, 'danger');
                        _checkRecoveryComplete();
                    }
                }
            });

        }, 15000); // Recommended: 15 second intervals

        pollingTimers[taskId] = timer;
    }

    function handleVideoResult(shotIndex, resp) {
        if (shotIndex >= shots.length) return;
        var shot = shots[shotIndex];

        // Extract video URL from the response
        var videoUrl = null;

        console.log('[VaiVideo] handleVideoResult full resp:', JSON.stringify(resp).substring(0, 2000));

        if (resp.result && resp.result.video_url) {
            videoUrl = resp.result.video_url;
        }

        // Also check top-level video_url
        if (!videoUrl && resp.video_url) {
            videoUrl = resp.video_url;
        }

        if (videoUrl) {
            shot.status = 'done';
            shot.videoUrl = videoUrl;
            App.showAlert(I18n.t('vai.video.shotGenerated').replace('{num}', shotIndex + 1), 'success');
        } else {
            shot.status = 'done';
            shot.videoUrl = null;
            console.warn('[VaiVideo] Could not extract video URL from response', resp);
            App.showAlert(I18n.t('vai.video.shotDoneNoVideo').replace('{num}', shotIndex + 1), 'warning');
        }

        if (currentShotIndex === shotIndex) {
            updateVideoPreview(shot);
            updateGenerateButtonState(shot);
            showProgress(false);
        }
        renderShotStrip();
        scheduleSave();
        renderHistoryList();

        // Check if all recovery-polling shots are now complete
        _checkRecoveryComplete();
    }

    function resumePolling() {
        for (var i = 0; i < shots.length; i++) {
            var s = shots[i];
            if (s.status === 'polling' && s.operationId && !pollingTimers[s.operationId]) {
                startPolling(i, s.operationId);
            }
        }
    }

    // After a polling shot completes (success or error), check if all recovery-polling is done
    function _checkRecoveryComplete() {
        if (!_recoveryBubbleId) return;
        var stillPolling = shots.filter(function(s) { return s.status === 'polling'; });
        if (stillPolling.length > 0) return;

        batchGenerating = false;
        var el = document.getElementById(_recoveryBubbleId);
        if (el) {
            var content = el.querySelector('.vai-batch-progress');
            if (content) {
                var doneShots = shots.filter(function(s) { return s.status === 'done' && s.videoUrl; });
                var errorShots = shots.filter(function(s) { return s.status === 'error'; });
                var closeBtn = el.querySelector('.vai-progress-close-btn');
                if (closeBtn) closeBtn.style.display = 'none';
                if (doneShots.length > 0 && errorShots.length === 0) {
                    content.innerHTML =
                        '<div class="fw-semibold text-success"><i class="bi bi-check-circle-fill me-1"></i>' + (I18n.t('vai.video.allShotsComplete') || '影片已生成完成！') + '</div>' +
                        '<div class="mt-2">' +
                            '<button class="btn btn-primary btn-sm me-2" onclick="VaiVideo.combineAll()"><i class="bi bi-film me-1"></i>' + (I18n.t('vai.video.play') || '播放') + '</button>' +
                            '<button class="btn btn-outline-secondary btn-sm" onclick="VaiVideo.downloadComplete()"><i class="bi bi-download me-1"></i>' + (I18n.t('vai.common.download') || '下載') + '</button>' +
                        '</div>';
                    // Save success to chatHistory
                    chatHistory.push({ role: 'assistant', content: '[generation_success]', generation_result: { success: true, message: null } });
                    saveChatHistory();
                } else {
                    var errMsg = errorShots.length > 0 ? (errorShots[0].errorMsg || (I18n.t('vai.video.batchFailed') || 'Generation failed')) : (I18n.t('vai.video.batchFailed') || 'Generation failed');
                    content.innerHTML =
                        '<div class="fw-semibold text-danger"><i class="bi bi-exclamation-triangle-fill me-1"></i>' + escapeHtml(errMsg) + '</div>' +
                        '<div class="mt-1 small text-muted">' + (I18n.t('vai.video.tryAgainHint') || '你可以修改分鏡後重新生成') + '</div>';
                    // Save error to chatHistory
                    chatHistory.push({ role: 'assistant', content: '[generation_error]', generation_result: { success: false, message: errMsg } });
                    saveChatHistory();
                }
            }
            var cc = document.getElementById('vaiChatMessages');
            if (cc) cc.scrollTop = cc.scrollHeight;
        }
        _recoveryBubbleId = null;
    }

    function stopAllPolling() {
        var keys = Object.keys(pollingTimers);
        for (var i = 0; i < keys.length; i++) {
            clearInterval(pollingTimers[keys[i]]);
        }
        pollingTimers = {};
    }

    // ─── Extend Video ─────────────────────────────────────────────
    // Extend = capture last frame of source shot → create new shot with that frame as refImage → generate
    function extendShot() {
        if (isReadOnly) return;
        if (currentShotIndex < 0 || currentShotIndex >= shots.length) return;

        var sourceShot = shots[currentShotIndex];
        if (sourceShot.status !== 'done' || !sourceShot.videoUrl) {
            App.showAlert(I18n.t('vai.video.cannotExtend'), 'warning');
            return;
        }

        var extPrompt = prompt(
            I18n.t('vai.video.extendPromptBody'),
            sourceShot.prompt || ''
        );
        if (extPrompt === null || extPrompt.trim() === '') return;
        extPrompt = extPrompt.trim();

        var sourceShotIdx = currentShotIndex;

        // Show extend progress while capturing frame
        showExtendProgress(true);

        // Step 1: Capture last frame of source video
        captureVideoLastFrame(sourceShot.videoUrl, function(frameDataUrl) {
            showExtendProgress(false);

            if (!frameDataUrl) {
                App.showAlert(I18n.t('vai.video.captureFrameFailed') || 'Failed to capture frame', 'danger');
                return;
            }

            // Step 2: Create new shot with last frame as refImage
            var arEl = document.getElementById('vaiVideoAspectRatio');
            var durEl = document.getElementById('vaiVideoDuration');

            var newShot = {
                id: 'shot_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6),
                prompt: extPrompt,
                refImage: frameDataUrl,
                _prevShotRef: true,
                aspectRatio: sourceShot.aspectRatio || '9:16',
                duration: durEl ? durEl.value : '10s',
                status: 'draft',
                videoUrl: null,
                operationId: null,
                errorMsg: null
            };

            // Insert right after the source shot
            shots.splice(sourceShotIdx + 1, 0, newShot);
            var newShotIndex = sourceShotIdx + 1;
            renderShotStrip();
            selectShot(newShotIndex);
            scheduleSave();

            // Step 3: Auto-trigger generation (reuse generateShot which handles refImage → base64 → API)
            generateShot();
        });
    }

    function showExtendProgress(show) {
        var el = document.getElementById('vaiExtendProgress');
        if (!el) return;
        el.style.display = show ? 'block' : 'none';
        if (show) {
            var bar = document.getElementById('vaiExtendProgressBar');
            if (bar) bar.style.width = '5%';
            var text = document.getElementById('vaiExtendProgressText');
            if (text) text.textContent = I18n.t('vai.video.preparingExtend');
        }
    }

    // ─── UI Helpers ───────────────────────────────────────────────
    function updateVideoPreview(shot) {
        var previewEl = document.getElementById('vaiVideoPreview');
        var playerEl = document.getElementById('vaiPreviewPlayer');

        if (shot && shot.videoUrl) {
            if (previewEl) previewEl.style.display = 'block';
            if (playerEl) {
                playerEl.src = shot.videoUrl;
                playerEl.load();
            }
        } else {
            if (previewEl) previewEl.style.display = 'none';
            if (playerEl) playerEl.src = '';
        }
    }

    function updateGenerateButtonState(shot) {
        var btn = document.getElementById('vaiGenShotBtn');
        var spinner = document.getElementById('vaiGenSpinner');
        var statusEl = document.getElementById('vaiGenStatus');

        if (!btn) return;

        var isProcessing = shot.status === 'generating' || shot.status === 'polling';

        if (isProcessing) {
            btn.disabled = true;
            btn.innerHTML = '<span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.video.generating');
            if (spinner) spinner.style.display = '';
            if (statusEl) {
                statusEl.textContent = shot.status === 'polling' ? I18n.t('vai.video.waitingApi') : I18n.t('vai.video.submitting');
                statusEl.className = 'small text-muted mt-1';
            }
        } else if (shot.status === 'error') {
            btn.disabled = false;
            btn.innerHTML = '<i class="bi bi-arrow-clockwise me-1"></i>' + I18n.t('vai.video.regenerate');
            if (spinner) spinner.style.display = 'none';
            if (statusEl) {
                statusEl.textContent = shot.errorMsg || I18n.t('vai.video.generationFailedStatus');
                statusEl.className = 'small text-danger mt-1';
            }
        } else if (shot.status === 'done') {
            btn.disabled = false;
            btn.innerHTML = '<i class="bi bi-arrow-clockwise me-1"></i>' + I18n.t('vai.video.regenerate');
            if (spinner) spinner.style.display = 'none';
            if (statusEl) {
                statusEl.textContent = I18n.t('vai.video.completedStatus');
                statusEl.className = 'small text-success mt-1';
            }
        } else {
            btn.disabled = false;
            btn.innerHTML = '<i class="bi bi-stars me-1"></i>' + I18n.t('vai.video.aiGenerateBtn');
            if (spinner) spinner.style.display = 'none';
            if (statusEl) {
                statusEl.textContent = '';
                statusEl.className = 'small text-muted mt-1';
            }
        }

        // Hide generate button for read-only projects
        if (isReadOnly) {
            btn.style.display = 'none';
        } else {
            btn.style.display = '';
        }
    }

    function showProgress(show) {
        var el = document.getElementById('vaiGenProgress');
        if (!el) return;
        el.style.display = show ? 'block' : 'none';
        if (show) {
            var bar = document.getElementById('vaiGenProgressBar');
            if (bar) bar.style.width = '5%';
            var text = document.getElementById('vaiGenProgressText');
            if (text) text.textContent = I18n.t('vai.video.submitting');
        }
    }

    // ─── Combine All Shots ────────────────────────────────────────
    var lastCombinedBlobUrl = null; // Blob URL for combined video download

    function combineAll() {
        var completedShots = shots.filter(function(s) {
            return s.status === 'done' && s.videoUrl;
        });

        if (completedShots.length === 0) return;

        // Deduplicate by videoUrl — in multi-batch mode, multiple shots
        // within the same batch share the same videoUrl. We only want to
        // play/download each unique video once.
        var seen = {};
        var uniqueShots = [];
        for (var i = 0; i < completedShots.length; i++) {
            if (!seen[completedShots[i].videoUrl]) {
                seen[completedShots[i].videoUrl] = true;
                uniqueShots.push(completedShots[i]);
            }
        }

        // If only 1 unique video, still show it as the "combined" result
        if (uniqueShots.length === 1) {
            _showCombinedChatBubble(uniqueShots);
            return;
        }

        // Multiple unique videos — create Blob for download, then show chat bubble
        var fetches = uniqueShots.map(function(s) {
            return fetch(s.videoUrl).then(function(r) { return r.blob(); });
        });
        Promise.all(fetches).then(function(blobs) {
            var combined = new Blob(blobs, { type: 'video/mp4' });
            if (lastCombinedBlobUrl) URL.revokeObjectURL(lastCombinedBlobUrl);
            lastCombinedBlobUrl = URL.createObjectURL(combined);
            _showCombinedChatBubble(uniqueShots);
            // Save combined video to project history
            _saveCombinedToHistory(combined);
        }).catch(function(err) {
            console.warn('[VaiVideo] Failed to create combined blob:', err);
            // Still show sequential player even if blob fails
            _showCombinedChatBubble(uniqueShots);
        });
    }

    function _showCombinedChatBubble(completedShots) {
        var container = document.getElementById('vaiChatMessages');
        if (!container) return;

        // Remove any existing combined bubble
        var existing = document.getElementById('vaiCombinedBubble');
        if (existing) existing.remove();

        var wrapper = document.createElement('div');
        wrapper.className = 'd-flex justify-content-start mb-2';
        wrapper.id = 'vaiCombinedBubble';

        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
        var title = (currentStoryboard && currentStoryboard.title) || (I18n.t('vai.video.combinedResult') || '合併影片結果');
        var shotCount = completedShots.length;

        var html =
            '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                '<i class="bi bi-robot"></i>' +
            '</div>' +
            '<div class="message-bubble ai position-relative" style="max-width: 85%;">' +
                '<div class="ai-msg-content">' +
                    '<div class="fw-semibold mb-2"><i class="bi bi-film me-1"></i>' + escapeHtml(title) +
                        ' <span class="badge bg-success ms-1">' + shotCount + ' ' + (I18n.t('vai.video.shots') || '鏡頭') + '</span></div>' +
                    '<video id="vaiCombinedChatPlayer" controls playsinline style="width: 100%; max-height: 360px; border-radius: 8px; background: #000;"></video>' +
                    '<div class="d-flex gap-2 mt-2">' +
                        '<button class="btn btn-sm btn-outline-primary" onclick="VaiVideo.downloadComplete()">' +
                            '<i class="bi bi-download me-1"></i>' + (I18n.t('vai.video.downloadCompleteVideo') || '下載完整影片') +
                        '</button>' +
                        '<button class="btn btn-sm btn-outline-secondary" onclick="VaiVideo.replayCombined()">' +
                            '<i class="bi bi-arrow-counterclockwise me-1"></i>' + (I18n.t('vai.video.replay') || '重播') +
                        '</button>' +
                    '</div>' +
                '</div>' +
            '</div>';

        wrapper.innerHTML = html;
        container.appendChild(wrapper);
        container.scrollTop = container.scrollHeight;

        // Set up sequential playback in the chat bubble player
        var player = document.getElementById('vaiCombinedChatPlayer');
        if (!player) return;

        var currentIdx = 0;
        player.src = completedShots[0].videoUrl;
        player.load();

        player.onended = function() {
            currentIdx++;
            if (currentIdx < completedShots.length) {
                player.src = completedShots[currentIdx].videoUrl;
                player.load();
                player.play().catch(function() {});
            }
        };

        player.play().catch(function() {});

        // Persist playback event to chatHistory (avoid duplicates)
        var lastMsg = chatHistory.length > 0 ? chatHistory[chatHistory.length - 1] : null;
        if (!lastMsg || lastMsg.content !== '[playback]') {
            chatHistory.push({
                role: 'assistant',
                content: '[playback]',
                playback: {
                    title: title,
                    shot_count: shotCount
                }
            });
            saveChatHistory();
        }
    }

    function replayCombined() {
        var completedShots = shots.filter(function(s) {
            return s.status === 'done' && s.videoUrl;
        });
        if (completedShots.length === 0) return;

        var player = document.getElementById('vaiCombinedChatPlayer');
        if (!player) return;

        var currentIdx = 0;
        player.src = completedShots[0].videoUrl;
        player.load();
        player.onended = function() {
            currentIdx++;
            if (currentIdx < completedShots.length) {
                player.src = completedShots[currentIdx].videoUrl;
                player.load();
                player.play().catch(function() {});
            }
        };
        player.play().catch(function() {});
    }

    function _saveCombinedToHistory(blob) {
        if (!currentProjectId) return;
        // Mark project as "done" so it appears in generation history
        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/llm/video/history/' + currentProjectId, {
                method: 'PATCH',
                body: JSON.stringify({ status: 'done' })
            }).then(function(resp) {
                console.log('[VaiVideo] Project marked as done:', currentProjectId);
                // Refresh history panel if open
                if (videoGenHistoryOpen) loadVideoGenHistory(true);
            }).catch(function(err) {
                console.warn('[VaiVideo] Failed to mark project as done:', err);
            });
        }
    }

    function closeCombinedResult() {
        // Legacy advanced mode player
        var el = document.getElementById('vaiCombinedResult');
        if (el) el.style.display = 'none';

        var player = document.getElementById('vaiCombinedPlayer');
        if (player) {
            player.pause();
            player.onended = null;
            player.src = '';
        }

        // Chat mode combined player
        var chatPlayer = document.getElementById('vaiCombinedChatPlayer');
        if (chatPlayer) {
            chatPlayer.pause();
            chatPlayer.onended = null;
            chatPlayer.src = '';
        }
    }

    // ─── Download ─────────────────────────────────────────────────
    function downloadShot() {
        if (currentShotIndex < 0 || currentShotIndex >= shots.length) return;
        var shot = shots[currentShotIndex];
        if (!shot.videoUrl) {
            App.showAlert(I18n.t('vai.video.noVideoToDownload'), 'warning');
            return;
        }

        var filename = 'vai-video-shot-' + (currentShotIndex + 1) + '.mp4';
        downloadFile(shot.videoUrl, filename);
    }

    function downloadCombined() {
        var player = document.getElementById('vaiCombinedPlayer');
        if (!player || !player.src) {
            App.showAlert(I18n.t('vai.video.noCombinedToDownload'), 'warning');
            return;
        }
        downloadFile(player.src, 'vai-video-combined.mp4');
    }

    function downloadFile(url, filename) {
        // For data URLs and same-origin URLs, use anchor download
        if (url.startsWith('data:') || url.startsWith('/') || url.startsWith(window.location.origin)) {
            var a = document.createElement('a');
            a.href = url;
            a.download = filename;
            a.target = '_blank';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            return;
        }

        // For cross-origin URLs (GCS signed URLs), fetch as blob first
        var xhr = new XMLHttpRequest();
        xhr.open('GET', url, true);
        xhr.responseType = 'blob';
        xhr.onload = function() {
            if (xhr.status === 200) {
                var blobUrl = URL.createObjectURL(xhr.response);
                var a = document.createElement('a');
                a.href = blobUrl;
                a.download = filename;
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                setTimeout(function() { URL.revokeObjectURL(blobUrl); }, 1000);
            } else {
                // Fallback: open in new tab
                window.open(url, '_blank');
            }
        };
        xhr.onerror = function() {
            window.open(url, '_blank');
        };
        xhr.send();
    }

    // ─── Sidebar Toggle (mobile) ──────────────────────────────────
    function toggleSidebar() {
        var sidebar = document.getElementById('vaiVideoSidebar');
        if (sidebar) sidebar.classList.toggle('show');
    }

    // ─── Utility ──────────────────────────────────────────────────
    function escapeHtml(str) {
        if (!str) return '';
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function extractErrorMessage(error) {
        if (typeof error === 'string') return error;
        if (typeof error === 'object' && error !== null) {
            if (error.message) return error.message;
            if (error.error) return extractErrorMessage(error.error);
            try { return JSON.stringify(error); } catch (e) { return I18n.t('vai.video.unknownError'); }
        }
        return I18n.t('vai.video.unknownError');
    }

    function formatRelativeTime(dateStr) {
        if (!dateStr) return '';
        var date;
        try {
            date = new Date(dateStr);
        } catch (e) {
            return '';
        }

        var now = new Date();
        var diffMs = now.getTime() - date.getTime();
        if (diffMs < 0) diffMs = 0;

        var diffSec = Math.floor(diffMs / 1000);
        var diffMin = Math.floor(diffSec / 60);
        var diffHour = Math.floor(diffMin / 60);
        var diffDay = Math.floor(diffHour / 24);
        var diffWeek = Math.floor(diffDay / 7);
        var diffMonth = Math.floor(diffDay / 30);

        if (diffSec < 60) return I18n.t('vai.video.timeJustNow');
        if (diffMin < 60) return I18n.t('vai.video.timeMinutesAgo').replace('{n}', diffMin);
        if (diffHour < 24) return I18n.t('vai.video.timeHoursAgo').replace('{n}', diffHour);
        if (diffDay < 7) return I18n.t('vai.video.timeDaysAgo').replace('{n}', diffDay);
        if (diffWeek < 4) return I18n.t('vai.video.timeWeeksAgo').replace('{n}', diffWeek);
        if (diffMonth < 12) return I18n.t('vai.video.timeMonthsAgo').replace('{n}', diffMonth);

        // Fallback to date string
        var pad = function(n) { return n < 10 ? '0' + n : '' + n; };
        return date.getFullYear() + '/' + pad(date.getMonth() + 1) + '/' + pad(date.getDate());
    }

    // ─── Chat Mode: Attachments ─────────────────────────────────────

    var VIDEO_CHAT_ALLOWED_EXT = ['jpg', 'jpeg', 'png', 'pdf', 'txt', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'webm', 'wav', 'mp3', 'm4a', 'ogg'];
    var VIDEO_CHAT_MAX_FILE_SIZE = 20 * 1024 * 1024; // 20MB

    function getVideoChatFileIconClass(filename) {
        var ext = (filename.split('.').pop() || '').toLowerCase();
        if (['jpg', 'jpeg', 'png'].indexOf(ext) >= 0) return 'bi bi-file-image file-icon file-img';
        if (['webm', 'wav', 'mp3', 'm4a', 'ogg'].indexOf(ext) >= 0) return 'bi bi-file-earmark-music file-icon';
        if (ext === 'pdf') return 'bi bi-file-pdf file-icon file-pdf';
        if (['doc', 'docx'].indexOf(ext) >= 0) return 'bi bi-file-word file-icon file-doc';
        if (['xls', 'xlsx'].indexOf(ext) >= 0) return 'bi bi-file-excel file-icon file-xls';
        if (['ppt', 'pptx'].indexOf(ext) >= 0) return 'bi bi-file-ppt file-icon file-ppt';
        if (ext === 'txt') return 'bi bi-file-text file-icon file-txt';
        return 'bi bi-file-earmark file-icon';
    }

    function handleVideoChatFileSelect(event) {
        var input = event.target || document.getElementById('vaiVideoChatFileInput');
        var files = input ? input.files : null;
        if (!files || files.length === 0) return;

        for (var i = 0; i < files.length; i++) {
            var file = files[i];
            var ext = (file.name.split('.').pop() || '').toLowerCase();
            if (VIDEO_CHAT_ALLOWED_EXT.indexOf(ext) < 0) {
                App.showAlert((I18n.t('vai.chat.unsupportedFormat') || 'Unsupported format: {ext}').replace('{ext}', ext), 'warning');
                continue;
            }
            if (file.size > VIDEO_CHAT_MAX_FILE_SIZE) {
                App.showAlert((I18n.t('vai.chat.fileTooLarge') || 'File too large: {name}').replace('{name}', file.name), 'warning');
                continue;
            }
            videoChatPendingFiles.push({
                file: file,
                id: 'vf_' + Date.now() + '_' + Math.random().toString(36).substr(2, 6),
                uploaded: false,
                url: null,
                name: file.name
            });
        }

        // Reset file input so same file can be selected again
        if (event.target) event.target.value = '';

        // Auto-upload each file immediately
        uploadVideoChatPendingFiles();
        renderVideoChatFilePreview();
    }

    function uploadVideoChatPendingFiles() {
        for (var i = 0; i < videoChatPendingFiles.length; i++) {
            var pf = videoChatPendingFiles[i];
            if (pf.uploaded || pf._uploading) continue;
            pf._uploading = true;
            (function(pf) {
                var formData = new FormData();
                formData.append('file', pf.file);

                var token = localStorage.getItem('auth_token');
                var headers = {};
                if (token && token !== 'temp_token') {
                    headers['Authorization'] = 'Bearer ' + token;
                }
                var tenantSubdomain = localStorage.getItem('tenant_subdomain');
                if (tenantSubdomain) headers['X-Tenant-Subdomain'] = tenantSubdomain;

                fetch('/api/v1/ai/upload-file', {
                    method: 'POST',
                    headers: headers,
                    body: formData
                }).then(function(resp) {
                    if (!resp.ok) throw new Error('Upload failed (' + resp.status + ')');
                    return resp.json();
                }).then(function(data) {
                    pf.uploaded = true;
                    pf._uploading = false;
                    pf.url = data.file_url || null;
                    pf.mime_type = data.mime_type || pf.file.type;
                    pf.base64 = data.data || null; // base64 data for inline
                    renderVideoChatFilePreview();
                }).catch(function(err) {
                    pf._uploading = false;
                    pf._error = true;
                    console.error('[VaiVideo] File upload failed:', pf.name, err);
                    App.showAlert((I18n.t('vai.chat.uploadFailed') || 'Upload failed: {name}').replace('{name}', pf.name).replace('{error}', err.message), 'danger');
                    renderVideoChatFilePreview();
                });
            })(pf);
        }
    }

    function removeVideoChatPendingFile(fileId) {
        videoChatPendingFiles = videoChatPendingFiles.filter(function(f) { return f.id !== fileId; });
        renderVideoChatFilePreview();
    }

    function renderVideoChatFilePreview() {
        var previewArea = document.getElementById('vaiVideoChatFilePreview');
        if (!previewArea) return;

        if (videoChatPendingFiles.length === 0) {
            previewArea.style.display = 'none';
            previewArea.innerHTML = '';
            return;
        }

        previewArea.style.display = 'flex';
        var html = '';
        for (var i = 0; i < videoChatPendingFiles.length; i++) {
            var f = videoChatPendingFiles[i];
            var name = f.name.length > 25 ? f.name.substring(0, 22) + '...' : f.name;
            var statusClass = f.uploaded ? '' : (f._error ? ' upload-error' : ' uploading');
            var isImage = f._isUrlImage || /\.(jpg|jpeg|png|gif|webp)$/i.test(f.name);

            if (isImage && f.url) {
                // Show image thumbnail for image attachments
                html +=
                    '<div class="ai-file-preview-item vai-attach-thumb' + statusClass + '" title="' + escapeHtml(f.name) + '">' +
                        '<img src="' + escapeHtml(f.url) + '" alt="' + escapeHtml(name) + '" class="vai-attach-thumb-img">' +
                        '<span class="remove-file" onclick="VaiVideo.removeVideoChatPendingFile(\'' + f.id + '\')">&times;</span>' +
                    '</div>';
            } else {
                var iconClass = getVideoChatFileIconClass(f.name);
                html +=
                    '<div class="ai-file-preview-item' + statusClass + '" title="' + escapeHtml(f.name) + '">' +
                        '<i class="' + iconClass + '"></i>' +
                        '<span class="file-name">' + escapeHtml(name) + '</span>' +
                        (f._uploading ? '<span class="spinner-border spinner-border-sm ms-1" style="width:12px;height:12px;"></span>' : '') +
                        '<span class="remove-file" onclick="VaiVideo.removeVideoChatPendingFile(\'' + f.id + '\')">&times;</span>' +
                    '</div>';
            }
        }
        previewArea.innerHTML = html;
        updateVideoMicSendToggle();
    }

    // ─── Chat Mode: Core ──────────────────────────────────────────

    function sendChatMessage() {
        var inputEl = document.getElementById('vaiChatInput');
        if (!inputEl) return;
        var text = inputEl.value.trim();
        if (!text && videoChatPendingFiles.length === 0) return;
        if (chatGenerating) return;

        // Collect attachments (already uploaded files)
        var attachments = [];
        for (var i = 0; i < videoChatPendingFiles.length; i++) {
            var f = videoChatPendingFiles[i];
            if (f.uploaded && f.url) {
                attachments.push({
                    url: f.url,
                    name: f.name,
                    type: f.mime_type || (f.file ? f.file.type : 'application/octet-stream'),
                    base64: f.base64 || null  // base64 data if available (from upload API)
                });
            }
        }

        // Add user bubble with attachment cards (not just text)
        // Remove welcome message (with suggestions) if present
        var container = document.getElementById('vaiChatMessages');
        if (container) {
            var suggestionsEl = container.querySelector('.vai-chat-suggestions');
            if (suggestionsEl) {
                var welcomeBubble = suggestionsEl.closest('.d-flex.mb-2');
                if (welcomeBubble) welcomeBubble.remove();
            }
        }
        appendChatBubble('user', text, attachments);
        inputEl.value = '';
        inputEl.style.height = 'auto';
        videoChatPendingFiles = [];
        renderVideoChatFilePreview();
        updateVideoMicSendToggle();

        // Get settings from current shot
        var _ss = getCurrentShotSettings();
        var aspectRatio = _ss.aspectRatio;
        var duration = _ss.duration;

        // Show typing indicator
        var typingId = appendTypingIndicator();
        chatGenerating = true;
        updateVideoMicSendToggle();

        // Detect language from user input (multi-language detection)
        var lang = detectLanguage(text);
        console.log('[VaiVideo] sendChatMessage: detected lang=' + lang + ', text=' + text.substring(0, 40));

        // Add user message to conversation history
        var userMsg = { role: 'user', content: text };
        if (attachments.length > 0) {
            userMsg.attachments = attachments.map(function(a) {
                return { url: a.url, name: a.name, type: a.type };
            });
        }
        chatHistory.push(userMsg);

        // Ensure a project exists (auto-create if first message), then call API
        ensureProject(text.length > 3 ? text.substring(0, 40) : null).then(function() {
            // Save user message immediately
            saveChatHistory();

            // Build current shots summary for AI context (so it can modify or re-plan)
            var currentShotsSummary = null;
            if (shots.length > 0) {
                currentShotsSummary = shots.map(function(s, idx) {
                    return {
                        index: idx + 1,
                        prompt: s.prompt || '',
                        duration: s.duration || '10s',
                        status: s.status || 'draft',
                        has_video: !!(s.videoUrl),
                        has_ref_image: !!(s.refImage)
                    };
                });
            }

            // Call multi-turn chat API
            var chatPayload = {
                message: text,
                history: chatHistory.slice(0, -1),
                attachments: attachments.map(function(a) {
                    return {
                        mime_type: a.type,
                        data: a.base64 || '',       // base64 data (from upload API)
                        file_url: a.url || '',       // URL path (fallback for URL-based images)
                        filename: a.name
                    };
                }),
                aspect_ratio: aspectRatio,
                duration: duration,
                language: lang
            };
            // Include current shots + storyboard so AI can modify or re-plan
            if (currentShotsSummary) {
                chatPayload.current_shots = currentShotsSummary;
            }
            if (currentStoryboard) {
                chatPayload.current_storyboard = currentStoryboard;
            }
            return App.apiRequest('/llm/video/chat', {
                method: 'POST',
                body: JSON.stringify(chatPayload)
            });
        }).then(function(resp) {
            removeTypingIndicator(typingId);
            chatGenerating = false;
            updateVideoMicSendToggle();
            console.log('[VaiVideo] Chat API response:', JSON.stringify({type: resp.type, title: resp.storyboard ? resp.storyboard.title : null, content: resp.content ? resp.content.substring(0, 60) : null}));

            if (resp.type === 'storyboard' && resp.storyboard) {
                // AI has enough info → generated storyboard
                currentStoryboard = resp.storyboard;
                // Store reference images from chat attachments (for image-to-video)
                if (resp.reference_images && resp.reference_images.length > 0) {
                    projectReferenceImages = resp.reference_images;
                    console.log('[VaiVideo] Received ' + projectReferenceImages.length + ' reference images for Kling');
                }
                // Sync shots from storyboard: update existing or add new ones
                syncShotsFromStoryboard(resp.storyboard, aspectRatio, duration);
                // Auto-fill first shot's reference image if backend generated one
                if (resp.first_shot_ref_image && shots.length > 0) {
                    shots[0].refImage = resp.first_shot_ref_image;
                    console.log('[VaiVideo] Auto-filled first shot reference image from Gemini scene generation');
                }
                chatHistory.push({ role: 'assistant', content: '[storyboard generated]' });
                appendStoryboardBubble(resp.storyboard, aspectRatio, duration);
                saveChatHistory();
            } else if (resp.type === 'message' && resp.content) {
                // AI needs more info → follow-up question
                chatHistory.push({ role: 'assistant', content: resp.content });
                appendChatBubble('ai', resp.content);
                saveChatHistory();
            } else if (resp.storyboard) {
                // Fallback: old-style storyboard response
                currentStoryboard = resp.storyboard;
                if (resp.reference_images && resp.reference_images.length > 0) {
                    projectReferenceImages = resp.reference_images;
                }
                syncShotsFromStoryboard(resp.storyboard, aspectRatio, duration);
                // Auto-fill first shot's reference image if backend generated one
                if (resp.first_shot_ref_image && shots.length > 0) {
                    shots[0].refImage = resp.first_shot_ref_image;
                    console.log('[VaiVideo] Auto-filled first shot reference image from Gemini scene generation');
                }
                chatHistory.push({ role: 'assistant', content: '[storyboard generated]' });
                appendStoryboardBubble(resp.storyboard, aspectRatio, duration);
                saveChatHistory();
            } else if (resp.error) {
                appendChatBubble('ai', resp.error);
            }
        }).catch(function(err) {
            removeTypingIndicator(typingId);
            chatGenerating = false;
            updateVideoMicSendToggle();
            var msg = (err && err.message) || 'AI storyboard generation failed';
            appendChatBubble('ai', msg);
        });
    }

    // WhatsApp-style mic/send toggle: no text → show mic, hide send; has text → show send, hide mic
    function updateVideoMicSendToggle() {
        var micBtn = document.getElementById('vaiVideoMicBtn');
        var sendBtn = document.getElementById('vaiChatSendBtn');
        if (!micBtn || !sendBtn) return;
        var input = document.getElementById('vaiChatInput');
        var hasText = input && input.value.trim().length > 0;
        var hasFiles = videoChatPendingFiles && videoChatPendingFiles.length > 0;
        if (hasText || hasFiles || chatGenerating) {
            micBtn.style.display = 'none';
            sendBtn.style.display = '';
            sendBtn.disabled = chatGenerating;
            if (chatGenerating) {
                sendBtn.innerHTML = '<span class="spinner-border spinner-border-sm"></span>';
            } else {
                sendBtn.innerHTML = '<i class="bi bi-send"></i>';
            }
        } else {
            micBtn.style.display = '';
            sendBtn.style.display = 'none';
        }
    }

    // Voice input — delegate to ai-chat's toggleAIVoiceInput targeting vaiChatInput
    function toggleVoiceInput() {
        if (typeof toggleAIVoiceInput === 'function') {
            // ai-chat.js toggleAIVoiceInput uses #aiMessageInput by default
            // Use VaiSTT if available, else fallback
            if (typeof VaiSTT !== 'undefined' && typeof VaiSTT.toggle === 'function') {
                var btn = document.getElementById('vaiVideoMicBtn');
                VaiSTT.toggle('vaiChatInput', btn);
            }
        } else if (typeof VaiSTT !== 'undefined' && typeof VaiSTT.toggle === 'function') {
            var btn2 = document.getElementById('vaiVideoMicBtn');
            VaiSTT.toggle('vaiChatInput', btn2);
        }
    }

    function useSuggestion(btn) {
        var text = btn.textContent || btn.innerText;
        var inputEl = document.getElementById('vaiChatInput');
        if (inputEl && text) {
            inputEl.value = text.trim();
            inputEl.style.height = 'auto';
            inputEl.style.height = Math.min(inputEl.scrollHeight, 120) + 'px';
            inputEl.focus();
            updateVideoMicSendToggle();
            sendChatMessage();
        }
    }

    // ─── Chat Bubble / Storyboard Rendering ─────────────────────

    function appendChatBubble(role, text, attachments) {
        var container = document.getElementById('vaiChatMessages');
        if (!container) return;

        var isUser = (role === 'user');
        var wrapper = document.createElement('div');
        wrapper.className = 'd-flex ' + (isUser ? 'justify-content-end' : 'justify-content-start') + ' mb-2';

        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
        var avatarIcon = isUser ? 'bi-person' : 'bi-robot';
        var avatarHtml = '<div class="avatar-circle ' + (isUser ? 'ms-2' : 'me-2') + ' flex-shrink-0" style="' + avatarStyle + '"><i class="bi ' + avatarIcon + '"></i></div>';

        // Build attachment cards HTML (same as vai-chat's ai-msg-attachment-card)
        var attachHtml = '';
        if (attachments && attachments.length > 0) {
            attachHtml = '<div class="ai-msg-attachments">';
            for (var i = 0; i < attachments.length; i++) {
                var att = attachments[i];
                var fname = att.name || att.filename || 'file';
                var furl = att.url || att.file_url || '';
                var displayName = fname.length > 30 ? fname.substring(0, 27) + '...' : fname;
                var isImage = /\.(jpg|jpeg|png|gif|webp)$/i.test(fname);
                var iconClass = isImage ? 'bi bi-file-image file-icon file-img' : 'bi bi-file-earmark file-icon';
                if (isImage && furl) {
                    attachHtml += '<div class="ai-msg-attachment-card" title="' + escapeHtml(fname) + '" onclick="openAiGeneratedImage(\'' + escapeHtml(furl) + '\')" role="button">' +
                        '<i class="' + iconClass + '"></i>' +
                        '<span>' + escapeHtml(displayName) + '</span>' +
                    '</div>';
                } else if (furl) {
                    attachHtml += '<a href="' + escapeHtml(furl) + '" download="' + escapeHtml(fname) + '" class="ai-msg-attachment-card" title="' + escapeHtml(fname) + '" style="text-decoration:none;color:inherit;">' +
                        '<i class="' + iconClass + '"></i>' +
                        '<span>' + escapeHtml(displayName) + '</span>' +
                    '</a>';
                } else {
                    attachHtml += '<div class="ai-msg-attachment-card" title="' + escapeHtml(fname) + '">' +
                        '<i class="' + iconClass + '"></i>' +
                        '<span>' + escapeHtml(displayName) + '</span>' +
                    '</div>';
                }
            }
            attachHtml += '</div>';
        }

        var contentHtml = '';
        if (text) contentHtml += '<div class="ai-msg-content">' + escapeHtml(text) + '</div>';
        contentHtml += attachHtml;

        var bubbleHtml = '<div class="message-bubble ' + (isUser ? 'user' : 'ai') + ' position-relative" style="max-width: 70%;">' +
            contentHtml +
            '</div>';

        if (isUser) {
            wrapper.innerHTML = bubbleHtml + avatarHtml;
        } else {
            wrapper.innerHTML = avatarHtml + bubbleHtml;
        }
        container.appendChild(wrapper);
        container.scrollTop = container.scrollHeight;
    }

    // escapeHtml defined above (line ~1913), not duplicated here

    function appendTypingIndicator() {
        var container = document.getElementById('vaiChatMessages');
        if (!container) return null;

        var id = 'typing_' + Date.now();
        var wrapper = document.createElement('div');
        wrapper.className = 'ai-thinking-indicator d-flex justify-content-start mb-2';
        wrapper.id = id;

        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
        wrapper.innerHTML =
            '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                '<i class="bi bi-robot"></i>' +
            '</div>' +
            '<div class="message-bubble ai position-relative" style="max-width: 70%;">' +
                '<div class="d-flex align-items-center">' +
                    '<span class="thinking-dots">' +
                        '<span class="dot"></span><span class="dot"></span><span class="dot"></span>' +
                    '</span>' +
                    '<span class="ms-2" style="opacity: 0.7;">' + (I18n.t('vai.chat.aiThinking') || 'AI 正在思考...') + '</span>' +
                '</div>' +
            '</div>';
        container.appendChild(wrapper);
        container.scrollTop = container.scrollHeight;
        return id;
    }

    function removeTypingIndicator(id) {
        if (!id) return;
        var el = document.getElementById(id);
        if (el) el.remove();
    }

    function appendStoryboardBubble(storyboard, aspectRatio, duration) {
        var container = document.getElementById('vaiChatMessages');
        if (!container) return;

        var wrapper = document.createElement('div');
        wrapper.className = 'd-flex justify-content-start mb-2';

        // Build shots list HTML
        var shotsHtml = '';
        for (var i = 0; i < storyboard.shots.length; i++) {
            var s = storyboard.shots[i];

            shotsHtml +=
                '<div class="vai-storyboard-shot">' +
                    '<div class="vai-storyboard-shot-num">' + (i + 1) + '</div>' +
                    '<div class="vai-storyboard-shot-info">' +
                        '<div class="vai-storyboard-shot-desc">' + escapeHtml(s.description) + '</div>' +
                        '<div class="vai-storyboard-shot-meta">' +
                            '<span class="badge vai-shot-type-scene"><i class="bi bi-camera-reels me-1"></i>' + (I18n.t('vai.video.shotTypeScene') || 'Scene') + '</span>' +
                            '<span class="badge bg-secondary">' + escapeHtml(s.duration || duration) + '</span>' +
                        '</div>' +
                    '</div>' +
                '</div>';
        }

        var title = storyboard.title || (I18n.t('vai.video.storyboardTitle') || 'Storyboard');
        var summary = storyboard.summary || '';
        var computedTotalSec = _calcTotalDurationSec(storyboard);
        var totalDurLabel = computedTotalSec + 's';
        var totalDurClass = computedTotalSec > MAX_TOTAL_DURATION_SEC ? ' text-danger fw-bold' : '';

        // Character cards — show generated character reference images
        var charsHtml = '';
        if (storyboard.characters && storyboard.characters.length > 0) {
            var charCards = '';
            for (var ci = 0; ci < storyboard.characters.length; ci++) {
                var ch = storyboard.characters[ci];
                var imgHtml = ch.image_url
                    ? '<img src="' + escapeAttr(ch.image_url) + '" alt="' + escapeAttr(ch.name) + '" style="width:64px;height:64px;object-fit:cover;border-radius:8px;border:1px solid #dee2e6;">'
                    : '<div style="width:64px;height:64px;border-radius:8px;background:#e9ecef;display:flex;align-items:center;justify-content:center;"><i class="bi bi-person-fill text-muted" style="font-size:1.5rem;"></i></div>';
                charCards +=
                    '<div class="vai-character-card d-flex gap-2 p-2 rounded" style="background:#fafafa;border:1px solid #eee;min-width:200px;max-width:260px;">' +
                        imgHtml +
                        '<div style="flex:1;min-width:0;">' +
                            '<div class="fw-semibold small text-truncate">' + escapeHtml(ch.name || '') + '</div>' +
                            '<div class="text-muted" style="font-size:0.75rem;line-height:1.3;max-height:3.9em;overflow:hidden;">' + escapeHtml(ch.description || '') + '</div>' +
                        '</div>' +
                    '</div>';
            }
            charsHtml =
                '<div class="vai-storyboard-characters mt-2 mb-2">' +
                    '<div class="fw-semibold small mb-1"><i class="bi bi-people-fill me-1"></i>' + (I18n.t('vai.video.characters') || 'Characters') + '</div>' +
                    '<div class="d-flex flex-wrap gap-2">' + charCards + '</div>' +
                '</div>';
        }

        // BGM removed — Kling 3.0 handles audio natively

        var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
        wrapper.innerHTML =
            '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                '<i class="bi bi-robot"></i>' +
            '</div>' +
            '<div class="message-bubble ai position-relative vai-storyboard-card" style="max-width: 70%;">' +
                '<div class="vai-storyboard-header">' +
                    '<div class="vai-storyboard-title">' +
                        '<i class="bi bi-film me-2"></i>' + escapeHtml(title) +
                    '</div>' +
                    (summary ? '<div class="vai-storyboard-summary">' + escapeHtml(summary) + '</div>' : '') +
                    '<div class="vai-storyboard-meta text-muted small">' +
                        storyboard.shots.length + ' ' + (I18n.t('vai.video.shots') || 'shots') +
                        ' · <span class="' + totalDurClass + '">' + totalDurLabel + ' / ' + MAX_TOTAL_DURATION_SEC + 's</span>' +
                        ' · ' + escapeHtml(aspectRatio) +
                    '</div>' +
                '</div>' +
                charsHtml +
                '<div class="vai-storyboard-shots">' + shotsHtml + '</div>' +
                '<div class="vai-storyboard-actions">' +
                    '<button class="btn btn-primary btn-sm" onclick="VaiVideo.generateAllShots()">' +
                        '<i class="bi bi-play-fill me-1"></i>' + (I18n.t('vai.video.startGenerating') || '開始生成') +
                    '</button>' +
                    '<button class="btn btn-outline-secondary btn-sm" onclick="VaiVideo.regenerateStoryboard()">' +
                        '<i class="bi bi-arrow-clockwise me-1"></i>' + (I18n.t('vai.video.regenerateStoryboard') || '重新規劃') +
                    '</button>' +
                    '<button class="btn btn-outline-secondary btn-sm" onclick="VaiVideo.editStoryboard()">' +
                        '<i class="bi bi-pencil-square me-1"></i>' + (I18n.t('vai.video.editStoryboard') || '編輯規劃') +
                    '</button>' +
                '</div>' +
            '</div>';

        container.appendChild(wrapper);
        container.scrollTop = container.scrollHeight;
    }

    // Copy storyboard JSON to clipboard
    function copyStoryboard() {
        if (!currentStoryboard) return;
        var json = JSON.stringify(currentStoryboard, null, 2);
        navigator.clipboard.writeText(json).then(function() {
            App.showAlert(I18n.t('vai.video.storyboardCopied') || '規劃已複製到剪貼板', 'success');
        }).catch(function() {
            // Fallback: select a temporary textarea
            var ta = document.createElement('textarea');
            ta.value = json;
            document.body.appendChild(ta);
            ta.select();
            document.execCommand('copy');
            document.body.removeChild(ta);
            App.showAlert(I18n.t('vai.video.storyboardCopied') || '規劃已複製到剪貼板', 'success');
        });
    }

    // Open a modal to edit storyboard with user-friendly Card UI
    function editStoryboard() {
        if (!currentStoryboard) return;
        var sb = currentStoryboard;

        // Remove old modal if exists (recreate each time for fresh data)
        var oldModal = document.getElementById('vaiEditStoryboardModal');
        if (oldModal) {
            var oldBs = bootstrap.Modal.getInstance(oldModal);
            if (oldBs) oldBs.dispose();
            oldModal.remove();
        }

        var modalEl = document.createElement('div');
        modalEl.id = 'vaiEditStoryboardModal';
        modalEl.className = 'modal fade';
        modalEl.tabIndex = -1;

        // Build shot cards HTML
        var shotsCardsHtml = '';
        for (var i = 0; i < sb.shots.length; i++) {
            shotsCardsHtml += buildShotCardHtml(sb.shots[i], i);
        }

        // Calculate initial total duration for edit modal
        var editInitialTotal = _calcTotalDurationSec(sb);

        modalEl.innerHTML =
            '<div class="modal-dialog modal-lg modal-dialog-scrollable">' +
                '<div class="modal-content">' +
                    '<div class="modal-header">' +
                        '<h5 class="modal-title"><i class="bi bi-pencil-square me-2"></i>' + (I18n.t('vai.video.editStoryboard') || '編輯規劃') + '</h5>' +
                        '<button type="button" class="btn-close" data-bs-dismiss="modal"></button>' +
                    '</div>' +
                    '<div class="modal-body">' +
                        // Title + Summary editable at top
                        '<div class="mb-3">' +
                            '<label class="form-label fw-semibold small">' + (I18n.t('vai.video.editTitle') || '標題') + '</label>' +
                            '<input type="text" class="form-control form-control-sm" id="vaiEditSbTitle" value="' + escapeAttr(sb.title || '') + '">' +
                        '</div>' +
                        '<div class="mb-3">' +
                            '<label class="form-label fw-semibold small">' + (I18n.t('vai.video.editSummary') || '摘要') + '</label>' +
                            '<textarea class="form-control form-control-sm" id="vaiEditSbSummary" rows="2">' + escapeHtml(sb.summary || '') + '</textarea>' +
                        '</div>' +
                        // Character editing section
                        (sb.characters && sb.characters.length > 0 ? (function() {
                            var charEditHtml = '<div class="mb-3 p-2 rounded" style="background: #fff8f0; border: 1px solid #f0dcc7;">' +
                                '<label class="form-label fw-semibold small mb-2"><i class="bi bi-people-fill text-warning me-1"></i>' + (I18n.t('vai.video.characters') || 'Characters') + '</label>' +
                                '<div id="vaiEditCharCards">';
                            for (var ci = 0; ci < sb.characters.length; ci++) {
                                var ch = sb.characters[ci];
                                var charImgInner = ch.image_url
                                    ? '<img src="' + escapeAttr(ch.image_url) + '" style="width:48px;height:48px;object-fit:cover;border-radius:6px;">'
                                    : '<div style="width:48px;height:48px;border-radius:6px;background:#e9ecef;display:flex;align-items:center;justify-content:center;"><i class="bi bi-person-fill text-muted"></i></div>';
                                charEditHtml +=
                                    '<div class="vai-edit-char-card d-flex gap-2 mb-2 p-2 rounded" style="background:#fff;border:1px solid #eee;" data-char-idx="' + ci + '" data-image-url="' + escapeAttr(ch.image_url || '') + '">' +
                                        '<div class="vai-edit-char-img flex-shrink-0" data-char-idx="' + ci + '" ' +
                                            'style="cursor:pointer;position:relative;" ' +
                                            'title="' + (I18n.t('vai.video.clickToChangeImage') || '點擊更換圖片') + '">' +
                                            charImgInner +
                                            '<div style="position:absolute;bottom:-2px;right:-2px;width:18px;height:18px;border-radius:50%;background:#0d6efd;color:#fff;display:flex;align-items:center;justify-content:center;font-size:10px;border:2px solid #fff;">' +
                                                '<i class="bi bi-pencil-fill"></i>' +
                                            '</div>' +
                                        '</div>' +
                                        '<div style="flex:1;min-width:0;">' +
                                            '<input type="text" class="form-control form-control-sm vai-edit-char-name mb-1" placeholder="' + (I18n.t('vai.video.charName') || 'Name') + '" value="' + escapeAttr(ch.name || '') + '">' +
                                            '<textarea class="form-control form-control-sm vai-edit-char-desc" rows="2" placeholder="' + (I18n.t('vai.video.charDescription') || 'Detailed appearance description') + '">' + escapeHtml(ch.description || '') + '</textarea>' +
                                        '</div>' +
                                    '</div>';
                            }
                            charEditHtml += '</div></div>';
                            return charEditHtml;
                        })() : '') +
                        '<hr>' +
                        '<div class="d-flex align-items-center justify-content-between mb-2">' +
                            '<span class="fw-semibold small">' + (I18n.t('vai.video.editShots') || '分鏡列表') + ' (' + sb.shots.length + ')</span>' +
                            '<div class="d-flex align-items-center gap-2">' +
                                '<span class="small" id="vaiEditTotalDuration">' +
                                    (I18n.t('vai.video.totalDuration') || '總時長') + ': <strong id="vaiEditTotalDurValue" class="' + (editInitialTotal > MAX_TOTAL_DURATION_SEC ? 'text-danger' : 'text-success') + '">' + editInitialTotal + 's</strong> / ' + MAX_TOTAL_DURATION_SEC + 's' +
                                '</span>' +
                                '<button type="button" class="btn btn-sm btn-outline-primary" id="vaiEditAddShotBtn"><i class="bi bi-plus-lg me-1"></i>' + (I18n.t('vai.video.addShot') || '新增分鏡') + '</button>' +
                            '</div>' +
                        '</div>' +
                        '</div>' +
                        '<div id="vaiEditShotCards">' + shotsCardsHtml + '</div>' +
                        '<div id="vaiEditStoryboardError" class="text-danger small mt-2" style="display:none;"></div>' +
                    '</div>' +
                    '<div class="modal-footer">' +
                        '<button type="button" class="btn btn-secondary" data-bs-dismiss="modal">' + (I18n.t('vai.video.cancel') || '取消') + '</button>' +
                        '<button type="button" class="btn btn-primary" id="vaiEditStoryboardSaveBtn"><i class="bi bi-check-lg me-1"></i>' + (I18n.t('vai.video.save') || '儲存') + '</button>' +
                    '</div>' +
                '</div>' +
            '</div>';
        document.body.appendChild(modalEl);

        // Live total duration recalculation when any shot duration changes
        _recalcEditTotalDuration = function() {
            var cardsContainer = document.getElementById('vaiEditShotCards');
            if (!cardsContainer) return;
            var durationSels = cardsContainer.querySelectorAll('.vai-edit-shot-duration');
            var total = 0;
            for (var di = 0; di < durationSels.length; di++) {
                total += _parseDurationSec(durationSels[di].value);
            }
            var valEl = document.getElementById('vaiEditTotalDurValue');
            if (valEl) {
                valEl.textContent = total + 's';
                valEl.className = total > MAX_TOTAL_DURATION_SEC ? 'text-danger fw-bold' : 'text-success';
            }
            // Disable/enable save button
            var saveBtn = document.getElementById('vaiEditStoryboardSaveBtn');
            if (saveBtn) {
                saveBtn.disabled = total > MAX_TOTAL_DURATION_SEC;
                if (total > MAX_TOTAL_DURATION_SEC) {
                    saveBtn.title = (I18n.t('vai.video.durationExceeded') || '總時長超過 ' + MAX_TOTAL_DURATION_SEC + ' 秒');
                } else {
                    saveBtn.title = '';
                }
            }
            // Show/hide error
            var errorEl = document.getElementById('vaiEditStoryboardError');
            if (errorEl) {
                if (total > MAX_TOTAL_DURATION_SEC) {
                    errorEl.textContent = (I18n.t('vai.video.durationExceeded') || '總時長超過限制') + ' (' + total + 's > ' + MAX_TOTAL_DURATION_SEC + 's)';
                    errorEl.style.display = '';
                } else {
                    errorEl.style.display = 'none';
                }
            }
        }
        // Attach change listener via event delegation on the shot cards container
        var editShotCardsEl = document.getElementById('vaiEditShotCards');
        if (editShotCardsEl) {
            editShotCardsEl.addEventListener('change', function(e) {
                if (e.target && e.target.classList.contains('vai-edit-shot-duration')) {
                    _recalcEditTotalDuration();
                }
            });
        }

        // Wire up save button
        document.getElementById('vaiEditStoryboardSaveBtn').addEventListener('click', function() {
            var errorEl = document.getElementById('vaiEditStoryboardError');
            errorEl.style.display = 'none';
            try {
                var edited = collectEditedStoryboard();
                if (!edited.shots || edited.shots.length === 0) {
                    throw new Error(I18n.t('vai.video.editMinOneShot') || '至少需要一個分鏡');
                }
                // Validate total duration
                var editTotal = _calcTotalDurationSec(edited);
                if (editTotal > MAX_TOTAL_DURATION_SEC) {
                    throw new Error((I18n.t('vai.video.durationExceeded') || '總時長超過限制') + ' (' + editTotal + 's > ' + MAX_TOTAL_DURATION_SEC + 's)');
                }
                // Apply edited storyboard
                currentStoryboard = edited;
                // Sync shots from edited storyboard
                var ssSync = getCurrentShotSettings();
                syncShotsFromStoryboard(edited, ssSync.aspectRatio, ssSync.duration);
                // Re-render storyboard bubble
                var container = document.getElementById('vaiChatMessages');
                if (container) {
                    var oldCards = container.querySelectorAll('.vai-storyboard-card');
                    for (var k = 0; k < oldCards.length; k++) {
                        var parentBubble = oldCards[k].closest('.d-flex.mb-2');
                        if (parentBubble) parentBubble.remove();
                    }
                }
                var ssRender = getCurrentShotSettings();
                appendStoryboardBubble(currentStoryboard, ssRender.aspectRatio, ssRender.duration);
                saveChatHistory();
                var bsModal = bootstrap.Modal.getInstance(modalEl);
                if (bsModal) bsModal.hide();
                App.showAlert(I18n.t('vai.video.storyboardUpdated') || '規劃已更新', 'success');
            } catch (e) {
                errorEl.textContent = e.message;
                errorEl.style.display = '';
            }
        });

        // Wire up add shot button
        document.getElementById('vaiEditAddShotBtn').addEventListener('click', function() {
            var cardsContainer = document.getElementById('vaiEditShotCards');
            if (!cardsContainer) return;
            var newIndex = cardsContainer.querySelectorAll('.vai-edit-shot-card').length;
            var newShot = { type: 'scene', description: '', duration: '10s' };
            var tempDiv = document.createElement('div');
            tempDiv.innerHTML = buildShotCardHtml(newShot, newIndex);
            var newCard = tempDiv.firstChild;
            cardsContainer.appendChild(newCard);
            // Wire up delete for new card
            wireDeleteShotBtn(newCard);
            // Update count
            var countEl = modalEl.querySelector('.fw-semibold.small');
            if (countEl) {
                var total = cardsContainer.querySelectorAll('.vai-edit-shot-card').length;
                countEl.textContent = (I18n.t('vai.video.editShots') || '分鏡列表') + ' (' + total + ')';
            }
            _recalcEditTotalDuration();
            newCard.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        });

        // Wire up delete buttons for existing cards
        var cards = modalEl.querySelectorAll('.vai-edit-shot-card');
        for (var c = 0; c < cards.length; c++) {
            wireDeleteShotBtn(cards[c]);
        }

        // Wire up character image click handlers (open image picker in 'character' mode)
        var charImgs = modalEl.querySelectorAll('.vai-edit-char-img');
        for (var ci2 = 0; ci2 < charImgs.length; ci2++) {
            (function(imgEl) {
                imgEl.addEventListener('click', function() {
                    var charIdx = parseInt(imgEl.getAttribute('data-char-idx'), 10);
                    if (isNaN(charIdx)) return;
                    _editCharIdx = charIdx;
                    openImagePicker('character');
                });
            })(charImgs[ci2]);
        }

        var modal = new bootstrap.Modal(modalEl);
        modal.show();
    }

    function wireDeleteShotBtn(cardEl) {
        var delBtn = cardEl.querySelector('.vai-edit-shot-delete');
        if (delBtn) {
            delBtn.addEventListener('click', function() {
                var container = document.getElementById('vaiEditShotCards');
                cardEl.remove();
                // Renumber remaining cards
                if (container) {
                    var remaining = container.querySelectorAll('.vai-edit-shot-card');
                    for (var r = 0; r < remaining.length; r++) {
                        var numEl = remaining[r].querySelector('.vai-edit-shot-num');
                        if (numEl) numEl.textContent = (r + 1);
                        remaining[r].dataset.shotIndex = r;
                    }
                    // Update count
                    var modal = document.getElementById('vaiEditStoryboardModal');
                    if (modal) {
                        var countEl = modal.querySelector('.fw-semibold.small');
                        if (countEl) countEl.textContent = (I18n.t('vai.video.editShots') || '分鏡列表') + ' (' + remaining.length + ')';
                    }
                }
                _recalcEditTotalDuration();
            });
        }
    }

    function buildShotCardHtml(shot, index) {
        return '<div class="vai-edit-shot-card card mb-2" data-shot-index="' + index + '">' +
            '<div class="card-body p-2">' +
                '<div class="d-flex align-items-center justify-content-between mb-2">' +
                    '<div class="d-flex align-items-center gap-2">' +
                        '<span class="badge bg-dark vai-edit-shot-num" style="min-width: 24px;">' + (index + 1) + '</span>' +
                        '<span class="badge vai-shot-type-scene" style="font-size: 0.8rem;"><i class="bi bi-camera-reels me-1"></i>' + (I18n.t('vai.video.shotTypeScene') || 'Scene') + '</span>' +
                        '<select class="form-select form-select-sm vai-edit-shot-duration" style="width: auto; font-size: 0.8rem;">' +
                            '<option value="5s"' + (shot.duration === '5s' ? ' selected' : '') + '>5s</option>' +
                            '<option value="10s"' + ((shot.duration === '10s' || !shot.duration) ? ' selected' : '') + '>10s</option>' +
                            '<option value="15s"' + (shot.duration === '15s' ? ' selected' : '') + '>15s</option>' +
                        '</select>' +
                    '</div>' +
                    '<button type="button" class="btn btn-sm btn-link text-danger p-0 vai-edit-shot-delete" title="' + (I18n.t('vai.video.deleteShot') || '刪除分鏡') + '"><i class="bi bi-trash3"></i></button>' +
                '</div>' +
                '<div class="mb-2">' +
                    '<label class="form-label small text-muted mb-0">' + (I18n.t('vai.video.editDescription') || '描述') + '</label>' +
                    '<textarea class="form-control form-control-sm vai-edit-shot-desc" rows="2" style="font-size: 0.85rem;">' + escapeHtml(shot.description || '') + '</textarea>' +
                '</div>' +
            '</div>' +
        '</div>';
    }

    function escapeAttr(str) {
        return String(str).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    function collectEditedStoryboard() {
        var title = document.getElementById('vaiEditSbTitle');
        var summary = document.getElementById('vaiEditSbSummary');
        var cardsContainer = document.getElementById('vaiEditShotCards');
        var cards = cardsContainer ? cardsContainer.querySelectorAll('.vai-edit-shot-card') : [];

        var editedShots = [];
        for (var i = 0; i < cards.length; i++) {
            var card = cards[i];
            var duration = card.querySelector('.vai-edit-shot-duration');
            var desc = card.querySelector('.vai-edit-shot-desc');

            var shotObj = {
                type: 'scene',
                description: desc ? desc.value.trim() : '',
                duration: duration ? duration.value : '10s'
            };
            editedShots.push(shotObj);
        }

        var edited = {
            title: title ? title.value.trim() : (currentStoryboard.title || ''),
            summary: summary ? summary.value.trim() : (currentStoryboard.summary || ''),
            shots: editedShots
        };

        if (currentStoryboard.total_length) edited.total_length = currentStoryboard.total_length;

        // Collect edited character data
        var charContainer = document.getElementById('vaiEditCharCards');
        if (charContainer) {
            var charCards = charContainer.querySelectorAll('.vai-edit-char-card');
            var editedChars = [];
            for (var ci = 0; ci < charCards.length; ci++) {
                var cc = charCards[ci];
                var nameEl = cc.querySelector('.vai-edit-char-name');
                var descEl = cc.querySelector('.vai-edit-char-desc');
                var charObj = {
                    name: nameEl ? nameEl.value.trim() : '',
                    description: descEl ? descEl.value.trim() : ''
                };
                // Read image_url from data attribute (may have been updated by image picker)
                var dataImgUrl = cc.getAttribute('data-image-url');
                if (dataImgUrl) {
                    charObj.image_url = dataImgUrl;
                } else if (currentStoryboard.characters && currentStoryboard.characters[ci]) {
                    charObj.image_url = currentStoryboard.characters[ci].image_url || '';
                }
                editedChars.push(charObj);
            }
            if (editedChars.length > 0) edited.characters = editedChars;
        } else if (currentStoryboard.characters) {
            edited.characters = JSON.parse(JSON.stringify(currentStoryboard.characters));
        }

        return edited;
    }

    // ─── Background-safe save helpers ──────────────────────────────
    // Save shots to a specific project via direct API (not relying on global currentProjectId)
    function _bgSaveShots(projectId, taskShots) {
        if (!projectId || projectId === PENDING_PROJECT_ID) return;
        var savableShots = taskShots.map(function(s) {
            var persistStatus = s.status;
            if (persistStatus === 'error' || persistStatus === 'generating') persistStatus = 'draft';
            var obj = { id: s.id, prompt: s.prompt, aspectRatio: s.aspectRatio, duration: s.duration, status: persistStatus, videoUrl: s.videoUrl, operationId: s.operationId };
            if (s.refImage && !s.refImage.startsWith('data:')) obj.refImage = s.refImage;
            return obj;
        });
        App.apiRequest('/llm/video/history/' + projectId, {
            method: 'PATCH',
            body: JSON.stringify({ shots: savableShots })
        }).catch(function(err) { console.error('[VaiVideo] bgSaveShots failed:', err); });
    }

    // Save chat history to a specific project via direct API
    function _bgSaveChatHistory(projectId, taskChatHistory, taskStoryboard) {
        if (!projectId || projectId === PENDING_PROJECT_ID) return;
        var saveData = { chat_history: taskChatHistory };
        if (taskStoryboard) saveData.storyboard = taskStoryboard;
        App.apiRequest('/llm/video/history/' + projectId, {
            method: 'PATCH',
            body: JSON.stringify(saveData)
        }).catch(function(err) { console.error('[VaiVideo] bgSaveChatHistory failed:', err); });
    }

    // Check if user is currently viewing a given project
    function _isViewingProject(projectId) {
        return currentProjectId && String(currentProjectId) === String(projectId);
    }

    function generateAllShots() {
        if (!currentStoryboard || !currentStoryboard.shots || currentStoryboard.shots.length === 0) {
            App.showAlert(I18n.t('vai.video.noStoryboardYet') || 'No storyboard to generate', 'warning');
            return;
        }
        if (batchGenerating) return;

        // Validate total duration before generating
        var preTotalSec = _calcTotalDurationSec(currentStoryboard);
        if (preTotalSec > MAX_TOTAL_DURATION_SEC) {
            App.showAlert((I18n.t('vai.video.durationExceeded') || '總時長超過限制') + ' (' + preTotalSec + 's > ' + MAX_TOTAL_DURATION_SEC + 's)', 'warning');
            return;
        }

        batchGenerating = true;
        var sb = currentStoryboard;
        var _bss = getCurrentShotSettings();
        var aspectRatio = _bss.aspectRatio;
        var duration = _bss.duration;

        // ── Capture project context at invocation time ──
        // These captured vars form the closure for the background task.
        // They do NOT reference the global shots/chatHistory after this point.
        var capturedProjectId = currentProjectId;
        var capturedChatHistory = chatHistory.slice(); // shallow copy
        var capturedStoryboard = JSON.parse(JSON.stringify(sb));
        var capturedRefImages = projectReferenceImages.slice();

        // Show progress as a chat bubble (AI message)
        var progressId = 'batch_' + Date.now();
        var chatContainer = document.getElementById('vaiChatMessages');
        if (chatContainer) {
            var wrapper = document.createElement('div');
            wrapper.className = 'd-flex justify-content-start mb-2';
            wrapper.id = progressId;
            var avatarStyle = 'width: 32px; height: 32px; font-size: 0.9rem; border: 1px solid #dee2e6; background: transparent; color: #6c757d; border-radius: 50%; display: inline-flex; align-items: center; justify-content: center;';
            wrapper.innerHTML =
                '<div class="avatar-circle me-2 flex-shrink-0" style="' + avatarStyle + '">' +
                    '<i class="bi bi-robot"></i>' +
                '</div>' +
                '<div class="message-bubble ai position-relative" style="max-width: 70%;">' +
                    '<div class="vai-batch-progress">' +
                        '<div class="d-flex align-items-center justify-content-between mb-1">' +
                            '<span class="fw-semibold"><i class="bi bi-play-circle me-1"></i>' + (I18n.t('vai.video.generatingVideo') || '影片生成中...') + '</span>' +
                            '<button class="btn btn-sm btn-link text-muted p-0 ms-2 vai-progress-close-btn" onclick="VaiVideo.dismissProgress(\'' + progressId + '\')" title="' + (I18n.t('vai.common.close') || '關閉') + '" style="font-size: 0.85rem; line-height: 1;">&times;</button>' +
                        '</div>' +
                        '<div class="vai-batch-progress-bar"><div class="vai-batch-progress-fill" style="width: 0%"></div></div>' +
                        '<div class="vai-batch-progress-text small text-muted mt-1">' +
                            (I18n.t('vai.video.preparingProject') || '建立專案中...') +
                        '</div>' +
                    '</div>' +
                '</div>';
            chatContainer.appendChild(wrapper);
            chatContainer.scrollTop = chatContainer.scrollHeight;
        }

        // Register background task (will be fully populated after project ensure)
        var task = {
            projectId: capturedProjectId,
            shots: [],
            chatHistory: capturedChatHistory,
            storyboard: capturedStoryboard,
            progressId: progressId,
            pollTimer: null,
            taskId: null,
            status: 'running',
            result: null,
            lastPct: 0,
            lastStatusText: ''
        };

        function updateProgress(pct, statusText) {
            task.lastPct = pct;
            task.lastStatusText = statusText || '';
            // Only update DOM if user is viewing this project
            if (!_isViewingProject(task.projectId)) return;
            var el = document.getElementById(progressId);
            if (el) {
                var bar = el.querySelector('.vai-batch-progress-fill');
                var txt = el.querySelector('.vai-batch-progress-text');
                if (bar) bar.style.width = pct + '%';
                if (txt) txt.textContent = statusText || '';
            }
            var overlayFill = document.getElementById('vaiGenOverlayFill');
            var overlayText = document.getElementById('vaiGenOverlayText');
            if (overlayFill) overlayFill.style.width = pct + '%';
            if (overlayText) overlayText.textContent = statusText || '';
        }

        function finishProgress(success, msg) {
            task.status = success ? 'done' : 'error';
            task.result = { success: success, message: msg || null };

            // Clear poll timer
            if (task.pollTimer) { clearInterval(task.pollTimer); task.pollTimer = null; }

            // Persist generation result to task's chatHistory and save via direct API
            task.chatHistory.push({
                role: 'assistant',
                content: success ? '[generation_success]' : '[generation_error]',
                generation_result: { success: success, message: msg || null }
            });
            _bgSaveChatHistory(task.projectId, task.chatHistory, task.storyboard);
            _bgSaveShots(task.projectId, task.shots);

            // Update sidebar to remove spinner
            renderHistoryList();

            // If user is still viewing this project, update DOM directly
            if (_isViewingProject(task.projectId)) {
                // Sync global state from task
                shots = task.shots;
                chatHistory = task.chatHistory;
                batchGenerating = false;

                // Remove storyboard overlay
                var overlay = document.getElementById('vaiGenOverlay');
                if (overlay) overlay.remove();

                // Re-enable storyboard action buttons
                var storyboardCard = document.querySelector('.vai-storyboard-card');
                if (storyboardCard) {
                    var actionBtns = storyboardCard.querySelectorAll('.vai-storyboard-actions button');
                    for (var bi = 0; bi < actionBtns.length; bi++) {
                        actionBtns[bi].disabled = false;
                        actionBtns[bi].classList.remove('disabled');
                    }
                }

                var el = document.getElementById(progressId);
                if (el) {
                    var content = el.querySelector('.vai-batch-progress');
                    if (content) {
                        var closeBtn = el.querySelector('.vai-progress-close-btn');
                        if (closeBtn) closeBtn.style.display = 'none';
                        if (success) {
                            content.innerHTML =
                                '<div class="fw-semibold text-success"><i class="bi bi-check-circle-fill me-1"></i>' + (msg || (I18n.t('vai.video.allShotsComplete') || '影片已生成完成！')) + '</div>' +
                                '<div class="mt-2">' +
                                    '<button class="btn btn-primary btn-sm me-2" onclick="VaiVideo.combineAll()"><i class="bi bi-film me-1"></i>' + (I18n.t('vai.video.play') || '播放') + '</button>' +
                                    '<button class="btn btn-outline-secondary btn-sm" onclick="VaiVideo.downloadComplete()"><i class="bi bi-download me-1"></i>' + (I18n.t('vai.common.download') || '下載') + '</button>' +
                                '</div>';
                        } else {
                            content.innerHTML =
                                '<div class="fw-semibold text-danger"><i class="bi bi-exclamation-triangle-fill me-1"></i>' + escapeHtml(msg || (I18n.t('vai.video.batchFailed') || 'Generation failed')) + '</div>' +
                                '<div class="mt-1 small text-muted">' + (I18n.t('vai.video.tryAgainHint') || '你可以修改分鏡後重新生成') + '</div>';
                        }
                    }
                    var cc = document.getElementById('vaiChatMessages');
                    if (cc) cc.scrollTop = cc.scrollHeight;
                }

                // Clean up task from registry since user saw the result
                delete bgTasks[task.projectId];
            }
            // If user is NOT viewing this project, the task stays in bgTasks
            // and will be shown when they switch back (handled by selectProjectData)
        }

        // Step 1: Ensure we have a real project (reuse current or create new)
        var projectTitle = sb.title || ('vAi Video ' + new Date().toLocaleDateString());

        var ensureProjectForGeneration;
        if (currentProjectId && currentProjectId !== PENDING_PROJECT_ID) {
            ensureProjectForGeneration = Promise.resolve();
        } else {
            ensureProjectForGeneration = App.apiRequest('/llm/video/history', {
                method: 'POST',
                body: JSON.stringify({ title: projectTitle, shots: [], status: 'active' })
            }).then(function(resp) {
                var newProject = resp.data || resp;
                if (resp.id && !newProject.id) newProject.id = resp.id;

                projects.unshift(newProject);
                renderHistoryList();

                var savedChatHistory = chatHistory.slice();
                var savedStoryboard = currentStoryboard;

                selectProjectData(newProject);

                chatHistory = savedChatHistory;
                currentStoryboard = savedStoryboard;
                saveChatHistory();

                // Update captured projectId to the real one
                capturedProjectId = newProject.id;
                task.projectId = newProject.id;
            });
        }

        ensureProjectForGeneration.then(function() {
            // Register task in bgTasks registry now that we have a real projectId
            bgTasks[task.projectId] = task;
            // Step 2: Populate task.shots from storyboard
            task.shots = [];
            for (var i = 0; i < capturedStoryboard.shots.length; i++) {
                var sbShot = capturedStoryboard.shots[i];
                task.shots.push({
                    id: 'shot_' + Date.now() + '_' + i + '_' + Math.random().toString(36).substr(2, 4),
                    prompt: sbShot.description,
                    refImage: null,
                    aspectRatio: aspectRatio,
                    duration: sbShot.duration || duration,
                    status: 'draft',
                    videoUrl: null,
                    operationId: null,
                    errorMsg: null,
                    type: 'scene'
                });
            }
            // Sync global shots if user is still viewing this project
            if (_isViewingProject(task.projectId)) {
                shots = task.shots;
                renderShotStrip();
            }
            _bgSaveShots(task.projectId, task.shots);

            // Keep storyboard bubble visible — add generation overlay on top (only if viewing)
            if (_isViewingProject(task.projectId)) {
                var chatContainer2 = document.getElementById('vaiChatMessages');
                var storyboardCard = document.querySelector('.vai-storyboard-card');
                if (storyboardCard) {
                    var oldOverlay = storyboardCard.querySelector('.vai-gen-overlay');
                    if (oldOverlay) oldOverlay.remove();

                    var overlay = document.createElement('div');
                    overlay.className = 'vai-gen-overlay';
                    overlay.id = 'vaiGenOverlay';
                    overlay.innerHTML =
                        '<div class="vai-gen-overlay-content">' +
                            '<div class="spinner-border text-primary mb-2" role="status"></div>' +
                            '<div class="fw-semibold" id="vaiGenOverlayTitle">' + (I18n.t('vai.video.generatingVideo') || '影片生成中...') + '</div>' +
                            '<div class="vai-gen-overlay-progress mt-2">' +
                                '<div class="vai-gen-overlay-bar"><div class="vai-gen-overlay-fill" id="vaiGenOverlayFill" style="width:0%"></div></div>' +
                                '<div class="small text-muted mt-1" id="vaiGenOverlayText">' +
                                    (I18n.t('vai.video.sendingToAI') || '正在提交至 AI 生成...') +
                                '</div>' +
                            '</div>' +
                        '</div>';
                    storyboardCard.appendChild(overlay);

                    var actionBtns = storyboardCard.querySelectorAll('.vai-storyboard-actions button');
                    for (var bi = 0; bi < actionBtns.length; bi++) {
                        actionBtns[bi].disabled = true;
                        actionBtns[bi].classList.add('disabled');
                    }

                    if (chatContainer2) chatContainer2.scrollTop = chatContainer2.scrollHeight;
                }
            }

            // Step 3: Send multi-shot request to backend
            updateProgress(10, I18n.t('vai.video.sendingToAI') || '正在提交至 AI 生成...');

            var reqBody = {
                prompt: capturedStoryboard.shots.map(function(s) { return s.description; }).join(' | '),
                aspect_ratio: aspectRatio,
                duration: duration,
                shots: capturedStoryboard.shots.map(function(s, idx) {
                    return { index: idx + 1, prompt: s.description, duration: s.duration || duration };
                })
            };

            // Include character reference images
            if (capturedStoryboard.characters && capturedStoryboard.characters.length > 0) {
                var charRefImages = [];
                for (var ci = 0; ci < capturedStoryboard.characters.length; ci++) {
                    if (capturedStoryboard.characters[ci].image_url) {
                        charRefImages.push({ data: capturedStoryboard.characters[ci].image_url, mime_type: 'image/png' });
                    }
                }
                if (charRefImages.length > 0) {
                    reqBody.reference_images = charRefImages.slice(0, 7);
                    console.log('[VaiVideo] Sending ' + reqBody.reference_images.length + ' character reference images');
                }
            }

            // Include project reference images (captured at invocation time)
            if (capturedRefImages.length > 0) {
                if (!reqBody.reference_images) reqBody.reference_images = [];
                for (var pri = 0; pri < capturedRefImages.length && reqBody.reference_images.length < 7; pri++) {
                    var pRef = capturedRefImages[pri];
                    if (pRef.data || pRef.file_url) {
                        reqBody.reference_images.push({
                            data: pRef.data ? ('data:' + (pRef.mime_type || 'image/jpeg') + ';base64,' + pRef.data) : pRef.file_url,
                            mime_type: pRef.mime_type || 'image/jpeg'
                        });
                    }
                }
            }

            App.apiRequest('/llm/video', {
                method: 'POST',
                body: JSON.stringify(reqBody)
            }).then(function(resp) {
                // Backend returns single task: { task_id, total_shots, ... }
                if (!resp.task_id) {
                    var errMsg3 = resp.error || 'Unknown error — no task_id returned';
                    for (var fi2 = 0; fi2 < task.shots.length; fi2++) {
                        task.shots[fi2].status = 'error';
                        task.shots[fi2].errorMsg = errMsg3;
                    }
                    if (_isViewingProject(task.projectId)) { shots = task.shots; renderShotStrip(); }
                    finishProgress(false, errMsg3);
                    return;
                }

                console.log('[VaiVideo] Got task_id=' + resp.task_id + ', polling...');
                task.taskId = resp.task_id;

                // Mark all shots as polling with the single task_id
                for (var si = 0; si < task.shots.length; si++) {
                    task.shots[si].status = 'polling';
                    task.shots[si].operationId = resp.task_id;
                }
                if (_isViewingProject(task.projectId)) { shots = task.shots; renderShotStrip(); }
                _bgSaveShots(task.projectId, task.shots);
                updateProgress(20, I18n.t('vai.video.aiProcessing') || 'AI 影片生成中...');
                renderHistoryList(); // show sidebar spinner

                // Poll single task
                var pollCount2 = 0;
                var maxPolls2 = 60;
                var consecutiveErrors2 = 0;
                var lastStatus2 = '';
                var sameStatusCount2 = 0;
                var STUCK_THRESHOLD2 = 20;

                task.pollTimer = setInterval(function() {
                    pollCount2++;
                    if (pollCount2 > maxPolls2) {
                        clearInterval(task.pollTimer);
                        task.pollTimer = null;
                        for (var ti = 0; ti < task.shots.length; ti++) {
                            if (task.shots[ti]) { task.shots[ti].status = 'error'; task.shots[ti].errorMsg = 'Timeout'; }
                        }
                        if (_isViewingProject(task.projectId)) { shots = task.shots; renderShotStrip(); }
                        finishProgress(false, 'Timeout — AI generation took too long');
                        return;
                    }

                    // Update progress bar
                    var pct = Math.min(95, 20 + Math.round((pollCount2 / maxPolls2) * 75));
                    updateProgress(pct, (I18n.t('vai.video.aiProcessing') || 'AI 影片生成中...') + ' [' + pollCount2 + '/' + maxPolls2 + ']');

                    App.apiRequest('/llm/video/' + task.taskId, { method: 'GET' })
                        .then(function(pollResp) {
                            consecutiveErrors2 = 0;

                            var curStat = pollResp.status || 'unknown';
                            if (curStat === lastStatus2) { sameStatusCount2++; } else { sameStatusCount2 = 0; lastStatus2 = curStat; }
                            if (sameStatusCount2 >= STUCK_THRESHOLD2 && sameStatusCount2 % 10 === 0) {
                                console.warn('[VaiVideo] Poll stuck — status=' + curStat + ' for ' + sameStatusCount2 + ' polls');
                            }

                            if (pollResp.done) {
                                clearInterval(task.pollTimer);
                                task.pollTimer = null;
                                if (pollResp.result && pollResp.result.video_url) {
                                    var videoUrl = pollResp.result.video_url;
                                    for (var di3 = 0; di3 < task.shots.length; di3++) {
                                        if (task.shots[di3]) {
                                            task.shots[di3].videoUrl = videoUrl;
                                            task.shots[di3].status = 'done';
                                        }
                                    }
                                    if (_isViewingProject(task.projectId)) {
                                        shots = task.shots;
                                        if (currentShotIndex >= 0 && currentShotIndex < shots.length) {
                                            updateVideoPreview(shots[currentShotIndex]);
                                            updateGenerateButtonState(shots[currentShotIndex]);
                                        }
                                        renderShotStrip();
                                    }
                                    _bgSaveShots(task.projectId, task.shots);
                                    updateProgress(100, I18n.t('vai.video.allShotsComplete') || '影片已生成完成！');
                                    finishProgress(true);
                                } else {
                                    var errMsg4 = (pollResp.error && pollResp.error.message) || pollResp.error || 'Generation failed';
                                    for (var fi4 = 0; fi4 < task.shots.length; fi4++) {
                                        if (task.shots[fi4]) { task.shots[fi4].status = 'error'; task.shots[fi4].errorMsg = errMsg4; }
                                    }
                                    if (_isViewingProject(task.projectId)) { shots = task.shots; renderShotStrip(); }
                                    _bgSaveShots(task.projectId, task.shots);
                                    finishProgress(false, errMsg4);
                                }
                            }
                        }).catch(function(err) {
                            consecutiveErrors2++;
                            console.warn('[VaiVideo] Poll error (' + consecutiveErrors2 + '/3):', err);
                            if (consecutiveErrors2 >= 3) {
                                clearInterval(task.pollTimer);
                                task.pollTimer = null;
                                for (var ei3 = 0; ei3 < task.shots.length; ei3++) {
                                    if (task.shots[ei3]) { task.shots[ei3].status = 'error'; task.shots[ei3].errorMsg = (err && err.message) || 'Poll failed'; }
                                }
                                if (_isViewingProject(task.projectId)) { shots = task.shots; renderShotStrip(); }
                                finishProgress(false, (err && err.message) || 'Poll failed');
                            }
                        });
                }, 15000);

            }).catch(function(err) {
                var errMsg4 = (err && err.message) || 'Request failed';
                for (var fi3 = 0; fi3 < task.shots.length; fi3++) {
                    task.shots[fi3].status = 'error';
                    task.shots[fi3].errorMsg = errMsg4;
                }
                if (_isViewingProject(task.projectId)) { shots = task.shots; renderShotStrip(); }
                finishProgress(false, errMsg4);
            });

        }).catch(function(err) {
            batchGenerating = false;
            task.status = 'error';
            delete bgTasks[task.projectId];
            finishProgress(false, (err && err.message) || 'Failed to prepare project');
        });
    }

    function dismissProgress(id) {
        // Find the bgTask that owns this progress bubble
        var taskKey = null;
        var task = null;
        for (var k in bgTasks) {
            if (bgTasks[k].progressId === id) {
                taskKey = k;
                task = bgTasks[k];
                break;
            }
        }

        if (task && task.status === 'running') {
            // Stop poll timer
            if (task.pollTimer) {
                clearInterval(task.pollTimer);
                task.pollTimer = null;
            }

            // Mark task as cancelled
            task.status = 'cancelled';
            task.result = { success: false, message: 'Cancelled by user' };

            // Mark all non-done shots as draft (preserve any that already completed)
            for (var si = 0; si < task.shots.length; si++) {
                if (task.shots[si].status !== 'done') {
                    task.shots[si].status = 'draft';
                    task.shots[si].operationId = null;
                    task.shots[si].errorMsg = null;
                }
            }

            // Save cancelled state to DB
            _bgSaveShots(task.projectId, task.shots);
            task.chatHistory.push({
                role: 'assistant',
                content: '[generation_cancelled]',
                generation_result: { success: false, message: 'Cancelled by user' }
            });
            _bgSaveChatHistory(task.projectId, task.chatHistory, task.storyboard);

            // If user is viewing this project, sync global state
            if (_isViewingProject(task.projectId)) {
                shots = task.shots;
                chatHistory = task.chatHistory;
                batchGenerating = false;
                renderShotStrip();

                // Remove storyboard overlay
                var overlay = document.getElementById('vaiGenOverlay');
                if (overlay) overlay.remove();

                // Re-enable storyboard action buttons
                var storyboardCard = document.querySelector('.vai-storyboard-card');
                if (storyboardCard) {
                    var actionBtns = storyboardCard.querySelectorAll('.vai-storyboard-actions button');
                    for (var bi = 0; bi < actionBtns.length; bi++) {
                        actionBtns[bi].disabled = false;
                        actionBtns[bi].classList.remove('disabled');
                    }
                }
            }

            // Remove from bgTasks registry
            delete bgTasks[taskKey];

            // Update sidebar (remove spinner)
            renderHistoryList();

            console.log('[VaiVideo] Generation cancelled by user for project:', task.projectId);
        }

        // Always remove the progress bubble DOM element
        var el = document.getElementById(id);
        if (el) el.remove();
    }

    function regenerateStoryboard() {
        if (chatGenerating) return;

        // Find the last user message to re-send
        var container = document.getElementById('vaiChatMessages');
        if (!container) return;

        var userBubbles = container.querySelectorAll('.message-bubble.user .ai-msg-content');
        if (userBubbles.length === 0) {
            App.showAlert(I18n.t('vai.video.noDescriptionYet') || 'Please describe your video idea first', 'warning');
            return;
        }

        var lastUserText = userBubbles[userBubbles.length - 1].textContent.trim();
        if (!lastUserText) return;

        // Get settings
        var _rss = getCurrentShotSettings();
        var aspectRatio = _rss.aspectRatio;
        var duration = _rss.duration;

        // Show typing
        var typingId = appendTypingIndicator();
        chatGenerating = true;
        updateVideoMicSendToggle();

        var lang = detectLanguage(lastUserText);

        App.apiRequest('/llm/video/storyboard', {
            method: 'POST',
            body: JSON.stringify({
                description: lastUserText,
                aspect_ratio: aspectRatio,
                duration: duration,
                language: lang
            })
        }).then(function(resp) {
            removeTypingIndicator(typingId);
            chatGenerating = false;
            updateVideoMicSendToggle();

            if (resp.storyboard) {
                currentStoryboard = resp.storyboard;
                appendStoryboardBubble(resp.storyboard, aspectRatio, duration);
            } else if (resp.error) {
                appendChatBubble('ai', resp.error);
            }
        }).catch(function(err) {
            removeTypingIndicator(typingId);
            chatGenerating = false;
            updateVideoMicSendToggle();
            appendChatBubble('ai', (err && err.message) || 'Failed to regenerate storyboard');
        });
    }

    // Download all completed shots as a single concatenated video file
    // Uses the browser to sequentially fetch and combine video blobs
    function downloadComplete() {
        var filename = (currentStoryboard && currentStoryboard.title ? currentStoryboard.title : 'video') + '_complete.mp4';

        // If we already have a combined blob URL, use it directly
        if (lastCombinedBlobUrl) {
            var a = document.createElement('a');
            a.href = lastCombinedBlobUrl;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            return;
        }

        var completedShots = shots.filter(function(s) {
            return s.status === 'done' && s.videoUrl;
        });
        if (completedShots.length === 0) {
            App.showAlert(I18n.t('vai.video.noShotsToDownload') || 'No completed shots to download', 'warning');
            return;
        }
        // Deduplicate by videoUrl (multi-batch: shots in same batch share URL)
        var seen = {};
        var uniqueShots = [];
        for (var ui = 0; ui < completedShots.length; ui++) {
            if (!seen[completedShots[ui].videoUrl]) {
                seen[completedShots[ui].videoUrl] = true;
                uniqueShots.push(completedShots[ui]);
            }
        }
        if (uniqueShots.length === 1) {
            // Single video — just download it directly
            var a = document.createElement('a');
            a.href = uniqueShots[0].videoUrl;
            a.download = filename.replace('_complete', '');
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            return;
        }
        // Multiple unique videos — concatenate blobs for download
        App.showAlert(I18n.t('vai.video.downloadStarting') || 'Starting download...', 'info');
        var fetches = uniqueShots.map(function(s) {
            return fetch(s.videoUrl).then(function(r) { return r.blob(); });
        });
        Promise.all(fetches).then(function(blobs) {
            var combined = new Blob(blobs, { type: 'video/mp4' });
            var url = URL.createObjectURL(combined);
            lastCombinedBlobUrl = url;
            var a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
        }).catch(function(err) {
            App.showAlert((I18n.t('vai.video.downloadFailed') || 'Download failed') + ': ' + (err.message || err), 'danger');
        });
    }

    // ─── Generation History (combined videos only) ────────────────
    var videoGenHistoryOpen = false;
    var videoGenHistoryItems = [];
    var videoGenHistoryTotal = 0;
    var videoGenHistoryOffset = 0;
    var videoGenHistoryLoading = false;
    var videoGenHistoryHasMore = false;
    var VIDEO_GEN_HISTORY_LIMIT = 20;

    function toggleVideoGenHistory() {
        var panel = document.getElementById('vaiVideoGenHistory');
        if (!panel) return;
        videoGenHistoryOpen = !videoGenHistoryOpen;
        panel.style.display = videoGenHistoryOpen ? 'block' : 'none';
        var btn = document.getElementById('vaiVideoHistoryBtn');
        if (btn) btn.classList.toggle('active', videoGenHistoryOpen);
        if (videoGenHistoryOpen) {
            videoGenHistoryItems = [];
            videoGenHistoryOffset = 0;
            videoGenHistoryHasMore = false;
            loadVideoGenHistory(true);
        }
    }

    function loadVideoGenHistory(reset) {
        if (videoGenHistoryLoading) return;
        var container = document.getElementById('vaiVideoGenHistoryList');
        if (!container) return;

        // If no real project selected (pending or none), show empty state
        if (!currentProjectId || currentProjectId === PENDING_PROJECT_ID) {
            container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;">' +
                '<i class="bi bi-film d-block mb-1" style="font-size: 1.2rem;"></i>' +
                (I18n.t('vai.video.noGenHistory') || '尚無已完成的影片') + '</div>';
            videoGenHistoryItems = [];
            return;
        }

        if (reset) {
            videoGenHistoryItems = [];
            videoGenHistoryOffset = 0;
            container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;"><span class="spinner-border spinner-border-sm me-1"></span>' + (I18n.t('vai.common.loading') || '載入中...') + '</div>';
        }

        videoGenHistoryLoading = true;
        var page = Math.floor(videoGenHistoryOffset / VIDEO_GEN_HISTORY_LIMIT) + 1;

        if (typeof App !== 'undefined' && App.apiRequest) {
            var historyUrl = '/llm/video/history?limit=' + VIDEO_GEN_HISTORY_LIMIT + '&page=' + page;
            if (currentProjectId && currentProjectId !== PENDING_PROJECT_ID) {
                historyUrl += '&project_id=' + encodeURIComponent(currentProjectId);
            }
            App.apiRequest(historyUrl).then(function(resp) {
                var items = resp.data || [];
                videoGenHistoryTotal = resp.total || 0;

                // Filter: only show projects with status "done" (completed generation)
                var doneItems = items.filter(function(item) {
                    var ef = item.extra_fields || item.ExtraFields || {};
                    var vi = ef.video_info || {};
                    return vi.status === 'done';
                });

                videoGenHistoryHasMore = items.length >= VIDEO_GEN_HISTORY_LIMIT;
                videoGenHistoryOffset += items.length;

                if (reset) {
                    videoGenHistoryItems = doneItems;
                } else {
                    videoGenHistoryItems = videoGenHistoryItems.concat(doneItems);
                }
                renderVideoGenHistory();
                videoGenHistoryLoading = false;
            }).catch(function(err) {
                console.error('[VaiVideo] Failed to load generation history:', err);
                if (reset) {
                    container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;">' + (I18n.t('vai.common.loadFailed') || '載入失敗') + '</div>';
                }
                videoGenHistoryLoading = false;
            });
        } else {
            videoGenHistoryLoading = false;
        }
    }

    function renderVideoGenHistory() {
        var container = document.getElementById('vaiVideoGenHistoryList');
        if (!container) return;

        if (videoGenHistoryItems.length === 0) {
            container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;">' +
                '<i class="bi bi-film d-block mb-1" style="font-size: 1.2rem;"></i>' +
                (I18n.t('vai.video.noGenHistory') || '尚無已完成的影片') + '</div>';
            return;
        }

        var html = videoGenHistoryItems.map(function(item) {
            var ef = item.extra_fields || item.ExtraFields || {};
            var shotsArr = ef.shots || [];
            var storyboard = ef.storyboard || {};
            var title = item.subject || item.Subject || storyboard.title || '影片';
            var titleShort = title.length > 30 ? title.substring(0, 27) + '...' : title;
            var createdAt = item.created_at || item.CreatedAt || '';
            var timeStr = createdAt ? new Date(createdAt).toLocaleString('zh-TW', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '';
            var itemId = item.id || item.ID || '';
            var shotCount = shotsArr.length;

            // Find first shot with video URL for thumbnail
            var thumbHtml = '';
            var firstVideoShot = null;
            for (var si = 0; si < shotsArr.length; si++) {
                if (shotsArr[si].videoUrl || shotsArr[si].video_url) {
                    firstVideoShot = shotsArr[si];
                    break;
                }
            }
            if (firstVideoShot) {
                var videoSrc = firstVideoShot.videoUrl || firstVideoShot.video_url;
                thumbHtml = '<video src="' + escapeHtml(videoSrc) + '" muted preload="metadata" class="vai-gen-history-thumb" style="object-fit:cover;border-radius:4px;"></video>';
            } else {
                thumbHtml = '<div class="vai-gen-history-thumb-placeholder"><i class="bi bi-film"></i></div>';
            }

            return '<div class="vai-gen-history-item" data-gen-id="' + itemId + '">' +
                thumbHtml +
                '<div class="vai-gen-history-info">' +
                    '<div class="vai-gen-history-prompt" title="' + escapeHtml(title) + '">' + escapeHtml(titleShort) + '</div>' +
                    '<div class="vai-gen-history-time">' + shotCount + ' ' + (I18n.t('vai.video.shots') || '鏡頭') + ' &middot; ' + timeStr + '</div>' +
                '</div>' +
                '<div class="vai-gen-history-actions">' +
                    '<button class="btn btn-sm btn-link text-primary p-0" onclick="VaiVideo.playVideoFromHistory(\'' + itemId + '\')" title="' + (I18n.t('vai.video.play') || '播放') + '"><i class="bi bi-play-circle"></i></button>' +
                    '<button class="btn btn-sm btn-link text-secondary p-0 ms-1" onclick="VaiVideo.downloadVideoFromHistory(\'' + itemId + '\')" title="' + (I18n.t('vai.common.download') || '下載') + '"><i class="bi bi-download" style="font-size: 0.75rem;"></i></button>' +
                    '<button class="btn btn-sm btn-link text-danger p-0 ms-1" onclick="VaiVideo.deleteVideoFromHistory(\'' + itemId + '\')" title="' + (I18n.t('vai.common.delete') || '刪除') + '"><i class="bi bi-trash" style="font-size: 0.75rem;"></i></button>' +
                '</div>' +
            '</div>';
        }).join('');

        if (videoGenHistoryHasMore) {
            html += '<div class="text-center text-muted py-2 vai-gen-load-more" style="font-size: 0.75rem; cursor: pointer;" onclick="VaiVideo.loadMoreVideoHistory()">' +
                '<i class="bi bi-arrow-down-circle me-1"></i>' + (I18n.t('vai.common.loadMore') || '載入更多') + '</div>';
        }

        container.innerHTML = html;

        // Infinite scroll
        if (!container._scrollListenerAttached) {
            container.addEventListener('scroll', function() {
                if (videoGenHistoryLoading || !videoGenHistoryHasMore) return;
                if (container.scrollTop + container.clientHeight >= container.scrollHeight - 40) {
                    loadVideoGenHistory(false);
                }
            });
            container._scrollListenerAttached = true;
        }
    }

    function loadMoreVideoHistory() {
        if (!videoGenHistoryLoading && videoGenHistoryHasMore) {
            loadVideoGenHistory(false);
        }
    }

    function playVideoFromHistory(projectId) {
        // Find the project in history items
        var item = null;
        for (var i = 0; i < videoGenHistoryItems.length; i++) {
            var id = videoGenHistoryItems[i].id || videoGenHistoryItems[i].ID;
            if (id === projectId) { item = videoGenHistoryItems[i]; break; }
        }
        if (!item) return;

        var ef = item.extra_fields || item.ExtraFields || {};
        var shotsArr = ef.shots || [];
        var videoShots = shotsArr.filter(function(s) { return s.videoUrl || s.video_url; });

        if (videoShots.length === 0) {
            App.showAlert(I18n.t('vai.video.noShotsToPlay') || '沒有可播放的鏡頭', 'warning');
            return;
        }

        // Ensure project is in the projects list for selectProject to find
        var found = false;
        for (var pi = 0; pi < projects.length; pi++) {
            if (String(projects[pi].id) === String(projectId)) { found = true; break; }
        }
        if (!found) {
            projects.unshift(item);
            renderHistoryList();
        }

        // Select this project to load it, then play combined
        selectProject(projectId);
        // Close history panel
        toggleVideoGenHistory();
        // After a short delay to let project load, trigger combine
        setTimeout(function() { combineAll(); }, 500);
    }

    function downloadVideoFromHistory(projectId) {
        var item = null;
        for (var i = 0; i < videoGenHistoryItems.length; i++) {
            var id = videoGenHistoryItems[i].id || videoGenHistoryItems[i].ID;
            if (id === projectId) { item = videoGenHistoryItems[i]; break; }
        }
        if (!item) return;

        var ef = item.extra_fields || item.ExtraFields || {};
        var shotsArr = ef.shots || [];
        var videoShots = shotsArr.filter(function(s) { return s.videoUrl || s.video_url; });

        if (videoShots.length === 0) {
            App.showAlert(I18n.t('vai.video.noShotsToDownload') || '沒有可下載的鏡頭', 'warning');
            return;
        }

        var title = item.subject || item.Subject || '影片';
        var filename = title + '_complete.mp4';

        if (videoShots.length === 1) {
            var a = document.createElement('a');
            a.href = videoShots[0].videoUrl || videoShots[0].video_url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            return;
        }

        App.showAlert(I18n.t('vai.video.downloadStarting') || 'Starting download...', 'info');
        var fetches = videoShots.map(function(s) {
            return fetch(s.videoUrl || s.video_url).then(function(r) { return r.blob(); });
        });
        Promise.all(fetches).then(function(blobs) {
            var combined = new Blob(blobs, { type: 'video/mp4' });
            var url = URL.createObjectURL(combined);
            var a = document.createElement('a');
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            setTimeout(function() { URL.revokeObjectURL(url); }, 5000);
        }).catch(function(err) {
            App.showAlert((I18n.t('vai.video.downloadFailed') || 'Download failed') + ': ' + (err.message || err), 'danger');
        });
    }

    function deleteVideoFromHistory(projectId) {
        if (!confirm(I18n.t('vai.video.deleteProjectConfirm') || '確定要刪除此影片？')) return;

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/llm/video/history/' + projectId, { method: 'DELETE' }).then(function() {
                loadVideoGenHistory(true);
            }).catch(function(err) {
                console.error('[VaiVideo] Failed to delete video from history:', err);
                App.showAlert(I18n.t('vai.common.deleteFailed') || '刪除失敗', 'danger');
            });
        }
    }

    // ─── Public API ───────────────────────────────────────────────
    return {
        init: init,
        createProject: createProject,
        selectProject: selectProject,
        deleteProject: deleteProject,
        renameProject: renameProject,
        getCurrentProjectId: function() { return currentProjectId; },
        deleteChatVideo: deleteChatVideo,
        addShot: addShot,
        selectShot: selectShot,
        deleteShot: deleteShot,
        generateShot: generateShot,
        regenerateShot: regenerateShot,
        extendShot: extendShot,
        updateShotSetting: updateShotSetting,
        handleImageUpload: handleImageUpload,
        setRefImage: setRefImage,
        removeRefImage: removeRefImage,
        openImagePicker: openImagePicker,
        searchProductImages: searchProductImages,
        downloadShot: downloadShot,
        downloadCombined: downloadCombined,
        combineAll: combineAll,
        closeCombinedResult: closeCombinedResult,
        toggleSidebar: toggleSidebar,
        switchSidebarTab: switchSidebarTab,
        removePrevShotRef: removePrevShotRef,
        usePrevShotFrame: usePrevShotFrame,
        // Chat mode
        sendChatMessage: sendChatMessage,
        toggleVoiceInput: toggleVoiceInput,
        useSuggestion: useSuggestion,
        switchToAdvancedMode: switchToAdvancedMode,
        switchToChatMode: switchToChatMode,
        generateAllShots: generateAllShots,
        regenerateStoryboard: regenerateStoryboard,
        editStoryboard: editStoryboard,
        copyStoryboard: copyStoryboard,
        // Chat attachments
        handleVideoChatFileSelect: handleVideoChatFileSelect,
        removeVideoChatPendingFile: removeVideoChatPendingFile,
        onImagePickerSelect: onImagePickerSelect,
        addImageAsAttachment: addImageAsAttachment,
        downloadComplete: downloadComplete,
        replayCombined: replayCombined,
        // Generation history
        toggleVideoGenHistory: toggleVideoGenHistory,
        loadMoreVideoHistory: loadMoreVideoHistory,
        playVideoFromHistory: playVideoFromHistory,
        downloadVideoFromHistory: downloadVideoFromHistory,
        deleteVideoFromHistory: deleteVideoFromHistory,
        dismissProgress: dismissProgress,
        dismissRecovery: dismissRecovery
    };
})();
