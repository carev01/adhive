package repository

import (
	"context"
	"testing"

	"github.com/carev01/adhive/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestArchiveRevisionRepository_GetByID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ArchiveRevision{}))

	repo := NewArchiveRevisionRepository(db)
	ctx := context.Background()

	// Create a revision
	revision := &model.ArchiveRevision{
		ID:         "test-rev-1",
		EntryID:    "entry-1",
		RevisionNo: 1,
		Engine:     model.ArchiveEnginePlaywright,
		RootPath:   "/data/archives/entry-1/rev-0001",
		IndexPath:  "/data/archives/entry-1/rev-0001/index.html",
		ManifestPath: "/data/archives/entry-1/rev-0001/manifest.json",
		Status:     model.ArchiveRevisionStatusSuccess,
	}
	err = repo.Create(ctx, revision)
	require.NoError(t, err)

	// GetByID - found
	found, err := repo.GetByID(ctx, "test-rev-1")
	require.NoError(t, err)
	assert.Equal(t, "entry-1", found.EntryID)
	assert.Equal(t, 1, found.RevisionNo)

	// GetByID - not found
	_, err = repo.GetByID(ctx, "nonexistent")
	assert.Error(t, err)
}
