package searchserver

import (
	"database/sql"
	"strconv"
)

// This file contains custom-built sqlite converters. We use these instead of
// rows.Scan(...) directly because the latter is a very slow function.

func tobool(bts sql.RawBytes) bool {
	return bts[0] == '1'
}

func toint32(bts sql.RawBytes) int32 {
	// ignore error. Note that this isn't really advisable in most cases.
	val, _ := strconv.Atoi(string(bts))
	return int32(val)
}

func toint64(bts sql.RawBytes) int64 {
	// ignore error. Note that this isn't really advisable in most cases.
	val, _ := strconv.ParseInt(string(bts), 10, 64)
	return val
}
