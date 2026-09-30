package service

import "time"

// Job is the persisted work record. Result files are owned separately from this history.
type Job struct {
	ID                 string     `json:"id"`
	Kind               string     `json:"kind,omitempty"`
	Path               string     `json:"path"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	Stage              string     `json:"stage,omitempty"`
	Phase              string     `json:"phase"`
	Model              string     `json:"model"`
	Effort             string     `json:"effort"`
	TranscriptionModel string     `json:"transcriptionModel,omitempty"`
	TranscriptKey      string     `json:"transcriptKey,omitempty"`
	TranscriptPath     string     `json:"transcriptPath,omitempty"`
	TranscriptPreview  string     `json:"transcriptPreview,omitempty"`
	TranscriptionLog   string     `json:"transcriptionLog,omitempty"`
	Result             string     `json:"result"`
	RecentOutput       string     `json:"recentOutput"`
	OutputPath         string     `json:"outputPath"`
	Error              string     `json:"error"`
	CreatedAt          time.Time  `json:"createdAt"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	Prompt             string     `json:"prompt"`
}

type Snapshot struct {
	StorageError      string          `json:"storageError,omitempty"`
	Jobs              []Job           `json:"jobs"`
	CodexReady        bool            `json:"codexReady"`
	WhisperReady      bool            `json:"whisperReady"`
	WhisperInstalling bool            `json:"whisperInstalling"`
	WhisperError      string          `json:"whisperError"`
	WhisperInstall    InstallProgress `json:"whisperInstall"`
}

type InstallProgress struct {
	Model      string `json:"model"`
	Stage      string `json:"stage"`
	Message    string `json:"message"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Log        string `json:"log"`
}
