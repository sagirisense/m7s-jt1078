package pkg

import (
	"crypto/tls"
	"github.com/go-resty/resty/v2"
	"time"
)

func onNoticeEvent(url string, httpBody map[string]any) {
	client := resty.New()
	client.SetTimeout(1 * time.Second)
	// 跳过证书验证
	client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	_, _ = client.R().
		SetBody(httpBody).
		ForceContentType("application/json; charset=utf-8").
		Post(url)
}
