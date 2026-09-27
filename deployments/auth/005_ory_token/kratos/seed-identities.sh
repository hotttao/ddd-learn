#!/bin/sh

# This is development data only. Passwords can be overridden by Compose
# environment variables; production identities must be provisioned by a
# protected management workflow.
set -eu

admin_url="${KRATOS_ADMIN_URL:-http://kratos:4434}"
alice_email="${ALICE_EMAIL:-alice@example.com}"
alice_password="${ALICE_PASSWORD:-Alice-password-2026}"
bob_email="${BOB_EMAIL:-bob@example.com}"
bob_password="${BOB_PASSWORD:-Bob-password-2026}"

wait_for_kratos() {
	printf 'Waiting for Kratos Admin API at %s...\n' "$admin_url"
	while ! wget -qO /dev/null "$admin_url/health/ready"; do
		sleep 1
	done
}

identity_exists() {
	email=$1
	wget -qO - "$admin_url/admin/identities?credentials_identifier=$(printf '%s' "$email" | sed 's/@/%40/g')" \
		| grep -Fq "\"email\":\"$email\""
}

create_identity() {
	name=$1
	email=$2
	password=$3
	role=$4

	if identity_exists "$email"; then
		printf 'Identity already exists: %s\n' "$email"
		return
	fi

	payload=$(cat <<EOF
{
  "schema_id": "default",
  "traits": {
    "email": "$email",
    "name": {"first": "$name"}
  },
  "metadata_admin": {
    "organization_id": "G",
    "role": "$role"
  },
  "credentials": {
    "password": {
      "config": {"password": "$password"}
    }
  }
}
EOF
)

	printf 'Creating %s (%s)...\n' "$name" "$email"
	wget -qO /dev/null \
		--header='Content-Type: application/json' \
		--post-data="$payload" \
		"$admin_url/admin/identities"
}

wait_for_kratos
create_identity "Alice" "$alice_email" "$alice_password" "admin"
create_identity "Bob" "$bob_email" "$bob_password" "member"
printf 'Kratos seed completed.\n'
