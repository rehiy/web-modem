/* =========================================
   全局日志面板组件 (Global Log Panel Component)
   ========================================= */

import { $, escapeHtml } from '../utils/dom.js';

/**
 * 全局日志面板类
 * 提供可收缩的悬浮窗日志显示功能
 */
export class Logger {

    /**
     * 构造函数
     */
    constructor() {
        this.isExpanded = true;
        this.container = $('#logContainer');
    }

    /**
     * 切换收缩/展开状态
     */
    toggle() {
        const panel = $('#logPanel');
        if (this.isExpanded) {
            panel.classList.remove('expanded');
            panel.classList.add('collapsed');
            this.isExpanded = false;
        } else {
            panel.classList.remove('collapsed');
            panel.classList.add('expanded');
            this.isExpanded = true;
        }
    }

    /**
     * 记录日志
     * @param {string} text - 日志文本
     * @param {string} type - 日志类型 (info, error, success)
     */
    log(text, type = 'info') {
        if (!this.container) return;

        const timestamp = new Date().toLocaleTimeString();
        const prefix = type === 'error' ? '❌ 错误: ' : type === 'success' ? '✅ 成功: ' : '';

        const logEntry = document.createElement('div');
        logEntry.className = `log-entry ${type}`;
        logEntry.innerHTML = `[${timestamp}] ${prefix}${escapeHtml(text)}`;

        this.container.appendChild(logEntry);
        this.container.scrollTop = this.container.scrollHeight;
    }

    /**
     * 记录信息日志
     * @param {string} text - 日志文本
     */
    info(text) {
        this.log(text, 'info');
    }

    /**
     * 记录错误日志
     * @param {string} text - 日志文本
     */
    error(text) {
        this.log(text, 'error');
    }

    /**
     * 记录成功日志
     * @param {string} text - 日志文本
     */
    success(text) {
        this.log(text, 'success');
    }

    /**
     * 清空日志
     */
    clear() {
        if (this.container) {
            this.container.innerHTML = '';
        }
    }
}