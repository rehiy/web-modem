/* =========================================
   API 工具函数 (API Utility Functions)
   ========================================= */

/**
 * API请求函数
 * 统一的API请求处理函数，处理HTTP请求和错误处理
 * @param {string} endpoint - API端点路径
 * @param {string} method - HTTP请求方法 (GET, POST, PUT, DELETE等)
 * @param {object} body - 请求体数据 (仅POST/PUT请求使用)
 * @returns {Promise<object>} 解析后的响应数据
 * @throws {Error} 当请求失败或响应状态码非200时抛出错误
 */
export async function apiRequest(endpoint, method = 'GET', body = null) {
    const options = {
        method,
        headers: { 'Content-Type': 'application/json' }
    };

    // 添加请求体（仅适用于POST/PUT等方法）
    if (body) {
        options.body = JSON.stringify(body);
    }

    try {
        // 发送请求
        const response = await fetch('/api' + endpoint, options);
        const data = await response.json();

        // 检查响应状态
        if (!response.ok) {
            throw new Error(data.error || '请求失败');
        }

        return data;
    } catch (error) {
        app.logger.error(`API请求失败: ${endpoint}`, error);
        throw error;
    }
}

/**
 * 构建查询字符串
 * 将对象参数转换为URL查询字符串，自动过滤空值
 * @param {object} params - 查询参数对象
 * @returns {string} 格式化后的查询字符串
 */
export function buildQueryString(params) {
    const queryParams = new URLSearchParams();

    // 遍历参数对象，过滤空值并添加到查询字符串
    Object.keys(params).forEach(key => {
        if (params[key] !== undefined && params[key] !== null && params[key] !== '') {
            queryParams.append(key, params[key]);
        }
    });

    return queryParams.toString();
}