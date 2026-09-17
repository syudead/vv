#!/usr/bin/env bash
# tasks.md の Phase 領域分類から、implement が UI 専用ループを必要とするか判定する。
# 依存は bash・awk・sed だけで、jq は使わない。

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./sdd-lib.sh
. "$script_dir/sdd-lib.sh"

usage_error() {
  printf 'sdd-ui-classify.sh: %s\n' "$1" >&2
  printf 'usage: sdd-ui-classify.sh [--root <repo_root>] --feature <specs/NNN-name> --phase <N>\n' >&2
  exit 2
}

classification_error() {
  printf 'sdd-ui-classify.sh: %s\n' "$1" >&2
  exit 3
}

root="."
feature=""
phase=""

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
tasks="$root/$feature/tasks.md"
[ -f "$tasks" ] || classification_error "tasks.md がありません: $feature/tasks.md"

domain_result="$(awk -v target="$phase" '
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
      marker_count++
      if (marker_count == 1) marker_value = line
    }
  }
  END { printf "%d\n%s\n", marker_count + 0, marker_value }
' "$tasks")"

line_count="$(printf '%s\n' "$domain_result" | sed -n '1p')"
domain_lines="$(printf '%s\n' "$domain_result" | sed -n '2p')"
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

if [ "$ui_planned" = true ]; then
  printf '{"ui_change":true,"source":"phase-domains","domains":[%s]}\n' "$domains_json"
else
  printf '{"ui_change":false,"source":"phase-domains","domains":[%s]}\n' "$domains_json"
fi
exit 0
