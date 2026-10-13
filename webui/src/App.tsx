import { useEffect, useState } from "react";
import { AppProvider } from "./context/AppContext";
import { AppShell, TabKey } from "./components/AppShell";
import { AlertsModule } from "./modules/AlertsModule";
import { EuiccModule } from "./modules/EuiccModule";
import { NetworkModule } from "./modules/NetworkModule";
import { OverviewModule } from "./modules/OverviewModule";
import { PhonebookModule } from "./modules/PhonebookModule";
import { SettingsModule } from "./modules/SettingsModule";
import { SimModule } from "./modules/SimModule";
import { SmsDbModule } from "./modules/SmsDbModule";
import { SmsModule } from "./modules/SmsModule";
import { TerminalModule } from "./modules/TerminalModule";
import { WebhookModule } from "./modules/WebhookModule";

const validTabs: TabKey[] = ["overview", "sms", "sim", "euicc", "network", "phonebook", "terminal", "webhook", "smsdb", "alerts", "settings"];

function tabFromHash(): TabKey {
  const hash = window.location.hash.replace(/^#\/?/, "");
  return validTabs.includes(hash as TabKey) ? (hash as TabKey) : "overview";
}

function CurrentModule({ tab }: { tab: TabKey }) {
  switch (tab) {
    case "overview":
      return <OverviewModule />;
    case "sms":
      return <SmsModule />;
    case "smsdb":
      return <SmsDbModule />;
    case "sim":
      return <SimModule />;
    case "euicc":
      return <EuiccModule />;
    case "network":
      return <NetworkModule />;
    case "phonebook":
      return <PhonebookModule />;
    case "webhook":
      return <WebhookModule />;
    case "alerts":
      return <AlertsModule />;
    case "settings":
      return <SettingsModule />;
    case "terminal":
      return <TerminalModule />;
    default:
      return <OverviewModule />;
  }
}

export default function App() {
  const [tab, setTab] = useState<TabKey>(() => tabFromHash());

  useEffect(() => {
    if (!window.location.hash) {
      window.history.replaceState(null, "", "#/overview");
    }

    function onHashChange() {
      setTab(tabFromHash());
    }

    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  function navigateTab(nextTab: TabKey) {
    if (tab === nextTab && window.location.hash.replace(/^#\/?/, "") === nextTab) return;
    window.location.hash = `/${nextTab}`;
    setTab(nextTab);
  }

  return (
    <AppProvider>
      <AppShell tab={tab} setTab={navigateTab}>
        <CurrentModule tab={tab} />
      </AppShell>
    </AppProvider>
  );
}
