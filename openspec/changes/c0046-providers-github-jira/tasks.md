## 1. GitHub Issues adapter (TDD)

- [ ] 1.1 Tests: `X-Hub-Signature-256` HMAC verify (valid/invalid/replay), `ParseTrigger` over
      an issue/PR comment, channel key, write-back against an `httptest` GitHub API.
- [ ] 1.2 Implement the adapter behind `provider.Adapter`; register it.

## 2. Jira adapter (TDD)

- [ ] 2.1 Tests: webhook verify (secret/JWT, valid/invalid), `ParseTrigger` over an issue
      comment, channel key, write-back against an `httptest` Jira API.
- [ ] 2.2 Implement the adapter behind `provider.Adapter`; register it.

## 3. Wiring

- [ ] 3.1 Blank-import both in `apps/mework-server`; confirm a `github`/`jira` connection
      activates the pipeline with no other changes.

## 4. Validation

- [ ] 4.1 `make vet` + `make test` green (adapter unit tests).
- [ ] 4.2 `openspec validate c0046-providers-github-jira --strict` passes.
