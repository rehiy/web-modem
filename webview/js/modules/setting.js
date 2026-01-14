import { $ } from '../utils/dom.js';
import { apiRequest } from '../utils/api.js';

/**
 * 设置管理模块
 * 提供系统设置的加载和更新功能
 */
export class SettingManager {
    constructor() {
        this.settings = {};
    }

    /**
     * 加载所有设置
     */
    async loadSettings() {
        try {
            this.settings = await apiRequest('/settings');
            this.updateUI();
            return this.settings;
        } catch (error) {
            app.logger.error('加载设置失败: ' + error);
            throw error;
        }
    }

    /**
     * 更新UI状态
     */
    updateUI() {
        // 更新短信存储设置
        const smsdbEnabledCheckbox = $('#smsdbEnabled');
        if (smsdbEnabledCheckbox) {
            smsdbEnabledCheckbox.checked = this.settings.smsdb_enabled === 'true' || this.settings.smsdb_enabled === true;
        }

        // 更新 Webhook 设置
        const webhookEnabledCheckbox = $('#webhookEnabled');
        if (webhookEnabledCheckbox) {
            webhookEnabledCheckbox.checked = this.settings.webhook_enabled === 'true' || this.settings.webhook_enabled === true;
        }
    }

    /**
     * 更新短信存储设置
     */
    async updateSmsdbSettings() {
        try {
            const enabledCheckbox = $('#smsdbEnabled');
            if (!enabledCheckbox) return;

            const enabled = enabledCheckbox.checked;
            await apiRequest('/settings/smsdb', 'PUT', { smsdb_enabled: enabled });

            // 更新本地缓存
            this.settings.smsdb_enabled = enabled.toString();

            app.logger.success(`数据库存储短信已${enabled ? '启用' : '禁用'}`);
        } catch (error) {
            app.logger.error('更新短信存储设置失败: ' + error);
            // 恢复 checkbox 状态
            this.updateUI();
        }
    }

    /**
     * 更新 Webhook 设置
     */
    async updateWebhookSettings() {
        try {
            const enabledCheckbox = $('#webhookEnabled');
            if (!enabledCheckbox) return;

            const enabled = enabledCheckbox.checked;
            await apiRequest('/settings/webhook', 'PUT', { webhook_enabled: enabled });

            // 更新本地缓存
            this.settings.webhook_enabled = enabled.toString();

            app.logger.success(`Webhook 功能已${enabled ? '启用' : '禁用'}`);
        } catch (error) {
            app.logger.error('更新 Webhook 设置失败: ' + error);
            // 恢复 checkbox 状态
            this.updateUI();
        }
    }
}
