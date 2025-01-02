package discovery

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"sort"
	"sync"
	"time"
)

const (
	HeartbeatInterval = 5 * time.Second
	NodeTimeout      = 30 * time.Second
	NodeCleanupTime  = 5 * time.Minute
	MaxNodes        = 100
	BroadcastPort   = 33333
)

// NodeStatus 节点状态
type NodeStatus int

const (
	NodeStatusUnknown NodeStatus = iota
	NodeStatusOnline
	NodeStatusBusy
	NodeStatusOffline
)

// Node 表示一个P3P节点
type Node struct {
	ID        string    `json:"id"`
	Addr      string    `json:"addr"`
	LastSeen  time.Time `json:"last_seen"`
	Speed     float64   `json:"speed"`      // 预估的传输速度（MB/s）
	Latency   int64     `json:"latency"`    // 延迟（毫秒）
	Available bool      `json:"available"`
	Status    NodeStatus `json:"status"`
	Load      float64   `json:"load"`       // 负载指数 0-1
	SuccessRate float64 `json:"success_rate"` // 传输成功率
}

// NodeEvent 节点事件
type NodeEvent struct {
	Type    string
	Node    *Node
	Time    time.Time
}

// Discovery 负责节点发现和管理
type Discovery struct {
	nodes    map[string]*Node
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	selfNode *Node
	conn     *net.UDPConn
	eventCh  chan NodeEvent
}

// NewDiscovery 创建新的发现服务实例
func NewDiscovery(selfAddr string) *Discovery {
	return &Discovery{
		nodes: make(map[string]*Node),
		selfNode: &Node{
			ID:        generateNodeID(selfAddr),
			Addr:      selfAddr,
			LastSeen:  time.Now(),
			Available: true,
			Status:    NodeStatusOnline,
			Load:      0,
			SuccessRate: 1.0,
		},
		eventCh: make(chan NodeEvent, 100),
	}
}

// Start 启动发现服务
func (d *Discovery) Start(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)
	
	// 启动UDP广播监听
	if err := d.startBroadcastListener(); err != nil {
		return err
	}

	// 启动心跳广播
	go d.heartbeatLoop()
	
	// 启动节点状态更新
	go d.updateLoop()

	// 启动事件处理
	go d.eventLoop()
	
	return nil
}

// startBroadcastListener 启动广播监听
func (d *Discovery) startBroadcastListener() error {
	addr := &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: BroadcastPort,
	}
	
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	
	d.conn = conn
	go d.receiveLoop()
	
	return nil
}

// heartbeatLoop 发送心跳包
func (d *Discovery) heartbeatLoop() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.broadcastHeartbeat()
		}
	}
}

// broadcastHeartbeat 广播心跳包
func (d *Discovery) broadcastHeartbeat() {
	data, err := json.Marshal(d.selfNode)
	if err != nil {
		log.Printf("心跳包序列化失败: %v", err)
		return
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4(255, 255, 255, 255),
		Port: BroadcastPort,
	}

	_, err = d.conn.WriteToUDP(data, addr)
	if err != nil {
		log.Printf("心跳包发送失败: %v", err)
	}
}

// receiveLoop 接收其他节点的广播
func (d *Discovery) receiveLoop() {
	buffer := make([]byte, 4096)
	for {
		n, addr, err := d.conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		var node Node
		if err := json.Unmarshal(buffer[:n], &node); err != nil {
			continue
		}

		// 忽略自己的广播
		if node.ID == d.selfNode.ID {
			continue
		}

		d.eventCh <- NodeEvent{
			Type: "heartbeat",
			Node: &node,
			Time: time.Now(),
		}
	}
}

// eventLoop 处理节点事件
func (d *Discovery) eventLoop() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case event := <-d.eventCh:
			d.handleNodeEvent(event)
		}
	}
}

// handleNodeEvent 处理节点事件
func (d *Discovery) handleNodeEvent(event NodeEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch event.Type {
	case "heartbeat":
		d.updateNode(event.Node)
	case "status_change":
		if node, exists := d.nodes[event.Node.ID]; exists {
			node.Status = event.Node.Status
			node.Load = event.Node.Load
		}
	}
}

// updateNode 更新节点信息
func (d *Discovery) updateNode(node *Node) {
	existing, exists := d.nodes[node.ID]
	if !exists {
		d.nodes[node.ID] = node
		return
	}

	// 更新现有节点信息
	existing.LastSeen = time.Now()
	existing.Speed = node.Speed
	existing.Latency = node.Latency
	existing.Available = true
	existing.Status = node.Status
	existing.Load = node.Load
	existing.SuccessRate = node.SuccessRate
}

// GetFastestNodes 获取最快的N个节点
func (d *Discovery) GetFastestNodes(n int) []*Node {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]*Node, 0, len(d.nodes))
	for _, node := range d.nodes {
		if node.Available && node.Status == NodeStatusOnline {
			nodes = append(nodes, node)
		}
	}

	// 根据综合评分排序
	sort.Slice(nodes, func(i, j int) bool {
		scoreI := d.calculateNodeScore(nodes[i])
		scoreJ := d.calculateNodeScore(nodes[j])
		return scoreI > scoreJ
	})

	if len(nodes) > n {
		nodes = nodes[:n]
	}
	return nodes
}

// calculateNodeScore 计算节点评分
func (d *Discovery) calculateNodeScore(node *Node) float64 {
	// 速度权重 0.4，延迟权重 0.2，负载权重 0.2，成功率权重 0.2
	speedScore := node.Speed / 100.0 * 0.4  // 假设100MB/s为满分
	latencyScore := (1.0 - float64(node.Latency)/1000.0) * 0.2  // 延迟1000ms为0分
	loadScore := (1.0 - node.Load) * 0.2
	successScore := node.SuccessRate * 0.2

	return speedScore + latencyScore + loadScore + successScore
}

// UpdateNodeMetrics 更新节点性能指标
func (d *Discovery) UpdateNodeMetrics(nodeID string, speed float64, latency int64, successRate float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if node, exists := d.nodes[nodeID]; exists {
		node.Speed = speed
		node.Latency = latency
		node.SuccessRate = successRate
		node.LastSeen = time.Now()
	}
}

// SetNodeStatus 设置节点状态
func (d *Discovery) SetNodeStatus(nodeID string, status NodeStatus, load float64) {
	d.eventCh <- NodeEvent{
		Type: "status_change",
		Node: &Node{
			ID:     nodeID,
			Status: status,
			Load:   load,
		},
		Time: time.Now(),
	}
}

// Stop 停止发现服务
func (d *Discovery) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
}

// updateLoop 定期更新节点状态
func (d *Discovery) updateLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.cleanupNodes()
		}
	}
}

// cleanupNodes 清理过期节点
func (d *Discovery) cleanupNodes() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for id, node := range d.nodes {
		if now.Sub(node.LastSeen) > 30*time.Second {
			node.Available = false
		}
		if now.Sub(node.LastSeen) > 5*time.Minute {
			delete(d.nodes, id)
		}
	}
}

// AddNode 添加或更新节点
func (d *Discovery) AddNode(node *Node) {
	d.mu.Lock()
	defer d.mu.Unlock()

	node.LastSeen = time.Now()
	d.nodes[node.ID] = node
}

// GetNodes 获取所有可用节点
func (d *Discovery) GetNodes() []*Node {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]*Node, 0, len(d.nodes))
	for _, node := range d.nodes {
		if node.Available {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// generateNodeID 生成节点ID
func generateNodeID(addr string) string {
	// TODO: 实现更好的ID生成算法
	return addr
} 