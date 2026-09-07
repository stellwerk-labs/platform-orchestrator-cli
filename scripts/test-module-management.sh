#!/usr/bin/env bash
set -euo pipefail

# Exercises the built client against a disposable local Core installation.
# Published history is intentionally retained and archived, never factory-reset.
octl=${1:?Usage: test-module-management.sh /path/to/octl}
[[ -x "$octl" ]] || { echo 'The octl binary is not executable' >&2; exit 1; }
case "${PO_API_URL:-}" in
  http://127.0.0.1:*|http://localhost:*) ;;
  *) echo 'This acceptance script only targets a loopback Core API' >&2; exit 1 ;;
esac
: "${PO_ORG_ID:?Set the disposable acceptance organization}"
: "${PO_AUTH_TOKEN:?Set the local acceptance bearer token}"
command -v jq >/dev/null
module_id="cli-release-$(uuidgen | tr '[:upper:]' '[:lower:]')"
call() { "$octl" "$@" --out json; }
check() { jq -e "$1" >/dev/null; }
publish() {
  jq -n --arg version "$1" --arg color "$2" '{
    semantic_version: $version, module_source: "inline",
    module_source_code: "variable \"color\" { type = string }\noutput \"name\" { value = var.color }",
    module_inputs: {color: $color}, module_params: {}, provider_mapping: {}, dependencies: {}, coprovisioned: []
  }' | call create module-version "$module_id" --set-json - --idempotency-key "publish-$1"
}
transition() {
  local version=$1 action=$2 revision
  revision=$(call get module-version "$module_id" "$version" | jq -r '.version.resource_version')
  call update module-version "$module_id" "$version" --action "$action" --expected-version "$revision" --reason 'CLI release acceptance'
}

call create resource-type "$module_id" --set-json '{"is_developer_accessible":true,"output_schema":{"type":"object","properties":{"name":{"type":"string"}}}}' | check '.id != null'
jq -n --arg id "$module_id" '{slug:$id, resource_type:$id, display_name:"CLI Release Service"}' |
  call create module-catalogue-entry --set-json - | check '.current_default_version_uuid == null'
first=$(publish 1.0.0 blue)
check '.lifecycle_status == "proposed" and .verification_status == "unverified"' <<<"$first"
[[ $(publish 1.0.0 blue | jq -r '.uuid') == $(jq -r '.uuid' <<<"$first") ]]
transition 1.0.0 promote | check '.lifecycle_status == "default"'
publish 1.1.0 green | check '.lifecycle_status == "proposed"'
call get module-version-comparison "$module_id" 1.0.0 1.1.0 |
  check '.before.module_inputs.color == "blue" and .after.module_inputs.color == "green"'
transition 1.1.0 promote | check '.lifecycle_status == "default"'
transition 1.1.0 mark-defective | check '.lifecycle_status == "defective"'
call get module-catalogue-entry "$module_id" | check '.current_default_version_uuid == null'
transition 1.0.0 restore | check '.lifecycle_status == "default"'
call get module-versions "$module_id" --include-deprecated --include-defective | check 'length == 2'
call get module-version-history "$module_id" 1.1.0 |
  check 'length == 3 and .[2].reason == "CLI release acceptance" and .[2].actor != null'
call get module-version-usage "$module_id" 1.0.0 | check 'type == "object"'
revision=$(call get module-catalogue-entry "$module_id" | jq -r '.resource_version')
call update module-catalogue-status "$module_id" --action archive --expected-version "$revision" --reason 'Retain CLI acceptance history' |
  check '.status == "archived"'
call get module-version "$module_id" 1.0.0 | check '.version.uuid != null'
"$octl" get module-catalogue --include-archived | grep -Fq "$module_id"
"$octl" get module-catalogue-entry "$module_id" | grep -Fq 'archived'
"$octl" get module-versions "$module_id" --include-defective --include-deprecated | grep -Fq 'defective'
"$octl" get module-version "$module_id" 1.0.0 | grep -Fq '1.0.0'
"$octl" get module-version-history "$module_id" 1.1.0 | grep -Fq 'CLI release acceptance'
"$octl" get module-version-comparison "$module_id" 1.0.0 1.1.0 | grep -Fq '"color":"green"'
"$octl" get module-version-usage "$module_id" 1.0.0 | grep -Fq 'Active environments:'
echo "PASS: real CLI publication, replay, comparison, promotion, defective, restoration, history, usage, archived retention and default table output ($module_id)"
