# Quality and operating limits

- Backend: `cd server && go test -race ./... && go vet ./...`
- Frontend: `cd ui && npm test && npm run check && npm run build`
- macOS에서 검증한 실제 흐름: SRT 및 WAV 등록 → 큐 → Codex 응답과 최근 출력 실시간 갱신 → 원본 폴더에 단일 Markdown 저장. Whisper `base` 설치와 전사도 확인했습니다.
- GPT-6 Astra, GPT-6 Sol, GPT-5.6 Sol의 `low`/`xhigh` 요청은 Codex CLI 0.158.0에서 실제 응답을 확인했습니다. 현재 PATH의 0.147.0은 GPT-6 모델을 표시하지 않으므로 UI가 선택을 비활성화합니다.
- Windows amd64와 arm64 교차 빌드는 통과했습니다. Whisper 설치·전사 흐름은 실제 Windows 기기에서 검증하지 못했습니다.
- 브라우저에서 파일 선택·미리보기·작업 등록·완료 상세를 확인했습니다. 390px와 1440px 너비에서 가로 넘침이 없고, 상세 화면의 결과·최근 출력은 넓은 화면에서 5:5입니다.
- 텍스트 입력은 2MB, 업로드 파일은 1GB, 폴더는 200개 파일로 제한합니다. 대용량 회의록은 Codex 모델 컨텍스트 한계에 영향을 받을 수 있습니다.
- macOS Whisper 자동 설치는 CMake가 필요합니다. 미디어 처리에는 ffmpeg가 필요합니다.
- 작업 취소·서버 종료는 실행 중인 Codex 프로세스를 중단합니다. 결과 파일은 완료 시에만 작성됩니다.
