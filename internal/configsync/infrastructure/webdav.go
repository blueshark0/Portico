package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	configsyncapp "portico/internal/configsync/application"
	configsyncdomain "portico/internal/configsync/domain"
)

type WebDAVClient struct {
	client *http.Client
}

func NewWebDAVClient() *WebDAVClient {
	return &WebDAVClient{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *WebDAVClient) TestConnection(ctx context.Context, settings configsyncdomain.SyncSettings) error {
	resp, err := c.doRequest(ctx, settings, "PROPFIND", settings.RemotePath, strings.NewReader(`<?xml version="1.0" encoding="utf-8"?><propfind xmlns="DAV:"><prop><displayname/></prop></propfind>`), map[string]string{
		"Depth":        "0",
		"Content-Type": "application/xml",
	})
	if err != nil {
		return fmt.Errorf("连接 WebDAV 服务器失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("WebDAV 用户名或密码错误")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusMultiStatus && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return fmt.Errorf("连接 WebDAV 服务器失败: %s", resp.Status)
	}
	return nil
}

func (c *WebDAVClient) Upload(ctx context.Context, settings configsyncdomain.SyncSettings, filename string, payload []byte) error {
	if err := c.ensureRemoteDir(ctx, settings); err != nil {
		return err
	}
	resp, err := c.doRequest(ctx, settings, http.MethodPut, path.Join(settings.RemotePath, filename), bytes.NewReader(payload), map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		return fmt.Errorf("上传云端配置失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("WebDAV 用户名或密码错误")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("上传云端配置失败: %s", resp.Status)
	}
	return nil
}

func (c *WebDAVClient) Download(ctx context.Context, settings configsyncdomain.SyncSettings, filename string) ([]byte, error) {
	resp, err := c.doRequest(ctx, settings, http.MethodGet, path.Join(settings.RemotePath, filename), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("下载云端配置失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, configsyncapp.ErrRemoteSnapshotNotFound
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("WebDAV 用户名或密码错误")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载云端配置失败: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *WebDAVClient) ensureRemoteDir(ctx context.Context, settings configsyncdomain.SyncSettings) error {
	normalized := strings.Trim(settings.RemotePath, "/")
	if normalized == "" {
		return nil
	}
	segments := strings.Split(normalized, "/")
	current := ""
	for _, segment := range segments {
		current += "/" + segment
		resp, err := c.doRequest(ctx, settings, "MKCOL", current, nil, nil)
		if err != nil {
			return fmt.Errorf("创建云端目录失败: %w", err)
		}
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusCreated, http.StatusMethodNotAllowed, http.StatusOK:
		default:
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("创建云端目录失败: %s", resp.Status)
			}
		}
	}
	return nil
}

func (c *WebDAVClient) doRequest(
	ctx context.Context,
	settings configsyncdomain.SyncSettings,
	method string,
	remotePath string,
	body io.Reader,
	headers map[string]string,
) (*http.Response, error) {
	target, err := buildRemoteURL(settings.Endpoint, remotePath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(settings.Username, settings.AppPassword)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return c.client.Do(req)
}

func buildRemoteURL(endpoint, remotePath string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("WebDAV 服务器地址无效: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("WebDAV 服务器地址无效")
	}
	base.Path = path.Join(base.Path, remotePath)
	return base.String(), nil
}
