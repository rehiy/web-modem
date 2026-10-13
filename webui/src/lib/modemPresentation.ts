import type { JsonMap } from "./api";

export type HealthTone = "ok" | "warn" | "bad" | "muted";

export type HealthState = {
  value: string;
  tone: HealthTone;
};

const unavailableValues = new Set(["", "-", "ERROR", "UNKNOWN", "未知"]);

export function hasDisplayValue(value: unknown) {
  if (value === undefined || value === null) return false;
  return !unavailableValues.has(String(value).trim().toUpperCase());
}

export function firstDisplayValue(...values: unknown[]) {
  return values.find(hasDisplayValue);
}

export function getSimHealth(info: JsonMap): HealthState {
  const raw = String(info.sim_status || "").trim().toUpperCase();
  const label = String(info.sim_status_label || info.sim_status || "").trim();

  if (raw === "READY") return { value: label || "就绪", tone: "ok" };
  if (["SIM PIN", "SIM PUK", "PH-SIM PIN"].includes(raw)) return { value: label || "需要解锁", tone: "warn" };
  if (raw.includes("NOT INSERTED") || raw.includes("NO SIM")) return { value: "未插卡", tone: "bad" };
  // CPIN 查询失败但能读到 ICCID/IMSI，说明 SIM 实际可用，兜底显示已识别
  if ((raw === "ERROR" || raw === "") && (hasDisplayValue(info.iccid) || hasDisplayValue(info.imsi))) {
    return { value: "已识别", tone: "ok" };
  }
  if (raw === "ERROR") return { value: "读取失败", tone: "bad" };
  if (!label || label === "-") return { value: "未知", tone: "muted" };
  return { value: label, tone: "warn" };
}

export function getRegistrationHealth(info: JsonMap): HealthState {
  const code = Number(info.registration_status_code);
  const value = String(info.registration_status || info.registration_status_label || "").trim();

  if (Number.isFinite(code)) {
    if (code === 1) return { value: value || "已注册", tone: "ok" };
    if (code === 5) return { value: value || "漫游注册", tone: "warn" };
    if (code === 2) return { value: value || "搜索中", tone: "warn" };
    if (code === 0 || code === 3) return { value: value || "未注册", tone: "bad" };
    return { value: value || "未知", tone: "muted" };
  }

  const normalized = value.toLowerCase();
  if (normalized.includes("漫游")) return { value, tone: "warn" };
  if (normalized.includes("已注册") || normalized.includes("registered") || normalized.includes("home")) return { value, tone: "ok" };
  if (normalized.includes("搜索")) return { value, tone: "warn" };
  if (normalized.includes("拒绝") || normalized.includes("未注册")) return { value, tone: "bad" };
  return { value: value || "未知", tone: "muted" };
}

export function getPdpHealth(info: JsonMap): HealthState {
  const raw = [info.pdp_active, info.context_active, info.pdp_status, info.data_session].find(
    (value) => value !== undefined && value !== null && value !== ""
  );
  if (raw === undefined) return { value: "未确认", tone: "muted" };
  if (raw === true || raw === 1) return { value: "已激活", tone: "ok" };
  if (raw === false || raw === 0) return { value: "未激活", tone: "warn" };

  const value = String(raw).trim();
  const normalized = value.toLowerCase();
  if (["active", "activated", "connected", "up"].includes(normalized)) return { value: "已激活", tone: "ok" };
  if (["inactive", "deactivated", "disconnected", "down"].includes(normalized)) return { value: "未激活", tone: "warn" };
  return { value: value || "未确认", tone: "muted" };
}

export function getSignalSummary(signal: JsonMap) {
  const rssi = Number(signal.rssi);
  const source = rssi === 99 ? undefined : firstDisplayValue(signal.dbm, signal.rsrp);
  const dbm = Number(source);
  if (!Number.isFinite(dbm)) return { value: "-", quality: "未知", text: "-", tone: "muted" as HealthTone, range: "" };

  // 参考范围随取值来源变化：dbm 来自 CSQ 换算（RSSI），否则回退到 RSRP
  const fromRssi = hasDisplayValue(signal.dbm) && Number(signal.dbm) === dbm;
  const range = fromRssi ? "RSSI 参考: -113 ~ -51 dBm" : "RSRP 参考: -140 ~ -44 dBm";
  const quality = dbm >= -70 ? "优秀" : dbm >= -85 ? "良好" : dbm >= -100 ? "较弱" : "很弱";
  const tone: HealthTone = dbm >= -85 ? "ok" : dbm >= -100 ? "warn" : "bad";
  const value = `${dbm} dBm`;
  return { value, quality, text: `${value} · ${quality}`, tone, range };
}

export function formatCsq(value: unknown) {
  const csq = Number(value);
  if (!Number.isFinite(csq) || csq === 99) return "";
  return csq >= 0 && csq <= 31 ? `${csq} / 31` : String(value);
}
