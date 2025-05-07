package v5

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cuteLittleDevil/go-jt808/protocol/jt1078"
	"github.com/cuteLittleDevil/m7s-jt1078/v5/pkg"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
	"golang.org/x/net/context"
	"io"
	"log/slog"
	"m7s.live/v5"
	m7sPkg "m7s.live/v5/pkg"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var _ = m7s.InstallPlugin[JT1078Plugin]()

type (
	JT1078Plugin struct {
		m7s.Plugin
		Intercom    jt1078Intercom      `default:"{}" desc:"音频配置"`
		RealTime    jt1078Stream        `default:"{}" desc:"实时推流"`
		Playback    jt1078Stream        `default:"{}" desc:"回放推流"`
		Simulations []jt1078Simulations `default:"[]" desc:"模拟客户端推流"`
		sessions    *pkg.AudioManager
	}

	jt1078Intercom struct {
		Enable       bool         `default:"false" desc:"是否开启音频"`
		Jt1078Webrtc jt1078Webrtc `default:"{}" desc:"webrtc相关配置"`
		AudioPorts   [2]int       `default:"[10000,10010]" desc:"音频端口 用于下发数据"`
		OnJoinURL    string       `default:"http://127.0.0.1:10011/api/v1/join-audio" desc:"设备连接到音频端口时"`
		OnLeaveURL   string       `default:"http://127.0.0.1:10011/api/v1/leave-audio" desc:"对讲客户端离开时"`
	}

	jt1078Webrtc struct {
		IP   string `default:"127.0.0.1" desc:"外网ip"`
		Port int    `default:"8443" desc:"浏览器对讲数据传入的端口"`
	}

	jt1078Stream struct {
		Addr       string `default:"0.0.0.0:1078" desc:"视频端口"`
		OnJoinURL  string `default:"http://127.0.0.1:10011/api/v1/join" desc:"第一个包正确解析时触发"`
		OnLeaveURL string `default:"http://127.0.0.1:10011/api/v1/leave" desc:"推流客户端离开时"`
		Prefix     string `default:"live/jt1078" desc:"推流前缀"`
	}

	jt1078Simulations struct {
		Name string `default:"./data/data.txt" desc:"文件名"`
		Addr string `default:"127.0.0.1:1078" desc:"地址"`
	}
)

func (j *JT1078Plugin) OnInit() (err error) {
	if j.RealTime.Addr != "" {
		if j.Intercom.Enable {
			j.Info("audio init",
				slog.Any("ports", j.Intercom.AudioPorts),
				slog.String("join", j.Intercom.OnJoinURL),
				slog.String("leave", j.Intercom.OnLeaveURL))
			j.sessions = pkg.NewAudioManager(j.Logger, j.Intercom.AudioPorts,
				func(am *pkg.AudioManager) {
					am.OnJoinURL = j.Intercom.OnJoinURL
					am.OnLeaveURL = j.Intercom.OnLeaveURL
				})
			if err := j.sessions.Init(); err != nil {
				j.Error("init error",
					slog.String("err", err.Error()))
				return err
			}
			go j.sessions.Run()
		}

		service := pkg.NewService(j.RealTime.Addr, j.Logger,
			pkg.WithURL(j.RealTime.OnJoinURL, j.RealTime.OnLeaveURL),
			pkg.WithPubFunc(func(ctx context.Context, pack *jt1078.Packet) (publisher *m7s.Publisher, err error) {
				streamPath := strings.Join([]string{
					j.RealTime.Prefix,
					pack.Sim,
					fmt.Sprintf("%d", pack.LogicChannel),
				}, "-")
				if pub, err := j.Publish(ctx, streamPath); err == nil {
					return pub, nil
				} else if errors.Is(err, m7sPkg.ErrStreamExist) { // 实时的流名称重复了 在给一次机会
					streamPath += fmt.Sprintf("-%d", time.Now().UnixMilli())
					return j.Publish(ctx, streamPath)
				} else {
					return pub, err
				}
			}),
			pkg.WithEnableIntercom(j.Intercom.Enable),
			pkg.WithSessions(j.sessions),
			pkg.WithPTSFunc(func(_ *jt1078.Packet) time.Duration {
				return time.Duration(time.Now().UnixMilli()) * 90 // 实时视频使用本机时间戳
			}),
		)
		go service.Run()
	}
	if j.Playback.Addr != "" {
		service := pkg.NewService(j.Playback.Addr, j.Logger,
			pkg.WithURL(j.Playback.OnJoinURL, j.Playback.OnLeaveURL),
			pkg.WithPubFunc(func(ctx context.Context, pack *jt1078.Packet) (publisher *m7s.Publisher, err error) {
				streamPath := strings.Join([]string{
					j.Playback.Prefix,
					pack.Sim,
					fmt.Sprintf("%d", pack.LogicChannel),
				}, "-")
				return j.Publish(ctx, streamPath) // 回放唯一
			}),
			pkg.WithPTSFunc(func(pack *jt1078.Packet) time.Duration {
				return time.Duration(pack.Timestamp) * 90 // 录像回放使用设备的
			}),
		)
		go service.Run()
	}
	if len(j.Simulations) > 0 {
		params := make([]any, 0, len(j.Simulations))
		for _, v := range j.Simulations {
			params = append(params, slog.String(v.Name, v.Addr))
		}
		j.Info("simulations", params...)
		go j.simulationPull()
	}
	return nil
}

func (j *JT1078Plugin) RegisterHandler() map[string]http.HandlerFunc {
	if !j.Intercom.Enable {
		return nil
	}

	api, err := j.getWebrtcApi()
	if err != nil {
		j.Error("get webrtc api error",
			slog.Any("err", err))
		return nil
	}

	return map[string]http.HandlerFunc{
		// 实际路由是插件名+api -> /jt1078/api/v1/intercom
		"/api/v1/intercom": func(w http.ResponseWriter, r *http.Request) {

			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			type Request struct {
				webrtc.SessionDescription
				Group []struct {
					Sim       string `json:"sim"`
					Channel   uint8  `json:"channel"`
					AudioPort int    `json:"audioPort"`
				}
			}

			var req Request
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			offer := req.SessionDescription
			offer.Type = webrtc.SDPTypeOffer

			peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var once sync.Once
			peerConnection.OnICEConnectionStateChange(func(c webrtc.ICEConnectionState) {
				j.Debug("ice state",
					slog.String("state", c.String()))
				switch c {
				case webrtc.ICEConnectionStateDisconnected, webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateClosed:
					once.Do(func() {
						_ = peerConnection.Close()
					})
				default:
				}
			})

			if err := peerConnection.SetRemoteDescription(offer); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if err := peerConnection.SetLocalDescription(answer); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			<-gatherComplete

			peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
				go func() {
					if track.Kind() == webrtc.RTPCodecTypeAudio {
						var (
							// mac电脑浏览器上测试 包大小是160
							audioData = make([]byte, 1024)
							seq       = uint16(0)
						)
						for {
							n, _, err := track.Read(audioData)
							if err != nil {
								j.Debug("read audio data fail",
									slog.Any("err", err))
								return
							} else if n > 0 {
								efficientData := audioData[:n]
								for _, v := range req.Group {
									p := jt1078.NewCustomPacket(v.Sim, v.Channel, func(p *jt1078.Packet) {
										p.Flag.PT = jt1078.PTG711A
										p.DataType = jt1078.DataTypeA // 音频包
										p.Timestamp = uint64(time.Now().UnixMilli())
										p.Seq = seq
										p.Body = efficientData
									})
									data, _ := p.Encode()
									j.sessions.SendAudioData(v.AudioPort, data)
								}
								seq++
							}
						}
					}
				}()
			})

			w.Header().Set("Content-Type", "application/json")
			response, _ := json.Marshal(*peerConnection.LocalDescription())
			if _, err := w.Write(response); err != nil {
				j.Error("write sdp answer fail",
					slog.Any("response", response),
					slog.Any("err", err))
			}
		},
	}
}

func (j *JT1078Plugin) getWebrtcApi() (api *webrtc.API, err error) {
	settingEngine := webrtc.SettingEngine{}
	mux, err := ice.NewMultiUDPMuxFromPort(j.Intercom.Jt1078Webrtc.Port)
	if err != nil {
		return nil, err
	}
	settingEngine.SetICEUDPMux(mux)
	settingEngine.SetNAT1To1IPs([]string{j.Intercom.Jt1078Webrtc.IP}, webrtc.ICECandidateTypeHost)
	api = webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine), webrtc.WithMediaEngine(func() *webrtc.MediaEngine {
		m := &webrtc.MediaEngine{}
		if codecErr := m.RegisterCodec(webrtc.RTPCodecParameters{
			// Channels: 1-单声道 2-立体声 3-多声道环绕
			RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA, ClockRate: 8000, Channels: 2, SDPFmtpLine: ""},
			// RTP有效负载(载荷)类型，RTP Payload Type https://blog.csdn.net/caoshangpa/article/details/53008018
			PayloadType: 8,
		}, webrtc.RTPCodecTypeAudio); codecErr != nil {
			err = fmt.Errorf("failed to register PCMA codec %w", codecErr)
		}
		return m
	}()))
	return api, err
}

func (j *JT1078Plugin) simulationPull() {
	time.Sleep(1 * time.Second) // 等待jt1078服务都启动好
	for _, v := range j.Simulations {
		go func(name string, addr string) {
			j.Warn("simulation pull",
				slog.String("name", name),
				slog.String("addr", addr))
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				j.Warn("simulation pull",
					slog.String("name", name),
					slog.String("addr", addr),
					slog.String("err", err.Error()))
				return
			}
			defer func() {
				_ = conn.Close()
			}()
			content, err := os.ReadFile(name)
			if err != nil {
				j.Warn("simulation pull",
					slog.String("name", name),
					slog.String("addr", addr),
					slog.String("err", err.Error()))
			}
			data, _ := hex.DecodeString(string(content))
			const groupSum = 1023
			for {
				start := 0
				end := 0
				for i := 0; i < len(data)/groupSum; i++ {
					start = i * groupSum
					end = start + groupSum
					_, _ = conn.Write(data[start:end])
					time.Sleep(50 * time.Millisecond)
				}
				_, _ = conn.Write(data[end:])
			}
		}(v.Name, v.Addr)
	}
}
