/**
 * DOM 选择器快捷方式
 * 提供便捷的DOM元素选择方法
 */

/** 选择单个元素 */
export const $ = document.querySelector.bind(document);

/** 选择多个元素 */
export const $$ = document.querySelectorAll.bind(document);

/**
 * 转义HTML特殊字符
 * 防止XSS攻击，将特殊字符转换为HTML实体
 * @param {string} text - 要转义的文本
 * @returns {string} 转义后的文本
 */
export function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}
