package pkg

import (
	"github.com/go-resty/resty/v2"
	"time"
)

func onNoticeEvent(url string, httpBody map[string]any) {
	client := resty.New()
	client.SetTimeout(1 * time.Second)
	_, _ = client.R().
		SetBody(httpBody).
		ForceContentType("application/json; charset=utf-8").
		Post(url)
}
