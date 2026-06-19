---
applied_by: 03-gh-go
applied_at: 2026-06-19
issue: 108
milestone: v0.1.7
audit: .github/audit/2026-06-19T08-37-41Z.md
---

# `enforce_admins` enabled on `main`

Surfaced by the repo audit on 2026-06-19 under
`Protection/WARN: enforce_admins disabled` in
`.github/audit/2026-06-19T08-37-41Z.md`.

## What changed

`enforce_admins.enabled` on the `main` branch protection rule is now
`true`. Admins can no longer bypass required status checks or
conversation resolution on pull requests targeting `main`.

The full protection payload was rewritten via the umbrella
`PUT /repos/{owner}/{repo}/branches/{branch}/protection` endpoint
with `enforce_admins: true` and `restrictions: null`. All previously
configured settings are preserved:

- `required_status_checks` (strict, `build-and-test`)
- `required_pull_request_reviews` (dismiss_stale_reviews=true,
  required_approving_review_count=0)
- `required_linear_history` = true
- `allow_force_pushes` = false
- `allow_deletions` = false
- `required_conversation_resolution` = true

## Verification

```text
$ gh api repos/danieljustus/symaira-fetch/branches/main/protection \
    | jq '.enforce_admins'
{
  "url": "https://api.github.com/repos/danieljustus/symaira-fetch/branches/main/protection/enforce_admins",
  "enabled": true
}
```

Closes #108
