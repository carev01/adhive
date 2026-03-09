package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/carev01/adhive/internal/importer"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
	"github.com/carev01/adhive/internal/shioriimport"
	"github.com/google/uuid"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// CLI flags
var (
	sqlPath      = flag.String("sql", "", "Path to Shiori MariaDB dump file (required)")
	jsonPath     = flag.String("json", "", "Path to parsed JSON from shiori-parser (optional, alternative to --sql)")
	archivesPath = flag.String("archives", "", "Path to Shiori archive directory (optional)")
	thumbsPath   = flag.String("thumbs", "", "Path to Shiori thumbnails directory (optional)")
	userID       = flag.String("user-id", "", "Target AdHive user UUID (required)")
	resume       = flag.Bool("resume", false, "Resume from checkpoint")
	checkpoint   = flag.String("checkpoint", "./import-checkpoint.json", "Checkpoint file path")
	dryRun       = flag.Bool("dry-run", false, "Parse SQL only, don't write to DB")
	verbose      = flag.Bool("verbose", false, "Detailed logging")
	batchSize    = flag.Int("batch-size", 50, "Entries per DB transaction")
	dbPath       = flag.String("db", "ad-catalog.db", "Path to AdHive SQLite database")
)

// Shiori data structures
type ShioriBookmark struct {
	ID         int
	URL        string
	Title      string
	Excerpt    string
	Author     string
	Content    string
	HTML       string
	CreatedAt  time.Time
	ModifiedAt time.Time
	HasContent bool
	Tags       []string
}

type ShioriTag struct {
	ID   int
	Name string
}

// IDMapper maps Shiori IDs to AdHive UUIDs
type IDMapper struct {
	ShioriToAdHive map[int]string `json:"shiori_to_adhive"`
	CompletedPhases []int          `json:"completed_phases"`
	LastEntryID     int            `json:"last_entry_id"`
	Timestamp       time.Time     `json:"timestamp"`
	IsResumed       bool          `json:"is_resumed"`  // true if loaded from checkpoint
}

type ImportError struct {
	Phase       string    `json:"phase"`
	BookmarkID  int       `json:"bookmark_id,omitempty"`
	TagID       int       `json:"tag_id,omitempty"`
	Error       string    `json:"error"`
	Timestamp   time.Time `json:"timestamp"`
}

type ImportStats struct {
	BookmarksFound   int            `json:"bookmarks_found"`
	BookmarksImported int           `json:"bookmarks_imported"`
	TagsFound       int            `json:"tags_found"`
	TagsImported    int            `json:"tags_imported"`
	ArchivesExtracted int          `json:"archives_extracted"`
	ThumbnailsConverted int        `json:"thumbnails_converted"`
	Skipped         int            `json:"skipped"`
	Errors          []ImportError  `json:"errors"`
}

var (
	errors       []ImportError
	stats        ImportStats
	idMapper     = &IDMapper{
		ShioriToAdHive: make(map[int]string),
	}
	tagNameToID = make(map[string]string) // tag name -> AdHive tag UUID
)

// Logging
func logInfo(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func logWarn(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

func logError(err ImportError) {
	errors = append(errors, err)
	log.Printf("[ERROR] %s: %s", err.Phase, err.Error)
}

func logVerbose(format string, args ...interface{}) {
	if *verbose {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func main() {
	flag.Parse()
	
	// Normal import flow...

	// Validate required flags
	if *sqlPath == "" && *jsonPath == "" {
		fmt.Fprintf(os.Stderr, "Error: either --sql or --json is required\n")
		os.Exit(2)
	}
	if *userID == "" {
		fmt.Fprintf(os.Stderr, "Error: --user-id is required\n")
		os.Exit(2)
	}

	logInfo("Starting Shiori import...")
	logInfo("SQL dump: %s", *sqlPath)
	logInfo("User ID: %s", *userID)

	// Increase scanner max token size for large HTML content
	const maxTokenSize = 10 * 1024 * 1024 // 10MB

	// Load checkpoint if resuming
	if *resume {
		if err := loadCheckpoint(); err != nil {
			logWarn("Could not load checkpoint: %v", err)
		} else {
			idMapper.IsResumed = true
			logInfo("Resuming from checkpoint (last entry ID: %d)", idMapper.LastEntryID)
		}
	}

	// Phase 1: Parse and import SQL data
	if err := importSQLData(); err != nil {
		logError(ImportError{Phase: "sql-import", Error: err.Error()})
		os.Exit(3)
	}

	// Save checkpoint after Phase 1
	if err := saveCheckpoint(); err != nil {
		logWarn("Failed to save checkpoint: %v", err)
	}

	// Phase 2: Extract archives (optional)
	if *archivesPath != "" {
		logInfo("Extracting archives from Shiori (Phase 2)...")
		
		// Don't load checkpoint for fresh import - we'll use the data we just imported
		// Only load checkpoint if we're doing archive-only extraction
		if *resume && idMapper.ShioriToAdHive == nil {
			if err := loadCheckpoint(); err != nil {
				logWarn("Could not load checkpoint: %v", err)
			}
		}

		// If we imported SQL data in this run, we need to query the DB for ID mapping
		// Otherwise use checkpoint mapping
		if len(idMapper.ShioriToAdHive) == 0 {
			logInfo("Querying database for entry ID mapping...")
			db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
			if err == nil {
				var entries []model.CatalogEntry
				db.Where("user_id = ?", *userID).Find(&entries)
				logInfo("Found %d entries in database for user %s", len(entries), *userID)
			}
		}

		// Connect to database
		db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
		if err != nil {
			logError(ImportError{Phase: "archive-extraction", Error: fmt.Sprintf("failed to connect to database: %v", err)})
			os.Exit(4)
		}

		// Create extractor
		extractor := importtool.NewBoltExtractor(*archivesPath, "./data")

		// Convert map[int]string to map[string]string
		idMapping := make(map[string]string)
		for k, v := range idMapper.ShioriToAdHive {
			idMapping[fmt.Sprintf("%d", k)] = v
		}

		// Extract all archives with a generous timeout (5 minutes for all)
		logInfo("Extracting %d archives (this may take a few minutes)...", len(idMapping))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		
		extractResults, err := extractor.ExtractAll(ctx, idMapping)
		if err != nil {
			logWarn("Archive extraction error: %v", err)
		}

		// Update entries with archive status and create DB records
		entryRepo := repository.NewEntryRepository(db)
		revisionRepo := repository.NewArchiveRevisionRepository(db)
		assetRepo := repository.NewArchiveAssetRepository(db)
		thumbRepo := repository.NewThumbnailCandidateRepository(db)
		
		successCount := 0
		failCount := 0
		
		for _, result := range extractResults {
			if result.Success {
				// Create archive revision record
				revision := &model.ArchiveRevision{
					ID:          result.RevisionID,
					EntryID:     result.EntryID,
					RevisionNo:  1,
					Engine:      "shiori-migrated",
					RootPath:    fmt.Sprintf("data/archives/%s/rev-0001", result.EntryID),
					IndexPath:   fmt.Sprintf("data/archives/%s/rev-0001/index.html", result.EntryID),
					ManifestPath: fmt.Sprintf("data/archives/%s/rev-0001/manifest.json", result.EntryID),
					Status:      model.ArchiveRevisionStatusSuccess,
					CapturedAt:  time.Now(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				if err := revisionRepo.Create(context.Background(), revision); err != nil {
					logWarn("Failed to create revision record for %s: %v", result.EntryID, err)
				}

				// Create asset records
				if len(result.Assets) > 0 {
					var assetRecords []*model.ArchiveAsset
					for _, asset := range result.Assets {
						assetRecord := &model.ArchiveAsset{
							ID:          uuid.New().String(),
							RevisionID:  result.RevisionID,
							RootPath:    fmt.Sprintf("data/archives/%s/rev-0001/assets", result.EntryID),
							SourceURL:   asset.SourceURL,
							LocalPath:   asset.LocalPath,
							ContentHash: asset.ContentHash,
							MimeType:    asset.MimeType,
							Bytes:       asset.Bytes,
							Kind:        model.ArchiveAssetKind(asset.Kind),
							DownloadStatus: model.ArchiveAssetDownloadStatusOK,
							CreatedAt:   time.Now(),
						}
						assetRecords = append(assetRecords, assetRecord)
					}
					if err := assetRepo.CreateBatch(context.Background(), assetRecords); err != nil {
						logWarn("Failed to create asset records: %v", err)
					}
				}

				// Update entry with archive info
				entry, err := entryRepo.GetByID(context.Background(), result.EntryID)
				if err == nil && entry != nil {
					entry.ArchiveStatus = model.ArchiveStatusSuccess
					entry.ArchivePath = fmt.Sprintf("archives/%s/rev-0001", result.EntryID)
					entry.ArchiveFidelity = model.ArchiveFidelityPartial
					entry.ArchiveCurrentRevisionID = &result.RevisionID

					// Extract thumbnail candidates from archive assets
					for _, asset := range result.Assets {
						// Only use image assets as thumbnail candidates
						if !strings.HasPrefix(asset.MimeType, "image/") {
							continue
						}

						// Score based on size (larger = better for thumbnail)
						score := float64(asset.Bytes)
						// Boost scores for common thumbnail-friendly formats
						if asset.MimeType == "image/jpeg" || asset.MimeType == "image/png" {
							score *= 1.5
						}

						candidate := &model.ThumbnailCandidate{
							ID:         uuid.New().String(),
							EntryID:    result.EntryID,
							RevisionID: &result.RevisionID,
							SourceType: model.ThumbnailCandidateSourceArchive,
							Path:       asset.LocalPath,
							Score:      score,
							Selected:   false,
							CreatedAt:  time.Now().UTC(),
						}

						if err := thumbRepo.Create(context.Background(), candidate); err != nil {
							logWarn("Failed to create thumbnail candidate: %v", err)
						}
					}

					entryRepo.Update(context.Background(), entry)
				}
				successCount++
			} else {
				failCount++
				logWarn("Failed to extract archive for entry %s: %s", result.BookmarkID, result.Error)
			}
		}

		logInfo("Archive extraction complete: %d success, %d failed", successCount, failCount)
	}

	// Phase 3: Convert thumbnails (optional)
	if *thumbsPath != "" {
		logInfo("Converting thumbnails (Phase 3)...")
		
		// Connect to database for thumbnail candidates
		db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
		if err != nil {
			logError(ImportError{Phase: "thumbnail-conversion", Error: err.Error()})
		} else {
			// Auto-migrate thumbnail_candidates table
			db.AutoMigrate(&model.ThumbnailCandidate{})
			
			// Get thumbnail candidate repo
			thumbRepo := repository.NewThumbnailCandidateRepository(db)
			
			// Create converter
			adhiveDataDir := "./data"
			converter := importer.NewThumbnailConverter(*thumbsPath, adhiveDataDir+"/thumbnails")
			
			// Convert each thumbnail
			for shioriID, adhiveID := range idMapper.ShioriToAdHive {
				candidate, err := converter.ConvertThumbnail(shioriID, adhiveID)
				if err != nil {
					logWarn("Failed to convert thumbnail for Shiori ID %d: %v", shioriID, err)
					stats.Skipped++
					continue
				}
				
				// Skip if no thumbnail exists for this entry
				if candidate == nil {
					// No thumbnail for this entry - skip silently
					continue
				}
				
				// Save to database
				if err := thumbRepo.Create(context.Background(), candidate); err != nil {
					logWarn("Failed to save thumbnail candidate: %v", err)
					stats.Skipped++
					continue
				}
				
				// Update entry's thumbnail path to API URL format (not file path)
				// Frontend expects: <img src="{entry.thumbnail_path}"/> to be API URL
				db.Model(&model.CatalogEntry{}).Where("id = ?", adhiveID).Updates(map[string]interface{}{
					"thumbnail_path":   "/api/v1/files/thumbnails/" + adhiveID,
					"thumbnail_source": model.ThumbnailSourceAuto,
				})
				
				stats.ThumbnailsConverted++
			}
			
			logInfo("Converted %d thumbnails", stats.ThumbnailsConverted)
		}
	}

	// Save checkpoint
	if err := saveCheckpoint(); err != nil {
		logWarn("Could not save checkpoint: %v", err)
	}

	// Print summary
	logInfo("Import complete!")
	logInfo("  Bookmarks: %d found, %d imported", stats.BookmarksFound, stats.BookmarksImported)
	logInfo("  Tags: %d found, %d imported", stats.TagsFound, stats.TagsImported)
	logInfo("  Skipped: %d", stats.Skipped)

	if len(errors) > 0 {
		logWarn("  Errors: %d", len(errors))
		if err := saveErrors(); err != nil {
			logWarn("Could not save error log: %v", err)
		}
		os.Exit(5)
	}

	os.Exit(0)
}

// parseSQLDump parses the MariaDB dump file (or loads from JSON)
func importSQLData() error {
	// If JSON path provided, load from pre-parsed JSON (recommended)
	if *jsonPath != "" {
		return importFromJSON(*jsonPath)
	}

	logInfo("Parsing SQL dump...")

	file, err := os.Open(*sqlPath)
	if err != nil {
		return fmt.Errorf("failed to open SQL file: %w", err)
	}
	defer file.Close()

	var bookmarks []ShioriBookmark
	var tags []ShioriTag

	// Increase scanner max token size for large HTML content
	const maxTokenSize = 10 * 1024 * 1024 // 10MB
	scanner := bufio.NewScanner(file)
	buf := make([]byte, maxTokenSize)
	scanner.Buffer(buf, maxTokenSize)
	var currentTable string
	var inValues bool
	var valuesBuffer strings.Builder

// 	logVerbose("Starting scan of SQL file...")

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Detect table
		if strings.HasPrefix(line, "INSERT INTO") {
			// Reset for new INSERT
			currentTable = ""
			inValues = false
			valuesBuffer.Reset()
			
			// Extract table name
			re := regexp.MustCompile("INSERT\\s+INTO\\s+[`]*(\\w+)[`]*")
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				currentTable = matches[1]
// 				logVerbose("Detected table: %s", currentTable)
			}
			// Check if VALUES is on this line
			upperLine := strings.ToUpper(line)
			valuesIdx := strings.Index(upperLine, "VALUES")
			if valuesIdx != -1 {
				// Check if there's data after VALUES on the same line
				afterValues := strings.TrimSpace(line[valuesIdx+6:])
				if len(afterValues) > 0 {
					// There's data on the same line
					valuesBuffer.WriteString(afterValues)
// 					logVerbose("Found VALUES with data, buffer: %d chars", valuesBuffer.Len())
				} else {
					// VALUES is on its own line, will get data on next lines
// 					logVerbose("VALUES keyword found, waiting for data...")
				}
				inValues = true
			}
			continue
		}

		if inValues {
			valuesBuffer.WriteString(" " + line)

			// Check if we've reached end of statement (ends with semicolon)
			if strings.HasSuffix(line, ";") {
				valuesStr := valuesBuffer.String()
				valuesBuffer.Reset()
				
				if currentTable == "" {
					logWarn("No table detected before semicolon, skipping")
					continue
				}

				switch currentTable {
				case "bookmark", "bookmarks":
					parsed, err := parseBookmarks(valuesStr)
					if err != nil {
						logWarn("Failed to parse bookmarks: %v", err)
					}
					bookmarks = append(bookmarks, parsed...)
					stats.BookmarksFound += len(parsed)

				case "tag", "tags":
					parsed, err := parseTags(valuesStr)
					if err != nil {
						logWarn("Failed to parse tags: %v", err)
					}
					tags = append(tags, parsed...)
					stats.TagsFound += len(parsed)
				}

				inValues = false
				currentTable = ""
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading SQL file: %w", err)
	}

	logInfo("Found %d bookmarks, %d tags", len(bookmarks), len(tags))

	if len(bookmarks) == 0 {
		logWarn("No bookmarks found in SQL dump")
		return nil
	}

	if *dryRun {
		logInfo("Dry run mode - skipping database write")
		return nil
	}

	// Connect to database
	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(
		&model.CatalogEntry{},
		&model.Tag{},
		&model.EntryTag{},
		&model.ArchiveRevision{},
		&model.ArchiveAsset{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Import tags first
	logInfo("Importing tags...")
	tagRepo := repository.NewTagRepository(db)
	for _, tag := range tags {
		tagID := uuid.New().String()
		tagNameToID[tag.Name] = tagID

		// Check if tag already exists for this user
		existing, _ := tagRepo.FindByName(*userID, tag.Name)
		if existing != nil {
			tagNameToID[tag.Name] = existing.ID
// 			logVerbose("Tag '%s' already exists, using existing", tag.Name)
			continue
		}

		newTag := &model.Tag{
			ID:        tagID,
			UserID:    *userID,
			Name:      tag.Name,
			Color:     "#6B7280",
			CreatedAt: time.Now(),
		}
		if err := tagRepo.Create(newTag); err != nil {
			logError(ImportError{Phase: "tag-import", TagID: tag.ID, Error: err.Error()})
			continue
		}
		stats.TagsImported++
	}
	logInfo("Imported %d tags", stats.TagsImported)

	// Import bookmarks
	logInfo("Importing bookmarks...")
	entryRepo := repository.NewEntryRepository(db)
	processed := 0
	skipped := 0

	for _, bm := range bookmarks {
		// Skip if already processed (resume) - only skip if we're actually resuming
		if idMapper.IsResumed && bm.ID <= idMapper.LastEntryID {
			skipped++
			continue
		}

		// Skip if URL already exists in AdHive for this user (duplicate)
		existingEntry, err := entryRepo.FindByURL(context.Background(), *userID, bm.URL)
		if err == nil && existingEntry != nil {
// 			logVerbose("Skipping duplicate URL: %s", bm.URL)
			idMapper.ShioriToAdHive[bm.ID] = existingEntry.ID // Map to existing entry
			skipped++
			continue
		}

		entryID := uuid.New().String()
		if processed < 3 {
			logInfo("DEBUG: Adding bm.ID=%d -> entryID=%s", bm.ID, entryID)
		}
		idMapper.ShioriToAdHive[bm.ID] = entryID

		archiveStatus := model.ArchiveStatusFailed
		if bm.HasContent {
			archiveStatus = model.ArchiveStatusPending
		}

		entry := &model.CatalogEntry{
			ID:             entryID,
			UserID:         *userID,
			URL:            bm.URL,
			Title:          bm.Title,
			Description:    bm.Excerpt,
			ArchiveStatus:  archiveStatus,
			ImportedFrom:   "shiori",
			CreatedAt:      bm.CreatedAt,
			UpdatedAt:      time.Now(),
		}

		if err := entryRepo.Create(context.Background(), entry); err != nil {
			logError(ImportError{Phase: "bookmark-import", BookmarkID: bm.ID, Error: err.Error()})
			continue
		}

		// Link tags
		for _, tagName := range bm.Tags {
			if tagID, ok := tagNameToID[tagName]; ok {
				entryTag := &model.EntryTag{
					EntryID: entryID,
					TagID:   tagID,
				}
				db.Create(entryTag)
			}
		}

		stats.BookmarksImported++
		processed++

		// Update checkpoint periodically
		if processed%*batchSize == 0 {
			idMapper.LastEntryID = bm.ID
			saveCheckpoint()
			logInfo("Imported %d bookmarks...", stats.BookmarksImported)
		}
	}

	idMapper.LastEntryID = bookmarks[len(bookmarks)-1].ID
	stats.Skipped = skipped
	logInfo("Imported %d bookmarks (%d skipped)", stats.BookmarksImported, skipped)

	return nil
}

// parseBookmarks parses bookmark INSERT VALUES - splits by ), or ); (outside quotes)
func parseBookmarks(values string) ([]ShioriBookmark, error) {
	var bookmarks []ShioriBookmark

	values = strings.TrimSpace(values)
	
	if len(values) == 0 {
		return bookmarks, nil
	}

	// Split by ")," outside quotes - handle both backslash escapes and doubled quotes
	var tuples []string
	tupleStart := 0
	inQuote := false
	var quoteChar byte
	escaped := false
	
	for i := 0; i < len(values); i++ {
		ch := values[i]
		
		// Handle escaped characters
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && i+1 < len(values) {
			// Check if next char is an escape sequence
			next := values[i+1]
			if next == '\\' || next == '"' || next == '\'' {
				escaped = true
				continue
			}
		}
		
		// Handle quote state
		if !inQuote && (ch == '"' || ch == '\'') {
			inQuote = true
			quoteChar = ch
			continue
		}
		if inQuote && ch == quoteChar {
			// Check for escaped quote (doubled quote like '' or "")
			if i+1 < len(values) && values[i+1] == quoteChar {
				// Skip both quotes - increment i by changing loop logic below instead
			} else {
				inQuote = false
			}
			continue
		}
		
		// Outside quotes - check for ), or );
		if !inQuote && i+1 < len(values) && ch == ')' && (values[i+1] == ',' || values[i+1] == ';') {
			tuple := values[tupleStart:i+1]
			tuples = append(tuples, tuple)
			tupleStart = i + 2
		}
	}
	
	// Add remaining
	if tupleStart < len(values) {
		tuples = append(tuples, values[tupleStart:])
	}

	// Parse each tuple
	for _, tuple := range tuples {
		tuple = strings.TrimSpace(tuple)
		if tuple == "" || len(tuple) < 10 {
			continue
		}
		// Remove surrounding parentheses if present
		tuple = strings.Trim(tuple, "()")
		bookmark, err := parseBookmarkTuple(tuple)
		if err != nil {
			logWarn("Failed to parse bookmark tuple: %v", err)
			continue
		}
		// Skip entries with empty URL
		if bookmark.URL == "" {
			continue
		}
		bookmarks = append(bookmarks, bookmark)
	}

	return bookmarks, nil
}

func parseBookmarkTuple(tuple string) (ShioriBookmark, error) {
// 	logVerbose("parseBookmarkTuple: tuple len=%d, first 50: %s", len(tuple), tuple[:min(50, len(tuple))])
	
	// Parse CSV-like, handling quoted strings
	fields, err := splitSQLValues(tuple)
	if err != nil {
		logWarn("splitSQLValues error: %v", err)
		return ShioriBookmark{}, err
	}

// 	logVerbose("splitSQLValues returned %d fields", len(fields))
	
	bm := ShioriBookmark{}

	// Based on Shiori schema: id, url, title, excerpt, author, public, content, html, created_at, has_content, modified_at
	if len(fields) >= 11 {
		// Clean ID field - remove leading '(' or other artifacts from SQL tuple
		idStr := strings.Trim(fields[0], "() ")
		bm.ID, _ = strconv.Atoi(idStr)
		bm.URL = unquoteSQL(fields[1])
		bm.Title = unquoteSQL(fields[2])
		bm.Excerpt = unquoteSQL(fields[3])
		bm.Author = unquoteSQL(fields[4])
		bm.Content = unquoteSQL(fields[6])
		
		// Parse created_at
		if createdAt, err := time.Parse("2006-01-02 15:04:05", fields[8]); err == nil {
			bm.CreatedAt = createdAt
		}
		
		bm.HasContent = fields[9] == "1"
		
		// Also check if HTML field is not empty - this means we have content even if has_content=0
		htmlContent := unquoteSQL(fields[7])
		if htmlContent != "" {
			bm.HTML = htmlContent
			bm.HasContent = true // We have HTML content
		}
		
		if modifiedAt, err := time.Parse("2006-01-02 15:04:05", fields[10]); err == nil {
			bm.ModifiedAt = modifiedAt
		}
	}

	return bm, nil
}

// parseTags parses tag INSERT VALUES
func parseTags(values string) ([]ShioriTag, error) {
	var tags []ShioriTag

	values = strings.TrimSpace(values)
	if !strings.HasPrefix(values, "(") {
		values = "(" + values
	}
	if !strings.HasSuffix(values, ")") {
		values = values + ")"
	}

	tupleRegex := regexp.MustCompile(`\(([^)]+)\)`)
	matches := tupleRegex.FindAllStringSubmatch(values, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tuple := match[1]
		fields, err := splitSQLValues(tuple)
		if err != nil {
			continue
		}

		if len(fields) >= 2 {
			tag := ShioriTag{
				ID:   toInt(fields[0]),
				Name: unquoteSQL(fields[1]),
			}
			tags = append(tags, tag)
		}
	}

	return tags, nil
}

// splitSQLValues splits SQL values handling quoted strings
func splitSQLValues(s string) ([]string, error) {
	var fields []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	escaped := false

	for i, r := range s {
		// Handle escaped characters
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && i+1 < len(s) {
			// Check if next char is an escape sequence
			next := rune(s[i+1])
			if next == '\\' || next == '"' || next == '\'' {
				escaped = true
				current.WriteRune(r) // Keep the backslash, unquoteSQL will handle it
				continue
			}
		}
		if !inQuote && (r == '"' || r == '\'') {
			inQuote = true
			quoteChar = r
			continue
		}
		if inQuote && r == quoteChar {
			// Check for escaped quote (doubled quote like '' or "")
			if i+1 < len(s) && rune(s[i+1]) == quoteChar {
				current.WriteRune(r)
				i++
				continue
			}
			inQuote = false
			continue
		}
		if !inQuote && r == ',' {
			fields = append(fields, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	fields = append(fields, current.String())

	return fields, nil
}

func unquoteSQL(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	// Unescape (order matters!)
	// First unescape escaped backslashes, then escaped quotes
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\'`, `'`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	// Handle double escaped quotes: '' or \"\"
	s = strings.ReplaceAll(s, `''`, `'`)
	return s
}

func toInt(s string) int {
	s = strings.TrimSpace(s)
	i, _ := strconv.Atoi(s)
	return i
}

// Checkpoint functions
func loadCheckpoint() error {
	data, err := os.ReadFile(*checkpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, idMapper)
}

func saveCheckpoint() error {
	idMapper.Timestamp = time.Now()
	data, err := json.MarshalIndent(idMapper, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*checkpoint, data, 0644)
}

func saveErrors() error {
	stats.Errors = errors
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("import-errors.json", data, 0644)
}

// importFromJSON loads pre-parsed data from shiori-parser JSON output
func importFromJSON(jsonPath string) error {
	logInfo("Loading from JSON: %s", jsonPath)

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Parse JSON structure from shiori-parser
	type JSONBookmark struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		Excerpt     string `json:"excerpt"`
		Author      string `json:"author"`
		CreatedAt   string `json:"created_at"`
		ModifiedAt  string `json:"modified_at"`
	}

	type JSONTag struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type JSONBookmarkTag struct {
		BookmarkID string `json:"bookmark_id"`
		TagID     string `json:"tag_id"`
	}

	type JSONData struct {
		Bookmarks    []JSONBookmark    `json:"bookmarks"`
		Tags         []JSONTag         `json:"tags"`
		BookmarkTags []JSONBookmarkTag `json:"bookmark_tags"`
	}

	var parsed JSONData
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	logInfo("Found %d bookmarks, %d tags", len(parsed.Bookmarks), len(parsed.Tags))
	stats.BookmarksFound = len(parsed.Bookmarks)
	stats.TagsFound = len(parsed.Tags)

	if *dryRun {
		logInfo("Dry run mode - skipping database write")
		return nil
	}

	// Connect to database
	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(
		&model.CatalogEntry{},
		&model.Tag{},
		&model.EntryTag{},
		&model.ArchiveRevision{},
		&model.ArchiveAsset{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Import tags first
	logInfo("Importing tags...")
	tagRepo := repository.NewTagRepository(db)
	bookmarkTagMap := make(map[string][]string) // bookmark ID -> list of tag names

	// Build bookmark-tag mapping first
	for _, bt := range parsed.BookmarkTags {
		bookmarkTagMap[bt.BookmarkID] = append(bookmarkTagMap[bt.BookmarkID], "")
	}

	for _, t := range parsed.Tags {
		tagID := uuid.New().String()
		tagNameToID[t.Name] = tagID

		// Check if tag already exists for this user
		existing, _ := tagRepo.FindByName(*userID, t.Name)
		if existing != nil {
			tagNameToID[t.Name] = existing.ID
// 			logVerbose("Tag '%s' already exists, using existing", t.Name)
			continue
		}

		newTag := &model.Tag{
			ID:        tagID,
			UserID:    *userID,
			Name:      t.Name,
			Color:     "#6B7280",
			CreatedAt: time.Now(),
		}
		if err := tagRepo.Create(newTag); err != nil {
			logError(ImportError{Phase: "tag-import", Error: err.Error()})
			continue
		}
		stats.TagsImported++
	}
	logInfo("Imported %d tags", stats.TagsImported)

	// Build tag lookup from bookmark_tags
	for _, bt := range parsed.BookmarkTags {
		// Find the tag name for this tag ID
		for _, t := range parsed.Tags {
			if t.ID == bt.TagID {
				bookmarkTagMap[bt.BookmarkID] = append(bookmarkTagMap[bt.BookmarkID], t.Name)
				break
			}
		}
	}

	// Import bookmarks
	logInfo("Importing bookmarks...")
	entryRepo := repository.NewEntryRepository(db)
	processed := 0
	skipped := 0

	for _, bm := range parsed.Bookmarks {
		// Skip if URL empty (invalid)
		if bm.URL == "" {
// 			logVerbose("Skipping bookmark with empty URL (ID: %s)", bm.ID)
			skipped++
			continue
		}

		// Skip if URL already exists in AdHive for this user (duplicate)
		existingEntry, err := entryRepo.FindByURL(context.Background(), *userID, bm.URL)
		if err == nil && existingEntry != nil {
// 			logVerbose("Skipping duplicate URL: %s", bm.URL)
			idMapper.ShioriToAdHive[toInt(bm.ID)] = existingEntry.ID
			skipped++
			continue
		}

		entryID := uuid.New().String()
		idMapper.ShioriToAdHive[toInt(bm.ID)] = entryID

		// Parse timestamps
		createdAt := time.Now()
		if bm.CreatedAt != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", bm.CreatedAt); err == nil {
				createdAt = t
			}
		}

		entry := &model.CatalogEntry{
			ID:             entryID,
			UserID:         *userID,
			URL:            bm.URL,
			Title:          bm.Title,
			Description:    bm.Excerpt,
			ArchiveStatus:  model.ArchiveStatusPending, // Will be updated after archive extraction
			ImportedFrom:   "shiori",
			CreatedAt:      createdAt,
			UpdatedAt:      time.Now(),
		}

		if err := entryRepo.Create(context.Background(), entry); err != nil {
			logError(ImportError{Phase: "bookmark-import", BookmarkID: toInt(bm.ID), Error: err.Error()})
			continue
		}

		// Link tags for this bookmark
		if tagNames, ok := bookmarkTagMap[bm.ID]; ok {
			for _, tagName := range tagNames {
				if tagName == "" {
					continue // skip empty
				}
				if tagID, ok := tagNameToID[tagName]; ok {
					entryTag := &model.EntryTag{
						EntryID: entryID,
						TagID:   tagID,
					}
					db.Create(entryTag)
				}
			}
		}

		stats.BookmarksImported++
		processed++

		// Update checkpoint periodically
		if processed%*batchSize == 0 {
			idMapper.LastEntryID = toInt(bm.ID)
			saveCheckpoint()
			logInfo("Imported %d bookmarks...", stats.BookmarksImported)
		}
	}

	idMapper.LastEntryID = toInt(parsed.Bookmarks[len(parsed.Bookmarks)-1].ID)
	stats.Skipped = skipped
	logInfo("Imported %d bookmarks (%d skipped)", stats.BookmarksImported, skipped)

	return nil
}
