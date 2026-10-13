import { createContext, ReactNode, useContext, useEffect, useMemo, useRef, useState } from "react";
import { api, formatPlmnOperator, getPlmnInfo, JsonMap, qs } from "../lib/api";

export type AppMessage = {
  id: number;
  tone: "success" | "error" | "info";
  title: string;
  detail?: string;
};

type AppContextValue = {
  modems: JsonMap[];
  selected: string;
  setSelected: (name: string) => void;
  selectedModem?: JsonMap;
  info: JsonMap;
  signal: JsonMap;
  settings: JsonMap;
  operatorLabel: string;
  logs: string[];
  messages: AppMessage[];
  busy: Record<string, boolean>;
  log: (message: string) => void;
  clearLogs: () => void;
  notify: (message: Omit<AppMessage, "id">) => void;
  dismissMessage: (id: number) => void;
  run: <T>(key: string, task: () => Promise<T>, success?: string) => Promise<T>;
  /** 读取设备列表；rescan=true 时让后端现场重新扫描串口 */
  refreshModems: (rescan?: boolean) => Promise<void>;
  /** 读取设备信息与信号；force=true 时跳过后端缓存现场查询 */
  loadInfo: (name?: string, force?: boolean) => Promise<void>;
  loadSettings: () => Promise<void>;
};

const AppContext = createContext<AppContextValue | null>(null);

export function AppProvider({ children }: { children: ReactNode }) {
  const [modems, setModems] = useState<JsonMap[]>([]);
  const [selected, setSelected] = useState("");
  const [info, setInfo] = useState<JsonMap>({});
  const [signal, setSignal] = useState<JsonMap>({});
  const [settings, setSettings] = useState<JsonMap>({});
  const [operatorLabel, setOperatorLabel] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [messages, setMessages] = useState<AppMessage[]>([]);
  const [busy, setBusy] = useState<Record<string, boolean>>({});

  const selectedModem = useMemo(() => modems.find((m) => m.name === selected), [modems, selected]);
  // 最新选中设备，供异步回调判断响应是否已过期（避免快速切换设备时旧响应覆盖新设备）
  const selectedRef = useRef(selected);
  selectedRef.current = selected;
  // 按设备缓存最近一次的信息/信号，切换设备时先展示旧数据，再后台刷新
  const infoCacheRef = useRef<Record<string, { info: JsonMap; signal: JsonMap }>>({});

  function log(message: string) {
    const stamp = new Date().toLocaleTimeString();
    setLogs((items) => [`${stamp} ${message}`, ...items].slice(0, 200));
  }

  function clearLogs() {
    setLogs([]);
  }

  function dismissMessage(id: number) {
    setMessages((items) => items.filter((item) => item.id !== id));
  }

  function notify(message: Omit<AppMessage, "id">) {
    const id = Date.now() + Math.floor(Math.random() * 1000);
    setMessages((items) => [...items, { id, ...message }].slice(-4));
    window.setTimeout(() => dismissMessage(id), 5000);
  }

  async function run<T>(key: string, task: () => Promise<T>, success?: string) {
    setBusy((current) => ({ ...current, [key]: true }));
    try {
      const result = await task();
      if (success) log(success);
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      log(message);
      notify({ tone: "error", title: "请求失败", detail: message });
      throw error;
    } finally {
      setBusy((current) => ({ ...current, [key]: false }));
    }
  }

  function fetchModems(rescan = false) {
    return api<JsonMap[]>(`/modem/list${rescan ? "?rescan=1" : ""}`);
  }

  function applyModems(list: JsonMap[]) {
    const normalized = Array.isArray(list) ? list : [];
    setModems(normalized);
    // 用函数式更新读取最新选中值：SSE 回调中闭包里的 selected 可能已过期
    setSelected((current) => (current && normalized.some((m) => m.name === current) ? current : normalized.find((m) => m.connected)?.name || normalized[0]?.name || ""));
  }

  async function refreshModems(rescan = false) {
    const list = await run("modems", () => fetchModems(rescan), rescan ? "设备扫描完成" : undefined);
    applyModems(list);
  }

  async function loadInfo(name = selected, force = false) {
    if (!name) return;
    const query = qs({ name, refresh: force ? 1 : undefined });
    const [nextInfo, nextSignal] = await Promise.all([
      run("info", () => api<JsonMap>(`/modem/info?${query}`)),
      run("signal", () => api<JsonMap>(`/modem/signal?${query}`))
    ]);
    const entry = { info: nextInfo || {}, signal: nextSignal || {} };
    infoCacheRef.current[name] = entry;
    if (selectedRef.current !== name) return;
    setInfo(entry.info);
    setSignal(entry.signal);
  }

  async function loadSettings() {
    const data = await run("settings", () => api<JsonMap>("/settings"));
    setSettings(data || {});
  }

  useEffect(() => {
    refreshModems().catch(() => undefined);
    loadSettings().catch(() => undefined);
    const stream = new EventSource("/events/modem");
    let lastSseErrorAt = 0;
    stream.onopen = () => log("SSE 已连接");
    stream.onmessage = (event) => log(`SSE ${event.data}`);
    // 设备上线/掉线/eUICC 判定变化时后端推送，静默读取列表快照（不提示、不占用按钮状态）
    stream.addEventListener("modems", () => {
      fetchModems().then(applyModems).catch(() => undefined);
    });
    stream.onerror = () => {
      log("SSE 连接中断");
      const now = Date.now();
      if (now - lastSseErrorAt > 10000) {
        lastSseErrorAt = now;
        notify({ tone: "error", title: "实时日志连接中断", detail: "正在等待浏览器自动重连" });
      }
    };
    return () => stream.close();
  }, []);

  useEffect(() => {
    const cached = infoCacheRef.current[selected];
    setInfo(cached?.info || {});
    setSignal(cached?.signal || {});
    if (selected) loadInfo(selected).catch(() => undefined);
  }, [selected]);

  useEffect(() => {
    const operator = info.operator || selectedModem?.operator || "";
    const code = String(operator || "").trim();
    if (!code) {
      setOperatorLabel("");
      return;
    }
    setOperatorLabel(formatPlmnOperator(code));
    if (!/^\d+$/.test(code)) return;

    let cancelled = false;
    getPlmnInfo(code)
      .then((plmn) => {
        if (!cancelled) setOperatorLabel(formatPlmnOperator(code, plmn));
      })
      .catch(() => {
        if (!cancelled) setOperatorLabel(formatPlmnOperator(code));
      });
    return () => {
      cancelled = true;
    };
  }, [info.operator, selectedModem?.operator]);

  return (
    <AppContext.Provider value={{ modems, selected, setSelected, selectedModem, info, signal, settings, operatorLabel, logs, messages, busy, log, clearLogs, notify, dismissMessage, run, refreshModems, loadInfo, loadSettings }}>
      {children}
    </AppContext.Provider>
  );
}

export function useApp() {
  const context = useContext(AppContext);
  if (!context) throw new Error("useApp must be used inside AppProvider");
  return context;
}
