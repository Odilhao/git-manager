# `--json` report shapes

`git-manager sync --json` and `git-manager status --json` each print one
JSON object to stdout: `sync.Report` (`internal/sync/run.go`) and
`status.Report` (`internal/status/status.go`) respectively. Both are
produced with `encoding/json`'s default field ordering and 2-space
indentation (`json.Encoder.SetIndent("", "  ")`).

**This schema is additive/forward-compatible.** New fields may be added to
either shape in a future release; a field present here is never renamed or
removed, and its JSON type never changes. Consumers (scripts, monitoring)
should ignore unknown fields rather than reject them.

## `sync.Report` (`sync --json`)

| Field | Type | Notes |
|---|---|---|
| `repos` | array of `RepoResult` | One entry per repo declared in the config, in declaration order. |
| `dry_run` | bool | True when `--dry-run` was passed; the reported activity is planned, not applied. |
| `overwrite` | bool | True when `--overwrite`/`--prune` was passed. |
| `error_count` | int | Count of `repos[].error` entries that are non-empty. |
| `duration_ms` | int64 | Wall-clock time the whole `Run()` call took, in milliseconds. Not a sum of the per-repo durations — this stays correct even if repos are ever synced concurrently. |

### `RepoResult`

| Field | Type | Notes |
|---|---|---|
| `name` | string | The repo's name from the config. |
| `path` | string | The resolved local checkout path. |
| `cloned` | bool | True if this run cloned (or, in dry-run, would clone) the repo. |
| `remotes` | `RemoteReport` | What remote reconciliation changed (or planned). See below. |
| `identity` | `IdentityReport` | What identity/signing config was written (or planned). See below. |
| `fetches` | array of `FetchResult`, omitted if empty | One entry per declared remote that was fetched. |
| `outcome` | string | One of `"success"`, `"partial"`, `"failure"` — derived from `error` and the activity fields above, never set independently. `"success"`: no error. `"partial"`: error set, but `cloned`, `remotes.Added`/`Updated`, `identity.Written` or `fetches` already recorded some activity. `"failure"`: error set, nothing accomplished. |
| `duration_ms` | int64 | Wall-clock time syncing this one repo took, in milliseconds. |
| `error` | string, omitted if empty | The error that stopped this repo's sync, if any. |

`remotes` (`RemoteReport`): `Added`, `Updated`, `Removed` — each an array of
`{"Name": string, "URL": string}` (or `null` if empty). For `Removed`, `URL`
is the URL the remote had before deletion.

`identity` (`IdentityReport`): `Written` — an array of
`{"Key": string, "Value": string}` (or `null` if empty), one entry per git
config key actually set.

`fetches[]` (`FetchResult`): `{"remote": string, "report": FetchReport}`,
where `FetchReport` is `{"Mode": "all"|"glob"|"regex", "RefSpecs":
[string]|null, "Branches": [string]|null}`.

### Example (`sync --dry-run --json`, one repo, a fresh clone planned)

```json
{
  "repos": [
    {
      "name": "example-project",
      "path": "/home/octocat/code/work/example-project",
      "cloned": true,
      "remotes": {
        "Added": [
          {"Name": "origin", "URL": "git@github.com:example-org/example-project.git"}
        ],
        "Updated": null,
        "Removed": null
      },
      "identity": {
        "Written": null
      },
      "fetches": [
        {"remote": "origin", "report": {"Mode": "all", "RefSpecs": null, "Branches": null}}
      ],
      "outcome": "success",
      "duration_ms": 41,
      "error": ""
    }
  ],
  "dry_run": true,
  "overwrite": false,
  "error_count": 0,
  "duration_ms": 41
}
```

## `status.Report` (`status --json`)

`status` reshapes the same data `sync --dry-run` would collect into a
drift-oriented view. Its `outcome` and `duration_ms` fields are mirrored
unchanged from the underlying `sync.Report`/`sync.RepoResult` — status
performs no classification or timing of its own.

| Field | Type | Notes |
|---|---|---|
| `repos` | array of `RepoResult` | One entry per repo declared in the config. |
| `drifted` | bool | True if any repo has drift or an error. |
| `error_count` | int | Count of `repos[].error` entries that are non-empty. |
| `duration_ms` | int64 | Mirrors `sync.Report.DurationMS` — the underlying `sync.Run(..., DryRun: true)` call's wall-clock time. |

### `RepoResult`

| Field | Type | Notes |
|---|---|---|
| `name` | string | |
| `path` | string | |
| `drifted` | bool | True if `cloned`, any of `remotes.added`/`updated`/`removed`, `identity.mismatched`, or `error` is non-empty. |
| `cloned` | bool | True if a sync would clone this repo. |
| `remotes` | `RemoteDrift` | `{"added": [...], "updated": [...], "removed": [...]}`, each an array of `{"name": string, "url": string}`, omitted if empty. |
| `identity` | `IdentityDrift` | `{"mismatched": [...]}`, each an array of `{"key": string, "value": string}`, omitted if empty. |
| `outcome` | string | Mirrors the underlying `sync.RepoResult.Outcome` — see the sync schema above. |
| `duration_ms` | int64 | Mirrors the underlying `sync.RepoResult.DurationMS`. |
| `error` | string, omitted if empty | |

### Example (`status --json`, one repo with a stale origin URL)

```json
{
  "repos": [
    {
      "name": "example-project",
      "path": "/home/octocat/code/work/example-project",
      "drifted": true,
      "cloned": false,
      "remotes": {
        "updated": [
          {"name": "origin", "url": "git@github.com:example-org/example-project.git"}
        ]
      },
      "identity": {},
      "outcome": "success",
      "duration_ms": 12
    }
  ],
  "drifted": true,
  "error_count": 0,
  "duration_ms": 12
}
```
