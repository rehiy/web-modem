package main

import (
	"log"
	"net/http"
	"os"

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

	// 启动服务器
	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, router.Apply()))
}
