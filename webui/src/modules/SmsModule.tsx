import { FormEvent, useEffect, useState } from "react";
import { MessageSquare, RefreshCw, Send, Trash2 } from "lucide-react";
import { Button, Notice, Panel, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, JsonMap, jsonBody, qs } from "../lib/api";
import { text } from "../lib/format";

// 设备存储短信状态（AT+CMGL PDU 模式 stat）
const smsStatusLabels: Record<string, string> = {
  "REC UNREAD": "未读",
  "REC READ": "已读",
  "STO UNSENT": "未发送",
  "STO SENT": "已发送"
};

export function SmsModule() {
  const { selected, busy, run, settings } = useApp();
  const [deviceSms, setDeviceSms] = useState<JsonMap[]>([]);
  const [smsForm, setSmsForm] = useState({ number: "", content: "" });

  async function listDeviceSms() {
    if (!selected) return;
    const data = await run("deviceSms", () => api<JsonMap[]>(`/modem/sms/list?${qs({ name: selected })}`));
    setDeviceSms(Array.isArray(data) ? data : []);
  }

  async function sendSms(event: FormEvent) {
    event.preventDefault();
    if (!selected || !smsForm.number.trim() || !smsForm.content) return;
    await run("sendSms", () => api("/modem/sms/send", jsonBody({ name: selected, number: smsForm.number.trim(), message: smsForm.content })), "短信已发送");
    setSmsForm({ number: "", content: "" });
  }

  // 长短信由多个分片组成，删除时需带上全部分片索引
  async function deleteDeviceSms(item: JsonMap) {
    if (!selected) return;
    const indices: number[] = Array.isArray(item.indices) && item.indices.length ? item.indices : [Number(item.index)];
    if (!confirm(`确认从设备删除这条短信？`)) return;
    await run(`deleteSms${item.index}`, () => api("/modem/sms/delete", jsonBody({ name: selected, indices })), "已删除设备短信");
    await listDeviceSms();
  }

  useEffect(() => {
    listDeviceSms().catch(() => undefined);
  }, []);

  return (
    <div className="grid">
      <Panel title="发送短信" icon={<Send size={18} />}>
        <form className="form-row" onSubmit={sendSms}>
          <input value={smsForm.number} onChange={(event) => setSmsForm({ ...smsForm, number: event.target.value })} placeholder="接收号码" aria-label="接收号码" />
          <input value={smsForm.content} onChange={(event) => setSmsForm({ ...smsForm, content: event.target.value })} placeholder="短信内容" aria-label="短信内容" />
          <Button icon={<Send size={16} />} busy={busy.sendSms} disabled={!selected || !smsForm.number.trim() || !smsForm.content}>
            发送
          </Button>
        </form>
      </Panel>

      <Panel
        title="设备存储短信"
        icon={<MessageSquare size={18} />}
        actions={
          <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.deviceSms} disabled={!selected} onClick={() => listDeviceSms().catch(() => undefined)}>
            刷新
          </Button>
        }
      >
        <Notice>
          {settings.smsdb_enabled === "false"
            ? "短信库自动存储已关闭，收信后仍会清理设备存储。需要保留短信时，请在设置中开启短信数据库存储。"
            : "新收到的短信会自动转存到「短信库」并从设备存储中删除，这里通常只显示尚未处理的短信。"}
        </Notice>
        <div className="list">
          {deviceSms.map((item, index) => (
            <article className="item" key={`${item.index ?? index}-${item.number}`}>
              <div>
                <strong>{text(item.number)}</strong>
                <p>{text(item.text)}</p>
                <span>{text(item.time)}</span>
              </div>
              {item.status && <StatusPill tone={item.status === "REC UNREAD" ? "warn" : "neutral"}>{smsStatusLabels[item.status] || item.status}</StatusPill>}
              <Button icon={<Trash2 size={15} />} variant="danger" busy={busy[`deleteSms${item.index}`]} onClick={() => deleteDeviceSms(item).catch(() => undefined)}>
                删除
              </Button>
            </article>
          ))}
          {!deviceSms.length && <div className="empty">{busy.deviceSms ? "正在读取…" : "设备存储中暂无短信"}</div>}
        </div>
      </Panel>
    </div>
  );
}
