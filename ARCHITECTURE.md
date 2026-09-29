# Architecture

- Go chi API는 로컬 경로 검증, 파일 미리보기, 업로드, 설정, 작업 상태와 SSE를 담당합니다.
- 메모리 큐는 등록 순서대로 한 작업만 실행합니다. 작업 스냅샷은 `server/data/jobs.json`에 원자적으로 저장합니다.
- 작업마다 별도 `codex app-server --listen stdio://` 프로세스를 열고 JSON-RPC `initialize → thread/start → turn/start`를 호출합니다. `item/agentMessage/delta`와 `item/commandExecution/outputDelta`를 UI에 전달합니다.
- Codex는 읽기 전용 샌드박스에서 자막 텍스트를 읽고 Markdown 본문만 반환합니다. Go 서버가 기존 결과를 덮어쓰지 않는 이름으로 `.md` 파일 하나를 저장합니다.
- 미디어는 프로젝트 안의 `whisper/` 모델·실행 파일과 로컬 ffmpeg로 전사합니다. 수동 전사와 회의록 작업의 자동 전사는 같은 처리 함수를 쓰며, 작업 큐에서 순차 실행합니다. 타임코드가 있는 SRT 전사문은 `server/data/transcripts/`에 보관하고 원본 경로·크기·수정 시각·모델이 같을 때 재사용합니다. 영상 회의록 작업은 같은 SRT를 결과 폴더에도 저장합니다. 단계와 제한된 로그는 기존 SSE 상태에 포함합니다.
- Whisper 설치는 작업 큐와 별개로 서버 컨텍스트에서 실행합니다. 다운로드 전송량과 macOS 빌드 출력은 설치 상태에 담아 SSE로 전달하고 서버 터미널에도 기록합니다.
- 인증 계층은 없습니다. 서버는 loopback 주소와 Host/Origin 검사를 사용하며 로컬 단일 사용자 실행을 전제로 합니다. 공용 네트워크에 노출하는 배포는 지원하지 않습니다.
