package transfer

import (
	"errors"
	"fmt"
	"time"
)

// 错误类型定义
var (
	ErrTimeout        = errors.New("传输超时")
	ErrNodeUnavailable = errors.New("节点不可用")
	ErrTooManyRetries = errors.New("重试次数过多")
	ErrChunkCorrupted = errors.New("分片数据损坏")
	ErrNetworkFailure = errors.New("网络故障")
)

// TransferError 传输错误
type TransferError struct {
	Err       error
	ChunkID   uint32
	NodeID    string
	Timestamp time.Time
	Retries   int
	Details   string
}

func (e *TransferError) Error() string {
	return fmt.Sprintf("传输错误: %v (分片: %d, 节点: %s, 重试: %d, 时间: %v, 详情: %s)",
		e.Err, e.ChunkID, e.NodeID, e.Retries, e.Timestamp, e.Details)
}

// ErrorHandler 错误处理器
type ErrorHandler struct {
	maxRetries    int
	retryInterval time.Duration
	errorLog      []TransferError
	maxLogSize    int
}

// NewErrorHandler 创建新的错误处理器
func NewErrorHandler(maxRetries int, retryInterval time.Duration) *ErrorHandler {
	return &ErrorHandler{
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
		maxLogSize:    1000,
	}
}

// HandleError 处理传输错误
func (h *ErrorHandler) HandleError(err error, chunkID uint32, nodeID string) (shouldRetry bool, retryAfter time.Duration) {
	transferErr, ok := err.(*TransferError)
	if !ok {
		transferErr = &TransferError{
			Err:       err,
			ChunkID:   chunkID,
			NodeID:    nodeID,
			Timestamp: time.Now(),
		}
	}

	// 记录错误
	h.logError(transferErr)

	// 根据错误类型决定重试策略
	switch {
	case errors.Is(err, ErrTimeout):
		// 超时错误，使用指数退避
		retryAfter = h.calculateBackoff(transferErr.Retries)
		return transferErr.Retries < h.maxRetries, retryAfter

	case errors.Is(err, ErrNodeUnavailable):
		// 节点不可用，立即切换节点
		return true, 0

	case errors.Is(err, ErrChunkCorrupted):
		// 数据损坏，立即重试
		return transferErr.Retries < h.maxRetries, 0

	case errors.Is(err, ErrNetworkFailure):
		// 网络故障，等待后重试
		return transferErr.Retries < h.maxRetries, h.retryInterval

	default:
		// 其他错误，使用标准重试间隔
		return transferErr.Retries < h.maxRetries, h.retryInterval
	}
}

// calculateBackoff 计算指数退避时间
func (h *ErrorHandler) calculateBackoff(retries int) time.Duration {
	if retries <= 0 {
		return h.retryInterval
	}
	backoff := h.retryInterval * time.Duration(1<<uint(retries-1))
	maxBackoff := 30 * time.Second
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}

// logError 记录错误
func (h *ErrorHandler) logError(err *TransferError) {
	if len(h.errorLog) >= h.maxLogSize {
		// 移除最旧的错误记录
		h.errorLog = h.errorLog[1:]
	}
	h.errorLog = append(h.errorLog, *err)
}

// GetErrorStats 获取错误统计
func (h *ErrorHandler) GetErrorStats() map[string]int {
	stats := make(map[string]int)
	for _, err := range h.errorLog {
		errType := err.Err.Error()
		stats[errType]++
	}
	return stats
}

// GetRecentErrors 获取最近的错误记录
func (h *ErrorHandler) GetRecentErrors(n int) []TransferError {
	if n <= 0 || n > len(h.errorLog) {
		n = len(h.errorLog)
	}
	return h.errorLog[len(h.errorLog)-n:]
}

// IsTransientError 判断是否为临时性错误
func IsTransientError(err error) bool {
	return errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrNetworkFailure) ||
		errors.Is(err, ErrChunkCorrupted)
}

// IsFatalError 判断是否为致命错误
func IsFatalError(err error) bool {
	return !IsTransientError(err)
}

// NewTransferError 创建新的传输错误
func NewTransferError(err error, chunkID uint32, nodeID string, details string) *TransferError {
	return &TransferError{
		Err:       err,
		ChunkID:   chunkID,
		NodeID:    nodeID,
		Timestamp: time.Now(),
		Details:   details,
	}
} 