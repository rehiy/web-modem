import { AlertCircle, Bell, BookUser, CheckCircle2, ChevronDown, ChevronUp, CreditCard, Database, FileKey, Info, Menu, MessageSquare, Network, RefreshCw, Settings, Signal, Terminal, Trash2, Webhook, X } from "lucide-react";
import { ReactNode, useEffect, useState } from "react";
import { Button, PageHeader } from "./ui";
import { useApp } from "../context/AppContext";
import { text } from "../lib/format";
import { getSignalSummary } from "../lib/modemPresentation";

export type TabKey = "overview" | "sms" | "sim" | "euicc" | "network" | "phonebook" | "smsdb" | "webhook" | "alerts" | "settings" | "terminal";

const tabGroups: Array<{ label: string; scope: "device" | "global"; tabs: Array<{ key: TabKey; label: string; icon: ReactNode; badge?: string }> }> = [
  {
    label: "单卡功能",
    scope: "device",
    tabs: [
      { key: "overview", label: "概览", icon: <Signal size={20} /> },
      { key: "sms", label: "短信", icon: <MessageSquare size={20} /> },
      { key: "sim", label: "SIM 应用", icon: <FileKey size={20} /> },
      { key: "euicc", label: "eSIM", icon: <CreditCard size={20} /> },
      { key: "network", label: "APN / PDP", icon: <Network size={20} /> },
      { key: "phonebook", label: "电话簿", icon: <BookUser size={20} /> },
      { key: "terminal", label: "终端", icon: <Terminal size={20} /> }
    ]
  },
  {
    label: "系统功能",
    scope: "global",
    tabs: [
      { key: "webhook", label: "Webhook", icon: <Webhook size={20} /> },
      { key: "smsdb", label: "短信库", icon: <Database size={20} /> },
      { key: "alerts", label: "告警", icon: <Bell size={20} /> },
      { key: "settings", label: "设置", icon: <Settings size={20} /> }
    ]
  }
];

const pageCopy: Record<TabKey, { eyebrow: string; title: string; description: string }> = {
  overview: { eyebrow: "设备状态", title: "设备概览", description: "集中查看 SIM、网络注册与信号质量。" },
  sms: { eyebrow: "设备功能", title: "短信", description: "发送、读取和管理设备存储中的短信。" },
  sim: { eyebrow: "设备功能", title: "SIM 应用", description: "查看卡片状态，并执行 USSD、PIN 与 SIM 工具操作。" },
  euicc: { eyebrow: "设备功能", title: "eSIM 管理", description: "下载、切换与维护 eUICC 中的 Profile 套餐。" },
  network: { eyebrow: "设备功能", title: "网络配置", description: "配置 PDP 上下文、运营商选择和连通性测试。" },
  phonebook: { eyebrow: "设备功能", title: "电话簿", description: "管理 SIM 卡、设备内存和本机号码记录。" },
  terminal: { eyebrow: "设备功能", title: "AT 终端", description: "直接发送 AT 指令并保留本次浏览器会话记录。" },
  webhook: { eyebrow: "系统功能", title: "Webhook", description: "配置事件推送，并查看最近的投递结果。" },
  smsdb: { eyebrow: "系统功能", title: "短信库", description: "检索、导出和清理已归档的短信记录。" },
  alerts: { eyebrow: "系统功能", title: "告警中心", description: "查看当前异常并处理已确认的告警。" },
  settings: { eyebrow: "系统功能", title: "设置", description: "检查版本并维护 Web Modem 服务。" },
};

export function AppShell({ tab, setTab, children }: { tab: TabKey; setTab: (tab: TabKey) => void; children: ReactNode }) {
  const { modems, selected, setSelected, selectedModem, info, signal, operatorLabel, logs, messages, busy, refreshModems, loadInfo, clearLogs, dismissMessage } = useApp();
  const deviceTabs: TabKey[] = ["overview", "sms", "sim", "euicc", "network", "phonebook", "terminal"];
  const isDeviceTab = deviceTabs.includes(tab);
  const [logOpen, setLogOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);

  // 抽屉打开时锁定背景滚动，避免移动端滚动穿透
  useEffect(() => {
    if (!drawerOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [drawerOpen]);
  const simIdentity = text(info.number || info.imsi || selectedModem?.number || selected, "-");
  const signalSummary = getSignalSummary(signal);
  // 当前模块标题，用于移动端顶栏展示
  const activeLabel = tabGroups.flatMap((group) => group.tabs).find((item) => item.key === tab)?.label ?? "Web Modem";
  const activeTab = tabGroups.flatMap((group) => group.tabs).find((item) => item.key === tab);
  const currentPage = pageCopy[tab];

  return (
    <div className={`app-shell ${isDeviceTab ? "" : "global-shell"}`}>
      <header className="mobile-topbar">
        <button className="icon-btn hamburger" onClick={() => setDrawerOpen(true)} aria-label="打开导航菜单">
          <Menu size={22} />
        </button>
        <strong className="mobile-title">{activeLabel}</strong>
      </header>

      {drawerOpen && <div className="nav-backdrop" onClick={() => setDrawerOpen(false)} aria-hidden />}

      <aside className={`sidebar ${drawerOpen ? "open" : ""}`}>
        <div className="brand">
          <Signal size={32} />
          <div>
            <strong>Web Modem</strong>
          </div>
          <button className="icon-btn nav-close" onClick={() => setDrawerOpen(false)} aria-label="关闭导航菜单">
            <X size={20} />
          </button>
        </div>

        <nav className="nav-rail" aria-label="模块">
          {tabGroups.map((group) => (
            <div className="nav-group" key={group.label}>
              <div className="nav-group-head">
                <span>{group.label}</span>
              </div>
              {group.tabs.map((item) => (
                <button key={item.key} className={tab === item.key ? "active" : ""} onClick={() => { setTab(item.key); setDrawerOpen(false); }}>
                  {item.icon}
                  <span>
                    <strong>{item.label}</strong>
                  </span>
                  {item.badge && <em>{item.badge}</em>}
                </button>
              ))}
            </div>
          ))}
        </nav>

      </aside>

      {isDeviceTab && (
        <header className="topbar">
          <div className="device-command">
            <label>
              <span>设备</span>
              <select value={selected} onChange={(event) => setSelected(event.target.value)} aria-label="当前设备">
                <option value="">{modems.length ? "选择设备" : "未发现设备"}</option>
                {modems.map((modem) => (
                  <option key={modem.name} value={modem.name}>
                    {modem.name} {modem.connected ? "已连接" : "已断开"}
                  </option>
                ))}
              </select>
            </label>
            <div className="top-stat">
              <span>手机号</span>
              <strong>{simIdentity}</strong>
            </div>
            <div className="top-stat">
              <span>连接状态</span>
              <strong className={selectedModem?.connected ? "online" : "warn"}>{selectedModem?.connected ? "已连接" : selected ? "未确认" : "未选择"}</strong>
            </div>
            <div className="top-stat">
              <span>运营商</span>
              <strong>{text(operatorLabel || info.operator || selectedModem?.operator || "-")}</strong>
            </div>
            <div className={`top-stat signal ${signalSummary.tone}`}>
              <span>信号</span>
              <strong>{signalSummary.text}</strong>
            </div>
            <div className="top-actions">
              <Button icon={<RefreshCw size={16} />} variant="ghost" busy={busy.modems} onClick={() => refreshModems(true).catch(() => undefined)}>
                扫描设备
              </Button>
              <Button icon={<RefreshCw size={16} />} busy={busy.info} disabled={!selected} onClick={() => loadInfo(selected, true).catch(() => undefined)}>
                刷新
              </Button>
            </div>
          </div>
        </header>
      )}

      <main className="workspace">
        {/* 设备类页面以「页面+设备」为 key：切换设备时重新挂载，清空上一台设备的查询结果，
            避免在新设备上误操作旧数据（如按旧索引删除短信/电话簿） */}
        <div className={`page-frame page-${tab}`} key={isDeviceTab ? `${tab}:${selected}` : tab}>
          <PageHeader {...currentPage} icon={activeTab?.icon} />
          {children}
        </div>
      </main>
      <div className="message-stack" aria-live="polite" aria-label="全局消息">
        {messages.map((message) => (
          <div className={`message ${message.tone}`} key={message.id}>
            {message.tone === "error" ? <AlertCircle size={18} /> : message.tone === "success" ? <CheckCircle2 size={18} /> : <Info size={18} />}
            <div>
              <strong>{message.title}</strong>
              {message.detail && <span>{message.detail}</span>}
            </div>
            <button onClick={() => dismissMessage(message.id)} aria-label="关闭消息">
              <X size={16} />
            </button>
          </div>
        ))}
      </div>
      <aside className={`live-log ${logOpen ? "open" : ""}`} aria-label="实时日志">
        <button className="live-log-head" onClick={() => setLogOpen((open) => !open)}>
          <span>
            <Signal size={15} />
            实时日志
          </span>
          <strong>{logs.length}</strong>
          {logOpen ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
        </button>
        {logOpen && (
          <>
            <div className="live-log-actions">
              <button onClick={clearLogs} disabled={!logs.length}>
                <Trash2 size={14} />
                清空
              </button>
            </div>
            <div className="live-log-body">
              {logs.map((line, index) => (
                <div className={line.includes("中断") || line.includes("Error") || line.includes("error") ? "bad" : line.includes("已连接") || line.includes("完成") ? "ok" : ""} key={`${line}-${index}`}>
                  {line}
                </div>
              ))}
              {!logs.length && <p>暂无实时日志</p>}
            </div>
          </>
        )}
      </aside>
    </div>
  );
}
