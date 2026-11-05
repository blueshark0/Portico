package main

// 与 build/config.yml、frontend/package.json 的版本号保持一致（打包元数据由 wails3 task common:update:build-assets 从 config.yml 生成）。
const AppVersion = "v1.0.0"

type VersionService struct {
	version string
}

func NewVersionService() *VersionService {
	return &VersionService{version: AppVersion}
}

func (vs *VersionService) CurrentVersion() string {
	return vs.version
}
