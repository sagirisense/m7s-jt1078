package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

func onEventJoinAudio(c *gin.Context) {
	type Request struct {
		Port    int    `json:"port"`
		Address string `json:"address"`
	}
	var req Request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusOK, Response{
			Code: http.StatusBadRequest,
			Msg:  "参数错误",
			Data: err.Error(),
		})
		return
	}
	b, _ := json.MarshalIndent(req, "", "  ")
	fmt.Println("音频加入成功", string(b))
	c.JSON(http.StatusOK, Response{
		Code: http.StatusOK,
		Msg:  "ok",
	})
}

func onEventRealTimeJoin(c *gin.Context) {
	type Request struct {
		StreamPath string `json:"streamPath"`
		Sim        string `json:"sim"`
		Channel    int    `json:"channel"`
	}
	var req Request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusOK, Response{
			Code: http.StatusBadRequest,
			Msg:  "参数错误",
			Data: err.Error(),
		})
		return
	}
	b, _ := json.MarshalIndent(req, "", "  ")
	fmt.Println("实时视频连接成功", string(b))
	c.JSON(http.StatusOK, Response{
		Code: http.StatusOK,
		Msg:  "ok",
	})
}

func onEventRealTimeLeave(c *gin.Context) {
	type Request struct {
		StreamPath string `json:"streamPath"`
		Sim        string `json:"sim"`
		Channel    int    `json:"channel"`
	}
	var req Request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusOK, Response{
			Code: http.StatusBadRequest,
			Msg:  "参数错误",
			Data: err.Error(),
		})
		return
	}
	b, _ := json.MarshalIndent(req, "", "  ")
	fmt.Println("实时视频取消", string(b))
	c.JSON(http.StatusOK, Response{
		Code: http.StatusOK,
		Msg:  "ok",
	})
}
