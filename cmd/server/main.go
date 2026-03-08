package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/carev01/adhive/internal/config"
	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/middleware"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/service"
	"github.com/carev01/adhive/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func main() {
	// Database setup
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "ad-catalog.db"
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
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

// setupRouter configures all routes
func setupRouter(authHandler *handler.AuthHandler, entryHandler *handler.EntryHandler, tagHandler *handler.TagHandler, interactionHandler *handler.InteractionHandler, archiveOpsHandler *handler.ArchiveOpsHandler, authMiddleware *middleware.AuthMiddleware, fileHandler *handler.FileHandler, thumbnailHandler *handler.ThumbnailHandler) *gin.Engine {
	r := gin.Default()

	// Global middleware
	r.Use(middleware.Logger())
	r.Use(middleware.Recover())
	r.Use(middleware.RawPathTraversalGuard())
	r.Use(middleware.CORS())

	// Health check - public
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API v1 routes
	api := r.Group("/api/v1")
	{
		// Public auth routes (no middleware)
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		// Protected auth routes (auth required)
		protected := api.Group("/auth")
		protected.Use(authMiddleware.Authenticate())
		{
			protected.POST("/logout", authHandler.Logout)
			protected.GET("/me", authHandler.Me)
		}

		// Protected entries routes (auth required)
		entries := api.Group("/entries")
		entries.Use(authMiddleware.Authenticate())
		{
			entries.GET("", entryHandler.List)
			entries.GET("/sources", entryHandler.Sources)
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
		{
			tags.GET("", tagHandler.List)
			tags.POST("", tagHandler.Create)
			tags.GET("/:id", tagHandler.Get)
			tags.PUT("/:id", tagHandler.Update)
			tags.DELETE("/:id", tagHandler.Delete)
		}

		archive := api.Group("/archive")
		archive.Use(authMiddleware.Authenticate())
		{
			archive.GET("/metrics", archiveOpsHandler.Metrics)
		}

		// Protected files routes (auth required)
		files := api.Group("/files")
		files.Use(authMiddleware.Authenticate())
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
