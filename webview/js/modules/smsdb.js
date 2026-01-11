/* =========================================
   短信存储管理模块 (Database SMS Management Module)
   ========================================= */

import { apiRequest, buildQueryString } from '../utils/api.js';
import { $, $$ } from '../utils/dom.js';

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
        this.setupEventListeners();
        this.extractTemplates();
    }

    /**
     * 设置事件监听器
     * 绑定短信存储相关的UI事件
     */
    setupEventListeners() {
        // 短信存储相关事件
        $('#refreshSmsdbBtn')?.addEventListener('click', () => this.listSmsdb());
        $('#deleteSelectedSmsdbBtn')?.addEventListener('click', () => this.deleteSelectedSmsdb());
        $('#exportSmsdbBtn')?.addEventListener('click', () => this.exportSmsdb());
        $('#searchSmsdbBtn')?.addEventListener('click', () => this.listSmsdb());
        $('#smsdbPrevPageBtn')?.addEventListener('click', () => this.smsdbPrevPage());
        $('#smsdbNextPageBtn')?.addEventListener('click', () => this.smsdbNextPage());
        $('#smsdbEnabled')?.addEventListener('change', () => this.updateSmsdbSettings());
        $('#smsdbCheckAll')?.addEventListener('change', () => this.toggleCheckAll());
    }

    /**
     * 提取模板
     * 从DOM中提取短信存储相关的模板
     */
    extractTemplates() {
        app.render.extractTemplate('smsdbItem', 'smsdbItem');
    }

    /* =========================================
       短信存储管理 (Database SMS Management)
       ========================================= */

    /**
     * 加载短信存储设置
     * 获取短信存储功能的启用状态
     */
    async loadSmsdbSettings() {
        try {
            const settings = await apiRequest('/smsdb/settings');
            const enabledCheckbox = $('#smsdbEnabled');
            if (enabledCheckbox) {
                enabledCheckbox.checked = settings.smsdb_enabled === 'true' || settings.smsdb_enabled === true;
            }
        } catch (error) {
            console.error('加载短信存储设置失败:', error);
        }
    }

    /**
     * 更新短信存储设置
     * 设置短信存储功能的启用状态
     */
    async updateSmsdbSettings() {
        try {
            const enabledCheckbox = $('#smsdbEnabled');
            if (!enabledCheckbox) return;

            const enabled = enabledCheckbox.checked;
            await apiRequest('/smsdb/settings', 'PUT', { smsdb_enabled: enabled });
            app.logger.success(`数据库存储短信已${enabled ? '启用' : '禁用'}`);
        } catch (error) {
            app.logger.error('更新设置失败');
        }
    }

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

            const queryString = buildQueryString(filter);
            const result = await apiRequest(`/smsdb/list?${queryString}`);

            this.total = result.total;
            this.displaySmsdbList(result.data);
            this.updateSmsdbPagination();
        } catch (error) {
            console.error('加载短信存储失败:', error);
        }
    }

    /**
     * 显示短信存储列表
     * 将短信数据渲染到表格中
     * @param {Array} smsList - 短信列表数据
     */
    displaySmsdbList(smsList) {
        const tbody = $('#smsdbTableBody');
        if (!tbody) return;

        tbody.innerHTML = '';

        if (!smsList || smsList.length === 0) {
            tbody.innerHTML = '<tr><td colspan="9" style="text-align: center; padding: 20px;">暂无短信</td></tr>';
            return;
        }

        const fragment = document.createDocumentFragment();
        smsList.forEach(sms => {
            const rowHtml = app.render.render('smsdbItem', {
                id: sms.id,
                direction: sms.direction === 'in' ? '📥 接收' : '📤 发送',
                send_number: sms.send_number || '-',
                receive_number: sms.receive_number || '-',
                content: sms.content,
                receive_time: new Date(sms.receive_time).toLocaleString(),
                sms_ids: sms.sms_ids
            });
            const tempDiv = document.createElement('tbody');
            tempDiv.innerHTML = rowHtml;
            while (tempDiv.firstChild) {
                fragment.appendChild(tempDiv.firstChild);
            }
        });
        tbody.appendChild(fragment);
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

        const checkboxes = $$('#smsdbTableBody input[type="checkbox"]');
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

    async deleteSelectedSmsdb() {
        if (this.selectedSmsdb.size === 0) {
            alert('请先选择要删除的短信');
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

    exportSmsdb() {
        alert('导出功能开发中...');
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
}