/* =========================================
   WebSocket 服务模块 (WebSocket Service Module)
   ========================================= */

/**
 * WebSocket服务类
 * 负责管理WebSocket连接和事件分发
 */
export class WebSocketService {

    /**
     * 构造函数
     */
    constructor() {
        this.ws = null;
        this.eventListeners = new Map();
        this.reconnectTimeout = null;
        this.pingInterval = null;
        this.connect(`ws://${location.host}/ws/modem`);
    }

    /**
     * 连接WebSocket
     * @param {string} url - WebSocket连接URL
     */
    connect(url) {
        if (this.ws?.readyState === WebSocket.OPEN) {
            return;
        }

        try {
            this.ws = new WebSocket(url);
            this.setupEventListeners();
        } catch (error) {
            this.scheduleReconnect(url);
            app.logger.error('WebSocket连接失败:', error);
        }
    }

    /**
     * 设置事件监听器
     */
    setupEventListeners() {
        this.ws.onopen = () => {
            app.logger.info('WebSocket 已连接');
            this.emit('connected');
        };

        this.ws.onmessage = (event) => {
            console.log(event.data)
            //  app.logger.info('WebSocket 消息:', event.data);
            this.emit('message', event);
        };

        this.ws.onerror = (error) => {
            app.logger.error('WebSocket 错误:', error);
            this.emit('error', error);
        };

        this.ws.onclose = (event) => {
            this.scheduleReconnect(this.ws.url);
            app.logger.info('WebSocket 已断开');
            this.emit('disconnected');
        };
    }

    /**
     * 添加事件监听器
     * @param {string} event - 事件名称
     * @param {Function} callback - 回调函数
     */
    on(event, callback) {
        if (!this.eventListeners.has(event)) {
            this.eventListeners.set(event, []);
        }
        this.eventListeners.get(event).push(callback);
    }

    /**
     * 移除事件监听器
     * @param {string} event - 事件名称
     * @param {Function} callback - 回调函数
     */
    off(event, callback) {
        if (this.eventListeners.has(event)) {
            const listeners = this.eventListeners.get(event);
            const index = listeners.indexOf(callback);
            if (index > -1) {
                listeners.splice(index, 1);
            }
        }
    }

    /**
     * 触发事件
     * @param {string} event - 事件名称
     * @param {any} data - 事件数据
     */
    emit(event, data) {
        if (this.eventListeners.has(event)) {
            const listeners = this.eventListeners.get(event);
            for (const callback of listeners) {
                try {
                    callback(data);
                } catch (error) {
                    console.error(`WebSocket事件处理错误 (${event}):`, error);
                }
            }
        }
    }

    /**
     * 发送消息
     * @param {string|Object} data - 要发送的数据
     */
    send(data) {
        if (this.ws?.readyState === WebSocket.OPEN) {
            const message = typeof data === 'object' ? JSON.stringify(data) : data;
            this.ws.send(message);
        }
    }

    /**
     * 断开连接
     */
    disconnect() {
        if (this.reconnectTimeout) {
            clearTimeout(this.reconnectTimeout);
            this.reconnectTimeout = null;
        }

        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }

    /**
     * 计划重连
     * @param {string} url - WebSocket连接URL
     */
    scheduleReconnect(url) {
        if (this.reconnectTimeout) {
            clearTimeout(this.reconnectTimeout);
        }

        this.reconnectTimeout = setTimeout(() => {
            this.connect(url);
        }, 5000);
    }
}
