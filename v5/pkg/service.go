package pkg

import (
	"context"
	"github.com/cuteLittleDevil/go-jt808/protocol/jt1078"
	"github.com/go-resty/resty/v2"
	"log/slog"
	"m7s.live/v5"
	"net"
	"net/http"
	"time"
)

func NewService(addr string, log *slog.Logger, opts ...Option) *Service {
	options := &Options{
		pubFunc: func(ctx context.Context, pack *jt1078.Packet) (publisher *m7s.Publisher, err error) {
			return nil, nil
		},
	}
	for _, op := range opts {
		op.F(options)
	}
	s := &Service{
		Logger: log,
		addr:   addr,
		opts:   options,
	}
	return s
}

type Service struct {
	*slog.Logger
	addr string
	opts *Options
}

func (s *Service) Run() {
	listen, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.Error("listen error",
			slog.String("addr", s.addr),
			slog.String("err", err.Error()))
		return
	}
	s.Info("listen tcp",
		slog.String("addr", s.addr))
	for {
		conn, err := listen.Accept()
		if err != nil {
			s.Warn("accept error",
				slog.String("err", err.Error()))
			return
		}
		client := newConnection(conn, s.Logger, s.opts.ptsFunc)
		var (
			httpBody  = map[string]any{}
			audioPort int
		)
		if s.opts.intercom {
			audioPort, err = s.opts.sessions.allocate()
			if err != nil {
				s.Warn("allocate error",
					slog.String("err", err.Error()))
				audioPort = -1
			}
			if s.opts.onAudioJoinURL != "" {
				if !s.onIntercomEvent(s.opts.onJoinURL, audioPort) {
					audioPort = 0
				}
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		client.onJoinEvent = func(c *connection, pack *jt1078.Packet) error {
			publisher, err := s.opts.pubFunc(ctx, pack)
			if err != nil {
				return err
			}
			c.publisher = publisher
			httpBody = map[string]any{
				"streamPath": c.publisher.StreamPath,
				"sim":        pack.Sim,
				"channel":    pack.LogicChannel,
				"audioPort":  audioPort,
			}
			go s.onNoticeEvent(s.opts.onJoinURL, httpBody)
			return nil
		}
		client.onLeaveEvent = func() {
			if s.opts.intercom {
				s.opts.sessions.recycle(audioPort)
			}
			if len(httpBody) > 0 {
				go s.onNoticeEvent(s.opts.onLeaveURL, httpBody)
			}
			cancel()
		}
		go func() {
			if err := client.run(); err != nil {
				s.Warn("run error",
					slog.Any("http body", httpBody),
					slog.String("err", err.Error()))
			}
		}()
	}
}

func (s *Service) onNoticeEvent(url string, httpBody map[string]any) {
	client := resty.New()
	client.SetTimeout(1 * time.Second)
	_, _ = client.R().
		SetBody(httpBody).
		ForceContentType("application/json; charset=utf-8").
		Post(url)
}

func (s *Service) onIntercomEvent(url string, audioPort int) bool {
	client := resty.New()
	client.SetTimeout(1 * time.Second)
	type Reply struct {
		UseAudio bool `json:"useAudio"`
	}
	var reply Reply
	response, err := client.R().
		SetBody(map[string]any{
			"audioPort": audioPort,
		}).
		SetResult(&reply).
		ForceContentType("application/json; charset=utf-8").
		Post(url)
	if err != nil {
		s.Warn("onIntercomEvent",
			slog.String("url", url),
			slog.String("err", err.Error()))
		return false
	}
	return response.StatusCode() == http.StatusOK && reply.UseAudio
}
