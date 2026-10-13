import { BellRing, CreditCard, Download, Pencil, RefreshCw, Trash2 } from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { Button, Notice, Panel, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, deleteJson, JsonMap, jsonBody, putJson, qs } from "../lib/api";

// EuiccDownload 含二维码库（@zxing/browser 较大），懒加载以减小主 bundle
const EuiccDownload = lazy(() => import("./EuiccDownload").then((m) => ({ default: m.EuiccDownload })));

interface EuiccProfile {
  iccid: string;
  number?: string;
  nickname?: string;
  serviceProviderName?: string;
  profileName?: string;
  state: "enabled" | "disabled";
  isEnabled: boolean;
}

interface EuiccInfo {
  eid: string;
  profiles: EuiccProfile[];
}

interface EuiccNotification {
  sequenceNumber: number;
  notificationEvent: string;
  address: string;
  iccid: string;
}

interface RotationConfig {
  modemName: string;
  enabled: boolean;
  intervalMin: number;
  primaryIccid: string;
  lastRunAt?: string;
}

export function EuiccModule() {
  const { selected, setSelected, busy, run, modems, loadInfo } = useApp();
  const [info, setInfo] = useState<EuiccInfo | null>(null);
  const [error, setError] = useState<string>("");
  const [notifications, setNotifications] = useState<EuiccNotification[]>([]);
  const [downloadOpen, setDownloadOpen] = useState(false);
  const [rotation, setRotation] = useState<RotationConfig | null>(null);

  // 设备统一由顶栏选择，本页不再自动改写全局选中设备；当前设备未识别为 eUICC 时，
  // 仅提示其它已识别的 eUICC 设备供一键切换
  const currentModem = modems.find((m) => m.name === selected);
  const otherEuiccModems = modems.filter((m) => m.connected && m.hasEuicc && m.name !== selected);
  // 只看本页的操作状态，避免顶栏刷新信息等无关请求把本页按钮全部禁用
  const isBusy = Object.entries(busy).some(([key, value]) => value && key.startsWith("euicc-"));
  const loading = !!busy["euicc-list"];
  const activeProfile = info?.profiles?.find((p) => p.isEnabled);

  const loadProfiles = useCallback(async () => {
    if (!selected) return;
    setError("");
    try {
      const data = await run("euicc-list", () =>
        api<EuiccInfo>(`/euicc/profiles?${qs({ name: selected })}`)
      );
      setInfo(data);
    } catch (e: any) {
      setError(e?.message || "读取失败");
      setInfo(null);
    }
  }, [selected, run]);

  // 加载待处理通知（Profile 操作后需回执给运营商的记录），独立容错
  const loadNotifications = useCallback(async () => {
    if (!selected) return;
    try {
      const r = await api<{ notifications: EuiccNotification[] }>(
        `/euicc/notifications?${qs({ name: selected })}`
      );
      setNotifications(r.notifications || []);
    } catch {
      setNotifications([]);
    }
  }, [selected]);

  // 加载该设备的 Profile 轮询配置
  const loadRotation = useCallback(async () => {
    if (!selected) return;
    try {
      const r = await api<RotationConfig>(`/euicc/rotation?${qs({ name: selected })}`);
      setRotation(r);
    } catch {
      setRotation(null);
    }
  }, [selected]);

  useEffect(() => {
    setInfo(null);
    setNotifications([]);
    setError("");
    setDownloadOpen(false);
    setRotation(null);
    if (selected) {
      loadProfiles();
      loadNotifications();
      loadRotation();
    }
  }, [selected]);

  async function handleEnable(iccid: string) {
    if (!selected) return;
    if (!confirm(`确认启用 Profile ${iccid}？\n切换后设备将短暂离线并重新注册网络。`)) return;
    try {
      const result = await run(
        `euicc-enable-${iccid}`,
        () => api<JsonMap>(`/euicc/profiles/${iccid}/enable`, jsonBody({ name: selected })),
        "Profile 已启用"
      );
      if (result.euicc) {
        setInfo(result.euicc as unknown as EuiccInfo);
      } else {
        await loadProfiles();
      }
      // 切换 Profile 后 ICCID/IMSI/运营商会变，刷新顶栏信息
      loadInfo(selected).catch(() => undefined);
    } catch (e: any) {
      setError(e?.message || "启用失败");
    }
  }

  async function handleDisable(iccid: string) {
    if (!selected) return;
    if (!confirm(`确认禁用 Profile ${iccid}？`)) return;
    try {
      const result = await run(
        `euicc-disable-${iccid}`,
        () => api<JsonMap>(`/euicc/profiles/${iccid}/disable`, jsonBody({ name: selected })),
        "Profile 已禁用"
      );
      if (result.euicc) {
        setInfo(result.euicc as unknown as EuiccInfo);
      } else {
        await loadProfiles();
      }
      loadInfo(selected).catch(() => undefined);
    } catch (e: any) {
      setError(e?.message || "禁用失败");
    }
  }

  async function handleDelete(iccid: string) {
    if (!selected) return;
    const confirmed = prompt(`删除操作不可撤销！\n输入 ICCID 最后 6 位以确认删除：`);
    if (confirmed === null) return;
    if (confirmed.trim().length < 6 || !iccid.endsWith(confirmed.trim())) {
      alert("确认码不匹配，已取消。");
      return;
    }
    try {
      await run(
        `euicc-delete-${iccid}`,
        () => api<JsonMap>(`/euicc/profiles/${iccid}`, deleteJson({ name: selected })),
        "Profile 已删除"
      );
      await loadProfiles();
    } catch (e: any) {
      setError(e?.message || "删除失败");
    }
  }

  // 手动设置/补录某个 Profile 的手机号（模块无法读取未启用 Profile 的号码）
  async function handleSetNumber(iccid: string, current: string) {
    if (!selected) return;
    const input = prompt("设置该 Profile 的手机号（MSISDN）：", current || "");
    if (input === null) return;
    const number = input.trim();
    if (!number) return;
    try {
      await run(
        `euicc-number-${iccid}`,
        () => api<JsonMap>(`/euicc/profiles/${iccid}/number`, putJson({ name: selected, number })),
        "号码已保存"
      );
      await loadProfiles();
      // 若是当前启用 Profile，同步刷新顶栏号码
      loadInfo(selected).catch(() => undefined);
    } catch (e: any) {
      setError(e?.message || "保存失败");
    }
  }

  // 保存轮询配置
  async function handleSaveRotation(cfg: RotationConfig) {
    if (!selected) return;
    try {
      const r = await run(
        "euicc-rotation-save",
        () =>
          api<RotationConfig>(
            "/euicc/rotation",
            putJson({ name: selected, enabled: cfg.enabled, intervalMin: cfg.intervalMin, primaryIccid: cfg.primaryIccid })
          ),
        "轮询配置已保存"
      );
      setRotation(r);
    } catch (e: any) {
      setError(e?.message || "保存失败");
    }
  }

  // 立即触发一轮轮询（异步）
  async function handleRunRotation() {
    if (!selected) return;
    try {
      await run(
        "euicc-rotation-run",
        () => api<JsonMap>(`/euicc/rotation/run?${qs({ name: selected })}`, jsonBody({ name: selected })),
        "已开始轮询（后台执行）"
      );
    } catch (e: any) {
      setError(e?.message || "启动失败");
    }
  }

  // 上报待处理通知回执给 SM-DP+/SM-DS
  async function handleNotify(seq: number) {
    if (!selected) return;
    try {
      await run(
        `euicc-notify-${seq}`,
        () => api<JsonMap>(`/euicc/notifications/${seq}/handle`, jsonBody({ name: selected })),
        "回执已上报"
      );
      loadNotifications();
    } catch (e: any) {
      setError(e?.message || "上报失败");
    }
  }

  return (
    <div className="module-root">
      <Panel
        title="eUICC 设备与套餐"
        icon={<CreditCard size={18} />}
        actions={
          <>
            <Button
              variant="primary"
              icon={<Download size={14} />}
              disabled={!selected || !info || isBusy}
              onClick={() => setDownloadOpen(true)}
            >
              下载新 Profile
            </Button>
            <Button
              variant="secondary"
              icon={<RefreshCw size={14} />}
              busy={loading}
              disabled={!selected || isBusy}
              onClick={() => {
                loadProfiles();
                loadNotifications();
              }}
            >
              刷新
            </Button>
          </>
        }
      >
        {!selected && (
          <div className="empty">请在顶栏选择设备</div>
        )}

        {selected && (
          <>
            {!currentModem?.hasEuicc && otherEuiccModems.length > 0 && (
              <Notice
                tone={error ? "warn" : "info"}
                actions={otherEuiccModems.map((m) => (
                  <Button key={m.name} variant="secondary" onClick={() => setSelected(m.name)}>
                    切换到 {m.name}
                  </Button>
                ))}
              >
                当前设备 {selected} 尚未识别为 eUICC，已识别的 eUICC 设备：{otherEuiccModems.map((m) => m.name).join("、")}
              </Notice>
            )}

            {error && <div className="error-hint">{error}</div>}

            {!error && !info && loading && <div className="euicc-loading">正在读取 eUICC 信息…</div>}

            {!error && info && (
              <div className="euicc-page">
                <section className="euicc-hero" aria-label="当前 eUICC 概览">
                  {/* 设备名已在顶栏展示，概览只放 eUICC 自身的信息 */}
                  <div className="euicc-hero-device">
                    <div className="euicc-hero-icon"><CreditCard size={24} /></div>
                    <div>
                      <span>当前套餐</span>
                      <strong>{activeProfile ? profileTitle(activeProfile) : "未启用任何 Profile"}</strong>
                      <p>{activeProfile ? `ICCID ${activeProfile.iccid}` : "在下方列表中选择一个 Profile 设为当前套餐"}</p>
                    </div>
                  </div>
                  <div className="euicc-hero-stats">
                    <div>
                      <span>EID</span>
                      <strong className="euicc-eid">{info.eid || "—"}</strong>
                    </div>
                    <div>
                      <span>已安装 Profile</span>
                      <strong>{info.profiles?.length ?? 0}</strong>
                    </div>
                    <div className="euicc-active-stat">
                      <span>当前号码</span>
                      <strong>{activeProfile?.number || "—"}</strong>
                    </div>
                  </div>
                </section>

                {downloadOpen && (
                  <Suspense fallback={<div className="euicc-loading">正在加载下载工具…</div>}>
                    <EuiccDownload
                      name={selected}
                      onClose={() => setDownloadOpen(false)}
                      onDownloaded={() => {
                        loadProfiles();
                        loadNotifications();
                      }}
                    />
                  </Suspense>
                )}

                <section className="euicc-section" aria-labelledby="euicc-profiles-title">
                  <div className="euicc-section-head">
                    <div>
                      <span className="euicc-eyebrow">已安装的套餐</span>
                      <h3 id="euicc-profiles-title">Profile 列表</h3>
                      <p>号码按 ICCID 保存在本机 SQLite 中，仅展示手动确认的登记记录。</p>
                    </div>
                    <StatusPill tone="neutral">{info.profiles?.length ?? 0} 个</StatusPill>
                  </div>

                  <div className="euicc-list">
                    {info.profiles?.length === 0 && (
                      <div className="euicc-empty-profiles">
                        <div><CreditCard size={22} /></div>
                        <div>
                          <strong>还没有 eSIM Profile</strong>
                          <p>通过上方的下载工作台添加运营商提供的套餐。</p>
                        </div>
                      </div>
                    )}
                    {info.profiles?.map((p) => {
                      const title = profileTitle(p);
                      const profileSubtitle = p.profileName && p.profileName !== title ? p.profileName : p.nickname && p.nickname !== title ? p.nickname : "";

                      return (
                        <article key={p.iccid} className={`euicc-profile-card ${p.isEnabled ? "enabled" : ""}`}>
                          <div className="euicc-profile-main">
                            <div className="euicc-profile-heading">
                              <div className="euicc-profile-icon"><CreditCard size={18} /></div>
                              <div>
                                <div className="euicc-profile-title-row">
                                  <strong className="euicc-provider">{title}</strong>
                                  <StatusPill tone={p.isEnabled ? "ok" : "neutral"}>
                                    {p.isEnabled ? "使用中" : "未启用"}
                                  </StatusPill>
                                </div>
                                {profileSubtitle && <span className="euicc-name">{profileSubtitle}</span>}
                              </div>
                            </div>

                            <dl className="euicc-profile-details">
                              <div>
                                <dt>ICCID</dt>
                                <dd>
                                  <code className="euicc-iccid">{p.iccid}</code>
                                  {p.nickname && profileSubtitle !== p.nickname && <span className="euicc-nick">{p.nickname}</span>}
                                </dd>
                              </div>
                              <div>
                                <dt>登记号码</dt>
                                <dd>
                                  <code className="euicc-number">{p.number || "—"}</code>
                                  <Button
                                    variant="ghost"
                                    icon={<Pencil size={12} />}
                                    busy={!!busy[`euicc-number-${p.iccid}`]}
                                    disabled={isBusy}
                                    onClick={() => handleSetNumber(p.iccid, p.number || "")}
                                  >
                                    {p.number ? "修改" : "设置"}
                                  </Button>
                                </dd>
                              </div>
                            </dl>
                          </div>
                          <div className="euicc-profile-actions">
                            {!p.isEnabled && (
                              <Button
                                variant="primary"
                                busy={!!busy[`euicc-enable-${p.iccid}`]}
                                disabled={isBusy}
                                onClick={() => handleEnable(p.iccid)}
                              >
                                设为当前套餐
                              </Button>
                            )}
                            {p.isEnabled && (
                              <Button
                                variant="secondary"
                                busy={!!busy[`euicc-disable-${p.iccid}`]}
                                disabled={isBusy}
                                onClick={() => handleDisable(p.iccid)}
                              >
                                停用
                              </Button>
                            )}
                            <Button
                              variant="danger"
                              icon={<Trash2 size={14} />}
                              busy={!!busy[`euicc-delete-${p.iccid}`]}
                              disabled={isBusy}
                              onClick={() => handleDelete(p.iccid)}
                            >
                              删除
                            </Button>
                          </div>
                        </article>
                      );
                    })}
                  </div>
                </section>

                <section className="euicc-section euicc-rotation-section" aria-labelledby="euicc-rotation-title">
                  <div className="euicc-section-head">
                    <div>
                      <span className="euicc-eyebrow">多号码收信</span>
                      <h3 id="euicc-rotation-title">Profile 轮询</h3>
                      <p>定时依次切换各卡，收取运营商短信中心暂存的短信，再切回主卡。实时性有限，但能尽量不漏收。</p>
                    </div>
                    <StatusPill tone={rotation?.enabled ? "ok" : "neutral"}>
                      {rotation?.enabled ? "已启用" : "未启用"}
                    </StatusPill>
                  </div>

                  {rotation && (
                    <RotationPanel
                      config={rotation}
                      profiles={info.profiles || []}
                      busy={busy}
                      locked={isBusy}
                      onSave={handleSaveRotation}
                      onRun={handleRunRotation}
                    />
                  )}
                </section>

                {notifications.length > 0 && (
                  <section className="euicc-notification-section" aria-labelledby="euicc-notifications-title">
                    <div className="euicc-section-head">
                      <div>
                        <span className="euicc-eyebrow">需要运营商确认</span>
                        <h3 id="euicc-notifications-title">待处理通知</h3>
                        <p>Profile 操作后的回执，不影响当前套餐使用。</p>
                      </div>
                      <StatusPill tone="warn">{notifications.length} 条</StatusPill>
                    </div>
                    <div className="euicc-list">
                      {notifications.map((n) => (
                        <article key={n.sequenceNumber} className="euicc-profile-card euicc-notification-card">
                          <div className="euicc-notification-icon"><BellRing size={18} /></div>
                          <div className="euicc-profile-main">
                            <div className="euicc-profile-id">
                              <strong>{n.notificationEvent}</strong>
                              <span className="euicc-notification-sequence">通知 #{n.sequenceNumber}</span>
                            </div>
                            <div className="euicc-profile-meta">
                              <span>ICCID</span>
                              <code className="euicc-iccid">{n.iccid}</code>
                              {n.address && <span className="euicc-nick">{n.address}</span>}
                            </div>
                          </div>
                          <div className="euicc-profile-actions">
                            <Button
                              variant="primary"
                              busy={!!busy[`euicc-notify-${n.sequenceNumber}`]}
                              disabled={isBusy}
                              onClick={() => handleNotify(n.sequenceNumber)}
                            >
                              上报回执
                            </Button>
                          </div>
                        </article>
                      ))}
                    </div>
                  </section>
                )}
              </div>
            )}

            {!error && !info && !loading && (
              <div className="empty">点击刷新读取 eUICC 信息</div>
            )}
          </>
        )}
      </Panel>
    </div>
  );
}

// profileTitle 返回 Profile 的展示名称：运营商名 > Profile 名 > 昵称
function profileTitle(p: EuiccProfile) {
  return p.serviceProviderName || p.profileName || p.nickname || "未命名 Profile";
}

// RotationPanel Profile 轮询的配置表单。表单状态本地维护，保存时回传。
function RotationPanel({
  config,
  profiles,
  busy,
  locked,
  onSave,
  onRun,
}: {
  config: RotationConfig;
  profiles: EuiccProfile[];
  busy: Record<string, boolean>;
  locked: boolean;
  onSave: (cfg: RotationConfig) => void;
  onRun: () => void;
}) {
  const [enabled, setEnabled] = useState(config.enabled);
  const [intervalMin, setIntervalMin] = useState(config.intervalMin || 30);
  const [primaryIccid, setPrimaryIccid] = useState(config.primaryIccid || "");

  // 配置变更（切换设备或保存返回）时同步本地表单
  useEffect(() => {
    setEnabled(config.enabled);
    setIntervalMin(config.intervalMin || 30);
    setPrimaryIccid(config.primaryIccid || "");
  }, [config]);

  const lastRun = config.lastRunAt ? new Date(config.lastRunAt).toLocaleString() : "—";

  return (
    <div className="euicc-rotation-form">
      <label className="euicc-rotation-toggle">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        <span>
          <strong>启用定时轮询</strong>
          <small>依次切换各 Profile 收取短信，完成后自动切回主卡。</small>
        </span>
      </label>

      <div className="euicc-rotation-fields">
        <label className="euicc-rotation-field">
          <span>轮询间隔</span>
          <div className="euicc-interval-input">
            <input
              type="number"
              min={10}
              value={intervalMin}
              disabled={!enabled}
              onChange={(e) => setIntervalMin(Math.max(10, Number(e.target.value) || 10))}
            />
            <em>分钟</em>
          </div>
          <small>最短间隔为 10 分钟</small>
        </label>

        <label className="euicc-rotation-field">
          <span>主卡（轮询后切回）</span>
          <select
            value={primaryIccid}
            disabled={!enabled}
            onChange={(e) => setPrimaryIccid(e.target.value)}
          >
            <option value="">自动（当前启用卡）</option>
            {profiles.map((p) => (
              <option key={p.iccid} value={p.iccid}>
                {[p.serviceProviderName, p.profileName, p.iccid].filter(Boolean).join(" ")}
              </option>
            ))}
          </select>
          <small>未选择时，使用当前已启用的 Profile</small>
        </label>
      </div>

      <div className="euicc-rotation-actions">
        <span className="euicc-rotation-hint">上次运行：{lastRun}</span>
        <div>
          <Button
            variant="secondary"
            busy={!!busy["euicc-rotation-run"]}
            disabled={locked}
            onClick={onRun}
          >
            立即轮询一次
          </Button>
          <Button
            variant="primary"
            busy={!!busy["euicc-rotation-save"]}
            disabled={locked}
            onClick={() => onSave({ ...config, enabled, intervalMin, primaryIccid })}
          >
            保存配置
          </Button>
        </div>
      </div>
    </div>
  );
}
