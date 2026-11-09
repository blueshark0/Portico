// Package buildinfo 保存构建期元数据，供后端服务与桌面壳共用。
package buildinfo

// Version 与 build/config.yml、frontend/package.json 的版本号保持一致；
// 也可用 -ldflags "-X portico/internal/shared/buildinfo.Version=vX.Y.Z" 覆盖，但桌面壳与 portico-server 必须使用相同的值。
// 桌面壳与 portico-server 依靠它判断 sidecar 是否需要重启，二者必须一同发布。
var Version = "v1.0.0"
