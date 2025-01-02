package network

import (
	"context"
	"net"
	"sync"
	"syscall"
)

const (
	// 优化UDP缓冲区大小
	UDPBufferSize = 16 * 1024 * 1024  // 增加到16MB buffer
	// 最大传输单元
	MTU = 8192  // 增加MTU大小以提高传输效率
)

// Transport 定义了P3P传输层接口
type Transport interface {
	// Start 启动传输服务
	Start(ctx context.Context) error
	// Stop 停止传输服务
	Stop() error
	// Send 发送数据
	Send(data []byte, target *net.UDPAddr) error
	// SetReceiveCallback 设置数据接收回调
	SetReceiveCallback(callback func(data []byte, from *net.UDPAddr))
}

// UDPTransport 实现基于UDP的高性能传输
type UDPTransport struct {
	conn     *net.UDPConn
	callback func(data []byte, from *net.UDPAddr)
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	metrics  *TransportMetrics
}

// TransportMetrics 传输性能指标
type TransportMetrics struct {
	BytesSent     uint64
	BytesReceived uint64
	PacketsLost   uint64
	mu            sync.RWMutex
}

// NewUDPTransport 创建新的UDP传输实例
func NewUDPTransport(port int) (*UDPTransport, error) {
	addr := &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: port,
	}
	
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	// 设置socket选项
	if err := setUDPSocketOptions(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return &UDPTransport{
		conn: conn,
		metrics: &TransportMetrics{},
	}, nil
}

// setUDPSocketOptions 优化UDP socket参数
func setUDPSocketOptions(conn *net.UDPConn) error {
	file, err := conn.File()
	if err != nil {
		return err
	}
	defer file.Close()
	fd := int(file.Fd())

	// 增大发送缓冲区
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, UDPBufferSize)
	if err != nil {
		return err
	}

	// 增大接收缓冲区
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, UDPBufferSize)
	if err != nil {
		return err
	}

	// 设置高优先级
	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_PRIORITY, 6)
	if err != nil {
		return err
	}

	// 启用快速路径
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_FASTOPEN, 1)
	if err != nil {
		return err
	}

	return nil
}

// Start 实现Transport接口
func (t *UDPTransport) Start(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.wg.Add(1)
	
	go t.receiveLoop()
	return nil
}

// receiveLoop 持续接收数据
func (t *UDPTransport) receiveLoop() {
	defer t.wg.Done()
	
	buffer := make([]byte, MTU)
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			n, addr, err := t.conn.ReadFromUDP(buffer)
			if err != nil {
				continue
			}
			
			t.metrics.mu.Lock()
			t.metrics.BytesReceived += uint64(n)
			t.metrics.mu.Unlock()
			
			if t.callback != nil {
				data := make([]byte, n)
				copy(data, buffer[:n])
				t.callback(data, addr)
			}
		}
	}
}

// Stop 实现Transport接口
func (t *UDPTransport) Stop() error {
	if t.cancel != nil {
		t.cancel()
	}
	t.wg.Wait()
	return t.conn.Close()
}

// Send 实现Transport接口
func (t *UDPTransport) Send(data []byte, target *net.UDPAddr) error {
	n, err := t.conn.WriteToUDP(data, target)
	if err == nil {
		t.metrics.mu.Lock()
		t.metrics.BytesSent += uint64(n)
		t.metrics.mu.Unlock()
	}
	return err
}

// SetReceiveCallback 实现Transport接口
func (t *UDPTransport) SetReceiveCallback(callback func(data []byte, from *net.UDPAddr)) {
	t.callback = callback
}

// GetMetrics 获取传输性能指标
func (t *UDPTransport) GetMetrics() *TransportMetrics {
	t.metrics.mu.RLock()
	defer t.metrics.mu.RUnlock()
	
	return &TransportMetrics{
		BytesSent:     t.metrics.BytesSent,
		BytesReceived: t.metrics.BytesReceived,
		PacketsLost:   t.metrics.PacketsLost,
	}
} 