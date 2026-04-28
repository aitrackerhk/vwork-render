// 圖片裁剪工具（避免重複載入造成 "Identifier 'ImageCropper' has already been declared"）
(function () {
    if (typeof window !== 'undefined' && window.ImageCropper) {
        return;
    }

class ImageCropper {
    constructor(options = {}) {
        // 檢測是否為移動設備（在構造函數中檢測，用於設置默認值）
        const isMobileDefault = typeof window !== 'undefined' && (
            window.innerWidth < 768 || 
            /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
        );
        
        this.options = {
            aspectRatio: options.aspectRatio || 1, // 默認正方形
            viewMode: options.viewMode || 1,
            dragMode: options.dragMode || 'move',
            autoCrop: true, // 強制啟用自動裁剪，確保 crop box 被創建
            autoCropArea: options.autoCropArea || 0.8,
            restore: false,
            guides: true,
            center: true,
            highlight: false,
            cropBoxMovable: true,
            cropBoxResizable: true,
            toggleable: true,
            minCropBoxWidth: 50,
            minCropBoxHeight: 50,
            // 移動端：默認禁用旋轉和縮放，避免 rotate 相關錯誤
            rotatable: options.rotatable !== undefined ? options.rotatable : !isMobileDefault,
            scalable: options.scalable !== undefined ? options.scalable : !isMobileDefault,
            zoomable: options.zoomable !== undefined ? options.zoomable : !isMobileDefault,
            ...options
        };
        this.cropper = null;
        this.modal = null;
        this._restoreFocusTo = null;
        this._resizeObserver = null;
        this._rebuildTimer = null;
        this._isRebuilding = false;
        this._isProcessing = false; // 防止重複處理
        this._currentCanvas = null; // 追蹤當前 canvas，用於清理
        this._currentBlob = null; // 追蹤當前 blob，用於清理
        this._confirmHandler = null; // 保存事件處理器引用，用於清理
        this._cropperReady = false; // Cropper ready 狀態
    }

    // 讓圖片(canvas)盡量吃滿 container 寬度，且 crop box(=view-box/face) 盡量大（不超出圖片）
    applyMaxLayout(imgContainer) {
        if (!this.cropper || !imgContainer) return;

        try {
        // 1) 先把圖片縮放到「不超出容器」(contain)，避免桌面端容器很寬但高度較小時，
        //    內部 img 被放大成很大（例如 766×766）造成看起來高度不一致
        const containerData = this.cropper.getContainerData();
        const imageData = this.cropper.getImageData();
        if (containerData && imageData && imageData.naturalWidth > 0) {
            const ratioW = containerData.width / imageData.naturalWidth;
            const ratioH = containerData.height / imageData.naturalHeight;
            const targetRatio = Math.min(ratioW, ratioH);

            if (targetRatio > 0 && typeof this.cropper.zoomTo === 'function') {
                this.cropper.zoomTo(targetRatio);
                } else if (imageData.ratio > 0 && targetRatio > 0 && typeof this.cropper.zoom === 'function') {
                // fallback：沒有 zoomTo 時用相對 zoom
                this.cropper.zoom(targetRatio / imageData.ratio - 1);
            }
        }

        // 2) 把 crop box 設成在 container 內、符合 aspectRatio 的最大矩形（=view-box/face 盡量大）
        const aspect = Number(this.options.aspectRatio) > 0 ? Number(this.options.aspectRatio) : 1;
        const cd = this.cropper.getContainerData();
            if (cd && cd.width > 0 && cd.height > 0 && typeof this.cropper.setCropBoxData === 'function') {
            let boxW = cd.width;
            let boxH = boxW / aspect;
            if (boxH > cd.height) {
                boxH = cd.height;
                boxW = boxH * aspect;
            }

            const left = Math.max(0, (cd.width - boxW) / 2);
            const top = Math.max(0, (cd.height - boxH) / 2);
            this.cropper.setCropBoxData({ left, top, width: boxW, height: boxH });
            }
        } catch (e) {
            console.warn('[Crop] Error in applyMaxLayout:', e);
            // 忽略錯誤，不影響主要功能
        }
    }

    // 顯示裁剪模態框
    // exportOptions:
    // - width/height: 指定輸出尺寸（像素）
    // - maxWidth: 只指定寬度，依 aspectRatio 推算高度
    // - mimeType: 預設 image/jpeg
    // - quality: 0~1，預設 0.9
    // - onCancel: 使用者取消/關閉時回調
    async showCropModal(file, callback, exportOptions = {}) {
        const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
        
        // 保存原始文件引用（移動端後端裁剪需要使用原始文件）
        const originalFile = file;
        // 保存原始和壓縮後的圖片尺寸（用於坐標轉換）
        let originalImageWidth = null;
        let originalImageHeight = null;
        let compressedImageWidth = null;
        let compressedImageHeight = null;
        
        // 移動端：每次裁剪前完全清理並重新初始化（防止內存累積）
        if (isMobile) {
            try {
                // 完全銷毀舊的實例
                if (this.cropper) {
                    this.cropper.destroy();
                    this.cropper = null;
                }
                // 清理所有資源
                if (this._currentCanvas) {
                    try {
                        const ctx = this._currentCanvas.getContext('2d');
                        if (ctx) {
                            ctx.clearRect(0, 0, this._currentCanvas.width, this._currentCanvas.height);
                        }
                        this._currentCanvas.width = 0;
                        this._currentCanvas.height = 0;
                    } catch (e) {
                        console.warn('[Crop] Error clearing canvas before new crop:', e);
                    }
                    this._currentCanvas = null;
                }
                if (this._currentBlob) {
                    try {
                        const url = URL.createObjectURL(this._currentBlob);
                        URL.revokeObjectURL(url);
                    } catch (e) {
                        console.warn('[Crop] Error revoking blob before new crop:', e);
                    }
                    this._currentBlob = null;
                }
                
                // 強制清理所有 Canvas（包括 Cropper 創建的預覽 Canvas）
                const allCanvases = document.querySelectorAll('canvas');
                allCanvases.forEach(canvas => {
                    try {
                        const ctx = canvas.getContext('2d');
                        if (ctx) {
                            ctx.clearRect(0, 0, canvas.width, canvas.height);
                        }
                        canvas.width = 0;
                        canvas.height = 0;
                    } catch (e) {
                        // ignore
                    }
                });
                
                // 強制垃圾回收提示（如果支持）
                if (window.gc && typeof window.gc === 'function') {
                    try {
                        window.gc();
                    } catch (e) {
                        // ignore
                    }
                }
                
                // 等待一段時間讓瀏覽器釋放內存（關鍵：給瀏覽器時間處理）
                await new Promise(resolve => setTimeout(resolve, 200));
            } catch (e) {
                console.warn('[Crop] Error cleaning up before new crop:', e);
            }
        }
        
        // 重置處理標誌（防止上次操作未完成）
        this._isProcessing = false;
        
        // 創建模態框
        const modalId = 'imageCropModal';
        const onCancel = exportOptions && typeof exportOptions.onCancel === 'function' ? exportOptions.onCancel : null;
        let confirmed = false;

        // 記錄開啟前焦點，關閉後還原（同時避免 aria-hidden 隱藏焦點警告）
        this._restoreFocusTo = (document.activeElement && document.activeElement instanceof HTMLElement)
            ? document.activeElement
            : null;

        const safeFocusBody = () => {
            const body = document.body;
            if (!body) return;
            const prevTabIndex = body.getAttribute('tabindex');
            if (prevTabIndex === null) body.setAttribute('tabindex', '-1');
            try { body.focus({ preventScroll: true }); } catch (_) { /* ignore */ }
            if (prevTabIndex === null) body.removeAttribute('tabindex');
        };

        let modal = document.getElementById(modalId);
        if (modal) {
            // 移動端：完全移除舊 modal（防止內存累積）
            if (isMobile) {
                try {
                    // 先銷毀 Cropper 實例
                    if (this.cropper) {
                        this.cropper.destroy();
                        this.cropper = null;
                    }
                    // 清理所有資源
                const oldImg = modal.querySelector('#cropImage');
                if (oldImg && oldImg.src && oldImg.src.startsWith('blob:')) {
                    URL.revokeObjectURL(oldImg.src);
                }
                    // 清理所有 canvas
                    const canvases = modal.querySelectorAll('canvas');
                    canvases.forEach(canvas => {
                        try {
                            const ctx = canvas.getContext('2d');
                            if (ctx) {
                                ctx.clearRect(0, 0, canvas.width, canvas.height);
                            }
                            canvas.width = 0;
                            canvas.height = 0;
                        } catch (e) {
                            // ignore
                        }
                    });
                    // 清理 Modal 實例
                    const inst = bootstrap.Modal.getInstance(modal);
                    if (inst) {
                        inst.hide();
                        inst.dispose();
                    }
                } catch (e) {
                    console.warn('[Crop] Error cleaning up old modal:', e);
                }
                // 完全移除 DOM
                try {
                    modal.remove();
                } catch (e) {
                    console.warn('[Crop] Error removing old modal:', e);
                }
                // 移動端：不等待，直接繼續（避免阻塞 UI）
            } else {
                // 桌面端：只清理資源，保留 modal DOM
                try {
                    const oldImg = modal.querySelector('#cropImage');
                    if (oldImg) {
                        if (oldImg.src && oldImg.src.startsWith('blob:')) {
                            URL.revokeObjectURL(oldImg.src);
                        }
                        oldImg.onload = null;
                        oldImg.onerror = null;
                        oldImg.src = '';
                    }
                    const canvases = modal.querySelectorAll('canvas');
                    canvases.forEach(canvas => {
                        try {
                            const ctx = canvas.getContext('2d');
                            if (ctx) {
                                ctx.clearRect(0, 0, canvas.width, canvas.height);
                            }
                            canvas.width = 0;
                            canvas.height = 0;
                        } catch (e) {
                            // ignore
                        }
                    });
            } catch (e) {
                console.warn('[Crop] Error cleaning up old modal image:', e);
            }
            
            try {
                const active = document.activeElement;
                if (active && modal.contains(active) && typeof active.blur === 'function') active.blur();
            } catch (_) { /* ignore */ }
            
            try {
                if (this.cropper) {
                    this.cropper.destroy();
                    this.cropper = null;
                }
            } catch (e) {
                console.warn('[Crop] Error destroying old cropper:', e);
            }
            
                if (this._currentCanvas) {
                    try {
                        const ctx = this._currentCanvas.getContext('2d');
                        if (ctx) {
                            ctx.clearRect(0, 0, this._currentCanvas.width, this._currentCanvas.height);
                        }
                        this._currentCanvas.width = 0;
                        this._currentCanvas.height = 0;
                    } catch (e) {
                        console.warn('[Crop] Error clearing canvas in showCropModal:', e);
                    }
                    this._currentCanvas = null;
                }
                
                if (this._currentBlob) {
                    try {
                        const url = URL.createObjectURL(this._currentBlob);
                        URL.revokeObjectURL(url);
                    } catch (e) {
                        console.warn('[Crop] Error revoking blob in showCropModal:', e);
                    }
                    this._currentBlob = null;
                }
                
                try {
                    const inst = bootstrap.Modal.getInstance(modal);
                if (inst) inst.dispose();
            } catch (_) { /* ignore */ }
            modal.remove();
            }
        }

        modal = document.createElement('div');
        modal.className = 'modal fade';
        modal.id = modalId;
        modal.setAttribute('tabindex', '-1');
        // 設置 z-index，確保在匹配結果 modal 之下（匹配結果 modal 是 1060）
        modal.style.zIndex = '1055';
        modal.innerHTML = `
            <div class="modal-dialog modal-lg modal-dialog-centered modal-fullscreen-sm-down">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">選擇範圍</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                    </div>
                    <div class="modal-body" style="padding: 1rem;">
                        <div class="d-flex flex-column align-items-center">
                            <div id="cropImageContainer" class="mb-3" style="width: 100%; max-width: 800px; max-height: 70vh; overflow: hidden; display: block; position: relative;">
                                <img id="cropImage" src="" alt="選擇範圍" style="max-width: 100%; max-height: 100%; width: 100%; height: auto; display: block;">
                            </div>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-outline-info" id="removeBgBtn" style="display: none;">
                            <i class="bi bi-magic"></i> 一鍵去背景
                        </button>
                        <div class="ms-auto">
                        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
                        <button type="button" class="btn btn-primary" id="cropConfirmBtn">確認選擇</button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(modal);
        this.modal = new bootstrap.Modal(modal);

        // 確保 Cropper 已載入
        if (typeof Cropper === 'undefined') {
            console.error('Cropper.js is not loaded');
            App.showAlert('圖片裁剪庫未載入，請刷新頁面重試', 'warning');
            safeFocusBody();
            try { this.destroy(); } catch (_) { /* ignore */ }
            try { modal.remove(); } catch (_) { /* ignore */ }
            return;
        }

        // 等 modal 真正顯示完 + 圖片載入完，再 init（避免拿到 0 寬高造成 wrap-box 很小）
        // 注意：原本在 FileReader.onload 內才註冊 shown.bs.modal，手機較慢時可能會「漏接」shown 事件，導致永遠不 init。
        let modalShown = false;
        let imageLoaded = false;
        let tryInit = () => {};

        const markModalShown = () => {
            modalShown = true;
            tryInit();
        };
        modal.addEventListener('shown.bs.modal', markModalShown, { once: true });

        // 若因時序/瀏覽器差異導致 shown 事件沒觸發（或已觸發），用 class 兜底
        const ensureModalShown = () => {
            if (modal.classList && modal.classList.contains('show')) {
                modalShown = true;
            }
        };

        // 關閉前若焦點仍在 modal 內，先移出；完全關閉後再 destroy/dispose/remove
        // 移動端：使用 MutationObserver 監控 body 樣式變化，強制恢復滾動（在 modal 創建後立即開始觀察）
        let bodyObserver = null;
        if (isMobile) {
            bodyObserver = new MutationObserver(() => {
                // 如果 modal 已關閉但 body 仍有 modal-open，強制清理
                if (!modal.classList.contains('show') && document.body.classList.contains('modal-open')) {
                    document.body.classList.remove('modal-open');
                    document.body.style.paddingRight = '';
                    document.body.style.overflow = '';
                    document.body.style.position = '';
                    document.body.style.height = '';
                    if (document.documentElement) {
                        document.documentElement.style.overflow = '';
                        document.documentElement.classList.remove('modal-open');
                        document.documentElement.style.height = '';
                    }
                }
            });
            bodyObserver.observe(document.body, {
                attributes: true,
                attributeFilter: ['class', 'style']
            });
        }
        
        // 確保清理 modal-backdrop 和 body 樣式的函數
        const cleanupBody = () => {
            const backdrops = document.querySelectorAll('.modal-backdrop');
            backdrops.forEach(backdrop => {
                // 只移除屬於這個 modal 的 backdrop
                if (backdrop.getAttribute('data-bs-modal') === modalId || 
                    (backdrops.length > 0 && !document.querySelector('.modal.show'))) {
                    try {
                        backdrop.remove();
                    } catch (e) {
                        console.warn('[Crop] Error removing backdrop:', e);
                    }
                }
            });
            
            // 強制清理 body 和 html 樣式（移動端特別重要）
            document.body.classList.remove('modal-open');
            document.body.style.paddingRight = '';
            document.body.style.overflow = '';
            document.body.style.position = '';
            document.body.style.height = '';
            
            if (document.documentElement) {
                document.documentElement.style.overflow = '';
                document.documentElement.classList.remove('modal-open');
                document.documentElement.style.height = '';
            }
        };
        
        modal.addEventListener('hide.bs.modal', () => {
            try {
                const active = document.activeElement;
                if (active && modal.contains(active)) safeFocusBody();
            } catch (_) { /* ignore */ }
            // 移動端：在 hide 時就開始清理
            if (isMobile) {
                cleanupBody();
            }
        });
        modal.addEventListener('hidden.bs.modal', () => {
            // 停止觀察
            if (bodyObserver) {
                bodyObserver.disconnect();
                bodyObserver = null;
            }
            const el = this._restoreFocusTo;
            this._restoreFocusTo = null;
            if (el && document.contains(el) && typeof el.focus === 'function') {
                try { el.focus({ preventScroll: true }); } catch (_) { el.focus(); }
            } else {
                safeFocusBody();
            }
            // 若未確認裁剪即關閉，視為取消
            try {
                if (!confirmed && onCancel) onCancel();
            } catch (_) { /* ignore */ }
            
            // 清理圖片資源（釋放 blob URL 和圖片引用）
            try {
                const img = document.getElementById('cropImage');
                if (img) {
                    // 如果是 blob URL，釋放它
                    if (img.src && img.src.startsWith('blob:')) {
                        try {
                            URL.revokeObjectURL(img.src);
                        } catch (e) {
                            console.warn('[Crop] Failed to revoke image blob URL:', e);
                        }
                    }
                    // 清理圖片引用
                    img.src = '';
                    img.onload = null;
                    img.onerror = null;
                }
            } catch (e) {
                console.warn('[Crop] Error cleaning up image:', e);
            }
            
            // 清理 FileReader
            try {
                if (reader) {
                    reader.onload = null;
                    reader.onerror = null;
                    reader.onabort = null;
                }
            } catch (e) {
                console.warn('[Crop] Error cleaning up FileReader:', e);
            }
            
            // 多次清理確保完全恢復
            cleanupBody();
            setTimeout(cleanupBody, 50);
            setTimeout(cleanupBody, 150);
            setTimeout(cleanupBody, 300);
            
            // 清理 canvas 和 blob
            if (this._currentCanvas) {
                try {
                    const ctx = this._currentCanvas.getContext('2d');
                    if (ctx) {
                        ctx.clearRect(0, 0, this._currentCanvas.width, this._currentCanvas.height);
                    }
                        } catch (e) {
                    console.warn('[Crop] Error clearing canvas on modal close:', e);
                }
                this._currentCanvas = null;
            }
            
            if (this._currentBlob) {
                try {
                    URL.revokeObjectURL(URL.createObjectURL(this._currentBlob));
                } catch (e) {
                    console.warn('[Crop] Error revoking blob on modal close:', e);
                }
                this._currentBlob = null;
            }
            
            // 清理事件監聽器
            const cropConfirmBtn = document.getElementById('cropConfirmBtn');
            if (cropConfirmBtn && this._confirmHandler) {
                try {
                    cropConfirmBtn.removeEventListener('click', this._confirmHandler);
                    cropConfirmBtn.removeEventListener('touchend', this._confirmHandler);
                } catch (e) {
                    console.warn('[Crop] Error removing event listeners on modal close:', e);
                }
                this._confirmHandler = null;
            }
            
            // 重置處理標誌
            this._isProcessing = false;
            
            try { this.destroy(); } catch (_) { /* ignore */ }
            try { modal.remove(); } catch (_) { /* ignore */ }
        }, { once: true });

        // 圖片預壓縮函數（手機端使用）
        const compressImageIfNeeded = (file, callback) => {
            const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
            
            // 桌面端：不壓縮
            if (!isMobile) {
                callback(file);
                return;
            }
            
            // 移動端：所有圖片都壓縮到 1500px（無論文件大小，減少內存壓力）
            console.log('[Crop] Compressing image before cropping...', { originalSize: file.size });
            
            const img = new Image();
            let objectUrl = null;
            let canvas = null;
            
            const cleanup = () => {
                // 釋放對象 URL
                if (objectUrl) {
                    try {
                        URL.revokeObjectURL(objectUrl);
                    } catch (e) {
                        console.warn('[Crop] Failed to revoke object URL:', e);
                    }
                    objectUrl = null;
                }
                // 清理圖片引用
                if (img) {
                    img.onload = null;
                    img.onerror = null;
                    img.src = '';
                }
                // 清理 canvas
                if (canvas) {
                    const ctx = canvas.getContext('2d');
                    if (ctx) {
                        ctx.clearRect(0, 0, canvas.width, canvas.height);
                    }
                    canvas.width = 0;
                    canvas.height = 0;
                    canvas = null;
                }
            };
            
            img.onload = () => {
                try {
                    // 保存原始圖片尺寸（在壓縮前）
                    originalImageWidth = img.naturalWidth || img.width;
                    originalImageHeight = img.naturalHeight || img.height;
                    
                    // 移動端：強制壓縮到最大 1500px（與 getCroppedCanvas 前的限制一致，減少內存使用）
                    const maxDimension = 1500;
                    let newWidth = originalImageWidth;
                    let newHeight = originalImageHeight;
                    
                    if (newWidth > maxDimension || newHeight > maxDimension) {
                        if (newWidth > newHeight) {
                            newWidth = maxDimension;
                            newHeight = Math.round((originalImageHeight * maxDimension) / originalImageWidth);
                        } else {
                            newHeight = maxDimension;
                            newWidth = Math.round((originalImageWidth * maxDimension) / originalImageHeight);
                        }
                    }
                    
                    // 保存壓縮後的尺寸
                    compressedImageWidth = newWidth;
                    compressedImageHeight = newHeight;
                    
                    // 創建 canvas 進行壓縮
                    canvas = document.createElement('canvas');
                    canvas.width = newWidth;
                    canvas.height = newHeight;
                    const ctx = canvas.getContext('2d');
                    
                    ctx.drawImage(img, 0, 0, newWidth, newHeight);
                    
                    // 轉換為 blob（移動端使用合理的品質）
                    const compressionQuality = 0.85;
                    canvas.toBlob((blob) => {
                        // 立即清理資源
                        cleanup();
                        
                        if (blob) {
                            console.log('[Crop] Image compressed', { 
                                originalSize: file.size, 
                                compressedSize: blob.size,
                                originalDimensions: `${originalImageWidth}x${originalImageHeight}`,
                                compressedDimensions: `${newWidth}x${newHeight}`,
                                reduction: Math.round((1 - blob.size / file.size) * 100) + '%'
                            });
                            // 創建新的 File 對象
                            const compressedFile = new File([blob], file.name, { type: 'image/jpeg' });
                            callback(compressedFile);
                        } else {
                            console.warn('[Crop] Compression failed, using original');
                            callback(file);
                        }
                    }, 'image/jpeg', compressionQuality);
                } catch (error) {
                    console.error('[Crop] Compression error:', error);
                    cleanup();
                    callback(file);
                }
            };
            
            img.onerror = () => {
                console.warn('[Crop] Failed to load image for compression, using original');
                cleanup();
                callback(file);
            };
            
            objectUrl = URL.createObjectURL(file);
            img.src = objectUrl;
        };
        
        // 讀取文件並顯示
        const reader = new FileReader();
        
        reader.onload = (e) => {
                const img = document.getElementById('cropImage');
                if (!img) {
                    console.error('cropImage element not found');
                    App.showAlert('圖片元素未找到，請刷新頁面重試', 'error');
                    safeFocusBody();
                    this.modal.hide();
                    return;
                }
            
            // 圖片載入狀態（用事件 + raf + decode 多重兜底，避免漏接 load）
            const markImageLoaded = () => { imageLoaded = true; };

            const initCropper = () => {
                if (this.cropper) {
                    try {
                    this.cropper.destroy();
                    } catch (e) {
                        console.warn('[Crop] Error destroying old cropper:', e);
                    }
                    this.cropper = null;
                }
                // 確保圖片容器有足夠的大小（移動端響應式處理）
                const imgContainer = img.parentElement;
                if (!imgContainer) {
                    console.error('[Crop] Invalid image container');
                    return;
                }
                // 確保圖片已完全加載（關鍵：避免 rotate 錯誤）
                if (!img.complete || img.naturalWidth === 0 || img.naturalHeight === 0) {
                    console.warn('[Crop] Image not fully loaded, waiting...', {
                        complete: img.complete,
                        naturalWidth: img.naturalWidth,
                        naturalHeight: img.naturalHeight
                    });
                    // 等待圖片加載完成
                    const checkImage = () => {
                        if (img.complete && img.naturalWidth > 0 && img.naturalHeight > 0) {
                            initCropper();
                        } else {
                            setTimeout(checkImage, 100);
                        }
                    };
                    setTimeout(checkImage, 100);
                    return;
                }
                
                // 獲取 modal-body 的實際可用空間
                const modalBody = imgContainer.closest('.modal-body');
                const isMobile = window.innerWidth < 768;
                let availableWidth = 800; // 默認桌面端寬度
                let availableHeight = 600; // 默認桌面端高度
                const viewportTargetHeight = window.innerHeight * (isMobile ? 0.65 : 0.75);
                
                if (modalBody) {
                    // Desktop：先把 modal-content 高度鎖定到穩定的 viewport 高度，避免高度被內容回饋影響造成「算出來偏小」
                    if (!isMobile) {
                        const modalContent = modal.querySelector('.modal-content');
                        if (modalContent) {
                            const targetContentH = Math.floor(window.innerHeight * 0.9);
                            modalContent.style.height = targetContentH + 'px';
                            modalContent.style.maxHeight = targetContentH + 'px';
                        }
                        // 避免 modal-body 自己滾動導致布局跳動
                        modalBody.style.overflow = 'hidden';
                    }

                    // 使用 modal-body 的實際尺寸，減去 padding
                    const bodyRect = modalBody.getBoundingClientRect();
                    const bodyStyle = window.getComputedStyle(modalBody);
                    const paddingLeft = parseFloat(bodyStyle.paddingLeft) || 0;
                    const paddingRight = parseFloat(bodyStyle.paddingRight) || 0;
                    const paddingTop = parseFloat(bodyStyle.paddingTop) || 0;
                    const paddingBottom = parseFloat(bodyStyle.paddingBottom) || 0;
                    
                    availableWidth = bodyRect.width - paddingLeft - paddingRight - 20; // 額外留出 20px 邊距

                    if (isMobile) {
                        availableHeight = viewportTargetHeight;
                    } else {
                        // desktop：以「鎖定後的 modal-body 真實高度」為準扣出可用高度，避免循環依賴內容高度
                        // 留 16px 額外邊距避免裁剪框貼邊
                        const innerBodyH = (bodyRect.height - paddingTop - paddingBottom - 16);
                        availableHeight = Math.min(innerBodyH > 0 ? innerBodyH : viewportTargetHeight, viewportTargetHeight);
                    }
                } else {
                    // 如果找不到 modal-body，使用窗口尺寸
                    availableWidth = isMobile ? window.innerWidth - 40 : 800;
                    availableHeight = viewportTargetHeight;
                }
                
                const minDisplaySize = isMobile ? 250 : 300; // 最小顯示尺寸，避免 cropper-wrap-box 太小
                // 重要：把可用高度做 clamp，避免 modal-body 高度過小/計算為 0 導致容器太矮，出現「圖片高度大過 cropper」的視覺問題
                const maxAllowedHeight = Math.floor((isMobile ? 0.65 : 0.75) * window.innerHeight);
                const safeAvailableHeight = Math.floor(Number(availableHeight) || 0);
                availableHeight = Math.max(
                    minDisplaySize,
                    Math.min(safeAvailableHeight || minDisplaySize, maxAllowedHeight || minDisplaySize)
                );
                
                // 設置容器大小：寬度保持 100%（讓 cropper-container width 也維持 100%），高度依照可用高度 + aspectRatio 算出「塞得下」的高度
                // 避免桌面端出現 container 寬很大、但高被限制，導致 cropper 內部 img 被算得很大（如 766×766）而 container 只有 421px 高
                const aspectRatio = (Number(this.options.aspectRatio) > 0) ? Number(this.options.aspectRatio) : 1;
                const targetH = Math.max(minDisplaySize, Math.floor(Number(availableHeight) || 0));

                imgContainer.style.width = '100%';
                imgContainer.style.maxWidth = '100%';
                imgContainer.style.minWidth = minDisplaySize + 'px';

                // 先讓寬度套用，再用實際寬度算高度（避免用 availableWidth 推算與實際 DOM 寬度有落差）
                const rectW = Math.floor((imgContainer.getBoundingClientRect().width || 0)) || Math.floor(Number(availableWidth) || 0) || minDisplaySize;
                let h = Math.floor(rectW / aspectRatio);
                if (h > targetH) h = targetH;
                h = Math.max(minDisplaySize, h);

                imgContainer.style.height = h + 'px';
                imgContainer.style.maxHeight = h + 'px';
                imgContainer.style.minHeight = minDisplaySize + 'px';
                // cropper 的 overlay 是 absolute 定位，這裡固定 overflow/position，避免高度/遮罩錯位
                imgContainer.style.overflow = 'hidden';
                imgContainer.style.position = 'relative';
                const containerRect = imgContainer.getBoundingClientRect();
                
                // 初始化 Cropper（使用 viewMode: 1 確保圖片填滿容器）
                // 注意：不啟用 responsive，避免自動調整導致閃爍；改由我們在 ready/resize 時做一次性調整
                const buildCropperOptions = () => {
                    // 注意：containerRect 可能在 modal 顯示/動畫的幾幀內變動，所以每次 build 都重新取一次最新值
                    const rect = imgContainer.getBoundingClientRect();
                    return {
                        ...this.options,
                        viewMode: 1, // 確保圖片填滿容器
                        responsive: false, // Cropper.js v1.5.13 沒有公開 resize()，因此用我們的 observer/rebuild 來同步
                        autoCrop: true, // 強制啟用自動裁剪（移動端關鍵）
                        autoCropArea: 1, // 盡量大的 crop area
                        minContainerWidth: Math.max(minDisplaySize, Math.floor(rect.width || 0)),
                        minContainerHeight: Math.max(minDisplaySize, Math.floor(rect.height || 0)),
                        // 移動端：禁用旋轉功能，避免 rotate 相關錯誤
                        rotatable: !isMobile, // 移動端禁用旋轉
                        scalable: !isMobile, // 移動端禁用縮放（避免相關錯誤）
                        zoomable: !isMobile, // 移動端禁用縮放（避免相關錯誤）
                        ready: () => {
                            try {
                                // 確保 cropper 已初始化
                                if (this.cropper && imgContainer) {
                                    // 強制調用 crop() 確保 crop box 被正確創建（移動端關鍵）
                                    try {
                                        if (typeof this.cropper.crop === 'function') {
                                            this.cropper.crop();
                                        }
                                    } catch (cropErr) {
                                        console.warn('[Crop] crop() in ready failed:', cropErr);
                                    }
                                    this._cropperReady = true;
                            // 讓 cropper-view-box/face 盡量大、圖片(canvas)盡量吃滿寬度
                                    requestAnimationFrame(() => {
                                        if (this.cropper) {
                                            this.applyMaxLayout(imgContainer);
                                        }
                                    });
                                }
                            } catch (e) {
                                console.warn('[Crop] Error in ready callback:', e);
                            }
                        }
                    };
                };

                const syncCropperContainerSize = () => {
                    try {
                        const cropperEl = imgContainer.querySelector('.cropper-container');
                        if (!cropperEl) return;
                        const h = Math.floor(imgContainer.clientHeight || 0);
                        if (h > 0) {
                            // 強制同步高度，避免桌面端 modal reflow 造成 cropper-container 高度落後
                            cropperEl.style.height = h + 'px';
                            cropperEl.style.maxHeight = h + 'px';
                        }
                        // 依需求：保持 width 100%
                        cropperEl.style.width = '100%';
                        cropperEl.style.maxWidth = '100%';
                    } catch (_) { /* ignore */ }
                };

                const createCropper = () => {
                    try {
                        this._cropperReady = false;
                        // 確保圖片元素有效
                        if (!img || !img.complete || img.naturalWidth === 0 || img.naturalHeight === 0) {
                            console.error('[Crop] Invalid image element for Cropper');
                            return;
                        }
                        
                        // 確保 Cropper 類存在
                        if (typeof Cropper === 'undefined') {
                            console.error('[Crop] Cropper.js is not loaded');
                            App.showAlert('圖片裁剪庫未載入，請刷新頁面重試', 'warning');
                            return;
                        }
                        
                        const options = buildCropperOptions();
                        this.cropper = new Cropper(img, options);
                        
                    // 建立後等一幀，確保 DOM 已插入 cropper-container，再同步一次尺寸
                    requestAnimationFrame(() => {
                            if (this.cropper) {
                                try {
                        syncCropperContainerSize();
                        this.applyMaxLayout(imgContainer);
                                } catch (e) {
                                    console.warn('[Crop] Error in post-create setup:', e);
                                }
                            }
                    });
                    } catch (e) {
                        console.error('[Crop] Error creating Cropper instance:', e);
                        App.showAlert('初始化裁剪工具失敗，請刷新頁面重試', 'error');
                        this.cropper = null;
                    }
                };

                const createAfterLayout = () => {
                    // 桌面端 modal 的高度常在顯示後幾幀才穩定：延後到下一/下下幀再 init，避免 cropper-container 用到偏小高度
                    if (!isMobile) {
                        requestAnimationFrame(() => requestAnimationFrame(() => createCropper()));
                    } else {
                        requestAnimationFrame(() => createCropper());
                    }
                };

                createAfterLayout();

                // 桌面端：modal 顯示/字體載入/滾動條變化可能讓容器高度在幾幀內變動
                // Cropper.js v1.5.13 沒有公開 resize()，因此用 ResizeObserver 監聽容器尺寸，必要時 destroy/recreate 以同步 overlay
                const scheduleRebuild = () => {
                    if (isMobile) return; // mobile 已穩定且 rebuild 成本較高
                    if (this._isRebuilding) return;
                    if (!this.cropper) return;
                    if (!modal.classList.contains('show')) return;
                    if (this._rebuildTimer) clearTimeout(this._rebuildTimer);
                    this._rebuildTimer = setTimeout(() => {
                        if (this._isRebuilding) return;
                        if (!this.cropper) return;
                        if (!modal.classList.contains('show')) return;
                        this._isRebuilding = true;
                        try { this.cropper.destroy(); } catch (_) { /* ignore */ }
                        try { createAfterLayout(); } catch (_) { /* ignore */ }
                        this._isRebuilding = false;
                    }, 80);
                };

                const onWindowResize = () => {
                    // 旋轉/縮放/視窗變化：桌面用 rebuild；mobile 仍只做 applyMaxLayout
                    if (isMobile) {
                        if (!this.cropper) return;
                        syncCropperContainerSize();
                        this.applyMaxLayout(imgContainer);
                        return;
                    }
                    scheduleRebuild();
                };

                window.addEventListener('resize', onWindowResize, { passive: true });
                // 初次初始化後再補一次同步（某些裝置/瀏覽器 ready 前後的 layout 會跳動）
                setTimeout(onWindowResize, 0);
                if (!isMobile) {
                    requestAnimationFrame(() => requestAnimationFrame(onWindowResize));
                }

                if (typeof ResizeObserver !== 'undefined') {
                    try {
                        if (this._resizeObserver) this._resizeObserver.disconnect();
                        this._resizeObserver = new ResizeObserver(() => scheduleRebuild());
                        this._resizeObserver.observe(imgContainer);
                    } catch (_) { /* ignore */ }
                }

                modal.addEventListener('hidden.bs.modal', () => {
                    window.removeEventListener('resize', onWindowResize);
                    if (this._resizeObserver) {
                        try { this._resizeObserver.disconnect(); } catch (_) { /* ignore */ }
                        this._resizeObserver = null;
                    }
                    if (this._rebuildTimer) {
                        clearTimeout(this._rebuildTimer);
                        this._rebuildTimer = null;
                    }
                    this._isRebuilding = false;
                }, { once: true });

                // 一鍵去背景按鈕
                const removeBgBtn = document.getElementById('removeBgBtn');
                if (removeBgBtn) {
                    // 移動端優化：按鈕文字改為"去背景"
                    const updateRemoveBgBtnText = () => {
                        if (window.innerWidth <= 768) {
                            if (removeBgBtn.innerHTML.includes('一鍵去背景')) {
                                removeBgBtn.innerHTML = removeBgBtn.innerHTML.replace('一鍵去背景', '去背景');
                            }
                        } else {
                            if (removeBgBtn.innerHTML.includes('去背景') && !removeBgBtn.innerHTML.includes('一鍵')) {
                                removeBgBtn.innerHTML = removeBgBtn.innerHTML.replace('去背景', '一鍵去背景');
                            }
                        }
                    };
                    updateRemoveBgBtnText();
                    window.addEventListener('resize', updateRemoveBgBtnText);
                    
                    removeBgBtn.style.display = 'block';
                    removeBgBtn.addEventListener('click', async () => {
                        if (!this.cropper) return;
                        
                        try {
                            removeBgBtn.disabled = true;
                            removeBgBtn.innerHTML = '<i class="bi bi-hourglass-split"></i> 處理中...';
                            
                            // 獲取裁剪區域的圖片數據
                            const canvas = this.cropper.getCroppedCanvas({
                                width: 800,
                                height: 800,
                                imageSmoothingEnabled: true,
                                imageSmoothingQuality: 'high'
                            });
                            
                            if (!canvas) {
                                throw new Error('無法獲取裁剪區域');
                            }
                            
                            // 使用 Canvas API 進行簡單的去背景處理（基於邊緣檢測和顏色相似度）
                            const processedCanvas = await this.removeBackground(canvas);
                            
                            // 將處理後的圖片轉換為 blob 並更新 cropper
                            processedCanvas.toBlob((blob) => {
                                if (blob) {
                                    const url = URL.createObjectURL(blob);
                                    img.src = url;
                                    
                                    // 重新初始化 cropper
                                    if (this.cropper) {
                                        this.cropper.destroy();
                                    }
                                    
                                    // 等待圖片加載後重新初始化
                                    const reloadImg = () => {
                                        if (!img.complete || img.naturalWidth === 0 || img.naturalHeight === 0) {
                                            setTimeout(reloadImg, 50);
                                            return;
                                        }
                                        
                                        try {
                                            // 確保容器尺寸正確
                                            const modalBody = imgContainer.closest('.modal-body');
                                            const isMobile = window.innerWidth < 768;
                                            let availableWidth = 800;
                                            let availableHeight = 600;
                                            const viewportTargetHeight = window.innerHeight * (isMobile ? 0.65 : 0.75);
                                            
                                            if (modalBody) {
                                                const bodyRect = modalBody.getBoundingClientRect();
                                                const bodyStyle = window.getComputedStyle(modalBody);
                                                const paddingLeft = parseFloat(bodyStyle.paddingLeft) || 0;
                                                const paddingRight = parseFloat(bodyStyle.paddingRight) || 0;
                                                const paddingTop = parseFloat(bodyStyle.paddingTop) || 0;
                                                const paddingBottom = parseFloat(bodyStyle.paddingBottom) || 0;
                                                
                                                availableWidth = bodyRect.width - paddingLeft - paddingRight - 20;
                                                
                                                if (isMobile) {
                                                    availableHeight = viewportTargetHeight;
                                                } else {
                                                    const innerBodyH = (bodyRect.height - paddingTop - paddingBottom - 16);
                                                    availableHeight = Math.min(innerBodyH > 0 ? innerBodyH : viewportTargetHeight, viewportTargetHeight);
                                                }
                                            } else {
                                                availableWidth = isMobile ? window.innerWidth - 40 : 800;
                                                availableHeight = viewportTargetHeight;
                                            }
                                            
                                            const minDisplaySize = isMobile ? 250 : 300;
                                            const maxAllowedHeight = Math.floor((isMobile ? 0.65 : 0.75) * window.innerHeight);
                                            const safeAvailableHeight = Math.floor(Number(availableHeight) || 0);
                                            availableHeight = Math.max(
                                                minDisplaySize,
                                                Math.min(safeAvailableHeight || minDisplaySize, maxAllowedHeight || minDisplaySize)
                                            );
                                            
                                            // 重新計算容器尺寸
                                            const aspectRatio = (Number(this.options.aspectRatio) > 0) ? Number(this.options.aspectRatio) : 1;
                                            const targetH = Math.max(minDisplaySize, Math.floor(Number(availableHeight) || 0));
                                            
                                            imgContainer.style.width = '100%';
                                            imgContainer.style.maxWidth = '100%';
                                            imgContainer.style.minWidth = minDisplaySize + 'px';
                                            
                                            const rectW = Math.floor((imgContainer.getBoundingClientRect().width || 0)) || Math.floor(Number(availableWidth) || 0) || minDisplaySize;
                                            let h = Math.floor(rectW / aspectRatio);
                                            if (h > targetH) h = targetH;
                                            h = Math.max(minDisplaySize, h);
                                            
                                            imgContainer.style.height = h + 'px';
                                            imgContainer.style.maxHeight = h + 'px';
                                            imgContainer.style.minHeight = minDisplaySize + 'px';
                                            imgContainer.style.overflow = 'hidden';
                                            imgContainer.style.position = 'relative';
                                            
                                            // 重新初始化 cropper
                                            const options = buildCropperOptions();
                                            this.cropper = new Cropper(img, options);
                                            
                                            // 等待 cropper 完全初始化後再同步尺寸
                                            requestAnimationFrame(() => {
                                                requestAnimationFrame(() => {
                                                    if (this.cropper) {
                                                        try {
                                                            syncCropperContainerSize();
                                                            this.applyMaxLayout(imgContainer);
                                                        } catch (e) {
                                                            console.warn('[Crop] Error syncing after remove background:', e);
                                                        }
                                                    }
                                                });
                                            });
                                            
                                            removeBgBtn.disabled = false;
                                            const btnText = window.innerWidth <= 768 ? '去背景' : '一鍵去背景';
                                            removeBgBtn.innerHTML = `<i class="bi bi-magic"></i> ${btnText}`;
                                            App.showAlert('背景已移除', 'success');
                                        } catch (error) {
                                            console.error('Error reinitializing cropper:', error);
                                            removeBgBtn.disabled = false;
                                            const btnText = window.innerWidth <= 768 ? '去背景' : '一鍵去背景';
                                            removeBgBtn.innerHTML = `<i class="bi bi-magic"></i> ${btnText}`;
                                            App.showAlert('重新初始化裁剪區域失敗: ' + error.message, 'danger');
                                        }
                                    };
                                    
                                    if (img.complete && img.naturalWidth > 0 && img.naturalHeight > 0) {
                                        reloadImg();
                                    } else {
                                        img.onload = reloadImg;
                                    }
                                } else {
                                    throw new Error('無法創建處理後的圖片');
                                }
                            }, 'image/png');
                        } catch (error) {
                            console.error('Remove background error:', error);
                            App.showAlert('去背景失敗: ' + error.message, 'danger');
                            removeBgBtn.disabled = false;
                            const btnText = window.innerWidth <= 768 ? '去背景' : '一鍵去背景';
                            removeBgBtn.innerHTML = `<i class="bi bi-magic"></i> ${btnText}`;
                        }
                    });
                }

                // 確認裁剪按鈕
                const cropConfirmBtn = document.getElementById('cropConfirmBtn');
                if (cropConfirmBtn) {
                    // 使用 touchstart 和 click 事件，确保移动端可以触发
                    const handleConfirm = (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        
                        // 防止重複處理
                        if (this._isProcessing) {
                            console.warn('[Crop] Already processing, ignoring duplicate click');
                            return;
                        }
                        
                        if (!this.cropper) {
                            console.error('Cropper not initialized');
                            App.showAlert('裁剪工具未初始化，請刷新頁面重試', 'error');
                            return;
                        }
                        
                        // 標記為處理中
                        this._isProcessing = true;
                        
                        const aspect = Number(this.options.aspectRatio) > 0 ? Number(this.options.aspectRatio) : 1;
                        const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
                        
                        // 手機端使用合理的默認尺寸（從較大尺寸開始，確保高品質輸出）
                        // 移動端從 800 開始，確保高品質
                        let desiredW = Number(exportOptions.width || exportOptions.maxWidth || (isMobile ? 800 : 800));
                        let desiredH = Number(exportOptions.height || Math.round(desiredW / aspect) || (isMobile ? 800 : 800));
                        
                        // 手機端合理限制最大尺寸（提高到 1000，確保高品質）
                        const maxMobileSize = isMobile ? 1000 : 2000;
                        if (desiredW > maxMobileSize || desiredH > maxMobileSize) {
                            if (desiredW > desiredH) {
                                desiredW = maxMobileSize;
                                desiredH = Math.round(desiredW / aspect);
                            } else {
                                desiredH = maxMobileSize;
                                desiredW = Math.round(desiredH * aspect);
                            }
                        }
                        
                        // 禁用按鈕，避免重複點擊
                        cropConfirmBtn.disabled = true;
                        cropConfirmBtn.textContent = '處理中...';
                        
                        // 移動端：使用後端裁剪 API（徹底解決內存問題）
                        if (isMobile) {
                            (async () => {
                                try {
                                    // 確保 cropper 已初始化
                                    if (!this.cropper) {
                                        throw new Error('Cropper not initialized');
                                    }
                                    
                                    // 等待一小段時間，確保 Cropper 完全準備好（移動端可能需要更多時間）
                                    await new Promise(resolve => setTimeout(resolve, 100));
                                    
                                    // 獲取圖片數據，用於 fallback 和驗證
                                    let imageData = null;
                                    try {
                                        if (typeof this.cropper.getImageData === 'function') {
                                            imageData = this.cropper.getImageData();
                                        }
                                    } catch (e) {
                                        console.warn('[Crop] Error getting image data:', e);
                                    }
                                    
                                    // 驗證圖片數據是否有效
                                    if (!imageData || imageData.naturalWidth === 0 || imageData.naturalHeight === 0) {
                                        throw new Error('圖片數據無效，無法進行裁剪');
                                    }
                                    
                                    // 獲取原始文件尺寸（如果還沒有設置）
                                    if (!originalImageWidth || !originalImageHeight) {
                                        // 從原始文件加載獲取尺寸
                                        await new Promise((resolve) => {
                                            const tempImg = new Image();
                                            const tempUrl = URL.createObjectURL(originalFile);
                                            tempImg.onload = () => {
                                                originalImageWidth = tempImg.naturalWidth || tempImg.width;
                                                originalImageHeight = tempImg.naturalHeight || tempImg.height;
                                                URL.revokeObjectURL(tempUrl);
                                                resolve();
                                            };
                                            tempImg.onerror = () => {
                                                URL.revokeObjectURL(tempUrl);
                                                // 如果失敗，使用 Cropper 的圖片數據
                                                if (imageData) {
                                                    originalImageWidth = imageData.naturalWidth || imageData.width;
                                                    originalImageHeight = imageData.naturalHeight || imageData.height;
                                                }
                                                resolve();
                                            };
                                            tempImg.src = tempUrl;
                                        });
                                    }
                                    
                                    // 獲取壓縮後的尺寸（從 Cropper 的圖片數據）
                                    if (!compressedImageWidth || !compressedImageHeight) {
                                        compressedImageWidth = imageData.naturalWidth || imageData.width;
                                        compressedImageHeight = imageData.naturalHeight || imageData.height;
                                    }
                                    
                                    console.log('[Crop] Image dimensions:', {
                                        original: { width: originalImageWidth, height: originalImageHeight },
                                        compressed: { width: compressedImageWidth, height: compressedImageHeight }
                                    });

                                    // 等待 Cropper ready（移動端可能需要較久）
                                    if (!this._cropperReady) {
                                        const readyStart = Date.now();
                                        while (!this._cropperReady && Date.now() - readyStart < 1200) {
                                            await new Promise(resolve => setTimeout(resolve, 60));
                                        }
                                    }

                                    // 強制觸發一次 crop()，確保 crop box data 可用
                                    if (this.cropper && typeof this.cropper.crop === 'function') {
                                        try {
                                            this.cropper.crop();
                                        } catch (e) {
                                            console.warn('[Crop] crop() call failed:', e);
                                        }
                                    }
                                    await new Promise(resolve => setTimeout(resolve, 60));
                                    
                                    // 關鍵修復：檢查 crop box 是否存在，如果不存在則強制創建
                                    const containerData = this.cropper.getContainerData ? this.cropper.getContainerData() : null;
                                    const imgData = this.cropper.getImageData ? this.cropper.getImageData() : null;
                                    let cropBoxCheck = this.cropper.getCropBoxData ? this.cropper.getCropBoxData() : null;
                                    
                                    console.log('[Crop] Pre-check:', {
                                        containerData: JSON.stringify(containerData),
                                        imageData: JSON.stringify(imgData),
                                        cropBoxData: JSON.stringify(cropBoxCheck),
                                        cropperReady: this._cropperReady
                                    });
                                    
                                    // 如果 crop box 為空或無效，強制設置一個覆蓋整個圖片的 crop box
                                    if (!cropBoxCheck || !cropBoxCheck.width || cropBoxCheck.width <= 0 || !cropBoxCheck.height || cropBoxCheck.height <= 0) {
                                        console.log('[Crop] Crop box is empty, forcing creation...');
                                        
                                        // 方法 1: 使用 setData 設置裁剪區域（整張圖片）
                                        if (imgData && imgData.naturalWidth > 0 && imgData.naturalHeight > 0) {
                                            try {
                                                // 先 enable cropping
                                                if (typeof this.cropper.enable === 'function') {
                                                    this.cropper.enable();
                                                }
                                                // 再調用 crop
                                                if (typeof this.cropper.crop === 'function') {
                                                    this.cropper.crop();
                                                }
                                                await new Promise(resolve => setTimeout(resolve, 50));
                                                
                                                // 設置裁剪區域為整個圖片
                                                if (typeof this.cropper.setData === 'function') {
                                                    this.cropper.setData({
                                                        x: 0,
                                                        y: 0,
                                                        width: imgData.naturalWidth,
                                                        height: imgData.naturalHeight
                                                    });
                                                    console.log('[Crop] Set initial crop data to full image');
                                                }
                                                await new Promise(resolve => setTimeout(resolve, 50));
                                                
                                                // 再次檢查
                                                cropBoxCheck = this.cropper.getCropBoxData();
                                                console.log('[Crop] After setData, cropBoxData:', JSON.stringify(cropBoxCheck));
                                            } catch (e) {
                                                console.warn('[Crop] Error forcing crop box:', e);
                                            }
                                        }
                                        
                                        // 方法 2: 如果還是空的，直接使用 setCropBoxData
                                        if (!cropBoxCheck || !cropBoxCheck.width || cropBoxCheck.width <= 0) {
                                            try {
                                                if (typeof this.cropper.setCropBoxData === 'function' && imgData) {
                                                    const boxWidth = imgData.width || 200;
                                                    const boxHeight = imgData.height || 200;
                                                    const boxLeft = imgData.left || 0;
                                                    const boxTop = imgData.top || 0;
                                                    
                                                    this.cropper.setCropBoxData({
                                                        left: boxLeft,
                                                        top: boxTop,
                                                        width: boxWidth,
                                                        height: boxHeight
                                                    });
                                                    console.log('[Crop] Forced setCropBoxData:', { left: boxLeft, top: boxTop, width: boxWidth, height: boxHeight });
                                                    await new Promise(resolve => setTimeout(resolve, 50));
                                                }
                                            } catch (e) {
                                                console.warn('[Crop] Error in setCropBoxData:', e);
                                            }
                                        }
                                    }
                                    
                                    // 獲取裁剪區域數據（使用 getData(true) 獲取相對於圖片的裁剪數據）
                                    if (typeof this.cropper.getData !== 'function') {
                                        throw new Error('Cropper getData method not available');
                                    }
                                    
                                    // 終極方案：直接從 DOM 讀取 crop box 位置（最可靠的方法）
                                    const getCropDataFromDOM = () => {
                                        try {
                                            const cropBox = document.querySelector('.cropper-crop-box');
                                            const cropperCanvas = document.querySelector('.cropper-canvas');
                                            const viewBox = document.querySelector('.cropper-view-box img');
                                            
                                            if (!cropBox || !cropperCanvas) {
                                                console.log('[Crop] DOM elements not found:', { cropBox: !!cropBox, cropperCanvas: !!cropperCanvas });
                                                return null;
                                            }
                                            
                                            // 使用 getBoundingClientRect 獲取實際屏幕坐標（最可靠）
                                            const boxRect = cropBox.getBoundingClientRect();
                                            const canvasRect = cropperCanvas.getBoundingClientRect();
                                            
                                            console.log('[Crop] DOM getBoundingClientRect:', {
                                                box: { left: boxRect.left, top: boxRect.top, width: boxRect.width, height: boxRect.height },
                                                canvas: { left: canvasRect.left, top: canvasRect.top, width: canvasRect.width, height: canvasRect.height }
                                            });
                                            
                                            // 獲取圖片的自然尺寸
                                            const cropperImg = document.querySelector('#cropImage');
                                            const naturalWidth = cropperImg ? (cropperImg.naturalWidth || compressedImageWidth) : compressedImageWidth;
                                            const naturalHeight = cropperImg ? (cropperImg.naturalHeight || compressedImageHeight) : compressedImageHeight;
                                            
                                            if (boxRect.width <= 0 || boxRect.height <= 0 || canvasRect.width <= 0 || canvasRect.height <= 0) {
                                                console.log('[Crop] Invalid rect dimensions');
                                                return null;
                                            }
                                            
                                            // crop box 相對於 canvas 的位置（屏幕像素）
                                            const relativeLeft = boxRect.left - canvasRect.left;
                                            const relativeTop = boxRect.top - canvasRect.top;
                                            
                                            // 計算縮放比例：自然尺寸 / 顯示尺寸
                                            const scaleX = naturalWidth / canvasRect.width;
                                            const scaleY = naturalHeight / canvasRect.height;
                                            
                                            console.log('[Crop] Scale factors:', { scaleX, scaleY, naturalWidth, naturalHeight });
                                            console.log('[Crop] Relative position:', { relativeLeft, relativeTop });
                                            
                                            // 計算裁剪框在原始圖片中的位置
                                            const cropX = Math.max(0, Math.round(relativeLeft * scaleX));
                                            const cropY = Math.max(0, Math.round(relativeTop * scaleY));
                                            const cropWidth = Math.round(boxRect.width * scaleX);
                                            const cropHeight = Math.round(boxRect.height * scaleY);
                                            
                                            // 邊界檢查
                                            const finalWidth = Math.min(cropWidth, naturalWidth - cropX);
                                            const finalHeight = Math.min(cropHeight, naturalHeight - cropY);
                                            
                                            console.log('[Crop] Calculated crop:', { x: cropX, y: cropY, width: finalWidth, height: finalHeight });
                                            
                                            if (finalWidth <= 0 || finalHeight <= 0) {
                                                console.log('[Crop] Invalid final dimensions');
                                                return null;
                                            }
                                            
                                            return {
                                                x: cropX,
                                                y: cropY,
                                                width: finalWidth,
                                                height: finalHeight
                                            };
                                        } catch (e) {
                                            console.warn('[Crop] Error reading from DOM:', e);
                                            return null;
                                        }
                                    };
                                    
                                    // 嘗試多次獲取數據（移動端可能需要重試）
                                    let cropData = null;
                                    
                                    // 首先嘗試從 DOM 直接讀取（最可靠）
                                    cropData = getCropDataFromDOM();
                                    if (cropData && cropData.width > 0 && cropData.height > 0) {
                                        console.log('[Crop] Got crop data from DOM:', JSON.stringify(cropData));
                                    } else {
                                        // 回退到 Cropper API
                                    for (let retry = 0; retry < 3; retry++) {
                                        try {
                                            const rawData = this.cropper.getData(true);
                                            console.log(`[Crop] getData attempt ${retry + 1}:`, JSON.stringify(rawData));
                                            
                                            // 檢查數據是否有效
                                            if (rawData && typeof rawData.width === 'number' && typeof rawData.height === 'number' &&
                                                rawData.width > 0 && rawData.height > 0) {
                                                cropData = rawData;
                                                break; // 成功獲取有效數據
                                            }
                                            
                                            // getData 返回無效值，嘗試用 getCropBoxData 和 getImageData 計算
                                            if (typeof this.cropper.getCropBoxData === 'function' && 
                                                typeof this.cropper.getImageData === 'function') {
                                                const cropBoxData = this.cropper.getCropBoxData();
                                                const imgData = this.cropper.getImageData();
                                                
                                                console.log(`[Crop] getCropBoxData attempt ${retry + 1}:`, JSON.stringify(cropBoxData));
                                                console.log(`[Crop] getImageData attempt ${retry + 1}:`, JSON.stringify(imgData));
                                                
                                                if (cropBoxData && imgData && 
                                                    cropBoxData.width > 0 && cropBoxData.height > 0 &&
                                                    imgData.width > 0 && imgData.height > 0) {
                                                    // 計算裁剪框相對於原始圖片的位置
                                                    const scaleX = imgData.naturalWidth / imgData.width;
                                                    const scaleY = imgData.naturalHeight / imgData.height;
                                                    
                                                    cropData = {
                                                        x: Math.max(0, Math.round((cropBoxData.left - imgData.left) * scaleX)),
                                                        y: Math.max(0, Math.round((cropBoxData.top - imgData.top) * scaleY)),
                                                        width: Math.round(cropBoxData.width * scaleX),
                                                        height: Math.round(cropBoxData.height * scaleY)
                                                    };
                                                    
                                                    // 邊界檢查
                                                    if (cropData.x + cropData.width > imgData.naturalWidth) {
                                                        cropData.width = imgData.naturalWidth - cropData.x;
                                                    }
                                                    if (cropData.y + cropData.height > imgData.naturalHeight) {
                                                        cropData.height = imgData.naturalHeight - cropData.y;
                                                    }
                                                    
                                                    console.log('[Crop] Calculated cropData from cropBoxData:', JSON.stringify(cropData));
                                                    
                                                    if (cropData.width > 0 && cropData.height > 0) {
                                                        break;
                                                    }
                                                }
                                            }
                                            
                                            if (retry < 2) {
                                                // 等待後重試
                                                await new Promise(resolve => setTimeout(resolve, 150));
                                            }
                                        } catch (e) {
                                            console.warn(`[Crop] Error getting crop data (attempt ${retry + 1}):`, e);
                                            if (retry < 2) {
                                                await new Promise(resolve => setTimeout(resolve, 150));
                                            }
                                        }
                                    }
                                    } // 結束 else 區塊（回退到 Cropper API）
                                    
                                    // 最後嘗試：如果 cropData 仍然無效，再次嘗試從 DOM 讀取
                                    if (!cropData || cropData.width <= 0 || cropData.height <= 0) {
                                        console.log('[Crop] Final fallback: trying DOM again');
                                        cropData = getCropDataFromDOM();
                                    }
                                    
                                    // 最後嘗試：如果 cropData 仍然無效，使用 getCropBoxData 計算
                                    if (!cropData || cropData.width <= 0 || cropData.height <= 0) {
                                        console.log('[Crop] Final fallback: trying getCropBoxData calculation');
                                        try {
                                            const cropBoxData = this.cropper.getCropBoxData();
                                            const imgData = this.cropper.getImageData();
                                            
                                            if (cropBoxData && imgData && 
                                                cropBoxData.width > 0 && cropBoxData.height > 0 &&
                                                imgData.naturalWidth > 0 && imgData.naturalHeight > 0) {
                                                
                                                const scaleX = imgData.naturalWidth / imgData.width;
                                                const scaleY = imgData.naturalHeight / imgData.height;
                                                
                                                cropData = {
                                                    x: Math.max(0, Math.round((cropBoxData.left - imgData.left) * scaleX)),
                                                    y: Math.max(0, Math.round((cropBoxData.top - imgData.top) * scaleY)),
                                                    width: Math.max(1, Math.round(cropBoxData.width * scaleX)),
                                                    height: Math.max(1, Math.round(cropBoxData.height * scaleY))
                                                };
                                                
                                                // 邊界檢查
                                                if (cropData.x < 0) cropData.x = 0;
                                                if (cropData.y < 0) cropData.y = 0;
                                                if (cropData.x + cropData.width > imgData.naturalWidth) {
                                                    cropData.width = Math.max(1, imgData.naturalWidth - cropData.x);
                                                }
                                                if (cropData.y + cropData.height > imgData.naturalHeight) {
                                                    cropData.height = Math.max(1, imgData.naturalHeight - cropData.y);
                                                }
                                                
                                                console.log('[Crop] Final fallback cropData:', JSON.stringify(cropData));
                                            }
                                        } catch (e) {
                                            console.error('[Crop] Final fallback failed:', e);
                                        }
                                    }
                                    
                                    if (!cropData) {
                                        throw new Error('無法獲取裁剪數據');
                                    }
                                    
                                    // 驗證裁剪數據有效性
                                    if (typeof cropData.x !== 'number' || typeof cropData.y !== 'number' || 
                                        typeof cropData.width !== 'number' || typeof cropData.height !== 'number') {
                                        throw new Error('裁剪數據格式無效');
                                    }
                                    
                                    // 如果裁剪尺寸仍然無效，使用壓縮後圖片尺寸作為 fallback（整張圖裁剪）
                                    if (cropData.width <= 0 || cropData.height <= 0) {
                                        console.warn(`[Crop] Invalid crop size: ${cropData.width}x${cropData.height}, using compressed image size`);
                                        if (compressedImageWidth > 0 && compressedImageHeight > 0) {
                                            cropData = {
                                                x: 0,
                                                y: 0,
                                                width: compressedImageWidth,
                                                height: compressedImageHeight
                                            };
                                        } else {
                                            throw new Error('無法確定裁剪區域');
                                        }
                                    }
                                    
                                    // 關鍵：將壓縮後圖片的坐標轉換為原始文件的坐標
                                    // cropData 是相對於壓縮後圖片（compressedImageWidth x compressedImageHeight）的坐標
                                    // 需要轉換為原始文件（originalImageWidth x originalImageHeight）的坐標
                                    let finalCropData = cropData;
                                    if (originalImageWidth && originalImageHeight && compressedImageWidth && compressedImageHeight) {
                                        const scaleX = originalImageWidth / compressedImageWidth;
                                        const scaleY = originalImageHeight / compressedImageHeight;
                                        
                                        finalCropData = {
                                            x: Math.round(cropData.x * scaleX),
                                            y: Math.round(cropData.y * scaleY),
                                            width: Math.round(cropData.width * scaleX),
                                            height: Math.round(cropData.height * scaleY)
                                        };
                                        
                                        // 邊界檢查
                                        if (finalCropData.x < 0) finalCropData.x = 0;
                                        if (finalCropData.y < 0) finalCropData.y = 0;
                                        if (finalCropData.x + finalCropData.width > originalImageWidth) {
                                            finalCropData.width = originalImageWidth - finalCropData.x;
                                        }
                                        if (finalCropData.y + finalCropData.height > originalImageHeight) {
                                            finalCropData.height = originalImageHeight - finalCropData.y;
                                        }
                                        
                                        console.log('[Crop] Coordinate conversion:', {
                                            scale: { x: scaleX, y: scaleY },
                                            before: cropData,
                                            after: finalCropData,
                                            original: { width: originalImageWidth, height: originalImageHeight },
                                            compressed: { width: compressedImageWidth, height: compressedImageHeight }
                                        });
                                    } else {
                                        console.warn('[Crop] Cannot convert coordinates, using cropData as-is', {
                                            originalImageWidth,
                                            originalImageHeight,
                                            compressedImageWidth,
                                            compressedImageHeight
                                        });
                                    }
                                    
                                    // 計算輸出尺寸
                                    let outputW = desiredW;
                                    let outputH = desiredH;
                                    
                                    // 確保輸出尺寸有效
                                    if (outputW <= 0) outputW = Math.max(1, Math.round(cropData.width));
                                    if (outputH <= 0) outputH = Math.max(1, Math.round(cropData.height));
                                    
                                    // 確保裁剪尺寸有效（最終檢查）
                                    if (finalCropData.width <= 0 || finalCropData.height <= 0) {
                                        throw new Error(`無法確定裁剪尺寸: ${finalCropData.width}x${finalCropData.height}`);
                                    }
                                    
                                    console.log('[Crop] Backend crop params:', {
                                        x: finalCropData.x,
                                        y: finalCropData.y,
                                        width: finalCropData.width,
                                        height: finalCropData.height,
                                        outputWidth: outputW,
                                        outputHeight: outputH,
                                        original: { width: originalImageWidth, height: originalImageHeight },
                                        compressed: { width: compressedImageWidth, height: compressedImageHeight }
                                    });
                                    
                                    // 創建 FormData
                                    const formData = new FormData();
                                    formData.append('file', originalFile);
                                    formData.append('x', finalCropData.x.toString());
                                    formData.append('y', finalCropData.y.toString());
                                    formData.append('width', finalCropData.width.toString());
                                    formData.append('height', finalCropData.height.toString());
                                    formData.append('outputWidth', outputW.toString());
                                    formData.append('outputHeight', outputH.toString());
                                    
                                    // 調用後端裁剪 API
                                    const token = localStorage.getItem('auth_token');
                                    const response = await fetch('/api/v1/crop', {
                                        method: 'POST',
                                        headers: {
                                            'Authorization': 'Bearer ' + token
                                        },
                                        body: formData
                                    });
                                    
                                    if (!response.ok) {
                                        const errorData = await response.json().catch(() => ({ error: '裁剪失敗' }));
                                        throw new Error(errorData.error || '裁剪失敗');
                                    }
                                    
                                    const result = await response.json();
                                    
                                    // 下載裁剪後的圖片並轉換為 Blob
                                    const imageResponse = await fetch(result.url);
                                    const blob = await imageResponse.blob();
                                    
                                    // 關閉 modal
                                    confirmed = true;
                                    safeFocusBody();
                                    this.modal.hide();
                                    
                                    // 調用 callback
                                    if (callback) {
                                        callback(blob);
                                    }
                                    
                                    // 重置處理標誌
                                    setTimeout(() => {
                                        this._isProcessing = false;
                                        cropConfirmBtn.disabled = false;
                                        cropConfirmBtn.textContent = '確認選擇';
                                    }, 300);
                                } catch (error) {
                                    console.error('[Crop] Backend crop failed:', error);
                                    
                                    // 顯示詳細錯誤信息（手機端調試用）
                                    let debugInfo = error.message || '圖片裁剪失敗';
                                    try {
                                        // 收集調試信息
                                        const cropperInfo = this.cropper ? {
                                            getData: JSON.stringify(this.cropper.getData(true)),
                                            getCropBoxData: JSON.stringify(this.cropper.getCropBoxData()),
                                            getImageData: JSON.stringify(this.cropper.getImageData())
                                        } : 'cropper is null';
                                        debugInfo += '\n\nDebug Info:\n' + JSON.stringify(cropperInfo, null, 2);
                                        debugInfo += `\noriginal: ${originalImageWidth}x${originalImageHeight}`;
                                        debugInfo += `\ncompressed: ${compressedImageWidth}x${compressedImageHeight}`;
                                    } catch (e) {
                                        debugInfo += '\n\nFailed to collect debug info: ' + e.message;
                                    }
                                    
                                    // 顯示錯誤面板
                                    if (window.JSErrorHandler && typeof window.JSErrorHandler.show === 'function') {
                                        window.JSErrorHandler.show();
                                    }
                                    
                                    App.showAlert(debugInfo, 'error');
                                    cropConfirmBtn.disabled = false;
                                    cropConfirmBtn.textContent = '確認選擇';
                                    this._isProcessing = false;
                                }
                            })();
                            return; // 移動端使用後端裁剪，直接返回
                        }
                        
                        // 桌面端：使用前端裁剪（原有邏輯）
                        // 遞歸降級處理函數 - 使用合理的降級策略，確保高品質輸出
                        const attemptCrop = async (targetW, targetH, quality, attempt = 1) => {
                            const maxAttempts = 10; // 10次嘗試足夠
                            // 手機端：降級策略，從800開始（確保高品質），降到200（仍然可用）
                            const sizes = isMobile ? [800, 700, 600, 500, 400, 350, 300, 250, 200, 180] : [2000, 1500, 1200, 1000, 800, 600, 500, 400, 300, 250];
                            // 品質也逐步降低（移動端從較高品質開始，確保輸出質量）
                            const qualities = isMobile ? [0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5, 0.45, 0.4, 0.35] : [0.9, 0.85, 0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5, 0.45];
                            
                            // 直接使用降級策略的對應值（確保每次嘗試都使用正確的尺寸）
                            let sizeIndex = Math.min(attempt - 1, sizes.length - 1);
                            // 如果超過最大嘗試次數，使用最後一個（最小的）尺寸
                            if (attempt > maxAttempts) {
                                sizeIndex = sizes.length - 1;
                            }
                            targetW = sizes[sizeIndex];
                                targetH = Math.round(targetW / aspect);
                            quality = qualities[sizeIndex];
                            
                            // 確保最小尺寸不小於 180（保持可用質量）
                            if (targetW < 180) {
                                targetW = 180;
                                targetH = Math.round(180 / aspect);
                                quality = 0.35; // 使用合理的最低品質
                            }
                            
                            console.log(`[Crop] Attempt ${attempt}/${maxAttempts}: ${targetW}x${targetH}, quality=${quality}, mobile=${isMobile}`);
                            
                            let timeoutId = null;
                            let blobCallbackCalled = false;
                            
                            const cleanup = () => {
                                if (timeoutId) {
                                    clearTimeout(timeoutId);
                                    timeoutId = null;
                                }
                            };
                            
                            const resetButton = () => {
                                cleanup();
                                cropConfirmBtn.disabled = false;
                                cropConfirmBtn.textContent = '確認選擇';
                            };
                            
                            const handleSuccess = (blob, canvasRef) => {
                                blobCallbackCalled = true;
                                cleanup();
                                
                                // 清理之前的 canvas 和 blob（立即清理，不保存引用）
                                if (this._currentCanvas && this._currentCanvas !== canvasRef) {
                                    try {
                                        const ctx = this._currentCanvas.getContext('2d');
                                        if (ctx) {
                                            ctx.clearRect(0, 0, this._currentCanvas.width, this._currentCanvas.height);
                                        }
                                        // 重置 canvas 尺寸以釋放內存
                                        this._currentCanvas.width = 0;
                                        this._currentCanvas.height = 0;
                                    } catch (e) {
                                        console.warn('[Crop] Error clearing previous canvas:', e);
                                    }
                                    this._currentCanvas = null;
                                }
                                
                                if (this._currentBlob && this._currentBlob !== blob) {
                                    try {
                                        // 注意：不能直接 revoke blob，需要先创建 URL
                                        const url = URL.createObjectURL(this._currentBlob);
                                        URL.revokeObjectURL(url);
                                    } catch (e) {
                                        console.warn('[Crop] Error revoking previous blob:', e);
                                    }
                                    this._currentBlob = null;
                                }
                                
                                if (!blob) {
                                    console.error(`[Crop] Attempt ${attempt}: toBlob returned null (canvas: ${canvasRef ? canvasRef.width + 'x' + canvasRef.height : 'null'}, quality: ${quality})`);
                                    // 清理當前 canvas（如果存在）
                                if (canvasRef) {
                                    try {
                                        const ctx = canvasRef.getContext('2d');
                                        if (ctx) {
                                            ctx.clearRect(0, 0, canvasRef.width, canvasRef.height);
                                        }
                                            canvasRef.width = 0;
                                            canvasRef.height = 0;
                                    } catch (e) {
                                            console.warn('[Crop] Error clearing canvas on null blob:', e);
                                    }
                                }
                                    if (attempt < maxAttempts) {
                                        console.log(`[Crop] Retrying with smaller size (attempt ${attempt + 1})...`);
                                        // 移動端給更多時間讓垃圾回收
                                        setTimeout(async () => await attemptCrop(targetW, targetH, quality, attempt + 1), isMobile ? 300 : 150);
                                    } else {
                                        // 最後一次嘗試失敗，使用最小可用尺寸
                                        console.warn(`[Crop] All attempts failed, trying minimum usable size (180x180, quality 0.35)...`);
                                        resetButton();
                                        // 不重置 _isProcessing，繼續最後一次嘗試
                                        setTimeout(async () => {
                                            await attemptCrop(180, Math.round(180 / aspect), 0.35, maxAttempts + 1);
                                        }, 500);
                                    }
                                    return;
                                }
                                
                                // 立即清理 canvas（不保存引用，避免內存累積）
                                if (canvasRef) {
                                    try {
                                        const ctx = canvasRef.getContext('2d');
                                        if (ctx) {
                                            ctx.clearRect(0, 0, canvasRef.width, canvasRef.height);
                                        }
                                        // 重置 canvas 尺寸以釋放內存（移動端特別重要）
                                        canvasRef.width = 0;
                                        canvasRef.height = 0;
                                    } catch (e) {
                                        console.warn('[Crop] Error clearing canvas after success:', e);
                                    }
                                }
                                
                                try {
                                    if (callback) {
                                        confirmed = true;
                                        // 在移動端，立即清理 Cropper 實例以釋放內存
                                        if (isMobile && this.cropper) {
                                            try {
                                                this.cropper.destroy();
                                                this.cropper = null;
                                                // 移動端：強制清理圖片元素
                                                const imgEl = document.getElementById('cropImage');
                                                if (imgEl) {
                                                    if (imgEl.src && imgEl.src.startsWith('blob:')) {
                                                        URL.revokeObjectURL(imgEl.src);
                                                    }
                                                    imgEl.src = '';
                                                    imgEl.onload = null;
                                                    imgEl.onerror = null;
                                                }
                                                // 移動端：清理 modal 中的所有 canvas（Cropper 創建的）
                                                const modal = document.getElementById('imageCropModal');
                                                if (modal) {
                                                    const canvases = modal.querySelectorAll('canvas');
                                                    canvases.forEach(canvas => {
                                                        try {
                                                            const ctx = canvas.getContext('2d');
                                                            if (ctx) {
                                                                ctx.clearRect(0, 0, canvas.width, canvas.height);
                                                            }
                                                            canvas.width = 0;
                                                            canvas.height = 0;
                                                        } catch (e) {
                                                            // ignore
                                                        }
                                                    });
                                                }
                                            } catch (e) {
                                                console.warn('[Crop] Error destroying cropper after success:', e);
                                            }
                                        }
                                        
                                        // 確保 modal 一定會關閉（無論 callback 是否成功）
                                        const closeModal = () => {
                                            try {
                                                safeFocusBody();
                                                if (this.modal) {
                                                    this.modal.hide();
                                                }
                                            } catch (e) {
                                                console.warn('[Crop] Error closing modal:', e);
                                                // 強制移除 modal
                                                try {
                                                    const modalEl = document.getElementById('imageCropModal');
                                                    if (modalEl) {
                                                        modalEl.style.display = 'none';
                                                        document.body.classList.remove('modal-open');
                                                        document.body.style.overflow = '';
                                                        document.body.style.paddingRight = '';
                                                    }
                                                } catch (e2) {
                                                    console.warn('[Crop] Error force closing modal:', e2);
                                                }
                                            }
                                        };
                                        
                                        // 先關閉 modal，再調用 callback（確保 modal 一定會關閉）
                                        closeModal();
                                        
                                        // 調用 callback（可能異步）
                                        try {
                                        callback(blob);
                                        } catch (callbackErr) {
                                            console.error('[Crop] Callback error:', callbackErr);
                                            // callback 錯誤不影響 modal 關閉
                                    }
                                    } else {
                                        // 沒有 callback 也要關閉 modal
                                    safeFocusBody();
                                    this.modal.hide();
                                    }
                                    // 延遲重置處理標誌，確保 modal 完全關閉
                                    setTimeout(() => {
                                        this._isProcessing = false;
                                        // 移動端：強制清理所有引用，幫助垃圾回收
                                        if (isMobile) {
                                            this._currentBlob = null;
                                            this._currentCanvas = null;
                                            // 移動端：強制清理所有 canvas 元素
                                            try {
                                                const allCanvases = document.querySelectorAll('canvas');
                                                allCanvases.forEach(canvas => {
                                                    if (canvas.width > 0 || canvas.height > 0) {
                                                        try {
                                                            const ctx = canvas.getContext('2d');
                                                            if (ctx) {
                                                                ctx.clearRect(0, 0, canvas.width, canvas.height);
                                                            }
                                                            canvas.width = 0;
                                                            canvas.height = 0;
                                                        } catch (e) {
                                                            // ignore
                                                        }
                                                    }
                                                });
                                            } catch (e) {
                                                console.warn('[Crop] Error clearing all canvases:', e);
                                            }
                                            // 移動端：完全移除 modal DOM（防止內存累積）
                                            try {
                                                const modalEl = document.getElementById('imageCropModal');
                                                if (modalEl) {
                                                    // 先清理所有資源
                                                    const imgEl = modalEl.querySelector('#cropImage');
                                                    if (imgEl && imgEl.src && imgEl.src.startsWith('blob:')) {
                                                        URL.revokeObjectURL(imgEl.src);
                                                    }
                                                    // 移除 modal
                                                    setTimeout(() => {
                                                        try {
                                                            modalEl.remove();
                                                        } catch (e) {
                                                            console.warn('[Crop] Error removing modal DOM:', e);
                                                        }
                                                    }, 300);
                                                }
                                            } catch (e) {
                                                console.warn('[Crop] Error cleaning modal DOM:', e);
                                            }
                                        }
                                    }, isMobile ? 500 : 300);
                                } catch (callbackError) {
                                    console.error('[Crop] Callback error:', callbackError);
                                    resetButton();
                                    this._isProcessing = false; // 重置處理標誌
                                    // 清理 canvas
                                    if (canvasRef) {
                                        try {
                                            const ctx = canvasRef.getContext('2d');
                                            if (ctx) {
                                                ctx.clearRect(0, 0, canvasRef.width, canvasRef.height);
                                            }
                                            canvasRef.width = 0;
                                            canvasRef.height = 0;
                                        } catch (e) {
                                            console.warn('[Crop] Error clearing canvas on callback error:', e);
                                        }
                                    }
                                    App.showAlert('處理圖片時發生錯誤，請重試', 'error');
                                }
                            };
                            
                            const handleError = (error, errorType) => {
                                cleanup();
                                console.error(`[Crop] Attempt ${attempt} ${errorType}:`, error);
                                
                                // 清理當前 canvas（如果存在）
                                if (this._currentCanvas) {
                                    try {
                                        const ctx = this._currentCanvas.getContext('2d');
                                        if (ctx) {
                                            ctx.clearRect(0, 0, this._currentCanvas.width, this._currentCanvas.height);
                                        }
                                        this._currentCanvas.width = 0;
                                        this._currentCanvas.height = 0;
                                    } catch (e) {
                                        console.warn('[Crop] Error clearing canvas on error:', e);
                                    }
                                    this._currentCanvas = null;
                                }
                                
                                if (attempt < maxAttempts) {
                                    console.log(`[Crop] Retrying with smaller size and lower quality (attempt ${attempt + 1})...`);
                                    // 移動端：給更多時間讓垃圾回收
                                    setTimeout(() => attemptCrop(targetW, targetH, quality, attempt + 1), isMobile ? 300 : 150);
                                } else {
                                    resetButton();
                                    this._isProcessing = false; // 重置處理標誌
                                    // 移動端：強制清理 Cropper 實例
                                    if (isMobile && this.cropper) {
                                        try {
                                            this.cropper.destroy();
                                            this.cropper = null;
                                        } catch (e) {
                                            console.warn('[Crop] Error destroying cropper on final error:', e);
                                        }
                                    }
                                    App.showAlert('圖片處理失敗，請嘗試選擇較小的圖片或使用較低解析度的照片', 'error');
                                }
                            };
                            
                            // 設置超時 - 手機端給更長時間，但根據嘗試次數調整
                            // 如果已經嘗試多次，說明尺寸可能還是太大，給更多時間
                            const timeoutDuration = isMobile ? (attempt > 4 ? 30000 : 20000) : 10000;
                            timeoutId = setTimeout(() => {
                                if (!blobCallbackCalled) {
                                    console.error(`[Crop] Attempt ${attempt}: toBlob timeout after ${timeoutDuration}ms`);
                                    if (attempt < maxAttempts) {
                                        cleanup();
                                        console.log(`[Crop] Retrying with smaller size due to timeout (attempt ${attempt + 1})...`);
                                        // 給更多時間讓垃圾回收
                                        setTimeout(async () => await attemptCrop(targetW, targetH, quality, attempt + 1), isMobile ? 500 : 200);
                                    } else {
                                        resetButton();
                                        this._isProcessing = false; // 重置處理標誌
                                        App.showAlert('圖片處理超時，請嘗試選擇較小的圖片', 'error');
                                    }
                                }
                            }, timeoutDuration);
                            
                            try {
                                // 確保 cropper 已初始化
                                if (!this.cropper) {
                                    throw new Error('Cropper not initialized');
                                }
                                
                                // 嘗試創建 canvas - 使用更保守的設置
                                let canvas;
                                try {
                                    // 檢查 getCroppedCanvas 方法是否存在
                                    if (typeof this.cropper.getCroppedCanvas !== 'function') {
                                        throw new Error('Cropper getCroppedCanvas method not available');
                                    }
                                    
                                    // 手機端使用合理的設置，使用 maxWidth/maxHeight 限制實際 Canvas 尺寸
                                    // 注意：預壓縮已經將圖片限制到 1500px，所以這裡不需要再次縮放
                                    const canvasOptions = {
                                        maxWidth: targetW,  // 關鍵：限制最大寬度，避免 Cropper 返回過大的 Canvas
                                        maxHeight: targetH, // 關鍵：限制最大高度，避免 Cropper 返回過大的 Canvas
                                        width: targetW,
                                        height: targetH,
                                        imageSmoothingEnabled: true,
                                        imageSmoothingQuality: isMobile ? (attempt > 5 ? 'medium' : 'high') : 'high',
                                        fillColor: '#ffffff'
                                    };
                                    
                                    canvas = this.cropper.getCroppedCanvas(canvasOptions);
                                } catch (canvasError) {
                                    console.error(`[Crop] Attempt ${attempt}: getCroppedCanvas failed:`, canvasError);
                                    handleError(canvasError, 'getCroppedCanvas');
                                    return;
                                }
                                
                                if (!canvas) {
                                    throw new Error('無法創建裁剪畫布');
                                }
                                
                                // 檢查 canvas 尺寸是否有效
                                if (canvas.width === 0 || canvas.height === 0) {
                                    throw new Error(`無效的畫布尺寸: ${canvas.width}x${canvas.height}`);
                                }
                                
                                // 檢查 canvas 尺寸（使用 maxWidth/maxHeight 後，實際尺寸應該不會超過限制）
                                // 但還是檢查一下，以防萬一
                                const maxCanvasPixels = isMobile ? 16777216 : 268435456; // 手機端約4Kx4K（合理限制），桌面端約16Kx16K
                                const actualPixels = canvas.width * canvas.height;
                                if (actualPixels > maxCanvasPixels) {
                                    console.warn(`[Crop] Canvas larger than expected: ${canvas.width}x${canvas.height} = ${actualPixels} pixels, max: ${maxCanvasPixels}`);
                                    // 如果 canvas 仍然太大（不應該發生，因為使用了 maxWidth/maxHeight），降級重試
                                    if (attempt < maxAttempts) {
                                        console.log(`[Crop] Canvas still too large, retrying with smaller size (attempt ${attempt + 1})...`);
                                        // 清理當前 canvas
                                        try {
                                            const ctx = canvas.getContext('2d');
                                            if (ctx) {
                                                ctx.clearRect(0, 0, canvas.width, canvas.height);
                                            }
                                            canvas.width = 0;
                                            canvas.height = 0;
                                        } catch (e) {
                                            // ignore
                                        }
                                        // 直接重試，使用降級策略的下一個尺寸
                                        setTimeout(async () => await attemptCrop(targetW, targetH, quality, attempt + 1), isMobile ? 300 : 150);
                                        return;
                                    }
                                    // 如果已經是最後一次嘗試，繼續使用當前 canvas（已經使用了 maxWidth/maxHeight，應該不會太大）
                                    console.warn(`[Crop] Last attempt, using current canvas (should be within limits)`);
                                }
                                
                                // 調用 toBlob - 使用多種策略確保成功
                                try {
                                    const doToBlob = () => {
                                        // 保存 canvas 引用用於清理
                                        const canvasToClean = canvas;
                                        let blobCallbackInvoked = false;
                                        
                                        // 主要 toBlob 調用
                                        canvas.toBlob((blob) => {
                                            if (blobCallbackInvoked) return; // 防止重複調用
                                            blobCallbackInvoked = true;
                                            
                                            // 回調完成後立即清理 canvas（移動端特別重要）
                                            handleSuccess(blob, canvasToClean);
                                            // 確保 canvas 被清理
                                            if (canvasToClean) {
                                                try {
                                                    const ctx = canvasToClean.getContext('2d');
                                                    if (ctx) {
                                                        ctx.clearRect(0, 0, canvasToClean.width, canvasToClean.height);
                                                    }
                                                    canvasToClean.width = 0;
                                                    canvasToClean.height = 0;
                                                } catch (e) {
                                                    console.warn('[Crop] Error clearing canvas in toBlob callback:', e);
                                                }
                                            }
                                        }, exportOptions.mimeType || 'image/jpeg', quality);
                                        
                                        // 如果 toBlob 返回 null 或超時，使用 fallback 方法
                                        setTimeout(() => {
                                            if (!blobCallbackInvoked && attempt < maxAttempts) {
                                                console.warn(`[Crop] toBlob may have failed, retrying with smaller size...`);
                                                blobCallbackInvoked = true; // 標記已處理
                                                // 清理 canvas
                                                if (canvasToClean) {
                                                    try {
                                                        const ctx = canvasToClean.getContext('2d');
                                                        if (ctx) {
                                                            ctx.clearRect(0, 0, canvasToClean.width, canvasToClean.height);
                                                        }
                                                        canvasToClean.width = 0;
                                                        canvasToClean.height = 0;
                                                    } catch (e) {
                                                        // ignore
                                                    }
                                                }
                                                // 重試更小的尺寸
                                                setTimeout(async () => await attemptCrop(targetW, targetH, quality, attempt + 1), isMobile ? 300 : 150);
                                            }
                                        }, 5000); // 5秒後檢查
                                    };
                                    
                                    // 手機端且嘗試次數多時，使用 requestIdleCallback 減少阻塞
                                    if (isMobile && attempt > 3 && typeof requestIdleCallback !== 'undefined') {
                                        requestIdleCallback(doToBlob, { timeout: 5000 });
                                    } else {
                                        doToBlob();
                                    }
                                } catch (toBlobError) {
                                    // 清理 canvas
                                    if (canvas) {
                                        try {
                                            const ctx = canvas.getContext('2d');
                                            if (ctx) {
                                                ctx.clearRect(0, 0, canvas.width, canvas.height);
                                            }
                                            canvas.width = 0;
                                            canvas.height = 0;
                                        } catch (e) {
                                            console.warn('[Crop] Error clearing canvas on toBlob error:', e);
                                        }
                                    }
                                    // 如果還有嘗試次數，繼續降級
                                    if (attempt < maxAttempts) {
                                        console.log(`[Crop] toBlob error, retrying with smaller size (attempt ${attempt + 1})...`);
                                        setTimeout(async () => await attemptCrop(targetW, targetH, quality, attempt + 1), isMobile ? 300 : 150);
                                    } else {
                                    handleError(toBlobError, 'toBlob');
                                    }
                                }
                            } catch (error) {
                                handleError(error, 'general');
                            }
                        };
                        
                        // 開始第一次嘗試（使用降級策略的第一個值，確保一致性）
                        // 注意：attemptCrop 內部會處理第一次嘗試的尺寸和品質
                        attemptCrop(desiredW, desiredH, 0, 1).catch(err => {
                            console.error('[Crop] Initial attempt failed:', err);
                            resetButton();
                            this._isProcessing = false;
                            App.showAlert('圖片處理失敗，請重試', 'error');
                        });
                    };
                    
                    // 清理舊的事件監聽器（如果存在）
                    if (this._confirmHandler) {
                        cropConfirmBtn.removeEventListener('click', this._confirmHandler);
                        cropConfirmBtn.removeEventListener('touchend', this._confirmHandler);
                    }
                    
                    // 保存處理器引用，用於後續清理
                    this._confirmHandler = handleConfirm;
                    
                    // 绑定多个事件确保移动端可以触发
                    cropConfirmBtn.addEventListener('click', handleConfirm);
                    cropConfirmBtn.addEventListener('touchend', handleConfirm);
                    
                    // 确保按钮在移动端可点击
                    cropConfirmBtn.style.pointerEvents = 'auto';
                    cropConfirmBtn.style.touchAction = 'manipulation';
                    cropConfirmBtn.style.zIndex = '1056';
                    cropConfirmBtn.style.position = 'relative';
                }
            };

            tryInit = () => {
                ensureModalShown();
                if (modalShown && imageLoaded) {
                    initCropper();
                }
            };

            // 先綁定事件，再設 src（避免某些瀏覽器/情境下同步完成導致漏接）
            let imageLoadTimeout = null;
            let imageLoadSuccess = false;
            
            img.addEventListener('load', () => {
                imageLoadSuccess = true;
                if (imageLoadTimeout) {
                    clearTimeout(imageLoadTimeout);
                    imageLoadTimeout = null;
                }
                markImageLoaded();
                tryInit();
            }, { once: true });
            
            img.addEventListener('error', () => {
                // 延遲檢查，因為某些瀏覽器可能會先觸發 error，然後圖片才真正加載完成
                imageLoadTimeout = setTimeout(() => {
                    // 再次檢查圖片是否真的加載失敗
                    if (!imageLoadSuccess && (!img.complete || img.naturalWidth === 0)) {
                        console.error('Failed to load image');
                        App.showAlert('圖片加載失敗，請重試', 'error');
                        safeFocusBody();
                        this.modal.hide();
                    }
                }, 100);
            }, { once: true });

            // 設置圖片源
            img.src = e.target.result;

            // 兜底：若圖片已經在事件綁定前完成（或某些瀏覽器不觸發 load），用 raf/complete 判斷一次
            requestAnimationFrame(() => {
                if (img.complete && img.naturalWidth > 0) {
                    imageLoadSuccess = true;
                    if (imageLoadTimeout) {
                        clearTimeout(imageLoadTimeout);
                        imageLoadTimeout = null;
                    }
                    markImageLoaded();
                    tryInit();
            }
            });

            // 兜底：更穩定的 decode（若支援）
            if (typeof img.decode === 'function') {
                img.decode().then(() => {
                    if (img.naturalWidth > 0) {
                        imageLoadSuccess = true;
                        if (imageLoadTimeout) {
                            clearTimeout(imageLoadTimeout);
                            imageLoadTimeout = null;
                        }
                        markImageLoaded();
                        tryInit();
                    }
                }).catch(() => {
                    // decode 失敗不一定代表圖片不可用（例如某些格式/瀏覽器），交給 onload/raf 處理
                });
            }

            // 若 modal 已經顯示完（手機慢機/快機都有可能），立即嘗試 init 一次
            tryInit();
        };
        
        reader.onerror = () => {
            console.error('Failed to read file');
            App.showAlert('讀取圖片失敗，請重試', 'error');
            safeFocusBody();
            this.modal.hide();
        };
        
        // 先顯示模態框（事件監聽已提前註冊），然後讀取文件
        this.modal.show();
        // 兜底：若 shown 事件因時序/動畫被略過，也用 0ms 檢查一次
        setTimeout(() => {
            ensureModalShown();
            tryInit();
        }, 0);
        
        // 使用壓縮函數處理文件（如果需要）
        compressImageIfNeeded(file, (processedFile) => {
            reader.readAsDataURL(processedFile);
        });
    }

    // 清理
    destroy() {
        // 清理 Cropper 實例
        if (this.cropper) {
            try {
                this.cropper.destroy();
            } catch (e) {
                console.warn('[Crop] Error destroying cropper:', e);
            }
            this.cropper = null;
        }
        
        // 清理 Modal
        if (this.modal) {
            try {
                this.modal.dispose();
            } catch (e) {
                console.warn('[Crop] Error disposing modal:', e);
            }
            this.modal = null;
        }
        
        // 清理事件監聽器
        if (this._confirmHandler) {
            const cropConfirmBtn = document.getElementById('cropConfirmBtn');
            if (cropConfirmBtn) {
                try {
                    cropConfirmBtn.removeEventListener('click', this._confirmHandler);
                    cropConfirmBtn.removeEventListener('touchend', this._confirmHandler);
                } catch (e) {
                    console.warn('[Crop] Error removing event listeners in destroy:', e);
                }
            }
            this._confirmHandler = null;
        }
        
        // 清理 canvas 和 blob
        if (this._currentCanvas) {
            try {
                const ctx = this._currentCanvas.getContext('2d');
                if (ctx) {
                    ctx.clearRect(0, 0, this._currentCanvas.width, this._currentCanvas.height);
                }
            } catch (e) {
                console.warn('[Crop] Error clearing canvas in destroy:', e);
            }
            this._currentCanvas = null;
        }
        
        if (this._currentBlob) {
            try {
                URL.revokeObjectURL(URL.createObjectURL(this._currentBlob));
            } catch (e) {
                console.warn('[Crop] Error revoking blob in destroy:', e);
            }
            this._currentBlob = null;
        }
        
        // 重置處理標誌
        this._isProcessing = false;
        
        // 清理 ResizeObserver
        if (this._resizeObserver) {
            try {
                this._resizeObserver.disconnect();
            } catch (e) {
                console.warn('[Crop] Error disconnecting ResizeObserver:', e);
            }
            this._resizeObserver = null;
        }
        
        // 清理定時器
        if (this._rebuildTimer) {
            try {
                clearTimeout(this._rebuildTimer);
            } catch (e) {
                console.warn('[Crop] Error clearing rebuild timer:', e);
            }
            this._rebuildTimer = null;
        }
        
        this._isRebuilding = false;
        this._restoreFocusTo = null;
    }

    // 去背景功能（使用 Canvas API 進行簡單的背景移除）
    async removeBackground(canvas) {
        return new Promise((resolve, reject) => {
            try {
                const ctx = canvas.getContext('2d');
                const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
                const data = imageData.data;
                
                // 獲取四個角的顏色作為背景色參考
                const getPixel = (x, y) => {
                    const idx = (y * canvas.width + x) * 4;
                    return {
                        r: data[idx],
                        g: data[idx + 1],
                        b: data[idx + 2],
                        a: data[idx + 3]
                    };
                };
                
                const corners = [
                    getPixel(0, 0), // 左上角
                    getPixel(canvas.width - 1, 0), // 右上角
                    getPixel(0, canvas.height - 1), // 左下角
                    getPixel(canvas.width - 1, canvas.height - 1) // 右下角
                ];
                
                // 計算平均背景色
                let avgR = 0, avgG = 0, avgB = 0;
                corners.forEach(pixel => {
                    avgR += pixel.r;
                    avgG += pixel.g;
                    avgB += pixel.b;
                });
                avgR = Math.floor(avgR / corners.length);
                avgG = Math.floor(avgG / corners.length);
                avgB = Math.floor(avgB / corners.length);
                
                // 顏色相似度閾值
                const threshold = 40;
                
                // 處理每個像素
                for (let i = 0; i < data.length; i += 4) {
                    const r = data[i];
                    const g = data[i + 1];
                    const b = data[i + 2];
                    
                    // 計算與背景色的距離
                    const distance = Math.sqrt(
                        Math.pow(r - avgR, 2) + 
                        Math.pow(g - avgG, 2) + 
                        Math.pow(b - avgB, 2)
                    );
                    
                    // 如果顏色與背景色相似，設為透明
                    if (distance < threshold) {
                        data[i + 3] = 0; // 設置 alpha 為 0（透明）
                    }
                }
                
                // 創建新的 canvas 並應用處理後的數據
                const newCanvas = document.createElement('canvas');
                newCanvas.width = canvas.width;
                newCanvas.height = canvas.height;
                const newCtx = newCanvas.getContext('2d');
                newCtx.putImageData(imageData, 0, 0);
                
                resolve(newCanvas);
            } catch (error) {
                reject(error);
            }
        });
    }
}

// 確保 ImageCropper 在全局作用域可用
if (typeof window !== 'undefined') {
    window.ImageCropper = ImageCropper;
}

})();

