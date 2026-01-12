package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rehiy/web-modem/database"
	"github.com/rehiy/web-modem/router"
)

const (
	listenPort = "8080"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = listenPort
	}

	// 初始化数据库
	if err := database.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

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
