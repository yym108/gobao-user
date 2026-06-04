// Package grpc 提供 User 服务的 gRPC Handler 实现。
package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yym108/gobao-pkg/errors"
	userv1 "github.com/yym108/gobao-proto/gen/go/gobao/user/v1"
	"github.com/yym108/gobao-user/internal/application"
	"github.com/yym108/gobao-user/internal/domain"
)

// UserHandler 实现 proto 生成的 UserServiceServer 接口。
// 嵌入 UnimplementedUserServiceServer 确保向前兼容（proto 新增 RPC 不会编译失败）。
type UserHandler struct {
	userv1.UnimplementedUserServiceServer                          // 向前兼容嵌入
	uc                                    *application.UserUseCase // 用例编排器
}

// NewUserHandler 构造 gRPC Handler。
//   - uc: 用例编排器，由 main.go 注入
func NewUserHandler(uc *application.UserUseCase) *UserHandler {
	return &UserHandler{uc: uc}
}

// Register 处理用户注册 RPC。
// 入参校验：email 非空、password >= 6 位。
// 错误映射：CodeConflict → AlreadyExists，CodeInternal → Internal。
func (h *UserHandler) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.RegisterResponse, error) {
	if req.GetEmail() == "" {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "email is required")).Err()
	}
	if len(req.GetPassword()) < 6 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "password must be at least 6 characters")).Err()
	}

	id, err := h.uc.Register(ctx, req.GetEmail(), req.GetPassword(), req.GetNickname())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err() // 统一错误码 → gRPC status 映射
	}
	return &userv1.RegisterResponse{UserId: id}, nil
}

// Login 处理用户登录 RPC。
// 入参校验：email 和 password 非空。
// 成功返回 JWT token、过期时间、用户 ID。
func (h *UserHandler) Login(ctx context.Context, req *userv1.LoginRequest) (*userv1.LoginResponse, error) {
	if req.GetEmail() == "" {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "email is required")).Err()
	}
	if req.GetPassword() == "" {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "password is required")).Err()
	}

	token, expiresAt, userID, err := h.uc.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.LoginResponse{
		AccessToken: token,
		ExpiresAt:   expiresAt,
		UserId:      userID,
	}, nil
}

// VerifyToken 处理 token 校验 RPC。
// 供其他微服务（如 Order）调用验证 token，网关本身通过本地 JWTManager 校验。
func (h *UserHandler) VerifyToken(ctx context.Context, req *userv1.VerifyTokenRequest) (*userv1.VerifyTokenResponse, error) {
	if req.GetAccessToken() == "" {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "access_token is required")).Err()
	}

	userID, email, err := h.uc.VerifyToken(ctx, req.GetAccessToken())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.VerifyTokenResponse{UserId: userID, Email: email}, nil
}

// GetUser 处理获取用户信息 RPC。
// 入参校验：user_id 必须为正数。
// CreatedAt 通过 timestamppb.New() 转换为 proto Timestamp 类��。
func (h *UserHandler) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	user, err := h.uc.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.GetUserResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}, nil
}

// FindUserByEmail 处理按邮箱查找用户 RPC，未找到时返回 found=false 而非错误。
func (h *UserHandler) FindUserByEmail(ctx context.Context, req *userv1.FindUserByEmailRequest) (*userv1.FindUserByEmailResponse, error) {
	user, err := h.uc.FindUserByEmail(ctx, req.GetEmail())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	if user == nil {
		return &userv1.FindUserByEmailResponse{Found: false}, nil
	}
	return &userv1.FindUserByEmailResponse{
		Found:    true,
		UserId:   user.ID,
		Email:    user.Email,
		Nickname: user.Nickname,
	}, nil
}

// GetProfile 处理获取当前用户可编辑资料 RPC。
func (h *UserHandler) GetProfile(ctx context.Context, req *userv1.GetProfileRequest) (*userv1.GetProfileResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	user, err := h.uc.GetUser(ctx, req.GetUserId())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.GetProfileResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarURL,
	}, nil
}

// UpdateProfile 处理更新昵称与头像地址 RPC。
func (h *UserHandler) UpdateProfile(ctx context.Context, req *userv1.UpdateProfileRequest) (*userv1.UpdateProfileResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	user, err := h.uc.UpdateProfile(ctx, req.GetUserId(), req.GetNickname(), req.GetAvatarUrl())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.UpdateProfileResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarURL,
	}, nil
}

// UploadAvatar 处理头像上传 RPC，保存文件并回写头像地址。
func (h *UserHandler) UploadAvatar(ctx context.Context, req *userv1.UploadAvatarRequest) (*userv1.UploadAvatarResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	user, err := h.uc.UploadAvatar(ctx, req.GetUserId(), req.GetFileName(), req.GetMimeType(), req.GetContent())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.UploadAvatarResponse{
		UserId:    user.ID,
		Email:     user.Email,
		Nickname:  user.Nickname,
		AvatarUrl: user.AvatarURL,
	}, nil
}

// ListAddresses 返回当前用户地址簿列表。
func (h *UserHandler) ListAddresses(ctx context.Context, req *userv1.ListAddressesRequest) (*userv1.ListAddressesResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	addresses, err := h.uc.ListAddresses(ctx, req.GetUserId())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}

	resp := &userv1.ListAddressesResponse{Addresses: make([]*userv1.Address, 0, len(addresses))}
	for _, address := range addresses {
		resp.Addresses = append(resp.Addresses, toProtoAddress(address))
	}
	return resp, nil
}

// GetAddress 返回当前用户的一条地址详情。
func (h *UserHandler) GetAddress(ctx context.Context, req *userv1.GetAddressRequest) (*userv1.GetAddressResponse, error) {
	if req.GetUserId() <= 0 || req.GetAddressId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id and address_id must be positive")).Err()
	}

	address, err := h.uc.GetAddress(ctx, req.GetUserId(), req.GetAddressId())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.GetAddressResponse{Address: toProtoAddress(address)}, nil
}

// CreateAddress 创建当前用户地址。
func (h *UserHandler) CreateAddress(ctx context.Context, req *userv1.CreateAddressRequest) (*userv1.CreateAddressResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	address, err := h.uc.CreateAddress(ctx, req.GetUserId(), application.CreateAddressCommand{
		ReceiverName:  req.GetReceiverName(),
		ReceiverPhone: req.GetReceiverPhone(),
		Province:      req.GetProvince(),
		City:          req.GetCity(),
		District:      req.GetDistrict(),
		AddressLine:   req.GetAddressLine(),
		PostalCode:    req.GetPostalCode(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.CreateAddressResponse{Address: toProtoAddress(address)}, nil
}

// UpdateAddress 更新当前用户地址。
func (h *UserHandler) UpdateAddress(ctx context.Context, req *userv1.UpdateAddressRequest) (*userv1.UpdateAddressResponse, error) {
	if req.GetUserId() <= 0 || req.GetAddressId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id and address_id must be positive")).Err()
	}

	address, err := h.uc.UpdateAddress(ctx, req.GetUserId(), req.GetAddressId(), application.UpdateAddressCommand{
		ReceiverName:  req.GetReceiverName(),
		ReceiverPhone: req.GetReceiverPhone(),
		Province:      req.GetProvince(),
		City:          req.GetCity(),
		District:      req.GetDistrict(),
		AddressLine:   req.GetAddressLine(),
		PostalCode:    req.GetPostalCode(),
		IsDefault:     req.GetIsDefault(),
	})
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.UpdateAddressResponse{Address: toProtoAddress(address)}, nil
}

// DeleteAddress 删除当前用户地址。
func (h *UserHandler) DeleteAddress(ctx context.Context, req *userv1.DeleteAddressRequest) (*userv1.DeleteAddressResponse, error) {
	if req.GetUserId() <= 0 || req.GetAddressId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id and address_id must be positive")).Err()
	}

	if err := h.uc.DeleteAddress(ctx, req.GetUserId(), req.GetAddressId()); err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.DeleteAddressResponse{Message: "address deleted"}, nil
}

// SetDefaultAddress 将指定地址设置为默认地址。
func (h *UserHandler) SetDefaultAddress(ctx context.Context, req *userv1.SetDefaultAddressRequest) (*userv1.SetDefaultAddressResponse, error) {
	if req.GetUserId() <= 0 || req.GetAddressId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id and address_id must be positive")).Err()
	}

	address, err := h.uc.SetDefaultAddress(ctx, req.GetUserId(), req.GetAddressId())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.SetDefaultAddressResponse{Address: toProtoAddress(address)}, nil
}

// SendPasswordResetCode 处理发送密码验证码 RPC。
func (h *UserHandler) SendPasswordResetCode(ctx context.Context, req *userv1.SendPasswordResetCodeRequest) (*userv1.SendPasswordResetCodeResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	if err := h.uc.SendPasswordResetCode(ctx, req.GetUserId()); err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.SendPasswordResetCodeResponse{Message: "verification code sent"}, nil
}

// GetPasswordResetCode 读取当前用户待用的改密验证码（仅开发/演示环境）。
func (h *UserHandler) GetPasswordResetCode(ctx context.Context, req *userv1.GetPasswordResetCodeRequest) (*userv1.GetPasswordResetCodeResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	code, err := h.uc.PeekPasswordResetCode(ctx, req.GetUserId())
	if err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.GetPasswordResetCodeResponse{Code: code}, nil
}

// ChangePassword 处理验证码改密 RPC。
func (h *UserHandler) ChangePassword(ctx context.Context, req *userv1.ChangePasswordRequest) (*userv1.ChangePasswordResponse, error) {
	if req.GetUserId() <= 0 {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "user_id must be positive")).Err()
	}

	if err := h.uc.ChangePassword(ctx, req.GetUserId(), req.GetCode(), req.GetNewPassword()); err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.ChangePasswordResponse{Message: "password changed"}, nil
}

// SendPasswordResetCodeByEmail 处理未登录找回密码的发码 RPC。
func (h *UserHandler) SendPasswordResetCodeByEmail(ctx context.Context, req *userv1.SendPasswordResetCodeByEmailRequest) (*userv1.SendPasswordResetCodeByEmailResponse, error) {
	if req.GetEmail() == "" {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "email is required")).Err()
	}

	if err := h.uc.SendPasswordResetCodeByEmail(ctx, req.GetEmail()); err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.SendPasswordResetCodeByEmailResponse{Message: "verification code sent"}, nil
}

// ResetPasswordByEmail 处理未登录找回密码的重置 RPC。
func (h *UserHandler) ResetPasswordByEmail(ctx context.Context, req *userv1.ResetPasswordByEmailRequest) (*userv1.ResetPasswordByEmailResponse, error) {
	if req.GetEmail() == "" {
		return nil, errors.ToGRPCStatus(errors.New(errors.CodeInvalidArg, "email is required")).Err()
	}

	if err := h.uc.ResetPasswordByEmail(ctx, req.GetEmail(), req.GetCode(), req.GetNewPassword()); err != nil {
		return nil, errors.ToGRPCStatus(err).Err()
	}
	return &userv1.ResetPasswordByEmailResponse{Message: "password changed"}, nil
}

// toProtoAddress 将领域地址对象转换为 proto 地址对象。
func toProtoAddress(address *domain.Address) *userv1.Address {
	if address == nil {
		return nil
	}
	return &userv1.Address{
		Id:            address.ID,
		UserId:        address.UserID,
		ReceiverName:  address.ReceiverName,
		ReceiverPhone: address.ReceiverPhone,
		Province:      address.Province,
		City:          address.City,
		District:      address.District,
		AddressLine:   address.AddressLine,
		PostalCode:    address.PostalCode,
		IsDefault:     address.IsDefault,
		CreatedAt:     timestamppb.New(address.CreatedAt),
		UpdatedAt:     timestamppb.New(address.UpdatedAt),
	}
}
