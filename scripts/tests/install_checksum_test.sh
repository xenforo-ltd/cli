#!/usr/bin/env bash
set -euo pipefail

parse_checksum() {
    local checksums_file="$1"
    local filename="$2"
    awk -v target="$filename" '
        NF >= 2 {
            name = $2
            sub(/^\*/, "", name)
            if (name == target) {
                print $1
                exit
            }
        }
    ' "$checksums_file"
}

assert_eq() {
    local expected="$1"
    local actual="$2"
    local message="$3"
    if [[ "$expected" != "$actual" ]]; then
        echo "FAIL: $message"
        echo "  expected: $expected"
        echo "  actual:   $actual"
        exit 1
    fi
}

assert_empty() {
    local actual="$1"
    local message="$2"
    if [[ -n "$actual" ]]; then
        echo "FAIL: $message"
        echo "  expected empty, got: $actual"
        exit 1
    fi
}

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
checksums="$tmp_dir/checksums.txt"

cat > "$checksums" <<'CHECKSUMS'
1111111111111111111111111111111111111111111111111111111111111111  xf-v1.2.3-linux-amd64.tar.gz
2222222222222222222222222222222222222222222222222222222222222222  xf-v1.2.3-linux-amd64.tar.gz.sig
3333333333333333333333333333333333333333333333333333333333333333 *xf-v1.2.3-darwin-arm64.tar.gz
CHECKSUMS

exact=$(parse_checksum "$checksums" "xf-v1.2.3-linux-amd64.tar.gz")
assert_eq "1111111111111111111111111111111111111111111111111111111111111111" "$exact" "exact filename match should select the correct hash"

collision=$(parse_checksum "$checksums" "xf-v1.2.3-linux-amd64.tar.gz.sig")
assert_eq "2222222222222222222222222222222222222222222222222222222222222222" "$collision" "substring collision should not affect exact match parsing"

star_line=$(parse_checksum "$checksums" "xf-v1.2.3-darwin-arm64.tar.gz")
assert_eq "3333333333333333333333333333333333333333333333333333333333333333" "$star_line" "parser should accept optional binary-marker prefix"

missing=$(parse_checksum "$checksums" "xf-v1.2.3-windows-amd64.zip")
assert_empty "$missing" "missing checksum entry should produce no hash"

echo "install_checksum_test.sh: PASS"
