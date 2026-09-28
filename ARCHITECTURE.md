# Architecture

- Go chi API는 로컬 경로 검증, 파일 미리보기, 업로드, 설정, 작업 상태와 SSE를 담당합니다.
- 메모리 큐는 등록 순서대로 한 작업만 실행합니다. 작업 스냅샷은 `server/data/jobs.json`에 원자적으로 저장합니다.
- 작업마다 별도 `codex app-server --listen stdio://` 프로세스를 열고 JSON-RPC `initialize → thread/start → turn/start`를 호출합니다. `item/agentMessage/delta`와 `item/commandExecution/outputDelta`를 UI에 전달합니다.
- Codex는 읽기 전용 샌드박스에서 자막 텍스트를 읽고 Markdown 본문만 반환합니다. Go 서버가 기존 결과를 덮어쓰지 않는 이름으로 `.md` 파일 하나를 저장합니다.
- 미디어는 프로젝트 안의 `whisper/` 모델·실행 파일과 로컬 ffmpeg로 먼저 전사합니다.
- 인증 계층은 없습니다. 서버는 loopback 주소와 Host/Origin 검사를 사용하며 로컬 단일 사용자 실행을 전제로 합니다. 공용 네트워크에 노출하는 배포는 지원하지 않습니다.
