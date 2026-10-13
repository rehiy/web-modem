import { Activity, AlertTriangle, CheckCircle2, CircleHelp, Cpu, IdCard, Layers3, Pencil, Signal, Smartphone, TowerControl, XCircle } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { useApp } from "../context/AppContext";
import { text } from "../lib/format";
import { firstDisplayValue, formatCsq, getPdpHealth, getRegistrationHealth, getSignalSummary, getSimHealth, hasDisplayValue, type HealthTone } from "../lib/modemPresentation";
import { Button, Panel } from "../components/ui";
import { apiOk, jsonBody } from "../lib/api";

export function OverviewModule() {
  const { selected, selectedModem, info, signal, operatorLabel, run, loadInfo } = useApp();
  const signalLevel = Math.max(0, Math.min(100, Number(signal.level || 0)));
  const simHealth = getSimHealth(info);
  const registrationHealth = getRegistrationHealth(info);
  const pdpHealth = getPdpHealth(info);
  const signalSummary = getSignalSummary(signal);
  const csq = formatCsq(signal.rssi);
  const roaming = text(info.roaming_label || (Number(info.registration_status_code) === 5 ? "是" : info.registration_status_code !== undefined ? "否" : ""));
  const signalHighlights = [
    { label: "CSQ", value: csq },
    { label: "SINR", value: firstDisplayValue(signal.sinr) },
    { label: "接入制式", value: firstDisplayValue(info.act_label, info.act) },
    { label: "频段", value: firstDisplayValue(signal.band) }
  ].filter((item) => hasDisplayValue(item.value));
  const radioDetails = [
    { label: "BER", value: hasDisplayValue(signal.ber_label) ? signal.ber_label : Number(signal.ber) === 99 ? undefined : firstDisplayValue(signal.ber) },
    { label: "PLMN", value: firstDisplayValue(signal.plmn, signal.mcc && signal.mnc ? `${signal.mcc}/${signal.mnc}` : "") },
    { label: "PCI/PSC", value: firstDisplayValue(signal.pci_label, signal.pci, signal.psc) },
    { label: "信道", value: firstDisplayValue(signal.channel_label, signal.earfcn, signal.arfcn, signal.uarfcn) },
    { label: "小区", value: firstDisplayValue(signal.cell_id, signal.cid) },
    { label: "TAC/LAC", value: firstDisplayValue(signal.tac, signal.lac) },
    { label: "RSRP", value: firstDisplayValue(signal.rsrp) },
    { label: "RSRQ", value: firstDisplayValue(signal.rsrq) }
  ].filter((item) => hasDisplayValue(item.value));

  // 设置本机号码（写入 SIM ON 存储，供 AT+CNUM / 控制台显示）
  async function handleEditNumber() {
    if (!selected) return;
    const input = prompt("设置本机号码（写入 SIM ON 存储）", text(info.number));
    if (input === null) return;
    const number = input.trim();
    if (!number) return;
    try {
      await run("own-number", () => apiOk(`/modem/sim/own-number`, jsonBody({ name: selected, number })), "本机号码已保存");
      loadInfo(selected);
    } catch {
      /* run 已提示 */
    }
  }

  return (
    <div className="overview-stack">
      {/* 数据刷新统一走顶部命令栏的「刷新」，面板内不再放重复按钮 */}
      <Panel title="设备健康" icon={<Activity size={18} />}>
        <div className="health-row">
          <HealthItem icon={<Cpu size={22} />} label="连接状态" value={selectedModem?.connected ? "已连接" : selected ? "未确认" : "未选择"} tone={selectedModem?.connected ? "ok" : selected ? "muted" : "bad"} />
          <HealthItem icon={<Smartphone size={22} />} label="SIM 状态" value={simHealth.value} tone={simHealth.tone} />
          <HealthItem icon={<TowerControl size={22} />} label="网络注册" value={registrationHealth.value} tone={registrationHealth.tone} />
          <HealthItem icon={<Layers3 size={22} />} label="PDP 会话" value={pdpHealth.value} tone={pdpHealth.tone} />
        </div>
      </Panel>

      <Panel title="SIM 身份" icon={<IdCard size={18} />} actions={
        <Button variant="secondary" icon={<Pencil size={14} />} disabled={!selected} onClick={handleEditNumber}>设置号码</Button>
      }>
        <div className="identity-grid">
          <IdentityItem label="手机号" value={text(info.number)} emphasis />
          <IdentityItem label="IMSI" value={text(info.imsi)} />
          <IdentityItem label="运营商" value={text(operatorLabel || info.operator || selectedModem?.operator)} />
          <IdentityItem label="漫游状态" value={roaming} />
        </div>
      </Panel>

      <div className="grid two overview-grid">
        <Panel title="信号质量" icon={<Signal size={18} />}>
          <div className="signal-layout">
            <div className="signal-gauge-block">
              <div className="signal-gauge" style={{ "--signal": `${signalLevel}%` } as CSSProperties}>
                <strong>{signalSummary.value.replace(" dBm", "")}</strong>
                <span>{signalSummary.value.endsWith("dBm") ? "dBm" : "信号"}</span>
                <em className={signalSummary.tone}>{signalSummary.quality}</em>
              </div>
              {signalSummary.range && <small className="signal-gauge-note">{signalSummary.range}</small>}
            </div>
            <div className="signal-meta-grid">
              {signalHighlights.map((item) => (
                <div key={item.label}>
                  <span>{item.label}</span>
                  <strong>{text(item.value)}</strong>
                </div>
              ))}
            </div>
          </div>
          {radioDetails.length > 0 && (
            <div className="radio-detail-grid">
              {radioDetails.map((item) => <InfoPair key={item.label} label={item.label} value={text(item.value)} />)}
            </div>
          )}
        </Panel>

        <Panel title="设备信息" icon={<Smartphone size={18} />}>
          <div className="info-list">
            <InfoRow label="制造商" value={text(info.manufacturer)} />
            <InfoRow label="型号" value={text(info.model || selected)} />
            <InfoRow label="固件版本" value={text(info.revision)} />
            <InfoRow label="IMEI" value={text(info.imei)} />
          </div>
        </Panel>
      </div>

    </div>
  );
}

function IdentityItem({ label, value, emphasis = false }: { label: string; value: string; emphasis?: boolean }) {
  return (
    <div className={emphasis ? "identity-item emphasis" : "identity-item"}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function InfoPair({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function HealthItem({ icon, label, value, tone }: { icon: ReactNode; label: string; value: string; tone: HealthTone }) {
  const stateIcon = tone === "ok" ? <CheckCircle2 size={20} /> : tone === "warn" ? <AlertTriangle size={20} /> : tone === "bad" ? <XCircle size={20} /> : <CircleHelp size={20} />;
  return (
    <div className={`health-item ${tone}`}>
      <div className="health-icon">{icon}</div>
      <div>
        <span>{label}</span>
        <strong>{value}</strong>
      </div>
      <span className="health-state-icon" aria-label={`状态：${value}`}>{stateIcon}</span>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
