const $ = document.querySelector.bind(document);
const $$ = document.querySelectorAll.bind(document);
const $$$ = document.createElement.bind(document);

class ModemManager {
    constructor() {
        this.ws = null;
        this.isBusy = false;
        this.templates = {};
        this.name = null;
        this.init();
    }

    init() {
        this.createTemplate();
        this.setupWebSocket();
        this.setupSMSCounter();
        this.refreshModems();
    }

    // ---------- API 接口 ----------

    async apiRequest(endpoint, method = 'GET', body = null) {
        if (this.isBusy) {
            this.logger('当前有请求正在进行，请稍候', 'error');
            throw new Error('请求被阻断');
        }

        this.toggleButtons(true);
        const options = { method, headers: { 'Content-Type': 'application/json' } };
        if (body) options.body = JSON.stringify(body);
        try {
            const response = await fetch('/api/v1' + endpoint, options);
            const data = await response.json();
            if (!response.ok) {
                const msg = data.error || '请求失败';
                this.logger(msg, 'error');
                throw new Error(msg);
            }
            return data;
        } finally {
            this.toggleButtons(false);
        }
    }

    // ---------- WebSocket 连接 ----------

    setupWebSocket() {
        this.ws = new WebSocket(`ws://${location.host}/ws`);
        this.ws.onopen = () => this.logger('WebSocket 已连接');
        this.ws.onmessage = (event) => this.logger(event.data);
        this.ws.onerror = (error) => this.logger('WebSocket 错误: ' + error);
        this.ws.onclose = () => {
            this.logger('WebSocket 已断开');
            setTimeout(() => this.setupWebSocket(), 5000);
        };
    }

    // ---------- 端口与操作 ----------

    async refreshModems() {
        try {
            const modems = await this.apiRequest('/modems');
            const select = $('#modemSelect');
            const current = select.value;
            select.innerHTML = '<option value="">-- 选择串口 --</option>';
            modems.forEach(modem => {
                const option = $$$('option');
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
            this.logger('已刷新串口列表');
        } catch (error) {
            console.error('刷新串口失败:', error);
        }
    }

    async loadModemRelatedInfo() {
        this.name = $('#modemSelect').value;
        if (!this.name) {
            this.logger('请选择可用串口', 'error');
            return null;
        }

        try {
            await this.getSignalStrength();
            await this.getModemInfo();
            await this.listSMS();
        } catch (error) {
            this.logger('串口相关信息加载失败', 'error');
        }
    }

    async sendATCommand() {
        const cmd = $('#atCommand').value.trim();
        if (!cmd) {
            this.logger('请输入 AT 命令', 'error');
            return;
        }

        try {
            const result = await this.apiRequest('/modem/at', 'POST', { name: this.name, command: cmd });
            this.addToTerminal(`> ${cmd}`);
            this.addToTerminal(result.response || '');
            $('#atCommand').value = '';
        } catch (error) {
            console.error('发送命令失败:', error);
        }
    }

    async getModemInfo() {
        const info = await this.apiRequest(`/modem/info?name=${encodeURIComponent(this.name)}`);
        this.displayModemInfo(info);
    }

    async getSignalStrength() {
        const signal = await this.apiRequest(`/modem/signal?name=${encodeURIComponent(this.name)}`);
        this.displaySignalInfo(signal);
    }

    async listSMS() {
        this.logger('正在读取短信列表 ...');
        const smsList = await this.apiRequest(`/modem/sms/list?name=${encodeURIComponent(this.name)}`);
        this.displaySMSList(smsList);
        this.logger(`已读取 ${smsList.length} 条短信`);
    }

    async sendSMS() {
        const number = $('#smsNumber').value.trim();
        const message = $('#smsMessage').value.trim();
        if (!number || !message) {
            this.logger('请输入号码和短信内容', 'error');
            return;
        }

        try {
            this.logger('正在发送短信 ...');
            await this.apiRequest('/modem/sms/send', 'POST', { name: this.name, number, message });
            this.logger('短信发送成功！', 'success');
            $('#smsNumber').value = '';
            $('#smsMessage').value = '';
            this.updateSMSCounter();
        } catch (error) {
            console.error('发送短信失败:', error);
        }
    }

    async deleteSMS(indices) {
        if (!confirm('确定要删除这条短信吗？')) {
            return;
        }

        try {
            this.logger(`正在删除短信 (Index: ${indices})...`);
            await this.apiRequest('/modem/sms/delete', 'POST', { name: this.name, indices });
            this.logger('短信删除成功！', 'success');
            this.listSMS(); // 刷新列表
        } catch (error) {
            console.error('删除短信失败:', error);
        }
    }

    // ---------- 短信计数器 ----------

    setupSMSCounter() {
        const textarea = $('#smsMessage');
        if (!textarea) return;
        const existing = $('#smsCounter');
        if (!existing) {
            const counter = $$$('div');
            counter.id = 'smsCounter';
            counter.style.cssText = 'margin-top: 5px; color: #666; font-size: 12px;';
            textarea.parentNode.appendChild(counter);
        }
        textarea.addEventListener('input', () => this.updateSMSCounter());
        this.updateSMSCounter();
    }

    updateSMSCounter() {
        const textarea = $('#smsMessage');
        const counter = $('#smsCounter');
        if (!textarea || !counter) return;
        const message = textarea.value;
        const hasUnicode = /[^\x00-\x7F]/.test(message);
        const maxChars = hasUnicode ? (message.length <= 70 ? 70 : 67) : (message.length <= 160 ? 160 : 153);
        const parts = Math.ceil(message.length / maxChars) || 1;
        const encoding = hasUnicode ? 'UCS2 (中文)' : 'GSM 7-bit';
        counter.innerHTML = `<span>字符数: ${message.length} / ${maxChars}</span> | <span>短信条数: ${parts}</span> | <span>编码: ${encoding}</span>`;
        if (parts > 3) {
            counter.style.color = '#ff4444';
            counter.innerHTML += ` <strong>⚠️ 消息过长，将分为 ${parts} 条发送</strong>`;
        } else if (parts > 1) {
            counter.style.color = '#ff9800';
        } else {
            counter.style.color = '#666';
        }
    }

    // ---------- UI 辅助函数 ----------

    escapeHtml(text) {
        const div = $$$('div');
        div.textContent = text;
        return div.innerHTML;
    }

    toggleButtons(disabled) {
        this.isBusy = disabled;
        $$('button').forEach(btn => btn.disabled = disabled);
        $$('select').forEach(btn => btn.disabled = disabled);
        $('#refreshBtn').innerText = disabled ? '加载中...' : '刷新';
    }

    addToTerminal(text) {
        const terminal = $('#terminal');
        terminal.innerHTML += this.escapeHtml(text) + '\n';
        terminal.scrollTop = terminal.scrollHeight;
    }

    logger(text, type = 'info') {
        const log = $('#log');
        const timestamp = new Date().toLocaleTimeString();
        const prefix = type === 'error' ? '❌ 错误: ' : type === 'success' ? '✅ 成功: ' : '';
        log.innerHTML += `[${timestamp}] ${prefix}${this.escapeHtml(text)}\n`;
        log.scrollTop = log.scrollHeight;
    }

    clearLog() {
        $('#log').innerHTML = '';
    }

    // ---------- 渲染 ----------

    createTemplate() {
        this.templates.modemInfo = $('#modemInfo').innerHTML || '';
        $('#modemInfo').innerHTML = '';
        this.templates.signalInfo = $('#signalInfo').innerHTML || '';
        $('#signalInfo').innerHTML = '';
        this.templates.smsItem = $('#smsList').innerHTML || '';
        $('#smsList').innerHTML = '';
    }

    renderTemplate(id, data) {
        const template = this.templates[id] || '';
        return template.replace(/\{([\w.]+)\}/g, (_, path) => {
            const value = path.split('.').reduce((obj, k) => (obj && obj[k] !== undefined ? obj[k] : undefined), data);
            const safe = value === undefined || value === null || value === '' ? '-' : value;
            if (typeof safe === 'object') {
                return JSON.stringify(safe);
            }
            return this.escapeHtml(String(safe));
        });
    }

    displayModemInfo(info) {
        const container = $('#modemInfo');
        container.innerHTML = this.renderTemplate('modemInfo', { info });
    }

    displaySignalInfo(signal) {
        const container = $('#signalInfo');
        container.innerHTML = this.renderTemplate('signalInfo', { signal });
    }

    displaySMSList(smsList) {
        const container = $('#smsList');
        container.innerHTML = smsList.map(sms => this.renderTemplate('smsItem', { sms })).join('');
    }
}

// ---------- 初始化 ----------

const app = new ModemManager();
