package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Input validation and browsing share these supported extensions.
var allowedText = map[string]bool{".srt": true, ".vtt": true, ".txt": true, ".md": true}
var allowedMedia = map[string]bool{".wav": true, ".mp3": true, ".m4a": true, ".mp4": true, ".mov": true, ".ogg": true}

func validateFile(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("절대경로가 필요합니다")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("일반 파일만 선택할 수 있습니다")
	}
	ext := strings.ToLower(filepath.Ext(real))
	if !allowedText[ext] && !allowedMedia[ext] {
		return "", fmt.Errorf("지원하지 않는 형식: %s", ext)
	}
	if info.Size() > 1000*1024*1024 {
		return "", errors.New("파일은 1GB 이하만 지원합니다")
	}
	return real, nil
}
func Supported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedText[ext] || allowedMedia[ext]
}
func IsText(path string) bool { return allowedText[strings.ToLower(filepath.Ext(path))] }

func Expand(paths []string) ([]string, error) {
	if len(paths) == 0 || len(paths) > 100 {
		return nil, errors.New("파일 또는 폴더를 1~100개 선택해 주세요")
	}
	var result []string
	seen := map[string]bool{}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return nil, errors.New("절대경로가 필요합니다")
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			var entries []string
			err = filepath.WalkDir(p, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.IsDir() {
					if path != p && (d.Name() == ".git" || d.Name() == "node_modules") {
						return filepath.SkipDir
					}
					return nil
				}
				if d.Type()&os.ModeSymlink != 0 {
					return nil
				}
				if Supported(path) {
					entries = append(entries, path)
				}
				if len(entries) > 200 {
					return errors.New("폴더당 200개 파일까지만 선택할 수 있습니다")
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			sort.Strings(entries)
			for _, path := range entries {
				real, err := validateFile(path)
				if err != nil {
					return nil, err
				}
				if !seen[real] {
					result = append(result, real)
					seen[real] = true
				}
			}
		} else {
			real, err := validateFile(p)
			if err != nil {
				return nil, err
			}
			if !seen[real] {
				result = append(result, real)
				seen[real] = true
			}
		}
		if len(result) > 200 {
			return nil, errors.New("한 번에 200개 파일까지만 선택할 수 있습니다")
		}
	}
	if len(result) == 0 {
		return nil, errors.New("지원하는 파일이 없습니다")
	}
	return result, nil
}

func (s *Service) Preview(path string) (string, error) {
	text, _, err := s.PreviewState(path)
	return text, err
}

func (s *Service) PreviewState(path string) (string, bool, error) {
	real, err := validateFile(path)
	if err != nil {
		return "", false, err
	}
	if !IsText(real) {
		s.mu.RLock()
		model := s.cfg.WhisperModel
		s.mu.RUnlock()
		transcript, ready, err := s.cachedTranscript(real, model)
		if err != nil {
			return "", false, err
		}
		if ready {
			return tailPreview(transcript, 64*1024), true, nil
		}
		return "미디어 파일입니다. 전사를 시작하면 이곳에 전사문이 표시됩니다.", false, nil
	}
	f, err := os.Open(real)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 64*1024))
	return string(b), false, err
}

type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

func Browse(path string) (string, []Entry, error) {
	if path == "" {
		path, _ = os.UserHomeDir()
	}
	if !filepath.IsAbs(path) {
		return "", nil, errors.New("절대경로가 필요합니다")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		return "", nil, err
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		p := filepath.Join(real, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if entry.IsDir() || Supported(p) {
			result = append(result, Entry{Name: entry.Name(), Path: p, IsDir: entry.IsDir(), Size: info.Size()})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return real, result, nil
}

func (s *Service) Import(name string, reader io.Reader) (string, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if !Supported(name) {
		return "", errors.New("지원하지 않는 형식입니다")
	}
	dir := filepath.Join(s.root, "inbox")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	for i := 0; i < 1000; i++ {
		filename := name
		if i > 0 {
			filename = fmt.Sprintf("%s_%d%s", base, i+1, ext)
		}
		path := filepath.Join(dir, filename)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		n, err := io.Copy(f, io.LimitReader(reader, 1000*1024*1024+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil || n > 1000*1024*1024 {
			os.Remove(path)
			return "", errors.New("업로드 실패 또는 1GB 제한 초과")
		}
		return path, nil
	}
	return "", errors.New("파일명을 정할 수 없습니다")
}
