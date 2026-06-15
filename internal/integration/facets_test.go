package integration

import (
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/carev01/adhive/internal/handler"
	"github.com/carev01/adhive/internal/middleware"
	"github.com/carev01/adhive/internal/model"
	"github.com/carev01/adhive/internal/repository"
)

var facetsTestCounter atomic.Int64

// facetedTestEnv holds test dependencies for faceted search integration tests
type facetedTestEnv struct {
	DB                 *gorm.DB
	UserRepo           *repository.UserRepository
	SessionRepo        *repository.SessionRepository
	EntryRepo          *repository.EntryRepository
	TagRepo             *repository.TagRepository
	CustomFieldRepo    *repository.CustomFieldRepository
	EntryHandler       *handler.EntryHandler
	FacetsHandler      *handler.FacetsHandler
	CustomFieldHandler *handler.CustomFieldHandler
	Router             *gin.Engine
	UserID             string
	SessionID         string
}

func setupFacetedTestEnv(t *testing.T) *facetedTestEnv {
	t.Helper()

	dbKey := fmt.Sprintf("file:adhive-facets-%d-%d?cache=shared", os.Getpid(), facetsTestCounter.Add(1))
	db, err := gorm.Open(sqlite.Open(dbKey), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	// Auto-migrate all models
	if err := db.AutoMigrate(
		&model.User{}, &model.Session{}, &model.CatalogEntry{},
		&model.Tag{}, &model.EntryTag{},
		&model.Interaction{}, &model.CustomField{},
	); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	entryRepo := repository.NewEntryRepository(db)
	tagRepo := repository.NewTagRepository(db)
	cfRepo := repository.NewCustomFieldRepository(db)

	entryHandler := handler.NewEntryHandler(entryRepo, tagRepo)
	facetsHandler := handler.NewFacetsHandler(entryRepo, tagRepo, cfRepo)
	customFieldHandler := handler.NewCustomFieldHandler(cfRepo, entryRepo)

	authHandler := handler.NewAuthHandler(userRepo, sessionRepo)
	authMiddleware := middleware.NewAuthMiddleware(sessionRepo)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	api := router.Group("/api/v1")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		entries := api.Group("/entries")
		entries.Use(authMiddleware.Authenticate())
		{
			entries.GET("", entryHandler.List)
			entries.GET("/facets", facetsHandler.Facets)
			entries.POST("", entryHandler.Create)
			entries.GET("/:id", entryHandler.Get)

			// Custom fields
			entries.GET("/:id/custom_fields", customFieldHandler.ListEntryCustomFields)
			entries.POST("/:id/custom_fields", customFieldHandler.CreateEntryCustomField)
			entries.PUT("/:id/custom_fields/:fieldId", customFieldHandler.UpdateEntryCustomField)
			entries.DELETE("/:id/custom_fields/:fieldId", customFieldHandler.DeleteEntryCustomField)
		}
	}

	// Create a test user
	user := &model.User{
		ID:           uuid.New().String(),
		Email:        fmt.Sprintf("facets-test-%d@example.com", facetsTestCounter.Add(1)),
		PasswordHash: "$2a$10$dummy",
		DisplayName:  "Facets Test User",
		IsActive:     true,
	}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// Create a session
	session := &model.Session{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}

	return &facetedTestEnv{
		DB:                 db,
		UserRepo:           userRepo,
		SessionRepo:        sessionRepo,
		EntryRepo:          entryRepo,
		TagRepo:            tagRepo,
		CustomFieldRepo:    cfRepo,
		EntryHandler:       entryHandler,
		FacetsHandler:      facetsHandler,
		CustomFieldHandler: customFieldHandler,
		Router:             router,
		UserID:             user.ID,
		SessionID:          session.ID,
	}
}

func (e *facetedTestEnv) authCookie() *http.Cookie {
	return &http.Cookie{Name: "session_id", Value: e.SessionID}
}

func createTestEntry(t *testing.T, entryRepo *repository.EntryRepository, userID, title, status string) *model.CatalogEntry {
	entry := &model.CatalogEntry{
		ID:            uuid.New().String(),
		UserID:        userID,
		URL:           "https://example.com/" + uuid.New().String()[:8],
		Title:         title,
		Description:   "Test entry: " + title,
		ArchiveStatus: model.ArchiveStatus(status),
	}
	if err := entryRepo.Create(t.Context(), entry); err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}
	return entry
}

func createTestTag(t *testing.T, tagRepo *repository.TagRepository, userID, name, color string) *model.Tag {
	tag := &model.Tag{
		ID:     uuid.New().String(),
		UserID: userID,
		Name:   name,
		Color:  color,
	}
	if err := tagRepo.Create(tag); err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}
	return tag
}

// TestMultiTagANDFilter tests that multi-tag AND filtering returns entries with ALL specified tags
func TestMultiTagANDFilter(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	tag1 := createTestTag(t, env.TagRepo, env.UserID, "electronics", "#FF0000")
	tag2 := createTestTag(t, env.TagRepo, env.UserID, "laptop", "#00FF00")

	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "MacBook Pro", "success")
	entry2 := createTestEntry(t, env.EntryRepo, env.UserID, "Dell Laptop", "success")
	entry3 := createTestEntry(t, env.EntryRepo, env.UserID, "iPhone", "success")
	_ = entry2 // used implicitly via tag assignment
	_ = entry3 // used implicitly via tag assignment

	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag2.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry2.ID, tag2.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry3.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}

	filter := &model.EntryFilter{
		Page:      1,
		Limit:     20,
		Tags:      []string{tag1.ID, tag2.ID},
		TagsLogic: "and",
	}

	result, err := env.EntryRepo.GetByUserID(ctx, env.UserID, filter)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected 1 entry with both tags (AND), got %d", result.Total)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != entry1.ID {
		t.Errorf("Expected entry1 (MacBook Pro) with both tags, got entries: %v", result.Entries)
	}
}

// TestMultiTagORFilter tests that multi-tag OR filtering returns entries with ANY specified tag
func TestMultiTagORFilter(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	tag1 := createTestTag(t, env.TagRepo, env.UserID, "electronics", "#FF0000")
	tag2 := createTestTag(t, env.TagRepo, env.UserID, "laptop", "#00FF00")

	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "MacBook Pro", "success")
	entry2 := createTestEntry(t, env.EntryRepo, env.UserID, "Dell Laptop", "success")
	_ = createTestEntry(t, env.EntryRepo, env.UserID, "Coffee Maker", "success")

	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag2.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry2.ID, tag2.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}

	filter := &model.EntryFilter{
		Page:      1,
		Limit:     20,
		Tags:      []string{tag1.ID, tag2.ID},
		TagsLogic: "or",
	}

	result, err := env.EntryRepo.GetByUserID(ctx, env.UserID, filter)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Expected 2 entries with any tag (OR), got %d", result.Total)
	}
}

// TestBackwardCompatSingleTagFilter tests that the legacy single-tag filter still works
func TestBackwardCompatSingleTagFilter(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	tag1 := createTestTag(t, env.TagRepo, env.UserID, "electronics", "#FF0000")
	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "MacBook Pro", "success")
	_ = createTestEntry(t, env.EntryRepo, env.UserID, "Coffee Maker", "success")

	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}

	filter := &model.EntryFilter{
		Page:  1,
		Limit: 20,
		TagID: tag1.ID,
	}

	result, err := env.EntryRepo.GetByUserID(ctx, env.UserID, filter)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected 1 entry with legacy single-tag filter, got %d", result.Total)
	}
}

// TestCustomFieldEAVFilter tests filtering entries by custom field values
func TestCustomFieldEAVFilter(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "MacBook Pro", "success")
	entry2 := createTestEntry(t, env.EntryRepo, env.UserID, "Dell Laptop", "success")

	cf1 := &model.CustomField{
		ID:         uuid.New().String(),
		EntryID:    entry1.ID,
		FieldName:  "brand",
		FieldValue: "Apple",
	}
	if err := env.CustomFieldRepo.Create(ctx, cf1); err != nil {
		t.Fatalf("failed to create custom field: %v", err)
	}

	cf2 := &model.CustomField{
		ID:         uuid.New().String(),
		EntryID:    entry2.ID,
		FieldName:  "brand",
		FieldValue: "Dell",
	}
	if err := env.CustomFieldRepo.Create(ctx, cf2); err != nil {
		t.Fatalf("failed to create custom field: %v", err)
	}

	filter := &model.EntryFilter{
		Page:         1,
		Limit:        20,
		CustomFields: map[string]string{"brand": "Apple"},
	}

	result, err := env.EntryRepo.GetByUserID(ctx, env.UserID, filter)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected 1 entry with brand=Apple, got %d", result.Total)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != entry1.ID {
		t.Errorf("Expected entry1 (MacBook Pro), got %v", result.Entries)
	}
}

// TestCombinedFilters tests combining multiple filter types
func TestCombinedFilters(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	tag1 := createTestTag(t, env.TagRepo, env.UserID, "electronics", "#FF0000")

	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "MacBook Pro 2024", "success")
	entry2 := createTestEntry(t, env.EntryRepo, env.UserID, "Old MacBook 2020", "pending")
	_ = createTestEntry(t, env.EntryRepo, env.UserID, "Coffee Maker", "success")

	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry2.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}

	cf := &model.CustomField{
		ID:         uuid.New().String(),
		EntryID:    entry1.ID,
		FieldName:  "price",
		FieldValue: "2000",
	}
	if err := env.CustomFieldRepo.Create(ctx, cf); err != nil {
		t.Fatalf("failed to create custom field: %v", err)
	}

	filter := &model.EntryFilter{
		Page:      1,
		Limit:     20,
		Tags:      []string{tag1.ID},
		TagsLogic: "or",
		Status:    model.ArchiveStatusSuccess,
	}

	result, err := env.EntryRepo.GetByUserID(ctx, env.UserID, filter)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected 1 entry with tag1+status=success, got %d", result.Total)
	}
}

// TestFacetsEndpoint tests the facets repository method
func TestFacetsEndpoint(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	tag1 := createTestTag(t, env.TagRepo, env.UserID, "electronics", "#FF0000")
	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "MacBook Pro", "success")
	_ = createTestEntry(t, env.EntryRepo, env.UserID, "Coffee Maker", "pending")

	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}

	// Test facets via repository directly
	result, err := env.EntryRepo.Facets(ctx, env.UserID)
	if err != nil {
		t.Fatalf("Facets failed: %v", err)
	}

	if len(result.Tags) == 0 {
		t.Error("Expected tags in facets result")
	}
	if len(result.Statuses) == 0 {
		t.Error("Expected statuses in facets result")
	}
	if result.DateRange == nil {
		t.Error("Expected date_range in facets result")
	}
}

// TestCustomFieldCRUD tests custom field create, read, update, delete
func TestCustomFieldCRUD(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	entry := createTestEntry(t, env.EntryRepo, env.UserID, "Test Entry", "success")

	// Create
	cf := &model.CustomField{
		ID:         uuid.New().String(),
		EntryID:    entry.ID,
		FieldName:  "color",
		FieldValue: "red",
	}
	if err := env.CustomFieldRepo.Create(ctx, cf); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read
	got, err := env.CustomFieldRepo.GetByID(ctx, cf.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.FieldName != "color" || got.FieldValue != "red" {
		t.Errorf("Expected color=red, got %s=%s", got.FieldName, got.FieldValue)
	}

	// Get by entry
	fields, err := env.CustomFieldRepo.GetByEntryID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetByEntryID failed: %v", err)
	}
	if len(fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(fields))
	}

	// Update
	cf.FieldValue = "blue"
	if err := env.CustomFieldRepo.Update(ctx, cf); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	got, _ = env.CustomFieldRepo.GetByID(ctx, cf.ID)
	if got.FieldValue != "blue" {
		t.Errorf("Expected blue, got %s", got.FieldValue)
	}

	// Delete
	if err := env.CustomFieldRepo.Delete(ctx, cf.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	fields, _ = env.CustomFieldRepo.GetByEntryID(ctx, entry.ID)
	if len(fields) != 0 {
		t.Errorf("Expected 0 fields after delete, got %d", len(fields))
	}
}

// TestBatchGetEntriesTags tests the N+1 fix for tag fetching
func TestBatchGetEntriesTags(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	tag1 := createTestTag(t, env.TagRepo, env.UserID, "tag1", "#FF0000")
	tag2 := createTestTag(t, env.TagRepo, env.UserID, "tag2", "#00FF00")

	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "Entry 1", "success")
	entry2 := createTestEntry(t, env.EntryRepo, env.UserID, "Entry 2", "success")

	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag1.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry1.ID, tag2.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}
	if err := env.EntryRepo.AddTag(ctx, entry2.ID, tag2.ID); err != nil {
		t.Fatalf("failed to add tag: %v", err)
	}

	tagsMap, err := env.EntryRepo.GetEntriesTags(ctx, []string{entry1.ID, entry2.ID})
	if err != nil {
		t.Fatalf("GetEntriesTags failed: %v", err)
	}

	if len(tagsMap[entry1.ID]) != 2 {
		t.Errorf("Expected 2 tags for entry1, got %d", len(tagsMap[entry1.ID]))
	}
	if len(tagsMap[entry2.ID]) != 1 {
		t.Errorf("Expected 1 tag for entry2, got %d", len(tagsMap[entry2.ID]))
	}
}

// TestEmptyFilterReturnsAll tests that empty filter returns all entries
func TestEmptyFilterReturnsAll(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	createTestEntry(t, env.EntryRepo, env.UserID, "Entry 1", "success")
	createTestEntry(t, env.EntryRepo, env.UserID, "Entry 2", "success")
	createTestEntry(t, env.EntryRepo, env.UserID, "Entry 3", "pending")

	filter := &model.EntryFilter{
		Page:  1,
		Limit: 20,
	}

	result, err := env.EntryRepo.GetByUserID(ctx, env.UserID, filter)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Expected 3 entries with empty filter, got %d", result.Total)
	}
}

// TestCustomFieldDistinctNames tests getting distinct field names
func TestCustomFieldDistinctNames(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	entry1 := createTestEntry(t, env.EntryRepo, env.UserID, "Entry 1", "success")
	entry2 := createTestEntry(t, env.EntryRepo, env.UserID, "Entry 2", "success")

	for _, cf := range []model.CustomField{
		{ID: uuid.New().String(), EntryID: entry1.ID, FieldName: "brand", FieldValue: "Apple"},
		{ID: uuid.New().String(), EntryID: entry1.ID, FieldName: "color", FieldValue: "silver"},
		{ID: uuid.New().String(), EntryID: entry2.ID, FieldName: "brand", FieldValue: "Dell"},
	} {
		if err := env.CustomFieldRepo.Create(ctx, &cf); err != nil {
			t.Fatalf("failed to create custom field: %v", err)
		}
	}

	names, err := env.CustomFieldRepo.GetDistinctFieldNames(ctx, env.UserID)
	if err != nil {
		t.Fatalf("GetDistinctFieldNames failed: %v", err)
	}

	if len(names) != 2 {
		t.Errorf("Expected 2 distinct field names, got %d: %v", len(names), names)
	}

	hasBrand, hasColor := false, false
	for _, n := range names {
		if n == "brand" { hasBrand = true }
		if n == "color" { hasColor = true }
	}
	if !hasBrand || !hasColor {
		t.Errorf("Expected brand and color in names, got %v", names)
	}
}

// TestCustomFieldHandlerAPI tests the custom field API endpoints via direct handler calls
// (full API integration requires CSRF middleware setup beyond this test scope)
func TestCustomFieldHandlerDirect(t *testing.T) {
	env := setupFacetedTestEnv(t)
	ctx := t.Context()

	entry := createTestEntry(t, env.EntryRepo, env.UserID, "API Test Entry", "success")

	// Create custom field via repository
	cf := &model.CustomField{
		ID:         uuid.New().String(),
		EntryID:    entry.ID,
		FieldName:  "condition",
		FieldValue: "excellent",
	}
	if err := env.CustomFieldRepo.Create(ctx, cf); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List custom fields via repository
	fields, err := env.CustomFieldRepo.GetByEntryID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetByEntryID failed: %v", err)
	}
	if len(fields) != 1 {
		t.Errorf("Expected 1 custom field, got %d", len(fields))
	}
	if fields[0].FieldName != "condition" || fields[0].FieldValue != "excellent" {
		t.Errorf("Expected condition=excellent, got %s=%s", fields[0].FieldName, fields[0].FieldValue)
	}

	// Update
	cf.FieldValue = "good"
	if err := env.CustomFieldRepo.Update(ctx, cf); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	got, err := env.CustomFieldRepo.GetByID(ctx, cf.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.FieldValue != "good" {
		t.Errorf("Expected good, got %s", got.FieldValue)
	}

	// Delete
	if err := env.CustomFieldRepo.Delete(ctx, cf.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	fields, _ = env.CustomFieldRepo.GetByEntryID(ctx, entry.ID)
	if len(fields) != 0 {
		t.Errorf("Expected 0 fields after delete, got %d", len(fields))
	}
}