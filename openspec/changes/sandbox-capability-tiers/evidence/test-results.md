# Test results — sandbox-capability-tiers

This change touches only Go code. No Python, TypeScript, or portal modules were modified.

## Go: libs/client

```
$ go test -race ./libs/client/...
ok  	mework/libs/client	2.019s
ok  	mework/libs/client/catalog	1.059s
ok  	mework/libs/client/cli	5.441s
ok  	mework/libs/client/cmd/mework	1.013s
ok  	mework/libs/client/enroll	1.019s
?   	mework/libs/client/osproc	[no test files]
ok  	mework/libs/client/runner	6.912s
ok  	mework/libs/client/subscribe	1.020s
?   	mework/libs/client/workspacefs	[no test files]
```

## Go: libs/mcp-server

```
$ go test -race ./libs/mcp-server/...
ok  	mework/libs/mcp-server	1.685s
```

## Go: libs/sandbox

```
$ go test -race ./libs/sandbox/...
ok  	mework/libs/sandbox	1.785s
ok  	mework/libs/sandbox/agent	1.029s
ok  	mework/libs/sandbox/cmd/mework-sandbox	1.049s
?   	mework/libs/sandbox/engine/cloudflare	[no test files]
?   	mework/libs/sandbox/engine/custom	[no test files]
?   	mework/libs/sandbox/engine/docker	[no test files]
ok  	mework/libs/sandbox/engine/local	1.020s
ok  	mework/libs/sandbox/runtime	1.011s
```

## Go: libs/shared

```
$ go test -race ./libs/shared/...
ok  	mework/libs/shared	1.213s
?   	mework/libs/shared/config	[no test files]
ok  	mework/libs/shared/core	1.012s
?   	mework/libs/shared/errors	[no test files]
?   	mework/libs/shared/grant	[no test files]
?   	mework/libs/shared/log	[no test files]
ok  	mework/libs/shared/plugin	1.010s
ok  	mework/libs/shared/policy	1.011s
ok  	mework/libs/shared/ports	1.012s
ok  	mework/libs/shared/providers/mello	1.014s
?   	mework/libs/shared/providers/mezon	[no test files]
ok  	mework/libs/shared/transport	1.011s
```

No DB-dependent pytest skips or e2e browser skips apply (no Python or TS toolchains touched).
