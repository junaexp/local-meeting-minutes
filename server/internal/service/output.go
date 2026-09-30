package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result exports use exclusive creation, so a repeated run never overwrites user files.
func saveMinutes(source, outputDir, markdown string) (string, error) {
	if outputDir == "" {
		outputDir = filepath.Dir(source)
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + "_회의록"
	for n := 0; n < 1000; n++ {
		name := base
		if n > 0 {
			name = fmt.Sprintf("%s_%d", base, n+1)
		}
		target := filepath.Join(outputDir, name+".md")
		err := writeNewTextFile(target, markdown)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		return target, nil
	}
	return "", errors.New("사용 가능한 회의록 파일명을 찾지 못했습니다")
}

func IsVideo(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp4" || ext == ".mov"
}

func saveVideoTranscript(source, outputDir, transcript string) (string, string, error) {
	if outputDir == "" {
		outputDir = filepath.Dir(source)
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	for n := 0; n < 1000; n++ {
		suffix := ""
		if n > 0 {
			suffix = fmt.Sprintf("_%d", n+1)
		}
		srtPath := filepath.Join(outputDir, base+"_전사"+suffix+".srt")
		mdPath := filepath.Join(outputDir, base+"_회의록"+suffix+".md")
		if _, err := os.Stat(mdPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", "", err
		}
		if err := writeNewTextFile(srtPath, transcript); os.IsExist(err) {
			continue
		} else if err != nil {
			return "", "", err
		}
		return srtPath, mdPath, nil
	}
	return "", "", errors.New("사용 가능한 SRT·회의록 파일명을 찾지 못했습니다")
}

func saveStandaloneTranscript(source, outputDir, transcript string) (string, error) {
	if outputDir == "" {
		outputDir = filepath.Dir(source)
	}
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + "_전사"
	for n := 0; n < 1000; n++ {
		name := base
		if n > 0 {
			name = fmt.Sprintf("%s_%d", base, n+1)
		}
		path := filepath.Join(outputDir, name+".srt")
		err := writeNewTextFile(path, transcript)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		return path, nil
	}
	return "", errors.New("사용 가능한 SRT 파일명을 찾지 못했습니다")
}

func writeNewTextFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}
