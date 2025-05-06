package main

import (
	"context"
	"fmt"
	_ "github.com/cuteLittleDevil/m7s-jt1078/v5"
	"io"
	"m7s.live/v5"
	_ "m7s.live/v5/plugin/mp4"
	_ "m7s.live/v5/plugin/preview"
	"net/http"
)

func init() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Println(r.URL.String(), string(body))
		//{
		//	"sim": "295696659617",
		//	"channel": 1,
		//	"streamPath": "live/jt1078-295696659617-1"
		//}
	})
	go func() {
		_ = http.ListenAndServe(":10011", nil)
	}()
}

func main() {
	ctx := context.Background()
	// 使用自定义模拟器推流 读取本地文件的
	fmt.Println("preview", "http://127.0.0.1:8080/preview")
	fmt.Println("模拟实时视频流地址", "http://127.0.0.1:8080/preview/live/jt1078-295696659617-1?type=mp4")
	fmt.Println("模拟回放视频流地址", "http://127.0.0.1:8080/preview/live/jt1079-295696659617-1?type=mp4")
	_ = m7s.Run(ctx, "./config.yaml")
}
