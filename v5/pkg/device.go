package pkg

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type device struct {
	conn         net.Conn
	stopChan     chan struct{}
	completeChan chan error
	stopOnce     sync.Once
	writeChan    <-chan []byte
	overTime     time.Duration
	err          error
}

func newDevice(conn net.Conn, writeChan <-chan []byte, overTime time.Duration) *device {
	return &device{
		conn:         conn,
		writeChan:    writeChan,
		overTime:     overTime,
		stopChan:     make(chan struct{}, 1),
		completeChan: make(chan error, 1),
	}
}

func (d *device) run() chan error {
	go d.read()
	go d.write()
	return d.completeChan
}

func (d *device) read() {
	// 模拟器测试读的数据 194
	buf := make([]byte, 1024)
	defer func() {
		clear(buf)
		d.stop()
	}()

	for {
		select {
		case <-d.stopChan:
			return
		default:
			if _, err := d.conn.Read(buf); err != nil {
				d.err = fmt.Errorf("read err %w", err)
				return
			}
		}
	}
}

func (d *device) write() {

	ticker := time.NewTicker(d.overTime)
	defer func() {
		ticker.Stop()
		d.stop()
	}()

	startTime := time.Now()
	for {
		select {
		case <-ticker.C:
			d.err = fmt.Errorf("over time %s", time.Since(startTime).String())
			return
		case <-d.stopChan:
			return
		case data := <-d.writeChan:
			ticker.Reset(d.overTime)
			if _, err := d.conn.Write(data); err != nil {
				d.err = fmt.Errorf("write err %w", err)
				return
			}
		}
	}
}

func (d *device) stop() {
	d.stopOnce.Do(func() {
		d.completeChan <- d.err
		close(d.completeChan)

		close(d.stopChan)
		_ = d.conn.Close()
	})
}
