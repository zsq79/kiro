package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"kiro2api/logger"
)

const (
	// DefaultConfigDir 默认配置目录（会挂载到volume）
	DefaultConfigDir = "/app/data"
	// ConfigFileName 配置文件名
	ConfigFileName = "tokens.json"
)

// ConfigStorage 配置持久化存储
type ConfigStorage struct {
	filePath string
	mutex    sync.RWMutex
}

// NewConfigStorage 创建配置存储
func NewConfigStorage() *ConfigStorage {
	// 确保配置目录存在
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	
	// 创建目录（如果不存在）
	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.Warn("创建配置目录失败，使用当前目录", 
			logger.String("dir", configDir),
			logger.Err(err))
		configDir = "."
	}
	
	filePath := filepath.Join(configDir, ConfigFileName)
	
	return &ConfigStorage{
		filePath: filePath,
	}
}

// Load 从文件加载配置
func (cs *ConfigStorage) Load() ([]AuthConfig, error) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	
	// 检查文件是否存在
	if _, err := os.Stat(cs.filePath); os.IsNotExist(err) {
		logger.Info("持久化配置文件不存在，将从环境变量加载",
			logger.String("file", cs.filePath))
		return nil, nil // 返回nil表示需要从环境变量加载
	}
	
	// 读取文件
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		logger.Warn("读取持久化配置失败",
			logger.String("file", cs.filePath),
			logger.Err(err))
		return nil, err
	}
	
	// 解析JSON
	var configs []AuthConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	
	logger.Info("从持久化文件加载配置成功",
		logger.String("file", cs.filePath),
		logger.Int("count", len(configs)))
	
	return configs, nil
}

// Save 保存配置到文件
func (cs *ConfigStorage) Save(configs []AuthConfig) error {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()
	
	// 序列化为格式化的JSON
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	
	// 写入文件
	if err := os.WriteFile(cs.filePath, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	
	logger.Info("配置已保存到持久化文件",
		logger.String("file", cs.filePath),
		logger.Int("count", len(configs)))
	
	return nil
}

