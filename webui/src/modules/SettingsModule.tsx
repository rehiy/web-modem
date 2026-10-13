import { CheckCircle2, Download, Info, Loader2, RefreshCw, RotateCcw, Sparkles } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button, Panel } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, JsonMap, jsonBody, putJson } from "../lib/api";
import { text } from "../lib/format";

export function SettingsModule() {
  const { busy, log, run, settings, loadSettings } = useApp();
  const [updateResult, setUpdateResult] = useState<JsonMap>({});
  const [isUpdating, setIsUpdating] = useState(false);
  const didAutoCheck = useRef(false);
  const updateInProgress = useRef(false);

  async function saveSetting(key: "smsdb_enabled" | "webhook_enabled", enabled: boolean) {
    const endpoint = key === "smsdb_enabled" ? "/settings/smsdb" : "/settings/webhook";
    await run(key, () => api(endpoint, putJson({ [key]: enabled })), "设置已保存");
    await loadSettings();
  }

  async function checkUpdate() {
    if (updateInProgress.current) return;
    const data = await run(
      "checkUpdate",
      () => api<JsonMap>(`/update/check?_=${Date.now()}`, { cache: "no-store" }),
      "更新检查完成"
    );
    setUpdateResult(data);
  }

  // 进入页面自动检查一次更新（用 ref 守卫，避免重复触发）
  useEffect(() => {
    if (didAutoCheck.current) return;
    didAutoCheck.current = true;
    checkUpdate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function applyUpdate(restart: boolean) {
    if (updateInProgress.current) return;
    if (!confirm(restart ? "确认更新并重启？" : "确认下载并应用更新？")) return;

    updateInProgress.current = true;
    setIsUpdating(true);
    try {
      const data = await run("applyUpdate", () => api<JsonMap>("/update/apply", jsonBody({ restart })), "更新操作已提交");
      setUpdateResult(data);

      if (restart) {
        const latestVersion = data.result?.latest_version || updateResult.latest_version;
        if (latestVersion) {
          log("更新已应用，等待服务重启并确认版本...");
          await waitForUpdatedVersion(latestVersion);
        }
      }
    } finally {
      updateInProgress.current = false;
      setIsUpdating(false);
    }
  }

  // 更新后轮询服务，确认新进程已加载目标版本；服务未恢复时最多补发一次重启。
  async function waitForUpdatedVersion(latestVersion: unknown) {
    const deadline = Date.now() + 60000;
    const retryAt = Date.now() + 10000;
    let serviceWasDown = false;
    let restartRetried = false;

    while (Date.now() < deadline) {
      await sleep(2000);
      const result = await tryCheckUpdate();
      if (!result) {
        serviceWasDown = true;
        log("服务重启中...");
        continue;
      }

      setUpdateResult(result);
      if (sameVersion(result.current_version, latestVersion) || !result.has_update) {
        log(`系统已重启，当前版本 ${result.current_version || latestVersion}`);
        return;
      }

      if (!restartRetried && (serviceWasDown || Date.now() >= retryAt)) {
        restartRetried = true;
        log("重启未完成，重新发送重启指令...");
        await api("/update/restart", jsonBody({})).catch(() => undefined);
        continue;
      }

      log("等待服务重启...");
    }

    throw new Error("更新已应用，但未确认服务重启完成，请刷新页面后检查版本");
  }

  async function tryCheckUpdate() {
    try {
      return await api<JsonMap>(`/update/check?_=${Date.now()}`, { cache: "no-store" });
    } catch (_) {
      return null;
    }
  }

  function sameVersion(left: unknown, right: unknown) {
    return normalizeVersion(left) === normalizeVersion(right);
  }

  function normalizeVersion(version: unknown) {
    return String(version || "").trim().replace(/^v/i, "");
  }

  function sleep(ms: number) {
    return new Promise((resolve) => window.setTimeout(resolve, ms));
  }

  async function restartApp() {
    if (!confirm("确认重启当前服务？")) return;
    await run("restart", () => api("/update/restart", jsonBody({})), "服务重启请求已提交");
  }

  // 检查结果与「应用更新」返回的包裹结构（{ status, result }）统一归一化后再渲染
  const result = (updateResult?.result ?? updateResult) as JsonMap;
  const updateInfo = (result?.info ?? {}) as JsonMap;
  const currentVersion = text(result?.current_version);
  const latestVersion = text(result?.latest_version || updateInfo.version);
  const hasUpdate = Boolean(result?.has_update);
  const message = text(updateInfo.message, "");
  const releaseNotes = String(updateInfo.release || "").trim();
  const checked = Boolean(result?.current_version || result?.latest_version || updateInfo.message);

  const statusTitle = isUpdating ? "正在应用更新…" : hasUpdate ? `发现新版本 ${latestVersion}` : checked ? "当前已是最新版本" : "检查更新";
  const statusDesc = isUpdating
    ? "更新应用后服务将自动重启，请稍候片刻"
    : hasUpdate
      ? "点击「应用」下载并安装新版本"
      : checked
        ? message
        : busy.checkUpdate
          ? "正在检查更新…"
          : "点击「检查」获取最新版本信息";

  return (
    <div className="grid">
      <Panel title="短信与通知">
        <div className="form-grid">
          <label className="field">
            <span>短信数据库存储</span>
            <input type="checkbox" checked={settings.smsdb_enabled === "true"} disabled={settings.smsdb_enabled === undefined || busy.smsdb_enabled || busy.settings} onChange={(event) => saveSetting("smsdb_enabled", event.target.checked).catch(() => undefined)} />
          </label>
          <label className="field">
            <span>Webhook 通知</span>
            <input type="checkbox" checked={settings.webhook_enabled === "true"} disabled={settings.webhook_enabled === undefined || busy.webhook_enabled || busy.settings} onChange={(event) => saveSetting("webhook_enabled", event.target.checked).catch(() => undefined)} />
          </label>
        </div>
      </Panel>
      <Panel
        title="更新"
        icon={<Download size={18} />}
        actions={
          <>
            <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.checkUpdate} disabled={isUpdating} onClick={checkUpdate}>
              检查
            </Button>
            <Button icon={<Download size={16} />} variant={hasUpdate ? "primary" : "secondary"} busy={busy.applyUpdate || isUpdating} disabled={isUpdating} onClick={() => applyUpdate(true)}>
              应用
            </Button>
            <Button icon={<RotateCcw size={16} />} variant="danger" busy={busy.restart} disabled={isUpdating} onClick={restartApp}>
              重启
            </Button>
          </>
        }
      >
        <div className={`update-status ${isUpdating || hasUpdate ? "has-update" : checked ? "latest" : ""}`}>
          <div className="update-status-icon">
            {isUpdating ? <Loader2 size={20} className="spin" /> : hasUpdate ? <Sparkles size={20} /> : checked ? <CheckCircle2 size={20} /> : <Info size={20} />}
          </div>
          <div className="update-status-text">
            <strong>{statusTitle}</strong>
            {statusDesc && <span>{statusDesc}</span>}
          </div>
        </div>

        <div className="update-version-grid">
          <div className="update-version-item">
            <span>当前版本</span>
            <strong>{currentVersion}</strong>
          </div>
          <div className={`update-version-item ${hasUpdate ? "latest" : ""}`}>
            <span>最新版本</span>
            <strong>{latestVersion}</strong>
          </div>
        </div>

        {releaseNotes && (
          <div className="update-notes">
            <span>更新说明</span>
            <pre>{releaseNotes}</pre>
          </div>
        )}
      </Panel>
    </div>
  );
}
