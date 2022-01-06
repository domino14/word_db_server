package rpc

//go:generate protoc --twirp_out=. --twirp_opt=paths=source_relative --go_out=. --go_opt=paths=source_relative ./wordsearcher/searcher.proto
