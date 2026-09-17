#!/usr/bin/env bash
# UI 変更かどうかを、spec の明示分類または変更対象パスから判定する。
#
# 明示分類は feature ディレクトリ内の spec.md / plan.md / tasks.md に置ける:
#   <!-- sdd-ui-change: yes -->
#   <!-- sdd-ui-change: no -->
#
# パス判定は git diff --name-only の出力を受ける想定で、UI の描画・配置・操作に関わる
# web/ 配下のファイルを UI 変更として扱う。依存は bash・grep・sed だけで、jq は使わない。

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./sdd-lib.sh
. "$script_dir/sdd-lib.sh"

usage_error() {
  printf 'sdd-ui-classify.sh: %s\n' "$1" >&2
  printf 'usage: sdd-ui-classify.sh [--root <repo_root>] [--feature <specs/NNN-name>] [--paths <file>]\n' >&2
  exit 2
}

root="."
feature=""
paths_file=""

while [ $# -gt 0 ]; do
  case "$1" in
    --root)
      shift
      [ $# -gt 0 ] || usage_error "--root に値がありません"
      root="$1"
      ;;
    --feature)
      shift
      [ $# -gt 0 ] || usage_error "--feature に値がありません"
      feature="$1"
      ;;
    --paths)
      shift
      [ $# -gt 0 ] || usage_error "--paths に値がありません"
      paths_file="$1"
      ;;
    *)
      usage_error "未知の引数: $1"
      ;;
  esac
  shift
done

[ -d "$root" ] || usage_error "--root のディレクトリがありません: $root"
if [ -n "$feature" ] && [ ! -d "$root/$feature" ]; then
  usage_error "--feature のディレクトリがありません: $root/$feature"
fi
if [ -n "$paths_file" ] && [ ! -r "$paths_file" ]; then
  usage_error "--paths のファイルが読めません: $paths_file"
fi

explicit=""
if [ -n "$feature" ]; then
  for doc in spec.md plan.md tasks.md; do
    file="$root/$feature/$doc"
    [ -f "$file" ] || continue
    found="$(sed -n 's/\r$//; s/.*sdd-ui-change:[[:space:]]*\(yes\|no\).*/\1/p' "$file" | sed -n '1p')"
    if [ -n "$found" ]; then
      explicit="$found"
      break
    fi
  done
fi

if [ "$explicit" = "yes" ]; then
  printf '%s\n' '{"ui_change":true,"source":"spec-explicit","matched_path":""}'
  exit 0
fi
if [ "$explicit" = "no" ]; then
  printf '%s\n' '{"ui_change":false,"source":"spec-explicit","matched_path":""}'
  exit 0
fi

matched=""
while IFS= read -r path; do
  path="${path%$'\r'}"
  [ -n "$path" ] || continue
  case "$path" in
    web/index.html|web/tailwind.config.ts|web/src/*.css|web/src/*.tsx|web/src/*.ts|web/src/**/*.css|web/src/**/*.tsx|web/src/**/*.ts)
      # api / preferences / theme の純粋な検査や生成型は、単独では画面変更とみなさない。
      case "$path" in
        web/src/api/*|web/src/preferences/*|web/src/theme/*|web/src/api/gen/*) continue ;;
      esac
      matched="$path"
      break
      ;;
    docs/screenshots/*|specs/*/assets/*)
      matched="$path"
      break
      ;;
  esac
done < "${paths_file:-/dev/stdin}"

if [ -n "$matched" ]; then
  esc="$(sdd_json_escape "$matched")"
  printf '{"ui_change":true,"source":"path","matched_path":"%s"}\n' "$esc"
else
  printf '%s\n' '{"ui_change":false,"source":"path","matched_path":""}'
fi
exit 0
