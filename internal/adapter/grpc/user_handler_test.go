package grpc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/yym108/gobao-pkg/authn"
	userv1 "github.com/yym108/gobao-proto/gen/go/gobao/user/v1"
	"github.com/yym108/gobao-user/internal/application"
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
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.byEmail[email]; ok {
		return u, nil
	}
	return nil, nil
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
	if u, ok := m.users[userID]; ok {
		u.Nickname = nickname
		u.AvatarURL = avatarURL
		u.UpdatedAt = time.Now()
	}
	return nil
}

func (m *mockRepo) UpdatePasswordHash(_ context.Context, userID int64, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[userID]; ok {
		u.PasswordHash = passwordHash
		u.UpdatedAt = time.Now()
	}
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

func setupBufconn(t *testing.T) userv1.UserServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	repo := newMockRepo()
	jwtMgr := authn.NewJWTManager("test-secret", time.Hour)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	uc := application.NewUserUseCase(repo, jwtMgr, rdb, nil, func(string, ...any) {})
	handler := NewUserHandler(uc)

	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, handler)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { srv.Stop() })

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return userv1.NewUserServiceClient(conn)
}

func TestGRPC_Register_Success(t *testing.T) {
	client := setupBufconn(t)
	resp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "alice@test.com", Password: "123456", Nickname: "Alice",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.GetUserId(), int64(0))
}

func TestGRPC_Register_InvalidEmail(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "", Password: "123456",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGRPC_Login_Success(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "bob@test.com", Password: "pass123", Nickname: "Bob",
	})
	require.NoError(t, err)

	resp, err := client.Login(context.Background(), &userv1.LoginRequest{
		Email: "bob@test.com", Password: "pass123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetAccessToken())
	assert.Greater(t, resp.GetUserId(), int64(0))
}

func TestGRPC_Login_WrongPassword(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "carol@test.com", Password: "correct", Nickname: "Carol",
	})
	require.NoError(t, err)

	_, err = client.Login(context.Background(), &userv1.LoginRequest{
		Email: "carol@test.com", Password: "wrong",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGRPC_VerifyToken_Valid(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "dave@test.com", Password: "pass123", Nickname: "Dave",
	})
	require.NoError(t, err)

	loginResp, err := client.Login(context.Background(), &userv1.LoginRequest{
		Email: "dave@test.com", Password: "pass123",
	})
	require.NoError(t, err)

	resp, err := client.VerifyToken(context.Background(), &userv1.VerifyTokenRequest{
		AccessToken: loginResp.GetAccessToken(),
	})
	require.NoError(t, err)
	assert.Equal(t, loginResp.GetUserId(), resp.GetUserId())
	assert.Equal(t, "dave@test.com", resp.GetEmail())
}

func TestGRPC_VerifyToken_Invalid(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.VerifyToken(context.Background(), &userv1.VerifyTokenRequest{
		AccessToken: "invalid-token",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGRPC_GetUser_NotFound(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.GetUser(context.Background(), &userv1.GetUserRequest{UserId: 9999})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGRPC_GetProfile_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "profile@test.com", Password: "pass123", Nickname: "Profile",
	})
	require.NoError(t, err)

	resp, err := client.GetProfile(context.Background(), &userv1.GetProfileRequest{
		UserId: registerResp.GetUserId(),
	})
	require.NoError(t, err)
	assert.Equal(t, "profile@test.com", resp.GetEmail())
	assert.Equal(t, "Profile", resp.GetNickname())
	assert.Equal(t, "", resp.GetAvatarUrl())
}

func TestGRPC_UpdateProfile_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "update@test.com", Password: "pass123", Nickname: "Before",
	})
	require.NoError(t, err)

	resp, err := client.UpdateProfile(context.Background(), &userv1.UpdateProfileRequest{
		UserId:    registerResp.GetUserId(),
		Nickname:  "After",
		AvatarUrl: "https://example.com/avatar.png",
	})
	require.NoError(t, err)
	assert.Equal(t, "After", resp.GetNickname())
	assert.Equal(t, "https://example.com/avatar.png", resp.GetAvatarUrl())
}

func TestGRPC_SendPasswordResetCode_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "code@test.com", Password: "pass123", Nickname: "Code",
	})
	require.NoError(t, err)

	_, err = client.SendPasswordResetCode(context.Background(), &userv1.SendPasswordResetCodeRequest{
		UserId: registerResp.GetUserId(),
	})
	require.NoError(t, err)
}

func TestGRPC_ChangePassword_InvalidCode(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "change@test.com", Password: "pass123", Nickname: "Change",
	})
	require.NoError(t, err)

	_, err = client.ChangePassword(context.Background(), &userv1.ChangePasswordRequest{
		UserId:      registerResp.GetUserId(),
		Code:        "111111",
		NewPassword: "newpass123",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGRPC_SendPasswordResetCodeByEmail_Success(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "forgot-code@test.com", Password: "pass123", Nickname: "Forgot",
	})
	require.NoError(t, err)

	_, err = client.SendPasswordResetCodeByEmail(context.Background(), &userv1.SendPasswordResetCodeByEmailRequest{
		Email: "forgot-code@test.com",
	})
	require.NoError(t, err)
}

func TestGRPC_ResetPasswordByEmail_InvalidCode(t *testing.T) {
	client := setupBufconn(t)
	_, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "forgot-reset@test.com", Password: "pass123", Nickname: "Forgot",
	})
	require.NoError(t, err)

	_, err = client.ResetPasswordByEmail(context.Background(), &userv1.ResetPasswordByEmailRequest{
		Email:       "forgot-reset@test.com",
		Code:        "111111",
		NewPassword: "newpass123",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGRPC_CreateAddress_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "address-create@test.com", Password: "pass123", Nickname: "Addr",
	})
	require.NoError(t, err)

	resp, err := client.CreateAddress(context.Background(), &userv1.CreateAddressRequest{
		UserId:        registerResp.GetUserId(),
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "上海市",
		City:          "上海市",
		District:      "浦东新区",
		AddressLine:   "世纪大道1号",
		PostalCode:    "200120",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.GetAddress().GetId(), int64(0))
	assert.True(t, resp.GetAddress().GetIsDefault())
}

func TestGRPC_ListAddresses_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "address-list@test.com", Password: "pass123", Nickname: "Addr",
	})
	require.NoError(t, err)
	_, err = client.CreateAddress(context.Background(), &userv1.CreateAddressRequest{
		UserId:        registerResp.GetUserId(),
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "上海市",
		City:          "上海市",
		District:      "浦东新区",
		AddressLine:   "世纪大道1号",
		PostalCode:    "200120",
	})
	require.NoError(t, err)

	resp, err := client.ListAddresses(context.Background(), &userv1.ListAddressesRequest{
		UserId: registerResp.GetUserId(),
	})
	require.NoError(t, err)
	require.Len(t, resp.GetAddresses(), 1)
	assert.Equal(t, "张三", resp.GetAddresses()[0].GetReceiverName())
}

func TestGRPC_SetDefaultAddress_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "address-default@test.com", Password: "pass123", Nickname: "Addr",
	})
	require.NoError(t, err)
	first, err := client.CreateAddress(context.Background(), &userv1.CreateAddressRequest{
		UserId:        registerResp.GetUserId(),
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "北京市",
		City:          "北京市",
		District:      "海淀区",
		AddressLine:   "知春路3号",
		PostalCode:    "100000",
	})
	require.NoError(t, err)
	second, err := client.CreateAddress(context.Background(), &userv1.CreateAddressRequest{
		UserId:        registerResp.GetUserId(),
		ReceiverName:  "李四",
		ReceiverPhone: "13900139000",
		Province:      "北京市",
		City:          "北京市",
		District:      "朝阳区",
		AddressLine:   "建国路8号",
		PostalCode:    "100020",
	})
	require.NoError(t, err)

	resp, err := client.SetDefaultAddress(context.Background(), &userv1.SetDefaultAddressRequest{
		UserId:    registerResp.GetUserId(),
		AddressId: second.GetAddress().GetId(),
	})
	require.NoError(t, err)
	assert.True(t, resp.GetAddress().GetIsDefault())

	firstResp, err := client.GetAddress(context.Background(), &userv1.GetAddressRequest{
		UserId:    registerResp.GetUserId(),
		AddressId: first.GetAddress().GetId(),
	})
	require.NoError(t, err)
	assert.False(t, firstResp.GetAddress().GetIsDefault())
}

func TestGRPC_DeleteAddress_Success(t *testing.T) {
	client := setupBufconn(t)
	registerResp, err := client.Register(context.Background(), &userv1.RegisterRequest{
		Email: "address-delete@test.com", Password: "pass123", Nickname: "Addr",
	})
	require.NoError(t, err)
	address, err := client.CreateAddress(context.Background(), &userv1.CreateAddressRequest{
		UserId:        registerResp.GetUserId(),
		ReceiverName:  "张三",
		ReceiverPhone: "13800138000",
		Province:      "四川省",
		City:          "成都市",
		District:      "高新区",
		AddressLine:   "天府大道10号",
		PostalCode:    "610000",
	})
	require.NoError(t, err)

	_, err = client.DeleteAddress(context.Background(), &userv1.DeleteAddressRequest{
		UserId:    registerResp.GetUserId(),
		AddressId: address.GetAddress().GetId(),
	})
	require.NoError(t, err)

	_, err = client.GetAddress(context.Background(), &userv1.GetAddressRequest{
		UserId:    registerResp.GetUserId(),
		AddressId: address.GetAddress().GetId(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}
