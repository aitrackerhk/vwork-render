/**
 * vWork Connector - 本地硬件連接服務
 * 與 vWork Connector.exe 本地程序通信
 * 用於連接打印機、卡機(Kpay等)等外部設備
 */
(function() {
    'use strict';

    const DEFAULT_PORT = 9527;
    const STORAGE_KEY = 'vwork_connector_config';

    // 連接狀態
    const ConnectionStatus = {
        DISCONNECTED: 'disconnected',
        CONNECTING: 'connecting',
        CONNECTED: 'connected',
        ERROR: 'error'
    };

    class VWorkConnector {
        constructor() {
            this.port = DEFAULT_PORT;
            this.baseUrl = `http://localhost:${this.port}`;
            this.status = ConnectionStatus.DISCONNECTED;
            this.lastError = null;
            this.printers = [];
            this.cardTerminals = [];
            this.listeners = {
                statusChange: [],
                printersUpdate: [],
                cardTerminalUpdate: []
            };
            
            this._loadConfig();
        }

        /**
         * 載入配置
         */
        _loadConfig() {
            try {
                const saved = localStorage.getItem(STORAGE_KEY);
                if (saved) {
                    const config = JSON.parse(saved);
                    this.port = config.port || DEFAULT_PORT;
                    this.baseUrl = `http://localhost:${this.port}`;
                }
            } catch (e) {
                console.warn('[VWorkConnector] 載入配置失敗:', e);
            }
        }

        /**
         * 保存配置
         */
        _saveConfig() {
            try {
                localStorage.setItem(STORAGE_KEY, JSON.stringify({
                    port: this.port
                }));
            } catch (e) {
                console.warn('[VWorkConnector] 保存配置失敗:', e);
            }
        }

        /**
         * 設置端口
         * @param {number} port - 端口號
         */
        setPort(port) {
            this.port = port;
            this.baseUrl = `http://localhost:${this.port}`;
            this._saveConfig();
        }

        /**
         * 添加事件監聽器
         * @param {string} event - 事件類型: statusChange, printersUpdate, cardTerminalUpdate
         * @param {Function} callback - 回調函數
         */
        on(event, callback) {
            if (this.listeners[event]) {
                this.listeners[event].push(callback);
            }
        }

        /**
         * 移除事件監聽器
         */
        off(event, callback) {
            if (this.listeners[event]) {
                this.listeners[event] = this.listeners[event].filter(cb => cb !== callback);
            }
        }

        /**
         * 觸發事件
         */
        _emit(event, data) {
            if (this.listeners[event]) {
                this.listeners[event].forEach(cb => {
                    try {
                        cb(data);
                    } catch (e) {
                        console.error('[VWorkConnector] 事件回調錯誤:', e);
                    }
                });
            }
        }

        /**
         * 設置連接狀態
         */
        _setStatus(status, error = null) {
            this.status = status;
            this.lastError = error;
            this._emit('statusChange', { status, error });
        }

        /**
         * 發送請求到 vWork Connector
         */
        async _request(endpoint, options = {}) {
            const url = `${this.baseUrl}${endpoint}`;
            const timeout = options.timeout || 5000;
            
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), timeout);
            
            try {
                const response = await fetch(url, {
                    ...options,
                    signal: controller.signal,
                    headers: {
                        'Content-Type': 'application/json',
                        ...options.headers
                    }
                });
                
                clearTimeout(timeoutId);
                
                if (!response.ok) {
                    const errorData = await response.json().catch(() => ({}));
                    throw new Error(errorData.message || `HTTP ${response.status}`);
                }
                
                return await response.json();
            } catch (e) {
                clearTimeout(timeoutId);
                if (e.name === 'AbortError') {
                    throw new Error('連接超時');
                }
                throw e;
            }
        }

        /**
         * 檢查連接狀態
         * @returns {Promise<boolean>}
         */
        async checkConnection() {
            this._setStatus(ConnectionStatus.CONNECTING);
            
            try {
                const result = await this._request('/api/status');
                this._setStatus(ConnectionStatus.CONNECTED);
                return true;
            } catch (e) {
                this._setStatus(ConnectionStatus.DISCONNECTED, e.message);
                return false;
            }
        }

        /**
         * 獲取打印機列表
         * @returns {Promise<Array>}
         */
        async getPrinters() {
            try {
                const result = await this._request('/api/printers');
                this.printers = result.printers || [];
                this._emit('printersUpdate', this.printers);
                return this.printers;
            } catch (e) {
                console.error('[VWorkConnector] 獲取打印機失敗:', e);
                throw e;
            }
        }

        /**
         * 測試打印機
         * @param {string} printerName - 打印機名稱
         * @returns {Promise<boolean>}
         */
        async testPrinter(printerName) {
            try {
                const result = await this._request('/api/printers/test', {
                    method: 'POST',
                    body: JSON.stringify({ printer: printerName })
                });
                return result.success;
            } catch (e) {
                console.error('[VWorkConnector] 測試打印機失敗:', e);
                throw e;
            }
        }

        /**
         * 使用系統打印機打印
         * @param {string} printerName - 打印機名稱
         * @param {string} content - 打印內容(HTML/Text)
         * @param {object} options - 打印選項
         */
        async print(printerName, content, options = {}) {
            try {
                const result = await this._request('/api/printers/print', {
                    method: 'POST',
                    body: JSON.stringify({
                        printer: printerName,
                        content: content,
                        options: options
                    }),
                    timeout: 30000
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 打印失敗:', e);
                throw e;
            }
        }

        // ==================== 卡機相關 API ====================

        /**
         * 獲取卡機列表/狀態
         * @returns {Promise<Array>}
         */
        async getCardTerminals() {
            try {
                const result = await this._request('/api/card-terminals');
                this.cardTerminals = result.terminals || [];
                this._emit('cardTerminalUpdate', this.cardTerminals);
                return this.cardTerminals;
            } catch (e) {
                console.error('[VWorkConnector] 獲取卡機列表失敗:', e);
                throw e;
            }
        }

        /**
         * 配置卡機連接
         * @param {object} config - 卡機配置
         * @param {string} config.type - 卡機類型 (kpay, etc.)
         * @param {string} config.ip - 卡機IP地址
         * @param {number} config.port - 卡機端口
         * @param {string} [config.merchantId] - 商戶ID
         */
        async configureCardTerminal(config) {
            try {
                const result = await this._request('/api/card-terminals/configure', {
                    method: 'POST',
                    body: JSON.stringify(config)
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 配置卡機失敗:', e);
                throw e;
            }
        }

        /**
         * 測試卡機連接
         * @param {string} terminalId - 卡機ID
         */
        async testCardTerminal(terminalId) {
            try {
                const result = await this._request('/api/card-terminals/test', {
                    method: 'POST',
                    body: JSON.stringify({ terminalId }),
                    timeout: 10000
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 測試卡機連接失敗:', e);
                throw e;
            }
        }

        /**
         * 發起卡機支付請求
         * @param {string} terminalId - 卡機ID
         * @param {object} payment - 支付信息
         * @param {number} payment.amount - 金額（分）
         * @param {string} payment.currency - 貨幣代碼
         * @param {string} payment.orderId - 訂單ID
         */
        async initiatePayment(terminalId, payment) {
            try {
                const result = await this._request('/api/card-terminals/payment', {
                    method: 'POST',
                    body: JSON.stringify({
                        terminalId,
                        ...payment
                    }),
                    timeout: 120000  // 2分鐘超時（等待用戶刷卡）
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 發起支付失敗:', e);
                throw e;
            }
        }

        /**
         * 取消支付
         * @param {string} terminalId - 卡機ID
         * @param {string} transactionId - 交易ID
         */
        async cancelPayment(terminalId, transactionId) {
            try {
                const result = await this._request('/api/card-terminals/cancel', {
                    method: 'POST',
                    body: JSON.stringify({ terminalId, transactionId }),
                    timeout: 30000
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 取消支付失敗:', e);
                throw e;
            }
        }

        /**
         * 查詢支付結果
         * @param {string} terminalId - 卡機ID
         * @param {string} transactionId - 交易ID
         */
        async queryPaymentStatus(terminalId, transactionId) {
            try {
                const result = await this._request('/api/card-terminals/query', {
                    method: 'POST',
                    body: JSON.stringify({ terminalId, transactionId })
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 查詢支付狀態失敗:', e);
                throw e;
            }
        }

        // ==================== 熱敏打印機 API ====================

        /**
         * 獲取熱敏打印機列表
         * @returns {Promise<Array>}
         */
        async getThermalPrinters() {
            try {
                const result = await this._request('/api/thermal-printers');
                return result.printers || [];
            } catch (e) {
                console.error('[VWorkConnector] 獲取熱敏打印機失敗:', e);
                throw e;
            }
        }

        /**
         * 使用熱敏打印機打印
         * @param {string} printerName - 打印機名稱/端口
         * @param {object} ticket - 票據數據
         */
        async printThermalTicket(printerName, ticket) {
            try {
                const result = await this._request('/api/thermal-printers/print', {
                    method: 'POST',
                    body: JSON.stringify({
                        printer: printerName,
                        ticket: ticket
                    }),
                    timeout: 15000
                });
                return result;
            } catch (e) {
                console.error('[VWorkConnector] 熱敏打印失敗:', e);
                throw e;
            }
        }
    }

    // 創建全局實例
    window.VWorkConnector = new VWorkConnector();
    window.VWorkConnectorClass = VWorkConnector;
    window.VWorkConnectorStatus = ConnectionStatus;

})();
