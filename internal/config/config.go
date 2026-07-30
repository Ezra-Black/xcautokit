package config

import (
	"os"
	"path/filepath"
)

const Version = "1.0.0"

type Config struct {
	DeviceUDID    string
	RecordingDir  string
	ScreenshotDir string
	RecordingCodec string
	ScreenshotFormat string
}

func Default() Config {
	home := os.Getenv("HOME")
	cfg := Config{
		DeviceUDID:       "booted",
		RecordingDir:     filepath.Join(home, "Pictures", "xcautokit", "recordings"),
		ScreenshotDir:    filepath.Join(home, "Pictures", "xcautokit", "screenshots"),
		RecordingCodec:   "hevc",
		ScreenshotFormat: "png",
	}
	_ = os.MkdirAll(cfg.RecordingDir, 0755)
	_ = os.MkdirAll(cfg.ScreenshotDir, 0755)
	return cfg
}
