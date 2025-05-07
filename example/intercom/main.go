package main

import (
	"context"
	"fmt"
	"github.com/cuteLittleDevil/go-jt808/service"
	_ "github.com/cuteLittleDevil/m7s-jt1078/v5"
	"github.com/gin-gonic/gin"
	_ "github.com/gin-gonic/gin"
	"m7s.live/v5"
	_ "m7s.live/v5/plugin/flv"
	_ "m7s.live/v5/plugin/mp4"
	_ "m7s.live/v5/plugin/preview"
)

func init() {
	go func() {
		_ = m7s.Run(context.Background(), "./config.yaml")
	}()
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, HEAD, PATCH, OPTIONS, GET, PUT")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	{
		goJt808 := service.New(
			service.WithHostPorts("0.0.0.0:12001"),
			service.WithCustomTerminalEventer(func() service.TerminalEventer {
				return &LogTerminal{}
			}),
		)
		go goJt808.Run()
		r.Use(func(c *gin.Context) {
			c.Set("jt808", goJt808)
		})
	}

	group := r.Group("/api/v1/jt808/")
	{
		group.POST("/9101", p9101)
		group.POST("/9102", p9102)
		group.POST("/9201", p9201)
		group.POST("/9202", p9202)
		group.POST("/9205", p9205)
		group.POST("/9206", p9206)
		group.POST("/9208", p9208)
	}
	onEvent := r.Group("/api/v1/jt808/event/")
	{
		onEvent.POST("/join-audio", onEventJoinAudio)
		onEvent.POST("/leave-audio", onEventLeaveAudio)
		onEvent.POST("/real-time-join", onEventRealTimeJoin)
		onEvent.POST("/real-time-leave", onEventRealTimeLeave)
	}
	r.Static("/", "./static")
	// https://go-jt808.online:12000/
	// https://124.221.30.46:12000
	fmt.Println("服务已启动 默认首页:", "https://go-jt808.online:12000/")
	fmt.Println(r.RunTLS(":12000", "go-jt808.online.crt", "go-jt808.online.key"))
}
