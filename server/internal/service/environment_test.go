package service

import "testing"

func TestFFmpegVersion(t *testing.T) {
	if got := ffmpegVersion("ffmpeg version 7.1.1 Copyright (c) FFmpeg developers\nconfiguration: --enable-gpl"); got != "7.1.1" {
		t.Fatalf("version: %q", got)
	}
	if got := ffmpegVersion("unexpected output"); got != "" {
		t.Fatalf("unexpected version: %q", got)
	}
}
