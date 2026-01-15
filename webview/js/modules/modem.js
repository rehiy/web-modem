import { $ } from '../utils/dom.js';
import { apiRequest, buildQueryString, getPlmnInfo } from '../utils/api.js';
import { AT_COMMANDS } from './modem.cmd.js';

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
        this.name = null;
        this.refreshModems();
        this.renderATCommandSelect();
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

            modems.forEach(modem => {
                const option = document.createElement('option');
                option.value = modem.name;
                option.textContent = modem.name + (modem.connected ? ' (已连接)' : '(已断开)');
                select.appendChild(option);
            });

            if (current && modems.find(p => p.name === current && p.connected)) {
                select.value = current;
            } else {
                const connected = modems.find(p => p.connected);
                if (connected) select.value = connected.name;
            }

            this.name = $('#modemSelect').value;
            await this.getModemInfo();
            await this.getSignalStrength();
            app.logger.info('已刷新串口列表');
        } catch (error) {
            app.logger.error('刷新串口失败: ' + error);
        }
    }

    /**
     * 获取Modem信息
     * 获取当前Modem的设备信息
     */
    async getModemInfo() {
        const queryString = buildQueryString({ name: this.name });
        const info = await apiRequest(`/modem/info?${queryString}`);
        // 渲染模板
        const container = $('#modemInfo');
        container.innerHTML = app.render.render('modemInfo', { info });
        // 使用运营商ID查询运营商信息，并替换当前的运营商名称
        getPlmnInfo(info.operator).then(plmn => {
            const replace = `${plmn.operator}(${info.operator})`
            container.innerHTML = container.innerHTML.replace(info.operator, replace);
        }).catch(error => {
            app.logger.error('获取PLMN信息失败: ' + error);
        });
    }

    /**
     * 获取信号强度
     * 获取当前Modem的信号强度信息
     */
    async getSignalStrength() {
        const queryString = buildQueryString({ name: this.name });
        const signal = await apiRequest(`/modem/signal?${queryString}`);
        // 渲染模板
        const container = $('#signalInfo');
        container.innerHTML = app.render.render('signalInfo', { signal });
    }

    /* =========================================
       AT 命令 (AT Commands)
       ========================================= */

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
            this.addToTerminal('terminal', `> ${cmd}`);
            this.addToTerminal('terminal', result.response || '');
            $('#atCommand').value = '';
        } catch (error) {
            app.logger.error('发送命令失败:', error);
        }
    }

    /**
     * 选择快捷AT命令
     * 当下拉菜单选择改变时，将选中的命令填充到输入框
     * @param {string} value - 选中的AT命令
     */
    selectATCommand(value) {
        const atCommandInput = $('#atCommand');
        if (atCommandInput) {
            atCommandInput.value = value;
            atCommandInput.focus();
        }
    }

    /**
     * 渲染AT命令快捷指令下拉菜单
     * 使用AT_COMMANDS数据动态生成选项
     */
    renderATCommandSelect() {
        const select = $('.form-select[onchange*=\"selectATCommand\"]');
        if (!select) return;

        // 清空现有选项（保留第一个\"-- 选择快捷指令 --\"选项）
        const firstOption = select.querySelector('option[value=\"\"]');
        select.innerHTML = '';
        if (firstOption) {
            select.appendChild(firstOption);
        } else {
            const newFirstOption = document.createElement('option');
            newFirstOption.value = '';
            newFirstOption.textContent = '-- 选择快捷指令 --';
            select.appendChild(newFirstOption);
        }

        // 根据AT_COMMANDS数据生成optgroup和option
        AT_COMMANDS.forEach(group => {
            const optgroup = document.createElement('optgroup');
            optgroup.label = group.label;
            group.options.forEach(opt => {
                const option = document.createElement('option');
                option.value = opt.value;
                option.textContent = opt.text;
                optgroup.appendChild(option);
            });
            select.appendChild(optgroup);
        });
    }

    /**
     * 向终端添加内容
     * 向指定的终端元素追加文本内容，并自动滚动到底部
     * @param {string} elementId - 终端元素的ID
     * @param {string} text - 要添加的文本内容
     */
    addToTerminal(elementId, text) {
        const terminal = $(`#${elementId}`);
        if (terminal) {
            const fragment = document.createDocumentFragment();
            const line = document.createElement('div');
            line.textContent = text;
            fragment.appendChild(line);
            terminal.appendChild(fragment);
            terminal.scrollTop = terminal.scrollHeight;
        }
    }

    /* =========================================
       短信收发 (SMS Recive/Send)
       ========================================= */

    /**
     * 列出短信
     * 获取当前Modem中的短信列表
     */
    async listSms() {
        app.logger.info('正在读取短信列表 ...');
        const queryString = buildQueryString({ name: this.name });
        const smsList = await apiRequest(`/modem/sms/list?${queryString}`);
        app.logger.success(`已读取 ${smsList.length} 条短信`);
        // 渲染模板
        const container = $('#smsList');
        if (!smsList || smsList.length === 0) {
            container.innerHTML = '内置存储暂无短信';
        } else {
            container.innerHTML = smsList.map(sms => app.render.render('smsItem', { sms })).join('');
        }
    }

    /**
     * 发送短信
     * 通过选中的Modem发送短信
     */
    async sendSms() {
        const number = $('#smsNumber').value.trim();
        const message = $('#smsMessage').value.trim();
        if (!number || !message) {
            app.logger.error('请输入号码和短信内容');
            return;
        }

        try {
            app.logger.info('正在发送短信 ...');
            await apiRequest('/modem/sms/send', 'POST', { name: this.name, number, message });
            app.logger.success('短信发送成功', number);
            $('#smsNumber').value = '';
            $('#smsMessage').value = '';
            this.updateSmsCounter();
        } catch (error) {
            app.logger.error('发送短信失败: ' + error);
        }
    }

    /**
     * 删除短信
     * 删除Modem中的指定短信
     * @param {Array|number} indices - 短信索引或索引数组
     */
    async deleteSms(indices) {
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
            await this.listSms();
        } catch (error) {
            app.logger.error('删除短信失败: ' + error);
        }
    }

    /* =========================================
       短信计数器 (SMS Counter)
       ========================================= */

    /**
     * 更新短信计数器
     * 根据短信内容计算字符数、编码方式和短信条数
     */
    updateSmsCounter() {
        const textarea = $('#smsMessage');
        const counter = $('#smsCounter');
        if (!textarea || !counter) return;

        const message = textarea.value;
        const hasUnicode = /[^\x00-\x7F]/.test(message);
        const maxChars = hasUnicode ? (message.length <= 70 ? 70 : 67) : (message.length <= 160 ? 160 : 153);
        const parts = Math.ceil(message.length / maxChars) || 1;
        const encoding = hasUnicode ? 'UCS2 (中文)' : 'GSM 7-bit';

        let counterHtml = `<span>字符数: ${message.length} / ${maxChars}</span> | <span>短信条数: ${parts}</span> | <span>编码: ${encoding}</span>`;

        if (parts > 3) {
            counter.style.color = '#ff4444';
            counterHtml += ` <strong>⚠️ 消息过长，将分为 ${parts} 条发送</strong>`;
        } else if (parts > 1) {
            counter.style.color = '#ff9800';
        } else {
            counter.style.color = '#666';
        }

        counter.innerHTML = counterHtml;
    }
}