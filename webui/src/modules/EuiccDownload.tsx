import { BrowserMultiFormatReader, IScannerControls } from "@zxing/browser";
import jsQR from "jsqr";
import { Camera, ChevronUp, Download, ImageUp, Keyboard, ScanLine, ShieldCheck, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui";
import { useApp } from "../context/AppContext";
import { api, JsonMap, jsonBody } from "../lib/api";

// EuiccDownload 提供 Profile 下载工作台，支持摄像头扫码、上传二维码、粘贴图片和手动输入激活码。
export function EuiccDownload({ name, onClose, onDownloaded }: { name: string; onClose: () => void; onDownloaded: () => void }) {
  const { run, busy } = useApp();
  const [ac, setAc] = useState("");
  const [imei, setImei] = useState("");
  const [error, setError] = useState("");
  const [scanning, setScanning] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  // scanning 为 true 时启动摄像头扫码；组件收起或卸载时始终释放摄像头。
  useEffect(() => {
    if (!scanning) return;
    setError("");
    let controls: IScannerControls | null = null;
    let cancelled = false;
    const reader = new BrowserMultiFormatReader();

    reader
      .decodeFromVideoDevice(undefined, videoRef.current!, (result, _e, ctrl) => {
        if (!result) return;
        const text = result.getText().trim();
        if (!text) return;
        setAc(text);
        setScanning(false);
        ctrl.stop();
      })
      .then((nextControls) => {
        if (cancelled) {
          nextControls.stop();
          return;
        }
        controls = nextControls;
      })
      .catch((e: any) => {
        if (cancelled) return;
        setScanning(false);
        setError("无法访问摄像头：" + (e?.message || "需 HTTPS 或 localhost，可改用上传或手动输入"));
      });

    return () => {
      cancelled = true;
      controls?.stop();
    };
  }, [scanning]);

  // 工作台打开时监听全局粘贴（Ctrl+V），从剪贴板识别二维码图片。
  useEffect(() => {
    function onPaste(e: ClipboardEvent) {
      const items = e.clipboardData?.items;
      if (!items) return;
      for (let i = 0; i < items.length; i++) {
        if (!items[i].type.startsWith("image/")) continue;
        const file = items[i].getAsFile();
        if (file) {
          e.preventDefault();
          parseImageFile(file);
          break;
        }
      }
    }
    window.addEventListener("paste", onPaste);
    return () => window.removeEventListener("paste", onPaste);
  }, []);

  // 解析图片文件中的二维码（上传与粘贴共用）。
  function parseImageFile(file: File) {
    setError("");
    const img = new Image();
    img.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = img.width;
      canvas.height = img.height;
      const ctx = canvas.getContext("2d");
      if (!ctx) return;
      ctx.drawImage(img, 0, 0);
      const imageData = ctx.getImageData(0, 0, img.width, img.height);
      const code = jsQR(imageData.data, imageData.width, imageData.height);
      if (code?.data) {
        setAc(code.data.trim());
      } else {
        setError("未在图片中识别到二维码，请换一张清晰的原图重试");
      }
      URL.revokeObjectURL(img.src);
    };
    img.onerror = () => setError("图片加载失败");
    img.src = URL.createObjectURL(file);
  }

  function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) parseImageFile(file);
    e.target.value = "";
  }

  function closeWorkbench() {
    setScanning(false);
    setError("");
    onClose();
  }

  async function handleDownload() {
    setError("");
    const code = ac.trim();
    if (!code) {
      setError("请先输入激活码，或通过扫码、上传二维码导入");
      return;
    }
    try {
      await run(
        "euicc-download",
        () => api<JsonMap>("/euicc/download", jsonBody({ name, activationCode: code, imei: imei.trim() })),
        "Profile 下载完成",
      );
      setAc("");
      closeWorkbench();
      onDownloaded();
    } catch (e: any) {
      setError(e?.message || "下载失败");
    }
  }

  const downloading = !!busy["euicc-download"];

  return (
    <section className="euicc-download-workbench" aria-labelledby="euicc-workbench-title">
      <div className="euicc-workbench-head">
        <div>
          <span className="euicc-eyebrow">新的 eSIM 套餐</span>
          <h3 id="euicc-workbench-title">下载 Profile</h3>
          <p>激活码仅用于本次下载，不会在页面中保存。</p>
        </div>
        <Button variant="ghost" icon={<ChevronUp size={16} />} disabled={downloading} onClick={closeWorkbench}>
          收起
        </Button>
      </div>

      <div className="euicc-download-steps" aria-label="下载流程">
        <span><b>1</b> 导入激活码</span>
        <span><b>2</b> 连接运营商</span>
        <span><b>3</b> 安装到 eUICC</span>
      </div>

      <form className="euicc-download-form" onSubmit={(event) => { event.preventDefault(); handleDownload(); }}>
        <label className="euicc-activation-field">
          <span>运营商激活码</span>
          <div className={ac.trim() ? "ready" : ""}>
            <Keyboard size={17} aria-hidden />
            <input
              value={ac}
              onChange={(e) => setAc(e.target.value)}
              placeholder="LPA:1$smdp.example.com$matchingId"
              autoComplete="off"
              spellCheck={false}
              aria-describedby="euicc-activation-help"
            />
            {ac && (
              <button type="button" className="euicc-clear-code" onClick={() => setAc("")} aria-label="清空激活码">
                <X size={15} />
              </button>
            )}
          </div>
        </label>
        <p id="euicc-activation-help" className="euicc-field-help">
          常见格式为 <code>LPA:1$域名$匹配码</code>。
        </p>

        <label className="euicc-activation-field">
          <span>IMEI（模块返回异常时手动填，15 位数字）</span>
          <div className={imei.trim() ? "ready" : ""}>
            <Keyboard size={17} aria-hidden />
            <input
              value={imei}
              onChange={(e) => setImei(e.target.value)}
              placeholder="如 356938035643809"
              autoComplete="off"
              spellCheck={false}
            />
            {imei && (
              <button type="button" className="euicc-clear-code" onClick={() => setImei("")} aria-label="清空 IMEI">
                <X size={15} />
              </button>
            )}
          </div>
        </label>

        <div className="euicc-import-options">
          <Button type="button" variant="secondary" icon={<Camera size={16} />} disabled={downloading} onClick={() => setScanning((value) => !value)}>
            {scanning ? "停止扫码" : "摄像头扫码"}
          </Button>
          <Button type="button" variant="secondary" icon={<ImageUp size={16} />} disabled={downloading} onClick={() => fileRef.current?.click()}>
            上传二维码
          </Button>
          <span className="euicc-paste-hint"><ScanLine size={15} /> 也可直接按 <kbd>Ctrl</kbd> + <kbd>V</kbd> 粘贴二维码图片</span>
        </div>

        <input ref={fileRef} type="file" accept="image/*" onChange={handleUpload} className="sr-only" />

        {scanning && (
          <div className="euicc-scanner">
            <video ref={videoRef} muted playsInline aria-label="摄像头二维码扫描画面" />
            <div><Camera size={16} /> 将二维码置于镜头中央</div>
          </div>
        )}

        {error && <div className="error-hint euicc-download-error">{error}</div>}

        <div className="euicc-download-submit">
          <div className="euicc-download-note">
            <ShieldCheck size={16} />
            <span>下载通常需要 30 秒至 2 分钟，请保持设备在线。</span>
          </div>
          <Button variant="primary" icon={<Download size={16} />} busy={downloading} disabled={downloading || !ac.trim()} type="submit">
            {downloading ? "正在下载…" : "下载并安装"}
          </Button>
        </div>
      </form>
    </section>
  );
}
