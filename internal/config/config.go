package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Port             string
	IINABin          string
	MPVBin           string
	UploadMode       string
	DeviceName       string
	PollInterval     time.Duration
	ProgressInterval time.Duration
	HTTPTimeout      time.Duration
	IPCDir           string
}

func ParseFlags() Config {
	port := flag.String("port", "8080", "设置服务监听的端口")
	iinaBin := flag.String("iina-bin", "", "IINA 可执行文件，留空时自动查找 iina-cli/iina")
	mpvBin := flag.String("mpv-bin", "", "MPV 可执行文件，留空时从 PATH 查找 mpv")
	uploadMode := flag.String("upload-mode", "dry-run", "进度上传模式: dry-run|live")
	deviceName := flag.String("device-name", "IINAServer", "回传 Emby 时使用的设备名称")
	pollInterval := flag.Duration("poll-interval", 5*time.Second, "播放器状态采样间隔")
	progressInterval := flag.Duration("progress-interval", 30*time.Second, "实时进度上报最小间隔")
	httpTimeout := flag.Duration("http-timeout", 15*time.Second, "HTTP 请求超时")
	ipcDir := flag.String("ipc-dir", defaultIPCDir(), "播放器 IPC socket 目录")
	flag.Parse()

	mode := strings.ToLower(strings.TrimSpace(*uploadMode))
	if mode != "live" {
		mode = "dry-run"
	}

	return Config{
		Port:             *port,
		IINABin:          strings.TrimSpace(*iinaBin),
		MPVBin:           strings.TrimSpace(*mpvBin),
		UploadMode:       mode,
		DeviceName:       strings.TrimSpace(*deviceName),
		PollInterval:     *pollInterval,
		ProgressInterval: *progressInterval,
		HTTPTimeout:      *httpTimeout,
		IPCDir:           *ipcDir,
	}
}

func (c Config) DefaultDryRunUpload() bool {
	return c.UploadMode != "live"
}

func defaultIPCDir() string {
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		return filepath.Join(cacheDir, "iinaserver", "ipc")
	}
	return filepath.Join(os.TempDir(), "iinaserver-ipc")
}
