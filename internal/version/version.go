package version

// Version 项目版本号
const Version = "1.01"

// ProjectName 项目名称
const ProjectName = "Kiro2API"

// GetVersionInfo 获取完整版本信息
func GetVersionInfo() string {
	return ProjectName + " v" + Version
}
