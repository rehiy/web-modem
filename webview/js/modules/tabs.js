import { $, $$ } from '../utils/dom.js';

/**
 * 标签管理器类
 * 负责管理应用中的标签切换和数据加载
 */
export class TabManager {
    /**
     * 构造函数
     * @param {Object} modemManager - Modem管理器实例
     * @param {Object} smsdbManager - 短信存储管理器实例
     * @param {Object} webhookManager - Webhook管理器实例
     */
    constructor() {
        this.currentTab = 'main';
    }

    /**
     * 切换标签
     * @param {string} tabName - 要切换到的标签名称
     */
    switchTab(tabName) {
        // 隐藏所有标签内容和导航标签
        $$('.tab-content').forEach(tab => tab.classList.remove('active'));
        $$('.nav-tab').forEach(nav => nav.classList.remove('active'));

        // 显示选中的标签内容和导航标签
        $(`#${tabName}Tab`)?.classList.add('active');
        $$('.nav-tab').forEach(nav => {
            if (nav.dataset.tab === tabName) {
                nav.classList.add('active');
            }
        });

        this.currentTab = tabName;
        this.loadTabData();
    }

    /**
     * 加载标签数据
     * 根据当前标签加载相应的数据和设置
     */
    loadTabData() {
        switch (this.currentTab) {
            case 'sms':
                if (app.modemManager) {
                    app.modemManager.listSMS();
                }
                break;
            case 'smsdb':
                if (app.settingManager) {
                    app.settingManager.loadSettings();
                }
                if (app.smsdbManager) {
                    app.smsdbManager.listSmsdb();
                }
                break;
            case 'webhook':
                if (app.settingManager) {
                    app.settingManager.loadSettings();
                }
                if (app.webhookManager) {
                    app.webhookManager.listWebhooks();
                }
                break;
            case 'main':
            default:
                // 主页面不加载数据
                break;
        }
    }
}