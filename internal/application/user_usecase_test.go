package application

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yym108/gobao-pkg/authn"
	"github.com/yym108/gobao-pkg/errors"
	"github.com/yym108/gobao-user/internal/domain"
)

type mockRepo struct {
	mu             sync.Mutex
	users          map[int64]*domain.User
	byEmail        map[string]*domain.User
	addresses      map[int64]*domain.Address
	addressesByUID map[int64][]int64
	nextID         int64
	nextAddressID  int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:          make(map[int64]*domain.User),
		byEmail:        make(map[string]*domain.User),
		addresses:      make(map[int64]*domain.Address),
		addressesByUID: make(map[int64][]int64),
		nextID:         1,
		nextAddressID:  1,
	}
}

func (m *mockRepo) Create(_ context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user.ID = m.nextID
	user.CreatedAt = time.Now()
	user.UpdatedAt = user.CreatedAt
	m.nextID++
	m.users[user.ID] = user
	m.byEmail[user.Email] = user
	return nil
}

func (m *mockRepo) FindByID(_ context.Context, id int64) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byEmail[email]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockRepo) ExistsByEmail(_ context.Context, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.byEmail[email]
	return ok, nil
}

func (m *mockRepo) UpdateProfile(_ context.Context, userID int64, nickname, avatarURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, ok := m.users[userID]
	if !ok {
		return nil
	}
	user.Nickname = nickname
	user.AvatarURL = avatarURL
	user.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) UpdatePasswordHash(_ context.Context, userID int64, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, ok := m.users[userID]
	if !ok {
		return nil
	}
	user.PasswordHash = passwordHash
	user.UpdatedAt = time.Now()
	return nil
}

func (m *mockRepo) ListAddresses(_ context.Context, userID int64) ([]*domain.Address, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.addressesByUID[userID]
	result := make([]*domain.Address, 0, len(ids))
	for _, id := range ids {
		if address, ok := m.addresses[id]; ok {
			copied := *address
			result = append(result, &copied)
		}
	}
	return result, nil
}

func (m *mockRepo) FindAddressByID(_ context.Context, addressID int64) (*domain.Address, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	address, ok := m.addresses[addressID]
	if !ok {
		return nil, nil
	}
	copied := *address
	return &copied, nil
}

func (m *mockRepo) CountAddressesByUserID(_ context.Context, userID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.addressesByUID[userID])), nil
}

func (m *mockRepo) CreateAddress(_ context.Context, address *domain.Address) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	address.ID = m.nextAddressID
	address.CreatedAt = now
	address.UpdatedAt = now
	m.nextAddressID++
	copied := *address
	m.addresses[address.ID] = &copied
	m.addressesByUID[address.UserID] = append(m.addressesByUID[address.UserID], address.ID)
	return nil
}

func (m *mockRepo) UpdateAddress(_ context.Context, address *domain.Address) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.addresses[address.ID]
	if !ok {
		return nil
	}
	address.CreatedAt = existing.CreatedAt
	address.UpdatedAt = time.Now()
	copied := *address
	m.addresses[address.ID] = &copied
	return nil
}

func (m *mockRepo) DeleteAddress(_ context.Context, addressID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	address, ok := m.addresses[addressID]
	if !ok {
		return nil
	}
	delete(m.addresses, addressID)
	ids := m.addressesByUID[address.UserID]
	filtered := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != addressID {
			filtered = append(filtered, id)
		}
	}
	m.addressesByUID[address.UserID] = filtered
	return nil
}

func (m *mockRepo) ClearDefaultAddresses(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.addressesByUID[userID] {
		if address, ok := m.addresses[id]; ok {
			address.IsDefault = false
			address.UpdatedAt = time.Now()
		}
	}
	return nil
}

func newTestUseCase() (*UserUseCase, *mockRepo) {
	repo := newMockRepo()
	jwt := authn.NewJWTManager("test-secret", time.Hour)
	return NewUserUseCase(repo, jwt, nil, nil, func(string, ...any) {}), repo
}

func newTestUseCaseWithRedis(t *testing.T) (*UserUseCase, *mockRepo, *miniredis.Miniredis) {
	t.Helper()
	repo := newMockRepo()
	jwt := authn.NewJWTManager("test-secret", time.Hour)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewUserUseCase(repo, jwt, rdb, nil, func(string, ...any) {}), repo, mr
}

func TestRegister_Success(t *testing.T) {
	uc, _ := newTestUseCase()
	id, err := uc.Register(context.Background(), "alice@test.com", "123456", "Alice")
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestRegister_DuplicateEmail(t *testing.T) {
	uc, _ := newTestUseCase()
	_, err := uc.Register(context.Background(), "dup@test.com", "123456", "A")
	require.NoError(t, err)

	_, err = uc.Register(context.Background(), "dup@test.com", "654321", "B")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeConflict, be.Code)
}

func TestLogin_Success(t *testing.T) {
	uc, _ := newTestUseCase()
	_, err := uc.Register(context.Background(), "bob@test.com", "pass123", "Bob")
	require.NoError(t, err)

	token, expiresAt, userID, err := uc.Login(context.Background(), "bob@test.com", "pass123")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Greater(t, expiresAt, time.Now().Unix())
	assert.Greater(t, userID, int64(0))
}

func TestLogin_WrongPassword(t *testing.T) {
	uc, _ := newTestUseCase()
	_, err := uc.Register(context.Background(), "carol@test.com", "correct", "Carol")
	require.NoError(t, err)

	_, _, _, err = uc.Login(context.Background(), "carol@test.com", "wrong")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeUnauth, be.Code)
}

func TestLogin_UserNotFound(t *testing.T) {
	uc, _ := newTestUseCase()
	_, _, _, err := uc.Login(context.Background(), "nobody@test.com", "whatever")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeUnauth, be.Code)
}

func TestVerifyToken_Valid(t *testing.T) {
	uc, _ := newTestUseCase()
	_, err := uc.Register(context.Background(), "dave@test.com", "pass", "Dave")
	require.NoError(t, err)

	token, _, _, err := uc.Login(context.Background(), "dave@test.com", "pass")
	require.NoError(t, err)

	userID, email, err := uc.VerifyToken(context.Background(), token)
	require.NoError(t, err)
	assert.Greater(t, userID, int64(0))
	assert.Equal(t, "dave@test.com", email)
}

func TestVerifyToken_Invalid(t *testing.T) {
	uc, _ := newTestUseCase()
	_, _, err := uc.VerifyToken(context.Background(), "garbage-token")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeUnauth, be.Code)
}

func TestGetUser_Found(t *testing.T) {
	uc, _ := newTestUseCase()
	id, err := uc.Register(context.Background(), "eve@test.com", "pass", "Eve")
	require.NoError(t, err)

	user, err := uc.GetUser(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "eve@test.com", user.Email)
	assert.Equal(t, "Eve", user.Nickname)
}

func TestGetUser_NotFound(t *testing.T) {
	uc, _ := newTestUseCase()
	_, err := uc.GetUser(context.Background(), 9999)
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeNotFound, be.Code)
}

func TestUpdateProfile_Success(t *testing.T) {
	uc, _ := newTestUseCase()
	id, err := uc.Register(context.Background(), "profile@test.com", "pass123", "Before")
	require.NoError(t, err)

	user, err := uc.UpdateProfile(context.Background(), id, "After", "https://example.com/avatar.png")
	require.NoError(t, err)
	assert.Equal(t, "After", user.Nickname)
	assert.Equal(t, "https://example.com/avatar.png", user.AvatarURL)
}

// fakeAvatarStore 是头像存储的测试替身，记录入参并返回固定 URL。
type fakeAvatarStore struct {
	folder string
}

func (f *fakeAvatarStore) Save(_ context.Context, folder, _ string, _ []byte) (string, string, error) {
	f.folder = folder
	return folder + "/key.png", "/avatars/" + folder + "/key.png", nil
}

func TestUploadAvatar_Success(t *testing.T) {
	repo := newMockRepo()
	jwt := authn.NewJWTManager("test-secret", time.Hour)
	store := &fakeAvatarStore{}
	uc := NewUserUseCase(repo, jwt, nil, store, func(string, ...any) {})
	id, err := uc.Register(context.Background(), "avatar@test.com", "pass123", "头像用户")
	require.NoError(t, err)

	user, err := uc.UploadAvatar(context.Background(), id, "me.png", "image/png", []byte{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("avatars/%d", id), store.folder)
	assert.Equal(t, fmt.Sprintf("/avatars/avatars/%d/key.png", id), user.AvatarURL)
	assert.Equal(t, "头像用户", user.Nickname)
}

func TestUploadAvatar_RejectsNonImage(t *testing.T) {
	repo := newMockRepo()
	jwt := authn.NewJWTManager("test-secret", time.Hour)
	uc := NewUserUseCase(repo, jwt, nil, &fakeAvatarStore{}, func(string, ...any) {})
	id, err := uc.Register(context.Background(), "avatar2@test.com", "pass123", "头像用户")
	require.NoError(t, err)

	_, err = uc.UploadAvatar(context.Background(), id, "x.txt", "text/plain", []byte{1})
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeInvalidArg, be.Code)
}

func TestUploadAvatar_RejectsEmptyContent(t *testing.T) {
	repo := newMockRepo()
	jwt := authn.NewJWTManager("test-secret", time.Hour)
	uc := NewUserUseCase(repo, jwt, nil, &fakeAvatarStore{}, func(string, ...any) {})

	_, err := uc.UploadAvatar(context.Background(), 1, "x.png", "image/png", nil)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeInvalidArg, be.Code)
}

func TestCreateAddress_FirstAddressAutoDefault(t *testing.T) {
	uc, _ := newTestUseCase()
	userID, err := uc.Register(context.Background(), "address-first@test.com", "pass123", "Addr")
	require.NoError(t, err)

	address, err := uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "上海市",
		City:          "上海市",
		District:      "浦东新区",
		AddressLine:   "世纪大道1号",
		PostalCode:    "200120",
	})
	require.NoError(t, err)
	assert.True(t, address.IsDefault)
}

func TestListAddresses_ReturnsUserAddresses(t *testing.T) {
	uc, _ := newTestUseCase()
	userID, err := uc.Register(context.Background(), "address-list@test.com", "pass123", "Addr")
	require.NoError(t, err)
	_, err = uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "浙江省",
		City:          "杭州市",
		District:      "西湖区",
		AddressLine:   "文三路1号",
		PostalCode:    "310000",
	})
	require.NoError(t, err)
	_, err = uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "李四",
		ReceiverPhone: "13900139000",
		Province:      "江苏省",
		City:          "南京市",
		District:      "鼓楼区",
		AddressLine:   "中山路2号",
		PostalCode:    "210000",
		IsDefault:     true,
	})
	require.NoError(t, err)

	addresses, err := uc.ListAddresses(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, addresses, 2)
	assert.Equal(t, "李四", addresses[1].ReceiverName)
	assert.True(t, addresses[1].IsDefault)
	assert.False(t, addresses[0].IsDefault)
}

func TestSetDefaultAddress_ClearsPreviousDefault(t *testing.T) {
	uc, _ := newTestUseCase()
	userID, err := uc.Register(context.Background(), "address-default@test.com", "pass123", "Addr")
	require.NoError(t, err)
	first, err := uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "北京市",
		City:          "北京市",
		District:      "海淀区",
		AddressLine:   "知春路3号",
		PostalCode:    "100000",
	})
	require.NoError(t, err)
	second, err := uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "李四",
		ReceiverPhone: "13900139000",
		Province:      "北京市",
		City:          "北京市",
		District:      "朝阳区",
		AddressLine:   "建国路8号",
		PostalCode:    "100020",
	})
	require.NoError(t, err)

	address, err := uc.SetDefaultAddress(context.Background(), userID, second.ID)
	require.NoError(t, err)
	assert.True(t, address.IsDefault)

	firstAddress, err := uc.GetAddress(context.Background(), userID, first.ID)
	require.NoError(t, err)
	assert.False(t, firstAddress.IsDefault)
}

func TestUpdateAddress_Success(t *testing.T) {
	uc, _ := newTestUseCase()
	userID, err := uc.Register(context.Background(), "address-update@test.com", "pass123", "Addr")
	require.NoError(t, err)
	address, err := uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "广东省",
		City:          "深圳市",
		District:      "南山区",
		AddressLine:   "科技园1号",
		PostalCode:    "518000",
	})
	require.NoError(t, err)

	updated, err := uc.UpdateAddress(context.Background(), userID, address.ID, UpdateAddressCommand{
		ReceiverName:  "王五",
		ReceiverPhone: "13700137000",
		Province:      "广东省",
		City:          "广州市",
		District:      "天河区",
		AddressLine:   "体育西路9号",
		PostalCode:    "510000",
	})
	require.NoError(t, err)
	assert.Equal(t, "王五", updated.ReceiverName)
	assert.Equal(t, "广州市", updated.City)
}

func TestDeleteAddress_Success(t *testing.T) {
	uc, _ := newTestUseCase()
	userID, err := uc.Register(context.Background(), "address-delete@test.com", "pass123", "Addr")
	require.NoError(t, err)
	address, err := uc.CreateAddress(context.Background(), userID, CreateAddressCommand{
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "四川省",
		City:          "成都市",
		District:      "高新区",
		AddressLine:   "天府大道10号",
		PostalCode:    "610000",
	})
	require.NoError(t, err)

	err = uc.DeleteAddress(context.Background(), userID, address.ID)
	require.NoError(t, err)

	_, err = uc.GetAddress(context.Background(), userID, address.ID)
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeNotFound, be.Code)
}

func TestGetAddress_NotOwner(t *testing.T) {
	uc, _ := newTestUseCase()
	ownerID, err := uc.Register(context.Background(), "address-owner@test.com", "pass123", "Owner")
	require.NoError(t, err)
	otherID, err := uc.Register(context.Background(), "address-other@test.com", "pass123", "Other")
	require.NoError(t, err)
	address, err := uc.CreateAddress(context.Background(), ownerID, CreateAddressCommand{
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "湖北省",
		City:          "武汉市",
		District:      "洪山区",
		AddressLine:   "珞喻路6号",
		PostalCode:    "430000",
	})
	require.NoError(t, err)

	_, err = uc.GetAddress(context.Background(), otherID, address.ID)
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeForbidden, be.Code)
}

func TestSendPasswordResetCode_StoresCodeInRedis(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	id, err := uc.Register(context.Background(), "code@test.com", "pass123", "Code")
	require.NoError(t, err)

	err = uc.SendPasswordResetCode(context.Background(), id)
	require.NoError(t, err)

	key := fmt.Sprintf("user:password_reset_code:%d", id)
	assert.True(t, mr.Exists(key))
	code, err := mr.Get(key)
	require.NoError(t, err)
	assert.Len(t, code, 6)
}

func TestFindUserByEmail_FoundAndMissing(t *testing.T) {
	uc, _ := newTestUseCase()
	id, err := uc.Register(context.Background(), "byemail@test.com", "pass123", "ByEmail")
	require.NoError(t, err)

	found, err := uc.FindUserByEmail(context.Background(), "byemail@test.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, id, found.ID)

	missing, err := uc.FindUserByEmail(context.Background(), "nobody@test.com")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestPeekPasswordResetCode_ReturnsSentCode(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	id, err := uc.Register(context.Background(), "peek@test.com", "pass123", "Peek")
	require.NoError(t, err)
	require.NoError(t, uc.SendPasswordResetCode(context.Background(), id))

	key := fmt.Sprintf("user:password_reset_code:%d", id)
	stored, err := mr.Get(key)
	require.NoError(t, err)

	peeked, err := uc.PeekPasswordResetCode(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, stored, peeked)
}

func TestPeekPasswordResetCode_NotFoundWhenNoCode(t *testing.T) {
	uc, _, _ := newTestUseCaseWithRedis(t)

	_, err := uc.PeekPasswordResetCode(context.Background(), 9999)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeNotFound, be.Code)
}

func TestChangePassword_Success(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	id, err := uc.Register(context.Background(), "change@test.com", "oldpass", "Changer")
	require.NoError(t, err)

	key := fmt.Sprintf("user:password_reset_code:%d", id)
	require.NoError(t, mr.Set(key, "123456"))

	err = uc.ChangePassword(context.Background(), id, "123456", "newpass123")
	require.NoError(t, err)

	_, _, _, err = uc.Login(context.Background(), "change@test.com", "newpass123")
	require.NoError(t, err)
	assert.False(t, mr.Exists(key))
}

func TestChangePassword_InvalidCode(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	id, err := uc.Register(context.Background(), "invalid-code@test.com", "oldpass", "Changer")
	require.NoError(t, err)

	key := fmt.Sprintf("user:password_reset_code:%d", id)
	require.NoError(t, mr.Set(key, "123456"))

	err = uc.ChangePassword(context.Background(), id, "654321", "newpass123")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeInvalidArg, be.Code)
}

func TestChangePassword_SameAsOldPassword(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	id, err := uc.Register(context.Background(), "same-password@test.com", "oldpass", "Changer")
	require.NoError(t, err)

	key := fmt.Sprintf("user:password_reset_code:%d", id)
	require.NoError(t, mr.Set(key, "123456"))

	err = uc.ChangePassword(context.Background(), id, "123456", "oldpass")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeInvalidArg, be.Code)
}

func TestSendPasswordResetCodeByEmail_StoresCodeInRedis(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	_, err := uc.Register(context.Background(), "forgot-code@test.com", "pass123", "Forgot")
	require.NoError(t, err)

	err = uc.SendPasswordResetCodeByEmail(context.Background(), "forgot-code@test.com")
	require.NoError(t, err)

	assert.True(t, mr.Exists("user:password_reset_code_email:forgot-code@test.com"))
	code, err := mr.Get("user:password_reset_code_email:forgot-code@test.com")
	require.NoError(t, err)
	assert.Len(t, code, 6)
}

func TestResetPasswordByEmail_Success(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	_, err := uc.Register(context.Background(), "forgot-reset@test.com", "oldpass", "Forgot")
	require.NoError(t, err)

	require.NoError(t, mr.Set("user:password_reset_code_email:forgot-reset@test.com", "123456"))

	err = uc.ResetPasswordByEmail(context.Background(), "forgot-reset@test.com", "123456", "newpass123")
	require.NoError(t, err)

	_, _, _, err = uc.Login(context.Background(), "forgot-reset@test.com", "newpass123")
	require.NoError(t, err)
	assert.False(t, mr.Exists("user:password_reset_code_email:forgot-reset@test.com"))
}

func TestResetPasswordByEmail_SameAsOldPassword(t *testing.T) {
	uc, _, mr := newTestUseCaseWithRedis(t)
	_, err := uc.Register(context.Background(), "forgot-same@test.com", "oldpass", "Forgot")
	require.NoError(t, err)

	require.NoError(t, mr.Set("user:password_reset_code_email:forgot-same@test.com", "123456"))

	err = uc.ResetPasswordByEmail(context.Background(), "forgot-same@test.com", "123456", "oldpass")
	require.Error(t, err)
	var be *errors.Error
	require.ErrorAs(t, err, &be)
	assert.Equal(t, errors.CodeInvalidArg, be.Code)
}
