import { FormEvent, useEffect, useMemo, useState } from "react";
import { History, Play, Plus, RefreshCw, RotateCcw, Save, Send, Trash2, Webhook, X } from "lucide-react";
import { Button, Field, Notice, Panel, RawBox, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, deleteJson, JsonMap, jsonBody, putJson, qs } from "../lib/api";
import { eventLabels, formatTime, matchLabels, text } from "../lib/format";
import { EVENT_VARIABLE_HINTS, PRESET_TEMPLATES_BY_EVENT } from "../lib/webhookTemplates";

const blankWebhook = {
  name: "",
  url: "",
  template: "{}",
  enabled: true,
  event_type: "sms_received",
  match_type: "all",
  match_value: "",
  retry_count: 2,
  retry_interval_seconds: 2,
  retry_backoff: true
};

export function WebhookModule() {
  const { busy, run, settings } = useApp();
  const [webhooks, setWebhooks] = useState<JsonMap[]>([]);
  const [webhookForm, setWebhookForm] = useState<JsonMap>(blankWebhook);
  const [selectedTemplate, setSelectedTemplate] = useState("custom");
  const [editingWebhook, setEditingWebhook] = useState<number | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [webhookPreview, setWebhookPreview] = useState("{}");
  const [deliveries, setDeliveries] = useState<JsonMap[]>([]);
  const templateOptions = useMemo(() => PRESET_TEMPLATES_BY_EVENT[webhookForm.event_type] || PRESET_TEMPLATES_BY_EVENT.sms_received, [webhookForm.event_type]);
  const variableHint = EVENT_VARIABLE_HINTS[webhookForm.event_type] || EVENT_VARIABLE_HINTS.sms_received;

  async function listWebhooks() {
    const data = await run("webhooks", () => api<JsonMap[]>("/webhook/list"));
    setWebhooks(Array.isArray(data) ? data : []);
  }

  async function saveWebhook(event: FormEvent) {
    event.preventDefault();
    const path = editingWebhook ? `/webhook/update?${qs({ id: editingWebhook })}` : "/webhook";
    await run("saveWebhook", () => api(path, editingWebhook ? putJson(webhookForm) : jsonBody(webhookForm)), "Webhook 已保存");
    closeDrawer();
    await listWebhooks();
  }

  function createWebhook() {
    setEditingWebhook(null);
    setWebhookForm(blankWebhook);
    setWebhookPreview("{}");
    setSelectedTemplate("custom");
    setDrawerOpen(true);
  }

  async function editWebhook(id: number) {
    const data = await run("editWebhook", () => api<JsonMap>(`/webhook/get?${qs({ id })}`));
    setEditingWebhook(id);
    setWebhookForm({ ...blankWebhook, ...data });
    setWebhookPreview("{}");
    setSelectedTemplate("custom");
    setDrawerOpen(true);
  }

  function closeDrawer() {
    setDrawerOpen(false);
    setEditingWebhook(null);
    setWebhookForm(blankWebhook);
    setWebhookPreview("{}");
    setSelectedTemplate("custom");
  }

  async function deleteWebhook(id: number) {
    if (!confirm("确认删除这个 Webhook？")) return;
    await run("deleteWebhook", () => api(`/webhook/delete?${qs({ id })}`, deleteJson()), "Webhook 已删除");
    await listWebhooks();
  }

  async function previewWebhook() {
    const data = await run("previewWebhook", () => api<JsonMap>("/webhook/preview", jsonBody(webhookForm)));
    setWebhookPreview(data.body || "{}");
  }

  async function testWebhook(id?: number) {
    const endpoint = id ? `/webhook/test?${qs({ id })}` : "/webhook/test";
    await run("testWebhook", () => api(endpoint, jsonBody(id ? {} : webhookForm)), "Webhook 测试已发送");
    await listDeliveries();
  }

  async function listDeliveries() {
    const data = await run("deliveries", () => api<JsonMap>("/webhook/deliveries?limit=30"));
    setDeliveries(data.data || []);
  }

  function templateText(template: JsonMap) {
    return JSON.stringify(template, null, 2);
  }

  function onEventTypeChange(eventType: string) {
    const templates = PRESET_TEMPLATES_BY_EVENT[eventType] || PRESET_TEMPLATES_BY_EVENT.sms_received;
    const nextTemplate = templates.generic ? templateText(templates.generic.template) : webhookForm.template;
    setSelectedTemplate(templates.generic ? "generic" : "custom");
    setWebhookForm({ ...webhookForm, event_type: eventType, template: nextTemplate });
  }

  function onTemplateChange(templateKey: string) {
    setSelectedTemplate(templateKey);
    if (templateKey === "custom") return;

    const preset = templateOptions[templateKey];
    if (!preset) return;
    setWebhookForm({ ...webhookForm, template: templateText(preset.template) });
  }

  useEffect(() => {
    listWebhooks().catch(() => undefined);
    listDeliveries().catch(() => undefined);
  }, []);

  return (
    <div className="webhook-page">
      {settings.webhook_enabled === "false" && <Notice>Webhook 全局开关已关闭，自动通知暂停；可在设置中启用。</Notice>}
      <Panel
        title="Webhook"
        icon={<Webhook size={18} />}
        actions={
          <>
            <Button icon={<Plus size={16} />} onClick={createWebhook}>
              新建
            </Button>
            <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.webhooks} onClick={listWebhooks}>
              刷新
            </Button>
          </>
        }
      >
        <div className="webhook-list">
          {webhooks.map((item) => (
            <article className="webhook-list-item" key={item.id} onClick={() => editWebhook(item.id)}>
              <div>
                <strong>{text(item.name)}</strong>
                <p>{text(item.url)}</p>
                <span>{eventLabels[item.event_type] || item.event_type} / {matchLabels[item.match_type] || item.match_type}</span>
              </div>
              <StatusPill tone={item.enabled ? "ok" : "neutral"}>{item.enabled ? "启用" : "停用"}</StatusPill>
              <div className="row-actions" onClick={(event) => event.stopPropagation()}>
                <Button icon={<Send size={15} />} variant="ghost" onClick={() => testWebhook(item.id)}>
                  测试
                </Button>
                <Button icon={<Trash2 size={15} />} variant="danger" onClick={() => deleteWebhook(item.id)}>
                  删除
                </Button>
              </div>
            </article>
          ))}
          {!webhooks.length && <div className="empty">暂无 Webhook，点击“新建”添加一个</div>}
        </div>
      </Panel>

      <Panel
        title="发送记录"
        icon={<History size={18} />}
        actions={
          <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.deliveries} onClick={listDeliveries}>
            刷新
          </Button>
        }
      >
        <div className="list deliveries">
          {deliveries.map((item) => (
            <article className="item" key={item.id}>
              <div>
                <strong>
                  {eventLabels[item.event_type] || text(item.event_type)}
                  {item.webhook_name ? ` · ${item.webhook_name}` : ""}
                </strong>
                <p>{text(item.url)}</p>
                <span>
                  {formatTime(item.created_at)}
                  {item.http_status_code ? ` · HTTP ${item.http_status_code}` : ""}
                  {item.duration_ms ? ` · ${item.duration_ms} ms` : ""}
                  {Number(item.attempt) > 1 ? ` · 第 ${item.attempt} 次尝试` : ""}
                  {item.error ? ` · ${item.error}` : ""}
                </span>
              </div>
              <StatusPill tone={item.status === "success" ? "ok" : "bad"}>{item.status === "success" ? "成功" : "失败"}</StatusPill>
            </article>
          ))}
          {!deliveries.length && <div className="empty">暂无发送记录</div>}
        </div>
      </Panel>

      {drawerOpen && (
        <div className="drawer-backdrop" onMouseDown={closeDrawer}>
          <aside className="drawer" aria-label={editingWebhook ? "编辑 Webhook" : "新建 Webhook"} onMouseDown={(event) => event.stopPropagation()}>
            <div className="drawer-head">
              <div>
                <span>{editingWebhook ? "编辑 Webhook" : "新建 Webhook"}</span>
                <strong>{editingWebhook ? text(webhookForm.name, "未命名 Webhook") : "创建新的事件回调"}</strong>
              </div>
              <button className="icon-btn" onClick={closeDrawer} aria-label="关闭">
                <X size={20} />
              </button>
            </div>

            <form className="stack-form drawer-body" onSubmit={saveWebhook}>
              <section className="drawer-section">
                <h3>基础信息</h3>
                <Field label="名称">
                  <input value={webhookForm.name} onChange={(event) => setWebhookForm({ ...webhookForm, name: event.target.value })} />
                </Field>
                <Field label="URL">
                  <input value={webhookForm.url} onChange={(event) => setWebhookForm({ ...webhookForm, url: event.target.value })} />
                </Field>
              </section>

              <section className="drawer-section">
                <h3>触发规则</h3>
                <div className="form-row">
                  <Field label="事件">
                    <select value={webhookForm.event_type} onChange={(event) => onEventTypeChange(event.target.value)} aria-label="事件类型">
                      {Object.entries(eventLabels).map(([value, label]) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <Field label="匹配">
                    <select value={webhookForm.match_type} onChange={(event) => setWebhookForm({ ...webhookForm, match_type: event.target.value })} aria-label="匹配方式">
                      {Object.entries(matchLabels).map(([value, label]) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>
                <Field label="匹配值">
                  <input value={webhookForm.match_value} onChange={(event) => setWebhookForm({ ...webhookForm, match_value: event.target.value })} placeholder="全部匹配可留空" />
                </Field>
              </section>

              <section className="drawer-section">
                <h3>请求模板</h3>
                <div className="template-toolbar compact">
                  <select value={selectedTemplate} onChange={(event) => onTemplateChange(event.target.value)} aria-label="预设模板">
                    <option value="custom">自定义模板</option>
                    {Object.entries(templateOptions).map(([key, preset]) => (
                      <option key={key} value={key}>
                        {preset.name}
                      </option>
                    ))}
                  </select>
                  <details>
                    <summary>可用变量</summary>
                    <p>{variableHint}</p>
                  </details>
                </div>
                <textarea className="template-editor" rows={7} value={webhookForm.template} onChange={(event) => setWebhookForm({ ...webhookForm, template: event.target.value })} />
              </section>

              <section className="drawer-section">
                <h3>重试策略</h3>
                <div className="webhook-options">
                  <label className="toggle">
                    <input type="checkbox" checked={webhookForm.enabled} onChange={(event) => setWebhookForm({ ...webhookForm, enabled: event.target.checked })} />
                    <span>启用</span>
                  </label>
                  <Field label="重试次数">
                    <input type="number" min={0} max={10} value={webhookForm.retry_count} onChange={(event) => setWebhookForm({ ...webhookForm, retry_count: Number(event.target.value) })} />
                  </Field>
                  <Field label="重试间隔">
                    <input type="number" min={1} value={webhookForm.retry_interval_seconds} onChange={(event) => setWebhookForm({ ...webhookForm, retry_interval_seconds: Number(event.target.value) })} />
                  </Field>
                  <label className="toggle">
                    <input type="checkbox" checked={webhookForm.retry_backoff} onChange={(event) => setWebhookForm({ ...webhookForm, retry_backoff: event.target.checked })} />
                    <span>退避</span>
                  </label>
                </div>
              </section>

              {webhookPreview !== "{}" && <RawBox lines={webhookPreview} />}
              <div className="drawer-actions">
                <Button icon={<Save size={16} />} busy={busy.saveWebhook}>
                  保存
                </Button>
                <Button icon={<Play size={16} />} type="button" variant="secondary" onClick={previewWebhook}>
                  预览
                </Button>
                <Button icon={<Send size={16} />} type="button" variant="secondary" onClick={() => testWebhook()}>
                  测试
                </Button>
                <Button icon={<RotateCcw size={16} />} type="button" variant="ghost" onClick={closeDrawer}>
                  取消
                </Button>
              </div>
            </form>
          </aside>
        </div>
      )}
    </div>
  );
}
