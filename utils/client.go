package utils

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"time"

	"kiro2api/config"
)

var (
	// SharedHTTPClient 共享的HTTP客户端实例，优化了连接池和性能配置
	SharedHTTPClient *http.Client
)

func init() {
	SharedHTTPClient = buildSharedHTTPClient()
}

// shouldSkipTLSVerify 根据GIN_MODE决定是否跳过TLS证书验证
func shouldSkipTLSVerify() bool {
	return os.Getenv("GIN_MODE") == "debug"
}

// DoRequest 执行HTTP请求
func DoRequest(req *http.Request) (*http.Response, error) {
	return SharedHTTPClient.Do(req)
}

func buildSharedHTTPClient() *http.Client {
	skipTLS := shouldSkipTLSVerify()
	if skipTLS {
		os.Stderr.WriteString("[WARNING] TLS证书验证已禁用 - 仅适用于开发/调试环境\n")
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: skipTLS}
	applyTLSProfile(tlsConfig)

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: config.HTTPClientKeepAlive,
			DualStack: true,
		}).DialContext,

		TLSHandshakeTimeout: config.HTTPClientTLSHandshakeTimeout,
		TLSClientConfig:     tlsConfig,
		ForceAttemptHTTP2:   resolveHTTP2Preference(),
		DisableCompression:  false,
	}

	return &http.Client{Transport: transport}
}

func applyTLSProfile(cfg *tls.Config) {
	if !config.IsStealthModeEnabled() {
		cfg.MinVersion = tls.VersionTLS12
		cfg.MaxVersion = tls.VersionTLS13
		cfg.CipherSuites = []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_128_GCM_SHA256,
		}
		return
	}

	if RandomBool() {
		cfg.MinVersion = tls.VersionTLS13
		cfg.MaxVersion = tls.VersionTLS13
	} else {
		cfg.MinVersion = tls.VersionTLS12
		cfg.MaxVersion = tls.VersionTLS13
	}

	cipherPool := []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}

	ShuffleUint16(cipherPool)

	maxSuites := int(RandomIntBetween(4, int64(len(cipherPool))))
	cfg.CipherSuites = append([]uint16{}, cipherPool[:maxSuites]...)
}

func resolveHTTP2Preference() bool {
	mode := config.HTTP2Mode()
	switch mode {
	case config.HTTP2ModeForce:
		return true
	case config.HTTP2ModeDisable:
		return false
	default:
		if config.IsStealthModeEnabled() {
			return RandomBool()
		}
		return false
	}
}
