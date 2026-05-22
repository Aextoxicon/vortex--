package main

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Metrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.JSON(http.StatusOK, gin.H{
		"goroutines": runtime.NumGoroutine(),
		"memory": gin.H{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"heap_objects":   m.HeapObjects,
		},
		"gc": gin.H{
			"num_gc":       m.NumGC,
			"pause_total_ms": float64(m.PauseTotalNs) / 1e6,
		},
		"cpu": gin.H{
			"num_cpu": runtime.NumCPU(),
		},
	})
}

func SetupMetricsRoute(r *gin.Engine, h *Handler) {
	r.GET("/metrics", h.Metrics)
}

func PrintStartupInfo() {
	fmt.Printf("=== Vortex 启动信息 ===\n")
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
