package transfer

import (
	"context"
	"sort"
	"sync"
	"time"
)

// TaskPriority 任务优先级
type TaskPriority int

const (
	PriorityLow TaskPriority = iota
	PriorityNormal
	PriorityHigh
)

// TransferTask 传输任务
type TransferTask struct {
	ID       string
	Priority TaskPriority
	Chunks   []uint32    // 需要传输的分片ID列表
	Nodes    []string    // 可用节点ID列表
	Created  time.Time
}

// TaskResult 任务结果
type TaskResult struct {
	TaskID    string
	ChunkID   uint32
	NodeID    string
	Success   bool
	Speed     float64
	Error     error
}

// Scheduler 传输调度器
type Scheduler struct {
	tasks    map[string]*TransferTask
	results  chan TaskResult
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc

	// 性能统计
	nodeStats map[string]*NodeStats
	statsMu   sync.RWMutex
}

// NodeStats 节点统计信息
type NodeStats struct {
	SuccessCount   int64
	FailureCount   int64
	TotalBytes     int64
	AverageSpeed   float64
	LastUpdateTime time.Time
}

// NewScheduler 创建新的调度器
func NewScheduler(ctx context.Context) *Scheduler {
	ctx, cancel := context.WithCancel(ctx)
	return &Scheduler{
		tasks:     make(map[string]*TransferTask),
		results:   make(chan TaskResult, 1000),
		ctx:       ctx,
		cancel:    cancel,
		nodeStats: make(map[string]*NodeStats),
	}
}

// AddTask 添加传输任务
func (s *Scheduler) AddTask(task *TransferTask) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task.Created = time.Now()
	s.tasks[task.ID] = task
}

// Schedule 执行调度
func (s *Scheduler) Schedule() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			task := s.getNextTask()
			if task == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			s.scheduleTasks(task)
		}
	}
}

// getNextTask 获取下一个要处理的任务
func (s *Scheduler) getNextTask() *TransferTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.tasks) == 0 {
		return nil
	}

	// 按优先级和创建时间排序
	tasks := make([]*TransferTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority > tasks[j].Priority
		}
		return tasks[i].Created.Before(tasks[j].Created)
	})

	return tasks[0]
}

// scheduleTasks 为任务分配节点
func (s *Scheduler) scheduleTasks(task *TransferTask) {
	// 获取节点性能统计
	nodeStats := s.getNodeStats(task.Nodes)

	// 按性能排序节点
	sort.Slice(task.Nodes, func(i, j int) bool {
		statI := nodeStats[task.Nodes[i]]
		statJ := nodeStats[task.Nodes[j]]
		if statI == nil || statJ == nil {
			return false
		}
		// 优先考虑速度,其次是成功率
		speedI := statI.AverageSpeed
		speedJ := statJ.AverageSpeed
		if speedI != speedJ {
			return speedI > speedJ
		}
		successRateI := float64(statI.SuccessCount) / float64(statI.SuccessCount + statI.FailureCount)
		successRateJ := float64(statJ.SuccessCount) / float64(statJ.SuccessCount + statJ.FailureCount)
		return successRateI > successRateJ
	})

	// 分配分片到节点
	chunkCount := len(task.Chunks)
	nodeCount := len(task.Nodes)
	if nodeCount == 0 || chunkCount == 0 {
		return
	}

	// 动态计算每个节点应该处理的分片数量
	chunksPerNode := make(map[string]int)
	totalWeight := 0.0
	for _, nodeID := range task.Nodes {
		if stats := nodeStats[nodeID]; stats != nil {
			// 根据速度和成功率计算权重
			successRate := float64(stats.SuccessCount) / float64(stats.SuccessCount + stats.FailureCount)
			if successRate < 0.5 {
				continue // 跳过成功率过低的节点
			}
			weight := stats.AverageSpeed * successRate
			if weight < 1.0 {
				weight = 1.0
			}
			totalWeight += weight
			chunksPerNode[nodeID] = int((weight / totalWeight) * float64(chunkCount))
		}
	}

	// 分配分片,优先分配给高性能节点
	chunkIndex := 0
	for nodeID, count := range chunksPerNode {
		end := chunkIndex + count
		if end > chunkCount {
			end = chunkCount
		}
		if chunkIndex >= end {
			continue
		}

		chunks := task.Chunks[chunkIndex:end]
		s.assignChunksToNode(task.ID, nodeID, chunks)
		chunkIndex = end
	}

	// 如果还有未分配的分片,分配给剩余节点
	if chunkIndex < chunkCount {
		remainingChunks := task.Chunks[chunkIndex:]
		for i, chunk := range remainingChunks {
			nodeID := task.Nodes[i%len(task.Nodes)]
			s.assignChunksToNode(task.ID, nodeID, []uint32{chunk})
		}
	}
}

// assignChunksToNode 将分片分配给节点
func (s *Scheduler) assignChunksToNode(taskID string, nodeID string, chunks []uint32) {
	// TODO: 实际发送传输请求到节点
	// 这里应该调用网络层发送请求
}

// HandleResult 处理传输结果
func (s *Scheduler) HandleResult(result TaskResult) {
	s.updateNodeStats(result)

	if !result.Success {
		// 重新调度失败的分片
		s.mu.Lock()
		if task, exists := s.tasks[result.TaskID]; exists {
			task.Chunks = append(task.Chunks, result.ChunkID)
		}
		s.mu.Unlock()
	}
}

// updateNodeStats 更新节点统计信息
func (s *Scheduler) updateNodeStats(result TaskResult) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	stats, exists := s.nodeStats[result.NodeID]
	if !exists {
		stats = &NodeStats{}
		s.nodeStats[result.NodeID] = stats
	}

	if result.Success {
		stats.SuccessCount++
	} else {
		stats.FailureCount++
	}

	// 使用指数移动平均更新速度
	alpha := 0.3 // 平滑因子
	stats.AverageSpeed = stats.AverageSpeed*(1-alpha) + result.Speed*alpha
	stats.LastUpdateTime = time.Now()
}

// getNodeStats 获取节点统计信息
func (s *Scheduler) getNodeStats(nodeIDs []string) map[string]*NodeStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	stats := make(map[string]*NodeStats)
	for _, nodeID := range nodeIDs {
		if nodeStat, exists := s.nodeStats[nodeID]; exists {
			stats[nodeID] = nodeStat
		}
	}
	return stats
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
} 