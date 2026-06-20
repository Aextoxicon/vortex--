package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/process"
)

var startTime = time.Now()

// Metrics 获取服务指标
// @Summary      获取服务指标
// @Description  获取服务的运行指标，包括进程信息、内存使用、线程数等
// @Tags         metrics
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "服务指标"
// @Router       /metrics [get]
func (h *Handler) Metrics(c *gin.Context) {
	pid := os.Getpid()
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"pid":     pid,
			"threads": runtime.NumGoroutine(),
		})
		return
	}

	memInfo, err := p.MemoryInfo()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"pid":     pid,
			"threads": runtime.NumGoroutine(),
		})
		return
	}

	numThreads, err := p.NumThreads()
	if err != nil {
		numThreads = int32(runtime.NumGoroutine())
	}

	c.JSON(http.StatusOK, gin.H{
		"pid":       pid,
		"threads":   numThreads,
		"go_thread": int32(runtime.NumGoroutine()),
		"memory": gin.H{
			"rss":       memInfo.RSS,
			"rss_human": formatBytes(memInfo.RSS),
			"vms":       memInfo.VMS,
			"vms_human": formatBytes(memInfo.VMS),
		},
		"uptime_ms": time.Since(startTime).Milliseconds(),
	})
}

func formatBytes(bytes uint64) string {
	switch {
	case bytes > 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(bytes)/1024/1024/1024)
	case bytes > 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	default:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
}

func PrintStartupInfo() {
	fmt.Printf("=== Vortex 启动信息 ===\n")
	fmt.Printf("版本: %s\n", GetVersion())
	fmt.Printf("CPU 核心: %d\n", runtime.NumCPU())
	fmt.Printf("GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("Go 版本: %s\n", runtime.Version())
	fmt.Printf("\n")
	fmt.Printf("建议配置:\n")

	cpu := runtime.NumCPU()
	if cpu <= 2 {
		fmt.Printf("  - BCRYPT_COST=8 (当前 CPU 核心较少)\n")
		fmt.Printf("  - DB_MAX_OPEN_CONNS=20\n")
		fmt.Printf("  - DB_MAX_IDLE_CONNS=10\n")
	} else if cpu <= 4 {
		fmt.Printf("  - BCRYPT_COST=9\n")
		fmt.Printf("  - DB_MAX_OPEN_CONNS=30\n")
		fmt.Printf("  - DB_MAX_IDLE_CONNS=15\n")
	} else {
		fmt.Printf("  - BCRYPT_COST=10\n")
		fmt.Printf("  - DB_MAX_OPEN_CONNS=50\n")
		fmt.Printf("  - DB_MAX_IDLE_CONNS=20\n")
	}
	fmt.Printf("\n")
}
