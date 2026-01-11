/* =========================================
   Webhook 管理模块 (Webhook Management Module)
   ========================================= */

import { apiRequest, buildQueryString } from '../utils/api.js';
import { $ } from '../utils/dom.js';

/**
 * Webhook管理器类
 * 负责管理Webhook配置，包括创建、编辑、删除、测试等功能
 */
export class WebhookManager {

    /**
     * 构造函数
     * 初始化Webhook管理器的基本状态和属性
     */
    constructor() {
        this.currentWebhookId = null;  // 当前编辑的 Webhook ID
        this.setupEventListeners();
    }

    /**
     * 设置事件监听器
     * 绑定Webhook相关的UI事件
     */
    setupEventListeners() {
        // Webhook 相关事件
        $('#refreshWebhooksBtn')?.addEventListener('click', () => this.listWebhooks());
        $('#saveWebhookBtn')?.addEventListener('click', () => this.saveWebhook());
        $('#testWebhookBtn')?.addEventListener('click', () => this.testWebhook());
        $('#webhookEnabled')?.addEventListener('change', () => this.updateWebhookSettings());
    }

    /* =========================================
       Webhook管理 (Webhook Management)
       ========================================= */

    /**
     * 加载Webhook设置
     * 获取Webhook功能的启用状态
     */
    async loadWebhookSettings() {
        try {
            const settings = await apiRequest('/webhook/settings');
            const enabledCheckbox = $('#webhookEnabled');
            if (enabledCheckbox) {
                enabledCheckbox.checked = settings.webhook_enabled === 'true' || settings.webhook_enabled === true;
            }
        } catch (error) {
            console.error('加载Webhook设置失败:', error);
        }
    }

    /**
     * 更新Webhook设置
     * 设置Webhook功能的启用状态
     */
    async updateWebhookSettings() {
        try {
            const enabledCheckbox = $('#webhookEnabled');
            if (!enabledCheckbox) return;

            const enabled = enabledCheckbox.checked;
            await apiRequest('/webhook/settings', 'PUT', { webhook_enabled: enabled });
            app.logger.success(`Webhook功能已${enabled ? '启用' : '禁用'}`);
        } catch (error) {
            app.logger.error('更新设置失败');
        }
    }

    /**
     * 列出Webhook配置
     * 获取所有已配置的Webhook列表
     */
    async listWebhooks() {
        try {
            const webhooks = await apiRequest('/webhook/list');
            this.displayWebhookList(webhooks);
        } catch (error) {
            console.error('加载Webhook列表失败:', error);
        }
    }

    /**
     * 显示Webhook列表
     * 将Webhook数据渲染到表格中
     * @param {Array} webhooks - Webhook列表数据
     */
    displayWebhookList(webhooks) {
        const tbody = $('#webhookTableBody');
        if (!tbody) return;

        tbody.innerHTML = '';

        if (!webhooks || webhooks.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 20px;">暂无 Webhook 配置</td></tr>';
            return;
        }

        const fragment = document.createDocumentFragment();
        webhooks.forEach(webhook => {
            const rowHtml = app.render.render('webhookItem', {
                id: webhook.id,
                name: webhook.name,
                url: webhook.url,
                enabled: webhook.enabled ? '✅ 启用' : '❌ 禁用',
                created_at: new Date(webhook.created_at).toLocaleString()
            });
            const tempDiv = document.createElement('tbody');
            tempDiv.innerHTML = rowHtml;
            while (tempDiv.firstChild) {
                fragment.appendChild(tempDiv.firstChild);
            }
        });
        tbody.appendChild(fragment);
    }

    async editWebhook(id) {
        try {
            const queryString = buildQueryString({ id });
            const webhook = await apiRequest(`/webhook/get?${queryString}`);
            this.currentWebhookId = id;
            $('#webhookFormTitle').textContent = '编辑 Webhook';
            $('#webhookName').value = webhook.name;
            $('#webhookURL').value = webhook.url;
            $('#webhookTemplate').value = webhook.template;
            $('#webhookEnabledCheckbox').checked = webhook.enabled;
        } catch (error) {
            console.error('加载Webhook详情失败:', error);
        }
    }

    resetForm() {
        this.currentWebhookId = null;
        $('#webhookFormTitle').textContent = '创建 Webhook';
        $('#webhookName').value = '';
        $('#webhookURL').value = '';
        $('#webhookTemplate').value = '{}';
        $('#webhookEnabledCheckbox').checked = true;
    }

    async saveWebhook() {
        const name = $('#webhookName').value.trim();
        const url = $('#webhookURL').value.trim();
        const template = $('#webhookTemplate').value.trim();
        const enabled = $('#webhookEnabledCheckbox').checked;

        if (!name || !url) {
            alert('请填写名称和URL');
            return;
        }

        // 验证模板是否为有效的JSON
        if (template && template !== '{}') {
            try {
                JSON.parse(template);
            } catch (e) {
                alert('模板必须是有效的JSON格式');
                return;
            }
        }

        try {
            const webhookData = { name, url, template, enabled };

            if (this.currentWebhookId) {
                // 更新
                const queryString = buildQueryString({ id: this.currentWebhookId });
                await apiRequest(`/webhook/update?${queryString}`, 'PUT', webhookData);
                app.logger.success('Webhook更新成功');
            } else {
                // 创建
                await apiRequest('/webhook', 'POST', webhookData);
                app.logger.success('Webhook创建成功');
            }

            this.resetForm();
            this.listWebhooks();
        } catch (error) {
            app.logger.error('保存Webhook失败: ' + error);
        }
    }

    async deleteWebhook(id) {
        if (!confirm('确定要删除这个Webhook吗？')) {
            return;
        }

        try {
            const queryString = buildQueryString({ id });
            await apiRequest(`/webhook/delete?${queryString}`, 'DELETE');
            app.logger.success('Webhook删除成功');
            this.listWebhooks();
        } catch (error) {
            app.logger.error('删除Webhook失败: ' + error);
        }
    }

    async testWebhook() {
        const url = $('#webhookURL').value.trim();
        if (!url) {
            alert('请先填写URL');
            return;
        }

        const name = $('#webhookName').value.trim() || '测试';
        const template = $('#webhookTemplate').value.trim() || '{}';

        try {
            // 如果模板不是有效的JSON，使用默认值
            if (template !== '{}') {
                JSON.parse(template);
            }
        } catch (e) {
            alert('模板必须是有效的JSON格式');
            return;
        }

        // 创建一个临时的webhook对象用于测试
        const testWebhook = {
            name: name,
            url: url,
            template: template,
            enabled: true
        };

        try {
            await apiRequest('/webhook/test', 'POST', testWebhook);
            app.logger.success('Webhook测试请求已发送');
        } catch (error) {
            app.logger.error('Webhook测试失败');
        }
    }

    async testExistingWebhook(id) {
        try {
            const queryString = buildQueryString({ id });
            await apiRequest(`/webhook/test?${queryString}`, 'POST');
            app.logger.success('Webhook测试请求已发送');
        } catch (error) {
            app.logger.error('Webhook测试失败');
        }
    }
}