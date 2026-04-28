// AI 表單輸入組件（獨立文件，不依賴 dynamic-form.js）
(function() {
    'use strict';

    // 檢查是否已經加載
    if (window.AIFormInput) {
        return;
    }

    class AIFormInput {
        constructor() {
            this.currentForm = null;
            this.fieldList = [];
            this.matchedResults = [];
            this.originalText = '';
            this.availableFields = [];
            this.mediaRecorder = null;
            this.audioChunks = [];
            this.isRecording = false;
            this.recordingTimer = null;
            this.recordingStartTime = null;
            this.shouldProcessSTT = true; // 標記是否應該處理 STT
        }

        // 檢查是否在表單頁面
        isFormPage() {
            const path = window.location.pathname;
            
            // 排除頁面編輯器頁面（/pages/new, /pages/{id}/edit）
            if (path.startsWith('/pages/')) {
                return false;
            }
            
            // 方法1: 檢查 URL 路徑（最可靠）
            // 必須包含 /new, /edit, /create, /update 等表單路徑
            const formPaths = ['/new', '/edit', '/create', '/update'];
            const hasFormPath = formPaths.some(p => {
                // 確保是完整路徑匹配，避免誤判（例如 /news 不應該匹配）
                const index = path.indexOf(p);
                if (index === -1) return false;
                // 檢查後面是否跟著 / 或結尾，確保是完整單詞
                const nextChar = path[index + p.length];
                return !nextChar || nextChar === '/';
            });
            
            if (hasFormPath) {
                return true;
            }
            
            // 方法2: 檢查是否有動態表單容器（dynamic-form.js 生成的）
            // 這個容器只在真正的表單頁面出現
            if (document.getElementById('dynamicFormContainer')) {
                return true;
            }
            
            // 方法3: 檢查特定的表單 ID（更嚴格）
            // 只檢查明確的表單 ID，不檢查所有 form 元素
            const specificFormIds = [
                'orderForm', 'productForm', 'customerForm', 'invoiceForm',
                'quotationForm', 'serviceOrderForm', 'purchaseOrderForm',
                'projectForm', 'payrollForm', 'enterpriseForm'
            ];
            
            const hasSpecificForm = specificFormIds.some(formId => {
                return document.getElementById(formId) !== null;
            });
            
            if (hasSpecificForm) {
                return true;
            }
            
            // 如果以上都不符合，則不是表單頁面
            return false;
        }

        // 初始化：在表單頁面時，在 vAi 按鈕上添加小按鈕
        init() {
            // 等待 DOM 加載完成
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', () => {
                    this.setupFloatingButtons();
                    setTimeout(() => this.initTooltips(), 500);
                });
            } else {
                this.setupFloatingButtons();
                setTimeout(() => this.initTooltips(), 500);
            }
        }

        // 移除小按鈕（當不在表單頁面時）
        removeFloatingButtons() {
            // 移除透明占位 div（會自動清理事件監聽器）
            const imagePlaceholder = document.getElementById('aiFormInputImagePlaceholder');
            const audioPlaceholder = document.getElementById('aiFormInputAudioPlaceholder');
            if (imagePlaceholder) {
                if (imagePlaceholder._cleanup) {
                    imagePlaceholder._cleanup();
                }
                imagePlaceholder.remove();
            }
            if (audioPlaceholder) {
                if (audioPlaceholder._cleanup) {
                    audioPlaceholder._cleanup();
                }
                audioPlaceholder.remove();
            }
        }

        // 在 vAi 浮動按鈕上設置小按鈕
        setupFloatingButtons() {
            // 先檢查是否在表單頁面
            const isForm = this.isFormPage();
            
            // 如果不在表單頁面，移除小按鈕
            if (!isForm) {
                this.removeFloatingButtons();
                return;
            }

            // 等待 vAi 按鈕出現
            const checkVAiButton = () => {
                const vaiBtn = document.getElementById('aiChatFloatingBtn');
                if (!vaiBtn) {
                    // 如果還沒出現，稍後再試
                    setTimeout(checkVAiButton, 500);
                    return;
                }

                // 檢查是否已經添加了小按鈕
                if (document.getElementById('aiFormInputImagePlaceholder') || document.getElementById('aiFormInputAudioPlaceholder')) {
                    return;
                }

                // 獲取 vAi 按鈕的位置和樣式
                const vaiStyles = window.getComputedStyle(vaiBtn);
                const vaiRect = vaiBtn.getBoundingClientRect();
                
                // 計算 bottom 和 right（從 CSS 或計算得出）
                const getBottom = () => {
                    const bottom = vaiStyles.bottom;
                    if (bottom && bottom !== 'auto') {
                        return bottom;
                    }
                    return (window.innerHeight - vaiRect.bottom) + 'px';
                };
                
                const getRight = () => {
                    const right = vaiStyles.right;
                    if (right && right !== 'auto') {
                        return right;
                    }
                    return (window.innerWidth - vaiRect.right) + 'px';
                };
                
                // 創建透明占位 div（圖片按鈕）
                const imagePlaceholder = document.createElement('div');
                imagePlaceholder.id = 'aiFormInputImagePlaceholder';
                imagePlaceholder.style.cssText = `
                    position: fixed;
                    bottom: ${getBottom()};
                    right: ${getRight()};
                    width: ${vaiBtn.offsetWidth}px;
                    height: ${vaiBtn.offsetHeight}px;
                    pointer-events: none;
                    z-index: 999;
                `;
                
                // 創建透明占位 div（語音按鈕）
                const audioPlaceholder = document.createElement('div');
                audioPlaceholder.id = 'aiFormInputAudioPlaceholder';
                audioPlaceholder.style.cssText = `
                    position: fixed;
                    bottom: ${getBottom()};
                    right: ${getRight()};
                    width: ${vaiBtn.offsetWidth}px;
                    height: ${vaiBtn.offsetHeight}px;
                    pointer-events: none;
                    z-index: 999;
                `;

                // 圖片 OCR 小按鈕（放在透明占位 div 中）
                const imageBtn = document.createElement('button');
                imageBtn.type = 'button';
                imageBtn.className = 'btn btn-sm btn-primary ai-form-input-btn ai-form-input-btn-image';
                imageBtn.innerHTML = '<i class="bi bi-image"></i>';
                imageBtn.title = 'AI 圖片輸入資料';
                imageBtn.setAttribute('data-bs-toggle', 'tooltip');
                imageBtn.setAttribute('data-bs-placement', 'left');
                imageBtn.setAttribute('data-i18n-title', 'aiInput.ocrImage');
                imageBtn.style.pointerEvents = 'auto';
                
                // 隱藏 tooltip 的輔助函數（移動端需要強制清理）
                const hideImageTooltip = () => {
                    if (typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
                        const tooltip = bootstrap.Tooltip.getInstance(imageBtn);
                        if (tooltip) {
                            try {
                                tooltip.hide();
                            } catch (e) {
                                console.warn('[AIForm] Error hiding image tooltip:', e);
                            }
                        }
                    }
                    // 強制移除 tooltip DOM 元素（移動端可能殘留）
                    setTimeout(() => {
                        const tooltipElements = document.querySelectorAll('.tooltip');
                        tooltipElements.forEach(el => {
                            // 檢查是否屬於這個按鈕的 tooltip
                            const tooltipId = el.getAttribute('id');
                            if (tooltipId && tooltipId.includes('image') || 
                                (el.textContent && el.textContent.includes('圖片'))) {
                                try {
                                    el.remove();
                                } catch (e) {
                                    // ignore
                                }
                            }
                        });
                    }, 50);
                };
                
                imageBtn.onclick = (e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    hideImageTooltip();
                    // 延遲執行，確保 tooltip 已隱藏
                    setTimeout(() => {
                        this.handleImageInput();
                    }, 100);
                };
                
                // 添加觸摸事件（移動端）- 立即隱藏 tooltip
                imageBtn.addEventListener('touchstart', (e) => {
                    e.stopPropagation();
                    hideImageTooltip();
                }, { passive: true });
                imageBtn.addEventListener('touchend', (e) => {
                    e.stopPropagation();
                    hideImageTooltip();
                }, { passive: true });
                imageBtn.addEventListener('touchcancel', (e) => {
                    e.stopPropagation();
                    hideImageTooltip();
                }, { passive: true });

                // 語音 STT 小按鈕（放在透明占位 div 中）
                const audioBtn = document.createElement('button');
                audioBtn.type = 'button';
                audioBtn.className = 'btn btn-sm btn-info ai-form-input-btn ai-form-input-btn-audio';
                audioBtn.innerHTML = '<i class="bi bi-mic"></i>';
                audioBtn.title = 'AI 語音輸入資料';
                audioBtn.setAttribute('data-bs-toggle', 'tooltip');
                audioBtn.setAttribute('data-bs-placement', 'left');
                audioBtn.setAttribute('data-i18n-title', 'aiInput.sttAudio');
                audioBtn.style.pointerEvents = 'auto';
                
                // 隱藏 tooltip 的輔助函數（移動端需要強制清理）
                const hideAudioTooltip = () => {
                    if (typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
                        const tooltip = bootstrap.Tooltip.getInstance(audioBtn);
                        if (tooltip) {
                            try {
                                tooltip.hide();
                            } catch (e) {
                                console.warn('[AIForm] Error hiding audio tooltip:', e);
                            }
                        }
                    }
                    // 強制移除 tooltip DOM 元素（移動端可能殘留）
                    setTimeout(() => {
                        const tooltipElements = document.querySelectorAll('.tooltip');
                        tooltipElements.forEach(el => {
                            // 檢查是否屬於這個按鈕的 tooltip
                            const tooltipId = el.getAttribute('id');
                            if (tooltipId && tooltipId.includes('audio') || 
                                (el.textContent && el.textContent.includes('語音'))) {
                                try {
                                    el.remove();
                                } catch (e) {
                                    // ignore
                                }
                            }
                        });
                    }, 50);
                };
                
                audioBtn.onclick = (e) => {
                    e.stopPropagation();
                    e.preventDefault();
                    hideAudioTooltip();
                    // 延遲執行，確保 tooltip 已隱藏
                    setTimeout(() => {
                        this.handleAudioInput();
                    }, 100);
                };
                
                // 添加觸摸事件（移動端）- 立即隱藏 tooltip
                audioBtn.addEventListener('touchstart', (e) => {
                    e.stopPropagation();
                    hideAudioTooltip();
                }, { passive: true });
                audioBtn.addEventListener('touchend', (e) => {
                    e.stopPropagation();
                    hideAudioTooltip();
                }, { passive: true });
                audioBtn.addEventListener('touchcancel', (e) => {
                    e.stopPropagation();
                    hideAudioTooltip();
                }, { passive: true });

                // 將按鈕添加到占位 div
                imagePlaceholder.appendChild(imageBtn);
                audioPlaceholder.appendChild(audioBtn);
                
                // 將占位 div 添加到 body
                document.body.appendChild(imagePlaceholder);
                document.body.appendChild(audioPlaceholder);
                
                // 初始化 tooltips（等待 DOM 更新和 i18n 翻譯完成）
                const initTooltipsForButtons = () => {
                    if (typeof bootstrap === 'undefined' || !bootstrap.Tooltip) {
                        return;
                    }
                    
                    // 確保 title 屬性已設置（如果 i18n 已更新，使用更新後的值）
                    if (typeof I18n !== 'undefined' && I18n.t) {
                        const imageTitle = I18n.t('aiInput.ocrImage', 'AI 圖片輸入資料');
                        const audioTitle = I18n.t('aiInput.sttAudio', 'AI 語音輸入資料');
                        if (imageTitle && imageTitle !== 'aiInput.ocrImage') {
                            imageBtn.setAttribute('title', imageTitle);
                        }
                        if (audioTitle && audioTitle !== 'aiInput.sttAudio') {
                            audioBtn.setAttribute('title', audioTitle);
                        }
                    }
                    
                    // 如果已經有 tooltip 實例，先銷毀
                    const existingImageTooltip = bootstrap.Tooltip.getInstance(imageBtn);
                    if (existingImageTooltip) {
                        existingImageTooltip.dispose();
                    }
                    const existingAudioTooltip = bootstrap.Tooltip.getInstance(audioBtn);
                    if (existingAudioTooltip) {
                        existingAudioTooltip.dispose();
                    }
                    
                    // 檢測是否為移動設備
                    const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
                    
                    // 創建新的 tooltip
                    // 移動端：禁用自動觸發，只在需要時手動顯示
                    const imageTooltip = new bootstrap.Tooltip(imageBtn, {
                        title: imageBtn.getAttribute('title') || 'AI 圖片輸入資料',
                        placement: 'left',
                        trigger: isMobile ? 'manual' : 'hover',
                        fallbackPlacements: ['right', 'top', 'bottom']
                    });
                    
                    const audioTooltip = new bootstrap.Tooltip(audioBtn, {
                        title: audioBtn.getAttribute('title') || 'AI 語音輸入資料',
                        placement: 'left',
                        trigger: isMobile ? 'manual' : 'hover',
                        fallbackPlacements: ['right', 'top', 'bottom']
                    });
                    
                    // 移動端：長按顯示 tooltip，點擊立即隱藏
                    if (isMobile) {
                        let imageTooltipTimeout = null;
                        let audioTooltipTimeout = null;
                        
                        // 圖片按鈕：長按顯示 tooltip
                        imageBtn.addEventListener('touchstart', () => {
                            imageTooltipTimeout = setTimeout(() => {
                                imageTooltip.show();
                            }, 500); // 長按 500ms 顯示
                        }, { passive: true });
                        
                        imageBtn.addEventListener('touchend', () => {
                            if (imageTooltipTimeout) {
                                clearTimeout(imageTooltipTimeout);
                                imageTooltipTimeout = null;
                            }
                            imageTooltip.hide();
                            // 強制清理 tooltip DOM
                            setTimeout(() => {
                                const tooltipElements = document.querySelectorAll('.tooltip');
                                tooltipElements.forEach(el => {
                                    try {
                                        el.remove();
                                    } catch (e) {
                                        // ignore
                                    }
                                });
                            }, 50);
                        }, { passive: true });
                        
                        imageBtn.addEventListener('touchcancel', () => {
                            if (imageTooltipTimeout) {
                                clearTimeout(imageTooltipTimeout);
                                imageTooltipTimeout = null;
                            }
                            imageTooltip.hide();
                        }, { passive: true });
                        
                        // 語音按鈕：長按顯示 tooltip
                        audioBtn.addEventListener('touchstart', () => {
                            audioTooltipTimeout = setTimeout(() => {
                                audioTooltip.show();
                            }, 500); // 長按 500ms 顯示
                        }, { passive: true });
                        
                        audioBtn.addEventListener('touchend', () => {
                            if (audioTooltipTimeout) {
                                clearTimeout(audioTooltipTimeout);
                                audioTooltipTimeout = null;
                            }
                            audioTooltip.hide();
                            // 強制清理 tooltip DOM
                            setTimeout(() => {
                                const tooltipElements = document.querySelectorAll('.tooltip');
                                tooltipElements.forEach(el => {
                                    try {
                                        el.remove();
                                    } catch (e) {
                                        // ignore
                                    }
                                });
                            }, 50);
                        }, { passive: true });
                        
                        audioBtn.addEventListener('touchcancel', () => {
                            if (audioTooltipTimeout) {
                                clearTimeout(audioTooltipTimeout);
                                audioTooltipTimeout = null;
                            }
                            audioTooltip.hide();
                        }, { passive: true });
                    }
                };
                
                // 延遲初始化，確保 i18n 有時間更新
                setTimeout(initTooltipsForButtons, 200);
                
                // 監聽窗口大小變化，更新占位 div 位置
                const updatePlaceholderPositions = () => {
                    const newRect = vaiBtn.getBoundingClientRect();
                    const newStyles = window.getComputedStyle(vaiBtn);
                    
                    // 計算新的 bottom 和 right
                    const newBottom = newStyles.bottom && newStyles.bottom !== 'auto' 
                        ? newStyles.bottom 
                        : (window.innerHeight - newRect.bottom) + 'px';
                    const newRight = newStyles.right && newStyles.right !== 'auto'
                        ? newStyles.right
                        : (window.innerWidth - newRect.right) + 'px';
                    
                    imagePlaceholder.style.bottom = newBottom;
                    imagePlaceholder.style.right = newRight;
                    audioPlaceholder.style.bottom = newBottom;
                    audioPlaceholder.style.right = newRight;
                    
                    // 更新寬高（以防按鈕大小變化）
                    imagePlaceholder.style.width = vaiBtn.offsetWidth + 'px';
                    imagePlaceholder.style.height = vaiBtn.offsetHeight + 'px';
                    audioPlaceholder.style.width = vaiBtn.offsetWidth + 'px';
                    audioPlaceholder.style.height = vaiBtn.offsetHeight + 'px';
                };
                
                // 使用防抖避免頻繁更新
                let resizeTimeout;
                const debouncedUpdate = () => {
                    clearTimeout(resizeTimeout);
                    resizeTimeout = setTimeout(updatePlaceholderPositions, 50);
                };
                
                // 監聽 resize 和 scroll
                window.addEventListener('resize', debouncedUpdate);
                window.addEventListener('scroll', debouncedUpdate, { passive: true });
                
                // 存儲清理函數
                imagePlaceholder._cleanup = () => {
                    window.removeEventListener('resize', updatePlaceholderPositions);
                    window.removeEventListener('scroll', updatePlaceholderPositions);
                };
                
                console.log('AIFormInput: Added small buttons in transparent placeholders');
                
                // 觸發 i18n 翻譯（如果有的話）
                if (typeof I18n !== 'undefined' && I18n.updatePage) {
                    setTimeout(() => {
                        I18n.updatePage();
                    }, 100);
                }
                
                // 初始化所有按鈕的 tooltip
                this.initTooltips();
            };

            checkVAiButton();
        }
        
        // 初始化 tooltips
        initTooltips() {
            if (typeof bootstrap === 'undefined' || !bootstrap.Tooltip) {
                return;
            }
            
            // 初始化 vAi 主按鈕的 tooltip
            const vaiBtn = document.getElementById('aiChatFloatingBtn');
            if (vaiBtn) {
                const existingTooltip = bootstrap.Tooltip.getInstance(vaiBtn);
                if (existingTooltip) {
                    existingTooltip.dispose();
                }
                
                // 檢測是否為移動設備
                const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
                
                const vaiTooltip = new bootstrap.Tooltip(vaiBtn, {
                    trigger: isMobile ? 'manual' : 'hover',
                    fallbackPlacements: ['left', 'top', 'bottom']
                });
                
                // 移動端：長按顯示 tooltip，點擊立即隱藏
                if (isMobile) {
                    let vaiTooltipTimeout = null;
                    
                    vaiBtn.addEventListener('touchstart', () => {
                        vaiTooltipTimeout = setTimeout(() => {
                            vaiTooltip.show();
                        }, 500); // 長按 500ms 顯示
                    }, { passive: true });
                    
                    vaiBtn.addEventListener('touchend', () => {
                        if (vaiTooltipTimeout) {
                            clearTimeout(vaiTooltipTimeout);
                            vaiTooltipTimeout = null;
                        }
                        vaiTooltip.hide();
                        // 強制清理 tooltip DOM
                        setTimeout(() => {
                            const tooltipElements = document.querySelectorAll('.tooltip');
                            tooltipElements.forEach(el => {
                                try {
                                    el.remove();
                                } catch (e) {
                                    // ignore
                                }
                            });
                        }, 50);
                    }, { passive: true });
                    
                    vaiBtn.addEventListener('touchcancel', () => {
                        if (vaiTooltipTimeout) {
                            clearTimeout(vaiTooltipTimeout);
                            vaiTooltipTimeout = null;
                        }
                        vaiTooltip.hide();
                    }, { passive: true });
                }
            }
            
            // 初始化小按鈕的 tooltips（現在在透明占位 div 中）
            const imagePlaceholder = document.getElementById('aiFormInputImagePlaceholder');
            const audioPlaceholder = document.getElementById('aiFormInputAudioPlaceholder');
            const imageBtn = imagePlaceholder?.querySelector('.ai-form-input-btn-image');
            const audioBtn = audioPlaceholder?.querySelector('.ai-form-input-btn-audio');
            
            if (imageBtn) {
                const existingTooltip = bootstrap.Tooltip.getInstance(imageBtn);
                if (existingTooltip) {
                    existingTooltip.dispose();
                }
                // 確保 title 屬性存在
                if (!imageBtn.getAttribute('title')) {
                    imageBtn.setAttribute('title', 'AI 圖片輸入資料');
                }
                // 如果 i18n 已更新，使用更新後的值
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const i18nTitle = I18n.t('aiInput.ocrImage', 'AI 圖片輸入資料');
                    if (i18nTitle && i18nTitle !== 'aiInput.ocrImage') {
                        imageBtn.setAttribute('title', i18nTitle);
                    }
                }
                
                // 檢測是否為移動設備
                const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
                
                const imageTooltip = new bootstrap.Tooltip(imageBtn, {
                    title: imageBtn.getAttribute('title') || 'AI 圖片輸入資料',
                    placement: 'left',
                    trigger: isMobile ? 'manual' : 'hover',
                    fallbackPlacements: ['right', 'top', 'bottom']
                });
                
                // 移動端：長按顯示 tooltip，點擊立即隱藏
                if (isMobile) {
                    let imageTooltipTimeout = null;
                    
                    imageBtn.addEventListener('touchstart', () => {
                        imageTooltipTimeout = setTimeout(() => {
                            imageTooltip.show();
                        }, 500);
                    }, { passive: true });
                    
                    imageBtn.addEventListener('touchend', () => {
                        if (imageTooltipTimeout) {
                            clearTimeout(imageTooltipTimeout);
                            imageTooltipTimeout = null;
                        }
                        imageTooltip.hide();
                        setTimeout(() => {
                            const tooltipElements = document.querySelectorAll('.tooltip');
                            tooltipElements.forEach(el => {
                                try {
                                    el.remove();
                                } catch (e) {
                                    // ignore
                                }
                            });
                        }, 50);
                    }, { passive: true });
                    
                    imageBtn.addEventListener('touchcancel', () => {
                        if (imageTooltipTimeout) {
                            clearTimeout(imageTooltipTimeout);
                            imageTooltipTimeout = null;
                        }
                        imageTooltip.hide();
                    }, { passive: true });
                }
            }
            
            if (audioBtn) {
                const existingTooltip = bootstrap.Tooltip.getInstance(audioBtn);
                if (existingTooltip) {
                    existingTooltip.dispose();
                }
                // 確保 title 屬性存在
                if (!audioBtn.getAttribute('title')) {
                    audioBtn.setAttribute('title', 'AI 語音輸入資料');
                }
                // 如果 i18n 已更新，使用更新後的值
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const i18nTitle = I18n.t('aiInput.sttAudio', 'AI 語音輸入資料');
                    if (i18nTitle && i18nTitle !== 'aiInput.sttAudio') {
                        audioBtn.setAttribute('title', i18nTitle);
                    }
                }
                
                // 檢測是否為移動設備
                const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
                
                const audioTooltip = new bootstrap.Tooltip(audioBtn, {
                    title: audioBtn.getAttribute('title') || 'AI 語音輸入資料',
                    placement: 'left',
                    trigger: isMobile ? 'manual' : 'hover',
                    fallbackPlacements: ['right', 'top', 'bottom']
                });
                
                // 移動端：長按顯示 tooltip，點擊立即隱藏
                if (isMobile) {
                    let audioTooltipTimeout = null;
                    
                    audioBtn.addEventListener('touchstart', () => {
                        audioTooltipTimeout = setTimeout(() => {
                            audioTooltip.show();
                        }, 500);
                    }, { passive: true });
                    
                    audioBtn.addEventListener('touchend', () => {
                        if (audioTooltipTimeout) {
                            clearTimeout(audioTooltipTimeout);
                            audioTooltipTimeout = null;
                        }
                        audioTooltip.hide();
                        setTimeout(() => {
                            const tooltipElements = document.querySelectorAll('.tooltip');
                            tooltipElements.forEach(el => {
                                try {
                                    el.remove();
                                } catch (e) {
                                    // ignore
                                }
                            });
                        }, 50);
                    }, { passive: true });
                    
                    audioBtn.addEventListener('touchcancel', () => {
                        if (audioTooltipTimeout) {
                            clearTimeout(audioTooltipTimeout);
                            audioTooltipTimeout = null;
                        }
                        audioTooltip.hide();
                    }, { passive: true });
                }
            }
        }

        // 處理圖片輸入
        async handleImageInput() {
            // 隱藏圖片按鈕的 tooltip
            const imagePlaceholder = document.getElementById('aiFormInputImagePlaceholder');
            const imageBtn = imagePlaceholder?.querySelector('.ai-form-input-btn-image');
            if (imageBtn && typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
                const tooltip = bootstrap.Tooltip.getInstance(imageBtn);
                if (tooltip) {
                    tooltip.hide();
                }
            }
            
            // 創建文件輸入
            const input = document.createElement('input');
            input.type = 'file';
            input.accept = 'image/*';
            input.onchange = async (e) => {
                const file = e.target.files[0];
                if (!file) return;

                // 使用現有的 ImageCropper 進行裁剪
                if (typeof ImageCropper !== 'undefined') {
                    const cropper = new ImageCropper({
                        aspectRatio: NaN, // 不限制比例
                        viewMode: 1
                    });

                    cropper.showCropModal(file, async (croppedFile) => {
                        await this.processOCR(croppedFile);
                    }, {
                        onCancel: () => {
                            // 用戶取消裁剪
                        }
                    });
                } else {
                    // 如果沒有 ImageCropper，直接處理
                    await this.processOCR(file);
                }
            };
            input.click();
        }

        // 處理語音輸入
        async handleAudioInput() {
            // 隱藏語音按鈕的 tooltip
            const audioPlaceholder = document.getElementById('aiFormInputAudioPlaceholder');
            const audioBtn = audioPlaceholder?.querySelector('.ai-form-input-btn-audio');
            if (audioBtn && typeof bootstrap !== 'undefined' && bootstrap.Tooltip) {
                const tooltip = bootstrap.Tooltip.getInstance(audioBtn);
                if (tooltip) {
                    tooltip.hide();
                }
            }
            
            if (this.isRecording) {
                // 停止錄音
                this.stopRecording();
            } else {
                // 開始錄音
                this.startRecording();
            }
        }

        // 開始錄音
        async startRecording() {
            try {
                const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
                this.mediaRecorder = new MediaRecorder(stream);
                this.audioChunks = [];
                this.shouldProcessSTT = true; // 重置標記

                this.mediaRecorder.ondataavailable = (event) => {
                    if (event.data.size > 0) {
                        this.audioChunks.push(event.data);
                    }
                };

                this.mediaRecorder.onstop = async () => {
                    stream.getTracks().forEach(track => track.stop());
                    // 只有在 shouldProcessSTT 為 true 時才處理 STT
                    if (this.shouldProcessSTT) {
                        const audioBlob = new Blob(this.audioChunks, { type: 'audio/webm' });
                        await this.processSTT(audioBlob);
                    } else {
                        // 用戶直接關閉了 popup，不處理 STT
                        console.log('用戶取消了錄音，不處理 STT');
                    }
                };

                this.mediaRecorder.start();
                this.isRecording = true;
                this.recordingStartTime = Date.now();
                this.showRecordingModal();
                
                // 設置 60 秒自動停止
                this.recordingTimer = setTimeout(() => {
                    if (this.isRecording) {
                        this.stopRecording();
                    }
                }, 60000); // 60 秒
            } catch (error) {
                console.error('錄音失敗:', error);
                if (typeof App !== 'undefined' && App.showAlert) {
                    App.showAlert('無法訪問麥克風，請檢查瀏覽器權限設置', 'danger');
                } else {
                    alert('無法訪問麥克風，請檢查瀏覽器權限設置');
                }
            }
        }

        // 停止錄音
        stopRecording() {
            if (this.mediaRecorder && this.isRecording) {
                // 清除計時器
                if (this.recordingTimer) {
                    clearTimeout(this.recordingTimer);
                    this.recordingTimer = null;
                }
                // 清除倒計時更新
                if (this.countdownInterval) {
                    clearInterval(this.countdownInterval);
                    this.countdownInterval = null;
                }
                this.mediaRecorder.stop();
                this.isRecording = false;
                this.hideRecordingModal();
            }
        }

        // 顯示錄音模態框
        showRecordingModal() {
            let modal = document.getElementById('aiRecordingModal');
            if (!modal) {
                modal = document.createElement('div');
                modal.id = 'aiRecordingModal';
                modal.className = 'modal fade';
                modal.innerHTML = `
                    <div class="modal-dialog modal-dialog-centered">
                        <div class="modal-content">
                            <div class="modal-header">
                                <h5 class="modal-title">正在錄音...</h5>
                                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                            </div>
                            <div class="modal-body text-center">
                                <div class="mb-3">
                                    <i class="bi bi-mic-fill text-danger" style="font-size: 3rem; animation: pulse 1s infinite;"></i>
                                </div>
                                <p id="recordingStatusText">正在錄音中，點擊停止按鈕結束錄音</p>
                                <div id="recordingCountdown" class="mt-2" style="display: none;">
                                    <span class="badge bg-warning text-dark" style="font-size: 1.2rem;">剩餘時間: <span id="countdownSeconds">10</span> 秒</span>
                                </div>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-danger" id="stopRecordingBtn">
                                    <i class="bi bi-stop-fill"></i> 停止錄音
                                </button>
                            </div>
                        </div>
                    </div>
                `;
                document.body.appendChild(modal);
                
                document.getElementById('stopRecordingBtn').onclick = () => {
                    this.stopRecording();
                };
                
                // 監聽 modal 關閉事件（用戶直接關閉 popup）
                modal.addEventListener('hide.bs.modal', () => {
                    // 如果用戶直接關閉 modal（不是通過停止按鈕），設置標記不處理 STT
                    if (this.isRecording) {
                        this.shouldProcessSTT = false;
                        // 停止錄音
                        if (this.mediaRecorder && this.isRecording) {
                            if (this.recordingTimer) {
                                clearTimeout(this.recordingTimer);
                                this.recordingTimer = null;
                            }
                            if (this.countdownInterval) {
                                clearInterval(this.countdownInterval);
                                this.countdownInterval = null;
                            }
                            this.mediaRecorder.stop();
                            this.isRecording = false;
                        }
                    }
                }, { once: false });
                
                // 移動端：監聽 modal 關閉事件，強制恢復滾動
                const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
                
                // 移動端：使用 MutationObserver 監控 body 樣式變化
                let bodyObserver = null;
                if (isMobile) {
                    bodyObserver = new MutationObserver(() => {
                        if (!modal.classList.contains('show') && document.body.classList.contains('modal-open')) {
                            document.body.classList.remove('modal-open');
                            document.body.style.paddingRight = '';
                            document.body.style.overflow = '';
                            document.body.style.position = '';
                            if (document.documentElement) {
                                document.documentElement.style.overflow = '';
                                document.documentElement.classList.remove('modal-open');
                            }
                        }
                    });
                    bodyObserver.observe(document.body, {
                        attributes: true,
                        attributeFilter: ['class', 'style']
                    });
                }
                
                const cleanupScroll = () => {
                    const backdrops = document.querySelectorAll('.modal-backdrop');
                    backdrops.forEach(backdrop => {
                        try {
                            backdrop.remove();
                        } catch (e) {
                            console.warn('[AIForm] Error removing backdrop:', e);
                        }
                    });
                    
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
                
                modal.addEventListener('hidden.bs.modal', () => {
                    if (bodyObserver) {
                        bodyObserver.disconnect();
                        bodyObserver = null;
                    }
                    cleanupScroll();
                    setTimeout(cleanupScroll, 50);
                    setTimeout(cleanupScroll, 150);
                    setTimeout(cleanupScroll, 300);
                }, { once: false });
            }
            
            // 更新 modal 內容（如果已存在）
            const statusText = modal.querySelector('#recordingStatusText');
            const countdownDiv = modal.querySelector('#recordingCountdown');
            const countdownSeconds = modal.querySelector('#countdownSeconds');
            
            // 開始倒計時更新（每 100ms 更新一次）
            this.countdownInterval = setInterval(() => {
                if (!this.isRecording || !this.recordingStartTime) {
                    return;
                }
                
                const elapsed = (Date.now() - this.recordingStartTime) / 1000; // 已錄音秒數
                const remaining = 60 - elapsed; // 剩餘秒數
                
                // 當剩餘 10 秒時開始顯示倒計時
                if (remaining <= 10 && remaining > 0) {
                    if (countdownDiv) {
                        countdownDiv.style.display = 'block';
                    }
                    if (countdownSeconds) {
                        countdownSeconds.textContent = Math.ceil(remaining);
                    }
                    if (statusText) {
                        statusText.textContent = `剩餘時間: ${Math.ceil(remaining)} 秒`;
                    }
                } else if (remaining > 10) {
                    if (countdownDiv) {
                        countdownDiv.style.display = 'none';
                    }
                    if (statusText) {
                        statusText.textContent = '正在錄音中，點擊停止按鈕結束錄音';
                    }
                }
            }, 100);
            
            const bsModal = new bootstrap.Modal(modal);
            bsModal.show();
        }

        // 隱藏錄音模態框
        hideRecordingModal() {
            // 清除倒計時更新
            if (this.countdownInterval) {
                clearInterval(this.countdownInterval);
                this.countdownInterval = null;
            }
            
            const modal = document.getElementById('aiRecordingModal');
            if (modal) {
                const bsModal = bootstrap.Modal.getInstance(modal);
                if (bsModal) {
                    bsModal.hide();
                }
            }
        }

        // 處理 OCR
        async processOCR(file) {
            try {
                // 顯示 loading
                if (typeof App !== 'undefined' && App.showLoading) {
                    App.showLoading('正在識別圖片中的文字...');
                }

                const formData = new FormData();
                formData.append('image', file);

                const response = await fetch('/api/v1/ai/ocr', {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('auth_token')}`
                    },
                    body: formData
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.error || 'OCR 處理失敗');
                }

                const result = await response.json();
                if (result.text) {
                    await this.matchFields(result.text);
                } else {
                    throw new Error('未識別到文字');
                }
            } catch (error) {
                console.error('OCR 處理失敗:', error);
                if (typeof App !== 'undefined' && App.hideLoading) {
                    App.hideLoading();
                }
                if (typeof App !== 'undefined' && App.showAlert) {
                    App.showAlert('OCR 處理失敗: ' + error.message, 'danger');
                } else {
                    alert('OCR 處理失敗: ' + error.message);
                }
            }
        }

        // 處理 STT
        async processSTT(audioBlob) {
            try {
                // 顯示 loading
                if (typeof App !== 'undefined' && App.showLoading) {
                    App.showLoading('正在識別語音...');
                }

                // 轉換為 WAV 格式（如果需要）
                const formData = new FormData();
                formData.append('audio', audioBlob, 'recording.webm');

                const response = await fetch('/api/v1/ai/stt', {
                    method: 'POST',
                    headers: {
                        'Authorization': `Bearer ${localStorage.getItem('auth_token')}`
                    },
                    body: formData
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.error || 'STT 處理失敗');
                }

                const result = await response.json();
                if (result.text) {
                    await this.matchFields(result.text);
                } else {
                    throw new Error('未識別到語音');
                }
            } catch (error) {
                console.error('STT 處理失敗:', error);
                if (typeof App !== 'undefined' && App.hideLoading) {
                    App.hideLoading();
                }
                if (typeof App !== 'undefined' && App.showAlert) {
                    App.showAlert('STT 處理失敗: ' + error.message, 'danger');
                } else {
                    alert('STT 處理失敗: ' + error.message);
                }
            }
        }

        // 獲取表單字段列表
        getFieldList() {
            const fields = [];
            
            // 允許的字段類型：包括 input, textarea, html-editor, yes/no, radiobox, checkbox, select2
            const allowedTypes = ['input', 'text', 'textarea', 'html-editor', 'yes-no', 'default-include-single', 'radio', 'radiobox', 'checkbox', 'checkbox-group', 'select2', 'select2-multi', 'relation-select'];
            
            // 嘗試從 dynamic-form 獲取字段配置
            if (window.dynamicForm && window.dynamicForm.config && window.dynamicForm.config.formFields) {
                window.dynamicForm.config.formFields.forEach(field => {
                    // 包括允許的字段類型
                    if (allowedTypes.includes(field.type) || field.type === 'select') {
                        const fieldData = {
                            name: field.name || field.key,
                            label: field.label || field.name || field.key,
                            type: field.type
                        };
                        
                        // 如果是 select2 且有 relationApi，只標記 API 路徑，不載入選項
                        if ((field.type === 'select2' || field.type === 'select2-multi' || field.type === 'relation-select') && field.relationApi) {
                            fieldData.relationApi = field.relationApi;
                            fieldData.relationValueKey = field.relationValueKey || field.relationKey || 'id';
                            fieldData.relationLabelKey = field.relationLabelKey || field.relationLabel || 'name';
                            // 不設置 options，標記為動態字段
                            fieldData.isDynamic = true;
                        }
                        // 如果是 select2 但有固定選項
                        else if ((field.type === 'select2' || field.type === 'select2-multi') && field.options) {
                            fieldData.options = field.options.map(opt => ({
                                value: opt.value,
                                label: opt.label || opt.value
                            }));
                        }
                        // 如果是 select 類型且有選項
                        else if (field.type === 'select' && field.options) {
                            fieldData.options = field.options.map(opt => ({
                                value: opt.value,
                                label: opt.label || opt.value
                            }));
                        }
                        // 如果是 yes/no, radio, checkbox 類型，獲取選項值
                        else if (['yes-no', 'default-include-single', 'radio', 'radiobox', 'checkbox'].includes(field.type) && field.options) {
                            fieldData.options = field.options.map(opt => ({
                                value: opt.value,
                                label: opt.label || opt.value
                            }));
                        }
                        
                        fields.push(fieldData);
                    }
                });
            } else {
                // 從表單中提取字段
                const form = document.querySelector('form[id*="Form"], form[id*="form"]');
                if (form) {
                    // 處理 select 元素（可能是 yes/no, radio, checkbox 的實現）
                    const selects = form.querySelectorAll('select');
                    selects.forEach(select => {
                        const name = select.name || select.id;
                        if (name) {
                            const label = this.getFieldLabel(select);
                            const options = Array.from(select.options).map(opt => ({
                                value: opt.value,
                                label: opt.text
                            }));
                            
                            // 判斷是否為 yes/no 類型（只有 true/false 選項）
                            const isYesNo = options.length === 2 && 
                                           options.some(o => o.value === 'true' || o.value === '1') &&
                                           options.some(o => o.value === 'false' || o.value === '0');
                            
                            if (isYesNo) {
                                fields.push({
                                    name: name,
                                    label: label || name,
                                    type: 'yes-no',
                                    options: options
                                });
                            } else if (options.length > 0) {
                                // 其他 select 類型（radio/select）
                                fields.push({
                                    name: name,
                                    label: label || name,
                                    type: 'select',
                                    options: options
                                });
                            }
                        }
                    });
                    
                    // 處理 checkbox 元素
                    const checkboxes = form.querySelectorAll('input[type="checkbox"]');
                    checkboxes.forEach(checkbox => {
                        const name = checkbox.name || checkbox.id;
                        if (name && !name.endsWith('[]')) {
                            // 單個 checkbox（yes/no）
                            const label = this.getFieldLabel(checkbox);
                            fields.push({
                                name: name,
                                label: label || name,
                                type: 'yes-no',
                                options: [
                                    { value: 'true', label: '是' },
                                    { value: 'false', label: '否' }
                                ]
                            });
                        } else if (name && name.endsWith('[]')) {
                            // checkbox 組（多選）
                            const label = this.getFieldLabel(checkbox);
                            const fieldName = name.replace('[]', '');
                            // 檢查是否已經添加過
                            if (!fields.find(f => f.name === fieldName)) {
                                // 獲取同組的所有 checkbox 選項
                                const groupCheckboxes = form.querySelectorAll(`input[type="checkbox"][name="${name}"]`);
                                const options = Array.from(groupCheckboxes).map(cb => ({
                                    value: cb.value,
                                    label: this.getFieldLabel(cb) || cb.value
                                }));
                                
                                fields.push({
                                    name: fieldName,
                                    label: label || fieldName,
                                    type: 'checkbox',
                                    options: options
                                });
                            }
                        }
                    });
                    
                    // 處理 radio 元素
                    const radios = form.querySelectorAll('input[type="radio"]');
                    const radioGroups = {};
                    radios.forEach(radio => {
                        const name = radio.name || radio.id;
                        if (name) {
                            if (!radioGroups[name]) {
                                radioGroups[name] = {
                                    name: name,
                                    label: this.getFieldLabel(radio) || name,
                                    options: []
                                };
                            }
                            radioGroups[name].options.push({
                                value: radio.value,
                                label: this.getFieldLabel(radio) || radio.value
                            });
                        }
                    });
                    Object.values(radioGroups).forEach(radioGroup => {
                        fields.push({
                            name: radioGroup.name,
                            label: radioGroup.label,
                            type: 'radio',
                            options: radioGroup.options
                        });
                    });
                    
                    // 處理 input 和 textarea 元素
                    const inputs = form.querySelectorAll('input[type="text"], input[type="number"], input[type="email"], input[type="date"], textarea');
                    inputs.forEach(input => {
                        const name = input.name || input.id;
                        const tagName = input.tagName.toLowerCase();
                        const type = input.type || (tagName === 'textarea' ? 'textarea' : 'text');
                        
                        // 只包括 input 和 textarea 元素，排除 hidden, button, submit
                        if (name && 
                            type !== 'hidden' && 
                            type !== 'button' && 
                            type !== 'submit' &&
                            (tagName === 'input' || tagName === 'textarea')) {
                            const label = this.getFieldLabel(input);
                            fields.push({
                                name: name,
                                label: label || name,
                                type: tagName === 'textarea' ? 'textarea' : type
                            });
                        }
                    });
                    
                    // 檢查是否有 html-editor 字段（通過 dynamic-form 生成的）
                    const htmlEditors = form.querySelectorAll('[id$="_editor"]');
                    htmlEditors.forEach(editor => {
                        // 查找對應的 textarea（html-editor 通常有一個隱藏的 textarea）
                        const editorId = editor.id;
                        const textareaId = editorId.replace('_editor', '');
                        const textarea = document.getElementById(textareaId);
                        if (textarea && textarea.name) {
                            // 檢查是否已經添加過（避免重複）
                            if (!fields.find(f => f.name === textarea.name)) {
                                const label = this.getFieldLabel(textarea);
                                fields.push({
                                    name: textarea.name,
                                    label: label || textarea.name,
                                    type: 'html-editor'
                                });
                            }
                        }
                    });
                }
            }

            return fields;
        }

        // 獲取字段標籤
        getFieldLabel(input) {
            // 嘗試從 label 元素獲取
            const id = input.id;
            if (id) {
                const label = document.querySelector(`label[for="${id}"]`);
                if (label) return label.textContent.trim();
            }

            // 嘗試從父元素獲取
            const parent = input.closest('.form-group, .mb-3, .col-md-6, .col-12');
            if (parent) {
                const label = parent.querySelector('label');
                if (label) return label.textContent.trim();
            }

            return null;
        }

        // 匹配字段
        async matchFields(text) {
            try {
                // 更新 loading 訊息
                if (typeof App !== 'undefined' && App.showLoading) {
                    App.showLoading('正在使用 AI 匹配字段...');
                }

                const fieldList = this.getFieldList();
                if (fieldList.length === 0) {
                    throw new Error('未找到表單字段');
                }

                const response = await fetch('/api/v1/ai/match-fields', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': `Bearer ${localStorage.getItem('auth_token')}`
                    },
                    body: JSON.stringify({
                        text: text,
                        field_list: fieldList
                    })
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.error || '字段匹配失敗');
                }

                const result = await response.json();
                // 即使沒有匹配結果，也顯示 popup 讓用戶手動配置
                if (result.matches && result.matches.length > 0) {
                    this.matchedResults = result.matches;
                } else {
                    // 如果沒有匹配結果，創建空的匹配結果，讓用戶手動配置
                    this.matchedResults = [];
                }
                // 保存原始文字和字段列表，用於手動配置
                this.originalText = text;
                this.availableFields = fieldList;
                
                // 隱藏 loading，顯示 popup
                if (typeof App !== 'undefined' && App.hideLoading) {
                    App.hideLoading();
                }
                this.showMatchPreview(text);
            } catch (error) {
                console.error('字段匹配失敗:', error);
                // 即使 API 調用失敗，也顯示 popup 讓用戶手動配置
                this.matchedResults = [];
                this.originalText = text;
                this.availableFields = this.getFieldList();
                
                // 隱藏 loading，顯示 popup
                if (typeof App !== 'undefined' && App.hideLoading) {
                    App.hideLoading();
                }
                this.showMatchPreview(text);
            }
        }

        // 顯示匹配結果預覽
        showMatchPreview(originalText) {
            // 清理所有残留的 modal-backdrop（防止与图片裁剪 modal 的 overlay 重叠）
            const existingBackdrops = document.querySelectorAll('.modal-backdrop');
            existingBackdrops.forEach(backdrop => {
                try {
                    backdrop.remove();
                } catch (e) {
                    console.warn('[AIForm] Error removing backdrop:', e);
                }
            });
            
            // 确保 body 的 modal-open class 被移除（如果有残留）
            if (document.body.classList.contains('modal-open')) {
                document.body.classList.remove('modal-open');
                // 移除可能残留的 padding-right（Bootstrap modal 添加的）
                document.body.style.paddingRight = '';
            }
            
            let modal = document.getElementById('aiMatchPreviewModal');
            if (!modal) {
                modal = document.createElement('div');
                modal.id = 'aiMatchPreviewModal';
                modal.className = 'modal fade';
                // 设置更高的 z-index 确保在图片裁剪 modal 之上
                modal.style.zIndex = '1060';
                document.body.appendChild(modal);
            } else {
                // 如果已存在，也设置 z-index
                modal.style.zIndex = '1060';
            }

            modal.innerHTML = `
                <div class="modal-dialog modal-lg modal-dialog-scrollable">
                    <div class="modal-content">
                        <div class="modal-header">
                            <h5 class="modal-title">AI 匹配結果預覽</h5>
                            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
                        </div>
                        <div class="modal-body">
                            <div class="mb-3">
                                <label class="form-label fw-bold">原始文字：</label>
                                <div class="border rounded p-2 bg-light" style="max-height: 150px; overflow-y: auto;">
                                    ${this.escapeHtml(originalText)}
                                </div>
                            </div>
                            <div class="mb-3">
                                <div class="d-flex justify-content-between align-items-center mb-2">
                                    <label class="form-label fw-bold mb-0">匹配結果：</label>
                                    <button type="button" class="btn btn-sm btn-outline-primary" id="addMatchBtn">
                                        <i class="bi bi-plus-circle"></i> 添加匹配
                                    </button>
                                </div>
                                <div id="matchResultsContainer"></div>
                                <div id="noMatchesHint" class="text-muted text-center py-3 border rounded bg-light" style="display: ${this.matchedResults.length === 0 ? 'block' : 'none'};">
                                    <small>未匹配到任何字段，請手動添加匹配項</small>
                                </div>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
                            <button type="button" class="btn btn-primary" id="copyToFieldsBtn">
                                <i class="bi bi-clipboard"></i> 複製到字段
                            </button>
                            <button type="button" class="btn btn-success" id="submitFormBtn">
                                <i class="bi bi-check-circle"></i> 直接提交
                            </button>
                        </div>
                    </div>
                </div>
            `;

            // 渲染匹配結果
            const container = modal.querySelector('#matchResultsContainer');
            this.renderMatchResults(container);

            // 綁定添加匹配按鈕
            document.getElementById('addMatchBtn').onclick = () => {
                this.addMatchItem(container);
            };

            // 綁定事件
            document.getElementById('copyToFieldsBtn').onclick = () => {
                this.copyToFields();
                const bsModal = bootstrap.Modal.getInstance(modal);
                if (bsModal) bsModal.hide();
            };

            document.getElementById('submitFormBtn').onclick = () => {
                this.copyToFields();
                this.submitForm();
                const bsModal = bootstrap.Modal.getInstance(modal);
                if (bsModal) bsModal.hide();
            };

            const bsModal = new bootstrap.Modal(modal);
            
            // 移動端：監聽 modal 關閉事件，強制恢復滾動
            const isMobile = window.innerWidth < 768 || /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
            
            // 移動端：使用 MutationObserver 監控 body 樣式變化，強制恢復滾動
            let bodyObserver = null;
            if (isMobile) {
                bodyObserver = new MutationObserver(() => {
                    // 如果 modal 已關閉但 body 仍有 modal-open，強制清理
                    if (!modal.classList.contains('show') && document.body.classList.contains('modal-open')) {
                        document.body.classList.remove('modal-open');
                        document.body.style.paddingRight = '';
                        document.body.style.overflow = '';
                        document.body.style.position = '';
                        if (document.documentElement) {
                            document.documentElement.style.overflow = '';
                            document.documentElement.classList.remove('modal-open');
                        }
                    }
                });
                bodyObserver.observe(document.body, {
                    attributes: true,
                    attributeFilter: ['class', 'style']
                });
            }
            
            // 監聽 modal 關閉事件，確保清理 body 樣式
            const cleanupScroll = () => {
                // 清理所有残留的 modal-backdrop
                const backdrops = document.querySelectorAll('.modal-backdrop');
                backdrops.forEach(backdrop => {
                    try {
                        backdrop.remove();
                    } catch (e) {
                        console.warn('[AIForm] Error removing backdrop:', e);
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
            
            // 監聽 hide 和 hidden 事件
            modal.addEventListener('hide.bs.modal', () => {
                if (isMobile) {
                    // 移動端：在 hide 時就開始清理
                    cleanupScroll();
                }
            }, { once: true });
            
            modal.addEventListener('hidden.bs.modal', () => {
                // 停止觀察
                if (bodyObserver) {
                    bodyObserver.disconnect();
                    bodyObserver = null;
                }
                
                // 多次清理確保完全恢復
                cleanupScroll();
                setTimeout(cleanupScroll, 50);
                setTimeout(cleanupScroll, 150);
                setTimeout(cleanupScroll, 300);
            }, { once: true });
            
            bsModal.show();
        }

        // 渲染匹配結果
        renderMatchResults(container) {
            // 先保存所有当前输入框的值
            const currentValues = {};
            if (container) {
                const inputs = container.querySelectorAll('input[data-index][data-field="value"]');
                inputs.forEach(input => {
                    const index = parseInt(input.dataset.index);
                    if (!isNaN(index)) {
                        currentValues[index] = input.value;
                    }
                });
                const selects = container.querySelectorAll('select[data-index][data-field="value"]');
                selects.forEach(select => {
                    const index = parseInt(select.dataset.index);
                    if (!isNaN(index)) {
                        currentValues[index] = select.value;
                    }
                });
            }
            
            container.innerHTML = '';
            this.matchedResults.forEach((match, index) => {
                // 如果有保存的值，使用保存的值，否则使用 match.value
                const savedValue = currentValues[index] !== undefined ? currentValues[index] : match.value;
                const matchDiv = document.createElement('div');
                matchDiv.className = 'border rounded p-3 mb-2 position-relative';
                
                // 獲取選中的字段信息
                const selectedField = this.availableFields.find(f => f.name === match.field_name);
                const isDynamicSelect2 = selectedField && selectedField.isDynamic;
                const hasOptions = selectedField && selectedField.options && selectedField.options.length > 0;
                const isSelectType = selectedField && (selectedField.type === 'select' || selectedField.type === 'select2' || selectedField.type === 'select2-multi');
                
                // 根據字段類型決定顯示什麼輸入框
                let valueInput = '';
                if (isDynamicSelect2) {
                    // select2 動態字段：顯示搜索輸入框，用戶可以輸入文本描述
                    // 前端會在應用時進行模糊匹配
                    valueInput = `
                        <input type="text" class="form-control form-control-sm" 
                               value="${this.escapeHtml(savedValue || '')}" 
                               data-index="${index}" data-field="value" 
                               placeholder="輸入選項的文本描述（如客戶名稱、產品名稱等）">
                        <small class="text-muted">提示：輸入選項的文本描述，系統會自動匹配對應的選項</small>
                    `;
                } else if (hasOptions || isSelectType) {
                    // 顯示 select，只能選擇預定義的值（包括 select, select2 有固定選項的情況）
                    const options = selectedField && selectedField.options ? selectedField.options : [];
                    valueInput = `
                        <select class="form-select form-select-sm" data-index="${index}" data-field="value">
                            <option value="">-- 選擇值 --</option>
                            ${options.map(opt => 
                                `<option value="${this.escapeHtml(opt.value)}" ${savedValue === opt.value || savedValue === opt.label ? 'selected' : ''}>${this.escapeHtml(opt.label || opt.value)}</option>`
                            ).join('')}
                        </select>
                    `;
                } else {
                    // 顯示 input，可以輸入文本
                    valueInput = `
                        <input type="text" class="form-control form-control-sm" 
                               value="${this.escapeHtml(savedValue || '')}" 
                               data-index="${index}" data-field="value" placeholder="輸入值">
                    `;
                }
                
                matchDiv.innerHTML = `
                    <div class="row g-2">
                        <div class="col-md-4">
                            <label class="form-label small">字段名稱：</label>
                            <select class="form-select form-select-sm" data-index="${index}" data-field="name" onchange="aiFormInput.onFieldNameChange(${index}, this.value)">
                                <option value="">-- 選擇字段 --</option>
                                ${this.availableFields.map(f => 
                                    `<option value="${this.escapeHtml(f.name)}" ${match.field_name === f.name ? 'selected' : ''}>${this.escapeHtml(f.label || f.name)}</option>`
                                ).join('')}
                            </select>
                        </div>
                        <div class="col-md-7">
                            <label class="form-label small">值：</label>
                            ${valueInput}
                        </div>
                        <div class="col-md-1 d-flex align-items-end">
                            <button type="button" class="btn btn-sm btn-outline-danger" onclick="aiFormInput.removeMatchItem(${index})" title="刪除">
                                <i class="bi bi-trash"></i>
                            </button>
                        </div>
                    </div>
                    ${match.confidence ? `<small class="text-muted">置信度: ${(match.confidence * 100).toFixed(1)}%</small>` : ''}
                `;
                container.appendChild(matchDiv);
            });
            
            // 更新"未匹配到任何字段"提示的显示状态
            const noMatchesHint = document.getElementById('noMatchesHint');
            if (noMatchesHint) {
                noMatchesHint.style.display = this.matchedResults.length === 0 ? 'block' : 'none';
            }
        }
        
        // 當字段名稱改變時，更新值輸入框
        onFieldNameChange(index, fieldName) {
            // 先保存当前的值（如果有的话）
            const currentValueElement = document.querySelector(`[data-index="${index}"][data-field="value"]`);
            const currentValue = currentValueElement ? (currentValueElement.tagName === 'SELECT' ? currentValueElement.value : currentValueElement.value) : '';
            
            // 更新匹配結果中的字段名稱
            if (this.matchedResults[index]) {
                this.matchedResults[index].field_name = fieldName;
                // 如果当前有值，保留它
                if (currentValue) {
                    this.matchedResults[index].value = currentValue;
                }
            }
            
            // 獲取選中的字段信息
            const selectedField = this.availableFields.find(f => f.name === fieldName);
            const isDynamicSelect2 = selectedField && selectedField.isDynamic;
            const hasOptions = selectedField && selectedField.options && selectedField.options.length > 0;
            const isSelectType = selectedField && (selectedField.type === 'select' || selectedField.type === 'select2' || selectedField.type === 'select2-multi');
            
            // 找到對應的值輸入框容器（label 的父元素）
            const valueLabel = document.querySelector(`[data-index="${index}"][data-field="value"]`)?.closest('.col-md-7')?.querySelector('label');
            const valueContainer = valueLabel?.nextElementSibling || document.querySelector(`[data-index="${index}"][data-field="value"]`)?.parentElement;
            
            if (valueContainer) {
                if (isDynamicSelect2) {
                    // select2 動態字段：顯示搜索輸入框
                    valueContainer.innerHTML = `
                        <input type="text" class="form-control form-control-sm" 
                               value="${this.escapeHtml(currentValue || '')}" 
                               data-index="${index}" data-field="value" 
                               placeholder="輸入選項的文本描述（如客戶名稱、產品名稱等）">
                        <small class="text-muted">提示：輸入選項的文本描述，系統會自動匹配對應的選項</small>
                    `;
                } else if (hasOptions || isSelectType) {
                    // 顯示 select，只能選擇預定義的值（包括 select, select2 有固定選項的情況）
                    const options = selectedField && selectedField.options ? selectedField.options : [];
                    valueContainer.innerHTML = `
                        <select class="form-select form-select-sm" data-index="${index}" data-field="value">
                            <option value="">-- 選擇值 --</option>
                            ${options.map(opt => 
                                `<option value="${this.escapeHtml(opt.value)}" ${currentValue === opt.value || currentValue === opt.label ? 'selected' : ''}>${this.escapeHtml(opt.label || opt.value)}</option>`
                            ).join('')}
                        </select>
                    `;
                } else {
                    // 顯示 input，可以輸入文本
                    valueContainer.innerHTML = `
                        <input type="text" class="form-control form-control-sm" 
                               value="${this.escapeHtml(currentValue || '')}" 
                               data-index="${index}" data-field="value" placeholder="輸入值">
                    `;
                }
            }
        }

        // 添加匹配項
        addMatchItem(container) {
            this.matchedResults.push({
                field_name: '',
                value: '',
                confidence: 0
            });
            this.renderMatchResults(container);
        }

        // 刪除匹配項
        removeMatchItem(index) {
            this.matchedResults.splice(index, 1);
            const container = document.querySelector('#matchResultsContainer');
            if (container) {
                this.renderMatchResults(container);
            }
        }

        // 複製到字段
        copyToFields() {
            // 更新匹配結果（從輸入框讀取）
            const modal = document.getElementById('aiMatchPreviewModal');
            if (modal) {
                // 分別處理 select 和 input
                const selects = modal.querySelectorAll('select[data-index]');
                selects.forEach(select => {
                    const index = parseInt(select.dataset.index);
                    const field = select.dataset.field;
                    if (this.matchedResults[index] && field === 'name') {
                        // 讀取 select 的選中值
                        const selectedValue = select.value;
                        this.matchedResults[index].field_name = selectedValue;
                        console.log(`更新字段名稱 [${index}]: ${selectedValue}`);
                    }
                });
                
                const inputs = modal.querySelectorAll('input[data-index]');
                inputs.forEach(input => {
                    const index = parseInt(input.dataset.index);
                    const field = input.dataset.field;
                    if (this.matchedResults[index] && field === 'value') {
                        this.matchedResults[index].value = input.value;
                        console.log(`更新字段值 [${index}]: ${input.value}`);
                    }
                });
                
                // 處理 select 類型的值輸入框（用於 yes/no, radio, checkbox, select, select2）
                const valueSelects = modal.querySelectorAll('select[data-index][data-field="value"]');
                valueSelects.forEach(select => {
                    const index = parseInt(select.dataset.index);
                    if (this.matchedResults[index]) {
                        this.matchedResults[index].value = select.value;
                        console.log(`更新字段值 [${index}]: ${select.value}`);
                    }
                });
            }

            // 填充表單字段
            this.matchedResults.forEach((match, idx) => {
                const fieldName = match.field_name;
                const value = match.value;

                console.log(`處理匹配項 [${idx}]: field_name="${fieldName}", value="${value}"`);

                const hasValue = value !== undefined && value !== null && String(value).trim() !== '';

                if (fieldName && hasValue) {
                    const normalizedFieldId = fieldName.startsWith('field_') ? fieldName : `field_${fieldName}`;
                    // 嘗試多種方式找到字段（優先匹配動態表單的 id）
                    let field = document.getElementById(normalizedFieldId) ||
                               document.querySelector(`[name="${fieldName}"]`) ||
                               document.getElementById(fieldName) ||
                               document.querySelector(`[id*="${fieldName}"]`);

                    // 如果命中的是容器，嘗試找內部的表單控件
                    if (field && !['INPUT', 'SELECT', 'TEXTAREA'].includes(field.tagName)) {
                        const innerField = field.querySelector('input, select, textarea');
                        if (innerField) {
                            field = innerField;
                        }
                    }

                    console.log(`查找字段 "${fieldName}":`, field ? '找到' : '未找到');

                    if (field) {
                        // 獲取字段配置
                        const fieldConfig = this.availableFields.find(f => f.name === fieldName);
                        const isDynamicSelect2 = fieldConfig && fieldConfig.isDynamic;

                        // checkbox-group：按 name="field[]" 直接匹配
                        if (fieldConfig && fieldConfig.type === 'checkbox-group') {
                            const values = Array.isArray(value)
                                ? value.map(v => String(v))
                                : String(value).split(',').map(v => v.trim()).filter(Boolean);
                            const checkboxes = document.querySelectorAll(`input[type="checkbox"][name="${fieldName}[]"]`);
                            checkboxes.forEach(checkbox => {
                                checkbox.checked = values.includes(checkbox.value) || values.includes(checkbox.getAttribute('value'));
                            });
                            return;
                        }

                        // radio：按 name 直接匹配（若尚未找到具體欄位）
                        if (!field && fieldConfig && (fieldConfig.type === 'radio' || fieldConfig.type === 'radiobox')) {
                            const radioGroup = document.querySelectorAll(`input[type="radio"][name="${fieldName}"]`);
                            radioGroup.forEach(radio => {
                                if (radio.value === value || radio.value === String(value)) {
                                    radio.checked = true;
                                }
                            });
                            return;
                        }
                        
                        if (field.tagName === 'SELECT') {
                            // 對於 select 字段
                            if (isDynamicSelect2) {
                                // select2 動態字段：進行模糊匹配
                                this.matchSelect2Value(field, value, fieldConfig);
                            } else {
                                // 普通 select：直接匹配
                                const option = Array.from(field.options).find(opt => 
                                    opt.text.includes(value) || opt.value === value || opt.text === value
                                );
                                if (option) {
                                    field.value = option.value;
                                } else {
                                    // 如果找不到匹配的選項，嘗試直接設置值（可能值本身就是有效的）
                                    field.value = value;
                                }
                            }
                        } else if (field.type === 'checkbox') {
                            // 對於 checkbox，設置 checked 狀態
                            if (value === 'true' || value === '1' || value === true) {
                                field.checked = true;
                            } else {
                                field.checked = false;
                            }
                        } else if (field.type === 'radio') {
                            // 對於 radio，找到匹配的 radio 按鈕並選中
                            const radioGroup = document.querySelectorAll(`input[type="radio"][name="${fieldName}"]`);
                            radioGroup.forEach(radio => {
                                if (radio.value === value || radio.value === String(value)) {
                                    radio.checked = true;
                                }
                            });
                        } else {
                            field.value = value;
                        }

                        // 觸發 change 事件
                        field.dispatchEvent(new Event('change', { bubbles: true }));
                        field.dispatchEvent(new Event('input', { bubbles: true }));
                    }
                }
            });

            if (typeof App !== 'undefined' && App.showAlert) {
                App.showAlert('已將匹配結果複製到表單字段', 'success');
            }
        }

        // 提交表單
        submitForm() {
            // 先複製到字段
            this.copyToFields();

            // 查找提交按鈕並點擊
            const submitBtn = document.querySelector('button[type="submit"], button[id*="save"], button[id*="submit"]');
            if (submitBtn) {
                submitBtn.click();
            } else if (window.dynamicForm && typeof window.dynamicForm.submitForm === 'function') {
                window.dynamicForm.submitForm();
            } else {
                const form = document.querySelector('form[id*="Form"], form[id*="form"]');
                if (form) {
                    form.requestSubmit();
                }
            }
        }

        // 匹配 select2 字段的值（模糊匹配）
        matchSelect2Value(selectElement, searchText, fieldConfig) {
            if (!selectElement || !searchText) return;
            
            // 如果 select2 已經初始化，使用 Select2 的搜索功能
            if (typeof $ !== 'undefined' && $(selectElement).hasClass('select2-hidden-accessible')) {
                const $select = $(selectElement);
                
                // 方法1：嘗試從已加載的選項中匹配
                if (selectElement._allOptions && selectElement._allOptions.length > 0) {
                    const labelKey = fieldConfig.relationLabelKey || 'name';
                    const valueKey = fieldConfig.relationValueKey || 'id';
                    
                    // 模糊匹配：找到最相似的選項
                    const matched = selectElement._allOptions.find(item => {
                        const label = item[labelKey] || item.name || item.code || '';
                        return label.toLowerCase().includes(searchText.toLowerCase()) || 
                               searchText.toLowerCase().includes(label.toLowerCase());
                    });
                    
                    if (matched) {
                        const matchedValue = matched[valueKey] || matched.id;
                        $select.val(matchedValue).trigger('change');
                        return;
                    }
                }
                
                // 方法2：使用 Select2 的搜索功能（如果支持）
                // 打開 Select2 下拉框並搜索
                $select.select2('open');
                const searchInput = $('.select2-search__field');
                if (searchInput.length > 0) {
                    searchInput.val(searchText).trigger('input');
                    // 等待選項加載後，選擇第一個匹配項
                    setTimeout(() => {
                        const firstOption = $select.find('option').filter((i, opt) => {
                            return $(opt).text().toLowerCase().includes(searchText.toLowerCase());
                        }).first();
                        if (firstOption.length > 0) {
                            $select.val(firstOption.val()).trigger('change');
                        }
                        $select.select2('close');
                    }, 500);
                }
            } else {
                // 如果 Select2 未初始化，從 DOM option 中匹配
                const option = Array.from(selectElement.options).find(opt => 
                    opt.text.toLowerCase().includes(searchText.toLowerCase()) ||
                    searchText.toLowerCase().includes(opt.text.toLowerCase())
                );
                if (option) {
                    selectElement.value = option.value;
                }
            }
        }

        // HTML 轉義
        escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    }

    // 創建全局實例
    window.AIFormInput = new AIFormInput();
    
    // 自動初始化
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => {
            window.AIFormInput.init();
        });
    } else {
        window.AIFormInput.init();
    }
    
    // 監聽動態內容變化（用於動態表單和 vAi 按鈕）
    const observer = new MutationObserver(() => {
        // 防抖：避免頻繁調用
        clearTimeout(window.AIFormInput._setupTimeout);
        window.AIFormInput._setupTimeout = setTimeout(() => {
            window.AIFormInput.setupFloatingButtons();
        }, 300);
    });
    
    observer.observe(document.body, {
        childList: true,
        subtree: true
    });

    // 頁面完全加載後再次檢查
    window.addEventListener('load', () => {
        setTimeout(() => {
            window.AIFormInput.setupFloatingButtons();
            // 初始化 tooltips
            window.AIFormInput.initTooltips();
        }, 500);
    });

    // 監聽頁面導航（SPA 或普通導航）
    let lastPath = window.location.pathname;
    setInterval(() => {
        const currentPath = window.location.pathname;
        if (currentPath !== lastPath) {
            lastPath = currentPath;
            // 路徑改變時，重新檢查並設置/移除按鈕
            setTimeout(() => {
                window.AIFormInput.setupFloatingButtons();
            }, 100);
        }
    }, 500);

    // 監聽 popstate 事件（瀏覽器前進/後退）
    window.addEventListener('popstate', () => {
        setTimeout(() => {
            window.AIFormInput.setupFloatingButtons();
        }, 100);
    });

})();

