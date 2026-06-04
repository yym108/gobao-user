//go:build integration

package mysql

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/yym108/gobao-user/internal/domain"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UserModel{}, &AddressModel{}))
	return db
}

func TestCreateAndFindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	user := &domain.User{Email: "alice@test.com", PasswordHash: "hash", Nickname: "Alice", AvatarURL: "https://example.com/alice.png"}
	require.NoError(t, repo.Create(ctx, user))
	assert.Greater(t, user.ID, int64(0))

	found, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "alice@test.com", found.Email)
	assert.Equal(t, "Alice", found.Nickname)
	assert.Equal(t, "https://example.com/alice.png", found.AvatarURL)
}

func TestFindByEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	user := &domain.User{Email: "bob@test.com", PasswordHash: "hash", Nickname: "Bob"}
	require.NoError(t, repo.Create(ctx, user))

	found, err := repo.FindByEmail(ctx, "bob@test.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, user.ID, found.ID)

	notFound, err := repo.FindByEmail(ctx, "nobody@test.com")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestExistsByEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	exists, err := repo.ExistsByEmail(ctx, "carol@test.com")
	require.NoError(t, err)
	assert.False(t, exists)

	user := &domain.User{Email: "carol@test.com", PasswordHash: "hash", Nickname: "Carol"}
	require.NoError(t, repo.Create(ctx, user))

	exists, err = repo.ExistsByEmail(ctx, "carol@test.com")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateDuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	u1 := &domain.User{Email: "dup@test.com", PasswordHash: "hash1", Nickname: "A"}
	require.NoError(t, repo.Create(ctx, u1))

	u2 := &domain.User{Email: "dup@test.com", PasswordHash: "hash2", Nickname: "B"}
	err := repo.Create(ctx, u2)
	assert.Error(t, err)
}

func TestUpdateProfile(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	user := &domain.User{Email: "profile@test.com", PasswordHash: "hash", Nickname: "Before", AvatarURL: ""}
	require.NoError(t, repo.Create(ctx, user))

	require.NoError(t, repo.UpdateProfile(ctx, user.ID, "After", "https://example.com/avatar.png"))

	found, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "After", found.Nickname)
	assert.Equal(t, "https://example.com/avatar.png", found.AvatarURL)
}

func TestUpdatePasswordHash(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	user := &domain.User{Email: "password@test.com", PasswordHash: "old-hash", Nickname: "User"}
	require.NoError(t, repo.Create(ctx, user))

	require.NoError(t, repo.UpdatePasswordHash(ctx, user.ID, "new-hash"))

	found, err := repo.FindByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "new-hash", found.PasswordHash)
}

func TestAddressCRUDAndDefaultFlow(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepo(db)
	ctx := context.Background()

	user := &domain.User{Email: "address@test.com", PasswordHash: "hash", Nickname: "Alice"}
	require.NoError(t, repo.Create(ctx, user))

	address1 := &domain.Address{
		UserID:        user.ID,
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "上海市",
		City:          "上海市",
		District:      "浦东新区",
		AddressLine:   "世纪大道1号",
		PostalCode:    "200120",
		IsDefault:     true,
	}
	require.NoError(t, repo.CreateAddress(ctx, address1))
	assert.Greater(t, address1.ID, int64(0))

	count, err := repo.CountAddressesByUserID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	address2 := &domain.Address{
		UserID:        user.ID,
		ReceiverName:  "李四",
		ReceiverPhone: "13900139000",
		Province:      "北京市",
		City:          "北京市",
		District:      "朝阳区",
		AddressLine:   "建国路8号",
		PostalCode:    "100020",
		IsDefault:     false,
	}
	require.NoError(t, repo.CreateAddress(ctx, address2))

	addresses, err := repo.ListAddresses(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, addresses, 2)

	require.NoError(t, repo.ClearDefaultAddresses(ctx, user.ID))
	address2.IsDefault = true
	require.NoError(t, repo.UpdateAddress(ctx, address2))

	found1, err := repo.FindAddressByID(ctx, address1.ID)
	require.NoError(t, err)
	require.NotNil(t, found1)
	assert.False(t, found1.IsDefault)

	found2, err := repo.FindAddressByID(ctx, address2.ID)
	require.NoError(t, err)
	require.NotNil(t, found2)
	assert.True(t, found2.IsDefault)

	require.NoError(t, repo.DeleteAddress(ctx, address1.ID))
	deleted, err := repo.FindAddressByID(ctx, address1.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}
