package main

import (
	"context"
	"log"

	"github.com/yudai/gotty/backend/localcommand"
	"github.com/yudai/gotty/server"
)

func main() {
	f, err := localcommand.NewFactory("bash", nil, &localcommand.Options{})
	if err != nil {
		panic(err)
	}

	log.Println("Starting web terminal server...")
	s, err := server.New(f, &server.Options{
		Address:     "0.0.0.0",
		Port:        "8026",
		PermitWrite: true,
	})
	if err != nil {
		panic(err)
	}

	// Run()方法是阻塞的，会一直运行直到服务器停止
	// 服务器启动成功的日志会在Run()内部输出
	if err := s.Run(context.Background()); err != nil {
		log.Printf("Server error: %v", err)
	}
	log.Println("Server stopped")
}
