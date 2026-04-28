// Features Slider JavaScript
(function() {
    'use strict';

    const sliderInterval = 10000; // 10 seconds
    let currentIndex = 0;
    let autoSlideTimer = null;
    let isPaused = false;
    let progressAnimation = null;

    // Helper function to get translation
    function getTranslation(key, fallback) {
        try {
            if (typeof I18n !== 'undefined' && I18n.t) {
                const translated = I18n.t(key);
                // If translation returns the key itself (meaning not found), use fallback
                if (translated && translated !== key) {
                    return translated;
                }
            }
        } catch(e) {
            // If translation fails, use fallback
        }
        return fallback;
    }

    function getFeatures() {
        // Get current language
        let lang = 'zh';
        try {
            if (typeof I18n !== 'undefined' && I18n.currentLang) {
                lang = I18n.currentLang;
            } else {
                lang = localStorage.getItem('u-nai_lang') || 'zh';
            }
        } catch(e) {
            lang = 'zh';
        }

        function normalizeVideoLang(input) {
            const v = String(input || '').trim();
            if (v === 'zh-CN') return 'zh-CN';
            if (v === 'zh') return 'zh';
            const lower = v.toLowerCase();
            if (lower.startsWith('en')) return 'en';
            if (lower.startsWith('zh-cn')) return 'zh-CN';
            if (lower.startsWith('zh')) return 'zh';
            return 'zh';
        }

        function videoFor(baseName) {
            const suffix = normalizeVideoLang(lang);
            return `/static/video/${baseName}_${suffix}.mp4`;
        }

        // Feature definitions with translation keys
        const featureDefs = [
            {
                icon: 'bi bi-cash-coin',
                color: '#198754',
                gradient: 'linear-gradient(135deg, #198754 0%, #20c997 100%)',
                videoBase: 'pos_demo',
                translationKey: 'features.slider.pos',
                fallback: {
                    title: 'POS 收銀台',
                    description: '即時商品搜索、購物車管理、快速結帳，整合訂單系統，讓您的銷售流程更加順暢高效。',
                    points: [
                        '即時商品搜索和庫存查詢',
                        '智能購物車管理',
                        '快速結帳流程',
                        '整合訂單和發票系統'
                    ]
                },
                fallbackEn: {
                    title: 'POS System',
                    description: 'Real-time product search, cart management, quick checkout, integrated order system for smooth and efficient sales.',
                    points: [
                        'Real-time product search and inventory query',
                        'Smart cart management',
                        'Quick checkout process',
                        'Integrated order and invoice system'
                    ]
                }
            },
            {
                icon: 'bi bi-cup-hot',
                color: '#fd7e14',
                gradient: 'linear-gradient(135deg, #fd7e14 0%, #dc3545 100%)',
                videoBase: 'dining_demo',
                translationKey: 'features.slider.dining',
                fallback: {
                    title: '餐飲',
                    description: '餐桌管理、點餐系統、廚房出單、排隊叫號，讓您的餐廳運營更高效順暢。',
                    points: [
                        '餐桌狀態實時追蹤',
                        '快速點餐與下單',
                        '廚房出單與進度管理',
                        '排隊叫號與候位系統'
                    ]
                },
                fallbackEn: {
                    title: 'Dining',
                    description: 'Table management, ordering system, kitchen tickets, queue management for efficient restaurant operations.',
                    points: [
                        'Real-time table status tracking',
                        'Quick ordering and checkout',
                        'Kitchen ticket and progress management',
                        'Queue and waiting list system'
                    ]
                }
            },
            {
                icon: 'bi bi-people',
                color: '#0d6efd',
                gradient: 'linear-gradient(135deg, #0d6efd 0%, #0dcaf0 100%)',
                videoBase: 'crm_demo',
                translationKey: 'features.slider.customer',
                fallback: {
                    title: '客戶',
                    description: '完整客戶資料、會員等級、積分系統、加盟管理，建立長久的客戶關係。',
                    points: [
                        '完整的客戶資料管理',
                        '會員等級和積分系統',
                        '加盟商管理',
                        '客戶互動記錄追蹤'
                    ]
                },
                fallbackEn: {
                    title: 'Customers',
                    description: 'Complete customer data, membership levels, points system, franchise management to build lasting customer relationships.',
                    points: [
                        'Complete customer data management',
                        'Membership levels and points system',
                        'Franchise management',
                        'Customer interaction tracking'
                    ]
                }
            },
            {
                icon: 'bi bi-box',
                color: '#198754',
                gradient: 'linear-gradient(135deg, #198754 0%, #20c997 100%)',
                videoBase: 'product_demo',
                translationKey: 'features.slider.product',
                fallback: {
                    title: '產品',
                    description: '產品庫存、類型、屬性、品牌一站式管理，隨時掌握產品動態。',
                    points: [
                        '產品庫存實時追蹤',
                        '多維度分類管理',
                        '產品屬性和品牌管理',
                        '庫存預警和補貨提醒'
                    ]
                },
                fallbackEn: {
                    title: 'Products',
                    description: 'One-stop management of product inventory, types, attributes, and brands, always stay on top of product dynamics.',
                    points: [
                        'Real-time inventory tracking',
                        'Multi-dimensional classification',
                        'Product attributes and brand management',
                        'Inventory alerts and replenishment reminders'
                    ]
                }
            },
            {
                icon: 'bi bi-cart',
                color: '#ffc107',
                gradient: 'linear-gradient(135deg, #ffc107 0%, #fd7e14 100%)',
                videoBase: 'order_demo',
                translationKey: 'features.slider.order',
                fallback: {
                    title: '訂單',
                    description: '訂單追蹤、發票管理、付款記錄完整流程，確保每個訂單都能順利完成。',
                    points: [
                        '訂單全流程追蹤',
                        '發票自動生成',
                        '付款記錄管理',
                        '訂單狀態實時更新'
                    ]
                },
                fallbackEn: {
                    title: 'Orders',
                    description: 'Order tracking, invoice management, payment records complete process to ensure every order is completed smoothly.',
                    points: [
                        'Full order process tracking',
                        'Automatic invoice generation',
                        'Payment record management',
                        'Real-time order status updates'
                    ]
                }
            },
            {
                icon: 'bi bi-tools',
                color: '#20c997',
                gradient: 'linear-gradient(135deg, #20c997 0%, #0dcaf0 100%)',
                videoBase: 'service_management',
                translationKey: 'features.slider.service',
                fallback: {
                    title: '服務',
                    description: '服務單建立、進度追蹤、服務標籤與收款流程一次完成，讓服務型業務流程更清晰。',
                    points: [
                        '服務單建立與狀態追蹤',
                        '服務標籤與分類管理',
                        '到款與發票流程整合',
                        '服務歷史與客戶紀錄串聯'
                    ]
                },
                fallbackEn: {
                    title: 'Services',
                    description: 'Create service orders, track progress, manage service tags, and handle payments in one clear workflow.',
                    points: [
                        'Service order creation and status tracking',
                        'Service tags and category management',
                        'Integrated payment and invoice flow',
                        'Linked service history and customer records'
                    ]
                }
            },
            {
                icon: 'bi bi-kanban',
                color: '#6f42c1',
                gradient: 'linear-gradient(135deg, #6f42c1 0%, #b197fc 100%)',
                videoBase: 'project_management',
                translationKey: 'features.slider.project',
                fallback: {
                    title: '項目',
                    description: '項目看板、里程碑與進度追蹤集中管理，協作更清楚、交付更準時。',
                    points: [
                        '項目階段與狀態視覺化',
                        '負責人與進度追蹤',
                        '里程碑與期限提醒',
                        '跨部門協作記錄彙整'
                    ]
                },
                fallbackEn: {
                    title: 'Projects',
                    description: 'Centralize boards, milestones, and progress tracking to keep teams aligned and deliveries on time.',
                    points: [
                        'Visual project stages and statuses',
                        'Owners and progress tracking',
                        'Milestones and deadline reminders',
                        'Cross-team collaboration logs'
                    ]
                }
            },
            {
                icon: 'bi bi-calculator',
                color: '#0dcaf0',
                gradient: 'linear-gradient(135deg, #0dcaf0 0%, #0d6efd 100%)',
                videoBase: 'accounting_demo',
                translationKey: 'features.slider.accounting',
                fallback: {
                    title: '會計',
                    description: '收入支出管理、採購單追蹤、財務報表，讓財務管理一目了然。',
                    points: [
                        '收入支出分類管理',
                        '採購單追蹤',
                        '財務報表自動生成',
                        '財務數據分析'
                    ]
                },
                fallbackEn: {
                    title: 'Accounting',
                    description: 'Income and expense management, purchase order tracking, financial reports for clear financial management.',
                    points: [
                        'Income and expense categorization',
                        'Purchase order tracking',
                        'Automatic financial report generation',
                        'Financial data analysis'
                    ]
                }
            },
            {
                icon: 'bi bi-sliders',
                color: '#8b5cf6',
                gradient: 'linear-gradient(135deg, #8b5cf6 0%, #a78bfa 100%)',
                videoBase: 'customization_demo',
                translationKey: 'features.slider.customization',
                fallback: {
                    title: '靈活自定義',
                    description: '根據業務需求自由調整網站首頁的內容與版面，無需技術背景，輕鬆打造專屬官網。',
                    points: [
                        '首頁區塊自定義',
                        '文案與圖片即時調整',
                        '版面配置彈性調整',
                        '隨時更新與發布'
                    ]
                },
                fallbackEn: {
                    title: 'Flexible Customization',
                    description: 'Freely customize your website homepage content and layout based on your business needs, with no technical background required.',
                    points: [
                        'Homepage sections customization',
                        'Edit copy and images instantly',
                        'Flexible layout adjustments',
                        'Update and publish anytime'
                    ]
                }
            }
        ];

        const isZh = lang === 'zh' || lang === 'zh-CN';

        // Build features array with translations
        return featureDefs.map(def => {
            const fallback = isZh ? def.fallback : def.fallbackEn;
            
            // Try to get translations
            let title = getTranslation(def.translationKey + '.title', fallback.title);
            let description = getTranslation(def.translationKey + '.description', fallback.description);
            
            // Get points translations
            let points = [];
            try {
                if (typeof I18n !== 'undefined' && I18n.t) {
                    const pointsData = I18n.t(def.translationKey + '.points');
                    if (pointsData && Array.isArray(pointsData)) {
                        points = pointsData;
                    } else {
                        points = fallback.points;
                    }
                } else {
                    points = fallback.points;
                }
            } catch(e) {
                points = fallback.points;
            }

            return {
                icon: def.icon,
                color: def.color,
                gradient: def.gradient,
                video: videoFor(def.videoBase),
                title: title,
                description: description,
                points: points
            };
        });
    }

    let features = getFeatures();

    function reloadFeatures() {
        features = getFeatures();
        renderIcons();
        renderContent();
        renderImages();
    }

    function initSlider() {
        renderIcons();
        renderContent();
        renderImages();
        startAutoSlide();
        setupEventListeners();
        
        // Reload features when language changes
        if (typeof window !== 'undefined') {
            // 监听自定义语言切换事件
            window.addEventListener('languageChanged', function(event) {
                // 延迟一下确保翻译已加载
                setTimeout(() => {
                    reloadFeatures();
                }, 150);
            });
            
            // 也监听 I18n 的 updatePage（作为备用）
            if (typeof I18n !== 'undefined') {
                const originalUpdatePage = I18n.updatePage;
                I18n.updatePage = function() {
                    const result = originalUpdatePage.call(this);
                    // 延迟一下确保翻译已加载
                    setTimeout(() => {
                        reloadFeatures();
                    }, 150);
                    return result;
                };
            }
        }
    }

    function renderIcons() {
        const container = document.getElementById('featureIconsNav');
        if (!container) return;

        container.innerHTML = features.map((feature, index) => `
            <div class="feature-icon-item ${index === 0 ? 'active' : ''}" 
                 data-index="${index}">
                <div class="icon-wrapper">
                    <i class="${feature.icon} fs-2"></i>
                </div>
                <div class="icon-label">${feature.title}</div>
                <div class="timer-line" style="display: ${index === 0 ? 'block' : 'none'};">
                    <div class="timer-progress"></div>
                </div>
            </div>
        `).join('');

        // Add click handlers
        container.querySelectorAll('.feature-icon-item').forEach((item, index) => {
            item.addEventListener('click', () => switchFeature(index));
        });
    }

    function renderContent() {
        const container = document.getElementById('featureContentArea');
        if (!container) return;

        container.innerHTML = features.map((feature, index) => `
            <div class="feature-content ${index === 0 ? 'active' : ''}" data-index="${index}">
                <h3 class="fw-bold mb-4">${feature.title}</h3>
                <p class="lead mb-4" style="color: #6c757d; line-height: 1.8;">${feature.description}</p>
                <ul class="list-unstyled">
                    ${feature.points.map(point => `
                        <li class="mb-3 d-flex align-items-center">
                            <svg width="24" height="24" viewBox="0 0 24 24" focusable="false" class="me-2" style="color: #22c55e; flex-shrink: 0;">
                                <path fill="currentColor" d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41L9 16.17z"></path>
                            </svg>
                            <span>${point}</span>
                        </li>
                    `).join('')}
                </ul>
            </div>
        `).join('');
    }

    function renderImages() {
        const container = document.getElementById('featureImageArea');
        if (!container) return;

        container.innerHTML = features.map((feature, index) => `
            <div class="feature-image-slide ${index === 0 ? 'active' : ''}" data-index="${index}" style="background: ${feature.gradient};">
                <video class="feature-slide-video" autoplay muted loop playsinline preload="auto">
                    <source src="${feature.video}" type="video/mp4">
                </video>
                <div class="feature-image-overlay">
                    <div class="feature-title-row d-flex align-items-center gap-2">
                        <i class="${feature.icon} feature-overlay-icon" style="color: rgba(255, 255, 255, 0.9);"></i>
                        <h4 class="feature-overlay-title mb-0" style="color: rgba(255, 255, 255, 0.9);">${feature.title}</h4>
                    </div>
                </div>
                <div class="feature-image-bottom-white"></div>
            </div>
        `).join('');

        document.querySelectorAll('.feature-image-slide').forEach((slide) => {
            const video = slide.querySelector('.feature-slide-video');
            if (!video) return;

            delete video.dataset.ready;

            video.addEventListener('loadeddata', () => {
                video.dataset.ready = '1';
                if (slide.classList.contains('active')) {
                    video.play().catch(() => {});
                } else {
                    try { video.pause(); } catch (e) {}
                }
            }, { once: true });

            video.addEventListener('error', () => {
                video.dataset.ready = '1';
            }, { once: true });

            try { video.load(); } catch (e) {}
            if (!slide.classList.contains('active')) {
                try { video.pause(); } catch (e) {}
            } else {
                video.play().catch(() => {});
            }
        });
    }

    function switchFeature(index) {
        if (index === currentIndex) return;
        
        // Update current index
        currentIndex = index;
        
        // Update icons
        document.querySelectorAll('.feature-icon-item').forEach((item, idx) => {
            if (idx === index) {
                item.classList.add('active');
                item.querySelector('.timer-line').style.display = 'block';
            } else {
                item.classList.remove('active');
                item.querySelector('.timer-line').style.display = 'none';
            }
        });

        // Update content
        document.querySelectorAll('.feature-content').forEach((item, idx) => {
            if (idx === index) {
                item.classList.add('active');
                item.style.display = 'block';
            } else {
                item.classList.remove('active');
                item.style.display = 'none';
            }
        });

        // Update images/videos
        document.querySelectorAll('.feature-image-slide').forEach((item, idx) => {
            const video = item.querySelector('.feature-slide-video');
            if (idx === index) {
                item.classList.add('active');
                if (video) {
                    if (video.dataset.ready === '1') {
                        video.play().catch(() => {});
                    } else {
                        video.addEventListener('loadeddata', () => {
                            if (item.classList.contains('active')) {
                                video.play().catch(() => {});
                            }
                        }, { once: true });
                        try { video.load(); } catch (e) {}
                    }
                }
            } else {
                item.classList.remove('active');
                if (video) {
                    video.pause();
                }
            }
        });

        // Restart timer
        restartTimer();
    }

    function startAutoSlide(shouldRestartTimer = true) {
        if (autoSlideTimer) clearInterval(autoSlideTimer);
        if (isPaused) return;
        if (shouldRestartTimer) {
            restartTimer();
        }
        
        autoSlideTimer = setInterval(() => {
            if (!isPaused) {
                const nextIndex = (currentIndex + 1) % features.length;
                switchFeature(nextIndex);
            }
        }, sliderInterval);
    }

    function restartTimer() {
        // Reset progress animation
        const progressBar = document.querySelector('.feature-icon-item.active .timer-progress');
        if (progressBar) {
            progressBar.style.animation = 'none';
            setTimeout(() => {
                progressBar.style.animation = `timerProgress ${sliderInterval}ms linear`;
            }, 10);
        }
    }

    function pauseSlider() {
        isPaused = true;
        const container = document.getElementById('featureSliderContainer');
        if (container) {
            container.classList.add('feature-slider-paused');
        }
        const progressBar = document.querySelector('.feature-icon-item.active .timer-progress');
        if (progressBar) {
            progressBar.style.animationPlayState = 'paused';
        }
        if (progressAnimation) {
            clearInterval(progressAnimation);
        }
    }

    function resumeSlider() {
        isPaused = false;
        const container = document.getElementById('featureSliderContainer');
        if (container) {
            container.classList.remove('feature-slider-paused');
        }
        const progressBar = document.querySelector('.feature-icon-item.active .timer-progress');
        if (progressBar) {
            progressBar.style.animationPlayState = 'running';
        }
        startAutoSlide(false);
    }

    function setupEventListeners() {
        const contentArea = document.getElementById('featureContentArea');
        if (contentArea) {
            contentArea.addEventListener('mouseenter', pauseSlider);
            contentArea.addEventListener('mouseleave', resumeSlider);
        }
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initSlider);
    } else {
        initSlider();
    }
})();
