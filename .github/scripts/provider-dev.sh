#!/usr/bin/env bash
set -euo pipefail

source_dir=$(cd -- "${PROVIDER_DIR:?Use make dev PROVIDER_DIR=/path/to/provider}" && pwd)
manifest="$source_dir/manifest.json"
provider=$(jq -er '.provider_type | strings | select(test("^[a-z0-9][a-z0-9_.-]*$"))' "$manifest")
binary=$(jq -er '.binary | strings | select(test("^[a-z0-9][a-z0-9_.-]*$"))' "$manifest")
icon=$(jq -r '.icon // empty' "$manifest")
case "$icon" in
    */*|*\\*|.|..) printf 'Icon must be a file next to manifest.json: %s\n' "$icon" >&2; exit 1 ;;
esac

mkdir -p -- "$PLUGIN_ROOT"
root=$(cd -- "$PLUGIN_ROOT" && pwd)
destination="$root/$provider"
stage="$root/.$provider-build"
if ! (umask 077; mkdir -- "$stage"); then
    printf 'Cannot stage %s in %s\n' "$provider" "$stage" >&2
    exit 1
fi
cleanup() {
    if [[ -e "$stage/previous" || -L "$stage/previous" ]] && [[ ! -e "$destination" && ! -L "$destination" ]]; then
        mv -- "$stage/previous" "$destination" || return
    fi
    rm -rf -- "$stage"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir -m 755 -- "$stage/package"
(
    cd -- "$source_dir"
    GOWORK="${GOWORK:-off}" go build -mod=readonly -o "$stage/package/$binary" .
)
chmod 755 "$stage/package/$binary"
cp -- "$manifest" "$stage/package/manifest.json"
if [[ -n "$icon" ]]; then
    cp -- "$source_dir/$icon" "$stage/package/$icon"
fi
if [[ -e "$destination" || -L "$destination" ]]; then
    mv -- "$destination" "$stage/previous"
fi
mv -- "$stage/package" "$destination"
printf 'Plugin ready at %s\nCERTIMATE_PLUGIN_DIR=%q\n' "$destination" "$root"
