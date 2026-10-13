package router

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/rehiy/web-modem/handler"
	"github.com/rehiy/web-modem/webview"
)

func Apply() http.Handler {
	r := mux.NewRouter()

	if target := os.Getenv("API_PROXY_TARGET"); target != "" {
		if err := ProxyRegister(r, target); err != nil {
			log.Printf("API proxy disabled: %v", err)
			LocalAPIRegister(r)
		}
	} else {
		LocalAPIRegister(r)
	}

	// 静态文件服务
	StaticServer(r)

	// 应用 Basic Auth 中间件
	return BasicAuthMiddleware(r)
}

func LocalAPIRegister(r *mux.Router) {
	api := r.PathPrefix("/api").Subrouter()
	ModemRegister(api)
	SmsdbRegister(api)
	SimAppRegister(api)
	PingRegister(api)
	EuiccRegister(api)
	WebhookRegister(api)
	AlertRegister(api)
	SettingRegister(api)
	UpdateRegister(api)

	// SSE
	SseRegister(r)
}

func ModemRegister(r *mux.Router) {
	mh := handler.NewModemHandler()

	// 模块列表
	r.HandleFunc("/modem/list", mh.ListModems).Methods("GET")

	// 模块操作
	r.HandleFunc("/modem/send", mh.SendModemCommand).Methods("POST")
	r.HandleFunc("/modem/info", mh.GetModemBasicInfo).Methods("GET")
	r.HandleFunc("/modem/signal", mh.GetModemSignal).Methods("GET")

	// 短信读写
	r.HandleFunc("/modem/sms/list", mh.ListModemSms).Methods("GET")
	r.HandleFunc("/modem/sms/send", mh.SendModemSms).Methods("POST")
	r.HandleFunc("/modem/sms/delete", mh.DeleteModemSms).Methods("POST")
}

func SmsdbRegister(r *mux.Router) {
	dh := handler.NewSmsdbHandler()

	// 短信存储管理
	r.HandleFunc("/smsdb/list", dh.ListSms).Methods("GET")
	r.HandleFunc("/smsdb/delete", dh.DeleteSmsBatch).Methods("POST")
	r.HandleFunc("/smsdb/sync", dh.SyncSms).Methods("POST")
}

func EuiccRegister(r *mux.Router) {
	eh := handler.NewEuiccHandler()
	r.HandleFunc("/euicc/profiles", eh.ListProfiles).Methods("GET")
	r.HandleFunc("/euicc/profiles/{iccid}/enable", eh.EnableProfile).Methods("POST")
	r.HandleFunc("/euicc/profiles/{iccid}/disable", eh.DisableProfile).Methods("POST")
	r.HandleFunc("/euicc/profiles/{iccid}/number", eh.SetProfileNumber).Methods("PUT")
	r.HandleFunc("/euicc/profiles/{iccid}", eh.DeleteProfile).Methods("DELETE")
	r.HandleFunc("/euicc/compare", eh.CompareProfiles).Methods("GET")
	r.HandleFunc("/euicc/info2", eh.GetInfo2).Methods("GET")
	r.HandleFunc("/euicc/addresses", eh.GetAddresses).Methods("GET")
	r.HandleFunc("/euicc/addresses", eh.SetDefaultSMDP).Methods("PUT")
	r.HandleFunc("/euicc/notifications", eh.ListNotifications).Methods("GET")
	r.HandleFunc("/euicc/notifications/{seq}/handle", eh.HandleNotification).Methods("POST")
	r.HandleFunc("/euicc/rotation", eh.GetRotation).Methods("GET")
	r.HandleFunc("/euicc/rotation", eh.SetRotation).Methods("PUT")
	r.HandleFunc("/euicc/rotation/run", eh.RunRotation).Methods("POST")
	r.HandleFunc("/euicc/download", eh.DownloadProfile).Methods("POST")
}

func SimAppRegister(r *mux.Router) {
	sh := handler.NewSimAppHandler()

	r.HandleFunc("/modem/sim/capabilities", sh.GetCapabilities).Methods("GET")
	r.HandleFunc("/modem/sim/ussd/send", sh.SendUSSD).Methods("POST")
	r.HandleFunc("/modem/sim/ussd/cancel", sh.CancelUSSD).Methods("POST")
	r.HandleFunc("/modem/sim/stk/session", sh.StartSTKSession).Methods("POST")
	r.HandleFunc("/modem/sim/stk/select", sh.SelectSTKItem).Methods("POST")
	r.HandleFunc("/modem/sim/stk/respond", sh.RespondSTK).Methods("POST")
	r.HandleFunc("/modem/sim/pin/verify", sh.VerifyPIN).Methods("POST")
	r.HandleFunc("/modem/sim/pin/change", sh.ChangePIN).Methods("POST")
	r.HandleFunc("/modem/sim/pin/lock", sh.SetPINLock).Methods("POST")
	r.HandleFunc("/modem/sim/pin/puk", sh.UnlockPUK).Methods("POST")
	r.HandleFunc("/modem/sim/files", sh.ReadSIMFiles).Methods("GET")
	r.HandleFunc("/modem/sim/own-number", sh.SetOwnNumber).Methods("POST")
	r.HandleFunc("/modem/sim/apdu", sh.SendAPDU).Methods("POST")
	r.HandleFunc("/modem/sim/phonebook", sh.ListPhonebook).Methods("GET")
	r.HandleFunc("/modem/sim/phonebook", sh.WritePhonebook).Methods("PUT")
	r.HandleFunc("/modem/sim/phonebook", sh.DeletePhonebook).Methods("DELETE")

	r.HandleFunc("/modem/network/contexts", sh.ListNetworkContexts).Methods("GET")
	r.HandleFunc("/modem/network/context", sh.SetNetworkContext).Methods("PUT")
	r.HandleFunc("/modem/network/context/activate", sh.ActivateNetworkContext).Methods("POST")
	r.HandleFunc("/modem/network/ip", sh.GetNetworkIP).Methods("GET")
	r.HandleFunc("/modem/network/operators", sh.ListOperators).Methods("GET")
	r.HandleFunc("/modem/network/operator/select", sh.SelectOperator).Methods("POST")
}

func PingRegister(r *mux.Router) {
	ph := handler.NewPingHandler()

	// 连通性测试：模组蜂窝链路 / 本机网络
	r.HandleFunc("/modem/network/ping", ph.ModemPing).Methods("POST")
	r.HandleFunc("/network/ping", ph.HostPing).Methods("POST")
}

func WebhookRegister(r *mux.Router) {
	wh := handler.NewWebhookHandler()

	// Webhook配置管理
	r.HandleFunc("/webhook", wh.CreateWebhook).Methods("POST")
	r.HandleFunc("/webhook/list", wh.ListWebhooks).Methods("GET")
	r.HandleFunc("/webhook/get", wh.GetWebhook).Methods("GET")
	r.HandleFunc("/webhook/update", wh.UpdateWebhook).Methods("PUT")
	r.HandleFunc("/webhook/delete", wh.DeleteWebhook).Methods("DELETE")
	r.HandleFunc("/webhook/preview", wh.PreviewWebhook).Methods("POST")
	r.HandleFunc("/webhook/deliveries", wh.ListWebhookDeliveries).Methods("GET")
	r.HandleFunc("/webhook/test", wh.TestWebhook).Methods("POST")
}

func AlertRegister(r *mux.Router) {
	ah := handler.NewAlertHandler()

	r.HandleFunc("/alerts/list", ah.ListAlerts).Methods("GET")
	r.HandleFunc("/alerts/scan", ah.ScanAlerts).Methods("POST")
	r.HandleFunc("/alerts/resolve", ah.ResolveAlerts).Methods("POST")
}

func SettingRegister(r *mux.Router) {
	sh := handler.NewSettingHandler()

	// 设置管理
	r.HandleFunc("/settings", sh.GetSettings).Methods("GET")
	r.HandleFunc("/settings/smsdb", sh.UpdateSmsdbSettings).Methods("PUT")
	r.HandleFunc("/settings/webhook", sh.UpdateWebhookSettings).Methods("PUT")
}

func UpdateRegister(r *mux.Router) {
	uh := handler.NewUpdateHandler()

	r.HandleFunc("/update/check", uh.CheckUpdate).Methods("GET")
	r.HandleFunc("/update/apply", uh.ApplyUpdate).Methods("POST")
	r.HandleFunc("/update/restart", uh.Restart).Methods("POST")
}

func SseRegister(r *mux.Router) {
	sse := handler.NewSseHandler()

	r.HandleFunc("/events/modem", sse.HandleModemEvents).Methods("GET")
}

func StaticServer(r *mux.Router) {
	var staticFS http.FileSystem
	if _, err := os.Stat("./webview/dist/index.html"); err == nil {
		staticFS = http.Dir("./webview/dist")
	} else if sub, err := fs.Sub(webview.FS, "dist"); err == nil {
		staticFS = http.FS(sub)
	}

	r.PathPrefix("/").Handler(spaStaticServer{fs: staticFS})
}

type spaStaticServer struct {
	fs http.FileSystem
}

func (s spaStaticServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.fs == nil {
		http.NotFound(w, r)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	file, err := s.fs.Open(path)
	if err == nil {
		_ = file.Close()
		http.FileServer(s.fs).ServeHTTP(w, r)
		return
	}

	r2 := new(http.Request)
	*r2 = *r
	r2.URL.Path = "/index.html"
	index, err := s.fs.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer index.Close()

	http.ServeContent(w, r2, "index.html", time.Time{}, index)
}
