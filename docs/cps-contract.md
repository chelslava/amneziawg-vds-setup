# CPS I1–I5 extension contract (design only)

This document defines a future, explicit opt-in extension. v2 does not generate, import, persist, or apply CPS values automatically.

The syntax and packet model follow the [AmneziaWG documentation](https://docs.amnezia.org/documentation/amnezia-wg/): each configured `I1`–`I5` is a separate packet in order, and a missing item does not shift the remaining items. A CPS value is a protocol fingerprint, not a generic password; treat it as sensitive operational configuration.

## Modes and UX

The future interface must require an explicit submode, for example `--cps-mode disabled|import|generate`, and a confirmation showing engine, target, and packet names. The default is `disabled`. `import` accepts a local, permission-checked file; `generate` requires a documented template and a confirmation that the generated signature is not a reusable example.

No CPS flag may be silently inferred from a client configuration, server state, image tag, or engine selection. Legacy → Upstream remains a separate installation, never a migration.

## Ownership and matching

| Field family | Ownership | Contract |
| --- | --- | --- |
| `I1`–`I5` | coordinated client/server protocol profile | Preserve order and packet presence. Validate the same profile on both ends where the selected AmneziaWG implementation requires it; never assume all five must exist. |
| `Jc`, `Jmin`, `Jmax` | per-endpoint transport behavior | Validate ranges independently; do not force equality with `I1`–`I5` or with `S*`/`H*`. |
| `S1`–`S4` | coordinated transport parameters | Match according to the selected protocol implementation and client/server role; do not apply a blanket “all values equal” rule. |
| `H1`–`H4` | implementation-specific dynamic header ranges | Store and validate as ranges when the implementation supports ranges; never collapse them into one static integer. |

Legacy/ WireSock compatibility must keep CPS disabled unless a separately verified client and server pair is selected. Upstream may expose CPS only after a capability probe and explicit documentation of the exact image/module/client versions. A configuration that cannot prove compatibility must fail closed before changing the server.

## State and secret boundaries

State may contain only `cps_mode`, `cps_profile_id`, `cps_fields_present`, `cps_source`, and `cps_sha256`. It must not contain raw `I1`–`I5`, generated random material, client configs, private keys, or passwords. The imported file is temporary, mode `0600`, and removed after the validated deployment artifact is created. Logs show field names and validation errors, never values.

## Validation, rollback, and migration

Validation must reject unknown tags, malformed hex, duplicate semantic tags where the selected parser forbids them, oversized `r`/`rc`/`rd` lengths, invalid timestamps/ranges, and an empty profile selected for `import`. It must run before any remote mutation. Deployment must create a full backup first, write the candidate atomically, run engine-specific health checks, and restore the backup on failure. Changing CPS mode or profile is an explicit reconfigure operation with a confirmation; it is not `update` and never migrates engines.

## Fixtures required before implementation

`docs/fixtures/cps-fixtures.json` contains only non-operational parser fixtures: valid syntax, missing middle packet, invalid hex, oversized random tag, and an empty import. These fixtures test parsing and redaction without embedding a real protocol signature.
