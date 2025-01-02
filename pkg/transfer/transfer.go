package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"
)

const (
	ChunkSize    = 8 * 1024 * 1024  // 增加到8MB 分片大小
	MaxChunks    = 4096             // 增加最大分片数
	MaxRetries   = 2                // 减少重试次数以加快失败恢复
	AckTimeout   = 100 * time.Millisecond  // 减少确认超时时间
	MaxConcurrentTransfers = 64     // 增加最大并发传输数
)

// TransferStats 传输统计
type TransferStats struct {
	StartTime    time.Time
	EndTime      time.Time
	BytesSent    int64
	BytesReceived int64
	Speed        float64  // MB/s
}

// Chunk 表示文件分片
type Chunk struct {
	ID       uint32
	Data     []byte
	Hash     []byte
	Received bool
	Retries  int
}

// Transfer 管理文件传输
type Transfer struct {
	ID        string
	Size      int64
	Chunks    []*Chunk
	mu        sync.RWMutex
	completed bool
	ctx       context.Context
	cancel    context.CancelFunc
	stats     TransferStats
	workers   int
}

// NewTransfer 创建新的传输任务
func NewTransfer(ctx context.Context, size int64, workers int) (*Transfer, error) {
	if size > int64(ChunkSize)*int64(MaxChunks) {
		return nil, errors.New("文件太大")
	}

	if workers <= 0 {
		workers = MaxConcurrentTransfers
	}

	t := &Transfer{
		ID:      generateTransferID(),
		Size:    size,
		workers: workers,
		stats: TransferStats{
			StartTime: time.Now(),
		},
	}

	numChunks := (size + int64(ChunkSize) - 1) / int64(ChunkSize)
	t.Chunks = make([]*Chunk, numChunks)

	for i := range t.Chunks {
		t.Chunks[i] = &Chunk{
			ID: uint32(i),
		}
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	return t, nil
}

// StartTransfer 开始传输
func (t *Transfer) StartTransfer(reader io.ReaderAt, writer io.WriterAt) error {
	workerChan := make(chan *Chunk, t.workers)
	errChan := make(chan error, 1)
	doneChan := make(chan bool, 1)

	// 启动工作协程
	for i := 0; i < t.workers; i++ {
		go func() {
			for chunk := range workerChan {
				if err := t.processChunk(reader, writer, chunk); err != nil {
					errChan <- err
					return
				}
			}
		}()
	}

	// 分发任务
	go func() {
		for _, chunk := range t.Chunks {
			if !chunk.Received {
				select {
				case workerChan <- chunk:
				case <-t.ctx.Done():
					return
				}
			}
		}
		close(workerChan)
		doneChan <- true
	}()

	// 等待完成或错误
	select {
	case err := <-errChan:
		return err
	case <-doneChan:
		t.stats.EndTime = time.Now()
		t.updateStats()
		return nil
	case <-t.ctx.Done():
		return t.ctx.Err()
	}
}

// processChunk 处理单个分片
func (t *Transfer) processChunk(reader io.ReaderAt, writer io.WriterAt, chunk *Chunk) error {
	if err := t.ReadChunk(reader, chunk.ID); err != nil {
		return err
	}

	if err := t.WriteChunk(writer, chunk); err != nil {
		return err
	}

	t.mu.Lock()
	t.stats.BytesReceived += int64(len(chunk.Data))
	t.mu.Unlock()

	return nil
}

// updateStats 更新传输统计
func (t *Transfer) updateStats() {
	duration := t.stats.EndTime.Sub(t.stats.StartTime).Seconds()
	if duration > 0 {
		t.stats.Speed = float64(t.stats.BytesReceived) / duration / 1024 / 1024
	}
}

// GetStats 获取传输统计
func (t *Transfer) GetStats() TransferStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stats
}

// ReadChunk 读取指定分片
func (t *Transfer) ReadChunk(reader io.ReaderAt, chunkID uint32) error {
	if int(chunkID) >= len(t.Chunks) {
		return errors.New("无效的分片ID")
	}

	chunk := t.Chunks[chunkID]
	offset := int64(chunkID) * int64(ChunkSize)
	
	// 计算实际分片大小
	size := ChunkSize
	if offset+int64(ChunkSize) > t.Size {
		size = int(t.Size - offset)
	}

	// 读取数据
	chunk.Data = make([]byte, size)
	_, err := reader.ReadAt(chunk.Data, offset)
	if err != nil {
		return err
	}

	// 计算哈希
	hash := sha256.New()
	hash.Write(chunk.Data)
	chunk.Hash = hash.Sum(nil)

	return nil
}

// WriteChunk 写入接收到的分片
func (t *Transfer) WriteChunk(writer io.WriterAt, chunk *Chunk) error {
	if int(chunk.ID) >= len(t.Chunks) {
		return errors.New("无效的分片ID")
	}

	// 验证哈希
	hash := sha256.New()
	hash.Write(chunk.Data)
	if !compareHash(hash.Sum(nil), chunk.Hash) {
		return errors.New("分片校验失败")
	}

	// 写入数据
	offset := int64(chunk.ID) * int64(ChunkSize)
	_, err := writer.WriteAt(chunk.Data, offset)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.Chunks[chunk.ID].Received = true
	t.mu.Unlock()

	return nil
}

// IsCompleted 检查是否所有分片都已接收
func (t *Transfer) IsCompleted() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.completed {
		return true
	}

	for _, chunk := range t.Chunks {
		if !chunk.Received {
			return false
		}
	}

	t.completed = true
	return true
}

// GetMissingChunks 获取未接收的分片ID列表
func (t *Transfer) GetMissingChunks() []uint32 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	missing := make([]uint32, 0)
	for _, chunk := range t.Chunks {
		if !chunk.Received {
			missing = append(missing, chunk.ID)
		}
	}
	return missing
}

// Stop 停止传输
func (t *Transfer) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
}

// generateTransferID 生成传输ID
func generateTransferID() string {
	// TODO: 实现更好的ID生成算法
	return time.Now().Format("20060102150405")
}

// compareHash 比较两个哈希值
func compareHash(h1, h2 []byte) bool {
	if len(h1) != len(h2) {
		return false
	}
	for i := range h1 {
		if h1[i] != h2[i] {
			return false
		}
	}
	return true
} 