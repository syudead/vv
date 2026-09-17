#!/usr/bin/env bash
# tasks.md の Phase 領域分類から、implement が UI 専用ループを必要とするか判定する。
# 変更パスは implement 後の分類漏れ検出にだけ使う。依存は bash・awk・sed だけで、jq は使わない。

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./sdd-lib.sh
. "$script_dir/sdd-lib.sh"

usage_error() {
  printf 'sdd-ui-classify.sh: %s\n' "$1" >&2
  printf 'usage: sdd-ui-classify.sh [--root <repo_root>] --feature <specs/NNN-name> --phase <N> [--paths <file>]\n' >&2
  exit 2
}

classification_error() {
  printf 'sdd-ui-classify.sh: %s\n' "$1" >&2
  exit 3
}

root="."
feature=""
phase=""
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
    --phase)
      shift
      [ $# -gt 0 ] || usage_error "--phase に値がありません"
      phase="$1"
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
[ -n "$feature" ] || usage_error "--feature は必須です"
[ -n "$phase" ] || usage_error "--phase は必須です"
case "$phase" in *[!0-9]*|'') usage_error "--phase は 1 以上の整数です: $phase" ;; esac
[ "$phase" -gt 0 ] || usage_error "--phase は 1 以上の整数です: $phase"
if [ ! -d "$root/$feature" ]; then
  usage_error "--feature のディレクトリがありません: $root/$feature"
fi
if [ -n "$paths_file" ] && [ ! -r "$paths_file" ]; then
  usage_error "--paths のファイルが読めません: $paths_file"
fi

tasks="$root/$feature/tasks.md"
[ -f "$tasks" ] || classification_error "tasks.md がありません: $feature/tasks.md"

domain_lines="$(awk -v target="$phase" '
  {
    line = $0
    sub(/\r$/, "", line)
    if (line ~ /^## Phase [0-9]+:/) {
      rest = line
      sub(/^## Phase /, "", rest)
      current = (substr(rest, 1, index(rest, ":") - 1) + 0 == target)
      next
    }
    if (line ~ /^## /) { current = 0; next }
    if (current && line ~ /<!--[[:space:]]*sdd-domains:[^>]*-->/) {
      sub(/^.*sdd-domains:[[:space:]]*/, "", line)
      sub(/[[:space:]]*-->.*$/, "", line)
      print line
    }
  }
' "$tasks")"

line_count="$(printf '%s\n' "$domain_lines" | sed '/^$/d' | awk 'END { print NR + 0 }')"
[ "$line_count" -gt 0 ] || classification_error "Phase $phase に sdd-domains がありません"
[ "$line_count" -eq 1 ] || classification_error "Phase $phase に sdd-domains が複数あります"

domains=""
ui_planned=false
trimmed_domains="$(printf '%s' "$domain_lines" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
case "$trimmed_domains" in
  ,*|*,|*,,*) classification_error "Phase $phase の sdd-domains に空の分類があります" ;;
esac
old_ifs="$IFS"
IFS=','
for raw in $trimmed_domains; do
  domain="$(printf '%s' "$raw" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
  case "$domain" in
    frontend-ui|frontend-non-ui|backend|infrastructure|documentation) ;;
    '') classification_error "Phase $phase の sdd-domains に空の分類があります" ;;
    *) classification_error "Phase $phase の未知の domain: $domain" ;;
  esac
  case ",$domains," in *",$domain,"*) classification_error "Phase $phase の domain が重複しています: $domain" ;; esac
  domains="${domains:+$domains,}$domain"
  [ "$domain" = "frontend-ui" ] && ui_planned=true
done
IFS="$old_ifs"

domains_json=""
old_ifs="$IFS"
IFS=','
for domain in $domains; do
  domains_json="${domains_json:+$domains_json,}\"$domain\""
done
IFS="$old_ifs"

matched=""
if [ -n "$paths_file" ]; then
  while IFS= read -r path; do
    path="${path%$'\r'}"
    [ -n "$path" ] || continue
    case "$path" in
      *.test.ts|*.test.tsx|*.spec.ts|*.spec.tsx|*/__tests__/*) continue ;;
      web/index.html|web/tailwind.config.ts|web/src/*.css|web/src/*.tsx|web/src/*.ts|web/src/**/*.css|web/src/**/*.tsx|web/src/**/*.ts)
        # API・設定・生成型は、単独では UI 分類漏れとみなさない。
        case "$path" in
          web/src/api/*|web/src/theme/*|web/src/api/gen/*) continue ;;
        esac
        matched="$path"
        break
        ;;
    esac
  done < "$paths_file"
fi

if [ "$ui_planned" = true ]; then
  printf '{"ui_change":true,"source":"phase-domains","domains":[%s],"classification_mismatch":false,"matched_path":""}\n' "$domains_json"
elif [ -n "$matched" ]; then
  esc="$(sdd_json_escape "$matched")"
  printf '{"ui_change":true,"source":"path-safety-net","domains":[%s],"classification_mismatch":true,"matched_path":"%s"}\n' "$domains_json" "$esc"
else
  printf '{"ui_change":false,"source":"phase-domains","domains":[%s],"classification_mismatch":false,"matched_path":""}\n' "$domains_json"
fi
exit 0
