package ginblog

import (
	"context"
	"fmt"
	g "gin-blog/internal/global"
	"gin-blog/internal/model"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// 根据配置文件初始化 slog 日志
func InitLogger(conf *g.Config) *slog.Logger {
	var level slog.Level
	switch conf.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	option := &slog.HandlerOptions{
		AddSource: false, // TODO: 外层
		Level:     level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format(time.DateTime))
				}
			}
			return a
		},
	}

	var handler slog.Handler
	var output io.Writer

	// 根据配置决定输出到文件还是控制台
	if conf.Log.Directory != "" {
		// 确保日志目录存在
		if err := os.MkdirAll(conf.Log.Directory, 0755); err != nil {
			// 这里还不能使用 slog，因为 logger 还没有初始化
			fmt.Printf("创建日志目录失败: %v, 将使用控制台输出\n", err)
			output = os.Stdout
		} else {
			// 创建日志文件，使用当前日期作为文件名
			logFile := filepath.Join(conf.Log.Directory, time.Now().Format("2006-01-02")+".log")
			file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				// 这里还不能使用 slog，因为 logger 还没有初始化
				fmt.Printf("创建日志文件失败: %v, 将使用控制台输出\n", err)
				output = os.Stdout
			} else {
				output = file
			}
		}
	} else {
		output = os.Stdout
	}

	switch conf.Log.Format {
	case "json":
		handler = slog.NewJSONHandler(output, option)
	case "text":
		fallthrough
	default:
		handler = slog.NewTextHandler(output, option)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// 根据配置文件初始化数据库
func InitDatabase(conf *g.Config) *gorm.DB {
	dbtype := conf.DbType()
	dsn := conf.DbDSN()

	var db *gorm.DB
	var err error

	var level logger.LogLevel
	switch conf.Server.DbLogMode {
	case "silent":
		level = logger.Silent
	case "info":
		level = logger.Info
	case "warn":
		level = logger.Warn
	case "error":
		fallthrough
	default:
		level = logger.Error
	}

	config := &gorm.Config{
		Logger:                                   logger.Default.LogMode(level),
		DisableForeignKeyConstraintWhenMigrating: true, // 禁用外键约束
		SkipDefaultTransaction:                   true, // 禁用默认事务（提高运行速度）
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 单数表名
		},
	}

	switch dbtype {
	case "mysql":
		db, err = gorm.Open(mysql.Open(dsn), config)
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(dsn), config)
	default:
		log.Fatal("不支持的数据库类型: ", dbtype)
	}

	if err != nil {
		log.Fatal("数据库连接失败", err)
	}
	slog.Info("数据库连接成功", "type", dbtype, "dsn", dsn)

	if conf.Server.DbAutoMigrate {
		if err := model.MakeMigrate(db); err != nil {
			log.Fatal("数据库迁移失败", err)
		}
		slog.Info("数据库自动迁移成功")
	}

	return db
}

// 根据配置文件初始化 Redis
func InitRedis(conf *g.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     conf.Redis.Addr,
		Password: conf.Redis.Password,
		DB:       conf.Redis.DB,
	})

	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.Fatal("Redis 连接失败: ", err)
	}

	slog.Info("Redis 连接成功", "addr", conf.Redis.Addr, "db", conf.Redis.DB, "password", conf.Redis.Password)
	return rdb
}
