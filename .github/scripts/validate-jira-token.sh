#!/usr/bin/env bash
set -euo pipefail

JIRA_BASE_URL="${JIRA_BASE_URL:-https://redhat.atlassian.net}"

if [[ ! "${JIRA_BASE_URL}" =~ ^https://[a-zA-Z0-9-]+\.atlassian\.net$ ]]; then
  echo "JIRA_BASE_URL must be an HTTPS atlassian.net host — got: ${JIRA_BASE_URL}"
  exit 1
fi

if [[ -z "${QUARANTINE_JIRA_API_TOKEN:-}" ]]; then
  echo "QUARANTINE_JIRA_API_TOKEN is not set — skipping validation"
  echo "status=missing" >> "${GITHUB_OUTPUT:-/dev/stdout}"
  exit 0
fi

if [[ -z "${JIRA_USER_EMAIL:-}" ]]; then
  echo "JIRA_USER_EMAIL is not set — skipping validation"
  echo "status=missing" >> "${GITHUB_OUTPUT:-/dev/stdout}"
  exit 0
fi

auth=$(printf '%s' "${JIRA_USER_EMAIL}:${QUARANTINE_JIRA_API_TOKEN}" | base64 | tr -d '\n')

curl_exit=0
http_code=$(curl -s -o /dev/null -w "%{http_code}" \
  --connect-timeout 10 \
  --max-time 30 \
  -H "Authorization: Basic ${auth}" \
  "${JIRA_BASE_URL}/rest/api/3/myself") || curl_exit=$?

if [[ "${curl_exit}" -eq 28 ]]; then
  echo "Jira probe timed out — network may be unreachable"
  echo "status=timeout" >> "${GITHUB_OUTPUT:-/dev/stdout}"
elif [[ "${curl_exit}" -ne 0 ]]; then
  echo "curl failed with exit code ${curl_exit}"
  exit "${curl_exit}"
elif [[ "${http_code}" == "200" ]]; then
  echo "status=valid" >> "${GITHUB_OUTPUT:-/dev/stdout}"
elif [[ "${http_code}" == "401" || "${http_code}" == "403" ]]; then
  echo "Jira token returned HTTP ${http_code} — token may be expired or invalid"
  echo "Rotate it at: https://id.atlassian.com/manage-profile/security/api-tokens"
  echo "status=expired" >> "${GITHUB_OUTPUT:-/dev/stdout}"
else
  echo "Jira probe returned unexpected HTTP ${http_code} — not a credential issue"
  echo "status=unknown" >> "${GITHUB_OUTPUT:-/dev/stdout}"
fi