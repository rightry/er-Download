package transfer

import (
	"sync"
	"time"
)

const (
	// 拥塞控制参数
	InitialWindow     = 32              // 增大初始窗口大小
	MinWindow         = 16              // 增大最小窗口大小
	MaxWindow         = 256             // 增大最大窗口大小
	RttAlpha         = 0.2             // 增大RTT平滑因子使其更快适应
	CongestionAlpha  = 0.3             // 增大拥塞度平滑因子
	ProbeInterval    = 3 * time.Second  // 减少探测间隔
)

// CongestionState 拥塞状态
type CongestionState int

const (
	SlowStart CongestionState = iota
	CongestionAvoidance
	FastRecovery
)

// CongestionController 拥塞控制器
type CongestionController struct {
	mu              sync.RWMutex
	state           CongestionState
	window          int           // 当前窗口大小
	threshold       int           // 慢启动阈值
	rtt            time.Duration  // 往返时间
	rttVar         time.Duration  // RTT变化
	congestionLevel float64       // 拥塞度 (0-1)
	lastProbeTime   time.Time
	
	// 性能统计
	totalSent       int64
	totalLost       int64
	retransmissions int64
}

// NewCongestionController 创建新的拥塞控制器
func NewCongestionController() *CongestionController {
	return &CongestionController{
		state:     SlowStart,
		window:    InitialWindow,
		threshold: MaxWindow,
		rtt:      100 * time.Millisecond, // 初始RTT估计
		lastProbeTime: time.Now(),
	}
}

// OnPacketSent 包发送事件
func (cc *CongestionController) OnPacketSent() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.totalSent++
}

// OnPacketLost 包丢失事件
func (cc *CongestionController) OnPacketLost() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.totalLost++
	cc.retransmissions++

	// 更新拥塞状态
	switch cc.state {
	case SlowStart:
		cc.threshold = cc.window / 2
		cc.window = cc.threshold
		cc.state = CongestionAvoidance
	case CongestionAvoidance:
		cc.threshold = cc.window / 2
		cc.window = cc.threshold
	case FastRecovery:
		cc.window = cc.threshold
	}

	// 更新拥塞度 - 使用更激进的恢复策略
	lossRate := float64(cc.totalLost) / float64(cc.totalSent)
	if lossRate < 0.1 { // 如果丢包率较低,保持较高的窗口
		cc.window = cc.window * 3 / 4
	}
	cc.congestionLevel = cc.congestionLevel*(1-CongestionAlpha) + lossRate*CongestionAlpha
}

// OnPacketAck 包确认事件
func (cc *CongestionController) OnPacketAck(rtt time.Duration) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// 更新RTT估计
	if cc.rtt == 0 {
		cc.rtt = rtt
		cc.rttVar = rtt / 2
	} else {
		rttDiff := cc.rtt - rtt
		if rttDiff < 0 {
			rttDiff = -rttDiff
		}
		cc.rttVar = (1-RttAlpha)*cc.rttVar + RttAlpha*rttDiff
		cc.rtt = (1-RttAlpha)*cc.rtt + RttAlpha*rtt
	}

	// 更新窗口大小
	switch cc.state {
	case SlowStart:
		cc.window++
		if cc.window >= cc.threshold {
			cc.state = CongestionAvoidance
		}
	case CongestionAvoidance:
		cc.window += 1 / cc.window
	case FastRecovery:
		// 保持当前窗口大小
	}

	// 确保窗口大小在合理范围内
	if cc.window < MinWindow {
		cc.window = MinWindow
	} else if cc.window > MaxWindow {
		cc.window = MaxWindow
	}
}

// GetWindow 获取当前窗口大小
func (cc *CongestionController) GetWindow() int {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.window
}

// GetCongestionLevel 获取当前拥塞度
func (cc *CongestionController) GetCongestionLevel() float64 {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.congestionLevel
}

// ShouldProbe 是否应该进行带宽探测
func (cc *CongestionController) ShouldProbe() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if time.Since(cc.lastProbeTime) < ProbeInterval {
		return false
	}

	// 只在拥塞度较低时进行探测
	return cc.congestionLevel < 0.1
}

// StartProbe 开始带宽探测
func (cc *CongestionController) StartProbe() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.lastProbeTime = time.Now()
	// 临时增加窗口大小进行探测
	cc.window = int(float64(cc.window) * 1.5)
	if cc.window > MaxWindow {
		cc.window = MaxWindow
	}
}

// GetStats 获取性能统计
func (cc *CongestionController) GetStats() (sent, lost, retrans int64) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.totalSent, cc.totalLost, cc.retransmissions
}

// GetRTT 获取当前RTT估计
func (cc *CongestionController) GetRTT() (rtt, rttVar time.Duration) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.rtt, cc.rttVar
} 