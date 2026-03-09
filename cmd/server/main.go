package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/carev01/adhive/internal/config"
	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/middleware"
	"github.com/carev01/adhive/internal/migrations"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/service"
	"github.com/carev01/adhive/internal/worker"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Version info - set via ldflags during build
var (
	version    = "latest"
	commit     = "unknown"
	buildDate  = "unknown"
)

func init() {
	// Allow environment variables to override build-time values
	if v := os.Getenv("APP_VERSION"); v != "" {
		version = v
	}
	if c := os.Getenv("APP_COMMIT"); c != "" {
		commit = c
	}
	if b := os.Getenv("APP_BUILD_DATE"); b != "" {
		buildDate = b
	}
}

// getIntEnv returns an integer from environment or default
func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

// getDurationEnv returns a duration from environment or default
func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

// getCORSOrigins returns allowed CORS origins from environment variable.
// Development defaults: localhost:5173, localhost:3000, localhost:8080
// Production default: empty (secure by default)
func getCORSOrigins() []string {
	envOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	
	if envOrigins == "" {
		// Check if running in development mode (no DB_PASSWORD set = local dev)
		dbPassword := os.Getenv("DB_PASSWORD")
		if dbPassword == "" {
			// Development mode - use default localhost origins
			return []string{
				"http://localhost:5173",
				"http://localhost:3000",
				"http://localhost:8080",
				"http://127.0.0.1:5173",
				"http://127.0.0.1:3000",
				"http://127.0.0.1:8080",
			}
		}
		// Production - require explicit configuration
		return []string{}
	}
	
	// Parse comma-separated origins
	origins := strings.Split(envOrigins, ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}
	return origins
}

func main() {
	// Database setup - uses unified DATA_DIR or DB_PATH (backwards compatible)
	dbPath := config.GetDBPath()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Configure SQLite for optimal performance
	if err := configureSQLite(db); err != nil {
		log.Printf("Warning: Failed to configure SQLite PRAGMAs: %v", err)
	}

	// Auto-migrate tables
	err = db.AutoMigrate(
		&model.User{},
		&model.Session{},
		&model.CatalogEntry{},
		&model.Tag{},
		&model.EntryTag{},
		&model.Interaction{},
		&model.ArchiveRevision{},
		&model.ArchiveAsset{},
		&model.ThumbnailCandidate{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Run database migrations (indexes, FTS, etc.)
	if err := migrations.V1CreateIndexes(db); err != nil {
		log.Printf("Warning: Failed to run index migrations: %v", err)
	}

	// Create FTS5 for full-text search
	if err := migrations.V2CreateFTS5(db); err != nil {
		log.Printf("Warning: Failed to create FTS5: %v", err)
	} else {
		// Repopulate FTS from existing data
		migrations.RepopulateFTS5(db)
	}

	// Repositories
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	entryRepo := repository.NewEntryRepository(db)
	tagRepo := repository.NewTagRepository(db)
	interactionRepo := repository.NewInteractionRepository(db)
	archiveRevisionRepo := repository.NewArchiveRevisionRepository(db)
	archiveAssetRepo := repository.NewArchiveAssetRepository(db)
	thumbnailCandidateRepo := repository.NewThumbnailCandidateRepository(db)

	// File storage
	storageConfig := config.DefaultStorageConfig()
	fileService := service.NewFileService(storageConfig)
	fileHandler := handler.NewFileHandlerWithRevisionRepo(fileService, entryRepo, thumbnailCandidateRepo, archiveRevisionRepo)
	thumbnailHandler := handler.NewThumbnailHandler(
		entryRepo,
		archiveAssetRepo,
		archiveRevisionRepo,
		thumbnailCandidateRepo,
		service.NewThumbnailService(storageConfig.BaseDir),
		storageConfig.BaseDir,
	)

	// Archive Worker (depends on storageConfig)
	dataDir := storageConfig.BaseDir
	archiveWorker := worker.NewArchiveWorker(entryRepo, dataDir)
	archiveWorker.SetArchivePersistence(db)
	archiveWorker.SetThumbnailCandidateRepository(thumbnailCandidateRepo)

	// Handlers
	authHandler := handler.NewAuthHandler(userRepo, sessionRepo)
	entryHandler := handler.NewEntryHandlerWithStorage(entryRepo, tagRepo, storageConfig)
	entryHandler.SetArchiveWorker(archiveWorker)
	tagHandler := handler.NewTagHandler(tagRepo)
	interactionHandler := handler.NewInteractionHandler(interactionRepo, entryRepo)
	archiveOpsHandler := handler.NewArchiveOpsHandler(archiveRevisionRepo, archiveAssetRepo, entryRepo, archiveWorker, storageConfig)

	// Initialize storage directories
	if err := fileHandler.InitStorage(); err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Middleware
	authMiddleware := middleware.NewAuthMiddleware(sessionRepo)

	// Router setup
	r := setupRouter(authHandler, entryHandler, tagHandler, interactionHandler, archiveOpsHandler, authMiddleware, fileHandler, thumbnailHandler)

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start archive worker
	archiveWorker.Start(ctx)

	// Start polling pending entries in background
	go archiveWorker.PollPendingEntries(ctx, 30*time.Second)

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down server...")
		cancel() // Stop workers
		archiveWorker.Stop()
		time.Sleep(1 * time.Second) // Give workers time to finish
		log.Println("Server stopped")
	}()

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// configureSQLite sets up SQLite performance PRAGMAs
func configureSQLite(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA cache_size=-64000;", // 64MB
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA temp_store=MEMORY;",
		"PRAGMA mmap_size=268435456;", // 256MB
	}

	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			log.Printf("Warning: Failed to set PRAGMA %s: %v", pragma, err)
		} else {
			log.Printf("SQLite PRAGMA applied: %s", pragma)
		}
	}

	return nil
}

// setupRouter configures all routes
func setupRouter(authHandler *handler.AuthHandler, entryHandler *handler.EntryHandler, tagHandler *handler.TagHandler, interactionHandler *handler.InteractionHandler, archiveOpsHandler *handler.ArchiveOpsHandler, authMiddleware *middleware.AuthMiddleware, fileHandler *handler.FileHandler, thumbnailHandler *handler.ThumbnailHandler) *gin.Engine {
	r := gin.Default()

	// Enable gzip compression for responses > 1KB
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	// Global middleware
	r.Use(middleware.Logger())
	r.Use(middleware.Recover())
	r.Use(middleware.RawPathTraversalGuard())
	r.Use(middleware.InputSanitizer())
	r.Use(middleware.SecurityHeaders())
	
	// Rate limiting: configurable via environment variables
	// Default: 100 requests per minute, can be adjusted with RATE_LIMIT and RATE_LIMIT_WINDOW
	rateLimit := getIntEnv("RATE_LIMIT", 100)
	rateLimitWindow := getDurationEnv("RATE_LIMIT_WINDOW", time.Minute)
	r.Use(middleware.RateLimit(rateLimit, rateLimitWindow))
	
	// Request size limit: 10MB for all requests
	r.Use(middleware.RequestSizeLimit(10 * 1024 * 1024))
	
	// Strict CORS (origins from environment, defaults to localhost in dev)
	r.Use(middleware.StrictCORS(getCORSOrigins()))

	// Health check - public
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Version info - public
	r.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version":    version,
			"commit":     commit,
			"build_date": buildDate,
		})
	})

	// API v1 routes
	api := r.Group("/api/v1")
	{
		// Public auth routes (no middleware)
		auth := api.Group("/auth")
		{
			auth.POST("/register", middleware.AuthRegisterRateLimit(), authHandler.Register)
			auth.POST("/login", middleware.AuthLoginRateLimit(), authHandler.Login)
			auth.GET("/csrf-token", middleware.CSRFTokenHandler())
		}

		// Protected auth routes (auth required)
		protected := api.Group("/auth")
		protected.Use(authMiddleware.Authenticate())
		protected.Use(middleware.CSRF())
		{
			protected.POST("/logout", authHandler.Logout)
			protected.GET("/me", authHandler.Me)
		}

		// Protected entries routes (auth required)
		entries := api.Group("/entries")
		entries.Use(authMiddleware.Authenticate())
		entries.Use(middleware.CSRF())
		{
			entries.GET("", entryHandler.List)
			entries.GET("/sources", entryHandler.Sources)
			entries.GET("/locations", entryHandler.Locations)
			entries.POST("/bulk/tag", entryHandler.BulkTag)
			entries.POST("/bulk/delete", entryHandler.BulkDelete)
			entries.POST("/bulk/archive", entryHandler.BulkArchive)
			entries.POST("", entryHandler.Create)
			entries.POST("/random", entryHandler.Random)
			entries.GET("/:id", entryHandler.Get)
			entries.PUT("/:id", entryHandler.Update)
			entries.DELETE("/:id", entryHandler.Delete)

			// Tag associations
			entries.POST("/:id/tags", tagHandler.AddEntryTag)
			entries.DELETE("/:id/tags/:tagId", tagHandler.RemoveEntryTag)

			// Interactions
			entries.GET("/:id/interaction", interactionHandler.Get)
			entries.PUT("/:id/interaction", interactionHandler.Upsert)
			entries.DELETE("/:id/interaction", interactionHandler.Delete)

			// Archive operations
			entries.GET("/:id/archive/revisions", archiveOpsHandler.ListRevisions)
			entries.POST("/:id/archive/refresh", archiveOpsHandler.Refresh)
			entries.DELETE("/:id/archive/revisions/:revisionId", archiveOpsHandler.DeleteRevision)
		}

		// Protected tags routes (auth required)
		tags := api.Group("/tags")
		tags.Use(authMiddleware.Authenticate())
		tags.Use(middleware.CSRF())
		{
			tags.GET("", tagHandler.List)
			tags.POST("", tagHandler.Create)
			tags.GET("/:id", tagHandler.Get)
			tags.PUT("/:id", tagHandler.Update)
			tags.DELETE("/:id", tagHandler.Delete)
		}

		archive := api.Group("/archive")
		archive.Use(authMiddleware.Authenticate())
		archive.Use(middleware.CSRF())
		{
			archive.GET("/metrics", archiveOpsHandler.Metrics)
		}

		// Protected files routes (auth required)
		files := api.Group("/files")
		files.Use(authMiddleware.Authenticate())
		files.Use(middleware.CSRF())
		{
			// Thumbnail candidates (before thumbUUID group to avoid UUID validation)
			files.GET("/thumbnails/:entryID/candidates", thumbnailHandler.ListCandidates)
			files.POST("/thumbnails/:entryID/select", thumbnailHandler.SelectCandidate)

			// Legacy archives endpoints
			files.POST("/archives/:entryID", fileHandler.UploadArchive)
			archivesUUID := files.Group("/archives/:entryID")
			archivesUUID.Use(middleware.RequireUUIDParam("entryID"))
			archivesUUID.GET("", fileHandler.ListArchives)
			archivesUUID.DELETE("", fileHandler.DeleteArchive)

			// Revision-aware secure archive serving
			files.GET("/archive/:entryID", fileHandler.GetArchive)
			files.GET("/archive/:entryID/:revisionID/*path", fileHandler.GetArchive)

			// Thumbnails with UUID validation
			thumbUUID := files.Group("/thumbnails/:entryID")
			thumbUUID.Use(middleware.RequireUUIDParam("entryID"))
			thumbUUID.POST("", fileHandler.UploadThumbnail)
			thumbUUID.GET("", fileHandler.GetThumbnail)
			thumbUUID.DELETE("", fileHandler.DeleteThumbnail)
			files.GET("/thumbnails/raw/*path", fileHandler.GetRawThumbnail)

			// Utilities
			files.GET("/stats", fileHandler.GetStorageStats)
		}
	}

	return r
}
