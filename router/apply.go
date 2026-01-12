package router

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/rehiy/web-modem/handler"
)

func Apply() *mux.Router {
	r := mux.NewRouter()

	// API 路由
	api := r.PathPrefix("/api").Subrouter()
	ModemRegister(api)
	SmsdbRegister(api)
	WebhookRegister(api)
	SettingRegister(api)

	// WebSocket
	WebSocketRegister(r)

	// 静态文件服务
	StaticServer(r)

	return r
}

func ModemRegister(r *mux.Router) {
	mh := handler.NewModemHandler()

	// 模块列表
	r.HandleFunc("/modem/list", mh.ListModems).Methods("GET")

	// 模块操作
	r.HandleFunc("/modem/send", mh.SendModemCommand).Methods("POST")
	r.HandleFunc("/modem/info", mh.GetModemBasicInfo).Methods("GET")
	r.HandleFunc("/modem/signal", mh.GetModemSignalStrength).Methods("GET")

	// 短信读写
	r.HandleFunc("/modem/sms/list", mh.ListModemSMS).Methods("GET")
	r.HandleFunc("/modem/sms/send", mh.SendModemSMS).Methods("POST")
	r.HandleFunc("/modem/sms/delete", mh.DeleteModemSMS).Methods("POST")
}

func SmsdbRegister(r *mux.Router) {
	dh := handler.NewSmsdbHandler()

	// 短信存储管理
	r.HandleFunc("/smsdb/list", dh.ListSMS).Methods("GET")
	r.HandleFunc("/smsdb/delete", dh.DeleteSMSBatch).Methods("POST")
	r.HandleFunc("/smsdb/sync", dh.SyncSMS).Methods("POST")
}

func WebhookRegister(r *mux.Router) {
	wh := handler.NewWebhookHandler()

	// Webhook配置管理
	r.HandleFunc("/webhook", wh.CreateWebhook).Methods("POST")
	r.HandleFunc("/webhook/list", wh.ListWebhooks).Methods("GET")
	r.HandleFunc("/webhook/get", wh.GetWebhook).Methods("GET")
	r.HandleFunc("/webhook/update", wh.UpdateWebhook).Methods("PUT")
	r.HandleFunc("/webhook/delete", wh.DeleteWebhook).Methods("DELETE")
	r.HandleFunc("/webhook/test", wh.TestWebhook).Methods("POST")
}

func SettingRegister(r *mux.Router) {
	sh := handler.NewSettingHandler()

	// 设置管理
	r.HandleFunc("/settings", sh.GetSettings).Methods("GET")
	r.HandleFunc("/settings/smsdb", sh.UpdateSmsdbSettings).Methods("PUT")
	r.HandleFunc("/settings/webhook", sh.UpdateWebhookSettings).Methods("PUT")
}

func WebSocketRegister(r *mux.Router) {
	ws := handler.NewWebSocketHandler()

	r.HandleFunc("/ws/modem", ws.HandleWebSocket)
}

func StaticServer(r *mux.Router) {
	fs := http.FileServer(http.Dir("./webview"))
	r.PathPrefix("/").Handler(fs)
}
