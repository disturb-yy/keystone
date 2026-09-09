#!/usr/bin/env bash

# Golden Path 只保存可复核的外部驱动边界；任何失败都保留现场，不自动清理。
set -Eeuo pipefail
IFS=$'\n\t'

readonly SCRIPT_NAME="golden-path-e2e.sh"
readonly CHANGE_INTENT=$(cat <<'EOF'
在当前 Go HTTP demo 中，保持 `GET /` 的 HTTP 200 和 `hello` 响应不变。新增 `GET /healthz`，使其返回 HTTP 200、media type `application/json` 和 JSON 值 `{"status":"ok"}`。新增确定性自动化测试直接覆盖 `/healthz`，并确保 `go test ./...` 通过。除实现该 endpoint 及其测试所必需的变更外，不修改 `.keystone/project.yaml` 或其他项目配置。
EOF
)

RUN_ROOT=""
SOURCE_ROOT=""
CALLER_REPO_ROOT=""
KEYSTONE_REVISION=""
PLATFORM=""
PLATFORM_ATTESTATION=""
CODEX_BINARY=""
PLAYWRIGHT_BROWSERS_PATH=""
RUN_ID=""
EVIDENCE_SET_ID=""
INPUT_MANIFEST=""
INTERNAL_RUN=0

RAW_ROOT=""
MATERIALS_ROOT=""
LEDGER_ROOT=""
BIN_ROOT=""
STATE_ROOT=""
CACHE_ROOT=""
DEMO_REPOSITORY=""
DAEMON_PID=""
BASE_SERVICE_PID=""
CANDIDATE_SERVICE_PID=""
BASE_SERVICE_ENDPOINT=""
CANDIDATE_SERVICE_ENDPOINT=""
DAEMON_ENDPOINT=""
DAEMON_INSTANCE_ID=""
PROJECT_ID=""
CHANGE_ID=""
BASE_REVISION=""
CANDIDATE_REVISION=""
LEDGER_SEQUENCE=0
LAST_CHECKPOINT="not_started"
FAILURE_CODE=""
FAILURE_MESSAGE=""
PACKET_CREATED=0
EXIT_CODE=0
CODEX_PROBE_EXIT=0

usage() {
	cat <<'EOF'
用法：
  scripts/golden-path-e2e.sh \
    --keystone-revision <完整 Git OID> \
    --run-root <不存在的新目录> \
    --platform linux|wsl \
    --codex-binary <Codex executable> \
    --playwright-browsers-path <已准备的浏览器目录> \
    [--platform-attestation host|vm]

Runner 只接受完整 revision、新的 run root、显式 Codex executable 和显式浏览器目录。
失败 Run 会留下脱敏 review packet 及受控现场；Runner 不发布成功 Evidence。
EOF
}

die() {
	FAILURE_CODE=$1
	FAILURE_MESSAGE=$2
	printf 'Golden Path failed: %s\n' "$FAILURE_CODE" >&2
	printf '%s\n' "$FAILURE_MESSAGE" >&2
	exit 1
}

set_checkpoint() {
	LAST_CHECKPOINT=$1
}

sha256_text() {
	printf '%s' "$1" | sha256sum | awk '{print $1}'
}

sha256_file() {
	sha256sum "$1" | awk '{print $1}'
}

file_digest_or_fail() {
	local path=$1
	if [[ ! -f "$path" ]]; then
		die "missing_input" "需要的文件不存在"
	fi
	sha256_file "$path"
}

uuid_v7() {
	local milliseconds timestamp random
	milliseconds=$(date +%s%3N 2>/dev/null || date +%s000)
	timestamp=$(printf '%012x' "$milliseconds")
	random=$(od -An -N9 -tx1 /dev/urandom | tr -d ' \n')
	if [[ ${#random} -ne 18 ]]; then
		die "random_source_unavailable" "无法生成 Run identity"
	fi
	printf '%s-%s-7%s-8%s-%s\n' \
		"${timestamp:0:8}" "${timestamp:8:4}" "${random:0:3}" "${random:3:3}" "${random:6:12}"
}

ensure_absolute_directory() {
	local path=$1 label=$2
	if [[ "$path" != /* ]]; then
		die "invalid_${label}" "${label} 必须是绝对路径"
	fi
}

safe_json_projection() {
	local input=$1 output=$2
	jq 'walk(if type == "object" then del(.repository_root, .repository_path, .manifest_path, .database_path, .workspace_path, .absolute_path, .source_path, .path, .prompt, .environment, .command, .argv) else . end)' "$input" >"$output"
}

write_material_text() {
	local name=$1 content=$2 temporary
	temporary=$(mktemp "$MATERIALS_ROOT/.material.XXXXXX")
	printf '%b\n' "$content" >"$temporary"
	mv "$temporary" "$MATERIALS_ROOT/$name"
}

write_material_file() {
	local name=$1 source=$2 temporary
	temporary=$(mktemp "$MATERIALS_ROOT/.material.XXXXXX")
	cp "$source" "$temporary"
	mv "$temporary" "$MATERIALS_ROOT/$name"
}

write_ledger() {
	local sequence=$1 operation=$2 scope=$3 expected_version=$4 key=$5 request_digest=$6 temporary
	temporary=$(mktemp "$LEDGER_ROOT/.ledger.XXXXXX")
	jq -n \
		--arg operation "$operation" \
		--arg scope "$scope" \
		--arg expected_version "$expected_version" \
		--arg key_fingerprint "$(sha256_text "$key")" \
		--arg request_digest "$request_digest" \
		--argjson sequence "$sequence" \
		'{sequence: $sequence, operation: $operation, scope: $scope, expected_version: $expected_version, idempotency_key_fingerprint: $key_fingerprint, request_digest: $request_digest}' \
		>"$temporary"
	mv "$temporary" "$LEDGER_ROOT/$(printf '%04d' "$sequence").json"
}

safe_path_digest() {
	printf '%s' "$PATH" | sha256sum | awk '{print $1}'
}

safe_browser_digest() {
	local directory=$1
	safe_directory_digest "$directory"
}

safe_directory_digest() {
	local directory=$1
	(
		cd "$directory"
		find . -type f -print0 | sort -z | while IFS= read -r -d '' file; do
			sha256sum "$file"
		done
	) | sha256sum | awk '{print $1}'
}

validate_uuid_v7() {
	[[ "$1" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]
}

parse_args() {
	while (($# > 0)); do
		case "$1" in
		--help|-h)
			usage
			exit 0
			;;
		--keystone-revision)
			(($# >= 2)) || die "invalid_arguments" "--keystone-revision 缺少值"
			KEYSTONE_REVISION=$2
			shift 2
			;;
		--run-root)
			(($# >= 2)) || die "invalid_arguments" "--run-root 缺少值"
			RUN_ROOT=$2
			shift 2
			;;
		--platform)
			(($# >= 2)) || die "invalid_arguments" "--platform 缺少值"
			PLATFORM=$2
			shift 2
			;;
		--platform-attestation)
			(($# >= 2)) || die "invalid_arguments" "--platform-attestation 缺少值"
			PLATFORM_ATTESTATION=$2
			shift 2
			;;
		--codex-binary)
			(($# >= 2)) || die "invalid_arguments" "--codex-binary 缺少值"
			CODEX_BINARY=$2
			shift 2
			;;
		--playwright-browsers-path)
			(($# >= 2)) || die "invalid_arguments" "--playwright-browsers-path 缺少值"
			PLAYWRIGHT_BROWSERS_PATH=$2
			shift 2
			;;
		--internal-run)
			INTERNAL_RUN=1
			shift
			;;
		--internal-source)
			(($# >= 2)) || die "invalid_arguments" "--internal-source 缺少值"
			SOURCE_ROOT=$2
			shift 2
			;;
		--run-id)
			(($# >= 2)) || die "invalid_arguments" "--run-id 缺少值"
			RUN_ID=$2
			shift 2
			;;
		--evidence-set-id)
			(($# >= 2)) || die "invalid_arguments" "--evidence-set-id 缺少值"
			EVIDENCE_SET_ID=$2
			shift 2
			;;
		*)
			die "invalid_arguments" "存在未知参数"
			;;
		esac
	done

	[[ -n "$KEYSTONE_REVISION" ]] || die "invalid_arguments" "必须提供 --keystone-revision"
	[[ -n "$RUN_ROOT" ]] || die "invalid_arguments" "必须提供 --run-root"
	[[ -n "$PLATFORM" ]] || die "invalid_arguments" "必须提供 --platform"
	[[ -n "$CODEX_BINARY" ]] || die "invalid_arguments" "必须提供 --codex-binary"
	[[ -n "$PLAYWRIGHT_BROWSERS_PATH" ]] || die "invalid_arguments" "必须提供 --playwright-browsers-path"
	[[ "$PLATFORM" == "linux" || "$PLATFORM" == "wsl" ]] || die "invalid_arguments" "--platform 只能是 linux 或 wsl"
	if [[ "$PLATFORM" == "linux" ]]; then
		[[ "$PLATFORM_ATTESTATION" == "host" || "$PLATFORM_ATTESTATION" == "vm" ]] || die "invalid_arguments" "linux 必须声明 host 或 vm"
	else
		[[ -z "$PLATFORM_ATTESTATION" ]] || die "invalid_arguments" "wsl 不接受 Linux host/vm 声明"
	fi
	ensure_absolute_directory "$RUN_ROOT" "run_root"
	ensure_absolute_directory "$PLAYWRIGHT_BROWSERS_PATH" "playwright_browsers_path"
	if [[ "$INTERNAL_RUN" == "0" ]]; then
		[[ "$KEYSTONE_REVISION" =~ ^([0-9a-fA-F]{40}|[0-9a-fA-F]{64})$ ]] || die "invalid_keystone_revision" "必须提供完整 40 或 64 位 Git OID"
	else
		validate_uuid_v7 "$RUN_ID" || die "invalid_run_id" "内部 RunID 不是 UUIDv7"
		[[ "$EVIDENCE_SET_ID" =~ ^[0-9a-f]{64}$ ]] || die "invalid_evidence_set_id" "内部 EvidenceSetID 无效"
	fi
}

initialize_run_directories() {
	if [[ "$INTERNAL_RUN" == "0" ]]; then
		[[ ! -e "$RUN_ROOT" && ! -L "$RUN_ROOT" ]] || die "run_root_exists" "run root 必须是此前不存在的新目录"
		mkdir -m 700 "$RUN_ROOT"
	else
		[[ -d "$RUN_ROOT" ]] || die "run_root_missing" "canonical Runner 找不到受控 run root"
	fi
	RAW_ROOT="$RUN_ROOT/raw"
	MATERIALS_ROOT="$RUN_ROOT/materials"
	LEDGER_ROOT="$RUN_ROOT/command-ledger"
	BIN_ROOT="$RUN_ROOT/bin"
	STATE_ROOT="$RUN_ROOT/local-state"
	CACHE_ROOT="$RUN_ROOT/go-cache"
	DEMO_REPOSITORY="$RUN_ROOT/demo-repository"
	INPUT_MANIFEST="$MATERIALS_ROOT/input-manifest.txt"
	mkdir -m 700 -p "$RAW_ROOT" "$MATERIALS_ROOT" "$LEDGER_ROOT" "$BIN_ROOT" "$STATE_ROOT" "$CACHE_ROOT"
	chmod 700 "$RUN_ROOT" "$RAW_ROOT" "$MATERIALS_ROOT" "$LEDGER_ROOT" "$BIN_ROOT" "$STATE_ROOT" "$CACHE_ROOT"
}

locate_caller_repository() {
	CALLER_REPO_ROOT=$(git rev-parse --show-toplevel 2>"$RAW_ROOT/git-rev-parse.err") || die "not_git_repository" "调用目录不是 Git Repository"
	CALLER_REPO_ROOT=$(cd "$CALLER_REPO_ROOT" && pwd -P)
}

validate_caller_repository() {
	[[ -n "$CALLER_REPO_ROOT" ]] || locate_caller_repository
	local status_output
	status_output=$(git -C "$CALLER_REPO_ROOT" status --porcelain=v1 --untracked-files=all --ignore-submodules=none)
	[[ -z "$status_output" ]] || die "caller_worktree_dirty" "调用者工作树必须干净"
	git -C "$CALLER_REPO_ROOT" cat-file -e "$KEYSTONE_REVISION^{commit}" 2>"$RAW_ROOT/revision.err" || die "invalid_keystone_revision" "revision 不是可解析的 commit OID"
}

prepare_input_manifest() {
	local fixture_digest intent_digest runner_digest dashboard_digest manifest fixture_files expected_fixture_files
	expected_fixture_files=$'testdata/golden-path-demo/.keystone/project.yaml\ntestdata/golden-path-demo/go.mod\ntestdata/golden-path-demo/main.go\ntestdata/golden-path-demo/main_test.go'
	fixture_files=$(git -C "$CALLER_REPO_ROOT" ls-tree -r --name-only "$KEYSTONE_REVISION" -- testdata/golden-path-demo | sort)
	[[ "$fixture_files" == "$expected_fixture_files" ]] || die "fixture_tree_invalid" "revision 中的 GoldenPathFixture 不符合冻结文件边界"
	fixture_digest=$(git -C "$CALLER_REPO_ROOT" archive --format=tar "$KEYSTONE_REVISION" -- testdata/golden-path-demo | sha256sum | awk '{print $1}')
	intent_digest=$(sha256_text "$CHANGE_INTENT")
	runner_digest=$(git -C "$CALLER_REPO_ROOT" show "$KEYSTONE_REVISION:scripts/golden-path-e2e.sh" | sha256sum | awk '{print $1}') || die "runner_script_missing" "revision 不包含 canonical Runner"
	dashboard_digest=$(git -C "$CALLER_REPO_ROOT" show "$KEYSTONE_REVISION:dashboard/package-lock.json" | sha256sum | awk '{print $1}') || die "dashboard_lockfile_missing" "revision 不包含 Dashboard lockfile"
	manifest=$(printf 'change_intent_sha256=%s\ndashboard_lockfile_sha256=%s\nfixture_tree_sha256=%s\nkeystone_revision=%s\nrunner_script_sha256=%s\n' \
		"$intent_digest" "$dashboard_digest" "$fixture_digest" "$KEYSTONE_REVISION" "$runner_digest")
	EVIDENCE_SET_ID=$(sha256_text "$manifest")
	INPUT_MANIFEST="$MATERIALS_ROOT/input-manifest.txt"
	write_material_text "input-manifest.txt" "$manifest"
	write_material_text "run-identity.txt" "golden_path_run_id=$RUN_ID\ngolden_path_evidence_set_id=$EVIDENCE_SET_ID\n"
}

archive_canonical_source() {
	SOURCE_ROOT="$RUN_ROOT/keystone-source"
	mkdir -m 700 "$SOURCE_ROOT"
	git -C "$CALLER_REPO_ROOT" archive --format=tar "$KEYSTONE_REVISION" | tar -x -C "$SOURCE_ROOT"
	[[ -x "$SOURCE_ROOT/scripts/golden-path-e2e.sh" ]] || chmod 700 "$SOURCE_ROOT/scripts/golden-path-e2e.sh"
}

initial_entry() {
	initialize_run_directories
	RUN_ID=$(uuid_v7)
	locate_caller_repository
	prepare_input_manifest
	validate_caller_repository
	archive_canonical_source
	set_checkpoint "archive_created"
	exec "$SOURCE_ROOT/scripts/golden-path-e2e.sh" \
		--internal-run \
		--internal-source "$SOURCE_ROOT" \
		--keystone-revision "$KEYSTONE_REVISION" \
		--run-root "$RUN_ROOT" \
		--platform "$PLATFORM" \
		--platform-attestation "$PLATFORM_ATTESTATION" \
		--codex-binary "$CODEX_BINARY" \
		--playwright-browsers-path "$PLAYWRIGHT_BROWSERS_PATH" \
		--run-id "$RUN_ID" \
		--evidence-set-id "$EVIDENCE_SET_ID"
}

canonical_entry() {
	[[ -d "$SOURCE_ROOT" ]] || die "archive_missing" "canonical Runner 缺少 archive source"
	[[ -f "$INPUT_MANIFEST" ]] || die "manifest_missing" "canonical Runner 缺少输入 manifest"
	if [[ "$(sed -n 's/^keystone_revision=//p' "$INPUT_MANIFEST")" != "$KEYSTONE_REVISION" ]]; then
		die "manifest_mismatch" "输入 manifest 的 revision 与参数不一致"
	fi
	write_material_text "platform-input.txt" $'platform='"$PLATFORM"$'\nattestation='"${PLATFORM_ATTESTATION:-none}"$'\n'
}

probe_platform() {
	set_checkpoint "platform_probe"
	local uname_name os_id os_version wsl_marker container_marker
	uname_name=$(uname -s)
	os_id=$(awk -F= '$1 == "ID" {gsub(/"/, "", $2); print $2; exit}' /etc/os-release 2>/dev/null || true)
	os_version=$(awk -F= '$1 == "VERSION_ID" {gsub(/"/, "", $2); print $2; exit}' /etc/os-release 2>/dev/null || true)
	wsl_marker=false
	if grep -Eqi 'microsoft|wsl' /proc/version /proc/sys/kernel/osrelease 2>/dev/null; then
		wsl_marker=true
	fi
	container_marker=false
	if [[ -e /.dockerenv || -e /run/.containerenv ]] || grep -Eqi 'docker|containerd|kubepods|podman' /proc/1/cgroup 2>/dev/null; then
		container_marker=true
	fi
	if [[ "$PLATFORM" == "wsl" && "$wsl_marker" != true ]]; then
		die "platform_provenance_unverified" "声明为 wsl 但未观察到 WSL marker"
	fi
	if [[ "$PLATFORM" == "linux" && ( "$uname_name" != "Linux" || "$wsl_marker" == true || "$container_marker" == true ) ]]; then
		die "platform_provenance_unverified" "声明为非 WSL Linux，但来源探针不满足 host/vm 条件"
	fi
	jq -n \
		--arg platform "$PLATFORM" \
		--arg attestation "${PLATFORM_ATTESTATION:-none}" \
		--arg uname "$uname_name" \
		--arg os_id "${os_id:-unknown}" \
		--arg os_version "${os_version:-unknown}" \
		--argjson wsl_marker "$wsl_marker" \
		--argjson container_marker "$container_marker" \
		'{platform: $platform, attestation: $attestation, uname: $uname, os_id: $os_id, os_version: $os_version, wsl_marker: $wsl_marker, container_marker: $container_marker}' \
		>"$RAW_ROOT/platform-provenance.json"
	write_material_file "platform-provenance.json" "$RAW_ROOT/platform-provenance.json"
}

resolve_codex_binary() {
	set_checkpoint "codex_binary_resolve"
	local resolved
	if [[ "$CODEX_BINARY" == */* ]]; then
		[[ -f "$CODEX_BINARY" && -x "$CODEX_BINARY" ]] || die "codex_binary_unavailable" "显式 Codex executable 不存在或不可执行"
		resolved=$(readlink -f "$CODEX_BINARY") || die "codex_binary_unavailable" "无法解析 Codex executable"
	else
		resolved=$(command -v "$CODEX_BINARY" 2>"$RAW_ROOT/codex-lookup.err") || die "codex_binary_unavailable" "无法从受控 PATH 找到 Codex executable"
		[[ -x "$resolved" ]] || die "codex_binary_unavailable" "解析出的 Codex executable 不可执行"
		resolved=$(readlink -f "$resolved") || die "codex_binary_unavailable" "无法解析 Codex executable"
	fi
	CODEX_BINARY=$resolved
	ln -sfn "$CODEX_BINARY" "$BIN_ROOT/codex"
	export PATH="$BIN_ROOT:$(dirname "$CODEX_BINARY"):$PATH"
	write_material_text "codex-input.txt" "binary_digest=$(sha256_file "$CODEX_BINARY")\npath_digest=$(safe_path_digest)\n"
}

run_codex_probe() {
	local output=$1 error=$2 status=0
	shift 2
	set +e
	timeout --foreground 30s "$CODEX_BINARY" "$@" >"$output" 2>"$error"
	status=$?
	set -e
	CODEX_PROBE_EXIT=$status
	return "$status"
}

probe_codex() {
	set_checkpoint "codex_preflight"
	local version_output="$RAW_ROOT/codex-version.stdout" version_error="$RAW_ROOT/codex-version.stderr"
	local login_output="$RAW_ROOT/codex-login.stdout" login_error="$RAW_ROOT/codex-login.stderr" version_exit login_exit
	if ! run_codex_probe "$version_output" "$version_error" --version; then
		version_exit=$CODEX_PROBE_EXIT
		jq -n --arg failure_code "codex_version_probe_failed" --arg version_stdout_sha256 "$(sha256_file "$version_output")" --arg version_stderr_sha256 "$(sha256_file "$version_error")" --argjson version_exit_code "$version_exit" \
			'{status:"failed",failure_code:$failure_code,version_result:"failed",version_exit_code:$version_exit_code,version_stdout_sha256:$version_stdout_sha256,version_stderr_sha256:$version_stderr_sha256}' \
			>"$RAW_ROOT/codex-preflight.json"
		write_material_file "codex-preflight.json" "$RAW_ROOT/codex-preflight.json"
		die "codex_version_probe_failed" "Codex version probe 失败"
	fi
	version_exit=$CODEX_PROBE_EXIT
	if ! run_codex_probe "$login_output" "$login_error" login status; then
		login_exit=$CODEX_PROBE_EXIT
		jq -n --arg failure_code "codex_auth_probe_failed" --arg version_stdout_sha256 "$(sha256_file "$version_output")" --arg version_stderr_sha256 "$(sha256_file "$version_error")" --arg login_stdout_sha256 "$(sha256_file "$login_output")" --arg login_stderr_sha256 "$(sha256_file "$login_error")" --argjson version_exit_code "$version_exit" --argjson login_exit_code "$login_exit" \
			'{status:"failed",failure_code:$failure_code,version_result:"passed",version_exit_code:$version_exit_code,version_stdout_sha256:$version_stdout_sha256,version_stderr_sha256:$version_stderr_sha256,login_result:"failed",login_exit_code:$login_exit,login_stdout_sha256:$login_stdout_sha256,login_stderr_sha256:$login_stderr_sha256}' \
			>"$RAW_ROOT/codex-preflight.json"
		write_material_file "codex-preflight.json" "$RAW_ROOT/codex-preflight.json"
		die "codex_auth_probe_failed" "Codex login status probe 失败"
	fi
	login_exit=$CODEX_PROBE_EXIT
	if grep -Eqi 'not[[:space:]]+logged|unauth|logged[[:space:]]+out|unknown|indeterminate' "$login_output" "$login_error"; then
		jq -n --arg failure_code "codex_auth_probe_indeterminate" --arg version_stdout_sha256 "$(sha256_file "$version_output")" --arg version_stderr_sha256 "$(sha256_file "$version_error")" --arg login_stdout_sha256 "$(sha256_file "$login_output")" --arg login_stderr_sha256 "$(sha256_file "$login_error")" --argjson version_exit_code "$version_exit" --argjson login_exit_code "$login_exit" \
			'{status:"failed",failure_code:$failure_code,version_result:"passed",version_exit_code:$version_exit_code,version_stdout_sha256:$version_stdout_sha256,version_stderr_sha256:$version_stderr_sha256,login_result:"indeterminate",login_exit_code:$login_exit,login_stdout_sha256:$login_stdout_sha256,login_stderr_sha256:$login_stderr_sha256}' \
			>"$RAW_ROOT/codex-preflight.json"
		write_material_file "codex-preflight.json" "$RAW_ROOT/codex-preflight.json"
		die "codex_auth_probe_indeterminate" "Codex login status 无法形成安全通过结论"
	fi
	jq -n \
		--arg version_result "passed" \
		--arg login_result "passed" \
		--argjson version_exit_code "$version_exit" \
		--argjson login_exit_code "$login_exit" \
		--arg version_digest "$(sha256_file "$version_output")" \
		--arg version_error_digest "$(sha256_file "$version_error")" \
		--arg login_digest "$(sha256_file "$login_output")" \
		--arg login_error_digest "$(sha256_file "$login_error")" \
		--arg path_digest "$(safe_path_digest)" \
		'{status:"passed",version_result:$version_result,version_exit_code:$version_exit_code,login_result:$login_result,login_exit_code:$login_exit_code,version_stdout_sha256:$version_digest,version_stderr_sha256:$version_error_digest,login_stdout_sha256:$login_digest,login_stderr_sha256:$login_error_digest,path_sha256:$path_digest}' \
		>"$RAW_ROOT/codex-preflight.json"
	write_material_file "codex-preflight.json" "$RAW_ROOT/codex-preflight.json"
}

prepare_browser() {
	set_checkpoint "browser_preflight"
	[[ -d "$PLAYWRIGHT_BROWSERS_PATH" ]] || die "browser_unavailable" "Playwright browser directory 不存在"
	[[ -x "$SOURCE_ROOT/dashboard/node_modules/.bin/playwright" ]] || die "browser_tool_unavailable" "canonical source 未安装锁定的 Playwright CLI"
	local count
	count=$(find "$PLAYWRIGHT_BROWSERS_PATH" -type f -print -quit | wc -l)
	[[ "$count" -gt 0 ]] || die "browser_unavailable" "Playwright browser directory 为空"
	local digest playwright_version cli_output="$RAW_ROOT/playwright-version.stdout" cli_error="$RAW_ROOT/playwright-version.stderr"
	if ! run_in_directory "$SOURCE_ROOT/dashboard" "$cli_output" "$cli_error" env "PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH" ./node_modules/.bin/playwright --version; then
		die "browser_tool_unavailable" "锁定的 Playwright CLI 版本探针失败"
	fi
	playwright_version=$(jq -r '.devDependencies.playwright // empty' "$SOURCE_ROOT/dashboard/package.json")
	[[ "$playwright_version" == "1.55.0" ]] || die "browser_tool_unavailable" "Playwright package 未使用冻结版本"
	digest=$(safe_browser_digest "$PLAYWRIGHT_BROWSERS_PATH")
	jq -n \
		--arg version "$playwright_version" \
		--arg browser_digest "$digest" \
		--arg cli_stdout_sha256 "$(sha256_file "$cli_output")" \
		--arg cli_stderr_sha256 "$(sha256_file "$cli_error")" \
		'{playwright_version:$version,browser_engine:"Chromium",headless:true,browser_digest:$browser_digest,cli_stdout_sha256:$cli_stdout_sha256,cli_stderr_sha256:$cli_stderr_sha256}' \
		>"$RAW_ROOT/browser-input.json"
	write_material_file "browser-input.json" "$RAW_ROOT/browser-input.json"
}

copy_fixture() {
	set_checkpoint "fixture_copy"
	[[ ! -e "$DEMO_REPOSITORY" ]] || die "demo_repository_exists" "受控 demo Repository 路径已经存在"
	mkdir -m 700 "$DEMO_REPOSITORY"
	tar -C "$SOURCE_ROOT" -cf - testdata/golden-path-demo | tar -C "$DEMO_REPOSITORY" --strip-components=2 -xf -
	[[ ! -e "$DEMO_REPOSITORY/.git" ]] || die "fixture_contains_git" "GoldenPathFixture 不能携带嵌套 Git"
	[[ -f "$DEMO_REPOSITORY/.keystone/project.yaml" ]] || die "fixture_manifest_missing" "Fixture 缺少 ProjectManifest V2"
}

initialize_demo_repository() {
	set_checkpoint "fixture_seed"
	local hooks_path="$RUN_ROOT/git-hooks"
	mkdir -m 700 "$hooks_path"
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" init -b main >"$RAW_ROOT/git-init.stdout" 2>"$RAW_ROOT/git-init.stderr"
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" config --local user.name "Keystone Golden Path"
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" config --local user.email "golden-path@invalid"
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" config --local commit.gpgSign false
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" config --local core.hooksPath "$hooks_path"
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" add --all
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" commit --no-verify -m "golden path base" >"$RAW_ROOT/git-seed.stdout" 2>"$RAW_ROOT/git-seed.stderr"
	BASE_REVISION=$(GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" rev-parse HEAD)
	local status_output
	status_output=$(GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" status --porcelain=v1 --untracked-files=all --ignore-submodules=none)
	[[ -z "$status_output" ]] || die "fixture_seed_dirty" "Fixture seed 后 Repository 不干净"
	write_material_text "demo-base.txt" "base_revision=$BASE_REVISION\n"
}

build_demo_service() {
	local source_directory=$1 output=$2
	(
		cd "$source_directory"
		go build -o "$output" .
	)
}

start_demo_service() {
	local service_name=$1 source_directory=$2
	local output="$RAW_ROOT/$service_name.stdout" error="$RAW_ROOT/$service_name.stderr" binary="$RAW_ROOT/$service_name.bin"
	build_demo_service "$source_directory" "$binary" || die "demo_build_failed" "$service_name demo service 构建失败"
	(
		cd "$source_directory"
		exec setsid "$binary" --listen 127.0.0.1:0
	) >"$output" 2>"$error" &
	if [[ "$service_name" == "base" ]]; then
		BASE_SERVICE_PID=$!
	else
		CANDIDATE_SERVICE_PID=$!
	fi
	local deadline=$(( $(date +%s) + 30 )) line=""
	while (( $(date +%s) <= deadline )); do
		if [[ -s "$output" ]]; then
			line=$(sed -n '1p' "$output")
			if [[ -n "$line" ]]; then
				break
			fi
		fi
		sleep 1
	done
	[[ -n "$line" ]] || die "demo_startup_timeout" "$service_name demo service 未在预算内宣告监听地址"
	local event address
	event=$(jq -r '.event // empty' <<<"$line" 2>/dev/null || true)
	address=$(jq -r '.address // empty' <<<"$line" 2>/dev/null || true)
	[[ "$event" == "demo_listening" && "$address" =~ ^127\.0\.0\.1:[1-9][0-9]*$ ]] || die "demo_startup_protocol_invalid" "$service_name demo service startup JSON 无效"
	if [[ "$service_name" == "base" ]]; then
		BASE_SERVICE_ENDPOINT=$address
	else
		CANDIDATE_SERVICE_ENDPOINT=$address
	fi
}

stop_owned_process() {
	local pid=$1
	[[ -n "$pid" ]] || return 0
	if kill -0 "$pid" 2>/dev/null; then
		kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
		local deadline=$(( $(date +%s) + 10 ))
		while kill -0 "$pid" 2>/dev/null && (( $(date +%s) <= deadline )); do
			sleep 1
		done
		if kill -0 "$pid" 2>/dev/null; then
			kill -KILL -- "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
		fi
	fi
	wait "$pid" 2>/dev/null || true
}

check_base_demo() {
	set_checkpoint "base_demo_observation"
	local base_source
	base_source=$(prepare_revision_archive "$BASE_REVISION" base)
	start_demo_service base "$base_source"
	local root_body="$RAW_ROOT/base-root.body" health_body="$RAW_ROOT/base-health.body"
	local root_status health_status
	root_status=$(curl -sS --connect-timeout 2 --max-time 10 -o "$root_body" -w '%{http_code}' "http://$BASE_SERVICE_ENDPOINT/" 2>"$RAW_ROOT/base-root.err") || die "demo_http_failed" "Base demo GET / 失败"
	health_status=$(curl -sS --connect-timeout 2 --max-time 10 -o "$health_body" -w '%{http_code}' "http://$BASE_SERVICE_ENDPOINT/healthz" 2>"$RAW_ROOT/base-health.err") || die "demo_http_failed" "Base demo health probe 失败"
	[[ "$root_status" == 200 ]] || die "demo_baseline_invalid" "Base demo GET / 不是 HTTP 200"
	[[ "$(sha256_file "$root_body")" == "$(sha256_text hello)" ]] || die "demo_baseline_invalid" "Base demo GET / body 不是 hello"
	[[ "$health_status" == 404 ]] || die "demo_baseline_invalid" "Base demo 不应预置业务 health endpoint"
	jq -n --arg root_status "$root_status" --arg root_body_sha256 "$(sha256_file "$root_body")" --arg health_status "$health_status" '{root_status:($root_status|tonumber),root_body_sha256:$root_body_sha256,health_status:($health_status|tonumber)}' >"$RAW_ROOT/base-http.json"
	write_material_file "base-http.json" "$RAW_ROOT/base-http.json"
	stop_owned_process "$BASE_SERVICE_PID"
	BASE_SERVICE_PID=""
}

run_captured() {
	local output=$1 error=$2 status=0
	shift 2
	set +e
	"$@" >"$output" 2>"$error"
	status=$?
	set -e
	return "$status"
}

run_in_directory() {
	local directory=$1 output=$2 error=$3 status=0
	shift 3
	set +e
	(
		cd "$directory"
		"$@"
	) >"$output" 2>"$error"
	status=$?
	set -e
	return "$status"
}

build_keystone_source() {
	set_checkpoint "keystone_source_validation"
	local go_test_stdout="$RAW_ROOT/go-test.stdout" go_test_stderr="$RAW_ROOT/go-test.stderr"
	local go_vet_stdout="$RAW_ROOT/go-vet.stdout" go_vet_stderr="$RAW_ROOT/go-vet.stderr"
	local go_build_stdout="$RAW_ROOT/go-build.stdout" go_build_stderr="$RAW_ROOT/go-build.stderr"
	local npm_ci_stdout="$RAW_ROOT/npm-ci.stdout" npm_ci_stderr="$RAW_ROOT/npm-ci.stderr"
	local dashboard_stdout="$RAW_ROOT/dashboard-build.stdout" dashboard_stderr="$RAW_ROOT/dashboard-build.stderr"
	if ! run_in_directory "$SOURCE_ROOT" "$go_test_stdout" "$go_test_stderr" go test ./...; then
		die "source_validation_failed" "Keystone source go test 失败"
	fi
	if ! run_in_directory "$SOURCE_ROOT" "$go_vet_stdout" "$go_vet_stderr" go vet ./...; then
		die "source_validation_failed" "Keystone source go vet 失败"
	fi
	if ! run_in_directory "$SOURCE_ROOT" "$go_build_stdout" "$go_build_stderr" go build ./...; then
		die "source_validation_failed" "Keystone source go build 失败"
	fi
	if ! run_in_directory "$SOURCE_ROOT/dashboard" "$npm_ci_stdout" "$npm_ci_stderr" npm ci --ignore-scripts; then
		die "dashboard_build_failed" "Dashboard npm ci 失败"
	fi
	if ! run_in_directory "$SOURCE_ROOT/dashboard" "$dashboard_stdout" "$dashboard_stderr" npm run build; then
		die "dashboard_build_failed" "Dashboard production build 失败"
	fi
	for binary in keystone keystone-daemon keystone-worker; do
		local output="$BIN_ROOT/$binary" stdout="$RAW_ROOT/$binary-build.stdout" stderr="$RAW_ROOT/$binary-build.stderr"
		if ! run_in_directory "$SOURCE_ROOT" "$stdout" "$stderr" go build -o "$output" "./cmd/$binary"; then
			die "keystone_build_failed" "$binary 构建失败"
		fi
		chmod 700 "$output"
	done
	jq -n \
		--arg revision "$KEYSTONE_REVISION" \
		--arg go_version "$(go version | sed 's/[[:space:]]\+/ /g')" \
		--arg keystone_sha256 "$(sha256_file "$BIN_ROOT/keystone")" \
		--arg daemon_sha256 "$(sha256_file "$BIN_ROOT/keystone-daemon")" \
		--arg worker_sha256 "$(sha256_file "$BIN_ROOT/keystone-worker")" \
		--arg dashboard_dist_sha256 "$(safe_directory_digest "$SOURCE_ROOT/dashboard/dist")" \
		'{keystone_revision:$revision,go_version:$go_version,binaries:{keystone:$keystone_sha256,keystone_daemon:$daemon_sha256,keystone_worker:$worker_sha256},dashboard_production_build_sha256:$dashboard_dist_sha256,source_checks:["go test ./...","go vet ./...","go build ./...","npm ci --ignore-scripts","npm run build"]}' \
		>"$RAW_ROOT/keystone-source.json"
	write_material_file "keystone-source.json" "$RAW_ROOT/keystone-source.json"
}

http_request() {
	local method=$1 path=$2 body=$3 output=$4 status_text
	local -a args=(curl -sS --connect-timeout 2 --max-time 15 -X "$method" -H 'Accept: application/json' -o "$output" -w '%{http_code}')
	if [[ -n "$body" ]]; then
		args+=(-H 'Content-Type: application/json' --data-binary "@$body")
	fi
	if status_text=$("${args[@]}" "$DAEMON_ENDPOINT$path" 2>"$output.err"); then
		HTTP_STATUS=$status_text
		return 0
	fi
	HTTP_STATUS=000
	return 1
}

query_public() {
	local path=$1 output=$2 attempt
	for attempt in 1 2 3 4 5; do
		if http_request GET "$path" "" "$output" && [[ "$HTTP_STATUS" == 200 ]]; then
			return 0
		fi
		if [[ "$HTTP_STATUS" != 000 && "$HTTP_STATUS" != 502 && "$HTTP_STATUS" != 503 && "$HTTP_STATUS" != 504 ]]; then
			return 1
		fi
		sleep "$attempt"
	done
	return 1
}

record_query() {
	local name=$1 path=$2 raw="$RAW_ROOT/$1.json" material="$MATERIALS_ROOT/$1.json"
	query_public "$path" "$raw" || die "public_query_failed" "公开 Query 失败"
	safe_json_projection "$raw" "$material" || die "unsafe_projection_failed" "公开 Query 安全投影失败"
}

public_write() {
	local method=$1 path=$2 body=$3 output=$4 attempt
	for attempt in 1 2 3 4; do
		if http_request "$method" "$path" "$body" "$output" && [[ "$HTTP_STATUS" =~ ^2[0-9][0-9]$ ]]; then
			return 0
		fi
		if [[ "$HTTP_STATUS" == 409 ]]; then
			die "public_write_conflict" "公开写入返回冲突；Runner 不创建新逻辑命令"
		fi
		if [[ "$HTTP_STATUS" != 000 && "$HTTP_STATUS" != 502 && "$HTTP_STATUS" != 503 && "$HTTP_STATUS" != 504 ]]; then
			return 1
		fi
		sleep "$attempt"
	done
	return 1
}

next_ledger_entry() {
	local operation=$1 scope=$2 expected_version=$3 key=$4 request_digest=$5
	LEDGER_SEQUENCE=$((LEDGER_SEQUENCE + 1))
	write_ledger "$LEDGER_SEQUENCE" "$operation" "$scope" "$expected_version" "$key" "$request_digest"
}

start_daemon() {
	set_checkpoint "daemon_startup"
	local output="$RAW_ROOT/daemon.stdout" error="$RAW_ROOT/daemon.stderr" metadata="$STATE_ROOT/runtime/instance.json"
	(
		cd "$SOURCE_ROOT"
		exec setsid "$BIN_ROOT/keystone-daemon" --data-dir "$STATE_ROOT" --dashboard-dir "$SOURCE_ROOT/dashboard/dist"
	) >"$output" 2>"$error" &
	DAEMON_PID=$!
	local deadline=$(( $(date +%s) + 60 ))
	while (( $(date +%s) <= deadline )); do
		if [[ -f "$metadata" ]]; then
			DAEMON_ENDPOINT=$(jq -r '.endpoint // empty' "$metadata" 2>/dev/null || true)
			DAEMON_INSTANCE_ID=$(jq -r '.instance_id // empty' "$metadata" 2>/dev/null || true)
			if [[ "$DAEMON_ENDPOINT" =~ ^127\.0\.0\.1:[1-9][0-9]*$ && "$DAEMON_INSTANCE_ID" =~ ^[0-9a-f-]{36}$ ]]; then
				break
			fi
		fi
		kill -0 "$DAEMON_PID" 2>/dev/null || die "daemon_startup_failed" "Daemon 进程提前退出"
		sleep 1
	done
	[[ -n "$DAEMON_ENDPOINT" ]] || die "daemon_startup_timeout" "Daemon 未在 60 秒内发布 RuntimeMetadata"
	local status_raw="$RAW_ROOT/daemon-status.json" status=0
	while (( $(date +%s) <= deadline )); do
		if http_request GET /v1/daemon/status "" "$status_raw" && [[ "$HTTP_STATUS" == 200 ]]; then
			if [[ "$(jq -r '.daemon_readiness // false' "$status_raw")" == true && "$(jq -r '.daemon_instance_id // empty' "$status_raw")" == "$DAEMON_INSTANCE_ID" ]]; then
				status=1
				break
			fi
		fi
		sleep 1
	done
	[[ "$status" == 1 ]] || die "daemon_readiness_timeout" "Daemon status 未在 60 秒内交叉校验通过"
	safe_json_projection "$status_raw" "$MATERIALS_ROOT/daemon-status.json"
}

stop_daemon_public() {
	[[ -n "$DAEMON_PID" ]] || return 0
	if [[ -n "$DAEMON_ENDPOINT" && -n "$DAEMON_INSTANCE_ID" ]]; then
		local body="$RAW_ROOT/daemon-stop.request.json" response="$RAW_ROOT/daemon-stop.response.json" attempt stopped=0
		jq -n --arg id "$DAEMON_INSTANCE_ID" '{daemon_instance_id:$id}' >"$body"
		next_ledger_entry "daemon.stop" "daemon" "0" "$DAEMON_INSTANCE_ID" "$(sha256_file "$body")"
		for attempt in 1 2 3 4; do
			if http_request POST /v1/daemon/stop "$body" "$response" && [[ "$HTTP_STATUS" == 200 ]]; then
				stopped=1
				break
			fi
			[[ "$HTTP_STATUS" == 000 || "$HTTP_STATUS" == 502 || "$HTTP_STATUS" == 503 || "$HTTP_STATUS" == 504 ]] || break
			sleep "$attempt"
		done
		if [[ "$stopped" != 1 ]]; then
			write_material_text "shutdown.json" '{"public_stop":"not_confirmed","process_tree_termination_attempted":true}'
		fi
	fi
	local deadline=$(( $(date +%s) + 30 ))
	while kill -0 "$DAEMON_PID" 2>/dev/null && (( $(date +%s) <= deadline )); do
		sleep 1
	done
	if kill -0 "$DAEMON_PID" 2>/dev/null; then
		stop_owned_process "$DAEMON_PID"
	else
		kill -TERM -- "-$DAEMON_PID" 2>/dev/null || true
		wait "$DAEMON_PID" 2>/dev/null || true
	fi
	DAEMON_PID=""
}

project_init() {
	set_checkpoint "project_init"
	local body="$RAW_ROOT/project-init.request.json" response="$RAW_ROOT/project-init.response.json" key
	key=$(uuid_v7)
	jq -n --arg repository_path "$DEMO_REPOSITORY" '{repository_path:$repository_path}' >"$body"
	next_ledger_entry "project.init" "project" "0" "$key" "$(sha256_file "$body")"
	public_write POST /v1/projects/init "$body" "$response" || die "project_init_failed" "Project init 公开 API 失败"
	[[ "$HTTP_STATUS" == 200 ]] || die "project_init_failed" "Project init 未返回 HTTP 200"
	PROJECT_ID=$(jq -r '.project.project_id // empty' "$response")
	[[ "$PROJECT_ID" =~ ^[0-9a-f-]{36}$ ]] || die "project_init_failed" "Project init 响应缺少 ProjectID"
	safe_json_projection "$response" "$MATERIALS_ROOT/project-init.json"
	record_query "project-query" "/v1/projects/$PROJECT_ID"
	record_query "project-events" "/v1/projects/$PROJECT_ID/events"
}

run_cli() {
	local output=$1 error=$2 status=0
	shift 2
	set +e
	(
		cd "$SOURCE_ROOT"
		GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 "$BIN_ROOT/keystone" --data-dir "$STATE_ROOT" "$@"
	) >"$output" 2>"$error"
	status=$?
	set -e
	return "$status"
}

show_change() {
	local output="$RAW_ROOT/change-show.json" error="$RAW_ROOT/change-show.err"
	run_cli "$output" "$error" change show "$CHANGE_ID" || die "change_query_failed" "Change show 失败"
	CHANGE_VERSION=$(jq -r '.change.version // 0' "$output")
	CHANGE_STATUS=$(jq -r '.change.status // empty' "$output")
	[[ "$CHANGE_VERSION" =~ ^[1-9][0-9]*$ ]] || die "change_query_failed" "Change show 缺少有效 version"
	safe_json_projection "$output" "$MATERIALS_ROOT/change-show.json"
}

create_change() {
	set_checkpoint "change_create"
	local output="$RAW_ROOT/change-create.json" error="$RAW_ROOT/change-create.err" body="$RAW_ROOT/change-create.request.json" key
	key=$(uuid_v7)
	jq -n --arg repository_path "$DEMO_REPOSITORY" --arg intent "$CHANGE_INTENT" '{repository_path:$repository_path,intent:$intent}' >"$body"
	next_ledger_entry "change.create" "change" "0" "$key" "$(sha256_file "$body")"
	if ! run_cli "$output" "$error" change create --repository-path "$DEMO_REPOSITORY" --intent "$CHANGE_INTENT" --idempotency-key "$key"; then
		die "change_create_failed" "Change create 失败"
	fi
	CHANGE_ID=$(jq -r '.change.change_id // empty' "$output")
	[[ "$CHANGE_ID" =~ ^[0-9a-f-]{36}$ ]] || die "change_create_failed" "Change create 响应缺少 ChangeID"
	CHANGE_VERSION=$(jq -r '.change.version // 0' "$output")
	safe_json_projection "$output" "$MATERIALS_ROOT/change-create.json"
}

query_change_trace() {
	set_checkpoint "public_trace_query"
	record_query "change-query" "/v1/changes/$CHANGE_ID"
	record_query "change-events" "/v1/changes/$CHANGE_ID/events"
	record_query "change-runs" "/v1/changes/$CHANGE_ID/runs"
	record_query "change-artifacts" "/v1/changes/$CHANGE_ID/artifacts"
	record_query "change-decisions" "/v1/changes/$CHANGE_ID/decisions"
	record_query "change-observation" "/v1/changes/$CHANGE_ID/observation"
	record_query "needs-human" "/v1/needs-human"
}

poll_ticket_graph() {
	set_checkpoint "ticket_graph_query"
	local output="$RAW_ROOT/ticket-graph.json" error="$RAW_ROOT/ticket-graph.err" deadline=$(( $(date +%s) + 1800 ))
	while (( $(date +%s) <= deadline )); do
		show_change
		if [[ "$CHANGE_STATUS" == "failed" || "$CHANGE_STATUS" == "cancelled" || "$CHANGE_STATUS" == "human_required" ]]; then
			die "planning_terminal_failure" "Planning 在形成 Canonical Ticket Graph 前进入终态"
		fi
		if run_cli "$output" "$error" change ticket-graph "$CHANGE_ID" && jq -e '.tickets | length > 0' "$output" >/dev/null 2>&1; then
			safe_json_projection "$output" "$MATERIALS_ROOT/ticket-graph.json"
			return 0
		fi
		sleep 2
	done
	die "ticket_graph_timeout" "Canonical Ticket Graph 未在预算内可查询"
}

query_execution() {
	local output="$RAW_ROOT/execution.json" error="$RAW_ROOT/execution.err"
	run_cli "$output" "$error" change execution show "$CHANGE_ID" || return 1
	safe_json_projection "$output" "$MATERIALS_ROOT/execution.json" || die "unsafe_projection_failed" "Execution Query 安全投影失败"
}

wait_ticket_state() {
	local ticket_id=$1 target=$2 deadline=$(( $(date +%s) + 1800 )) state
	while (( $(date +%s) <= deadline )); do
		if query_execution; then
			state=$(jq -r --arg ticket_id "$ticket_id" '.tickets[]? | select(.ticket_id == $ticket_id) | .state' "$RAW_ROOT/execution.json" | head -n 1)
			if [[ "$state" == "$target" ]]; then
				return 0
			fi
			if [[ "$state" == "human_required" || "$state" == "failed" || "$state" == "cancelled" ]]; then
				die "execution_terminal_failure" "Ticket 执行进入不可自动恢复状态"
			fi
		fi
		sleep 2
	done
	die "execution_timeout" "Ticket 执行未在预算内达到目标状态"
}

wait_ticket_verification() {
	local ticket_id=$1 deadline=$(( $(date +%s) + 900 )) status
	while (( $(date +%s) <= deadline )); do
		if query_execution; then
			status=$(jq -r --arg ticket_id "$ticket_id" '.tickets[]? | select(.ticket_id == $ticket_id) | .verification_status' "$RAW_ROOT/execution.json" | head -n 1)
			case "$status" in
			pass) return 0 ;;
			human_required|fail|unavailable) die "verification_terminal_failure" "Ticket Verify 未形成 PASS" ;;
			esac
		fi
		sleep 2
	done
	die "verification_timeout" "Ticket Verify 未在预算内达到 PASS"
}

wait_ticket_commit() {
	local ticket_id=$1 deadline=$(( $(date +%s) + 900 )) status after_revision
	while (( $(date +%s) <= deadline )); do
		if query_execution; then
			status=$(jq -r --arg ticket_id "$ticket_id" '.tickets[]? | select(.ticket_id == $ticket_id) | .commit_status' "$RAW_ROOT/execution.json" | head -n 1)
			after_revision=$(jq -r --arg ticket_id "$ticket_id" '.tickets[]? | select(.ticket_id == $ticket_id) | .commit_after_revision // empty' "$RAW_ROOT/execution.json" | head -n 1)
			if [[ "$status" == "committed" && "$after_revision" =~ ^[0-9a-f]{40,64}$ ]]; then
				return 0
			fi
			if [[ "$status" == "human_required" || "$status" == "failed" ]]; then
				die "commit_terminal_failure" "Ticket Commit 未形成可复核 Commit"
			fi
		fi
		sleep 2
	done
	die "commit_timeout" "Ticket Commit 未在预算内完成"
}

query_ticket_diff() {
	local ticket_id=$1 ordinal=$2 refs ref_id ref_output
	record_query "change-artifacts" "/v1/changes/$CHANGE_ID/artifacts"
	refs=$(jq -r '.artifacts[]? | select(.kind == "diff") | [.artifact_ref_id,.artifact_id,.kind] | @tsv' "$RAW_ROOT/change-artifacts.json")
	[[ -n "$refs" ]] || die "diff_missing" "Ticket 未形成 diff Artifact"
	while IFS=$'\t' read -r ref_id artifact_id kind; do
		[[ -n "$ref_id" ]] || continue
		ref_output="$RAW_ROOT/ticket-$ordinal-diff-$ref_id.json"
		query_public "/v1/changes/$CHANGE_ID/artifacts/$ref_id/content" "$ref_output" || die "diff_missing" "diff Artifact content 不可 Query"
		[[ "$HTTP_STATUS" == 200 ]] || die "diff_missing" "diff Artifact content 未返回 HTTP 200"
		jq -n \
			--arg ticket_id "$ticket_id" \
			--arg artifact_ref_id "$ref_id" \
			--arg artifact_id "$artifact_id" \
			--arg kind "$kind" \
			--arg content_sha256 "$(sha256_file "$ref_output")" \
			'{ticket_id:$ticket_id,artifact_ref_id:$artifact_ref_id,artifact_id:$artifact_id,kind:$kind,content_sha256:$content_sha256}' \
			>"$MATERIALS_ROOT/ticket-$ordinal-diff.json"
		return 0
	done <<<"$refs"
	die "diff_missing" "diff Artifact content 未形成"
}

execute_change() {
	set_checkpoint "change_execute"
	show_change
	local key output="$RAW_ROOT/change-execute.json" error="$RAW_ROOT/change-execute.err" body="$RAW_ROOT/change-execute.request.json"
	key=$(uuid_v7)
	jq -n --argjson expected_version "$CHANGE_VERSION" '{expected_version:$expected_version}' >"$body"
	next_ledger_entry "change.execute" "change/$CHANGE_ID" "$CHANGE_VERSION" "$key" "$(sha256_file "$body")"
	if ! run_cli "$output" "$error" change execute "$CHANGE_ID" --expected-version "$CHANGE_VERSION" --idempotency-key "$key"; then
		die "change_execute_failed" "Change Execute 失败"
	fi
	jq -e --arg change_id "$CHANGE_ID" '.execution.change_id == $change_id' "$output" >/dev/null || die "change_execute_failed" "Execute 回执缺少 Execution ReadModel"
	safe_json_projection "$output" "$MATERIALS_ROOT/change-execute.json"

	local graph_rows ticket_id ordinal
	graph_rows=$(jq -r '.tickets | sort_by(.ordinal)[] | [.ticket_id,.ordinal] | @tsv' "$RAW_ROOT/ticket-graph.json")
	[[ -n "$graph_rows" ]] || die "ticket_graph_empty" "Canonical Ticket Graph 没有 Ticket"
	while IFS=$'\t' read -r ticket_id ordinal; do
		[[ -n "$ticket_id" && -n "$ordinal" ]] || continue
		set_checkpoint "ticket_${ordinal}_execution"
		wait_ticket_state "$ticket_id" succeeded
		query_ticket_diff "$ticket_id" "$ordinal"

		set_checkpoint "ticket_${ordinal}_verify"
		show_change
		key=$(uuid_v7)
		body="$RAW_ROOT/ticket-$ordinal-verify.request.json"
		jq -n --argjson expected_version "$CHANGE_VERSION" '{expected_version:$expected_version}' >"$body"
		next_ledger_entry "ticket.verify" "change/$CHANGE_ID/ticket/$ticket_id" "$CHANGE_VERSION" "$key" "$(sha256_file "$body")"
		output="$RAW_ROOT/ticket-$ordinal-verify.json"
		error="$RAW_ROOT/ticket-$ordinal-verify.err"
		if ! run_cli "$output" "$error" change verify "$CHANGE_ID" "$ticket_id" --expected-version "$CHANGE_VERSION" --idempotency-key "$key"; then
			die "ticket_verify_failed" "Ticket Verify 失败"
		fi
		safe_json_projection "$output" "$MATERIALS_ROOT/ticket-$ordinal-verify.json"
		wait_ticket_verification "$ticket_id"

		set_checkpoint "ticket_${ordinal}_commit"
		show_change
		key=$(uuid_v7)
		body="$RAW_ROOT/ticket-$ordinal-commit.request.json"
		jq -n --argjson expected_version "$CHANGE_VERSION" '{expected_version:$expected_version}' >"$body"
		next_ledger_entry "ticket.commit" "change/$CHANGE_ID/ticket/$ticket_id" "$CHANGE_VERSION" "$key" "$(sha256_file "$body")"
		output="$RAW_ROOT/ticket-$ordinal-commit.json"
		error="$RAW_ROOT/ticket-$ordinal-commit.err"
		if ! run_cli "$output" "$error" change commit "$CHANGE_ID" "$ticket_id" --expected-version "$CHANGE_VERSION" --idempotency-key "$key"; then
			die "ticket_commit_failed" "Ticket Commit 失败"
		fi
		jq -e --arg ticket_id "$ticket_id" '.execution.tickets[]? | select(.ticket_id == $ticket_id) | .commit_after_revision // empty | test("^[0-9a-f]{40,64}$")' "$output" >/dev/null || die "ticket_commit_failed" "Commit 回执缺少 after revision"
		safe_json_projection "$output" "$MATERIALS_ROOT/ticket-$ordinal-commit.json"
		wait_ticket_commit "$ticket_id"
	done <<<"$graph_rows"
}

final_verify_change() {
	set_checkpoint "final_verify"
	show_change
	local key output="$RAW_ROOT/final-verify.json" error="$RAW_ROOT/final-verify.err" body="$RAW_ROOT/final-verify.request.json"
	key=$(uuid_v7)
	jq -n --argjson expected_version "$CHANGE_VERSION" '{expected_version:$expected_version}' >"$body"
	next_ledger_entry "change.final_verify" "change/$CHANGE_ID" "$CHANGE_VERSION" "$key" "$(sha256_file "$body")"
	if ! run_cli "$output" "$error" change final-verify "$CHANGE_ID" --expected-version "$CHANGE_VERSION" --idempotency-key "$key"; then
		die "final_verify_failed" "Change FinalVerify 失败"
	fi
	safe_json_projection "$output" "$MATERIALS_ROOT/final-verify.json"
	local deadline=$(( $(date +%s) + 900 )) final_status candidate
	while (( $(date +%s) <= deadline )); do
		if query_execution; then
			final_status=$(jq -r '.final_verification // empty' "$RAW_ROOT/execution.json")
			candidate=$(jq -r '.candidate_revision // empty' "$RAW_ROOT/execution.json")
			if [[ "$final_status" == "pass" && "$candidate" =~ ^[0-9a-f]{40,64}$ ]]; then
				CANDIDATE_REVISION=$candidate
				return 0
			fi
			if [[ "$final_status" == "fail" || "$final_status" == "human_required" || "$final_status" == "unavailable" ]]; then
				die "final_verify_terminal_failure" "FinalVerify 未形成 PASS"
			fi
		fi
		sleep 2
	done
	die "final_verify_timeout" "FinalVerify 未在预算内完成"
}

prepare_revision_archive() {
	local revision=$1 name=$2 output="$RUN_ROOT/$2-source"
	mkdir -m 700 "$output"
	GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" archive --format=tar "$revision" | tar -x -C "$output"
	printf '%s\n' "$output"
}

verify_candidate_service() {
	set_checkpoint "candidate_acceptance"
	local candidate_source
	candidate_source=$(prepare_revision_archive "$CANDIDATE_REVISION" candidate)
	local base_manifest_sha candidate_manifest_sha changed_files
	base_manifest_sha=$(sha256_file "$RUN_ROOT/base-source/.keystone/project.yaml")
	candidate_manifest_sha=$(sha256_file "$candidate_source/.keystone/project.yaml")
	[[ "$candidate_manifest_sha" == "$base_manifest_sha" ]] || die "candidate_manifest_changed" "Candidate 修改了 .keystone/project.yaml"
	changed_files=$(GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git -C "$DEMO_REPOSITORY" diff --name-only "$BASE_REVISION" "$CANDIDATE_REVISION")
	printf '%s\n' "$changed_files" >"$RAW_ROOT/candidate-changed-files.txt"
	if grep -Fxq '.keystone/project.yaml' "$RAW_ROOT/candidate-changed-files.txt"; then
		die "candidate_manifest_changed" "Candidate revision 的变更列表包含 ProjectManifest"
	fi
	write_material_text "candidate-changed-files.txt" "$changed_files"
	local test_stdout="$RAW_ROOT/candidate-go-test.stdout" test_stderr="$RAW_ROOT/candidate-go-test.stderr"
	if ! run_in_directory "$candidate_source" "$test_stdout" "$test_stderr" go test ./...; then
		die "candidate_test_failed" "Candidate revision go test 失败"
	fi
	if ! rg -n --fixed-strings '/healthz' "$candidate_source/main_test.go" >"$RAW_ROOT/candidate-healthz-test-match.txt" 2>"$RAW_ROOT/candidate-healthz-test-match.err"; then
		die "candidate_test_missing" "Candidate revision 没有直接覆盖 /healthz 的自动化测试"
	fi
	start_demo_service candidate "$candidate_source"
	local root_body="$RAW_ROOT/candidate-root.body" root_status health_body="$RAW_ROOT/candidate-health.body" health_headers="$RAW_ROOT/candidate-health.headers" health_status content_type
	root_status=$(curl -sS --connect-timeout 2 --max-time 10 -o "$root_body" -w '%{http_code}' "http://$CANDIDATE_SERVICE_ENDPOINT/" 2>"$RAW_ROOT/candidate-root.err") || die "candidate_http_failed" "Candidate GET / 失败"
	health_status=$(curl -sS --connect-timeout 2 --max-time 10 -D "$health_headers" -o "$health_body" -w '%{http_code}' "http://$CANDIDATE_SERVICE_ENDPOINT/healthz" 2>"$RAW_ROOT/candidate-health.err") || die "candidate_http_failed" "Candidate GET /healthz 失败"
	content_type=$(sed -n 's/^[Cc]ontent-[Tt]ype:[[:space:]]*//p' "$health_headers" | tr -d '\r' | tail -n 1)
	[[ "$root_status" == 200 ]] || die "candidate_acceptance_failed" "Candidate GET / 不是 HTTP 200"
	[[ "$(sha256_file "$root_body")" == "$(sha256_text hello)" ]] || die "candidate_acceptance_failed" "Candidate GET / body 不是 hello"
	[[ "$health_status" == 200 ]] || die "candidate_acceptance_failed" "Candidate GET /healthz 不是 HTTP 200"
	[[ "$content_type" == application/json* ]] || die "candidate_acceptance_failed" "Candidate /healthz media type 不是 application/json"
	jq -e 'type == "object" and (keys | length == 1) and .status == "ok"' "$health_body" >/dev/null || die "candidate_acceptance_failed" "Candidate /healthz JSON 不是精确 status=ok"
	jq -n \
		--arg candidate_revision "$CANDIDATE_REVISION" \
		--arg root_status "$root_status" \
		--arg root_body_sha256 "$(sha256_file "$root_body")" \
		--arg health_status "$health_status" \
		--arg content_type "$content_type" \
		--arg health_body_sha256 "$(sha256_file "$health_body")" \
		'{candidate_revision:$candidate_revision,root:{status:($root_status|tonumber),body_sha256:$root_body_sha256},healthz:{status:($health_status|tonumber),content_type:$content_type,json_sha256:$health_body_sha256}}' \
		>"$MATERIALS_ROOT/candidate-http.json"
	stop_owned_process "$CANDIDATE_SERVICE_PID"
	CANDIDATE_SERVICE_PID=""
}

run_dashboard_observation() {
	set_checkpoint "dashboard_observation"
	local output="$RAW_ROOT/dashboard-driver.json" error="$RAW_ROOT/dashboard-driver.err"
	mkdir -m 700 "$RAW_ROOT/screenshots"
	set +e
	(
		cd "$SOURCE_ROOT/dashboard"
		env "GP_DAEMON_ORIGIN=http://$DAEMON_ENDPOINT" \
			"GP_PROJECT_ID=$PROJECT_ID" \
			"GP_CHANGE_ID=$CHANGE_ID" \
			"GP_SCREENSHOT_DIR=$RAW_ROOT/screenshots" \
			"PLAYWRIGHT_BROWSERS_PATH=$PLAYWRIGHT_BROWSERS_PATH" \
			node --input-type=module -
	) >"$output" 2>"$error" <<'EOF'
import { chromium } from "playwright";

const origin = process.env.GP_DAEMON_ORIGIN;
const projectID = process.env.GP_PROJECT_ID;
const changeID = process.env.GP_CHANGE_ID;
const screenshotDir = process.env.GP_SCREENSHOT_DIR;
const pages = [
  ["dashboard", "/"],
  ["project", `/projects/${projectID}`],
  ["change", `/changes/${changeID}`],
  ["needs-human", "/needs-human"],
];

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
const requests = [];
page.on("request", (request) => {
  const url = new URL(request.url());
  if (url.origin === origin && url.pathname.startsWith("/v1/")) {
    requests.push({ method: request.method(), path: url.pathname });
  }
});

try {
  const observations = [];
  for (const [name, path] of pages) {
    const response = await page.goto(origin + path, { waitUntil: "domcontentloaded", timeout: 30000 });
    if (!response || !response.ok()) {
      throw new Error(`dashboard route failed: ${name}`);
    }
    await page.waitForTimeout(1000);
    await page.screenshot({ path: `${screenshotDir}/${name}.png`, fullPage: true });
    observations.push({ name, path, status: response.status() });
  }

  const updatesBefore = requests.filter((request) => request.path === "/v1/updates").length;
  const observationBeforeReload = requests.filter((request) => request.path === `/v1/changes/${changeID}/observation`).length;
  await page.context().setOffline(true);
  await page.waitForTimeout(1500);
  await page.context().setOffline(false);
  await page.waitForTimeout(3500);
  await page.reload({ waitUntil: "domcontentloaded", timeout: 30000 });
  await page.waitForTimeout(1000);
  const updatesAfter = requests.filter((request) => request.path === "/v1/updates").length;
  const observationAfterReload = requests.filter((request) => request.path === `/v1/changes/${changeID}/observation`).length;
  if (updatesBefore < 1 || updatesAfter < updatesBefore || observationAfterReload <= observationBeforeReload) {
    throw new Error("dashboard SSE/reload reconstruction was not observed");
  }
  process.stdout.write(JSON.stringify({
    pages: observations,
    api_query_count: requests.length,
    refresh_stream_connections: updatesAfter,
    observation_queries_before_reload: observationBeforeReload,
    observation_queries_after_reload: observationAfterReload,
  }));
} finally {
  await browser.close();
}
EOF
	local status=$?
	set -e
	if [[ "$status" != 0 ]]; then
		die "dashboard_observation_failed" "Dashboard production Playwright 观察失败"
	fi
	jq -e '.pages | length == 4' "$output" >/dev/null || die "dashboard_observation_failed" "Dashboard 未观察到四个深链接"
	local screenshots='[]' file name bytes digest
	while IFS= read -r file; do
		name=${file##*/}
		bytes=$(wc -c <"$file")
		digest=$(sha256_file "$file")
		screenshots=$(jq -c --arg name "$name" --arg digest "$digest" --argjson bytes "$bytes" '. + [{name:$name,sha256:$digest,bytes:$bytes}]' <<<"$screenshots")
	done < <(find "$RAW_ROOT/screenshots" -type f -name '*.png' -print | sort)
	jq --argjson screenshots "$screenshots" \
		--slurpfile browser "$MATERIALS_ROOT/browser-input.json" \
		--slurpfile source "$MATERIALS_ROOT/keystone-source.json" \
		'. + {screenshots:$screenshots,production_origin:"<daemon-loopback>",browser:$browser[0],production_build_sha256:$source[0].dashboard_production_build_sha256}' \
		"$output" >"$MATERIALS_ROOT/dashboard-observation.json"
	record_query "final-observation" "/v1/changes/$CHANGE_ID/observation"
}

create_review_packet() {
	local status=$1 outcome failure review_items='[]' file relative media bytes digest review_id unsigned_packet
	set +e
	if [[ -n "$CANDIDATE_SERVICE_PID" ]]; then
		stop_owned_process "$CANDIDATE_SERVICE_PID"
		CANDIDATE_SERVICE_PID=""
	fi
	if [[ -n "$BASE_SERVICE_PID" ]]; then
		stop_owned_process "$BASE_SERVICE_PID"
		BASE_SERVICE_PID=""
	fi
	stop_daemon_public
	outcome="failed"
	[[ "$status" == 0 && "$LAST_CHECKPOINT" == "completed" ]] && outcome="passed"
	failure=${FAILURE_CODE:-none}
	[[ "$outcome" == "passed" ]] && failure="none"
	review_id=$(uuid_v7)
	while IFS= read -r file; do
		relative=${file#"$MATERIALS_ROOT/"}
		case "$relative" in
			*.json) media='application/json' ;;
			*.txt) media='text/plain' ;;
			*) media='application/octet-stream' ;;
		esac
		bytes=$(wc -c <"$file")
		digest=$(sha256_file "$file")
		review_items=$(jq -c --arg name "$relative" --arg role "safe_material" --arg media "$media" --arg digest "$digest" --argjson bytes "$bytes" '. + [{name:$name,role:$role,media_type:$media,byte_length:$bytes,sha256:$digest}]' <<<"$review_items")
	done < <(find "$MATERIALS_ROOT" -maxdepth 1 -type f ! -name 'review-manifest.json' -print | sort)
	jq -n --argjson items "$review_items" '{items:$items}' >"$MATERIALS_ROOT/review-manifest.json"
	digest=$(sha256_file "$MATERIALS_ROOT/review-manifest.json")
	unsigned_packet="$RAW_ROOT/review-packet-input.json"
	jq -n \
		--arg review_id "$review_id" \
		--arg run_id "$RUN_ID" \
		--arg evidence_set_id "$EVIDENCE_SET_ID" \
		--arg status "$outcome" \
		--arg checkpoint "$LAST_CHECKPOINT" \
		--arg failure_code "$failure" \
		--argjson exit_code "$status" \
		--arg manifest_sha256 "$digest" \
		--arg platform "$PLATFORM" \
		'{schema_version:"golden-path-review-packet.v1",review_id:$review_id,run_id:$run_id,evidence_set_id:$evidence_set_id,platform:$platform,status:$status,last_checkpoint:$checkpoint,failure_code:$failure_code,exit_code:$exit_code,manifest_sha256:$manifest_sha256,success_evidence_published:false}' \
		>"$unsigned_packet"
	digest=$(sha256_file "$unsigned_packet")
	jq --arg packet_sha256 "$digest" '. + {packet_sha256:$packet_sha256}' "$unsigned_packet" >"$RUN_ROOT/review-packet.json"
	chmod -R go-rwx "$RUN_ROOT" 2>/dev/null || true
	PACKET_CREATED=1
}

on_exit() {
	local status=$1
	if [[ "$PACKET_CREATED" == 1 || -z "$MATERIALS_ROOT" || ! -d "$MATERIALS_ROOT" ]]; then
		return
	fi
	create_review_packet "$status"
}

main() {
	export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
	parse_args "$@"
	if [[ "$INTERNAL_RUN" == 0 ]]; then
		initial_entry
		return
	fi
	initialize_run_directories
	export GOCACHE="$CACHE_ROOT"
	canonical_entry
	probe_platform
	resolve_codex_binary
	probe_codex
	copy_fixture
	initialize_demo_repository
	check_base_demo
	build_keystone_source
	prepare_browser
	start_daemon
	project_init
	create_change
	query_change_trace
	poll_ticket_graph
	execute_change
	final_verify_change
	query_change_trace
	verify_candidate_service
	run_dashboard_observation
	set_checkpoint "completed"
}

trap 'on_exit "$?"' EXIT
main "$@"
