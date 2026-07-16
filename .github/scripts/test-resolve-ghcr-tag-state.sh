#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
resolver="${script_dir}/resolve-ghcr-tag-state.sh"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

mkdir -p "${tmp_dir}/bin"
cat > "${tmp_dir}/bin/curl" <<'MOCK_CURL'
#!/usr/bin/env bash
set -euo pipefail

args=" $* "
if [[ "${args}" == *" https://ghcr.io/token "* ]]; then
  if [[ "${MOCK_STATE}" == "multiple-token-documents" ]]; then
    printf '%s\n%s\n' '{"errors":[{"code":"temporary"}]}' '{"token":"registry-token"}'
  else
    printf '%s\n' '{"token":"registry-token"}'
  fi
  exit 0
fi

if [[ "${args}" == *" --head "* ]]; then
  if [[ "${MOCK_STATE}" == "absent" ]]; then
    printf '%s' 404
  else
    printf '%s' 200
  fi
  exit 0
fi

if [[ "${args}" == *"/manifests/"* ]]; then
  cat "${MOCK_ACTUAL_MANIFEST}"
  exit 0
fi

echo "Unexpected curl invocation: $*" >&2
exit 1
MOCK_CURL
chmod +x "${tmp_dir}/bin/curl"

cat > "${tmp_dir}/expected.json" <<'JSON'
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "size": 101,
      "platform": {"architecture": "amd64", "os": "linux"}
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "size": 202,
      "platform": {"architecture": "arm64", "os": "linux"}
    }
  ]
}
JSON

cat > "${tmp_dir}/matching.json" <<'JSON'
{
  "manifests": [
    {
      "platform": {"os": "linux", "architecture": "arm64"},
      "size": 202,
      "digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "mediaType": "application/vnd.oci.image.manifest.v1+json"
    },
    {
      "platform": {"os": "linux", "architecture": "amd64"},
      "size": 101,
      "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "mediaType": "application/vnd.oci.image.manifest.v1+json"
    }
  ],
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "schemaVersion": 2
}
JSON

cat > "${tmp_dir}/mismatch.json" <<'JSON'
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "size": 303,
      "platform": {"architecture": "amd64", "os": "linux"}
    }
  ]
}
JSON

run_resolver() {
  local state="$1"
  local actual_manifest="$2"
  env \
    PATH="${tmp_dir}/bin:${PATH}" \
    MOCK_STATE="${state}" \
    MOCK_ACTUAL_MANIFEST="${actual_manifest}" \
    REGISTRY=ghcr.io \
    IMAGE_NAME=alioth-software-llc/v2bx \
    IMAGE_VERSION=v0.4.1-alioth.1 \
    "${resolver}" "${tmp_dir}/expected.json"
}

[[ "$(run_resolver absent "${tmp_dir}/matching.json")" == "absent" ]] \
  || fail "absent tag was not classified as absent"

[[ "$(run_resolver matching "${tmp_dir}/matching.json")" == "matching" ]] \
  || fail "equivalent manifest was not classified as matching"

if run_resolver mismatch "${tmp_dir}/mismatch.json" >/dev/null 2>&1; then
  fail "different manifest was accepted"
fi

if run_resolver multiple-token-documents "${tmp_dir}/matching.json" >/dev/null 2>&1; then
  fail "multiple token response documents were accepted"
fi

printf '%s\n' "resolve-ghcr-tag-state tests passed"
