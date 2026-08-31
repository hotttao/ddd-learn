# Talos local setup

Run Talos with its OSS config from the vendored submodule and expose the admin
API only to the local Docker network. Create an API key through the admin API,
then derive a JWT with `iss=talos`, `aud=xhs-api`, and a short `exp` (for
example five minutes). Set the stable machine identity in the `act` claim;
Oathkeeper copies that claim to `service_actor` in its internal token.

Talos' derived-key JWKS endpoint is consumed by Oathkeeper. Do not publish the
Talos admin endpoint or commit signing keys to this repository.

