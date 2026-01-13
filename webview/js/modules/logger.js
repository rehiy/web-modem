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
     * 清空日志
     */
    clear() {
        if (this.container) {
            this.container.innerHTML = '';
        }
    }

    /**
     * 记录日志
     */
    log(...args) {
        if (!this.container) return;

        const type = (typeof args[args.length - 1] === 'string' &&
            ['info', 'error', 'success'].includes(args[args.length - 1]))
            ? args.pop()
            : 'info';

        const text = args.map(arg => (
            typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        )).join(' ');

        const timestamp = new Date().toLocaleTimeString();
        const prefix = type === 'error' ? '错误: ' : type === 'success' ? '成功: ' : '';

        const logEntry = document.createElement('div');
        logEntry.className = `log-entry ${type}`;
        logEntry.innerHTML = `[${timestamp}] ${prefix}${escapeHtml(text)}`;

        this.container.appendChild(logEntry);
        this.container.scrollTop = this.container.scrollHeight;
    }

    /**
     * 记录信息日志
     */
    info(...args) {
        this.log(...args, 'info');
    }

    /**
     * 记录错误日志
     */
    error(...args) {
        this.log(...args, 'error');
    }

    /**
     * 记录成功日志
     */
    success(...args) {
        this.log(...args, 'success');
    }
}