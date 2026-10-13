import { FormEvent, useEffect, useState } from "react";
import { Activity, Globe2, Network, Play, RefreshCw, RotateCcw, Save, Server } from "lucide-react";
import { Button, Notice, Panel, RawBox, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, apiOk, JsonMap, jsonBody, putJson, qs } from "../lib/api";
import { text } from "../lib/format";

export function NetworkModule() {
  const { selected, busy, run, loadInfo } = useApp();
  const [contexts, setContexts] = useState<JsonMap[]>([]);
  const [contextForm, setContextForm] = useState({ cid: 1, pdp_type: "IP", apn: "" });
  const [ips, setIps] = useState<JsonMap>({});
  const [operators, setOperators] = useState<JsonMap[]>([]);
  const [operatorForm, setOperatorForm] = useState({ mode: "0", format: "2", operator: "", act: "" });
  const [pingForm, setPingForm] = useState({ host: "8.8.8.8", count: 4 });
  const [ping, setPing] = useState<JsonMap | null>(null);

  async function listContexts() {
    if (!selected) return;
    const data = await run("contexts", () => api<JsonMap>(`/modem/network/contexts?${qs({ name: selected })}`));
    setContexts(data.data || []);
  }

  async function saveContext(event: FormEvent) {
    event.preventDefault();
    if (!selected) return;
    await run("saveContext", () => apiOk("/modem/network/context", putJson({ name: selected, ...contextForm })), "PDP 上下文已保存");
    await listContexts();
  }

  async function activateContext(cid: number, active: boolean) {
    if (!selected) return;
    await run(`context-${cid}`, () => apiOk("/modem/network/context/activate", jsonBody({ name: selected, cid, state: active ? 1 : 0 })), active ? "PDP 已激活" : "PDP 已关闭");
    await listContexts();
  }

  async function readNetworkIp() {
    if (!selected) return;
    const data = await run("ip", () => api<JsonMap>(`/modem/network/ip?${qs({ name: selected })}`), "已读取 PDP IP");
    setIps(data);
  }

  async function scanOperators() {
    if (!selected) return;
    const data = await run("operators", () => api<JsonMap>(`/modem/network/operators?${qs({ name: selected })}`), "运营商扫描完成");
    setOperators(data.data || []);
  }

  async function selectOperator(payload: JsonMap) {
    if (!selected) return;
    if (!confirm("切换选网方式会让模块重新注册网络，期间短暂断网，确认继续？")) return;
    await run("operatorSelect", () => apiOk("/modem/network/operator/select", jsonBody({ name: selected, ...payload })), "运营商选择已提交");
    await loadInfo();
  }

  async function runPing(source: "modem" | "host") {
    if (source === "modem" && !selected) return;
    const key = source === "modem" ? "modemPing" : "hostPing";
    const endpoint = source === "modem" ? "/modem/network/ping" : "/network/ping";
    const body = source === "modem" ? { name: selected, ...pingForm } : { ...pingForm };
    const data = await run(key, () => api<JsonMap>(endpoint, jsonBody(body)), "连通性测试完成");
    setPing(data.data || null);
  }

  // 当前设备的 PDP 上下文读取很快，进入页面自动加载
  useEffect(() => {
    listContexts().catch(() => undefined);
  }, []);

  // AT+CGPADDR 返回按 CID 的地址列表
  function contextIp(cid: number) {
    const list: JsonMap[] = Array.isArray(ips.data) ? ips.data : [];
    return list.find((item) => Number(item.cid) === Number(cid))?.ip;
  }

  function pingTone(result: JsonMap): "ok" | "warn" | "bad" {
    if (!result.received) return "bad";
    return result.lost ? "warn" : "ok";
  }

  function pingLabel(result: JsonMap) {
    if (!result.received) return "不通";
    return result.lost ? "部分丢包" : "连通";
  }

  return (
    <div className="grid two">
      <Panel
        title="PDP 上下文"
        icon={<Network size={18} />}
        actions={
          <>
            <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.contexts} disabled={!selected} onClick={() => listContexts().catch(() => undefined)}>
              刷新
            </Button>
            <Button icon={<Globe2 size={16} />} variant="secondary" busy={busy.ip} disabled={!selected} onClick={() => readNetworkIp().catch(() => undefined)}>
              读取 IP
            </Button>
          </>
        }
      >
        <form className="form-row" onSubmit={(event) => saveContext(event).catch(() => undefined)}>
          <input type="number" min={1} value={contextForm.cid} placeholder="CID" aria-label="CID" title="CID" onChange={(event) => setContextForm({ ...contextForm, cid: Number(event.target.value) })} />
          <select value={contextForm.pdp_type} aria-label="PDP 类型" onChange={(event) => setContextForm({ ...contextForm, pdp_type: event.target.value })}>
            <option>IP</option>
            <option>IPV6</option>
            <option>IPV4V6</option>
          </select>
          <input value={contextForm.apn} onChange={(event) => setContextForm({ ...contextForm, apn: event.target.value })} placeholder="APN" aria-label="APN" />
          <Button icon={<Save size={16} />} busy={busy.saveContext}>
            保存
          </Button>
        </form>
        <div className="list">
          {contexts.map((ctx) => (
            <article className="item" key={ctx.cid}>
              <div>
                <strong>
                  CID {ctx.cid} / {text(ctx.pdp_type)}
                </strong>
                <p>{text(ctx.apn)}</p>
                <span>{text(ctx.ip || contextIp(ctx.cid), "未读取 IP")}</span>
              </div>
              {ctx.state !== undefined && <StatusPill tone={ctx.state === 1 ? "ok" : "neutral"}>{ctx.state === 1 ? "已激活" : "未激活"}</StatusPill>}
              <div className="row-actions">
                <Button icon={<Play size={15} />} variant="secondary" busy={busy[`context-${ctx.cid}`]} onClick={() => activateContext(ctx.cid, true).catch(() => undefined)}>
                  激活
                </Button>
                <Button icon={<RotateCcw size={15} />} variant="ghost" busy={busy[`context-${ctx.cid}`]} onClick={() => activateContext(ctx.cid, false).catch(() => undefined)}>
                  关闭
                </Button>
              </div>
            </article>
          ))}
          {!contexts.length && <div className="empty">{busy.contexts ? "正在读取…" : "暂无 PDP 上下文"}</div>}
        </div>
        <RawBox lines={ips.raw || ips.data?.raw} empty="暂无 IP 响应" />
      </Panel>

      <Panel
        title="运营商"
        icon={<Globe2 size={18} />}
        actions={
          <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.operators} disabled={!selected} onClick={() => scanOperators().catch(() => undefined)}>
            扫描网络
          </Button>
        }
      >
        <Notice>扫描可用网络通常需要 30–90 秒，期间该设备的其它操作会排队等待。</Notice>
        <div className="form-row">
          <select value={operatorForm.mode} aria-label="选网模式" onChange={(event) => setOperatorForm({ ...operatorForm, mode: event.target.value })}>
            <option value="0">自动</option>
            <option value="1">手动</option>
            <option value="2">注销</option>
            <option value="4">手动优先</option>
          </select>
          <select value={operatorForm.format} aria-label="运营商格式" onChange={(event) => setOperatorForm({ ...operatorForm, format: event.target.value })}>
            <option value="2">数字</option>
            <option value="0">长名称</option>
            <option value="1">短名称</option>
          </select>
          <input value={operatorForm.operator} onChange={(event) => setOperatorForm({ ...operatorForm, operator: event.target.value })} placeholder="运营商（如 46000）" aria-label="运营商" />
          <input value={operatorForm.act} onChange={(event) => setOperatorForm({ ...operatorForm, act: event.target.value })} placeholder="接入技术（可选，如 7=LTE）" aria-label="接入技术" />
          <Button icon={<Save size={16} />} busy={busy.operatorSelect} onClick={() => selectOperator({ mode: Number(operatorForm.mode), format: Number(operatorForm.format), operator: operatorForm.operator, act: operatorForm.act === "" ? undefined : Number(operatorForm.act) }).catch(() => undefined)}>
            应用
          </Button>
        </div>
        <div className="list">
          {operators.map((op, index) => (
            <article className="item" key={`${op.numeric}-${index}`}>
              <div>
                <strong>{text(op.long_name || op.short_name || op.numeric)}</strong>
                <p>
                  {text(op.numeric)} / {text(op.act_label || op.act)}
                </p>
                <span>{text(op.status_label)}</span>
              </div>
              <Button icon={<Play size={15} />} variant="secondary" onClick={() => selectOperator({ mode: 1, format: 2, operator: op.numeric, act: op.act }).catch(() => undefined)}>
                选择
              </Button>
            </article>
          ))}
          {!operators.length && <div className="empty">{busy.operators ? "正在扫描…" : "暂无扫描结果"}</div>}
        </div>
      </Panel>

      <Panel title="连通性测试" icon={<Activity size={18} />}>
        <div className="form-row">
          <input value={pingForm.host} onChange={(event) => setPingForm({ ...pingForm, host: event.target.value })} placeholder="目标地址，如 8.8.8.8" aria-label="目标地址" />
          <input type="number" min={1} max={10} value={pingForm.count} aria-label="次数" title="次数" onChange={(event) => setPingForm({ ...pingForm, count: Number(event.target.value) })} />
          <Button icon={<Play size={16} />} busy={busy.modemPing} disabled={!selected} onClick={() => runPing("modem").catch(() => undefined)}>
            模组 Ping
          </Button>
          <Button icon={<Server size={16} />} variant="secondary" busy={busy.hostPing} onClick={() => runPing("host").catch(() => undefined)}>
            主机 Ping
          </Button>
        </div>
        {ping && (
          <div className="list">
            <article className="item">
              <div>
                <strong>
                  {ping.source === "modem" ? "模组蜂窝链路" : "主机网络"} → {text(ping.host)}
                </strong>
                <p>
                  发送 {ping.sent} / 接收 {ping.received} / 丢失 {ping.lost}
                  {ping.received ? ` · 时延 ${Math.round(ping.min_ms)}/${Math.round(ping.avg_ms)}/${Math.round(ping.max_ms)} ms（最小/平均/最大）` : ""}
                </p>
                <span>
                  {text(ping.command)}
                  {ping.finished === false ? " · 结果不完整（等待超时截断）" : ""}
                  {ping.error_msg ? ` · ${ping.error_msg}` : ""}
                </span>
              </div>
              <StatusPill tone={pingTone(ping)}>{pingLabel(ping)}</StatusPill>
            </article>
          </div>
        )}
        <RawBox lines={ping?.raw} empty="暂无测试结果" />
      </Panel>
    </div>
  );
}
