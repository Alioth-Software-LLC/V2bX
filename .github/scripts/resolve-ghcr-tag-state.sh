#!/usr/bin/env bash
set -euo pipefail

if (( $# != 1 )); then
  echo "Usage: $0 EXPECTED_MANIFEST_FILE" >&2
  exit 1
fi

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "Required environment variable ${name} is empty" >&2
    exit 1
  fi
}

require_env REGISTRY
require_env IMAGE_NAME
require_env IMAGE_VERSION

expected_manifest_file="$1"
if [[ ! -f "${expected_manifest_file}" || ! -r "${expected_manifest_file}" ]]; then
  echo "Expected manifest file is not readable: ${expected_manifest_file}" >&2
  exit 1
fi

if [[ "${REGISTRY}" != "ghcr.io" ]]; then
  echo "Unsupported registry host: ${REGISTRY}" >&2
  exit 1
fi
if [[ ! "${IMAGE_NAME}" =~ ^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)+$ ]]; then
  echo "Invalid lowercase container image name: ${IMAGE_NAME}" >&2
  exit 1
fi
if [[ ! "${IMAGE_VERSION}" =~ ^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$ ]]; then
  echo "Invalid container image tag: ${IMAGE_VERSION}" >&2
  exit 1
fi

normalize_image_index() {
  jq -ceS -s '
    if length != 1 then
      error("expected exactly one JSON document")
    else
      .[0]
      | if type != "object"
          or .schemaVersion != 2
          or ((.mediaType == "application/vnd.oci.image.index.v1+json"
            or .mediaType == "application/vnd.docker.distribution.manifest.list.v2+json") | not)
          or (.manifests | type) != "array"
          or (.manifests | length) == 0
          or ([.manifests[].digest
            | select((type == "string" and test("^sha256:[0-9a-f]{64}$")) | not)] | length) != 0
        then
          error("invalid container image index")
        else
          .manifests |= sort_by(.digest)
        end
    end
  ' "$@"
}

if ! expected_manifest="$(normalize_image_index "${expected_manifest_file}")"; then
  echo "Expected manifest is not a valid container image index" >&2
  exit 1
fi

if [[ -n "${GHCR_TOKEN:-}" ]]; then
  require_env GHCR_USERNAME
  token_payload="$({
    curl --silent --show-error --fail --location \
      --retry 3 --retry-all-errors \
      --user "${GHCR_USERNAME}:${GHCR_TOKEN}" \
      --get "https://${REGISTRY}/token" \
      --data-urlencode "service=${REGISTRY}" \
      --data-urlencode "scope=repository:${IMAGE_NAME}:pull"
  })"
else
  token_payload="$({
    curl --silent --show-error --fail --location \
      --retry 3 --retry-all-errors \
      --get "https://${REGISTRY}/token" \
      --data-urlencode "service=${REGISTRY}" \
      --data-urlencode "scope=repository:${IMAGE_NAME}:pull"
  })"
fi
registry_token="$({
  jq -er -s '
    if length != 1 then
      error("expected exactly one token response")
    else
      (.[0].token // .[0].access_token)
    end
    | if type == "string" and length > 0 then
        .
      else
        error("registry token is missing")
      end
  ' <<<"${token_payload}"
})"

manifest_accept='application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json'
manifest_url="https://${REGISTRY}/v2/${IMAGE_NAME}/manifests/${IMAGE_VERSION}"
manifest_status="$({
  curl --silent --show-error --location \
    --retry 3 --retry-all-errors \
    --output /dev/null --write-out '%{http_code}' --head \
    --header "Authorization: Bearer ${registry_token}" \
    --header "Accept: ${manifest_accept}" \
    "${manifest_url}"
})"

case "${manifest_status}" in
  404)
    printf '%s\n' absent
    exit 0
    ;;
  200)
    manifest_payload="$({
      curl --silent --show-error --fail --location \
        --retry 3 --retry-all-errors \
        --header "Authorization: Bearer ${registry_token}" \
        --header "Accept: ${manifest_accept}" \
        "${manifest_url}"
    })"
    if ! published_manifest="$(normalize_image_index <<<"${manifest_payload}")"; then
      echo "Published tag does not contain a valid container image index" >&2
      exit 1
    fi
    if [[ "${published_manifest}" != "${expected_manifest}" ]]; then
      echo "Refusing to overwrite published image ${REGISTRY}/${IMAGE_NAME}:${IMAGE_VERSION}: manifest differs" >&2
      exit 1
    fi
    printf '%s\n' matching
    exit 0
    ;;
  *)
    echo "Cannot resolve image tag state: registry returned HTTP ${manifest_status}" >&2
    exit 1
    ;;
esac
