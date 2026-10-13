import { useState } from "react";
import { AlertTriangle, FileKey, LockKeyhole, Play, RefreshCw, RotateCcw, Save, Send, ShieldAlert } from "lucide-react";
import { Button, Field, Panel, RawBox, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, apiOk, JsonMap, jsonBody, qs } from "../lib/api";
import { text } from "../lib/format";
import { getSimHealth } from "../lib/modemPresentation";

export function SimModule() {
  const { selected, info, busy, run, loadInfo } = useApp();
  const [capabilities, setCapabilities] = useState<JsonMap[]>([]);
  const [ussdCode, setUssdCode] = useState("");
  const [ussdResult, setUssdResult] = useState<JsonMap>({});
  const [pinForm, setPinForm] = useState({ pin: "", oldPin: "", newPin: "", puk: "", pukPin: "", lockPassword: "", lockEnabled: true });
  const [simFiles, setSimFiles] = useState<JsonMap[]>([]);
  const [apdu, setApdu] = useState({ command: "", result: {} as JsonMap });
  const [stk, setStk] = useState({ item: "", text: "", command: "", result: [] as JsonMap[] });
  const simHealth = getSimHealth(info);

  async function probeCapabilities() {
    if (!selected) return;
    const data = await run("capabilities", () => api<JsonMap>(`/modem/sim/capabilities?${qs({ name: selected })}`), "SIM 能力探测完成");
    setCapabilities(data.data || []);
  }

  async function sendUSSD() {
    if (!selected || !ussdCode) return;
    const data = await run("ussd", () => api<JsonMap>("/modem/sim/ussd/send", jsonBody({ name: selected, code: ussdCode })), "USSD 请求完成");
    setUssdResult(data);
  }

  async function cancelUSSD() {
    if (!selected) return;
    const data = await run("ussdCancel", () => api<JsonMap>("/modem/sim/ussd/cancel", jsonBody({ name: selected })), "USSD 已取消");
    setUssdResult(data);
  }

  async function pinAction(kind: "verify" | "change" | "lock" | "puk") {
    if (!selected) return;
    if (kind === "verify" && !confirm("确认提交 PIN 验证？")) return;
    if (kind === "change" && !confirm("确认修改 SIM PIN？")) return;
    if (kind === "lock" && !confirm(`${pinForm.lockEnabled ? "启用" : "关闭"} PIN 锁？`)) return;
    if (kind === "puk") {
      const typed = prompt('PUK 错误次数有限，输入 "确认PUK解锁" 才会继续');
      if (typed !== "确认PUK解锁") return;
    }
    const payloads = {
      verify: { name: selected, pin: pinForm.pin },
      change: { name: selected, old_pin: pinForm.oldPin, new_pin: pinForm.newPin },
      lock: { name: selected, pin_type: "SC", enable: pinForm.lockEnabled, password: pinForm.lockPassword },
      puk: { name: selected, puk: pinForm.puk, new_pin: pinForm.pukPin }
    };
    const paths = {
      verify: "/modem/sim/pin/verify",
      change: "/modem/sim/pin/change",
      lock: "/modem/sim/pin/lock",
      puk: "/modem/sim/pin/puk"
    };
    await run(`pin-${kind}`, () => apiOk(paths[kind], jsonBody(payloads[kind])), "PIN 操作完成");
    setPinForm((current) => ({ ...current, pin: "", oldPin: "", newPin: "", puk: "", pukPin: "", lockPassword: "" }));
    await loadInfo();
  }

  async function readSimFiles() {
    if (!selected) return;
    const data = await run("simFiles", () => api<JsonMap>(`/modem/sim/files?${qs({ name: selected })}`), "已读取 SIM 文件");
    setSimFiles(data.data || []);
  }

  async function sendApdu() {
    if (!selected || !apdu.command) return;
    const data = await run("apdu", () => api<JsonMap>("/modem/sim/apdu", jsonBody({ name: selected, apdu: apdu.command })), "APDU 已发送");
    setApdu((current) => ({ ...current, result: data }));
  }

  async function stkAction(kind: "session" | "select" | "respond") {
    if (!selected) return;
    const paths = {
      session: "/modem/sim/stk/session",
      select: "/modem/sim/stk/select",
      respond: "/modem/sim/stk/respond"
    };
    const payload = {
      session: { name: selected },
      select: { name: selected, item_id: Number(stk.item || 0), command: stk.command },
      respond: { name: selected, text: stk.text, command: stk.command }
    };
    const data = await run(`stk-${kind}`, () => api<JsonMap>(paths[kind], jsonBody(payload[kind])), "STK 操作完成");
    setStk((current) => ({ ...current, result: Array.isArray(data.data) ? data.data : [data.data || data] }));
  }

  return (
    <div className="grid two">
      <Panel
        title="SIM 状态"
        icon={<FileKey size={18} />}
        actions={
          <>
            <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.capabilities} disabled={!selected} onClick={() => probeCapabilities().catch(() => undefined)}>
              能力探测
            </Button>
            <Button icon={<FileKey size={16} />} variant="secondary" busy={busy.simFiles} disabled={!selected} onClick={() => readSimFiles().catch(() => undefined)}>
              读取文件
            </Button>
          </>
        }
      >
        <div className="status-grid">
          <StatusPill tone={simHealth.tone === "muted" ? "neutral" : simHealth.tone}>{simHealth.value}</StatusPill>
          <StatusPill>{text(info.imsi, "IMSI -")}</StatusPill>
          <StatusPill>{text(info.number, "MSISDN -")}</StatusPill>
        </div>
        <div className="cap-grid">
          {capabilities.map((cap) => (
            <div className="cap" key={cap.key}>
              <strong>{text(cap.label)}</strong>
              <StatusPill tone={cap.supported ? "ok" : "bad"}>{cap.supported ? "支持" : "不支持"}</StatusPill>
              <code>{text(cap.command)}</code>
            </div>
          ))}
          {!capabilities.length && <div className="empty">点击「能力探测」检查模块支持的 SIM 指令</div>}
        </div>
        <div className="file-list">
          {simFiles.map((file) => (
            <article className="item" key={file.key}>
              <div>
                <strong>
                  {text(file.label)} {file.file_id ? `(${file.file_id})` : ""}
                </strong>
                <p>{text(file.decoded || file.error || file.hex)}</p>
              </div>
              <StatusPill tone={file.error ? "bad" : "ok"}>{file.error ? "失败" : "OK"}</StatusPill>
            </article>
          ))}
        </div>
      </Panel>

      <Panel title="SIM 操作" icon={<LockKeyhole size={18} />}>
        <div className="form-row">
          <input value={ussdCode} onChange={(event) => setUssdCode(event.target.value)} placeholder="*101#" />
          <Button icon={<Send size={16} />} busy={busy.ussd} disabled={!ussdCode} onClick={() => sendUSSD().catch(() => undefined)}>
            USSD
          </Button>
          <Button icon={<RotateCcw size={16} />} variant="secondary" busy={busy.ussdCancel} onClick={() => cancelUSSD().catch(() => undefined)}>
            取消
          </Button>
        </div>
        <RawBox lines={ussdResult.data?.result?.raw || ussdResult.raw || [ussdResult.data?.message || ussdResult.message].filter(Boolean)} empty="暂无 USSD 返回" />

        <div className="operation-accordion">
          <details>
            <summary>PIN / PUK</summary>
            <div className="split-form">
              <Field label="PIN">
                <input type="password" value={pinForm.pin} onChange={(event) => setPinForm({ ...pinForm, pin: event.target.value })} />
              </Field>
              <Button icon={<LockKeyhole size={16} />} variant="secondary" busy={busy["pin-verify"]} onClick={() => pinAction("verify").catch(() => undefined)}>
                解锁
              </Button>
              <Field label="旧 PIN">
                <input type="password" value={pinForm.oldPin} onChange={(event) => setPinForm({ ...pinForm, oldPin: event.target.value })} />
              </Field>
              <Field label="新 PIN">
                <input type="password" value={pinForm.newPin} onChange={(event) => setPinForm({ ...pinForm, newPin: event.target.value })} />
              </Field>
              <Button icon={<Save size={16} />} variant="secondary" busy={busy["pin-change"]} onClick={() => pinAction("change").catch(() => undefined)}>
                修改
              </Button>
              <Field label="锁密码">
                <input type="password" value={pinForm.lockPassword} onChange={(event) => setPinForm({ ...pinForm, lockPassword: event.target.value })} />
              </Field>
              <label className="toggle">
                <input type="checkbox" checked={pinForm.lockEnabled} onChange={(event) => setPinForm({ ...pinForm, lockEnabled: event.target.checked })} />
                <span>PIN 锁</span>
              </label>
              <Button icon={<ShieldAlert size={16} />} variant="secondary" busy={busy["pin-lock"]} onClick={() => pinAction("lock").catch(() => undefined)}>
                设置
              </Button>
              <Field label="PUK">
                <input type="password" value={pinForm.puk} onChange={(event) => setPinForm({ ...pinForm, puk: event.target.value })} />
              </Field>
              <Field label="PUK 新 PIN">
                <input type="password" value={pinForm.pukPin} onChange={(event) => setPinForm({ ...pinForm, pukPin: event.target.value })} />
              </Field>
              <Button icon={<AlertTriangle size={16} />} variant="danger" busy={busy["pin-puk"]} onClick={() => pinAction("puk").catch(() => undefined)}>
                PUK
              </Button>
            </div>
          </details>

          <details>
            <summary>STK</summary>
            <div className="form-row">
              <input value={stk.item} onChange={(event) => setStk({ ...stk, item: event.target.value })} placeholder="STK 项 ID" />
              <input value={stk.command} onChange={(event) => setStk({ ...stk, command: event.target.value })} placeholder="原始 STK 指令" />
              <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy["stk-session"]} onClick={() => stkAction("session").catch(() => undefined)}>
                菜单
              </Button>
              <Button icon={<Play size={16} />} variant="secondary" busy={busy["stk-select"]} onClick={() => stkAction("select").catch(() => undefined)}>
                选择
              </Button>
            </div>
            <div className="form-row">
              <input value={stk.text} onChange={(event) => setStk({ ...stk, text: event.target.value })} placeholder="STK 文本响应" />
              <Button icon={<Send size={16} />} variant="secondary" busy={busy["stk-respond"]} onClick={() => stkAction("respond").catch(() => undefined)}>
                响应
              </Button>
            </div>
            <RawBox lines={stk.result.flatMap((item) => item?.result?.raw || item?.raw || [])} empty="暂无 STK 返回" />
          </details>

          <details>
            <summary>APDU</summary>
            <div className="form-row">
              <input value={apdu.command} onChange={(event) => setApdu({ ...apdu, command: event.target.value })} placeholder="00A4000C023F00" />
              <Button icon={<Send size={16} />} variant="secondary" busy={busy.apdu} disabled={!apdu.command} onClick={() => sendApdu().catch(() => undefined)}>
                APDU
              </Button>
            </div>
            {apdu.result.data?.sw1 && (
              <div className="status-grid">
                <StatusPill tone={apdu.result.ok ? "ok" : "bad"}>SW {apdu.result.data.sw1}{apdu.result.data.sw2}</StatusPill>
                {apdu.result.data.data && <code>{apdu.result.data.data}</code>}
              </div>
            )}
            <RawBox lines={apdu.result.raw || apdu.result.result?.raw} empty="暂无 APDU 返回" />
          </details>
        </div>
      </Panel>
    </div>
  );
}
