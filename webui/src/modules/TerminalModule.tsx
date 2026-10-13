import { FormEvent, useEffect, useState } from "react";
import { Send, Terminal, Trash2 } from "lucide-react";
import { Button, Panel, RawBox } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, JsonMap, jsonBody } from "../lib/api";
import { rawLines } from "../lib/format";

const terminalHistoryKey = "web-modem-terminal-history";
const maxHistoryLines = 200;

export function TerminalModule() {
  const { selected, busy, run } = useApp();
  const [terminal, setTerminal] = useState({ command: "AT", timeout: 5 });
  const [history, setHistory] = useState<string[]>(() => {
    try {
      const saved = window.localStorage.getItem(terminalHistoryKey);
      const parsed = saved ? JSON.parse(saved) : [];
      return Array.isArray(parsed) ? parsed.map(String).slice(-maxHistoryLines) : [];
    } catch {
      return [];
    }
  });

  useEffect(() => {
    window.localStorage.setItem(terminalHistoryKey, JSON.stringify(history.slice(-maxHistoryLines)));
  }, [history]);

  function appendHistory(lines: string[]) {
    setHistory((current) => [...current, ...lines].slice(-maxHistoryLines));
  }

  async function sendTerminal(event: FormEvent) {
    event.preventDefault();
    if (!selected || !terminal.command) return;
    const stamp = new Date().toLocaleTimeString();
    try {
      const data = await run(
        "terminal",
        () => api<JsonMap>("/modem/send", jsonBody({ name: selected, command: terminal.command, timeout: terminal.timeout })),
        "AT 指令完成"
      );
      const response: string[] = rawLines(data.response || data.raw || data);
      appendHistory([`${stamp} ${selected} > ${terminal.command}`, ...(response.length ? response.map((line) => `  ${line}`) : ["  无响应"])]);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      appendHistory([`${stamp} ${selected} > ${terminal.command}`, `  ERROR ${message}`]);
    }
  }

  function clearHistory() {
    setHistory([]);
  }

  return (
    <div className="grid terminal-page">
      <Panel
        title="AT 终端"
        icon={<Terminal size={18} />}
        actions={
          <Button icon={<Trash2 size={16} />} variant="ghost" disabled={!history.length} onClick={clearHistory}>
            清空
          </Button>
        }
      >
        <form className="form-row terminal-form" onSubmit={sendTerminal}>
          <input value={terminal.command} onChange={(event) => setTerminal({ ...terminal, command: event.target.value })} />
          <label className="timeout-field" title="超时时间">
            <input type="number" min={1} max={120} value={terminal.timeout} aria-label="超时时间，秒" onChange={(event) => setTerminal({ ...terminal, timeout: Number(event.target.value) })} />
            <span>秒</span>
          </label>
          <Button icon={<Send size={16} />} busy={busy.terminal}>
            发送
          </Button>
        </form>
        <RawBox lines={history} empty="暂无终端历史" />
      </Panel>
    </div>
  );
}
