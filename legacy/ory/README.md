# Ory auth/authz playground

These files describe the smallest local topology used by the example:

* Kratos owns browser identities and sessions.
* Talos owns service API keys and derives short-lived JWTs.
* Oathkeeper is the only gateway in front of `xhs_service`.
* Keto answers relationship checks; `xhs_service` keeps business data in memory.

The YAML files are intentionally examples rather than production secrets. In a
real deployment, put signing keys and database URLs in a secret manager. The
Talos admin API must remain on an internal network (it has no built-in auth).

Typical flow:

1. Create a Talos API key and call its internal `apiKeys:derive` endpoint.
2. Send the returned JWT to Oathkeeper as `Authorization: Bearer ...`.
3. Oathkeeper validates Talos' JWKS and issues the internal JWT consumed by xhs.
4. xhs asks Keto whether `service_actor` may access the requested resource.

