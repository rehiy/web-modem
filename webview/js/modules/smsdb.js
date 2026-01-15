import { $, $$, copyText } from '../utils/dom.js';
import { apiRequest, buildQueryString } from '../utils/api.js';

/**
 * 短信存储管理器类
 * 负责管理数据库中的短信数据，包括增删改查、分页、筛选等功能
 */
export class SmsdbManager {
    /**
     * 构造函数
     * 初始化短信存储管理器的基本状态和属性
     */
    constructor() {
        this.page = 0;                    // 当前页码
        this.pageSize = 50;               // 每页显示数量
        this.total = 0;                   // 总记录数
        this.selectedSmsdb = new Set();   // 选中的短信ID集合
    }

    /* =========================================
       短信存储管理 (Database SMS Management)
       ========================================= */

    /**
     * 列出短信存储
     * 根据分页和筛选条件获取短信列表
     */
    async listSmsdb() {
        try {
            const filter = {
                limit: this.pageSize,
                offset: this.page * this.pageSize
            };

            // 添加过滤条件
            const sendNumber = $('#smsdbFilterSendNumber')?.value.trim();
            if (sendNumber) {
                filter.send_number = sendNumber;
            }

            const direction = $('#smsdbFilterDirection')?.value;
            if (direction) {
                filter.direction = direction;
            }

            const startDate = $('#smsdbFilterStartDate')?.value;
            if (startDate) {
                filter.start_time = new Date(startDate).toISOString();
            }

            const endDate = $('#smsdbFilterEndDate')?.value;
            if (endDate) {
                const end = new Date(endDate);
                end.setHours(23, 59, 59, 999);
                filter.end_time = end.toISOString();
            }

            // 使用 header 中的当前 modem 进行过滤
            const modemName = $('#modemSelect')?.value;
            if (modemName) {
                filter.modem_name = modemName;
            }

            const queryString = buildQueryString(filter);
            const result = await apiRequest(`/smsdb/list?${queryString}`);

            this.total = result.total;
            this.displaySmsList(result.data);
            this.updateSmsdbPagination();
        } catch (error) {
            app.logger.error('加载短信存储失败: ' + error);
        }
    }

    /**
     * 显示短信存储列表
     * 将短信数据渲染到表格中
     * @param {Array} smsList - 短信列表数据
     */
    displaySmsList(smsList) {
        const tbody = $('#smsdbList');
        if (!tbody) return;

        if (!smsList || smsList.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="empty-table-cell">暂无短信</td></tr>';
            return;
        }

        tbody.innerHTML = smsList.map(sms => app.render.render('smsdbItem', {
            id: sms.id,
            direction: sms.direction === 'in' ? '📥 接收' : '📤 发送',
            send_number: sms.send_number || '-',
            receive_number: sms.receive_number || '-',
            content: sms.content,
            receive_time: new Date(sms.receive_time).toLocaleString(),
            sms_ids: sms.sms_ids
        })).join('');
    }

    toggleSmsdbSelection(id) {
        if (this.selectedSmsdb.has(id)) {
            this.selectedSmsdb.delete(id);
        } else {
            this.selectedSmsdb.add(id);
        }
    }

    toggleCheckAll() {
        const checkAll = $('#smsdbCheckAll');
        if (!checkAll) return;

        const checkboxes = $$('#smsdbList input[type="checkbox"]');
        checkboxes.forEach(checkbox => {
            checkbox.checked = checkAll.checked;
            this.toggleSmsdbSelection(parseInt(checkbox.value));
        });
    }

    async deleteSmsdb(id) {
        if (!confirm('确定要删除这条短信吗？')) {
            return;
        }

        try {
            await apiRequest('/smsdb/delete', 'POST', { ids: [id] });
            app.logger.success('短信删除成功');
            this.listSmsdb();
        } catch (error) {
            app.logger.error('删除短信失败: ' + error);
        }
    }

    async copySmsdb(content) {
        try {
            // 方法1: 使用 Clipboard API (现代浏览器)
            if (navigator.clipboard && navigator.clipboard.writeText) {
                await navigator.clipboard.writeText(content);
                app.logger.success('短信内容已复制到剪贴板');
                return;
            }
            // 方法2: 使用传统的 document.execCommand (兼容性更好)
            const textArea = document.createElement('textarea');
            textArea.value = content;
            textArea.style.position = 'fixed';
            textArea.style.left = '-9999px';
            textArea.style.top = '0';
            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();
            if (document.execCommand('copy')) {
                app.logger.success('短信内容已复制到剪贴板');
            } else {
                app.logger.error('复制失败，请手动复制');
            }
            document.body.removeChild(textArea);
        } catch (error) {
            app.logger.error('复制失败: ' + error.message);
        }
    }

    async deleteSelectedSmsdb() {
        if (this.selectedSmsdb.size === 0) {
            app.logger.error('请先选择要删除的短信');
            return;
        }

        if (!confirm(`确定要删除选中的 ${this.selectedSmsdb.size} 条短信吗？`)) {
            return;
        }

        try {
            const ids = Array.from(this.selectedSmsdb);
            await apiRequest('/smsdb/delete', 'POST', { ids });
            app.logger.success(`成功删除 ${ids.length} 条短信`);
            this.selectedSmsdb.clear();
            this.listSmsdb();
        } catch (error) {
            app.logger.error('批量删除短信失败: ' + error);
        }
    }

    smsdbPrevPage() {
        if (this.page > 0) {
            this.page--;
            this.listSmsdb();
        }
    }

    smsdbNextPage() {
        const totalPages = Math.ceil(this.total / this.pageSize);
        if (this.page < totalPages - 1) {
            this.page++;
            this.listSmsdb();
        }
    }

    updateSmsdbPagination() {
        const totalPages = Math.ceil(this.total / this.pageSize);
        const pageInfo = $('#smsdbPageInfo');
        const prevBtn = $('#smsdbPrevPageBtn');
        const nextBtn = $('#smsdbNextPageBtn');

        if (pageInfo) {
            pageInfo.textContent = `第 ${this.page + 1} 页 / 共 ${totalPages} 页 (总计: ${this.total} 条)`;
        }

        if (prevBtn) {
            prevBtn.disabled = this.page === 0;
        }

        if (nextBtn) {
            nextBtn.disabled = this.page >= totalPages - 1;
        }
    }

    /* =========================================
       短信同步 (SMS Synchronization)
       ========================================= */

    /**
     * 同步当前选中的Modem短信到数据库
     */
    async syncCurrentModemSms() {
        const modemName = $('#modemSelect').value;
        if (!modemName) {
            app.logger.error('请先选择串口');
            return;
        }
        await this.syncModemSms(modemName);
    }

    /**
     * 同步指定Modem的短信到数据库
     * @param {string} modemName - Modem名称
     */
    async syncModemSms(modemName) {
        try {
            app.logger.info(`正在同步 ${modemName} 的短信...`);
            const result = await apiRequest('/smsdb/sync', 'POST', { name: modemName });

            if (result.error) {
                app.logger.error(`[${result.modemName}] ${result.error}`);
            } else if (result.newCount > 0) {
                app.logger.success(`[${result.modemName}] 同步 ${result.newCount} 条新短信 (共 ${result.totalCount} 条)`);
                await this.listSmsdb();
            } else {
                app.logger.info(`[${result.modemName}] 无新短信 (共 ${result.totalCount} 条)`);
            }
        } catch (error) {
            app.logger.error(`同步 ${modemName} 短信失败: ` + error);
        }
    }
}