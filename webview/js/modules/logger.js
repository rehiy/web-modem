import { $ } from '../utils/dom.js';

/**
 * 全局日志面板类
 * 提供可收缩的悬浮窗日志显示功能
 */
export class Logger {
    /**
     * 构造函数
     */
    constructor() {
        this.expanded = true;
        this.container = $('#logContainer');
        this.panel = $('#logPanel');
        this.initResize();
    }

    /**
     * 初始化拖动调整大小功能
     */
    initResize() {
        const resizeHandle = $('.logger-resize-handle');
        if (!resizeHandle) {
            return;
        }

        let startX, startY;
        let startWidth, startHeight;
        let isResizing = false;

        resizeHandle.addEventListener('mousedown', (e) => {
            isResizing = true;
            startX = e.clientX;
            startY = e.clientY;
            startWidth = this.panel.offsetWidth;
            startHeight = this.panel.offsetHeight;
            this.panel.classList.add('resizing');
            e.preventDefault();
        });

        document.addEventListener('mousemove', (e) => {
            if (!isResizing) return;
            const deltaX = startX - e.clientX;
            const deltaY = startY - e.clientY;
            const newWidth = Math.max(300, Math.min(800, startWidth + deltaX));
            const newHeight = Math.max(200, Math.min(window.innerHeight * 0.8, startHeight + deltaY));
            this.panel.style.width = newWidth + 'px';
            this.panel.style.height = newHeight + 'px';
        });

        document.addEventListener('mouseup', () => {
            if (isResizing) {
                isResizing = false;
                this.panel.classList.remove('resizing');
            }
        });
    }

    /**
     * 切换收缩/展开状态
     */
    toggle() {
        if (this.expanded) {
            this.panel.classList.remove('expanded');
            this.panel.classList.add('collapsed');
            this.expanded = false;
        } else {
            this.panel.classList.remove('collapsed');
            this.panel.classList.add('expanded');
            this.expanded = true;
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

        const datetime = new Date().toLocaleTimeString();
        const logEntry = document.createElement('div');
        logEntry.innerHTML = `[${datetime}] ${text}`;
        logEntry.className = `log-entry ${type}`;

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