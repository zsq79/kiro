package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kiro2api/config"
	"kiro2api/utils"
)

type HeaderManager struct {
	stealthEnabled bool
	strategy       string
}

type agentProfile struct {
	userAgent     string
	xAmzUserAgent string
	agentMode     string
	acceptLang    string
	kiroMachineID string // 用于 Telemetry 请求
}

// Kiro IDE 真实请求头模板（基于实际抓包数据）
var kiroAgentProfiles = []agentProfile{
	// 这些是模拟真实 Kiro IDE 的配置文件
	// 格式：user-agent 和 x-amz-user-agent 都包含 KiroIDE-版本号-哈希值
	{"", "", "vibe", "en-US,en;q=0.9", ""},
	{"", "", "vibe", "en-US,en;q=0.8,zh-CN;q=0.6", ""},
	{"", "", "vibe", "en-US,en;q=0.85", ""},
	{"", "", "vibe", "en-US,en;q=0.7,ja-JP;q=0.3", ""},
}

// AWS 官方工具包的请求头模板（原有的）
var realAgentProfiles = []agentProfile{
	{"aws-toolkit-vscode/1.84.0 VisualStudioCode/1.96.2 Windows_NT/10.0.22631 NodeJS/20.10.0", "aws-sdk-js/3.598.0 command/CodeWhispererRuntime#1.1.58 ua/3.46.0 os/windows#10.0.22631 lang/nodejs#20.10.0 md/vscode#1.96.2 api/codewhisperer#1.1.58", "spec", "en-US,en;q=0.9", ""},
	{"aws-toolkit-vscode/1.83.0 VisualStudioCode/1.95.3 darwin-arm64 NodeJS/20.11.1", "aws-sdk-js/3.590.0 command/CodeWhispererRuntime#1.1.52 ua/3.44.0 os/macos#14.4.1 lang/nodejs#20.11.1 md/vscode#1.95.3 api/codewhisperer#1.1.52", "spec", "en-US,en;q=0.8,zh-CN;q=0.6", ""},
	{"AWS-Toolkit-For-JetBrains/1.68.0 IntelliJ-IDEA/2024.2.2 macOS/14.5", "aws-sdk-java/1.12.681 app/JetBrains-IDEA#2024.2.2 os/macos#14.5 lang/java#17 api/codewhisperer#1.0.22", "preview", "en-US,en;q=0.9", ""},
	{"AWS-Toolkit-For-JetBrains/1.66.0 PyCharm/2024.1.3 Linux/6.8.12", "aws-sdk-java/1.12.663 app/PyCharm#2024.1.3 os/linux#6.8.12 lang/java#17 api/codewhisperer#1.0.19", "spec", "en-US,en;q=0.7,de-DE;q=0.4", ""},
	{"aws-cli/2.17.41 Python/3.11.9 Linux/6.6.32 botocore/2.17.41", "aws-cli/2.17.41 Python/3.11.9 Linux/6.6.32 botocore/2.17.41 api/codewhisperer#1.0.15", "edge", "en-US", ""},
	{"boto3/1.34.144 Python/3.10.14 Darwin/22.6.0", "boto3/1.34.144 Python/3.10.14 Darwin/22.6.0 botocore/1.34.144 api/codewhisperer#1.0.15", "spec", "en-US,en;q=0.85", ""},
	{"aws-sdk-go/1.48.0 macOS/13.6", "aws-sdk-go/1.48.0 os/macos#13.6 lang/go#1.22 api/codewhisperer#1.0.14", "preview", "en-US,en;q=0.9,fr-FR;q=0.3", ""},
	{"aws-sdk-java/1.12.671 Linux/5.15.0-113", "aws-sdk-java/1.12.671 os/linux#5.15.0-113 lang/java#17 api/codewhisperer#1.0.28", "spec", "en-US,en;q=0.7", ""},
	{"aws-sdk-js/3.552.0 Node/v20.11.1 linux-x64", "aws-sdk-js/3.552.0 ua/3.35.1 os/linux#5.15.0 lang/nodejs#20.11.1 md/custom#cli api/codewhisperer#1.0.40", "internal", "en-US,en;q=0.8,es-ES;q=0.2", ""},
	{"aws-cdk/2.139.0 Node/v18.19.0 linux-x64", "aws-cdk/2.139.0 os/linux#6.8.9 lang/nodejs#18.19.0 api/codewhisperer#1.0.11", "spec", "en-US", ""},
	{"aws-sam-cli/1.120.0 Python/3.11.5 Windows/10.0.22631", "aws-sam-cli/1.120.0 os/windows#10.0.22631 lang/python#3.11.5 api/codewhisperer#1.0.12", "spec", "en-US,en;q=0.8", ""},
	{"AmplifyCLI/12.10.1 Node/v18.20.3 darwin-arm64", "AmplifyCLI/12.10.1 os/macos#14.4.1 lang/nodejs#18.20.3 api/codewhisperer#1.0.16", "spec", "en-US,en;q=0.7,ja-JP;q=0.3", ""},
	{"Terraform/1.8.5 aws-provider/5.64.0 darwin-arm64", "Terraform/1.8.5 aws-provider/5.64.0 os/macos#14.5 lang/go#1.22 api/codewhisperer#1.0.33", "spec", "en-US,en;q=0.5", ""},
	{"Pulumi/3.110.2 nodejs/20.11.1 linux-x64", "Pulumi/3.110.2 nodejs/20.11.1 os/linux#6.7.12 api/codewhisperer#1.0.20", "spec", "en-US,en;q=0.65", ""},
	{"ServerlessFramework/3.39.0 Node/20.15.0 darwin-x64", "ServerlessFramework/3.39.0 Node/20.15.0 os/macos#13.6 api/codewhisperer#1.0.14", "preview", "en-US,en;q=0.6", ""},
	{"AWSAppStudio/0.7.19 Linux/5.10.198", "AWSAppStudio/0.7.19 os/linux#5.10.198 lang/nodejs#18.19.1 api/codewhisperer#1.0.12", "spec", "en-US,en;q=0.9", ""},
	{"AWS-Toolkit-For-JetBrains/1.70.0 Rider/2024.2.1 Windows/10.0.22635", "aws-sdk-java/1.12.690 app/Rider#2024.2.1 os/windows#10.0.22635 api/codewhisperer#1.0.31", "spec", "en-US,en;q=0.7,ru-RU;q=0.2", ""},
	{"aws-toolkit-vscode/1.86.0 VisualStudioCode/1.97.1 linux-x64 NodeJS/20.17.0", "aws-sdk-js/3.610.0 command/CodeWhispererRuntime#1.1.63 ua/3.47.0 os/linux#6.9.4 lang/nodejs#20.17.0 md/vscode#1.97.1 api/codewhisperer#1.1.63", "spec", "en-US,en;q=0.82", ""},
	{"aws-toolkit-vscode/1.87.1 VisualStudioCode/1.98.2 darwin-arm64 NodeJS/20.19.0", "aws-sdk-js/3.621.0 command/CodeWhispererRuntime#1.1.70 ua/3.49.0 os/macos#14.6 lang/nodejs#20.19.0 md/vscode#1.98.2 api/codewhisperer#1.1.70", "spec", "en-US,en;q=0.88,zh-TW;q=0.5", ""},
	{"aws-toolkit-vscode/1.82.0 VisualStudioCode/1.94.2 linux-arm64 NodeJS/20.9.0", "aws-sdk-js/3.582.0 command/CodeWhispererRuntime#1.1.45 ua/3.42.0 os/linux#6.6.15 lang/nodejs#20.9.0 md/vscode#1.94.2 api/codewhisperer#1.1.45", "internal", "en-US,en;q=0.83,ko-KR;q=0.3", ""},
	{"aws-toolkit-vscode/1.81.0 VisualStudioCode/1.93.1 Windows_NT/10.0.19045 NodeJS/18.20.2", "aws-sdk-js/3.572.0 command/CodeWhispererRuntime#1.1.38 ua/3.41.0 os/windows#10.0.19045 lang/nodejs#18.20.2 md/vscode#1.93.1 api/codewhisperer#1.1.38", "spec", "en-US,en;q=0.8", ""},
	{"aws-toolkit-vscode/1.79.2 VisualStudioCode/1.92.1 linux-x64 NodeJS/18.18.2", "aws-sdk-js/3.560.0 command/CodeWhispererRuntime#1.1.30 ua/3.38.0 os/linux#5.15.134 lang/nodejs#18.18.2 md/vscode#1.92.1 api/codewhisperer#1.1.30", "preview", "en-US,en;q=0.75,pt-BR;q=0.3", ""},
	{"AWS-Toolkit-For-JetBrains/1.65.0 WebStorm/2023.3.4 macOS/13.6.6", "aws-sdk-java/1.12.640 app/WebStorm#2023.3.4 os/macos#13.6.6 api/codewhisperer#1.0.26", "spec", "en-US,en;q=0.9", ""},
	{"aws-toolkit-vscode/1.80.0 VisualStudioCode/1.92.0 Windows_NT/10.0.22621 NodeJS/18.19.1", "aws-sdk-js/3.563.0 command/CodeWhispererRuntime#1.1.33 ua/3.39.0 os/windows#10.0.22621 lang/nodejs#18.19.1 md/vscode#1.92.0 api/codewhisperer#1.1.33", "spec", "en-US,en;q=0.74,zh-CN;q=0.5", ""},
	{"aws-toolkit-vscode/1.78.0 VisualStudioCode/1.91.0 linux-x64 NodeJS/18.18.0", "aws-sdk-js/3.554.0 command/CodeWhispererRuntime#1.1.24 ua/3.37.0 os/linux#5.10.210 lang/nodejs#18.18.0 md/vscode#1.91.0 api/codewhisperer#1.1.24", "spec", "en-US,en;q=0.76", ""},
	{"aws-toolkit-vscode/1.88.0 VisualStudioCode/1.99.0 Windows_NT/10.0.22631 NodeJS/22.2.0", "aws-sdk-js/3.630.0 command/CodeWhispererRuntime#1.1.75 ua/3.50.0 os/windows#10.0.22631 lang/nodejs#22.2.0 md/vscode#1.99.0 api/codewhisperer#1.1.75", "spec", "en-US,en;q=0.85", ""},
	{"aws-toolkit-vscode/1.77.1 VisualStudioCode/1.90.2 darwin-x64 NodeJS/18.17.1", "aws-sdk-js/3.548.0 command/CodeWhispererRuntime#1.1.21 ua/3.36.0 os/macos#13.6.4 lang/nodejs#18.17.1 md/vscode#1.90.2 api/codewhisperer#1.1.21", "spec", "en-US,en;q=0.72", ""},
	{"aws-toolkit-vscode/1.76.0 VisualStudioCode/1.89.1 linux-arm64 NodeJS/18.17.0", "aws-sdk-js/3.540.0 command/CodeWhispererRuntime#1.1.18 ua/3.35.0 os/linux#5.15.124 lang/nodejs#18.17.0 md/vscode#1.89.1 api/codewhisperer#1.1.18", "spec", "en-US,en;q=0.65", ""},
	{"aws-toolkit-vscode/1.75.2 VisualStudioCode/1.88.1 linux-x64 NodeJS/18.16.1", "aws-sdk-js/3.533.0 command/CodeWhispererRuntime#1.1.12 ua/3.34.0 os/linux#5.4.0 lang/nodejs#18.16.1 md/vscode#1.88.1 api/codewhisperer#1.1.12", "edge", "en-US,en;q=0.67", ""},
	{"aws-toolkit-vscode/1.74.1 VisualStudioCode/1.87.2 darwin-arm64 NodeJS/18.16.0", "aws-sdk-js/3.528.0 command/CodeWhispererRuntime#1.1.9 ua/3.33.0 os/macos#13.5 lang/nodejs#18.16.0 md/vscode#1.87.2 api/codewhisperer#1.1.9", "spec", "en-US,en;q=0.69", ""},
	{"aws-toolkit-vscode/1.73.0 VisualStudioCode/1.86.1 Windows_NT/10.0.19045 NodeJS/18.15.0", "aws-sdk-js/3.520.0 command/CodeWhispererRuntime#1.1.5 ua/3.32.0 os/windows#10.0.19045 lang/nodejs#18.15.0 md/vscode#1.86.1 api/codewhisperer#1.1.5", "spec", "en-US,en;q=0.7", ""},
	{"aws-toolkit-vscode/1.72.0 VisualStudioCode/1.85.1 linux-x64 NodeJS/18.14.2", "aws-sdk-js/3.515.0 command/CodeWhispererRuntime#1.1.2 ua/3.31.0 os/linux#5.15.0 lang/nodejs#18.14.2 md/vscode#1.85.1 api/codewhisperer#1.1.2", "spec", "en-US,en;q=0.68", ""},
}

var acceptEncodings = []string{
	"gzip, deflate, br",
	"gzip, deflate",
	"gzip",
	"br, gzip",
}

var agentModes = []string{
	"vibe",
	"vibe",
	"vibe",
	"vibe",
	"vibe",
	"vibe",
	"vibe",
}

func NewHeaderManager() *HeaderManager {
	return &HeaderManager{
		stealthEnabled: config.IsStealthModeEnabled(),
		strategy:       config.ActiveHeaderStrategy(),
	}
}

// Apply 应用请求头
// tokenIdentifier 用于生成稳定的用户画像（版本号等），同一个 token 在一段时间内保持一致
func (m *HeaderManager) Apply(req *http.Request, isStream bool, tokenIdentifier string) {
	if !m.stealthEnabled {
		applyLegacyHeaders(req, isStream)
		return
	}

	profile := m.selectProfile(tokenIdentifier)
	req.Header.Set("User-Agent", profile.userAgent)
	req.Header.Set("x-amz-user-agent", profile.xAmzUserAgent)
	req.Header.Set("x-amzn-kiro-agent-mode", profile.agentMode)
	req.Header.Set("Accept-Language", profile.acceptLang)
	req.Header.Set("Accept-Encoding", chooseString(acceptEncodings))

	if !isStream {
		req.Header.Set("Accept", "application/json")
	}

	req.Header.Set("X-Amzn-Trace-Id", buildTraceID())
	req.Header.Set("X-Amzn-RequestId", strings.ToUpper(utils.RandomHex(32)))
}

func (m *HeaderManager) selectProfile(tokenIdentifier string) agentProfile {
	switch m.strategy {
	case config.HeaderStrategyRandom:
		return m.randomProfile(tokenIdentifier)
	default:
		// 默认使用 Kiro IDE 格式（更真实）
		return m.kiroProfile(tokenIdentifier)
	}
}

// kiroProfile 生成符合真实 Kiro IDE 请求头格式的 profile
// tokenIdentifier: 用于生成稳定的用户画像，同一个 token 在一段时间内保持一致
// 基于抓包数据（2024-12-28 Kiro 0.8.0）：
// user-agent: aws-sdk-js/1.0.27 ua/2.1 os/win32#10.0.26100 lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-0.8.0-{hash}
// x-amz-user-agent: aws-sdk-js/1.0.27 KiroIDE-0.8.0-{hash}
func (m *HeaderManager) kiroProfile(tokenIdentifier string) agentProfile {
	// 生成稳定的种子：基于 token + 当前周数（每周可能轻微变化，模拟升级）
	// 使用年份+周数，这样每周用户画像可能会变化（模拟真实的软件更新）
	year, week := time.Now().ISOWeek()
	stableSeed := fmt.Sprintf("%s-%d-W%d", tokenIdentifier, year, week)
	seedHash := sha256.Sum256([]byte(stableSeed))

	// Kiro 0.8.0 统一使用最新版本
	kiroVersion := "0.8.0"

	// 生成 KiroIDE 哈希（64位十六进制）
	// 基于 token 的稳定哈希，同一个 token 始终使用相同的 hash（模拟真实用户）
	kiroHash := generateStableKiroHash(seedHash)
	kiroSignature := fmt.Sprintf("KiroIDE-%s-%s", kiroVersion, kiroHash)

	// 基于稳定种子生成操作系统信息
	osInfo := stableKiroOS(seedHash)

	// 基于稳定种子生成 Node.js 版本（20-22，Kiro 0.8.0 使用较新版本）
	nodeJSMajor := 20 + (int(seedHash[2]) % 3) // 20-22
	nodeJSMinor := int(seedHash[3]) % 22       // 0-21
	nodeJSPatch := int(seedHash[4]) % 10       // 0-9

	// ua 版本（2.0-2.5）- 基于稳定种子
	uaMajor := 2
	uaMinor := int(seedHash[5]) % 6 // 0-5

	// 生成 mode 标识（E/A/B/C 等）- 基于稳定种子
	modes := []string{"E", "A", "B", "C", "D"}
	modeFlag := modes[int(seedHash[6])%len(modes)]

	// Kiro 0.8.0 默认使用 vibe 模式
	agentModeChoice := "vibe"

	// Accept-Language 基于稳定种子
	acceptLangs := []string{"en-US,en;q=0.9", "en-US,en;q=0.8,zh-CN;q=0.6", "en-GB,en;q=0.7", "en-US,en;q=0.85"}
	acceptLang := acceptLangs[int(seedHash[8])%len(acceptLangs)]

	// x-amz-user-agent: 简化格式（与真实 KiroIDE 0.8.0 一致）
	xAmzUA := fmt.Sprintf("aws-sdk-js/1.0.27 %s", kiroSignature)

	// user-agent: 完整格式（与真实 KiroIDE 0.8.0 一致）
	userAgent := fmt.Sprintf("aws-sdk-js/1.0.27 ua/%d.%d %s lang/js md/nodejs#%d.%d.%d api/codewhispererstreaming#1.0.27 m/%s %s",
		uaMajor, uaMinor,
		osInfo,
		nodeJSMajor, nodeJSMinor, nodeJSPatch,
		modeFlag,
		kiroSignature)

	// 生成机器 ID（用于 Telemetry）- 基于稳定种子
	machineIDHash := sha256.Sum256([]byte(fmt.Sprintf("%s-machine", stableSeed)))
	machineID := hex.EncodeToString(machineIDHash[:16]) // 使用前 16 字节作为机器 ID

	return agentProfile{
		userAgent:     userAgent,
		xAmzUserAgent: xAmzUA,
		agentMode:     agentModeChoice,
		acceptLang:    acceptLang,
		kiroMachineID: machineID,
	}
}

func (m *HeaderManager) randomProfile(tokenIdentifier string) agentProfile {
	// 为旧版本策略保留随机行为（已过时）
	ua := fmt.Sprintf("aws-sdk-js/%d.%d.%d ua/%d.%d os/%s lang/%s md/%s api/codewhisperer#%d.%d.%d", utils.RandomIntBetween(1, 4), utils.RandomIntBetween(0, 30), utils.RandomIntBetween(0, 90), utils.RandomIntBetween(2, 5), utils.RandomIntBetween(0, 30), randomPlatform(), randomLanguageRuntime(), randomClientSurface(), utils.RandomIntBetween(1, 2), utils.RandomIntBetween(0, 20), utils.RandomIntBetween(0, 80))
	primaryUA := fmt.Sprintf("aws-toolkit-%s/%d.%d.%d %s NodeJS/%d.%d.%d", randomEditor(), utils.RandomIntBetween(1, 2), utils.RandomIntBetween(60, 95), utils.RandomIntBetween(0, 9), randomHostApp(), utils.RandomIntBetween(16, 22), utils.RandomIntBetween(0, 19), utils.RandomIntBetween(0, 9))

	return agentProfile{
		userAgent:     primaryUA,
		xAmzUserAgent: ua,
		agentMode:     chooseString(agentModes),
		acceptLang:    chooseString([]string{"en-US,en;q=0.9", "en-US,en;q=0.8,zh-CN;q=0.4", "en-GB,en;q=0.7", "en-US,en;q=0.85,fr-FR;q=0.3"}),
		kiroMachineID: "",
	}
}

func chooseAgentProfile(profiles []agentProfile) agentProfile {
	idx := int(utils.RandomIntBetween(0, int64(len(profiles)-1)))
	return profiles[idx]
}

func chooseString(options []string) string {
	return options[int(utils.RandomIntBetween(0, int64(len(options)-1)))]
}

func randomPlatform() string {
	options := []string{
		fmt.Sprintf("windows#10.0.%d", utils.RandomIntBetween(18362, 22640)),
		fmt.Sprintf("macos#14.%d", utils.RandomIntBetween(0, 6)),
		fmt.Sprintf("macos#13.%d", utils.RandomIntBetween(0, 7)),
		fmt.Sprintf("linux#%d.%d", utils.RandomIntBetween(5, 6), utils.RandomIntBetween(4, 19)),
	}
	return chooseString(options)
}

func randomLanguageRuntime() string {
	options := []string{
		fmt.Sprintf("nodejs#%d.%d.%d", utils.RandomIntBetween(18, 22), utils.RandomIntBetween(0, 20), utils.RandomIntBetween(0, 9)),
		fmt.Sprintf("python#%d.%d.%d", utils.RandomIntBetween(3, 3), utils.RandomIntBetween(8, 12), utils.RandomIntBetween(0, 9)),
		fmt.Sprintf("java#%d", utils.RandomIntBetween(11, 21)),
	}
	return chooseString(options)
}

func randomClientSurface() string {
	surfaces := []string{
		"vscode#1.96.0",
		"vscode#1.95.3",
		"vscode#1.94.2",
		"jetbrains#2024.2",
		"jetbrains#2023.3",
		"cli",
		"browser",
	}
	return chooseString(surfaces)
}

func randomEditor() string {
	editors := []string{"vscode", "jetbrains", "studio", "cloud9", "cli"}
	return chooseString(editors)
}

func randomHostApp() string {
	apps := []string{
		"VisualStudioCode/1.98.2", "VisualStudioCode/1.95.3", "IntelliJ-IDEA/2024.2.2", "PyCharm/2024.1.3", "Rider/2024.2", "Cloud9/2024.10", "CodeCatalyst/1.24.3",
	}
	return chooseString(apps)
}

func buildTraceID() string {
	epoch := time.Now().Unix()
	return fmt.Sprintf("Root=1-%08x-%024s;Parent=%016s;Sampled=%d", epoch, utils.RandomHex(24), utils.RandomHex(16), utils.RandomIntBetween(0, 1))
}

// generateKiroHash 生成 64 位十六进制哈希，模拟 KiroIDE 的签名（已废弃）
// 真实示例: 954cd22dda111dffc3592dc86986f7e9860c20f6ba8201a62cbd92f69950e7c0
// 注意：此函数已被 generateStableKiroHash 替代，保留仅为兼容性
func generateKiroHash() string {
	// 生成随机数据
	randomData := fmt.Sprintf("%s-%d-%s",
		utils.RandomHex(16),
		time.Now().UnixNano(),
		utils.RandomHex(16))

	// 计算 SHA256 哈希
	hash := sha256.Sum256([]byte(randomData))
	return hex.EncodeToString(hash[:])
}

// generateStableKiroHash 生成稳定的 KiroIDE 哈希（基于 token 绑定）
// 同一个 token 始终得到相同的哈希，模拟真实用户的固定客户端标识
func generateStableKiroHash(seedHash [32]byte) string {
	// 使用 seedHash 的一部分作为输入，确保稳定性
	// 格式化为类似真实 KiroIDE 的 64 位哈希
	hashInput := fmt.Sprintf("kiro-stable-%x", seedHash[14:])
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:])
}

// stableKiroOS 基于稳定的哈希值生成 Kiro IDE 格式的操作系统信息
// 同一个 token 在一段时间内会得到相同的操作系统信息
func stableKiroOS(seedHash [32]byte) string {
	// 使用哈希值的不同字节来决定操作系统类型和版本
	platformChoice := int(seedHash[9]) % 3 // 0=Windows, 1=macOS, 2=Linux

	switch platformChoice {
	case 0:
		// Windows: os/win32#10.0.{build}
		build := 19041 + (int(seedHash[10])<<8+int(seedHash[11]))%(27000-19041)
		return fmt.Sprintf("os/win32#10.0.%d", build)
	case 1:
		// macOS: os/darwin#{major}.{minor}
		major := 13 + (int(seedHash[12]) % 3) // 13-15
		minor := int(seedHash[13]) % 8        // 0-7
		return fmt.Sprintf("os/darwin#%d.%d", major, minor)
	case 2:
		// Linux: os/linux#{kernel}
		major := 5 + (int(seedHash[14]) % 2)  // 5-6
		minor := 4 + (int(seedHash[15]) % 17) // 4-20
		patch := int(seedHash[16]) % 201      // 0-200
		return fmt.Sprintf("os/linux#%d.%d.%d", major, minor, patch)
	}

	return "os/win32#10.0.22621"
}

// randomKiroOS 生成 Kiro IDE 格式的操作系统信息（随机版本，已过时）
// 格式: os/{platform}#{version}
func randomKiroOS() string {
	platforms := []struct {
		name    string
		minVer  int64
		maxVer  int64
		pattern string
	}{
		// Windows: os/win32#10.0.{build}
		{"win32", 19041, 27000, "os/win32#10.0.%d"},
		// macOS: os/darwin#{major}.{minor}
		{"darwin", 13, 15, "os/darwin#%d.%d"},
		// Linux: os/linux#{kernel}
		{"linux", 5, 6, "os/linux#%d.%d.%d"},
	}

	platform := platforms[utils.RandomIntBetween(0, int64(len(platforms)-1))]

	switch platform.name {
	case "win32":
		build := utils.RandomIntBetween(platform.minVer, platform.maxVer)
		return fmt.Sprintf(platform.pattern, build)
	case "darwin":
		major := utils.RandomIntBetween(platform.minVer, platform.maxVer)
		minor := utils.RandomIntBetween(0, 7)
		return fmt.Sprintf("os/darwin#%d.%d", major, minor)
	case "linux":
		major := utils.RandomIntBetween(platform.minVer, platform.maxVer)
		minor := utils.RandomIntBetween(4, 20)
		patch := utils.RandomIntBetween(0, 200)
		return fmt.Sprintf("os/linux#%d.%d.%d", major, minor, patch)
	}

	return "os/win32#10.0.22621"
}

func applyLegacyHeaders(req *http.Request, isStream bool) {
	req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.27 KiroIDE-legacy")
	req.Header.Set("User-Agent", "aws-sdk-js/1.0.27 ua/legacy")
	req.Header.Set("Accept-Encoding", "gzip")
	if !isStream {
		req.Header.Set("Accept", "application/json")
	}
}
