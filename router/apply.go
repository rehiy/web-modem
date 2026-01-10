package router

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rehiy/web-modem/handler"
)

func Apply() *mux.Router {
	// 初始化路由器
	r := mux.NewRouter()
	api := r.PathPrefix("/api/v1").Subrouter()

	// Modem API
	ModemRegister(api)

	// 静态文件服务
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("webview")))

	return r
}

func ModemRegister(r *mux.Router) {
	mh := handler.NewModemHandler()

	// 模块列表
	r.HandleFunc("/modem/list", mh.ListModems).Methods("GET")

	// 模块操作
	r.HandleFunc("/modem/send", mh.SendATCommand).Methods("POST")
	r.HandleFunc("/modem/info", mh.GetModemInfo).Methods("GET")
	r.HandleFunc("/modem/signal", mh.GetSignalStrength).Methods("GET")

	// 短信读写
	r.HandleFunc("/modem/sms/list", mh.ListSMS).Methods("GET")
	r.HandleFunc("/modem/sms/send", mh.SendSMS).Methods("POST")
	r.HandleFunc("/modem/sms/delete", mh.DeleteSMS).Methods("POST")

	// 侦听模块通知
	r.HandleFunc("/modem/ws", mh.HandleWebSocket)
}
