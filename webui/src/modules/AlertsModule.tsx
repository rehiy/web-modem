import { useEffect, useState } from "react";
import { Bell, CheckCircle2, RefreshCw, ScanSearch } from "lucide-react";
import { Button, Panel, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, JsonMap, jsonBody, qs } from "../lib/api";
import { formatTime, severityLabels, text } from "../lib/format";

const statusTabs: Array<{ value: string; label: string }> = [
  { value: "active", label: "活动中" },
  { value: "resolved", label: "已恢复" },
  { value: "all", label: "全部" }
];

function severityTone(severity: string): "bad" | "warn" | "neutral" {
  if (severity === "critical") return "bad";
  if (severity === "warning") return "warn";
  return "neutral";
}

export function AlertsModule() {
  const { busy, run } = useApp();
  const [alerts, setAlerts] = useState<JsonMap[]>([]);
  const [alertSummary, setAlertSummary] = useState<JsonMap>({});
  const [status, setStatus] = useState("active");

  async function loadAlerts(nextStatus = status) {
    const data = await run("alerts", () => api<JsonMap>(`/alerts/list?${qs({ status: nextStatus, limit: 100 })}`));
    setAlerts(data.data || []);
    setAlertSummary(data.summary || {});
  }

  async function scanAlerts() {
    await run("scanAlerts", () => api<JsonMap>("/alerts/scan", jsonBody({})), "告警扫描完成");
    await loadAlerts();
  }

  async function resolveAlerts(all = false, id?: number) {
    if (all && !confirm("确认将全部活动告警标记为已恢复？")) return;
    await run(all ? "resolveAlerts" : `resolveAlert${id}`, () => api("/alerts/resolve", jsonBody(all ? { all: true } : { ids: [id] })), "告警已恢复");
    await loadAlerts();
  }

  function changeStatus(next: string) {
    setStatus(next);
    loadAlerts(next).catch(() => undefined);
  }

  useEffect(() => {
    loadAlerts().catch(() => undefined);
  }, []);

  const activeCount = Number(alertSummary.active || 0);

  return (
    <Panel
      title="告警"
      icon={<Bell size={18} />}
      actions={
        <>
          <Button icon={<ScanSearch size={16} />} variant="secondary" busy={busy.scanAlerts} onClick={() => scanAlerts().catch(() => undefined)}>
            立即检查
          </Button>
          <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.alerts} onClick={() => loadAlerts().catch(() => undefined)}>
            刷新
          </Button>
          <Button icon={<CheckCircle2 size={16} />} variant="secondary" busy={busy.resolveAlerts} disabled={!activeCount} onClick={() => resolveAlerts(true).catch(() => undefined)}>
            全部恢复
          </Button>
        </>
      }
    >
      <div className="metric-row">
        {(["critical", "warning", "info"] as const).map((key) => (
          <div key={key}>
            <span>{severityLabels[key]}</span>
            <strong>{text(alertSummary[key] || 0)}</strong>
          </div>
        ))}
      </div>
      <div className="row-actions" role="tablist" aria-label="告警状态">
        {statusTabs.map((tab) => (
          <Button key={tab.value} variant={status === tab.value ? "primary" : "ghost"} role="tab" aria-selected={status === tab.value} onClick={() => changeStatus(tab.value)}>
            {tab.label}
          </Button>
        ))}
      </div>
      <div className="list">
        {alerts.map((item) => (
          <article className="item" key={item.id}>
            <div>
              <strong>{text(item.message || item.type)}</strong>
              {item.detail && <p>{item.detail}</p>}
              <span>
                {text(item.source)} · 最近 {formatTime(item.last_seen || item.created_at)}
                {Number(item.count) > 1 ? ` · 共 ${item.count} 次` : ""}
                {item.resolved_at ? ` · 恢复于 ${formatTime(item.resolved_at)}` : ""}
              </span>
            </div>
            <StatusPill tone={item.status === "resolved" ? "ok" : severityTone(item.severity)}>
              {item.status === "resolved" ? "已恢复" : severityLabels[item.severity] || text(item.severity)}
            </StatusPill>
            {item.status !== "resolved" && (
              <Button icon={<CheckCircle2 size={15} />} variant="secondary" busy={busy[`resolveAlert${item.id}`]} onClick={() => resolveAlerts(false, item.id).catch(() => undefined)}>
                恢复
              </Button>
            )}
          </article>
        ))}
        {!alerts.length && <div className="empty">{busy.alerts ? "正在读取…" : status === "active" ? "暂无活动告警" : "暂无告警记录"}</div>}
      </div>
    </Panel>
  );
}
