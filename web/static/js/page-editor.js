// stripTenantPrefix: remove /co/{subdomain} prefix from a link for display in the editor.
// This ensures users always see clean relative paths like "/shop" instead of "/co/xxx/shop".
function _stripTenantLinkPrefix(link) {
  if (!link) return '';
  var sd = window.tenantSubdomain || localStorage.getItem('tenant_subdomain') || '';
  if (!sd) return link;
  var prefix = '/co/' + sd;
  if (link === prefix || link === prefix + '/') return '/';
  if (link.startsWith(prefix + '/')) return link.substring(prefix.length);
  return link;
}

var PageEditor = {
  pageId: null,
  pageData: null,
  components: [],
  selectedComponent: null,
  selectedElement: null, // 當前選中的元素（文字/按鈕/圖片）
  allPages: [],
  currentPageIndex: -1,
  viewMode: 'desktop', // 'desktop' or 'mobile'
  history: [], // 操作歷史記錄
  historyIndex: -1, // 當前歷史記錄索引
  maxHistorySize: 50, // 最大歷史記錄數量
  addComponentAfterIndex: null, // 當前要在哪個元件後添加新元件
  addComponentToSectionColumn: null, // 當前要添加到區塊容器的哪個欄位 { sectionIndex, columnIndex }
  labelHideTimeout: null, // label 隱藏定時器
  savingComponentIndex: null, // 當前要保存為區塊的元件索引
  isSavingBlock: false, // 是否正在保存區塊（防止重複保存）
  hasUnsavedChanges: false, // 是否有未保存的更改
  enterpriseName: '', // 企業名稱
  productMetaLoaded: false,
  productMetaLoading: false,
  productTypes: [],
  productCategories: [],
  productAttributes: [],

  // Theme color resolution: empty value = follow theme, non-empty = user customized.
  //
  // For inline styles in preview HTML, use themeColor() which returns either:
  //   - The user's custom color value (when data has a value)
  //   - A CSS var() reference (when data is empty, meaning "follow theme")
  //
  // For color picker inputs, use getThemeVar() to get the computed hex value.

  // Get computed value of a CSS custom property (returns actual hex, for color pickers)
  getThemeVar(varName, fallback) {
    const val = getComputedStyle(document.documentElement).getPropertyValue(varName).trim();
    return val || fallback || '';
  },

  // Returns CSS value for inline styles: user value or var(--theme-xxx, fallback)
  themeColor(dataValue, cssVar, fallback) {
    if (!dataValue) {
      return `var(${cssVar}, ${fallback})`;
    }
    return dataValue;
  },

  // Returns actual hex color for <input type="color"> (needs concrete value, not CSS var)
  themeColorHex(dataValue, cssVar, fallback) {
    if (dataValue) return dataValue;
    return this.getThemeVar(cssVar, fallback);
  },

  // Renders a color picker with "Reset to Theme" button.
  // inputId: the DOM id for the color input
  // label: display label text
  // dataValue: current saved value (empty = following theme)
  // cssVar: the CSS variable name (e.g., '--theme-hero-bg')
  // fallback: fallback hex value
  renderThemeColorPicker(inputId, label, dataValue, cssVar, fallback) {
    const hexValue = this.themeColorHex(dataValue, cssVar, fallback);
    const isFollowingTheme = !dataValue;
    const themeLabel = this.t('pages.pageEditor.labels.common.followingTheme', 'Following theme');
    const resetLabel = this.t('pages.pageEditor.labels.common.resetToTheme', 'Reset to theme');
    return `
          <div class="mb-3">
            <label class="form-label">${label}</label>
            <div class="d-flex align-items-center gap-2">
              <input type="color" class="form-control form-control-color theme-color-picker" id="${inputId}" value="${hexValue}" data-css-var="${cssVar}" data-fallback="${fallback}">
              ${isFollowingTheme
                ? `<span class="badge bg-secondary" id="${inputId}-indicator" style="font-size: 0.7rem;">${themeLabel}</span>`
                : `<button type="button" class="btn btn-sm btn-outline-secondary" id="${inputId}-indicator" style="font-size: 0.7rem; white-space: nowrap;" onclick="PageEditor.resetColorToTheme('${inputId}', '${cssVar}', '${fallback}')">${resetLabel}</button>`
              }
            </div>
            <input type="hidden" id="${inputId}-is-theme" value="${isFollowingTheme ? '1' : '0'}">
          </div>`;
  },

  // Called when user manually changes a color picker — marks it as custom (not following theme)
  onColorPickerChange(inputId, cssVar, fallback) {
    const hiddenInput = document.getElementById(inputId + '-is-theme');
    if (hiddenInput) hiddenInput.value = '0';
    // Replace badge with reset button
    const indicator = document.getElementById(inputId + '-indicator');
    if (indicator && indicator.tagName === 'SPAN') {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn btn-sm btn-outline-secondary';
      btn.style.fontSize = '0.7rem';
      btn.style.whiteSpace = 'nowrap';
      btn.id = inputId + '-indicator';
      btn.textContent = this.t('pages.pageEditor.labels.common.resetToTheme', 'Reset to theme');
      btn.onclick = () => this.resetColorToTheme(inputId, cssVar, fallback);
      indicator.replaceWith(btn);
    }
  },

  // Get color value for saving: empty string if following theme, otherwise the picker value
  getSaveColor(inputId) {
    const isTheme = document.getElementById(inputId + '-is-theme');
    if (isTheme && isTheme.value === '1') return '';
    return document.getElementById(inputId)?.value || '';
  },

  // Reset a color picker to follow theme (called from reset button onclick)
  resetColorToTheme(inputId, cssVar, fallback) {
    const hexValue = this.getThemeVar(cssVar, fallback);
    const input = document.getElementById(inputId);
    if (input) input.value = hexValue;
    // Mark as following theme
    const hiddenInput = document.getElementById(inputId + '-is-theme');
    if (hiddenInput) hiddenInput.value = '1';
    // Replace button with badge
    const container = input?.parentElement;
    if (container) {
      const btn = container.querySelector('button');
      if (btn) {
        const badge = document.createElement('span');
        badge.className = 'badge bg-secondary';
        badge.style.fontSize = '0.7rem';
        badge.textContent = this.t('pages.pageEditor.labels.common.followingTheme', 'Following theme');
        btn.replaceWith(badge);
      }
    }
    // Re-render preview and save state
    this.updateComponentData();
    this.saveState();
  },

  t(key, fallback) {
    try {
      if (typeof I18n !== 'undefined' && I18n.t) {
        const v = I18n.t(key);
        // 若語系檔缺 key，很多 i18n 實作會直接回傳 key 本身；此時使用 fallback
        if (v === key || v === undefined || v === null || v === '') return fallback;
        return v;
      }
    } catch {}
    return fallback;
  },

  async init() {
    if (!App.checkAuth()) return;
    
    this.showLoading();
    try {
      // 檢查螢幕寬度
      this.checkScreenSize();
      window.addEventListener('resize', () => {
        this.checkScreenSize();
      });
      
      // 載入企業名稱
      await this.loadEnterpriseName();
    
    const pageIdAttr = document.getElementById('pageEditorPage').dataset.pageId;
    this.pageId = pageIdAttr && pageIdAttr.trim() !== '' ? pageIdAttr : null;
    this.isNew = !this.pageId;


    if (this.isNew) {
      // 新建模式：初始化空數據
      this.pageData = {
        name: '',
        slug: '',
        topnav_style: 'default',
        status: 'draft',
        components: []
      };
      this.components = [];
      
      // 禁用預覽按鈕
      document.getElementById('previewBtn').disabled = true;
      document.getElementById('previewBtn').title = this.t('pages.pageEditor.saveBeforePreview', 'Please save the page first');
    } else {
      // 編輯模式：載入現有頁面
      await this.loadPage();
      this.updatePreviewUrl();
    }
    
    // 初始化歷史記錄（保存初始狀態）
    this.saveState();
    
    this.bindEvents();
    this.renderComponents();
      this.loadPageSelector();
    } catch (error) {
      console.error('頁面載入失敗:', error);
      App.showError(this.t('pages.pageEditor.loadPageFailedRetry', 'Failed to load page. Please try again.'));
    } finally {
      this.hideLoading();
    }
  },
  
  async loadEnterpriseName() {
    try {
      const enterprise = await App.apiRequest('/enterprises/me');
      this.enterpriseName = enterprise.name || enterprise.Name || '';
    } catch (error) {
      console.error('載入企業名稱失敗:', error);
      this.enterpriseName = '';
    }
  },
  
  async loadPageSelector() {
    const pageListContainer = document.getElementById('pageListContainer');
    const addNewPageOption = document.getElementById('addNewPageOption');
    const pageEditorTitle = document.getElementById('pageEditorTitle');
    
    if (!pageListContainer || !addNewPageOption) return;
    
    // 绑定加页面选项
    addNewPageOption.addEventListener('click', async (e) => {
      e.preventDefault();
      e.stopPropagation();
      
      // 检查是否有未保存的更改
      if (this.hasUnsavedChanges && this.pageId) {
        const shouldSave = confirm(this.t('pages.pageEditor.unsavedChangesCreateNew', 'You have unsaved changes. Save current page and create a new page?\n\nOK: Save & create new page\nCancel: Discard changes & create new page'));
        if (shouldSave) {
          await this.save();
        }
      }
      
      // 关闭 dropdown
      const dropdown = bootstrap.Dropdown.getInstance(pageEditorTitle);
      if (dropdown) dropdown.hide();
      
      // 跳转到新页面
      if (typeof Router !== 'undefined' && Router.go) {
        Router.go('/pages/new');
      } else {
        window.location.href = '/pages/new';
      }
    });
    
    try {
      const response = await App.apiRequest('/pages?limit=1000');
      const pages = response.data || response || [];
      
      if (pages.length === 0) {
        pageListContainer.innerHTML = '<div class="text-center text-muted py-2"><small>' + this.t('pages.pageEditor.noPages', 'No pages yet') + '</small></div>';
        return;
      }
      
      pageListContainer.innerHTML = pages.map(page => {
        const isCurrentPage = this.pageId && page.id === this.pageId;
        return `
          <li>
            <a class="dropdown-item ${isCurrentPage ? 'active' : ''}" href="/pages/${page.id}/edit" ${isCurrentPage ? 'onclick="event.preventDefault(); return false;"' : ''}>
              ${page.name || this.t('common.untitled', 'Untitled')} ${isCurrentPage ? '<i class="bi bi-check float-end"></i>' : ''}
            </a>
          </li>
        `;
      }).join('');
    } catch (error) {
      pageListContainer.innerHTML = '<div class="text-center text-danger py-2"><small>' + this.t('common.loadError', 'Load failed') + '</small></div>';
    }
  },

  async loadAllPages() {
    try {
      const response = await App.apiRequest('/pages?limit=1000');
      this.allPages = response.data || response || [];
      // 找到當前頁面在列表中的位置
      if (this.pageId) {
        this.currentPageIndex = this.allPages.findIndex(p => p.id === this.pageId);
      }
    } catch (error) {
      console.error('載入頁面列表失敗:', error);
      this.allPages = [];
    }
  },

  updateNavigationButtons() {
    const prevBtn = document.getElementById('prevPageBtn');
    const nextBtn = document.getElementById('nextPageBtn');
    
    if (this.currentPageIndex > 0) {
      prevBtn.disabled = false;
    } else {
      prevBtn.disabled = true;
    }
    
    if (this.currentPageIndex >= 0 && this.currentPageIndex < this.allPages.length - 1) {
      nextBtn.disabled = false;
    } else {
      nextBtn.disabled = true;
    }
  },

  updatePreviewUrl() {
    const previewUrlContainer = document.getElementById('previewUrlContainer');
    const previewUrl = document.getElementById('previewUrl');
    const previewUrlOpenBtn = document.getElementById('previewUrlOpenBtn');
    
    if (this.pageData && this.pageData.slug) {
      // 获取租户子域名
      const tenantSubdomain = window.tenantSubdomain || localStorage.getItem('tenant_subdomain') || 'test';
      const path = `/co/${tenantSubdomain}/${this.pageData.slug}/`;
      // 获取当前域名并拼接完整 URL
      const fullUrl = window.location.origin + path;
      previewUrl.textContent = fullUrl;
      if (previewUrlOpenBtn) {
        previewUrlOpenBtn.href = fullUrl;
      }
      previewUrlContainer.style.display = 'block';
    } else {
      previewUrlContainer.style.display = 'none';
    }
  },

  // 檢查屏幕寬度
  checkScreenSize() {
    const screenLimitAlert = document.getElementById('screenLimitAlert');
    const editorViewArea = document.querySelector('.editor-view-area');
    
    if (!screenLimitAlert) return;
    
    const screenWidth = window.innerWidth;
    
    if (screenWidth < 1000) {
      screenLimitAlert.classList.remove('d-none');
      // 禁用页面滚动
      document.body.style.overflow = 'hidden';
      if (editorViewArea) {
        editorViewArea.style.pointerEvents = 'none';
        editorViewArea.style.opacity = '0.5';
      }
    } else {
      screenLimitAlert.classList.add('d-none');
      // 恢复页面滚动
      document.body.style.overflow = '';
      if (editorViewArea) {
        editorViewArea.style.pointerEvents = 'auto';
        editorViewArea.style.opacity = '1';
      }
    }
  },

  // 保存當前狀態到歷史記錄
  saveState() {
    // 深拷貝 components 數組
    const state = JSON.parse(JSON.stringify(this.components));
    
    // 如果當前不在歷史記錄末尾，刪除後面的記錄（因為用戶執行了新操作）
    if (this.historyIndex < this.history.length - 1) {
      this.history = this.history.slice(0, this.historyIndex + 1);
    }
    
    // 添加新狀態
    this.history.push(state);
    
    // 限制歷史記錄大小
    if (this.history.length > this.maxHistorySize) {
      this.history.shift();
    } else {
      this.historyIndex++;
    }
    
    // 更新按鈕狀態
    this.updateHistoryButtons();
  },

  // 更新 Undo/Redo 按鈕狀態
  updateHistoryButtons() {
    const undoBtn = document.getElementById('undoBtn');
    const redoBtn = document.getElementById('redoBtn');
    
    if (undoBtn) undoBtn.disabled = this.historyIndex <= 0;
    if (redoBtn) redoBtn.disabled = this.historyIndex >= this.history.length - 1;
  },

  // 撤銷操作
  undo() {
    if (this.historyIndex > 0) {
      this.historyIndex--;
      this.restoreState(this.history[this.historyIndex]);
      this.updateHistoryButtons();
    }
  },

  // 重做操作
  redo() {
    if (this.historyIndex < this.history.length - 1) {
      this.historyIndex++;
      this.restoreState(this.history[this.historyIndex]);
      this.updateHistoryButtons();
    }
  },

  // 恢復狀態
  restoreState(state) {
    this.components = JSON.parse(JSON.stringify(state));
    this.renderComponents();
    // 清除選中狀態
    this.selectedComponent = null;
    const propertiesContent = document.getElementById('propertiesContent');
    if (propertiesContent) {
      propertiesContent.innerHTML = `
        <div class="text-center py-5">
          <i class="bi bi-cursor fs-1 d-block mb-3"></i>
          <p>${this.t('pages.pageEditor.selectComponentToEdit', 'Select a component to edit properties')}</p>
        </div>
      `;
      propertiesContent.classList.remove('has-form');
    }
  },

  async loadPage() {
    try {
      this.pageData = await App.apiRequest(`/pages/${this.pageId}`);
      this.components = this.pageData.components || [];
      
      // 應用容器寬度
      this.applyContainerWidth();
      
      // 頁面資料已載入，會在 showPageSettings 時填充到 modal
      this.renderComponents();
    } catch (error) {
      App.showError(this.t('common.loadError', 'Load failed') + ': ' + (error.error || error.message));
    }
  },

  bindEvents() {
    // 側邊欄圖標點擊事件
    const libraryIcon = document.getElementById('componentLibraryIcon');
    const libraryPanel = document.getElementById('componentLibraryPanel');
    const closeLibraryBtn = document.getElementById('closeComponentLibrary');
    
    if (libraryIcon) {
      libraryIcon.addEventListener('click', () => {
        // 切換 active 狀態
        document.querySelectorAll('.sidebar-icon-btn').forEach(btn => {
          btn.classList.remove('active');
        });
        libraryIcon.classList.add('active');
        this.showComponentLibrary();
      });
    }
    
    if (closeLibraryBtn) {
      closeLibraryBtn.addEventListener('click', () => {
        this.hideComponentLibrary();
        // 移除 active 狀態
        if (libraryIcon) {
          libraryIcon.classList.remove('active');
        }
      });
    }
    
    // 區塊庫按鈕事件
    const blockLibraryIcon = document.getElementById('blockLibraryIcon');
    const closeBlockLibraryBtn = document.getElementById('closeBlockLibrary');
    
    if (blockLibraryIcon) {
      blockLibraryIcon.addEventListener('click', () => {
        // 切換 active 狀態
        document.querySelectorAll('.sidebar-icon-btn').forEach(btn => {
          btn.classList.remove('active');
        });
        blockLibraryIcon.classList.add('active');
        this.showBlockLibrary();
      });
    }
    
    if (closeBlockLibraryBtn) {
      closeBlockLibraryBtn.addEventListener('click', () => {
        this.hideBlockLibrary();
        // 移除 active 狀態
        if (blockLibraryIcon) {
          blockLibraryIcon.classList.remove('active');
        }
      });
    }
    
    // 點擊面板外部關閉
    document.addEventListener('click', (e) => {
      // 關閉元件庫 panel
      if (libraryPanel && libraryPanel.style.display !== 'none') {
        if (!libraryPanel.contains(e.target) && 
            !libraryIcon.contains(e.target) &&
            !e.target.closest('.component-library-panel')) {
          this.hideComponentLibrary();
          if (libraryIcon) {
            libraryIcon.classList.remove('active');
          }
        }
      }
      
      // 關閉區塊庫 panel
      const blockLibraryPanel = document.getElementById('blockLibraryPanel');
      const blockLibraryIcon = document.getElementById('blockLibraryIcon');
      if (blockLibraryPanel && blockLibraryPanel.style.display !== 'none') {
        if (!blockLibraryPanel.contains(e.target) && 
            !blockLibraryIcon.contains(e.target) &&
            !e.target.closest('.component-library-panel')) {
          this.hideBlockLibrary();
          if (blockLibraryIcon) {
            blockLibraryIcon.classList.remove('active');
          }
        }
      }
    });
    
    // 監聽滾動，當滾動超過 pebar 時，讓側邊欄 fixed
    const pebar = document.getElementById('pebar');
    const sidebar = document.querySelector('.editor-sidebar');
    const viewArea = document.querySelector('.editor-view-area');
    
    if (pebar && sidebar && viewArea) {
      const updateSidebarPosition = () => {
        const pebarHeight = pebar.offsetHeight;
        // 設置 CSS 變量，用於動態設置 top 值
        sidebar.style.setProperty('--pebar-height', `${pebarHeight}px`);
        const libraryPanel = document.getElementById('componentLibraryPanel');
        if (libraryPanel) {
          libraryPanel.style.setProperty('--pebar-height', `${pebarHeight}px`);
        }
        const propertiesPanel = document.getElementById('propertiesPanel');
        if (propertiesPanel) {
          propertiesPanel.style.setProperty('--pebar-height', `${pebarHeight}px`);
        }
      };
      
      // 處理 window 滾動（用於判斷是否應該 fixed）
      const handleWindowScroll = () => {
        const pebarHeight = pebar.offsetHeight;
        // 只使用 window 的滾動位置來判斷是否應該 fixed
        const scrollTop = window.pageYOffset || document.documentElement.scrollTop;
        
        const propertiesPanel = document.getElementById('propertiesPanel');
        
        // 使用 requestAnimationFrame 确保在下一帧应用样式，避免闪烁
        requestAnimationFrame(() => {
          if (scrollTop > pebarHeight) {
            sidebar.classList.add('fixed');
            if (propertiesPanel) {
              propertiesPanel.classList.add('fixed');
              viewArea.classList.add('properties-panel-fixed');
            }
          } else {
            sidebar.classList.remove('fixed');
            if (propertiesPanel) {
              propertiesPanel.classList.remove('fixed');
              viewArea.classList.remove('properties-panel-fixed');
            }
          }
        });
      };
      
      // 處理 panel 位置更新（只在 panel 顯示時更新）
      const updatePanelPositions = () => {
        // 如果元件庫 panel 顯示中，更新其位置
        const libraryPanel = document.getElementById('componentLibraryPanel');
        if (libraryPanel && libraryPanel.style.display !== 'none') {
          const libraryIcon = document.getElementById('componentLibraryIcon');
          if (libraryIcon) {
            const iconRect = libraryIcon.getBoundingClientRect();
            libraryPanel.style.left = `${iconRect.right}px`;
            libraryPanel.style.top = `${iconRect.top}px`;
            libraryPanel.style.maxHeight = `calc(100vh - ${iconRect.top}px)`;
          }
        }
        
        // 如果區塊庫 panel 顯示中，更新其位置
        const blockLibraryPanel = document.getElementById('blockLibraryPanel');
        if (blockLibraryPanel && blockLibraryPanel.style.display !== 'none') {
          const blockLibraryIcon = document.getElementById('blockLibraryIcon');
          if (blockLibraryIcon) {
            const iconRect = blockLibraryIcon.getBoundingClientRect();
            blockLibraryPanel.style.left = `${iconRect.right}px`;
            blockLibraryPanel.style.top = `${iconRect.top}px`;
            blockLibraryPanel.style.maxHeight = `calc(100vh - ${iconRect.top}px)`;
          }
        }
      };
      
      // 初始設置
      updateSidebarPosition();
      
      // 監聽窗口大小變化，更新 pebar 高度
      window.addEventListener('resize', () => {
        updateSidebarPosition();
        updatePanelPositions();
      });
      
      // 只監聽 window 的滾動來判斷是否應該 fixed（不監聽 viewArea 的滾動）
      window.addEventListener('scroll', handleWindowScroll, true);
      
      // 監聽 viewArea 的滾動，只用於更新 panel 位置（不影響 fixed 狀態）
      viewArea.addEventListener('scroll', updatePanelPositions);
      
      // 初始檢查
      handleWindowScroll();
      updatePanelPositions();
    }
    
    // 添加元件按鈕
    this.bindComponentAddButtons();
    
    // 取消添加模式按钮（点击提示区域外）
    document.addEventListener('click', (e) => {
      if ((this.addComponentAfterIndex !== null || this.addComponentToSectionColumn !== null) && !e.target.closest('#addComponentAfterPrompt') && !e.target.closest('.component-add-btn')) {
        this.cancelAddComponentAfter();
      }
    });

    // 保存按鈕
    document.getElementById('saveBtn').addEventListener('click', () => this.save());
    
    // 保存區塊按鈕
    const saveBlockBtn = document.getElementById('saveBlockBtn');
    if (saveBlockBtn) {
      saveBlockBtn.addEventListener('click', () => this.confirmSaveBlock());
    }
    
    // 區塊名稱輸入框 Enter 鍵支持
    const blockNameInput = document.getElementById('blockName');
    if (blockNameInput) {
      blockNameInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          this.confirmSaveBlock();
        }
      });
    }
    
    // 預覽按鈕
    document.getElementById('previewBtn').addEventListener('click', async () => {
      if (this.isNew || !this.pageData || !this.pageData.slug) {
        App.showError(this.t('pages.pageEditor.saveBeforePreview', 'Please save the page first'));
        return;
      }
      // 先保存当前状态
      await this.save();
      // 获取租户子域名
      const tenantSubdomain = window.tenantSubdomain || localStorage.getItem('tenant_subdomain') || 'test';
      window.open(`/co/${tenantSubdomain}/${this.pageData.slug}/`, '_blank');
    });

    // Undo 按鈕
    document.getElementById('undoBtn').addEventListener('click', () => {
      this.undo();
    });

    // Redo 按鈕
    document.getElementById('redoBtn').addEventListener('click', () => {
      this.redo();
    });

    // 鍵盤快捷鍵：Ctrl+Z (Undo), Ctrl+Y (Redo)
    document.addEventListener('keydown', (e) => {
      if (e.ctrlKey || e.metaKey) {
        if (e.key === 'z' && !e.shiftKey) {
          e.preventDefault();
          this.undo();
        } else if (e.key === 'y' || (e.key === 'z' && e.shiftKey)) {
          e.preventDefault();
          this.redo();
        }
      }
    });

    // 桌面視圖按鈕
    document.getElementById('desktopViewBtn').addEventListener('click', () => {
      this.setViewMode('desktop');
    });

    // 手機視圖按鈕
    document.getElementById('mobileViewBtn').addEventListener('click', () => {
      this.setViewMode('mobile');
    });

    // Slug 自動生成（基於名稱）- 在 modal 中
    // 注意：由於 modal 是動態的，我們需要在 modal 顯示時綁定事件
    // 這裡先不綁定，在 showPageSettings 中綁定
  },

  navigateToPrevPage() {
    if (this.currentPageIndex > 0) {
      const prevPage = this.allPages[this.currentPageIndex - 1];
      if (prevPage && prevPage.id) {
        const target = `/pages/${prevPage.id}/edit`;
        if (typeof Router !== 'undefined' && Router.go) {
          Router.go(target);
        } else {
          window.location.href = target;
        }
      }
    }
  },

  navigateToNextPage() {
    if (this.currentPageIndex >= 0 && this.currentPageIndex < this.allPages.length - 1) {
      const nextPage = this.allPages[this.currentPageIndex + 1];
      if (nextPage && nextPage.id) {
        const target = `/pages/${nextPage.id}/edit`;
        if (typeof Router !== 'undefined' && Router.go) {
          Router.go(target);
        } else {
          window.location.href = target;
        }
      }
    }
  },

  setViewMode(mode) {
    this.viewMode = mode;
    const container = document.getElementById('componentsContainer');
    const desktopBtn = document.getElementById('desktopViewBtn');
    const mobileBtn = document.getElementById('mobileViewBtn');
    
    if (mode === 'mobile') {
      container.style.maxWidth = '375px';
      container.style.margin = '0 auto';
      container.classList.add('mobile-preview');
      // 设置容器查询上下文，模拟移动设备视口
      container.style.containerType = 'inline-size';
      container.style.width = '375px';
      desktopBtn.classList.remove('active');
      mobileBtn.classList.add('active');
    } else {
      container.style.maxWidth = '100%';
      container.style.margin = '0';
      container.style.width = 'auto';
      container.style.containerType = 'normal';
      container.classList.remove('mobile-preview');
      desktopBtn.classList.add('active');
      mobileBtn.classList.remove('active');
    }
  },

  addComponent(type, afterIndex = null) {
    const defaultData = this.getDefaultComponentData(type);
    const component = {
      id: null, // 新建時為 null
      component_type: type,
      component_data: defaultData,
      sort_order: this.components.length,
      is_active: true
    };
    
    // 檢查是否要添加到區塊容器的欄位
    if (this.addComponentToSectionColumn) {
      const { sectionIndex, columnIndex } = this.addComponentToSectionColumn;
      const section = this.components[sectionIndex];
      
      if (section && section.component_type === 'section') {
        // 確保 column_children 結構存在
        if (!section.component_data.column_children) {
          const columns = section.component_data.columns || 1;
          section.component_data.column_children = Array(columns).fill(0).map(() => []);
        }
        
        // 確保欄位索引有效
        while (section.component_data.column_children.length <= columnIndex) {
          section.component_data.column_children.push([]);
        }
        
        // 添加到指定欄位
        if (!section.component_data.column_children[columnIndex]) {
          section.component_data.column_children[columnIndex] = [];
        }
        section.component_data.column_children[columnIndex].push(component);
        
        this.saveState();
        
        // 保存当前选中的元件索引，以便在重新渲染后恢复选中状态和 padding
        const wasSelected = this.selectedComponent && this.components.indexOf(this.selectedComponent) === sectionIndex;
        
        this.renderComponents();
        
        // 如果之前是选中状态，恢复选中状态和 padding
        if (wasSelected) {
          setTimeout(() => {
            const sectionItem = document.querySelector(`.component-item[data-index="${sectionIndex}"]`);
            if (sectionItem) {
              this.selectComponent(section);
            }
          }, 0);
        }
        
        // 滚动到區塊容器元件
        setTimeout(() => {
          const sectionItem = document.querySelector(`.component-item[data-index="${sectionIndex}"]`);
          if (sectionItem) {
            // 直接滚动到區塊容器位置，确保可见
            sectionItem.scrollIntoView({ behavior: 'smooth', block: 'center' });
          }
        }, 150);
        
        this.cancelAddComponentAfter();
        return;
      }
    }
    
    if (afterIndex !== null && afterIndex >= 0 && afterIndex < this.components.length) {
      // 在指定位置後添加
      this.components.splice(afterIndex + 1, 0, component);
      // 更新所有元件的 sort_order
      this.components.forEach((comp, i) => {
        comp.sort_order = i;
      });
    } else {
      // 添加到末尾
    this.components.push(component);
    }
    
    this.saveState(); // 保存狀態到歷史記錄
    this.renderComponents();
    this.selectComponent(component);
    
    // 滚动到新添加的元件
    setTimeout(() => {
      const index = this.components.indexOf(component);
      const item = document.querySelector(`.component-item[data-index="${index}"]`);
      if (item) {
        // 直接滚动到元件位置，确保可见
        item.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }
    }, 150);
    
    // 取消添加模式
    if (this.addComponentAfterIndex !== null) {
      this.cancelAddComponentAfter();
    }
  },
  
  bindComponentAddButtons() {
    // 移除舊的事件監聽器（通過克隆節點）
    const library = document.getElementById('componentLibrary');
    if (library) {
      const newLibrary = library.cloneNode(true);
      library.parentNode.replaceChild(newLibrary, library);
    }
    
    // 綁定元件庫中的添加按鈕
    document.querySelectorAll('.component-add-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const type = btn.dataset.type;
        if (this.addComponentAfterIndex !== null) {
          // 如果在添加模式，在指定位置後添加
          this.addComponent(type, this.addComponentAfterIndex);
        } else if (this.addComponentToSectionColumn) {
          // 如果要添加到區塊容器的欄位
          this.addComponent(type);
        } else {
          // 否則添加到末尾
          this.addComponent(type);
        }
        // 添加元件後關閉元件庫
        this.hideComponentLibrary();
      });
    });
  },
  
  showComponentLibrary() {
    // 先關閉區塊庫
    this.hideBlockLibrary();
    
    const libraryPanel = document.getElementById('componentLibraryPanel');
    const libraryIcon = document.getElementById('componentLibraryIcon');
    const pebar = document.getElementById('pebar');
    
    if (libraryPanel && libraryIcon) {
      // 獲取按鈕的位置
      const iconRect = libraryIcon.getBoundingClientRect();
      const pebarHeight = pebar ? pebar.offsetHeight : 73;
      
      // 設置 panel 的位置，使其跟隨按鈕
      libraryPanel.style.left = `${iconRect.right}px`;
      libraryPanel.style.top = `${iconRect.top}px`;
      libraryPanel.style.maxHeight = `calc(100vh - ${iconRect.top}px)`;
      libraryPanel.style.display = 'block';
      
      // 監聽滾動和窗口大小變化，更新位置
      const updatePanelPosition = () => {
        if (libraryPanel.style.display !== 'none' && libraryIcon) {
          const iconRect = libraryIcon.getBoundingClientRect();
          libraryPanel.style.left = `${iconRect.right}px`;
          libraryPanel.style.top = `${iconRect.top}px`;
          libraryPanel.style.maxHeight = `calc(100vh - ${iconRect.top}px)`;
        }
      };
      
      // 移除舊的監聽器（如果有的話）
      if (this.panelPositionUpdateHandler) {
        window.removeEventListener('scroll', this.panelPositionUpdateHandler);
        window.removeEventListener('resize', this.panelPositionUpdateHandler);
      }
      
      // 添加新的監聽器
      this.panelPositionUpdateHandler = updatePanelPosition;
      window.addEventListener('scroll', updatePanelPosition, true);
      window.addEventListener('resize', updatePanelPosition);
    }
  },

  hideComponentLibrary() {
    const libraryPanel = document.getElementById('componentLibraryPanel');
    if (libraryPanel) {
      libraryPanel.style.display = 'none';
    }
  },

  showAddComponentAfter(index) {
    // 設置添加模式
    this.addComponentAfterIndex = index;
    this.addComponentToSectionColumn = null;
    const component = this.components[index];
    const componentLabel = this.getComponentTypeLabel(component.component_type);
    
    // 顯示元件庫
    this.showComponentLibrary();
    
    // 顯示提示
    const prompt = document.getElementById('addComponentAfterPrompt');
    const promptText = document.getElementById('addComponentAfterText');
    if (prompt && promptText) {
      const tpl = this.t('pages.pageEditor.prompts.addAfter', 'Select a component to add after "{name}"');
      promptText.textContent = String(tpl).replace('{name}', componentLabel);
      prompt.style.display = 'block';
    }
    
    // 高亮所有添加按鈕
    document.querySelectorAll('.component-add-btn').forEach(btn => {
      btn.classList.add('btn-primary');
      btn.classList.remove('btn-outline-primary');
    });
    
    // 高亮當前元件的 +元件 按鈕
    const item = document.querySelector(`.component-item[data-index="${index}"]`);
    if (item) {
      const addBtn = item.querySelector('.add-component-after-btn');
      if (addBtn) {
        addBtn.classList.add('active');
      }
    }
  },
  
  cancelAddComponentAfter() {
    // 取消添加模式
    this.addComponentAfterIndex = null;
    this.addComponentToSectionColumn = null;
    
    // 隱藏提示
    const prompt = document.getElementById('addComponentAfterPrompt');
    if (prompt) {
      prompt.style.display = 'none';
    }
    
    // 恢復所有添加按鈕樣式
    document.querySelectorAll('.component-add-btn').forEach(btn => {
      btn.classList.remove('btn-primary');
      btn.classList.add('btn-outline-primary');
    });
    
    // 恢復所有 +元件 按鈕樣式
    document.querySelectorAll('.add-component-after-btn').forEach(btn => {
      btn.classList.remove('active');
    });
  },
  
  showAddComponentToSectionColumn(sectionIndex, columnIndex) {
    // 設置添加模式到區塊容器的欄位
    this.addComponentAfterIndex = null;
    this.addComponentToSectionColumn = { sectionIndex, columnIndex };
    const section = this.components[sectionIndex];
    const sectionLabel = this.getComponentTypeLabel(section.component_type);
    
    // 顯示元件庫
    this.showComponentLibrary();
    
    // 顯示提示
    const prompt = document.getElementById('addComponentAfterPrompt');
    const promptText = document.getElementById('addComponentAfterText');
    if (prompt && promptText) {
      const tpl = this.t('pages.pageEditor.prompts.addToSectionColumn', 'Select a component to add into column {n}');
      promptText.textContent = String(tpl).replace('{n}', String(columnIndex + 1));
      prompt.style.display = 'block';
    }
    
    // 高亮所有添加按鈕
    document.querySelectorAll('.component-add-btn').forEach(btn => {
      btn.classList.add('btn-primary');
      btn.classList.remove('btn-outline-primary');
    });
    
    // 高亮當前欄位的 +元件 按鈕
    const sectionItem = document.querySelector(`.component-item[data-index="${sectionIndex}"]`);
    if (sectionItem) {
      const addBtn = sectionItem.querySelector(`.section-column-add-btn[data-column-index="${columnIndex}"]`);
      if (addBtn) {
        addBtn.classList.add('active');
      }
    }
  },

  getDefaultComponentData(type) {
    const defaults = {
      hero: {
        title: this.t('pages.pageEditor.defaults.hero.title', 'Welcome Title'),
        subtitle: this.t('pages.pageEditor.defaults.hero.subtitle', 'Subtitle text'),
        background_image: '',
        button_text: this.t('pages.pageEditor.defaults.hero.buttonText', 'Learn more'),
        button_link: '#'
      },
      text: {
        content: this.t('pages.pageEditor.defaults.text.content', 'Your text goes here...')
      },
      image: {
        src: '',
        alt: this.t('pages.pageEditor.defaults.image.alt', 'Image'),
        width: '100%'
      },
      button: {
        text: this.t('pages.pageEditor.defaults.button.text', 'Button'),
        link: '#',
        style: 'primary'
      },
      section: {
        background_color: '',
        padding: '10px',
        columns: 1,
        column_children: [[]] // 每個欄位的子元件數組，例如：[[], [], []] 表示3欄
      },
      heading: {
        text: this.t('pages.pageEditor.defaults.heading.text', 'Heading'),
        level: 'h2',
        align: 'left'
      },
      header: {
        logo: '',
        logo_text: this.t('pages.pageEditor.defaults.common.logoText', 'Logo'),
        menu_items: []
      },
      nav: {
        logo: '',
        logo_text: this.t('pages.pageEditor.defaults.common.logoText', 'Logo'),
        menu_items: [],
        show_login_icon: true,
        show_cart_icon: true,
        fixed: false
      },
      'product-list': {
        category_id: null,
        limit: 12,
        columns: 3,
        full_list: false,
        show_product_type_filter: false,
        show_brand_filter: false
      },
      'service-list': {
        service_type_id: null,
        limit: 12,
        columns: 3,
        service_detail_page: ''
      },
      footer: {
        logo: '',
        column1_content: this.t('pages.pageEditor.defaults.footer.column1Content', 'Company intro text'),
        column2_menu_items: [],
        column3_menu_items: [],
        column4_menu_items: [],
        copyright: this.t('pages.pageEditor.defaults.footer.copyright', '© 2025 All rights reserved'),
        bg_color: '',
        text_color: '',
        padding: '2rem 0'
      },
      list: {
        menu_items: []
      },
      'order-list': {
        limit: 10,
        show_status_filter: true,
        show_date_filter: true
      },
      'login-register': {
        show_login: true,
        show_register: true,
        login_method: 'phone_or_email',
        redirect_after_login: '/',
        redirect_after_register: '/'
      },
      cart: {
        show_checkout_button: true,
        show_continue_shopping: true
      },
      checkout: {
        show_shipping_form: true,
        show_payment_form: true,
        payment_methods: ['credit_card', 'paypal']
      },
      'user-area': {
        show_profile: true,
        show_orders: true,
        show_addresses: true
      },
      'service-booking': {
        title: this.t('pages.pageEditor.defaults.serviceBooking.title', '服務預約'),
        subtitle: this.t('pages.pageEditor.defaults.serviceBooking.subtitle', '請依照以下步驟完成預約'),
        step1_title: this.t('pages.pageEditor.defaults.serviceBooking.step1Title', '選擇服務'),
        step2_title: this.t('pages.pageEditor.defaults.serviceBooking.step2Title', '選擇時間'),
        step3_title: this.t('pages.pageEditor.defaults.serviceBooking.step3Title', '確認資料'),
        show_service_select: true,
        show_staff_select: true,
        show_date_picker: true,
        show_time_slots: true,
        require_login: false,
        success_message: this.t('pages.pageEditor.defaults.serviceBooking.successMessage', '預約成功！我們會盡快與您聯繫確認。'),
        button_text: this.t('pages.pageEditor.defaults.serviceBooking.buttonText', '確認預約'),
        primary_color: '#0d6efd'
      },
      'dining-menu': {
        title: this.t('pages.pageEditor.defaults.diningMenu.title', '菜單'),
        subtitle: this.t('pages.pageEditor.defaults.diningMenu.subtitle', '瀏覽餐點與價格'),
        height: '720px',
        show_title: true
      },
      'dining-table-reservation': {
        title: this.t('pages.pageEditor.defaults.diningReservation.title', '預約餐桌'),
        subtitle: this.t('pages.pageEditor.defaults.diningReservation.subtitle', '填寫聯絡資訊完成預約'),
        height: '720px',
        show_title: true
      },
      'banner-slider': {
        slides: [
          {
            image: '',
            title: this.t('pages.pageEditor.defaults.bannerSlider.title1', '主打標題 1'),
            subtitle: this.t('pages.pageEditor.defaults.bannerSlider.subtitle1', '描述文字 1'),
            button_text: this.t('pages.pageEditor.defaults.bannerSlider.buttonText', '了解更多'),
            button_link: '#'
          },
          {
            image: '',
            title: this.t('pages.pageEditor.defaults.bannerSlider.title2', '主打標題 2'),
            subtitle: this.t('pages.pageEditor.defaults.bannerSlider.subtitle2', '描述文字 2'),
            button_text: this.t('pages.pageEditor.defaults.bannerSlider.buttonText', '了解更多'),
            button_link: '#'
          }
        ],
        height: '360px',
        autoplay: true,
        interval: 5000,
        show_indicators: true,
        show_arrows: true,
        text_align: 'left',
        text_color: '#ffffff',
        overlay_color: 'rgba(0, 0, 0, 0.45)'
      },
      'google-map': {
        lat: 25.033964,
        lng: 121.564468,
        zoom: 14,
        height: '400px',
        marker_title: '',
        address: ''
      },
      'custom-html': {
        html_content: '<p>Your HTML here</p>'
      }
    };
    return defaults[type] || {};
  },

  async loadProductMeta() {
    if (this.productMetaLoading || this.productMetaLoaded) return;
    this.productMetaLoading = true;
    try {
      const [typesResp, categoriesResp, attributesResp] = await Promise.all([
        App.apiRequest('/product-types?limit=1000'),
        App.apiRequest('/products/categories?limit=1000'),
        App.apiRequest('/product-attributes?limit=1000')
      ]);
      this.productTypes = typesResp.data || typesResp || [];
      this.productCategories = categoriesResp.data || categoriesResp || [];
      this.productAttributes = attributesResp.data || attributesResp || [];
      this.productMetaLoaded = true;
    } catch (error) {
      console.error('載入產品資訊失敗:', error);
      this.productTypes = [];
      this.productCategories = [];
      this.productAttributes = [];
      this.productMetaLoaded = true;
    } finally {
      this.productMetaLoading = false;
      if (this.selectedComponent && this.selectedComponent.component_type === 'product-list') {
        this.renderProperties(this.selectedComponent);
      }
    }
  },

  formatProductTypeName(type) {
    if (!type) return '';
    const name = type.name || type.Name || type.title || '';
    const parentName = type.parent?.name || type.parent?.Name || type.parent_name || type.parentName || '';
    return parentName ? `${parentName} / ${name}` : name;
  },

  renderProductMetaPanel() {
    if (!this.productMetaLoaded && !this.productMetaLoading) {
      this.loadProductMeta();
    }

    if (this.productMetaLoading) {
      return `
        <div class="mt-4">
          <h6 class="mb-2">${this.t('pages.pageEditor.labels.productList.metaTitle', 'Product meta')}</h6>
          <div class="text-muted small">${this.t('common.loading', '載入中...')}</div>
        </div>
      `;
    }

    const typeLabels = (this.productTypes || [])
      .map((type) => this.formatProductTypeName(type))
      .filter(Boolean);
    const categoryLabels = (this.productCategories || [])
      .map((cat) => (typeof cat === 'string' ? cat : cat.name || cat.Name || cat.title || ''))
      .filter(Boolean);
    const attributeLabels = (this.productAttributes || [])
      .map((attr) => attr.name || attr.Name || attr.title || '')
      .filter(Boolean);

    const renderBadges = (items, emptyText) => {
      if (!items.length) return `<div class="text-muted small">${emptyText}</div>`;
      return items
        .map((label) => `<span class="badge bg-light text-dark border me-1 mb-1">${label}</span>`)
        .join('');
    };

    return `
      <div class="mt-4">
        <h6 class="mb-2">${this.t('pages.pageEditor.labels.productList.metaTitle', 'Product meta')}</h6>
        <div class="mb-3">
          <div class="small text-muted mb-1">${this.t('pages.pageEditor.labels.productList.types', 'Types')}</div>
          <div>${renderBadges(typeLabels, this.t('pages.pageEditor.labels.productList.noTypes', 'No product types'))}</div>
        </div>
        <div class="mb-3">
          <div class="small text-muted mb-1">${this.t('pages.pageEditor.labels.productList.categories', 'Categories')}</div>
          <div>${renderBadges(categoryLabels, this.t('pages.pageEditor.labels.productList.noCategories', 'No categories'))}</div>
        </div>
        <div class="mb-3">
          <div class="small text-muted mb-1">${this.t('pages.pageEditor.labels.productList.attributes', 'Attributes')}</div>
          <div>${renderBadges(attributeLabels, this.t('pages.pageEditor.labels.productList.noAttributes', 'No attributes'))}</div>
        </div>
      </div>
    `;
  },

  renderComponents() {
    const container = document.getElementById('componentsContainer');
    if (this.components.length === 0) {
      const emptyHint = (typeof I18n !== 'undefined' && I18n.t)
        ? I18n.t('pages.pageEditor.emptyStateHint')
        : '點擊左上角元件庫圖標或 +元件 按鈕添加元件到頁面';
      container.innerHTML = `
        <div class="text-center text-muted py-5" id="emptyState" style="display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 400px;">
          <div style="display: flex; flex-direction: column; align-items: center; justify-content: center;">
            <i class="bi bi-layout-wtf mb-3" style="font-size: 3rem; opacity: 0.3;"></i>
            <p style="margin: 0;" data-i18n="pages.pageEditor.emptyStateHint">${emptyHint}</p>
          </div>
        </div>
      `;
      return;
    }

    container.innerHTML = this.components.map((comp, index) => {
      const isNavBottomComponent = comp.component_type === 'nav' && (comp.component_data?.menu_position || 'right') === 'bottom';
      const linkedBadge = comp.block_id
        ? `<span class="badge bg-info text-white ms-1" title="${this.t('pages.pageEditor.linkedBlock', 'Linked Block')} — ${this.t('pages.pageEditor.linkedBlockHint', 'Changes to this block affect all pages using it')}"><i class="bi bi-link-45deg"></i></span>`
        : '';
      return `
        <div class="component-item${comp.block_id ? ' linked-block' : ''}${isNavBottomComponent ? ' nav-bottom-component' : ''}" data-index="${index}" data-component-id="${comp.id || 'new'}" draggable="true">
          <i class="bi bi-grip-vertical drag-handle"></i>
          <div class="component-actions">
            <button class="btn btn-sm" onclick="PageEditor.moveComponentUp(${index})" ${index === 0 ? 'disabled' : ''} title="${this.t('pages.pageEditor.actions.moveUp', 'Move up')}">
              <i class="bi bi-arrow-up"></i>
            </button>
            <button class="btn btn-sm" onclick="PageEditor.moveComponentDown(${index})" ${index === this.components.length - 1 ? 'disabled' : ''} title="${this.t('pages.pageEditor.actions.moveDown', 'Move down')}">
              <i class="bi bi-arrow-down"></i>
            </button>
            <button class="btn btn-sm" onclick="PageEditor.editComponent(${index})" title="${this.t('pages.pageEditor.actions.edit', 'Edit')}">
              <i class="bi bi-pencil"></i>
            </button>
            ${comp.block_id
              ? `<button class="btn btn-sm btn-outline-warning" onclick="PageEditor.unlinkBlock(${index})" title="${this.t('pages.pageEditor.unlinkBlock', 'Unlink from block (make independent copy)')}"><i class="bi bi-link-45deg"></i></button>`
              : `<button class="btn btn-sm btn-outline-success" onclick="PageEditor.saveComponentAsBlock(${index})" title="${this.t('pages.pageEditor.saveAsBlock', 'Save as Block')}"><i class="bi bi-save"></i></button>`
            }
            <button class="btn btn-sm btn-outline-danger" onclick="PageEditor.deleteComponent(${index})" title="${this.t('pages.pageEditor.actions.delete', 'Delete')}">
              <i class="bi bi-trash"></i>
            </button>
          </div>
          <div class="component-preview">${linkedBadge}${this.renderComponentPreview(comp)}</div>
          <div class="component-bottom-buttons">
            <button class="btn btn-sm add-component-after-btn" data-index="${index}" title="${this.t('pages.pageEditor.actions.addAfter', 'Add after')}">
              <i class="bi bi-plus-lg"></i> <span>${this.t('pages.pageEditor.addComponent', 'Component')}</span>
            </button>
            <button class="btn btn-sm edit-component-btn" data-index="${index}" title="${this.t('pages.pageEditor.actions.editProperties', 'Edit properties')}">
              <i class="bi bi-pencil"></i>
            </button>
          </div>
        </div>
      `;
    }).join('');

    // 初始化 Google Map 預覽
    this.initGoogleMapPreviews();
    
    // 綁定所有可編輯元素的事件
    container.querySelectorAll('.component-preview').forEach(preview => {
      this.bindEditableElements(preview);
    });
    
    // 绑定所有 section-column-add-btn 的点击事件（在 container 级别，因为按钮在 section-column-content 内部）
    container.querySelectorAll('.section-column-add-btn').forEach(btn => {
      // 移除旧的事件监听器（通过克隆节点）
      const newBtn = btn.cloneNode(true);
      btn.parentNode.replaceChild(newBtn, btn);
      
      newBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        e.preventDefault();
        const sectionIndex = parseInt(newBtn.dataset.sectionIndex);
        const columnIndex = parseInt(newBtn.dataset.columnIndex);
        if (!isNaN(sectionIndex) && !isNaN(columnIndex)) {
          this.showAddComponentToSectionColumn(sectionIndex, columnIndex);
        }
      });
    });
    
    // 禁用 view area 内所有超链接和 redirect
    container.querySelectorAll('a[href]').forEach(link => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        return false;
      });
    });
    
    // 綁定 column 內元件的拖拽事件
    container.querySelectorAll('.column-child-item').forEach(childItem => {
      childItem.addEventListener('dragstart', (e) => {
        e.stopPropagation();
        childItem.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', 'column-child');
      });
      
      childItem.addEventListener('dragend', (e) => {
        childItem.classList.remove('dragging');
        container.querySelectorAll('.drag-over-column').forEach(col => {
          col.classList.remove('drag-over-column');
        });
      });
    });

    // 綁定點擊選擇
    container.querySelectorAll('.component-item').forEach(item => {
      item.addEventListener('click', (e) => {
        // 如果点击的是可编辑元素（文字、按钮、图片），不选择组件
        if (e.target.closest('.editable-text[contenteditable="true"]') || 
            e.target.matches('button, a.btn, input[type="button"], input[type="submit"]') ||
            e.target.matches('img') ||
            e.target.closest('.component-actions') ||
            e.target.closest('.column-child-actions')) {
          // 这些元素已经有自己的事件处理，不触发组件选择
          e.stopPropagation();
          return;
        }
        
        // 如果点击的是功能按钮，阻止事件冒泡并保持显示
        if (e.target.closest('.component-actions')) {
          e.stopPropagation();
          // 确保功能按钮保持显示
          const actions = item.querySelector('.component-actions');
          if (actions) {
            actions.style.display = 'flex';
          }
          return;
        }
        // 如果点击的是 edit-component-btn，显示元件属性
        if (e.target.closest('.edit-component-btn')) {
          e.stopPropagation();
          e.preventDefault();
          const btn = e.target.closest('.edit-component-btn');
          if (btn) {
            const index = parseInt(btn.dataset.index);
            if (!isNaN(index)) {
              this.selectComponent(this.components[index]);
            }
          }
          return;
        }
        
        // 如果点击的是 +元件 按钮（包括區塊容器的欄位按鈕）
        const addBtn = e.target.closest('.add-component-after-btn');
        const sectionAddBtn = e.target.closest('.section-column-add-btn');
        if (addBtn || sectionAddBtn) {
          e.stopPropagation();
          e.preventDefault();
          
          // 檢查是否是區塊容器的欄位按鈕
          if (sectionAddBtn) {
            const sectionIndex = parseInt(sectionAddBtn.dataset.sectionIndex);
            const columnIndex = parseInt(sectionAddBtn.dataset.columnIndex);
            if (!isNaN(sectionIndex) && !isNaN(columnIndex)) {
              this.showAddComponentToSectionColumn(sectionIndex, columnIndex);
            }
          } else if (addBtn) {
            // 一般元件的 +元件 按鈕
            const index = parseInt(addBtn.dataset.index);
            if (!isNaN(index)) {
              this.showAddComponentAfter(index);
            }
          }
          return;
        }
        
        // 其他情况：选择组件
        const index = parseInt(item.dataset.index);
        this.selectComponent(this.components[index]);
      });
      
      // 绑定 edit-component-btn 的点击事件
      item.querySelectorAll('.edit-component-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
          e.stopPropagation();
          e.preventDefault();
          const index = parseInt(btn.dataset.index);
          if (!isNaN(index)) {
            this.selectComponent(this.components[index]);
          }
        });
      });
      
      // 单独绑定 add-component-after-btn 的点击事件，确保能响应
      item.querySelectorAll('.add-component-after-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
          e.stopPropagation();
          e.preventDefault();
          const index = parseInt(btn.dataset.index);
          if (!isNaN(index)) {
            this.showAddComponentAfter(index);
          }
        });
      });
      
      // 绑定 function bar 的自动收起逻辑
      const componentActions = item.querySelector('.component-actions');
      if (componentActions) {
        let actionsTimeout = null;
        let isHoveringActions = false; // hover 到 function bar 的标志
        
        // 显示完整 function bar
        const showActions = () => {
          if (actionsTimeout) {
            clearTimeout(actionsTimeout);
            actionsTimeout = null;
          }
          // 使用 setProperty 确保覆盖之前的样式
          componentActions.style.setProperty('transform', 'translateX(0)', 'important');
          componentActions.style.setProperty('opacity', '1', 'important');
        };
        
        // 2秒后收起 function bar
        const hideActions = () => {
          if (!isHoveringActions) {
            actionsTimeout = setTimeout(() => {
              componentActions.style.setProperty('transform', 'translateX(calc(100% - 12px))', 'important');
              componentActions.style.setProperty('opacity', '0.7', 'important');
            }, 2000);
          }
        };
        
        // 鼠标进入 function bar 时保持显示
        componentActions.addEventListener('mouseenter', () => {
          isHoveringActions = true;
          // 清除可能存在的定时器
          if (actionsTimeout) {
            clearTimeout(actionsTimeout);
            actionsTimeout = null;
          }
          showActions();
        });
        
        // 鼠标离开 function bar 时，2秒后收起
        componentActions.addEventListener('mouseleave', () => {
          isHoveringActions = false;
          hideActions();
        });
        
        // 点击选中时显示完整，2秒后自动收起
        // 注意：实际的2秒收起逻辑在 selectComponent 函数中处理
        // 这里只负责显示
        item.addEventListener('click', () => {
          showActions();
        });
        
        // 初始状态：function bar 应该是收起的
        componentActions.style.setProperty('transform', 'translateX(calc(100% - 12px))', 'important');
        componentActions.style.setProperty('opacity', '0.7', 'important');
      }
      
      // 检查元件高度，如果小于50px则添加small-component类
      const checkComponentHeight = () => {
        const preview = item.querySelector('.component-preview');
        if (preview) {
          const height = preview.offsetHeight;
          if (height < 50) {
            item.classList.add('small-component');
          } else {
            item.classList.remove('small-component');
          }
        }
      };
      
      // 初始检查
      checkComponentHeight();
      
      // 监听尺寸变化
      const resizeObserver = new ResizeObserver(() => {
        checkComponentHeight();
      });
      const preview = item.querySelector('.component-preview');
      if (preview) {
        resizeObserver.observe(preview);
      }
      
      // function bar 的事件绑定已在上面完成，这里不再重复绑定
      
      // 检查元件高度（在鼠标移入时也检查）
      item.addEventListener('mouseenter', () => {
        checkComponentHeight();
      });
    });

    // 綁定拖放事件
    this.bindDragAndDrop(container);
    
    // 重新綁定添加元件按鈕（因為可能重新渲染了）
    this.bindComponentAddButtons();
  },

  bindDragAndDrop(container) {
    const items = container.querySelectorAll('.component-item');
    let draggedElement = null;
    let draggedIndex = null;

    // 監聽容器範圍外的拖動
    document.addEventListener('dragover', (e) => {
      if (draggedElement) {
        const containerRect = container.getBoundingClientRect();
        const isInsideContainer = (
          e.clientX >= containerRect.left &&
          e.clientX <= containerRect.right &&
          e.clientY >= containerRect.top &&
          e.clientY <= containerRect.bottom
        );
        
        if (!isInsideContainer) {
          e.dataTransfer.dropEffect = 'none';
        }
      }
    });

    document.addEventListener('drop', (e) => {
      if (draggedElement) {
        const containerRect = container.getBoundingClientRect();
        const isInsideContainer = (
          e.clientX >= containerRect.left &&
          e.clientX <= containerRect.right &&
          e.clientY >= containerRect.top &&
          e.clientY <= containerRect.bottom
        );
        
        if (!isInsideContainer && !e.target.closest('.component-item')) {
          e.preventDefault();
          App.showError(this.t('pages.pageEditor.errors.dropOutside', 'Cannot drop component outside the editor area'));
          draggedElement.classList.remove('dragging');
          items.forEach(i => {
            i.classList.remove('drag-over');
          });
          draggedElement = null;
          draggedIndex = null;
          return false;
        }
      }
    });

    items.forEach((item, index) => {
      // 拖動開始
      item.addEventListener('dragstart', (e) => {
        draggedElement = item;
        draggedIndex = parseInt(item.dataset.index);
        item.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/html', item.innerHTML);
        // 設置拖動圖像為半透明
        e.dataTransfer.setDragImage(item, 0, 0);
      });

      // 拖動結束
      item.addEventListener('dragend', (e) => {
        item.classList.remove('dragging');
        // 移除所有拖動相關的樣式
        items.forEach(i => {
          i.classList.remove('drag-over');
        });
        
        // 檢查是否在容器範圍內
        const containerRect = container.getBoundingClientRect();
        const isInsideContainer = (
          e.clientX >= containerRect.left &&
          e.clientX <= containerRect.right &&
          e.clientY >= containerRect.top &&
          e.clientY <= containerRect.bottom
        );
        
        if (!isInsideContainer) {
          App.showError(this.t('pages.pageEditor.errors.dropOutside', 'Cannot drop component outside the editor area'));
        }
      });

      // 拖動進入
      item.addEventListener('dragenter', (e) => {
        e.preventDefault();
        if (item !== draggedElement) {
          item.classList.add('drag-over');
        }
      });

      // 拖動離開
      item.addEventListener('dragleave', (e) => {
        item.classList.remove('drag-over');
      });

      // 拖動懸停（允許放置）
      item.addEventListener('dragover', (e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        if (item !== draggedElement && !item.classList.contains('drag-over')) {
          item.classList.add('drag-over');
        }
      });

      // 放置
      item.addEventListener('drop', (e) => {
        e.preventDefault();
        item.classList.remove('drag-over');

        if (draggedElement && draggedElement !== item) {
          const dropIndex = parseInt(item.dataset.index);
          const dropComponent = this.components[dropIndex];
          
          // 檢查是否拖入區塊容器
          if (dropComponent.component_type === 'section') {
            // 將元件添加到區塊容器的第一欄（column_children[0]）
            const draggedComponent = this.components[draggedIndex];
            
            // 初始化 column_children 結構
            if (!dropComponent.component_data.column_children) {
              const columns = dropComponent.component_data.columns || 1;
              dropComponent.component_data.column_children = Array(columns).fill(0).map(() => []);
              // 如果有舊的 children，遷移到第一欄
              if (dropComponent.component_data.children && Array.isArray(dropComponent.component_data.children)) {
                dropComponent.component_data.column_children[0] = dropComponent.component_data.children;
                delete dropComponent.component_data.children;
              }
            }
            
            // 確保第一欄存在
            if (!dropComponent.component_data.column_children[0]) {
              dropComponent.component_data.column_children[0] = [];
            }
            
            // 添加到第一欄
            dropComponent.component_data.column_children[0].push(draggedComponent);
            
            // 從主元件列表中移除
            this.components.splice(draggedIndex, 1);
            
            // 更新 sort_order
            this.components.forEach((comp, i) => {
              comp.sort_order = i;
            });
            
            // 保存狀態到歷史記錄
            this.saveState();
            
            // 重新渲染
            this.renderComponents();
            
            // 重新選中區塊容器
            this.selectComponent(dropComponent);
          } else {
            // 普通拖放：重新排序元件數組
            const draggedComponent = this.components[draggedIndex];
            this.components.splice(draggedIndex, 1);
            this.components.splice(dropIndex, 0, draggedComponent);
            
            // 更新 sort_order
            this.components.forEach((comp, i) => {
              comp.sort_order = i;
            });
            
            // 保存狀態到歷史記錄
            this.saveState();
            
            // 重新渲染
            this.renderComponents();
            
            // 重新選中被拖動的元件
            const newIndex = this.components.indexOf(draggedComponent);
            this.selectComponent(draggedComponent);
          }
        }

        draggedElement = null;
        draggedIndex = null;
      });
    });

    // 為區塊容器的每個欄位添加拖放支持
    setTimeout(() => {
      const sectionColumns = container.querySelectorAll('.section-column-content');
      sectionColumns.forEach((column) => {
        const sectionIndex = parseInt(column.dataset.sectionIndex);
        const columnIndex = parseInt(column.dataset.columnIndex);
        
        if (isNaN(sectionIndex) || isNaN(columnIndex)) return;
        
        const section = this.components[sectionIndex];
        if (!section || section.component_type !== 'section') return;

        column.addEventListener('dragover', (e) => {
          e.preventDefault();
          e.stopPropagation();
          e.dataTransfer.dropEffect = 'move';
          column.classList.add('drag-over-column');
        });

        column.addEventListener('dragleave', (e) => {
          column.classList.remove('drag-over-column');
        });

        column.addEventListener('drop', (e) => {
          e.preventDefault();
          e.stopPropagation();
          column.classList.remove('drag-over-column');

          // 獲取當前拖動的元件
          const draggedItem = container.querySelector('.dragging');
          if (!draggedItem) return;
          
          // 檢查是否是 column 內的子元件
          const isColumnChild = draggedItem.classList.contains('column-child-item');
          let draggedComponent;
          let isChild = false;
          
          if (isColumnChild) {
            // 從 column_children 中獲取元件
            const childSectionIndex = parseInt(draggedItem.dataset.sectionIndex);
            const childColumnIndex = parseInt(draggedItem.dataset.columnIndex);
            const childIndex = parseInt(draggedItem.dataset.childIndex);
            
            if (!isNaN(childSectionIndex) && !isNaN(childColumnIndex) && !isNaN(childIndex)) {
              const childSection = this.components[childSectionIndex];
              if (childSection && childSection.component_type === 'section' && 
                  childSection.component_data.column_children && 
                  childSection.component_data.column_children[childColumnIndex]) {
                draggedComponent = childSection.component_data.column_children[childColumnIndex][childIndex];
                isChild = true;
                
                // 從原位置移除
                childSection.component_data.column_children[childColumnIndex].splice(childIndex, 1);
              }
            }
          } else {
            // 從主元件列表中獲取
          const draggedIdx = parseInt(draggedItem.dataset.index);
          if (isNaN(draggedIdx)) return;
            draggedComponent = this.components[draggedIdx];
          }

          if (!draggedComponent) return;
          
          // 如果拖動的是區塊容器內的子元件（但不是 column-child），先從原位置移除
          if (!isChild) {
          for (let i = 0; i < this.components.length; i++) {
            const comp = this.components[i];
              if (comp.component_type === 'section') {
                // 檢查新的 column_children 結構
                if (comp.component_data.column_children && Array.isArray(comp.component_data.column_children)) {
                  for (let colIdx = 0; colIdx < comp.component_data.column_children.length; colIdx++) {
                    const columnChildren = comp.component_data.column_children[colIdx];
                    if (Array.isArray(columnChildren)) {
                      const childIndex = columnChildren.findIndex(c => c === draggedComponent);
                      if (childIndex !== -1) {
                        columnChildren.splice(childIndex, 1);
                        isChild = true;
                        break;
                      }
                    }
                  }
                }
                // 兼容舊的 children 結構
                if (!isChild && comp.component_data.children && Array.isArray(comp.component_data.children)) {
              const childIndex = comp.component_data.children.findIndex(c => c === draggedComponent);
              if (childIndex !== -1) {
                comp.component_data.children.splice(childIndex, 1);
                isChild = true;
                  }
                }
                if (isChild) break;
              }
            }
          }

          // 如果不是子元件，從主列表移除
          if (!isChild && !isColumnChild) {
            const draggedIdx = parseInt(draggedItem.dataset.index);
            if (!isNaN(draggedIdx)) {
            this.components.splice(draggedIdx, 1);
            }
          }

          // 添加到目標欄位
          // 使用 column_children 結構：每個欄位有獨立的子元件數組
          if (!section.component_data.column_children) {
            // 如果沒有 column_children，嘗試從舊的 children 結構遷移
            const columns = section.component_data.columns || 1;
            section.component_data.column_children = Array(columns).fill(0).map(() => []);
            if (section.component_data.children && Array.isArray(section.component_data.children)) {
              // 將舊的 children 分配到各欄
              const childrenPerColumn = Math.ceil(section.component_data.children.length / columns);
              section.component_data.children.forEach((child, idx) => {
                const colIdx = Math.floor(idx / childrenPerColumn);
                if (colIdx < columns) {
                  section.component_data.column_children[colIdx].push(child);
                }
              });
            }
            // 移除舊的 children
            delete section.component_data.children;
          }
          
          // 確保 column_children 數組足夠長
          const columns = section.component_data.columns || 1;
          while (section.component_data.column_children.length < columns) {
            section.component_data.column_children.push([]);
          }
          
          // 直接追加到指定欄位的末尾
          if (!section.component_data.column_children[columnIndex]) {
            section.component_data.column_children[columnIndex] = [];
          }
          section.component_data.column_children[columnIndex].push(draggedComponent);

          // 更新 sort_order
          this.components.forEach((comp, i) => {
            comp.sort_order = i;
          });

          // 保存狀態到歷史記錄
          this.saveState();

          // 重新渲染
          this.renderComponents();

          // 重新選中區塊容器
          this.selectComponent(section);
        });
      });
    }, 100);
  },

  initGoogleMapPreviews() {
    // Always re-query DOM to avoid stale references after re-render
    const getContainers = () => document.querySelectorAll('.gmap-preview-container');
    const containers = getContainers();
    if (!containers.length) return;

    const placeholder = '<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#6c757d;"><i class="bi bi-geo-alt" style="font-size:2rem;margin-right:0.5rem;"></i> Google Map</div>';

    const doInit = () => {
      // Re-query DOM so we always operate on current elements
      const els = getContainers();
      els.forEach(el => {
        // Skip if already initialised
        if (el.dataset.gmapInit === '1') return;
        el.dataset.gmapInit = '1';

        const lat = parseFloat(el.dataset.lat) || 25.033964;
        const lng = parseFloat(el.dataset.lng) || 121.564468;
        const zoom = parseInt(el.dataset.zoom) || 14;
        const markerTitle = el.dataset.marker || '';

        if (typeof google === 'undefined' || !google.maps) {
          el.innerHTML = placeholder;
          return;
        }

        // Ensure the container is attached to the document
        if (!document.body.contains(el)) {
          return;
        }

        const map = new google.maps.Map(el, {
          center: { lat, lng },
          zoom,
          mapTypeControl: false,
          streetViewControl: false,
          fullscreenControl: false,
          gestureHandling: 'cooperative'
        });
        new google.maps.Marker({ map, position: { lat, lng }, title: markerTitle });
      });
    };

    if (typeof google !== 'undefined' && google.maps) {
      // Delay to ensure DOM is fully laid out after innerHTML assignment
      setTimeout(doInit, 100);
    } else if (!document.getElementById('page-editor-gmap-script')) {
      const apiKey = document.querySelector('meta[name="google-maps-api-key"]')?.content || '';
      if (!apiKey) {
        containers.forEach(el => {
          if (el.dataset.gmapInit === '1') return;
          el.dataset.gmapInit = '1';
          el.innerHTML = placeholder;
        });
        return;
      }
      window._pageEditorGmapCallbacks = window._pageEditorGmapCallbacks || [];
      window._pageEditorGmapCallbacks.push(doInit);
      window._pageEditorGmapReady = function() {
        (window._pageEditorGmapCallbacks || []).forEach(fn => fn());
        window._pageEditorGmapCallbacks = [];
      };
      const s = document.createElement('script');
      s.id = 'page-editor-gmap-script';
      s.src = 'https://maps.googleapis.com/maps/api/js?key=' + apiKey + '&callback=_pageEditorGmapReady';
      s.async = true;
      s.defer = true;
      document.head.appendChild(s);
    } else {
      // Script tag already added but not yet loaded — queue callback
      window._pageEditorGmapCallbacks = window._pageEditorGmapCallbacks || [];
      window._pageEditorGmapCallbacks.push(doInit);
    }
  },

  renderComponentPreview(comp) {
    const type = comp.component_type;
    const data = comp.component_data || {};
    
    // 辅助函数：获取元素的保存样式
    const getElementStyle = (field, menuIndex = null) => {
      if (!data.element_styles) return '';
      
      // 对于菜单项，使用 menu_index 来查找样式
      let styleKey = field;
      if (field === 'menu_item_text' && menuIndex !== null && menuIndex !== undefined) {
        styleKey = `menu_item_text_${menuIndex}`;
      }
      
      if (data.element_styles[styleKey]) {
        return data.element_styles[styleKey];
      }
      return '';
    };
    
    switch (type) {
      case 'hero':
        const heroBgStyle = data.background_image 
          ? `background-image: url('${data.background_image}'); background-size: cover; background-position: center;`
          : '';
        const heroBgColor = this.themeColor(data.bg_color, '--theme-hero-bg', '#0d6efd');
        const heroTextColor = this.themeColor(data.text_color, '--theme-hero-text', '#ffffff');
        const heroHeadingColor = this.themeColor(data.heading_color, '--theme-hero-heading', '#ffffff');
        const heroMinHeight = data.min_height || '300px';
        const heroTextAlign = data.text_align || 'center';
        const heroTextAlignClass = heroTextAlign === 'center' ? 'text-center' : heroTextAlign === 'right' ? 'text-end' : 'text-start';
        const heroBgClass = '';
        return `
          <div class="component-label">${this.getComponentTypeLabel('hero')}</div>
          <div class="container-fluid ${heroBgClass} d-flex align-items-center" style="${heroBgStyle}${heroBgStyle ? '' : ` background-color: ${heroBgColor};`} color: ${heroTextColor}; min-height: ${heroMinHeight};">
            <div class="container ${heroTextAlignClass} py-5">
              <h1 class="display-1 mb-3 editable-text" contenteditable="true" data-field="title" style="color: ${heroHeadingColor};${getElementStyle('title') ? ' ' + getElementStyle('title') : ''}">${data.title || '歡迎標題'}</h1>
              <p class="lead mb-4 editable-text" contenteditable="true" data-field="subtitle" style="color: ${heroTextColor};${getElementStyle('subtitle') ? ' ' + getElementStyle('subtitle') : ''}">${data.subtitle || '副標題文字'}</p>
              <a href="${data.button_link || '#'}" class="btn btn-light btn-lg editable-text" contenteditable="true" data-field="button_text" style="color: ${this.themeColor('', '--theme-primary', '#007bff')};${getElementStyle('button_text') ? ' ' + getElementStyle('button_text') : ''}">${data.button_text || '了解更多'}</a>
            </div>
          </div>
        `;
      case 'banner-slider':
        const slides = Array.isArray(data.slides) ? data.slides : [];
        const sliderHeight = data.height || '360px';
        const sliderTextAlign = data.text_align || 'left';
        const sliderTextAlignClass = sliderTextAlign === 'center' ? 'text-center' : sliderTextAlign === 'right' ? 'text-end' : 'text-start';
        const sliderTextColor = this.themeColor(data.text_color, '--theme-hero-text', '#ffffff');
        const sliderOverlay = data.overlay_color || 'rgba(0, 0, 0, 0.45)';
        const sliderId = `bannerCarouselPreview-${this.components.indexOf(comp)}`;
        const sliderBgFallback = this.themeColor('', '--theme-primary', '#007bff');
        const sliderIndicators = slides.map((_, idx) => `
          <button type="button" data-bs-target="#${sliderId}" data-bs-slide-to="${idx}" class="${idx === 0 ? 'active' : ''}"></button>
        `).join('');
        return `
          <div class="component-label">${this.getComponentTypeLabel('banner-slider')}</div>
          ${slides.length === 0 ? `
            <div class="border rounded d-flex align-items-center justify-content-center" style="min-height: ${sliderHeight}; background-color: ${sliderBgFallback};">
              <div style="color: ${sliderTextColor}; opacity: 0.7;">${this.t('pages.pageEditor.labels.bannerSlider.noSlides', 'No slides yet')}</div>
            </div>
          ` : `
            <div id="${sliderId}" class="carousel slide" data-bs-ride="${data.autoplay !== false ? 'carousel' : 'false'}" data-bs-interval="${data.interval || 5000}">
              ${data.show_indicators !== false ? `<div class="carousel-indicators">${sliderIndicators}</div>` : ''}
              <div class="carousel-inner">
                ${slides.map((slide, idx) => {
                  const hasImage = !!slide.image;
                  const bgStyle = hasImage
                    ? `background-image: url('${slide.image}'); background-size: cover; background-position: center;`
                    : `background-color: ${sliderBgFallback};`;
                  return `
                    <div class="carousel-item ${idx === 0 ? 'active' : ''}">
                      <div class="d-flex align-items-center" style="min-height: ${sliderHeight}; ${bgStyle}">
                        <div class="w-100" style="background-color: ${sliderOverlay}; min-height: ${sliderHeight};">
                          <div class="container py-5 ${sliderTextAlignClass}" style="color: ${sliderTextColor};">
                            <h2 class="mb-2">${slide.title || this.t('pages.pageEditor.defaults.bannerSlider.title1', '主打標題')}</h2>
                            <p class="mb-3">${slide.subtitle || ''}</p>
                            ${slide.button_text ? `<a href="${slide.button_link || '#'}" class="btn btn-light btn-sm" style="color: ${this.themeColor('', '--theme-primary', '#007bff')};">${slide.button_text}</a>` : ''}
                          </div>
                        </div>
                      </div>
                    </div>
                  `;
                }).join('')}
              </div>
              ${data.show_arrows !== false ? `
                <button class="carousel-control-prev" type="button" data-bs-target="#${sliderId}" data-bs-slide="prev">
                  <span class="carousel-control-prev-icon" aria-hidden="true"></span>
                </button>
                <button class="carousel-control-next" type="button" data-bs-target="#${sliderId}" data-bs-slide="next">
                  <span class="carousel-control-next-icon" aria-hidden="true"></span>
                </button>
              ` : ''}
            </div>
          `}
        `;
      case 'text':
        const textColor = this.themeColor(data.text_color, '--theme-text', '#333333');
        const textFontSize = data.font_size || '1rem';
        const textLineHeight = data.line_height || '1.8';
        return `
          <div class="component-label">${this.getComponentTypeLabel('text')}</div>
          <div class="container">
            <div class="row">
              <div class="col-12">
                <p class="mb-0 editable-text" contenteditable="true" data-field="content" style="line-height: ${textLineHeight}; color: ${textColor}; font-size: ${textFontSize};${getElementStyle('content') ? ' ' + getElementStyle('content') : ''}">${data.content || '這裡是文字內容...'}</p>
              </div>
            </div>
          </div>
        `;
      case 'image':
        return `
          <div class="component-label">${this.getComponentTypeLabel('image')}</div>
          <div class="container">
            <div class="row">
              <div class="col-12 text-center">
                ${data.src ? `<div style="aspect-ratio: 18 / 9; overflow: hidden; border-radius: 0.375rem;"><img src="${data.src}" alt="${data.alt || 'Image'}" class="img-fluid" style="width: 100%; height: 100%; object-fit: cover;"></div>` : `<div class="bg-light border border-2 border-dashed rounded p-5" style="aspect-ratio: 18 / 9; display: flex; flex-direction: column; align-items: center; justify-content: center;"><i class="bi bi-image fs-1 text-muted d-block mb-3"></i><span class="text-muted">${data.alt || 'Image'}</span></div>`}
              </div>
            </div>
          </div>
        `;
      case 'button':
        const buttonSize = data.size || 'lg';
        const buttonAlign = data.align || 'center';
        const alignClass = buttonAlign === 'center' ? 'text-center' : buttonAlign === 'right' ? 'text-end' : 'text-start';
        // If custom color saved, resolve through theme; otherwise rely on Bootstrap btn-* class
        const resolvedBtnColor = data.button_color ? this.themeColor(data.button_color, '--theme-btn-primary-bg', '#0d6efd') : '';
        const resolvedBtnTextColor = data.button_text_color ? this.themeColor(data.button_text_color, '--theme-btn-primary-text', '#ffffff') : '';
        const buttonStyle = resolvedBtnColor ? `background-color: ${resolvedBtnColor} !important; border-color: ${resolvedBtnColor} !important;` : '';
        const buttonTextStyle = resolvedBtnTextColor ? `color: ${resolvedBtnTextColor} !important;` : '';
        const combinedStyle = (buttonStyle + ' ' + buttonTextStyle).trim();
        return `
          <div class="component-label">${this.getComponentTypeLabel('button')}</div>
          <div class="container">
            <div class="row">
              <div class="col-12 ${alignClass}">
                <a href="${data.link || '#'}" class="btn btn-${data.style || 'primary'} ${buttonSize ? 'btn-' + buttonSize : ''} editable-text" contenteditable="true" data-field="text"${combinedStyle || getElementStyle('text') ? ` style="${combinedStyle}${getElementStyle('text') ? (combinedStyle ? ' ' : '') + getElementStyle('text') : ''}"` : ''}>${data.text || 'Button'}</a>
              </div>
            </div>
          </div>
        `;
      case 'section':
        const columns = data.columns || 1;
        // 使用 Bootstrap 响应式列类：移动端每行一栏（col-12），桌面端根据栏数分配
        const columnClass = columns === 1 ? 'col-12' : columns === 2 ? 'col-12 col-md-6' : columns === 3 ? 'col-12 col-md-4' : 'col-12 col-md-3';
        
        // 使用新的 column_children 結構，如果沒有則從舊的 children 遷移
        let columnChildrenArrays = [];
        if (data.column_children && Array.isArray(data.column_children)) {
          // 使用新的結構
          columnChildrenArrays = data.column_children;
          // 確保數組長度匹配欄數
          while (columnChildrenArrays.length < columns) {
            columnChildrenArrays.push([]);
          }
        } else if (data.children && Array.isArray(data.children)) {
          // 從舊的 children 結構遷移
          columnChildrenArrays = Array(columns).fill(0).map(() => []);
          const childrenPerColumn = Math.ceil(data.children.length / columns);
          data.children.forEach((child, idx) => {
            const colIdx = Math.floor(idx / childrenPerColumn);
            if (colIdx < columns) {
              columnChildrenArrays[colIdx].push(child);
            }
          });
          // 更新數據結構
          data.column_children = columnChildrenArrays;
          delete data.children;
        } else {
          // 初始化空數組
          columnChildrenArrays = Array(columns).fill(0).map(() => []);
        }
        
        // 檢查是否有任何 column 有子元件
        const hasAnyChildren = columnChildrenArrays.some(col => col && col.length > 0);
        
        return `
          <div class="component-label">${this.getComponentTypeLabel('section')} (${columns} ${this.t('pages.pageEditor.units.columns', 'cols')})</div>
          <div class="container-fluid multi-col" style="background-color: ${this.themeColor(data.background_color, '--theme-bg', '#ffffff')};">
            <div class="row">
              ${Array(columns).fill(0).map((_, i) => {
                const columnChildren = columnChildrenArrays[i] || [];
                // 如果有任何 column 有子元件，則所有 dropzone 都不顯示 min-height
                const dropzoneStyle = hasAnyChildren 
                  ? 'display: flex; align-items: center; justify-content: center;'
                  : 'min-height: 200px; display: flex; align-items: center; justify-content: center;';
                return `
                  <div class="${columnClass}">
                    <div class="section-column-content" data-section-index="${this.components.indexOf(comp)}" data-column-index="${i}" style="position: relative; padding-bottom: 35px;">
                      ${columnChildren.length === 0 ? 
                        `<div class="text-center text-muted small section-column-dropzone border border-2 border-dashed rounded p-3 bg-light" style="${dropzoneStyle}">${this.t('pages.pageEditor.dropComponentHere', 'Drag component here')}</div>` :
                        columnChildren.map((child, childIdx) => {
                          // 遞歸渲染子元件預覽（簡化版）
                          const childType = child.component_type;
                          const childData = child.component_data || {};
                          let childPreview = '';
                          switch(childType) {
                            case 'text':
                              childPreview = `<div class="p-2 border rounded mb-2 bg-white">${childData.content || '文字內容...'}</div>`;
                              break;
                            case 'heading':
                              childPreview = `<${childData.level || 'h3'} class="p-2 border rounded mb-2 bg-white">${childData.text || 'Heading'}</${childData.level || 'h3'}>`;
                              break;
                            case 'button':
                              childPreview = `<div class="p-2 border rounded mb-2 bg-white text-center"><a href="${childData.link || '#'}" class="btn btn-sm btn-${childData.style || 'primary'}">${childData.text || 'Button'}</a></div>`;
                              break;
                            case 'image':
                            const childImageSrc = childData.src || '';
                            childPreview = childImageSrc 
                              ? `<div style="aspect-ratio: 18 / 9; overflow: hidden; border-radius: 0.375rem;"><img src="${childImageSrc}" alt="${childData.alt || 'Image'}" class="img-fluid" style="width: 100%; height: 100%; object-fit: cover;"></div>`
                              : `<div class="bg-light border border-2 border-dashed rounded p-3" style="aspect-ratio: 18 / 9; display: flex; flex-direction: column; align-items: center; justify-content: center;"><i class="bi bi-image fs-2 text-muted"></i></div>`;
                              break;
                            default:
                              childPreview = `<div class="p-2 border rounded mb-2 bg-white">${this.getComponentTypeLabel(childType)}</div>`;
                          }
          // 添加拖拽属性和功能按钮
                          return `
                            <div class="column-child-item" 
                                 draggable="true" 
                                 data-section-index="${this.components.indexOf(comp)}" 
                                 data-column-index="${i}" 
                                 data-child-index="${childIdx}"
                                 style="position: relative;">
                              <div class="column-child-actions">
                                <button class="btn btn-xs" onclick="PageEditor.moveColumnChildUp(${this.components.indexOf(comp)}, ${i}, ${childIdx})" ${childIdx === 0 ? 'disabled' : ''} title="${this.t('pages.pageEditor.actions.moveUp', 'Move up')}">
                                  <i class="bi bi-arrow-up"></i>
                                </button>
                                <button class="btn btn-xs" onclick="PageEditor.moveColumnChildDown(${this.components.indexOf(comp)}, ${i}, ${childIdx})" ${childIdx === columnChildren.length - 1 ? 'disabled' : ''} title="${this.t('pages.pageEditor.actions.moveDown', 'Move down')}">
                                  <i class="bi bi-arrow-down"></i>
                                </button>
                                <button class="btn btn-xs" onclick="PageEditor.editColumnChild(${this.components.indexOf(comp)}, ${i}, ${childIdx})" title="${this.t('pages.pageEditor.actions.edit', 'Edit')}">
                                  <i class="bi bi-pencil"></i>
                                </button>
                                <button class="btn btn-xs btn-outline-danger" onclick="PageEditor.removeChildFromSectionColumn(${this.components.indexOf(comp)}, ${i}, ${childIdx})" title="${this.t('pages.pageEditor.actions.delete', 'Delete')}">
                                  <i class="bi bi-trash"></i>
                                </button>
                              </div>
                              ${childPreview}
                            </div>
                          `;
                        }).join('')
                      }
                      <button class="btn btn-sm btn-primary section-column-add-btn" data-section-index="${this.components.indexOf(comp)}" data-column-index="${i}" title="${this.t('pages.pageEditor.actions.addToColumn', 'Add to column')}">
                        <i class="bi bi-plus-lg"></i> ${this.t('pages.pageEditor.addComponent', 'Component')}
                      </button>
                    </div>
                  </div>
                `;
              }).join('')}
            </div>
          </div>
        `;
      case 'heading':
        const headingTag = data.level || 'h2';
        const headingTextColor = this.themeColor(data.text_color, '--theme-heading-color', '#333333');
        const headingTextAlign = data.text_align || 'left';
        const headingMargin = data.margin || '1.5rem 0';
        const headingTextAlignClass = headingTextAlign === 'center' ? 'text-center' : headingTextAlign === 'right' ? 'text-end' : 'text-start';
        // 使用 Bootstrap 的 display 类来增强标题样式
        // NOTE: h3 不使用 display-3（避免「title 元件」被過度放大）
        const displayClass = headingTag === 'h1' ? 'display-1' : headingTag === 'h2' ? 'display-2' : headingTag === 'h3' ? '' : headingTag === 'h4' ? 'display-4' : headingTag === 'h5' ? 'display-5' : headingTag === 'h6' ? 'display-6' : '';
        return `
          <div class="component-label">${this.getComponentTypeLabel('heading')} (${headingTag.toUpperCase()})</div>
          <div class="container">
            <div class="row">
              <div class="col-12">
                <${headingTag} class="${displayClass} ${headingTextAlignClass} editable-text" contenteditable="true" data-field="text" style="color: ${headingTextColor}; margin: ${headingMargin};${getElementStyle('text') ? ' ' + getElementStyle('text') : ''}">${data.text || '標題文字'}</${headingTag}>
              </div>
            </div>
          </div>
        `;
      case 'header':
        const headerBgColor = this.themeColor(data.bg_color, '--theme-nav-bg', '#ffffff');
        const headerTextColor = this.themeColor(data.menu_text_color, '--theme-nav-text', '#333333');
        const headerBorderColor = this.themeColor('', '--theme-nav-border', '#dee2e6');
        const logoText = data.logo_text || this.enterpriseName || 'Logo';
        return `
          <div class="component-label">${this.getComponentTypeLabel('header')}</div>
          <header style="background-color: ${headerBgColor}; color: ${headerTextColor}; border-bottom: 1px solid ${headerBorderColor}; padding: 1rem 0;">
            <div class="container-fluid">
              <div class="d-flex justify-content-between align-items-center">
                <div class="d-flex align-items-center">
                  ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 40px; max-width: 150px; margin-right: 1rem;">` : ''}
                  <a class="navbar-brand editable-text" contenteditable="true" data-field="logo_text" href="#" style="color: ${headerTextColor}; font-weight: bold; font-size: 1.25rem; text-decoration: none;${getElementStyle('logo_text') ? ' ' + getElementStyle('logo_text') : ''}">
                    ${logoText}
                  </a>
                </div>
                <div class="d-flex align-items-center gap-3">
                  ${data.show_login_icon !== false ? `<a href="${data.login_icon_link || '/co/' + (document.body.dataset.tenantSubdomain || 'test') + '/user/'}" style="color: ${headerTextColor}; font-size: 1.5rem; text-decoration: none;" title="${data.login_icon_title || this.t('publicSite.nav.login', 'Login')}"><i class="bi ${data.login_icon_class || 'bi-person-circle'}"></i></a>` : ''}
                  ${data.show_cart_icon !== false ? `<a href="${data.cart_icon_link || '/co/' + (document.body.dataset.tenantSubdomain || 'test') + '/cart/'}" style="color: ${headerTextColor}; font-size: 1.5rem; text-decoration: none;" title="${data.cart_icon_title || this.t('publicSite.nav.cart', 'Cart')}"><i class="bi ${data.cart_icon_class || 'bi-cart'}"></i></a>` : ''}
                </div>
              </div>
            </div>
            <div class="container-fluid" style="border-top: 1px solid ${headerBorderColor}; margin-top: 0.75rem; padding-top: 0.75rem;">
              <nav class="navbar navbar-expand-lg p-0">
                <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#headerNav${this.components.indexOf(comp)}" aria-controls="headerNav${this.components.indexOf(comp)}" aria-expanded="false" aria-label="Toggle navigation">
                  <span class="navbar-toggler-icon"></span>
                </button>
                <div class="collapse navbar-collapse" id="headerNav${this.components.indexOf(comp)}">
                  <ul class="navbar-nav">
                    ${(data.menu_items || []).map((item, index) => {
                      const menuItemStyle = getElementStyle('menu_item_text', index);
                      return `
                      <li class="nav-item">
                        <a class="nav-link editable-text" contenteditable="true" data-field="menu_item_text" data-menu-index="${index}" href="${item.link || '#'}" style="color: ${headerTextColor}; text-decoration: none;${menuItemStyle ? ' ' + menuItemStyle : ''}">${item.text || 'Link'}</a>
                      </li>
                    `;
                    }).join('')}
              </ul>
                </div>
            </nav>
            </div>
          </header>
        `;
      case 'nav':
        const navBgColor = this.themeColor(data.bg_color, '--theme-nav-bg', '#ffffff');
        const menuTextColor = this.themeColor(data.menu_text_color, '--theme-nav-text', '#333333');
        const menuHoverColor = this.themeColor(data.menu_hover_color, '--theme-nav-hover', '#0d6efd');
        const navBorderColor = this.themeColor('', '--theme-nav-border', '#dee2e6');
        const isFixed = data.fixed === true;
        const fixedClass = isFixed ? 'fixed-top' : '';
        const menuPosition = data.menu_position || 'right';
        // Determine menu alignment class based on menu_position
        let menuAlignClass = 'me-auto'; // right: logo left, menu left, icons right
        let menuFlexClass = '';
        if (menuPosition === 'center') {
          menuAlignClass = 'mx-auto';
          menuFlexClass = 'justify-content-center';
        } else if (menuPosition === 'left') {
          menuAlignClass = 'me-auto';
        } else if (menuPosition === 'bottom') {
          menuAlignClass = 'me-auto';
        }
        // Build topbar HTML
        const topbarHtml = data.show_topbar ? `
          <div class="public-nav-topbar" style="background: ${this.themeColor(data.topbar_bg_color, '--theme-surface', '#f1f1f1')}; color: ${this.themeColor(data.topbar_text_color, '--theme-primary', '#f97316')}; text-align: center; padding: 6px 0; font-size: 13px; font-weight: 700;">
            ${data.topbar_text || ''}
          </div>
        ` : '';
        // Build icons HTML
        const navIconsHtml = `
           <div class="d-flex align-items-center gap-3" style="margin-left: 15px;">
            ${data.show_login_icon ? `<a href="${data.login_icon_link || '/co/' + (document.body.dataset.tenantSubdomain || 'test') + '/user/'}" style="color: ${menuTextColor}; font-size: 1.25rem; text-decoration: none;" title="${data.login_icon_title || this.t('publicSite.nav.login', 'Login')}"><i class="bi ${data.login_icon_class || 'bi-person-circle'}"></i></a>` : ''}
            ${data.show_cart_icon ? `<a href="${data.cart_icon_link || '/co/' + (document.body.dataset.tenantSubdomain || 'test') + '/cart/'}" style="color: ${menuTextColor}; font-size: 1.25rem; text-decoration: none; position: relative;" title="${data.cart_icon_title || this.t('publicSite.nav.cart', 'Cart')}"><i class="bi ${data.cart_icon_class || 'bi-cart'}"></i></a>` : ''}
          </div>
        `;
        // Build menu items HTML
        const navMenuItemsHtml = (data.menu_items || []).map((item, index) => {
          const hoverColor = menuHoverColor;
          const textColor = menuTextColor;
          const menuItemStyle = getElementStyle('menu_item_text', index);
          return `
            <li class="nav-item">
              <a class="nav-link editable-text" contenteditable="true" data-field="menu_item_text" data-menu-index="${index}" href="${item.link || '#'}" style="color: ${textColor}; font-weight: 500; transition: color 0.2s;${menuItemStyle ? ' ' + menuItemStyle : ''}" onmouseover="this.style.color='${hoverColor}'" onmouseout="this.style.color='${textColor}'">${item.text || 'Link'}</a>
            </li>
          `;
        }).join('');

        if (menuPosition === 'bottom') {
          // Bottom layout: logo + icons on top row, menu items on second row
          // Uses navbar-collapse so mobile preview CSS can properly collapse it
          return `
            <div class="component-label">${this.getComponentTypeLabel('nav')}</div>
            ${topbarHtml}
            <nav class="navbar navbar-expand-lg ${fixedClass}" style="background-color: ${navBgColor}; border-bottom: 1px solid ${navBorderColor};${isFixed ? ' z-index: 1030;' : ''} flex-wrap: wrap;">
              <div class="container-fluid" style="flex-wrap: wrap;">
                <div class="d-flex w-100 align-items-center justify-content-between" style="padding: 0.5rem 0;">
                  <a class="navbar-brand editable-text mb-0" contenteditable="true" data-field="logo_text" href="#" style="color: ${menuTextColor}; font-weight: bold; font-size: 1.25rem;${getElementStyle('logo_text') ? ' ' + getElementStyle('logo_text') : ''}">
                    ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 40px; max-width: 150px;">` : (data.logo_text || 'Logo')}
                  </a>
                  <div class="d-flex align-items-center">
                    ${navIconsHtml}
                    <button class="navbar-toggler ms-2" type="button" data-bs-toggle="collapse" data-bs-target="#navbarNav${this.components.indexOf(comp)}" aria-expanded="false" aria-label="Toggle navigation">
                      <span class="navbar-toggler-icon"></span>
                    </button>
                  </div>
                </div>
                <div class="collapse navbar-collapse nav-bottom-menu w-100" id="navbarNav${this.components.indexOf(comp)}" style="border-top: 1px solid ${navBorderColor};">
                  <ul class="navbar-nav flex-row flex-wrap justify-content-center gap-1" style="padding: 0.4rem 0;">
                    ${navMenuItemsHtml}
                  </ul>
                </div>
              </div>
            </nav>
          `;
        } else if (menuPosition === 'center') {
          // Center layout: logo left, menu center (absolute), icons right
          return `
            <div class="component-label">${this.getComponentTypeLabel('nav')}</div>
            ${topbarHtml}
            <nav class="navbar navbar-expand-lg ${fixedClass}" style="background-color: ${navBgColor}; border-bottom: 1px solid ${navBorderColor};${isFixed ? ' z-index: 1030;' : ''}">
              <div class="container-fluid position-relative">
                <a class="navbar-brand editable-text" contenteditable="true" data-field="logo_text" href="#" style="color: ${menuTextColor}; font-weight: bold; font-size: 1.25rem;${getElementStyle('logo_text') ? ' ' + getElementStyle('logo_text') : ''}">
                  ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 40px; max-width: 150px;">` : (data.logo_text || 'Logo')}
                </a>
                <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarNav${this.components.indexOf(comp)}" aria-expanded="false" aria-label="Toggle navigation">
                  <span class="navbar-toggler-icon"></span>
                </button>
                <div class="collapse navbar-collapse" id="navbarNav${this.components.indexOf(comp)}">
                  <ul class="navbar-nav ${menuAlignClass} mb-2 mb-lg-0 ${menuFlexClass}">
                    ${navMenuItemsHtml}
                  </ul>
                  ${navIconsHtml}
                </div>
              </div>
            </nav>
          `;
        } else if (menuPosition === 'left') {
          // Left layout: logo left, menu immediately after logo, icons far right
          return `
            <div class="component-label">${this.getComponentTypeLabel('nav')}</div>
            ${topbarHtml}
            <nav class="navbar navbar-expand-lg ${fixedClass}" style="background-color: ${navBgColor}; border-bottom: 1px solid ${navBorderColor};${isFixed ? ' z-index: 1030;' : ''}">
              <div class="container-fluid">
                <a class="navbar-brand editable-text" contenteditable="true" data-field="logo_text" href="#" style="color: ${menuTextColor}; font-weight: bold; font-size: 1.25rem;${getElementStyle('logo_text') ? ' ' + getElementStyle('logo_text') : ''}">
                  ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 40px; max-width: 150px;">` : (data.logo_text || 'Logo')}
                </a>
                <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarNav${this.components.indexOf(comp)}" aria-expanded="false" aria-label="Toggle navigation">
                  <span class="navbar-toggler-icon"></span>
                </button>
                <div class="collapse navbar-collapse" id="navbarNav${this.components.indexOf(comp)}">
                  <ul class="navbar-nav me-auto mb-2 mb-lg-0">
                    ${navMenuItemsHtml}
                  </ul>
                  ${navIconsHtml}
                </div>
              </div>
            </nav>
          `;
        } else {
          // Right (default): logo left, menu pushed right with icons
          return `
            <div class="component-label">${this.getComponentTypeLabel('nav')}</div>
            ${topbarHtml}
            <nav class="navbar navbar-expand-lg ${fixedClass}" style="background-color: ${navBgColor}; border-bottom: 1px solid ${navBorderColor};${isFixed ? ' z-index: 1030;' : ''}">
              <div class="container-fluid">
                <a class="navbar-brand editable-text" contenteditable="true" data-field="logo_text" href="#" style="color: ${menuTextColor}; font-weight: bold; font-size: 1.25rem;${getElementStyle('logo_text') ? ' ' + getElementStyle('logo_text') : ''}">
                  ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 40px; max-width: 150px;">` : (data.logo_text || 'Logo')}
                </a>
                <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarNav${this.components.indexOf(comp)}" aria-expanded="false" aria-label="Toggle navigation">
                  <span class="navbar-toggler-icon"></span>
                </button>
                <div class="collapse navbar-collapse" id="navbarNav${this.components.indexOf(comp)}">
                  <ul class="navbar-nav ms-auto mb-2 mb-lg-0">
                    ${navMenuItemsHtml}
                  </ul>
                  ${navIconsHtml}
                </div>
              </div>
            </nav>
          `;
        }
      case 'footer':
        const footerBgColor = this.themeColor(data.bg_color, '--theme-footer-bg', '#f8f9fa');
        const footerTextColor = this.themeColor(data.text_color, '--theme-footer-text', '#6c757d');
        const footerBorderColor = this.themeColor('', '--theme-footer-border', 'rgba(0,0,0,0.1)');
        const footerPadding = data.padding || '2rem 0';
        // Footer 固定為 4 欄：第一欄是圖片+文字，之後三欄是垂直選單
        return `
          <div class="component-label">${this.getComponentTypeLabel('footer')}</div>
          <footer style="background-color: ${footerBgColor}; color: ${footerTextColor}; padding: ${footerPadding};">
            <div class="container">
              <div class="row">
                <div class="col-md-3 mb-3 mb-md-0">
                  ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 60px; margin-bottom: 1rem;">` : ''}
                  <div class="editable-text" contenteditable="true" data-field="column1_content" style="white-space: pre-line;">${data.column1_content || 'Company intro text'}</div>
                </div>
                <div class="col-md-3 mb-3 mb-md-0">
                  <ul class="list-unstyled">
                    ${(data.column2_menu_items || []).map(item => `
                      <li class="mb-2">
                        <a href="${item.link || '#'}" class="text-decoration-none" style="color: ${footerTextColor};">${item.text || 'Link'}</a>
                      </li>
                    `).join('')}
                  </ul>
                </div>
                <div class="col-md-3 mb-3 mb-md-0">
                  <ul class="list-unstyled">
                    ${(data.column3_menu_items || []).map(item => `
                      <li class="mb-2">
                        <a href="${item.link || '#'}" class="text-decoration-none" style="color: ${footerTextColor};">${item.text || 'Link'}</a>
                      </li>
                    `).join('')}
                  </ul>
                </div>
                <div class="col-md-3 mb-3 mb-md-0">
                  <ul class="list-unstyled">
                    ${(data.column4_menu_items || []).map(item => `
                      <li class="mb-2">
                        <a href="${item.link || '#'}" class="text-decoration-none" style="color: ${footerTextColor};">${item.text || 'Link'}</a>
                      </li>
                    `).join('')}
                  </ul>
                </div>
              </div>
              <div class="row mt-4 pt-3" style="border-top: 1px solid ${footerBorderColor};">
                <div class="col-12 text-center">
                  <p class="mb-0 editable-text" contenteditable="true" data-field="copyright" style="font-size: 0.875rem;">${data.copyright || '© 2025 All rights reserved'}</p>
                </div>
              </div>
            </div>
          </footer>
        `;
      case 'list':
        return `
          <div class="component-label">${this.getComponentTypeLabel('list')}</div>
          <div class="container py-4">
            <ul class="list-unstyled">
              ${(data.menu_items || []).map(item => `
                <li class="mb-2">
                  <a href="${item.link || '#'}" class="text-decoration-none">${item.text || '選單項目'}</a>
                </li>
              `).join('')}
              ${(data.menu_items || []).length === 0 ? `<li class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</li>` : ''}
            </ul>
          </div>
        `;
      case 'order-list':
        return `
          <div class="component-label">${this.getComponentTypeLabel('order-list')}</div>
          <div class="container py-4">
              <div class="table-responsive">
                <table class="table table-hover">
                  <thead>
                    <tr>
                      <th>${this.t('pages.pageEditor.preview.orderList.orderNumber', 'Order No.')}</th>
                      <th>${this.t('pages.pageEditor.preview.orderList.date', 'Date')}</th>
                      <th>${this.t('pages.pageEditor.preview.orderList.status', 'Status')}</th>
                      <th>${this.t('pages.pageEditor.preview.orderList.total', 'Total')}</th>
                      <th>${this.t('pages.pageEditor.preview.orderList.actions', 'Actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>#ORD-001</td>
                      <td>2025-01-01</td>
                      <td><span class="badge bg-success">${this.t('common.status.completed', 'Completed')}</span></td>
                      <td>$120.00</td>
                      <td><button class="btn btn-sm btn-outline-primary">${this.t('common.actions.view', 'View')}</button></td>
                    </tr>
                    <tr>
                      <td>#ORD-002</td>
                      <td>2025-01-05</td>
                      <td><span class="badge bg-warning text-dark">${this.t('common.status.processing', 'Processing')}</span></td>
                      <td>$85.50</td>
                      <td><button class="btn btn-sm btn-outline-primary">${this.t('common.actions.view', 'View')}</button></td>
                    </tr>
                  </tbody>
                </table>
              </div>
          </div>
        `;
      case 'blog-list':
        const blogLimit = data.limit || 10;
        const blogColumns = data.columns || 1;
        const blogShowExcerpt = data.show_excerpt !== false;
        const blogShowImage = data.show_image !== false;
        return `
          <div class="component-label">${this.getComponentTypeLabel('blog-list')}</div>
          <div class="container py-4">
            <div class="row">
              ${Array.from({ length: blogLimit }, (_, i) => `
                <div class="col-md-${12 / blogColumns} mb-4">
                  <div class="card h-100">
                    ${blogShowImage ? `
                      <div class="card-img-top bg-light d-flex align-items-center justify-content-center" style="height: 200px;">
                        <i class="bi bi-image fs-1 text-muted"></i>
                      </div>
                    ` : ''}
                    <div class="card-body">
                      <h5 class="card-title">${this.t('pages.pageEditor.preview.blog.titlePrefix', 'Blog post')} ${i + 1}</h5>
                      ${blogShowExcerpt ? `<p class="card-text text-muted">${this.t('pages.pageEditor.preview.blog.excerpt', 'This is the excerpt...')}</p>` : ''}
                      <div class="d-flex justify-content-between align-items-center">
                        <small class="text-muted">2025-01-01</small>
                        <a href="#" class="btn btn-sm btn-outline-primary">${this.t('pages.pageEditor.preview.blog.readMore', 'Read more')}</a>
                      </div>
                    </div>
                  </div>
                </div>
              `).join('')}
            </div>
          </div>
        `;
      case 'login-register':
        return `
          <div class="component-label">${this.getComponentTypeLabel('login-register')}</div>
          <div class="container py-4">
            <div class="row justify-content-center">
              <div class="col-md-6">
                <ul class="nav nav-tabs mb-3" role="tablist">
                  <li class="nav-item">
                    <button class="nav-link active" data-bs-toggle="tab" data-bs-target="#login-tab">${this.t('pages.pageEditor.preview.auth.login', 'Login')}</button>
                  </li>
                  <li class="nav-item">
                    <button class="nav-link" data-bs-toggle="tab" data-bs-target="#register-tab">${this.t('pages.pageEditor.preview.auth.register', 'Register')}</button>
                  </li>
                </ul>
                <div class="tab-content">
                  <div class="tab-pane fade show active" id="login-tab">
                    <form>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.auth.phoneOrEmail', 'Phone or email')}</label>
                        <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.auth.phoneOrEmailPlaceholder', 'Phone or email@example.com')}">
                      </div>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.auth.password', 'Password')}</label>
                        <input type="password" class="form-control" placeholder="${this.t('pages.pageEditor.preview.auth.password', 'Password')}">
                      </div>
                      <button type="submit" class="btn btn-primary w-100">${this.t('pages.pageEditor.preview.auth.login', 'Login')}</button>
                    </form>
                  </div>
                  <div class="tab-pane fade" id="register-tab">
                    <form>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.auth.name', 'Name')}</label>
                        <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.auth.name', 'Name')}">
                      </div>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.auth.phone', 'Phone')}</label>
                        <input type="tel" class="form-control" placeholder="${this.t('pages.pageEditor.preview.auth.phone', 'Phone')}">
                      </div>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.auth.email', 'Email')}</label>
                        <input type="email" class="form-control" placeholder="email@example.com">
                      </div>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.auth.password', 'Password')}</label>
                        <input type="password" class="form-control" placeholder="${this.t('pages.pageEditor.preview.auth.password', 'Password')}">
                      </div>
                      <button type="submit" class="btn btn-primary w-100">${this.t('pages.pageEditor.preview.auth.register', 'Register')}</button>
                    </form>
                  </div>
                </div>
              </div>
            </div>
          </div>
        `;
      case 'contact-form':
        const showName = data.show_name !== false;
        const showEmail = data.show_email !== false;
        const showPhone = data.show_phone !== false;
        const showMessage = data.show_message !== false;
        const submitText = data.submit_button_text || this.t('pages.pageEditor.preview.contact.submit', 'Submit');
        return `
          <div class="component-label">${this.getComponentTypeLabel('contact-form')}</div>
          <div class="container py-4">
            <div class="row justify-content-center">
              <div class="col-md-8">
                <form id="contact-form-preview">
                  ${showName ? `
                    <div class="mb-3">
                      <label class="form-label">${this.t('pages.pageEditor.preview.contact.name', 'Name')} <span class="text-danger">*</span></label>
                      <input type="text" class="form-control" name="name" required>
                    </div>
                  ` : ''}
                  ${showEmail ? `
                    <div class="mb-3">
                      <label class="form-label">${this.t('pages.pageEditor.preview.contact.email', 'Email')} <span class="text-danger">*</span></label>
                      <input type="email" class="form-control" name="email" required>
                    </div>
                  ` : ''}
                  ${showPhone ? `
                    <div class="mb-3">
                      <label class="form-label">${this.t('pages.pageEditor.preview.contact.phone', 'Phone')}</label>
                      <input type="tel" class="form-control" name="phone">
                    </div>
                  ` : ''}
                  ${showMessage ? `
                    <div class="mb-3">
                      <label class="form-label">${this.t('pages.pageEditor.preview.contact.message', 'Message')} <span class="text-danger">*</span></label>
                      <textarea class="form-control" name="message" rows="5" required></textarea>
                    </div>
                  ` : ''}
                  <button type="submit" class="btn btn-primary">${submitText}</button>
                </form>
              </div>
            </div>
          </div>
        `;
      case 'cart':
        return `
          <div class="component-label">${this.getComponentTypeLabel('cart')}</div>
          <div class="container py-4">
            <div class="row">
              <div class="col-md-8">
                <div class="card">
                  <div class="card-body">
                    <h5 class="card-title">${this.t('pages.pageEditor.preview.cart.title', 'Cart')}</h5>
                    <div class="table-responsive">
                      <table class="table">
                        <thead>
                          <tr>
                            <th>${this.t('pages.pageEditor.preview.cart.columns.product', 'Product')}</th>
                            <th>${this.t('pages.pageEditor.preview.cart.columns.quantity', 'Qty')}</th>
                            <th>${this.t('pages.pageEditor.preview.cart.columns.price', 'Price')}</th>
                            <th>${this.t('pages.pageEditor.preview.cart.columns.subtotal', 'Subtotal')}</th>
                            <th></th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr>
                            <td>產品名稱</td>
                            <td><input type="number" class="form-control form-control-sm" value="1" min="1" style="width: 80px;"></td>
                            <td>$50.00</td>
                            <td>$50.00</td>
                            <td><button class="btn btn-sm btn-outline-danger"><i class="bi bi-trash"></i></button></td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              </div>
              <div class="col-md-4">
                <div class="card">
                  <div class="card-body">
                    <h5 class="card-title">${this.t('pages.pageEditor.preview.cart.summary', 'Order Summary')}</h5>
                    <div class="d-flex justify-content-between mb-2">
                      <span>${this.t('pages.pageEditor.preview.cart.subtotal', 'Subtotal')}</span>
                      <span id="cart-subtotal">$50.00</span>
                    </div>
                    <div class="mb-3 mt-3">
                      <button class="btn btn-sm btn-outline-secondary w-100 d-flex justify-content-between align-items-center" type="button" data-bs-toggle="collapse" data-bs-target="#cartDiscountCollapse" aria-expanded="false">
                        <span><i class="bi bi-tag"></i> ${this.t('pages.pageEditor.preview.cart.discounts', 'Discounts')}</span>
                        <i class="bi bi-chevron-down"></i>
                      </button>
                      <div class="collapse mt-2" id="cartDiscountCollapse">
                        <div class="mb-3">
                          <label class="form-label small fw-semibold">${this.t('pages.pageEditor.preview.cart.couponCode', 'Coupon code')}</label>
                          <div class="input-group input-group-sm">
                            <input type="text" class="form-control" id="cart-coupon-code" placeholder="${this.t('pages.pageEditor.preview.cart.couponCodePlaceholder', 'Enter coupon code')}">
                            <button type="button" class="btn btn-outline-primary">${this.t('pages.pageEditor.preview.cart.apply', 'Apply')}</button>
                          </div>
                          <small class="text-muted" id="cart-coupon-message"></small>
                        </div>
                        <div class="mb-3">
                          <label class="form-label small fw-semibold">${this.t('pages.pageEditor.preview.cart.usePoints', 'Use points')}</label>
                          <div class="input-group input-group-sm">
                            <input type="number" class="form-control" id="cart-points-used" min="0" value="0" disabled>
                            <span class="input-group-text">${this.t('pages.pageEditor.preview.cart.points', 'Points')}</span>
                          </div>
                          <small class="text-muted">${this.t('pages.pageEditor.preview.cart.availablePoints', 'Available points')}: <span id="cart-available-points" class="fw-bold">0</span></small>
                        </div>
                      </div>
                    </div>
                    <div class="d-flex justify-content-between mb-2">
                      <span>${this.t('pages.pageEditor.preview.cart.discount', 'Discount')}</span>
                      <span id="cart-discount" style="color: var(--theme-success, #10b981);">-$0.00</span>
                    </div>
                    <div class="d-flex justify-content-between mb-2">
                      <span>${this.t('pages.pageEditor.preview.cart.shipping', 'Shipping')}</span>
                      <span id="cart-shipping">$10.00</span>
                    </div>
                    <hr>
                    <div class="d-flex justify-content-between fw-bold">
                      <span>${this.t('pages.pageEditor.preview.cart.total', 'Total')}</span>
                      <span id="cart-total">$60.00</span>
                    </div>
                    <button class="btn btn-primary w-100 mt-3">${this.t('pages.pageEditor.preview.cart.goCheckout', 'Go to checkout')}</button>
                    <button class="btn btn-outline-secondary w-100 mt-2">${this.t('pages.pageEditor.preview.cart.continueShopping', 'Continue shopping')}</button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        `;
      case 'checkout':
        return `
          <div class="component-label">${this.getComponentTypeLabel('checkout')}</div>
          <div class="container py-4">
            <div class="row">
              <div class="col-md-8">
                <h5>${this.t('pages.pageEditor.preview.checkout.shippingInfo', 'Shipping Information')}</h5>
                <form class="mb-4">
                  <div class="row mb-3">
                    <div class="col-md-6">
                      <label class="form-label">${this.t('pages.pageEditor.preview.checkout.name', 'Name')}</label>
                      <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.checkout.name', 'Name')}">
                    </div>
                    <div class="col-md-6">
                      <label class="form-label">${this.t('pages.pageEditor.preview.checkout.phone', 'Phone')}</label>
                      <input type="tel" class="form-control" placeholder="${this.t('pages.pageEditor.preview.checkout.phone', 'Phone')}">
                    </div>
                  </div>
                  <div class="mb-3">
                    <label class="form-label">${this.t('pages.pageEditor.preview.checkout.address', 'Address')}</label>
                    <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.checkout.address', 'Address')}">
                  </div>
                  <div class="row mb-3">
                    <div class="col-md-6">
                      <label class="form-label">${this.t('pages.pageEditor.preview.checkout.city', 'City')}</label>
                      <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.checkout.city', 'City')}">
                    </div>
                    <div class="col-md-6">
                      <label class="form-label">${this.t('pages.pageEditor.preview.checkout.postalCode', 'Postal code')}</label>
                      <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.checkout.postalCode', 'Postal code')}">
                    </div>
                  </div>
                </form>
                <h5>${this.t('pages.pageEditor.preview.checkout.paymentMethod', 'Payment Method')}</h5>
                <form>
                  <div class="mb-3">
                    <div class="form-check">
                      <input class="form-check-input" type="radio" name="payment" id="credit-card" checked>
                      <label class="form-check-label" for="credit-card">${this.t('pages.pageEditor.preview.checkout.creditCard', 'Credit card')}</label>
                    </div>
                    <div class="form-check">
                      <input class="form-check-input" type="radio" name="payment" id="paypal">
                      <label class="form-check-label" for="paypal">PayPal</label>
                    </div>
                    <small class="text-muted d-block mt-2">${this.t('pages.pageEditor.preview.checkout.paymentHint', '(Payment options will be loaded from your settings)')}</small>
                  </div>
                </form>
              </div>
              <div class="col-md-4">
                <div class="card">
                  <div class="card-body">
                    <h5 class="card-title">${this.t('pages.pageEditor.preview.checkout.summary', 'Order Summary')}</h5>
                    <div class="d-flex justify-content-between mb-2">
                      <span>${this.t('pages.pageEditor.preview.checkout.subtotal', 'Subtotal')}</span>
                      <span>$50.00</span>
                    </div>
                    <div class="d-flex justify-content-between mb-2">
                      <span>${this.t('pages.pageEditor.preview.checkout.shipping', 'Shipping')}</span>
                      <span>$10.00</span>
                    </div>
                    <hr>
                    <div class="d-flex justify-content-between fw-bold">
                      <span>${this.t('pages.pageEditor.preview.checkout.total', 'Total')}</span>
                      <span>$60.00</span>
                    </div>
                    <button class="btn btn-primary w-100 mt-3">${this.t('pages.pageEditor.preview.checkout.confirmOrder', 'Confirm order')}</button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        `;
      case 'user-area':
        return `
          <div class="component-label">${this.getComponentTypeLabel('user-area')}</div>
          <div class="container py-4">
            <div class="row">
              <div class="col-md-3">
                <div class="list-group">
                  <a href="#" class="list-group-item list-group-item-action active">${this.t('pages.pageEditor.preview.userArea.profile', 'Profile')}</a>
                  <a href="#" class="list-group-item list-group-item-action">${this.t('pages.pageEditor.preview.userArea.orders', 'My Orders')}</a>
                  <a href="#" class="list-group-item list-group-item-action">${this.t('pages.pageEditor.preview.userArea.addresses', 'Addresses')}</a>
                  <a href="#" class="list-group-item list-group-item-action">${this.t('pages.pageEditor.preview.userArea.logout', 'Logout')}</a>
                </div>
              </div>
              <div class="col-md-9">
                <div class="card">
                  <div class="card-body">
                    <h5 class="card-title">${this.t('pages.pageEditor.preview.userArea.profile', 'Profile')}</h5>
                    <form>
                      <div class="row mb-3">
                        <div class="col-md-6">
                          <label class="form-label">${this.t('pages.pageEditor.preview.userArea.name', 'Name')}</label>
                          <input type="text" class="form-control" value="張三">
                        </div>
                        <div class="col-md-6">
                          <label class="form-label">${this.t('pages.pageEditor.preview.userArea.phone', 'Phone')}</label>
                          <input type="tel" class="form-control" value="1234-5678">
                        </div>
                      </div>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.userArea.email', 'Email')}</label>
                        <input type="email" class="form-control" value="user@example.com">
                      </div>
                      <div class="mb-3">
                        <label class="form-label">${this.t('pages.pageEditor.preview.userArea.address', 'Address')}</label>
                        <textarea class="form-control" rows="3">地址內容</textarea>
                      </div>
                      <button type="submit" class="btn btn-primary">${this.t('pages.pageEditor.preview.userArea.save', 'Save')}</button>
                    </form>
                  </div>
                </div>
              </div>
            </div>
          </div>
        `;
      case 'service-booking':
        const primaryColor = this.themeColor(data.primary_color, '--theme-primary', '#0d6efd');
        const sbBorderColor = this.themeColor('', '--theme-card-border', '#dee2e6');
        const sbMutedBg = this.themeColor('', '--theme-surface-hover', '#e9ecef');
        const sbMutedText = this.themeColor('', '--theme-text-muted', '#6c757d');
        return `
          <div class="component-label">${this.getComponentTypeLabel('service-booking')}</div>
          <div class="container py-4">
            <div class="text-center mb-4">
              <h2 class="mb-2">${data.title || this.t('pages.pageEditor.preview.serviceBooking.title', '服務預約')}</h2>
              <p class="text-muted">${data.subtitle || this.t('pages.pageEditor.preview.serviceBooking.subtitle', '請依照以下步驟完成預約')}</p>
            </div>
            
            <!-- 步驟指示器 -->
            <div class="d-flex justify-content-center mb-4">
              <div class="d-flex align-items-center">
                <div class="rounded-circle d-flex align-items-center justify-content-center text-white" style="width: 36px; height: 36px; background: ${primaryColor};">
                  <span>1</span>
                </div>
                <span class="ms-2 me-4 fw-medium">${data.step1_title || this.t('pages.pageEditor.preview.serviceBooking.step1', '選擇服務')}</span>
                
                <div class="border-top" style="width: 40px; border-color: ${sbBorderColor} !important;"></div>
                
                <div class="rounded-circle d-flex align-items-center justify-content-center ms-4" style="width: 36px; height: 36px; background: ${sbMutedBg}; color: ${sbMutedText};">
                  <span>2</span>
                </div>
                <span class="ms-2 me-4 text-muted">${data.step2_title || this.t('pages.pageEditor.preview.serviceBooking.step2', '選擇時間')}</span>
                
                <div class="border-top" style="width: 40px; border-color: ${sbBorderColor} !important;"></div>
                
                <div class="rounded-circle d-flex align-items-center justify-content-center ms-4" style="width: 36px; height: 36px; background: ${sbMutedBg}; color: ${sbMutedText};">
                  <span>3</span>
                </div>
                <span class="ms-2 text-muted">${data.step3_title || this.t('pages.pageEditor.preview.serviceBooking.step3', '確認資料')}</span>
              </div>
            </div>
            
            <!-- 步驟 1：選擇服務 -->
            <div class="card shadow-sm">
              <div class="card-body p-4">
                <h5 class="card-title mb-4"><i class="bi bi-1-circle me-2" style="color: ${primaryColor};"></i>${data.step1_title || this.t('pages.pageEditor.preview.serviceBooking.step1', '選擇服務')}</h5>
                
                ${data.show_service_select !== false ? `
                <div class="mb-4">
                  <label class="form-label fw-medium">${this.t('pages.pageEditor.preview.serviceBooking.selectService', '請選擇服務項目')}</label>
                  <div class="row g-3">
                    <div class="col-md-4">
                      <div class="card border-2" style="border-color: ${primaryColor} !important; cursor: pointer;">
                        <div class="card-body text-center py-3">
                          <i class="bi bi-scissors fs-3 mb-2" style="color: ${primaryColor};"></i>
                          <div class="fw-medium">${this.t('pages.pageEditor.preview.serviceBooking.service1', '基礎服務')}</div>
                          <small class="text-muted">$500 / 60 ${this.t('pages.pageEditor.preview.serviceBooking.minutes', '分鐘')}</small>
                        </div>
                      </div>
                    </div>
                    <div class="col-md-4">
                      <div class="card border" style="cursor: pointer;">
                        <div class="card-body text-center py-3">
                          <i class="bi bi-star fs-3 mb-2 text-muted"></i>
                          <div class="fw-medium">${this.t('pages.pageEditor.preview.serviceBooking.service2', '進階服務')}</div>
                          <small class="text-muted">$800 / 90 ${this.t('pages.pageEditor.preview.serviceBooking.minutes', '分鐘')}</small>
                        </div>
                      </div>
                    </div>
                    <div class="col-md-4">
                      <div class="card border" style="cursor: pointer;">
                        <div class="card-body text-center py-3">
                          <i class="bi bi-gem fs-3 mb-2 text-muted"></i>
                          <div class="fw-medium">${this.t('pages.pageEditor.preview.serviceBooking.service3', 'VIP 服務')}</div>
                          <small class="text-muted">$1,200 / 120 ${this.t('pages.pageEditor.preview.serviceBooking.minutes', '分鐘')}</small>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                ` : ''}
                
                ${data.show_staff_select !== false ? `
              <div class="mb-3">
                <label class="form-label">${this.t('pages.pageEditor.preview.serviceBooking.step1', 'Select Service')}</label>
                <select class="form-select">
                  <option selected>${this.t('pages.pageEditor.preview.serviceBooking.selectService', 'Please select a service')}</option>
                  <option value="1">${this.t('pages.pageEditor.preview.serviceBooking.service1', 'Basic Service')} ($500 / 30 ${this.t('pages.pageEditor.preview.serviceBooking.minutes', 'minutes')})</option>
                  <option value="2">${this.t('pages.pageEditor.preview.serviceBooking.service2', 'Advanced Service')} ($800 / 60 ${this.t('pages.pageEditor.preview.serviceBooking.minutes', 'minutes')})</option>
                  <option value="3">${this.t('pages.pageEditor.preview.serviceBooking.service3', 'VIP Service')} ($1500 / 90 ${this.t('pages.pageEditor.preview.serviceBooking.minutes', 'minutes')})</option>
                </select>
              </div>
              
              ${data.show_staff_select ? `
                <div class="mb-3">
                  <label class="form-label">${this.t('pages.pageEditor.preview.serviceBooking.selectStaff', 'Select Staff (Optional)')}</label>
                  <select class="form-select">
                    <option value="">${this.t('pages.pageEditor.preview.serviceBooking.anyStaff', 'Any Staff')}</option>
                    <option value="1">Alice</option>
                    <option value="2">Bob</option>
                  </select>
                </div>
              ` : ''}
              
              <div class="d-grid">
                <button class="btn btn-primary" style="background-color: ${primaryColor}; border-color: ${primaryColor};">${this.t('pages.pageEditor.preview.serviceBooking.next', 'Next')}</button>
              </div>
                ` : ''}
                
                <div class="text-end mt-4">
                  <button class="btn btn-lg" style="background: ${primaryColor}; color: white;">
                    ${this.t('pages.pageEditor.preview.serviceBooking.next', '下一步')} <i class="bi bi-arrow-right"></i>
                  </button>
                </div>
              </div>
            </div>
          </div>
        `;
      case 'dining-menu':
        const menuTitle = data.title || this.t('pages.pageEditor.defaults.diningMenu.title', '菜單');
        const menuSubtitle = data.subtitle || this.t('pages.pageEditor.defaults.diningMenu.subtitle', '瀏覽餐點與價格');
        return `
          <div class="component-label">${this.getComponentTypeLabel('dining-menu')}</div>
          <div class="container py-4">
            <div class="border rounded p-4 bg-light">
              <div class="fw-semibold mb-1">${menuTitle}</div>
              <div class="text-muted small mb-3">${menuSubtitle}</div>
              <div class="row g-2">
                <div class="col-12 col-md-6">
                  <div class="d-flex justify-content-between border rounded bg-white p-2">
                    <span>${this.t('pages.pageEditor.preview.diningMenu.dishA', 'Signature Dish A')}</span>
                    <span class="fw-semibold">$180</span>
                  </div>
                </div>
                <div class="col-12 col-md-6">
                  <div class="d-flex justify-content-between border rounded bg-white p-2">
                    <span>${this.t('pages.pageEditor.preview.diningMenu.dishB', 'Special Dish B')}</span>
                    <span class="fw-semibold">$220</span>
                  </div>
                </div>
                <div class="col-12 col-md-6">
                  <div class="d-flex justify-content-between border rounded bg-white p-2">
                    <span>${this.t('pages.pageEditor.preview.diningMenu.salad', 'Seasonal Salad')}</span>
                    <span class="fw-semibold">$150</span>
                  </div>
                </div>
                <div class="col-12 col-md-6">
                  <div class="d-flex justify-content-between border rounded bg-white p-2">
                    <span>${this.t('pages.pageEditor.preview.diningMenu.drinks', 'Beverages')}</span>
                    <span class="fw-semibold">$80</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        `;
      case 'dining-table-reservation':
        const reservationTitle = data.title || this.t('pages.pageEditor.defaults.diningReservation.title', '預約餐桌');
        const reservationSubtitle = data.subtitle || this.t('pages.pageEditor.defaults.diningReservation.subtitle', '填寫聯絡資訊完成預約');
        return `
          <div class="component-label">${this.getComponentTypeLabel('dining-table-reservation')}</div>
          <div class="container py-4">
            <div class="card shadow-sm">
              <div class="card-body">
                <h5 class="mb-2">${reservationTitle}</h5>
                <p class="text-muted">${reservationSubtitle}</p>
                <div class="row g-2">
                  <div class="col-md-6">
                    <label class="form-label">${this.t('pages.pageEditor.preview.contact.name', 'Name')}</label>
                    <input type="text" class="form-control" placeholder="${this.t('pages.pageEditor.preview.contact.demoName', 'John Doe')}">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">${this.t('pages.pageEditor.preview.contact.phone', 'Phone')}</label>
                    <input type="tel" class="form-control" placeholder="09xx-xxx-xxx">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">${this.t('pages.pageEditor.preview.diningReservation.people', 'Number of people')}</label>
                    <input type="number" class="form-control" min="1" value="2">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">${this.t('pages.pageEditor.preview.diningReservation.time', 'Time')}</label>
                    <input type="text" class="form-control" placeholder="2026/02/01 19:00">
                  </div>
                </div>
                <button class="btn btn-primary w-100 mt-3">${this.t('pages.pageEditor.labels.diningReservation.submit', '送出預約')}</button>
              </div>
            </div>
          </div>
        `;
      case 'product-list':
        const productColumns = data.columns || 3;
        // Use Bootstrap responsive column classes (matching public page)
        const productColumnClass = productColumns === 2 ? 'col-md-6 col-sm-12' : productColumns === 3 ? 'col-md-4 col-sm-6' : 'col-md-3 col-sm-6';
        const productLimit = data.limit || 12;
        const productDetailPage = data.product_detail_page || '';
        return `
          <div class="component-label">${this.getComponentTypeLabel('product-list')} (${productColumns} ${this.t('pages.pageEditor.units.columns', 'cols')}, ${productLimit} ${this.t('pages.pageEditor.units.items', 'items')})</div>
          <div class="container py-4">
            <div class="row g-3">
              ${Array(productLimit).fill(0).map((_, index) => {
                const productLink = productDetailPage ? productDetailPage : '#';
                return `<div class="${productColumnClass}">
                  <a href="${productLink}" style="text-decoration: none; color: inherit;">
                    <div class="card h-100 border-0 shadow-sm">
                      <div class="bg-light" style="aspect-ratio: 1 / 1; overflow: hidden; border-radius: 0.375rem 0.375rem 0 0;">
                        <img src="/static/product.jpg" alt="Product" style="width: 100%; height: 100%; object-fit: cover;" onerror="this.style.display='none'; this.parentElement.innerHTML='<i class=\\'bi bi-image fs-1 text-muted d-flex align-items-center justify-content-center h-100\\'></i>';">
                      </div>
                      <div class="card-body text-center">
                        <h5 class="card-title mb-2">${this.t('pages.pageEditor.preview.productList.name', 'Product name')}</h5>
                        <p class="card-text text-muted mb-0">${this.t('pages.pageEditor.preview.productList.price', 'Price')}</p>
                      </div>
                    </div>
                  </a>
                </div>`;
              }).join('')}
            </div>
          </div>
        `;
      case 'service-list':
        const serviceColumns = data.columns || 3;
        const serviceColumnClass = serviceColumns === 2 ? 'col-md-6 col-sm-12' : serviceColumns === 3 ? 'col-md-4 col-sm-6' : 'col-md-3 col-sm-6';
        const serviceLimit = data.limit || 12;
        const serviceDetailPage = data.service_detail_page || '';
        return `
          <div class="component-label">${this.getComponentTypeLabel('service-list')} (${serviceColumns} ${this.t('pages.pageEditor.units.columns', 'cols')}, ${serviceLimit} ${this.t('pages.pageEditor.units.items', 'items')})</div>
          <div class="container py-4">
            <div class="row g-3">
              ${Array(serviceLimit).fill(0).map((_, index) => {
                const serviceLink = serviceDetailPage ? serviceDetailPage : '#';
                return `<div class="${serviceColumnClass}">
                  <a href="${serviceLink}" style="text-decoration: none; color: inherit;">
                    <div class="card h-100 border-0 shadow-sm">
                      <div class="bg-light" style="aspect-ratio: 1 / 1; overflow: hidden; border-radius: 0.375rem 0.375rem 0 0;">
                        <img src="/static/product.jpg" alt="Service" style="width: 100%; height: 100%; object-fit: cover;" onerror="this.style.display='none'; this.parentElement.innerHTML='<i class=\\'bi bi-tools fs-1 text-muted d-flex align-items-center justify-content-center h-100\\'></i>';">
                      </div>
                      <div class="card-body text-center">
                        <h5 class="card-title mb-2">${this.t('pages.pageEditor.preview.serviceList.name', 'Service name')}</h5>
                        <p class="card-text text-muted mb-0">${this.t('pages.pageEditor.preview.serviceList.price', 'Price')}</p>
                      </div>
                    </div>
                  </a>
                </div>`;
              }).join('')}
            </div>
          </div>
        `;
      case 'product-detail':
        return `
          <div class="component-label">${this.getComponentTypeLabel('product-detail')}</div>
          <div class="container py-4">
            <div class="row">
              <div class="col-md-6">
                <div class="bg-light d-flex align-items-center justify-content-center" style="aspect-ratio: 1 / 1; border-radius: 0.375rem;">
                  <i class="bi bi-image fs-1 text-muted"></i>
                </div>
              </div>
              <div class="col-md-6">
                <h2 class="editable-text" contenteditable="true" data-field="product_name">${this.t('pages.pageEditor.preview.productList.name', 'Product name')}</h2>
                <p class="fs-4 text-primary editable-text" contenteditable="true" data-field="product_price">$0.00</p>
                <div class="mb-3 editable-text" contenteditable="true" data-field="product_description">
                  <p>${this.t('pages.pageEditor.preview.productDetail.description', 'Product Description')}</p>
                </div>
                <button class="btn btn-primary btn-lg">${this.t('pages.pageEditor.preview.productDetail.addToCart', 'Add to Cart')}</button>
              </div>
            </div>
          </div>
        `;
      case 'google-map':
        const mapLat = data.lat || 25.033964;
        const mapLng = data.lng || 121.564468;
        const mapZoom = data.zoom || 14;
        let mapHeight = '400px';
        {
          let h = (data.height || '400px').toString().trim();
          // Strip all trailing unit suffixes, then re-add one
          h = h.replace(/(px|rem|em|vh|vw|%)+$/gi, '');
          if (/^\d+(\.\d+)?$/.test(h)) h += 'px';
          else if (!/\d+(px|%|rem|em|vh|vw)$/i.test(h)) h = '400px';
          mapHeight = h;
        }
        const mapMarkerTitle = (data.marker_title || '').replace(/"/g, '&quot;');
        const mapId = 'gmap-preview-' + (comp ? comp.id || '' : Math.random().toString(36).substr(2, 9));
        return `
          <div class="component-label">${this.getComponentTypeLabel('google-map')}</div>
          <div style="height: ${mapHeight}; position: relative; background: #e9ecef; border-radius: 0.375rem; overflow: hidden;">
            <div id="${mapId}" class="gmap-preview-container" data-lat="${mapLat}" data-lng="${mapLng}" data-zoom="${mapZoom}" data-marker="${mapMarkerTitle}" style="width: 100%; height: 100%;"></div>
          </div>
        `;
      case 'custom-html':
        const customHtmlContent = data.html_content || '<p>Your HTML here</p>';
        return `
          <div class="component-label">${this.getComponentTypeLabel('custom-html')}</div>
          <div class="container">
            <div class="custom-html-preview">${customHtmlContent}</div>
          </div>
        `;
      default:
        return `
            <div style="padding: 1.5rem; outline: 1px dashed rgba(222, 226, 230, 0.4); outline-offset: -1px; border-radius: 4px; text-align: center; color: var(--theme-text-muted, #6c757d);">
             ${this.t('pages.pageEditor.unknownComponentType', 'Unknown component type')}: ${type}
           </div>
        `;
    }
  },

  selectComponent(component) {
    this.selectedComponent = component;
    
    // 清除之前的 label 隐藏定时器
    if (this.labelHideTimeout) {
      clearTimeout(this.labelHideTimeout);
      this.labelHideTimeout = null;
    }
    
    // 清除所有元件的 function bar 隐藏定时器
    document.querySelectorAll('.component-actions').forEach(actions => {
      const actionsTimeout = actions.dataset.actionsTimeout;
      if (actionsTimeout) {
        clearTimeout(parseInt(actionsTimeout));
        actions.dataset.actionsTimeout = '';
      }
    });
    
    // 更新視覺選中狀態
    document.querySelectorAll('.component-item').forEach(item => {
      item.classList.remove('selected');
      // 清除底部 padding
      item.style.paddingBottom = '';
      // 隐藏未选中项的功能按钮
      const actions = item.querySelector('.component-actions');
      if (actions && !item.classList.contains('selected')) {
        actions.style.display = 'none';
      }
      // 立即隐藏所有未选中项的 label
      const label = item.querySelector('.component-label');
      if (label && !item.classList.contains('selected')) {
        label.style.display = 'none';
        label.style.opacity = '0';
      }
    });
    const index = this.components.indexOf(component);
    const item = document.querySelector(`.component-item[data-index="${index}"]`);
    if (item) {
      item.classList.add('selected');
      // 添加底部 padding，防止底部按钮叠在其他元素上
      item.style.paddingBottom = '46px';
      // 显示选中项的功能按钮（完整显示）
      const actions = item.querySelector('.component-actions');
      if (actions) {
        actions.style.display = 'flex';
        actions.style.setProperty('transform', 'translateX(0)', 'important');
        actions.style.setProperty('opacity', '1', 'important');
        
        // 点击后2秒自动收起（即使选中状态）
        // 查找该元件的 function bar 定时器
        const actionsTimeout = actions.dataset.actionsTimeout;
        if (actionsTimeout) {
          clearTimeout(parseInt(actionsTimeout));
        }
        
        // 设置新的定时器，2秒后收起（参考 label 的逻辑）
        const timeoutId = setTimeout(() => {
          // 再次检查是否还是当前选中的元件（跟 label 的逻辑完全一样）
          if (item.classList.contains('selected')) {
            // 不检查 hover 状态，直接收起（跟 label 一样）
            actions.style.setProperty('transform', 'translateX(calc(100% - 12px))', 'important');
            actions.style.setProperty('opacity', '0.7', 'important');
          }
        }, 2000);
        
        // 保存定时器 ID 到 dataset
        actions.dataset.actionsTimeout = timeoutId.toString();
      }
      
      // 显示 label 2秒后隐藏
      const label = item.querySelector('.component-label');
      if (label) {
        label.style.display = 'block';
        label.style.opacity = '1';
        // 清除之前的定时器（双重保险）
        if (this.labelHideTimeout) {
          clearTimeout(this.labelHideTimeout);
        }
        // 2秒后隐藏
        this.labelHideTimeout = setTimeout(() => {
          // 再次检查是否还是当前选中的元件
          if (item.classList.contains('selected')) {
            label.style.opacity = '0';
            setTimeout(() => {
              if (item.classList.contains('selected')) {
                label.style.display = 'none';
              }
            }, 200); // 等待 transition 完成
          }
        }, 2000);
      }
      
      // 检查元件高度
      const preview = item.querySelector('.component-preview');
      if (preview) {
        const height = preview.offsetHeight;
        if (height < 50) {
          item.classList.add('small-component');
        } else {
          item.classList.remove('small-component');
        }
      }
    }
    
    // 渲染屬性面板
    this.renderProperties(component);
  },

  // 輔助函數：解析帶單位的值（如 "16px" -> {value: 16, unit: "px"}）
  parseValueWithUnit(value) {
    if (!value || typeof value !== 'string') return { value: '', unit: 'px' };
    const match = value.match(/^([\d.]+)(px|rem|em|%)?$/);
    if (match) {
      return { value: match[1], unit: match[2] || 'px' };
    }
    return { value: value, unit: 'px' };
  },
  
  // 輔助函數：解析內邊距（如 "10px 20px" -> {top: 10, right: 20, bottom: 10, left: 20, unit: "px"}）
  parsePadding(padding) {
    if (!padding || typeof padding !== 'string') {
      return { top: '', right: '', bottom: '', left: '', unit: 'px' };
    }
    const parts = padding.trim().split(/\s+/);
    const firstPart = this.parseValueWithUnit(parts[0] || '');
    const unit = firstPart.unit || 'px';
    
    if (parts.length === 1) {
      // 單一值：所有邊相同
      return { top: firstPart.value, right: firstPart.value, bottom: firstPart.value, left: firstPart.value, unit };
    } else if (parts.length === 2) {
      // 兩個值：上下、左右
      const secondPart = this.parseValueWithUnit(parts[1] || '');
      return { top: firstPart.value, right: secondPart.value, bottom: firstPart.value, left: secondPart.value, unit };
    } else if (parts.length === 4) {
      // 四個值：上、右、下、左
      const secondPart = this.parseValueWithUnit(parts[1] || '');
      const thirdPart = this.parseValueWithUnit(parts[2] || '');
      const fourthPart = this.parseValueWithUnit(parts[3] || '');
      return { top: firstPart.value, right: secondPart.value, bottom: thirdPart.value, left: fourthPart.value, unit };
    }
    return { top: firstPart.value, right: '', bottom: '', left: '', unit };
  },
  
  // 輔助函數：生成帶單位的輸入框 HTML
  renderSizeInput(id, label, value, placeholder = '') {
    const parsed = this.parseValueWithUnit(value || '');
    return `
      <div class="mb-3">
        <label class="form-label">${label}</label>
        <div class="input-group">
          <input type="text" class="form-control" id="${id}" value="${parsed.value}" placeholder="${placeholder}">
          <select class="form-select" style="width: 120px;" id="${id}-unit">
            <option value="px" ${parsed.unit === 'px' ? 'selected' : ''}>px</option>
            <option value="rem" ${parsed.unit === 'rem' ? 'selected' : ''}>rem</option>
            <option value="em" ${parsed.unit === 'em' ? 'selected' : ''}>em</option>
            <option value="%" ${parsed.unit === '%' ? 'selected' : ''}>%</option>
          </select>
        </div>
      </div>
    `;
  },
  
  // 輔助函數：生成內邊距輸入框 HTML（5個：上下左右+單位）
  renderPaddingInput(id, label, value) {
    const parsed = this.parsePadding(value || '');
    return `
      <div class="mb-3">
        <label class="form-label">${label}</label>
        <div class="row g-2">
          <div class="col-3">
            <input type="text" class="form-control form-control-sm" id="${id}-top" value="${parsed.top}" placeholder="${this.t('pages.pageEditor.padding.top', 'Top')}">
          </div>
          <div class="col-3">
            <input type="text" class="form-control form-control-sm" id="${id}-right" value="${parsed.right}" placeholder="${this.t('pages.pageEditor.padding.right', 'Right')}">
          </div>
          <div class="col-3">
            <input type="text" class="form-control form-control-sm" id="${id}-bottom" value="${parsed.bottom}" placeholder="${this.t('pages.pageEditor.padding.bottom', 'Bottom')}">
          </div>
          <div class="col-3">
            <input type="text" class="form-control form-control-sm" id="${id}-left" value="${parsed.left}" placeholder="${this.t('pages.pageEditor.padding.left', 'Left')}">
          </div>
          <div class="col-12 mt-1">
            <select class="form-select form-select-sm" id="${id}-unit">
              <option value="px" ${parsed.unit === 'px' ? 'selected' : ''}>px</option>
              <option value="rem" ${parsed.unit === 'rem' ? 'selected' : ''}>rem</option>
              <option value="em" ${parsed.unit === 'em' ? 'selected' : ''}>em</option>
              <option value="%" ${parsed.unit === '%' ? 'selected' : ''}>%</option>
            </select>
          </div>
        </div>
      </div>
    `;
  },
  
  // 輔助函數：從帶單位的輸入框獲取值
  getSizeValue(id) {
    const value = document.getElementById(id)?.value || '';
    const unit = document.getElementById(`${id}-unit`)?.value || 'px';
    return value ? `${value}${unit}` : '';
  },
  
  // 輔助函數：從內邊距輸入框獲取值
  getPaddingValue(id) {
    const top = document.getElementById(`${id}-top`)?.value || '';
    const right = document.getElementById(`${id}-right`)?.value || '';
    const bottom = document.getElementById(`${id}-bottom`)?.value || '';
    const left = document.getElementById(`${id}-left`)?.value || '';
    const unit = document.getElementById(`${id}-unit`)?.value || 'px';
    
    if (!top && !right && !bottom && !left) return '';
    
    // 如果所有值相同，返回單一值
    if (top === right && right === bottom && bottom === left) {
      return `${top}${unit}`;
    }
    // 如果上下相同且左右相同，返回兩個值
    if (top === bottom && right === left) {
      return `${top}${unit} ${right}${unit}`;
    }
    // 否則返回四個值
    return `${top}${unit} ${right}${unit} ${bottom}${unit} ${left}${unit}`;
  },

  renderProperties(component) {
    const panel = document.getElementById('propertiesContent');
    const type = component.component_type;
    const data = component.component_data || {};
    
    let html = `<h6 class="mb-3">${this.getComponentTypeLabel(type)}</h6>`;
    
    // Show linked block warning if this component references a block
    if (component.block_id) {
      const compIndex = this.components.indexOf(component);
      html += `
        <div class="alert alert-info py-2 px-3 mb-3" style="font-size: 0.85rem;">
          <i class="bi bi-link-45deg"></i> <strong>${this.t('pages.pageEditor.linkedBlockLabel', 'Linked Block')}</strong><br>
          <small>${this.t('pages.pageEditor.linkedBlockWarning', 'This component is linked to a shared block. Click \'Edit Block\' to update all pages using this block at once; otherwise, you\'ll need to update each page individually.')}</small>
          <div class="mt-2 d-flex gap-2">
            <button class="btn btn-sm btn-outline-primary" onclick="PageEditor.editLinkedBlock('${component.block_id}')">
              <i class="bi bi-pencil"></i> ${this.t('pages.pageEditor.editBlock', 'Edit Block')}
            </button>
            <button class="btn btn-sm btn-outline-secondary" onclick="PageEditor.unlinkBlock(${compIndex})">
              <i class="bi bi-x-lg"></i> ${this.t('pages.pageEditor.unlinkAndEdit', 'Unlink')}
            </button>
          </div>
        </div>
      `;
    }
    
    let contentHtml = '';
    let styleHtml = '';
    
    switch (type) {
      case 'hero':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.hero.title', 'Title')}</label>
            <input type="text" class="form-control" id="prop-title" value="${data.title || ''}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.hero.subtitle', 'Subtitle')}</label>
            <input type="text" class="form-control" id="prop-subtitle" value="${data.subtitle || ''}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.hero.buttonText', 'Button text')}</label>
            <input type="text" class="form-control" id="prop-button-text" value="${data.button_text || ''}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.hero.buttonLink', 'Button link')}</label>
            <input type="text" class="form-control" id="prop-button-link" value="${_stripTenantLinkPrefix(data.button_link || '#')}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.hero.buttonStyle', 'Button style')}</label>
            <select class="form-select" id="prop-button-style">
              <option value="primary" ${data.button_style === 'primary' || !data.button_style ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.primary', 'Primary')}</option>
              <option value="secondary" ${data.button_style === 'secondary' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.secondary', 'Secondary')}</option>
              <option value="success" ${data.button_style === 'success' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.success', 'Success')}</option>
              <option value="danger" ${data.button_style === 'danger' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.danger', 'Danger')}</option>
              <option value="warning" ${data.button_style === 'warning' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.warning', 'Warning')}</option>
              <option value="info" ${data.button_style === 'info' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.info', 'Info')}</option>
              <option value="light" ${data.button_style === 'light' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.light', 'Light')}</option>
              <option value="dark" ${data.button_style === 'dark' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.dark', 'Dark')}</option>
            </select>
          </div>
        `;
        styleHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.hero.backgroundImage', 'Background image')}</label>
            <div class="d-flex align-items-center gap-2 mb-2">
              ${data.background_image ? `<img src="${data.background_image}" alt="背景" style="max-height: 80px; max-width: 150px; border: 1px solid #dee2e6; border-radius: 4px; padding: 4px;">` : ''}
              <input type="file" class="form-control form-control-sm" id="prop-bg-image-file" accept="image/*" style="display: none;">
              <button type="button" class="btn btn-sm btn-outline-primary" onclick="document.getElementById('prop-bg-image-file').click();">${data.background_image ? this.t('pages.pageEditor.actions.replace', 'Replace') : this.t('common.upload', 'Upload')} ${this.t('pages.pageEditor.labels.hero.backgroundImage', 'Background image')}</button>
              ${data.background_image ? `<button type="button" class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeHeroBgImage();">${this.t('common.remove', 'Remove')}</button>` : ''}
            </div>
            <input type="hidden" id="prop-background-image" value="${data.background_image || ''}">
            <div class="mb-3">
              ${this.renderThemeColorPicker('prop-bg-color', this.t('pages.pageEditor.labels.hero.backgroundColorNoImage', 'Background color (used when no image)'), data.bg_color, '--theme-hero-bg', '#0d6efd')}
            </div>
          </div>
          ${this.renderThemeColorPicker('prop-text-color', this.t('pages.pageEditor.labels.common.textColor', 'Text color'), data.text_color, '--theme-hero-text', '#ffffff')}
          ${this.renderThemeColorPicker('prop-heading-color', this.t('pages.pageEditor.labels.hero.headingColor', 'Heading color'), data.heading_color, '--theme-hero-heading', '#ffffff')}
          ${this.renderSizeInput('prop-min-height', this.t('pages.pageEditor.labels.common.minHeight', 'Min height'), data.min_height || '300px')}
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '3rem')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.textAlign', 'Text align')}</label>
            <select class="form-select" id="prop-text-align">
              <option value="left" ${data.text_align === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${data.text_align === 'center' || !data.text_align ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${data.text_align === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
            </select>
          </div>
        `;
        break;
      case 'text':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.text.content', 'Content')}</label>
            <textarea class="form-control" id="prop-content" rows="5">${data.content || ''}</textarea>
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-text-color', this.t('pages.pageEditor.labels.common.textColor', 'Text color'), data.text_color, '--theme-text', '#333333')}
          ${this.renderSizeInput('prop-font-size', this.t('pages.pageEditor.labels.common.fontSize', 'Font size'), data.font_size || '1rem')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.text.lineHeight', 'Line height')}</label>
            <input type="text" class="form-control" id="prop-line-height" value="${data.line_height || '1.8'}" placeholder="${this.t('pages.pageEditor.placeholders.exampleLineHeight', 'e.g., 1.8')}">
          </div>
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '1.5rem')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.textAlign', 'Text align')}</label>
            <select class="form-select" id="prop-text-align">
              <option value="left" ${data.text_align === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${data.text_align === 'center' ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${data.text_align === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
              <option value="justify" ${data.text_align === 'justify' ? 'selected' : ''}>${this.t('pages.pageEditor.align.justify', 'Justify')}</option>
            </select>
          </div>
        `;
        break;
      case 'heading':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.heading.text', 'Heading text')}</label>
            <input type="text" class="form-control" id="prop-text" value="${data.text || ''}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.heading.level', 'Heading level')}</label>
            <select class="form-select" id="prop-level">
              <option value="h1" ${data.level === 'h1' ? 'selected' : ''}>H1</option>
              <option value="h2" ${data.level === 'h2' ? 'selected' : ''}>H2</option>
              <option value="h3" ${data.level === 'h3' ? 'selected' : ''}>H3</option>
            </select>
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-text-color', this.t('pages.pageEditor.labels.common.textColor', 'Text color'), data.text_color, '--theme-heading-color', '#333333')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.textAlign', 'Text align')}</label>
            <select class="form-select" id="prop-text-align">
              <option value="left" ${data.text_align === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${data.text_align === 'center' ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${data.text_align === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
            </select>
          </div>
          ${this.renderPaddingInput('prop-margin', this.t('pages.pageEditor.labels.heading.marginY', 'Vertical margin'), data.margin || '1.5rem 0')}
        `;
        break;
      case 'section':
        // 收集所有栏的子元件（用于显示）
        let allChildren = [];
        if (data.column_children && Array.isArray(data.column_children)) {
          // 使用新的 column_children 结构
          data.column_children.forEach((columnChildren, colIdx) => {
            if (Array.isArray(columnChildren)) {
              columnChildren.forEach((child, childIdx) => {
                allChildren.push({ child, colIdx, childIdx });
              });
            }
          });
        } else if (data.children && Array.isArray(data.children)) {
          // 兼容旧的 children 结构
          data.children.forEach((child, idx) => {
            allChildren.push({ child, colIdx: 0, childIdx: idx });
          });
        }
        
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.section.columns', 'Columns')}</label>
            <select class="form-select" id="prop-columns">
              <option value="1" ${data.columns === 1 ? 'selected' : ''}>1</option>
              <option value="2" ${data.columns === 2 ? 'selected' : ''}>2</option>
              <option value="3" ${data.columns === 3 ? 'selected' : ''}>3</option>
              <option value="4" ${data.columns === 4 ? 'selected' : ''}>4</option>
            </select>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.section.children', 'Child components')} (${allChildren.length})</label>
            <div class="border rounded p-2" style="max-height: 200px; overflow-y: auto;">
              ${allChildren.length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.labels.section.noChildrenHint', 'No child components yet. Drag components into the section to add.')}</small>` : ''}
              ${allChildren.map((item, idx) => `
                <div class="d-flex justify-content-between align-items-center mb-1 p-1 bg-light rounded">
                  <small>${this.t('pages.pageEditor.labels.section.columnPrefix', 'Column')} ${item.colIdx + 1}: ${this.getComponentTypeLabel(item.child.component_type || 'unknown')}</small>
                  <button class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeChildFromSection(${this.components.indexOf(component)}, ${item.colIdx}, ${item.childIdx})" title="${this.t('common.remove', 'Remove')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `).join('')}
            </div>
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-background-color', this.t('pages.pageEditor.labels.common.backgroundColor', 'Background color'), data.background_color, '--theme-bg', '#ffffff')}
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '10px')}
        `;
        break;
      case 'nav':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.nav.logoImage', 'Logo image')}</label>
            <div class="d-flex align-items-center gap-2 mb-2">
              ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 50px; max-width: 150px; border: 1px solid #dee2e6; border-radius: 4px; padding: 4px;">` : ''}
              <input type="file" class="form-control form-control-sm" id="prop-logo-file" accept="image/*" style="display: none;">
              <button type="button" class="btn btn-sm btn-outline-primary" onclick="document.getElementById('prop-logo-file').click();">${data.logo ? this.t('pages.pageEditor.actions.replace', 'Replace') : this.t('common.upload', 'Upload')} Logo</button>
              ${data.logo ? `<button type="button" class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeNavLogo();">${this.t('common.remove', 'Remove')}</button>` : ''}
            </div>
            <input type="hidden" id="prop-logo" value="${data.logo || ''}">
            <div class="mb-3">
              <label class="form-label">${this.t('pages.pageEditor.labels.nav.logoText', 'Logo text (shown when no image)')}</label>
              <input type="text" class="form-control" id="prop-logo-text" value="${data.logo_text || 'Logo'}">
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.nav.menuItems', 'Menu items')}</label>
            <div id="nav-menu-items-list" class="border rounded p-2 mb-2" style="max-height: 200px; overflow-y: auto;">
              ${(data.menu_items || []).length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</small>` : ''}
              ${(data.menu_items || []).map((item, idx) => {
                const isPageLink = item.link && item.link.startsWith('/page/');
                const customLink = isPageLink ? '' : _stripTenantLinkPrefix(item.link || '');
                return `
                <div class="d-flex justify-content-between align-items-center mb-2 p-2 bg-light rounded" data-menu-index="${idx}">
                  <div class="flex-grow-1">
                    <input type="text" class="form-control form-control-sm mb-1" value="${item.text || ''}" placeholder="${this.t('pages.pageEditor.placeholders.menuText', 'Menu text')}" onchange="PageEditor.updateMenuItem(${idx}, 'text', this.value);">
                    <div class="d-flex gap-2">
                      <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" onchange="PageEditor.updateMenuItemLinkType(${idx}, this.value);">
                        <option value="page" ${isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                        <option value="custom" ${!isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                      </select>
                      ${isPageLink ? `
                        <select class="form-select form-select-sm flex-grow-1" onchange="PageEditor.updateMenuItem(${idx}, 'link', this.value);">
                          <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                          ${this.allPages.map(page => `<option value="/page/${page.slug}" ${item.link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                        </select>
                      ` : ''}
                    </div>
                    ${!isPageLink ? `
                      <div class="w-100 mt-2">
                        <input type="text" class="form-control form-control-sm" value="${customLink}" placeholder="${this.t('pages.pageEditor.props.customLinkPlaceholder', 'Enter link (e.g., /about or https://example.com)')}" onchange="PageEditor.updateMenuItem(${idx}, 'link', this.value);">
                      </div>
                    ` : ''}
                  </div>
                  <button class="btn btn-sm btn-outline-danger ms-2" onclick="PageEditor.removeMenuItem(${idx});" title="${this.t('pages.pageEditor.actions.delete', 'Delete')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `;
              }).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" onclick="PageEditor.addMenuItem();">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.props.addMenuItem', 'Add menu item')}
            </button>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.showIcons', 'Show icons')}</label>
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-login-icon" ${data.show_login_icon !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-login-icon">${this.t('pages.pageEditor.props.loginIcon', 'Login icon')}</label>
            </div>
            ${data.show_login_icon !== false ? `
              <div class="mb-2 ms-4">
                <label class="form-label small">${this.t('pages.pageEditor.props.loginIconLink', 'Login icon link')}</label>
                <div class="d-flex gap-2">
                  <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" id="prop-login-icon-link-type" onchange="PageEditor.updateNavIconLinkType('login', this.value);">
                    <option value="page" ${data.login_icon_link && data.login_icon_link.startsWith('/page/') ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                    <option value="custom" ${!data.login_icon_link || !data.login_icon_link.startsWith('/page/') ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                  </select>
                  ${data.login_icon_link && data.login_icon_link.startsWith('/page/') ? `
                    <select class="form-select form-select-sm flex-grow-1" id="prop-login-icon-link" onchange="PageEditor.updateNavIconLink('login', this.value);">
                      <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                      ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.login_icon_link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                    </select>
                  ` : `
                    <input type="text" class="form-control form-control-sm flex-grow-1" id="prop-login-icon-link" value="${_stripTenantLinkPrefix(data.login_icon_link || '')}" placeholder="${this.t('pages.pageEditor.props.enterLink', 'Enter link')}" onchange="PageEditor.updateNavIconLink('login', this.value);">
                  `}
                </div>
              </div>
            ` : ''}
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-cart-icon" ${data.show_cart_icon !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-cart-icon">${this.t('pages.pageEditor.props.cartIcon', 'Cart icon')}</label>
            </div>
            ${data.show_cart_icon !== false ? `
              <div class="mb-2 ms-4">
                <label class="form-label small">${this.t('pages.pageEditor.props.cartIconLink', 'Cart icon link')}</label>
                <div class="d-flex gap-2">
                  <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" id="prop-cart-icon-link-type" onchange="PageEditor.updateNavIconLinkType('cart', this.value);">
                    <option value="page" ${data.cart_icon_link && data.cart_icon_link.startsWith('/page/') ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                    <option value="custom" ${!data.cart_icon_link || !data.cart_icon_link.startsWith('/page/') ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                  </select>
                  ${data.cart_icon_link && data.cart_icon_link.startsWith('/page/') ? `
                    <select class="form-select form-select-sm flex-grow-1" id="prop-cart-icon-link" onchange="PageEditor.updateNavIconLink('cart', this.value);">
                      <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                      ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.cart_icon_link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                    </select>
                  ` : `
                    <input type="text" class="form-control form-control-sm flex-grow-1" id="prop-cart-icon-link" value="${_stripTenantLinkPrefix(data.cart_icon_link || '')}" placeholder="${this.t('pages.pageEditor.props.enterLink', 'Enter link')}" onchange="PageEditor.updateNavIconLink('cart', this.value);">
                  `}
                </div>
              </div>
            ` : ''}
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-fixed" ${data.fixed === true ? 'checked' : ''}>
              <label class="form-check-label" for="prop-fixed">${this.t('pages.pageEditor.props.fixedTop', 'Fixed to top')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.menuPositionLabel', 'Menu position')}</label>
            <select class="form-select" id="prop-menu-position">
              <option value="right" ${(data.menu_position || 'right') === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.props.menuPosition.right', 'Right')}</option>
              <option value="left" ${data.menu_position === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.props.menuPosition.left', 'Left')}</option>
              <option value="center" ${data.menu_position === 'center' ? 'selected' : ''}>${this.t('pages.pageEditor.props.menuPosition.center', 'Center')}</option>
              <option value="bottom" ${data.menu_position === 'bottom' ? 'selected' : ''}>${this.t('pages.pageEditor.props.menuPosition.bottom', 'Bottom')}</option>
            </select>
          </div>
          <hr>
          <div class="mb-3">
            <label class="form-label fw-bold">${this.t('pages.pageEditor.props.topbar', 'Top bar')}</label>
            <div class="form-check mb-2">
              <input class="form-check-input" type="checkbox" id="prop-show-topbar" ${data.show_topbar === true ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-topbar">${this.t('pages.pageEditor.props.showTopbar', 'Show top bar')}</label>
            </div>
            ${data.show_topbar === true ? `
              <div class="ms-3">
                <div class="mb-2">
                  <label class="form-label small">${this.t('pages.pageEditor.props.topbarText', 'Top bar text')}</label>
                  <input type="text" class="form-control form-control-sm" id="prop-topbar-text" value="${data.topbar_text || ''}">
                </div>
                <div class="mb-2">
                  <label class="form-label small">${this.t('pages.pageEditor.props.topbarBgColor', 'Top bar background')}</label>
                  <input type="color" class="form-control form-control-color form-control-sm" id="prop-topbar-bg-color" value="${data.topbar_bg_color || '#f1f1f1'}">
                </div>
                <div class="mb-2">
                  <label class="form-label small">${this.t('pages.pageEditor.props.topbarTextColor', 'Top bar text color')}</label>
                  <input type="color" class="form-control form-control-color form-control-sm" id="prop-topbar-text-color" value="${data.topbar_text_color || '#f97316'}">
                </div>
                <div class="form-check mb-2">
                  <input class="form-check-input" type="checkbox" id="prop-topbar-hide-on-scroll" ${data.topbar_hide_on_scroll !== false ? 'checked' : ''}>
                  <label class="form-check-label small" for="prop-topbar-hide-on-scroll">${this.t('pages.pageEditor.props.topbarHideOnScroll', 'Hide on scroll')}</label>
                </div>
              </div>
            ` : ''}
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-bg-color', this.t('pages.pageEditor.props.backgroundColor', 'Background color'), data.bg_color, '--theme-nav-bg', '#ffffff')}
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.props.padding', 'Padding'), data.padding || '0.75rem 2rem')}
          ${this.renderThemeColorPicker('prop-menu-text-color', this.t('pages.pageEditor.props.menuTextColor', 'Menu text color'), data.menu_text_color, '--theme-nav-text', '#333333')}
          ${this.renderThemeColorPicker('prop-menu-hover-color', this.t('pages.pageEditor.labels.nav.menuHoverColor', 'Menu hover color'), data.menu_hover_color, '--theme-nav-hover', '#0d6efd')}
        `;
        break;
      case 'product-list':
        const isFullList = data.full_list === true;
        contentHtml = `
          <div class="mb-3">
            <div class="form-check mb-3">
              <input class="form-check-input" type="checkbox" id="prop-full-list" ${isFullList ? 'checked' : ''} onchange="PageEditor.toggleProductListFullList();">
              <label class="form-check-label" for="prop-full-list">${this.t('pages.pageEditor.labels.productList.fullList', 'Show full list')}</label>
            </div>
            <div class="mb-3">
              <label class="form-label" for="prop-limit">${isFullList ? this.t('pages.pageEditor.productList.perPageCount', 'Per page') : this.t('pages.pageEditor.productList.displayCount', 'Display count')}</label>
            <input type="number" class="form-control" id="prop-limit" value="${data.limit || 12}" min="1" max="100">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.columns', 'Columns')}</label>
            <select class="form-select" id="prop-columns">
              <option value="2" ${data.columns === 2 ? 'selected' : ''}>2</option>
              <option value="3" ${data.columns === 3 ? 'selected' : ''}>3</option>
              <option value="4" ${data.columns === 4 ? 'selected' : ''}>4</option>
            </select>
          </div>
            ${isFullList ? `
              <div class="mb-3" id="product-type-filter-container">
                <div class="form-check">
                  <input class="form-check-input" type="checkbox" id="prop-show-product-type-filter" ${data.show_product_type_filter === true ? 'checked' : ''}>
                  <label class="form-check-label" for="prop-show-product-type-filter">${this.t('pages.pageEditor.labels.productList.showProductTypeFilter', 'Show product type filter')}</label>
                </div>
              </div>
              <div class="mb-3" id="brand-filter-container">
                <div class="form-check">
                  <input class="form-check-input" type="checkbox" id="prop-show-brand-filter" ${data.show_brand_filter === true ? 'checked' : ''}>
                  <label class="form-check-label" for="prop-show-brand-filter">${this.t('pages.pageEditor.labels.productList.showBrandFilter', 'Show brand filter')}</label>
                </div>
              </div>
            ` : `
              <div class="mb-3" id="product-type-filter-container" style="display: none;">
                <div class="form-check">
                  <input class="form-check-input" type="checkbox" id="prop-show-product-type-filter" ${data.show_product_type_filter === true ? 'checked' : ''}>
                  <label class="form-check-label" for="prop-show-product-type-filter">${this.t('pages.pageEditor.labels.productList.showProductTypeFilter', 'Show product type filter')}</label>
                </div>
              </div>
              <div class="mb-3" id="brand-filter-container" style="display: none;">
                <div class="form-check">
                  <input class="form-check-input" type="checkbox" id="prop-show-brand-filter" ${data.show_brand_filter === true ? 'checked' : ''}>
                  <label class="form-check-label" for="prop-show-brand-filter">${this.t('pages.pageEditor.labels.productList.showBrandFilter', 'Show brand filter')}</label>
                </div>
              </div>
            `}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.productList.productDetailPage', 'Product detail page')}</label>
            <select class="form-select" id="prop-product-detail-page">
              <option value="">${this.t('pages.pageEditor.labels.common.noLink', 'No link')}</option>
              ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.product_detail_page === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
            </select>
          </div>
          ${this.renderProductMetaPanel()}
        `;
        styleHtml = `
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '2rem 0')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.productList.gap', 'Product card gap')}</label>
            <input type="text" class="form-control" id="prop-gap" value="${data.gap || '1rem'}" placeholder="${this.t('pages.pageEditor.placeholders.exampleGap', 'e.g., 1rem')}">
          </div>
        `;
        break;
      case 'service-list':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label" for="prop-service-limit">${this.t('pages.pageEditor.labels.serviceList.displayCount', 'Display count')}</label>
            <input type="number" class="form-control" id="prop-service-limit" value="${data.limit || 12}" min="1" max="100">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.columns', 'Columns')}</label>
            <select class="form-select" id="prop-service-columns">
              <option value="2" ${data.columns === 2 ? 'selected' : ''}>2</option>
              <option value="3" ${data.columns === 3 ? 'selected' : ''}>3</option>
              <option value="4" ${data.columns === 4 ? 'selected' : ''}>4</option>
            </select>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.serviceList.serviceDetailPage', 'Service detail page')}</label>
            <select class="form-select" id="prop-service-detail-page">
              <option value="">${this.t('pages.pageEditor.labels.common.noLink', 'No link')}</option>
              ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.service_detail_page === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
            </select>
          </div>
        `;
        styleHtml = `
          ${this.renderPaddingInput('prop-service-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '2rem 0')}
        `;
        break;
      case 'product-detail':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.productDetail.name', 'Product name')}</label>
            <input type="text" class="form-control" id="prop-product-name" value="${data.product_name || '產品名稱'}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.productDetail.price', 'Product price')}</label>
            <input type="text" class="form-control" id="prop-product-price" value="${data.product_price || '$0.00'}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.productDetail.description', 'Product description')}</label>
            <textarea class="form-control" id="prop-product-description" rows="5">${data.product_description || '產品描述'}</textarea>
          </div>
        `;
        styleHtml = `
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '2rem 0')}
        `;
        break;
      case 'banner-slider':
        const slides = Array.isArray(data.slides) ? data.slides : [];
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.bannerSlider.slides', 'Slides')}</label>
            <div id="banner-slides-list" class="border rounded p-2" style="max-height: 320px; overflow-y: auto;">
              ${slides.length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.labels.bannerSlider.noSlides', 'No slides yet')}</small>` : ''}
              ${slides.map((slide, idx) => `
                <div class="border rounded p-2 mb-2 bg-light" data-slide-index="${idx}">
                  <div class="d-flex align-items-center gap-2 mb-2">
                    ${slide.image ? `<img src="${slide.image}" alt="slide" style="width: 64px; height: 40px; object-fit: cover; border-radius: 4px; border: 1px solid #dee2e6;">` : `<div class="bg-white border rounded d-flex align-items-center justify-content-center" style="width: 64px; height: 40px;"><i class="bi bi-image text-muted"></i></div>`}
                    <div class="flex-grow-1">
                      <input type="file" class="form-control form-control-sm prop-banner-image-file" data-slide-index="${idx}" accept="image/*">
                    </div>
                    <button class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeBannerSlide(${idx});" title="${this.t('pages.pageEditor.actions.delete', 'Delete')}">
                      <i class="bi bi-trash"></i>
                    </button>
                  </div>
                  <div class="mb-2">
                    <label class="form-label small">${this.t('pages.pageEditor.labels.bannerSlider.title', 'Title')}</label>
                    <input type="text" class="form-control form-control-sm" value="${slide.title || ''}" onchange="PageEditor.updateBannerSlide(${idx}, 'title', this.value);">
                  </div>
                  <div class="mb-2">
                    <label class="form-label small">${this.t('pages.pageEditor.labels.bannerSlider.subtitle', 'Subtitle')}</label>
                    <input type="text" class="form-control form-control-sm" value="${slide.subtitle || ''}" onchange="PageEditor.updateBannerSlide(${idx}, 'subtitle', this.value);">
                  </div>
                  <div class="row g-2">
                    <div class="col-6">
                      <label class="form-label small">${this.t('pages.pageEditor.labels.bannerSlider.buttonText', 'Button text')}</label>
                      <input type="text" class="form-control form-control-sm" value="${slide.button_text || ''}" onchange="PageEditor.updateBannerSlide(${idx}, 'button_text', this.value);">
                    </div>
                    <div class="col-6">
                      <label class="form-label small">${this.t('pages.pageEditor.labels.bannerSlider.buttonLink', 'Button link')}</label>
                      <input type="text" class="form-control form-control-sm" value="${_stripTenantLinkPrefix(slide.button_link || '')}" onchange="PageEditor.updateBannerSlide(${idx}, 'button_link', this.value);">
                    </div>
                  </div>
                </div>
              `).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary mt-2" onclick="PageEditor.addBannerSlide();">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.actions.addSlide', 'Add slide')}
            </button>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-banner-autoplay" ${data.autoplay !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-banner-autoplay">${this.t('pages.pageEditor.labels.bannerSlider.autoplay', 'Autoplay')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.bannerSlider.interval', 'Interval (ms)')}</label>
            <input type="number" class="form-control" id="prop-banner-interval" value="${data.interval || 5000}" min="1000" step="500">
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-banner-show-indicators" ${data.show_indicators !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-banner-show-indicators">${this.t('pages.pageEditor.labels.bannerSlider.showIndicators', 'Show indicators')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-banner-show-arrows" ${data.show_arrows !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-banner-show-arrows">${this.t('pages.pageEditor.labels.bannerSlider.showArrows', 'Show arrows')}</label>
            </div>
          </div>
        `;
        styleHtml = `
          ${this.renderSizeInput('prop-banner-height', this.t('pages.pageEditor.labels.bannerSlider.height', 'Height'), data.height || '360px')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.bannerSlider.textAlign', 'Text align')}</label>
            <select class="form-select" id="prop-banner-text-align">
              <option value="left" ${data.text_align === 'left' || !data.text_align ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${data.text_align === 'center' ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${data.text_align === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
            </select>
          </div>
          ${this.renderThemeColorPicker('prop-banner-text-color', this.t('pages.pageEditor.labels.bannerSlider.textColor', 'Text color'), data.text_color, '--theme-hero-text', '#ffffff')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.bannerSlider.overlayColor', 'Overlay color')}</label>
            <input type="text" class="form-control" id="prop-banner-overlay-color" value="${data.overlay_color || 'rgba(0, 0, 0, 0.45)'}">
          </div>
        `;
        break;
      case 'footer':
        // Footer 固定為 4 欄：第一欄是圖片+文字，之後三欄是垂直選單
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.footer.column1LogoImage', 'Column 1 - Logo image')}</label>
            <div class="d-flex align-items-center gap-2 mb-2">
              ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 50px; max-width: 150px; border: 1px solid #dee2e6; border-radius: 4px; padding: 4px;">` : ''}
              <input type="file" class="form-control form-control-sm" id="prop-logo-file" accept="image/*" style="display: none;">
              <button type="button" class="btn btn-sm btn-outline-primary" onclick="document.getElementById('prop-logo-file').click();">${data.logo ? this.t('pages.pageEditor.actions.replace', 'Replace') : this.t('common.upload', 'Upload')} Logo</button>
              ${data.logo ? `<button type="button" class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeFooterLogo();">${this.t('common.remove', 'Remove')}</button>` : ''}
            </div>
            <input type="hidden" id="prop-logo" value="${data.logo || ''}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.footer.column1Content', 'Column 1 - Text content')}</label>
            <textarea class="form-control" id="prop-column1-content" rows="3">${data.column1_content || 'Company intro text'}</textarea>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.footer.column2MenuItems', 'Column 2 - Menu items')}</label>
            <div id="footer-column2-menu-items-list" class="border rounded p-2 mb-2" style="max-height: 200px; overflow-y: auto;">
              ${(data.column2_menu_items || []).length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</small>` : ''}
              ${(data.column2_menu_items || []).map((item, idx) => {
                const isPageLink = item.link && item.link.startsWith('/page/');
                const customLink = isPageLink ? '' : _stripTenantLinkPrefix(item.link || '');
                return `
                <div class="d-flex justify-content-between align-items-center mb-2 p-2 bg-light rounded" data-menu-index="${idx}" data-column="2">
                  <div class="flex-grow-1">
                    <input type="text" class="form-control form-control-sm mb-1" value="${item.text || ''}" placeholder="${this.t('pages.pageEditor.placeholders.menuText', 'Menu text')}" onchange="PageEditor.updateFooterMenuItem(2, ${idx}, 'text', this.value);">
                    <div class="d-flex gap-2">
                      <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" onchange="PageEditor.updateFooterMenuItemLinkType(2, ${idx}, this.value);">
                        <option value="page" ${isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                        <option value="custom" ${!isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                      </select>
                      ${isPageLink ? `
                        <select class="form-select form-select-sm flex-grow-1" onchange="PageEditor.updateFooterMenuItem(2, ${idx}, 'link', this.value);">
                          <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                          ${this.allPages.map(page => `<option value="/page/${page.slug}" ${item.link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                        </select>
                      ` : ''}
                    </div>
                    ${!isPageLink ? `
                      <div class="w-100 mt-2">
                        <input type="text" class="form-control form-control-sm" value="${customLink}" placeholder="${this.t('pages.pageEditor.props.customLinkPlaceholder', 'Enter link (e.g., /about or https://example.com)')}" onchange="PageEditor.updateFooterMenuItem(2, ${idx}, 'link', this.value);">
                      </div>
                    ` : ''}
                  </div>
                  <button class="btn btn-sm btn-outline-danger ms-2" onclick="PageEditor.removeFooterMenuItem(2, ${idx});" title="${this.t('common.remove', 'Remove')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `;
              }).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" onclick="PageEditor.addFooterMenuItem(2);">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.props.addMenuItem', 'Add menu item')}
            </button>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.footer.column3MenuItems', 'Column 3 - Menu items')}</label>
            <div id="footer-column3-menu-items-list" class="border rounded p-2 mb-2" style="max-height: 200px; overflow-y: auto;">
              ${(data.column3_menu_items || []).length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</small>` : ''}
              ${(data.column3_menu_items || []).map((item, idx) => {
                const isPageLink = item.link && item.link.startsWith('/page/');
                const customLink = isPageLink ? '' : _stripTenantLinkPrefix(item.link || '');
                return `
                <div class="d-flex justify-content-between align-items-center mb-2 p-2 bg-light rounded" data-menu-index="${idx}" data-column="3">
                  <div class="flex-grow-1">
                    <input type="text" class="form-control form-control-sm mb-1" value="${item.text || ''}" placeholder="${this.t('pages.pageEditor.placeholders.menuText', 'Menu text')}" onchange="PageEditor.updateFooterMenuItem(3, ${idx}, 'text', this.value);">
                    <div class="d-flex gap-2">
                      <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" onchange="PageEditor.updateFooterMenuItemLinkType(3, ${idx}, this.value);">
                        <option value="page" ${isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                        <option value="custom" ${!isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                      </select>
                      ${isPageLink ? `
                        <select class="form-select form-select-sm flex-grow-1" onchange="PageEditor.updateFooterMenuItem(3, ${idx}, 'link', this.value);">
                          <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                          ${this.allPages.map(page => `<option value="/page/${page.slug}" ${item.link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                        </select>
                      ` : ''}
                    </div>
                    ${!isPageLink ? `
                      <div class="w-100 mt-2">
                        <input type="text" class="form-control form-control-sm" value="${customLink}" placeholder="${this.t('pages.pageEditor.props.customLinkPlaceholder', 'Enter link (e.g., /about or https://example.com)')}" onchange="PageEditor.updateFooterMenuItem(3, ${idx}, 'link', this.value);">
                      </div>
                    ` : ''}
                  </div>
                  <button class="btn btn-sm btn-outline-danger ms-2" onclick="PageEditor.removeFooterMenuItem(3, ${idx});" title="${this.t('common.remove', 'Remove')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `;
              }).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" onclick="PageEditor.addFooterMenuItem(3);">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.props.addMenuItem', 'Add menu item')}
            </button>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.footer.column4MenuItems', 'Column 4 - Menu items')}</label>
            <div id="footer-column4-menu-items-list" class="border rounded p-2 mb-2" style="max-height: 200px; overflow-y: auto;">
              ${(data.column4_menu_items || []).length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</small>` : ''}
              ${(data.column4_menu_items || []).map((item, idx) => {
                const isPageLink = item.link && item.link.startsWith('/page/');
                const customLink = isPageLink ? '' : _stripTenantLinkPrefix(item.link || '');
                return `
                <div class="d-flex justify-content-between align-items-center mb-2 p-2 bg-light rounded" data-menu-index="${idx}" data-column="4">
                  <div class="flex-grow-1">
                    <input type="text" class="form-control form-control-sm mb-1" value="${item.text || ''}" placeholder="${this.t('pages.pageEditor.placeholders.menuText', 'Menu text')}" onchange="PageEditor.updateFooterMenuItem(4, ${idx}, 'text', this.value);">
                    <div class="d-flex gap-2">
                      <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" onchange="PageEditor.updateFooterMenuItemLinkType(4, ${idx}, this.value);">
                        <option value="page" ${isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                        <option value="custom" ${!isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                      </select>
                      ${isPageLink ? `
                        <select class="form-select form-select-sm flex-grow-1" onchange="PageEditor.updateFooterMenuItem(4, ${idx}, 'link', this.value);">
                          <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                          ${this.allPages.map(page => `<option value="/page/${page.slug}" ${item.link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                        </select>
                      ` : ''}
                    </div>
                    ${!isPageLink ? `
                      <div class="w-100 mt-2">
                        <input type="text" class="form-control form-control-sm" value="${customLink}" placeholder="${this.t('pages.pageEditor.props.customLinkPlaceholder', 'Enter link (e.g., /about or https://example.com)')}" onchange="PageEditor.updateFooterMenuItem(4, ${idx}, 'link', this.value);">
                      </div>
                    ` : ''}
                  </div>
                  <button class="btn btn-sm btn-outline-danger ms-2" onclick="PageEditor.removeFooterMenuItem(4, ${idx});" title="${this.t('common.remove', 'Remove')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `;
              }).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" onclick="PageEditor.addFooterMenuItem(4);">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.props.addMenuItem', 'Add menu item')}
            </button>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.footer.copyright', 'Copyright text')}</label>
            <input type="text" class="form-control" id="prop-copyright" value="${data.copyright || '© 2025 All rights reserved'}">
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-bg-color', this.t('pages.pageEditor.labels.common.backgroundColor', 'Background color'), data.bg_color, '--theme-footer-bg', '#f8f9fa')}
          ${this.renderThemeColorPicker('prop-text-color', this.t('pages.pageEditor.labels.common.textColor', 'Text color'), data.text_color, '--theme-footer-text', '#6c757d')}
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '2rem 0')}
        `;
        break;
      case 'order-list':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.orderList.perPage', 'Items per page')}</label>
            <input type="number" class="form-control" id="prop-limit" value="${data.limit || 10}" min="1" max="100">
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-status-filter" ${data.show_status_filter !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-status-filter">${this.t('pages.pageEditor.labels.orderList.showStatusFilter', 'Show status filter')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-date-filter" ${data.show_date_filter !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-date-filter">${this.t('pages.pageEditor.labels.orderList.showDateFilter', 'Show date filter')}</label>
            </div>
          </div>
        `;
        styleHtml = `<p class="text-muted">${this.t('pages.pageEditor.messages.noStyleSettings', 'No style settings for this component.')}</p>`;
        break;
      case 'blog-list':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.blogList.displayCount', 'Display count')}</label>
            <input type="number" class="form-control" id="prop-limit" value="${data.limit || 10}" min="1" max="100">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.columns', 'Columns')}</label>
            <select class="form-select" id="prop-columns">
              <option value="1" ${(data.columns || 1) == 1 ? 'selected' : ''}>1</option>
              <option value="2" ${(data.columns || 1) == 2 ? 'selected' : ''}>2</option>
              <option value="3" ${(data.columns || 1) == 3 ? 'selected' : ''}>3</option>
              <option value="4" ${(data.columns || 1) == 4 ? 'selected' : ''}>4</option>
            </select>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-image" ${data.show_image !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-image">${this.t('pages.pageEditor.labels.blogList.showImage', 'Show image')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-excerpt" ${data.show_excerpt !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-excerpt">${this.t('pages.pageEditor.labels.blogList.showExcerpt', 'Show excerpt')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.blogList.categoryFilter', 'Category filter (optional)')}</label>
            <input type="text" class="form-control" id="prop-category" value="${data.category || ''}" placeholder="${this.t('pages.pageEditor.labels.blogList.categoryPlaceholder', 'Leave empty to show all categories')}">
          </div>
        `;
        styleHtml = `<p class="text-muted">${this.t('pages.pageEditor.messages.noStyleSettings', 'No style settings for this component.')}</p>`;
        break;
      case 'contact-form':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-name" ${data.show_name !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-name">${this.t('pages.pageEditor.props.contact.showName', 'Show name')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-email" ${data.show_email !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-email">${this.t('pages.pageEditor.props.contact.showEmail', 'Show email')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-phone" ${data.show_phone !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-phone">${this.t('pages.pageEditor.props.contact.showPhone', 'Show phone')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-message" ${data.show_message !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-message">${this.t('pages.pageEditor.props.contact.showMessage', 'Show message')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.submitButtonText', 'Submit button text')}</label>
            <input type="text" class="form-control" id="prop-submit-button-text" value="${data.submit_button_text || this.t('pages.pageEditor.preview.contact.submit', 'Submit')}">
          </div>
        `;
        styleHtml = `<p class="text-muted">${this.t('pages.pageEditor.messages.noStyleSettings', 'No style settings for this component.')}</p>`;
        break;
      case 'service-booking':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.title', 'Title')}</label>
            <input type="text" class="form-control" id="prop-title" value="${data.title || this.t('pages.pageEditor.defaults.serviceBooking.title', 'Book a Service')}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.subtitle', 'Subtitle')}</label>
            <input type="text" class="form-control" id="prop-subtitle" value="${data.subtitle || this.t('pages.pageEditor.defaults.serviceBooking.subtitle', 'Choose a service and book your appointment')}">
          </div>
          <hr>
          <h6 class="text-muted mb-3">${this.t('pages.pageEditor.props.serviceBooking.stepLabels', 'Step Labels')}</h6>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.step1Title', 'Step 1 Title')}</label>
            <input type="text" class="form-control" id="prop-step1-title" value="${data.step1_title || this.t('pages.pageEditor.defaults.serviceBooking.step1', 'Select Service')}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.step2Title', 'Step 2 Title')}</label>
            <input type="text" class="form-control" id="prop-step2-title" value="${data.step2_title || this.t('pages.pageEditor.defaults.serviceBooking.step2', 'Choose Date & Time')}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.step3Title', 'Step 3 Title')}</label>
            <input type="text" class="form-control" id="prop-step3-title" value="${data.step3_title || this.t('pages.pageEditor.defaults.serviceBooking.step3', 'Confirm Booking')}">
          </div>
          <hr>
          <h6 class="text-muted mb-3">${this.t('pages.pageEditor.props.serviceBooking.displayOptions', 'Display Options')}</h6>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-service-select" ${data.show_service_select !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-service-select">${this.t('pages.pageEditor.props.serviceBooking.showServiceSelect', 'Show service selection')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-staff-select" ${data.show_staff_select !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-staff-select">${this.t('pages.pageEditor.props.serviceBooking.showStaffSelect', 'Show staff selection')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-date-picker" ${data.show_date_picker !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-date-picker">${this.t('pages.pageEditor.props.serviceBooking.showDatePicker', 'Show date picker')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-time-slots" ${data.show_time_slots !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-time-slots">${this.t('pages.pageEditor.props.serviceBooking.showTimeSlots', 'Show time slots')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-require-login" ${data.require_login ? 'checked' : ''}>
              <label class="form-check-label" for="prop-require-login">${this.t('pages.pageEditor.props.serviceBooking.requireLogin', 'Require login to book')}</label>
            </div>
          </div>
          <hr>
          <h6 class="text-muted mb-3">${this.t('pages.pageEditor.props.serviceBooking.messages', 'Messages')}</h6>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.successMessage', 'Success message')}</label>
            <textarea class="form-control" id="prop-success-message" rows="2">${data.success_message || this.t('pages.pageEditor.defaults.serviceBooking.successMessage', 'Your booking has been confirmed! We will contact you shortly.')}</textarea>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.serviceBooking.buttonText', 'Confirm button text')}</label>
            <input type="text" class="form-control" id="prop-button-text" value="${data.button_text || this.t('pages.pageEditor.defaults.serviceBooking.buttonText', 'Confirm Booking')}">
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-primary-color', this.t('pages.pageEditor.props.serviceBooking.primaryColor', 'Primary color'), data.primary_color, '--theme-primary', '#0d6efd')}
        `;
        break;
      case 'dining-menu':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-dining-menu-show-title" ${data.show_title !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-dining-menu-show-title">${this.t('pages.pageEditor.labels.diningMenu.showTitle', 'Show title')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.diningMenu.title', 'Title')}</label>
            <input type="text" class="form-control" id="prop-dining-menu-title" value="${data.title || this.t('pages.pageEditor.defaults.diningMenu.title', '菜單')}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.diningMenu.subtitle', 'Subtitle')}</label>
            <input type="text" class="form-control" id="prop-dining-menu-subtitle" value="${data.subtitle || this.t('pages.pageEditor.defaults.diningMenu.subtitle', '瀏覽餐點與價格')}">
          </div>
        `;
        styleHtml = `
          ${this.renderSizeInput('prop-dining-menu-height', this.t('pages.pageEditor.labels.diningMenu.height', 'Height'), data.height || '720px')}
        `;
        break;
      case 'dining-table-reservation':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-dining-reservation-show-title" ${data.show_title !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-dining-reservation-show-title">${this.t('pages.pageEditor.labels.diningReservation.showTitle', 'Show title')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.diningReservation.title', 'Title')}</label>
            <input type="text" class="form-control" id="prop-dining-reservation-title" value="${data.title || this.t('pages.pageEditor.defaults.diningReservation.title', '預約餐桌')}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.diningReservation.subtitle', 'Subtitle')}</label>
            <input type="text" class="form-control" id="prop-dining-reservation-subtitle" value="${data.subtitle || this.t('pages.pageEditor.defaults.diningReservation.subtitle', '填寫聯絡資訊完成預約')}">
          </div>
        `;
        styleHtml = `
          ${this.renderSizeInput('prop-dining-reservation-height', this.t('pages.pageEditor.labels.diningReservation.height', 'Height'), data.height || '720px')}
        `;
        break;
      case 'login-register':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-login" ${data.show_login !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-login">${this.t('pages.pageEditor.props.showLogin', 'Show login')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-register" ${data.show_register !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-register">${this.t('pages.pageEditor.props.showRegister', 'Show register')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.loginMethod', 'Login method')}</label>
            <select class="form-select" id="prop-login-method">
              <option value="phone_or_email" ${data.login_method === 'phone_or_email' || !data.login_method ? 'selected' : ''}>${this.t('pages.pageEditor.props.loginMethodOptions.phoneOrEmail', 'Phone or email')}</option>
              <option value="email_only" ${data.login_method === 'email_only' ? 'selected' : ''}>${this.t('pages.pageEditor.props.loginMethodOptions.emailOnly', 'Email only')}</option>
              <option value="phone_only" ${data.login_method === 'phone_only' ? 'selected' : ''}>${this.t('pages.pageEditor.props.loginMethodOptions.phoneOnly', 'Phone only')}</option>
            </select>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.redirectAfterLogin', 'Redirect after login')}</label>
            <input type="text" class="form-control" id="prop-redirect-after-login" value="${_stripTenantLinkPrefix(data.redirect_after_login || '/')}" placeholder="/">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.redirectAfterRegister', 'Redirect after register')}</label>
            <input type="text" class="form-control" id="prop-redirect-after-register" value="${_stripTenantLinkPrefix(data.redirect_after_register || '/')}" placeholder="/">
          </div>
        `;
        styleHtml = `<p class="text-muted">${this.t('pages.pageEditor.messages.noStyleSettings', 'No style settings for this component.')}</p>`;
        break;
      case 'cart':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-checkout-button" ${data.show_checkout_button !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-checkout-button">${this.t('pages.pageEditor.labels.cart.showCheckoutButton', 'Show checkout button')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-continue-shopping" ${data.show_continue_shopping !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-continue-shopping">${this.t('pages.pageEditor.labels.cart.showContinueShopping', 'Show continue shopping button')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-coupon" ${data.show_coupon !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-coupon">${this.t('pages.pageEditor.labels.cart.showCoupon', 'Show coupon input')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-points" ${data.show_points !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-points">${this.t('pages.pageEditor.labels.cart.showPoints', 'Show points')}</label>
            </div>
          </div>
        `;
        styleHtml = null; // 沒有樣式設置
        break;
      case 'checkout':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-shipping-form" ${data.show_shipping_form !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-shipping-form">${this.t('pages.pageEditor.labels.checkout.showShippingForm', 'Show shipping form')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-payment-form" ${data.show_payment_form !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-payment-form">${this.t('pages.pageEditor.labels.checkout.showPaymentForm', 'Show payment form')}</label>
            </div>
          </div>
        `;
        styleHtml = null; // 沒有樣式設置
        break;
      case 'user-area':
        contentHtml = `
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-profile" ${data.show_profile !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-profile">${this.t('pages.pageEditor.props.userArea.showProfile', 'Show profile')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-orders" ${data.show_orders !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-orders">${this.t('pages.pageEditor.props.userArea.showOrders', 'Show orders')}</label>
            </div>
          </div>
          <div class="mb-3">
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-addresses" ${data.show_addresses !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-addresses">${this.t('pages.pageEditor.props.userArea.showAddresses', 'Show addresses')}</label>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.userArea.loginPage', 'Login page')}</label>
            <select class="form-select" id="prop-user-area-login-page">
              <option value="">${this.t('pages.pageEditor.labels.common.noLink', 'No link')}</option>
              ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.login_page === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
            </select>
          </div>
        `;
        styleHtml = `<p class="text-muted">${this.t('pages.pageEditor.messages.noStyleSettings', 'No style settings for this component.')}</p>`;
        break;
      case 'list':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.nav.menuItems', 'Menu items')}</label>
            <div id="list-menu-items-list" class="border rounded p-2 mb-2" style="max-height: 200px; overflow-y: auto;">
              ${(data.menu_items || []).length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</small>` : ''}
              ${(data.menu_items || []).map((item, idx) => {
                const isPageLink = item.link && item.link.startsWith('/page/');
                const customLink = isPageLink ? '' : _stripTenantLinkPrefix(item.link || '');
                return `
                <div class="d-flex justify-content-between align-items-center mb-2 p-2 bg-light rounded" data-menu-index="${idx}">
                  <div class="flex-grow-1">
                    <input type="text" class="form-control form-control-sm mb-1" value="${item.text || ''}" placeholder="${this.t('pages.pageEditor.placeholders.menuText', 'Menu text')}" onchange="PageEditor.updateListMenuItem(${idx}, 'text', this.value);">
                    <div class="d-flex gap-2">
                      <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" onchange="PageEditor.updateListMenuItemLinkType(${idx}, this.value);">
                        <option value="page" ${isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                        <option value="custom" ${!isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                      </select>
                      ${isPageLink ? `
                        <select class="form-select form-select-sm flex-grow-1" onchange="PageEditor.updateListMenuItem(${idx}, 'link', this.value);">
                          <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                          ${this.allPages.map(page => `<option value="/page/${page.slug}" ${item.link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                        </select>
                      ` : ''}
                    </div>
                    ${!isPageLink ? `
                      <div class="w-100 mt-2">
                        <input type="text" class="form-control form-control-sm" value="${customLink}" placeholder="${this.t('pages.pageEditor.props.customLinkPlaceholder', 'Enter link (e.g., /about or https://example.com)')}" onchange="PageEditor.updateListMenuItem(${idx}, 'link', this.value);">
                      </div>
                    ` : ''}
                  </div>
                  <button class="btn btn-sm btn-outline-danger ms-2" onclick="PageEditor.removeListMenuItem(${idx});" title="${this.t('common.remove', 'Remove')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `;
              }).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" onclick="PageEditor.addListMenuItem();">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.props.addMenuItem', 'Add menu item')}
            </button>
          </div>
        `;
        styleHtml = null; // 沒有樣式設置
        break;
      case 'image':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.image.image', 'Image')}</label>
            <div class="d-flex align-items-center gap-2 mb-2">
              ${data.src ? `<img src="${data.src}" alt="Image" style="max-height: 100px; max-width: 200px; border: 1px solid #dee2e6; border-radius: 4px; padding: 4px;">` : ''}
              <input type="file" class="form-control form-control-sm" id="prop-image-file" accept="image/*" style="display: none;">
              <button type="button" class="btn btn-sm btn-outline-primary" onclick="document.getElementById('prop-image-file').click();">${data.src ? this.t('pages.pageEditor.actions.replace', 'Replace') : this.t('common.upload', 'Upload')} ${this.t('pages.pageEditor.labels.image.image', 'Image')}</button>
              ${data.src ? `<button type="button" class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeImage();">${this.t('common.remove', 'Remove')}</button>` : ''}
            </div>
            <input type="hidden" id="prop-src" value="${data.src || ''}">
            <div class="mb-3">
              <label class="form-label">${this.t('pages.pageEditor.labels.image.alt', 'Alt text')}</label>
              <input type="text" class="form-control" id="prop-alt" value="${data.alt || 'Image'}">
            </div>
          </div>
        `;
        styleHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.image.width', 'Width')}</label>
            <input type="text" class="form-control" id="prop-width" value="${data.width || '100%'}" placeholder="${this.t('pages.pageEditor.placeholders.exampleWidth', 'e.g., 100%')}">
          </div>
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '2rem')}
        `;
        break;
      case 'button':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.button.text', 'Button text')}</label>
            <input type="text" class="form-control" id="prop-text" value="${data.text || 'Button'}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.button.link', 'Link')}</label>
            <input type="text" class="form-control" id="prop-link" value="${_stripTenantLinkPrefix(data.link || '#')}">
          </div>
        `;
        styleHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.button.style', 'Button style')}</label>
            <select class="form-select" id="prop-style">
              <option value="primary" ${data.style === 'primary' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.primary', 'Primary')}</option>
              <option value="secondary" ${data.style === 'secondary' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.secondary', 'Secondary')}</option>
              <option value="success" ${data.style === 'success' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.success', 'Success')}</option>
              <option value="danger" ${data.style === 'danger' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.danger', 'Danger')}</option>
              <option value="warning" ${data.style === 'warning' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.warning', 'Warning')}</option>
              <option value="info" ${data.style === 'info' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.info', 'Info')}</option>
              <option value="light" ${data.style === 'light' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.light', 'Light')}</option>
              <option value="dark" ${data.style === 'dark' ? 'selected' : ''}>${this.t('pages.pageEditor.buttonStyles.dark', 'Dark')}</option>
            </select>
          </div>
          ${this.renderThemeColorPicker('prop-button-color', this.t('pages.pageEditor.labels.button.customColor', 'Button color (custom)'), data.button_color, '--theme-btn-primary-bg', '#0d6efd')}
          ${this.renderThemeColorPicker('prop-button-text-color', this.t('pages.pageEditor.labels.button.textColor', 'Button text color'), data.button_text_color, '--theme-btn-primary-text', '#ffffff')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.button.size', 'Button size')}</label>
            <select class="form-select" id="prop-size">
              <option value="sm" ${data.size === 'sm' ? 'selected' : ''}>${this.t('pages.pageEditor.sizes.sm', 'Small')}</option>
              <option value="" ${!data.size || data.size === '' ? 'selected' : ''}>${this.t('pages.pageEditor.sizes.md', 'Normal')}</option>
              <option value="lg" ${data.size === 'lg' ? 'selected' : ''}>${this.t('pages.pageEditor.sizes.lg', 'Large')}</option>
            </select>
          </div>
          ${this.renderPaddingInput('prop-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), data.padding || '1.5rem')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.button.align', 'Button align')}</label>
            <select class="form-select" id="prop-align">
              <option value="left" ${data.align === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${data.align === 'center' || !data.align ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${data.align === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
            </select>
          </div>
        `;
        break;
      case 'header':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.nav.logoImage', 'Logo image')}</label>
            <div class="d-flex align-items-center gap-2 mb-2">
              ${data.logo ? `<img src="${data.logo}" alt="Logo" style="max-height: 50px; max-width: 150px; border: 1px solid #dee2e6; border-radius: 4px; padding: 4px;">` : ''}
              <input type="file" class="form-control form-control-sm" id="prop-logo-file" accept="image/*" style="display: none;">
              <button type="button" class="btn btn-sm btn-outline-primary" onclick="document.getElementById('prop-logo-file').click();">${data.logo ? this.t('pages.pageEditor.actions.replace', 'Replace') : this.t('common.upload', 'Upload')} Logo</button>
              ${data.logo ? `<button type="button" class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeHeaderLogo();">${this.t('common.remove', 'Remove')}</button>` : ''}
            </div>
            <input type="hidden" id="prop-logo" value="${data.logo || ''}">
            <div class="mb-3">
              <label class="form-label">${this.t('pages.pageEditor.labels.nav.logoText', 'Logo text (shown when no image)')}</label>
              <input type="text" class="form-control" id="prop-logo-text" value="${data.logo_text || 'Logo'}">
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.nav.menuItems', 'Menu items')}</label>
            <div id="header-menu-items-list" class="border rounded p-2 mb-2" style="max-height: 200px; overflow-y: auto;">
              ${(data.menu_items || []).length === 0 ? `<small class="text-muted">${this.t('pages.pageEditor.preview.list.empty', 'No menu items')}</small>` : ''}
              ${(data.menu_items || []).map((item, idx) => {
                const isPageLink = item.link && item.link.startsWith('/page/');
                const customLink = isPageLink ? '' : _stripTenantLinkPrefix(item.link || '');
                return `
                <div class="d-flex justify-content-between align-items-center mb-2 p-2 bg-light rounded" data-menu-index="${idx}">
                  <div class="flex-grow-1">
                    <input type="text" class="form-control form-control-sm mb-1" value="${item.text || ''}" placeholder="${this.t('pages.pageEditor.placeholders.menuText', 'Menu text')}" onchange="PageEditor.updateMenuItem(${idx}, 'text', this.value);">
                    <div class="d-flex gap-2">
                      <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" onchange="PageEditor.updateMenuItemLinkType(${idx}, this.value);">
                        <option value="page" ${isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.page', 'Page')}</option>
                        <option value="custom" ${!isPageLink ? 'selected' : ''}>${this.t('pages.pageEditor.props.linkType.custom', 'Custom')}</option>
                      </select>
                      ${isPageLink ? `
                        <select class="form-select form-select-sm flex-grow-1" onchange="PageEditor.updateMenuItem(${idx}, 'link', this.value);">
                          <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                          ${this.allPages.map(page => `<option value="/page/${page.slug}" ${item.link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                        </select>
                      ` : ''}
                    </div>
                    ${!isPageLink ? `
                      <div class="w-100 mt-2">
                        <input type="text" class="form-control form-control-sm" value="${customLink}" placeholder="${this.t('pages.pageEditor.props.customLinkPlaceholder', 'Enter link (e.g., /about or https://example.com)')}" onchange="PageEditor.updateMenuItem(${idx}, 'link', this.value);">
                      </div>
                    ` : ''}
                  </div>
                  <button class="btn btn-sm btn-outline-danger ms-2" onclick="PageEditor.removeMenuItem(${idx});" title="${this.t('common.remove', 'Remove')}">
                    <i class="bi bi-x"></i>
                  </button>
                </div>
              `;
              }).join('')}
            </div>
            <button type="button" class="btn btn-sm btn-outline-primary" onclick="PageEditor.addMenuItem();">
              <i class="bi bi-plus"></i> ${this.t('pages.pageEditor.props.addMenuItem', 'Add menu item')}
            </button>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.props.showIcons', 'Show icons')}</label>
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-login-icon" ${data.show_login_icon !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-login-icon">${this.t('pages.pageEditor.props.loginIcon', 'Login icon')}</label>
            </div>
            ${data.show_login_icon !== false ? `
              <div class="mb-2 ms-4">
                <label class="form-label small">${this.t('pages.pageEditor.props.loginIconLink', 'Login icon link')}</label>
                <div class="d-flex gap-2">
                  <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" id="prop-header-login-icon-link-type" onchange="PageEditor.updateHeaderIconLinkType('login', this.value);">
                    <option value="page" ${data.login_icon_link && data.login_icon_link.startsWith('/page/') ? 'selected' : ''}>選擇頁面</option>
                    <option value="custom" ${!data.login_icon_link || !data.login_icon_link.startsWith('/page/') ? 'selected' : ''}>自訂連結</option>
                  </select>
                  ${data.login_icon_link && data.login_icon_link.startsWith('/page/') ? `
                    <select class="form-select form-select-sm flex-grow-1" id="prop-header-login-icon-link" onchange="PageEditor.updateHeaderIconLink('login', this.value);">
                      <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                      ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.login_icon_link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                    </select>
                  ` : `
                    <input type="text" class="form-control form-control-sm flex-grow-1" id="prop-header-login-icon-link" value="${_stripTenantLinkPrefix(data.login_icon_link || '')}" placeholder="${this.t('pages.pageEditor.props.enterLink', 'Enter link')}" onchange="PageEditor.updateHeaderIconLink('login', this.value);">
                  `}
                </div>
              </div>
            ` : ''}
            <div class="form-check">
              <input class="form-check-input" type="checkbox" id="prop-show-cart-icon" ${data.show_cart_icon !== false ? 'checked' : ''}>
              <label class="form-check-label" for="prop-show-cart-icon">${this.t('pages.pageEditor.props.cartIcon', 'Cart icon')}</label>
            </div>
            ${data.show_cart_icon !== false ? `
              <div class="mb-2 ms-4">
                <label class="form-label small">${this.t('pages.pageEditor.props.cartIconLink', 'Cart icon link')}</label>
                <div class="d-flex gap-2">
                  <select class="form-select form-select-sm flex-shrink-0" style="width: 150px;" id="prop-header-cart-icon-link-type" onchange="PageEditor.updateHeaderIconLinkType('cart', this.value);">
                    <option value="page" ${data.cart_icon_link && data.cart_icon_link.startsWith('/page/') ? 'selected' : ''}>選擇頁面</option>
                    <option value="custom" ${!data.cart_icon_link || !data.cart_icon_link.startsWith('/page/') ? 'selected' : ''}>自訂連結</option>
                  </select>
                  ${data.cart_icon_link && data.cart_icon_link.startsWith('/page/') ? `
                    <select class="form-select form-select-sm flex-grow-1" id="prop-header-cart-icon-link" onchange="PageEditor.updateHeaderIconLink('cart', this.value);">
                      <option value="">${this.t('pages.pageEditor.props.selectPage', 'Select page...')}</option>
                      ${this.allPages.map(page => `<option value="/page/${page.slug}" ${data.cart_icon_link === `/page/${page.slug}` ? 'selected' : ''}>${page.name}</option>`).join('')}
                    </select>
                  ` : `
                    <input type="text" class="form-control form-control-sm flex-grow-1" id="prop-header-cart-icon-link" value="${_stripTenantLinkPrefix(data.cart_icon_link || '')}" placeholder="${this.t('pages.pageEditor.props.enterLink', 'Enter link')}" onchange="PageEditor.updateHeaderIconLink('cart', this.value);">
                  `}
                </div>
              </div>
            ` : ''}
          </div>
        `;
        styleHtml = `
          ${this.renderThemeColorPicker('prop-bg-color', this.t('pages.pageEditor.labels.common.backgroundColor', 'Background color'), data.bg_color, '--theme-nav-bg', '#ffffff')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.padding', 'Padding')}</label>
            <input type="text" class="form-control" id="prop-padding" value="${data.padding || '1rem 2rem'}" placeholder="${this.t('pages.pageEditor.placeholders.examplePadding', 'e.g., 1rem 2rem')}">
          </div>
          ${this.renderThemeColorPicker('prop-menu-text-color', this.t('pages.pageEditor.props.menuTextColor', 'Menu text color'), data.menu_text_color, '--theme-nav-text', '#333333')}
        `;
        break;
      case 'google-map':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.googleMap.address', 'Address')}</label>
            <div class="input-group">
              <input type="text" class="form-control" id="prop-gmap-address" value="${data.address || ''}" placeholder="${this.t('pages.pageEditor.placeholders.googleMapAddress', 'Enter address to locate on map')}">
              <button type="button" class="btn btn-outline-primary btn-sm" onclick="PageEditor.geocodeMapAddress();">
                <i class="bi bi-search"></i>
              </button>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.googleMap.lat', 'Latitude')}</label>
            <input type="number" class="form-control" id="prop-gmap-lat" value="${data.lat || 25.033964}" step="0.000001">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.googleMap.lng', 'Longitude')}</label>
            <input type="number" class="form-control" id="prop-gmap-lng" value="${data.lng || 121.564468}" step="0.000001">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.googleMap.zoom', 'Zoom')}</label>
            <input type="range" class="form-range" id="prop-gmap-zoom" min="1" max="20" value="${data.zoom || 14}">
            <small class="text-muted">${data.zoom || 14}</small>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.googleMap.markerTitle', 'Marker title')}</label>
            <input type="text" class="form-control" id="prop-gmap-marker-title" value="${data.marker_title || ''}">
          </div>
        `;
        styleHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.googleMap.height', 'Map height')}</label>
            <input type="text" class="form-control" id="prop-gmap-height" value="${data.height || '400px'}" placeholder="e.g., 400px">
          </div>
        `;
        break;
      case 'custom-html':
        contentHtml = `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.customHtml.htmlContent', 'HTML Content')}</label>
            <textarea class="form-control font-monospace" id="prop-html-content" rows="12" style="font-size: 0.85rem;">${(data.html_content || '').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</textarea>
            <small class="text-muted">${this.t('pages.pageEditor.labels.customHtml.hint', 'Supports HTML, inline CSS, and script tags.')}</small>
          </div>
        `;
        styleHtml = null;
        break;
      default:
        contentHtml = `<p class="text-muted">${this.t('pages.pageEditor.messages.noEditableProps', 'This component type has no editable properties yet.')}</p>`;
        styleHtml = null; // 沒有樣式設置
    }
    
    // 檢查是否有樣式設置（只要有內容就顯示 Style tab；即使只是提示文字也顯示在 Style tab 內）
    const hasStyleSettings = styleHtml !== null && styleHtml !== '';
    
    // Tab 導航（如果有樣式設置才顯示 tab）
    if (hasStyleSettings) {
      html += `
        <ul class="nav nav-tabs mb-3" role="tablist">
          <li class="nav-item" role="presentation">
            <button class="nav-link active" id="content-tab" data-bs-toggle="tab" data-bs-target="#content-panel" type="button" role="tab" aria-controls="content-panel" aria-selected="true">${this.t('pages.pageEditor.tabs.content', 'Content')}</button>
          </li>
          <li class="nav-item" role="presentation">
            <button class="nav-link" id="style-tab" data-bs-toggle="tab" data-bs-target="#style-panel" type="button" role="tab" aria-controls="style-panel" aria-selected="false">${this.t('pages.pageEditor.tabs.style', 'Style')}</button>
          </li>
        </ul>
        <div class="tab-content">
          <div class="tab-pane fade show active" id="content-panel" role="tabpanel" aria-labelledby="content-tab">
      `;
    }
    
    // 將內容和樣式放入對應的 tab
    html += contentHtml;
    if (hasStyleSettings) {
    html += `</div><div class="tab-pane fade" id="style-panel" role="tabpanel" aria-labelledby="style-tab">`;
    html += styleHtml;
    html += `</div></div>`;
    }
    
    panel.innerHTML = html;
    
    
    // 如果有表單元素，添加 has-form 類
    const hasForm = panel.querySelectorAll('input, select, textarea').length > 0;
    if (hasForm) {
      panel.classList.add('has-form');
    } else {
      panel.classList.remove('has-form');
    }
    
    // 綁定屬性變更事件
    // 使用事件委託，避免重新綁定時丟失焦點
    // 對於 textarea 和 input[type="text"]，使用 input 事件實時更新預覽
    // 對於 select 和 input[type="color"]，使用 change 事件
    panel.addEventListener('input', (e) => {
      // element 專屬屬性面板（文字/按鈕/圖片）內的輸入都有 data-element-id
      // 這些輸入會由 bindElementPropertiesEvents() 自己處理，不能再觸發 updateComponentData()
      // 否則會在元素樣式調整時重渲染整個元件，導致文字回退為預設值（例如 hero:「歡迎」變「歡迎標題」）
      if (e.target && e.target.hasAttribute && e.target.hasAttribute('data-element-id')) return;
      if (e.target.matches('textarea, input[type="text"]')) {
        this.updateComponentData();
      }
    }, true);
    
    panel.addEventListener('change', (e) => {
      if (e.target && e.target.hasAttribute && e.target.hasAttribute('data-element-id')) return;
      if (e.target.matches('select, input[type="color"], input[type="number"], input[type="checkbox"]')) {
        // For theme color pickers: mark as custom BEFORE updateComponentData reads the value,
        // so getSaveColor sees is-theme='0' and persists the chosen color correctly.
        if (e.target.matches('input[type="color"].theme-color-picker')) {
          const inputId = e.target.id;
          const cssVar = e.target.dataset.cssVar || '';
          const fallback = e.target.dataset.fallback || '';
          this.onColorPickerChange(inputId, cssVar, fallback);
        }
        this.updateComponentData();
        // select 和 color 改變時立即保存歷史記錄
        if (this.updateComponentDataTimeout) {
          clearTimeout(this.updateComponentDataTimeout);
        }
        this.saveState();
      }
      // 處理 logo 文件上傳（nav, header, footer）
      if (e.target.matches('input[type="file"]#prop-logo-file')) {
        if (this.selectedComponent && this.selectedComponent.component_type === 'footer') {
          this.handleFooterLogoUpload(e.target.files[0]);
        } else {
        this.handleLogoUpload(e.target.files[0]);
        }
      }
      // 處理 hero 背景圖片上傳
      if (e.target.matches('input[type="file"]#prop-bg-image-file')) {
        this.handleHeroBgImageUpload(e.target.files[0]);
      }
      // 處理圖片上傳
      if (e.target.matches('input[type="file"]#prop-image-file')) {
        this.handleImageUpload(e.target.files[0]);
      }
      // 處理 banner slider 圖片上傳
      if (e.target.matches('input[type="file"].prop-banner-image-file')) {
        const slideIndex = parseInt(e.target.dataset.slideIndex || '-1');
        if (!isNaN(slideIndex) && slideIndex >= 0) {
          this.handleBannerSlideImageUpload(slideIndex, e.target.files[0]);
        }
      }
    }, true);
  },

  detectTextAndButtons(component) {
    const texts = [];
    const buttons = [];
    
    // 獲取元件的預覽 DOM
    const index = this.components.indexOf(component);
    const item = document.querySelector(`.component-item[data-index="${index}"]`);
    if (!item) return { texts, buttons };
    
    const preview = item.querySelector('.component-preview');
    if (!preview) return { texts, buttons };
    
    // 檢測所有文字元素 (h1-h6, p, span, div 等包含文字的元素)
    const textElements = preview.querySelectorAll('h1, h2, h3, h4, h5, h6, p, span, div, a:not(.btn), li, td, th');
    textElements.forEach(el => {
      const text = el.textContent?.trim();
      if (text && text.length > 0 && !el.closest('.btn')) {
        const computedStyle = window.getComputedStyle(el);
        texts.push({
          text: text.substring(0, 50),
          element: el,
          color: computedStyle.color,
          fontSize: computedStyle.fontSize,
          fontWeight: computedStyle.fontWeight
        });
      }
    });
    
    // 檢測所有按鈕元素
    const buttonElements = preview.querySelectorAll('.btn, button, a.btn, input[type="button"], input[type="submit"]');
    buttonElements.forEach(el => {
      const text = el.textContent?.trim() || el.value || '';
      if (text.length > 0) {
        const computedStyle = window.getComputedStyle(el);
        buttons.push({
          text: text.substring(0, 50),
          element: el,
          bgColor: computedStyle.backgroundColor,
          textColor: computedStyle.color,
          size: el.classList.contains('btn-sm') ? 'sm' : el.classList.contains('btn-lg') ? 'lg' : ''
        });
      }
    });
    
    return { texts, buttons };
  },

  updateComponentData() {
    if (!this.selectedComponent) return;
    
    const type = this.selectedComponent.component_type;
    const data = {};
    
    
    // 保存檢測到的文字和按鈕樣式
    const detectedElements = this.detectTextAndButtons(this.selectedComponent);
    if (detectedElements.texts.length > 0) {
      data.detected_texts = detectedElements.texts.map((textEl, idx) => {
        const textId = `detected-text-${idx}`;
        const colorEl = document.getElementById(`${textId}-color`);
        const fontSizeEl = document.getElementById(`${textId}-font-size`);
        const fontWeightEl = document.getElementById(`${textId}-font-weight`);
        return {
          color: colorEl?.value || textEl.color,
          font_size: fontSizeEl?.value || textEl.fontSize,
          font_weight: fontWeightEl?.value || textEl.fontWeight
        };
      });
    }
    
    if (detectedElements.buttons.length > 0) {
      data.detected_buttons = detectedElements.buttons.map((btnEl, idx) => {
        const btnId = `detected-button-${idx}`;
        const bgColorEl = document.getElementById(`${btnId}-bg-color`);
        const textColorEl = document.getElementById(`${btnId}-text-color`);
        const sizeEl = document.getElementById(`${btnId}-size`);
        return {
          bg_color: bgColorEl?.value || btnEl.bgColor,
          text_color: textColorEl?.value || btnEl.textColor,
          size: sizeEl?.value || btnEl.size
        };
      });
    }
    
    switch (type) {
      case 'hero':
        data.title = document.getElementById('prop-title')?.value || '';
        data.subtitle = document.getElementById('prop-subtitle')?.value || '';
        data.button_text = document.getElementById('prop-button-text')?.value || '';
        data.button_link = _stripTenantLinkPrefix(document.getElementById('prop-button-link')?.value || '#');
        data.button_style = document.getElementById('prop-button-style')?.value || 'primary';
        data.background_image = document.getElementById('prop-background-image')?.value || '';
        data.bg_color = this.getSaveColor('prop-bg-color');
        data.text_color = this.getSaveColor('prop-text-color');
        data.heading_color = this.getSaveColor('prop-heading-color');
        data.min_height = this.getSizeValue('prop-min-height') || '300px';
        data.padding = this.getPaddingValue('prop-padding') || '3rem';
        data.text_align = document.getElementById('prop-text-align')?.value || 'center';
        break;
      case 'text':
        data.content = document.getElementById('prop-content')?.value || '';
        data.text_color = this.getSaveColor('prop-text-color');
        data.font_size = this.getSizeValue('prop-font-size') || '1rem';
        data.line_height = document.getElementById('prop-line-height')?.value || '1.8';
        data.padding = this.getPaddingValue('prop-padding') || '1.5rem';
        data.text_align = document.getElementById('prop-text-align')?.value || 'left';
        break;
      case 'heading':
        data.text = document.getElementById('prop-text')?.value || '';
        data.level = document.getElementById('prop-level')?.value || 'h2';
        data.text_color = this.getSaveColor('prop-text-color');
        data.text_align = document.getElementById('prop-text-align')?.value || 'left';
        data.margin = this.getPaddingValue('prop-margin') || '1.5rem 0';
        break;
      case 'section':
        data.background_color = this.getSaveColor('prop-background-color');
        data.padding = this.getPaddingValue('prop-padding') || '10px';
        const newColumns = parseInt(document.getElementById('prop-columns')?.value || '1');
        const currentData = this.selectedComponent.component_data;
        const oldColumns = currentData.columns || 1;
        
        // 如果欄數改變，需要保留現有的欄內元件
        if (newColumns !== oldColumns) {
          // 收集所有現有欄的子元件（從 component_data 讀取，而非新建的空 data）
          let existingColumnChildren = [];
          if (currentData.column_children && Array.isArray(currentData.column_children)) {
            // 保留現有的 column_children 結構
            existingColumnChildren = currentData.column_children;
          } else if (currentData.children && Array.isArray(currentData.children)) {
            // 從舊的 children 結構遷移
            existingColumnChildren = Array(oldColumns).fill(0).map(() => []);
            const childrenPerColumn = Math.ceil(currentData.children.length / oldColumns);
            currentData.children.forEach((child, idx) => {
              const colIdx = Math.floor(idx / childrenPerColumn);
              if (colIdx < oldColumns) {
                existingColumnChildren[colIdx].push(child);
              }
            });
          }
          
          // 創建新的 column_children 結構
          data.column_children = Array(newColumns).fill(0).map(() => []);
          
          // 保留現有欄的元件，按順序分配到新欄
          if (newColumns >= oldColumns) {
            // 欄數增加：保留所有現有欄的元件，多出的欄為空
            existingColumnChildren.forEach((columnChildren, colIdx) => {
              if (colIdx < newColumns) {
                data.column_children[colIdx] = columnChildren || [];
              }
            });
          } else {
            // 欄數減少：將所有元件合併後重新分配
            let allChildren = [];
            existingColumnChildren.forEach(columnChildren => {
              if (Array.isArray(columnChildren)) {
                allChildren = allChildren.concat(columnChildren);
              }
            });
            
            // 按順序分配到新欄
            allChildren.forEach((child, idx) => {
              const colIdx = idx % newColumns;
              if (!data.column_children[colIdx]) {
                data.column_children[colIdx] = [];
              }
              data.column_children[colIdx].push(child);
            });
          }
          
          // 移除舊的 children
          delete data.children;
        }
        // 欄數沒變時，不覆蓋 column_children，保留原有子元件
        
        data.columns = newColumns;
        break;
      case 'nav':
        data.logo = document.getElementById('prop-logo')?.value || '';
        data.logo_text = document.getElementById('prop-logo-text')?.value || this.enterpriseName || 'Logo';
        data.show_login_icon = document.getElementById('prop-show-login-icon')?.checked !== false;
        data.show_cart_icon = document.getElementById('prop-show-cart-icon')?.checked !== false;
        if (data.show_login_icon) {
          data.login_icon_link = _stripTenantLinkPrefix(document.getElementById('prop-login-icon-link')?.value || '');
        }
        if (data.show_cart_icon) {
          data.cart_icon_link = _stripTenantLinkPrefix(document.getElementById('prop-cart-icon-link')?.value || '');
        }
        data.fixed = document.getElementById('prop-fixed')?.checked === true;
        data.menu_position = document.getElementById('prop-menu-position')?.value || 'right';
        data.show_topbar = document.getElementById('prop-show-topbar')?.checked === true;
        if (data.show_topbar) {
          data.topbar_text = document.getElementById('prop-topbar-text')?.value || '';
          data.topbar_bg_color = document.getElementById('prop-topbar-bg-color')?.value || '#f1f1f1';
          data.topbar_text_color = document.getElementById('prop-topbar-text-color')?.value || '#f97316';
          data.topbar_hide_on_scroll = document.getElementById('prop-topbar-hide-on-scroll')?.checked !== false;
        }
        data.bg_color = this.getSaveColor('prop-bg-color');
        data.padding = this.getPaddingValue('prop-padding') || '0.75rem 2rem';
        data.menu_text_color = this.getSaveColor('prop-menu-text-color');
        data.menu_hover_color = this.getSaveColor('prop-menu-hover-color');
        // menu_items 由 addMenuItem/updateMenuItem/removeMenuItem 管理
        break;
      case 'header':
        data.logo = document.getElementById('prop-logo')?.value || '';
        data.logo_text = document.getElementById('prop-logo-text')?.value || 'Logo';
        data.show_login_icon = document.getElementById('prop-show-login-icon')?.checked !== false;
        data.show_cart_icon = document.getElementById('prop-show-cart-icon')?.checked !== false;
        if (data.show_login_icon) {
          data.login_icon_link = _stripTenantLinkPrefix(document.getElementById('prop-header-login-icon-link')?.value || '');
        }
        if (data.show_cart_icon) {
          data.cart_icon_link = _stripTenantLinkPrefix(document.getElementById('prop-header-cart-icon-link')?.value || '');
        }
        data.bg_color = this.getSaveColor('prop-bg-color');
        data.padding = this.getPaddingValue('prop-padding') || '1rem 2rem';
        data.menu_text_color = this.getSaveColor('prop-menu-text-color');
        // menu_items 由 addMenuItem/updateMenuItem/removeMenuItem 管理
        break;
      case 'product-list':
        data.full_list = document.getElementById('prop-full-list')?.checked === true;
        data.limit = parseInt(document.getElementById('prop-limit')?.value || '12');
        data.columns = parseInt(document.getElementById('prop-columns')?.value || '3');
        data.show_product_type_filter = document.getElementById('prop-show-product-type-filter')?.checked === true;
        data.show_brand_filter = document.getElementById('prop-show-brand-filter')?.checked === true;
        data.product_detail_page = document.getElementById('prop-product-detail-page')?.value || '';
        data.padding = this.getPaddingValue('prop-padding') || '2rem 0';
        data.gap = document.getElementById('prop-gap')?.value || '1rem';
        break;
      case 'service-list':
        data.limit = parseInt(document.getElementById('prop-service-limit')?.value || '12');
        data.columns = parseInt(document.getElementById('prop-service-columns')?.value || '3');
        data.service_detail_page = document.getElementById('prop-service-detail-page')?.value || '';
        data.padding = this.getPaddingValue('prop-service-padding') || '2rem 0';
        break;
      case 'footer':
        data.logo = document.getElementById('prop-logo')?.value || '';
        data.column1_content = document.getElementById('prop-column1-content')?.value || '';
        // menu_items 由 addFooterMenuItem/updateFooterMenuItem/removeFooterMenuItem 管理
        data.copyright = document.getElementById('prop-copyright')?.value || '© 2025 All rights reserved';
        data.bg_color = this.getSaveColor('prop-bg-color');
        data.text_color = this.getSaveColor('prop-text-color');
        data.padding = this.getPaddingValue('prop-padding') || '2rem 0';
        break;
      case 'list':
        // menu_items 由 addListMenuItem/updateListMenuItem/removeListMenuItem 管理
        break;
      case 'order-list':
        data.limit = parseInt(document.getElementById('prop-limit')?.value || '10');
        data.show_status_filter = document.getElementById('prop-show-status-filter')?.checked === true;
        data.show_date_filter = document.getElementById('prop-show-date-filter')?.checked === true;
        const orderListLoginPageSelect = document.getElementById('prop-order-list-login-page');
        if (orderListLoginPageSelect) {
          data.login_page = orderListLoginPageSelect.value || '';
        }
        break;
      case 'blog-list':
        data.limit = parseInt(document.getElementById('prop-limit')?.value || '10');
        data.columns = parseInt(document.getElementById('prop-columns')?.value || '1');
        data.show_image = document.getElementById('prop-show-image')?.checked !== false;
        data.show_excerpt = document.getElementById('prop-show-excerpt')?.checked !== false;
        data.category = document.getElementById('prop-category')?.value || '';
        break;
      case 'contact-form':
        data.show_name = document.getElementById('prop-show-name')?.checked !== false;
        data.show_email = document.getElementById('prop-show-email')?.checked !== false;
        data.show_phone = document.getElementById('prop-show-phone')?.checked !== false;
        data.show_message = document.getElementById('prop-show-message')?.checked !== false;
        data.submit_button_text = document.getElementById('prop-submit-button-text')?.value || this.t('pages.pageEditor.preview.contact.submit', 'Submit');
        break;
      case 'service-booking':
        data.title = document.getElementById('prop-title')?.value || this.t('pages.pageEditor.defaults.serviceBooking.title', 'Book a Service');
        data.subtitle = document.getElementById('prop-subtitle')?.value || this.t('pages.pageEditor.defaults.serviceBooking.subtitle', 'Choose a service and book your appointment');
        data.step1_title = document.getElementById('prop-step1-title')?.value || this.t('pages.pageEditor.defaults.serviceBooking.step1', 'Select Service');
        data.step2_title = document.getElementById('prop-step2-title')?.value || this.t('pages.pageEditor.defaults.serviceBooking.step2', 'Choose Date & Time');
        data.step3_title = document.getElementById('prop-step3-title')?.value || this.t('pages.pageEditor.defaults.serviceBooking.step3', 'Confirm Booking');
        data.show_service_select = document.getElementById('prop-show-service-select')?.checked !== false;
        data.show_staff_select = document.getElementById('prop-show-staff-select')?.checked !== false;
        data.show_date_picker = document.getElementById('prop-show-date-picker')?.checked !== false;
        data.show_time_slots = document.getElementById('prop-show-time-slots')?.checked !== false;
        data.require_login = document.getElementById('prop-require-login')?.checked || false;
        data.success_message = document.getElementById('prop-success-message')?.value || this.t('pages.pageEditor.defaults.serviceBooking.successMessage', 'Your booking has been confirmed! We will contact you shortly.');
        data.button_text = document.getElementById('prop-button-text')?.value || this.t('pages.pageEditor.defaults.serviceBooking.buttonText', 'Confirm Booking');
        data.primary_color = this.getSaveColor('prop-primary-color');
        break;
      case 'dining-menu':
        data.show_title = document.getElementById('prop-dining-menu-show-title')?.checked !== false;
        data.title = document.getElementById('prop-dining-menu-title')?.value || this.t('pages.pageEditor.defaults.diningMenu.title', '菜單');
        data.subtitle = document.getElementById('prop-dining-menu-subtitle')?.value || this.t('pages.pageEditor.defaults.diningMenu.subtitle', '瀏覽餐點與價格');
        data.height = this.getSizeValue('prop-dining-menu-height') || '720px';
        break;
      case 'dining-table-reservation':
        data.show_title = document.getElementById('prop-dining-reservation-show-title')?.checked !== false;
        data.title = document.getElementById('prop-dining-reservation-title')?.value || this.t('pages.pageEditor.defaults.diningReservation.title', '預約餐桌');
        data.subtitle = document.getElementById('prop-dining-reservation-subtitle')?.value || this.t('pages.pageEditor.defaults.diningReservation.subtitle', '填寫聯絡資訊完成預約');
        data.height = this.getSizeValue('prop-dining-reservation-height') || '720px';
        break;
      case 'cart':
        data.show_checkout_button = document.getElementById('prop-show-checkout-button')?.checked !== false;
        data.show_continue_shopping = document.getElementById('prop-show-continue-shopping')?.checked !== false;
        data.show_coupon = document.getElementById('prop-show-coupon')?.checked !== false;
        data.show_points = document.getElementById('prop-show-points')?.checked !== false;
        break;
      case 'login-register':
        data.show_login = document.getElementById('prop-show-login')?.checked !== false;
        data.show_register = document.getElementById('prop-show-register')?.checked !== false;
        data.login_method = document.getElementById('prop-login-method')?.value || 'phone_or_email';
        data.redirect_after_login = _stripTenantLinkPrefix(document.getElementById('prop-redirect-after-login')?.value || '/');
        data.redirect_after_register = _stripTenantLinkPrefix(document.getElementById('prop-redirect-after-register')?.value || '/');
        break;
      case 'cart':
        data.show_checkout_button = document.getElementById('prop-show-checkout-button')?.checked !== false;
        data.show_continue_shopping = document.getElementById('prop-show-continue-shopping')?.checked !== false;
        break;
      case 'checkout':
        data.show_shipping_form = document.getElementById('prop-show-shipping-form')?.checked !== false;
        data.show_payment_form = document.getElementById('prop-show-payment-form')?.checked !== false;
        break;
      case 'product-detail':
        data.product_name = document.getElementById('prop-product-name')?.value || '產品名稱';
        data.product_price = document.getElementById('prop-product-price')?.value || '$0.00';
        data.product_description = document.getElementById('prop-product-description')?.value || '產品描述';
        data.padding = this.getPaddingValue('prop-padding') || '2rem 0';
        break;
      case 'banner-slider':
        data.height = this.getSizeValue('prop-banner-height') || '360px';
        data.autoplay = document.getElementById('prop-banner-autoplay')?.checked !== false;
        data.interval = parseInt(document.getElementById('prop-banner-interval')?.value || '5000');
        data.show_indicators = document.getElementById('prop-banner-show-indicators')?.checked !== false;
        data.show_arrows = document.getElementById('prop-banner-show-arrows')?.checked !== false;
        data.text_align = document.getElementById('prop-banner-text-align')?.value || 'left';
        data.text_color = this.getSaveColor('prop-banner-text-color');
        data.overlay_color = document.getElementById('prop-banner-overlay-color')?.value || 'rgba(0, 0, 0, 0.45)';
        // slides 由 add/update/remove 处理
        break;
      case 'user-area':
        data.show_profile = document.getElementById('prop-show-profile')?.checked !== false;
        data.show_orders = document.getElementById('prop-show-orders')?.checked !== false;
        data.show_addresses = document.getElementById('prop-show-addresses')?.checked !== false;
        const userAreaLoginPageSelect = document.getElementById('prop-user-area-login-page');
        if (userAreaLoginPageSelect) {
          data.login_page = userAreaLoginPageSelect.value || '';
        }
        break;
      case 'image':
        data.src = document.getElementById('prop-src')?.value || '';
        data.alt = document.getElementById('prop-alt')?.value || 'Image';
        data.width = document.getElementById('prop-width')?.value || '100%';
        data.padding = this.getPaddingValue('prop-padding') || '2rem';
        break;
      case 'button':
        data.text = document.getElementById('prop-text')?.value || 'Button';
        data.link = _stripTenantLinkPrefix(document.getElementById('prop-link')?.value || '#');
        data.style = document.getElementById('prop-style')?.value || 'primary';
        data.button_color = this.getSaveColor('prop-button-color');
        data.button_text_color = this.getSaveColor('prop-button-text-color');
        data.size = document.getElementById('prop-size')?.value || '';
        data.padding = this.getPaddingValue('prop-padding') || '1.5rem';
        data.align = document.getElementById('prop-align')?.value || 'center';
        break;
      case 'google-map':
        data.lat = parseFloat(document.getElementById('prop-gmap-lat')?.value) || 25.033964;
        data.lng = parseFloat(document.getElementById('prop-gmap-lng')?.value) || 121.564468;
        data.zoom = parseInt(document.getElementById('prop-gmap-zoom')?.value) || 14;
        {
          let h = (document.getElementById('prop-gmap-height')?.value || '400px').trim();
          // Normalize: strip all unit suffixes, then append 'px' once
          h = h.replace(/(px|%|rem|em|vh|vw)+$/i, '');
          if (/^\d+(\.\d+)?$/.test(h)) h += 'px';
          else if (!/\d+(px|%|rem|em|vh|vw)$/i.test(h)) h += 'px';
          data.height = h;
        }
        data.marker_title = document.getElementById('prop-gmap-marker-title')?.value || '';
        data.address = document.getElementById('prop-gmap-address')?.value || '';
        break;
      case 'custom-html':
        data.html_content = document.getElementById('prop-html-content')?.value || '';
        break;
    }
    
    // 检查是否是栏位内的元件（不在主 components 数组中）
    const isColumnChildComponent = !this.components.includes(this.selectedComponent);
    
    if (isColumnChildComponent) {
      // 栏位内的元件：直接更新数据，然后重新渲染整个 section
      this.selectedComponent.component_data = { ...this.selectedComponent.component_data, ...data };
      this.saveState();
      this.renderComponents();
      
        // 重新选中该元件
      setTimeout(() => {
        // 查找包含这个子元件的 section
        for (let i = 0; i < this.components.length; i++) {
          const comp = this.components[i];
          if (comp.component_type === 'section' && comp.component_data.column_children) {
            for (let colIdx = 0; colIdx < comp.component_data.column_children.length; colIdx++) {
              const columnChildren = comp.component_data.column_children[colIdx];
              const childIdx = columnChildren.findIndex(child => child === this.selectedComponent);
              if (childIdx !== -1) {
                // 先清除所有栏内元件的选中状态
                document.querySelectorAll('.column-child-item').forEach(item => {
                  item.style.outline = '';
                  item.style.outlineOffset = '';
                });
                this.editColumnChild(i, colIdx, childIdx);
                return;
              }
            }
          }
        }
      }, 100);
    } else {
      // 主元件：正常更新
    this.selectedComponent.component_data = { ...this.selectedComponent.component_data, ...data };
    
    // 保存当前选中元素的信息和样式（如果存在）
    let savedElementInfo = null;
    let savedElementStyles = null;
    if (this.selectedElement) {
      savedElementInfo = {
        elementId: this.selectedElement.dataset.elementId,
        field: this.selectedElement.dataset.field,
        tag: this.selectedElement.tagName.toLowerCase(),
        className: this.selectedElement.className
      };
      
      // 保存所有内联样式（包括通过 setProperty 设置的样式）
      savedElementStyles = {};
      // 直接保存完整的 style.cssText，这样可以保留所有样式和 important 标志
      if (this.selectedElement.style && this.selectedElement.style.cssText) {
        savedElementStyles._cssText = this.selectedElement.style.cssText;
      }
      
      // 也保存各个样式属性的值和优先级（作为备用）
      const styleProperties = ['color', 'font-size', 'font-weight', 'text-align', 'background-color', 'border-color', 'padding'];
      styleProperties.forEach(prop => {
        const value = this.selectedElement.style.getPropertyValue(prop);
        const priority = this.selectedElement.style.getPropertyPriority(prop);
        if (value) {
          savedElementStyles[prop] = { value, priority };
        }
      });
    }
    
    // 只更新選中元件的預覽，不重新渲染整個列表和屬性面板
    const index = this.components.indexOf(this.selectedComponent);
    const item = document.querySelector(`.component-item[data-index="${index}"]`);
    if (item) {
      const isNavBottomComponent = this.selectedComponent.component_type === 'nav' && (this.selectedComponent.component_data?.menu_position || 'right') === 'bottom';
      item.classList.toggle('nav-bottom-component', isNavBottomComponent);
      const previewContainer = item.querySelector('.component-preview');
      if (previewContainer) {
        previewContainer.innerHTML = this.renderComponentPreview(this.selectedComponent);

          // 初始化 Google Map 預覽（innerHTML 不會執行 script，需手動初始化）
          this.initGoogleMapPreviews();
          
          // 綁定可編輯元素的事件
          this.bindEditableElements(previewContainer);
          
          // 恢复选中元素的 data-element-id 和样式（如果之前有选中元素）
          if (savedElementInfo && savedElementInfo.elementId) {
            // 尝试通过 field 和 tag 查找元素
            let foundElement = null;
            if (savedElementInfo.field) {
              const candidates = previewContainer.querySelectorAll(`${savedElementInfo.tag}[data-field="${savedElementInfo.field}"]`);
              // 尝试匹配 className
              for (const candidate of candidates) {
                if (candidate.className === savedElementInfo.className) {
                  candidate.dataset.elementId = savedElementInfo.elementId;
                  foundElement = candidate;
                  break;
                }
              }
              // 如果只有一个匹配，使用它
              if (candidates.length === 1 && !foundElement) {
                candidates[0].dataset.elementId = savedElementInfo.elementId;
                foundElement = candidates[0];
              }
            }
            
            // 恢复样式
            if (foundElement && savedElementStyles) {
              // 如果有保存的完整 cssText，直接应用（这是最可靠的方法）
              if (savedElementStyles._cssText) {
                foundElement.style.cssText = savedElementStyles._cssText;
              } else {
                // 否则逐个应用样式属性（备用方法）
                Object.keys(savedElementStyles).forEach(prop => {
                  if (prop !== '_cssText' && savedElementStyles[prop]) {
                    const { value, priority } = savedElementStyles[prop];
                    foundElement.style.setProperty(prop, value, priority || 'important');
                  }
                });
              }
              this.selectedElement = foundElement;
            }
          }
        }
      }
    }
    
    // 使用防抖來避免每次輸入都保存歷史記錄
    if (this.updateComponentDataTimeout) {
      clearTimeout(this.updateComponentDataTimeout);
    }
    this.updateComponentDataTimeout = setTimeout(() => {
      this.saveState(); // 保存狀態到歷史記錄
      this.updateComponentDataTimeout = null;
    }, 1000); // 1秒延遲
  },

  bindEditableElements(previewContainer) {
    // 綁定所有可編輯元素的事件
    previewContainer.querySelectorAll('.editable-text[contenteditable="true"]').forEach(el => {
      // 移除舊的事件監聽器（如果有的話）
      const newEl = el.cloneNode(true);
      el.parentNode.replaceChild(newEl, el);
      
      // 綁定 blur 事件，當失去焦點時保存內容
      newEl.addEventListener('blur', (e) => {
        const field = e.target.dataset.field;
        const menuIndex = e.target.dataset.menuIndex;
        
        if (field) {
          if (field === 'menu_item_text' && menuIndex !== undefined) {
            // 更新選單項目文字
            const index = parseInt(menuIndex);
            if (this.selectedComponent.component_data.menu_items && this.selectedComponent.component_data.menu_items[index]) {
              this.selectedComponent.component_data.menu_items[index].text = e.target.textContent.trim();
            }
          } else {
            // 更新一般欄位
            this.selectedComponent.component_data[field] = e.target.textContent.trim();
          }
          
          // 保存狀態
          this.saveState();
        }
      });
      
      // 單擊時顯示該元素的專屬屬性面板
      newEl.addEventListener('click', (e) => {
        e.stopPropagation();
        e.preventDefault();
        // 用 currentTarget，避免點到內層 <span data-i18n> 時把 span 當成選中元素，進而在重渲染後覆蓋文字
        this.showElementProperties(e.currentTarget);
      });
    });
    
    // 綁定按鈕元素的單擊事件（顯示專屬屬性）
    // 排除特殊按钮：section-column-add-btn, add-component-after-btn, edit-component-btn, component-actions 内的按钮
    previewContainer.querySelectorAll('button, a.btn, input[type="button"], input[type="submit"]').forEach(btn => {
      // 跳过特殊按钮
      if (btn.classList.contains('section-column-add-btn') || 
          btn.classList.contains('add-component-after-btn') || 
          btn.classList.contains('edit-component-btn') ||
          btn.closest('.component-actions') ||
          btn.closest('.column-child-actions')) {
        return;
      }
      
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        e.preventDefault();
        this.showElementProperties(e.currentTarget);
      });
    });
    
    // 綁定圖片元素的單擊事件（顯示專屬屬性）
    previewContainer.querySelectorAll('img').forEach(img => {
      img.addEventListener('click', (e) => {
        e.stopPropagation();
        e.preventDefault();
        this.showElementProperties(e.currentTarget);
      });
    });
    
    // 禁用 view area 内所有超链接（除了已绑定事件的按钮）
    previewContainer.querySelectorAll('a[href]:not(.btn)').forEach(link => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        return false;
      });
    });
  },
  
  showElementProperties(element) {
    // 归一化：如果传进来的是内层节点（例如 span/data-i18n），提升到真正可编辑的承载元素
    if (element && element.closest) {
      const normalized = element.closest('.editable-text[contenteditable="true"], button, a.btn, input[type="button"], input[type="submit"], img');
      if (normalized) element = normalized;
    }

    // 找到元素所属的组件
    const componentItem = element.closest('.component-item');
    if (!componentItem) return;
    
    const index = parseInt(componentItem.dataset.index);
    if (isNaN(index)) return;
    
    const component = this.components[index];
    if (!component) return;
    
    // 先激活组件（显示 label 和 function bar）
    this.selectComponent(component);
    
    // 識別元素類型
    const isText = element.matches('h1, h2, h3, h4, h5, h6, p, span, div, a:not(.btn), li, td, th, label, small');
    const isButton = element.matches('button, a.btn, input[type="button"], input[type="submit"]');
    const isImage = element.matches('img');
    
    if (!isText && !isButton && !isImage) return;
    
    // 獲取元素的當前樣式和屬性
    const computedStyle = window.getComputedStyle(element);
    
    // 獲取 previewContainer
    const item = document.querySelector(`.component-item[data-index="${index}"]`);
    if (!item) return;
    const previewContainer = item.querySelector('.component-preview');
    if (!previewContainer) return;
    
    // 生成唯一的元素 ID
    const elementId = `element-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    element.dataset.elementId = elementId;
    
    // 保存當前選中的元素，用於返回
    this.selectedElement = element;
    
    // 創建專屬屬性面板 HTML（完全替換屬性面板）
    let elementPropsHtml = '';
    
    if (isText) {
      const textColor = computedStyle.color;
      const fontSize = computedStyle.fontSize;
      const fontWeight = computedStyle.fontWeight;
      const textAlign = computedStyle.textAlign;
      const isLink = element.tagName.toLowerCase() === 'a' && !element.classList.contains('btn');
      const linkUrl = isLink ? _stripTenantLinkPrefix(element.getAttribute('href') || '') : '';
      
      const fontSizeParsed = this.parseValueWithUnit(fontSize);
      
      elementPropsHtml = `
        <div class="mb-3">
          <button class="btn btn-sm btn-outline-secondary mb-3" onclick="PageEditor.backToComponentProperties()">
            <i class="bi bi-arrow-left"></i> ${this.t('pages.pageEditor.elementProps.backToComponentProperties', 'Back to component properties')}
          </button>
          <h6 class="mb-3">${this.t('pages.pageEditor.elementProps.text.title', 'Text element')}</h6>
          ${isLink ? `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.elementProps.linkUrl', 'Link URL')}</label>
            <input type="text" class="form-control element-link-url" data-element-id="${elementId}" value="${linkUrl}" placeholder="${this.t('pages.pageEditor.placeholders.exampleLink', 'e.g., /about or https://example.com')}">
          </div>
          ` : ''}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.textColor', 'Text color')}</label>
            <input type="color" class="form-control form-control-color element-text-color" data-element-id="${elementId}" value="${this.rgbToHex(textColor)}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.fontSize', 'Font size')}</label>
            <div class="input-group">
              <input type="text" class="form-control element-font-size" data-element-id="${elementId}" value="${fontSizeParsed.value}" placeholder="${this.t('pages.pageEditor.placeholders.exampleFontSize', 'e.g., 16')}">
              <select class="form-select element-font-size-unit" style="width: 120px;" data-element-id="${elementId}">
                <option value="px" ${fontSizeParsed.unit === 'px' ? 'selected' : ''}>px</option>
                <option value="rem" ${fontSizeParsed.unit === 'rem' ? 'selected' : ''}>rem</option>
                <option value="em" ${fontSizeParsed.unit === 'em' ? 'selected' : ''}>em</option>
                <option value="%" ${fontSizeParsed.unit === '%' ? 'selected' : ''}>%</option>
              </select>
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.elementProps.fontWeight', 'Font weight')}</label>
            <select class="form-select element-font-weight" data-element-id="${elementId}">
              <option value="">${this.t('pages.pageEditor.common.default', 'Default')}</option>
              <option value="100" ${fontWeight === '100' ? 'selected' : ''}>100</option>
              <option value="300" ${fontWeight === '300' ? 'selected' : ''}>300</option>
              <option value="400" ${fontWeight === '400' || fontWeight === 'normal' ? 'selected' : ''}>400</option>
              <option value="500" ${fontWeight === '500' ? 'selected' : ''}>500</option>
              <option value="600" ${fontWeight === '600' ? 'selected' : ''}>600</option>
              <option value="700" ${fontWeight === '700' || fontWeight === 'bold' ? 'selected' : ''}>700</option>
              <option value="900" ${fontWeight === '900' ? 'selected' : ''}>900</option>
            </select>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.textAlign', 'Text align')}</label>
            <select class="form-select element-text-align" data-element-id="${elementId}">
              <option value="left" ${textAlign === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${textAlign === 'center' ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${textAlign === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
            </select>
          </div>
        </div>
      `;
    } else if (isButton) {
      const backgroundColor = computedStyle.backgroundColor;
      const textColor = computedStyle.color;
      const borderColor = computedStyle.borderColor;
      const padding = computedStyle.padding;
      const isLinkButton = element.tagName.toLowerCase() === 'a';
      const linkUrl = isLinkButton ? _stripTenantLinkPrefix(element.getAttribute('href') || '') : '';
      const parentAlign = element.parentElement ? window.getComputedStyle(element.parentElement).textAlign : 'left';
      
      elementPropsHtml = `
        <div class="mb-3">
          <button class="btn btn-sm btn-outline-secondary mb-3" onclick="PageEditor.backToComponentProperties()">
            <i class="bi bi-arrow-left"></i> ${this.t('pages.pageEditor.elementProps.backToComponentProperties', 'Back to component properties')}
          </button>
          <h6 class="mb-3">${this.t('pages.pageEditor.elementProps.button.title', 'Button element')}</h6>
          ${isLinkButton ? `
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.elementProps.linkUrl', 'Link URL')}</label>
            <input type="text" class="form-control element-button-link-url" data-element-id="${elementId}" value="${linkUrl}" placeholder="${this.t('pages.pageEditor.placeholders.exampleLink', 'e.g., /about or https://example.com')}">
          </div>
          ` : ''}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.backgroundColor', 'Background color')}</label>
            <input type="color" class="form-control form-control-color element-button-bg-color" data-element-id="${elementId}" value="${this.rgbToHex(backgroundColor)}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.common.textColor', 'Text color')}</label>
            <input type="color" class="form-control form-control-color element-button-text-color" data-element-id="${elementId}" value="${this.rgbToHex(textColor)}">
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.elementProps.borderColor', 'Border color')}</label>
            <input type="color" class="form-control form-control-color element-button-border-color" data-element-id="${elementId}" value="${this.rgbToHex(borderColor)}">
          </div>
          ${this.renderPaddingInput('element-button-padding', this.t('pages.pageEditor.labels.common.padding', 'Padding'), padding || '0.5rem 1rem')}
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.button.align', 'Button align')}</label>
            <select class="form-select element-button-align" data-element-id="${elementId}">
              <option value="left" ${parentAlign === 'left' ? 'selected' : ''}>${this.t('pages.pageEditor.align.left', 'Left')}</option>
              <option value="center" ${parentAlign === 'center' ? 'selected' : ''}>${this.t('pages.pageEditor.align.center', 'Center')}</option>
              <option value="right" ${parentAlign === 'right' ? 'selected' : ''}>${this.t('pages.pageEditor.align.right', 'Right')}</option>
            </select>
          </div>
        </div>
      `;
    } else if (isImage) {
      const src = element.src || element.getAttribute('src') || '';
      const alt = element.alt || '';
      
      elementPropsHtml = `
        <div class="mb-3">
          <button class="btn btn-sm btn-outline-secondary mb-3" onclick="PageEditor.backToComponentProperties()">
            <i class="bi bi-arrow-left"></i> ${this.t('pages.pageEditor.elementProps.backToComponentProperties', 'Back to component properties')}
          </button>
          <h6 class="mb-3">${this.t('pages.pageEditor.elementProps.image.title', 'Image element')}</h6>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.image.image', 'Image')}</label>
            <div class="d-flex align-items-center gap-2 mb-2">
              ${src ? `<img src="${src}" alt="Preview" style="max-height: 100px; max-width: 200px; border: 1px solid #dee2e6; border-radius: 4px; padding: 4px;">` : ''}
              <input type="file" class="form-control form-control-sm element-image-file" data-element-id="${elementId}" accept="image/*" style="display: none;">
              <button type="button" class="btn btn-sm btn-outline-primary" onclick="document.querySelector('.element-image-file[data-element-id=\\'${elementId}\\']').click();">${src ? this.t('pages.pageEditor.actions.replace', 'Replace') : this.t('common.upload', 'Upload')} ${this.t('pages.pageEditor.labels.image.image', 'Image')}</button>
              ${src ? `<button type="button" class="btn btn-sm btn-outline-danger" onclick="PageEditor.removeElementImage('${elementId}');">${this.t('common.remove', 'Remove')}</button>` : ''}
            </div>
          </div>
          <div class="mb-3">
            <label class="form-label">${this.t('pages.pageEditor.labels.image.alt', 'Alt text')}</label>
            <input type="text" class="form-control element-image-alt" data-element-id="${elementId}" value="${alt}" placeholder="${this.t('pages.pageEditor.placeholders.altText', 'Enter alt text')}">
          </div>
        </div>
      `;
    }
    
    // 直接替換整個屬性面板內容
    const propertiesContent = document.getElementById('propertiesContent');
    if (propertiesContent) {
      propertiesContent.innerHTML = elementPropsHtml;
      propertiesContent.classList.add('has-form');
      
      // 綁定事件 - 使用保存的 selectedElement（要修改的 DOM 元素）
      setTimeout(() => {
        if (this.selectedElement) {
          this.bindElementPropertiesEvents(this.selectedElement);
        }
      }, 50);
    }
  },
  
  backToComponentProperties() {
    // 返回顯示元件屬性
    if (this.selectedComponent) {
      this.renderProperties(this.selectedComponent);
      this.selectedElement = null;
    }
  },
  
  bindElementPropertiesEvents(element) {
    const elementId = element.dataset.elementId;
    if (!elementId) return;
    
    // 保存元素的关键信息，用于在重新渲染后查找
    const elementField = element.dataset.field;
    const elementTag = element.tagName.toLowerCase();
    const elementClass = element.className;
    
    // 辅助函数：保存元素样式到 component_data
    const saveElementStyleToData = (currentElement) => {
      if (!currentElement || !this.selectedComponent) return;
      
      const field = currentElement.dataset.field;
      if (!field) return;
      
      // 对于菜单项，使用 menu_index 来创建唯一的键
      let styleKey = field;
      if (field === 'menu_item_text' && currentElement.dataset.menuIndex !== undefined) {
        styleKey = `menu_item_text_${currentElement.dataset.menuIndex}`;
      }
      
      // 初始化 element_styles 对象
      if (!this.selectedComponent.component_data.element_styles) {
        this.selectedComponent.component_data.element_styles = {};
      }
      
      // 保存完整的 style.cssText
      if (currentElement.style && currentElement.style.cssText) {
        this.selectedComponent.component_data.element_styles[styleKey] = currentElement.style.cssText;
      }
    };
    
    // 辅助函数：动态查找当前元素（因为可能被重新渲染）
    const findCurrentElement = () => {
      // 首先尝试通过 elementId 查找
      let currentElement = document.querySelector(`[data-element-id="${elementId}"]`);
      if (currentElement) return currentElement;
      
      // 如果找不到，尝试通过 field 和 tag 查找
      if (elementField) {
        const candidates = document.querySelectorAll(`${elementTag}[data-field="${elementField}"]`);
        // 尝试匹配 className
        for (const candidate of candidates) {
          if (candidate.className === elementClass) {
            // 重新设置 elementId
            candidate.dataset.elementId = elementId;
            return candidate;
          }
        }
        // 如果只有一个匹配，使用它
        if (candidates.length === 1) {
          candidates[0].dataset.elementId = elementId;
          return candidates[0];
        }
      }
      
      // 如果还是找不到，返回 null
      return null;
    };
    
    // 文字屬性事件
    const textColorInput = document.querySelector(`.element-text-color[data-element-id="${elementId}"]`);
    const fontSizeInput = document.querySelector(`.element-font-size[data-element-id="${elementId}"]`);
    const fontSizeUnitSelect = document.querySelector(`.element-font-size-unit[data-element-id="${elementId}"]`);
    const fontWeightSelect = document.querySelector(`.element-font-weight[data-element-id="${elementId}"]`);
    const textAlignSelect = document.querySelector(`.element-text-align[data-element-id="${elementId}"]`);
    const linkUrlInput = document.querySelector(`.element-link-url[data-element-id="${elementId}"]`);
    
    if (textColorInput) {
      textColorInput.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.style.setProperty('color', textColorInput.value, 'important');
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          // 更新 selectedElement 引用
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    if (fontSizeInput) {
      const updateFontSize = () => {
        const unit = fontSizeUnitSelect?.value || 'px';
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.style.setProperty('font-size', `${fontSizeInput.value}${unit}`, 'important');
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          // 更新 selectedElement 引用
          this.selectedElement = currentElement;
          this.saveState();
        }
      };
      fontSizeInput.addEventListener('input', updateFontSize);
      if (fontSizeUnitSelect) {
        fontSizeUnitSelect.addEventListener('change', updateFontSize);
      }
    }
    
    if (fontWeightSelect) {
      fontWeightSelect.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.style.setProperty('font-weight', fontWeightSelect.value || 'normal', 'important');
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          // 更新 selectedElement 引用
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    if (textAlignSelect) {
      textAlignSelect.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          // 如果是內聯元素，需要設置父元素的 text-align
          if (currentElement.style.display === 'inline' || currentElement.tagName.toLowerCase() === 'a') {
            const parent = currentElement.parentElement;
            if (parent) {
              parent.style.setProperty('text-align', textAlignSelect.value, 'important');
            }
          } else {
            currentElement.style.setProperty('text-align', textAlignSelect.value, 'important');
          }
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          // 更新 selectedElement 引用
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    if (linkUrlInput) {
      linkUrlInput.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          const strippedValue = _stripTenantLinkPrefix(linkUrlInput.value || '#');
          currentElement.setAttribute('href', strippedValue);
          // 更新 component_data 中對應的連結
          if (this.selectedComponent) {
            const field = currentElement.dataset.field;
            if (field === 'button_link') {
              this.selectedComponent.component_data.button_link = strippedValue;
            }
          }
          // 更新 selectedElement 引用
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    // 按鈕屬性事件
    const buttonBgColorInput = document.querySelector(`.element-button-bg-color[data-element-id="${elementId}"]`);
    const buttonTextColorInput = document.querySelector(`.element-button-text-color[data-element-id="${elementId}"]`);
    const buttonBorderColorInput = document.querySelector(`.element-button-border-color[data-element-id="${elementId}"]`);
    const buttonPaddingTopInput = document.querySelector(`#element-button-padding-top`);
    const buttonPaddingRightInput = document.querySelector(`#element-button-padding-right`);
    const buttonPaddingBottomInput = document.querySelector(`#element-button-padding-bottom`);
    const buttonPaddingLeftInput = document.querySelector(`#element-button-padding-left`);
    const buttonPaddingUnitSelect = document.querySelector(`#element-button-padding-unit`);
    const buttonAlignSelect = document.querySelector(`.element-button-align[data-element-id="${elementId}"]`);
    const buttonLinkUrlInput = document.querySelector(`.element-button-link-url[data-element-id="${elementId}"]`);
    
    if (buttonBgColorInput) {
      buttonBgColorInput.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.style.setProperty('background-color', buttonBgColorInput.value, 'important');
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    if (buttonTextColorInput) {
      buttonTextColorInput.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.style.setProperty('color', buttonTextColorInput.value, 'important');
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    if (buttonBorderColorInput) {
      buttonBorderColorInput.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.style.setProperty('border-color', buttonBorderColorInput.value, 'important');
          // 保存样式到 component_data
          saveElementStyleToData(currentElement);
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    // 更新按钮内边距（五格形式）
    const updateButtonPadding = () => {
      const currentElement = findCurrentElement();
      if (!currentElement) return;
      
      const top = buttonPaddingTopInput?.value || '';
      const right = buttonPaddingRightInput?.value || '';
      const bottom = buttonPaddingBottomInput?.value || '';
      const left = buttonPaddingLeftInput?.value || '';
      const unit = buttonPaddingUnitSelect?.value || 'px';
      
      if (!top && !right && !bottom && !left) {
        currentElement.style.removeProperty('padding');
        this.selectedElement = currentElement;
        this.saveState();
        return;
      }
      
      // 如果所有值相同，返回单一值
      let paddingValue = '';
      if (top === right && right === bottom && bottom === left) {
        paddingValue = `${top}${unit}`;
      } else if (top === bottom && right === left) {
        // 如果上下相同且左右相同，返回两个值
        paddingValue = `${top}${unit} ${right}${unit}`;
      } else {
        // 否则返回四个值
        paddingValue = `${top}${unit} ${right}${unit} ${bottom}${unit} ${left}${unit}`;
      }
      currentElement.style.setProperty('padding', paddingValue, 'important');
      // 保存样式到 component_data
      saveElementStyleToData(currentElement);
      this.selectedElement = currentElement;
      this.saveState();
    };
    
    if (buttonPaddingTopInput) {
      buttonPaddingTopInput.addEventListener('input', updateButtonPadding);
    }
    if (buttonPaddingRightInput) {
      buttonPaddingRightInput.addEventListener('input', updateButtonPadding);
    }
    if (buttonPaddingBottomInput) {
      buttonPaddingBottomInput.addEventListener('input', updateButtonPadding);
    }
    if (buttonPaddingLeftInput) {
      buttonPaddingLeftInput.addEventListener('input', updateButtonPadding);
    }
    if (buttonPaddingUnitSelect) {
      buttonPaddingUnitSelect.addEventListener('change', updateButtonPadding);
    }
    
    if (buttonAlignSelect) {
      buttonAlignSelect.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (!currentElement) return;
        
        // 找到按钮的父容器（通常是 col-12 或 container）
        let container = currentElement.parentElement;
        // 如果父元素是 col-12，再找上一层的 row
        if (container && container.classList.contains('col-12')) {
          container = container.parentElement;
        }
        // 如果父元素是 row，再找上一层的 container
        if (container && container.classList.contains('row')) {
          container = container.parentElement;
        }
        // 设置对齐方式
        if (container) {
          // 移除所有对齐类
          container.classList.remove('text-start', 'text-center', 'text-end', 'd-flex', 'justify-content-start', 'justify-content-center', 'justify-content-end');
          // 添加新的对齐类
          if (buttonAlignSelect.value === 'left') {
            container.classList.add('d-flex', 'justify-content-start');
          } else if (buttonAlignSelect.value === 'center') {
            container.classList.add('d-flex', 'justify-content-center');
          } else if (buttonAlignSelect.value === 'right') {
            container.classList.add('d-flex', 'justify-content-end');
          }
        }
        this.selectedElement = currentElement;
        this.saveState();
      });
    }
    
    if (buttonLinkUrlInput) {
      buttonLinkUrlInput.addEventListener('change', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          const strippedValue = _stripTenantLinkPrefix(buttonLinkUrlInput.value || '#');
          currentElement.setAttribute('href', strippedValue);
          // 更新 component_data 中對應的連結
          if (this.selectedComponent) {
            const field = currentElement.dataset.field;
            if (field === 'button_link' || field === 'link') {
              this.selectedComponent.component_data[field] = strippedValue;
            }
          }
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
    
    // 圖片屬性事件
    const imageFileInput = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`);
    const imageSrcInput = document.querySelector(`.element-image-src[data-element-id="${elementId}"]`);
    const imageAltInput = document.querySelector(`.element-image-alt[data-element-id="${elementId}"]`);
    
    if (imageFileInput) {
      imageFileInput.addEventListener('change', async (e) => {
        const file = e.target.files[0];
        if (file) {
          const currentElement = findCurrentElement();
          if (currentElement) {
            await this.handleElementImageUpload(file, elementId, currentElement);
          }
        }
      });
    }
    
    // 移除 URL 输入框的事件绑定，只保留上传功能
    
    if (imageAltInput) {
      imageAltInput.addEventListener('input', () => {
        const currentElement = findCurrentElement();
        if (currentElement) {
          currentElement.alt = imageAltInput.value;
          this.selectedElement = currentElement;
          this.saveState();
        }
      });
    }
  },
  
  async handleElementImageUpload(file, elementId, element) {
    if (!file || !this.selectedElement) return;
    
    try {
      const formData = new FormData();
      formData.append('file', file);
      
      const response = await App.apiRequest('/upload', {
        method: 'POST',
        body: formData
      });
      
      if (response.url) {
        // 更新圖片元素的 src
        element.src = response.url;
        
        // 更新隱藏的 src input
        const imageSrcInput = document.querySelector(`.element-image-src[data-element-id="${elementId}"]`);
        if (imageSrcInput) {
          imageSrcInput.value = response.url;
        }
        
        // 更新預覽圖片
        const previewImg = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement?.querySelector('img');
        if (previewImg) {
          previewImg.src = response.url;
        }
        
        // 更新按鈕文字
        const uploadBtn = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement?.querySelector('button');
        if (uploadBtn) {
          uploadBtn.textContent = '更換 圖片';
        }
        
        // 顯示移除按鈕（如果還沒有）
        const removeBtn = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement?.querySelector('.btn-outline-danger');
        if (!removeBtn) {
          const btnContainer = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement;
          if (btnContainer) {
            const newRemoveBtn = document.createElement('button');
            newRemoveBtn.type = 'button';
            newRemoveBtn.className = 'btn btn-sm btn-outline-danger';
            newRemoveBtn.textContent = '移除';
            newRemoveBtn.onclick = () => this.removeElementImage(elementId);
            btnContainer.appendChild(newRemoveBtn);
          }
        }
        
        // 更新 component_data 中對應的圖片 URL
        const field = element.closest('.component-preview')?.querySelector('[data-field]')?.dataset.field;
        if (field && this.selectedComponent) {
          if (field.includes('image') || field.includes('src')) {
            this.selectedComponent.component_data[field] = response.url;
          }
        }
        
        this.saveState();
      }
    } catch (error) {
      App.showError('上傳圖片失敗: ' + (error.error || error.message));
    }
  },
  
  async removeElementImage(elementId) {
    if (!this.selectedElement) return;
    
    const element = this.selectedElement;
    const imageSrcInput = document.querySelector(`.element-image-src[data-element-id="${elementId}"]`);
    
    // 清除圖片
    element.src = '';
    if (imageSrcInput) {
      imageSrcInput.value = '';
    }
    
    // 移除預覽圖片
    const previewImg = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement?.querySelector('img');
    if (previewImg) {
      previewImg.remove();
    }
    
    // 更新按鈕文字
    const uploadBtn = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement?.querySelector('button');
    if (uploadBtn) {
      uploadBtn.textContent = '上傳 圖片';
    }
    
    // 移除移除按鈕
    const removeBtn = document.querySelector(`.element-image-file[data-element-id="${elementId}"]`)?.parentElement?.querySelector('.btn-outline-danger');
    if (removeBtn) {
      removeBtn.remove();
    }
    
    // 更新 component_data
    const field = element.closest('.component-preview')?.querySelector('[data-field]')?.dataset.field;
    if (field && this.selectedComponent) {
      if (field.includes('image') || field.includes('src')) {
        this.selectedComponent.component_data[field] = '';
      }
    }
    
    this.saveState();
  },
  
  rgbToHex(rgb) {
    // 將 rgb/rgba 顏色轉換為 hex
    if (rgb.startsWith('#')) return rgb;
    const match = rgb.match(/\d+/g);
    if (!match || match.length < 3) return '#000000';
    const r = parseInt(match[0]).toString(16).padStart(2, '0');
    const g = parseInt(match[1]).toString(16).padStart(2, '0');
    const b = parseInt(match[2]).toString(16).padStart(2, '0');
    return `#${r}${g}${b}`;
  },


  deleteComponent(index) {
    this.components.splice(index, 1);
    this.saveState(); // 保存狀態到歷史記錄
    this.renderComponents();
    this.selectedComponent = null;
    document.getElementById('propertiesContent').innerHTML = `
      <div class="text-center py-5">
        <i class="bi bi-cursor fs-1 d-block mb-3"></i>
        <p>${this.t('pages.pageEditor.selectComponentToEdit', 'Select a component to edit properties')}</p>
      </div>
    `;
    // 移除 has-form 類（因為沒有表單）
    document.getElementById('propertiesContent').classList.remove('has-form');
  },

  editComponent(index) {
    this.selectComponent(this.components[index]);
  },

  async handleLogoUpload(file) {
    if (!file || !this.selectedComponent || (this.selectedComponent.component_type !== 'nav' && this.selectedComponent.component_type !== 'header')) return;
    
    try {
      const formData = new FormData();
      formData.append('file', file);
      
      const response = await App.apiRequest('/upload', {
        method: 'POST',
        body: formData
      });
      
      if (response.url) {
        this.selectedComponent.component_data.logo = response.url;
        document.getElementById('prop-logo').value = response.url;
        this.saveState();
        this.updateComponentData();
        this.renderProperties(this.selectedComponent);
      }
    } catch (error) {
      App.showError('上傳 Logo 失敗: ' + (error.error || error.message));
    }
  },

  async removeNavLogo() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'nav') return;
    this.selectedComponent.component_data.logo = '';
    if (document.getElementById('prop-logo')) {
      document.getElementById('prop-logo').value = '';
    }
    this.saveState();
    this.updateComponentData();
    this.renderProperties(this.selectedComponent);
  },

  async addMenuItem() {
    if (!this.selectedComponent || (this.selectedComponent.component_type !== 'nav' && this.selectedComponent.component_type !== 'header')) return;
    if (!this.selectedComponent.component_data.menu_items) {
      this.selectedComponent.component_data.menu_items = [];
    }
    this.selectedComponent.component_data.menu_items.push({ text: '', link: '' });
    this.saveState();
    this.renderProperties(this.selectedComponent);
  },

  updateMenuItemLinkType(index, linkType) {
    if (!this.selectedComponent || (this.selectedComponent.component_type !== 'nav' && this.selectedComponent.component_type !== 'header')) return;
    if (this.selectedComponent.component_data.menu_items && this.selectedComponent.component_data.menu_items[index]) {
      if (linkType === 'page') {
        // 切換到頁面選擇模式，清空自訂連結
        this.selectedComponent.component_data.menu_items[index].link = '';
      } else {
        // 切換到自訂連結模式，如果當前是頁面連結則清空
        const currentLink = this.selectedComponent.component_data.menu_items[index].link || '';
        if (currentLink.startsWith('/page/')) {
          this.selectedComponent.component_data.menu_items[index].link = '';
        }
      }
      this.saveState();
      this.renderProperties(this.selectedComponent);
    }
  },

  updateMenuItem(index, field, value) {
    if (!this.selectedComponent || (this.selectedComponent.component_type !== 'nav' && this.selectedComponent.component_type !== 'header')) return;
    if (this.selectedComponent.component_data.menu_items && this.selectedComponent.component_data.menu_items[index]) {
      // Strip /co/{subdomain} prefix from link values so we always store relative paths
      if (field === 'link') {
        value = _stripTenantLinkPrefix(value);
      }
      this.selectedComponent.component_data.menu_items[index][field] = value;
      this.updateComponentData();
    }
  },

  removeMenuItem(index) {
    if (!this.selectedComponent || (this.selectedComponent.component_type !== 'nav' && this.selectedComponent.component_type !== 'header')) return;
    if (this.selectedComponent.component_data.menu_items) {
      this.selectedComponent.component_data.menu_items.splice(index, 1);
      this.saveState();
      this.renderProperties(this.selectedComponent);
    }
  },

  async removeHeaderLogo() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'header') return;
    this.selectedComponent.component_data.logo = '';
    if (document.getElementById('prop-logo')) {
      document.getElementById('prop-logo').value = '';
    }
    this.saveState();
    this.updateComponentData();
    this.renderProperties(this.selectedComponent);
  },

  async handleHeroBgImageUpload(file) {
    if (!file || !this.selectedComponent || this.selectedComponent.component_type !== 'hero') return;
    
    try {
      const formData = new FormData();
      formData.append('file', file);
      
      const response = await App.apiRequest('/upload', {
        method: 'POST',
        body: formData
      });
      
      if (response.url) {
        this.selectedComponent.component_data.background_image = response.url;
        document.getElementById('prop-background-image').value = response.url;
        this.saveState();
        this.updateComponentData();
        this.renderProperties(this.selectedComponent);
      }
    } catch (error) {
      App.showError('上傳背景圖片失敗: ' + (error.error || error.message));
    }
  },

  async removeHeroBgImage() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'hero') return;
    this.selectedComponent.component_data.background_image = '';
    if (document.getElementById('prop-background-image')) {
      document.getElementById('prop-background-image').value = '';
    }
    this.saveState();
    this.updateComponentData();
    this.renderProperties(this.selectedComponent);
  },

  async handleBannerSlideImageUpload(index, file) {
    if (!file || !this.selectedComponent || this.selectedComponent.component_type !== 'banner-slider') return;
    try {
      const formData = new FormData();
      formData.append('file', file);
      const response = await App.apiRequest('/upload', {
        method: 'POST',
        body: formData
      });
      if (response.url) {
        if (!Array.isArray(this.selectedComponent.component_data.slides)) {
          this.selectedComponent.component_data.slides = [];
        }
        if (!this.selectedComponent.component_data.slides[index]) {
          this.selectedComponent.component_data.slides[index] = { image: '', title: '', subtitle: '', button_text: '', button_link: '' };
        }
        this.selectedComponent.component_data.slides[index].image = response.url;
        this.saveState();
        this.updateComponentData();
        this.renderProperties(this.selectedComponent);
      }
    } catch (error) {
      App.showError('上傳 Banner 圖片失敗: ' + (error.error || error.message));
    }
  },

  addBannerSlide() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'banner-slider') return;
    if (!Array.isArray(this.selectedComponent.component_data.slides)) {
      this.selectedComponent.component_data.slides = [];
    }
    this.selectedComponent.component_data.slides.push({
      image: '',
      title: this.t('pages.pageEditor.defaults.bannerSlider.title1', '主打標題'),
      subtitle: this.t('pages.pageEditor.defaults.bannerSlider.subtitle1', '描述文字'),
      button_text: this.t('pages.pageEditor.defaults.bannerSlider.buttonText', '了解更多'),
      button_link: '#'
    });
    this.saveState();
    this.renderProperties(this.selectedComponent);
    this.updateComponentData();
  },

  updateBannerSlide(index, field, value) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'banner-slider') return;
    if (!Array.isArray(this.selectedComponent.component_data.slides)) return;
    if (!this.selectedComponent.component_data.slides[index]) return;
    if (field === 'button_link') {
      value = _stripTenantLinkPrefix(value);
    }
    this.selectedComponent.component_data.slides[index][field] = value;
    this.updateComponentData();
  },

  removeBannerSlide(index) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'banner-slider') return;
    if (!Array.isArray(this.selectedComponent.component_data.slides)) return;
    this.selectedComponent.component_data.slides.splice(index, 1);
    this.saveState();
    this.renderProperties(this.selectedComponent);
    this.updateComponentData();
  },

  async handleImageUpload(file) {
    if (!file || !this.selectedComponent || this.selectedComponent.component_type !== 'image') return;
    
    try {
      const formData = new FormData();
      formData.append('file', file);
      
      const response = await App.apiRequest('/upload', {
        method: 'POST',
        body: formData
      });
      
      if (response.url) {
        this.selectedComponent.component_data.src = response.url;
        document.getElementById('prop-src').value = response.url;
        this.saveState();
        this.updateComponentData();
        this.renderProperties(this.selectedComponent);
      }
    } catch (error) {
      App.showError('上傳圖片失敗: ' + (error.error || error.message));
    }
  },

  async removeImage() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'image') return;
    this.selectedComponent.component_data.src = '';
    if (document.getElementById('prop-src')) {
      document.getElementById('prop-src').value = '';
    }
    this.saveState();
    this.updateComponentData();
    this.renderProperties(this.selectedComponent);
  },

  geocodeMapAddress() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'google-map') return;
    const address = document.getElementById('prop-gmap-address')?.value;
    if (!address || typeof google === 'undefined' || !google.maps) return;
    const geocoder = new google.maps.Geocoder();
    geocoder.geocode({ address }, (results, status) => {
      if (status === 'OK' && results && results[0]) {
        const loc = results[0].geometry.location;
        const latInput = document.getElementById('prop-gmap-lat');
        const lngInput = document.getElementById('prop-gmap-lng');
        if (latInput) latInput.value = loc.lat();
        if (lngInput) lngInput.value = loc.lng();
        this.updateComponentData();
      } else {
        App.showError(this.t('pages.pageEditor.messages.geocodeFailed', 'Could not find this address'));
      }
    });
  },

  async removeFooterLogo() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'footer') return;
    this.selectedComponent.component_data.logo = '';
    if (document.getElementById('prop-logo')) {
      document.getElementById('prop-logo').value = '';
    }
    this.saveState();
    this.updateComponentData();
    this.renderProperties(this.selectedComponent);
  },

  async handleFooterLogoUpload(file) {
    if (!file || !this.selectedComponent || this.selectedComponent.component_type !== 'footer') return;
    
    try {
      const formData = new FormData();
      formData.append('file', file);
      
      const response = await App.apiRequest('/upload', {
        method: 'POST',
        body: formData
      });
      
      if (response.url) {
        this.selectedComponent.component_data.logo = response.url;
        document.getElementById('prop-logo').value = response.url;
        this.saveState();
        this.updateComponentData();
        this.renderProperties(this.selectedComponent);
      }
    } catch (error) {
      App.showError('上傳 Logo 失敗: ' + (error.error || error.message));
    }
  },

  addFooterMenuItem(column) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'footer') return;
    const menuItemsKey = `column${column}_menu_items`;
    if (!this.selectedComponent.component_data[menuItemsKey]) {
      this.selectedComponent.component_data[menuItemsKey] = [];
    }
    this.selectedComponent.component_data[menuItemsKey].push({ text: '', link: '' });
    this.saveState();
    this.renderProperties(this.selectedComponent);
  },

  updateFooterMenuItemLinkType(column, index, linkType) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'footer') return;
    const menuItemsKey = `column${column}_menu_items`;
    if (this.selectedComponent.component_data[menuItemsKey] && this.selectedComponent.component_data[menuItemsKey][index]) {
      if (linkType === 'page') {
        this.selectedComponent.component_data[menuItemsKey][index].link = '';
      } else {
        const currentLink = this.selectedComponent.component_data[menuItemsKey][index].link || '';
        if (currentLink.startsWith('/page/')) {
          this.selectedComponent.component_data[menuItemsKey][index].link = '';
        }
      }
      this.saveState();
      this.renderProperties(this.selectedComponent);
    }
  },

  updateFooterMenuItem(column, index, field, value) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'footer') return;
    const menuItemsKey = `column${column}_menu_items`;
    if (this.selectedComponent.component_data[menuItemsKey] && this.selectedComponent.component_data[menuItemsKey][index]) {
      // Strip /co/{subdomain} prefix from link values so we always store relative paths
      if (field === 'link') {
        value = _stripTenantLinkPrefix(value);
      }
      this.selectedComponent.component_data[menuItemsKey][index][field] = value;
      this.updateComponentData();
    }
  },

  removeFooterMenuItem(column, index) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'footer') return;
    const menuItemsKey = `column${column}_menu_items`;
    if (this.selectedComponent.component_data[menuItemsKey]) {
      this.selectedComponent.component_data[menuItemsKey].splice(index, 1);
      this.saveState();
      this.renderProperties(this.selectedComponent);
    }
  },

  addListMenuItem() {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'list') return;
    if (!this.selectedComponent.component_data.menu_items) {
      this.selectedComponent.component_data.menu_items = [];
    }
    this.selectedComponent.component_data.menu_items.push({ text: '', link: '' });
    this.saveState();
    this.renderProperties(this.selectedComponent);
  },

  updateListMenuItemLinkType(index, linkType) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'list') return;
    if (this.selectedComponent.component_data.menu_items && this.selectedComponent.component_data.menu_items[index]) {
      if (linkType === 'page') {
        this.selectedComponent.component_data.menu_items[index].link = '';
      } else {
        const currentLink = this.selectedComponent.component_data.menu_items[index].link || '';
        if (currentLink.startsWith('/page/')) {
          this.selectedComponent.component_data.menu_items[index].link = '';
        }
      }
      this.saveState();
      this.renderProperties(this.selectedComponent);
    }
  },

  updateListMenuItem(index, field, value) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'list') return;
    if (this.selectedComponent.component_data.menu_items && this.selectedComponent.component_data.menu_items[index]) {
      // Strip /co/{subdomain} prefix from link values so we always store relative paths
      if (field === 'link') {
        value = _stripTenantLinkPrefix(value);
      }
      this.selectedComponent.component_data.menu_items[index][field] = value;
      this.updateComponentData();
    }
  },

  removeListMenuItem(index) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'list') return;
    if (this.selectedComponent.component_data.menu_items) {
      this.selectedComponent.component_data.menu_items.splice(index, 1);
      this.saveState();
      this.renderProperties(this.selectedComponent);
    }
  },

  updateNavIconLinkType(iconType, linkType) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'nav') return;
    const linkKey = `${iconType}_icon_link`;
    if (linkType === 'page') {
      this.selectedComponent.component_data[linkKey] = '';
    } else {
      const currentLink = this.selectedComponent.component_data[linkKey] || '';
      if (currentLink.startsWith('/page/')) {
        this.selectedComponent.component_data[linkKey] = '';
      }
    }
    this.saveState();
    this.renderProperties(this.selectedComponent);
  },

  updateNavIconLink(iconType, value) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'nav') return;
    const linkKey = `${iconType}_icon_link`;
    // Strip /co/{subdomain} prefix so we always store relative paths
    this.selectedComponent.component_data[linkKey] = _stripTenantLinkPrefix(value);
    this.updateComponentData();
  },

  updateHeaderIconLinkType(iconType, linkType) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'header') return;
    const linkKey = `${iconType}_icon_link`;
    if (linkType === 'page') {
      this.selectedComponent.component_data[linkKey] = '';
    } else {
      const currentLink = this.selectedComponent.component_data[linkKey] || '';
      if (currentLink.startsWith('/page/')) {
        this.selectedComponent.component_data[linkKey] = '';
      }
    }
    this.saveState();
    this.renderProperties(this.selectedComponent);
  },

  updateHeaderIconLink(iconType, value) {
    if (!this.selectedComponent || this.selectedComponent.component_type !== 'header') return;
    const linkKey = `${iconType}_icon_link`;
    // Strip /co/{subdomain} prefix so we always store relative paths
    this.selectedComponent.component_data[linkKey] = _stripTenantLinkPrefix(value);
    this.updateComponentData();
  },

  moveColumnChildUp(sectionIndex, columnIndex, childIndex) {
    const section = this.components[sectionIndex];
    if (!section || section.component_type !== 'section') return;
    
    if (section.component_data.column_children && Array.isArray(section.component_data.column_children)) {
      const columnChildren = section.component_data.column_children[columnIndex];
      if (Array.isArray(columnChildren) && childIndex > 0) {
        const temp = columnChildren[childIndex];
        columnChildren[childIndex] = columnChildren[childIndex - 1];
        columnChildren[childIndex - 1] = temp;
        this.saveState();
        this.renderComponents();
        // 重新選中移動後的元件
        setTimeout(() => {
          this.editColumnChild(sectionIndex, columnIndex, childIndex - 1);
        }, 100);
      }
    }
  },

  moveColumnChildDown(sectionIndex, columnIndex, childIndex) {
    const section = this.components[sectionIndex];
    if (!section || section.component_type !== 'section') return;
    
    if (section.component_data.column_children && Array.isArray(section.component_data.column_children)) {
      const columnChildren = section.component_data.column_children[columnIndex];
      if (Array.isArray(columnChildren) && childIndex < columnChildren.length - 1) {
        const temp = columnChildren[childIndex];
        columnChildren[childIndex] = columnChildren[childIndex + 1];
        columnChildren[childIndex + 1] = temp;
        this.saveState();
        this.renderComponents();
        // 重新選中移動後的元件
        setTimeout(() => {
          this.editColumnChild(sectionIndex, columnIndex, childIndex + 1);
        }, 100);
      }
    }
  },

  editColumnChild(sectionIndex, columnIndex, childIndex) {
    const section = this.components[sectionIndex];
    if (!section || section.component_type !== 'section') return;
    
    // 先清除所有栏内元件的选中状态
    document.querySelectorAll('.column-child-item').forEach(item => {
      item.style.outline = '';
      item.style.outlineOffset = '';
    });
    
    // 清除主元件的选中状态
    document.querySelectorAll('.component-item').forEach(item => {
      item.classList.remove('selected');
    });
    
    if (section.component_data.column_children && Array.isArray(section.component_data.column_children)) {
      const columnChildren = section.component_data.column_children[columnIndex];
      if (Array.isArray(columnChildren) && childIndex >= 0 && childIndex < columnChildren.length) {
        const child = columnChildren[childIndex];
        // 直接选中这个子元件（它可能不在主 components 数组中）
        this.selectedComponent = child;
        this.renderProperties(child);
        
        // 高亮显示对应的栏位内元件
        setTimeout(() => {
          const childItem = document.querySelector(`.column-child-item[data-section-index="${sectionIndex}"][data-column-index="${columnIndex}"][data-child-index="${childIndex}"]`);
          if (childItem) {
            childItem.style.outline = '2px solid #0d6efd';
            childItem.style.outlineOffset = '2px';
            childItem.scrollIntoView({ behavior: 'smooth', block: 'center' });
          }
        }, 100);
      }
    }
  },

  removeChildFromSectionColumn(sectionIndex, columnIndex, childIndex) {
    const section = this.components[sectionIndex];
    if (!section || section.component_type !== 'section') return;
    
    // 支持新的 column_children 结构
    if (section.component_data.column_children && Array.isArray(section.component_data.column_children)) {
      if (columnIndex !== undefined && columnIndex !== null && 
          section.component_data.column_children[columnIndex] && 
          Array.isArray(section.component_data.column_children[columnIndex])) {
        const columnChildren = section.component_data.column_children[columnIndex];
        if (childIndex !== undefined && childIndex !== null && childIndex >= 0 && childIndex < columnChildren.length) {
          columnChildren.splice(childIndex, 1);
          this.saveState();
          this.renderComponents();
        }
      }
    }
  },

  removeChildFromSection(sectionIndex, columnIndex, childIndex) {
    const section = this.components[sectionIndex];
    if (!section || section.component_type !== 'section') return;
    
    let child = null;
    
    // 支持新的 column_children 结构
    if (section.component_data.column_children && Array.isArray(section.component_data.column_children)) {
      if (columnIndex !== undefined && columnIndex !== null) {
        const columnChildren = section.component_data.column_children[columnIndex];
        if (Array.isArray(columnChildren) && childIndex !== undefined && childIndex !== null) {
          child = columnChildren[childIndex];
      if (child) {
            columnChildren.splice(childIndex, 1);
          }
        }
      }
    }
    // 兼容旧的 children 结构
    else if (section.component_data.children && Array.isArray(section.component_data.children)) {
      if (childIndex !== undefined && childIndex !== null) {
        child = section.component_data.children[childIndex];
        if (child) {
        section.component_data.children.splice(childIndex, 1);
        }
      }
    }
    
    if (child) {
      // 將子元件移回主元件列表
        child.sort_order = this.components.length;
        this.components.push(child);
        
        // 更新所有元件的 sort_order
        this.components.forEach((comp, i) => {
          comp.sort_order = i;
        });
        
        this.saveState();
        this.renderComponents();
        this.selectComponent(section);
    }
  },

  moveComponentUp(index) {
    if (index === 0) return;
    
    // 交換位置
    const temp = this.components[index];
    this.components[index] = this.components[index - 1];
    this.components[index - 1] = temp;
    
    // 更新 sort_order
    this.components.forEach((comp, i) => {
      comp.sort_order = i;
    });
    
    this.saveState(); // 保存狀態到歷史記錄
    this.renderComponents();
    
    // 重新選中移動後的元件
    const movedComponent = this.components[index - 1];
    this.selectComponent(movedComponent);
    
    // 檢查元件是否在屏幕內，如果不在則滾動到該元件
    setTimeout(() => {
      const container = document.getElementById('componentsContainer');
      const componentItems = container.querySelectorAll('.component-item');
      if (componentItems[index - 1]) {
        const rect = componentItems[index - 1].getBoundingClientRect();
        const viewArea = document.querySelector('.editor-view-area');
        const viewAreaRect = viewArea.getBoundingClientRect();
        
        // 檢查元件是否在可見區域內
        if (rect.top < viewAreaRect.top || rect.bottom > viewAreaRect.bottom) {
          componentItems[index - 1].scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }
    }, 50);
  },

  moveComponentDown(index) {
    if (index === this.components.length - 1) return;
    
    // 交換位置
    const temp = this.components[index];
    this.components[index] = this.components[index + 1];
    this.components[index + 1] = temp;
    
    // 更新 sort_order
    this.components.forEach((comp, i) => {
      comp.sort_order = i;
    });
    
    this.saveState(); // 保存狀態到歷史記錄
    this.renderComponents();
    
    // 重新選中移動後的元件
    const movedComponent = this.components[index + 1];
    this.selectComponent(movedComponent);
    
    // 檢查元件是否在屏幕內，如果不在則滾動到該元件
    setTimeout(() => {
      const container = document.getElementById('componentsContainer');
      const componentItems = container.querySelectorAll('.component-item');
      if (componentItems[index + 1]) {
        const rect = componentItems[index + 1].getBoundingClientRect();
        const viewArea = document.querySelector('.editor-view-area');
        const viewAreaRect = viewArea.getBoundingClientRect();
        
        // 檢查元件是否在可見區域內
        if (rect.top < viewAreaRect.top || rect.bottom > viewAreaRect.bottom) {
          componentItems[index + 1].scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }
    }, 50);
  },

  toggleProductListFullList() {
    const isFullList = document.getElementById('prop-full-list')?.checked === true;
    const productTypeFilterContainer = document.getElementById('product-type-filter-container');
    const brandFilterContainer = document.getElementById('brand-filter-container');
    const limitLabel = document.querySelector('label[for="prop-limit"]');
    
    if (isFullList) {
      // 显示筛选选项
      if (productTypeFilterContainer) productTypeFilterContainer.style.display = 'block';
      if (brandFilterContainer) brandFilterContainer.style.display = 'block';
      if (limitLabel) limitLabel.textContent = this.t('pages.pageEditor.productList.perPageCount', 'Per page');
    } else {
      // 隐藏筛选选项
      if (productTypeFilterContainer) productTypeFilterContainer.style.display = 'none';
      if (brandFilterContainer) brandFilterContainer.style.display = 'none';
      if (limitLabel) limitLabel.textContent = this.t('pages.pageEditor.productList.displayCount', 'Display count');
    }
  },

  getComponentTypeLabel(type) {
    const map = {
      header: 'header',
      nav: 'nav',
      hero: 'hero',
      text: 'text',
      image: 'image',
      button: 'button',
      section: 'section',
      heading: 'heading',
      'product-list': 'productList',
      'service-list': 'serviceList',
      footer: 'footer',
      'order-list': 'orderList',
      'blog-list': 'blogList',
      'contact-form': 'contactForm',
      'service-booking': 'serviceBooking',
      'login-register': 'loginRegister',
      cart: 'cart',
      checkout: 'checkout',
      'user-area': 'userArea',
      list: 'verticalMenu',
      'product-detail': 'productDetail',
      'banner-slider': 'bannerSlider',
      'dining-menu': 'diningMenu',
      'dining-table-reservation': 'diningReservation',
      'google-map': 'googleMap',
      'custom-html': 'customHtml'
    };
    const suffix = map[type];
    if (!suffix) return type;
    return this.t(`pages.pageEditor.components.${suffix}`, type);
  },

  showPageSettings() {
    // 填充 modal 中的數據
    const nameInput = document.getElementById('modalPageName');
    const slugInput = document.getElementById('modalPageSlug');
    const statusSelect = document.getElementById('modalPageStatus');
    
    if (!nameInput || !slugInput || !statusSelect) {
      App.showError('無法找到頁面設定表單元素');
      return;
    }
    
    if (this.pageData) {
      nameInput.value = this.pageData.name || '';
      slugInput.value = this.pageData.slug || '';
      statusSelect.value = this.pageData.status || 'draft';
    } else {
      nameInput.value = '';
      slugInput.value = '';
      statusSelect.value = 'draft';
    }
    
    // 綁定 slug 自動生成事件（使用 once 選項，每次顯示 modal 時重新綁定）
    // 先移除舊的事件監聽器（通過克隆節點）
    const nameInputClone = nameInput.cloneNode(true);
    nameInput.parentNode.replaceChild(nameInputClone, nameInput);
    const slugInputClone = slugInput.cloneNode(true);
    slugInput.parentNode.replaceChild(slugInputClone, slugInput);
    
    // 重新獲取元素
    const freshNameInput = document.getElementById('modalPageName');
    const freshSlugInput = document.getElementById('modalPageSlug');
    
    if (freshNameInput && freshSlugInput) {
      // 綁定 slug 自動生成
      freshNameInput.addEventListener('input', (e) => {
        if (!freshSlugInput.value || freshSlugInput.dataset.autoGenerated === 'true') {
          const slug = e.target.value
            .toLowerCase()
            .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
            .replace(/^-+|-+$/g, '');
          freshSlugInput.value = slug;
          freshSlugInput.dataset.autoGenerated = 'true';
        }
      });

      // 手動編輯 slug 時取消自動生成
      freshSlugInput.addEventListener('input', (e) => {
        e.target.dataset.autoGenerated = 'false';
      });
    }
    
    // 顯示 modal
    const modal = new bootstrap.Modal(document.getElementById('pageSettingsModal'));
    modal.show();
  },

  toggleContainerWidthInput() {
    const containerWidthSelect = document.getElementById('modalContainerWidth');
    const customWidthContainer = document.getElementById('customWidthContainer');
    
    if (containerWidthSelect && customWidthContainer) {
      if (containerWidthSelect.value === 'custom') {
        customWidthContainer.style.display = 'block';
      } else {
        customWidthContainer.style.display = 'none';
      }
    }
  },

  showStyleSettings() {
    const defaultTitleColorInput = document.getElementById('styleDefaultTitleColor');
    const defaultContentColorInput = document.getElementById('styleDefaultContentColor');
    const containerWidthSelect = document.getElementById('styleContainerWidth');
    const customWidthInput = document.getElementById('styleCustomWidth');
    
    if (this.pageData) {
      // 載入樣式設定
      if (defaultTitleColorInput) {
        defaultTitleColorInput.value = this.pageData.default_title_color || '#212529';
      }
      if (defaultContentColorInput) {
        defaultContentColorInput.value = this.pageData.default_content_color || '#212529';
      }
      
      // 載入容器寬度設定
      if (containerWidthSelect && this.pageData.container_width) {
        if (this.pageData.container_width === 'full') {
          containerWidthSelect.value = 'full';
        } else if (this.pageData.container_width === 'custom') {
          containerWidthSelect.value = 'custom';
          if (customWidthInput) {
            customWidthInput.value = this.pageData.custom_width || 1200;
            document.getElementById('styleCustomWidthContainer').style.display = 'block';
          }
        } else {
          containerWidthSelect.value = this.pageData.container_width || '1200';
        }
      } else if (containerWidthSelect) {
        containerWidthSelect.value = '1200';
      }
    } else {
      if (defaultTitleColorInput) defaultTitleColorInput.value = '#212529';
      if (defaultContentColorInput) defaultContentColorInput.value = '#212529';
      if (containerWidthSelect) containerWidthSelect.value = '1200';
      if (customWidthInput) customWidthInput.value = 1200;
    }
    
    // 觸發容器寬度輸入框顯示/隱藏
    this.toggleStyleContainerWidthInput();
    
    // 顯示 modal
    const modalElement = document.getElementById('styleSettingsModal');
    if (!modalElement) {
      App.showError('無法找到樣式設定 Modal');
      return;
    }
    
    let modal = bootstrap.Modal.getInstance(modalElement);
    if (!modal) {
      modal = new bootstrap.Modal(modalElement);
    }
    modal.show();
  },

  toggleStyleContainerWidthInput() {
    const containerWidthSelect = document.getElementById('styleContainerWidth');
    const customWidthContainer = document.getElementById('styleCustomWidthContainer');
    
    if (containerWidthSelect && customWidthContainer) {
      if (containerWidthSelect.value === 'custom') {
        customWidthContainer.style.display = 'block';
      } else {
        customWidthContainer.style.display = 'none';
      }
    }
  },

  saveStyleSettings() {
    const defaultTitleColorInput = document.getElementById('styleDefaultTitleColor');
    const defaultContentColorInput = document.getElementById('styleDefaultContentColor');
    const containerWidthSelect = document.getElementById('styleContainerWidth');
    const customWidthInput = document.getElementById('styleCustomWidth');
    
    // 驗證自訂寬度
    let containerWidth = containerWidthSelect?.value || '1200';
    if (containerWidth === 'custom') {
      const customWidth = parseInt(customWidthInput?.value || '1200');
      if (isNaN(customWidth) || customWidth < 100 || customWidth > 2000) {
        App.showError('自訂寬度必須在 100-2000px 之間');
        return;
      }
    }

    // 更新本地數據
    if (!this.pageData) {
      this.pageData = {};
    }
    
    // 保存樣式設定
    if (defaultTitleColorInput) {
      this.pageData.default_title_color = defaultTitleColorInput.value;
    }
    if (defaultContentColorInput) {
      this.pageData.default_content_color = defaultContentColorInput.value;
    }
    
    // 保存容器寬度設定
    if (containerWidthSelect) {
      this.pageData.container_width = containerWidth;
      if (containerWidth === 'custom' && customWidthInput) {
        this.pageData.custom_width = parseInt(customWidthInput.value);
      }
    }
    
    // 應用容器寬度到 componentsContainer
    this.applyContainerWidth();
    
    // 關閉 modal
    const modal = bootstrap.Modal.getInstance(document.getElementById('styleSettingsModal'));
    if (modal) {
      modal.hide();
    }
    
    App.showSuccess('樣式設定已更新');
  },

  savePageSettings() {
    // 驗證必填字段
    const nameInput = document.getElementById('modalPageName');
    const slugInput = document.getElementById('modalPageSlug');
    const statusSelect = document.getElementById('modalPageStatus');
    
    if (!nameInput || !slugInput || !statusSelect) {
      App.showError('無法找到頁面設定表單元素');
      return;
    }
    
    const name = nameInput.value.trim();
    const slug = slugInput.value.trim();
    
    if (!name) {
      App.showError('請輸入頁面名稱');
      return;
    }
    if (!slug) {
      App.showError('請輸入網址路徑');
      return;
    }
    
    // 驗證自訂寬度
    let containerWidth = containerWidthSelect?.value || '1200';
    if (containerWidth === 'custom') {
      const customWidth = parseInt(customWidthInput?.value || '1200');
      if (isNaN(customWidth) || customWidth < 100 || customWidth > 2000) {
        App.showError('自訂寬度必須在 100-2000px 之間');
        return;
      }
    }

    // 更新本地數據
    if (!this.pageData) {
      this.pageData = {};
    }
    this.pageData.name = name;
    this.pageData.slug = slug;
    this.pageData.status = statusSelect.value;
    
    // 保存樣式設定
    if (defaultTitleColorInput) {
      this.pageData.default_title_color = defaultTitleColorInput.value;
    }
    if (defaultContentColorInput) {
      this.pageData.default_content_color = defaultContentColorInput.value;
    }
    
    // 保存容器寬度設定
    if (containerWidthSelect) {
      this.pageData.container_width = containerWidth;
      if (containerWidth === 'custom' && customWidthInput) {
        this.pageData.custom_width = parseInt(customWidthInput.value);
      }
    }
    
    // 應用容器寬度到 componentsContainer
    this.applyContainerWidth();
    
    // 關閉 modal
    const modal = bootstrap.Modal.getInstance(document.getElementById('pageSettingsModal'));
    if (modal) {
      modal.hide();
    }
    
    // 更新預覽網址
    this.updatePreviewUrl();
    
    App.showSuccess('頁面設定已更新');
  },
  
  applyContainerWidth() {
    const container = document.getElementById('componentsContainer');
    if (!container || !this.pageData) return;
    
    const containerWidth = this.pageData.container_width || '1200';
    
    if (containerWidth === 'full') {
      container.style.maxWidth = '100%';
    } else if (containerWidth === 'custom') {
      const customWidth = this.pageData.custom_width || 1200;
      container.style.maxWidth = `${customWidth}px`;
    } else {
      container.style.maxWidth = '1200px';
    }
  },

  showLoading() {
    const overlay = document.getElementById('pageEditorLoading');
    if (overlay) overlay.classList.remove('d-none');
  },

  hideLoading() {
    const overlay = document.getElementById('pageEditorLoading');
    if (overlay) overlay.classList.add('d-none');
  },

  async save() {
    try {
      // 驗證必填字段（從 pageData 獲取）
      if (!this.pageData || !this.pageData.name || !this.pageData.slug) {
        App.showError('請先設定頁面名稱和網址路徑');
        this.showPageSettings();
        return;
      }
      
      const name = this.pageData.name.trim();
      const slug = this.pageData.slug.trim();

      const pageData = {
        name: name,
        slug: slug,
        status: this.pageData.status || 'draft'
      };
      
      // 保留 is_homepage 字段（如果存在）
      if (this.pageData.hasOwnProperty('is_homepage')) {
        pageData.is_homepage = this.pageData.is_homepage;
      }

      if (this.isNew) {
        // 新建模式：先創建頁面
        const newPage = await App.apiRequest('/pages', {
          method: 'POST',
          body: JSON.stringify(pageData)
        });
        
        this.pageId = newPage.id;
        this.isNew = false;
        this.pageData = newPage;
        
        // 更新 URL（不刷新頁面）
        window.history.replaceState({}, '', `/pages/${this.pageId}/edit`);
        document.getElementById('pageEditorPage').dataset.pageId = this.pageId;
        document.getElementById('pageEditorTitle').textContent = '編輯頁面';
        
        // 啟用預覽按鈕
        document.getElementById('previewBtn').disabled = false;
        document.getElementById('previewBtn').title = '';
        
        // 重新載入頁面列表並更新導航按鈕
      } else {
        // 編輯模式：更新頁面
        await App.apiRequest(`/pages/${this.pageId}`, {
          method: 'PUT',
          body: JSON.stringify(pageData)
        });
        
        // 更新本地數據（保留所有原有字段，包括 is_homepage）
        this.pageData = { ...this.pageData, ...pageData };
      }
      
      // 保存元件
      await App.apiRequest(`/pages/${this.pageId}/components`, {
        method: 'PUT',
        body: JSON.stringify({ components: this.components })
      });
      
      // If we were editing a linked block, save the block data too
      if (this.editingBlockId) {
        await this.saveLinkedBlock();
      }
      
      // 更新預覽網址
      this.updatePreviewUrl();
      
      // 顯示保存成功 modal
      this.showSaveSuccessModal();
    } catch (error) {
      App.showError(this.t('common.saveError', 'Save failed') + ': ' + (error.error || error.message));
    }
  },

  showSaveSuccessModal() {
    const modal = new bootstrap.Modal(document.getElementById('saveSuccessModal'));
    const previewUrlInput = document.getElementById('saveSuccessPreviewUrl');
    const openPreviewUrlBtn = document.getElementById('openPreviewUrlBtn');
    const copyPreviewUrlBtn = document.getElementById('copyPreviewUrlBtn');
    const statusInfo = document.getElementById('saveSuccessStatusInfo');
    
    if (!this.pageData || !this.pageData.slug) {
      App.showError(this.t('pages.pageEditor.cannotGetPageInfo', 'Cannot get page info'));
      return;
    }
    
    // 獲取租戶子域名
    const tenantSubdomain = document.body.dataset.tenantSubdomain || 'test';
    const previewUrl = `/co/${tenantSubdomain}/${this.pageData.slug}/`;
    const fullUrl = window.location.origin + previewUrl;
    
    // 設置預覽連結
    if (previewUrlInput) {
      previewUrlInput.value = fullUrl;
    }
    
    // 設置打開連結按鈕
    if (openPreviewUrlBtn) {
      openPreviewUrlBtn.href = previewUrl;
    }
    
    // 設置狀態提示
    if (statusInfo) {
      if (this.pageData.status === 'published') {
        statusInfo.textContent = this.t('pages.pageEditor.publishedHint', 'Page status is Published. Everyone can access it.');
        statusInfo.parentElement.classList.remove('alert-info');
        statusInfo.parentElement.classList.add('alert-success');
      } else {
        statusInfo.textContent = this.t('pages.pageEditor.draftOnlyHint', 'Page status is Draft. Only you can see it. To make it public, change status to Published in Page Settings.');
        statusInfo.parentElement.classList.remove('alert-success');
        statusInfo.parentElement.classList.add('alert-info');
      }
    }
    
    // 複製連結功能
    if (copyPreviewUrlBtn) {
      copyPreviewUrlBtn.addEventListener('click', () => {
        previewUrlInput.select();
        document.execCommand('copy');
        copyPreviewUrlBtn.innerHTML = '<i class="bi bi-check"></i>';
        setTimeout(() => {
          copyPreviewUrlBtn.innerHTML = '<i class="bi bi-clipboard"></i>';
        }, 2000);
      });
    }
    
    modal.show();
  },

  // 保存元件為區塊
  saveComponentAsBlock(componentIndex) {
    if (componentIndex === undefined || componentIndex === null) {
      App.showError(this.t('pages.pageEditor.selectComponentToSaveBlock', 'Please select a component to save as a block'));
      return;
    }
    
    const component = this.components[componentIndex];
    if (!component) {
      App.showError(this.t('pages.pageEditor.componentNotFound', 'Component not found'));
      return;
    }
    
    // 保存當前元件索引
    this.savingComponentIndex = componentIndex;
    
    // 顯示 modal
    const modal = new bootstrap.Modal(document.getElementById('saveBlockModal'));
    document.getElementById('blockName').value = '';
    modal.show();
  },

  confirmSaveBlock() {
    const blockName = document.getElementById('blockName').value.trim();
    if (!blockName) {
      App.showError(this.t('pages.pageEditor.enterBlockName', 'Please enter block name'));
      return;
    }
    
    if (this.savingComponentIndex === undefined || this.savingComponentIndex === null) {
      App.showError('元件不存在');
      return;
    }
    
    const component = this.components[this.savingComponentIndex];
    if (!component) {
      App.showError('元件不存在');
      return;
    }
    
    // 保存區塊到數據庫
    this.saveBlockToDatabase(blockName, component);
  },

  async saveBlockToDatabase(blockName, component) {
    // 防止重复保存
    if (this.isSavingBlock) {
      return;
    }
    this.isSavingBlock = true;
    
    try {
      const blockData = {
        name: blockName,
        component_type: component.component_type,
        component_data: component.component_data
      };
      
      await App.apiRequest('/blocks', {
        method: 'POST',
        body: JSON.stringify(blockData)
      });
      
      App.showSuccess(this.t('pages.pageEditor.blockSaved', 'Block saved'));
      
      // 關閉 modal
      const modal = bootstrap.Modal.getInstance(document.getElementById('saveBlockModal'));
      if (modal) {
        modal.hide();
      }
      
      // 清除保存状态
      this.savingComponentIndex = null;
      
      // 重新載入區塊庫
      if (document.getElementById('blockLibraryPanel').style.display !== 'none') {
        this.loadBlocks();
      }
    } catch (error) {
      App.showError(this.t('pages.pageEditor.saveBlockFailed', 'Failed to save block') + ': ' + (error.error || error.message));
    } finally {
      this.isSavingBlock = false;
    }
  },

  async deleteBlock(blockId) {
    try {
      await App.apiRequest(`/blocks/${blockId}`, {
        method: 'DELETE'
      });
      
      App.showSuccess(this.t('pages.pageEditor.blockDeleted', 'Block deleted'));
      
      // 重新載入區塊庫
      this.loadBlocks();
    } catch (error) {
      App.showError(this.t('pages.pageEditor.deleteBlockFailed', 'Failed to delete block') + ': ' + (error.error || error.message));
    }
  },

  showBlockLibrary() {
    // 先關閉元件庫
    this.hideComponentLibrary();
    
    const blockLibraryPanel = document.getElementById('blockLibraryPanel');
    const blockLibraryIcon = document.getElementById('blockLibraryIcon');
    
    if (blockLibraryPanel && blockLibraryIcon) {
      // 獲取按鈕的位置
      const iconRect = blockLibraryIcon.getBoundingClientRect();
      
      // 設置 panel 的位置，使其跟隨按鈕
      blockLibraryPanel.style.left = `${iconRect.right}px`;
      blockLibraryPanel.style.top = `${iconRect.top}px`;
      blockLibraryPanel.style.maxHeight = `calc(100vh - ${iconRect.top}px)`;
      blockLibraryPanel.style.display = 'block';
      
      // 載入區塊列表
      this.loadBlocks();
    }
  },

  hideBlockLibrary() {
    const blockLibraryPanel = document.getElementById('blockLibraryPanel');
    if (blockLibraryPanel) {
      blockLibraryPanel.style.display = 'none';
    }
  },

  async loadBlocks() {
    const blockLibrary = document.getElementById('blockLibrary');
    if (!blockLibrary) return;
    
    try {
      blockLibrary.innerHTML = '<div class="text-center text-muted py-3"><i class="bi bi-inbox fs-4 d-block mb-2"></i><small>' + this.t('common.loading', 'Loading...') + '</small></div>';
      
      const response = await App.apiRequest('/blocks');
      const blocks = response.data || [];
      
      if (blocks.length === 0) {
        blockLibrary.innerHTML = '<div class="text-center text-muted py-3"><i class="bi bi-inbox fs-4 d-block mb-2"></i><small>' + this.t('pages.pageEditor.noBlocks', 'No blocks yet') + '</small></div>';
        return;
      }
      
      blockLibrary.innerHTML = blocks.map(block => `
        <div class="position-relative d-inline-block w-100 mb-2">
          <div class="d-flex gap-1">
            <button class="btn btn-outline-primary btn-sm block-add-btn flex-grow-1 text-start" data-block-id="${block.id}" title="${block.name}">
              <i class="bi bi-box"></i> ${block.name}
            </button>
            <button class="btn btn-sm btn-outline-secondary block-edit-btn" data-block-id="${block.id}" title="${this.t('pages.pageEditor.editName', 'Edit name')}">
              <i class="bi bi-pencil"></i>
            </button>
            <button class="btn btn-sm btn-outline-danger block-delete-btn" data-block-id="${block.id}" data-block-name="${block.name}" title="${this.t('pages.pageEditor.deleteBlock', 'Delete block')}" style="background-color: white !important;">
              <i class="bi bi-trash"></i>
            </button>
          </div>
        </div>
      `).join('');
      
      // 綁定區塊添加按鈕
      blockLibrary.querySelectorAll('.block-add-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
          // 如果点击的是删除或编辑按钮，不触发添加
          if (e.target.closest('.block-delete-btn') || e.target.closest('.block-edit-btn') || e.target.closest('.block-edit-name-btn') || e.target.closest('.block-cancel-edit-btn')) {
            return;
          }
          e.stopPropagation();
          const blockId = btn.dataset.blockId;
          await this.addBlockToPage(blockId);
          this.hideBlockLibrary();
        });
      });
      
      // 綁定區塊編輯名稱按鈕
      blockLibrary.querySelectorAll('.block-edit-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
          e.stopPropagation();
          e.preventDefault();
          const blockId = btn.dataset.blockId;
          const blockItem = btn.closest('.position-relative');
          const blockAddBtn = blockItem.querySelector('.block-add-btn');
          // 提取区块名称（移除图标和空格）
          const blockName = blockAddBtn ? blockAddBtn.textContent.trim().replace(/^.*? /, '').trim() : '';
          
          // 顯示編輯 modal
          const modal = new bootstrap.Modal(document.getElementById('editBlockNameModal'));
          const nameInput = document.getElementById('editBlockNameInput');
          const saveBtn = document.getElementById('saveBlockNameBtn');
          
          if (nameInput) {
            nameInput.value = blockName;
            nameInput.dataset.blockId = blockId;
            
            // 綁定保存按鈕（先移除舊的事件監聽器）
            const newSaveBtn = saveBtn.cloneNode(true);
            saveBtn.parentNode.replaceChild(newSaveBtn, saveBtn);
            
            newSaveBtn.addEventListener('click', async () => {
              const newName = nameInput.value.trim();
              if (!newName) {
                App.showError('區塊名稱不能為空');
                return;
              }
              
              try {
                await App.apiRequest(`/blocks/${blockId}`, {
                  method: 'PUT',
                  body: JSON.stringify({ name: newName })
                });
                
                App.showSuccess('區塊名稱已更新');
                
                // 關閉 modal
                const modalInstance = bootstrap.Modal.getInstance(document.getElementById('editBlockNameModal'));
                if (modalInstance) {
                  modalInstance.hide();
                }
                
                // 重新載入區塊庫
                this.loadBlocks();
              } catch (error) {
                App.showError('更新區塊名稱失敗: ' + (error.error || error.message));
              }
            });
          }
          
          modal.show();
        });
      });
      
      // 綁定區塊刪除按鈕
      blockLibrary.querySelectorAll('.block-delete-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
          e.stopPropagation();
          e.preventDefault();
          const blockId = btn.dataset.blockId;
          const blockName = btn.dataset.blockName || this.t('pages.pageEditor.thisBlock', 'this block');
          
          if (confirm(this.t('pages.pageEditor.confirmDeleteBlock', 'Are you sure you want to delete block \"{name}\"? This cannot be undone.').replace('{name}', blockName))) {
            await this.deleteBlock(blockId);
          }
        });
      });
    } catch (error) {
      blockLibrary.innerHTML = '<div class="text-center text-danger py-3"><small>' + this.t('common.loadError', 'Load failed') + ': ' + (error.error || error.message) + '</small></div>';
    }
  },

  async addBlockToPage(blockId) {
    try {
      const block = await App.apiRequest(`/blocks/${blockId}`);
      
      // 創建新元件，使用區塊的數據並保留 block_id 引用（linked block）
      const newComponent = {
        id: null,
        block_id: block.id,
        component_type: block.component_type,
        component_data: block.component_data,
        sort_order: this.components.length,
        is_active: true
      };
      
      // 檢查是否要添加到區塊容器的欄位
      if (this.addComponentToSectionColumn) {
        const { sectionIndex, columnIndex } = this.addComponentToSectionColumn;
        const section = this.components[sectionIndex];
        
        if (section && section.component_type === 'section') {
          if (!section.component_data.column_children) {
            const columns = section.component_data.columns || 1;
            section.component_data.column_children = Array(columns).fill(0).map(() => []);
          }
          
          if (section.component_data.column_children[columnIndex]) {
            section.component_data.column_children[columnIndex].push(newComponent);
            this.saveState();
            this.renderComponents();
            this.cancelAddComponentAfter();
            return;
          }
        }
      }
      
      // 添加到指定位置或末尾
      if (this.addComponentAfterIndex !== null) {
        this.components.splice(this.addComponentAfterIndex + 1, 0, newComponent);
        this.components.forEach((comp, i) => {
          comp.sort_order = i;
        });
      } else {
        this.components.push(newComponent);
      }
      
      this.saveState();
      this.renderComponents();
      this.selectComponent(newComponent);
      
      // 滚动到新添加的元件
      setTimeout(() => {
        const index = this.components.indexOf(newComponent);
        const item = document.querySelector(`.component-item[data-index="${index}"]`);
        if (item) {
          item.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      }, 150);
      
      // 取消添加模式
      if (this.addComponentAfterIndex !== null) {
        this.cancelAddComponentAfter();
      }
    } catch (error) {
      App.showError('添加區塊失敗: ' + (error.error || error.message));
    }
  }
};

// Unlink a component from its referenced block (make it an independent copy)
PageEditor.unlinkBlock = function(index) {
  const comp = this.components[index];
  if (!comp || !comp.block_id) return;

  if (!confirm(this.t('pages.pageEditor.confirmUnlink', 'Unlink this component from the shared block? It will become an independent copy and no longer receive updates from the block.'))) {
    return;
  }

  delete comp.block_id;
  this.saveState();
  this.renderComponents();
  App.showSuccess(this.t('pages.pageEditor.unlinked', 'Component unlinked from block'));
};

// Edit the source block that a linked component references.
// Opens a modal where the user can edit the block's component_data directly,
// and saves changes back to the block (affecting all pages that reference it).
PageEditor.editLinkedBlock = async function(blockId) {
  try {
    const block = await App.apiRequest(`/blocks/${blockId}`);
    if (!block) {
      App.showError('Block not found');
      return;
    }

    // Find the component referencing this block so we can update its local preview
    const comp = this.components.find(c => c.block_id === blockId);

    // Use the existing edit flow: temporarily select a virtual component with the block data,
    // but we intercept save to PUT to the block endpoint.
    // Simpler approach: update the block data via the current properties panel.
    // When user clicks "Edit Block", we:
    // 1. Set the selectedComponent to the linked component
    // 2. Mark a flag so that property changes go to the block via API
    // 3. On property change, also update local component_data for preview

    this.editingBlockId = blockId;
    this.editingBlockName = block.name;

    if (comp) {
      // Sync local data with block's latest data
      comp.component_type = block.component_type;
      comp.component_data = { ...block.component_data };
      this.selectedComponent = comp;
      this.renderComponents();
      this.renderProperties(comp);
    }

    App.showSuccess(this.t('pages.pageEditor.editingBlock', 'Now editing block: ') + block.name + ' — ' + this.t('pages.pageEditor.editBlockReminder', 'Remember to save when done'));
  } catch (error) {
    App.showError('Failed to load block: ' + (error.error || error.message));
  }
};

// Save the currently-editing linked block's data back to the server
PageEditor.saveLinkedBlock = async function() {
  if (!this.editingBlockId || !this.selectedComponent) return;

  try {
    await App.apiRequest(`/blocks/${this.editingBlockId}`, {
      method: 'PUT',
      body: JSON.stringify({
        component_type: this.selectedComponent.component_type,
        component_data: this.selectedComponent.component_data
      })
    });

    App.showSuccess(this.t('pages.pageEditor.blockUpdated', 'Block updated — all pages using this block will reflect the changes'));
    this.editingBlockId = null;
    this.editingBlockName = null;
  } catch (error) {
    App.showError('Failed to update block: ' + (error.error || error.message));
  }
};

// 扩展 PageEditor 对象 - 将 showPageSettings 和 savePageSettings 添加到 PageEditor
PageEditor.showPageSettings = function() {
    // 填充 modal 中的數據
    const nameInput = document.getElementById('modalPageName');
    const slugInput = document.getElementById('modalPageSlug');
    const statusSelect = document.getElementById('modalPageStatus');
    
    if (!nameInput || !slugInput || !statusSelect) {
      App.showError('無法找到頁面設定表單元素');
      return;
    }
    
    if (this.pageData) {
      nameInput.value = this.pageData.name || '';
      slugInput.value = this.pageData.slug || '';
      statusSelect.value = this.pageData.status || 'draft';
    } else {
      nameInput.value = '';
      slugInput.value = '';
      statusSelect.value = 'draft';
    }
    
    // 綁定 slug 自動生成事件（使用 once 選項，每次顯示 modal 時重新綁定）
    // 先移除舊的事件監聽器（通過克隆節點）
    const nameInputClone = nameInput.cloneNode(true);
    nameInput.parentNode.replaceChild(nameInputClone, nameInput);
    const slugInputClone = slugInput.cloneNode(true);
    slugInput.parentNode.replaceChild(slugInputClone, slugInput);
    
    // 重新獲取元素
    const freshNameInput = document.getElementById('modalPageName');
    const freshSlugInput = document.getElementById('modalPageSlug');
    
    if (freshNameInput && freshSlugInput) {
      // 綁定 slug 自動生成
      freshNameInput.addEventListener('input', (e) => {
        if (!freshSlugInput.value || freshSlugInput.dataset.autoGenerated === 'true') {
          const slug = e.target.value
            .toLowerCase()
            .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
            .replace(/^-+|-+$/g, '');
          freshSlugInput.value = slug;
          freshSlugInput.dataset.autoGenerated = 'true';
        }
      });

      // 手動編輯 slug 時取消自動生成
      freshSlugInput.addEventListener('input', (e) => {
        e.target.dataset.autoGenerated = 'false';
      });
    }
    
    // 顯示 modal
    const modal = new bootstrap.Modal(document.getElementById('pageSettingsModal'));
    modal.show();
};

PageEditor.savePageSettings = function() {
    // 驗證必填字段
    const nameInput = document.getElementById('modalPageName');
    const slugInput = document.getElementById('modalPageSlug');
    const statusSelect = document.getElementById('modalPageStatus');
    
    if (!nameInput || !slugInput || !statusSelect) {
      App.showError('無法找到頁面設定表單元素');
      return;
    }
    
    const name = nameInput.value.trim();
    const slug = slugInput.value.trim();
    
    if (!name) {
      App.showError('請輸入頁面名稱');
      return;
    }
    if (!slug) {
      App.showError('請輸入網址路徑');
      return;
    }

    // 更新本地數據
    if (!this.pageData) {
      this.pageData = {};
    }
    this.pageData.name = name;
    this.pageData.slug = slug;
    this.pageData.status = statusSelect.value;
    
    // 關閉 modal
    const modal = bootstrap.Modal.getInstance(document.getElementById('pageSettingsModal'));
    if (modal) {
      modal.hide();
    }
    
    // 更新預覽網址
    this.updatePreviewUrl();
    
    App.showSuccess('頁面設定已更新');
};

PageEditor.save = async function() {
    try {
      // 驗證必填字段（從 pageData 獲取）
      if (!this.pageData || !this.pageData.name || !this.pageData.slug) {
        App.showError('請先設定頁面名稱和網址路徑');
        this.showPageSettings();
        return;
      }
      
      const name = this.pageData.name.trim();
      const slug = this.pageData.slug.trim();

      const pageData = {
        name: name,
        slug: slug,
        status: this.pageData.status || 'draft'
      };
      
      // 保留 is_homepage 字段（如果存在）
      if (this.pageData.hasOwnProperty('is_homepage')) {
        pageData.is_homepage = this.pageData.is_homepage;
      }

      if (this.isNew) {
        // 新建模式：先創建頁面
        const newPage = await App.apiRequest('/pages', {
          method: 'POST',
          body: JSON.stringify(pageData)
        });
        
        this.pageId = newPage.id;
        this.isNew = false;
        this.pageData = newPage;
        
        // 更新 URL（不刷新頁面）
        window.history.replaceState({}, '', `/pages/${this.pageId}/edit`);
        document.getElementById('pageEditorPage').dataset.pageId = this.pageId;
        document.getElementById('pageEditorTitle').textContent = '編輯頁面';
        
        // 啟用預覽按鈕
        document.getElementById('previewBtn').disabled = false;
        document.getElementById('previewBtn').title = '';
        
        // 重新載入頁面列表並更新導航按鈕
      } else {
        // 編輯模式：更新頁面
        await App.apiRequest(`/pages/${this.pageId}`, {
          method: 'PUT',
          body: JSON.stringify(pageData)
        });
        
        // 更新本地數據（保留所有原有字段，包括 is_homepage）
        this.pageData = { ...this.pageData, ...pageData };
      }
      
      // 保存元件
      await App.apiRequest(`/pages/${this.pageId}/components`, {
        method: 'PUT',
        body: JSON.stringify({ components: this.components })
      });
      
      // If we were editing a linked block, save the block data too
      if (this.editingBlockId) {
        await this.saveLinkedBlock();
      }
      
      // 更新預覽網址
      this.updatePreviewUrl();
      
      App.showSuccess(this.t('common.saveSuccess', 'Saved successfully'));
    } catch (error) {
      App.showError(this.t('common.saveError', 'Save failed') + ': ' + (error.error || error.message));
  }
};

// ─── Page Editor Bootstrap ─────────────────────────────────────
// Single entry point for both full-page load and SPA navigation.
// Sets up page-editor body class, sidebar observer, and initializes PageEditor.
function _bootstrapPageEditor() {
  // Guard: only run when page-editor template is present
  if (!document.getElementById('pageEditorPage')) return;

  // Add page-editor class to body (hides CMS sidebar, resets margins)
  document.body.classList.add('page-editor');

  var sidebar = document.getElementById('sidebar');
  var overlay = document.getElementById('sidebarOverlay');
  var toggleBtn = document.getElementById('pageEditorSidebarToggle');

  if (sidebar) {
    sidebar.classList.remove('show');
  }

  // Listen for overlay click to show toggle button
  if (overlay) {
    overlay.addEventListener('click', function() {
      if (toggleBtn) {
        toggleBtn.style.display = 'flex';
      }
    });
  }

  // Watch sidebar show/hide to toggle the button
  if (sidebar) {
    // Disconnect previous observer if any
    if (PageEditor._sidebarObserver) {
      try { PageEditor._sidebarObserver.disconnect(); } catch (e) {}
    }
    PageEditor._sidebarObserver = new MutationObserver(function(mutations) {
      mutations.forEach(function(mutation) {
        if (mutation.attributeName === 'class' && toggleBtn) {
          if (sidebar.classList.contains('show')) {
            toggleBtn.style.display = 'none';
          } else {
            toggleBtn.style.display = 'flex';
          }
        }
      });
    });
    PageEditor._sidebarObserver.observe(sidebar, { attributes: true });
  }

  // Register SPA cleanup so leaving page-editor restores layout
  if (typeof Router !== 'undefined' && Router.onCleanup) {
    Router.onCleanup(function() {
      document.body.classList.remove('page-editor');
      if (PageEditor._sidebarObserver) {
        try { PageEditor._sidebarObserver.disconnect(); } catch (e) {}
        PageEditor._sidebarObserver = null;
      }
    });
  }

  // Initialize the editor
  PageEditor.init();
}

// Full-page load
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', _bootstrapPageEditor);
} else {
  _bootstrapPageEditor();
}

// Toggle page editor sidebar (used by onclick in template)
function togglePageEditorSidebar() {
  var sidebar = document.getElementById('sidebar');
  var overlay = document.getElementById('sidebarOverlay');
  var toggleBtn = document.getElementById('pageEditorSidebarToggle');

  if (sidebar) {
    sidebar.classList.toggle('show');
    if (overlay) {
      overlay.classList.toggle('show');
    }

    if (sidebar.classList.contains('show')) {
      if (toggleBtn) {
        toggleBtn.style.display = 'none';
      }
    } else {
      if (toggleBtn) {
        toggleBtn.style.display = 'flex';
      }
    }
  }
}
