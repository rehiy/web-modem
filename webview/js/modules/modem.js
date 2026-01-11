/* =========================================
   Modem 管理模块 (Modem Management Module)
   ========================================= */

import { apiRequest, buildQueryString } from '../utils/api.js';
import { $, addToTerminal } from '../utils/dom.js';

/**
 * Modem管理器类
 * 负责管理所有Modem相关的操作，包括连接、通信、短信处理等
 */
export class ModemManager {

    /**
     * 构造函数
     * 初始化Modem管理器的基本状态和属性
     */
    constructor() {
        this.isBusy = false;      // 操作繁忙状态标志
        this.name = null;         // 当前选中的Modem名称
        this.init();
    }

    /**
     * 初始化方法
     * 执行所有必要的初始化操作
     */
    init() {
        this.createTemplate();
        this.setupSMSCounter();
        this.refreshModems();
        this.setupEventListeners();
    }

    /**
     * 设置事件监听器
     * 绑定所有Modem相关的UI事件
     */
    setupEventListeners() {
        // Modem 相关事件
        $('#modemSelect')?.addEventListener('change', () => this.loadModemRelatedInfo());
        $('#refreshBtn')?.addEventListener('click', () => this.refreshModems());
        $('#getModemInfoBtn')?.addEventListener('click', () => this.getModemInfo());
        $('#getSignalStrengthBtn')?.addEventListener('click', () => this.getSignalStrength());
        $('#listSMSBtn')?.addEventListener('click', () => this.listSMS());
        $('#sendSMSBtn')?.addEventListener('click', () => this.sendSMS());
        $('#sendATCommandBtn')?.addEventListener('click', () => this.sendATCommand());
    }

    /* =========================================
       端口与操作 (Ports & Operations)
       ========================================= */

    /**
     * 刷新Modem列表
     * 获取所有可用的Modem设备并更新选择框
     */
    async refreshModems() {
        try {
            const modems = await apiRequest('/modem/list');
            const select = $('#modemSelect');
            const current = select.value;
            select.innerHTML = '<option value="">-- 选择串口 --</option>';

            // 填充Modem选择框
            modems.forEach(modem => {
                const option = document.createElement('option');
                option.value = modem.name;
                option.textContent = modem.name + (modem.connected ? ' (已连接)' : '(已断开)');
                select.appendChild(option);
            });

            // 优先保持当前选择，否则选第一个已连接
            if (current && modems.find(p => p.name === current && p.connected)) {
                select.value = current;
            } else {
                const connected = modems.find(p => p.connected);
                if (connected) select.value = connected.name;
            }

            // 端口刷新后自动加载一次相关信息
            this.loadModemRelatedInfo();
            app.logger.info('已刷新串口列表');
        } catch (error) {
            app.logger.error('刷新串口失败: ' + error);
        }
    }

    /**
     * 加载Modem相关信息
     * 获取当前选中Modem的信号强度、设备信息和短信列表
     * @returns {Promise<null>}
     */
    async loadModemRelatedInfo() {
        this.name = $('#modemSelect').value;
        if (!this.name) {
            app.logger.error('请选择可用串口');
            return null;
        }

        try {
            await this.getSignalStrength();
            await this.getModemInfo();
            await this.listSMS();
        } catch (error) {
            app.logger.error('串口相关信息加载失败');
        }
    }

    /**
     * 发送AT命令
     * 向选中的Modem发送自定义AT命令
     */
    async sendATCommand() {
        const cmd = $('#atCommand').value.trim();
        if (!cmd) {
            app.logger.error('请输入 AT 命令');
            return;
        }

        try {
            const result = await apiRequest('/modem/send', 'POST', { name: this.name, command: cmd });
            addToTerminal('terminal', `> ${cmd}`);
            addToTerminal('terminal', result.response || '');
            $('#atCommand').value = '';
        } catch (error) {
            console.error('发送命令失败:', error);
        }
    }

    /**
     * 获取Modem信息
     * 获取当前Modem的设备信息
     */
    async getModemInfo() {
        const queryString = buildQueryString({ name: this.name });
        const info = await apiRequest(`/modem/info?${queryString}`);
        this.displayModemInfo(info);
    }

    /**
     * 获取信号强度
     * 获取当前Modem的信号强度信息
     */
    async getSignalStrength() {
        const queryString = buildQueryString({ name: this.name });
        const signal = await apiRequest(`/modem/signal?${queryString}`);
        this.displaySignalInfo(signal);
    }

    /**
     * 列出短信
     * 获取当前Modem中的短信列表
     */
    async listSMS() {
        app.logger.info('正在读取短信列表 ...');
        const queryString = buildQueryString({ name: this.name });
        const smsList = await apiRequest(`/modem/sms/list?${queryString}`);
        this.displaySMSList(smsList);
        app.logger.info(`已读取 ${smsList.length} 条短信`);
    }

    /**
     * 发送短信
     * 通过选中的Modem发送短信
     */
    async sendSMS() {
        const number = $('#smsNumber').value.trim();
        const message = $('#smsMessage').value.trim();
        if (!number || !message) {
            app.logger.error('请输入号码和短信内容');
            return;
        }

        try {
            app.logger.info('正在发送短信 ...');
            await apiRequest('/modem/sms/send', 'POST', { name: this.name, number, message });
            app.logger.success('短信发送成功！');
            $('#smsNumber').value = '';
            $('#smsMessage').value = '';
            this.updateSMSCounter();
        } catch (error) {
            app.logger.error('发送短信失败: ' + error);
        }
    }

    /**
     * 删除短信
     * 删除Modem中的指定短信
     * @param {Array|number} indices - 短信索引或索引数组
     */
    async deleteSMS(indices) {
        if (!this.name) {
            app.logger.error('请先选择串口');
            return;
        }

        // 确保indices是数组
        const indicesArray = Array.isArray(indices) ? indices : [indices];
        if (!confirm(`确定要删除选中的 ${indicesArray.length} 条短信吗？`)) {
            return;
        }

        try {
            app.logger.info('正在删除短信...');
            await apiRequest('/modem/sms/delete', 'POST', { name: this.name, indices: indicesArray });
            app.logger.success('短信删除成功！');
            // 删除成功后重新加载短信列表
            await this.listSMS();
        } catch (error) {
            app.logger.error('删除短信失败: ' + error);
        }
    }

    /* =========================================
       短信计数器 (SMS Counter)
       ========================================= */

    /**
     * 设置短信计数器
     * 创建并初始化短信字符计数显示
     */
    setupSMSCounter() {
        const textarea = $('#smsMessage');
        if (!textarea) return;

        const existing = $('#smsCounter');
        if (!existing) {
            const counter = document.createElement('div');
            counter.id = 'smsCounter';
            counter.style.cssText = 'margin-top: 5px; color: #666; font-size: 12px;';
            textarea.parentNode.appendChild(counter);
        }

        textarea.addEventListener('input', () => this.updateSMSCounter());
        this.updateSMSCounter();
    }

    /**
     * 更新短信计数器
     * 根据短信内容计算字符数、编码方式和短信条数
     */
    updateSMSCounter() {
        const textarea = $('#smsMessage');
        const counter = $('#smsCounter');
        if (!textarea || !counter) return;

        const message = textarea.value;
        const hasUnicode = /[^\x00-\x7F]/.test(message);
        const maxChars = hasUnicode ? (message.length <= 70 ? 70 : 67) : (message.length <= 160 ? 160 : 153);
        const parts = Math.ceil(message.length / maxChars) || 1;
        const encoding = hasUnicode ? 'UCS2 (中文)' : 'GSM 7-bit';

        // 使用模板渲染计数器内容
        const counterHtml = app.render.render('smsCounterTemplate', {
            length: message.length,
            maxChars: maxChars,
            parts: parts,
            encoding: encoding
        });

        counter.innerHTML = counterHtml;

        if (parts > 3) {
            counter.style.color = '#ff4444';
            counter.innerHTML += ` <strong>⚠️ 消息过长，将分为 ${parts} 条发送</strong>`;
        } else if (parts > 1) {
            counter.style.color = '#ff9800';
        } else {
            counter.style.color = '#666';
        }
    }

    /* =========================================
       UI 渲染 (UI Rendering)
       ========================================= */

    /**
     * 创建模板
     * 从HTML中提取模板并清空原始内容
     */
    createTemplate() {
        app.render.extractTemplate('modemInfo', 'modemInfo');
        app.render.extractTemplate('signalInfo', 'signalInfo');
        app.render.extractTemplate('smsList', 'smsItem');
        app.render.extractTemplate('smsCounterTemplate', 'smsCounterTemplate');
    }

    /**
     * 渲染模板
     * 使用数据填充模板中的占位符
     * @param {string} id - 模板ID
     * @param {object} data - 模板数据
     * @returns {string} 渲染后的HTML
     */
    renderTemplate(id, data) {
        return app.render.render(id, data);
    }

    /**
     * 显示Modem信息
     * @param {object} info - Modem信息数据
     */
    displayModemInfo(info) {
        const container = $('#modemInfo');
        container.innerHTML = this.renderTemplate('modemInfo', { info });
    }

    /**
     * 显示信号强度信息
     * @param {object} signal - 信号强度数据
     */
    displaySignalInfo(signal) {
        const container = $('#signalInfo');
        container.innerHTML = this.renderTemplate('signalInfo', { signal });
    }

    /**
     * 显示短信列表
     * @param {Array} smsList - 短信列表数据
     */
    displaySMSList(smsList) {
        const container = $('#smsList');
        if (!smsList || smsList.length === 0) {
            container.innerHTML = app.render.render('smsItem', {
                sms: {
                    phoneNumber: '📱<br>暂无短信',
                    time: '',
                    text: '',
                    indices: 0
                }
            });
        } else {
            container.innerHTML = smsList.map(sms => app.render.render('smsItem', { sms })).join('');
        }
    }

    /**
     * 清空日志
     * 清空实时日志显示区域
     */
    clearLog() {
        const logContainer = $('#log');
        if (logContainer) {
            logContainer.innerHTML = '';
            app.logger.info('日志已清空');
        }
    }
}