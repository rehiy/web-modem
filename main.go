package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/rehiy/web-modem/handlers"
)

const (
	defaultPort = "8080"
	apiPrefix   = "/api/v1"
)

func main() {
	// 初始化路由器
	r := mux.NewRouter()
	api := r.PathPrefix(apiPrefix).Subrouter()

	// 调制解调器路由
	api.HandleFunc("/modems", handlers.ListModems).Methods("GET")
	api.HandleFunc("/modem/at", handlers.SendATCommand).Methods("POST")
	api.HandleFunc("/modem/info", handlers.GetModemInfo).Methods("GET")
	api.HandleFunc("/modem/signal", handlers.GetSignalStrength).Methods("GET")

	// 短信读写路由
	api.HandleFunc("/modem/sms/list", handlers.ListSMS).Methods("GET")
	api.HandleFunc("/modem/sms/send", handlers.SendSMS).Methods("POST")
	api.HandleFunc("/modem/sms/delete", handlers.DeleteSMS).Methods("POST")

	// WebSocket
	r.HandleFunc("/ws", handlers.HandleWebSocket)

	// 静态文件服务
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("frontend")))

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
