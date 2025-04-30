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
		onJoinURL         string
	}

	session struct {
		// 是否使用
		use bool
		// 把音频数据发送给设备
		audioChan chan<- []byte
	}
)

func NewAudioManager(logger *slog.Logger, audioPorts [2]int, onJoinURL string) *AudioManager {
	return &AudioManager{
		logger:            logger,
		operationFuncChan: make(chan audioOperationFunc, 10),
		audioPorts:        audioPorts,
		onJoinURL:         onJoinURL,
	}
}

func (s *AudioManager) Init() error {
	audios := make(map[int]*session, 10)
	for port := s.audioPorts[0]; port <= s.audioPorts[1]; port++ {
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
								})
								if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
									s.logger.Debug("connection close",
										slog.Int("port", port),
										slog.Any("device addr", conn.RemoteAddr().String()),
										slog.Any("err", err))
									return
								}
								s.logger.Error("read data",
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
	s.audios = audios
	return nil
}

func (s *AudioManager) Run() {
	for {
		select {
		case opFunc := <-s.operationFuncChan:
			opFunc(s.audios)
		}
	}
}

func (s *AudioManager) SendAudioData(port int, data []byte) {
	ch := make(chan struct{})
	s.operationFuncChan <- func(record map[int]*session) {
		defer close(ch)
		if v, ok := record[port]; ok {
			v.audioChan <- data
		}
	}
	<-ch
}

func (s *AudioManager) allocate() (int, error) {
	type Message struct {
		audioPort int
		Err       error
	}
	ch := make(chan *Message)
	defer close(ch)
	s.operationFuncChan <- func(record map[int]*session) {
		msg := &Message{
			audioPort: -1,
			Err:       fmt.Errorf("音频端口都被使用了"),
		}
		defer func() {
			ch <- msg
		}()
		for k, v := range record {
			if !v.use {
				v.use = true
				msg.audioPort = k
				msg.Err = nil
				return
			}
		}
	}
	msg := <-ch
	return msg.audioPort, msg.Err
}

func (s *AudioManager) recycle(audioPort int) {
	ch := make(chan struct{})
	s.operationFuncChan <- func(record map[int]*session) {
		defer close(ch)
		if _, ok := record[audioPort]; ok {
			record[audioPort].use = false
		}
	}
	<-ch
}
