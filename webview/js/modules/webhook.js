/* =========================================
   Webhook 管理模块 (Webhook Management Module)
   ========================================= */

import { $ } from '../utils/dom.js';
import { apiRequest, buildQueryString } from '../utils/api.js';

/**
 * 预设模板定义
 * 包含模板数据及显示名称
 */
const PRESET_TEMPLATES = {
    generic: {
        name: "通用格式",
        template: {
            event: "sms_received",
            data: {
                content: "{{content}}",
                send_number: "{{send_number}}",
                receive_number: "{{receive_number}}",
                receive_time: "{{receive_time}}",
                sms_ids: "{{sms_ids}}",
                direction: "{{direction}}"
            },
            timestamp: "{{receive_time}}"
        }
    },
    wechat_work: {
        name: "企业微信机器人",
        template: {
            msgtype: "text",
            text: {
                content: "收到短信\n发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}"
            }
        }
    },
    feige: {
        name: "飞鸽传书",
        template: {
            title: "新短信通知",
            content: "发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}",
            timestamp: "{{receive_time}}"
        }
    },
    dingtalk: {
        name: "钉钉机器人",
        template: {
            msgtype: "text",
            text: {
                content: "【短信通知】\n发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}"
            }
        }
    },
    feishu: {
        name: "飞书机器人",
        template: {
            msg_type: "text",
            content: {
                text: "【短信通知】\n发件人: {{send_number}}\n收件人: {{receive_number}}\n内容: {{content}}\n时间: {{receive_time}}"
            }
        }
    },
    discord: {
        name: "Discord",
        template: {
            content: "📱 **收到新短信**",
            embeds: [
                {
                    title: "短信详情",
                    color: 5814783,
                    fields: [
                        {
                            name: "发件人",
                            value: "{{send_number}}",
                            inline: true
                        },
                        {
                            name: "收件人",
                            value: "{{receive_number}}",
                            inline: true
                        },
                        {
                            name: "内容",
                            value: "{{content}}"
                        },
                        {
                            name: "时间",
                            value: "{{receive_time}}",
                            inline: true
                        }
                    ],
                    timestamp: "{{receive_time}}"
                }
            ]
        }
    },
    slack: {
        name: "Slack",
        template: {
            text: "📱 收到新短信",
            blocks: [
                {
                    type: "header",
                    text: {
                        type: "plain_text",
                        text: "短信通知"
                    }
                },
                {
                    type: "section",
                    fields: [
                        {
                            type: "mrkdwn",
                            text: "*发件人:*\n{{send_number}}"
                        },
                        {
                            type: "mrkdwn",
                            text: "*收件人:*\n{{receive_number}}"
                        }
                    ]
                },
                {
                    type: "section",
                    text: {
                        type: "mrkdwn",
                        text: "*内容:*\n{{content}}"
                    }
                },
                {
                    type: "section",
                    text: {
                        type: "mrkdwn",
                        text: "*时间:* {{receive_time}}"
                    }
                }
            ]
        }
    },
    telegram: {
        name: "Telegram Bot",
        template: {
            chat_id: "",
            text: "📱 *新短信通知*\n\n发件人: `{{send_number}}`\n收件人: `{{receive_number}}`\n内容: {{content}}\n时间: {{receive_time}}",
            parse_mode: "Markdown"
        }
    }
};

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
        // 初始化预设模板选项
        this.initPresetTemplates();
    }

    /**
     * 初始化预设模板下拉选项
     * 根据 PRESET_TEMPLATES 自动生成选项
     */
    initPresetTemplates() {
        const select = $('#webhookTemplateSelect');
        if (!select) return;

        // 清空现有选项（保留第一个"自定义"选项）
        const customOption = select.querySelector('option[value="custom"]');
        select.innerHTML = '';
        if (customOption) {
            select.appendChild(customOption);
        } else {
            const newCustomOption = document.createElement('option');
            newCustomOption.value = 'custom';
            newCustomOption.textContent = '自定义';
            select.appendChild(newCustomOption);
        }

        // 根据 PRESET_TEMPLATES 生成选项
        Object.keys(PRESET_TEMPLATES).forEach(key => {
            const preset = PRESET_TEMPLATES[key];
            if (preset.name && preset.template) {
                const option = document.createElement('option');
                option.value = key;
                option.textContent = preset.name;
                select.appendChild(option);
            }
        });
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
            app.logger.error('加载 Webhook 设置失败: ' + error);
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
            app.logger.success(`Webhook 功能已${enabled ? '启用' : '禁用'}`);
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
            const tbody = $('#webhookList');
            if (!webhooks || webhooks.length === 0) {
                tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; padding: 20px;">暂无 Webhook 配置</td></tr>';
                return;
            }
            tbody.innerHTML = webhooks.map(webhook => app.render.render('webhookItem', {
                id: webhook.id,
                name: webhook.name,
                url: webhook.url,
                enabled: webhook.enabled ? '✅' : '❌',
                created_at: new Date(webhook.created_at).toLocaleString()
            })).join('');
        } catch (error) {
            app.logger.error('加载 Webhook 列表失败: ' + error);
        }
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
            $('#webhookTemplateSelect').value = 'custom';
        } catch (error) {
            app.logger.error('加载 Webhook 详情失败: ' + error);
        }
    }

    resetForm() {
        this.currentWebhookId = null;
        $('#webhookFormTitle').textContent = '创建 Webhook';
        $('#webhookName').value = '';
        $('#webhookURL').value = '';
        $('#webhookTemplate').value = '{}';
        $('#webhookEnabledCheckbox').checked = true;
        $('#webhookTemplateSelect').value = 'custom';
    }

    /**
     * 应用预设模板
     * 当用户从下拉框选择预设模板时，自动填充模板内容
     */
    applyPresetTemplate() {
        const select = $('#webhookTemplateSelect');
        const templateKey = select.value;
        const templateTextarea = $('#webhookTemplate');

        if (!templateTextarea) return;

        // 如果选择了自定义模板，不进行任何操作
        if (templateKey === 'custom') {
            return;
        }

        // 获取预设模板
        const preset = PRESET_TEMPLATES[templateKey];
        if (preset && preset.template) {
            // 将预设模板格式化为JSON字符串，美化输出
            templateTextarea.value = JSON.stringify(preset.template, null, 2);
        }
    }

    /**
     * 保存Webhook配置
     * 创建或更新Webhook设置
     */
    async saveWebhook() {
        const name = $('#webhookName').value.trim();
        const url = $('#webhookURL').value.trim();
        const template = $('#webhookTemplate').value.trim();
        const enabled = $('#webhookEnabledCheckbox').checked;

        if (!name || !url) {
            app.logger.error('请填写名称和 URL');
            return;
        }

        // 验证模板是否为有效的JSON
        if (template && template !== '{}') {
            try {
                JSON.parse(template);
            } catch (e) {
                app.logger.error('模板必须是有效的 JSON 格式');
                return;
            }
        }

        try {
            const webhookData = { name, url, template, enabled };

            if (this.currentWebhookId) {
                const queryString = buildQueryString({ id: this.currentWebhookId });
                await apiRequest(`/webhook/update?${queryString}`, 'PUT', webhookData);
                app.logger.success('Webhook 更新成功');
            } else {
                await apiRequest('/webhook', 'POST', webhookData);
                app.logger.success('Webhook 创建成功');
            }

            this.resetForm();
            this.listWebhooks();
        } catch (error) {
            app.logger.error('保存 Webhook 失败: ' + error);
        }
    }

    async deleteWebhook(id) {
        if (!confirm('确定要删除这个 Webhook 吗？')) {
            return;
        }

        try {
            const queryString = buildQueryString({ id });
            await apiRequest(`/webhook/delete?${queryString}`, 'DELETE');
            app.logger.success('Webhook 删除成功');
            this.listWebhooks();
        } catch (error) {
            app.logger.error('删除 Webhook 失败: ' + error);
        }
    }

    async testWebhook(id = null) {
        try {
            if (id) {
                // 测试已存在的webhook
                const queryString = buildQueryString({ id });
                await apiRequest(`/webhook/test?${queryString}`, 'POST');
            } else {
                // 测试表单中的webhook
                const url = $('#webhookURL').value.trim()
                const name = $('#webhookName').value.trim() || '测试';
                const template = $('#webhookTemplate').value.trim() || '{}';

                // 验证webhook地址
                if (!url) {
                    app.logger.error('请先填写 URL');
                    return;
                }

                // 验证模板是否为有效的JSON
                if (template !== '{}') {
                    try {
                        JSON.parse(template);
                    } catch (e) {
                        app.logger.error('模板必须是有效的 JSON 格式');
                        return;
                    }
                }

                await apiRequest('/webhook/test', 'POST', {
                    name: name,
                    url: url,
                    template: template,
                    enabled: true
                });
            }

            app.logger.success('Webhook 测试请求已发送');
        } catch (error) {
            app.logger.error('Webhook 测试失败');
        }
    }
}