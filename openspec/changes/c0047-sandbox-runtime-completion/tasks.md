## 1. mework-sandbox binary (TDD)

- [ ] 1.1 Smoke test: the binary selects an engine and runs start→exec(stdin)→stop with the
      `local` engine (no real Claude needed — stub backend).
- [ ] 1.2 Implement engine selection + a `runtime.Manager`-driven execution surface; graceful
      shutdown; clear errors. Add a `mework-sandbox` Makefile build target.

## 2. Cloudflare engine lifecycle (TDD)

- [ ] 2.1 Tests: `Mount`/`Stop`/`Destroy`/`Signals` against a stub remote; where unsupported,
      a typed `ErrUnsupported` is returned and `Caps()` reflects it.
- [ ] 2.2 Implement the lifecycle methods (or typed-unsupported); no silent no-ops.

## 3. Resolve the permission stub

- [ ] 3.1 Verify nothing imports `libs/server/permission`; remove the empty package (or
      implement real helpers used by the middleware). Update any references.

## 4. Validation

- [ ] 4.1 `make vet` + `make build` (incl. `mework-sandbox`) + `make test` green.
- [ ] 4.2 `openspec validate c0047-sandbox-runtime-completion --strict` passes.
