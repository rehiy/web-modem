export function text(value: any, fallback = "-") {
  if (value === undefined || value === null || value === "") return fallback;
  return String(value);
}

/** 将接口时间（ISO 字符串）格式化为本地时间，无效值原样返回 */
export function formatTime(value: any) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return text(value);
  return date.toLocaleString();
}

export const severityLabels: Record<string, string> = {
  critical: "严重",
  warning: "警告",
  info: "提示"
};

export function rawLines(value: any) {
  if (Array.isArray(value)) return value.map(String);
  if (typeof value === "string") return value.split("\n").filter(Boolean);
  if (value?.raw && Array.isArray(value.raw)) return value.raw.map(String);
  return [];
}

export const eventLabels: Record<string, string> = {
  all: "全部事件",
  sms_received: "收到短信",
  sms_sent_success: "发送成功",
  sms_sent_failed: "发送失败",
  alert_triggered: "告警触发",
  alert_resolved: "告警恢复",
  sms_sync_completed: "同步完成",
  system_update_completed: "更新完成",
  modem_online: "设备上线",
  ip_changed: "IP 变化"
};

export const matchLabels: Record<string, string> = {
  all: "全部",
  receive_number: "接收号码",
  send_number: "发送号码",
  modem_name: "串口",
  device_imei: "IMEI",
  sim_iccid: "ICCID",
  sim_imsi: "IMSI",
  operator: "运营商",
  direction: "方向",
  content_contains: "内容包含",
  alert_type: "告警类型",
  alert_severity: "告警级别",
  alert_source: "告警来源"
};
