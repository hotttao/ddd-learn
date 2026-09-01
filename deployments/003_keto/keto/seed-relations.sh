#!/bin/sh

# This is development data only. Production relationship changes must go
# through an authenticated management workflow.
set -eu

kratos_admin_url="${KRATOS_ADMIN_URL:-http://kratos:4434}"
keto_write_url="${KETO_WRITE_URL:-http://keto:4467}"
alice_email="${ALICE_EMAIL:-alice@example.com}"
bob_email="${BOB_EMAIL:-bob@example.com}"
organization_id="${ORGANIZATION_ID:-G}"

wait_for_url() {
	url=$1
	printf 'Waiting for %s...\n' "$url"
	while ! curl -fsS "$url" >/dev/null; do
		sleep 1
	done
}

email_query() {
	printf '%s' "$1" | sed 's/@/%40/g'
}

identity_id() {
	email=$1
	response=$(curl -fsS "$kratos_admin_url/admin/identities?credentials_identifier=$(email_query "$email")")
	id=$(printf '%s' "$response" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -n 1)
	if [ -z "$id" ]; then
		printf 'Unable to find Kratos identity: %s\n' "$email" >&2
		exit 1
	fi
	printf '%s' "$id"
}

write_user_tuple() {
	relation=$1
	subject_id=$2
	payload=$(cat <<EOF
{
  "namespace": "Organization",
  "object": "$organization_id",
  "relation": "$relation",
  "subject_id": "User:$subject_id"
}
EOF
)
	write_payload "$payload" "Organization:$organization_id#$relation@User:$subject_id"
}

write_subject_set_tuple() {
	relation=$1
	subject_relation=$2
	payload=$(cat <<EOF
{
  "namespace": "Organization",
  "object": "$organization_id",
  "relation": "$relation",
  "subject_set": {
    "namespace": "Organization",
    "object": "$organization_id",
    "relation": "$subject_relation"
  }
}
EOF
)
	write_payload "$payload" "Organization:$organization_id#$relation@Organization:$organization_id#$subject_relation"
}

write_payload() {
	payload=$1
	description=$2
	status=$(curl -sS -o /dev/null -w '%{http_code}' \
		-X PUT "$keto_write_url/admin/relation-tuples" \
		-H 'Content-Type: application/json' \
		--data "$payload")
	case "$status" in
		2??|409)
			printf 'Keto tuple ready: %s (HTTP %s)\n' "$description" "$status"
			;;
		*)
			printf 'Unable to write Keto tuple (HTTP %s): %s\n' "$status" "$payload" >&2
			exit 1
			;;
	esac
}

wait_for_url "$kratos_admin_url/health/ready"
wait_for_url "$keto_write_url/health/ready"

alice_id=$(identity_id "$alice_email")
bob_id=$(identity_id "$bob_email")

# Alice is the organization administrator.
write_user_tuple admins "$alice_id"
# Bob is an ordinary organization member.
write_user_tuple members "$bob_id"

# Organization G owns all three operations. Entitlements are projected to all
# roles so this layer describes the organization ceiling, not role grants.
for role in members admins; do
	write_subject_set_tuple entitled_start_crawl "$role"
	write_subject_set_tuple entitled_view_content "$role"
	write_subject_set_tuple entitled_modify_keywords "$role"
done

# Both roles can start a crawl and view content.
for role in members admins; do
	write_subject_set_tuple granted_start_crawl "$role"
	write_subject_set_tuple granted_view_content "$role"
done

# Only administrators can modify crawl keywords.
write_subject_set_tuple granted_modify_keywords admins

printf 'Keto relationship seed completed.\n'
