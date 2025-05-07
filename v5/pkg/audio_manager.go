package pkg

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

type audioOperationFunc func(record map[int]*session)

type (
	AudioManager struct {
		logger            *slog.Logger
		operationFuncChan chan audioOperationFunc
		audioPorts        [2]int
		audios            map[int]*session
		OnJoinURL         string
		OnLeaveURL        string
	}

	session struct {
		// 是否使用
		use bool
		// 把音频数据发送给设备
		audioChan chan<- []byte
	}
)

func NewAudioManager(logger *slog.Logger, audioPorts [2]int, opts ...func(am *AudioManager)) *AudioManager {
	am := &AudioManager{
		logger:            logger,
		operationFuncChan: make(chan audioOperationFunc, 10),
		audioPorts:        audioPorts,
	}
	for _, opt := range opts {
		opt(am)
	}
	return am
}

func (am *AudioManager) Init() error {
	audios := make(map[int]*session, 10)
	for port := am.audioPorts[0]; port <= am.audioPorts[1]; port++ {
		ch := make(chan []byte, 100)
		listen, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			return err
		}
		go func(writeChan <-chan []byte, port int) {
			for {
				conn, err := listen.Accept()
				if err == nil {
					// 1. 设备连接到这个端口 发送回调
					go onNoticeEvent(am.OnJoinURL, map[string]any{
						"port":    port,
						"address": conn.RemoteAddr().String(),
					})
					var (
						stopChan = make(chan struct{})
						once     sync.Once
					)

					// 2. 读取设备数据 只读不处理
					go func() {
						buf := make([]byte, 10*1024)
						defer clear(buf)
						for {
							if _, err := conn.Read(buf); err != nil {
								once.Do(func() {
									close(stopChan)
									_ = conn.Close()
								})
								if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
									am.logger.Debug("connection close",
										slog.Int("port", port),
										slog.Any("device addr", conn.RemoteAddr().String()),
										slog.Any("err", err))
									return
								}
								am.logger.Error("read data",
									slog.Int("port", port),
									slog.Any("device addr", conn.RemoteAddr().String()),
									slog.Any("err", err))
								return
							}
						}
					}()

					// 3. 把浏览器收集的音频数据发给设备
					go func() {
						for {
							select {
							case <-stopChan:
								// 4. 设备断开了
								onNoticeEvent(am.OnLeaveURL, map[string]any{
									"port":    port,
									"address": conn.RemoteAddr().String(),
								})
								return
							case data := <-writeChan:
								if _, err := conn.Write(data); err != nil {
									return
								}
							}
						}
					}()
				}
			}
		}(ch, port)
		audios[port] = &session{
			use:       false,
			audioChan: ch,
		}
	}
	am.audios = audios
	return nil
}

func (am *AudioManager) Run() {
	for {
		select {
		case opFunc := <-am.operationFuncChan:
			opFunc(am.audios)
		}
	}
}

func (am *AudioManager) SendAudioData(port int, data []byte) {
	ch := make(chan struct{})
	am.operationFuncChan <- func(record map[int]*session) {
		defer close(ch)
		if v, ok := record[port]; ok {
			v.audioChan <- data
		}
	}
	<-ch
}
