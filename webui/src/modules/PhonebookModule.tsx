import { FormEvent, useState } from "react";
import { BookUser, RefreshCw, Save, Trash2 } from "lucide-react";
import { Button, Panel } from "../components/ui";
import { useApp } from "../context/AppContext";
import { apiOk, deleteJson, JsonMap, putJson, qs } from "../lib/api";
import { text } from "../lib/format";

const storageLabels: Record<string, string> = {
  SM: "SIM 卡",
  ME: "设备内存",
  ON: "本机号码"
};

export function PhonebookModule() {
  const { selected, busy, run } = useApp();
  const [phonebook, setPhonebook] = useState<JsonMap>({});
  const [phoneForm, setPhoneForm] = useState({ storage: "SM", index: "", number: "", name: "" });
  // 列表对应的存储区；删除按列表所属存储区执行，避免切换下拉框后误删其它存储区的同索引记录
  const listedStorage = String(phonebook.storage || phoneForm.storage);
  const entries: JsonMap[] = phonebook.entries || [];

  async function listPhonebook(storage = phoneForm.storage) {
    if (!selected) return;
    const data = await run("phonebook", () => apiOk(`/modem/sim/phonebook?${qs({ name: selected, storage })}`));
    setPhonebook(data.data || data);
  }

  function changeStorage(storage: string) {
    setPhoneForm({ ...phoneForm, storage });
    setPhonebook({});
  }

  async function savePhonebook(event: FormEvent) {
    event.preventDefault();
    if (!selected || !phoneForm.index || !phoneForm.number.trim()) return;
    await run(
      "savePhone",
      () => apiOk("/modem/sim/phonebook", putJson({ name: selected, storage: phoneForm.storage, index: Number(phoneForm.index), number: phoneForm.number.trim(), label: phoneForm.name.trim() })),
      "电话簿已写入"
    );
    setPhoneForm({ ...phoneForm, index: "", number: "", name: "" });
    await listPhonebook();
  }

  async function deletePhonebook(index: number) {
    if (!selected) return;
    if (!confirm(`确认删除「${storageLabels[listedStorage] || listedStorage}」电话簿第 ${index} 项？`)) return;
    await run("deletePhone", () => apiOk("/modem/sim/phonebook", deleteJson({ name: selected, storage: listedStorage, index })), "电话簿已删除");
    await listPhonebook(listedStorage);
  }

  // 编辑：把记录填回表单，修改后按同一索引覆盖写入
  function editEntry(entry: JsonMap) {
    setPhoneForm({ storage: listedStorage, index: String(entry.index), number: text(entry.number, ""), name: text(entry.name, "") });
  }

  return (
    <Panel
      title="电话簿"
      icon={<BookUser size={18} />}
      actions={
        <Button icon={<RefreshCw size={16} />} variant="secondary" busy={busy.phonebook} disabled={!selected} onClick={() => listPhonebook().catch(() => undefined)}>
          读取
        </Button>
      }
    >
      <form className="form-row" onSubmit={(event) => savePhonebook(event).catch(() => undefined)}>
        <select value={phoneForm.storage} aria-label="存储区" onChange={(event) => changeStorage(event.target.value)}>
          {Object.entries(storageLabels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
        <input type="number" min={1} value={phoneForm.index} onChange={(event) => setPhoneForm({ ...phoneForm, index: event.target.value })} placeholder="位置（索引）" aria-label="位置" />
        <input value={phoneForm.number} onChange={(event) => setPhoneForm({ ...phoneForm, number: event.target.value })} placeholder="号码" aria-label="号码" />
        <input value={phoneForm.name} onChange={(event) => setPhoneForm({ ...phoneForm, name: event.target.value })} placeholder="姓名" aria-label="姓名" />
        <Button icon={<Save size={16} />} busy={busy.savePhone} disabled={!selected || !phoneForm.index || !phoneForm.number.trim()}>
          写入
        </Button>
      </form>
      <div className="list">
        {entries.map((entry) => (
          <article className="item" key={entry.index}>
            <div>
              <strong>
                {entry.index}. {text(entry.name, "（无姓名）")}
              </strong>
              <p>{text(entry.number)}</p>
              <span>{text(storageLabels[listedStorage] || listedStorage)}</span>
            </div>
            <div className="row-actions">
              <Button variant="ghost" onClick={() => editEntry(entry)}>
                编辑
              </Button>
              <Button icon={<Trash2 size={15} />} variant="danger" busy={busy.deletePhone} onClick={() => deletePhonebook(entry.index).catch(() => undefined)}>
                删除
              </Button>
            </div>
          </article>
        ))}
        {!entries.length && <div className="empty">{busy.phonebook ? "正在读取…" : `点击「读取」加载${storageLabels[phoneForm.storage] || ""}电话簿`}</div>}
      </div>
    </Panel>
  );
}
