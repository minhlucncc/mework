## 1. Object-store-backed artifact store (TDD)

- [ ] 1.1 Test: put→list→get round-trip via the `fs` driver (temp dir); the dummy "not yet
      wired" error is gone.
- [ ] 1.2 Implement an `ObjectStore`-backed store over `storage.Manager`, keying artifacts at
      `runs/<runID>/artifacts/<name>`.
- [ ] 1.3 Sanitize `name` (reject `..`/separators); test traversal is rejected.

## 2. Wire + serve (TDD)

- [ ] 2.1 `hub.NewServer` constructs the real store from `cfg.Storage` (default `fs`).
- [ ] 2.2 `/runs/{runID}/artifacts` (list) and `/runs/{runID}/artifacts/{name}` (download)
      serve real data; presign where the driver supports it, else proxy-stream.
- [ ] 2.3 Test: cross-tenant download is denied via the auth context.

## 3. Validation

- [ ] 3.1 `make vet` + `make test` green.
- [ ] 3.2 `openspec validate c0043-artifact-store --strict` passes.
