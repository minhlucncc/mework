module examples/remote-claude

go 1.25.7

require (
	github.com/jackc/pgx/v5 v5.10.0
	mework/libs/client v0.0.0
	mework/libs/sandbox v0.0.0
	mework/libs/server v0.0.0
	mework/libs/shared v0.0.0
)

replace (
	mework/libs/client => ../../libs/client
	mework/libs/sandbox => ../../libs/sandbox
	mework/libs/server => ../../libs/server
	mework/libs/shared => ../../libs/shared
)
