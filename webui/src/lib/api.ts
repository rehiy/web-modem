export type JsonMap = Record<string, any>;

const plmnFallback: Record<string, string> = {
  "46000": "中国移动",
  "46002": "中国移动",
  "46004": "中国移动",
  "46007": "中国移动",
  "46008": "中国移动",
  "46001": "中国联通",
  "46006": "中国联通",
  "46009": "中国联通",
  "46003": "中国电信",
  "46005": "中国电信",
  "46011": "中国电信",
  "46015": "中国广电"
};

export async function api<T = any>(endpoint: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api${endpoint}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init.headers || {})
    }
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

/**
 * 调用业务操作接口。部分后端接口在 AT 指令返回 ERROR 时仍以 HTTP 200 + ok:false 响应，
 * 直接用 api() 会被当作成功；写操作统一走此函数，ok:false 时按失败抛出。
 */
export async function apiOk<T extends JsonMap = JsonMap>(endpoint: string, init: RequestInit = {}): Promise<T> {
  const data = await api<T>(endpoint, init);
  if (data && data.ok === false) {
    throw new Error(data.error || data.result?.error || "操作失败，模块返回错误");
  }
  return data;
}

export function qs(params: JsonMap) {
  const search = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") return;
    search.set(key, String(value));
  });
  return search.toString();
}

export function jsonBody(body: JsonMap): RequestInit {
  return { method: "POST", body: JSON.stringify(body) };
}

export function putJson(body: JsonMap): RequestInit {
  return { method: "PUT", body: JSON.stringify(body) };
}

export function deleteJson(body?: JsonMap): RequestInit {
  return body ? { method: "DELETE", body: JSON.stringify(body) } : { method: "DELETE" };
}

// PLMN 查询结果在页面生命周期内不变，按代码缓存请求，避免切换设备时重复访问外部接口；
// 失败的请求移出缓存以便下次重试
const plmnCache = new Map<string, Promise<JsonMap>>();

export function getPlmnInfo(operator: string | number): Promise<JsonMap> {
  const code = String(operator).trim();
  let pending = plmnCache.get(code);
  if (!pending) {
    pending = fetchPlmnInfo(code);
    plmnCache.set(code, pending);
    pending.catch(() => plmnCache.delete(code));
  }
  return pending;
}

async function fetchPlmnInfo(code: string): Promise<JsonMap> {
  const res = await fetch(`https://api.rehi.org/plmn/${encodeURIComponent(code)}`);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

export function formatPlmnOperator(operator: unknown, plmn?: JsonMap) {
  const code = String(operator || "").trim();
  if (!code) return "";
  if (!/^\d+$/.test(code)) return code;
  const name = String(plmn?.operator || plmn?.name || plmn?.brand || plmnFallback[code] || "").trim();
  return name ? `${name}(${code})` : code;
}
