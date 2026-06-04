package main

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/yym108/gobao-pkg/authn"
	"github.com/yym108/gobao-pkg/cache"
	pkgcfg "github.com/yym108/gobao-pkg/config"
	"github.com/yym108/gobao-pkg/grpcx"
	"github.com/yym108/gobao-pkg/logger"
	"github.com/yym108/gobao-pkg/server"
	userv1 "github.com/yym108/gobao-proto/gen/go/gobao/user/v1"

	usergrpc "github.com/yym108/gobao-user/internal/adapter/grpc"
	mysqlrepo "github.com/yym108/gobao-user/internal/adapter/repository/mysql"
	localstore "github.com/yym108/gobao-user/internal/adapter/storage/local"
	"github.com/yym108/gobao-user/internal/application"
	"github.com/yym108/gobao-user/internal/config"
)

func main() {
	// 1. 加载配置：默认值适用于 Docker Compose 环境，环境变量 USER_* 覆盖
	cfg := config.Config{
		HTTPAddr:      ":8080",
		GRPCAddr:      ":9090",
		LogLevel:      "info",
		MySQLDSN:      "root:root@tcp(mysql-user:3306)/user?charset=utf8mb4&parseTime=True&loc=Local",
		RedisAddr:     "redis:6379",
		JWTSecret:     "gobao-dev-secret-change-in-prod",
		JWTExpiry:     "24h",
		AvatarRoot:    "/data/avatars",
		AvatarBaseURL: "/avatars",
	}
	if err := pkgcfg.Load("USER", "", &cfg); err != nil {
		panic("load user config: " + err.Error())
	}

	// 2. 初始化日志
	log := logger.New("user", cfg.LogLevel)
	defer func() { _ = log.Sync() }()

	// 3. 连接 MySQL（带重试，等待 MySQL 就绪后再继续）
	var (
		db  *gorm.DB
		err error
	)
	for i := 1; i <= 30; i++ {
		db, err = gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{})
		if err == nil {
			sqlDB, pingErr := db.DB()
			if pingErr == nil {
				pingErr = sqlDB.Ping()
			}
			if pingErr == nil {
				break
			}
			err = pingErr
		}
		log.Warn("mysql not ready, retrying " + fmt.Sprintf("%d/30", i) + ": " + err.Error())
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatal("mysql connect failed after 30 retries: " + err.Error())
	}
	if err := db.AutoMigrate(&mysqlrepo.UserModel{}, &mysqlrepo.AddressModel{}); err != nil {
		log.Fatal("auto migrate failed: " + err.Error())
	}

	// 4. 解析 JWT 过期时间字符串（如 "24h"），解析失败回退为 24 小时
	jwtExpiry, err := time.ParseDuration(cfg.JWTExpiry)
	if err != nil {
		jwtExpiry = 24 * time.Hour
	}

	// 5. 初始化 Redis，用于暂存密码验证码与发送限流状态。
	rdb, err := cache.NewClient(cache.Config{
		Addr: cfg.RedisAddr,
		DB:   0,
	})
	if err != nil {
		log.Fatal("redis connect failed: " + err.Error())
	}
	defer func() { _ = rdb.Close() }()

	// 6. 依赖装配链：Repo → JWTManager/Redis → UseCase → gRPC Handler
	repo := mysqlrepo.NewUserRepo(db)
	jwtMgr := authn.NewJWTManager(cfg.JWTSecret, jwtExpiry)
	avatarStore := localstore.NewAvatarStore(cfg.AvatarRoot, cfg.AvatarBaseURL)
	uc := application.NewUserUseCase(repo, jwtMgr, rdb, avatarStore, func(format string, args ...any) {
		log.Info(fmt.Sprintf(format, args...))
	})
	handler := usergrpc.NewUserHandler(uc)

	// 7. 注册信号监听，SIGINT/SIGTERM 触发优雅关停
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 8. 创建并启动 gRPC + HTTP 双协议服务
	s := server.New("user", server.Options{
		HTTPAddr: cfg.HTTPAddr,
		GRPCAddr: cfg.GRPCAddr,
		GRPCOpts: []grpc.ServerOption{
			// 拦截器链：TraceID 透传 → Recover 捕获 panic
			grpc.ChainUnaryInterceptor(grpcx.TraceID(), grpcx.Recover()),
		},
		Register: func(gs *grpc.Server) {
			userv1.RegisterUserServiceServer(gs, handler) // 注册 User gRPC 服务
		},
		HTTPRegister: func(mux *http.ServeMux) {
			// 对外暴露头像静态文件，路径前缀与落库的 avatar_url 一致
			usergrpc.RegisterAvatarHTTP(mux, cfg.AvatarBaseURL, cfg.AvatarRoot)
		},
	})

	log.Info("starting user service")
	if err := s.Run(ctx); err != nil {
		log.Fatal(err.Error())
	}
}
