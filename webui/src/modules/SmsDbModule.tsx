import { Clipboard, ChevronLeft, ChevronRight, Database, RefreshCw, Search, Trash2, UploadCloud } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Button, Field, Panel, StatusPill } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, JsonMap, jsonBody, qs } from "../lib/api";
import { formatTime, text } from "../lib/format";

const pageSize = 50;

export function SmsDbModule() {
  const { busy, run, notify } = useApp();
  const [sms, setSms] = useState<JsonMap[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [filters, setFilters] = useState({
    device_key: "",
    send_number: "",
    direction: "",
    start_date: "",
    end_date: ""
  });

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const allChecked = useMemo(() => sms.length > 0 && sms.every((item) => selectedIds.has(Number(item.id))), [sms, selectedIds]);

  function queryFor(nextPage = page) {
    const query: JsonMap = {
      limit: pageSize,
      offset: nextPage * pageSize,
      device_key: filters.device_key,
      send_number: filters.send_number,
      direction: filters.direction
    };

    if (filters.start_date) query.start_time = new Date(filters.start_date).toISOString();
    if (filters.end_date) {
      const end = new Date(filters.end_date);
      end.setHours(23, 59, 59, 999);
      query.end_time = end.toISOString();
    }

    return query;
  }

  async function listSms(nextPage = page) {
    const data = await run("smsdb", () => api<JsonMap>(`/smsdb/list?${qs(queryFor(nextPage))}`), "已读取短信库");
    setSms(data.data || []);
    setTotal(Number(data.total) || 0);
    setPage(nextPage);
    setSelectedIds(new Set());
  }

  async function applyFilters() {
    await listSms(0);
  }

  async function syncAllSms() {
    await run("syncAllSms", () => api("/smsdb/sync", jsonBody({})), "全部短信同步完成");
    await listSms(0);
  }

  async function deleteSms(ids: number[]) {
    if (!ids.length) return;
    if (!confirm(`确认删除 ${ids.length} 条短信记录？`)) return;
    await run("deleteDbSms", () => api("/smsdb/delete", jsonBody({ ids })), "短信库记录已删除");
    await listSms(page);
  }

  async function copySms(content: string) {
    if (!content) return;
    try {
      // 非 HTTPS（局域网 IP 访问）下 clipboard API 不可用，需要提示用户
      if (!navigator.clipboard) throw new Error("当前页面不是安全上下文（HTTPS/localhost），浏览器禁止写入剪贴板");
      await navigator.clipboard.writeText(content);
      notify({ tone: "success", title: "已复制短信内容" });
    } catch (error) {
      notify({ tone: "error", title: "复制失败", detail: error instanceof Error ? error.message : String(error) });
    }
  }

  function toggle(id: number) {
    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleAll() {
    setSelectedIds((current) => {
      if (allChecked) return new Set();
      const next = new Set(current);
      sms.forEach((item) => next.add(Number(item.id)));
      return next;
    });
  }

  // 进入页面时默认加载第一页，无需手动点筛选
  useEffect(() => {
    listSms(0).catch(() => undefined);
  }, []);

  return (
    <Panel
      title={total ? `短信库（${total} 条）` : "短信库"}
      icon={<Database size={18} />}
      actions={
        <>
          <Button icon={<UploadCloud size={16} />} variant="secondary" busy={busy.syncAllSms} onClick={syncAllSms}>
            同步全部
          </Button>
          <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.smsdb} onClick={() => listSms()}>
            刷新
          </Button>
        </>
      }
    >
      <div className="filter-bar smsdb-filter">
        <Field label="设备标识">
          <input value={filters.device_key} onChange={(event) => setFilters({ ...filters, device_key: event.target.value })} placeholder="IMEI / ICCID / IMSI / 串口 / 号码" />
        </Field>
        <Field label="发送号码">
          <input value={filters.send_number} onChange={(event) => setFilters({ ...filters, send_number: event.target.value })} placeholder="+86..." />
        </Field>
        <Field label="方向">
          <select value={filters.direction} onChange={(event) => setFilters({ ...filters, direction: event.target.value })}>
            <option value="">全部方向</option>
            <option value="in">接收</option>
            <option value="out">发送</option>
          </select>
        </Field>
        <Field label="开始日期">
          <input type="date" value={filters.start_date} onChange={(event) => setFilters({ ...filters, start_date: event.target.value })} />
        </Field>
        <Field label="结束日期">
          <input type="date" value={filters.end_date} onChange={(event) => setFilters({ ...filters, end_date: event.target.value })} />
        </Field>
        <div className="filter-actions">
          <Button icon={<Search size={16} />} busy={busy.smsdb} onClick={applyFilters}>
            筛选
          </Button>
          <Button icon={<Trash2 size={16} />} variant="danger" disabled={!selectedIds.size} onClick={() => deleteSms(Array.from(selectedIds))}>
            删除选中
          </Button>
        </div>
      </div>

      <div className="table-wrap">
        <table className="data-table smsdb-table">
          <thead>
            <tr>
              <th>
                <input type="checkbox" checked={allChecked} onChange={toggleAll} aria-label="选择全部短信" />
              </th>
              <th>方向</th>
              <th>发送号码</th>
              <th>接收号码 / 设备</th>
              <th>内容</th>
              <th>时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {sms.map((item) => (
              <tr key={item.id}>
                <td className="col-check">
                  <input type="checkbox" checked={selectedIds.has(Number(item.id))} onChange={() => toggle(Number(item.id))} aria-label={`选择短信 ${item.id}`} />
                </td>
                <td data-label="方向">
                  <StatusPill tone={item.direction === "in" ? "ok" : "neutral"}>{item.direction === "in" ? "接收" : "发送"}</StatusPill>
                </td>
                <td data-label="发送号码">{text(item.send_number)}</td>
                <td data-label="接收号码 / 设备">
                  <strong>{text(item.receive_number || getSmsDeviceLabel(item))}</strong>
                  <small>{text(item.modem_name || item.device_imei || item.sim_iccid)}</small>
                </td>
                <td className="message-cell" data-label="内容">{text(item.content)}</td>
                <td data-label="时间">{formatTime(item.receive_time || item.created_at)}</td>
                <td className="col-actions">
                  <div className="row-actions">
                    <Button icon={<Clipboard size={15} />} variant="ghost" onClick={() => copySms(text(item.content, ""))}>
                      复制
                    </Button>
                    <Button icon={<Trash2 size={15} />} variant="danger" onClick={() => deleteSms([Number(item.id)])}>
                      删除
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
            {!sms.length && (
              <tr>
                <td colSpan={7}>
                  <div className="empty">暂无短信库记录</div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="pagination">
        <span>
          第 {page + 1} 页 / 共 {totalPages} 页 (总计: {total} 条)
        </span>
        <div className="row-actions">
          <Button icon={<ChevronLeft size={16} />} variant="secondary" disabled={page <= 0} onClick={() => listSms(page - 1)}>
            上一页
          </Button>
          <Button icon={<ChevronRight size={16} />} variant="secondary" disabled={page >= totalPages - 1} onClick={() => listSms(page + 1)}>
            下一页
          </Button>
        </div>
      </div>
    </Panel>
  );
}

function getSmsDeviceLabel(sms: JsonMap) {
  const number = sms.direction === "out" ? sms.send_number : sms.receive_number;
  return [sms.device_imei, number, sms.modem_name].find((item) => {
    const value = String(item || "").trim();
    return value && value !== "-" && value.toLowerCase() !== "unknown";
  });
}
