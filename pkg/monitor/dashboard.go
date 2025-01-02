package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MetricType 指标类型
type MetricType string

const (
	MetricSpeed      MetricType = "speed"
	MetricLatency    MetricType = "latency"
	MetricLossRate   MetricType = "loss_rate"
	MetricCongestion MetricType = "congestion"
)

// MetricPoint 指标数据点
type MetricPoint struct {
	Timestamp time.Time
	Value     float64
}

// NodeMetrics 节点性能指标
type NodeMetrics struct {
	NodeID       string
	Speed       float64  // MB/s
	Latency     int64   // ms
	LossRate    float64 // 0-1
	Congestion  float64 // 0-1
	ActiveTasks int
}

// Dashboard 性能监控面板
type Dashboard struct {
	mu            sync.RWMutex
	nodeMetrics   map[string]*NodeMetrics
	metricHistory map[string]map[MetricType][]MetricPoint
	maxHistory    int
	updateChan    chan MetricUpdate
	server        *http.Server
}

// MetricUpdate 指标更新
type MetricUpdate struct {
	NodeID string
	Type   MetricType
	Value  float64
}

// NewDashboard 创建新的监控面板
func NewDashboard(port int) *Dashboard {
	d := &Dashboard{
		nodeMetrics:   make(map[string]*NodeMetrics),
		metricHistory: make(map[string]map[MetricType][]MetricPoint),
		maxHistory:    1000,
		updateChan:    make(chan MetricUpdate, 1000),
	}

	// 设置HTTP服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", d.handleMetrics)
	mux.HandleFunc("/nodes", d.handleNodes)
	mux.HandleFunc("/history", d.handleHistory)

	d.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return d
}

// Start 启动监控面板
func (d *Dashboard) Start() error {
	// 启动指标更新处理
	go d.processUpdates()

	// 启动HTTP服务器
	return d.server.ListenAndServe()
}

// Stop 停止监控面板
func (d *Dashboard) Stop() error {
	close(d.updateChan)
	return d.server.Close()
}

// UpdateMetric 更新性能指标
func (d *Dashboard) UpdateMetric(nodeID string, metricType MetricType, value float64) {
	d.updateChan <- MetricUpdate{
		NodeID: nodeID,
		Type:   metricType,
		Value:  value,
	}
}

// processUpdates 处理指标更新
func (d *Dashboard) processUpdates() {
	for update := range d.updateChan {
		d.mu.Lock()
		
		// 更新当前指标
		metrics, exists := d.nodeMetrics[update.NodeID]
		if !exists {
			metrics = &NodeMetrics{NodeID: update.NodeID}
			d.nodeMetrics[update.NodeID] = metrics
		}

		switch update.Type {
		case MetricSpeed:
			metrics.Speed = update.Value
		case MetricLatency:
			metrics.Latency = int64(update.Value)
		case MetricLossRate:
			metrics.LossRate = update.Value
		case MetricCongestion:
			metrics.Congestion = update.Value
		}

		// 更新历史记录
		if _, exists := d.metricHistory[update.NodeID]; !exists {
			d.metricHistory[update.NodeID] = make(map[MetricType][]MetricPoint)
		}

		history := d.metricHistory[update.NodeID][update.Type]
		point := MetricPoint{
			Timestamp: time.Now(),
			Value:     update.Value,
		}

		// 保持历史记录在最大长度以内
		if len(history) >= d.maxHistory {
			history = history[1:]
		}
		history = append(history, point)
		d.metricHistory[update.NodeID][update.Type] = history

		d.mu.Unlock()
	}
}

// handleMetrics 处理指标查询请求
func (d *Dashboard) handleMetrics(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.nodeMetrics)
}

// handleNodes 处理节点列表请求
func (d *Dashboard) handleNodes(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]string, 0, len(d.nodeMetrics))
	for nodeID := range d.nodeMetrics {
		nodes = append(nodes, nodeID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// handleHistory 处理历史数据请求
func (d *Dashboard) handleHistory(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node")
	metricType := MetricType(r.URL.Query().Get("metric"))

	d.mu.RLock()
	defer d.mu.RUnlock()

	if nodeHistory, exists := d.metricHistory[nodeID]; exists {
		if history, exists := nodeHistory[metricType]; exists {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(history)
			return
		}
	}

	http.Error(w, "未找到数据", http.StatusNotFound)
}

// GetNodeMetrics 获取节点性能指标
func (d *Dashboard) GetNodeMetrics(nodeID string) *NodeMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if metrics, exists := d.nodeMetrics[nodeID]; exists {
		return metrics
	}
	return nil
}

// GetMetricHistory 获取指标历史数据
func (d *Dashboard) GetMetricHistory(nodeID string, metricType MetricType) []MetricPoint {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if nodeHistory, exists := d.metricHistory[nodeID]; exists {
		return nodeHistory[metricType]
	}
	return nil
} 