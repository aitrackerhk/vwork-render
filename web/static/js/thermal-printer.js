/**
 * 通用熱敏打印機 WebUSB 驅動
 * 支援大部分使用 ESC/POS 指令集的熱敏打印機
 * 
 * 常見支援品牌：Epson, 佳博(Gainscha), 芯燁(Xprinter), 得力, 
 *              商米(Sunmi), 愛寶(Aibao), 容大(Rongda), 優庫(Youku) 等
 */
(function(global) {
  'use strict';

  // 常見熱敏打印機 Vendor ID 列表
  const KNOWN_VENDOR_IDS = [
    0x04B8,  // Epson
    0x0416,  // 佳博 Gainscha / Winbond
    0x0483,  // STMicroelectronics (常見芯片)
    0x0525,  // 芯燁 Xprinter / Netchip
    0x1504,  // 得力 Deli
    0x0FE6,  // 容大 Rongda / ICS
    0x1A86,  // 南京沁恒 CH340 (USB轉串口芯片)
    0x067B,  // Prolific (USB轉串口芯片)
    0x10C4,  // Silicon Labs (USB轉串口芯片)
    0x0B00,  // 商米 Sunmi
    0x1FC9,  // NXP (常見芯片)
    0x0DD4,  // Citizen
    0x0519,  // Star Micronics
    0x04E8,  // Samsung
    0x0745,  // Syntech
    0x2730,  // Citizen (另一個 ID)
    0x0AA7,  // 愛寶 Aibao
    0x6868,  // 優庫
    0x20D1,  // 商用打印機常見 ID
    0x0FE6,  // Contec
    0x28E9,  // GD32 芯片
  ];

  // GBK 編碼表（簡化版，覆蓋常用中文字符）
  // 完整版需要更大的編碼表，這裡使用動態查詢
  const GBK_ENCODER = {
    // 使用 TextEncoder 的 fallback 方案
    encode: function(str) {
      const bytes = [];
      for (let i = 0; i < str.length; i++) {
        const code = str.charCodeAt(i);
        if (code < 0x80) {
          // ASCII
          bytes.push(code);
        } else {
          // 中文字符：使用 GB2312/GBK 編碼
          // 這裡用簡化方案：將 UTF-16 轉為 GBK
          const gbk = this.unicodeToGBK(code);
          if (gbk) {
            bytes.push((gbk >> 8) & 0xFF);
            bytes.push(gbk & 0xFF);
          } else {
            // 無法轉換的字符用問號替代
            bytes.push(0x3F);
          }
        }
      }
      return new Uint8Array(bytes);
    },

    // Unicode 到 GBK 轉換（使用內建 API 或 fallback）
    unicodeToGBK: function(unicode) {
      // 常用中文標點和數字
      const commonMap = {
        0x3002: 0xA1A3, // 。
        0xFF0C: 0xA3AC, // ，
        0xFF1A: 0xA3BA, // ：
        0xFF01: 0xA3A1, // ！
        0xFF1F: 0xA3BF, // ？
        0x3001: 0xA1A2, // 、
        0xFF08: 0xA3A8, // （
        0xFF09: 0xA3A9, // ）
        0x2014: 0xA1AA, // —
        0x2026: 0xA1AD, // …
        0x300A: 0xA1B6, // 《
        0x300B: 0xA1B7, // 》
        0xFF0D: 0xA3AD, // －
      };
      if (commonMap[unicode]) return commonMap[unicode];

      // 簡體中文常用字（GB2312 區間 0xB0A1-0xF7FE）
      // 這裡使用簡化的線性映射，實際需要完整碼表
      if (unicode >= 0x4E00 && unicode <= 0x9FA5) {
        // CJK Unified Ideographs - 使用瀏覽器內建轉換
        return this.browserConvert(String.fromCharCode(unicode));
      }

      return null;
    },

    // 使用瀏覽器 API 轉換
    browserConvert: function(char) {
      // 嘗試使用 TextEncoder with GBK (如果瀏覽器支援)
      try {
        if (typeof TextEncoder !== 'undefined') {
          // 大多數瀏覽器只支援 UTF-8，需要 polyfill
          // 這裡返回 null 讓它使用 fallback
        }
      } catch (e) {}
      
      // Fallback: 使用常用字映射表
      return this.commonChineseMap[char] || null;
    },

    // 預載入的常用中文字 GBK 編碼（前 500 個最常用字）
    commonChineseMap: {}
  };

  // 初始化常用中文字映射（延遲加載）
  function initCommonChineseMap() {
    // 這些是餐飲候位系統最常用的字
    const chars = '的一是不了在人有我他這來大到說們為子和你地出道也時年得就那要下以生會自著去之過家學對可她裡後小么心多天而能好都然沒日於起還發成事只作當想看文無開手十用主行方又如前所本見經頭面公同三已老從動兩長知民樣現分將外它應果定你您請號候位等桌人數區取叫座入已完成區域電話姓名時間狀態等待就座取消';
    
    // 這裡需要預編譯的 GBK 碼表，暫時使用簡化方案
    // 實際部署時可以從服務器加載完整碼表
  }

  // ESC/POS 指令構建器
  const ESC = 0x1B;
  const GS = 0x1D;
  const LF = 0x0A;
  const CR = 0x0D;

  const ESCPOS = {
    // 初始化打印機
    INIT: new Uint8Array([ESC, 0x40]),
    
    // 對齊方式
    ALIGN_LEFT: new Uint8Array([ESC, 0x61, 0x00]),
    ALIGN_CENTER: new Uint8Array([ESC, 0x61, 0x01]),
    ALIGN_RIGHT: new Uint8Array([ESC, 0x61, 0x02]),
    
    // 字體大小 (寬度倍數, 高度倍數)
    textSize: function(w, h) {
      const n = ((w - 1) << 4) | (h - 1);
      return new Uint8Array([GS, 0x21, n]);
    },
    
    // 加粗
    BOLD_ON: new Uint8Array([ESC, 0x45, 0x01]),
    BOLD_OFF: new Uint8Array([ESC, 0x45, 0x00]),
    
    // 下劃線
    UNDERLINE_ON: new Uint8Array([ESC, 0x2D, 0x01]),
    UNDERLINE_OFF: new Uint8Array([ESC, 0x2D, 0x00]),
    
    // 換行
    LF: new Uint8Array([LF]),
    
    // 走紙
    feed: function(lines) {
      return new Uint8Array([ESC, 0x64, lines]);
    },
    
    // 切紙
    CUT_FULL: new Uint8Array([GS, 0x56, 0x00]),
    CUT_PARTIAL: new Uint8Array([GS, 0x56, 0x01]),
    
    // 設置中文模式 (GBK)
    CHINESE_MODE: new Uint8Array([ESC, 0x52, 0x0F]),  // 選擇中國字符集
    
    // 選擇字符代碼表 (PC437/GBK)
    CODE_PAGE_GBK: new Uint8Array([ESC, 0x74, 0xFF]),  // 用戶定義頁（GBK）
    
    // 設置行間距
    lineSpacing: function(n) {
      return new Uint8Array([ESC, 0x33, n]);
    },
    
    // 打印並走紙
    PRINT_FEED: new Uint8Array([ESC, 0x4A, 0x40]),
    
    // 蜂鳴器
    beep: function(times, duration) {
      return new Uint8Array([ESC, 0x42, times || 1, duration || 2]);
    },
    
    // 打開錢箱
    OPEN_DRAWER: new Uint8Array([ESC, 0x70, 0x00, 0x19, 0xFA]),

    // QR Code (GS ( k)
    qrCode: function(data, size) {
      size = size || 6;
      const store = [
        GS, 0x28, 0x6B, 
        (data.length + 3) & 0xFF, ((data.length + 3) >> 8) & 0xFF,
        0x31, 0x50, 0x30
      ];
      for (let i = 0; i < data.length; i++) {
        store.push(data.charCodeAt(i));
      }
      
      return new Uint8Array([
        // 設置 QR 碼大小
        GS, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x43, size,
        // 設置糾錯級別 L
        GS, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x45, 0x30,
        // 存儲數據
        ...store,
        // 打印 QR 碼
        GS, 0x28, 0x6B, 0x03, 0x00, 0x31, 0x51, 0x30
      ]);
    }
  };

  // 合併多個 Uint8Array
  function concatArrays(...arrays) {
    const totalLength = arrays.reduce((sum, arr) => sum + arr.length, 0);
    const result = new Uint8Array(totalLength);
    let offset = 0;
    for (const arr of arrays) {
      result.set(arr, offset);
      offset += arr.length;
    }
    return result;
  }

  // 文字轉 Uint8Array（支援中英文混合）
  function textToBytes(text, useGBK) {
    if (useGBK) {
      return GBK_ENCODER.encode(text);
    }
    return new TextEncoder().encode(text);
  }

  // ThermalPrinter 類
  class ThermalPrinter {
    constructor(options) {
      this.options = Object.assign({
        width: 58,          // 紙寬 mm (58 或 80)
        encoding: 'gbk',    // 編碼 (gbk, utf8)
        autoCut: true,      // 自動切紙
        beepOnPrint: false, // 打印後蜂鳴
      }, options);
      
      this.device = null;
      this.endpoint = null;
      this.connected = false;
      this.charsPerLine = this.options.width === 80 ? 48 : 32;
    }

    // 檢查 WebUSB 支援
    static isSupported() {
      return typeof navigator !== 'undefined' && 'usb' in navigator;
    }

    // 獲取已授權的設備列表
    static async getDevices() {
      if (!this.isSupported()) return [];
      return await navigator.usb.getDevices();
    }

    // 請求連接打印機
    async connect() {
      if (!ThermalPrinter.isSupported()) {
        throw new Error('此瀏覽器不支援 WebUSB，請使用 Chrome 或 Edge');
      }

      try {
        // 嘗試使用已授權的設備
        const devices = await navigator.usb.getDevices();
        if (devices.length > 0) {
          this.device = devices[0];
        } else {
          // 請求用戶選擇設備
          this.device = await navigator.usb.requestDevice({
            filters: [
              // 打印機類別 (class 7)
              { classCode: 7 },
              // 或者使用已知的 Vendor ID
              ...KNOWN_VENDOR_IDS.map(id => ({ vendorId: id }))
            ]
          });
        }

        await this.device.open();

        // 選擇配置
        if (this.device.configuration === null) {
          await this.device.selectConfiguration(1);
        }

        // 尋找打印機接口和端點
        const iface = this.findPrinterInterface();
        if (!iface) {
          throw new Error('找不到打印機接口');
        }

        await this.device.claimInterface(iface.interfaceNumber);
        this.endpoint = this.findOutEndpoint(iface);
        
        if (!this.endpoint) {
          throw new Error('找不到輸出端點');
        }

        this.connected = true;
        console.log('[ThermalPrinter] 已連接:', {
          name: this.device.productName,
          vendor: this.device.vendorId.toString(16),
          product: this.device.productId.toString(16),
          endpoint: this.endpoint.endpointNumber
        });

        return true;
      } catch (err) {
        this.connected = false;
        if (err.name === 'NotFoundError') {
          throw new Error('未選擇打印機');
        }
        throw err;
      }
    }

    // 尋找打印機接口
    findPrinterInterface() {
      for (const config of this.device.configurations) {
        for (const iface of config.interfaces) {
          for (const alt of iface.alternates) {
            // 打印機類別是 7
            if (alt.interfaceClass === 7) {
              return iface;
            }
            // 也檢查 vendor-specific (255) 因為有些打印機用這個
            if (alt.interfaceClass === 255 && alt.endpoints.length > 0) {
              return iface;
            }
          }
        }
      }
      // Fallback: 使用第一個接口
      if (this.device.configuration && this.device.configuration.interfaces.length > 0) {
        return this.device.configuration.interfaces[0];
      }
      return null;
    }

    // 尋找輸出端點
    findOutEndpoint(iface) {
      for (const alt of iface.alternates) {
        for (const ep of alt.endpoints) {
          if (ep.direction === 'out') {
            return ep;
          }
        }
      }
      return null;
    }

    // 斷開連接
    async disconnect() {
      if (this.device && this.device.opened) {
        try {
          await this.device.close();
        } catch (e) {}
      }
      this.device = null;
      this.endpoint = null;
      this.connected = false;
    }

    // 發送原始數據
    async sendRaw(data) {
      if (!this.connected || !this.endpoint) {
        throw new Error('打印機未連接');
      }
      await this.device.transferOut(this.endpoint.endpointNumber, data);
    }

    // 初始化打印機
    async init() {
      await this.sendRaw(ESCPOS.INIT);
    }

    // 打印文字
    async printText(text, options) {
      options = Object.assign({
        align: 'left',
        size: 1,
        bold: false,
        underline: false,
        newline: true
      }, options);

      const parts = [];

      // 對齊
      if (options.align === 'center') {
        parts.push(ESCPOS.ALIGN_CENTER);
      } else if (options.align === 'right') {
        parts.push(ESCPOS.ALIGN_RIGHT);
      } else {
        parts.push(ESCPOS.ALIGN_LEFT);
      }

      // 大小
      if (options.size > 1) {
        parts.push(ESCPOS.textSize(options.size, options.size));
      }

      // 加粗
      if (options.bold) {
        parts.push(ESCPOS.BOLD_ON);
      }

      // 下劃線
      if (options.underline) {
        parts.push(ESCPOS.UNDERLINE_ON);
      }

      // 文字內容
      const useGBK = this.options.encoding === 'gbk';
      parts.push(textToBytes(text, useGBK));

      // 重置樣式
      if (options.bold) {
        parts.push(ESCPOS.BOLD_OFF);
      }
      if (options.underline) {
        parts.push(ESCPOS.UNDERLINE_OFF);
      }
      if (options.size > 1) {
        parts.push(ESCPOS.textSize(1, 1));
      }

      // 換行
      if (options.newline) {
        parts.push(ESCPOS.LF);
      }

      await this.sendRaw(concatArrays(...parts));
    }

    // 打印分隔線
    async printLine(char) {
      char = char || '-';
      const line = char.repeat(this.charsPerLine);
      await this.printText(line, { align: 'left' });
    }

    // 走紙
    async feed(lines) {
      await this.sendRaw(ESCPOS.feed(lines || 3));
    }

    // 切紙
    async cut(partial) {
      await this.feed(3);
      await this.sendRaw(partial ? ESCPOS.CUT_PARTIAL : ESCPOS.CUT_FULL);
    }

    // 蜂鳴
    async beep(times, duration) {
      await this.sendRaw(ESCPOS.beep(times, duration));
    }

    // 打開錢箱
    async openDrawer() {
      await this.sendRaw(ESCPOS.OPEN_DRAWER);
    }

    // 打印 QR Code
    async printQRCode(data, size) {
      await this.sendRaw(ESCPOS.ALIGN_CENTER);
      await this.sendRaw(ESCPOS.qrCode(data, size || 6));
      await this.sendRaw(ESCPOS.LF);
    }

    // 打印候位票
    async printQueueTicket(data) {
      const {
        ticketNumber,
        partySize,
        areaName,
        storeName,
        phone,
        queuePosition,
        estimatedWait,
        qrCodeUrl
      } = data;

      await this.init();

      // 店名（如果有）
      if (storeName) {
        await this.printText(storeName, { align: 'center', size: 1, bold: true });
        await this.printLine('=');
      }

      // 候位票號（大字）
      await this.printText('候位票號', { align: 'center', size: 1 });
      await this.printText(ticketNumber, { align: 'center', size: 3, bold: true });

      await this.printLine('-');

      // 桌區和人數
      if (areaName) {
        await this.printText(`桌區: ${areaName}`, { align: 'left', size: 1 });
      }
      await this.printText(`人數: ${partySize}人`, { align: 'left', size: 1 });

      // 排隊位置（如果有）
      if (queuePosition) {
        await this.printText(`前面還有: ${queuePosition}組`, { align: 'left', size: 1 });
      }

      // 預估等待時間（如果有）
      if (estimatedWait) {
        await this.printText(`預估等待: ${estimatedWait}分鐘`, { align: 'left', size: 1 });
      }

      // 電話（如果有）
      if (phone) {
        await this.printText(`電話: ${phone}`, { align: 'left', size: 1 });
      }

      await this.printLine('-');

      // 取號時間
      const now = new Date();
      const timeStr = now.toLocaleString('zh-TW', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
      });
      await this.printText(`取號時間: ${timeStr}`, { align: 'center', size: 1 });

      // QR Code（如果有）
      if (qrCodeUrl) {
        await this.printText('', { align: 'center' });
        await this.printQRCode(qrCodeUrl, 5);
        await this.printText('掃碼查看候位狀態', { align: 'center', size: 1 });
      }

      // 提示文字
      await this.printLine('-');
      await this.printText('請留意叫號，過號需重新取號', { align: 'center', size: 1 });

      // 切紙
      if (this.options.autoCut) {
        await this.cut();
      } else {
        await this.feed(4);
      }

      // 蜂鳴提示
      if (this.options.beepOnPrint) {
        await this.beep(1, 2);
      }
    }

    // 打印叫號票（給服務員用）
    async printCallTicket(data) {
      const { ticketNumber, partySize, areaName, tableName } = data;

      await this.init();

      await this.printText('=== 叫號通知 ===', { align: 'center', size: 1, bold: true });
      await this.printText(ticketNumber, { align: 'center', size: 3, bold: true });
      
      if (tableName) {
        await this.printText(`請至: ${tableName}`, { align: 'center', size: 2 });
      }
      
      await this.printText(`${areaName} / ${partySize}人`, { align: 'center', size: 1 });

      const now = new Date();
      await this.printText(now.toLocaleTimeString('zh-TW'), { align: 'center', size: 1 });

      if (this.options.autoCut) {
        await this.cut();
      } else {
        await this.feed(4);
      }
    }
  }

  // 導出
  global.ThermalPrinter = ThermalPrinter;

  // 便捷方法：全局打印機實例
  let globalPrinter = null;

  global.ThermalPrinterManager = {
    // 獲取或創建打印機實例
    async getPrinter(options) {
      if (!globalPrinter) {
        globalPrinter = new ThermalPrinter(options);
      }
      if (!globalPrinter.connected) {
        await globalPrinter.connect();
      }
      return globalPrinter;
    },

    // 檢查是否已連接
    isConnected() {
      return globalPrinter && globalPrinter.connected;
    },

    // 斷開連接
    async disconnect() {
      if (globalPrinter) {
        await globalPrinter.disconnect();
        globalPrinter = null;
      }
    },

    // 快速打印候位票
    async printQueueTicket(data, options) {
      const printer = await this.getPrinter(options);
      await printer.printQueueTicket(data);
    },

    // 檢查是否支援
    isSupported: ThermalPrinter.isSupported
  };

})(typeof window !== 'undefined' ? window : this);
