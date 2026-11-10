# spine-echo-platform 릴리스 운영

대상 버전은 `v0.2.1`입니다. HTTP 입력 통합 테스트는 공개 Spine `v0.7.2-preview.2`에 고정합니다. 기존 Echo `v0.2.0` 정식 태그는 유지합니다.

## 채널과 의존성

| 채널 | Echo 후보 |
| --- | --- |
| Preview | `v0.2.1-preview.1` |
| RC | `v0.2.1-rc.1` |
| 정식 | `v0.2.1` |

이 플랫폼은 코어와 별도로 발행합니다. 승격할 때는 검증한 플랫폼 소스와 `go.mod`, `go.sum`을 유지합니다. 공개 설치와 테스트 결과는 해당 태그의 release manifest에서 확인합니다.

## 최초 설정

```sh
python3 scripts/release/configure_branches.py --bootstrap
```

이 명령은 preview와 rc 생성 계획만 출력합니다. 검토한 경우에만 `--bootstrap --apply`로 누락된 브랜치를 main에서 만듭니다. 기존 브랜치와 태그는 이동하지 않습니다. 첫 작업 PR을 preview에 반영한 다음 보호 규칙 조회와 `--apply`를 진행합니다.

보호 규칙은 기본 브랜치 preview, 대상별 필수 CI, 승격 방향, RC와 main의 merge commit, 태그 수정과 삭제 금지, 발행 환경을 설정합니다. 정식은 지정 리뷰어 승인과 main 제한, 관리자 우회 금지를 요구합니다. 이 설정의 적용과 PR 게시, 병합, 발행은 사용자가 검토한 후 진행합니다.

## Preview

공개 코어 버전으로 `go mod tidy`, `go mod verify`, `go test -race ./...`를 실행합니다.

변경을 커밋해 preview에 반영하고 전체 소스 CI를 확인합니다. **Actions → Publish Preview**에서 preview 브랜치, 이 모듈의 새 태그와 HEAD 전체 SHA를 지정합니다.

발행 workflow는 실제 코어 태그로 vet, race와 모듈 재현성을 검사합니다. Kafka와 RabbitMQ 구현은 실제 브로커도 검사합니다. CI와 공개 SHA를 사용한 소비자 검증이 성공하면 태그를 생성하고, 새로운 캐시의 별도 소비자에서 해당 태그로 다시 빌드합니다. 발행된 모듈의 테스트도 실행하며 실패나 skip을 허용하지 않습니다.

성공한 Preview와 RC는 `release-manifest-<태그>` artifact를 남기고 GitHub Release를 만들지 않습니다. artifact에는 소스, 실제 버전 조합, 검증 범위와 workflow 실행 증거가 들어갑니다. 정식만 GitHub Release를 만듭니다.

## 승격

Preview 완료 후 72시간, RC 완료 후 24시간을 관찰합니다. 정식 발행 간격은 168시간이며 긴급 관찰 기간 단축 사유는 20자 이상 기록합니다. CI, 공개 검증과 정식 승인은 생략하지 않습니다.

검증한 Preview 태그에서 전용 승격 브랜치를 만들고, RC를 대상으로 PR을 엽니다.

```sh
git switch -c promote/rc/v0.2.1-rc.1 v0.2.1-preview.1
```

승격 PR은 merge commit으로 병합합니다. Preview 태그와 병합 결과의 소스 fingerprint가 일치해야 합니다. 소스나 의존성이 바뀌면 새 Preview 후보에서 검증을 다시 시작합니다.

코어 릴리스의 전체 조합 검증은 코어 저장소의 **Complete Core Release Combination**에서 별도로 실행합니다.

## 검증 명령

```sh
python3 -m unittest discover -s test/release -p 'test_*.py'
node --test scripts/release/branch_flow.test.cjs
python3 scripts/release/release.py validate-policy
python3 scripts/release/release.py check-module --channel preview
actionlint -shellcheck= -pyflakes=
```

발행 전에 공개 코어로 로컬 테스트를 실행합니다. 원격 CI와 태그 발행 성공은 Actions에서 별도로 확인합니다.

## Discord 태그 알림

태그 생성에 성공하면 바로 [Spine 공통 Discord Action](https://github.com/NARUBROWN/spine/tree/b55c182f4d0722fca5aaf3b963bca20cf0dcc4d4/.github/actions/discord-tag)을 호출합니다. 알림 제목에 저장소 이름과 태그를 표시하고 태그 페이지와 발행 실행 기록으로 연결합니다. 이 알림은 태그 생성 시점의 기록이며 공개 설치 검증과 Release 게시 완료를 보장하지 않습니다.

| 태그 형식 | GitHub Actions repository secret |
| --- | --- |
| `vX.Y.Z-preview.N` | `DISCORD_WEBHOOK_PREVIEW` |
| `vX.Y.Z-rc.N` | `DISCORD_WEBHOOK_RC` |
| `vX.Y.Z` | `DISCORD_WEBHOOK_RELEASE` |

각 Discord 채널의 webhook URL을 이 저장소의 **Settings → Secrets and variables → Actions**에 위 이름으로 등록합니다. URL은 코드나 로그에 넣지 않습니다. 공통 Action은 전체 커밋 SHA로 고정하며 호출 workflow는 위 세 secret만 전달합니다.

`GITHUB_TOKEN`으로 만든 태그의 후속 이벤트에 의존하지 않고 기존 발행 workflow에서 직접 전송합니다. secret 누락과 전송 오류는 경고를 남기고 발행 검증을 계속합니다. 자동 재시도는 하지 않으며 응답 유실 시 채널 수신 여부를 먼저 확인합니다. 수동 태그 push와 GitHub Release 이벤트는 별도 알림으로 연결하지 않습니다.
