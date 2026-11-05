package application

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	configsyncdomain "portico/internal/configsync/domain"

	"golang.org/x/crypto/scrypt"
)

const (
	kdfName   = "scrypt"
	kdfN      = 1 << 15
	kdfR      = 8
	kdfP      = 1
	kdfKeyLen = 32
	saltSize  = 16
	nonceSize = 12
)

type encryptedEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	UpdatedAt     string `json:"updatedAt"`
	DeviceName    string `json:"deviceName"`
	AppVersion    string `json:"appVersion"`
	KDF           string `json:"kdf"`
	Salt          string `json:"salt"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
}

func EncryptSnapshot(
	snapshot configsyncdomain.ConfigSnapshot,
	passphrase string,
	deviceName string,
	appVersion string,
	now time.Time,
) ([]byte, configsyncdomain.RemoteSnapshotMeta, error) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}

	salt, err := randomBytes(saltSize)
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	nonce, err := randomBytes(nonceSize)
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}

	updatedAt := now.UTC().Format(time.RFC3339)
	ciphertext := aead.Seal(nil, nonce, payload, nil)
	envelope := encryptedEnvelope{
		SchemaVersion: configsyncdomain.SchemaVersion,
		UpdatedAt:     updatedAt,
		DeviceName:    deviceName,
		AppVersion:    appVersion,
		KDF:           kdfName,
		Salt:          base64.StdEncoding.EncodeToString(salt),
		Nonce:         base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:    base64.StdEncoding.EncodeToString(ciphertext),
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	return data, configsyncdomain.RemoteSnapshotMeta{
		Exists:        true,
		UpdatedAt:     updatedAt,
		DeviceName:    deviceName,
		AppVersion:    appVersion,
		SchemaVersion: configsyncdomain.SchemaVersion,
	}, nil
}

func DecryptSnapshot(data []byte, passphrase string) (configsyncdomain.ConfigSnapshot, configsyncdomain.RemoteSnapshotMeta, error) {
	envelope, err := decodeEnvelope(data)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	if envelope.KDF != kdfName {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, fmt.Errorf("暂不支持的加密算法")
	}

	salt, err := base64.StdEncoding.DecodeString(envelope.Salt)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, fmt.Errorf("云端配置文件损坏")
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, fmt.Errorf("云端配置文件损坏")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, fmt.Errorf("云端配置文件损坏")
	}

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, fmt.Errorf("同步口令错误或云端文件损坏")
	}

	var snapshot configsyncdomain.ConfigSnapshot
	if err := json.Unmarshal(plaintext, &snapshot); err != nil {
		return configsyncdomain.ConfigSnapshot{}, configsyncdomain.RemoteSnapshotMeta{}, fmt.Errorf("云端配置文件损坏")
	}
	meta := configsyncdomain.RemoteSnapshotMeta{
		Exists:        true,
		UpdatedAt:     envelope.UpdatedAt,
		DeviceName:    envelope.DeviceName,
		AppVersion:    envelope.AppVersion,
		SchemaVersion: envelope.SchemaVersion,
	}
	return snapshot, meta, nil
}

func ReadEnvelopeMeta(data []byte) (configsyncdomain.RemoteSnapshotMeta, error) {
	envelope, err := decodeEnvelope(data)
	if err != nil {
		return configsyncdomain.RemoteSnapshotMeta{}, err
	}
	return configsyncdomain.RemoteSnapshotMeta{
		Exists:        true,
		UpdatedAt:     envelope.UpdatedAt,
		DeviceName:    envelope.DeviceName,
		AppVersion:    envelope.AppVersion,
		SchemaVersion: envelope.SchemaVersion,
	}, nil
}

func decodeEnvelope(data []byte) (encryptedEnvelope, error) {
	var envelope encryptedEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return encryptedEnvelope{}, fmt.Errorf("云端配置文件损坏")
	}
	if envelope.SchemaVersion == 0 || strings.TrimSpace(envelope.UpdatedAt) == "" || strings.TrimSpace(envelope.Ciphertext) == "" {
		return encryptedEnvelope{}, fmt.Errorf("云端配置文件损坏")
	}
	return envelope, nil
}

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, fmt.Errorf("请先保存同步口令")
	}
	return scrypt.Key([]byte(passphrase), salt, kdfN, kdfR, kdfP, kdfKeyLen)
}

func randomBytes(size int) ([]byte, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
