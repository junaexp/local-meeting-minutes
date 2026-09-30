# Quality and operating limits

- Backend: `cd server && go test -race ./... && go vet ./...`
- UI 정렬: `cd ui && npm run format:check` (`npm run format`으로 정렬)
- Frontend: `cd ui && npm test && npm run check && npm run build`
- macOS에서 검증한 실제 흐름: SRT 및 WAV 등록 → 큐 → Codex 응답과 최근 출력 실시간 갱신 → 원본 폴더에 단일 Markdown 저장. Whisper `base` 설치와 전사도 확인했습니다.
- GPT-6 Astra, GPT-6 Sol, GPT-5.6 Sol의 `low`/`xhigh` 요청은 Codex CLI 0.158.0에서 실제 응답을 확인했습니다. 현재 PATH의 0.147.0은 GPT-6 모델을 표시하지 않으므로 UI가 선택을 비활성화합니다.
- Windows amd64와 arm64 교차 빌드는 통과했습니다. Whisper 설치·전사 흐름은 실제 Windows 기기에서 검증하지 못했습니다.
- 설치 진행 표시는 로컬 HTTP 서버로 다운로드 전송량을 검증하고, 이미 설치된 모델·실행 파일을 사용하는 완료 상태를 검증했습니다. 전체 원격 모델 다운로드와 macOS CMake 빌드의 새 설치 과정은 이번 변경에서 실제로 재실행하지 않았습니다.
- 영상 회의록 작업은 자동 전사 SRT 내보내기, 캐시 재사용, 기존 파일과 이름 충돌 시 동일 번호의 SRT·MD 저장, Codex 실패 후 SRT 보존을 Go 테스트로 검증했습니다. 설정 화면의 다운로드 진행률·로그와 작업 상세의 SRT 경로는 UI 테스트로 검증했습니다.
- 수동 전사, SRT 보관·재사용, 원본·모델 변경 시 재전사, 작업 취소는 가짜 실행기로 Go 작업 큐를 검증했습니다. FFmpeg·Whisper 출력 스트리밍과 자식 프로세스 취소는 별도의 테스트용 실행 파일로 검증했습니다. macOS에서는 설치된 Whisper `base` 모델과 FFmpeg로 생성한 짧은 음성 파일을 실제 SRT로 전사했습니다. 이 실제 도구 검증은 `MEET_TO_MD_REAL_MEDIA_TEST=1 go test ./internal/service -run TestProcessTranscriberWithInstalledTools -v`로 재실행할 수 있습니다.
- 기존 화면은 브라우저에서 파일 선택·미리보기·작업 등록·완료 상세와 390px/1440px 가로 넘침을 확인했습니다. 이번 미디어 화면은 1280px에서 파일 선택·전사문/로그 영역·가로 넘침을 확인했습니다. 상세 화면의 두 출력 영역은 넓은 화면에서 5:5입니다.
- 텍스트 입력은 2MB, 업로드 파일은 1GB, 폴더는 200개 파일로 제한합니다. 대용량 회의록은 Codex 모델 컨텍스트 한계에 영향을 받을 수 있습니다.
- macOS Whisper 자동 설치는 CMake가 필요합니다. 미디어 처리에는 ffmpeg가 필요합니다.
- 작업 취소·서버 종료는 실행 중인 Codex 프로세스를 중단합니다. 영상 SRT는 전사 완료 직후 저장되어 이후 회의록 작성이 실패하거나 취소되어도 남습니다. Markdown은 Codex 작업이 성공한 뒤에만 작성됩니다.

## 구조 정리 및 성능 검증 (2026-09-30)

- 최종 검증: Go 경쟁 상태 검사를 포함한 테스트 38개, UI 테스트 35개, `go vet`, Svelte/TypeScript 검사, UI 빌드 및 포맷 검사를 통과했습니다. Windows amd64·arm64 교차 빌드도 통과했습니다.
- 합성 디스크 지연 중 상태 조회·로그 갱신·활성 작업 취소, 저장 실패 표시·복구, 저장 중 새 로그 보존, 삭제 실패 롤백, 중복 작업 등록 차단, 종료 시 저장을 Go 테스트로 검증합니다. Codex 출력 채널을 가득 채운 테스트용 자식 프로세스도 취소 후 종료되는지 확인합니다.
- UI 테스트는 처리 목록·미리보기 유지, 완료 후 명시적 재실행, 목록 청소 후 늦은 전사 완료로 항목이 부활하지 않는 동작, 저장 오류 표시, 기존 알림·전사·스크롤 기능을 검증합니다. 재접속 시 최근 요약에서 제외된 오래된 작업은 상세 기록으로 복구하며, 복구 요청 동시 실행은 4개로 제한합니다.
- 재현 가능한 부하용 서버: `cd ui && npm run build && node scripts/performance-fixture.mjs`. `http://127.0.0.1:8792`에서 합성 작업 1,000개 중 요약 250개를 200ms 간격으로 전송합니다. `/fixture/cart`의 JSON은 합성 입력 200개입니다. 이 도구는 실제 파일과 모델에 접근하지 않고 배포 번들에 포함되지 않습니다.
- macOS Apple M4 Pro/Chrome에서 입력 200개, 대기·실행 200개, 완료 요약 50개, 긴 미리보기로 측정했습니다. 1440px에서 미리보기 내부 높이 648px가 유지되며 세로 스크롤이 동작했습니다. 실제 내부 폭 500px에서도 가로 넘침이 없고 파일 변경 시 스크롤이 맨 위로 돌아왔습니다.
- 합성 스트림 중 설정 모달 클릭부터 두 번째 렌더링 프레임까지 약 37ms였습니다. 별도 약 2.4초 관찰에서 결과 갱신·취소가 동작했고 50ms 초과 long task는 관측되지 않았습니다. 이는 짧은 합성 측정으로 실제 Whisper의 CPU/GPU 부하나 장시간 실행 성능을 보장하지 않습니다. 브라우저 콘솔 오류·경고는 없었습니다.
- `go test ./internal/service -run '^$' -bench BenchmarkUISnapshotDense -benchtime=200ms -benchmem`: 누적 1,000개/활성 200개 조건의 스냅샷 생성·JSON 인코딩은 약 0.184ms/op였습니다. 전체 HTTP 응답 시간이나 이전 버전 대비 개선율을 의미하지 않습니다.
- 실제 Windows 실행과 실제 Codex/Whisper 모델의 장시간 처리는 이번 구조 정리에서 재실행하지 않았습니다. 브라우저 검증은 Chrome DevTools를 사용한 합성 환경의 제한된 범위입니다.
