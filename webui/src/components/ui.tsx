import { Loader2 } from "lucide-react";
import { ReactNode } from "react";
import { rawLines } from "../lib/format";

export function PageHeader({
  eyebrow,
  title,
  description,
  icon,
}: {
  eyebrow: string;
  title: string;
  description: string;
  icon?: ReactNode;
}) {
  return (
    <header className="page-header">
      {icon && <span className="page-header-icon" aria-hidden>{icon}</span>}
      <div className="page-header-copy">
        <span>{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
    </header>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
    </label>
  );
}

export function Panel({ title, icon, actions, children }: { title: string; icon?: ReactNode; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="panel">
      <div className="panel-head">
        <h2>
          {icon && <span className="panel-title-icon" aria-hidden>{icon}</span>}
          <span>{title}</span>
        </h2>
        {actions && <div className="panel-actions">{actions}</div>}
      </div>
      {children}
    </section>
  );
}

export function Button({
  children,
  icon,
  variant = "primary",
  busy = false,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  icon?: ReactNode;
  variant?: "primary" | "secondary" | "danger" | "ghost";
  busy?: boolean;
}) {
  const { disabled, ...buttonProps } = props;
  return (
    <button className={`btn ${variant}`} {...buttonProps} disabled={busy || disabled}>
      {busy ? <Loader2 size={16} className="spin" /> : icon}
      <span>{children}</span>
    </button>
  );
}

export function RawBox({ lines, empty = "无原始响应" }: { lines: any; empty?: string }) {
  const rows = rawLines(lines);
  return <pre className="raw-box">{rows.length ? rows.join("\n") : empty}</pre>;
}

/** 页面内提示条，用于说明当前上下文或给出可操作的建议 */
export function Notice({ children, tone = "info", actions }: { children: ReactNode; tone?: "info" | "warn"; actions?: ReactNode }) {
  return (
    <div className={`notice ${tone}`}>
      <div className="notice-body">{children}</div>
      {actions && <div className="notice-actions">{actions}</div>}
    </div>
  );
}

export function StatusPill({ children, tone = "neutral" }: { children: ReactNode; tone?: "ok" | "warn" | "bad" | "neutral" }) {
  return <span className={`pill ${tone}`}>{children}</span>;
}
