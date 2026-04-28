// vAI Sketch Tool — Canvas-based drawing with AI image generation
// Tools: select, pen, rect, ellipse, line, text, eraser
// Features: image upload (draggable), save/load sketches, Gemini AI generation

var VaiSketch = (function() {
    'use strict';

    // ─── State ────────────────────────────────────────────────────
    var canvas, ctx;
    var currentTool = 'select';
    var strokeColor = '#333333';
    var fillColor = '#ffffff';
    var strokeWidth = 2;
    var isDrawing = false;
    var startX = 0, startY = 0;

    // Objects on canvas (for select/move)
    var objects = []; // { type, x, y, w, h, data, ... }
    var selectedObject = null;
    var dragOffsetX = 0, dragOffsetY = 0;
    var isDragging = false;
    var isResizing = false;
    var resizeHandle = null; // 'nw','ne','sw','se'
    var hasMoved = false; // true once drag/resize exceeds threshold
    var DRAG_THRESHOLD = 3; // pixels before drag begins

    // Aspect ratio constraint
    var aspectRatio = null; // null = free, or { w: N, h: N }

    // z-index counter — every new element gets an incrementing zIndex
    var nextZIndex = 1;

    // Pen (freehand) strokes
    var currentPath = [];

    // Undo/redo history
    var undoStack = [];
    var redoStack = [];

    // Sketches saved list
    var sketches = [];
    var currentSketchId = null;
    var currentSketchTitle = '';
    var PENDING_SKETCH_ID = '__pending__'; // 尚未儲存的暫存草圖 ID

    // Sidebar tab state: 'sketches' or 'chat'
    var sidebarTab = 'sketches';
    var chatGenerations = [];
    var chatGenTotal = 0;
    var chatGenOffset = 0;
    var chatGenLoading = false;
    var chatGenHasMore = false;
    var CHAT_GEN_LIMIT = 20;

    // Temp shape preview (rect/ellipse/line while drawing)
    var previewShape = null;

    // Attached reference images for AI generation (base64 data URLs, max 2)
    var attachedImages = []; // [{dataUrl: string}]

    // Dirty flag — true when canvas has unsaved changes
    var isDirty = false;

    // ─── Image Library ────────────────────────────────────────────
    // Categories and reference images for the template library.
    // The first item in 'all' is always blank (skip).
    // Images will be provided later — use placeholder paths for now.
    var imageLibraryCategories = [
        { id: 'all', label: '全部' },
        { id: 'promo', label: '促銷優惠' },
        { id: 'product', label: '產品介紹' },
        { id: 'device', label: '儀器推廣' },
        { id: 'result', label: '效果見證' },
        { id: 'event', label: '節日公告' }
    ];

    var imageLibraryItems = [
        // Blank / Skip (always first)
        { id: 'blank', category: 'all', title: '空白畫布', thumbnail: '', isBlank: true },
        // AI-generated template images (inspired by real ad styles, different content)
        { id: 'tpl_01', category: 'device',  title: 'EMS Body Sculpting Device',  thumbnail: '/static/img/sketch-library/template_01.png' },
        { id: 'tpl_02', category: 'device',  title: 'Teeth Whitening Laser',      thumbnail: '/static/img/sketch-library/template_02.png' },
        { id: 'tpl_03', category: 'promo',   title: 'Coffee BOGO Promo',          thumbnail: '/static/img/sketch-library/template_03.png' },
        { id: 'tpl_04', category: 'event',   title: 'Studio Holiday Hours',       thumbnail: '/static/img/sketch-library/template_04.png' },
        { id: 'tpl_05', category: 'promo',   title: 'Pet Grooming Trial $29',     thumbnail: '/static/img/sketch-library/template_05.png' },
        { id: 'tpl_06', category: 'product', title: 'Earbuds Comparison',         thumbnail: '/static/img/sketch-library/template_06.png' },
        { id: 'tpl_07', category: 'promo',   title: 'Keratin Treatment Deal',     thumbnail: '/static/img/sketch-library/template_07.png' },
        { id: 'tpl_08', category: 'result',  title: 'Fitness Transformation',     thumbnail: '/static/img/sketch-library/template_08.png' },
        { id: 'tpl_09', category: 'device',  title: 'Bubble Tea Grand Opening',   thumbnail: '/static/img/sketch-library/template_09.png' },
    ];
    var libraryActiveCategory = 'all';

    // ─── Initialization ───────────────────────────────────────────
    function init() {
        canvas = document.getElementById('vaiSketchCanvas');
        if (!canvas) return;
        ctx = canvas.getContext('2d');

        resizeCanvas();
        window.addEventListener('resize', resizeCanvas);

        // Mouse events
        canvas.addEventListener('mousedown', onMouseDown);
        canvas.addEventListener('mousemove', onMouseMove);
        canvas.addEventListener('mouseup', onMouseUp);
        canvas.addEventListener('mouseleave', onMouseUp);

        // Touch events
        canvas.addEventListener('touchstart', onTouchStart, { passive: false });
        canvas.addEventListener('touchmove', onTouchMove, { passive: false });
        canvas.addEventListener('touchend', onTouchEnd);

        // Keyboard shortcuts
        document.addEventListener('keydown', onKeyDown);

        // Load sketch list
        loadSketchList();

        // Prompt enter key + mic/send toggle
        var promptInput = document.getElementById('vaiSketchPrompt');
        if (promptInput) {
            promptInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    generateWithAI();
                }
            });
            promptInput.addEventListener('input', function() {
                updateSketchMicGenToggle();
            });
        }
        // Initialize mic/gen toggle (gen hidden by default, mic visible)
        updateSketchMicGenToggle();

        // Redraw canvas when I18n translations become ready (for placeholder text)
        document.addEventListener('languageChanged', function() {
            redrawCanvas();
            // Update pending sketch title with correct translation
            var pending = sketches.find(function(s) { return s.id === PENDING_SKETCH_ID; });
            if (pending) {
                pending.title = I18n.t('vai.sketch.newSketch');
                renderSketchList();
            }
        });

        // Warn user before leaving if there are unsaved changes
        window.addEventListener('beforeunload', function(e) {
            if (isDirty) {
                e.preventDefault();
                e.returnValue = '';
            }
        });

        console.log('[VaiSketch] Initialized');
    }

    function resizeCanvas() {
        var wrapper = document.getElementById('vaiSketchCanvasWrapper');
        if (!wrapper || !canvas) return;
        var rect = wrapper.getBoundingClientRect();
        var wrapperW = rect.width;
        var wrapperH = rect.height;

        var canvasW, canvasH;

        if (aspectRatio) {
            // Calculate canvas size that fits inside wrapper while maintaining aspect ratio
            var targetRatio = aspectRatio.w / aspectRatio.h;
            var wrapperRatio = wrapperW / wrapperH;
            if (targetRatio > wrapperRatio) {
                // Width-limited
                canvasW = wrapperW;
                canvasH = wrapperW / targetRatio;
            } else {
                // Height-limited
                canvasH = wrapperH;
                canvasW = wrapperH * targetRatio;
            }
            canvasW = Math.floor(canvasW);
            canvasH = Math.floor(canvasH);
        } else {
            canvasW = Math.floor(wrapperW);
            canvasH = Math.floor(wrapperH);
        }

        // Save current image data
        var imageData = null;
        if (canvas.width > 0 && canvas.height > 0) {
            try { imageData = ctx.getImageData(0, 0, canvas.width, canvas.height); } catch(e) {}
        }

        canvas.width = canvasW;
        canvas.height = canvasH;

        // Apply CSS size and centering
        if (aspectRatio) {
            canvas.style.width = canvasW + 'px';
            canvas.style.height = canvasH + 'px';
            canvas.style.position = 'absolute';
            canvas.style.left = Math.floor((wrapperW - canvasW) / 2) + 'px';
            canvas.style.top = Math.floor((wrapperH - canvasH) / 2) + 'px';
        } else {
            canvas.style.width = '100%';
            canvas.style.height = '100%';
            canvas.style.position = '';
            canvas.style.left = '';
            canvas.style.top = '';
        }

        // Restore
        if (imageData) {
            try { ctx.putImageData(imageData, 0, 0); } catch(e) {}
        }
        redrawCanvas();
    }

    // ─── z-index helpers ─────────────────────────────────────────
    function getNextZIndex() {
        return nextZIndex++;
    }

    function recalcNextZIndex() {
        // Recalculate nextZIndex from existing objects (e.g. after load/undo/redo)
        var maxZ = 0;
        for (var i = 0; i < objects.length; i++) {
            if (objects[i].zIndex !== undefined && objects[i].zIndex > maxZ) {
                maxZ = objects[i].zIndex;
            }
        }
        nextZIndex = maxZ + 1;
    }

    function getObjectsSortedByZIndex() {
        // Return a shallow copy sorted by zIndex ascending (lowest first = drawn first)
        return objects.slice().sort(function(a, b) {
            return (a.zIndex || 0) - (b.zIndex || 0);
        });
    }

    // ─── Tool Management ──────────────────────────────────────────
    function setTool(tool) {
        currentTool = tool;
        // Update toolbar active state
        document.querySelectorAll('.vai-sketch-toolbar [data-tool]').forEach(function(btn) {
            btn.classList.toggle('active', btn.getAttribute('data-tool') === tool);
        });
        // Update cursor
        if (canvas) {
            if (tool === 'pen' || tool === 'eraser') canvas.style.cursor = 'crosshair';
            else if (tool === 'text') canvas.style.cursor = 'text';
            else if (tool === 'select') canvas.style.cursor = 'default';
            else canvas.style.cursor = 'crosshair';
        }
        // Hide text input if switching away from text
        if (tool !== 'text') {
            var textInput = document.getElementById('vaiSketchTextInput');
            if (textInput) textInput.style.display = 'none';
        }
    }

    function setStrokeColor(color) { strokeColor = color; }
    function setFillColor(color) { fillColor = color; }
    function setStrokeWidth(w) {
        strokeWidth = parseInt(w) || 2;
        var label = document.getElementById('vaiSketchStrokeWidthLabel');
        if (label) label.textContent = strokeWidth;
    }

    function setAspectRatio(value) {
        if (!value || value === 'free') {
            aspectRatio = null;
        } else {
            var parts = value.split(':');
            if (parts.length === 2) {
                aspectRatio = { w: parseInt(parts[0]), h: parseInt(parts[1]) };
            }
        }
        resizeCanvas();
    }

    // Simplify a width/height pair to a small integer ratio (e.g. 896:1152 → 3:4)
    function simplifyRatio(w, h) {
        function gcd(a, b) { return b === 0 ? a : gcd(b, a % b); }
        var g = gcd(w, h);
        var rw = w / g;
        var rh = h / g;
        // If the simplified numbers are too large, try to match known ratios
        var known = [[1,1],[4,3],[3,4],[7,9],[9,7],[16,9],[9,16],[3,2],[2,3]];
        var actual = rw / rh;
        for (var i = 0; i < known.length; i++) {
            if (Math.abs(actual - known[i][0] / known[i][1]) < 0.02) {
                return { w: known[i][0], h: known[i][1] };
            }
        }
        // Fallback: cap to reasonable numbers
        if (rw > 20 || rh > 20) {
            // Round to nearest known
            var bestDiff = 999, best = { w: rw, h: rh };
            for (var j = 0; j < known.length; j++) {
                var diff = Math.abs(actual - known[j][0] / known[j][1]);
                if (diff < bestDiff) { bestDiff = diff; best = { w: known[j][0], h: known[j][1] }; }
            }
            return best;
        }
        return { w: rw, h: rh };
    }

    // Sync the aspect ratio <select> dropdown to reflect the current ratio.
    // If the ratio is not among the existing options, dynamically add it.
    function syncAspectRatioSelect(rw, rh) {
        var sel = document.getElementById('vaiSketchAspectRatio');
        if (!sel) return;
        var target = rw + ':' + rh;
        var found = false;
        for (var i = 0; i < sel.options.length; i++) {
            if (sel.options[i].value === target) {
                sel.value = target;
                found = true;
                break;
            }
        }
        if (!found) {
            // Dynamically add this ratio as an option
            var opt = document.createElement('option');
            opt.value = target;
            opt.textContent = target;
            sel.appendChild(opt);
            sel.value = target;
        }
    }

    // Ask user if they want to change canvas aspect ratio to match the added image.
    // Shows a custom modal with the detected ratio. Calls callback() after user decides.
    // The image is always added regardless; ratio change is optional.
    function promptAspectRatioChange(imgW, imgH, callback) {
        var ratio = simplifyRatio(imgW, imgH);
        var newRatioStr = ratio.w + ':' + ratio.h;

        // If already the same ratio, skip prompt
        if (aspectRatio && aspectRatio.w === ratio.w && aspectRatio.h === ratio.h) {
            callback();
            return;
        }

        // Show the ratio prompt modal
        var modal = document.getElementById('vaiRatioPromptModal');
        if (!modal) { callback(); return; }

        var ratioLabel = document.getElementById('vaiRatioPromptLabel');
        if (ratioLabel) ratioLabel.textContent = newRatioStr;

        var bsModal = new bootstrap.Modal(modal, { backdrop: 'static', keyboard: false });

        // Clean up any previous listeners
        var btnYes = document.getElementById('vaiRatioPromptYes');
        var btnNo = document.getElementById('vaiRatioPromptNo');
        var newBtnYes = btnYes.cloneNode(true);
        var newBtnNo = btnNo.cloneNode(true);
        btnYes.parentNode.replaceChild(newBtnYes, btnYes);
        btnNo.parentNode.replaceChild(newBtnNo, btnNo);

        newBtnYes.addEventListener('click', function() {
            aspectRatio = { w: ratio.w, h: ratio.h };
            syncAspectRatioSelect(ratio.w, ratio.h);
            resizeCanvas();
            bsModal.hide();
            callback();
        });
        newBtnNo.addEventListener('click', function() {
            bsModal.hide();
            callback();
        });

        bsModal.show();
    }

    // ─── Mouse / Touch Handlers ───────────────────────────────────
    function getCanvasXY(e) {
        var rect = canvas.getBoundingClientRect();
        return {
            x: (e.clientX || (e.touches && e.touches[0] && e.touches[0].clientX) || 0) - rect.left,
            y: (e.clientY || (e.touches && e.touches[0] && e.touches[0].clientY) || 0) - rect.top
        };
    }

    function onTouchStart(e) {
        e.preventDefault();
        if (e.touches.length === 1) {
            var fakeEvent = { clientX: e.touches[0].clientX, clientY: e.touches[0].clientY, button: 0 };
            onMouseDown(fakeEvent);
        }
    }
    function onTouchMove(e) {
        e.preventDefault();
        if (e.touches.length === 1) {
            var fakeEvent = { clientX: e.touches[0].clientX, clientY: e.touches[0].clientY };
            onMouseMove(fakeEvent);
        }
    }
    function onTouchEnd(e) {
        onMouseUp(e);
    }

    function onMouseDown(e) {
        if (e.button && e.button !== 0) return;
        var pos = getCanvasXY(e);
        startX = pos.x;
        startY = pos.y;

        if (currentTool === 'select') {
            // Check if clicking resize handle of selected object
            if (selectedObject) {
                var handle = hitTestResizeHandle(selectedObject, pos.x, pos.y);
                if (handle) {
                    isResizing = true;
                    hasMoved = false;
                    resizeHandle = handle;
                    return;
                }
            }
            // Check if clicking an object
            var hit = hitTestObject(pos.x, pos.y);
            if (hit) {
                selectedObject = hit;
                isDragging = true;
                hasMoved = false;
                // For pen/eraser, use path bounds center as drag anchor
                if (hit.type === 'pen' || hit.type === 'eraser') {
                    dragOffsetX = pos.x;
                    dragOffsetY = pos.y;
                } else {
                    dragOffsetX = pos.x - hit.x;
                    dragOffsetY = pos.y - hit.y;
                }
                showPropsPanel(hit);
                redrawCanvas();
            } else {
                selectedObject = null;
                isDragging = false;
                hasMoved = false;
                hidePropsPanel();
                redrawCanvas();
            }
            return;
        }

        if (currentTool === 'text') {
            showTextInput(pos.x, pos.y);
            return;
        }

        isDrawing = true;
        if (currentTool === 'pen' || currentTool === 'eraser') {
            currentPath = [{ x: pos.x, y: pos.y }];
        }
    }

    function onMouseMove(e) {
        if (!canvas) return;
        var pos = getCanvasXY(e);

        // Update cursor for select tool
        if (currentTool === 'select' && selectedObject && !isDragging && !isResizing) {
            var handle = hitTestResizeHandle(selectedObject, pos.x, pos.y);
            if (handle) {
                canvas.style.cursor = (handle === 'nw' || handle === 'se') ? 'nwse-resize' : 'nesw-resize';
            } else if (hitTestSingleObject(selectedObject, pos.x, pos.y)) {
                canvas.style.cursor = 'move';
            } else {
                canvas.style.cursor = 'default';
            }
        }

        if (currentTool === 'select' && isResizing && selectedObject) {
            var dx = pos.x - startX;
            var dy = pos.y - startY;
            // Only start resize once movement exceeds threshold
            if (!hasMoved) {
                if (Math.abs(dx) < DRAG_THRESHOLD && Math.abs(dy) < DRAG_THRESHOLD) return;
                hasMoved = true;
                saveUndoState(); // save state BEFORE first mutation
            }
            var obj = selectedObject;
            if (resizeHandle === 'se') {
                obj.w = Math.max(10, obj.w + dx);
                obj.h = Math.max(10, obj.h + dy);
            } else if (resizeHandle === 'sw') {
                obj.x += dx;
                obj.w = Math.max(10, obj.w - dx);
                obj.h = Math.max(10, obj.h + dy);
            } else if (resizeHandle === 'ne') {
                obj.w = Math.max(10, obj.w + dx);
                obj.y += dy;
                obj.h = Math.max(10, obj.h - dy);
            } else if (resizeHandle === 'nw') {
                obj.x += dx;
                obj.y += dy;
                obj.w = Math.max(10, obj.w - dx);
                obj.h = Math.max(10, obj.h - dy);
            }
            startX = pos.x;
            startY = pos.y;
            redrawCanvas();
            return;
        }

        if (currentTool === 'select' && isDragging && selectedObject) {
            var ddx = pos.x - startX;
            var ddy = pos.y - startY;
            // Only start drag once movement exceeds threshold
            if (!hasMoved) {
                if (Math.abs(ddx) < DRAG_THRESHOLD && Math.abs(ddy) < DRAG_THRESHOLD) return;
                hasMoved = true;
                saveUndoState(); // save state BEFORE first mutation
            }
            var obj = selectedObject;
            if (obj.type === 'pen' || obj.type === 'eraser') {
                // Offset all path points by the delta
                var dx = pos.x - dragOffsetX;
                var dy = pos.y - dragOffsetY;
                for (var pi = 0; pi < obj.path.length; pi++) {
                    obj.path[pi].x += dx;
                    obj.path[pi].y += dy;
                }
                dragOffsetX = pos.x;
                dragOffsetY = pos.y;
            } else if (obj.type === 'line') {
                // Offset both endpoints
                var newX = pos.x - dragOffsetX;
                var newY = pos.y - dragOffsetY;
                var ldx = newX - obj.x;
                var ldy = newY - obj.y;
                obj.x = newX;
                obj.y = newY;
                obj.x2 += ldx;
                obj.y2 += ldy;
            } else {
                obj.x = pos.x - dragOffsetX;
                obj.y = pos.y - dragOffsetY;
            }
            redrawCanvas();
            return;
        }

        if (!isDrawing) return;

        if (currentTool === 'pen') {
            currentPath.push({ x: pos.x, y: pos.y });
            redrawCanvas();
            // Draw current path preview
            drawPath(currentPath, strokeColor, strokeWidth, false);
        } else if (currentTool === 'eraser') {
            currentPath.push({ x: pos.x, y: pos.y });
            redrawCanvas();
            drawPath(currentPath, '#ffffff', strokeWidth * 3, false);
        } else if (currentTool === 'rect' || currentTool === 'ellipse' || currentTool === 'line') {
            previewShape = { type: currentTool, x: startX, y: startY, w: pos.x - startX, h: pos.y - startY };
            redrawCanvas();
            drawShapePreview(previewShape);
        }
    }

    function onMouseUp(e) {
        if (currentTool === 'select') {
            if ((isDragging || isResizing) && hasMoved) {
                // Undo state was already saved BEFORE first mutation in onMouseMove
                // Refresh panel values after move/resize
                if (selectedObject) showPropsPanel(selectedObject);
            }
            isDragging = false;
            isResizing = false;
            hasMoved = false;
            resizeHandle = null;
            return;
        }

        if (!isDrawing) return;
        isDrawing = false;

        var pos = getCanvasXY(e.changedTouches ? e.changedTouches[0] || e : e);

        saveUndoState();

        if (currentTool === 'pen' && currentPath.length > 1) {
            objects.push({
                type: 'pen',
                path: currentPath.slice(),
                color: strokeColor,
                width: strokeWidth,
                x: 0, y: 0, w: 0, h: 0,
                zIndex: getNextZIndex()
            });
        } else if (currentTool === 'eraser' && currentPath.length > 1) {
            objects.push({
                type: 'eraser',
                path: currentPath.slice(),
                color: '#ffffff',
                width: strokeWidth * 3,
                x: 0, y: 0, w: 0, h: 0,
                zIndex: getNextZIndex()
            });
        } else if (currentTool === 'rect') {
            var w = pos.x - startX;
            var h = pos.y - startY;
            if (Math.abs(w) > 2 && Math.abs(h) > 2) {
                objects.push({
                    type: 'rect',
                    x: w > 0 ? startX : startX + w,
                    y: h > 0 ? startY : startY + h,
                    w: Math.abs(w),
                    h: Math.abs(h),
                    stroke: strokeColor,
                    fill: fillColor,
                    lineWidth: strokeWidth,
                    zIndex: getNextZIndex()
                });
            }
        } else if (currentTool === 'ellipse') {
            var ew = pos.x - startX;
            var eh = pos.y - startY;
            if (Math.abs(ew) > 2 && Math.abs(eh) > 2) {
                objects.push({
                    type: 'ellipse',
                    x: ew > 0 ? startX : startX + ew,
                    y: eh > 0 ? startY : startY + eh,
                    w: Math.abs(ew),
                    h: Math.abs(eh),
                    stroke: strokeColor,
                    fill: fillColor,
                    lineWidth: strokeWidth,
                    zIndex: getNextZIndex()
                });
            }
        } else if (currentTool === 'line') {
            var lw = pos.x - startX;
            var lh = pos.y - startY;
            if (Math.abs(lw) > 2 || Math.abs(lh) > 2) {
                objects.push({
                    type: 'line',
                    x: startX, y: startY,
                    x2: pos.x, y2: pos.y,
                    w: Math.abs(lw), h: Math.abs(lh),
                    stroke: strokeColor,
                    lineWidth: strokeWidth,
                    zIndex: getNextZIndex()
                });
            }
        }

        currentPath = [];
        previewShape = null;
        redrawCanvas();
    }

    // ─── Keyboard Shortcuts ───────────────────────────────────────
    function onKeyDown(e) {
        // Don't intercept if typing in input/textarea
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

        if (e.ctrlKey || e.metaKey) {
            if (e.key === 'z') { e.preventDefault(); undo(); }
            else if (e.key === 'y') { e.preventDefault(); redo(); }
            else if (e.key === 's') { e.preventDefault(); save(); }
            return;
        }

        switch(e.key.toLowerCase()) {
            case 'v': setTool('select'); break;
            case 'p': setTool('pen'); break;
            case 'r': setTool('rect'); break;
            case 'e': setTool('ellipse'); break;
            case 'l': setTool('line'); break;
            case 't': setTool('text'); break;
            case 'x': setTool('eraser'); break;
            case 'delete':
            case 'backspace':
                if (selectedObject) { deleteSelected(); }
                break;
        }
    }

    // ─── Drawing Functions ────────────────────────────────────────
    function redrawCanvas() {
        if (!ctx || !canvas) return;
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        // White background
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, canvas.width, canvas.height);

        // Placeholder when canvas is empty
        if (objects.length === 0) {
            ctx.save();
            ctx.fillStyle = '#bbb';
            ctx.font = '16px sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            var translated = (typeof I18n !== 'undefined' && I18n.t) ? I18n.t('vai.sketch.canvasPlaceholder') : null;
            var text = (translated && translated !== 'vai.sketch.canvasPlaceholder') ? translated : '在此畫布上繪圖';
            ctx.fillText(text, canvas.width / 2, canvas.height / 2);
            ctx.restore();
        }

        // Draw all objects sorted by zIndex (lowest first)
        var sorted = getObjectsSortedByZIndex();
        for (var i = 0; i < sorted.length; i++) {
            drawObject(sorted[i]);
        }

        // Draw selection handles
        if (selectedObject && currentTool === 'select') {
            drawSelectionHandles(selectedObject);
        }
    }

    function drawObject(obj) {
        if (!ctx) return;
        ctx.save();

        if (obj.type === 'pen' || obj.type === 'eraser') {
            drawPath(obj.path, obj.color, obj.width, true);
        } else if (obj.type === 'rect') {
            ctx.strokeStyle = obj.stroke;
            ctx.fillStyle = obj.fill;
            ctx.lineWidth = obj.lineWidth;
            if (obj.fill && obj.fill !== 'transparent') ctx.fillRect(obj.x, obj.y, obj.w, obj.h);
            ctx.strokeRect(obj.x, obj.y, obj.w, obj.h);
        } else if (obj.type === 'ellipse') {
            ctx.strokeStyle = obj.stroke;
            ctx.fillStyle = obj.fill;
            ctx.lineWidth = obj.lineWidth;
            ctx.beginPath();
            ctx.ellipse(obj.x + obj.w/2, obj.y + obj.h/2, obj.w/2, obj.h/2, 0, 0, Math.PI * 2);
            if (obj.fill && obj.fill !== 'transparent') ctx.fill();
            ctx.stroke();
        } else if (obj.type === 'line') {
            ctx.strokeStyle = obj.stroke;
            ctx.lineWidth = obj.lineWidth;
            ctx.beginPath();
            ctx.moveTo(obj.x, obj.y);
            ctx.lineTo(obj.x2, obj.y2);
            ctx.stroke();
        } else if (obj.type === 'text') {
            ctx.font = (obj.fontSize || 16) + 'px sans-serif';
            ctx.fillStyle = obj.color || strokeColor;
            ctx.textBaseline = 'top';
            var lines = (obj.text || '').split('\n');
            for (var li = 0; li < lines.length; li++) {
                ctx.fillText(lines[li], obj.x, obj.y + li * (obj.fontSize || 16) * 1.2);
            }
            // Update bounding box for selection
            var maxW = 0;
            for (var mi = 0; mi < lines.length; mi++) {
                var mw = ctx.measureText(lines[mi]).width;
                if (mw > maxW) maxW = mw;
            }
            obj.w = maxW + 4;
            obj.h = lines.length * (obj.fontSize || 16) * 1.2;
        } else if (obj.type === 'image') {
            if (obj._img && obj._img.complete) {
                if (obj.opacity !== undefined && obj.opacity < 1) {
                    ctx.globalAlpha = obj.opacity;
                }
                ctx.drawImage(obj._img, obj.x, obj.y, obj.w, obj.h);
                ctx.globalAlpha = 1;
            }
        }

        ctx.restore();
    }

    function drawPath(path, color, width, isFinal) {
        if (!ctx || !path || path.length < 2) return;
        ctx.save();
        ctx.strokeStyle = color;
        ctx.lineWidth = width;
        ctx.lineCap = 'round';
        ctx.lineJoin = 'round';
        ctx.beginPath();
        ctx.moveTo(path[0].x, path[0].y);
        for (var i = 1; i < path.length; i++) {
            ctx.lineTo(path[i].x, path[i].y);
        }
        ctx.stroke();
        ctx.restore();
    }

    function drawShapePreview(shape) {
        if (!ctx || !shape) return;
        ctx.save();
        ctx.strokeStyle = strokeColor;
        ctx.fillStyle = fillColor;
        ctx.lineWidth = strokeWidth;
        ctx.setLineDash([5, 5]);

        if (shape.type === 'rect') {
            ctx.strokeRect(shape.x, shape.y, shape.w, shape.h);
        } else if (shape.type === 'ellipse') {
            ctx.beginPath();
            var cx = shape.x + shape.w / 2;
            var cy = shape.y + shape.h / 2;
            ctx.ellipse(cx, cy, Math.abs(shape.w/2), Math.abs(shape.h/2), 0, 0, Math.PI * 2);
            ctx.stroke();
        } else if (shape.type === 'line') {
            ctx.beginPath();
            ctx.moveTo(shape.x, shape.y);
            ctx.lineTo(shape.x + shape.w, shape.y + shape.h);
            ctx.stroke();
        }

        ctx.restore();
    }

    // ─── Selection & Hit Testing ──────────────────────────────────
    function hitTestObject(x, y) {
        // Sort by zIndex descending — topmost (highest zIndex) first
        var sorted = getObjectsSortedByZIndex().reverse();
        for (var i = 0; i < sorted.length; i++) {
            if (hitTestSingleObject(sorted[i], x, y)) return sorted[i];
        }
        return null;
    }

    function hitTestSingleObject(obj, x, y) {
        if (obj.type === 'pen' || obj.type === 'eraser') {
            // Check proximity to any point in path
            for (var p = 0; p < obj.path.length; p++) {
                var dx = x - obj.path[p].x;
                var dy = y - obj.path[p].y;
                if (dx * dx + dy * dy < 100) return true;
            }
            return false;
        }
        if (obj.type === 'line') {
            // Distance from point to line segment
            return distToSegment(x, y, obj.x, obj.y, obj.x2, obj.y2) < 8;
        }
        // Bounding box test for rect, ellipse, text, image
        return x >= obj.x && x <= obj.x + obj.w && y >= obj.y && y <= obj.y + obj.h;
    }

    function distToSegment(px, py, x1, y1, x2, y2) {
        var A = px - x1, B = py - y1, C = x2 - x1, D = y2 - y1;
        var dot = A*C + B*D;
        var lenSq = C*C + D*D;
        var param = lenSq !== 0 ? dot / lenSq : -1;
        var xx, yy;
        if (param < 0) { xx = x1; yy = y1; }
        else if (param > 1) { xx = x2; yy = y2; }
        else { xx = x1 + param*C; yy = y1 + param*D; }
        var dx = px - xx, dy = py - yy;
        return Math.sqrt(dx*dx + dy*dy);
    }

    var HANDLE_SIZE = 8;
    function hitTestResizeHandle(obj, x, y) {
        if (!obj || obj.type === 'pen' || obj.type === 'eraser' || obj.type === 'line') return null;
        var corners = getCorners(obj);
        for (var key in corners) {
            var c = corners[key];
            if (Math.abs(x - c.x) < HANDLE_SIZE && Math.abs(y - c.y) < HANDLE_SIZE) return key;
        }
        return null;
    }

    function getCorners(obj) {
        return {
            nw: { x: obj.x, y: obj.y },
            ne: { x: obj.x + obj.w, y: obj.y },
            sw: { x: obj.x, y: obj.y + obj.h },
            se: { x: obj.x + obj.w, y: obj.y + obj.h }
        };
    }

    function drawSelectionHandles(obj) {
        if (!ctx || !obj) return;
        ctx.save();
        ctx.strokeStyle = '#8b5cf6';
        ctx.lineWidth = 1;
        ctx.setLineDash([4, 4]);

        if (obj.type === 'line') {
            // Draw endpoints
            ctx.setLineDash([]);
            ctx.fillStyle = '#8b5cf6';
            ctx.fillRect(obj.x - 4, obj.y - 4, 8, 8);
            ctx.fillRect(obj.x2 - 4, obj.y2 - 4, 8, 8);
        } else if (obj.type === 'pen' || obj.type === 'eraser') {
            // Bounding box of path
            var bounds = getPathBounds(obj.path);
            ctx.strokeRect(bounds.x - 4, bounds.y - 4, bounds.w + 8, bounds.h + 8);
        } else {
            ctx.strokeRect(obj.x - 2, obj.y - 2, obj.w + 4, obj.h + 4);
            ctx.setLineDash([]);
            ctx.fillStyle = '#8b5cf6';
            var corners = getCorners(obj);
            for (var key in corners) {
                var c = corners[key];
                ctx.fillRect(c.x - HANDLE_SIZE/2, c.y - HANDLE_SIZE/2, HANDLE_SIZE, HANDLE_SIZE);
            }
        }
        ctx.restore();
    }

    function getPathBounds(path) {
        var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
        for (var i = 0; i < path.length; i++) {
            if (path[i].x < minX) minX = path[i].x;
            if (path[i].y < minY) minY = path[i].y;
            if (path[i].x > maxX) maxX = path[i].x;
            if (path[i].y > maxY) maxY = path[i].y;
        }
        return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
    }

    function deleteSelected() {
        if (!selectedObject) return;
        saveUndoState();
        objects = objects.filter(function(o) { return o !== selectedObject; });
        selectedObject = null;
        hidePropsPanel();
        redrawCanvas();
    }

    function deselectObject() {
        selectedObject = null;
        hidePropsPanel();
        redrawCanvas();
    }

    function duplicateSelected() {
        if (!selectedObject) return;
        saveUndoState();
        var clone = JSON.parse(JSON.stringify(selectedObject));
        // Remove _img reference (will be re-created for images)
        if (clone.type === 'image' && clone.dataUrl) {
            var img = new Image();
            img.src = clone.dataUrl;
            clone._img = img;
        }
        // Offset duplicate slightly
        clone.x = (clone.x || 0) + 20;
        clone.y = (clone.y || 0) + 20;
        if (clone.x2 !== undefined) { clone.x2 += 20; clone.y2 += 20; }
        if (clone.path) {
            clone.path = clone.path.map(function(p) { return { x: p.x + 20, y: p.y + 20 }; });
        }
        clone.zIndex = getNextZIndex();
        objects.push(clone);
        selectedObject = clone;
        showPropsPanel(clone);
        redrawCanvas();
    }

    function bringToFront() {
        if (!selectedObject) return;
        saveUndoState();
        // Assign the highest zIndex
        selectedObject.zIndex = getNextZIndex();
        redrawCanvas();
    }

    function sendToBack() {
        if (!selectedObject) return;
        saveUndoState();
        // Find the current minimum zIndex and go below it
        var minZ = Infinity;
        for (var i = 0; i < objects.length; i++) {
            if ((objects[i].zIndex || 0) < minZ) minZ = objects[i].zIndex || 0;
        }
        selectedObject.zIndex = minZ - 1;
        redrawCanvas();
    }

    // ─── Properties Panel ─────────────────────────────────────────
    function showPropsPanel(obj) {
        if (!obj) return;
        var panel = document.getElementById('vaiSketchProps');
        var body = document.getElementById('vaiPropsBody');
        var title = document.getElementById('vaiPropsTitle');
        if (!panel || !body) return;

        // Set title based on type
        var typeLabels = {
            pen: I18n.t('vai.sketch.props.types.pen'), eraser: I18n.t('vai.sketch.props.types.eraser'), rect: I18n.t('vai.sketch.props.types.rect'), ellipse: I18n.t('vai.sketch.props.types.ellipse'),
            line: I18n.t('vai.sketch.props.types.line'), text: I18n.t('vai.sketch.props.types.text'), image: I18n.t('vai.sketch.props.types.image')
        };
        if (title) title.textContent = typeLabels[obj.type] || I18n.t('vai.sketch.props.title');

        var html = '';

        // Position section (for most types)
        if (obj.type !== 'pen' && obj.type !== 'eraser') {
            html += '<div class="props-section">';
            html += '<label>' + I18n.t('vai.sketch.props.position') + '</label>';
            html += '<div class="props-row">';
            html += '<div><label>X</label><input type="number" class="form-control" value="' + Math.round(obj.x || 0) + '" onchange="VaiSketch.updateSelectedProp(\'x\', +this.value)"></div>';
            html += '<div><label>Y</label><input type="number" class="form-control" value="' + Math.round(obj.y || 0) + '" onchange="VaiSketch.updateSelectedProp(\'y\', +this.value)"></div>';
            html += '</div>';
            // Width / Height for resizable objects
            if (obj.type !== 'line') {
                html += '<div class="props-row">';
                html += '<div><label>' + I18n.t('vai.sketch.props.width') + '</label><input type="number" class="form-control" value="' + Math.round(obj.w || 0) + '" onchange="VaiSketch.updateSelectedProp(\'w\', +this.value)" min="1"></div>';
                html += '<div><label>' + I18n.t('vai.sketch.props.height') + '</label><input type="number" class="form-control" value="' + Math.round(obj.h || 0) + '" onchange="VaiSketch.updateSelectedProp(\'h\', +this.value)" min="1"></div>';
                html += '</div>';
            }
            html += '</div>';
        }

        // Stroke / Fill section
        if (obj.type === 'rect' || obj.type === 'ellipse') {
            html += '<div class="props-section">';
            html += '<label>' + I18n.t('vai.sketch.props.strokeColor') + '</label>';
            html += '<input type="color" class="form-control form-control-color" value="' + (obj.stroke || '#333333') + '" onchange="VaiSketch.updateSelectedProp(\'stroke\', this.value)">';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.fillColor') + '</label>';
            html += '<input type="color" class="form-control form-control-color" value="' + (obj.fill || '#ffffff') + '" onchange="VaiSketch.updateSelectedProp(\'fill\', this.value)">';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.lineWidth') + '</label>';
            html += '<input type="range" class="form-range" min="1" max="20" value="' + (obj.lineWidth || 2) + '" oninput="VaiSketch.updateSelectedProp(\'lineWidth\', +this.value)">';
            html += '</div>';
        } else if (obj.type === 'line') {
            html += '<div class="props-section">';
            html += '<label>' + I18n.t('vai.sketch.props.strokeColor') + '</label>';
            html += '<input type="color" class="form-control form-control-color" value="' + (obj.stroke || '#333333') + '" onchange="VaiSketch.updateSelectedProp(\'stroke\', this.value)">';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.lineWidth') + '</label>';
            html += '<input type="range" class="form-range" min="1" max="20" value="' + (obj.lineWidth || 2) + '" oninput="VaiSketch.updateSelectedProp(\'lineWidth\', +this.value)">';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.endpoint') + '</label>';
            html += '<div class="props-row">';
            html += '<div><label>X2</label><input type="number" class="form-control" value="' + Math.round(obj.x2 || 0) + '" onchange="VaiSketch.updateSelectedProp(\'x2\', +this.value)"></div>';
            html += '<div><label>Y2</label><input type="number" class="form-control" value="' + Math.round(obj.y2 || 0) + '" onchange="VaiSketch.updateSelectedProp(\'y2\', +this.value)"></div>';
            html += '</div>';
            html += '</div>';
        } else if (obj.type === 'pen' || obj.type === 'eraser') {
            html += '<div class="props-section">';
            html += '<label>' + I18n.t('vai.sketch.props.color') + '</label>';
            html += '<input type="color" class="form-control form-control-color" value="' + (obj.color || '#333333') + '" onchange="VaiSketch.updateSelectedProp(\'color\', this.value)">';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.lineWidth') + '</label>';
            html += '<input type="range" class="form-range" min="1" max="20" value="' + (obj.width || 2) + '" oninput="VaiSketch.updateSelectedProp(\'width\', +this.value)">';
            html += '</div>';
        } else if (obj.type === 'text') {
            html += '<div class="props-section">';
            html += '<label>' + I18n.t('vai.sketch.props.textContent') + '</label>';
            html += '<textarea class="form-control" rows="3" style="font-size: 0.8rem;" onchange="VaiSketch.updateSelectedProp(\'text\', this.value)">' + escapeHtml(obj.text || '') + '</textarea>';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.color') + '</label>';
            html += '<input type="color" class="form-control form-control-color" value="' + (obj.color || '#333333') + '" onchange="VaiSketch.updateSelectedProp(\'color\', this.value)">';
            html += '<label class="mt-1">' + I18n.t('vai.sketch.props.fontSize') + '</label>';
            html += '<input type="number" class="form-control" value="' + (obj.fontSize || 16) + '" min="8" max="200" onchange="VaiSketch.updateSelectedProp(\'fontSize\', +this.value)">';
            html += '</div>';
        } else if (obj.type === 'image') {
            html += '<div class="props-section">';
            html += '<label>' + I18n.t('vai.sketch.props.opacity') + '</label>';
            html += '<input type="range" class="form-range" min="0" max="100" value="' + Math.round((obj.opacity !== undefined ? obj.opacity : 1) * 100) + '" oninput="VaiSketch.updateSelectedProp(\'opacity\', +this.value / 100)">';
            html += '<button class="btn btn-sm btn-outline-info w-100 mt-2" id="vaiRemoveBgBtn" onclick="VaiSketch.removeImageBackground()"><i class="bi bi-magic me-1"></i>' + I18n.t('vai.sketch.props.removeBackground') + '</button>';
            html += '<button class="btn btn-sm btn-outline-secondary w-100 mt-1" onclick="document.getElementById(\'vaiSketchReplaceImg\').click()"><i class="bi bi-arrow-repeat me-1"></i>' + I18n.t('vai.sketch.props.replaceImage') + '</button>';
            html += '<input type="file" id="vaiSketchReplaceImg" accept="image/*" style="display:none;" onchange="VaiSketch.replaceSelectedImage(event)">';
            html += '</div>';
        }

        // Layer order section (all types)
        html += '<div class="props-section">';
        html += '<label>' + I18n.t('vai.sketch.props.layerOrder') + '</label>';
        html += '<div class="d-flex gap-1">';
        html += '<button class="btn btn-sm btn-outline-secondary flex-fill" onclick="VaiSketch.bringToFront()" title="' + I18n.t('vai.sketch.props.bringToFront') + '"><i class="bi bi-front"></i></button>';
        html += '<button class="btn btn-sm btn-outline-secondary flex-fill" onclick="VaiSketch.sendToBack()" title="' + I18n.t('vai.sketch.props.sendToBack') + '"><i class="bi bi-back"></i></button>';
        html += '</div>';
        html += '</div>';

        body.innerHTML = html;
        panel.style.display = 'flex';
    }

    function hidePropsPanel() {
        var panel = document.getElementById('vaiSketchProps');
        if (panel) panel.style.display = 'none';
    }

    function updateSelectedProp(prop, value) {
        if (!selectedObject) return;
        saveUndoState();
        selectedObject[prop] = value;
        redrawCanvas();
        // Refresh panel to reflect updated values (e.g. computed w/h for text)
        showPropsPanel(selectedObject);
    }

    function replaceSelectedImage(event) {
        if (!selectedObject || selectedObject.type !== 'image') return;
        var file = event.target.files[0];
        if (!file) return;
        event.target.value = '';

        var reader = new FileReader();
        reader.onload = function(e) {
            var localDataUrl = e.target.result;
            var img = new Image();
            img.onload = function() {
                saveUndoState();
                selectedObject._img = img;
                selectedObject.dataUrl = localDataUrl;
                // Keep position and size
                redrawCanvas();
                showPropsPanel(selectedObject);

                // Upload to server and replace inline dataUrl
                var obj = selectedObject;
                uploadImageToServer(localDataUrl).then(function(serverUrl) {
                    obj.dataUrl = serverUrl;
                });
            };
            img.src = localDataUrl;
        };
        reader.readAsDataURL(file);
    }

    // ─── Remove Background (Canvas-based) ──────────────────────────
    function removeImageBackground() {
        if (!selectedObject || selectedObject.type !== 'image' || !selectedObject._img) return;

        var btn = document.getElementById('vaiRemoveBgBtn');
        if (btn) {
            btn.disabled = true;
            btn.innerHTML = '<i class="bi bi-hourglass-split me-1"></i>' + I18n.t('vai.sketch.props.removingBackground');
        }

        try {
            saveUndoState();

            // Draw current image onto a temporary canvas at its natural size
            var srcImg = selectedObject._img;
            var tempCanvas = document.createElement('canvas');
            tempCanvas.width = srcImg.naturalWidth || srcImg.width;
            tempCanvas.height = srcImg.naturalHeight || srcImg.height;
            var ctx = tempCanvas.getContext('2d');
            ctx.drawImage(srcImg, 0, 0, tempCanvas.width, tempCanvas.height);

            var imageData = ctx.getImageData(0, 0, tempCanvas.width, tempCanvas.height);
            var data = imageData.data;

            // Sample corners to determine average background color
            var getPixel = function(x, y) {
                var idx = (y * tempCanvas.width + x) * 4;
                return { r: data[idx], g: data[idx + 1], b: data[idx + 2], a: data[idx + 3] };
            };

            // Sample multiple points along each edge for more robust detection
            var samples = [];
            var w = tempCanvas.width, h = tempCanvas.height;
            var edgeCount = 8; // sample points per edge
            for (var i = 0; i < edgeCount; i++) {
                var tx = Math.floor(w * i / edgeCount);
                var ty = Math.floor(h * i / edgeCount);
                samples.push(getPixel(tx, 0));           // top edge
                samples.push(getPixel(tx, h - 1));       // bottom edge
                samples.push(getPixel(0, ty));            // left edge
                samples.push(getPixel(w - 1, ty));        // right edge
            }

            // Compute average background color from edge samples
            var avgR = 0, avgG = 0, avgB = 0;
            samples.forEach(function(p) { avgR += p.r; avgG += p.g; avgB += p.b; });
            avgR = Math.floor(avgR / samples.length);
            avgG = Math.floor(avgG / samples.length);
            avgB = Math.floor(avgB / samples.length);

            // Color distance threshold — pixels within this distance are made transparent
            var threshold = 40;

            for (var i = 0; i < data.length; i += 4) {
                var dr = data[i] - avgR;
                var dg = data[i + 1] - avgG;
                var db = data[i + 2] - avgB;
                var distance = Math.sqrt(dr * dr + dg * dg + db * db);
                if (distance < threshold) {
                    data[i + 3] = 0; // set alpha to 0 (transparent)
                }
            }

            // Write processed data to a new canvas
            var newCanvas = document.createElement('canvas');
            newCanvas.width = tempCanvas.width;
            newCanvas.height = tempCanvas.height;
            var newCtx = newCanvas.getContext('2d');
            newCtx.putImageData(imageData, 0, 0);

            // Convert back to data URL and update the object
            var newDataUrl = newCanvas.toDataURL('image/png');
            var newImg = new Image();
            newImg.onload = function() {
                selectedObject._img = newImg;
                selectedObject.dataUrl = newDataUrl;
                redrawCanvas();
                showPropsPanel(selectedObject);
                if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.props.backgroundRemoved'), 'success');

                // Upload processed image to server
                var obj = selectedObject;
                uploadImageToServer(newDataUrl).then(function(serverUrl) {
                    obj.dataUrl = serverUrl;
                });
            };
            newImg.onerror = function() {
                if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.props.removeBackgroundFailed'), 'danger');
                if (btn) {
                    btn.disabled = false;
                    btn.innerHTML = '<i class="bi bi-magic me-1"></i>' + I18n.t('vai.sketch.props.removeBackground');
                }
            };
            newImg.src = newDataUrl;
        } catch (err) {
            console.error('Remove background error:', err);
            if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.props.removeBackgroundFailed') + ': ' + err.message, 'danger');
            if (btn) {
                btn.disabled = false;
                btn.innerHTML = '<i class="bi bi-magic me-1"></i>' + I18n.t('vai.sketch.props.removeBackground');
            }
        }
    }

    // ─── Text Tool ────────────────────────────────────────────────
    function showTextInput(x, y) {
        var textInput = document.getElementById('vaiSketchTextInput');
        if (!textInput) return;
        var wrapper = document.getElementById('vaiSketchCanvasWrapper');
        var wrapperRect = wrapper.getBoundingClientRect();
        var canvasRect = canvas.getBoundingClientRect();

        textInput.style.display = 'block';
        textInput.style.left = (x + canvasRect.left - wrapperRect.left) + 'px';
        textInput.style.top = (y + canvasRect.top - wrapperRect.top) + 'px';
        textInput.style.color = strokeColor;
        textInput.value = '';
        textInput.focus();

        // Remove old listener
        textInput.onkeydown = function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                var text = textInput.value.trim();
                if (text) {
                    saveUndoState();
                    objects.push({
                        type: 'text',
                        text: text,
                        x: x,
                        y: y,
                        w: 100,
                        h: 20,
                        color: strokeColor,
                        fontSize: Math.max(14, strokeWidth * 6),
                        zIndex: getNextZIndex()
                    });
                    redrawCanvas();
                }
                textInput.style.display = 'none';
                textInput.value = '';
            } else if (e.key === 'Escape') {
                textInput.style.display = 'none';
                textInput.value = '';
            }
        };
    }

    // ─── Image Upload to Server ──────────────────────────────────
    // Upload a base64 data URL to the server, returns a persistent /uploads/... URL.
    // Falls back to inline dataUrl on failure so the canvas still works.
    function uploadImageToServer(dataUrl) {
        if (!dataUrl || !dataUrl.startsWith('data:')) {
            // Already a URL — no upload needed
            return Promise.resolve(dataUrl);
        }
        return App.apiRequest('/ai/sketch-image-upload', {
            method: 'POST',
            body: JSON.stringify({ data_url: dataUrl })
        }).then(function(resp) {
            return resp.url || dataUrl; // fallback to inline if server didn't return url
        }).catch(function(err) {
            console.error('[VaiSketch] Image upload failed, keeping inline:', err);
            return dataUrl; // graceful fallback
        });
    }

    var productImagesCache = [];
    var imagePickerModal = null;

    function openImagePicker() {
        if (!imagePickerModal) {
            var el = document.getElementById('vaiImagePickerModal');
            if (el && typeof bootstrap !== 'undefined') {
                imagePickerModal = new bootstrap.Modal(el);
            }
        }
        if (imagePickerModal) {
            imagePickerModal.show();
            loadProductImages();
        }
    }

    function loadProductImages(search) {
        var grid = document.getElementById('vaiProductImgGrid');
        if (!grid) return;

        grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.common.loading') + '</div>';

        var url = '/products?limit=50';
        if (search && search.trim()) url += '&search=' + encodeURIComponent(search.trim());

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest(url).then(function(resp) {
                var products = resp.data || resp || [];
                productImagesCache = products;
                renderProductImageGrid(products);
            }).catch(function(err) {
                console.error('Failed to load products:', err);
                grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' + I18n.t('vai.common.loadFailed') + '</div>';
            });
        }
    }

    var productSearchTimer = null;
    function searchProductImages(query) {
        clearTimeout(productSearchTimer);
        productSearchTimer = setTimeout(function() {
            loadProductImages(query);
        }, 300);
    }

    function renderProductImageGrid(products) {
        var grid = document.getElementById('vaiProductImgGrid');
        if (!grid) return;

        // Filter to products with images
        var withImages = products.filter(function(p) { return p.image_url && p.image_url.trim(); });

        if (withImages.length === 0) {
            grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">' +
                '<i class="bi bi-image d-block mb-1" style="font-size: 1.5rem;"></i>' +
                I18n.t('vai.common.noProductImages') + '</div>';
            return;
        }

        grid.innerHTML = withImages.map(function(p) {
            return '<div class="vai-product-img-item" onclick="VaiSketch.addProductImageToCanvas(\'' + escapeHtml(p.image_url).replace(/'/g, "\\'") + '\')" title="' + escapeHtml(p.name) + '">' +
                '<img src="' + escapeHtml(p.image_url) + '" alt="' + escapeHtml(p.name) + '" loading="lazy">' +
                '<div class="vai-product-img-name">' + escapeHtml(p.name) + '</div>' +
            '</div>';
        }).join('');
    }

    function addProductImageToCanvas(imageUrl) {
        // Close the modal
        if (imagePickerModal) imagePickerModal.hide();

        var img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = function() {
            promptAspectRatioChange(img.width, img.height, function() {
                saveUndoState();
                var maxW = canvas.width * 0.6;
                var maxH = canvas.height * 0.6;
                var scale = Math.min(maxW / img.width, maxH / img.height, 1);
                var w = img.width * scale;
                var h = img.height * scale;
                var x = (canvas.width - w) / 2;
                var y = (canvas.height - h) / 2;

                var newObj = {
                    type: 'image',
                    x: x, y: y, w: w, h: h,
                    _img: img,
                    dataUrl: imageUrl,
                    zIndex: getNextZIndex()
                };
                objects.push(newObj);
                setTool('select');
                selectedObject = newObj;
                showPropsPanel(newObj);
                redrawCanvas();
            });
        };
        img.onerror = function() {
            if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.loadProductImageFailed'), 'danger');
        };
        img.src = imageUrl;
    }

    function handleImageUpload(event) {
        // Close the picker modal if open
        if (imagePickerModal) imagePickerModal.hide();

        var file = event.target.files[0];
        if (!file) return;
        event.target.value = '';

        var reader = new FileReader();
        reader.onload = function(e) {
            var localDataUrl = e.target.result;
            var img = new Image();
            img.onload = function() {
                promptAspectRatioChange(img.width, img.height, function() {
                    saveUndoState();
                    // Scale image to fit canvas (max 60% of canvas)
                    var maxW = canvas.width * 0.6;
                    var maxH = canvas.height * 0.6;
                    var scale = Math.min(maxW / img.width, maxH / img.height, 1);
                    var w = img.width * scale;
                    var h = img.height * scale;
                    // Center on canvas
                    var x = (canvas.width - w) / 2;
                    var y = (canvas.height - h) / 2;

                    var newObj = {
                        type: 'image',
                        x: x, y: y, w: w, h: h,
                        _img: img,
                        dataUrl: localDataUrl, // temporary inline until upload completes
                        zIndex: getNextZIndex()
                    };
                    objects.push(newObj);
                    // Auto-select for immediate resize
                    setTool('select');
                    selectedObject = newObj;
                    showPropsPanel(newObj);
                    redrawCanvas();

                    // Upload to server and replace inline dataUrl with persistent URL
                    uploadImageToServer(localDataUrl).then(function(serverUrl) {
                        newObj.dataUrl = serverUrl;
                    });
                });
            };
            img.src = localDataUrl;
        };
        reader.readAsDataURL(file);
    }

    // ─── Undo / Redo ──────────────────────────────────────────────
    function saveUndoState() {
        // Deep clone objects (excluding _img references, we keep dataUrl)
        undoStack.push(serializeObjects(objects));
        redoStack = [];
        isDirty = true;
        // Limit stack size
        if (undoStack.length > 50) undoStack.shift();
    }

    function undo() {
        if (undoStack.length === 0) return;
        redoStack.push(serializeObjects(objects));
        var state = undoStack.pop();
        objects = deserializeObjects(state);
        recalcNextZIndex();
        selectedObject = null;
        hidePropsPanel();
        redrawCanvas();
    }

    function redo() {
        if (redoStack.length === 0) return;
        undoStack.push(serializeObjects(objects));
        var state = redoStack.pop();
        objects = deserializeObjects(state);
        recalcNextZIndex();
        selectedObject = null;
        hidePropsPanel();
        redrawCanvas();
    }

    // ─── Serialization ────────────────────────────────────────────
    function serializeObjects(objs) {
        return JSON.parse(JSON.stringify(objs.map(function(o) {
            var clone = {};
            for (var k in o) {
                if (k === '_img') continue; // skip Image objects
                clone[k] = o[k];
            }
            return clone;
        })));
    }

    function deserializeObjects(data) {
        return data.map(function(o) {
            if (o.type === 'image' && o.dataUrl) {
                var img = new Image();
                img.onload = function() {
                    // Trigger redraw once remote image loads
                    redrawCanvas();
                };
                img.src = o.dataUrl;
                o._img = img;
            }
            return o;
        });
    }

    // ─── Clear Canvas ─────────────────────────────────────────────
    function clearCanvas() {
        if (objects.length === 0) return;
        saveUndoState();
        objects = [];
        nextZIndex = 1;
        selectedObject = null;
        hidePropsPanel();
        redrawCanvas();
    }

    function deleteCurrent() {
        if (!currentSketchId || currentSketchId === PENDING_SKETCH_ID) {
            // No saved sketch or pending — just clear canvas and remove pending
            if (currentSketchId === PENDING_SKETCH_ID) {
                sketches = sketches.filter(function(s) { return s.id !== PENDING_SKETCH_ID; });
                currentSketchId = null;
                renderSketchList();
            }
            clearCanvas();
            return;
        }
        if (!confirm(I18n.t('vai.sketch.deleteConfirmPermanent'))) return;
        deleteSketch(currentSketchId);
    }

    // ─── Save / Load Sketches ─────────────────────────────────────
    function save(options) {
        options = options || {};
        if (objects.length === 0) {
            if (!options.silent && typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.canvasEmpty'), 'info');
            return;
        }

        var title = currentSketchTitle || '';
        if (!currentSketchId || currentSketchId === PENDING_SKETCH_ID) {
            if (options.silent) {
                // Auto-save with a default title
                title = I18n.t('vai.sketch.sketchPrefix') + new Date().toLocaleDateString('zh-TW');
            } else {
                title = prompt(I18n.t('vai.sketch.enterSketchName'), I18n.t('vai.sketch.sketchPrefix') + new Date().toLocaleDateString('zh-TW'));
                if (!title || !title.trim()) return;
                title = title.trim();
            }
        }

        // Always compute aspect ratio from actual canvas dimensions so load
        // can restore the correct size even when the user chose "free".
        var saveRatio = simplifyRatio(canvas.width, canvas.height);
        var saveRatioStr = saveRatio.w + ':' + saveRatio.h;

        var data = {
            title: title,
            objects: serializeObjects(objects),
            canvas_width: canvas.width,
            canvas_height: canvas.height,
            aspect_ratio: saveRatioStr,
            thumbnail: generateThumbnail()
        };

        var isNewSketch = !currentSketchId || currentSketchId === PENDING_SKETCH_ID;
        var method = isNewSketch ? 'POST' : 'PUT';
        var url = isNewSketch ? '/ai/sketches' : '/ai/sketches/' + currentSketchId;

        App.apiRequest(url, {
            method: method,
            body: JSON.stringify(data)
        }).then(function(resp) {
            var newId = resp.id || currentSketchId;
            // Remove pending sketch from list if it was a new save
            if (isNewSketch) {
                sketches = sketches.filter(function(s) { return s.id !== PENDING_SKETCH_ID; });
            }
            currentSketchId = newId;
            currentSketchTitle = title;
            isDirty = false;
            if (!options.silent) App.showAlert(I18n.t('vai.sketch.saved'), 'success');
            loadSketchList();

            // If this was a newly created sketch, link any orphaned generation
            // records (created before the sketch was saved) to this sketch_id
            if (isNewSketch && newId) {
                App.apiRequest('/ai/sketch-generations/link-orphaned', {
                    method: 'PUT',
                    body: JSON.stringify({ sketch_id: newId })
                }).then(function(linkResp) {
                    if (linkResp.count > 0 && genHistoryOpen) {
                        loadGenerationHistory();
                    }
                }).catch(function(err) {
                    console.error('Failed to link orphaned generations:', err);
                });
            }
        }).catch(function(err) {
            console.error('Failed to save sketch:', err);
            var errMsg = (err && err.message) || I18n.t('vai.sketch.saveFailedRetry');
            App.showAlert(I18n.t('vai.common.saveFailed') + ': ' + errMsg, 'danger');
        });
    }

    function loadSketchList() {
        var container = document.getElementById('vaiSketchList');
        if (!container) return;

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketches').then(function(resp) {
                // 保留暫存草圖（如果有的話）
                var pendingSketch = sketches.find(function(s) { return s.id === PENDING_SKETCH_ID; });
                sketches = resp.data || resp || [];
                if (pendingSketch) {
                    sketches.unshift(pendingSketch);
                }
                renderSketchList();
                // Auto-create new sketch on page entry
                if (!currentSketchId && objects.length === 0) {
                    createNew();
                }
            }).catch(function(err) {
                console.error('Failed to load sketches:', err);
                sketches = [];
                renderSketchList();
                // Still auto-create on empty state
                if (!currentSketchId && objects.length === 0) {
                    createNew();
                }
            });
        }
    }

    function renderSketchList() {
        var container = document.getElementById('vaiSketchList');
        if (!container) return;

        if (sketches.length === 0) {
            container.innerHTML = '<div class="text-center text-muted p-3" style="font-size: 0.8rem;">' +
                '<i class="bi bi-brush mb-2 d-block" style="font-size: 1.5rem;"></i>' +
                '<span data-i18n="vai.sketch.noSketches">' + I18n.t('vai.sketch.noSketches') + '</span><br>' +
                '<span data-i18n="vai.sketch.noSketchesHint">' + I18n.t('vai.sketch.noSketchesHint') + '</span></div>';
            return;
        }

        container.innerHTML = sketches.map(function(s) {
            var isActive = s.id === currentSketchId;
            var timeStr = s.updated_at ? new Date(s.updated_at).toLocaleDateString('zh-TW') : '';
            return '<div class="vai-sketch-list-item ' + (isActive ? 'active' : '') + '" onclick="VaiSketch.loadSketch(\'' + s.id + '\')" data-sketch-id="' + s.id + '">' +
                '<i class="bi bi-file-earmark-image"></i>' +
                '<div class="sketch-info">' +
                    '<div class="sketch-title">' + escapeHtml(s.title || I18n.t('vai.common.unnamed')) + '</div>' +
                    '<div class="sketch-time">' + timeStr + '</div>' +
                '</div>' +
                '<button class="btn btn-sm btn-link text-danger p-0" onclick="event.stopPropagation(); VaiSketch.deleteSketch(\'' + s.id + '\')" title="' + I18n.t('vai.common.delete') + '">' +
                    '<i class="bi bi-trash" style="font-size: 0.75rem;"></i>' +
                '</button>' +
            '</div>';
        }).join('');
    }

    function loadSketch(id) {
        var sketch = sketches.find(function(s) { return s.id === id; });
        if (!sketch) return;

        // Clicking on the pending sketch just switches to it (empty canvas + library)
        if (id === PENDING_SKETCH_ID) {
            currentSketchId = PENDING_SKETCH_ID;
            currentSketchTitle = '';
            objects = [];
            nextZIndex = 1;
            selectedObject = null;
            isDirty = false;
            hidePropsPanel();
            redrawCanvas();
            renderSketchList();
            showImageLibrary();
            return;
        }

        // Warn if there are unsaved changes (but not when loading the same sketch)
        if (isDirty && id !== currentSketchId) {
            if (!confirm(I18n.t('vai.sketch.unsavedNew'))) return;
        }

        hideImageLibrary();

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketches/' + id).then(function(resp) {
                applySketchData(resp);
            }).catch(function(err) {
                console.error('Failed to load sketch:', err);
                App.showAlert(I18n.t('vai.sketch.loadSketchFailed'), 'danger');
            });
        }

        currentSketchId = id;
        currentSketchTitle = sketch.title || '';
        renderSketchList();
    }

    function applySketchData(data) {
        if (!data) return;

        // 1. Restore aspect ratio BEFORE resizing canvas.
        //    New saves always store the computed ratio (e.g. "7:9").
        //    Legacy saves may have "free" or empty — fall back to canvas_width/canvas_height.
        if (data.aspect_ratio && data.aspect_ratio !== 'free') {
            var parts = data.aspect_ratio.split(':');
            if (parts.length === 2) {
                aspectRatio = { w: parseInt(parts[0]), h: parseInt(parts[1]) };
            } else {
                aspectRatio = null;
            }
        } else if (data.canvas_width && data.canvas_height) {
            // Legacy fallback: compute ratio from saved dimensions
            var fallback = simplifyRatio(data.canvas_width, data.canvas_height);
            aspectRatio = { w: fallback.w, h: fallback.h };
        } else {
            aspectRatio = null;
        }

        // 2. Sync dropdown to match restored ratio
        if (aspectRatio) {
            syncAspectRatioSelect(aspectRatio.w, aspectRatio.h);
        } else {
            var sel = document.getElementById('vaiSketchAspectRatio');
            if (sel) sel.value = 'free';
        }

        // 3. Resize canvas to match the aspect ratio (fills wrapper)
        resizeCanvas();

        // 4. Deserialize objects
        objects = deserializeObjects(data.objects || []);

        // 5. Scale object coordinates if saved canvas size differs from current
        var savedW = data.canvas_width;
        var savedH = data.canvas_height;
        if (savedW && savedH && canvas &&
            (canvas.width !== savedW || canvas.height !== savedH)) {
            var scaleX = canvas.width / savedW;
            var scaleY = canvas.height / savedH;
            for (var i = 0; i < objects.length; i++) {
                var obj = objects[i];
                if (obj.x !== undefined) obj.x = Math.round(obj.x * scaleX);
                if (obj.y !== undefined) obj.y = Math.round(obj.y * scaleY);
                if (obj.w !== undefined) obj.w = Math.round(obj.w * scaleX);
                if (obj.h !== undefined) obj.h = Math.round(obj.h * scaleY);
                // Scale stroke/path points
                if (obj.type === 'stroke' && obj.points) {
                    for (var p = 0; p < obj.points.length; p++) {
                        obj.points[p].x = Math.round(obj.points[p].x * scaleX);
                        obj.points[p].y = Math.round(obj.points[p].y * scaleY);
                    }
                }
                // Scale text font size proportionally (use average scale)
                if (obj.type === 'text' && obj.fontSize) {
                    var avgScale = (scaleX + scaleY) / 2;
                    obj.fontSize = Math.round(obj.fontSize * avgScale);
                }
            }
        }

        recalcNextZIndex();
        undoStack = [];
        redoStack = [];
        selectedObject = null;
        isDirty = false;
        hidePropsPanel();
        redrawCanvas();
    }

    function deleteSketch(id) {
        // 暫存草圖直接從前端移除，不需呼叫 API
        if (id === PENDING_SKETCH_ID) {
            sketches = sketches.filter(function(s) { return s.id !== PENDING_SKETCH_ID; });
            if (currentSketchId === PENDING_SKETCH_ID) {
                currentSketchId = null;
                currentSketchTitle = '';
                objects = [];
                selectedObject = null;
                isDirty = false;
                redrawCanvas();
            }
            renderSketchList();
            return;
        }

        if (!confirm(I18n.t('vai.sketch.deleteConfirm'))) return;

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketches/' + id, { method: 'DELETE' }).then(function() {
                if (currentSketchId === id) {
                    currentSketchId = null;
                    currentSketchTitle = '';
                    objects = [];
                    selectedObject = null;
                    isDirty = false;
                    redrawCanvas();
                }
                loadSketchList();
            }).catch(function(err) {
                console.error('Failed to delete sketch:', err);
                App.showAlert(I18n.t('vai.common.deleteFailed'), 'danger');
            });
        }
    }

    function createNew() {
        if (isDirty) {
            if (!confirm(I18n.t('vai.sketch.unsavedNew'))) return;
        }

        // 如果已有 pending 草圖，直接切換過去
        if (sketches.some(function(s) { return s.id === PENDING_SKETCH_ID; })) {
            currentSketchId = PENDING_SKETCH_ID;
            currentSketchTitle = '';
            objects = [];
            nextZIndex = 1;
            selectedObject = null;
            undoStack = [];
            redoStack = [];
            isDirty = false;
            hidePropsPanel();
            redrawCanvas();
            renderSketchList();
            showImageLibrary();
            return;
        }

        // 建立暫存草圖項目，顯示在 sidebar
        var sketchTitle = I18n.t('vai.sketch.newSketch');
        if (!sketchTitle || sketchTitle === 'vai.sketch.newSketch') sketchTitle = '新圖片';
        var pendingSketch = {
            id: PENDING_SKETCH_ID,
            title: sketchTitle,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
            _pending: true
        };
        sketches.unshift(pendingSketch);

        currentSketchId = PENDING_SKETCH_ID;
        currentSketchTitle = '';
        objects = [];
        nextZIndex = 1;
        selectedObject = null;
        undoStack = [];
        redoStack = [];
        isDirty = false;
        hidePropsPanel();
        redrawCanvas();
        renderSketchList();
        showImageLibrary();
    }

    // ─── Image Library ────────────────────────────────────────────
    function showImageLibrary() {
        var overlay = document.getElementById('vaiSketchLibraryOverlay');
        if (!overlay) return;
        libraryActiveCategory = 'all';
        renderLibraryTabs();
        renderLibraryGrid();
        overlay.style.display = '';
    }

    function hideImageLibrary() {
        var overlay = document.getElementById('vaiSketchLibraryOverlay');
        if (overlay) overlay.style.display = 'none';
    }

    function renderLibraryTabs() {
        var container = document.getElementById('vaiSketchLibraryTabs');
        if (!container) return;

        // Determine which categories actually have items
        var catsWithItems = {};
        imageLibraryItems.forEach(function(item) {
            if (!item.isBlank) catsWithItems[item.category] = true;
        });

        container.innerHTML = imageLibraryCategories.map(function(cat) {
            // Always show 'all', hide categories with no items
            if (cat.id !== 'all' && !catsWithItems[cat.id]) return '';
            var active = cat.id === libraryActiveCategory ? ' active' : '';
            return '<button class="vai-sketch-library-tab' + active + '" data-cat="' + cat.id + '" onclick="VaiSketch.filterLibrary(\'' + cat.id + '\')">' +
                escapeHtml(cat.label) + '</button>';
        }).join('');
    }

    function renderLibraryGrid() {
        var container = document.getElementById('vaiSketchLibraryGrid');
        if (!container) return;

        var filtered = imageLibraryItems.filter(function(item) {
            if (libraryActiveCategory === 'all') return true;
            return item.category === libraryActiveCategory || item.isBlank;
        });

        container.innerHTML = filtered.map(function(item) {
            if (item.isBlank) {
                return '<div class="vai-sketch-library-item vai-sketch-library-blank" onclick="VaiSketch.selectLibraryItem(\'blank\')">' +
                    '<div class="vai-sketch-library-blank-inner">' +
                    '<i class="bi bi-file-earmark-plus"></i>' +
                    '<span>' + escapeHtml(item.title) + '</span>' +
                    '</div></div>';
            }
            return '<div class="vai-sketch-library-item" onclick="VaiSketch.selectLibraryItem(\'' + item.id + '\')">' +
                '<img src="' + escapeHtml(item.thumbnail) + '" alt="' + escapeHtml(item.title) + '" loading="lazy">' +
                '<div class="vai-sketch-library-item-title">' + escapeHtml(item.title) + '</div>' +
                '</div>';
        }).join('');
    }

    function filterLibrary(categoryId) {
        libraryActiveCategory = categoryId;
        renderLibraryTabs();
        renderLibraryGrid();
    }

    function selectLibraryItem(itemId) {
        hideImageLibrary();

        if (itemId === 'blank') {
            // User wants a blank canvas — do nothing, canvas is already empty
            return;
        }

        var item = imageLibraryItems.find(function(i) { return i.id === itemId; });
        if (!item || !item.thumbnail) return;

        // Load the template image onto the canvas as a reference image object
        var img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = function() {
            // Auto-adapt canvas aspect ratio to match the reference image
            var imgW = img.naturalWidth;
            var imgH = img.naturalHeight;
            var ratio = simplifyRatio(imgW, imgH);
            aspectRatio = { w: ratio.w, h: ratio.h };
            syncAspectRatioSelect(ratio.w, ratio.h);
            resizeCanvas();

            // Place image to fill the canvas
            var newObj = {
                type: 'image',
                x: 0,
                y: 0,
                w: canvas.width,
                h: canvas.height,
                _img: img,
                dataUrl: item.thumbnail,
                zIndex: getNextZIndex()
            };
            objects.push(newObj);
            setTool('select');
            selectedObject = newObj;
            showPropsPanel(newObj);
            saveUndoState();
            redrawCanvas();
            console.log('[VaiSketch] Library image loaded:', item.title, canvas.width + 'x' + canvas.height, '(' + ratio.w + ':' + ratio.h + ')');
        };
        img.onerror = function() {
            console.error('[VaiSketch] Failed to load library image:', item.thumbnail);
        };
        img.src = item.thumbnail;
    }

    // ─── Sidebar Tab Switching ────────────────────────────────────
    function switchSidebarTab(tab) {
        sidebarTab = tab;
        var sketchList = document.getElementById('vaiSketchList');
        var chatList = document.getElementById('vaiChatGenList');
        var tabs = document.querySelectorAll('.vai-sidebar-tab');
        tabs.forEach(function(t) {
            t.classList.toggle('active', t.getAttribute('data-tab') === tab);
        });
        if (tab === 'sketches') {
            if (sketchList) sketchList.style.display = '';
            if (chatList) chatList.style.display = 'none';
        } else {
            if (sketchList) sketchList.style.display = 'none';
            if (chatList) chatList.style.display = '';
            loadChatGenerations();
        }
    }

    function loadChatGenerations(reset) {
        if (chatGenLoading) return;
        var container = document.getElementById('vaiChatGenList');
        if (!container) return;

        if (reset === undefined) reset = true;
        if (reset) {
            chatGenerations = [];
            chatGenOffset = 0;
            chatGenHasMore = false;
            container.innerHTML = '<div class="text-center text-muted p-3" style="font-size: 0.8rem;"><span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.common.loading') + '</div>';
        }

        chatGenLoading = true;

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketch-generations?source=chat&limit=' + CHAT_GEN_LIMIT + '&offset=' + chatGenOffset).then(function(resp) {
                var items = resp.data || [];
                chatGenTotal = resp.total || 0;
                chatGenHasMore = resp.has_more || false;
                chatGenOffset += items.length;

                if (reset) {
                    chatGenerations = items;
                } else {
                    chatGenerations = chatGenerations.concat(items);
                }
                renderChatGenerations();
                chatGenLoading = false;
            }).catch(function(err) {
                console.error('Failed to load chat generations:', err);
                if (reset) {
                    chatGenerations = [];
                    renderChatGenerations();
                }
                chatGenLoading = false;
            });
        } else {
            chatGenLoading = false;
        }
    }

    function renderChatGenerations() {
        var container = document.getElementById('vaiChatGenList');
        if (!container) return;

        if (chatGenerations.length === 0) {
            container.innerHTML = '<div class="text-center text-muted p-3" style="font-size: 0.8rem;">' +
                '<i class="bi bi-chat-dots mb-2 d-block" style="font-size: 1.5rem;"></i>' +
                '<span data-i18n="vai.sketch.noChatGenerations">' + I18n.t('vai.sketch.noChatGenerations') + '</span><br><small><span data-i18n="vai.sketch.noChatGenerationsHint">' + I18n.t('vai.sketch.noChatGenerationsHint') + '</span></small></div>';
            return;
        }

        var html = chatGenerations.map(function(item) {
            var timeStr = item.created_at ? new Date(item.created_at).toLocaleString('zh-TW', {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'}) : '';
            var promptShort = (item.prompt || '').length > 30 ? item.prompt.substring(0, 30) + '...' : (item.prompt || '');
            var thumbHtml = '';
            if (item.has_image && item.image_url) {
                thumbHtml = '<img src="' + escapeHtml(item.image_url) + '" class="vai-chat-gen-thumb" alt="Generated" loading="lazy">';
            } else {
                thumbHtml = '<div class="vai-chat-gen-thumb-placeholder"><i class="bi bi-image"></i></div>';
            }
            return '<div class="vai-sketch-list-item vai-chat-gen-item" data-gen-id="' + item.id + '">' +
                thumbHtml +
                '<div class="sketch-info">' +
                    '<div class="sketch-title" title="' + escapeHtml(item.prompt || '') + '">' + escapeHtml(promptShort) + '</div>' +
                    '<div class="sketch-time">' + timeStr + '</div>' +
                '</div>' +
                '<div class="d-flex flex-column gap-1">' +
                    (item.has_image ? '<button class="btn btn-sm btn-link text-primary p-0" onclick="event.stopPropagation(); VaiSketch.replaceChatGenImage(\'' + item.id + '\')" title="' + I18n.t('vai.sketch.replaceCanvas') + '"><i class="bi bi-arrow-repeat" style="font-size: 0.8rem;"></i></button>' : '') +
                    (item.has_image ? '<button class="btn btn-sm btn-link text-primary p-0" onclick="event.stopPropagation(); VaiSketch.applyChatGenImage(\'' + item.id + '\')" title="' + I18n.t('vai.sketch.addToCanvas') + '"><i class="bi bi-plus-circle" style="font-size: 0.8rem;"></i></button>' : '') +
                    (item.has_image ? '<button class="btn btn-sm btn-link text-secondary p-0" onclick="event.stopPropagation(); VaiSketch.downloadFromGen(\'' + item.id + '\')" title="' + I18n.t('vai.common.download') + '"><i class="bi bi-download" style="font-size: 0.75rem;"></i></button>' : '') +
                    '<button class="btn btn-sm btn-link text-danger p-0" onclick="event.stopPropagation(); VaiSketch.deleteChatGeneration(\'' + item.id + '\')" title="' + I18n.t('vai.common.delete') + '"><i class="bi bi-trash" style="font-size: 0.75rem;"></i></button>' +
                '</div>' +
            '</div>';
        }).join('');

        // Load more indicator
        if (chatGenHasMore) {
            html += '<div class="text-center text-muted py-2 vai-gen-load-more" style="font-size: 0.75rem; cursor: pointer;" onclick="VaiSketch.loadMoreChatGen()">' +
                '<i class="bi bi-arrow-down-circle me-1"></i>' + I18n.t('vai.common.loadMore') + '</div>';
        }

        container.innerHTML = html;

        // Attach scroll listener for infinite scroll
        if (!container._scrollListenerAttached) {
            container.addEventListener('scroll', function() {
                if (chatGenLoading || !chatGenHasMore) return;
                if (container.scrollTop + container.clientHeight >= container.scrollHeight - 40) {
                    loadChatGenerations(false);
                }
            });
            container._scrollListenerAttached = true;
        }
    }

    function loadMoreChatGen() {
        if (!chatGenLoading && chatGenHasMore) {
            loadChatGenerations(false);
        }
    }

    function applyChatGenImage(id) {
        fetchGenImage(id, function(imgSrc) {
            applyAiImage(imgSrc);
        });
    }

    function replaceChatGenImage(id) {
        fetchGenImage(id, function(imgSrc) {
            replaceCanvasWithAiImage(imgSrc);
        });
    }

    function deleteChatGeneration(id) {
        if (!confirm(I18n.t('vai.sketch.deleteGenerationConfirm'))) return;

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketch-generations/' + id, { method: 'DELETE' }).then(function() {
                loadChatGenerations(true);
            }).catch(function(err) {
                console.error('Failed to delete chat generation:', err);
                App.showAlert(I18n.t('vai.common.deleteFailed'), 'danger');
            });
        }
    }

    function generateThumbnail() {
        if (!canvas) return '';
        try {
            // Create a small thumbnail
            var thumbCanvas = document.createElement('canvas');
            thumbCanvas.width = 200;
            thumbCanvas.height = 150;
            var thumbCtx = thumbCanvas.getContext('2d');
            thumbCtx.drawImage(canvas, 0, 0, 200, 150);
            return thumbCanvas.toDataURL('image/jpeg', 0.6);
        } catch(e) {
            return '';
        }
    }

    // ─── AI Generation ────────────────────────────────────────────
    function showGeneratingOverlay() {
        var wrapper = document.getElementById('vaiSketchCanvasWrapper');
        if (!wrapper) return;
        // Remove existing overlay if any
        hideGeneratingOverlay();
        var overlay = document.createElement('div');
        overlay.id = 'vaiSketchGeneratingOverlay';
        overlay.className = 'vai-sketch-generating-overlay';
        overlay.innerHTML =
            '<div class="vai-sketch-generating-content">' +
                '<div class="spinner-border text-primary" role="status" style="width: 2.5rem; height: 2.5rem;">' +
                    '<span class="visually-hidden">Generating...</span>' +
                '</div>' +
                '<p class="mt-3 mb-0">' + I18n.t('vai.sketch.aiGenerating') + '</p>' +
                '<small class="text-muted mt-1">' + I18n.t('vai.sketch.aiGeneratingHint') + '</small>' +
            '</div>';
        wrapper.appendChild(overlay);
    }

    function hideGeneratingOverlay() {
        var overlay = document.getElementById('vaiSketchGeneratingOverlay');
        if (overlay) overlay.remove();
    }

    // ─── Attachment (reference image for AI prompt) ─────────────
    function handleAttachment(event) {
        var file = event.target.files && event.target.files[0];
        if (!file) return;
        event.target.value = '';

        if (attachedImages.length >= 2) {
            if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.maxAttachments'), 'warning');
            return;
        }

        // Close the attach picker modal if open
        if (attachPickerModal) attachPickerModal.hide();

        var reader = new FileReader();
        reader.onload = function(e) {
            attachedImages.push({ dataUrl: e.target.result });
            renderAttachPreviews();
            updateSketchMicGenToggle();
        };
        reader.readAsDataURL(file);
    }

    function removeAttachment(index) {
        if (index >= 0 && index < attachedImages.length) {
            attachedImages.splice(index, 1);
        }
        renderAttachPreviews();
        updateSketchMicGenToggle();
    }

    function renderAttachPreviews() {
        var preview = document.getElementById('vaiSketchAttachPreview');
        if (!preview) return;
        if (attachedImages.length === 0) {
            preview.style.display = 'none';
            preview.innerHTML = '';
            return;
        }
        preview.style.display = '';
        var html = '';
        for (var i = 0; i < attachedImages.length; i++) {
            html += '<div class="vai-sketch-attach-item">' +
                '<img src="' + attachedImages[i].dataUrl + '" alt="ref">' +
                '<button type="button" class="vai-sketch-attach-remove" onclick="VaiSketch.removeAttachment(' + i + ')" title="' + I18n.t('vai.sketch.removeAttachment') + '">&times;</button>' +
            '</div>';
        }
        if (attachedImages.length < 2) {
            html += '<div class="vai-sketch-attach-add" onclick="document.getElementById(\'vaiSketchAttachInput\').click()" title="' + I18n.t('vai.sketch.attachRef') + '">+</div>';
        }
        preview.innerHTML = html;
    }

    function clearAllAttachments() {
        attachedImages = [];
        renderAttachPreviews();
        updateSketchMicGenToggle();
    }

    // ─── Attachment Picker (3-source: Upload, Sketch History, Products) ───
    var attachPickerModal = null;
    var attachProductSearchTimer = null;

    function openAttachPicker() {
        if (!attachPickerModal) {
            var el = document.getElementById('vaiSketchAttachPickerModal');
            if (el && typeof bootstrap !== 'undefined') {
                attachPickerModal = new bootstrap.Modal(el);
            }
        }
        if (attachPickerModal) {
            // Check attachment limit
            if (attachedImages.length >= 2) {
                if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.maxAttachments'), 'warning');
                return;
            }
            // Reset to Upload tab
            var firstTab = document.querySelector('#vaiSketchAttachPickerModal .nav-link:first-child');
            if (firstTab && typeof bootstrap !== 'undefined') {
                var tab = new bootstrap.Tab(firstTab);
                tab.show();
            }
            attachPickerModal.show();
            loadAttachHistory();
            loadAttachProducts();
        }
    }

    function loadAttachHistory() {
        var grid = document.getElementById('vaiSketchAttachHistoryGrid');
        if (!grid) return;
        grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.common.loading') + '</div>';
        App.apiRequest('/ai/sketches').then(function(resp) {
            var sketches = (resp.data || resp || []).filter(function(s) { return s.thumbnail && s.thumbnail.trim(); });
            if (sketches.length === 0) {
                grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><i class="bi bi-brush d-block mb-1" style="font-size: 1.5rem;"></i>' + I18n.t('vai.video.noSketchImages') + '</div>';
                return;
            }
            grid.innerHTML = sketches.map(function(s) {
                var safeUrl = (s.thumbnail || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                var title = (s.title || I18n.t('vai.video.imageFallbackTitle')).replace(/"/g, '&quot;');
                return '<div class="vai-product-img-item" onclick="VaiSketch.addAttachFromUrl(\'' + safeUrl + '\')" title="' + title + '">' +
                    '<img src="' + (s.thumbnail || '') + '" alt="' + title + '" loading="lazy">' +
                    '<div class="vai-product-img-name">' + title + '</div></div>';
            }).join('');
        }).catch(function() {
            grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">Failed to load</div>';
        });
    }

    function loadAttachProducts(search) {
        var grid = document.getElementById('vaiSketchAttachProductGrid');
        if (!grid) return;
        grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.common.loading') + '</div>';
        var url = '/products?limit=50';
        if (search && search.trim()) url += '&search=' + encodeURIComponent(search.trim());
        App.apiRequest(url).then(function(resp) {
            var products = (resp.data || resp || []).filter(function(p) { return p.image_url && p.image_url.trim(); });
            if (products.length === 0) {
                grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;"><i class="bi bi-image d-block mb-1" style="font-size: 1.5rem;"></i>No product images</div>';
                return;
            }
            grid.innerHTML = products.map(function(p) {
                var safeUrl = (p.image_url || '').replace(/'/g, "\\'").replace(/"/g, '&quot;');
                var name = (p.name || '').replace(/"/g, '&quot;');
                return '<div class="vai-product-img-item" onclick="VaiSketch.addAttachFromUrl(\'' + safeUrl + '\')" title="' + name + '">' +
                    '<img src="' + (p.image_url || '') + '" alt="' + name + '" loading="lazy">' +
                    '<div class="vai-product-img-name">' + name + '</div></div>';
            }).join('');
        }).catch(function() {
            grid.innerHTML = '<div class="text-center text-muted py-4" style="font-size: 0.85rem;">Failed to load</div>';
        });
    }

    function searchAttachProducts(query) {
        clearTimeout(attachProductSearchTimer);
        attachProductSearchTimer = setTimeout(function() {
            loadAttachProducts(query);
        }, 300);
    }

    function addAttachFromUrl(url) {
        if (attachedImages.length >= 2) {
            if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.maxAttachments'), 'warning');
            return;
        }
        if (attachPickerModal) attachPickerModal.hide();
        // Store URL directly — renderAttachPreviews handles both dataUrl and url
        attachedImages.push({ dataUrl: url });
        renderAttachPreviews();
        updateSketchMicGenToggle();
    }

    function generateWithAI() {
        var promptInput = document.getElementById('vaiSketchPrompt');
        var prompt = promptInput ? promptInput.value.trim() : '';
        if (!prompt) {
            if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.enterPrompt'), 'warning');
            return;
        }

        var genBtn = document.getElementById('vaiSketchGenBtn');
        if (genBtn) {
            genBtn.disabled = true;
            genBtn.innerHTML = '<span class="spinner-border spinner-border-sm"></span>';
        }

        // Hide generation history panel if open
        if (genHistoryOpen) {
            var histPanel = document.getElementById('vaiSketchGenHistory');
            if (histPanel) histPanel.style.display = 'none';
            genHistoryOpen = false;
            var histBtn = document.getElementById('vaiSketchHistoryBtn');
            if (histBtn) histBtn.classList.remove('active');
        }

        // Show loading overlay to prevent user modifications
        showGeneratingOverlay();

        // Get canvas as base64 image (only if user has drawn something)
        var canvasDataUrl = '';
        if (objects.length > 0) {
            try {
                canvasDataUrl = canvas.toDataURL('image/png');
            } catch(e) {
                console.error('Failed to get canvas data:', e);
            }
        }

        var requestBody = {
            prompt: prompt,
            sketch_image: canvasDataUrl,
            sketch_id: currentSketchId || '',
            model: 'gemini',
            reference_images: attachedImages.map(function(a) { return a.dataUrl; })
        };

        // Call backend AI generate endpoint
        App.apiRequest('/ai/sketch-generate', {
            method: 'POST',
            body: JSON.stringify(requestBody)
        }).then(function(resp) {
            hideGeneratingOverlay();
            clearAllAttachments(); // Clear after send
            if (genBtn) {
                genBtn.disabled = false;
                genBtn.innerHTML = '<i class="bi bi-stars"></i>';
            }
            updateSketchMicGenToggle();
            showAiResult(resp);
            // Auto-save after AI generation
            save({ silent: true });
            // Refresh generation history if panel is open
            if (genHistoryOpen) loadGenerationHistory();
        }).catch(function(err) {
            hideGeneratingOverlay();
            console.error('AI generation failed:', err);
            if (genBtn) {
                genBtn.disabled = false;
                genBtn.innerHTML = '<i class="bi bi-stars"></i>';
            }
            var errMsg = (err && err.message) || I18n.t('vai.sketch.generationFailedRetry');
            if (typeof App !== 'undefined') App.showAlert(I18n.t('vai.sketch.aiGenerationFailed') + ': ' + errMsg, 'danger');
        });
    }

    function showAiResult(resp) {
        var resultArea = document.getElementById('vaiSketchAiResult');
        var contentArea = document.getElementById('vaiSketchAiResultContent');
        if (!resultArea || !contentArea) return;

        resultArea.style.display = 'block';

        if (resp.image_url || resp.image_data) {
            var imgSrc = resp.image_url || ('data:image/png;base64,' + resp.image_data);
            var escapedSrc = imgSrc.replace(/'/g, "\\'");
            contentArea.innerHTML =
                '<img src="' + imgSrc + '" class="vai-sketch-ai-result-img" alt="AI Generated">' +
                '<div class="d-flex gap-2 mt-2 flex-wrap">' +
                    '<button class="btn btn-sm btn-primary" onclick="VaiSketch.replaceCanvasWithAiImage(\'' + escapedSrc + '\')"><i class="bi bi-arrow-repeat me-1"></i>' + I18n.t('vai.sketch.replaceCanvas') + '</button>' +
                    '<button class="btn btn-sm btn-outline-primary" onclick="VaiSketch.applyAiImage(\'' + escapedSrc + '\')"><i class="bi bi-plus-circle me-1"></i>' + I18n.t('vai.sketch.addToCanvas') + '</button>' +
                    '<a href="' + imgSrc + '" download="vai-sketch-ai.png" class="btn btn-sm btn-outline-secondary"><i class="bi bi-download me-1"></i>' + I18n.t('vai.common.download') + '</a>' +
                '</div>';
        } else if (resp.text) {
            contentArea.innerHTML = '<p class="text-muted small">' + escapeHtml(resp.text) + '</p>';
        } else {
            contentArea.innerHTML = '<p class="text-muted small">' + I18n.t('vai.sketch.noAiResult') + '</p>';
        }
    }

    function applyAiImage(imgSrc) {
        var img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = function() {
            promptAspectRatioChange(img.width, img.height, function() {
                saveUndoState();
                var maxW = canvas.width * 0.8;
                var maxH = canvas.height * 0.8;
                var scale = Math.min(maxW / img.width, maxH / img.height, 1);
                var w = img.width * scale;
                var h = img.height * scale;
                var x = (canvas.width - w) / 2;
                var y = (canvas.height - h) / 2;

                var newObj = {
                    type: 'image',
                    x: x, y: y, w: w, h: h,
                    _img: img,
                    dataUrl: imgSrc,
                    zIndex: getNextZIndex()
                };
                objects.push(newObj);
                redrawCanvas();
                setTool('select');
                selectedObject = newObj;
                redrawCanvas();
                closeAiResult();

                // Upload to server if it's a base64 data URL
                if (imgSrc && imgSrc.startsWith('data:')) {
                    uploadImageToServer(imgSrc).then(function(serverUrl) {
                        newObj.dataUrl = serverUrl;
                    });
                }
            });
        };
        img.src = imgSrc;
    }

    function replaceCanvasWithAiImage(imgSrc) {
        var img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = function() {
            // Auto-adjust aspect ratio to match the generated image
            var ratio = simplifyRatio(img.width, img.height);
            aspectRatio = { w: ratio.w, h: ratio.h };
            syncAspectRatioSelect(ratio.w, ratio.h);
            resizeCanvas();

            saveUndoState();
            // Clear all existing objects
            objects = [];
            nextZIndex = 1;
            selectedObject = null;
            hidePropsPanel();

            // Place image filling the entire canvas
            var newObj = {
                type: 'image',
                x: 0, y: 0, w: canvas.width, h: canvas.height,
                _img: img,
                dataUrl: imgSrc,
                zIndex: getNextZIndex()
            };
            objects.push(newObj);
            redrawCanvas();
            setTool('select');
            selectedObject = newObj;
            redrawCanvas();
            closeAiResult();

            // Upload to server if it's a base64 data URL
            if (imgSrc && imgSrc.startsWith('data:')) {
                uploadImageToServer(imgSrc).then(function(serverUrl) {
                    newObj.dataUrl = serverUrl;
                });
            }
        };
        img.src = imgSrc;
    }

    function closeAiResult() {
        var resultArea = document.getElementById('vaiSketchAiResult');
        if (resultArea) resultArea.style.display = 'none';
    }

    // ─── Generation History ───────────────────────────────────────
    var genHistoryOpen = false;
    var genHistoryItems = [];
    var genHistoryTotal = 0;
    var genHistoryOffset = 0;
    var genHistoryLoading = false;
    var genHistoryHasMore = false;
    var GEN_HISTORY_LIMIT = 20;

    function toggleGenerationHistory() {
        var panel = document.getElementById('vaiSketchGenHistory');
        if (!panel) return;
        genHistoryOpen = !genHistoryOpen;
        panel.style.display = genHistoryOpen ? 'block' : 'none';
        // Update button active state
        var btn = document.getElementById('vaiSketchHistoryBtn');
        if (btn) btn.classList.toggle('active', genHistoryOpen);
        if (genHistoryOpen) {
            genHistoryItems = [];
            genHistoryOffset = 0;
            genHistoryHasMore = false;
            loadGenerationHistory(true);
        }
    }

    function loadGenerationHistory(reset) {
        if (genHistoryLoading) return;
        var container = document.getElementById('vaiSketchGenHistoryList');
        if (!container) return;

        if (reset) {
            genHistoryItems = [];
            genHistoryOffset = 0;
            container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;"><span class="spinner-border spinner-border-sm me-1"></span>' + I18n.t('vai.common.loading') + '</div>';
        }

        genHistoryLoading = true;

        if (typeof App !== 'undefined' && App.apiRequest) {
            var url = '/ai/sketch-generations?limit=' + GEN_HISTORY_LIMIT + '&offset=' + genHistoryOffset;
            if (currentSketchId) {
                url += '&sketch_id=' + encodeURIComponent(currentSketchId);
            } else {
                // Show orphaned generations (created before sketch was saved)
                url += '&orphaned=true';
            }
            // Only show sketch-source generations in this panel
            url += '&source=sketch';
            App.apiRequest(url).then(function(resp) {
                var items = resp.data || [];
                genHistoryTotal = resp.total || 0;
                genHistoryHasMore = resp.has_more || false;
                genHistoryOffset += items.length;

                if (reset) {
                    genHistoryItems = items;
                } else {
                    genHistoryItems = genHistoryItems.concat(items);
                }
                renderGenerationHistory(reset);
                genHistoryLoading = false;
            }).catch(function(err) {
                console.error('Failed to load generation history:', err);
                if (reset) {
                    container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;">' + I18n.t('vai.common.loadFailed') + '</div>';
                }
                genHistoryLoading = false;
            });
        } else {
            genHistoryLoading = false;
        }
    }

    function renderGenerationHistory(reset) {
        var container = document.getElementById('vaiSketchGenHistoryList');
        if (!container) return;

        if (genHistoryItems.length === 0) {
            container.innerHTML = '<div class="text-center text-muted py-3" style="font-size: 0.8rem;">' +
                '<i class="bi bi-clock-history d-block mb-1" style="font-size: 1.2rem;"></i>' +
                I18n.t('vai.sketch.noGenHistory') + '</div>';
            return;
        }

        // Build HTML for current batch of items
        var startIdx = reset ? 0 : (genHistoryItems.length - (genHistoryOffset - (genHistoryItems.length - (genHistoryOffset))));
        // Simpler: always re-render all items (list is small per page)
        var html = genHistoryItems.map(function(item) {
            var timeStr = item.created_at ? new Date(item.created_at).toLocaleString('zh-TW', {month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'}) : '';
            var promptShort = (item.prompt || '').length > 40 ? item.prompt.substring(0, 40) + '...' : (item.prompt || '');
            var thumbHtml = '';
            if (item.has_image && item.image_url) {
                thumbHtml = '<img src="' + escapeHtml(item.image_url) + '" class="vai-gen-history-thumb" alt="Generated" loading="lazy">';
            } else {
                thumbHtml = '<div class="vai-gen-history-thumb-placeholder"><i class="bi bi-image"></i></div>';
            }
            return '<div class="vai-gen-history-item" data-gen-id="' + item.id + '">' +
                thumbHtml +
                '<div class="vai-gen-history-info">' +
                    '<div class="vai-gen-history-prompt" title="' + escapeHtml(item.prompt || '') + '">' + escapeHtml(promptShort) + '</div>' +
                    '<div class="vai-gen-history-time">' + timeStr + '</div>' +
                '</div>' +
                '<div class="vai-gen-history-actions">' +
                    (item.has_image ? '<button class="btn btn-sm btn-link text-primary p-0" onclick="VaiSketch.replaceCanvasFromGen(\'' + item.id + '\')" title="' + I18n.t('vai.sketch.replaceCanvas') + '"><i class="bi bi-arrow-repeat"></i></button>' : '') +
                    (item.has_image ? '<button class="btn btn-sm btn-link text-primary p-0 ms-1" onclick="VaiSketch.addToCanvasFromGen(\'' + item.id + '\')" title="' + I18n.t('vai.sketch.addToCanvas') + '"><i class="bi bi-plus-circle"></i></button>' : '') +
                    (item.has_image ? '<button class="btn btn-sm btn-link text-secondary p-0 ms-1" onclick="VaiSketch.downloadFromGen(\'' + item.id + '\')" title="' + I18n.t('vai.common.download') + '"><i class="bi bi-download" style="font-size: 0.75rem;"></i></button>' : '') +
                    '<button class="btn btn-sm btn-link text-danger p-0 ms-1" onclick="VaiSketch.deleteGeneration(\'' + item.id + '\')" title="' + I18n.t('vai.sketch.deleteGen') + '"><i class="bi bi-trash" style="font-size: 0.75rem;"></i></button>' +
                '</div>' +
            '</div>';
        }).join('');

        // Append load-more indicator if there are more items
        if (genHistoryHasMore) {
            html += '<div class="text-center text-muted py-2 vai-gen-load-more" style="font-size: 0.75rem; cursor: pointer;" onclick="VaiSketch.loadMoreHistory()">' +
                '<i class="bi bi-arrow-down-circle me-1"></i>' + I18n.t('vai.common.loadMore') + '</div>';
        }

        container.innerHTML = html;

        // Attach scroll listener for infinite scroll
        if (!container._scrollListenerAttached) {
            container.addEventListener('scroll', function() {
                if (genHistoryLoading || !genHistoryHasMore) return;
                if (container.scrollTop + container.clientHeight >= container.scrollHeight - 40) {
                    loadGenerationHistory(false);
                }
            });
            container._scrollListenerAttached = true;
        }
    }

    function loadMoreHistory() {
        if (!genHistoryLoading && genHistoryHasMore) {
            loadGenerationHistory(false);
        }
    }

    // Fetch full image from a generation and apply/replace/download
    function fetchGenImage(genId, callback) {
        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketch-generations/' + genId).then(function(data) {
                var img = data.result_image || '';
                if (img) {
                    callback(img);
                } else {
                    App.showAlert(I18n.t('vai.common.loadFailed'), 'warning');
                }
            }).catch(function(err) {
                console.error('Failed to fetch generation image:', err);
                App.showAlert(I18n.t('vai.common.loadFailed'), 'danger');
            });
        }
    }

    function addToCanvasFromGen(genId) {
        fetchGenImage(genId, function(imgSrc) {
            applyAiImage(imgSrc);
        });
    }

    function replaceCanvasFromGen(genId) {
        fetchGenImage(genId, function(imgSrc) {
            replaceCanvasWithAiImage(imgSrc);
        });
    }

    function downloadFromGen(genId) {
        fetchGenImage(genId, function(imgSrc) {
            downloadGeneration(imgSrc, genId);
        });
    }

    function downloadGeneration(imgSrc, genId) {
        if (!imgSrc) return;
        var filename = 'vai-sketch-' + (genId || Date.now()) + '.png';
        // For base64 data URLs, create a blob and trigger download
        if (imgSrc.startsWith('data:')) {
            try {
                var parts = imgSrc.split(',');
                var mime = parts[0].match(/:(.*?);/)[1] || 'image/png';
                var ext = mime.split('/')[1] || 'png';
                filename = 'vai-sketch-' + (genId || Date.now()) + '.' + ext;
                var byteStr = atob(parts[1]);
                var ab = new ArrayBuffer(byteStr.length);
                var ia = new Uint8Array(ab);
                for (var i = 0; i < byteStr.length; i++) {
                    ia[i] = byteStr.charCodeAt(i);
                }
                var blob = new Blob([ab], { type: mime });
                var url = URL.createObjectURL(blob);
                var a = document.createElement('a');
                a.href = url;
                a.download = filename;
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
            } catch (e) {
                console.error('Download failed:', e);
                // Fallback: open in new tab
                window.open(imgSrc, '_blank');
            }
        } else {
            // For server URLs, fetch as blob then download
            fetch(imgSrc).then(function(resp) {
                return resp.blob();
            }).then(function(blob) {
                var url = URL.createObjectURL(blob);
                var a = document.createElement('a');
                a.href = url;
                a.download = filename;
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
            }).catch(function(e) {
                console.error('Download failed:', e);
                // Fallback: simple link download
                var a = document.createElement('a');
                a.href = imgSrc;
                a.download = filename;
                a.target = '_blank';
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
            });
        }
    }

    function deleteGeneration(id) {
        if (!confirm(I18n.t('vai.sketch.deleteGenerationConfirm'))) return;

        if (typeof App !== 'undefined' && App.apiRequest) {
            App.apiRequest('/ai/sketch-generations/' + id, { method: 'DELETE' }).then(function() {
                loadGenerationHistory(true);
            }).catch(function(err) {
                console.error('Failed to delete generation:', err);
                App.showAlert(I18n.t('vai.common.deleteFailed'), 'danger');
            });
        }
    }

    // ─── Sidebar Toggle (mobile) ──────────────────────────────────
    function toggleSidebar() {
        var sidebar = document.getElementById('vaiSketchSidebar');
        if (!sidebar) return;
        sidebar.classList.toggle('show');
        // Manage backdrop overlay
        var backdrop = document.getElementById('vaiSketchSidebarBackdrop');
        if (sidebar.classList.contains('show')) {
            if (!backdrop) {
                backdrop = document.createElement('div');
                backdrop.id = 'vaiSketchSidebarBackdrop';
                backdrop.className = 'vai-sidebar-backdrop';
                backdrop.onclick = function() { toggleSidebar(); };
                sidebar.parentElement.appendChild(backdrop);
            }
            backdrop.style.display = 'block';
        } else {
            if (backdrop) backdrop.style.display = 'none';
        }
    }

    // ─── Utilities ────────────────────────────────────────────────
    function escapeHtml(text) {
        var div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // WhatsApp-style toggle: no text → show mic, hide gen; has text → show gen, hide mic
    function updateSketchMicGenToggle() {
        var promptInput = document.getElementById('vaiSketchPrompt');
        var micBtn = document.querySelector('#vaiSketchAiBar .vai-stt-btn');
        var genBtn = document.getElementById('vaiSketchGenBtn');
        if (!micBtn || !genBtn) return;
        var hasText = promptInput && promptInput.value.trim().length > 0;
        if (hasText) {
            micBtn.style.display = 'none';
            genBtn.style.display = '';
        } else {
            micBtn.style.display = '';
            genBtn.style.display = 'none';
        }
    }

    // ─── Public API ───────────────────────────────────────────────
    return {
        init: init,
        setTool: setTool,
        setStrokeColor: setStrokeColor,
        setFillColor: setFillColor,
        setStrokeWidth: setStrokeWidth,
        setAspectRatio: setAspectRatio,
        openImagePicker: openImagePicker,
        handleImageUpload: handleImageUpload,
        searchProductImages: searchProductImages,
        addProductImageToCanvas: addProductImageToCanvas,
        handleAttachment: handleAttachment,
        removeAttachment: removeAttachment,
        undo: undo,
        redo: redo,
        clearCanvas: clearCanvas,
        deleteCurrent: deleteCurrent,
        save: save,
        loadSketch: loadSketch,
        deleteSketch: deleteSketch,
        deleteSelected: deleteSelected,
        createNew: createNew,
        generateWithAI: generateWithAI,
        applyAiImage: applyAiImage,
        replaceCanvasWithAiImage: replaceCanvasWithAiImage,
        closeAiResult: closeAiResult,
        toggleSidebar: toggleSidebar,
        // Image Library
        showImageLibrary: showImageLibrary,
        hideImageLibrary: hideImageLibrary,
        filterLibrary: filterLibrary,
        selectLibraryItem: selectLibraryItem,
        // Sidebar tabs (sketches vs chat generations)
        switchSidebarTab: switchSidebarTab,
        applyChatGenImage: applyChatGenImage,
        replaceChatGenImage: replaceChatGenImage,
        deleteChatGeneration: deleteChatGeneration,
        loadMoreChatGen: loadMoreChatGen,
        // Generation History
        toggleGenerationHistory: toggleGenerationHistory,
        loadMoreHistory: loadMoreHistory,
        addToCanvasFromGen: addToCanvasFromGen,
        replaceCanvasFromGen: replaceCanvasFromGen,
        downloadFromGen: downloadFromGen,
        deleteGeneration: deleteGeneration,
        downloadGeneration: downloadGeneration,
        // Properties panel
        deselectObject: deselectObject,
        duplicateSelected: duplicateSelected,
        updateSelectedProp: updateSelectedProp,
        replaceSelectedImage: replaceSelectedImage,
        removeImageBackground: removeImageBackground,
        bringToFront: bringToFront,
        sendToBack: sendToBack,
        // Attachment picker (3-source)
        openAttachPicker: openAttachPicker,
        addAttachFromUrl: addAttachFromUrl,
        searchAttachProducts: searchAttachProducts
    };
})();
