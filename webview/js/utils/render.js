import { $, escapeHtml } from './dom.js';

/**
 * UI渲染器类
 * 负责模板渲染和UI更新
 */
export class UIrender {

    /**
     * 构造函数
     * @param {Object} templates - 模板对象
     */
    constructor() {
        this.templates = {};
        this.extractTemplate('modemInfo', 'modemInfo');
        this.extractTemplate('signalInfo', 'signalInfo');
        this.extractTemplate('smsList', 'smsItem');
        this.extractTemplate('smsdbList', 'smsdbItem');
        this.extractTemplate('webhookList', 'webhookItem');
    }

    /**
     * 从DOM中提取模板
     * @param {string} elementId - 元素ID
     * @param {string} templateKey - 模板键名
     */
    extractTemplate(elementId, templateKey) {
        if (!this.templates[templateKey]) {
            const element = $(`#${elementId}`);
            if (element) {
                this.templates[templateKey] = element.innerHTML || '';
                element.innerHTML = '';
            }
        }
    }

    /**
     * 渲染模板
     * @param {string} templateKey - 模板键名
     * @param {Object} data - 模板数据
     * @returns {string} 渲染后的HTML
     */
    render(templateKey, data) {
        const template = this.templates[templateKey] || '';
        return template.replace(/\{([\w.]+)\}/g, (_, path) => {
            const value = path.split('.').reduce((obj, k) => (obj && obj[k] !== undefined ? obj[k] : undefined), data);
            const safe = value === undefined || value === null || value === '' ? '-' : value;
            return typeof safe === 'object' ? JSON.stringify(safe) : escapeHtml(String(safe));
        });
    }
}
