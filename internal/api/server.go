package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/timholm/market-scanner/internal/config"
	"github.com/timholm/market-scanner/internal/db"
	"github.com/timholm/market-scanner/internal/scanner"
)

// Server is the HTTP API for market-scanner.
type Server struct {
	cfg     *config.Config
	db      *db.DB
	scanner *scanner.Scanner
	router  *gin.Engine
}

// New creates a new API server.
func New(cfg *config.Config, database *db.DB, sc *scanner.Scanner) *Server {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{
		cfg:     cfg,
		db:      database,
		scanner: sc,
		router:  gin.New(),
	}
	s.router.Use(gin.Recovery())
	s.routes()
	return s
}

func (s *Server) routes() {
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/scan/:name", s.handleScan)
	s.router.POST("/scan", s.handleScanPost)
	s.router.GET("/reports", s.handleReports)
	s.router.GET("/reports/:name", s.handleReport)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.router.Run(s.cfg.ListenAddr)
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "market-scanner",
	})
}

func (s *Server) handleScan(c *gin.Context) {
	name := c.Param("name")
	problem := c.Query("problem")

	result, err := s.scanner.Scan(c.Request.Context(), name, problem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Persist the result.
	if saveErr := s.db.SaveScan(name, problem, result.NoveltyScore, result.Recommendation, result); saveErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scan succeeded but failed to save: " + saveErr.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ScanRequest is the POST body for /scan.
type ScanRequest struct {
	Name    string `json:"name" binding:"required"`
	Problem string `json:"problem"`
}

func (s *Server) handleScanPost(c *gin.Context) {
	var req ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := s.scanner.Scan(c.Request.Context(), req.Name, req.Problem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if saveErr := s.db.SaveScan(req.Name, req.Problem, result.NoveltyScore, result.Recommendation, result); saveErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "scan succeeded but failed to save: " + saveErr.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) handleReports(c *gin.Context) {
	reports, err := s.db.ListScans(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reports)
}

func (s *Server) handleReport(c *gin.Context) {
	name := c.Param("name")
	report, err := s.db.GetScan(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if report == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no scan found for " + name})
		return
	}
	c.JSON(http.StatusOK, report)
}
