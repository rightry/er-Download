package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// 全局状态
var (
	downloads     = make(map[string]*Download)
	nodes        = make(map[string]*Node)
	metrics      = &Metrics{}
	downloadLock sync.RWMutex
	nodeLock     sync.RWMutex
	metricsLock  sync.RWMutex
)

// Download 表示一个下载任务
type Download struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	FileName     string    `json:"fileName"`
	Size         int64     `json:"size"`
	Progress     float64   `json:"progress"`
	Speed        int64     `json:"speed"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Priority     string    `json:"priority"`
	SavePath     string    `json:"savePath"`
	Connections  int       `json:"connections"`
	RemainingTime int64    `json:"remainingTime"`
}

// Node 表示一个P3P节点
type Node struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"lastSeen"`
	Speed     int64     `json:"speed"`
	Latency   int64     `json:"latency"`
	Available bool      `json:"available"`
}

// Metrics 表示系统性能指标
type Metrics struct {
	Network struct {
		DownloadSpeed int64 `json:"downloadSpeed"`
		UploadSpeed   int64 `json:"uploadSpeed"`
		Connections   int   `json:"connections"`
		Latency       int64 `json:"latency"`
	} `json:"network"`
	System struct {
		CPUUsage    float64 `json:"cpuUsage"`
		MemoryUsage float64 `json:"memoryUsage"`
		DiskUsage   float64 `json:"diskUsage"`
	} `json:"system"`
	P2P struct {
		ActiveNodes int     `json:"activeNodes"`
		Seeders     int     `json:"seeders"`
		Leechers    int     `json:"leechers"`
		ShareRatio  float64 `json:"shareRatio"`
	} `json:"p2p"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源的WebSocket连接
	},
}

// CORS中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	r := mux.NewRouter()

	// 添加CORS中间件
	r.Use(corsMiddleware)

	// API路由
	r.HandleFunc("/api/downloads", getDownloads).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/downloads", createDownload).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/downloads/{id}", getDownload).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/downloads/{id}/pause", pauseDownload).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/downloads/{id}/resume", resumeDownload).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/downloads/{id}/cancel", cancelDownload).Methods("POST", "OPTIONS")

	r.HandleFunc("/api/nodes", getNodes).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/nodes/{id}", getNode).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/metrics", getMetrics).Methods("GET", "OPTIONS")
	r.HandleFunc("/ws/metrics", handleMetricsWebSocket)

	// 启动模拟数据更新
	go simulateMetricsUpdates()

	// 启动服务器
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// API处理函数
func getDownloads(w http.ResponseWriter, r *http.Request) {
	downloadLock.RLock()
	defer downloadLock.RUnlock()

	downloadList := make([]*Download, 0, len(downloads))
	for _, d := range downloads {
		downloadList = append(downloadList, d)
	}

	json.NewEncoder(w).Encode(downloadList)
}

func createDownload(w http.ResponseWriter, r *http.Request) {
	var download Download
	if err := json.NewDecoder(r.Body).Decode(&download); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 确保保存路径存在
	if err := os.MkdirAll(download.SavePath, 0755); err != nil {
		http.Error(w, "创建下载目录失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	download.ID = generateID()
	download.CreatedAt = time.Now()
	download.Status = "downloading"

	downloadLock.Lock()
	downloads[download.ID] = &download
	downloadLock.Unlock()

	// 启动模拟下载进度更新
	go simulateDownloadProgress(download.ID)

	json.NewEncoder(w).Encode(download)
}

func getDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	downloadLock.RLock()
	download, ok := downloads[id]
	downloadLock.RUnlock()

	if !ok {
		http.Error(w, "Download not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(download)
}

func pauseDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	downloadLock.Lock()
	download, ok := downloads[id]
	if ok {
		download.Status = "paused"
	}
	downloadLock.Unlock()

	if !ok {
		http.Error(w, "Download not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(download)
}

func resumeDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	downloadLock.Lock()
	download, ok := downloads[id]
	if ok {
		download.Status = "downloading"
	}
	downloadLock.Unlock()

	if !ok {
		http.Error(w, "Download not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(download)
}

func cancelDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	downloadLock.Lock()
	delete(downloads, id)
	downloadLock.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func getNodes(w http.ResponseWriter, r *http.Request) {
	nodeLock.RLock()
	defer nodeLock.RUnlock()

	nodeList := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}

	json.NewEncoder(w).Encode(nodeList)
}

func getNode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	nodeLock.RLock()
	node, ok := nodes[id]
	nodeLock.RUnlock()

	if !ok {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(node)
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
	metricsLock.RLock()
	defer metricsLock.RUnlock()

	json.NewEncoder(w).Encode(metrics)
}

func handleMetricsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	// 每秒发送一次性能指标数据
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metricsLock.RLock()
		err := conn.WriteJSON(metrics)
		metricsLock.RUnlock()

		if err != nil {
			log.Println(err)
			return
		}
	}
}

// 辅助函数
func generateID() string {
	return time.Now().Format("20060102150405")
}

// 模拟数据更新
func simulateMetricsUpdates() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metricsLock.Lock()
		// 模拟更高的网络指标
		metrics.Network.DownloadSpeed = 1024 * 1024 * 50 // 50MB/s
		metrics.Network.UploadSpeed = 1024 * 1024 * 25   // 25MB/s
		metrics.Network.Connections = 64
		metrics.Network.Latency = 20 // 20ms

		// 模拟系统指标
		metrics.System.CPUUsage = 50  // 50%
		metrics.System.MemoryUsage = 60 // 60%
		metrics.System.DiskUsage = 70   // 70%

		// 模拟P2P指标
		metrics.P2P.ActiveNodes = 200
		metrics.P2P.Seeders = 100
		metrics.P2P.Leechers = 50
		metrics.P2P.ShareRatio = 2.0
		metricsLock.Unlock()
	}
}

func simulateDownloadProgress(id string) {
	ticker := time.NewTicker(100 * time.Millisecond) // 更新频率提高到100ms
	defer ticker.Stop()

	for range ticker.C {
		downloadLock.Lock()
		download, ok := downloads[id]
		if !ok {
			downloadLock.Unlock()
			return
		}

		if download.Status != "downloading" {
			downloadLock.Unlock()
			continue
		}

		// 模拟更快的下载进度
		download.Progress += 5 // 每100ms增加5%
		download.Speed = 1024 * 1024 * 50 // 50MB/s
		download.RemainingTime = int64((100 - download.Progress) * float64(download.Size) / float64(download.Speed))

		if download.Progress >= 100 {
			download.Status = "completed"
			now := time.Now()
			download.CompletedAt = &now
			downloadLock.Unlock()
			return
		}

		downloadLock.Unlock()
	}
} 