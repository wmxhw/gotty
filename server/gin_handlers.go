package server

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/gin-gonic/gin"
)

// GinHandlers 包含所有GoTTY的Gin处理器
type GinHandlers struct {
	server  *Server
	counter *counter
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewGinHandlers 创建Gin处理器集合
func (server *Server) NewGinHandlers(ctx context.Context) *GinHandlers {
	cctx, cancel := context.WithCancel(ctx)
	counter := newCounter(time.Duration(server.options.Timeout) * time.Second)

	return &GinHandlers{
		server:  server,
		counter: counter,
		ctx:     cctx,
		cancel:  cancel,
	}
}

// Index 主页处理器
func (h *GinHandlers) Index(c *gin.Context) {
	titleVars := h.server.titleVariables(
		[]string{"server", "master"},
		map[string]map[string]interface{}{
			"server": h.server.options.TitleVariables,
			"master": map[string]interface{}{
				"remote_addr": c.ClientIP(),
			},
		},
	)

	titleBuf := new(bytes.Buffer)
	err := h.server.titleTemplate.Execute(titleBuf, titleVars)
	if err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	indexVars := map[string]interface{}{
		"title": titleBuf.String(),
	}

	indexBuf := new(bytes.Buffer)
	err = h.server.indexTemplate.Execute(indexBuf, indexVars)
	if err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", indexBuf.Bytes())
}

// AuthToken 认证token处理器
func (h *GinHandlers) AuthToken(c *gin.Context) {
	c.Header("Content-Type", "application/javascript")
	c.String(http.StatusOK, "var gotty_auth_token = '%s';", h.server.options.Credential)
}

// Config 配置处理器
func (h *GinHandlers) Config(c *gin.Context) {
	c.Header("Content-Type", "application/javascript")
	c.String(http.StatusOK, "var gotty_term = '%s';", h.server.options.Term)
}

// WebSocket WebSocket处理器
func (h *GinHandlers) WebSocket(c *gin.Context) {
	// 升级到WebSocket连接
	conn, err := h.server.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// 处理WebSocket连接
	err = h.server.processWSConn(h.ctx, conn)
	closeReason := "normal close"
	if err != nil {
		log.Printf("WebSocket connection error: %v", err)
		closeReason = "error: " + err.Error()
	}

	log.Printf("Connection closed: %s, client: %s, connections: %d/%d",
		closeReason, c.ClientIP(), h.counter.count(), h.server.options.MaxConnection)

	if h.server.options.Once {
		h.cancel()
	}
}

// StaticFiles 静态文件处理器工厂
func (h *GinHandlers) StaticFiles(prefix string) gin.HandlerFunc {
	// 创建静态文件处理器
	fileServer := http.FileServer(
		&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: "static"},
	)

	return func(c *gin.Context) {
		log.Println("StaticFiles full request path:", c.Request.URL.Path)

		// 获取通配符路径参数
		filepath := c.Param("filepath")
		log.Println("StaticFiles filepath param:", filepath)

		// 重构请求路径，去掉路由组前缀
		// 例如：/terminal/js/gotty-bundle.js -> /js/gotty-bundle.js
		var requestPath string
		if c.Request.URL.Path == "/favicon.png" || strings.HasSuffix(c.Request.URL.Path, "/favicon.png") {
			requestPath = "/favicon.png"
		} else if filepath != "" {
			// 对于 /js/*filepath 和 /css/*filepath 路由
			if strings.Contains(c.Request.URL.Path, "/js/") {
				requestPath = "/js" + filepath
			} else if strings.Contains(c.Request.URL.Path, "/css/") {
				requestPath = "/css" + filepath
			} else {
				requestPath = filepath
			}
		} else {
			requestPath = c.Request.URL.Path
		}

		log.Println("StaticFiles final request path:", requestPath)

		// 创建新的请求，修改路径
		req := c.Request.Clone(c.Request.Context())
		req.URL.Path = requestPath

		fileServer.ServeHTTP(c.Writer, req)
	}
}

// Counter 获取连接计数器（用于外部监控）
func (h *GinHandlers) Counter() *counter {
	return h.counter
}

// Context 获取上下文（用于外部控制）
func (h *GinHandlers) Context() context.Context {
	return h.ctx
}

// Cancel 取消上下文
func (h *GinHandlers) Cancel() {
	h.cancel()
}

// RegisterRoutes 便捷方法：在指定的路由组中注册所有路由
func (h *GinHandlers) RegisterRoutes(group *gin.RouterGroup) {
	// 主页
	group.GET("/", h.Index)

	// 路径规范化：无尾部斜杠重定向到有尾部斜杠
	// 这确保JavaScript能正确构造WebSocket URL
	group.GET("", func(c *gin.Context) {
		// 重定向到带斜杠的版本
		redirectURL := c.Request.URL.Path + "/"
		log.Printf("Redirecting %s to %s for path normalization", c.Request.URL.Path, redirectURL)
		c.Redirect(301, redirectURL)
	})

	// WebSocket
	group.GET("/ws", h.WebSocket)

	// 配置文件
	group.GET("/auth_token.js", h.AuthToken)
	group.GET("/config.js", h.Config)

	// 静态文件
	staticHandler := h.StaticFiles("")
	group.GET("/js/*filepath", staticHandler)
	group.GET("/css/*filepath", staticHandler)
	group.GET("/favicon.png", staticHandler)
}

// RegisterGlobalRoutes 在全局路由中注册静态资源（解决HTML相对路径问题）
// 当HTML模板使用相对路径（如 ./css/index.css）时，浏览器会请求根路径下的资源
// 这个方法将根路径的静态资源请求代理到路由组中
func (h *GinHandlers) RegisterGlobalRoutes(router *gin.Engine, groupPrefix string) {
	// 代理根路径的静态资源到路由组
	router.GET("/css/*filepath", func(c *gin.Context) {
		// 记录原始路径并重定向到正确的路由组
		log.Printf("Global static request: %s -> %s%s", c.Request.URL.Path, groupPrefix, c.Request.URL.Path)
		c.Redirect(302, groupPrefix+c.Request.URL.Path)
	})

	router.GET("/js/*filepath", func(c *gin.Context) {
		log.Printf("Global static request: %s -> %s%s", c.Request.URL.Path, groupPrefix, c.Request.URL.Path)
		c.Redirect(302, groupPrefix+c.Request.URL.Path)
	})

	router.GET("/auth_token.js", func(c *gin.Context) {
		log.Printf("Global config request: %s -> %s%s", c.Request.URL.Path, groupPrefix, c.Request.URL.Path)
		c.Redirect(302, groupPrefix+c.Request.URL.Path)
	})

	router.GET("/config.js", func(c *gin.Context) {
		log.Printf("Global config request: %s -> %s%s", c.Request.URL.Path, groupPrefix, c.Request.URL.Path)
		c.Redirect(302, groupPrefix+c.Request.URL.Path)
	})

	router.GET("/favicon.png", func(c *gin.Context) {
		log.Printf("Global favicon request: %s -> %s%s", c.Request.URL.Path, groupPrefix, c.Request.URL.Path)
		c.Redirect(302, groupPrefix+c.Request.URL.Path)
	})
}

// RegisterRoutesWithGlobalMapping 一键注册路由并自动处理全局静态资源映射
func (h *GinHandlers) RegisterRoutesWithGlobalMapping(router *gin.Engine, groupPrefix string) {
	// 注册路由组
	group := router.Group(groupPrefix)
	h.RegisterRoutes(group)

	// 注册全局静态资源映射
	h.RegisterGlobalRoutes(router, groupPrefix)
}

// BasicAuthMiddleware 基础认证中间件
func (h *GinHandlers) BasicAuthMiddleware() gin.HandlerFunc {
	if !h.server.options.EnableBasicAuth {
		return func(c *gin.Context) { c.Next() }
	}

	creds := strings.SplitN(h.server.options.Credential, ":", 2)
	if len(creds) != 2 {
		log.Printf("Warning: Invalid credential format, should be user:pass")
		return func(c *gin.Context) { c.Next() }
	}

	accounts := gin.Accounts{
		creds[0]: creds[1],
	}
	return gin.BasicAuth(accounts)
}
