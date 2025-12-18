package main

import (
    "log"
    "net/http"
    "os"

    "github.com/gorilla/mux"
    "github.com/rs/cors"

    "modem-manager/handlers"
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
    api.HandleFunc("/modem/send", handlers.SendATCommand).Methods("POST")
    api.HandleFunc("/modem/info", handlers.GetModemInfo).Methods("GET")
    api.HandleFunc("/modem/signal", handlers.GetSignalStrength).Methods("GET")
    
    // 短信路由
    api.HandleFunc("/modem/sms/list", handlers.ListSMS).Methods("GET")
    api.HandleFunc("/modem/sms/send", handlers.SendSMS).Methods("POST")

    // WebSocket 和静态文件
    r.HandleFunc("/ws", handlers.HandleWebSocket)
    r.PathPrefix("/").Handler(http.FileServer(http.Dir("frontend")))

    // 启动服务器
    port := os.Getenv("PORT")
    if port == "" {
        port = defaultPort
    }

    log.Printf("Server starting on :%s", port)
    log.Fatal(http.ListenAndServe(":"+port, cors.AllowAll().Handler(r)))
}
