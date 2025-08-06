package main

import (
	"context"
	"log"
	"net/http"
	"os/exec"

	"github.com/gin-gonic/gin"
	"github.com/yudai/gotty/backend/localcommand"
	"github.com/yudai/gotty/server"
)

func main() {
	// 创建你自己的Gin引擎
	router := gin.Default()

	// 创建gotty后端
	var shell string
	if _, err := exec.LookPath("bash"); err == nil {
		shell = "bash"
	} else if _, err := exec.LookPath("sh"); err == nil {
		shell = "sh"
	} else {
		panic("Neither bash nor sh found")
	}

	factory, err := localcommand.NewFactory(shell, nil, &localcommand.Options{})
	if err != nil {
		panic(err)
	}

	// 配置gotty服务器（不设置Address和Port，因为我们要用外部的Gin）
	gottyServer, err := server.New(factory, &server.Options{
		PermitWrite: true,
		// 其他配置...
	})
	if err != nil {
		panic(err)
	}

	// 创建gotty的Gin处理器
	gottyHandlers := gottyServer.NewGinHandlers(context.Background())

	// 示例1: 最简单的集成方式 - 自动处理HTML相对路径问题
	// 访问: http://localhost:8027/terminal/
	var prefix = "/api/v2/system/terminal"
	gottyHandlers.RegisterRoutesWithGlobalMapping(router, prefix)

	// 示例1-old: 手动方式（如果需要更精细的控制）
	// terminalGroup := router.Group("/terminal")
	// {
	// 	// 可选：添加认证中间件
	// 	// terminalGroup.Use(gottyHandlers.BasicAuthMiddleware())
	//
	// 	// 注册所有gotty路由
	// 	gottyHandlers.RegisterRoutes(terminalGroup)
	// }
	// // 手动注册全局静态资源映射
	// gottyHandlers.RegisterGlobalRoutes(router, "/terminal")

	// 示例2: 灵活集成 - 手动注册各个路由到不同位置
	// 主页在根路径
	// router.GET("/", gottyHandlers.Index)

	// // WebSocket在自定义路径
	// router.GET("/ws/shell", gottyHandlers.WebSocket)

	// // 静态文件在专门的路由组
	// staticGroup := router.Group("/assets")
	// {
	// 	staticHandler := gottyHandlers.StaticFiles("/assets")
	// 	staticGroup.GET("/js/*filepath", staticHandler)
	// 	staticGroup.GET("/css/*filepath", staticHandler)
	// 	staticGroup.GET("/favicon.png", staticHandler)
	// }

	// // 配置文件
	// router.GET("/api/auth_token.js", gottyHandlers.AuthToken)
	// router.GET("/api/config.js", gottyHandlers.Config)

	// 示例3: 多实例集成 - 同一个应用中运行多个gotty实例
	// setupMultipleInstances(router)

	// 你自己的其他路由
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	router.GET("/api/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"app":   "my-app",
			"gotty": "integrated",
		})
	})

	// 启动服务器
	log.Println("Starting server with integrated GoTTY...")
	log.Println("GoTTY Terminal 1: http://localhost:8080/terminal/")
	log.Println("GoTTY Terminal 2: http://localhost:8080/shell1/")
	log.Println("GoTTY Terminal 3: http://localhost:8080/shell2/")
	log.Println("Main App: http://localhost:8080/")
	log.Println("Health Check: http://localhost:8080/health")

	if err := router.Run(":8027"); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// 多实例示例
func setupMultipleInstances(router *gin.Engine) {
	// 创建第二个gotty实例 - bash
	factory1, _ := localcommand.NewFactory("bash", nil, &localcommand.Options{})
	gottyServer1, _ := server.New(factory1, &server.Options{
		PermitWrite: true,
		TitleFormat: "Bash Terminal - {{ .hostname }}",
	})
	handlers1 := gottyServer1.NewGinHandlers(context.Background())

	shell1Group := router.Group("/shell1")
	{
		handlers1.RegisterRoutes(shell1Group)
	}

	// 创建第三个gotty实例 - 可以是其他命令
	factory2, _ := localcommand.NewFactory("sh", nil, &localcommand.Options{})
	gottyServer2, _ := server.New(factory2, &server.Options{
		PermitWrite:     true,
		TitleFormat:     "Shell Terminal - {{ .hostname }}",
		EnableBasicAuth: true,
		Credential:      "admin:secret",
	})
	handlers2 := gottyServer2.NewGinHandlers(context.Background())

	shell2Group := router.Group("/shell2")
	{
		// 这个实例有认证
		shell2Group.Use(handlers2.BasicAuthMiddleware())
		handlers2.RegisterRoutes(shell2Group)
	}
}
