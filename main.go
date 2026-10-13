package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/router"
	"github.com/rehiy/web-modem/service"
)

const (
	listenPort = "8080"
)

func main() {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = listenPort
	}

	// 连接数据库
	if err := database.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// 启动 IP 变化监控
	service.StartIPMonitor(60 * time.Second)
	// 启动设备监视：立即扫描一次，之后每 5 秒检查，有断开端口时重扫（断线重连），
	// 并定期全量扫描刷新设备列表快照
	service.GetModemService().StartModemWatch(5 * time.Second)
	// 启动短信兜底轮询（补录 +CMTI 通知丢失的短信并清理设备存储，防止存储写满后收信静默）
	service.StartSmsPollWatch(2 * time.Minute)
	// 启动 eUICC Profile 轮询（多 eSIM 轮流上线收取各号码暂存短信，按设备配置开关）
	service.StartEuiccRotationWatch()
	service.EnsureUSBSerialDrivers()

	// 启动服务器
	go func() {
		log.Printf("Server starting on :%s", port)
		log.Fatal(http.ListenAndServe(":"+port, router.Apply()))
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
}
